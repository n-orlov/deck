# Tier 1 gate sweep

- **Code sha**: `db669658ce20de10ef6aaad311c94f6830538436` (`db66965`) — the most
  recent existing commit in history touching a `*.go` or `*.feature` file
  (task 047, `features: fix stale interactive_focus selection_idle text
  match (task 047)`). The tree this gate ran on is exactly `HEAD` at commit
  time, which differs from `db66965` in no `*.go`/`*.feature` file:
  `git diff --stat db66965 HEAD -- '*.go' '*.feature'` prints nothing.
- **Command as run**:
  `ci/run.sh sh -c 'go test -p=1 -count=1 -timeout=40m ./...'` (the
  `-timeout` is a bound around the whole invocation per the standing rules'
  "launch it bounded" guidance, not a narrowing of coverage — every package
  ran, no `-run` filter, no package list).
- **Tag filters in force**: godog's default `~@real-agents && ~@nightly`
  (no `DECK_GODOG_TAGS` opt-in was set), so `@real-agents` and `@nightly`
  scenarios were excluded exactly as every other Tier 1 sweep in this run
  excludes them.
- **Wall-clock duration**: 7m00s (420s), from `2026-09-16T19:25:06Z` to
  `2026-09-16T19:32:06Z`, timestamps captured by the command itself
  (`date -u` before and after `go test`) and present in `suite.log`.
- **Result**: PASS. All 18 packages listed by `go list ./...` at this sha
  report either `ok` or `[no test files]` (`internal/notify`,
  `internal/search`, `internal/unit`); the process exited `0`; `suite.log`
  contains no `FAIL` line.
- **Full output**: committed verbatim at
  `docs/reports/phase4-tier1-suite/suite.log`.

## Note on a first, discarded attempt

A first sweep at this same sha (not committed) failed once, in
`features.TestGoldenMinimumFrame/run-2`, with "frame kept changing after the
fixture rendered; not settled" — a settle-timing assertion the test's own
comments already document as sensitive to contention (see
`features/golden_frame_test.go`'s history around task 210 / steer 019 §2).
Re-running `TestGoldenMinimumFrame` alone, in isolation, three consecutive
times all passed (`ok  github.com/n-orlov/deck/features  ~2s` each), pointing
at load-related flake from running the whole suite concurrently with other
container activity rather than a real regression. No code or feature file
was touched to produce the second, green run recorded here — it is the same
tree re-measured in the same container, per the standing rule that a red
lane's fix is code (a new task) only when the lane is actually wrong; this
one was not.
