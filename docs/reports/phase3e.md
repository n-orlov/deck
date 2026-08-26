# Phase 3e report

Skeleton created by task 322 while closing out R58c (the padTrunc/truncateToWidth
call-site audit) — the audit's own success criteria requires its enumeration to
live here, not only in the commit message. Populated incrementally by later
tasks (R59-R62 by tasks 329-332); task 326 filled in the remaining
per-requirement evidence for R52-R58 (sections above), the whole-suite/
stability citations and the final per-requirement table (below) that ties
every requirement to its commits, its non-vacuous test evidence and its
revert-red proof.

- **Tool versions**, resolved with `ci/run.sh sh -c 'go version; tmux -V'`
  (same sibling image, `deck-ci:local`, used for every command cited in this
  report): `go1.25.13 linux/amd64`, `tmux 3.5a`. Reproduced fresh while
  closing out this task (validation-failed for missing tool versions) —
  literal output below, and already on file at
  [`docs/reports/toolchain-versions.md`](toolchain-versions.md).

```
$ ci/run.sh go version
go version go1.25.13 linux/amd64
$ ci/run.sh tmux -V
tmux 3.5a
```

## R52: one-shot select-this-new-session intent (tasks 301, 302)

SPEC amendment 6584299 §11.1: creating a session selects its row immediately,
by id, even when attention order does not place it at index 0, and only once
(a later `sessionsLoaded` must not re-steal a selection the user has since
moved).

**Task 301** (`d7f304d`): `shellCreated`'s success path now records the
created session's id in `Model.pendingSelectSessionID` instead of only
refreshing the list. The first `sessionsLoaded` whose filtered `m.sessions`
actually contains that id selects the row **by id, never by index**, scrolls
it into view via a new `scrollSessionIntoView`, and clears the field. A
filter query that excludes the new session leaves both selection and query
untouched; an id that never appears leaves the field set (harmless).

New tests in `internal/tui/new_session_select_test.go`:
`TestPendingSelectSessionIDSelectsNewRowNotIndexZero` (a),
`TestPendingSelectSessionIDIsOneShot` (b),
`TestPendingSelectSessionIDNeverAppearing` (c),
`TestPendingSelectSessionIDRespectsActiveFilter` (d), plus
`TestPendingSelectSessionIDScrollsIntoView`.

```
$ ci/run.sh go test -count=1 ./internal/tui/ -run 'TestPendingSelectSessionID' -v
--- PASS: TestPendingSelectSessionIDSelectsNewRowNotIndexZero (0.00s)
--- PASS: TestPendingSelectSessionIDIsOneShot (0.00s)
--- PASS: TestPendingSelectSessionIDNeverAppearing (0.00s)
--- PASS: TestPendingSelectSessionIDRespectsActiveFilter (0.00s)
--- PASS: TestPendingSelectSessionIDScrollsIntoView (0.00s)
PASS
ok  	github.com/n-orlov/deck/internal/tui	0.004s
```

Red proof: `tui.go` reverted to `791e089` (pre-3e), the new test file kept —
build fails outright (`pendingSelectSessionID`/`scrollSessionIntoView`
undefined), since the intent wiring the tests exercise does not exist yet.

**Task 302** (`b34ea92`): two scenarios in
`features/new_session_selection.feature` (tag `@new-session-selection`).
Index-0 was ruled out by seeding a `waiting`-status anchor session first (an
attention rank-3 `starting` session is provably ordered after a rank-0
`waiting` one) and asserting the row *order* (anchor, then new session at
index 1) with the existing `screen shows sessions in this order:` step
**before** asserting selection — so "selection happens to be at the top"
cannot pass by coincidence. Full reasoning:
[`docs/reports/phase3e-302-r52-godog/README.md`](phase3e-302-r52-godog/README.md).

```
$ ci/run.sh sh -c 'DECK_GODOG_TAGS=@new-session-selection go test -count=1 ./features/ -run TestFeatures -v'
2 scenarios (2 passed), 19 steps (19 passed)
```
Green log: [`phase3e-302-r52-godog/green.log`](phase3e-302-r52-godog/green.log).

Red proof (task 301's `internal/tui/tui.go` half of `d7f304d` reverted via
`git apply -R`, new files kept):
```
2 scenarios (2 failed), 19 steps (12 passed, 2 failed, 5 skipped)
```
Red log: [`phase3e-302-r52-godog/red-task301-reverted.log`](phase3e-302-r52-godog/red-task301-reverted.log).
`tui.go` restored byte-identical immediately after capture.

## R53: `[ui] sort_order` — schema, comparators, render, live-apply, godog (tasks 303-310)

SPEC amendment 6584299 §11.2: a configurable session order — `attention`
(unchanged default), `created`, `activity`, `name` — reachable from the
settings view, applying live on save without moving the selection off the
same *session*.

**Task 303** (`cbf4eb5`): declares `ui.sort_order` as a `KindEnum` field in
`internal/config/schema.go` (values `attention/created/activity/name`,
default `"attention"`), plumbed through `toml.go` (parse + default),
`toml_write.go` (serialise), `config.Settings`/`FileConfig`, and
`internal/tui/settings.go`'s `settingsEditsFromSettings` +
`settingsEnumValue`/`settingsSetEnum`. `TestSchemaPinsKeySet` and
`TestSchemaScopes` updated by adding exactly the one key
(`ScopeRestartToApply` at this point — task 306 flips it to `ScopeGlobal`
once a live-apply consumer exists). No comparator/render behaviour changes;
the settings view shows the key with zero new widget code (schema-generic
display). `ci/run.sh go test -count=1 ./internal/config/ ./internal/tui/` —
both `ok`.

**Task 304** (`893ff6e`): `sortSessionsByOrder` is the one dispatch entry
point — attention (delegates to `sortSessionsByAttentionStable`, unchanged),
created (`CreatedAt` desc), activity (`StatusAt` desc), name
(case-insensitive asc) — every comparator sharing the same three-level
composite key (primary key, then previous-frame relative position via the
new `previousPositionKey`, then ID ascending) per `attention.go:130`'s
intransitivity warning. `internal/tui/sort_order_test.go` adds per-comparator
ordering tests plus `TestSortComparatorsThreeWayTieIsTransitive`, which
exhaustively checks asymmetry+transitivity over a mixed-previous-membership
3-way tie set for all four orders (attention included, via the refactored
`attentionLessStable(previous)`). `ci/run.sh go test -count=1 ./internal/tui/`
— `ok` (0.489s); existing `attention_test.go`/`attention_wiring_test.go`
unmodified and still green.

**Task 305** (`bb18d5b`): `sessionsLoaded` resolves the configured order via
`Model.effectiveSortOrder` and sorts with `sortSessionsByOrder`, selection
still preserved by id. An unknown/malformed value (unreachable from a real
`config.toml` — parse-time `KindEnum` rejects it there — but reachable from a
`Settings` built directly, e.g. in a test) falls back to `attention` and
surfaces a `sortOrderBanner` on the first painted frame, on `themeBanner`'s
own footing (wired into `computeLayout`'s reserved rows). Grouping
composition: `reorderPreservingGrouping` (`group.go`) keeps which workspace
group renders **first** tied to attention's "most urgent member leads" rule
regardless of `sort_order`, while rows *within* each group follow the chosen
order. `ci/run.sh go test -count=1 ./internal/tui/ ./internal/config/` —
green (0.5s/0.03s). `features/attention_sort.feature` passes **unmodified**
(`git diff --stat` empty), 24s.

**Task 306** (`6a7e7ac`): flips `ui.sort_order`'s schema `Scope` to
`ScopeGlobal` (naming `settingsApplyLiveFields` as the consumer, mirroring
`ui.mouse`/`ui.preview_fit`). `settingsApplyLiveFields` copies a changed
`sort_order` into the running `m.settings` (guarded by
`EnvOverrides["ui.sort_order"]`, a no-op today since no `DECK_SORT_ORDER`
exists, kept for parity) and calls the new `Model.resortSessionsLive()`,
which re-sorts `m.baseSessions`/`m.sessions` in place, preserving the
selected **session by id** (never index) and calling
`scrollSessionIntoView`. New test `TestSettingsSaveResortsLivePreservingSelectionByID`
(20-session fixture; switching to `created` moves the selected session's
index from 0 to 19). Red proof: removing the `settingsApplyLiveFields`
sort_order branch made the test fail with
`m.settings.SortOrder = "attention" after save, want "created"`.
`ci/run.sh go test -count=1 ./internal/tui/ ./internal/config/` — both `ok`.

**Task 307** (`300bfb5`): `features/sort_order.feature` (tag `@sort-order`),
one scenario per order over a shared four-session fixture
(`ord-alpha/bravo/charlie/delta`) engineered so all four orders render four
different, pairwise-disagreeing sequences:

```
attention: ord-delta, ord-bravo, ord-alpha, ord-charlie   (waiting -> error -> running -> idle)
created:   ord-bravo, ord-delta, ord-charlie, ord-alpha   (created_at descending, newest first)
activity:  ord-charlie, ord-alpha, ord-delta, ord-bravo   (status_at descending, newest first)
name:      ord-alpha, ord-bravo, ord-charlie, ord-delta   (case-insensitive ascending)
```

Full fixture table, the settle-race this feature surfaced during authoring
(a premature `screen contains "ord-alpha"` gate let the `name` scenario pass
by fixture-order coincidence against an always-attention stub — fixed by
gating on `screen contains "waiting"` instead) and the new
`the state database session "X" has created_at N seconds ago` harness step:
[`docs/reports/phase3e-307-r53-orders/README.md`](phase3e-307-r53-orders/README.md).

```
$ ci/run.sh sh -c 'DECK_GODOG_TAGS=@sort-order go test -count=1 -v ./features/ -run TestFeatures'
4 scenarios (4 passed), 67 steps (67 passed)
```
Green log: [`phase3e-307-r53-orders/green.log`](phase3e-307-r53-orders/green.log).

Red proof (`sortSessionsByOrder` temporarily hard-wired to always return
`sortSessionsByAttentionStable`):
```
4 scenarios (1 passed, 3 failed)
--- PASS: .../sort_order_defaults_to_attention...
--- FAIL: .../sort_order_=_created...
--- FAIL: .../sort_order_=_activity...
--- FAIL: .../sort_order_=_name...
```
Red log: [`phase3e-307-r53-orders/red-wrong-comparator-reverted.log`](phase3e-307-r53-orders/red-wrong-comparator-reverted.log).
`sort_order.go` restored byte-identical afterwards.
`features/attention_sort.feature` passes unmodified: `ci/run.sh sh -c 'DECK_GODOG_TAGS=@attention-sort go test -count=1 ./features/'` — `ok` (24.2s).

**Task 308** (`8f37a36`): one further scenario
(`@requirement-53-sort-order-live-apply`) selects `ord-alpha` (attention
index 2), opens `,` settings, cycles `ui.sort_order` to `created` with `+`,
saves with `ctrl+s`, and asserts BOTH the resulting row order AND that
`ord-alpha` is still selected — even though `created` moves it to index 3
(an index-preserving-not-id-preserving implementation would leave the
marker on a *different* session, `ord-charlie`, so the fixture fails such an
implementation rather than passing it by accident). Red proof (task 306's
live-apply branch commented out): the row order stayed `attention` after
save, failing `deck client renders session "ord-delta" at line 3, want
strictly after session "ord-bravo" at line 5`.
`ci/run.sh sh -c "DECK_GODOG_TAGS='@sort-order' go test -count=1 ./features/"`
— green (19.6s, 5 scenarios).

**Task 309** (`ae25f2a`): `@requirement-53-sort-order-with-grouping` —
`group_by_workspace` on plus `sort_order = name` over two workspaces of two
sessions each, engineered so group leadership (attention-driven, unchanged)
disagrees with a naive alphabetical-leadership rule, and each group's
within-group order under `name` disagrees with that group's own attention
order. Confirms task 305/306's `reorderPreservingGrouping` composition; no
product code changed by this task. Red proof: reverting the
`m.groupingEnabled()` branch (falling back to plain `sortSessionsByOrder`,
no grouping-order preservation) rendered `gso-alpha-ws`'s group first
instead of `gso-beta-ws`'s (alphabetical leadership instead of attention
leadership) — scenario failed. `ci/run.sh sh -c "DECK_GODOG_TAGS='@sort-order' go test -count=1 ./features/"`
— `ok`, 23.9s (6 scenarios). `git diff --stat features/attention_sort.feature`
— empty.

**Task 310** (`9da97fe`): `TestSettingsEditsFromSettingsCoversEveryFlatKey`
walks `config.Schema`'s flat keys, stages a non-default value into a
`config.FileConfig` via the existing `settingsSet*` helpers, and asserts
`settingsEditsFromSettings` does not silently drop it — the exact omission
class task 215 found for `ui.preview_fit`. Red proof (commenting out
`SortOrder: s.File.SortOrder,`):
```
ui.sort_order (kind enum): settingsEditsFromSettings dropped the
staged value (got "", want "__task310_probe__")
```
`ci/run.sh go test -count=1 ./internal/tui/` — `ok` (0.474s).

## R54: hit-test the press first while interactive, so a sidebar click re-targets (tasks 313, 314)

SPEC amendment 6584299 §11.5: a left press over a sidebar row while
interactive re-targets to that row (restoring the previous window's
geometry, entering the new one), rather than being routed to the
drag-to-copy path; a press on the already-interactive row is a true no-op
(no leave, no re-enter, no resize).

**Task 313** (`e6e1af3`): `tui.go`'s `tea.MouseMsg` interactive branch now
hit-tests a left press before assuming task 216's drag-to-copy gesture. A
press resolving to a sidebar row calls the new
`retargetInteractiveSidebarClick` (`mouse.go`): a no-op if the row is already
the interactive target, otherwise `exitInteractive` (restoring window
geometry byte-exact, the same sequence Ctrl+Q runs) then `enterInteractive`
on the new row. Presses over the preview or the seam fall straight through
to task 216's drag-to-copy path, unchanged; the `mouse.go:216-221` comment
is updated to describe the new routing.

New tests (`internal/tui/mouse_interactive_retarget_test.go`):
`TestInteractivePressOnAlreadyTargetRowIsANoOp` (sentinels in
`interactiveScrollOffset`/`previewFitSessionID` — the fields
`exitInteractive` always clears — survive a press on the already-interactive
row), `TestInteractivePressOnADifferentSidebarRowRetargets` (task 311/203's
floor-refusal trick proves `enterInteractive` is actually attempted on the
new row), `TestInteractivePressOverPreviewStillFallsThroughToDragToCopy`.
Red proof (`mouse.go`/`tui.go` reverted to pre-313 via `git stash`):
`TestInteractivePressOnADifferentSidebarRowRetargets` failed ("retargeting
press left selected = 0, want 1"); the other two passed by coincidence (old
routing already left them unchanged there). Files restored byte-identical.
`ci/run.sh go test -count=1 ./internal/tui/ ./internal/interactive/` — green.

**Task 314** (`d55f174`): the no-op scenario in `features/mouse.feature`
(`@requirement-54-sidebar-click-on-interactive-row-is-a-no-op`) is
restructured around **two** sessions — it first retargets from A to B (a
step that only succeeds because task 313's retarget path exists), and only
then clicks B's own now-interactive row again to exercise the no-op branch.
Reverting task 313 (`git revert --no-commit e6e1af3`) now fails **both**
scenarios red — the no-op scenario failing at its retarget precondition
(`deck client "A" does not have session "retarget-noop-b" selected: timed
out waiting for frame "> retarget-noop-b"`), not a coincidental later step.
With task 313 present: `ci/run.sh sh -c 'DECK_GODOG_TAGS="@requirement-54-sidebar-click-retargets-interactive-mode,@requirement-54-sidebar-click-on-interactive-row-is-a-no-op" go test -count=1 -v -run TestFeatures ./features/'`
— 2 scenarios, 36 steps, all passed, 3.7s. Full `@mouse-bindings` tag green
(25.4s). `features/interactive_selection.feature` untouched (`git diff
--stat` empty).

**Known anomaly, left untouched, out of scope for 326**: this task carries a
recorded validation finding (`tasks.json`'s `validationNotes` on task 314)
that the no-op scenario, run in isolation with only itself
(`@requirement-54-sidebar-click-on-interactive-row-is-a-no-op` alone, not
paired with the retargeting scenario in the same suite run), does not go red
against a reverted task 313 — the two-session restructuring in `d55f174`
makes the no-op scenario's own red proof depend on the retargeting scenario
having already been shown capable of detecting 313's absence in the same
run, per that scenario's `validationNotes`. Recorded here rather than
re-litigated; task 314 stands `completed` in `tasks.json` per prior worker
decisions, unchanged by this report task.

## R55: single sidebar click selects and enters interactive mode (tasks 311, 312)

SPEC amendment 6584299 §11.4 (reverses the old double-click gate): one press
on a sidebar row both selects it and enters interactive mode.

**Task 311** (`8eba4d4`): `clickSidebarRow` now selects the row and calls
`enterInteractive` on every press. Deleted dead double-click state:
`doubleClickWindow`, `lastClickIndex`, `lastClickAt` — `rg` proof (no
remaining references outside `prds/`, protected/read-only) is in the commit
message. Full attach (`a`) still has no mouse affordance
(`attachSelected` is only ever called from the `a` key case). Rewrote
`mouse_test.go`'s double-click-oriented tests:
`TestClickSidebarRowSelectsAndEntersInteractiveNeverCallsAttachSelected`
(a click never calls `attachSelected`) and
`TestClickSidebarRowEntersInteractiveModeOnOnePress` (task 203's 7-row
floor-refusal trick proves the single press actually reached
`enterInteractive`, deterministically, without a live tmux server). Red
proof (`mouse.go`/`tui.go` reverted to `9da97fe`):
`TestClickSidebarRowEntersInteractiveModeOnOnePress` failed (`attachError
"" does not name the 7-row floor`); restored byte-identical.
`ci/run.sh go test -count=1 ./internal/tui/` — passes (0.55s).

**Task 312** (`02a02cd`): rewrites (never deletes/tag-excludes) both
behaviourally-stale `mouse.feature` scenarios. New
`@requirement-33-click-selects-and-enters-interactive-mode`: one press on a
not-yet-selected row asserts BOTH selection moved AND interactive mode
entered, then Ctrl+Q returns to the list, then re-entering the same session
by `↵` is asserted to produce a frame **byte-identical** (real frame
comparison via `features/mouse_synthesis_test.go`'s capture/compare steps,
clock-masked — not a model-field comparison) to the one captured right after
the click-entry. Rewritten `@requirement-33-double-click-enters-interactive-mode`:
double-clicking still lands in interactive mode with no error text, its
second press now landing harmlessly on the (now-interactive) preview.

```
$ ci/run.sh sh -c 'DECK_GODOG_TAGS=@mouse-bindings go test -count=1 -v -run TestFeatures ./features/'
7 scenarios, 99 steps, all green, 7.15s
```

Red proof (`mouse.go`/`tui.go` reverted to `8eba4d4~1`, i.e. pre-311): the
new single-click scenario failed (interactive mode never entered on the
first press); the double-click scenario legitimately still passed (it
already entered interactive mode via its old second-press gate pre-311, so
it alone does not discriminate 311's diff — only the single-click scenario
does). Files restored byte-identical (`git checkout HEAD`).

## R56: three further built-in themes — matrix, cobalt, parchment (tasks 315-317)

SPEC amendment 6584299 §11.6: adding a built-in theme is a one-file-drop
(`.toml`) plus one registry-line contract; no other production code should
need to change.

**Task 315** (`a02168a`): `internal/theme/builtin/matrix.toml` (dark green
phosphor) + one `builtinFiles` line in `registry.go` — no other production
code touched (`grep -rn '"empire"\|"daylight"' internal/ cmd/`, excluding
`_test.go`, finds only `registry.go`'s `DefaultName` constant; `t`/settings
already resolve `[ui] theme`'s choices dynamically from `theme.Builtins()`).
Seven §7 status tokens (authored hex, all seven mutually distinct):

| token | hex |
|---|---|
| waiting | `#ffff33` |
| running | `#00ff66` |
| idle | `#33cc66` |
| starting | `#33aaff` |
| stopped | `#889988` |
| error | `#ff3333` |
| archived | `#66aa77` |

16-colour quantisation legibility checked (not required distinct at 315 —
task 317's job): `waiting/running/error` stay distinct
(`ffff00/00ff00/ff0000`), but `idle+stopped+archived+hint+badge` all collapse
onto the same ANSI grey (`7f7f7f` ×4) under 16-colour quantisation — each
individually still clears the 3:1 contrast floor
(`TestBuiltinContrastFloor`, `TestSessionRowTokensClearContrastFloorOnSurface`,
both hex and quantised) but is not pairwise distinguishable from the others
in that mode. Left as-is per 315's own criteria (contrast floor +
hex-distinctness only); resolving the quantised collision was task 317's
proving ground, not 315's. `internal/theme/contrast.go` and its threshold
are untouched (`git diff --stat internal/theme/contrast.go` — empty).
`TestBuiltinQuantizationPinned` gains matrix's entry, computed offline (a
Python reimplementation of `quantize()`/`hexRGB`, not by calling production
code and asserting self-agreement). `ci/run.sh go test -count=1
./internal/theme/ ./internal/tui/` — green.

**Task 316** (`7559666`, `bb6afec`): two more built-ins, `cobalt` (dark
blue) and `parchment` (warm sepia light), each as a `.toml` drop + one
`registry.go` line + one `TestBuiltinQuantizationPinned` entry. `git show
--stat 7559666` touches exactly two production files (`cobalt.toml`,
`registry.go`) plus the quantisation pin in `quantize_test.go`, proving the
one-file-drop claim. Cobalt's seven hexes: waiting `#ffd166`, running
`#2ecc71`, idle `#5c7a99`, starting `#3fa9f5`, stopped `#5c7a99`
(deliberately == idle, matching the empire/daylight/matrix precedent),
error `#ff5c5c`, archived `#7f9bb3`. Parchment's: waiting `#7a3b12`, running
`#2d6a2f`, idle `#8a7a5c`, starting `#a15c2e` (== accent, daylight's own
precedent), stopped `#8a7a5c` (== idle), error `#a12f2f`, archived `#6f6248`.
`TestBuiltinsHaveOneDarkOneLight` needed **no change**: it already asserts
`len(Builtins())>=2` plus at-least-one-dark/at-least-one-light (not exactly
one of each), so a 4th/5th built-in doesn't touch it — pre-verified in
`notes.md` before this task started (see the "PRD spot-check corrections"
section at the end of this report), confirmed again here.
`ci/run.sh go test -count=1 ./internal/theme/ ./internal/tui/` — green for
all five built-ins (empire, daylight, matrix, cobalt, parchment).
`features/theme_geometry_test.go` (renders every built-in twice) —
`TestThemeChangesAttributesButNotFrameGeometry` passes in `2.02s` wall-clock
(unicode 0.75s + ascii 0.74s), up negligibly from ~1.9s with three themes.

**Task 317** (`78043ee`): `TestMatrixStatusTokensRenderAsSevenDistinctColours`
renders each of the six real `session.Status` words plus the archived
flag's own `▣` badge through the **real production path**
(`Model.sidebarRowLines` → `colorToken` → `theme.Theme.Color`) under the
built-in matrix theme, into a `vt.Emulator`, and reads the actually-painted
foreground colour per-cell off that emulator's grid (`cellFgHex`/`findCol`,
the same extraction every other colour-token proof in the package uses) —
never comparing `matrix.toml`'s authored strings directly, never scraping
raw escape bytes. Gotcha hit and fixed while writing this: naming sessions
`sess-<status>` made the status word a substring of the session *name*, so
`findCol` (first-match) silently read the name segment's colour instead of
the status word's — a false negative masking five of six statuses as
colliding on `#00ff88` (title/accent's shared hex). Sessions are now named
`pane0..pane5/pane99`, unrelated to any status word. Red proof (idle and
starting temporarily collided on `#33aaff` in `matrix.toml`):
```
matrix_status_tokens_test.go:98: matrix theme: 1 of the seven status
tokens render as the SAME painted colour (not pairwise-distinct):
#33aaff: starting, idle
--- FAIL: TestMatrixStatusTokensRenderAsSevenDistinctColours (0.01s)
```
`matrix.toml` restored (`git diff --stat` on it — empty after revert).
`ci/run.sh go test -count=1 ./internal/tui/ ./internal/theme/` — both `ok`.

## R57: seam (and its T-junctions) follows either-panel-focused, not `previewBorderToken` (tasks 318, 319)

SPEC amendment 6584299 §11.7: the seam between sidebar and preview, plus its
`┬`/`┴` T-junctions, reads `border_focus` whenever **either** panel is
focused (list mode: sidebar; interactive mode: preview) — not only while the
preview specifically holds focus.

**Task 318** (`9c13b47`): new `seamBorderToken()` (`panel.go`) resolves the
rule; new `mainViewOverlayActive()` names the one true negative case
(neither panel focused: the theme picker `t` or the filter input `/`, both
of which keep `mainView` rendering underneath per their own file comments
but intercept every keystroke first). `previewTopLine`/`previewBottomLine`/
`previewContentLine` now colour **only** the seam corner/vertical-bar
column with `seamBorderToken()` — the rest of the preview's own border is
untouched, still `previewBorderToken`; glyph ownership (preview draws every
seam glyph) is unchanged. `sidebarBorderToken` and the sidebar's own border
are untouched; stacked-mode (no seam exists there) is untouched.

Tests: `internal/tui/seam_border_test.go` covers sidebar-focused,
interactive, neither-focused (both theme-picker and filter traps), stacked
mode as an explicit regression guard, and the collapsed strip. Updated
`TestSidebarBorderIsFocusPreviewBorderIsUnfocused`
(`main_view_theme_test.go`), whose old seam assertion was exactly the
behaviour this task changes — moved that check to the preview's own right
border. Red proof 1 (seam following `previewBorderToken`-only logic):
```
seam_border_test.go:68: sidebar focused: seam = #334155, want border_focus token #0d9488
```
Red proof 2 (naive hard-coded always-`border_focus`) also demonstrated per
the commit message, discriminating the neither-focused negative.
`ci/run.sh go test -count=1 ./internal/tui/` — passes.

**Task 319** (`63c9e60`): `features/seam_focus.feature` asserts, against a
live client's per-cell rendered grid (never text-scraping), that the seam's
top T-junction (row 0, column 35 at the harness's default 100×24 terminal /
default `sidebar_width` 35 — column derived empirically via a throwaway
scratch render, not asserted from unit-test internals) reads `border_focus`
while the sidebar holds focus, `border_focus` while interactive (preview
focused), plain `border` while neither holds focus (`/` filter open), and
back to `border_focus` once the filter clears. Red proof
(`git revert --no-commit 9c13b47`, feature file kept untracked, then
`git revert --abort` to restore):
```
1 scenarios (1 failed)
... client "seam" cell at row 0 column 35 has foreground #334155, want #0d9488
```
Green: `ci/run.sh sh -c 'DECK_GODOG_TAGS=@requirement-57-seam-follows-either-panel-focused go test -count=1 ./features/ -run TestFeatures -v'`
— 1 scenario (1 passed), 13 steps (13 passed). `ci/run.sh go test -count=1
./internal/tui/` — `ok`. `TestGoldenMinimumFrame` — `PASS`
(`sha256 4c9db23524f49...`); `git diff --stat
features/testdata/golden/side_by_side_80x24.golden` — empty.

## R58a: `truncateToWidth` closes an open background span it truncates through (task 320)

SPEC §11.3 ("every truncated coloured run re-emits its own reset").
`settingsRenderRow` used to open a row's selection/surface background span
once and close it once, only after every segment's text — `padTrunc` →
`truncateToWidth` truncating that composed line before reaching the reset
dropped the reset outright, leaving the background open for whatever the
caller concatenates next to inherit (the exact leak class tasks 321-323 go
on to fix at their own call sites).

`truncateToWidth` (`408a1b7`) now tracks background-setting/-clearing SGR
parameters (`40-47`/`100-107`, extended `48;5;N`/`48;2;R;G;B`, and the
`0`/`49` resets that close them) as every escape passes through untouched,
and appends a synthetic `\x1b[0m` (zero visible cost) if a span is still
open once it stops advancing — whether cut short by budget or because the
source never closed it at all. Foreground-only spans are deliberately left
untouched (a dropped foreground reset only recolours already-unshown text;
a dropped background reset paints another panel's own columns) —
`escape_width_test.go`'s existing
`TestTruncateToWidthKeepsEscapeBytesItPassesOver` truncates a
foreground-only run mid-span and asserts NO reset is synthesised there.

New `internal/tui/escape_background_test.go`:
`TestTruncateToWidthClosesOpenBackgroundSpanMidRun` (open background span,
truncated mid-span → state closed, plus a source that never closes its span
at all), `TestTruncateToWidthLeavesPlainTextAlone` (no-escape case),
`TestTruncateToWidthDoesNotDoubleCloseAnAlreadyClosedBackground`
(escape-only-tail case — a source that already closes its own span must not
get a redundant second reset), `TestTruncateToWidthWideGlyphsStillCloseTheirBackground`
(existing wide-glyph budget 1..6 sweep, now also asserting the span still
closes at every level). Red proof: fix reverted via `git stash` (new test
file kept untracked, not stashed) — the new tests fail against the
pre-fix implementation. `ci/run.sh go test -count=1 ./internal/tui/` —
green, including `escape_width_test.go`, `crop_wide_test.go` and
`preview_crop_test.go` unmodified.

## R58b: sidebar row highlight fills the panel's full inner width (task 321)

`sidebarContentLine` (`f74ed7f`) now opens the row's
selection/selection_idle/surface-stripe background **once** right after the
left border and closes it **once** at the very end — spanning the leading
pad column, the text (or its pad-fill for a short name), and the trailing
pad column, on both lines of a row. Previously `sidebarRowLines` opened and
closed that background itself around only its own composed text, leaving a
short name's pad-fill and the sidebar's flanking single-space columns
uncoloured. `sidebarRowLines` now returns `(lines, bg token)`; a new
`sidebarRowBackground` answers which token (if any) applies. `settings.go`
gains `settingsRenderRowOpen` (composes foreground colours like
`settingsRenderRow` but opens no background and emits no closing reset, so
a caller can open/close around it) — used only by `sidebarRowLines` at this
point (task 322 finds and fixes the other callers that needed the same
treatment).

New `internal/tui/sidebar_row_fill_test.go`:
`TestSidebarSelectionBackgroundFillsFullPanelWidth`,
`TestSidebarSelectionIdleBackgroundFillsFullPanelWidth`,
`TestSidebarStripeBackgroundFillsFullPanelWidth` — each renders the real
frame into a `vt.Emulator` and asserts every column from col 1 to `sw-1` on
both lines of the row carries the expected background hex, for a
short-named session so pad-fill columns dominate. Red proof
(`sidebarContentLine` reverted to ignore `bg`, `sidebarRowLines` reverted to
compose via the old bg/idleBg path): all three new tests failed with `row 0
col 1 has no background at all, want #26324b` (and the selection_idle/
stripe equivalents). `ci/run.sh go test -count=1 ./internal/tui/` — passes
(full package).

## R58c: padTrunc/truncateToWidth call-site audit (task 322, commit `c86e422`)

Enumeration of every padTrunc/truncateToWidth call site, per call site, whether
it could leak an open background/SGR span past its own text into the
pad-fill/flanking columns a caller's own background wrapper spans:

1. `panel.go` `sidebarContentLine` (side-by-side sidebar rows) — task 321
   already fixed this: bg opens after the border, closes once at the end,
   AFTER padTrunc's pad-fill. No change needed here.

2. `panel.go` `fullBoxContentLine` (stacked-mode sidebar/preview panels AND
   every `framedDialog`/`framedDialogScrollable` dialog body) — **REAL GAP,
   FIXED**. Task 321 moved `sidebarRowLines` to open no background of its own
   (`settingsRenderRowOpen`), on the assumption `sidebarContentLine` (its one
   side-by-side caller) was the only site that needed the wrapper treatment.
   `renderStackedFrame`'s sidebar loop feeds the SAME `sidebarEntry.bg` into
   `fullBoxContentLine`, which never got the bg parameter at all — so stacked
   mode painted NO selection/stripe background whatsoever, not merely a
   too-narrow one. `fullBoxContentLine` now takes a `bg theme.Token` and
   opens/closes it exactly like `sidebarContentLine`; `renderStackedFrame`
   threads `sidebarEntry.bg` through. `framedDialog`/`framedDialogScrollable`
   pass `""` (dialogs have no per-row background, only foreground-coloured
   body text via `colorToken`, which self-resets per call — nothing to leak).

3. `panel.go` `collapsedStripContentLine` (3-column collapsed strip) — safe,
   no change. `collapsedStripLines()` emits only the plain »/digit glyphs with
   zero colour tokens (via `m.glyph`, no `colorToken`/`backgroundSGR` call), so
   there is no background, open or otherwise, to leak.

4. `panel.go` `previewContentLine` (preview panel body) — safe, no change.
   Preview content is foreign tmux pane output cropped by `cropRow`, whose two
   `truncateToWidth` calls are already escape-honest via task 320 (a truncated
   span closes itself). `previewContentLine` adds no background of its own
   around that text, so there is nothing this layer could leak that task 320
   doesn't already close.

5. `settings.go` `settingsLeftContentLine`/`settingsRightContentLine` (the
   settings takeover's category/field/env-entry/search-result lists) — **REAL
   GAP, FIXED**. `settingsRenderRow` used to open bg (selection/selection_idle)
   at the very start of a row's text and close it with a SINGLE trailing reset
   immediately after that row's own segments — before
   `settingsLeftContentLine`/`settingsRightContentLine`'s own padTrunc
   pad-fill and flanking padding columns ever ran. Exactly task 321's original
   sidebar bug, one level removed: the highlight stopped at `"> UI"` (4
   columns) instead of spanning the whole panel width. Fixed by introducing
   `settingsListLine{text, bg}` (mirrors `sidebarRowLines`/
   `sidebarContentLine`'s own split exactly): every category/field/env-entry/
   search-result row now composes its text via `settingsRenderRowOpen` (no bg,
   no closing reset) and carries its background token separately;
   `settingsLeftContentLine`/`settingsRightContentLine` open bg once after the
   border and close once at the very end, spanning the pad-fill. `fitLines` is
   now generic (`fitLines[T any]`) so it fits `[]settingsListLine` the same way
   it always fit `[]string` — the zero value (`text ""`, `bg ""`) is exactly
   "no background", matching a truly empty string row under the old shape.
   `settingsRenderRow` itself is now dead (nothing calls it) and removed;
   `settingsRenderRowOpen`'s doc comment updated to say so.

6. `settings.go` `settingsFooterLine` / `tui.go` `belowMinimumNotice` — safe,
   no change. Plain uncoloured text (`truncateToWidth` called directly, no
   `colorToken`/`backgroundSGR` anywhere in the string).

### Non-vacuous evidence

`internal/tui/panel_leak_audit_test.go`, three new tests, each red-first
verified by reverting commit `c86e422`'s `panel.go`/`settings.go`/`tui.go` diff
in place (kept the new test file) and rerunning:

```
TestStackedSidebarSelectionBackgroundFillsFullPanelWidth (gap 2 above)
  reverted: "stacked selected row: row 0 col 1 has no background at
  all, want #26324b" -- FAIL
  fixed:    PASS

TestSettingsCategoryRowBackgroundFillsFullPanelWidth (gap 5, left list)
  reverted: "settings category row: col 1 has no background at all,
  want #26324b" -- FAIL
  fixed:    PASS

TestSettingsFieldRowBackgroundFillsFullPanelWidth (gap 5, right list)
  reverted: "settings field row: col 31 has no background at all, want
  #26324b" -- FAIL
  fixed:    PASS
```

```
$ ci/run.sh go test -count=1 ./internal/tui/
ok   (0.5s)
$ ci/run.sh go build ./...
(clean)
$ ci/run.sh go vet ./internal/tui/...
(clean)
$ gofmt -l internal/tui/*.go
(clean)
```

### Discovered but out of scope for 322

Running the full `@settings` feature tag surfaces TWO pre-existing failures
present already at 321 (`f74ed7f`), unrelated to commit `c86e422` — confirmed
by reverting that commit's diff and rerunning both scenarios in isolation,
which fail identically on the unmodified 321 tree:

- "a settings takeover save of ui.theme changes the running client's own
  render live" (requirement 19) times out cycling the Theme field to
  "daylight": tasks 315/316 added three more built-in themes without updating
  this scenario's +/- cycle count.
- "settings offers clearing the recent-directory history" (requirement 17)
  times out waiting for "cleared recent directory history": task 303 inserted
  `ui.sort_order` between `group_by_workspace` and `clear_recent_cwds` without
  adding the extra `j` this scenario's own in-file comment ("task 215 inserted
  preview_fit... a sixth j is needed") already flags as necessary whenever a
  field is inserted — a seventh `j` is now needed and was never added.

Recorded as task 333 in `tasks.json` (fixed separately, see that task's
evidence once it lands, cross-referenced here by task 326).

## R58d: per-cell background rectangle/seam godog evidence (task 323)

`features/panel_background_rectangle.feature` adds two scenarios read per-cell
off a real running client (never text-scraping), over a fixture with
`[ui] sort_order = "name"` and `group_by_workspace = false` so the row order
is deterministic: `rec-aaa`, `rec-bbb-selected-session-with-a-name-far-too-
long-to-fit-in-the-sidebar-at-all` (79 chars, truncates), `rec-ccc`,
`rec-ddd-stripe`.

Frame-row math (proved with a literal frame dump during authoring, quoted
below): row 0 is the sidebar's own top border, row 1 is the socket-info
header line, so the FIRST session's two lines start at row 2, not row 1 as
an earlier draft of this file assumed. Position 0 (rec-aaa) -> rows 2-3,
position 1 (rec-bbb, selected) -> rows 4-5, position 2 (rec-ccc) -> rows 6-7,
position 3 (rec-ddd-stripe, odd stripe phase) -> rows 8-9. Sidebar content
columns run 1-34, seam is column 35.

Debug frame dump that found the off-by-one (temporary step, removed before
the final commit):

```
+ deck - sessions -----------------+---------------------------------------------------------------+
| socket: deck_test_143_2          | /tmp/deck-scenario-476548515/walking-skeleton-cwd             |
|   rec-aaa running                |                                                               |
|   created <relative-time>               | No live preview captured for this row yet.                    |
| > rec-bbb-selected-session-wi... |                                                               |
|   created <relative-time>               |                                                               |
|   rec-ccc running                |                                                               |
|   created <relative-time>               |                                                               |
|   rec-ddd-stripe running         |                                                               |
|   created <relative-time>               |                                                               |
```

Two new cell-attribute steps were registered (`features/cell_attributes_test.go`):
`cells at row R columns C1 to C2 have background token "T"` (a per-column
loop, so a highlight that stops one column short anywhere in the rectangle
fails, not just at a spot-checked column) and `cell at row R column C has no
background set` (reads `Style.Bg == nil` directly, so "no colour" is never
confused with "some non-matching colour").

Green (`ci/run.sh sh -c 'DECK_GODOG_TAGS="@requirement-58-selection-background-fills-rectangle-and-seam-stays-clear,@requirement-58-surface-stripe-fills-rectangle" go test -count=1 ./features/ -run TestFeatures'`):
2 scenarios, 2 passed, 26 steps passed, 2.69s.

Red proof (`git revert --no-commit c86e422 f74ed7f 408a1b7`, i.e. tasks
322/321/320 reverted, same tree, same tags): 2 scenarios, 2 failed --
`client "A" cell at row 4 column 1: cell " " has no background colour set
(terminal default)` (selection scenario) and `client "A" cell at row 8
column 1: cell " " has no background colour set (terminal default)`
(surface-stripe scenario). Workspace restored with `git reset --hard HEAD`
immediately after (`git status --short` empty, `git diff --stat` empty).

`TestGoldenMinimumFrame` remains green on the final tree; `git diff --stat`
on `features/testdata/golden/side_by_side_80x24.golden` is empty (file
untouched by this task).

## R59: stop writing `probe.miss` as an event (steer 3e-001, task 329)

SPEC §7 amendment quoted in steer 3e-001 §3: "A diagnostic sampling result is
a column, not an event." At the default reconcile cadence one session that
samples but never matches a §7 probe rule was writing a fresh `probe.miss`
row to `events` roughly twice a second, forever, with no reader for the kind
(operator-reported hang; part of the 3e-001 root cause alongside the missing
events indexes (R60) and the render-path store read (R61)).

`internal/store/store.go`'s `RecordProbeMiss` no longer calls
`mutateSessionWithEvent` (which always pairs its UPDATE with an events
INSERT in the same transaction). It now runs a plain
`UPDATE sessions SET last_probe_at = ? WHERE id = ?`, checks `RowsAffected`
itself, and appends nothing to `events`. The `i` detail dialog's "sampled,
no rule matched" line (`internal/tui/tui.go:4064-4067`) is untouched — it
reads only `session.LastProbeAt > session.StatusAt`, never the events table.

Test (`internal/service/reconcile_test.go`,
`TestProbeMissRecordsSampleAgeWithoutTouchingStatus`) drives a session whose
sampled pane provably matches no §7 probe rule (`echo 'nothing recognisable
here'`) through `ReconcileWithProbes`, then asserts BOTH halves together per
steer 3e-001 §6.2: `last_probe_at` advanced to the reconcile clock's `now`,
AND the session's total `events` count is unchanged across the miss (a
separate `SELECT count(*) FROM events WHERE session_id = ? AND kind =
'probe.miss'` is also asserted `= 0`). Asserting only the count-unchanged
half would also pass if `RecordProbeMiss` were deleted outright, which would
break the `i` dialog — hence both halves are required in the same test.

Green (`ci/run.sh go test -count=1 ./internal/store/ ./internal/service/ ./internal/tui/`):
all three `ok` (store 1.835s, service 3.176s, tui 0.698s).

Red proof: `git stash push -- internal/store/store.go` (reverting only the
fix, keeping the new test), then
`ci/run.sh go test -count=1 ./internal/service/ -run TestProbeMissRecordsSampleAgeWithoutTouchingStatus -v`:

```
=== RUN   TestProbeMissRecordsSampleAgeWithoutTouchingStatus
    reconcile_test.go:384: session events count = 1 after probe miss, want unchanged from 0 (a probe miss must never append to events, SPEC §7 amendment)
--- FAIL: TestProbeMissRecordsSampleAgeWithoutTouchingStatus (0.05s)
FAIL
FAIL	github.com/n-orlov/deck/internal/service	0.057s
```

Workspace restored with `git stash pop` immediately after; re-run of the
same test then passes (`ok  	github.com/n-orlov/deck/internal/service	0.059s`).

No other production code references the `probe.miss` kind string;
`internal/tui/badge_detail_test.go`'s probe-miss assertions read
`LastProbeAt`/`StatusAt` directly and needed no change.

## R60: index events (steer 3e-001, task 330)

SPEC.md's amended events DDL (commit `6584299`) declares two indexes that a
fresh v1-created `events` table never had:

```
CREATE INDEX events_at ON events(at DESC, seq DESC);         -- §12's newest-first reads are never a full scan
CREATE INDEX events_session_kind ON events(session_id, kind); -- §6.4's env-apply reads likewise
```

`internal/store/store.go` gains `schemaV5` (both `CREATE INDEX IF NOT EXISTS`
statements) and bumps `SchemaVersion` 4->5; `migrate`'s switch grows a
`case 4: ... fallthrough`-terminated arm so a fresh database (falls through
from case 0) and an existing v1-v4 database (opens directly into case
1/2/3/4) both land on v5 through the same statements. The migration is
index-only -- no `ALTER TABLE`, no row touched.

**Migration correctness** (`internal/store/store_test.go`,
`TestOpenMigratesV4FixtureAddsEventsIndexesWithoutTouchingExistingRows`):
builds a byte-for-byte v4 fixture (schemaV1-V4 statements, `schema_version =
4`) with one `sessions` row and three `events` rows spanning both a real
`session_id` and two orphaned (`NULL`) rows -- the two shapes
`events_session_kind` covers. Opens it via `OpenPath` (migrates to v5 in
place), then asserts: `schema_version = 5`; both `events_at` and
`events_session_kind` exist in `sqlite_master`; the `events` row count is
still exactly 3 and every column of every row (`session_id`, `at`, `kind`,
`reason`, `payload`) reads back byte-identical to what the fixture wrote,
in original `seq` order; the `sessions` row count is still 1 and its `name`
is unchanged. Passes for a fresh database too (every other store test opens
a fresh `OpenPath` and lands on `SchemaVersion = 5` with both indexes
present, since schemaV5 always runs as part of the v1-created chain).

**Query-plan correctness on a seeded LARGE table, never on elapsed time**
(steer 3e-001 §6.1) (`TestListEventsQueryPlanUsesEventsAtIndexOnSeededLargeTable`):
a plan assertion on a small table is vacuous -- SQLite's planner picks either
a scan or an index on a handful of rows in microseconds either way -- so the
test seeds 20,000 `events` rows via a single prepared-statement transaction,
then runs `EXPLAIN QUERY PLAN` on ListEvents' own exact statement
(`SELECT seq, session_id, at, kind, reason, payload FROM events ORDER BY at
DESC, seq DESC LIMIT 200`). Assertion is on the plan TEXT: no row is the
bare string `"SCAN events"` (a full-table scan; with the index the row
reads `"SCAN events USING INDEX events_at"`, so the check is an exact-match
on the bare string, not a substring check, since `"SCAN events"` is itself a
prefix of the indexed form), and no row contains `"TEMP B-TREE"` (an
unindexed `ORDER BY` sorts via a temp b-tree as its own separate plan row).
A final `ListEvents(limit=1)` call over the seeded table proves the index
changed the plan without changing the result (still returns the newest row,
`at = 19999`).

Green (`ci/run.sh go test -count=1 ./internal/store/`): `ok
github.com/n-orlov/deck/internal/store 1.589s` (includes an update to
`TestOpenRefusesNewerFixtureWithoutMutation`, which had hardcoded
`schema_version = 5` as its "one newer than supported" fixture -- now
`SchemaVersion + 1` computed at test time so this task's own 4->5 bump does
not silently turn it into a same-version fixture that skips the refusal
path; a comment records why).

Red proof: `git diff internal/store/store.go > /tmp/task330.patch; git
stash -- internal/store/store.go` (reverts only schemaV5/the migration
switch arm/SchemaVersion, keeping both new tests), then
`ci/run.sh go test -count=1 ./internal/store/ -run
'TestOpenMigratesV4FixtureAddsEventsIndexesWithoutTouchingExistingRows|TestListEventsQueryPlanUsesEventsAtIndexOnSeededLargeTable'
-v`:

```
=== RUN   TestOpenMigratesV4FixtureAddsEventsIndexesWithoutTouchingExistingRows
    store_test.go:1718: index events_at after v4->v5 migration = 0, <nil>; want 1
--- FAIL: TestOpenMigratesV4FixtureAddsEventsIndexesWithoutTouchingExistingRows (0.12s)
=== RUN   TestListEventsQueryPlanUsesEventsAtIndexOnSeededLargeTable
    store_test.go:1847: query plan = [SCAN events USE TEMP B-TREE FOR ORDER BY], want no bare full-table scan of events (events_at index missing or unused)
--- FAIL: TestListEventsQueryPlanUsesEventsAtIndexOnSeededLargeTable (0.09s)
FAIL
FAIL	github.com/n-orlov/deck/internal/store	0.213s
```

Workspace restored with `git stash pop` immediately after; re-run of
`ci/run.sh go test -count=1 ./internal/store/` then passes
(`ok  github.com/n-orlov/deck/internal/store 1.651s`).

## R61: get the event-log store read out of the render path (steer 3e-001, task 331)

SPEC §11.4 amendment (steer 3e-001 §3/§6.3, commit `6584299`): "a dialog
never reads the store from its render path." Before this task, `eventLogBody`
-- called from `eventLogView`, which `View()` calls every frame while `E`'s
dialog is open -- ran `m.store.ListEvents(...)` directly, so a live client
left with the event log open re-queried `events` on every single rendered
frame (previewTick alone fires every `DECK_PREVIEW_MS`; reconcileTick every
`DECK_RECONCILE_MS`), part of the 3e-001 hang alongside R59/R60.

`internal/tui/event_log.go`'s `eventLogBody`/`eventLogView` now read only
two new `Model` fields, `eventLogRows []store.Event` and `eventLogErr
error` -- never `m.store` directly. The one real read is `loadEventLog`, a
`tea.Cmd` (mirroring `loadArchivedSessions`' own shape) that `internal/tui/
tui.go`'s `"E"` key handler dispatches exactly once, the same keypress that
sets `eventLogOpen = true` and resets `eventLogScroll`/`eventLogRows`/
`eventLogErr`. Its reply is consumed by one new `Update` case,
`eventLogLoaded`. `updateEventLog`'s PgUp/PgDn continue to measure
`eventLogBody()`'s output for scroll bounds, but that now measures the
in-memory rows too, not a fresh store call. Grep proof `eventLogView`/
`eventLogBody` never call `m.store`:

```
$ grep -n 'm\.store\.' internal/tui/event_log.go
97:	events, err := m.store.ListEvents(context.Background(), maxEventLogRows)
```

(the one hit is inside `loadEventLog`, never inside `eventLogView`/
`eventLogBody`). `E` is not deleted (steer 3e-001 §7); `maxEventLogRows`
(200) and `maxEventLogPayloadRunes` (96) are unchanged.

**Test** (`internal/tui/event_log_render_path_test.go`,
`TestEventLogViewNeverReReadsTheStoreOnceOpen`, per steer 3e-001 §6.3): uses
an INSTRUMENTED real store -- `internal/store/store.go` gained an atomic
`eventsListCalls` counter incremented inside `Store.ListEvents` itself (not
a mock/interface substitute layered over the concrete `*Store` type every
other `tui` code path depends on directly) and a `ListEventsCallCount()`
reader. The test opens the event log (runs the real `"E"` Update, executes
the returned `loadEventLog` Cmd, feeds its reply back through `Update`),
asserts the call count is exactly 1, then runs FIVE more rounds of
`View()` + `reconcileTick` + `previewTick` + `View()` while the dialog
stays open, and asserts the count is STILL exactly 1 -- not merely small.
A single-frame test could not distinguish load-once from load-every-frame;
this one deliberately drives several ticks and renders first.

Green (`ci/run.sh go test -count=1 ./internal/tui/ ./internal/store/`):
both `ok` (tui 0.678s-0.822s across runs, store 1.780s-1.915s).

Red proof: temporarily edited `eventLogBody` in place back to its
pre-fix shape (direct `m.store.ListEvents` call inside the render path,
keeping the new test and the store instrumentation untouched -- a
signature-compatible change, so no patch/stash round-trip was needed), then
`ci/run.sh go test -count=1 ./internal/tui/ -run
TestEventLogViewNeverReReadsTheStoreOnceOpen -v`:

```
=== RUN   TestEventLogViewNeverReReadsTheStoreOnceOpen
    event_log_render_path_test.go:66: after several ticks and renders, ListEvents was called 11 times, want exactly 1 (the render path must never re-read the store)
--- FAIL: TestEventLogViewNeverReReadsTheStoreOnceOpen (0.03s)
FAIL
FAIL	github.com/n-orlov/deck/internal/tui	0.031s
```

(1 call from the initial open + 10 more, one per `View()` in the five
round-trips -- the two `reconcileTick`/`previewTick` `Update` calls
themselves never call `View()`, so the count is `1 + 5*2 = 11`, exactly
matching how many times `eventLogBody` actually ran.) `eventLogBody` was
then restored to its post-fix shape and the suite re-run green.

`features/event_log.feature` (unmodified) still passes end-to-end through
the real bubbletea runtime, which DOES execute `Update`'s returned Cmd
(unlike the unit test above, which does so explicitly): `ci/run.sh sh -c
'DECK_GODOG_TAGS=@requirement-32 go test -count=1 ./features/'` -- both
`@requirement-32` scenarios pass (2 scenarios, 29 steps, ~2.6s).

**Pre-existing, unrelated failure noticed during this task's full
`./features/` run** (not caused by this task's diff -- confirmed by
`git stash`-ing this task's changes and re-running the same test against
the unmodified task-330 tip, `faba630`, which fails identically):
`TestBlackBoxAssertionsObserveRealSession`
(`features/assertions_test.go:1068`) hardcodes `databaseSchemaVersion(...,
4)`, which task 330's `SchemaVersion` 4->5 bump left stale (`schema version
= 5, want 4`). Flagged for task 324 to root-cause/fix, not fixed here (out
of this task's scope, and fixing a hardcoded schema-version assertion is
unrelated to the render-path change this task makes).

## R62: bounded events retention (steer 3e-001, task 332)

SPEC §6.5/§12 amendment (steer 3e-001 §3/§6.4/§7, commit `6584299`):
`event_retention_days` (default 30) bounds how long `events` rows are kept;
deletion runs oldest first, in bounded batches, on store open and
thereafter at most once an hour, with no automatic `VACUUM`.

`event_retention_days` is a new top-level, schema-declared `KindInteger`
field (`internal/config/schema.go`, `Default: 30`, `IntBounds.Min: 1`,
`Scope: ScopeRestartToApply` -- the same restart-to-apply shape and reason
as `stale_after`/`tmux_mouse`: the sole consumer is `cmd/deck/main.go`'s
`tuiReconcile` closure, which has no path back into a refreshed
`config.Settings`). `internal/config/toml.go`/`toml_write.go` parse and
serialise it; `config.Settings`/`FileConfig` carry it;
`internal/tui/settings.go`'s `settingsEditsFromSettings`,
`settingsIntegerValue` and `settingsSetInteger` all include it. R7's
generated settings view picks it up for free, confirmed via the existing
schema-parity tests rather than any new settings-specific test:
`TestSchemaPinsKeySet` and `TestSchemaScopes`
(`internal/config/schema_test.go`) were updated by adding exactly this one
key, and `TestSettingsCategoriesGroupEveryFlatKeyExactlyOnce`
(`internal/tui/settings_test.go`, unmodified) still passes because it
walks `config.Schema` generically -- it needed no edit at all to pick up
the new field.

`Store.EnforceEventRetention(ctx, retentionDays, now)`
(`internal/store/store.go`) is the deletion engine. `now` is UnixMilli, the
same units `events.at`/`sessions.last_probe_at` already use elsewhere in
this file. Its own throttle -- "on store open and thereafter at most once
an hour" -- is persisted in `ui_state` (key `event_retention_last_run_at`),
not held only in memory, because deck's own hook-command invocations of
`store.Open` are each a fresh, short-lived process: an in-memory throttle
would silently reset every time and never actually throttle across
processes. A call is a real deletion pass whenever no `ui_state` row exists
yet (first ever call) or at least `eventRetentionMinInterval` (1 hour) has
passed since the last real pass; every call in between is a single cheap
`ui_state` SELECT. Each real pass loops `DELETE FROM events WHERE seq IN
(SELECT seq FROM events WHERE at < ? ORDER BY at ASC, seq ASC LIMIT
?)` (oldest first) in batches of `eventRetentionBatchRows` (500), capped at
`eventRetentionMaxBatches` (200) batches per call (100,000 rows), so one
call's synchronous work stays bounded even against a very large backlog;
any remainder is left for the next hourly pass. No `VACUUM` call exists
anywhere in this file.

`cmd/deck/main.go` wires it in exactly one place, per the schema comment's
own claim: `run()` calls `db.EnforceEventRetention` once, unconditionally,
right after `store.Open` (satisfying "on store open" literally, since the
throttle above always lets the very first call through), and again inside
the `tuiReconcile` closure on every reconcile tick thereafter (throttled to
hourly by the same mechanism). `runHook`'s own separate `store.Open` call
deliberately does NOT call it, for the same reason the hook path already
wires only `ReconcileWithin` and never probes: hooks share a tight,
measured latency budget, and the always-running main client's own next
reconcile tick already guarantees catch-up within the hour.

**Test** (`internal/store/event_retention_test.go`, per steer 3e-001
§6.4): `TestEnforceEventRetentionDeletesOldOnesKeepingBoundaryAndRecentRows`
seeds 1,200 rows strictly older than the cutoff (`kind='old'`, more than
2x `eventRetentionBatchRows` so a single call must loop across at least
three batches to finish), one row exactly AT the cutoff (`kind='boundary'`)
and 50 rows strictly inside the window (`kind='recent'`), then asserts
BOTH halves together after one `EnforceEventRetention` call: every `old`
row is gone AND the `boundary` row AND all 50 `recent` rows survive -- a
one-sided assertion ("old rows are gone") would also pass a retention pass
that deleted everything, and the other one-sided assertion ("recent rows
survive") would also pass a pass that deletes nothing. The boundary row's
fate is explicit: the deletion query is `at < cutoff` (strict), so a row
whose `at` EQUALS the cutoff falls INSIDE the retention window and is kept
(asserted present, not absent).
`TestEnforceEventRetentionThrottlesToAtMostOnceAnHour` separately proves
the hourly throttle: a first call (simulating store-open) always deletes;
a second call 10 minutes later, against a freshly-seeded row that is
already due, is a no-op (row survives); a third call more than an hour
after the first lets a real pass through again and the row is finally
cleared. `TestEnforceEventRetentionRejectsNonPositiveWindow` covers the
two invalid-input guards.

Green (`ci/run.sh go test -count=1 ./internal/store/ -run
EnforceEventRetention -v`): all three tests pass (0.19s-0.22s total across
runs).

Red proof: temporarily replaced the batch loop's condition with `false &&
...` in place (keeping the cutoff computation, so the change compiles) to
disable the deletion pass entirely while leaving the throttle bookkeeping
intact, then re-ran the same three tests:

```
=== RUN   TestEnforceEventRetentionDeletesOldOnesKeepingBoundaryAndRecentRows
    event_retention_test.go:78: old rows remaining after EnforceEventRetention = 1200, want 0 (all 1200 rows strictly older than the cutoff must be gone)
    event_retention_test.go:102: events remaining after EnforceEventRetention = 1251, want 51 (boundary + recent only)
--- FAIL: TestEnforceEventRetentionDeletesOldOnesKeepingBoundaryAndRecentRows (0.13s)
=== RUN   TestEnforceEventRetentionThrottlesToAtMostOnceAnHour
    event_retention_test.go:141: events after first (store-open) EnforceEventRetention call = 1, want 0
--- FAIL: TestEnforceEventRetentionThrottlesToAtMostOnceAnHour (0.02s)
=== RUN   TestEnforceEventRetentionRejectsNonPositiveWindow
--- PASS: TestEnforceEventRetentionRejectsNonPositiveWindow (0.02s)
FAIL
FAIL	github.com/n-orlov/deck/internal/store	0.180s
FAIL
```

(old rows survive with the deletion disabled -- exactly the failure mode
a one-sided "old rows gone" assertion alone would have missed if the
throttle bookkeeping had instead been the part removed.) The batch loop
was then restored and the suite re-run green.

`ci/run.sh go test -count=1 ./internal/config/ ./internal/tui/
./internal/store/` all `ok`; `ci/run.sh go build ./...` and `ci/run.sh go
vet ./...` both clean.

## Per-requirement evidence table

Every commit sha cited anywhere in this report (34 distinct shas, including
the two protected-path shas `6584299`/`791e089`) resolves in this repository
(`git cat-file -e <sha>`, checked while writing this table); every cited log
path exists under `docs/reports/` at the paths given (both checked
mechanically, not by eye).

| Req | Short description | Tasks | Key commits | Non-vacuous test evidence | Revert-red proof |
|---|---|---|---|---|---|
| R52 | New session auto-selects its row by id, one-shot | 301, 302 | `d7f304d`, `b34ea92` | `internal/tui/new_session_select_test.go` (5 tests); `features/new_session_selection.feature` (2 scenarios, tag `@new-session-selection`) | Build fails with 301's tui.go reverted (field undefined); both scenarios fail red with 301 reverted — [`phase3e-302-r52-godog/red-task301-reverted.log`](phase3e-302-r52-godog/red-task301-reverted.log) |
| R53 | `[ui] sort_order`: attention/created/activity/name, schema+comparators+render+live-apply+grouping | 303-310 | `cbf4eb5`,`893ff6e`,`bb18d5b`,`6a7e7ac`,`300bfb5`,`8f37a36`,`ae25f2a`,`9da97fe` | `internal/tui/sort_order_test.go`, `sort_order_render_test.go`, `sort_order_live_apply_test.go`; `features/sort_order.feature` (6 scenarios, tag `@sort-order`); the four differing sequences quoted in the R53 section above | Wrong-comparator stub red — [`phase3e-307-r53-orders/red-wrong-comparator-reverted.log`](phase3e-307-r53-orders/red-wrong-comparator-reverted.log); live-apply branch removed red (in commit message, `6a7e7ac`); grouping-preservation branch removed red (in commit message, `ae25f2a`) |
| R54 | Sidebar click re-targets interactive mode; already-target row is a true no-op | 313, 314 | `e6e1af3`, `d55f174` | `internal/tui/mouse_interactive_retarget_test.go` (3 tests); `features/mouse.feature` `@requirement-54-*` (2 scenarios) | `git stash` of 313 red — in `e6e1af3`'s commit message; both scenarios red with 313 reverted — in `d55f174`'s commit message (see also the R54 section's "known anomaly" paragraph above: the no-op scenario's red proof is coupled to running alongside the retargeting scenario in the same suite invocation, per task 314's `validationNotes`) |
| R55 | Single sidebar click both selects and enters interactive mode | 311, 312 | `8eba4d4`, `02a02cd` | `internal/tui/mouse_test.go` (rewritten); `features/mouse.feature` `@requirement-33-*` (2 rewritten scenarios) | `mouse.go`/`tui.go` reverted to `9da97fe` red — in `8eba4d4`'s commit message; reverted to `8eba4d4~1` red — in `02a02cd`'s commit message |
| R56 | Three further built-in themes (matrix, cobalt, parchment), one-file-drop contract | 315, 316, 317 | `a02168a`, `7559666`, `bb6afec`, `78043ee` | `internal/theme/quantize_test.go` `TestBuiltinQuantizationPinned` (3 new entries); `internal/tui/matrix_status_tokens_test.go` `TestMatrixStatusTokensRenderAsSevenDistinctColours`; `features/theme_geometry_test.go` still green (2.02s, 5 themes) | idle/starting hex-collision in `matrix.toml` red — in `78043ee`'s commit message |
| R57 | Seam + T-junctions follow either-panel-focused, not `previewBorderToken` alone | 318, 319 | `9c13b47`, `63c9e60` | `internal/tui/seam_border_test.go` (5 cases); `features/seam_focus.feature` (1 scenario, per-cell foreground token) | Old previewBorderToken-only logic red — in `9c13b47`'s commit message; feature-level red with `9c13b47` reverted — in `63c9e60`'s commit message |
| R58a | `truncateToWidth` closes an open background span it truncates through | 320 | `408a1b7` | `internal/tui/escape_background_test.go` (4 cases) | `git stash` of the fix, new test file kept untracked — in `408a1b7`'s commit message |
| R58b | Sidebar row highlight fills the panel's full inner width on both lines | 321 | `f74ed7f` | `internal/tui/sidebar_row_fill_test.go` (3 cases) | `sidebarContentLine`/`sidebarRowLines` reverted to ignore `bg` — in `f74ed7f`'s commit message |
| R58c | Every other padTrunc/truncateToWidth call site audited and fixed for bg leaks | 322 | `c86e422`, `f827581` | `internal/tui/panel_leak_audit_test.go` (3 cases); full 8-site enumeration in the R58c section above | 3 tests red with `c86e422`'s diff reverted in place — quoted verbatim in the R58c section above |
| R58d | Per-cell godog proof of the selection rectangle and the non-bleeding seam | 323 | `9079382` | `features/panel_background_rectangle.feature` (2 scenarios, per-cell background token steps) | `git revert --no-commit c86e422 f74ed7f 408a1b7` — 2 scenarios red, quoted verbatim in the R58d section above |
| R59 | `probe.miss` is no longer written as an event | 329 | (see task notes in `tasks.json`) | `internal/store` test per steer 3e-001 §6.2 | Fix reverted — event count increases; see the R59 section above |
| R60 | `events` gains `events_at`/`events_session_kind` indexes, applied via migration | 330 | `faba630` | `internal/store` migration + seeded 20k-row `EXPLAIN QUERY PLAN` test | Index dropped — plan assertion red; see the R60 section above |
| R61 | Event-log dialog reads the store once on open, not from `View()` | 331 | `b448d19` | `internal/tui/event_log_render_path_test.go` `TestEventLogViewNeverReReadsTheStoreOnceOpen` | `eventLogBody` reverted — read count grows to 11 across 5 ticks; see the R61 section above |
| R62 | Bounded events retention, `event_retention_days`, throttled batched deletion | 332 | `8d63481` | `internal/store/event_retention_test.go` (boundary + batching scale) | Deletion loop disabled — old rows survive; quoted verbatim in the R62 section above |
| — | `settings.feature` j-count/theme-cycle staleness from 303/315-316 | 333 | `3c212cf` | `features/settings.feature` `@requirement-17-*`/requirement-19 scenarios | Pre-fix red (16 scenarios, 14 passed, 2 failed) — in `3c212cf`'s commit message |

### Whole-suite and stability evidence

**Green whole-suite run (task 324, superseded — see task 407 below)**:
`ci/run.sh go test -p=1 -count=1 ./...` at commit `7ebafce`, every package
`ok` or `[no test files]`, `EXIT=0`. 1-min loadavg `1.73` → `2.72`. Full
root-cause narrative for the three pre-existing failures found and fixed
before this run (a settle-race in `selectSessionByNameThenSend`, a stale
schema-version literal, two stale R52 auto-select fixture assumptions, and
one isolated, host-load-only `mouse.feature` flake) is in
[`phase3e-fullsuite/README.md`](phase3e-fullsuite/README.md); raw log:
[`phase3e-fullsuite/go-test-p1-count1-all.log`](phase3e-fullsuite/go-test-p1-count1-all.log).

**Green whole-suite run at the final code commit (task 407, current)**:
`ci/run.sh go test -p=1 -count=1 ./...` at commit
`75861e0533bf6ea77bdfe4e72b25fa1566ab33e1` (`75861e0`) — the tree's tip
after tasks 401-411's repair pass, and the last commit to touch non-docs
code. Every package `ok` or `[no test files]`, `EXIT=0`, 17-line output.
Loadavg samples `3.25` → `2.40` → `3.16` → `2.82` across the run. Full
narrative and raw log:
[`phase3e-407-whole-suite-at-75861e0/README.md`](phase3e-407-whole-suite-at-75861e0/README.md),
[`phase3e-407-whole-suite-at-75861e0/go-test-p1-count1-all.log`](phase3e-407-whole-suite-at-75861e0/go-test-p1-count1-all.log).
This citation replaces the stale `7ebafce` one above as the current
whole-suite evidence; the `7ebafce` entry is kept for its own root-cause
narrative, not as live evidence of the current tree's state.

**10-run stability (task 325, superseded — see task 408 below)**:
`ci/stability.sh 10` at the same commit's descendant report sha `5ee9094`
— **7/10 passed**, published honestly (not re-run for a streak). Every one
of the four total failure instances across the ten runs is named and
root-caused in
[`phase3e-stability/README.md`](phase3e-stability/README.md): two instances
of a pre-existing, already-documented SIGWINCH-settle-window flake shared by
`mouse.feature`/`preview.feature` (host load, not a product bug — isolation-
proven 3/3 clean), and one instance (run 1) of a genuine test-synchronisation
bug in `internal/interactive`'s `TestSessionResizeDuringLiveDrainIsRaceFree`
(a drain goroutine that could still call `t.Fatalf` after the test function
had already returned under contention) — root-caused, fixed with a real
`sync.WaitGroup` join in commit `fb9bd71`, and reproduced deliberately via 8
concurrent `ci/run.sh` containers
([`phase3e-stability/redproof-interactive-goroutine-leak/`](phase3e-stability/redproof-interactive-goroutine-leak/)).
The published 7/10 rate at sha `5ee9094` is left unchanged (the fix landed
on a later commit and was not re-run against the original ten, per the
non-negotiable against re-running for a streak).

**10-run stability at the final code commit (task 408, current)**:
`ci/stability.sh 10` at commit `75861e0` — the same sha task 407 cited —
**7/10 passed**, published honestly (not re-run for a streak; the numeric
rate happens to match task 325's, but the commit and the failure instances
differ and are not assumed to be the same event). Both failure mechanisms
across the three failing runs (runs 4, 9, 10) are pre-existing and already
documented elsewhere, not new defects: `create_cwd_ghost.feature`'s
`shell`-agent `starting`→`running` fast-forward race (runs 4, 10; same
mechanism as
[`phase3d-210-ghost-completion-starting-race.md`](phase3d-210-ghost-completion-starting-race.md)'s
sibling scenario) and the pre-existing SIGWINCH-settle-window flake in
`preview.feature`'s `@steer-018-preview-fit-on-navigation` (run 9; same
mechanism as `phase3e-stability/README.md` item 1 above). No task-401-411
mechanism and no `internal/interactive` recurrence of task 325's own fixed
bug appeared in any of the ten runs. No code change was forced, so task 407
did not need to be redone. Full narrative and all ten per-run logs:
[`phase3e-408-stability-10-at-75861e0/README.md`](phase3e-408-stability-10-at-75861e0/README.md).
This citation replaces the `5ee9094` one above as the current stability
evidence; the `5ee9094` entry is kept for its own root-cause narrative
(the `fb9bd71` `internal/interactive` fix), not as live evidence of the
current tree's state.

### Cross-references not yet written at the time this table was filled in

- Task 327 (`docs/reports/phase3e-findings.md`) records every PRD claim
  found wrong, every SPEC ambiguity hit, and every defect found but not
  fixed — including the R56 `TestBuiltinsHaveOneDarkOneLight` pre-check
  quoted in this report's own "PRD spot-check corrections" note in
  `notes.md`, and task 314's recorded no-op-scenario coupling anomaly noted
  in the R54 section above.
- Task 328 (close-out) performs the protected-path-by-sha verification and
  the final clean-tree/pushed check; not run yet as of this table.

