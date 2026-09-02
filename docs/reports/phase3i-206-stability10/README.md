# Phase 3i task 206 — `ci/stability.sh 10` at the final code sha

## Final code sha

The ten runs in this directory were made at final code sha
(`git log -1 --format=%H -- '*.go' '*.feature'`):

```
a559e7c61a00ab5fa31c2d98eaf5ce787744e4dc
```

`HEAD` and `origin/main` were both `4b44b27c0456d307ff99468792802b4cc5617f27`
(task 204's docs-only commit) before the run was launched and remained so
after it completed — no code commit landed during the run, so this gate
validates exactly the code state of `a559e7c`, the same sha task 204's
whole-suite sweep already covers.

**Note on a stale artifact discarded before this run**: `/tmp/deck-stability.SH2jgK`
(also `10/10 passed`, exit 0) was found in the container's `/tmp` at the start
of this task, but it belongs to **approach 1's task 129**
(`docs/reports/phase3i-129-stability10/`), run at sha `b9243a1f415ba9ca77cc2ffa2ec557ca8b1be4cb`
— an older, pre-201/202/203 code state. It was NOT reused for this report; the
run below is a fresh invocation made after confirming the tree was clean at
`4b44b27`/`a559e7c`.

## Command shape (backgrounded, per standing rules)

```
nohup sh -c 'timeout 5400 ci/stability.sh 10 > /tmp/deck-stability-206/out.log 2>&1; echo $? > /tmp/deck-stability-206/out.log.exitstatus' >/dev/null 2>&1 &
```

launched 15:41:06 UTC, polled in `sleep 180` loops against
`/tmp/deck-stability-206/out.log.exitstatus`, never piped through `tee` for
the exit-status-bearing command — `ci/stability.sh` itself follows the same
no-`tee`-on-the-status-line discipline internally (see its own header
comment), and this wrapper preserves that: the `echo $? >` runs immediately
after the `timeout ci/stability.sh 10` command line, before anything else
touches the file.

Actual wall-clock: launched 15:41:06 UTC, completed 16:48:xx UTC —
**~67 minutes** for 10 runs (close to the ~66 min estimate in the standing
rules; each individual run averaged ~6.5-7.5 min, consistent with docker
sibling container startup/teardown overhead per run, plus the `features`
package's ~330s Godog suite dominating each run).

## Script's own captured exit status

From `echo $? > /tmp/deck-stability-206/out.log.exitstatus` immediately after
the `timeout 5400 ci/stability.sh 10` invocation:

```
0
```

(copied verbatim into `summary.log.exitstatus` in this directory.)

## Result

**`summary.log`** in this directory is the verbatim `summary.log` produced by
`ci/stability.sh` (copied byte-for-byte from the script's own `$outdir`,
`/tmp/deck-stability.vpBbUF/summary.log` — not reconstructed from the polling
transcript above).

Final tally line, quoted verbatim from `summary.log`:

```
10/10 passed
```

No rounding: this is the actual measured count. All 10 runs report
`PASS (exit 0)`; none failed, so there is no failure to name or per-run log
to copy in (the "every failing run named by number and log path" clause does
not apply here — there were none).

## Per-run breakdown (from `summary.log`) and log paths

Each run's full per-run log lives at `/tmp/deck-stability.vpBbUF/run-<N>.log`
(container-local `/tmp`, not copied byte-for-byte into this directory beyond
the combined `summary.log` above, which already contains each run's full
`go test` output verbatim).

| Run | Result | Per-run log path |
|-----|--------|-------------------|
| 1   | PASS (exit 0) | `/tmp/deck-stability.vpBbUF/run-1.log` |
| 2   | PASS (exit 0) | `/tmp/deck-stability.vpBbUF/run-2.log` |
| 3   | PASS (exit 0) | `/tmp/deck-stability.vpBbUF/run-3.log` |
| 4   | PASS (exit 0) | `/tmp/deck-stability.vpBbUF/run-4.log` |
| 5   | PASS (exit 0) | `/tmp/deck-stability.vpBbUF/run-5.log` |
| 6   | PASS (exit 0) | `/tmp/deck-stability.vpBbUF/run-6.log` |
| 7   | PASS (exit 0) | `/tmp/deck-stability.vpBbUF/run-7.log` |
| 8   | PASS (exit 0) | `/tmp/deck-stability.vpBbUF/run-8.log` |
| 9   | PASS (exit 0) | `/tmp/deck-stability.vpBbUF/run-9.log` |
| 10  | PASS (exit 0) | `/tmp/deck-stability.vpBbUF/run-10.log` |

## Carried-forward out-of-scope findings: recurrence check

The seven findings carried forward from earlier phases (never claimed fixed
by this phase) are:

1. F2 — golden-frame settle
2. F20 — `status_recovery` dup-pane
3. F22 — `ByteArrivalPattern`
4. F37 — `sort_order` latent race
5. F7 — quantisation collisions
6. the `filter.feature` dd/undo race
7. the OSC 52 clipboard question

`summary.log` (all ten runs' full `go test -p=1 -count=1 ./...` output,
including the `features` package's Godog scenarios) was searched for any
signal tied to these findings — a `FAIL` line, a `panic`, a `race detected`
report, or any of the finding identifiers/keywords themselves
(`golden.frame`, `status_recovery`, `ByteArrivalPattern`, `sort_order`,
`filter.feature`, `osc.?52`, `clipboard`):

```
$ grep -ilE 'FAIL|panic|race detected' run-*.log
(no matches)
$ grep -iE 'F2\b|F20\b|F22\b|F37\b|F7\b|golden.frame|status_recovery|ByteArrivalPattern|sort_order|filter\.feature|osc.?52|clipboard' run-*.log
(no matches)
```

**None of the seven recurred in any of the ten runs.** Every package line in
every run reads `ok` or `[no test files]`; no failure, panic, or race
condition surfaced in any of the ten repetitions. There is no per-finding
log path to cite because there is no recurrence to cite it for.

This is not a claim that any of the seven findings is fixed — they remain
out-of-scope, latent, and carried forward per the standing rules; this gate
simply did not trip over any of them in this particular set of ten runs.
