# Full-suite gate sweep — approach 2, re-run at the corrected tail sha (task 011b)

- **Tail code sha**: `e93a7902582672d799dadfa8bbaa4f1f25eb28dd` (`e93a790`,
  `tui: sidebar row's permission badge follows SPEC §11's non-safe rule
  (task 011)`) — unchanged from the sha task 013 cited. The intervening
  commits (`425c9dc` Tier 2 record, `059704a` task 013's own red-lane
  report, `1f38195` this task's golden-fixture regeneration) touch no
  `*.go`/`*.feature` file: `git diff --stat e93a790 HEAD -- '*.go'
  '*.feature'` prints nothing at commit time, so the tail code sha stays
  `e93a790` even though the *tree* now differs from task 013's tree by the
  one golden-fixture byte fixed below.
- **What changed since task 013's own sweep**: task 013 (`059704a`) ran
  this exact command at this exact sha and found `TestGoldenMinimumFrame`
  red against a stale golden fixture — a real regression left over from
  task 011, not a flake (see that report, preserved as history in git log
  at `059704a`). Task 011b regenerated
  `features/testdata/golden/side_by_side_80x24.golden` via the test's own
  `UPDATE_GOLDEN=1` path (commit `1f38195`) and this sweep is that fix's
  own from-scratch, unnarrowed re-verification, run fresh per the standing
  rules ("never re-run under the task that found the red lane").
- **Command as run** (unnarrowed — every package, no `-run` filter, no
  package list; `-timeout` only bounds the whole invocation, per the
  standing rules' launch guidance):

  ```
  ci/run.sh sh -c 'go test -p=1 -count=1 -timeout=40m ./...'
  ```

- **Skips in force** (same as task 013's sweep, unchanged):
  - godog's default tag filter, `~@real-agents && ~@nightly` (no
    `DECK_GODOG_TAGS` opt-in), excluding `@real-agents`/`@nightly`
    scenarios.
  - The real-binary skips baked into the fixture PATH/agent probes for
    codex, claude and pi: scenarios and probes needing a genuine `codex`,
    `claude` or `pi` executable are satisfied here by the
    `cmd/fake-codex`/`cmd/fake-claude`/`cmd/fake-pi` fixtures — no real
    agent binaries exist in this container.
- **Wall-clock duration**: 7m32s (452s), from `2026-09-17T00:17:13Z` to
  `2026-09-17T00:24:45Z` (date markers wrapping the command directly, not
  polled), close to the ≈7m expected from the standing rules' measurement
  at `db66965` and task 013's own 7m16s at this same sha.
- **Exit status**: `0` (captured directly from the command's own exit
  code, immediately after it returned — never through a `tee` pipe; see
  `ci/stability.sh`'s header comment on the mislabelled-PASS defect this
  avoids).
- **Full output**: committed verbatim at
  `docs/reports/phase4-cure-final-suite/suite.log`. This run used
  `go test`'s default (non-verbose) reporting, exactly the standing rules'
  measured command; godog's own `TestFeatures` writes its per-scenario
  pretty-format log through `TestingT: t` (`features/godog_test.go`),
  which `go test` only surfaces on failure or under `-v` — since nothing
  failed, `suite.log` shows only the per-package summary line, which is
  the same information task 013's report table below is built from. (Two
  earlier from-scratch runs at this sha in this session, also unnarrowed
  and both clean, are not separately committed — this is the single sweep
  the criteria call for; its own log is representative, having been
  reproduced identically before being kept.)

## Result: PASS — every package green

Per-package result (`go list ./...` at this sha lists 18 packages):

| Package | Result |
| --- | --- |
| `cmd/deck` | ok (8.1s) |
| `cmd/fake-claude` | ok (0.8s) |
| `cmd/fake-codex` | ok (0.1s) |
| `cmd/fake-pi` | ok (0.8s) |
| `features` | ok (386.6s) |
| `internal/agent` | ok (0.01s) |
| `internal/audit` | ok (0.02s) |
| `internal/config` | ok (0.03s) |
| `internal/hookrecv` | ok (4.5s) |
| `internal/interactive` | ok (11.6s) |
| `internal/notify` | `[no test files]` |
| `internal/search` | `[no test files]` |
| `internal/service` | ok (7.4s) |
| `internal/store` | ok (2.8s) |
| `internal/theme` | ok (0.01s) |
| `internal/tmux` | ok (20.2s) |
| `internal/tui` | ok (4.3s) |
| `internal/unit` | `[no test files]` |

`features` covers both `TestGoldenMinimumFrame` (now comparing against the
regenerated golden fixture — both `run-1` and `run-2` subtests pass) and
godog's own `TestFeatures` (352 scenarios / 4168 steps at the last verbose
count, task 013's report; scenario content is unchanged by this task, only
the golden fixture's expected bytes moved) — the package result is `ok`
with no visible subtest failure, which for a non-verbose run is the
authoritative pass signal `go test` provides.

## Disposition

The golden-fixture regression task 013 found is fixed (task 011b, commit
`1f38195`) and this from-scratch, unnarrowed sweep at the same tail code
sha (`e93a790`) is clean. This overwrites task 013's own report directory
per the standing rules ("013/014/015 are re-run from scratch afterwards,
never re-run under the task that found the red lane") — task 013's own
finding stands as history in git log at `059704a`, unedited. Tasks 016-018
should read this file, not the superseded content, as current evidence.
