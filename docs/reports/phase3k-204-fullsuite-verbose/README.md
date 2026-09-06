# Phase 3k task 204 — verbose companion run, Gherkin scenario/step tally (review cure)

## Why this exists (finding F34) — tally source only, not the deliverable sweep

`ci/run.sh go test -p=1 -count=1 ./...` (task 203's whole-suite gate, re-run at the new final
code sha after the review cure) never prints the godog scenario/step tally. Per phase 3g finding
F34: with `features/godog_test.go:35`'s `godog.Options.TestingT` set to the enclosing `*testing.T`,
Go's own `-v`-gated output-buffering behaviour (not anything godog does) discards a passing
package's stdout/stderr/`t.Log` entirely under the mandated non-verbose launcher. The tally is
readable only from a companion run whose command differs by adding `-v`.

**This companion exists only because the non-verbose launcher cannot print the tally.** It
publishes nothing else and gates nothing: task 203's whole-suite gate
(`docs/reports/phase3k-203-fullsuite/`) remains the deliverable gate for approach 02; this run
supersedes nothing about it. Same two-sweep discipline as `docs/reports/phase3k-023-fullsuite-verbose/`
(the equivalent companion for approach 01), now re-run because this review-cure approach
re-establishes both gates at a new final code sha.

## Code sha this run ran at, and the unchanged final code sha

The final code sha (`git log -1 --format=%H -- '*.go' '*.feature'`), the same sha task 201/202
landed and task 203's whole-suite gate cites:

```
4e09f2de90dcde04bd8fc20c77097e593f2fee5b
```

This run itself was made at (current `HEAD`/`origin/main` while the run executed, before this
task's own docs-only commit lands):

```
b36eb91480156145880deafec57298402f2f446a
```

`git status --porcelain` was empty and `git rev-parse HEAD origin/main` agreed
(`b36eb91480156145880deafec57298402f2f446a` both) while this run executed, and
`git diff --stat 4e09f2de90dcde04bd8fc20c77097e593f2fee5b..b36eb91480156145880deafec57298402f2f446a`
touches only `docs/reports/phase3k-203-fullsuite/{README.md,fullsuite.log,fullsuite.log.exitstatus}`
— docs-only, nothing under `*.go` or `*.feature` — so `b36eb91` is a docs-only descendant of the
final code sha `4e09f2d`, exactly as this task requires. `git log --oneline 4e09f2d..HEAD -- '*.go' '*.feature'`
prints nothing, confirmed again immediately before this report was written.

## Command run (verbatim)

```
ci/run.sh go test -count=1 -v ./features/
```

(This task's own success criteria specify this exact command — no `-p=1`, path `./features/`
rather than `./features/...` — differing cosmetically from task 023's companion; both select the
same single `features` package.)

## Execution shape

Launched exactly once, backgrounded under `timeout 3600`, never blocked on, and polled with
`sleep` only:

```
nohup sh -c 'timeout 3600 ci/run.sh go test -count=1 -v ./features/ > docs/reports/phase3k-204-fullsuite-verbose/verbose.log 2>&1; echo $? > docs/reports/phase3k-204-fullsuite-verbose/verbose.log.exitstatus' >/dev/null 2>&1 &
```

Started 2026-09-06T01:48:38Z. Polled with `sleep 60`, `sleep 120`, `sleep 120`, `sleep 90`; the
log grew each poll (1107, 3474, 5589 lines) until the fourth poll found `verbose.log.exitstatus`
present alongside a complete 6296-line log. Total wall time about 6 minutes, far under the 3600s
bound, consistent with the `features` package's own measured ~332s run.

## Exit status

`verbose.log.exitstatus` in this directory contains:

```
0
```

`verbose.log` is the full, unedited stdout+stderr of the run (6296 lines, raw ANSI colour codes
included — nothing stripped, nothing truncated).

## Gherkin tally — quoted verbatim from verbose.log

`verbose.log` lines 5819–5820 are the tally this report publishes. They are quoted below
**byte for byte, including the raw ANSI ESC (0x1b) bytes godog wrote** — this block is the
unmodified output of `sed -n '5819,5820p' verbose.log`, so a viewer that interprets escapes
will render `335 passed` / `3888 passed` in green, and a byte comparison against the log
succeeds:

```
335 scenarios ([32m335 passed[0m)
3888 steps ([32m3888 passed[0m)
```

Proof that the two lines above are byte-identical to the log's own (each `grep -aF` searches
`README.md` for the exact bytes of the corresponding log line, ESC bytes included):

```
$ grep -c -aF "$(sed -n 5819p verbose.log)" README.md
1
$ grep -c -aF "$(sed -n 5820p verbose.log)" README.md
1
```

The same two lines with every byte made visible (`cat -v` renders ESC as `^[`), so the escape
bytes are legible in a plain reader as well — a rendering of the bytes above, not a second
quotation:

```
$ sed -n '5819,5820p' verbose.log | cat -v
335 scenarios (^[[32m335 passed^[[0m)
3888 steps (^[[32m3888 passed^[[0m)
```

**Which lines are the suite's own tally, and which are the documented passing negative
self-test** — all six tally-shaped lines in the log, ESC bytes stripped here purely so the line
numbers are readable (this listing is an index, the verbatim quotation is the block above):

```
$ grep -a -n -E '^[0-9]+ scenarios \(|^[0-9]+ steps \(' verbose.log | sed $'s/\x1b\\[[0-9;]*m//g'
5819:335 scenarios (335 passed)
5820:3888 steps (3888 passed)
6163:1 scenarios (1 undefined)
6164:1 steps (1 undefined)
6188:1 scenarios (1 failed)
6189:1 steps (1 failed)
```

The first pair (lines 5819–5820) is the suite's own feature run (`TestFeatures`) and is the tally
this report publishes. The remaining four lines (6163–6164, 6188–6189 — the "1 undefined" and
"1 failed" pairs) belong to `TestGodogRejectsUndefinedAndFailedSteps`, a self-test of the godog
runner that deliberately feeds it one undefined step and one failing step to prove both are
rejected; each subtest prints its own one-scenario tally as fixture output, and the outer test
itself passes:

```
$ sed -n '6191,6193p' verbose.log
--- PASS: TestGodogRejectsUndefinedAndFailedSteps (0.00s)
    --- PASS: TestGodogRejectsUndefinedAndFailedSteps/undefined (0.00s)
    --- PASS: TestGodogRejectsUndefinedAndFailedSteps/failed (0.00s)
```

Publishing the numbers exactly as measured: **335 scenarios (335 passed)**, **3888 steps (3888
passed)** — unchanged from the phase 3k task 023 tally (also 335/3888), consistent with this
approach adding no new feature scenarios: tasks 201/202 changed only `internal/service/availability.go`
and added one Go unit test, no `.feature` file.

## Package result line and skip disposition

```
$ grep -aE '^(ok|FAIL|\?)' verbose.log
ok  	github.com/n-orlov/deck/features	331.939s
```

One `ok`, no `FAIL`, exactly the one package this run targeted. One `--- SKIP` line, unrelated
to the tally:

```
$ grep -a -n -e '^--- SKIP' verbose.log
6206:--- SKIP: TestI1KeystrokeDropReproduction (0.00s)
```

(`TestI1KeystrokeDropReproduction` is an opt-in reproduction driver gated on `DECK_I1_REPRO=1`,
pre-existing and out of this plan's scope — the same disposition every earlier verbose companion
recorded.)

## Confirmation nothing was narrowed

```
$ grep -c -e ' -run ' -e DECK_GODOG_PATHS verbose.log
0
```

No `-run`, no `DECK_GODOG_PATHS` anywhere in the command or the log.

## Outcome

Exit status **0**. Gherkin tally as measured: **335 scenarios (335 passed)**, **3888 steps
(3888 passed)**, at log lines 5819 and 5820. The one package this run targeted (`features`) is
`ok`; nothing failed.

This run is the Gherkin tally source only, and exists only because the mandated non-verbose
launcher structurally cannot print `N scenarios (N passed)` (finding F34). Task 203's whole-suite
gate (`docs/reports/phase3k-203-fullsuite/README.md`, exit status 0, at final code sha
`4e09f2de90dcde04bd8fc20c77097e593f2fee5b`) is what this approach's termination rule cites; this
companion adds only the scenario/step count that sweep's own log cannot contain.

## Contents

- `verbose.log` — the full, unedited stdout+stderr of `ci/run.sh go test -count=1 -v ./features/`.
- `verbose.log.exitstatus` — the captured `$?` of that run (`0`).
- this `README.md`.
