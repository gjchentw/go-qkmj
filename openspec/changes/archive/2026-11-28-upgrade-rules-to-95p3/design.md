# Design

## Context

`internal/rules` 已完成 100% QKMJ 0.94/基線規則引擎（Slice 1–3），且維持純 Go stdlib（零 goroutine、零 channel、零 I/O、零 `init()`、零 `time.Now()`、注入式 `rand.Source`）與 `MatchEngine` → `HandEngine` → `TurnEngine` → `ClaimWindow` 清晰分層。本次變更將全部 6 份 `rules/*` 規格全面演進至 QKMJ 95p3+（動機見 `proposal.md`），並以 `golang-design-patterns` 確保新增規則不會把既有 FSM 變成難以維護的條件義大利麵。

## Goals / Non-Goals

**Goals:**
- 將四種特殊和局（四風連和、四槓和、一炮三響、九么九和）、立直與天聽（公平版）、宣告早裁（0.95o）、四圈統計（0.95p）與三色同刻非門清減台完整落實為純函式與確定性 FSM。
- 透過顯式型別與小職責輔助物件（`HandEndReason.IsDraw()`、`FindRiichiDiscardTiles`、`roundCycleTracker`、`ClaimWindow.canEarlyResolve`）讓非法狀態不可表示並維持各函式循環複雜度低於 linter 上限（`< 15`）。
- 嚴格守護公平性不變量：立直絕不揭露他家暗手；所有新事件沿用 sealed `Event` 介面與 `Audience()`/`TargetSeat()` 強制分離公私資訊。

**Non-Goals:**
- 不實作 VIP 等級、經驗值、登入救濟、負數踢出（屬 `store/account-economy` 與 `lobby/*`）。
- 不實作立直自動出牌之延遲計時器（由未來 `internal/table` Actor 依事件節奏送出 `ActionDraw` + `ActionDiscard(drawnTile)`，規則引擎僅強制驗證 `ErrRiichiLocked`）。
- 不保留舊 0.94 與 95p3+ 並存之雙軌旗標（本次為規格全面演進，避免無謂分支複雜度）。

## Decisions

### 1. 強型別和局列舉與 `HandEndReason.IsDraw()` 單一分類點
- **決策**：在 [internal/rules/events.go](file:///home/antigravity/gjchentw/go-qkmj/internal/rules/events.go) 擴充 `HandEndReason`（零值 `HandEndUnknown` 哨兵維持不變）：
  - `HandEndWin`、`HandEndExhausted`、`HandEndAborted`
  - 新增 `HandEndFourWinds`、`HandEndFourKongs`、`HandEndTripleRon`、`HandEndNineTerminals`
  - 新增方法 `(r HandEndReason) IsDraw() bool` 回傳是否為上述五種和局之一。
- **理由**：符合 `golang-design-patterns`「以明確型別邊界消除重複分支」原則；`HandEngine.finalizeHandEnd`、`MatchEngine.advanceMatchProgress` 與統計累計只需判斷 `reason.IsDraw()`，不需四處重複列舉 5 種常數，同時保留精確和局類型供下游 TUI 顯示中文名稱。

### 2. 宣告早裁（0.95o）與一炮三響的安全共存演算法
- **決策**：在 [internal/rules/claim.go](file:///home/antigravity/gjchentw/go-qkmj/internal/rules/claim.go) 中：
  - `ClaimResolution` 新增 `IsTripleRon bool`（當 `windowKind` 為 `ClaimWindowDiscard` 或 `ClaimWindowRobKong` 且 `winCount == 3` 時設為 `true`）。
  - `Submit` 後呼叫純函式 `canEarlyResolve() bool`：
    1. 計算已提交之最高優先宣告 `bestKind`、已提交胡牌數 `winCount`、已提交中順時針最早胡牌者距離 `bestWinDist`，以及尚未決定者中含各選項之情況（`undecidedCanWin`、`undecidedCanKong`、`undecidedCanPong`、`earlierUndecidedCanWin`）。
    2. 若 `bestKind == ClaimWin`：僅當 `!earlierUndecidedCanWin` **且** `!(canTripleRonWindow && winCount+undecidedCanWin >= 3)` 時立即早裁。
    3. 若 `bestKind == ClaimKong`：僅當 `undecidedCanWin == 0` 時立即早裁（同牌最多僅一家能槓）。
    4. 若 `bestKind == ClaimPong`：僅當 `undecidedCanWin == 0 && undecidedCanKong == 0` 時立即早裁（同牌最多僅一家能碰）。
    5. 若達成早裁，立即清除任何進行中的 `chowSeat`（取消 `WaitChowChoice`）並標記 `resolved = true`。
- **理由**：數學上保證任何早裁結果皆與「等待全員表態」100% 等價，不會漏判截胡，也不會漏判一炮三響；且對家碰成立時可立刻取消下家的 `WaitChowChoice`，消除無意義等待。
- **替代方案**：僅在 `ClaimWin` 時早裁。因 0.95o 明確包含碰/槓蓋過吃時不等低優先者，故完整實作全部優先階層。

### 3. 原子式立直宣告（`SelfDeclareRiichi`）與摸切鎖定（`ErrRiichiLocked`）
- **決策**：
  - 擴充 `SelfDeclareKind` 新增 `SelfDeclareRiichi`（必填合法非花牌 `Tile`）與 `SelfDeclareNineTerminals`。
  - 新增純函式 `FindRiichiDiscardTiles(concealed []Tile, melds []Meld) []Tile`（於 `decompose.go` 或 `turn.go`）：對暗手每種不重複非花牌試刪 1 張後，測試 34 種非花牌是否存在至少一張使 `CanWin` 成立，回傳去重排序之合法聽牌棄牌清單。
  - 在 `TurnEngine.enterSelfCheck`：
    - 若未立直、門清（`isConcealed(seat)`）、`wall.DrawableRemaining() > 0` 且 `len(riichiDiscards) > 0`，於 `SelfDeclarePromptEvent` 填入 `CanRiichi: true` 與 `RiichiDiscardTiles`。
    - 若玩家提交 `SelfDeclareRiichi` 且 `Tile` 屬於 `RiichiDiscardTiles`，原子執行：標記 `riichi[seat] = true`、依 `!anyMeldDeclared && !hasDiscarded[seat]` 標記 `tianTing[seat]`、發出公開 `RiichiDeclaredEvent{Seat, IsTianTing}`，並直接呼叫 `applyDiscard` 打出 `action.Tile`（累加 `thinkElapsed` 並進入出牌宣告視窗）。
  - 立直後的限制（全由 `TurnEngine` 強制把關）：
    - 不可宣告吃、碰、明槓（`computeDiscardSeatOptions` 僅計算胡）、不可宣告暗槓與加槓（`enterSelfCheck` 跳過槓掃描）。
    - 出牌鎖定摸切：`applyDiscard` 檢查 `if e.riichi[seat] && action.Tile != e.lastDrawnTile` 回傳 sentinel error `ErrRiichiLocked`。
    - 永久過水：`enterSelfCheck` 在 `fromDraw && e.passedWin[seat] && !canWinNow` 條件加上 `&& !e.riichi[seat]`，確保立直後過水永不解除。
- **理由**：單一原子動作直接消除「宣告立直後卻亂打沒聽牌」的非法狀態，也不需新增 `PhaseWaitRiichiDiscard` 狀態；把自動摸切節奏留給上層 Actor，同時在規則層以 `ErrRiichiLocked` 死守不可換牌的不變量。

### 4. 四風連和、四槓和與九么九和的狀態追蹤
- **決策**：在 `TurnEngine` 內以輕量值欄位精準追蹤（零額外配置）：
  - **四風連和**：在 `advanceAfterNoClaim`（剛出牌無人宣告且未達海底）檢查 `len(e.discards) == 4 && !e.anyMeldDeclared` 且前 4 張出牌皆為同一張風牌（`t.IsWind()` 且四張相等）→ `finishHandDraw(HandEndFourWinds)`。
  - **四槓和**：以 `kongCount int` 與 `kongSeats [5]bool` 追蹤已成立的明槓、暗槓與（未被搶槓的）加槓；每當 `distinctKongSeats() >= 2 && kongCount >= 4` 時設 `pendingFourKongsDraw = true`。該玩家補牌後不提供第 5 槓（僅可自摸或出牌）；若補牌未自摸且打出之牌無人胡牌（在 `advanceAfterNoClaim`），觸發 `finishHandDraw(HandEndFourKongs)`！
  - **九么九和**：於 `enterSelfCheck` 檢查 `!e.anyMeldDeclared && !e.hasDiscarded[seat] && countDistinctTerminalOrHonor(e.concealed[seat]) >= 9`，設 `CanNineTerminals = true`；宣告 `SelfDeclareNineTerminals` 時立即呼叫 `finishHandDraw(HandEndNineTerminals)`。
- **理由**：所有判定皆為純狀態欄位與 O(17) 掃描，且正確遵守「胡牌優先於和局」（第 4 風被胡、第 4 槓補牌自摸或棄牌被胡、加槓被搶槓皆由胡牌優先）。

### 5. 四圈統計追蹤器（`roundCycleTracker`）與純耗時累加
- **決策**：
  - 於 [internal/rules/match.go](file:///home/antigravity/gjchentw/go-qkmj/internal/rules/match.go) 定義封裝值型別 `roundCycleTracker`，記錄 `cycleNumber`（從 1 起算）、`elapsed time.Duration`（由 `MatchEngine.Apply` 傳入之 `thinkElapsed` 累加，絕不讀系統時鐘）以及 `[5]SeatCycleStats`（`NetDelta`、`DealIns`、`DiscardWins`、`SelfDrawWins`、`HandsPlayed`、`Draws`）。
  - 每當一局完成結算（`!wasSettled && m.hand.Settled()`）：更新本將統計（胡牌：`NetDelta += delta`、`HandsPlayed++`、自摸/八仙過海計入 `SelfDrawWins`、放槍/搶槓/七搶一計入 `DiscardWins` 並對 `Payer` 計入 `DealIns`；任一和局：`Draws++`）。
  - 當 `advanceMatchProgress` 讓 `m.roundWind == WindNorth && nextWind == WindEast`（完成北風圈進入下一將東風圈）時，發出公開 `RoundCycleCompletedEvent{CycleNumber, Elapsed, Stats}`（含防禦性 map 拷貝）並呼叫 `tracker.resetNextCycle()`，再發出 `MatchProgressedEvent` 開始下一將。
  - 若中途 `PlayerLeft` 回到 `Gathering`，下次 `StartMatch` 重置 `tracker` 從第 1 將起算。

### 6. 台型規則更新（`rules/scoring`）
- **決策**：
  - `ScoreContext` 新增 `IsRiichi bool` 與 `IsTianTing bool`（由 `TurnEngine.finishHandByWin` 帶入，並在 `validateScoreContext` 驗證 `IsTianTing` 不可與非門清並存？注意：暗槓不破門清，且 `IsTianTing` 僅於門清立直成立，故驗證 `!isConcealed && (IsRiichi || IsTianTing)` 若為真則不給台或於 `TurnEngine` 已保證門清立直後無法破門清）。
  - 新增 `FanRiichi`（「立直」，1 台）與 `FanTianTing`（「天聽」，8 台）；`appendSituationalFans` 中若 `ctx.IsTianTing` 則計 `FanTianTing` 8 台（取代 `FanRiichi`），否則若 `ctx.IsRiichi` 則計 `FanRiichi` 1 台。
  - 修改 `appendTripletAndKongFans` 中 `FanThreeSuitedTriplets`：門清計 2 台、非門清計 1 台（與 `FanThreeSuitedSequences` / `FanPureStraight` 相同減台語意）。

## Risks / Trade-offs

- **[Risk] 立直棄牌掃描 `FindRiichiDiscardTiles` 在每次摸牌都執行，會不會影響效能？** → **Mitigation**：僅在「未立直且門清且可摸張數 > 0」時掃描；先對暗手 17 張去重（最多 17 種牌），對每種棄牌候選只測試「暗手中相關牌種與其相鄰 ±1 數牌」（或最多 34 種非花牌，單次 `CanWin` 約微秒級），實測單局耗時 < 1ms。
- **[Risk] `A19_三色同刻` 黃金測試案例從 2 台改為 1 台（破壞性台數演進）** → **Mitigation**：已在 `proposal.md` 標記 **BREAKING**，同步更新 `rules/scoring` 規格與 `scoring_samples_test.go`（A19 更新為非門清 1 台，並新增門清三色同刻 2 台驗證）。
- **[Risk] 四槓和若於最後一張可摸牌達成，或第 4 槓補牌摸到花牌耗盡牌牆** → **Mitigation**：若補花導致無牌可補，原有海底流局邏輯立即觸發（依然是和局）；若順利補到非花牌，打出安全牌後於 `advanceAfterNoClaim` 優先判定 `HandEndFourKongs`（於 `DrawableRemaining() == 0` 檢查之前）。
