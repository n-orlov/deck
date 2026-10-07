# Phase 4m — GitHub Copilot CLI as a fifth agent kind

## What the operator gets

One GitHub issue on `n-orlov/deck`: **#72**, "Agent support: GitHub Copilot CLI". (#73, Kiro, is **out of
scope**; so is #65.)

`deck` creates, lists, restarts, resumes, kills and previews `copilot` sessions the way it does Claude,
Codex and Pi: kind `copilot`, a status model driven by hooks with a pane probe for the gaps hooks leave,
the `safe` / `edits` / `yolo` profiles, durable conversation identity, and a fake agent that lets the
whole feature suite run in CI without the real CLI or a licence.

**Issue #72 and its comments are the detailed design. Read it in full, comments included, before
starting.** The spike comment on #72 (the findings of `copilot` 1.0.93, each marked verified, documented
or inferred) is the source for every behaviour named below. Where #72 and this PRD disagree, this PRD
wins. Where either disagrees with `SPEC.md`, fix whichever is wrong in the same commit (R225).

## Why now

Phase 4j put every quality gate on with zero exceptions: coverage (total ≥ 85%, every package ≥ 80%),
CRAP ≤ 10 for every function, golangci-lint at zero findings, govulncheck, Trivy. **All new code in this
phase meets those gates from its first commit.** A function that would exceed CRAP 10 is split in the
same commit; a new file has its tests in the same commit. No gate is loosened, no exception is added.

## Issues in scope, and the sha

#72 only. **Not in scope: #65, #73.**

Symbols and file names in this PRD are as of `38f68ce0fd`. **Drift from that sha is expected and is not a
finding**: find the current site and carry on. The code the new kind follows is `internal/agent/codex.go`
(adapter), `internal/agent/probe.go` (probe rules and their `testdata/probes/*-PROVENANCE.md` convention),
`cmd/fake-codex` (fake agent), `features/codex_hooks*` and `features/fake_agent_drift*` (hook and drift
scenarios), `internal/hookrecv/receiver.go` (hook payloads → status), `cmd/deck/main.go` (registry), and
`SPEC.md` §5, §7, §8.

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
  - when an approach ends in review, read the findings before replanning.

## Non-negotiables

- **No weakened test.** A test pinning behaviour this phase changes is rewritten in the same commit,
  naming the covering test it becomes. Nothing else about the suite is retired.
- **No raised timeout, budget, worker count or per-test bound.** `internal/racebuild` is untouched.
- **No new required contract field** in a hook or event payload. The copilot hook payload is mapped onto
  the existing receiver fields; a field deck needs that the receiver lacks is optional.
- **No gate loosened and no exception added** (`ci/quality.json` only ever tightens; no allow-list,
  baseline, bulk `//nolint`).
- **The three existing adapters behave identically** (argv, env, probe verdicts, hook mapping, rendering,
  schema, keymap), except where a requirement below names the change. Their existing tests pass unchanged.
- **Deck never writes the operator's Copilot state:** nothing under `~/.copilot` (or `$COPILOT_HOME`) is
  created, edited or deleted by deck code; in particular deck never writes `trustedFolders`,
  `settings.json` or `config.json`.
- **Hooks are observational.** The copilot hook entries print nothing on stdout and exit 0, and the
  plugin never registers `permissionRequest` or `preToolUse` (a hook that can deny or approve a tool is
  out of bounds).

## Ground rules

- **No path is protected in this phase.** The operator authorises the job to edit any file it needs,
  including `SPEC.md`, `prds/`, `ci/Dockerfile` and `.github/workflows/`. Edits stay within what a
  requirement needs: this is not licence to loosen a gate, weaken a test or change a release rule.
  `SPEC.md` is updated inside each task that changes behaviour (R225).
- **Never touch the operator's live state:** `~/.local/share/deck/`, `~/.config/deck/`,
  `~/.local/state/deck/`, `~/.claude/`, `~/.copilot/`, any `tmux -L deck` or `tmux -L deck-*` server.
  Tests use temp `HOME`/`XDG_*`/`DECK_HOME`/`CLAUDE_CONFIG_DIR`/`COPILOT_HOME` and private sockets.
- **The real `copilot` binary** (installed at `~/.local/bin/copilot`, logged in) is used **only** by
  `@real-agents` scenarios (R224) and only with `COPILOT_HOME` pointing at a temp dir. No other test
  needs it, and CI never runs it.
- **Docker is for `ci/run.sh`, `ci/stability.sh` and the gate scripts, and nothing else.** No
  `docker prune`, no wildcard or label-filtered `rm`/`rmi`, never signal by pattern (resolve a pid,
  verify it, then signal that pid). Never touch the `deck-ws-*` runners.
- **No paperwork.** No report, findings, audit or close-out file in the repo, no `docs/DELIVERY-LOG.md`
  row. Measurements and run ids go to `/run/ralphd/artifacts`. The one tracked prose artefact this phase
  adds is the probe capture provenance R223 names, which the existing probe convention already requires.
- **GitHub scope.** The job may push to `main`, read the Actions API, dispatch `ci.yml` on `main`, and
  **comment** on #72 with the commit and the test node that closes each requirement. It does **not close
  issues**: the operator does. No branches, PRs, tags or releases; no cancelling, re-running or deleting
  runs; no change to settings, rulesets, environments, Pages, runners or secrets.
- **CI must stay green.** No red or rerun-rescued run on `main` is acceptable.

## Requirements

Order of work: R216, R217 (wave 1, identity and launch), R218, R219, R220 (wave 2, status), R221, R222,
R223 (wave 3, terminal and honesty), R224 (wave 4, fake agent and drift), R225 (spec, inside each task),
then the sweep. Each requirement is a behaviour plus the test that fails without it. `features/`
scenarios are named by what they assert.

### R216 — the `copilot` adapter: kind, capabilities, argv, transcript

Register `agent.NewCopilot()` beside Codex, Pi, Claude and shell in `cmd/deck/main.go`. A new
`internal/agent/copilot.go`:

1. `Kind()` is `copilot`. `Capabilities()`: profiles `safe`, `edits`, `yolo`; `AssignsConversationID`
   true; `Resumable` true; `HasTranscript` true; executable `copilot`; `TranscriptEnvKeys` =
   `["COPILOT_HOME"]`.
2. `Launch` argv is `copilot --session-id <conversation-id>` followed by the profile flags, then
   `ExtraArgs` verbatim. Profile flags: `safe` none; `edits` `--allow-tool=write`; `yolo` `--allow-all`.
   `--no-auto-update` is added to every launch.
3. **`Resume` returns the identical argv to `Launch`** for the same conversation id and profile. It never
   emits `--resume`, and no argv this adapter builds ever contains `--continue`, `--resume`, `--connect`,
   `--remote`, `--acp` or `--yolo`-as-a-bare-alias for anything but the `yolo` profile's `--allow-all`.
   (Rationale, from #72: `--resume` fails for a session killed before its first message while
   `--session-id` creates-or-resumes, so no relaunch special case and no `FreshRelauncher` is needed. The
   adapter does **not** implement `FreshRelauncher`.)
4. `TranscriptPaths` returns `<root>/session-state/<id>/events.jsonl` where `<root>` is
   `Env["COPILOT_HOME"]` when non-empty, else `<Home>/.copilot`; `ok=false` when `Home` is unknown and no
   `COPILOT_HOME` is given, when the conversation id is not a single path component, or when the file
   does not exist. Never an error, never a guess.
5. A conversation id that is not a UUID is rejected by `Launch` with an error (Copilot exits 1 on it), so
   the failure is deck's, not a dead pane's.
- **Tests (`internal/agent`):** one test per item 1-5 (argv for each of the three profiles; `Resume`
  equals `Launch` for each profile and carries `ExtraArgs`; the forbidden-flag set is absent from every
  profile's argv, mirroring `codex_forbidden_flags_test.go`; each `TranscriptPaths` branch; the non-UUID
  rejection). A `features/` scenario with the fake agent (R224): create a copilot session, restart it
  before any message, and it is running on the **same** conversation id with `--session-id` in its argv.
  A service-level test that a copilot session is never routed through the fresh-relaunch path.

### R217 — per-launch hook instrumentation through a deck-owned plugin directory

`Instrument` returns argv `--plugin-dir <dir>` and env `DECK_EXE=<DeckExecutable>`; **only for `yolo`
it also returns `COPILOT_ALLOW_ALL=true`** (it suppresses the folder-trust screen, and grants nothing the
`yolo` profile does not already grant). `Instrument` stays a pure function.

- `<dir>` is a **static, deck-owned plugin directory under deck's own data root** (`DeckHome`), never
  under `~/.copilot`. It holds a `plugin.json` and a `hooks.json`; its content is a constant of the deck
  binary. The launch path in `internal/service` (not `Instrument`) creates it idempotently before the
  launch: atomic write (temp file + rename), directory mode 0700, and a no-op when the content already
  matches. A launch whose plugin dir cannot be written degrades to **no `--plugin-dir`** (the probe
  carries the session) and surfaces the same kind of non-fatal note other degraded launches use; it never
  fails the launch.
- `hooks.json` subscribes to exactly these six events: `userPromptSubmitted`, `sessionStart`,
  `notification`, `agentStop`, `errorOccurred`, `sessionEnd`. Each entry runs
  `"$DECK_EXE" _hook` with a 5 s timeout, prints nothing to stdout and exits 0 even when `DECK_EXE` is
  unset or the hook fails. It reads the deck row identity from the pane environment the launch already
  exports (`DECK_SESSION_ID`, `DECK_HOME`, the launch generation), exactly as the other kinds' hooks do.
- The three existing kinds' instrumentation is untouched.
- **Tests:** `Instrument` unit tests per profile (argv, env, and `COPILOT_ALLOW_ALL` for `yolo` only, never
  `safe` or `edits`); a test that the plugin content subscribes to exactly the six events and registers
  neither `permissionRequest` nor `preToolUse`; the idempotent/atomic/0700 write, the unwritable-dir
  degradation (launch still succeeds, no `--plugin-dir`), and that nothing is written under a temp
  `HOME/.copilot`.

### R218 — hook payloads drive the status model

Map the six events onto the existing receiver (`internal/hookrecv`), over this **finite** set of cases:

| event | status | reason |
|---|---|---|
| `userPromptSubmitted` | running | `prompt` |
| `sessionStart` | running | the payload's `source` (`new` or `resume`) |
| `notification` with `notification_type` `permission_prompt` | waiting | `permission_prompt` |
| `notification` with `notification_type` `elicitation_dialog` | waiting | `elicitation_dialog` |
| `notification` with any other `notification_type` | no status change | none |
| `agentStop` | idle | `end_turn` or the payload's `stopReason`; the payload's `transcriptPath` is recorded as the row's transcript when it is a regular file under the resolved copilot root |
| `errorOccurred` | **no status change** by itself (errors are `recoverable:true` and repeat; the pane probe's `✗` line is what sets `error`, R219) | none |
| `sessionEnd` | stopped | the payload's `reason` |

- **Identity.** A payload whose `sessionId` differs from the row's conversation id is **not applied** to
  that row (Copilot's `/clear`, `/new`, `/resume` and `/fork` swap the live id): the row keeps its
  deck-assigned id and status, and one event-log entry records the mismatch. A payload with no
  `sessionId` is applied by the deck row identity from the environment, as for the other kinds.
- `sessionStart` does **not** fire at launch (only at the first prompt): a copilot row is `starting`
  from launch until the first hook or probe verdict, and that is not an error.
- A copilot row's `_hook` call obeys every existing `_hook` rule (budgets, the stale-binary message,
  auto-heal); none is copied or special-cased.
- **Tests (`internal/hookrecv` and `internal/service`):** one test per table row (the "no status change"
  rows assert the row is unchanged); the id-mismatch test (row unchanged, one logged entry); the
  `transcriptPath` accepted and rejected (outside the root, not a regular file); a `features/` scenario
  per status the fake agent fires (R224): running on a prompt, waiting on a permission notification and
  on an elicitation notification, idle on `agentStop`, stopped on `sessionEnd`.

### R219 — the pane probe for copilot, and abort demotion

Add copilot rules to `internal/agent/probe.go`. Verdicts, in this **precedence** (first match wins),
matched on substrings that survive an 80-column wrap, never on a full footer line:

1. `Confirm folder trust` and `Do you trust the files in this folder?` → `waiting`, reason `folder trust`.
2. `Do you want to ` and `↑/↓ to navigate · enter to select · esc to cancel` → `waiting`, `permission prompt`.
3. `Copilot needs information.` → `waiting`, `question`.
4. a footer line with `Working` and one of `esc edit prompt` / `esc interrupt` (the leading glyph is any
   of `○ ◎ ● ◉`) → `running`, `working indicator`.
5. a line starting `✗ ` between the last turn and the context line, with no `Working` footer → `error`,
   `error line`. A line starting `! ` is a warning and is **not** an error.
6. the idle footer (`· / commands`, or `@ files · # issues` with a draft typed) and none of the above →
   `idle`, `ready`.
7. otherwise empty (no verdict).

- **Abort demotion.** Copilot fires **no hook** when the user aborts a turn (`Ctrl-C`), so a row left
  `running` by `userPromptSubmitted` / `sessionStart` with a pane that shows the idle footer (rule 6) for
  the existing probe-confirmation window is demoted to `idle` by the probe. The window is the one the
  other hook-instrumented kinds already use; no new constant is invented for it.
- **Profile honesty.** A pane whose footer shows `Allow All` on a copilot session launched as `safe` or
  `edits` is reported through the **existing degraded-profile surface** (SPEC §5; the TUI's
  degraded-profile cue) with the reason that Copilot's own settings elevated the profile. The verdict is
  never the reverse (a `yolo` session showing `Manual Approval` is not flagged: `--allow-all-tools`
  legitimately shows it).
- **Fixtures.** Each rule's pane is a real capture from `copilot` 1.0.93, stored under
  `internal/agent/testdata/probes/` with a `copilot-PROVENANCE.md` stating the capture method, as the
  codex fixtures do. The captures are re-taken from the real binary under a temp `COPILOT_HOME` and a
  private tmux socket; none is invented.
- **Tests (`internal/agent`):** one test per rule 1-7 on its real capture, a **precedence** test per
  adjacent pair that could both match (trust vs permission, permission vs working, working vs error),
  the 80-column wrapped footer, the `! ` warning not being an error, the abort demotion (and that a
  genuinely working pane is not demoted), and the degraded-profile report for `safe`/`edits` showing
  `Allow All` and not for `yolo`.

### R220 — the create dialog, list, and profile surfaces know `copilot`

Over this **finite** set of surfaces, `copilot` appears wherever the other agent kinds do and nowhere it
should not:

1. the new-session dialog's agent choice, with `copilot` selectable and the profile choices `safe`,
   `edits`, `yolo`;
2. the session row's agent badge/glyph and the detail pane's agent line, rendering `copilot`;
3. `deck new` / the CLI and the config file accept `agent = "copilot"`, with an unknown kind still
   rejected with the existing error;
4. the agent-availability check (`features/agent_availability.feature`): a host with no `copilot` on
   `PATH` reports it unavailable the way a missing `codex` is, and the create dialog does not offer a
   launch that cannot start;
5. the `edits` profile's honesty note (SPEC §5) states the real mapping: `--allow-tool=write` means edits
   do not prompt but shell commands still do.
- **Tests:** one unit or `features/` assertion per surface 1-5; a test that every pre-existing kind's
  create-dialog, badge and availability behaviour is unchanged.

### R221 — the interactive preview and string filter cope with Copilot's terminal behaviour

The captured byte stream of a real `copilot` 1.0.93 start, a turn, and an exit (a fixture, captured as in
R219) is fed to the interactive grid (`internal/interactive`) and must leave a correct grid. Over this
**finite** list of behaviours:

1. the startup queries (`OSC 10/11/4 ; ? ST`, `CSI ? 996 n`, `CSI > q`, `CSI ? u`, DECRQM `CSI ? 12 $ p`
   and `CSI ? 1007 $ p`) write **no text** into the grid and produce no stray reply into the pane;
2. `OSC 0 ; <title> BEL` (BEL-terminated, not ST) titles never reach the grid, including a title that
   contains the user's prompt text and non-ASCII characters (R209's filter handles the BEL terminator);
3. the alt-screen enter/exit, scroll regions (`CSI t;b r`) and the hide/show-cursor storm (`CSI ? 25 l/h`
   around every move) leave the grid and the cursor-visible bit correct at the end of the stream: the
   cursor is **not** drawn from an intermediate hidden/shown bit;
4. mouse modes `1003` + `1006` and focus reporting `1004` set by the stream are reflected in the grid's
   reported modes, so the existing wheel/click forwarding (§11.9) treats copilot like any other
   mouse-tracking full-screen app; deck never injects `CSI I` / `CSI O`;
5. a resize while the fixture's app is "running" (the fake agent, R224, redraws on SIGWINCH) reflows the
   grid to the new size with no stale cells.
- **Tests (`internal/interactive`, `internal/tui`):** one test per item 1-5 on the fixture; terminal bytes
  outside an escape string are never altered (the existing byte-for-byte property test still passes).

### R222 — a copilot session's environment and isolation

Over this **finite** list:

1. `COPILOT_HOME` is a **declared transcript env key** (R216) and is resolved from the session's own
   env layering (SPEC §6.1), never from deck's ambient process environment;
2. a session's `COPILOT_HOME` override is honoured by both the plugin directory choice's non-use (the
   plugin dir stays under deck's data root regardless) and `TranscriptPaths`;
3. deck's launch never sets, clears or edits `COPILOT_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`,
   `COPILOT_MODEL`, `COPILOT_ALLOW_ALL` (except R217's `yolo` rule) or `HTTPS_PROXY`; the session env
   editor (SPEC §6.2) treats `COPILOT_*` like any other user-provided variable and secret-looking values
   obey §6.4;
4. a startup stall (no output at all for the launch window, the dead-proxy case) is `starting`, never
   `error`, and clears when output arrives.
- **Tests:** one per item 1-4.

### R223 — documenting the real behaviour: `@real-agents` coverage and the probe provenance

- A `@real-agents` scenario for copilot (excluded from CI like the others, run by the operator) asserts,
  against the installed `copilot` with a temp `COPILOT_HOME`: `--session-id <uuid>` creates a session
  directory; the same argv resumes it; a session killed with SIGKILL before its first message is
  relaunched by `Resume`'s argv; the plugin hooks fire for `userPromptSubmitted` and `agentStop`; and the
  six probe fixtures' key substrings still appear in a live pane. It skips with an explicit message when
  `copilot` is absent or not logged in, and never prints a token.
- `internal/agent/testdata/probes/copilot-PROVENANCE.md` (R219) states the CLI version the fixtures were
  captured with.
- **Tests:** the scenario itself, plus a unit test that the skip path triggers when `copilot` is not on
  `PATH` (so CI proves the scenario cannot fail for a missing binary).

### R224 — `fake-copilot` and the drift check

A new `cmd/fake-copilot` (modelled on `cmd/fake-codex`): a deterministic fake of the Copilot TUI and its
hook behaviour, so every copilot scenario above runs in CI.

1. It accepts the argv R216 builds (`--session-id`, `--plugin-dir`, `--allow-tool=write`, `--allow-all`,
   `--no-auto-update`) and **rejects** every flag R216 forbids, with Copilot's real error text.
2. It creates `$COPILOT_HOME/session-state/<id>/workspace.yaml` at launch and `events.jsonl` at the first
   prompt (or clean shutdown), never after SIGKILL; `--session-id` of an existing session resumes it, of a
   new valid UUID creates it, of a non-UUID exits 1.
3. It reads the plugin's `hooks.json` from `--plugin-dir` and runs the entries for the six events with
   the real payload shapes of #72 (camelCase keys, `sessionId`, `timestamp`, `cwd`, per-event fields), so
   `agentStop` carries `transcriptPath`, `notification` carries `notification_type`, and `sessionStart`
   fires at the first prompt only.
4. It renders the idle footer, the `Working` footer, the permission dialog, the question dialog, the trust
   prompt and a `✗` error line with the same substrings R219's fixtures contain, redraws on SIGWINCH, and
   emits the R221 startup sequence (queries, alt screen, mouse 1003/1006, focus 1004, BEL-terminated
   `OSC 0` titles).
5. A scripted `Ctrl-C` during work aborts the turn **without firing any hook** (the real behaviour).
- **Drift check:** `features/fake_agent_drift*` gains a copilot case asserting the fake's footer and dialog
  strings match the probe fixtures' key substrings and its hook payloads match #72's shapes, so the fake
  cannot silently diverge from what R219 matches.
- **Tests:** a unit test per item 1-5 in `cmd/fake-copilot`; the `features/` scenarios R216, R218 and R219
  name, using the fake; the drift case.

### R225 — the spec matches the code

`SPEC.md` is updated in the same commit as each behaviour change: R6's "Four agents" becomes five; §5
(the profile table and the `edits`/`yolo` honesty notes, including that user settings can elevate a safe
launch and that the trust screen is a `waiting` state); §7 (the copilot status rows, abort demotion, the
`starting` window); §8 gains a **copilot section** next to Claude/Codex/Pi (argv, plugin instrumentation,
hook table, transcript path, probe rules and their precedence); §3.1/§4 mention the `copilot` agent
value; §13 lists `fake-copilot`; the README's agent list names Copilot CLI. Everything is present tense.
- **Test:** the existing spec-consistency and README-lists tests, extended only where they enumerate agent
  kinds, pass; **no new test asserts SPEC prose**.

## Definition of done

- R216–R225 behaviours ship, each with the test its requirement names, red without the behaviour.
- `ci.yml` is green at the final pushed `origin/main` head, push run and the one dispatched run, with an
  empty flaky record; coverage, CRAP, lint, govulncheck and Trivy gates hold with no exception added.
- `SPEC.md` and the README agree with the code for every behaviour above.
- No secret is exposed and nothing under the operator's live state or `~/.copilot` was touched.
- #72 has a comment per requirement naming the commit and the test node.

## Out of scope

#65, #73 (Kiro), Copilot's `plan` and `autopilot` modes as deck profiles, `--remote`/`--connect`/`--acp`/
`copilot app`, tracking identity across `/clear`·`/new`·`/resume`·`/fork` (R218 only refuses to corrupt the
row), reading `session-store.db` for cross-session search (§12), writing Copilot's `trustedFolders`,
installing `copilot` in CI, themes, and anything not listed above.

## Escape hatch

If the run proves that a requirement contradicts `SPEC.md` beyond what R225 lets it fix, that a behaviour
in #72 does not hold in the installed `copilot` (the version has moved since 1.0.93), that a requirement
cannot be met without a gate exception, or that a named test cannot be written, it must not delete a
test, add an exception or quietly narrow the requirement. File a petition with the evidence and the
smallest change that would resolve it, notify the operator, and carry on with everything the petition
does not block. The operator rules by amendment.

## Notifications

Use the `notify` hat tool in the same iteration as the work, a few hundred characters, never a secret:
anything that blocks work outright (including a petition), each wave finished, every rejection and cure
pass, the proof run's result, and one terminal summary. A missed or late send is cured by mentioning it
in the next one; it is never a success criterion and never a finding.

## Budget and shape

Iterations 200, approaches 2, `vigilant` on, a 120-minute iteration cap, a 12-hour wall-clock window.
Each requirement R216–R224 is one implementation task (code, tests and spec together) except: R219 is
pre-split into (a) the real captures, fixtures and provenance, (b) the probe rules and their precedence
tests, (c) abort demotion and the degraded-profile report; R224 is pre-split into (a) argv, session
files and hooks, (b) rendering, startup sequence and abort, (c) the drift case; R221 is pre-split into
its items 1-2, 3-4 and 5. Then the three sweep tasks; the final sweep task is marked `sweep: true`.

## Materiality rubric

Reject only on substance:

- a requirement's behaviour is absent or wrong in the live code or workflows;
- a test the requirement names is missing, red, or also passes on the code without the behaviour;
- a gate is loosened, an exception is added, or a test was deleted, skipped or weakened to pass;
- any proof run in the Definition of done is red or has a non-empty flaky record;
- the spec contradicts the live code for a behaviour in this PRD;
- a secret is exposed, in the tree, a workflow log or a published report;
- a guard in the Ground rules or Non-negotiables is broken (the operator's deck directories, `~/.copilot`
  or a `tmux -L deck*` server touched, a hook that can approve or deny a tool, a branch/PR/tag/release
  created, a run cancelled or re-run, an issue closed).

The working clauses: file the rule, not the example; read the direction of the evidence (a line that
could support either side supports neither); a review after a cure pass confirms the cure landed and
does not re-audit the product; judge a hat or router by its contract, not its taste; a stale sweep is
never process.

Everything else verifies, with the gap recorded as a residual note: wording, form, provenance, process,
and **any number or claim in prose outside the spec** (commit messages, issue comments, notes, run
artifacts). There is no advisory list. One exception: a run that never reached `suite` because GitHub
failed to provision a runner, or because of an Actions API 5xx, is not judged.
