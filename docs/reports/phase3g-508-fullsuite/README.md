# Phase 3g — task 508: one whole-suite sweep at the final code sha, every exclusion named

## Exercised sha

`8f8e214acdda005b716b70b5fbab7789c97489af` (`docs: re-measure ci/stability.sh
10 in a run-only iteration so finding 1's 10/10 rests on the mandated
execution shape (task 507)`) — `git rev-parse HEAD` captured in the same shell
call immediately before launching the sweep, and again immediately after this
report's own log files were copied into place, both printing the identical
sha. `git status --short` was clean before the launch (nothing to disturb the
tree the suite compiled).

Proof the tree this report's own commit lands on is unchanged from the
exercised sha in every pattern that matters, run **after** this commit:

```
$ git diff 8f8e214..HEAD -- '*.go' '*.feature' '*.sh' '*.toml' go.mod go.sum
(empty)
```

## `features/godog_test.go`'s `defaultTags` — untouched since the run's own base

```
$ git log --oneline 1cfbd5a..HEAD -- features/godog_test.go
6718823 tmux: reclaim a leaked interactive pipe at the next start (task 030)
904419c features: prove R76's repair through the field's own hook route, not a keypress (task 002)

$ git log -p 1cfbd5a..HEAD -- features/godog_test.go | grep -c defaultTags
0
```

Both commits are approach-01 ancestors that only add a
`registerXSteps(sc)` call to `initializeScenario`; neither touches the
`defaultTags` line. It reads `"~@real-agents && ~@nightly"` at HEAD exactly as
it always has (`features/godog_test.go:16`).

## The whole-suite run (the deliverable)

Launched exactly as prescribed — backgrounded, never blocked on, polled with
`sleep`+`tail`/`cat` only:

```
$ git rev-parse HEAD
8f8e214acdda005b716b70b5fbab7789c97489af
$ nohup sh -c 'timeout 2400 ci/run.sh go test -p=1 -count=1 ./... > /run/ralphd/artifacts/suite-508.log 2>&1; s=$?; printf "%s\n" "$s" > /run/ralphd/artifacts/suite-508.exitstatus' &
```

No `-run` narrowing, no `DECK_GODOG_PATHS`/`DECK_GODOG_TAGS` override. Wall
time was ~6 min 20 s from launch to the exit-status file appearing (well
inside the 8–11 min budget); `features` alone took 310.0 s.

**Captured exit status: `0`** — read straight from
`/run/ralphd/artifacts/suite-508.exitstatus`, written by the same `sh -c`
invocation that ran the command, never inferred from the log's absence of
`FAIL`.

Full, unedited log, committed as
[`full-suite.log`](full-suite.log), reproduced here in full (17 lines, one
per package):

```
ok  	github.com/n-orlov/deck/cmd/deck	7.338s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.786s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.774s
ok  	github.com/n-orlov/deck/features	310.015s
ok  	github.com/n-orlov/deck/internal/agent	0.004s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.025s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.012s
ok  	github.com/n-orlov/deck/internal/interactive	10.866s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	4.135s
ok  	github.com/n-orlov/deck/internal/store	2.358s
ok  	github.com/n-orlov/deck/internal/theme	0.003s
ok  	github.com/n-orlov/deck/internal/tmux	19.321s
ok  	github.com/n-orlov/deck/internal/tui	1.136s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

14 packages report `ok`; 3 report `[no test files]`. Nothing reports `FAIL`.
This is the **one and only** whole-suite invocation made at `8f8e214` for
this task — no run was discarded or repeated.

## Every exclusion and skip, named

1. **Tag exclusions** — `features/godog_test.go`'s `defaultTags` is
   `"~@real-agents && ~@nightly"` (line 16, quoted above), the one authority
   for suite coverage; this run did not touch it or override it with any
   `DECK_GODOG_TAGS`/`DECK_GODOG_PATHS` env var.
2. **Three `[no test files]` packages** — `internal/notify`, `internal/search`
   and `internal/unit`, all visible as `?` lines in the log above. None of the
   three has ever had a test file; this is not new to this run.
3. **The harness self-test's deliberate injected step failure** —
   `features/godog_test.go:145-169`'s `TestGodogRejectsUndefinedAndFailedSteps`
   builds a throwaway godog suite with a step registered to always return
   `errors.New("deliberate step failure")`, runs it, and asserts godog's own
   `suite.Run()` returns nonzero. It is *designed* to observe a godog-reported
   failure on every run (visible as a `Failed steps:` block whenever this
   package is run with `-v`, e.g.
   `docs/reports/phase3g-112-stability10/README.md:250-259`); its own outer Go
   test always passes, and it is not a product failure. It ran silently as
   part of the `features` package's own `ok` line above
   (`features/godog_test.go` lives in package `features`), because this
   deliverable run has no `-v` and `go test` suppresses stdout on a passing
   package.

## Gherkin scenario tally

`go test` without `-v` suppresses per-scenario/per-step detail on a passing
package (as it did in the deliverable log above), so the tally is surfaced by
a second, non-deliverable, `-v` invocation of the **same** selection (same
`defaultTags`, no override, `-run TestFeatures` selects the same and only Go
test function `go test ./features/...` would run regardless — it does not
narrow which scenarios execute):

```
$ ci/run.sh go test -v ./features/ -run TestFeatures -count=1
...
311 scenarios (311 passed)
3523 steps (3523 passed)
4m52.068946945s
--- PASS: TestFeatures (292.09s)
PASS
ok  	github.com/n-orlov/deck/features	292.413s
```

**311 scenarios, 311 passed, 0 failed** (3523 steps, 3523 passed) — meets the
≥311-scenario, all-passed bar. Full tail (scenario/step totals through the
final `--- PASS`/`ok` lines, ANSI colour stripped) committed as
[`features-scenario-total-tail.log`](features-scenario-total-tail.log).

## Toolchain versions

Queried in the same image the suite ran in, immediately after the run:

```
$ ci/run.sh go version
go version go1.25.13 linux/amd64
$ ci/run.sh tmux -V
tmux 3.5a
```

## Net

Whole suite green (exit 0) at `8f8e214`, 17/17 package lines `ok` or
`[no test files]`, 311/311 Gherkin scenarios passed, every exclusion and skip
named above, no narrowing used for the deliverable run itself.
