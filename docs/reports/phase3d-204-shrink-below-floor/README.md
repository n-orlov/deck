# Task 204 — leave interactive mode and restore on a mid-session shrink below the 7-row floor

Review finding F3's second half: tasks 201-203 fixed enterInteractive's own AT-ENTRY
refusal (deck was observed entering interactive at 41x6 instead of refusing). That
fix only checks the floor once, at Enter. `previewTitle`/`previewContentSize`
(`internal/tui/tui.go:3092`) recompute the preview box's inner size on every render,
so nothing re-checked the floor after a shrink WHILE ALREADY interactive — the exact
same degrade-instead-of-refuse defect, reached by resize instead of by Enter.

## Fix

`internal/tui/tui.go`'s `Update`, `tea.WindowSizeMsg` case: right after `m.width`/
`m.height` take the new size, if `m.interactive` and the new `previewContentSize()`
is `width <= 0` or `height < interactiveMinInnerRows`, deck calls the ordinary
`exitInteractive()` (restores window geometry in II-9's order, releases ownership,
tears down the transport) and sets `m.attachError` to a message naming the floor and
offering `a`. This is the *default* behaviour named in task 204's successCriteria
(leave and restore), not the alternative — no finding entry was required in
`docs/reports/phase3b-findings.md`.

## Evidence in this directory

- `unit-test-red-without-fix.log` — `TestWindowShrinkBelowTheFloorLeavesInteractiveModeAndRestoresTheList`
  run against `tui.go` with the new `WindowSizeMsg` branch reverted (fix backed out,
  applied, tested, then restored): FAILs with "deck remained interactive at 41x6
  inner rows... F3's degrade defect, now reached by resize instead of entry".
- `unit-test-green.log` — the same test, with the fix in place: PASS. The test also
  proves non-vacuousness the other direction: a resize that stays above the floor
  (80x24 -> 80x20) must NOT leave interactive mode.
- `godog-new-scenario.log` — the new
  `@requirement-48-leave-interactive-mode-on-shrink-below-floor` scenario in
  `features/interactive_refusals.feature`, isolated tag run: enters interactive at
  full size, resizes the real PTY to 80x9 (requirement 48's own fixture size, whose
  `previewContentSize()` is 41x6), asserts the "7-row floor"/"press a to attach"/
  "deck - sessions" text and that the private tmux window's geometry is back to
  what it was captured as before Enter — PASS.
- `non-regression-runs.log` — isolated tag runs of the three existing
  `@requirement-47/48` refusal scenarios plus the passive
  `@requirement-21-preview-no-side-effects` scenario, and a `-run` filter selecting
  both II-11 exactly-two-SIGWINCH-per-cycle scenarios (untagged, selected by their
  godog-generated subtest name `TestFeatures/a_full_enter...`) — all PASS,
  unmodified.
- `full-features-package-green.log` — one full `ci/run.sh go test -count=1
  ./features/` run after the change: `ok ... 260.219s`, every scenario in the
  package (not just the ones touched here) green.
- `internal-tui-package-green.log` — `ci/run.sh go test -count=1 ./internal/tui/`:
  green.
- `vet-clean.log` / `gofmt-clean.log` — both empty (clean).

## Files changed

- `internal/tui/tui.go` — the `WindowSizeMsg` re-check.
- `internal/tui/interactive_test.go` —
  `TestWindowShrinkBelowTheFloorLeavesInteractiveModeAndRestoresTheList`.
- `features/interactive_refusals.feature` —
  `@requirement-48-leave-interactive-mode-on-shrink-below-floor` scenario.
