# Task 1202 — status_probe.feature's stale-sampling scenario under the widened repair

## Command run five times

```
ci/run.sh env DECK_GODOG_PATHS=status_probe.feature go test ./features/ -run TestFeatures -count=1
```

Logs `run-1.log` .. `run-5.log` in this directory, each with its own captured
exit-status file `run-N.log.exitstatus`. All five are `0`:

```
$ for i in 1 2 3 4 5; do cat run-$i.log.exitstatus; done
0
0
0
0
0
```

Each log's tail confirms the package passed (`ok github.com/n-orlov/deck/features ...s`).

## Pre-change red step (measured at HEAD `c176751`, before this task's edit)

```
step error: session "sampled pi" verdict = "starting"/"tmux" reason "tmux pane is alive; terminal row corrected", want "error"/"probe" reason "agent error" (err=<nil>)
```

## What broke it and why re-pointing (not deleting) is correct

The "Stale sampling is visible, precedence-aware, and agent-only" scenario's
"sampled pi" leg fires a pi-adapter probe against a fixture (`pi/error.txt`)
that the adapter's rules resolve to `error`. Before task 1101 re-widened
`repairTerminalRowWithLivePane` (SPEC.md:560-566) to every terminal row, that
probe-written `error`/`probe`/"agent error" row stood: the scenario asserted
it directly.

Task 1101's widening changed nothing about the probe write itself — the probe
still runs on the same reconcile pass, still resolves `pi/error.txt` to
`error`, and still records one `probe.error` event — but it changed what the
*next* reconcile pass does with the resulting row. `error` is now, per
SPEC.md:560-566's own broadened wording ("a terminal status ... claims there
is nothing running here, and a live, non-dead pane is direct evidence against
it ... draws no distinction by source"), an invariant violation exactly like a
`stopped` row with a live pane. Because "sampled pi"'s tmux pane is alive and
not dead throughout the scenario, the very next pass calls
`repairTerminalRowWithLivePane`, which resets the row to
`starting`/`tmux`/"tmux pane is alive; terminal row corrected" and clears the
probe's terminal verdict. By the time any client or test observes the
database, the probe-sourced `error` row has already been self-healed away.

This is a genuine **probe/repair oscillation**: a probe-sourced terminal
verdict for an agent whose pane stays alive can never survive past the
reconcile pass immediately following the one that wrote it, under the widened
rule. The write happens (one `probe.error` event is durably recorded — this
scenario now asserts `the probe event count for session "sampled pi" is 1` to
keep that evidence retained rather than dropped), but the *row* it produced is
gone before anything downstream of reconcile can act on it, and no polling
window can reliably observe the transient state without racing the very next
pass (a race features/interactive_scroll.feature's own
`clientRowContainsAcrossSeveralProbeCycles` comment shows the plan already
anticipated for a *different* scenario, on the mistaken assumption there —
predating task 1101's widening — that a "bare probe-sourced error with no
pane-exit verdict" is left alone; the widened rule draws no such exception,
so that comment is now stale too, and is a separate finding, not this task's
to fix). Re-pointing this scenario's assertions onto the deterministic state
the repair actually produces — `starting`/`tmux`/"tmux pane is alive; terminal
row corrected" in the database, and the row containing the literal word
`starting` client-side, in place of the racy `error`/`probe`/"agent error" and
`sampled` — keeps every original assertion (status+source+reason check, one
row-contains check) doing real, stable work instead of asserting a value the
current code path can never leave observable at rest. The database and
row-contains assertions are retained, not deleted or loosened: they now check
what the widened repair guarantees will be true, rather than a transient
window the widening abolished.

The oscillation itself — a probe-sourced terminal verdict for a live-paned
agent is always superseded by SPEC.md:560-566's repair on the very next pass,
so no client can ever durably observe a probe's own terminal verdict for such
a session — is exactly the kind of behavior change task 1101's widening
produces that deserves a findings-report row. It is recorded for task 1208's
findings report (`docs/reports/phase3g-findings.md`) to pick up rather than
silently absorbed into this scenario's fix.
