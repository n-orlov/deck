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

## Addendum: re-measurement after validation attempt 1

This section, and the logs under `reverify/`, were landed by a second commit
carrying the same task id (task 001) immediately after the revert commit
`f700025`; a report cannot name its own sha, and published history is never
amended here. The revert commit itself is unchanged — `f700025` remains the
only commit of this task that touches any non-`docs/reports/` path.

Validation attempt 1 recorded two objections. Both are answered here with
measurements rather than with edits to the scenario.

### 1. The targeted run's exit status, re-measured four times

Attempt 1 reported that rerunning the command above exited 0 (with a verbose
companion showing 2 scenarios / 35 steps passed), contradicting the exit 1
recorded above. Re-measured at this same tree, four times, each exit status
captured by `echo $? > ...` in the same shell call that ran the command:

| log | command | exit status |
| --- | --- | --- |
| `reverify/run-1.log` | `ci/run.sh env DECK_GODOG_PATHS=status_probe.feature go test ./features/ -run TestFeatures -count=1` | `reverify/run-1.exitstatus` = `1` |
| `reverify/run-2.log` | same | `reverify/run-2.exitstatus` = `1` |
| `reverify/run-3.log` | same | `reverify/run-3.exitstatus` = `1` |
| `reverify/run-4-plain.log` | `ci/run.sh env DECK_GODOG_PATHS=status_probe.feature go test ./features/ -count=1` (no `-run`) | `reverify/run-4-plain.exitstatus` = `1` |

All four fail at the same step, with the same text (quoted verbatim from
`reverify/run-1.log`, control characters stripped only for line wrapping):

```
step error: session "sampled pi" verdict = "starting"/"tmux" reason "tmux pane
is alive; terminal row corrected", want "error"/"probe" reason "agent error"
(err=<nil>)
    godog_test.go:39: godog feature suite failed
FAIL
FAIL	github.com/n-orlov/deck/features	8.014s
FAIL
```

In `reverify/run-4-plain.log` the failing test is named twice as
`--- FAIL: TestFeatures`, and the `want "error"/"probe" reason "agent error"`
text occurs 6 times; the `1 scenarios (1 failed)` tally at that log's tail
belongs to the unrelated meta-test `TestGodogRejectsUndefinedAndFailedSteps`,
which deliberately runs a failing fixture feature in a temp dir and passes.

The exit-1 record above therefore stands, and is not narrowed by `-run`: the
plain form the task's own criteria name (`run-4-plain`) is red too.

**Recorded as a finding, not as a fix:** attempt 1's exit-0 observation is not
reproducible here (0 of 4), but it is credible for this scenario, because the
assertion is a race the report should not have described as deterministic. The
restored step polls the state database for the probe-sourced `error` verdict
while `repairTerminalRowWithLivePane` (still widened, until task 003 lands) is
free to overwrite that row with `starting`/`tmux` on the next reconcile tick.
If a poll observes the row before the repair tick, the step passes and the
suite exits 0. So this scenario is expected-red-but-timing-dependent between
tasks 001 and 003, and an exit 0 here proves nothing about the ruling. This
is carried into phase 3h's findings file (task 014), and the standing
expectation is unchanged: the only run that settles the scenario is the one
task 003 makes green.

### 2. The protected-path check's one line is `a24ff8d`, not this task's work

Attempt 1 also reported that
`git log --oneline de90a5c..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md`
prints one line instead of nothing. It does, and it will for every task in
this phase:

```
$ git log --oneline de90a5c..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
a24ff8d plan: add Phase 3h — bring the suite to the §7 ruling, fix F31, hold the gate at one sha (operator)

$ git show --stat --oneline a24ff8d
a24ff8d plan: add Phase 3h — bring the suite to the §7 ruling, fix F31, hold the gate at one sha (operator)
 prds/phase3h-suite-reconciliation.md | 152 +++++++++++++++++++++++++++++++++++
 1 file changed, 152 insertions(+)
```

`a24ff8d` is de90a5c's immediate child and this phase's own plan commit: it
adds `prds/phase3h-suite-reconciliation.md`, a protected path, and it was
already on `main` (and on `origin/main`) before task 001 started. It is not
any task's commit. Making that check print nothing would require rewriting or
dropping published history, which this run's standing rules forbid without
exception. The check that is actually meaningful — and that holds — is that no
commit *this run writes* touches a protected path:

```
$ git log --oneline a24ff8d..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
$ git diff a24ff8d..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
```

both print nothing (verified for `f700025` and for this addendum commit; see
the commit message of this commit for the same pair re-run at its own sha).
