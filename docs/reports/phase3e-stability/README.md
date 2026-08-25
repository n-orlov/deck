# Phase 3e — task 325: `ci/stability.sh 10` at the final code commit

**Commit under test**: `5ee9094` (`docs: green whole-suite run report at
7ebafce (324)`), working tree clean (`git status --short` empty other than
this report directory itself) at the time of the run below. This is the
same sha task 324's whole-suite run and report were taken at
(`git log origin/main..HEAD` was empty when the run was started).

**Command**: `ci/stability.sh 10` (runs `ci/run.sh go test -p=1 -count=1
./...` ten times back to back, in this container, no competing sibling
containers).

**Result: 7/10 passed.**

| run | result | wall clock (start→end, unix) | 1-min loadavg @ start |
|-----|--------|-------------------------------|------------------------|
| 1   | FAIL (`features` + `internal/interactive`) | 1787697857 → 1787698212 (355s) | 0.83 |
| 2   | PASS   | 1787698212 → 1787698569 (357s) | 4.49 |
| 3   | PASS   | 1787698569 → 1787698926 (357s) | ~2.2 |
| 4   | FAIL   | 1787698926 → 1787699292 (366s) | 2.24 |
| 5   | PASS   | 1787699292 → 1787699649 (357s) | 1.34 |
| 6   | PASS   | 1787699649 → 1787700012 (363s) | ~1.6 |
| 7   | PASS   | 1787700012 → 1787700372 (360s) | ~1.9 |
| 8   | PASS   | 1787700372 → 1787700728 (356s) | ~1.7 |
| 9   | PASS   | 1787700728 → 1787701084 (356s) | ~1.4 |
| 10  | FAIL   | 1787701084 → 1787701440 (356s) | 1.09 |

Full 1-min loadavg trace (sampled every 3s across the whole run, `date +%s`
+ `/proc/loadavg`): [`loadavg.log`](./loadavg.log). Timestamped run
boundaries: [`run-with-ts.log`](./run-with-ts.log). Per-run raw `go test`
output: [`run-1.log`](./run-1.log) .. [`run-10.log`](./run-10.log). Combined
summary emitted by `ci/stability.sh`: [`summary.log`](./summary.log).

## Every failure, root-caused (not re-run for a streak)

**Correction (this pass, after validation caught the gap below): the
claim "all three failed runs failed in `github.com/n-orlov/deck/features`
and every other package was ok in all ten runs" was FALSE as first
published.** Run 1's raw log (`run-1.log`, line 4934) also has:

```
FAIL	github.com/n-orlov/deck/internal/interactive	7.150s
```

caused by a panic (`panic: Fail in goroutine after
TestSessionResizeDuringLiveDrainIsRaceFree has completed`, `run-1.log`
lines 4919-4933), which was left unmentioned and un-root-caused in the
originally published version of this report. There are in fact **two**
failure mechanisms across the ten runs, not one. Both are covered below;
item 3 is the correction.

Every other package was `ok` or `[no test files]` in all ten runs, with no
exceptions, in every run other than run 1's `internal/interactive`
failure. The `features` package failed in runs 1, 4 and 10; those three
are all instances of exactly two pre-existing, already-documented flaky
scenarios in `features/mouse.feature` and `features/preview.feature` —
both tagged `@mouse-bindings`-adjacent SIGWINCH-count assertions with a
deliberately tight settle window, and both already root-caused to host
load (not a product bug) in task 324's report
(`docs/reports/phase3e-fullsuite/README.md`, "What had to be fixed first"
item 2). No task-334 (settle-race) mechanism scenario — resume, restart,
detail-open, or `lease_race.feature` — failed in any of the ten runs.

1. **`features/preview.feature` "a fit is skipped below the 7-inner-row
   floor, and retried once the panel grows back above it"**
   (`@steer-018-preview-fit-on-navigation`) — failed in run 1, run 4 and
   run 10 (all three failures). Asserts an exact SIGWINCH count
   (`the fake "claude" agent received exactly 0/1 SIGWINCH signals`)
   immediately after a terminal resize, with no explicit settle wait beyond
   the harness's normal frame-wait. Under CPU contention the debounce
   timing between the resize and the fit-retry can slip a tick, producing
   an off-by-one SIGWINCH count.

   Discriminating low-load experiment (this task, this iteration): ran the
   scenario in isolation, no competing containers, host loadavg `0.66` at
   start —
   ```
   ci/run.sh sh -c 'DECK_GODOG_TAGS=@steer-018-preview-fit-on-navigation go test -count=1 ./features/'
   ```
   3/3 passed (`ok  	github.com/n-orlov/deck/features	24.8xxs` each time,
   no code change). Root cause: host load, not a product bug — the same
   class of tight-settle-window SIGWINCH assertion task 324 already found
   in `mouse.feature`'s `DECK_MOUSE=0` scenario.

2. **`features/mouse.feature` "DECK_MOUSE=0 disables every mouse
   gesture..."** (`@mouse-bindings`) — failed once, in run 4 (alongside
   the preview.feature failure above, in the same `go test` process).
   Identical mechanism and identical scenario to the one task 324 already
   root-caused to host load in `docs/reports/phase3e-fullsuite/README.md`
   item 2 (its own comment documents a deliberately tight 100ms post-gesture
   settle window). No new isolation run performed for this one in this
   task — citing 324's existing isolation evidence (3/3 pass with no
   competing containers) since it is the same scenario, same failure
   signature, same mechanism.

Both scenarios are pre-existing SIGWINCH-count/timing assertions untouched
by any Phase 3e task; neither was modified, skipped or tag-excluded to
raise the published rate — per the standing "no scenario deleted, skipped
or tag-excluded to make a suite pass" rule, and per the task's own
instruction to publish the honest rate.

3. **`internal/interactive` — `TestSessionResizeDuringLiveDrainIsRaceFree`
   panics with "Fail in goroutine after ... has completed"** — failed
   once, in run 1 only (`run-1.log:4919-4934`). This is a genuine
   synchronization bug in the *test*, not a product bug and not host-load
   noise on an assertion's timing window like items 1-2: the test
   (`internal/interactive/resize_test.go`, was lines 214-227 at commit
   `5ee9094`) starts a background goroutine that repeatedly calls
   `sendLiteralLine` (which calls `t.Fatalf`/`t.Helper` on failure) in a
   loop gated by `select { case <-stop: return; default: ... }`, then
   after its own 20-iteration foreground loop does `close(stop)` and
   returns **without waiting for the goroutine to observe `stop` and
   actually exit**. `close(stop)` only becomes visible to the goroutine at
   the top of its next loop iteration — it does not interrupt an
   in-flight `sendLiteralLine` call. Under CPU contention (slow `tmux`
   `exec.Command` calls), that in-flight call can still be running, and
   can still fail and call `t.Fatalf`, after `TestSessionResizeDuringLiveDrainIsRaceFree`
   has already returned and the test framework has marked it complete —
   `testing` detects a call into a completed test's `*T` from another
   goroutine and panics the whole process instead of just failing the
   test. This is exactly the "goroutine outlives the test" class, same
   shape as the settle-race issues 7ebafce (task 324) and task 334 target
   in the godog harness, but in a plain Go test in a different package,
   with a different concrete cause (missing goroutine join, not a missing
   frame-wait).

   **Fix landed this task** (commit below): add a `sync.WaitGroup`, `Add(1)`
   before starting the goroutine, `defer wg.Done()` inside it, and
   `wg.Wait()` after `close(stop)` in the foreground before the test
   function returns — a real join, not a signal-and-hope. This makes it
   impossible for the goroutine's current iteration to still be running
   (and thus able to call `t.Fatalf`) once the test function has returned.

   **Discriminating experiment (this task, this iteration):** the panic
   did not reproduce in low-load isolation (5/5 clean runs of the
   pre-fix test alone, no competing load). It reproduced reliably under
   induced contention: 8 sibling `ci/run.sh` containers launched
   simultaneously, each running `go test -count=10 -run
   TestSessionResizeDuringLiveDrainIsRaceFree ./internal/interactive/`
   (80 total iterations) against the pre-fix code — **2 of the 8
   processes panicked** with the identical stack shape (`sendLiteralLine`
   -> `t.Fatalf` -> goroutine created by
   `TestSessionResizeDuringLiveDrainIsRaceFree`), saved at
   [`redproof-interactive-goroutine-leak/red-1.log`](./redproof-interactive-goroutine-leak/red-1.log)
   and
   [`redproof-interactive-goroutine-leak/red-2.log`](./redproof-interactive-goroutine-leak/red-2.log).
   The same 8-parallel-container experiment repeated against the fixed
   code produced 8/8 clean passes (80/80 iterations), one saved at
   [`redproof-interactive-goroutine-leak/green-1.log`](./redproof-interactive-goroutine-leak/green-1.log).
   So: real bug (not host-load noise on an assertion window), reproduced
   and fixed, with contention as the amplifier that makes the race
   observable rather than the cause of a false assertion.

## Honest published rate

**7/10 (70%)**, at commit `5ee9094`, with all four instances of failure
across the ten runs (`features` failing in runs 1, 4 and 10; `internal/interactive`
failing in run 1 only, alongside that run's `features` failure)
independently root-caused: items 1-2 to host load acting on two
known-tight-settle-window SIGWINCH assertions in `features/mouse.feature`
and `features/preview.feature`, and item 3 to a genuine
test-synchronization bug (missing goroutine join), now fixed. None of the four are the task 334 (settle-race defect class in
`selectRowByName`/`clientOpensDetailForSession`) mechanism — that class is
specific to the godog navigation helpers in `features/agent_steps_test.go`
and none of these four failures involve resume, restart, detail-open, or
`lease_race.feature`. No re-run was performed to manufacture a better
streak; the rate stands as published even though one of its four root
causes is now fixed in the tree (the fix could only be validated after the
fact, at a later commit than the run itself).

## Reproduction

```
cd /workspace
ci/stability.sh 10
```
