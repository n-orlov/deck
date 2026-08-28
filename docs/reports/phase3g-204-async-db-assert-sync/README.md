# Task 204 — synchronise the state-database assertions after an async keypress

## Root cause (stability run 7, review finding 1)

`docs/reports/phase3g-112-stability10/run-7.log:5019`:

```
Error: after scenario hook failed: session "filter-dd-archived" has deleted_at=1787938118179, want not tombstoned
```

`features/filter.feature`'s `@requirement-33-dd-reaches-and-tombstones-an-archived-row` scenario
presses `u` to undo a `dd` tombstone and immediately asserts
`the state database session "filter-dd-archived" is not tombstoned`. The `u` keypress dispatches
the undo's store mutation from a goroutine (an `internal/service` `Cmd`), exactly the way
`stateDatabaseSessionIsReaped`'s own pre-existing poll already accounted for the async
`deleteGraceExpired` reap tick. `stateDatabaseSessionIsNotTombstoned` (`features/kill_delete_undo_test.go:448`,
pre-fix) read `deleted_at` exactly once right after the keypress, so it raced the mutation and, on
run 7, lost: it observed the still-tombstoned row and failed the scenario.

## Fix

`features/kill_delete_undo_test.go` gets one shared helper, `waitForSessionColumnState`, used by
every state-database assertion reachable immediately after an async keypress:

- `stateDatabaseSessionIsTombstoned`
- `stateDatabaseSessionIsNotTombstoned`
- `stateDatabaseSessionIsArchived`
- `stateDatabaseSessionIsNotArchived`
- `stateDatabaseSessionIsReaped` (already polled via its own hand-rolled loop; now goes through the
  same shared helper via a `sessionRowCount` accessor, so there is exactly one bounded-wait
  implementation for the whole file instead of one per call site)

Each of these now polls its existing single-read accessor (`sessionDeletedAt`, `sessionArchivedAt`,
`sessionRowCount`) on a 25ms interval until the wanted value appears or a bounded deadline (3s for
tombstone/archive, 5s for reap, matching the pre-existing reap deadline) passes. No assertion was
weakened: `satisfied` is exactly the same boolean test the single read used to make inline, and once
the deadline passes the helper still returns a `fmt.Errorf` naming the column, the last observed
value, the elapsed timeout and the wanted state — the same failure shape the old code produced, just
reached after waiting instead of instantly.

## Evidence

### Red: the wait still fails clearly when the state never arrives

To prove the helper does not hide a genuine mismatch (only synchronises a real, eventually-true
one), `features/filter.feature` was temporarily edited so the step right after the `u` undo
asserted `is tombstoned` instead of `is not tombstoned` — a state the row will never reach, since
`u` clears `deleted_at` back to 0. Running the targeted scenario against the fixed helper:

```
$ ci/run.sh env DECK_GODOG_PATHS=filter.feature go test ./features/ -run TestFeatures -count=1
...
step error: session "filter-dd-archived" still has deleted_at=0 after 3s, want tombstoned
godog_test.go:39: godog feature suite failed
FAIL
FAIL	github.com/n-orlov/deck/features	9.086s
```

Full output: `red-timeout-never-arrives.log`. The scenario fails after the full 3s deadline with a
clear message naming the column, the value observed, the elapsed wait and the wanted state — exactly
the failure shape a genuinely-wrong assertion should produce, and not a silent pass. The `.feature`
edit was reverted immediately after capturing this log (`git checkout -- features/filter.feature`);
it is not part of the committed diff.

### Green: 20 consecutive targeted runs, ≥5 under real scheduling pressure

Command (repeated 20 times, ~6s each):

```
ci/run.sh env DECK_GODOG_PATHS=filter.feature go test ./features/ -run TestFeatures -count=1
```

All 20 runs exited 0 (`20-consecutive-runs.log`). A full `ci/run.sh go test -p=1 -count=1 ./...`
whole-suite run was started in the background immediately before the loop and was still executing
(inside its own `features` package run, per the baseline's ~316s figure) for the entire 124s the 20
targeted runs took (19:36:33–19:38:37 vs. the parallel suite's own start at 19:36:32, confirmed still
running via `ps` at +178s elapsed) — so every one of the 20 runs, not merely 5, ran with the second
suite invocation's CPU/scheduling pressure live on the same host.

## Files touched

- `features/kill_delete_undo_test.go` — the fix (shared `waitForSessionColumnState` helper,
  `sessionRowCount` accessor, four+one call sites converted).
- `features/godog_test.go` — untouched (`defaultTags` unchanged).
