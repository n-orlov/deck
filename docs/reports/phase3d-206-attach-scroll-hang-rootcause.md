# Task 206: root-cause of `@requirement-48-wheel-scrolls-attached-pane-without-typing`'s copy-mode-exit hang

## Symptom

`features/attach_scroll.feature:11`'s `@requirement-48-wheel-scrolls-attached-pane-without-typing`
scenario occasionally never returns from
`clientExitsCopyModeOnAttachedPane`'s `WaitForFrameGone(ctx, false, attachScrollTopMarker)` after
sending tmux's copy-mode cancel key ("q"). Before task 205's default-deadline fix this hung the
whole `TestFeatures` process to Go's hard 10-minute timeout (the exact failure mode task 205's own
evidence run hit, `docs/reports/phase3d-205-wait-deadline/full-suite-run-with-known-206-flake.log`).
After 205 it fails fast (tens of seconds) with a bounded diagnostic instead, which is what made
isolating this root cause practical.

## Reproduction

Baseline (pre-fix) isolated-run batches this iteration:
- 2/15 failures (loadavg 3.7–17 across the batch)
- 4/61 failures across two further batches (loadavg ~6–7)

All failures show the identical shape: `clientExitsCopyModeOnAttachedPane`'s `WaitForFrameGone`
times out (task 205's 45s default bound) waiting for `ATTACH_SCROLL_TOP_MARKER` to leave the
frame. Two full raw captures are committed as
`docs/reports/phase3d-206-attach-scroll-hang/before-fix-repro/repro-hang-{1,2}.log`.

## Root cause: confirmed against the real tmux server, not the harness's screen emulator

The failing frame alone is ambiguous: it could mean tmux is genuinely still in copy-mode, or it
could mean the harness's own `vt10x`-based screen emulator (`features/pty_driver_test.go`) has
merely desynced from a server that already exited. These were told apart by querying the real
tmux server directly over its control socket at the moment of failure (bypassing the emulator
entirely):

```
tmux -L <scenario socket> list-panes -a -F '#{pane_id} in_mode=#{pane_in_mode} mode=#{?pane_in_mode,#{copy_cursor_line}/#{history_size},-}'
```

Result, captured live during four separate hangs this iteration: `%0 in_mode=1
mode=SCROLL_LINE_26/95` every time. `#{pane_in_mode}=1` is the server's own ground truth — the
pane genuinely never left copy-mode. This rules out an emulator artifact: the "q" keystroke
itself never registered as a cancel against the server.

Mechanism: `clientScrollsWheelUpNTimesAt` (`features/attach_scroll_test.go`) sends 30 SGR
wheel-up reports essentially back-to-back (microseconds apart, confirmed via the driver's own
`sentLog` diagnostic), then waits for a frame containing the top-of-scrollback marker before the
scenario proceeds. That wait proves tmux rendered *a* frame with the marker at some point during
the burst — not that the server has finished executing every queued `WheelUp` command. "q" can
still be sent to the client's pty while tmux's server is still draining the tail of that queue,
and occasionally races it: the cancel keystroke is processed as ordinary progress-through-the-
queue timing that leaves the server still in copy-mode by the time downstream code checks.

## What did NOT work

**A fixed settle before "q"** (300ms) was tried first and made things strictly worse: 40/40
isolated runs still failed, but on a *different* symptom — a post-cancel byte-identical frame
comparison mismatch, not a hang. Cause: while idle in copy-mode, tmux's own position/clock
overlay (`HH:MM:SS [line/total]`, rendered every second) ticks; `NormalizeFrame`
(`features/pty_driver_test.go`) only strips deck's own ISO timestamps (`wallTimestamp`,
`relativeTime`), never this overlay, so any nontrivial dwell in copy-mode risks a divergent final
frame. Evidence for this attempt was not preserved (a scratch experiment), but the effect was
reproduced twice, including via a resend-on-verified-timeout retry loop that still had to wait out
part of a `WaitForFrameGone` bound before checking ground truth (3/40 failures, same mismatch
symptom).

## The fix

`clientExitsCopyModeOnAttachedPane` now polls the real tmux server's own `#{copy_cursor_line}`
(`waitForCopyModeQueueToDrain`, `features/attach_scroll_test.go`) until it reports the same value
on two consecutive reads 5ms apart, proving the queued `WheelUp` commands have actually finished
executing, *before* sending "q" — not a sleep, an event-driven poll against ground truth that adds
no meaningful dwell in the success case (typically resolves in a handful of 5ms ticks) and is
bounded at 2s. This targets the actual race (cancel sent while the queue is still draining)
without reintroducing the dwell that broke the fixed-settle attempt.

## Evidence

`docs/reports/phase3d-206-attach-scroll-hang/after-fix-10-consecutive/` — 10 consecutive isolated
runs of the tag, all green (`summary.log`, individual `run-N.log`, `loadavg-before.txt`/
`loadavg-after.txt`, loadavg 5.4–7.7 throughout). Two further un-committed batches of 50 and 40
runs (90 total) during development were also all green. The scenario itself
(`features/attach_scroll.feature`) is unchanged; no assertion removed, no `@flaky`/`@nightly` tag
added, no product timeout widened — the fix is entirely inside the harness step
(`features/attach_scroll_test.go`).

## Correction to the standing flake list

`docs/reports/phase3-findings.md`'s "Standing pre-existing flake list", item 2's first bullet
(this scenario, classified as a "load-correlated PTY-under-load timeout") is corrected in that
file: this was a harness pacing race (cancel-key-vs-draining-mouse-queue), reproduced at loadavg as
low as 3.7 (well under the 4.0 that bullet's own load-correlation claim used), not a load artifact,
and it is now fixed rather than merely catalogued.
