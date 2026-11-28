# Design

## Context

`internal/rules` 已完成牌型/牌牆/拆牌/計分（Slice 1）與宣告視窗/回合引擎 `TurnEngine`（Slice 2）。Slice 3 在不破壞既有 `TurnEngine` 邊界與 `internal/rules` 純 stdlib 不變量（零 goroutine、零 channel、零 I/O、零 `init()`、零系統時鐘、注入式 `rand.Source`）的前提下，向上組裝單局生命週期狀態機 `HandEngine` 與多局外層進程狀態機 `MatchEngine`（動機見 `proposal.md`）。

## Goals / Non-Goals

**Goals:**
- 提供可確定性重播、零副作用的 `HandEngine` 與 `MatchEngine`，完整覆蓋 QKMJ 基線 `hand-lifecycle` 與 `match-progression`。
- 遵循 `golang-design-patterns`：以具驗證回傳之 Functional Options 設計建構子、零值為 Unknown 哨兵的 Enum、Fail-fast 驗證（錯誤時零狀態變更）、清晰組合（`MatchEngine` 組合 `HandEngine`，`HandEngine` 組合 `Wall` 與 `TurnEngine`）。
- 嚴格維護公平性與隱藏資訊隔離：開局發牌與補花公私分離；暗手與暗槓牌面僅在胡牌或流局後的 `HandsRevealedEvent` 才揭露，中途離桌中止不揭露。
- 透過 `HandSettledEvent` 提供語意完整的結算輸出（含每家 `Before`/`Delta`/`After` 與 `IncrementPlayedCount`），讓未來 `internal/table` 無須解析內部狀態即可原子呼叫 `internal/store.SettleHand`。

**Non-Goals:**
- 不實作帳號存取、登入紀錄、`/stat` 排名與入桌門檻檢查（屬 `store/account-economy` 與 `lobby/*`）。
- 不實作 95p3+ 進階規則（四風連和、四槓和、九么九、一炮三響、立直/天聽、早裁、四圈終點），保留於下一個獨立 OpenSpec change，透過新 Functional Option 擴充。
- 不實作 Goroutine Actor、逾時自動出牌或網路傳輸（屬 `internal/table` 與 `internal/adapter/*`）。

## Decisions

### 1. 雙層組合狀態機：`MatchEngine` → `HandEngine` → `TurnEngine`
- **決策**：新增 `HandEngine`（[internal/rules/hand.go](file:///home/antigravity/gjchentw/go-qkmj/internal/rules/hand.go)）封裝單局開局準備、回合委派、亮牌、結算、全員確認與中止；新增 `MatchEngine`（[internal/rules/match.go](file:///home/antigravity/gjchentw/go-qkmj/internal/rules/match.go)）封裝多局進程、餘額演進、莊家/連莊/圈風推進與離桌重設。
- **理由**：`TurnEngine` 已專注於「莊家第一手到胡牌/流局」的核心摸打與宣告；以組合而非把開局/結算/換圈硬塞進 `TurnEngine`，能維持每個型別職責單一、循環複雜度低且便於獨立單元測試。
- **替代方案**：將所有狀態合併進單一巨型 `Engine` 結構。因狀態數過多且難以獨立測試單局開局補花或跨圈推進，故不採納。

### 2. 遵循 `golang-design-patterns` 的 Functional Options 與建構期驗證
- **決策**：
  - `NewHandEngine(cfg HandConfig, opts ...HandOption) (*HandEngine, error)`：`HandConfig` 攜帶必填的單局固定輸入（`RoundWind`、`Dealer`、`DealerStreak`、`Balances`、`Base`、`PerFan`、`Rand`）；`HandOption`（型別 `func(*handOptions) error`）提供可選覆寫，如 `WithWall(w *Wall)`、`WithDoorSeat(s Seat)`。
  - `NewMatchEngine(rng rand.Source, opts ...MatchOption) (*MatchEngine, error)`：`MatchOption`（型別 `func(*matchOptions) error`）提供 `WithStakes(base, perFan int)`（預設 `DefaultBase=500`, `DefaultPerFan=200`）、`WithWallFactory(fn func(rand.Source) (*Wall, error))`、`WithDoorSeatPicker(fn func(rand.Source) Seat)`。
  - 每個 Option 在建構期即驗證參數合法性並回傳錯誤（例如負數底台、nil factory、無效門風座位），落實 Fail-fast 與未來擴充零破壞性（例如未來 95p3 新增 `WithMaxRounds(4)`）。
- **替代方案**：僅用純 struct config（呼叫端每次都得填滿預設底台與工廠函式）或 Builder pattern（對本套件過度設計）。Functional Options 兼顧簡潔預設與未來演進。

### 3. 零值哨兵 Enum 與不可表示的非法狀態
- **決策**：
  - `HandPhase`：`HandPhaseUnknown = iota`（0）、`HandPhaseNotStarted`、`HandPhasePlaying`、`HandPhaseWaitReady`、`HandPhaseFinished`、`HandPhaseAborted`。
  - `MatchPhase`：`MatchPhaseUnknown = iota`（0）、`MatchPhaseGathering`、`MatchPhaseInHand`。
  - 所有操作入口先驗證 Phase、Seat 與 Action，任何錯誤皆立即返回且不異動內部欄位。

### 4. 開局補花與 `TurnEngine` 銜接契約
- **決策**：`HandEngine.Start()` 依序執行：
  1. 產生或使用注入的 144 張 `Wall`；以注入式 `rand.Source`（`rand.New(rng).IntN(4)`，或 `WithDoorSeat` 覆寫）決定 `doorSeat`，計算四家 `seatWinds`，發出公開 `HandStartedEvent`。
  2. 依序發牌（每家 16 張、莊家 17 張），發出公開 `HandDealtPublicEvent` 與四份私有 `HandDealtEvent`（每份皆 `slices.Clone` 防禦拷貝）。
  3. 自莊家起順時針巡迴四家，對每家執行迴圈：凡暗手有花牌即移入公開花區、呼叫 `wall.Draw()` 補牌，發出公開 `FlowerExposedEvent` 與私有 `TileDrawnEvent{IsSupplement: true}`；若補到仍是花牌立即重複直到該家暗手無花才換下家。期間不開啟花牌宣告視窗。
  4. 將完成補花後的 `wall`、四家暗手與花牌傳入 `NewTurnEngine(...)` 並呼叫 `turn.Start()`：若開局即有 8 張花或 7+1 花，`TurnEngine.Start()` 會直接開啟花牌宣告視窗，否則進入莊家第一手自身宣告或 `WaitDiscard`。
- **理由**：直接複用 Slice 2 已驗證的 `TurnEngine.Start()` 與開局花牌視窗邏輯，完全消除重複程式碼。

### 5. 結算時機、亮牌與離桌中止的一致性語意
- **決策**：
  - 一旦 `turn.Apply(...)` 使 `turn.Phase() == PhaseHandEnded`（`HandEndWin` 或 `HandEndExhausted`），`HandEngine` 在同一次 `Apply` 回傳中緊接產出：
    1. `HandsRevealedEvent`（公開四家暗手與 `ProjectMeld(m, true)` 揭露暗槓牌面）
    2. `HandSettledEvent`（公開 `Reason`、`Score`、四家 `SeatBalanceChange{Before, Delta, After}`、`IncrementPlayedCount`）
    3. `WaitReadyEvent`（公開進入全員確認）
    並切至 `HandPhaseWaitReady`。
  - `MatchEngine` 於收到該次結算後**立即**更新 `MatchEngine.balances`（不等到四人確認完畢才更新）。如此一來：
    - `Table Actor` 收到 `HandSettledEvent` 即可同步呼叫 `store.SettleHand` 原子寫入帳戶（早於下一局開始）。
    - 若有人於 `HandPhasePlaying` 離桌，`Abort` 不亮牌、不結算、不改餘額。
    - 若有人於 `HandPhaseWaitReady` 離桌，已結算之餘額變動**保留不回溯**，僅中止牌局並回到 `MatchPhaseGathering`。
- **理由**：完整對齊 `rules/hand-lifecycle` 的亮牌/結算/全員確認順序與 `rules/match-progression` 的「結算記入帳戶不回溯」與「有人離開中止回等人」。

### 6. 事件模型擴充（維持 Sealed `Event` 介面）
- **決策**：於 [internal/rules/events.go](file:///home/antigravity/gjchentw/go-qkmj/internal/rules/events.go) 新增 10 種強型別事件（皆實作既有 sealed `Event` 介面與 `Audience()` / `TargetSeat()`）：
  - **Public**：`MatchStartedEvent`、`HandStartedEvent`、`HandDealtPublicEvent`、`HandsRevealedEvent`、`HandSettledEvent`、`WaitReadyEvent`、`PlayerReadyEvent`、`MatchProgressedEvent`、`MatchAbortedEvent`。
  - **Private**：`HandDealtEvent`（僅傳給目標座位本人）。
- **理由**：延續 Slice 2 的型別安全事件設計，下游 `internal/table` 僅依 `Audience()` 與 `TargetSeat()` 派發即可 100% 維持隱藏資訊隔離。

## Risks / Trade-offs

- **[Risk] 測試撰寫跨多局進程（如連莊、過莊、北風回東風）若需走完整 144 張牌摸打將過於冗長脆弱** → **Mitigation**：透過 `WithWallFactory` 與 `WithDoorSeatPicker` Functional Options 注入精準構造之牌牆（例如起手天胡、開局八仙過海、最後一張流局），讓每局可在 1–3 個動作內確定性結束並完整驗證真實流程，無須繞過 `TurnEngine`。
- **[Risk] `HandStartedEvent` / `HandsRevealedEvent` 若直接暴露內部 slice 參考，外層可能誤改引擎狀態** → **Mitigation**：所有事件中的 slice 與 map（`Tiles`、`Melds`、`SeatWinds`、`Balances`）皆於建構事件時進行防禦性拷貝（`slices.Clone` / 新建 map）。
- **[Risk] 開局補花在極端非法自訂牌牆中理論上可能遇到無牌可補** → **Mitigation**：合法 144 張牌牆在發完 65 張後必餘 63 張可摸牌且全副僅 8 張花，開局補花不可能耗盡；但程式碼仍明確檢查 `wall.Draw()` 回傳錯誤以防呆（Fail-fast，絕不忽略錯誤或 panic）。
