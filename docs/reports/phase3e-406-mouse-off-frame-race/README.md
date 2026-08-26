# Task 406 — the whole-suite-only `DECK_MOUSE=0` frame-equality failure and the hung-client kill

Base tree: `0f4db8a` (tip when this task started).

## 1. The mechanism, cited by file:line

`features/mouse.feature`'s `@requirement-37-deck-mouse-disables-gestures` scenario creates two
shell sessions, marks both `idle` directly in the state database, waits for `idle` to appear on
screen, captures a baseline frame, sends a click (a no-op, since `DECK_MOUSE=0`), then asserts the
frame is unchanged. The review's whole-suite run failed there with:

```
Error: client "A" frame changed after gesture   (preview pane content differed:
       "61x27 of 80x24" vs "$"; sidebar selection identical in both frames)
```

**Cause:** the passive preview's fit-on-selection feature (`[ui] preview_fit`, default **true**,
`internal/tui/tui.go:1269` `Model.previewFit`, wired from the `previewTick` case at
`internal/tui/tui.go:1804`) resizes the *selected* session's real tmux window to match the
preview panel's content box the first time that session becomes selected, via
`tmux.Client.FitWindowToPane` (`internal/tmux/geometry.go:266`), which converges in 3-4
sequential `resize-window` calls (`internal/tmux/geometry.go:246-296`'s own doc comment). Until
that fit lands, the freshly-created session's window is still at tmux's own default size
(80x24, never explicitly set by `new-session`, `internal/tmux/tmux.go:249`), which is *larger*
than the panel's content box (61x27 in this scenario's 100x30 terminal), so
`cropPreviewBottomLeft` (`internal/tui/panel.go:594-624`) prepends a `"WxH of realWxrealH"`
geometry-mismatch line — `"61x27 of 80x24"` — ahead of the pane's own first visible row. Once
the fit lands, `realWidth`/`realHeight` equal the content box, `cropped` becomes `false`, the
notice line disappears, and the pane's own first row (a bare shell prompt, `"$"`) is what shows
instead.

`FitWindowToPane`'s command is issued asynchronously (inside a `tea.Cmd` goroutine, only applied
to the model once `previewFitDone` arrives, `internal/tui/tui.go:1808-1810`) and is not bounded by
anything the scenario's own fixed-sleep-then-snapshot baseline (`clientCapturesFrameAs`'s original
`time.Sleep(50ms)` before this task) waits for. Under load, the fit's completion can land in the
gap between the baseline capture and the later compare — a change with no gesture involved at
all. The scenario does not disable `preview_fit` (`features/preview.feature`'s own
`@requirement-21-preview-no-side-effects` scenario, `features/preview.feature:35`, deliberately
disables it via `the deck config disables preview fit`/`DECK_PREVIEW_FIT=0` specifically to
isolate itself from this exact class of race — the DECK_MOUSE=0 scenario simply never had to
before this task, since it does not otherwise touch preview_fit at all).

**Root-caused, not theorised**: reproduced directly (§2) by injecting a controlled delay on
exactly the `resize-window` command this mechanism issues, nothing else.

## 2. Reproduction

A `tmux` wrapper script (`tmuxwrap-experiment/tmux-wrapper.sh`) shadows every deck client's `PATH`
(via a one-line, never-committed edit to `features/lifecycle_test.go`'s `Environment()`, reverted
immediately after every experiment — `git diff features/lifecycle_test.go` is empty in this task's
commit) and adds a configurable `sleep` in front of `resize-window` calls only, then execs the
real `tmux` unchanged for everything else.

At induced heavy contention (loadavg ~55-60, deliberately from 40 `busybox` sibling containers
spinning `while true; do :; done`, per the standing rule "reproduce contention deliberately"),
sweeping the injected delay 0.05s-0.7s x 3 trials against the **unmodified** tree reproduced the
review's exact failure signature twice (`pre-fix-repro/`, `resize-window` calls confirmed firing
via the wrapper's own log, delayed exactly as configured) — same `"61x27 of 80x24"` vs `"$"` text,
same accompanying `surviving deck client: hung deck client killed after 1s` + goroutine dump.

## 3. The hung-client-kill goroutine dump is a consequence of (1), not a second defect

`Stop`'s own doc comment (`features/pty_driver_test.go`, `SIGQUIT` before `SIGKILL`) explains it:
once the scenario's `Then` step fails, the scenario body never reaches its own
`deck client "A" exits cleanly` step, so the after-scenario hook finds client "A" still running
and calls `Stop`, which — because nothing ever asked the client to quit — always times out and
prints exactly this diagnostic. It fires identically for *any* mid-scenario failure that leaves a
client running, confirmed by triggering it via an unrelated mutant (§5) and seeing the identical
message shape. Nothing to root-cause independently.

## 4. Fix

Adding a quiescence wait — poll until no new PTY output for a bounded window, not a flat sleep —
to the SHARED `clientCapturesFrameAs` helper (used by 13 "captures its frame as" call sites across
the suite) was tried first and is deliberately **not** what shipped: it regressed
`attach_scroll.feature`'s `@requirement-48-wheel-scrolls-attached-pane-without-typing` scenario,
caught before commit (`attach-scroll-regression-check/`) — that scenario's own baseline races a
*different* async source (an attached real tmux client's status line naming its window `tmux*`
until automatic-rename settles it to `sh*`, which pushes no bytes down the pty on its own until
some later event forces a redraw), for which "wait longer before calling it quiet" does not help
and actively hurts (baseline unmodified tree: 3/3 green in isolation; widened fix: 5/5 RED in
isolation at loadavg ~3.7).

**What shipped instead**: a new, separately-named driver method and step, used only by the one
scenario this task's own reproduction implicates:

- `ScreenDriver.WaitForQuiescence` (`features/pty_driver_test.go`): polls the same `d.updated`
  channel `WaitForFrame`/`WaitForFrameFunc`/`WaitForFrameGone` already use, resetting a quiet-timer
  on every new render and returning the last frame once `quietFor` has elapsed with none.
- `clientCapturesSettledFrameAs` / `"deck client %q captures its settled frame as %q"`
  (`features/mouse_synthesis_test.go`), a sibling of the existing `clientCapturesFrameAs` /
  `"captures its frame as"` step, calling `WaitForQuiescence` with a 400ms quiet window instead of
  a flat sleep. 400ms was chosen, not guessed: it must clear **both** `DECK_PREVIEW_MS` (50ms) and
  `scenarioReconcileInterval` (250ms, `features/lifecycle_test.go`) with margin — a shorter window
  (200ms was tried first) can land in the ordinary gap *between* two reconcile ticks and mistake
  "nothing has rendered because the next tick isn't due yet" for "settled", which let a
  not-yet-`idle`-reconciled session or a not-yet-issued `previewFit` slip through as a false
  baseline in this task's own sweep (`post-fix-sweep-scoped/summary.log`'s early failures before
  400ms).
- `features/mouse.feature`'s DECK_MOUSE=0 scenario is the ONLY caller changed to the new step.
  Every other "captures its frame as" call site (attach_scroll, dialogs, kill_delete_undo,
  harness, preview, settings) is untouched, proven green (§"Regression check").

The original `clientCapturesFrameAs` and `CaptureSnapshot` are otherwise unchanged.

## 5. The comparison's discriminating power is unchanged

Nothing about `clientFrameStillMatchesCaptured` (the actual `Then ... frame still matches ...`
assertion) was touched — only how the *baseline* is captured changed. Proof the assertion still
catches a real change: mutated `deckConfigDisablesMouse` (`features/mouse_control_test.go`) to
leave mouse reporting enabled instead of setting `DECK_MOUSE=0`, so the "no-op" click actually
retargets the sidebar selection. Tag alone, RED (`mutant-mouse-enabled-red.log`):

```
--- FAIL: TestFeatures (2.33s)
    --- FAIL: TestFeatures/DECK_MOUSE=0_disables_every_mouse_gesture,_and_only_the_shortcut_is_lost (2.31s)
```

Reverted immediately after (`git diff features/mouse_control_test.go` empty in the shipped
commit).

## 6. Post-fix reproduction sweep

Same delay sweep as §2 (0.05s-1.0s x 3 trials = 21 runs), re-run against the scoped fix at
moderate contention (loadavg ~3-24, close to the review's own reported 0.62->2.43):
**19/21 green** (`post-fix-sweep-scoped/`). The two residual failures both needed a single
`resize-window` call delayed 0.5s — double `scenarioReconcileInterval` and ten times
`DECK_PREVIEW_MS` — traced to a related, lower-priority defect (§7), not the mechanism this task
fixes, and far outside the review's own observed load range.

**Correction (task 406 validation follow-up):** the sentence this replaces originally
claimed a per-run `uptime` sample existed for every one of the 39 sweep rows above
and under `pre-fix-repro/README.md`/`post-fix-sweep-scoped/README.md`. That was false
as written — neither sub-directory's table, nor any `*.log` file in this report, ever
contained a loadavg value; only a single aggregate range was sampled per whole batch.
The sweep has since been re-run with a real `uptime` sample taken immediately before
every invocation: see `pre-fix-repro/loadavg-redo/README.md` (18 runs, 4 reproduce the
mechanism, all 4 failure logs attached with their loadavg) and
`post-fix-sweep-scoped/loadavg-redo/README.md` (31 runs including 10 extra at the
residual-risk delay value, 31/31 green). Every row in both redo tables carries its own
`loadavg_before` (the 1/5/15-minute `uptime` triple sampled right before that row).

## 7. Residual risk (found, not fixed — named per this loop's own findings convention)

At an artificially large single `resize-window` delay (0.5s, double the 250ms reconcile interval),
`Model.previewFit` can re-fire for the same still-selected session on the very next `previewTick`
before the PREVIOUS invocation's `previewFitDone` has arrived and updated `previewFitSessionID`
(`internal/tui/tui.go:1804-1810` — the guard at `tui.go:1280` compares against the field, but
nothing prevents a second `previewFit()` `tea.Cmd` from being scheduled while the first is still
in flight), producing overlapping `FitWindowToPane` invocations for the same session. This is a
genuine, if narrow, timing hazard in the product code, not just the test harness — but at
production's real default cadence (`DefaultPreviewMS` = 250ms, `DefaultReconcileMS` = 500ms,
`internal/config/config.go:23-24`, five and two times this scenario's aggressive 50ms/250ms test
cadence) and `resize-window`'s normal sub-millisecond local-tmux latency, the window for this to
matter in practice is far narrower than what the delay sweep needed to hit it twice in 21 runs.
Not fixed here: task 406's scope is the reported test flake, not a speculative product hardening
pass; recorded for `docs/reports/phase3e-findings.md` (task 409) as a defect found but not fixed.

## 8. `defaultTags` unchanged

```
$ grep -rn defaultTags features/
features/godog_test.go:15:const defaultTags = "~@real-agents && ~@nightly"
```

No `sleep`, no skip, no `@flaky`/`@wip` added anywhere in this task's diff.

## 9. Repository state

`git status --short` empty, nothing unpushed, at the commit this task lands. `.tmp406/` (the
experiment scratch directory: the tmux-delay wrapper script plus its own runtime log) was used for
every experiment above and is deleted before commit — its one durable artifact,
`tmuxwrap-experiment/tmux-wrapper.sh`, is copied into this report dir instead.
