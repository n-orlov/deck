# Task 012 — R73 leg 1: line-granular overlay scroll (issue #7)

## What landed

`internal/tui/panel.go`: `dialogScrollBy` gained a **step parameter**
(`dialogScrollBy(current int, body string, dir, step int)`), keeping its existing
`[0, dialogMaxScroll(body)]` clamping untouched; a `step < 1` is treated as 1 so a
keypress can never be a silent no-op. Two thin named wrappers over that one scroller
(no second scroller was written, per the PRD's "add a parameter" instruction):

- `dialogScrollByPage` — step = `dialogContentBudget()` (PgUp/PgDn's old behaviour, unchanged).
- `dialogScrollByLines` — step = 1 (up/down and their `j`/`k` aliases).

Handlers: `updateHelpView` (`internal/tui/tui.go`), `updateDetailView`
(`internal/tui/rename.go`) and `updateEventLog` (`internal/tui/event_log.go`) each gained
`case "up", "k":` / `case "down", "j":` bound to `dialogScrollByLines`, with `pgup`/`pgdown`
switched to the explicitly-named `dialogScrollByPage`.

Not in this task (separate tasks 013 / 014): wheel routing, and the help overlay's own text.

## Judgement call recorded here (task 014 also records it)

**Pages still do not overlap.** `dialogScrollByPage` steps the full `dialogContentBudget()`,
so consecutive pages share no line — deliberately kept as task 078 shipped it, because the
new one-line arrows now cover the "read across the page seam" case a one-line overlap would
have existed for. Recorded as a decision, not inherited silently.

## Tests

`internal/tui/overlay_line_scroll_test.go` (new), three tests × the three overlays as
subtests, all driving the real `Update`/`View` path with `key()`:

- `TestScrollableOverlaysScrollOneLineWithArrows` — after `down`, the overlay's FIRST
  rendered content row is the row that was SECOND before the press; a second `down` advances
  one more; `up` retreats by exactly one and lands back on the row one `down` had shown.
- `TestScrollableOverlaysScrollOneLineWithJK` — identical assertions for `j`/`k`.
- `TestScrollableOverlaysStillPageWithPgDn` — `pgdown` still moves by a whole
  `dialogContentBudget()` (clamped by the body's own `dialogMaxScroll`, which is what the
  event-log fixture hits: 14, not a full 22-row page), and `pgup` returns to the top row.
  The test refuses to run if a page step here were < 2 lines (it would then not discriminate).

The assertions compare **rendered rows**, not `helpScroll`/`detailScroll`/`eventLogScroll`,
and not "the view changed" — the PRD's whole test-design point, since binding `down` to the
page step also changes the view.

Fixtures reuse the existing over-flowing ones (`heightBoundTestSession`,
`openEventLogTestStore` + 30 events, the 273-line help text) so no subtest is vacuous.

## Green run

```
$ cat /proc/loadavg
1.45 2.14 2.57 2/4162 13118
$ ci/run.sh go test -count=1 ./internal/tui/
ok  	github.com/n-orlov/deck/internal/tui	0.662s
```

Also green: `ci/run.sh go vet ./internal/tui/`, `ci/run.sh go build ./...`, and
`ci/run.sh go test -count=1 ./cmd/deck/` → `ok ... 5.386s` (loadavg `4.97 3.10 2.88`) — the
PTY help-window test is unaffected because no help line was added in this task.

## Revert-and-reproduce (would this go red if the fix were reverted?)

The plausible naive implementation is exactly cause 2 of the PRD's root cause: bind the new
keys, but to the page step. `internal/tui/panel.go` copied to `/tmp/panel.go.orig`, then
mutated in place (not deleted):

```go
func (m Model) dialogScrollByLines(current int, body string, dir int) int {
	return m.dialogScrollBy(current, body, dir, m.dialogContentBudget())   // naive: a page
}
```

```
$ ci/run.sh go test -count=1 -run 'TestScrollableOverlays' ./internal/tui/
--- FAIL: TestScrollableOverlaysScrollOneLineWithArrows (0.03s)
    --- FAIL: TestScrollableOverlaysScrollOneLineWithArrows/help_overlay (0.00s)
    --- FAIL: TestScrollableOverlaysScrollOneLineWithArrows/detail_view (0.00s)
    --- FAIL: TestScrollableOverlaysScrollOneLineWithArrows/event_log (0.00s)
--- FAIL: TestScrollableOverlaysScrollOneLineWithJK (0.02s)
    --- FAIL: TestScrollableOverlaysScrollOneLineWithJK/help_overlay (0.00s)
    --- FAIL: TestScrollableOverlaysScrollOneLineWithJK/detail_view (0.00s)
    --- FAIL: TestScrollableOverlaysScrollOneLineWithJK/event_log (0.00s)
FAIL	github.com/n-orlov/deck/internal/tui	0.078s
```

Two of the six failure messages, showing the assertion is about the *distance moved*:

```
overlay_line_scroll_test.go:164: event log: after "down" the first visible row advanced by 14 lines, want exactly 1
overlay_line_scroll_test.go:192: event log: after "j" the first visible row advanced by 14 lines, want exactly 1
overlay_line_scroll_test.go:164: help overlay: after "down" the first visible row is not any row of the previous view -- it moved by more than one page, want exactly 1 line
```

(`TestScrollableOverlaysStillPageWithPgDn` correctly stays green under the mutation — it
pins the page step, which the mutation does not change.)

Restored afterwards: `cp /tmp/panel.go.orig internal/tui/panel.go`, `diff` empty,
`git status --short` showing only this task's four modified files plus the new test file.
Full log of the reverted run: `artifacts/task012-r73-line-scroll-reverted.log`.
