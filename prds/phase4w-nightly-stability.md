# Phase 4w — the nightly extended build is green every night

## What the operator gets

1. The timer-triggered nightly build of `deck/ci/main` (`EXTENDED=true`: `-race`, fresh vulnerability databases,
   `ci/stability.sh 3`) passes on an unchanged tree night after night. A red nightly means a real defect, not a
   flaky test.
2. Each flake below is fixed at its cause, in the product where the product is wrong and in the harness where the
   harness is wrong. No bound is relaxed and no retry hides a failure.

## Why (evidence, 2026-11-17, not a deliverable)

Six of the eight nightlies from build 51 to build 58 failed. Build 53 and build 54 failed the Quality stage on a
stdlib govulncheck finding, which `f8eded878db` (Go 1.26.11) has since fixed. Every other failure was the Stability
stage: the full suite runs three times and all three must pass, and each night a different test failed once in
three runs. PR #135 (`228abb31323`..`7378a06e1c1`) fixed four classes: the tmux spawn window, shell promotion
before the frame, `TestAttachThroughPTY` and hook-batch overlap and masking linearity under `-race`. The classes
below either failed **after** #135 merged (builds 57, 58) or were not touched by it.

The per-run logs are archived on each build as `ci-results/stability/**/*.log`
(`GET …/job/main/<n>/artifact/ci-results/stability/`); the console holds an excerpt.

| Class | Build, run | Test | Failure |
|-------|-----------|------|---------|
| F1 | 57 run 3 | `determinism.feature` "deterministic frames, shared clock stepping, and generated identifiers" | `after scenario hook failed: normalized deterministic frames differ` |
| F2 | 58 run 3 | same scenario | `released hook for session "clock step hook": exit status 1: deck hook: post-hook liveness pass: list tmux sessions for reconciliation: tmux -L deck_test_… list-panes -a -F …: signal: killed` |
| F3 | 57 run 2 | `activity_resize_inert.feature` "a click on the lower of two idle sessions leaves the activity order unchanged, …" | step "the private tmux window for session "act-old" is resized by another process to 100x27": `private tmux window for session "act-old" is 61x27, want 100x27` |
| F4 | 49 run 1 | `cmd/deck` `TestHookBoundHoldsForFailingDeathScriptsAndAHungIncomingScriptAlone/failing_incoming_and_failing_deaths` | `event_hook_bound_test.go:123: hook exit = 1, stderr "deck hook: post-hook liveness pass: mark session \"row-2\" stopped: begin session status update: context deadline exceeded"` |
| F5 | 42 run 2 | `codex_hooks.feature` "two Codex rows in one directory adopt their own ids and report a real approval" | `production-resolved transcript path "…/rollout-….jsonl" does not match the pane's own announced transcript_path ""` |
| F6 | 48 run 3, 55 run 3 | `interactive_focus.feature` "the preview's top border names the target session …" and `themes.feature` "a built-in theme colours each of the seven §7 status tokens, …" | `client "<name>" did not show "running" within 500ms: timed out waiting for frame "running"` |

F2 and F4 share a shape: the post-hook liveness pass in `deck hook` fails the whole hook when one of its own
tmux or SQLite calls runs out of its context. F6 failed before #135, and `228abb31323` may already cover it
(shell promotion asserted in the store before the frame). R420 says how to settle that.

## Issues in scope

None filed. This PRD is the whole statement. **Not in scope:** new features, the release trust chain issues
(#105–#132), Jenkins job configuration and the `Jenkinsfile` stage layout (the stability run count stays 3), and
any test outside the classes above unless it fails in this run's own sweep or CI builds (see R421).

Code to follow (symbols as of `9ba1680e9f0`; drift is expected and is not a finding — if a reference has drifted,
find the current site and carry on): `cmd/deck` hook command and its post-hook liveness pass;
`internal/service/reconcile.go`; `internal/tmux/tmux.go` (`list-panes`, `paneFactsFormat`); `internal/store` (busy
timeout, immediate transactions); `features/determinism.feature`, `features/activity_resize_inert.feature`,
`features/codex_hooks.feature`, `features/interactive_focus.feature`, `features/themes.feature` and their step files; `ci/stability.sh`; `docs/ci.md`.

## Requirements

Every class follows the same rule. The run finds the cause from the archived logs and the code, then fixes it at
that cause:

- **Product cause** (deck does the wrong thing under load or a race): fix the product, and add a test that fails
  without the fix and makes the race **deterministic** (an injected delay, a blocked lock, a fake clock, a stubbed
  tmux that hangs). A test that only fails "sometimes" does not count.
- **Harness cause** (the test or its steps assume an ordering or timing the product never promised): fix the test so
  it waits on the product's own observable state (the store, the tmux window, a log line) instead of wall time or
  an assumed order. The fixed test still fails if the behaviour it names is absent.

Each class task also runs the affected test or scenario file 20 times in a row with the one-package or one-scenario
recipe, at least 10 of them under `-race`, and all must pass. The output goes to the run's artifacts, never into the
repository.

### Part A — the post-hook liveness pass (F2, F4)

- **R416 (a liveness-pass timeout does not fail the hook).** When the post-hook liveness pass in `deck hook` cannot
  finish one of its tmux or store calls within its bound, the hook still exits with the status its own event
  handling earned. The liveness pass logs one line naming the call that ran out, and the next reconcile or hook
  repairs what this pass skipped. The hook stays inside its documented total bound. Tests: a `cmd/deck` test with a
  stubbed tmux whose `list-panes` hangs past the bound (F2), and a test that holds the store's write lock past the
  status-update bound (F4). Each asserts the hook's exit status, its wall time inside the existing bound, and the log
  line. Both fail on `9ba1680e9f0`. If the cause turns out to be different (for example, the hook's bound is spent
  before the liveness pass starts), fix that cause instead and name it in the commit message. The tests still pin
  the two failures in the table.

### Part B — the scenarios (F1, F3, F5, F6)

- **R417 (F1: deterministic frames).** "deterministic frames, shared clock stepping, and generated identifiers"
  passes 20 of 20. Find the frame difference from build 57's archived run-3 log, and fix whichever side is
  non-deterministic: the product frame under `DECK_ASCII`/`NO_COLOR`/frozen clock, or the step's capture point.
- **R418 (F3: external resize).** The step "the private tmux window for session … is resized by another process to
  WxH" waits until tmux itself reports the new size, or deck's own preview fit no longer overrides an external
  resize, whichever the cause shows. `activity_resize_inert.feature` passes 20 of 20. R409–R412's behaviour (a
  resize is never output) is unchanged.
- **R419 (F5: Codex transcript path).** "two Codex rows in one directory adopt their own ids and report a real
  approval" passes 20 of 20. If deck resolves the transcript path before the fake Codex pane has announced one, that
  is a product race: deck must not adopt a path the pane has not announced. Prove it with a deterministic test.
- **R420 (F6: 'running' within 500 ms).** Run both scenarios 20 times on the current head. If both pass 20 of 20
  and the cause is the one `228abb31323` fixed, the task closes with that finding in its notes. Otherwise fix the
  cause by the rule above, without raising the 500 ms bound.

### Part C — anything new, spec and sweep

- **R421 (a new flake is in scope).** A test that fails in this run's own sweep, push builds or extended build, and
  passes on a re-run, is a new class. It gets its own task and is fixed by the same rule. A failure that needs
  something outside the repository (Jenkins agent provisioning, an API 5xx, a full disk) is not a flake. Record it
  in the run notes and trigger another build.
- **R422 (spec).** Where a fix changes product behaviour (R416 for certain, R419 if it is a product race), SPEC
  states the new behaviour in present tense in the same commit. Harness-only fixes do not touch SPEC.
- **R423 (the sweep).** The last three tasks, in the order given in Standing rules, all at one `origin/main` head,
  each `sweep: true`, committing nothing.

Pre-split into tasks at plan time: R416 F2; R416 F4; R417; R418; R419; R420; then the three sweep tasks. R421 tasks
are added when a new class appears. R422 is done inside each task that changes behaviour. The tasks are independent
of each other, except that R417 comes after R416 (F2 is the same scenario).

## Materiality rubric

Reject only on substance:

- a requirement's behaviour is absent or wrong in the live code;
- a test the requirement names is missing, red, or asserts the opposite of the requirement;
- the spec contradicts the live code;
- a secret is exposed.

Everything else verifies, with the gap recorded as a residual note: wording, form, provenance, process, and
**any number or claim in prose outside the spec** (commit messages, notes, issue comments, run artifacts). Prose
outside the spec is never a blocking ground.

Working clauses: file the rule, not the example; read the direction of the evidence; a review after a cure pass
confirms the cure landed and does not re-audit the product; judge a hat or router by its contract, not its taste;
a stale sweep is never process.

## Standing rules

- **Test recipes, verbatim:**
  - one package: `ci/run.sh go test -count=1 ./internal/<pkg>/` (or `./cmd/deck/`); add `-race` for the race runs
  - one scenario file: `DECK_GODOG_PATHS=<file>.feature ci/run.sh go test -count=1 ./features/ -run TestFeatures`
  - repeat a test N times: `-count=N` on the same recipe
  - lint: `ci/run.sh ci/lint.sh`; the gates: `ci/run.sh ci/quality.sh`
  - the whole suite: `ci/run.sh ci/suite.sh`, never run in parallel with another heavy command.
  - stability: `ci/stability.sh 3`, only in the sweep.
  - Always run from a shell with no `DECK_*` variable set (except `DECK_GODOG_PATHS` in the recipe above).
- **Only the final sweep runs a whole tier.** Every other task runs the narrowest recipe that covers what it
  touched, plus `ci/run.sh ci/lint.sh` before every push.
- **Commits are made per task and pushed to `main`.** Title `<area>: <imperative summary> (task NNN, R###)`.
- **CI is Jenkins, not GitHub Actions.** There are no workflows. `docs/ci.md` describes the pipeline. A push to
  `main` builds the multibranch job `deck/ci` branch `main` through a webhook, with an hourly scan as backstop.
  Jenkins is tailnet-only, at `https://jenkins.wisent-owl.ts.net/`. The API credentials are in
  `/config/creds/jenkins.env` (`JENKINS_URL`, `JENKINS_USER`, `JENKINS_TOKEN`; `set -a; . /config/creds/jenkins.env;
  set +a`). `…` below is `$JENKINS_URL/job/deck/job/ci`. Use them only as curl basic auth
  (`-u "$JENKINS_USER:$JENKINS_TOKEN"`), and never print the file, the token or an auth header. Pass `curl -g`,
  because the `tree=` queries contain brackets. A build lists two `lastBuiltRevision` SHA1s: the homelab shared
  library's checkout and the repo's. A build is of a sha when either of them equals it.
- **After every push**, at the start of the next iteration and before picking a task, find the `deck/ci/main`
  build of that sha and poll it to its result:
  - `GET …/job/main/api/json?tree=builds[number,result,building,actions[lastBuiltRevision[SHA1]]]{0,10}`
    finds the build;
  - `GET …/job/main/<n>/api/json` polls its result;
  - `GET …/job/main/<n>/consoleText` reads its log.

  A build that is not `SUCCESS` is the next task, fixed at its root (R421 if it is a new flake). If no build for
  the sha appears within 10 minutes, trigger one with `POST …/job/main/build` (it uses the parameter defaults).
- **The nightly runs at 03:17 UTC** on the same job. A nightly that lands during the run is evidence like any
  other build: read its result, and treat a failure by R421. Do not wait for one.
- **Waits.** A wait longer than about 85% of the iteration cap is spanned with `deferred`, never by sleeping past
  the cap. An extended build takes about 100 minutes.
- **The sweep's shape.** The last three tasks run in this order, all at the same `origin/main` head, and each is
  marked `sweep: true`:
  1. the whole `go test -p=1 -count=1 ./...` tier under `ci/run.sh`, then `go build ./...`, `go vet ./...` and
     `gofmt -l` on the tracked `.go` files;
  2. `ci/run.sh ci/suite.sh ci-results && ci/run.sh ci/quality.sh ci-results`;
  3. confirm that the push build at that head is `SUCCESS` and that its archived `ci-results/flaky-go.txt` and
     `ci-results/flaky-features.txt` are empty or absent
     (`GET …/job/main/<n>/artifact/ci-results/flaky-go.txt`). Then trigger **one** extended build with
     `POST …/job/main/buildWithParameters?EXTENDED=true`, poll it to its result, and require `SUCCESS` with
     `3/3 passed` in the Stability stage and empty flaky files. A build that never reached the Suite stage (an agent
     provisioning failure, or an API 5xx) is not judged; trigger another. A Stability failure is a new R421 class,
     not a reason to re-trigger until green.

  Sweep tasks commit nothing. **Nothing is committed after the sweep.** If `origin/main` moved past the swept sha,
  the sweep is stale and re-runs. A stale sweep is never process.
- **Escape hatch.** A requirement the run proves contradictory or impossible is petitioned (not silently dropped),
  with the evidence; review adjudicates the petition. A bound the run proves wrong (for example, one that no correct
  implementation can meet on the CI agent) is petitioned with measurements, never raised in a commit.
- **Engine defects to avoid by hand.** The engine cannot be patched by this run:
  - every requirement here names its finite cases, and so must the tasks;
  - do not signal COMPLETE while tasks are pending;
  - when an approach ends in review, read the findings before replanning;
  - after any cure pass, check `tasks.json` for a task that depends on itself and repair it by steer.

## Non-negotiables

- **No weakened test.** A test pinning behaviour this phase changes is rewritten in the same commit, naming the
  covering test it becomes. A flaky test is fixed, never deleted, skipped, tagged out of the stability run or
  wrapped in a retry. Nothing else about the suite is retired.
- **No raised timeout, budget, worker count or per-test bound.** The 500 ms frame waits, the hook bounds, the
  75-process spawn budget, the stability run count of 3 and its all-must-pass rule all stay as they are.
- **No new required contract field** in a hook or event payload deck receives from an agent. **No schema change.**
- **No gate loosened and no exception added** (`ci/quality.json` only ever tightens; the CRAP ceiling holds).
- **Existing behaviour is identical** except where a requirement names the change.
- **No pane text and no secret** is written to a log line, an event or a golden file.

## Budget

150 iterations, 2 approaches, 24 h wall clock, 120-minute iteration cap. Peer plan review and peer verdict gate
on, vigilant, reflect, router task selection, Docker access for the sibling toolchain, host networking (Jenkins is
tailnet-only).
