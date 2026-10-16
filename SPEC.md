# deck — product spec

A terminal session manager for CLI coding agents. Named sessions with durable
conversations, an attention-sorted list, and N concurrent clients — on one host, with
tmux as the only runtime dependency.

Interaction model: **`deck` is a TUI. Every user action happens in the UI.** There is no
user-facing command line: the only arguments choose *which* deck opens — `deck [<profile>]`
(§3.4) — plus `--profiles` and `--version`, and none of them performs an action. The two
exceptions are detached verbs: `deck new --agent <kind>` (§3.4), a shortcut for the create
dialog, and `deck update` (§3.4, §13.1), which installs a newer release over the running binary.

**deck is the primary way to control a session; direct `tmux` is the fallback for the awkward
cases.** Where deck's own view and a bare `tmux attach` elsewhere want different things from
the same session, deck's view wins — it is where the user is, and a manager that degrades its
own display to stay polite to a hypothetical second client optimises for the rarer case.
`tmux -L deck ls` and `tmux -L deck attach` stay supported and surfaced in help (§3): they
are how a user recovers when the TUI is broken, not the intended daily path. Two consequences
run through this document — deck may change a session's geometry to suit its own panel (§11,
§11.9), and a second client attached at a different size may therefore see a cropped view
(§3.3). Both are costs this design accepts and states rather than designs around.

> Name note: `deck` collides on `$PATH` with Kong's `decK` CLI if that's ever installed.
> Binary name is one constant; flagged, not blocking.

**How this document works.** It describes the product as it is meant to be, in the present
tense: one design, with the reasons that constrain it. It does **not** narrate its own
revisions — no "changed from", no "previously", no "the reviewer found", no phase numbers.
Three other places carry that record and are the only ones that should: `git log` on this
file for how the design changed and why, `docs/PLAN.md` for what gets built when, and
`docs/DELIVERY-LOG.md` for what actually happened. **A sentence that only makes sense if
you already know what the spec used to say, or which phase is in flight, is a defect here**
— it rots on its own schedule and it makes a reader trust the least reliable copy of the
history. Reasons are not history: "the preview floor is 40 columns because narrower wraps
into hash-soup" is a constraint and belongs; "the preview floor was raised to 40" does not.

---

## 1. Why

Existing managers are either too heavy (web dashboards, ACP workers, plugin engines,
Docker sandboxing, tunnels) or built around a workflow that doesn't apply — one git
worktree per task. What's actually needed is three things:

1. **Named, cwd-anchored sessions whose conversations survive a host reboot** — the
   conversation, not just a shell in the right directory.
2. **A status column** — which session is working, which is waiting on me, which died.
3. **N concurrent clients** — a TUI on the desktop, more on SSH ttys from another machine,
   all showing the same truth.

### Hard requirements (the acceptance bar)

| # | Requirement | Consequence |
|---|---|---|
| **R1** | **No worktrees, no git.** A session is `(name, cwd, agent, args, env)`. Its cwd is often a working directory from which the agent clones and edits many repositories itself. | No git columns, no branch/PR/CI awareness, no worktree lifecycle. Nothing on disk is *owned* by a session, which makes teardown cheap and safe. |
| **R2** | **Several sessions may share one cwd.** Two agents in the same directory is normal, not an edge case. | `--continue` / `resume --last` / any "most recent in this directory" resolution is **banned**: it would collapse N sessions onto one conversation. Resume is always by explicit conversation id. |
| **R3** | **Durable identity.** A named session and its conversation outlive a reboot, an agent upgrade, and `tmux kill-server`. Nothing is auto-restarted; **resume is one keypress, on demand.** | Durable store of `name → cwd → agent → conversation id`. `stopped` is a normal, first-class state, not an error. No boot-time restore service, no autostart. |
| **R4** | **N concurrent TUIs, one host.** Desktop plus several SSH ttys, hopping between machines mid-task. | State lives in tmux + SQLite (WAL). No process is authoritative, no process is required. Whole-list rewrites from in-memory state are a forbidden pattern; every mutation is a targeted `UPDATE`. Launches take a row lease so two TUIs can't double-start one session. |
| **R5** | **Lightweight, portable.** One static binary plus tmux, on any Linux. | Go, no cgo, no node, no browser, no Docker, no root, no daemon. systemd is *optional*. No assumption about shell, distro, terminal, or network: the daily update check (§6.5) is the only network use, and `update_check = false` removes it. |
| **R6** | **Five agents:** Claude Code, Pi / oh-my-pi, Codex CLI, GitHub Copilot CLI, and a plain `bash` shell session. | Adapter interface with capability degradation — hooks where they exist, pane heuristics where they don't, and honest UI about the difference. |
| **R7** | **TUI-only.** All actions in the UI: create, resume, kill, delete, env edit, permission mode, conversation lock, sidebar pin, search, config. | Discoverability is a feature, not a nicety: inline help, no hidden verbs a user needs. |
| **R8** | **Black-box testable, BDD-specified.** Every behaviour in this spec is expressed as Gherkin and verified against the **real binary** driven through a real terminal, with no in-process hooks and no test-only code paths in the product. | The binary must be *drivable* (keystrokes in) and *observable* (rendered screen, tmux state, files, event-hook invocations, structured log) from outside. Determinism controls — state dir redirection, frozen clock, fixed tick, no animation, no colour — are documented, supported configuration, not test scaffolding (§13). |

### Non-goals (out — do not add)

Git worktrees, branches, PRs, CI · web UI / HTTP server / PWA / tunnels · Docker or
sandboxing · ACP or any structured-render protocol · plugins · **a theme *engine***
(themes are colour-only data files, and a theme can change nothing but colour — §11.6) · **a
user-facing CLI or scripting surface** (choosing a profile at launch, `deck new --agent <kind>` and `deck update`, §3.4, are not one) ·
**multi-host / remote sessions** · declarative config files describing the session set · multiple windows or a shell drawer per session
(one agent or shell per session, full stop) · orchestration, task queues, kanban,
auto-approval · env profiles or secret-manager integrations · auto-restart on crash ·
idle reaping or any timer that stops a running session · **inbound remote control**
(event hooks are one-way; see §10) · cost/token dashboards · MCP management · Windows.

---

## 2. Stack

- Go 1.25+, one module, one static binary `deck`.
- TUI: `charmbracelet/bubbletea` + `lipgloss` + `bubbles` (latest stable, pinned). **Bubble
  Tea stays at v1.3.10**, and `charmbracelet/x/vt` — §11.9's cell grid — coexists with it: the
  incompatibility previously believed to force a v2 upgrade was a stale transitive
  `x/cellbuf`, cleared by `go get github.com/charmbracelet/x/cellbuf@latest`. The one thing
  v1.3.10 genuinely cannot represent is **`Ctrl+Enter`**: both enhanced encodings arrive as an
  unexported `unknownCSISequenceMsg`, `KeyEnter == KeyCtrlM` so no `KeyType` value can mean
  it, it would additionally require `extended-keys always` in the *user's own* tmux config,
  and `send-keys` cannot emit it in either direction. It is therefore not bound anywhere; a
  future v2 migration (`charm.land/bubbletea/v2`, which needs no opt-in) could offer it as a
  configurable alias.
- Store: `modernc.org/sqlite` (pure Go, no cgo) — `WAL`, `busy_timeout=5000` (in force from the very first read of every open — carried on the connection DSN, so a writer holding the lock while a hook opens the store does not fail the schema-version read), `foreign_keys=ON`.
- tmux via the `tmux` CLI. No control mode in v1. **Minimum tmux 3.2**, required for
  `remain-on-exit failed` (§7) and the `window-size` option (§3.3).
  **Every recurring tmux read costs a constant number of `tmux` processes, whatever the number
  of sessions.** Each call is a fork, an exec and a server connect, and the server pays for
  every one from every open deck, so a poll that runs once per session per tick makes the cost
  grow as sessions × open decks — a dozen sessions measured as a steady 8 % of a core per deck,
  plus the server's own share, with nothing on screen moving. So the reconcile tick is **one**
  `tmux` process (§7), a passive preview tick is **one**, and in interactive mode (§11.9) the
  preview tick's backstop checks and the dead-pane poll together are **one** per tick. Where a
  tick needs several facts, they come from one `list-panes -a -F` or one `display-message`
  with a combined format (window user options such as `@deck_isize_owner` included, since
  tmux expands them as formats), or from `;`-chained commands in one invocation. Two things
  are outside this rule: §7's status probe, which samples one pane per probe-eligible session by
  design, at §7's cadence; and the one-off reads of a user action (entry, a keystroke's identity
  check, a resize), which do not recur.
  The "tmux unavailable — install tmux 3.2 or newer" notice is for exactly that condition,
  detected at start-up, and nothing else: a tmux call that fails **at runtime** is a
  transient read error, shown as such with no install advice and cleared by the next
  successful reload. A target that vanishes between two tmux calls — a session, window **or
  pane** removed between a list and a capture — is a removal race and is skipped like the
  others, never reported as an error.
- Runtime deps: `tmux`, plus whichever agent CLIs the user has. Nothing else.
- Test tooling (never linked into the release binary): `cucumber/godog` for Gherkin, a
  VT100 emulator for screen parsing, a capture script for event-hook assertions, and the
  `cmd/fake-*` agent binaries. The harness drives the real `deck` binary (§13).
- Paths: XDG with fallbacks — `$XDG_DATA_HOME/deck/` (default `~/.local/share/deck/`),
  `$XDG_CONFIG_HOME/deck/config.toml`, `$XDG_STATE_HOME/deck/log`. `state.db` is `0600`.
  These are the **default profile's** paths; a named profile nests the same three under
  `profiles/<name>/` (§3.4).

```
cmd/deck/main.go          TUI entrypoint; hidden internal verbs (§3.1)
cmd/fake-*/               fake agent binaries honouring the real argv contracts (§13.2)
internal/tui/             bubbletea model, views, dialogs, keymap, layout, theme
internal/store/           sqlite schema, migrations, queries — the only writer API
internal/tmux/            new-session/list/kill/attach/send-keys/capture-pane/env
internal/service/         the layer between TUI and store/tmux: create, resume, kill,
                          reconcile — every state transition in §7 has its home here
internal/config/          config.toml schema: parse, defaults, and the settings view's
                          field set generated from the same declaration (§6.5, §11.5)
internal/audit/           JSONL structured log incl. the launch audit (§13.1)
internal/agent/           Adapter interface + registry
internal/agent/claude.go  hook injection, assigned session id
internal/agent/pi.go      assigned session id
internal/agent/codex.go   hook injection, id reported by the first hook
internal/agent/shell.go   bash/zsh/fish session, history + scrollback + cwd
internal/hookrecv/        stdin JSON → store event → event-hook dispatch
internal/notify/          event-hook spawner: argv + env + JSON stdin, timeout, output tail
internal/search/          cross-session search over events + transcripts
internal/unit/            embedded systemd user unit template (optional install)
```

---

## 3. Architecture

```
 ┌──────────────┐  ┌──────────────┐  ┌──────────────┐   N clients on N ttys, one host
 │ deck (TUI)   │  │ deck (TUI)   │  │ deck (TUI)   │   no global lock; per-row lease
 │ local term   │  │ ssh tty      │  │ ssh tty      │
 └──────┬───────┘  └──────┬───────┘  └──────┬───────┘
        └─────────────────┴─────────────────┘
                          │ read + targeted writes
             ┌────────────▼──────────────┐      ┌──────────────────────────────┐
             │ SQLite  state.db  (WAL)   │◄─────┤ deck _hook  (short-lived)    │
             │ sessions · events         │      │ spawned by the agent's hooks;│
             └────────────┬──────────────┘      │ writes status, runs the hook │
                          │ mirrors             └──────────────────────────────┘
             ┌────────────▼──────────────┐
             │ tmux server   -L deck     │      No daemon. Nothing runs when you
             │ sessions: deck_<slug>     │      aren't looking except the agents.
             └───────────────────────────┘
```

**Truth model.** tmux is authoritative for *liveness* (does the pane exist). SQLite is
authoritative for *identity and intent* (name, cwd, agent, conversation id, env,
permission mode) and caches the last known *status*. A session in the DB with no tmux
session is `stopped` — the normal state after a reboot.

**No daemon, by construction.** Status is written by `deck _hook`, a short-lived process
the agent's own hook system spawns. The event hook (§10) is spawned by that same process. If no TUI is running, hook-instrumented agents still record status and still
fire it. Liveness is reconciled by whichever TUI is running, and lazily by `_hook`.

**Honest limitation to surface in the UI:** agents without a hook mechanism (`bash`)
are classified by pane heuristics, which only run while a TUI is open. Their rows show a
"sampled" indicator; a hook-instrumented row shows "live". Do not paper over this. The
badge follows the **event source of the row's last verdict, never the agent kind** — which
is exactly what keeps it honest for Codex, whose hooks are real but do not begin firing
until the session's first prompt (§8.2): such a row reads "sampled" until then and "live"
after, with no special case anywhere in the code. A Pi row reads "live" from its first
extension event (§8.3) and "sampled" before it.

### 3.1 Hidden internal verbs

Not a CLI. These exist because external systems need an executable to invoke, and are
undocumented in the UI, excluded from help, and prefixed `_`:

| verb | invoked by | contract |
|---|---|---|
| `deck _hook` | agent hook config | reads one JSON object on stdin, writes one status update + one event, then spawns the event hook (§10) and — on the non-session-end path only — runs one bounded liveness pass while that script runs (§10.4), before exiting (a process death it records spawns the event hook too, §10.4), which is what "lazily by `_hook`" in §3 and "the next `_hook` invocation" in §7 mean. It never probes: pane heuristics are the TUI's, and putting them on the agent's critical path would also falsify §10.3's second limitation. **Two separate budgets:** the store write completes in < 20 ms **uncontended** (measured on a monotonic clock, §13.1 — under multi-client write contention SQLite may legally hold a writer up to `busy_timeout`, so the budget assertion belongs in a single-writer scenario, not a `@multiclient` one; it is a budget for a normal build, so a `-race` build, several times slower by design, does not assert it); event-hook dispatch runs only after the status update and its event row are committed, is bounded separately by `event_hook_timeout` (§10.3), and on the session-end path is started detached (no timeout, no recorded result) and never waited for. A missing, failing or slow script changes neither `_hook`'s exit code nor anything it prints: the dispatch outcome is never an error of the hook. **A state database newer than the hook binary** (the agent keeps the hook command of the deck that launched it, so after deck is upgraded an older `_hook` can meet a newer schema) exits 1 without touching the database and prints, on stderr, a message naming the hook executable's absolute path, its version and its schema, and the database's schema, and saying to restart the session from deck (`R`) so it gets the current hook command. It never says "upgrade deck": that wording stays on the TUI's own store-open path, where the user can upgrade the binary they are running. **Auto-heal:** every store open records, in the `meta` table's `writer_binary` row, the absolute path of the deck binary that last migrated or opened the database for writing (an additive row, not a schema change; the write is skipped when the row already names the running binary, and a failed write never fails the open). Before printing that message, an older `_hook` that has already buffered its stdin payload re-execs the recorded binary once, with the same argv (`_hook`), the buffered payload on stdin and `DECK_HOOK_REEXEC=1` in the environment, and exits with the re-exec'd process's exit code, so the status update lands and the session never notices. The re-exec happens only when the recorded file exists, is an executable regular file, is not the hook's own file, and reports a schema at least as new as the database's (asked with the hidden `deck _schema` verb, which prints the newest schema that binary supports, bounded at 3 s). The re-exec'd writer itself runs under a 10 s timeout: a writer still running at that point is killed and `_hook` prints the restart message above instead, so a wedged writer cannot hold the agent's hook open. `DECK_HOOK_REEXEC` is the loop guard: a process that carries it never re-execs. When any condition fails, `_hook` prints the restart message above and does not re-exec. **The message, the auto-heal with its loop guard and the stale-binding hint (§11.4) hold the same way for Claude, Codex and Pi.** A `copilot` agent session gets its hook command from deck's plugin directory (§8.4): each plugin hook runs `"$DECK_EXE" _hook`, with `DECK_EXE` the launching deck binary in the pane environment, so the session is bound to that binary like the other kinds (the row records it, §11.4) and the pane probe (§7) still carries the status wherever no hook fires. `_hook` does not tell the agent kinds apart (it applies the payload to the row it names), and Claude and Codex embed the identical command, the single-quoted absolute path of the launching deck binary followed by ` _hook`, in `--settings` JSON and in each `-c hooks.<Event>` override respectively; Pi gets the same binary through `DECK_PI_HOOK_EXECUTABLE` for its deck-owned extension (§8.3), which runs it with `_hook` as a separate argv element and no shell, so a Pi session is bound to its launching deck binary and goes stale the same way. |
| `deck _serve-tmux` | optional systemd unit | starts the `deck` tmux server with the right server options and exits. |
| `deck _debug ...` | developers | inspection helpers, built only with the `debug` build tag. Not in release binaries. |

### 3.2 tmux contract

Sessions live on a dedicated socket, `tmux -L deck`, never the default one. A named
profile's socket is `deck-<name>` (§3.4); everything below applies to it unchanged.

- The user's interactive tmux is untouchable; `deck` can neither clobber it nor be
  clobbered by it. `tmux -L deck ls` is the escape hatch if the TUI breaks — surfaced in
  the help view, because plain `tmux attach` will *not* find deck sessions.
- deck creates the server itself on first use and sets, in the same invocation:
  `set -s exit-empty off` (an empty server exits immediately otherwise),
  `set -g remain-on-exit failed` (§7 — keeps a dead pane only when the command exited
  non-zero, which is what makes crash detection possible at all),
  `set -g window-size latest` (§3.3), `setw -g aggressive-resize on`, and `set -g mouse on`
  (top-level `tmux_mouse`, default true, §6.5) so that a wheel notch scrolls the pane's
  scrollback instead of reaching the shell as a history-recall arrow key: with mouse
  reporting off, tmux never claims wheel events, and the outer terminal's alternate-scroll
  behaviour turns them into `Up`/`Down`. One window and one pane per session (below) means
  none of the pane-switching or window-list side effects of `mouse on` have anything to act
  on; the cost is that a drag no longer makes the *terminal's* own selection, which is why
  deck provides its own over the preview (§11.8) and why the terminal's **Shift** override
  remains the route when `mouse` is off. The help view states which of the two applies.
  `set -g history-limit <N>`. tmux's default is 2000, and deck has never set it. This
  matters because §11.9's interactive mode narrows the window: output produced while narrow
  consumes history rows roughly **2.7× faster**, and rows evicted past the limit never
  return — measured at 23 of 40 logical lines destroyed at `history-limit 100` where an
  unnarrowed control lost none. A pane's limit is fixed when the pane is created, so this
  must be set before `new-session`, not after.
  `detach-on-destroy` is deliberately left at its default (`on`) so that killing a session
  another client is viewing returns that client to its TUI rather than silently hopping it
  into an unrelated session.
- Session naming: `deck_<slug>`, slug `[a-z0-9_-]+` derived from the name, uniqueness
  enforced in SQLite. **The slug is fixed when the session is created.** A rename, whether
  typed in the `i` dialog (§11.4) or adopted from the harness (§8), changes the display
  name (`sessions.name`) alone: the stored slug and the live tmux session `deck_<slug>` keep
  their names, so attach, capture and the `DECK_SESSION_SLUG` environment (§6) never move
  under a running agent.
  **`.` and `:` are excluded from slugs** —
  tmux rejects them in session names because they are target-syntax separators.
- **A blank name in the create modal is filled in, not rejected.** deck defaults it to
  `<basename of cwd>-<MMDD-HHMM>` from local wall-clock time (`deck-0820-1443`), because the
  common case is "start something here, now", and making the user invent a name first is a
  toll on the product's fastest path. Names are unique, so a collision appends the smallest
  free `-2`, `-3` suffix rather than failing the create. Two consequences worth stating: the
  default is only *derived*, not special — rename (§11.4) treats it like any other name; the
  row records the derived name as `name_source = 'derived'` provenance (§4) while a typed name
  stores `user`, and so does a later rename; and
  under §13.1's frozen clock it is deterministic, so a second session created in the same
  scenario exercises the collision suffix instead of flaking on it.
- One window, one pane per session (R6/non-goals). No drawers, no extra windows.
- Env is applied by launching the pane's command as `env K=V … <argv>` — portable across
  tmux versions — and mirrored with `set-environment -t` so any future pane agrees.

### 3.3 Attach

**`a` attaches the client directly**: `tmux -L deck attach -t deck_<slug>`. On detach the
client returns to the TUI, which resumes its render loop. `Enter` enters §11.9's interactive
preview instead — the cheaper, reversible half of the same intent — and `a` is the
escalation. Both resize the shared window under `window-size latest`; that is not new, and
§11.9 states the difference in what size each picks.

**`a` releases deck's own pin first.** Every fit deck makes leaves `window-size manual` in the
window options as a side effect of `resize-window`, and those shadow the global `latest`. A
client attaching under one is shown the window at deck's chosen size — cropped into a corner
of its own terminal, and staying there, since nothing re-expresses a size for a window that
cannot follow a client. Passive fit unsets the option itself, and issues no `resize-window` at
all while a client is attached (§11) — so no deck's preview, this one or another on the same
server, re-crops a terminal that is already attached. That leaves `a` the pins
`a` did not write: one from another deck process on the same server, or from a deck that was
killed while interactive and not yet reclaimed. `a` unsets the window-local option before it
hands the terminal over. The one pin it leaves is a **live owner's** (§11.9) — that size is
held for a grid another process is drawing right now, and releasing it would resize the window
under that process the moment this client attached. So: **deck's own preview never crops a full
attach**; another tmux session, deck-mediated or a direct `tmux attach`, still may, exactly as
the shared-geometry paragraph below describes.

**Geometry, stated honestly:** two clients attached to the same session at different
terminal sizes *share* one view. Grouped sessions do not fix this — a group shares its
windows, and deck sessions have exactly one window (§3.2), so every client necessarily
displays the same window. `window-size latest` makes the most recently active client
govern the size, and `aggressive-resize` keeps the window matched to it; an idle client on
a smaller terminal will therefore see truncation until it becomes the active one. This is
the same behaviour every tmux-based manager has, it is not worth a redesign, and it must
be documented in the help view rather than promised away.

**Detaching does not restore geometry** when the detaching client was the only one. Under
`window-size latest`, "latest" means the last client to *express* a size, and once it is gone
nothing re-expresses the old one. Restoring is therefore a positive action, not a
consequence — §11.9 specifies it for interactive mode, and an ordinary `a` attach/detach
leaves the window at the size the attaching client had.

Nested tmux (running the TUI inside another tmux) is out of scope for v1: detect `$TMUX`,
warn, and attach with `TMUX` unset.

### 3.4 Profiles: independent decks side by side

A **profile** is a whole, independent deck — its own config, state, log and tmux socket —
so that work and home, or two organisations, never share a session list. (It is unrelated to
§5's *permission* profiles; the word is qualified wherever the two could be confused.)

- **Selection is the only argument.** `deck` and `deck default` open the default profile;
  `deck work` opens `work`; `DECK_PROFILE=work deck` does the same. Precedence: positional
  argument, then `DECK_PROFILE`, then `default`; an empty `DECK_PROFILE` is unset. At most one
  positional argument (`deck work home` is an error), and flags are recognised before it, so an
  unknown `-x` is `error: unknown flag -x`, never a profile name. `--version` and the hidden
  verbs (§3.1) are unchanged. `deck --profiles` lists profiles and exits.
- **`deck new` is the profile `new`; `deck new <option>…` creates a session.** Bare `deck new`
  is exactly what the first bullet says -- the profile named `new` is a valid profile name, so
  it stays reachable. `new` followed by options is instead the creation form, which opens no
  TUI: `deck new --agent <kind> [--name <n>] [--cwd <dir>] [--permission <p>]` (each option
  also as `--option=value`) creates one detached session on the resolved profile
  (`DECK_PROFILE`, else `default`) through the same service as the create dialog and prints its
  name. `--cwd` defaults to the working directory, a blank or omitted `--name` to the same
  `<basename of cwd>-<MMDD-HHMM>` default as the create modal (§3.2; collisions take the first
  free `-2`, `-3` suffix; stored as `derived` provenance; a given `--name` stores `user` and is
  used verbatim), `--permission`
  to `safe`. `<kind>` is any registered agent -- `claude`, `codex`, `copilot`, `pi`, `shell` --
  and is taken from `--agent`, else from config.toml's `agent` key (§6.5); neither is exit 2
  `error: deck new needs --agent KIND (or an agent key in config.toml)`. An unregistered kind
  is exit 2 `error: unknown agent kind "<kind>"` -- the create service's own message, before
  anything on disk is opened -- and an unknown option is `error: unknown flag <arg>`. A kind
  whose executable is not on `PATH` is refused by the create service (§6.3) with exit 1.
- **`deck update` is a detached verb, and a profile named `update` is reached with
  `DECK_PROFILE=update`.** The word `update` as the first argument always runs the verb (it opens no
  TUI), so `deck update` never opens the profile `update`; `DECK_PROFILE=update deck` does, and
  `deck update` is unaffected by `DECK_PROFILE`. Like `--version` it is answered before the
  configuration is loaded and before tmux is looked for, so it works with neither. `deck update`
  installs the latest release over the running binary through the verified path of §13.1;
  `deck update --check` makes one request and changes nothing (no file, no state row) and reports
  `update available: <old> -> <new>` or `deck <old> is up to date`; `deck update --version vX.Y.Z`
  pins a release and permits a downgrade (it cannot be combined with `--check`). An install prints
  `deck updated: <old> -> <new> (<path>)` and a note that a running deck must be restarted to use the
  new version (running sessions are not disturbed); when the latest release is not newer it prints
  `already the latest release` and changes nothing. Every failure (a refusal of §13.1, a checksum or
  attestation failure, no network, an unknown option) prints `deck update: <cause>` on stderr and
  exits 1, with the installed binary byte-identical; an unknown option or a stray argument is exit
  2. An offline `--check` is such an error and changes nothing.
- **A deck stays on the profile it launched on.** There is no switcher and no in-place
  switch: changing profile is quitting and starting `deck <other>`, and decks on different
  profiles run side by side in different terminals. Several clients on one profile behave
  exactly as R4 says.
- **The default profile is the flat layout and socket `deck`**, byte-for-byte: no migration,
  no file move, no socket rename, and `profiles/default/` is never created. **A named profile
  lives under `profiles/<name>/` and uses socket `deck-<name>`:**

  | mode | default | `<name>` |
  |---|---|---|
  | XDG | `$XDG_CONFIG_HOME/deck/config.toml`, `$XDG_DATA_HOME/deck/state.db`, `$XDG_STATE_HOME/deck/log/` | the same three with `profiles/<name>/` inserted after `deck/` |
  | `DECK_HOME` | `$DECK_HOME/{config.toml,state.db,log/,clock.now}` | `$DECK_HOME/profiles/<name>/{config.toml,state.db,log/,clock.now}` |

  Everything that hangs off the data root — captures (§9.4), history, the shared frozen clock
  (§13.1) — follows the profile's root, so two profiles never share a clock or a capture.
  **User themes are shared**: every profile discovers them in the default profile's
  `themes/` directory (§11.6), because a theme is colour data, not state. `DECK_TMUX_SOCKET`
  still overrides the derived socket name and wins.
- **Names match `^[a-z0-9][a-z0-9_-]{0,15}$`**: 1–16 characters, lowercase letters, digits, `-`
  and `_`, starting with a letter or digit. The first-character rule excludes flags, `_`
  internals, `.` and `..` in one rule; lowercase-only keeps `Work` and `work` from colliding on
  a case-insensitive filesystem; 16 keeps `deck-<name>` far below the Unix socket path cap
  under a long `TMUX_TMPDIR` and fits the sidebar header. `default` is valid and always means
  the flat layout. **One validator** checks every entry point — the argument, `DECK_PROFILE`,
  `_hook`'s pane environment, and the `--profiles` scan — before anything touches disk or
  tmux, and its message quotes what was typed and names the rule broken
  (`error: profile name "acme.prod" contains "."; allowed: a-z 0-9 - _`,
  `error: profile names are lowercase; did you mean "work"?`, a length message naming the
  length and the limit of 16, a first-character message; a `DECK_PROFILE` failure is prefixed
  `DECK_PROFILE="…":`). An invalid name, an unknown flag or a second positional argument
  exits **2**.
- **Profiles are created lazily, behind a typo guard.** Launch order is validate → confirm if
  unknown → create → launch. An unknown valid name asks once on the terminal —
  `deck: no profile "wrok" yet (known: default, work). Create it? [y/N]` — and only `y`
  creates it; anything else exits 1 and creates nothing. **If stdin is not a terminal deck
  refuses** with the same message, exit 1, and creates nothing. Creation copies the default
  profile's `config.toml` once, if it has one, and the two files are independent from then on;
  `state.db` and `log/` come from the normal first-launch path. **Whether a profile exists is
  a directory scan** of the data root's `profiles/` (`$XDG_DATA_HOME/deck/profiles/`, or
  `$DECK_HOME/profiles/`) — there is no registry to drift from the disk; creation makes the
  profile's config, data and log directories together.
- **Every pane carries `DECK_PROFILE`** (§6.1), `default` included, and `_hook` resolves its
  paths from it with the same resolver the TUI uses, so a pane writes to the database of the
  profile that created it for its whole life. A pane with no `DECK_PROFILE` resolves to
  default. If the pane's `DECK_PROFILE` is invalid, or names a profile whose directory no
  longer exists, `_hook` writes nothing, creates nothing, **never falls back to default's
  database** (that would record one organisation's status in another's), prints one line on
  stderr naming the pane and the profile, and exits **exactly 1** — never 2, which several
  Claude Code hook events read as "block this action".
- **`deck --profiles`** prints one line per profile, default first: name, socket, config and
  data paths, and "last used" — the mtime of a `last_used` marker the TUI touches in the
  profile's data root at launch, default included (a directory's own mtime moves with WAL
  checkpoints and log rotation). It never opens a `state.db`. A directory under `profiles/` whose name fails
  validation is listed flagged `(invalid name: not selectable)`, and the exit code is still 0.
- **A hard wall.** A profile never reads another profile's `state.db`: no cross-profile
  counts, no aggregate view. Settings (§11.5) always write the running profile's own
  `config.toml`. **deck never deletes a profile** — it owns a socket that may hold live
  agents; help gives the manual steps (`tmux -L deck-<name> kill-server`, then remove its
  `profiles/<name>/` directories).
- **Display.** For a named profile the sidebar's header line reads
  `profile: <name> · socket: deck-<name>` (the socket half elides first when it does not fit —
  the name is what matters at a glance) and the terminal title is `deck: <name>`. For default
  the line stays exactly `socket: deck` and deck sets no title, as it never has.

---

## 4. Data model

`$XDG_DATA_HOME/deck/state.db`, mode `0600`, schema version in `meta`.

The daily update check (§6.5) keeps its answer outside every `state.db`, in one small JSON file
that all profiles share: `update-check.json` in `$DECK_HOME` when `DECK_HOME` is set, otherwise
`$XDG_CACHE_HOME/deck/update-check.json` (`~/.cache/deck/update-check.json`). It holds the time of
the last attempt and the latest release tag seen, nothing else (no credential, no identifier), and it
is replaced atomically, under the lock file `update-check.json.lock` beside it (§6.5). It is the single thing profiles share; a profile's sessions, settings and
`state.db` stay its own.

```sql
CREATE TABLE sessions (
  id                 TEXT PRIMARY KEY,      -- deck's own uuid, stable forever
  name               TEXT NOT NULL UNIQUE,
  slug               TEXT NOT NULL UNIQUE,  -- tmux session = deck_<slug>
  cwd                TEXT NOT NULL,         -- create-time (the cleaned path), never overwritten; NOT unique (R2)
  last_cwd           TEXT,                  -- pane's cwd at the last capture (§9.4); resume target
  agent              TEXT NOT NULL,         -- claude | pi | codex | copilot | shell
  launch_args        TEXT NOT NULL DEFAULT '[]', -- JSON array, extra agent args
  env                TEXT NOT NULL DEFAULT '{}', -- JSON map, per-session overrides
  env_dirty          INTEGER NOT NULL DEFAULT 0, -- env edited while running → restart to apply (§6.2)
  launch_dirty       INTEGER NOT NULL DEFAULT 0, -- a launch input other than env edited while running (§6.2)
  captured_path      TEXT NOT NULL,         -- PATH at create time (§6.3)
  pre_launch         TEXT,                  -- one shell line run in the pane before the agent (§6.4)
  post_destroy       TEXT,                  -- one shell line run after A or dd, never after x (§9.2)
  login_shell        INTEGER NOT NULL DEFAULT 0, -- run argv via `$SHELL -lc`
  permission_profile TEXT NOT NULL DEFAULT 'safe', -- safe|plan|edits|yolo (§5)
  permission_profile_reason TEXT,        -- why the profile degraded (§5); NULL = it didn't
  conversation_id    TEXT,                  -- the agent's own session id; NULL until known
  resume_pin         TEXT,                  -- forced conversation id
  resume_state       TEXT NOT NULL DEFAULT 'auto', -- auto | pinned | cleared
  status             TEXT NOT NULL,         -- §7
  status_reason      TEXT,
  status_source      TEXT NOT NULL,         -- hook | probe | tmux | user
  status_at          INTEGER NOT NULL,
  killed_by_user     INTEGER NOT NULL DEFAULT 0, -- terminal user verdict; hooks can't undo it
  pane_exit_status   INTEGER,               -- from tmux pane_dead_status; NULL = not dead
  crash_tail         TEXT,                  -- pane tail at death, last 200 lines (§7)
  notify_epoch       INTEGER NOT NULL DEFAULT 0, -- bumped when an attention state resolves (§10.3)
  last_message       TEXT,                  -- last assistant message, truncated 2 KiB
  sensitive          INTEGER NOT NULL DEFAULT 0, -- suppress scrollback capture (§8)
  event_hook_enabled INTEGER,               -- tri-state (§10.2): NULL = inherit, 1 = on, 0 = off
  event_hook_events  TEXT,                  -- JSON list of event kinds replacing the global list; NULL = inherit
  hook_fired         TEXT,                  -- JSON (kind,reason) pairs fired in this notify_epoch (§10.3)
  important          INTEGER NOT NULL DEFAULT 0, -- exported to the event hook (§10.1)
  group_id           INTEGER,               -- manual group (§11); NULL = the implicit "default"
  seen_status_at     INTEGER NOT NULL DEFAULT 0, -- the status_at stamp the user has seen (R318, schema v18); a row's status is seen iff seen_status_at = status_at. Replaces the dropped `acknowledged` bit; the v18 rung backfills it to status_at where acknowledged was 1, else 0
  launch_lease_owner TEXT,                  -- pid@boot_id#generation holding a start (§9.3)
  launch_lease_until INTEGER NOT NULL DEFAULT 0,
  last_probe_at      INTEGER NOT NULL DEFAULT 0, -- last pane sample that matched no §7 rule; a column, never an event (§7)
  last_output_at     INTEGER NOT NULL DEFAULT 0, -- the output clock: ms of the newest transcript-region output reconcile confirmed (§7); a column, never an event
  last_seen_at       INTEGER NOT NULL DEFAULT 0, -- the last_output_at value the user has viewed (R324, schema v19); a column, never an event. A row is output-unseen iff last_output_at > last_seen_at (§7, §11)
  probe_miss_streak  INTEGER NOT NULL DEFAULT 0, -- consecutive probe passes that matched no §7 rule (R315); a column, never an event
  created_at         INTEGER NOT NULL,
  last_attached_at   INTEGER NOT NULL DEFAULT 0,
  archived_at        INTEGER NOT NULL DEFAULT 0,
  pinned_at          INTEGER NOT NULL DEFAULT 0, -- sidebar pin (§11); 0 = not pinned, else when it was pinned
  name_source        TEXT NOT NULL DEFAULT 'user' CHECK (name_source IN ('user','derived','harness')), -- who last wrote `name`: user = typed in deck, derived = deck's default from the cwd or first prompt, harness = adopted from the agent's own session title
  name_adopted       INTEGER NOT NULL DEFAULT 0 CHECK (name_adopted IN (0,1)), -- 1 = `name` came from the harness (adopted, or frozen as `user` because a human chose it inside the harness); such a name never reaches a launch or resume argv, and a rename typed in deck clears it
  pending_harness_name TEXT,                -- the name deck pushed into the running harness and has not yet seen confirmed (§8 Pushing a rename); NULL = nothing pending
  pending_harness_name_at INTEGER,          -- when it was seen (ms); NULL together with pending_harness_name
  harness_name_adopt INTEGER,               -- tri-state per-session adoption override: NULL = inherit the global setting, 1 = adopt, 0 = never adopt
  hook_executable    TEXT,                      -- absolute path of the deck binary the agent's hook command was last launched with (§11.4); NULL = none recorded
  deleted_at         INTEGER NOT NULL DEFAULT 0  -- tombstone; purged after grace (§9.2)
);

CREATE TABLE events (               -- append-only within a retention bound: audit trail, search corpus, event-hook source
  seq        INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT REFERENCES sessions(id) ON DELETE CASCADE,
  at         INTEGER NOT NULL,
  kind       TEXT NOT NULL,         -- the writer's own name for what it recorded, stored as written and never
                                    -- renamed: a hook's event (session_start, user_prompt_submitted,
                                    -- notification, permission_request, stop, stop_failure, session_end),
                                    -- probe.<status>, tmux.pane_dead, tmux.session_gone, killed, launch.*,
                                    -- restart, env_applied, note, set_*, harness_rename_refused, harness_rename_failed, ... It is NOT the event hook's
                                    -- vocabulary: §10.4 maps a stored kind to the offered kind (§10.1)
  reason     TEXT,
  payload    TEXT,                  -- bounded JSON
  hook_kind      TEXT,              -- the offered kind the event hook was spawned for (§10.3); NULL = no hook ran
  hook_exit      INTEGER,           -- its exit status (-1: timed out, killed by a signal or never started)
  hook_timed_out INTEGER,           -- 1 when event_hook_timeout expired and the process group was killed
  hook_output    TEXT,              -- capped, scrubbed tail of its stdout and stderr (§10.1)
  hook_error     TEXT               -- why it could not be started (missing, not executable), else NULL
);

CREATE INDEX events_at ON events(at DESC, seq DESC);         -- §12's newest-first reads are never a full scan
CREATE INDEX events_session_kind ON events(session_id, kind); -- §6.4's env-apply reads likewise

CREATE TABLE groups (               -- manual session groups (§11); machine-local, not config.toml
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE COLLATE NOCASE  -- "default" is reserved: it is group_id IS NULL
);                                  -- membership is sessions.group_id, so a rename carries it

CREATE TABLE ui_state (             -- machine-local UI state, never in config.toml (§6.5)
  key        TEXT PRIMARY KEY,      -- layout_mode, sidebar_width (§11.2), collapsed_groups (§11)
  value      TEXT NOT NULL
);

CREATE TABLE recent_cwds (          -- the create modal's directory history (§11.7)
  path       TEXT PRIMARY KEY,      -- resolved absolute path, deduplicated
  used_seq   INTEGER NOT NULL       -- monotonic, NOT a timestamp: order stays assertable
);                                  -- under a frozen DECK_CLOCK (§13.1)

CREATE TABLE bookmarks (            -- the `bookmarks` setting's slots (§6.5, §11); machine-local
  slot       INTEGER PRIMARY KEY CHECK (slot BETWEEN 0 AND 9), -- one session per slot
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE -- a session may hold several slots
);
```

Invariants:
- `cwd` is not unique and carries no git columns — R1 and R2 enforced by schema.
- `archived_at` and `deleted_at` are **flags, not statuses**: an archived session keeps
  whatever `status` it had. Archiving requires `stopped`, and on a live session `A` confirms
  first and then kills and archives as one action (§9.2, §11.4) — so an archived row can
  never hide a live agent.
- **Both flags are reversible, and an archived row is not startable.** `archived_at` has an
  unarchive (`U`, §11) exactly as `deleted_at` has a restore, and `r`/`R` refuse an archived
  row and name that key rather than launching it. Neither half stands alone: a one-way
  archive flag *plus* a resumable archived row is precisely the combination that puts a live
  agent behind the filter with no way back, so the invariant above is upheld in both
  directions — `Archive` guards live → archived, resume guards archived → live (§9.1).
- Every mutation is a targeted `UPDATE … WHERE id = ?` in a transaction. Never rewrite a
  table from in-memory state (R4).
- `events` retained 30 days by default, pruned on TUI start.
- `ui_state` and `recent_cwds` are the only tables not keyed to a session, and neither is
  load-bearing: losing them costs a remembered layout and a prefilled path, never a
  session or a conversation. Dropping either is therefore a legal recovery action, and a
  missing row degrades to the documented default rather than to an error.
- Every column above is reachable by migration from schema version 1 — the store is never
  rebuilt and a session row is never recreated to gain a field.
- Schema version 10 retires the earlier notification-rule and snooze columns (and an outbox
  table, which no shipped schema creates) and adds the nullable `event_hook_enabled`, `event_hook_events`
  and `hook_fired` columns shown above; every migrated row reads back "inherit, nothing
  fired". `hook_fired` is cleared (set to `NULL`) in the same write that advances
  `notify_epoch`, so a new attention episode may fire its hooks again (§10.3).
- Schema version 11 adds the nullable `hook_kind`, `hook_exit`, `hook_timed_out`, `hook_output`
  and `hook_error` columns to `events`, the event hook's result against the event it ran for
  (§10.3); every migrated event reads back "no hook ran".
- Schema version 12 adds `name_source`, `pending_harness_name`, `pending_harness_name_at` and
  `harness_name_adopt` to `sessions`. `name_source` is `user`, `derived` or `harness` (enforced by
  the column) and records who last wrote `name`; every migrated row reads `user`, so a name that
  existed before the upgrade is never overwritten. The other three columns are nullable and
  every migrated row reads them `NULL`: nothing pending, and no per-session adoption override.
  The create modal and `deck new` write `derived` when the name was left blank and `user` when it
  was typed; a harness adoption writes `harness`.
- Schema version 13 adds `name_adopted` to `sessions`. A migrated row keeps the origin schema 12
  recorded: it reads 1 when its `name_source` is `harness`, or when its latest `renamed` event
  came from a harness (`reason` `harness` or `harness:<kind>`) and named its current name (an
  explicit harness title adopted or frozen as `user`); every other migrated row reads 0. A harness
  adoption (including one frozen as `name_source = user`) sets it to 1, in the same transaction
  as the name; a rename typed in deck sets it back to 0.
  A rename writes `name_source` in the same transaction as `name` and the `renamed` event, whose
  `reason` is the source of the rename; a source other than `derived` or `harness` is `user`.
- Schema version 14 adds `bookmarks`: slot 0-9 mapped to a session id. The slot is the primary
  key, so a slot names one session and assigning a taken slot moves it rather than duplicating it,
  while a session may hold several slots. Deleting a session removes its slots: tombstoning it
  (`deleted_at` set) clears them in the same write, and reaping the row cascades. A bookmark of a
  deleted session is therefore never restored by `u`. Archiving a session (`A`, §4) clears its slots
  the same way: `ArchiveSession` deletes the session's `bookmarks` rows in the transaction that sets
  `archived_at`, so a failed archive leaves the slots in place. Un-archiving (`U`) and undoing an
  archive (`u`) do not restore them; the slots stay empty until assigned again. Clearing an empty
  slot is not an error. The table is machine-local, survives a restart,
  and is not load-bearing: a missing slot is an empty slot, and a jump to an empty slot is a no-op
  with a one-line status message. A migrated database starts with no slot assigned.

- Schema version 15 adds `sessions.last_output_at` (the output clock, §7), `0` until a session's
  transcript has been seen to change. `ALTER TABLE ADD COLUMN` touches no existing value, so a migrated
  database starts every session at 0 and recreates no row. The column is written by a plain
  `UPDATE` and never as an `events` row.
- Schema version 19 adds `sessions.last_seen_at` (R324, §7, §11), the `last_output_at` value the user has
  viewed. The rung backfills it to `last_output_at`, so an upgrade does not light every existing session.
  It is written by a plain `UPDATE` (`MarkViewed`) and never as an `events` row.
- Schema version 16 adds `sessions.probe_miss_streak` (R315, §7), `0` on a migrated row. It is written by
  plain `UPDATE`s and never as an `events` row.
- Schema version 17 adds the index `events_session_at` on `events(session_id, at)` (R315, §7), so the newest
  event of a session (the unverified marker's quiet window) is one index probe. It adds no column and
  rewrites no row.

---

## 5. Permission modes

A deck-level profile per session, translated per adapter — never a boolean.

The flag names below are upstream contracts, not deck's: Claude's `--permission-mode`
accepts `manual | plan | acceptEdits | auto | dontAsk | bypassPermissions`. Keeping this
table true as those CLIs move is the job of the `@real-agents` suite (§13.5), never of a
reader's memory — and the `safe` row is the standing proof that it has to be: Claude renamed
its own default mode from `default` to `manual` between 2.1.71 and 2.1.259, so a deck that
names the mode explicitly refuses to launch on one of those versions. **`safe` therefore
passes no mode flag at all.** Claude's built-in default *is* the interactive-approval mode on
every release, old and new, so naming it buys nothing and costs a version dependency. The
three non-default profiles keep their explicit flags, whose names have been stable.

Codex's approval surface was **verified against codex-cli 0.154.0**: `-a/--ask-for-approval`
accepts `on-request | never` (only those two — the older `untrusted`/`on-failure` values and
`--full-auto` are gone), and `-s/--sandbox` accepts
`read-only | workspace-write | danger-full-access`. Per the structured-flag preference below,
`yolo` composes `-a never -s danger-full-access` rather than
`--dangerously-bypass-approvals-and-sandbox`, which is the same effect under a name that
churns. **Codex's `safe` passes its flags explicitly and never relies on the CLI's own
default**, because that default is user-configurable: `approval_policy` and `sandbox_mode` in
the user's `~/.codex/config.toml` can — and on real installations do — set `never` and
`danger-full-access` globally, so a `safe` session that omitted the flags would silently
inherit full access. A profile named `safe` that depends on the user's config not being
permissive is not a profile, it is a wish.

| deck profile | Claude Code | Pi | Codex | Copilot CLI | shell |
|---|---|---|---|---|---|
| `safe` (default) | **no flag** — Claude's own default mode | default | `-a on-request -s workspace-write` (explicit, never the CLI default) | **no flag** | n/a |
| `plan` | `--permission-mode plan` | n/a → falls back to `safe`, shown in UI | n/a → falls back to `safe`, shown in UI | n/a → falls back to `safe`, shown in UI | n/a |
| `edits` | `--permission-mode acceptEdits` | `--approve` | `-a never -s workspace-write` | `--allow-tool=write` | n/a |
| `yolo` | `--permission-mode bypassPermissions` | `--approve` | `-a never -s danger-full-access` | `--allow-all` | n/a |

- Prefer the structured mode flag over a `--dangerously-*` flag where both exist: same
  effect, less flag-name churn. Unsupported profiles degrade to the nearest safe one and
  say so in the row detail rather than silently lying.
- **Copilot's `edits` is narrower than Claude's.** `--allow-tool=write` lets Copilot's file
  edits through without prompting, while shell commands still prompt: a copilot `edits` session
  can still go `waiting` on a shell command, which is correct, not a stuck session. The create
  dialog offers `copilot` with exactly `safe`, `edits` and `yolo` (no `plan`), and only when
  `copilot` resolves on `PATH`; a host without it lists `copilot` under "not on PATH" like a
  missing `codex`.
- **Persisted**, so a `yolo` session comes back `yolo` on resume. That's the point, and
  that's why it needs a badge visible in the list, in the detail pane, and in every
  event-hook payload.
- `yolo` is offered when `allow_yolo = true` in config (default false), and then needs no
  further ceremony: choosing it in the create modal, or switching to it with `P` inside
  the `i` detail dialog (§11.4), takes effect directly. With `yolo_default = true` (default false) the create modal opens already
  on `yolo`; it is inert while `allow_yolo` is false, and settings says so on the row rather
  than silently ignoring it. The gate is a deployment decision, not a per-launch speed bump —
  a confirm on every create trains the user to press it, and the safeguard that survives
  habituation is that the profile is *visible* everywhere it applies (below), not that it is
  tedious to choose.
- **Copilot's own settings can elevate a `safe` or `edits` launch.** Copilot reads user
  settings of its own, and deck writes nothing under `~/.copilot` (§8.4), so a session launched
  `safe` (no flag) or `edits` can still start in Copilot's `Allow All` mode, and deck cannot stop
  it. The profile deck stores is the one it launched with; it is never rewritten. When the
  pane probe (§7) samples a copilot row launched `safe` or `edits` whose footer, below the
  composer, shows `Allow All` (the footer of a normal launch shows `Manual Approval`), it stores
  a reason saying Copilot's own settings elevated the profile, which the detail pane shows on
  the same `degraded:` line an unsupported profile uses (an existing reason is kept and
  extended, and a pane already reported is not rewritten). A `yolo` launch is `Allow All` by
  design and is never flagged, and Claude, Codex and Pi rows are never audited this way. The
  **folder-trust screen** (`Confirm folder trust`) is not a profile question: it is a `waiting`
  state with reason `folder trust` (§8.4), which only `yolo` suppresses (it sets
  `COPILOT_ALLOW_ALL=true`), and attaching to answer it is the user's step, as for any prompt.
- Claude hook payloads carry `permission_mode`, so if the user changes it in-session the
  row is reconciled from the hook instead of drifting. **Codex's payloads carry a field of
  the same name and it must not be used this way.** Verified on 0.154.0 it reports only
  `default` or `bypassPermissions`, tracks the approval policy alone, and is blind to `-s`
  entirely — so reconciling a codex row from it would re-label deck's own `edits` session
  (`-a never -s workspace-write`) as `yolo`. A field being present is not the same as it
  being the same field.
- In `yolo`, permission prompts never fire, so the `waiting` column goes quiet — attention
  then comes only from questions / needs-input notifications. Document this in help; it is
  a frequent "status is broken" false alarm.
- Adapter capabilities are declared, not assumed: each adapter reports which profiles it
  supports, and the create modal only offers those. An adapter likewise declares the
  **executable** its launch argv starts, and the create modal offers only the kinds whose
  executable resolves on the launch `PATH` at the moment the modal opens (§6.3) — a kind that
  is registered but not installed is absent from the field, not listed and doomed. `shell`
  declares no executable and is always offered: it is the floor, the one session kind a fresh
  host can always create.

---

## 6. Environment

Flat per-session overrides. No profiles, no bundles, no secret managers (non-goals).

### 6.1 Layers

Resolution, lowest to highest: environment of the process that started the tmux **server**
→ `[env]` in `config.toml` → session `env` map → **deck's own session context**. The env
editor shows the effective value per key with its winning layer.

**The session context is deck-owned and unoverridable.** Every pane deck launches — every
adapter, `shell` included, on create and on resume alike — carries the launching session's own
row facts as `DECK_SESSION_*`. They are merged last, above the user's own layers, precisely so
that a session `env` or a config `[env]` key of the same name cannot lie to a hook about which
session it is running for:

| variable | value |
|---|---|
| `DECK_SESSION_ID` | the row's uuid (§4) — the identity `_hook` already carries (§8.1) |
| `DECK_SESSION_NAME` | the display `name` as of this launch |
| `DECK_SESSION_SLUG` | `slug`, which is tmux's `deck_<slug>` identity (§3.2) |
| `DECK_SESSION_CWD` | the directory the pane was launched in |
| `DECK_SESSION_AGENT` | the adapter kind: `claude`, `pi`, `codex`, `copilot` or `shell` |
| `DECK_SESSION_GROUP` | the manual group's name (§11), empty for the implicit `default` group |
| `DECK_SESSION_PROFILE` | the **resolved** permission profile in force (§5), never the requested one |
| `DECK_SESSION_CONVERSATION_ID` | the agent's own conversation id where the adapter assigns one before launch (§8), empty otherwise |
| `DECK_SESSION_LAUNCH_KIND` | `create` on a first launch, `resume` on every relaunch — `r` and `R` are both `resume`, since first-launch-or-not is the distinction a hook can act on |

Two rules make this usable from a shell hook. **Every variable is always exported**, empty
rather than absent when the column behind it is unset, so a hook can branch on a value without
first testing for existence — "unset" and "empty" being different states is a trap in shell.
And it is a **launch-time snapshot, not a live view**: a rename (§9.2) or a profile switch (§5)
reaches an already-running pane no more than an `env` edit does, which is precisely what §6.2
is about.

`DECK_HOME`, `DECK_PROFILE` and `DECK_LAUNCH_GENERATION` are not session properties and are
unchanged — the first two name the data root and the profile a hook writes to (§13.1, §3.4;
`DECK_PROFILE` is always set, `default` included), the third is the launch lease's own
discriminator (§9.3) and is absent when a launch took no lease. All three are likewise
deck-owned.
Adapter *instrumentation* (§8.1) stays what it is, the facts an adapter owns such as Claude's
`--settings` hook config, and never restates a session fact: a name or a cwd duplicated per
adapter is a name or a cwd that will drift.

### 6.2 Editing while running

tmux env changes reach only *new* processes, and a launch input is by definition consumed at
launch, so a mid-flight edit of either is inherently restart-to-apply:

1. Edit in the TUI → the row's own column is written and a dirty flag set. `e`'s env editor
   writes the session `env` map, sets `env_dirty = 1` and mirrors to
   `tmux set-environment -t`; the **launch-inputs editor** (§11.4, reached from `i` exactly as
   rename is) writes `pre_launch`, `post_destroy`, `launch_args` or `login_shell` and sets
   `launch_dirty = 1`.
2. The row shows an `env↻` badge for the first and a `launch↻` badge for the second:
   *changed, not yet applied*. Two flags rather than one because they are cleared by different
   things — see 3 — and a row may legitimately carry both at once.
3. `R` restarts the pane and relaunches with the **resume** argv — new environment, new launch
   inputs, same conversation — and clears both flags. For `shell` sessions `R` also offers
   "inject instead" (`export K=V` into the live shell), which genuinely works there and clears
   `env_dirty` **alone**: typing an export into a live shell cannot retroactively re-run a
   `pre_launch` that has already run, so an injected env never clears `launch_dirty`.

**Not every launch input is editable, and the exclusions are identity rather than difficulty.**
`agent` is the adapter's identity — changing it invalidates the conversation, the argv and the
instrumentation in one move. `cwd` is create-time by R2, and `slug` is tmux's identity (§3.2),
which is why a rename changes `name` alone. `captured_path` is derived (§6.3), not typed. Each
of those is changed by creating a session, not by editing one.

Nothing is applied silently. A restart is always an explicit keypress.

### 6.3 The PATH trap

A tmux server started by a systemd user unit inherits the user manager's environment:
thin `PATH`, no shell rc files, no agent SSH socket, no keyring. This is the single most
common reason a resumed session fails to launch. Mitigations, all three:

- `captured_path` records the `PATH` in effect when the session was created. It sits
  **between** the server environment and `config.toml`'s `[env]` in the §6.1 order: it beats
  the (possibly thin) inherited `PATH` and loses to any `PATH` the user sets in `[env]` or
  in the session's own env map. Full order, lowest to highest: server env → `captured_path`
  → config `[env]` → session `env` → §6.1's deck-owned session context (which contains no
  `PATH` and so never participates in this resolution, but is above all of it).
- `login_shell = true` runs the pane command through `$SHELL -lc`, giving a full login
  environment where that's wanted. **It also lets rc files rewrite `PATH`, discarding
  `captured_path`** — that is the trade, it is what the option is *for*, and the two are
  therefore mutually exclusive by design: enabling `login_shell` marks `captured_path`
  advisory and the health view says so.
- **Availability is probed, not assumed, before a launch is attempted.** The create modal
  lists only the kinds whose declared executable (§5) resolves on the `PATH` a new pane would
  get — deck's own `PATH` as `captured_path`, then `[env]`'s override if any — probed when the
  modal opens, never per frame (§11.4). Create and resume both refuse, before any pane exists,
  an agent whose executable does not resolve on the pane's resolved `PATH`: create reports it
  in-dialog and writes nothing; resume lands the row in `error` with that reason (§9.1). When
  `login_shell` is on, the login shell's own rc files decide `PATH` and deck cannot judge
  membership, so the refusal is skipped on both paths and the modal's listing is the best guess
  the non-login `PATH` gives — `shell` remains as the floor either way.
- The health view flags any session whose agent binary is not resolvable from the
  environment the server will actually use, and flags a session where `login_shell` and an
  explicit `PATH` override disagree.

### 6.4 Secrets

Session `env` values are stored literally in `state.db`. Therefore:

- `state.db` is `0600`; the parent directory is `0700`.
- Values whose key matches `*TOKEN*|*SECRET*|*KEY*|*PASSWORD*|*CREDENTIAL*` are masked in
  every view and in event-hook payloads; reveal is a per-view explicit toggle.
- Env values never enter `events`, event-hook payloads, or logs.
- Deleted and overwritten env values are zeroed in `state.db`: it runs with
  `PRAGMA secure_delete=ON`, so freed database pages are overwritten rather than kept. Stale
  WAL frames are not rewritten; they stay in `state.db-wal` until SQLite overwrites them in a
  later checkpoint cycle, so the `0600` mode on `state.db` and its `-wal`/`-shm` siblings is
  the protection for that window. The WAL stays on disk across closes (a reset WAL would bring
  back the fresh-WAL fsync on the hook path), so `journal_size_limit=0` is deliberately not
  used.
- `pre_launch` exists precisely so secrets need not be stored at all: one shell line run
  in the pane before the agent starts (typically sourcing a file the user already keeps
  outside deck). Recommended in help over putting tokens in `env`.
- **A hook's exports reach that session's agent and nothing else.** `pre_launch` runs in the
  pane, in the same shell that then execs the agent, so whatever it exports is inherited by
  that agent by construction — and by nothing else: not deck's own process, not another
  session, not the tmux server's environment, and not the tmux *session* environment table that
  §3.2's `-e` mirror populates for future panes. None of it is in any column of `state.db`,
  which is the whole reason this is the preferred home for a secret. That inheritance is a
  promise, not an accident of the implementation.
- **Scrollback is the one leak path, and the hook owns it.** The hook's output lands in the
  pane and §9.4 captures the pane, so a hook that echoes a secret has written it under
  `$DECK_HOME/captures/<session_id>/`. The safe shape, which help states: emit `export K=V` on
  stdout for the caller to `eval "$(…)"`, send diagnostics to stderr, and never echo a value. A
  session whose hook cannot be that careful sets `sensitive` (§8) and gives up capture.
- **A hook must be idempotent, because it runs on every launch and not once per session.**
  `pre_launch` fires on create, on `r`, on `R`, on the `r` after a `U`, and on the first `r`
  after a host restart — §9.1 restores nothing at boot, so a reboot simply leaves every row
  `stopped · resumable`. A hook that provisions an external resource therefore provisions it
  again on the next launch, which is exactly what makes §9.2's fail-open teardown tolerable:
  whatever a teardown released, the next launch rebuilds.

### 6.5 The config file

One file per profile — `$XDG_CONFIG_HOME/deck/config.toml` for default,
`$XDG_CONFIG_HOME/deck/profiles/<name>/config.toml` for a named one (§3.4) — with a declared
schema:

| where | keys |
|---|---|
| top level | `allow_yolo` (default false, §5), `yolo_default` (default false, §5 — inert unless `allow_yolo`), `stale_after` (default 45 s, §7), `capture_min_interval` (§9.4), `tmux_mouse` (default true, §3.2 — `false` restores tmux's own default and with it the arrow-key behaviour), `event_retention_days` (default 30, §12), `pre_launch` (empty by default, §6.4 — the global launch hook), `agent` (empty by default, §3.4 — the kind `deck new` creates when `--agent` is absent; it is a string stored verbatim and checked against the registered kinds only when `deck new` uses it, so a kind deck does not know fails there with `unknown agent kind`; the create dialog ignores it), `post_destroy` (empty by default, §9.2 — the global teardown hook), `event_hook` (empty by default, §10.1 — the one event script; empty makes §10 inert), `event_hook_default` (default false, §10.2), `adopt_harness_names` (default true, §11.4), `bookmarks` (default false, `DECK_BOOKMARKS`, §11.5 — the numbered session bookmarks), `update_check` (default true, `DECK_UPDATE_CHECK` — the daily check for a newer release, below), `auto_update` (default false, `DECK_AUTO_UPDATE` — true implies the check), `event_hook_events` (default `["waiting","error","ended"]`, §10.2), `event_hook_timeout` (default 3 s, §10.3) |
| `[env]` | the middle PATH/env layer (§6.1) |
| `[ui]` | `theme` (§11.6), `ascii` (§11), `mouse` (default true, §11.8), `preview_fit` (default true, §11), `preview_paint` (default `"fit"`, one of `fit`/`nofit`/`bg`/`off`, §11.3), `sort_order` (default `"attention"`, one of `attention`/`created`/`activity`/`name`, §11), `default_group_first` (default false, §11), `attach_on_new` (default true, §11), `attach_on_click` (default true, §11.8), `select_on_drag` (default true, §11.8), `attach_on_resume` (default false, §9.1), `recent_cwd_limit` (default 5, §11.7). **Not** `layout_mode`, `sidebar_width` or the recent-directory list itself — those are machine-local UI state/history and live in `state.db` (§11.2, §11.7), so a keypress never rewrites this file, `note_timeout` (whole seconds or a quoted duration, at least 1; default 10, `DECK_NOTE_TIMEOUT` in whole seconds overrides it, §11.3 — how long a transient footer note stays up), `output_bucket` (whole seconds or a quoted duration, at least 1; default 10, `DECK_OUTPUT_BUCKET` in whole seconds overrides it, §7, §11 — the width of one `last_output_at` bucket, which the output clock writes at and the `activity` sort orders by) |

The settings view (§11.5) edits this file: `,` opens it and `,` or `esc` closes it (§11.5
spells out the discard prompt and the text-entry modes where `,` is a literal character).

**Change detection.** A running TUI polls for edits to `config.toml` and to the user theme
directory (§11.6) **every 30 s at most** (`DECK_CONFIG_POLL_MS`, a test-only override that
production never sets, §13.1, is the only way to a shorter interval). Each poll computes a
*fingerprint* — `config.toml`'s mtime and size, plus the name, mtime and size of every entry
in `$XDG_CONFIG_HOME/deck/themes/` (a missing file or directory is a value of its own, so
creating or deleting either counts as a change) — and compares it with the last-seen one. An
**unchanged fingerprint never parses anything**; a changed one re-reads `config.toml` and the
theme files exactly once, then adopts the new fingerprint (an invalid file is therefore parsed
once, not on every poll, and the next write is picked up). The reload opens these files
read-only and calls no config writer: it never rewrites `config.toml`, whatever it finds there.

**The event-hook keys (§10).** `event_hook` is a string (an executable path, or an executable
and its arguments separated by spaces; no shell), `event_hook_default` a boolean,
`event_hook_events` a one-line array of quoted event kinds (`["waiting", "error", "ended"]`
by default; `[]` is a real value, no kind offered) and `event_hook_timeout` whole seconds, or a
quoted duration, of at least 1 (3 by default). Validation is at load, like every key: a word in
`event_hook_events` outside the offered set of §10.1 (`started`, `resumed`, `waiting`, `idle`,
`error`, `ended`, `killed`, `renamed`; `prompt`, `env` and `note` are not offered), a malformed array, or an
`event_hook_timeout` below 1 is an error naming the file, the line and the key. Settings
writes them back through the same atomic writer.

**Applied live, no restart.** When a poll finds a change, the running TUI re-applies every
key of the schema above that is not listed below, through the same path a save in the settings
view (§11.5) uses: `allow_yolo`, `yolo_default`, `ui.theme`, `ui.ascii`, `ui.mouse` (the
terminal's mouse reporting is switched on or off with it), `ui.preview_fit`, `ui.preview_paint`,
`ui.sort_order` (the loaded sessions are re-sorted, the selection stays on its session),
`ui.default_group_first`, `ui.attach_on_new`, `ui.attach_on_click`, `ui.select_on_drag`,
`ui.attach_on_resume`, `ui.note_timeout` (the next note follows it), `ui.output_bucket` (the output clock writes at the new width from its next read, and an `activity` list re-sorts at once), `event_hook`, `event_hook_default`, `adopt_harness_names` (the next adoption or
push follows it), `bookmarks`, `update_check`, `auto_update` (a save or reload that turns the check on runs it at once, one that turns `auto_update` on installs an offered release at once, and one that turns them off stops every later automatic request or install), `event_hook_events` and
`event_hook_timeout` (the dispatcher reads them from the running settings when an event fires,
so the next event follows them). A key the file did not change is left as it is, and a key a `DECK_*`
variable overrides stays pinned to the environment. A changed user theme file re-resolves the
active theme too, so editing its colours takes effect on the next poll without touching
`[ui] theme`.

**Which instances reload, and which profile.** A reload is a feature of the binary an instance
was started from: an instance started by a binary older than this feature never polls, so after
upgrading deck it must be restarted once (a restarted instance, and every instance started by
the new binary after it, then follows every later edit without another restart). A reload never
crosses a profile: each instance polls and re-reads only its own profile's `config.toml` (§3.4),
so a change saved in one profile's instance reaches the other instances of that same profile and
never an instance of another profile, the default profile included. Black-box scenarios
(`features/config_reload.feature`) hold this for `ui.theme` (settings view and `t` picker, default
and named profile, and the real-install layout with `DECK_HOME` unset) and for `ui.ascii`,
`ui.mouse` and `ui.sort_order`.

**Restart-required keys.** `stale_after`, `capture_min_interval`, `event_retention_days`, `tmux_mouse`, `interactive_ms`, `interactive_transport`, `pre_launch`, `agent`, `post_destroy`, `ui.recent_cwd_limit`, `[env]`
A reload refreshes only these keys' file value (so the settings view shows what is on disk); the
running process keeps using the value it started with until deck restarts, which is also what the
settings view labels *restart-to-apply* (§11.5). A new schema key is classified as one or the
other in the same change that adds it.

**A reload never gets in the way.** Four cases, each with a test:
1. *An open settings edit is not clobbered.* While the settings view (§11.5) holds an unsaved
   edit (a staged change, the discard prompt, or an open text editor), a reload is held back and
   the pending edit stays exactly as it is. When the edit is **saved**, the held reload is folded
   into the write: every key the user did not change takes the reloaded value, every key they
   changed keeps theirs, so the save neither drops the other instance's change nor the user's
   own. When the edit is **cancelled** (or reverted), the held reload is applied at once. Either
   way every held component is applied, including a changed user theme file whose configured
   name did not change (unless the saved edit chose a different theme, which then stands). A view
   that is open but untouched takes the reload and shows it.
2. *An instance's own save is not an external change.* After a save the instance records the
   `config.toml` fingerprint of what it just wrote, so the next poll finds nothing changed and
   applies nothing a second time (no flicker). This holds for every instance-local writer of
   `config.toml`: the settings view's save and the `t` theme picker's selection both record the
   fingerprint and refresh the running file snapshot. A user theme file that changed meanwhile,
   and any later external write, is still seen.
3. *A half-written or invalid `config.toml` changes nothing.* The previous settings stay in
   force, no key falls back to its default, nothing panics, and the main view shows one line,
   `config.toml not reloaded, keeping the previous settings: <error>`, above the sessions. The
   file is parsed once (the fingerprint was adopted), and the next valid write is applied and
   clears the line.
4. *UI state is not touched.* `state.db`'s `ui_state` (`layout_mode`, `sidebar_width`, the
   collapsed groups) is neither read nor written by a reload; those are per-install state, not
   configuration.

Environment always outranks the file: `DECK_ASCII` set in the environment overrides
`[ui] ascii`, as every `DECK_*` knob overrides its file counterpart (§13.1 depends on
this — the harness must be able to pin behaviour regardless of what a config file says).

**The two hook keys are global defaults that compose with a session's own, never replace it.**
A global hook is the *self-selecting* rule — one line, installed once, that reads §6.1's
session context and decides for itself whether this session is one it cares about — and a
session's own hook is the specific exception. Both run, **global first**, joined so that a
failure of the first short-circuits the second (§9.1's fail-closed launch for `pre_launch`).
Global-first is also what lets a session's hook observe whatever the global one exported, in
the same shell, so the two compose instead of racing. The presence of a per-session hook never
shadows the global one. Both keys are editable in settings (§11.5) like every other flat key,
and labelled *restart-to-apply* there, because a hook is consumed at launch.

**The update check (R297) is deck's only network use.** With `update_check` on (the default), deck
asks the release server (`https://github.com/n-orlov/deck`, or `DECK_UPDATE_URL`, §13.1) for the
latest release tag on its own goroutine, never the UI's, and at most once every 24 hours across all
profiles and all running instances: the answer, a failure included, is stored in the shared
`update-check.json` (§4), and a check inside that window makes no request. The read-the-cache, decide, request and record-the-answer steps are one operation under an exclusive advisory file lock (`update-check.json.lock`, beside the cache), so instances and profiles that start together make one request between them, not one each; a check waiting on the lock reads the answer the holder recorded. The request is the plain
`GET <base>/releases/latest` and nothing more: deck sends no telemetry, no identifier, no session,
path or configuration data, and the only header is the standard HTTP client one. A failed or offline
check shows no indicator, returns no error to the user and is not retried inside the window. Setting
`update_check = false` (or `DECK_UPDATE_CHECK=0`) disables it: deck then makes no outbound request
at all. `auto_update` implies the check, so the check runs when either key is on. Both keys are
read from the running settings each time a check is requested or an answer is applied, never from a
copy taken at start-up. A settings save or an external reload that turns the check on (`update_check`,
or `auto_update`, which implies it) runs the daily check at once, without a restart, still bound by
the 24-hour cache; one that turns `auto_update` on while a newer release is on offer installs it at
once; one that turns the check off makes no later request, and an answer already in flight is shown
but never installed while `auto_update` is off. A key a `DECK_*` variable overrides stays pinned.
The explicit `deck update` verb (§3.4, §13.1) is the one other request and runs only when asked.

**Auto-update (R300).** With `auto_update = true` (or `DECK_AUTO_UPDATE=1`), a check that finds a
newer stable release installs it at once, with no prompt, through the same verified update `deck
update` and `ctrl+u` run (§13.1: checksum, then build attestation, then an atomic replace): the
indicator reads `updating to vX.Y.Z…` while it runs and Installed (§11) when it ends. deck never
restarts itself and never touches sessions; the new binary runs at the next start. A failed
install leaves the binary byte-for-byte as it was, never a partial file, and the indicator reads
Failed with the cause. A running deck attempts the install once, at its start-up check (or when a live change first turns the check or `auto_update` on), and never
retries it while it runs; a later start inside the 24-hour window reads the cached answer (the
check itself makes no request) and, while that answer still names a newer release, attempts the install again.
Where the update would be refused (an install directory deck cannot write, a
package-managed prefix, a `dev` build) deck downloads no archive: the unwritable and managed
cases show their reason in the indicator, and a `dev` build, which is never outdated, shows
Skipped with its reason (`auto-update skipped: vX.Y.Z — dev build, install a release with
install.sh`). With `auto_update` off, deck downloads no archive on its own: a newer release is
only offered (Available, `ctrl+u`), and a `dev` build shows nothing.

**Terminal-safe update errors (R330).** Text a release server or `gh` sent back (a redirect path, an
HTTP reason phrase, a `checksums.txt` field, `gh` stderr) can hold terminal control characters. Every
C0 (U+0000-U+001F, newline included), DEL (U+007F) and C1 (U+0080-U+009F) character is removed from it,
a C1 control sent as one raw byte 0x80-0x9F outside a valid UTF-8 sequence (0x9B is CSI to an 8-bit
terminal) as well as one UTF-8 encoded, before it reaches the footer's `update failed` note, the `ctrl+u` confirmation (target and current
version) or `deck update`'s stderr (`update.StripControl`); printable and multi-byte UTF-8 text stays.
The removal runs before the credential redaction, so a secret an echo split with a control character
is whole again and is still masked. `TestStripControl`, `TestServerTextErrorsCarryNoControlCharacters`
and `TestRedactionStillAppliesToSanitisedText` (`internal/update`), `TestUpdateFailedTextStripsControlCharacters`,
`TestCtrlUDialogStripsControlCharacters` and `TestUpdateFailedTextRedactsSanitisedText` (`internal/tui`)
and `TestUpdateVerbStderrStripsControlCharacters` (`cmd/deck`) cover ESC, BEL, a C1 CSI byte (raw and UTF-8 encoded), an OSC
title sequence, a newline, a normal string and a multi-byte string at those sinks.

**Live sessions across an update (R301).** An update never disturbs a running agent. The atomic
replace makes the next `_hook` run any session's agent starts the new binary, with the session's
status and the state database unchanged (`PRAGMA integrity_check` stays `ok`) while an older TUI
keeps running. In the other direction, a newer TUI over sessions whose agents still run the older
binary's hook command is served by the existing mechanisms alone: the older hook meets a newer
schema, re-execs the `writer_binary` the TUI recorded (§3.1), and the new version lands the
update. No further mechanism is involved. `TestUpdateOldTUINewHooks` and
`TestUpdateNewTUIOldHooks` in `cmd/deck/` exercise both directions with real binaries.

Two rules that hold for every key, present and future:

- **The schema is the single source of truth** for parsing the file *and* for generating the
  settings view (§11.5). A key that exists in one and not the other is a defect; this is
  what keeps R7's "no capability is file-only" true as configuration grows.
- **An unknown key is ignored, not rejected**, so a config written by a newer deck still
  loads in an older one — but an unparseable *value* for a known key is a stated error
  naming the file and line, never a silent fallback to the default.

---

## 7. Status model

This is the single authoritative state machine. §8.1 and §9.1 defer to it.

Transitions, exhaustively. This table and the status table under it **are** the machine; no
other section restates a transition, they only refer here:

| from | trigger | to |
|---|---|---|
| `stopped` | `r` — start or resume (§9.1) | `starting` |
| `starting` | first agent signal: `UserPromptSubmit` or a working indicator the probe sees | `running` |
| `starting` | `SessionStart` (startup, `--resume`, `--continue`, `clear`, `compact`; Claude, Codex, Copilot) | `idle` |
| `idle` | `SessionStart` | stays `idle` |
| `starting` | no signal for `stale_after` | stays `starting`, becomes probe-eligible |
| `starting` | pane is alive, **`shell` rows only** | `running` — see the shell rule below |
| `running` | needs input: a `permission_prompt` or `elicitation_dialog` notification, or a prompt or question the probe sees | `waiting` |
| *any* | Claude `idle_prompt` notification (R308) | unchanged — recorded as an event, never applied |
| `running` | turn finished cleanly | `idle` |
| `running` | turn or API failure | `error` |
| `waiting` | answered: probe verdict or a later hook (attaching, clicking and previewing never answer it, R320) | `running` |
| `idle` | new prompt | `running` |
| `error` | new prompt, or a retry that succeeds | `running` |
| *any* | pane exit 0 · `x` kill · session-end hook | `stopped` |
| *any* | pane exit ≠ 0 | `error`, with `pane_exit_status` and a crash tail |

The return edges are load-bearing, not decoration: answering a prompt (`waiting → running`),
sending a new prompt after a turn (`idle → running`), and recovering from a transient
failure (`error → running`) are the product's core loop. `archived` is absent from the table
on purpose — it is a flag (§4), orthogonal to status, settable only on a `stopped` session and
clearable by unarchiving (§9.2).

| status | meaning | UI |
|---|---|---|
| `starting` | tmux session created, no agent signal yet | dim |
| `running` | agent is working | normal |
| `waiting` | **needs me** — permission prompt, question | bright, in the attention count, sorted first, seen or not |
| `idle` | turn finished cleanly | check, `last_message` in the detail pane |
| `error` | turn or process died | red, unseen marker sticks until seen, crash tail captured; needs attention only while unseen (§7) |
| `stopped` | record alive, no tmux session (post-reboot, killed) | grey, labelled *resumable* |
| `archived` | hidden from the default list, retained; **not startable** — unarchive (`U`) to resume | hidden behind a filter |

Rules:
- **`SessionStart` lands on `idle` (R307).** For Claude, Codex and Copilot, a `SessionStart` hook
  (`sessionStart` for Copilot) — startup, `--resume`, `--continue`, `clear` or `compact` — sets `idle`,
  never `running`. Only a `UserPromptSubmit` (`userPromptSubmitted`) hook, or a working indicator the
  probe sees, enters `running`. A resumed session that has been given no prompt therefore stays
  `idle`: a probe tick that matches no rule leaves it untouched (`idle` is probe-eligible only at the sparse cadence below). Pi's
  own `session_start` extension event keeps `running` (§8.3).
  A `SessionStart` never revives a process-crash row (an `error` carrying a pane exit status), the
  same veto a `running` hook has. Copilot fires `userPromptSubmitted` and then `sessionStart` on its
  first prompt, so that first turn reads `idle` until the next hook or a probe that sees the working
  indicator; the next prompt's `userPromptSubmitted` enters `running`.
- **Idle probe cadence (R314).** An `idle` row is probe-eligible once its `status_at` is at least
  **4 × `stale_after`** old (`starting`, `running` and `waiting` stay at 1 × `stale_after`). The multiple is
  fixed, not a setting, and it applies only on the TUI's probing path, never in `deck _hook`. This is the
  sparse check for a turn begun without a hook: a probe that sees the working indicator moves the row to
  `running` with source `probe`; a probe that matches nothing leaves it `idle` (`last_probe_at` only). An
  `error` or `stopped` row is never probed.
- **Unverified running row (R315).** A `running` row with no event for at least **3 × `stale_after`**
  and whose probe has matched no rule on **3 consecutive** passes is shown as *unverified*: its status
  glyph is `◔` (ASCII `-`) instead of `◐` (ASCII `~`), in the same one column (§11). The miss streak is the
  column `sessions.probe_miss_streak` (schema v16): `RecordProbeMiss` adds one with a plain `UPDATE`, never
  an event, and any probe verdict (even one that lost precedence), any hook verdict and any status write
  sets it back to 0, which clears the marker at once. "No event" is measured from the newest `events.at`
  recorded for the session, of any kind and from any writer (the newer of it and `status_at` is used), read
  at render time through the `events_session_at` index (schema v17) and stored on no row: a repeat verdict
  that leaves `status_at` alone (R303) still restarts the 3 × `stale_after` window. The marker is derived at render time and stored
  nowhere: the status stays `running`, the status word reads `running`, no text is added, and the row keeps
  its sort position in every order. It never applies with `stale_after` 0 (probing off) and never to a
  `starting`, `waiting`, `idle`, `error` or `stopped` row.
- **Movement fallback (R316).** An `idle` row whose transcript region (§4, the `last_output_at` clock) changed in
  **two consecutive output buckets** becomes `running` (source `movement`, reason `output movement`); a
  counted bucket is one the clock wrote `last_output_at` for, so footer-only churn and deck-caused output
  (R305) never count, and one counted bucket alone, or two with a quiet bucket between them, enters nothing.
  It leaves with **one quiet bucket**: once the bucket after the newest counted one has passed without a
  counted write, a `running` row the fallback itself set returns to `idle`. It never ends a `running` row
  any other source set, and only moves `idle` to `running` and back. Pi has no hook verdict to defer to and
  uses movement directly. Claude, Codex and Copilot use it only as the stale fallback: the store refuses a
  movement write over a `hook` verdict younger than `stale_after`, so a fresh hook always wins and a stale
  one does not. The streak lives in memory beside the clock (a restart starts from a baseline), the write
  is an ordinary status write with an event (`movement.running`, `movement.idle`), and it runs only on the
  TUI's probing path with `stale_after` above 0. The pass that writes it takes no probe of that row. Its
  quality badge reads `sampled`, like a probe.
- **Precedence:** `user-terminal` > `hook` > `probe` > `tmux`. A user kill sets
  `killed_by_user`, which an in-flight hook arriving milliseconds later cannot undo —
  explicit human action outranks automation. Below that, a probe never overwrites a fresher
  hook verdict, and `tmux` only ever supplies liveness.
- A transition into `waiting` or `error` leaves the row unseen (`seen_status_at` reset to 0, so it
  differs from the new `status_at`); a row is seen exactly when `seen_status_at = status_at`.
  `Y` sets `seen_status_at = status_at`; engagement (R321) does too, and nothing else does. **Attach, click
  and passive preview are inert (R320):** they never change `status` or the seen state, write no `attached`
  event and no `notify_epoch` bump, and the row does not move, in the `attention` order or the `activity`
  order. This holds for every way the keyboard reaches the pane through deck: `a`'s full attach and the
  interactive preview (§11.9), whether entered by `↵`, forced by `F`, reached by a sidebar or preview click,
  or entered by `[ui] attach_on_new` / `attach_on_resume`. `Y` is the way to mark a row seen without
  engaging, so it applies only where there is something unseen: on
  a row already seen it does nothing at all — no write, no event, no message —
  and §11.3's footer does not offer it there. Being
  *displaced* from the preview by another client records nothing either. Covering tests:
  `TestAttachIsInertOnStatusSeenAndPosition` (eight cases: `waiting`, `error`, `running`, `idle` under the
  `attention` and `activity` orders), `TestClickIsInertOnStatusSeenAndPosition` and
  `TestPassivePreviewIsInertOnStatusSeenAndPosition`.
  **Engagement sets seen (R321).** On an unseen `waiting` or `error` row, in `a`'s full attach, in the
  interactive preview or over the passive preview, the first of these sets `seen_status_at = status_at`
  through one function (`Model.engageRow`, one `Store.MarkStatusSeen` write): the first key forwarded to
  the pane, the first scroll, and the first change of the pane's screen content. In the interactive and
  passive preview deck sees them itself: a forwarded key; the wheel over the interactive preview, a page
  scroll, or a wheel notch over the *passive* preview (which still scrolls nothing, §11.8); and a change of
  the interactive preview's live screen since entry (compared on the preview tick). In a full attach the
  real tmux client draws the terminal, but deck stays in its key path: the client runs on a pty deck owns,
  and deck copies the user's terminal (put in raw mode, its size and resizes passed on) to it byte for
  byte. Beside it a watcher samples tmux read-only every 200 ms (`tmux.Client.AttachSample`: pane size,
  copy mode and scroll position, the `prefix`/`prefix2` options, the attach client's key table, and the
  visible screen; the first sample is taken before the client exists) and reports the first trigger when
  the client exits. A key is a key event decoded from the bytes deck forwarded -- not a mouse report, a
  focus change or the terminal's reply to a tmux query, not the prefix key and not the key after it --
  confirmed by the client still being attached in its `root` table at the next sample, so `prefix d` and
  any other detach never count; it involves no clock, so a key in the attach's first instant counts. A
  scroll is the pane entering copy mode or moving its scroll position; a screen change is the screen's
  hash moving at an unchanged size from the pre-attach baseline, where a size change (the attach's own
  resize) and the 1 s after it only re-baseline. An attach with none of these stays inert
  (R320). Engagement sets seen only: no status, reason, source, `status_at`, event or `notify_epoch`
  changes, and no row moves. An engaged `error` row is therefore held in the sidebar tier it had before
  engagement while its `status_at` is the one engagement saw (`attentionSortRank`); the count and `space`
  read the seen predicate as it is, so it leaves them at once, and it takes the seen-`error` tier only
  when it is seen with no engagement hold (`Y`, a later status, a restart). The write names the
  `status_at` it saw, so a newer prompt is never hidden by an engagement with an older one, and a second
  engagement on an already-seen row issues no write at all. Covering tests:
  `TestEngagementFirstForwardedKeySetsSeen`, `TestEngagementFirstInteractiveScrollSetsSeen`,
  `TestEngagementFirstPassivePreviewScrollSetsSeen`, `TestEngagementFirstScreenContentChangeSetsSeen`,
  `TestFullAttachFirstForwardedKeyIsReported`, `TestFullAttachFirstScrollIsReported`,
  `TestFullAttachFirstScreenContentChangeIsReported`, `TestFullAttachTriggersSetSeenThroughEngageRow`,
  `TestFullAttachWithoutEngagementReportsNothing`, `TestFullAttachDetachByKeyReportsNothing`,
  `TestAttachKeyProxyCountsOnlyForwardedKeys`,
  `TestSecondEngagementWritesNothing`, `TestEngagementChangesNoStatusAndMovesNoRow` and
  `TestEngagementHoldEndsWithTheStatus`. Leaving an attention state bumps `notify_epoch` (§10.3).
  **Dwell sets seen (R322).** A dwell of 1.5 s (`seenDwell`) with the pane in view also sets seen, through
  the same `Model.engageRow` write, but only when the row's prompt was already on screen when the dwell
  started. A row is *in view* when it is the one the preview shows (the selected row, or the interactive
  target), it is an unseen `waiting`/`error`, the preview panel is shown (not collapsed away below its
  floor, §11.2, and no modal or the settings takeover over it), the capture on screen is its own, and the
  terminal window has not reported losing focus. *The prompt on screen* means the row's agent adapter
  probes the pane text in view (the passive capture, or the interactive grid) and reports the row's own
  status; an agent with no probe rule for it never reaches seen by dwell. The dwell is followed after
  every message, so anything that stops being true ends it -- arrowing to another row, the panel
  collapsing, the window blurring, a new `status_at` -- and the next time the row is in view a new dwell
  starts and reads its prompt-at-start afresh: time spent before arrowing past a row, or while unfocused,
  never counts. A prompt that appears mid-dwell (the pane showed no prompt at the start) stays unseen until
  an engagement (R321) sets it; the dwell only ever sets seen, so it changes no status, writes no event and
  moves no row, and a row already seen or already written for its `status_at` is not written again. The
  check runs on the preview tick (`DECK_PREVIEW_MS`), so the write lands on the first tick at or after the
  1.5 s mark. Focus is learnt from the terminal's focus reports, which `cmd/deck` requests for its own
  window (`tea.WithReportFocus`); the model consumes them and, as before, never writes a focus report into
  a pane. A terminal that sends none leaves deck focused. Covering tests:
  `TestDwellWithPromptOnScreenAtStartSetsSeen`, `TestDwellPromptAppearingMidDwellStaysUnseenUntilEngagement`,
  `TestDwellWithPaneCollapsedDoesNotSetSeen`, `TestDwellWithDeckUnfocusedDoesNotSetSeen`,
  `TestDwellArrowingPastARowDoesNotSetSeen` and `TestPreviewTickRunsTheDwellTrigger`.
- **Needs attention (R319).** A row needs attention when it is `waiting`, or `error` **and
  unseen**; seen means `seen_status_at = status_at`, an equality and never an ordering. A
  seen `waiting` row still needs attention (it is still blocked on the user): it stays in the
  top tier, in the strip's count and a `space` target. A seen `error` row does not: it is
  dropped from the count and from `space`, and sorts just above `stopped` (after `idle`).
  The `attention` sort tier, the collapsed strip's count and `space` all call that one
  predicate (`NeedsAttention`, `internal/tui/attention.go`), so the three cannot disagree. The one
  exception is R321's engagement hold (above): an `error` row engagement set seen keeps its sidebar
  position until its `status_at` changes, because engagement moves no row.
  Because the test is equality, a clock that steps back or two stamps in the same
  millisecond cannot hide a prompt: a transition into `waiting` or `error` resets
  `seen_status_at` to 0, so a later prompt is unseen whatever stamp it carries.
- **A repeat write is inert.** A status write that leaves the status unchanged — a probe
  seeing `waiting`, `running` or `error` again, a hook or the user restating the current
  status — does not touch `status_at`, `notify_epoch`, `hook_fired` or the seen state
  (`seen_status_at`); precedence decides the stored `status_source` exactly as it does for a
  transition. It updates the reason (a hook repeating the status with a different reason
  changes the reason and not `status_at`), and `last_probe_at` may still advance. The event
  is still recorded. Only a change of status dates the row, opens an attention episode and
  marks it unseen, so a row cleared with `Y` and then re-probed `waiting` stays cleared and
  keeps its position in the `attention` sort order (which orders by `status_at`); the
  `activity` order reads `last_output_at` (§7, §11) and no status write can move it. A hook repeating the current status therefore does not extend a hook verdict's
  freshness window: staleness (§7, below) is measured from the last change of status.
- **Attaching never answers a `waiting` row (R320).** The old rule that attaching moved a `waiting` row to
  `running` is removed: looking at a session is not evidence that the prompt was answered. A `waiting` row
  leaves `waiting` on evidence only: a later hook, or a probe verdict (including the staleness rule below).
- Staleness: a `starting`, `running` **or `waiting`** session with no event for
  `stale_after` (default 45 s) becomes eligible for probing. For non-hook agents, probing
  is the only source. `waiting` is included for the unattended half of the same hole: a
  prompt answered from a raw `tmux attach` (not through deck) fires no hook either, so the
  probe is what corrects it. This does not violate precedence — by the time a `waiting` row
  is probe-eligible, the hook verdict it would override is at least `stale_after` old, and
  "a probe never overwrites a *fresher* hook verdict" is unchanged.
- **Copilot rows.** A `copilot` row's hook statuses are the table in §8.4: `userPromptSubmitted`
  → `running` (`prompt`), `sessionStart` → `idle` (the payload's `source`), `notification`
  of type `permission_prompt` or `elicitation_dialog` → `waiting` (that type), `agentStop` →
  `idle` (`end_turn` or the payload's `stopReason`), `sessionEnd` → `stopped` (the payload's
  `reason`). `errorOccurred` and a `notification` of any other type change nothing: errors are
  recoverable and repeat, so only the pane probe's `✗` line sets `error`. **The `starting`
  window.** Copilot fires `sessionStart` at the first prompt, never at launch, so a copilot
  row is `starting` from launch until its first hook or probe verdict, and that is not an
  error: it stays `starting` and probe-eligible after `stale_after` like any other row.
- **Copilot abort demotion.** Interrupting a Copilot turn with Ctrl-C fires no hook (no
  `agentStop` follows an abort), so a row `userPromptSubmitted` left `running` would stay
  `running` while the pane shows the idle footer. Nothing new corrects it: the ordinary probe
  does, under the rules above and with the same `stale_after` window every other hook kind
  uses (no copilot-specific constant). Once the row's hook verdict is at least `stale_after`
  old and the pane shows the idle footer, the probe records `idle` (`ready`); while the pane
  shows the `Working` footer the row stays `running`, and a hook verdict younger than the window
  is never probed over.
- **Claude idle probe (R310).** Claude probes `idle` (reason `idle footer`) when the permission-mode footer
  `⏵⏵ … (shift+tab to cycle)` is present and the `esc to interrupt` working indicator is absent, whatever the
  composer holds: an unsent draft does not change status (`Claude.PromptReady` is what detects drafts). The old
  `Claude Code` + `❯` banner rule is gone, since the banner scrolls away. Goldens in
  `internal/agent/testdata/probes/claude/` (a scrolled pane with an empty composer, the same with a draft, the
  alternate screen with no scrollback, and an `esc to interrupt` pane) reproduce observed pane shapes and are not
  byte captures; `claude-PROVENANCE.md` says so. **Verification:** the bypass-permissions footer text
  (`⏵⏵ bypass permissions on (shift+tab to cycle)`) is taken from the phase 4t PRD and is the only mode text a
  golden carries. The default-mode footer text was **not verified**: no repo evidence of it exists, so no golden
  covers it, and the rule matches on the `⏵⏵` and `(shift+tab to cycle)` markers rather than a mode string.
- **Claude live region (R311).** For Claude, `API Error:` (`error`, reason `api error`) and the permission-dialog
  markers (`Do you want to proceed?` with `Yes`, `waiting`, reason `permission prompt`) count only in the **live
  region** of the capture: from the last transcript line down to the composer, composer excluded. A transcript
  line starts with `●` (assistant message or tool call), `⎿` (tool result) or the prompt glyph `❯`/`>` not followed
  by a dialog option number (an echoed user turn); the composer is the chrome below the last two separator rules,
  or, on a pane with no rules, a bare trailing prompt line. A marker above the last transcript line has been
  followed by newer output, so it is history: an old `API Error:` above a live composer is not `error`, and an
  answered `Do you want to proceed?` above a live composer is not `waiting`. A live dialog is `waiting` and a live
  error is `error`. A pane with no transcript line is live throughout. Only these two rules are anchored; the
  working indicator and the idle footer are read from the whole capture as before.
- **Claude waiting shapes (R312, probe half of R309).** The Claude probe reads these dialog markers as `waiting`,
  each in the live region (R311) and each with `Yes` on screen; the marker is the first text, the reason follows,
  and the golden under `internal/agent/testdata/probes/claude/` is named:
  `Do you want to proceed?` (`permission prompt`, `waiting.txt`);
  `Do you want to make this edit` (`file edit confirmation`, `waiting-edit.txt`);
  `Do you want to create` (`file create confirmation`, `waiting-create.txt`);
  `Do you trust the files in this folder` (`folder trust`, `waiting-trust.txt`).
  The edit, create and trust markers are the ones `claude_gate.go` already lists; the goldens are hand-written
  reproductions, not captures (`claude-PROVENANCE.md`). **Petitioned, no golden and no rule:** an
  **AskUserQuestion** choice list (question/elicitation), **plan approval**, and a **write**-file confirmation
  wording. The repository holds no capture, fixture, report or code that states any of their texts (searched: the
  Claude and Copilot probe corpora, `claude_gate.go`, `docs/reports/`, `docs/spikes/`, `prds/`, `cmd/fake-*`,
  and the phase 2 real-Claude logs), so none is guessed; until one is established the hook side
  (`permission_prompt`, `elicitation_dialog`) is the only source for those shapes. The idle, running, starting
  and error goldens never probe `waiting`.
- **Codex terminal error is anchored (R313).** Codex's `■` error rule (`error`, reason `terminal error`) matches only
  on the **last content line**, the way pi's `Error:` is anchored: the last content block above codex's composer
  (the last `›` line within the final three non-blank lines, then its footer) must be led by a `■ ` in column 0.
  Codex indents an agent cell's and a tool result's continuation lines, and a long error line wraps with indented
  continuations, so the block's head is found by reading up over indented lines. A `■` in a markdown body, in a
  progress bar (`[■■■□□]`) or in a tool's output, followed or surrounded by more transcript, is not `error`. A
  trailing `■ unexpected status ...` line is. Goldens: `testdata/probes/codex/error.txt`.
- Probe heuristics live in one table-driven file with golden-file tests over captured pane
  text — a fixture corpus per agent, so a spinner or prompt redesign upstream is a
  one-fixture fix.
- **Liveness and clean-vs-crash exit.** `remain-on-exit failed` (§3.2) means a pane whose
  command exits **0** is destroyed with its session, while a pane exiting **non-zero**
  stays as a dead pane. So the 500 ms reconcile — **one** `list-panes -a -F` over the whole
  server, carrying each pane's session name with `pane_dead` / `pane_dead_status`, and never a
  `list-panes` per session (§2) — distinguishes the two without guessing. A server with no
  sessions, or no server at all, reads as "every session gone":

  | observation | result |
  |---|---|
  | session gone | `stopped` — clean exit, `/exit`, `exit` in a shell, or an explicit kill |
  | session present, pane dead, status ≠ 0 | `error` with `pane_exit_status`, plus a crash tail captured *before* the session is torn down |
  | session present, pane alive | keep the current status — unless the row *denies* the live pane, which is the invariant violation below |

  This is what makes a crash tail capturable at all — with the tmux default
  (`remain-on-exit off`) the pane and session vanish on death and there is nothing left to
  capture from. It also stops a shell session where the user typed `exit` from being
  reported as a red `error`.

  **A dead pane is collected on sight, never retained.** The same pass that observes it
  captures the tail, writes `error` with `pane_exit_status` and `crash_tail`, and *then* kills
  the session. So the stored tail is the only crash artifact, and `tmux -L deck ls` agrees
  with the list within one tick. Keeping the corpse around for forensics would mean two
  answers to "what did it print" — a bounded tail in the store and a full frozen scrollback on
  the socket — while also holding the session name against the next resume and leaving crashed
  sessions on the socket indefinitely. Two properties follow and both are load-bearing under
  R4: collection is **idempotent and unleased** — the tail is written once (`WHERE
  pane_exit_status IS NULL`, first writer wins) and killing an already-gone session is a
  no-op, not an error, so N clients seeing one corpse need no lease between them — and with
  no TUI running nothing collects at all, which is the unattended gap stated below rather
  than a new one.

  **A terminal row that denies a live pane is an invariant violation, and the same pass
  repairs it.** A `stopped` row — whatever wrote it — and an `error` row that carries a
  `pane_exit_status` or whose source is `tmux` or `user` all claim the process is gone, and a
  live, non-dead pane is direct evidence against that claim. So when the reconcile observes
  one under such a row it **corrects the row from what it can observe** (the liveness and
  probe rules above), records the correction as an event, and touches the pane not at all:
  the pane is the part that is right. Dead-pane collection is decided first, so a corpse is
  still collected rather than mistaken for a live contradiction.

  **A hook- or probe-sourced `error` with no `pane_exit_status` is not a violation, and is
  never repaired.** It is this section's own transition-table row — `running → error` on a
  turn or API failure — and nothing about a failed turn implies the pane died: the pane
  being alive is what that state *describes*, not evidence against it. Repairing it would
  erase the one verdict the hook channel exists to deliver, milliseconds after it arrived
  and before anything could observe it. Nor is such a row action-wedged the way the repaired
  shapes are: kill, resume and the rest read it as the live session it is, and
  `error → running` on the next prompt or a retry that succeeds is its ordinary exit.

  This is the one self-healing rule in §7, and it earns that exception because the alternative
  is a row that every action refuses. Resume declines — nothing needs launching, a pane is
  already there; kill declines — the row says it is already stopped; and the only escape is a
  keystroke pair the user has to know is an escape. Status writes are not the only way in:
  §9.3's per-launch generation closes the out-of-order-hook path that produced this in
  practice, but deck being killed between a kill and its status write reaches the same state,
  and so does any future writer that gets the order wrong. An invariant that repairs itself is
  cheaper than every action having to tolerate its violation, and far cheaper than a user
  discovering the violation as a session that cannot be recovered.
- **A `shell` session has no agent signal, ever.** It has no hooks to fire and nothing
  meaningful to probe, so the rules above would leave it at `starting` for its entire life
  — not just until some later capability lands. For `shell` rows only, tmux liveness
  therefore promotes `starting → running`, where `running` means "the pane is alive" rather
  than "an agent is working". This is the one place `tmux` supplies more than liveness, and
  it is sound precisely because no higher-precedence source exists for a shell that could
  ever contradict it. It does **not** generalise to agent rows: inferring `running` for an
  agent from a live pane is the fabricated status §7 exists to forbid, because there the
  higher-precedence sources do exist and may disagree. One consequence in the UI: a shell
  row's `starting` label is plain `starting`, never `starting · awaiting signal` — that
  suffix names a signal a shell will never have, so it is agent-only copy.
- **Crash detection is not instantaneous when unattended.** A `SIGKILL`ed or OOM-killed
  agent fires no hook, so the transition to `error` — and its event hook — happens on the
  next TUI tick or the next non-session-end `_hook` invocation of any session on the deck
  (its liveness pass, §3.1), whichever comes first; the process that records the `error`
  dispatches its event hook (§10.4). The same holds for a clean disappearance (the tmux
  session is simply gone, recorded `stopped` as `tmux.session_gone`), which offers `ended`.
  Stated plainly rather than implied to be live. (`StopFailure` *is* a hook, so ordinary
  turn/API failures do fire the hook unattended; process death does not.)
- **Never auto-relaunch** (non-goal): a crash loop must not be able to burn tokens or retry
  a destructive action.

**A diagnostic sampling result is a column, not an event.** A probe that matches no rule
changes no status and is recorded only by overwriting `sessions.last_probe_at`, which is
what the `i` detail dialog reads. It MUST NOT append to `events`: a per-tick row with no
reader is unbounded write amplification, and at §6.5's default reconcile cadence one
unmatched session alone produces two rows a second forever. The general rule the events
table is held to: **a kind is only written if something reads that kind**, and the reader is
named where the kind is introduced.

**The output clock (`sessions.last_output_at`).** Reconcile keeps one more clock per session, and it
answers "when did the agent last print something", not "what is its status": nothing in the
precedence above reads it. tmux's `window_activity` (whole seconds) only *nominates* a moment,
because it also moves for footer and statusline churn (`ctx:%`, branch, model). Each probing
reconcile tick (the TUI's, never the hook-critical `Reconcile`) compares the window's activity bucket
(`window_activity` divided by the bucket width, `[ui] output_bucket`, 10 s by default, applied live) with the last one examined for that session. A
tick in the same bucket reads and writes nothing (except inside a discount window, below). When the bucket has advanced, deck captures the
pane, cuts it at the boundary the probe's `lastContentLine` uses, **the earlier of the last two
separator lines**, and hashes the **transcript region** above it (trailing blanks ignored). Only a
changed hash counts: it writes `last_output_at` as `window_activity` in milliseconds. A bucket
whose transcript hash did not change, such as a footer-only update, writes nothing. So two changes
inside one bucket write once (the second sees no new bucket), and a tick with no bucket change
writes nothing. The first reading of a session after deck starts is only a baseline and counts
nothing, so a restart does not make every row look freshly active; the in-memory baseline is not
persisted. The write is a plain `UPDATE` of that one column, never an `events` row (the same rule
as `last_probe_at` above), and it leaves `status`, `status_source` and `status_at` untouched.

**The unseen marker (`sessions.last_seen_at`, R324).** One marker answers "is there something here I have
not looked at". A row is **unseen** iff `last_output_at > last_seen_at`, or its status is `waiting` or
`error` and not seen per R319 (`seen_status_at != status_at`). A `running` row shows no output-unseen
marker: its output is the expected state, so only an attention-unseen status can mark it (it never has
one, because `waiting` and `error` are not `running`). `last_seen_at` is written only on a *view
change* (a view of a row whose `last_output_at` it has not yet copied) or a *bucket change* (output in a
new bucket landing under a view); a tick that changes neither writes nothing, and it is never written
per tick. A view copies `last_output_at` into it (`Store.MarkViewed`), never a wall-clock time, so a
stepped clock cannot hide or invent output. Output the clock does not count does not set the marker:
statusline-only churn writes no `last_output_at`, so it leaves a row seen. Setting or clearing the marker
never moves a row in any of the four sort orders (no comparator reads `last_seen_at`, and the marker is
not in the attention tier). The one status-seen flip that does reorder is R319's own: marking an
unseen `error` seen drops it to the seen-`error` tier. What counts as a view is R325's (§11, "What counts as viewed"). Tests:
`TestSessionUnseenPredicate`, `TestMarkViewedWritesOnlyOnAChange`,
`TestStatuslineChurnDoesNotSetTheUnseenMarker`, `TestMarkerFlipMovesNoRow{Attention,Created,Activity,Name}Order`.

**Deck-caused output is discounted (R305).** Output deck itself provokes is not the agent's activity,
so a transcript change the pane shows inside a *discount window* does not advance `last_output_at`
(the change is still hashed and remembered, so the same text never counts later). This holds in a
bucket already examined too: a tick whose `window_activity` moved inside a window captures and
re-baselines the hash even though the bucket did not advance (it writes nothing), so a deck-caused
redraw in the same bucket as the last reading is not counted by the next bucket's read. The window is
**5 seconds, and never more than one output bucket** (`min(bucket, 5 s)`), and it starts at each of:
a deck-caused resize (a preview fit that issued a `resize-window`, and interactive entry and exit,
which resize the window and make the pane redraw on SIGWINCH), a key deck forwards to the pane in
interactive mode (each forwarded key restarts the window; a key deck does not forward does not), and
an attach. An attach is open-ended: it discounts from the moment `a` hands the terminal to the
tmux client until the client detaches, plus one window for the detach repaint, because deck cannot
see what is typed meanwhile. A window is compared with the pane's `window_activity`, not with the
tick that reads it, so a tick that lands late still discounts output that happened inside the
window. Output whose `window_activity` falls after the window ends counts as before. The windows
are in memory, like the baseline.

---

## 8. Agent adapters

An adapter turns a session into **argv**, and nothing else: it never starts a process, never
writes to the store, and never reaches into the TUI. `internal/service` owns the pane and
the row; `internal/tui` consumes adapters only through this interface and the registry, so
adding an agent kind is one file plus one registry entry and no TUI change (R1).

```go
type Adapter interface {
    Kind() string                                      // claude | pi | codex | copilot | shell
    Capabilities() Caps                                // declared, never assumed
    Launch(in LaunchInput) (argv []string, err error)
    Resume(in ResumeInput) (argv []string, err error)
}

type Caps struct {
    Profiles              []string // the §5 profiles this adapter really supports
    AssignsConversationID bool     // accepts a deck-minted id at launch
    Resumable             bool     // Resume is meaningful at all
}
```

`LaunchInput`/`ResumeInput` carry the cwd, the conversation id (when deck assigns it), the
already-resolved permission profile, and the row's extra `launch_args`. They are
deliberately not store rows: `internal/agent` has no dependency on the persistence layer,
which is what keeps an adapter unit-testable as a pure function of its input.

Four further methods join the interface with the capabilities they serve. Each is asked only
of an adapter whose `Caps` claim it, so a kind that lacks one omits the behaviour rather
than faking it:

| method | serves |
|---|---|
| `Instrument(in LaunchInput) (argv []string, env map[string]string)` | per-session hook injection (§8.1) |
| `Probe(pane string) (status, reason string)` | pane-text classification where no hook exists (§7) |
| `TranscriptPaths(in) ([]string, error)` | cross-session search over transcripts (§12) |

### Session names (R249, R250)

`Caps.Name` is a `NameCaps` declaring how the agent takes, generates, accepts and reports a
session name. Its zero value claims nothing, and an adapter that declares nothing is never
asked about names:

```go
type NameCaps struct {
    LaunchName  NameLaunch // none | flag | flag_new_session_only | sendkeys_after_ready | extension
    AutoTitle   bool       // the agent generates its own title
    PushRename  NamePush   // none | sendkeys | extension
    ObserveName []string   // hook | pane_title | transcript | sqlite | yaml | extension
}
```

Two optional interfaces serve those fields, and a caller reaches them only through the
`agent.ReadName` and `agent.PushName` dispatchers, which ask an adapter only when its caps
claim the capability:

| method | asked only when | returns |
|---|---|---|
| `ReadName(in NameProbeInput) (name string, explicit bool, ok bool)` | `ObserveName` is non-empty | the agent's best current name for a bound conversation id; `explicit` says a human chose it |
| `PushName(in NamePushInput) (keys []string, ok bool)` | `PushRename` is `sendkeys` | the keystroke plan, one send-keys call per element, that renames the running agent |

An adapter that implements either method without claiming it is never called: the dispatcher
returns "no answer" and the spy records zero calls.

`LaunchInput.Name` and `ResumeInput.Name` carry the name an adapter passes to its agent; an
empty string means pass no name. `internal/service` fills them from the row only when
`name_source` is `user` and `name_adopted` is 0, and never with a blank name. A `derived` or
`harness` row therefore launches with no name flag, so the agent keeps generating its own title,
and a name a human chose inside the harness (adopted as `explicit`, frozen as `user`) is not
passed back either: only a name entered in deck reaches an argv.

#### Adopting a harness name (R251)

Adoption, and the push of the next section, run for a row only while the setting allows it: the
row's own `harness_name_adopt` override when set, otherwise the global `adopt_harness_names`
(§11.4). When it is off, neither the hook triggers nor the tick adopt, and a rename is not pushed.

deck reads the agent's own name for a row and adopts it. Adoption runs from exactly these
triggers and no others: the `Stop`, `UserPromptSubmit`, `agentStop` and `session_info_changed`
hooks (after the hook's status write has landed), and the existing reconcile tick. The tick is
the pass that already runs; adoption adds no ticker, goroutine or poll of its own. A hook of a
launch deck has replaced, and one that resolves to no row, triggers nothing. `session_info_changed`
is a hook event that carries no status: it is recorded, never applied.

Only a row whose `name_source` is `derived` or `harness` is eligible, only on a bound
conversation id, and only through `agent.ReadName` on an adapter whose caps claim it. A `user`
row is frozen: adoption never changes it. For an eligible row the outcome is one of:

| `ReadName` result | outcome |
|---|---|
| no answer, or an empty name | row unchanged |
| equal to the row's current name | row unchanged |
| a placeholder (`Claude Code`, `GitHub Copilot`, `π`) | row unchanged |
| any other name | row renamed; `renamed` event with source `harness:<kind>`; `name_source = harness` |
| the same, and another row already holds the name or its slug | the name takes the first free suffix `-2`, `-3`, ... |
| a name the harness reports as `explicit` (a human chose it inside the harness) and that differs from deck's | row renamed as above with `name_source = user`, which freezes it from then on |
| the same, but suffix resolution leaves the displayed name unchanged | no rename, no `renamed` event and the slug untouched, yet `name_source = user`: the row is frozen from then on |

A harness title is untrusted input, so `agent.SanitiseHarnessName` cleans it before any of the
checks above: terminal escape sequences (CSI, OSC and their string forms), C0 and C1 control
characters, and invisible format characters (bidi and zero-width marks U+200B-U+200F,
U+202A-U+202E, U+2066-U+2069 and U+FEFF; the word joiner and invisible operators U+2060-U+2064; the
soft hyphen U+00AD; the Arabic letter mark U+061C; the Mongolian vowel separator U+180E; the tag
characters U+E0000-U+E007F) are removed; tabs and newlines become a space and runs of
whitespace collapse; a leading glyph prefix (the Claude `✳` and spinner frames, Pi's `π - `) is
stripped; and the result is cut to 200 runes. A title that is empty after cleaning is no answer, so
nothing is adopted. The adopted name is stored as data only: it is never put in a shell command or
in a launch or resume argv (a `derived` or `harness` row launches with no name, R250, and neither
does a row frozen on an explicit harness name, R253), and adoption sends no
keystroke.

A name is taken when another row holds the same name or slug, or the same name after that cleaning
(escapes, controls and the format characters above removed, whitespace collapsed), so two names
differing only by an invisible character get a `-2` suffix.

Adoption writes the display name only. The tmux session name `deck_<slug>` is never changed. The
`name_source` check and the write share one store transaction, so a user rename that lands
meanwhile is not overwritten, and an adoption failure never fails a hook or a reconcile pass.

#### Pushing a rename (R252)

A rename in the `i` dialog (`name_source = user`, the tmux slug untouched) is also pushed into the
running agent when its adapter declares `PushRename = sendkeys`. The push is a keystroke plan from
`agent.PushName`, sent one `send-keys` call per element, in order, to the pane id resolved once from
the slug: `Enter` goes as a tmux key name and every other element, a control byte included, as
literal text. A shell, an adapter that does not claim `sendkeys` and a name the adapter declines
send nothing.

A rename destined for a harness (a non-shell row whose adapter claims a push, adoption on, running or
stopped) resolves a name or slug another row holds the way an adopted title does: the first free
`-2`, `-3`, ... suffix instead of a refusal. The resolved name is what is stored and pushed; the slug,
and so the tmux session, is unchanged. A shell row, or a row with adoption off, keeps the plain
"already exists" refusal.

The name in the plan is the row's name cleaned per harness by `agent.SanitisePushName`: control
characters and escape sequences are removed first; then Copilot takes 1-100 runes and no `"` (a
`"` becomes `'`, and the name is cut to 100 runes), Claude is cut to 200 runes, and any other kind
to 200. A name that is empty after cleaning is refused (`harness_rename_refused`): a blank name is
never passed to a harness. The echo check compares the cleaned name.

The plan is sent only when the row's status is `idle` or `running` and no harness dialog is focused.
An adapter that can recognise its own dialogs (a permission prompt, a trust screen, a picker)
implements `agent.DialogDetector`, and deck reads the pane's visible screen to ask it. Any other
status, or a focused dialog, is refused: deck sends no keystroke, records a `harness_rename_refused`
event whose reason names why, and shows a footer note with the same reason while the row stays
selected, until the next list key.

| status | push |
|---|---|
| `idle`, `running`, no dialog focused | the plan is sent |
| `waiting` | refused: the harness is waiting for an answer |
| `starting` | refused: the harness is still starting |
| `error` | refused: the harness is in an error state |
| a harness dialog is focused | refused: the dialog would swallow the keystrokes |
| `stopped` | nothing is sent and nothing is logged; the name goes in at the next launch |

The table holds for every sendkeys adapter, Codex included. The one exception is not a push: the
readiness-gated startup naming of a fresh Codex row (§8.2) types into a `starting` row once its prompt is ready.

The rename itself always stands; a refused or failed push never fails it.

After a send, an adapter that claims an observation source has the pushed name held in
`pending_harness_name`, stamped with the send time. While a name is pending, the hook and tick
observe path does not adopt anything for that row. When the observed harness name equals the pending
name, the push is confirmed: the field clears, and the echo is not adopted, so `name_source` and
`name` are unchanged by it. When a pending name stays unconfirmed for 30 seconds, the push failed:
deck records exactly one `harness_rename_failed` event, clears the field, and never sends the
keystrokes again. A plan that cannot be sent is the same failure at once: one `harness_rename_failed`
event, one attempt. A row that stops while a name is pending drops it silently.

Confirmation does not wait for a conversation id. A row with none yet (Codex mints its id at the
first prompt, so a startup name is pending before any exists) is still observed: the adapter's
`ReadName` refuses every source that needs a valid bound id (the thread database, a transcript) and
answers only from the id-free ones, so Codex's own pane title `<name> | <cwd base>` confirms the
pending name before the first prompt. The clear is the same (no failed event, no retry); an
unconfirmed name still times out as above.

| | Claude Code | Pi / oh-my-pi | Codex CLI | Copilot CLI | shell (bash/zsh/fish) |
|---|---|---|---|---|---|
| **conversation id** | **deck assigns**: `--session-id <uuid>` | **deck assigns**: `--session-id <id>` (created if missing), plus a display name | agent mints it; deck adopts it from the first hook (§8.2) | **deck assigns**: `--session-id <uuid>` (a UUID; any other id is refused by `Launch`) | none |
| **resume** | `--resume <uuid>` (fork = new id, offered explicitly); `--session-id <uuid>` while the conversation has no transcript yet (§9.1) | `--session-id <id>` | `resume <id>` by id | `--session-id <uuid>`, the launch argv again (§8.4) | recreate shell (§9.1) |
| **id discovery** | not needed | not needed | **§8.2** — `SessionStart` reports it; no filesystem search, no lease | not needed | n/a |
| **status** | **hooks → `deck _hook`** (live) | **extension → `deck _hook`** (live), probe until its first event (§8.3) | **hooks → `deck _hook`** (live), probe until the first prompt (§8.2) | **hooks → `deck _hook`** (live, §8.4); the pane probe (§7) covers the window before the first prompt, aborted turns, error lines and degraded launches (no `--plugin-dir`) | probe (sampled) |
| **banned** | `--continue` | `--continue` | `resume --last` | `--continue`, `--resume`, `--connect`, `--remote`, `--acp` | — |

**A conversation id is one path component.** A conversation id reaches deck in a hook payload, so
it is untrusted. Claude's, Codex's, Copilot's and Pi's `TranscriptPaths` decline (`ok = false`, no file
opened) for an id that is empty, is `.` or `..`, or contains a path separator (`/` or `\`); only
an id that is a single plain path component is joined into a transcript path, and a valid id
resolves to the same path as ever. Claude's relaunch decision (§9.1) treats such an id as
unknowable and resumes, never starting it again.

### 8.1 Claude instrumentation

Hooks are injected **per session** via the settings-on-the-command-line mechanism. Nothing
is written to the user's global settings file: a deck session is instrumented, an ordinary
agent run in the same directory is untouched.

Precisely: hook entries **merge** across settings sources rather than overriding them, so
`deck _hook` is *added* to whatever the user's and the project's settings already define.
Two consequences to design for rather than discover: a project's own hooks fire inside deck
sessions too, and on the session-end path they share the same budget `_hook` is racing —
which is one more reason that path never waits for the event hook (§10.3).

**Launch name (R254).** Claude declares `NameCaps.LaunchName=flag`. A row whose `name_source`
is `user` and whose name is not blank launches with `--name=<name>`: on the first launch
(`--session-id`), on `--resume`, and on the fresh `--session-id` relaunch of a conversation
that has no transcript yet (R207). Each carries the flag exactly once, after the permission
flags and before the row's `launch_args`. The name is one attached argv element (R268), so a
name that begins with `-` (`-rm`) is never a flag of its own, and it passes through the same
sanitiser as Pi's and Copilot's (control characters and escape sequences go, whitespace
collapses, it is trimmed and cut to 200 runes); a name with nothing left passes no flag. A
`derived` or `harness` row passes no name flag, so Claude keeps its own auto-title. `Launch`
and `Resume` refuse, with a deck-side error that names the flag and no argv, a `launch_args`
entry that is `-n`, `--name`, `--name=<x>`, `-n=<x>` or the attached short form `-nFOO`: deck
owns the name Claude is given. Every other extra argument (`-p foo`, `--x`, one that holds a
newline) is appended unchanged, each as the one argv element it was.

**Reading Claude's name (R255).** Claude declares `NameCaps.AutoTitle=true` and
`ObserveName=[hook, transcript, pane_title]`. A trigger hook (`Stop`, `UserPromptSubmit`) is read
in this order, and the first source that answers wins:

1. The hook's `session_title`, an optional payload field (a payload without it behaves exactly as
   before). A non-empty one that differs from the row's name, after sanitising, and is not a
   placeholder is adopted as a name a human chose: `name_source = user`, which freezes the row. Only
   a `derived` or `harness` row takes it; a `user` row is never changed.
2. The tail of the transcript: the hook's `transcript_path`, or, when the hook named none (the
   reconcile tick), the conversation's own file under `<claude config dir>/projects/<cwd>/<id>.jsonl`.
   The hook's path is used only when, cleaned and symlink-resolved, it is an existing regular `.jsonl`
   file inside `<claude config dir>/projects` (the session's `CLAUDE_CONFIG_DIR`, else `~/.claude`): a
   path outside that root, a `..` escape, a symlink (or a symlinked directory) that resolves outside it,
   a path that is not a regular file, and a path that does not exist are all ignored, and the
   conversation's own derived file is read instead (R271, R279). The reader
   refuses a FIFO, socket, device or symlink before reading: it checks the path with `lstat`, opens
   it `O_NONBLOCK|O_NOFOLLOW` and checks the opened file again, so a FIFO with no writer cannot
   hang the read (R270). It opens the file read-only and reads from the end, at most 512 KiB; when it starts
   mid-file the cut first line is dropped, and a last line that is not a complete JSON object (a
   write in progress) is skipped. It understands three record shapes, `custom-title`
   (`customTitle`), `agent-name` (`agentName`) and `ai-title` (`aiTitle`), and the last such record
   in the tail wins. `custom-title` and `agent-name` are names a human chose (`explicit`, so
   `name_source = user`); an `ai-title` is not (`name_source = harness`).
3. The pane title, read with its own `display-message -p -t =deck_<slug>: #{pane_title}` call per
   pane and only when the transcript gave no title (`paneFactsFormat` never carries it, since a
   title is free text). Claude puts an activity glyph and a space in front of its title (`✳ `, or a
   braille spinner frame); the glyph is stripped, a title without one is not Claude's (tmux shows the
   host name until Claude sets its own) and `Claude Code` is the placeholder, so both are no name.

`~/.claude/sessions/<pid>.json` is never opened: it holds an auto-title Claude rewrites and is not a
source. A title from any source is sanitised as above before it is compared or stored.

**Pushing a rename into Claude (R256).** Claude declares `NameCaps.PushRename = sendkeys` and its
`PushName` returns a three-element keystroke plan, one `send-keys` call per element, in this order:

1. `Ctrl-U` (the byte `0x15`, typed as literal text), which clears the input line so a half-typed
   prompt is not glued onto the command;
2. `/rename <name>`, the command and the name as one literal text delivery;
3. `Enter`, as a lone tmux key name in a delivery of its own, never folded into the text, so Claude
   has the command on its input line before it is submitted.

The name is cleaned for Claude before it is typed: control characters and escape sequences are
removed (so a newline can never submit the command early) and it is cut to 200 runes. `PushName`
applies this itself as well as the service, and declines (`ok = false`) a name with nothing left, so
a blank name is never typed. The generic gate (status `idle` or `running`, no focused dialog, the echo
check and the 30-second confirmation) is the one in "Pushing a rename" above.

**Claude's push gate (R267).** Claude implements `agent.DialogDetector` and `agent.PromptGate`, so the
push reads the pane's visible screen and sends the plan only when the composer is ready and no dialog
is focused, whatever status the row stores (a stored `running` or `idle` can be stale while a
permission prompt is up). A dialog is focused when the screen shows a permission or trust prompt
(`Do you want to proceed?`, `Do you trust the files in this folder`, `Esc to cancel`), a numbered option
under the selection cursor, or a pane the probe classifies as waiting. The composer is ready when the
last line that starts with the prompt glyph (`❯` or `>`, after any box border) holds nothing, or only
Claude's `Try "..."` placeholder: earlier such lines are echoed turns, a draft after the glyph is
unsent text, and a screen with no such line (startup) has no composer. Because the plan opens with
`Ctrl-U`, this keeps a user's half-typed prompt from being discarded. A refused push sends no key,
records one `harness_rename_refused` event naming why, and shows the footer note, as for any other
refusal.

Subscribed events — a handful per turn, never per tool call:

| event | → status | notes |
|---|---|---|
| session start | `idle` | confirms the assigned id; the source field distinguishes fresh vs resumed vs compacted. **`SessionStart` lands on `idle`, never `running` (R307)**, including `--resume` and `--continue`: a session that has started has not been asked anything. The `UserPromptSubmit` below, or a working indicator the probe sees, is what enters `running` |
| user prompt submitted | `running` | also feeds prompt count; **enters `running`** |
| notification, `notification_type` `permission_prompt` or `elicitation_dialog` | `waiting` | **the golden signal**; the notification type becomes `status_reason` |
| notification, `notification_type` `idle_prompt` | no change | **R308**: it only says the agent has been idle a while. It writes no status, no `status_at`, no `seen_status_at` or `notify_epoch` change and no attention transition on an idle, a running or a waiting row, and `Stop` then `idle_prompt` leaves the row `idle`. No `idle_since` value is stamped: the notification is recorded as a `notification` event (reason `idle_prompt`) and nothing else |
| notification, any other `notification_type` (`auth_success`, a type this deck build has not seen, or none) | no change | **R309 hook half**: recorded as a `notification` event, never applied. Only the two types above are `waiting`, the same two Copilot's `notification` maps (§8.4). The probe-side producers (permission, question, plan approval, trust prompt) are §7's probe rules |
| stop | `idle` | carries the last assistant message — use it; the transcript file lags |
| stop-failure | `error` | error type becomes `status_reason` |
| session end | `stopped` | **Session-end hooks share a ~1.5 s total budget**, raisable to ~60 s only by declaring a longer per-hook timeout. deck declares a modest one and still does one SQLite write, dispatches the `ended` event hook detached (§10.3: started and left running, no timeout and no recorded result) and exits — a script's latency is the user's to bound, so it never holds the agent's exit. |

`deck _hook` resolves the session by `conversation_id`, falling back to the tmux pane
identity in its environment. Unresolvable events are stored as orphans, never dropped
silently. Budget: read stdin, one `UPDATE`, one `INSERT`, dispatch the event hook (§10.4, bounded by `event_hook_timeout`, detached on session end), exit.

The event set, the payload fields (`session_id`, `cwd`, `transcript_path`,
`permission_mode`), the enumerated notification types, the last-assistant-message field on
stop and the stop-failure event are all upstream contracts, not deck's. They are re-verified
by `@real-agents` (§13.5) rather than trusted indefinitely, and every one of them has a probe
fallback (§7) — so an upstream change degrades a row from live to sampled instead of
breaking it.

Codex's own event source is real, verified and specified in §8.2, and Pi's (an extension
API) in §8.3.

### 8.2 Codex instrumentation

Codex mints its own conversation id, which is why this subsection used to specify id
*discovery*: a serialised, claim-based search for a transcript written after launch, because
R2 makes the naive rule unsound — with two Codex sessions started in the *same* directory
seconds apart, "a transcript created after launch whose cwd matches" matches both candidates
for both sessions, which is the banned "most recent" rule wearing a cwd filter.

**That machinery is not built, and is not going to be.** Verified against codex-cli 0.154.0,
codex's own `SessionStart` hook payload carries `session_id`, `transcript_path` and `cwd`, so
the id arrives *from the agent*, already addressed to the pane deck launched — the ambiguity
never arises, so nothing has to be serialised, claimed or adjudicated. `DiscoverID` is
therefore not part of the adapter interface and `internal/agent/codex.go` performs no
filesystem search.

deck adopts the id through the receiver path that already exists: `deck _hook` resolves the
session by the row id in its environment when the payload's conversation id is not one deck
knows, and a `SessionStart` whose id differs from the row's writes it to the row (§8.1's own
rule, unchanged). None of that is codex-specific.

**Hooks are injected inline; nothing is written to disk.** Codex accepts an entire hook
configuration as a command-line override — `-c 'hooks.<Event>=[{hooks=[{type="command",
command="…"}]}]'` — which buys §8.1's guarantee for codex too: a deck session is
instrumented, the user's `$CODEX_HOME` is untouched, and an ordinary codex run in the same
directory is unaffected. Per-session facts never go in the hook *command*, which is a
constant `deck _hook`, because codex's trust hash covers the command string (below); they
arrive in the payload and in the pane environment the hook process inherits (verified:
`DECK_SESSION_ID` is visible to the hook).

**Codex will not run an untrusted hook, and deck buys trust with
`--dangerously-bypass-hook-trust`.** An untrusted hook is **skipped silently** — no warning,
no error, no exit code, nothing deck could detect from codex's output. Trust is otherwise a
`[hooks.state."<source>:<event>:<group>:<hook>"] trusted_hash = "sha256:…"` entry in the
`$CODEX_HOME` `config.toml`, and that hash is a function of the event, the group's matcher
and the normalised hook definition only — portable and constant for a *fixed* command
string, but deck's command embeds deck's own absolute executable path, and the preimage
algorithm was not recoverable from the binary, so deck cannot compute the entry it would
need. The two alternatives are worse: harvesting the hash once by driving codex's
interactive trust UI writes to the user's config and breaks on any codex release that
changes the hash, and running codex out of a deck-owned `$CODEX_HOME` splits the user's
session history and credentials in two, since `codex resume <id>` only works in the
`$CODEX_HOME` that owns the rollout file. One flag on deck's own launch is the smaller
price.

It is a real reduction and is recorded as one: for the duration of a deck-launched codex,
the user's *own* configured hooks also run without review. deck's mitigation is that it
writes no hooks anywhere — every hook deck adds is visible in the pane's own command line —
and that the flag affects nothing else: approvals and the sandbox are governed by §5's
profile flags, never by this one.

Subscribed events — the same shape as §8.1, and deliberately **not**
`PreToolUse`/`PostToolUse`, which fire once per tool call and carry the tool's whole output:

| event | → status | notes |
|---|---|---|
| `SessionStart` | `idle` | **lands on `idle`, never `running` (R307)**; carries `session_id` — the id deck adopts — and `source` (`startup` \| `resume` \| `clear` \| `compact`). Codex fires it at the first prompt, immediately followed by `UserPromptSubmit` |
| `UserPromptSubmit` | `running` | also feeds prompt count; **enters `running`** |
| `PermissionRequest` | `waiting` | **the golden signal**, codex's analogue of Claude's notification: `tool_name` (`Bash` for a command approval, `apply_patch` for a file edit) plus `tool_input.command`, and `tool_input.description` when the model supplied a justification |
| `Stop` | `idle` | carries the final assistant text under `last_assistant_message` — the same field name Claude uses; nullable when the turn produced no text |
| `SessionEnd` | `stopped` | `reason` observed only as `other`, on both `/quit` and a double `Ctrl+C`. The one event with no output schema: fire-and-forget, it cannot influence codex |

Where the two agents overlap, codex uses **the same event names and the same payload field
names** as Claude (`session_id`, `cwd`, `transcript_path`, `hook_event_name`,
`last_assistant_message`, `permission_mode`, `source`): its hook surface is a deliberate
Claude-compatible shim, so the receiver parses codex payloads with no new payload work.
`PermissionRequest` is the only genuinely new event name. Codex has no analogue of Claude's
stop-failure event, so a codex row's `error` comes from the probe or from process death, not
from a hook. The probe reads codex's `■` error glyph only as the head of the last content block above the
composer (R313, §7), never from a `■` elsewhere in the capture.

**The one hard limitation: in the interactive TUI `SessionStart` does not fire at launch — it
fires when the first prompt is submitted.** A freshly launched, never-prompted codex is
uninstrumented, so for that window:

- status comes from the probe and the row honestly reads "sampled" (§3), with no special
  case: the badge already follows the last verdict's source;
- the row has **no conversation id**, says so, and is not resumable — `r` refuses with that
  reason rather than guessing. Nothing of value is lost: a codex session that has never been
  prompted has no conversation to resume.

Resume is otherwise stable, verified end to end: `codex resume <id>` replays the
conversation, and `SessionStart` fires again with the same `session_id`, the same
`transcript_path` (the rollout file is appended, not rotated) and `source: "resume"`.
`resume --last` stays banned (R2). Two upstream facts deck depends on: resume needs a TTY,
which a tmux pane always is; and it finds a session only in the `$CODEX_HOME` that owns
`sessions/<yyyy>/<mm>/<dd>/rollout-<ISO>-<id>.jsonl` — one more reason deck uses the user's
own `$CODEX_HOME` rather than one of its own.

Every contract above is upstream, re-verified by `@real-agents` (§13.5) rather than trusted
indefinitely, and every one has the probe fallback (§7): if a codex release stops firing
deck's hooks, a codex row degrades from live to sampled instead of breaking.

**A typed name is applied by `/rename` (R257, best effort).** Codex has no name flag, so it
declares `NameCaps.LaunchName = sendkeys_after_ready` and `PushRename = sendkeys`; its argv
never carries a name. The plan is `/rename <name>` typed as literal text, a pause of about 0.7 s
(`agent.CodexRenameGap`, carried by the adapter's `PushGap` and waited through
`Service.Pause`, never an unmeasured sleep), then a lone `Enter`; without the gap Codex's
composer reads text and Enter arriving together as a paste and keeps the Enter as a newline.
The text is sent only when the probe reports the prompt ready (`agent.PromptGate`): the empty
composer of a fresh or an idle session, with no update, trust or approval modal over it. A
working turn, an error, a typed draft and a modal are not ready, and nothing is sent. The
reconcile tick applies a `user` row's name to a row still `starting`, retrying on later ticks
until the screen is ready, and records `harness_name_sent` (reason: the name) before the
keystrokes go out, so a tick inside the gap never types the name twice, and so a name is
typed once per value and never repeated by a relaunch. A later rename of a live row takes the
same plan through the push gate of §8 "Pushing a rename", which is the same gate as for every
other sendkeys adapter: only an `idle` or `running` row with no focused dialog is typed into,
and on top of that Codex's prompt must be ready. A `starting`, `waiting` or `error` row is
refused with a `harness_rename_refused` event and a footer note. Only the startup naming above
accepts a `starting` row, because a never-prompted Codex stays `starting`; the two paths are
separate, so an ordinary rename of a still-starting Codex row is not pushed and its name is
typed by the startup naming instead. A send that fails records
`harness_rename_failed`, is not retried, and leaves deck's name in place. The mechanism is
best effort: it depends on Codex's composer timing and on screen text, and a Codex release
that changes either leaves the name unapplied rather than broken.

**No TUI is needed to apply it.** The same attempt (the readiness gate, the paced text and
`Enter`, the `harness_name_sent` marker) is `Service.ApplyLaunchNameWhenReady`: it repeats the
tick's attempt every 250 ms for a `user` row until the name is typed, the row is past `starting`
or gone, or two minutes pass. A detached `deck new --agent codex --name N` starts it as the
hidden verb `deck _apply_name <id>` in its own session with no terminal, so the name reaches Codex
with no other deck running; the create modal's session is served by the running TUI's tick. A
tick and the applier racing for one row type the name once, because the marker is claimed in a
single statement (`store.ClaimSessionEvent`). A prompt that never becomes ready records
`harness_rename_failed` (reason `launch name not sent: <why>`), the applier exits 1 with the
reason on stderr and in the audit log, and deck's name stays as given. An applier that cannot even be started (the re-exec fails) is the same failure at the
launch boundary: `deck new` still succeeds with the row and its name, and records
`harness_rename_failed` (reason `launch name not sent: the applier was not started: <why>`) as well
as the audit line.

**Reading Codex's name (R258, best effort).** Codex generates a title about a second after the
first prompt and stores it in `<CODEX_HOME or ~/.codex>/state_5.sqlite`, table `threads`, column
`name` (`NULL` until the thread has one). It declares `NameCaps.AutoTitle=true` and
`ObserveName=[sqlite, pane_title]`; no Codex hook payload carries a name. `ReadName` opens the
database with `mode=ro` and a 250 ms busy timeout, reads `name` for the row whose `id` is the
bound conversation id, and never writes: the database file is byte-identical after a read.
The query is capped, `SELECT substr(name, 1, 1024)`, so a value of any size is never copied whole
into deck: the read takes at most 1024 characters, then `SanitiseHarnessName` cleans them and cuts
the result to 200 runes (the bound of "Adopting a harness name"), and that is what `ReadName` returns.
`name IS NULL`, and a thread with no row yet, mean "none yet" and change nothing. A missing
database, a missing `threads` table and a `threads` table without the `id` or `name` column are
three degrades of the same kind: the name is read from the pane title instead (`[spinner ]<name> |
<cwd basename>`, read with the per-pane call of §8.1 and accepted only when it carries that
` | <cwd basename>` suffix, so tmux's host-name default is never adopted), the degrade is
logged once per process to the audit log, and a row that has no readable name keeps the dummy name
it already had. The `_5` in the file name is Codex's own schema version, so a release that bumps it
leaves the file missing and lands in the same path.

Codex does not say whether a human chose a name, so the explicit/generated distinction is a
**best-effort heuristic**: the first name read for a `derived` row is Codex's generated title
(`name_source = harness`); a later read that differs from the name deck adopted (ignoring a
collision suffix `-2`, `-3`) was changed after the generated title, which Codex writes only once,
so a human chose it inside Codex (`/rename`), and the row becomes `name_source = user`, which
freezes it. A name changed by deck's own push is held back by the pending-name echo of §8 "Pushing
a rename" before it reaches this comparison. It can be wrong in the corners: a name a human typed before the
first prompt is adopted as generated, and only a further change freezes the row.

---


### 8.3 Pi instrumentation

Pi's event source is its extension API, verified against pi 0.84.1: `pi -e <file>` loads one
extension for that process only, and the extension subscribes Pi's own events. Nothing is
written to the user's global Pi settings or extension directories. Every Pi launch (create,
resume, restart) adds `-e <data root>/pi/deck-hook.js` to the argv and puts the session's hook
executable in the pane environment as `DECK_PI_HOOK_EXECUTABLE`: the absolute path of the
launching deck binary, verbatim and unquoted (Claude and Codex embed the same binary, quoted, in
their own launch argv, §3.1). The launch also exports `DECK_LAUNCH_GENERATION` when it holds a lease,
as it does for the other two. A running Pi therefore keeps the hook binary of the deck that
launched it, which is what the R204 message, the auto-heal and the stale-binding hint (§3.1,
§11.4) all assume.

The extension file is a constant: deck writes it, through a temporary file and a rename, before
each launch that needs it, so every deck binary writes the same bytes. When the environment holds
no hook executable the extension does nothing. Each handler runs that executable with `_hook` as
its only argument, as separate argv elements and with no shell (so a path holding spaces, quotes
or `;` is passed intact and no string is ever parsed as a command line), with
one Claude-shaped JSON payload on stdin — `hook_event_name`, Pi's own `session_id` (the id deck
assigned with `--session-id`), and the event's fields — and swallows every failure, so a hook
problem never stops the agent. A hook that fails shows its stderr as a warning in Pi's UI when it
has one: for a database newer than the hook binary, that is the R204 restart message.

| Pi event | hook event | → status | notes |
|---|---|---|---|
| `session_start` | `SessionStart` | `running` | `source` is Pi's reason (`startup`, `resume`, `new`, `fork`, `reload`) |
| `before_agent_start` | `UserPromptSubmit` | `running` | |
| `agent_settled` | `Stop` | `idle` | `agent_end` is not used: Pi can still retry or continue after it |
| `session_shutdown` | `SessionEnd` | `stopped` | `reason` is `clear` (new), `resume` (resume, fork, reload) or `other` (quit); the first two only replace the session inside the same process and never stop the row |
| `session_info_changed` | `session_info_changed` | none | event only; carries `name` and `cleared` (below) |

**Pi's name (R262).** The extension also reports Pi's session name, through two optional payload
fields no hook is required to carry. On `session_start` it adds `session_name` (what
`pi.getSessionName()` returns, omitted when Pi holds none; `--name` writes it before the event). It
subscribes `session_info_changed` (Pi's name-change event, fired by `/name` and
`pi.setSessionName()`, not by `--name` at startup) and sends a payload of `hook_event_name`
`session_info_changed`, `session_id`, `name` (empty when Pi's name is undefined) and `cleared`
(true exactly when Pi's name was removed). `_hook` decodes `session_name`, `name` and `cleared`
as optional fields: `session_name` on `SessionStart` and `name` on `session_info_changed` are the
name the harness reports a human chose, and hand the row to name adoption (§8) as a hint, so a
`derived` or `harness` row takes that name with `name_source=user` (Pi never titles a session
itself, so every Pi name is explicit) and a `user` row stays frozen. A `SessionStart` without
`session_name` triggers no adoption, a cleared name carries no hint and leaves deck's name
unchanged, and a payload without the fields behaves exactly as it did before they existed. The
status write is untouched: `session_info_changed` writes the event only. `cmd/fake-pi` plays a
name change with the pane command `{"command":"name","name":"x"}` (an empty name is a cleared
one), firing the extension's `session_info_changed` payload.

**Pushing a rename into Pi: the control file (R263).** Pi declares `PushRename=sendkeys` and
`ObserveName=[extension]`; the preferred transport is not a keystroke but a per-session control
file the extension owns. The launch puts `DECK_PI_CONTROL_DIR` in the pane environment, the
directory `<deck home>/pi/control` of the launching profile's own data root (so a second profile
has its own directory and never reaches another profile's Pi). In `session_start` the extension
creates, truncated, `<DECK_PI_CONTROL_DIR>/<Pi session id>.jsonl` (the id deck assigned with
`--session-id`; an id that is not a single safe path component opens nothing), writes as its
first line `{"pid":<Pi's process id>}`, and watches it.
Each line deck appends is one JSON object, `{"name":"<sanitised name>"}`; a complete line whose
name is a non-blank string is handed to `pi.setSessionName()`, any other line is ignored, and a
partial line waits for its newline. `session_shutdown` stops the watch, closes the file and removes
it, so a `session_start` that replaces the session inside the same process opens the new session's
file. A rename push (§8 "Pushing a rename") to a Pi row first tries the file: when it exists, is
a regular file and its owner is shown running, deck appends the line, sends no keystroke, and holds
the sanitised name as pending for its echo. The liveness check is finite: the file's first line
must be a JSON object whose `pid` is greater than 1, and a signal-0 probe of that pid must find a
process (one this user may not signal still counts as running). A pid the probe finds gone means a
dead Pi left the file behind, so deck removes it and the push takes the keystroke plan below, never
reporting a send through the file. A file with no readable pid line (empty, no newline yet, or any
other first line) is not live either and is left in place, since the extension may be between
creating it and writing the line. The append opens with `O_NOFOLLOW`, so a control path that is a
symlink is never written through. Pushing through the file does not need the idle/running gate or the dialog check, since nothing is
typed. When the file is unavailable (no extension running, no hook executable in the environment,
a launch without a data root) the push is the keystroke plan `Ctrl-U`, `/name <name>`, `Enter`
under the §8 gate, and only then. Part of that gate is Pi's dialog check: the adapter is a
`DialogDetector` that reports a focused dialog when a screen line carries the key hints Pi draws at
the foot of a selector or input dialog (`↑↓ navigate` or `enter submit`, together with `cancel`;
with the default keybindings `↑↓ navigate  enter select  escape/ctrl+c cancel`, the line every
`ui.select` and `ui.confirm` dialog shows). While one is up the push is refused with "a harness
dialog is focused", because the typed line would answer the dialog instead of renaming. `setSessionName` fires Pi's `session_info_changed`, so the
extension reports the pushed name back; the receiver recognises it as the pending push and clears
it (R252 echo suppression), and it is never adopted as a name a human chose in Pi. `cmd/fake-pi`
plays the extension's file handling when it runs with the extension and `DECK_PI_CONTROL_DIR`
(its own pid on the first line): it opens the control file at its `start` pane command (session_start), not at launch, so a Pi row
that has not started has no file and a rename push to it takes the gated keystroke path. Its
`dialog` pane command draws a `ui.confirm` selector with that hint line, and the next typed line that
is not a JSON command answers the dialog and never reaches the editor.
`cmd/fake-pi` persists a name the way Pi does, as `{"type":"session_info","id":"<8 hex>",
"parentId":<previous entry id or null>,"timestamp":"<ISO-8601>","name":"<name>"}` lines in the
session file (`~/.pi/agent/sessions/--<cwd>--/<timestamp>_<id>.jsonl`): `--name` appends one at
launch and fires no hook, and the `name` pane command, a typed `/name <name>` line and a control-file
line each append one and fire `session_info_changed`. The fixture never writes a name on its own,
as Pi never titles a session. `features/harness_names_pi.feature` (`@harness-names`) covers the
explicit, deck-rename-pushed, harness-side-rename-adopted and focused-dialog refusal cases through
those records.

Pi has no event for a permission prompt or a failed turn, so a Pi row never reports `waiting` or
`error` from a hook; those verdicts still come from the probe (§7) and from reconcile.

**Launch name (R261).** Pi declares `NameCaps.LaunchName=flag` and `AutoTitle=false`: Pi never
titles a session itself, so deck's name is the only name it has. A row whose `name_source` is
`user` and whose name is not blank launches and resumes with `--name=<name>` (one attached
argv element, R268, so a name that begins with `-` is never a flag of its own), once, after the
profile's flag (`--approve`) and before the row's `launch_args`; the flag rides every launch
and every resume, so the name survives a restart. A `derived` or `harness` row passes no
`--name`, and a blank name never passes one. The name is sanitised first (control characters
go, whitespace collapses, it is trimmed and cut to the shared harness limit). `-n`, `--name`,
`--name=<x>`, `-n=<x>` and the attached short form `-nFOO` in `launch_args` are refused by
`Launch` and `Resume` with an error naming the flag, because deck owns the name; every other
extra argument (`-p foo`, `--x`, one that holds a newline) is appended unchanged.
`cmd/fake-pi` accepts `-n`/`--name <value>` and `--name=<value>` and echoes it.

### 8.4 Copilot CLI

GitHub Copilot CLI is the fifth agent kind, `copilot` (executable `copilot`). Deck assigns the
conversation id, which must be a UUID: `Launch` refuses any other id with an error, because
Copilot exits 1 on it and the failure is then deck's, not a dead pane's.

**Argv.** `copilot --session-id <conversation-id>`, then the profile's flags (§5), then
`--no-auto-update`, then the row's `launch_args` verbatim. `Launch` and `Resume` refuse, with a
deck-side error and no argv, a `launch_args` entry that is `--continue`, `--resume`, `--connect`,
`--remote`, `--acp`, `--yolo`, `-n` or `--name`, bare or in `--flag=value` form (the name flag also in the
attached short form `-nFOO`), for every profile; any other extra argument (for example `--model gpt-5`,
`-p foo`, `--x`, or one that holds a newline) is appended unchanged:

| profile | flags |
|---|---|
| `safe` | none |
| `edits` | `--allow-tool=write` |
| `yolo` | `--allow-all` |

`--session-id` creates the session when it is new and resumes it when it exists. **`Resume`
therefore returns the argv `Launch` returns** for the same conversation id, profile and
`launch_args`, minus the name flag (below). It never emits `--resume`, which fails for a session killed before its first
message, so no relaunch special case exists: the adapter does not implement
`FreshRelauncher` (§9.1) and the service always takes the resume path. No argv built here
contains `--continue`, `--resume`, `--connect`, `--remote`, `--acp` or `--yolo`.

**Launch name (R259).** Copilot declares `NameCaps.LaunchName=flag_new_session_only`. A row
whose `name_source` is `user` and whose name is not blank launches a *new* conversation with
`--name=<name>` (one attached argv element, R268, so a name that begins with `-` is never a
flag of its own), once, after the profile's flags and before `--no-auto-update`: the first launch
and the fresh-once launch, which mints a new conversation id. `Resume` (a relaunch of an
existing conversation id) never carries `-n`, so Copilot keeps the name it already holds, and
a `derived` or `harness` row never passes one. The name is sanitised first: `"` becomes `'`,
control characters go, it is trimmed and cut to 100 runes. A blank name passes no flag. `-n`,
`--name`, `-n=<x>`, `--name=<x>` and `-nFOO` in `launch_args` are refused by `Launch` and `Resume` with a
deck-side error that names the flag and no argv: deck owns the name.

**Reading Copilot's name (R260).** Copilot declares `NameCaps.AutoTitle=true` and
`ObserveName=[hook, yaml]`. The source of truth is `<copilot home>/session-state/<conversation
id>/workspace.yaml` (the home is the session's `COPILOT_HOME`, else `~/.copilot`), read when a
trigger hook (`agentStop`, `session_info_changed`) or the reconcile tick asks. The reader parses
it with a real YAML parser (`gopkg.in/yaml.v3`), so a name holding `:`, `'`, `#` or Unicode comes
back exactly as Copilot wrote it, and reads at most 64 KiB. A `workspace.yaml` that is a FIFO,
socket, device or symlink is refused before it is read (`lstat`, then an `O_NONBLOCK|O_NOFOLLOW`
open), so a FIFO with no writer is "no name" and never blocks the read (R270). A `user_named: false` file with a
non-empty `name` is Copilot's own generated title: adopted as `name_source = harness`. A
`user_named: true` file whose name differs from deck's was chosen by a human (`-n`, `/rename`):
adopted as a name a human chose, which freezes the row. No `name`, a blank one, an `id` that
names another conversation, an unsafe conversation id, a missing or unparsable file are all "no
name". So is a file whose `user_named` marker is absent, null or not a YAML boolean node: a
string-typed value (`!!str yes`, a quoted `"true"`) or a YAML 1.1 word (`yes`, `off`) is not
`true`/`false`, so a present `false`
is told apart from a missing key, and a changed upstream contract (R265) leaves the row's dummy
name and status alone. deck never writes the file.

**Pushing a rename into Copilot (R260).** Copilot declares `PushRename=sendkeys`. The plan is
two send-keys calls: the text `/rename <name>`, then a lone `Enter`. The name is cleaned again
for Copilot (`"` becomes `'`, control characters go, at most 100 runes) and a name with nothing
left is declined. The push gate, dialog check, echo wait and failure events are those of §8
"Pushing a rename". Copilot implements `agent.DialogDetector`: the folder-trust prompt, a
permission dialog and a question dialog on the visible screen, recognised by the same rules the
pane probe uses, are a focused dialog, so a rename is refused while one is up even when the hook
status still reads `running`. `fake-copilot` writes `name:` with `user_named: true` for an explicit `-n`,
and gives a session without a name a generated `name:` (its first words) with `user_named: false`
at the first prompt, never replacing a name and never retitling on a later prompt. A typed
`/rename <name>` (or `/session rename <name>`) line, deck's push or a human's, writes the name with
`user_named: true` at once, idle or mid-turn, and the fixture titles its terminal
`<name> - GitHub Copilot` (OSC 0) whenever a name is set, at launch included.

**Capabilities.** Profiles `safe`, `edits` and `yolo` (no `plan`: it falls back to `safe`, §5);
`AssignsConversationID`, `Resumable` and `HasTranscript` are true; the declared transcript
environment key is `COPILOT_HOME`, resolved from the session's own layering (§6.1), never
from deck's ambient environment.

**Transcript path.** `<root>/session-state/<conversation-id>/events.jsonl`, where `<root>` is
the session's resolved `COPILOT_HOME` when it is non-empty and `<home>/.copilot` otherwise. The
lookup declines (`ok = false`, never an error, never a guess) when neither a `COPILOT_HOME`
nor a home directory is known, when the id is not one path component, and when the file does
not exist.

**Instrumentation.** A copilot launch loads a **static, deck-owned plugin directory** with
`--plugin-dir <data root>/copilot/plugin` (never under `~/.copilot`: deck writes nothing into
Copilot's home) and exports `DECK_EXE=<absolute path of the launching deck binary>` (plus the
launch generation, §3.1, when the launch holds one). Only the `yolo` profile also exports
`COPILOT_ALLOW_ALL=true`, which suppresses Copilot's folder-trust screen and grants nothing
`--allow-all` does not already grant; `safe` and `edits` never get it. `Instrument` is a pure
function of the launch input; the plugin files are written by the launch path in
`internal/service`, once per create, resume and restart, before the pane starts: each file is
replaced through a temporary file and a rename, the directory is mode 0700, and a file that
already holds the constant content is left untouched.

**Environment and isolation.** A session's `COPILOT_HOME` is an ordinary entry of the §6.1 layers
(server env, then config `[env]`, then session env): it moves the transcript root above and
nothing else, because the plugin directory stays under deck's data root whatever the session
sets, and deck never writes under it. Deck's launch never sets, clears or edits
`COPILOT_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`, `COPILOT_MODEL` or `HTTPS_PROXY`, and
sets `COPILOT_ALLOW_ALL` only for `yolo` (above); a value the operator put in a layer reaches the
pane as written. The session env editor (§6.2) has no copilot-specific rule: a `COPILOT_*`
variable is listed, edited and persisted like any other, and one whose name looks secret (for
example `COPILOT_GITHUB_TOKEN`) is masked by §6.4. A pane that prints nothing at all for the
launch window, such as a Copilot waiting on a dead proxy, stays `starting` (§7) and is never
marked `error` by deck; it leaves `starting` when its first output gives the pane probe or a
hook a verdict.

The directory holds a `plugin.json` and a `hooks.json` whose content is a constant of the deck
binary. `hooks.json` subscribes to exactly six observational events: `userPromptSubmitted`,
`sessionStart`, `notification`, `agentStop`, `errorOccurred` and `sessionEnd`. It never
registers `permissionRequest` or `preToolUse`, because those hooks can decide for the user.
Each entry is a bash command with a 5 s timeout that runs `"$DECK_EXE" _hook` with the event's
name in `DECK_HOOK_EVENT` and Copilot's JSON payload on stdin, prints nothing on stdout and
exits 0 whether `DECK_EXE` is unset or the hook fails. The deck row identity comes from the
pane environment the launch already exports (§6.1).

**Hook table.** `deck _hook` reads the event name from `DECK_HOOK_EVENT` when it is set (Copilot's
payloads carry none; the environment's event wins over a `hook_event_name` the payload also carries) and the conversation from the payload's camelCase `sessionId`.
Every other call keeps the payload's own `hook_event_name` and `session_id`. The finite set:

| event | status | reason |
|---|---|---|
| `userPromptSubmitted` | `running` | `prompt` |
| `sessionStart` | `idle` | the payload's `source` (`new` or `resume`); **lands on `idle`, never `running` (R307)**, and `userPromptSubmitted` enters `running` |
| `notification`, `notification_type` `permission_prompt` | `waiting` | `permission_prompt` |
| `notification`, `notification_type` `elicitation_dialog` | `waiting` | `elicitation_dialog` |
| `notification`, any other `notification_type` | no change | none |
| `agentStop` | `idle` | `end_turn`, or the payload's `stopReason` |
| `errorOccurred` | no change | none |
| `sessionEnd` | `stopped` | the payload's `reason` |

A no-change event still leaves its event-log entry; the row is untouched. An `agentStop`
whose `transcriptPath` is a regular file under the resolved Copilot root (`COPILOT_HOME`, else
`<home>/.copilot`, symlinks resolved) is recorded on the row as a `transcript` event; a path
outside the root, or one that is not a regular file, is ignored. **Identity.** A payload whose
`sessionId` differs from the row's conversation id (Copilot's `/clear`, `/new`, `/resume` and
`/fork` swap the live id) is not applied: the row keeps its deck-assigned id and status and
exactly one event, kind `<event kind>.identity_mismatch`, records it; the row never follows
the new id. A payload with no `sessionId` is applied through the `DECK_SESSION_ID` row
identity like any other kind's. The superseded-launch rule (R74), the budgets, the stale-binary
message and the auto-heal apply unchanged.

**Pane probe.** Copilot's pane verdicts come from `internal/agent/probe.go` and are matched on
substrings that survive an 80-column wrap (the wrap splits `Manual Approval` across rows), never on a
whole footer line. The rules, in **precedence order** (the first match wins):

| # | rule (all of it must hold) | status | reason |
|---|---|---|---|
| 1 | `Confirm folder trust` and `Do you trust the files in this folder?` | `waiting` | `folder trust` |
| 2 | `Do you want to ` and `↑/↓ to navigate · enter to select · esc to cancel` | `waiting` | `permission prompt` |
| 3 | `Copilot needs information.` | `waiting` | `question` |
| 4 | a line of the live footer (the lines below the composer's last separator rule, never a transcript line above it) starts with a spinner glyph (`○ ◎ ● ◉`) and holds `Working` and `esc interrupt` or `esc edit prompt` | `running` | `working indicator` |
| 5 | a line starting `✗ ` after the last turn (`❯ `) and above the context line (the cwd line over the composer) | `error` | `error line` |
| 6 | the idle footer: `· / commands`, or `@ files · # issues` when a draft is typed | `idle` | `ready` |
| 7 | anything else | no verdict | none |

A line starting `! ` is a warning and never an error. The trust dialog and the permission dialog share
the navigation line, so the trust sentences are tested first. A `✗ ` line is also how Copilot prints a
declined tool call or a failed slash command; any such line after the last turn reads as `error`
until the next turn scrolls it away. The rules are fitted to the real 1.0.93 captures in
`internal/agent/testdata/probes/copilot/`.

A launch whose plugin directory cannot be written is **not failed**: it starts without
`--plugin-dir`, records a `note` event on the session naming the cause, and the row's status
then comes from the probe alone (§7).

## 9. Lifecycle

### 9.1 Resume, on demand

There is no boot-time restore and no `autostart` (R3). After a reboot the list is intact,
every session reads `stopped · resumable`, and `r` brings one back:

- Create `deck_<slug>` at `cwd` on the deck socket; run the launch hooks — the global
  `pre_launch` from `config.toml` (§6.5) and then the session's own, either of which may be
  empty; launch the agent with its **resume** argv and the session's env/permission profile.
- **The launch hooks are fail-closed**, and this is the one place that says so: they run in the
  pane, ahead of the agent, joined to it so that a non-zero exit short-circuits everything after
  it. The agent is never started, the pane is retained with the hook's own output visible
  (`remain-on-exit failed`, §3.2), and the row lands in `error` with that as its reason. A
  session that would start *without* the credential its hook was supposed to fetch is worse than
  a session that refuses to start, so refusing is the behaviour, not a failure mode of it.
- **With `[ui] attach_on_resume` on (default false, §6.5), a successful `r` or `R` goes on to
  enter the interactive preview on that session**, through the same auto-enter intent a create
  arms (§11) — once its pane is live, only in the client that pressed the key, cancelled by any
  key or click first, and refused exactly as `↵` would be — while `U` (unarchive, §9.2) and
  `u`'s undo of an `x` never arm it.
- A resumed session enters `starting` and becomes `running` on the agent's first signal,
  exactly as in §7 — there is no special post-resume status. Hook agents typically reach
  `running` within a second; probe agents may sit in `starting` until `stale_after`, which
  the row shows as `starting · awaiting signal`. **No prompt is ever re-sent** — resume
  reconstitutes context, it never resumes autonomous work.
- Resume failure (unknown id, missing directory, agent binary gone) → `error` with the
  reason, row retained. Never delete, and never silently start a *fresh* conversation in
  place of a failed resume.
- **Resume and restart refuse an archived session** (`R` routes through resume, so one guard
  covers both). The check happens **before the launch lease is taken** (§9.3), so an archived
  row never even briefly reads `starting`, and nothing is created: no tmux session, no pane.
  The row is retained and the message names `U` (unarchive, §9.2) as the way forward. This is
  the archived → live half of §4's invariant; without it, `stopped + archived` → `r` yields a
  live agent hidden behind the filter, with its hooks unroutable and its status frozen.
- **Resume clears `killed_by_user`.** The flag exists so an in-flight hook cannot undo an
  explicit kill (§7); once the user explicitly resumes, that verdict is spent — if it
  survived, every future hook for the session would be outranked forever and the row could
  never leave `stopped` by automation again.
- **Resume likewise clears `pane_exit_status` and the crash tail**, on the same reasoning and
  for the same class of failure. They are a verdict about a pane that no longer exists; once
  resume has created its replacement, a stored crash must not be read as a statement about
  the live pane. A crash verdict that outlives its pane silently removes the row from
  reconciliation and blocks the hook transitions that would correct it, which is the same
  "spent verdict outranks everything forever" bug as the one above.
- **Locking a conversation** (the *resume mode*, reached with `c` inside the `i` detail dialog,
  §11.4), for forcing a specific conversation: a lock sets `resume_state = pinned` and is
  sticky across restarts; a one-shot "start fresh" reverts to `auto` afterwards. The stored
  value keeps its historical name `pinned`, but the UI calls this a *lock* and never a pin:
  "pin" in the UI means only §11's sidebar pin, which is unrelated to resume.
- **A Claude conversation that has no transcript yet is started again, not resumed.** Claude
  writes no transcript until the first message, and `claude --resume <uuid>` of a conversation
  with none fails ("No conversation found"), so a session restarted or resumed before its first
  message would fail every time. The relaunch decision for `r` and `R` is a finite set of cases:
  1. Claude, the conversation **not locked**, the home directory known, no `CLAUDE_CONFIG_DIR`
     in the session's env layering (session env over config `[env]` over the tmux server's
     environment, falling back to deck's own), and **no transcript file** at
     `$HOME/.claude/projects/<cwd with separators as "-">/<conversation id>.jsonl`, where the
     stored cwd is cleaned (`filepath.Clean`: no trailing or doubled separator, no `.` or `..`
     element) before its separators are encoded, and the row's stored cwd is not changed →
     `--session-id <the same uuid>`. The conversation id on the row is unchanged.
  2. A transcript file exists → `--resume <uuid>`, even when its metadata cannot be read (a
     symlink whose target is unreadable still counts as an existing entry). Absence has to be
     *positively known*: only a filesystem answer of "does not exist" for the transcript path
     (or a directory sitting where the file should be) selects case 1. Any other lookup
     failure (permission denied, a path component that is not a directory, a name too long, an
     I/O error) leaves the transcript state unknown and keeps `--resume`.
  3. The conversation is locked (`resume_state = pinned`) → `--resume`.
  4. `CLAUDE_CONFIG_DIR` is set (the transcript location cannot be known) → `--resume`.
  5. The home directory is unknown → `--resume`.
  6. Codex, Copilot and Pi → their resume argv from the table in §8, whatever the transcript state. Copilot's resume argv is its launch argv (§8.4), so a copilot session restarted before its first message comes back on the same conversation id and is never relaunched fresh.
  Where an adapter reads a transcript from a cwd-derived path, the stored cwd is cleaned first: Pi's transcript directory (`$HOME/.pi/agent/sessions/--<cwd>--`) is encoded from the cleaned cwd, so a trailing slash, a doubled slash or a `.`/`..` element finds the same transcript. Codex (globbed by conversation id) and Copilot (`session-state/<id>/events.jsonl`) never read the cwd, so its spelling cannot change their result.

  `resume_state = fresh-once` is unchanged: it launches a new conversation id once.
- `shell` sessions "resume" by recreating the shell with their history file, replayed
  scrollback, and last known working directory (§9.4). A shell session never re-runs a
  previous command on resume.

### 9.2 Kill and delete

Teardown is cheap because R1 means no session owns anything on disk. That earns a
low-friction UI — but the friction scales with how hard the action is to undo and how
obvious its effect is, which is why `x` needs no confirmation while the two actions that
also *remove the row from view* do:

| action | key | effect |
|---|---|---|
| kill | `x` | `tmux kill-session` immediately. Row → `stopped`. Conversation untouched, resumable. 10 s undo toast (`u` = resume). |
| delete | `dd` | kill + tombstone the row (`deleted_at`). Hidden immediately, undoable for 60 s, **reaped** after — see below. |
| purge conversation | in the delete confirm only | additionally deletes the agent's transcript. Never implicit, never default, always a separate explicit choice. |
| archive | `A` | **confirms first** (§11.4), then keeps the record and hides the row from the default list. On a live session the same confirm covers the kill, and says so in as many words — the dialog names the session and states that a live agent will be killed. Nothing is written until it is confirmed. On success a toast says what happened, with `u` to undo, and the row is hidden in the same frame as that toast. An archived row is **not startable** (§9.1). |
| unarchive | `U` | clear `archived_at`. The row returns to the default list reading `stopped · resumable`, and `r` works on it again. Reachable wherever an archived row is — inside the `/` filter's results — and it is what `deleted_at`'s restore already is: the reversal without which archiving is a one-way door. |
| bulk | `m` marks | `x` / `dd` act on the mark set. |

deck never writes to or deletes anything inside a session's `cwd`.

**`post_destroy` runs on `A` and `dd`, and on nothing else.** A session whose launch hook
provisioned an external resource (§6.4) needs somewhere to release it, so those two actions —
the ones above that remove the row from view, both of which confirm or are undoable, and both of
which mean *I am done with this* — run the session's own `post_destroy` and then the global one
from `config.toml` (§6.5). Its shape is the inverse of `pre_launch`'s at every point where the
two could be confused:

- **Not in a pane.** The pane is already dead, so the hook runs as a subprocess of deck under a
  bounded timeout, with §6.1's session context in its environment — minus
  `DECK_SESSION_LAUNCH_KIND`, which has no meaning here, and plus `DECK_TEARDOWN_KIND`
  (`archive` or `delete`) so one hook can serve both. Its stdout is diagnostic; there is no
  child process left to export to.
- **Session's own first, then the global one** — the reverse of §6.5's launch order, and
  deliberately so: teardown unwinds in the order that releases the specific before the general.
  Each runs in its own subprocess, so neither observes the other's environment and, because both
  are fail-open, the first one's exit status never suppresses the second.
- **Fail-open, and reported.** `pre_launch` must be able to refuse a launch; a teardown hook
  cannot be allowed to refuse a teardown that has already happened. A non-zero exit or a timeout
  raises a toast and records an event, and never resurrects the row or reverses the action.
- **`x` does not run it.** `x` is the cheap, unconfirmed, everyday action and the row it leaves
  is `stopped · resumable`: the session has not stopped being a session, and its external
  resources should still be there when it resumes.
- **Reaping does not run it either.** A tombstone is reaped after its window by whichever process
  happens to open the store next, and a create or rename that reuses the name reaps it early —
  hanging a user-visible external effect off any of those is indefensible. The hook fires at the
  keypress that decided it.
- **Undo restores the row; the next `r` restores the resource.** `dd`'s 60 s undo, `A`'s undo and
  `U` all return the row `stopped`, never running, so no undo re-runs anything at the moment it
  happens — the next launch does, by §6.4's idempotency rule. The toast says exactly that, so the
  user is not left guessing which half came back.
- A bulk `dd` over a mark set runs it once per marked session.

**What `dd` removes, exactly.** *Purge* was doing two unrelated jobs in that table, so they
are named apart: the conversation purge is the checkbox, and the tombstone is **reaped**.
Reaping deletes the `sessions` row and every deck-owned row hanging off it — events,
the `waiting` and `notify_epoch` state — together with deck's
own per-session files, meaning §9.4's history file and captured scrollback. Afterwards
nothing remains to list, to search (§12) or to resume: the session is gone from deck without
a trace. The one deliberate exception is the JSONL log (§13.1), which keeps its append-only
record of what happened, because a log that rewrites itself when a row is deleted is not a
log.

**A tombstone that outlives its process is reaped at the next store open**, once its window
has passed. Undo is a live-session affordance and not a durable one — nothing that survives a
restart is still undoable — so a tombstone waiting for a grace window that no running process
is counting is not protecting anything. Left unreaped it leaks the row, its events, its files
and its **name**, none of which anything would ever collect.

**A deleted session's name is available again, and taking it forfeits that session's undo.**
"Gone without a trace" includes the name: a name that can never be reused is the deleted
session still occupying the one namespace the user types into. So creating a session — or
renaming one — with a name (or a §3.2 slug) whose only holder is a tombstoned row reaps that
row and proceeds. The 60 s undo dies with it, which is the accepted trade: the user named the
row they deleted, and then asked for its name back. Because that create silently destroys
another row's record, the dialog says so where it happens (§11.4's in-dialog validation), and
a `u` afterwards reports that the session was reaped when its name was reused rather than
failing obscurely.

**An archived row keeps its name, and `dd` is the way to free it.** Archiving keeps the record
on purpose and `U` reverses it, so letting a second session quietly take the name would make
the restored row's identity ambiguous — the refusal here is correct, and only its *message* has
to earn its keep: it names the archived holder and both ways out, `U` to bring it back or `dd`
to delete it. `dd` therefore applies to an archived row exactly as it does to any other.
Archived is a hidden state, never a protected one, and the row is reachable for it wherever
archived rows are reachable at all (§11.10).

**Deleting a deck session never touches the agent's own history.** Claude's and Pi's
transcripts live where those tools put them, and they belong to those tools — R1's "no
session owns anything on disk" cuts both ways. So `dd` kills the pane and forgets the row
while the conversation itself remains resumable by the agent's own CLI, and a user who
deletes a row in deck has not lost their transcript. The single exception is the explicit
**purge conversation** choice in the delete confirm — the one place deck removes a file it
did not create, which is why it is never implicit, never the default, and names the exact
path it will delete before doing it.

### 9.3 Launch leases (R4)

Two TUIs pressing `r` on the same `stopped` session must not double-launch. The
transaction that flips `stopped → starting` also CAS-acquires
`launch_lease_owner`/`launch_lease_until` (owner = `pid@boot_id#<generation>`, TTL ~30 s). A
stale lease (dead pid or expired TTL) is breakable. Only `pid@boot_id` is compared for
identity; the generation is carried in the same column rather than a second one because it is
minted by, and dies with, the same acquisition.

**The generation identifies the launch attempt, not the row.** It is **random**, never a clock
reading or a counter: deck's clock is injectable and frozen in tests (§13.1), so two launches
of one row can share a timestamp, and a discriminator two launches can share is not one. It is
exported into the pane beside the session id, so an event arriving from a pane can be told from
one arriving from the pane that replaced it — `DECK_SESSION_ID` names the row and cannot answer
that question. **A hook carrying a superseded generation is recorded and does not write
status**, for every event name rather than the one that motivated it: it is genuine history
from a pane that is genuinely gone, and the row it would otherwise describe belongs to a
different launch. A hook carrying no generation at all, against a row that has none, behaves as
it always did.
The `i` detail dialog's `Hook declined` line treats the replaced pane's own `session_end` as
expected and hides it; §11.4 gives the two declines that stay loud.

**A launch releases its lease the moment it concludes** — pane up or launch failed, on every
exit path — by clearing `launch_lease_until`, CASed on the exact owner that acquired it. The
lease exists to keep a second launcher out while a launch is *in flight*, and nothing else: once
the row is no longer `stopped` the status check already refuses a second launcher, so holding
the lease past the launch protects nothing while making the paragraph below impossible to honour
for 30 s. Release clears the **deadline only** and deliberately keeps the owner, because the
generation half of it is what the rule above discriminates on, and a pane can still emit a hook
long after 30 s.

**"starting elsewhere" is a claim about another client, so it is only made when one is
actually there.** A failed acquisition has two unrelated causes and they must not share a
message: another client holds a live lease → *starting elsewhere*; or the row was never
leasable in the first place because it is not `stopped` → the row's own status and reason
(*already running*, *already starting*). Reporting the second as the first sends the user
hunting for a second TUI that does not exist, and it hides the real state of the row — the
same class of lie as a fabricated status (§7). The store's answer therefore distinguishes
"held by <owner>" from "not leasable, status is <status>", and the UI says which.

### 9.4 Shell-session state

For `shell` sessions, and reused for agent sessions where noted:

- **History**: a per-session history file under the deck data dir, with the shell
  configured to append after every command (bash: history append + per-command flush;
  zsh: incremental append; fish: a named history session). A hard reboot loses nothing,
  and up-arrow after a resume is that session's own history — not the user's global soup.
- **Scrollback replay**, with an explicit owner for every capture — there is no daemon, so
  "a 5-minute tick" would have belonged to nobody:

  | capture trigger | who does it |
  |---|---|
  | user kills or stops a session | the TUI, before tearing the session down |
  | agent exits (clean or crash) | `_hook` on the session-end path; for crashes, the reconciler, using the dead pane that `remain-on-exit failed` preserved (§7) |
  | TUI shutdown | the TUI, for every live session |
  | opportunistic refresh | any `_hook` invocation older than `capture_min_interval` |

  Consequence, stated rather than implied: with no TUI running and no hook activity —
  an unattended shell session, say — the newest capture may be minutes or hours stale, and
  a power cut loses everything since. **History files do not have this problem** (they are
  appended per command); only the visual replay does.

  Mechanically, replay is not a screen write: the captured text (last N lines, default
  5 000, escapes preserved, compressed) is emitted by the pane's own first command, before
  the agent is `exec`'d. Default **on for `shell` sessions, off for agents**, because a
  resumed agent repaints its own conversation and would draw over the replay, showing the
  tail twice. Skipped entirely when `sensitive = 1`, and the config states plainly that
  whatever was on screen lands on disk.
- **Working directory**: snapshot the pane's current path at capture time and resume there
  rather than at the original `cwd`.
- **Where this lives on disk**: the cwd snapshot is the `last_cwd` column on `sessions`
  (nullable; `cwd` stays the create-time value and is never overwritten), and captures live
  under `$DECK_HOME/captures/<session_id>/`, referenced by convention rather than by row —
  so a missing capture file degrades to no replay, never to an error.
- Not included: environment snapshots on exit (magic, and secret-laden), command journals.

### 9.5 tmux server lifetime (optional systemd)

deck starts the server itself when needed, so nothing is required. An **optional** user
unit, installable from the health view, supervises the server across logins:
`Type=oneshot` with `RemainAfterExit=yes` (tmux daemonises itself, so `oneshot` tracks no
MAINPID for systemd to signal on stop), `exit-empty off` set in the same invocation (an
empty server exits immediately otherwise), and `KillMode=process` plus a no-op stop so
stopping the unit never sweeps the agents.

**The unit alone does not make sessions outlive a logout — lingering does.** Without
`loginctl enable-linger`, the user manager and its whole cgroup go away at logout, taking
the tmux server with it regardless of `KillMode`. So the health view treats linger as a
*requirement* of the outlive-logout goal, not a hint: it reports linger state and prints the
exact command. deck never runs privileged or account-level commands itself. The unit is
templated per profile (§3.4), one instance per socket.

---

## 10. Event hooks

deck has no notification client, no channel types and no templates. It has **one** extension
point: a user-supplied executable, the *event hook*, that deck spawns when a session records
a status-change event. The script decides what that means — a Telegram message, a desktop
notification, a webhook call, a log line, a screenshot upload. Integrating a service is a
script, not code in deck, and deck ships no service-specific client. It is the same shape as
`pre_launch` and `post_destroy` (§6.4, §9.2): one user-owned program, handed the session's
facts, that selects for itself. It is called an *event hook* rather than a notification
because nothing restricts it to messages.

### 10.1 The script and its contract

`event_hook` (§6.5, top level, empty by default) is one executable path or argv, run **without
a shell** on the same footing as an agent binary. **The path must be absolute**
(a bare name is looked up on `PATH`; a relative path such as `./hook.sh`, `scripts/hook.sh` or
`~/hook.sh` is not accepted, because `deck _hook` runs in the agent's directory, where it names a
different file or none). The settings view refuses to save a newly entered relative path, with a note;
a relative path already in a loaded `config.toml` is not a load error, and the health view (§11.4)
shows an `Event hook WARNING` naming it. **No script configured means the whole
feature is inert**, whatever any per-session setting says: the per-session fields are never
read and nothing is spawned.

Invocation: `event_hook <event>`. When `event_hook` is an argv, the event kind is still
argv[1]: the configured fixed arguments follow it (`script <event> <fixed>...`).

- **argv[1]** is the event kind, one of the **offered set**: `started`, `resumed`,
  `waiting`, `idle`, `error`, `ended`, `killed`, `renamed`. Nothing else is offered: a typed
  `prompt`, an `env` change, a `note` and every other stored kind (§4, mapped by §10.4) are the
  audit trail's, and a hook for each keystroke-grained event would be noise on the agent's
  critical path.
- **`renamed`** is offered when a session's name changes. Its `event.reason` (and
  `DECK_EVENT_REASON`) is the rename's **source**, the same text the stored `renamed` event
  records (§4): `user` for a rename typed in deck (the `i` dialog), `harness:<kind>` (for
  example `harness:claude`) for a name deck adopted from its harness (§8 "Adopting a harness
  name"). The new name is `session.name` and `event.message`; the tmux slug is never part of it.
  It is not in the default `event_hook_events`, so it spawns only when a list names it.
- **NUL is stripped.** A NUL byte in any string exported in the environment or the payload (the
  session fields, the offered kind, both reasons, the message, the deck host and version) is removed
  before anything else, because a NUL in an environment entry makes the OS refuse the whole spawn; the
  event is delivered with the rest of the text intact, attached and detached alike, and its
  `(kind, reason)` pair (§10.3) is claimed once as for any other event.
- **Environment:** every `DECK_SESSION_*` variable of §6.1, exactly as `pre_launch` sees them
  (always exported, empty rather than absent), plus `DECK_EVENT_KIND`, `DECK_EVENT_REASON`
  (the `status_reason`, e.g. a notification type), `DECK_EVENT_MESSAGE` (the last assistant
  message or crash summary, truncated and redacted per §6.4) and `DECK_EVENT_AT` (RFC 3339).
- **The name is untrusted.** `DECK_SESSION_NAME`, `session.name` and, for `renamed`, `DECK_EVENT_MESSAGE`
  and `event.message` carry a name the harness reported (§8 "Adopting a harness name"). deck cleans
  it (escapes, controls and invisible characters removed, at most 200 runes) but it is still
  text an outside program chose, and may hold quotes, `$(...)`, backticks or `;`. A hook script
  must not interpolate it into a shell string (`sh -c "... $DECK_SESSION_NAME"`, `eval`, an unquoted
  expansion): read it as data, from a quoted variable or the JSON on stdin. The event message is
  redacted per §6.4 like any other, so a secret-shaped `KEY=VALUE` in an adopted name is masked there.
- **stdin:** the full payload as one JSON object, versioned:
  `{version, session: {id, name, cwd, agent, status, reason, permission_profile, group,
  important}, event: {kind, at, reason, message}, deck: {host, version}}`. A script that only
  needs the kind reads argv and never touches stdin.
- Bodies are size-capped. **Env values never appear** in the environment variables above, in
  the payload, or in the captured output record (§6.4); `message` is withheld, not truncated,
  for a `sensitive` session (§8).
  Redaction masks the value of every secret-shaped `KEY=VALUE` pair in the message (the key
  may be quoted, as in `{"api_key": "v"}`, or quoted with `\"` as in JSON serialised into a JSON
  string, `{\"api_key\":\"v\"}`, at any level of escaping and with or without space around the
  colon, where the value is masked through its matching escaped quote, also when it contains
  whitespace or ends in backslashes, and the text after it stays; a `KEY: VALUE` pair counts too) and of these other
  shapes, wherever they appear in the message, the reasons and the captured output tail: the
  token after `Bearer` (`Authorization: Bearer <token>` or `Bearer <token>` alone); the value of
  a flag whose last word is `token`, `secret`, `key`, `password`, `passwd` or `credential`
  (`--token abc`, `--password=abc`, `--api-key abc`; `--tokens-per-minute 5` is not one); a body
  of at least eight characters behind `sk-`, `ghp_` (also `gho_`, `ghu_`, `ghs_`, `ghr_`),
  `github_pat_` or `AKIA`; and a three-segment JWT (header.payload.signature, the header being base64url JSON in any legal
  spacing, so `{ "alg"…` encoded as `eyAi…` counts as much as `eyJ…`; dotted prose whose first segment is not a JSON object is left alone). A quoted value (a pair's, a quoted key's, a
  flag's or a `Bearer` token's) is masked through its real closing quote: a backslash escapes the byte after it, so
  `"ab\"cd"` and `"a\\"` are each one value and no suffix is left in the clear. Every shape is
  found in the original text and overlapping matches are masked as one span, so another shape
  inside a quoted secret value (`--password "a\" b Bearer c"`) is part of that value and never
  takes its closing quote. A secret pair is found inside a non-secret pair's value too
  (`error: GITHUB_TOKEN=abc` masks `abc`). The same masking
  feeds `DECK_EVENT_MESSAGE`, the stdin JSON and the stored script-output tail. Anything else is
  left byte-for-byte as it was. Redaction also removes
  every non-empty session env value, of any length, by value, from the message and from the
  captured output tail: there is no length floor, so a one-character value is removed too.
  `deck _hook` runs inside the agent and so inherits the session env, under its own keys and
  under any other name that copied a value; the inherited environment is therefore filtered by
  value, not only by key: an inherited variable whose name is one of the session's env keys, or
  whose entry (name or value) contains any non-empty session env value, of any length, is not
  passed on to the script at all. A short value can so withhold an ordinary variable such as
  `PATH` from the script (deck still finds the script itself on its own `PATH`), so a script
  that needs one sets it itself. The attached and the detached (session-end) spawn apply the
  identical rule, with no length exception. The same by-value removal applies to every other string exported in the
  environment or the payload (the session fields, both reasons, the deck host and version) and
  to every not-started error's text, the detached spawn's payload-file failure included, so
  neither the stored result nor the `deck.jsonl` line carries a value; both reasons also get the
  `KEY=VALUE` masking. Only deck's own fixed vocabulary is exempt, because §10.1 fixes it: the
  variable names, the payload's keys and `version`, the offered kind and the RFC 3339 `at`; a
  value that happens to occur inside one of those is coincidence, not a copy of the session env.
  The spawner is `internal/notify`, which is handed the env only to scrub it.

### 10.2 Per-session control

| field | meaning |
|---|---|
| `event_hook_enabled` | tri-state: inherit (default) · on · off |
| `event_hook_events` | optional list of offered kinds; absent = inherit the global list |

- The global defaults are `event_hook_default` (on/off for a session whose own flag is
  *inherit*; **default off**) and `event_hook_events` (the default list; default
  `["waiting", "error", "ended"]`), both in §6.5 and both editable in settings (§11.5).
- A session's list **replaces** the global one entirely. There is no partial merge — merge
  semantics are a support burden and a debugging trap.
- **deck filters before it spawns.** An event whose kind is not enabled for the session never
  starts a process. Anything finer — only `important` sessions, quiet hours, a per-agent
  rule — is the script's to decide from `DECK_SESSION_*` and the clock; deck has no
  `quiet_hours`, no `only = "important"` filter and no snooze that suppresses a hook.
- Both fields are set in the create dialog and in the launch-inputs editor (§11.4), and
  **apply immediately**: unlike a launch input they are read at dispatch, not consumed at
  launch, so they are not restart-to-apply and set no dirty flag (§6.2).

### 10.3 Delivery

- **Fire-and-forget.** The script runs under a bounded timeout (`event_hook_timeout`, default
  3 s) and is killed — its whole process group — when it expires. deck keeps **no outbox and
  never retries**: a script that wants retries owns them.
- The **exit status and a capped tail of stdout and stderr** are recorded against the event
  (the `hook_*` columns of the event's own row, §4, written after the spawn returns: the row
  itself is committed first, and a script that cannot be started records its error instead)
  and shown in the session detail and the health view (§11.4). A non-zero exit or a timeout is
  a visible fact on the row, never a silent no-op. The health view also probes that the
  configured script exists and is executable, alongside `PATH`.
- **Every spawn is also one structured-log line** (`"event": "event_hook"` in
  `$DECK_HOME/log/deck.jsonl`, §13.1): the session id, the offered kind, the exit status
  (`-1` for a timeout, a signal or a script that never started, with that error's text), a
  `timed_out` flag, and `script_duration_ms`, the script's own monotonic run time. The
  output tail is never logged; it lives only on the event row. A detached session-end spawn
  (below) logs `"detached": true` and no exit status or duration, because deck never waits for
  either. An offer that spawns nothing (inert, disabled, unlisted, deduped) logs nothing.
- **`renamed` is never deduped.** A rename is a fact and not an attention episode, so every
  rename spawns the hook again, even for a source that already fired in the epoch, and no pair
  is added to `hook_fired`.
- **Dedupe by epoch, not forever.** `notify_epoch` increments whenever the session leaves an
  attention state (§7). Within one epoch, a `(kind, reason)` pair spawns the hook once: a
  re-fired prompt in the same attention episode is one spawn, and the same prompt tomorrow is
  a new epoch and spawns again. The set of pairs already fired in the current epoch is
  `hook_fired` (§4), cleared when the epoch advances. A permanently unique key would silently
  mute a recurring prompt for the lifetime of the session.
- **No debounce, deliberately.** "Suppress if it resolves within 20 s" needs something awake
  20 s later to fire the survivors; with no daemon the only thing that would ever wake up is
  the next hook — which, for a session blocked waiting on you, never comes. So dispatch is
  immediate, and a prompt you answer in three seconds still pinged you.
- The hook never suppresses or delays the event log: the event is written first, and a
  failing or slow script cannot lose it.
- **Skipped on the session-end path.** `deck _hook` never waits for a script while shutting a
  session down (§8.1); an `ended` event for a session-end payload is dispatched detached,
  with no timeout record, rather than holding the agent's exit.

### 10.4 Who spawns it

**Whichever deck process records the event**, through one dispatch function called from every
event-write path: `deck _hook` for the payloads a Claude or Codex hook, or the Pi extension, delivers, and the
running TUI for events deck detects by itself — a probe-classified shell status change (or a Pi change its extension has not reported),
a reconcile-detected process death (a retained failed pane offers `error`, a tmux session that
disappeared cleanly offers `ended`), and the user's own `killed`. A process death that `deck _hook`'s
post-hook liveness pass (§3.1) detects is dispatched by that `_hook` the same way, so an unattended
crash still fires. It is spawned once the pass has returned, so the pass's short reconcile
budget neither cuts the script short nor is spent by it: the script is bounded by
`event_hook_timeout` like every attached spawn. When one pass records several deaths their
offers run concurrently, each claimed, spawned and recorded on its own event, so the whole
batch holds the caller for one `event_hook_timeout` however many sessions died, never one per
session. `deck _hook`'s own event is claimed (durably, before anything spawns) first and its
script then runs *while* the liveness pass and that death batch run, not before them: the
agent-facing call is held for at most one `event_hook_timeout` plus the reconcile budget
(about 3.5 s at the default 3 s), never a full incoming timeout followed by a second full
death-batch timeout, so it stays inside the 5 s hook limit the Copilot plugin installs
(§8.4). `_hook` returns only after every one of those scripts has finished or been cut at
its timeout and its result is recorded. Each of those writes its
event row first and then calls the dispatch function with the row's seq, so the result is stored
against that row; the running TUI reads the four event-hook settings per event, so a save or a
config reload applies to the next one. A probe verdict that only repeats the status already on
the row is not a change and dispatches nothing, and a script's failure never fails the kill, the
probe or the reconcile pass that recorded the event. Dispatch never happens twice
for one event: the dedupe above is the guard, and the process that wrote the event is the one
that dispatches it. `deck _hook` calls the dispatch function for every payload it records
(Claude, Codex, Pi and Copilot alike) once the status update and the event row are committed,
and only for a change that landed: an unresolved (orphan) payload, one from a superseded launch,
an identity mismatch, a status write that lost to a higher-precedence source, and a SessionEnd
that only ended a conversation inside a live pane (`/clear`, `/resume`) offer nothing. The
`hook_fired` pair is claimed with one atomic add of exactly that pair before the spawn, so two deck
processes recording the same event spawn once, while a different (kind, reason) pair recorded at the
same time in the same epoch is never suppressed by the contention: a dedupe skip means that very pair
was already claimed in that epoch. The session-end payload's `ended` event is
started detached (`notify.Start`): payload on stdin from an already-unlinked temporary file (never a
pipe, so handing over a payload of any length never waits on the script reading it), output
discarded, no timeout and no recorded result, in its own process group.

**Which kind a recorded change offers.** The stored `events.kind` values (§4) are not the
offered kinds (`session_start`, `stop`, `session_end`, `probe.<status>`, ... are what writers
store, and they are never renamed), so the dispatch function maps the stored kind, and for a
session start its reason, to the offered kind. A stored kind this table does not name offers
nothing and never spawns:

| stored `events.kind` | written by | offered kind |
|---|---|---|
| `session_start`, reason `resume` | hook | `resumed` |
| `session_start`, reason `compact` | hook | none (a compaction is not a start) |
| `session_start`, any other reason | hook | `started` |
| `notification` | hook | `waiting` |
| `permission_request` | hook | `waiting` |
| `stop` | hook | `idle` |
| `stop_failure` | hook | `error` |
| `session_end` | hook | `ended` |
| `probe.waiting` | TUI probe | `waiting` |
| `probe.idle` | TUI probe | `idle` |
| `probe.error` | TUI probe | `error` |
| `tmux.pane_dead` | reconcile | `error` |
| `tmux.session_gone` | reconcile | `ended` |
| `killed` | the user | `killed` |
| `renamed` | the user, or adoption from a harness | `renamed` (reason = the source, above) |

Every other stored kind (`user_prompt_submitted`, `error_occurred`, `probe.running`,
`launch.ready`, `launch.failed`, `restart`, a `.superseded` or
`.identity_mismatch` hook, ...) offers nothing. The mapping is `notify.OfferedKind`; the filter
and the dedupe above are `notify.Dispatch`, which runs its checks in the §10.1-§10.3 order
(script, offered set, enabled flag, effective list, epoch) and reads no per-session field
until the script and the offered set have passed.

The three limits this accepts, all of which belong in the help view rather than in a footnote:

1. **There is no retry**, and so no outbox to drain: a hook that fails at 02:00 has failed,
   and the only record is the exit status on the event.
2. **Probe-classified agents fire only while a TUI runs.** Shell sessions have no
   event source of their own (§8), so unattended they change status — and therefore fire the
   hook — never. Claude, Codex and Pi sessions fire unattended (Codex from its first prompt on,
   §8.2; Pi from its first extension event, §8.3, and only for the transitions that table
   lists). Only Claude reports turn and API failures that way, via the stop-failure hook:
   Codex has no equivalent event, so an unattended codex failure waits for a TUI.
3. **Process death is detected late.** A `SIGKILL`ed or OOM-killed agent of any kind fires no
   hook, so its `error` event waits for the next TUI tick or the next `_hook` of any session (§7).

The help view (`?`) carries this contract under "Event hook": argv, the environment names,
stdin, the timeout, no retry, the epoch dedupe and the three limits above, and the README
carries a Telegram (`curl`) and a desktop (`notify-send`) example script with the idempotency
and no-retry notes. Both are pinned by tests (`internal/tui/event_hook_help_test.go`), the
scripts by a `sh -n` syntax check.

### 10.5 Out of scope

**Inbound remote control.** Replying into a session from a phone would require a
long-polling daemon and a service-specific protocol, contradicting both the no-daemon and
service-agnostic constraints. The hook is one-way. If it's ever wanted, the natural shape is a
separate program that writes to deck's store — not deck growing a listener.

---

## 11. TUI

```
╭ deck ─── 2 waiting · 1 error · 7 sessions ─────┬ ◐ perf-sweep ── claude · safe ─────────╮
│  service-a              ~/work/service-a       │ ~/work/service-a · conv 4f9c…a21       │
│ ● api-refactor   claude  live    waiting 2m    │                                        │
│ ● flaky-tests    claude  live    waiting 6m    │ > run the benchmark suite              │
│ ◐ perf-sweep     claude  live    running 4s    │   ⠋ bench/throughput … 14/31           │
│ ○ dep-audit      codex   live    idle   31m    │                                        │
│  infra                  ~/work/infra           │ (live pane capture, escapes preserved, │
│ ✗ tf-migrate  yolo claude live   error   1h    │  1 s tick, selected row only — never   │
│ ■ notes           shell   —      stopped 2d    │  interactive; ↵ for a real terminal)   │
│ ■ triage     env↻ pi      sampled stopped 4d   │                                        │
│                                                │                                        │
╰────────────────────────────────────────────────┴────────────────────────────────────────╯
 ↵ attach · space next · n new · r resume · x kill · , settings · f find · ? help
```

That frame is an illustration drawn at 91 columns with the sidebar widened to 49; the
default width, the floors, and what happens at deck's 80-column minimum are §11.2's, not
this drawing's — and a session's real **two-line** composition is stated in the bullets
below rather than drawn here, because at the default 35 columns it does not fit on one.

The shape is a **session sidebar beside a live preview**, not a full-width list. The
sidebar is the permanent spine of the product — it is what you scan to answer "which
session needs me" — and the preview is what makes an answer actionable without attaching.
Both are described below; §11.2 covers what happens when the terminal is too narrow to
hold them side by side.

- **Grouping is by manually defined groups, never derived from the filesystem.** A group is a
  name the user chose — `sprint work`, `tooling maintenance` — held in §4's `groups` table with
  membership on `sessions.group_id`. Deriving groups from `cwd` (deck's earlier behaviour)
  grouped by an accident of where a session was started, which is not how anyone organises
  work; and never by repo, which was never the shape either.
  - **`default` is not a row.** It is `group_id IS NULL`, which is what makes "it always
    exists", "it is always last", "it cannot be renamed" and "it cannot be deleted"
    structural facts rather than four rules to enforce. A session belongs to `default` until
    the user says otherwise, and a `group_id` that no longer resolves — another client deleted
    it between this client's load and its render — renders under `default` rather than
    vanishing.
  - **Order is alphabetical, case-insensitive, with `default` last by default** regardless of
    where its name would sort — or **first**, when `[ui] default_group_first` is set (§6.5).
    Rows *within* a group follow the sort order below. Group order
    is deliberately not attention-ranked: a manual group is a stable place the user learns the
    position of, and a list whose headers reshuffle when a session starts waiting is a list
    you cannot navigate from memory. The cost is real and accepted — a `waiting` row can sit
    below the fold in an alphabetically-late group — and it is paid for by the same two things
    that make a non-`attention` sort order safe: the attention-walk key and the collapsed
    strip's count.
    - **`default_group_first` moves one pivot and nothing else.** It is the single exception
      to "not attention-ranked", and it is not one: it is a *stable* user choice, made once,
      that says where the ungrouped sessions live. For a user whose ungrouped sessions are
      the ones they work in, pinning them to the bottom of the sidebar puts the most-used
      list furthest from the eye. The remaining groups stay alphabetical either way, so the
      positions the user has learned do not otherwise move. This is a preference, not
      machine-local state, so it lives in `config.toml` beside `sort_order` and not in
      `ui_state`.
  - **Every header carries its member count, including `(0)`.** A group the user defined but
    has not filled yet still renders: it is how they see the group exists and where to put
    something. Under an active filter (§11.10) only groups with a match render, and the count
    is what is shown — an empty group is a standing place in the default list, not a row to
    pad filter results with.
  - **Groups collapse; rows do not.** A header click toggles collapse (§11.8), and
    **collapse state persists in `ui_state`** so a group collapsed
    yesterday is still collapsed today — a durable named group is not a transient view the way
    the old derived buckets were. Selection never lands on a hidden row.
    - **A group header is a cursor stop.** `↑`/`↓` and the rest of §11.3's list navigation
      land on headers as well as on session rows, and `c` folds or unfolds **the header under
      the cursor** (`←`/`→` fold and unfold explicitly). Resolving the fold target from the
      *selected session's* group instead cannot express the operation at all: a folded group
      has no visible row to select, so the group the user most wants to reopen is the one
      group they cannot name, and a defined-but-empty group can never be reached by keyboard
      at any time. A header cursor is also what makes the empty group's standing place above
      usable rather than merely visible.
    - **Folding never evicts the cursor from the group it folded.** Folding from a session
      row leaves the cursor on that group's own header, so the same key immediately undoes it;
      folding from a header leaves the cursor where it is. A fold that pushed the cursor into
      the *next* group would turn one key into a walk down the sidebar, folding everything.
    - **Every session-scoped key is inert while the cursor is on a header** — it names no
      session, so `↵`, `a`, `x`, `dd`, `i` and the rest do nothing rather than acting on some
      nearby row the user did not point at. This is one rule for all of them, not a decision
      each binding makes for itself.
    - **A header starts at the panel's first content column**, level with the sidebar's own
      `socket:` line — it reserves no gutter. §11.3's gutter belongs to session rows, which
      is where `>` and `✓` are drawn; a header carries neither, so reserving their columns on
      it only indented every header for nothing. The cursor on a header is shown instead by
      the `selection` background across the header's full inner width **and reverse video
      over the header's text**, so the cue survives `NO_COLOR` and `ascii` (an attribute, not
      a colour), and a header's width and left column never shift as the cursor moves on or
      off it.
  - **Membership is set where the session is.** The create modal has a `Group` field that
    cycles the available groups exactly as the `Agent` field cycles kinds (§11.4), defaulting
    to the last group created into; the `i` detail dialog moves an existing session between
    groups. The marked set is **not** extended to moves: `x` and `dd` remain the only batch
    verbs.
  - **The group list is edited in settings (§11.5)**, which is where deleting one lives too.
    Deleting a group asks which of two things to do with its members — move them all to
    `default`, or delete them — and the destructive branch is `dd`'s own batch path (§9.2),
    confirm dialog, tombstone, transcript-purge offer and single-`u` restore included, never a
    second implementation of deletion. An empty group needs no prompt. deck never mass-kills
    sessions as a side effect of a list edit.
- Sort: **configurable**, `[ui] sort_order` (§6.5), four values, `attention` the default:
  - `attention` (default) — `waiting` (oldest first, seen or not) → unseen `error` → `running` →
    `starting` → `idle` → seen `error` → `stopped` (seen means `seen_status_at = status_at`, §7). This is the order every earlier phase shipped and the one the list
    exists to provide; it stays the default precisely because it is the order that answers
    "which session needs me".
  - `created` — newest first, by `sessions.created_at` descending.
  - `activity` — most recent output first, by `sessions.last_output_at` (§7's output clock)
    bucketed by `[ui] output_bucket` (10 s by default), newest bucket first, the session `id`
    ascending breaking a tie inside one bucket. Two sessions that printed within the same
    bucket therefore keep a fixed relative order from tick to tick, and a chatty pane moves
    its row at most once per bucket. A session that has not printed since deck began
    recording (`last_output_at` 0) sorts last. A status change does not move a row in this
    order. The order follows `output_bucket` live.
  - `name` — `name` ascending, case-insensitively, so `Api` and `api` sort together.

  **Pinned sessions come first within their group, in every order.** `p` pins or unpins
  (§11's pin rule below); a pinned row (`pinned_at ≠ 0`) sorts above every unpinned row of
  the same group, `waiting` and `error` rows included — a pin is the user saying "this one
  first", and an attention tier that outranked it would make the pin a suggestion, exactly
  as a hidden status tier would make a non-attention order one. Inside each tier the
  configured order above applies unchanged, including its `id` tie-break, so the order stays
  total. A pin never moves a row to another group and never reorders groups. Attention stays
  reachable regardless: the attention-walk key and the collapsed strip's count are
  order-independent and still reach a `waiting` row below a pinned one.

  Every order is total: each falls back to `id` ascending on a tie, so a re-sort can never
  swap two rows out from under an in-flight keyboard idiom (§11's marked-set navigation
  depends on this, and a coin-flip tie-break has already caused one defect — see the
  stable-sort rule below). A non-`attention` order does **not** re-rank by status at all:
  the whole point of choosing one is that the user, not deck, decides what "first" means, and
  a hidden status tier would make the chosen order a suggestion. Attention itself remains
  reachable in every order through §11's own attention-walk key and the collapsed strip's
  count, which are what make a non-attention order safe to offer rather than a way to lose a
  waiting prompt. Sort order is *preference*, not machine-local UI state: edited in settings
  (§11.5) with an explicit save, so §6.5's rule that a keypress never rewrites `config.toml`
  still holds, and it is not in `ui_state`. The **group list**, by contrast, is machine-local
  and lives in `state.db` (§4) — a group is a place on this machine, not a preference that
  travels with a config file.
- **The viewport follows the cursor, always.** Every key that moves the selection — `↑`/`↓`,
  `PgUp`/`PgDn`, `g`/`G`, the attention walk, a fold, an edit to the `/` query — leaves the
  selected row inside the visible window, keeping one whole row of context beyond it where the
  list allows and sitting flush at the list's own ends where it does not. A sidebar that
  renders no visible selection is a sidebar whose next keystroke acts on a session the user
  cannot see, which is the same class of defect as a mis-aimed click (§11.8). The wheel is the
  one deliberate exception and stays one: it scrolls without selecting (§11.8), so it may drift
  away from the cursor — and the next selection move brings the cursor back into view rather
  than the wheel's position being preserved. **A drift is kept until the user acts, not until
  deck does.** A background reload, re-sort or re-group while the list is drifted keeps the
  wheel's offset (clamped to the new length) and does not snap back to the selection, which
  still follows its session by identity off screen — a wheel scroll that lasted only until
  the next tick would be no scroll at all. What ends a drift is the user's next key that
  either moves the selection or **acts on it**: every session- or header-scoped key (`↵`,
  `a`, `F`, `x`, `dd`, `A`, `r`, `i`, a mark, a fold and the rest) first brings the selection
  back into view and then does exactly what it does today, so any confirmation it raises is
  raised over a list where its target is visible. Keys that name no selection (`?`, `q`,
  `<`/`>`, settings) leave the drift alone. **While §11.9's interactive mode is active, input
  to the live pane is acting:** a keystroke or paste that interactive mode forwards first
  brings the selected row — the interactive target — back into view, then is forwarded
  exactly as it would have been, on the same press, with nothing swallowed; `Ctrl+Q` leaving
  interactive mode brings it back too. Reading is not acting: a wheel over the interactive
  preview (its scrollback), a drag-to-copy, and a background reload, re-sort or re-group leave
  the drift alone.
- **A re-sort never moves the selection.** Selection follows the session, not the row index —
  whatever was selected before a reload, a re-group or a sort-order change is still selected
  after it, and the viewport scrolls to keep it visible rather than the selection sliding to
  whatever now occupies that index. This is stated here because the sort is now user-visible
  and switchable at runtime: changing the order is a view operation, not a navigation one.
- **A newly created session is selected as soon as it appears.** Creating a session is an
  explicit act aimed at that session, so the list selects it on the load that first contains
  it and scrolls it into view, in every sort order and whether or not grouping is on. This
  matters most in `attention`, where a brand-new `starting` row sorts fifth of six and can
  land off-screen: a create whose result the user then has to hunt for is a create that did
  not finish. It is a one-shot intent tied to that session's id, not a standing rule — it is
  satisfied once, by the first load that contains the row, and a later reload does not
  re-steal a selection the user has since moved. If the session never appears, nothing moves.
  **The creating client then enters the interactive preview on it** (§11.9), unless
  `[ui] attach_on_new` (default true, §6.5) is false: a create is aimed at working in that
  session, and a create that stops one `↵` short of it is again a create that did not finish.
  This is deck's one *auto-enter* intent, tied to the session's id, and a fresh create from the
  `n` modal is one of exactly two things that arm it; the other is an `r` or `R` under
  `[ui] attach_on_resume` (§9.1), and both behave identically from here on. It is armed only in
  the client that asked; another client on the same socket never enters on its behalf. It
  waits for the row to have a live pane, because a new row often lands as `starting` before
  tmux reports one, and the wait is bounded: if no live pane appears within a few preview
  ticks, the intent is dropped silently and the session simply stays selected. The user always
  wins — any key, any click, a selection move, or a dialog, overlay, filter or settings opened
  before entry cancels it for good, and nothing starts while the create modal is still open.
  Entry is `↵`'s own entry, so a refused one is `↵`'s refusal, shown the way `↵` shows it,
  with the session left selected in the list and no retry.
- **A session is a two-line row, and the order within each line is fixed.** Line 1 carries
  the §11.3 gutter, the status glyph, the pin marker `✦` (ASCII `*`) on a pinned row only,
  the name, then the unseen marker, the live/sampled
  quality badge (§3) and the status word. Line 2 carries the gutter, `env↻` and `launch↻`
  when either is dirty, the row's age, the kind word of an agent that declares one (only `copilot`
does; `claude`, `codex`, `pi` and `shell` rows carry no agent word and render exactly as they
always did), and the permission badge for non-`safe` **last**. The
  transient badges come before the age because they are news; the permission badge is
  standing configuration and goes at the end, where it is still visible without displacing
  anything that changes.
- **The age is rendered bare** — `2m ago`, `just now` — with no `created` label. It is the
  only timestamp on a row, so the word says nothing the column position does not, at a cost
  of eight of the sidebar's ~33 content columns on every row in the list.
- **The marked set is shown in the gutter, never as a text badge.** A `✓` on the second line
  of §11.3's gutter bar, with the selection arrow on the first, so both cues coexist. A badge
  at the end of line 1's badge run is the first thing truncation drops, which loses the cue
  precisely when the sidebar is too narrow to count marked rows by eye.
- **The sidebar pin.** `p` on a session row toggles its pin: it sets `pinned_at` to now, or
  back to 0, as one targeted `UPDATE` (§4). There is no dialog and no confirm — `p` again is
  the undo. With a marked set, `p` acts on the whole set: it unpins them all when every
  marked row is pinned, and pins them all otherwise. A pin is machine-local list state, like
  the group list: it lives in `state.db`, never in `config.toml`, so the keypress rewrites no
  file (§6.5). It survives restart, kill and resume, rename, archive and a group move, and a
  `dd` restore brings the row back pinned, because the pin is a column on the row itself. A
  pin records no event: it is how the list is arranged, not something that happened to the
  session. A pinned row shows `✦` before its name, drawn in the `accent` token; under
  `NO_COLOR` the glyph alone carries it, as the status glyphs do. `i` shows `pinned: yes`/`no`
  and also toggles it with `p`.
- **The unseen marker (R324) is `◆`** (U+25C6, one column, EAW `Ambiguous` like the status glyphs), ASCII `!`,
  drawn after the name in the row's status token. It is distinct from the `●` waiting status glyph, which
  it used to share (the old marker was `●` too, so an unseen waiting row drew `●` twice). Neither
  form collides with any status glyph (`? ~ o . # x`, `-`), the pin (`✦`/`*`) or the selection cue (`>`).
  The sidebar title carries the count as `deck — sessions · N unseen` (ASCII `deck - sessions - N unseen`),
  shown only when N is at least 1; N counts the rows that carry the marker (§7).
- **What counts as viewed (R325).** A row is *viewed* when the passive preview has been visible on it for
  at least 1.5 s, or when interactive mode is on it (no dwell: interactive mode is the view). *Visible*
  is the same test as the R322 dwell's: the row is the one the preview shows (the selected row, or the
  interactive target), the preview panel is on screen (stacked below its floor it is collapsed away and
  views nothing), no modal or takeover covers it, the pane's capture is the row's own and live, and the
  terminal window has not reported losing focus. While a row stays in view the preview tick keeps
  copying `last_output_at` into `last_seen_at` (`Store.MarkViewed`, a write only when the two differ,
  once per new output bucket), so a watched session never goes unseen. Arrowing away from a row, a
  collapsed preview or a blurred window ends the clock; coming back starts a new 1.5 s, so rows passed
  over while arrowing down a list stay unseen and only the row the cursor rests on is cleared. Viewing
  writes no status, no `seen_status_at` and no event, and moves no row; it is independent of the
  R322 prompt dwell, which sets status-seen only. Tests: `TestPreviewVisibleForDwellMarksRowViewed`,
  `TestInteractiveModeOnARowMarksItViewed`, `TestWatchedSessionDoesNotGoUnseen`,
  `TestCollapsedPreviewViewsNothing`, `TestUnfocusedDeckViewsNothing`,
  `TestArrowingDownUnseenRowsLeavesPassedOverRowsUnseen`, `TestPreviewTickRunsTheViewingTrigger`.
- **Unseen jumps (R326).** In the list, `Alt+U` selects the next unseen session in list order from the
  current row and `Alt+Shift+U` the previous one, both wrapping through the painted list (visible rows
  only; the current row is reached last when it is the only unseen one). *Unseen* is the one predicate
  the `◆` marker and the `N unseen` header use, so a **seen `error` row with output not yet viewed is
  reachable until it is viewed**; `space` keeps its own, status-only meaning (a `waiting` row or an unseen
  `error` row) and never lands on a row that is merely unseen. With nothing unseen a one-line footer
  message (`No unseen sessions`) is shown and the selection stays. Landing views the row exactly as
  R325 does (`last_seen_at` takes `last_output_at`, once) and nothing else: no `seen_status_at`, no
  status, no event, and no row moves. **Decoder:** tmux keeps the case of `M-u` and `M-U` (the bytes are
  `ESC u` and `ESC U`) and the decoder names them `alt+u` and `alt+U`, so `Alt+Shift+U` is distinguishable
  from `Alt+U` and the gap `Alt+Shift+Tab` has (§11.9: tmux discards that modifier) does not apply.
  Tests: `TestAltUJumpsToTheNextUnseenSessionAndOnlyViewsIt`,
  `TestAltShiftUJumpsToThePreviousUnseenSession`, `TestAltUAndAltShiftUWrapThroughTheList`,
  `TestAltUWithNothingUnseenShowsAOneLineMessageAndMovesNothing`,
  `TestASeenErrorIsReachableByAltUUntilItIsViewed`, `TestSpaceIsUnaffectedByTheUnseenJumps`,
  `TestAltShiftUIsDistinguishableFromAltUThroughTmux`. The interactive-mode form is R327.
- Status glyphs `●` waiting · `◐` running · `○` idle · `◌` starting · `■` stopped ·
  `✗` error · `▣` archived. Under `DECK_ASCII=1` / `[ui] ascii` each is one ASCII column: `?` waiting,
  `~` running, `o` idle, `.` starting, `#` stopped, `x` error. An **unverified** `running` row (§7, R315:
  3 × `stale_after` without an event, measured from the session's newest event, and 3 consecutive probe misses) draws `◔` (ASCII `-`) in the running
  glyph's column, in the running token; the status word and the sort position do not change. One column, always in the same column, so the shape of the
  list is readable before any text is. **No glyph deck renders may have East Asian Width
  `Wide` or `Fullwidth`** (`⏸` U+23F8, `⚡` U+26A1 and most emoji): those occupy two cells
  in some terminals and one in others, so a column-aligned list built on one silently
  shears on the other. EAW-`Ambiguous` codepoints are accepted — the rule cannot honestly
  be "single-width everywhere", because even `●` and the box-drawing borders are
  Ambiguous and no usable glyph set avoids them — with the caveat documented in help that
  a terminal configured for ambiguous-wide (some CJK setups) should use `DECK_ASCII=1`,
  whose fallback set is pure ASCII and immune. Badges that would want a pictogram
  (`yolo`, `env↻`) are text instead.
- Sidebar default width 35 columns, user-adjustable and persisted; the preview takes the
  rest.
- Preview: pane capture with escapes preserved (`capture-pane -e`), **250 ms tick**,
  selected row only. A passive tick is **one** tmux process however many sessions exist: it
  captures the selected session's pane by its target, with no `list-sessions` first, and
  a target that has vanished is the same transient, inert answer as before (the next
  reconcile tick corrects the selection). No passive capture runs while interactive mode
  (§11.9) is on — the grid is what the preview shows. Passive preview runs no PTY emulator — it is a capture, so it cannot be
  typed into; `↵` (§11.9) and `a` are the two ways to reach a real terminal.
- **The preview attaches no tmux client.** This is the load-bearing property, not an
  implementation detail: a second client sized to the panel reflows the shared window under
  §3.2's `window-size latest` and keeps re-expressing its own size thereafter, and the only
  escape — pinning the window — regresses every ordinary `↵` attach instead
  (`docs/spikes/tmux-embedded-preview.md` measured both). Capture reads. Where deck changes
  geometry it does so with an explicit `resize-window` it can account for, never as a side
  effect of holding a client open.
- **The preview fits the selected session's window to the panel** — `[ui] preview_fit`,
  default true (§6.5) — because the interaction model makes deck's panel the primary view of
  a session rather than a courtesy window onto someone else's. Three properties make that
  affordable: the fit
  is **coalesced against the preview tick**, so walking a list fits the row the user settles
  on and not every row passed on the way — but the coalescing is per *pane*, not per row:
  **a relaunch invalidates it.** `r`, `R` and the `u` that undoes a kill all replace the
  window with a new one at tmux's own default size, and they do it without moving the
  selection, so a fit already satisfied for the selected session must not suppress the fit
  the new pane needs. Otherwise the one row the user is watching is the one row that stays
  80×24 until they select away and back, which is the opposite of what coalescing is for.
  **Returning from a full attach invalidates it for the same reason**: `a` left the window at
  the attaching client's own size and detaching does not restore it (§3.3), so the row the
  user lands back on needs its fit again, without having moved the selection either.
  **A host terminal resize invalidates it as well**, and applies at once: a `WindowSizeMsg`
  that changes the terminal's size re-fits the selected session's window to the new panel box
  in the same update, with no key, click or selection change in between; a resize to the size
  deck already has is a no-op that issues no tmux call; a resize that arrives while a modal or
  the settings takeover covers the list is held and applied by the very update that closes the last covering screen (or lands the fit that was in flight), with no preview tick or other event in between;
  it is **skipped below §11.9's 7-row inner floor**,
  leaving the pane cropped, because a box that small has no transcript in it worth reflowing
  for; and it is **best-effort, owning and restoring nothing** — a session the user looked at
  is left at the size deck last chose, and any attaching client re-expresses its own size
  under `window-size latest` and simply wins. That last property has to be *given back*, not
  merely left untaken: `resize-window` writes `window-size manual` into the **window** options
  (§11.9), which shadow the global, so a bare fit would leave the window unable to follow any
  client at all — the next `a`, or any bare `tmux attach`, cropped into the panel's box for as
  long as the pin survived. **Passive fit therefore unsets the window-local `window-size` after
  fitting.** The size survives that (tmux sizes a window from its clients, and a window with
  none keeps what it was last given); only the hold goes. Best-effort does not extend to fighting an
  **owner**, though: passive fit stands down entirely, issuing no `resize-window` at all,
  while another live process holds §11.9's ownership claim on that window. An owner has
  recorded a size it is going to put back, so a passive fit landing under it is the one case
  where "deck last chose" is a lie — and without this, merely *selecting* a row in a second
  deck would resize a window the first deck is typing into. The capture still renders, at
  whatever size the owner is holding. It stands down for an **attached client** too, for the
  mirror-image reason: under `window-size latest` that client owns the size, so the only fit
  that could hold is a pinned one — the crop §3.3 forbids — and an unpinned one merely bounces
  a terminal somebody is watching through two reflows to land back where it started. So while
  `#{session_attached}` is non-zero passive fit issues no `resize-window`, and does the one
  thing that is still worth doing: **it unsets the window-local `window-size`**, releasing a
  pin an earlier fit (this deck's, or another build's on the same socket) left behind, so an
  attach that was cropped stops being cropped. Neither stand-down latches: both conditions are
  transient, and the row regains its fit as soon as the claim is released or the client
  detaches, without the user selecting away and back.
- **The cost is stated, not hidden.** A fit sends the agent `SIGWINCH` and reflows its output,
  so §3.2's history arithmetic applies: output produced while narrow consumes scrollback rows
  faster and evicted rows never return. A second client attached at another size sees the crop
  that implies (§3.3). Help states both. `preview_fit = false` restores a wholly passive
  preview — `capture-pane -e` poll, no resize, cropped bottom-left — for a user who would
  rather read a cropped pane than reflow an agent's.
- **A pane deck is not fitting is cropped — never reflowed to fake a fit.** The crop is
  anchored **bottom-left**: the newest rows, from column one, because that is where an
  agent's current activity and its prompt are. Lines cut at the right edge are marked, and
  the panel states the real geometry (`45×22 of 120×40`) so the user knows they are looking
  at a window rather than the whole pane. That form is a *crop* statement; a fitted pane
  states its geometry the way §11.9's interactive mode does, because the crop form would
  degenerate to `45×22 of 45×22` and read as a bug.
- **The preview has two modes, and they differ in input, not in geometry.** **Passive**
  preview is the default: a `capture-pane -e` poll, no attached client, no pipe, no
  keystrokes, no scroll, and the coalesced fit above unless `preview_fit` is off. §11.9's
  **interactive** mode is entered deliberately, per session, and adds the pipe, the cell grid,
  keystroke forwarding and its own bounded scrollback — with geometry it records, claims and
  restores rather than leaves. Reading a session may resize it; only interactive mode may
  *type* into it.
- **Cropping and elision are cell-aware, and never split a wide cell.** Foreign pane output
  is the one place deck cannot enforce its own no-wide-glyphs rule (§11's glyph list binds
  deck, not the agent), and a session name is user-supplied text. So where a crop or an
  ellipsis boundary falls inside a double-width cell, deck emits a **space** for that cell
  and never half a glyph: half a wide character is not a character, and a terminal handed one
  shifts every cell after it, which shears the panel border the frame is measured against.
- **The preview does not scroll.** It shows the pane's current visible content and nothing
  behind it — no capture history, no wheel scroll, no `PgUp`. `↵` is how you read what
  scrolled past, and it is one keystroke away. The alternatives are a second history
  mechanism alongside §9.4's, or a ring of deck's own frames with 250 ms holes in it, and
  neither earns its keep against a real terminal that already has scrollback.
- **A row with no live pane shows what deck knows, not nothing.** An `error` row renders
  the crash tail §7 captured, headed with the fact that it is the last output before the
  exit and is not live — the preview's best moment is answering *why did it die* without an
  attach. `stopped`, `archived`, and a `starting` row whose pane does not exist yet render
  a one-line placeholder naming the state. Stale bytes are never presented as live.
- Since there is no CLI (R7), **every** capability is reachable and discoverable in the
  UI: create modal (name — blank is filled in and recorded as `derived` provenance, §3.2 —, cwd picker, agent — available kinds only, §6.3 — permission profile, env, pre_launch,
  post_destroy, args), env editor, launch-inputs editor (§6.2), permission switcher,
  pin/unpin, rename, health
  view (tmux version, socket, agents on PATH, PATH resolvability, optional unit install),
  event log, search, **a settings view over every config key (§11.5)**, and a help overlay
  with the full keymap. A capability that can only be reached by editing a file by hand is
  a defect against R7, not a documentation gap.

Keymap: `↵` attach · `space` next needing attention (a `waiting` row or an unseen `error` row, §7) · `Alt+U` / `Alt+Shift+U` next / previous unseen session (R326, below) · `Y` mark seen (acknowledge; only where something is unseen) · `n` new · `r`
resume/start · `R` restart preserving conversation · `x` kill (undo toast) · `dd` delete ·
`s` send message (§11.1) · `i` session detail (§11.4 — **rename, the launch-inputs editor, the
group move, the permission profile `P` and the conversation lock `c` are actions inside it**,
not top-level keys) · `e` env editor · `p` pin/unpin in the sidebar (§11) · `E` event log · `f` find (§12) · `F` force-attach the interactive preview (§11.9) · `/`
filter list · `m` mark · `A` archive
(confirms, §9.2) · `U` unarchive (§9.2) · `u` undo · `g`/`G` top/bottom · `c` fold/unfold the
group under the cursor, `←`/`→` fold/unfold explicitly (§11) · `,` settings
(§11.5) · `t` theme picker (§11.6) · `|` cycle layout mode, `<`/`>` sidebar width (§11.2) ·
`?` help · `q` quit.

**Bookmarks (R275).** With the `bookmarks` setting on (§11.5), the list binds ten numbered
session slots. `Alt+0`…`Alt+9` jump to the session held in that slot: the jump moves the
selection only, and attaching stays a separate action. A slot is set to the session under the
cursor by the chord `Alt+M` then a digit `0`…`9`, which is layout-independent, and on a US
layout the ten `Alt+Shift+digit` keys (`alt+!`, `alt+@`, `alt+#`, `alt+$`, `alt+%`, `alt+^`,
`alt+&`, `alt+*`, `alt+(`, `alt+)` for slots 1…9 and 0) set a slot as a convenience. `Esc` or
any other key after `Alt+M` cancels the chord and is then handled as it normally is, whichever layer
handles it (the refusal banner's `Esc`, an open dialog, `Ctrl+Q`); a later bare digit never sets a slot. **Clearing (R286, R287).** `Alt+M` then `Delete`
clears every slot held by the session under the cursor, and only that session's; another session's
slots are untouched, and a one-line message names the cleared slot(s) (`Removed bookmark 3`, or
`Removed bookmark 1,5` for two). The chord prompt offers it only when that session holds a bookmark: it
then reads `Press Delete to remove bookmark N` (`N,M` for two slots) after the usual prompt, and for an
unbookmarked session it is the usual prompt unchanged and `Delete` is not offered. A `Delete` after
`Alt+M` on an unbookmarked session cancels the chord like any other non-digit key and is then handled as
it normally is; a `Delete` outside a chord behaves as it always did. The chord's session is fixed when
`Alt+M` is pressed (the session under the cursor in the list, the attached session in interactive mode)
and `Delete` clears that session's slots by identity: a background reload that moves the selection or drops
the session between the two keys never redirects the clear to another session. If the prompt offered
`Delete` and the session no longer holds a slot by then, the key is consumed with the message `Bookmark:
the session no longer holds a bookmark` instead of falling through onto the new selection. A jump to
an empty slot changes nothing and shows a one-line status message in the footer. With the
setting off none of these keys is bound in the list and every one reaches whatever it reached
before.

**Bookmark display and reveal (R277, R278).** With the setting on, a bookmarked row draws the
slot digit(s) it holds before its name, `[3]` for one slot and `[1,7]` for two, in the `badge`
token, and the footer and the `?` overlay list the bookmark keys (`alt+0-9/m bookmarks` and
`alt+m del clear bookmark` in the footer; the jump, the `Alt+M` chord, the `Alt+Shift+digit` keys and
the `Alt+M` then `Delete` clear action in the overlay). With the
setting off no slot digit is drawn and no footer or help line mentions bookmarks. A jump always
leaves its target selected and visible: when an active filter hides the target the filter is
cleared, when the target sits in a collapsed group that group is expanded, and a target in the
pin section or already visible is simply selected. A target that is not in the list at all
(archived and unmatched, or deleted) changes nothing and shows the one-line message.

While a session is attached (interactive mode, §11.9) and the setting is on, the same keys are
intercepted next to `Ctrl+Q` and never forwarded to the pane: `Alt+0`…`Alt+9`, `Alt+M`, and the ten
`Alt+Shift+digit` keys. A jump switches the attached view to the slot's session and stays in interactive
mode, without returning to the list; a jump to an empty slot or to a stopped session leaves the
current view in place and shows a one-line message. Every attached bookmark message (the `Alt+M`
prompt, a slot set, a jump and its refusals) shows in the interactive footer's right-aligned note
slot and clears on the next key. A slot is set on the session the attached view
shows. A bare digit completes a pending `Alt+M` chord, and so does `Delete` when the attached session
holds a bookmark: it clears that session's slots as in the list (same prompt, same message), is
intercepted and sends zero bytes to the pane. A `Delete` that does not complete the chord (the attached
session holds no bookmark, or no chord is pending) is forwarded to the pane exactly as before. A bare digit
is otherwise forwarded to the pane unchanged, as is every `Alt+<key>` outside the bound set (`Alt+A`, `Alt+X`, `Alt+.`). `Esc` or any other
key after `Alt+M` cancels the chord and is then forwarded as usual (`Ctrl+Q` leaves interactive mode and
cancels it too; leaving any other way drops it). With the setting off every one of
these keys is forwarded to the pane exactly as before. **`Ctrl+digit` is deliberately not bound:** a terminal only delivers it with user tmux
configuration, and for `2`…`8` it collides with legacy control keys (`Ctrl+2` is NUL,
`Ctrl+3`…`Ctrl+7` are ESC, FS, GS, RS, US and `Ctrl+8` is DEL).

**Every capability in this section has a key or a documented entry point here, and every
key here has a scenario (§13.5).** A capability listed above with nowhere to reach it is an
R7 defect, and a key listed here with no binding is a §11.3 defect — the two lists are
checked against each other, not maintained independently.

The mouse is a **shortcut over that keymap and never an alternative to it** (§11.8): click a
row to switch to it *and* enter its interactive preview, `Ctrl+Q` to come back out to the
list, wheel to scroll, drag the seam to resize the sidebar. Every mouse action names the key it duplicates, so the keymap above stays the
complete description of what deck can do.

Constraints: **80×24 minimum**, resize-safe at every size above it, and the degradation
path is specified rather than emergent (§11.2). No colour assumptions beyond 16 colours:
a theme's truecolour values are used when the terminal advertises truecolour and are
otherwise quantised to the 16-colour ANSI set, so **every theme remains legible on a
16-colour terminal** (§11.6). Usable without a nerd font — the glyphs above are all
BMP box-drawing/geometric characters, and an ASCII fallback exists for every one of them
(`DECK_ASCII`, §13.1).

### 11.1 Sending text without attaching

**Superseded by §11.9.** This protocol was narrow because typing into a full-screen editor
blind is dangerous — it is refused in `waiting` precisely because "a menu is on screen and
the keystrokes would blind-pick an option". Interactive preview removes that premise: the
user can see the menu and the caret. `s` is therefore not built; §11.9 is the answer to the
same need. The protocol is kept below as the record of what was specified, and of the
reasoning that still binds any future blind-send path.

`s` types into another program's full-screen editor, so the protocol is narrow on purpose:

- **Only from `idle`.** Refused in `waiting` (a menu is on screen and the keystrokes would
  blind-pick an option), in `running` (the input line may not exist), and in `starting`.
  The refusal names the reason and offers attach instead. There is no `--force`: if you want
  to answer a prompt, attach — that is one keypress.
- Delivery is literal (`send-keys -l`), wrapped in bracketed paste where the adapter
  declares support, followed by a single explicit submit keystroke. Single-line only;
  multi-line text is refused with a pointer to attach.
- Each adapter declares whether it supports send-without-attach at all. One that doesn't
  simply omits the action, rather than having deck guess at its input model.
- Every send is recorded as an event, so an unexpected agent reaction is traceable to it.

### 11.2 Layout modes

Three modes, one of which is always in force. `|` cycles them and the choice is persisted;
otherwise the mode is chosen from the viewport width, and a resize re-chooses it.

| mode | chosen when (`auto`) | sidebar | preview |
|---|---|---|---|
| `side-by-side` | width ≥ 80 — every supported width | left, `sidebar_width` (default 35), floor 24 | remainder, floor 40 |
| `stacked` | width < 80 — below deck's minimum, best-effort | full width, top, height `min(max(rows/3, 5), 12)` | full width, below, floor 8 |
| `collapsed` | user choice only, never automatic | 3-column strip showing `»` above the attention count, rendered vertically | everything else |

**All widths and floors in this table are total columns for that panel, borders and
padding included** — a floor already pays for §11.3's border and padding cells. At exactly
80×24 in `auto`, the mode is `side-by-side`: sidebar 35 total, preview 45 total, above its
40-total floor with the default sidebar. That frame — side-by-side, 35/45, at 80×24 — is
the golden minimum-size frame the harness asserts.

Every number above is a floor with a reason, and the reasons belong in the spec because
they are what stops a later change from quietly breaking a size nobody tests:

- **80 columns** is the boundary because it is deck's own supported minimum (§11). Unlike
  the tools this layout borrows from, deck does not target phone-width terminals, so
  `stacked` is not a peer mode: it is the honest degradation for a below-minimum terminal,
  kept because rendering *something* legible at 70 columns beats rendering a sheared
  side-by-side. A user may still select `stacked` deliberately at any width via `|`.
- **Preview floor 40** is the narrowest crop that still carries meaning. Agents print at 80
  columns or wider, so a 40-column window already shows half a line and asks the reader to
  infer the rest; below that the preview is worse than absent, so it is not shown.
- **Sidebar floor 24** is `glyph + name + status` with nothing elided; a narrower sidebar
  cannot answer the one question it exists to answer.
- **Stacked list 5–12 rows**: 5 is selection plus one neighbour plus the spinner row, so
  the list still conveys movement; 12 keeps a tall terminal from starving the preview. The
  8-row stacked preview floor is also §11.9's refusal threshold: measured against a
  Claude-shaped full-screen program, an inner box of 6 rows renders pure chrome and zero
  transcript, and 7 inner rows is the smallest usable box. A real agent that soft-wraps its
  input box needs more, so 7 is a floor, not a target.
- **Collapsed strip 3 columns** is the `»` glyph plus its two borders. It exists to give
  the preview the maximum possible width while keeping the attention count on screen —
  one glance still answers "does anything need me", and `|` restores the sidebar.

`|` cycles `auto → side-by-side → stacked → collapsed → auto`; the explicit modes pin the
layout regardless of width, and `auto` returns to width-based selection. When the sidebar
is focused, `<`/`>` adjust `sidebar_width` by one column, clamped to `[24, width − 40]`.
**Both `layout_mode` and `sidebar_width` persist in `state.db`, not `config.toml`** —
they are machine-local UI state, like window geometry, so a keypress never rewrites the
config file and §11.5's settings takeover remains the config file's only writer. A pinned
mode that cannot hold its floors at the current width falls back to rendering as `auto`
for as long as that is true, without overwriting the pinned choice.

Below 80×24 deck does not attempt a fourth layout: `auto` renders the stacked mode as far
as it fits and states in the footer that the terminal is below the supported minimum. A
truncated-but-honest frame beats an unpredictable one.

### 11.3 Panel chrome

- **Rounded borders** (`╭╮╰╯`) on every panel, dialog and overlay. One border style
  throughout; no mixing.
- **One character of horizontal padding** inside the sidebar and preview so content never
  touches a border. Dialogs manage their own internal spacing.
- **A single seam.** The sidebar draws top, left and bottom borders only; the preview draws
  all four, and its left border *is* the divider. Two adjacent `Borders::ALL` panels
  produce a heavy `││` seam that reads as two windows rather than one surface.
- **Focus is visible, and there are exactly two places it can be.** The sidebar is focused
  by default; §11.9's interactive preview is the second and only other stop, and it is
  entered by `Enter` rather than by a `tab` cycle, because a cycle would imply stops that do
  nothing. The focused surface's border uses the theme's `border_focus` token and the
  unfocused one uses `border`, so a dialog that opens takes focus and the sidebar's border
  reverts; the sidebar's selected row uses **`selection_idle`** while focus is elsewhere,
  which is what that token has always meant. **The seam is shared, and takes `border_focus`
  whenever *either* panel it divides is focused.** It is drawn by the preview (the single-seam
  rule above) but it is also the sidebar's own right-hand edge, and colouring it from its
  drawer alone produced a visibly wrong frame: a focused sidebar with three `border_focus`
  edges and a `border` one, which reads as a panel that is half-focused rather than as the
  focused surface. Ownership of the *glyph* and ownership of the *colour* are separate
  questions here, and only the glyph belongs to one panel. The corner T-junctions (`┬`/`┴`)
  where the seam meets the sidebar's top and bottom borders follow the seam.
- **A row highlight is a rectangle.** The selected row's `selection`/`selection_idle`
  background, and the alternating `surface` stripe, both fill the panel's **entire inner
  width** for **every line of the row** — from the first column after the left border to the
  last column before the seam, padding included — not merely the cells the row's text
  happens to occupy. A highlight that ends where the text ends makes a two-line row look
  like two ragged blocks of different widths, and makes the list's shape unreadable, which
  is the one thing §11's glyph column exists to protect. It also must not extend *past* that
  inner width: a background left open across the seam paints a column that belongs to another
  panel, so **every truncated coloured run re-emits its own reset** — truncation that drops a
  trailing SGR reset is a defect in the truncation, not a rendering trade-off.
- **deck paints its own canvas — both halves of it, never one.** The `background` **and `text`**
  tokens (§11.6) are painted together across every cell deck draws — borders, padding, row
  bodies, headers, the footer, dialog interiors, the empty state — rather than either of them
  left to whatever the terminal's own colours happen to be. A theme that only ever sets
  foregrounds is not a theme, it is a suggestion: a `light` palette on a terminal configured
  dark renders near-black text on near-black, and the only rows that stay legible are the ones
  the `surface` stripe happens to paint. That failure is not the stripe's fault and is not
  fixed by softening it — with the stripe removed, *every* row becomes unreadable rather than
  every second one. It also makes §11.6's contrast floor meaningful: a ratio computed against
  `background` is a claim about a colour pair no cell displays until deck paints it.
  The same argument runs in the other direction, and that is the easier half to miss: a cell
  given a background but **no foreground** inherits the *terminal's* default text colour, so
  its contrast is not merely low, it is **undefined** — the identical build reads clean or
  unreadable depending on a profile deck cannot inspect, and a light palette under a terminal
  defaulting to white text paints white on cream. Naming `background` while leaving `text`
  unnamed is therefore the same class of defect as naming neither, and a floor measured against
  a foreground no cell actually asserts measures nothing. Because a reset (`\x1b[0m`) clears
  both at once, painting the canvas means **re-opening the pair after every inner reset**, not
  prefixing one escape per line and hoping.
  **Captured pane output is painted under, not over — `[ui] preview_paint`, default `fit`.**
  A previewed pane's cells are the tenant's, and the earlier rule here was that deck never
  touched them at all. That rule was one layer short of the defect it was protecting against:
  a cell the agent left at the *terminal's default* asserts no colours, so its contrast is
  undefined exactly as the paragraph above describes, and "the tenant's colours surviving
  intact" was in those cells the terminal profile's colours, which are not the tenant's either.
  So deck paints its canvas **under** a capture, and the rule is stated per cell, by what the
  agent actually expressed:
  a cell at the terminal's default gets deck's `background`/`text` pair;
  a cell whose **foreground** the agent chose keeps that colour's **hue**, its lightness moved
  only as far as §11.6's floor against deck's background requires;
  a cell whose **background** the agent chose keeps it, with deck's `text` fitted against that
  background instead — the mirror of the previous case, since deck's foreground is already open
  on the row and "leave it alone" is not one of the available outcomes;
  and a cell whose **foreground and background the agent set both** is untouched in every mode,
  including undoing a fit deck made a moment earlier when the two arrived in separate sequences.
  Fitting moves lightness only, never hue: an agent's blue stays blue, which is how one agent's
  output is told from another's at a glance. Deck still paints the frame and the padding columns
  around a capture and emits a reset after it — the pane's colours must not leak into deck's
  frame either. `preview_paint = "nofit"` paints without ever adjusting an agent's colour,
  `"bg"` paints backgrounds only, and `"off"` restores the earlier rule exactly, byte for byte,
  for a user who would rather read an agent's colours exactly as chosen than have deck's theme
  reach into them at all.
  **An attached pane is painted `surface`, not `background`**, so the answer to "are my
  keystrokes going to this pane?" survives the paint: with both preview modes filled in the
  same tone that question would be answerable only from the border. `surface` is §11.6's
  "elevated region" tone, which is what an attached pane is; `selection` would be a larger step
  and is deliberately not used, because a pane filled with the selected-row colour devalues the
  cue that colour exists for.
- **The selected row has a gutter, not only a background.** (Session rows only: a group
  header has no gutter and carries its cursor cue as §11's header bullet states.) The row's leftmost columns are a
  painted bar in `accent`, carrying `>` on the row's first line with `background` as its
  *foreground*; §11's marked set puts its `✓` on the second line of the same bar, so a row
  that is both selected and marked shows both cues at once without either competing for a
  cell. A `selection` background alone is a low-contrast cue in several palettes — it is by
  design a close relative of `surface`, which the stripe already uses — and "which row am I
  on" is the single most-asked question of the list. The marker text lives in its **own
  columns**, outside the row's text run, so that truncating a long name can never synthesise
  a reset inside the highlight and break the rectangle above. `background`-on-`accent` and
  `background`-on-`badge` therefore join §11.6's contrast floor, and both glyphs survive
  `NO_COLOR` because they are text in a fixed column, not a colour.
  **Colour is not sufficient on its own.**
  `NO_COLOR` drops deck to monochrome, and deck's own golden frames are captured that way,
  so a focus indication carried only by a border colour is invisible to the user *and* to
  the tests. While interactive, the preview's top border therefore carries the target
  session's name as text — which doubles as the wrong-target safeguard, since a user must be
  able to see which pane is receiving their keystrokes. Any glyph in it obeys §11's
  no-East-Asian-Wide rule and has a `DECK_ASCII` fallback. A keyboard-only UI that cannot
  show where the keys are going is unusable; equally, a focus stop that changes nothing is
  worse than none at all, because the footer would then have to advertise keys that do
  nothing.
- **The footer is one line, outside both panels**, in the key/description pattern
  (`↵ attach · n new · …`). It is contextual: it lists what is bound *now*, in this mode.
  **It never lists a key that is not bound** — a footer advertising a verb the binary does
  not have is worse than no footer, because it is the one place a user is entitled to trust.
  **Nor does it list a key that would refuse the current selection.** `x` is absent on a row
  with no pane to kill, `r` on a row that is already running, `A` on a row that is already
  archived, `U` on one that is not, `Y` on a row with nothing to acknowledge (§7's
  seen state, `seen_status_at = status_at`), and every per-row key when the list
  is empty; with a mark
  set in force the question is asked of the marked rows, since that is what the key would act
  on. This is the same argument one step finer: a key that is bound but refuses *here* is,
  from the user's side, indistinguishable from one that does not exist, and discovering the
  difference costs a keystroke and a failure message. Eligibility has exactly one definition
  per action, shared by the footer and the key handler — a footer deciding it separately is a
  footer that will drift out of agreement with the behaviour it advertises.
- **The footer's fixed set is curated for the keys worth a whole line of the frame.** It
  carries navigation, `↵`, `a`, `Y`, `n`, `x`, `r`, `R`, `dd`, the eligible one of `A`/`U`,
  `,`, `i`, `?` and `q`. Rarely-used per-row actions — the sidebar pin `p`, and the actions
  that live inside `i` — stay bound, stay in the `?` overlay and in §11's keymap, and stay out
  of the footer: one
  line is a budget, and spending it on keys a user presses monthly crowds out `dd`, `U` and
  `,`. `U` in particular has to be there when it applies, because it is the reversal of an
  action that otherwise looks one-way, and `,` because settings has no other visible entry
  point in the default frame. Absence from the footer is never absence from the keymap, and
  §11's keymap plus the `?` overlay remain the complete list. With the `bookmarks` setting on,
  two more entries, `alt+0-9/m bookmarks` and `alt+m del clear bookmark`, trail the fixed set and are
  the first to be elided on a narrow frame; with the setting off the footer never mentions bookmarks.
- **The footer also carries the selected row's status reason**, on its left, separated from
  the keys. §7's reasons are prose — `awaiting signal`, `resumable`, `pane failed after the
  stale frame` — and a sidebar with 31 content columns cannot hold a name and a reason on the
  same line without eliding one of them. The footer has the whole terminal width, and the
  reason is only ever wanted for the row the user is on, so this is where it costs nothing.
  The row itself carries **glyph, name, status word and badges, and no reason text**; the full
  reason is also in the `i` detail dialog, which is where it belongs when it is long.
- **Transient notes share one right-aligned slot on the footer line and add no row to the
  frame.** The twelve finite note sources are the selection-copy and field-copy confirmations,
  the attach/kill/archive failure message, the resume note, the single and batch kill-undo
  notes, the single and batch delete-undo notes, the archive-undo note, the archive-undone
  note, the teardown-hook failure note and the rename push note (which shows only while the
  row it describes is selected, and not in interactive mode). In interactive mode the
  attached bookmark message (§11) shares the same slot. They render in the footer line in the list and in
  interactive mode alike (where the footer is the interactive one). The frame reserves no row
  for them, so a note appearing or clearing never moves the panels or the preview's first
  line. When several are set the newest is shown, and the older one's state, for example an
  open undo window, is untouched and is shown again once the newer note clears. A note set
  again is the newest even when its text is unchanged (a second kill of the same session, the
  same resume note again). A note is one
  line: a multi-line message is collapsed to one and, when it is longer than the line, elided
  with an ellipsis and never wrapped. It keeps its text up to the full width; the footer's own
  content gives up whole trailing entries to make room, and an entry that does not fit whole is
  left out, never clipped: the list legend shows whole entries (then `…`) or none, and the
  interactive footer shows the `Ctrl+Q leave interactive mode` hint whole or not at all, never a
  bare or partial key, when the note leaves less room than the hint needs. The below-minimum notice keeps the
  footer over any note. A failure message too long for the line stays whole in the `i` detail
  dialog, which shows the attach error wrapped across lines. The banners above the
  frame, the pending-delete prompt and the filter status line are not notes and keep their
  own rows.
- **The update indicator rests in the same slot** (R298, §6.5, §13.1) and adds no row: the frame
  has the same number of lines with it shown and hidden, at 80 columns and wide, in the list and
  in interactive mode. It is the slot's lowest-priority text: any other note shows over it, and it
  is back once that note clears. It has these states, by what the once-a-day check and the
  install preflight found:
  - **Available** (the check found a newer stable release, this install can be updated and
    `auto_update` is off; with `auto_update` on the release is installed at once, §6.5, and the
    indicator goes straight to the progress text): `update available: vX.Y.Z — press ctrl+u to install`.
  - **Installed** (after an auto or manual update): `update installed: vX.Y.Z — restart deck to
    apply`. It replaces the Available text and stays until deck restarts: no later check result,
    failure or setting change takes it back. The TUI keeps the update's `Result.Attestation`
    (R331): when the install was verified by the checksum alone (`checksum-only`, no authenticated
    `gh`), the text reads `update installed: vX.Y.Z (checksum-only, attestation not verified) —
    restart deck to apply`. An attested install keeps the plain text. The state is the indicator's
    `attestation` field; the tests assert it and the `checksum-only` token, not the sentence.
  - **Failed** (an update attempt failed): `update failed — see deck update`, followed by the
    cause in parentheses.
  - **Refused** (a newer release exists but this install cannot take it): `update available:
    vX.Y.Z — ` and the reason instead of `ctrl+u`: `install directory is not writable, see deck
    update`, or `installed by a package manager, update it there`.
  - **Skipped** (`auto_update` on, a `dev` build, a release exists): `auto-update skipped:
    vX.Y.Z — dev build, install a release with install.sh`. Nothing is downloaded.
  - **Hidden** when `update_check` and `auto_update` are both off, when the check is offline or
    failed (silently), when the build is current, and for a `dev` build with `auto_update` off.
    Installed is the one state the settings do not hide.

  The indicator is fitted like any other note: it is one line, elided with an ellipsis when it is
  longer than the line, and the footer's own content gives up whole trailing entries to make room
  (at 80x24 with Available shown the list legend keeps the entries that fit whole and drops the
  rest; none is cut mid-entry).
- **`ctrl+u` installs the offered update** (R299). While the indicator reads Available, `ctrl+u`
  in the list opens a confirmation (§11.4) and, once confirmed, runs the same verified update
  as `deck update` (§13.1) in the background; the UI stays responsive. The indicator reads
  `updating to vX.Y.Z — downloading and verifying…` while it runs, then Installed on success.
  On failure the indicator reads Failed with the cause, and the old binary stays exactly as it
  was: nothing partial is ever left. It never restarts deck and never touches sessions. In every
  other state (Installed, Failed, Refused for an unwritable or managed install, Hidden, and
  while an update is already running) `ctrl+u` is inert: it opens no dialog, starts no update
  and is absent from the footer legend and the `?` help; while Available is up both list it
  (`ctrl+u update deck`). It has no other list-mode binding. In a text field it keeps its
  line-editing meaning (§11.11, delete from the caret to the start). **In interactive mode
  (§11.9) deck never handles it, whatever the indicator reads: it is forwarded to the pane as
  the single byte 0x15, exactly as for any other key, so no dialog opens and no update starts.**
- **A note expires.** The selection-copy, field-copy, attach-failure, resume and archive-undone
  notes clear on their own `[ui] note_timeout` after they were set (10 s by default; whole
  seconds, at least 1; `DECK_NOTE_TIMEOUT` overrides the file; a save applies it to the next
  note). The tick is generation-guarded: a note set again, even with the same text, starts a
  new tick, and a tick left over from an older note clears nothing, so a note replaced before
  it expires keeps its full time. The kill, delete and archive undo notes keep their `Undo` /
  delete-grace windows and the teardown-hook note keeps `Undo`; the rename push note and the
  attached bookmark message keep their own lifetimes. Every note still clears on the next
  event that clears it today.

### 11.4 Dialogs

Every dialog is a bordered, centred modal over a dimmed backdrop, and they all obey one
contract so learning any one of them teaches the rest:

- `esc` cancels and changes nothing. `↵` submits. `↑`/`↓` move between fields.
  `←`/`→`/`space` change a selection — on a selection field. **On a text field `←`/`→` move
  the caret and `space` types a space**, with every other editing key §11.11 gives a text
  field; a dialog whose fields are a mix gets both, decided by the focused field's kind, never
  by the dialog. A dialog may declare **additional load-bearing keys
  of its own**, but only if it states them inline where they apply — the `r` that reveals a
  masked secret (§6.4) is the canonical example: an explicit per-view toggle, named on screen
  at the place it acts. Nothing *undeclared* is load-bearing.
- **The update confirmation** (`ctrl+u` while the update indicator reads Available, §11) is a
  dialog with no fields: it shows the current version and the target version, says that
  confirming downloads the release, verifies its checksum and replaces the binary in the
  background, and that deck keeps running and does not touch sessions. Before anything runs it
  also states, on a line `Attestation: <state>`, whether the build attestation will be verified
  (R331): `verified` when an authenticated `gh` is on `PATH`; `checksum-only` when there is none,
  where the checksum alone is used, which catches corruption only; `refused` when there is none
  and `DECK_REQUIRE_ATTESTATION=1`, where the update will be refused and nothing installed. The
  state is asked each time the dialog opens, by the same test the update itself applies
  (`update.PlanAttestation`), and is the model's `updateAttPlan`.
  `↵` installs and closes the dialog, leaving the progress state in the footer indicator; `esc`
  cancels and changes nothing — no request, no write. Every other key is swallowed, so a second
  `ctrl+u` inside it does nothing. It is never offered in interactive mode, where `ctrl+u` is
  the pane's (§11.9).
- **`tab` is reserved for completion and never moves between fields.** On a path field it is
  bash's completion key and nothing else (§11.7); on every other field, and in every dialog
  that has no path field at all, it is unbound. A key that navigates on seven fields and
  completes on the eighth means two things a few rows apart, and *which* thing the user gets
  depends on whether anything happens to exist on disk under the text they just typed — so
  the field where completion matters most is the field where navigation stops being
  predictable. Reserving the key costs one habit and buys navigation that is identical in
  every dialog. A dialog's own footer names the keys it binds, so the reservation is visible
  rather than inferred. This is a rule about moving between a dialog's *fields*; §11.5's
  settings takeover is not a dialog and its `tab` switches between two whole regions, which is
  the same thing §11.3 means when it says a focus cycle would imply stops that do nothing.
- **A transient list inside a dialog owns `↑`/`↓` while it is open**, and says so on screen:
  the completion candidate list (§11.7) is the one that exists, it moves its highlight with
  `↑`/`↓`, accepts with `↵` or `tab`, and closes with `esc` — one step short of the contract's
  own `esc`, so backing out of a list does not throw the user out of the dialog in the same
  keystroke. Field navigation resumes the moment it closes. This is a declared per-field key
  set, not an exception to the contract.
- **An overlay with no fields keeps `esc` and spends `↑`/`↓` on scrolling.** The `?` help
  overlay, the `i` session detail and the `E` event log have nothing to submit and nothing to
  move between, so the navigation keys are theirs: `↑`/`↓` (and their `j`/`k` aliases) scroll a
  line, `PgUp`/`PgDn` a page, and the wheel a line. Paging alone is not enough — a reader who
  overshoots by two lines should not have to page back and forth to find them. A contract key a
  surface has nothing to do with is left **unbound**, never made to do something invented.
- **Validation is in-dialog and specific**, and it retains what the user typed. A dialog
  never closes to reveal an error somewhere else.
- **Destructive actions confirm**, and the confirmation names the target and what will
  survive it (`kill notes — the session's history and conversation id are kept`).
- **A dialog is themed like every other surface**, in §11.6's tokens and at render time: the
  title in `title`, a field's label in `hint` and its value in `text`, explanatory help in
  `dimmed`, the keys in a footer legend in `key`, a validation message in `error`, and the
  focused field carrying the same `selection` treatment a selected list row does. A dialog is
  where deck asks for a decision, and the two destructive confirms are where it asks for the
  most consequential ones, so an unthemed dialog puts flat undifferentiated text exactly where
  the user most needs to tell a warning from a value from a hint. Colour is applied where the
  frame is drawn and never baked into the strings the model holds, so `NO_COLOR` and
  `DECK_ASCII` still yield a fully legible dialog and the golden frames stay assertable.
- **A dialog opens on the choice the user last made**, wherever it has one to remember, and
  *labels* it as such so nothing is silently assumed on their behalf: the `cwd` field's
  last-used prefill (§11.7), the create modal's **agent**, and §5's `yolo_default`. The
  remembered value is machine-local UI state (§11.2's `ui_state`), promoted only when an
  action *succeeds* — an abandoned dialog changes no default — validated against what is
  currently available (an agent whose adapter is no longer registered, or whose executable no
  longer resolves on the launch `PATH` (§6.3), falls back to the built-in default and drops the
  *last used* label), and re-derives anything computed from it, since a permission profile is a
  function of the agent it applies to. Repetition is the norm in this product: the same agent
  in the same directory, over and over, is what a session manager is *for*, and a dialog that
  forgets makes the user re-state it every time.
- **The mouse can neither cancel nor confirm.** A click outside a dialog does nothing, and
  no dialog action is reachable by mouse alone (§11.8).
- Width targets 80% of the viewport, clamped to `[26, 80]` columns. At every supported
  width (§11: minimum 80) that resolves to 64–80 columns; the lower clamp and the
  take-the-full-viewport rule at or below 26 columns apply only on a below-minimum
  terminal, as best-effort behaviour to preserve the input area, and are not a supported
  size with test obligations.

**A dialog never reads the store from its render path.** `View()` runs after every message,
including §6.5's reconcile and preview ticks, so a store read inside a view function is a
query per frame — several a second — and one slow enough to outlast the tick interval wedges
the program unrecoverably, keystrokes included, because the queue grows faster than it
drains. A dialog whose content comes from the store loads it into model state via a
`tea.Cmd` when it opens, and renders that state.

The inventory, all reachable from the list. **A dialog exists only once the behaviour behind
it does**: §11.3's "never list a key that is not bound" applies here too, so a dialog for
unbuilt behaviour is simply absent rather than a stub that opens onto nothing
(`docs/PLAN.md` is where each one is assigned to a phase). Create session · session detail
`i` — which is where §5's degradation reason and §7's `last_message` live, and from which
**rename**, the **launch-inputs editor**, the group move, the **permission profile picker**
(`P`), the **conversation lock** (`c`, §9.1) and the sidebar pin toggle (`p`) are reached — a
top-level `P` is unbound · confirm (kill, delete, purge, archive) · delete options (tombstone
vs purge) ·
send message (§11.1) · env editor · **launch-inputs editor** (§6.2 — `pre_launch`,
`post_destroy`, `launch_args`, `login_shell`; every field labelled *restart-to-apply*. The two
hook lines are shown verbatim rather than masked — they are commands, not values, and §6.4's
whole recommendation is that the command *sources* a secret rather than containing one. The
session's **event-hook** fields, `event_hook_enabled` and `event_hook_events` (§10.2), are
also edited here but are *not* restart-to-apply: they are read at dispatch, apply immediately
and set no dirty flag; the last hook exit status and output tail (§10.3) are shown in the
detail) ·
theme picker (§11.6) · event log · health view ·
find (§12) · **lost attach (§11.9)** · help overlay. Settings is deliberately *not* a dialog
— see below.

**The per-session event-hook fields (§10.2).** Two fields carry a session's own event-hook
controls, in the same order and wording in both dialogs: `event_hook_enabled`, a cycled
selection (`←`/`→`/`space`) of *inherit* (the default; follow `event_hook_default`), *on* and
*off*, and `event_hook_events`, a text field (§11.11) holding comma-separated offered kinds
(`waiting, error, ended`). An **empty** `event_hook_events` field is *inherit* (the global list
applies, stored as NULL); the word `none` is the empty list (the hook is offered nothing, stored
as `[]`); anything else must be a word of the offered set of §10.1, and a word outside it is
refused in the dialog with a note naming `event_hook_events` and the word, the typed text kept.
The **create dialog** lists them after *Login shell* and before *Group*, for every agent kind
including `shell`, both defaulting to inherit. The **launch-inputs editor** lists them after
the four launch inputs (`Event hook (event_hook_enabled)`, `Event hook kinds
(event_hook_events)`), and unlike those four their help says *applies immediately, no
restart*: submitting writes them through their own mutator, which sets neither `env_dirty` nor
`launch_dirty`, so no `env↻`/`launch↻` badge appears for an edit of only these two (an edit of
a launch input beside them still sets `launch_dirty`, for the launch input). No top-level key
is added; both dialogs are reached exactly as before.

**The harness-name adoption setting and its per-session override (§8).** The global key
`adopt_harness_names` (a boolean, **on** by default) decides whether deck adopts the name a
harness chose for a session and pushes a name typed in deck into a running harness. Off means
neither: no row is renamed from a harness's title, and `r` renames the row in deck alone and
sends nothing to the pane (a name typed on a `user` row still reaches the harness at its next
launch). It applies immediately, on the next hook, tick or rename after a save or a config
reload. A session's own **adoption override** is the nullable column `harness_name_adopt`
(§4), a cycled selection (`←`/`→`/`space`) of *inherit* (the default; follow
`adopt_harness_names`), *on* and *off*, edited in the **launch-inputs editor** as the last
field, `Adopt harness name (harness_name_adopt)`, mirroring `event_hook_enabled`: *on* adopts
and pushes for that session even while the global key is off, *off* does neither even while it
is on. Like the event-hook fields it is read when adoption or a push runs, so its help says
*applies immediately, no restart*, and submitting it sets neither `env_dirty` nor `launch_dirty`.
The create dialog does not offer it; a new row inherits.

**The last hook result and the script probe (§10.3).** Deck has no separate health screen: the
health view is the block of lines above the panels that already carries the tmux note (§9.5),
and it holds the event hook's lines. Both surfaces show the newest hook result recorded in the
`events` table's `hook_*` columns, as one summary line of marker, exit and kind, with its age:
`ok · exit status 0 · for idle · 2m ago`, `FAILED · exit status 3 · …`,
`TIMED OUT · killed when event_hook_timeout expired · …` or
`FAILED · could not start: <error> · …` (`-` for the dot under `ascii`). A non-zero exit, a
timeout and a script that could not start are marked in the `error` colour token and carry
`FAILED` or `TIMED OUT` in the text, so the mark does not depend on colour. The **`i` detail**
shows it as `Last event hook:` for that session's newest result, with the capped output tail
(its last four non-blank lines) under `Hook output:`, and shows nothing for a session no hook ran
for. The **health lines** show `Event hook last run:` for the newest result of any session with
the last two output lines, and only while `event_hook` is set. The health lines also **probe the
configured script** next to the `PATH` probe (§6.3): its first word must be a path that exists
and is an executable regular file, or a bare name that resolves on `PATH`; otherwise the line
`Event hook FAILED: <script>: script does not exist; events are recorded but nothing runs` (or
`script is not executable`) appears. Both facts are read on the sessions reload, never per frame,
and an unset `event_hook` reads and probes nothing.

**A session bound to an older deck binary gets a hint, never an error.** An agent keeps the hook
command of the deck that launched it (§3.1), so after deck is upgraded, moved or removed a running
agent still calls the old path. Every launch of an agent that installs hooks (create, resume,
restart; Claude, Codex and Pi — a shell launches no hook command) records the launching deck's
absolute path in `sessions.hook_executable` once its pane is up. When that path differs from the
running deck's, or the file no longer exists, the `i` detail dialog shows
`hooks: bound to <path> — restart (R) to refresh` (`-` for the dash under `ascii`), and the
footer's status reason for the row ends with the same text after a ` · `, so it is visible
without opening `i`. It is a hint on a healthy row: the row's status, source and stored reason
are untouched and it is never an error state. It stays until a restart or resume relaunches the
agent under the running deck, which records that path and clears it. The hint reads only the
row's recorded path, never its agent kind, so it holds the same way for Claude, Codex and Pi; a
row that recorded no path (created before schema 9, and every shell launch, which records none)
shows no hint.

**The `i` detail dialog's `Hook declined` line is an alarm, so it is shown only for a decline
that is not expected.** A hook declined because it came from a replaced launch (§9.3) is
recorded as a `<kind>.superseded` event, and a restart or resume produces one almost every
time: the pane it replaced delivers its own `session_end` after the new launch has taken the
row. That decline is correct and routine, so `i` does not show it — no `Hook declined` line
appears for it. Two cases stay loud, and only these: a declined hook of any other kind (`stop`,
`notification`, …) from a replaced launch, and a declined `session_end` that carried no launch
generation while the row holds one and no restart or resume came before it. A generation-less
`session_end` that follows a restart or resume is the replaced pre-lease pane ending, and is
hidden like the first. The line shows the newest declined hook that is not hidden. `E` is
unaffected: it lists every `.superseded` event, `session_end.superseded` included, raw.

### 11.5 Settings

Settings is a **full-screen takeover**, not a modal: a category list on the left, the
selected category's fields on the right, `/` to fuzzy-search every field by label *and*
description. A category list plus per-field descriptions inside a centred 80-column box
would be a worse version of the file editor the user already has, and the field set only
grows. The takeover is also the honest concession: deck has real configuration, and R7 means
the TUI must be the place it is edited.

- **Every flat key in `config.toml` is editable here**, and the view is generated from the
  same schema that parses the file, so a new flat key cannot be added without appearing in
  settings. `allow_yolo` reachable only by hand-editing a file is exactly the R7 violation
  this closes. The event hook (§10) adds no structured table: its keys are flat, so there
  is currently no exception to this rule.
- Field kinds are explicit: toggle, integer with bounds, string, path (with a picker),
  enum (cycled), list-of-strings, and *link* (opens the owning dialog, per the exception
  above). Each field states what it does and what changes when it changes.
- **`event_hook_events` is a comma-separated text field.** `enter` opens the §11.11 text editor
  on the staged list written as `waiting, error, ended`; `enter` commits it (spaces around
  words and empty words are dropped, and empty text is the empty list, not a cancel). A word
  outside the offered kinds of §10.1 is refused with a note naming `event_hook_events` and the
  word: the editor stays open on the typed text, nothing is staged, and `esc` abandons it. The
  other three keys use the ordinary editors (`event_hook` the text editor, `event_hook_default`
  a toggle, `event_hook_timeout` an integer stepped by `+`/`-`, never below 1 s). All four are
  global scope and apply to the running client on save (§6.5), so none says *restart-to-apply*.
- **`adopt_harness_names` is a toggle** in the General category, on by default, global scope. It
  applies to the running client on save (§6.5), so it does not say *restart-to-apply*; a
  session's own override (§11.4) beats it in both directions.
- Navigation, spelled out because the takeover is not a §11.4 dialog and the main view has
  no `tab` binding for it to echo (§11.3): `tab`/`←`/`→` switch between the category list
  and the field list, `↑`/`↓` move within the focused list, `/` searches, `ctrl+s` saves,
  `esc` prompts to discard if anything changed and otherwise closes. `,` — the same key that
  opens settings (§11.3) — closes it too, through the identical `esc` code path: it prompts
  to discard when something changed and otherwise closes, never a second, parallel
  implementation. **While a value is being typed** — a string, path, list entry, env key or value, a group name, or the `/` search —
  the field is a §11.11 text field and its editing keys win: `←`/`→` move the caret there and
  do not switch lists, and `,` is a literal character there, in the group create and rename
  prompts included, rather than the close shortcut. `,` is ignored while the discard prompt or
  the group-delete confirm is up: it neither answers nor dismisses either. The footer names it
  beside `esc` (`esc/, close`).
- **`bookmarks` is a toggle** in the General category, **off** by default, global scope, and
  `DECK_BOOKMARKS` overrides the file value (the environment wins, like the other toggles). The
  settings takeover is the menu that flips it: `enter` on the row stages it and `ctrl+s` saves it
  to `config.toml`. A save applies to the running client at the next key, so it does not say
  *restart-to-apply*. While it is off no bookmark key is bound, no slot digit is drawn, and no
  footer or help line mentions bookmarks.
- **`update_check` and `auto_update` are toggles** in the General category, global scope, with
  `DECK_UPDATE_CHECK` (default **on**) and `DECK_AUTO_UPDATE` (default **off**) overriding the file
  value. `enter` on the row stages the flip and `ctrl+s` saves it to `config.toml`; a save applies at
  the next check, so neither says *restart-to-apply*. `auto_update` on implies the check (§6.5).
- **`[ui] attach_on_click`** (default true, `DECK_ATTACH_ON_CLICK`) sits beside `attach_on_new`
  and `attach_on_resume` and governs whether a sidebar row click or double-click enters the
  interactive preview (§11.8). It is `ScopeGlobal`: a save applies to the running client at
  the next click, and an env override wins over the file value like the other two.
- **`[ui] select_on_drag`** ("Click and drag to select", default true, `DECK_SELECT_ON_DRAG`)
  governs whether a left drag over the interactive preview selects and copies text or is
  forwarded to a pane program that tracks the mouse (§11.8). It is `ScopeGlobal`: a save applies
  to the running client at the next press, and an env override wins over the file value.
- **Save is explicit** (`ctrl+s` or the Save action), a discard prompt guards unsaved
  changes on `esc`, and the write is atomic — settings must never be able to leave an
  unparseable `config.toml` behind.
- **The group list (§11) is the one section that is not staged.** Groups live in `state.db`,
  not `config.toml`, so creating, renaming or deleting one takes effect immediately and there
  is nothing for `ctrl+s` to write or for the discard prompt to revert. That difference is
  **stated in the section itself** rather than left for the user to discover, and `esc` must
  not offer to discard group edits it cannot discard: a prompt that claims to undo something
  it has already committed is worse than no prompt.
- **Scope is labelled per field**: global (`config.toml`), or per-session override where
  one exists (§6.1). A field that only takes effect on the next launch says
  *restart-to-apply*, consistent with §6.2 and the permission profile switch (§5). A setting that claims to have
  taken effect on a live pane when it has not is the same class of lie as a fabricated
  status.
- Settings edits configuration and nothing else: it cannot create, kill or resume a session,
  and nothing in a session's lifecycle (§9) is reachable from it — **with exactly one
  carve-out, and it is not a loophole.** Deleting a group (§11) must say what becomes of its
  members, and "move them all to `default`" cannot be the only answer offered: a user retiring
  a group of finished work would then have to empty it by hand first. So the delete prompt
  offers the destructive branch too, and when it is chosen the deletion runs through §9.2's
  own `dd` batch path — the same confirm dialog naming what survives, the same tombstone, the
  same `DECK_DELETE_GRACE_MS` window in which one `u` restores the whole batch. Settings never
  gains a deletion of its own, nothing is destroyed without the confirm the list would have
  shown, and every such delete is reversible for as long as any other `dd` is. An empty group
  is deleted with no prompt at all, because there is nothing to decide.

### 11.6 Themes

Colour is a first-class, user-owned artifact rather than constants in the render code.

- A theme is one TOML file. Built-ins are embedded in the binary; user themes live in
  `$XDG_CONFIG_HOME/deck/themes/*.toml` and are discovered at start-up. Adding a built-in
  is a one-file drop plus one registry entry — no per-theme code, no per-theme test.
  **The built-in set is open-ended.** It is required to contain *at least* one `dark` and one
  `light` appearance so `NO_COLOR`-adjacent and light-terminal users have a starting point;
  it is not limited to one of each, and a test that pins the built-in count or asserts
  "exactly one dark and one light" is asserting an implementation accident rather than this
  rule. Every built-in, new ones included, owes the contrast floor and the quantisation
  pinning below — those are the per-theme obligations, and they are data obligations, not
  code. A built-in whose palette cannot meet the contrast floor is the wrong palette: the
  floor is never the thing that gets relaxed to admit a theme.
- Selection is `[ui] theme = "<name>"` in `config.toml`, editable in settings (§11.5) and
  from the `t` picker, which previews the theme live on the real list while you move
  through the options and reverts on `esc`.
- **An unknown or unparseable theme name falls back to the default and says so** in the
  health view and on first paint. It never silently renders the default as though the
  chosen theme had applied.

The token set is semantic, not positional — tokens name *meanings*, so a theme author never
has to know which widget draws what:

```toml
name = "empire"
appearance = "dark"            # "dark" | "light" — drives contrast direction

[colors]
background        = "#16161a"  # the canvas deck paints (§11.3), not the terminal's
surface           = "#1f1f24"  # elevated rows, footer, dialog interiors
border            = "#45403a"
border_focus      = "#d97706"  # the focused panel (§11.3)
selection         = "#2e2a26"  # selected row, focused panel
selection_idle    = "#27231f"  # selected row, unfocused panel
title             = "#f5c518"
text              = "#ddd8cf"
dimmed            = "#8a8378"  # starting rows, elided detail
hint              = "#aaa396"  # footer descriptions
key               = "#f59e0b"  # footer/help keycaps
accent            = "#e2552f"
group             = "#ddd8cf"  # workspace headers
search_match      = "#f5c518"
badge             = "#aaa396"  # live/sampled, env↻
badge_warn        = "#f59e0b"  # non-safe permission profiles, yolo
waiting           = "#f5c518"  # the seven §7 statuses, one token each
running           = "#4ade80"
idle              = "#8a8378"
starting          = "#c2761a"
stopped           = "#6a7a99"
error             = "#e5533d"
archived          = "#b0a898"
```

- **The built-ins stay visibly different from each other.** For every unordered pair of
  built-in themes, the mean CIEDE2000 ΔE over a fixed token set — `background`, `surface`,
  `title`, `text`, `key`, `border_focus` and the seven §7 status colours — is at least **15**.
  The test iterates the registry, so a later built-in is held to the same floor with no new
  test, and on failure it names the closest pair. Each built-in has its own identity rather than a
  nudged copy of another: `empire` is near-black charcoal with amber, brick-red and gold
  accents, and `cobalt` is a saturated cobalt blue with ice, cyan and white accents and one
  warm warning colour (`badge_warn`), the red `error` status aside.
  `gruvbox-dark` is the warm brown dark of the upstream gruvbox palette and `solarized-dark` the
  teal-grey dark of upstream Solarized — both follow the upstream colours by name, and their files
  credit the upstream project and its MIT licence in a header comment. `amber` is black with
  monochrome amber phosphor, so its seven status colours differ by tone and brightness rather than
  hue alone. `high-contrast` is black and white with saturated primaries, and its `text` on
  `background` clears WCAG AAA (7:1).
- **The nine built-ins** are `empire` (the default), `daylight`, `parchment`, `matrix`, `cobalt`,
  `gruvbox-dark`, `solarized-dark`, `amber` and `high-contrast`.
- **Status colours stay distinguishable within every built-in.** The seven §7 status colours of
  one theme are pairwise at least **10** apart in CIEDE2000 ΔE, so no two statuses read as the
  same colour in the list. The test iterates the registry — a later built-in is held to the same
  floor with no new test — and on failure it names the theme and the offending pair of tokens.
- **The seven status tokens are exactly the seven statuses in §7.** If §7 grows a status,
  the theme schema grows a token; a status rendered in a colour borrowed from another
  status is a defect, because the colour is the fastest thing a human reads in the list.
- **16-colour floor, made assertable.** Truecolour values are used when the terminal
  advertises truecolour; otherwise each token is quantised at load time to the nearest of
  the 16 ANSI colours by Euclidean RGB distance **against deck's declared reference
  palette** — the xterm defaults, fixed here because terminals do not agree on what the
  16 colours are, and both "nearest" and any contrast number are undefined without one:
  `000000 cd0000 00cd00 cdcd00 0000ee cd00cd 00cdcd e5e5e5` (0–7) and
  `7f7f7f ff0000 00ff00 ffff00 5c5cff ff00ff 00ffff ffffff` (8–15). The quantised palette
  is what renders. Legibility after quantisation is a tested property with a stated
  method: for every built-in theme, `text`, `hint`, `title` and each of the seven status
  tokens must hold a WCAG contrast ratio ≥ 3:1 against `background`, `text` against
  `selection`, and `background` against both `accent` and `badge` — the two backgrounds
  §11.3's selection gutter paints its markers on — computed over **both** the hex palette and
  its quantisation to the reference palette. This is a loader-level golden test, like §7's probe fixtures — the
  spec's black-box rule (§13) applies to behaviour, and palette arithmetic is data.
  Rendering under the quantised palette *is* behaviour, so §13.1 gains
  `DECK_COLOR_DEPTH=truecolor|16` to force either path deterministically in a pty test
  regardless of what the harness terminal advertises. `NO_COLOR` drops to monochrome and
  status is then carried by the glyph column (§11) alone, which is why the glyphs are
  load-bearing and never decorative.
- A theme cannot change layout, spacing, glyphs or keybindings. It is colour only. This is
  what keeps `DECK_ASCII`, the 80×24 floor and the harness's frame assertions independent
  of whatever theme is loaded.
- One naming note, since the word does double duty: `archived` is a retention flag in §4's
  data-model terms and a display state in §7's table. The theme schema follows the *display*
  taxonomy, which is why it gets a token — the sidebar renders archived rows behind the
  filter and needs a colour for them.

### 11.7 Path entry and recent working directories

Typing a full path by hand is the single most common keystroke cost in deck, because every
session starts with one. Three mechanisms, all on the same text-input behaviour (§11.11) so the
create modal's `cwd`, and any later path field, behave identically:

**Recent working directories.** deck remembers the last **5** distinct directories a
session was created in, most-recent-first, in `state.db` (§4's `recent_cwds`) — not in
`config.toml`, since it is machine-local history rather than preference. The limit is
`[ui] recent_cwd_limit` (default 5, §6.5). Creating a session stores its `cwd` as the cleaned path (`filepath.Clean`: no trailing or doubled
separator, no `.` element; agent and shell sessions alike) and promotes that `cwd` to the
front, deduplicated by resolved absolute path, evicting the oldest beyond the limit.

- The `cwd` field is **pre-filled with the most recent entry**, so the common case —
  another session where you just were — is `n`, a name, `↵`. On a first run with no
  history it pre-fills the directory deck itself was started in. The value is **offered,
  not committed**, under §11.11's rule for an offered value: a printable first keystroke or a
  paste replaces it wholesale, while a caret or editing key accepts it and edits it in place —
  so `/home/me/proj-a` becomes `/home/me/proj-b` with `backspace` and one character, not a
  retyped path. The field labels it as the last used so nothing is silently assumed on the
  user's behalf.
- `Ctrl+P`/`Ctrl+N` in the field cycle the recent list, shell-history style, showing
  `recent 2/5` so the user knows both where they are and that more exist. This is a declared
  per-field key set under §11.4's contract, and the field's own help line names it. Each step
  replaces the whole text and puts the caret at its end. They are
  readline's history bindings, chosen for the same reason `tab` completion is: the fingers
  already know them. `↑`/`↓` are **not** bound here — they move between fields in every dialog
  (§11.4), and a path field does not get to redefine the navigation keys of the dialog it sits
  in.
- Recency is ordered by a **monotonic sequence, not the wall clock**, so the order stays
  deterministic and assertable while `DECK_CLOCK` is frozen (§13.1): anything ordered by a
  frozen clock has no order at all.
- The list is history, and paths can themselves be sensitive: settings (§11.5) offers
  clearing it, and it is never included in event-hook payloads.

**Completion happens at the end of the field, and only there.** Ghost and `tab` both complete
the segment the caret ends, and with the caret anywhere else there is no ghost and `tab` does
nothing — the fish and zsh autosuggestion contract. Completing a segment in the middle of a
path would have to decide what happens to the text after the caret, and every answer to that
surprises somebody; a user who wants to complete there deletes the tail first, which is one
`ctrl+k`.

**Ghost completion.** With the caret at the end of the field, deck shows the completion
inline in the theme's `dimmed` token, and `→` (or `end`) accepts it; with no ghost showing,
those keys are §11.11's caret keys and nothing more. Directories only —
deck is never asking for a file here. The segment being completed is the text after the
last `/`; hidden directories are candidates only when that segment starts with `.`; a
leading `~` expands. A single match completes to it plus a trailing `/`, so the next
segment can be typed immediately.

**Ambiguity ghosts nothing.** When several directories match and there is no further common
prefix, deck shows the match count (`3 matches — tab to list`) and ghosts **nothing** — it
never ghosts the alphabetically-first candidate, which is the tempting shortcut here.
Ghosting one arbitrary candidate makes `→` a coin flip that silently sends the
session to the wrong directory, and a wrong `cwd` is not a typo the user notices — it is a
session that works and is in the wrong place. `tab` completes to the longest common prefix
when that advances, and otherwise lists the candidates for selection: bash's contract,
which is the one users already have in their fingers.

### 11.8 Mouse navigation

The sidebar exists to be switched between, and a click is the cheapest switch there is. So
deck reports mouse events and binds them — under one rule that governs everything else in
this section:

**No capability is ever mouse-only.** Every binding below duplicates a key from §11's
keymap, and the key remains the primary, documented path. A mouse-only affordance is an R7
defect in exactly the way a file-only one is: it makes a capability unreachable for a user
who cannot or does not use a mouse, and unreachable for the harness. Correspondingly, deck
renders no control that only a mouse can operate — no scrollbar that is the only way to
scroll, no close button that is the only way to dismiss.

| event | effect | key it duplicates |
|---|---|---|
| click or double-click a sidebar row | selects that row **and enters §11.9's interactive preview on it**; with `[ui] attach_on_click` false (default true, §6.5) it **only selects**: the list keeps keyboard focus, the preview stays passive, and no entry, ownership claim or fit happens | `↑`/`↓` then `↵` |
| click a group header | toggle collapse | `c` (§11) |
| wheel over the sidebar | scroll the list, without selecting | `↑`/`↓`/`PgUp`/`PgDn` |
| drag the seam | adjust `sidebar_width` live | `<`/`>` |
| drag over the preview | select text; release copies it. With `[ui] select_on_drag` false (default true, §6.5) the drag goes to a pane program that tracks the mouse instead | `a`, then tmux's own copy-mode |
| `Alt`+drag or `Ctrl`+drag over the preview | select the rectangle spanned by the two corner cells; release copies one line per row (see **Rectangular selection** below). Governed by `[ui] select_on_drag` like a plain drag | `a`, then tmux's own copy-mode |
| click the collapsed strip | restore the previous non-collapsed mode | `|` |
| click the passive preview | enter §11.9's interactive preview on the **already selected** row | `↵` |
| click empty sidebar space while interactive | leave interactive mode | `Ctrl+Q` |

**A wheel over the passive preview does nothing, and a click over it is `↵`.** The passive
preview is a non-interactive, non-scrolling crop (§11): there is no viewport for a wheel to
move, and no gesture aimed at the preview may fall through to the sidebar instead. A click
there enters interactive mode on the session that is **already selected** — the one the
preview is showing — through `↵`'s own path, with every one of its refusals (§11.9) and its
inertness on a header cursor (§11). It never moves the selection: the objection this
paragraph used to make, that a mis-aimed click must not quietly move the selection and fire
§7's side effects on some other session, still holds and is still met, and the cost that
remains — a click can resize the shown session's live window — is the one the single-click
reversal below already accepted. The press that enters does not also begin a drag-to-copy
selection. Symmetrically, **while interactive, a click on sidebar space that is no row, no
header and not the collapsed strip** (the blank below the last row) is `Ctrl+Q`: the same
teardown, restore and release, with the selection left where it is; in list mode that space
still does nothing. **A drag is the exception**,
because selecting text is reading rather than acting: it takes no focus, changes no status and
moves no selection in the list. **While §11.9's interactive mode is active, the panel under the
pointer decides what the wheel scrolls:** over the preview it scrolls the grid's own
scrollback, which is the one viewport that exists there — unless the pane's own program has
asked for mouse reporting, in which case the notch is that program's (§11.9); over the sidebar it scrolls the list
exactly as it does in list mode — the viewport only, never the selection or the interactive
target, without leaving interactive mode or resizing anything — and the resulting drift ends
as §11 says, at the next input to the live pane; a click over the
preview is the drag-to-copy gesture's press and never a navigation (a click over the
*sidebar* does navigate and re-target, per the bullets below — the panel the gesture starts
in is what decides). A drag that begins on the seam adjusts the seam and a drag that begins in the preview
selects — the gesture is resolved by where it *started*, so a selection that runs off the edge
does not turn into a resize halfway through. Full attach (`a`) has no mouse affordance at all,
which this section's rule permits: no capability is mouse-*only*, not every key has a
gesture.

Four decisions in that table are load-bearing, and each is the safer of two options rather
than the obvious one:

- **A single click DOES hand over the keyboard, deliberately, and this is a reversal.** Every
  earlier revision of this section said the opposite: a single click selected, a double-click
  entered, and the reasoning was that entering interactive mode resizes a live agent's window
  (§11.9) so a stray click must not be able to cause it. The operator overrode that after
  living with it: in practice the click is nearly always aimed at "show me this session and
  let me type", the double-click was a tax on the common case, and the failure it prevented —
  clicking a row and *not* getting the keyboard — turned out to be the one that actually bit,
  because a user who has clicked a session starts typing into a list that is still reading
  those keys as navigation. So: one click selects the row **and** enters interactive mode on
  it, and `Ctrl+Q` returns to the list. The cost is stated rather than discovered: **a
  mis-aimed sidebar click now resizes that session's live window** (§11.9's fit), and the
  mitigation is `Ctrl+Q`, not a confirmation — a confirm on a click would recreate the tax
  the double-click was. **Full attach (`a`) keeps no mouse affordance at all** and is
  deliberately still key-only: it hands over the *whole terminal*, which `Ctrl+Q` cannot undo,
  so the argument this bullet just reversed still holds there and is the reason it is not
  reversed everywhere.
- **`[ui] attach_on_click` turns that handover off.** With it false (default true, §6.5,
  `DECK_ATTACH_ON_CLICK`, editable in Settings §11.5), a click **and a double-click** on a
  sidebar row only select it: the list keeps keyboard focus, the preview stays passive, and
  no entry, ownership claim or fit happens, so a key typed next is a list
  key. A double-click is two presses on the row and has no handler of its own, so the one
  setting governs both. `↵` still enters. The setting does not touch the group-header click
  (collapse), a click on the passive preview (that is `↵`), the collapsed-strip click, a
  sidebar click that re-targets while interactive mode is already active, or `Ctrl+Q`.
- **A sidebar click works while interactive mode is active, and re-targets it.** While the
  preview owns the keyboard, a click on a sidebar row selects that row and moves interactive
  mode to it — leaving the previous session's window (restoring its size per §11.9) and
  entering the new one. Without this the sidebar becomes dead surface exactly when the user
  most wants it, since single-click entry means the common state is "attached", and a click
  that did nothing would be indistinguishable to the user from deck having hung. Clicking the
  row that is *already* the interactive target changes nothing (no leave-and-re-enter, so no
  gratuitous resize of a live window). A click over the **preview** while interactive is
  unchanged from below: it is the drag-to-copy gesture's press, never a navigation.
- **The wheel scrolls the list under the pointer and changes neither focus nor selection.**
  Scrolling to look at something is not selecting it. Attach, click and preview write no
  status and no seen state (§7, R320), but a wheel that moved the selection would still turn an
  idle gesture into a navigation. Over the preview it scrolls nothing, because there is nothing there
  to scroll.
- **A click outside a dialog does nothing.** It neither cancels nor confirms; `esc` cancels
  (§11.4). "Click outside to dismiss" puts cancel and confirm a few cells apart on a
  destructive confirmation, which is precisely where an accidental click is least
  affordable. Inside a dialog the wheel scrolls a scrollable body and a click focuses a
  field — nothing more.
- **Hit-testing asks the layout what it drew.** A click resolves to a row by consulting the
  same layout that rendered the frame, never by independently recomputing row heights or
  panel offsets. Two implementations of the geometry drift the moment grouping, elision or
  a mode change touches one of them, and the symptom is a click that selects the wrong
  session — silent, intermittent, and indistinguishable from a user's mis-click.

**Selection and copy.** Enabling mouse reporting takes the terminal's own selection behaviour
over, so deck provides the selection itself rather than leaving the user a worse tool for
reading output in: **a drag beginning inside the preview selects, and releasing copies.** The
gesture is tmux's, deliberately — a manager whose own view is the primary one cannot ask
the user to leave it to copy a line. The selection is over the cells deck drew, which in
§11.9's interactive mode includes the grid's own scrollback. **An in-progress selection is
visible.** From the first motion after the press until the release, the selected cells are marked with the
`selection` token — the same treatment a selected sidebar row carries (§11.3) — and the
marking clears when the release commits the copy. A selection the user cannot see is a
selection they cannot aim: the gesture is tmux's, and so is the feedback. **A click is not a selection:** a press released without
motion marks nothing and copies nothing, and the press alone draws no marking — only a drag selects. **Every mouse event but the selection drag reaches the pane program that tracks the mouse.** While
interactive, the program with mouse reporting on (DEC modes 1000, 1002 or 1003) receives, in the
encoding it asked for (the SGR report under mode 1006, release `m`; the X10 form otherwise, release
button code 3), each of these over the preview: the press, the motion while it is held and the
release of the **middle** and **right** buttons and of the additional buttons 8-11 (codes 128-131,
SGR only: X10 has no form for them), each as it arrives with that button's code (middle 1, right 2)
and the cell the pointer is over (motion adds 32 to the code; the release cell is clamped into the
preview, so a gesture that runs off the edge still ends; a click, below, releases at its press cell); motion with **no button** held (code
35); and the sideways wheel (codes 66 and 67). A release that arrives from deck's terminal in the
X10 form names no button, so each one ends exactly one gesture still held: an ON-mode selection
first, else the last button pressed while it is still down, else the held button with the lowest
code. With several buttons down every one of them receives its release, and an anonymous release
with nothing held sends nothing. The press of a gesture must begin inside the
preview and without `Shift`: a gesture that begins on the sidebar, or with `Shift`, is never
forwarded in part. Motion is sent only to a program that asked for it: held-button motion to mode
1002 or 1003 (plain 1000 asks for press and release alone), motion with no button to mode 1003
alone; deck asks its own terminal for cell motion, so no-button motion reaches the program only
when the terminal delivers it. The **left** button is forwarded the same way as a click (below)
and, with `select_on_drag` false, as a drag. All of it **shares the wheel's routing rule (R183,
§11.9)**: forwarded only when the program tracks the mouse, the grid is at live and `Shift` is not
held; a program that does not track the mouse (a plain shell), `Shift` held and a scrolled-back
grid forward nothing. A click of the left, middle or right button (or of an additional button) is forwarded as a press
and a release at the **press cell**, in the encoding the program asked for, when its release
arrives with no motion since that button's press, whatever cell the terminal names for the
release; a drag that did report motion keeps its real motion cells and ends at its release cell
(clamped into the preview). Each held button keeps its own press cell. An ON-mode left drag is the selection above and
the program receives nothing from it. A forwarded event is input to the pane and ends a sidebar
drift (§11), nothing forwarded is highlighted or copied, and an event the encoding cannot carry
(X10 past column or row 223, or under mode 1005 or 1015) is dropped. **`[ui] select_on_drag` chooses who owns the left drag.** With it true
(default, §6.5, `DECK_SELECT_ON_DRAG`, editable in Settings §11.5 as "Click and drag to select") a
left drag over the preview selects, highlights and copies on release exactly as above, and a pane
program that tracks the mouse receives nothing from it; every other mouse event is forwarded. With it false deck intercepts
nothing: the left drag is the program's too, so the press, the motion while the button is held and
the release are forwarded as mouse reports in the encoding the program asked for (motion carries
the button code plus 32 and is sent only to a program in mode 1002 or 1003), under the same
routing rule as the wheel and clicks, and nothing is highlighted and nothing is copied. A program
that does not track the mouse receives nothing and the drag does nothing. A `Shift` held at the
press keeps the selection route in both settings. A click is forwarded either way. **The marking is
linear for a plain drag**, because the copy is: it covers exactly the run `SelectedText`
would return for the same anchor and current cell, so what is highlighted and what is
copied can never disagree. **Rectangular selection.** A drag with `Alt` held, or with `Ctrl`
held, selects the rectangle spanned by the press cell and the current cell instead (`Ctrl` is the
fallback for window managers and terminals that swallow `Alt`; each works alone). `Ctrl`+`Alt`
together, and anything with `Shift` held, are not bindings: such a drag selects by line like a
plain one. The modifiers are read on the press and fix the kind of selection for that drag. The rectangle is
marked with the `selection` token over every cell of its column range, blank cells included, on
every row between the two corners, and it is the same block the release copies: for each row, the
characters inside the column range with trailing blanks trimmed, joined with `\n`, one line per
row. A row that ends before the range gives a shorter, possibly empty, line and is never padded
to the width of the rectangle. A double-width glyph is copied whole when its left cell is inside
the range, and not at all when only its right cell is: half a glyph is never emitted. The
corners may be dragged in any of the four directions with the same result. The rectangle is
clamped to the preview where the drag began (a drag that runs onto the sidebar or off the screen
stops at the preview's edge), so sidebar text never mixes into the copy. Where a plain drag
selects, the rectangle is available, in the interactive preview, and `[ui] select_on_drag` false turns
off both kinds: an `Alt` or `Ctrl` drag then goes to a pane program that tracks the mouse like
any other drag and selects nothing. **Caveats.** Many window managers grab `Alt`+drag to move the
window, in which case deck never receives the drag: use `Ctrl`+drag, or change the window
manager's modifier. A terminal reports modifiers in the mouse report's button code (SGR adds 4
for `Shift`, 8 for `Alt`/`Meta`, 16 for `Ctrl`); some terminals omit the `Alt` bit or take
`Alt`+drag for their own selection, and `Ctrl`+drag is independent of that. When deck runs
inside tmux, tmux must pass the report through with its modifier bits intact (tmux does by default
with `mouse on` and does not rewrite them), and a tmux or terminal binding on the same chord
takes it before deck sees it; `Ctrl`+`Alt` is no alternative, since it still depends on `Alt`, and
`Shift` stays the terminal's own selection override. The copy is written to a **tmux
buffer** on deck's own server, which always works and is what `tmux paste-buffer` reads;
where the outer terminal permits it an **OSC 52** write additionally reaches the user's system
clipboard, and that half is best-effort by nature — it depends on the terminal and on tmux's
`set-clipboard`, so it is never the only thing a copy does. Mouse reporting stays
**opt-outable** — `[ui] mouse` (default true, §6.5) and `DECK_MOUSE` (§13.1) — and with it off
the terminal's own `shift` override is the route again; the help view states which of the two
applies rather than leaving it to be discovered.

**Encoding and hygiene.** deck uses SGR extended reporting (1006), so coordinates past
column 223 are correct rather than wrapped; it must not depend on X10 encoding. Reporting is
enabled on start and **disabled on every exit path, including a panic** — a deck that exits
without turning it off leaves the user's shell printing escape sequences at every mouse
move, which reads as a corrupted terminal rather than as deck's fault. Because those
enable/disable sequences are bytes in the stream, scenarios asserting an exact byte stream
(§11.2's golden frame) set `DECK_MOUSE=0`.

**Degradation.** A terminal that reports no mouse events loses the shortcuts and nothing
else. Below deck's 80×24 minimum, and in `stacked` and `collapsed` modes (§11.2), hit-testing
follows whatever the layout actually drew, on the same best-effort footing as the rest of the
frame.

**Deliberately not, on top of §1's non-goals:** no right-click and no context menus (deck has
no menu concept to hang them on, and a menu would become the second place every action is
declared); no drag-to-reorder (§11's sort order is *chosen* from a fixed set of four rules,
never arranged by hand — a hand-arranged list has no rule to re-apply when a session's status
or name changes, and it would stop answering "which session needs me"); no clickable
footer (it is a hint line, not a toolbar, and §11.3 already binds it to what is bound *now*).

### 11.9 Interactive preview

`Enter` hands the keyboard to the selected session without leaving the list. deck fits the
session's window to the preview panel, streams the pane into an in-process cell grid, and
forwards keystrokes to it. `Ctrl+Q` returns. `a` remains the escalation to a real terminal.
`Ctrl+U` is among the forwarded keys: deck never handles it here, so the update confirmation (§11.4) is
not offered and the pane receives the single byte 0x15 whatever the update indicator reads.
Entering is an attachment in §7's sense, and an attachment is inert (R320): it writes no status, no seen
state and no event, so the row neither changes nor moves. A refused entry claims nothing either.
A successful create from `n` (§11, `[ui] attach_on_new`) or resume from `r`/`R` (§9.1,
`[ui] attach_on_resume`) enters by itself through exactly this path — the same refusals and
banner, the same claim and fit — so there is one way in, not
two.

**A host terminal resize re-fits an interactive session at once too.** While interactive, a
`WindowSizeMsg` that changes the preview box fits the held window to the new box and reseeds
the grid at that size, in the same update and with no further input; a resize to the current
size touches nothing, and one that drops the box below the 7-row floor leaves interactive mode
as the refusals below state.

What separates this from §11's passive fit is **ownership**, not permission: passive fitting
picks a size and leaves it, while interactive mode records what it found, claims it, and puts
it back. The interaction model settles whether deck may resize a session at all, and `a` has
always resized the window; interactive mode changes what may be *typed* into a pane and what
must be restored afterwards.

- **Geometry is owned, claimed and restored.** Entering records the window's dimensions and
  its window-local `window-size` value, claims ownership in a pid-tagged window option, then
  resizes the **window** (never the pane — on a split window chrome is proportional and a
  pane-targeting loop cannot converge) and sets `window-size manual` on it explicitly. Setting
  it is not redundant with the resize: a window a passive fit (§11) already brought to this
  exact box needs no resize at all, so nothing would write the pin, and interactive mode is the
  one path that genuinely needs the size *held* — its grid is drawn against it. Exiting resizes
  back *only when no client is attached*, then unsets the window-local `window-size`. The order is load-bearing and
  `set -g window-size latest` does not substitute for it: `resize-window` writes `manual`
  into the **window** options, which shadow the global. Cost is exactly two `SIGWINCH` per
  cycle.
- **Refused rather than degraded, in three cases**, each naming its reason and offering `a`:
  another client is attached to that session — one window has one size, and an attached client
  re-expresses its own under `window-size latest`, so the fit cannot be *held*, and unlike
  §11's best-effort passive fit interactive mode needs a held size for its grid to stay
  correct; the preview box has fewer than **7 inner rows**, which is deck's stacked height
  floor and leaves no transcript at all; or ownership is held by a live process. Two of those
  three are refusals about *someone else's* claim on the window rather than about deck's
  ability to do the job, so both also offer `F` (below) and say so; the row floor is deck's
  own limit and offers only `a`.
- **Every refusal is shown in the preview pane, not only in the footer, and keys stay live.**
  A refused entry — the three cases above, a stopped session, a pane that is not live, any
  other error that stops entry, and the fall-out when the panel shrinks below the floor while
  interactive — draws one banner over the preview: boxed, centred, spanning the pane's width
  but not its height, so the passive capture stays visible around it. Line one is the
  headline (`NOT ATTACHED: <session> …`), line two the reason, line three the way out for
  that reason (`F` and `a` for contention, `a` for the floor) and, always, that keys are going
  to the list. It is high-visibility in every render mode: a warning background in colour,
  reverse video plus bold under `NO_COLOR`, and a plain `+-|` box under `ascii`. **It does
  not take the keyboard**: every list key keeps working, because the failure being prevented
  is not seeing the refusal, and a modal would swap it for a dialog to dismiss. It belongs to
  the refused session and clears when the selection moves, when an entry (`↵`, `F`, `a`)
  succeeds, when a later tick finds the reason gone (the holder left, the session started),
  or on `Esc` — which dismisses the banner *before* any other layer `Esc` clears. Every entry
  path shows it: `↵`, `F`, a sidebar click and a preview click. No refusal renders as a
  footer line alone; notes that are not refusals (undo, copy, resume) stay where they are.
- **`F` forces entry over whoever holds the window.** One operator working one set of sessions
  from more than one place is the ordinary case, not an anomaly, and the refusals above leave
  them with no way to move the keyboard except by hunting down the other client. `F` is
  identical to `↵` in every respect but two: it does not refuse for an attached client, and
  it claims ownership *over* a live holder instead of standing down for one. Every other
  refusal still applies unchanged — a stopped session, the 7-inner-row floor, a pane that is
  not live — because those are not contention. There is **no priority between a full attach
  and a preview**: forcing over an attached client resizes the window under them, which is
  the accepted cost of the interaction model deck is built around, and it never detaches
  them. With nothing to steal, `F` and `↵` are the same key.
- **Exactly one client wins a contested window.** The claim protocol already decides this and
  needs nothing added: a forced claim writes over whatever is there and then re-reads, and
  only the value that survives its own confirm-read is a claim. Simultaneous forcers therefore
  produce one winner and no corrupt state; every loser stands down with no error, back into
  the ordinary refusal — which still offers `F`, so trying again is a keypress, not a puzzle.
- **The original geometry outlives every steal.** A stealer that captured the window's size at
  steal time would capture the *previous holder's fitted* size and hand that back on exit, so
  the real pre-preview geometry would be lost at the first steal and drift further at each one
  after. So the geometry to restore is recorded **beside the ownership claim, in the window's
  own options**, written by whoever finds none there and read — never rewritten — by whoever
  takes the claim afterwards. It is restored and cleared by the last holder to let go
  legitimately, and by nobody else: a holder whose claim was stolen restores nothing, closes
  only its own transport, and leaves the window to its new owner. The SIGKILL-reclaim path
  reads the same record and keeps standing down for a live claim, so a crashed process's
  cleanup can never resize a window out from under the client that took over from it.
- **Only deck's own pipe directories are reclaimed.** The start-time reclaim scans the shared
  temp directory for `deck-interactive-pipe-*` entries, a place any local user can write to, so
  it removes an entry only when it is a real directory (`lstat`, never a symlink), owned by the
  current user, with permission mode exactly `0700` (any other mode, tighter or looser, and any
  setuid/setgid/sticky bit disqualify it) — what deck itself creates. A fresh pipe directory is
  always created separately and never reuses a skipped entry. Each entry is
  re-checked with its own `lstat` right before it is touched, never trusted from the scan's
  listing, so an entry swapped for a symlink after the scan is still recognised as one. A
  symlink, a non-directory, a directory owned by someone else and a directory whose mode is not
  `0700` (looser, such as `0755`, or restrictive, such as `0500` or `0600`) are left in place
  and not reported.
- **A pipe still being armed is not a leak.** An entry's claim record is written only once the
  whole entry has succeeded, so a second deck starting while the first is mid-entry would
  otherwise find a record-less directory and delete the FIFO the entry is arming. Every pipe
  directory therefore names the process that created it (an `owner.pid` marker, written
  before the directory takes its `deck-interactive-pipe-*` name, so no scan ever sees one
  without it), and a record-less entry whose owner is still alive is left untouched. A
  record-less entry with a dead or missing owner is removed as before.
- **A displaced client is told, and its keyboard is stopped first.** Losing the pane silently
  is worse than losing it: the next keystrokes would go somewhere the user cannot see, or
  nowhere. Whichever way the loss happens — its claim stolen by `F`, or a full attach arriving
  and re-expressing its own size — the losing client leaves interactive mode and raises a
  dialog that names the session, says another client took over, and dismisses on `↵`. It
  swallows every key while it is up, because that is its point. Detection rides the preview
  tick, with the pipe transport's own displacement signal as the fast path; the keystrokes
  typed in the window between the steal and the tick that notices are a **known, accepted
  loss**, deliberately not paid for with a tmux round-trip per keystroke.
  **A dead pane is not a takeover.** The same tick read also answers whether the pane is
  gone. When the read fails because the target no longer resolves, or reports the pane dead,
  the tick reports a dead pane **only**: the claim and attached-client checks are skipped, the
  dialog is not raised, and the grid is told its pane died, which is the path the operator
  sees instead. Only a live pane whose window another client holds raises the dialog.
- **The transport is `pipe-pane -IO` into a `charmbracelet/x/vt` grid**, seeded from
  `capture-pane -e -N` plus the pane state tmux exposes as formats, and **reseeded on every
  resize** — resizing the grid alone leaves it wrong for seconds. The pipe is armed before
  the seed is taken. `pipe-pane` is single-holder per pane, so a second reader displaces the
  first silently; a reader that sees EOF while `pane_pipe` is still 1 has been displaced,
  falls back to passive capture, and **says so**. A dead pane never closes the pipe at all,
  so `pane_dead` is polled rather than inferred from EOF.
- **An escape string never prints into the grid.** The vt parser is byte-based, so a UTF-8
  character carrying a byte in `0x80`–`0x9F` inside a string — the `✳` (`E2 9C B3`) of a Claude
  Code window title `ESC ] 0 ; ✳ name BEL` holds `0x9C`, the C1 string terminator — ends the
  string early and the rest of the title prints at the cursor, over the input row. One stateful
  pre-filter sits in front of every grid write, shared by the live pipe drain, the seed write,
  every reseed and the capture transport's loop. Inside an OSC (`ESC ]`), DCS (`ESC P`), SOS
  (`ESC X`), PM (`ESC ^`) or APC (`ESC _`) string it drops the bytes that would end the string
  early: `0x80`–`0x9F` in an OSC and in a DCS's passthrough, every byte `>= 0x80` in an SOS, PM
  or APC string and in a DCS header, whose payload the parser reads as ASCII only. A string ends
  where the parser ends it: `BEL` (OSC only), `ESC` (so `ESC \`), `CAN` or `SUB`. `ESC \`
  ends every kind from every header and payload state, an empty DCS (`ESC P ESC \`) included —
  there the parser reads the `ESC` as payload, but the filter still treats the string as ended.
  A second `ESC` after `ESC P`, like an `ESC` in a later DCS header state, starts an escape
  sequence, so `ESC P ESC ESC ]` opens an OSC whose `BEL` ends it. Outside a
  string every byte is forwarded unchanged — ordinary text, including every non-ASCII
  character, reaches the grid byte for byte — and nothing is buffered: a chunk is forwarded as it
  arrives, and the filter's state carries across chunks, so a read split anywhere (between `E2`
  and `9C B3` included) yields the same grid as an unsplit one. The pane never needs a string's
  payload, and the parser still sees each string's start and end.
- **Foreign bytes never reach the outer terminal.** Only composed cells are emitted. This is
  not an optimisation: pane bytes passed through leave the *outer* terminal on the alternate
  screen and reprogram its scrolling region. Passive preview gets this property free from
  `capture-pane`, which carries no such sequences; the moment bytes come from a pipe, the
  grid is the only safe consumer.
- **Input is dispatched on a verified identity**, re-resolved immediately before every send:
  socket path, server pid, `pane_id`, **`pane_pid`** and session name. `pane_id` alone is not
  sufficient — `respawn-pane` keeps it, and everything else tmux reports, unchanged.
- **The grid keeps its own bounded scrollback, and the wheel over the preview scrolls it**
  (over the sidebar the wheel scrolls the list, §11.8) — **unless the pane's program tracks the
  mouse.** A full-screen program on the alternate screen has no tmux history, so for it the
  grid's scrollback is empty and a wheel that only ever scrolls the grid does nothing at all,
  while the program itself (an agent's full-screen renderer, `less`, `btop`) is waiting to be
  told about the wheel. So deck does what tmux does under `mouse on`: while the program has
  mouse reporting on (DEC modes 1000, 1002 or 1003, as the grid tracks them from the seed and
  the live stream — never a new tmux read per notch), a wheel notch over the preview is
  **forwarded** to the pane as the program asked to receive it — an SGR report (`CSI < 64|65 ;
  x ; y M`) under mode 1006, the X10 form otherwise — at the pane cell under the pointer, through
  the same verified-identity send path as a keystroke (below), and the grid does not scroll.
  The X10 form cannot encode a column or row past 223; a notch there, and a notch under the
  1005/1015 encodings deck does not produce, is dropped — a **stated gap**, for the same reason
  §11.9's key gaps are: an encoding the program would misread is worse than none. Three things
  keep the grid's own scrollback reachable: with the grid already **scrolled back**, the wheel
  keeps scrolling the grid until it is back at live; **`Shift`+wheel always scrolls the grid**,
  the same override terminals give for programs that grab the mouse; and the routing is
  re-decided at every notch, so a program that turns reporting off (or exits back to its shell)
  hands the wheel back immediately. A forwarded notch is input to the pane, so it ends a sidebar
  drift exactly as a forwarded key does (§11).
  **Copilot is one of these programs.** `copilot` runs on the alternate screen (1049) with mouse
  modes 1003 + 1006 and focus reporting 1004, hides and shows the cursor around every move
  (`CSI ? 25 l/h`) and may set scroll regions (`CSI t ; b r`); the grid follows the stream, so
  after the program exits the main screen is back with the summary it printed, the cursor bit is the
  stream's final state rather than an intermediate one, and 1003 + 1006 route wheel notches, clicks
  and hover exactly as for any mouse-tracking full-screen app. The grid reports mode 1004
  (`Grid.FocusReporting`) from the live stream, separately from the mouse modes: it never makes a
  pane look mouse-tracking. deck does not act on it: it never writes a focus-in or focus-out report
  (`CSI I`, `CSI O`) into the pane. A resize while copilot is working is the ordinary full reseed of
  a fresh grid: copilot redraws on `SIGWINCH`, and the reflowed grid is the new size with none of
  the old size's cells left.
- **The pane's text cursor is drawn.** A full `tmux attach` shows where the program's cursor
  is, and editing a prompt in the middle of a line — arrows, word jumps, a deletion — is
  guesswork without it. deck draws the grid's cursor cell in **reverse video**: it survives
  `NO_COLOR` and `ascii`, reads in every theme, and needs no new token, and it is the same
  device the header cursor (§11) already uses for the same reason. The position and
  visibility are the grid's, from the seed's `cursor_x`/`cursor_y`/`cursor_flag` and the live
  stream after it — never a separate tmux read — and the cell is composed in the same frame as
  the rows it sits in, so it can never be drawn against stale text. It is **not drawn** while
  the program has hidden its cursor (DECTCEM off, `cursor_flag = 0`) — full-screen programs
  that paint their own cursor would otherwise show two — nor while the grid is scrolled back,
  since the cursor lives on the live screen; it returns at live. A cell the program itself
  painted in reverse video is the program's cursor and is shown as painted under every
  `[ui] preview_paint` mode. The passive preview draws no cursor.
- **The one-off entry seed reaches back into the pane's own tmux scrollback**, so a session
  that ran for an hour before anyone previewed it can be scrolled back over immediately rather
  than presenting an empty buffer. The entry capture starts at `-<bound>` rather than `0`,
  where the bound is what the grid can hold — pulling more would only be discarded on arrival.
  tmux clamps that start to the history that exists, and reflows history to the pane's width,
  so the fit deck performs *before* seeding is what makes the captured rows map one-to-one onto
  grid rows with no rewrapping of deck's own. Three consequences are load-bearing rather than
  incidental: `-N`'s trailing blank rows are **part of the body**, because with history
  prepended the rows are bottom-anchored and dropping them would shift the whole picture down
  and push the live screen into scrollback; an **alternate-screen pane is seeded with no
  history at all**, since tmux returns stale pre-launch rows above the alternate screen and the
  pane's history is frozen while it is up, so the suppression is a second *tmux-side* capture
  and never a slice of the first (`capture-pane -e` inherits SGR across rows, so a slice can
  strip the pen the surviving top row relied on); and a pane too busy to capture atomically
  **degrades to a history-less seed rather than refusing entry**.
- **Only the entry seed reads history; the periodic reseeds do not.** The capture transport's
  poll loop and the post-displacement fallback both rebuild the grid wholesale several times a
  second, where a history-inclusive reseed costs roughly half a core at a real pane size, so
  they stay on the visible-screen range — and the capture transport's entry seed therefore asks
  for none either, rather than paying for history its own next tick discards. Interactive
  scrollback beyond the visible screen is a property of the **pipe** transport, and a
  displacement gives it up along with the pipe.
- **Modified navigation keys forward, like the unmodified ones, by tmux key name.**
  `Ctrl`, `Shift` and `Alt` combinations with the arrows, `Home`, `End` and the page keys are
  what word-wise movement and selection are built from in every agent's line editor, so a mode
  where they silently vanish is a mode the user has to leave to edit a line. Their encoding is
  as mode-dependent as a bare arrow's, so tmux names them for the same reason it names those.
  The forwardable set is **enumerated and tested key by key**, never left to a default branch:
  a key deck cannot encode must be a known, listed gap, because the failure it otherwise
  produces is a keystroke that does nothing and reports nothing.
  **The listed gaps are two, and both are gaps for the same reason** — forwarding them would
  deliver a *different* key than the one pressed, which is worse than dropping it. `Alt+Insert`:
  tmux names and translates it correctly, but the decoder deck reads its own input through has no
  entry for those bytes and carries xterm's `Shift+Delete` sequence under that name instead, so
  deck cannot reliably tell the two apart on the way in and must not claim the key on the way
  out. `Alt+Shift+Tab`: tmux itself discards the modifier, emitting bytes identical to a bare
  `Shift+Tab`, so naming it would forward `Shift+Tab` for a physical `Alt+Shift+Tab` — silently
  dropping a modifier, which is the failure this bullet exists to forbid, one modifier smaller.
  A gap is a **stated** gap: it lives in the enumeration with its reason, so that a decoder or
  tmux release that closes it is a change to make deliberately rather than a discovery.
- **Honesty about what the user cannot see.** When the target has not repainted since the
  resize, the panel says so; an empty frame otherwise reads as deck being broken rather than
  the agent being wedged. Help states that previewing and entering interactive mode both
  resize the agent's window, that output produced while narrow consumes scrollback faster, and
  that `[ui] preview_fit = false` turns the passive half of that off.

### 11.10 The list filter

`/` narrows the list as you type, incrementally, over what a row already shows — name, `cwd`,
group and status. The query is a §11.11 text field, and every edit re-narrows. It is a **view over the list and never a mutation**: nothing about a
session changes because it is hidden or shown.

- **It widens the pool to archived rows** while a query is in force, and only then. Archived
  rows are absent from the default list by definition (§9.2), so a filter that could only
  narrow the default list would leave them unreachable — this is the route back to one, which
  is what makes `U` and `dd` reachable on an archived row at all.
- **`↵` closes the text field and leaves the query in force**, so the ordinary keymap — arrows,
  `m`, `↵`, `U`, `dd` — acts on the narrowed list. A filter is a working set, not a modal
  search box you have to leave before you can do anything.
- **`esc` clears the query and returns to the unfiltered list**, from the open text field *and*
  from a query held in force with the field closed. Both, because the line that states the
  filter is in force also names the key that clears it, and a UI that advertises a key which
  does nothing is the §11.3 footer defect in a different place. When something nearer is open —
  an overlay, a dialog, a mark set — `esc` dismisses that first; the held filter is what it
  reaches when there is nothing nearer, and one press never does two of those at once.
- **The sidebar states that a filter is in force**, with the match count, so a hidden row is
  never mistaken for a deleted one. That statement is the whole reason hiding rows is safe.

### 11.11 Text fields

Every place deck takes typed text is **one line editor with a real caret**, the same one in
every field: the create modal's name, `cwd`, launch arguments, env, pre-launch and
post-destroy fields; rename; the launch-inputs editor; the env editor; every typed value in
settings (§11.5), including a group name and the settings search; and the `/` filter (§11.10).
A field that can only append and trim its end makes a typo in the middle a retype of the whole
line, and a set of fields that each behave a little differently means the user learns a field
instead of learning deck. The keys are readline's, because that is what the fingers already
know:

| keys | effect |
|---|---|
| `←` / `→`, `ctrl+b` / `ctrl+f` | caret one character left / right |
| `home` / `end`, `ctrl+a` / `ctrl+e` | caret to the start / end |
| `alt+b` / `alt+f`, `ctrl+←` / `ctrl+→` | caret one word left / right |
| `backspace` (`ctrl+h`) / `delete` (`ctrl+d`) | delete the character before / under the caret |
| `ctrl+w` | delete back to the previous whitespace |
| `alt+backspace` | delete back one word |
| `ctrl+u` / `ctrl+k` | delete from the caret to the start / to the end |
| typing, bracketed paste | insert at the caret |
| `alt+w` | copy the whole field's text |

- **A character is a grapheme cluster and a width is in cells.** The caret steps over a
  combining sequence or an emoji as one character, and an East-Asian-wide character occupies
  two cells and is never split. A field narrower than its text **scrolls horizontally** to keep
  the caret in view, and marks the clipped side or sides with `…` (`...` under `DECK_ASCII`).
- **A word** for `alt+b`/`alt+f`/`ctrl+←`/`ctrl+→`/`alt+backspace` is a run of letters and
  digits, so `/`, `-`, `.` and `_` are boundaries and a path is edited a segment at a time;
  `ctrl+w` alone deletes to whitespace, bash's `unix-word-rubout`. Both are bash's own split.
- **A paste is one insertion of text, not a stream of keys.** It lands at the caret in one
  step, and control characters in it — newlines and tabs included — are dropped, because every
  field is one line and a pasted trailing newline is never meant as text.
- **The caret is drawn as a reverse-video cell** on the focused field only, the device §11.9
  uses for a pane's cursor, so it survives `NO_COLOR`; at the end of the text it is a reversed
  blank. No field draws an `_` or any other stand-in.
- **An offered value is one the field starts with but the user has not chosen** — §11.7's
  last-used `cwd`, and the current value a rename, a settings edit or the env editor opens on.
  It is drawn as a selection, the whole text in `selection` with the caret at its end. While it
  is offered, a **printable keystroke or a paste replaces it wholesale**, and any **caret or
  editing key accepts it** and then acts on it as ordinary text: `←` steps into it, `backspace`
  deletes its last character. A value is either offered or not; there is no per-field variant
  of this rule.
- **`alt+w` copies through §11.8's copy path**: the field's whole text to the tmux buffer, and
  best-effort to the system clipboard with OSC 52, confirmed on screen the way a drag-to-copy
  is. A masked secret (§6.4) copies only while it is revealed. Paste needs nothing from deck —
  the terminal's own paste arrives as a bracketed paste.
- **Field navigation is not the editor's.** `↑`/`↓`, `↵`, `esc` and `tab` keep §11.4's meaning
  (and §11.7's on a path field), and a key the table does not list is not swallowed by the
  field; a field's own declared keys (§11.7's `ctrl+p`/`ctrl+n`) are named on its help line.
- **Not provided:** multiple lines, undo, a kill ring or `ctrl+y`, and selecting part of a
  field. Copy is whole-field because the one copy that a field cannot do without is getting its
  value out at all.

---

## 12. Cross-session search

`f` opens a search view over three corpora, ranked and grouped by session:

1. session metadata (name, cwd, group, args),
2. the event log (statuses, reasons, last messages),
3. **agent transcripts**, located per adapter via `TranscriptPaths` — the agents' own
   on-disk conversation files, read-only.

v1 is an on-demand bounded scan (worker pool, size and age caps, cancellable, streaming
results) — answering "which session was I doing X in" without an index to keep coherent.
A SQLite FTS5 index over events plus transcript digests is a later option if scans get
slow; FTS5 is available in the pure-Go driver, so it costs no new dependency.

Hits open the owning session; a hit in a `stopped` session offers resume directly from the
results.

**Retention.** `events` is append-only in the sense that nothing rewrites or reorders a row,
not in the sense that it grows without limit. `event_retention_days` (default 30, §6.5)
bounds it: rows older than the window are deleted on store open and thereafter at most once
an hour, oldest first, in bounded batches so the delete never blocks a write for longer than
one batch. Retention is a floor on history, not a cap on rows — a burst inside the window is
kept whole rather than trimmed to a row count, because a row-count cap silently discards the
newest evidence of exactly the incident that produced the burst. `VACUUM` is never automatic:
freed pages are reused, and reclaiming file space is an explicit operator action.

---

## 13. Testing

The spec is executable. Every requirement in §1 and every behaviour below is written as
Gherkin and verified against the **released binary**, driven through a real terminal
against a real tmux — no in-process model harness, no test-only branches in product code,
no assertions on internal Go APIs.

### 13.1 What the binary must expose to be testable

These are product features — documented, supported, harmless in normal use — not
scaffolding. Without them the binary is not black-box testable at all, which is why they are
listed here rather than left to a test package:

| control | mechanism | why |
|---|---|---|
| **State isolation** | `DECK_HOME` overrides the data/config/state root (XDG resolution applies only when unset); a named profile (§3.4) nests under `$DECK_HOME/profiles/<name>/`. | Each scenario gets a pristine root; parallel scenarios never collide. |
| **tmux isolation** | `DECK_TMUX_SOCKET` overrides the socket name (default `deck`, or `deck-<name>` for a named profile, §3.4). | Scenarios run concurrently against private servers; teardown kills exactly one. |
| **Frozen / stepped clock** | `DECK_CLOCK=<rfc3339>` pins wall-clock now; `DECK_CLOCK_STEP` advances it on demand. **Wall clock only — durations, timeouts and budgets always use a monotonic clock and are never frozen**, or the §13.5 budget assertions would all measure zero. | Relative times ("2m", "31m") and quiet-hours windows become assertable without making elapsed time unmeasurable. |
| **Deterministic rendering** | `NO_COLOR`, `DECK_ASCII=1` (no nerd glyphs), `DECK_ANIM=0` (no spinner frames), fixed `COLUMNS`×`LINES`. | Screen text is byte-stable, so golden frames are meaningful. |
| **Explicit colour override** | `DECK_COLOR` forces colour on or off as a boolean, overriding both `NO_COLOR` and terminal detection. | `NO_COLOR` can only ever *disable*; a test that needs colour deliberately on — or a terminal deck mis-detects — has no other lever. |
| **Colour depth override** | `DECK_COLOR_DEPTH=truecolor\|16` forces the render path, overriding COLORTERM/TERM detection. | §11.6's quantised palette is behaviour, and a pty test cannot otherwise deterministically reach it — the harness terminal's advertised depth would decide which renderer runs. |
| **Mouse reporting override** | `DECK_MOUSE` forces mouse reporting on or off as a boolean, overriding `[ui] mouse` (§11.8). | Enabling reporting writes enable/disable sequences into the stream, so byte-exact frame assertions (§11.2's golden frame) need it off; mouse scenarios need it on regardless of the config file. |
| **Deterministic ids** | `DECK_ID_SEED` makes generated session/conversation UUIDs reproducible. | Assert exact resume arguments. |
| **Bounded ticks** | `DECK_RECONCILE_MS` (default 500) and `DECK_PREVIEW_MS` (default 250) — two rates, two knobs, matching §7 and §11. | Tests wait on state, not on wall clock; low values make scenarios fast. |
| **Config poll rate** | `DECK_CONFIG_POLL_MS` (default 30000, a duration like the ticks above) — how often the running TUI compares the §6.5 config/theme fingerprint. | **Test-only**: production never sets it, and it exists to shorten the poll (the production interval is already the 30 s upper bound), so a two-instance scenario sees a peer's edit in milliseconds, not half a minute. |
| **Interactive render rate** | `DECK_INTERACTIVE_MS` — §11.9's grid render-coalescing interval. A duration, like the two ticks above. | Render frequency, not parsing, dominates the transport's cost, so it is the one axis worth pinning in a scenario. |
| **Interactive transport** | `DECK_INTERACTIVE_TRANSPORT=pipe\|capture` pins §11.9's render path. | A *selector over two implementations of one contract*, not a behaviour switch: both paths must satisfy the same scenarios, so a scenario can exercise either deterministically. The **scrollback scenarios are the one exception**, and named as such: §11.9 gives interactive scrollback to the pipe transport only, because the capture path's own poll tick rebuilds the grid from the visible screen, so those scenarios are pipe-only by construction rather than by oversight. Stated explicitly because this section otherwise forbids knobs that change what the product does. |
| **Update source** | `DECK_UPDATE_URL` overrides the release base URL `deck update` (and the update check) use, default `https://github.com/n-orlov/deck`. `<base>/releases/latest` must answer with a redirect to `<base>/releases/tag/vX.Y.Z`, and the archive `deck_<os>_<arch>.tar.gz` and `checksums.txt` are fetched from `<base>/releases/download/<tag>/`. The base must be `https`; plain `http` is accepted only for a loopback host (`127.0.0.1`, `::1`, `localhost`), and every redirect hop is held to the same rule. | Every update test points deck at an `httptest` loopback fake release server, so no test touches the network. A fixture that launches a deck binary for another purpose sets `DECK_UPDATE_CHECK=0` (the scenario launcher and the pty driver do by default). `cmd/deck`, `internal/tui`, `internal/update` and `features` run behind a rejecting loopback proxy (`internal/testnet`) that fails the package naming any public host a test or a deck it launched asked for. |
| **Update error redaction** | A configured release URL may carry credentials (`https://user:password@mirror/...`, or a token in the query). Every update error and message that quotes a URL (HTTP status, truncated body, size limit, unreachable host, refused scheme or unparsable URL, a wrapped `net/http` or `gh` error, and the TUI's `update failed` footer note) shows the scheme, host and path but replaces the userinfo with `***` and every query value with `***`. The userinfo masked is what Go's `url.Parse` takes as userinfo: everything up to the last `@` of the authority, whatever it holds (`'`, `@`, `(`, `)`, `!`, `$`, `&`, `*`, `+`, `,`, `;`, `=`, `~`, `%` or non-ASCII bytes); a configured URL that does not parse, or has no `://`, is masked up to its last `@`. Inside a Go-quoted URL (a wrapped `net/http` error) a backslash escape such as `\"` is part of the userinfo or query value, not its end. `update.Run`, `update.Check` and the footer note redact the whole message, so a cause formatted elsewhere is covered as well. Before any pattern runs, `update.Run`, `update.Check` and every update message that quotes the configured URL replace that URL's own userinfo, password and query values whole, wherever they appear: each raw, percent-decoded, percent-encoded and Go-quoted. A credential of any length, even one byte, is masked whole where its position is fixed by its anchor (`key=` before a query value, `://` or `:` before and `@` after a userinfo or password), so `?t=K>Z` and `?t=K Z` leave no `Z`; a secret of at least 4 bytes is also replaced wherever it appears. A configured credential of 1-3 bytes that coincides with non-secret text of the configured URL (its scheme, host, port, path or a query key, e.g. a two-byte `/q` equal to part of the path) is outside this guarantee, because that text cannot be told from the credential. Text a release server or `gh` sent back (an HTTP reason phrase, a redirect's path, a malformed `Location` header on any status that a parse error quotes, a `checksums.txt` hash field, bytes a transport or body-read error quotes, `gh` output) may echo the credential anywhere, so inside that text every configured secret, of any length and in each of the forms above, is replaced wherever it appears, and so is a prefix of one, two bytes or longer, that a closing quote cuts off (`invalid URL escape "%Q>"` quotes only three bytes of what was sent). Every cause of an error type declared outside deck (a `net/url`, `net`, `net/http` or TLS error), except a file-system, process, syscall or deadline error, counts as such server text even where deck did not mark it so; only an occurrence lying wholly inside the configured URL's scheme, host, port, path or a query key is kept, so a dial error still names the host. The status code is printed from the parsed number, never from the server's status text, and deck's own wording around server text is never rewritten. Each is then also replaced piece by piece, cut at the bytes that end a pattern match (whitespace, `"`, `<`, `>`, `\`, `/`, `?`, `#`, `&`). A credential holding such a byte (`?token=a"b`, a password with a `\`) therefore leaves no piece of itself behind. The cause (status, `unexpected EOF`, `larger than N bytes`) stays, and a failure still leaves the old binary and directory untouched. | The real-binary rows of `TestUpdateVerbRedactsMirrorCredentials`, `TestCtrlUFailureNeverShowsMirrorPassword`, `TestCtrlUFailureNeverShowsOddMirrorPassword` and `internal/update/redact_test.go` use sentinel passwords and query tokens, including ones holding `'`, `@`, sub-delims, `"`, `\`, `<` and a space, and assert no piece of them reaches stdout, stderr or the frame (`TestCtrlUFailureNeverShowsBreakingSecret` and `TestRunAndCheckRedactBreakingSecrets` for the last four). The `short …` rows of `TestUpdateVerbRedactsMirrorCredentials` and `TestCtrlUFailureNeverShowsBreakingSecret`, `TestRedactShortBreakingSecrets` and `TestRunAndCheckRedactShortSecrets` do the same for one- and three-byte delimiter-bearing query and userinfo credentials (`K>Z`, `K Z`, `Z`). `TestUpdateVerbScrubsServerEchoedCredentials` (real binary), `TestCtrlUFailureNeverShowsServerEchoedCredential` (TUI), `TestRunAndCheckScrubServerEchoedSecrets` and `TestChecksumFieldEchoIsScrubbed` use a loopback mirror that echoes a short credential (`Q>7`, `Z`, `K"Q`, `K>Z`, `R>`, `Q>Z`, a two-byte password) in a reason phrase, percent-encoded, in the latest redirect's path, in a malformed status line, in a malformed `Location` header (on a 503, a 200, the latest redirect and a download redirect) and in `checksums.txt`, and assert none of it reaches the output while the status code, the server's other words and the host stay. `TestRedactErrScrubsUnmarkedForeignErrors` covers a foreign cause no boundary marked and a local file-system cause that keeps its wording. |
| **Update attestation** | `deck update` downloads the archive and `checksums.txt` through an authenticated `gh` (`gh release download <tag> -R n-orlov/deck`) when one is on `PATH` and the base is the default one, and over plain HTTPS otherwise (also when the `gh` download fails). After the checksum check it runs `gh attestation verify <archive> --repo n-orlov/deck` on the downloaded archive when an authenticated `gh` is present; every nonzero verification result aborts, leaving the old binary byte-identical and no file beside it; this includes a release that has no attestation (`no attestations found`), which is a verification failure when `gh` is authenticated. Only when there is no authenticated `gh` (absent, or `gh auth status` fails) does the update proceed on the checksum alone and say so, unless `DECK_REQUIRE_ATTESTATION=1`, which aborts instead. (`install.sh` keeps its own, more lenient rule for a release published before attestations existed: §13.2, release attestation.) | Tests put a fake `gh` alone on `PATH` that records each call; no test reaches the real `gh` or the network. |
| **Update refusals** | `deck update` changes nothing, prints a message and never escalates privileges (no `sudo`, `doas` or `pkexec` is ever run) when: the running build's version is `dev`; the binary, before or after resolving symlinks, lies at or under one of these managed prefixes: `/usr/bin`, `/usr/sbin`, `/bin`, `/sbin`, `/usr/lib`, `/usr/libexec`, `/opt/homebrew`, `/usr/local/Cellar`, `/home/linuxbrew`, `/nix/store`, `/snap` (end of managed prefixes; Homebrew on macOS is `/opt/homebrew` or `/usr/local/Cellar`, on Linux `/home/linuxbrew`; a bare `/usr` is not listed, so a manual install in `/usr/local/bin` is updated); or the binary's directory is not writable, in which case the message names the directory and says to re-run with sudo or to reinstall to `~/.local/bin` via `DECK_INSTALL_DIR` (§13.2). The refusals are checked before any network request. | One test per refusal, each asserting the binary's sha256 is unchanged; a further test puts recording fakes for `sudo`, `doas` and `pkexec` on `PATH` and asserts none is invoked. The real-binary test in `cmd/deck/` cannot create `/usr/bin`, so it builds with `-tags deckupdatetest`, which lets `DECK_UPDATE_TEST_ROOT` relocate the tree the prefixes are matched under (never part of a release). |
| **Structured log** | JSONL to `$DECK_HOME/log/deck.jsonl`: every state transition, launch argv, hook receipt with duration, and one `event_hook` line per event-hook spawn with its exit status and the script's duration (none for a detached session-end spawn, §10.3). | The observability surface for things not visible on screen — argv, timings, retries. |
| **Launch audit** | Each launch appends the exact argv + resolved env keys (values redacted) to the log. | Proves "resume by id, never `--continue`" (R2) without reading agent internals. |

Nothing above changes behaviour; they narrow non-determinism. `DECK_*` variables are listed
in the help view.

### 13.2 Harness

```
 feature files (Gherkin)
        │  godog
 ┌──────▼───────────────────────────────────────────────────────────┐
 │ steps: keys in · screen out · files · hook calls · log           │
 └──┬──────────────┬───────────────┬──────────────┬────────────────┘
    │              │               │              │
 ┌──▼───────┐  ┌───▼──────────┐ ┌──▼───────────┐ ┌▼───────────────┐
 │ pty +    │  │ real tmux    │ │ fake agents  │ │ capture script │
 │ VT100    │  │ private sock │ │ on PATH      │ │ (event hook)   │
 │ emulator │  │              │ │              │ │                │
 └──────────┘  └──────────────┘ └──────────────┘ └────────────────┘
```

- **Driving.** `deck` runs in a pty at a fixed geometry; steps send raw keystrokes. Screen
  state is read from a VT100 emulator's cell grid, then normalised (trailing space
  stripped, non-frozen timestamps masked) before matching. Multi-client scenarios spawn N
  ptys against one `DECK_HOME`.

  **A pty is not a terminal emulator.** Bubbletea probes the terminal during start-up —
  background colour (OSC 11) and cursor position (CPR) — and *waits for the replies* before
  it renders its first frame. A bare pty transports bytes and answers nothing, so a harness
  that only reads will hang before frame one and look like a broken TUI. The harness must
  therefore answer those probes (or drive the program through something that does). This is
  measured behaviour of the toolchain, not a theory — `ci/SPIKE.md` has the evidence.

- **Where it runs.** All build and test work happens in a throwaway sibling container
  carrying Go + tmux (`ci/Dockerfile`, driven by `ci/run.sh`), with the repository
  bind-mounted from its **host** path and a named volume holding the module/build cache. Two
  constraints that are easy to get wrong and fail confusingly: a sibling container's bind
  source must be the *host* path (a container-local `/workspace` mounts empty), and commands
  must go through `sh -c`, never `sh -lc`, whose login shell resets `PATH` and loses the
  toolchain. Leaving root-owned files in the workspace is a defect, not a nuisance.
- **CI runs the same thing.** The GitHub workflows run the suite through `ci/run.sh` in the
  same `ci/Dockerfile` image as a local run, on self-hosted runners reserved for this repo,
  and only for trusted refs (pushes, schedules, and pull requests from this repository —
  never a fork's code on a self-hosted runner). A pull request's suite check gates its
  merge; a red push or nightly run on `main` alerts the operator. A failing test is retried
  once, and a pass on the retry is green but recorded as **flaky**, never hidden. A release
  tag publishes only for a sha that is an ancestor of `origin/main` (`git merge-base --is-ancestor`,
  on a checkout that fetched full history) **and** whose suite check is green: a tag on a side branch
  or an unmerged commit never publishes, and a reachability check that cannot be answered fails the
  gate rather than passing it. Only suite runs triggered by a push
  or a pull request count: nightly (schedule) and manual (`workflow_dispatch`) runs alert but
  never gate a release, so a flaky or `-race` nightly cannot block a sha whose push run passed.
  Only the `suite` check published by `ci.yml` counts: a check run named `suite` from any other
  workflow (such as `pages-pr-publish.yml`) is ignored, green or red. A run whose workflow path is
  missing, null, empty or anything other than `.github/workflows/ci.yml` (with or without GitHub's
  `@<ref>` suffix) is not `ci.yml`'s, so its suite check never gates. `release.yml` pins its
  actions by commit SHA like the other workflows. **Release attestation (R236):** `release.yml`
  attests the four tarballs and `checksums.txt` with `actions/attest-build-provenance` (pinned by
  commit SHA, tag in a trailing comment), the job holding exactly the extra `id-token: write` and
  `attestations: write` permissions. `install.sh`, after the checksum check, runs
  `gh attestation verify --repo n-orlov/deck` on the archive when an authenticated `gh` is present,
  and a verification failure always aborts with a non-zero exit and nothing installed. Without a
  usable `gh`, or for a release published before attestations existed, it prints one line saying
  only the checksum was verified and continues, unless `DECK_REQUIRE_ATTESTATION=1`, which aborts
  instead. The maintainer's `v*` tag ruleset is an operator step documented in `docs/ci.md`, never
  applied by the build. Both `FROM` lines of `ci/Dockerfile` are pinned by `@sha256` digest,
  with the tag kept in the reference, so a re-pushed tag cannot change the CI image.
  **Main's Actions history is a truthful signal:** every run on `main` is green unless the code
  is broken. A newer push to `main` cancels the superseded push run
  (one concurrency group per ref), and a cancelled push run reports nothing and notifies no one; a
  dispatch or nightly run always runs to completion, because it never shares a concurrency group
  with pushes, so a push cannot cancel it; every push to `main` runs, whatever it changes (no `paths-ignore`);
  a draft pull request starts no run (`pull_request` types `opened`, `synchronize`, `reopened`, `ready_for_review`; the `lint`, `suite`, `report`, `pr-comment` and `notify` jobs also require `github.event.pull_request.draft == false`), and the branch/draft-PR working practice is in `docs/ci.md`, "Working practice"; the PR-preview publisher never starts for a `main` run, so it leaves no skipped run behind;
  and a wall-clock budget (such as `_hook`'s 20 ms, §3.1) is asserted on normal builds only, never on
  the `-race` build. A transient failure of the hosted Pages deploy is retried, not reported as
  red. Only a superseded push or pull-request run is ever cancelled.
- **Quality gates.** CI also gates the code's own quality, through one entry point,
  `ci/run.sh ci/quality.sh`, with the tools pinned in `ci/Dockerfile`, so a local run and CI give
  the same verdict. Every gate is on only because the product already passes it **with zero
  exceptions** — there is no allow-list of offending functions, no baseline file of existing debt
  and no grandfathered code — and then prevents regression. The gates: coverage of the merged
  unit-plus-`features/` profile (total ≥ 85%, every product package ≥ 80%); CRAP (per function,
  `cc² × (1 − cov)³ + cc`) at a ratcheted ceiling that ends at 10; golangci-lint with a checked-in
  config; `govulncheck` on code deck calls; and `trivy fs` for vulnerable dependencies, secrets and
  misconfiguration, re-run by the nightly because new advisories appear without code changes. A
  gate that finds no input — no packages, an empty profile, no scored functions — **fails**; it
  never passes vacuously. Every threshold lives in one checked-in file, and a test fails if one is
  loosened relative to the merge-base. A `//nolint` names its linter and gives a reason; a
  `.trivyignore` entry gives a reason and a review-by date and fails once that date passes. A
  quality failure means the code is broken, never harness noise. Before a release tag, the
  operator may ask for an on-demand security review of the range since the last reviewed tag; a
  confirmed HIGH or CRITICAL finding blocks the release, and nothing about a review is tracked in
  the repository.
- **tmux.** A real tmux on a per-scenario socket. Steps may assert tmux facts directly
  (`session exists`, `pane command is …`, `environment contains …`) — that's observable
  outside the app. Two of those facts carry §11's central preview guarantee and are
  therefore named here: **`list-clients` is empty** while a preview is live, and
  `#{window_width}x#{window_height}` is unchanged across any amount of previewing.
- **Fake agents.** `fake-claude`, `fake-pi`, `fake-codex`, `fake-copilot` on `PATH`: tiny programs that
  honour the real argument contracts (`--session-id`, `--resume`, `--permission-mode` — and,
  for `fake-codex`, the differently-shaped one codex actually has: no id at launch,
  `resume <id>`, `-a`/`-s`, and hooks arriving as an inline `-c hooks.…` override instead of
  a settings file; and for `fake-copilot`, `--session-id <uuid>` that creates or resumes, a
  `session-state/<id>/` directory under `$COPILOT_HOME` with `workspace.yaml` at launch and
  `events.jsonl` only at the first prompt or a clean shutdown, and the hook commands of the
  `--plugin-dir` `hooks.json` run with Copilot's camelCase payloads, `sessionStart` at the first
  prompt only (a held prompt leaves the turn open until a `stop` command fires `agentStop`); it rejects `--continue`, `--resume`, `--connect`, `--remote`, `--acp` and `--yolo`
  as unknown arguments; with `FAKE_COPILOT_SCREEN=1` it draws Copilot's full-screen TUI instead of plain
  progress lines: the real start sequence (colour/version/kitty-keyboard queries, `DECRQM` 12 and 1007, alternate
  screen 1049, mouse 1003+1006, focus 1004, a BEL-terminated `OSC 0` title), the idle and `Working` footers,
  the permission and question dialogs, the folder-trust prompt (`FAKE_COPILOT_TRUST=1` starts on it) and a `✗`
  error line, each with the substrings §8.4's probe rules match; it redraws at the new size on `SIGWINCH`, and
  `Ctrl-C` during a turn aborts it back to the idle footer **without firing any hook**, as Copilot does),
  `fake-claude` keeps a session name the way Claude does: `--name`, a typed `/rename <name>` line and the
  `rename` pane command append a `custom-title` and a paired `agent-name` record (`sessionId` keyed) to the
  transcript and set the pane title `✳ <name>` with `OSC 0` (a `--name` on a conversation with no transcript
  sets the title at once and reaches the transcript with the first prompt, so a name alone never creates it); the `ai-title` pane command appends a generated
  `ai-title` record (suppressed once a name is explicit); a reopened transcript restores its last title; and
  `SessionStart`/`UserPromptSubmit` payloads carry `session_title` only for an explicit name, never a
  generated one (`@harness-names`, `features/harness_names.feature`),
  `fake-codex` keeps a thread name the way Codex does: the first prompt creates the thread row in
  `$CODEX_HOME/state_5.sqlite` (`threads.name` NULL until named); a typed `/rename <name>` line or the `rename`
  pane command sets the name, appends `{"id","thread_name","updated_at"}` to `session_index.jsonl` and sets the
  pane title `<name> | <cwd basename>` with `OSC 0` (a name typed before the first prompt is held, shown in the
  title and persisted with the thread); the `ai-title` pane command names a still-unnamed thread once and is
  ignored afterwards; no hook payload carries a name; a `stop` repaints the screen without the finished turn's
  working indicator (`@harness-names`, `features/harness_names_codex.feature`),
  write transcript files in the real on-disk layout, print recognisable pane text on
  demand, fire hook payloads at `deck _hook` on command, and can be told to hang, crash,
  or exit. They are the *contract* under test — real-agent conformance is a separate,
  tagged suite (§13.5); `fake_agent_drift_copilot.feature` is untagged, so CI itself holds fake-copilot to
  the recorded probe fixtures' key strings and to #72's payload keys. They also **record every terminal size they observe** — the initial
  one and each `SIGWINCH` — where a step can read it, which is what makes §11's fit and
  §11.9's restore assertions about the agent's own experience rather than inferences from
  tmux's bookkeeping. It is also what a scenario reads to prove the negatives: that a fit is
  coalesced to one `SIGWINCH` per settled selection rather than one per row walked, that no
  fit reaches a pane below the 7-row floor, and that `preview_fit = false` produces none at
  all.
- **Capture script.** A script registered as `event_hook` that appends its argv, a fixed set
  of `DECK_SESSION_*` and `DECK_EVENT_*` variables and its stdin payload to a file, prints one
  output line, and spawns nothing itself (shell builtins only); steps assert on invocations
  received, the payload, dedupe collapses, and non-invocation for a disabled or filtered kind.
  It is the only program any scenario makes deck run for the hook, and none reaches a
  notification service. Event-hook behaviour is fully black-box because §10 has no built-in
  service. A slow variant (it holds each invocation open on a fifo past the timeout, and is
  asserted dead after the kill) covers §10.3; the recorded exit status is asserted on the
  event's row for the clean exit, and a non-zero exit is pinned where the spawner lives
  (`internal/notify`) and in the session detail (§11.4).
- **Resize and attributes.** §11.2's "a resize re-chooses the mode" requires the driver to
  resize the pty mid-scenario (`TIOCSWINSZ` + `SIGWINCH`) and re-read the grid; §11.6's
  theme assertions require the emulator's per-cell SGR attributes, not only its text. Both
  are harness capabilities in their own right, and each is a prerequisite of the scenarios
  that depend on it rather than something those scenarios can fake.
- **Mouse events.** §11.8's bindings require the driver to synthesise **SGR (1006) mouse
  reports** into the pty — press, release, double-click within the terminal's own interval,
  wheel up/down, and motion for a seam drag — addressed by cell coordinates. This is a third
  harness capability of the same kind, and it is what keeps "no capability is mouse-only"
  checkable rather than aspirational: a click's effect is asserted against the same rendered
  grid as the keystroke it duplicates, so the two paths are proven to agree.
- **Isolation & teardown.** Per scenario: fresh `DECK_HOME`, fresh socket, fresh sink,
  fake-agent stubs reset. Teardown kills the socket and removes the root, and fails loudly
  on a leaked tmux server or a surviving child.
- **Reboot.** Real reboots don't belong in CI. `tmux kill-server -L <socket>` is the
  in-suite equivalent (from deck's point of view: every session gone, store intact) and is
  the step used by `@reboot` scenarios. A *cold* variant additionally restarts the harness
  with a stripped environment (thin `PATH`, no shell rc) to exercise §6.3. A genuine
  power-cycle check stays a tagged nightly/manual scenario, since only that catches
  `fsync`-level loss.

### 13.3 Feature layout

One file per area of behaviour, named for the area — never for the phase or the change that
introduced it, so a file is renamed only if the behaviour it covers is redefined:

```
features/
  harness.feature               the driver itself: the pty answers OSC 11/CPR, the grid is
                                readable, and isolation/teardown are per-scenario
  walking_skeleton.feature      the real binary starts, renders a frame, exits cleanly
  determinism.feature           §13.1 — every DECK_* control does exactly what it claims
  store.feature                 §4 — schema, migrations, targeted-UPDATE discipline
  tmux_contract.feature         §3.2 — private socket, server options, naming, env
  fake_agent.feature            the §13.2 fixtures honour the real argv contracts
  fake_agent_drift.feature      a fixture that stops matching its real CLI fails loudly
  agent_session.feature         registry → launch argv → live pane, per adapter kind
  create_session.feature        agent choice, cwd, args, env, pre_launch, name collisions,
                                §11.7 recent-cwd prefill/cycling and ghost/tab completion
  same_directory.feature        R2 — N sessions, one cwd, no conversation cross-talk
  durable_identity.feature      R3 — @reboot: stopped·resumable, resume by id, no autostart
  resume_failure.feature        §9.1 — unknown id, missing cwd, agent gone: error, not fresh
  launch_lease.feature          §9.3 — CAS acquire, TTL, stale-break, held vs not-leasable
  lease_race.feature            §9.3 — two clients press r, exactly one launch
  concurrency.feature           R4 — N clients, propagation, SIGKILL survival
  permission_modes.feature      §5 — profile → argv mapping, badge, yolo gate with no confirm,
                                yolo_default opens the modal on yolo and is inert without the
                                gate, degradation
  status_claude_hooks.feature   R6 — waiting/running/idle/error via hook payloads, live badge
  status_probe.feature          R6 — sampled badge, staleness, precedence over probe
  crash.feature                 §7 — error + crash tail + event hook, and never auto-relaunch
  environment.feature           §6 — layering, env↻, restart applies (and only restart)
  kill_delete_undo.feature      §9.2 — x/dd, undo windows, tombstone, cwd never touched,
                                reap leaves no trace, the agent's transcript survives `dd`
  shell_state.feature           §9.4 — history, scrollback replay, cwd restore, sensitive
  event_hooks.feature           §10 — a capture script as the hook: enable/disable, allow-list
                                filtering before the spawn, a session's list replacing the
                                global one, inert with no script, timeout kill recorded on the
                                event, session-end never waited for, epoch dedupe, and the
                                TUI's own killed event
  codex_hooks.feature           §8.2 — inline hook injection, the id adopted from
                                SessionStart, PermissionRequest → waiting, and no id (so no
                                resume) before the first prompt
  layout_modes.feature          §11.2 — auto selection, | cycling, resize re-choice, floors
  preview.feature               §11 — coalesced fit, floor-skip, preview_fit=false is passive,
                                no attached client, no scroll, crop when unfitted, crash tail
                                for error, placeholder with no pane
  attention_sort.feature        §7/§11 — attention order, the collapsed strip's count,
                                space walks what needs me
  session_groups.feature        §11 — manual groups: alphabetical with default last or first,
                                counts including (0), collapse persisted, header as a cursor
                                stop and fold/unfold by keyboard, create/move/edit/delete
  mouse.feature                 §11.8 — click selects and enters interactive, wheel scrolls
                                without selecting, the header click toggling collapse in list
                                mode and in interactive mode alike, seam drag resizes, preview
                                drag selects and release copies, DECK_MOUSE=0 disables
  attach_on_click.feature       §11.8 — [ui] attach_on_click off: click and double-click only
                                select, the next key goes to the list
  settings.feature              §11.5 — schema-generated fields, explicit save, atomicity
  config_reload.feature         §6.5 — one client's saved theme reaches a second client live
  themes.feature                §11.6 — picker, live preview/revert, fallback says so,
                                quantised rendering under DECK_COLOR_DEPTH=16
  search.feature                §12 — metadata/events/transcripts, resume from a hit
  health.feature                §9.5 — no tmux, old tmux, missing agent, PATH unresolvable
  real_agent_smoke.feature      @real-agents — the thin conformance subset (§13.5)
  real_agent_copilot.feature    @real-agents — the installed Copilot CLI (§8.4, §13.5)
  real_agent_names.feature      @real-agents — the upstream session-name contracts (§13.5)
  fake_agent_drift_copilot.feature  untagged — fake-copilot's footer/dialog/error strings hold the key
                                substrings of the recorded probe fixtures and its hook payloads carry the
                                keys #72 recorded, so a changed fake string or key fails normal CI
```

Tags: `@reboot`, `@slow`, `@multiclient`, `@nightly`, `@real-agents`. Default CI run
excludes `@nightly` and `@real-agents`.

### 13.4 The three scenarios that define the product

```gherkin
@reboot
Scenario: three sessions in one directory keep their own conversations   # R2 + R3
  Given a working directory "~/work/svc"
  And sessions "alpha", "beta", "gamma" for agent "claude" in that directory
  And each has exchanged a distinct message with its agent
  When the tmux server is killed                      # CI stand-in for a host reboot
  And deck is restarted
  Then all of "alpha", "beta", "gamma" show status "stopped" labelled "resumable"
  And no tmux session exists                          # nothing auto-started
  When I resume "beta"
  Then the launch audit for "beta" contains "--resume <beta.conversation_id>"
  And the launch audit for "beta" does not contain "--continue"
  And "beta" replays its own last message, not "alpha"'s

@multiclient
Scenario: two clients cannot double-launch one session                  # R4
  Given clients "A", "B", "C" attached to the same deck home
  And a stopped session "triage"
  When "B" and "C" both press "r" within 100ms
  Then exactly one launch appears in the log
  And the other client shows "starting elsewhere"
  And after 1 reconcile all three clients show "triage" as "starting"
  When the agent for "triage" fires "session_start"
  Then after 1 reconcile all three clients show "triage" as "running"

Scenario: waiting is truthful, deduped per episode, and cleared         # R6
  Given a running session "api" for agent "claude"
  And an event hook enabled for kind "waiting"
  When the agent fires notification type "permission_prompt"
  Then "api" shows "waiting" with reason "permission_prompt" within 1 reconcile
  And the event hook is invoked 1 time with kind "waiting"   # immediate, no debounce
  When the same notification type fires again
  Then the event hook has still been invoked 1 time             # dedupe, same epoch
  When the agent fires "stop" with message "done"
  Then "api" shows "idle" and the detail pane contains "done"
  When the agent fires notification type "permission_prompt" again
  Then the event hook is invoked a 2nd time                     # new epoch after resolution
```

And three that pin the distinctions most easily got wrong — a clean exit read as a crash, a
crash read as nothing at all, and two conversations collapsed into one:

```gherkin
Scenario: a shell session that exits cleanly is stopped, not an error    # §7
  Given a session "notes" for agent "shell"
  When I type "exit" in its pane
  Then "notes" shows status "stopped"
  And the event hook is not invoked
  And no crash tail is recorded

Scenario: a crashed agent is an error with its exit status               # §7
  Given a running session "api" for agent "claude"
  When its process is killed with SIGKILL
  Then within 1 reconcile "api" shows "error"
  And the crash tail contains the last lines that were on the pane
  And the launch count for "api" is still 1                   # never auto-relaunch

@codex
Scenario: a Codex row adopts the id its own first hook reports           # R2 + §8.2
  Given a working directory "~/work/svc"
  When I create Codex sessions "one" and "two" there within 2s
  Then each row has no conversation id, reads "sampled", and refuses resume
  When each session is prompted once
  Then each row reads "live" with the distinct id its own SessionStart carried
  And neither row's id is the other's transcript
  When "one" asks to run a command that needs approval
  Then "one" shows "waiting" with the requested tool name as its status reason
```

### 13.5 Coverage beyond the headline scenarios

Also specified as features, one scenario per rule: resume-argv per adapter (including
Codex's refusal to resume a row whose first hook has not yet reported an id, and the ban on
"most recent"); `_hook`'s store write inside its
budget, measured from monotonic log durations, with a separate scenario for the session-end
detached-dispatch path; store migration from the previous schema version; scrollback replay
identical modulo the cap, absent entirely when `sensitive`, and off by default for agent
sessions; capture ownership (kill, session end, TUI shutdown, opportunistic) each covered;
env edit shows `env↻` and applies **only** after restart, with the conversation preserved;
`captured_path` precedence, and `login_shell` overriding it; the event hook receives the §10.1 contract (argv, env, versioned stdin), is
filtered before spawning, respects a per-session list replacing the global one, is inert
with no script configured, is killed at its timeout with its exit status and output tail
recorded rather than silently no-op'ing, and never delays the session-end path; `s` refused from every status except `idle`;
`A` on a live session opening a confirm that names the kill and **writing nothing until it is
confirmed**; resume and restart refused for an archived session, asserted by the *absence of a
tmux session* rather than by the returned outcome; unarchive returning a row to the default
list without a filter; a user kill not resurrected by a late-arriving hook; a crash verdict
not outliving the pane it describes, so a resumed session reconciles and its hooks apply;
every keybinding in §11 has at least one scenario; health view on a box with no tmux, tmux
older than the minimum, a missing agent binary, an agent unresolvable from the unit's
`PATH`, no session bus, and linger disabled.

**`@real-agents`** runs a thin conformance subset against actually-installed agent CLIs —
does `--session-id` still exist, does the hook payload still carry the fields §8.1 relies
on, is the transcript still where the adapter looks. It is expected to break when an agent
upgrades: that's its job. Kept out of the default run so upstream churn never blocks a
commit.

The Copilot half (`real_agent_copilot.feature`) runs against the installed `copilot` with a
temporary `COPILOT_HOME` (never `~/.copilot`). It asserts that `--session-id <uuid>` creates
the session directory and that the same argv resumes it with no second directory, that a
session SIGKILLed before its first message is relaunched by Resume's argv, that deck's plugin
hooks deliver `userPromptSubmitted` and `agentStop`, and that a live pane carries the key
substrings of each of the six probe fixtures (trust, permission, question, working, error,
idle); a unit test keeps those substrings inside the recorded fixtures. Its first step skips
the scenario with a stated reason when `copilot` is not on `PATH` or cannot answer one
non-interactive prompt (not logged in), and no reason or log line ever carries a token. A
unit test proves the skip fires for a missing binary, so the default run cannot fail for it.

**Name contracts (R265).** `real_agent_names.feature` pins, against the installed CLIs, the
four upstream stores deck reads session names from (§8): Claude's `custom-title`
(`customTitle`), `agent-name` (`agentName`) and `ai-title` (`aiTitle`) transcript records,
Codex's `state_5.sqlite` `threads.name`, Copilot's `workspace.yaml` `name` and `user_named`,
and Pi's `session_info` session entry (`name`). Each Given step skips its scenario with a
stated reason when the CLI is not on `PATH`, and the assertions name the record, key or
column that moved. Claude's `ai-title` is generated only for a launch without `--name`
(an explicit name suppresses it, R254), so its scenario creates the session unnamed (blank
name, `name_source = derived`), while `custom-title` and `agent-name` are asserted on an
explicit launch after `/rename`; a non-live test (`TestRealAgentNamesFeatureSetupMatchesExplicitNameSuppression`)
checks the feature file against that rule. **Degrade rule:** a broken contract (a renamed record type or field, a
missing column or table, a renamed yaml key, a Pi event without its name field) never
renames the row, never changes its status and never raises an error: the row keeps its dummy
name (`name_source` stays `derived`). CI never runs `@real-agents`, so that rule is proved in
the default tier by `TestBrokenContract*` in `internal/service`, one per harness, driven
through the real adoption path and without `DECK_GODOG_TAGS`; the default tag filter is
unchanged.

**Fixture corpus.** Probe heuristics (§7) are driven by captured pane text per agent per
state, stored as fixtures and asserted through the UI badge, so a spinner or prompt
redesign upstream is a one-fixture fix.

---

## 14. Open questions

1. **Pi's remaining events.** Pi's extension API (§8.3) gives the hook events a Pi row needs
   for live status, but none for a permission prompt or a failed turn. Whether pi exposes them,
   and so whether the probe corpus can shrink, is open. **Codex's spike has been run** and its
   answer is §8.2.
2. **Codex hook trust without a dangerous flag.** §8.2 pays `--dangerously-bypass-hook-trust`
   because the `trusted_hash` preimage was not recoverable and deck's hook command embeds a
   machine-specific path. If a future codex exposes a non-interactive trust command, or if
   the hash algorithm becomes known, deck can seed trust instead and drop the flag. Related
   and also open: whether `acceptEdits`, `plan` or `dontAsk` are reachable at all on codex
   (no flag combination produced them; `--approve-for-me` and the config's own
   `permission_profile` keys are untested), which is the only route to a real `plan` profile
   for codex rather than §5's degrade-to-`safe`.
3. **Scrollback default.** 5 000 lines with escapes is generous and writes screen contents
   to disk. Smaller default, or `sensitive` inverted (opt-in capture)?
4. **Probe cadence when no TUI runs.** Non-hook agents go unclassified while unattended.
   Accept, or allow an opt-in periodic probe (which reintroduces something daemon-shaped)?
5. **Fork-on-resume.** Resuming a conversation that another client already resumed can, on
   some agents, produce divergent transcripts. Detect and offer fork, or refuse the second
   resume via the lease?
6. **Binary name** `deck` vs Kong's `decK` on `$PATH`.
7. **Immediate event hook vs noise.** Dropping debounce (§10.3) means a prompt answered in
   three seconds still pinged you. Acceptable, or is a *resolution* event ("no longer
   waiting") the better shape — the same information without needing a timer?
8. **Shared attach geometry** (§3.3). Living with `window-size latest` is still the plan, and
   §11.9 has now shown a bounded resize is survivable and byte-exactly reversible, at exactly
   two `SIGWINCH` per cycle. What remains is the **bystander squeeze**: one window has one
   size, so fitting it for one viewer necessarily squeezes anyone already attached. deck
   **refuses rather than inflicts it** (§11.9's first refusal). If two-terminals-at-once turns
   out to be a daily annoyance rather than a rare one, the only real fix is one tmux session
   per client per agent, which is a different architecture.
9. **Is ~53 MiB of resident memory per gridded pane acceptable?** Measured for one 120×40
   emulator. deck has no memory budget to judge it against, and it is the one axis on which
   §11.9's grid is materially worse than polling.
10. **Do real agents repaint their full transcript on widening?** If not, the alternate
    screen's lack of scrollback makes §11.9's fit destroy transcript rows irreversibly —
    measured at 19 of 40 rows against synthetic programs. Unmeasured against a real agent:
    every spike was fenced from launching one.
