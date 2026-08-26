# Task 014 — R73 (issue #7) help-overlay text + the two recorded judgement calls

Commit: `tui: name the overlay scroll bindings in the help overlay (task 014)` (see `git log`).
Requirement R73, GitHub issue #7. Leg 3 of 3 (leg 1 = task 012 line scroll, leg 2 = task 013 wheel routing).

## 1. What the help overlay now says

`helpText` (`internal/tui/tui.go`) gained six lines, in two places:

* the `↑/↓ or j/k` entry's tail — while `?` help, `E` the event log or `i` the detail view covers
  the list, "these same keys instead scroll that overlay's own content **by exactly one line per
  press**, leaving the session selection where it was";
* the `PgUp/PgDn` entry's tail — paging an overlay is "**a whole page per press, so consecutive
  pages share no line**; ↑/↓ or j/k there move one line at a time, and with mouse reporting on a
  wheel notch over the overlay does the same";
* a new Mouse-section entry — "`wheel over an overlay`  scroll ? help, E the event log or i the
  detail view by one line (like ↑/↓ or j/k); no other overlay scrolls, and a click or a drag over
  any overlay still does nothing".

Pinned through two existing tests (both read the live `helpText`, no fixture copy):
`TestEmptyAndHelpViewsAreDiscoverable` (`internal/tui/tui_test.go`, `View()` at 100x400) and
`TestDeckBinaryEmptyHelpAndQuitThroughPTY` (`cmd/deck/main_test.go`, the real PTY at 260x100).

### Would it go red if the text were reverted?

Yes — mutation captured in `task014-help-text-mutation.log`: with the six help lines removed and
everything else at HEAD, both tests fail naming all six phrases
(`help view missing "wheel over an overlay"`, `released help missing "pages share no line" through
the real PTY`, …). Restored afterwards; `git status --short` clean before the commit.

### Deliberate assertion/measurement updates (no golden frame pinned this text)

* `cmd/deck/main_test.go`'s PTY help window stays at **260 rows**: re-measured after the added
  lines, `helpView` renders **239** lines at Cols 100 (was 229), so 21 rows of headroom remain and
  bubbletea's top-cropping never engages. Measurement in `task014-help-height-probe.log`.
* The help overlay's own scroll extent moved (its content grew): at 80x24 `helpText` is 238 raw
  lines wrapping to **379** lines, content budget 22, `dialogMaxScroll` 357 (same log). No test
  asserts an absolute extent — `TestHelpOverlayScrollReachesEveryLine` presses PgDn 40 times
  (40 × 22 = 880 > 357, still reaches the bottom) and the R73 line-scroll/wheel tests compare rows
  relatively — so nothing needed weakening. Three stale "273 lines at 80x24" comments
  (`panel.go`, `tui.go`, `height_bound_test.go`) were refreshed to the re-measured 379;
  `help_keymap_parity_test.go`'s 273 is explicitly "measured against HEAD of this commit" and is
  left as the historical record it is.
* `help-keymap-parity` (`TestHelpOverlayKeymapMatchesBoundKeys`,
  `TestFooterKeyLegendNamesOnlyBoundKeys`) is unaffected by construction: it reads only each Keys
  entry's *leading* token(s), and all six added lines are continuation lines (4-space indent) or a
  Mouse-section entry. Green in the package run below.

## 2. Judgement call (a): PgUp/PgDn page overlap — **kept non-overlapping**

`dialogScrollByPage` (`internal/tui/panel.go`) steps by the whole `dialogContentBudget()`, so
consecutive pages share **no** line. R73 deliberately leaves that as task 078 shipped it: the new
one-line arrow/j/k step is exactly the "read across the page seam" affordance a one-line overlap
would have bought, so changing the page step would have been a second, redundant mechanism (and
would have made `PgDn` then `PgUp` no longer a round trip to the same offset). The decision is now
stated in the product's own help text ("a whole page per press, so consecutive pages share no
line") and in `dialogScrollByPage`'s doc comment, not only in a report.

## 3. Judgement call (b): can a `framedDialog` dialog overflow the frame? — **yes, three can**

`framedDialog` (unlike `framedDialogScrollable`) neither clips nor scrolls, and nothing above it
bounds the body. Checked by rendering each dialog at 80x24 and counting `View()`'s lines with a
throwaway probe (`task014-framed-overflow-probe_test.go.txt`, deleted before the commit; full
output in `task014-framed-overflow-probe.log`, loadavg 3.47 at the time):

```
env editor, 3 variables                    rendered  12 lines, frame height 24 -> fits
env editor, 15 variables                   rendered  24 lines, frame height 24 -> fits
env editor, 16 variables                   rendered  25 lines, frame height 24 -> OVERFLOWS
env editor, 24 variables                   rendered  33 lines, frame height 24 -> OVERFLOWS
env editor, 60 variables                   rendered  69 lines, frame height 24 -> OVERFLOWS
rename                                     rendered  14 lines, frame height 24 -> fits
create, untouched defaults                 rendered  29 lines, frame height 24 -> OVERFLOWS
create, every field filled plus an error   rendered  31 lines, frame height 24 -> OVERFLOWS
restart choice                             rendered  18 lines, frame height 24 -> fits
delete confirm, purge chosen with a path   rendered  17 lines, frame height 24 -> fits
bulk delete confirm, 20 marked             rendered  32 lines, frame height 24 -> OVERFLOWS
archive confirm, not stopped               rendered  21 lines, frame height 24 -> fits
profile switch                             rendered  12 lines, frame height 24 -> fits
pin                                        rendered  14 lines, frame height 24 -> fits
settings takeover                          rendered  24 lines, frame height 24 -> fits
```

Findings (recorded, **not** fixed — R73's criteria put a fix out of scope unless trivial, and it is
not: each of the three needs a stored scroll offset, key/wheel routing, and — for the env editor and
create dialog — reconciliation with their own cursor/field navigation, i.e. leg-1/leg-2-sized work
apiece):

* **`e` env editor overflows from 16 resolved keys up** (one row per key; threshold measured exactly
  between 15 → 24 lines and 16 → 25 lines at 80x24). The likeliest of the three to be hit in real
  use — a config `[env]` table plus a session env map easily exceeds 15 keys.
* **`n` create dialog overflows at 80x24 even untouched** (29 lines), i.e. deck's documented minimum
  terminal cannot show the whole create modal at all; filling every field and provoking a validation
  error takes it to 31.
* **bulk `dd` confirm overflows once the mark set is large** (20 marks → 32 lines; it prints one line
  per marked session, so ~13 marks is the threshold at 80x24).

The `settings` takeover is not a `framedDialog` at all (`settingsView` bounds itself to
`frameSize()` minus borders/footer) and measured exactly 24 lines; `rename`, `restart-choice`,
`delete-confirm` (single, purge chosen, longest path) and `archive-confirm` all fit with room to
spare. These three overflows belong in `docs/reports/phase3f-findings.md` (task 024) and are
referenced from `framedDialogScrollable`'s own doc comment, whose previous "unlike every other
§11.4 dialog -- bounded by its own field count" claim this check disproved and which is now
corrected in place.

## 4. Test evidence

* `ci/run.sh go test -count=1 ./internal/tui/` → `ok` (0.68s).
* `ci/run.sh go test -count=1 -v -run TestDeckBinaryEmptyHelpAndQuitThroughPTY ./cmd/deck/` → `PASS`
  (0.85s, real PTY, not skipped).
* `ci/run.sh go test -p=1 -count=1 ./internal/tui/ ./features/` → `task014-criteria-suite.log`.
* loadavg recorded in each log.
