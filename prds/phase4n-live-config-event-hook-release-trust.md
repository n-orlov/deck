# Phase 4n — live config reload, the event hook, and release-trust follow-ups

## What the operator gets

Three GitHub issues on `n-orlov/deck`:

- **#74** — a running deck picks up `config.toml` and user theme changes made by another instance, within
  about 30 seconds, without a restart.
- **#65** — SPEC §10 is already rewritten (the *event hook*). This phase implements it: one user-supplied
  executable that deck spawns on session status-change events, plus its schema, config, dialogs, health
  surface and tests. `internal/notify` is a stub today.
- **#71** — the two items still open: `install.sh` verifies a build attestation, and the maintainer
  steps for a `v*` tag ruleset are written down. (Items 1, 2-reachability, 4-7 shipped in v0.2.10.)

**Read each issue in full, comments included, before starting.** Where an issue and this PRD disagree,
this PRD wins. Where either disagrees with `SPEC.md`, fix whichever is wrong in the same commit.
(#73, Kiro, is out of scope.)

## Why now

Phase 4j put every quality gate on with zero exceptions: coverage (total ≥ 85%, every package ≥ 80%),
CRAP ≤ 10 for every function, golangci-lint at zero findings, govulncheck, Trivy. **All new code in this
phase meets those gates from its first commit.** A function that would exceed CRAP 10 is split in the
same commit; a new file has its tests in the same commit. No gate is loosened, no exception is added.

## Issues in scope, and the sha

#74, #65, #71. **Not in scope: #73.**

Symbols and file names in this PRD are as of `c0efde90d7`. **Drift from that sha is expected and is not a
finding**: find the current site and carry on. Code to follow: `internal/config/` (`config.go`,
`schema.go`, `toml.go`, `toml_write.go`), `internal/tui/settings.go`, `internal/store/store.go`
(schema and migrations, `schema_v8_migration_test.go` as the migration-test pattern),
`internal/hookrecv/receiver.go`, `internal/service/` (event-write and reconcile paths), `cmd/deck/main.go`
(`_hook`), `internal/notify/doc.go`, `install.sh`, `.github/workflows/release.yml`, `ci/releasegate`,
`ci/workflowcheck`, and `SPEC.md` §4, §6, §6.5, §10, §11.

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
  naming the covering test it becomes. Nothing else about the suite is retired. The tests that pin the
  `z` snooze key and the `notify_rules`/`outbox`/`snoozed_until` schema are such tests (R233, R228).
- **No raised timeout, budget, worker count or per-test bound.** `internal/racebuild` is untouched.
- **No new required contract field** in a hook or event payload deck *receives* from an agent. The event
  hook payload deck *sends* is new and is exactly SPEC §10.1.
- **No gate loosened and no exception added** (`ci/quality.json` only ever tightens; no allow-list,
  baseline, bulk `//nolint`).
- **Existing agents behave identically** (argv, env, probe verdicts, hook mapping, rendering) except
  where a requirement names the change.
- **The event hook never blocks the agent or loses an event.** The event row is written before any
  spawn; a slow, failing or absent script cannot delay `deck _hook` past its existing bound, and
  session-end never waits for it.
- **A config reload never writes.** The reload path only reads `config.toml` and theme files.

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
  **comment** on #74, #65 and #71 with the commit and the test node that closes each requirement. It does
  **not close issues**: the operator does. No branches, PRs, tags or releases; no cancelling, re-running
  or deleting runs; **no change to settings, rulesets, environments, Pages, runners or secrets** (R236's
  ruleset is documented, never applied).
- **CI must stay green.** No red or rerun-rescued run on `main` is acceptable.

## Requirements

Numbering continues from R225.

### #74 — live config reload

**R226 — detection and live apply.** A running TUI re-reads `config.toml` and the user theme files
(SPEC §11.6) when they change, polling at an interval of at most 30 s, by comparing mtime and size (and
the theme directory's entry names, mtimes and sizes) against the last-seen values; an unchanged
fingerprint never re-parses. On a change it re-applies, without a restart, each key the settings view
writes: the `[ui]` keys in `internal/config/schema.go`'s settings schema (at least `theme`, `ascii`,
`mouse`, `sort_order`). A key that cannot apply live is named in SPEC §6.5 as restart-required, by name.
Tests: one table-driven case per settings-schema entry proving it is applied live or listed
restart-required (the table is built from the schema, so a new key fails the test until it is
classified); a theme-file edit changes the active theme; an unchanged fingerprint parses zero times.
Pre-split: (a) the poller and fingerprint, (b) apply per key and the SPEC classification.

**R227 — safety of the reload.** Four cases, each with a test:
1. an instance with an open settings edit (any field changed and not saved) is **not** clobbered: the
   pending edit survives the reload, and the reload is applied after the edit is saved or cancelled;
2. an instance's **own** save does not trigger a second apply or a visible flicker (it recognises the
   fingerprint it just wrote);
3. a half-written or invalid `config.toml` keeps the previous config, shows a one-line non-fatal
   notice in the health view, never panics and never resets a key to its default; the next valid
   write is picked up;
4. `state.db` UI state (`layout_mode`, `sidebar_width`) is neither read from nor written by the reload.
A BDD scenario against the real binary with two instances on one temp `DECK_HOME`/`XDG_CONFIG_HOME`
proves one instance's theme change reaches the other (the scenario may shorten the poll interval only
through a test-only env override that production never sets, never by raising a bound).

### #65 — the event hook

**R228 — schema.** A migration to the next store version drops `notify_rules`, `outbox` and
`snoozed_until`, and adds per-session `event_hook_enabled` (tri-state), `event_hook_events` (optional
list) and `hook_fired` (the `(kind, reason)` pairs fired in the current `notify_epoch`, cleared when
the epoch advances). Migration tests, following `schema_v8_migration_test.go`: a database at the
previous version migrates with every other column and row intact; a fresh database equals the migrated
one; a database that already has the new columns is a no-op.

**R229 — config and settings schema.** Top-level `event_hook` (path or argv, empty by default),
`event_hook_default` (bool, default off), `event_hook_events` (default `["waiting","error","ended"]`),
`event_hook_timeout` (seconds, default 3). Each is in `internal/config` with parsing, validation (an
unknown event kind or a non-positive timeout is a load error naming the key), write-back, and the
settings view; `event_hook_events` is a comma-separated text field in the settings dialog. The existing
settings/schema parity tests cover all four. Tests: parse, invalid values, write-back round trip.

**R230 — the spawner.** `internal/notify` becomes a small spawner per SPEC §10.1 and §10.3: runs the
configured executable **without a shell**, `argv[1]` = event kind, the `DECK_SESSION_*` set (same as
`pre_launch`) plus `DECK_EVENT_KIND/REASON/MESSAGE/AT`, and the versioned JSON payload on stdin; a
timeout kills the whole process group; stdout/stderr are captured to a cap; exit status and the capped
tail are returned for recording; message text is truncated and redacted per §6.4 and **withheld** for a
`sensitive` session; env values never appear in env, payload or record. Tests, one each: argv and env
and stdin contents; no shell is involved (an argument with shell metacharacters arrives verbatim);
timeout kills a script that forks a child; output cap; redaction; `sensitive` withholds `message`;
a missing or non-executable script returns a recordable error, not a panic.

**R231 — filtering and dedupe.** One dispatch function decides whether to spawn, in this order, per
SPEC §10.1-§10.3: no `event_hook` ⇒ inert (per-session fields never read); kind not in the offered set
(`started, resumed, waiting, idle, error, ended, killed`) ⇒ no spawn; effective enabled = the session's
flag, else `event_hook_default`; effective list = the session's list **replacing** the global one (no
merge); epoch dedupe via `hook_fired`. Tests, the finite cases: inert without a script; each of the
three non-offered kinds never spawns; enabled inherit/on/off × default on/off (all six combinations);
session list replaces rather than merges; the same `(kind, reason)` in one epoch spawns once, and again
after the epoch advances; a different reason in the same epoch spawns.

**R232 — who fires.** The dispatch function is called from every event-write path (SPEC §10.4):
`deck _hook` for Claude/Codex/Pi/Copilot payloads, and the running TUI/service for probe-classified
changes, reconcile-detected process death, and the user's `killed`. The event row is written **before**
the spawn; the session-end path dispatches detached with no timeout record and `deck _hook` returns
without waiting. Exit status and the output tail are stored against the event. Tests, one per path:
a hook payload, a probe change, a reconcile death, a `killed`, and the session-end path (which must
return while a deliberately slow script is still running). Pre-split: (a) `_hook` and session-end,
(b) TUI/service paths and recording.

**R233 — TUI.** Per-session `event_hook_enabled` and the allow-list are fields in the launch-inputs
editor reached from the `i` dialog and in the create dialog; they apply immediately and set no dirty
flag (§6.2). `event_hook_default`, `event_hook_events` and `event_hook_timeout` are settings-dialog
fields. The session detail and the health view show the last hook exit status and output tail, mark a
non-zero exit or timeout visibly, and probe that the configured script exists and is executable. **The
`z` snooze key is removed**: keymap, footer, help, `snoozed_until` and the snooze-duration dialog, with
the footer/help parity tests rewritten to the keymap without it, in the same commit. No other key
changes. Tests: each field round-trips through its dialog; the detail and health rendering for exit 0,
non-zero, timeout and script-missing; `z` is unbound and absent from footer and help.
Pre-split: (a) dialogs and settings fields, (b) detail/health surface and script probe, (c) `z` removal.

**R234 — help and example.** The help view documents the contract (argv, env, stdin, timeout, no retry,
dedupe, the three limits of §10.4). The README carries two example scripts (Telegram via `curl`,
desktop via `notify-send`) with the idempotency and no-retry notes. Test: the help view contains the
contract's named parts (argv, env names, stdin, timeout, no retry), and each example script in the README
passes `sh -n`.

**R235 — BDD against the real binary.** Gherkin scenarios with a capture script, covering: enable and
disable, allow-list filtering, replace-not-merge, no-script-inert, timeout, session-end not blocked,
epoch dedupe, and a TUI-originated event. SPEC §1, §3, §4, §6, §6.5, §7, §8, §11 and §13 are made
consistent with the code in the commits that change the behaviour (the §10 text already exists; fix it
where the implementation proves it wrong).

### #71 — release trust

**R236 — attestation and the tag ruleset.**
1. `release.yml` produces a build-provenance attestation for the release tarballs and `checksums.txt`
   (the GitHub artifact-attestation action, pinned by 40-hex commit SHA with the tag in a trailing
   comment per R197, with only the extra `id-token: write` and `attestations: write` permissions that
   job needs).
2. `install.sh`, after the existing checksum check, verifies the downloaded archive with
   `gh attestation verify --repo n-orlov/deck` when an authenticated `gh` is present. A verification
   **failure always aborts** with a non-zero exit and nothing installed. When `gh` is absent or
   unauthenticated it prints one line saying only the checksum was verified and continues, unless
   `DECK_REQUIRE_ATTESTATION=1`, which aborts instead. A release published before this change has no
   attestation: that case is the same as "cannot verify" (continue with the note), not a failure.
3. `docs/ci.md` states the maintainer's `v*` tag ruleset step with the exact `gh api` command, as an
   operator step this job never runs.
Tests (a fake `gh` on `PATH`, no network): verification pass installs; verification fail aborts and
installs nothing; no `gh` continues with the note; no `gh` with `DECK_REQUIRE_ATTESTATION=1` aborts; a
pre-attestation release continues; `ci/workflowcheck` fails if the attest step is unpinned or the
permissions widen beyond the two named. The first real attested release is the operator's next tag; the
job cannot tag, so it proves this with the fakes only and says so in its issue comment.

## Definition of done

- R226–R236 behaviours ship, each with the test its requirement names, red without the behaviour.
- `ci.yml` is green at the final pushed `origin/main` head, push run and the one dispatched run, with an
  empty flaky record; coverage, CRAP, lint, govulncheck and Trivy gates hold with no exception added.
- `SPEC.md` and the README agree with the code for every behaviour above.
- No secret is exposed and nothing under the operator's live state was touched.
- #74, #65 and #71 each have a comment per requirement naming the commit and the test node.

## Out of scope

#73 (Kiro); inbound remote control (§10.5); debounce; fsnotify or any watcher other than the poll;
reloading `state.db` UI state; applying the tag ruleset or any repo setting; signing with a key the
operator would have to custody; a notification client, channel type, template or outbox of any kind;
anything not listed above.

## Escape hatch

If the run proves that a requirement contradicts `SPEC.md` beyond what it may fix, cannot be met without
a gate exception, or that a named test cannot be written, it must not delete a test, add an exception or
quietly narrow the requirement. File a petition with the evidence and the smallest change that would
resolve it, notify the operator, and carry on with everything the petition does not block. The operator
rules by amendment.

## Notifications

Use the `notify` hat tool in the same iteration as the work, a few hundred characters, never a secret:
anything that blocks work outright (including a petition), each issue finished, every rejection and cure
pass, the proof run's result, and one terminal summary. A missed or late send is cured by mentioning it
in the next one; it is never a success criterion and never a finding.

## Budget and shape

Iterations 200, approaches 3, `vigilant` on, a 120-minute iteration cap, a wall-clock window of about
23 hours. Each requirement R226-R236 is one implementation task (code, tests and spec together) except
those pre-split above: R226 into (a)-(b), R232 into (a)-(b), R233 into (a)-(c). Order: #74, then #71,
then #65 (the largest). Then the three sweep tasks; the final sweep task is marked `sweep: true`.

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
