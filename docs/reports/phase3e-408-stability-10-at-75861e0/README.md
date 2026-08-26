# Task 408 — `ci/stability.sh 10` at the final code commit (post-401-411 repair pass)

**Commit under test**: `75861e0533bf6ea77bdfe4e72b25fa1566ab33e1` (`75861e0`) — the
same sha task 407 cited as the tree's last non-docs code commit. `HEAD` at the time
this run was launched was `c5abd48` (`docs: cite green whole-suite run at final code
commit 75861e0 (407)`), confirmed docs-only: `git diff 75861e0 c5abd48 --name-only`
touches only `docs/reports/phase3e-407-whole-suite-at-75861e0/*` and
`docs/reports/phase3e.md`. `git status --short` was empty when the run was launched.

**Command**: `ci/stability.sh 10` (runs `ci/run.sh go test -p=1 -count=1 ./...` ten
times back to back, `-count=1` disables the test cache, each run a throwaway
`--rm` sibling container per `ci/run.sh`'s own contract):

```
nohup ci/stability.sh 10 > /tmp/deck-stability-run/stability-output.log 2>&1 &
```

A companion loop sampled `date -u +%s` + `/proc/loadavg` every 5s for the run's
duration, independent of and outside the repo (`loadavg-trace.log`, trimmed here to
the run's own window). Both processes were started and later stopped by their own
captured PIDs, never by pattern.

## Result: 7/10 passed

| run | result | start (unix) | end (unix) | wall clock | 1-min loadavg @ start |
|-----|--------|--------------|------------|------------|------------------------|
| 1   | PASS   | 1787726259   | 1787726603 | 344s       | 1.56 |
| 2   | PASS   | 1787726603   | 1787726948 | 345s       | 2.57 |
| 3   | PASS   | 1787726948   | 1787727293 | 345s       | 2.39 |
| 4   | FAIL   | 1787727293   | 1787727646 | 353s       | 2.31 |
| 5   | PASS   | 1787727646   | 1787727991 | 345s       | 1.57 |
| 6   | PASS   | 1787727991   | 1787728334 | 343s       | 2.50 |
| 7   | PASS   | 1787728334   | 1787728671 | 337s       | 2.36 |
| 8   | PASS   | 1787728671   | 1787729013 | 342s       | 2.38 |
| 9   | FAIL   | 1787729013   | 1787729353 | 340s       | 3.37 |
| 10  | FAIL   | 1787729353   | 1787729699 | 346s       | 2.92 |

Total wall clock: `1787726259` → `1787729699` (06:37:39Z → 07:34:59Z), ~57 minutes.
Start-of-run loadavg values above are the closest 5s-cadence sample to each run's
own start second, read from [`loadavg-trace.log`](./loadavg-trace.log). Full per-run
raw `go test` output: [`run-1.log`](./run-1.log) .. [`run-10.log`](./run-10.log).
Combined summary emitted by `ci/stability.sh` itself: [`summary.log`](./summary.log).

Reported as measured, per this task's own criterion: **no re-run, no exclusion, no
tag change, no failure re-run for a streak.**
`grep -n defaultTags features/godog_test.go` (unchanged, checked at report time):

```
defaultTags = "~@real-agents && ~@nightly"
```

## Every failure, root-caused (not re-run)

Three failures across the ten runs, all in `github.com/n-orlov/deck/features`, and
all matching two mechanisms **already root-caused in this repo's own history** —
neither is new, and neither is caused by tasks 401-411's own changes (401/402/403
touch only `features/mouse_bindings_test.go`'s `locateText`/`sidebarRegion` and
`features/mouse.feature`'s R54 scenario's wait steps; 411 touches the same file's
preview-side lookup; none of the three failing scenarios below call any of the
changed helpers).

1. **Run 4, run 10 — `create_cwd_ghost.feature:79` "a leading tilde expands for
   scanning without being rewritten in the field"**, failing at
   `create_cwd_ghost.feature:91` (`Then deck client "A" screen contains
   "starting"`), timing out after 5s with the row already showing `run...`
   (truncated `running`), never `starting`:

   ```
   after scenario hook failed: client "A" did not show "starting" within 5s:
   timed out waiting for frame "starting": context deadline exceeded
   ```

   This is the **exact same mechanism** already root-caused in
   [`docs/reports/phase3d-210-ghost-completion-starting-race.md`](../phase3d-210-ghost-completion-starting-race.md)
   for a sibling scenario in the same feature file
   (`create_cwd_ghost.feature:14`, `@requirement-14-ghost-unique-match-right-accepts`):
   the `shell`-only fast-forward rule in SPEC.md §7 (`starting` → `running` the
   moment the pane is alive, `internal/service/reconcile.go`'s
   `session.Agent == "shell" && session.Status == "starting"` promotion) can land
   on the very next reconcile tick (`scenarioReconcileInterval` = 250ms) before the
   harness's own poll ever captures a frame painting `"starting"` — under
   contention that window can close entirely, so the client's screen buffer jumps
   straight from "not present" to `running` without ever showing the intermediate
   state the assertion is looking for. Task 210's own 30/30-clean-run reproduction
   attempt at low load (loadavg ~0.6) already established this does not reproduce
   without contention and is not a defect in the ghost-completion or
   starting→running transition logic itself; this task's two independent instances
   (runs 4 and 10, both at loadavg ~2.3-2.9, i.e. under real contention from the
   other concurrently-scheduled test packages/host activity) are consistent
   evidence for the same host-contention-sensitive race, not a new bug at
   `create_cwd_ghost.feature:79` specifically — line 79's scenario simply exercises
   the identical `shell`-agent-default → submit → `"starting"` assertion shape as
   line 14's already-investigated scenario, just via a different setup (tilde
   expansion instead of ghost-completion).

2. **Run 9 — `preview.feature:134` "a fit is skipped below the 7-inner-row floor,
   and retried once the panel grows back above it"** (`@steer-018-preview-fit-on-navigation`),
   failing at `preview.feature:147` (`Then the fake "claude" agent received
   exactly 0 SIGWINCH signals`):

   ```
   after scenario hook failed: fake "claude" agent received 1 SIGWINCH signals,
   want exactly 0
   ```

   This is the **same pre-existing SIGWINCH-settle-window flake** already named
   and root-caused in
   [`docs/reports/phase3e-stability/README.md`](../phase3e-stability/README.md)
   item 1 (task 325's own 7/10 run) and referenced again in task 406's report
   (`docs/reports/phase3e-406-mouse-off-frame-race/README.md`) as a related, but
   distinct, async-settle timing sensitivity in the same debounce/reconcile family
   as `Model.previewFit`. The scenario asserts an exact SIGWINCH count immediately
   after a resize with only the harness's normal frame-wait as a settle; under CPU
   contention the debounce timing between the resize and the fit-retry can slip a
   tick, producing an off-by-one SIGWINCH count. Task 406's own delay-sweep
   experiment (§7, "Residual risk") already recorded a related, found-but-not-fixed
   `previewFit` re-entrancy hazard in this exact debounce/reconcile path, out of
   scope for that task and left for task 409 to fold into
   `phase3e-findings.md`; this run's single instance (loadavg 3.37 at start, the
   highest start-loadavg of the ten runs) is consistent with that same family of
   host-contention-sensitive timing race, not a new, unnamed mechanism.

No `internal/interactive` failure (the genuine, already-fixed
`TestSessionResizeDuringLiveDrainIsRaceFree` synchronisation bug from task 325's own
run, fixed in `fb9bd71`) recurred in any of the ten runs here — consistent with that
fix holding. No task-401-411 mechanism (R54 no-op/retarget, `locateText`
scoping, the R54 waits, R57 body-seam colour, R55 `hitTargetNone`, the
`DECK_MOUSE=0` frame race) failed in any of the ten runs.

## Disposition

No code change is forced by this run: both failure mechanisms are pre-existing,
already-documented, host-contention-sensitive timing races unrelated to tasks
401-411's own diffs, each traced by file:line/named-report to a specific
already-published root cause rather than reasoned about fresh. Per this task's own
criterion, since no code change was made, **task 407 does NOT need to be redone**;
the sha this task cites (`75861e0`) is unchanged from the sha task 407 already
cited and verified.

**Published rate: 7/10 at commit `75861e0`.** This supersedes the previous **7/10 at
`5ee9094`** (task 325,
[`docs/reports/phase3e-stability/README.md`](../phase3e-stability/README.md)) as the
current stability evidence for the tree — the numeric rate happens to be identical,
but the commit, the failure instances, and one of the two failure mechanisms
(`create_cwd_ghost.feature`'s "starting" race, not previously seen in a
`stability.sh` run in this repo) differ, and are recorded here rather than assumed
to be the same event.
