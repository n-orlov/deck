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

### Green: 20 consecutive targeted runs, all 20 under real scheduling pressure

Command (repeated 20 times, ~6.5s each):

```
ci/run.sh env DECK_GODOG_PATHS=filter.feature go test ./features/ -run TestFeatures -count=1
```

Captured by the committed harness `parallel-pressure-runs.sh`, which is what produced both logs and
re-runs the whole exercise unattended from the repository root. It starts a **second suite
invocation** — `ci/run.sh go test ./features/ -run TestFeatures -count=1`, i.e. the whole godog suite
over every feature file — in the background first, and only then executes the 20 targeted runs, so
the scheduling pressure of the failing stability run is live on the host throughout.

Overlap is established by the logs alone, not by narrative:

- `parallel-suite.log` is the second invocation's own output, bracketed by its wall-clock start and
  end: started `19:48:40.848`, ended `19:53:45.112`, `ok github.com/n-orlov/deck/features 303.304s`,
  `exit=0`.
- `20-consecutive-runs.log` header records that invocation's shell pid (`17594`), its sibling
  container id (`aebe2c4de218`) and that container's `docker inspect` `StartedAt`
  (`2026-08-28T19:48:41.153471769Z`) plus its `Cmd`.
- every one of the 20 run blocks carries its own `start=`/`end=` timestamps, its exit code, a
  `docker ps` snapshot of this run's sibling containers taken at the end of that run — in which
  container `aebe2c4de218` is still listed and ageing (`started=41 seconds ago`, `About a minute
  ago`, ...) — a `ps -p 17594` liveness check, and the derived `OVERLAP=yes/no` verdict.
- runs 1–20 span `19:48:41.895`–`19:50:57.824`, entirely inside the parallel invocation's
  `19:48:40.848`–`19:53:45.112` window.

Final line of `20-consecutive-runs.log`: `SUMMARY green=20/20 overlapped=20/20
parallel_suite_exit=0`. So all 20 runs passed and all 20 — not merely the 5 required — overlapped the
second suite invocation, which itself passed. Host: `Linux 7.0.12-201.fc44.x86_64`, `nproc: 28`,
tree at `54fd6e0` (the two dirty files the header reports are this harness script and this README).

## Files touched

- `features/kill_delete_undo_test.go` — the fix (shared `waitForSessionColumnState` helper,
  `sessionRowCount` accessor, four+one call sites converted).
- `features/godog_test.go` — untouched (`defaultTags` unchanged).
- `parallel-pressure-runs.sh` — the evidence harness for the green run above (report-local, not
  product or test code).
