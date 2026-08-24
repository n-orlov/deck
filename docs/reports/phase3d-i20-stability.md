# I-20 / requirement 46: ten-run stability measurement after the final code commit (task 076)

## What was run

`ci/stability.sh 10`, run from a clean state (`-count=1` disables the test
cache; each run gets `ci/run.sh`'s own `--rm` sibling container) after task
075's commit `49b54ed`, which is declared THE last code commit of the run.
No code landed between 075's commit and this measurement.

Per-run logs, the script's own combined summary, and an independent
30-second-interval host-load trace collected for the whole duration are
committed alongside this report under `docs/reports/phase3d-i20-stability-logs/`
(`run-1.log` .. `run-10.log`, `summary.log`, `loadtrace.log`).

`features/godog_test.go:15`'s `defaultTags` was read at the start of this
task and is unmodified: exactly `"~@real-agents && ~@nightly"`.

## Result: 4/10 passed

| run | result | wall time (features pkg) | approx mean 1-min loadavg during the run (28 cores) | failure |
|-----|--------|---------------------------|-------------------------------------------------------|---------|
| 1  | PASS | ~5 min  | 4.05 | — |
| 2  | FAIL | 10m0s (timeout) | 3.68 | `attach_scroll.feature:11` (@requirement-48, wheel scroll) |
| 3  | PASS | ~5 min  | 4.66 | — |
| 4  | PASS | ~5 min  | 4.44 | — |
| 5  | FAIL | 10m0s (timeout) | 3.77 | `attach_scroll.feature:11` (@requirement-48, wheel scroll) |
| 6  | FAIL | 249.78s | 4.26 | `interactive_refusals.feature` 7-row-floor scenario |
| 7  | FAIL | 249.68s | 4.63 | `interactive_refusals.feature` 7-row-floor scenario |
| 8  | FAIL | 249.17s | 3.93 | `interactive_refusals.feature` 7-row-floor scenario, **plus** `internal/tmux` `TestPanePipeWasClosedIsTrueBeforeAnyBlockedReadCanObserveOurOwnCloseAsEOF` |
| 9  | PASS | ~5 min  | 4.48 | — |
| 10 | FAIL | 249.16s | 4.86 | `interactive_refusals.feature` 7-row-floor scenario |

Loadavg column is the mean of the 1-minute `uptime` load average sampled
every 30s during that run's wall-clock window (derived from `loadtrace.log`
timestamps and each run log's file-close time; method: window
`[prev_run_end, this_run_end]`, arithmetic mean of the 1-min figures inside
it). Host: 28 cores. A second, unrelated `ralphd` job
(`ralphd-selfdev-v09-fsm-loop`) was confirmed running on the same host for
the whole measurement (`docker ps`), which is the source of this load and is
outside this run's control.

## Failure taxonomy (three distinct causes, not one)

1. **`attach_scroll.feature:11` (@requirement-48-wheel-scrolls-attached-pane-without-typing)**
   hangs the whole `TestFeatures` process to Go's hard 10-minute test timeout
   (runs 2, 5), stuck on `Then deck client "A" attached pane shows the top of
   the scrollback`. This is the SAME scenario task 091's own full-suite run
   hit under a concurrent `ralphd` job (recorded in task 091's notes/handoff
   notes.md already), and the same scenario task 075 saw once. It has never
   been seen to fail deterministically at low/idle load in any isolated
   rerun on record.

2. **`interactive_refusals.feature`'s 7-row-floor scenario**
   (`entering interactive mode is refused while the preview box has fewer
   than 7 inner rows`) fails with `after scenario hook failed: ... did not
   show "7-row floor" within 5s: timed out waiting for frame ... context
   deadline exceeded` (runs 6, 7, 8, 10 — the majority of this run's
   failures). This is also already on record in notes.md's standing
   pre-existing-flakes list as a load-correlated PTY timeout, previously
   seen by task 075.

3. **`internal/tmux`'s `TestPanePipeWasClosedIsTrueBeforeAnyBlockedReadCanObserveOurOwnCloseAsEOF`**
   (run 8 only) — the documented close-race flake, sibling of the
   `TestPanePipeReceivesGenuineEOFOnDisplacement...` family already on
   record. Task 075 confirmed 3/3 isolated-green for this exact test before
   declaring the final commit; this run's single failure is consistent with
   "genuinely intermittent, not deterministically broken," not a regression.

None of these three is new: all three are already named in notes.md's
"Open pre-existing flakes" list or task 075's own notes before this
measurement. No scenario was excluded, no re-running was done to chase a
clean streak, and no `@flaky`/`@nightly` tag was added to make any of them
disappear from the default run — `defaultTags` is unchanged (verified above).

## Load correlation

The per-run mean loadavg (3.68 - 4.86) is lower than the loadavg (6.18-7.28)
Phase 3's I-1 measurement (task 004) used to reproduce the original
requirement-29 keystroke-drop bug, yet still produced a much worse rate
(4/10) than that same measurement's baseline runs at idle load (1.80-2.56,
10/10 clean, e.g. task 075's own single-run citations). The correlation is
therefore not a hard threshold effect at a specific loadavg number, but the
result of *any* sustained concurrent load from the second `ralphd` job
sharing this host's 28 cores for the ~50-minute duration of this
measurement — consistent with the PTY-scheduling-sensitivity class already
named in notes.md, just triggered here at a lower observed 1-minute average
than previously recorded (short scheduling spikes are not visible in a
1-minute rolling average).

## Bar verdict

**The 10/10 bar is missed: 4/10.** Per this task's own successCriteria, this
is published as measured, with no re-run to manufacture a clean streak, no
scenario exclusion, and `defaultTags` unchanged. All three failure causes are
pre-existing, already-documented flakes (not new regressions introduced by
task 075 or any commit before it) — but the measured pass rate at this run's
prevailing (shared-host, moderately loaded) conditions is 4/10, not 10/10,
and that number stands as the honest result of this task.
