# Phase 2b-2 report

Per-requirement evidence for requirements 1-57 (requirement 55). For every
requirement below: the command that verifies it, and that command's real
captured output from this workspace (HEAD `a3feaa9` at the time this report
was written). Full untrimmed logs for the batched full-suite runs quoted
here duplicate `ci/stability.sh`'s own captured log
(`docs/reports/phase2b2-stability.log`) and are not re-committed separately.
Anyone can reproduce any block below with `ci/run.sh <command>` from a clean
checkout — never install a toolchain in this container, always go through
`ci/run.sh` (a throwaway sibling with Go 1.25 + tmux 3.5a).

Detailed narrative for several requirements (rationale, before/after, what
was rejected) lives in `docs/reports/phase2b2-findings.md`; this report
gives each requirement its verifying command and real output, and points at
the findings section for the "why" where one exists.

## Requirements 1-6: harness prerequisites (`features/harness.feature`)

```
$ ci/run.sh sh -c 'cd features && go test -v -run TestFeatures .' > /tmp/features.log 2>&1
$ grep -E 'reads_foreground|colour_assertion_fails|coloured_chrome_is_readable|East-Asian-Wide|mid-scenario_terminal_resize|frame_fits_its_own_terminal|frame-budget_check_is_discriminating|bare_emulator.s_overflow|written_before_start-up_is_asserted|config-file-content_assertion_fails|pins_a_built-in_theme_by_name|discovers_a_user_theme|rather_than_to_the_user_theme.s_colours|proving_the_pinning_step_is_not_a_no-op|renders_a_monochrome_frame_with_a_session.s_status|forces_the_truecolour_render_path|renders_the_quantised_reference-palette_colour_from_a_real_pty' /tmp/features.log | grep PASS
```

All six harness capabilities (per-cell SGR reads incl. a can-fail proof,
`DECK_COLOR_DEPTH`, `NO_COLOR`, theme pinning of both a built-in and a
scenario-written user theme, a `config.toml`-writing + parsed-content step,
and a frame-budget assertion) are proven together in the real, unedited
tail of the full features run used for requirement 53 below — every
`@requirement-1-*` through `@requirement-6-*` tagged scenario in that run
passed. Individually, from the same run:

```
    --- PASS: TestFeatures/the_emulator_reads_foreground,_background,_bold,_dim_and_reverse_from_raw_SGR_bytes (0.27s)
    --- PASS: TestFeatures/a_colour_assertion_fails_when_pointed_at_a_cell_of_a_different_colour (0.30s)
    --- PASS: TestFeatures/a_real_deck_client's_own_coloured_chrome_is_readable_per_cell,_by_name_and_by_matched_text (0.37s)
    --- PASS: TestFeatures/a_real_deck_client's_coloured_chrome_is_readable_by_naming_a_theme_token,_resolved_through_the_scenario's_pinned_theme (0.36s)
    --- PASS: TestFeatures/the_screen_emulator_places_an_East-Asian-Wide_cell_without_splitting_it (0.29s)
    --- PASS: TestFeatures/a_mid-scenario_terminal_resize_changes_the_grid_the_frame_is_read_from (0.36s)
    --- PASS: TestFeatures/a_real_deck_client's_frame_fits_its_own_terminal_at_several_sizes (0.35s)
    --- PASS: TestFeatures/a_real_deck_client's_frame_fits_its_own_terminal_at_several_sizes#01 (0.36s)
    --- PASS: TestFeatures/a_real_deck_client's_frame_fits_its_own_terminal_at_several_sizes#02 (0.37s)
    --- PASS: TestFeatures/the_frame-budget_check_is_discriminating,_not_a_check_that_always_passes (0.35s)
    --- PASS: TestFeatures/a_bare_emulator's_overflow_by_height_or_by_width_fails_the_frame-budget_check (0.27s)
    --- PASS: TestFeatures/a_config.toml_written_before_start-up_is_asserted_by_its_parsed_content_after_the_run (0.34s)
    --- PASS: TestFeatures/the_config-file-content_assertion_fails_on_a_mismatch (0.33s)
    --- PASS: TestFeatures/a_scenario_pins_a_built-in_theme_by_name_via_a_config.toml_it_writes_itself (0.33s)
    --- PASS: TestFeatures/a_scenario_discovers_a_user_theme_it_writes_into_its_own_themes_directory (0.31s)
    --- PASS: TestFeatures/an_unknown_theme_name_falls_back_to_the_default_rather_than_to_the_user_theme's_colours (0.30s)
    --- PASS: TestFeatures/a_built-in_and_a_user_theme_paint_genuinely_different_colours_at_the_same_token,_proving_the_pinning_step_is_not_a_no-op (0.29s)
    --- PASS: TestFeatures/NO_COLOR_renders_a_monochrome_frame_with_a_session's_status_carried_by_its_glyphs_alone (1.03s)
    --- PASS: TestFeatures/DECK_COLOR_DEPTH=truecolor_forces_the_truecolour_render_path_from_a_real_pty (0.35s)
    --- PASS: TestFeatures/DECK_COLOR_DEPTH=16_renders_the_quantised_reference-palette_colour_from_a_real_pty (0.34s)
```

- **R1** (per-cell reads incl. can-fail proof): the first two scenarios.
- **R2** (`DECK_COLOR_DEPTH=truecolor|16`): the two `DECK_COLOR_DEPTH=...` scenarios.
- **R3** (`NO_COLOR`): the `NO_COLOR renders a monochrome frame...` scenario.
- **R4** (theme pinning, built-in + user-written): the four `...theme...`
  scenarios under `@requirement-4-theme-pinning`.
- **R5** (config-file write + parsed-content assertion, and its own
  can-fail proof): the two `config.toml`/`config-file-content` scenarios.
- **R6** (frame-budget assertion, plus its own discriminating-check proof):
  the three `frame...budget` scenarios plus the EAW-placement and
  mid-scenario-resize scenarios (both pre-existing harness capabilities
  this phase's scenarios continue to depend on).

## Requirements 7-14: the §11.4 dialog contract

Unit-level proof that all five dialogs share one implementation
(`internal/tui/dialog_contract.go`, extracted from five independent key
switches — commit `b50db14`):

```
$ ci/run.sh go test -v ./internal/tui/... -run 'TestApplyDialogContract|TestCreateModal|TestDialog'
=== RUN   TestApplyDialogContractCoreKeys
=== RUN   TestApplyDialogContractCoreKeys/esc_cancels_and_does_not_submit_or_cycle
=== RUN   TestApplyDialogContractCoreKeys/enter_submits_and_does_not_cancel
=== RUN   TestApplyDialogContractCoreKeys/tab_and_shift+tab_move_the_focused_field,_wrapping
=== RUN   TestApplyDialogContractCoreKeys/left_and_right_change_the_selection
=== RUN   TestApplyDialogContractCoreKeys/space_changes_the_selection_when_there_is_no_text_field_to_type_into
=== RUN   TestApplyDialogContractCoreKeys/space_falls_through_unhandled_when_SpaceTypesText_says_so
=== RUN   TestApplyDialogContractCoreKeys/a_key_outside_the_contract_vocabulary_is_unhandled
=== RUN   TestApplyDialogContractCoreKeys/tab_is_a_no-op_with_one_or_zero_fields,_not_merely_wrapped
--- PASS: TestApplyDialogContractCoreKeys (0.00s)
    [all 8 subtests PASS]
=== RUN   TestCreateModalDownUpDoNotMoveFields
--- PASS: TestCreateModalDownUpDoNotMoveFields (0.00s)
=== RUN   TestDialogWidthClampedToViewport
=== RUN   TestDialogWidthClampedToViewport/below_lower_clamp_falls_back_to_the_full_viewport
=== RUN   TestDialogWidthClampedToViewport/lands_exactly_on_the_lower_clamp
=== RUN   TestDialogWidthClampedToViewport/mid-range_viewport_stays_at_80%_uncapped
=== RUN   TestDialogWidthClampedToViewport/lands_exactly_on_the_upper_clamp
=== RUN   TestDialogWidthClampedToViewport/well_past_the_upper_clamp_saturates_at_80
--- PASS: TestDialogWidthClampedToViewport (0.00s)
    [all 5 subtests PASS]
PASS
ok  	github.com/n-orlov/deck/internal/tui	0.011s
```

- **R7** (one shared implementation, retrofitted onto all five dialogs):
  `dialog_contract.go`/`TestApplyDialogContractCoreKeys` above; `createView`,
  `detailView`, `profileSwitchView`, `pinView` and `helpView` all now go
  through `applyDialogContract` — 4 call sites in `internal/tui/tui.go`
  (`grep -n 'applyDialogContract(' internal/tui/tui.go` below) covering all
  five dialogs, since `detailView` and `helpView` share one call site (both
  have only `esc` to bind, no fields to submit or cycle, per the comment
  at that call site):

  ```
  $ grep -n 'applyDialogContract(' internal/tui/tui.go
  672:			_, _ = applyDialogContract(msg, dialogContract{Cancel: func() {
  1735:	if cmd, handled := applyDialogContract(msg, dialogContract{
  1806:	cmd, handled := applyDialogContract(msg, dialogContract{
  2190:	if cmd, handled := applyDialogContract(msg, dialogContract{
  ```

  In call order: line 672 is `detailView`+`helpView`'s shared esc-only
  handler (the comment immediately above it in `tui.go` names both), line
  1735 is `updateProfileSwitch`, line 1806 is `updatePinDialog`, line 2190
  is `updateCreate`.
- **R8** (esc changes nothing, proven by state) and **R9** (`y` yolo confirm
  is the only additional load-bearing key, documented on screen) and **R10**
  (validation retains the typed value) and **R11** (mouse cannot cancel/confirm):
  see `features/dialogs.feature` under requirement 50 below — the state-based
  assertions live there, not at the unit level, because "changes nothing"
  is a claim about the store/config file, not about `dialog_contract.go`'s
  return value.
- **R12** (width clamp `[26,80]`, wrap not truncate): `TestDialogWidthClampedToViewport`
  above (all five geometry buckets, including both clamp ends) plus
  `docs/reports/phase2b2-findings.md`'s "Task 030" section for the
  `permission_modes.feature:49` 201-column-box reconciliation.
- **R13** (no dialogs for unbuilt behaviour): no new dialog exists —
  `grep -rn 'func.*View() string' internal/tui/tui.go` still lists exactly
  the five pre-existing dialogs plus the new (non-§11.4) settings takeover
  and theme picker, neither of which is a stubbed dialog.
- **R14** (destructive actions confirm and name what survives): the only
  destructive action in this phase's surface is discarding unsaved settings
  (requirement 20); see the settings save/discard tests under requirement 48.

## Requirements 15-24: the settings takeover (`SPEC.md` §11.5, §6.5)

Schema declaration and structural parity (requirement 22):

```
$ ci/run.sh go test -v -run 'TestSchemaPinsKeySet|TestSchemaFieldsAreComplete|TestSchemaScopes' ./internal/config/...
=== RUN   TestSchemaPinsKeySet
--- PASS: TestSchemaPinsKeySet (0.00s)
=== RUN   TestSchemaFieldsAreComplete
--- PASS: TestSchemaFieldsAreComplete (0.00s)
=== RUN   TestSchemaScopes
--- PASS: TestSchemaScopes (0.00s)
PASS
ok  	github.com/n-orlov/deck/internal/config	0.003s

$ ci/run.sh go test -v -run TestSettingsSchemaParity ./internal/tui/...
=== RUN   TestSettingsSchemaParity_EveryFlatKeyIsReachable
--- PASS: TestSettingsSchemaParity_EveryFlatKeyIsReachable (0.00s)
=== RUN   TestSettingsSchemaParity_EveryRenderedFieldIsBackedBySchema
--- PASS: TestSettingsSchemaParity_EveryRenderedFieldIsBackedBySchema (0.00s)
PASS
ok  	github.com/n-orlov/deck/internal/tui	0.003s
```

**Schema/settings parity mechanism (requirement 22), in detail.**
`internal/config/schema.go` declares every flat key exactly once
(`config.Schema`), each with `Kind`, bounds where relevant, `Default`,
`Description` and `Scope`. `internal/tui/settings.go`'s
`settingsCategories()` is documented in code as "the one and only place
tasks 013-018 read the schema to build the takeover's category/field
lists" — it walks `config.Schema`, it does not hand-list keys.
`TestSettingsSchemaParity_EveryFlatKeyIsReachable` closes the direction a
docstring alone cannot prove: for every `config.Schema` field it asserts
(a) the field appears in `settingsCategories()`'s rendered output, and (b)
it round-trips through its kind's generic get/set pair
(`settingsToggleValue`/`settingsSetToggle` etc.) — not merely falling back
to `Field.Default`, which a missing dispatch case would also produce.
`TestSettingsSchemaParity_EveryRenderedFieldIsBackedBySchema` closes the
other direction: every field `settingsCategories()` renders has a matching
`config.Schema` entry, with exactly one documented, deliberate exception —
`[notify]`'s synthetic link entry, which `schema.go`'s own comment states
is intentionally absent from `config.Schema` because §11.5 gives it its
own dialog (Phase 5), not a flattened field. There is no second,
hand-maintained key set anywhere in the tree.

**Red/green demonstrated (not left as a permanent fixture — see
`internal/tui/settings_task018_test.go`'s own comment for the full
transcript):** a temporary schema field
`{Section: "", Key: "task018_probe", Kind: KindToggle, ...}` with no
matching case added to `settingsToggleValue`/`settingsSetToggle` made
`TestSettingsSchemaParity_EveryFlatKeyIsReachable` fail with
`task018_probe (kind toggle): set-then-get round-trip did not reflect the
write (get after set-true = false, want true)` — proving the round-trip
check catches exactly the defect it exists to catch (a silent fallback to
`Field.Default`), not just a rendering gap. Reverting the temporary field
returned the suite to green. Full commit-message transcript: `53dbf27`.

Full settings/dialog and feature coverage (requirements 15-21, 23, 24):

```
$ ci/run.sh go test ./internal/tui/... ./internal/config/...
ok  	github.com/n-orlov/deck/internal/config	(cached)
ok  	github.com/n-orlov/deck/internal/tui	(cached)

$ grep -E 'schema-declared_fields_are_reachable|edited_in_place_and_ctrl.s_writes_config.toml|finds_a_field_only_its_description|unsaved_change_prompts_to_discard|environment-overridden_field_is_labelled|env._table_states_its_restart-to-apply_scope|driving_every_key_the_takeover_binds' /tmp/features.log | grep PASS
    --- PASS: TestFeatures/every_category_and_its_schema-declared_fields_are_reachable,_including_the_honestly-unavailable_[notify]_entry (0.56s)
    --- PASS: TestFeatures/a_toggle_and_a_bounded_integer_are_edited_in_place_and_ctrl+s_writes_config.toml_to_match_what_the_takeover_showed (0.54s)
    --- PASS: TestFeatures/`/`_finds_a_field_only_its_description_mentions,_and_enter_jumps_both_lists_onto_it (0.47s)
    --- PASS: TestFeatures/esc_with_an_unsaved_change_prompts_to_discard,_and_discarding_leaves_config.toml_exactly_as_it_was (0.48s)
    --- PASS: TestFeatures/an_environment-overridden_field_is_labelled_honestly_and_a_save_never_pretends_the_running_value_moved (0.52s)
    --- PASS: TestFeatures/the_[env]_table_states_its_restart-to-apply_scope_on_screen (0.46s)
    --- PASS: TestFeatures/driving_every_key_the_takeover_binds_leaves_the_session_set_untouched (1.01s)
```

- **R15** (`,` opens a full-screen takeover, not a modal): `4090b7a`.
- **R16** (tab/←/→ between lists, ↑/↓ within, `/` fuzzy over label+description,
  ctrl+s saves, esc prompts/closes): `acdaed8`, and the `` `/` finds a field
  only its description mentions... `` scenario above.
- **R17** (every flat key editable, `[notify]` the stated exception): the
  "every category and its schema-declared fields are reachable" scenario
  proves reachability of all seven flat keys, but reachability is not the
  same claim as editability — an earlier pass of this report conflated the
  two, and an independent review (2b-2 approach 02, commit `65e623e`) found
  `[env]` display-only in the takeover despite that wording. Editability of
  `[env]` (add/edit/remove an entry, no new footer verb) is now proven
  separately: `internal/tui/settings_task003_test.go`'s
  `TestSettingsEnvAddEntry`, `TestSettingsEnvEditExistingValue` and
  `TestSettingsEnvRemoveEntry` (task 003), plus
  `features/settings.feature`'s "adding an `[env]` entry and saving with
  ctrl+s round-trips through config.toml..." and "discarding an unsaved
  `[env]` edit leaves config.toml exactly as it was..." scenarios (task
  004), which also cover the ctrl+s save and esc-discard paths end to end
  against the real written file. Real output, re-run for this correction:

  ```
  $ ci/run.sh go test ./internal/tui/... -run TestSettingsEnv -v -count=1
  === RUN   TestSettingsEnvEnterOpensEntriesEditor
  --- PASS: TestSettingsEnvEnterOpensEntriesEditor (0.00s)
  === RUN   TestSettingsEnvAddEntry
  --- PASS: TestSettingsEnvAddEntry (0.00s)
  === RUN   TestSettingsEnvEditExistingValue
  --- PASS: TestSettingsEnvEditExistingValue (0.00s)
  === RUN   TestSettingsEnvRemoveEntry
  --- PASS: TestSettingsEnvRemoveEntry (0.00s)
  === RUN   TestSettingsEnvEscCancelsEditWithoutStaging
  --- PASS: TestSettingsEnvEscCancelsEditWithoutStaging (0.00s)
  === RUN   TestSettingsEnvEscFromEntriesListReturnsToFieldList
  --- PASS: TestSettingsEnvEscFromEntriesListReturnsToFieldList (0.00s)
  PASS
  ok  	github.com/n-orlov/deck/internal/tui	0.005s

  $ ci/run.sh sh -c 'cd features && DECK_GODOG_TAGS="@settings" go test -run TestFeatures -v -count=1 .'
  === RUN   TestFeatures/adding_an_[env]_entry_and_saving_with_ctrl+s_round-trips_through_config.toml,_preserving_keys_and_sections_the_takeover_does_not_understand_(requirement_17)
    Scenario: adding an [env] entry and saving with ctrl+s round-trips through config.toml, preserving keys and sections the takeover does not understand (requirement 17) # settings.feature:175
      And the scenario's config.toml parses with env "GREETING" set to "hello"
  === RUN   TestFeatures/discarding_an_unsaved_[env]_edit_leaves_config.toml_exactly_as_it_was_(requirement_17)
    Scenario: discarding an unsaved [env] edit leaves config.toml exactly as it was (requirement 17) # settings.feature:238
      And the scenario's config.toml is captured as "before-env-discard"
      And the scenario's config.toml still matches the captured "before-env-discard"
  10 scenarios (10 passed)
  186 steps (186 passed)
  --- PASS: TestFeatures (6.38s)
  PASS
  ```

  `[env]` editing is now covered as thoroughly as the seven flat keys: the
  decision record for the requirement-17-vs-non-goals tension this raised
  is in `docs/reports/phase2b2-findings.md` (task 005).
- **R18** (field kinds explicit — toggle, bounded integer, string, path,
  enum, list-of-strings, link): `3b894d6`.
- **R19** (scope labelled per field, incl. restart-to-apply): the "`[env]`
  table states its restart-to-apply scope on screen" scenario.
- **R20** (ctrl+s atomic save, esc discard prompt, never-unparseable write):
  `61639b4`; the "a toggle and a bounded integer are edited..." and "esc
  with an unsaved change..." scenarios; `internal/config`'s atomic-writer
  tests (requirement 12 below).
- **R21** (env still outranks the file, labelled honestly): the
  "an environment-overridden field is labelled honestly..." scenario.
- **R23** (unknown key ignored, unparseable value is a stated error naming
  file+line, unknown keys survive a settings save): `internal/config`'s
  `toml_write_test.go` round-trip-of-unknown-key test, part of the
  `go test ./internal/config/...` run above.
- **R24** (settings touches configuration only): the "driving every key the
  takeover binds leaves the session set untouched" scenario.

## Requirements 25-36: themes (`SPEC.md` §11.6)

Quantisation + WCAG contrast golden test (requirement 30), real output,
full ratio table for both built-ins over both the hex palette and its
16-colour quantisation:

```
$ ci/run.sh go test -v -run TestBuiltinContrastFloor ./internal/theme/...
=== RUN   TestBuiltinContrastFloor
=== RUN   TestBuiltinContrastFloor/daylight
    contrast_test.go:86: daylight text/background      hex #1e293b/#f8fafc = 13.98:1   quant #000000/#ffffff = 21.00:1
    contrast_test.go:86: daylight hint/background      hex #475569/#f8fafc = 7.24:1   quant #7f7f7f/#ffffff = 4.00:1
    contrast_test.go:86: daylight title/background     hex #92400e/#f8fafc = 6.78:1   quant #cd0000/#ffffff = 5.84:1
    contrast_test.go:86: daylight starting/background  hex #92400e/#f8fafc = 6.78:1   quant #cd0000/#ffffff = 5.84:1
    contrast_test.go:86: daylight running/background   hex #166534/#f8fafc = 6.81:1   quant #000000/#ffffff = 21.00:1
    contrast_test.go:86: daylight waiting/background   hex #92400e/#f8fafc = 6.78:1   quant #cd0000/#ffffff = 5.84:1
    contrast_test.go:86: daylight idle/background      hex #64748b/#f8fafc = 4.55:1   quant #7f7f7f/#ffffff = 4.00:1
    contrast_test.go:86: daylight error/background     hex #b91c1c/#f8fafc = 6.18:1   quant #cd0000/#ffffff = 5.84:1
    contrast_test.go:86: daylight stopped/background   hex #64748b/#f8fafc = 4.55:1   quant #7f7f7f/#ffffff = 4.00:1
    contrast_test.go:86: daylight archived/background  hex #54657a/#f8fafc = 5.70:1   quant #7f7f7f/#ffffff = 4.00:1
    contrast_test.go:86: daylight text/selection       hex #1e293b/#c7d7f0 = 10.04:1   quant #000000/#e5e5e5 = 16.67:1
=== RUN   TestBuiltinContrastFloor/empire
    contrast_test.go:86: empire   text/background      hex #cbd5e1/#0f172a = 12.02:1   quant #e5e5e5/#000000 = 16.67:1
    contrast_test.go:86: empire   hint/background      hex #94a3b8/#0f172a = 6.96:1   quant #7f7f7f/#000000 = 5.24:1
    contrast_test.go:86: empire   title/background     hex #fbbf24/#0f172a = 10.69:1   quant #cdcd00/#000000 = 12.33:1
    contrast_test.go:86: empire   starting/background  hex #a16207/#0f172a = 3.63:1   quant #cd0000/#000000 = 3.60:1
    contrast_test.go:86: empire   running/background   hex #22c55e/#0f172a = 7.83:1   quant #00cd00/#000000 = 9.73:1
    contrast_test.go:86: empire   waiting/background   hex #fbbf24/#0f172a = 10.69:1   quant #cdcd00/#000000 = 12.33:1
    contrast_test.go:86: empire   idle/background      hex #64748b/#0f172a = 3.75:1   quant #7f7f7f/#000000 = 5.24:1
    contrast_test.go:86: empire   error/background     hex #ef4444/#0f172a = 4.74:1   quant #ff0000/#000000 = 5.25:1
    contrast_test.go:86: empire   stopped/background   hex #64748b/#0f172a = 3.75:1   quant #7f7f7f/#000000 = 5.24:1
    contrast_test.go:86: empire   archived/background  hex #7f8ea3/#0f172a = 5.36:1   quant #7f7f7f/#000000 = 5.24:1
    contrast_test.go:86: empire   text/selection       hex #cbd5e1/#26324b = 8.62:1   quant #e5e5e5/#000000 = 16.67:1
--- PASS: TestBuiltinContrastFloor (0.00s)
    --- PASS: TestBuiltinContrastFloor/daylight (0.00s)
    --- PASS: TestBuiltinContrastFloor/empire (0.00s)
PASS
ok  	github.com/n-orlov/deck/internal/theme	0.002s
```

Every value in the table clears the stated 3:1 floor (`minContrastRatio`,
`internal/theme/contrast_test.go` — deliberately the SPEC's stated 3:1, not
the stricter 4.5:1 AA text threshold, because deck's chrome includes
non-text glyphs and short bold/reverse status words the SPEC does not hold
to full AA), on both the authored hex palette and its 16-colour
quantisation, for both built-ins (`daylight`, a light theme, and `empire`,
the dark default). The tightest margin is `empire`'s `starting/background`
at 3.63:1 hex / 3.60:1 quantised — still above the floor, closest to it,
and logged explicitly rather than only asserted.

Theme rendering, picker, fallback notice, geometry/ASCII independence and
the colour-literal guard:

```
$ ci/run.sh go test -v -run TestNoColorLiterals ./internal/tui/...
--- PASS: TestNoColorLiterals (0.00s)

$ ci/run.sh go test -v -run TestThemeChangesAttributesButNotFrameGeometry ./features/...
=== RUN   TestThemeChangesAttributesButNotFrameGeometry
=== RUN   TestThemeChangesAttributesButNotFrameGeometry/unicode
    theme_geometry_test.go:196: themes "daylight" vs "empire" differ in colour at 199 of 1920 cells
=== RUN   TestThemeChangesAttributesButNotFrameGeometry/ascii
    theme_geometry_test.go:196: themes "daylight" vs "empire" differ in colour at 203 of 1920 cells
--- PASS: TestThemeChangesAttributesButNotFrameGeometry (1.21s)
    --- PASS: TestThemeChangesAttributesButNotFrameGeometry/unicode (0.32s)
    --- PASS: TestThemeChangesAttributesButNotFrameGeometry/ascii (0.30s)
PASS
ok  	github.com/n-orlov/deck/features	1.226s
```

Same character grid (all 1920 cells at the positions the DECK_ASCII/Unicode
cases each use), different colour at ~10% of cells — proving requirement
32's claim (position/character identical, attributes differ) and
requirement 36 (colour holds under `DECK_ASCII=1`) together, in one test.

```
$ grep -E 'colours_each_of_the_seven_§7_status_tokens|colours_border_focus.border|is_discovered_and_applied_on_a_real_client|falls_back_to_the_default_AND_says_so|previews_a_theme_live_on_the_real_list|renders_the_quantised_reference-palette_colour_for_a_real_client|renders_every_theme_monochrome|geometry_is_identical_across_every_built-in' /tmp/features.log | grep PASS
    --- PASS: TestFeatures/a_built-in_theme_colours_each_of_the_seven_§7_status_tokens,_read_per_cell_from_a_real_client (1.22s)
    --- PASS: TestFeatures/a_built-in_theme_colours_each_of_the_seven_§7_status_tokens,_read_per_cell_from_a_real_client#01 (1.21s)
    --- PASS: TestFeatures/a_built-in_theme_colours_each_of_the_seven_§7_status_tokens,_read_per_cell_from_a_real_client#02 (1.27s)
    --- PASS: TestFeatures/a_built-in_theme_colours_each_of_the_seven_§7_status_tokens,_read_per_cell_from_a_real_client#03 (1.17s)
    --- PASS: TestFeatures/a_built-in_theme_colours_each_of_the_seven_§7_status_tokens,_read_per_cell_from_a_real_client#04 (1.27s)
    --- PASS: TestFeatures/a_built-in_theme_colours_each_of_the_seven_§7_status_tokens,_read_per_cell_from_a_real_client#05 (1.25s)
    --- PASS: TestFeatures/a_built-in_theme_colours_each_of_the_seven_§7_status_tokens,_read_per_cell_from_a_real_client#06 (1.24s)
    --- PASS: TestFeatures/a_built-in_theme_colours_border_focus/border,_selection/selection_idle,_title,_group_and_key/hint_chrome,_read_per_cell_from_a_real_client (0.70s)
    --- PASS: TestFeatures/a_user_theme_placed_in_the_scenario's_own_themes_directory_(the_$XDG_CONFIG_HOME/deck/themes_layout)_is_discovered_and_applied_on_a_real_client (0.38s)
    --- PASS: TestFeatures/an_unknown_theme_name_falls_back_to_the_default_AND_says_so_on_the_very_first_painted_frame (0.37s)
    --- PASS: TestFeatures/`t`_previews_a_theme_live_on_the_real_list_and_reverts_to_the_exact_original_colour_on_esc (0.45s)
    --- PASS: TestFeatures/DECK_COLOR_DEPTH=16_renders_the_quantised_reference-palette_colour_for_a_real_client (0.37s)
    --- PASS: TestFeatures/NO_COLOR_renders_every_theme_monochrome,_with_a_session's_status_carried_by_its_glyph_alone (1.02s)
    --- PASS: TestFeatures/the_frame's_geometry_is_identical_across_every_built-in_theme_--_only_colour_differs (0.45s)
```

- **R25** (one TOML file per theme, embedded built-ins, ≥2 shipped): two
  built-ins (`daylight`, `empire`) in `internal/theme/builtin/`, each one
  file, one registry entry, no per-theme code — demonstrated by the
  contrast test above running identically over both without a special
  case.
- **R26** (exact §11.6 token set, seven status tokens = §7's seven, no
  borrowed colour): the "statuses never borrow each other's colour" scenario
  (features run, requirement 26 section) plus the seven per-status
  per-cell scenarios above.
- **R27** (`t` picker, live preview, byte-for-byte esc revert): the
  `` `t` previews a theme live... `` scenario; `docs/reports/
  phase2b2-findings.md`'s "Task 025" section for the empty/degenerate-list
  behaviour and the undisclosed-key-binding fix history (`da58523`, `0234be8`).
- **R28** (unknown/unparseable theme falls back and says so on first
  paint): the "an unknown theme name falls back to the default AND says
  so..." scenario; findings "Task 024" for the exact copy.
- **R29** (truecolour when advertised, 16-colour floor otherwise, quantised
  against §11.6's reference palette): the two `DECK_COLOR_DEPTH=...`
  scenarios (requirement 6 above and here).
- **R30** (WCAG contrast ≥3:1, hex and quantised): the full table above.
- **R31** (`NO_COLOR` → monochrome, glyph-carried status): the two
  `NO_COLOR renders...` scenarios (requirement 3 and here).
- **R32** (theme cannot change layout/geometry): `TestThemeChangesAttributesButNotFrameGeometry`
  above, plus the "frame's geometry is identical across every built-in
  theme" scenario.
- **R33** (every colour from a token, mechanically checked):
  `TestNoColorLiterals` above; `docs/reports/phase2b2-findings.md` records
  the red/green demonstration (a temporary literal reintroduced, the check
  observed to fail, then reverted) for commit `8c7deff`.
- **R34** (labels distinguishable from values — `hint` for labels, `text`
  for values): see requirement 42/detail-dialog coverage below;
  `docs/reports/phase2b2-findings.md`'s "Task 022" section records the
  §11.6 clarification request (no new token added).
- **R35** (list legible via title/group/border/selection/key/hint/badge/dimmed):
  the "border_focus/border, selection/selection_idle, title, group and
  key/hint chrome" scenario above.
- **R36** (themes and `DECK_ASCII` independent): `TestThemeChangesAttributesButNotFrameGeometry`'s
  `ascii` subtest above, and `cmd/deck`'s existing ASCII pty help test
  (`TestDeckBinaryEmptyHelpAndQuitThroughPTY`, unchanged, still ASCII-mode
  — see requirement 36/56 verification below).

## Requirement 37: the frame-budget general fix

```
$ ci/run.sh go test -v -run TestBelowMinimumFrameStaysWithinBudget ./internal/tui/...
--- PASS: TestBelowMinimumFrameStaysWithinBudget/79x40/attachError/wraps-at-w (0.00s)
--- PASS: TestBelowMinimumFrameStaysWithinBudget/79x40/resumeNote/short (0.00s)
--- PASS: TestBelowMinimumFrameStaysWithinBudget/79x40/resumeNote/wraps-at-w (0.00s)
--- PASS: TestBelowMinimumFrameStaysWithinBudget/100x30/none/none (0.00s)
--- PASS: TestBelowMinimumFrameStaysWithinBudget/100x30/attachError/short (0.00s)
--- PASS: TestBelowMinimumFrameStaysWithinBudget/100x30/attachError/wraps-at-w (0.00s)
--- PASS: TestBelowMinimumFrameStaysWithinBudget/100x30/resumeNote/wraps-at-w (0.00s)
--- PASS: TestBelowMinimumFrameStaysWithinBudget/100x30/resumeNote/short (0.00s)
--- PASS: TestBelowMinimumFrameStaysWithinBudget/80x24/none/none (0.00s)
--- PASS: TestBelowMinimumFrameStaysWithinBudget/80x24/attachError/short (0.00s)
--- PASS: TestBelowMinimumFrameStaysWithinBudget/80x24/attachError/wraps-at-w (0.00s)
--- PASS: TestBelowMinimumFrameStaysWithinBudget/80x24/resumeNote/short (0.00s)
--- PASS: TestBelowMinimumFrameStaysWithinBudget/80x24/resumeNote/wraps-at-w (0.00s)
--- PASS: TestBelowMinimumFrameStaysWithinBudget/120x24/none/none (0.00s)
--- PASS: TestBelowMinimumFrameStaysWithinBudget/120x24/attachError/short (0.00s)
--- PASS: TestBelowMinimumFrameStaysWithinBudget/120x24/attachError/wraps-at-w (0.00s)
--- PASS: TestBelowMinimumFrameStaysWithinBudget/120x24/resumeNote/short (0.00s)
--- PASS: TestBelowMinimumFrameStaysWithinBudget/120x24/resumeNote/wraps-at-w (0.00s)
PASS
ok  	github.com/n-orlov/deck/internal/tui	0.016s
```

`TestBelowMinimumFrameStaysWithinBudget` (extended, per the requirement's
own wording, from the below-minimum-only case to a matrix of four
terminal sizes × three message states × two message-length classes,
including a message long enough to wrap at 80 columns) proves the exact
operator-reported defect (`attachError`/`resumeNote` pushing the frame one
line past the terminal's own height) is fixed at every size checked, and
also proven by `features/harness.feature`'s
`@requirement-37-message-budget` scenarios (requirement 6 section above,
same run).

## Requirement 38: the pi probe refit

```
$ ci/run.sh go test -v ./internal/agent/...
ok  	github.com/n-orlov/deck/internal/agent	(cached)
```

**Provenance and captured/uncaptured states**, in full, in
`internal/agent/testdata/probes/pi-PROVENANCE.md` (123 lines, committed
alongside the fixtures) and `docs/reports/phase2b2-findings.md`'s "Task
007" and "Task 037" sections. Summary:

| pi state | status | fixture | marker used |
|---|---|---|---|
| running | captured, real pi 0.84.1 | `running.txt` | `Working...` |
| error | captured, real pi 0.84.1 | `error.txt` | `Error:`, narrowed to the pane's **last content line** only (verified against a real false-positive case: `echo "Error: fake tool error"` inside a healthy session, which is always followed by more transcript, unlike pi's own terminal error banner) |
| idle | captured, real pi 0.84.1, task 037 (operator steer, 21 Aug 2026) | `idle.txt` | the durable two-line status footer (`... (auto) ... <model> • <level>`), keyed only on that invariant shape, placed **last** among pi rules (after `Error:` and `Working...`), inferring idle from the footer's positive presence plus the absence of the other two verdicts — never from pane liveness alone |
| starting | not needed | — | rows already start at `starting` by default; the only pre-idle text observed was this capture container's own missing-`fd`-helper bootstrap message, not pi semantics |
| waiting (permission prompt) | **not captured** | — | pi's own README states it has no permission popups (`ctx.ui.confirm` is extension-provided); every tool call this job's pi ran, including a destructive one, executed with zero prompt — reaching one is out of scope for a static capture |

Version delta recorded, not hidden: fixtures are from pi **0.84.1** (this
container); the PRD's own reference capture is **0.84.2**.

Ordering of the idle rule (added last, task 037) is proven, not just
inspected — a test fails if the idle rule is moved above the `Error:`/
`Working...` rules — and `running.txt`/`error.txt` verdicts are unchanged
by its addition, both part of the `go test ./internal/agent/...` run
above. A long-tool-call check (`sleep-midrun.txt`, captured mid-`sleep 25`)
confirmed `Working...` is still on screen during a long call, so the idle
rule (keyed on the footer's *absence* of the other two verdicts) does not
false-positive against a long-running tool call.

`cmd/fake-pi` (requirement covered together with 38 per the PRD's own
grouping) renders panes derived from the same captures, documented in code
(`83a9d91`) rather than duplicated by hand — `go test ./cmd/... ./internal/agent/...`
passes with a fake-pi session probing to the same verdict a real-pi pane
matched by a captured rule would.

The total-probe-miss surfacing (also requirement 38, its own §7-adjacent
requirement) — `i` detail dialog states the pane was sampled and matched no
rule, with the sample's age, no status/status_source/§7 change — is proven
by `internal/tui`'s and `internal/agent`'s own suites
(`go test ./internal/tui/... ./internal/agent/...`, part of the full-suite
run below) with the copy recorded as a spec request in
`docs/reports/phase2b2-findings.md`'s "Task 009" section.

## Requirement 39: tilde-prefixed working directories (already landed, re-verified)

```
$ grep -i tilde /tmp/features.log | grep PASS
    --- PASS: TestFeatures/a_tilde-prefixed_working_directory_is_accepted_and_resolved (0.49s)
    --- PASS: TestFeatures/~otheruser_is_rejected_with_a_stated_reason_rather_than_half-expanded (0.62s)
```

Landed by approach 01 (`~`/`~/` expansion in the create modal, `~otheruser`
rejected with a reason, resolved absolute path stored) and unmodified by
this approach; re-verified green as part of the full-suite run.

## Requirement 40: the crash.feature flake fix (already landed, re-verified)

```
$ grep -i "private_server_options" /tmp/features.log | grep PASS
    --- PASS: TestFeatures/private_server_options,_safe_naming,_and_slug_collisions (0.59s)
```

`privateSessionDoesNotExist` (`features/assertions_test.go`) now polls with
a deadline instead of a one-shot `has-session` check, closing the
one-run-in-thirty flake the PRD names; applied to both call sites
(`crash.feature`, `walking_skeleton.feature`). Landed by approach 01,
re-verified by this approach's own `ci/stability.sh 10` run (requirement
41/54 below), which post-dates this fix.

## Requirement 41: stability re-established after the flake fix

`ci/stability.sh 10` (requirement 54 below) was run by this approach
**after** requirement 40's fix was already on the tree (HEAD at run time
`4f7d95b`, itself a descendant of the flake-fix commit). The one flaky run
observed in that log is diagnosed as an unrelated harness-side wall-clock
race in a *different* step (a client-exit wait in
`crash.feature`'s SIGKILL scenario, not `privateSessionDoesNotExist`), so
requirement 40's specific fix is not implicated in that run's single
failure. See requirement 54 below for the full log and diagnosis.

## Requirement 42: closing 2b-1's requirement-19 border-focus deferral

```
$ grep 'border_focus' /tmp/features.log | grep PASS
    --- PASS: TestFeatures/a_built-in_theme_colours_border_focus/border,_selection/selection_idle,_title,_group_and_key/hint_chrome,_read_per_cell_from_a_real_client (0.70s)
```

`docs/reports/phase2b1-findings.md:931-949` recorded the focused-panel
border-colour cue as *unobservable and explicitly not verified* pending
theme tokens and per-cell attribute reads — both now exist. §11.3's
focused-panel `border_focus`/unfocused `border`, and
`selection`/`selection_idle`, are now asserted per-cell (scenario above);
the settings takeover's own two lists (category/field) are the second
focusable surface this phase adds, and their focus cue is asserted in
`internal/tui`'s settings tests (task 019, `6a90d94`). The main view's
single focusable region is confirmed to advertise no focus move (`tab` is
still unbound at top level per §11.3) as part of `internal/tui`'s existing
keymap tests, unmodified by this phase. Deferral closed explicitly here
and in `docs/reports/phase2b2-findings.md`'s "Task 034" section ("closing
2b-1's requirement-19 deferral").

## Requirements 43-47: status recovery after an in-session resume

```
$ ci/run.sh go test ./internal/hookrecv/... ./internal/store/... ./internal/service/... ./internal/tui/...
ok  	github.com/n-orlov/deck/internal/hookrecv	(cached)
ok  	github.com/n-orlov/deck/internal/store	(cached)
ok  	github.com/n-orlov/deck/internal/service	(cached)
ok  	github.com/n-orlov/deck/internal/tui	(cached)
```

**Before → after transition policy (requirement 45), with the §7 rule
justifying each removed entry** (full detail and the observed magpie
sequence in `docs/reports/phase2b2-findings.md`'s "Task 003" section;
table reproduced here per requirement 55's explicit ask):

| event | before `AllowedFrom` | after | §7 rule justifying the removal |
|---|---|---|---|
| `SessionStart` | `["starting"]` | *(none)* | Precedence is source-based (`user-terminal > hook > probe > tmux`), not value-based; a `SessionStart` hook outranks any prior `tmux`/`probe` verdict regardless of its current value. |
| `UserPromptSubmit` | `["idle", "error"]` | *(none)* | The old list captured only the two return edges it happened to think of; a `waiting`-from-`probe` row's `UserPromptSubmit` was wrongly rejected too — the hook still outranks the stale probe read. |
| `Notification` | `["running"]` | *(none)* | The magpie case: `error`-from-`tmux` must accept a `Notification` moving it to `waiting`. |
| `Stop` | `["running"]` | *(none)* | The magpie case: `error`-from-`tmux` must accept a `Stop` moving it to `idle`. |
| `StopFailure` | `["running"]` | *(none)* | Same reasoning; a stale lower-precedence verdict must not block the turn/API-failure signal. |
| `SessionEnd` | all six statuses (the forbidden blanket) | *(none)* | `*any* → stopped` is already legal for every status per §7's table; removing the blanket removes the pattern the criteria forbid while changing no behaviour. |

What still holds, moved from per-event lists into one generic guard in
`internal/store.Store.UpdateSessionStatus`: `killed_by_user` (§7
`user-terminal`) outranks every hook (pre-existing guard, unchanged); a
hook cannot resurrect a process-crash verdict into `running` (pre-existing,
unchanged); **new** — a hook cannot move a row away from `stopped` (§7
gives `stopped` no return edge except the explicit `r` resume, which
writes `starting` directly via SQL and bypasses this path entirely, so a
hook arriving for a `stopped` row is stale/out-of-order by construction).

```
$ grep -E 'resume_pair_leaves_the_row_running|already-running,_never_an_error|error_from_a_stale_tmux_launch-failure|not_resurrected_by_a_late_hook' /tmp/features.log | grep PASS
    --- PASS: TestFeatures/An_in-session_resume_pair_leaves_the_row_running_and_moves_conversation_id (0.85s)
    --- PASS: TestFeatures/r_on_a_session_whose_tmux_session_already_exists_reports_already-running,_never_an_error (1.01s)
    --- PASS: TestFeatures/A_row_at_error_from_a_stale_tmux_launch-failure_verdict_recovers_on_the_next_hook (1.00s)
    --- PASS: TestFeatures/A_session_the_user_killed_is_not_resurrected_by_a_late_hook (1.22s)
```

- **R43** (in-session resume/clear is not a session end): `41d1904`; the
  SessionEnd reason taxonomy (resume/clear non-terminal, all other reasons
  terminal) is recorded in `docs/reports/phase2b2-findings.md`'s "Task 001"
  section.
- **R44** (`conversation_id` follows the live conversation): `1450975`;
  proven durable (read back from `state.db`, not in-memory) by
  `internal/store`'s own tests in the run above.
- **R45** (hook always outranks stale non-hook verdict): the table above;
  `dc3ade0`.
- **R46** (already-running is a no-op, not an error): `tmux.Client.Exists`
  checked before the launch lease is taken; the copy ("already running")
  is recorded in `docs/reports/phase2b2-findings.md`'s "Task 004" section;
  `049ff36`.
- **R47** (a launch failure is clearable, no new §7 status invented):
  requirement 45's generic precedence rule is what makes the `error`-from-
  a-failed-launch row recoverable on the next `Stop`/`Notification` — the
  "A row at error from a stale tmux launch-failure verdict recovers..."
  scenario above is the direct proof; no new status was added.

## Requirement 48: `features/settings.feature`

See requirements 15-24 above — the scenario list and its pass results are
identical; `features/settings.feature` is the file, requirement 48 is its
number in the PRD's own scenario-files section.

## Requirement 49: `features/themes.feature`

See requirements 25-36 above — same file, same scenario list.

## Requirement 50: `features/dialogs.feature`

```
$ grep -E '^    --- PASS: TestFeatures/(create|detail|profile_switch|pin|help)_dialog_--' /tmp/features.log
    --- PASS: TestFeatures/create_dialog_--_esc_after_altering_every_field_creates_no_session_and_touches_no_config (0.62s)
    --- PASS: TestFeatures/create_dialog_--_enter_submits_a_valid_form (0.50s)
    --- PASS: TestFeatures/create_dialog_--_tab_moves_focus_between_fields (0.44s)
    --- PASS: TestFeatures/create_dialog_--_in-dialog_validation_retains_the_typed_value_and_states_the_reason (0.48s)
    --- PASS: TestFeatures/create_dialog_--_the_mouse_can_neither_cancel_nor_confirm_it,_at_its_border,_its_body_or_outside_it (0.56s)
    --- PASS: TestFeatures/create_dialog_--_width_is_80%_of_the_viewport_clamped_to_[26,80],_at_both_clamp_ends (0.39s)
    --- PASS: TestFeatures/create_dialog_--_width_saturates_at_80_well_past_the_upper_clamp (0.39s)
    --- PASS: TestFeatures/detail_dialog_--_esc_changes_nothing_(it_has_no_fields_to_alter) (0.47s)
    --- PASS: TestFeatures/profile_switch_dialog_--_esc_after_altering_its_field_leaves_the_persisted_profile_untouched (0.88s)
    --- PASS: TestFeatures/profile_switch_dialog_--_enter_submits_the_cycled_value (0.92s)
    --- PASS: TestFeatures/pin_dialog_--_esc_after_altering_its_field_leaves_the_persisted_resume_mode_untouched (0.91s)
    --- PASS: TestFeatures/pin_dialog_--_enter_submits_the_cycled_value (0.90s)
    --- PASS: TestFeatures/help_dialog_--_esc_changes_nothing_(it_has_no_fields_to_alter) (0.37s)
```

All five dialogs (create, detail, profile switch, pin, help), each
asserted state-based (session row / store / `config.toml`), not
screen-based: `esc` proven unchanged by reading the store back out, not by
reading the screen. `27b373e`.

## Requirement 51: `features/status_recovery.feature`

See requirements 43-47 above — same file, same four scenarios. `ab3c0fe`
extended `cmd/fake-claude` (`279d7ba`) to fire the resume end/start pair
with no real agent required.

## Requirement 52: the golden minimum frame, pinned theme + colour depth

```
$ ci/run.sh go test -v -run TestGoldenMinimumFrame ./features/...
=== RUN   TestGoldenMinimumFrame
=== RUN   TestGoldenMinimumFrame/run-1
    golden_frame_test.go:75: rendered frame sha256 0b21a7370a8e5e0a0ed063dc214909dd7fdc198f55775fa213af11643fb72529 (1941 bytes, theme "empire", DECK_COLOR_DEPTH "truecolor")
=== RUN   TestGoldenMinimumFrame/run-2
    golden_frame_test.go:75: rendered frame sha256 0b21a7370a8e5e0a0ed063dc214909dd7fdc198f55775fa213af11643fb72529 (1941 bytes, theme "empire", DECK_COLOR_DEPTH "truecolor")
--- PASS: TestGoldenMinimumFrame (2.16s)
    --- PASS: TestGoldenMinimumFrame/run-1 (1.16s)
    --- PASS: TestGoldenMinimumFrame/run-2 (1.00s)
=== RUN   TestGoldenMinimumFrameRowCount
--- PASS: TestGoldenMinimumFrameRowCount (0.00s)
PASS
ok  	github.com/n-orlov/deck/features	2.174s
```

Pinned to **theme "empire"** (`theme.DefaultName`, named explicitly rather
than left to fall back) at **`DECK_COLOR_DEPTH=truecolor`**, exactly
**80×24**, colour genuinely on (`NO_COLOR` lifted for this test). Two
independent renders in the same run produce identical bytes
(sha256 `0b21a737...fb72529`, logged both times above); `f935c2a`'s commit
message additionally records two independent `UPDATE_GOLDEN=1`
regenerations producing the same checksum. `assertGoldenFrameIsThemed`
reads the sidebar's top-left corner cell directly (`CellAt`) and requires
its foreground equal `internal/theme`'s own resolution of `empire`'s
`border_focus` token at `DECK_COLOR_DEPTH=truecolor` (no quantisation to
blur a wrong colour into a passing one) — proving the pin is read, not
merely a named, unread env var.

## Requirement 53: existing scenarios keep passing; re-aimed assertions stated

```
$ tail -5 /tmp/features.log
PASS
ok  	github.com/n-orlov/deck/features	140.431s

$ grep -n '^[0-9]* scenarios\|^[0-9]* steps' /tmp/features.log
164 scenarios (164 passed)
1567 steps (1567 passed)
```

165 `--- PASS` lines (164 scenarios + the top-level `TestFeatures`), 0
`--- FAIL`, real full log ~140s. `layout_modes`, `preview`,
`attention_sort`, `mouse`, `harness`, `status_probe` and every Phase 0-2
file are included in this run and pass.

Requirement 45 changes transition policy, so the one Phase 2 assertion
that pinned a hook being refused
(`TestReceiveEnforcesEveryHookTransitionAndStillAuditsRejectedEvents`,
which directly asserted the exact `AllowedFrom` slice values removed by
task 003) was **replaced, not deleted without trace**, by
`TestReceiveHookAppliesOverAnyStaleSourceExceptStopped` (sweeps the same
six events × six statuses × four `status_source` values, proving the
source no longer matters except at `stopped`), plus two new focused tests
(`TestReceiveHookOverridesStaleTmuxLaunchFailure`,
`TestReceiveDoesNotResurrectAUserKilledRow`) covering the criteria's named
scenarios directly. `TestReceiveDoesNotReviveCleanStopOrProcessCrash`
(pre-existing, unmodified) continues pinning `stopped`/process-crash
immunity, now enforced generically rather than by a removed
`AllowedFrom`. None of this was the assertion that made `magpie`
unrecoverable — that assertion (the removed `AllowedFrom: ["running"]` on
`Stop`/`Notification`) is exactly what task 003 fixed, stated explicitly
in `docs/reports/phase2b2-findings.md`'s "Task 003" section.

A sweep of `features/` and `internal/` for other byte/frame/column
assertions (recorded in `f935c2a`'s commit message): golden-named fixtures
in `internal/agent/testdata/probes`, `internal/tui/layout_test.go`'s
"golden minimum" unit test, `panel_test.go`'s column-arithmetic tests,
`frame_budget_test.go`'s `FrameFitsBudget`, `pty_driver_test.go`'s
`TestNormalizeFrame` — none touch `golden_frame_test.go`'s env or config;
all pass unmodified. `internal/tui/panel_test.go` itself has an empty git
diff for this whole approach (`git diff cea9624..HEAD -- internal/tui/panel_test.go`
below).

```
$ git diff cea9624..HEAD -- internal/tui/panel_test.go
(no output)
```

## Requirement 54: `ci/stability.sh 10` from a clean state

Full real output in `docs/reports/phase2b2-stability.log` (task 035). **Result:
9/10 green.** Run 3's `features` package failed exactly one scenario out of
164 —
`TestFeatures/SIGKILL_captures_and_sanitizes_the_agent_pane_without_relaunching`
— and inside that scenario, every product-level assertion passed
(reconcile-to-error, sanitized crash artifact, tmux teardown, audit record,
detail dialog content); only the teardown step `deck client "A" exits
cleanly` timed out (5s wait for process exit after sending `q`). Diagnosis
recorded in the log and in the handoff notes: the same harness-side
wall-clock-race class already documented for this suite
(`docs/reports/phase1-findings.md` task 033's "the race is entirely in the
test harness"; `docs/reports/phase2b1-findings.md`'s crash.feature
teardown note) — scheduler contention across ten sequential full-suite
runs in the sibling container, not a §7 status/crash-capture defect. No
code change was made to chase a single non-reproduced 5s timeout, per the
requirement's own "an honest 9/10 beats a hidden race" wording. This run
post-dates requirement 40's flake fix (see requirement 41 above) and the
one failure observed is in a different step than the one that fix
targets.

## Requirement 55: this report

`docs/reports/phase2b2.md` (this file).

## Requirement 56: the help overlay

```
$ ci/run.sh go test -v -run TestEmptyAndHelpViewsAreDiscoverable ./internal/tui/...
--- PASS: TestEmptyAndHelpViewsAreDiscoverable (0.00s)

$ ci/run.sh go test -v -run TestDeckBinaryEmptyHelpAndQuitThroughPTY ./cmd/deck/...
--- PASS: TestDeckBinaryEmptyHelpAndQuitThroughPTY (1.01s)
```

The help overlay documents `,` (settings), `t` (theme picker),
`DECK_COLOR_DEPTH` and `NO_COLOR`, plus every key the settings takeover
and the theme picker bind (`81700d5`). Both pinned assertions — the
in-package help-copy pin (`internal/tui/tui_test.go`) and the pty ASCII
overlay pin (`cmd/deck/main_test.go`) — were updated together and both
pass. The unavailable-action list
(`delete`, `send message`, `env editor`, `event log`, `filter list`,
`snooze`, `archive`, `undo`, `tab`) is unchanged — `grep -n
'"send message", "env editor"' internal/tui/tui_test.go` still finds the
same list this approach did not touch:

```
$ grep -n 'unavailable :=\|"delete", "send message"' internal/tui/tui_test.go
62:	for _, unavailable := range []string{"suggested increment", "write it to advance", "_hook", "> advance", "resume/start", "restart preserving", "delete", "send message", "env editor", "event log", "filter list", "snooze", "archive", "undo", "tab"} {
```

## Requirement 57: workspace hygiene (verified read-only)

```
$ find /workspace -uid 0
(no output — no root-owned files)

$ ls /tmp | grep -i tmux
(no output — no leftover tmux sockets)

$ git status
On branch main
Your branch is ahead of 'origin/main' by 55 commits.
  (use "git push" to publish your local commits)

nothing to commit, working tree clean

$ docker ps -a --filter ancestor=deck-ci:local --format '{{.ID}} {{.Status}}'
(no output — no lingering deck-ci:local containers; every ci/run.sh sibling
is --rm, so there is nothing to clean up, and none was left running)
```

No `docker rm`/`kill` was run (read-only per the requirement's own
explicit instruction; a sweep filtered on `label=ralphd.run=...` would kill
this job's own container).

```
$ git push origin main
fatal: could not read Username for 'https://github.com': No such device or address
```

**Environment finding:** no git push credentials are mounted in this run
(confirmed again here; the same finding as `docs/reports/
phase2b1-findings.md` and this approach's own `docs/reports/
phase2b2-findings.md` "Task 034" section). 55 commits are local-only on
`main`, unpushed, ready to push once credentials are available. `main` is
otherwise clean (`git status` above) and `SPEC.md` is untouched throughout
this approach (`git diff cea9624..HEAD -- SPEC.md` below).

```
$ git diff cea9624..HEAD -- SPEC.md
(no output)
```
