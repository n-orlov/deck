# Task 1003 — `ci/stability.sh 10` at the approach's final code sha

## Code sha measured

`a5f8f6b` (task 1001, the last commit in this approach that touches `*.go`,
`*.feature`, `*.sh`, `*.toml`, `go.mod` or `go.sum`).

This run was launched from HEAD `838fa74` (task 1002's commit, doc-only), and
the empty diff below proves HEAD carries the exact same code state as
`a5f8f6b`:

```
$ git diff --stat a5f8f6b..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum
<no output>
```

(command run from `/workspace`, exit status 0, output empty)

## Launch (exactly once, bounded, backgrounded)

```
nohup sh -c 'timeout 7200 ci/stability.sh 10 > /run/ralphd/artifacts/stability10.log 2>&1; printf "%s\n" "$?" > /run/ralphd/artifacts/stability10.exitstatus' &
```

Launched 2026-08-30 02:54:35 UTC. Polled with
`sleep 600; grep -c '^=== RUN' /run/ralphd/artifacts/stability10.log` seven
times at roughly 10, 20, 30, 40, 50, 60 and 70 minutes elapsed, never blocked
on; the run itself had already finished by the 70-minute poll (`run-10.log`
and `summary.log` both mtime-stamped 03:55:55, i.e. 61m20s after launch).
Total wall time ~61 minutes — faster than the measured shape's ~80-95 min
band, consistent with a quiet host during this run.

## Result

The script's own summary line, quoted verbatim from `summary.log`:

```
10/10 passed
```

`script.exitstatus` (captured in the same shell call as the run itself, never
inferred): `0`.

All 10 runs passed; there is no failing run to name.

## Files in this directory

- `summary.log` — the script's own combined summary (all 20 `=== RUN N ===`
  / `=== RUN N: PASS ===` markers plus the per-run `go test` output and the
  final tally line).
- `script.exitstatus` — `0`, captured in the same shell call that ran
  `ci/stability.sh 10`.
- `run-1.log` … `run-10.log` — each run's own `ci/run.sh go test -p=1
  -count=1 ./...` output (17 packages: 14 `ok`, 3 `[no test files]` —
  `internal/notify`, `internal/search`, `internal/unit` — matching task
  1002's whole-suite sweep).

## Verification

```
$ git ls-files docs/reports/phase3g-1003-stability10/
docs/reports/phase3g-1003-stability10/README.md
docs/reports/phase3g-1003-stability10/run-1.log
docs/reports/phase3g-1003-stability10/run-10.log
docs/reports/phase3g-1003-stability10/run-2.log
docs/reports/phase3g-1003-stability10/run-3.log
docs/reports/phase3g-1003-stability10/run-4.log
docs/reports/phase3g-1003-stability10/run-5.log
docs/reports/phase3g-1003-stability10/run-6.log
docs/reports/phase3g-1003-stability10/run-7.log
docs/reports/phase3g-1003-stability10/run-8.log
docs/reports/phase3g-1003-stability10/run-9.log
docs/reports/phase3g-1003-stability10/script.exitstatus
docs/reports/phase3g-1003-stability10/summary.log
```

The gate is green: `ci/stability.sh 10` is 10/10 at `a5f8f6b`, the run's
final code sha. This run is not repeated to improve the number — it was
already 10/10.
