# Phase 4b Tier 1 evidence

This is **targeted Tier 1 evidence, not the whole-suite gate**. It exercises
only the packages and feature files Tier 1's own four requirements (R133
parts 1-2, R134, R135) touched, plus the golden-frame regression test. It is
not a substitute for the full `ci/run.sh go test -p=1 -count=1 ./...` sweep
that the tail sweep tasks (021-023) run separately.

Tier 1 code sha: `6fcb7bdb97c3e9314b441b87c17712414e11fdf6`
(`interactive: snap to the live bottom only when a keystroke actually
forwards bytes (#30)` — the last Tier 1 code commit, task 004/R135).

All evidence below was captured against a clean `git status --porcelain`
tree at that sha, via `ci/run.sh` (the Go/tmux toolchain sibling; this
container has no local Go toolchain).

## Evidence items

| # | Item | Invocation | Exit status | Log |
|---|------|-----------|:---:|-----|
| 1 | Build of the Go packages | `ci/run.sh go build ./...` | 0 | `build.log` |
| 2 | Vet of the Go packages | `ci/run.sh go vet ./...` | 0 | `vet.log` |
| 3 | `internal/tui` package tests | `ci/run.sh go test -count=1 ./internal/tui/` | 0 | `internal-tui.log` |
| 4 | `cmd/deck` package tests | `ci/run.sh go test -count=1 ./cmd/deck/...` | 0 | `cmd-deck.log` |
| 5 | godog run over the three interactive feature files | `ci/run.sh env DECK_GODOG_PATHS=interactive_scroll.feature,interactive_repaint_notice.feature,interactive_geometry.feature go test -count=1 -run TestFeatures -v ./features/` | 0 | `godog-three-features.log` |
| 6 | Golden-frame test (`features/golden_frame_test.go`) | `ci/run.sh go test -count=1 -run TestGoldenMinimumFrame -v ./features/` | 0 | `golden-frame.log` |

All six exit statuses are `0` (green), taken from the captured log's own
`EXIT:$?` trailer (items 1-4) or the test binary's own terminal `PASS`/`ok`
line plus the same trailer (items 5-6) — never inferred from log prose.

### Item 5 detail — scenario count

`godog-three-features.log` reports:

```
9 scenarios (9 passed)
118 steps (118 passed)
```

covering `features/interactive_scroll.feature`,
`features/interactive_repaint_notice.feature` and
`features/interactive_geometry.feature` together (paths passed
comma-separated via `DECK_GODOG_PATHS`, per `features/godog_test.go`'s
`godogPaths()`). `DECK_GODOG_TAGS` was left at its default
(`~@real-agents && ~@nightly`); none of the scenarios in these three files
carry either excluded tag, so the default-tag run covers all of them.

### Item 6 detail

`golden-frame.log` shows both `TestGoldenMinimumFrame` (2 repeated runs,
identical sha256 `6b0187d02eb6f51845dbce68ccd895b4f03f96179369395c29c7b824af305dda`,
theme `empire`, `DECK_COLOR_DEPTH=truecolor`) and the companion
`TestGoldenMinimumFrameRowCount` passing in the same package invocation.
