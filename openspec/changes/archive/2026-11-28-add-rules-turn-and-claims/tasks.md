# Tasks

## 1. 強型別規則事件與可見性邊界（`rules/claim-arbitration`、`rules/turn-cycle`）

- [x] 1.1 以 TDD 建立 `internal/rules/events.go` 與 `internal/rules/events_test.go`：定義 `EventAudience`（公開／私有）、強型別 `Event` 介面與全部事件型別（含 `TurnChangedEvent`、`DrawPromptEvent`、`TileDrawnEvent`、`FlowerExposedEvent`、`SelfDeclarePromptEvent`、`ClaimPromptEvent`、`ChowChoicePromptEvent`、`MeldDeclaredEvent`、`DiscardedEvent`、`PassedWinChangedEvent`、`HandEndedEvent`），並驗證私有事件必帶明確 `TargetSeat`、暗槓事件不含牌面，以 `make test-race` 驗證通過

## 2. 宣告視窗與仲裁引擎（`rules/claim-arbitration`）

- [x] 2.1 以 TDD 建立 `internal/rules/claim.go` 與 `internal/rules/claim_test.go`：實作 `ClaimWindow`（支援出牌、搶槓、花牌三種視窗）、`ComputeChowOptions`、私有選項過濾、平行等待與不可更改決定、二階段 `WaitChowChoice`、胡＞槓＞碰＞吃優先仲裁、觸發者下家起算截胡及僅公開最終結果，並以 `make test-race` 驗證通過

## 3. 回合狀態機與 0.95m 嚴格過水（`rules/turn-cycle`、`rules/claim-arbitration`）

- [x] 3.1 以 TDD 建立 `internal/rules/turn.go` 與 `internal/rules/turn_test.go`（核心摸打與自身宣告）：實作 `TurnEngine`、`NewTurnEngine`、`Start` 與 `Apply`，涵蓋莊家第一手直接自查、手動摸牌與自動連續補花、全手暗槓與既有碰牌加槓掃描、槓後手動補牌與連續槓、加槓搶槓視窗及注入式累計思考時間，並以 `make test-race` 驗證通過
- [x] 3.2 擴充 `internal/rules/turn_test.go`（進階邊界與過水）：完整驗證 0.95m 嚴格過水（放棄放槍／搶槓／自摸進入過水、過水中摸可胡牌仍不給自摸、摸花維持過水、摸不可胡非花牌私下解除）、花牌宣告（八仙過海、七搶一、放棄不重問、不適用過水）、宣告結果轉移（跳座與移走牌河牌）、可摸張數 0 時僅能胡（海底摸月／河底撈魚／最後一張花牌保護保留牌流局）及非法動作零狀態污染，並以 `make test-race` 驗證通過

## 4. 整體品質與架構純粹性驗證

- [x] 4.1 驗證 `internal/rules` 僅匯入 Go 標準庫且不使用 goroutine、channel、I/O、`time.Now()` 或全域 RNG，執行 `make check`、`make coverage-check`（總覆蓋率 >= 80%）與 `GOWORK=off make check` 全部通過

## Workflow follow-up

- 執行 5 軸 `code-review-and-quality` 審查並解決所有 Critical / High 議題。
- 使用者確認後執行 `openspec-sync-specs`、`openspec-archive-change`，提交並推送，接著啟動切片 3（`rules/hand-lifecycle` + `rules/match-progression`）。
