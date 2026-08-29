# Task 804 — sort_order.feature's error rows given a route the reconcile cannot repair

## The race (named by task 801/F36, left latent by 801/802/803)

`features/sort_order.feature` posed its `ord-bravo`/`gso-ab` "error" tier
directly with the harness's raw `the state database session "X" has status
"error" N seconds ago` write, while the session's tmux pane was still alive.
SPEC §7's self-heal (`internal/service.reconcile`'s
`repairTerminalRowWithLivePane`) treats a bare `error`/`stopped` row paired
with a live, non-dead pane as an invariant violation and repairs it straight
back to `running`/`starting` on the very next reconcile tick (task 701/703,
review finding 1). The six sites below were passing only because every
scenario's order assertion ran and settled before that repair tick had a
chance to fire — a latent race, not a genuine pass.

Sites (`grep -n 'has status "error"\|has status "stopped"' features/sort_order.feature`
before this change):

```
35:    And the state database session "ord-bravo" has status "error" 40 seconds ago   (attention scenario)
63:    And the state database session "ord-bravo" has status "error" 40 seconds ago   (created scenario)
91:    And the state database session "ord-bravo" has status "error" 40 seconds ago   (activity scenario)
119:   And the state database session "ord-bravo" has status "error" 40 seconds ago   (name scenario)
149:   And the state database session "ord-bravo" has status "error" 40 seconds ago   (live-apply scenario)
223:   And the state database session "gso-ab" has status "error" 30 seconds ago      (group_by_workspace scenario)
```

All six are now genuine: `grep -n 'has status "error"\|has status "stopped"' features/sort_order.feature`
returns nothing.

## Reproducing the red (pre-change, unmodified route)

The race cannot be observed by merely running the suite (the assertion
always outruns the repair). To demonstrate it deterministically, a
throwaway `git worktree` was created at the pre-change commit (`bedb65a`),
and the attention scenario's own `Then` block was given ONE extra existing
step — `after one configured reconcile interval deck client "A" screen
still contains "waiting"` — which forces a full reconcile-interval wait
(the repair's own trigger cadence) before the order table is checked. No
other scenario or step was touched; the trial edit lived only in the
worktree and was discarded (`git worktree remove --force` +
`git worktree prune`) the moment the log was captured — `git status
--porcelain` in `/workspace` was empty throughout.

Command (from the worktree, workspace root mounted, per `ci/SPIKE.md`):

```
ci/run.sh sh -c 'cd .scratch-804 && env DECK_GODOG_PATHS=sort_order.feature go test ./features/ -run TestFeatures -count=1'
```

Result: exit 1 (`pre-change-red.log`, `pre-change-red.exitstatus`), quoting
exactly the race predicted:

```
deck client renders session "ord-alpha" at line 5, want strictly after session "ord-bravo" at line 7
```

This is the repair having already promoted `ord-bravo` from `error` to
`running` (source `tmux`) before the assertion ran, which reorders it past
`ord-alpha` in the attention tier (both now rank `running`; `ord-alpha`'s
`StatusAt` is older, so it sorts first) — the exact "latent" failure task
801/802/803's notes named but did not reproduce for this file.

## The fix

Every "error" line is replaced with a genuine nonzero pane exit
(`shell session "X" exits with status 1`, `features/crash_test.go`'s
existing `shellSessionExitsWithNonzeroStatus`, already used this way by task
703 in `attention_sort.feature`/`status_theme.feature`/`themes.feature`).
tmux's `remain-on-exit failed` retains the dead pane; reconcile's
crash-collection path (not the live-pane repair path) observes it, writes a
real `tmux`-sourced `error` with `pane_exit_status` set, and kills the tmux
session — durably out of the repair's reach, since there is no longer a
live pane to trigger it. Each site adds
`within one configured reconcile interval deck client "A" row "X" contains
"error"` right after the existing `screen contains "waiting"` wait, so the
crash has genuinely settled before the order table (or, for
group_by_workspace, the group-leadership assertions) is read.

The `activity` scenario (line 91, now spanning more lines) sorts purely by
`StatusAt` descending, so `ord-bravo`'s crash — whose `StatusAt` otherwise
lands at whatever real wall-clock moment the crash-collecting reconcile tick
actually ran — needed to be pinned back to the fixture's engineered age
(40 seconds ago) without perturbing `status`/`status_source`. A new step,
`the state database session "X" has status_at N seconds ago`
(`setSessionStatusAtSecondsAgo`, `features/sort_order_test.go`, mirroring
`setSessionCreatedAtSecondsAgo`'s own reasoning), does exactly that raw,
single-column write; by the time it runs the row is already terminal with
no live pane (crash-collection already killed it), so this write can never
race the live-pane repair the way the original raw status write did. No
other scenario in this file needed it: `attention`/`created`/`name` only
ever look at the `error` *tier*, and `group_by_workspace`'s group-leadership
rule (`internal/tui/group.go`) is also tier-based, not `StatusAt`-based.

No `screen shows sessions in this order` table or R53 selection-preservation
assertion (`has session "X" selected`) changed a single line — confirmed by
`git diff features/sort_order.feature | grep -E '^[+-].*\|'` (empty) and
`git diff features/sort_order.feature | grep -E '^[+-].*selected'` (empty).
`features/godog_test.go` is untouched; scenario count is unchanged at 6.

## Evidence

- `pre-change-red.log` / `.exitstatus` — the reproduced red, worktree-only
  trial edit, fully reverted before any commit.
- `run1.log` .. `run5.log` and their `.exitstatus` files — five consecutive
  green runs of the fixed file, each via:
  ```
  ci/run.sh env DECK_GODOG_PATHS=sort_order.feature go test ./features/ -run TestFeatures -count=1
  ```
  All five: exit 0, `ok github.com/n-orlov/deck/features`.

## Scope

Touches only `features/sort_order.feature` and `features/sort_order_test.go`
(one new step). No product code, no other feature file, no `defaultTags`,
no `t.Skip`. F2/F20/F22/F31 are unrelated to this file and were not
encountered.
