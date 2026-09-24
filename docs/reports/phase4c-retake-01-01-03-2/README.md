# Retake: internal/tui/new_session_select_test.go at cure-01-01-2's leaves

Task **retake-01-01-03-2**, depending on **cure-01-01-2** (R137/R136:
selection never lands on a hidden row or absent filtered header; reload
preserves selection identity, fix commit `3d058b5`). Tree sha this retake
was taken at: `806414a11e7516ddf2d7ff5b6e5a161720b72a93` (HEAD at the time
of this task, clean and equal to `origin/main`).

## Why this file needed a retake

`new_session_select_test.go` covers requirement 52's one-shot
`pendingSelectSessionID` selection intent (introduced `d7f304d`, later
adjusted for the header-or-row cursor type in `0d46c45`). It was not
itself edited by cure-01-01-2's fix commit, but cure-01-01-2 changed the
selection-normalization and one-shot-creation-override code paths this
file's tests exercise (the same `tui.go` machinery that also now runs the
folded-group/archived-reload/explicit-header logic added by that cure).
This retake re-runs all five tests in the file at the tree cure-01-01-2
actually leaves, to confirm they still pass after that fix landed (not
merely at the moment the cure's own commit was authored).

## Evidence (`ci/run.sh`, docker sibling, at `806414a`)

Targeted (all five tests in this file):

```
ci/run.sh go test -count=1 -v -run \
  'TestPendingSelectSessionIDSelectsNewRowNotIndexZero|TestPendingSelectSessionIDIsOneShot|TestPendingSelectSessionIDNeverAppearing|TestPendingSelectSessionIDRespectsActiveFilter|TestPendingSelectSessionIDScrollsIntoView' \
  ./internal/tui/
```

Result: all five `PASS`, `ok` overall — [`targeted.log`](targeted.log).

Full package (regression check, same sha):

```
ci/run.sh go test -count=1 ./internal/tui/
```

Result: `ok` — [`package.log`](package.log).

## Verdict

`internal/tui/new_session_select_test.go`'s five tests
(`TestPendingSelectSessionIDSelectsNewRowNotIndexZero`,
`TestPendingSelectSessionIDIsOneShot`,
`TestPendingSelectSessionIDNeverAppearing`,
`TestPendingSelectSessionIDRespectsActiveFilter`,
`TestPendingSelectSessionIDScrollsIntoView`) are re-taken (re-recorded) at
the tree cure-01-01-2 leaves and all pass, with the surrounding
`internal/tui` package also green at the same sha.
