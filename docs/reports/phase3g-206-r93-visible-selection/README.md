# Task 206 (R93 / steer 018): draw the drag-to-copy selection while the drag is in progress

SPEC §11.8's amended "Selection and copy" paragraph (task 205, `b69b5ba`): "An
in-progress selection is visible. From the press until the release, the
selected cells are marked with the `selection` token ... and the marking
clears when the release commits the copy... The marking is linear, not
rectangular, because the copy is: it covers exactly the run `SelectedText`
would return for the same anchor and current cell."

## What landed

- `internal/interactive/grid.go`: `Session.SelectionHighlightRange(offset,
  height, viewRow, fromCol, fromRow, toCol, toRow) (startCol, endCol int, ok
  bool)` — `AbsoluteRow`'s own sibling. It resolves `viewRow`'s membership by
  calling `AbsoluteRow` itself (never a second, independently derived row
  window) and applies the identical endpoint-swap / partial-first-row /
  partial-last-row column-bound rule `SelectedText`'s own doc comment
  describes, so the highlighted range and the copied text can never
  disagree.
- `internal/tui/interactive_select.go`: `highlightInProgressSelection` (the
  single grid draw's selection pass) and `highlightRangeSGR` (the
  ANSI/wide-rune-aware column-range wrap, reusing `panel.go`'s own
  `ansiEscapeLen`/`cellWidth`). It fires only while `m.interactiveSelecting`
  is true (press to release; `commitInteractiveSelection` already clears it
  unconditionally on release) and resolves the drag's anchor/current cells
  through the SAME `m.interactiveScrollOffset` and
  `interactiveGrid.AbsoluteRow` conversion `commitInteractiveSelection`
  itself uses. Highlighting is a BACKGROUND-only span (theme.Selection,
  closed with SGR 49 built via `fmt.Sprintf`, never a bare reset) so the
  pane's own foreground styling under the selection survives untouched.
- `internal/tui/interactive.go`: `interactiveBodyLines` calls
  `highlightInProgressSelection` right after `RenderRows`, before the
  not-repainted blank check — the one, single grid draw site.
- `internal/interactive/selection_test.go`: two new tests.
  `TestSelectionHighlightRangeAgreesWithSelectedTextAcrossWrappedRows` is
  the agreement property the task asks for: for a drag spanning three
  wrapped rows, the cells `SelectionHighlightRange` marks — read directly
  off the grid via the SAME `ScrollbackCellAt`/`CellAt` split
  `SelectedText` itself uses, never `SelectedText`'s own output reparsed —
  are joined (trimming each row's own trailing spaces, the same rule
  `SelectedText` applies) and compared byte-for-byte against
  `SelectedText`'s own return value for the identical anchor/current pair.
  No golden frame, no escape-code assertion.
  `TestSelectionHighlightRangeReportsNoHighlightOutsideSelectedRows` proves
  a row outside `[fromRow, toRow]` gets `ok == false`, not a zero-width
  range.
- No `internal/theme/builtin/*.toml` change and no new
  `internal/theme/contrast_test.go` pairing: the highlight only ever
  changes a row's BACKGROUND (theme.Selection); it never pairs a NEW
  foreground token against it, so the existing `text/selection` floor
  entry (`contrastChecks()`) already covers the general case.
- `internal/tui/panel.go`, `internal/tui/tui.go`,
  `internal/theme/builtin/*.toml`: no diff (`git diff --stat` — passive
  preview path and theme palette both untouched).
- `oscClipboardWriter`/`writeOSCClipboardBestEffort`/`SetSelectionBuffer`
  call sites: no diff (grep in `interactive_select.go` shows the exact
  same lines as before this task).

## Evidence (revert-and-reproduce, captured at implementation time)

- `before-fix.log`: this task's own new test file
  (`internal/interactive/selection_test.go`, the post-fix version) built
  against `b69b5ba` (the commit immediately before this task's code
  change, via `git worktree add --detach .scratch-206 b69b5ba`) —
  `go build`/`go test ./internal/interactive/` fails: `s.SelectionHighlightRange
  undefined`. The command run: `ci/run.sh sh -c 'cd .scratch-206 && go test
  -count=1 ./internal/interactive/'`.
- `after-fix.log`: the same two new tests green at this task's own code —
  `ci/run.sh go test -count=1 -v ./internal/interactive/ -run
  'TestSelectionHighlightRange'`.
- `targeted-suite.log`: the exact command the task's successCriteria names
  — `ci/run.sh go test -count=1 ./internal/interactive/ ./internal/tui/
  ./internal/theme/` — green.
- `interactive-preview-features.log`: every interactive-preview-touching
  `.feature` file (`interactive_selection.feature`,
  `interactive_focus.feature`, `interactive_geometry.feature`,
  `interactive_option_tables.feature`, `interactive_pipe_leak.feature`,
  `interactive_refusals.feature`, `interactive_repaint_notice.feature`,
  `interactive_scroll.feature`, `interactive_sigwinch_budget.feature`,
  `mouse.feature`, `preview.feature`, `seam_focus.feature`) run together —
  44/44 scenarios, 537/537 steps, green. Command:
  `ci/run.sh env DECK_GODOG_PATHS=<the 12 files above,comma-separated>
  go test ./features/ -run TestFeatures -count=1 -v`.
- Also confirmed unchanged and green:
  `TestSelectedTextAcrossRowsIsLinearNotRectangular` and
  `TestAbsoluteRowMatchesRenderRowsOwnWindow`
  (`internal/interactive/selection_test.go`, neither test's own body was
  touched by this task).

## Out of scope / not claimed

- No new `.feature` scenario asserting the visible highlight byte-for-byte
  — the task's successCriteria explicitly asks for the agreement property
  at the unit level ("not a golden-frame byte or escape-code assertion"),
  which `TestSelectionHighlightRangeAgreesWithSelectedTextAcrossWrappedRows`
  delivers; `interactive_selection.feature`'s existing two scenarios (from
  the prior approach's task 216) were re-run only to prove no regression.
- OSC 52 clipboard defect and F2/F22 flakes: untouched, not attempted, per
  standing rules.
