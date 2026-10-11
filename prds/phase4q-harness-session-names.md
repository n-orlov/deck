# Phase 4q — session names linked with the harness (#81, #82, #83, #84, #85)

## What the operator gets

deck's session name and the harness's own session name (the title in its resume picker and terminal title)
stay in step, in both directions, for Claude Code, Codex, Copilot CLI and Pi:

1. **Explicit.** A name typed in the create modal or `deck new --name` is set as the harness session name at
   launch, and the harness's auto-title never overwrites it.
2. **Auto.** A blank name keeps deck's dummy `<cwd-basename>-<MMDD-HHMM>`. After the first prompt, deck adopts
   the harness's generated title in its place and keeps following later changes until the user renames.
3. **Rename.** A rename in deck's `i` dialog reaches a running harness where it is safe, or is refused with a
   visible reason. A rename done inside the harness is adopted by deck and freezes following.

Pi never auto-titles, so case 2 does not apply to it; every Pi name is explicit.

## Issues in scope

#81 (umbrella, the model), #82 (Claude), #83 (Codex), #84 (Copilot), #85 (Pi). Read each in full, including
the capability matrix in #81. Where this PRD and an issue differ, this PRD wins. **Not in scope:** #73
(Kiro), a rename RPC for Codex via `codex app-server` (the issue shows it does not update the running TUI),
Claude's statusLine `session_name` channel, Claude's undocumented peer socket, editing Copilot's
`workspace.yaml` from outside, and name uniqueness (no harness enforces it).

Code to follow: `internal/agent/agent.go` (`Caps`, `LaunchInput`), `internal/agent/{claude,codex,copilot,pi}.go`,
`internal/agent/pi_hook.go` and the Pi extension source, `internal/hookrecv/`, `internal/store/store.go`
(`RenameSession`, the sessions schema and migrations), `internal/service/` (`AgentCreateInput`),
`internal/tui/tui.go` (`resolveCreateName`, the `i` dialog), `cmd/deck` (`deck new`), `cmd/fake-*`,
`internal/notify/` and `SPEC.md` §3.2, §8, §10, §11. Symbols are as of `0e2cf2c2226`; drift is expected and
is not a finding. If a reference has drifted, find the current site and carry on.

## Decisions already taken (operator, 2026-10-11)

- **Follow mode.** An auto-named row keeps tracking the harness's latest title until the user renames in deck
  or inside the harness. Either freezes it.
- **Rename push** goes into the running harness where safe, and is also applied at the next relaunch via
  `--name` where the CLI supports it.
- **Codex** gets `/rename` via send-keys right after startup behind a readiness gate. Best effort, with a
  visible `harness_rename_failed` event on failure.
- **Collisions.** An adopted or pushed title that collides with another deck row gets the `-2`, `-3` suffix,
  the same rule as the blank-name create default.

## Requirements

Each requirement names the test that fails without it. Tests live in the package named, or in a `features/`
scenario using the fake agents.

### Model (#81)

- **R248 — name provenance.** `sessions.name_source TEXT NOT NULL DEFAULT 'user'` exists with values
  `user`, `derived`, `harness`. A migration test shows every pre-existing row reads `user`. A table test with
  one case per entry point shows the stored value: typed name in the create modal → `user`; `deck new --name X`
  → `user`; blank name in either → `derived`; deck rename → `user`; harness adoption → `harness`. The
  derived default is distinguishable after creation, which `resolveCreateName` alone does not allow today.
- **R249 — launch input.** `LaunchInput` carries `Name string`, empty meaning "pass no name". A test per
  adapter shows a `derived` row's argv contains no name flag (a blank name passed as `--name` would suppress the
  auto-title), and a `user` row's argv follows R254–R257.
- **R250 — name capabilities.** `Caps` declares `NameCaps` with the four fields from #81 (`LaunchName`,
  `AutoTitle`, `PushRename`, `ObserveName`). A table test asserts the exact declared value per adapter, so a
  capability is declared and never assumed (SPEC §8). Adapters add the optional `ReadName` and `PushName` only
  where their caps claim it; a test shows the dispatcher never calls either on an adapter whose caps do not.
- **R251 — adoption.** On `Stop`, `UserPromptSubmit`, `agentStop`, `session_info_changed` and the existing
  tick (never a new poll), rows with `name_source` in (`derived`, `harness`) are updated from `ReadName`.
  A table test, one case per finite outcome, shows: empty name → unchanged; equal to current → unchanged;
  harness placeholder (`Claude Code`, `GitHub Copilot`, `π`) → unchanged; new title → renamed with
  `source=harness:<kind>` and `name_source=harness`; collision → `-2` suffix; harness reports an explicit name
  differing from deck's → adopted and `name_source=user`; a `user` row → never changed.
- **R252 — push and echo suppression.** Renaming a running row for an adapter with `PushRename=sendkeys` sends
  the keystroke plan only when status is `idle` or `running` and no dialog is focused. A table test shows
  `waiting`, `starting` and `error` are refused with an event and a footer note naming the reason. The pushed
  name is held in `pending_harness_name`; a test shows the observed echo is not adopted as a user rename and
  the field clears on confirmation and on timeout. A failed confirmation emits `harness_rename_failed` and
  never retries. A stopped row sends nothing; the name goes in at the next launch.
- **R253 — hygiene and cross-cutting.** Adopted titles are untrusted input. A test shows terminal escapes,
  control characters and a glyph prefix are stripped, the length is bounded, and no adopted title reaches a
  shell or argv. The `renamed` event carries its source and a test shows an event-hook script receives a
  harness-originated rename. A global setting `adopt_harness_names` (default on) and a per-session tri-state
  override in the `i` dialog (like `event_hook_enabled`) are tested: off means no adoption and no push. Pushed
  names are sanitised per harness: Copilot 1–100 chars with no `"`, Claude capped at 200. `launch_args` refusal
  lists cover each harness's name flag.

### Claude Code (#82)

- **R254.** `Launch` and `Resume` add `--name <n>` only for `user` rows, including the fresh `--session-id`
  relaunch path. `-n`/`--name` in `launch_args` is refused with a reason. Test: argv per row kind.
- **R255.** Adoption reads the last `ai-title` record from the hook's `transcript_path`, falling back to
  `#{pane_title}` with the `✳ ` prefix stripped and `Claude Code` ignored. `~/.claude/sessions/<pid>.json` is
  never read for auto-titles. The tail reader is bounded, reads from the end, tolerates a partial last line, and
  understands `custom-title`, `agent-name` and `ai-title` (last record wins). A non-empty hook `session_title`
  that differs from deck's name is adopted and freezes the row (`hookrecv` gains the field). Tests: a fixture
  transcript per record shape and for a truncated tail; a hook payload with and without `session_title`.
- **R256.** The Claude push plan is `C-u`, then `/rename <n>`, then `Enter` as a separate send-keys call. Test
  asserts the exact call sequence.

### Codex (#83)

- **R257.** A typed name is applied by `/rename <n>`, ~0.7 s, `Enter`, sent only when the probe reports the
  prompt ready, never on a fixed sleep, and never while an update or trust modal is up. Failure records
  `harness_rename_failed` and keeps deck's name. Test: the sequence and timing gap, and the modal-up refusal.
- **R258.** Adoption reads `threads.name` for the known conversation id from `state_5.sqlite` with `mode=ro` and
  a short busy timeout, never writing. `name IS NULL` means "none yet". A missing database, a missing table, or a
  changed column shape degrades to `pane_title` parsing, logs once, and keeps the dummy name. Tests: a fixture
  database for each of those four shapes. The explicit/generated distinction is the best-effort heuristic from
  #83 (a name that changes after deck's own push, and differs from the first generated one, freezes the row);
  SPEC says it is best effort.

### Copilot CLI (#84)

- **R259.** `-n <name>` is added on the **first launch only**, for `user` rows; `Resume` and any relaunch with an
  existing id never add it. The name is sanitised (`"` → `'`, trimmed, clamped to 100 runes). `-n`/`--name` is
  added to `copilotForbiddenExtra`. Tests: argv for first launch, resume, relaunch; refusal of a user `-n`.
- **R260.** Adoption parses `workspace.yaml` for the conversation id with a real YAML parser and a bounded read.
  `user_named: false` with a non-empty `name` is adopted as `harness`. `user_named: true` and a name differing
  from deck's is adopted and freezes the row. Quoted names (`:`, `'`, Unicode) round-trip. Tests: one fixture
  per shape. `fake-copilot` writes `name:` and `user_named:` so features cover it.

### Pi (#85)

- **R261.** `Launch` and `Resume` add `--name <n>` for `user` rows, re-passed on resume. A blank deck name
  passes nothing and the row keeps the dummy name. Tests: argv per row kind.
- **R262.** The deck extension (`deck-hook.js`) sends `session_name` on `session_start` and a new payload
  (`name`, `cleared`) on `session_info_changed`; `hookrecv` maps it to an adoption with `name_source=user`. A
  cleared name leaves deck's name unchanged. Tests: the Go side on payload fixtures, and the extension source
  asserted to subscribe to both events.
- **R263.** Deck → Pi push uses a per-session control file under the deck home keyed by conversation id, opened
  in `session_start` and closed in `session_shutdown`, calling `pi.setSessionName()`. Echo suppression follows
  R252. send-keys `/name X` is the fallback only. Tests: the control-file round trip, and that a deck push does
  not loop back as a user rename.

### Fakes, features, spec

- **R264 — fake agents and features.** `cmd/fake-claude|codex|copilot|pi` emit and persist names in the real
  record shapes. `features/` has, per harness, one scenario for each case the harness supports: explicit,
  auto-adopt (not Pi), deck rename pushed, harness-side rename adopted, and the refusal on a focused dialog.
  Each scenario is tagged and deterministic: it asserts through the fake agents, not through timing.
- **R265 — real agents.** `@real-agents` scenarios pin the upstream contracts named in the issues (Claude record
  names, Codex `state_5` table and column, Copilot `workspace.yaml` keys, Pi `session_info`). A changed contract
  must degrade to "dummy name kept", never an error; a test with the contract broken shows that.
- **R266 — spec.** `SPEC.md` describes name provenance, the adapter name contract, adoption, push, the `renamed`
  event reasons, the settings, and the capability matrix, in present tense. It is updated inside each task that
  changes behaviour, never in a docs-only task, and contradicts nothing in the live code.

## Task plan (pre-split)

Run in this order; each is its own task and commits on its own. R253's pieces ride with the task that needs them.

1. R248 — schema, migration, entry-point plumbing.
2. R249, R250 — `LaunchInput.Name`, `NameCaps`, dispatcher guard.
3. R251 — adoption engine with fake adapters.
4. R252 — push engine, echo suppression.
5. R253 — sanitiser, event source, setting and `i` dialog override, forbidden flags.
6. R254–R256 — Claude.
7. R259–R260 — Copilot.
8. R261–R263 — Pi.
9. R257–R258 — Codex.
10. R264 — features per harness (one task per harness: Claude, Codex, Copilot, Pi).
11. R265 — real-agent pins.
12. Sweep, three tasks per the standing rules, marked `sweep: true`.

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
- **After every push**, at the start of the next iteration and before picking a task, poll that push run to its
  conclusion via the Actions API. A red run is the next task, fixed at its root.
- **Waits.** A wait longer than about 85% of the iteration cap is spanned with `deferred`, never by sleeping
  past the cap.
- **The sweep's shape** (the last three tasks, in this order, all at the same `origin/main` head):
  1. the whole `go test -p=1 -count=1 ./...` tier under `ci/run.sh`, then `go build ./...`, `go vet ./...`
     and `gofmt -l` on the tracked `.go` files;
  2. `ci/run.sh ci/suite.sh ci-results && ci/run.sh ci/quality.sh ci-results`;
  3. confirm the push run at that head is `success`, then dispatch `ci.yml` **once** and read its flaky record.
  Sweep tasks commit nothing. **Nothing is committed after the sweep**: no report, record, close-out or notes
  file. `origin/main`'s head when the run ends is the swept sha. If it moved past the swept sha, the sweep is
  stale and re-runs; a stale sweep is never process. Issue closure is a comment naming the commit and the test
  node, not a tracked file. A review cure is the only thing that may land after the sweep, and it re-runs the
  whole sweep on the cured sha.
- **The CI dispatch recipe.** Use the Actions REST API with the operator's PAT from the git credentials:
  `POST /repos/n-orlov/deck/actions/workflows/ci.yml/dispatches` with `{"ref":"main"}`, then poll
  `GET /repos/n-orlov/deck/actions/runs?event=workflow_dispatch&head_sha=<sha>` to its conclusion, and read the
  run's log and artifacts. Dispatch **once**, at the final pushed sha. A run that never reached `suite`
  (runner provisioning failure, API 5xx) is not judged: dispatch another.
- **Live testing against real agents is out of scope.** The run never launches claude, codex, copilot or pi for
  real and needs no agent credentials; the operator verifies live afterwards. Upstream versions the issues were
  measured on: claude 2.1.287, codex-cli 0.156.1, copilot 1.0.93, pi 0.99.1. The run proves behaviour against
  the fake agents and fixtures; `@real-agents` scenarios are written to the existing tag's conventions and run
  only where CI already runs that tag.
- **Docker.** The job has the host docker socket for `ci/run.sh` siblings. Never remove, stop or signal a
  container by label, wildcard or pattern (the `ralphd.run` label sweep kills the job itself); only by exact
  name, and only ones this run created.
- **Engine defects to avoid by hand.** The engine cannot be patched by this run:
  - a task titled "every X" over an open domain becomes a stall-detector trap, so this PRD names its finite
    cases and so must the tasks;
  - do not signal COMPLETE while tasks are pending;
  - when an approach ends in review, read the findings before replanning;
  - after any cure pass, check `tasks.json` for a task that depends on itself and repair it by steer.

## Materiality rubric

Reject only on substance:

- a requirement's behaviour is absent or wrong in the live code;
- a test the requirement names is missing, red, or asserts the opposite of the requirement;
- the spec contradicts the live code;
- a secret is exposed.

Everything else verifies, with the gap recorded as a residual note: wording, form, provenance, process, and
**any number or claim in prose outside the spec** (commit messages, notes, issue comments, run artifacts).
Prose outside the spec is never a blocking ground. Working clauses:

- file the rule, not the example — a finding names the general rule a bullet stands for;
- read the direction of the evidence — a log line that could support either side is not evidence for either;
- a review after a cure pass confirms the cure landed and does not re-audit the product from scratch;
- a stale sweep is never process.

## Non-negotiables

- **No weakened test.** A test pinning behaviour this phase changes is rewritten in the same commit, naming the
  covering test it becomes. Nothing else about the suite is retired.
- **No raised timeout, budget, worker count or per-test bound.** `internal/racebuild` is untouched.
- **No new required contract field** in a hook or event payload deck *receives* from an agent. The new Pi
  payload and `session_title` are optional fields; their absence leaves behaviour unchanged.
- **No gate loosened and no exception added** (`ci/quality.json` only ever tightens).
- **Existing rows and agents behave identically** except where a requirement names the change. Every existing
  row keeps its name (`name_source=user`).
- **A blank name is never passed to a harness.**
- **Rename never touches the tmux slug** (`deck_<slug>`), so adoption cannot disturb a live pane.
- **Profiles stay isolated.** No code path reads or writes another profile's config, state or socket.
- **A config reload never writes.** deck never writes to a harness's own database or transcript; the Codex
  database is opened read-only.
- **The event hook never blocks the agent or loses an event**; redaction only ever removes text.

## Out of scope and escape hatch

Out of scope is listed under "Issues in scope". If a requirement is contradictory or impossible against a
harness release (an upstream record shape moved), the run petitions it in the workspace petitions file with the
evidence, and ships the degrade-to-dummy-name behaviour of R265 for that harness meanwhile.

## Definition of Done

The behaviours above are shipped, the tests they name are green in CI at the final pushed sha, `SPEC.md` is
consistent with the code, and no secret is exposed. Nothing else.

## Budget

Iterations 400, approaches 6, wall clock 12 h, per-iteration cap 120 min. Peer gate: the operator's current
defaults from `~/.ralphd/config.yaml`.
