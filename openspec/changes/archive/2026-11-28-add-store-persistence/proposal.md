# Proposal

## Why

`internal/rules` 已完成 QKMJ 95p3+ 全部純狀態機與計分規則；為了讓後續 `internal/table` 與 `internal/lobby` 具備跨重啟、跨節點一致的帳號認證、95p3+ 經濟結算、偏好設定與低介入管理能力，現在需要建立 `internal/store` 持久化層與本地／rqlite Docker Compose 執行環境。

## What Changes

- **升級現代密碼雜湊基線（`architecture`）**：將安全基線的密碼雜湊演算法由 `bcrypt (cost >= 12)` 演進為 **Argon2id**（OWASP 推薦 memory-hard 參數、PHC 格式、常數時間比對，密碼長度 8–128 bytes），同步對齊現代密碼安全標準。
- **建立 `store/account-economy` 能力**：
  - 帳號認證與管理：名稱限定 `^[0-9A-Za-z]{1,20}$`（保留大小寫顯示、唯一性比對不分大小寫）、保留 `qkmj` 字頭禁止真人註冊與登入、Argon2id 雜湊與改密碼（`/passwd`）、不提供 `/suicide`（杜絕負債洗錢與刷榜）。
  - QKMJ 95p3+ 經濟與戰績：新帳號初始賭幣 `20000`、登入救濟（`balance < 20000 && games_played >= 16` 自動補回 `20000`）、開桌不設負數門檻、`games_played >= 16` 計算全服並列排名；累計全服統一的 `balance`、`games_played`、`self_draw_wins`、`discard_wins`、`deal_ins`、`draws`、`abandoned_hands`、`login_count`、`last_login_at` 與 `last_node_name`。
  - 內建真實 AI 帳號與零和結算：資料庫初始化冪等建立 `QKMJ0`–`QKMJ9`（`RoleBot`）共 10 個真實帳號；`SettleHand` 以單一原子批次寫入 `hand_settlements`（含 `hand_id` 冪等鍵、`node_name` 來源節點與四家明細）並以**相對增量**更新四家（真人與 AI 一視同仁）餘額與戰績。
  - 輕量 RBAC、稽核與玩家自治：`account_roles`（`player`、`moderator`、`admin`、`bot`）搭配強型別權限、`accounts.banned_until/banned_reason` 跨節點封鎖與 `moderation_logs` 不可變稽核軌跡；`user_preferences`（取代 `.qkmjrc`，跨 SSH/Web 同步）與 `user_blocks`（個人黑名單，暫不做 `user_friends`）。
- **建立 `store/persistence-backends` 能力**：
  - 提供語意化原子儲存介面（絕不外洩 `*sql.Tx`），同時支援純 Go SQLite（`modernc.org/sqlite`，本地開發／測試／單機）與 `rqlite`（純 Go `net/http` 參數化原子批次 `?transaction=true`，支援 1–3 節點 Raft HA Sidecar）。
  - 啟動時自動執行內嵌版本化 `schema_migrations` 與 `QKMJ0`–`QKMJ9` 冪等 seed。
- **建立本地建置 Docker Compose Stack 與最小啟動入口**：
  - 新增純 Go 多階段 `Dockerfile`、最小可啟動之 `cmd/qkmj`（讀取旗標／環境變數、連接 Store 完成 Migration／Seed 並支援優雅關機）以及 `compose.yaml`（`qkmj` 本地 build + 官方 `rqlite/rqlite` image，支援單節點預設與 `ha` 3 節點 Raft profile）。

## Capabilities

### New Capabilities
- `store/account-economy`: 定義 QKMJ 95p3+ 帳號認證（Argon2id、`[0-9A-Za-z]{1,20}`、`qkmj` 保留字頭與 `QKMJ0`–`QKMJ9` 內建 AI 帳號）、經濟救濟與排名、相對原子增量結算與來源節點稽核、輕量 RBAC 封鎖日誌、個人偏好與個人黑名單之持久化行為。
- `store/persistence-backends`: 定義 `internal/store` 語意化原子操作邊界、純 Go SQLite（`modernc.org/sqlite`）與 `rqlite` HTTP 原子批次雙後端契約、內嵌版本化自動 Migration、AI 帳號 Seed，以及 `qkmj` + 官方 `rqlite` Docker Compose 部署堆疊。

### Modified Capabilities
- `architecture`: 將「安全基線與外部輸入邊界驗證」之密碼雜湊規範由 `bcrypt (cost >= 12)` 升級為 `Argon2id`（OWASP 推薦參數與 PHC 格式）。

## Impact

- **新增套件與檔案**：`internal/store/*`、`cmd/qkmj/main.go`、`Dockerfile`、`compose.yaml`。
- **新增 Go 依賴**（`go.mod`）：`modernc.org/sqlite`、`golang.org/x/crypto`（`argon2`）；`rqlite` 後端直接以 Go stdlib `net/http` 實作，不增加額外 rqlite 第三方套件。
- **規範與文件更新**：同步更新 `openspec/specs/architecture/spec.md` 與 `AGENTS.md` 中的密碼安全基線敘述；備忘留待後續層（`table/*`、`lobby/*`、`tui/*`）：Password UX、整桌靜音模式、`/kick` 禁重入同桌與私人密碼桌。
