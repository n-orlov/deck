# Task 805 — status_recovery.feature's stale-tmux-verdict scenario, made independent of a live-pane error surviving a tick

## The scenario and the latent race

`features/status_recovery.feature`'s "A row at error from a stale tmux
launch-failure verdict recovers on the next hook" scenario (requirement 45:
a stale lower-precedence verdict must not block a later hook from landing)
used to do this, right after creating session "recoverable" (which leaves the
row with a live tmux pane):

```gherkin
And the state database session "recoverable"'s status is forced to "error" from tmux as a stale launch-failure verdict
Then the state database session "recoverable" is "error" from "tmux" with killed_by_user=0
```

`forceSessionStatusToStaleTMuxVerdict` (`features/status_recovery_test.go`)
writes `status='error', status_source='tmux'` directly into the state
database with a single `UPDATE`. The very next assertion re-reads the row
with no wait at all (`databaseSessionTerminalFields`, `features/assertions_test.go`)
and expects to still see that raw forced value.

That is exactly the precondition `internal/service/reconcile.go`'s
`repairTerminalRowWithLivePane` exists to catch (SPEC §7 / requirement 76,
closed for this approach by tasks 801-804 and this plan's R76 design ruling):
a terminal status (`error` or `stopped`) paired with a live, non-dead tmux
pane is an invariant violation, not evidence to act on. The repair is
unconditional -- it does not look at `status_reason`, at how long ago
`status_at` was set, or at whether the verdict came from a "stale
launch-failure" narrative; it only checks liveness. Session "recoverable"'s
pane is alive (the session launched successfully before the forced
`UPDATE`), so the row is repair-eligible the moment the `UPDATE` commits.

Deck's reconcile loop ticks on a fixed cadence during every feature scenario:

```go
// features/lifecycle_test.go:206
const scenarioReconcileInterval = 250 * time.Millisecond
```

(passed to the process under test as `DECK_RECONCILE_MS=250`, `features/lifecycle_test.go:229`).
So between the forcing `UPDATE` and the following bare `SELECT`, a reconcile
tick landing first — a goroutine-scheduling delay, GC pause, or just CI load
stretching the gap past 250ms — repairs `error`/`tmux` to `starting`/`tmux`
(the neutral state a fresh pane always starts at, since liveness alone gives
an agent row no other verdict; see `repairTerminalRowWithLivePane`'s comment
block) before the assertion ever runs. The original scenario had no
synchronization against that repair at all: it was a race against a fixed,
externally-driven 250ms clock with no lower bound on how much wall time
elapses between the write and the read in a loaded CI sandbox, exactly the
same latent-race shape task 804 found and fixed in `sort_order.feature`
(quoted in `docs/reports/phase3g-804-sort-order-error-route/README.md`) and
the shape the comment already sitting above the "dup pane" scenario in this
same file names for that scenario's own case.

## Reproducing the race (red)

Per the standing evidence rule, the race was reproduced by forcing the tick
to land first, deterministically, in a throwaway `git worktree` at the
unmodified pre-fix commit (`46dad5e`), never touching the live workspace:

```
git worktree add --detach .scratch-805 HEAD   # HEAD == 46dad5e, unmodified
```

Inside the worktree only, one line was inserted between the forcing step and
the original assertion, forcing the scenario to *wait for the repair to
land* before making the original claim:

```gherkin
And the state database session "recoverable"'s status is forced to "error" from tmux as a stale launch-failure verdict
And within one configured reconcile interval the state database session "recoverable" is "starting" from "tmux" with killed_by_user=0
Then the state database session "recoverable" is "error" from "tmux" with killed_by_user=0
```

Run (siblings mount `$RALPHD_HOST_WORKSPACE`, which is why the worktree is
addressed as a subdirectory rather than by absolute path):

```
ci/run.sh sh -c 'cd .scratch-805 && DECK_GODOG_PATHS=status_recovery.feature go test ./features/ -run TestFeatures -count=1'
```

Result: `1 failed`, with the failing step exactly the original assertion,
proving the repair really does erase the forced "error" before that
assertion would otherwise have observed it:

```
step error: session "recoverable" terminal fields = status "starting", source "tmux", killed_by_user=0; want "error", "tmux", 0
```

Full (trimmed) output: `red-worktree-trial.log` in this directory. The
worktree was fully removed afterward (`git worktree remove --force
.scratch-805` + `git worktree prune`) and `git status --porcelain` was
confirmed empty before any commit — no experiment touched the live tree.

## The fix: re-point onto the SPEC §7 repair

Per this approach's standing R76 design ruling (the repair stays
unconditional; affected scenarios are re-pointed, not the repair narrowed),
the scenario now waits for the mandated repair instead of asserting the
pre-repair state with no wait, reusing the exact assertion route the "dup
pane" scenario earlier in this same file already established for this
repair:

```gherkin
And the state database session "recoverable"'s status is forced to "error" from tmux as a stale launch-failure verdict
Then within one configured reconcile interval the state database session "recoverable" is "starting" from "tmux" with killed_by_user=0
When fake Claude session "recoverable" fires "Stop" for itself using conversation identity:
  | last_assistant_message | recovered after stale tmux verdict |
Then within one configured reconcile interval deck client "A" screen contains "idle"
And the state database session "recoverable" is "idle" from "hook" with killed_by_user=0
And session "recoverable" has one "stop" event with payload field "last_assistant_message" equal to "recovered after stale tmux verdict"
```

Requirement 45's own point is untouched by this: the row now carries a
*different* lower-precedence, tmux-sourced verdict (`starting` instead of
`error`) when the hook fires, but it is still lower precedence than a hook,
and the scenario still proves the later `Stop` hook lands and wins — the
final assertions (`is "idle" from "hook"`, the `stop` event with its
`last_assistant_message` payload) are unchanged from before this fix. No
scenario was removed, retagged, or skipped; the file keeps its original 4
scenarios.

## Verification: 5 consecutive green runs

Command (identical each run):

```
ci/run.sh env DECK_GODOG_PATHS=status_recovery.feature go test ./features/ -run TestFeatures -count=1
```

| run | log | exit |
|---|---|---|
| 1 | `green-run-1.log` | `0` (`green-run-1.log.exitstatus`) |
| 2 | `green-run-2.log` | `0` (`green-run-2.log.exitstatus`) |
| 3 | `green-run-3.log` | `0` (`green-run-3.log.exitstatus`) |
| 4 | `green-run-4.log` | `0` (`green-run-4.log.exitstatus`) |
| 5 | `green-run-5.log` | `0` (`green-run-5.log.exitstatus`) |

All five: `ok  github.com/n-orlov/deck/features`.

## Files in this directory

- `red-worktree-trial.log` — the forced-race reproduction described above,
  run against the unmodified pre-fix scenario in a throwaway worktree
  (trimmed of an unrelated pty goroutine dump for size; the one load-bearing
  failure line is preserved verbatim, repeated at the end for clarity).
- `green-run-1.log` .. `green-run-5.log` + `.exitstatus` — the 5 consecutive
  post-fix runs.
