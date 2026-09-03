# Phase 3j — launch and teardown hooks

## Goal

GH issue #20. `pre_launch` (SPEC §6.4) is deck's sanctioned home for a secret that must not
live in `state.db`, but a `pre_launch` script cannot tell **which session it is running for**,
there is nowhere to declare one **once** for every session, there is no symmetric place to
**release** whatever it acquired, and none of it can be **changed on a row that already
exists**. Each of those four gaps on its own is an inconvenience; together they rule out the
whole class of self-selecting launch hooks — one script, installed once in the user's own home,
that inspects the session it is about to launch and decides for itself what to do.

This phase closes all four. It gives every pane the launching session's own facts as
`DECK_SESSION_*`, adds a global `pre_launch` and a global/per-session `post_destroy`, makes the
four editable launch inputs editable, and states the two contracts the whole design rests on —
that a hook's exports reach that session's agent and nothing else, and that a hook runs on
*every* launch and must therefore be idempotent.

Six product requirements and one documentation one. Nothing here is a new subsystem: the pane
is already composed so that `pre_launch` sees the whole env overlay and the agent inherits
whatever it exports — the work is to promise that, extend it, and make it reachable.

## Read this before anything else

- **`SPEC.md` is already correct and is the authority.** The operator amended it for this phase
  in commit `24b57fd` (`spec: launch and teardown hooks — session context, global hooks,
  editable launch inputs (operator)`): §4's DDL gained `post_destroy` and `launch_dirty`, §6.1
  gained the deck-owned session-context layer and its nine variables, §6.2 was rewritten around
  two dirty flags and named the inputs that are *not* editable and why, §6.4 gained the
  export-reaches-the-agent contract, the scrollback leak path and the idempotency rule, §6.5
  gained the two global hook keys and the global-first composition rule, §9.1 gained the
  fail-closed statement, §9.2 gained the whole `post_destroy` paragraph, and §11's capability
  list, §11's keymap and §11.4's dialog inventory gained the launch-inputs editor.
  **Where this PRD and `SPEC.md` disagree, `SPEC.md` wins** and the disagreement is a finding
  for `docs/reports/phase3j-findings.md`, never an edit.
- **Protected paths — `SPEC.md`, `prds/`, `ci/Dockerfile`, `ci/SPIKE.md` — are read-only for
  this job**, no exception, and steering cannot license one. The audit for this run is exactly
  this command, and it must print nothing:

  ```sh
  BASE=$(git log --format=%H --diff-filter=A -1 -- prds/phase3j-launch-and-teardown-hooks.md)
  git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
  ```

  The base is the commit that **added this file**, computed rather than pasted, so that commit
  and the SPEC amendment before it are outside the range by construction and the check is
  satisfiable by a forward-only run from its first iteration onward. Nothing about earlier
  history is this run's problem and no report should re-litigate it.
- **No new top-level key.** The launch-inputs editor is reached from the `i` session detail
  dialog exactly as rename is — the operator ruled on this — so §11's keymap and §11.3's footer
  fixed set are untouched and `help_keymap_parity_test.go`,
  `footer_bindings_parity_test.go` and `footer_handler_agreement_test.go` must all still pass
  **unedited**. `footer_bindings_parity_test.go` parses `SPEC.md`'s own prose at test time; if it
  fails, the code is wrong, not the spec.
- **Read these five files before writing any code.** Most of this phase is an extension of
  something already there, and the doc comments already explain the constraint:
  - `internal/service/agent.go` — `buildPaneCommand` (the `pre_launch && exec "$@"` composition
    and its fail-closed doc comment), `applyInstrumentation` (which already merges adapter env
    **last**, the exact position R104's context must occupy), and `resolveLaunchEnv` (§6.3's
    order).
  - `internal/service/resume.go` — the resume path builds the pane through the *same*
    `resolveLaunchEnv` → `applyInstrumentation` → `buildPaneCommand` sequence. That is why
    R104 and R105 are one change and not two, and it is what makes §6.4's idempotency rule
    already true today.
  - `internal/agent/claude.go`, `pi.go`, `shell.go` — `Claude.Instrument` is the only source of
    `DECK_SESSION_ID`/`DECK_HOME` today; `Pi.Instrument` and `Shell.Instrument` both return
    `nil, nil`, so a `pi` or `shell` pane carries nothing deck-specific at all. R104's move is
    what makes those two adapters equal.
  - `internal/config/schema.go` — one entry per flat `config.toml` key carrying kind, scope,
    default and description, and the single source both the parser and §11.5's settings view are
    generated from. R105 and R107's config keys are two entries here, not two hand-written
    parsers.
  - `internal/service/inject.go` — `InjectEnv`, the shell-only "inject instead" that clears
    `env_dirty` by typing `export K=V` into a live pane. It is the reason §6.2 has **two** dirty
    flags: an injected env legitimately clears `env_dirty` and must never clear
    `launch_dirty`, because typing into a live shell cannot re-run a `pre_launch` that already
    ran. Conflating the two flags is a defect against R108 even if every test passes.
- **The harness already has the vocabulary these scenarios need.** Do not invent a step that
  duplicates one of: `a session "S" is created with agent "A"`, the create-modal steps in
  `features/create_session.feature`, `deck client "X" is started`, the env-editor steps and the
  `env↻` assertions in `features/environment.feature`, the kill/delete/undo steps in
  `features/kill_delete_undo.feature`, `the state database session "S" is "T" from "SRC" …`,
  and the pane-capture assertions used throughout. `features/environment.feature` and
  `features/create_session.feature` are the two files to read for the idiom.

## Materiality

Review has one rubric for this phase, and it is this section.

**Blocking** — a requirement's stated behaviour is absent, contradicted, or asserted by a test
that passes against a product that does not have it (a fake green); a gate was not run at the
final code sha; a protected path was modified by this run; a secret or a real credential appears
anywhere in the tree; `agent`, `cwd`, `slug` or `captured_path` acquired a mutator; `post_destroy`
runs on `x` or on reaping; a hook's failure blocks a teardown, or a hook's failure fails to block
a launch.

**Curable in place** — a wrong or missing citation, a stale sha, a report section that is thin,
a scenario title, a findings-file omission, a doc-comment wording. Cure it with a docs-only
commit; it does not invalidate the gates and does not force a replan.

**Advisory, never blocking** — a carried-forward finding from an earlier phase recurring in the
gate (report it with its log path); a stability measurement below 10/10 that is published
honestly with every failure named; style, altitude and naming opinions; the operator
notification.

**Termination.** The **final code sha** is the last commit that touches `*.go` or `*.feature`.
Both gates are run at it and R110's documents cite it. A **docs-only tail commit** — the report,
the findings file, `docs/DELIVERY-LOG.md`, or a citation cure — does not invalidate either gate
and is itself exempt from re-verification. There is no fixed point to chase here: cite the code
sha, then write the documents.

## Requirements

### R104 — every pane carries its own session's context, for every adapter, on create and on resume

- Build §6.1's `DECK_SESSION_*` map **in `internal/service`**, from the row plus the launch kind,
  and merge it **after** `applyInstrumentation` so it is last and therefore unoverridable — a
  session `env` key or a config `[env]` key named `DECK_SESSION_NAME` must not be able to lie to
  a hook. The nine variables, their values and their emptiness rules are SPEC §6.1's table; that
  table is the specification and this requirement does not restate it.
- **Move `DECK_SESSION_ID` and `DECK_HOME` out of `Claude.Instrument`** into that map. Their
  values must not change — `deck _hook` (§8.1) resolves a session by `DECK_SESSION_ID` and writes
  under `DECK_HOME`, so a claude pane must end up with byte-identical values from the new source.
  `Claude.Instrument` keeps what it owns: the `--settings` hook JSON and
  `DECK_LAUNCH_GENERATION`, whose **absent-when-no-lease** semantics are deliberate (see its doc
  comment) and are not changed to empty-when-unset by this requirement.
- **Every adapter.** After this, a `pi` and a `shell` pane carry the same nine variables a
  `claude` pane does. This is the point of the requirement: an adapter that returns `nil` from
  `Instrument` must not thereby be a session without an identity.
- **Both launch paths.** `CreateAgent`, `CreateShell` → `DECK_SESSION_LAUNCH_KIND=create`;
  `Resume` (and therefore `Restart`, which routes through it) → `resume`. One construction
  function used by all of them; two copies that drift is a defect against this requirement even
  if both pass.
- Unit evidence in `internal/service`: for each registered adapter kind, every variable present
  with the right value on the create path and on the resume path; `create` vs `resume` differs
  and nothing else does; a session `env` and a config `[env]` entry for the same name both lose;
  an unset `workspace`/`conversation_id` exports as empty **rather than absent**;
  `DECK_LAUNCH_GENERATION` is still absent when no lease was taken.

### R105 — a global `pre_launch`, composed global-first with the session's own

- One new flat key in `internal/config/schema.go`: `pre_launch`, string, default empty, scope
  *restart-to-apply*, with a description that says what it is and that it runs for **every**
  session. Because §11.5's view is generated from the schema, it appears in settings by
  construction — assert that it does rather than hand-building a field for it.
- Wire it to `service.Service` the way `ConfigEnv` is wired, and compose in `buildPaneCommand`:
  **global first, then the session's own if it is set**, joined with `&&` so a failing global
  hook short-circuits both the session hook and the agent. Global-first is what lets the session
  hook read what the global one exported, in the same shell; a per-session hook never shadows
  the global one.
- **Fail-closed, unchanged and now stated (§9.1).** A non-zero exit from either hook means the
  agent is never exec'd, the pane is retained with the hook's own output (`remain-on-exit
  failed`), and the row lands in `error` with that reason.
- **An empty global hook changes nothing.** With `pre_launch` unset in `config.toml`, the pane
  command must be **byte-identical** to what today's tree produces for the same session,
  including the no-`pre_launch`-and-no-`login_shell` fast path that returns the bare argv. Pin
  this; it is the regression that would otherwise reach every existing session on the host.
- Unit evidence in `internal/service`: the four combinations of global/session hook set and
  unset produce the exact expected argv; the composed script's order is global-then-session; a
  global hook alone reaches the agent; and the fail-closed short-circuit holds for a failing
  global hook with a passing session hook.

### R106 — the env-mutation contract, promised rather than incidental

`pre_launch` runs in the same shell that then execs the agent, so anything it exports is
inherited by that agent and by nothing else. That already falls out of `buildPaneCommand`'s
`exec "$@"`, which is exactly the problem: the feature the operator is building rests on it and
it is currently an accident of the implementation. §6.4 now states it. Make it true on purpose:

- A doc comment on `buildPaneCommand` naming the guarantee and its three boundaries, and tests
  that hold each boundary:
  - the exported value **is** in the agent process's environment;
  - it is **not** in the tmux *session* environment table (`show-environment`) — deck's `-e`
    mirror is for future panes and a hook's export is not part of it;
  - it is **not** in any column of `state.db` for that session, which is why §6.4 prefers a hook
    over `env` for a secret in the first place.
- The positive case wants a real tmux server on a private socket, the way `internal/service`'s
  existing launch tests do; a `shell` session whose `pre_launch` exports a value and whose pane
  is then asked for it is the cheapest honest shape.
- **Scrollback is the one leak path** and §6.4 names the safe shape. This requirement owns the
  test; R109 owns telling the user.

### R107 — `post_destroy`, per-session and global, on `A` and `dd` and nothing else

- **Store.** `schemaV6` adds `post_destroy TEXT` (and R108's `launch_dirty`) by
  `ALTER TABLE ... ADD COLUMN`, `SchemaVersion` goes to 6, and the migration switch gains its
  `case 5:` arm in the established shape. The column is written by create, read by the session
  SELECT, and updated by R108's mutator. **An existing `state.db` must open and migrate cleanly**
  — the operator's live database is large and has real sessions in it.
- **Config.** A second flat schema key, `post_destroy`, same kind and scope as R105's.
- **Service.** `Archive` and `Delete` run the hooks after the action they belong to has durably
  succeeded. Everything about how they run is SPEC §9.2's paragraph and is not restated here,
  but four points are load-bearing enough to name: it is a **deck subprocess, not a pane**, with
  R104's context in its environment minus `DECK_SESSION_LAUNCH_KIND` and plus
  `DECK_TEARDOWN_KIND` (`archive`/`delete`); it is **fail-open** — a non-zero exit or a timeout
  raises a toast and records an event and never reverses the action; it is bounded by a **30 s
  timeout per hook**, fixed in code as a named constant, not a config key; and the session's own
  hook runs **before** the global one, each in its own subprocess.
- **The two negatives are the requirement, as much as the positive.** `x` (`Kill`) does not run
  it, and neither does reaping — not `Reap`, not the store-open sweep, not the create/rename
  path that reaps a tombstone to reuse its name. Both negatives get their own test, asserted on
  an observable side effect of the hook (a file it writes), not on a mock's call count.
- A failure records an event (`kind = 'note'`, since §4's event kinds are a closed list and this
  phase does not add one) and raises a toast; the row is not resurrected.
- A bulk `dd` over a mark set runs the hooks once per marked session.
- Unit evidence in `internal/service` and `internal/tui`: `A` and `dd` each run session-then-global
  once; `x` and every reap path run neither; a non-zero hook exit leaves the archive/delete
  durably done and surfaces the event; a hook that outlives the timeout is killed and reported;
  a bulk `dd` over two marks fires twice.

### R108 — the four editable launch inputs, editable, restart-to-apply

- **Store.** `schemaV6` adds `launch_dirty INTEGER NOT NULL DEFAULT 0`. One mutator updates
  `pre_launch`, `post_destroy`, `launch_args` and `login_shell` and sets `launch_dirty = 1` in
  the same statement. `R`'s restart clears **both** flags; `InjectEnv` clears `env_dirty`
  **only** — see the note in "Read this before anything else", and §6.2's step 3.
- **The exclusions are enforced, not merely documented.** `agent`, `cwd`, `slug` and
  `captured_path` must have no mutator. Pin it with a source-scanning test in `internal/store`
  in the same spirit as `footer_bindings_parity_test.go`: no `UPDATE sessions SET` statement in
  the package names any of those four columns. A test that asserts this by calling a method that
  does not exist proves nothing.
- **TUI.** A launch-inputs editor reached from the `i` session detail dialog, exactly as rename
  is: built through `applyDialogContract` (`internal/tui/dialog_contract.go`), in `Update`'s
  overlay interceptor chain and in the mouse-suppression condition every other overlay is
  already in, obeying §11.4 — `esc` cancels and changes nothing, `↵` submits, `↑`/`↓` move
  between fields, `tab` is **not** a field-navigation key, validation is in-dialog and retains
  what the user typed, and the dialog is themed in §11.6's tokens. Four fields, each labelled
  *restart-to-apply*; the two hook lines are shown verbatim, not masked (§11.4 — they are
  commands, not values).
- **The badge.** A row with a pending launch-input edit shows `launch↻`, beside `env↻` when both
  are pending. It reads *changed, not yet applied* and `R` is the only thing that applies it.
  Both badges must be able to be on one row at once without either being lost to width.
- **The create modal gains a `post_destroy` field**, next to `pre_launch`, per §11's capability
  list. It is the same one-line string field, with the same field help.
- Unit evidence in `internal/store` and `internal/tui`: each of the four columns round-trips an
  UPDATE and sets `launch_dirty`; a restart clears both flags; an env injection clears only
  `env_dirty`; the four excluded columns have no mutator; the editor writes what was typed and
  `esc` writes nothing; the badge appears, and appears alongside `env↻`.

### R109 — a user can find out what a hook must be

Everything in this requirement is user-visible copy plus its test, and it is the cheapest
requirement in the phase. It exists because §6.4's two rules are useless if they live only in
`SPEC.md`.

- The `?` help overlay, the create modal's field help for `pre_launch`/`post_destroy`, the
  launch-inputs editor and settings' descriptions for the two global keys must between them say:
  that a launch hook runs on **every** launch (create, `r`, `R`, an `r` after `U`, and the first
  `r` after a host restart) and must therefore be idempotent; that it is fail-closed and the
  session will refuse to start if it fails; that a teardown hook is fail-open, runs on `A` and
  `dd` and **not** on `x`; that a `post_destroy` and an undo mean the row comes back `stopped`
  and the next `r` rebuilds what was released; and the safe shape for a secret — emit
  `export K=V` on stdout for the caller to `eval "$(…)"`, diagnostics to stderr, never echo the
  value, and set `sensitive` (§8) if the hook cannot be that careful.
- Tests in the shape `create_field_help_test.go` already uses. Copy is asserted by substance,
  not verbatim: assert that the idempotency claim, the fail-closed claim, the not-on-`x` claim
  and the never-echo claim are each present somewhere a user can reach.

### R110 — the record matches the tree

At the final code sha, in `docs/`:

- `docs/reports/phase3j.md` — a requirement table in the phase 3h/3i style, one row per
  R104–R110, each naming the commits and the evidence path that discharge it.
- `docs/reports/phase3j-findings.md` — everything found on the way, the disposition of both
  gates, and any place where the tree and `SPEC.md` disagreed (as a finding, never an edit).
- `docs/DELIVERY-LOG.md` gains this phase's paragraph.
- GH issue #20 is the source of this phase; the report says which requirement discharges which
  of the issue's numbered sections R1–R4, so closing it is a reading rather than an argument.

## Feature scenarios

Two new files. Every scenario asserts an observable — a pane's environment, a durable row, a
file a hook wrote — never a mock.

`features/launch_hooks.feature`:

1. **A global hook self-selects on the session's name.** With a global `pre_launch` that exports
   a variable only when `$DECK_SESSION_NAME` matches a pattern, a session whose name matches
   carries it and a session whose name does not, does not. This is the phase's headline: one
   line of config, two sessions, opposite outcomes, and deck knows nothing about the pattern.
2. **Global and session hooks compose, global first.** Both set; the session hook's behaviour
   depends on a value the global one exported, proving the order and the shared shell.
3. **A failing global hook refuses the launch.** The row is `error`, the agent never started, and
   the hook's own output is visible in the retained pane.
4. **A `pi` or `shell` session carries the context** — the adapter that had none before.
5. **Session `env` cannot override the context.** A session `env` entry for `DECK_SESSION_NAME`
   loses to the real name.
6. **`create` and `resume` differ.** One session, created and then resumed, whose hook records
   `$DECK_SESSION_LAUNCH_KIND` both times.
7. **A launch input edited on a live row shows `launch↻` and applies only on `R`.**

`features/teardown_hooks.feature`:

8. **`dd` runs the teardown hook once; `x` runs it not at all.** Proven by the artefact the hook
   writes, with the same session killed and then deleted.
9. **`A` runs it, and the undo does not re-run anything** — the row comes back `stopped` and the
   toast says the next `r` rebuilds.
10. **A failing teardown hook does not block the deletion**, and the failure is on the record.
11. **A bulk `dd` over two marked rows runs it twice**, once per session, with each session's own
    context.

## Ordering

R104 → R105 → R106 → R107 → R108 → R109, then R110 against the final code sha. The context
comes before the global hook that reads it, the global hook before the contract that describes
what it may export, and `post_destroy` before the editor that has to offer a field for it. R109 can land any time after R107 and is a good task to slot between two heavy ones. Each feature
scenario lands with the requirement it proves, never in a batch at the end.

R107's schema migration and R108's are **one** migration (`schemaV6`, two columns) landed once,
by whichever requirement gets there first, and the other then uses it. Two migrations to version
6 is a conflict, not a merge.

## Green when

- `ci/run.sh go test -p=1 -count=1 ./...` exits **0** at the final code sha. Background it and
  poll. Per phase 3g finding F34 the non-verbose launcher cannot print the Gherkin tally — take
  the tally from a companion `-v` run on a docs-only descendant.
- `ci/stability.sh 10` is run **from a clean state at the final code sha** and published
  verbatim from `summary.log` with the script's own captured exit status. 10/10 is the target; a
  9/10 is published as 9/10 with every failure named and its per-run log path, and is advisory
  per the materiality rubric — never rounded up, and never re-run merely to improve the number.
- The R110 documents exist and cite only shas that resolve and paths that are tracked.
- The protected-path audit command in "Read this before anything else" prints nothing.

## Non-goals

- **Anything about the motivating gateway** — its auth, its key lifetimes, its revocation
  semantics, or the ticket convention that selects it. That lives in a script in the operator's
  own home and will evolve there. deck must not learn about any of it; the only thing this phase
  owes that script is context, a place to be declared, and a place to release.
- **Updating a live pane's environment in place.** Restart-to-apply is the model (§6.2). The
  stale `DECK_SESSION_NAME` in a pane that was renamed after launch is a consequence of that
  model, documented by §6.1's launch-time-snapshot rule, and is not a defect to fix here.
- **Editing `agent` or `cwd`.** §6.2 names the reasons; R108 enforces the absence.
- **Per-agent-kind hook declarations** (`[hooks.claude]` and friends). One global hook for every
  session; `$DECK_SESSION_AGENT` is exported precisely so a script can branch on adapter kind
  itself, and §11.5's takeover wants flat keys.
- **A configurable hook timeout.** A named constant, not a key.
- **A new event kind.** §4's list is closed for this phase; a hook failure is a `note`.
- **A new top-level keybinding**, a new footer entry, or any change to §11.3's fixed set.
- No theme palette / `internal/theme/builtin/*.toml` change, no contrast-floor allowlist, no
  test-only branch or env knob in product code (R8).
- No Phase 4 work, no Codex adapter, and no re-litigation of anything R76–R103 already landed.
- The carried-forward findings that are out of scope and never claimed fixed: F2 (golden-frame
  settle), F20 (`status_recovery` dup-pane), F22 (`ByteArrivalPattern`), F37 (the `sort_order`
  latent race), F7 quantisation collisions, the `filter.feature` dd/undo race, and the OSC 52
  clipboard-reliability question. A recurrence in the gate is reported with its log path and is
  advisory.
- Do not touch `features/godog_test.go`'s `defaultTags`, and never narrow a deliverable sweep
  with `-run` or `DECK_GODOG_PATHS`.

## For the planner

Phase 3g lost four approaches to the harness's own stall rule and phase 3h lost approaches the
same way; phase 3i's second run finished its work but never got a verdict. Plan accordingly:

- **One unit of work per task.** A requirement is usually two or three tasks — the product
  change, its unit evidence, its scenario. One feature file is one task. The whole-suite sweep is
  its own task; the stability measurement is its own task and owns its whole iteration.
- **No task whose title contains "every".** A task that cannot flip its status inside roughly one
  iteration is two tasks.
- **R104 is the spine.** R105, R107 and R108 all consume the context map it builds; land it first
  and land it complete, including the move out of `Claude.Instrument`, rather than adding
  variables incrementally across three approaches.
- R107 and R108 share `schemaV6` — see "Ordering". R109 is deliberately small and independent.
- Notify the operator when the run finishes (the `telegram-notify` skill is placed). A late send
  is fine and a missed send is cured by sending it when noticed; this is advisory and never a
  blocking finding.

Known tooling gotchas, inherited verbatim from 3g/3h/3i's notes:

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
