# Phase 3j task 031 — verbose companion sweep, Gherkin scenario/step tally

## Why this exists (finding F34) — tally source only, not the deliverable sweep

`ci/run.sh go test -p=1 -count=1 ./...` — task 030's deliverable sweep, refreshed by task 058 —
never prints the godog scenario/step tally. Per phase 3g finding F34: with
`features/godog_test.go:35`'s `godog.Options.TestingT` set to the enclosing `*testing.T`, Go's own
`-v`-gated output-buffering behaviour (not anything godog does) discards a passing package's
stdout/stderr/`t.Log` entirely under the mandated non-verbose launcher, for any duration, any tree,
any amount of re-running. The tally is readable only from a companion run whose sole command-line
difference is `-v`.

**This companion exists only because the non-verbose launcher cannot print the tally (finding
F34).** It publishes nothing else and gates nothing: **task 058's sweep remains the deliverable
gate**, and this verbose run supersedes nothing about it. That is the two-sweep discipline F34
established and every later phase's own `-verbose` report follows (e.g.
`docs/reports/phase3h-203-fullsuite-verbose/`, `docs/reports/phase3i-128-fullsuite-verbose/`):
never fix (the launcher and `features/godog_test.go` are unedited, per standing rules), always
disclose via a second, docs-only companion sweep.

## Supersedes

This refresh **supersedes the tally published at code sha
`a44ee320b93186496d56364836b0aed00a6f1e0b`** (the earlier state of this same directory, task 031
per operator ruling 003-031); tasks 041–046 landed between that sha and the current code sha, so
that tally is stale. An intermediate refresh of this directory was committed as
`8bba297625a5cc8411d37a6f80232d8eb1067265` at the same code sha as this one, but its quoted tally
lines dropped the ESC bytes godog emits and were therefore not verbatim; this run's captures and
the byte-exact quoting below replace it. This directory is refreshed **in place** — there is no
new numbered report directory.

## Code sha this run ran at

```
$ git log -1 --format=%H -- '*.go' '*.feature'
b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7
```

That is task 058's own final code sha (its refreshed report at
`docs/reports/phase3j-030-fullsuite/README.md` cites the same sha), so this tally is current with
the deliverable gate. `git status --porcelain` was empty and `git rev-parse HEAD origin/main`
agreed (`8bba297625a5cc8411d37a6f80232d8eb1067265` both) while this sweep ran; `8bba297` is a
docs-only descendant of `b29afb8` — nothing under `*.go` or `*.feature` moved between them.

## Command run (verbatim; only difference from task 058's sweep is `-v`)

```
ci/run.sh go test -p=1 -count=1 -v ./...
```

## Execution shape

Launched exactly once, backgrounded under `timeout 2400`, never blocked on, and polled
EXCLUSIVELY with `sleep 60` — no other interval anywhere, and no inspection of the log or the
exit-status file before the first `sleep 60`:

```
nohup sh -c 'timeout 2400 ci/run.sh go test -p=1 -count=1 -v ./... > /tmp/verbose-059.log 2>&1; echo $? > /tmp/verbose-059.log.exitstatus' >/dev/null 2>&1 &
```

Started 2026-09-03T20:16:17Z. The launch call inspected nothing; the first look at the output came
after a `sleep 60`, and every later look after another `sleep 60` (seven polls in all, at
~20:17Z through ~20:23Z, each showing only the log's size growing until the seventh found
`/tmp/verbose-059.log.exitstatus` present at 2026-09-03T20:23Z). Total wall time about 7m30s, far
under the 40-minute `timeout`, consistent with task 058's ~6-minute non-verbose sweep plus `-v`'s
output overhead (the `features` package alone took 363.749s / ~6m04s here).

## Exit status

`verbose.log.exitstatus` in this directory contains:

```
0
```

`verbose.log` is the full, unedited stdout+stderr of the run (9961 lines, raw ANSI colour
codes included — nothing stripped, nothing truncated).

## Gherkin tally — quoted verbatim from verbose.log

Byte-exact copy of the tally lines, ESC bytes and all (12 ESC bytes across the six lines;
your pager or browser may render the colour codes rather than show them):

```
$ grep -a -E '^[0-9]+ scenarios \(|^[0-9]+ steps \(' verbose.log
330 scenarios ([32m330 passed[0m)
3824 steps ([32m3824 passed[0m)
1 scenarios ([33m1 undefined[0m)
1 steps ([33m1 undefined[0m)
1 scenarios ([31m1 failed[0m)
1 steps ([31m1 failed[0m)
```

The same six lines with their log line numbers (`grep -a -n`), still byte-exact after the
`NNN:` prefix:

```
5842:330 scenarios ([32m330 passed[0m)
5843:3824 steps ([32m3824 passed[0m)
6181:1 scenarios ([33m1 undefined[0m)
6182:1 steps ([33m1 undefined[0m)
6206:1 scenarios ([31m1 failed[0m)
6207:1 steps ([31m1 failed[0m)
```

The suite's own feature run (`TestFeatures`) is the first pair, at log lines 5842–5843. With the
ANSI escapes stripped for readability — this block is a transcription, not the verbatim quote
above:

```
330 scenarios (330 passed)
3824 steps (3824 passed)
```

Publishing the numbers exactly as measured, not an expected pair: **330 scenarios (330 passed)**,
**3824 steps (3824 passed)**. (One step more than the superseded `a44ee32` tally's 330 scenarios /
3823 steps: `features/teardown_hooks.feature` gained one step assertion in commit `52e529b`,
"features: assert failing teardown hook's toast text" — same scenario count, one more step inside
an existing scenario.)

The remaining four tally lines (6181–6182, 6206–6207) belong to
`TestGodogRejectsUndefinedAndFailedSteps`, a self-test of the godog runner that deliberately feeds
it one undefined step and one failing step to prove both are rejected; each subtest prints its own
one-scenario tally. Those are fixture output, not the suite's tally, and the outer test passes:

```
$ sed -n '6209,6211p' verbose.log
--- PASS: TestGodogRejectsUndefinedAndFailedSteps (0.00s)
    --- PASS: TestGodogRejectsUndefinedAndFailedSteps/undefined (0.00s)
    --- PASS: TestGodogRejectsUndefinedAndFailedSteps/failed (0.00s)
```

## Package result lines (verbatim), matching task 058's non-verbose sweep

```
$ grep -aE '^(ok|FAIL|\?)' verbose.log
ok  	github.com/n-orlov/deck/cmd/deck	7.492s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.790s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.770s
ok  	github.com/n-orlov/deck/features	363.749s
ok  	github.com/n-orlov/deck/internal/agent	0.005s
ok  	github.com/n-orlov/deck/internal/audit	0.021s
ok  	github.com/n-orlov/deck/internal/config	0.032s
ok  	github.com/n-orlov/deck/internal/hookrecv	5.656s
ok  	github.com/n-orlov/deck/internal/interactive	13.268s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	6.374s
ok  	github.com/n-orlov/deck/internal/store	2.538s
ok  	github.com/n-orlov/deck/internal/theme	0.004s
ok  	github.com/n-orlov/deck/internal/tmux	19.527s
ok  	github.com/n-orlov/deck/internal/tui	3.453s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

17 result lines in all (`grep -acE '^(ok|FAIL|\?)' verbose.log` → 17): **14** `ok` packages
(`grep -ac '^ok' verbose.log` → 14) and **3** `[no test files]` packages
(`grep -ac 'no test files' verbose.log` → 3: `internal/notify`, `internal/search`,
`internal/unit`), the same 14 + 3 split task 058's refreshed non-verbose sweep report publishes.
No `FAIL` line anywhere in the log (`grep -ac FAIL verbose.log` → 0). One `--- SKIP` line,
unrelated to the tally:

```
$ grep -a -n -e '^--- SKIP' verbose.log
6224:--- SKIP: TestI1KeystrokeDropReproduction (0.00s)
```

(`TestI1KeystrokeDropReproduction` is an opt-in reproduction driver gated on `DECK_I1_REPRO=1`,
pre-existing and out of this plan's scope — the same disposition every earlier verbose companion
recorded. Task 058's non-verbose sweep sees no skip marker at all, because `-v`-gated output is
exactly what F34 says is suppressed there.)

## Confirmation nothing was narrowed

```
$ grep -c -e ' -run ' -e DECK_GODOG_PATHS verbose.log
0
```

No `-run`, no `DECK_GODOG_PATHS` in the command or the log — the full, un-narrowed `./...` shape,
the deliverable command plus `-v` and nothing else.

## Outcome

Exit status **0**. Gherkin tally as measured: **330 scenarios (330 passed)**, **3824 steps (3824
passed)**, at log lines 5842 and 5843. All 17 package result lines are `ok` or `[no test files]`;
nothing failed.

This run is the Gherkin tally source only, and exists only because the mandated non-verbose
launcher structurally cannot print `N scenarios (N passed)` (finding F34). **Task 058's sweep
remains the deliverable gate** — its report at `docs/reports/phase3j-030-fullsuite/README.md`,
captured at this same code sha `b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7` with exit status 0, is
what this plan's termination rule cites; this companion adds only the scenario/step count that
sweep's own log cannot contain. It supersedes the tally previously published here at code sha
`a44ee320b93186496d56364836b0aed00a6f1e0b`.

## Contents

- `verbose.log` — the full, unedited stdout+stderr of `ci/run.sh go test -p=1 -count=1 -v ./...`.
- `verbose.log.exitstatus` — the captured `$?` of that run (`0`).
- this `README.md`.
