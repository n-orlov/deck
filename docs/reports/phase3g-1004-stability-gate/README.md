# Task 1004 — close the 10/10 stability gate at the final code sha

## Outcome: (a) — the gate is already 10/10, this is the closure record

Task 1003 already measured `ci/stability.sh 10` at the approach's final code
sha and it came back green. This task's job is to check that measurement is
the one to close on and cite it — no fix, no re-run.

## The measurement being closed

- Log path: [`docs/reports/phase3g-1003-stability10/summary.log`](../phase3g-1003-stability10/summary.log)
- Its last line, quoted verbatim: `10/10 passed`
- `docs/reports/phase3g-1003-stability10/script.exitstatus` content, captured
  in the same shell call as the run itself: `0`
- Code sha measured: `a5f8f6b` (task 1001's commit — the last one in this
  approach to touch `*.go`, `*.feature`, `*.sh`, `*.toml`, `go.mod` or
  `go.sum`)

That report's own README already carries the empty-diff proof against the
HEAD it was written at (`838fa74`); re-shown here against *this* task's HEAD:

```
$ git diff --stat a5f8f6b..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum
<no output>
```

(run from `/workspace`, exit status `0`, output empty — confirms no commit
between task 1003's and this one touched code, so the sha task 1003 measured
is still the tree's final code state)

## F2 / F22 — named, not claimed fixed, not observed this measurement

Per this run's standing rules, `TestGoldenMinimumFrame`'s settle flake (F2)
and `internal/interactive`'s `TestSessionRendersAreCoalescedAgainstAKnownByteArrivalPattern`
`ByteArrivalPattern` flake (F22) are out of scope and must never be claimed
fixed; a recurrence is reported with its log path, never rounded into "fixed".

This measurement (task 1003's ten runs) did not surface either one — a grep
of every one of its committed logs for both tests' names finds nothing:

```
$ grep -l "TestGoldenMinimumFrame\|ByteArrivalPattern" \
    docs/reports/phase3g-1003-stability10/run-*.log \
    docs/reports/phase3g-1003-stability10/summary.log
<no output>
```

That is a clean 10/10, not a fix: F2 was last discussed in
[`phase3g-findings.md`§4](../phase3g-findings.md#4-f2--the-golden-frame-settle-flake-no-recurrence-found)
("this is not proof F2 is fixed... only that this phase's own work did not
trip it"), and F22's own recorded recurrence log path is
[`phase3g-030-reclaim-leaked-interactive-pipe/criterion-packages-run1.log:2-3`](../phase3g-030-reclaim-leaked-interactive-pipe/criterion-packages-run1.log).
Neither is touched, re-tested, or claimed resolved by this task; both remain
open per the standing rules and per `phase3g-findings.md`'s own rows for
them.

## Verification

```
$ git ls-files docs/reports/phase3g-1004-stability-gate/
docs/reports/phase3g-1004-stability-gate/README.md
```

The gate is closed on task 1003's measurement: `ci/stability.sh 10` is
`10/10 passed` (`script.exitstatus` `0`) at code sha `a5f8f6b`, which is
still, verifiably, this tree's final code state.
