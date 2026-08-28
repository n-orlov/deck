# Phase 3g task 302 — `ci/stability.sh 10` at the frozen code head

## Launch

```
$ cd /workspace && git rev-parse HEAD
bdc18796c5633208403d9ae25760ed87edc08724
$ nohup timeout 7200 ci/stability.sh 10 > /run/ralphd/artifacts/stability-302.log 2>&1 &
```

- Launch sha (`git rev-parse HEAD` recorded at launch): **`bdc18796c5633208403d9ae25760ed87edc08724`** (`bdc1879`).
- Launched 2026-08-28 21:07 UTC, completed 2026-08-28 22:09 UTC (~62 min wall for the ten
  runs; well inside the 120-min iteration cap, and the command itself was started as a
  non-blocking background job per `nohup ... &` and polled, never blocked on).
- `git rev-parse HEAD` at report-authoring time is unchanged: still `bdc1879` (verified
  immediately below); no other commit landed on the tree between launch and this report.

## Observed rate

Verbatim from `summary.log` (also the last line of `stability-302.log`):

```
7/10 passed
```

## Script's own exit status

The stability run was launched with `nohup ... &` per the task's required launch
command, which backgrounds the job immediately — there is no single shell call that both
starts the run and waits 60-80 minutes for it to finish, so the literal `$?` of the launch
call is only the exit status of the `&`-backgrounding itself (0), not of `ci/stability.sh`.
The process was polled to completion (`ps -p <pid>`, log tail) rather than `wait`-ed on from
a *different* shell (background jobs are not attached to a new shell's job table, so a later
bash call cannot `wait` on that PID either), so the real exit status was not captured as a
literal `$?` in either the launch call or a poll call.

`ci/stability.sh`'s own exit logic (`ci/stability.sh`, end of file) is:
```sh
if [ "$fail" -gt 0 ]; then
    exit 1
fi
exit 0
```
i.e. the exit status is a deterministic function of the observed fail count. `summary.log`
records `fail=3` (runs 4, 7 and 10 below), so by the script's own documented logic the exit
status of this invocation was **1**. This is derived from the script's own read source and
the observed per-run PASS/FAIL markers below, not re-run to obtain a literal capture.

## Per-run results

| run | result | notes |
|-----|--------|-------|
| 1 | PASS (exit 0) | |
| 2 | PASS (exit 0) | |
| 3 | PASS (exit 0) | |
| 4 | **FAIL** (exit 1) | see below |
| 5 | PASS (exit 0) | |
| 6 | PASS (exit 0) | |
| 7 | **FAIL** (exit 1) | see below |
| 8 | PASS (exit 0) | |
| 9 | PASS (exit 0) | |
| 10 | **FAIL** (exit 1) | see below |

Full logs: `run-1.log` .. `run-10.log` in this directory (copied verbatim out of the
script's `mktemp` outdir, `/tmp/deck-stability.gd2mKZ`, after the run completed).
Combined log: `summary.log`. Frozen-tree proof: `frozen-tree-check.log`.

### `=== RUN 4: FAIL` — `docs/reports/phase3g-302-stability10/run-4.log`

- Failing scenario: **"the collapsed strip's attention count matches the sort's own notion
  of attention"** (`features/attention_sort.feature:92`, failing step at line 104).
- Error (`run-4.log`, package `github.com/n-orlov/deck/features`,
  `TestFeatures/the_collapsed_strip's_attention_count_matches_the_sort's_own_notion_of_attention`):
  `after scenario hook failed: deck client "A" collapsed strip attention count = 1, want 2`.

### `=== RUN 7: FAIL` — `docs/reports/phase3g-302-stability10/run-7.log`

- Failing scenario: **"a failing pre_launch leaves visible evidence without attaching"**
  (`features/crash.feature:46`, failing step at line 48).
- Error (`run-7.log`, `TestFeatures/a_failing_pre_launch_leaves_visible_evidence_without_attaching`):
  `after scenario hook failed: timed out waiting for frame "starting": context deadline exceeded`.

### `=== RUN 10: FAIL` — `docs/reports/phase3g-302-stability10/run-10.log`

- Failing scenario: **"a wheel notch scrolls an attached pane's scrollback and leaves the
  shell's input line untouched"** (`features/attach_scroll.feature:11`, failing step at
  line 20).
- Error (`run-10.log`,
  `TestFeatures/a_wheel_notch_scrolls_an_attached_pane's_scrollback_and_leaves_the_shell's_input_line_untouched`):
  `after scenario hook failed: client "A" frame changed after gesture, want unchanged from
  captured "before-wheel-scroll"`.

Note: each of `run-4.log`, `run-7.log` and `run-10.log` also contains a later, unrelated
`Scenario: error binding` / `a failing step` failure with error text `deliberate step
failure`. That is the Godog self-test fixture `TestGodogRejectsUndefinedAndFailedSteps`
(in `features/`) deliberately injecting an undefined/failed step to prove the harness
correctly labels such steps as failures — it is not a product bug and is not one of the
three real failures tallied above; it is the same self-test in every run (including the
seven PASS runs' packages, where it is not separately visible in the terse `go test`
output because per-test detail is only printed for the failing top-level package).

## Measured surface — exclusions and no-test packages

- `features/godog_test.go`'s `defaultTags` (line 16): `"~@real-agents && ~@nightly"` —
  scenarios tagged `@real-agents` or `@nightly` are excluded from every one of these ten
  runs. File is unmodified by this task (verified: `frozen-tree-check.log`'s empty diff
  covers `*.feature`; `godog_test.go` itself is `*.go`, also covered by that same empty
  diff).
- Three packages have no test files and report `[no test files]` in every PASS run's log
  (see e.g. `run-1.log`): `internal/notify`, `internal/search`, `internal/unit`.

## Frozen-tree proof

`frozen-tree-check.log` (this directory) records, in one shell call each:
```
$ git rev-parse HEAD
bdc18796c5633208403d9ae25760ed87edc08724

$ git diff --stat bdc1879..HEAD -- "*.go" "*.feature" "*.sh" "*.toml" go.mod go.sum ; echo "exit status: $?"
exit status: 0
```
Empty diff, confirming the ten runs above measured the code tree at `bdc1879` and that
tree has not moved between launch and the writing of this report.

## Disposition

This task is a measurement, not a fix. Per the standing rules, a green suite (or a 9/10 or
7/10 one) is not itself evidence a requirement is met, and this task must not be re-run to
improve the number: **7/10 is the honest result at `bdc1879` and is reported as-is.** The
three failing scenarios above are candidates for later disposition (fix, revert-and-reproduce
evidence, or a named finding) under whichever follow-up task in this plan is licensed to
touch product/test code for stability (see `docs/reports/phase3g-findings.md` and task 303's
scope) — this task does not attempt that itself.
