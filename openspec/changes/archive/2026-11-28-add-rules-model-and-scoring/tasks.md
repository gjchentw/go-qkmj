# Tasks

## 1. 牌型、座位與動作詞彙（`rules/game-model`）

- [x] 1.1 以 TDD 建立 `internal/rules/tile.go` 與 `internal/rules/tile_test.go`：實作從 1 開始的 `Tile`（`1..42`、`0=TileInvalid`）、`Suit`、`Wind`、`Seat`（`Seat1..Seat4` 含 `Next`/`Prev`/`Opposite`）、`MeldKind`、`Meld`（含等價張數與合法性檢查）及全形繁體中文名稱，並以 `make test-race` 驗證所有牌型與邊界案例通過
- [x] 1.2 以 TDD 建立 `internal/rules/wall.go` 與 `internal/rules/wall_test.go`：實作接受 `math/rand/v2.Source` 的 `NewWall`（`nil` 回傳 `ErrNilRandSource`）、144 張組成驗證、決定性洗牌、最後 16 張保留牌不可摸保護（`DrawableRemaining` / `Draw` / `ErrWallExhausted`），並以 `make test-race` 驗證通過
- [x] 1.3 以 TDD 建立 `internal/rules/action.go` 與 `internal/rules/action_test.go`：定義五類強型別玩家動作（摸牌、出牌、自身宣告、宣告、確認）與公開／私有／隱藏資訊結構及基本動作驗證，並以 `make test-race` 驗證通過

## 2. 胡牌拆解引擎（`rules/scoring`）

- [x] 2.1 以 TDD 建立 `internal/rules/decompose.go` 與 `internal/rules/decompose_test.go`：實作 `CanWin` 與 `Decompose`（枚舉所有 5 組 + 1 對拆法並標記 `WinTile` 歸屬的 `WinRole`，確保字牌不可組順子、花牌不可入暗手、多拆法去重），並以 `make test-race` 驗證通過

## 3. 台數計算與付款公式（`rules/scoring`）

- [x] 3.1 以 TDD 建立 `internal/rules/scoring.go` 與 `internal/rules/scoring_test.go`：實作 `Score(ScoreContext) (ScoreResult, error)`，涵蓋全部 QKMJ 基線台型（基本、情境、風牌、三元與字一色、花牌與七搶一／八仙過海、順子類含平胡真兩面聽、刻子與槓含精確暗刻歸屬、花色與么九）、顯式取代鏈、多拆法取最高總台數，以及各家收付與莊家 `1 + 2×連莊數` 加台公式，並以 `make test-race` 驗證通過
- [x] 3.2 建立 `internal/rules/scoring_samples_test.go`：將 `../qkmj/docs/qkmj-rules/scoring-samples.md` 全數 59 手黃金樣本（A01–A58 與 B01）加上搶槓、七搶一、八仙過海、平胡邊張／中洞／單釣、多拆法取最高台與莊家加台付款案例實作為 table-driven 測試，並以 `make test-race` 驗證全部通過

## 4. 整體品質與架構純粹性驗證

- [x] 4.1 驗證 `internal/rules` 僅匯入 Go 標準庫且不使用 goroutine、channel、I/O 或全域 RNG，執行 `make check`、`make coverage-check`（總覆蓋率 >= 80%）與 `GOWORK=off make check` 全部通過

## Workflow follow-up

- 執行 5 軸 `code-review-and-quality` 審查並解決所有 Critical / High 議題。
- 使用者確認後執行 `openspec-sync-specs`、`openspec-archive-change`，提交並推送，接著啟動切片 2（`rules/claim-arbitration` + `rules/turn-cycle`）。
