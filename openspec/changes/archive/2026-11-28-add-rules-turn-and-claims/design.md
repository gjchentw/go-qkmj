# Design

## Context

本變更實作 `internal/rules` 第二個垂直切片：`rules/claim-arbitration` 與 `rules/turn-cycle`，建構於切片 1 已落地的 `Tile`、`Seat`、`Wall`、`Meld`、`PlayerAction`、`CanWin` 與 `Score` 之上。動機見 [proposal.md](proposal.md)，規範需求見 [specs/rules/claim-arbitration/spec.md](specs/rules/claim-arbitration/spec.md) 與 [specs/rules/turn-cycle/spec.md](specs/rules/turn-cycle/spec.md)。

架構約束（見 [architecture](../../../specs/architecture/spec.md)）：
- `internal/rules` 維持純 Go 標準庫、零 goroutine、零 channel、零 I/O、零 `time.Now()` 與零全域狀態。
- 所有對外事件 MUST 區分公開事件與私有事件（附帶 `TargetSeat`），在資料結構層面即隔離隱藏資訊，供上層 `internal/table` 的 Perspective Projector 直接安全路由。

## Goals / Non-Goals

**Goals:**
- 建立獨立、可單元測試的 `ClaimWindow`（宣告視窗仲裁器）與 `TurnEngine`（單局回合狀態機）。
- 完整覆蓋 QKMJ 基線狀態轉移：莊家第一手、手動摸牌、槓後手動補牌、自動連續補花、全手暗槓／加槓掃描、搶槓視窗、八仙過海／七搶一花牌視窗、0.95m 嚴格過水、截胡、最後一張僅能胡與流局保護。
- 每次狀態轉換皆以純函式風格回傳強型別 `[]Event`（明確標註 `Audience` 為全桌公開或單一座位私有），方便切片 3 與 `internal/table` 直接消費。

**Non-Goals:**
- 不實作洗牌發牌、開局補花、亮牌與等待全員 `WaitReady`（歸屬切片 3：`rules/hand-lifecycle`）。
- 不實作多局莊家連莊、圈風前進與離桌作廢（歸屬切片 3：`rules/match-progression`）。
- 不實作 95p3+ 擴充規則（0.95o 早裁、立直、四風連和、九么九和等，留待基線 3 切片完成後以獨立變更補上）。

## Decisions

### Decision 1: 統一強型別 `Event` 封裝公開與私有輸出（`internal/rules/events.go`）
- **選擇**：定義 `EventAudience`（`AudiencePublic` / `AudiencePrivate`）與強型別事件介面 `Event`（實作具體事件如 `TurnChangedEvent`、`DrawPromptEvent`、`TileDrawnEvent`、`FlowerExposedEvent`、`SelfDeclarePromptEvent`、`ClaimPromptEvent`、`ChowChoicePromptEvent`、`MeldDeclaredEvent`、`DiscardedEvent`、`PassedWinChangedEvent`、`HandEndedEvent`），並由 `Event.TargetSeat()` 標明私有收件者（公開事件為 `SeatUnknown`）。
- **理由**：
  1. `internal/table` 的 Perspective Projector 可直接依 `Audience` 與 `TargetSeat` 派發，不需拆解 `map[string]string` 或二次反推可見性。
  2. 私有事件（`TileDrawnEvent`、`SelfDeclarePromptEvent`、`ClaimPromptEvent`、`ChowChoicePromptEvent`、`PassedWinChangedEvent`）天然只會送往單一座位；公開事件（如 `MeldDeclaredEvent` 透過 `ProjectMeld(m, false)`）天然不含暗槓牌面。
- **替代方案**：每次 `Apply` 回傳龐大的全桌快照 struct 再由呼叫端自行 diff。容易在 diff 過程意外洩漏他家暗牌或漏發過水通知，故不採用。

### Decision 2: `ClaimWindow` 獨立封裝三種宣告視窗與吃法選擇（`internal/rules/claim.go`）
- **選擇**：將 `ClaimWindow` 設計為可獨立建構與測試的狀態機子元件，支援 `ClaimWindowDiscard`、`ClaimWindowRobKong` 與 `ClaimWindowFlower` 三種模式，並管理 `pending` 座位、`chowChoiceSeat`、`decisions` 與 `Resolve()`（胡＞槓＞碰＞吃 + 觸發者下家起算順時針截胡）。
- **理由**：宣告仲裁是麻將規則最容易出錯的併發決策核心；獨立封裝後可用 table-driven 測試窮舉驗證「多人平行決定」、「重複提交拒絕」、「吃被碰蓋過」、「雙家／三家截胡繞回」、「吃法選擇二階段」與「未成立宣告不外洩」。
- **細節**：當玩家有 >= 2 種吃法且提交 `ClaimChow` 時若 `Chow` 為零值，該座位轉入 `WaitChowChoice` 並發出私有 `ChowChoicePromptEvent`；若僅 1 種吃法或已攜帶合法 `ChowOption`，則一次完成決定。如此同時相容單步 API（AI / 測試）與雙步鍵盤互動（TUI）。

### Decision 3: `TurnEngine` 作為單局回合純 FSM，接受注入式初始配置與思考耗時（`internal/rules/turn.go`）
- **選擇**：定義 `TurnPhase`（`PhaseWaitDraw`、`PhaseWaitSelfDeclare`、`PhaseWaitDiscard`、`PhaseWaitClaims`、`PhaseWaitChowChoice`、`PhaseHandEnded`）與 `TurnEngine`：
  - 建構子 `NewTurnEngine(cfg TurnConfig) (*TurnEngine, error)` 接受已發牌（且已完成開局補花）的 4 家暗手／花牌、剩餘 `Wall`、圈風、各家門風、莊家與連莊數、底／台設定；啟動時直接呼叫 `Start() []Event` 檢查開局花牌宣告或直接進入莊家第一手 `SelfCheck`（不摸牌）。
  - 唯一狀態入口 `Apply(action PlayerAction, thinkElapsed time.Duration) ([]Event, error)`：驗證動作與當前 `TurnPhase`、拒絕非法輸入且**保證錯誤時零狀態污染**、回傳本次轉移產生的強型別事件序列。
  - 若 `thinkElapsed < 0` 回傳錯誤；出牌時將 `thinkElapsed` 累加至 `thinkingTime[seat]` 並隨 `DiscardedEvent` 公開。
- **理由**：切片 3 的 `HandEngine` 只需負責洗牌、抽門風、發牌、開局補花、亮牌與 `WaitReady`，開局補花完畢直接將牌桌交由 `TurnEngine` 驅動直到 `PhaseHandEnded`，職責清晰且零重複程式碼。

### Decision 4: 0.95m 嚴格過水與花牌宣告去重機制
- **選擇**：
  - `passedWin [5]bool` 追蹤各家過水狀態；放棄 `SelfDeclareSelfWin`、或在 `ClaimWindowDiscard`/`ClaimWindowRobKong` 擁有 `ClaimWin` 卻提交非胡選項時置為 `true` 並發送私有 `PassedWinChangedEvent{Passed: true}`。
  - 摸到或補到非花牌 `t` 時，若 `passedWin[seat]` 為真且 `!CanWin(concealedBefore, melds, t)`，則置為 `false` 並發送私有 `PassedWinChangedEvent{Passed: false}`；若可胡則維持過水且不提供自摸。
  - 花牌宣告去重以 `declinedFlowerKey`（記錄已放棄之 `(ownerSeat, targetCount)`）防止同一 7+1 或 8 花分布重複詢問；花牌視窗完全獨立於 `passedWin`。

### Decision 5: 可摸張數為 0 摸到花牌的邊界收斂
- **選擇**：若玩家摸走最後一張可摸牌（`DrawableRemaining() == 0`）恰好為花牌，系統先公開亮花並檢查花牌宣告（八仙過海／七搶一）；若無人宣告，因不可動用最後 16 張保留牌，直接轉入 `PhaseHandEnded`（`HandEndExhausted` 海底流局）。

## Risks / Trade-offs

- **[Risk] 暗手刪除牌或升級碰牌為加槓時，若先改狀態才驗錯，可能造成狀態半毀**
  → **Mitigation**：所有 `Apply` 路徑皆嚴格遵守「先完整驗證（phase、seat、tile 存在性、合法選項）→ 再套用狀態變更」，任何非法動作保證原子不變。
- **[Risk] `TurnEngine` 的 `Apply` 與狀態轉移邏輯容易膨脹超過 `gocyclo` 上限（15）**
  → **Mitigation**：依階段拆分為獨立小函式（`applyDraw`、`handleDrawnTile`、`handleFlowerDraw`、`enterSelfCheck`、`applySelfDeclare`、`applyDiscard`、`applyClaim`、`resolveActiveClaims`、`finishHandByWin`、`finishHandExhausted`），確保每個函式單一職責且複雜度低於 12。
- **[Risk] 天胡、地胡、人胡與杠上開花旗標在跨回合轉移時未即時重設造成誤判**
  → **Mitigation**：`afterSupplement` 僅於槓後補牌或中盤補花後設為 `true`，於該家出牌或轉移時立即清零；`anyMeldDeclared` 於任一吃／碰／明槓／加槓／暗槓成立即設為 `true`，搭配每家 `hasDiscarded[seat]` 精準控制第一輪條件，並撰寫專門的單元測試驗證。
