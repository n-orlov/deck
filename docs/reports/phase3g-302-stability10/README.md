# Phase 3g task 302 — `ci/stability.sh 10` at the frozen code head

## Launch

Recorded in one shell call at launch time (2026-08-28 22:21 UTC):

```
$ cd /workspace && git rev-parse HEAD
d0e36e4eb5448e1e3315756aecddea675d898c64
$ nohup sh -c 'timeout 7200 ci/stability.sh 10 > /run/ralphd/artifacts/stability-302.log 2>&1; s=$?;
      printf "=== ci/stability.sh 10 exit status: %s ... ===\n" "$s" >> /run/ralphd/artifacts/stability-302.log;
      printf "%s\n" "$s" > /run/ralphd/artifacts/stability-302.exitstatus' &
launched pid=25839
```

- Launch sha (`git rev-parse HEAD` recorded at launch): **`d0e36e4eb5448e1e3315756aecddea675d898c64`**
  (`d0e36e4`), clean tree, `== origin/main`.
- Launched 2026-08-28 22:21 UTC, all ten runs finished by 2026-08-28 23:27 UTC (~66 min wall).
  Started as a non-blocking background job and polled (`grep '^=== RUN' <log>`), never blocked on;
  nothing else was run in that iteration.
- The `sh -c` wrapper is the only deviation from the bare `nohup timeout 7200 ci/stability.sh 10 > … &`
  line: it exists so that `s=$?` reads **the script's own exit status in the same shell call,
  immediately after the command line that produced it, with no pipe** — the same discipline
  `ci/stability.sh` itself documents. The wrapper adds no pipe and does not touch the suite.

## Observed rate

Verbatim from `summary.log` (also the second-to-last line of the driver log,
`stability-302-driver.log`, a copy of `/run/ralphd/artifacts/stability-302.log`):

```
8/10 passed
```

## Script's own exit status

Captured, not derived — `s=$?` in the same shell call as the `timeout 7200 ci/stability.sh 10`
command line, before anything else ran:

```
$ cat /run/ralphd/artifacts/stability-302.exitstatus
1
```

and the same captured value appended to the run's own log (last line of
`stability-302-driver.log`):

```
=== ci/stability.sh 10 exit status: 1 (captured with 0 in the same shell call, immediately after the command line, no pipe) ===
```

**Exit status: 1.** (Wart in that line, recorded rather than edited away: the literal `$?` I put
inside the parenthetical was itself expanded by the shell — to `0`, the status of the preceding
`s=$?` assignment — so the parenthetical prints `0` where it meant to say "`$?`". The
authoritative captured value is the `%s`-substituted `1` at the start of the line and the `1`
in `stability-302.exitstatus`; it is consistent with `ci/stability.sh`'s own
`if [ "$fail" -gt 0 ]; then exit 1` and with `fail=2` below, but it was *captured*, not inferred
from the fail count.)

## Per-run results

| run | result | notes |
|-----|--------|-------|
| 1 | PASS (exit 0) | |
| 2 | **FAIL** (exit 1) | `TestSigwinchCountDistinguishesTwoFromThree`, see below |
| 3 | PASS (exit 0) | |
| 4 | PASS (exit 0) | |
| 5 | PASS (exit 0) | |
| 6 | PASS (exit 0) | |
| 7 | PASS (exit 0) | |
| 8 | PASS (exit 0) | |
| 9 | PASS (exit 0) | |
| 10 | **FAIL** (exit 1) | `TestDeckBinaryEmptyHelpAndQuitThroughPTY`, see below |

Full logs: `run-1.log` .. `run-10.log` in this directory (copied verbatim out of the script's
`mktemp` outdir, `/tmp/deck-stability.LAal98`, after the run completed). Combined log:
`summary.log`. Driver log with the captured exit status: `stability-302-driver.log`.
Frozen-tree proof: `frozen-tree-check.log`.

### `=== RUN 2: FAIL` — `docs/reports/phase3g-302-stability10/run-2.log`

- Failing test: **`TestSigwinchCountDistinguishesTwoFromThree`**, package
  `github.com/n-orlov/deck/features` (a Go test in that package, not a Gherkin scenario;
  no `.feature` scenario failed in this run).
- Error (`run-2.log`, at the tail of the `features` package output):
  `sigwinch_count_test.go:80: sigwinch count after 2 resizes = 1, want exactly 2`, then
  `FAIL github.com/n-orlov/deck/features 320.542s`.
- The Gherkin suite itself was green in that run: `run-2.log:4969` reads
  `311 scenarios (311 passed)`; the only other scenario tallies in the file are the harness
  self-tests' `1 scenarios (1 undefined)` and `1 scenarios (1 failed)` fixtures described below.
- Every other package in that run reported `ok` or `[no test files]`.

### `=== RUN 10: FAIL` — `docs/reports/phase3g-302-stability10/run-10.log`

- Failing test: **`TestDeckBinaryEmptyHelpAndQuitThroughPTY`**, package
  `github.com/n-orlov/deck/cmd/deck` (again a Go test, not a Gherkin scenario; the `features`
  package reported `ok` in this run).
- Error (`run-10.log`, first line): `main_test.go:500: released help missing "copies it into
  deck's own tmux buffer" through the real PTY:` followed by the captured help frame, which is
  truncated mid-line at `| drag over the preview      while interactive, selects text; relea`
  — i.e. the PTY capture ended before the asserted help line had been written.
- Every other package in that run reported `ok` or `[no test files]`.

Note on a failure marker that appears in **every** run, pass or fail: the `features` package
output always contains `Scenario: error binding` / `Given a failing step` with
`Error: deliberate step failure` against a `/tmp/TestGodogRejectsUndefinedAndFailedSteps…/
failure.feature`. That is the harness self-test `TestGodogRejectsUndefinedAndFailedSteps`
deliberately injecting an undefined/failed step to prove the harness labels such steps as
failures. It is not a product failure and is not counted above.

## Measured surface — exclusions and no-test packages

- `features/godog_test.go`'s `defaultTags` (line 16): `"~@real-agents && ~@nightly"` —
  scenarios tagged `@real-agents` or `@nightly` are excluded from all ten runs. That file is
  **unmodified by this task** (covered by `frozen-tree-check.log`'s empty `*.go` diff and empty
  `git status --porcelain` for the same globs).
- Three packages have no test files and report `[no test files]` in every run (see e.g.
  `run-1.log`): `internal/notify`, `internal/search`, `internal/unit`.

## Frozen-tree proof

`frozen-tree-check.log` (this directory) records, each with its exit status in the same shell
call: `git rev-parse HEAD` = `d0e36e4…`, an **empty** `git diff --stat d0e36e4..HEAD --
"*.go" "*.feature" "*.sh" "*.toml" go.mod go.sum`, and an **empty** `git status --porcelain`
for the same globs. The only change this task commits is documentation under
`docs/reports/phase3g-302-stability10/`, so the ten runs measured exactly the code tree at
`d0e36e4`.

## Relation to the superseded first measurement

An earlier measurement of this same task ran at `bdc1879` (the code tree of `d0e36e4` — the
only commit between them is `d0e36e4` itself, which is documentation only) and observed
**7/10**, but published a *derived* exit status instead of a captured one, which is why this
task was re-measured rather than repaired in prose. That first run's ten per-run logs,
`summary.log` and README remain in git history at commit `d0e36e4` under this same path, and
its driver log is kept at `/run/ralphd/artifacts/stability-302-attempt1.log`. Its three
failures were `attention_sort.feature:92` (collapsed-strip attention count), `crash.feature:46`
(timed out waiting for frame "starting") and `attach_scroll.feature:11` (frame changed after a
wheel-scroll gesture) — a **disjoint** set from this run's two, so across the twenty runs
measured at this code tree five distinct instabilities have now been observed.

## Disposition

This task is a measurement, not a fix. **8/10 is the honest result at `d0e36e4` and is
reported as-is**; the run was not repeated to improve the number, and per the standing rules
an 8/10 is not rounded up and is not evidence that any requirement is met. The two failures
above (plus the first measurement's three) are the raw input for whichever follow-up task is
licensed to touch test code for stability or to name a finding; note in particular that
re-baselining a SIGWINCH count is permitted only against a derivation written and committed
*before* the change, never read off this failure.
