# I-20 / requirement 46: ten-run stability measurement after the final code commit (task 076)

## SUPERSEDING DECLARATION (task 210, this run's F1 repair pass, recorded by task 211)

**The measurement below (4/10, taken at task 076's `49b54ed`, an earlier run's final code commit)
is superseded.** This run's own review found requirement 46/I-20's substance unmet by that result
(`discovered.reviewFindings.F1` in `tasks.json`) and opened task 210 to drive the rate to 10/10.

Task 210 root-caused and fixed every failure-causing mechanism this file's original measurement
hit or that task 209's later re-measurement surfaced (a full list, each with its own fix commit,
is in task 210's own `notes` field in `tasks.json` and in the per-cause reports it cites), then
took a fresh `ci/stability.sh 10` at the resulting final code commit **`f715a56`**
(`features: fix delete-undo/restore DB-read race in kill_delete_undo.feature (210)`; docs commit
`d59ba28`). **Result: 10/10 PASS** — every one of the 14 testable packages `ok` in every one of
the 10 runs, zero `FAIL` lines anywhere across all 10 logs, host loadavg low and stable
throughout (peak 14.81 1-min, sampled repeatedly across the ~60-minute run). Full evidence — the
load trace table, per-run logs, `summary.log` — is committed under
`docs/reports/phase3d-210-stability-10of10/` (see that directory's own `README.md`).

This is the clean-streak branch of task 210's `successCriteria`, so no further root-cause
writeup was needed for that final measurement itself — but the ORIGINAL measurement below is
kept, with two corrections applied to its taxonomy, because two of its named causes were
previously (and wrongly) generalized into the standing "load-correlated PTY timeout" /
"pre-existing flake" bucket. Both corrections predate this rewrite (tasks 201 and 206) but the
labels in this file's own prose, below, were never actually fixed until now (this task, 211):

- **`interactive_refusals.feature`'s 7-row-floor scenario (this file's taxonomy item 2) is NOT a
  load-correlated PTY timeout and is NOT pre-existing.** The scenario
  (`interactive_refusals.feature:29`) was itself created in the run that authored requirement 48,
  so "pre-existing" was never accurate. Task 201 reproduced the failure on the FIRST TRY at 1-min
  loadavg 3.73 (well under this file's own load-correlation range) and the failing frame's own
  title bar read `interactive 41x6 fitted` — deck ENTERED interactive mode at a 6-inner-row box
  instead of refusing, so the scenario's wait for "7-row floor" text timed out waiting for text
  that never gets composed. Root cause: a harness resize step that returned before deck processed
  the resize, racing a floor check that ran once at entry with no re-check after a shrink. Fixed by
  tasks 202 (harness: `ScreenDriver.ResizeAndAwaitRender`) and 204 (product:
  `Model.Update`'s `tea.WindowSizeMsg` case now exits interactive mode on a shrink below the
  floor), backstopped by task 203's tmux-free unit test
  (`TestEnterInteractiveRefusesBelowTheSevenRowFloorWithoutAnyTmuxCall`). Full writeup:
  `docs/reports/phase3d-201-req48-degrade-rootcause.md`.
- **`attach_scroll.feature:11`'s wheel-scroll scenario (this file's taxonomy item 1) is also NOT a
  load-correlated PTY timeout.** Task 206 reproduced it isolated at loadavg as low as 3.7 (under
  this file's own 4.0 load-correlation threshold), root-caused it to a genuine harness pacing race
  against the real tmux server (confirmed via the server's own `#{pane_in_mode}`, not the harness
  screen emulator), and fixed it with `features/attach_scroll_test.go`'s
  `waitForCopyModeQueueToDrain`. Full writeup and 10-consecutive-isolated-run evidence:
  `docs/reports/phase3d-206-attach-scroll-hang-rootcause.md`.

The `internal/tmux` close-race family named in this file's original taxonomy item 3
(`TestPanePipeWasClosedIsTrueBeforeAnyBlockedReadCanObserveOurOwnCloseAsEOF`) and its sibling were
also separately root-caused and fixed, by task 210 part (b) — see
`docs/reports/phase3d-210-pipe-displacement-chatter-race.md` and
`docs/reports/phase3-findings.md`'s "Standing pre-existing flake list" item 1.

The rest of this file (below) is left exactly as task 076 originally wrote it, as the historical
record of what that measurement showed and what was believed about it at the time — not as a
live claim. Do not cite anything below this line as the current state of I-20; cite the
superseding declaration above, or `docs/reports/phase3d-210-stability-10of10/` directly.

---

## Original measurement (task 076, SUPERSEDED — see above)

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

1. ~~**`attach_scroll.feature:11` (@requirement-48-wheel-scrolls-attached-pane-without-typing)**
   hangs the whole `TestFeatures` process to Go's hard 10-minute test timeout
   (runs 2, 5), stuck on `Then deck client "A" attached pane shows the top of
   the scrollback`. This is the SAME scenario task 091's own full-suite run
   hit under a concurrent `ralphd` job (recorded in task 091's notes/handoff
   notes.md already), and the same scenario task 075 saw once. It has never
   been seen to fail deterministically at low/idle load in any isolated
   rerun on record.~~ **Corrected by task 206 (see the superseding declaration above): this is
   NOT a load artifact.** It is a real harness pacing race between a burst of `WheelUp` SGR
   reports and the copy-mode cancel key sent right after it, reproduced at loadavg as low as 3.7
   (below this measurement's own range) and fixed in `features/attach_scroll_test.go`. See
   `docs/reports/phase3d-206-attach-scroll-hang-rootcause.md`.

2. ~~**`interactive_refusals.feature`'s 7-row-floor scenario**
   (`entering interactive mode is refused while the preview box has fewer
   than 7 inner rows`) fails with `after scenario hook failed: ... did not
   show "7-row floor" within 5s: timed out waiting for frame ... context
   deadline exceeded` (runs 6, 7, 8, 10 — the majority of this run's
   failures). This is also already on record in notes.md's standing
   pre-existing-flakes list as a load-correlated PTY timeout, previously
   seen by task 075.~~ **Corrected by task 201 (see the superseding declaration above): this is
   NOT a load-correlated PTY timeout and NOT pre-existing** (the scenario was created in the same
   run that added requirement 48). Reproduced on the first try at loadavg 3.73; the failing
   frame's title bar reads `interactive 41x6 fitted` — deck degrades into interactive mode
   instead of refusing, so the wait times out on text that never gets composed. Fixed by tasks
   202/203/204. See `docs/reports/phase3d-201-req48-degrade-rootcause.md`.

3. **`internal/tmux`'s `TestPanePipeWasClosedIsTrueBeforeAnyBlockedReadCanObserveOurOwnCloseAsEOF`**
   (run 8 only) — the documented close-race flake, sibling of the
   `TestPanePipeReceivesGenuineEOFOnDisplacement...` family already on
   record. Task 075 confirmed 3/3 isolated-green for this exact test before
   declaring the final commit; this run's single failure is consistent with
   "genuinely intermittent, not deterministically broken," not a regression. **Later
   root-caused and fixed anyway, by task 210 part (b)** (the underlying single-`Read`-vs-chatter
   race applied to this test's two siblings too, not just this one) — see
   `docs/reports/phase3d-210-pipe-displacement-chatter-race.md`.

At the time this measurement was taken (task 076), none of these three was believed new: all
three were named in notes.md's "Open pre-existing flakes" list or task 075's own notes. No
scenario was excluded, no re-running was done to chase a clean streak, and no `@flaky`/`@nightly`
tag was added to make any of them disappear from the default run — `defaultTags` was unchanged
(verified above). **As corrected above, causes 1 and 2 were never actually load-correlated or
pre-existing — they were real, fixable defects, since fixed** (tasks 201-206); only cause 3 was a
genuine intermittent flake, also since fixed (task 210 part (b)).

## Load correlation

The per-run mean loadavg (3.68 - 4.86) is lower than the loadavg (6.18-7.28)
Phase 3's I-1 measurement (task 004) used to reproduce the original
requirement-29 keystroke-drop bug, yet still produced a much worse rate
(4/10) than that same measurement's baseline runs at idle load (1.80-2.56,
10/10 clean, e.g. task 075's own single-run citations). **This correlation
analysis is itself superseded by the corrections above**: causes 1 and 2 (the majority of this
run's 6 failures) were not actually load effects at all, so the apparent load-severity
relationship this section originally inferred does not hold. Retained verbatim as a record of
the reasoning used at the time, not as a standing claim.

## Bar verdict

**Superseded — see the declaration at the top of this file.** At the time (task 076), the 10/10
bar was missed at 4/10, published as measured with no re-run, no exclusion, and `defaultTags`
unchanged, exactly as stated below. That result is no longer the state of I-20: task 210's
follow-up measurement at the resulting final code commit (`f715a56`) is a clean **10/10**
(`docs/reports/phase3d-210-stability-10of10/`), reached only after every cause named above was
actually investigated and fixed — not manufactured by re-running until a streak appeared.

**The 10/10 bar is missed: 4/10.** Per this task's own successCriteria, this
is published as measured, with no re-run to manufacture a clean streak, no
scenario exclusion, and `defaultTags` unchanged. All three failure causes are
pre-existing, already-documented flakes (not new regressions introduced by
task 075 or any commit before it) — but the measured pass rate at this run's
prevailing (shared-host, moderately loaded) conditions is 4/10, not 10/10,
and that number stands as the honest result of this task.
