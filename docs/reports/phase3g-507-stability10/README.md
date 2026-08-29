# Phase 3g task 507 — `ci/stability.sh 10` at the final code tree

## Why this directory exists

Review finding 1's gate is `ci/stability.sh 10` at **10/10** on the phase's
final code tree. Task 505 (9/10, superseded on execution-shape grounds by
task 512, 8/10) and task 512 both measured an earlier point on this tree;
task 506 dispositioned the one failure both of them named
(`TestSigwinchCountDistinguishesTwoFromThree`,
`features/sigwinch_count_test.go:89`) as an out-of-scope open finding (F28).
This task re-measures at the tree as it stands after 506, to establish
whether the gate is now met. It is not: the run below observed **9/10**, and
the sole failure is, once again, the same test at the same line. Per this
task's own criteria, a rate short of 10/10 is published honestly rather than
rounded up, and the task is reported as not met.

## Launch

- **Launch commit** (`git rev-parse HEAD`, printed by the same shell call
  that issued the launch line below, with nothing else in that call):
  `7fa6f8903aa761cb615d01853a2800e462ef0cf6`.
- **Launch command**, the only other thing in that shell call, issued
  byte-for-byte as prescribed, with no added redirection anywhere in the
  pipeline and no extra file:

  ```
  nohup sh -c 'timeout 7200 ci/stability.sh 10 > /run/ralphd/artifacts/stability-507.log 2>&1; s=$?; printf "%s\n" "$s" > /run/ralphd/artifacts/stability-507.exitstatus' &
  ```

- Between the launch and the driver exiting, the only commands issued were
  three polls of the form `sleep N; grep '^=== RUN'
  /run/ralphd/artifacts/stability-507.log` (N = 600, 1500, 1800) — no `echo`,
  no `cat`, no `ls`, no reading a per-run log, no editing, nothing else
  touched the machine while the suite ran. Byte/line claims and the
  frozen-tree diff below were checked only after the driver had exited.
- **Script exit status** (captured with `s=$?` in the same shell call that
  ran the script, never through a pipe): `1`, because one of the ten runs
  failed. It is committed here as `script.exitstatus`, 2 bytes
  (`wc -c script.exitstatus` = 2: the digit and its newline), reading `1`.
- No stray artifact was created: after the driver exited, the only files
  under `/run/ralphd/artifacts/` matching `stability-507` are
  `stability-507.log` and `stability-507.exitstatus`.

## Observed rate, quoted verbatim from `summary.log`

```
full per-run logs and combined summary log kept in: /tmp/deck-stability.7I8BRr
9/10 passed
```

**9/10.** This is published as measured; it is not rounded up, and it does
**not** close review finding 1, whose gate is 10/10 at the final code
commit. Run 8 failed; runs 1–7, 9 and 10 passed.

## Per-run PASS/FAIL table

PASS/FAIL comes from `driver.log`'s `=== RUN n: PASS/FAIL (exit N) ===`
markers, cross-checked against each `run-N.log`'s own package result lines.
The test-time column is that run's `features` package time from its own
`run-N.log` (`go test -p=1` runs packages serially; `features` dominates
each run's wall time).

| Run | Result | Failing test (file:line) | `features` pkg time | Per-run log |
|-----|--------|---------------------------|----------------------|-------------|
| 1   | PASS   | —                                                                                    | 305.336s | `run-1.log`  |
| 2   | PASS   | —                                                                                    | 307.423s | `run-2.log`  |
| 3   | PASS   | —                                                                                    | 307.935s | `run-3.log`  |
| 4   | PASS   | —                                                                                    | 308.455s | `run-4.log`  |
| 5   | PASS   | —                                                                                    | 306.943s | `run-5.log`  |
| 6   | PASS   | —                                                                                    | 310.870s | `run-6.log`  |
| 7   | PASS   | —                                                                                    | 307.733s | `run-7.log`  |
| 8   | FAIL   | `TestSigwinchCountDistinguishesTwoFromThree` (`features/sigwinch_count_test.go:89`)  | 309.918s | `run-8.log`  |
| 9   | PASS   | —                                                                                    | 308.011s | `run-9.log`  |
| 10  | PASS   | —                                                                                    | 315.533s | `run-10.log` |

Summing every package's own reported time per run (not just `features`)
across all ten runs totals 3601.6s; the driver's own log (`driver.log`, 523
bytes / 22 lines by `wc -c`/`wc -l`) carries no timestamps, so no per-run
wall clock beyond each run's own summed package time can be read from the
committed evidence.

Integrity check on the committed evidence: prefixing each `run-N.log` with
its `=== RUN n ===` marker, appending its own `=== RUN n: PASS (exit 0) ===`
/ `=== RUN n: FAIL (exit 1) ===` marker, then the two tally lines (`full
per-run logs and combined summary log kept in: ...` and `9/10 passed`)
reproduces `summary.log` **byte for byte** — reconstructed into a scratch
file and compared with `cmp`, exit status 0, in the same shell call that
measured `wc -c summary.log` = **891 715 bytes** and `wc -l summary.log` =
**5 196 lines**. So the ten per-run logs committed here are exactly the ten
runs the driver labelled.

## The one failure

```
--- FAIL: TestSigwinchCountDistinguishesTwoFromThree (2.18s)
    sigwinch_count_test.go:89: sigwinch count after 1st resize = 0, want exactly 1 before sending the 2nd
FAIL
FAIL	github.com/n-orlov/deck/features	309.918s
```

- **Run 8** — `TestSigwinchCountDistinguishesTwoFromThree`,
  `features/sigwinch_count_test.go:89` (the `t.Fatalf` guarding that the
  first resize's SIGWINCH is observably recorded before the second is
  raised); log `run-8.log`, `--- FAIL` at line 5004, package line `FAIL
  github.com/n-orlov/deck/features` at line 5007.

No other test or scenario failed: `run-8.log` contains exactly one `--- FAIL`
line, and the nine passing runs contain none. This is the same flake already
named by 505/512 and dispositioned as out-of-scope finding F28 by task 506 —
nothing here licenses a claim that it is fixed, and nothing here licenses
weakening or skipping the test. This report does not attempt a product fix:
it establishes the measured rate at this tree, as the task requires when the
rate falls short of 10/10.

Cumulative rate for this specific race across every stability measurement on
this tree lineage: 505 (2/20 across its two rounds), 512 (2/10), and this
task (1/10) — **5 hits in 40 runs**, roughly 1-in-8, consistent with the
low-probability residual F28 already describes; this run neither confirms
nor contradicts that estimate, it adds one more data point to it.

## Frozen-tree proof

No code commit landed between the launch and this report:

```
$ git rev-parse HEAD
7fa6f8903aa761cb615d01853a2800e462ef0cf6
$ git diff 7fa6f8903aa761cb615d01853a2800e462ef0cf6..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum
<empty>
```

The diff was measured with `| wc -c` = 0 after the driver exited, and this
task's own commit adds only files under this directory (`git status
--porcelain` before this commit shows only this directory as untracked).

## Named skips

- `features/godog_test.go`'s `defaultTags`: `~@real-agents && ~@nightly` —
  `@real-agents` and `@nightly` scenarios are therefore not part of these ten
  runs. `defaultTags` was not edited by this task.
- Three packages have no test files and print `?   ... [no test files]` in
  every run: `internal/notify`, `internal/search`, `internal/unit`.
- The harness self-test `TestGodogRejectsUndefinedAndFailedSteps` deliberately
  injects a step failure inside its own subtest in every run (visible in
  `run-8.log`'s `features` package output as `Scenario: error binding` /
  `deliberate step failure`, and in every passing run's verbose output the
  same way). It is a self-test proving the harness rejects undefined and
  failed steps, not a product failure; every passing run's `ok
  github.com/n-orlov/deck/features` line confirms the package still passes
  with it present.

## Result

**Finding 1's gate is not met by this measurement: 9/10, not 10/10.** The
task is reported as not met so it can be re-attempted — either by a further
stability run (accepting the residual race's low-probability nature and
hoping for a clean ten), or by a product-level fix to the sigwinch-count
race first. Task 509/511's report updates should reflect this run's addition
to the cumulative rate (now 5/40) alongside F28.
