# Retake: internal/tui/cure_01_01_2_reload_selection_test.go at cure-01-01-3's leaves

Task **retake-01-01-03-3**, depending on **cure-01-01-3** (fix commit
`610be00`: never let a background arrival steal an explicitly navigated
header-only-sidebar cursor, R136/R137, SPEC §11, #31/#32). Tree sha this
retake was taken at: `7d7f4f7fa2b54ccb249214c91db541931ef2d7d1` (HEAD at
the time of this task, clean and equal to `origin/main`).

## Why this file needed a retake

`internal/tui/cure_01_01_2_reload_selection_test.go` is cure-01-01-2's own
regression file (fix commit `3d058b5`), covering R136/R137 selection
normalization across background reload, folded-group creation intent,
and archived-reload header/row clamping. `cure-01-01-3` landed a further
fix to the same selection machinery (`Model.selectedByUser`,
`internal/tui/tui.go:365`-`384`, `:2612`, `:5885`-`5888`) that this file
does not itself touch but whose surrounding code paths it exercises, so
this retake confirms the four tests still pass at the tree cure-01-01-3
actually leaves (not merely at the moment cure-01-01-2's own commit was
authored).

`git diff 3d058b5 610be00 -- internal/tui/cure_01_01_2_reload_selection_test.go`
is empty: the file's content is unchanged between cure-01-01-2 and
cure-01-01-3; only the surrounding production code moved.

## Evidence (`ci/run.sh`, docker sibling, at `7d7f4f7`)

Targeted (all four tests in this file):

```
ci/run.sh go test -count=1 -v -run \
  'TestReview113ExplicitHeaderSurvivesBackgroundArrival|TestReview113NewSessionIntentCannotSelectFoldedRow|TestReview113ArchivedReloadCannotLeaveAbsentHeader|TestReview113ArchivedReloadCannotSelectHiddenRow' \
  ./internal/tui/
```

Result: all four `PASS`, `ok` overall — [`targeted.log`](targeted.log).

Full package (regression check, same sha):

```
ci/run.sh go test -count=1 ./internal/tui/
```

Result: `ok` — [`package.log`](package.log).

## Verdict

`internal/tui/cure_01_01_2_reload_selection_test.go`'s four tests
(`TestReview113ExplicitHeaderSurvivesBackgroundArrival`,
`TestReview113NewSessionIntentCannotSelectFoldedRow`,
`TestReview113ArchivedReloadCannotLeaveAbsentHeader`,
`TestReview113ArchivedReloadCannotSelectHiddenRow`) are re-taken
(re-recorded) at the tree cure-01-01-3 leaves and all pass, with the
surrounding `internal/tui` package also green at the same sha.
