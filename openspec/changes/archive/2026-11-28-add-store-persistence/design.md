# Design

## Context

`go-qkmj` 已完成純領域層 `internal/rules`（QKMJ 95p3+）。本變更建立持久化層 `internal/store`、最小啟動入口 `cmd/qkmj` 與本地建置／官方 `rqlite` 之 Docker Compose 堆疊（動機與範圍見 `proposal.md`，行為契約見三份規格檔）。

核心架構約束（對齊 `openspec/specs/architecture/spec.md` 與 `golang-design-patterns`）：
- `internal/store` 僅提供語意化原子方法，絕不外洩 `*sql.Tx` 或驅動內部型別，且嚴禁 `init()` 與可變全域狀態。
- `go-qkmj` 維持 100% 純 Go（`CGO_ENABLED=0`）：開發／測試用 `modernc.org/sqlite`，正式 1–3 節點 HA 透過 Go stdlib `net/http` 連線 sidecar `rqlited`。
- 禁止高頻 DB heartbeat；僅於低頻語意事件（註冊、登入、結算、偏好、黑名單、管理操作）寫入資料庫。

## Goals / Non-Goals

**Goals:**
- 以統一 `Store` 門面搭配內部 `executor` 抽象，讓 SQLite 與 `rqlite` 通過同一套語意契約測試套件（Contract Test Suite）。
- 以 `golang-design-patterns` 確保建構期參數驗證（Functional Options 回傳 `error`）、明確資源釋放（`Close()`）、外部 I/O 逾時（`context.Context`）、可注入時鐘與 Argon2id 參數（兼顧正式環境安全強度與測試速度）。
- 支援多節點水平擴展：以相對增量結算防 Lost Update、以 `hand_id` 防重試重複入帳、以 `node_name` 溯源來源節點與登入位置。
- 提供可直接 `docker compose up --build` 啟動並實測 `qkmj` + 官方 `rqlite/rqlite` 的本地建置堆疊（含單節點預設與 `ha` 3 節點 Raft profile）。

**Non-Goals:**
- 不實作 SSH／Web 閘道、TUI 介面、Lobby Actor、Table Actor 與 AI 決策邏輯（留待後續領域變更）。
- 不實作 Password UX 互動流程、整桌靜音模式、`/kick` 禁重入同桌、私人密碼桌與好友清單 `user_friends`（分別留待後續 `table/*`、`lobby/*`、`tui/*` 或未來增量）。
- 不在 `compose.yaml` 中加入反向代理／負載平衡服務（待 `adapter/*` 實作時再追加）。

## Decisions

### 1. 語意化 `Store` 門面 + 內部 `executor` 雙後端抽象
- **決策**：對外僅公開 `*Store` 與語意化方法，內部透過未匯出介面 `executor` 隔離後端差異：
  ```go
  type stmt struct {
      query string
      args  []any
  }

  type execResult struct {
      lastInsertID int64
      rowsAffected int64
  }

  type executor interface {
      Query(ctx context.Context, s stmt) ([][]any, error)
      ExecBatch(ctx context.Context, stmts []stmt) ([]execResult, error)
      Close() error
  }
  ```
- **建構器與 Functional Options**（遵循 `golang-design-patterns`）：
  - `OpenSQLite(ctx context.Context, dsn string, opts ...Option) (*Store, error)`
  - `OpenRqlite(ctx context.Context, endpoints []string, opts ...Option) (*Store, error)`
  - `type Option func(*storeConfig) error`：支援 `WithClock(func() time.Time)`、`WithArgon2Params(Argon2Params)`、`WithMaxBlocks(int)`、`WithRequestTimeout(time.Duration)`、`WithReadConsistency(ReadConsistency)`、`WithHTTPClient(*http.Client)`；所有無效參數於建構期即失敗（Fail Fast）。
- **理由**：
  - `rqlite` Raft 僅支援單一 HTTP 批次原子交易（`?transaction=true`），不支援跨網路來回的互動式 `*sql.Tx`。透過 `ExecBatch` 抽象，SQLite 以 `db.BeginTx` + `defer tx.Rollback()` + `tx.Commit()` 實作，`rqlite` 以 `POST /db/execute?transaction=true` 實作，兩端語意 100% 等價。
  - 以 Go stdlib `net/http` 自寫薄 `rqliteExecutor`（約 180 行），避免引入大型第三方 rqlite client 依賴，並可直接以 `net/http/httptest` 精準測試批次 JSON、錯誤回滾、多節點 failover 與 timeout。
- **替代方案**：導入第三方 `gorqlite`（實作 `database/sql/driver`）。因 `database/sql` 的 `Tx` 語意與 rqlite Raft 單次批次模型不合，且會增加外部依賴與維護面，故不採納。

### 2. SQLite 與 rqlite 共通 Schema 與 `//go:embed` 版本化 Migration
- **決策**：將版本化 SQL 檔置於 `internal/store/migrations/` 並以 `//go:embed migrations/*.sql` 內嵌；`Open*` 時自動建立 `schema_migrations`、依序於單一原子批次執行未套用版本，並冪等 seed `QKMJ0`–`QKMJ9` AI 帳號。
- **核心資料表設計**（全部使用標準 SQLite 語法與 UTC RFC3339Nano 時間字串，確保 SQLite 與 rqlite JSON 序列化完全一致）：
  - `schema_migrations(version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)`
  - `accounts`：`id INTEGER PRIMARY KEY AUTOINCREMENT`、`username TEXT NOT NULL`、`username_lower TEXT NOT NULL UNIQUE`、`password_hash TEXT NOT NULL`、`balance INTEGER NOT NULL DEFAULT 20000`、`games_played INTEGER NOT NULL DEFAULT 0`、`self_draw_wins INTEGER NOT NULL DEFAULT 0`、`discard_wins INTEGER NOT NULL DEFAULT 0`、`deal_ins INTEGER NOT NULL DEFAULT 0`、`draws INTEGER NOT NULL DEFAULT 0`、`abandoned_hands INTEGER NOT NULL DEFAULT 0`、`login_count INTEGER NOT NULL DEFAULT 0`、`last_login_at TEXT`、`last_node_name TEXT NOT NULL DEFAULT ''`、`banned_until TEXT`、`banned_reason TEXT NOT NULL DEFAULT ''`、`created_at TEXT NOT NULL`
  - `account_roles(account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE, role_name TEXT NOT NULL, PRIMARY KEY(account_id, role_name))`
  - `moderation_logs(id INTEGER PRIMARY KEY AUTOINCREMENT, actor_account_id INTEGER NOT NULL REFERENCES accounts(id), target_account_id INTEGER NOT NULL REFERENCES accounts(id), action TEXT NOT NULL, reason TEXT NOT NULL DEFAULT '', expires_at TEXT, created_at TEXT NOT NULL)`
  - `hand_settlements(hand_id TEXT PRIMARY KEY, node_name TEXT NOT NULL, end_reason TEXT NOT NULL, entries_json TEXT NOT NULL, settled_at TEXT NOT NULL)`
  - `user_preferences(account_id INTEGER PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE, prefs_json TEXT NOT NULL, updated_at TEXT NOT NULL)`
  - `user_blocks(account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE, blocked_account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE, created_at TEXT NOT NULL, PRIMARY KEY(account_id, blocked_account_id), CHECK(account_id != blocked_account_id))`
- **SQLite 連線參數**：強制啟用 `PRAGMA foreign_keys = ON`、`PRAGMA busy_timeout = 5000`；檔案模式啟用 `PRAGMA journal_mode = WAL`；`:memory:` 模式固定 `db.SetMaxOpenConns(1)` 避免連線池開出多份獨立記憶體庫。
- **理由**：`user_preferences` 以版本友善之 JSON 結構保存，未來新增 TUI 偏好欄位無須額外 ALTER TABLE，且未知欄位反序列化自然略過。

### 3. Argon2id 密碼雜湊、保留字頭與 `QKMJ0`–`QKMJ9` 內建 AI 帳號
- **決策**：
  - 帳號名稱以編譯期預編譯正規表示式 `^[0-9A-Za-z]{1,20}$` 驗證；`strings.HasPrefix(lower, "qkmj")` 視為系統保留字頭，拒絕真人註冊（`ErrReservedUsername`）與密碼登入（`ErrBotLoginForbidden`）。
  - 密碼以 `golang.org/x/crypto/argon2.IDKey` 搭配 16-byte `crypto/rand` 隨機鹽雜湊，預設採用 OWASP 推薦參數（`MemoryKiB: 19456`, `Iterations: 2`, `Parallelism: 1`, `KeyLen: 32`, `SaltLen: 16`），以標準 PHC 字串 `$argon2id$v=19$m=19456,t=2,p=1$<b64salt>$<b64hash>` 保存，驗證使用 `crypto/subtle.ConstantTimeCompare`。
  - AI 帳號 `QKMJ0`–`QKMJ9` 於 seed 階段寫入不可比對哨兵雜湊 `"!"` 並賦予 `bot` 角色；`Authenticate` 檢查帳號具 `bot` 角色或保留字頭時直接拒絕。
  - 對外公開的 `Account` DTO **絕不包含 `PasswordHash` 欄位**，避免意外序列化外洩。
- **理由**：相較 `bcrypt`，Argon2id 具 memory-hard 抗 GPU 特性且無 72 bytes 截斷缺陷；透過 `WithArgon2Params` 選項讓單元測試可使用較輕參數維持毫秒級執行速度，同時保留獨立測試驗證預設 OWASP 參數。

### 4. 輕量強型別 RBAC 與低介入治理
- **決策**：
  - 以 1 起算（0 為 `Unknown` 哨兵）的強型別列舉定義 `Role`（`RolePlayer`、`RoleModerator`、`RoleAdmin`、`RoleBot`）、`Permission`（`PermTablePlay`、`PermUserKick`、`PermUserBan`、`PermServerAnnounce`、`PermRoleAssign`）與 `ModerationAction`（`ModActionBan`、`ModActionUnban`、`ModActionAssignRole`、`ModActionRevokeRole`）。
  - 權限映射定義於 Go 程式碼唯讀表，`Account.HasPermission(p Permission) bool` 為純函式，無須多表 JOIN。
  - 所有管理寫入操作（`BanAccount`、`UnbanAccount`、`AssignRole`、`RevokeRole`）皆先驗證操作者權限，再於單一 `ExecBatch` 同步寫入異動與 `moderation_logs`。
  - 提供 `BootstrapAdmin(ctx, username)` 或啟動旗標（`--bootstrap-admin`）供維運初期賦予第一位 `admin` 角色，避免「沒有第一個 admin」的雞生蛋問題。

### 5. `SettleHand` 跨節點相對增量與零和原子性
- **決策**：
  - `SettleHand(ctx, HandSettlement)` 先驗證 `HandID != ""`、`NodeName != ""`、恰有 4 位不重複帳號、胡牌結算 4 家 `Delta` 總和為 0、和局 4 家 `Delta` 皆為 0。
  - 先查詢並解析 4 位帳號之 `id`（若任一帳號不存在立即回傳 `ErrAccountNotFound`；因不提供 `/suicide`，已存在帳號不會被刪除）。
  - 於單一 `ExecBatch` 執行：
    1. `INSERT INTO hand_settlements (hand_id, node_name, end_reason, entries_json, settled_at) VALUES (?, ?, ?, ?, ?)`（主鍵衝突轉譯為 `ErrDuplicateSettlement`）
    2. 4 筆 `UPDATE accounts SET balance = balance + ?, games_played = games_played + ?, self_draw_wins = self_draw_wins + ?, discard_wins = discard_wins + ?, deal_ins = deal_ins + ?, draws = draws + ? WHERE id = ?`
- **理由**：以相對增量更新確保多節點併發寫入 rqlite Raft Leader 時零 Lost Update，並以 `hand_id` 確保 HTTP 重試不重複入帳。

### 6. 本地建置 `Dockerfile`、最小 `cmd/qkmj` 與官方 `rqlite` Compose Stack
- **決策**：
  - `cmd/qkmj/main.go`：支援旗標與環境變數（`QKMJ_STORE_DRIVER`、`QKMJ_STORE_DSN`、`QKMJ_NODE_NAME`、`QKMJ_RQLITE_CONSISTENCY`、`QKMJ_BOOTSTRAP_ADMIN`），未設定 Node Name 時產生 Docker 風格隨機名稱；使用 `signal.NotifyContext` 監聽 `SIGINT`/`SIGTERM` 優雅關閉 Store。
  - `Dockerfile`：兩階段建置（`golang:1.24-alpine` 以 `CGO_ENABLED=0` 編譯靜態執行檔 → `gcr.io/distroless/static-debian12:nonroot` 執行）。
  - `compose.yaml`：
    - 預設服務：`rqlite-1`（官方 `rqlite/rqlite:8` 含 `/readyz` healthcheck 與 named volume `rqlite1-data`）+ `qkmj`（`build: .`，連線 `http://rqlite-1:4001`，等待 `rqlite-1` healthy）。
    - `ha` profile：擴充 `rqlite-2`、`rqlite-3`（3 節點 Raft 叢集）與 `qkmj-2`、`qkmj-3`（各自指向本地對應之 `rqlite-2:4001`、`rqlite-3:4001`，演示水平擴展部署拓撲）。

## Risks / Trade-offs

- **[Risk] SQLite `:memory:` 在 Go `database/sql` 連線池開啟多條連線時會各自建立獨立空庫** → **Mitigation**：偵測 `:memory:` 或 `mode=memory` 時強制 `db.SetMaxOpenConns(1)`；檔案模式則開啟 `WAL` + `busy_timeout=5000`。
- **[Risk] `rqlite` 節點短暫切換 Leader 時 HTTP 寫入可能回傳 301 Redirect 或暫時錯誤** → **Mitigation**：`rqliteExecutor` 設定 `http.Client` 跟隨重導向（保留 POST body）並依 endpoints 清單進行尊重 `ctx.Err()` 的健康節點容錯切換；上層重試同一 `HandID` 亦受 `hand_settlements` 冪等保護。
- **[Risk] `rqlite` `INSERT` 主鍵衝突或外鍵錯誤的錯誤字串格式與 `modernc.org/sqlite` 略有差異** → **Mitigation**：統一將 SQLite 與 rqlite 的 `UNIQUE constraint failed` 錯誤訊息標準化映射為 `ErrUsernameTaken` / `ErrDuplicateSettlement`，並在雙後端合約測試中共同驗證。
- **[Risk] 預設 Argon2id 參數（19 MiB）在數百個並發單元測試重複執行可能拖慢 CI** → **Mitigation**：正式預設維持 OWASP 推薦參數（`19 MiB, t=2, p=1`），測試專用 `WithArgon2Params` 注入輕量參數，並由獨立安全測試明確斷言預設參數達 OWASP 下限。

## Migration Plan

1. 本次為首版資料庫 schema（`v001_initial.sql`），無既有線上資料遷移風險。
2. 啟動 `OpenSQLite` 或 `OpenRqlite` 時自動冪等執行 Migration 與 `QKMJ0`–`QKMJ9` Seed。
3. 同步更新 `AGENTS.md` 安全基線段落以反映 Argon2id 升級，並維持 `go.mod` 零本地 `replace`。
