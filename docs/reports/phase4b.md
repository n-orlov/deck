# Phase 4b — manual session groups + interactive scroll cue (task 024)

Per-requirement record for the whole phase (`prds/phase4b-manual-groups-and-scroll-cue.md`,
GH #25/#30), written on the freeze-line tail after the whole-suite gate (task 021),
the guards (task 022) and the ten-run stability sweep (task 023) had run.

- **Code sha this file is written against:** `f13c848ee21da85d00dfa2e26026ab640f9624ef`
  (`f13c848`, "settings: keep a rejected group name's inline validation on
  screen (R131, #30)") — the **final code sha**: the last commit touching any
  `*.go` or `*.feature` file, per `git log --oneline -- '*.go' '*.feature'`
  with HEAD at this file's own commit. Every commit after it — `0d966e4`,
  `257133e`, `c6a0876`, `b502044`, `93f5790`, `0e1d6e8`, `0563dab`, and this
  file's own — is docs-only (`docs/reports/`, `docs/DELIVERY-LOG.md`,
  `docs/roadmap.md`, `docs/prds/README.md`); re-derived at this commit with
  `git diff --stat f13c848..HEAD -- . ':(exclude)docs/'`, which prints nothing.
  This file's own commit sits on top of that docs-only tail and is itself
  record-only.
- **Task 021's gate sha (the whole-suite sweep's own launch sha):**
  `baf92eda072f6b59ba9b788340f71ed798077504` (`baf92ed`, "docs: re-verify task
  019's group-delete branches at HEAD (R131 part 2, #30)") — task 021's own
  gate ran here and returned RED (exit 1); that result stands in the record as
  history and is never erased. It is **not**, as an earlier revision of this
  file claimed, the sha the requirements below are scored against, and it is no
  longer byte-identical to the shipped tree: four code-touching commits landed
  after it under the cure track (a standing exception to the freeze line, which
  otherwise runs docs-only from task 021's launch on) — `4a1e352`/`8d934d1`
  (`cure-01-01-2`, R133) and `8f8e9e8`/`f13c848` (`cure-01-02-2`, R131) — on top
  of `021-cure-01`/`021-cure-02` (`db9732b`/`a224e43`), which themselves landed
  before those four. `git diff --stat baf92ed..HEAD -- '*.go' '*.feature'` now
  prints **18 files, +925/-167**, not nothing; every requirement below is scored
  at this file's own code sha, `f13c848`, not at `baf92ed`.
- **Docs-only tail:** this file, `docs/reports/phase4b-findings.md` (task 025) and
  this phase's row in `docs/DELIVERY-LOG.md` (task 026) are the docs-only tail
  sitting on top of `f13c848` — no code lands after it outside the cure track
  itself, which by this file's own commit has closed.
- **What this revision supersedes.** The previous landing (`eb6b2ce`, itself
  correcting `b96015e`/`6c2b95c`) scored the phase at gate sha `baf92ed` and
  quoted a gate **RED at `baf92ed`** (exit 1) and stability **9/10 at `baf92ed`**,
  true statements about the tree at the moment `eb6b2ce` landed (22:03:53Z).
  Since then: `021-cure-01` (`db9732b`, 22:13:13Z) and `021-cure-02` (`a224e43`,
  22:18:14Z) fixed the gate's own two red lanes (a `features` create-step wait
  and a theme-settle comparison, neither a defect in any of the seven
  requirements), and `021-resweep-01` re-ran the whole suite from scratch at
  `a224e43` and got GREEN (exit 0) — the gate is now green at the shipped tree,
  not merely in isolation. Separately, `cure-01-01-2` (`4a1e352`/`8d934d1`) and
  `cure-01-02-2` (`8f8e9e8`/`f13c848`) landed real product code AFTER
  `a224e43` — R133's offset now heals through the shared
  `interactiveScrollState` cell rather than only inside
  `scrollInteractiveByLines`, and R131's settings panel keeps a rejected group
  name's inline validation visible past the viewport — neither of which the
  gate at `a224e43` covers, so `cure-01-01-3` re-ran the ten-run stability
  sweep at the fully-shipped tree, `c6a0876` (code-identical to `f13c848`):
  **10/10 passed**, the gate-strength evidence for the sha this file is
  written against. This revision scores every requirement at `f13c848`, names
  all four of the R133/R131 cure commits `eb6b2ce` omitted entirely, and
  restates the gate/stability headline from the current green record while
  keeping `baf92ed`'s RED result and the superseded 9/10 result on the record
  as history, per "Attribution corrections carried forward" and "Gate result
  and stability headline" below.
- **What task 024's own landing adds on top of that.** Every sha, path, test
  name, line reference and commit-count statement in this file was re-derived
  against the tree at this commit rather than carried forward: the two
  mis-attributions the first validation attempt rejected (`c1dd5f1`'s file set
  and `9eefaa2` vs `bf1c085`'s environment rename) were re-proved with
  `git diff-tree`/`git show` and are recorded under "Attribution corrections
  carried forward" item 5; two commit-count phrasings under R128 and R129 that
  did not match their own bullet lists were corrected (eight and nine commits,
  not seven and eight); the guards item that an earlier revision listed as
  still open is closed, because task 022 (`0e1d6e8`) re-took its report at
  `baf92ed` by its own criteria and this task re-ran build/vet/`gofmt` at the
  shipped code as well (all exit `0`,
  [`docs/reports/phase4b-task024/README.md`](./phase4b-task024/README.md));
  and the four targeted packages were re-run green at the shipped code in this
  same iteration.

## Tier 1 → Tier 2 decision

Both tiers were started and shipped. The decision to proceed with Tier 2 —
**GO** — its budget snapshot and its rationale are recorded in full at
[`docs/reports/phase4b-tier2-decision.md`](./phase4b-tier2-decision.md) (task 006,
commit `16fc86a`). That record is the source of truth for the tier branch taken
and names task 020 as the last code-touching task on the GO branch; this file
does not restate its budget arithmetic.

## Gate result and stability headline

- **Whole-suite gate: GREEN, exit `0`,** at `021-resweep-01`'s launch sha
  `a224e4339173bad93342abdeb5e946da71753090` (`a224e43`) — 463s wall clock, 18
  package lines = 15 `ok`, 0 `FAIL`, 3 `[no test files]` (`internal/notify`,
  `internal/search`, `internal/unit`), no `-run` filter and no package list
  (`ci/run.sh go test -p=1 -count=1 ./...`). This is the gate result of record
  for the shipped tree. **Task 021's own gate, at its own launch sha `baf92ed`,
  was RED (exit 1, 489s)** on two load-sensitive `features` timeouts — that
  result is history, never erased, and is restated under
  ["Not shipped / still open"](#not-shipped--still-open) together with the two
  carve tasks (`021-cure-01` at `db9732b`, `021-cure-02` at `a224e43` itself)
  that cured each lane and the from-scratch re-sweep (`021-resweep-01`) that
  produced the green result quoted here, per this run's standing rule that a
  red lane's fix is carved into a new task and the sweep re-run from scratch.
  Four more code-touching commits landed after `a224e43` under the cure
  track — `4a1e352`/`8d934d1` (R133) and `8f8e9e8`/`f13c848` (R131) — so
  `a224e43`'s own gate log does not by itself cover the final code sha
  `f13c848`; the ten-run stability sweep below re-ran the whole suite at that
  later, fully-shipped tree and is the gate-strength evidence for it. Source:
  [`docs/reports/phase4b-final-suite/README.md`](./phase4b-final-suite/README.md).
- **Ten-run stability: 10/10 passed,** at HEAD `c6a0876` (`cure-01-01-3`),
  code-identical to the final code sha `f13c848` (only docs commits sit
  between them) — ~74 minutes wall clock for the ten runs, all eighteen package
  result lines green in every run, zero `FAIL` anywhere. Quoted from the
  source: "**10/10 passed.** … All ten runs exited 0. No failure occurred in
  any run." Neither known-open flake class (the transient-`starting` assertion
  cured by `021-cure-01`/`021-cure-02`, or the SIGWINCH exact-count assertion)
  occurred in any of the ten runs. This supersedes an earlier 9/10 result taken
  at `baf92ed` (the one failure being the now-cured transient-`starting` class),
  which stays on the historical record at commit `bcd80b9` and in
  `docs/DELIVERY-LOG.md`, never erased. Source:
  [`docs/reports/phase4b-stability10/README.md`](./phase4b-stability10/README.md).
- **Guards (task 022, commit `0e1d6e8`):** build, vet and `gofmt` over the
  touched set (that report's own `git diff --name-only c3b530a..baf92ed -- '*.go'`,
  67 Go files), plus the read-only
  path check, all exit `0` — recorded at
  [`docs/reports/phase4b-guards/README.md`](./phase4b-guards/README.md).
  That report is measured at `baf92ed` **by its own criteria** ("at the same
  code sha task 021 gated"), in a scratch worktree pinned to that sha, since
  HEAD has diverged from it; it is deliberately not re-anchored to the shipped
  sha. Because that leaves the shipped code itself unguarded by that report,
  the same three guards were re-run at HEAD under this task —
  `ci/run.sh go build ./...`, `ci/run.sh go vet ./...`, `ci/run.sh gofmt -l .`,
  all exit `0`, with `gofmt -l` listing only the three untracked
  `.spike-preview/` files this run's standing rules leave alone. Logs and real
  exit statuses:
  [`docs/reports/phase4b-task024/README.md`](./phase4b-task024/README.md).
- Task 021's own red lanes and how they were carried are restated under
  ["Not shipped / still open"](#not-shipped--still-open) at the end of this file.

## Tier 1 — the interactive scroll cue (GH #30)

### R133 — the scrolled-back view says so, from the offset actually used

**Shipped**, in five commits (two landings plus three cures) — the mechanism actually shipped is the shared `interactiveScrollState` cell written in the render path (`internal/tui/interactive.go:562-563` calling `m.setInteractiveScrollOffset`, `internal/tui/interactive_scroll.go:43-72`), not `scrollInteractiveByLines` storing the clamp on its own int field as the first cure (`60a551d`) landed it; the two cures below (`cure-01-01-2`) replaced that mechanism after review found it incomplete:

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
- `4a1e352` (`cure-01-01-2`, first attempt) — a resize that reseeds the grid
  with a shorter or empty real history, or a background visible-only reseed
  under `interactive_transport = capture`, changed what `RenderRows` would
  clamp to without either scroll helper ever running, so the stored offset
  stayed stale-high until whatever scroll command happened next, if ever.
  Added `healInteractiveScrollOffsetFromRender`
  (`internal/tui/interactive_scroll.go`), factored out of
  `scrollInteractiveByLines`' own inline heal, and called it both there and at
  the top of `Update` (`internal/tui/tui.go`) for every message while
  `m.interactive` is true. Test
  (`internal/tui/interactive_scroll_render_heal_test.go`):
  `TestInteractiveScrollHealAtUpdateTopLeavesNonInteractiveModelsUntouched`.
- `8d934d1` (`cure-01-01-2`, the fix that actually ships) — neither `60a551d`
  nor `4a1e352` reaches a render with no `Update` between it and the next
  input event (the case `artifacts/review2/tui-reviewer-prd-test.go`'s
  `TestReviewerScrollHealAfterVisibleOnlyReseed` finding names), because the
  offset still lived in a plain `int` field on `Model`, and a clamped value
  `RenderRows` returned could only ever be written onto whichever
  value-receiver copy of `Model` happened to be on the stack. The fix moves
  the position into `interactiveScrollState`, one cell every copy of `Model`
  points at, read through `m.interactiveScrollOffset()` and written through
  `m.setInteractiveScrollOffset(n)`; `scrollInteractiveByLines` now heals its
  base from that cell before applying its delta. RED FIRST at `4a1e352`
  (`artifacts/cure-01-01-2/redfirst-at-4a1e352.log`): stored offset stuck at
  76 against a real scrollback of 0. Test
  (`internal/tui/interactive_scroll_render_heal_test.go`):
  `TestInteractiveScrollOffsetHealsAfterVisibleOnlyReseedOnTheNextUpdate`. The
  `[0, interactive.ScrollbackMaxLines]` clamp and the entry-only history seed
  policy are unchanged by any of the three commits above.

Evidence: `internal/tui` green in Tier 1's targeted record
(`docs/reports/phase4b-tier1-suite/internal-tui.log`, task 005), re-taken after
the first cure at `docs/reports/phase4b-retake-01-01/` (commit `d99dc5c`),
green again in the gate's `internal/tui 6.408s` line at `baf92ed`, and
re-taken a second time at `cure-01-01-2`'s own leaves
(`docs/reports/phase4b-retake-01-01-2/`, tree sha `f13c848`): both
`interactive_scroll_persist_test.go` tests plus the package as a whole PASS.

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

**Shipped**, across eight commits (five landings, one preparatory seam, two cures):

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

**Shipped**, across nine commits (seven landings plus two cure landings), with a
tenth (`e67dd63`) repairing the `features` suite behind them:

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

**Shipped**, across eight commits (three landings, five cures):

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
- `8f8e9e8` (`cure-01-02-2`, first attempt) — `settingsGroupsViewLines` built
  the create/rename input and the delete confirm AFTER every group row, then
  handed the whole slice to `fitLines`, which just truncates to the frame's
  row budget: at 80x24 with 24 persisted groups, selecting a group near the
  end (group-23) left the selected row — and any editor/confirm attached to
  it — entirely off screen. Fix: attach the create/rename input or the delete
  confirm directly to the selected group's own row, and size/center a
  scrolling window (`settingsGroupsWindow`) over the group rows so the
  selected block always stays inside the row budget, appending a
  `groups N-M of T -- up/down scrolls` line when the window is narrower than
  the full list. Tests
  (`internal/tui/settings_groups_test.go`):
  `TestSettingsGroupsListLongerThanViewportKeepsSelectionAndEditorVisible`,
  `TestSettingsGroupsWindowKeepsSelectedBlockWhollyInsideCapacity`.
- `f13c848` (`cure-01-02-2`, attempt 2, the fix that actually ships) — attempt
  1 above still appended `m.settingsGroupNote` (the inline validation row
  explaining a rejected name) AFTER the selected block's window had already
  spent the row budget, so the trailing `fitLines` truncated exactly the row
  that says why the typed name was refused (reproduced: 24 groups at 80x24,
  select group-23, `n`, type `default`, enter — the frame showed the input
  but no `reserved` note anywhere). `settingsGroupsViewLines` now builds the
  selected group's whole extra block — separator, input/confirm, and any
  attached inline note — as a measured slice BEFORE the window is sized, and
  passes its real length as `settingsGroupsWindow`'s `selectedExtra`, so the
  note's row (and the scroll-position row) is reserved out of the window's
  capacity instead of being truncated behind it. Tests (same file):
  `TestSettingsGroupsListLongerThanViewportKeepsSelectionAndEditorVisible`
  gains the two rejected-name subtests
  (`create_rejected_name_keeps_the_validation_visible`,
  `rename_rejected_name_keeps_the_validation_visible`, at both 80x24 and
  100x40);
  `TestSettingsGroupsWindowKeepsSelectedBlockWhollyInsideCapacity` now spans
  extras 0,2,3,4,6,9 since the block cost is measured, not a constant.
  Re-taken at cure-01-02-2's own leaves
  (`docs/reports/phase4b-retake-01-04-01/`, tree sha `0d966e4`, code-identical
  to `f13c848`): all 9 top-level `TestSettingsGroups*` tests (and every
  subtest) PASS, package green.

**Targeted re-verification at this file's code sha.** `ci/run.sh go test -count=1
./internal/tui/ ./internal/store/ ./internal/config/ ./internal/service/`
at `c6a0876` (code-identical to the final code sha `f13c848`, per
`docs/reports/phase4b-stability10/README.md`'s own sha statement):
`ok internal/tui` (7.177s–8.034s across the retakes above), `ok internal/store`,
`ok internal/config`, `ok internal/service` — exit 0 in every re-take, so
every store/tui/config/service test named above is green at the sha this
record is written against. Re-run once more at this file's own commit's parent
(`0563dab`, byte-identical in code to `f13c848` per the exclude-docs diff
above): `ok internal/tui 7.719s`, `ok internal/store 4.715s`,
`ok internal/config 0.035s`, `ok internal/service 7.994s`, exit `0`
([`docs/reports/phase4b-task024/targeted-tests.log`](./phase4b-task024/targeted-tests.log),
exit status in the same directory's `exit-status.txt`).

## Not shipped / still open

Every one of the seven requirements (R133, R134, R135, R128, R129, R130, R131)
shipped; Tier 2 was started and completed per the GO decision. The whole-suite
gate and the ten-run stability sweep are both **green at the shipped tree**
(gate `a224e43`, stability `c6a0876`) as of this revision — what follows is the
history of how they got there, not an open gap:

- **Task 021's own gate was RED at `baf92ed` (exit 1)** on two load-sensitive
  timeouts inside the `features` harness, both green when re-run in isolation
  at the same sha: the `kill_delete_undo.feature:688` scenario's create step
  waiting for a transient `starting` frame the client has already left, and
  `TestThemeChangesAttributesButNotFrameGeometry/ascii`
  (`features/theme_geometry_test.go:185`) comparing two snapshots 100ms apart.
  Neither was a product defect in any of the seven requirements, and neither
  was patched under task 021 itself: per this run's standing rule, a red lane
  a sweep finds is a new task and the sweep is re-run from scratch afterwards.
  `021-cure-01` (`db9732b`) and `021-cure-02` (`a224e43`) cured the two lanes,
  and `021-resweep-01` re-ran the whole suite from scratch at `a224e43` and
  got GREEN (exit 0, 463s, all 18 package lines `ok`/`[no test files]`,
  0 `FAIL`) — the gate result of record quoted under "Gate result and
  stability headline" above. All three carry `splitFrom: "021"` in the run's
  own task state and stand `validated`.
- **Stability was 9/10 at `baf92ed`**, the one failure being the same
  transient-`starting` class, recorded as advisory with its log path at the
  time. `cure-01-01-3` re-ran the ten-run sweep at the fully-shipped tree,
  `c6a0876` (code-identical to the final code sha `f13c848`), after four more
  code-touching commits (`4a1e352`, `8d934d1`, `8f8e9e8`, `f13c848`) landed
  past `a224e43` with no stability run of the shipped tree yet on record: 10/10
  passed, no instance of either known-open flake class. The superseded 9/10
  result stays on the historical record at commit `bcd80b9` and in
  `docs/DELIVERY-LOG.md`, never erased.
- **Previously listed as open, now closed:** an earlier revision of this file
  recorded the guards as unmeasured at the shipped tree, because
  `docs/reports/phase4b-guards/README.md` is pinned to `baf92ed`. Task 022
  (`0e1d6e8`) re-took that report at `baf92ed` on purpose — that is what its
  own criteria ask for — and this task re-ran build, vet and `gofmt` at the
  shipped code as well, all exit `0`
  ([`docs/reports/phase4b-task024/README.md`](./phase4b-task024/README.md)).
  Both shas are now measured, so nothing about the guards is outstanding.
- **The one evidence asymmetry that remains, stated plainly:** there is no
  *single-run* whole-suite gate log taken at the final code sha `f13c848`
  itself. The from-scratch gate of record ran at `a224e43`, four cure commits
  earlier; the gate-strength evidence for `f13c848` is the ten-run stability
  sweep at `c6a0876` (code-identical to `f13c848`), which runs the same
  unfiltered `ci/run.sh go test -p=1 -count=1 ./...` ten times and passed
  10/10. This run deliberately did not spend a further ~8 minutes
  re-single-running a suite already run ten times green on that exact code.
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
4. **R133 and R131, this revision** — the previous landing (`eb6b2ce`) described
   R133's shipped mechanism as `scrollInteractiveByLines` storing the clamp
   (`60a551d`) and never mentioned `4a1e352`, `8d934d1`, `8f8e9e8` or `f13c848`
   anywhere, so neither requirement's record described what actually shipped:
   R133 heals through the shared `interactiveScrollState` cell written in the
   render path (`internal/tui/interactive.go:562-563`,
   `internal/tui/interactive_scroll.go:43-72`, landed by `4a1e352`/`8d934d1`),
   and R131's settings panel keeps a rejected group name's inline validation on
   screen past the viewport (`8f8e9e8`/`f13c848`). All four commits are named,
   with their own mechanism and tests, under R133 and R131 above in this
   revision.

5. **Where the validation rejection of `eb6b2ce` landed.** Task 024's first
   validation attempt rejected the `eb6b2ce` revision of this file for exactly
   the two mis-attributions numbered 1 and 2 above (`c1dd5f1` claimed to have
   touched `cmd/deck/main_test.go`/`internal/tui/tui_test.go`, and the
   `DECK_SESSION_GROUP` rename attributed to `9eefaa2`). Both proofs were
   re-run at this commit, not carried forward:
   `git diff-tree --no-commit-id --name-only -r c1dd5f1` prints exactly
   `internal/tui/interactive_footer_scroll_advertisement_test.go` and
   `internal/tui/tui.go`; `git show bf1c085 -- internal/service/session_context.go`
   contains the `-"DECK_SESSION_WORKSPACE": session.WorkspaceColumn` /
   `+DECK_SESSION_GROUP` change, while `git show 9eefaa2 --stat` touches only
   `internal/service/session_context.go` and its test, adding 124 lines and
   renaming no environment variable. R134's section and R128's `bf1c085`/
   `9eefaa2` bullets above state it that way.

Test function names quoted in this revision were re-read from the files in the
tree at `f13c848`, not carried forward: the `f171168` guard renamed several of
them (for example the service test is
`TestSessionContextEnvExportsGroupNotLegacyField`, not the
`…NotWorkspace` name the superseded revision used).

## Report paths cited above (resolve at this commit)

- [`docs/reports/phase4b-tier1-suite/README.md`](./phase4b-tier1-suite/README.md)
- [`docs/reports/phase4b-final-suite/README.md`](./phase4b-final-suite/README.md)
- [`docs/reports/phase4b-stability10/README.md`](./phase4b-stability10/README.md)
- [`docs/reports/phase4b-tier2-decision.md`](./phase4b-tier2-decision.md)
- [`docs/reports/phase4b-retake-01-01-2/README.md`](./phase4b-retake-01-01-2/README.md)
- [`docs/reports/phase4b-retake-01-04-01/README.md`](./phase4b-retake-01-04-01/README.md)
- [`docs/reports/phase4b-guards/README.md`](./phase4b-guards/README.md)
- [`docs/reports/phase4b-task024/README.md`](./phase4b-task024/README.md)
