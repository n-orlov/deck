# Phase 3i — task 405: ten-run stability gate

## What this is

`ci/stability.sh 10` run once, in the background, at the frozen final code sha,
per the standing rules' stability-gate procedure (bounded `timeout`, polled with
`sleep 180`, exit status read from a `.exitstatus` file rather than through a
`tee` pipe).

## Sha the gate ran at

Code is frozen as of task 402's commit
`3b70bfbc7e3552ff375ae675af117805a1eee944` (`3b70bfb`): `git log --oneline
3b70bfb..HEAD -- '*.go' '*.feature'` is empty at the commit this gate ran at
and remains empty now (checked immediately before and after the run).

This gate itself ran with the repository at commit `a6b382d4678dbbbed672fa29547a2a614a8cc268`
(`a6b382d`, task 404's docs publish, the HEAD in place when this task started
and unchanged throughout — `HEAD` and `origin/main` both resolve to it before
and after the run). Every file the gate's `go test -p=1 -count=1 ./...` reads
(`*.go`, `*.feature`) is byte-identical to `3b70bfb`'s copy, so the ten runs
below exercise exactly the frozen final code.

## Launch and poll record

```
nohup sh -c 'timeout 6000 ci/stability.sh 10 > /tmp/stability-405.log 2>&1; echo $? > /tmp/stability-405.log.exitstatus' >/dev/null 2>&1 &
```

Launched 2026-09-02T20:39:06Z. Polled with `sleep 180` (no other duration) until
the exit-status file appeared — **21 polls** — at 2026-09-02T21:43:02Z (last
poll observed the file already present, timestamped 21:41). Elapsed ≈ 64
minutes, consistent with the standing rules' ~67-minute measurement.

## Captured exit status

```
$ cat /tmp/stability-405.log.exitstatus
0
```

The script's own overall exit status is **0** (all runs passed; the script
exits 1 only if any run's `go test` exit status was non-zero — see
`ci/stability.sh`'s tail).

## Per-run PASS/FAIL

| Run | Result |
|-----|--------|
| 1   | PASS (exit 0) |
| 2   | PASS (exit 0) |
| 3   | PASS (exit 0) |
| 4   | PASS (exit 0) |
| 5   | PASS (exit 0) |
| 6   | PASS (exit 0) |
| 7   | PASS (exit 0) |
| 8   | PASS (exit 0) |
| 9   | PASS (exit 0) |
| 10  | PASS (exit 0) |

**10/10 passed.** No FAIL runs occurred, so there is no per-run log path to
cite for a failure. Per-run logs for all ten runs (including the passing ones)
were kept by the script in its own scratch output directory
(`/tmp/deck-stability.*`, listed in `summary.log`'s closing line below) but are
not committed here — only the script's own `summary.log` is published,
verbatim, as required.

Published exactly as measured: 10/10, never rounded up (there was nothing to
round) and not re-run to improve the number.

## `summary.log` verbatim, with `cmp`

`summary.log` in this directory is the stability script's own summary log,
copied byte-for-byte from the run's scratch output directory:

```
$ cmp /tmp/deck-stability.yes4OG/summary.log docs/reports/phase3i-405-stability10/summary.log
$ echo $?
0
```

(`cmp` printed nothing and exited 0 — byte-identical.) `summary.log`'s own
closing lines:

```
full per-run logs and combined summary log kept in: /tmp/deck-stability.yes4OG
10/10 passed
```

## Superseded gates

`docs/reports/phase3i-206-stability10/` (old final code sha
`a559e7c61a00ab5fa31c2d98eaf5ce787744e4dc`) is stale — superseded by this
directory, not a discharge of this task.
