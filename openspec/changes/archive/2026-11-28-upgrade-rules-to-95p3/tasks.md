# Tasks

## 1. 動作詞彙、事件與和局列舉擴充（`rules/game-model`）

- [x] 1.1 以 TDD（RED -> GREEN -> REFACTOR）擴充 `internal/rules/action.go`、`internal/rules/events.go` 與對應測試：新增 `SelfDeclareRiichi`（須含有效非花棄牌）、`SelfDeclareNineTerminals`、`HandEndFourWinds`、`HandEndFourKongs`、`HandEndTripleRon`、`HandEndNineTerminals`、`HandEndReason.IsDraw()`、`RiichiDeclaredEvent`、`RoundCycleCompletedEvent`（含 `SeatCycleStats`）與 `SelfDeclarePromptEvent` 新欄位（`CanNineTerminals`、`CanRiichi`、`RiichiDiscardTiles`），並通過 `make test-race`

## 2. 計分演進（`rules/scoring`）

- [x] 2.1 以 TDD 在 `internal/rules/scoring.go`、`internal/rules/scoring_test.go` 與 `internal/rules/scoring_samples_test.go` 新增 `FanRiichi`（立直 1 台）與 `FanTianTing`（天聽 8 台，取代立直 1 台），更新 `ScoreContext`（`IsRiichi`、`IsTianTing`），並將 `FanThreeSuitedTriplets` 演進為門清 2 台、非門清 1 台（更新 `A19` 並新增門清三色同刻 2 台與立直/天聽測試），通過 `make test-race`

## 3. 宣告早裁與一炮三響（`rules/claim-arbitration`）

- [x] 3.1 以 TDD 在 `internal/rules/claim.go` 與 `internal/rules/claim_test.go` 實作一炮三響和局（`ClaimResolution.IsTripleRon`）與 0.95o 宣告早裁（`canEarlyResolve`：下家胡且無三響可能立即早裁、三家皆可胡時保留等待至排除或湊滿三響、碰/槓即時早裁並取消進行中之 `WaitChowChoice`），驗證完整邊界情境並通過 `make test-race`

## 4. 四種特殊和局與立直／天聽回合引擎（`rules/turn-cycle`）

- [x] 4.1 以 TDD 在 `internal/rules/decompose.go`（或 `internal/rules/turn.go`）與測試實作純函式 `FindRiichiDiscardTiles` 與 `countDistinctTerminalOrHonor`，驗證 16 張聽牌棄牌掃描準確性（含暗槓門清）與 9 種么九牌去重計數，並通過 `make test-race`
- [x] 4.2 以 TDD 在 `internal/rules/turn.go` 與 `internal/rules/turn_test.go` 實作四風連和（開局四連同風無人胡）、四槓和（兩家以上累計 4 槓且補牌打出安全牌；單家獨攬四槓不和局；第 4 槓補牌自摸或被胡/搶槓由胡優先）、九么九和（第一巡未中斷可選宣告）與一炮三響銜接，並通過 `make test-race`
- [x] 4.3 以 TDD 在 `internal/rules/turn.go` 與 `internal/rules/turn_test.go` 實作公平版立直與天聽：`SelfDeclareRiichi` 原子宣告並棄牌、公開 `RiichiDeclaredEvent`（不洩露他家暗手）、立直後禁止吃/碰/暗槓/加槓、出牌強制鎖定摸切（`ErrRiichiLocked`）、立直後過水永久鎖定至本局結束，以及天聽（8 台）與立直（1 台）實際結算，並通過 `make test-race`

## 5. 單局統一和局與四圈統計進程（`rules/hand-lifecycle` + `rules/match-progression`）

- [x] 5.1 以 TDD 在 `internal/rules/hand.go`、`internal/rules/match.go` 與對應測試統一五種和局處理（亮四家暗手與暗槓、零金額變動、`IncrementPlayedCount=false`、四人 `WaitReady`、莊家連莊 `dealerStreak++`），並實作 `roundCycleTracker` 與 `RoundCycleCompletedEvent`（打完北風圈回東風圈輸出四家輸贏、放槍、胡牌、自摸次數與累計耗時後歸零繼續下一將），通過 `make test-race`

## 6. 品質閘門與 5 軸審查

- [x] 6.1 驗證 `internal/rules` 維持純 Go stdlib 零外部依賴、無 goroutine/channel/I/O/`time.Now()`/`init()`，執行 `make check`、`make coverage-check`（覆蓋率 >= 80%）與 `GOWORK=off make check`，並完成 5 軸 `code-review-and-quality` 審查與所有 Critical/High 問題修復

## Workflow follow-up

- 使用者審查並確認後，將 6 份 delta specs 同步合併至 `openspec/specs/rules/*/spec.md` 並歸檔本變更。
- 歸檔完成後進入下一架構層（如 `internal/store` 或 `internal/table`）。
