# Phase 4o — residual follow-ups: cross-instance reload proof, resize propagation, notify hardening, test-fixture bound

## What the operator gets

1. A running deck instance picks up a theme or settings change made in another instance **of the same
   profile**, proven against the real binary on the real paths a user takes (theme picker and settings
   view), not only by unit tests (#74).
2. Resizing the host terminal window re-fits the attached session preview and its pane at once, with no
   click needed (#75).
3. The event hook's redaction masks quoted-key, `Bearer` and `--flag` secret shapes, and a NUL byte in an
   env-bound field no longer suppresses the hook (#77, #78).
4. The `/tmp/deck-interactive-pipe-*` reclaim checks owner and mode before trusting a directory (#71 item 6).
5. The fake-agent readiness poll in the Godog fixtures has its own finite deadline and a diagnostic (#76).

## Why now

v0.2.12 shipped with these as the known residuals of phase 4n. Profiles stay **completely isolated**
(SPEC §3.4): this phase changes nothing about that, and a reload never crosses a profile boundary. The
operator's complaint on #74 is that a theme change does not reach other running instances; the working
tree's reload poller exists (`internal/tui/config_poll.go`, `internal/config/reload.go`), so the first job
is to find out, with the real binary, whether any same-profile path still misses.

All new code meets the Phase 4j gates from its first commit (coverage total ≥ 85% and every package ≥ 80%,
CRAP ≤ 10 per function, golangci-lint at zero, govulncheck, Trivy). No gate is loosened, no exception added.

## Issues in scope, and the sha

#74, #75, #76, #77, #78, and **item 6 only of #71** (items 1–5 and 7 are done). **Not in scope: #73.**

Symbols are as of `7ddbaf3d8f8`; **drift is expected and is not a finding**: find the current site and
carry on. Code to follow: `internal/tui/config_poll.go`, `internal/tui/theme_picker.go`,
`internal/tui/settings.go`, `internal/config/reload.go`, `internal/notify/{redact,payload,notify,detach}.go`,
`internal/service/{eventhook,reconcile}.go`, `internal/tmux/pipe.go`, `internal/tui/` and
`internal/tmux/` resize paths (commit `9cc5ca92c07`), `features/fake_agent_feature_test.go`,
`features/assertions_test.go`, `SPEC.md` §3.4, §6.5, §10, §11.

## Standing rules

- **Test recipes, verbatim:**
  - one package: `ci/run.sh go test -count=1 ./internal/<pkg>/`
  - one scenario file: `DECK_GODOG_PATHS=<file>.feature ci/run.sh go test -count=1 ./features/ -run TestFeatures`
  - lint: `ci/run.sh ci/lint.sh`; the gates: `ci/run.sh ci/quality.sh`
  - the whole suite: `ci/run.sh ci/suite.sh`, never run in parallel with another heavy command.
  - Always run from a shell with no `DECK_*` variable set.
- **Only the final sweep runs a whole tier.** Every other task runs the narrowest recipe that covers what
  it touched, plus `ci/run.sh ci/lint.sh` before every push.
- **Commits are made per task and pushed to `main`.** Title `<area>: <imperative summary> (task NNN, R###)`.
- **After every push**, at the start of the next iteration and before picking a task, poll that push run
  to its conclusion via the Actions API. A red run is the next task, fixed at its root.
- **Waits.** A wait longer than about 85% of the iteration cap is spanned with `deferred`, never by
  sleeping past the cap.
- **The sweep's shape** (the last three tasks, in this order, all at the same `origin/main` head):
  1. the whole `go test -p=1 -count=1 ./...` tier under `ci/run.sh`, then `go build ./...`,
     `go vet ./...` and `gofmt -l` on the tracked `.go` files;
  2. `ci/run.sh ci/suite.sh ci-results && ci/run.sh ci/quality.sh ci-results`;
  3. confirm the push run at that head is `success`, then dispatch `ci.yml` **once** and read its flaky
     record.
  Sweep tasks commit nothing. **Nothing is committed after the sweep.** If `origin/main` moved past the
  swept sha, the sweep is stale and re-runs. A stale sweep is never process.
- **The CI dispatch recipe.** Use the Actions REST API with the operator's PAT from the git credentials:
  `POST /repos/n-orlov/deck/actions/workflows/ci.yml/dispatches` with `{"ref":"main"}`, then poll
  `GET /repos/n-orlov/deck/actions/runs?event=workflow_dispatch&head_sha=<sha>` to its conclusion, and
  read the run's log and artifacts. Dispatch **once**, at the final pushed sha. A run that never reached
  `suite` (runner provisioning failure, API 5xx) is not judged: dispatch another.
- **Engine defects to avoid by hand.** The engine cannot be patched by this run:
  - a task titled "every X" over an open domain becomes a stall-detector trap, so this PRD names its
    finite cases and so must the tasks;
  - do not signal COMPLETE while tasks are pending;
  - when an approach ends in review, read the findings before replanning;
  - after any cure pass, check `tasks.json` for a task that depends on itself and repair it by steer.


## Non-negotiables

- **No weakened test.** A test pinning behaviour this phase changes is rewritten in the same commit,
  naming the covering test it becomes. Nothing else about the suite is retired.
- **No raised timeout, budget, worker count or per-test bound.** `internal/racebuild` is untouched.
- **No new required contract field** in a hook or event payload deck *receives* from an agent.
- **No gate loosened and no exception added** (`ci/quality.json` only ever tightens).
- **Existing agents behave identically** except where a requirement names the change.
- **Profiles stay isolated.** No code path reads or writes another profile's config, state or socket.
- **A config reload never writes.**
- **The event hook never blocks the agent or loses an event**; redaction only ever removes text.

## Ground rules

- **No path is protected in this phase.** The operator authorises the job to edit any file it needs,
  including `SPEC.md`, `prds/`, `install.sh`, `ci/Dockerfile` and `.github/workflows/`. Edits stay within
  what a requirement needs: this is not licence to loosen a gate, weaken a test or change a release
  rule other than as R236 states. `SPEC.md` is updated inside each task that changes behaviour.
- **Never touch the operator's live state:** `~/.local/share/deck/`, `~/.config/deck/`,
  `~/.local/state/deck/`, `~/.claude/`, `~/.copilot/`, any `tmux -L deck` or `tmux -L deck-*` server.
  Tests use temp `HOME`/`XDG_*`/`DECK_HOME` and private sockets.
- **No test spawns anything but a capture script.** The event-hook tests use a small script that records
  argv, selected env and stdin to a temp file. No test calls a real notification service.
- **Docker is for `ci/run.sh`, `ci/stability.sh` and the gate scripts, and nothing else.** No
  `docker prune`, no wildcard or label-filtered `rm`/`rmi`, never signal by pattern (resolve a pid,
  verify it, then signal that pid). Never touch the `deck-ws-*` runners.
- **No paperwork.** No report, findings, audit or close-out file in the repo, no `docs/DELIVERY-LOG.md`
  row. Measurements and run ids go to `/run/ralphd/artifacts`.
- **GitHub scope.** The job may push to `main`, read the Actions API, dispatch `ci.yml` on `main`, and
  **comment** on #74, #75, #76, #77, #78 and #71 with the commit and the test node that closes each requirement. It does
  **not close issues**: the operator does. No branches, PRs, tags or releases; no cancelling, re-running
  or deleting runs; **no change to settings, rulesets, environments, Pages, runners or secrets** (R236's
  ruleset is documented, never applied).
- **CI must stay green.** No red or rerun-rescued run on `main` is acceptable.


## Requirements

### #74 — cross-instance reload, proven on the real path

**R237 — reproduce first, with the real binary.** Two real `deck` processes of the **same profile** on one
temp `DECK_HOME` (and a second pair on a named profile) run under a private tmux socket. In one, change the
theme through **each user path**: the theme picker, and the settings view's theme field. The other process
must show the new theme within the poll interval (the test may shorten it only through
`DECK_CONFIG_POLL_MS`). A third process of a **different** profile must not change. If this already holds,
the deliverable is that BDD scenario, red on the code without the poller; if any path misses, fix it at the
root (a path that never calls the save that writes `config.toml`, a poller never started, a fingerprint that
ignores the file the path writes, a write that does not change mtime or size) and keep the scenario.
Pre-split: (a) reproduction scenario for the theme picker path, (b) the settings-view path and the
cross-profile isolation case, (c) any fix the reproduction demands.

**R238 — the other `[ui]` keys.** The same real-binary scenario covers one key each of `ascii`, `mouse`
and `sort_order` (the finite set; the settings-schema classification test from phase 4n stays). SPEC §6.5
states that an instance started by a binary older than the reload feature must be restarted once, and
that a reload never crosses a profile.

### #75 — host terminal resize

**R239 — resize propagates to the preview and the attached pane with no further input.** A host
`WindowSizeMsg` / SIGWINCH re-fits the preview layout and resizes the attached session's pane/pty to the
new geometry in the same update, without waiting for a click or selection event. Finite cases, each with a
test: (1) preview-only view, grow; (2) preview-only view, shrink; (3) attached interactive session, grow
and shrink; (4) a resize to the size the terminal already has stays a no-op (do not regress `9cc5ca92c07`);
(5) a resize arriving while a modal or the settings takeover is open applies when it closes. Name the
root cause in the commit (the skipped path), and add a BDD scenario that resizes the real pty of a real
`deck` process and asserts the pane size without any key or mouse input.

### #77 — redaction shapes

**R240 — `maskSecretAssignments` and its siblings mask these finite shapes**, each a table case that fails
on the current code: quoted key then separator (`{"api_key": "sk-x"}`, `"password":"hunter2"`);
`Authorization: Bearer <token>` and `Bearer <token>` alone; `--token abc`, `--password=abc`,
`--api-key abc` secret-shaped flags (with `=` and with a space); the prefixes `sk-`, `ghp_`, `github_pat_`,
`AKIA` followed by their key body, and a three-segment JWT. The already-masked shapes
(`export GITHUB_TOKEN=x`, `AWS_SECRET_ACCESS_KEY: x`) stay masked. A non-secret line (`--tokens-per-minute 5`
style prose, `author: bob`) is not altered. The redacted text still feeds `DECK_EVENT_MESSAGE`, the stdin
JSON and the stored script-output tail through the same function. SPEC §10 lists the shapes.

### #78 — NUL in env-bound fields

**R241 — every env-bound string is NUL-free.** `buildEnv` (or `sanitize`) strips `\x00` from every
env-bound field (message, session name, and every other string it sets), so `Spawn` never fails with
`environment variable contains NUL` and the script runs. Tests: a table over each env-bound field with an
embedded NUL runs the capture script and sees the event; the (kind, reason) pair is still claimed once.
Also: a relative `event_hook` path is rejected on save and surfaced as a health-view warning if found in a
loaded config (SPEC §10 states absolute paths only).

### #71 item 6 — pipe-dir reclaim

**R242 — the `/tmp/deck-interactive-pipe-*` reclaim trusts only a directory it owns.** Before reclaiming
or reusing a directory with the prefix it checks it is a real directory (not a symlink), owned by the
current uid, and mode `0700`; otherwise it leaves it alone and creates a fresh one. Tests: wrong owner
(simulated through an injected stat function), symlink, and loose mode each leave the directory untouched;
the owned `0700` directory is still reclaimed.

### #76 — fake-agent readiness bound

**R243 — the readiness wait has its own finite deadline.** The poll in `launchLongRunning` and the
SIGKILL step wait at most the existing launch budget in total (repeated pane queries do not renew it) and
on expiry return an error carrying the last pane facts. The delayed-start / SIGKILL behaviour stays.
Test: a held launcher under a context with no deadline fails within the budget with the diagnostic; the
existing delayed-start test stays green.

## Definition of done

- R237–R243 behaviours ship, each with the test its requirement names, red without the behaviour.
- `ci.yml` is green at the final pushed `origin/main` head, push run and the one dispatched run, with an
  empty flaky record; all quality gates hold with no exception added.
- `SPEC.md` and the README agree with the code for every behaviour above.
- No secret is exposed and nothing under the operator's live state was touched.
- #74, #75, #76, #77, #78 and #71 each have a comment naming the commit and the test node per requirement.

## Out of scope

#73 (Kiro); cross-profile sharing of any setting; fsnotify or any watcher other than the poll; reloading
`state.db` UI state; inbound remote control; any repo setting or release; anything not listed above.

## Escape hatch

If a requirement contradicts `SPEC.md` beyond what it may fix, cannot be met without a gate exception, or a
named test cannot be written, do not delete a test, add an exception or quietly narrow it. File a petition
with the evidence and the smallest change that resolves it, notify the operator, and carry on with
everything the petition does not block. The operator rules by amendment.

## Notifications

Use the `notify` hat tool in the same iteration as the work, a few hundred characters, never a secret:
anything that blocks work outright, each issue finished, every rejection and cure pass, the proof run's
result, and one terminal summary. A missed or late send is never a success criterion or a finding.

## Budget and shape

Iterations 200, approaches 3, `vigilant` on, a 120-minute iteration cap, a wall-clock window of about 23
hours. One implementation task per requirement (code, tests and spec together) except R237 pre-split above.
Order: R240, R241, R242, R243 (small), then R237–R238, then R239 (largest). Then the three sweep tasks; the
final sweep task is `sweep: true`.

## Materiality rubric

Reject only on substance:

- a requirement's behaviour is absent or wrong in the live code or workflows;
- a test the requirement names is missing, red, or also passes on the code without the behaviour;
- a gate is loosened, an exception is added, or a test was deleted, skipped or weakened to pass;
- any proof run in the Definition of done is red or has a non-empty flaky record;
- the spec contradicts the live code for a behaviour in this PRD;
- a secret is exposed, in the tree, a workflow log or a published report;
- a guard in the Ground rules or Non-negotiables is broken (the operator's deck directories or a
  `tmux -L deck*` server touched, an event or the agent blocked by the hook, a branch/PR/tag/release
  created, a run cancelled or re-run, an issue closed, a repo setting or ruleset changed).

The working clauses: file the rule, not the example; read the direction of the evidence (a line that
could support either side supports neither); a review after a cure pass confirms the cure landed and
does not re-audit the product; judge a hat or router by its contract, not its taste; a stale sweep is
never process.

Everything else verifies, with the gap recorded as a residual note: wording, form, provenance, process,
and **any number or claim in prose outside the spec** (commit messages, issue comments, notes, run
artifacts). There is no advisory list. One exception: a run that never reached `suite` because GitHub
failed to provision a runner, or because of an Actions API 5xx, is not judged.
