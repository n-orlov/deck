# Task 204 — `ci/stability.sh 10` at the final code sha

This task owns its whole iteration and starts no other work.

## What was run, and how

Execution shape (background, no pipe on the exit-status-bearing command):

```
nohup sh -c 'timeout 6000 ci/stability.sh 10 > /tmp/stability204.log 2>&1; echo $? > /tmp/stability204.log.exitstatus' >/dev/null 2>&1 &
```

polled with `sleep 180` (the last few polls used longer sleeps as the run
approached completion; the polling cadence never affected the measurement,
only how often this iteration checked on it). Measured wall time: started
`2026-08-30T14:55:05Z`, `run-10` finished `2026-08-30T15:56Z` per its own
`ls -la` mtime — **~61 minutes**, inside the ~60-70 min estimate in the
standing rules.

## Sha this ran at

Final code sha (`git log -1 --format=%H -- '*.go' '*.feature'`) **before and
after** this run: `4b1d4dcbd4480013470a0555795e6c64db3bf96d` (`4b1d4dc`,
task 201's own sha). Docs-only-descendant guard, empty as required:

```
$ git diff --stat 4b1d4dc..HEAD -- '*.go' '*.feature'
(no output)
```

No code changed between task 201's sha and this task's HEAD, so this
measurement is valid at the phase's final code sha.

## Result, published as measured

`summary.log`'s final line, verbatim:

```
10/10 passed
```

Captured exit status (the script's own `$?`, written to `.exitstatus` by the
launcher's `echo $? > <out>.exitstatus`, never through a pipe): **`0`**.

All ten `run-N.log` files are the unedited `go test -p=1 -count=1 ./...`
output for that run (17 lines each: package-level `ok`/`?` lines only, since
`ci/stability.sh` invokes the non-verbose form of that command, not the
verbose companion). `summary.log` is the script's own concatenation of all
ten run logs plus its `=== RUN N ===` bracketing lines and the closing
outdir/tally lines, copied byte-for-byte from the script's own `mktemp -d`
output directory (an ephemeral scratch path outside the repo, not cited here since it is not
a tracked path).

10/10 is what happened: no run failed, so 10/10 is published as 10/10 —
nothing to round, nothing re-run.

## Out-of-scope findings: recurrence check

Each run log was checked (`grep -a -i FAIL run-N.log` across all ten — zero
hits in every file) since `ci/stability.sh` labels a run FAIL only from
`go test`'s own non-zero exit, and any of the flakes below manifesting would
show up as a `FAIL` line in that run's package output. One line per finding,
per the task's required checklist:

- **F2** (golden-frame settle): no recurrence in any of the 10 runs — no hit.
- **F20** (status_recovery dup-pane): no recurrence in any of the 10 runs — no hit.
- **F22** (`ByteArrivalPattern`): no recurrence in any of the 10 runs — no hit.
- **F37** (sort_order latent race): no recurrence in any of the 10 runs — no hit.
- **`filter.feature` dd/undo race**: no recurrence in any of the 10 runs — no hit.

None of these is claimed fixed by this task; they remain out of scope and
are reported here only as "did or did not recur in this measurement", per
the standing rules. This run recorded no hit for any of them, so there is no
per-run log path to cite for a recurrence.

## Provenance checks

```
$ git rev-parse HEAD
011b04bc655bf843b1f5ac1434e51106152cbf41
$ git log -1 --format=%H -- '*.go' '*.feature'
4b1d4dcbd4480013470a0555795e6c64db3bf96d
```

Files in this report directory: `summary.log`, `run-1.log` .. `run-10.log`,
`.exitstatus` (holds `0`), this `README.md`.
