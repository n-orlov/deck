# Phase 4 report — codex adapter, chrome legibility, field backlog

**Approach 2 (cure-and-reverify).** Written once, at this approach's final tail code sha
(per the standing rules, this record is not re-audited by any later task). This report is
record-only. Approach 1's own record (`docs/reports/phase4-final-suite/`,
`phase4-guards/`, `phase4-stability10/`, and the version of this file superseded by this
rewrite, all at `db66965`) stays as history and is not edited.

- **Tail code sha**: `0ba550a5e50bdfc84586d5328a0690af9c9888c4` (`0ba550a`, `features: settle
  the golden frame on a quiet PTY, not a torn read (task 011b)`), named in
  `docs/reports/phase4-cure-final-suite/README.md` (task 013) — the most recent commit in
  this approach's history touching a `*.go` or `*.feature` file. Confirmed unchanged at
  commit time: `git diff --stat 0ba550a HEAD -- '*.go' '*.feature'` prints nothing.
- **Protected-path audit** (the PRD's own command, run against the base commit that added
  `prds/phase4-codex-and-chrome.md`, `08a1ffe3eb229f8ebe5ba9791fbb3e3cec6e0c06`, computed
  fresh, never pasted):
  `git log --oneline 08a1ffe3eb229f8ebe5ba9791fbb3e3cec6e0c06..HEAD -- SPEC.md prds/
  ci/Dockerfile ci/SPIKE.md` prints nothing — no commit in either approach of this run
  touched any protected path.

## Review findings from this approach's plan gate — what was done about each

This approach exists to cure exactly these five findings (`review-findings.json`,
`docs/reports/phase4-review-protocol.md`'s own header). Nothing else in Tier 1 was
"improved" — the standing rules ("APPROACH 2 IS A CURE, NOT A REBUILD") forbid it.

| Finding | What it said | What this approach did | Commit(s) |
| --- | --- | --- | --- |
| **B0** — blocking, not curable in code: required independent execution unavailable (the reviewer's Python disposable-clone-import protocol cannot apply to a Go module) | The reviewer's own measurement protocol assumes an importable Python package; this repository is the Go module `github.com/n-orlov/deck` and has none. | Task 001 supplied `ci/review.sh`, a Go-compatible analogue: it clones the repo disposably, asserts the Go equivalent identity property (module path `github.com/n-orlov/deck`; `go list -f '{{.Dir}}' ./internal/agent` resolves under the clone, not `/workspace`), then runs a caller-supplied `go test` target inside that clone. `docs/reports/phase4-review-protocol.md` records the identity assertion and a narrow smoke run (`./internal/agent/`, not a whole-suite run). No Python packaging was added to the Go product (explicitly forbidden by the standing rules and by review itself). The operator was notified of the protocol and the decision it needs, per the standing rules' notify list. | `2786d3c` (task 001) |
| **B1** — blocking, cured: R118's test oracle explicitly exempted deck-owned preview cells (the empty-preview placeholder, blank fill, crop marker/padding) from the "every deck-owned cell carries the canvas background" claim | `panel_background_themes.feature` carved out columns 37–97 as "not deck-owned for this claim's purposes" even though they hold deck's own placeholder copy; `tui.go:5092–5103`'s placeholder generation and `panel.go`'s preview builders routed every preview text string, regardless of provenance, outside `canvasBackground`. | Task 002 gave every returned preview body line an explicit provenance (`previewLineOwner`: `previewLineDeckOwned` for the no-session sentence, `previewPlaceholderLines`' copy, and blank fill; `previewLineForeign` for a live capture's own bytes) and routed only the deck-owned lines through `canvasBackground` in both layouts. Task 003 did the matching work for a captured row's own fill/crop-marker cells, painting the pad columns past the capture and the crop marker itself while leaving the capture's own cells untouched. Task 004 replaced the feature file's exemption paragraph with the one true SPEC §11.3 exception (an actual pane capture) and asserted the background token over the preview interior for all five built-ins plus `NO_COLOR`/`DECK_COLOR_DEPTH=16`. | `96b0ba9` (task 002), `0e72ec1` (task 003), `4614bff` (task 004) |
| **B2** — blocking, cured: Codex transcript data was wired inside the TUI, violating the PRD's no-`internal/tui`-edit-to-add-a-kind constraint | `eebb2411`'s `TranscriptInput.CodexHome` field and its `tui.go` population/resolution of the literal `CODEX_HOME` key put codex-specific knowledge inside `internal/tui`. | Task 005 replaced `CodexHome` with a generic `agent.Caps.TranscriptEnvKeys []string` an adapter declares and a generic `Env map[string]string` the TUI populates from that declared list — no kind-specific field or branch remains in `internal/tui`; `grep -rn 'CODEX_HOME' internal/tui` prints nothing. Task 006 extended the black-box registry-swap guard to prove a replacement adapter can declare its own invented transcript env key with no edit under `internal/tui`. Task 007 regression-tested transcript lookup through the production caller (`transcriptPathFor`, the `dd`-purge-choice path) with three competing `CODEX_HOME` values (ambient process env, config `[env]`, session env) live at once, proving the session's own value wins and the ambient value's tree is never consulted. | `e69c3d8` (task 005), `ef9571d` (task 006), `1e97059` (task 007) |
| **B3** — blocking, cured: R127's `@codex` scenario did not test the demanded two-second timing bound or own-hook attribution; a swapped-stored-ids negative control would still pass | `codex_hooks.feature`'s prompt step polled for and discarded the fake-codex `session-id:` banner; the "distinct ids" and "transcript doesn't mention the other's id" assertions would not detect two rows' stored ids being swapped with each other. | Task 008 captured each pane's own authoritative SessionStart identity (`session_id` and `transcript_path`) independently of the store, and added a step asserting a named row's persisted conversation id equals its own pane's announced `session_id`, and that PRODUCTION's own `TranscriptPaths` seam (not the test-side glob) resolves to that pane's own announced `transcript_path`. A new Go test, `TestCodexIdentityMismatchCatchesSwappedStoredIDs` (`features/codex_hooks_swap_test.go`), feeds the same comparison the two rows' stored ids swapped and asserts it fails on both halves (id and transcript path), proving the oracle is attribution-sensitive, not merely distinctness-sensitive. Task 009 added the two-second creation-bound and same-cwd assertions, measured from the store's own `created_at`/`cwd` columns, not scenario prose. | `2ff6024` (task 008), `98ac4e7` (task 009) |
| **R1** — residual, cured: the R116 rationale was factually wrong, and a real SPEC-vs-code discrepancy (the sidebar `[safe]` badge) was undisclosed | `internal/agent/claude.go`'s comments said `manual` "is not a value the real CLI accepts", when SPEC §5/R116 describe a version-dependent rename (`default` → `manual` between Claude Code 2.1.71 and 2.1.259); `docs/reports/phase4-report.md` repeated the wrong rationale. Separately, SPEC §11 restricts the permission badge to non-`safe` profiles while R120's parenthetical implied every profile gets one, and `phase4-findings.md` never recorded that disagreement. | Task 010 corrected both `claudeProfileFlags`'s and `claudePermissionArgs`'s comments to state the version rename plainly (no argv behaviour change; existing claude argv tests untouched). Task 011 went further than the documentation-only cure R1 asked for and fixed the underlying behavioural gap directly: `profileBadgeSegment`/its caller now render no badge at all for `safe`, citing SPEC.md:1339, while `plan`/`edits`/`yolo` keep theirs — closing the SPEC-vs-code discrepancy in code rather than leaving it as a disclosed residual. This report carries the corrected rationale below (R116) in place of the old, incorrect one. | `cf2d53b` (task 010), `e93a790` (task 011) |

A golden-fixture regression surfaced while re-sweeping after task 011 (task 011b, below);
it is not itself a review finding and is reported under "Both gates".

## Tier 1 requirements — R116 through R127

Each verdict below cites this approach's own commits and tests for anything this approach
touched (B1/B2/B3/R1's five requirements: R116, R118, R121, R127, plus R120's badge fix
under task 011); every other requirement is unchanged from approach 1's already-`validated`
work at commits inside `08a1ffe3..db66965` (this approach's own tail sha `0ba550a` sits on
top of that unchanged tree — `git diff --stat db66965 0ba550a -- '*.go' '*.feature'` touches
only the files named below) and keeps its original citation.

### R116 — the claude `safe` profile composes no `--permission-mode` flag

**Shipped**, argv unchanged from approach 1; rationale corrected this approach (answers R1's
first half). `internal/agent/claude.go`'s `claudePermissionArgs` returns nil argv for the
`safe` profile (commit `d1d075dc`, approach 1). Its comments — and `claudeProfileFlags`'s —
now state the *true* reason: Claude Code renamed the mode's own spelling from `default` to
`manual` somewhere between CLI versions 2.1.71 and 2.1.259, and omitting the flag for `safe`
is the version-independent fix, never a claim that `manual` "is not a value the real CLI
accepts" (the old, incorrect rationale this report is replacing). Commit `cf2d53b` (task
010).

- Test: `TestClaude_LaunchAndResumeArgv` (`internal/agent/claude_test.go`) — table asserts no
  `--permission-mode` flag at all for `safe`; unchanged by task 010 (no argv behaviour
  change), confirmed still green by the full-suite gate below.
- Feature: `features/permission_modes.feature`'s `csafe` assertions assert absence of
  `--permission-mode`, unchanged from approach 1.

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

**Shipped**, cured this approach (answers B1). `Model.canvasBackground`
(`internal/tui/panel.go`) remains the single composition helper (approach 1, commits
`7695d704`/`448cecc4`), but B1 found its coverage of the preview column narrowed to exclude
deck's own placeholder/fill/crop cells behind a foreign-content exception meant only for an
actual capture. This approach closed that gap:

- Task 002 (`96b0ba9`) gave `previewBodyLines` a second return value, `[]previewLineOwner`,
  tagging every line `previewLineDeckOwned` (the no-session sentence, `previewPlaceholderLines`'
  own copy, blank fill) or `previewLineForeign` (a live capture's own bytes), and routed only
  the deck-owned lines through `canvasBackground` in both `previewContentLine` (side-by-side)
  and `fullBoxPreviewContentLine` (stacked).
- Task 003 (`0e72ec1`) did the matching work for a captured row's own line: the fill columns
  past the capture's own visible bytes (`padTrunc`) and any crop marker now carry
  `theme.Background`, separated from the capture by an explicit reset, while every cell of the
  capture itself keeps the pane's own attributes.
- Task 004 (`4614bff`) replaced `panel_background_themes.feature`'s exemption paragraph
  (which had declared the preview interior "not deck-owned for this claim's purposes") with
  the one true SPEC §11.3 exception — an actual pane capture — and added the
  `cellsRangeHaveBackgroundToken` assertion over the preview's inner columns for all five
  built-ins plus `NO_COLOR`/`DECK_COLOR_DEPTH=16`.

- Tests: `TestNoLiveCapturePreviewInteriorCarriesDeckBackgroundSideBySide`,
  `TestNoLiveCapturePreviewInteriorCarriesDeckBackgroundStacked` (task 002,
  `internal/tui/preview_pane_repaint_test.go`); `TestCapturedPaneFillPastCaptureCarriesDeckBackground`,
  `TestCapturedPaneCropMarkerCarriesDeckBackground` (task 003, same file); the pre-existing
  `TestCapturedPaneSideBySideKeepsOwnColourFrameCarriesDeckBackground` and stacked counterpart
  (approach 1) remain green and continue to prove a capture's own cells are never repainted;
  `TestProfileSwitchTokensMatchSpec`, `TestPinTokensMatchSpec`, `TestRestartChoiceTokensMatchSpec`
  (`internal/tui/profile_pin_restart_theme_test.go`, approach 1, unchanged) prove the dialog
  frame builders.
- Feature: `features/panel_background_themes.feature` — 7 scenarios (7 passed), 82 steps (82
  passed) at this approach's tail sha; log at
  `docs/reports/phase4-cure-logs/panel_background_themes.log` (task 004's targeted run).

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

**Shipped**, base row composition from approach 1; the badge's `safe`-suppression rule added
this approach (folded into R1's cure). `sidebarRowLines` (`internal/tui/tui.go`) composes the
badge segment after `relativeTime` with no `created ` label (approach 1, commit `4c80e5cb`).
R1 found SPEC §11 restricts that badge to non-`safe` profiles while R120's own parenthetical
implied every profile gets one, and `phase4-findings.md` had never recorded the disagreement.
Task 011 (`e93a790`) resolved it in code, citing SPEC.md:1339 at the paint site
(`profileBadgeSegment`, `internal/tui/tui.go`, and its caller): a `safe` row now renders no
badge at all on the sidebar row, while `plan`/`edits`/`yolo` keep theirs as line 2's last
segment. Surfaces other than the sidebar row (the `i` detail dialog and other `profileBadge`
callers) keep their prior behaviour, unchanged.

- Tests: `TestSidebarCreatedLineRendersDimmed` (`internal/tui/sidebar_hierarchy_test.go`,
  approach 1); `TestSidebarRowHidesSafeBadgeButKeepsNonSafeBadgeAtMinimumWidth`
  (`internal/tui/profile_badge_safe_hidden_test.go`, task 011) asserts both the `safe`
  no-badge case and the `plan`/`edits`/`yolo` badge-last case at the sidebar's minimum
  content width.
- Task 011 forced a golden-fixture regression (`features/testdata/golden/side_by_side_80x24.golden`
  still expected the pre-fix `[safe]` badge) that task 011b fixed — see "Both gates" below.

### R121 — internal/agent/codex.go argv/caps; no conversation id minted/passed/stored; TranscriptPaths over an agent-neutral env seam; forbidden-flag guard

**Shipped**, argv/caps/create-path/forbidden-flag sub-parts unchanged from approach 1; the
transcript sub-part's *mechanism* replaced this approach (answers B2).

- Argv and declared capabilities, create-path identity, forbidden-flag guard: unchanged from
  approach 1 (commits `0dd7d625`, `0288b4c5`, `8a7cdb0a`). Tests: `TestCodex_Capabilities`,
  `TestCodex_LaunchArgv`, `TestCodex_ResumeArgv`, `TestCodex_UnknownProfile`,
  `TestCodex_LaunchSucceedsWithEmptyConversationID`, `TestCodex_ResumeRefusesEmptyConversationID`
  (`internal/agent/codex_test.go`); `TestCreateAgentConsultsAssignsConversationIDPerAdapter`
  (`internal/service/agent_test.go`); `TestCodex_NeverEmitsForbiddenFlags`
  (`internal/agent/codex_forbidden_flags_test.go`).
- Transcript resolution over `CODEX_HOME`: B2 found the prior mechanism (`eebb2411`, approach
  1) wired the literal `CODEX_HOME` key and its resolution inside `internal/tui`, violating
  the PRD's no-`internal/tui`-edit-to-add-a-kind constraint. Task 005 (`e69c3d8`) replaced
  `TranscriptInput.CodexHome` with `agent.Caps.TranscriptEnvKeys []string` (an adapter's own
  declaration) and a generic `Env map[string]string`; codex is the only adapter that declares
  a key (`CODEX_HOME`) and its `TranscriptPaths` still reads only from that map.
  `grep -rn 'CODEX_HOME' internal/tui` prints nothing. Task 006 (`ef9571d`) extended the
  black-box registry-swap guard to this capability. Task 007 (`1e97059`) regression-tested the
  production caller with three competing home values live at once.
  Tests: `TestCodexTranscriptPathFindsRealFileUnderDefaultHome`,
  `TestCodexTranscriptPathHonoursSessionCodexHomeOverride`, `TestCodexTranscriptPathMissDegrades`
  (`internal/agent/transcript_test.go`, approach 1, unaffected by the seam change);
  `TestBlackBoxRegistrySwapTranscriptEnvKeyNeedsNoTUIEdit`
  (`internal/tui/registry_guard_test.go`, task 006); `TestTranscriptPathForCodexPrefersSessionEnvOverConfigAndAmbient`
  (`internal/tui`, task 007).

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

**Shipped**, unchanged from approach 1 — not in this approach's scope. Commits `16bcf63e`
(task 020), `2b31afdc` (task 021), `8471807c` (task 022).

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

**Shipped**, ordinary-places and real-agent sub-parts unchanged from approach 1; the `@codex`
scenario's oracle strengthened this approach (answers B3).

- `features/codex_hooks.feature`'s one `@codex`-tagged scenario, "two Codex rows in one
  directory adopt their own ids and report a real approval" (approach 1, commit `78220185`),
  gained the attribution and timing oracles B3 found missing:
  - Task 008 (`2ff6024`) captures each pane's own authoritative SessionStart identity
    (`session_id` and `transcript_path`), independent of anything the store holds, and asserts
    a named row's persisted conversation id equals its own pane's `session_id`, and that
    production's own `TranscriptPaths` seam resolves to that pane's own `transcript_path`
    (whose first-line `session_meta` carries the same `session_id`/`cwd`) — not a test-side
    glob keyed on the stored id. A negative-control Go test,
    `TestCodexIdentityMismatchCatchesSwappedStoredIDs` (`features/codex_hooks_swap_test.go`),
    feeds the same comparison the two rows' ids swapped and fails on both the id and the
    transcript-path half, proving the oracle is attribution-sensitive rather than merely
    distinctness-sensitive.
  - Task 009 (`98ac4e7`) asserts both rows' persisted `cwd` is the same directory and that
    their two creations happened within two seconds of each other, read from the store's own
    `created_at`/`cwd` columns rather than scenario prose.
  - Log (both tasks, superseding approach 1's 27-step log): `docs/reports/phase4-cure-logs/codex_hooks.log`
    — 1 scenario (1 passed), 31 steps (31 passed).
- Codex in the ordinary feature places (approach 1, task 025, commits `ed40e426`/`a11cc860`):
  `features/agent_availability.feature`, `features/permission_modes.feature`,
  `features/agent_session.feature`, `features/kill_delete_undo.feature` — unchanged, not in
  this approach's scope.
- One `@real-agents` codex conformance scenario (approach 1, task 026, commit `50e07029`):
  `features/real_agent_smoke.feature`'s `@codex-real-agent-conformance` scenario — unchanged,
  not in this approach's scope.

## Tier 2 — status stated plainly: NOT STARTED

**Tier 2 (R128–R131) was not started in this approach either.** Task 012's decision record
(`docs/reports/phase4-tier2-decision.md`'s "Approach 2 re-affirmation" section, commit
`425c9dc`) re-affirmed approach 1's own not-started decision without narrowing the gap:
approach 2's scope was bounded to review's B0–B3 and R1 (ten small, single-purpose tasks,
002–011, each a fix or regression test over already-shipped Tier 1 code, not a new feature
surface); Tier 2's four requirements remain the same materially larger, higher-risk body of
work (an all-or-nothing store-schema migration, a four-call-site sidebar grouping-model
replacement with flat-mode removal, a create-modal field, an `i`-dialog move path, and a full
settings groups CRUD surface) approach 1 already declined to start under a comparable
wall-clock deadline. At the moment task 012 wrote its record (iteration 173): 633 iterations
remaining, ~11h36m of wall-clock remaining against deadline `2026-09-17T11:00:06Z`. Task 012
named task 011 as this approach's last code-touching task at the time it was written; that
was superseded immediately afterward by task 011b's golden-fixture fix (below), which is this
approach's actual, final last-code-touching task — the freeze line began the moment 011b's own
last `*.go`/`*.feature` commit (`0ba550a`) landed, exactly as the standing rules require when a
later cure supersedes an earlier "last code-touching task" designation. Tasks 029–038 were not
part of this approach's plan and landed no code.

**(This report's own criterion requires this stated as one paragraph): the SPEC-versus-code
grouping gap, unchanged from approach 1.** SPEC's group-based session organization (a `groups`
table, `sessions.group_id`, sidebar grouping by group id, a create-modal Group field, and a
settings groups CRUD section) remains unimplemented. The code continues to group sessions by
workspace instead, via `store.go`'s `DefaultWorkspace` field and `internal/tui`'s
workspace-based grouping gated by the `ui.group_by_workspace` config key — the exact mechanism
R129 would have required removing (flat mode, `group_by_workspace`,
`reorderPreservingGrouping`). This is the plan's explicitly disclosed, accepted,
**not-a-finding** consequence of the Tier-1/Tier-2 budget decision, re-affirmed rather than
narrowed by this approach — it is not scored in `docs/reports/phase4-findings.md`, per task
012's decision record and the standing rules' own statement of that same rule.

## Both gates

### Full-suite gate sweep (task 013)

- **Command**: `ci/run.sh sh -c 'go test -p=1 -count=1 -timeout=40m ./...'` — every package,
  no `-run` filter, no package list.
- **Tail code sha**: `0ba550a`.
- **Duration**: 7m21s (441s), `2026-09-17T02:02:30Z` → `2026-09-17T02:09:51Z`.
- **Result**: PASS. All 18 packages (`go list ./...`) report `ok` or `[no test files]`; exit
  `0`; no `FAIL` line.
- **Evidence**: `docs/reports/phase4-cure-final-suite/{README.md,suite.log}`.
- **What this sweep found and fixed en route**: task 013's *first* attempt, run at `e93a790`
  (task 011's own tail sha at the time), found `TestGoldenMinimumFrame` red — a real
  regression: task 011 correctly hid the sidebar's `[safe]` badge per SPEC.md:1339 but never
  regenerated `features/testdata/golden/side_by_side_80x24.golden`, which still expected the
  pre-fix frame (`docs/reports/phase4-cure-final-suite/README.md`'s own history, commit
  `059704a`). Task 011b regenerated that golden fixture via the test's own `UPDATE_GOLDEN=1`
  path (`1f38195`) and re-swept, but that sweep was still intermittently red on the *same* test
  for a second, independent reason: the "settled" baseline was taken with a bare
  `client.Frame(true)` immediately after the last content gate — satisfied the instant the row
  carrying its substring is written, while the renderer could still be mid-repaint — so the
  baseline could itself be a torn frame (reproduced directly at 3 of 12 sub-runs, every
  failure the same shape: `before` missing the bottom border/footer that `after` then has).
  Commit `0ba550a` takes the baseline from `ScreenDriver.WaitForQuiescence` (300ms quiet
  window, longer than deck's 250ms `previewTick`) with a bounded retry; 16 of 16 sub-runs green
  over `-count=8` after the fix, and this table's own sweep (run *from scratch*, never re-run
  under task 013 itself, per the standing rules) is the clean, final result at the corrected
  tail sha `0ba550a`.

### Guard evidence (task 014)

- **Commands** (each via `ci/run.sh`, full scope, no filter): `go build ./...`, `go vet ./...`,
  `gofmt -l .`.
- **Tail code sha**: `0ba550a`.
- **Result**: `go build ./...` and `go vet ./...` both exit `0` with empty output. `gofmt -l .`
  exits `0` and lists exactly the four **pre-existing** drift files (measured at `08a1ffe`, per
  the standing rules — none of this approach's own `.go` files are in this list):
  - `internal/theme/quantize_test.go`
  - `.spike-preview/cmd/conformance/main.go`
  - `.spike-preview/conformance/conformance.go`
  - `.spike-preview/conformance/conformance_test.go`

  No other path appears in the `gofmt -l .` output.
- **Evidence**: `docs/reports/phase4-cure-guards/README.md`.

### Ten-run stability sweep (task 015)

- **Command**: `ci/stability.sh 10` (ten independent runs of `ci/run.sh go test -p=1 -count=1
  ./...`, `-count=1` disables the test cache, each run its own `--rm` sibling).
- **Tail code sha**: `0ba550a` (unchanged from the final gate and the guards).
- **Duration**: 1h12m24s total (`2026-09-17T02:11:37Z` → `2026-09-17T03:24:01Z`), each run
  ~6m–7m.
- **Result**: **10/10 PASS.** `grep -h 'FAIL' run-*.log` over all ten committed logs returns
  nothing; each log shows the same 15 `ok` package lines plus the three `[no test files]`
  packages, 18 packages per run. Neither of the two known-open flake classes recurred in this
  sweep: the golden-frame settle race (fixed at the root in `0ba550a`, as above) did not occur
  in any of the ten runs, and `TestSigwinchCountDistinguishesTwoFromThree` did not occur
  either. (An earlier, superseded sweep of this directory at `e93a790`, task 011b's own first
  attempt, recorded 7/10 — two runs red on the golden-frame settle race this approach's own
  `0ba550a` fix resolved, and one run red on a single, non-reproduced
  `TestSendKeysInvalidHexByteIsSilentlyDiscarded` `capture-pane` miss
  (`internal/tmux/literal_send_test.go:123`) that stays disclosed via the `FINDING:` line in
  commit `b98ce9c`'s body and did not recur in any of these ten clean runs.)
- **Evidence**: `docs/reports/phase4-cure-stability10/{README.md,run-1..10.log,summary.log}`.

## Summary

Every Tier 1 requirement (R116–R127) is green at this approach's tail code sha `0ba550a`,
cited above against its own commit(s) and test(s); five of them (R116, R118, R120's badge
rule, R121's transcript seam, R127's `@codex` oracle) carry this approach's own cures for
review's B1, B2, B3 and R1, and this report's R116 section states the corrected
`default`→`manual` version-rename rationale in place of the earlier, incorrect one. B0 was
answered with a Go-compatible measurement protocol (task 001), not a Python packaging
workaround. Tier 2 (R128–R131) was not started, re-affirming approach 1's own budget decision
and not narrowing the disclosed, not-scored SPEC-versus-code grouping gap. All three mandatory
sweeps (the full-suite gate, the build/vet/gofmt guards, and the ten-run stability sweep) are
reported above with their commands, durations and the shared tail code sha `0ba550a`, each
pointing at its own committed directory under `docs/reports/` (`phase4-cure-final-suite/`,
`phase4-cure-guards/`, `phase4-cure-stability10/`).
