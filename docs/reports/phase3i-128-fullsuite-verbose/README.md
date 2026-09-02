# Task 128 — verbose companion sweep, Gherkin scenario/step tally

## Why this exists (finding F34)

`ci/run.sh go test -p=1 -count=1 ./...` (task 127's non-verbose deliverable sweep) never prints
the godog scenario/step tally. Per phase 3g finding F34: with `features/godog_test.go:35`'s
`godog.Options.TestingT` set to the enclosing `*testing.T`, Go's own `-v`-gated output-buffering
behaviour (not anything godog does) discards a passing package's stdout/stderr/`t.Log` entirely
under the mandated non-verbose launcher, for any duration, any tree, any amount of re-running.
The tally is readable only from a companion run whose sole command-line difference is `-v`. This
report is that companion for task 127's sweep, following the exact two-sweep discipline finding
F34 and every later phase's own `-verbose` report (e.g. `phase3h-203-fullsuite-verbose/`) already
established: never fix (the launcher/test file are unedited, per standing rules), always
disclose via a second, docs-only sweep.

## Final code sha this sweep ran at

```
$ git log -1 --format=%H -- '*.go' '*.feature'
b9243a1f415ba9ca77cc2ffa2ec557ca8b1be4cb
```

Identical to task 127's own final code sha (`b9243a1`) — nothing code-side moved between the two
sweeps. Proof this sweep's tree is a docs-only descendant of it — no `*.go`/`*.feature` path
changed between task 127's sha and this report's own tree:

```
$ git diff --stat b9243a1f415ba9ca77cc2ffa2ec557ca8b1be4cb..HEAD -- '*.go' '*.feature'
(empty)
```

`git status --porcelain` was clean and `git rev-parse HEAD origin/main` agreed
(`0c022e8846cb0056c9019c6f6629fd004ab028c1` both) immediately before this sweep started; the only
change since is this report directory itself (untracked until this row's commit).

## Command run (verbatim; only difference from task 127's sweep is `-v`)

```
ci/run.sh go test -p=1 -count=1 -v ./...
```

## Execution shape

Launched exactly as the standing rules mandate, backgrounded and never blocked on, never piped
into `tee`:

```
nohup sh -c 'cd /workspace && timeout 2400 ci/run.sh go test -p=1 -count=1 -v ./... > verbose.log 2>&1; echo $? > verbose.log.exitstatus' >/dev/null 2>&1 &
```

Polled with `sleep 60` / `sleep 180` loops, checking only for `verbose.log.exitstatus`'s
existence/content (never reading progress through a pipe). Started 2026-09-02T13:05:52Z, observed
complete on the poll after 2026-09-02T13:15:52Z (~9-10 minutes total, consistent with task 127's
non-verbose sweep at ≈7m19s plus `-v`'s output overhead — the `features` package alone took
327.222s / ~5m27s, matching task 127's ~5m26s for the same package).

## Exit status

`verbose.log.exitstatus` contains:

```
0
```

`verbose.log` in this directory is the full, unedited stdout+stderr of the run (1,275,560 bytes,
9638 lines, including raw ANSI colour codes godog emits — nothing stripped, nothing truncated).

## Gherkin tally — quoted verbatim with line numbers in the log

```
$ grep -a -n -E '^[0-9]+ scenarios \(|^[0-9]+ steps \(' verbose.log
5642:319 scenarios ([32m319 passed[0m)
5643:3682 steps ([32m3682 passed[0m)
5970:1 scenarios ([33m1 undefined[0m)
5971:1 steps ([33m1 undefined[0m)
5995:1 scenarios ([31m1 failed[0m)
5996:1 steps ([31m1 failed[0m)
```

The two tallies at lines **5642-5643** are the suite's own feature run (`TestFeatures`), ANSI
codes stripped for readability here:

```
5642:319 scenarios (319 passed)
5643:3682 steps (3682 passed)
```

Publishing the numbers exactly as measured, not an expected pair: **319 scenarios (319 passed)**,
**3682 steps (3682 passed)**. (Higher than phase 3h's `311 scenarios (311 passed)` / `3532 steps
(3532 passed)` because this phase's tasks — including 122 and 126 — added feature scenarios since
then; not a discrepancy.)

The remaining four tally lines (5970-5971, 5995-5996) belong to
`TestGodogRejectsUndefinedAndFailedSteps`, a self-test of the godog runner itself that
deliberately feeds it one undefined step and one failing step to prove they're rejected; each of
its two subtests prints its own tiny godog tally (`1 scenarios (1 undefined)`, `1 scenarios (1
failed)`). Those four lines are fixture output, not the suite's own tally, and the outer test is
itself `--- PASS`:

```
$ sed -n '5993,5997p' verbose.log
--- PASS: TestGodogRejectsUndefinedAndFailedSteps (0.00s)
    --- PASS: TestGodogRejectsUndefinedAndFailedSteps/undefined (0.00s)
    --- PASS: TestGodogRejectsUndefinedAndFailedSteps/failed (0.00s)
```

## Package result lines (verbatim), matching task 127's non-verbose sweep

```
$ grep -aE '^(ok|FAIL|\?)' verbose.log
ok  	github.com/n-orlov/deck/cmd/deck	7.498s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.788s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.768s
ok  	github.com/n-orlov/deck/features	327.222s
ok  	github.com/n-orlov/deck/internal/agent	0.005s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.027s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.273s
ok  	github.com/n-orlov/deck/internal/interactive	11.072s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	4.411s
ok  	github.com/n-orlov/deck/internal/store	2.342s
ok  	github.com/n-orlov/deck/internal/theme	0.005s
ok  	github.com/n-orlov/deck/internal/tmux	19.517s
ok  	github.com/n-orlov/deck/internal/tui	3.560s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

17 package result lines (`grep -acE '^(ok|FAIL|\?)' verbose.log` → 17), matching task 127's
non-verbose sweep exactly (same 3 `[no test files]` packages: `internal/notify`,
`internal/search`, `internal/unit`). No `FAIL` line anywhere in the log. One `--- SKIP` line,
unrelated to the tally, pre-existing and out of scope for this plan:

```
$ grep -a -n -e '^--- SKIP' verbose.log
6013:--- SKIP: TestI1KeystrokeDropReproduction (0.00s)
```

(`TestI1KeystrokeDropReproduction` is an opt-in reproduction driver gated on `DECK_I1_REPRO=1`,
pre-existing, not part of this plan — same disposition task 127 and every earlier verbose
companion recorded.)

## Confirmation nothing was narrowed

```
$ grep -c -e ' -run ' -e DECK_GODOG_PATHS verbose.log
0
```

No `-run`, no `DECK_GODOG_PATHS` anywhere in the command or the log — the full, un-narrowed
`./...` shape, exactly as task 127's own sweep and the standing rules require.

## Outcome

Exit status **0**. Gherkin tally as measured: **319 scenarios (319 passed)**, **3682 steps (3682
passed)** — at log lines 5642 and 5643 respectively. All 17 packages `ok` or `[no test files]`;
nothing failed.

## Contents

- `verbose.log` — the full, unedited stdout+stderr of `ci/run.sh go test -p=1 -count=1 -v ./...`.
- `verbose.log.exitstatus` — the captured `$?` of that run (`0`).
- this `README.md`.
