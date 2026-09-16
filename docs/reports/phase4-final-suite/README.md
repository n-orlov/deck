# Final gate sweep

- **Code sha**: `db669658ce20de10ef6aaad311c94f6830538436` (`db66965`) — the
  most recent existing commit in history touching a `*.go` or `*.feature`
  file (task 047, `features: fix stale interactive_focus selection_idle
  text match (task 047)`). It is unchanged since the Tier 1 sweep (task
  027) because Tier 2 was not started (task 028's decision) and every
  intervening task (029–038) landed no code. The tree this gate ran on is
  exactly `HEAD` at commit time, which differs from `db66965` in no
  `*.go`/`*.feature` file: `git diff --stat db66965 HEAD -- '*.go'
  '*.feature'` prints nothing.
- **Command as run**:
  `ci/run.sh sh -c 'go test -p=1 -count=1 -timeout=40m ./...'` (the
  `-timeout` is a bound around the whole invocation per the standing rules'
  "launch it bounded" guidance, not a narrowing of coverage — every
  package ran, no `-run` filter, no package list).
- **Skips in force**:
  - godog's default tag filter, `~@real-agents && ~@nightly` (no
    `DECK_GODOG_TAGS` opt-in was set), excluding `@real-agents` and
    `@nightly` scenarios exactly as every earlier sweep in this run.
  - The real-binary skips baked into the fixture PATH / agent probes for
    codex, claude and pi: scenarios and probes that require a genuine
    `codex`, `claude` or `pi` executable are skipped in this container in
    favor of the `fake-codex`/`fake-claude`/`fake-pi` fixtures — no real
    agent binaries are installed here.
- **Wall-clock duration**: 7m01s (421s), from `2026-09-16T19:55:07Z` to
  `2026-09-16T20:02:08Z`, timestamps captured by the command itself
  (`date -u` before and after `go test`) and present in `suite.log`.
- **Result**: PASS. All 18 packages listed by `go list ./...` at this sha
  report either `ok` or `[no test files]` (`internal/notify`,
  `internal/search`, `internal/unit`); the process exited `0`; `suite.log`
  contains no `FAIL` line.
- **Full output**: committed verbatim at
  `docs/reports/phase4-final-suite/suite.log`.
