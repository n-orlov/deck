# Task 027 evidence: esc clears a filter held in force

## Fix
`internal/tui/tui.go`'s top-level `esc` case (Model.Update, ~line 2312) now
also clears a filter query held in force (m.filtering == false,
m.filterQuery != "") once it has cleared the mark set — but only when the
mark set was already empty, so one press never clears two things at once.

## Red evidence (binding reverted)
Reproduced by stashing only the `internal/tui/tui.go` change (new tests and
new feature scenario kept in place) and re-running:

- `ci/run.sh go test -count=1 ./internal/tui/ -run
  'TestEscClearsAFilterHeldInForceWithoutReopeningTheField|...'`
  -> see red-unit-tests.log: `TestEscClearsAFilterHeldInForceWithoutReopeningTheField`
  FAILs with `top-level esc left a held query in force: "alpha-agent"`.
  The three exclusivity tests (mark-set, help overlay, rename dialog) PASS
  unchanged, since they were never affected by the binding.

- `ci/run.sh env DECK_GODOG_PATHS=filter.feature go test ./features/ -run
  TestFeatures -count=1 -v` -> see red-feature-summary.log /
  red-feature-tail.log: the new scenario "esc clears a filter held in force
  at the plain list, with the text field never reopened" FAILs — the second
  session never reappears after the bare escape, and the client hangs
  waiting for "esc-hold-beta" until the 5s step timeout, then never reaches
  "exits cleanly" (SIGQUIT dump on the harness' cleanup kill).

## Green evidence (fix restored)
After `git stash pop` (restoring internal/tui/tui.go):
- `ci/run.sh go test -count=1 ./internal/tui/` -> ok
- `ci/run.sh env DECK_GODOG_PATHS=filter.feature go test ./features/ -run
  TestFeatures -count=1` -> ok (7 scenarios, all passed)
