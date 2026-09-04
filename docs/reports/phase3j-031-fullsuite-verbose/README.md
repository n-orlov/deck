# Phase 3j task 031 — verbose companion sweep, Gherkin scenario/step tally

## Why this exists (finding F34) — tally source only, not the deliverable sweep

`ci/run.sh go test -p=1 -count=1 ./...` — task 030's deliverable sweep, refreshed by task 081 at
the current final code sha — never prints the godog scenario/step tally. Per phase 3g finding
F34: with `features/godog_test.go:35`'s `godog.Options.TestingT` set to the enclosing `*testing.T`,
Go's own `-v`-gated output-buffering behaviour (not anything godog does) discards a passing
package's stdout/stderr/`t.Log` entirely under the mandated non-verbose launcher, for any
duration, any tree, any amount of re-running. The tally is readable only from a companion run
whose sole command-line difference is `-v`.

**This companion exists only because the non-verbose launcher cannot print the tally (finding
F34).** It publishes nothing else and gates nothing: **task 081's sweep remains the deliverable
gate**, and this verbose run supersedes nothing about it. That is the two-sweep discipline F34
established and every later phase's own `-verbose` report follows (e.g.
`docs/reports/phase3h-203-fullsuite-verbose/`, `docs/reports/phase3i-128-fullsuite-verbose/`):
never fix (the launcher and `features/godog_test.go` are unedited, per standing rules), always
disclose via a second, docs-only companion sweep.

## Supersedes

This refresh **supersedes the tally previously published at code sha
`b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7`** (the earlier state of this same directory, task 082's
predecessor content at that sha); task 080 landed between that sha and the current code sha
(a comment-only correction in `features/launch_hooks.feature`, which nonetheless moves the final
code sha per the PRD's own "Termination" rule), so that tally is stale with respect to the
current final code sha even though the measured numbers below are unchanged. This directory is
refreshed **in place** — there is no new numbered report directory.

## Code sha this run ran at

```
$ git log -1 --format=%H -- '*.go' '*.feature'
4fbd452430501805a860dd229ddca1cd3f5c1cd6
```

That is task 080's own commit — the current final code sha per the PRD's "Termination" rule
(task 080 edited `features/launch_hooks.feature`, a comment-only change that still moves the
final code sha) — and task 081's refreshed sweep report at
`docs/reports/phase3j-030-fullsuite/README.md` cites the same sha, so this tally is current with
the deliverable gate. `git status --porcelain` was empty and `git rev-parse HEAD origin/main`
agreed (`680237e45b73b3666c81571e56065a41c9e81c0b` both) while this sweep ran; that commit (task
081, docs-only, touching only `docs/reports/phase3j-030-fullsuite/`) is a docs-only descendant of
`4fbd452430501805a860dd229ddca1cd3f5c1cd6` — nothing under `*.go` or `*.feature` moved between
them.

## Command run (verbatim; only difference from task 081's sweep is `-v`)

```
ci/run.sh go test -p=1 -count=1 -v ./...
```

## Execution shape

Launched exactly once, backgrounded under `timeout 2400`, never blocked on, and polled
EXCLUSIVELY with `sleep 60` — no other interval anywhere, and no inspection of the log or the
exit-status file before the first `sleep 60`:

```
nohup sh -c 'timeout 2400 ci/run.sh go test -p=1 -count=1 -v ./... > docs/reports/phase3j-031-fullsuite-verbose/verbose.log 2>&1; echo $? > docs/reports/phase3j-031-fullsuite-verbose/verbose.log.exitstatus' >/dev/null 2>&1 &
```

Started 2026-09-04T00:57Z. The launch call inspected nothing; the first look at the log came
after a `sleep 60`, and every later look after another `sleep 60` (seven polls in all, at
~00:58Z through ~01:03Z, each showing only the log's size growing — 1009, 2281, 3445, 4353,
5614, 8086 lines — until the seventh poll found `verbose.log.exitstatus` present, with an mtime
of 2026-09-04T01:03:05Z, and the log complete at 9961 lines). Total wall time about 6 minutes,
far under the 40-minute `timeout`, consistent with task 081's ~6-minute non-verbose sweep plus
`-v`'s output overhead (the `features` package alone took 338.853s / ~5m39s here).

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

**Which lines are the documented passing negative self-test, and which are the suite's own
tally:** the first pair (lines 5842–5843, "330 scenarios (330 passed)" / "3824 steps (3824
passed)") is the suite's own feature run (`TestFeatures`) and is the tally this report publishes.
The remaining four lines (6181–6182, 6206–6207 — the "1 undefined" and "1 failed" pairs) belong to
`TestGodogRejectsUndefinedAndFailedSteps`, a self-test of the godog runner that deliberately feeds
it one undefined step and one failing step to prove both are rejected; each subtest prints its own
one-scenario tally as fixture output, and the outer test itself **passes**:

```
$ sed -n '6209,6211p' verbose.log
--- PASS: TestGodogRejectsUndefinedAndFailedSteps (0.00s)
    --- PASS: TestGodogRejectsUndefinedAndFailedSteps/undefined (0.00s)
    --- PASS: TestGodogRejectsUndefinedAndFailedSteps/failed (0.00s)
```

With the ANSI escapes stripped for readability — this block is a transcription, not the verbatim
quote above:

```
330 scenarios (330 passed)
3824 steps (3824 passed)
```

Publishing the numbers exactly as measured, not an expected pair: **330 scenarios (330 passed)**,
**3824 steps (3824 passed)** — unchanged from the superseded `b29afb8` tally (task 080 touched
only a comment block in `features/launch_hooks.feature`, adding and removing no scenario or step).

## Package result lines (verbatim), matching task 081's non-verbose sweep

```
$ grep -aE '^(ok|FAIL|\?)' verbose.log
ok  	github.com/n-orlov/deck/cmd/deck	7.595s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.791s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.774s
ok  	github.com/n-orlov/deck/features	338.853s
ok  	github.com/n-orlov/deck/internal/agent	0.005s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.023s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.080s
ok  	github.com/n-orlov/deck/internal/interactive	11.116s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	6.371s
ok  	github.com/n-orlov/deck/internal/store	2.543s
ok  	github.com/n-orlov/deck/internal/theme	0.006s
ok  	github.com/n-orlov/deck/internal/tmux	19.610s
ok  	github.com/n-orlov/deck/internal/tui	3.634s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

17 result lines in all (`grep -acE '^(ok|FAIL|\?)' verbose.log` → 17): **14** `ok` packages
(`grep -ac '^ok' verbose.log` → 14) and **3** `[no test files]` packages
(`grep -ac 'no test files' verbose.log` → 3: `internal/notify`, `internal/search`,
`internal/unit`), the same 14 + 3 split task 081's refreshed non-verbose sweep report publishes.
No `FAIL` line anywhere in the log (`grep -ac FAIL verbose.log` → 0). One `--- SKIP` line,
unrelated to the tally:

```
$ grep -a -n -e '^--- SKIP' verbose.log
6224:--- SKIP: TestI1KeystrokeDropReproduction (0.00s)
```

(`TestI1KeystrokeDropReproduction` is an opt-in reproduction driver gated on `DECK_I1_REPRO=1`,
pre-existing and out of this plan's scope — the same disposition every earlier verbose companion
recorded. Task 081's non-verbose sweep sees no skip marker at all, because `-v`-gated output is
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
launcher structurally cannot print `N scenarios (N passed)` (finding F34). **Task 081's sweep
remains the deliverable gate** — its report at `docs/reports/phase3j-030-fullsuite/README.md`,
captured at this same code sha `4fbd452430501805a860dd229ddca1cd3f5c1cd6` with exit status 0, is
what this plan's termination rule cites; this companion adds only the scenario/step count that
sweep's own log cannot contain. It supersedes the tally previously published here at code sha
`b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7`.

## Contents

- `verbose.log` — the full, unedited stdout+stderr of `ci/run.sh go test -p=1 -count=1 -v ./...`.
- `verbose.log.exitstatus` — the captured `$?` of that run (`0`).
- this `README.md`.
