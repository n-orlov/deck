# Task 001 — forward-revert a53146a (status_probe's stale-sampling scenario)

## Ruling this restores against

de90a5c ("spec: resolve §7's error-under-live-pane self-contradiction and land
§11.8's R93 wording") settled that SPEC.md §7's transition table is the
design: a hook- or probe-sourced `error` row with no `pane_exit_status` is
that table's own live state (`running → error` on a turn or API failure) and
is **never** repaired, even with the pane alive throughout. Only a `stopped`
row, or an `error` row carrying a `pane_exit_status` or a `tmux`/`user`
source, is the invariant violation `repairTerminalRowWithLivePane` may
correct (SPEC.md:561-578).

a53146a (task 1202) re-pointed this scenario's "sampled pi" assertions onto
`0b7dce5`'s widened repair — which had (wrongly, per de90a5c) started
repairing bare probe-sourced `error` rows too — instead of treating the
widening as the defect it turned out to be. This commit forward-reverts
a53146a, restoring the scenario's original assertions:

```
And the state database session "sampled pi" has probe status "error" with reason "agent error"
And within one configured reconcile interval deck client "A" row "sampled pi" contains "sampled"
```

and dropping the `databaseSessionStatusSourceReason` step helper a53146a
added (unused once its only caller reverts).

## Command run and what it reported

```
ci/run.sh env DECK_GODOG_PATHS=status_probe.feature go test ./features/ -run TestFeatures -count=1
```

Exit status: **1** (FAIL), as expected — task 003 (the narrow revert of
0b7dce5's widening in `internal/service/reconcile.go`) has not landed yet, so
`repairTerminalRowWithLivePane` still repairs the bare probe-sourced `error`
row this scenario now asserts against. The failing step:

```
step error: session "sampled pi" verdict = "starting"/"tmux" reason "tmux pane
is alive; terminal row corrected", want "error"/"probe" reason "agent error"
(err=<nil>)
```

This is the mirror image of a53146a's own "pre-change red step" note — this
scenario is expected red between task 001 and task 003, per the plan's
standing note that the tree stays red for the three R94 scenarios until then.
No green run is claimed here. Task 003 must land, and this feature re-run
green, before any completion claim for this scenario.
