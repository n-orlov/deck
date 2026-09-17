# Phase 4 report — codex adapter, chrome legibility, field backlog

> **Run status: NO ACCEPTED VERDICT — read this before relying on anything below.**
> The ralphd run `deck-phase4` was **aborted by the operator** (`ralphctl stop --force`) at
> `2026-09-17T18:04:10Z`; its own record ends `state: aborted`, `reason: "aborted by operator"`,
> `verdict: **unverified**`. **Review never accepted any approach of this run.** Approaches 1
> and 2 were rejected; approach 3's last review pass (pass 256, `2026-09-17T12:16:15Z` →
> `13:33:49Z`) raised two blocking findings — `B1-NC` against R118 and `B2-ENV` against R121 —
> which were cured afterwards (`594b0b4`/`bfdb69e` and `a260abf`) but **never re-reviewed**,
> because the run was aborted before another review pass ran. Nothing in this report is a
> sign-off.
>
> What this report *is*: measurement. Every per-requirement verdict, both mandatory sweeps and
> the whole cure history below are accurate at the tail code sha they cite, and a future phase's
> planning or review hat can rely on them **as evidence, not as an accepted verdict**.
>
> **R132 was not completed.** The documentation tasks whose entire job was to re-cite this
> record at the final tail code sha `a260abf` (tasks 003-008) were reset to `pending` when the
> cure pass moved the tail, and never re-ran before the abort. Some of their work was picked up
> by cure-03-01-2's own docs commit `32b3639` (both sweeps, and this report's sha citations);
> the citations that were still stale after that — and the framing that read as though the work
> had been signed off — were corrected by an out-of-band audit after the run ended. Bullets
> below that say "not re-recorded by the run" mean exactly that.

**Approach 3 (narrow cure of approach 2's rejection).** Written at this approach's final tail
code sha, then audited and corrected out-of-band after the run ended (see the run-status block
above; the standing rules' "written once, never re-audited by a later task" convention governed
the run, and stopped governing when the run did).
This report is record-only. Approach 1's own record (`docs/reports/phase4-final-suite/`,
`phase4-guards/`, `phase4-stability10/`) and approach 2's own record
(`docs/reports/phase4-cure-final-suite/`, `phase4-cure-guards/`, `phase4-cure-stability10/`)
stay as history and are not edited. So do the two prior versions of this file superseded by this
rewrite: approach 1's, written against tail sha `db66965` and committed as `55376d2`, and
approach 2's, written against tail sha `0ba550a` and committed as `bb42aea`/`1ee3cbd`. (Read them
through the *commits* — `git show 55376d2:docs/reports/phase4-report.md` — not through the tail
shas they cite; this file did not yet exist at `db66965`.)

- **Tail code sha**: `a260abfaa36fe96068fb19f4735d66b3b040459b` (`a260abf`, `tui: close the
  foreign-content-to-deck boundary reset independently of deck's own colour (task
  cure-03-01-2)`), named in `docs/reports/phase4-a3-final-suite/README.md` and
  `docs/reports/phase4-a3-stability10/README.md` (both re-recorded at this sha) — the most
  recent commit in this run's history touching a `*.go` or `*.feature` file. It supersedes
  three earlier recordings, each of which measured a tree this run then changed: `3568bd7`
  (task 002's own tail), `7bb1f8a` (the tail after review pass 234's first two cures) and
  `bfdb69e` (the tail after R121's cure). **This approach's own code-touching commits** are
  exactly the eight that `git log 990acc1..HEAD -- '*.go' '*.feature'` returns (`990acc1` is this
  approach's start head): `92619cf` and `48bce3d` (task 001, crop geometry line and blank fill,
  plus its fixture correction), `3568bd7` (task 002, interactive-preview notice and pad rows),
  `2a04e5a` (cure-03-01, settings-footer paint), `7bb1f8a` (cure-03-02, real-Codex first-hook
  wait), `594b0b4` (cure-03-02-2, R121's server-env transcript layer), `bfdb69e` (cure-03-02-2's
  own override-control strengthening) and `a260abf` (cure-03-01-2, R118's foreign-content-to-deck
  boundary reset). Between them they touch only `internal/tui/*.go`, `internal/tmux/geometry.go`
  and `features/*.go`/`*.feature`. Confirmed unchanged at HEAD:
  `git diff --stat a260abfaa36fe96068fb19f4735d66b3b040459b HEAD -- '*.go' '*.feature'` prints
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
- Feature scenario: `features/preview.feature:208`, *"r resumes the selected stopped row and
  passive preview re-fits away from tmux's unfit 80x24 default"* (exact title verified at HEAD;
  earlier recordings of this report paraphrased it as "r resume re-fits away from tmux's 80x24
  default", which is not a scenario name in the tree).

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
- **The settings takeover's own footer row, in every settings mode** (`internal/tui/
  settings.go`), review pass 234's own B1 finding: `settingsFooterLine` returned its composed
  text directly, never routed through `canvasBackground` the way `mainView`'s own `footerLine`
  (task 004/R118, approach 2) already was, so the settings takeover's footer row leaked the
  terminal's own background in every mode -- normal, discard-confirm, search, `[env]` list,
  `[env]` edit and the free-text string editor -- instead of carrying deck's own background
  token. Task cure-03-01 (commit `2a04e5a` -- "tui: paint the settings takeover footer through
  the shared canvas helper (task cure-03-01)") split `settingsFooterLine` into
  `settingsFooterLineContent` (the existing text composition, unchanged) and a thin wrapper
  painting it through `canvasBackground(theme.Background, ...)`, mirroring `footerLine`/
  `footerLineContent`'s own split, fixed once in the one function every settings view already
  calls through.
  Tests: `TestSettingsFooterCarriesDeckBackground` (six modes across all five built-in themes,
  real cells off `Model.View()` via a `vt.Emulator`) and
  `TestSettingsFooterUnderNoColorCarriesNoEscapes` (`internal/tui/
  settings_footer_background_test.go`).
  Feature: `features/settings_footer_background.feature`'s three real-binary scenarios --
  "the settings takeover's plain footer paints deck background across its whole row", "the
  settings takeover's discard-confirm footer paints deck background across its whole row" and
  "the settings takeover's search footer paints deck background across its whole row" -- each
  closing the takeover with esc before exiting (`q` is not bound while `m.settingsOpen`).

- **The foreign-content-to-deck boundary reset, gated on deck's own colour only (task
  cure-03-01-2)**: `canvasResetIfPainting` (`internal/tui/panel.go`) -- the explicit
  `"\x1b[0m"` `previewContentLine`/`fullBoxPreviewContentLine`/`paintForeignFill` emit right
  after a captured pane's own SGR bytes so the pane's colour cannot bleed into deck's own
  border/pad/crop-marker/chrome past it -- fired only when DECK's own colour painting was
  enabled. Under `NO_COLOR` it never fired at all, so a captured row that itself left SGR open
  (a real full-width coloured row with no closing reset of its own) leaked its own
  foreground/background straight into deck's chrome past the boundary. Fixed by commit
  `a260abf` ("tui: close the foreign-content-to-deck boundary reset independently of deck's
  own colour (task cure-03-01-2)"): the reset now fires when EITHER deck's own colour is
  enabled OR the foreign text itself carries any escape byte -- a plain, escape-free foreign
  row under a colour-disabled build still gets no reset (nothing was ever opened, preserving
  every existing plain-NO_COLOR-text/geometry assertion), while a row carrying an escape byte
  gets the defensive close regardless of deck's own colour setting. The captured bytes
  themselves are never scanned, repainted or stripped.
  Tests: `TestForeignBoundaryResetClosesCapturedSGRUnderNoColor`
  (`internal/tui/preview_foreign_boundary_reset_test.go`, both production preview
  content-line builders directly, side-by-side and stacked) and
  `TestForeignBoundaryResetClosesRealTmuxCaptureUnderNoColor`
  (`internal/tui/preview_foreign_boundary_reset_live_test.go`, a REAL tmux `CapturePreview`
  through `Model.View()`, both layouts, with a colour-enabled control proving the same
  scenario already worked pre-fix). Both go red on the pre-fix tree and green after; full
  detail, including the red/green logs, in `docs/reports/phase4-r118-foreign-reset/README.md`.

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
- **Log relabel (task 007)**: `docs/reports/phase4-cure-logs/panel_background_themes.log` is
  approach 2 task 004's own earlier targeted run of `features/panel_background_themes.feature`
  alone (7 scenarios, 7 passed; 82 steps, 82 passed) — a point-in-time record from approach 2,
  not a measurement at any tail sha of this run. This approach's own confirmation that the same
  feature file still passes at this approach's own tail sha `a260abf` is the full-suite gate
  cited separately above and under "Both sweeps" below (task 003), never this log.

### R119 — the selection/mark gutter occupies its own columns; four colour states; feature scenario; contrast floor

**Shipped**, unchanged from approach 1 — not in this approach's scope. Commits `903418a8`,
`06b4521d`/`8d4efd7c`, `651ecd1f`, `2af27b60` (approach 1).

- Tests: `TestSidebarGutterPlainRowPaintsNoBar`, `TestSidebarGutterSelectedRowIsAccentWithBackgroundArrow`,
  `TestSidebarGutterMarkedUnselectedRowIsBadgeWithCheck`, `TestSidebarGutterMarkedAndSelectedRowStaysAccent`
  (`internal/tui/sidebar_gutter_color_test.go`); `TestStackedSidebarGutterSelectedRowIsAccentWithBackgroundArrow`,
  `TestStackedSidebarGutterMarkedUnselectedRowIsBadgeWithCheck`, `TestStackedSidebarGutterGlyphsSurviveNoColor`
  (`internal/tui/stacked_gutter_test.go`); `TestGutterBarContrastFloor` (`internal/theme/contrast_test.go`).
- Feature: `features/panel_background_rectangle.feature`'s two gutter-cells scenarios (commit
  `651ecd1f`), tagged `@requirement-119-gutter-background-tokens` (line 212) and
  `@requirement-119-gutter-text-survives-no-color` (line 255).

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

**Shipped**, cured further this approach (the environment-layering half of the seam,
task cure-03-02-2).

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
- **The session's own launch environment, not the observing TUI's ambient environment (task
  cure-03-02-2, commits `594b0b4` and `bfdb69e`)**: the production caller resolved each
  adapter-declared `TranscriptEnvKeys` entry through `resolveEnvKey`, whose lowest (server env)
  layer was `os.LookupEnv` — this process's own ambient environment. That silently treated the
  observing TUI's environment as if it were the already-running tmux server's, so a TUI started
  after the ambient value moved resolved the wrong transcript root for a session whose server
  still carried the old one. `594b0b4` adds `tmux.Client.ServerEnvironment` (`show-environment
  -g`, the server's own global table, set once when that server started) and wires
  `resolveEnvKey`'s server-env branch to it via `m.tmuxClient`, with `transcriptPathFor`
  threading a context through; SPEC §6.1/§6.3 precedence (session > config > captured_path for
  PATH > server env), default-home and not-found behaviour are unchanged, no Codex-specific
  production branch was added, and the adapter still reads no ambient environment of its own.
  Test: `TestTranscriptPathForUsesActualServerEnvironmentNotObserverAmbient`
  (`internal/tui/transcript_server_env_test.go`) — a REAL private tmux server started under
  `CODEX_HOME`=root-A, this test process's own ambient value then moved to root-B (the running
  server cannot follow it), same-conversation-id transcript files under both roots, and fixture
  assertions that both the server's `show-environment -g` table and its pane's
  `/proc/<pid>/environ` really carry root-A. `bfdb69e` then gave each override layer its own
  distinct root (`sessionHome`, `configHome`) so the positive controls actually detect a
  precedence regression, and added `session-override-wins-over-config-override`. Both reds are
  recorded verbatim: `docs/reports/phase4-r121-server-env/prefix-observer-ambient-red.log` (the
  pre-`594b0b4` code with this test present — only the no-override divergence sub-test fails)
  and `.../precedence-inversion-red.log` (precedence inverted in `resolveEnvKey` — all three
  override sub-tests fail while the divergence one passes), against `.../green.log` at
  `bfdb69e`.
- **R2 citation correction (task 007)**: approach 2 task 005's own commit message asserted
  that `grep -rn 'CODEX_HOME' internal/tui` had an empty result. That literal all-files claim no
  longer holds once approach 2's own follow-on tasks 006 and 007 landed
  `internal/tui/registry_guard_test.go` and `internal/tui/transcript_env_layers_test.go`, each
  of which names `CODEX_HOME` by string in comments and fixture values to prove the seam is
  agent-neutral. Task 007 recorded 14 matches across those two files, measured at the then-current
  tail `3568bd7`/`7bb1f8a`. **Re-measured at the current tail `a260abf`, the count is 26 matches
  across THREE test files** — `internal/tui/registry_guard_test.go`,
  `internal/tui/transcript_env_layers_test.go` and `internal/tui/transcript_server_env_test.go`,
  the third added by the R121 cure (`594b0b4`, extended by `bfdb69e`); `grep -rln 'CODEX_HOME'
  internal/tui` confirms exactly that file set. None is in a non-test file and none is in
  `panel.go`, `tui.go`, `interactive.go` or any other production caller. The property that
  actually matters — and the one R2 names — still
  holds: the production transcript-resolution caller resolves only the keys an adapter declares
  via `Caps.TranscriptEnvKeys` and carries no `CODEX_HOME`-shaped field or branch of its own;
  all 26 remaining matches are intentional test comments and fixture values proving that
  seam, not a codex-specific production path. The product-level seam approach 2 task 005
  delivered is unaffected by this citation correction — only the all-files empty-result
  phrasing of the evidence was ever wrong.

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

**Shipped**, cured further this approach (review pass 234's second new red, and the transcript
ownership half of the cure pass).

- **The real-Codex first-hook wait, `waitForRealCodexHook`** (`features/real_agent_hooks_test.go`):
  review pass 234's finding — the helper rejected an initially empty conversation id the
  instant it was called, before its own bounded 20s polling loop ever ran, rejecting the
  documented, normal asynchronous interval between codex's first prompt (SPEC §8.2 / R125)
  and its first SessionStart hook. Task cure-03-02 (commit `7bb1f8a` — "features: wait for
  codex's asynchronous first-hook identity adoption before checking (task cure-03-02)") makes
  the helper wait for authoritative adoption within that same 20s deadline instead, deriving
  the expected `session_id` only once adoption has actually happened; payload/cwd checks and
  the diagnostic timeout are unchanged.
  Test: `TestWaitForRealCodexHookAllowsDelayedFirstHookAdoption`
  (`features/real_agent_hooks_test.go`) — a deterministic regression against the real wait
  helper: an initially empty row, adoption and its SessionStart hook delivered together after
  a 200ms delay, a successful result within the deadline, plus an already-adopted control. The
  Codex-less `real_agent_smoke.feature` scenario still skips cleanly (no real `codex` CLI in
  this sandbox).
- **Transcript ownership (task cure-03-02-2)**: a session's `transcript_path` is its own, and is
  now resolved from the environment that session's *own* tmux server actually handed its pane
  (`594b0b4`, `bfdb69e`; see R121 above for the layering fix and its regression), so two
  same-conversation-id transcript files under two different `CODEX_HOME` roots can no longer be
  confused by a later observing TUI whose ambient value differs from the server's. Combined with
  `TestCodexIdentityMismatchCatchesSwappedStoredIDs` below, identity and transcript ownership are
  both asserted against the pane's own authoritative hook payload rather than the observer's
  environment.
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

## Tier 2 — R128 through R131, each stated individually: NOT STARTED

Review pass 234's residual R3 asked for R128–R131 as four individually labelled verdicts
rather than one grouped Tier 2 disposition. Each is stated on its own below; all four share the
same wall-clock rationale, given once here rather than four times.

The budget numbers in this rationale were written at `2026-09-17T11:51:32Z`, against the
deadline then in force (`2026-09-17T18:01:18Z`, operator-extended from `2026-09-17T11:00:06Z`,
with max approaches temporarily raised to 4). **Both moved afterwards and are recorded here as
history, not as current facts**: the operator set the deadline to `2026-09-17T20:00:00Z` at
`2026-09-17T14:16:46Z`, and max approaches went back to 3 at `2026-09-17T12:06:08Z` (the run
therefore ended on approach 3 of 3). The run was aborted at `2026-09-17T18:04:10Z`, before
either the extended deadline or any Tier 2 work. The sweep durations this rationale cited
(≈7m6s for the gate, ≈1h11m41s for the stability sweep) were the recordings at the then-current
tail; **both sweeps were re-run twice more after that** and the current numbers under "Both
sweeps" below are 9m35s and 1h10m14s at `a260abf`. The conclusion the rationale reached is
unaffected — the remaining wall clock went entirely to the cures and the record, and no Tier 2
code landed in any approach of this run — but do not quote the figures in this paragraph as
current.

Where the remaining wall clock actually went: curing review pass 234's two new blocking reds
(B1's settings-footer residual, task cure-03-01; the premature real-Codex first-hook rejection,
task cure-03-02), writing the record tail (tasks 003-010), then curing review pass 256's two
new blocking reds (`B2-ENV`, R121's environment layering, task cure-03-02-2; `B1-NC`, R118's
foreign-content-to-deck boundary reset, task cure-03-01-2) and re-running both mandatory sweeps
from scratch at each resulting new tail sha — a bounded cure of an already-rejected approach,
not a reopening of Tier 2. No Tier 2 code landed in this approach; this re-affirms, and does not
narrow, the not-started status approaches 1 and 2 already recorded under their own comparable
deadlines.

### R128 — the group model replaces the workspace label (T2)

**NOT STARTED.** Against the wall-clock budget above, `state.db` still carries `sessions.
workspace` and `store.DefaultWorkspace`; no `groups` table, `sessions.group_id`, migration, or
name-uniqueness/reserved-name enforcement was added, and `DECK_SESSION_WORKSPACE`/the
notification payload's `workspace` field were not renamed. This requirement's own migration is
explicitly all-or-nothing with R129–R131 (Tier 2's own PRD text), and is itself a full schema
migration with a fixture-DB test matrix — a materially larger, higher-risk unit of work than
this approach's own bounded scope (review's two new reds plus the record tail) affords inside
the remaining budget.

### R129 — the sidebar renders manual groups (T2)

**NOT STARTED**, and could not start independently of R128 (Tier 2 is all-or-nothing): the
sidebar still groups by workspace via `internal/tui`'s `ui.group_by_workspace`-gated logic;
`reorderPreservingGrouping`, the flat no-header mode and the `group_by_workspace` config
key/env override/schema row/settings row are all still present, none removed. This
requirement's own scope — group-id-keyed navigation across `internal/tui/group.go`, header
member counts including empty groups, collapse persistence in `ui_state`, and rewriting the
flat/grouped navigation-parity tests to prove grouped behaviour alone — is a four-call-site
replacement of already-working code, not an addition, and was not attempted against the same
remaining budget.

### R130 — membership is set where the session is (T2)

**NOT STARTED.** The create modal has no `Group` field (it still has no group concept to
cycle), no last-group-created-into `ui_state` key was added, and the `i` detail dialog has no
move-to-group picker. This requirement depends on R128's schema and R129's group model existing
first; against the wall-clock budget above, with R128/R129 not started, R130 could not start
either.

### R131 — the group list is edited in settings (T2)

**NOT STARTED.** Settings has no groups section — no `n`/`r`/`d` create/rename/delete, no
move-members-to-`default`-or-delete-them branch on a non-empty group's deletion, and no seam
added to the existing `dd` batch-delete path for a settings-originated call. Like R130, this
requirement is unreachable without R128's schema, and represents the largest single remaining
unit of Tier 2's UI surface (a full CRUD section reusing SPEC §11.5's delete lifecycle); it was
not attempted against the same remaining wall-clock budget the three verdicts above cite.

**(One paragraph, as this report's own criterion requires): the SPEC-versus-code grouping gap,
unchanged from approaches 1 and 2.** SPEC's group-based session organization (a `groups` table,
`sessions.group_id`, sidebar grouping by group id, a create-modal Group field, and a settings
groups CRUD section — R128–R131 above) remains unimplemented. The code continues to group
sessions by workspace instead, via `store.go`'s `DefaultWorkspace` field and `internal/tui`'s
workspace-based grouping gated by the `ui.group_by_workspace` config key — the exact mechanism
R129 would have required removing (flat mode, `group_by_workspace`,
`reorderPreservingGrouping`). This is the plan's explicitly disclosed, accepted,
**not-a-finding** consequence of the Tier-1/Tier-2 budget decision, re-affirmed rather than
narrowed by this approach: it is not scored in `docs/reports/phase4-findings.md`, and it is
never "cured" by editing SPEC — SPEC.md is a protected path in this run and stays untouched.

## R132 — the record matches the tree (T1)

**NOT COMPLETED by this run.** R132 is the only requirement whose deliverable is this record
itself, and it is the one Tier 1 requirement this approach did **not** finish: the record tasks
whose job was to re-cite every document at the final tail code sha `a260abf` (tasks 003-008) were
reset to `pending` when the cure pass moved the tail, and the run was aborted before they re-ran.
Its verdict is stated bullet by bullet below, each saying what actually stands at `a260abf` and
what does not:

- **`docs/reports/phase4-report.md` states, per requirement, what shipped, the commits and the
  tests that prove it, and for anything that did not ship what is missing and why.** Substance
  green; provenance mixed. This file was rewritten under task 005 at `7bb1f8a` (`daea2fd`),
  re-cited at `bfdb69e` under task cure-03-02-2 (`7fe0b26`), and re-cited again at the tail
  `a260abf` under task cure-03-01-2 (`32b3639`) — it previously stood at this approach's own
  earlier `3568bd7` recording (`e5058e5`), itself following approach 2's `0ba550a`. Task 005 was
  **never re-run at `a260abf`**; `32b3639` re-cited the sha-bearing sections but left the Tier 2
  budget paragraph and this section's own bullets standing at their pre-cure wording, and a
  post-run audit corrected those (see the run-status block at the top). Every requirement number
  R116–R132 does carry its own verdict above: R116–R127 individually (R118, R121 and R127 each
  carrying this approach's own further cures, cure-03-01, cure-03-02, cure-03-02-2 and — R118
  again — cure-03-01-2), R128–R131 each individually labelled **NOT STARTED** in their own
  subsections (review pass 234's residual R3), with the wall-clock budget behind them and the
  SPEC-versus-code grouping gap in the one disclosed paragraph that section requires, and R132
  here. Every commit sha, `Test*` name, scenario name and file path this file cites was
  re-checked against the tree at HEAD during the post-run audit (`git cat-file -e <sha>^{commit}`
  per sha, `git grep 'func <Test>('` per test name, `git cat-file -e HEAD:<path>` per path); all
  resolve.
- **`docs/reports/phase4-report.md`'s review-findings section records the review dispositions.**
  Partial. "Review findings from this approach's plan gate — review pass 234 (task 006)" below
  records review pass 234's B1 (cured by tasks 001/002/cure-03-01) and B0 (adjudicated and
  withdrawn by operator ruling) — that section landed as `4542e96`. Review pass **256**'s three
  findings were raised *after* that section was written and task 006 never re-ran, so the post-run
  audit added "Review findings from review pass 256 — the last review of this run" below rather
  than leave them unrecorded.
- **`docs/reports/phase4-findings.md` carries every finding this run made and chose not to
  fix, each with a file:line and a reason, including the two known-unverified codex items by
  name.** **Not re-recorded at the tail by the run.** Task 008's last commit (`8d2f8f7`) put that
  file at the then-current tail `7bb1f8a`; task 008 was reset to `pending` by the cure pass and
  never re-ran, so the ledger's Inventory, its crop-preview `panel.go` line numbers and its
  mouse-race section all measured a superseded tree. Corrected out-of-band by the post-run audit,
  which also added the ledger's ninth entry (the mouse-gesture race quoted by `7fe0b26`), a tenth
  (review pass 256's `R-METADATA` residual) and an eleventh (GitHub issue
  [n-orlov/deck#28](https://github.com/n-orlov/deck/issues/28), the interactive preview dropping
  every Alt-modified special key).
- **`docs/DELIVERY-LOG.md` gains this approach's entry in the existing shape.** Added by task 009
  (`6c397ab`) and corrected by task 010 (`f0ac40b`) at the then-current tail `7bb1f8a`; **not
  re-recorded at `a260abf` by the run**. Its current state is outside this report's own audit
  scope — read that file itself, not this bullet, for what it now says.
- **Both gates are reported with their commands, their durations and the sha they ran at — the
  final code sha per Materiality's termination rule.** Green — see "Both sweeps" immediately
  below: the full-suite gate plus build/vet/gofmt guards and the ten-run stability
  sweep, each with its command as run, its duration, the shared tail code sha
  `a260abf`, and its own committed directory under `docs/reports/`. These two *were* re-recorded
  at `a260abf`, by `32b3639`, which is why they are the most trustworthy numbers in this record.
  One caveat, disclosed as review pass 256's `R-METADATA` residual: the *derived* package-duration
  sum that used to be quoted alongside the gate's wall clock was wrong in every recording, and has
  been removed from both this report and `docs/reports/phase4-a3-final-suite/README.md` rather
  than replaced with a third number — the per-package table in that README and `full-suite.log`
  itself are the primary record.

Tasks 003–010's own bullets landed as docs-only commits on top of the earlier tail `7bb1f8a`
(`abd963f`, `c069c34`, `daea2fd`, `4542e96`, `8ae503c`, `8d2f8f7`, `6c397ab`, `f0ac40b`);
the cure pass then landed R121's fix (`594b0b4`, `bfdb69e`), which moved the tail and forced
both sweeps to be re-run from scratch at `bfdb69e`; this cure pass then landed R118's
foreign-content-to-deck boundary reset fix (`a260abf`, task cure-03-01-2), which moved the
tail again and forced both sweeps to be re-run from scratch a second time, at `a260abf` --
the measurements reported below, and the sha every section above now cites. Per the PRD's own
Materiality termination rule (and the standing rules that restate it) a docs-only tail commit
invalidates neither sweep and is itself exempt from re-verification, so the record tail's own
docs-only commits do not reopen the sweeps reported below.

## Review findings from this approach's plan gate — review pass 234 (task 006)

This section states, for each of the plan gate's two blocking findings, what this approach did
about it and why. Neither disposition below is written ahead of the paint or evidence it
describes -- B1's cure commits already carry the fix and its test; B0's disposition is read from
a named, dated snapshot of this run's own record, not asserted from memory.

These are review pass **234**'s findings (`2026-09-17T07:59:39Z` → `09:22:48Z`), the pass that
opened this approach's cure work. A **later** review pass, 256, ran after the record tail below
and raised three more findings; those are recorded in their own section further down, not here.

### B1 -- R118 omitted deck-generated live-capture geometry/vertical-fill and the interactive branch's own notice/pad rows -- **cured this approach**

At this approach's plan gate, B1 was blocking: `cropPreviewBottomLeft`'s own geometry line and
synthesized vertical blank-fill rows (`internal/tui/panel.go`) were deck-generated but
`previewBodyLines`'s live-capture branch marked the *whole* crop slice foreign, so those cells
leaked the terminal's own background instead of `theme.Background`. This section does not record
that gap as an accepted, disclosed residual (it was never optional Tier-2 scope) -- B1 was
blocking review, and a blocking finding is cured, not accepted.
`docs/reports/phase4-findings.md`'s earlier entry that *did* call it an accepted residual was
superseded by task 008 (`9d1e654`, refreshed again at `8d2f8f7`); that ledger's entry 3 now
states the fix and cites both cure commits, kept as a historical entry rather than deleted.

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

- **Task cure-03-01** (commit `2a04e5a` -- "tui: paint the settings takeover footer through the
  shared canvas helper (task cure-03-01)") cured review pass 234's own new B1 finding, raised
  against this same approach's plan gate: `settingsFooterLine` (`internal/tui/settings.go`)
  returned its composed footer text as a bare string with no `theme.Background` token, so the
  settings takeover's own footer row leaked the terminal's background in every settings mode.
  Task cure-03-01 split `settingsFooterLine` into `settingsFooterLineContent` (the existing text
  composition, unchanged) and a thin `canvasBackground`-wrapping caller, painting the footer the
  same way the crop and interactive branches above were already painted. New tests:
  `TestSettingsFooterCarriesDeckBackground` (six modes across all five built-in themes) and
  `TestSettingsFooterUnderNoColorCarriesNoEscapes`
  (`internal/tui/settings_footer_background_test.go`), plus
  `features/settings_footer_background.feature`'s three real-binary scenarios.

All three commits, and all their new tests, are unchanged citations of R118's own section above
-- this section adds no new evidence, it states B1's disposition against evidence already cited.
B1 is **cured**, not an accepted residual and not a cure claimed before its own commits landed
(all three commits predate this report; `git cat-file -e 92619cf^{commit}`,
`git cat-file -e 48bce3d^{commit}`, `git cat-file -e 3568bd7^{commit}` and
`git cat-file -e 2a04e5a^{commit}` all resolve).

### B0 -- the reviewer's Python-package disposable-clone measurement protocol does not apply to this Go repository -- **ADJUDICATED AND WITHDRAWN by operator ruling**

B0's own clause (review's finding text) is: an operator-authorized Go-compatible disposable-
clone/import-identity protocol is needed before independent behavioural verification of
R116-R127 can run; a worker-authored script or task plan is not, by itself, an operator amendment
of the reviewer contract.

A prior snapshot of this section, taken at `2026-09-17T07:17:13Z` and recorded against commit
`b9ffef3`, read the run's own record (an empty `/run/ralphd/steering` and the one-way `notify`
send log) and concluded the authorization was still missing. That snapshot has since been
superseded: the operator recorded a wave-scoped ruling into `/run/ralphd/steering` at
`2026-09-17T07:59:01Z` (delivered as `001-b0-authorized-use-ci-review-sh.md`), which the
steering hat applied at `2026-09-17T09:28:47Z`, and the ruling itself is filed at
`/config/amendments/001-WAVE.md`, timestamped `2026-09-17T07:57:41Z`. That amendment is the
disposition now on record, replacing the `b9ffef3` snapshot's "still missing" reading rather
than standing alongside it.

**Disposition: ADJUDICATED AND WITHDRAWN.** `/config/amendments/001-WAVE.md` states, verbatim:

> I am the operator. I authorize the substitution the reviewer asked for in B0's own remedy ("an
> operator-authorized Go-compatible disposable-clone protocol is needed").

and names the concrete replacement:

> Use `ci/review.sh`, committed at 2786d3c by approach 2's task 001, and documented at
> docs/reports/phase4-review-protocol.md. It is the Go analogue of the prompt's check and
> preserves the property the prompt exists to guarantee

B0 is therefore **withdrawn as a blocking finding by operator decision**, and `ci/review.sh`
(commit `2786d3c`) plus `docs/reports/phase4-review-protocol.md` is the authorized,
Go-compatible replacement for the review prompt's Python disposable-clone bootstrap, for the
rest of this run. This disposition is not curable-in-code and needed none: the gap was never a
product defect, only a missing operator authorization, and that authorization has now been
granted and is on record. No Python packaging, `pyproject.toml`, `setup.py` or `ralphd` module
has been or will be added to this Go repository to manufacture identity evidence -- the ruling
forbids it and so does the standing-rules block governing this run. R116-R127's own
per-requirement verdicts above (this report's earlier sections) are the real measurements taken
under the authorized protocol that this ruling unblocked; none of them reads "NOT VERIFIED" for
want of B0.

## Review findings from review pass 256 — the last review of this run (post-run audit)

This section was **not** written by the run. Review pass 256 (`2026-09-17T12:16:15Z` →
`13:33:49Z`) ran after the record tail above had already landed, so the record task that would
have recorded its dispositions (task 006) had already completed and was never re-run. It is
added here by a post-run audit so a future phase does not have to reconstruct the last review's
findings from the ralphd run record. All three are quoted from that record
(`review-findings.json` for run `deck-phase4`); pass 256 reviewed final code sha `7bb1f8a` via
its documentation descendant `f0ac40b`.

**Pass 256 never re-reviewed the cures below.** It raised the two blocking findings, the worker
cured both, and the operator aborted the run before another review pass could judge the result.
So: cured and self-verified, **not accepted**.

### B1-NC (blocking, curable) — R118: the foreign-content boundary reset was gated on deck's own colour — **cured by `a260abf`, not re-reviewed**

Pass 256's clause: *"R118: captured pane content is never repainted, and deck emits a reset after
it so the pane's colours cannot leak into deck's frame; SPEC section 11.3; requirement-sensitive
tests."* Its evidence, taken in the operator-authorized disposable clone, was that
`canvasResetIfPainting` returned an empty string whenever `Color` was false, so under `NO_COLOR`
a real 80-column tmux row printed with foreground `#112233`/background `#445566` and no closing
reset leaked `#445566` into deck's own right border — at `(119,1)` side-by-side and `(119,13)`
stacked — while both colour-enabled controls passed.

Cured by task cure-03-01-2, commit `a260abf`. The full disposition, the fix's own reasoning and
its two regression tests are in R118's own section above (the "foreign-content-to-deck boundary
reset" bullet); the red/green logs are in `docs/reports/phase4-r118-foreign-reset/README.md`.

### B2-ENV (blocking, curable) — R121: `TranscriptPaths` read the observer's ambient environment, not the session's own server environment — **cured by `594b0b4`/`bfdb69e`, not re-reviewed**

Pass 256's clause: *"R121: TranscriptPaths uses the session's own environment layering (SPEC
section 6.1), with caller-supplied Codex home; R127 transcript ownership."* Its evidence: at
`7bb1f8a`, `internal/tui/env_editor.go` returned `os.LookupEnv(key)` from the *observing* TUI and
labelled it the server layer, so a private real tmux server started under `CODEX_HOME`=root-A —
with both `show-environment -g` and the live pane's `/proc/<pid>/environ` verified to carry
root-A — still resolved root-B's same-id transcript file once only the observing process's
ambient value had moved.

Cured by task cure-03-02-2, commits `594b0b4` (the `tmux.Client.ServerEnvironment` layer) and
`bfdb69e` (distinct roots per override layer). Full disposition in R121's own section above; the
two verbatim reds and the green are in `docs/reports/phase4-r121-server-env/`.

### R-METADATA (residual, curable) — R132: inaccurate derived metadata in the gate records — **partially closed after the run; scope narrowed, not invented**

Pass 256's clause: *"R132: the record matches the tree; accurate gate metadata."* It found two
things, both in *supporting* metadata rather than in any suite result, and classed them residual
under the PRD's explicit curable-in-place class — explicitly **not** an invented green run and
**not** evidence that a required suite was narrowed:

1. A **derived package-duration sum** that did not match the log it claimed to summarise. Pass
   256 measured this against the recording then in place (the `abd963f`/`7bb1f8a` log: audit
   derived `422.637s`, the record said `419.7s`). The same class of error was present in every
   recording of this run's lineage, including the current one at `a260abf`, where the fifteen `ok`
   package durations in `docs/reports/phase4-a3-final-suite/full-suite.log` sum to **`567.046s`**
   and the record said `566.6s`. **Remedy applied after the run:** the derived sum was *removed*
   from both `docs/reports/phase4-a3-final-suite/README.md` and this report's gate section rather
   than replaced with a third number — the per-package table and `full-suite.log` are the primary
   record, and the wall-clock figure (575s) is kept and explicitly labelled as wall clock. The
   stated wall-clock envelope and the green package results were never contradicted by the
   finding.
2. **`docs/reports/phase4-a3-stability10/README.md`'s skips claim** said the only skips were the
   three no-test packages, although `features/godog_test.go` excludes `@real-agents` and
   `@nightly` by default and `features/i1_repro_test.go` is opt-in. This report and the
   full-suite README already disclosed the tag filter correctly. That half of the finding is
   against the stability README, which is outside this report's own edit scope — read that file
   for its current wording.

R-METADATA is recorded in `docs/reports/phase4-findings.md` as an open ledger entry, since no
review pass ever confirmed either half closed.

## Both sweeps

### Full-suite gate sweep + build/vet/gofmt guards (task 003)

- **Command**: `ci/run.sh go test -p=1 -count=1 -timeout=40m ./...` — every package, no
  `-run` filter, no package list. Build/vet/gofmt guards: `ci/run.sh sh -c 'go build ./...'`,
  `ci/run.sh go vet ./...`, `ci/run.sh gofmt -l .`.
- **Tail code sha**: `a260abfaa36fe96068fb19f4735d66b3b040459b` (`a260abf`), confirmed by
  `git log --format=%H -1 -- '*.go' '*.feature'` == `git rev-parse HEAD` at launch.
- **Duration**: **9m35s (575s) wall clock**, `2026-09-17T16:08:19Z` → `2026-09-17T16:17:54Z`
  (timestamps taken by the driver script immediately before and after the command). That is wall
  clock, not a test-time total: it covers the per-package durations `go test` itself reports plus
  sibling-container startup/teardown overhead. **No derived sum of those per-package durations is
  quoted here** — every recording in this run's lineage quoted one and every one of them was
  wrong (review pass 256's `R-METADATA` residual), so the primary record is the per-package table
  in `docs/reports/phase4-a3-final-suite/README.md` and `full-suite.log` itself, both of which a
  reader can re-derive from directly. Longer than the ~7m4s-7m21s measured at the earlier tails
  (the `features` package alone ran 501.5s here vs. 362-419s before) — ordinary
  host-scheduler/sibling-container variance, not a different command or a narrowed sweep.
- **Result**: PASS (`go test` exit 0). All 15 packages with tests report `ok` (`cmd/deck`,
  `cmd/fake-claude`,
  `cmd/fake-codex`, `cmd/fake-pi`, `features`, `internal/agent`, `internal/audit`,
  `internal/config`, `internal/hookrecv`, `internal/interactive`, `internal/service`,
  `internal/store`, `internal/theme`, `internal/tmux`, `internal/tui`); `internal/notify`,
  `internal/search` and `internal/unit` report `[no test files]`; no `FAIL` line anywhere
  (`grep -c '^FAIL' full-suite.log` == 0). Build and vet guards both exit 0 with empty output.
  `gofmt -l .` exits 0 and lists exactly the four **pre-existing** drift paths (no
  `.review-clone/` present at recording time — confirmed by `ls .review-clone` failing — so
  the fifth pre-existing path does not appear):
  - `internal/theme/quantize_test.go`
  - `.spike-preview/cmd/conformance/main.go`
  - `.spike-preview/conformance/conformance.go`
  - `.spike-preview/conformance/conformance_test.go`

  None of these paths were touched by this approach's own commits (tasks 001, 002, cure-03-01,
  cure-03-02, cure-03-02-2 and cure-03-01-2 touched only `internal/tui/*.go`,
  `internal/tmux/geometry.go` and `features/*.go`/`*.feature`, none of these four).
- **Skips in force**: godog's default `~@real-agents && ~@nightly` tag filter, and the
  real-binary skips that leave the `cmd/fake-claude`/`cmd/fake-codex`/`cmd/fake-pi` stubs as
  the tested surface for agent adapters (no real Claude/Codex/pi CLI reachable from this
  sandbox).
- **Evidence**: `docs/reports/phase4-a3-final-suite/{README.md,full-suite.log,build.log,
  vet.log,gofmt.log}` — the source this section's own numbers are taken from, per this task's
  own criteria.

### Ten-run stability sweep (task 004)

- **Command**: `ci/stability.sh 10` (ten independent repetitions of `ci/run.sh go test -p=1
  -count=1 ./...`, `-count=1` disables the test cache, each run its own `--rm` sibling).
- **Tail code sha**: `a260abfaa36fe96068fb19f4735d66b3b040459b` (`a260abf`, unchanged from the
  gate above — same sha, same tree; `HEAD` at launch was `a260abf` itself).
- **Duration**: **1h10m14s** end to end (script launched `2026-09-17T16:17:54Z`; `run-1.log`
  completed `2026-09-17T16:24:54Z`; `run-10.log`/summary completed `2026-09-17T17:28:08Z`) —
  matching the ~1h12m plan-time estimate; **no features-only fallback was needed or taken**. This
  supersedes the ten-run records previously written at the prior tails `3568bd7` (commits
  `5f1fb5c`/`deca67c`), `7bb1f8a` (commit `c069c34`) and `bfdb69e` (9/10, run 5's
  mouse-gesture fixture race): the cure pass then landed `a260abf` (R118's foreign-content-
  to-deck boundary reset fix) on top of `bfdb69e`, so that record measures a superseded tree.
- **Result**: **10/10 PASS — clean.** All ten runs passed with `go test` exit 0
  (`ci/stability.sh` exit 0, `stability-summary.log`: `10/10 passed`). Every one of the ten
  per-run logs lists all 18 packages `go list ./...` returns for this module, so no repetition
  was narrowed, and no run was re-run, discarded or replaced. The prior recording's one
  failing scenario (`TestFeatures` → *a single click on a sidebar row selects it and enters
  interactive mode on the same press, and Ctrl+Q returns to the list*,
  `features/mouse.feature:10`, its after-scenario frame-unchanged hook at
  `features/mouse_synthesis_test.go:254` racing the sidebar's own `starting`→`running`
  reconcile transition) did **not** reproduce in any of these ten runs; it is a probabilistic
  fixture-timing race, independent of this approach's cures (its territory is
  `features/mouse.feature` and `features/mouse_synthesis_test.go`, never touched by any commit
  in this approach), and it remains an OPEN, disclosed finding in
  `docs/reports/phase4-findings.md` — this clean 10/10 does not retract that finding, it only
  reports that this sweep did not observe it. Both known-open advisory flakes named in the
  standing rules
  (`TestSigwinchCountDistinguishesTwoFromThree`, `features/sigwinch_count_test.go:24`;
  `TestSendKeysInvalidHexByteIsSilentlyDiscarded`'s empty-capture case,
  `internal/tmux/literal_send_test.go:123`) were searched for by name across all ten logs and
  appeared in none of them; a manifested advisory flake would read **FAIL** with that test
  named and the advisory label applied, never a bare PASS, since both cases report through
  `t.Fatalf` and `ci/stability.sh` labels a repetition from `go test`'s own exit status,
  captured immediately after the un-piped command, never from log text.
- **Evidence**: `docs/reports/phase4-a3-stability10/{README.md,run-1.log..run-10.log,
  stability-summary.log}` — the source this section's own numbers are taken from, per this
  task's own criteria.

## Summary

**Read the run-status block at the top of this file first.** This run has **no accepted
verdict**: it was aborted by the operator, its record ends `verdict: unverified`, and review
never accepted any approach. Everything below is what was measured, not what was signed off.

Every Tier 1 requirement (R116–R127) is green **as measured by this approach's own tests and
sweeps** at its tail code sha `a260abf` — green in the sense that the requirement-sensitive tests
and the unnarrowed full suite pass, not in the sense that a review pass accepted it,
cited above against its own commit(s) and test(s); R118 now carries FOUR same-class B1 cures
across this run (`92619cf`/`48bce3d` for the crop-decoration geometry line and blank-fill rows,
`3568bd7` for the interactive-preview branch's notice and pad rows, `2a04e5a` for the settings
takeover's footer row, and this cure pass's own `a260abf` for the foreign-content-to-deck
boundary reset that was gated on deck's own colour only — review pass 256's `B1-NC`), R127
carries this approach's own `7bb1f8a` cure of the premature real-Codex first-hook rejection —
review pass 234's second new red — and R121 additionally carries the cure pass's own
`594b0b4`/`bfdb69e` fix (review pass 256's `B2-ENV`), which resolves each session's transcript
environment from that session's own tmux server instead of the observing TUI's ambient
environment. Every other Tier 1
requirement keeps its unchanged prior
citation from approaches 1/2 (R116, R119, R120, R122–R126 unchanged from approach 1 or 2 as
noted per-section above). **Review pass 256's two blocking findings were cured but never
re-reviewed**, and its `R-METADATA` residual stays open — see "Review findings from review pass
256" above. Tier 2 (R128–R131) remains not started, now stated as four
individually labelled verdicts (review pass 234's residual R3) rather than one grouped
disposition, each re-affirming approaches 1 and 2's own budget decision and not narrowing the
disclosed, not-scored SPEC-versus-code grouping gap. (The deadline those verdicts cite,
`2026-09-17T18:01:18Z`, was superseded by the operator's `2026-09-17T20:00:00Z` extension before
the run was aborted at `2026-09-17T18:04:10Z`; see the Tier 2 section for the full timeline.)
Both mandatory sweeps for this approach (the
full-suite gate with the build/vet/gofmt guards, green; and the ten-run stability sweep,
**10/10 clean**, with the earlier recording's mouse-gesture fixture race remaining an OPEN,
disclosed finding that did not reproduce this run) are reported
above with their commands, durations and the shared tail code sha `a260abf`, taken from
`docs/reports/phase4-a3-final-suite/README.md` and `docs/reports/phase4-a3-stability10/
README.md`, which is also R132's fifth bullet; these two are the parts of the record that the run
*did* re-record at the final tail sha (`32b3639`), and are correspondingly the most trustworthy
numbers here. **R132 itself is NOT COMPLETED** — its own section above says why, bullet by
bullet: tasks 003-008 were reset to `pending` when the cure pass moved the tail and never re-ran,
so `docs/reports/phase4-findings.md` and `docs/DELIVERY-LOG.md` were never re-recorded at
`a260abf` by the run, and the stale citations that left behind were corrected by a post-run
audit instead. Each sweep points at its own committed directory
under `docs/reports/` (`phase4-a3-final-suite/`, `phase4-a3-stability10/`).
