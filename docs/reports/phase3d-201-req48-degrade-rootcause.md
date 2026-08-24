# Root cause of `@requirement-48-refuse-preview-below-seven-rows`'s intermittent failure (F3)

## Reproduction (by this task, not carried over from a prior measurement)

Command:

```
ci/run.sh sh -c 'DECK_GODOG_TAGS="@requirement-48-refuse-preview-below-seven-rows" go test -count=1 -run TestFeatures ./features/'
```

`cat /proc/loadavg` immediately before the run: `3.73 4.35 4.45` — 1-min loadavg 3.73, **below
4.0**. The scenario failed on the first try (no retries needed to reproduce). Full output:
`docs/reports/phase3d-201-req48-repro.log` (committed alongside this file).

The failing frame, verbatim from the log:

```
+ deck - sessions -----------------+  squeezed interactive 41x6 fitted - Ctrl+Q+
| socket: deck_test_99_2           | deck: the agent has not repainted sinc... |
| v walking-skeleton-cwd  /tmp/... |                                           |
| > squeezed running               |                                           |
|   created just now               |                                           |
|                                  |                                           |
|                                  |                                           |
+----------------------------------+-------------------------------------------+
```

This title bar reads `interactive 41x6 fitted` — deck **entered interactive mode** at a
6-inner-row box. The scenario's own assertion is `client "cramped" did not show "7-row floor"
within 5s: timed out waiting for frame "7-row floor"` — deck never showed the refusal message at
all; it degraded into a squeezed interactive session instead. This is not a slow-repaint timeout
on the refusal frame; the refusal frame never gets composed because the refusal branch never
fires.

## Mechanism

Two independent gaps compound into this failure:

**(a) The harness's resize step returns before deck has processed the resize.**
`features/resize_test.go:33-43`'s `resizeNamedClient` calls `client.Resize(cols, rows)`
(`features/pty_driver_test.go:136-148`) and returns as soon as that call returns. `Resize` itself
only does two things before returning: `pty.Setsize` on the real kernel PTY (which asks the
kernel to deliver `SIGWINCH` to deck's foreground process group asynchronously — it does not wait
for deck to have handled it) and resizing the **emulator's own** `d.screen`/`d.budget` model so a
later `Frame`/`GridSize` call reads the new geometry. Nothing in `Resize` or its caller blocks
until deck's own `tea.Program` has processed a `tea.WindowSizeMsg` and re-rendered. So the
scenario's very next step — pressing `Enter` to invoke `enterInteractive` — races deck's SIGWINCH
handling: whichever finishes first decides whether `m.previewContentSize()` sees the new (7-row)
or old (pre-shrink, ≥7-row) box at the moment `Enter` is evaluated. When deck wins the race,
`enterInteractive`'s floor check (`internal/tui/interactive.go:80-83`) is evaluated against a box
that is still the *old*, larger size, so `height < interactiveMinInnerRows` is false and the
refusal never fires — deck instead proceeds to attach and enters interactive mode, later
recomputing `previewContentSize()` at the *new*, smaller size once deck's own resize has caught
up, which is what makes the title bar read `41x6 fitted` at the moment the frame is captured.

**(b) The floor check runs only once, at entry, and nothing re-checks it afterward.**
`enterInteractive` (`internal/tui/interactive.go:37-83`) computes `previewContentSize()` and
compares it to `interactiveMinInnerRows` a single time, before attaching. Once inside interactive
mode, `previewTitle()` (`internal/tui/tui.go:3086-3095`) recomputes `previewContentSize()` on
**every render** (to compose the `"WxH fitted"` title), but that recomputation only feeds the
title string — it is never fed back into the floor check. So even setting aside gap (a), a
terminal that shrinks below the floor *while already interactive* (whether via the harness's
resize step or a real user's terminal) is never caught: deck stays interactive, rendering
`"41x6 fitted"` in its own title bar, at a box the requirement says must never be entered.

Both gaps matter independently: (a) alone explains why *this specific scenario* (resize, then
immediately try to enter) is racy at ~20–40% failure depending on load-driven scheduling luck; (b)
is what lets the race's losing outcome (entering at the smaller size) look exactly like the frame
above rather than self-correcting on the next render.

## Correction to the "Standing pre-existing flake list"

`docs/reports/phase3-findings.md`'s "Standing pre-existing flake list" (entry 2, second bullet)
previously classified this scenario as timing out "waiting for the '7-row floor' frame under
load" and carried it as a load-correlated PTY timeout in the same family as the
`@requirement-48-wheel-scrolls-attached-pane-without-typing` hang. That classification is
corrected in the same commit as this file: this failure is **not** load-correlated (reproduced on
the first try at 1-min loadavg 3.73, well under the 4.0 the scenario has previously been blamed
on) and **not** pre-existing (the scenario itself, `interactive_refusals.feature:29`, was created
in this run to cover requirement 48). It is a real race between the harness's resize step and
deck's own SIGWINCH handling, compounded by a floor check that only runs once at entry — see
"Mechanism" above. No product or harness behaviour is changed by this commit; the fix is tracked
separately (tasks 202-204).
