# Phase 4b — manual session groups + interactive scroll cue (task 024)

Per-requirement record for the whole phase (`prds/phase4b-manual-groups-and-scroll-cue.md`,
GH #25/#30), written on the freeze-line tail after the whole-suite gate (task 021),
the guards (task 022) and the ten-run stability sweep (task 023) had run.

- **Code sha this file is written against:** `bcd80b923670f0de3f0859bed8dd2e7dacb3f468`
  (`bcd80b9`, "docs: run the ten-run stability sweep at baf92ed …" — the tip of
  `main` at the moment this revision was authored, tree clean, `HEAD ==
  origin/main`). This file's own commit sits directly on top of it and is
  record-only.
- **Task 021's gate sha (the whole-suite sweep's launch sha, and the sha every
  requirement below is scored against):** `baf92eda072f6b59ba9b788340f71ed798077504`
  (`baf92ed`, "docs: re-verify task 019's group-delete branches at HEAD (R131
  part 2, #30)"). `baf92ed` is itself a docs commit; the last code-touching
  commit under it is `d179b2c` ("tmux: wait out a slow pipe-pane job instead of
  timing out on it"), so those two carry identical Go sources. Every commit from
  `8d28f01` (task 021's record) through this file's own commit is record-only
  (`docs/reports/`, `docs/DELIVERY-LOG.md`, `docs/roadmap.md`,
  `docs/prds/README.md`): `git diff --stat baf92ed..HEAD -- '*.go' '*.feature'`
  prints nothing, so the code tree at `bcd80b9` — and at this file's own commit —
  is byte-identical to `baf92ed`'s, and the requirement scoring below holds at
  all of them.
- **Docs-only tail:** this file, `docs/reports/phase4b-findings.md` (task 025) and
  this phase's row in `docs/DELIVERY-LOG.md` (task 026) are the docs-only tail of
  the phase — no code lands after the freeze line, which began at task 021's
  launch.
- **What this revision supersedes.** The first two landings of this file
  (`b96015e`, corrected in `6c2b95c`) scored the phase at the earlier gate sha
  `0806ba6` and quoted a *green* gate (exit 0, 440s) and a 10/10 stability run.
  Both of those sweeps were re-run after the cure wave (`60a551d`…`57e1a6a`) and
  the task 016–020 re-verifications landed, and the numbers changed: the gate of
  record is now **RED at `baf92ed`** and stability is **9/10 at `baf92ed`**. The
  quotations in "Gate result and stability headline" below are taken from the
  current contents of the two report files named there, not carried forward from
  the superseded revision. Two commit attributions that the earlier revision got
  wrong are restated correctly here and listed under "Attribution corrections
  carried forward".

## Tier 1 → Tier 2 decision

Both tiers were started and shipped. The decision to proceed with Tier 2 —
**GO** — its budget snapshot and its rationale are recorded in full at
[`docs/reports/phase4b-tier2-decision.md`](./phase4b-tier2-decision.md) (task 006,
commit `16fc86a`). That record is the source of truth for the tier branch taken
and names task 020 as the last code-touching task on the GO branch; this file
does not restate its budget arithmetic.

## Gate result and stability headline

- **Whole-suite gate (task 021): RED, exit `1`,** 489s wall clock, at gate sha
  `baf92eda072f6b59ba9b788340f71ed798077504` — 18 package lines = 14 `ok`, 1
  `FAIL` (`features`, 413.717s) and 3 `[no test files]` (`internal/notify`,
  `internal/search`, `internal/unit`). Quoted from the source: "**Result at the
  launch sha: RED (exit 1).** One package, `features`, failed with two
  load-sensitive timeouts; every other package is green", and "**the gate's own
  exit status at `baf92ed` is 1**, and it stands as the result of record. A lane
  that is red on the gate IS red". Invocation, per the same source:
  `ci/run.sh go test -p=1 -count=1 ./...`, no `-run` filter and no package list.
  Source: [`docs/reports/phase4b-final-suite/README.md`](./phase4b-final-suite/README.md).
- **Ten-run stability (task 023): 9/10 passed,** same gate sha `baf92ed`, ~73
  minutes wall clock for the ten runs. Quoted from the source: "**9/10 passed.**
  … One run (run 2) hit `exit 1`. The other nine (runs 1, 3–10) exited 0."
  The one failure is the transient-`starting` flake class already open from the
  gate, named there as advisory with its log path (`run-02.log`); no instance of
  the second known-open class (the SIGWINCH exact-count assertion) occurred in
  any of the ten runs. Source:
  [`docs/reports/phase4b-stability10/README.md`](./phase4b-stability10/README.md).
- **Guards (task 022):** build, vet and `gofmt` over the touched set, plus the
  read-only path check, all exit `0` at the same sha — recorded at
  `docs/reports/phase4b-guards/README.md`.
- Both red lanes and how they are carried are restated under
  ["Not shipped / still open"](#not-shipped--still-open) at the end of this file.

## Tier 1 — the interactive scroll cue (GH #30)

### R133 — the scrolled-back view says so, from the offset actually used

**Shipped**, in three commits (two landings plus one cure):

- `8bd8a91` (task 001) — `interactiveBodyLines` (`internal/tui/interactive.go`)
  now consumes `RenderRows`' second return (the clamped, actually-used offset)
  and heals `m.interactiveScrollOffset` to it instead of discarding it as at
  launch sha `c3b530a`; `interactive_scroll.go`'s clamp bounds are untouched, per
  the standing rules. Tests (`internal/tui/interactive_scroll_heal_test.go`):
  `TestInteractiveBodyLinesHealsTheStoredOffsetToTheClampedUsedOffset` and
  `TestInteractiveBodyLinesHealingReenablesTheNotRepaintedNotice`, both recorded
  as confirmed red against the pre-fix discard.
- `e44b17f` (task 002) — the interactive footer renders the "how far back / not
  live" cue from the healed offset (`interactiveScrollCue`, glyph resolved
  through `m.glyph` so `[ui] ascii` falls back to ASCII;
  `interactiveAtTopOfScrollback` gives top-of-scrollback its own wording). Tests
  (`internal/tui/interactive_footer_scroll_cue_test.go`):
  `TestInteractiveFooterCueReportsTheClampedScrolledBackPosition`,
  `TestInteractiveFooterCueTopOfScrollbackWordingDiffersFromOrdinary`, and
  `TestInteractiveFooterAtLiveBottomRendersNoCueAndMatchesPreChangeFooter`
  (offset 0 renders no cue and a byte-identical footer).
- `60a551d` (cure-01-01) — the heal above lands on a value-receiver copy built
  inside `View()` and discarded when the render returns, so the model bubbletea
  keeps between `Update` calls never observed it. `scrollInteractiveByLines`
  (`internal/tui/interactive_scroll.go`) — the sole `Update`-time entry point the
  page keys and the mouse wheel both funnel through — now runs the same
  `RenderRows` clamp before returning and stores its result, guarded on
  `interactiveGrid.Grid() != nil`. Tests
  (`internal/tui/interactive_scroll_persist_test.go`):
  `TestStoredScrollOffsetHealedAcrossViewAndNextScroll` and
  `TestInteractiveDispatcherNilDoesNotSnapStoredOffset`.

Evidence: `internal/tui` green in Tier 1's targeted record
(`docs/reports/phase4b-tier1-suite/internal-tui.log`, task 005), re-taken after
the cure at `docs/reports/phase4b-retake-01-01/` (commit `d99dc5c`), and green
again in the gate's `internal/tui 6.408s` line at `baf92ed`.

### R134 — the interactive footer advertises the scroll keys

**Shipped** in `c1dd5f1` (task 003). `interactiveFooterLine` names
Shift+PgUp/PgDn (already bound by interactive mode) while scrolled back and
degrades three optional segments — the R133 cue, the new scroll-key
advertisement, and the pre-existing forward-note reminder — from least to most
essential, width-budgeted with `stringWidth` the way `footerLegendWithin`
degrades the list-mode legend (the added code *mirrors* that function, named in
its own comment, and calls neither it nor `elideToWidth`), with `Ctrl+Q` never
among the droppable segments. The commit touches exactly two files
(`git diff-tree --no-commit-id --name-only -r c1dd5f1`):
`internal/tui/tui.go` and `internal/tui/interactive_footer_scroll_advertisement_test.go`.
Help (`helpText`'s Keys/Mouse sections) and the mouse table already named
Shift+PgUp/PgDn before this task and needed **no** wording change, so the pinned
substrings in `cmd/deck/main_test.go` and `internal/tui/tui_test.go` were left
untouched — the commit message records exactly that, and neither file is in the
commit. Tests (`internal/tui/interactive_footer_scroll_advertisement_test.go`):
`TestInteractiveFooterNamesTheScrollKeysWhenRoomAllows`,
`TestInteractiveFooterAtTheLiveBottomNeverNamesTheScrollKeys`,
`TestInteractiveFooterDegradesWithoutOverflowingTheFrame` (drives widths
200/100/80/60/20, asserting the frame is never exceeded and `Ctrl+Q` survives
every degradation step), and
`TestInteractiveFooterScrollKeyWordingAgreesWithHelpAndMouseTables`, which
asserts the footer agrees with the *existing* help/mouse wording rather than
changing either. Golden 80×24 frame test (`features/golden_frame_test.go`) green
— item 6 of `docs/reports/phase4b-tier1-suite/README.md`.

### R135 — a keystroke that forwards nothing must not snap the view to the live bottom

**Shipped** in `6fcb7bd` (task 004). `updateInteractive`'s
`m.interactiveScrollOffset = 0` reset now runs only after the dispatcher-nil
guard *and* after `interactiveNamedKey`/`interactiveLiteralPayload` agrees to
forward the key, inside each of their own branches; `Ctrl+Q`'s earlier return is
untouched. Tests (`internal/tui/interactive_forward_gate_test.go`):
`TestUpdateInteractiveLeavesTheScrollOffsetUntouchedForAKeyNeitherHelperForwards`
(Alt+Insert into a scrolled-back, real-tmux-backed model — confirmed failing
against the pre-fix code) and
`TestUpdateInteractiveResetsTheScrollOffsetForAForwardedKey`.

**Tier 1 targeted evidence** (all six items — build, vet, `internal/tui`,
`cmd/deck`, three interactive `.feature` files, golden frame, all exit `0`) is
recorded at
[`docs/reports/phase4b-tier1-suite/README.md`](./phase4b-tier1-suite/README.md)
(task 005, commit `ae81fee`, Tier 1 code sha `6fcb7bd`).

## Tier 2 — manual session groups (GH #25)

### R128 — the group model replaces the workspace label

**Shipped**, across seven commits (four landings, one preparatory seam, two cures):

- `6104aec` (task 007, preparation) — the old workspace reads routed through one
  group-key seam in `internal/tui/group.go`, behaviour unchanged, so the model
  swap below had a single site to re-aim.
- `bf1c085` (task 008, part 1) — `schemaV7` (SPEC §4's `groups` table +
  `sessions.group_id`, no foreign key by design), the one-transaction migration,
  and the removal of `sessions.workspace` / `store.DefaultWorkspace` /
  `Session.Workspace` / `Session.WorkspaceColumn`, all in one commit per the
  standing rules' irreversible-migration discipline (existing rows migrate to a
  NULL `group_id`, which reads and renders as `default`; the group list starts
  empty). This same commit also carries the environment rename
  `DECK_SESSION_WORKSPACE` → `DECK_SESSION_GROUP` (holding `GroupName` verbatim,
  the old name never aliased) in `internal/service/session_context.go` — its own
  message states it. Tests:
  `internal/store/schema_v7_migration_test.go`
  (`TestSchemaV7MigratesV6LegacyColumnRowsToNullGroupID` over a fixture DB built
  by the previous schema with several distinct legacy values, plus
  `TestSchemaV7MigrationIsAtomicOnMidMigrationFailure`), and
  `TestSessionContextEnvExportsGroupNotLegacyField` in
  `internal/service/session_context_test.go`.
- `9eefaa2` (also task 008) — the notification-payload half `bf1c085` left out:
  `internal/service/session_context.go` gains exported `NotificationSession`
  (json tags = SPEC §10.1's eight session field names, including `group`) and
  `NotificationSessionPayload`, projecting one `store.Session` row onto them with
  `group` reading `store.Session.GroupName` verbatim. The commit touches exactly
  `internal/service/session_context.go` and
  `internal/service/session_context_test.go`; it adds no schema and no migration,
  and it does **not** rename the environment variable — that landed in `bf1c085`,
  above. Test: `TestNotificationSessionPayloadCarriesGroupNotLegacyField`,
  asserting the rendered JSON (not the Go field) carries `group`, never the legacy
  name, in exactly those eight fields.
- `60075ec` + `5adeb83` (task 009, part 2) — group CRUD in
  `internal/store/group.go` with SPEC's name rules: trimmed, non-empty, no
  control characters anywhere, case-insensitive uniqueness (`COLLATE NOCASE`),
  `default` reserved in any case, 32-char cap. Tests
  (`internal/store/group_crud_test.go`):
  `TestCreateGroupRejectsReservedDefaultName`,
  `TestCreateGroupRejectsCaseInsensitiveDuplicate`,
  `TestCreateGroupRejects33CharacterName`,
  `TestCreateGroupAccepts32CharacterName`,
  `TestGroupNameRejectsControlCharactersAnywhere`.
- `f171168` (task 014) — the guard closing R128/R129: no legacy workspace
  identifier survives in `internal/store`, `internal/tui` or `internal/service`.
  Test: `TestNoLegacyGroupModelIdentifierSurvivesInStoreTUIOrService` in
  `internal/tui/registry_guard_test.go` (the black-box idiom the PRD names).
- `386649d` (cure-01-03) — `CreateSession` built its returned `Session` from its
  input instead of resolving `GroupID` against the `groups` table, so `GroupName`
  — and therefore `DECK_SESSION_GROUP` on every create-path launch — read back
  empty whichever group the caller named. It now resolves `GroupName` inside the
  insert's own transaction, empty only for a nil `GroupID` or one naming no live
  group row (dangling membership renders under `default` rather than vanishing,
  SPEC §11). Tests: `internal/store/store_test.go` plus
  `TestSessionContextEnvExportsNamedGroupThroughRealPaneLaunch` and
  `TestSessionContextEnvGroupCreateResumeParityAndDanglingMembership`
  (`internal/service/session_context_test.go`); re-taken at
  `docs/reports/phase4b-retake-01-03/` (commit `ce80b11`).
- `57e1a6a` (cure-01-06) — `groups.id` becomes `INTEGER PRIMARY KEY
  AUTOINCREMENT`, so SQLite never reissues a deleted group's id onto a new row
  and nothing keyed on it (`sessions.group_id`, `ui_state`'s `last_create_group`,
  a tombstoned member's own `group_id`) can silently rebind onto an unrelated
  group instead of degrading to `default`. Tests:
  `TestDeleteGroupThenCreateNewGroupNeverReusesTheDeletedID`
  (`internal/store/group_crud_test.go`),
  `TestRememberedCreateGroupDoesNotRebindAcrossRestartAfterDeleteAndReplacement`
  (`internal/tui/create_last_used_group_test.go`) and
  `TestSettingsGroupDeleteDBranchUndoDoesNotRebindOntoAGroupCreatedMeanwhile`
  (`internal/tui/settings_groups_test.go`); re-taken at
  `docs/reports/phase4b-retake-01-06*/` (commits `bc25f34`, `4266a13`,
  `3edc64d`).

### R129 — the sidebar renders manual groups

**Shipped**, across eight commits (six landings plus two cure landings):

- `35806e8` + `3e3e9f7` (task 011, part 1) — alphabetical, case-insensitive group
  order with `default` always last; the header budgeted against the panel's real
  text width. Tests (`internal/tui/group_order_test.go`):
  `TestGroupOrderAaaLeadsAlphabetically`,
  `TestGroupOrderZzzSortsBeforeDefaultDespiteName`,
  `TestGroupOrderDefaultAlwaysLastRegardlessOfEmptyStringSortingFirst`,
  `TestGroupHeaderTextElidesNameAtSidebarFloorKeepingChevronAndCount`.
- `89125f2` (task 012, part 2) — `[ui] group_by_workspace` and
  `DECK_GROUP_BY_WORKSPACE` deleted across `internal/config`
  (`config.go`/`schema.go`/`toml.go`/`toml_write.go`), flat (header-less) sidebar
  mode deleted, and `reorderPreservingGrouping`'s attention-follow behaviour
  deleted. Tests: `internal/config/toml_write_test.go`'s
  `TestStaleGroupByWorkspaceLineIsSkippedAndPreservedByteIdentical` and
  `TestWriteConfigFileEnvDropsRemovedKeyAddsNewKey`;
  `internal/tui/flat_sidebar_test.go` and
  `internal/tui/settings_group_by_staging_test.go` were **deleted with the
  behaviour** rather than inverted, per the standing rules (neither path exists
  in the tree at this sha).
- `3044e9b` (task 010) + `8e9de6e` (task 013, part 3) — sidebar collapse and
  mouse hit-test keyed by group id, with `collapsed_groups` and the last
  created-into group persisted in `ui_state`, surviving a Model rebuild. Tests:
  `TestNavigationPrimitivesRespectGroupIDKeyedCollapse` and
  `TestCollapsedGroupsSurviveModelRebuildFromUIState`
  (`internal/tui/group_id_navigation_test.go`),
  `TestCollapsedGroupsRoundTripsAcrossReopen` and
  `TestLastCreateGroupDanglingIDDegradesToDefaultNotEmptyValue`
  (`internal/store/ui_state_groups_test.go`).
- `f171168` (task 014) — see R128 above; the same commit closes both
  requirements' "no legacy workspace identifier" clause.
- `8773100` (task 015, part 4) — `/` matches name, group and `cwd`, with
  per-group matching counts under an active filter. Tests
  (`internal/tui/filter_test.go`): `TestFilterByGroupShowsOnlyTheMatchingRow`,
  `TestFilterByGroupNameSurfacesEveryMemberOfThatGroup`,
  `TestFilterHidesTheHeaderOfAGroupWithNoMatch`.
- `f77368f` + `db0f94b` (cure-01-02) — `groupSessions()` only ever bucketed
  loaded sessions, so a group the user had defined but not filled left no trace
  in the sidebar and the structural `default` could disappear entirely.
  `sessionsLoaded`/`loadSessions` now fetch `store.ListGroups()` on the same
  reload as `ListSessions()` into `Model.allGroups`; `groupSessions()` seeds a
  zero-member bucket for every cached group plus `default` while unfiltered only
  (an empty group can never match a query, SPEC §11), and `sidebarEntries`'
  zero-sessions short-circuit now fires only for an active filter with no
  matches, so an empty store still renders the `default` header with `(0)` beside
  the empty-state copy. Tests (`internal/tui/empty_groups_render_test.go`):
  `TestUnfilteredSidebarAlwaysRendersDefaultWithZeroSessionsAndZeroGroups`,
  `TestEmptyGroupsRenderFromDBWithZeroSessions`,
  `TestEmptyGroupsRenderFromDBWithUnrelatedPopulatedGroup`,
  `TestEmptyGroupsHiddenUnderAnActiveFilter`,
  `TestEmptyGroupCollapsesByItsDurableID`; plus
  `TestGroupHeaderTextCountsPopulatedAndEmptyGroups`
  (`internal/tui/group_order_test.go`), re-taken at
  `docs/reports/phase4b-retake-01-02/` (commit `8dd524d`).

`e67dd63` (features-package repair after the workspace drop and the group-order
change) keeps the `features` suite in step with this requirement's landing.

### R130 — membership is set where the session is

**Shipped**, across four commits (two landings, two cures) plus `57e1a6a`:

- `ddeab8d` + `338a600` (task 016, part 1) — the create modal gains a `Group`
  field cycling the available groups alphabetically with `default` last,
  defaulting to the last group created into (persisted in `ui_state` beside the
  last-used-agent key, same fallback-to-`default` precedent as
  `GetLastCreateAgent`; the cure made a create into `default` forget a remembered
  named group). Tests (`internal/tui/create_last_used_group_test.go`):
  `TestRememberedCreateGroupSurvivesRestart`,
  `TestCreateGroupDefaultDegradesWhenRememberedGroupDeleted`,
  `TestCreateIntoDefaultGroupForgetsTheRememberedNamedGroup`; plus
  `TestSetLastCreateGroupDefaultForgetsTheRememberedNamedGroup`
  (`internal/store/ui_state_groups_test.go`) and the create-into-a-group scenario
  in `features/create_session.feature` / `features/create_session_test.go`.
  Re-verified at HEAD by task 016 (`f80f771`,
  `docs/reports/phase4b-task016/`).
- `c754c43` (task 017, part 2) + cure `bf964b8` — the `i` detail dialog shows the
  session's group and a `g` picker moves it, resolved off the session row itself
  (the cure replaced a stale-snapshot read found after the first landing);
  `internal/service/group_move.go` carries the move. Tests
  (`internal/tui/group_move_test.go`):
  `TestMoveSessionGroupThroughPickerChangesOnlyThatOneRow`,
  `TestMoveSessionGroupToDefaultClearsGroupID`,
  `TestDetailDialogNamesTheSessionsGroupBeforeThePickerOpens`,
  `TestGDoesNotDisturbQCtrlCIOrRInsideDetail`,
  `TestDetailFooterAndHelpBothNameGForGroupMove`. Re-verified at HEAD by task 017
  (`0dac638`, `b8ccad4`, `docs/reports/phase4b-task017/`), whose validation run
  also surfaced the pipe-pane fifo flake cured in `d179b2c`.
- `57e1a6a` (cure-01-06) — see R128 above: a remembered create-group id can no
  longer be reissued to an unrelated group after a delete.

### R131 — the group list is edited in settings

**Shipped**, across six commits (three landings, three cures):

- `8038b4d` (task 018, part 1) — a settings Groups section where `n` creates
  (inline validation from R128's name rules), `r` renames, both applied
  immediately rather than staged, and `esc` never offers a discard prompt in that
  section. Tests (`internal/tui/settings_groups_test.go`):
  `TestSettingsGroupsNCreatesGroupImmediately`,
  `TestSettingsGroupsCreateShowsValidateGroupNameErrorInline`,
  `TestSettingsGroupsRenameLeavesMemberGroupIDsUntouched`,
  `TestSettingsGroupsCopyStatesEditsApplyImmediately`,
  `TestSettingsGroupsEscOffersNoDiscardPrompt`,
  `TestSettingsGroupsEscDuringCreateCancelsOnlyTheEditorNotTheTakeover`.
- `74263ab` (task 019, part 2) + cure `61b6f7d` — `d` deletes: an empty group
  goes with no prompt (`default` is never a row), a non-empty group opens the
  two-branch confirm (`m` → clear every member's `group_id` then drop the row;
  `d` → routes into the **existing** `dd` batch path via
  `m.marked`/`m.deleteConfirming` → `updateBulkDeleteConfirm`/`Service.Delete`,
  no second deletion implementation). The cure drops the group row only once the
  batch actually commits, so a partial failure keeps it. Tests
  (`internal/tui/settings_groups_test.go`):
  `TestSettingsGroupDeleteEmptyGroupHasNoPrompt`,
  `TestSettingsGroupDeleteNonEmptyOpensTwoBranchPrompt`,
  `TestSettingsGroupDeleteMBranchMovesMembersToDefaultAndDropsGroup`,
  `TestSettingsGroupDeleteDBranchReachesTheSameServiceCallDDDoes`,
  `TestSettingsGroupDeleteDBranchDropsTheGroupRowOnceTheBatchCommits`,
  `TestSettingsGroupDeleteDBranchKeepsTheGroupWhenAMemberFailsToDelete`,
  `TestOrdinaryBulkDeleteNeverDropsAGroupRow`,
  `TestSettingsGroupDeleteDBranchOneUndoRestoresTheWholeBatch`, plus the
  destructive scenario in `features/kill_delete_undo.feature`. Re-verified at
  HEAD by task 019 (`baf92ed`, `docs/reports/phase4b-task019/`); the row drop
  lives in `sessionsBulkDeleted` (`internal/tui/tui.go`) and fires only when the
  batch reported no error.
- `42c1ffc` (cure-01-04) — the delete's empty/non-empty decision, the `m`
  branch's loop and the `d` branch's hand-off all read the sidebar's current,
  filter-narrowed view, so an active filter silently dropped a marked member and
  an archived member was invisible to all three. A new
  `Model.groupMemberSessions(ctx, groupID)` reads the group's complete persisted
  member set via `store.ListSessionsIncludingArchived`, frozen onto
  `m.settingsGroupDeleteMembers` when the confirm opens and read by all three
  decisions. Tests: `TestSettingsGroupDeleteDBranchIncludesFilteredOutMembers`,
  `TestSettingsGroupDeleteMBranchClearsArchivedMembersToo`,
  `TestSettingsGroupDeleteArchivedOnlyGroupStillOpensPrompt`.
- `43b7202` (cure-01-05) — the reused `dd` batch now offers the same explicit,
  non-default transcript-purge choice (`m.bulkDeletePurgeValue`, defaulted to
  keep) for a group delete, resolving each session's transcript through the
  existing `transcriptPathFor`/`purgeSvc` seam and skipping a session whose
  adapter declares none; `bulkDeleteBatch`/`purgeSvc` stay the one seam ordinary
  `dd` and the group delete share. Test:
  `TestSettingsGroupDeleteDBranchOffersPurgeChoiceAndOneUndoStillRestoresTombstones`.
- `0806ba6` (task 020) — the shared-`state.db` case: two `store.OpenPath` handles
  on one file; create and rename seen through `ListGroups`, rename leaving member
  `group_id` untouched, delete observed through the second client's real TUI
  reload path degrading to `default` rather than vanishing. Tests
  (`internal/tui/group_shared_state_db_test.go`):
  `TestSharedStateDBGroupEditsVisibleAcrossClients`,
  `TestGroupCreatedByAnotherClientAppearsAfterReload`; re-taken after the cures
  at `docs/reports/phase4b-retake-01-02-02/` (commit `049199f`).

**Targeted re-verification at this file's code sha.** `ci/run.sh go test -count=1
./internal/tui/ ./internal/store/ ./internal/config/ ./internal/service/` at
`bcd80b9` (code-identical to `baf92ed`): `ok internal/tui 7.177s`,
`ok internal/store 4.479s`, `ok internal/config 0.035s`,
`ok internal/service 7.691s` — exit 0, so every store/tui/config/service test
named above is green at the sha this record is written against.

## Not shipped / still open

Every one of the seven requirements (R133, R134, R135, R128, R129, R130, R131)
shipped; Tier 2 was started and completed per the GO decision. What is *not*
delivered is a green whole-suite gate:

- **The gate is RED at `baf92ed` (exit 1)** on two load-sensitive timeouts inside
  the `features` harness, both green when re-run in isolation at the same sha:
  the `kill_delete_undo.feature:688` scenario's create step waiting for a
  transient `starting` frame the client has already left, and
  `TestThemeChangesAttributesButNotFrameGeometry/ascii`
  (`features/theme_geometry_test.go:185`) comparing two snapshots 100ms apart.
  Neither is a product defect in any of the seven requirements, and neither was
  patched under task 021: per this run's standing rule, a red lane a sweep finds
  is a new task and the sweep is re-run from scratch afterwards. The three
  follow-ups (`021-cure-01`, `021-cure-02`, `021-resweep-01`) live in the run's
  own task state, which is the authority for their status; this file claims
  nothing about whether they have run.
- **Stability is 9/10 at the same sha**, the one failure being the same
  transient-`starting` class, recorded as advisory with its log path.
- Residual gaps this run chose not to fix — including anything that is a finding
  rather than an unmet requirement — are
  [`docs/reports/phase4b-findings.md`](./phase4b-findings.md)'s job (task 025),
  not this file's.

## Attribution corrections carried forward

Two mis-attributions from this file's first landing (`b96015e`, corrected in
`6c2b95c`) are stated correctly above and recorded here rather than silently
dropped, together with a third correction:

1. **R134 / `c1dd5f1`** — the first landing claimed the commit also updated the
   help and mouse wording and the pinned substrings in `cmd/deck/main_test.go`
   and `internal/tui/tui_test.go`. It did not: `git diff-tree --no-commit-id
   --name-only -r c1dd5f1` lists only `internal/tui/tui.go` and
   `internal/tui/interactive_footer_scroll_advertisement_test.go`, and the commit
   message states those existing strings already named the keys.
2. **R128 / `9eefaa2`** — the first landing attributed the
   `DECK_SESSION_WORKSPACE` → `DECK_SESSION_GROUP` rename to `9eefaa2`.
   `git show bf1c085` proves the rename landed in `bf1c085`; `9eefaa2` added only
   the SPEC §10.1 notification-payload projection carrying the `group` field.
3. **R134 / `c1dd5f1`, minor** — it named `elideToWidth`/`footerLegendWithin` as
   the seams the degradation runs through. The added code budgets with
   `stringWidth` and only *mirrors* `footerLegendWithin` (named in its comment);
   it calls neither.

Test function names quoted in this revision were re-read from the files in the
tree at `bcd80b9`, not carried forward: the `f171168` guard renamed several of
them (for example the service test is
`TestSessionContextEnvExportsGroupNotLegacyField`, not the
`…NotWorkspace` name the superseded revision used).

## Report paths cited above (resolve at this commit)

- [`docs/reports/phase4b-tier1-suite/README.md`](./phase4b-tier1-suite/README.md)
- [`docs/reports/phase4b-final-suite/README.md`](./phase4b-final-suite/README.md)
- [`docs/reports/phase4b-stability10/README.md`](./phase4b-stability10/README.md)
- [`docs/reports/phase4b-tier2-decision.md`](./phase4b-tier2-decision.md)
