# Phase 1 report

This file accumulates Phase 1 evidence task by task (extended, not replaced,
by later tasks — see task 033 for the full requirement-by-requirement report).

## @real-agents smoke test against a real Claude CLI (task 029)

`features/real_agent_smoke.feature` proves, against an actually-installed
`claude` binary (not the `fake-claude` fixture used by every other
scenario), that deck assigns a UUID conversation id at session-create time
and passes that same id back on `--resume`. It is tagged `@real-agents`, so
it is excluded from the default suite by `features/godog_test.go`'s
`defaultTags = "~@real-agents && ~@nightly"` — the default run's scenario
count is unaffected whether or not a real `claude` is installed, and passes
with none installed (verified in this environment, which has no `claude` on
PATH).

To run it on a machine with a real Claude CLI on `PATH` (and, if the
installed CLI requires it, valid Claude credentials/config already set up
for that CLI to run non-interactively), run exactly this command from the
repository root:

```sh
DECK_GODOG_TAGS=@real-agents go test -run TestFeatures -v ./features/...
```

`features/godog_test.go`'s `TestFeatures` reads the `DECK_GODOG_TAGS`
environment variable and, when set, uses it as the Godog tag expression
instead of the hardcoded `defaultTags` (`~@real-agents && ~@nightly`);
`defaultTags` itself, and therefore every ordinary `go test ./...` run, is
unchanged.
