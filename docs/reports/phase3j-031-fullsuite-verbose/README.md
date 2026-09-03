# Phase 3j task 031 — verbose companion sweep, Gherkin scenario/step tally

## Why this exists (finding F34) — tally source only, not the deliverable sweep

`ci/run.sh go test -p=1 -count=1 ./...` — task 030's (refreshed by task 058's) deliverable sweep —
never prints the godog scenario/step tally. Per phase 3g finding F34: with
`features/godog_test.go:35`'s `godog.Options.TestingT` set to the enclosing `*testing.T`, Go's own
`-v`-gated output-buffering behaviour (not anything godog does) discards a passing package's
stdout/stderr/`t.Log` entirely under the mandated non-verbose launcher, for any duration, any tree,
any amount of re-running. The tally is readable only from a companion run whose sole command-line
difference is `-v`.

This report is **that companion only** — it exists solely to publish the Gherkin tally that the
mandated non-verbose launcher structurally cannot print. **Task 058's sweep remains the
deliverable gate**; this verbose run supersedes nothing about it and is not itself the gate. This
follows the exact two-sweep discipline established by finding F34 and every later phase's own
`-verbose` report (e.g. `phase3h-203-fullsuite-verbose/`, `phase3i-128-fullsuite-verbose/`): never
fix (the launcher and `features/godog_test.go` are unedited, per standing rules), always disclose
via a second, docs-only companion sweep.

## Supersedes

This refresh **supersedes the earlier tally published at code sha
`a44ee320b93186496d56364836b0aed00a6f1e0b`** (the previous refresh of this same directory, task
031 per operator ruling `003-031`). Task 058 re-ran the deliverable sweep at the new final code
sha `b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7` (tasks 041–046 landed between the two shas); this
companion is re-run at that same sha to keep the tally current with the deliverable gate. This
directory is refreshed **in place**; there is no new numbered report directory.

## Code sha this run corresponds to

```
$ git log -1 --format=%H -- '*.go' '*.feature'
b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7
```

This is task 058's own final code sha (its refreshed report at
`docs/reports/phase3j-030-fullsuite/README.md` cites the same sha). `git status --porcelain` was
empty and `git rev-parse HEAD origin/main` agreed (`b82e3de49e1540d59c56adfad89957d044c403d1`
both) immediately before this sweep started; `b82e3de` is itself a docs-only descendant of
`b29afb8` (task 058's own docs-only publish commit) — nothing code-side (`*.go`/`*.feature`) has
moved between the two. The only change since is this report directory itself (untracked until
this row's own commit, which is likewise docs-only).

## Command run (verbatim; only difference from task 058's sweep is `-v`)

```
ci/run.sh go test -p=1 -count=1 -v ./...
```

## Execution shape

Launched exactly as the standing rules mandate, backgrounded under `timeout 2400` and never
blocked on, polled EXCLUSIVELY with `sleep 60` loops (checking only for the exit-status file's
existence/content, never reading progress through a pipe; no sleep longer than 60s anywhere in the
polling loop):

```
nohup sh -c 'timeout 2400 ci/run.sh go test -p=1 -count=1 -v ./... > /tmp/verbose-059.log 2>&1; echo $? > /tmp/verbose-059.log.exitstatus' >/dev/null 2>&1 &
```

Started 2026-09-03T20:05:58Z. Polled with `sleep 60` at 20:07:03Z, 20:08:05Z, 20:09:07Z,
20:10:10Z, 20:11:13Z (log growing each time, no exit-status file yet), then found the exit-status
file present on the poll at 20:13:29Z (file's own mtime: 20:12). Total wall time about 6m30s, well
under the 40-minute `timeout` and consistent with task 058's non-verbose sweep at ~6 minutes plus
`-v`'s output overhead (the `features` package alone took 338.333s / ~5m38s here).

## Exit status

`verbose.log.exitstatus` contains:

```
0
```

`verbose.log` in this directory is the full, unedited stdout+stderr of the run (9961 lines,
including raw ANSI colour codes godog emits — nothing stripped, nothing truncated).

## Gherkin tally — quoted verbatim with line numbers in the log

```
$ grep -a -n -E '^[0-9]+ scenarios \(|^[0-9]+ steps \(' verbose.log
5842:330 scenarios ([32m330 passed[0m)
5843:3824 steps ([32m3824 passed[0m)
6181:1 scenarios ([33m1 undefined[0m)
6182:1 steps ([33m1 undefined[0m)
6206:1 scenarios ([31m1 failed[0m)
6207:1 steps ([31m1 failed[0m)
```

The two tallies at lines **5842-5843** are the suite's own feature run (`TestFeatures`), ANSI
codes stripped for readability here:

```
5842:330 scenarios (330 passed)
5843:3824 steps (3824 passed)
```

Publishing the numbers exactly as measured, not an expected pair: **330 scenarios (330 passed)**,
**3824 steps (3824 passed)**. (Grown by one step from the superseded `a44ee32` tally's 330
scenarios / 3823 steps: `features/teardown_hooks.feature` gained one step assertion in task 044,
commit `52e529b`, "features: assert failing teardown hook's toast text" — same scenario count,
one more step in the existing scenario.)

The remaining four tally lines (6181-6182, 6206-6207) belong to
`TestGodogRejectsUndefinedAndFailedSteps`, a self-test of the godog runner itself that
deliberately feeds it one undefined step and one failing step to prove they're rejected; each of
its two subtests prints its own tiny godog tally (`1 scenarios (1 undefined)`, `1 scenarios (1
failed)`). Those four lines are fixture output, not the suite's own tally, and the outer test is
itself `--- PASS`:

```
$ sed -n '6209,6211p' verbose.log
--- PASS: TestGodogRejectsUndefinedAndFailedSteps (0.00s)
    --- PASS: TestGodogRejectsUndefinedAndFailedSteps/undefined (0.00s)
    --- PASS: TestGodogRejectsUndefinedAndFailedSteps/failed (0.00s)
```

## Package result lines (verbatim), matching task 058's non-verbose sweep

```
$ grep -aE '^(ok|FAIL|\?)' verbose.log
ok  	github.com/n-orlov/deck/cmd/deck	7.435s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.794s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.767s
ok  	github.com/n-orlov/deck/features	338.333s
ok  	github.com/n-orlov/deck/internal/agent	0.004s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.023s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.193s
ok  	github.com/n-orlov/deck/internal/interactive	10.990s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	6.530s
ok  	github.com/n-orlov/deck/internal/store	2.547s
ok  	github.com/n-orlov/deck/internal/theme	0.006s
ok  	github.com/n-orlov/deck/internal/tmux	19.672s
ok  	github.com/n-orlov/deck/internal/tui	3.561s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

17 package result lines (`grep -acE '^(ok|FAIL|\?)' verbose.log` → 17), matching task 058's
non-verbose sweep exactly: same 15 `ok` packages, same 3 `[no test files]` packages
(`internal/notify`, `internal/search`, `internal/unit`). No `FAIL` line anywhere in the log. One
`--- SKIP` line, unrelated to the tally, pre-existing and out of scope for this plan:

```
$ grep -a -n -e '^--- SKIP' verbose.log
6224:--- SKIP: TestI1KeystrokeDropReproduction (0.00s)
```

(`TestI1KeystrokeDropReproduction` is an opt-in reproduction driver gated on `DECK_I1_REPRO=1`,
pre-existing, not part of this plan — same disposition every earlier verbose companion recorded,
including task 058's own non-verbose sweep report which found no skip markers at all under its
non-verbose launcher because `-v`-gated output is exactly what F34 says is suppressed there.)

## Confirmation nothing was narrowed

```
$ grep -c -e ' -run ' -e DECK_GODOG_PATHS verbose.log
0
```

No `-run`, no `DECK_GODOG_PATHS` anywhere in the command or the log — the full, un-narrowed
`./...` shape, exactly as task 058's own sweep and the standing rules require.

## Outcome

Exit status **0**. Gherkin tally as measured: **330 scenarios (330 passed)**, **3824 steps (3824
passed)** — at log lines 5842 and 5843 respectively. All 17 packages `ok` or `[no test files]`;
nothing failed.

**This run is the Gherkin tally source only** (phase 3g finding F34): the mandated non-verbose
launcher can never print `N scenarios (N passed)` for any tree or any duration, so the tally is
read from this `-v` companion instead. **Task 058's sweep remains the deliverable gate** — its own
`docs/reports/phase3j-030-fullsuite/` report, captured at the same code sha
(`b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7`) with exit status 0 and all 15 testable packages `ok`,
is the gate this plan's termination rule cites; this report adds only the scenario/step count that
sweep's own log cannot contain. This refresh supersedes the earlier tally published at code sha
`a44ee320b93186496d56364836b0aed00a6f1e0b`.

## Contents

- `verbose.log` — the full, unedited stdout+stderr of `ci/run.sh go test -p=1 -count=1 -v ./...`.
- `verbose.log.exitstatus` — the captured `$?` of that run (`0`).
- this `README.md`.
