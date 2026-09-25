# Phase 4c — per-requirement report (cure-01-07, operator ruling 001)

Final code sha (last commit touching a path outside `docs/`):
`be7cdbc996b356a2b17ad31a4b7e095e0a98c98a` ("ci: forward DECK_* selectors
into the sibling (task 015)"), confirmed with `git diff-tree
--no-commit-id --name-only -r be7cdbc` (lists only `ci/run.sh`, outside
`docs/`) and `git merge-base --is-ancestor be7cdbc HEAD` (clean). Every
commit after it up to this report's own HEAD is docs-only (`e6306a7`,
`057971b`, and the commit this retake itself lands in — each confirmed the
same way), so this report is written against `be7cdbc` with no
behavioural code changed underneath it. This is the third retake of this
report (`retake-01-01-07`, steering 003/004): the first retake was written
against `10c5021` and
folded in `cure-01-01-2` (`3d058b5`, a sweep-found reload-selection
regression) and task 015's original list-footer redo (`10c5021`); the
second retake folded in `cure-01-01-3` (`610be00`) — an explicitly
navigated header-only-sidebar cursor could still be stolen by a background
arrival that `cure-01-01-2`'s heuristic could not tell apart from its own
automatic-promotion case; this pass folds in the two further non-`docs/`
commits that landed on top of `610be00`: `cure-01-01-4` (`2d282e2`, R136 —
an Esc that clears a held filter still only normalized the selection onto
SOME visible stop by numeric position, never called
`followSelectionViewport`, and could hand the survivor a different session
id once unfiltering shifted row order) and task 015's own `ci/run.sh` fix
(`be7cdbc` — the command task 015's success criterion names,
`DECK_GODOG_PATHS=… ci/run.sh …`, silently ran the whole feature suite
instead of the one named file because the sibling started with a fresh
environment and no selector reached it; CI-plumbing only, no product
behaviour, no R136-R139 section other than the sha citation itself).

Scope: the four requirements phase 4c added or re-touched — **R136**
(viewport follow, GH #31), **R137** (header cursor, GH #32), **R138**
(header click while a preview is live, GH #33) and **R139**
(`default_group_first`, GH #34) — plus the seven behavioural findings
(F1/F2/F3/F6/F7/F8-residual reworded, F5 split into F5a/F5b/F5c) review
raised against the original landing and this cure wave fixed in place.

## R136 — the viewport follows the cursor (GH #31)

**What shipped.** `internal/tui`'s selection-viewport seam
(`m.setSelection`/`followSelectionViewport`, task 007) followed by an
eleven-binding test suite (task 008) covering every R136 table row plus the
wheel-drift and 80x24-flush edge cases. Review found the seam itself
correct but two call sites that bypass it: cure-01-05 (`4e30475`) makes
re-sort/re-group and every mutating path normalize onto a real, visible
stop through the same seam instead of assigning `m.selected` directly, and
task 022's own sweep found two further escapes review had not
named — a session arriving under an empty header (`340b4d6`) and the same
defect reached through `updateFilter`'s live-narrowing keystrokes
(`1ad530b`) — both cured inside the sweep that found them, per the
standing "a lane found red is cured inside the task holding it" rule. A
later sweep (post-review113) found a fourth escape on top of cure-01-05's
own promotion rule and the create/archive reload paths: cure-01-01-2
(`3d058b5`) narrows the header-gains-its-first-row promotion to fire only
when the WHOLE sidebar had zero sessions before the reload, not merely the
selected header's own bucket, so an explicit header the user deliberately
navigated to now survives a background session arrival with no local
creation intent. Review then found cure-01-01-2's `hadNoSessionsAtAll`
heuristic still could not tell an explicitly navigated header on a
header-only sidebar apart from the automatic zero-value-cursor promotion
it exists for: both look identical from the pre-reload state alone (whole
sidebar header-only, selected header's own bucket empty). cure-01-01-3
(`610be00`) adds `Model.selectedByUser`, set the moment `setSelection`
runs — the one seam every deliberate selection gesture (up/down, PgUp/
PgDn, space, `c`/left/right, `g`/`G`, both `/` filter paths, and a sidebar
mouse click) assigns `m.selected` through, and never cleared again — and
gates the promotion on `!selectedByUser` so it only ever fires for the
genuinely automatic case. Review 154 then re-ran the accepted R152-2
counterexamples at `610be00` and found one gesture still bypassed the seam
entirely: the list-mode top-level `esc` case (reached once Enter has
returned focus to the list) that clears a held filter query normalized the
cleared selection onto SOME visible stop by numeric position via
`nearestVisibleSelection`, and never called `followSelectionViewport` at
all — and, per binding ruling 002, unfiltering can also insert an
earlier-sorting nonmatch back before the kept session, shifting its row
index and handing the survivor a *different* session's id even when a row
at the same position stays visible. cure-01-01-4 (`2d282e2`) fixes both:
it captures the selected session's id before clearing `filterQuery`,
re-anchors on that id in the new unfiltered list (falling back to
`nearestVisibleSelection` only when the cursor no longer names a visible
stop — a header cursor's group id never goes stale this way), and then
calls `followSelectionViewport` unconditionally.

**Commits.** `9eced9e2`, `d2184388` (task 007, seam) · `cfeb2f30` (task 008,
tests) · `4e30475c` (cure-01-05) · `340b4d6`, `1ad530b` (task 022 sweep
cures) · `3d058b5` (cure-01-01-2) · `610be00` (cure-01-01-3) ·
`2d282e2` (cure-01-01-4).

**Test IDs.** `TestViewportFollowsUpAndK`, `TestViewportFollowsDownAndJ`,
`TestViewportFollowsPgUp`, `TestViewportFollowsPgDown`,
`TestViewportFollowsG`, `TestViewportFollowsCapitalG`,
`TestViewportFollowsSpace`, `TestViewportFollowsC`,
`TestViewportFollowsFilterQueryEdit`,
`TestViewportWheelDriftIsNotPreservedAfterDown`,
`TestViewportFlushDegradesAt80x24WithNoBlankTail` (all
`internal/tui/viewport_follow_gestures_test.go`); plus
`TestDeckBinaryRefreshesAllConcurrentClients` (`cmd/deck`),
`TestFeatures/create_and_kill_...` and `TestGoldenMinimumFrame` (`features`)
for the two task-022 sweep cures; plus
`TestReview113ExplicitHeaderSurvivesBackgroundArrival` for cure-01-01-2
(`internal/tui/cure_01_01_2_reload_selection_test.go`); plus
`TestCure010103ExplicitHeaderOnEmptySidebarSurvivesBackgroundArrival` for
cure-01-01-3 (`internal/tui/cure_01_01_3_header_cursor_test.go`); plus
`TestReview152ClosedFilterEscapeKeepsSelectionVisible`,
`TestReview152HeldFilterEscapeKeepsHeaderVisible` and
`TestReview154HeldFilterEscapePreservesSessionID` for cure-01-01-4
(`internal/tui/cure_01_01_4_esc_filter_viewport_test.go`).

**Exact fail-before result** (probe 1, against launch sha `2752c9e`, from
`docs/reports/phase4c-probes/r136.md`):

```
=== RUN   TestViewportFollowsUpAndK
=== RUN   TestViewportFollowsUpAndK/up
    viewport_follow_gestures_test.go:116: up: sidebarScroll = 0 leaves selection span [57,58] outside window [0,10)
--- FAIL: TestViewportFollowsUpAndK (0.00s)
```

and, for the task-022 sweep cure, the unnarrowed-suite failure `340b4d6`
fixed (`docs/reports/phase4c-fullsuite/README.md`):
`cmd/deck`'s `TestDeckBinaryRefreshesAllConcurrentClients` timed out
"waiting for \"resumable\" within 1.25s" at `2bb61a8` — a client with zero
sessions whose cursor was promoted onto the default group's header never
budged once a session arrived under it. And, for cure-01-01-2, the
review113 probe (`/run/ralphd/artifacts/review113/hidden-stop-probes.log`):

```
=== RUN   TestReview113ExplicitHeaderSurvivesBackgroundArrival
    review113_reload_test.go:18: background arrival stole explicit header cursor {1 0 2} -> {0 3 0} without a creation intent or selection key
--- FAIL: TestReview113ExplicitHeaderSurvivesBackgroundArrival (0.00s)
```

and, for cure-01-01-3, review's own probe reproduced against the tree
cure-01-01-2 left, HEAD `10c5021`
(`/run/ralphd/artifacts/review132/review132_transitions_test.go`, and this
cure's own reproduction, `/run/ralphd/artifacts/cure-01-01-3-failbefore.log`):

```
background arrival stole explicitly navigated header on header-only
sidebar: {1 0 2} -> {0 0 0}; pending=""
--- FAIL: TestCure010103ExplicitHeaderOnEmptySidebarSurvivesBackgroundArrival (0.00s)
```

and, for cure-01-01-4, review 154's own probe reproduced against the tree
cure-01-01-3 left, HEAD `610be00`
(`/run/ralphd/artifacts/review154/held-filter-identity.log` and
`filter-escape.log`):

```
=== RUN   TestReview152HeldFilterEscapeKeepsHeaderVisible
    review152_filter_controls_test.go:44: list Esc clearing held filter with header cursor: sidebarScroll = 0 leaves selection span [30,30] outside window [0,21)
--- FAIL: TestReview152HeldFilterEscapeKeepsHeaderVisible (0.00s)
=== RUN   TestReview152ClosedFilterEscapeKeepsSelectionVisible
    review152_filter_test.go:25: list Esc clearing held filter: sidebarScroll = 0 leaves selection span [31,32] outside window [0,21)
--- FAIL: TestReview152ClosedFilterEscapeKeepsSelectionVisible (0.00s)
```

and, separately, session-identity survival:

```
=== RUN   TestReview154HeldFilterEscapePreservesSessionID
    review154_held_filter_identity_test.go:45: held-filter Esc changed selected session ID: kept-id -> other-id (raw index=0)
--- FAIL: TestReview154HeldFilterEscapePreservesSessionID (0.00s)
```

**Status:** shipped and re-audited at the final code sha `be7cdbc`; all
eleven probes in `phase4c-probes/r136.md` fail against `2752c9e` and pass
against HEAD, cure-01-01-2's four review113 regression tests (all red at
`46b4073`, per the commit's own fail-before log) pass against HEAD,
cure-01-01-3's own regression test (red at `10c5021`, per
`cure-01-01-3-failbefore.log`) passes against HEAD, and cure-01-01-4's
three regression tests (all red at `610be00`, per
`/run/ralphd/artifacts/review154/held-filter-identity.log` and
`filter-escape.log` above) pass against HEAD too, alongside the
automatic-promotion and local-create paths none of them must regress
(`TestReview113*`/`TestCure0105*`, `cure-01-01-3-related.log`).

## R137 — a group header is a cursor stop (GH #25 follow-on, #32)

**What shipped.** The widest change of the phase: `sidebarCursor`, a
compiler-enforced header-or-row value (task 012), a shared
`guardSessionScopedKey` helper making every session-scoped key (including
`x` and `dd`) inert on a header (task 013), `c`/`left`/`right` fold-unfold
of the header under the cursor including empty and structural-default
groups (task 014), and header-cursor naming in the detail footer/help/
scenarios (task 015). Review found four residual gaps the original landing
missed: `x`/`dd`/detail `r`/`l` were only guarded on *some* header states
(cure-01-01, `cdce264`), headers carried no visible selection cue at all
(cure-01-02, `e23bc49`), an empty or structural-default group with zero
total sessions could not be folded/unfolded (cure-01-04, `3529eb9`), and the
re-sort/re-group seam (shared with R136 above) could normalize the cursor
onto a hidden or filtered-out header (cure-01-05, `4e30475`); cure-01-01-2
(`3d058b5`, shared with R136 above) closed a fifth, sweep-found gap where
`archivedSessionsLoaded` clamped onto a raw row index instead of
normalizing onto a live, visible stop after an archive removal emptied a
selected header's bucket. Review then rejected task 015's own footer
deliverable a second time: the detail-dialog footer named `c`/`left`/
`right` (`bedf7ee6`) but the LIST footer — the only surface on screen while
the cursor is actually walking onto a header, since `i` is inert there too
(task 013's shared guard) — named none of them. `10c5021` fixes that by
placing the cue in SPEC §11.3's status-reason slot on the footer's left,
which a header cursor leaves empty by construction.

**Commits.** `0d46c454`, `38e29a3f`, `505256da` (task 012) ·
`7f0a4e4d`, `73828a8b` (task 013) · `1804a728`, `5b9aee67` (task 014) ·
`ef1af220`, `bedf7ee6` (task 015) · `cdce2641` (cure-01-01) ·
`e23bc499` (cure-01-02) · `3529eb90` (cure-01-04) · `4e30475c` (cure-01-05)
· `3d058b5` (cure-01-01-2) · `10c5021` (task 015 list-footer redo) ·
`610be00` (cure-01-01-3, shared with R136 above).

**Test IDs.** `TestRepeatedCFoldsNoMoreThanOneGroup`,
`TestLeftRightFoldUnfoldEmptyDefinedGroup` (`internal/tui/fold_unfold_test.go`);
`TestGGKeysLandOnStopsWhenTheSidebarIsHeaderOnly` (`internal/tui/group_test.go`);
`TestSessionScopedKeysAreInertOnAHeader`
(`internal/tui/session_scoped_guard_test.go`);
`TestReview113ArchivedReloadCannotLeaveAbsentHeader`,
`TestReview113ArchivedReloadCannotSelectHiddenRow`
(`internal/tui/cure_01_01_2_reload_selection_test.go`);
`TestListFooterNamesTheHeaderCursorFoldKeys`
(`internal/tui/header_cursor_copy_test.go`); plus
`TestCure010103ExplicitHeaderOnEmptySidebarSurvivesBackgroundArrival`
(`internal/tui/cure_01_01_3_header_cursor_test.go`), cure-01-01-3's own
regression, shared with R136 above.

**Exact fail-before result** (probe 4, product hunk reverted on the landed
tree, from `docs/reports/phase4c-probes/r137.md`):

```
=== RUN   TestSessionScopedKeysAreInertOnAHeader/dd
    session_scoped_guard_test.go:229: dd: keypress 1 of 2 ("d") mutated the model while the cursor rested on a header: pendingDelete: false -> true
--- FAIL: TestSessionScopedKeysAreInertOnAHeader (0.00s)
```

and, for `10c5021`'s list-footer redo, the fail-before at `3d058b5`
(`/run/ralphd/artifacts/task-015-list-footer-fail-before.log`),
`TestListFooterNamesTheHeaderCursorFoldKeys`:

```
header_cursor_copy_test.go:220: list footer on a group header does not
  name "c folds/unfolds": "↑/↓ · n new · , settings · ? help · q quit"
header_cursor_copy_test.go:220: list footer on a group header does not
  name "← folds": "↑/↓ · n new · , settings · ? help · q quit"
```

**Status:** shipped and re-audited at the final code sha `be7cdbc`; all
four probes in `phase4c-probes/r137.md` fail with their own product hunk
reverted and pass on the landed tree, cure-01-01-2's two archived-reload
regression tests plus task 015's list-footer redo test all pass against
HEAD, and cure-01-01-3's own regression test (shared with R136 above,
red at `10c5021`) passes against HEAD too. Neither of the two commits
between `610be00` and `be7cdbc` touches this section's code:
cure-01-01-4 (`2d282e2`) is confined to the list-mode top-level `esc`
branch in `tui.go` (R136's viewport/id-preserve seam, not
`guardSessionScopedKey` or the fold/unfold handlers), and task 015's
`ci/run.sh` fix touches only CI plumbing.

## R138 — the header click works while a preview is live (GH #33)

**What shipped.** `resolveSidebarPress` (task 005) routes an interactive-
mode header/collapsed-strip press through the same resolver as list mode,
toggling collapse without a resize and preserving `m.interactive`. Review
found the interactive preview's *border title* still resolved by cursor
position rather than by which session actually holds the keyboard: folding
the interactive session's own group (a header press, or `c`/left with the
cursor elsewhere) could silently retarget the preview to whatever the
cursor lands on next. Fixed by resolving the title by claim, not cursor
(cure-01-03, `c864e6b`), with a feature-level PTY scenario proving the
fold is allowed while the pane keeps the keyboard (`5da35c5`).

**Commits.** `1ba7007e` (task 005) · `c864e6b9`, `5da35c55` (cure-01-03).

**Test IDs.**
`TestInteractiveHeaderPressTogglesCollapseWithoutResize`,
`TestHeaderPressParityBetweenListAndInteractiveMode`
(`internal/tui/mouse_interactive_header_press_test.go`); the cure-01-03
feature scenario in `features/` (fold the interactive session's own group
with a live PTY preview).

**Exact fail-before result** (probe 1, product hunk reverted in a scratch
worktree over `1ba7007`, from `docs/reports/phase4c-probes/r138.md`):

```
=== RUN   TestInteractiveHeaderPressTogglesCollapseWithoutResize
    mouse_interactive_header_press_test.go:53: header press while interactive did not collapse the infra group
--- FAIL: TestInteractiveHeaderPressTogglesCollapseWithoutResize (0.02s)
```

**Status:** shipped and re-audited at the final code sha `be7cdbc`; both
probes in `phase4c-probes/r138.md` fail against the reverted hunk and pass
on the landed tree. No code in this section changed between `1ad530b` and
`be7cdbc` (task 015's list-footer redo, cure-01-01-2/-01-01-3/-01-01-4 all
touch R136/R137's reload/footer/header-cursor/esc-filter paths, and task
015's `ci/run.sh` fix touches only CI plumbing — none touch R138's
press-resolver code).

## R139 — `default_group_first` (GH #34)

**What shipped.** `[ui] default_group_first` plumbed through as a
`KindToggle` bool (task 001), `groupSortsBefore` takes a `defaultFirst`
parameter so the structural default group alone can sort first (task 002),
and the setting applies live on save through `settingsApplyLiveFields` with
no restart (task 003). Review's F6 finding covered R139 too — the
re-sort/re-group seam shared with R136/R137 — cured by the same
cure-01-05 (`4e30475`) commit, so a live `default_group_first` toggle
normalizes the cursor correctly instead of stranding it.

**Commits.** `0ef480f2` (task 001) · `4911855b` (task 002) ·
`b7d7b7d6` (task 003) · `4e30475c` (cure-01-05).

**Test IDs.**
`TestGroupOrderDefaultFirstFlagTrueMovesStructuralDefaultAloneToFront`
(`internal/tui/group_default_first_test.go`);
`TestSettingsSaveReordersGroupsLivePreservingSelection`
(`internal/tui/default_group_first_live_apply_test.go`);
`TestConfigFileUIDefaultGroupFirstDefaultsToFalseAndRoundTripsThroughWrite`
(`internal/config/config_test.go`).

**Exact fail-before result** (probe 3, product hunk reverted, from
`docs/reports/phase4c-probes/r139.md`):

```
=== RUN   TestConfigFileUIDefaultGroupFirstDefaultsToFalseAndRoundTripsThroughWrite
    config_test.go:675: [ui] default_group_first = true should set Settings.DefaultGroupFirst
--- FAIL: TestConfigFileUIDefaultGroupFirstDefaultsToFalseAndRoundTripsThroughWrite (0.00s)
```

**Status:** shipped and re-audited at the final code sha `be7cdbc`; all
three probes in `phase4c-probes/r139.md` fail against their reverted hunk
(or, for probe 1's ordering assertion, against the reviewer's own
body-only revert `artifacts/review/r139-order-reverted.patch`) and pass on
the landed tree. No code in this section changed between `1ad530b` and
`be7cdbc` either (cure-01-01-2/-01-01-3/-01-01-4 all touch R136/R137's
seam, and task 015's `ci/run.sh` fix touches only CI plumbing).

## Evidence cited

- Whole-suite gate at the final code sha `be7cdbc`, exit 0, 19 packages
  (15 `ok` + 4 `[no test files]`, 476s / ~7.9 min):
  `docs/reports/phase4c-fullsuite/README.md` (log: `fullsuite.log`, exit
  status: `fullsuite.exit`) — re-recorded by `a547e38` after
  cure-01-01-4/task 015, superseding the prior evidence taken at `610be00`.
- Ten-run stability sweep at `be7cdbc` (the tree cure-01-01-4 and task 015
  leave), 10/10 PASS, no FAIL marker in any run, same 19 packages, no
  advisory flake recurred: `docs/reports/phase4c-stability10/README.md`
  (`run-1.log`..`run-10.log`) — re-recorded by `retake-01-01-05` (commit
  `e6306a7`), superseding the prior sweep taken at `610be00`.
- Final-code guards (build, vet, gofmt, protected-path audit, `schemaV8`
  search) at `be7cdbc`, all five green:
  `docs/reports/phase4c-guards/README.md` — re-recorded by
  `retake-01-01-06` (commit `057971b`), superseding the prior guards taken
  at `610be00`.
- Four per-requirement probe audits, each isolating its own fix from a
  scratch worktree or an in-place revert:
  `docs/reports/phase4c-probes/r136.md`, `r137.md`, `r138.md`, `r139.md`.

## Not covered here

Findings not tied to one of R136-R139 (the two known advisory flake classes,
the docker-socket infrastructure stall, and the F8 probe-wording residual
cured in `2bb61a8`, cure-01-10) belong in `docs/reports/phase4c-findings.md`,
cure-01-08's own deliverable and not yet written as of this report — not
repeated or anticipated here.
