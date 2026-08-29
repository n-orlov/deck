# Phase 3g task 512 — `ci/stability.sh 10`, literal launcher, correct metadata

## Why this directory exists

Task 505 measured `ci/stability.sh 10` at the post-501/502/503 tree honestly
(9/10, sole failure `TestSigwinchCountDistinguishesTwoFromThree`), but its
round-2 report was rejected on an **execution-shape** residual, not on the
measured substance:

- `/run/ralphd/artifacts/stability-505.nohup` is a 0-byte file left behind by
  the actual launch, proving an extra redirection was added to the pipeline
  at launch time — while `docs/reports/phase3g-505-stability10/README.md`
  claimed the launch command was used unmodified.
- That README also stated `summary.log` is 891 628 bytes; `stat`/`wc -c`
  report 891 714 bytes.

This directory redoes the capture with the launcher issued **verbatim**, and
every size/line-count claim below is checked against the committed file in the
same shell call that states it. It supersedes
`docs/reports/phase3g-505-stability10/` for the same reason round 2 there
superseded its own round 1: execution-shape integrity, not because either
directory's numbers were found untrue. `phase3g-505-stability10/` is left
exactly as it stands, unedited.

An earlier round of *this* task (commit `05f85dd`, 10/10) was itself rejected
on the same class of residual — its launch shell call also ran `echo` and
`date -u`, and two of its polls added `echo`/`cat` — so the capture published
below replaces it in this same directory. Its numbers were not disputed
either; the measurement was simply redone under a clean shape.

## Launch

- **Launch commit** (`git rev-parse HEAD`, printed by the same shell call that
  issued the launch line below, with nothing else in that call):
  `05f85dd54007702a9e4ea2a35150554ead146343`.
- **Launch command**, the only other thing in that shell call, issued
  byte-for-byte as prescribed with no added redirection anywhere in the
  pipeline and no extra file:

  ```
  nohup sh -c 'timeout 7200 ci/stability.sh 10 > /run/ralphd/artifacts/stability-512.log 2>&1; s=$?; printf "%s\n" "$s" > /run/ralphd/artifacts/stability-512.exitstatus' &
  ```

- **No stray artifact was created.** After the driver exited,
  `ls /run/ralphd/artifacts/ | grep 512` lists exactly two files —
  `stability-512.exitstatus` and `stability-512.log` — and nothing else: no
  `stability-512.nohup` sentinel of the kind that sank 505's report, and no
  `nohup.out` anywhere in the worktree (`git status --porcelain` after the run
  showed only the modified files in this directory).
- Between the launch and the driver exiting, the only commands issued were
  four polls of the form `sleep N; grep '^=== RUN'
  /run/ralphd/artifacts/stability-512.log` (N = 1500, 1500, 780, 45) — no
  `echo`, no `cat`, no `ls`, no reading a per-run log, no editing, nothing
  else touched the machine while the suite ran.
- **Script exit status** (captured with `s=$?` in the same shell call that ran
  the script, never through a pipe): `1`, because two of the ten runs failed.
  It is committed here as `script.exitstatus`, which is 2 bytes
  (`wc -c script.exitstatus` = 2: the digit and its newline) and reads `1`.

Wall clock, from the two timestamps that exist: the shell call immediately
preceding the launch call printed `date -u` = 2026-08-29 10:31:39 UTC, and
`/run/ralphd/artifacts/stability-512.exitstatus` has mtime 2026-08-29
11:32:18 UTC, so the ten runs took roughly **60m39s** end to end. `driver.log`
carries no timestamps, so no per-run wall clock can be read from the committed
evidence.

## Observed rate, quoted verbatim from `summary.log`

```
full per-run logs and combined summary log kept in: /tmp/deck-stability.eovHWP
8/10 passed
```

**8/10.** This is published as measured; it is not rounded up, and it does
**not** close review finding 1, whose gate is 10/10 at the final code commit
(task 507). Runs 1 and 4 failed; runs 2, 3, 5, 6, 7, 8, 9 and 10 passed.

## Per-run PASS/FAIL table

PASS/FAIL comes from `driver.log`'s `=== RUN n: PASS/FAIL (exit N) ===`
markers, cross-checked against each `run-N.log`'s own package result lines.
`driver.log` carries no timestamps, so the time column is instead each run's
own summed package test time, derived from that run's committed `run-N.log`
(`go test -p=1` runs packages serially, so the sum is that run's test time
excluding its container start and Go build).

| Run | Result | Failing test (file:line)                                              | Test time, summed from `run-N.log` | Per-run log  |
|-----|--------|-----------------------------------------------------------------------|------------------------------------|--------------|
| 1   | FAIL   | `TestSigwinchCountDistinguishesTwoFromThree` (`features/sigwinch_count_test.go:89`) | 359.6s (5m59.6s) | `run-1.log`  |
| 2   | PASS   | —                                                                     | 359.6s (5m59.6s)                   | `run-2.log`  |
| 3   | PASS   | —                                                                     | 359.1s (5m59.1s)                   | `run-3.log`  |
| 4   | FAIL   | `TestSigwinchCountDistinguishesTwoFromThree` (`features/sigwinch_count_test.go:89`) | 359.5s (5m59.5s) | `run-4.log`  |
| 5   | PASS   | —                                                                     | 355.3s (5m55.3s)                   | `run-5.log`  |
| 6   | PASS   | —                                                                     | 358.0s (5m58.0s)                   | `run-6.log`  |
| 7   | PASS   | —                                                                     | 364.6s (6m04.6s)                   | `run-7.log`  |
| 8   | PASS   | —                                                                     | 360.2s (6m00.2s)                   | `run-8.log`  |
| 9   | PASS   | —                                                                     | 362.2s (6m02.2s)                   | `run-9.log`  |
| 10  | PASS   | —                                                                     | 356.0s (5m56.0s)                   | `run-10.log` |

Those ten sums total 3594.1s (59m54.1s), 45s short of the ~60m39s end-to-end
elapsed above; the residue is the ten `--rm` sibling containers' start-up and
Go build time, which no committed log records per run.

Integrity check on the committed evidence: prefixing each `run-N.log` with its
`=== RUN n ===` marker, appending its own `=== RUN n: PASS (exit 0) ===` /
`=== RUN n: FAIL (exit 1) ===` marker and then the two tally lines
(`full per-run logs and combined summary log kept in: ...` and `8/10 passed`)
reproduces `summary.log` **byte for byte** — reconstructed into a scratch file
and compared with `cmp`, exit status 0, in the same shell call that measured
`wc -c summary.log` = **1 773 967 bytes** and `wc -l summary.log` = **10 200
lines**. So the ten per-run logs committed here are exactly the ten runs the
driver labelled. `driver.log`, the driver's own stdout, is 523 bytes and 22
lines by `wc -c`/`wc -l` in that same call.

## The two failures

Both failing runs failed in the same place, with the same assertion:

```
--- FAIL: TestSigwinchCountDistinguishesTwoFromThree (2.18s)
    sigwinch_count_test.go:89: sigwinch count after 1st resize = 0, want exactly 1 before sending the 2nd
FAIL
FAIL	github.com/n-orlov/deck/features	308.941s
```

- **Run 1** — `TestSigwinchCountDistinguishesTwoFromThree`,
  `features/sigwinch_count_test.go:89` (the `t.Fatalf` guarding that the first
  resize's SIGWINCH is observably recorded before the second is raised); log
  `run-1.log`, `--- FAIL` at line 5004, package line
  `FAIL github.com/n-orlov/deck/features` at line 5007.
- **Run 4** — the same test at the same `features/sigwinch_count_test.go:89`,
  reported at 2.15s; log `run-4.log`, `--- FAIL` at line 5004.

No other test or scenario failed in either run: each of `run-1.log` and
`run-4.log` contains exactly one `--- FAIL` line, and the eight passing runs
contain none. This is the same flake task 505 named — 505 saw it twice in its
own twenty runs, this task's earlier round saw it zero times in ten, and this
round saw it twice in ten. Its disposition (fix, or record as an open finding)
belongs to tasks 506/509/511; nothing here licenses a claim that it is fixed,
and nothing here licenses weakening or skipping the test.

## Frozen-tree proof

No code commit landed between the launch and this report:

```
$ git rev-parse HEAD
05f85dd54007702a9e4ea2a35150554ead146343
$ git diff 05f85dd54007702a9e4ea2a35150554ead146343..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum
<empty>
```

The diff was measured with `| wc -c` = 0 after the driver exited, and this
task's own commit adds only files under this directory.

## Named skips (unchanged from 505)

- `features/godog_test.go`'s `defaultTags`: `~@real-agents && ~@nightly` —
  `@real-agents` and `@nightly` scenarios are therefore not part of these ten
  runs. `defaultTags` was not edited by this task.
- Three packages have no test files and print `?   ... [no test files]` in
  every run: `internal/notify`, `internal/search`, `internal/unit`.
- The harness self-test `TestGodogRejectsUndefinedAndFailedSteps` deliberately
  injects a step failure inside its own subtest in every run (visible in the
  `features` package output of the failing runs' verbose logs as
  `Scenario: error binding` / `deliberate step failure`). It is a self-test
  proving the harness rejects undefined and failed steps, not a product
  failure; the passing runs' `ok github.com/n-orlov/deck/features` line
  confirms the package still passes with it present.

## Relationship to 505

`docs/reports/phase3g-505-stability10/` (round 2, launch sha `d863538`)
measured **9/10** at an earlier point of the same frozen-tree lineage, with
`TestSigwinchCountDistinguishesTwoFromThree` as its sole failure — the same
test at the same file:line as both failures here. This directory supersedes it
as the measurement of record for execution-shape integrity only; 505's 9/10 is
not contradicted, and 505's own report is left untouched.

Across everything measured on this tree the sigwinch-count race stands at 4
hits in 40 runs (505 round 1 and round 2: 2/20; this task's earlier round:
0/10; this round: 2/10) — a low-probability race whose rate is stable enough
that neither a 10/10 nor an 8/10 round should be read as it appearing or
disappearing.
