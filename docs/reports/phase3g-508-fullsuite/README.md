# Phase 3g — task 508: one whole-suite sweep at the final code sha, every exclusion named

## Why this report contains two sweeps

The first pass of this task ran the prescribed launcher verbatim
(`ci/run.sh go test -p=1 -count=1 ./...`, no `-v`) and then sourced the Gherkin
scenario tally from a *second*, `-run TestFeatures`-narrowed invocation. That
second invocation was the objection: a tally must be quoted from the sweep's own
log, and no `-run` narrowing may appear anywhere in the evidence.

The reason the first pass reached for a second invocation is a property of
`go test` itself, not a shortcut: **in package-list mode (`./...`), `go test`
discards a passing package's stdout, stderr and `t.Log` output unless `-v` is
given.** So the mandated non-verbose launcher can *never* emit godog's
`N scenarios (N passed)` summary, however long it runs. Proof, from a throwaway
two-package module built in a sibling of the same `deck-ci:local` image
(committed verbatim as
[`gotest-output-buffering-probe.log`](gotest-output-buffering-probe.log)):

```
$ go test -p=1 -count=1 ./...            # the mandated form
ok  	probe/a	0.002s
ok  	probe/b	0.002s
$ go test -p=1 -count=1 -v ./...         # same tree, same tests
=== RUN   TestA
STDOUT-TALLY: 311 scenarios (311 passed)
STDERR-TALLY: 311 scenarios (311 passed)
    a_test.go:3: TLOG-TALLY: 311 scenarios
--- PASS: TestA (0.00s)
```

`probe/a`'s test writes that line to stdout, to stderr and through `t.Log`, and
passes; none of the three survives the non-verbose run. This is why the two
requirements — "launched as `go test -p=1 -count=1 ./...`" and "the Gherkin
tally quoted from the log" — cannot both hold of a single log.

Resolution, without narrowing anything: **sweep B** below re-runs the whole
suite with the launcher's only change being `-v`, so one log carries the
per-package result lines *and* the Gherkin tally *and* the harness self-test's
injected failure. Sweep A (the verbatim launcher) is retained unchanged, since
its exit status is the one produced by the exact prescribed command line. Both
sweeps exercise the identical code tree (proof below); neither used `-run` and
neither used `DECK_GODOG_PATHS` or `DECK_GODOG_TAGS`.

## Exercised shas, and the code tree they share

* Sweep A: `8f8e214acdda005b716b70b5fbab7789c97489af`
* Sweep B: `550a265561ef6452cee06acb75b1b95ae8180812` (sweep A's report commit;
  docs-only on top of `8f8e214`) — captured with `git rev-parse HEAD` in the
  same shell call that launched the run, and committed as
  [`launch-sha-verbose.txt`](launch-sha-verbose.txt).

Both shas are docs-only descendants of the final *code* commit `b0a4e7d`, so
every sweep in this report compiled the same product and the same tests:

```
$ git diff 8f8e214..550a265 -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum
(empty)
$ git diff 550a265..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum
(empty)          # re-run after this report's own commit, which is docs-only
```

`git status --short` was clean before each launch, so nothing perturbed the
tree the suites compiled.

## `features/godog_test.go`'s `defaultTags` — untouched since the run's own base

```
$ git log --oneline 1cfbd5a..HEAD -- features/godog_test.go
6718823 tmux: reclaim a leaked interactive pipe at the next start (task 030)
904419c features: prove R76's repair through the field's own hook route, not a keypress (task 002)

$ git log -p 1cfbd5a..HEAD -- features/godog_test.go | grep -c defaultTags
0
```

Both commits are approach-01 ancestors that only add a `registerXSteps(sc)`
call to `initializeScenario`; neither touches the `defaultTags` line. It reads
`"~@real-agents && ~@nightly"` at HEAD exactly as it always has
(`features/godog_test.go:16`).

## Sweep A — the verbatim launcher (exit status of the exact prescribed line)

```
$ git rev-parse HEAD
8f8e214acdda005b716b70b5fbab7789c97489af
$ nohup sh -c 'timeout 2400 ci/run.sh go test -p=1 -count=1 ./... > /run/ralphd/artifacts/suite-508.log 2>&1; s=$?; printf "%s\n" "$s" > /run/ralphd/artifacts/suite-508.exitstatus' &
```

Backgrounded, never blocked on, polled with `sleep`+`tail` only. ~6 min 20 s
wall; `features` alone 310.0 s. **Captured exit status `0`**, read from
`/run/ralphd/artifacts/suite-508.exitstatus`, written by the same `sh -c` that
ran the command — never inferred from the absence of `FAIL`. Full unedited log:
[`full-suite.log`](full-suite.log) (17 lines, one per package, 14 `ok` + 3
`[no test files]`, no `FAIL`).

## Sweep B — the whole suite, verbose, one log with everything (the deliverable)

```
$ git rev-parse HEAD
550a265561ef6452cee06acb75b1b95ae8180812
$ nohup sh -c 'timeout 2400 ci/run.sh go test -p=1 -count=1 -v ./... > /run/ralphd/artifacts/suite-508b.log 2>&1; s=$?; printf "%s\n" "$s" > /run/ralphd/artifacts/suite-508b.exitstatus' &
```

Backgrounded and polled (one `sleep 420` then `wc -l`/`tail`), never blocked
on; ~7 min wall. Whole package list `./...`, **no `-run` narrowing, no
`DECK_GODOG_PATHS`, no `DECK_GODOG_TAGS`** — `-v` is the only difference from
sweep A's command line, and it changes what is *printed*, not what is *run*.

**Captured exit status `0`** —
[`full-suite-verbose.exitstatus`](full-suite-verbose.exitstatus), written by
the same `sh -c` invocation. Full unedited 9171-line log:
[`full-suite-verbose.log`](full-suite-verbose.log).

One line per Go package, from that log's tail:

```
ok  	github.com/n-orlov/deck/cmd/deck	7.754s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.792s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.768s
ok  	github.com/n-orlov/deck/features	306.592s
ok  	github.com/n-orlov/deck/internal/agent	0.003s
ok  	github.com/n-orlov/deck/internal/audit	0.019s
ok  	github.com/n-orlov/deck/internal/config	0.025s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.611s
ok  	github.com/n-orlov/deck/internal/interactive	11.154s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	4.551s
ok  	github.com/n-orlov/deck/internal/store	2.520s
ok  	github.com/n-orlov/deck/internal/theme	0.006s
ok  	github.com/n-orlov/deck/internal/tmux	19.373s
ok  	github.com/n-orlov/deck/internal/tui	1.132s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

14 packages `ok`, 3 `[no test files]`, 17/17 accounted for, nothing `FAIL`
(`grep -nE '^\s*--- FAIL|^FAIL' full-suite-verbose.log` matches nothing).

### Gherkin scenario tally, quoted from that same log

`full-suite-verbose.log:5423-5426` (ANSI colour stripped for legibility; the
committed log keeps the escapes):

```
311 scenarios (311 passed)
3523 steps (3523 passed)
4m50.088026723s
--- PASS: TestFeatures (290.10s)
```

**311 scenarios, 311 passed, 0 failed; 3523 steps, 3523 passed** — meets the
≥311-scenario, all-passed bar, from the sweep's own log with no narrowing
anywhere. The per-scenario `--- PASS: TestFeatures/...` lines follow it for all
311. [`features-scenario-total-tail.log`](features-scenario-total-tail.log) is
the first pass's tail of the same totals, kept as history; the authoritative
tally is the one above.

## Every exclusion and skip in these runs, named

1. **Tag exclusions** — `features/godog_test.go:16`'s `defaultTags` is
   `"~@real-agents && ~@nightly"`: `@real-agents` scenarios (they would drive a
   real `claude`/`pi` binary) and `@nightly` scenarios are deliberately not
   selected. `defaultTags` is the one authority for suite coverage; neither
   sweep edited it or overrode it via `DECK_GODOG_TAGS`.
2. **Three `[no test files]` packages** — `internal/notify`, `internal/search`
   and `internal/unit`, the three `?` lines above. None has ever carried a test
   file; nothing about these runs changed that.
3. **The harness self-test's deliberate injected step failure** —
   `TestGodogRejectsUndefinedAndFailedSteps` (`features/godog_test.go:145-169`)
   builds throwaway godog suites, one with an unregistered step and one with a
   step returning `errors.New("deliberate step failure")`, and asserts godog
   reports both. Sweep B's `-v` makes it visible in the log itself
   (`full-suite-verbose.log:5738-5773`, colour stripped):

   ```
   === RUN   TestGodogRejectsUndefinedAndFailedSteps/undefined
   1 scenarios (1 undefined)
   1 steps (1 undefined)
   === RUN   TestGodogRejectsUndefinedAndFailedSteps/failed
   --- Failed steps:
     Scenario: error binding # /tmp/TestGodogRejectsUndefinedAndFailedStepsfailed3265389831/001/failure.feature:2
       Given a failing step # .../failure.feature:3
         Error: deliberate step failure
   1 scenarios (1 failed)
   1 steps (1 failed)
   --- PASS: TestGodogRejectsUndefinedAndFailedSteps (0.00s)
   ```

   Those `1 undefined` / `1 failed` tallies belong to the two nested throwaway
   suites, not to the 311-scenario product suite, and the outer Go test
   **passes** — it is asserting that godog refuses bad steps. It is not a
   product failure and appears in every run.
4. **The one Go `--- SKIP` in the whole sweep** —
   `full-suite-verbose.log:5784-5786`:

   ```
   === RUN   TestI1KeystrokeDropReproduction
       i1_repro_test.go:37: opt-in reproduction driver for I-1 (task 004); set DECK_I1_REPRO=1 to run. See docs/reports/phase3d-i1-repro.log for a committed run.
   --- SKIP: TestI1KeystrokeDropReproduction (0.00s)
   ```

   An opt-in phase-3d reproduction driver, gated on `DECK_I1_REPRO=1`, with a
   committed run of its own at `docs/reports/phase3d-i1-repro.log`. It is the
   *only* skipped Go test in the sweep (`grep -c '^\s*--- SKIP'` = 1) and the
   only skip beyond the tag exclusions above.

## Toolchain versions

Queried in the same image the suites ran in:

```
$ ci/run.sh go version
go version go1.25.13 linux/amd64
$ ci/run.sh tmux -V
tmux 3.5a
```

## Net

Whole suite green at the final code tree (`b0a4e7d`'s code, exercised at
`8f8e214` and `550a265`): captured exit status `0` from both the verbatim
launcher and the verbose whole-suite sweep, 17/17 package lines `ok` or
`[no test files]`, **311/311 Gherkin scenarios and 3523/3523 steps passed**
quoted from the sweep's own log, and every exclusion named — the two
`defaultTags` tag filters, the three test-free packages, the harness
self-test's deliberate injected failure, and the single opt-in `--- SKIP`.
No `-run` narrowing and no `DECK_GODOG_PATHS`/`DECK_GODOG_TAGS` override was
used by either sweep.
