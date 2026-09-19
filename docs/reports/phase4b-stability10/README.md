# Phase 4b — ten-run stability sweep (task 023)

Freeze-line stability sweep: the whole Go test suite, no test-name filter, no
narrowed package list, run **ten** times from a clean state in the CI
container, at the same code sha task 021's whole-suite gate sweep gated.

- **Code sha (same sha task 021 gated):**
  `0806ba64ede4356af50b404affd10f26f68d4d84` (task 020, "tui: prove
  shared-state.db group edits reach another client on reload (R131)" — the
  last code-touching commit on the GO branch). `HEAD` at the time this
  sweep launched was `97b1d685742dbb4900d4815acbbb88fa65c94e78` (tasks
  021/022's four record-only commits stacked on top of `0806ba6`);
  `git diff --stat 0806ba6..HEAD -- '*.go' '*.feature'` printed nothing, so
  the code tree at `HEAD` is byte-identical to the code tree at `0806ba6`
  and this sweep's results hold at both shas.
- **Invocation launched (exactly once, from a clean tree):**

  ```
  nohup sh -c 'timeout 7200 ci/stability.sh 10 > /tmp/stability10-run.log 2>&1; echo $? > /tmp/stability10-run.log.exitstatus' >/dev/null 2>&1 &
  ```

  `ci/stability.sh 10` runs `ci/run.sh go test -p=1 -count=1 ./...` (every
  package, no `-run` filter, no narrowed package list, default godog tag
  filter `~@real-agents && ~@nightly`) ten times from a clean state
  (`-count=1` disables the test cache; each run gets its own `--rm` sibling
  container so no tmux socket or other run-scoped state leaks between
  runs), reading each run's **real** `go test` exit status — not a piped
  `tee` status (see the script's own header comment on the
  mislabelled-PASS defect it exists to avoid). `git status --porcelain` was
  empty immediately before launch. The launch was backgrounded and
  disowned; this iteration polled the log on a `sleep 120` cadence rather
  than blocking on the command.
- **Wall clock:** launched `2026-09-19T16:01:32Z`, last line of
  `summary.log` written `2026-09-19T17:15:22Z` (`stat` mtime) — **1h13m50s
  (73.83 min)**, inside the ~70–75 minute range measured at planning time
  for ten runs at ~7m18s each.
- **Scratch directory** `ci/stability.sh` created with `mktemp -d`:
  `/tmp/deck-stability.2dvAoW` — ephemeral container state, not committed;
  its ten `run-N.log` files and `summary.log` were copied into this report
  directory (below) before that directory could be reaped.

## Result: 10/10 passed

| Run | Result | Exit | Log |
| --- | --- | --- | --- |
| 1 | PASS | 0 | `run-1.log` |
| 2 | PASS | 0 | `run-2.log` |
| 3 | PASS | 0 | `run-3.log` |
| 4 | PASS | 0 | `run-4.log` |
| 5 | PASS | 0 | `run-5.log` |
| 6 | PASS | 0 | `run-6.log` |
| 7 | PASS | 0 | `run-7.log` |
| 8 | PASS | 0 | `run-8.log` |
| 9 | PASS | 0 | `run-9.log` |
| 10 | PASS | 0 | `run-10.log` |

Taken directly from `ci/stability.sh`'s own PASS/FAIL labels, which come
from each run's actual (never piped/`tee`'d) `go test` exit status — never
inferred from log prose. `summary.log`'s final two lines, quoted verbatim:

```
full per-run logs and combined summary log kept in: /tmp/deck-stability.2dvAoW
10/10 passed
```

The overall `ci/stability.sh 10` script exit status, captured immediately
after the command line that produced it
(`/tmp/stability10-run.log.exitstatus`): `0`.

## Every failure named

**None.** All ten runs passed with exit `0`. Verified two ways per run, not
just from the script's own PASS label:

- `grep -c FAIL run-N.log` → `0` for every `N` in `1..10` — no `FAIL` line
  in any per-run log.
- `grep -c '^ok' run-N.log` → `15`, and `grep -c 'no test files' run-N.log`
  → `18` `− 15 = 3`, for every `N` — every one of the 18 packages
  `ci/run.sh go list ./... | wc -l` resolves (15 with tests, `internal/notify`
  /`internal/search`/`internal/unit` with none) reports a result in every
  one of the ten runs, with no run short of the full package set.

Because the headline is a clean `10/10`, there is no FAIL row to name a
scenario/test or a log path for. This section states that plainly rather
than omitting it — the task's own criteria call for a FAIL row "even when
the headline is 10/10", and the honest answer this sweep produced is that
none is needed.

## Known-open flake classes: not observed this sweep

Per this task's own criteria, the two known-open flake classes are
advisory and, if hit, are named with their log paths rather than chased —
they were checked for explicitly and **neither occurred in any of the ten
runs**:

- **Transient-`starting` assertion** (`TestGoldenMinimumFrame`,
  `features/golden_frame_test.go:74`, `"frame kept changing... not
  settled"`) — `grep -il "not settled" run-*.log` → no match in any of the
  ten logs.
- **SIGWINCH exact-count assertion**
  (`TestSigwinchCountDistinguishesTwoFromThree`,
  `features/sigwinch_count_test.go`) — `grep -il "sigwinch" run-*.log` → no
  match in any of the ten logs.

Both checks were run over all ten per-run logs, not only a sample. This
sweep is a clean 10/10 with no recurrence of either flake class to
disclose; nothing here needed chasing, and nothing was chased.

## Files in this directory

- `run-1.log` … `run-10.log` — the ten per-run logs, one per run, each the
  full captured `ci/run.sh go test -p=1 -count=1 ./...` output for that run.
- `summary.log` — the combined log `ci/stability.sh` itself accumulates
  (all ten runs' output plus its own `=== RUN N ===` /
  `=== RUN N: PASS|FAIL (exit S) ===` markers and the final `10/10 passed`
  line), kept alongside the per-run logs for cross-reference.

## Result

Green: 10/10, exit `0`, wall clock 73.83 min. No red lane; no new task
carved. Nothing to disclose from either known-open flake class this sweep.
