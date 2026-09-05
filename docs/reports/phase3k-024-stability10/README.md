# Task 024: ten-run stability gate at the final code sha

## Command launched (exactly once, from a clean tree)

```
nohup sh -c 'timeout 7200 ci/stability.sh 10 > docs/reports/phase3k-024-stability10/summary.log 2>&1; echo $? > docs/reports/phase3k-024-stability10/summary.log.exitstatus' >/dev/null 2>&1 &
```

Launched disowned/backgrounded, then polled with `sleep 120` only (never
blocked on), per the standing rule and this task's own success criteria.
`git status --porcelain` was empty immediately before launch.

## Poll table (as observed this iteration)

| Poll | Interval slept | Sweep state observed |
|------|-----------------|-----------------------|
| 1    | `sleep 5`  | `=== RUN 1 ===` just started |
| 2    | `sleep 120` | still run 1 |
| 3    | `sleep 120` | still run 1 |
| 4    | `sleep 120` | still run 1 |
| 5    | `sleep 120` | run 1 PASS, run 2 started |
| 6    | `sleep 120` | still run 2 |
| 7    | `sleep 120` | still run 2 |
| 8    | `sleep 120` | still run 2 |
| 9    | `sleep 120` | run 2 PASS, run 3 started |
| 10   | `sleep 120` | still run 3 |
| 11   | `sleep 120` | still run 3 |
| 12   | `sleep 120` | run 3 PASS, run 4 started |
| 13   | `sleep 120` | still run 4 |
| 14   | `sleep 120` | still run 4 |
| 15   | `sleep 120` | run 4 PASS, run 5 started |
| 16   | `sleep 120` | still run 5 |
| 17   | `sleep 120` | still run 5 |
| 18   | `sleep 120` | run 5 PASS, run 6 started |
| 19   | `sleep 120` | still run 6 |
| 20   | `sleep 120` | still run 6 |
| 21   | `sleep 120` | still run 6 |
| 22   | `sleep 120` | run 6 PASS, run 7 started |
| 23   | `sleep 120` | still run 7 |
| 24   | `sleep 120` | still run 7 |
| 25   | `sleep 120` | run 7 PASS, run 8 started |
| 26   | `sleep 120` | still run 8 |
| 27   | `sleep 120` | still run 8 |
| 28   | `sleep 120` | run 8 PASS, run 9 started |
| 29   | `sleep 120` | still run 9 |
| 30   | `sleep 120` | still run 9 |
| 31   | `sleep 120` | run 9 PASS, run 10 started |
| 32   | `sleep 120` | still run 10 |
| 33   | `sleep 120` | still run 10 |
| 34   | `sleep 120` | still run 10 |
| 35   | `sleep 120` | run 10 PASS; `10/10 passed` printed; `summary.log.exitstatus` present |

Total elapsed until completion: roughly 71 minutes of polling, consistent
with (slightly above) the ~66.5-minute figure measured in phase 3j.

## Final code sha

```
$ git log -1 --format=%H -- '*.go' '*.feature'
d88c6625c4ccca71b0d31f7b5864ba030ed39e53
```

The gate ran against a tree whose HEAD (`66f04fec5f03e57abc5c7a3c8112698195f83208`
at launch time) is a docs-only descendant of that same final code sha — no
`*.go` or `*.feature` file has changed since `d88c662`.

## Result — quoted from `summary.log`

```
=== RUN 1 ===
=== RUN 1: PASS (exit 0) ===
=== RUN 2 ===
=== RUN 2: PASS (exit 0) ===
=== RUN 3 ===
=== RUN 3: PASS (exit 0) ===
=== RUN 4 ===
=== RUN 4: PASS (exit 0) ===
=== RUN 5 ===
=== RUN 5: PASS (exit 0) ===
=== RUN 6 ===
=== RUN 6: PASS (exit 0) ===
=== RUN 7 ===
=== RUN 7: PASS (exit 0) ===
=== RUN 8 ===
=== RUN 8: PASS (exit 0) ===
=== RUN 9 ===
=== RUN 9: PASS (exit 0) ===
=== RUN 10 ===
=== RUN 10: PASS (exit 0) ===
full per-run logs and combined summary log kept in: /tmp/deck-stability.5WgkJO
10/10 passed
```

`summary.log.exitstatus` reads `0`.

**Pass count, exactly as `summary.log` states it: 10/10 passed.** No FAIL
lines, no non-zero per-run exit code.

The `/tmp/deck-stability.5WgkJO` per-run logs referenced by the script's own
final line are ephemeral scratch state inside this run's container (per
`ci/stability.sh`'s own `mktemp -d` design) and are not copied here; the
combined `summary.log` committed in this directory is the complete,
authoritative record of every run's PASS/FAIL decision and is exactly what
the gate command produced.
