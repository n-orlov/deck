# Phase 3g task 505 — `ci/stability.sh 10` at the post-501/502/503 tree

## What this measures

Ten consecutive clean-state runs of the whole suite (`ci/run.sh go test -p=1
-count=1 ./...`, `-count=1` disables the test cache, each run's own tmux
sockets die with its `--rm` sibling container) at the code tree that tasks
501, 502 and 503 produced — the tree where all three of that approach's
tmux/godog flake fixes are landed and no `*.go`, `*.feature`, `*.sh`, `*.toml`,
`go.mod` or `go.sum` file has changed since.

This directory holds **two rounds** of that measurement:

- **Round 2 — the measurement of record**, at the top level of this directory
  (`driver.log`, `script.exitstatus`, `summary.log`, `run-1.log` …
  `run-10.log`). Launched at sha `d863538` and produced by a **run-only**
  iteration: after the launch, the only command issued until the driver exited
  was the prescribed poll `grep '^=== RUN'
  /run/ralphd/artifacts/stability-505.log` (preceded by a `sleep`), so nothing
  competed with the suite for the machine.
- **Round 1 — the earlier round, kept as history**, in `round1/` (same file
  names). Launched at sha `af288eb`. Its numbers were verified accurate, but
  the iteration that produced it also read `ci/stability.sh`, listed
  `/tmp/deck-stability.*` and grepped run 5's log *while runs 6–10 were still
  executing*, i.e. it added load to the machine that was measuring a
  timing-sensitive suite. It is superseded by round 2 for that reason, not
  because anything in it was found untrue. See **Round 1** below.

Both rounds observed the same rate and the same single failing test.

## Round 2 (measurement of record)

- **Launch commit**: `d86353852213ce1da742a12eb790d125fce76b15` (`git rev-parse
  HEAD`, captured in the same shell call as the launch below).
- **Launch command** (exact, backgrounded, never blocked on):

  ```
  nohup sh -c 'timeout 7200 ci/stability.sh 10 > /run/ralphd/artifacts/stability-505.log 2>&1; s=$?; printf "%s\n" "$s" > /run/ralphd/artifacts/stability-505.exitstatus' &
  ```

- **Script exit status** (captured with `s=$?` in the same shell call that ran
  it, never through a pipe): `1` — see `script.exitstatus`. A non-zero script
  exit is expected and correct whenever any of the 10 runs FAILs; it is not
  itself evidence of a driver bug.
- Wall clock: launched 2026-08-29 08:10:43 UTC (the shell call that captured
  `git rev-parse HEAD` and launched the driver); the wrapper's write of
  `/run/ralphd/artifacts/stability-505.exitstatus` — the last thing that
  happens after the driver exits — is timestamped 09:12:01 UTC, so the ten runs
  took **61m18s** end to end. Those two timestamps are the only wall-clock
  facts claimed here; per-run start/end times are **not** recoverable from any
  file committed in this directory (see the table's note).

### Observed rate, quoted verbatim from `summary.log`

```
full per-run logs and combined summary log kept in: /tmp/deck-stability.RRzwNI
9/10 passed
```

**9/10.** Not rounded up, not rounded to 10/10. This does not close the phase
gate (10/10 at the final code commit is required); it is the honest measurement
at this tree.

### Per-run PASS/FAIL table

PASS/FAIL comes from `driver.log`'s `=== RUN n: PASS/FAIL (exit N) ===`
markers, cross-checked against each `run-N.log`'s own `ok`/`FAIL` package
lines. **`driver.log` carries no timestamps**, so no per-run wall clock can be
read from it; the time column below is instead each run's own summed package
test time, derived from that run's tracked `run-N.log` (`go test -p=1` runs
packages serially, so the sum is the run's test time excluding that run's
container start and Go build).

| Run | Result   | Test time, summed from `run-N.log`'s package durations | Per-run log  |
|-----|----------|-------------------------------------------------------|--------------|
| 1   | PASS     | 369.9s (6m10s)                                        | `run-1.log`  |
| 2   | PASS     | 367.0s (6m07s)                                        | `run-2.log`  |
| 3   | PASS     | 373.7s (6m14s)                                        | `run-3.log`  |
| 4   | PASS     | 360.5s (6m01s)                                        | `run-4.log`  |
| 5   | PASS     | 363.4s (6m03s)                                        | `run-5.log`  |
| 6   | **FAIL** | 369.6s (6m10s)                                        | `run-6.log`  |
| 7   | PASS     | 356.3s (5m56s)                                        | `run-7.log`  |
| 8   | PASS     | 360.5s (6m01s)                                        | `run-8.log`  |
| 9   | PASS     | 360.7s (6m01s)                                        | `run-9.log`  |
| 10  | PASS     | 366.2s (6m06s)                                        | `run-10.log` |

Those ten sums total 3647.8s (60m48s), 30s short of the 61m18s end-to-end
elapsed above; the residue is the ten `--rm` sibling containers' start-up and
Go build time, which no tracked log records per run.

Integrity check on the committed evidence: prefixing each `run-N.log` with its
`=== RUN n ===` marker, appending its `=== RUN n: PASS/FAIL (exit N) ===`
marker and then the two tally lines reproduces `summary.log` **byte for byte**
(891 628 bytes), so the ten per-run logs here are exactly the ten runs the
driver labelled.

### The one real failure

**Run 6** (`run-6.log`) — `go test ./...` exited non-zero because:

```
--- FAIL: TestSigwinchCountDistinguishesTwoFromThree (2.16s)
    sigwinch_count_test.go:89: sigwinch count after 1st resize = 0, want exactly 1 before sending the 2nd
FAIL
FAIL	github.com/n-orlov/deck/features	318.362s
```

- **Failing test**: `TestSigwinchCountDistinguishesTwoFromThree`
- **File:line of the failing assertion**: `features/sigwinch_count_test.go:89`
- **Per-run log**: `docs/reports/phase3g-505-stability10/run-6.log` (line 5004
  onward)
- This is the same test task 303 (`157bb52`) synchronised the inter-resize
  pacing for, and the same failure mode and same file:line that round 1
  observed in its run 5 (`round1/run-5.log:5004`). Two independent 10-run
  rounds at this tree therefore each show it once: the test can still observe
  `count == 0` after the first resize at roughly 1-in-10. This is a **finding
  for tasks 506/509/511**, not something this measurement task is licensed to
  fix — 505's job is to name it, not chase it.
- All other 9 runs (1–5, 7–10) show every package `ok` with no `--- FAIL` line
  anywhere in their log.

### Frozen-tree proof

No code commit landed between the launch and this report:

```
$ git rev-parse HEAD
d86353852213ce1da742a12eb790d125fce76b15
$ git diff d86353852213ce1da742a12eb790d125fce76b15..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum
<empty>
```

(The commit that adds this round's files touches only this report directory —
`*.md` and `*.log` — so it cannot appear in that diff. Round 1's own frozen
proof, `git diff af288eb..HEAD` over the same paths, is likewise empty:
`af288eb..d863538` is a docs-only commit.)

## Named skips (coverage boundary, not this run's doing)

- `features/godog_test.go`'s `defaultTags` — `~@real-agents && ~@nightly`
  (`features/godog_test.go:16`) — is the one authority for which scenarios this
  suite runs; this measurement did not touch it and every scenario it excludes
  stays excluded across all 10 runs of both rounds.
- Three packages carry no tests and show `[no test files]` in every run's log
  (`run-1.log:10-17`): `internal/notify`, `internal/search`, `internal/unit`.

## Harness self-test noise (not a product failure)

`features.TestGodogRejectsUndefinedAndFailedSteps` deliberately runs an inner
godog sub-suite containing one scenario designed to fail (its scratch feature
file is literally `/tmp/TestGodogRejectsUndefinedAndFailedStepsfailed…/001/failure.feature`),
then asserts that godog correctly reports that failure. That deliberate step
failure is printed in **every** run, and is not a product failure. Go's `go
test` only flushes a package's buffered stdout to the log when something in
that package's run fails overall, so it is *visible* only in the log of a run
whose `features` package failed for another reason — here `run-6.log:4996-5002`
(`Error: deliberate step failure`, `1 scenarios (1 failed)`), dumped because
`TestSigwinchCountDistinguishesTwoFromThree` failed in the same package. It
never produces a `--- FAIL:` line for `TestGodogRejectsUndefinedAndFailedSteps`
itself; in run 6 that line exists only for
`TestSigwinchCountDistinguishesTwoFromThree`. In the 9 PASS runs the self-test
still executes (it is unconditional) and its injected failure still prints into
the package's buffer, which Go discards for a passing package without `-v`.

## Round 1 (earlier round, superseded, kept in `round1/`)

- **Launch commit**: `af288ebd6237610f517944a34647351180a3e528`; same launch
  command and same captured-`$?` discipline as round 2.
- **Rate, verbatim from `round1/summary.log`**:

  ```
  full per-run logs and combined summary log kept in: /tmp/deck-stability.00xGJj
  9/10 passed
  ```

  **9/10**; `round1/script.exitstatus` is `1`.
- Per-run results: runs 1–4 and 6–10 PASS, **run 5 FAIL** —
  `TestSigwinchCountDistinguishesTwoFromThree` at
  `features/sigwinch_count_test.go:89` ("sigwinch count after 1st resize = 0,
  want exactly 1 before sending the 2nd"), log `round1/run-5.log`. Per-run
  summed test times were 359.7 / 355.9 / 355.5 / 356.5 / 362.6 / 364.0 / 366.8
  / 365.5 / 363.7 / 363.7 s; end-to-end 60m44s (launch 06:56:47 UTC, driver's
  last write 07:57:31 UTC).
- Why it is superseded: the criteria pin this task to a run-only iteration
  (launch, then poll with `grep '^=== RUN' …` only). Round 1's iteration
  additionally read `ci/stability.sh`, listed `/tmp/deck-stability.*` and
  grepped/sed'd run 5's log while runs 6–10 were still pending. Those are
  read-only commands and no number in `round1/` depends on them, but they are
  extra load on the machine measuring a timing-sensitive suite — exactly what
  the run-only rule exists to exclude. Round 1 is kept in full because deleting
  a measurement that was made and published is not honest, and because its
  agreement with round 2 (same rate, same single test, same file:line) is
  itself evidence about the flake's frequency.
- Two timing claims in round 1's first revision (commit `16c3697`) were also
  corrected in `d863538`: an end time of 07:59:26 UTC was the poll time rather
  than the driver's last write (07:57:31 UTC, so 60m44s not 62m39s), and a
  per-run wall-clock column was attributed to "driver.log timestamps" when
  `driver.log` has none. The figures quoted above are the corrected ones.

## Files in this directory

- `README.md` — this file
- `driver.log` — round 2's top-level `ci/stability.sh 10` stdout (RUN markers +
  final `9/10 passed` line), i.e. `/run/ralphd/artifacts/stability-505.log` as
  captured at launch
- `script.exitstatus` — round 2's captured `$?` of the whole `ci/stability.sh
  10` invocation (`1`), read with `s=$?` in the same shell call, never through
  a pipe
- `summary.log` — round 2's `ci/stability.sh` combined log (RUN markers + every
  run's full `go test` output concatenated + final tally lines)
- `run-1.log` … `run-10.log` — round 2's ten runs, each run's own `go test
  -p=1 -count=1 ./...` output in isolation
- `round1/` — the same five kinds of file for round 1 (`driver.log`,
  `script.exitstatus`, `summary.log`, `run-1.log` … `run-10.log`)
