# Phase 4b — ten-run whole-suite stability sweep

## Code sha and invocation

- Code sha: **baf92ed** — the sha task 021's whole-suite gate ran against (see
  `docs/reports/phase4b-final-suite/README.md`). This report was written at HEAD
  `c8e8c5e`, which is docs-only on top of baf92ed: `git diff --stat baf92ed..HEAD -- '*.go'`
  is empty, so the code under test is byte-identical to baf92ed's.
- Invocation, run ten times back-to-back from a clean state each time, via
  `ci/stability.sh 10` (which itself shells out per run to
  `ci/run.sh go test -p=1 -count=1 ./...` in the CI container, capturing the real
  `go test` exit status per run — never a piped/tee'd status):

  ```
  ci/stability.sh 10
  ```

- `-count=1` disables the test cache and `--rm` on the sibling container drops any
  tmux/SQLite state between runs, so each of the ten runs starts clean.

## Headline

**9/10 passed.**

- Total wall clock for the ten runs: **~73 minutes** (started ~20:39:33 UTC,
  finished 21:52:56 UTC 2026-09-19, per the per-run log file timestamps — within
  the ~70–75 minute band expected for ten runs at the baseline ~7m18s/run).
- One run (run 2) hit `exit 1`. The other nine (runs 1, 3–10) exited 0.

## Per-run table

| Run | Result | Exit | Duration (features pkg) | Log |
| --- | ------ | ---- | ------------------------ | --- |
| 1  | PASS | 0 | features 369.946s | [run-01.log](run-01.log) |
| 2  | FAIL | 1 | features 416.885s (TestFeatures 399.57s) | [run-02.log](run-02.log) |
| 3  | PASS | 0 | features 366.434s | [run-03.log](run-03.log) |
| 4  | PASS | 0 | features 365.882s | [run-04.log](run-04.log) |
| 5  | PASS | 0 | features 367.423s | [run-05.log](run-05.log) |
| 6  | PASS | 0 | features 364.345s | [run-06.log](run-06.log) |
| 7  | PASS | 0 | features 366.162s | [run-07.log](run-07.log) |
| 8  | PASS | 0 | features 365.399s | [run-08.log](run-08.log) |
| 9  | PASS | 0 | features 364.560s | [run-09.log](run-09.log) |
| 10 | PASS | 0 | features 366.870s | [run-10.log](run-10.log) |

Combined log of all ten runs (as the script wrote it, in order): [summary.log](summary.log)

## The one FAIL, named

Run 2, `exit 1`:

```
--- FAIL: TestFeatures/settings'_group-delete_d_branch_routes_through_the_same_dd_batch_confirm_and_one_u_restores_the_whole_batch_(R131_part_2) (47.23s)
    suite.go:640: after scenario hook failed: timed out waiting for frame "starting": context deadline exceeded
```

- Scenario: `features/kill_delete_undo.feature:688` — *"settings' group-delete d
  branch routes through the same dd batch confirm and one u restores the whole
  batch (R131 part 2)"*.
- This is the **transient-`starting` assertion** flake class already named as
  open in `docs/reports/phase4b-final-suite/README.md` and carved as
  `021-cure-01` (a different scenario in the same feature file, same wait
  pattern: the client has already progressed past the transient `starting`
  label — the dumped frame shows a live `$` prompt — by the time the harness's
  post-scenario wait checks for it, so the wait times out against a state the
  suite already moved past). It is named here as **advisory**, with its log
  path (`run-02.log`, line ~6434 onward for the `--- FAIL` block), rather than
  chased under this task: the fix for this flake class belongs to `021-cure-01`
  / `021-cure-02` and the resweep at `021-resweep-01`, not to this stability
  sweep.
- No instance of the second known-open flake class (the SIGWINCH exact-count
  assertion in `features/harness.feature:199`'s scenario outline) occurred in
  any of the ten runs. Had one occurred, it would have been named here the same
  way, advisory, with its log path — not chased.
- All eighteen package result lines (14 `ok`, `features` at either `ok` or
  `FAIL`, 3 `[no test files]` for `internal/notify`, `internal/search`,
  `internal/unit`) are present in every one of the ten run logs; no other
  package failed in any run.

## Logs committed

All eleven log files listed in the table above and referenced by filename,
plus this README, live under `docs/reports/phase4b-stability10/`:
`run-01.log` … `run-10.log`, `summary.log`.
