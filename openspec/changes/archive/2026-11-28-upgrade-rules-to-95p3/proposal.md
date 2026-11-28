# Proposal

## Why

目前 `internal/rules` 已完成 100% QKMJ 0.94/基線麻將純規則引擎（涵蓋 `rules/{game-model,scoring,claim-arbitration,turn-cycle,hand-lifecycle,match-progression}` 六項規格）。為與原作者 QKMJ 0.95p3+ 的完整規則體系對齊並兼顧 `go-qkmj` 的公平性不變量，需將六份 `rules/*` 規格全面演進至 95p3+：補齊四種特殊和局（四風連和、四槓和、一炮三響、九么九和）、立直與天聽、宣告早裁（0.95o）、四圈（一將）統計資料（0.95p）以及三色同刻非門清減台（0.95 說明書）。

## What Changes

- **四種特殊和局（0.95e）**：擴充 `HandEndReason` 區分五種和局（`HandEndExhausted`、`HandEndFourWinds`、`HandEndFourKongs`、`HandEndTripleRon`、`HandEndNineTerminals`）並提供 `IsDraw() bool`；實作四風連和、四槓和（須兩家以上且第 4 槓補牌打出安全牌）、一炮三響（三家同時宣告胡改為和局）、九么九和（第一巡未中斷且暗手含 >= 9 種么九牌可選擇宣告）。五種和局皆統一比照海底流局：亮四家暗手與暗槓、零金額變動、`IncrementPlayedCount = false`、進入四人 `WaitReady`、莊家連莊（`dealerStreak++`）。
- **立直與天聽（0.95i–0.95m，公平版）**：擴充 `SelfDeclareKind` 新增 `SelfDeclareRiichi` 與 `SelfDeclareNineTerminals`；門清聽牌且可摸張數 > 0 時，可單一動作宣告立直並打出指定聽牌棄牌（保證真聽牌）；第一巡未中斷即標記天聽。立直後鎖定手牌（禁止吃、碰、暗槓、加槓，且僅能摸切剛摸進之牌，違規回傳 `ErrRiichiLocked`）；不揭露他家暗手以維護公平性；立直期間可胡而未胡即永久過水至本局結束。新增台型「立直 1 台」、「天聽 8 台（取代立直 1 台）」。
- **宣告早裁（0.95o）**：改進 `ClaimWindow` 仲裁判定，只要已提交之最高優先宣告不可能被尚未表態之座位超越，且（於可能構成一炮三響之視窗中）已無湊滿三家同胡的可能時，立即完成仲裁而不必等待低優先座位，同時即時結束未完成的 `WaitChowChoice`。
- **四圈（一將）統計（0.95p）**：在 `MatchEngine` 維護每四圈（東→南→西→北）各家輸贏金額、放槍、胡牌、自摸次數與注入式累計思考時間；當北風圈 4 號過莊回東風圈 1 號時，發出公開 `RoundCycleCompletedEvent` 並歸零統計繼續下一將。
- **台型規格演進（0.95 說明書）**：**BREAKING** — `FanThreeSuitedTriplets`（三色同刻）演進為門清 2 台、破壞門清 1 台（同步更新 `A19` 黃金測試案例並新增門清三色同刻 2 台驗證）。

## Capabilities

### New Capabilities
<!-- 本次為既有 6 份 rules/* 規格之全面演進，無需新增頂層 Capability 目錄 -->

### Modified Capabilities
- `rules/game-model`: 擴充強型別自身宣告詞彙（新增 `SelfDeclareRiichi`、`SelfDeclareNineTerminals`）與和局原因列舉（五種和局與 `IsDraw()`），並於公開資訊納入立直/天聽狀態與四圈統計。
- `rules/scoring`: 新增情境台型「立直（1 台）」與「天聽（8 台，取代立直）」；修改刻子與槓台型「三色同刻」為門清 2 台、破門清 1 台。
- `rules/claim-arbitration`: 新增一炮三響和局覆寫截胡、宣告早裁（0.95o，並與一炮三響安全共存），以及立直後過水永久鎖定至本局結束。
- `rules/turn-cycle`: 新增四風連和、四槓和、九么九和與立直/天聽回合流程與鎖定摸切規則。
- `rules/hand-lifecycle`: 將結束方式、亮牌、結算與全員確認規則由單一海底流局擴展為五種和局統一處理。
- `rules/match-progression`: 將連莊推進擴展至全部五種和局，並新增打完四圈（一將）發出統計事件後歸零繼續下一將。

## Impact

- **Affected Code**: `internal/rules/{action,events,scoring,claim,turn,hand,match}.go` 及其對應 `*_test.go`（含 `scoring_samples_test.go`）。
- **Architecture & Dependencies**: 維持 `internal/rules` 零外部套件依賴（僅 Go stdlib）、零 goroutine/channel/I/O/系統時鐘，並遵循 `golang-design-patterns` 之明確狀態模型、Fail-fast 驗證與低複雜度職責分解。
- **Downstream Compatibility**: `TurnEngine`、`HandEngine`、`MatchEngine` 既有簽章向後相容；新增 `RiichiDeclaredEvent` 與 `RoundCycleCompletedEvent` 實作既有 sealed `Event` 介面。
