# Phase 3g task 507 — `ci/stability.sh 10` at the final code tree

## Round 2 supersedes round 1 — 10/10

Round 1 (below, kept for history — its numbers are not disputed) measured
**9/10** at commit `7fa6f890`, with the sole failure the recurring
`TestSigwinchCountDistinguishesTwoFromThree` startup race
(`features/sigwinch_count_test.go:89`). Between round 1 and this round,
commit `b0a4e7d` (task 507, `docs/reports/phase3g-507-sigwinch-startup-race/`)
root-caused and fixed that race: the test used to "confirm" the SIGWINCH
counter started at 0 by polling a file that reads as 0 when it does not
exist yet, which passed trivially the instant the fixture process was
`exec`'d — long before the fixture's own `signal.Notify` call had actually
run — so the very first resize's real `SIGWINCH` could arrive while the
fixture's disposition for it was still the default (ignore) and be silently
discarded, with no way for any later wait to recover it. The test now waits
for the fixture's own initial size-log entry (proof `signal.Notify` has run)
before sending that first resize. Full root-cause writeup, red-before
(4/60 under artificial CPU contention) and green-after (60/60 twice, same
contention) are in that directory; product code was not touched.

This round re-measures at the tree with that fix in place, using the exact
mandated launch shape, and observes a clean ten.

## Round 2 launch

- **Launch commit** (`git rev-parse HEAD`, printed by the same shell call
  that issued the launch line below, with nothing else in that call):
  `b0a4e7d610d3c1b84205705deb50578b2c0515e4`.
- **Launch command**, the only other thing in that shell call, issued
  byte-for-byte as prescribed:

  ```
  nohup sh -c 'timeout 7200 ci/stability.sh 10 > /run/ralphd/artifacts/stability-507.log 2>&1; s=$?; printf "%s\n" "$s" > /run/ralphd/artifacts/stability-507.exitstatus' &
  ```

  (An earlier launch attempt in this same iteration was aborted before any
  run completed because its shell call also contained a stray `echo`
  reporting the backgrounded PID — a discipline breach of the same class
  that sank 512 round 1 — so that partial run's own PID (`94309`, plus its
  two orphaned children `94310`/`94311`) was killed by exact PID, verified
  by `ps` before each `kill`, and its partial log/exitstatus files were
  removed before this clean relaunch. No data from that aborted attempt
  appears anywhere in this report.)

- Between the clean launch and the driver exiting, the only commands issued
  were eight polls of the form `sleep N; grep '^=== RUN'
  /run/ralphd/artifacts/stability-507.log` (N = 480 x7, plus one 480 that
  observed the tenth run already complete) — no `echo`, no `cat`, no `ls`,
  no reading a per-run log, no editing, nothing else touched the machine
  while the suite ran. Byte/line claims and the frozen-tree diff below were
  checked only after the driver had exited (confirmed via `ps`: no
  `stability.sh`/`go test`/`ci/run.sh` process remained, and
  `stability-507.exitstatus` already held its value).
- **Script exit status** (captured with `s=$?` in the same shell call that
  ran the script, never through a pipe): `0`. Committed here as
  `script.exitstatus`, `wc -c script.exitstatus` = 2 bytes (the digit `0`
  and its newline).
- No stray artifact: after the driver exited, the only files under
  `/run/ralphd/artifacts/` matching `stability-507` were `stability-507.log`
  and `stability-507.exitstatus` (checked with `ls /run/ralphd/artifacts/ |
  grep 507`, run after the driver's exit was confirmed).

## Observed rate, quoted verbatim from `summary.log`

```
full per-run logs and combined summary log kept in: /tmp/deck-stability.VkUAuu
10/10 passed
```

**10/10.** All ten runs passed. Review finding 1's gate — `ci/stability.sh
10` at 10/10 on the phase's final code tree — is met by this measurement.

## Per-run PASS/FAIL table

PASS/FAIL comes from `driver.log`'s `=== RUN n: PASS/FAIL (exit N) ===`
markers, cross-checked against each `run-N.log`'s own package result lines
(all sixteen packages read `ok`/`?   [no test files]` in every run — no
`FAIL` line anywhere in any of the ten logs).

| Run | Result | `features` pkg time | Per-run log | Log size (`wc -c`) |
|-----|--------|----------------------|-------------|---------------------|
| 1   | PASS   | 325.583s | `run-1.log`  | 894 bytes |
| 2   | PASS   | 314.156s | `run-2.log`  | 894 bytes |
| 3   | PASS   | 312.243s | `run-3.log`  | 894 bytes |
| 4   | PASS   | 306.597s | `run-4.log`  | 894 bytes |
| 5   | PASS   | 305.529s | `run-5.log`  | 894 bytes |
| 6   | PASS   | 306.202s | `run-6.log`  | 894 bytes |
| 7   | PASS   | 305.301s | `run-7.log`  | 894 bytes |
| 8   | PASS   | 309.963s | `run-8.log`  | 894 bytes |
| 9   | PASS   | 306.772s | `run-9.log`  | 894 bytes |
| 10  | PASS   | 310.531s | `run-10.log` | 894 bytes |

Every run-log is exactly 894 bytes — expected shape for a fully-passing
`go test -p=1 -count=1 ./...` (no `-v`, no failure to dump verbose scenario
output for), sixteen terse `ok`/`?` lines each, package-for-package
identical in content shape run to run and differing only in elapsed times.
`driver.log` is 524 bytes / 22 lines (`wc -c`/`wc -l`).

Integrity check on the committed evidence: prefixing each `run-N.log` with
its `=== RUN n ===` marker, appending its own `=== RUN n: PASS (exit 0) ===`
marker, then the two tally lines (`full per-run logs and combined summary
log kept in: ...` and `10/10 passed`) reproduces `summary.log` **byte for
byte** — reconstructed into a scratch file and compared with `cmp`, exit
status 0, in the same shell call that measured `wc -c summary.log` =
**9464 bytes** for both the reconstruction and the committed file. So the
ten per-run logs committed here are exactly the ten runs the driver
labelled, and none was hand-edited.

## Cumulative sigwinch-count race history (now closed)

Before the fix landing between round 1 and round 2, this specific race had
been hit 5 times in 40 stability-run repetitions across this tree's lineage
(505: 2/20, 512: 2/10, 507 round 1: 1/10). This round's 10/10, following the
fix, is the first clean ten observed for this race and is consistent with
(though on its own does not statistically prove) the race being closed;
`docs/reports/phase3g-507-sigwinch-startup-race/`'s dedicated red/green
evidence (4/60 before, 0/120 after, under artificial contention) is the
stronger claim for causation. F28 in
`docs/reports/phase3g-findings.md` should be read alongside this fix, not
disputed by it — F28 disclosed the race honestly as it stood before the fix
existed.

## Named skips

- `features/godog_test.go`'s `defaultTags`: `~@real-agents && ~@nightly` —
  `@real-agents` and `@nightly` scenarios are therefore not part of these ten
  runs. `defaultTags` was not edited by this task or by the sigwinch fix.
- Three packages have no test files and print `?   ... [no test files]` in
  every run: `internal/notify`, `internal/search`, `internal/unit`.
- The harness self-test `TestGodogRejectsUndefinedAndFailedSteps` deliberately
  injects a step failure inside its own subtest in every run; because none
  of these ten runs failed, that self-test's pass is folded into each run's
  terse `ok  github.com/n-orlov/deck/features` line rather than appearing as
  visible text in these particular logs (it only prints verbosely on a
  package failure) — it is still exercised every run, per the package
  passing, and remains a self-test proving the harness rejects undefined and
  failed steps, never a product failure.

## Frozen-tree proof

No code commit landed between the launch and this report:

```
$ git rev-parse HEAD
b0a4e7d610d3c1b84205705deb50578b2c0515e4
$ git diff b0a4e7d610d3c1b84205705deb50578b2c0515e4..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum | wc -c
0
```

The diff was measured after the driver exited; `git status --porcelain`
before this task's commit shows only this directory's files (already
present from round 1, now overwritten) as modified/untracked.

## Result

**Finding 1's gate is met: 10/10 at commit `b0a4e7d610d3c1b84205705deb50578b2c0515e4`,**
which is this report's own HEAD (frozen-tree diff empty). The residual
sigwinch-count startup race that blocked every prior attempt on this tree
lineage is fixed (`docs/reports/phase3g-507-sigwinch-startup-race/`), and
this round is the first stability-run evidence of the fix holding across a
full ten-run cycle.

---

## Round 1 (superseded, kept for history — 9/10 at commit `7fa6f890`)

### Why this directory exists

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

### Launch

- **Launch commit**: `7fa6f8903aa761cb615d01853a2800e462ef0cf6`.
- **Launch command**, byte-for-byte as prescribed:

  ```
  nohup sh -c 'timeout 7200 ci/stability.sh 10 > /run/ralphd/artifacts/stability-507.log 2>&1; s=$?; printf "%s\n" "$s" > /run/ralphd/artifacts/stability-507.exitstatus' &
  ```

- Polls: three of `sleep N; grep '^=== RUN' /run/ralphd/artifacts/stability-507.log`
  (N = 600, 1500, 1800).
- **Script exit status**: `1` (one of ten runs failed), 2 bytes.

### Observed rate, quoted verbatim from that round's `summary.log`

```
full per-run logs and combined summary log kept in: /tmp/deck-stability.7I8BRr
9/10 passed
```

**9/10.** Run 8 failed (`TestSigwinchCountDistinguishesTwoFromThree`,
`features/sigwinch_count_test.go:89`); runs 1–7, 9 and 10 passed. That
round's `summary.log` was 891 715 bytes / 5 196 lines (dominated by run 8's
verbose failure dump); its per-run logs and driver.log are not preserved in
this directory's current file set (overwritten by round 2's smaller,
all-passing logs) — this section is retained as the historical record of
what was observed and reported at the time, per the same pattern task 512
used when it superseded task 505.

### Result (as reported at the time)

Finding 1's gate was not met by round 1's measurement: 9/10, not 10/10. The
task was reported as not met so it could be re-attempted — round 2, above,
is that re-attempt, via a product-level fix to the sigwinch-count race
followed by a fresh measurement.
