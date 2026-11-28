# Tasks

## 1. 生命週期與牌局事件擴充

- [x] 1.1 以 TDD（RED -> GREEN -> REFACTOR）在 `internal/rules/events.go` 與 `internal/rules/events_test.go` 新增單局生命週期與牌局進程強型別事件（`MatchStartedEvent`、`HandStartedEvent`、`HandDealtPublicEvent`、`HandDealtEvent`、`HandsRevealedEvent`、`HandSettledEvent`、`WaitReadyEvent`、`PlayerReadyEvent`、`MatchProgressedEvent`、`MatchAbortedEvent` 與 `SeatBalanceChange`、`SeatReveal`），驗證所有事件之 `Audience()`、`TargetSeat()` 與 `HandsRevealedEvent` 暗槓牌面公開投影（`ProjectMeld(m, true)`），並通過 `make test-race`

## 2. 單局生命週期純狀態機（`rules/hand-lifecycle`）

- [x] 2.1 以 TDD 在 `internal/rules/hand.go` 與 `internal/rules/hand_test.go` 實作 `HandEngine` 建構子（含 Functional Options：`WithWall`、`WithDoorSeat` 及建構期驗證）、`HandPhase` 零值哨兵 Enum 與 `Start()` 自動開局流程（洗牌、均勻隨機開門決定門風、16/17 張發牌公私分離、莊家起順時針自動連續開局補花且期間不開啟花牌宣告視窗、銜接 `TurnEngine.Start()` 開局花牌宣告與莊家第一手），驗證完整事件序列、隱藏資訊隔離與 `make test-race`
- [x] 2.2 以 TDD 在 `internal/rules/hand.go` 與 `internal/rules/hand_test.go` 實作 `HandEngine.Apply` 與 `HandEngine.Abort`：胡牌（含開局八仙過海/七搶一、天胡、放槍、搶槓）與流局後自動依序輸出 `HandsRevealedEvent`（含揭露暗槓）、`HandSettledEvent`（每家 `Before`/`Delta`/`After` 與 `IncrementPlayedCount`）與 `WaitReadyEvent`；實作 `WaitReady` 四人確認（含流局亦須全員確認、拒絕重複確認與非確認動作）與中途離桌中止（不亮牌、不結算、不改餘額），並通過 `make test-race`

## 3. 外層牌局進程純狀態機（`rules/match-progression`）

- [x] 3.1 以 TDD 在 `internal/rules/match.go` 與 `internal/rules/match_test.go` 實作 `MatchEngine` 建構子（Functional Options：`WithStakes`、`WithWallFactory`、`WithDoorSeatPicker`，預設底 500/台 200 及建構期驗證）、`MatchPhase`、`StartMatch`（東風圈、1 號做莊、連莊 0 並自動開始第一局）、`Apply`（結算即時反映餘額、全員確認後依規則更新莊家/連莊/圈風並自動開啟下一局，含莊家胡牌連莊、流局連莊、閒家胡牌過莊、東→南→西→北→東無終點循環），並通過 `make test-race`
- [x] 3.2 以 TDD 在 `internal/rules/match.go` 與 `internal/rules/match_test.go` 實作 `MatchEngine.PlayerLeft` 與端對端多局整合測試：驗證摸打中途離桌不亮牌不結算並回 `Gathering`、`WaitReady` 階段離桌保留已結算餘額不回溯、再次湊滿 4 人重開時恢復東風圈 1 號連莊 0，以及負餘額玩家於同場牌局內可繼續遊玩，並通過 `make test-race`

## 4. 品質閘門與規格驗證

- [x] 4.1 驗證 `internal/rules` 僅依賴 Go stdlib（無 goroutine、channel、I/O、`time.Now()`、全域可變狀態與 `init()`），執行 `make check`、`make coverage-check`（覆蓋率 >= 80%）與 `GOWORK=off make check`，並完成 5 軸 `code-review-and-quality` 審查與所有 Critical/High 問題修復

## Workflow follow-up

- 使用者審查並確認後，將 `rules/hand-lifecycle` 與 `rules/match-progression` delta specs 同步至 `openspec/specs/` 並歸檔本變更。
- 歸檔完成後進入下一階段：開啟 95p3+ 進階規則 OpenSpec 變更（四風連和、四槓和、九么九和、一炮三響和局、立直/天聽、早裁、四圈統計等）。
