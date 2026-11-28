# Proposal

## Why

架構總綱 `architecture` 已定錨 `go-qkmj` 的單向分層與雙層 Capability Domain 邊界，現在需要率先建立最底層、零外部依賴的純領域核心 `internal/rules` 第一個垂直切片：`rules/game-model` 與 `rules/scoring`。以 QKMJ 基線規則落地牌型模型、注入式亂數洗牌牆、胡牌拆解與完整台數／付款計算，能先建立高覆蓋率的決定性測試基石，再依序推進後續的宣告仲裁、回合與牌局狀態機（以及基線完成後的 95p3+ 擴充規則）。

## What Changes

- 新增 `rules/game-model` 能力規格與 `internal/rules` 基礎模型：
  - 144 張台灣 16 張麻將牌型詞彙（萬／索／筒／風／三元／8 張花牌）與全形繁體中文名稱。
  - 4 座位輪轉（`Seat1` → `Seat2` → `Seat3` → `Seat4` → `Seat1`）、牌組（吃／碰／明槓／加槓／暗槓）、手牌張數不變量（非回合 16 張、摸／吃／碰後 17 張、每組牌組計 3 張）。
  - 注入 `math/rand/v2.Source` 的 144 張隨機洗牌牌牆與最後 16 張不可摸保留牌保護。
  - 強型別玩家動作詞彙與公開／私有／隱藏資訊可見性契約。
- 新增 `rules/scoring` 能力規格與無狀態計台／付款純函式：
  - 5 組 + 1 對胡牌型判定（`CanWin`）與全拆法列舉（`Decompose`），標記胡牌張歸屬以精準區分平胡兩面聽與暗刻歸屬。
  - QKMJ 基線全部台型與取代鏈（基本台、情境台、風牌、三元與字一色、花牌與七搶一／八仙過海、順子類、刻子與槓、花色與么九），取總台數最高之拆法。
  - 付款公式（預設底 500、每台 200，支付者為莊家另加 `1 + 2×連莊數` 台）與完整胡牌結果結構。
  - 納入 59 手 QKMJ 黃金測試樣本與邊界案例（搶槓、七搶一、八仙過海、平胡邊張／中洞／兩面聽、暗槓保門清、多拆法取最高台）作為 Go TDD 測試。

## Capabilities

### New Capabilities

- `rules/game-model`: 定義 `internal/rules` 共用的牌型、座位與輪轉順序、注入 RNG 的 144 張牌牆與 16 張保留牌、手牌與牌組、玩家動作詞彙，以及公開／私有／隱藏資訊邊界。
- `rules/scoring`: 定義無狀態的胡牌型判定、多拆法列舉與取最高總台數、QKMJ 基線全部台型條件與取代關係、各家付款公式與胡牌結果契約。

### Modified Capabilities

（無，不變更既有的 `architecture` 主規格。）

## Impact

- **Go 程式碼**：新增純標準庫套件 `internal/rules/`（`tile.go`、`wall.go`、`action.go`、`decompose.go`、`scoring.go` 及對應 `*_test.go`），零第三方套件依賴、零 goroutine、零 I/O。
- **後續切片解鎖**：為切片 2（`rules/claim-arbitration`、`rules/turn-cycle`）與切片 3（`rules/hand-lifecycle`、`rules/match-progression`）提供共用詞彙、`Wall`、`CanWin` 與 `Score` 函式；未來 95p3+ 擴充規則（四風連和、立直等）將以增量變更建構於此基線之上。
