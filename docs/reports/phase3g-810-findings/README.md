# Task 810 — the standalone F37 reproducer

[`../phase3g-findings.md`](../phase3g-findings.md)'s finding **F37** records the
red this approach's own baseline (task 804) found in
`features/sort_order.feature`. Its first filing printed a reproduction command
that only worked after the reader made an uncommitted feature-file edit by hand,
so the command was not standalone: run verbatim against the pre-fix commit
`bedb65a` the file passes, because the order assertion outruns the reconcile
tick that fires the SPEC §7 live-pane repair.

This directory makes the reproduction mechanical. Everything it needs is tracked
in the repository:

| file | what it is |
| --- | --- |
| [`reproduce-f37.sh`](reproduce-f37.sh) | the whole reproduction, no arguments, no manual editing: `sh docs/reports/phase3g-810-findings/reproduce-f37.sh` from the repository root |
| [`f37-force-repair-race.patch`](f37-force-repair-race.patch) | the one-line forcing patch the script applies, so the forcing edit is reviewable instead of described |
| [`f37-repro.log`](f37-repro.log) / [`f37-repro.exitstatus`](f37-repro.exitstatus) | that script's own unedited output and captured exit status, produced by task 810 |

## What the script does

1. `git worktree add --detach .scratch-810-f37-repro bedb65a` — the pre-fix
   commit, in a throwaway worktree **inside** the workspace, because `ci/run.sh`
   mounts only the workspace into its sibling container.
2. `git -C .scratch-810-f37-repro apply f37-force-repair-race.patch` — inserts
   one **already-existing** step, `after one configured reconcile interval deck
   client "A" screen still contains "waiting"`, ahead of the attention
   scenario's order table. No assertion, no table row, no other scenario and no
   step definition changes. Forcing the reconcile tick to land before the read
   is what makes the latent race observable at all; without it the defect is
   invisible, which is exactly what F37 records.
3. `ci/run.sh sh -c 'cd .scratch-810-f37-repro && env
   DECK_GODOG_PATHS=sort_order.feature go test ./features/ -run TestFeatures
   -count=1'`.
4. Removes the worktree on every exit path (`trap ... EXIT INT TERM`) and exits
   with the test's own status.

The workspace tree is never mutated: the patch is applied only inside the
throwaway worktree, and `git status --porcelain` in `/workspace` was empty
before and after the capture below.

## Captured result

```
sh docs/reports/phase3g-810-findings/reproduce-f37.sh
```

→ exit **1** (`f37-repro.exitstatus`), with the failure F37 quotes, at
`f37-repro.log:45` (and again in godog's summary at `:313` and in the `go test`
tail at `:427`):

```
after scenario hook failed: deck client renders session "ord-alpha" at line 5,
want strictly after session "ord-bravo" at line 7
```

and the frame in the same log showing why — `ord-bravo` rendered as `running`,
not `error`, because the repair promoted it before the order table was read.
The trailing goroutine dump in the log is the scenario's own cleanup of the
client it had to kill after the failed assertion, not a second defect.

The fix (task 804, `46dad5e`) and its five green runs are recorded in
[`../phase3g-804-sort-order-error-route/README.md`](../phase3g-804-sort-order-error-route/README.md);
this directory adds only the reproduction path for the red.
