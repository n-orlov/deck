# Phase 0 evidence

**Command:** `ci/run.sh go test -count=1 -v ./...`
**Result:** exit 0; **elapsed wall time:** 11.116 seconds (`time`, recorded
with that command).

The complete, unedited stdout/stderr capture — the required raw evidence,
including its raw ANSI escape bytes — is retained at
[`verbose-suite-capture-source.log`](verbose-suite-capture-source.log). A
readable rendering of that same transcript, with Godog's pretty-formatter
ANSI colour escape codes stripped for plain-text readability and explicitly
labelled as a normalization (not as raw or unedited), is at
[`verbose-suite-capture.md`](verbose-suite-capture.md). It was produced with
`-count=1` and `-v`, so it is not a cached `go test` result. The capture
contains every `=== RUN`, `--- PASS`, package result, the Godog formatter
output, and the `real/user/sys` timing lines.

All 9 tested packages returned `ok`; there is no `FAIL` anywhere in the run.
The Godog-driven `TestFeatures` test reported:

```text
13 scenarios (13 passed)
92 steps (92 passed)
```

The default run reported **7** feature headings, **13** scenarios, and **92**
steps, all passing. The repository has an additional `@real-agents` feature
outside the default tag expression; it is deliberately not represented in the
normal Phase 0 suite count.

`TestGodogRejectsUndefinedAndFailedSteps` is a separate self-test of the
Godog wiring itself: it deliberately runs synthetic failing/undefined inner
Godog suites and asserts Godog reports them correctly. Its `1 failed`/`1
undefined` totals are expected self-test output, not suite failures — the
outer test reports `--- PASS`. See
[`verbose-suite-capture.md`](verbose-suite-capture.md) for the full
explanation.

## Unit-test count

This report uses the following counting convention, defined and verified in
full in [`test-count-evidence.md`](test-count-evidence.md): a top-level Go
test is counted once per line of the exact form `=== RUN   TestXxx` (no `/`,
so Godog subtests such as `TestFeatures/<scenario>/<step>` are excluded)
against the uncached verbose transcript
[`verbose-suite-capture-source.log`](verbose-suite-capture-source.log). Under
that convention there are **42** top-level Go tests. Of those, **1** is
`TestFeatures`, the Godog umbrella test that internally drives the 13
scenarios / 92 steps above; it is called out separately because it is not a
"plain" single-assertion unit test like the other 41.

## Resolved versions

The real command output for the toolchain is retained in
[`toolchain-versions.md`](toolchain-versions.md):

```text
go version go1.25.13 linux/amd64
tmux 3.5a
```

| Component | Resolved version | Source |
|---|---:|---|
| Go | `go1.25.13 linux/amd64` | [`toolchain-versions.md`](toolchain-versions.md) |
| tmux | `3.5a` | [`toolchain-versions.md`](toolchain-versions.md) |
| godog | `v0.16.0` | `go.mod` |
| Bubble Tea | `v1.3.10` | `go.mod` |
| VT100 emulator | `v0.0.0-20220301184237-5011da428d02` | `go.sum` |
| PTY | `v1.1.24` | `go.mod` |
| SQLite | `v1.56.0` | `go.mod` |

## Build and vet evidence

`ci/run.sh go build ./...` and `ci/run.sh go vet ./...` both ran clean
(silent, exit 0) against the corrected tree. Full commands and exit codes are
retained in [`build-vet-evidence.md`](build-vet-evidence.md).

## Ten-run stability evidence

Counting every full ten-run (or attempted ten-run) sequence run while gathering
this evidence, there were **2 total sequence attempts**: an earlier sequence
was discarded after hitting a genuine flake (a pane-read-before-output race,
fixed as part of gathering this evidence — see the gotcha writeup in
[`ten-run-stability.md`](ten-run-stability.md)); a second full ten-run
sequence, run from the beginning after the fix, passed 10/10. The retained
transcript is exclusively that successful second attempt, and its loop itself
emits the final count, not just Markdown prose:

```text
=== FINAL COUNT: 10 / 10 runs passed ===
```

Full transcript: [`ten-run-stability.log`](ten-run-stability.log); report with
the per-run pass markers, the exact counting/final-count-emitting loop
script, and the truthful attempt total: [`ten-run-stability.md`](ten-run-stability.md).

## Gotchas

Each gotcha below was actually discovered while building or hardening this suite; the
consequence is what happens if a future change forgets it.

- **Bubble Tea's OSC 11 / CPR probes.** On startup Bubble Tea queries the terminal's
  background colour (OSC 11) and cursor position (CPR, `\x1b[6n`) and blocks rendering
  frame 1 until it gets replies. A bare PTY is only a byte transport; it never answers
  these queries on its own. `features/pty_driver_test.go`'s `ScreenDriver` answers both
  probes itself as soon as they are observed in the child's output. **Consequence if
  dropped:** every PTY-driven scenario (`walking_skeleton.feature`, help/empty-state
  tests, and any other `StartScreenDriver` user) hangs waiting for a first frame that
  never arrives, instead of failing fast.
- **`sh -c` vs `sh -lc` in the CI sibling wrapper.** `ci/run.sh` and `ci/SPIKE.md` use
  `sh -c '...'` to run module commands inside the sibling container's non-default working
  directory. `ci/SPIKE.md` records that `sh -lc` was tried and rejected: a login shell
  resets `PATH` and produces `go: not found`, because the image's Go path is set up for a
  non-login shell. **Consequence if dropped:** switching to `-lc` (or any login-shell
  invocation) silently breaks every `ci/run.sh go ...` command in this exact way.
- **SQLite initialization ordering.** `internal/store/store.go`'s `OpenPath` reads the
  schema version and rejects a too-new database *before* it calls `os.Chmod` on the home
  directory or sets `journal_mode`/`busy_timeout`/`foreign_keys`. **Consequence if
  dropped:** reordering these steps so permissions or PRAGMAs are applied before the
  version check means opening a newer, unsupported database can mutate a fixture (its
  permissions or journal files) before the open is refused, violating the requirement
  that a newer database is left completely untouched.
- **The first multi-client run must finish empty-database first-use migration before a
  second client opens the same durable root.** `features/lifecycle_test.go`'s
  `TestScenarioHarnessSharesIsolationAndCleansUp` (lines ~266-284) starts a first client
  and explicitly `WaitForFrame`s until it renders "No sessions" before starting a second
  client against the same store, instead of starting both clients concurrently against a
  brand-new, empty database. This is a distinct gotcha from the newer-schema-rejection
  ordering above: that one is about the order of steps *inside* a single `OpenPath` call,
  this one is about the order of *which client opens first* across multiple processes.
  **Consequence if dropped:** starting a second client concurrently with the first against
  an as-yet-uninitialized database races the first client's schema creation/migration; the
  scenario would then be exercising an undefined concurrent-first-open race instead of the
  intended multi-client-against-an-already-initialized-store behaviour, and could flake or
  corrupt the fixture database depending on scheduling.
- **`capture-pane` preserves terminal soft-wraps.** `features/fake_agent_feature_test.go`'s
  `outputHasFakeClaudeContent` strips newlines (but not other whitespace) before matching
  the fixture's argv record, because `tmux capture-pane` reintroduces a hard newline at
  every soft line-wrap boundary, which would otherwise split a long single-line argv
  record across two `capture-pane` output lines and fail an exact-substring match.
  **Consequence if dropped:** matching the raw captured text without normalising wraps
  makes the assertion flaky whenever the terminal geometry or argv length pushes the
  record across a wrap boundary.
- **The original fake-agent hold-based pane race (tasks 001/002).** The fixture used to
  sleep for a fixed `FAKE_CLAUDE_HOLD_MS` before exiting so the test had time to read its
  pane, because tmux's `remain-on-exit failed` — deck's unchanged product contract, still
  tested as such elsewhere — only keeps a *failed* pane's content around after exit; a
  successful pane would otherwise vanish immediately when tmux destroys it. The fix
  removed the hold entirely and instead has `features/fake_agent_feature_test.go`'s
  `launch` set global `remain-on-exit on` on the *fixture's own private tmux server only*
  (deck's product session/server configuration is untouched), so both clean- and
  nonzero-exit panes are retained after their process exits. With the pane retained,
  `outputContains` polls the public `tmux capture-pane -p -S -` (full scrollback) against
  that still-present pane until the expected output appears or a deadline passes; the
  full-scrollback flag only controls how much of the retained pane's buffer is read, it
  does not by itself keep tmux from destroying the pane. **Consequence if dropped:**
  reintroducing a fixed hold reintroduces exactly the race it was meant to hide, and
  removing the fixture-local `remain-on-exit on` setting (or conflating it with deck's
  `remain-on-exit failed` contract) would let successful panes disappear before the test
  can read them, regardless of how much scrollback `capture-pane` is asked to read.
- **The read-before-any-output pane race (found live during task 007).** Even after the
  scrollback fix above, `outputContains` originally read `capture-pane` exactly once,
  immediately after launching the fixture, with no wait at all — so on a slow scheduler
  tick it could read the pane before the fixture process had even started writing, let
  alone finished. This is a distinct race from the hold-based one: it is about *timing of
  the read*, not about *what capture window is read*. The fix makes `outputContains` poll
  `capture-pane` every 25 ms up to a 3 s deadline until the banner, permission-mode line,
  and exact argv are all present, mirroring the pattern `waitForPaneDeadStatus` already
  used later in the same scenario. **Consequence if dropped:** the suite intermittently
  fails the fake-agent scenario under load (it flaked on run 1 of the first attempted
  ten-run sequence in task 007) even though the product and fixture are both correct,
  because the assertion looks before the fixture has necessarily run at all.

## Requirement coverage

The table below was rechecked against the tree delivered by tasks 001–009 (post fake-agent
race fixes, post evidence corrections): every cited test/feature file and test function
name was confirmed to exist under its listed path, and every requirement is covered by
currently-passing evidence rather than a stale or removed test. No row below makes a false
or stale coverage claim as of this recheck.

| Requirement | Corrective verification |
|---|---|
| R1 | `ci/run.sh go build ./...` and `ci/run.sh go vet ./...`; root module declares `github.com/n-orlov/deck`, Go 1.25. |
| R2 | The recorded uncached normal command runs the complete Go and Godog suite. |
| R3 | `walking_skeleton.feature`; `TestDeckBinaryEmptyHelpAndQuitThroughPTY`. |
| R4 | `TestDeckBinaryEmptyHelpAndQuitThroughPTY`, `TestEmptyAndHelpViewsAreDiscoverable`, and `TestHelpTogglesAndEscapeCloses` verify only released actions are advertised and usable. |
| R5 | `tmux_contract.feature` missing/old tmux scenarios. |
| R6 | `determinism.feature`; `TestControlsAndClock`, `TestASCIIColorAndFrozenRelativeTimeRendering`, and the configured runtime cadence coverage. |
| R7 | `determinism.feature`; `TestControlsAndClock`, `TestSuccessfulCreationAdvancesConfiguredFrozenClock`, and `TestDurationAdvancesWithFrozenWallClock`. |
| R8 | `TestJSONLRecordsTransitionsAndLaunchAudit`, `TestCreateShellRecordsCoherentFailureWhenTmuxCannotLaunch`, and walking-skeleton audit assertions. |
| R9–R12 | `store.feature` plus the recorded `internal/store` initialization, migration, refusal, mutation, event, and slug tests. |
| R13–R15 | `tmux_contract.feature`, walking skeleton, and recorded `internal/tmux` lifecycle/attach/bootstrap tests. |
| R16–R17 | `walking_skeleton.feature`, shell-create PTY test, and `concurrency.feature`. |
| R18 | `concurrency.feature` scenario “a live client reconciles an externally killed private server” and `TestReconcilerStopsDisappearedSessionAndDoesNotRelaunchServer`. |
| R19 | `harness.feature`, PTY-driver/lifecycle tests, and the walking-skeleton direct `list-panes` assertion that exactly one pane runs `sh`. |
| R20–R21 | The recorded default feature suite; `fake_agent.feature` and `cmd/fake-claude` tests, with the pane-observation race fixed (see `docs/reports/phase0-findings.md`). |
| R22 | This count-bearing report and the matching retained captures named throughout. |
| R23 | The ten-run stability evidence in [`ten-run-stability.md`](ten-run-stability.md) / [`ten-run-stability.log`](ten-run-stability.log) supersedes any prior two-run captures. |

## Retained captures

- [`verbose-suite-capture.md`](verbose-suite-capture.md) / [`verbose-suite-capture-source.log`](verbose-suite-capture-source.log) — full uncached verbose suite output and wall timing.
- [`test-count-evidence.md`](test-count-evidence.md) — the top-level Go test counting convention, command, and 42-test result.
- [`toolchain-versions.md`](toolchain-versions.md) — resolved Go and tmux version output.
- [`build-vet-evidence.md`](build-vet-evidence.md) — zero-exit `go build`/`go vet` captures.
- [`ten-run-stability.md`](ten-run-stability.md) / [`ten-run-stability.log`](ten-run-stability.log) — ten consecutive clean-state suite passes, plus the pane-read race discovered and fixed while gathering them.

The job container has no local Go or tmux. All toolchain commands use the
repository's `ci/run.sh` sibling-container wrapper. No protected specification
or CI-image files were modified for this evidence.
