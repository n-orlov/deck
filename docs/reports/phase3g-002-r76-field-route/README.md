# Task 002 evidence — field-route repair proof

## Command (both runs)

```
ci/run.sh env DECK_GODOG_PATHS=terminal_repair_field_route.feature go test ./features/ -run TestFeatures -count=1
```

## Green run (task 001's repair present, HEAD tree)

Scenario: `A SessionEnd hook wedges an agent row while its pane survives, and
Reconcile repairs it with nobody at the keyboard` in
`features/terminal_repair_field_route.feature`.

```
ok  	github.com/n-orlov/deck/features	1.465s
```

Re-ran 3 additional times for stability, all green, ~1.43s-1.48s each, no
flakiness observed.

## Red run (task 001 reverted)

Reverted task 001's product code only, for this verification, then restored
it immediately afterward (workspace left clean):

```
git checkout 1cfbd5a -- internal/service/reconcile.go internal/store/store.go
ci/run.sh env DECK_GODOG_PATHS=terminal_repair_field_route.feature go test ./features/ -run TestFeatures -count=1
git checkout HEAD -- internal/service/reconcile.go internal/store/store.go
```

Result: **FAIL**, exit code 1. Full raw output saved in `red-raw-full.log`
(includes a SIGQUIT goroutine dump the harness prints when a scenario's
deck-client process is killed after a failed assertion — that dump is normal
harness behavior on failure, not part of the assertion itself).

Key quoted lines from that run:

```
1 scenarios (1 failed)
11 steps (6 passed, 1 failed, 4 skipped)
--- FAIL: TestFeatures (5.39s)
    --- FAIL: TestFeatures/A_SessionEnd_hook_wedges_an_agent_row_while_its_pane_survives,_and_Reconcile_repairs_it_with_nobody_at_the_keyboard (5.38s)
    ...
    step error: session "wedged" terminal fields never reached status "starting", source "tmux", killed_by_user=0 within one reconcile interval; last observed status "stopped", source "hook", killed_by_user=0
    godog_test.go:39: godog feature suite failed
FAIL
FAIL	github.com/n-orlov/deck/features	5.397s
FAIL
```

This confirms: without task 001's `repairTerminalRowWithLivePane` fix, the
row wedged by the field-route hook (`SessionEnd` fired via the fake claude
binary while its pane/process stays alive, no state.db hand-editing) stays
stuck at `status="stopped", source="hook"` forever instead of being repaired
to `starting`/`tmux` within one reconcile interval — the scenario reproduces
the original bug and fails red on the pre-fix tree, and passes green on the
fixed tree.

## Note: unrelated discovered regression (not in scope for task 002)

While investigating, `features/status_recovery.feature`'s "dup pane"
scenario was observed to time out/hang against the current HEAD (with task
001's fix present). Task 001's `repairTerminalRowWithLivePane` appears to
convert that scenario's wedged "stopped" row back to "starting" before the
scenario's own assertion (expecting text `stopped - resumable`) is checked,
so the scenario never gets its expected pre-repair text and hangs. This is a
pre-existing regression from task 001, discovered as a side effect of
building this evidence — not fixed here since only task 002 is in scope for
this iteration. Flagged in notes.md for a future task.
