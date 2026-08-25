# Phase 3e report

Skeleton created by task 322 while closing out R58c (the padTrunc/truncateToWidth
call-site audit) — the audit's own success criteria requires its enumeration to
live here, not only in the commit message. Populated incrementally as later
tasks land; task 326 is responsible for filling in the remaining per-requirement
evidence table (R52-R58, R59-R62, the four sort sequences, the truncating
name/width fixture, matrix's palette evidence, and every revert-red proof)
before the phase closes.

## R58c: padTrunc/truncateToWidth call-site audit (task 322, commit `c86e422`)

Enumeration of every padTrunc/truncateToWidth call site, per call site, whether
it could leak an open background/SGR span past its own text into the
pad-fill/flanking columns a caller's own background wrapper spans:

1. `panel.go` `sidebarContentLine` (side-by-side sidebar rows) — task 321
   already fixed this: bg opens after the border, closes once at the end,
   AFTER padTrunc's pad-fill. No change needed here.

2. `panel.go` `fullBoxContentLine` (stacked-mode sidebar/preview panels AND
   every `framedDialog`/`framedDialogScrollable` dialog body) — **REAL GAP,
   FIXED**. Task 321 moved `sidebarRowLines` to open no background of its own
   (`settingsRenderRowOpen`), on the assumption `sidebarContentLine` (its one
   side-by-side caller) was the only site that needed the wrapper treatment.
   `renderStackedFrame`'s sidebar loop feeds the SAME `sidebarEntry.bg` into
   `fullBoxContentLine`, which never got the bg parameter at all — so stacked
   mode painted NO selection/stripe background whatsoever, not merely a
   too-narrow one. `fullBoxContentLine` now takes a `bg theme.Token` and
   opens/closes it exactly like `sidebarContentLine`; `renderStackedFrame`
   threads `sidebarEntry.bg` through. `framedDialog`/`framedDialogScrollable`
   pass `""` (dialogs have no per-row background, only foreground-coloured
   body text via `colorToken`, which self-resets per call — nothing to leak).

3. `panel.go` `collapsedStripContentLine` (3-column collapsed strip) — safe,
   no change. `collapsedStripLines()` emits only the plain »/digit glyphs with
   zero colour tokens (via `m.glyph`, no `colorToken`/`backgroundSGR` call), so
   there is no background, open or otherwise, to leak.

4. `panel.go` `previewContentLine` (preview panel body) — safe, no change.
   Preview content is foreign tmux pane output cropped by `cropRow`, whose two
   `truncateToWidth` calls are already escape-honest via task 320 (a truncated
   span closes itself). `previewContentLine` adds no background of its own
   around that text, so there is nothing this layer could leak that task 320
   doesn't already close.

5. `settings.go` `settingsLeftContentLine`/`settingsRightContentLine` (the
   settings takeover's category/field/env-entry/search-result lists) — **REAL
   GAP, FIXED**. `settingsRenderRow` used to open bg (selection/selection_idle)
   at the very start of a row's text and close it with a SINGLE trailing reset
   immediately after that row's own segments — before
   `settingsLeftContentLine`/`settingsRightContentLine`'s own padTrunc
   pad-fill and flanking padding columns ever ran. Exactly task 321's original
   sidebar bug, one level removed: the highlight stopped at `"> UI"` (4
   columns) instead of spanning the whole panel width. Fixed by introducing
   `settingsListLine{text, bg}` (mirrors `sidebarRowLines`/
   `sidebarContentLine`'s own split exactly): every category/field/env-entry/
   search-result row now composes its text via `settingsRenderRowOpen` (no bg,
   no closing reset) and carries its background token separately;
   `settingsLeftContentLine`/`settingsRightContentLine` open bg once after the
   border and close once at the very end, spanning the pad-fill. `fitLines` is
   now generic (`fitLines[T any]`) so it fits `[]settingsListLine` the same way
   it always fit `[]string` — the zero value (`text ""`, `bg ""`) is exactly
   "no background", matching a truly empty string row under the old shape.
   `settingsRenderRow` itself is now dead (nothing calls it) and removed;
   `settingsRenderRowOpen`'s doc comment updated to say so.

6. `settings.go` `settingsFooterLine` / `tui.go` `belowMinimumNotice` — safe,
   no change. Plain uncoloured text (`truncateToWidth` called directly, no
   `colorToken`/`backgroundSGR` anywhere in the string).

### Non-vacuous evidence

`internal/tui/panel_leak_audit_test.go`, three new tests, each red-first
verified by reverting commit `c86e422`'s `panel.go`/`settings.go`/`tui.go` diff
in place (kept the new test file) and rerunning:

```
TestStackedSidebarSelectionBackgroundFillsFullPanelWidth (gap 2 above)
  reverted: "stacked selected row: row 0 col 1 has no background at
  all, want #26324b" -- FAIL
  fixed:    PASS

TestSettingsCategoryRowBackgroundFillsFullPanelWidth (gap 5, left list)
  reverted: "settings category row: col 1 has no background at all,
  want #26324b" -- FAIL
  fixed:    PASS

TestSettingsFieldRowBackgroundFillsFullPanelWidth (gap 5, right list)
  reverted: "settings field row: col 31 has no background at all, want
  #26324b" -- FAIL
  fixed:    PASS
```

```
$ ci/run.sh go test -count=1 ./internal/tui/
ok   (0.5s)
$ ci/run.sh go build ./...
(clean)
$ ci/run.sh go vet ./internal/tui/...
(clean)
$ gofmt -l internal/tui/*.go
(clean)
```

### Discovered but out of scope for 322

Running the full `@settings` feature tag surfaces TWO pre-existing failures
present already at 321 (`f74ed7f`), unrelated to commit `c86e422` — confirmed
by reverting that commit's diff and rerunning both scenarios in isolation,
which fail identically on the unmodified 321 tree:

- "a settings takeover save of ui.theme changes the running client's own
  render live" (requirement 19) times out cycling the Theme field to
  "daylight": tasks 315/316 added three more built-in themes without updating
  this scenario's +/- cycle count.
- "settings offers clearing the recent-directory history" (requirement 17)
  times out waiting for "cleared recent directory history": task 303 inserted
  `ui.sort_order` between `group_by_workspace` and `clear_recent_cwds` without
  adding the extra `j` this scenario's own in-file comment ("task 215 inserted
  preview_fit... a sixth j is needed") already flags as necessary whenever a
  field is inserted — a seventh `j` is now needed and was never added.

Recorded as task 333 in `tasks.json` (fixed separately, see that task's
evidence once it lands, cross-referenced here by task 326).

## Per-requirement evidence table

_To be completed by task 326: R52-R58, R59-R62, plus the whole-suite and
stability evidence and every revert-red proof cited by sha and log path._
