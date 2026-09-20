# Phase 4b — manual session groups + interactive scroll cue (approach 2, task 009)

Re-written once, from scratch, against approach 2's own pinned code revision —
never carried forward from the superseded approach-1 landing this file held
before this commit. Per-requirement record for the whole phase
(`prds/phase4b-manual-groups-and-scroll-cue.md`, GH #25/#30), covering both the
original delivery (approach 1) and the four review-finding cures approach 2
added on top of it (B1, B2, B3, R1 — tasks 001-004).

## The code revision this file scores

- **Pinned revision:** `70c7430df3b23a46fb735e8573e26ec55908adeb` (`70c7430`,
  "tui: name the manual group model in the c help line, not the removed
  workspace model (R1)") — the sha task 005 pinned
  ([`docs/reports/phase4b-a2-code-revision.md`](./phase4b-a2-code-revision.md))
  as this approach's last code-touching commit. The freeze holds from task
  005's launch on: every commit since is `docs/reports/**` /
  `docs/DELIVERY-LOG.md` / `docs/PLAN.md` only, so the tree this file scores
  against has not moved since the pin.
- **Tree-object hashes, re-read at this file's own commit and matched against
  task 005's pin:**

  | path | at `70c7430` (task 005's pin) | at this file's own commit (`HEAD`) | pair |
  | --- | --- | --- | --- |
  | `internal` | `15506734d4989e111e871a419ebf46c94a3b59a3` | `15506734d4989e111e871a419ebf46c94a3b59a3` | EQUAL |
  | `cmd` | `27ff2eba72ef6a63a6cf49285c4ddc6660b7fb0d` | `27ff2eba72ef6a63a6cf49285c4ddc6660b7fb0d` | EQUAL |
  | `features` | `b5dbe2f565eb96b2654f8d1d64fcc51eb711f60d` | `b5dbe2f565eb96b2654f8d1d64fcc51eb711f60d` | EQUAL |
  | `ci` | `0a183631a2beea070ab0f7d8fa027aecf423e7b0` | `0a183631a2beea070ab0f7d8fa027aecf423e7b0` | EQUAL |

  Left column copied verbatim from `docs/reports/phase4b-a2-code-revision.md`
  (task 005's own pin); right column read fresh at this commit with
  `git rev-parse HEAD:<path>` for each of the four paths — all four equal,
  confirming no code-touching commit landed between the pin and this file's
  own commit.
- **What this file supersedes.** The previous revision of this file (the last
  commit before this one, `bfe3b68` and its ancestors) scored approach 1's
  seven requirements against approach 1's own final code sha, `f13c848`, a
  tree that review later found four findings against (B1, B2, B3, R1). That
  content is superseded, not merely extended: this revision re-derives every
  commit and test identity below from the current tree, at `70c7430`, which is
  `f13c848` plus the four cure commits.

## The four review-finding cures (tasks 001-004), recorded under the requirements they belong to

| finding | cure commit | fixes | owning requirement |
| --- | --- | --- | --- |
| B1 | `113b552ca2fe42bb1b3d10e42f14cdc3f735c8e6` | the sidebar's `/default` filter matched `sessionGroupKey` (raw `GroupName`, empty for NULL/dangling membership) instead of the same `sessionGroupLabel` seam the header and the `i`/`g` dialogs resolve through, so `/default` matched nothing even though the header both cases fall under reads `default` | R129 (sidebar filter) |
| B2 | `075c59c35fbbc22461f748bbe87e87a8ac4139e4` | `sessionsLoaded` refreshed `m.allGroups` but never `m.settingsGroups`, the open settings Groups panel's own snapshot, so a second client's create/rename/delete landed on disk but stayed invisible in an already-open panel until the operator pressed `n`/`r`/`d` themselves | R131 (settings groups panel) |
| B3 | `00f33a5ac204bc2f558352a18c40fefa24ad3bab` | `fallbackLoop`'s displacement-notice write scrolled the already-full reseeded grid like any other overflowing write, creating artificial scrollback that R133's own cue then reported as a scrolled-back position | R133 (scroll cue) |
| R1 | `70c7430df3b23a46fb735e8573e26ec55908adeb` | the `c` help line (and its two pinned test assertions) still said "workspace group" — R128/R129 removed the workspace model and left only the manual group — restated as "manual group", wording-only | R129 (help prose, the residual the pre-cure record left open under "R129 help prose residual") |

Each cure's own regression test, red-before/green-after evidence and targeted
re-run is in its own commit message and `docs/reports/phase4b-a2-cures/{b1,b2,b3,r1}/`
(task 006 attests all four together with one combined targeted re-run,
[`docs/reports/phase4b-a2-cures/README.md`](./phase4b-a2-cures/README.md)).
The requirement sections below cite each cure once, under the requirement it
actually repairs, rather than re-stating the finding narrative.

## Tier 1 → Tier 2 decision

Both tiers were started and shipped in approach 1. The decision to proceed
with Tier 2 — **GO** — its budget snapshot and its rationale are recorded in
full at [`docs/reports/phase4b-tier2-decision.md`](./phase4b-tier2-decision.md)
(approach 1's task 006, commit `16fc86a`); this file does not restate that
budget arithmetic, and approach 2 made no tier decision of its own (per the
standing rules, the tier branch is settled).

## Gate result and stability headline (approach 2's own sweeps, at this pinned revision)

- **Whole-suite gate: GREEN, exit `0`, 445s** — task 007's own from-scratch
  sweep (`ci/run.sh go test -p=1 -count=1 ./...`, no `-run` filter, no package
  list), run at commit `d0b3100` (code-identical to `70c7430`, the freeze
  having already started): 18 package result lines, 15 `ok` + 3
  `[no test files]` (`internal/notify`, `internal/search`, `internal/unit`),
  0 `FAIL`. Headline and counts copied verbatim from
  [`docs/reports/phase4b-a2-final-suite/README.md`](./phase4b-a2-final-suite/README.md)
  and its own `gate-exit-status.txt` (`exit=0 elapsed=445s`) — not re-measured
  by this task.
- **Ten-run stability sweep: 10/10 passed, 0 FAIL** — task 008's own
  `ci/stability.sh 10` run, launched at commit `5fb84b9` (task 007's own
  commit, code-identical to `70c7430`): every one of the ten `run-N.log`
  files and `summary.log`'s own tally line read `PASS`/`10/10 passed`, and
  `failure-grep.log` (a `grep -n 'FAIL'` over all ten per-run logs) is
  committed empty. Elapsed ≈75.7 minutes, within the ~70-75 min reference
  cost. Headline copied verbatim from
  [`docs/reports/phase4b-a2-stability10/README.md`](./phase4b-a2-stability10/README.md)
  and its own `summary.log` — not re-measured by this task.
- Both sweeps independently verified their own four tree-object hashes equal
  task 005's pin (via the pytest tree-hash harness each carries forward from
  task 006); this file's own hash table above is a third, independent read at
  a later commit, and all three agree.

No red lane was found by either sweep, so this file scores every requirement
below as fully delivered with no residual gap from the sweeps themselves.

## Tier 1 — the interactive scroll cue (GH #30)

### R133 — the scrolled-back view says so, from the offset actually used

**Shipped.** The mechanism that ships is the shared `interactiveScrollState`
cell written in the render path (`internal/tui/interactive.go:562-563` calling
`m.setInteractiveScrollOffset`, `internal/tui/interactive_scroll.go:43-72`),
landed across approach 1's `8bd8a91`, `e44b17f`, `60a551d`, `4a1e352` and
`8d934d1` (the render-path heal that actually ships; `4a1e352` was superseded
by it after review found a value-receiver gap) — `interactiveBodyLines` heals
the stored offset to `RenderRows`' clamped, actually-used return value instead
of discarding it, the footer renders the "how far back / not live" cue from
that healed offset (with its own top-of-scrollback wording), and every
`Update` call for an interactive model re-heals from the render before any
scroll helper runs. Tests, all still present at `70c7430`
(`internal/tui`): `TestInteractiveBodyLinesHealsTheStoredOffsetToTheClampedUsedOffset`,
`TestInteractiveBodyLinesHealingReenablesTheNotRepaintedNotice`,
`TestInteractiveFooterCueReportsTheClampedScrolledBackPosition`,
`TestInteractiveFooterCueTopOfScrollbackWordingDiffersFromOrdinary`,
`TestInteractiveFooterAtLiveBottomRendersNoCueAndMatchesPreChangeFooter`,
`TestStoredScrollOffsetHealedAcrossViewAndNextScroll`,
`TestInteractiveDispatcherNilDoesNotSnapStoredOffset`,
`TestInteractiveScrollOffsetHealsFromTheRenderAfterVisibleOnlyReseed`,
`TestInteractiveScrollOffsetHealsOnTheNextUpdateWithNoRenderInBetween`,
`TestInteractiveScrollHealAtUpdateTopLeavesNonInteractiveModelsUntouched`. The
`[0, interactive.ScrollbackMaxLines]` clamp and the entry-only history seed
policy are unchanged by any of these.

**Approach 2's cure, under this requirement:** `00f33a5` (B3, #30) —
`fallbackLoop`'s displacement-notice write (`internal/interactive/grid.go`)
scrolled the reseeded, visible-screen-only grid like any other overflowing
write, so the artificial scrollback it created survived the reseed and R133's
own cue reported it as a scrolled-back position after a pipe displacement.
`writeDisplacementNotice` now writes the notice and clears exactly the
scrollback that write itself created, inside the same lock scope
`writeGrid` already uses, on both `fallbackLoop` call sites — the
`[0, ScrollbackMaxLines]` clamps and both reseed loops' seed ranges are
untouched. Test: `internal/tui TestDisplacementFallbackReseedLeavesNoArtificialScrollbackOrCue`
(real tmux: scrolls back into history, displaces deck's pipe with a second
`pipe-pane -IO`, waits for the fallback to actually reseed, then asserts
`ScrollbackLen()==0`, stored offset `0`, no "not live" footer cue, and the
notice still visible). `internal/interactive TestNoNonTestFileInThisPackageAsksForTheHistoryRange`
(the entry-only-history-seed source guard) still passes, confirming the cure
did not touch the history-range policy.

### R134 — the interactive footer advertises the scroll keys

**Shipped**, unchanged by approach 2. `interactiveFooterLine` names
Shift+PgUp/PgDn while scrolled back and degrades three optional segments (the
R133 cue, the scroll-key advertisement, the forward-note reminder) from least
to most essential, width-budgeted, with `Ctrl+Q` never droppable — commit
`c1dd5f1` (approach 1 task 003), touching only
`internal/tui/tui.go` and its own new test file; help and mouse wording
already named the keys and needed no change. Tests (`internal/tui`):
`TestInteractiveFooterNamesTheScrollKeysWhenRoomAllows`,
`TestInteractiveFooterAtTheLiveBottomNeverNamesTheScrollKeys`,
`TestInteractiveFooterDegradesWithoutOverflowingTheFrame`,
`TestInteractiveFooterScrollKeyWordingAgreesWithHelpAndMouseTables`. Golden
80×24 frame test (`features TestGoldenMinimumFrame`) is the gate-strength
evidence that the frame budget still holds. No cure from tasks 001-004 touches
this requirement.

### R135 — a keystroke that forwards nothing must not snap the view to the live bottom

**Shipped**, unchanged by approach 2. `updateInteractive`'s
`interactiveScrollOffset = 0` reset runs only after the dispatcher-nil guard
*and* after one of the two forwarding helpers agrees to forward the key —
commit `6fcb7bd` (approach 1 task 004). Tests (`internal/tui`):
`TestUpdateInteractiveLeavesTheScrollOffsetUntouchedForAKeyNeitherHelperForwards`
(Alt+Insert, real-tmux-backed, confirmed failing pre-fix),
`TestUpdateInteractiveResetsTheScrollOffsetForAForwardedKey`. Live-behaviour
proof: `features/interactive_scroll.feature`, scenario "typing while scrolled
back snaps the view back to the live bottom". No cure from tasks 001-004
touches this requirement.

## Tier 2 — manual session groups (GH #25)

### R128 — the group model replaces the workspace label

**Shipped**, unchanged in mechanism by approach 2 (its own cures land under
R129/R131 below). Schema `schemaV7` (`groups` table + `sessions.group_id`, no
foreign key by design) plus the one-transaction migration and the removal of
`sessions.workspace` / `store.DefaultWorkspace` / `Session.Workspace` /
`Session.WorkspaceColumn` — commit `bf1c085` (approach 1 task 008 part 1),
which also carries the `DECK_SESSION_WORKSPACE` → `DECK_SESSION_GROUP`
environment rename. `9eefaa2` (same task) adds the SPEC §10.1
notification-payload projection (`group`, never the legacy name). Group CRUD
with SPEC's name rules (trimmed, non-empty, no control characters, case-
insensitive uniqueness, `default` reserved, 32-char cap) — `60075ec` +
`5adeb83` (task 009). AUTOINCREMENT on `groups.id` so a deleted group's id is
never reissued — `57e1a6a` (cure-01-06). The "no legacy identifier" guard —
`f171168` (task 014). Tests (all still present at `70c7430`):
`internal/store TestSchemaV7MigratesV6LegacyColumnRowsToNullGroupID`,
`TestSchemaV7MigrationIsAtomicOnMidMigrationFailure`,
`TestCreateGroupRejectsReservedDefaultName`,
`TestCreateGroupRejectsCaseInsensitiveDuplicate`,
`TestCreateGroupRejects33CharacterName`, `TestCreateGroupAccepts32CharacterName`,
`TestGroupNameRejectsControlCharactersAnywhere`,
`TestDeleteGroupThenCreateNewGroupNeverReusesTheDeletedID`;
`internal/service TestSessionContextEnvExportsGroupNotLegacyField`,
`TestNotificationSessionPayloadCarriesGroupNotLegacyField`,
`TestSessionContextEnvExportsNamedGroupThroughRealPaneLaunch`,
`TestSessionContextEnvGroupCreateResumeParityAndDanglingMembership`;
`internal/tui TestNoLegacyGroupModelIdentifierSurvivesInStoreTUIOrService`.
No cure from tasks 001-004 changes this requirement's own mechanism, but R1
(below, filed under R129) removes the one remaining "workspace group" surface
this requirement's removal left in rendered text.

### R129 — the sidebar renders manual groups

**Shipped.** Alphabetical, case-insensitive group order with `default`
always last (`35806e8`+`3e3e9f7`); the flat/attention-follow legacy sidebar
mode and `[ui] group_by_workspace`/`DECK_GROUP_BY_WORKSPACE` deleted
(`89125f2`); collapse and mouse hit-test keyed by durable group id, persisted
across a Model rebuild (`3044e9b`+`8e9de6e`); `/` matching name, group and
`cwd` (`8773100`); a group with no members still renders its header with
`(0)` (`f77368f`+`db0f94b`, cure-01-02, after review found empty groups could
disappear entirely). Tests (`internal/tui`, still present at `70c7430`):
`TestGroupOrderAaaLeadsAlphabetically`,
`TestGroupOrderZzzSortsBeforeDefaultDespiteName`,
`TestGroupOrderDefaultAlwaysLastRegardlessOfEmptyStringSortingFirst`,
`TestGroupHeaderTextElidesNameAtSidebarFloorKeepingChevronAndCount`,
`TestGroupHeaderTextCountsPopulatedAndEmptyGroups`,
`TestNavigationPrimitivesRespectGroupIDKeyedCollapse`,
`TestCollapsedGroupsSurviveModelRebuildFromUIState`,
`TestFilterByGroupShowsOnlyTheMatchingRow`,
`TestFilterByGroupNameSurfacesEveryMemberOfThatGroup`,
`TestFilterHidesTheHeaderOfAGroupWithNoMatch`,
`TestUnfilteredSidebarAlwaysRendersDefaultWithZeroSessionsAndZeroGroups`,
`TestEmptyGroupsRenderFromDBWithZeroSessions`,
`TestEmptyGroupsRenderFromDBWithUnrelatedPopulatedGroup`,
`TestEmptyGroupsHiddenUnderAnActiveFilter`,
`TestEmptyGroupCollapsesByItsDurableID`,
`TestGroupCreatedByAnotherClientAppearsAfterReload`; `internal/store`
`TestCollapsedGroupsRoundTripsAcrossReopen`,
`TestLastCreateGroupDanglingIDDegradesToDefaultNotEmptyValue`,
`TestSetLastCreateGroupDefaultForgetsTheRememberedNamedGroup`.

**Approach 2's cures, under this requirement:**

- **B1** — `113b552` — the sidebar filter's group leg matched
  `sessionGroupKey` (raw `GroupName`, empty for NULL/dangling membership)
  instead of `sessionGroupLabel`, the seam the header and `i`/`g` dialogs
  already resolve through, so typing `/default` matched nothing even though
  both cases render under the literal `default` header. Switched the filter
  to `sessionGroupLabel`. Test: `internal/tui TestFilterByDefaultMatchesTheSidebarsOwnDefaultLabel`
  (real store-backed: one NULL-group_id session, one dangling-group_id
  session, one named-group control; asserts `/default` keeps both default
  members, shows `default  (2)`, excludes the named-group control).
- **R1** — `70c7430` — the `c` help line (and its two pinned test
  assertions, `internal/tui/tui_test.go:73` and `cmd/deck/main_test.go:482`)
  still said "toggle the selected row's workspace group collapsed/expanded";
  R128/R129 removed the workspace field and `store.DefaultWorkspace` earlier
  in this same phase, leaving the manual group as the only grouping model. Now
  reads "the selected row's manual group", wording-only, no behaviour
  change; `docs/reports/phase4b-a2-cures/r1/grep-workspace-group.txt` records
  that the six remaining "workspace group" matches in `internal`/`cmd` at
  this commit are five `//` comments and one test's own internal
  failure-diagnostic string, none of them rendered product text. This closes
  the "R129 help prose residual" gap the pre-cure record (approach 1's own
  `docs/reports/phase4b.md`, superseded by this file) left open.

`e67dd63` (approach 1, features-package repair after the workspace drop and
the group-order change) keeps the `features` suite in step with this
requirement's landing; no feature-file change was needed for either B1 or R1.

### R130 — membership is set where the session is

**Shipped**, unchanged in mechanism by approach 2. The create modal's `Group`
field, cycling available groups alphabetically with `default` last and
defaulting to the last group created into, persisted in `ui_state`
(`ddeab8d`+`338a600`). The `i` detail dialog's `g` picker moves a session's
group, resolved off the session row itself (`c754c43`+`bf964b8`, the cure
replacing a stale-snapshot read). `57e1a6a` (cure-01-06, see R128) prevents a
remembered create-group id from being reissued to an unrelated group after a
delete. Tests (`internal/tui`): `TestRememberedCreateGroupDoesNotRebindAcrossRestartAfterDeleteAndReplacement`,
`TestRememberedCreateGroupSurvivesRestart`,
`TestCreateGroupDefaultDegradesWhenRememberedGroupDeleted`,
`TestCreateIntoDefaultGroupForgetsTheRememberedNamedGroup`,
`TestMoveSessionGroupThroughPickerChangesOnlyThatOneRow`,
`TestMoveSessionGroupToDefaultClearsGroupID`,
`TestDetailDialogNamesTheSessionsGroupBeforeThePickerOpens`,
`TestGDoesNotDisturbQCtrlCIOrRInsideDetail`,
`TestDetailFooterAndHelpBothNameGForGroupMove`. Live-behaviour proof:
`features/create_session.feature`, scenario "creating a session into a named
group persists its group_id". No cure from tasks 001-004 touches this
requirement.

### R131 — the group list is edited in settings

**Shipped.** A settings Groups section where `n` creates (inline validation
from R128's name rules), `r` renames, both applied immediately, `esc` offers
no discard prompt (`8038b4d`). `d` deletes: empty group with no prompt,
non-empty group opens a two-branch confirm, the destructive branch routing
through the *existing* `dd` batch path — no second deletion implementation
(`74263ab`+`61b6f7d`); the delete decision reads the group's complete
persisted member set, including filtered-out and archived members
(`42c1ffc`, cure-01-04); the reused `dd` batch offers the same
non-default transcript-purge choice for a group delete (`43b7202`,
cure-01-05). The shared-`state.db` case: create/rename/delete each observed
through a second client's own reload (`0806ba6`). The settings panel keeps a
rejected group name's inline validation visible past the viewport, even for
a group near the end of a list longer than the frame (`8f8e9e8`+`f13c848`,
cure-01-02-2). Tests (`internal/tui`, still present at `70c7430`):
`TestSettingsGroupsNCreatesGroupImmediately`,
`TestSettingsGroupsCreateShowsValidateGroupNameErrorInline`,
`TestSettingsGroupsRenameLeavesMemberGroupIDsUntouched`,
`TestSettingsGroupsCopyStatesEditsApplyImmediately`,
`TestSettingsGroupsEscOffersNoDiscardPrompt`,
`TestSettingsGroupsEscDuringCreateCancelsOnlyTheEditorNotTheTakeover`,
`TestSettingsGroupDeleteEmptyGroupHasNoPrompt`,
`TestSettingsGroupDeleteNonEmptyOpensTwoBranchPrompt`,
`TestSettingsGroupDeleteMBranchMovesMembersToDefaultAndDropsGroup`,
`TestSettingsGroupDeleteDBranchReachesTheSameServiceCallDDDoes`,
`TestSettingsGroupDeleteDBranchDropsTheGroupRowOnceTheBatchCommits`,
`TestSettingsGroupDeleteDBranchKeepsTheGroupWhenAMemberFailsToDelete`,
`TestOrdinaryBulkDeleteNeverDropsAGroupRow`,
`TestSettingsGroupDeleteDBranchOneUndoRestoresTheWholeBatch`,
`TestSettingsGroupDeleteDBranchIncludesFilteredOutMembers`,
`TestSettingsGroupDeleteMBranchClearsArchivedMembersToo`,
`TestSettingsGroupDeleteArchivedOnlyGroupStillOpensPrompt`,
`TestSettingsGroupDeleteDBranchOffersPurgeChoiceAndOneUndoStillRestoresTombstones`,
`TestSettingsGroupsListLongerThanViewportKeepsSelectionAndEditorVisible`,
`TestSettingsGroupsWindowKeepsSelectedBlockWhollyInsideCapacity`;
`internal/tui TestSharedStateDBGroupEditsVisibleAcrossClients`. Live-behaviour
proof: `features/kill_delete_undo.feature`, scenario "settings' group-delete
d branch routes through the same dd batch confirm and one u restores the
whole batch (R131 part 2)".

**Approach 2's cure, under this requirement:** `075c59c` (B2) —
`sessionsLoaded` refreshed `m.allGroups` but never `m.settingsGroups`, the
open Groups panel's own snapshot, so a create/rename/delete applied by a
second client sharing the same `state.db` landed on disk immediately but
stayed invisible in an already-open panel until the operator pressed
`n`/`r`/`d` themselves. `sessionsLoaded` now also refreshes
`m.settingsGroups`, gated on the panel actually being open on the groups
category and on a successful groups read, reselecting by the currently
selected row's durable group id rather than by index. Tests:
`internal/tui TestSettingsGroupsPanelRefreshesOnOrdinaryReload` (a create, a
rename and a delete through a second client all show up in an already-open
panel after its next ordinary reload) and
`TestSettingsGroupsPanelReloadPreservesSelectionAndInProgressEditing`.

## Nothing not delivered

All seven requirements (R133, R134, R135, R128, R129, R130, R131) are shipped
at the pinned revision, and all four review findings this approach set out
to cure (B1, B2, B3, R1) are cured on top of them — nothing from the original
PRD scope, and nothing from the review findings, remains outstanding. Both
sweeps this approach ran (task 007's gate, task 008's stability10) are green
at this exact revision with no red lane, so there is no gate/stability gap
to disclose either. This section exists to state that plainly rather than to
list a gap, per this task's own criterion.

## Test identity manifest and checker

Every test identity cited above (Go test functions and the two named `.feature`
scenarios) is listed once, one line per identity, in
[`test-manifest.txt`](./phase4b-a2-record/test-manifest.txt)
(`docs/reports/phase4b-a2-record/test-manifest.txt`), in the form
`<go package> <TestFunc>` or `<features/x.feature> <scenario title>`.
[`check-test-manifest.sh`](./phase4b-a2-record/check-test-manifest.sh)
(committed beside it) resolves every line against the tree at the pinned
revision above — Go lines via `git show <rev>:<file> | grep '^func <name>('`
over every `_test.go` file directly inside the named package directory,
scenario lines via a literal `Scenario: <title>` match inside the named
`.feature` file — and first cross-checks the pin's own four tree-object
hashes. No suite is run by this check; its own output is committed as
[`check-test-manifest.log`](./phase4b-a2-record/check-test-manifest.log)
(88 manifest lines, all `OK`, exit `0`). Re-run it with:

```
sh docs/reports/phase4b-a2-record/check-test-manifest.sh
```

## Report paths cited above (resolve at this commit)

- [`docs/reports/phase4b-a2-code-revision.md`](./phase4b-a2-code-revision.md) (task 005)
- [`docs/reports/phase4b-a2-cures/README.md`](./phase4b-a2-cures/README.md) (task 006)
- [`docs/reports/phase4b-a2-final-suite/README.md`](./phase4b-a2-final-suite/README.md) (task 007)
- [`docs/reports/phase4b-a2-stability10/README.md`](./phase4b-a2-stability10/README.md) (task 008)
- [`docs/reports/phase4b-tier2-decision.md`](./phase4b-tier2-decision.md) (approach 1)
