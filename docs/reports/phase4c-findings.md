# Phase 4c findings (cure-01-08)

Companion to [`phase4c.md`](phase4c.md) (the per-requirement evidence report
for R136-R139): what that report does not carry — the review-raised
behavioural findings this cure wave fixed, where each fix now lives in the
tree (`file:line`), the two known advisory flake classes, and the
iterations 48-54 infrastructure stall. Re-taken a third time
(`retake-01-01-08`) at the tree `cure-01-01-4` leaves; the final code sha is
now `be7cdbc996b356a2b17ad31a4b7e095e0a98c98a` (task 015's `ci/run.sh`
DECK_*-forwarding fix — confirmed via `git diff-tree --no-commit-id
--name-only -r be7cdbc`, which lists only `ci/run.sh`, outside `docs/`, so
it changes no `file:line` below). `cure-01-01-4` (`2d282e23b059793647b29`
`951c0f97335c8b065c3`) landed between the previous retake and this one — the
fix that makes the list-mode `esc` branch re-anchor a row cursor on the
cleared filter's session id and unconditionally follow the viewport, per
binding ruling 002. Every commit after `2d282e2` up to `HEAD` touches only
`docs/` (`6051c30`, `7d7f4f7`, `40bd883`, this file's own commit, confirmed
via `git show --stat --format=''`), so every `file:line` below resolves
identically at `2d282e2`/`be7cdbc` and at this file's own commit. The
previous retake (against `610be00`, before `cure-01-01-4` landed) is
superseded; this pass adds a new F0-3 sub-finding for `cure-01-01-4` itself
and re-verifies every other `file:line` at the current line numbers (they
shifted: `cure-01-01-4` inserted 31 net lines into `internal/tui/tui.go`
ahead of the F2 handlers and everything below them).

## 1. Retained findings — review-raised, cured in place

F1-F8 below were each filed as a blocking finding against the original
phase 4c landing (`/run/ralphd/review-findings.json`, iteration 64) and
cured in a dedicated `cure-01-0N` commit under operator ruling 001. F0 is
different — a later sweep over this cure wave's own tree, not the
iteration-64 review, found two more escapes of the same reload-selection
seam plus a rejected footer deliverable; both were cured the same way
(in place, on this tree) and are recorded here for the same reason: this
file is where a fix's `file:line` lives. F1-F8 are all
`curable: true` and none was waived — see `phase4c.md` for the
fail-before test text and probe citations; this section names only where
the fix itself now lives.

### F0 (sweep-found, not in the original review-findings.json) — a reload could still land selection on a hidden row or an absent header, and the header-promotion rule over-fired

Two more escapes of the same R136/R137 reload-selection seam F6 and task
022's own sweep cures already narrowed, found by a later sweep over this
cure wave's own tree and fixed in `cure-01-01-2` (`3d058b5`):
`TestReview113NewSessionIntentCannotSelectFoldedRow`,
`TestReview113ArchivedReloadCannotLeaveAbsentHeader`,
`TestReview113ArchivedReloadCannotSelectHiddenRow` and
`TestReview113ExplicitHeaderSurvivesBackgroundArrival` were all red before
the fix (`artifacts/review113/hidden-stop-probes.log`), and separately,
task 015's `10c5021` fixed a review rejection of task 015's own original
footer deliverable (the list footer named none of the header cursor's
fold keys, and the detail-dialog footer naming them was unreachable since
`i` is inert on a header):

- `internal/tui/tui.go:2519` (`selectedGroupHadNoRows` capture) through
  `internal/tui/tui.go:2612` — `selectVisibleStopAfterReload`'s
  header-gains-its-first-row promotion is gated on `hadNoSessionsAtAll`
  (the whole sidebar had zero sessions before the reload), not merely the
  selected header's own bucket, so a header the user deliberately
  navigated to while other sessions were already on screen survives a
  background arrival.
- `internal/tui/tui.go:2626` — `sessionsLoaded`'s `pendingSelectSessionID`
  one-shot override now unfolds the new session's own group first when it
  is collapsed, before selecting the row.
- `internal/tui/tui.go:2678` (selection preserve-by-id) and
  `internal/tui/tui.go:2685` (`selectVisibleStopAfterReload(false)`) —
  `archivedSessionsLoaded` now runs the same preserve-by-id/normalize dance
  `sessionsLoaded` already does, instead of only clamping a raw row index.
- `internal/tui/tui.go:5105` (wired into `footerLineContent`'s SPEC §11.3
  status-reason slot) and `internal/tui/tui.go:6581`
  (`headerCursorFooterCue`'s own definition): while the cursor is
  on a group header the LIST footer (the only surface on screen at that
  point) now names `c folds/unfolds it · ← folds · → unfolds`. Fail-before
  at `3d058b5` (`artifacts/task-015-list-footer-fail-before.log`),
  `TestListFooterNamesTheHeaderCursorFoldKeys`.

### F0-2 (sweep-found, cure-01-01-3) — the header-promotion rule still could not tell a deliberate navigation from its own automatic case

`cure-01-01-2`'s `hadNoSessionsAtAll` heuristic still could not tell an
explicitly navigated header on a header-only sidebar apart from the
automatic zero-value-cursor promotion it exists for
(`TestCure0105FirstSessionUnderHeaderOnlyLoadFollowsSelection`): both look
identical from the pre-reload state alone (whole sidebar header-only, the
selected header's own bucket empty). Fixed in `cure-01-01-3` (`610be00`);
fail-before HEAD `10c5021`, review's own transitions log
(`artifacts/review132/transitions.log`): "background arrival stole
explicitly navigated header on header-only sidebar: {1 0 2} -> {0 0 0};
pending=\"\"", `TestReview132ExplicitHeaderOnEmptySidebarSurvivesBackgroundArrival`
(committed as `TestCure010103ExplicitHeaderOnEmptySidebarSurvivesBackgroundArrival`,
`internal/tui/cure_01_01_3_header_cursor_test.go:22`):

- `internal/tui/tui.go:365`-`384` — new `Model.selectedByUser` field, set
  the moment `setSelection` runs (the ONE seam every deliberate selection
  gesture — up/down, PgUp/PgDn, space, `c`/left/right, `g`/`G`, both `/`
  filter paths, and a sidebar mouse click — assigns `m.selected` through)
  and never cleared again; never persisted (in-memory `Model` field only).
- `internal/tui/tui.go:5915`-`5918` — `setSelection` now sets
  `m.selectedByUser = true` alongside the existing `m.selected = c` and
  `m.followSelectionViewport()` calls.
- `internal/tui/tui.go:2612` — `sessionsLoaded`'s promotion call is now
  gated on `!m.selectedByUser` too, so it fires only for the genuinely
  automatic case.

### F0-3 (sweep-found, cure-01-01-4) — the list-mode Esc that clears a held filter neither preserved selection identity nor followed the viewport

Review 154 re-ran the accepted R152-2 counterexamples at `610be00` and found
the list-mode `esc` branch (`tui.go`'s top-level `"esc"` case, reached once
`Enter` has returned focus to the list) still only normalized the cleared
selection onto SOME visible stop by numeric position via
`nearestVisibleSelection`, and never called `followSelectionViewport` at
all: `TestReview152ClosedFilterEscapeKeepsSelectionVisible` and
`TestReview152HeldFilterEscapeKeepsHeaderVisible` both FAIL on `610be00`
with the selection span outside the scroll window
(`/run/ralphd/artifacts/review154/filter-escape.log`: "list Esc clearing
held filter with header cursor: sidebarScroll = 0 leaves selection span
[30,30] outside window [0,21)" and "...span [31,32] outside window
[0,21)"). Binding ruling 002 also requires the SAME session id survive,
not merely a row at the same position — unfiltering can insert an
earlier-sorting nonmatch back before the kept session, shifting its
index — and `TestReview154HeldFilterEscapePreservesSessionID` FAILS on
`610be00` with kept-id -> other-id
(`/run/ralphd/artifacts/review154/held-filter-identity.log`: "held-filter
Esc changed selected session ID: kept-id -> other-id (raw index=0)").
Fixed in `cure-01-01-4` (`2d282e2`):

- `internal/tui/tui.go:3592`-`3628` — the `"esc"` case now captures the
  selected session's id (row cursor only; a header cursor's group id never
  goes stale this way) before clearing `filterQuery`, re-anchors on that
  id in the new unfiltered list, falls back to `nearestVisibleSelection`
  only when the cursor no longer names a visible stop, and then calls
  `m.followSelectionViewport()` unconditionally — instead of handing the
  old numeric index straight to `nearestVisibleSelection` and never
  following the viewport at all.
  `TestEscOnAMarkedSetClearsOnlyTheMarksNotAHeldFilter` (the one-layer
  marks-only-clear control) is unaffected and still passes.

### F1 — session-scoped keys were not unconditionally inert on a header

`x`/`dd` bypassed the header guard whenever a mark was already set, and
detail-mode `r`/`l` never asked the shared guard at all (`len(m.sessions)
> 0` says nothing about what the cursor names). Cured in `cdce2641`:

- `internal/tui/rename.go:71` — detail `r` (rename) now calls
  `m.guardSessionScopedKey("r")` before touching a session.
- `internal/tui/rename.go:86` — detail `l` (launch-inputs editor) now
  calls `m.guardSessionScopedKey("l")` the same way.
- `internal/tui/session_scoped_guard.go:69` — `guardSessionScopedKey`
  itself, with the marked-batch exemption removed so `x`/`dd` are inert
  on a header regardless of the mark set.

### F2 — an empty or structural-default group with zero total sessions could not be folded

`c`/left/right on a header required `len(m.sessions) > 0` even though
`cursorGroupID` resolves a header cursor's own id with no session lookup.
Cured in `3529eb90`:

- `internal/tui/tui.go:4139` (`c`), `internal/tui/tui.go:4177` (`left`),
  `internal/tui/tui.go:4194` (`right`) — the stale `len(m.sessions) > 0`
  guard is removed from all three handlers; `!m.help && !m.detail` is
  unchanged.

### F3 — a header cursor painted no visible selection cue

Moving the cursor between two headers left every fully-painted sidebar
line byte-identical (text, gutter, background) — no cue at all, in colour
or monochrome. Cured in `e23bc499`:

- `internal/tui/group.go:650` — `headerSelectionCue` composes a header's
  own gutter/background through the same `sidebarGutterBar`/
  `sidebarSelectionToken` a row's own cue already uses, so a header reads
  as selected via its glyph even under `NO_COLOR`.

### F6 — the re-sort/re-group seam could strand the cursor off-screen or on a hidden stop

Saving `default_group_first` moved the selected row/header cursor's
rendered entry span without re-clamping the scroll offset; a fresh
session load after restart, a rename, or a filter query change could also
normalize onto a hidden or absent stop. Cured in `4e30475c`:

- `internal/tui/settings.go:517` — `settingsApplyLiveFields`'s
  `default_group_first` branch now calls `m.followSelectionViewport()`
  after flipping the flag.
- `internal/tui/tui.go:2646` — `sessionsLoaded`'s reload path calls the
  same `m.followSelectionViewport()` once the preserved/normalized cursor
  is resolved.

### F7 — folding the interactive session's own group dropped its name from the preview border

`previewTitle` resolved the interactive session's name off the CURSOR
(`m.selectedSession()`), and folding that session's own group retargets a
row cursor onto the group's header — silently blanking the border title
while the pane still held the keyboard. Cured in `c864e6b9` (unit
fixture) and `5da35c55` (live-PTY feature scenario):

- `internal/tui/tui.go:5963` — new `interactiveTargetSession`, keyed off
  `m.interactiveWindowTarget` rather than the cursor.
- `internal/tui/tui.go:6399` — `previewTitle` now prefers
  `interactiveTargetSession` over `m.selectedSession()`.

### F8 (residual) — probe-report wording overstated a compile failure and cited the wrong issue

`phase4c-probes/r139.md`'s probe 1 disposition treated a build failure
(the test's 3-arg call needing the `defaultFirst` parameter) as itself
proof the ordering *behaviour* was absent, conflating compile-time API
evidence with runtime behaviour evidence; `r136.md`'s heading cited GH #33
instead of #31. Cured in `2bb61a8b` (docs-only, touches only
`docs/reports/phase4c-probes/`):

- `docs/reports/phase4c-probes/r139.md` — a compiling order-regression
  probe (old default-last body, intact signature) and its quoted runtime
  failure were added alongside the compile-time note.
- `docs/reports/phase4c-probes/r136.md:1` — heading now cites GH #31.

## 2. Evidence findings — closed by dedicated record tasks, not repeated here

F4 (ten-run stability sweep) and F5a/F5b/F5c (phase report, this findings
file plus the DELIVERY-LOG row, and final-code guards) were findings
against *missing evidence*, not product behaviour — `curable: true`,
"curable in place, never grounds for a replan" per ruling 001. Each has
its own carrier task and its own record: `docs/reports/phase4c-stability10/`
(cure-01-06), `docs/reports/phase4c.md` (cure-01-07), this file plus the
DELIVERY-LOG row below (cure-01-08), and `docs/reports/phase4c-guards/`
(cure-01-09). They are not re-litigated here.

## 3. Known flake classes — advisory

The PRD names two flake classes as advisory: a transient-`starting`
assertion and a `SIGWINCH` exact-count assertion. Neither recurred in the
committed sweep: `docs/reports/phase4c-stability10/README.md` records
10/10 PASS across `run-1.log`..`run-10.log`, and `grep -n '^--- FAIL\|^FAIL'`
across all ten committed logs returns nothing — there is no log path to
cite for either class this time, since neither one fired. Both remain
advisory in general per the PRD; a future sweep that does hit one should
publish it with its log path rather than re-run to chase a clean headline.

## 4. Infrastructure — the iterations 48-54 docker-socket stall

Across iterations 48-54 of this run, `/var/run/docker.sock` was absent
from the job container, making `ci/run.sh` (and therefore every CI-lane
command, including validation) unreachable for roughly 73 minutes. This
was a harness/infrastructure condition, not a defect in the product under
test — recorded here per operator ruling 001 with no `file:line`, since
nothing in the tree caused or fixed it. The socket was present again for
every command this cure wave actually ran on the CI lane, including both
the whole-suite gate re-run (`task 022`) and the ten-run stability sweep
(`cure-01-06`): `ls -la /var/run/docker.sock` and `ci/run.sh go version`
were both re-checked live before each, per the standing "re-check, then
proceed" rule, and neither sweep nor gate encountered the outage.

## 5. How to re-check every citation in this file

```
git show --stat --format='' 2d282e23b059793647b29951c0f97335c8b065c3
git diff --stat be7cdbc996b356a2b17ad31a4b7e095e0a98c98a..HEAD -- '*.go' '*.feature'   # empty
sed -n '69,90p'   internal/tui/rename.go
sed -n '360,400p'  internal/tui/tui.go
sed -n '2495,2690p' internal/tui/tui.go
sed -n '3560,3630p' internal/tui/tui.go
sed -n '4139,4202p' internal/tui/tui.go
sed -n '640,660p'   internal/tui/group.go
sed -n '505,520p'   internal/tui/settings.go
sed -n '5910,5970p' internal/tui/tui.go
sed -n '6395,6405p' internal/tui/tui.go
sed -n '5100,5107p' internal/tui/tui.go
sed -n '6550,6585p' internal/tui/tui.go
grep -n '^--- FAIL\|^FAIL' docs/reports/phase4c-stability10/run-*.log   # empty
```
