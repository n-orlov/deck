# Phase 2 — status truth report

## Verification capture

The following commands were run from the repository root at clean source
checkpoints containing the transition fix. Their complete, unedited command
output is retained at repository-relative paths:

| Command | Tested commit | Raw capture | Result |
|---|---|---|---|
| `ci/run.sh go build ./...` | `7bcb96e96915f6fef1b0a5958eeb463051e6a7c0` | [`phase2-build.log`](phase2-build.log) | exit 0 |
| `ci/run.sh go vet ./...` | `ef9881df77b99877a7b03f14146190b9dd668a98` | [`phase2-vet.log`](phase2-vet.log) | exit 0 |
| `ci/run.sh go test -v -count=1 ./...` | `d562d6fbe53b9f1db296c1f54d8ab1b8bce15baf` | [`phase2-full-verbose-run.log`](phase2-full-verbose-run.log) | exit 0; **55.277 seconds wall time** |

The full run reports **21 features**, **48 scenarios (48 passed)**, and **546
steps (546 passed)** under the unchanged default `~@real-agents && ~@nightly`
tag expression. The feature count is the 21 ANSI-coloured `Feature:` headings in
the retained Godog output (one heading per executed `.feature` file). Godog's
deliberately undefined and deliberately failing private harness self-tests
also appear in the raw log; their enclosing Go test passes because rejecting
those outcomes is what it tests. They are not members of the real
21-feature/48-scenario/546-step suite.

**Top-level Go-test counting convention.** Count lines matching
`^=== RUN   Test[A-Za-z0-9_]*$` in the verbose capture. This counts each
package-level `func Test...` invocation and excludes slash-suffixed `t.Run`
subtests. On that convention this run contains **187 top-level Go tests**.
All tested packages report `ok`; `internal/notify`, `internal/search`, and
`internal/unit` report `[no test files]`.

Resolved in the same `ci/run.sh` image:

```text
go version go1.25.13 linux/amd64
tmux 3.5a
github.com/cucumber/godog v0.16.0
```

## Closure of the Phase 2 review findings

| Review finding | Released-binary or retained evidence |
|---|---|
| Attaching to an error row did not acknowledge it | `status_attach.feature: attach acknowledges a live error without replacing its verdict` proves the error reason/source/epoch survive while acknowledgement and the unseen marker clear. |
| A non-leasable resume could misleadingly claim a held lease | `launch_lease.feature: a stale stopped row reports its new non-leasable verdict instead of a lease conflict` proves `r` refreshes the actual error/reason and never shows `starting elsewhere`; the adjacent live in-TTL scenario pins the unchanged lease wording. |
| `DECK_CLOCK_STEP` had no production trigger | `determinism.feature: deterministic frames, shared clock stepping, and generated identifiers` sends `SIGUSR1` to an already-running released client, proves two clients and a later `_hook` share the stepped instant, and reaches probe staleness without a 45-second sleep. |
| Real-Claude conformance had not been executed | The passing authenticated Claude Code 2.1.237 capture [`phase2-real-claude-authenticated.log`](phase2-real-claude-authenticated.log) proves inline hooks are accepted, `SessionStart` reaches the released `deck _hook`, and a genuine `UserPromptSubmit` payload supplies all four required non-empty string fields. **R36 is met** without synthesizing `permission_mode`; the earlier `SessionStart` omission remains documented in the [`version matrix`](phase2-real-claude-version-matrix.log) and [`findings`](phase2-findings.md). |
| Stale hooks could violate §7's exhaustive transition table | `internal/hookrecv: TestReceiveMappingTable` crosses every declared hook with every current state, while `internal/store: TestStatusTransitionEnforcesCallerPolicyInsideEventTransaction` and `TestStatusTransitionDoesNotReviveProcessCrashWithHook` prove atomic policy enforcement and crash-terminal protection. Released-binary scenarios `A pane-fired hook cannot override a user-terminal verdict` and `A pane-fired hook cannot revive a process-crash terminal row` prove rejected payloads remain audited without resurrecting either terminal row. The policy and SPEC ambiguity are detailed in the [`findings`](phase2-findings.md#exhaustive-transition-policy-review-resolution). |

## Requirement evidence (R1–R45)

Every named scenario and test below appears in the successful verbose capture
unless explicitly marked opt-in. Feature paths are under `features/`; Go-test
paths are repository-relative.

| R | Recorded proving run or scenario/test |
|---:|---|
| 1 | `status_claude_hooks.feature` proves the legal event sequence plus pane-fired stale-hook rejection for cleanly stopped and process-crash terminal rows; `cmd/deck: TestReleasedDeckHookIsOneShotAndDoesNotBootstrapStateOrTmux` proves the released hidden one-shot path. |
| 2 | `cmd/deck: TestReleasedDeckHookIsOneShotAndDoesNotBootstrapStateOrTmux` covers absent/stale state, malformed/extra JSON, no state creation, and no tmux bootstrap. |
| 3 | `status_claude_hooks.feature: Every declared Claude hook maps to honest status through both identity routes`; `internal/hookrecv: TestReceiveResolutionPrefersConversationThenUsesInjectedIdentity`. |
| 4 | `internal/hookrecv: TestReceivePreservesUnresolvedPayloadAsOrphan`; `internal/store: TestRecordOrphanEventUsesNullSessionAndRequiresTimestamp`. |
| 5 | `hook_contract.feature: An uncontended hook store write stays below twenty milliseconds` and `Session end performs one write and no subsequent work`. The audited `store_duration_ms` brackets only the store callback/transaction with `time.Now`'s monotonic component—not process startup, JSON parsing, SQLite open, liveness, or audit append. |
| 6 | `internal/agent: TestClaude_InstrumentReturnsInlineHooksAndDeckEnvironmentWithoutIO`; inline `--settings` JSON keeps the adapter pure and writes no settings file. |
| 7 | The same instrumentation test checks the supplied absolute executable; service launch coverage is in `internal/service: TestCreateAgentAssignsConversationIDAndLaunchesClaudeArgv`. |
| 8 | `status_claude_hooks.feature` checks the pane's scenario hook environment, scenario-local store activity, and no deck state in the working directory. |
| 9 | `internal/service: TestCreateAgentAssignsConversationIDAndLaunchesClaudeArgv` and resume coverage distinguish injected environment from persisted user environment. |
| 10 | `internal/agent: TestShellInstrumentIsEmpty`, `internal/service: TestCreateAgentShellHasNoInstrumentation`, and `internal/hookrecv: TestReceiveRejectsShellTarget`. |
| 11 | `status_claude_hooks.feature: Every declared Claude hook maps...` fires all six declared events through SPEC-legal edges and checks status, subtype/reason, message, acknowledgement, epoch, and stored event. `internal/hookrecv: TestReceiveMappingTable` exhaustively crosses each event with all current states and verifies rejected events remain audited. |
| 12 | `cmd/fake-claude: TestPaneCommandsFireEveryInjectedHookWithControllablePayload`; upstream uncertainty is recorded in [`phase2-findings.md`](phase2-findings.md), while the table remains the SPEC contract. |
| 13 | The Claude-hook scenario checks Stop's message in both SQLite and detail; `internal/store: TestStatusTransitionProtectsUserKillAndPersistsHookAndCrashFields` pins UTF-8-safe 2 KiB truncation. |
| 14 | `internal/agent: TestProbeGoldenPaneCorpus`, `TestProbeDeclinesUnknownTextAndShellIsIneligible`, and `TestProbeRuleTableHasOneRulePerGolden`. |
| 15 | `status_probe.feature: Real fake-agent panes render the complete probe golden corpus` sends the same ten fixture files used by `TestProbeGoldenPaneCorpus` through real panes byte-for-byte. |
| 16 | `status_probe.feature: Stale sampling is visible, precedence-aware, and agent-only`; focused service tests cover starting/running/waiting eligibility, the boundary, shell exclusion, and TUI-only probing. |
| 17 | The stale-sampling scenario signals a running released client with `SIGUSR1` to advance across `stale_after` rather than sleeping or calculating an absolute instant; `internal/config: TestDeckHomeClocksShareOnDemandAdvance`. |
| 18 | The stale-sampling scenario records a losing `probe.waiting` event against a fresh hook and a winning probe correction against a stale hook. |
| 19 | The stale-sampling scenario visibly checks `live` on hook, `sampled` on Claude/Pi probe rows, and no sampled badge on shell. |
| 20 | `internal/tui: TestDetailShowsSourceFrozenClockAgeAndStatusArtifacts`; the black-box stale-sampling scenario proves list badges. |
| 21 | `status_user_kill.feature: a running hook cannot undo an explicit user kill` plus `status_claude_hooks.feature` pane-fired scenarios prove late hooks cannot revive a clean stop, user-terminal verdict, or process-crash terminal row. Store tests prove the allow-list decision and event audit are atomic. |
| 22 | The same user-kill scenario proves kill → resume clears the flag → hook reaches running. |
| 23 | `internal/tui: TestYAcknowledgesOnlySelectedRowDurably` reconstructs the model/store and proves only the selected marker remains cleared after restart; hook/error transition behavior is covered by store transition tests. |
| 24 | `status_attach.feature` proves both `attach clears waiting and acknowledges it in one transition` and `attach acknowledges a live error without replacing its verdict`; the latter keeps error source/reason/epoch while durably clearing the unseen marker. |
| 25 | The attach scenario checks epoch `0→1`; the T3 Claude-hook scenario checks waiting → idle and error → stopped epoch changes in the same durable transitions. |
| 26 | `internal/service: TestReconcilerStopsDisappearedSessionAndDoesNotRelaunchServer` and `TestReconcilerCapturesAndCollectsCrashFirstWriterOnly`; all three observations include live preservation. |
| 27 | `crash.feature: a shell exit zero is a clean stop without a crash artifact`. |
| 28 | `crash.feature: SIGKILL captures and sanitizes the agent pane without relaunching`; `internal/service: TestCrashTailStripsControlsAndKeepsLast200Lines`. |
| 29 | `crash.feature: racing clients collect one corpse idempotently` checks first artifact stability, disappearance, and launch count 1; tmux kill idempotence is integration-tested. |
| 30 | `shell_liveness.feature` separately proves a live shell reaches running and a live unsignalled agent remains starting. |
| 31 | `launch_lease.feature: a stale stopped row reports its new non-leasable verdict instead of a lease conflict` provides released-binary proof alongside `internal/store: TestAcquireLaunchLeaseNonStoppedIsDistinctFromHeldLease` and `internal/service: TestResumeNonLeasableReturnsActualStatusAndReason`; the live in-TTL scenario retains the `starting elsewhere` contract. |
| 32 | `status_claude_hooks.feature: Every declared Claude hook maps...` contains T3's running → permission-prompt waiting → Stop idle/message → waiting sequence and direct epoch assertions, with no notification-delivery stub. |
| 33 | `lease_race.feature: three clients racing resume on one row produce exactly one launch` completes T2 with a pane-fired SessionStart and three running rows; `shell_liveness.feature` retains the separate no-fabricated-agent-running check. |
| 34 | The clean-shell and SIGKILL scenarios in `crash.feature` check status, tail/no-tail, and launch count 1. |
| 35 | `concurrency.feature: one hook-driven status change propagates to every client`. |
| 36 | **Met.** The documented opt-in command exited 0 with authenticated genuine Claude Code 2.1.237. The unedited [`authenticated capture`](phase2-real-claude-authenticated.log) proves `SessionStart` injection and released-`_hook` delivery, then captures a genuine `UserPromptSubmit` payload with non-empty string `session_id`, `cwd`, `transcript_path`, and `permission_mode` (`default`); all 3 scenarios and 27 steps pass without aliases or synthesized fields. The [`version matrix`](phase2-real-claude-version-matrix.log) separately preserves the observed `SessionStart` omission in versions 1.0.128–2.1.200. |
| 37 | This report and its linked repository-relative build, vet, full-suite, stability, authenticated-Claude, version-matrix, and [`state-machine findings`](phase2-findings.md#exhaustive-transition-policy-review-resolution) evidence. |
| 38 | A real `ci/stability.sh 10` run from clean final production/test source checkpoint `20b19dd5e30fc1af0801229c2df36ff30447290b` passed **10/10**. Its complete combined package output, per-run exit labels, command, commit, clean-worktree statement, and final exit 0 are retained unedited in [`phase2-stability.log`](phase2-stability.log). The runner uses `-p=1` so each repetition exercises the sub-second tmux/SQLite contracts without unrelated Go-package load, while every package, feature, scenario, and test still executes on every run. |
| 39 | The run uses the unchanged default tag expression. No Phase 1 scenario was removed; shell assertions were re-aimed to live-shell promotion while `shell_liveness.feature` retains the unsignalled-agent negative assertion. Final protected-file/no-exclusion proof is paired with the fixed-commit stability checkpoint. |
| 40 | The operator walkthrough below. |
| 41 | `cmd/fake-claude: TestPaneCommandsFireEveryInjectedHookWithControllablePayload` and `TestPaneHookCommandRequiresInjectedSettingsRatherThanCallingDeckDirectly`; the Claude-hook feature fires commands from the pane. |
| 42 | `cmd/fake-claude`/`cmd/fake-pi: TestPaneFixtureCommandRendersNamedFileByteForByte` and the complete-corpus black-box scenario. |
| 43 | `SIGUSR1` is the documented on-demand trigger for a running deck client. `internal/config: TestDeckHomeClocksShareOnDemandAdvance` proves lock-serialized exact increments, `cmd/deck: TestRunningClientSIGUSR1AdvancesSharedClock` proves the production signal path, and `determinism.feature` proves two live clients plus a later hook share the result. |
| 44 | `fake_agent.feature: SIGKILL targets the agent process and retains its failed pane`; the crash scenarios consume that process-level step. |
| 45 | `crash.feature: a different hook detects a crash while no TUI is running`; `cmd/deck: TestReleasedHookBoundsStalledTmuxAndSkipsItForSessionEnd` proves timeout, no SessionEnd liveness, and no probing. |

## Operator walkthrough: trust the status column

This uses a real Claude CLI and an isolated store/socket. Claude hook names and
payload fields are upstream contracts; if an installed Claude version has
changed them, the opt-in conformance scenario should fail rather than deck
silently inventing a status.

### 1. Build and isolate

From the repository root:

```sh
go build -o /tmp/deck-phase2 ./cmd/deck
export DECK_HOME="$(mktemp -d)"
export DECK_TMUX_SOCKET="deck-phase2-smoke-$$"
mkdir -p /tmp/deck-phase2-cwd
/tmp/deck-phase2
```

Working means deck opens with an empty list and creates only
`$DECK_HOME/state.db`/logs and its private `tmux -L "$DECK_TMUX_SOCKET"`
server—not your normal tmux server.

### 2. Create Claude and observe `running`

In deck press:

1. `n` (create modal), type `status-smoke`.
2. `Tab`, type `/tmp/deck-phase2-cwd`.
3. `Tab` to Agent, then `Right` until `claude` is selected.
4. Leave the safe permission profile selected and press `Enter`.

The initial row is honestly `starting · awaiting signal`. When real Claude's
injected SessionStart hook arrives it changes to **`running live`** within one
reconcile interval. It must not become running merely because a pane exists.
Press `Enter` to attach; detach back to deck with tmux's default `Ctrl-b d`.

### 3. Observe `running → waiting → idle`, then acknowledge with `Y`

Attach with `Enter`, type this prompt exactly, and press `Enter`:

```text
Use the Bash tool to run exactly: printf 'phase2 permission check\n' >> /tmp/deck-phase2-cwd/permission-check.txt
```

Under the selected `safe`/Claude `manual` permission profile, wait until the
Bash permission dialog appears, but do not approve it yet. UserPromptSubmit
keeps the row at `running live`. Detach with the exact tmux sequence
`Ctrl-b`, release both keys, then `d` while that dialog is outstanding. The
Notification hook changes the row to **`waiting live`**, shows reason
`permission_prompt`, and adds the unseen marker.

With that row selected, press uppercase **`Y`**. Working means the unseen
marker clears and stays clear after quitting with `q` and reopening
`/tmp/deck-phase2`; `Y` acknowledges but deliberately does not falsify the
waiting verdict. To continue the exact smoke path, press `Enter` on the
waiting row; deck acknowledges it and changes it to `running` before attaching.
In Claude's permission dialog, press `Down` until **Yes, allow once** is
highlighted and press `Enter`. (If that option is already highlighted, press
`Enter` without `Down`; do not choose the persistent-allow option.) Wait for
Claude to report completion, then detach with `Ctrl-b`, release, `d`. The Stop
hook changes the row to **`idle live`**. Press `i` to open detail: it shows
`source: hook`, the frozen-wall-clock-controlled verdict age, and Claude's last
assistant message.

Working sequence: `starting` only before a signal, then `running live` →
`waiting live` with an unseen marker/reason → marker cleared by `Y` (status
still waiting) → `idle live` after Stop.

### 4. Crash the pane process and inspect the tail

First attach and arrange recognisable recent pane output, then detach. From a
second shell, obtain and kill the **process in the pane**, not deck and not the
tmux session:

```sh
pane_pid=$(tmux -L "$DECK_TMUX_SOCKET" list-panes \
  -t deck_status-smoke -F '#{pane_pid}')
kill -KILL "$pane_pid"
```

Within one reconcile interval the row becomes **`error`**. Deck captures the
plain-text last 200 pane lines before collecting the retained dead session; it
does not relaunch. Select the row and press `i`. Working means detail shows the
nonzero exit status and recognisable beginning/end of the sanitized crash
tail, while this reports no session:

```sh
tmux -L "$DECK_TMUX_SOCKET" has-session -t deck_status-smoke
```

### 5. Cleanup and real-agent conformance

Quit deck with `q`, then:

```sh
tmux -L "$DECK_TMUX_SOCKET" kill-server 2>/dev/null || true
rm -rf "$DECK_HOME" /tmp/deck-phase2-cwd /tmp/deck-phase2
```

Run the opt-in upstream contract check from the repository root with an
installed/authenticated Claude CLI:

```sh
DECK_GODOG_TAGS=@real-agents go test -run TestFeatures -v ./features/...
```

Rough edges: Pi has no hook path in this phase and therefore remains honestly
`sampled`; probe changes require a running TUI and a verdict old enough for
`stale_after`; source age is wall-clock age, while hook transaction duration
is monotonic; SessionEnd intentionally performs no enqueue until Phase 5; and
the remaining unprovoked real-Claude event shapes remain upstream assumptions;
the opt-in scenario has confirmed `SessionStart` and `UserPromptSubmit` against
Claude Code 2.1.237.

## Gotchas and consequences

- Frozen time is shared through the resolved data root's `clock.now`. Send
  `SIGUSR1` to any running deck client (`kill -USR1 <deck-client-pid>`) to take
  the shared file lock and advance it by exactly `DECK_CLOCK_STEP`; callers do
  not calculate or write an absolute instant. All clients and later hook
  subprocesses read the result consistently. Store status writers require
  explicit time.
- Hook latency is the monotonic store transaction span. Process-lifetime audit
  duration and frozen wall time would respectively measure the wrong work and
  make the assertion unfalsifiable.
- A fresh higher-quality hook may make a sampled probe lose. Durable losing
  probe evidence is required so unchanged status cannot pass against dead
  probe code.
- tmux reports signal death separately; deck records the conventional
  `128+signal` exit status. Crash capture must happen before idempotent session
  collection and strips controls at capture time.
- Reconcile reads durable launch intent before acting on tmux snapshots; this
  prevents an older absent snapshot from stopping a newly launched row.
- SessionEnd's no-liveness/no-probe/no-enqueue behavior is a temporary Phase 2
  boundary. Phase 5 must add enqueue while preserving the critical-path
  exclusions.
- The default suite intentionally excludes `@real-agents`, but the documented
  opt-in command separately passed with authenticated genuine Claude Code
  2.1.237. `SessionStart` proves injection/delivery, and a genuine
  `UserPromptSubmit` supplies all four required fields, including
  `permission_mode`; the earlier SessionStart omission remains an upstream
  finding in [`phase2-findings.md`](phase2-findings.md), not a normalized payload.
- The final clean production/test checkpoint stability run at `20b19dd5e30fc1af0801229c2df36ff30447290b`
  passed 10/10. Complete per-package output, every run label, and final exit 0
  are retained in [`phase2-stability.log`](phase2-stability.log); the report
  does not infer stability from one green full-suite run.
- Shell liveness means `running`; agent pane liveness does not. Re-aimed Phase
  1 shell assertions preserve the original protection with a separate live,
  unsignalled-agent negative scenario.
