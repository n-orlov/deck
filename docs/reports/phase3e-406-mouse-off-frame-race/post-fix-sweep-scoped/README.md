Same delay sweep (0.05s-1.0s x 3 trials = 21 runs), re-run against the
scoped fix (`clientCapturesSettledFrameAs`/`WaitForQuiescence`, used only
by mouse.feature's `@requirement-37-deck-mouse-disables-gestures`
scenario) at moderate contention (loadavg ~3-24, close to the review's own
reported 0.62->2.43):

delay=0.05 trial=1 exit=0
delay=0.05 trial=2 exit=0
delay=0.05 trial=3 exit=0
delay=0.1  trial=1 exit=0
delay=0.1  trial=2 exit=0
delay=0.1  trial=3 exit=0
delay=0.2  trial=1 exit=0
delay=0.2  trial=2 exit=0
delay=0.2  trial=3 exit=0
delay=0.3  trial=1 exit=0
delay=0.3  trial=2 exit=0
delay=0.3  trial=3 exit=0
delay=0.5  trial=1 exit=1   <- postfix4-0.5-1.log (this dir)
delay=0.5  trial=2 exit=1   <- postfix4-0.5-2.log (this dir)
delay=0.5  trial=3 exit=0
delay=0.7  trial=1 exit=0
delay=0.7  trial=2 exit=0
delay=0.7  trial=3 exit=0
delay=1.0  trial=1 exit=0
delay=1.0  trial=2 exit=0
delay=1.0  trial=3 exit=0

19/21 green. The two residual failures both need a SINGLE resize-window
call delayed 0.5s -- double `scenarioReconcileInterval` (250ms) and ten
times `DECK_PREVIEW_MS` (50ms) -- which is far outside the review's own
observed loadavg range (0.62->2.43) and is discussed as a separate,
lower-priority finding (not fixed by this task) in README.md's "Residual
risk" section: previewFit re-fires every previewTick while a fit for the
SAME still-selected session is still in flight (previewFitSessionID is
only updated once previewFitDone arrives, tui.go:1804-1810), so an
external command slower than one tick interval can overlap with a second,
concurrent invocation for the same session.

**No per-run loadavg was captured in this original sweep** (only the
aggregate "~3-24" range above, sampled a handful of times across the whole
batch, not per row) -- corrected by `loadavg-redo/README.md` in this
directory, which re-runs this sweep (plus 10 extra delay=0.5s trials) with a
real `uptime` sample before every invocation and states its own, real (not
identical) result.
