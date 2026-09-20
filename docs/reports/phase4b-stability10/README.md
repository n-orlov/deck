# Phase 4b — ten-run whole-suite stability sweep

## Why this report was replaced

The previous version of this report pinned itself to **baf92ed** ("the code
under test is byte-identical to baf92ed's") and reported 9/10, the one
failure being the transient-`starting` assertion class later cured by
`021-cure-01` (db9732b). At that point `git diff --stat baf92ed..HEAD --
'*.go' '*.feature'` was already 18 files, +925/-167 (including
`internal/tui/interactive_scroll.go` +163 and `internal/tui/settings.go`
+200) — real product code landed by `021-cure-01`/`021-cure-02`,
`cure-01-01-2` and `cure-01-02-2` after that sweep ran, so the old report no
longer described the tree it claimed to describe, and no stability run of
any kind existed for the shipped tree. This report replaces it with a fresh
ten-run sweep taken at the current shipped sha.

This same artifact also satisfies task 023 ("sweep: ten-run stability with
every failure named"), whose own criteria target this exact file. Verified
fresh at task 023's own launch: `git diff --stat c6a0876..HEAD -- '*.go'
'*.feature'` is empty — no code-touching commit has landed since this sweep
was taken, including through the two docs-only commits (`93f5790`,
`0e1d6e8`) that followed it — so the 10/10 result and every per-run log
below still describe the code sha currently checked out, and no re-run was
needed.

## Code sha and invocation

- Code sha: **c6a0876** (`HEAD` at the time this sweep was run) — includes
  all of `021-cure-01` (db9732b), `021-cure-02` (a224e43), `cure-01-01-2`
  (8d934d1), and `cure-01-02-2` / its follow-up (8f8e9e8, f13c848).
- Invocation, run ten times back-to-back from a clean state each time, via
  `ci/stability.sh 10` (which itself shells out per run to
  `ci/run.sh go test -p=1 -count=1 ./...` in the CI container, capturing the
  real `go test` exit status per run — never a piped/tee'd status):

  ```
  ci/stability.sh 10
  ```

- `-count=1` disables the test cache and `--rm` on the sibling container drops
  any tmux/SQLite state between runs, so each of the ten runs starts clean.

## Headline

**10/10 passed.**

- Total wall clock for the ten runs: **~74 minutes** (started ~00:13:45 UTC,
  finished 01:27:33 UTC 2026-09-20, per the output directory's own creation
  time and the summary log's last line — within the ~70–75 minute band
  expected for ten runs at the baseline ~7m18s/run).
- All ten runs exited 0. No failure occurred in any run.

## Per-run table

| Run | Result | Exit | Duration (features pkg) | Log |
| --- | ------ | ---- | ------------------------ | --- |
| 1  | PASS | 0 | features 369.229s | [run-01.log](run-01.log) |
| 2  | PASS | 0 | features 366.149s | [run-02.log](run-02.log) |
| 3  | PASS | 0 | features 365.882s | [run-03.log](run-03.log) |
| 4  | PASS | 0 | features 366.912s | [run-04.log](run-04.log) |
| 5  | PASS | 0 | features 369.560s | [run-05.log](run-05.log) |
| 6  | PASS | 0 | features 365.672s | [run-06.log](run-06.log) |
| 7  | PASS | 0 | features 368.638s | [run-07.log](run-07.log) |
| 8  | PASS | 0 | features 368.847s | [run-08.log](run-08.log) |
| 9  | PASS | 0 | features 368.972s | [run-09.log](run-09.log) |
| 10 | PASS | 0 | features 368.071s | [run-10.log](run-10.log) |

Combined log of all ten runs (as the script wrote it, in order):
[summary.log](summary.log)

## Failures, named

**None.** All ten runs exited 0 and every one of the eighteen package result
lines (14 `ok`, `features` `ok`, 3 `[no test files]` for `internal/notify`,
`internal/search`, `internal/unit`) is present and green in every one of the
ten run logs — checked with `grep FAIL run-*.log` (no matches in any of the
ten files) and `grep -c '^ok\|^?' run-*.log` (18 in every file). This
headline is published with the same failure-naming discipline the criteria
require even though there is nothing to name: had any run failed, the
failing scenario or test and that run's committed log path would be listed
here, per the pattern used in the superseded baf92ed-era report.

Neither of the two known-open flake classes occurred in any of the ten runs:

- The transient-`starting` assertion (the class the superseded report's one
  failure belonged to, cured by `021-cure-01`/`021-cure-02`).
- The SIGWINCH exact-count assertion in `features/harness.feature:199`'s
  scenario outline.

Had either occurred, it would be named here the same way — advisory, with
its log path — rather than chased under this task.

## Logs committed

All eleven log files listed in the table above and referenced by filename,
plus this README, live under `docs/reports/phase4b-stability10/`:
`run-01.log` … `run-10.log`, `summary.log`. The prior baf92ed-era per-run
logs (including the unpadded `run-1.log` … `run-9.log` duplicates and the
9/10 `run-02.log` FAIL log) have been removed from this directory; that
9/10 result and its FAIL detail remain on the historical record in git
history at commit `bcd80b9` and in `docs/DELIVERY-LOG.md`.
