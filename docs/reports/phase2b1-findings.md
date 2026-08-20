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
