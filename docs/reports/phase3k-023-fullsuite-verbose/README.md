# Phase 3k task 023 — verbose companion run, Gherkin scenario/step tally

> **Superseded 2026-09-06 (task cure-02-02).** An independent review found two remaining probe
> gaps after this run (`lookPathIn` accepted a mode-0644 regular file and a FIFO named like an
> agent's binary); approach 02 cured both (`cure-01-01` `7349dd6`, `201` `c8b00cc`, `202`
> `4e09f2d`), moving the phase's final code sha to `4e09f2de90dcde04bd8fc20c77097e593f2fee5b`.
> This report's tally at `d88c6625c4ccca71b0d31f7b5864ba030ed39e53` is preserved below unchanged
> as history; it is **not** the gate of record. The current verbose-companion evidence is
> `docs/reports/phase3k-204-fullsuite-verbose/` (task 204, same tally, current final code sha).

## Why this exists (finding F34) — tally source only, not the deliverable sweep

`ci/run.sh go test -p=1 -count=1 ./...` — task 021's deliverable sweep — never prints the godog
scenario/step tally. Per phase 3g finding F34: with `features/godog_test.go:35`'s
`godog.Options.TestingT` set to the enclosing `*testing.T`, Go's own `-v`-gated output-buffering
behaviour (not anything godog does) discards a passing package's stdout/stderr/`t.Log` entirely
under the mandated non-verbose launcher, for any duration, any tree, any amount of re-running.
The tally is readable only from a companion run whose command differs by adding `-v`.

**This companion exists only because the non-verbose launcher cannot print the tally (finding
F34).** It publishes nothing else and gates nothing: **task 021's sweep remains the deliverable
gate** (validated, `docs/reports/phase3k-021-fullsuite/`), and this run supersedes nothing about
it. That is the two-sweep discipline F34 established and every later phase's own `-verbose`
report has followed (e.g. `docs/reports/phase3j-031-fullsuite-verbose/`,
`docs/reports/phase3i-303-fullsuite-verbose/`): never fix (the launcher and
`features/godog_test.go` are unedited, per standing rules), always disclose via a second,
docs-only companion run.

This run is scoped to `./features/...` — the one package that produces the Gherkin tally — per
this task's own wording ("a `-v` companion run of the **features package**"), rather than the
whole-repo `./...` some earlier phases' companions used; nothing in this task's success criteria
asks for the other 16 packages' result lines, only the scenario/step tally and the two shas.

## Code shas this run ran at (its final-code-sha claim superseded 2026-09-06, task 209)

The sha task 021's deliverable sweep and task 022's disposition README cite, which was the last
`*.go`/`*.feature` commit (`git log -1 --format=%H -- '*.go' '*.feature'`) **when this run was
made**. It is no longer: approach 02's cures `c8b00cc` (task 201) and `4e09f2d` (task 202) landed
`*.go` changes afterwards, so the phase's final code sha is now
`4e09f2de90dcde04bd8fc20c77097e593f2fee5b` and the current verbose companion is
`docs/reports/phase3k-204-fullsuite-verbose/` (whole-suite gate of record:
`docs/reports/phase3k-203-fullsuite/`):

```
d88c6625c4ccca71b0d31f7b5864ba030ed39e53
```

This run itself was made at:

```
28dc76129c6dd8548a72ca1d0b16c94ead29dd5a
```

`git status --porcelain` was empty and `git rev-parse HEAD origin/main` agreed
(`28dc76129c6dd8548a72ca1d0b16c94ead29dd5a` both) while this run executed, and
`git diff --stat d88c6625c4ccca71b0d31f7b5864ba030ed39e53..28dc76129c6dd8548a72ca1d0b16c94ead29dd5a`
touches only `docs/reports/phase3k-021-fullsuite/{README.md,sweep.log,sweep.log.exitstatus}` —
docs-only, nothing under `*.go` or `*.feature` — so `28dc761` is a docs-only descendant of
`d88c662`, which was the final code sha at the time this task ran, exactly as this task required.

## Command run (verbatim)

```
ci/run.sh go test -p=1 -count=1 -v ./features/...
```

## Execution shape

Launched exactly once, backgrounded under `timeout 1800`, never blocked on, and polled with
`sleep 60` only:

```
nohup sh -c 'timeout 1800 ci/run.sh go test -p=1 -count=1 -v ./features/... > docs/reports/phase3k-023-fullsuite-verbose/verbose.log 2>&1; echo $? > docs/reports/phase3k-023-fullsuite-verbose/verbose.log.exitstatus' >/dev/null 2>&1 &
```

Started 2026-09-05T22:02:02Z. Polled with `sleep 60` five times; the log grew each poll (1838,
3034, 4084, 5266 lines) until the fifth poll found `verbose.log.exitstatus` present alongside a
complete 6296-line log. Total wall time about 5 minutes, far under the 1800s bound, consistent
with the `features` package's own measured 336.938s.

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
passed)** — up from the phase 3j tally of 330 scenarios / 3824 steps, consistent with the 5
scenarios tasks 016–020 added to `features/agent_availability.feature` in this phase.

## Package result line and skip disposition

```
$ grep -aE '^(ok|FAIL|\?)' verbose.log
ok  	github.com/n-orlov/deck/features	336.938s
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
launcher structurally cannot print `N scenarios (N passed)` (finding F34). **Task 021's sweep
remains the deliverable gate of approach 01** — its report at
`docs/reports/phase3k-021-fullsuite/README.md`, captured with exit status 0 at
`d88c6625c4ccca71b0d31f7b5864ba030ed39e53`, the final code sha *at that time* and since superseded
by `4e09f2de90dcde04bd8fc20c77097e593f2fee5b` (current whole-suite gate of record:
`docs/reports/phase3k-203-fullsuite/`) — is what this plan's termination rule cited; this
companion adds only the scenario/step count that sweep's own log cannot contain.

## Contents

- `verbose.log` — the full, unedited stdout+stderr of `ci/run.sh go test -p=1 -count=1 -v ./features/...`.
- `verbose.log.exitstatus` — the captured `$?` of that run (`0`).
- this `README.md`.
