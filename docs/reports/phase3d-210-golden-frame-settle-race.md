# Task 210 (part 2a): root cause and fix — `TestGoldenMinimumFrame`'s "not settled" flake

## Failure

`features/golden_frame_test.go`'s `TestGoldenMinimumFrame` failed intermittently (3/10 in task
209's `ci/stability.sh 10` measurement — "frame kept changing, not settled") with:

```
frame kept changing after the fixture rendered; not settled
```

showing a preview panel that shrank from `41x37` to `41x21` (panel content width x height, per
`internal/tui/panel.go:443`'s `"%dx%d of %dx%d"` geometry line) between the "before" and "after"
capture, dropping the fixture's own first three lines (`PREVIEW FIXTURE: fitting`, `row 1 of 5`,
`row 2 of 5`) off the bottom-anchored preview.

Reproduced directly: `ci/run.sh sh -c 'go test -count=30 -run TestGoldenMinimumFrame ./features/'`
at 1-min loadavg 7-9 failed with this exact before/after pair (see repro logs below) — matching
209's taxonomy exactly, confirming this was never a load-correlated flake per se, but a genuine
race the test's own setup does not close.

## Root cause: a settle condition satisfiable before the thing it waits for finishes

Per steer 019 §2's own hint, this was exactly that shape. The test's setup:

```go
client.Resize(80, 24)                              // shrink the CLIENT's own terminal
client.WaitForFrame(ctx, true, "of 80x24")         // wait for the PANE's geometry line
client.WaitForFrame(ctx, true, "row 5 of 5")       // wait for the fixture's last line
settled := client.Frame(true)
time.Sleep(150 * time.Millisecond)
if got := client.Frame(true); got != settled { t.Fatalf(...) }
```

`"of 80x24"` names the **tmux pane's own real geometry** (`realWidth x realHeight` in
`panel.go`'s geometry-line format), which is deck's fixed default and does not depend on the
*client's* terminal size at all — it was already present in the frame at the client's pre-resize
80×40 size, before any reflow. `"row 5 of 5"` (the fixture's last line) was likewise already
visible in the tall, pre-resize panel (it shows every fixture line at that height). So **both**
waits are satisfiable immediately, at the stale 80×40-derived panel geometry, without the
client's own SIGWINCH-driven reflow to 80×24 having happened yet. The settle check that follows
(`Frame()` / sleep 150ms / `Frame()` again) is the only thing standing between that stale capture
and a wrong "settled" verdict — and when the real resize lands inside that 150ms window (a race,
not a certainty, hence the ~30% rate), the check correctly detects the change and fails.

This is precisely task 202/F3's previously-fixed race
(`features/pty_driver_test.go`'s `ResizeAndAwaitRender` doc comment: "exactly this race let
`@requirement-48-refuse-preview-below-seven-rows`'s Enter be judged against the pre-resize
preview box"), which that helper's own doc comment wrongly listed `golden_frame_test.go` as
*unaffected* by.

### `ResizeAndAwaitRender` alone was tried first, and found insufficient here

The first fix attempt swapped the bare `client.Resize(80, 24)` for
`client.ResizeAndAwaitRender(ctx, 80, 24)` (which polls for bubbletea's own full-screen repaint
marker, `"\x1b[H"`, appearing in the raw output stream after the resize call). This is the
existing, precedented fix for this exact race class — but a second isolated 30-run rerun
(`ci/run.sh sh -c 'go test -count=30 -run TestGoldenMinimumFrame ./features/'`) still reproduced
the identical failure once. Root cause of *that*: `golden_frame_test.go`'s screen has
`previewTick`/`reconcileTick` firing every 250ms/500ms regardless of any resize, and each one
also triggers a full-screen repaint carrying the same marker. `ResizeAndAwaitRender` only proves
*some* repaint happened after the resize call in wall-clock time — not that the repaint it caught
is the one that actually processed the resulting `WindowSizeMsg`. A tick-driven repaint that
lands first, still using the pre-resize geometry, satisfies the wait just as well as the real one.

## Fix

Two layers, both landed:

1. Kept `ResizeAndAwaitRender` (cheap, still a real — if insufficient alone — gate; genuinely
   proves *a* post-call repaint happened, ruling out the pathological case of racing the very
   first repaint after the resize call).
2. Added a **content-specific** gate that is a direct function of the new panel height, not a
   proxy for "a render happened": `client.WaitForFrameGone(ctx, true, "row 1 of 5")`, before the
   existing `"row 5 of 5"` wait. At 80×40 the panel is tall enough to show the fixture's first
   line alongside its last; at the golden's own 80×24 it is not (bottom-anchoring, requirement
   23, crops it out). Waiting for `"row 1 of 5"` to leave the screen is waiting for the resize's
   own effect on the panel to land — it cannot be satisfied by an unrelated tick's repaint that
   merely happens to fire after the resize call.

Also corrected `features/pty_driver_test.go`'s `ResizeAndAwaitRender` doc comment, which
previously (and now provably wrongly) listed `golden_frame_test.go` as unaffected by the
task-202/F3 race, and added a note that any Resize call on a *ticking* screen should pair
`ResizeAndAwaitRender` with a content-specific gate, not rely on the marker alone.

## Non-vacuousness

- **Before this task's fix** (bare `client.Resize`): isolated `-count=30` rerun at 1-min loadavg
  ~8 reproduced the exact failure (see below).
- **Midway** (`ResizeAndAwaitRender` alone, no content-specific gate): a second isolated
  `-count=30` rerun still reproduced the identical failure once — proving the marker-only fix
  was not sufficient by itself, not merely under-tested.
- **After the full fix** (both gates): two independent isolated `-count=30` reruns (60 sub-runs
  each, 120 sub-runs total) at 1-min loadavg 7-10, zero failures.
- **Revert-and-reproduce**: `git stash` (removing both this task's changes), then
  `ci/run.sh sh -c 'go test -count=30 -run TestGoldenMinimumFrame ./features/'` at 1-min loadavg
  ~8 reproduced the exact same before/after frame pair (panel `41x37` → `41x21`, `row 1 of 5`
  present then gone) on run 1 of a 30-run batch; `git stash pop` restored the fix, confirmed via
  `git diff --name-only` showing only the intended two files and `gofmt -l` clean.

Scratch repro logs are container-local (not committed), per this run's existing convention for
ad hoc diagnostic reruns cited by path (see e.g. `docs/reports/phase3d-210-restart-audit-race.md`):
this iteration's own terminal output is the record (transcript), since no separate log file was
kept on disk this time.

## Scope note

This resolves the **second** of task 210's four named failure classes (3/10 in 209's taxonomy).
Still open: `internal/tmux`'s `TestPanePipeReceivesGenuineEOFOnDisplacementWithPanePipeStillOne`
(1/10, root-cause together with task 207's fix per steer 019 §2) and the "unique directory match
ghosts" failure (2/10, not yet investigated). Task 210 remains in-progress; see `notes.md` for
the handoff.
