# Phase 2 — status truth report

## Verification capture

The following commands were run from the repository root at `36f1c3e`. Their
complete, unedited command output is retained at repository-relative paths:

| Command | Raw capture | Result |
|---|---|---|
| `ci/run.sh go build ./...` | [`phase2-build.log`](phase2-build.log) | exit 0 |
| `ci/run.sh go vet ./...` | [`phase2-vet.log`](phase2-vet.log) | exit 0 |
| `time ci/run.sh go test -v -count=1 ./...` | [`phase2-full-verbose-run.log`](phase2-full-verbose-run.log) | exit 0; `real 1m5.282s` |

The full run reports **21 features**, **45 scenarios (45 passed)**, and **501
steps (501 passed)** under the default `~@real-agents && ~@nightly` tag
expression. The feature count is the 21 ANSI-coloured `Feature:` headings in
the retained Godog output (one heading per executed `.feature` file). Godog's
deliberately undefined and deliberately failing private harness self-tests
also appear in the raw log; their enclosing Go test passes because rejecting
those outcomes is what it tests. They are not members of the real
21-feature/45-scenario/501-step suite.

**Top-level Go-test counting convention.** Count lines matching
`^=== RUN   Test[A-Za-z0-9_]*$` in the verbose capture. This counts each
package-level `func Test...` invocation and excludes slash-suffixed `t.Run`
subtests. On that convention this run contains **178 top-level Go tests**.
All tested packages report `ok`; `internal/notify`, `internal/search`, and
`internal/unit` report `[no test files]`.

Resolved in the same `ci/run.sh` image:

```text
go version go1.25.13 linux/amd64
tmux 3.5a
github.com/cucumber/godog v0.16.0
```

## Requirement evidence (R1–R45)

Every named scenario and test below appears in the successful verbose capture
unless explicitly marked opt-in. Feature paths are under `features/`; Go-test
paths are repository-relative.

| R | Recorded proving run or scenario/test |
|---:|---|
| 1 | `status_claude_hooks.feature: Every declared Claude hook maps to honest status through both identity routes`; `cmd/deck: TestReleasedDeckHookIsOneShotAndDoesNotBootstrapStateOrTmux` proves the released hidden one-shot path. |
| 2 | `cmd/deck: TestReleasedDeckHookIsOneShotAndDoesNotBootstrapStateOrTmux` covers absent/stale state, malformed/extra JSON, no state creation, and no tmux bootstrap. |
| 3 | `status_claude_hooks.feature: Every declared Claude hook maps to honest status through both identity routes`; `internal/hookrecv: TestReceiveResolutionPrefersConversationThenUsesInjectedIdentity`. |
| 4 | `internal/hookrecv: TestReceivePreservesUnresolvedPayloadAsOrphan`; `internal/store: TestRecordOrphanEventUsesNullSessionAndRequiresTimestamp`. |
| 5 | `hook_contract.feature: An uncontended hook store write stays below twenty milliseconds` and `Session end performs one write and no subsequent work`. The audited `store_duration_ms` brackets only the store callback/transaction with `time.Now`'s monotonic component—not process startup, JSON parsing, SQLite open, liveness, or audit append. |
| 6 | `internal/agent: TestClaude_InstrumentReturnsInlineHooksAndDeckEnvironmentWithoutIO`; inline `--settings` JSON keeps the adapter pure and writes no settings file. |
| 7 | The same instrumentation test checks the supplied absolute executable; service launch coverage is in `internal/service: TestCreateAgentAssignsConversationIDAndLaunchesClaudeArgv`. |
| 8 | `status_claude_hooks.feature` checks the pane's scenario hook environment, scenario-local store activity, and no deck state in the working directory. |
| 9 | `internal/service: TestCreateAgentAssignsConversationIDAndLaunchesClaudeArgv` and resume coverage distinguish injected environment from persisted user environment. |
| 10 | `internal/agent: TestShellInstrumentIsEmpty`, `internal/service: TestCreateAgentShellHasNoInstrumentation`, and `internal/hookrecv: TestReceiveRejectsShellTarget`. |
| 11 | `status_claude_hooks.feature: Every declared Claude hook maps...` fires all six declared events and checks status, subtype/reason, message, acknowledgement, epoch, and stored event. `internal/hookrecv: TestReceiveMappingTable` pins the single declarative table. |
| 12 | `cmd/fake-claude: TestPaneCommandsFireEveryInjectedHookWithControllablePayload`; upstream uncertainty is recorded in [`phase2-findings.md`](phase2-findings.md), while the table remains the SPEC contract. |
| 13 | The Claude-hook scenario checks Stop's message in both SQLite and detail; `internal/store: TestStatusTransitionProtectsUserKillAndPersistsHookAndCrashFields` pins UTF-8-safe 2 KiB truncation. |
| 14 | `internal/agent: TestProbeGoldenPaneCorpus`, `TestProbeDeclinesUnknownTextAndShellIsIneligible`, and `TestProbeRuleTableHasOneRulePerGolden`. |
| 15 | `status_probe.feature: Real fake-agent panes render the complete probe golden corpus` sends the same ten fixture files used by `TestProbeGoldenPaneCorpus` through real panes byte-for-byte. |
| 16 | `status_probe.feature: Stale sampling is visible, precedence-aware, and agent-only`; focused service tests cover starting/running/waiting eligibility, the boundary, shell exclusion, and TUI-only probing. |
| 17 | The stale-sampling scenario advances `clock.now` across `stale_after` rather than sleeping; `internal/config: TestDeckHomeClocksShareOnDemandAdvance`. |
| 18 | The stale-sampling scenario records a losing `probe.waiting` event against a fresh hook and a winning probe correction against a stale hook. |
| 19 | The stale-sampling scenario visibly checks `live` on hook, `sampled` on Claude/Pi probe rows, and no sampled badge on shell. |
| 20 | `internal/tui: TestDetailShowsSourceFrozenClockAgeAndStatusArtifacts`; the black-box stale-sampling scenario proves list badges. |
| 21 | `status_user_kill.feature: a running hook cannot undo an explicit user kill` and the pane-fired terminal-precedence scenario in `status_claude_hooks.feature`. |
| 22 | The same user-kill scenario proves kill → resume clears the flag → hook reaches running. |
| 23 | `internal/tui: TestYAcknowledgesOnlySelectedRowDurably` reconstructs the model/store and proves only the selected marker remains cleared after restart; hook/error transition behavior is covered by store transition tests. |
| 24 | `status_attach.feature: attach clears waiting and acknowledges it in one transition`. |
| 25 | The attach scenario checks epoch `0→1`; the T3 Claude-hook scenario checks waiting → idle and error → stopped epoch changes in the same durable transitions. |
| 26 | `internal/service: TestReconcilerStopsDisappearedSessionAndDoesNotRelaunchServer` and `TestReconcilerCapturesAndCollectsCrashFirstWriterOnly`; all three observations include live preservation. |
| 27 | `crash.feature: a shell exit zero is a clean stop without a crash artifact`. |
| 28 | `crash.feature: SIGKILL captures and sanitizes the agent pane without relaunching`; `internal/service: TestCrashTailStripsControlsAndKeepsLast200Lines`. |
| 29 | `crash.feature: racing clients collect one corpse idempotently` checks first artifact stability, disappearance, and launch count 1; tmux kill idempotence is integration-tested. |
| 30 | `shell_liveness.feature` separately proves a live shell reaches running and a live unsignalled agent remains starting. |
| 31 | `internal/store: TestAcquireLaunchLeaseNonStoppedIsDistinctFromHeldLease`, `internal/service: TestResumeNonLeasableReturnsActualStatusAndReason`, and the unchanged live-owner assertions in `launch_lease.feature`. |
| 32 | `status_claude_hooks.feature: Every declared Claude hook maps...` contains T3's running → permission-prompt waiting → Stop idle/message → waiting sequence and direct epoch assertions, with no notification-delivery stub. |
| 33 | `lease_race.feature: three clients racing resume on one row produce exactly one launch` completes T2 with a pane-fired SessionStart and three running rows; `shell_liveness.feature` retains the separate no-fabricated-agent-running check. |
| 34 | The clean-shell and SIGKILL scenarios in `crash.feature` check status, tail/no-tail, and launch count 1. |
| 35 | `concurrency.feature: one hook-driven status change propagates to every client`. |
| 36 | Opt-in `real_agent_smoke.feature: real claude accepts injected hooks and supplies the upstream payload contract`, runnable exactly as documented below. No Claude binary was installed here, so this conformance scenario is intentionally excluded from the default capture and is not claimed as executed. `features/real_agent_hooks_test.go` does execute the strict field/type rejection logic in the default run. |
| 37 | This report and the three repository-relative raw captures above. |
| 38 | A real fixed-commit `ci/stability.sh 10` attempt at `a017146` is retained in [`phase2-stability.log`](phase2-stability.log). It failed honestly at **3/10**, exposing timing and SQLite-lock failures; therefore R38 is **not yet proved**. The successful 10/10 fixed-commit run must replace this failed evidence after those defects are fixed. |
| 39 | The run uses the unchanged default tag expression. No Phase 1 scenario was removed; shell assertions were re-aimed to live-shell promotion while `shell_liveness.feature` retains the unsignalled-agent negative assertion. Final protected-file/no-exclusion proof is paired with the fixed-commit stability checkpoint. |
| 40 | The operator walkthrough below. |
| 41 | `cmd/fake-claude: TestPaneCommandsFireEveryInjectedHookWithControllablePayload` and `TestPaneHookCommandRequiresInjectedSettingsRatherThanCallingDeckDirectly`; the Claude-hook feature fires commands from the pane. |
| 42 | `cmd/fake-claude`/`cmd/fake-pi: TestPaneFixtureCommandRendersNamedFileByteForByte` and the complete-corpus black-box scenario. |
| 43 | `internal/config: TestDeckHomeClocksShareOnDemandAdvance`, `TestFrozenClockUsesResolvedDataRootWithoutDeckHome`, plus `status_probe.feature`'s running-client advance. The help overlay documents writing `clock.now`. |
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
real-Claude event availability remains an upstream assumption until the
opt-in scenario is run in that environment.

## Gotchas and consequences

- Frozen time is shared through the resolved data root's `clock.now`; writing
  it, not a TUI key or process-local counter, advances all clients and hook
  subprocesses consistently. Store status writers require explicit time.
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
- The default suite cannot establish real Claude conformance: `@real-agents`
  is intentionally opt-in and no Claude executable was available in this
  environment. Declared events/fields and the exact permission-dialog label
  remain strict upstream assumptions, documented in
  [`phase2-findings.md`](phase2-findings.md), not silently normalized facts.
- The first real fixed-commit stability attempt passed only 3/10. Its retained
  summary names the commit and exit status; the per-run failures included
  multiclient one-second refresh deadlines, PTY shutdown deadlines, hook
  latency over 20 ms, a SQLite busy read, and raced post-resume source
  expectations. R38 remains open rather than being inferred from one green
  full-suite run.
- Shell liveness means `running`; agent pane liveness does not. Re-aimed Phase
  1 shell assertions preserve the original protection with a separate live,
  unsignalled-agent negative scenario.
