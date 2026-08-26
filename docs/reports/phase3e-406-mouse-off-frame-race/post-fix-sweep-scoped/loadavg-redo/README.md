# Loadavg-instrumented redo of the post-fix delay sweep

Same correction as `../../pre-fix-repro/loadavg-redo/README.md` explains: the
original `post-fix-sweep-scoped/` sweep recorded no per-run `uptime` sample despite
`README.md` §6 claiming one existed for every run. This directory re-runs the same
delay sweep (0.05s-1.0s x 3 trials = 21 runs) against the SHIPPED fix (no feature
file reverted this time — `clientCapturesSettledFrameAs`/`WaitForQuiescence` is
exercised as-is), plus 10 extra trials at delay=0.5s specifically (the one value
that produced the original sweep's 2 residual failures), all with a real `uptime`
sample taken immediately before each invocation.

Contention this time was induced more lightly (~13-25 1-minute loadavg, scaled down
from the pre-fix redo's ~20 containers to ~8, since this sweep does not need to hit
the mechanism — it is checking the fix holds, not reproducing the original defect),
closer to the review's own reported moderate range (0.62->2.43) than the original
sweep's claimed "~3-24" aggregate, though still somewhat higher.

## Per-run table (`table.txt`, this directory)

All 21 scheduled runs plus 10 extra delay=0.5s trials: **31/31 green**, 1-minute
loadavg ranging ~13.6 to ~25.1 across the batch (see `table.txt` for every row).
This redo did **not** reproduce the original sweep's 2 residual delay=0.5s
failures (`postfix4-0.5-1.log`/`postfix4-0.5-2.log`, still in the parent directory,
undated by loadavg) — a real result, not a claim that the residual `previewFit`
re-entrancy hazard documented in the top-level `README.md` §7 no longer exists.
That hazard is a genuine race (confirmed by reading `internal/tui/tui.go:1804-1810`
again for this redo: `previewFitSessionID` is still only updated once
`previewFitDone` arrives, so nothing prevents a second `previewFit` `tea.Cmd`
being scheduled while the first is in flight) that this redo's own delay-sweep
methodology cannot force to reproduce on demand any more reliably than the
original sweep's own 2/21 rate suggests it should. §7's "found, not fixed,
recorded for task 409" disposition stands unchanged.

`loadavg_before` is the exact `uptime` 1/5/15-minute triple sampled in the calling
shell immediately before that row's `ci/run.sh ... go test ...` invocation started.
