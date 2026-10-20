# Phase 4v — the output clock does not count a resize as output

## What the operator gets

1. Clicking, previewing or entering a session that has been idle for days does not move its row in the `activity`
   order. A deck-caused or tmux-caused resize, and the text reflow that comes with it, is never output, whichever
   deck process (or tmux client) made the resize.
2. Real output still counts. The first transcript change after a resize counts as usual.
3. Each time the output clock writes `last_output_at`, it logs the reason. The next false bump can then be proved
   from `deck.jsonl`.

## Why (evidence, 2026-10-20, not a deliverable)

A repro on real tmux 3.6b and the real `Observe` → `captureTranscriptReadings` → `agent.TranscriptCuts` path found
three ways a resize gets counted:

- **A. tmux reflows the capture without moving `window_activity`.** When a window is resized, `capture-pane`
  returns re-wrapped text about 15 ms later, but `window_activity` stays put. No tick looks inside the discount
  window, so the baseline is never refreshed. The next unrelated `window_activity` move (footer or statusline churn,
  possibly hours later) reads a new bucket, finds a different hash and writes `last_output_at`. This is the "idle
  for days, floats up on click" shape.
- **B. The discount span starts after the resize.** `OutputClock.Discount` sets `from: now`, after
  `resize-window` returns and, on interactive entry, after several more tmux round trips. `discounted()` covers a
  `window_activity` second only if `span.from <= second*1000+999`. A redraw stamped in the second before `from` is
  counted. This happened in 1 of 6 real resizes. In an already-examined bucket the miss also skips the re-baseline,
  so the next bucket counts the reflow.
- **C. A discount is visible only to the process that made it.** Discount spans live in the memory of one
  `OutputClock`. The operator runs `deck` and `deck-head` on one socket. Each runs its own clock against the shared
  store, so a resize made by one counts in the other. A real tmux client's own attach, detach or terminal resize
  goes through no deck discount at all.

The live store shows the signature: sessions with no hook and idle for 3+ days have a recent `last_output_at`, and
two unrelated sessions were bumped in the same second.

## Issues in scope

None filed. This PRD is the whole statement. **Not in scope:** any change to the `attention`, `created` or `name`
orders, the unseen marker's rules (R324/R325) other than what R409 implies, the movement fallback (R316) other
than what R409 implies, the hook path, a schema change, and Jenkins job configuration.

Code to follow (symbols as of `12ca92b506d`; drift is expected and is not a finding — if a reference has drifted,
find the current site and carry on):

- `internal/service/output_clock.go`: `OutputClock`, `Observe`, `observeNewBucket`, `observeExaminedBucket`,
  `discounted`, `Discount`, `AttachStarted`, `AttachEnded`, `outputState`.
- `internal/service/reconcile.go`: the `s.Output.Observe(…, pane.WindowActivity, …)` call site.
  `internal/tmux/tmux.go`: `paneFactsFormat` (it already reads `pane_width`/`pane_height`), `Pane`.
- `internal/tui/tui.go`: `previewFit`, `fitPreviewPane`, `previewFitStandDown`, `discountOutput`,
  `releaseForeignPreviewPin`. `internal/tui/interactive.go`: `enterInteractive`, `armInteractiveClaim`,
  `resizeInteractiveToPreview`, interactive exit. `internal/tui/mouse.go`: `clickSidebarRow`.
- SPEC §7 "The output clock", "Deck-caused output is discounted (R305)", R320. SPEC §11 the `activity` order.

## Requirements

### Part A — a size change is a baseline, never output

- **R409 (size-keyed baseline).** The output clock stores the pane size (`pane_width` × `pane_height`) with each
  transcript reading. The size comes from the same pane facts reconcile already reads. A pane whose size differs
  from the stored reading's size is re-captured **on that tick, whether or not its bucket advanced**. The new
  reading becomes the baseline and nothing is counted: no `last_output_at` write and no movement-fallback (R316)
  counted bucket. If the size read just before the capture differs from the size read just after it, the reading
  is a baseline, never a count. A transcript change at an unchanged size counts exactly as today. Finite cases,
  each a table test in `internal/service` driving the real cut and hash (`captureTranscriptReadings`) over a
  claude-style pane (`●` bullet, frame rules, `❯` composer, footer). Every test fails at the parent:
  1. resize with a reflow and no `window_activity` move, then footer churn in a later bucket: no write (mechanism
     A);
  2. resize with a reflow and `window_activity` moved in the same bucket: no write;
  3. resize with a reflow and `window_activity` moved into a new bucket: no write;
  4. resize, then real transcript output in a later bucket at the new size: one write;
  5. real output with no resize: one write (control);
  6. the size changes between the facts read and the capture: baseline, no write;
  7. a resize and back to the original size before the next tick, with an identical capture: no write;
  8. the movement fallback does not enter `running` on cases 1–3.

  Size handling is independent of the discount (R410). A resize that no discount covers, such as another
  process's, a real tmux client's or a terminal resize, never counts.
- **R410 (the discount covers the second it was caused in).** A discount span starts no later than the instant
  before deck issued the action it covers, and a `window_activity` second that overlaps the span in any way is
  covered. Concretely, every resize, attach, detach and forwarded-key site takes the instant before it issues
  the tmux call and passes it to `Discount`, `AttachStarted` or `AttachEnded`. `discounted()` treats a span as
  covering a second iff the span and `[second*1000, second*1000+999]` overlap. An examined-bucket tick whose
  `window_activity` moved inside a span re-baselines, as today. Finite cases, table tests that fail at the parent:
  1. a redraw stamped in the second before the call returned, new bucket: no write;
  2. the same in an already-examined bucket, then footer churn in the next bucket: no write;
  3. the existing same-second, inside-window and past-window controls keep their current outcomes, and past the
     window still counts.
- **R411 (every deck resize site discounts).** Each site below issues a resize discount bound by R410, or runs
  no resize. Each one has a test in `internal/tui` asserting that the discounter saw the cause for that session.
  The cases are finite:
  1. passive preview fit;
  2. interactive entry;
  3. interactive exit;
  4. `resizeInteractiveToPreview` (a layout, sidebar-width or terminal resize while interactive);
  5. the unpin in `previewFitStandDown` while a client is attached;
  6. `releaseForeignPreviewPin`.

### Part B — end to end, and provable next time

- **R412 (click is inert in `activity` order, with a real reflow).**
  - `TestClickIsInertOnStatusSeenAndPosition` gains an `activity`-order case. It uses a fake pane whose capture
    reflows when it is resized, and checks that the clicked idle row's `last_output_at` and position are
    unchanged after the click and after a later footer-churn tick.
  - One Godog scenario in the existing `features/` tmux harness covers the same thing on real tmux. Two idle
    sessions are in `activity` order and the lower one is clicked. After the click, and after the clicked
    session's statusline churns in a later bucket, the order is unchanged.
  - A second scenario covers the second process. A second deck client on the same socket resizes a session's
    window, and the first client's `activity` order does not change.
  - Both scenarios fail at the parent.
- **R413 (an output write says why).** Each `last_output_at` write logs one `output.recorded` line to the deck log
  with these fields:
  - the session id;
  - the old and new `last_output_at`;
  - `window_activity`;
  - the bucket;
  - the pane size;
  - the baseline's pane size;
  - whether a discount span was present for the second.

  A re-baseline caused by a size change (R409) logs one `output.rebaselined` line with the old and new size. No
  line carries pane text or hashes. A test asserts the fields of both lines and that no capture text appears in
  them.

### Part C — spec and sweep

- **R414 (spec).** SPEC §7's output clock and R305 paragraphs and §11's `activity` bullet state R409–R413 in present
  tense, in the same commits as the behaviour. That includes: the size-keyed baseline and that it re-captures on
  a size change without a bucket advance; that a resize from any source, deck's own or otherwise, is never output;
  the overlap rule for the discount; and the two log lines. No paragraph contradicts the code.
- **R415 (the sweep).** The last three tasks, in the order given in Standing rules, all at one `origin/main` head,
  each `sweep: true`, committing nothing.

Pre-split into tasks at plan time: R409 cases 1–3 and 6–7 (the size-keyed baseline); R409 cases 4, 5 and 8 (real
output and the movement fallback); R410; R411 cases 1–3; R411 cases 4–6; R412 Go test; R412 scenarios; R413; then
the three sweep tasks. R414 is done inside each task that changes behaviour. Order: R409 before R410; R410 before
R411; R412 after R409–R411; R413 is independent after R409.

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
  - one package: `ci/run.sh go test -count=1 ./internal/<pkg>/`
  - one scenario file: `DECK_GODOG_PATHS=<file>.feature ci/run.sh go test -count=1 ./features/ -run TestFeatures`
  - lint: `ci/run.sh ci/lint.sh`; the gates: `ci/run.sh ci/quality.sh`
  - the whole suite: `ci/run.sh ci/suite.sh`, never run in parallel with another heavy command.
  - Always run from a shell with no `DECK_*` variable set.
- **Only the final sweep runs a whole tier.** Every other task runs the narrowest recipe that covers what it
  touched, plus `ci/run.sh ci/lint.sh` before every push.
- **Commits are made per task and pushed to `main`.** Title `<area>: <imperative summary> (task NNN, R###)`.
- **CI is Jenkins, not GitHub Actions.** There are no workflows. `docs/ci.md` describes the pipeline. A push to
  `main` builds the multibranch job `deck/ci` branch `main` through a webhook, with an hourly scan as backstop.
  Jenkins is tailnet-only, at `https://jenkins.wisent-owl.ts.net/`. The API credentials are in
  `/config/creds/jenkins.env` (`JENKINS_URL`, `JENKINS_USER`, `JENKINS_TOKEN`; `set -a; . /config/creds/jenkins.env;
  set +a`). `…` below is `$JENKINS_URL/job/deck/job/ci`. Use them only as curl basic auth
  (`-u "$JENKINS_USER:$JENKINS_TOKEN"`), and never print the file, the token or an auth header. Pass `curl -g`, because the `tree=` queries contain
  brackets. A build lists two `lastBuiltRevision` SHA1s: the homelab shared library's checkout and the repo's. A
  build is of a sha when either of them equals it.
- **After every push**, at the start of the next iteration and before picking a task, find the `deck/ci/main`
  build of that sha and poll it to its result:
  - `GET …/job/main/api/json?tree=builds[number,result,building,actions[lastBuiltRevision[SHA1]]]{0,10}`
    finds the build;
  - `GET …/job/main/<n>/api/json` polls its result;
  - `GET …/job/main/<n>/consoleText` reads its log.

  A build that is not `SUCCESS` is the next task, fixed at its root. If no build for the sha appears within 10
  minutes, trigger one with `POST …/job/main/build` (it uses the parameter defaults).
- **Waits.** A wait longer than about 85% of the iteration cap is spanned with `deferred`, never by sleeping past
  the cap.
- **The sweep's shape.** The last three tasks run in this order, all at the same `origin/main` head, and each is
  marked `sweep: true`:
  1. the whole `go test -p=1 -count=1 ./...` tier under `ci/run.sh`, then `go build ./...`, `go vet ./...` and
     `gofmt -l` on the tracked `.go` files;
  2. `ci/run.sh ci/suite.sh ci-results && ci/run.sh ci/quality.sh ci-results`;
  3. confirm that the push build at that head is `SUCCESS` and that its archived `ci-results/flaky-go.txt` and
     `ci-results/flaky-features.txt` are empty or absent
     (`GET …/job/main/<n>/artifact/ci-results/flaky-go.txt`). Then trigger **one** extended build with
     `POST …/job/main/buildWithParameters?EXTENDED=true`, poll it to its result, and require `SUCCESS` with empty
     flaky files. A build that never reached the Suite stage (an agent provisioning failure, or an API 5xx) is
     not judged; trigger another.

  Sweep tasks commit nothing. **Nothing is committed after the sweep.** If `origin/main` moved past the swept sha,
  the sweep is stale and re-runs. A stale sweep is never process.
- **Escape hatch.** A requirement the run proves contradictory or impossible is petitioned (not silently dropped),
  with the evidence; review adjudicates the petition.
- **Engine defects to avoid by hand.** The engine cannot be patched by this run:
  - every requirement here names its finite cases, and so must the tasks;
  - do not signal COMPLETE while tasks are pending;
  - when an approach ends in review, read the findings before replanning;
  - after any cure pass, check `tasks.json` for a task that depends on itself and repair it by steer.

## Non-negotiables

- **No weakened test.** A test pinning behaviour this phase changes is rewritten in the same commit, naming the
  covering test it becomes. Nothing else about the suite is retired. The existing R305 discount tests and the
  passive-preview non-perturbation guarantee stay green.
- **No raised timeout, budget, worker count or per-test bound**, and no widening of the 5 s discount window.
  The fix is the size-keyed baseline, not a longer guess.
- **No new required contract field** in a hook or event payload deck receives from an agent. **No schema change.**
- **No gate loosened and no exception added** (`ci/quality.json` only ever tightens; the CRAP ceiling holds).
- **Existing behaviour is identical** except where a requirement names the change. Real output still counts, and
  the `attention`, `created` and `name` orders are untouched.
- **No pane text and no secret** is written to a log line, an event or a golden file.

## Budget

200 iterations, 2 approaches, 24 h wall clock, 120-minute iteration cap. Peer plan review and peer verdict gate
on, vigilant, reflect, router task selection, Docker access for the sibling toolchain, host networking (Jenkins is
tailnet-only).
