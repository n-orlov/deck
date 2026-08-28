# Task 111 — whole suite green at the final code commit

## Exercised code sha

`9f61e2143fef364bb89bda1a625454f4d60bb17c` — this is also the tree HEAD was at
when the runs below were executed, and it is the last commit that touches any
of `*.go`, `*.feature`, `*.toml`, `*.sh`, `go.mod`, `go.sum` as of this task's
own commit (this report is docs-only: `.md`/`.log` files under
`docs/reports/`).

Proof the exercised sha and HEAD are (still) the same tree for those patterns,
run **after** this report's own commit lands:

```
$ git diff --stat 9f61e2143fef364bb89bda1a625454f4d60bb17c HEAD -- '*.go' '*.feature' '*.toml' '*.sh' go.mod go.sum
(empty)
```

## `features/godog_test.go`'s `defaultTags` — untouched

```
$ git log --oneline 1cfbd5a..HEAD -- features/godog_test.go
6718823 tmux: reclaim a leaked interactive pipe at the next start (task 030)
904419c features: prove R76's repair through the field's own hook route, not a keypress (task 002)
```

Both commits (both approach-01, both ancestors of this run's own work) only
add a `registerXSteps(sc)` call to `initializeScenario`; neither touches the
`defaultTags` line. Confirmed directly — grepping the diff itself for the
token turns up nothing:

```
$ git log -p 1cfbd5a..HEAD -- features/godog_test.go | grep -c defaultTags
0
```

`defaultTags` is still `"~@real-agents && ~@nightly"` at HEAD, exactly as it
was at `b6cbbc7`.

## The whole-suite run

Command (no `-run` filter, no `DECK_*` env override):

```
ci/run.sh go test -p=1 -count=1 ./...
```

Run twice to get both the full package-level log and a reliably-captured
process exit code (the first run was backgrounded per the standing budget
rule and its shell-level exit status was unrecoverable across tool-call
boundaries; the second run, synchronous, is the one whose exit code is
quoted below — both runs' package output agrees line for line, module
durations aside). Full output of the second, authoritative run is committed
as [`full-suite.log`](full-suite.log); reproduced here in full:

```
ok  	github.com/n-orlov/deck/cmd/deck	7.436s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.784s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.767s
ok  	github.com/n-orlov/deck/features	316.596s
ok  	github.com/n-orlov/deck/internal/agent	0.005s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.024s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.116s
ok  	github.com/n-orlov/deck/internal/interactive	11.319s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	4.549s
ok  	github.com/n-orlov/deck/internal/store	2.473s
ok  	github.com/n-orlov/deck/internal/theme	0.004s
ok  	github.com/n-orlov/deck/internal/tmux	19.482s
ok  	github.com/n-orlov/deck/internal/tui	1.033s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
EXIT_CODE=0
```

Every package reports `ok` (or `[no test files]`); nothing reports `FAIL`.
`EXIT_CODE=0`, captured with `echo "EXIT_CODE=$?"` immediately after the
command in the same shell invocation — not inferred from the absence of a
`FAIL` line.

## Scenario total vs. `b6cbbc7`'s baseline

`b6cbbc7` (approach 02's pre-fix state of record) ran the same `defaultTags`
selection and had **3 failing scenarios** — the ones `docs/reports/phase3g-findings.md` calls F24/F25/F26, fixed respectively by task 101
(`2549406`), task 102 (`89682e5`) and task 103 (`51b7f17`).

At this task's exercised sha (`9f61e21`), the same selection (`defaultTags`,
untouched, no `DECK_GODOG_TAGS`/`DECK_GODOG_PATHS` override) runs
**311 scenarios, 311 passed, 0 failed** (3520 steps, 3520 passed):

```
$ ci/run.sh go test -v ./features/ -run TestFeatures -count=1
...
311 scenarios (311 passed)
3520 steps (3520 passed)
5m1.651188305s
--- PASS: TestFeatures (301.66s)
PASS
ok  	github.com/n-orlov/deck/features	302.045s
```

This second, `-v` invocation is not the deliverable whole-suite run (that's
the `go test -p=1 -count=1 ./...` run above, whose own log stays silent about
per-scenario detail because `go test` suppresses stdout on a passing package
unless `-v` is given) — it exists solely to surface godog's own scenario/step
tally, which is otherwise invisible on a green run. `-run TestFeatures`
selects the same (and only) Go test function `go test ./features/` would run
regardless; it does not change which scenarios execute, and no
`DECK_GODOG_TAGS`/`DECK_GODOG_PATHS` override was set. Its last ~330 lines
(the final scenario-level `--- PASS` breakdown plus the summary) are
committed as [`features-scenario-total-tail.log`](features-scenario-total-tail.log)
(ANSI colour codes stripped).

Net: 3 failing scenarios at `b6cbbc7` → 0 failing scenarios (311/311 green)
at `9f61e21`, whole suite (`./...`) exit 0.
