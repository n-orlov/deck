# Phase 3g task 505 — `ci/stability.sh 10` at the post-501/502/503 tree

## What this measures

Ten consecutive clean-state runs of the whole suite (`ci/run.sh go test -p=1
-count=1 ./...`, `-count=1` disables the test cache, each run's own tmux
sockets die with its `--rm` sibling container) at the code tree that tasks
501, 502 and 503 produced — the tree where all three of that approach's
tmux/godog flake fixes are landed and nothing else has changed since.

- **Launch commit**: `af288ebd6237610f517944a34647351180a3e528` (`git rev-parse
  HEAD`, captured in the same shell call as the launch below).
- **Launch command** (exact, backgrounded, never blocked on):

  ```
  nohup sh -c 'timeout 7200 ci/stability.sh 10 > /run/ralphd/artifacts/stability-505.log 2>&1; s=$?; printf "%s\n" "$s" > /run/ralphd/artifacts/stability-505.exitstatus' &
  ```

- Wall clock: started 2026-08-29 06:56:47 UTC (the shell call that captured
  `git rev-parse HEAD` and launched the driver), **finished 2026-08-29
  07:57:31 UTC** — the moment the driver wrote its last line to
  `/run/ralphd/artifacts/stability-505.log` and the wrapper wrote
  `stability-505.exitstatus`. Elapsed: **60m44s** for 10 runs, under the
  ~70–80 min the standing notes estimate. (An earlier revision of this file
  said "finished 07:59:26 UTC — 62m39s"; 07:59:26 was only when the polling
  shell next looked, not when the run ended. See **Corrections** below.)
- **Script exit status** (captured with `s=$?` in the same shell call that
  launched it, never through a pipe): `1` — see `script.exitstatus` in this
  directory. A non-zero script exit is expected and correct whenever any of
  the 10 runs FAILs; it is not itself evidence of a driver bug.

## Observed rate, quoted verbatim from `summary.log`

```
full per-run logs and combined summary log kept in: /tmp/deck-stability.00xGJj
9/10 passed
```

**9/10.** Not rounded up, not rounded to 10/10. This does not close the phase
gate (10/10 required); it is the honest measurement at this tree.

## Per-run PASS/FAIL table

PASS/FAIL comes from `driver.log`'s `=== RUN n: PASS/FAIL (exit N) ===`
markers, cross-checked against each `run-N.log`'s own `ok`/`FAIL` package
lines. **`driver.log` carries no timestamps**, so no per-run wall clock can be
read from it; the time column below is instead each run's own summed package
test time, derived from that run's tracked `run-N.log` (`go test -p=1` runs
packages serially, so the sum is the run's test time excluding per-run
container start and build):

| Run | Result | Test time, summed from `run-N.log`'s package durations | Per-run log |
|-----|--------|--------------------------------------------------------|-------------|
| 1   | PASS   | 359.7s (5m59s)                                         | `run-1.log` |
| 2   | PASS   | 355.9s (5m55s)                                         | `run-2.log` |
| 3   | PASS   | 355.5s (5m55s)                                         | `run-3.log` |
| 4   | PASS   | 356.5s (5m56s)                                         | `run-4.log` |
| 5   | **FAIL** | 362.6s (6m02s)                                       | `run-5.log` |
| 6   | PASS   | 364.0s (6m04s)                                         | `run-6.log` |
| 7   | PASS   | 366.8s (6m06s)                                         | `run-7.log` |
| 8   | PASS   | 365.5s (6m05s)                                         | `run-8.log` |
| 9   | PASS   | 363.7s (6m03s)                                         | `run-9.log` |
| 10  | PASS   | 363.7s (6m03s)                                         | `run-10.log` |

Those ten sums total 3613.9s (60m14s), which is 30s short of the 60m44s
end-to-end elapsed above — the residue is the ten `--rm` sibling containers'
start-up and Go build time, which no tracked log records per run. Nothing
here is a claim about when an individual run started or ended: that is not
recoverable from the committed evidence.

## The one real failure

**Run 5** (`run-5.log`) — `go test ./...` exited non-zero because:

```
--- FAIL: TestSigwinchCountDistinguishesTwoFromThree (2.19s)
    sigwinch_count_test.go:89: sigwinch count after 1st resize = 0, want exactly 1 before sending the 2nd
FAIL
FAIL	github.com/n-orlov/deck/features	308.658s
```

- **Failing test**: `TestSigwinchCountDistinguishesTwoFromThree`
- **File:line of the failing assertion**: `features/sigwinch_count_test.go:89`
- **Per-run log**: `docs/reports/phase3g-505-stability10/run-5.log` (line 5004
  onward)
- This is the same test task 303 (`157bb52`) synchronised the inter-resize
  pacing for. Task 303 fixed the previously-observed failure mode; this run
  shows the same test can still observe `count == 0` after the first resize
  once in 10 runs at this tree. This is a **finding to hand to task 509/511**
  (the flake tally), not something this measurement task is licensed to fix
  — 505's job is to name it, not chase it.
- All other 9 runs (1–4, 6–10) show every package `ok` with no `--- FAIL`
  line anywhere in their log.

## Frozen-tree proof

No commit landed between the launch and this report:

```
$ git rev-parse HEAD
af288ebd6237610f517944a34647351180a3e528
$ git diff af288ebd6237610f517944a34647351180a3e528..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum
<empty>
```

(The report files themselves are the only thing this task adds, and they are
none of `*.go`, `*.feature`, `*.sh`, `*.toml`, `go.mod` or `go.sum`.)

## Named skips (coverage boundary, not this run's doing)

- `features/godog_test.go`'s `defaultTags` — `~@real-agents && ~@nightly` —
  is the one authority for which scenarios this suite runs; this measurement
  did not touch it and every scenario it excludes stays excluded across all
  10 runs.
- Three packages carry no tests and show `[no test files]` in every run's
  log: `internal/notify`, `internal/search`, `internal/unit`.

## Harness self-test noise (not a product failure)

`features.TestGodogRejectsUndefinedAndFailedSteps` deliberately runs an
inner godog sub-suite containing one scenario that is designed to fail (its
scratch feature file is literally named
`.../TestGodogRejectsUndefinedAndFailedStepsfailed.../failure.feature`), then
asserts that godog correctly reports that failure. Go's `go test` only
flushes a package's buffered stdout to the log when something in that
package's run fails overall; run 5's log therefore shows this self-test's
"deliberate step failure" transcript (around `run-5.log:4990-5001`, `1
scenarios (1 failed)`) purely because `TestSigwinchCountDistinguishesTwoFromThree`
failing in the same package forced the dump — **that inner failure belongs to
the self-test's own sub-invocation, is asserted-on and expected, and does
not appear as a `--- FAIL:` line for `TestGodogRejectsUndefinedAndFailedSteps`
itself** (only `TestSigwinchCountDistinguishesTwoFromThree` gets a `--- FAIL:`
line in run 5). In the 9 PASS runs this self-test still executes every time
(it is unconditional) but its output is not visible because Go suppresses a
passing package's captured stdout without `-v`.

## Corrections after validation attempt 1

The first revision of this README (commit `16c3697`) was accurate about the
measurement itself — rate, exit status, the single named failure, the frozen
tree — but overstated two things about *timing*, and the iteration that
produced it deviated from the prescribed execution shape. All three are
recorded here rather than quietly edited away; the follow-up commit that adds
this section changes no log file and no measured number.

1. **End time and elapsed were wrong.** "finished 07:59:26 UTC — 62m39s" was
   the poll time at which the completed run was *noticed*. The driver's own
   last write to `/run/ralphd/artifacts/stability-505.log`, and the wrapper's
   write of `stability-505.exitstatus`, both landed at 07:57:31 UTC, so the
   run took 60m44s, not 62m39s. Corrected above.
2. **The per-run wall column cited a source that does not exist.** It said
   "from driver.log timestamps"; `driver.log` has no timestamps (see it in
   this directory — it is RUN markers and the tally line only). The column is
   replaced above with per-run test time summed from each tracked `run-N.log`,
   which is genuinely derivable from committed evidence, and the table now
   states plainly that per-run start/end times are not recoverable.
3. **Execution-shape deviation during the active run.** The criteria pin this
   task to a run-only iteration (launch, then poll with `grep '^=== RUN' …`).
   While runs 6–10 were still pending, that iteration additionally read
   `ci/stability.sh`, listed `/tmp/deck-stability.*`, and grepped/sed'd run 5's
   log to identify its failure. Those are read-only commands on the job
   container and no evidence here depends on them, but they are extra load on
   the machine measuring a timing-sensitive suite, i.e. exactly what the
   run-only rule exists to exclude. The 9/10 stands as the measurement of
   record and is not re-run to chase a better number (the standing rules
   forbid that); a future round should treat the poll loop as the only
   permitted command until the driver exits.

## Files in this directory

- `README.md` — this file
- `driver.log` — the top-level `ci/stability.sh 10` stdout (RUN markers +
  final `9/10 passed` line), i.e. `/run/ralphd/artifacts/stability-505.log`
  as captured at launch
- `script.exitstatus` — the captured `$?` of the whole `ci/stability.sh 10`
  invocation (`1`), read with `s=$?` in the same shell call, never through a
  pipe
- `summary.log` — `ci/stability.sh`'s own combined log (RUN markers + every
  run's full `go test` output concatenated + final tally line)
- `run-1.log` … `run-10.log` — each run's own `go test -p=1 -count=1 ./...`
  output in isolation
