Pre-fix reproduction sweep (unmodified `clientCapturesFrameAs`, artificial
`resize-window` latency injected via a `tmux` PATH-shadowing wrapper) at
induced host contention (loadavg ~55-60, 40 deliberately-spun busybox
containers). 6 delay values x 3 trials each = 18 runs of
`@requirement-37-deck-mouse-disables-gestures` alone:

delay=0.05 trial=1 exit=0
delay=0.05 trial=2 exit=0
delay=0.05 trial=3 exit=1   <- repro-1 (this dir)
delay=0.1  trial=1 exit=0
delay=0.1  trial=2 exit=0
delay=0.1  trial=3 exit=1
delay=0.2  trial=1 exit=1   <- repro-2 (this dir)
delay=0.2  trial=2 exit=1
delay=0.2  trial=3 exit=0
delay=0.3  trial=1 exit=0
delay=0.3  trial=2 exit=0
delay=0.3  trial=3 exit=0
delay=0.5  trial=1 exit=1
delay=0.5  trial=2 exit=1
delay=0.5  trial=3 exit=0
delay=0.7  trial=1 exit=1
delay=0.7  trial=2 exit=1
delay=0.7  trial=3 exit=0

Both attached logs show the exact review signature: baseline preview shows
"61x27 of 80x24" (crop/geometry notice, real tmux window still at its
un-resized default 80x24), compare shows "$" (notice gone, actual shell
content -- previewFit's resize-window converged in the gap), plus the
accompanying "surviving deck client: hung deck client killed after 1s" +
goroutine dump (a consequence of the scenario aborting before its own
"exits cleanly" step, not a second defect).

**No per-run loadavg was captured in this original sweep** (only the
aggregate "~55-60" range above, sampled a handful of times across the whole
batch, not per row) -- corrected by `loadavg-redo/README.md` in this
directory, which re-runs this same sweep with a real `uptime` sample before
every invocation and states its own, real (not identical) reproduction
count.
