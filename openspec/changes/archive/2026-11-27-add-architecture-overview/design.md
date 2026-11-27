# Design

## Context

`go-qkmj` 是經典台灣 16 張麻將終端遊戲 QKMJ 的現代 Go 重製版，採用 OpenSpec-Led Go TDD 開發流程，並以原生 UTF-8 Server-Side TUI 同時服務 SSH 與 Web 玩家。變更動機與範圍見 [proposal.md](proposal.md)；完整系統不變量與能力邊界見 [specs/architecture/spec.md](specs/architecture/spec.md)。

上游依賴均已完成驗證並釋出正式版本：
- `github.com/ginmills/ginmill v1.2.0`：Gin 路由組合框架。
- `github.com/gjchentw/go-kittyd v0.2.0`：內嵌 `@xterm/xterm` v6 與 `Sarasa Fixed TC` WebFont（`unicode-range` 切分）、預設啟用同源 CSWSH 防護的 Web Terminal 轉接層。

## Goals / Non-Goals

**Goals:**
- 建立所有後續領域提案（`rules/*`、`store/*`、`table/*`、`bot/*`、`lobby/*`、`tui/*`、`adapter/*`）皆須遵守的單向分層與套件邊界。
- 在實作前解決關鍵系統設計取捨，特別是 Web 端 In-Process 終端橋接的背壓與生命週期安全、AI 與真人完全對稱的公平性隔離，以及跨 SQLite / rqlite 的儲存抽象。
- 讓未來架構不變量可透過 Go 單元測試、`-race`、`goleak` 與 import 邊界測試自動化驗證。

**Non-Goals:**
- 本變更不撰寫任何 Go 實作程式碼，也不修改 `go.mod`。
- 不在此設計個別麻將規則（如台型公式、過水細節）或 80×24 畫布逐格座標，這些細節分別歸屬於對應的 `<domain>/<capability>` 提案。
- 不設計跨節點分散式即時牌桌同步（每節點維持獨立的 Lobby 與 Table Actors，僅共享持久化帳號／經濟／戰績儲存層）。

## Decisions

### Decision 1: 採雙層 `<domain>/<capability>` 規格邊界 + 頂層 `architecture` 總綱
- **選擇**：頂層建立 `openspec/specs/architecture/spec.md` 描述跨層總綱與套件地圖；各領域能力依 `<domain>/<capability>` 切分（例如 `rules/game-model`、`table/table-actor`、`tui/table-canvas`）。
- **理由**：OpenSpec CLI 原生支援雙層 capability path，既能對齊 `../qkmj` 的細粒度參考結構（避免單一 `rules` 或 `tui` 規格膨脹至上千行），又能確保每個 OpenSpec 變更聚焦在單一可驗證的垂直切片。
- **替代方案**：7 個單層大規格（`rules`、`store`、`table`、`bot`、`lobby`、`tui`、`adapter`）。因麻將規則與 TUI 狀態機各自包含多組獨立生命週期，單層規格會讓後續提案 delta 難以閱讀與審查，故不採用。

### Decision 2: Consumer-Side Port 介面與單一 Composition Root（`cmd/qkmj`）
- **選擇**：遵循 Go 慣用法，由呼叫端套件定義所需最小介面——`internal/lobby` 定義 `AccountStore`、`internal/table` 定義 `HandSettler` 與 `BotFactory`、`internal/table` 定義 `Player`；由 `cmd/qkmj` 統一建立具體實作並透過 Functional Options / Constructor 注入。
- **理由**：
  1. 徹底消除循環依賴：`internal/table` 無須匯入 `internal/store` 或 `internal/bot`。
  2. 編譯期公平性防線：`internal/bot` 僅能依賴 `internal/table` 匯出的 `Player` 契約與強型別事件／指令，結構上無法存取 `table` 未匯出的牌牆或他家手牌欄位。
  3. 測試零負擔：各層可用輕量 fake/stub 獨立測試，無須啟動真實 DB 或網路伺服器。
- **替代方案**：引入集中式 `internal/ports` 大介面套件或使用 DI framework。容易形成肥介面與隱式依賴，不符合 Go 簡潔原則，故不採用。

### Decision 3: Web 入口採「Bounded In-Process `kittyd.Terminal` Bridge」而非 `os/exec` 子行程
- **選擇**：`internal/adapter/web` 透過 `go-kittyd v0.2.0` 的 `kittyd.SessionFactory` 回傳自訂的 in-process `kittyd.Terminal` 實作，於同進程內直接驅動 `bubbletea/v2` `tea.Program`（與 SSH `wish/v2` 的 in-process 模式完全對稱）。
- **理由**：零 `os/exec` 攻擊面、零額外進程與 OS PTY fd 消耗，並讓 SSH 與 Web 使用者在同一個 `Lobby Actor` 與 `Table Actor` 記憶體空間中直接對戰，無須額外 IPC 協定。
- **替代方案**：每個 Web 連線 `os/exec` 啟動一個 `qkmj` CLI 子行程透過本地 TCP/SSH 連回主伺服器。記憶體成本高、引入額外本地認證與 `os/exec` 風險，故不採用。

### Decision 4: `internal/store` 採語意化原子介面（禁止外洩 `*sql.Tx`）
- **選擇**：`internal/store` 提供領域語意方法（如 `CreateAccount`、`Authenticate`、`SettleHand`、`UpdatePreferences`、`ListRankings`），由各自後端（`modernc.org/sqlite` 使用 SQLite transaction；`rqlite` 使用 atomic batch HTTP API）在內部實作原子性。
- **理由**：`rqlite` 透過 HTTP Raft 批次請求保證原子性，並不具備傳統跨多次 round-trip 的長連線 `*sql.Tx` 語意；隱藏交易細節可讓本地開發／測試（純 Go SQLite）與正式 HA（`rqlite`）共享完全一致的業務層程式碼。
- **替代方案**：在業務層使用標準 `database/sql` 的 `*sql.Tx` 編排多條 SQL。將無法平順對接 `rqlite` 原子批次模型，故不採用。

### Decision 5: Single-Owner Actor 搭配型別安全事件與視角投影
- **選擇**：每個 `Lobby`（每節點 1 個）與每張 `Table`（每桌 1 個）由單一 goroutine 獨佔其可變狀態；所有指令以強型別 struct 透過 channel 進入，所有對外事件經 `Perspective Projector` 依收件者座位過濾後派發。
- **理由**：避免細粒度多層 mutex 鎖順序死鎖，且將「隱藏資訊過濾」集中在單一必經出口，便於用單元測試對每種事件窮舉驗證不外洩暗牌。
- **替代方案**：以 `sync.RWMutex` 保護共享 `Table` 結構並由呼叫端直接操作方法。在宣告視窗倒數、斷線接管與 AI 回應併發情境下極易出現鎖競爭與資訊過濾遺漏，故不採用。

## Risks / Trade-offs

- **[Risk] 裸用 `io.Pipe()` 的零緩衝背壓導致慢速 Web 連線卡死 `bubbletea/v2` 甚至反壓 `Table Actor`**
  → **Mitigation**：`webTerminal` 輸出端 MUST 加入有界環狀緩衝或 buffered channel（例如 64 KB 上限），當慢速連線持續無法消化輸出時主動關閉該 session；同時 `Table Actor` → `Player` 的事件通道亦維持有界與慢消費者主動斷線策略，徹底隔離爆炸半徑。
- **[Risk] `io.Pipe()` 非真實 TTY，導致 `bubbletea/v2` 顏色降級為無色 ASCII 且無法 `TIOCGWINSZ`**
  → **Mitigation**：建立 `tea.NewProgram` 時顯式注入 `tea.WithColorProfile(colorprofile.TrueColor)`、`tea.WithEnvironment([]string{"TERM=xterm-256color", "COLORTERM=truecolor"})` 與預設 `tea.WithWindowSize(80, 24)`。
- **[Risk] 雙向關閉競態（WebSocket 先斷 vs. TUI 先 `/quit`）導致 `bubbletea` 或 `kittyd` pump goroutine 洩漏**
  → **Mitigation**：以 `context.WithCancel` 與 `sync.Once` 統一協調 `Close()`、關閉 `inW`/`outW`/`outR` 與取消程式上下文，並於 `adapter/web` 實作階段以 `go test -race` 與 `goleak` 覆蓋所有斷線時序。
- **[Risk] `xterm.js` 連線後首筆 `resize` 訊息早於 `program.Run()` 啟動，造成首屏尺寸遺失或阻塞**
  → **Mitigation**：`webTerminal` 以鎖保護保存最新 `(rows, cols)`，`Resize()` 在 `ctx.Done()` 保護下非阻塞派發 `tea.WindowSizeMsg`，且 `tui` 初始化時主動同步最新視窗尺寸。
- **[Risk] In-Process TUI 會話發生未預期 panic 拖垮整個伺服器進程**
  → **Mitigation**：保留 `bubbletea/v2` 內建 panic 捕捉，並在每個 SSH/Web session launcher goroutine 外層加上 `defer recover()`、結構化日誌記錄與安全離桌清理。
- **[Risk] 雙層規格數量較多，日後可能與實際程式碼依賴漂移**
  → **Mitigation**：於後續實作階段加入自動化套件依賴邊界測試（檢查 `go list -json ./internal/...` 或 `depguard` lint 規則），將 `architecture` 總綱的單向依賴規則編碼為 CI 檢查。

## Migration Plan

本變更僅建立 `go-qkmj` 的首份架構總綱規格，無既有執行環境或資料遷移需求：
1. 經審閱確認後同步至 `openspec/specs/architecture/spec.md` 並封存 `add-architecture-overview`。
2. 後續依底層至上層順序逐一提案與 TDD 實作：`rules/*` → `store/*` → `table/*` → `bot/*` → `lobby/*` → `tui/*` → `adapter/*`。
