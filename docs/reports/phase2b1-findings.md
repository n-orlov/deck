# Phase 2b-1 findings

This file accumulates findings across Phase 2b-1's tasks. It is written to
incrementally by task 013 (below) and by later tasks (018, 020, 023, 024, 033,
034 in `SPEC.md`/PRD's own numbering); each task adds its own dated section
rather than overwriting earlier ones.

## Task 013: re-aiming negative assertions whose copy moved to the footer

### The sweep

Task 012 moved a session row's status *reason* text (` · resumable`,
`starting · awaiting signal`) off the row and onto the footer
(`Model.selectedRowReason()`, `internal/tui/tui.go`). A repo-wide sweep for
negative screen-text assertions was run to find every assertion at risk of
going vacuous because the copy it polices moved surfaces:

```
grep -rn "does not contain" features/*.feature
grep -rn "!strings.Contains(view\|!strings.Contains(frame\|!strings.Contains(detail\|strings.Contains(view, \"" \
  internal/tui/*_test.go internal/tui/*.go features/*.go cmd/deck/*_test.go
```

Findings, by site:

| Site | Copy | Scope of the check | At risk from task 012? |
| --- | --- | --- | --- |
| `features/shell_liveness.feature:10` | `awaiting signal` | `deck client "A" screen does not contain ...` — whole rendered frame | Yes — this is the copy that moved. |
| `features/launch_lease.feature:20,32,46,57` | `starting elsewhere` | `deck client "A" screen does not contain ...` — whole rendered frame | Named directly in the PRD table for this task. |
| `features/lease_race.feature:30-32` | `running` | `row "unsignalled agent" does not contain ...` — scoped to one row | No — `running` is a bare status word, never a reason; still lives on the row. |
| `features/status_attach.feature:30` | `!` (unseen marker) | row-scoped | No — unrelated glyph, not reason copy. |
| `features/status_probe.feature:55` | `sampled` | row-scoped | No — a verdict-source badge, not reason copy, never moved. |
| `features/permission_modes.feature:55` | `yolo (left/right cycles` | whole-frame, inside the create modal | No — create-modal copy, untouched by task 012. |
| `features/agent_session.feature:15,30`, `features/durable_identity.feature:40`, `features/permission_modes.feature:25,35`, `features/same_directory.feature:21` | `--continue`, `--dangerously`, `--approve`, a conversation id | audit-log argv, not screen text at all | No — out of scope for a screen-text sweep. |
| `internal/tui/resume_test.go:126`, `internal/tui/tui_test.go:80,103` | `starting elsewhere`, `awaiting signal` | Go view-model unit tests | Already re-aimed by task 012 itself (selects the row, checks the footer). |
| `internal/tui/profile_switch_test.go:207`, `internal/tui/badge_detail_test.go:66` | `yolo`, `Permission profile: safe` | Go view-model unit tests | No — unrelated copy (permission-profile badges), never moved. |

Only the six feature-file sites in the first two rows carry copy that task 012
relocated. Everything else in the sweep either polices a bare status word or
badge that always lived on the row, or copy from an unrelated subsystem
(create modal, permission profiles, audit log) that task 012 never touched.
No finding beyond these six sites required re-aiming.

### Why these six sites were already non-vacuous, and the injection proof

All six sites use the `deck client "A" screen does not contain "..."` step
(`clientScreenDoesNotContain`, `features/agent_steps_test.go:857`), which
checks `client.Frame(false)` — the *entire* rendered frame, footer included —
not a single row. Because the check was already whole-frame, relocating the
reason text from the row to the footer did not by itself make these
assertions vacuous: the footer was always inside the checked surface.

This was verified empirically rather than assumed, by deliberately injecting
each forbidden string into the footer and confirming the existing step still
fails:

1. **`starting elsewhere` (`launch_lease.feature`).** Temporarily removed the
   `m.resumeNote = ""` reset on a successful, non-conflicting resume in
   `internal/tui/tui.go`'s `sessionResumed` handler, so `resumeNote` would
   stick after a lease was cleared and the resume actually ran. Running only
   `@launch-lease` (`DECK_GODOG_TAGS="@launch-lease" go test -run TestFeatures
   ./features/...`) then failed exactly the expected step:
   `Scenario: a live in-TTL lease blocks a second launch and the row stays
   usable` → `And deck client "A" screen does not contain "starting
   elsewhere"` → `Error: ... screen unexpectedly contains "starting
   elsewhere"`. The other three `launch_lease` scenarios, which never take
   the "successful resume after a lease was live" path, were unaffected and
   still passed. The edit was reverted (`git diff --stat` clean afterwards).

2. **`awaiting signal` (`shell_liveness.feature`).** Temporarily made
   `Model.selectedRowReason()` unconditionally return `"starting · awaiting
   signal"` regardless of the selected session's actual status or agent.
   Running only `@shell-liveness` then failed both scenarios: `a live shell
   is promoted within one reconcile interval` on `screen does not contain
   "awaiting signal"`, and `a live unsignalled agent remains starting` on
   `screen still contains "starting - awaiting signal"` (it now saw the
   glyph-joined `·` form injected regardless of `DECK_ASCII`, which the ASCII
   dash-joined assertion no longer matched either — a second, incidental
   confirmation that the check is exact-substring and not merely "some status
   text present"). The edit was reverted (`git diff --stat` clean
   afterwards).

Both experiments demonstrate the step function inspects the whole frame, so
a regression that leaked the moved reason copy back onto an unselected
surface, held it open longer than it should, or otherwise let it linger would
be caught by the existing assertions exactly as before task 012. No feature
file needed rewording; the sweep's conclusion is that these six sites were
already correctly scoped and remain non-vacuous, and the proof above is the
evidence for that claim rather than an assumption.

### Suite status

`ci/run.sh go test -p=1 -count=1 ./...` is green after this task (no
production or feature-file changes were made — only the temporary,
fully-reverted injections described above, confirmed reverted via `git diff
--stat` showing no changes to `internal/tui/tui.go` before the final test
run).

## Task 018: cropping the pane bottom-left, real geometry stated, right-cut lines marked

### The marker

A line cut at the right edge has its **last visible column replaced** (never
appended past `contentWidth`) with `Model.cropMarker()`: `»` in the default
glyph set, `>` under `DECK_ASCII` (`internal/tui/panel.go`). It is
deliberately a distinct helper from `ellipsis()` (`…`/`...`) even though both
resolve through the same `m.glyph` pattern: `ellipsis()` truncates *deck's
own* copy (labels, session names) and always leaves room to render itself in
full; the crop marker truncates *foreign pane output* deck did not write and
is a single substituted column, never an inserted run of characters, so the
panel's own border always lands in the same screen column regardless of the
crop offset (verified for the wide-cell case in task 019).

### The geometry line's position

SPEC requirement 23's `45×22 of 120×40` line (rendered as `45x22 of 120x40`
— ASCII `x`, matching every other glyph in this panel's own copy, not the
agent's) is the **first line of the preview panel's content**, and is
rendered **only when the real pane is larger than the panel in width or
height** (`realWidth > contentWidth || realHeight > contentHeight`,
`Model.cropPreviewBottomLeft`, `internal/tui/panel.go`). A pane that fits
entirely carries no geometry line, since the user is then looking at the
whole pane and there is no window onto a larger one to name. Reserving the
top content row for it shrinks the visible "newest rows" window by exactly
one row when it is shown; the alternative (a status line below the content)
was rejected because it would be the *first* thing scrolled off when the
content itself is tall, defeating the point of stating the real geometry
whenever a crop is in effect.

### Anchoring

"Bottom-left, newest rows" (SPEC requirement 23) is implemented as: take the
last `avail` rows of the capture (`avail = contentHeight`, minus one when the
geometry line is shown) when the capture has more rows than `avail`
(vertical crop), and pad with blank rows *after* the real content when it
has fewer (so a pane shorter than the panel is top-anchored with blank space
below it, never stretched to fill the gap — SPEC's own "never stretched"
requirement, verified directly against task 008's `fitting.txt` fixture).
Every row is cropped to `contentWidth` columns from column one (horizontal
crop is always left-anchored; SPEC states no horizontal anchoring choice).

### Real geometry, not inferred from content

The real pane geometry comes from tmux's own `pane_width`/`pane_height`
(`internal/tmux.Pane.Width`/`Height`, sourced via `list-panes -F
"...|#{pane_width}|#{pane_height}"` and threaded through
`PreviewCapture.Width`/`Height`), not from counting non-blank bytes in the
capture. tmux trims trailing blank lines/columns from a `capture-pane`
reply, so a capture with fewer non-blank rows than the pane's real height
still belongs to a pane whose declared geometry may be larger — the
geometry line's "of WxH" half is a fact about the pane, never a guess from
what happened to be printed.

### Suite status

New unit tests: `internal/tui/preview_crop_test.go`
(`TestCropPreviewBottomLeftFitsWithoutGeometryLine`,
`TestCropPreviewBottomLeftCropsOversizedPane`,
`TestCropPreviewBottomLeftClampsToShortHistory`,
`TestCropPreviewBottomLeftZeroSize`), exercising task 008's checked-in
`fitting.txt` and `oversized.txt` fixtures directly (the latter at exactly
SPEC's own `45x22 of 120x40` example panel size). `ci/run.sh go test -p=1
-count=1 ./...` was green on every run but one: a single run hit
`features/status_probe.feature`'s "Stale sampling is visible..." scenario
failing on `session "raced claude" has 0 "probe.waiting" events`, an
assertion this task's changes never touch (it is about hook-vs-probe
timestamp precedence, unrelated to the preview panel). Four immediate
full-suite reruns afterwards (`go test ./features/... -count=3`, then two
more full `./...` passes) were all green, consistent with pre-existing
timing sensitivity in that scenario's frozen-clock race rather than a
regression from this task.

## Task 019: cell-aware crop and elision (SPEC requirement 24)

### Width model

Added `cellWidth`/`stringWidth` (thin wrappers over
`github.com/mattn/go-runewidth`, already an indirect dependency via
bubbletea/lipgloss and now promoted to a direct `require` in go.mod) and
two builders, `truncateToWidth`/`padToWidth` (`internal/tui/panel.go`):
`truncateToWidth` walks runes accumulating *display* width and stops
**before** a rune that would only partially fit in the remaining budget —
that rune is dropped whole, never emitted half — and `padToWidth` fills the
remainder with single-column spaces. Every caller that used to reason in
rune count (`padTrunc`, `cropPreviewBottomLeft`'s marker substitution) now
reasons in cells, so a result's `stringWidth(...)` is always exactly the
requested width, which is what keeps the panel border landing in the same
column regardless of what glyphs preceded it.

### `padTrunc` (session-name elision)

Unchanged contract (pad-or-truncate to exactly `width` columns, ellipsis
when there is room), but "columns" is now `stringWidth`, not `len([]rune)`.
When truncating, the ellipsis's own width is reserved first (`ellW =
stringWidth(m.ellipsis())`, 1 for `…`/3 for `...`), the remaining budget is
filled via `truncateToWidth`/`padToWidth`, and the ellipsis is appended —
so a session name containing East-Asian-Wide characters elides on whole
glyphs and the sidebar's right-hand seam still lands in the same column as
an all-ASCII name would.

### `cropPreviewBottomLeft` (preview crop)

Extracted the per-row logic into `cropRow(row string, contentWidth int)
string`: a row that already fits (`stringWidth(row) <= contentWidth`) is
only padded; an overflowing row reserves the marker's own width
(`markerW`, 1 for `»`/`>`), truncates the content into the remaining budget
with `truncateToWidth` (never mid-glyph), pads that to the budget, then
appends the marker as a fresh, whole column — the marker itself can now
never land as the right half of what used to be a wide glyph, and a wide
glyph that would have straddled the old truncation point is dropped in
full rather than showing its left column alone.

### Evidence

`internal/tui/crop_wide_test.go` (new): `TestCropRowNeverSplitsWideGlyph`
runs task 008's `wide.txt` fixture's mixed 界/┌ (width-2/width-1
alternating) line through `cropRow` at every `contentWidth` from 1 to 22
and asserts the output's display width equals `contentWidth` exactly at
every offset (the "border stays in the same column for every crop offset"
assertion the task calls for) plus that the rune-width sum matches (no
fractional glyph). `TestCropPreviewBottomLeftWideFixtureKeepsBorderColumn`
repeats the same check through the full `cropPreviewBottomLeft` entry
point across all three of `wide.txt`'s distinct lines at several panel
widths. `TestPadTruncElidesWideSessionNameWithoutSplitting` covers the
session-name half with a 10-glyph all-wide name at every width from 1 to
22. All of task 018's pre-existing crop tests still pass unchanged (their
fixtures are ASCII, where rune count and display width coincide, so the
switch to cell-based accounting is invisible to them).

`ci/run.sh go build ./...`, `go vet ./...` and `go test -p=1 -count=1
./...` all green. One `TestFeatures` run hit a pre-existing, unrelated
flake in `crash.feature`'s "a different hook detects a crash while no TUI
is running" scenario (a private tmux session lingering past its expected
teardown, a hook-timing race unrelated to preview cropping); an immediate
rerun of `./features/...` alone and a second full `./...` run afterwards
were both green.

### go.mod change

`github.com/mattn/go-runewidth` moved from an `// indirect` entry to the
main `require` block (still v0.0.23, no version bump) since `internal/tui`
now imports it directly; `go mod tidy` made no other change to go.mod or
go.sum.

## Task 020: rows with no live pane (SPEC requirement 26)

`internal/tui/tui.go`'s `previewBodyLines` used to render one generic "No
live preview captured for this row yet." message for every non-live
selection, regardless of *why* there was nothing live. Task 020 splits that
into a `previewPlaceholderLines` dispatch on `session.Status`, each branch
naming the state rather than leaving the viewer to guess:

### Placeholder copy (recorded verbatim, per successCriteria)

- `error`: header `"Last output before exit — not live:"` (ASCII fallback
  `"Last output before exit - not live:"` via the existing `m.glyph`
  convention), followed by the session's stored §7 `crash_tail` wrapped to
  the panel width. A tail longer than the available body rows is anchored
  to its **newest** (last) lines — mirroring requirement 23's own
  bottom-left crop anchoring — via the new `crashTailPreviewLines` helper.
  An error row with an empty crash tail still gets the header plus
  `"No crash output was captured."`, so it is never mistaken for a live
  but blank pane.
- `stopped`: `"Session is stopped. No live preview to show."`
- `archived`: `"Session is archived. No live preview to show."` (the
  `archived_at` flag/status distinction from SPEC §4 is not yet wired to
  any session in this phase — see below — so this branch is dispatched on
  a literal `Status == "archived"` string, exercised directly by a unit
  test constructing such a session, ready for whichever later phase
  actually sets it.)
- `starting` (no pane yet): `"Session is starting; no pane yet."` Reached
  only when the most recent capture attempt for that exact session id
  found no live pane (or none has been attempted yet), so a `starting`
  shell agent that already has a pane still gets the ordinary live crop,
  never this placeholder.
- Any other status reaching the placeholder branch (e.g. `running`/
  `waiting`/`idle` on the very first tick right after selection, before a
  capture has landed) keeps the pre-020 CWD-plus-notice copy; this was out
  of task 020's four named cases.

### `archived` reachability

Session structs have no `ArchivedAt`/status-flag field surfaced yet (the
schema column exists — `archived_at INTEGER NOT NULL DEFAULT 0` — but
nothing in `internal/store` reads or writes it; archive/reap/purge is
explicitly out of scope for phase 2b-1 per the PRD). Task 020's placeholder
is therefore unit-tested by constructing a `store.Session{Status:
"archived"}` directly rather than through any reachable UI path — the
dispatch is ready the day a later phase starts setting that status/flag,
but nothing in this phase can currently produce it live.

### Evidence

`internal/tui/preview_placeholder_test.go` (new):
`TestPreviewBodyLinesNoLivePaneStates` covers all four named states
end-to-end through `previewBodyLines`, asserting each body mentions its
state and never contains the old generic copy.
`TestCrashTailPreviewLinesAnchorsToNewestLines` proves the bottom-anchoring
under a tight height budget; `TestCrashTailPreviewLinesEmptyTail` covers
the empty-tail case. `ci/run.sh go build ./...`, `go vet ./...` and
`go test -p=1 -count=1 ./...` all green.

## Task 021: preview suppression below its floor (SPEC requirements 25, 27)

`internal/tui/layout.go`'s `ComputeLayout` gained a `PreviewShown bool`
field on `LayoutResult`:

- **Side-by-side**: always `true`. `ClampSidebarWidth` already keeps
  `Preview.Width` at or above the 40-column floor whenever side-by-side is
  the *effective* mode at all — `autoMode` only chooses it at width ≥ 80,
  and a pinned side-by-side that cannot hold 24+40=64 columns is already
  redirected to `autoMode` before the switch that sets `PreviewShown` runs.
  There is no width at which side-by-side is rendered with its preview
  below the floor, so this is a documented invariant, not a live branch.
- **Collapsed**: always `true`. The strip's own preview has no floor
  (§11.2: "the collapsed strip has no floor to violate at any width ≥ its
  own 3 columns" — see `layout.go`'s existing comment on the stacked/
  collapsed case). Requirement 25 names only the 40-column and 8-row
  floors, both belonging to side-by-side and stacked.
- **Stacked**: the only mode where suppression is live. When
  `rows - stackedListHeight(rows) < StackedPreviewFloor (8)`, the list
  height is widened to the full `rows` (the sidebar/list "takes the
  space") and `Preview.Height` is forced to 0; `PreviewShown` is `false`.
  Boundary: `rows=13` → preview height exactly 8 → shown; `rows=12` →
  preview height 7 → suppressed. `renderStackedFrame` already only draws
  the preview panel when `ph >= 2` (pre-existing task-014 code), so
  `Preview.Height == 0` alone makes the panel disappear with no further
  rendering change needed.

`Model.capturePreview()` (task 017's tick engine) now returns `nil` — no
`capture-pane` call at all — whenever `m.computeLayout().PreviewShown` is
`false`, in addition to its pre-existing no-session/no-selection guards.
The reschedule tick (`previewTick`'s own `tea.Tick` re-arm) is left
running unconditionally so the engine resumes captures the moment the
frame grows back past the floor; only the capture-pane call itself is
gated. Proved in `internal/tui/preview_capture_test.go`'s
`TestCapturePreviewStopsWhenPreviewNotShown` via the same spy-closure seam
task 017's own tests use (a `previewCapture` func that records whether it
was ever invoked) — no structured log or separate command-capture seam was
needed since the capture function itself is already an injectable seam.

No scroll state, capture history, or PgUp path for the preview existed
before this task (`grep -rn "previewScroll\|preview.*[Hh]istory\|PgUp.*review\|previewOffset"`
finds nothing in `internal/tui` or `features`); PgUp/PgDn have driven only
the sidebar list since task 014 (§11.3 requirement 19). This part of the
success criteria was already an existing invariant, verified rather than
newly enforced.

Evidence: `internal/tui/layout_test.go`'s
`TestComputeLayoutSideBySideAndCollapsedAlwaysShowPreview` and
`TestComputeLayoutStackedSuppressesPreviewBelowItsFloor`;
`internal/tui/preview_capture_test.go`'s
`TestCapturePreviewStopsWhenPreviewNotShown`. `ci/run.sh go build ./...`,
`go vet ./...` and `go test -p=1 -count=1 ./...` all green.

## Task 023: attention sort with a documented, total tie-break (SPEC requirements 28, 29)

### The function

`internal/tui/attention.go`'s `sortSessionsByAttention` is the sort's only
implementation. It takes an unordered `[]store.Session`, returns a new
slice (the input is never mutated — proved by
`TestSortSessionsByAttentionDoesNotMutateInput`), ordered by:

1. **Group** (requirement 28), via `attentionRank`: `waiting` → `error` →
   `running` → `starting` → `idle` → `stopped`. Any status outside that
   six-member enumeration (there is no reachable path to one today —
   `archived`'s column exists but nothing reads/writes it yet, per task
   020's own finding above) ranks after `stopped` rather than vanishing or
   jumping ahead of it (`TestSortSessionsByAttentionUnknownStatusSortsLast`).
2. **The tie-break key (requirement 29)**, applied inside every group:
   ascending `StatusAt`, then ascending session `ID`. `StatusAt` is the
   timestamp of the session's *current* status, so within the `waiting`
   group this key alone already reads as "oldest first" — the session
   that has been waiting longest sorts first
   (`TestSortSessionsByAttentionWaitingOldestFirst`) — and every other
   group orders its members by the same rule for one consistent
   definition rather than a special case per status. `ID` is unique and
   stable for a session's lifetime, so it is a total order; combined with
   `StatusAt` (also total once `ID` breaks its ties), the whole key is
   total. A frozen clock — two sessions sharing both status and
   `StatusAt` — therefore still resolves to exactly one order, every time
   (`TestSortSessionsByAttentionTiesBrokenByID`, run 20 times to rule out
   `sort.SliceStable`'s ever depending on inbound order for that case —
   it doesn't, because `ID` fully resolves the tie before stability would
   matter).

**The tie-break key, stated plainly for requirement 29: `(StatusAt asc,
ID asc)`, evaluated only after the requirement-28 group rank.**

### Scope of this task vs. 024/025/026

This task lands the pure sort function with full unit coverage; it is not
yet wired into `Model.sessions`' render order. `Model` still renders
sessions in `store.ListSessions`' `created_at, id` order (unchanged since
task 001). Wiring is deliberately deferred to task 024 (grouping by
workspace restructures how the sidebar's rows are built, which is the
natural point to also apply this sort within each group) and task 025
(the "one shared attention source" that the sort, the collapsed strip's
count, and `space` all consult) — task 026's `attention_sort.feature`
proves the fully-wired, visible order.

Evidence: `internal/tui/attention_test.go`, `ci/run.sh go build ./...`,
`go vet ./...` and `go test -p=1 -count=1 ./...` all green.

## Task 031: the golden minimum frame (requirement 42)

`features/testdata/golden/side_by_side_80x24.golden` is the checked-in,
byte-exact record of SPEC §11.2's golden minimum-size frame: side-by-side
layout at 80×24, sidebar 35 total columns / preview 45 total columns,
`DECK_MOUSE=0` (no SGR bytes to fold into the comparison), with one live
`claude` session whose pane content is task 008's deterministic `fitting`
preview fixture, rendered by a real fake-claude process through deck's own
real preview-capture engine — the preview region is asserted, not carved
out of the comparison.

`features/golden_frame_test.go`'s `TestGoldenMinimumFrame` renders this
frame twice, from two independent freshly created scenario homes, and
compares each render byte-for-byte against the checked-in file — "running
the assertion twice from clean state passes both times" is exercised on
every invocation, not merely claimed once. It was additionally run as two
fully separate `go test` process invocations (distinct pids) during
development to rule out any hidden dependency on within-process test
ordering; both produced identical bytes to the checked-in golden.

Regeneration command (reviewed, deliberate changes only — the golden is
generated, never hand-edited):

```
UPDATE_GOLDEN=1 ci/run.sh go test -run TestGoldenMinimumFrame ./features/...
```

Two nondeterminism sources needed pinning beyond the harness's usual
`DECK_ASCII`/`NO_COLOR`/`DECK_ANIM=0` defaults, both discovered by actually
running the regeneration twice and diffing:

- **The tmux socket name is rendered on screen.** `internal/tui`'s sidebar
  always renders a `socket: <DECK_TMUX_SOCKET>` line at the top when the
  setting is non-empty (`sidebarEntries`, `internal/tui/tui.go`), and the
  scenario harness's default socket name (`deck_test_<pid>_<sequence>`)
  is neither pid- nor sequence-stable across regenerations. The test pins
  `ScenarioHarness.Socket` to a fixed literal before starting any client.
- **The session's working directory is echoed into the workspace group
  header**, `groupHeaderText`'s `marker + workspace + "  " + cwd` — and the
  scenario's cwd lives under a randomized `os.MkdirTemp` directory. This
  turned out *not* to need pinning: the header is truncated to the
  sidebar's fixed inner content width (`padTrunc`), and the truncation
  point falls inside the constant `/tmp/deck` prefix common to every
  `MkdirTemp("", "deck-scenario-")` path, before the random suffix — so
  the visible text is `/tmp/deck...` in every run regardless of the actual
  suffix. Confirmed empirically (identical bytes across separate `go test`
  process invocations) rather than assumed.

One more fact worth recording because it looks like a bug at first glance:
the golden frame's preview shows `row 4 of 5`/`row 5 of 5`, not the
fixture's own title line or `row 1`. This is requirement 23's bottom-left
crop anchoring working as designed (task 018): the fake-claude pane's real
geometry is tmux's own server default, 80×24 (deck never resizes a pane it
only captures), which is taller than the panel's ~21-row content height at
80×24 side-by-side, so the crop keeps the *newest* visible rows of the real
pane, not the oldest. The test waits on the crop's own `of 80x24` geometry
line and the fixture's last line rather than its title, and records why.

Evidence: `ci/run.sh go test -run TestGoldenMinimumFrame -v ./features/...`
(twice, separate processes, byte-identical); full
`ci/run.sh go test -p=1 -count=1 ./...` green.

## Task 033: sweep for remaining full-width-row assumptions

### Why this sweep is different from task 013's

Task 013 swept for *negative* screen-text assertions whose policed copy
physically moved surface (row → footer). Task 033 sweeps for a different
failure mode entirely: an assertion written back when a "row" meant *the
entire terminal width*, one session per line, with no preview panel sharing
any of that horizontal space. Since task 014, a rendered row is two
independent halves on the same screen line — the sidebar's ~33 content
columns (at the 35-column default `sidebar_width`) and the preview's own
content to its right — so an assertion of that vintage can go wrong two
opposite ways: (a) it expected text past column ~33 that the sidebar no
longer has room for (a truncation regression), or (b) it treats "this text
is on the same line as that text" as proof the two are related, when the
line now also carries unrelated preview-panel content on its right half (a
false-positive risk introduced by concatenating two panels' content onto
one Go string per visual row).

### The sweep

```
grep -rn "screen contains\|screen does not contain" features/*.feature | wc -l   # 105 sites
grep -roh 'session "[^"]*"' features/*.feature | sed 's/session "//;s/"$//' \
  | awk '{ print length, $0 }' | sort -rn | uniq                                # longest real names ~23 cols
grep -rn 'contains "[^"]\{45,\}"' features/*.feature                            # long literal strings
grep -rn "row.*contains" features/*.feature                                     # row-scoped (not screen-scoped) checks
grep -rn "100\|width:\|Width:\|Cols:" internal/tui/*_test.go cmd/deck/*_test.go   # fixed-geometry test setups
```

Findings, by category:

1. **Long session names (>20 columns).** `features/harness.feature:95-98` and
   `features/mouse.feature:80-83` are the only names near or past the
   default sidebar's ~33-column content width (a 57-column literal
   containing the marker `WIDENOW`). Both already assert the marker is *not*
   visible at the default width and *is* visible only after widening
   `sidebar_width` with `>` (task 009's own step) — these are exactly the
   pattern task 009/014 introduced, not a leftover gap. Every other named
   session across `features/*.feature` (longest: `deck_externally-stopped`
   at 23 columns, `changed verdict target` at 22) fits inside
   `sidebarRowLines`' documented 33-column budget (`internal/tui/tui.go`,
   the comment on `sidebarRowLines`) with room to spare for its marker,
   status word and badges, so none of them silently rely on truncation
   landing in their favour.

2. **Row-scoped checks that now share a screen line with the preview panel
   (the false-positive risk in (b) above).** `clientRowContainsWithinReconcile`
   (`features/status_probe_test.go:238`) and
   `clientRowDoesNotContainAfterReconcileInterval`
   (`features/assertions_test.go:143`) both operate on
   `strings.Split(client.Frame(false), "\n")` — a raw screen line, which in
   side-by-side layout is `sidebar-content + preview-content` concatenated.
   Every call site names a *unique session name* as the row anchor
   (`"raced claude"`, `"hook shared"`, `"race target"`, `"failed prompt"`,
   `"stale claude"`, `"sampled pi"`, ...) alongside a short status/quality
   word (`"running"`, `"starting"`, `"live"`, `"sampled"`, `"!"`). For a
   false positive, the preview panel's content on that exact visual line
   would need to contain the literal session name — the preview never
   renders any session's name (`previewTitle` returns `""` specifically to
   avoid exactly this kind of collision, see its own doc comment) and cropped
   pane bytes come from a fake agent's fixed fixture/prompt text, not from
   any string containing a test's session name. Verified this holds for the
   two closest cases by inspection: `status_probe.feature`'s selected row
   during the race is deliberately moved to a "probe shell" row (see
   `raceFreshHookAgainstProbe`'s own comment on why), so the preview panel
   on "raced claude"'s own line is that *other* row's line's continuation —
   still never containing the string `"raced claude"`. No site in this
   category needed re-aiming; the anchor half of every check is a name that
   cannot appear in the preview's own vocabulary.

3. **Fixed test geometries (`Cols:`/`width, height =`).** Every
   `internal/tui/*_test.go`, `cmd/deck/*_test.go` and `features/*.feature`
   site setting an explicit terminal size uses width ≥ 80 (side-by-side) or
   explicitly exercises the <80 stacked/collapsed path on purpose
   (`layout_test.go`'s 79/80/81-column cases, `layout_modes.feature`'s
   below-80×24 scenario) — none silently assumes the old single-panel
   full-width row shape at a width that would now render side-by-side.
   `cmd/deck/main_test.go`'s three PTY sizes are all `Cols: 100` (task 032's
   help-overlay pin uses `Rows: 130` for the taller dialog, `Rows: 24` for
   the other two) — all comfortably side-by-side, matching what each test
   actually asserts about.

4. **Hand-computed column/row coordinates.** `features/mouse_bindings_test.go`
   (task 029) explicitly avoids this failure mode by design — its own doc
   comment states a hand-computed column/row "would silently drift the
   moment grouping, elision or a mode change shifts where a row actually
   lands", so `locateText` finds the target text in the client's own current
   frame first and clicks through whatever cell it actually occupies.
   `internal/tui/panel_test.go`'s seam test (`TestSideBySideFrameHasOneSeamAndOneColumnPadding`)
   locates the seam by searching for `│`, never a fixed column offset. No
   site anywhere in `features/` or `internal/tui/` hand-computes a screen
   coordinate against an assumed full-terminal-width row layout.

5. **The `i` detail dialog's fixed-format lines** (e.g.
   `launch_lease.feature:34`'s `"Status reason:      pane failed after the
   stale frame"`, `permission_modes.feature:46`) are unrelated to the
   sidebar/preview split: the detail dialog is a centered modal overlay
   (task 012's own reason-text destination alongside the footer) whose width
   is independent of `sidebar_width`, and its content predates and is
   untouched by this phase's panel chrome.

### Conclusion

No assertion was found that still assumes the pre-chrome full-terminal-width
row shape in a way that is currently wrong, vacuous, or newly at risk of a
false positive from the sidebar/preview concatenation. Tasks 009, 013, 014,
028 and 029 already re-aimed or designed around every site this sweep
turned up; this task's own contribution is recording that sweep and its
reasoning (category 2 above, the false-positive risk from concatenated
sidebar+preview screen lines, was not previously written up anywhere) rather
than any further code change. No test, feature file or production file was
modified by this task.

Evidence: sweep commands above run directly against the checked-out tree;
`ci/run.sh go build ./...`, `go vet ./...` and
`ci/run.sh go test -p=1 -count=1 ./...` all green (features ~93s), matching
the pre-existing baseline with no changes.

## Task 034: remaining required findings, collected in one place

This section pulls together the items task 034's own successCriteria calls
out that were not yet gathered in one place above (each was already decided
by an earlier task's commit; this only records the decision and, where the
PRD's "Findings, not spec edits" section names it as a likely candidate,
cites the exact phrasing).

### The `ui_state` migration decision (requirement 12, task 010)

`internal/store`'s schema version was bumped from 1 to 2 (commit `cbfa75b`).
The new table is `ui_state (key TEXT PRIMARY KEY, value TEXT NOT NULL)` — a
plain key/value table rather than dedicated `layout_mode`/`sidebar_width`
columns bolted onto an existing table, so a later phase's own UI-state keys
(SPEC §11 anticipates more than these two) need no further migration. The
migration path is additive-only (`CREATE TABLE ui_state` on the 1→2 step,
nothing touches `sessions`), proved by
`TestOpenMigratesV1FixtureToUIStateWithoutRecreatingSessionRow`: a v1
fixture's session row survives migration at its original `id`, satisfying
SPEC §4's invariant that a session row is never recreated to gain a field.
`GetLayoutMode`/`GetSidebarWidth` degrade a missing row to the documented
defaults (`auto`, `35`) rather than erroring, so a client that has never
written `ui_state` yet (e.g. one only ever run before task 016 shipped) opens
cleanly. `SetLayoutMode`/`SetSidebarWidth` upsert (`INSERT ... ON CONFLICT
... DO UPDATE`) against the table's own primary key, so repeated `|`/`<`/`>`
presses never accumulate duplicate rows.

### How a group header renders before the theme system exists (requirement 30, tasks 024/039)

SPEC §11.6's theme/token system is explicitly Phase 2b-2 (out of scope this
phase — see "Non-goals for this phase" below). `groupHeaderText`
(`internal/tui/group.go`, commit `5be9487`) therefore renders through the
same mechanism every other piece of this phase's chrome uses to stay
ASCII-safe without any new palette entry: `Model.glyph(wide, ascii)`, picking
`▾`/`▸` (expanded/collapsed marker) or their `v`/`>` fallbacks under
`DECK_ASCII`, exactly the pattern `cropMarker`/`ellipsis` (task 018/019) and
the sidebar's own bullets already use. No new theme token, color, or style
was introduced — the header is plain text (`marker + " " + workspace + "  "
+ cwd`, truncated to the sidebar's content width like any other row) drawn
in whatever the sidebar's existing default style already is, so Phase 2b-2's
token set has nothing of this phase's to retrofit or collide with.

### Every new `DECK_*` control introduced this phase

Only one: **`DECK_MOUSE`** (requirement 3, commit `e6627fc`) — a boolean
overriding `[ui] mouse` (default `true`), parsed with the same
`boolEnv`/invalid-value-errors convention as every pre-existing `DECK_*`
knob (`DECK_ASCII`, `DECK_ANIM`, `DECK_COLOR`). Documented in the help
overlay by task 032 (commit `f27b798`) and in `SPEC.md` §13.1/§6.5 (already
present, task 006's commit predates this findings entry). No other new
`DECK_*` variable was added this phase — `DECK_PREVIEW_MS` (task 017) reused
an existing knob whose default (250) explicitly did not change, and
`DECK_HOME`/`DECK_TMUX_SOCKET`/`DECK_CLOCK`/`DECK_CLOCK_STEP`/
`DECK_RECONCILE_MS`/`DECK_ID_SEED` all predate this phase
(`grep -n "getenv(\"DECK_" internal/config/config.go` confirms the full
list).

### Every deferral, with its target phase

- **`archived` session status/flag (task 020's placeholder branch).**
  Nothing in this phase (or any phase before it) sets `sessions.archived_at`
  or an `archived` status on a live session — archive/reap/purge is Phase 3
  per `prds/phase2b1-visible-shell.md`'s "Non-goals for this phase" list.
  The placeholder dispatch exists and is unit-tested against a
  directly-constructed `store.Session{Status: "archived"}` so it is ready
  the day Phase 3 starts producing that status live, but it is unreachable
  through any UI path today.
- **The full §11.4 dialog contract retrofit, the §11.5 settings takeover,
  and the §11.6 theme system** (including the palette picker and quantised
  token set) — all explicitly **Phase 2b-2**, per the PRD's own "Non-goals
  for this phase" section. This phase's own new chrome (panel borders,
  group headers, the golden frame) deliberately introduces no new theme
  token so Phase 2b-2 has a clean slate.
- **The create modal's completion, §11.7 path entry, the env editor,
  kill/undo toasts, `dd` tombstones, reap, purge, archive, bulk marks,
  rename, and the event log** — **Phase 3**.
- **The Codex adapter** — **Phase 4**.
- **Notification channels, rules and the outbox** — **Phase 5**.
- **Scrollback replay, history files and `last_cwd`** — **Phase 6**.
- **Send-without-attach (§11.1), cross-session search and the health
  view** — **Phase 7**.
- **systemd units** — no phase named yet in the PRD; listed only as a
  non-goal.
- **The attention sort's wiring into `Model`'s actual render order**
  (task 023's own scope note, above) — deferred at the time of task 023 to
  tasks 024/025/026 within *this same phase*, not to a later phase; recorded
  here only because task 023's section above already states it and task
  034 asked for "every deferral" — this one was fully paid off before this
  report was written (tasks 024, 038, 039, 026 all completed).

### `git diff --stat` proof that `SPEC.md`, `prds/`, `ci/Dockerfile` and `ci/SPIKE.md` are unmodified

`SPEC.md` is read-only to this job per the PRD ("Findings, not spec edits").
The last commit to touch any of these four paths before this phase's task
001 began was `e5d2b58` (an operator-steering PRD edit, itself already
landed before task 001's baseline was recorded); every commit from task 001
(`baseline`, uncommitted per its own successCriteria) through this task
leaves them untouched:

```
$ git diff --stat e5d2b58..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
$ echo $?
0
```

(empty diff, exit 0 — confirmed by re-running the command above against the
current `HEAD` before writing this section).

### Suite status

This task only edited this findings document; no test, feature file or
production file changed. `git status --short` shows only this file.
`ci/run.sh go build ./...`, `go vet ./...` and
`ci/run.sh go test -p=1 -count=1 ./...` all green (verified after this
edit, matching the pre-existing baseline).

## Task 040: four harness hazards this run actually lost hours to

Task 034's sweep above collected the required per-requirement findings but
missed the harness's own traps — the ones with no corresponding SPEC
requirement, discoverable only by hitting them. Operator steering 005 asked
for these four to be written down before the phase closes, each as symptom
first, then cause, then workaround.

### 1. The bounded sidebar can hide a row from a polling step, and the step hangs instead of failing

**Symptom:** a step that hangs rather than fails. A scenario built around
`WaitForFrame` (or any "screen contains this row's text" assertion) runs to
its full timeout instead of producing a clean failure.

**Cause:** the sidebar (task 014 onward) is a fixed-height panel, not the
full-screen list the harness was written against in Phase 1/2b-0. A
scenario that creates more sessions than the sidebar's visible height at
the terminal size it runs at leaves the newest row scrolled out of view.
The polling step keeps re-capturing a frame that will never contain the
text it wants, because the row genuinely isn't drawn — there is nothing
wrong with deck, only with the scenario's assumption of an unbounded list.

**Workaround:** size each scenario's session count to the sidebar's visible
height at the terminal geometry the scenario actually runs at, or assert
through a step that fails loudly on a missing row instead of polling
silently — `sessionsRenderInOrder` in `features/attention_sort_test.go`
does this: it fails immediately with the actual vs. expected row order
rather than waiting out a timeout.

### 2. The live preview races any step that intercepts `capture-pane` by pane ID

**Symptom:** a step that expects to be the only caller of a specific
`tmux capture-pane -t <pane-id>` invocation gets its interception consumed
by an unrelated caller, and the expected event/output never arrives — this
is exactly what task 038's validation failure turned out to be (fixed at
commit `a091061`).

**Cause:** the preview engine (tasks 017–021) captures whichever row is
currently selected using the same `capture-pane`-by-pane-ID technique a test
step uses to intercept or pause a specific capture. `capture-pane` carries
no caller identity, so a wrapper that pauses "the next capture-pane against
this pane ID" cannot distinguish the preview's own frequent, ticking calls
(task 017, one per `DECK_PREVIEW_MS`) from the step's single intended call —
whichever fires first consumes the interception.

**Workaround:** move selection off the pane under test before arming the
wrapper, so the live preview is capturing some other row's pane and the
intercepted pane only ever receives the step's own call. This is currently
written down only as a comment in `features/status_probe_test.go` (around
the "keep the hook/probe race step off the live preview's pane" note); this
entry gives it a second, permanent home.

### 3. A fixture name containing the substring a completion helper polls for makes the helper return early

**Symptom:** a keystroke vanishes with no error — the step that types it
reports success, but the next step behaves as if it was never typed.

**Cause:** the create-flow completion helper polls the rendered frame for
the literal string `"starting"` to know when session creation has finished.
A fixture named `s-starting` matched that same literal — not because
creation had actually finished, but because the typed `Name` field's own
text, still on screen mid-entry, contained the string the helper was
searching for. The helper returned long before creation actually finished,
and the next step's keystroke was silently swallowed by `updateCreate`
while `m.creating` was still true. This is the bug that cost task 026 a
1h26m iteration.

**Workaround (applied):** renamed the fixture to `s-agent` (commit
`d7be609`). The general rule this leaves for later phases: never name a
test fixture after a literal string the harness itself polls for — check
any new fixture name against every `strings.Contains`/`WaitFor`-style
polled literal in the harness before using it.

### 4. A raw pty burst of the same key can lose all but the first byte

**Symptom:** a sequence of identical keypresses sent back to back appears
to only take effect once — e.g. the `|` layout-mode cycle (auto →
side-by-side → stacked → collapsed) written as `|`,`|`,`|` in a row
collapses to a single effective press and the mode ends up one step short
of where the scenario expects it.

**Cause:** writing identical bytes to the pty back to back with no pacing
lets deck's input loop coalesce them into fewer effective reads — this is a
property of the raw pty write path in the harness, not of any assertion or
of deck's key handling itself.

**Workaround (applied):** `sendClientKeys` (`features/assertions_test.go`)
paces every send by 25ms, matching the pacing `selectRowByName` has used
since Phase 1 for the same reason. This belongs to the harness's write
path — it strengthens nothing being asserted and weakens no assertion; any
new step that writes repeated keys should route through `sendClientKeys`
(or replicate its pacing) rather than writing raw bytes directly.

