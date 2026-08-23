# I-18: full-suite sweep citation (task 023)

## Run

- Command: `ci/run.sh go test -p=1 -count=1 ./...`
- Commit under test (clean tree, `git status --short` empty before and after):
  `12300a80fa29c39f60474c848243c0ba4cca2bb4`
- Started (UTC): 2026-08-23 18:02:07 / Finished: 2026-08-23 18:08:57 (wall clock includes the
  sibling-container `docker run` startup this iteration's polling measured; the `go test` process
  itself reports `features 255.191s` plus ~10s for the other packages).
- `nproc` (host container): 28
- `loadavg` samples taken during the run: 2.12 (start), 2.44/2.82/2.62 (near the end,
  `uptime` at 18:08:57 read `2.44, 2.82, 2.62`).
- Exit status: **1 (FAIL)** — `github.com/n-orlov/deck/features` failed; every other package
  (`cmd/deck`, `cmd/fake-claude`, `cmd/fake-pi`, `internal/agent`, `internal/audit`,
  `internal/config`, `internal/hookrecv`, `internal/service`, `internal/store`, `internal/theme`,
  `internal/tmux`, `internal/tui`) passed.
- Full raw output committed verbatim at `docs/reports/phase3d-i18-fullsuite-sweep.log` (5527 lines).
- Baseline for comparison (planner, task 119's diff in the tree, not this commit): EXIT=0 in 226s
  at load 1.8. This run is EXIT=1 in ~255s (features package) at load ~2.1–2.8 — slower and higher
  load than the baseline, and this run's tree additionally carries Part I tasks 001–022 (more
  scenarios: `features` alone is now 261 real scenarios vs whatever the baseline measured).

## Scenario-level result

`godog`'s own tally (log line 4924): **261 scenarios (255 passed, 6 failed)**, 2807 steps (2790
passed, 6 failed, 11 skipped). (The log also shows two more godog runs inside the same `go test`
invocation — `TestGodogRejectsUndefinedAndFailedSteps`'s own deliberate self-test fixtures at lines
5486 and 5510, which exist to prove godog itself surfaces undefined/failing steps; they are not
product scenarios and their "1 undefined" / "1 failed" are expected, not a regression.)

### The 6 failed scenarios

| # | Scenario | Feature:line | Failure |
|---|----------|---------------|---------|
| 1 | `login_shell marks captured_path advisory in the row and its detail` | `agent_session.feature:55` | `deck client "A" exits cleanly` hung, killed after 5s |
| 2 | `SIGKILL captures and sanitizes the agent pane without relaunching` | `crash.feature:15` | same — hung on exit, killed after 5s |
| 3 | `a failing pre_launch leaves visible evidence without attaching` | `crash.feature:46` | same — hung on exit, killed after 5s |
| 4 | `a stale stopped row reports its new non-leasable verdict instead of a lease conflict` | `launch_lease.feature:23` | same — hung on exit, killed after 5s |
| 5 | `an unsupported profile degrades visibly rather than lying` | `permission_modes.feature:40` | same — hung on exit, killed after 5s |
| 6 | `settings offers clearing the recent-directory history, and clearing it costs only the prefill, never a session` | `settings.feature:334` | `client "A" screen contains "cleared recent directory history"` timed out after 5s |

### Root cause, 5 of 6 (#1–#5): identical, deterministic, already on record

Every one of #1–#5's recorded input timelines ends `..."i"` then `"q"` (open detail view, then
press bare `q`). `internal/tui/rename.go`'s `updateDetailView` only matches `"i"`/`"r"` while
`m.detail` is true — a bare `q` is swallowed as a no-op instead of closing the detail view (or
quitting), so the harness's "exits cleanly" step (which sends `q` to quit) never observes the
program exit and the 5s watchdog kills it. This is the exact gotcha already recorded in the
notes file ("a bare `q` while `m.detail` is true is swallowed as a no-op by
`internal/tui/rename.go`'s `updateDetailView`") from the `agent_session.feature` case alone — this
sweep shows it is not scenario-specific, it reproduces identically in `crash.feature` (both
scenarios), `launch_lease.feature`, and `permission_modes.feature`. It is a real, deterministic
product bug (any scenario that opens detail view with `i` then tries to exit with `q` hits it),
not a load-correlated flake like I-1's keystroke drop.

**Follow-up task 079** (added to `tasks.json`, not completed in this task) carves out fixing it.

### #6: previously documented, load/timing flake

`settings.feature:334`'s clear-recent-cwd scenario timing out is the same pre-existing flake
already named in the handoff notes ("a settings clear-recent-cwd timing scenario ... hung to the
harness's timeout on separate whole-suite runs"). Not touched by this task; tracked as a residual
until whichever task next needs settings.feature green picks it up.

## Honest statement

This sweep is **not** the 10/10 stability citation (that is task 076, collected after Part I+II's
final commit, per I-20/requirement 46). It is the one-time full-suite run this run's I-18 asks for.
Exit status is 1 (FAIL), stated as measured, not re-run to manufacture a clean pass: 6/261
scenarios failed, 5 sharing one already-diagnosed root cause (task 079 carved out to fix it), 1 a
previously-documented timing flake. No scenario was skipped, retagged, or deleted to reach this
number.
