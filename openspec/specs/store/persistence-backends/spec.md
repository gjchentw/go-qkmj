# store/persistence-backends Specification

## Purpose

定義 `internal/store` 的語意化原子儲存邊界、純 Go SQLite（`modernc.org/sqlite`）與 `rqlite` HTTP 原子批次雙後端實作契約、內嵌版本化自動 Migration、AI 帳號 Seed，以及 `qkmj` 本地建置搭配官方 `rqlite` 映像檔之 Docker Compose 堆疊。

## Requirements

### Requirement: 語意化原子儲存邊界與零交易外洩
`internal/store` SHALL 僅對外公開語意化原子方法（如 `CreateAccount`、`Authenticate`、`SettleHand`、`BanAccount`、`SavePreferences`），MUST NOT 將 `*sql.Tx` 或底層驅動型別暴露至業務層。所有外部 I/O 呼叫 MUST 接受 `context.Context` 並套用明確逾時；所有 SQL 陳述 MUST 使用參數化查詢且禁止拼接使用者輸入；套件 MUST NOT 使用 `init()` 註冊可變全域狀態或全域資料庫連線單例。

#### Scenario: 呼叫端透過相同 Store 契約操作雙後端
- **WHEN** 測試或上層服務分別以 SQLite 與 `rqlite` 後端建立 `Store` 並執行完整帳號、結算、RBAC 與黑名單流程
- **THEN** 兩者具備完全一致之回傳結構、錯誤語意與原子性保證，且呼叫端不接觸任何 `*sql.Tx`

### Requirement: 純 Go SQLite 後端（本地開發與測試）
系統 SHALL 支援使用純 Go `modernc.org/sqlite`（支援檔案路徑與 `:memory:`）開啟 SQLite 後端，啟用 `PRAGMA foreign_keys = ON`、WAL 模式（檔案模式）與 `busy_timeout`，並將每個多語句語意操作包裝於單一資料庫交易中；批次中任一語句失敗 MUST 完整回滾。

#### Scenario: SQLite 多語句批次失敗自動回滾
- **WHEN** 透過 SQLite 後端執行 `SettleHand` 時其中一筆更新觸發約束錯誤或重複 `hand_id`
- **THEN** 整個批次完整回滾，`hand_settlements` 與所有帳號餘額均維持執行前狀態

### Requirement: rqlite HTTP 原子批次後端（1 至 3 節點 Raft HA）
系統 SHALL 支援以 Go 標準庫 `net/http` 實作 `rqlite` 後端，透過單一參數化原子請求（`POST /db/execute?transaction=true` 與 `POST /db/query`）與本機 sidecar 或多節點 endpoint 通訊，支援可設定之讀一致性（預設 `weak`）、請求逾時、節點失敗容錯與 HTTP 重導向跟隨。批次中任一語句在 rqlite 回傳錯誤時，整批交易 MUST 視為失敗。

#### Scenario: rqlite 後端以單一原子批次送出結算並處理錯誤
- **WHEN** 透過 `rqlite` 後端呼叫 `SettleHand`，測試伺服器檢查收到的 HTTP 請求載荷，並於另一案例回傳批次內語句錯誤
- **THEN** 請求包含 `transaction=true` 與完整參數化語句陣列；若伺服器回傳錯誤則 `SettleHand` 回傳包裝後錯誤且不誤判成功

#### Scenario: rqlite 多節點備援與逾時保護
- **WHEN** 第一個 rqlite endpoint 無法連線而第二個正常回應，或所有節點皆超時無回應
- **THEN** 前者自動切換至健康節點完成操作；後者依 `context` 逾時快速失敗而不永久阻塞

### Requirement: 啟動版本化自動 Migration 與冪等 Seed
呼叫 `Open` 初始化任一後端時，系統 SHALL 依版本順序冪等執行內嵌（`//go:embed`）SQL migrations 並記錄於 `schema_migrations`，完成後自動冪等建立 `QKMJ0`–`QKMJ9` 內建 AI 帳號。對已初始化之資料庫重複啟動 MUST 為無副作用之冪等操作。

#### Scenario: 重複啟動已遷移資料庫保持冪等
- **WHEN** 對同一份 SQLite 檔案或 rqlite 實例連續呼叫 `Open` 與 `Close` 兩次
- **THEN** `schema_migrations` 不重複套用既有版本，且既有玩家與 `QKMJ0`–`QKMJ9` 資料完整保留

### Requirement: 本地建置 qkmj 與官方 rqlite Docker Compose 堆疊
專案 SHALL 提供純 Go 多階段 `Dockerfile`（`CGO_ENABLED=0`、非 root 執行 `cmd/qkmj`）、最小可啟動之 `cmd/qkmj`（讀取旗標／環境變數初始化 `internal/store` 並支援 `SIGINT`/`SIGTERM` 優雅關機），以及 `compose.yaml`：以本地 source code 建置 `qkmj` 服務、以官方 `rqlite/rqlite` image 部署資料庫（預設單節點含健康檢查與持久卷，另提供 `ha` profile 啟動 3 節點 Raft 叢集）。

#### Scenario: Compose 堆疊設定符合本地建置與官方 rqlite 部署契約
- **WHEN** 檢查 `Dockerfile`、`compose.yaml` 與 `cmd/qkmj` 啟動流程
- **THEN** `qkmj` 透過本地 `build` 建置並以 `rqlite` 後端連線至官方 `rqlite/rqlite` 服務完成 Migration 與 AI Seed，收到停止訊號時乾淨關閉 Store 連線
