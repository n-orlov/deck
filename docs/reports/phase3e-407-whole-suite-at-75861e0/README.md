# Task 407 — green whole-suite run at the final code commit

## Sha under test

`75861e0533bf6ea77bdfe4e72b25fa1566ab33e1` (`75861e0`) — the tree's tip at
the time this task ran, and the last commit that touched non-docs code
(task 411's `locateText`/`locatePreviewText` fix). `git diff 668a94c HEAD
--name-only` shows only `docs/reports/...` paths plus the three
`features/*.go` files task 411 changed; nothing under `internal/`, `cmd/`
or `features/*.feature` changed between `668a94c` and `75861e0` besides
task 411's own fix. `git cat-file -e 75861e0533bf6ea77bdfe4e72b25fa1566ab33e1`
succeeds.

This redoes task 407's own prior attempt, which was correctly blocked: that
attempt found the whole-suite run failing deterministically at `668a94c`
because of the `locateText` regression task 411 later fixed (see task 407's
own `notes` field in `tasks.json` for the three failing-attempt logs at
`668a94c`, kept as history under
`/run/ralphd/artifacts/task407-repro/` — not committed to the repo, per the
"never chain two whole-suite runs" budget rule; those were exploratory runs
against a commit this report does NOT cite).

## Command and result

```
ci/run.sh go test -p=1 -count=1 ./...
```

Run once this iteration (whole-suite budget), backgrounded and polled per
the standing rule. Loadavg samples taken with `uptime` immediately before
launch and at two points during the run: `3.25` (start) → `2.40` (mid) →
`3.16` (near end, features package still running) → `2.82` (just after
completion). Wall clock: launched at `06:16:42Z`, `features` package
finished at ~`06:21:42Z` (300.143s reported by `go test` itself), full
run done by `06:25:54Z`.

Exit code: `0`. Full raw log committed at
[`go-test-p1-count1-all.log`](go-test-p1-count1-all.log) — 17 lines, every
line `ok` or `[no test files]`:

```
ok  	github.com/n-orlov/deck/cmd/deck	5.049s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.793s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.772s
ok  	github.com/n-orlov/deck/features	300.143s
ok  	github.com/n-orlov/deck/internal/agent	0.004s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.029s
ok  	github.com/n-orlov/deck/internal/hookrecv	3.537s
ok  	github.com/n-orlov/deck/internal/interactive	10.628s
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	2.894s
ok  	github.com/n-orlov/deck/internal/store	1.716s
ok  	github.com/n-orlov/deck/internal/theme	0.005s
ok  	github.com/n-orlov/deck/internal/tmux	19.130s
ok  	github.com/n-orlov/deck/internal/tui	0.521s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

No `sleep`, no skip, no `@flaky`/`@wip`, no scenario deleted or retagged to
reach this result — this is the tree exactly as task 411 left it.

## Conclusion

The whole-suite run is green at `75861e0`, superseding the stale `7ebafce`
citation in `docs/reports/phase3e.md`'s whole-suite section (that citation
predates tasks 401–411 entirely). `docs/reports/phase3e.md` is updated to
cite this sha and this log path.
