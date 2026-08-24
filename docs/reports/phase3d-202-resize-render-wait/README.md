# Task 202 — make the harness resize step land before the next keystroke

## Root cause (task 201's finding, closed here)

`features/resize_test.go`'s `deck client "X" terminal is resized to WxH` step
called `ScreenDriver.Resize`, which returns as soon as `pty.Setsize` succeeds
and the driver's own emulator grid has been resized locally. It never waited
for deck's own Bubble Tea program to observe the SIGWINCH the kernel delivers,
process the resulting `tea.WindowSizeMsg`, and re-render at the new size. The
very next step in `@requirement-48-refuse-preview-below-seven-rows`
(`deck client "cramped" enters interactive mode`, i.e. `Enter`) could
therefore reach deck before that re-render, so the floor check in
`internal/tui/interactive.go` was evaluated against the stale, pre-resize
preview box — deck entered interactive mode anyway, squeezed to `41x6`,
instead of refusing.

## Fix

Added `ScreenDriver.ResizeAndAwaitRender` (`features/pty_driver_test.go`):
identical to `Resize` (same `pty.Setsize` + emulator resize), plus a bounded
poll (5s, matching this package's other checkpoints) on the driver's own raw
output for `"\x1b[H"` (`ansi.CursorHomePosition`) appearing *after* the
resize — the sequence bubbletea's alt-screen renderer writes at the start of
every full-screen flush, including the one the resize's `WindowSizeMsg`
triggers. The poll uses the existing `d.updated` signal channel exactly as
`WaitForFrame`/`WaitForFrameGone` do — an instrument, never a sleep — and
returns a diagnostic (raw + frame) on timeout.

`features/resize_test.go`'s `resizeNamedClient` now calls
`ResizeAndAwaitRender` instead of `Resize`. No other `Resize` caller changed
(`features/sigwinch_count_test.go`, `features/golden_frame_test.go`,
`features/fake_agent_size_test.go` all call `ScreenDriver.Resize` directly,
not through this step, and are unaffected).

## Evidence

- `req48-10-isolated-runs.log`: 10 consecutive isolated runs of
  `DECK_GODOG_TAGS="@requirement-48-refuse-preview-below-seven-rows"` after
  the fix, all `ok` — host 1-min loadavg climbing from ~6.5 to ~17 across the
  10 runs (unrelated background load on the box), confirming the fix holds
  under rising load, not just at the quiet baseline task 201 reproduced the
  bug at.
- `other-resize-step-tags.log`: one isolated run each of every other tag
  whose scenario uses the `deck client "X" terminal is resized to WxH` step
  (enumerated with `rg -n "terminal is resized to" features/*.feature`,
  excluding `@requirement-4-fake-agent-sizes`, which uses the unrelated
  `the fake "<agent>" agent terminal is resized to` step):
  `@requirement-21-preview-no-side-effects`,
  `@requirement-27-preview-suppressed-below-floor`,
  `@requirement-38-layout-modes`, `@requirement-14`, `@requirement-1-resize`,
  `@multiclient` (concurrency.feature's Feature-level tag, covering the
  scenario at line 54). All `ok`.
- `ci/run.sh go build ./... && go vet ./... && gofmt -l $(git ls-files '*.go')`
  clean (no output).
