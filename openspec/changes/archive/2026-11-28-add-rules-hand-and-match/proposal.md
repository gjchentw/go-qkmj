# Proposal

## Why

目前 `internal/rules` 已完成牌型與台數計分（Slice 1：`rules/game-model`、`rules/scoring`）以及單局內宣告仲裁與回合循環（Slice 2：`rules/claim-arbitration`、`rules/turn-cycle`），但仍缺少串起「洗牌→門風→發牌→開局補花→回合→亮牌→結算→全員確認」的完整單局生命週期，以及跨多局的「開賽→連莊/輪莊/換圈→無終點循環→有人離桌中止重設」外層牌局狀態機。補齊此切片即可完成 100% QKMJ 基線麻將純規則引擎，讓後續 `internal/table` 與 `internal/bot` 可直接驅動完整牌局，也為下一個 95p3+ 進階規則變更建立穩固基準。

## What Changes

- 新增 `internal/rules/hand.go`（與測試）：實作單局生命週期純狀態機 `HandEngine`，涵蓋洗牌、每局隨機開門決定門風、16/17 張發牌、莊家起順時針自動開局補花、開局花牌宣告與莊家第一手銜接（委派至 `TurnEngine`）、胡牌/流局後亮四家暗手與暗槓牌面、純計分結算（含金額變動與已玩局數旗標）、全員確認 `WaitReady`（含流局亦須四人皆確認），以及中途離桌中止（不亮牌、不結算）。
- 新增 `internal/rules/match.go`（與測試）：實作外層牌局進程純狀態機 `MatchEngine`，涵蓋 4 人入座開賽（東風圈、1 號做莊、連莊 0）、結算後立即反映帳戶餘額變動、全員確認後依規則更新莊家/連莊數/圈風（北風圈後回到東風圈無終點）並自動開下一局，以及任一階段有人離桌時立即作廢本局回到等人狀態（已結算之局不回溯，再次湊滿 4 人自東風圈 1 號連莊 0 重新開始）。
- 擴充 `internal/rules/events.go`（與測試）：補齊單局生命週期與牌局進程所需的強型別公開/私有事件（`MatchStartedEvent`、`HandStartedEvent`、`HandDealtPublicEvent`、`HandDealtEvent`、`HandsRevealedEvent`、`HandSettledEvent`、`WaitReadyEvent`、`PlayerReadyEvent`、`MatchProgressedEvent`、`MatchAbortedEvent`）。
- 依 `golang-design-patterns` 採用 Functional Options（`HandOption`、`MatchOption` 具建構期驗證）、零值哨兵 Enum（`HandPhase`、`MatchPhase`）、無 `init()` 與無全域可變狀態之純 stdlib 設計。

## Capabilities

### New Capabilities
- `rules/hand-lifecycle`: 定義單局完整生命週期純狀態機（洗牌、隨機開門分配門風、發牌、開局補花、開局花牌宣告、莊家第一手、胡牌/流局亮牌與結算、全員確認 `WaitReady`、中途離桌中止）。
- `rules/match-progression`: 定義外層牌局進程純狀態機（湊滿 4 人開賽、局後莊家/連莊/圈風推進、北風圈後回東風圈無終點循環、結算記入餘額不回溯、有人離開作廢本局回到等人並於下次開賽重設）。

### Modified Capabilities
<!-- 無既有規格需求變更 -->

## Impact

- **Affected Code**: `internal/rules/events.go`、`internal/rules/events_test.go`、新增 `internal/rules/hand.go`、`internal/rules/hand_test.go`、`internal/rules/match.go`、`internal/rules/match_test.go`。
- **Dependencies**: 維持 `internal/rules` 零外部套件依賴（僅 Go stdlib，含 `math/rand/v2` 注入式亂數來源），絕不引入 `internal/store`、`internal/table`、goroutine、channel、I/O 或系統時鐘。
- **Downstream Contracts**: `HandSettledEvent` 提供完整的每家餘額變化（`Before`/`Delta`/`After`）與 `IncrementPlayedCount` 旗標，供未來 `internal/table` 透過 `HandSettler` port 呼叫 `internal/store.SettleHand` 原子寫入帳戶。
