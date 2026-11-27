# Tasks

## 1. 架構總綱規格審查與驗證

- [x] 1.1 驗證 `openspec/changes/add-architecture-overview/specs/architecture/spec.md` 與 `proposal.md`、`design.md` 一致性，執行 `openspec validate add-architecture-overview --strict` 確認 0 errors
- [x] 1.2 檢核總綱規格中的兩張 Mermaid 圖（靜態分層與 Capability Domain 映射圖、執行期並發拓撲與公平性邊界圖），確認 7 大雙層 Capability Domains（`rules/*`、`store/*`、`table/*`、`bot/*`、`lobby/*`、`tui/*`、`adapter/*`）與 Go package 路徑、`AGENTS.md` 不變量完全對齊
- [x] 1.3 檢核 `design.md` 中 Bounded In-Process `kittyd.Terminal` Bridge 的 5 項風險緩解策略、Consumer-Side Port 依賴方向與語意化原子儲存邊界已完整對應至 `specs/architecture/spec.md` 的 Requirements 與 Scenarios

## Workflow follow-up

- 使用者審閱通過後執行規格同步（`openspec-sync-specs`）與封存（`openspec-archive-change`），將總綱落地至 `openspec/specs/architecture/spec.md` 並執行 `openspec validate --all --strict`。
- 提交並推送至 `gjchentw/go-qkmj` 主分支，作為後續 `rules/*` → `store/*` → `table/*` → `bot/*` → `lobby/*` → `tui/*` → `adapter/*` 各領域提案的共用基準。
