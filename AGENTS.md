# AGENTS.md

Project instructions and workflow orchestration for `go-qkmj`.

## Workflow Orchestration (OpenSpec-Led Go TDD)

1. **Spec First**: All feature and behavior changes MUST start with an OpenSpec proposal and `tasks.md`. Do not write production code before the spec is approved.
2. **TDD Execution**: During OpenSpec `apply`, strictly follow `test-driven-development` (RED -> GREEN -> REFACTOR) for every task.
3. **Go Standards**:
   - Follow `use-modern-go` aligned with `go.mod`.
   - Apply `golang-testing`, `golang-design-patterns`, `golang-error-handling`, and `golang-concurrency`.
   - Every green verification MUST run `go test -race ./...` (or `make test-race`) — not just a single test file.
4. **Pre-Archive Gate**: Before archiving an OpenSpec change or opening a PR:
   - Run `make check` (`fmt-check`, `vet`, `lint`, `test-coverage`) and `GOWORK=off make check`.
   - Run `code-review-and-quality` (5-axes review: Correctness, Readability, Architecture, Security, Performance) against the full `git diff` and resolve all blocking (Critical / High) findings.
5. **Language Policy**:
   - OpenSpec artifacts (`proposal.md`, `design.md`, `specs/`, `tasks.md`) and user-facing UI/messages MUST use Traditional Chinese (正體中文). Code identifiers and concise technical comments stay in English.
6. **Multi-Repo Workspace Discipline**:
   - Local cross-repo development may use the parent `go.work`, but NEVER commit local `replace` directives in `go.mod`.
   - Release/tag upstream dependencies bottom-up (`ginmill` -> `go-kittyd` -> `go-qkmj`) and verify `GOWORK=off make check` before archiving or tagging.

## Architecture Invariants (`go-qkmj`)

1. **Strict Layering (No Reverse Dependencies)**:
   - `internal/rules`: Pure functions and pure FSM only. MUST NOT use goroutines, channels, I/O, global state, or wall-clock/global RNG (shuffle MUST accept an injected `rand.Source`).
   - `internal/table` & `internal/lobby`: Single-owner Actor goroutines with explicit dependency injection. MUST NOT use package-level singletons (e.g., no `GlobalLobby`) and MUST NOT import `gin`, `ginmill`, `go-kittyd`, `wish`, or `bubbletea`.
   - `internal/tui`: Charm v2 (`bubbletea/v2` + `lipgloss/v2` Canvas/Compositor). Communicate with core layers strictly via strongly-typed Commands and perspective-filtered Events — NEVER `map[string]string` payloads.
   - `internal/adapter/ssh` (`wish/v2`) & `internal/adapter/web` (`gin` + `ginmill` + `go-kittyd`): Thin terminal/PTY adapters only.
2. **Fair Play & Hidden Information Isolation**:
   - Wall order, opponents' concealed hands, concealed kong faces, and opponents' claim options MUST NEVER be included in any event delivered to a `Player` (including table host and AI bots) before rules permit disclosure.
   - AI bots interact strictly through the same `Player` interface as humans and MUST NOT access Table internals.
3. **Native UTF-8 / Unicode Terminal**:
   - Render tiles using full-width Traditional Chinese text (`一萬`, `紅中`, etc.) and Unicode box/block characters; do NOT use `U+1F000+` Mahjong emoji symbols as the default tile face.
   - NEVER compute column alignment from byte length; always use display width / grapheme width so default SSH clients worldwide align cleanly.
4. **Horizontal Scaling & Storage Boundary**:
   - Per-node independent Lobby & Table Actors with a Docker-style random Node Name displayed in the UI (overrideable via env/flag).
   - `internal/store` exposes semantic atomic operations only (e.g., `SettleHand`); NEVER leak `*sql.Tx` to business layers so deployments stay compatible with `rqlite` atomic batch transactions. Avoid high-frequency DB heartbeat writes.
   - Use pure-Go SQLite (`modernc.org/sqlite`) for local dev/tests and `rqlite` (1-node to 3-node Raft HA) for production.
5. **Security Baseline**:
   - Hash passwords with `Argon2id` (OWASP memory-hard parameters, PHC format, constant-time comparison); NEVER concatenate SQL strings; NEVER pass user input to `os/exec`; validate all external inputs at system boundaries.
6. **Upstream Reference (`../qkmj`)**:
   - Treat `../qkmj` (`openspec` branch) as a read-only reference oracle (`qkmj-rules/*` aligns closely; `qkmj-ux/*` MUST be adapted for native Unicode and Server-Side TUI). NEVER bulk-copy unimplemented specs into `openspec/specs/`.

## Verification Commands

- `make test` — Run all unit tests.
- `make test-race` — Run full test suite with Go race detector (`-race -count=1`).
- `make test-coverage` — Run tests with `-race` and generate coverage profile + per-function summary.
- `make coverage-check` — Verify total coverage meets `COVERAGE_MIN` (default `80%`).
- `make lint` — Run `golangci-lint` using `.golangci.yml`.
- `make check` — Run the complete pre-archive quality gate (`fmt-check`, `vet`, `lint`, `test-coverage`).
- `GOWORK=off make check` — Verify standalone build/test/lint without local `go.work`.
