# Design

## Context

本變更實作架構總綱 [architecture](../../../specs/architecture/spec.md) 中 `internal/rules` 套件的第一個垂直切片：`rules/game-model` 與 `rules/scoring`。動機與範圍見 [proposal.md](proposal.md)，規範需求見 [specs/rules/game-model/spec.md](specs/rules/game-model/spec.md) 與 [specs/rules/scoring/spec.md](specs/rules/scoring/spec.md)。

關鍵架構約束：
- `internal/rules` MUST 僅匯入 Go 標準函式庫（Go 1.24.3），嚴禁 goroutine、channel、I/O、全域可變狀態、系統時鐘與全域 RNG。
- 所有台數與拆法判定 100% 對齊 `../qkmj` 基線規則（並以 `../qkmj/docs/qkmj-rules/scoring-samples.md` 59 手黃金樣本驗證），95p3+ 擴充規則留待基線 3 個切片完成後以獨立增量變更補上。

## Goals / Non-Goals

**Goals:**
- 以零配置或低配置（`[TileCount]int` 固定計數陣列）實作高速、決定性的 5 組 + 1 對全拆法列舉與計台引擎。
- 以從 1 開始的強型別列舉（0 為無效哨兵）讓非法牌型、非法座位、非法動作在型別層與建構邊界即時暴露。
- 提供清晰、無狀態的 `CanWin`、`Decompose` 與 `Score` API，供後續切片 2（`claim-arbitration`、`turn-cycle`）與切片 3（`hand-lifecycle`、`match-progression`）直接複用。

**Non-Goals:**
- 不實作回合、宣告視窗、單局與多局狀態機（歸屬切片 2 與切片 3）。
- 不實作 95p3+ 候選規則（立直、天聽、四風連和、四槓和、九么九和、一炮三響和局、四圈統計）。
- 不處理帳戶餘額持久化或 TUI 畫面排版（分別歸屬 `internal/store` 與 `internal/tui`）。

## Decisions

### Decision 1: `Tile` 採用緊湊 `uint8` 編碼（`1..42`，`0` 為 `TileInvalid`）
- **選擇**：將 42 種牌以連續整數 `1..42` 定義（`1..9` 萬、`10..18` 索、`19..27` 筒、`28..31` 東南西北風、`32..34` 紅中白板青發、`35..42` 春夏秋冬梅蘭菊竹），並提供 `TileInvalid Tile = 0` 與常數 `TileMax = 42`、`NonFlowerMax = 34`。
- **理由**：
  1. 胡牌拆解（`CanWin` / `Decompose`）可直接使用堆疊上的 `[NonFlowerMax + 1]int` 固定陣列，免去 map 雜湊與大量切片分配，單次胡牌判定在微秒級完成。
  2. 零值 `Tile(0)` 天然代表無效值，符合 Go 慣用防呆原則，避免未初始化結構誤被視為「一萬」。
  3. 透過常數區間即可 $O(1)$ 判斷花色、點數、字牌、么九、中張與花牌對應門風。
- **替代方案**：使用 `struct { Suit Suit; Rank uint8 }` 複合型別。容易產生非法組合（如花牌配點數 9），且作為陣列索引需額外轉換，故不採用。

### Decision 2: `Wall` 接受注入的 `math/rand/v2.Source`
- **選擇**：`NewWall(src rand.Source) (*Wall, error)` 接受 Go 1.24 標準庫 `math/rand/v2.Source`，以 `rand.New(src)` 執行 Fisher-Yates 洗牌；若 `src == nil` 回傳 `ErrNilRandSource`。
- **理由**：符合 `AGENTS.md` 與 `architecture` 總綱的純粹性要求，正式環境可注入由 `crypto/rand` 播種的 `ChaCha8` source（不可預測），測試環境可注入固定種子的 `PCG` source（100% 決定性重播）。
- **替代方案**：使用全域 `math/rand/v2.Shuffle` 或舊版 `math/rand`。會引入隱藏全域狀態，違反架構不變量，故不採用。

### Decision 3: `ScoreContext` 明確分離 `Concealed` 與 `WinTile`，並於 `Decompose` 追蹤胡牌張歸屬
- **選擇**：`ScoreContext` 接收胡牌前的暗手切片 `Concealed []Tile` 與單張 `WinTile Tile`，由 `Decompose(concealed, melds, winTile)` 輸出所有合法的 `Decomposition`（包含 5 組 `Group`、1 對 `Pair` 以及 `WinRole`：標記 `WinTile` 究竟是作為眼、暗刻/明刻、或是某組順子中的左/中/右位置）。
- **理由**：
  1. **平胡兩面聽**：只需檢查 `WinTile` 所在順子 `[r, r+1, r+2]`，當 `WinTile == r && r <= 6` 或 `WinTile == r+2 && r >= 2` 即為真兩面聽，自動排除邊張（`12→3`、`89→7`）、中洞與單釣。
  2. **暗刻歸屬**：當 `WinKind` 為放槍或搶槓時，僅排除 `WinRole` 指向為「由 `WinTile` 湊成之刻子」的那一組，徹底避免原版 0.94 樣本 A15（胡牌湊順子卻誤扣同牌種暗刻）的 bug。
  3. **多拆法取最高台**：同一手牌若有多種 `WinRole`（例如 `WinTile` 可當眼也可當兩面順子），每種拆法與 `WinRole` 組合都會獨立評估台數並取總台數最高者。
- **替代方案**：將 `WinTile` 直接併入 `Concealed` 後再反推。會遺失「哪一組使用了 `WinTile`」的語意，難以正確處理平胡與暗刻邊界，故不採用。

### Decision 4: 台型定義採「宣告式台型清單 + 顯式取代集合」
- **選擇**：定義強型別 `FanID`（從 1 開始）與對應台型名稱／台數，於單一拆法評估完所有成立條件後，以明確的取代規則（例如 `門清自摸` 取代 `門清`+`自摸`、`五暗刻`/`字一色`/`混老頭`/`清老頭` 取代 `碰碰胡`、`清老頭` 取代 `純全帶麼`+`混老頭`、`春夏秋冬` 取代對應正花台）過濾後加總，確保同一取代鏈只寫在單一位置且結果排序穩定。
- **理由**：取代邏輯集中、易讀、零隱式副作用，未來 95p3+ 擴充台型（立直、天聽）時可直接擴充 `FanID` 而不破壞既有邏輯。

### Decision 5: 檔案切分與純粹性測試
- **選擇**：在 `internal/rules/` 下切分為 5 個高內聚檔案：`tile.go`（牌型／花色／風／座位／牌組）、`wall.go`（牌牆與 16 張保留牌）、`action.go`（強型別動作與可見性型別）、`decompose.go`（`CanWin` / `Decompose`）、`scoring.go`（`Score` 與付款計算），並搭配對應單元測試與 `scoring_samples_test.go`（完整收錄 59 手 QKMJ 黃金樣本）。

## Risks / Trade-offs

- **[Risk] 複雜清一色或九蓮寶燈類手牌存在多種拆法與 `WinRole` 組合，可能出現重複拆法或指數爆炸**
  → **Mitigation**：對暗手拆法以「固定順序枚舉對子 → 按最小剩餘牌遞迴拆刻子／順子（強制順子與刻子規範序）」去重，並僅針對包含 `WinTile` 的候選位置展開 `WinRole`；對 16 張麻將而言，合法拆法極小（通常 < 32 種），執行耗時遠低於 1ms。
- **[Risk] 台型取代鏈遺漏邊界組合（如混老頭同時符合碰碰胡與混全帶麼、字一色同時符合碰碰胡與四暗刻、五暗刻同時符合碰碰胡）**
  → **Mitigation**：以表格驅動單元測試完整覆蓋 59 手黃金樣本（特別是 A15、A25、A26、A27、A34、A37、A38、A43、A46–A51、A58、B01）外加額外高台複合手牌（字一色+五暗刻、清老頭+四暗刻、搶槓、七搶一、八仙過海、平胡邊張／中洞／單釣）。
- **[Risk] 付款預設底／台（500 / 200）若呼叫端未設定為 0，可能導致金額算成 0**
  → **Mitigation**：定義常數 `DefaultBase = 500`、`DefaultPerFan = 200`；當 `ScoreContext.Base == 0 && ScoreContext.PerFan == 0` 時自動套用預設值，若傳入負數則回傳驗證錯誤。
