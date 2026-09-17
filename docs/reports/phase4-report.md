# Phase 4 report — codex adapter, chrome legibility, field backlog

**Approach 3 (narrow cure of approach 2's rejection).** Written once, at this approach's final
tail code sha (per the standing rules, this record is not re-audited by any later task).
This report is record-only. Approach 1's own record (`docs/reports/phase4-final-suite/`,
`phase4-guards/`, `phase4-stability10/`) and approach 2's own record
(`docs/reports/phase4-cure-final-suite/`, `phase4-cure-guards/`, `phase4-cure-stability10/`,
and the two prior versions of this file superseded by this rewrite, at `db66965` and
`0ba550a` respectively) stay as history and are not edited.

- **Tail code sha**: `3568bd7971a782fadbf589d79ce5777c0f1b5315` (`3568bd7`, `tui: paint the
  interactive preview branch's own notice and pad rows (task 002)`), named in
  `docs/reports/phase4-a3-final-suite/README.md` (task 003) — the most recent commit in this
  run's history touching a `*.go` or `*.feature` file, and this approach's own last
  code-touching task. Confirmed unchanged at report-writing time:
  `git diff --stat 3568bd7971a782fadbf589d79ce5777c0f1b5315 HEAD -- '*.go' '*.feature'` prints
  nothing.
- **Protected-path audit** (the PRD's own command, run against the base commit that added
  `prds/phase4-codex-and-chrome.md`, `08a1ffe3eb229f8ebe5ba9791fbb3e3cec6e0c06`, computed
  fresh, never pasted):
  `git log --oneline 08a1ffe3eb229f8ebe5ba9791fbb3e3cec6e0c06..HEAD -- SPEC.md prds/
  ci/Dockerfile ci/SPIKE.md` prints nothing — no commit in any approach of this run touched
  any protected path.

## Tier 1 requirements — R116 through R127

Each verdict below cites this approach's own commits and tests for anything this approach
touched (R118, cured twice more this approach); every other requirement is unchanged from
approach 2's already-`validated` work (itself unchanged from approach 1 except where approach
2's own B0/B1/B2/B3/R1 cures are noted) and keeps its prior citation.

### R116 — the claude `safe` profile composes no `--permission-mode` flag

**Shipped**, unchanged from approach 2 — not in this approach's scope. `internal/agent/claude.go`'s
`claudePermissionArgs` returns nil argv for the `safe` profile (commit `d1d075dc`, approach 1).
Its comments — and `claudeProfileFlags`'s — state the version-rename rationale (Claude Code
renamed the mode's own spelling from `default` to `manual` somewhere between CLI versions
2.1.71 and 2.1.259; commit `cf2d53b`, approach 2 task 010).

- Test: `TestClaude_LaunchAndResumeArgv` (`internal/agent/claude_test.go`) — table asserts no
  `--permission-mode` flag at all for `safe`.
- Feature: `features/permission_modes.feature`'s `csafe` assertions assert absence of
  `--permission-mode`.

### R117 — a relaunch that created a pane clears the passive-fit latch

**Shipped**, unchanged from approach 1 — not in this approach's scope.
`sessionResumed`/`sessionRestarted`/the batch-`u` path (`internal/tui/tui.go`) clear
`previewFitSessionID` only when the outcome that fired actually created a pane. Commits
`f9de4a5c`, `815f2ea7`, `80ec60b0` (approach 1).

- Tests: `TestSessionResumedThatCreatedAPaneClearsTheLatch`,
  `TestSessionRestartedThatCreatedAPaneClearsTheLatch`,
  `TestSessionResumedNoopOutcomesLeaveTheLatchSetAndIssueNoFit`,
  `TestSessionRestartedNoopOutcomesLeaveTheLatchSetAndIssueNoFit`,
  `TestSessionsBulkResumedClearsTheLatchOnlyForItsOwnPaneCreatingEntry`,
  `TestSessionsBulkResumedLeavesTheLatchWhenItNamesNoneOfIts`,
  `TestSessionsBulkResumedClearsTheLatchDespiteAnotherEntrysError`,
  `TestSessionsBulkResumedLeavesTheLatchWhenTheLatchedEntryFailed`
  (`internal/tui/preview_fit_resume_latch_test.go`).
- Feature scenario: the "r resume re-fits away from tmux's 80x24 default" scenario in
  `features/preview.feature`.

### R118 — one canvas composition helper, applied across panel.go's chrome builders, now covering every deck-owned preview cell

**Shipped**, cured further this approach (a same-class B1 residual review had not itself
listed). Approach 2 gave `previewBodyLines`' no-session/placeholder/blank-fill and captured-row
fill/crop-marker cells their own provenance and painted the deck-owned ones (commits `96b0ba9`,
`0e72ec1`, `4614bff`, approach 2 tasks 002-004). Two further deck-owned surfaces were found
still blanket-marked foreign, one at plan time for this approach and one already in review's
own list:

- **`cropPreviewBottomLeft`'s own geometry line and vertical blank-fill rows** (`panel.go`):
  `previewBodyLines`' live-capture branch marked the *whole* crop slice
  `foreignPreviewLines(len(lines))`, so the geometry line (`WxH of realWxrealH`) and the
  synthesized blank-fill rows below a short capture leaked the terminal's own background
  instead of `theme.Background`. Task 001 (`92619cf`, fixture correction `48bce3d`) gave
  `cropPreviewBottomLeft` its own per-row `[]previewLineOwner` (the geometry line and blank-fill
  rows `previewLineDeckOwned`, every `cropRow`-built row `previewLineForeign`), routed through
  `previewContentLine` and `fullBoxPreviewContentLine` in both frames, over `theme.Builtins()`.
  Test: `TestCropDecorationsCarryDeckBackground`
  (`internal/tui/crop_decoration_background_test.go`) — a live pane 2 rows tall (shorter than
  the preview content height, forcing blank-fill rows) and wide enough to force the geometry
  line, whose first captured row leaves an SGR attribute open; asserts the geometry line and a
  blank-fill row carry `theme.Background` across their interior span in both layouts over all
  five built-in themes, while the capture's own rows — including the one leaving an attribute
  open — keep the pane's own foreground/background untouched.
- **The interactive-preview branch's own notice and pad rows** (`interactive.go`), same class,
  found at plan time and not itself in review's own findings list: `interactiveBodyLines`
  prepended `interactiveNotRepaintedNotice` and padded via `fitLines`, both deck's own composed
  copy, but `previewBodyLines`' interactive branch also blanket-marked the whole slice foreign.
  Task 002 (`3568bd7`) gave `interactiveBodyLines` the same per-row provenance treatment,
  factoring the notice-then-pad/truncate sequence into `fitInteractiveBodyLines(lines,
  contentHeight, notice)` so it is directly testable; grid/highlight rows stay
  `previewLineForeign`, the notice line and any pad row are `previewLineDeckOwned`.
  Tests: `TestFitInteractiveBodyLinesOwnership` (`internal/tui/interactive_test.go`, both the
  blank-grid notice case and the short-grid pad case) and
  `TestInteractiveNotRepaintedNoticeCarriesDeckBackground`
  (`internal/tui/interactive_notice_background_test.go`, full `View()` over `theme.Builtins()`
  in both frames via a real quiet tmux pane).

- Tests carried over unchanged from approach 2:
  `TestNoLiveCapturePreviewInteriorCarriesDeckBackgroundSideBySide`,
  `TestNoLiveCapturePreviewInteriorCarriesDeckBackgroundStacked`
  (`internal/tui/preview_no_capture_background_test.go`);
  `TestCapturedPaneFillPastCaptureCarriesDeckBackground`,
  `TestCapturedPaneCropMarkerCarriesDeckBackground`
  (`internal/tui/preview_pane_fill_marker_test.go`); the pre-existing
  `TestCapturedPaneSideBySideKeepsOwnColourFrameCarriesDeckBackground` and its stacked
  counterpart `TestCapturedPaneStackedKeepsOwnColourFrameCarriesDeckBackground`
  (`internal/tui/preview_pane_repaint_test.go`, approach 1), which continue to prove a
  capture's own cells are never repainted.
- Feature: `features/panel_background_themes.feature`, `features/preview.feature` and
  `features/panel_background_rectangle.feature` all pass at this approach's tail sha
  (part of the full-suite gate below; task 002's own commit message additionally quotes each
  feature file passing individually).

### R119 — the selection/mark gutter occupies its own columns; four colour states; feature scenario; contrast floor

**Shipped**, unchanged from approach 1 — not in this approach's scope. Commits `903418a8`,
`06b4521d`/`8d4efd7c`, `651ecd1f`, `2af27b60` (approach 1).

- Tests: `TestSidebarGutterPlainRowPaintsNoBar`, `TestSidebarGutterSelectedRowIsAccentWithBackgroundArrow`,
  `TestSidebarGutterMarkedUnselectedRowIsBadgeWithCheck`, `TestSidebarGutterMarkedAndSelectedRowStaysAccent`
  (`internal/tui/sidebar_gutter_color_test.go`); `TestStackedSidebarGutterSelectedRowIsAccentWithBackgroundArrow`,
  `TestStackedSidebarGutterMarkedUnselectedRowIsBadgeWithCheck`, `TestStackedSidebarGutterGlyphsSurviveNoColor`
  (`internal/tui/stacked_gutter_test.go`); `TestGutterBarContrastFloor` (`internal/theme/contrast_test.go`).
- Feature: `features/panel_background_rectangle.feature`'s gutter-cells scenarios (commit
  `651ecd1f`).

### R120 — line 2 renders a bare age with the permission badge last, badge shown only for a non-`safe` profile

**Shipped**, unchanged from approach 2 — not in this approach's scope. `sidebarRowLines`
(`internal/tui/tui.go`) composes the badge segment after `relativeTime` with no `created `
label (approach 1, commit `4c80e5cb`); the badge's `safe`-suppression rule
(`profileBadgeSegment` and its caller, citing SPEC.md:1339) was added in approach 2, commit
`e93a790`.

- Tests: `TestSidebarCreatedLineRendersDimmed` (`internal/tui/sidebar_hierarchy_test.go`,
  approach 1); `TestSidebarRowHidesSafeBadgeButKeepsNonSafeBadgeAtMinimumWidth`
  (`internal/tui/profile_badge_safe_hidden_test.go`, approach 2) asserts both the `safe`
  no-badge case and the `plan`/`edits`/`yolo` badge-last case at the sidebar's minimum
  content width.

### R121 — internal/agent/codex.go argv/caps; no conversation id minted/passed/stored; TranscriptPaths over an agent-neutral env seam; forbidden-flag guard

**Shipped**, unchanged from approach 2 — not in this approach's scope.

- Argv and declared capabilities, create-path identity, forbidden-flag guard: unchanged from
  approach 1 (commits `0dd7d625`, `0288b4c5`, `8a7cdb0a`). Tests: `TestCodex_Capabilities`,
  `TestCodex_LaunchArgv`, `TestCodex_ResumeArgv`, `TestCodex_UnknownProfile`,
  `TestCodex_LaunchSucceedsWithEmptyConversationID`, `TestCodex_ResumeRefusesEmptyConversationID`
  (`internal/agent/codex_test.go`); `TestCreateAgentConsultsAssignsConversationIDPerAdapter`
  (`internal/service/agent_test.go`); `TestCodex_NeverEmitsForbiddenFlags`
  (`internal/agent/codex_forbidden_flags_test.go`).
- Transcript resolution over an agent-neutral env seam: approach 2 replaced
  `TranscriptInput.CodexHome` with `agent.Caps.TranscriptEnvKeys []string` (an adapter's own
  declaration) and a generic `Env map[string]string` (commit `e69c3d8`); codex is the only
  adapter that declares a key (`CODEX_HOME`) and its `TranscriptPaths` still reads only from
  that map. Approach 2 extended the black-box registry-swap guard to this capability (commit
  `ef9571d`) and regression-tested the production caller with three competing home values live
  at once (commit `1e97059`). Tests: `TestCodexTranscriptPathFindsRealFileUnderDefaultHome`,
  `TestCodexTranscriptPathHonoursSessionCodexHomeOverride`, `TestCodexTranscriptPathMissDegrades`
  (`internal/agent/transcript_test.go`, approach 1, unaffected by the seam change);
  `TestBlackBoxRegistrySwapTranscriptEnvKeyNeedsNoTUIEdit`
  (`internal/tui/registry_guard_test.go`, approach 2);
  `TestTranscriptPathForCodexPrefersSessionEnvOverConfigAndAmbient`
  (`internal/tui/transcript_env_layers_test.go`, approach 2).

### R122 — codex Instrument injects five inline hooks and writes nothing

**Shipped**, unchanged from approach 1 — not in this approach's scope. Commits `9f4908a0`/
`be45fddc` (approach 1).

- Tests: `TestCodex_InstrumentEncodesInlineHooksNoIO`,
  `TestCodex_InstrumentEmitsGenerationEnvOnlyWhenLeaseHeld`,
  `TestCodex_InstrumentEncoderHandlesQuoteBackslashSpace`,
  `TestCodex_InstrumentCommandExecutesFromAHostileExecutablePath`,
  `TestCodex_InstrumentWritesNoFilesAnywhereUnderCodexHome`,
  `TestCodex_InstrumentCommandCarriesNoSessionSpecificSubstring`
  (`internal/agent/codex_test.go`).

### R123 — the receiver maps PermissionRequest to waiting with tool_name as its reason

**Shipped**, unchanged from approach 1 — not in this approach's scope. Commit `1ba47a57`.

- Test: `TestReceiveCodexFiveEvents` (`internal/hookrecv/receiver_codex_test.go`).

### R124 — codex probe rules fitted to the supplied corpus

**Shipped**, unchanged from approach 1 — not in this approach's scope. Commit `af407a4e`.

- Tests: `TestProbeGoldenPaneCorpus`, `TestProbeRuleTableHasOneRulePerGolden`
  (`internal/agent/probe_test.go`).

### R125 — a codex row with no id yet is a state, not an error

**Shipped**, unchanged from approach 1 — not in this approach's scope. Commit `8c8e0c28`.

- Tests: `TestResumeRefusesACodexRowWithNoConversationIDYet`,
  `TestRestartRefusesALiveCodexRowWithNoConversationIDYet`,
  `TestResumeAndRestartStillWorkOnceCodexHasAConversationID`,
  `TestDetailDialogShowsMissingCodexConversationIDHonestly`,
  `TestStatusSourceQualityReadsSampledForAnIDlessCodexRowWithNoPerKindBadge`
  (`internal/tui/codex_no_id_test.go`); `TestKillingAndRecreatingAnIDlessCodexRowIsAFreshLaunchNotAResume`
  (`internal/service/codex_no_id_test.go`); `TestReceiveCodexSessionStartAdoptsTheRowsConversationID`,
  `TestReceiveCodexSupersededSessionStartDoesNotMoveTheConversationID`,
  `TestReceiveCodexSecondSessionStartWithTheSameIDIsANoOpThatLogsNoChange`
  (`internal/hookrecv/receiver_codex_test.go`).

### R126 — cmd/fake-codex argv contract, silent trust gate; session lifecycle, rollout transcript, pane text; PATH-builder registration and drift alarm

**Shipped**, unchanged from approach 1 — not in this approach's scope. Commits `16bcf63e`,
`2b31afdc`, `8471807c`.

- Tests: `TestRejectsSessionIDAndFullAuto`, `TestAcceptsResumeAsPositionalSubcommand`,
  `TestAcceptsOnlyDocumentedApprovalValues`, `TestAcceptsOnlyDocumentedSandboxValues`,
  `TestAcceptsRepeatedConfigOverrides`, `TestParseHookOverrideRoundTripsAgainstTheRealEncoder`,
  `TestHookTrustGateFiresOnlyWhenBypassFlagIsPresent`,
  `TestPaneHookCommandRequiresInjectedOverrideEvenWhenTrusted`,
  `TestExitCodeIsControlledOnlyByFixtureEnvironment`, `TestUnknownOptionIsRejected`,
  `TestSessionStartNeverFiresAtLaunchOnlyAfterTheFirstPrompt`,
  `TestSessionLifecycleFiresAllFiveEventsWithRealFieldNames`, `TestMintsADistinctUUIDPerInvocation`,
  `TestResumeReusesTheGivenIDWithSourceResume`, `TestRolloutTranscriptIsWrittenAndLocatedByTranscriptPaths`,
  `TestPaneStateCommandsMatchEveryCodexProbeVerdict`, `TestPermissionAndStopRequireAPromptFirst`,
  `TestExitCommandFiresSessionEndOnlyOnceASessionHasStarted`, `TestCleanExitModeEndsTheProcessWithStatusZero`,
  `TestHangModeBlocksOnThePaneUntilACommandOrEOFArrives`, `TestCrashModeIsTheSameNonzeroExitCodeControlAsFakeClaude`
  (`cmd/fake-codex/main_test.go`). The `fake_agent_drift.feature` codex scenario stays
  `@real-agents`-tagged and is skipped in this container (no installed `codex` CLI), by design,
  unchanged from approach 1.

### R127 — features/codex_hooks.feature with SPEC §13.4's @codex scenario; codex covered in the ordinary feature places; one @real-agents codex conformance scenario

**Shipped**, unchanged from approach 2 — not in this approach's scope.

- `features/codex_hooks.feature`'s one `@codex`-tagged scenario carries the attribution and
  timing oracles approach 2 added: each pane's own authoritative SessionStart identity
  (`session_id`/`transcript_path`), a negative-control Go test
  (`TestCodexIdentityMismatchCatchesSwappedStoredIDs`, `features/codex_hooks_swap_test.go`),
  and the two-second creation-bound/same-cwd assertions read from the store's own columns
  (commits `2ff6024`, `98ac4e7`, approach 2 tasks 008/009).
- Codex in the ordinary feature places (approach 1, commits `ed40e426`/`a11cc860`):
  `features/agent_availability.feature`, `features/permission_modes.feature`,
  `features/agent_session.feature`, `features/kill_delete_undo.feature` — unchanged, not in
  this approach's scope.
- One `@real-agents` codex conformance scenario (approach 1, commit `50e07029`):
  `features/real_agent_smoke.feature`'s `@codex-real-agent-conformance` scenario — unchanged,
  not in this approach's scope.

## Tier 2 — status stated plainly: NOT STARTED

**Tier 2 (R128–R131) was not started in this approach.** At plan time (`2026-09-17T04:58Z`)
approximately 6 hours of wall clock remained against the deadline
`2026-09-17T11:00:06Z`, while the two mandatory sweeps this approach's own tasks 003 and 004
require — the full-suite gate (measured at ~7m21s/441s) and the ten-run stability sweep
(measured at ~1h12m) — alone consume roughly 1h20m of that budget. Against that remaining
wall clock, this approach's scope was bounded to review's one remaining blocking finding (B1's
same-class residual, tasks 001–002) plus residual R2's citations and the record (tasks 005–009):
a materially smaller, lower-risk body of work than Tier 2's four requirements, which remain an
all-or-nothing store-schema migration, a four-call-site sidebar grouping-model replacement with
flat-mode removal, a create-modal field, an `i`-dialog move path, and a full settings groups
CRUD surface — the same larger surface approaches 1 and 2 already declined to start under
comparable deadlines. No Tier 2 code landed in this approach; this decision only re-affirms,
and does not narrow, the not-started status approaches 1 and 2 already recorded.

**(One paragraph, as this report's own criterion requires): the SPEC-versus-code grouping gap,
unchanged from approaches 1 and 2.** SPEC's group-based session organization (a `groups` table,
`sessions.group_id`, sidebar grouping by group id, a create-modal Group field, and a settings
groups CRUD section) remains unimplemented. The code continues to group sessions by workspace
instead, via `store.go`'s `DefaultWorkspace` field and `internal/tui`'s workspace-based
grouping gated by the `ui.group_by_workspace` config key — the exact mechanism R129 would have
required removing (flat mode, `group_by_workspace`, `reorderPreservingGrouping`). This is the
plan's explicitly disclosed, accepted, **not-a-finding** consequence of the Tier-1/Tier-2
budget decision, re-affirmed rather than narrowed by this approach: it is not scored in
`docs/reports/phase4-findings.md`, and it is never "cured" by editing SPEC — SPEC.md is a
protected path in this run and stays untouched.

## R132 — the record matches the tree (T1)

**Shipped for this approach's own record, at the tail code sha `3568bd7`.** R132 is the only
requirement whose deliverable is this record itself, so its verdict is stated against its own
four bullets:

- **`docs/reports/phase4-report.md` states, per requirement, what shipped, the commits and the
  tests that prove it, and for anything that did not ship what is missing and why.** Green —
  this file, rewritten from scratch under task 005 at `3568bd7` (it previously stood at
  approach 2's `0ba550a`). Every requirement number R116–R132 carries a verdict above:
  R116–R127 individually, R128–R131 as the explicitly grouped **NOT STARTED** verdict in the
  Tier 2 section (with the wall-clock budget behind it and the SPEC-versus-code grouping gap
  in the one disclosed paragraph that section requires), and R132 here. Every commit sha and
  every `Test*`/scenario name this file cites was checked to resolve in the tree at report-
  writing time (`git cat-file -e <sha>^{commit}` per sha, `grep` per name).
- **`docs/reports/phase4-report.md`'s review-findings section records B1's and B0's
  dispositions.** Green — see "Review findings from this approach's plan gate (task 006)"
  below: B1 cured by tasks 001/002 with their commits and tests; B0's disposition read from
  the identified reporting snapshot (tail code sha, `/run/ralphd/steering`, this run's notify
  record), citing `ci/review.sh` and `docs/reports/phase4-review-protocol.md`, with no operator
  authorization found and exactly what is needed named.
- **`docs/reports/phase4-findings.md` carries every finding this run made and chose not to
  fix, each with a file:line and a reason, including the two known-unverified codex items by
  name.** Not yet at this approach's tail — that is task 008's own record task, following
  task 007's R2 citation correction.
- **`docs/DELIVERY-LOG.md` gains this approach's entry in the existing shape.** Not yet at
  this approach's tail either; it is the last record task (009).
- **Both gates are reported with their commands, their durations and the sha they ran at — the
  final code sha per Materiality's termination rule.** Green — see "Both sweeps" immediately
  below: the full-suite gate plus build/vet/gofmt guards (task 003) and the ten-run stability
  sweep (task 004), each with its command as run, its duration, the shared tail code sha
  `3568bd7`, and its own committed directory under `docs/reports/`.

This report's own remaining bullets are docs-only work on top of `3568bd7`. Per the PRD's own
Materiality termination rule (and the standing rules that restate it) a docs-only tail commit
invalidates neither sweep and is itself exempt from re-verification, so tasks 006–009 land
against this same tail code sha and do not reopen the sweeps reported below. This report is
written once, at that sha, and is not re-audited by any later task.

## Review findings from this approach's plan gate (task 006)

This section states, for each of the plan gate's two blocking findings, what this approach did
about it and why. Neither disposition below is written ahead of the paint or evidence it
describes -- B1's cure commits already carry the fix and its test; B0's disposition is read from
a named, dated snapshot of this run's own record, not asserted from memory.

### B1 -- R118 omitted deck-generated live-capture geometry/vertical-fill and the interactive branch's own notice/pad rows -- **cured this approach**

At this approach's plan gate, B1 was blocking: `cropPreviewBottomLeft`'s own geometry line and
synthesized vertical blank-fill rows (`internal/tui/panel.go`) were deck-generated but
`previewBodyLines`'s live-capture branch marked the *whole* crop slice foreign, so those cells
leaked the terminal's own background instead of `theme.Background`. This section does not record
that gap as an accepted, disclosed residual (it was never optional Tier-2 scope, and
`docs/reports/phase4-findings.md`'s prior entry calling it one is task 008's own item to remove,
not this section's business) -- B1 was blocking review, and a blocking finding is cured, not
accepted.

- **Task 001** (commits `92619cf` -- "tui: paint the crop geometry line and blank fill (task
  001)" -- and `48bce3d`, a fixture correction making the crop-decoration test pane genuinely
  short enough to force the blank-fill path) gave `cropPreviewBottomLeft` its own per-row
  `[]previewLineOwner` (the geometry line and blank-fill rows `previewLineDeckOwned`, every
  `cropRow`-built row `previewLineForeign`), routed through `previewContentLine` and
  `fullBoxPreviewContentLine` in both layouts. New test: `TestCropDecorationsCarryDeckBackground`
  (`internal/tui/crop_decoration_background_test.go`).
- **Task 002** (commit `3568bd7` -- "tui: paint the interactive preview branch's own notice and
  pad rows (task 002)") cured the same class of gap in the interactive-preview branch, found at
  this approach's own plan time and not itself in review's B1 finding text: `interactiveBodyLines`
  (`internal/tui/interactive.go`) prepended `interactiveNotRepaintedNotice` and padded via
  `fitLines`, both deck's own composed copy, but the interactive branch of `previewBodyLines` also
  blanket-marked the whole slice foreign. Task 002 gave `interactiveBodyLines` the same per-row
  provenance treatment via `fitInteractiveBodyLines(lines, contentHeight, notice)`. New tests:
  `TestFitInteractiveBodyLinesOwnership` (`internal/tui/interactive_test.go`) and
  `TestInteractiveNotRepaintedNoticeCarriesDeckBackground`
  (`internal/tui/interactive_notice_background_test.go`).

Both commits, and both sets of new tests, are unchanged citations of R118's own section above --
this section adds no new evidence, it states B1's disposition against evidence already cited. B1
is **cured**, not an accepted residual and not a cure claimed before its own commits landed (both
commits predate this report; `git cat-file -e 92619cf^{commit}`, `git cat-file -e 48bce3d^{commit}`
and `git cat-file -e 3568bd7^{commit}` all resolve).

### B0 -- the reviewer's Python-package disposable-clone measurement protocol does not apply to this Go repository -- **disposition read from a named snapshot; authorization still missing**

B0's own clause (review's finding text) is: an operator-authorized Go-compatible disposable-
clone/import-identity protocol is needed before independent behavioural verification of
R116-R127 can run; a worker-authored script or task plan is not, by itself, an operator amendment
of the reviewer contract.

**Reporting snapshot** this disposition is read from:

- **Tail code sha**: `3568bd7971a782fadbf589d79ce5777c0f1b5315` (unchanged since task 002; see
  this report's own header).
- **`/run/ralphd/steering` read at `2026-09-17T07:17:13Z`**: the directory exists and is empty
  (`find /run/ralphd/steering -mindepth 1` returns nothing) -- no operator instruction of any kind
  has been delivered to this run through that channel, let alone a B0 authorization.
- **This run's own notify record** (`/run/ralphd/events.jsonl`'s `notification.sent` events): 53
  sends recorded end to end (`2026-09-16T07:34:27Z` through `2026-09-17T07:14:53Z`), every one
  `httpStatus: 200`, each carrying only a `textSha256` of its own outbound text -- the channel is
  a Telegram push notification, one-way engine-to-operator with **no reply channel** (the
  `telegram-notify` skill's own file: "It is send-only -- there is no reply channel, so never wait
  for an answer"). B0's own authorization was named to the operator over exactly this channel in
  an early iteration of this approach (per the standing rules' notify list: "B0 and the exact
  authorization it needs"), but the channel structurally cannot carry a reply back into this run
  -- an authorization, if the operator granted one out of band, would have to arrive via
  `/run/ralphd/steering`, which the same snapshot above shows empty.
- **The Go analogue offered in place of the reviewer's own protocol**: `ci/review.sh` (commit
  `2786d3c`, approach 2) and `docs/reports/phase4-review-protocol.md`, which documents that
  script's disposable git-ignored clone plus its own identity assertion (`go list -m` resolves to
  `github.com/n-orlov/deck`; `go list -f '{{.Dir}}' ./internal/agent` resolves under the clone,
  not the original checkout) as the direct Go equivalent of the reviewer's Python
  `pip install -e .` / `ralphd.__file__`-under-clone check.

**Disposition at this snapshot: the authorization is still missing.** Neither
`/run/ralphd/steering` nor any record this run can read carries an operator statement accepting
`ci/review.sh`'s identity assertion as satisfying B0's clause in place of the Python-package
protocol -- review's own finding is explicit that a worker-authored script or an approved task
plan does not itself constitute that amendment, and nothing at this snapshot changes that.
**Exactly what is needed**: an explicit operator authorization, delivered through a channel this
run can read back (i.e. `/run/ralphd/steering`, not a one-way notify send), stating that
`ci/review.sh`'s disposable-clone module-identity check (`docs/reports/phase4-review-protocol.md`)
is accepted as the Go-compatible substitute for the reviewer's own Python disposable-clone/
import-identity protocol for this repository. Until that arrives, B0 remains blocking and is not
curable by any task in this Go repository's own code or tests, per the standing rules (no Python
packaging is ever added to satisfy it).

## Both sweeps

### Full-suite gate sweep + build/vet/gofmt guards (task 003)

- **Command**: `ci/run.sh go test -p=1 -count=1 -timeout=40m ./...` — every package, no
  `-run` filter, no package list. Build/vet/gofmt guards: `ci/run.sh sh -c 'go build ./...'`,
  `ci/run.sh go vet ./...`, `ci/run.sh gofmt -l .`.
- **Tail code sha**: `3568bd7971a782fadbf589d79ce5777c0f1b5315`.
- **Duration**: ≈6m52s (~412s), `2026-09-17T05:31:29Z` → `2026-09-17T05:38:21Z` (consistent
  with the ~441s/7m21s measured for the same command earlier in this approach, warm cache;
  ordinary variance, not a different command or a narrowed sweep).
- **Result**: PASS. All packages with tests report `ok` (`cmd/deck`, `cmd/fake-claude`,
  `cmd/fake-codex`, `cmd/fake-pi`, `features`, `internal/agent`, `internal/audit`,
  `internal/config`, `internal/hookrecv`, `internal/interactive`, `internal/service`,
  `internal/store`, `internal/theme`, `internal/tmux`, `internal/tui`); `internal/notify`,
  `internal/search` and `internal/unit` report `[no test files]`; no `FAIL` line anywhere.
  Build and vet guards both exit 0 with empty output. `gofmt -l .` exits 0 and lists exactly
  the four **pre-existing** drift paths (no `.review-clone/` present at recording time, so
  the fifth pre-existing path does not appear):
  - `internal/theme/quantize_test.go`
  - `.spike-preview/cmd/conformance/main.go`
  - `.spike-preview/conformance/conformance.go`
  - `.spike-preview/conformance/conformance_test.go`

  None of these paths were touched by this approach's own commits (tasks 001/002 touched only
  `internal/tui/*.go`).
- **Skips in force**: godog's default `~@real-agents && ~@nightly` tag filter, and the
  real-binary skips that leave the `cmd/fake-claude`/`cmd/fake-codex`/`cmd/fake-pi` stubs as
  the tested surface for agent adapters (no real Claude/Codex/pi CLI reachable from this
  sandbox).
- **Evidence**: `docs/reports/phase4-a3-final-suite/{README.md,full-suite.log,build.log,
  vet.log,gofmt.log}`.

### Ten-run stability sweep (task 004)

- **Command**: `ci/stability.sh 10` (ten independent repetitions of `ci/run.sh go test -p=1
  -count=1 ./...`, `-count=1` disables the test cache, each run its own `--rm` sibling).
- **Tail code sha**: `3568bd7971a782fadbf589d79ce5777c0f1b5315` (unchanged from the gate above
  — `git diff --stat 3568bd7 HEAD -- '*.go' '*.feature'` empty at recording time).
- **Duration**: ≈1h09m end to end (script launched ~05:48:16Z; `run-1.log` created
  `2026-09-17T05:55:31Z`; `run-10.log`/summary completed `2026-09-17T06:57:29Z`) — under the
  ~1h12m plan-time estimate; **no features-only fallback was needed or taken**.
- **Result**: **10/10 PASS.** No run's `go test` exit status was non-zero and no `FAIL` line
  appears in any of the ten per-run logs. Both known-open advisory flakes named in the standing
  rules (`TestSigwinchCountDistinguishesTwoFromThree`; `internal/tmux`'s
  `TestSendKeysInvalidHexByteIsSilentlyDiscarded` empty-capture case) were checked for by name
  across all ten logs and appeared in none of them, so no row in the summary table carries the
  advisory label this sweep (the criterion only requires labelling rows where a flake actually
  appears; a manifested flake would read **FAIL** with that test named and the advisory label
  applied, never a bare PASS, since both cases report through `t.Fatalf` and
  `ci/stability.sh` labels a repetition from `go test`'s own exit status).
- **Evidence**: `docs/reports/phase4-a3-stability10/{README.md,run-1.log..run-10.log,
  stability-summary.log}`.

## Summary

Every Tier 1 requirement (R116–R127) is green at this approach's tail code sha `3568bd7`,
cited above against its own commit(s) and test(s); R118 carries two further, same-class B1
cures this approach delivered (`92619cf`/`48bce3d` for the crop-decoration geometry line and
blank-fill rows, `3568bd7` for the interactive-preview branch's notice and pad rows), on top of
approach 2's own already-shipped B1/B2/B3/R1 cures which this report continues to cite
unchanged for R116, R120, R121 and R127. Tier 2 (R128–R131) remains not started, re-affirming
approaches 1 and 2's own budget decision and not narrowing the disclosed, not-scored
SPEC-versus-code grouping gap. Both mandatory sweeps for this approach (the full-suite gate
with the build/vet/gofmt guards, and the ten-run stability sweep) are reported above with
their commands, durations and the shared tail code sha, which is also R132's fourth bullet;
R132's own verdict is stated in its own section above (this report green at `3568bd7`; the
review-findings section, `phase4-findings.md` and `docs/DELIVERY-LOG.md` are the three
immediately following docs-only record tasks, 006–009). Each sweep points at its own committed
directory under `docs/reports/` (`phase4-a3-final-suite/`, `phase4-a3-stability10/`).
