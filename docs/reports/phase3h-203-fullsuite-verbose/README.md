# Task 203 — verbose companion sweep, Gherkin scenario/step tally

## Why this exists (finding F34)

`ci/run.sh go test -p=1 -count=1 ./...` (task 202's non-verbose deliverable sweep) never prints
the godog scenario/step tally — per finding F34 the non-verbose launcher cannot print it. This
report is the verbose companion, the ONLY command-line difference from task 202 being `-v`.

## Final code sha this sweep ran at

```
$ git log -1 --format=%H -- '*.go' '*.feature'
4b1d4dcbd4480013470a0555795e6c64db3bf96d
```

Task 201's code commit `4b1d4dc` is still the final code sha. Proof this sweep's tree is a
docs-only descendant of it — no `*.go`/`*.feature` path changed between task 201's sha and the
commit that publishes this report:

```
$ git diff --stat 4b1d4dc..HEAD -- '*.go' '*.feature'
(empty)
```

Task 202's docs-only sweep commit, published earlier at the same final code sha, is
`c9e88cb59832b0148af7540e4d107faa086c2542` ("docs: publish the whole-suite sweep at the new final
code sha 4b1d4dc (task 202)").

## Command run (verbatim; only difference from task 202 is `-v`)

```
ci/run.sh go test -p=1 -count=1 -v ./...
```

## Execution shape

Launched exactly as mandated, backgrounded and never blocked on, never piped into `tee`:

```
nohup sh -c 'timeout 2400 ci/run.sh go test -p=1 -count=1 -v ./... > <log> 2>&1; echo $? > <log>.exitstatus' >/dev/null 2>&1 &
```

Polled with `sleep 60` in a loop, checking only for the `.exitstatus` file's existence (never
reading progress through a pipe). Launched `2026-08-30T14:35:54Z`; the `.exitstatus` file
appeared by the 8th poll (~8 minutes later) — measured runtime **~7-8m**, consistent with task
009's historical `-v` measurement at the previous final code sha. The exit status was read from
that file, never from a pipe:

```
$ cat docs/reports/phase3h-203-fullsuite-verbose/.exitstatus
0
```

`verbose.log` in this directory is the full, unedited stdout+stderr of the run (1,227,287 bytes,
9327 lines, including the raw ANSI colour codes godog emits — nothing stripped, nothing
truncated).

## Gherkin tally — quoted verbatim with line numbers in the log

```
$ grep -a -n -E '^[0-9]+ scenarios \(|^[0-9]+ steps \(' docs/reports/phase3h-203-fullsuite-verbose/verbose.log
5445:311 scenarios ([32m311 passed[0m)
5446:3532 steps ([32m3532 passed[0m)
5765:1 scenarios ([33m1 undefined[0m)
5766:1 steps ([33m1 undefined[0m)
5790:1 scenarios ([31m1 failed[0m)
5791:1 steps ([31m1 failed[0m)
```

The two tallies at lines **5445-5446** are the suite's own feature run
(`TestFeatures`), ANSI codes stripped for readability here:

```
5445:311 scenarios (311 passed)
5446:3532 steps (3532 passed)
```

Publishing the numbers exactly as measured, not an expected pair: **311 scenarios (311 passed)**,
**3532 steps (3532 passed)**.

The remaining four tally lines (5765-5766, 5790-5791) belong to
`TestGodogRejectsUndefinedAndFailedSteps`, a self-test of the godog runner itself that
deliberately feeds it one undefined step and one failing step to prove they're rejected; each of
its two subtests prints its own tiny godog tally (`1 scenarios (1 undefined)`,
`1 scenarios (1 failed)`). Those four lines are fixture output, not the suite's own tally, and the
outer test is itself `--- PASS`:

```
$ sed -n '5806,5809p' docs/reports/phase3h-203-fullsuite-verbose/verbose.log
--- PASS: TestGodogRejectsUndefinedAndFailedSteps (0.00s)
    --- PASS: TestGodogRejectsUndefinedAndFailedSteps/undefined (0.00s)
    --- PASS: TestGodogRejectsUndefinedAndFailedSteps/failed (0.00s)
```

## Package result lines (verbatim), matching task 202's non-verbose sweep

```
$ grep -aE '^(ok|FAIL|\?)' docs/reports/phase3h-203-fullsuite-verbose/verbose.log
ok  	github.com/n-orlov/deck/cmd/deck	7.703s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.790s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.769s
ok  	github.com/n-orlov/deck/features	321.819s
ok  	github.com/n-orlov/deck/internal/agent	0.005s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.022s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.597s
ok  	github.com/n-orlov/deck/internal/interactive	11.279s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	4.364s
ok  	github.com/n-orlov/deck/internal/store	2.454s
ok  	github.com/n-orlov/deck/internal/theme	0.004s
ok  	github.com/n-orlov/deck/internal/tmux	19.636s
ok  	github.com/n-orlov/deck/internal/tui	1.477s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

17 package result lines (`grep -acE '^(ok|FAIL|\?)' verbose.log` → 17), matching task 202's
non-verbose sweep exactly. No `FAIL` line anywhere in the log. One `--- SKIP` line, unrelated to
the tally, pre-existing and out of scope for this plan:

```
$ grep -a -n -e '^--- SKIP' docs/reports/phase3h-203-fullsuite-verbose/verbose.log
5808:--- SKIP: TestI1KeystrokeDropReproduction (0.00s)
```

(`TestI1KeystrokeDropReproduction` is an opt-in reproduction driver gated on `DECK_I1_REPRO=1`,
pre-existing, not part of this plan.)

## Confirmation nothing was narrowed

```
$ grep -ac -e ' -run ' -e DECK_GODOG_PATHS docs/reports/phase3h-203-fullsuite-verbose/verbose.log
0
```

## Outcome

Exit status **0**. Gherkin tally as measured: **311 scenarios (311 passed)**, **3532 steps (3532
passed)** — at log lines 5445 and 5446 respectively. All 17 packages `ok` or `[no test files]`;
nothing failed.

## Contents

- `verbose.log` — the full, unedited stdout+stderr of `ci/run.sh go test -p=1 -count=1 -v ./...`.
- `.exitstatus` — the captured `$?` of that run (`0`).
- this `README.md`.

## This report's own commit

This report (the three files above) was published by commit `fc358c0` (full sha
`fc358c079ad263a0006437fb7592feef72c5fa2f` — resolve with `git log -1 --format=%H fc358c0` in a
clone of this history), subject "docs: publish the verbose companion sweep's Gherkin tally at
4b1d4dc (task 203)", the sole commit carrying the `(task 203)` marker. This present line is added
by an immediately-following, docs-only addendum commit that carries no `(task 203)` marker (per
the notes file's self-referential-sha convention: a commit cannot quote its own sha inside its own
content, so naming it requires a following commit).
