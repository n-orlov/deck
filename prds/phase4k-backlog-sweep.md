# Phase 4k — The remaining backlog: small UI fixes, mouse forwarding and selection, stale hook bindings

## What the operator gets

Seven GitHub issues on `n-orlov/deck` (#65, the event-hook rewrite, is **out of scope**):

1. **`,` closes Settings** (#63).
2. **No false alarm for the expected old-pane `session_end`** in the `i` detail pane (#64).
3. **A setting for mouse attach** (#62): with it off, a sidebar click or double-click only selects.
4. **Mouse forwarding in the interactive preview** (#67): a click is not a selection; clicks of every
   button reach the app; only a left drag selects; and a new "Click and drag to select" setting.
5. **Rectangular (block) selection** with Alt+drag, Ctrl+drag as the fallback (#66).
6. **Stale hook bindings after a schema bump** (#56): a correct message, auto-heal where it is safe,
   and a restart hint in the TUI.
7. **The two protected-path leftovers of #61**: SHA-pinned actions in `release.yml`, and a release
   gate that accepts the "suite" check from `ci.yml` only.

The issue bodies **and their comments** are the detailed design. **Read each issue in full, comments
included, before starting its requirement.** Where an issue and this PRD disagree, this PRD wins.
Where either disagrees with `SPEC.md`, fix whichever is wrong in the same commit (R206).

## Why now

Phase 4j put every quality gate on with zero exceptions: coverage (total ≥ 85%, every package ≥ 80%),
CRAP ≤ 10 for every function, golangci-lint at zero findings, govulncheck, Trivy. **All new code in
this phase meets those gates from its first commit.** A feature that needs a function over CRAP 10 is
split in the same commit; a new file needs its tests in the same commit. No gate is loosened, no
exception is added.

## Issues in scope, and the sha

#56, #61 (the two leftover items only), #62, #63, #64, #66, #67. **Not in scope: #65.**

Symbols and file names in this PRD are as of `463bef1dc9`. **Drift from that sha is expected and is
not a finding**: find the current site and carry on.

## Standing rules

- **Test recipes, verbatim:**
  - one package: `ci/run.sh go test -count=1 ./internal/<pkg>/`
  - one scenario file: `DECK_GODOG_PATHS=<file>.feature ci/run.sh go test -count=1 ./features/ -run TestFeatures`
  - lint: `ci/run.sh ci/lint.sh`; the gates: `ci/run.sh ci/quality.sh`
  - the whole suite: `ci/run.sh ci/suite.sh`, never run in parallel with another heavy command.
  - Always run from a shell with no `DECK_*` variable set.
- **Only the final sweep runs a whole tier.** Every other task runs the narrowest recipe that covers
  what it touched, plus `ci/run.sh ci/lint.sh` before every push.
- **Commits are made per task and pushed to `main`.** Title `<area>: <imperative summary> (task NNN, R###)`.
- **After every push**, at the start of the next iteration and before picking a task, poll that push
  run to its conclusion via the Actions API. A red run is the next task, fixed at its root.
- **Waits.** A wait longer than about 85% of the iteration cap is spanned with `deferred`, never by
  sleeping past the cap.
- **The sweep's shape** (the last three tasks, in this order, all at the same `origin/main` head):
  1. the whole `go test -p=1 -count=1 ./...` tier under `ci/run.sh`, then `go build ./...`,
     `go vet ./...` and `gofmt -l` on the tracked `.go` files;
  2. `ci/run.sh ci/suite.sh ci-results && ci/run.sh ci/quality.sh ci-results`;
  3. confirm the push run at that head is `success`, then dispatch `ci.yml` **once** and read its
     flaky record.
  Sweep tasks commit nothing. **Nothing is committed after the sweep.** If `origin/main` moved past
  the swept sha, the sweep is stale and re-runs. A stale sweep is never process.
- **The CI dispatch recipe.** Use the Actions REST API with the operator's PAT from the git
  credentials: `POST /repos/n-orlov/deck/actions/workflows/ci.yml/dispatches` with `{"ref":"main"}`,
  then poll `GET /repos/n-orlov/deck/actions/runs?event=workflow_dispatch&head_sha=<sha>` to its
  conclusion, and read the run's log and artifacts. Dispatch **once**, at the final pushed sha. A run
  that never reached `suite` (runner provisioning failure, API 5xx) is not judged: dispatch another.
- **Engine defects to avoid by hand.** The engine cannot be patched by this run:
  - a task titled "every X" over an open domain becomes a stall-detector trap, so this PRD names its
    finite cases and so must the tasks;
  - do not signal COMPLETE while tasks are pending;
  - when an approach ends in review, read the findings before replanning.

## Non-negotiables

- **No weakened test.** A test pinning behaviour this phase changes is rewritten in the same commit,
  naming the covering test it becomes. Nothing else about the suite is retired.
- **No raised timeout, budget, worker count or per-test bound.** `internal/racebuild` is untouched.
- **No new required contract field** in a hook or event payload.
- **No gate loosened and no exception added** (`ci/quality.json` only ever tightens; no allow-list,
  baseline, bulk `//nolint`).
- **Refactors keep the keymap, schema, rendering and timing identical** except where a requirement
  below names the change.

## Ground rules

- **No path is protected in this phase.** The operator explicitly authorises the job to edit any
  file it needs, including the paths earlier phases held read-only (`SPEC.md`, `prds/`,
  `ci/Dockerfile`, `ci/SPIKE.md`, `.github/workflows/release.yml`, `ci/releasegate/`). Edits stay
  within what a requirement needs: this is not licence to loosen a gate, weaken a test or change the
  release gate's rule beyond R205. `SPEC.md` is updated inside each task that changes behaviour (R206).
- **Never touch the operator's live state:** `~/.local/share/deck/`, `~/.config/deck/`,
  `~/.local/state/deck/`, any `tmux -L deck` or `tmux -L deck-*` server. Tests use temp
  `HOME`/`XDG_*`/`DECK_HOME` and private sockets.
- **Docker is for `ci/run.sh`, `ci/stability.sh` and the gate scripts, and nothing else.** No
  `docker prune`, no wildcard or label-filtered `rm`/`rmi`, never signal by pattern (resolve a pid,
  verify it, then signal that pid). Never touch the `deck-ws-*` runners.
- **No paperwork.** No report, findings, audit or close-out file in the repo, no
  `docs/DELIVERY-LOG.md` row. Measurements and run ids go to `/run/ralphd/artifacts`.
- **GitHub scope.** The job may push to `main`, read the Actions API, dispatch `ci.yml` on `main`,
  and **comment** on the issues in scope with the commit and the test node that closes each. It does
  **not close issues**: the operator does. No branches, PRs, tags or releases; no cancelling,
  re-running or deleting runs; no change to settings, environments, Pages, runners or secrets.
- **CI must stay green.** No red or rerun-rescued run on `main` is acceptable.

## Requirements

Order of work: R199, R200, R201 (wave 1), R202, R203 (wave 2), R204, R205 (wave 3), then the sweep.
Each requirement is a behaviour plus the test that fails without it. `features/` scenarios are
named by what they assert; where an issue lists unit tests and scenarios, all of them are required.

### R199 — `,` closes Settings (#63)

- In Settings, `,` behaves exactly like `esc`, through the same code path: it closes when nothing is
  dirty and raises the discard prompt when something is.
- `,` is a **literal character**, and does not close, in each of these editing modes: string, path and
  list entry; env key; env value; group name; the `/` search; the group create prompt; the group
  rename prompt. It is ignored while the discard prompt or the group-delete confirm is up.
- The footer/hint text in Settings names `,` beside `esc`.
- **Tests:** one unit test per case above (closes clean, prompts dirty, literal in each of the seven
  editing modes, ignored under each of the two confirms), and a `features/` scenario that opens
  Settings with `,` and closes it with `,`.

### R200 — an expected old-pane `session_end` is not a fault (#64)

- A `session_end` declined because it came from the replaced launch generation (the
  `session_end.superseded` event) is **not** shown as "Hook declined" in `i`. It is hidden or shown
  as an informational line (for example `Previous launch's session_end ignored (expected after restart)`).
- "Hook declined" stays loud for the two suspicious cases, and only these: a non-`session_end` kind
  (`stop`, `notification`, …) from a replaced generation; and a hook carrying no generation while
  the row holds one and no restart/resume preceded it.
- `E` keeps showing the raw `.superseded` event unchanged.
- **Tests:** unit: restart then a late `session_end` from the old generation shows no "Hook declined"
  line; a late `stop` from the old generation still shows it; a generation-less hook with no
  preceding restart still shows it; `E` still lists the raw event. A `features/` scenario restarts a
  session and asserts `i` shows no alarm for the teardown `session_end`.

### R201 — setting: attach on click (#62, including its addendum)

- New `[ui] attach_on_click`, default **true** (today's behaviour). Lives beside `attach_on_new` and
  `attach_on_resume` in Settings, with a `DECK_ATTACH_ON_CLICK` env override of the same shape and the
  same settings plumbing (file, env, edit, scope parity).
- With it **false**, a **single click and a double-click** on a sidebar row only select: the list
  keeps keyboard focus, the preview stays passive, and no entry, ownership claim or fit happens.
  `↵` still enters.
- Unchanged by the setting: group-header click (collapse), click on the passive preview, click on the
  collapsed strip, re-targeting a sidebar click while already interactive, and `Ctrl+Q`.
- Any legacy double-click handler that bypasses the setting is found and routed through it.
- **Tests:** unit: setting on → click enters; off → single click selects only; off → double-click
  selects only, with no attachment recorded; `↵` afterwards still enters; the env override beats the
  file value; the settings parity tests cover the new key. `features/`: with it off, click a row then
  type `j` (selection moves, nothing reaches the pane); the same with a double-click.

### R202 — forward the mouse in the interactive preview (#67, including its clarification)

- **A click is not a selection.** A press released without motion marks nothing and copies nothing;
  the press alone draws no highlight.
- **Forwarding.** In interactive mode, when the pane program tracks the mouse (1000/1002/1003
  reporting), the grid is at live and Shift is not held, a click of the **left, middle or right**
  button is forwarded as a press and a release at the press cell, with that button's code, in the
  encoding the program asked for (SGR, release `m`; X10, release button code 3). It reuses the
  routing rule the wheel already uses (R183, #59).
- A program that does **not** track the mouse receives nothing from a click, as before.
- **New setting "Click and drag to select"**, `[ui] select_on_drag`, default **true**, with a
  `DECK_SELECT_ON_DRAG` env override and the same settings plumbing as R201:
  - **ON:** a left-button drag selects, highlights and copies on release, and the app receives
    nothing from the drag. Every other mouse event is forwarded.
  - **OFF:** deck intercepts nothing. All mouse events reach the app, including drags, which are
    forwarded as motion/drag reports; no highlight, nothing copied.
- Unchanged: the passive preview (a click enters interactive, #37), Shift held, and a scrolled-back
  grid keep their routing; drag-to-copy stays interactive-only.
- **Tasks, pre-split:** (a) click-is-not-a-selection plus the generalised mouse-report encoder; (b)
  left/middle/right forwarding; (c) the setting and the OFF-mode drag forwarding.
- **Tests:** unit: the encoder for each button in SGR and X10; a no-motion click copies nothing and
  draws no highlight; each of the three buttons forwards press+release at the press cell for a
  tracking app; a non-tracking app receives nothing; Shift and a scrolled-back grid do not forward; a
  drag selects and the app receives nothing; with the setting OFF a drag is forwarded as motion and
  nothing is selected; the default is ON on a fresh profile. `features/`: a click reaches a
  mouse-tracking fixture app with the right button code, and a plain shell gets nothing.

### R203 — rectangular selection (#66)

- Holding **Alt** while left-dragging selects a rectangle spanned by the two corner cells, highlighted
  as a rectangle. **Ctrl+drag** does the same (the fallback for window managers and terminals that
  swallow Alt). Ctrl+Alt and Shift are not bindings.
- On release the clipboard receives, for each row, the characters in the selected column range with
  trailing blanks trimmed, joined with `\n`. Rows shorter than the range give shorter, possibly empty,
  lines, never padded.
- A double-width glyph is included whole if its left cell is inside the rectangle; half a glyph is
  never emitted.
- The rectangle is clamped to the pane where the drag started.
- A plain drag keeps today's line-based selection. The rectangle is available wherever a plain drag
  selects today, and is governed by R202's setting: with "Click and drag to select" OFF there is no
  selection of either kind.
- The caveats (Alt grabbed by window managers, tmux/terminal modifier delivery) are documented in the
  mouse section of the spec.
- **Tests:** unit: corner order in all four directions; trailing-blank trim; a short row; a
  double-width glyph straddling each edge; clamping to the start pane; Alt and Ctrl each select a
  rectangle; plain drag is unchanged; the OFF setting disables it. `features/`: an Alt+drag copies
  the expected block from a fixture screen.

### R204 — hooks bound to an older deck binary (#56)

The hook command is the absolute path of the deck binary that launched the session; a running agent
keeps it. Cover exactly these four, and the three harnesses (Claude, Codex, Pi) for each:

1. **A correct message.** When `_hook` meets a state database newer than it supports, it prints a
   message naming the hook executable, its version and schema, and the database schema, and says to
   restart the session from deck (`R`). It never says "upgrade deck". The TUI's own open path keeps
   "upgrade deck".
2. **Auto-heal.** The store records, in a `meta` row (an additive write, not a schema change), the
   absolute path of the deck binary that last migrated or opened the DB for writing. An older
   `_hook` that meets a newer schema re-execs that binary with the same argv and the buffered stdin
   payload when the file exists, is executable, is not itself, and reports a schema at least as new.
   An env marker guards the loop: a re-exec never re-execs. When any condition fails, it prints the
   R204.1 message and does not re-exec.
3. **Stale-binding hint.** The hook executable a session was launched with is persisted (a launch fact
   already stored, or one column in its own schema bump with a migration test). When it differs from
   the running deck's, or the file is missing, `i` shows `hooks: bound to <path> — restart (R) to
   refresh` and the row's status reason says so until the session is restarted or resumed. It is a
   hint, not an error state.
4. **Check the other harnesses.** The Codex and Pi launch paths are read, and each gets the same
   tests as Claude.
- **Tasks, pre-split:** (a) the message; (b) the writer-path `meta` row and the re-exec with its loop
  guard; (c) persisting the launch executable and the `i`/status-reason hint; (d) Codex and Pi
  coverage.
- **Tests:** a `_hook` built with a lower `SchemaVersion` (ldflag or test hook, never a real old
  release) against a newer DB prints the R204.1 message, names the path, omits "upgrade deck"; with a
  recorded writer present it re-execs once and the status update lands with the payload intact; with
  the writer missing, not executable, or itself there is no re-exec; the loop guard holds; a stale
  binding shows the hint in `i` and the status reason and the hint clears after `R`; each harness has
  the same cases. `features/`: each of Claude, Codex and Pi launched under binary A, deck then run as
  binary B at a newer schema; the next hook heals through the re-exec, or the stale-binding hint
  shows when A cannot re-exec.
- Out of scope: rewriting a live agent's settings in place.

### R205 — the two #61 leftovers

- **`release.yml` pins its actions by SHA**, as `ci.yml` and `pages-pr-publish.yml` already do, with
  the tag as a trailing comment. The pins the previous run measured: `actions/checkout`
  `3d3c42e5aac5ba805825da76410c181273ba90b1` (v7) and `actions/setup-go`
  `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e` (v7); re-verify them against the upstream tags before
  committing. The existing workflow-pinning test is extended to cover `release.yml`, so a tag-pinned
  action there fails it.
- **`ci/releasegate` accepts the "suite" check only when it belongs to `ci.yml`.** A check named
  "suite" from any other workflow (for example a job in `pages-pr-publish.yml`) does not satisfy the
  gate. The gate's other behaviour is unchanged: push and PR runs only, the most recent gating run
  wins, pagination, and the malformed-response handling.
- **Tests:** a tag-pinned action in `release.yml` fails the pin test; a fixture where a non-`ci.yml`
  workflow publishes a green "suite" check makes the gate fail, and one from `ci.yml` makes it pass;
  the gate's existing tests pass unchanged. The coverage floor and CRAP ceiling hold for the package.

### R206 — the spec matches the code

`SPEC.md` is updated in the same commit as each behaviour above, in present tense: §11.5 and §6.5
(R199, R201, R202 settings and keys), §11.8 (R201 click table, R202 forwarding rule and setting, R203
rectangular selection and caveats), the detail-pane section for R200, and §6.1/§9 or wherever the
hook command and its failure modes are specified (R204). No history, no "shipped in" lines. The
sweep's reviewer finds no sentence in the spec that contradicts the live code for these behaviours.

## Definition of done

Behaviours R199–R205 shipped, each with its named tests green; the spec consistent with the code
(R206); no secret exposed; **all lanes green at the final pushed sha**, proven by the push run at
that sha plus **one** `workflow_dispatch` with an empty flaky record; the three sweep tasks done at
the same `origin/main` head with nothing committed after them. Nothing else.

## Out of scope

#65 (the event-hook rewrite of SPEC §10) and anything not listed above. Do not start it, and do not
leave `internal/notify` changes behind.

## Escape hatch

If the run proves that a requirement contradicts `SPEC.md` beyond what R206 lets it fix, that a
requirement cannot be met without a gate exception, or that a named test cannot be written, it must
not delete a test, add an exception or quietly narrow the requirement. File a petition with the
evidence and the smallest change that would resolve it, notify the operator, and carry on with
everything the petition does not block. The operator rules by amendment.

## Notifications

Use the `notify` hat tool in the same iteration as the work, a few hundred characters, never a
secret: anything that blocks work outright (including a petition), each wave finished, every
rejection and cure pass, the proof run's result, and one terminal summary. A missed or late send is
cured by mentioning it in the next one; it is never a success criterion and never a finding.

## Budget and shape

Iterations 300, approaches 2, `vigilant` on, a 120-minute iteration cap, a 24-hour wall-clock
window. Pre-split as stated in R202 and R204; every other requirement is one implementation task
(code, tests and spec together), then the three sweep tasks. The final sweep task is marked
`sweep: true`.

## Materiality rubric

Reject only on substance:

- a requirement's behaviour is absent or wrong in the live code or workflows;
- a test the requirement names is missing, red, or also passes on the unfixed tree;
- a gate is loosened, an exception is added, or a test was deleted, skipped or weakened to pass;
- any proof run in the Definition of done is red or has a non-empty flaky record;
- the spec contradicts the live code for a behaviour in this PRD;
- a secret is exposed, in the tree, a workflow log or a published report;
- a guard in the Ground rules is broken (the operator's deck directories or a `tmux -L deck*`
  server touched, a branch/PR/tag/release created, a run cancelled or re-run, an issue closed).
  Editing a formerly protected path is **not** a finding.

The working clauses: file the rule, not the example; read the direction of the evidence (a line that
could support either side supports neither); a review after a cure pass confirms the cure landed and
does not re-audit the product; judge a hat or router by its contract, not its taste; a stale sweep is
never process.

Everything else verifies, with the gap recorded as a residual note: wording, form, provenance,
process, and **any number or claim in prose outside the spec** (commit messages, issue comments,
notes, run artifacts). There is no advisory list. One exception: a run that never reached `suite`
because GitHub failed to provision a runner, or because of an Actions API 5xx, is not judged.
