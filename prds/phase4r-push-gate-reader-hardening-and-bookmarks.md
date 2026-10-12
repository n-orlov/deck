# Phase 4r — rename-push and name-reader hardening (#88, #89) and numbered session bookmarks (#87)

## What the operator gets

1. A deck rename never answers a pending Claude permission dialog. Claude's rename push is gated on the
   screen like the other adapters (#88).
2. A hostile or broken file can no longer hang `deck _hook` or the reconcile tick: harness name readers
   never block on a FIFO, the hook-supplied transcript path is confined, harness-reported names cannot hide
   invisible text, and the smaller LOW items listed in #88 and #89 are closed (#88, #89).
3. Numbered session bookmarks (#87): opt-in, off by default. `Alt+0`..`Alt+9` select the session bookmarked
   in that slot; a two-key chord sets a slot. Works in the list and while attached.

## Issues in scope

#87, #88 and #89, nothing else. Read each in full; this PRD wins where it differs. **Not in scope:** #73,
#86, #81–#85 (their closure is already commented), the Ctrl+digit / Ctrl+Shift+digit forms of #87 (Bubble Tea
v1.3.10 cannot report them; a v2 migration is a separate phase), and any change to how names are adopted.

Code to follow: `internal/service/push.go` (`pushRefusal`), `internal/agent/claude_push.go`,
`internal/agent/claude.go`, `copilot.go`, `pi.go` (argv builders and `*ForbiddenExtraFlag`),
`internal/agent/pi_control.go`, `internal/agent/claude_name.go` (`claudeOpen`, `claudeNamePath`),
`internal/agent/copilot_name.go` (`readCopilotWorkspace`), `internal/agent/name_sanitise.go`
(`isFormatOverride`), `internal/service/adopt.go` (`uniqueAdoptedName`, `nameTaken`), the dialog/prompt
detectors of the Codex, Pi and Copilot adapters (the pattern to copy), `internal/tui/interactive.go`
(`updateInteractive`, the Ctrl+Q interception), `internal/tui/tui.go` (main menu, footer, help),
`internal/config/schema.go`, `internal/store/` (schema ladder), `SPEC.md` §4, §8, §11.
Symbols are as of `c7689cffbde`; drift is expected and is not a finding.

## Requirements

### Part A — #88 Claude rename push gate

- **R267.** The Claude adapter detects a focused dialog and a ready prompt on the pane screen, as the other
  push adapters do. `pushRefusal` refuses a Claude push unless the composer is ready and no dialog is
  focused. Finite cases, each a table-test row that fails at the parent: (a) permission dialog up while the
  stored status is `running`; (b) same while `idle`; (c) composer with typed, unsent text; (d) empty ready
  composer — pushed. A refused push sends no keys at all and records the same refusal event the other
  adapters record.
- **R268.** Launch names are made consistent: Claude's launch name goes through the same control-stripping
  sanitiser as Pi and Copilot; Claude, Copilot and Pi pass the name as one attached argv element
  (`--name=<v>` / `-n=<v>` form, or the CLI's equivalent shown safe by a test with a name that begins with
  `-`), so a name starting with `-` is never a separate flag. `*ForbiddenExtraFlag` also refuses the attached
  short form (`-nFOO`). One table test per adapter covers `-p foo`, `--x`, `-nFOO`, an embedded newline.
- **R269.** A push to a Pi whose control file was left behind by a dead Pi does not report success. Deck
  treats a control file as live only when it can show the owning Pi is running (finite check chosen by the
  run and named in SPEC §8, for example a heartbeat/pid line the extension writes); a stale file is removed
  and the push is refused or sent by the fallback path. A test pins both the stale and the live case. The
  append uses `O_NOFOLLOW`.

### Part B — #89 name readers

- **R270.** `claudeOpen` and `readCopilotWorkspace` never block on a non-regular file: they Lstat (or open
  `O_NONBLOCK|O_NOFOLLOW`) and refuse FIFOs, sockets, devices and symlinks before reading. A test creates a
  FIFO with no writer at each path and shows `ReadName` returns within a second with no name; it fails at the
  parent by timing out.
- **R271.** `claudeNamePath` uses a hook-supplied `transcript_path` only when it resolves, after cleaning and
  symlink resolution, to a `.jsonl` regular file under the Claude projects root (`CLAUDE_CONFIG_DIR` or
  default); otherwise it uses the derived path. A table test: inside the root; outside the root; `..`
  escape; symlink pointing outside; a FIFO.
- **R272.** `isFormatOverride` additionally strips U+2060–2064, U+00AD, U+061C, U+180E and the tag block
  U+E0000–E007F. Adopted-name uniqueness compares the sanitised form, so two names that differ only by
  stripped characters get the `-2` suffix. Table test with one row per range.
- **R273.** The Codex name read caps the value it reads (`substr`/`LIMIT` in the query or a length check
  before the full copy) at a stated bound; a test with an oversize `threads.name` returns the truncated
  sanitised name without reading it whole. SPEC §10 states that `DECK_SESSION_NAME` and the event message
  carry a harness-reported (untrusted) name and that hook scripts must not interpolate it into a shell
  string; a test shows redaction runs over the adopted name in the event message.

### Part C — #87 numbered bookmarks

- **R274 (setting).** Config key `bookmarks` (bool, default false) with a main-menu toggle, in the config
  schema with an env override like its siblings. While off: no bookmark key is bound in the list or in
  interactive mode (the keys reach the agent untouched), no slot digit is drawn, no footer/help line
  mentions them. A test for each of the four surfaces.
- **R275 (keys).** While on: `Alt+0`..`Alt+9` jump to the session bookmarked in that slot (selection only;
  attaching stays a separate action). Setting a slot is the chord `Alt+M` then a digit `0`..`9`, which is
  layout-independent; on a US layout `Alt+Shift+digit` (`alt+!`, `alt+@`, …) also sets, as a convenience.
  `Esc` or any other key cancels a pending chord and is then handled normally. Bubble Tea v1 delivers these
  as `alt+<key>`; a table test per key shape drives the model.
- **R276 (attached).** While a session is attached (interactive mode) and the setting is on, the same keys
  are intercepted next to Ctrl+Q and not forwarded; jumping switches to the target session's interactive
  view without returning to the list. With the setting off the bytes are forwarded. A test pins forward vs
  intercept, and a test shows a bare digit and `Alt+<non-digit>` are never stolen.
- **R277 (store).** Schema ladder rung adds a `bookmarks` table (slot 0–9 → session id, one session per slot,
  a session may hold several slots). Assigning a taken slot moves it, not duplicates. Deleting a session
  removes its slots; a jump to an empty slot is a no-op with a one-line status message. Survives restart: a
  test reopens the store. The migration test covers upgrading a v13 database.
- **R278 (display and filter).** With the setting on, a bookmarked row shows its slot digit(s); the footer and
  `?` help list the keys. A jump to a row hidden by a filter, a collapsed group or a pin section clears the
  filter or expands the group so the row is selected and visible. One test per case (filtered out, collapsed
  group, pinned, visible).
- **R279 (spec).** SPEC.md §4 (data model), §8 (push gate, readers) and §11 (keys, menu, footer) state the
  above in present tense, in the commits that change the behaviour, and say that Ctrl+digit is not supported
  on Bubble Tea v1.

Pre-split into tasks at plan time: R267, R268, R269, R270+R271, R272, R273, R274, R275, R276, R277, R278,
then the three sweep tasks. R279 is satisfied inside each task, not as a docs-only task.

## Materiality rubric

Reject only on substance:

- a requirement's behaviour is absent or wrong in the live code;
- a test the requirement names is missing, red, or asserts the opposite of the requirement;
- the spec contradicts the live code;
- a secret is exposed.

Everything else verifies, with the gap recorded as a residual note: wording, form, provenance, process, and
**any number or claim in prose outside the spec** (commit messages, notes, issue comments, run artifacts).
Prose outside the spec is never a blocking ground.

Working clauses: file the rule, not the example; read the direction of the evidence; a review after a cure
pass confirms the cure landed and does not re-audit the product; judge a hat or router by its contract, not
its taste; a stale sweep is never process.

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
- **The sweep's shape** (the last three tasks, in this order, all at the same `origin/main` head, each
  marked `sweep: true`):
  1. the whole `go test -p=1 -count=1 ./...` tier under `ci/run.sh`, then `go build ./...`,
     `go vet ./...` and `gofmt -l` on the tracked `.go` files;
  2. `ci/run.sh ci/suite.sh ci-results && ci/run.sh ci/quality.sh ci-results`;
  3. confirm the push run at that head is `success`, then dispatch `ci.yml` **once** and read its flaky
     record.
  Sweep tasks commit nothing. **Nothing is committed after the sweep.** If `origin/main` moved past the
  swept sha, the sweep is stale and re-runs. A stale sweep is never process. Closing #87, #88 and #89 is a
  comment naming the commit and the test node, made after the sweep.
- **The CI dispatch recipe.** Use the Actions REST API with the operator's PAT from the git credentials:
  `POST /repos/n-orlov/deck/actions/workflows/ci.yml/dispatches` with `{"ref":"main"}`, then poll
  `GET /repos/n-orlov/deck/actions/runs?event=workflow_dispatch&head_sha=<sha>` to its conclusion, and
  read the run's log and artifacts. Dispatch **once**, at the final pushed sha. A run that never reached
  `suite` (runner provisioning failure, API 5xx) is not judged: dispatch another.
- **Escape hatch.** A requirement the run proves contradictory or impossible is petitioned (not silently
  dropped), with the evidence; review adjudicates the petition.
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
- **Existing agents behave identically** except where a requirement names the change; with `bookmarks` off
  every key reaches the agent exactly as before.
- **Profiles stay isolated.** No code path reads or writes another profile's config, state or socket.
- **A config reload never writes.**
- **The event hook never blocks the agent or loses an event**; redaction only ever removes text.

## Budget

500 iterations, 6 approaches, 24 h wall clock, 120-minute iteration cap.
