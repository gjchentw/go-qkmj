# Proposal

## Why

`go-qkmj` 計畫以雙層 `<domain>/<capability>` 作為後續所有功能規格的邊界，但專案目前尚未建立統一的系統架構總綱，容易在平行開發各領域能力時出現跨層循環依賴、隱藏資訊外洩、或不同入口（SSH / Web）行為不一致的問題。現在上游依賴（`ginmill v1.2.0`、`go-kittyd v0.2.0`）皆已釋出就緒，正是先定錨真實程式開發架構與各 Capability Domain 邊界的最佳時機。

## What Changes

- 新增頂層架構總綱規格 `architecture`（`openspec/specs/architecture/spec.md`），作為全專案真實 Go 套件結構、依賴方向與系統不變量的權威基準。
- 以 Mermaid 架構圖明確描繪：
  1. 靜態分層、Go 套件結構（`cmd/qkmj` 與 `internal/*`）與 7 大 Capability Domains（`rules/*`、`store/*`、`table/*`、`bot/*`、`lobby/*`、`tui/*`、`adapter/*`）的對應與單向依賴規則。
  2. 執行期並發拓撲（SSH / Web 雙入口、Bounded In-Process Terminal Bridge、Single-Owner Lobby / Table Actors、統一 `table.Player` 公平性邊界與視角過濾投影）。
- 確立雙層 `<domain>/<capability>` 規格組織規範與後續演進原則（對應但不直接整包複製 `../qkmj` 參考規格）。
- 記錄關鍵系統設計取捨（Bounded In-Process `kittyd.Terminal` 的背壓/TTY/生命週期/Resize/Panic 防護、Consumer-Side Port 設計、語意化原子儲存邊界）。

## Capabilities

### New Capabilities

- `architecture`: 定義 `go-qkmj` 真實程式開發架構總綱，包含套件分層與單向依賴規則、7 大雙層 Capability Domain 邊界與映射索引、Single-Owner Actor 並發拓撲、統一 `Player` 介面與隱藏資訊隔離、Bounded In-Process 雙入口終端橋接、語意化原子儲存邊界與安全基線。

### Modified Capabilities

（無，本專案尚未建立任何既有主規格。）

## Impact

- **OpenSpec 規格體系**：於 `openspec/specs/architecture/spec.md` 建立頂層架構總綱，成為後續 `rules/*`、`store/*`、`table/*`、`bot/*`、`lobby/*`、`tui/*`、`adapter/*` 所有提案與實作必須遵守的邊界契約。
- **Go 程式碼與依賴**：本變更屬於純架構規格定錨，不新增或修改 Go 實作程式碼，也不更動 `go.mod`。
- **上游相依定位**：明確鎖定 Web 入口整合 `github.com/ginmills/ginmill v1.2.0` 與 `github.com/gjchentw/go-kittyd v0.2.0`、SSH 入口整合 `charm.land/wish/v2`、TUI 層整合 Charm v2（`bubbletea/v2` + `lipgloss/v2`）。
