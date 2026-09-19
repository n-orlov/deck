# Retake: internal/store/ui_state_groups_test.go at cure-01-06's leaves

Task: retake-01-06-02 (review-cure origin, depends on cure-01-06).

## What was re-taken

All 6 test functions in `internal/store/ui_state_groups_test.go`, re-run at
HEAD `bc25f34` (cure-01-06's own commit `57e1a6a` — the AUTOINCREMENT fix for
groups.id — confirmed an ancestor of HEAD via `git merge-base --is-ancestor`;
HEAD also carries the rest of the cure wave and all prior retakes):

- `TestCollapsedGroupsRoundTripsAcrossReopen`
- `TestCollapsedGroupsAbsentRowYieldsEmptySet`
- `TestLastCreateGroupAbsentRowDegradesToDefault`
- `TestLastCreateGroupDanglingIDDegradesToDefaultNotEmptyValue`
- `TestLastCreateGroupOverwritesInPlace`
- `TestSetLastCreateGroupDefaultForgetsTheRememberedNamedGroup`

## Command run

```
ci/run.sh go test -v -count=1 -run '<the 6 test names above, |-joined>' ./internal/store/
```
(see `targeted.log`)

Package-wide check for regressions:

```
ci/run.sh go test -count=1 ./internal/store/
```
(see `package.log`)

## Result

All 6 tests PASS. `internal/store` package also green (3.269s), no
regression. No code changes were needed — this is a pure re-take/re-record
of existing passing tests at cure-01-06's leaves.
