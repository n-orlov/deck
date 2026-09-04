# Phase 3k — agent availability

## Goal

GH issue #21. The create modal's Agent field is a compile-time list: `cmd/deck/main.go` and
`internal/tui/tui.go`'s `defaultAgentRegistry` each register `shell`, `claude` and `pi`, and the
field cycles `Registry.Kinds()` — so on a host where `pi` is not installed, `pi` is still offered,
the create succeeds, tmux starts a pane whose child cannot exec, and the row lands in `error ·
tmux pane exited with status 127` a reconcile tick later, with `command not found` in a crash
tail the user has to scroll for. Resume, by contrast, already preflights the binary
(`internal/service/resume.go`'s `lookPathIn`) and refuses cleanly before any pane exists.

This phase makes availability something deck **probes** rather than assumes: each adapter
declares the executable it launches, the modal lists only the kinds whose executable resolves on
the launch `PATH` at the moment the modal opens, `shell` is the floor and is always listed, and
create gains the same preflight resume already has. The operator's ruling, verbatim: *supported
agents need to be probed to appear on the list. bare minimum — plain shell, always available.*

Four product requirements and one documentation one. It is a small phase; its one real cost is
in `features/`, where a class of scenarios has only ever passed because a create was never
preflighted.

## Read this before anything else

- **`SPEC.md` is already correct and is the authority.** The operator amended it for this phase
  in the commit titled `spec: agent availability — the create modal offers only agents whose
  executable resolves, shell always (operator)`: §5 gained the declared-executable sentence and
  the `shell`-is-the-floor rule, §6.3 gained the "availability is probed, not assumed" bullet
  (which `PATH` is probed, when, the create/resume refusal, and the `login_shell` exemption),
  §11's capability list names the field as *available kinds only*, and §11.4's remembered-choice
  rule now falls back for an unavailable executable as well as an unregistered adapter and drops
  the *last used* label when it does. **Where this PRD and `SPEC.md` disagree, `SPEC.md` wins**
  and the disagreement is a finding for `docs/reports/phase3k-findings.md`, never an edit.
- **Protected paths — `SPEC.md`, `prds/`, `ci/Dockerfile`, `ci/SPIKE.md` — are read-only for
  this job**, no exception, and steering cannot license one. The audit for this run is exactly
  this command, and it must print nothing:

  ```sh
  BASE=$(git log --format=%H --diff-filter=A -1 -- prds/phase3k-agent-availability.md)
  git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
  ```

  The base is the commit that **added this file**, computed rather than pasted, so that commit
  and the SPEC amendment before it are outside the range by construction. Nothing about earlier
  history is this run's problem and no report should re-litigate it.
- **The CI container has neither `claude` nor `pi`.** `ci/Dockerfile` installs no agent, and the
  operator's `~/.local/bin` is not mounted. Today that is invisible, because a create is never
  preflighted: `features/permission_modes.feature`'s first two scenarios, for instance, create
  four `claude` and three `pi` sessions with **no** fake on `PATH` — the panes die with status
  127 and the scenarios pass anyway because they assert only the persisted profile. After R112
  those kinds are simply not in the field, `ensureCreateModalAgent`
  (`features/agent_steps_test.go`) cannot cycle to them, and the scenario times out. **That is the
  phase working, not a regression to paper over**: the fix is the fixture (put the fake on `PATH`
  before the client starts), never a knob that lists an uninstalled agent. R114 owns this.
- **There is no `pi` fixture step.** `a fake "claude" binary is on PATH for future deck clients`
  exists (`installFakeClaudeOnPATH`); `cmd/fake-pi` exists and `configureProbeScenario`
  (`features/status_probe_test.go`) builds it into the same `agentPATHDir` — but no Gherkin step
  does. R114 adds one, in the exact shape of the claude step, and reuses `installFakeClaudeOnPATH`'s
  linger-wrapper idiom (its doc comment explains the race it exists for).
- **Adding an agent kind must still need no edit under `internal/tui`.** That is the registry's
  reason to exist (`internal/agent/agent.go`'s package comment) and
  `internal/tui/registry_guard_test.go`'s `TestBlackBoxRegistrySwapNeedsNoTUIEdit` pins it. So the
  TUI must not learn any binary name: it receives availability as **data** — which registered
  kinds are available right now — from `internal/service`, the way it already receives the
  registry and the `createAgentSession` closure. The guard test's *setup* may inject an
  all-available prober (its adapter has no binary); its *assertion* is not weakened.
- **`View()` never probes.** SPEC §11.4: a dialog never reads anything slow from its render path.
  Availability is computed once when `n` opens the modal (a handful of `stat` calls, no tmux
  round-trip) and held in model state for the life of that dialog; the next `n` re-probes, which
  is how installing an agent while deck runs takes effect without a restart.
- **Read these before writing any code:** `internal/service/resume.go` (`lookPathIn` and the
  `login_shell` exemption around its one call — R113 is *that*, on the create path);
  `internal/service/agent.go` (`CreateAgent`, and `resolveLaunchEnv` for which `PATH` a pane
  gets); `internal/agent/agent.go` (`Caps`, `Adapter`, `Registry`); `internal/tui/tui.go`'s
  `pickCreateAgent`, `defaultCreateAgent`, `createAgentHelp`, and the `cycleOption` call on the
  Agent field; `internal/tui/create_last_used_agent_test.go` and `registry_guard_test.go` for
  the test idiom; `features/agent_steps_test.go`'s `installFakeClaudeOnPATH` and
  `ensureCreateModalAgent`.
- **The harness already has the vocabulary these scenarios need.** Do not invent a step that
  duplicates one of: `deck client "X" is started`, `deck client "X" creates <kind> session "S"
  with permission profile "P"`, `a fake "claude" binary is on PATH for future deck clients`, the
  create-modal keystroke steps in `features/create_session.feature` and `features/dialogs.feature`,
  `deck client "X" screen contains "…"`, and `the state database session "S" …`.

## Materiality

Review has one rubric for this phase, and it is this section.

**Blocking** — a requirement's stated behaviour is absent, contradicted, or asserted by a test
that passes against a product that does not have it (a fake green); a gate was not run at the
final code sha; a protected path was modified by this run; a secret or a real credential appears
anywhere in the tree; `shell` is ever absent from the Agent field; a registered-but-uninstalled
kind is listed; a create of an uninstalled agent (with `login_shell` off) produces a row or a
tmux session; a scenario is made to pass by listing an uninstalled agent, by a test-only env knob
in product code (R8), or by narrowing the sweep.

**Curable in place** — a wrong or missing citation, a stale sha, a stale count or "most recently
updated by" sentence in a report, a thin report section, a scenario title, a findings-file
omission, a doc-comment wording. Cure it with a docs-only commit; it does not invalidate the
gates and does not force a replan. **Phase 3j lost five of its seven approaches to this class**
being treated as blocking; it is not.

**Advisory, never blocking** — a carried-forward finding from an earlier phase recurring in the
gate (report it with its log path); a stability measurement below 10/10 that is published
honestly with every failure named; style, altitude and naming opinions; the operator
notification.

**Termination.** The **final code sha** is the last commit that touches `*.go` or `*.feature`.
Both gates are run at it and R115's documents cite it. A **docs-only tail commit** — the report,
the findings file, `docs/DELIVERY-LOG.md`, or a citation cure — does not invalidate either gate
and is itself exempt from re-verification. The record is written **once**, at that sha; it does
not embed self-counting scripts, does not publish citation totals, and is not re-audited by a
later task. There is no fixed point to chase: cite the code sha, then write the documents.

## Requirements

### R111 — an adapter declares its executable, and availability is one probe shared by list and launch

- Each adapter declares the executable its launch argv starts — the planner chooses the shape (a
  `Caps` field or an `Adapter` method), but it is declared once per adapter, next to `Kind()`,
  and it must equal what `Launch`/`Resume` actually put in `argv[0]` (`"claude"`, `"pi"`). A test
  pins that equality for every registered adapter so the two cannot drift. `shell` declares the
  empty executable: nothing to probe.
- **One probe function**, in `internal/service` (or `internal/agent`, if it needs no service
  state), answers "is kind K available under `PATH` P" as `lookPathIn(executable, P)`, with the
  empty executable **always available**. `lookPathIn` moves out of `resume.go` to sit beside it;
  resume's preflight and R113's create preflight and R112's listing all call this one function.
  Two copies that could disagree about a binary is a defect against this requirement.
- **Which `PATH`.** The one a new pane would get, per SPEC §6.3: deck's own `os.Getenv("PATH")`
  as `captured_path`, then config `[env]`'s `PATH` if it sets one — i.e. exactly
  `resolveLaunchEnv(os.Getenv("PATH"), nil)["PATH"]`. No session `env` exists at list time and it
  does not participate. A service method returns the set of available kinds for the current
  registry under that `PATH`.
- Unit evidence in `internal/service` (and `internal/agent` for the declaration): `shell` is
  available under an empty `PATH`; `pi` is unavailable when absent from P and available when an
  executable named `pi` is placed in a temp dir on P; a directory named `pi` does not count; a
  config `[env]` `PATH` participates and wins over the process `PATH`; every registered
  adapter's declared executable equals its `Launch` argv[0]. `lookPathIn` itself gets the direct
  tests it has never had.

### R112 — the Agent field lists available kinds only; `shell` always

- The create modal receives, on `n`, the available-kind set from R111 and the Agent field cycles
  **that**, not `Registry.Kinds()`. `shell` is always in it. The set is model state for the life
  of the open dialog and is never recomputed from `View()`; the next `n` probes again.
- **The remembered agent** (`pickCreateAgent`, GH #14): a `lastCreateAgent` that is registered
  but not currently available falls back to `defaultCreateAgent` over the *available* set, and
  the "(last used)" label is not shown for the fallback — SPEC §11.4's rule, extended from
  *registered* to *available*.
- **The field says what it hid.** The Agent row's help names the registered kinds that were not
  listed and why, in one short clause (e.g. `not on PATH: pi`), so §11's R7 discoverability
  holds: a user who expected `pi` learns in the dialog that deck could not find it, rather than
  concluding deck dropped support. With nothing hidden the help is unchanged. This is help copy,
  not a list entry — a hidden kind is never cyclable.
- Nothing else about the modal changes: field order, the profile field's re-derivation from the
  agent, the title, `esc`, validation.
- Unit evidence in `internal/tui`, through `Model.Update`/`createFieldRows` rather than by poking
  fields, with availability injected as data: the field cycles only available kinds; `shell` is
  present when the injected set is `{shell}`; a remembered `pi` falls back to the default without
  the "(last used)" label when `pi` is unavailable, and is honoured with the label when it is; the
  help names the hidden kinds; `View()` calls no prober (inject one that fails the test if
  called outside the open path). `TestBlackBoxRegistrySwapNeedsNoTUIEdit` still passes with its
  assertion intact.

### R113 — create preflights the executable, exactly as resume does

- `CreateAgent` runs R111's probe against the launch `PATH` it has already resolved for the pane,
  **before** `Store.CreateSession` — a missing executable means **no row and no tmux session**,
  not an error row. The message names the kind and the executable (`agent binary "pi" not found
  on PATH`), and the modal shows it in-dialog and retains everything typed (SPEC §11.4).
- **`login_shell` exempts it**, byte-for-byte the same rule as `resume.go`'s: the login shell's
  rc files decide `PATH`, deck cannot judge membership, so the check is skipped and the launch
  proceeds to whatever the pane's shell resolves. The listing (R112) still uses the non-login
  `PATH` as its best guess — that asymmetry is SPEC §6.3's and is not a defect.
- This is the race guard behind R112 (binary removed between `n` and `↵`, or a config `[env]`
  `PATH` that hides it) and the direct fix for the `127` crash row in GH #21.
- Unit evidence in `internal/service`: an uninstalled agent with `login_shell` off returns an
  error and leaves **no** row and **no** tmux session (asserted by absence on a real private
  socket, the way `resume_test.go` asserts refusals); the same input with `login_shell` on
  proceeds past the preflight; an installed agent is unaffected, and the pane command produced
  for it is byte-identical to today's tree.

### R114 — the harness stops relying on uninstalled agents

- **A `pi` fixture step**: `a fake "pi" binary is on PATH for future deck clients`, in
  `features/agent_steps_test.go`, in the exact shape of the claude step — builds `cmd/fake-pi`
  into `agentPATHDir` under the name `pi`, with the same linger wrapper and for the same reason.
  `configureProbeScenario` keeps working unchanged (or calls the new helper; either is fine).
- **Every scenario that drives the modal to a non-shell kind installs that kind's fake before the
  client it drives is started.** The inventory is this command; each hit is a scenario to read:

  ```sh
  grep -n 'creates \(claude\|pi\) session\|opens the create modal for agent' features/*.feature
  ```

  Known offenders as of the PRD cut: `permission_modes.feature` (its first two scenarios, and
  any other that creates `pi` — the `drift` scenario installs only `claude` and then creates
  `pi`), `crash.feature`'s failing-`pre_launch` scenario, and some scenarios in
  `durable_identity`, `lease_race`, `same_directory`, `status_claude_hooks` and `preview` that
  create without the step in *their own* scenario (a `Background` counts). `real_agent_smoke.feature`
  is `@real-agents` and uses real binaries — leave it. A scenario's assertions are not changed by
  this; only its `Given`.
- **New scenarios**, in `features/create_session.feature` or a new `features/agent_availability.feature`,
  every one asserting an observable:
  1. **Nothing installed → only `shell`.** A client started with no fake on `PATH` opens `n`; the
     Agent row reads `shell` and cycling never leaves it; the help names the hidden kinds.
  2. **Only `claude` installed → `claude, shell`.** The fake-claude step alone; `pi` is not
     cyclable and is named as hidden.
  3. **Installing between opens takes effect.** Open `n` with nothing installed and `esc`; put the
     fake on `PATH` *for a new client*, start it, open `n`: `claude` is listed. (A deck client's
     environment is fixed at process start — the step's doc comment says so — hence a second
     client, not a second open of the first.)
  4. **The create preflight refuses in-dialog.** With `claude` listed, remove the fake from
     `agentPATHDir` before `↵`; the modal stays open, shows `not found on PATH`, retains the typed
     name, and the state database has **no** row of that name.
  5. **A remembered `pi` falls back when `pi` is gone.** Create a `pi` session with the fake
     installed; a new client with no `pi` fake opens `n` on `shell` with no "(last used)" label.
- Unit tests under `internal/tui` that today construct the default registry and expect `pi` or
  `claude` in the Agent field are converted to inject an availability set — never to depend on
  what the CI container happens to have on `PATH`.

### R115 — the record matches the tree

At the final code sha, in `docs/`:

- `docs/reports/phase3k.md` — a requirement table in the phase 3i/3j style, one row per
  R111–R115, each naming the commits and the evidence path that discharge it; the final code sha;
  both gates' dispositions quoted from their logs. Which requirement discharges which numbered
  section of GH issue #21's design, so closing the issue is a reading rather than an argument.
- `docs/reports/phase3k-findings.md` — everything found on the way, the protected-path audit's
  output (empty), and any place where the tree and `SPEC.md` disagreed (as a finding, never an
  edit). The R114 inventory belongs here: which scenarios needed a fixture and which commit gave
  it to them.
- `docs/DELIVERY-LOG.md` gains this phase's paragraph.

Written once, at the final code sha, as one task. No citation-audit scripts, no sha counts, no
"kept current since" sentences — see Materiality's termination rule.

## Ordering

R111 → R112 → R113 → R114 → R115. The declaration and probe come first because both the list and
the preflight consume them; the list before the preflight because the preflight is the list's race
guard; the harness work after both because it is what the whole-suite sweep needs to be green.
Each unit test lands with the requirement it proves. **R114's fixture inventory may be planned
as its own early task** — reading the scenarios costs nothing and de-risks the sweep — but its
edits land after R112, since before R112 they change nothing observable.

## Green when

- `ci/run.sh go test -p=1 -count=1 ./...` exits **0** at the final code sha. Background it and
  poll. Per phase 3g finding F34 the non-verbose launcher cannot print the Gherkin tally — take
  the tally from a companion `-v` run on a docs-only descendant.
- `ci/stability.sh 10` is run **from a clean state at the final code sha** and published
  verbatim from `summary.log` with the script's own captured exit status. 10/10 is the target; a
  9/10 is published as 9/10 with every failure named and its per-run log path, and is advisory
  per the materiality rubric — never rounded up, and never re-run merely to improve the number.
- The R115 documents exist and cite only shas that resolve and paths that are tracked.
- The protected-path audit command in "Read this before anything else" prints nothing.

## Non-goals

- **The health view** (SPEC §6.3 last bullet, §9.5, §11.4's inventory). It is the right home for
  *per-session* resolvability under the tmux server's actual (possibly systemd-thin) environment,
  which a create-time probe cannot cover. Not this phase.
- **A per-agent command or path config key**, or any way to list an uninstalled agent.
- **Probing per keystroke, per tick, or in `View()`.** Once per `n`.
- **Changing what resume does** beyond moving `lookPathIn`; its behaviour and its `login_shell`
  exemption are already right and are the model.
- **Hiding or altering existing rows** whose agent is now missing. They list, preview, archive
  and delete as before; `r`/`R` already fail cleanly.
- **A Codex adapter**, Phase 4, or any adapter beyond the three registered.
- No theme palette / `internal/theme/builtin/*.toml` change, no contrast-floor allowlist, no
  test-only branch or env knob in product code (R8), no new top-level keybinding, no change to
  §11.3's fixed footer set — `help_keymap_parity_test.go`, `footer_bindings_parity_test.go` and
  `footer_handler_agreement_test.go` must all still pass **unedited**.
- The carried-forward findings that are out of scope and never claimed fixed: F2 (golden-frame
  settle), F20 (`status_recovery` dup-pane), F22 (`ByteArrivalPattern`), F37 (the `sort_order`
  latent race), F7 quantisation collisions, the `filter.feature` dd/undo race, and the OSC 52
  clipboard-reliability question. A recurrence in the gate is reported with its log path and is
  advisory.
- Do not touch `features/godog_test.go`'s `defaultTags`, and never narrow a deliverable sweep
  with `-run` or `DECK_GODOG_PATHS`.

## For the planner

This run has **two approaches**, not six. Phase 3j's approach 1 delivered every product
requirement; approaches 2–7 were spent on record hygiene that this PRD's Materiality section now
makes curable-in-place. Plan so that approach 1 finishes:

- **One unit of work per task.** A requirement is two or three tasks — the product change, its
  unit evidence, its scenario. One feature file is one task. R114's fixture sweep is one task per
  feature file that needs it, not one task for all of them. The whole-suite sweep is its own
  task; the stability measurement is its own task and owns its whole iteration. R115 is **one**
  task.
- **No task whose title contains "every".** A task that cannot flip its status inside roughly one
  iteration is two tasks.
- **R111 is the spine**; land it complete, including moving `lookPathIn`, before R112.
- **Expect the first full sweep to fail in `features/`** on scenarios that create an uninstalled
  agent — that is R114's inventory confirming itself, and the cure is the fixture step, never a
  listing knob. Read the failing scenario's `Given` before anything else.
- Notify the operator when the run finishes (the `telegram-notify` skill is placed). A late send
  is fine and a missed send is cured by sending it when noticed; advisory, never blocking.

Known tooling gotchas, inherited verbatim from 3g–3j's notes:

- **No Go and no tmux in the container** — everything runs through `ci/run.sh`, which starts a
  sibling container. The docker socket is mounted for this run precisely so that works.
- **Never clean up docker by label, filter or wildcard.** `docker prune`, `docker rm`/`kill`/
  `stop` filtered on `label=ralphd.run=`, and any wildcard `rm`/`rmi` include **this job's own
  container**: a sweep like that SIGKILLs the run mid-iteration and loses the iteration's work.
  Remove a container or image only by exact name and exact tag, and only one you created. Same
  rule for processes: never `pkill -f` or `killall` — resolve a pid, verify what it is, signal
  that pid. Other checkouts on this host are other runs' live workspaces; do not edit, stash or
  clean any tree but `/workspace`.
- `edit`/`write` corrupt `\r` in `.feature` files — `od -c` first.
- Each bash call is a fresh shell; capture `$?` in the same call.
- At most one whole-suite run per iteration.
- Commit subjects `<area>: <why> (task NNN)` with this phase's own task ids; push every completed
  task's commit; never force-push or rewrite history.
- A deck-managed tmux session is named `deck_<slug>`; the `internal/tui` real-tmux unit harness is
  `selectionTestSocket(name)` + `newQuietSelectionPane(t, socket, session, w, h)`.
