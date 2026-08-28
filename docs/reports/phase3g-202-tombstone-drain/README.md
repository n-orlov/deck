# Task 202 — drain the whole expired-tombstone backlog (review finding 3, R79)

## Defect
`Store.SweepTombstones` (`internal/store/store.go`) always wrote its hourly
throttle stamp (`tombstone_sweep_last_run_at` in `ui_state`) after every real
pass, even when that pass left rows behind because the backlog was bigger
than one `tombstoneSweepBatchRows` (200) batch. The very next call — the
first `tuiReconcile` tick, a fraction of a second later per
`settings.Reconcile` (default 500ms) — then saw a fresh throttle stamp and
skipped entirely, so a backlog bigger than one batch needed one extra real
**hour** per leftover batch to finish draining, not the same open/startup
cycle. `TestSweepTombstonesReapsOneBoundedBatchPerCall` pinned exactly that:
it manufactured three calls an hour apart from each other to prove the
450-row backlog it seeded drained across those three widely-spaced calls —
i.e. it asserted the defect, not the fix.

## Fix
- `SweepTombstones` now returns `(bool, error)`: the bool reports whether at
  least one more row is still older than `deleteGrace` after this batch. The
  throttle stamp is written only when that bool is `false` — a backlog
  larger than one batch keeps `lastRun` at its prior value (or `0` on a
  fresh store), so the very next call is *still* treated as "never run" and
  performs its own batch immediately, with no wait.
- A new `DrainExpiredTombstones` calls `SweepTombstones` in a loop, driven
  purely by that returned bool, until the backlog is gone (or the hourly
  throttle genuinely has nothing left to do). It never calls `time.Now()`;
  `now` is forwarded unchanged to every call in the chain.
- `cmd/deck/main.go`'s pre-first-frame, store-open call site is UNCHANGED in
  shape: it still calls plain `SweepTombstones` once and ignores the bool,
  so it is still bounded to exactly one batch, however large the backlog.
- `cmd/deck/main.go`'s `tuiReconcile` closure (the tick caller, already
  invoked every `settings.Reconcile` — 500ms by default) now calls
  `DrainExpiredTombstones` instead of a single bounded pass, so the FIRST
  tick after store open finishes draining any backlog left over from the
  store-open call, still within the same open/startup cycle.

## Evidence
- `red.log`: the rewritten test file (`internal/store/tombstone_sweep_test.go`)
  applied against the PRE-fix `internal/store/store.go` /
  `cmd/deck/main.go` (both reverted via `git stash` for this run only, then
  restored). `internal/store` fails to build — `SweepTombstones` still
  returns a single `error`, so the new call sites (`more, err :=
  st.SweepTombstones(...)`) don't type-check. `cmd/deck`'s own failure in
  this log (`TestDeckBinaryEmptyHelpAndQuitThroughPTY`) is unrelated to this
  change (main.go was reverted alongside store.go for this snapshot) and is
  confirmed a pre-existing flake below, not a regression: it passes cleanly
  once the fix is restored (see `green.log`).
- `green.log` / `full.log`: same command, full fix in place — both green.
- `red-behavioural.log` / `green-behavioural.log`: the *behavioural* half of
  the revert-and-reproduce pair, added because the `red.log` above only shows
  the rewritten committed test failing to **build** against the pre-fix
  signature, which does not by itself display the defect. The probe
  (`r79-behavioural-probe.go.txt`, kept as `.txt` so it is not compiled by
  the module) uses no post-fix API: it seeds a 450-row expired backlog, makes
  the one bounded store-open pass, then makes ten `SweepTombstones` calls
  500ms apart (the default `settings.Reconcile` ticks of that same startup)
  and requires zero expired rows at the end.
  - pre-fix (`git worktree` at `dd90a28^`, `red-behavioural.log`): FAIL —
    `expired rows still present after the store-open pass and 10 reconcile
    ticks = 250, want 0`, i.e. the throttle stamped by the store-open pass
    turned every tick of that cycle into a no-op.
  - post-fix (`git worktree` at `dd90a28`, same probe with only its two call
    sites adapted to the two-value return, `green-behavioural.log`): PASS —
    the ticks reap 200 then 50 and the backlog is gone inside the same cycle.
  Both worktrees were throwaway and were removed after the runs; neither
  probe file is committed as a `_test.go`, so no extra test enters the suite.

## Expectation change (`TestSweepTombstonesReapsOneBoundedBatchPerCall`)
Rewritten in place (task 202's success criteria: no committed test may still
assert that a backlog survives store open). It still proves the
pre-first-frame call is bounded to exactly one batch (`more == true`, exactly
`tombstoneSweepBatchRows` rows reaped, oldest first) but then drains the
REST of the 450-row backlog via `DrainExpiredTombstones` — the production
continuation `cmd/deck`'s tick caller uses — all at the SAME `now` (no hour
jumps manufactured by the test's own clock), and asserts zero expired rows
remain afterwards. `TestSweepTombstonesThrottlesToAtMostOnceAnHour` and
`TestSweepTombstonesHonoursFrozenClock` are unchanged in intent (only
updated for the new two-value return) and stay green: every backlog in those
tests is a single row, so it is always fully drained in the same batch that
reaps it, and the throttle stamp is written exactly as before.

## Commands
```
ci/run.sh go test -count=1 ./internal/store/ ./internal/tui/ ./cmd/deck/
```
Exit 0 (`full.log`).
