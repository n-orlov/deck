# Phase 3j task 031 — verbose companion sweep, Gherkin scenario/step tally

## Why this exists (finding F34) — tally source only, not the deliverable sweep

`ci/run.sh go test -p=1 -count=1 ./...` — task 030's deliverable sweep — never prints the godog
scenario/step tally. Per phase 3g finding F34: with `features/godog_test.go:35`'s
`godog.Options.TestingT` set to the enclosing `*testing.T`, Go's own `-v`-gated output-buffering
behaviour (not anything godog does) discards a passing package's stdout/stderr/`t.Log` entirely
under the mandated non-verbose launcher, for any duration, any tree, any amount of re-running. The
tally is readable only from a companion run whose sole command-line difference is `-v`.

This report is **that companion only** — it exists solely to publish the Gherkin tally that the
mandated non-verbose launcher structurally cannot print. **Task 030's non-verbose sweep at
`fbbda8f6aea2243e2c1f312bf346f14044a82210` remains the deliverable sweep**; this verbose run
supersedes nothing about it and is not itself the gate. This follows the exact two-sweep
discipline established by finding F34 and every later phase's own `-verbose` report (e.g.
`phase3h-203-fullsuite-verbose/`, `phase3i-128-fullsuite-verbose/`): never fix (the launcher and
`features/godog_test.go` are unedited, per standing rules), always disclose via a second,
docs-only companion sweep.

## Code sha this run corresponds to

```
$ git log -1 --format=%H -- '*.go' '*.feature'
fbbda8f6aea2243e2c1f312bf346f14044a82210
```

Identical to task 030's own final code sha — nothing code-side (`*.go`/`*.feature`) has moved
since task 030's sweep. This report's own tree is a **docs-only descendant** of that sha: `HEAD`
at the time this run started was `2ad633d99fd74bf4ecdc08b456a7e2cfaf496641` (task 030's own
docs-only publish commit), and the diff back to the code sha touches no code path:

```
$ git diff --stat fbbda8f6aea2243e2c1f312bf346f14044a82210..2ad633d99fd74bf4ecdc08b456a7e2cfaf496641 -- '*.go' '*.feature'
(empty)
```

`git status --porcelain` was empty and `git rev-parse HEAD origin/main` agreed
(`2ad633d99fd74bf4ecdc08b456a7e2cfaf496641` both) immediately before this sweep started; the only
change since is this report directory itself (untracked until this row's own commit, which is
likewise docs-only).

## Command run (verbatim; only difference from task 030's sweep is `-v`)

```
ci/run.sh go test -p=1 -count=1 -v ./...
```

## Execution shape

Launched exactly as the standing rules mandate, backgrounded under `timeout 1800` and never
blocked on, polled with `sleep 60`/`sleep 180` loops (checking only for the exit-status file's
existence/content, never reading progress through a pipe):

```
nohup sh -c 'timeout 1800 ci/run.sh go test -p=1 -count=1 -v ./... > /tmp/sweep-031v.log 2>&1; echo $? > /tmp/sweep-031v.log.exitstatus' >/dev/null 2>&1 &
```

Started 2026-09-03T12:40:31Z, observed complete on the poll after 2026-09-03T12:45:40Z (well
under the 30-minute `timeout`, consistent with task 030's non-verbose sweep at ~6 minutes plus
`-v`'s output overhead — the `features` package alone took 325.559s / ~5m26s here).

## Exit status

`verbose.log.exitstatus` contains:

```
0
```

`verbose.log` in this directory is the full, unedited stdout+stderr of the run (9804 lines,
including raw ANSI colour codes godog emits — nothing stripped, nothing truncated).

## Gherkin tally — quoted verbatim with line numbers in the log

```
$ grep -a -n -E '^[0-9]+ scenarios \(|^[0-9]+ steps \(' verbose.log
5745:326 scenarios ([32m326 passed[0m)
5746:3752 steps ([32m3752 passed[0m)
6080:1 scenarios ([33m1 undefined[0m)
6081:1 steps ([33m1 undefined[0m)
6105:1 scenarios ([31m1 failed[0m)
6106:1 steps ([31m1 failed[0m)
```

The two tallies at lines **5745-5746** are the suite's own feature run (`TestFeatures`), ANSI
codes stripped for readability here:

```
5745:326 scenarios (326 passed)
5746:3752 steps (3752 passed)
```

Publishing the numbers exactly as measured, not an expected pair: **326 scenarios (326 passed)**,
**3752 steps (3752 passed)**.

The remaining four tally lines (6080-6081, 6105-6106) belong to
`TestGodogRejectsUndefinedAndFailedSteps`, a self-test of the godog runner itself that
deliberately feeds it one undefined step and one failing step to prove they're rejected; each of
its two subtests prints its own tiny godog tally (`1 scenarios (1 undefined)`, `1 scenarios (1
failed)`). Those four lines are fixture output, not the suite's own tally, and the outer test is
itself `--- PASS`:

```
$ sed -n '6108,6112p' verbose.log
--- PASS: TestGodogRejectsUndefinedAndFailedSteps (0.00s)
    --- PASS: TestGodogRejectsUndefinedAndFailedSteps/undefined (0.00s)
    --- PASS: TestGodogRejectsUndefinedAndFailedSteps/failed (0.00s)
```

## Package result lines (verbatim), matching task 030's non-verbose sweep

```
$ grep -aE '^(ok|FAIL|\?)' verbose.log
ok  	github.com/n-orlov/deck/cmd/deck	7.254s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.788s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.769s
ok  	github.com/n-orlov/deck/features	325.559s
ok  	github.com/n-orlov/deck/internal/agent	0.004s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.026s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.224s
ok  	github.com/n-orlov/deck/internal/interactive	10.979s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	5.623s
ok  	github.com/n-orlov/deck/internal/store	2.653s
ok  	github.com/n-orlov/deck/internal/theme	0.006s
ok  	github.com/n-orlov/deck/internal/tmux	19.596s
ok  	github.com/n-orlov/deck/internal/tui	3.733s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

17 package result lines (`grep -acE '^(ok|FAIL|\?)' verbose.log` → 17), matching task 030's
non-verbose sweep exactly: same 15 `ok` packages, same 3 `[no test files]` packages
(`internal/notify`, `internal/search`, `internal/unit`). No `FAIL` line anywhere in the log. One
`--- SKIP` line, unrelated to the tally, pre-existing and out of scope for this plan:

```
$ grep -a -n -e '^--- SKIP' verbose.log
6123:--- SKIP: TestI1KeystrokeDropReproduction (0.00s)
```

(`TestI1KeystrokeDropReproduction` is an opt-in reproduction driver gated on `DECK_I1_REPRO=1`,
pre-existing, not part of this plan — same disposition every earlier verbose companion recorded,
including task 030's own non-verbose sweep report which found no skip markers at all under its
non-verbose launcher because `-v`-gated output is exactly what F34 says is suppressed there.)

## Confirmation nothing was narrowed

```
$ grep -c -e ' -run ' -e DECK_GODOG_PATHS verbose.log
0
```

No `-run`, no `DECK_GODOG_PATHS` anywhere in the command or the log — the full, un-narrowed
`./...` shape, exactly as task 030's own sweep and the standing rules require.

## Outcome

Exit status **0**. Gherkin tally as measured: **326 scenarios (326 passed)**, **3752 steps (3752
passed)** — at log lines 5745 and 5746 respectively. All 17 packages `ok` or `[no test files]`;
nothing failed.

**This run is the Gherkin tally source only** (phase 3g finding F34): the mandated non-verbose
launcher can never print `N scenarios (N passed)` for any tree or any duration, so the tally is
read from this `-v` companion instead. **Task 030 remains the deliverable sweep** — its own
`docs/reports/phase3j-030-fullsuite/` report, captured at the same code sha
(`fbbda8f6aea2243e2c1f312bf346f14044a82210`) with exit status 0 and all 15 testable packages `ok`,
is the gate this plan's termination rule cites; this report adds only the scenario/step count that
sweep's own log cannot contain.

## Contents

- `verbose.log` — the full, unedited stdout+stderr of `ci/run.sh go test -p=1 -count=1 -v ./...`.
- `verbose.log.exitstatus` — the captured `$?` of that run (`0`).
- this `README.md`.
