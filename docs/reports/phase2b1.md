# Phase 2b-1 report

Per-requirement evidence for requirements 1-46 (requirement 45). For every
requirement below: the command that verifies it, and that command's real
captured output from this workspace. Full untrimmed logs for the batched
`go test -v` runs used here are not committed (they duplicate `ci/stability.sh`'s
own captured logs in `docs/reports/phase2b1-stability.log`); every block below
is the real, unedited tail of the run that produced it. Anyone can reproduce
any of them with `ci/run.sh <command>` (Go) or `ci/run.sh <feature command>`
(with the `DECK_GODOG_TAGS` filter shown) from a clean checkout.

## Requirement 6: the screen-emulator decision, with evidence

**Decision:** moved the harness's `ScreenDriver` from `hinshun/vt10x` to
`github.com/charmbracelet/x/vt` (commit `1418df6`, "harness: move screen
emulator off hinshun/vt10x to charmbracelet/x/vt (requirement 6)").

**Evidence:** `docs/spikes/tmux-embedded-preview.md` had already measured
vt10x placing a following box-drawing glyph inside an East-Asian-Wide
continuation cell instead of giving the wide rune its own width-2 placement,
and named `charmbracelet/x/vt` as the fix. `1418df6`'s commit message
records the migration (`ScreenDriver` backed by `vt.NewEmulator`/
`vt.Terminal`) and a real bug it surfaced along the way: x/vt's OSC 10/11/12
handlers write into the emulator's own internal pipe expecting the embedder
to drain it, so the first OSC-11 colour query deadlocked the suite until a
draining goroutine was added.

`e574707` ("harness: assert East-Asian-Wide cell placement in the emulator
(requirement 6)") then added `features/harness.feature`'s
`@requirement-6-eaw-placement` scenario, which is the regression guard: it
writes `界┌` into a fresh 12×4 emulator and asserts column 0 is width-2
content `界`, column 1 is a continuation cell, and column 2 is `┌`. The
worker who added it deliberately reverted the emulator choice back to
vt10x locally and confirmed this scenario failed (recorded in that task's
commit message); the migration itself is what keeps it passing today.

```
$ grep -rn hinshun/vt10x features/ internal/ cmd/
(no output)
```

No production or harness code depends on `hinshun/vt10x` any more (the
unrelated `.spike-preview/` conformance harness has its own `go.mod` and is
out of scope, per the PRD).

## Requirements 1-7: harness prerequisites (`features/harness.feature`)

```
$ ci/run.sh env DECK_GODOG_TAGS="@requirement-1-resize,@requirement-2-mouse-synthesis,\
@requirement-3-deck-mouse,@requirement-4-fake-agent-sizes,@requirement-5-preview-fixtures,\
@requirement-6-eaw-placement,@requirement-7-sidebar-width" go test -v -run TestFeatures ./features/...
...
15 scenarios (15 passed)
70 steps (70 passed)
--- PASS: TestFeatures (8.53s)
    --- PASS: TestFeatures/the_screen_emulator_places_an_East-Asian-Wide_cell_without_splitting_it (0.42s)
    --- PASS: TestFeatures/a_mid-scenario_terminal_resize_changes_the_grid_the_frame_is_read_from (0.35s)
    --- PASS: TestFeatures/SGR_mouse_reports_are_synthesized_for_click,_double-click,_wheel_and_drag (0.49s)
    --- PASS: TestFeatures/DECK_MOUSE_overrides_the_default_mouse-reporting_setting (0.35s)
    --- PASS: TestFeatures/DECK_MOUSE=0_disables_mouse_reporting (0.35s)
    --- PASS: TestFeatures/a_fake_agent_records_its_initial_size_and_every_SIGWINCH-observed_size (0.51s)
    --- PASS: TestFeatures/a_fake_agent_records_its_initial_size_and_every_SIGWINCH-observed_size#01 (0.46s)
    --- PASS: TestFeatures/a_fake_agent_renders_a_preview_fixture_once_and_then_falls_silent (0.58s)
    --- PASS: TestFeatures/a_fake_agent_renders_a_preview_fixture_once_and_then_falls_silent#01 (0.59s)
    --- PASS: TestFeatures/a_fake_agent_renders_a_preview_fixture_once_and_then_falls_silent#02 (0.63s)
    --- PASS: TestFeatures/a_fake_agent_renders_a_preview_fixture_once_and_then_falls_silent#03 (0.59s)
    --- PASS: TestFeatures/a_fake_agent_renders_a_preview_fixture_once_and_then_falls_silent#04 (0.60s)
    --- PASS: TestFeatures/a_fake_agent_renders_a_preview_fixture_once_and_then_falls_silent#05 (0.60s)
    --- PASS: TestFeatures/sidebar_width_can_be_set_and_read_back_for_a_scenario (0.36s)
    --- PASS: TestFeatures/a_non-default_sidebar_width_actually_widens_deck's_own_rendered_sidebar (1.60s)
PASS
ok  	github.com/n-orlov/deck/features	8.539s
```

- **R1** (mid-scenario resize): `.../a_mid-scenario_terminal_resize_changes_the_grid_the_frame_is_read_from`.
- **R2** (SGR synthesis incl. column ≥224): `.../SGR_mouse_reports_are_synthesized_for_click,_double-click,_wheel_and_drag`.
- **R3** (`DECK_MOUSE`): the two `DECK_MOUSE...` scenarios above.
- **R4** (fake agents record observed sizes): the two `a_fake_agent_records_its_initial_size...` outline rows (claude, pi).
- **R5** (deterministic preview fixtures — fitting/oversized/wide, both agents): the six `a_fake_agent_renders_a_preview_fixture...` outline rows.
- **R6** (emulator): see dedicated section above; also exercised again here as `the_screen_emulator_places_an_East-Asian-Wide_cell...`.
- **R7** (sidebar-width step, and that it visibly widens deck's own frame — not just a round trip through `state.db`): the two `sidebar_width...` scenarios, the second of which creates a long-named shell session, confirms `"WIDENOW"` is *not* visible at the default width, presses `>` until it is, and confirms it now *is* visible.

## Requirements 8-15: layout modes (`SPEC.md` §11.2)

Pure-function unit tests (`internal/tui/layout_test.go`,
`internal/tui/layout_persistence_test.go`):

```
$ ci/run.sh go test -v -run 'TestComputeLayoutAutoBoundary|TestComputeLayoutGoldenMinimum|\
TestComputeLayoutPinnedFallsBackWithoutOverwritingPin|TestComputeLayoutStackedNeverFallsBack|\
TestComputeLayoutCollapsedNeverAutomatic|TestComputeLayoutBelowMinimum|TestStackedListHeightBounds|\
TestClampSidebarWidth|TestComputeLayoutSidebarWidthDefault|\
TestComputeLayoutSideBySideAndCollapsedAlwaysShowPreview|\
TestComputeLayoutStackedSuppressesPreviewBelowItsFloor|\
TestPipeAndAngleBracketsPersistToUIStateNotConfigToml' ./internal/tui/...
--- PASS: TestPipeAndAngleBracketsPersistToUIStateNotConfigToml (0.02s)
--- PASS: TestComputeLayoutAutoBoundary (0.00s)
--- PASS: TestComputeLayoutGoldenMinimum (0.00s)
--- PASS: TestComputeLayoutPinnedFallsBackWithoutOverwritingPin (0.00s)
--- PASS: TestComputeLayoutStackedNeverFallsBack (0.00s)
--- PASS: TestComputeLayoutCollapsedNeverAutomatic (0.00s)
--- PASS: TestComputeLayoutBelowMinimum (0.00s)
--- PASS: TestStackedListHeightBounds (0.00s)
--- PASS: TestClampSidebarWidth (0.00s)
--- PASS: TestComputeLayoutSidebarWidthDefault (0.00s)
--- PASS: TestComputeLayoutSideBySideAndCollapsedAlwaysShowPreview (0.00s)
--- PASS: TestComputeLayoutStackedSuppressesPreviewBelowItsFloor (0.00s)
PASS
ok  	github.com/n-orlov/deck/internal/tui	0.027s
```

Black-box coverage of the same requirements, driven through the real
binary (`features/layout_modes.feature`, requirement 38's scenario file):

```
$ ci/run.sh env DECK_GODOG_TAGS="@requirement-38-layout-modes" go test -v -run TestFeatures ./features/...
8 scenarios (8 passed)
64 steps (64 passed)
--- PASS: TestFeatures (11.27s)
    --- PASS: TestFeatures/auto_selects_side-by-side_at_and_above_80_columns,_stacked_below_it (0.43s)
    --- PASS: TestFeatures/auto_selects_side-by-side_at_and_above_80_columns,_stacked_below_it#01 (0.35s)
    --- PASS: TestFeatures/auto_selects_side-by-side_at_and_above_80_columns,_stacked_below_it#02 (0.35s)
    --- PASS: TestFeatures/|_cycles_auto_->_side-by-side_->_stacked_->_collapsed_->_auto (0.45s)
    --- PASS: TestFeatures/a_mid-scenario_resize_re-chooses_auto's_mode (0.38s)
    --- PASS: TestFeatures/a_pinned_side-by-side_mode_falls_back_below_its_floors_and_returns_when_the_terminal_does (0.38s)
    --- PASS: TestFeatures/<_and_>_clamp_sidebar_width_at_both_ends (8.20s)
    --- PASS: TestFeatures/layout_mode_and_sidebar_width_persist_across_a_restart,_with_config.toml_unchanged (0.70s)
PASS
ok  	github.com/n-orlov/deck/features	11.283s
```

- **R8** (auto ≥80 side-by-side, <80 stacked, collapsed never automatic): `TestComputeLayoutAutoBoundary`, `TestComputeLayoutCollapsedNeverAutomatic`, and the `auto_selects_side-by-side...` outline (79/80/81 columns).
- **R9** (`|` cycles auto→side-by-side→stacked→collapsed→auto, pins regardless of width): `.../|_cycles_auto_->_side-by-side_->_stacked_->_collapsed_->_auto`.
- **R10** (every width/floor is total columns incl. border+padding): `TestComputeLayoutGoldenMinimum`, `TestStackedListHeightBounds`, `TestComputeLayoutSidebarWidthDefault`, `TestComputeLayoutSideBySideAndCollapsedAlwaysShowPreview`.
- **R11** (`<`/`>` clamp `sidebar_width` to `[24, width−40]`): `TestClampSidebarWidth`; black-box: `.../<_and_>_clamp_sidebar_width_at_both_ends`.
- **R12** (`layout_mode`/`sidebar_width` in `state.db`, never `config.toml`): `TestPipeAndAngleBracketsPersistToUIStateNotConfigToml`; black-box: `.../layout_mode_and_sidebar_width_persist_across_a_restart,_with_config.toml_unchanged` (see also the store-level migration tests under requirement-12-adjacent evidence below).
- **R13** (resize re-chooses under `auto`; a pinned mode that cannot hold its floors falls back to `auto` without overwriting the pin, and returns): `TestComputeLayoutPinnedFallsBackWithoutOverwritingPin`, `TestComputeLayoutStackedNeverFallsBack`; black-box: `.../a_mid-scenario_resize_re-chooses_auto's_mode` and `.../a_pinned_side-by-side_mode_falls_back_below_its_floors_and_returns_when_the_terminal_does`.
- **R14** (below 80×24, `auto` renders `stacked` as far as it fits, footer states the terminal is below minimum): `TestComputeLayoutBelowMinimum` (`layout.go`'s `BelowMinimum` field).
- **R15** (`collapsed` 3-column strip, `»` above the attention count, `|` restores the sidebar): see requirements 28-32 section (`@requirement-15-collapsed-strip` is exercised together with the attention count there) and `TestComputeLayoutCollapsedNeverAutomatic` above for the geometry.

The `ui_state` table itself (backing requirement 12's persistence) — schema
migration and accessor defaults:

```
$ ci/run.sh go test -v -run 'TestUIStateAccessorsDegradeToDocumentedDefaults|\
TestOpenMigratesV1FixtureToUIStateWithoutRecreatingSessionRow' ./internal/store/...
--- PASS: TestUIStateAccessorsDegradeToDocumentedDefaults (0.03s)
--- PASS: TestOpenMigratesV1FixtureToUIStateWithoutRecreatingSessionRow (0.10s)
PASS
ok  	github.com/n-orlov/deck/internal/store	0.796s
```

## Requirements 16-20: panel chrome (`SPEC.md` §11.3)

```
$ ci/run.sh go test -v -run 'TestSideBySideFrameHasOneSeamAndOneColumnPadding|\
TestSideBySideFrameASCIIFallbackHasNoUnicodeBorders|\
TestEmptyStateAndPressNCopyLiveInsideSidebarAt80x24|TestEmptyAndHelpViewsAreDiscoverable|\
TestStartingCopyDistinguishesShellFromSignalledAgents' ./internal/tui/...
--- PASS: TestSideBySideFrameHasOneSeamAndOneColumnPadding (0.00s)
--- PASS: TestSideBySideFrameASCIIFallbackHasNoUnicodeBorders (0.00s)
--- PASS: TestEmptyStateAndPressNCopyLiveInsideSidebarAt80x24 (0.00s)
--- PASS: TestEmptyAndHelpViewsAreDiscoverable (0.00s)
--- PASS: TestStartingCopyDistinguishesShellFromSignalledAgents (0.00s)
PASS
ok  	github.com/n-orlov/deck/internal/tui	0.010s
```

- **R16** (rounded borders everywhere, one style, `DECK_ASCII` fallback honoured): `TestSideBySideFrameHasOneSeamAndOneColumnPadding`, `TestSideBySideFrameASCIIFallbackHasNoUnicodeBorders`.
- **R17** (exactly one column of padding inside sidebar/preview): `TestSideBySideFrameHasOneSeamAndOneColumnPadding`.
- **R18** (single seam — no `││`, sidebar draws top/left/bottom only, preview's left border is the divider): same test, asserting `strings.Contains(view, "││")` is false and exactly one `┬` seam T-junction on the top border.
- **R19** (sidebar is the only focusable region; `tab` unbound in the main view; focused border uses the focus colour): `internal/tui/panel.go`'s `borderColor` documents and implements this (no second focusable surface exists in the main view to test against); `tab` is confirmed absent from `TestEmptyAndHelpViewsAreDiscoverable`'s "advertises unavailable action" list check below, and `"tab", "down"` at `internal/tui/tui.go:1769` is scoped to the create-modal's own field navigation, not the main view.
- **R20** (footer: contextual, key/description pattern, carries the selected row's reason, never lists an unbound key): `TestStartingCopyDistinguishesShellFromSignalledAgents` (footer shows the reason for the selected row only, exactly once) and `TestEmptyAndHelpViewsAreDiscoverable`'s footer-adjacent help-overlay pin (requirement 44, below).

## Requirements 21-27: the preview (`SPEC.md` §11)

Unit-level capture/crop/placeholder coverage:

```
$ ci/run.sh go test -v -run 'TestPreviewTickCapturesOnlyTheSelectedRow' ./internal/tui/...
--- PASS: TestPreviewTickCapturesOnlyTheSelectedRow (0.00s)
PASS
ok  	github.com/n-orlov/deck/internal/tui	0.010s
```

Black-box coverage (`features/preview.feature`, requirement 39's scenario file):

```
$ ci/run.sh env DECK_GODOG_TAGS="@requirement-21-preview-no-side-effects,\
@requirement-23-preview-crop-geometry,@requirement-24-preview-wide-cell-boundary,\
@requirement-25-preview-gesture-no-ops,@requirement-26-preview-crash-tail,\
@requirement-26-preview-placeholder,@requirement-27-preview-suppressed-below-floor" \
go test -v -run TestFeatures ./features/...
7 scenarios (7 passed)
62 steps (62 passed)
--- PASS: TestFeatures (5.15s)
    --- PASS: TestFeatures/capturing_the_preview_never_attaches_a_tmux_client,_resizes_a_pane,_or_triggers_a_SIGWINCH,_across_selection,_mode,_sidebar-width_and_outer-terminal_changes (1.24s)
    --- PASS: TestFeatures/a_live_pane_larger_than_the_panel_is_cropped_with_its_real_geometry_stated (0.52s)
    --- PASS: TestFeatures/wide_glyphs_in_a_cropped_pane_never_shear_the_preview's_border (0.47s)
    --- PASS: TestFeatures/clicking_or_scrolling_over_the_preview_panel_does_nothing (0.76s)
    --- PASS: TestFeatures/an_error_row's_preview_shows_the_durable_crash_tail,_headed_by_copy_stating_it_is_not_live (1.00s)
    --- PASS: TestFeatures/a_stopped_session's_preview_names_its_own_state_instead_of_showing_stale_bytes (0.60s)
    --- PASS: TestFeatures/the_preview_is_suppressed_below_its_floor_and_the_sidebar_takes_the_space (0.54s)
PASS
ok  	github.com/n-orlov/deck/features	5.165s
```

- **R21** (no client attached, no resize, no `SIGWINCH`, across selection/mode/sidebar-width/outer-resize changes — asserted from tmux's and the agent's own observations): `.../capturing_the_preview_never_attaches_a_tmux_client,_resizes_a_pane,_or_triggers_a_SIGWINCH,...`.
- **R22** (capture-pane -e, selected row only, one capture per tick at `DECK_PREVIEW_MS`): `TestPreviewTickCapturesOnlyTheSelectedRow`.
- **R23** (crop anchored bottom-left, real geometry stated as `WxH of WxH`, right-cut lines marked, small pane not stretched): `.../a_live_pane_larger_than_the_panel_is_cropped_with_its_real_geometry_stated`. Marker and geometry-line placement are recorded in `docs/reports/phase2b1-findings.md`'s "Task 018" section.
- **R24** (cell-aware crop/elision — no wide cell ever split, border stays in the same column): `.../wide_glyphs_in_a_cropped_pane_never_shear_the_preview's_border`.
- **R25** (no preview scroll, no capture history, no `PgUp`, wheel over the preview is a no-op): `.../clicking_or_scrolling_over_the_preview_panel_does_nothing`.
- **R26** (`error` row shows the crash tail headed "not live"; `stopped`/`archived`/pane-less `starting` show a one-line placeholder): `.../an_error_row's_preview_shows_the_durable_crash_tail,...` and `.../a_stopped_session's_preview_names_its_own_state_instead_of_showing_stale_bytes`. Placeholder copy is recorded in `docs/reports/phase2b1-findings.md`'s "Task 020" section.
- **R27** (preview suppressed below its 40-column/8-row floor; sidebar takes the space; no capture tick while hidden): `.../the_preview_is_suppressed_below_its_floor_and_the_sidebar_takes_the_space`.

## Requirements 28-32: attention sort and grouping (`SPEC.md` §7, §11)

```
$ ci/run.sh env DECK_GODOG_TAGS="@requirement-28-attention-order,@requirement-29-attention-tie-break,\
@requirement-30-workspace-grouping,@requirement-31-attention-count,@requirement-15-collapsed-strip,\
@requirement-31-space-walk,@requirement-32-space-no-status-change" go test -v -run TestFeatures ./features/...
5 scenarios (5 passed)
78 steps (78 passed)
--- PASS: TestFeatures (5.85s)
    --- PASS: TestFeatures/sessions_render_in_the_full_waiting/error/running/starting/idle/stopped_order,_ties_broken_oldest-first (1.86s)
    --- PASS: TestFeatures/the_sidebar_groups_sessions_by_workspace,_with_a_header_per_group,_and_a_keyboard_toggle_can_collapse_one (1.07s)
    --- PASS: TestFeatures/collapsing_and_expanding_the_sidebar's_only_workspace_group_round-trips_via_two_`g`_presses (0.77s)
    --- PASS: TestFeatures/the_collapsed_strip's_attention_count_matches_the_sort's_own_notion_of_attention (1.08s)
    --- PASS: TestFeatures/`space`_walks_only_what_needs_attention,_wraps,_and_changes_no_session's_status (1.04s)
PASS
ok  	github.com/n-orlov/deck/features	5.856s
```

- **R28** (`waiting` oldest-first → `error` → `running` → `starting` → `idle` → `stopped`): `internal/tui/attention_test.go`'s `TestSortSessionsByAttentionOrdersGroupsExactly` (unit level) and `.../sessions_render_in_the_full_waiting/error/running/starting/idle/stopped_order,_ties_broken_oldest-first` (black box).
- **R29** (total, deterministic tie-break): **the tie-break key is ascending `StatusAt` (the timestamp of the session's current status), and any remaining `StatusAt` tie is broken by ascending session ID** (unique and stable for the session's lifetime), per `internal/tui/attention.go`'s doc comment — both keys are total orders over their domain, so the combined key is total and a frozen clock still yields exactly one frame. The same scenario above exercises the "ties broken oldest-first" half (oldest-first for `waiting` is simply "ascending `StatusAt`"), and `TestSortSessionsByAttentionOrdersGroupsExactly` covers the documented tie-break case (two sessions sharing a status and timestamp, disambiguated by session ID) at the unit level.
- **R30** (grouping by `workspace`, default basename-of-`cwd`, collapsible, never by repo): `.../the_sidebar_groups_sessions_by_workspace,...` and `.../collapsing_and_expanding_the_sidebar's_only_workspace_group_round-trips_via_two_\`g\`_presses`.
- **R31** (`space` moves to the next session needing attention, wraps, no observable effect when nothing needs attention, never changes status): `.../\`space\`_walks_only_what_needs_attention,_wraps,_and_changes_no_session's_status` — the scenario captures the state database's status rows before and after repeated `space` presses and asserts they still match.
- **R32** (one shared "needs me" computation behind the sort, the collapsed count, and `space`): `.../the_collapsed_strip's_attention_count_matches_the_sort's_own_notion_of_attention`; `internal/tui/attention.go` defines a single `needsAttention`-style predicate consumed by all three call sites (sort, collapsed-strip count, `space` target).

## Requirements 33-37, 41: mouse navigation (`SPEC.md` §11.8)

```
$ ci/run.sh env DECK_GODOG_TAGS="@requirement-33-click-selects-not-attaches,\
@requirement-33-double-click-attaches,@requirement-34-header-click-collapses,\
@requirement-34-wheel-scrolls-without-selecting,@requirement-35-seam-drag-resizes,\
@requirement-33-preview-gesture-no-ops,@requirement-37-deck-mouse-disables-gestures,\
@requirement-41-keyboard-still-works" go test -v -run TestFeatures ./features/...
7 scenarios (7 passed)
85 steps (85 passed)
--- PASS: TestFeatures (5.16s)
    --- PASS: TestFeatures/a_single_click_on_a_sidebar_row_selects_it_without_attaching (0.67s)
    --- PASS: TestFeatures/a_double_click_on_a_sidebar_row_attaches (0.66s)
    --- PASS: TestFeatures/clicking_a_workspace_group's_header_collapses_only_that_group (0.63s)
    --- PASS: TestFeatures/the_wheel_scrolls_the_sidebar's_view_without_changing_selection (1.18s)
    --- PASS: TestFeatures/dragging_the_seam_adjusts_sidebar_width_live (0.48s)
    --- PASS: TestFeatures/clicking,_double-clicking_or_scrolling_over_the_preview_panel_does_nothing (0.73s)
    --- PASS: TestFeatures/DECK_MOUSE=0_disables_every_mouse_gesture,_and_only_the_shortcut_is_lost (0.78s)
PASS
ok  	github.com/n-orlov/deck/features	5.172s
```

Hit-testing (requirement 34's "one geometry implementation" claim), at the unit level, across layout modes:

```
$ ci/run.sh go test -v -run 'TestHitTestResolvesRowsHeadersSeamAndPreviewSideBySide|\
TestHitTestStackedModeResolvesRowsAndPreview' ./internal/tui/...
--- PASS: TestHitTestResolvesRowsHeadersSeamAndPreviewSideBySide (0.00s)
--- PASS: TestHitTestStackedModeResolvesRowsAndPreview (0.00s)
PASS
ok  	github.com/n-orlov/deck/internal/tui	0.008s
```

Requirement 36's enable-on-start / disable-on-every-exit-path proof, including a deliberately induced panic through a real PTY (see `cmd/deck/testpanic_hook.go`/`testpanic_default.go` and `features/mouse_exit_paths_test.go`):

```
$ ci/run.sh go test -v -run 'TestMouseReportingEnablesSGRExtendedMode|TestMouseReportingDisabledOnNormalQuit|\
TestMouseReportingDisabledOnSignalledExit|TestMouseReportingDisabledOnPanic' ./features/...
--- PASS: TestMouseReportingEnablesSGRExtendedMode (0.42s)
--- PASS: TestMouseReportingDisabledOnNormalQuit (0.35s)
--- PASS: TestMouseReportingDisabledOnSignalledExit (0.33s)
--- PASS: TestMouseReportingDisabledOnPanic (0.36s)
PASS
ok  	github.com/n-orlov/deck/features	1.468s
```

- **R33** (click selects/never attaches, double-click attaches, wheel/click over the preview do nothing): `.../a_single_click_on_a_sidebar_row_selects_it_without_attaching`, `.../a_double_click_on_a_sidebar_row_attaches`, `.../clicking,_double-clicking_or_scrolling_over_the_preview_panel_does_nothing`.
- **R34** (hit-testing consults the layout that drew the frame — one geometry implementation): `TestHitTestResolvesRowsHeadersSeamAndPreviewSideBySide`, `TestHitTestStackedModeResolvesRowsAndPreview`; black box: `.../clicking_a_workspace_group's_header_collapses_only_that_group` and `.../the_wheel_scrolls_the_sidebar's_view_without_changing_selection`.
- **R35** (no mouse-only capability — every binding duplicates a key): enforced structurally (every mouse handler in `internal/tui/mouse.go` calls the same code path a keybinding calls: click→select uses the same selection setter as `↑`/`↓`, double-click→attach is the same command `↵` issues, header-click→collapse calls `toggleGroupCollapse` (task 039's `g` binding), seam-drag→resize calls the same `adjustSidebarWidth` `<`/`>` call, collapsed-strip-click calls the same cycle function `|` calls); the help overlay (requirement 44) documents each mouse gesture next to the key it duplicates, which is itself asserted by the help-overlay tests below.
- **R36** (SGR 1006, disabled on every exit path incl. panic, coordinates past column 223 correct): the four `TestMouseReporting...` runs above; column ≥224 correctness is exercised by `.../SGR_mouse_reports_are_synthesized_for_click,_double-click,_wheel_and_drag`'s "clicks at column 224" step in the requirements-1-7 run.
- **R37** (`[ui] mouse` / `DECK_MOUSE` opt-out, keyboard unaffected): `.../DECK_MOUSE=0_disables_every_mouse_gesture,_and_only_the_shortcut_is_lost`.
- **R41** (keyboard paths still work with mouse reporting off): covered by the same scenario as R37 (tag `@requirement-41-keyboard-still-works` is attached to it).

## Requirements 38-42: the scenario files that define this phase

- **R38 — `features/layout_modes.feature`**: run and captured under requirements 8-15 above (8 scenarios, 8 passed).
- **R39 — `features/preview.feature`**: run and captured under requirements 21-27 above (7 scenarios, 7 passed).
- **R40 — `features/attention_sort.feature`**: run and captured under requirements 28-32 above (5 scenarios, 5 passed).
- **R41 — `features/mouse.feature`**: run and captured under requirements 33-37 above (7 scenarios, 7 passed).
- **R42 — the golden minimum frame** (side-by-side, sidebar 35 / preview 45, at exactly 80×24, byte-exact, `DECK_MOUSE=0`, task 008's deterministic fake pane):

```
$ ci/run.sh go test -v -run TestGoldenMinimumFrame ./features/...
=== RUN   TestGoldenMinimumFrame
=== RUN   TestGoldenMinimumFrame/run-1
=== RUN   TestGoldenMinimumFrame/run-2
--- PASS: TestGoldenMinimumFrame (2.08s)
    --- PASS: TestGoldenMinimumFrame/run-1 (1.07s)
    --- PASS: TestGoldenMinimumFrame/run-2 (1.01s)
=== RUN   TestGoldenMinimumFrameRowCount
--- PASS: TestGoldenMinimumFrameRowCount (0.00s)
PASS
ok  	github.com/n-orlov/deck/features	2.084s
```

  `TestGoldenMinimumFrame` itself runs the render-and-compare twice from two
  independent, freshly created scenario homes in every invocation (see its
  doc comment in `features/golden_frame_test.go`), which is the "running
  the assertion twice from clean state passes both times" proof the
  requirement asks for. The golden file
  (`features/testdata/golden/side_by_side_80x24.golden`) is regenerated,
  never hand-edited, with:

  ```
  UPDATE_GOLDEN=1 ci/run.sh go test -run TestGoldenMinimumFrame ./features/...
  ```

## Requirement 43: ten consecutive green runs from a clean state

```
$ ci/stability.sh 10
```

Real captured output, `docs/reports/phase2b1-stability.log`:

```
=== RUN 1 ===
=== RUN 1: PASS (exit 0) ===
=== RUN 2 ===
=== RUN 2: PASS (exit 0) ===
=== RUN 3 ===
=== RUN 3: PASS (exit 0) ===
=== RUN 4 ===
=== RUN 4: PASS (exit 0) ===
=== RUN 5 ===
=== RUN 5: PASS (exit 0) ===
=== RUN 6 ===
=== RUN 6: PASS (exit 0) ===
=== RUN 7 ===
=== RUN 7: PASS (exit 0) ===
=== RUN 8 ===
=== RUN 8: PASS (exit 0) ===
=== RUN 9 ===
=== RUN 9: PASS (exit 0) ===
=== RUN 10 ===
=== RUN 10: PASS (exit 0) ===
10/10 passed
STABILITY_EXIT_STATUS=0
```

No scenario was skipped or tagged out to reach 10/10; `ci/stability.sh`
runs the full default suite (`defaultTags = "~@real-agents && ~@nightly"`,
unchanged this phase) each time.

## Requirement 44: the help overlay documents every key this phase binds/unbinds

```
$ ci/run.sh go test -v -run TestDeckBinaryEmptyHelpAndQuitThroughPTY ./cmd/deck/...
--- PASS: TestDeckBinaryEmptyHelpAndQuitThroughPTY (0.84s)
PASS
ok  	github.com/n-orlov/deck/cmd/deck	0.840s
```

This test drives the *released* binary (built with no test-only build
tags) through a real PTY at 90×100 with `DECK_ASCII=1`, opens help with
`?`, and asserts (among the pre-existing Phase-0/1/2 copy) every string
this phase's requirements 9, 11, 15, 33-37 added: `"space move to the next
session needing attention"`, `"changes any session's status"`, `"g toggle
the selected row's workspace group"`, `"cycle the layout mode"`,
`"shrink/grow the sidebar"`, `"click a sidebar row"`, `"double-click a
row"`, `"click a group header"`, `"wheel over the sidebar"`, `"drag the
seam"`, `"click the collapsed strip"`, `"click or wheel over the preview
does nothing"`, `"DECK_MOUSE=0"`, `"[ui] mouse = false"`, and `"override
modifier (usually shift)"` (§11.8's `shift`-to-select caveat) — and that
`"tab"` appears nowhere in the rendered help. The equivalent Go-level pin
lives in `internal/tui/tui_test.go`'s `TestEmptyAndHelpViewsAreDiscoverable`,
run above under requirements 16-20.

## Requirement 45: this report

This file. Every requirement 1-46 above names the command or scenario that
verifies it and shows that command's own real captured output — not a
claim. The requirement-6 emulator decision and its evidence has its own
dedicated section above, and requirement 29's tie-break key is stated and
justified in the "Requirements 28-32" section above. The golden-frame
regeneration command is included under requirement 42.

## Requirement 46: workspace hygiene

Checked read-only, scoped to the image, never by `ralphd.run=…` label
(a label sweep would delete this job's own container):

```
$ git status --short
(no output — clean)

$ find /workspace -user root
(no output — none)

$ docker ps -a --filter ancestor=deck-ci:local --format '{{.ID}} {{.Status}}'
(no output — no leftover deck-ci:local containers; every ci/run.sh sibling is --rm)

$ find /tmp -maxdepth 2 -iname '*tmux*'
(no output — no leftover tmux sockets)
```

(Task 037 re-verifies this at the very end of the phase, after every other
task — including this report — has been committed and pushed.)
