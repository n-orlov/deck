# Phase 4 report — codex adapter, chrome legibility, field backlog

Written once, at the tail code sha (task 042; per the standing rules, this record is not
re-audited by any later task). This report is record-only.

- **Tail code sha**: `db669658ce20de10ef6aaad311c94f6830538436` (`db66965`), named in
  `docs/reports/phase4-final-suite/README.md` (task 039) — the most recent commit in history
  touching a `*.go` or `*.feature` file. Confirmed unchanged at commit time:
  `git diff --stat db66965 HEAD -- '*.go' '*.feature'` prints nothing.
- **Protected-path audit** (the PRD's own command, run against the base commit that added
  `prds/phase4-codex-and-chrome.md`, `08a1ffe3eb229f8ebe5ba9791fbb3e3cec6e0c06`):
  `git log --oneline 08a1ffe3eb229f8ebe5ba9791fbb3e3cec6e0c06..HEAD -- SPEC.md prds/ ci/Dockerfile
  ci/SPIKE.md` prints nothing — no commit in this run touched any protected path.

## Tier 1 requirements — R116 through R127

### R116 — the claude `safe` profile composes no `--permission-mode` flag

**Shipped.** `internal/agent/claude.go`'s `claudePermissionArgs` (replacing
`claudePermissionFlag`) returns nil argv for the `safe` profile instead of mapping it to
`--permission-mode manual` (not a real Claude Code CLI value). Commit `d1d075dc` ("agent: claude
safe profile composes no --permission-mode flag (task 001)").

- Test: `TestClaude_LaunchAndResumeArgv` (`internal/agent/claude_test.go`) — table asserts no
  `--permission-mode` flag at all for `safe`.
- Feature: `features/permission_modes.feature`'s `csafe` assertions switched from "contains
  --permission-mode"/"contains manual" to "does not contain --permission-mode".

### R117 — a relaunch that created a pane clears the passive-fit latch

**Shipped.** `sessionResumed`/`sessionRestarted`/the batch-`u` path
(`internal/tui/tui.go`) clear `previewFitSessionID` only when the outcome that fired actually
created a pane (`service.ResumeStarted`) and names the session the latch currently holds; every
no-op outcome leaves it untouched. Commits `f9de4a5c` (task 002, single resume/restart),
`815f2ea7` (task 002, batch `u`), `80ec60b0` (task 003, feature scenario).

- Tests: `TestSessionResumedThatCreatedAPaneClearsTheLatch`,
  `TestSessionRestartedThatCreatedAPaneClearsTheLatch`,
  `TestSessionResumedNoopOutcomesLeaveTheLatchSetAndIssueNoFit`,
  `TestSessionRestartedNoopOutcomesLeaveTheLatchSetAndIssueNoFit`,
  `TestSessionsBulkResumedClearsTheLatchOnlyForItsOwnPaneCreatingEntry`,
  `TestSessionsBulkResumedLeavesTheLatchWhenItNamesNoneOfIts`,
  `TestSessionsBulkResumedClearsTheLatchDespiteAnotherEntrysError`,
  `TestSessionsBulkResumedLeavesTheLatchWhenTheLatchedEntryFailed`
  (all in `internal/tui/preview_fit_resume_latch_test.go`).
- Feature scenario: "a coloured pane's SGR escapes never shear the preview's border" area of
  `features/preview.feature` gained the `r`-resume re-fit scenario (task 003's commit names it
  precisely as "r resume re-fits away from tmux's 80x24 default"); log at
  `docs/reports/phase4-scenario-logs/preview-r117.log` — 14 scenarios (14 passed), 151 steps (151
  passed).

### R118 — one canvas composition helper, applied across panel.go's chrome builders

**Shipped.** `Model.canvasBackground` (`internal/tui/panel.go:511`) is the single composition
helper; `panel.go`'s chrome builders and the settings/dialog frame builders route through it
(commits `7695d704` task 004, `448cecc4` task 005), captured pane content is proven never
repainted (task 006), and every built-in theme's frame-cell assertions are proven per theme
(task 007).

- Tests: `TestCapturedPaneSideBySideKeepsOwnColourFrameCarriesDeckBackground`,
  `TestCapturedPaneStackedKeepsOwnColourFrameCarriesDeckBackground`
  (`internal/tui/preview_pane_repaint_test.go`); `TestProfileSwitchTokensMatchSpec`,
  `TestPinTokensMatchSpec`, `TestRestartChoiceTokensMatchSpec` and their
  `...StyledBodyMatchesPlainBodyOnceStripped` counterparts
  (`internal/tui/profile_pin_restart_theme_test.go`) prove the dialog frame builders compose
  through the same helper.
- Feature: `features/panel_background_themes.feature` — five built-in-theme scenarios (`empire`,
  `daylight`, `matrix`, `cobalt`, `parchment`) each asserting the theme's background token fills
  every deck-owned cell of the frame including pad columns, plus the `NO_COLOR`/
  `DECK_COLOR_DEPTH=16` scenarios; log at
  `docs/reports/phase4-scenario-logs/panel-background-themes.log`.

### R119 — the selection/mark gutter occupies its own columns; four colour states; feature
scenario; contrast floor

**Shipped.** The gutter moved into its own fixed columns outside the row's text run
(`internal/tui/tui.go`/`panel.go`, commit `903418a8` task 008), its four states (plain/selected/
marked/marked-and-selected) and colour rules are proven by unit tests (task 009), a feature
scenario asserts the gutter's cells by background token including `NO_COLOR` glyph survival
(task 010), and the contrast floor gained `background-on-accent`/`background-on-badge` pairs
(task 011, commit `2af27b60`).

- Tests: `TestSidebarGutterPlainRowPaintsNoBar`, `TestSidebarGutterSelectedRowIsAccentWithBackgroundArrow`,
  `TestSidebarGutterMarkedUnselectedRowIsBadgeWithCheck`,
  `TestSidebarGutterMarkedAndSelectedRowStaysAccent` (`internal/tui/sidebar_gutter_color_test.go`);
  `TestStackedSidebarGutterSelectedRowIsAccentWithBackgroundArrow`,
  `TestStackedSidebarGutterMarkedUnselectedRowIsBadgeWithCheck`,
  `TestStackedSidebarGutterGlyphsSurviveNoColor` (`internal/tui/stacked_gutter_test.go`);
  `TestGutterBarContrastFloor` (`internal/theme/contrast_test.go:403`).
- Feature: `features/panel_background_rectangle.feature` — the two gutter-cells scenarios added
  by commit `651ecd1f` (task 010): a colour-enabled client's gutter columns carry accent/badge per
  state across both physical lines of a row's block, and a NO_COLOR client's gutter columns still
  carry the `>`/`*` glyphs as plain text at the same fixed column. Log at
  `docs/reports/phase4-scenario-logs/gutter-cells.log` (also `gutter-rectangle.log` for the
  pre-existing rectangle scenarios this same commit fixed).

### R120 — line 2 renders a bare age with the permission badge last

**Shipped.** `sidebarRowLines` (`internal/tui/tui.go`) now composes the badge segment after
`relativeTime` and drops the `created ` label text entirely. Commit `4c80e5cb` (task 012).

- Test: `TestSidebarCreatedLineRendersDimmed` (`internal/tui/sidebar_hierarchy_test.go`), updated
  to key off the bare "just now" age instead of the `created ` label; `internal/tui/tui_test.go`'s
  ASCII/frozen-view literal assertions and `features/determinism_test.go`/
  `features/sidebar_width_test.go` also updated and re-verified green (`rg -n 'created '
  internal/tui features` shows no remaining rendered-frame assertion for that string, per the
  commit's own verification note).

### R121 — internal/agent/codex.go argv/caps; no conversation id minted/passed/stored;
TranscriptPaths over session CODEX_HOME; forbidden-flag guard

**Shipped**, across four sub-parts:

- Argv and declared capabilities (task 013, commit `0dd7d625`): `TestCodex_Capabilities`,
  `TestCodex_LaunchArgv`, `TestCodex_ResumeArgv`, `TestCodex_UnknownProfile`,
  `TestCodex_LaunchSucceedsWithEmptyConversationID`, `TestCodex_ResumeRefusesEmptyConversationID`
  (`internal/agent/codex_test.go`).
- The create path mints/passes/stores no conversation id for codex (task 014, commit `0288b4c5`):
  `TestCreateAgentConsultsAssignsConversationIDPerAdapter` (`internal/service/agent_test.go:140`)
  — asserts a created codex row's stored `conversation_id` is empty while claude's is not.
- `TranscriptPaths` over a session-supplied `CODEX_HOME` (task 015, commit `eebb2411`):
  `TestCodexTranscriptPathFindsRealFileUnderDefaultHome`,
  `TestCodexTranscriptPathHonoursSessionCodexHomeOverride`, `TestCodexTranscriptPathMissDegrades`
  (`internal/agent/transcript_test.go`).
- Forbidden-flag guard (task 016, commit `8a7cdb0a`): `TestCodex_NeverEmitsForbiddenFlags`
  (`internal/agent/codex_forbidden_flags_test.go`) — asserts Launch/Resume/Instrument argv for
  safe/edits/yolo never contain `--last`, `--full-auto` or
  `--dangerously-bypass-approvals-and-sandbox`.

### R122 — codex Instrument injects five inline hooks and writes nothing

**Shipped.** `Codex.Instrument` (`internal/agent/codex.go`) encodes five inline `-c` hook
overrides and performs no filesystem writes. Commits `9f4908a0`/`be45fddc` (task 017).

- Tests: `TestCodex_InstrumentEncodesInlineHooksNoIO`,
  `TestCodex_InstrumentEmitsGenerationEnvOnlyWhenLeaseHeld`,
  `TestCodex_InstrumentEncoderHandlesQuoteBackslashSpace`,
  `TestCodex_InstrumentCommandExecutesFromAHostileExecutablePath`,
  `TestCodex_InstrumentWritesNoFilesAnywhereUnderCodexHome`,
  `TestCodex_InstrumentCommandCarriesNoSessionSpecificSubstring`
  (`internal/agent/codex_test.go`).

### R123 — the receiver maps PermissionRequest to waiting with tool_name as its reason

**Shipped.** One new `Mappings` entry (`PermissionRequest` → `waiting`, `ReasonField:
tool_name`) and one new payload field (`ToolName`, json `tool_name`) — no second receiver, no
per-agent mapping table, no codex payload struct. Commit `1ba47a57` (task 018).

- Test: `TestReceiveCodexFiveEvents` (`internal/hookrecv/receiver_codex_test.go`) — drives
  `Receive` with payload text copied verbatim from the codex-cli spike doc's Q3c cases (Bash and
  apply_patch tool names), asserting `wantStatus: "waiting"` and `wantReason` equal to the tool
  name in each case.

### R124 — codex probe rules fitted to the supplied corpus

**Shipped.** `internal/agent/probe.go`'s rule table extended to cover the codex real-capture
corpus. Commit `af407a4e` (task 019).

- Test: `TestProbeGoldenPaneCorpus` (`internal/agent/probe_test.go:45`) — runs every golden pane
  capture in the corpus through the probe and asserts the expected verdict;
  `TestProbeRuleTableHasOneRulePerGolden` (`probe_test.go:153`) pins the rule-table shape as a
  guard against an invented probe rule with no corresponding fixture.

### R125 — a codex row with no id yet is a state, not an error

**Shipped.** Commit `8c8e0c28` (task 023): `internal/tui`, `internal/service` and
`internal/hookrecv` each treat an id-less codex row as an ordinary state.

- Tests: `TestResumeRefusesACodexRowWithNoConversationIDYet`,
  `TestRestartRefusesALiveCodexRowWithNoConversationIDYet`,
  `TestResumeAndRestartStillWorkOnceCodexHasAConversationID`,
  `TestDetailDialogShowsMissingCodexConversationIDHonestly`,
  `TestStatusSourceQualityReadsSampledForAnIDlessCodexRowWithNoPerKindBadge`
  (`internal/tui/codex_no_id_test.go`);
  `TestKillingAndRecreatingAnIDlessCodexRowIsAFreshLaunchNotAResume`
  (`internal/service/codex_no_id_test.go`);
  `TestReceiveCodexSessionStartAdoptsTheRowsConversationID`,
  `TestReceiveCodexSupersededSessionStartDoesNotMoveTheConversationID`,
  `TestReceiveCodexSecondSessionStartWithTheSameIDIsANoOpThatLogsNoChange`
  (`internal/hookrecv/receiver_codex_test.go`).

### R126 — cmd/fake-codex argv contract, silent trust gate; session lifecycle, rollout
transcript, pane text; PATH-builder registration and drift alarm

**Shipped**, across three sub-parts:

- Argv contract and silent hook-trust gate (task 020, commit `16bcf63e`):
  `TestRejectsSessionIDAndFullAuto`, `TestAcceptsResumeAsPositionalSubcommand`,
  `TestAcceptsOnlyDocumentedApprovalValues`, `TestAcceptsOnlyDocumentedSandboxValues`,
  `TestAcceptsRepeatedConfigOverrides`, `TestParseHookOverrideRoundTripsAgainstTheRealEncoder`,
  `TestHookTrustGateFiresOnlyWhenBypassFlagIsPresent`,
  `TestPaneHookCommandRequiresInjectedOverrideEvenWhenTrusted`,
  `TestExitCodeIsControlledOnlyByFixtureEnvironment`, `TestUnknownOptionIsRejected`
  (`cmd/fake-codex/main_test.go`).
- Session lifecycle, rollout transcript and pane text (task 021, commit `2b31afdc`):
  `TestSessionStartNeverFiresAtLaunchOnlyAfterTheFirstPrompt`,
  `TestSessionLifecycleFiresAllFiveEventsWithRealFieldNames`,
  `TestMintsADistinctUUIDPerInvocation`, `TestResumeReusesTheGivenIDWithSourceResume`,
  `TestRolloutTranscriptIsWrittenAndLocatedByTranscriptPaths`,
  `TestPaneStateCommandsMatchEveryCodexProbeVerdict`,
  `TestPermissionAndStopRequireAPromptFirst`,
  `TestExitCommandFiresSessionEndOnlyOnceASessionHasStarted`,
  `TestCleanExitModeEndsTheProcessWithStatusZero`,
  `TestHangModeBlocksOnThePaneUntilACommandOrEOFArrives`,
  `TestCrashModeIsTheSameNonzeroExitCodeControlAsFakeClaude` (`cmd/fake-codex/main_test.go`).
- Fixture PATH-builder registration and drift alarm (task 022, commit `8471807c`):
  `installFakeCodexOnPATH` (and its future-client/removed variants) registered in
  `features/agent_steps_test.go`'s idiom mirroring claude/pi; `features/fake_agent_drift.feature`
  gained the "fake Codex flags conform to installed Codex CLI" scenario. This scenario is tagged
  `@real-agents` at the feature level and is **skipped in this container** — no real `codex` CLI
  is installed here, so godog's default tag filter (`~@real-agents && ~@nightly`) excludes it
  entirely (confirmed: `docs/reports/phase4-scenario-logs/fake-agent-drift-codex.log` reports "No
  scenarios, No steps" under that filter, by design, exactly as the claude/pi drift scenarios are
  also skipped here).

### R127 — features/codex_hooks.feature with SPEC §13.4's @codex scenario; codex covered in
the ordinary feature places; one @real-agents codex conformance scenario

**Shipped**, across three sub-parts:

- `features/codex_hooks.feature` (task 024, commit `78220185`): one `@codex`-tagged scenario,
  "two Codex rows in one directory adopt their own ids and report a real approval", proving in
  order that neither row mints a conversation id at creation, and that a permission request maps
  to `waiting` naming the tool. Log:
  `docs/reports/phase4-scenario-logs/codex-hooks.log` — 1 scenario (1 passed), 27 steps (27
  passed).
- Codex in the ordinary feature places (task 025, commit `ed40e426`/`a11cc860`):
  `features/agent_availability.feature` (codex in the not-on-PATH list and offered/withheld
  scenarios), `features/permission_modes.feature` (codex profile-degradation-through-
  `ResolveProfile` scenarios), `features/agent_session.feature`-equivalent resume-argv scenario,
  `features/kill_delete_undo.feature`. Logs:
  `docs/reports/phase4-scenario-logs/agent_availability-task025.log` (6 scenarios, all PASS,
  including "with only codex installed the Agent field offers shell and codex but not claude or
  pi"), `permission_modes-task025.log` (6 scenarios, all PASS, including "codex degrades an
  unsupported plan profile to safe, visibly, through ResolveProfile"),
  `agent_session-task025.log` (6 scenarios, all PASS, including "R restarts a running codex
  session with the resume argv, never composing --last"), `kill_delete_undo-task025.log` (6
  scenarios, all PASS).
- One `@real-agents` codex conformance scenario (task 026, commit `50e07029`):
  `features/real_agent_smoke.feature` gained one scenario tagged
  `@codex-real-agent-conformance` (feature-level `@real-agents`), "create a real codex session
  and confirm the injected hook contract, skipping cleanly without an installed CLI". Log:
  `docs/reports/phase4-scenario-logs/real-agents-codex-skip.log` — 1 scenario (1 passed), 9 steps
  (9 skipped) — the scenario itself passes by cleanly skipping every step once it detects no real
  `codex` CLI is installed in this container, exactly as its title states.

## Tier 2 — status stated plainly: NOT STARTED

**Tier 2 (R128–R131, tasks 029–038) was not started.** Task 028's decision record
(`docs/reports/phase4-tier2-decision.md`, commits `f143286`/`1e6a465`) made the call at
iteration 105, citing the Tier 1 gate's PASS at `db66965` as the precondition and the wall-clock
budget at that moment: 699/800 iterations remaining but only ~15h15m to the deadline
(`2026-09-17T11:00:02Z`), against an observed Tier-1 pace of ~3.1 iterations/~24min per task and
a conservative 8–12 hour estimate for Tier 2's ten materially-larger tasks (an all-or-nothing
store schema migration, a four-call-site sidebar grouping-model rewrite plus flat-mode removal,
and a full settings groups CRUD surface) on top of the mandatory tail (039 final gate, 040
guards, 041's ~75-minute ten-run stability sweep, 042–044 reports). Tasks 029–038 each read that
decision and landed no code, resolved via the plan's own `tier2_conditionality` escape clause.

**R128 (task 42's own criterion requires this stated as one paragraph): the SPEC-versus-code
grouping gap.** SPEC's group-based session organization (a `groups` table, `sessions.group_id`,
sidebar grouping by group id, a create-modal Group field, and a settings groups CRUD section)
remains unimplemented. The code continues to group sessions by workspace instead, via
`store.go`'s `DefaultWorkspace` field and `internal/tui`'s workspace-based grouping gated by the
`ui.group_by_workspace` config key — the exact mechanism R129 would have required removing (flat
mode, `group_by_workspace`, `reorderPreservingGrouping`). This is the plan's explicitly disclosed,
accepted, **not-a-finding** consequence of the Tier-1/Tier-2 budget decision — it is not scored in
`docs/reports/phase4-findings.md`, per task 028's decision record and the standing rules' own
statement of that same rule.

## Both gates

### Final gate sweep (task 039)

- **Command**: `ci/run.sh sh -c 'go test -p=1 -count=1 -timeout=40m ./...'` — every package, no
  `-run` filter, no package list.
- **Tail code sha**: `db66965`.
- **Duration**: 7m01s (421s), `2026-09-16T19:55:07Z` → `2026-09-16T20:02:08Z`.
- **Result**: PASS. All 18 packages report `ok` or `[no test files]`; exit `0`; no `FAIL` line.
- **Evidence**: `docs/reports/phase4-final-suite/{README.md,suite.log}`.

(The Tier 1 gate sweep, task 027, ran the identical command at the identical sha before Tier 2's
decision was made, also PASS, 7m00s; evidence at `docs/reports/phase4-tier1-suite/{README.md,
suite.log}` — the two are the same tree since no code landed in between.)

### Ten-run stability sweep (task 041)

- **Command**: `ci/stability.sh 10` (ten independent runs of `ci/run.sh go test -p=1 -count=1
  ./...`, `-count=1` disables the test cache, each run its own `--rm` sibling).
- **Tail code sha**: `db66965` (unchanged from the final gate).
- **Duration**: ~74 minutes total (`2026-09-16T20:10:xxZ` → `21:24:xxZ`), each run ~6m10s–6m30s.
- **Result**: **7/10 PASS.** Runs 4, 8, 9 FAIL on `TestGoldenMinimumFrame`
  (`features/golden_frame_test.go:74`, "frame kept changing... not settled") — the known-open
  transient-`starting`-assertion flake class named in task 041's own criteria, a settle-check
  race between the client's resize reflow and preview/reconcile ticks (task 210's already-
  documented quiescence race), advisory and not a new defect. No occurrence of the other named
  flake class (`TestSigwinchCountDistinguishesTwoFromThree`) in this sweep. No other
  package/scenario failed in any of the ten runs.
- **Evidence**: `docs/reports/phase4-stability10/{README.md,run-1..10.log,summary.log}`.

### Guard evidence (task 040)

- **Commands** (each via `ci/run.sh`, full scope, no filter): `go build ./...`, `go vet ./...`,
  `gofmt -l .`.
- **Tail code sha**: `db66965`.
- **Result**: `go build ./...` and `go vet ./...` both exit `0` with empty output. `gofmt -l .`
  exits `0` and lists exactly the four **pre-existing** drift files (measured at `08a1ffe`, per
  the standing rules — none reformatted by this run):
  - `internal/theme/quantize_test.go`
  - `.spike-preview/cmd/conformance/main.go`
  - `.spike-preview/conformance/conformance.go`
  - `.spike-preview/conformance/conformance_test.go`

  No other path appears in the `gofmt -l .` output.
- **Evidence**: `docs/reports/phase4-guards/README.md`.

## Summary

Every Tier 1 requirement (R116–R127) is green, each cited above against its own commit(s) and
test(s). Tier 2 (R128–R131) was not started, an accepted budget outcome recorded at task 028 and
not scored as a finding. Both mandatory gates (final full-suite sweep and the ten-run stability
sweep) are reported above with their commands, durations and the shared tail code sha `db66965`,
and the guard evidence (build/vet/gofmt) is reported with the four pre-existing gofmt-drift files
named as pre-existing, not new drift from this run.
