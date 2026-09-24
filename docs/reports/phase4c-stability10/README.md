# Phase 4c — ten-run stability sweep (cure-01-06)

Sweep ran with `ci/stability.sh 10` at code sha **`da92f63663a711b94ee8b82641e45ff1270df54a`**
(== `main` == `origin/main` at the time of the sweep — the final code sha after the
behavioural cures `cure-01-01`..`cure-01-05` and the whole-suite gate `022` had all
landed; supersedes the stale task-015 sha reference per operator ruling 1).

Each run invokes `ci/run.sh go test -p=1 -count=1 ./...` from a clean state (fresh
per-run container, `-count=1` disables the test cache) and is labelled PASS/FAIL from
`go test`'s own exit status, never a pipe's (see `ci/stability.sh` header comment for
why that distinction matters).

## Result: **10/10 passed**

| Run | Result | Log |
|-----|--------|-----|
| 1 | PASS (exit 0) | [`run-1.log`](./run-1.log) |
| 2 | PASS (exit 0) | [`run-2.log`](./run-2.log) |
| 3 | PASS (exit 0) | [`run-3.log`](./run-3.log) |
| 4 | PASS (exit 0) | [`run-4.log`](./run-4.log) |
| 5 | PASS (exit 0) | [`run-5.log`](./run-5.log) |
| 6 | PASS (exit 0) | [`run-6.log`](./run-6.log) |
| 7 | PASS (exit 0) | [`run-7.log`](./run-7.log) |
| 8 | PASS (exit 0) | [`run-8.log`](./run-8.log) |
| 9 | PASS (exit 0) | [`run-9.log`](./run-9.log) |
| 10 | PASS (exit 0) | [`run-10.log`](./run-10.log) |

No failure occurred in any of the 10 runs (`grep -n '^--- FAIL\|^FAIL'` across all
ten committed logs returns nothing), so neither of the two known advisory flake
classes (transient-`starting`, `SIGWINCH` exact count) recurred in this sweep — there
is nothing to mark advisory this time; both remain advisory in general per the PRD.

Each run reports the same 19 packages (15 `ok`, 4 `[no test files]`:
`internal/notify`, `internal/search`, `internal/unit`, `internal/tui/sidebarcursor`),
consistent with `ci/run.sh go list ./...` at this sha.

Raw combined script output (all ten `=== RUN N ===` markers plus the final
`10/10 passed` summary line) is preserved in the sweep's own tmp working directory
from the run that produced these logs; the per-run logs above are the ones actually
committed as evidence.
