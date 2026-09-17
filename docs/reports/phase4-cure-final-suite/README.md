# Full-suite gate sweep — approach 2, at tail code sha `0ba550a`

This file is **task 013's gate of record**. Exactly ONE unnarrowed whole-tree
run exists at the tail code sha named below, and it is the run this file
reports (measured and committed by task 011b's re-sweep, `0ca8667`, per the
standing rules' "013/014/015 are re-run from scratch afterwards, never re-run
under the task that found the red lane" — task 013 is the task that found the
red lane). The earlier runs described under "Why this sha" below were sweeps of
**different, superseded trees** (`e93a790`, and the `1f38195` tree), not repeats
of this one; they stay in git history as the record of how the red lane was
found and fixed.

- **Tail code sha**: `0ba550a5e50bdfc84586d5328a0690af9c9888c4` (`0ba550a`,
  `features: settle the golden frame on a quiet PTY, not a torn read (task
  011b)`). This is the sha tasks 014-018 cite. It supersedes the two
  earlier candidates this directory named in turn: `e93a790` (task 011,
  cited by task 013's own red sweep and by 011b's first attempt) and the
  tree at `1f38195` (011b's golden-fixture regeneration, a `*.feature`
  testdata file, not a `*.go` file).
- **Why this sha, and what changed since 011b's first attempt**: task 013
  (`059704a`) ran this exact command at `e93a790` and found
  `TestGoldenMinimumFrame` red — a real regression left over from task 011
  against a stale golden fixture. Task 011b regenerated
  `features/testdata/golden/side_by_side_80x24.golden` via the test's own
  `UPDATE_GOLDEN=1` path (`1f38195`) and re-swept, but that sweep was
  still intermittently red on the SAME test for a second, independent
  reason: `renderGoldenMinimumFrame` took its "settled" baseline with a
  bare `client.Frame(true)` immediately after its last content gate, and
  that gate is satisfied the moment the row carrying its substring is
  written — while the renderer is still emitting the rest of the same
  repaint. The baseline could therefore be a genuinely torn frame,
  reproduced directly here at 3 of 12 sub-runs
  (`ci/run.sh sh -c 'go test -count=6 -run ^TestGoldenMinimumFrame$
  ./features/ -v'`), every failure the same shape: `before` missing the
  bottom border and the footer line that `after` then has. Commit
  `0ba550a` takes the baseline from `ScreenDriver.WaitForQuiescence`
  (300ms quiet window, longer than deck's 250ms `previewTick`; the same
  fix `clientCapturesFrameAs` already applies for the same reason) and
  retries the still-identical compare up to five times. 16 of 16 sub-runs
  green over `-count=8` after the change. No product code is touched and
  the byte-exact comparison against the checked-in golden is unchanged.
  This sweep is that fix's own from-scratch, unnarrowed verification, run
  fresh per the standing rules ("never re-run under the task that found
  the red lane").
- **Command as run** (unnarrowed — every package, no `-run` filter, no
  package list; `-timeout` only bounds the whole invocation, per the
  standing rules' launch guidance):

  ```
  ci/run.sh sh -c 'go test -p=1 -count=1 -timeout=40m ./...'
  ```

- **Skips in force** (unchanged from task 013's sweep):
  - godog's default tag filter, `~@real-agents && ~@nightly` (no
    `DECK_GODOG_TAGS` opt-in), excluding `@real-agents`/`@nightly`
    scenarios.
  - The real-binary skips baked into the fixture PATH/agent probes for
    codex, claude and pi: scenarios and probes needing a genuine `codex`,
    `claude` or `pi` executable are satisfied here by the
    `cmd/fake-codex`/`cmd/fake-claude`/`cmd/fake-pi` fixtures — no real
    agent binaries exist in this container.
- **Wall-clock duration**: 7m21s (441s), from `2026-09-17T02:02:30Z` to
  `2026-09-17T02:09:51Z` (the finish stamp is the mtime of the file the
  wrapper wrote with the command's own exit status, immediately after it
  returned), in line with the ≈7m expected from the standing rules'
  measurement at `db66965`.
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
  failed, `suite.log` shows only the per-package summary lines, which is
  what the table below is built from.

## Result: PASS — every package green

Per-package result (`go list ./...` at this sha lists 18 packages; the
lines below are `suite.log` verbatim):

| Package | Result |
| --- | --- |
| `cmd/deck` | ok (7.959s) |
| `cmd/fake-claude` | ok (0.792s) |
| `cmd/fake-codex` | ok (0.122s) |
| `cmd/fake-pi` | ok (0.773s) |
| `features` | ok (378.948s) |
| `internal/agent` | ok (0.010s) |
| `internal/audit` | ok (0.018s) |
| `internal/config` | ok (0.024s) |
| `internal/hookrecv` | ok (4.362s) |
| `internal/interactive` | ok (11.159s) |
| `internal/notify` | `[no test files]` |
| `internal/search` | `[no test files]` |
| `internal/service` | ok (6.819s) |
| `internal/store` | ok (2.627s) |
| `internal/theme` | ok (0.004s) |
| `internal/tmux` | ok (19.833s) |
| `internal/tui` | ok (3.903s) |
| `internal/unit` | `[no test files]` |

`features` covers both `TestGoldenMinimumFrame` (comparing against the
regenerated golden fixture, with the quiescence-based baseline of
`0ba550a` — both `run-1` and `run-2` subtests pass) and godog's own
`TestFeatures` (352 scenarios / 4168 steps at the last verbose count, task
013's report; scenario content is unchanged by task 011b — only the golden
fixture's expected bytes and that one test's settle gate moved). The
package result is `ok` with no visible subtest failure, which for a
non-verbose run is the authoritative pass signal `go test` provides.

## Task 013 disposition

- The gate at the tail code sha is a single unnarrowed run: `go test -p=1
  -count=1 -timeout=40m ./...`, exit `0`, 18/18 packages accounted for
  (`ci/run.sh go list ./...` = 18 packages at this sha), 7m21s.
- The tested tree is still the tail code tree: `git diff --stat 0ba550a HEAD
  -- '*.go' '*.feature'` prints nothing, so every commit on top of `0ba550a`
  is record-only and the gate stays valid at it.
- Supporting spot-check taken while closing task 013, deliberately NOT part of
  the gate (it is narrowed, so it can never be one): `ci/run.sh sh -c 'go test
  -count=2 -run "^TestGoldenMinimumFrame$" ./features/'` → `ok ... 5.275s`,
  i.e. the lane that was red at `e93a790` is still green at the tail sha. The
  gate above remains the only unnarrowed measurement.

## Disposition

Both red lanes are answered: the golden-fixture regression task 013 found
(fixed in `1f38195`) and the torn-baseline race that kept the same test
intermittently red afterwards (fixed in `0ba550a`). This from-scratch,
unnarrowed sweep at the new tail code sha is clean, exit `0`. It
overwrites the earlier content of this directory per the standing rules
("013/014/015 are re-run from scratch afterwards, never re-run under the
task that found the red lane") — task 013's own red-lane finding stands as
history in git log at `059704a`, unedited, as does 011b's first-attempt
report in the history of this file. Tasks 016-018 read this file, and the
sha named at the top of it, as current evidence.
