# Tasks

## 1. 依賴與專案規範對齊

- [x] 1.1 在 `go.mod` 新增 `modernc.org/sqlite` 與 `golang.org/x/crypto`（確認無任何本地 `replace`），更新 `AGENTS.md` 安全基線段落（密碼雜湊改為 Argon2id），並以 `go mod tidy && go test -race ./...` 驗證既有測試全綠

## 2. 領域型別、驗證、Argon2id 與 RBAC 模型（`store/account-economy`）

- [x] 2.1 以 TDD（RED -> GREEN -> REFACTOR）建立 `internal/store/types.go`、`internal/store/errors.go`、`internal/store/password.go` 與對應單元測試：實作帳號名稱驗證（`^[0-9A-Za-z]{1,20}$`、不分大小寫正規化、`qkmj` 保留字頭拒絕）、Argon2id PHC 雜湊與常數時間比對（8–128 bytes、預設 OWASP 參數 + 測試參數注入）、強型別列舉（`Role`、`Permission`、`ModerationAction`、`ReadConsistency`、`Account.HasPermission`），並通過 `make test-race`

## 3. 雙後端執行器與內嵌版本化 Migration（`store/persistence-backends`）

- [x] 3.1 以 TDD 建立 `internal/store/migrations/001_initial.sql`、`internal/store/backend.go`、`internal/store/sqlite.go` 與 `internal/store/migrate.go`：實作純 Go `modernc.org/sqlite` 後端（`foreign_keys=ON`、WAL、`:memory:` 單連線池、單一交易 `ExecBatch` 錯誤完整回滾）與 `//go:embed` 自動版本化 Migration + `QKMJ0`–`QKMJ9` 冪等 Seed，撰寫重複啟動冪等與回滾測試並通過 `make test-race`
- [x] 3.2 以 TDD 建立 `internal/store/rqlite.go` 與 `internal/store/rqlite_test.go`：以 Go stdlib `net/http` 實作 `rqlite` 參數化批次執行器（`POST /db/execute?transaction=true`、`POST /db/query?level=...`、多節點 failover、301 重導向跟隨、`context` 逾時保護與批次語句錯誤回傳），以 `httptest.Server` 驗證所有正常與異常路徑並通過 `make test-race`

## 4. 語意化 Store 帳號、經濟、結算、RBAC、偏好與黑名單（`store/account-economy`）

- [x] 4.1 以 TDD 建立 `internal/store/store.go`、`internal/store/account.go` 與測試：實作 Functional Options（`WithClock`、`WithArgon2Params`、`WithMaxBlocks`、`WithRequestTimeout`、`WithReadConsistency`、`WithHTTPClient`）、`CreateAccount`（初始 `20000`、預設 `player` 角色）、`Authenticate`（拒絕 `bot`/保留字頭與封鎖帳號、更新 `login_count`/`last_login_at`/`last_node_name`、滿 16 局且 `< 20000` 觸發登入救濟回傳 `ReliefGranted`）、`ChangePassword`、`GetAccount`、`GetAccountRank`（滿 16 局並列排名）與 `ListBotAccounts`，並通過 `make test-race`
- [x] 4.2 以 TDD 建立 `internal/store/settlement.go` 與測試：實作 `SettleHand`（驗證 4 家不重複、胡牌零和與和局全零、`hand_id` 冪等防重複、`node_name` 與四家明細寫入 `hand_settlements`、四家真人與 AI 一視同仁相對增量更新餘額與生涯戰績）與 `RecordAbandonedHand`（累加 `abandoned_hands`），並通過 `make test-race`
- [x] 4.3 以 TDD 建立 `internal/store/moderation.go`、`internal/store/preferences.go`、`internal/store/blocks.go` 與測試：實作 RBAC 權限檢查之 `BanAccount`、`UnbanAccount`、`AssignRole`、`RevokeRole`、`BootstrapAdmin` 與 `ListModerationLogs`（單一交易同步寫入 `moderation_logs`），以及 `GetPreferences`/`SavePreferences`（預設值與未知欄位容錯）與 `BlockUser`/`UnblockUser`/`IsBlocked`/`ListBlockedUsers`（禁封自己與上限保護），驗證 SQLite 與 rqlite 合約測試全通過 `make test-race`

## 5. 最小啟動入口與 Docker Compose Stack（`store/persistence-backends`）

- [x] 5.1 以 TDD 建立 `cmd/qkmj/main.go`、`cmd/qkmj/main_test.go`、`Dockerfile` 與 `compose.yaml`：實作最小可啟動 `cmd/qkmj`（支援旗標／環境變數選擇 `sqlite`/`rqlite`、自動產生 Docker 風格隨機 Node Name、`--bootstrap-admin`、啟動完成 Migration 與 Seed 並以 `SIGINT`/`SIGTERM` 優雅關閉 Store）、純 Go `CGO_ENABLED=0` 非 root 多階段 `Dockerfile`，以及本地 build `qkmj` + 官方 `rqlite/rqlite` 映像檔之 `compose.yaml`（單節點預設含 `/readyz` 健康檢查與持久卷 + `ha` 3 節點 Raft profile），以 `go test -race ./...`、`go build ./cmd/qkmj` 與 `docker compose config`（含 `--profile ha`）驗證

## 6. 品質閘門與 5 軸審查

- [x] 6.1 驗證 `internal/store` 無 `*sql.Tx` 外洩、無 `init()`／可變全域狀態且僅使用參數化 SQL，執行 `make check`、`make coverage-check`（覆蓋率 >= 80%）與 `GOWORK=off make check`，並完成 5 軸 `code-review-and-quality` 審查與所有 Critical/High 問題修復

## Workflow follow-up

- 使用者審查並確認後，將 `architecture` delta 與 `store/account-economy`、`store/persistence-backends` 同步至 `openspec/specs/` 並歸檔本變更。
- 歸檔完成後進入下一架構層（`internal/table` 與 `internal/bot`），並延續備忘清單（Password UX、整桌靜音模式、`/kick` 禁重入同桌、私人密碼桌）。
