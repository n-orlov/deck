# Task 404 — R57 body-cell seam colour coverage gap

## Gap

Before this task, `internal/tui/seam_border_test.go` (task 318, R57's SPEC
amendment 6584299) only sampled the seam's **T-junction** — the corner
glyph one row below/above the panel titles, at `frameTopRow` — for the
three focus states: sidebar focused, interactive (preview focused), and
neither focused (an overlay owns the keyboard). The T-junction glyph comes
from `previewTopLine`/`previewBottomLine` (`internal/tui/panel.go:510-538`),
which both explicitly branch on `seam` to pick `m.seamBorderToken()`. The
seam's plain **body** cells — every row strictly between the top and
bottom border rows — come from a *separate* call site,
`previewContentLine` (`internal/tui/panel.go:547-550`), which could regress
independently (e.g. reverted to the hard-coded `m.previewBorderToken()`
that task 318's own file comment says the seam used *before* that task)
while every existing T-junction test kept passing, because the two call
sites are never both exercised by the same assertion.

## Fix

Added three new unit tests to `internal/tui/seam_border_test.go`, mirroring
the three existing T-junction tests but reading a **body row** instead of
row 0:

- `TestSeamBodyRowIsFocusedWhenSidebarHasFocus` — sidebar focused, body
  seam column expected `border_focus`.
- `TestSeamBodyRowStaysFocusedInInteractiveMode` — interactive (preview
  focused), body seam column expected `border_focus`.
- `TestSeamBodyRowIsPlainBorderWhenNeitherPanelFocused` — theme picker
  open / filter input focused, body seam column expected plain `border`.

Two new helpers support them:

- `frameBottomRow` — `frameTopRow`'s mirror, locates the row carrying the
  frame's own bottom border (`m.box().bottomLeft` prefix), scanning from
  the end of the view.
- `frameBodyRow` — returns `frameTopRow+1`, verifying it is strictly less
  than `frameBottomRow`. `topRow+1` is always a content row for any box at
  least 3 rows tall (top border + ≥1 content row + bottom border), which
  every model these tests use (default 160×30, or the collapsed/stacked
  variants task 318's siblings exercise) satisfies.

No product code was changed and no `features/seam_focus.feature` change
was needed — the gap was a missing assertion in the unit-test coverage,
not a missing behaviour, and adding Go-level tests is enough to close it
under the task's own "(and/or internal/tui/seam_border_test.go)" wording.

## Red proof

Mutated `previewContentLine` (`internal/tui/panel.go:547-550`) to use
`m.previewBorderToken()` for the body's *left* (seam) cell too, i.e.
reverted it to task 318's own documented pre-fix behaviour:

```diff
 func (m Model) previewContentLine(width int, text string) string {
 	bc := m.box()
 	inner := width - 4
-	return m.borderColor(m.seamBorderToken(), bc.vertical) + " " + m.padTrunc(text, inner) + " " + m.borderColor(m.previewBorderToken(), bc.vertical)
+	return m.borderColor(m.previewBorderToken(), bc.vertical) + " " + m.padTrunc(text, inner) + " " + m.borderColor(m.previewBorderToken(), bc.vertical)
 }
```

`ci/run.sh go test -count=1 -run 'TestSeamBodyRow' -v ./internal/tui/`
against the mutant (`mutant-red.log`):

```
=== RUN   TestSeamBodyRowIsFocusedWhenSidebarHasFocus
    seam_border_test.go:113: sidebar focused: seam body row 1 = #334155, want border_focus token #0d9488 (either-panel-focused rule)
--- FAIL: TestSeamBodyRowIsFocusedWhenSidebarHasFocus (0.00s)
=== RUN   TestSeamBodyRowStaysFocusedInInteractiveMode
--- PASS: TestSeamBodyRowStaysFocusedInInteractiveMode (0.00s)
...
FAIL
```

Only the "sidebar focused, not interactive" case goes red under this
mutant — exactly the discriminator task 318's own file comment names for
the pre-existing T-junction test (`TestSeamIsFocusedWhenSidebarHasFocus`
"is this task's fixture trap"): when interactive, `previewBorderToken()`
itself already resolves to `border_focus` (the preview holds focus), so
that state is indistinguishable from the mutant; when neither panel is
focused, both tokens read plain `border`, also indistinguishable. Only the
sidebar-focused/preview-unfocused state proves the body seam column reads
`border_focus` for a reason OTHER than "the preview happens to be
focused" — which is exactly the property `seamBorderToken()` exists to
supply and `previewBorderToken()` does not.

## Green proof (before and after the mutant)

- `new-tests-green-before-mutant.log` — all three new tests green against
  the clean tree, before the mutant was ever applied.
- `restored-green.log` — `internal/tui/panel.go` restored from
  `/tmp/panel.go.orig` (`git diff internal/tui/panel.go` empty again), same
  three tests green again.

## Golden frame / package checks

- `internal-tui-package-green.log` — `ci/run.sh go test -count=1 ./internal/tui/`
  full package green after restoring, `go vet ./internal/tui/...` clean,
  `gofmt -l internal/tui/seam_border_test.go` empty.
- `golden-frame-green.log` — `ci/run.sh go test -count=1 -run TestGoldenMinimumFrame -v ./features/...`
  green (both runs, identical sha256, `TestGoldenMinimumFrameRowCount`
  green too). `git status --short` at task start and end shows only
  `internal/tui/seam_border_test.go` modified — no golden-frame fixture
  file was touched.

## Files changed

- `internal/tui/seam_border_test.go` — three new tests + two new helpers
  (`frameBottomRow`, `frameBodyRow`). No product code changed.
