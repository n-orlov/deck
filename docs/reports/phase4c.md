# Phase 4c — per-requirement report (cure-01-07, operator ruling 001)

Final code sha (last commit touching a path outside `docs/`):
`1ad530b3c63335c6a06ca07950640dafe410d75b` ("tui: follow selection off an
empty header through live filter edits too (task 022, #32)"). Every commit
after it is docs-only (`da92f63`, `6d861a0`, `8eca5bf` — each confirmed via
`git show --stat --format=''`), so this report is written against that sha
with no behavioural code changed underneath it.

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
standing "a lane found red is cured inside the task holding it" rule.

**Commits.** `9eced9e2`, `d2184388` (task 007, seam) · `cfeb2f30` (task 008,
tests) · `4e30475c` (cure-01-05) · `340b4d6`, `1ad530b` (task 022 sweep
cures).

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
for the two task-022 sweep cures.

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
budged once a session arrived under it.

**Status:** shipped and re-audited at the final code sha; all eleven probes
in `phase4c-probes/r136.md` fail against `2752c9e` and pass against HEAD.

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
onto a hidden or filtered-out header (cure-01-05, `4e30475`).

**Commits.** `0d46c454`, `38e29a3f`, `505256da` (task 012) ·
`7f0a4e4d`, `73828a8b` (task 013) · `1804a728`, `5b9aee67` (task 014) ·
`ef1af220`, `bedf7ee6` (task 015) · `cdce2641` (cure-01-01) ·
`e23bc499` (cure-01-02) · `3529eb90` (cure-01-04) · `4e30475c` (cure-01-05).

**Test IDs.** `TestRepeatedCFoldsNoMoreThanOneGroup`,
`TestLeftRightFoldUnfoldEmptyDefinedGroup` (`internal/tui/fold_unfold_test.go`);
`TestGGKeysLandOnStopsWhenTheSidebarIsHeaderOnly` (`internal/tui/group_test.go`);
`TestSessionScopedKeysAreInertOnAHeader`
(`internal/tui/session_scoped_guard_test.go`).

**Exact fail-before result** (probe 4, product hunk reverted on the landed
tree, from `docs/reports/phase4c-probes/r137.md`):

```
=== RUN   TestSessionScopedKeysAreInertOnAHeader/dd
    session_scoped_guard_test.go:229: dd: keypress 1 of 2 ("d") mutated the model while the cursor rested on a header: pendingDelete: false -> true
--- FAIL: TestSessionScopedKeysAreInertOnAHeader (0.00s)
```

**Status:** shipped and re-audited at the final code sha; all four probes
in `phase4c-probes/r137.md` fail with their own product hunk reverted and
pass on the landed tree.

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

**Status:** shipped and re-audited at the final code sha; both probes in
`phase4c-probes/r138.md` fail against the reverted hunk and pass on the
landed tree.

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

**Status:** shipped and re-audited at the final code sha; all three probes
in `phase4c-probes/r139.md` fail against their reverted hunk (or, for probe
1's ordering assertion, against the reviewer's own body-only revert
`artifacts/review/r139-order-reverted.patch`) and pass on the landed tree.

## Evidence cited

- Whole-suite gate at the final code sha `1ad530b`, exit 0, 19/19 packages:
  `docs/reports/phase4c-fullsuite/README.md` (log: `fullsuite.log`, exit
  status: `fullsuite.exit`).
- Ten-run stability sweep at `da92f63` (the final code sha plus the two
  docs-only gate commits ahead of it), 10/10 PASS, no FAIL marker in any
  run: `docs/reports/phase4c-stability10/README.md` (`run-1.log`..
  `run-10.log`).
- Four per-requirement probe audits, each isolating its own fix from a
  scratch worktree or an in-place revert:
  `docs/reports/phase4c-probes/r136.md`, `r137.md`, `r138.md`, `r139.md`.

## Not covered here

Findings not tied to one of R136-R139 (the two known advisory flake classes,
the docker-socket infrastructure stall, and the F8 probe-wording residual
cured in `2bb61a8`, cure-01-10) belong in `docs/reports/phase4c-findings.md`,
cure-01-08's own deliverable and not yet written as of this report — not
repeated or anticipated here.
