# deck — product spec

A terminal session manager for CLI coding agents. Named sessions with durable
conversations, an attention-sorted list, and N concurrent clients — on one host, with
tmux as the only runtime dependency.

Interaction model: **`deck` is a TUI. Every user action happens in the UI.** There is no
user-facing command line.

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
| **R5** | **Lightweight, portable.** One static binary plus tmux, on any Linux. | Go, no cgo, no node, no browser, no Docker, no root, no daemon. systemd is *optional*. No assumption about shell, distro, terminal, or network. |
| **R6** | **Four agents:** Claude Code, Pi / oh-my-pi, Codex CLI, and a plain `bash` shell session. | Adapter interface with capability degradation — hooks where they exist, pane heuristics where they don't, and honest UI about the difference. |
| **R7** | **TUI-only.** All actions in the UI: create, resume, kill, delete, env edit, permission mode, pin, search, config. | Discoverability is a feature, not a nicety: inline help, no hidden verbs a user needs. |
| **R8** | **Black-box testable, BDD-specified.** Every behaviour in this spec is expressed as Gherkin and verified against the **real binary** driven through a real terminal, with no in-process hooks and no test-only code paths in the product. | The binary must be *drivable* (keystrokes in) and *observable* (rendered screen, tmux state, files, outbound webhooks, structured log) from outside. Determinism controls — state dir redirection, frozen clock, fixed tick, no animation, no colour — are documented, supported configuration, not test scaffolding (§13). |

### Non-goals (out — do not add)

Git worktrees, branches, PRs, CI · web UI / HTTP server / PWA / tunnels · Docker or
sandboxing · ACP or any structured-render protocol · plugins · **a theme *engine***
(themes are colour-only data files, and a theme can change nothing but colour — §11.6) · **a
user-facing CLI or scripting surface** · **multi-host / remote sessions** · declarative
config files describing the session set · multiple windows or a shell drawer per session
(one agent or shell per session, full stop) · orchestration, task queues, kanban,
auto-approval · env profiles or secret-manager integrations · auto-restart on crash ·
idle reaping or any timer that stops a running session · **inbound remote control**
(notifications are one-way; see §10) · cost/token dashboards · MCP management · Windows.

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
- Store: `modernc.org/sqlite` (pure Go, no cgo) — `WAL`, `busy_timeout=5000`, `foreign_keys=ON`.
- tmux via the `tmux` CLI. No control mode in v1. **Minimum tmux 3.2**, required for
  `remain-on-exit failed` (§7) and the `window-size` option (§3.3).
- Runtime deps: `tmux`, plus whichever agent CLIs the user has. Nothing else.
- Test tooling (never linked into the release binary): `cucumber/godog` for Gherkin, a
  VT100 emulator for screen parsing, `net/http/httptest` for webhook capture, and the
  `cmd/fake-*` agent binaries. The harness drives the real `deck` binary (§13).
- Paths: XDG with fallbacks — `$XDG_DATA_HOME/deck/` (default `~/.local/share/deck/`),
  `$XDG_CONFIG_HOME/deck/config.toml`, `$XDG_STATE_HOME/deck/log`. `state.db` is `0600`.

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
internal/agent/codex.go   post-launch id discovery
internal/agent/shell.go   bash/zsh/fish session, history + scrollback + cwd
internal/hookrecv/        stdin JSON → store event → notify dispatch
internal/notify/          channel abstraction: webhook | command | desktop
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
             │ sessions · events · outbox│      │ spawned by the agent's hooks;│
             └────────────┬──────────────┘      │ writes status, sends notifs  │
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
the agent's own hook system spawns. Notifications are dispatched by that same process
(§10). If no TUI is running, hook-instrumented agents still record status and still
notify. Liveness is reconciled by whichever TUI is running, and lazily by `_hook`.

**Honest limitation to surface in the UI:** agents without a hook mechanism (Pi, Codex,
`bash`) are classified by pane heuristics, which only run while a TUI is open. Their rows
show a "sampled" indicator; a Claude row shows "live". Do not paper over this.

### 3.1 Hidden internal verbs

Not a CLI. These exist because external systems need an executable to invoke, and are
undocumented in the UI, excluded from help, and prefixed `_`:

| verb | invoked by | contract |
|---|---|---|
| `deck _hook` | agent hook config | reads one JSON object on stdin, writes one status update + one event, then dispatches or enqueues notifications, then — on the non-session-end path only — runs one bounded liveness pass before exiting, which is what "lazily by `_hook`" in §3 and "the next `_hook` invocation" in §7 mean. It never probes: pane heuristics are the TUI's, and putting them on the agent's critical path would also falsify §10.3's second limitation. **Two separate budgets:** the store write completes in < 20 ms **uncontended** (measured on a monotonic clock, §13.1 — under multi-client write contention SQLite may legally hold a writer up to `busy_timeout`, so the budget assertion belongs in a single-writer scenario, not a `@multiclient` one); notification dispatch is bounded separately by the channel timeout (§10.3) and is skipped entirely on the session-end path. |
| `deck _serve-tmux` | optional systemd unit | starts the `deck` tmux server with the right server options and exits. |
| `deck _debug ...` | developers | inspection helpers, built only with the `debug` build tag. Not in release binaries. |

### 3.2 tmux contract

Sessions live on a dedicated socket, `tmux -L deck`, never the default one.

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
  enforced in SQLite. Rename renames both. **`.` and `:` are excluded from slugs** —
  tmux rejects them in session names because they are target-syntax separators.
- **A blank name in the create modal is filled in, not rejected.** deck defaults it to
  `<workspace>-<MMDD-HHMM>` from local wall-clock time (`deck-0820-1443`), because the
  common case is "start something here, now", and making the user invent a name first is a
  toll on the product's fastest path. Names are unique, so a collision appends the smallest
  free `-2`, `-3` suffix rather than failing the create. Two consequences worth stating: the
  default is only *derived*, not special — rename (§11.4) treats it like any other name; and
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

---

## 4. Data model

`$XDG_DATA_HOME/deck/state.db`, mode `0600`, schema version in `meta`.

```sql
CREATE TABLE sessions (
  id                 TEXT PRIMARY KEY,      -- deck's own uuid, stable forever
  name               TEXT NOT NULL UNIQUE,
  slug               TEXT NOT NULL UNIQUE,  -- tmux session = deck_<slug>
  cwd                TEXT NOT NULL,         -- create-time, never overwritten; NOT unique (R2)
  last_cwd           TEXT,                  -- pane's cwd at the last capture (§9.4); resume target
  agent              TEXT NOT NULL,         -- claude | pi | codex | shell
  launch_args        TEXT NOT NULL DEFAULT '[]', -- JSON array, extra agent args
  env                TEXT NOT NULL DEFAULT '{}', -- JSON map, per-session overrides
  env_dirty          INTEGER NOT NULL DEFAULT 0, -- edited while running → restart to apply
  captured_path      TEXT NOT NULL,         -- PATH at create time (§6.3)
  pre_launch         TEXT,                  -- one shell line run in the pane before the agent
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
  notify_epoch       INTEGER NOT NULL DEFAULT 0, -- bumped when an attention state resolves (§10.2)
  last_message       TEXT,                  -- last assistant message, truncated 2 KiB
  sensitive          INTEGER NOT NULL DEFAULT 0, -- suppress scrollback capture (§8)
  notify_rules       TEXT,                  -- JSON override of global rules (§10); NULL = inherit
  important          INTEGER NOT NULL DEFAULT 0, -- eligible for "milestones only" rules
  workspace          TEXT,                  -- free-text grouping label
  snoozed_until      INTEGER NOT NULL DEFAULT 0,
  acknowledged       INTEGER NOT NULL DEFAULT 1,
  launch_lease_owner TEXT,                  -- pid@boot_id#generation holding a start (§9.3)
  launch_lease_until INTEGER NOT NULL DEFAULT 0,
  last_probe_at      INTEGER NOT NULL DEFAULT 0, -- last pane sample that matched no §7 rule; a column, never an event (§7)
  created_at         INTEGER NOT NULL,
  last_attached_at   INTEGER NOT NULL DEFAULT 0,
  archived_at        INTEGER NOT NULL DEFAULT 0,
  deleted_at         INTEGER NOT NULL DEFAULT 0  -- tombstone; purged after grace (§9.2)
);

CREATE TABLE events (               -- append-only within a retention bound: audit trail, search corpus, notify source
  seq        INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT REFERENCES sessions(id) ON DELETE CASCADE,
  at         INTEGER NOT NULL,
  kind       TEXT NOT NULL,         -- started|prompt|waiting|idle|error|ended|resumed|killed|env|note
                                    -- closed vocabulary: a kind absent from this list is a defect, not an extension
  reason     TEXT,
  payload    TEXT                   -- bounded JSON
);

CREATE INDEX events_at ON events(at DESC, seq DESC);         -- §12's newest-first reads are never a full scan
CREATE INDEX events_session_kind ON events(session_id, kind); -- §6.4's env-apply reads likewise

CREATE TABLE outbox (               -- notifications; dispatched inline, retried opportunistically
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT, at INTEGER NOT NULL,
  channel    TEXT NOT NULL, kind TEXT NOT NULL,
  body       TEXT NOT NULL,         -- rendered payload
  dedupe_key TEXT UNIQUE,       -- session:kind:reason:notify_epoch — see §10.2
  sent_at    INTEGER NOT NULL DEFAULT 0,
  attempts   INTEGER NOT NULL DEFAULT 0,
  last_error TEXT
);

CREATE TABLE ui_state (             -- machine-local UI state, never in config.toml (§6.5)
  key        TEXT PRIMARY KEY,      -- layout_mode, sidebar_width (§11.2)
  value      TEXT NOT NULL
);

CREATE TABLE recent_cwds (          -- the create modal's directory history (§11.7)
  path       TEXT PRIMARY KEY,      -- resolved absolute path, deduplicated
  used_seq   INTEGER NOT NULL       -- monotonic, NOT a timestamp: order stays assertable
);                                  -- under a frozen DECK_CLOCK (§13.1)
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

---

## 5. Permission modes

A deck-level profile per session, translated per adapter — never a boolean.

The flag names below are upstream contracts, not deck's: Claude's `--permission-mode`
accepts `manual | plan | acceptEdits | auto | dontAsk | bypassPermissions`. Keeping this
table true as those CLIs move is the job of the `@real-agents` suite (§13.5), never of a
reader's memory. Codex's approval surface is **unverified** — candidates are
`--ask-for-approval`, `--full-auto`,
`--dangerously-bypass-approvals-and-sandbox`; the adapter declares no `edits`/`yolo`
support until one is confirmed.

| deck profile | Claude Code | Pi | Codex | shell |
|---|---|---|---|---|
| `safe` (default) | `--permission-mode manual` | default | default | n/a |
| `plan` | `--permission-mode plan` | n/a → falls back to `safe`, shown in UI | n/a | n/a |
| `edits` | `--permission-mode acceptEdits` | `--approve` | unverified → unsupported | n/a |
| `yolo` | `--permission-mode bypassPermissions` | `--approve` | unverified → unsupported | n/a |

- Prefer the structured mode flag over a `--dangerously-*` flag where both exist: same
  effect, less flag-name churn. Unsupported profiles degrade to the nearest safe one and
  say so in the row detail rather than silently lying.
- **Persisted**, so a `yolo` session comes back `yolo` on resume. That's the point, and
  that's why it needs a badge visible in the list, in the detail pane, and in every
  notification body.
- `yolo` is offered when `allow_yolo = true` in config (default false), and then needs no
  further ceremony: choosing it in the create modal, or switching to it with `P`, takes
  effect directly. With `yolo_default = true` (default false) the create modal opens already
  on `yolo`; it is inert while `allow_yolo` is false, and settings says so on the row rather
  than silently ignoring it. The gate is a deployment decision, not a per-launch speed bump —
  a confirm on every create trains the user to press it, and the safeguard that survives
  habituation is that the profile is *visible* everywhere it applies (below), not that it is
  tedious to choose.
- Claude hook payloads carry `permission_mode`, so if the user changes it in-session the
  row is reconciled from the hook instead of drifting.
- In `yolo`, permission prompts never fire, so the `waiting` column goes quiet — attention
  then comes only from questions / needs-input notifications. Document this in help; it is
  a frequent "status is broken" false alarm.
- Adapter capabilities are declared, not assumed: each adapter reports which profiles it
  supports, and the create modal only offers those.

---

## 6. Environment

Flat per-session overrides. No profiles, no bundles, no secret managers (non-goals).

### 6.1 Layers

Resolution, lowest to highest: environment of the process that started the tmux **server**
→ `[env]` in `config.toml` → session `env` map. The env editor shows the effective value
per key with its winning layer.

### 6.2 Editing while running

tmux env changes reach only *new* processes, so a mid-flight edit is inherently
restart-to-apply:

1. Edit in the TUI (`e`) → writes the session `env` map, sets `env_dirty = 1`, mirrors to
   `tmux set-environment -t`.
2. The row shows an `env↻` badge: *changed, not yet applied*.
3. `R` restarts the pane and relaunches with the **resume** argv — new environment, same
   conversation. For `shell` sessions, `R` also offers "inject instead" (`export K=V` into
   the live shell), which genuinely works there.

Nothing is applied silently. A restart is always an explicit keypress.

### 6.3 The PATH trap

A tmux server started by a systemd user unit inherits the user manager's environment:
thin `PATH`, no shell rc files, no agent SSH socket, no keyring. This is the single most
common reason a resumed session fails to launch. Mitigations, all three:

- `captured_path` records the `PATH` in effect when the session was created. It sits
  **between** the server environment and `config.toml`'s `[env]` in the §6.1 order: it beats
  the (possibly thin) inherited `PATH` and loses to any `PATH` the user sets in `[env]` or
  in the session's own env map. Full order, lowest to highest: server env → `captured_path`
  → config `[env]` → session `env`.
- `login_shell = true` runs the pane command through `$SHELL -lc`, giving a full login
  environment where that's wanted. **It also lets rc files rewrite `PATH`, discarding
  `captured_path`** — that is the trade, it is what the option is *for*, and the two are
  therefore mutually exclusive by design: enabling `login_shell` marks `captured_path`
  advisory and the health view says so.
- The health view flags any session whose agent binary is not resolvable from the
  environment the server will actually use, and flags a session where `login_shell` and an
  explicit `PATH` override disagree.

### 6.4 Secrets

Session `env` values are stored literally in `state.db`. Therefore:

- `state.db` is `0600`; the parent directory is `0700`.
- Values whose key matches `*TOKEN*|*SECRET*|*KEY*|*PASSWORD*|*CREDENTIAL*` are masked in
  every view and in notification bodies; reveal is a per-view explicit toggle.
- Env values never enter `events`, notification payloads, or logs.
- `pre_launch` exists precisely so secrets need not be stored at all: one shell line run
  in the pane before the agent starts (typically sourcing a file the user already keeps
  outside deck). Recommended in help over putting tokens in `env`.

### 6.5 The config file

One file, `$XDG_CONFIG_HOME/deck/config.toml`, with a declared schema:

| where | keys |
|---|---|
| top level | `allow_yolo` (default false, §5), `yolo_default` (default false, §5 — inert unless `allow_yolo`), `stale_after` (default 45 s, §7), `capture_min_interval` (§9.4), `tmux_mouse` (default true, §3.2 — `false` restores tmux's own default and with it the arrow-key behaviour), `event_retention_days` (default 30, §12) |
| `[env]` | the middle PATH/env layer (§6.1) |
| `[ui]` | `theme` (§11.6), `ascii` (§11), `mouse` (default true, §11.8), `preview_fit` (default true, §11), `group_by_workspace` (default true, §11), `sort_order` (default `"attention"`, one of `attention`/`created`/`activity`/`name`, §11), `recent_cwd_limit` (default 5, §11.7). **Not** `layout_mode`, `sidebar_width` or the recent-directory list itself — those are machine-local UI state/history and live in `state.db` (§11.2, §11.7), so a keypress never rewrites this file |
| `[notify]` | channels and rules (§10) — structured tables, edited via their own dialog (§11.5) |

Environment always outranks the file: `DECK_ASCII` set in the environment overrides
`[ui] ascii`, as every `DECK_*` knob overrides its file counterpart (§13.1 depends on
this — the harness must be able to pin behaviour regardless of what a config file says).

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
| `starting` | first agent signal | `running` |
| `starting` | no signal for `stale_after` | stays `starting`, becomes probe-eligible |
| `starting` | pane is alive, **`shell` rows only** | `running` — see the shell rule below |
| `running` | needs input: permission prompt, question, idle prompt | `waiting` |
| `running` | turn finished cleanly | `idle` |
| `running` | turn or API failure | `error` |
| `waiting` | answered: probe verdict, or the user attaches | `running` |
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
| `waiting` | **needs me** — permission prompt, question, idle prompt | bright, in the attention count, sorted first |
| `idle` | turn finished cleanly | check, `last_message` in the detail pane |
| `error` | turn or process died | red, unseen marker sticks, crash tail captured |
| `stopped` | record alive, no tmux session (post-reboot, killed) | grey, labelled *resumable* |
| `archived` | hidden from the default list, retained; **not startable** — unarchive (`U`) to resume | hidden behind a filter |

Rules:
- **Precedence:** `user-terminal` > `hook` > `probe` > `tmux`. A user kill sets
  `killed_by_user`, which an in-flight hook arriving milliseconds later cannot undo —
  explicit human action outranks automation. Below that, a probe never overwrites a fresher
  hook verdict, and `tmux` only ever supplies liveness.
- `waiting` and `error` set `acknowledged = 0`; cleared by attaching or by `Y`. "Attaching"
  means both ways the keyboard reaches the pane through deck: `a`'s full attach and `↵`'s
  interactive preview (§11.9) — the same durable transaction applies to either. Leaving an
  attention state bumps `notify_epoch` (§10.2).
- **Attaching to a `waiting` row also clears the status to `running`** (not only the
  acknowledgement): answering the prompt is why you attached, deck watched you do it, and
  no hook fires on a prompt being answered — the subscribed hooks (§8.1) are per-turn, not
  per-tool-call, so the next event is the turn's `stop`. Without this rule a row you just
  unblocked stays `waiting` until the turn ends — a standing false positive in the one
  signal the product exists to provide.
- Staleness: a `starting`, `running` **or `waiting`** session with no event for
  `stale_after` (default 45 s) becomes eligible for probing. For non-hook agents, probing
  is the only source. `waiting` is included for the unattended half of the same hole: a
  prompt answered from a raw `tmux attach` (not through deck) fires no hook either, so the
  probe is what corrects it. This does not violate precedence — by the time a `waiting` row
  is probe-eligible, the hook verdict it would override is at least `stale_after` old, and
  "a probe never overwrites a *fresher* hook verdict" is unchanged.
- Probe heuristics live in one table-driven file with golden-file tests over captured pane
  text — a fixture corpus per agent, so a spinner or prompt redesign upstream is a
  one-fixture fix.
- **Liveness and clean-vs-crash exit.** `remain-on-exit failed` (§3.2) means a pane whose
  command exits **0** is destroyed with its session, while a pane exiting **non-zero**
  stays as a dead pane. So the 500 ms reconcile (`list-sessions` + `list-panes -F` with
  `pane_dead` / `pane_dead_status`) distinguishes the two without guessing:

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
  agent fires no hook, so the transition to `error` — and its notification — happens on the
  next TUI tick or the next `_hook` invocation for that session, whichever comes first.
  Stated plainly rather than implied to be live. (`StopFailure` *is* a hook, so ordinary
  turn/API failures do notify unattended; process death does not.)
- **Never auto-relaunch** (non-goal): a crash loop must not be able to burn tokens or retry
  a destructive action.

**A diagnostic sampling result is a column, not an event.** A probe that matches no rule
changes no status and is recorded only by overwriting `sessions.last_probe_at`, which is
what the `i` detail dialog reads. It MUST NOT append to `events`: a per-tick row with no
reader is unbounded write amplification, and at §6.5's default reconcile cadence one
unmatched session alone produces two rows a second forever. The general rule the events
table is held to: **a kind is only written if something reads that kind**, and the reader is
named where the kind is introduced.

---

## 8. Agent adapters

An adapter turns a session into **argv**, and nothing else: it never starts a process, never
writes to the store, and never reaches into the TUI. `internal/service` owns the pane and
the row; `internal/tui` consumes adapters only through this interface and the registry, so
adding an agent kind is one file plus one registry entry and no TUI change (R1).

```go
type Adapter interface {
    Kind() string                                      // claude | pi | codex | shell
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
| `DiscoverID(ctx, in, since) (string, error)` | post-launch conversation-id discovery (§8.2) |
| `TranscriptPaths(in) ([]string, error)` | cross-session search over transcripts (§12) |

| | Claude Code | Pi / oh-my-pi | Codex CLI | shell (bash/zsh/fish) |
|---|---|---|---|---|
| **conversation id** | **deck assigns**: `--session-id <uuid>` | **deck assigns**: `--session-id <id>` (created if missing), plus a display name | agent mints it | none |
| **resume** | `--resume <uuid>` (fork = new id, offered explicitly) | `--session-id <id>` | `resume <id>` by id | recreate shell (§9.1) |
| **id discovery** | not needed | not needed | **§8.2** — serialised, claim-based; ambiguity is a first-class outcome | n/a |
| **status** | **hooks → `deck _hook`** (live) | probe (sampled) | probe (sampled) | probe (sampled) |
| **banned** | `--continue` | `--continue` | `resume --last` | — |

### 8.1 Claude instrumentation

Hooks are injected **per session** via the settings-on-the-command-line mechanism. Nothing
is written to the user's global settings file: a deck session is instrumented, an ordinary
agent run in the same directory is untouched.

Precisely: hook entries **merge** across settings sources rather than overriding them, so
`deck _hook` is *added* to whatever the user's and the project's settings already define.
Two consequences to design for rather than discover: a project's own hooks fire inside deck
sessions too, and on the session-end path they share the same budget `_hook` is racing —
which is one more reason that path only enqueues (§10.3).

Subscribed events — a handful per turn, never per tool call:

| event | → status | notes |
|---|---|---|
| session start | `running` | confirms the assigned id; the source field distinguishes fresh vs resumed vs compacted |
| user prompt submitted | `running` | also feeds prompt count |
| notification (permission prompt / question / needs-input / idle prompt) | `waiting` | **the golden signal**; the notification type becomes `status_reason` |
| stop | `idle` | carries the last assistant message — use it; the transcript file lags |
| stop-failure | `error` | error type becomes `status_reason` |
| session end | `stopped` | **Session-end hooks share a ~1.5 s total budget**, raisable to ~60 s only by declaring a longer per-hook timeout. deck declares a modest one and still does one SQLite write, enqueue only, exit — dispatch latency is bounded by a remote endpoint we don't control, so it never belongs here. |

`deck _hook` resolves the session by `conversation_id`, falling back to the tmux pane
identity in its environment. Unresolvable events are stored as orphans, never dropped
silently. Budget: read stdin, one `UPDATE`, one `INSERT`, dispatch or enqueue, exit.

The event set, the payload fields (`session_id`, `cwd`, `transcript_path`,
`permission_mode`), the enumerated notification types, the last-assistant-message field on
stop and the stop-failure event are all upstream contracts, not deck's. They are re-verified
by `@real-agents` (§13.5) rather than trusted indefinitely, and every one of them has a probe
fallback (§7) — so an upstream change degrades a row from live to sampled instead of
breaking it.

Adapter-specific event sources for Pi and Codex (both have plausible hooks — an extension
API and a notify command respectively) are deferred; until then they are honestly labelled
"sampled" in the UI.

### 8.2 Codex conversation-id discovery

Codex mints its own id, so it must be discovered after launch. R2 makes the naive rule
unsound: with two Codex sessions launched in the *same* directory seconds apart,
"a transcript created after launch whose cwd matches" matches both candidates for both
sessions — which is the banned "most recent" rule wearing a cwd filter. Therefore:

1. **Serialise — store-backed, not process-wide.** At most one Codex launch is in its
   discovery window at a time, enforced by a CAS discovery lease in `state.db` with the
   same shape as the §9.3 launch lease (owner `pid@boot_id`, TTL, stale-break on dead
   owner). A process-local mutex would be unsound under R4's own model: N concurrent TUIs
   are N processes, each holding its *own* mutex, and the banned ambiguity returns
   silently. A second Codex launch — from any client — queues behind the lease.
   Queue position is visible in the UI (`starting · awaiting id`).
2. **Claim.** Every transcript path already bound to a session is excluded from candidacy,
   and a discovered path is written to the row in the same transaction that clears the
   mutex, so no two sessions can ever hold one transcript.
3. **Ambiguity is an outcome, not a coin flip.** If the window closes with zero or more than
   one unclaimed candidate, the session stays live with `conversation_id = NULL`, is shown
   as `id unresolved`, and offers two explicit actions: pick from the candidate list
   (showing first lines and timestamps), or leave unresolved. An unresolved session is
   usable but not resumable, and says so.
4. **Never** fall back to "most recent" and never guess (R2).

If a future Codex accepts a caller-assigned id or name at launch, this entire subsection
collapses into the assigned-id path — see §14.2.

---

## 9. Lifecycle

### 9.1 Resume, on demand

There is no boot-time restore and no `autostart` (R3). After a reboot the list is intact,
every session reads `stopped · resumable`, and `r` brings one back:

- Create `deck_<slug>` at `cwd` on the deck socket; run `pre_launch` if set; launch the
  agent with its **resume** argv and the session's env/permission profile.
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
- Pinning, for forcing a specific conversation: pin sets `resume_state = pinned` and is
  sticky across restarts; a one-shot "start fresh" reverts to `auto` afterwards.
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
| archive | `A` | **confirms first** (§11.4), then keeps the record and hides the row from the default list. On a live session the same confirm covers the kill, and says so in as many words — the dialog names the session and states that a live agent will be killed. Nothing is written until it is confirmed. On success a toast says what happened, with `u` to undo. An archived row is **not startable** (§9.1). |
| unarchive | `U` | clear `archived_at`. The row returns to the default list reading `stopped · resumable`, and `r` works on it again. Reachable wherever an archived row is — inside the `/` filter's results — and it is what `deleted_at`'s restore already is: the reversal without which archiving is a one-way door. |
| bulk | `m` marks | `x` / `dd` act on the mark set. |

deck never writes to or deletes anything inside a session's `cwd`.

**What `dd` removes, exactly.** *Purge* was doing two unrelated jobs in that table, so they
are named apart: the conversation purge is the checkbox, and the tombstone is **reaped**.
Reaping deletes the `sessions` row and every deck-owned row hanging off it — events,
notification outbox entries, the `waiting` and `notify_epoch` state — together with deck's
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
exact command. deck never runs privileged or account-level commands itself.

---

## 10. Notifications

A pluggable, service-agnostic dispatch layer. deck knows nothing about any specific
notification service; integrating one is configuration, not code.

### 10.1 Channels

Declared in `config.toml`. Three types, all generic:

| type | behaviour |
|---|---|
| `webhook` | HTTP request to a user-supplied URL: configurable method (default `POST`), headers, and a body **template**. Timeout, TLS verification, and retry count are per-channel. This is how any hosted or self-hosted notifier is integrated — deck ships no service-specific client. |
| `command` | Execute a user-supplied argv with the rendered payload on stdin (and as env vars). Covers desktop notifiers, local scripts, anything on the box. |
| `desktop` | Convenience wrapper over `command` for a freedesktop notification. **Does not degrade silently:** unreachability is recorded as a channel error on the outbox row and surfaced in the health view. This matters because a `_hook` spawned from a tmux server that systemd started has no session bus, so the channel is unavailable in precisely the deployment §6.3 warns about — the health view therefore probes the bus alongside `PATH`. |

Body rendering is a text template over a documented, versioned payload:
`{session: {name, cwd, agent, status, reason, permission_profile, workspace, important},
event: {kind, at, message}, deck: {host, version}}`. Templates are user-authored, so any
JSON shape a target expects can be produced — including nesting the message inside a
service-specific envelope. Rendered bodies are size-capped and redacted per §6.4.

### 10.2 Rules

A rules table, global with per-session override (`notify_rules`), evaluated per event:

```toml
[notify]
quiet_hours = "23:30-07:30"        # local time; suppressed events are still logged
[[notify.rule]]
on       = ["waiting", "error"]    # any event kind from §4
channels = ["ops-webhook"]
[[notify.rule]]
on       = ["idle"]
only     = "important"             # milestone-style: only sessions flagged important
channels = ["ops-webhook", "desktop"]
```

- Every event kind in §4 (`started, prompt, waiting, idle, error, ended, resumed, killed,
  env, note`) is a valid `on` value — the rule grammar and the event vocabulary are one
  list, not two.
- Every rule is configurable; a per-session rule set **replaces** the global one entirely
  (no partial merge — merge semantics are a support burden and a debugging trap).
- **No debounce, deliberately.** "Suppress if it resolves within 20 s" needs something
  awake 20 s later to fire the survivors; with no daemon, the only thing that would ever
  wake up is the next hook — which, for a session blocked waiting on you, never comes. A
  debounced `waiting` would therefore be a notification that is *never* sent, in exactly
  the case the product exists to catch. So dispatch is immediate, and the cost is accepted:
  a prompt you answer in three seconds still pinged you.
- **Dedupe with an epoch, not forever.** The key is
  `session:kind:reason:notify_epoch`; `notify_epoch` increments whenever the session leaves
  an attention state (§7). A re-fired prompt within the same attention episode notifies
  once; the same prompt tomorrow is a new epoch and notifies again. A permanently unique
  key would silently mute a recurring prompt for the lifetime of the session.
- `snoozed_until` and quiet hours suppress dispatch but never suppress the event log.

### 10.3 Delivery

Dispatched inline by `deck _hook` (per-channel timeout, default 3 s), so notifications work
with no TUI open and no daemon. Session-end events only enqueue (§8.1). Failures land in
`outbox` and are retried by the next hook invocation or TUI tick.

The three limits this design accepts, all of which belong in the help view rather than in
a footnote:

1. **Retry needs a next event.** A delivery that fails at 02:00, with no TUI open and no
   further hook activity for that session, sits in the outbox until morning. There is no
   timer, because a timer is a daemon.
2. **Probe-classified agents notify only while a TUI runs.** Pi, Codex and shell sessions
   have no event source of their own (§8), so unattended they change status — and therefore
   notify — never. Claude sessions notify unattended, including turn and API failures via
   the stop-failure hook.
3. **Process death is detected late.** A `SIGKILL`ed or OOM-killed agent of any kind fires
   no hook, so its `error` notification waits for the next tick or hook (§7).

### 10.4 Out of scope

**Inbound remote control.** Replying into a session from a phone would require a
long-polling daemon and a service-specific protocol, contradicting both the no-daemon and
service-agnostic constraints. Notifications are one-way in v1. If it's ever wanted, the
natural shape is a separate program that writes to deck's store — not deck growing a
listener.

---

## 11. TUI

```
╭ deck ─── 2 waiting · 1 error · 7 sessions ─────┬ ◐ perf-sweep ── claude · safe ─────────╮
│  service-a              ~/work/service-a       │ ~/work/service-a · conv 4f9c…a21       │
│ ● api-refactor   claude  live    waiting 2m    │                                        │
│ ● flaky-tests    claude  live    waiting 6m    │ > run the benchmark suite              │
│ ◐ perf-sweep     claude  live    running 4s    │   ⠋ bench/throughput … 14/31           │
│ ○ dep-audit      codex   sampled idle   31m    │                                        │
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
this drawing's.

The shape is a **session sidebar beside a live preview**, not a full-width list. The
sidebar is the permanent spine of the product — it is what you scan to answer "which
session needs me" — and the preview is what makes an answer actionable without attaching.
Both are described below; §11.2 covers what happens when the terminal is too narrow to
hold them side by side.

- Grouping by `workspace` (default: basename of `cwd`), collapsible, and **optional**:
  `[ui] group_by_workspace` (default `true`, §6.5). Never by repo. With grouping off the
  sidebar renders one flat list in the sort order below with **no header rows**, which is a
  different row budget for §11.2's page-size and elision maths — the flat list is specified
  here rather than left to a job to invent. Collapse state is meaningless in flat mode and is
  absent rather than inert. Grouping is *preference*, not machine-local UI state: it is edited
  in settings (§11.5) with an explicit save, so §6.5's rule that a keypress never rewrites
  `config.toml` still holds, and it is not in `ui_state` alongside `layout_mode`.
- Sort: **configurable**, `[ui] sort_order` (§6.5), four values, `attention` the default:
  - `attention` (default) — `waiting` (oldest first) → `error` → `running` → `starting` →
    `idle` → `stopped`. This is the order every earlier phase shipped and the one the list
    exists to provide; it stays the default precisely because it is the order that answers
    "which session needs me".
  - `created` — newest first, by `sessions.created_at` descending.
  - `activity` — most recent first, by `sessions.status_at` descending. "Activity" is a
    **status change** (§7's own timestamp), not last pane output: deck records no
    per-session output clock, and inventing one is a schema change, not a sort option.
  - `name` — `name` ascending, case-insensitively, so `Api` and `api` sort together.

  Every order is total: each falls back to `id` ascending on a tie, so a re-sort can never
  swap two rows out from under an in-flight keyboard idiom (§11's marked-set navigation
  depends on this, and a coin-flip tie-break has already caused one defect — see the
  stable-sort rule below). A non-`attention` order does **not** re-rank by status at all:
  the whole point of choosing one is that the user, not deck, decides what "first" means, and
  a hidden status tier would make the chosen order a suggestion. Attention itself remains
  reachable in every order through §11's own attention-walk key and the collapsed strip's
  count, which are what make a non-attention order safe to offer rather than a way to lose a
  waiting prompt. Sort order is *preference*, not machine-local UI state: edited in settings
  (§11.5) with an explicit save, like `group_by_workspace` above, so §6.5's rule that a
  keypress never rewrites `config.toml` still holds, and it is not in `ui_state`.
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
- Live/sampled badge per row (§3), permission badge for non-`safe`, `env↻` when dirty.
- Status glyphs `●` waiting · `◐` running · `○` idle · `◌` starting · `■` stopped ·
  `✗` error · `▣` archived. One column, always in the same column, so the shape of the
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
  selected row only. Passive preview runs no PTY emulator — it is a capture, so it cannot be
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
  on and not every row passed on the way; it is **skipped below §11.9's 7-row inner floor**,
  leaving the pane cropped, because a box that small has no transcript in it worth reflowing
  for; and it is **best-effort, owning and restoring nothing** — a session the user looked at
  is left at the size deck last chose, and any attaching client re-expresses its own size
  under `window-size latest` and simply wins.
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
  UI: create modal (name, cwd picker, agent, permission profile, env, pre_launch, args),
  env editor, permission switcher, pin/unpin, rename, notification rules editor, health
  view (tmux version, socket, agents on PATH, PATH resolvability, optional unit install),
  event log, search, **a settings view over every config key (§11.5)**, and a help overlay
  with the full keymap. A capability that can only be reached by editing a file by hand is
  a defect against R7, not a documentation gap.

Keymap: `↵` attach · `space` next needing attention · `Y` acknowledge · `n` new · `r`
resume/start · `R` restart preserving conversation · `x` kill (undo toast) · `dd` delete ·
`s` send message (§11.1) · `i` session detail (§11.4 — **rename is an action inside it**,
not a top-level key) · `e` env editor · `P` permission profile · `p` pin conversation ·
`E` event log · `f` find (§12) · `/` filter list · `m` mark · `z` snooze · `A` archive
(confirms, §9.2) · `U` unarchive (§9.2) · `u` undo · `g`/`G` top/bottom · `,` settings
(§11.5) · `t` theme picker (§11.6) · `|` cycle layout mode, `<`/`>` sidebar width (§11.2) ·
`?` help · `q` quit.

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
  trailing SGR reset is a defect in the truncation, not a rendering trade-off. **Colour is not sufficient on its own.**
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
  archived, `U` on one that is not, and every per-row key when the list is empty; with a mark
  set in force the question is asked of the marked rows, since that is what the key would act
  on. This is the same argument one step finer: a key that is bound but refuses *here* is,
  from the user's side, indistinguishable from one that does not exist, and discovering the
  difference costs a keystroke and a failure message. Eligibility has exactly one definition
  per action, shared by the footer and the key handler — a footer deciding it separately is a
  footer that will drift out of agreement with the behaviour it advertises.
- **The footer's fixed set is curated for the keys worth a whole line of the frame.** It
  carries navigation, `↵`, `a`, `Y`, `n`, `x`, `r`, `R`, `dd`, the eligible one of `A`/`U`,
  `,`, `i`, `?` and `q`. Rarely-used per-row actions — the permission switcher `P`, pin `p` —
  stay bound, stay in the `?` overlay and in §11's keymap, and stay out of the footer: one
  line is a budget, and spending it on keys a user presses monthly crowds out `dd`, `U` and
  `,`. `U` in particular has to be there when it applies, because it is the reversal of an
  action that otherwise looks one-way, and `,` because settings has no other visible entry
  point in the default frame. Absence from the footer is never absence from the keymap, and
  §11's keymap plus the `?` overlay remain the complete list.
- **The footer also carries the selected row's status reason**, on its left, separated from
  the keys. §7's reasons are prose — `awaiting signal`, `resumable`, `pane failed after the
  stale frame` — and a sidebar with 31 content columns cannot hold a name and a reason on the
  same line without eliding one of them. The footer has the whole terminal width, and the
  reason is only ever wanted for the row the user is on, so this is where it costs nothing.
  The row itself carries **glyph, name, status word and badges, and no reason text**; the full
  reason is also in the `i` detail dialog, which is where it belongs when it is long.

### 11.4 Dialogs

Every dialog is a bordered, centred modal over a dimmed backdrop, and they all obey one
contract so learning any one of them teaches the rest:

- `esc` cancels and changes nothing. `↵` submits. `↑`/`↓` move between fields.
  `←`/`→`/`space` change a selection. A dialog may declare **additional load-bearing keys
  of its own**, but only if it states them inline where they apply — the `r` that reveals a
  masked secret (§6.4) is the canonical example: an explicit per-view toggle, named on screen
  at the place it acts. Nothing *undeclared* is load-bearing.
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
  currently available (an agent whose adapter is no longer registered falls back to the
  built-in default), and re-derives anything computed from it, since a permission profile is a
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
**rename** is reached · confirm (kill, delete, purge, archive) · delete options (tombstone
vs purge) · permission profile picker · pin conversation · send message (§11.1) · env editor
· snooze duration · notification rules · theme picker (§11.6) · event log · health view ·
find (§12) · help overlay. Settings is deliberately *not* a dialog — see below.

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
  this closes. **Structured tables are the stated exception**: `[notify]`'s channels and
  `[[notify.rule]]` arrays are edited in the notification rules dialog (§11.4), and settings
  shows them as a single navigable entry that opens it rather than flattening them into
  fields they don't fit.
- Field kinds are explicit: toggle, integer with bounds, string, path (with a picker),
  enum (cycled), list-of-strings, and *link* (opens the owning dialog, per the exception
  above). Each field states what it does and what changes when it changes.
- Navigation, spelled out because the takeover is not a §11.4 dialog and the main view has
  no `tab` binding for it to echo (§11.3): `tab`/`←`/`→` switch between the category list
  and the field list, `↑`/`↓` move within the focused list, `/` searches, `ctrl+s` saves,
  `esc` prompts to discard if anything changed and otherwise closes.
- **Save is explicit** (`ctrl+s` or the Save action), a discard prompt guards unsaved
  changes on `esc`, and the write is atomic — settings must never be able to leave an
  unparseable `config.toml` behind.
- **Scope is labelled per field**: global (`config.toml`), or per-session override where
  one exists (§6.1). A field that only takes effect on the next launch says
  *restart-to-apply*, consistent with §6.2 and `P` (§5). A setting that claims to have
  taken effect on a live pane when it has not is the same class of lie as a fabricated
  status.
- Settings edits configuration and nothing else: it cannot create, kill, resume or delete a
  session, and nothing in a session's lifecycle (§9) is reachable from it.

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
background        = "#0f172a"  # panel interiors
surface           = "#172033"  # elevated rows, footer, dialog interiors
border            = "#334155"
border_focus      = "#0d9488"  # the focused panel (§11.3)
selection         = "#26324b"  # selected row, focused panel
selection_idle    = "#37415c"  # selected row, unfocused panel
title             = "#fbbf24"
text              = "#cbd5e1"
dimmed            = "#64748b"  # starting rows, elided detail
hint              = "#94a3b8"  # footer descriptions
key               = "#d97706"  # footer/help keycaps
accent            = "#d97706"
group             = "#cbd5e1"  # workspace headers
search_match      = "#fbbf24"
badge             = "#94a3b8"  # live/sampled, env↻
badge_warn        = "#fbbf24"  # non-safe permission profiles, yolo
waiting           = "#fbbf24"  # the seven §7 statuses, one token each
running           = "#22c55e"
idle              = "#64748b"
starting          = "#a16207"
stopped           = "#64748b"
error             = "#ef4444"
archived          = "#475569"
```

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
  tokens must hold a WCAG contrast ratio ≥ 3:1 against `background`, and `text` against
  `selection`, computed over **both** the hex palette and its quantisation to the
  reference palette. This is a loader-level golden test, like §7's probe fixtures — the
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
session starts with one. Three mechanisms, all on the same text-input behaviour so the
create modal's `cwd`, and any later path field, behave identically:

**Recent working directories.** deck remembers the last **5** distinct directories a
session was created in, most-recent-first, in `state.db` (§4's `recent_cwds`) — not in
`config.toml`, since it is machine-local history rather than preference. The limit is
`[ui] recent_cwd_limit` (default 5, §6.5). Creating a session promotes its `cwd` to the
front, deduplicated by resolved absolute path, evicting the oldest beyond the limit.

- The `cwd` field is **pre-filled with the most recent entry**, so the common case —
  another session where you just were — is `n`, a name, `↵`. On a first run with no
  history it pre-fills the directory deck itself was started in. Typing replaces the
  pre-filled value wholesale (it is offered, not committed), and the field labels it as
  the last used so nothing is silently assumed on the user's behalf.
- `Ctrl+P`/`Ctrl+N` in the field cycle the recent list, shell-history style, showing
  `recent 2/5` so the user knows both where they are and that more exist. This is a declared
  per-field key set under §11.4's contract, and the field's own help line names it. They are
  readline's history bindings, chosen for the same reason `tab` completion is: the fingers
  already know them. `↑`/`↓` are **not** bound here — they move between fields in every dialog
  (§11.4), and a path field does not get to redefine the navigation keys of the dialog it sits
  in.
- Recency is ordered by a **monotonic sequence, not the wall clock**, so the order stays
  deterministic and assertable while `DECK_CLOCK` is frozen (§13.1): anything ordered by a
  frozen clock has no order at all.
- The list is history, and paths can themselves be sensitive: settings (§11.5) offers
  clearing it, and it is never included in notification payloads.

**Ghost completion.** With the cursor at the end of the field, deck shows the completion
inline in the theme's `dimmed` token, and `→` (or `end`) accepts it. Directories only —
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
| click a sidebar row | selects that row **and enters §11.9's interactive preview on it** | `↑`/`↓` then `↵` |
| click a workspace group header | toggle collapse | the grouping key (§11) |
| wheel over the sidebar | scroll the list, without selecting | `↑`/`↓`/`PgUp`/`PgDn` |
| drag the seam | adjust `sidebar_width` live | `<`/`>` |
| drag over the preview | select text; release copies it | `a`, then tmux's own copy-mode |
| click the collapsed strip | restore the previous non-collapsed mode | `|` |

**A click or a wheel over the passive preview does nothing**, and that is a binding too. The
passive preview is a non-interactive, non-scrolling crop (§11): there is no focus to take and
no viewport to move, and a gesture aimed at the preview must not fall through to the sidebar
instead. A mis-aimed click that quietly moved the selection would fire §7's status side
effects from what the user experienced as a click on some text. **A drag is the exception**,
because selecting text is reading rather than acting: it takes no focus, changes no status and
moves no selection in the list. **While §11.9's interactive mode is active the wheel scrolls
the grid's own scrollback**, which is the one viewport that does exist; a click over the
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
  Scrolling to look at something is not selecting it. A wheel that moved the selection would
  fire status-changing side effects (§7's attach-clears-`waiting` is one keystroke away)
  from an idle gesture. Over the preview it scrolls nothing, because there is nothing there
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
visible.** From the press until the release, the selected cells are marked with the
`selection` token — the same treatment a selected sidebar row carries (§11.3) — and the
marking clears when the release commits the copy. A selection the user cannot see is a
selection they cannot aim: the gesture is tmux's, and so is the feedback. **The marking is
linear, not rectangular**, because the copy is: it covers exactly the run `SelectedText`
would return for the same anchor and current cell, so what is highlighted and what is
copied can never disagree. The copy is written to a **tmux
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
Entering is an attachment in §7's sense: the same durable transaction as `a`'s attach answers
a `waiting` row and acknowledges an `error` row, applied only once entry has actually
succeeded — a refused entry claims nothing.

What separates this from §11's passive fit is **ownership**, not permission: passive fitting
picks a size and leaves it, while interactive mode records what it found, claims it, and puts
it back. The interaction model settles whether deck may resize a session at all, and `a` has
always resized the window; interactive mode changes what may be *typed* into a pane and what
must be restored afterwards.

- **Geometry is owned, claimed and restored.** Entering records the window's dimensions and
  its window-local `window-size` value, claims ownership in a pid-tagged window option, then
  resizes the **window** (never the pane — on a split window chrome is proportional and a
  pane-targeting loop cannot converge). Exiting resizes back *only when no client is
  attached*, then unsets the window-local `window-size`. The order is load-bearing and
  `set -g window-size latest` does not substitute for it: `resize-window` writes `manual`
  into the **window** options, which shadow the global. Cost is exactly two `SIGWINCH` per
  cycle.
- **Refused rather than degraded, in three cases**, each naming its reason and offering `a`:
  another client is attached to that session — one window has one size, and an attached client
  re-expresses its own under `window-size latest`, so the fit cannot be *held*, and unlike
  §11's best-effort passive fit interactive mode needs a held size for its grid to stay
  correct; the preview box has fewer than **7 inner rows**, which is deck's stacked height
  floor and leaves no transcript at all; or ownership is held by a live process.
- **The transport is `pipe-pane -IO` into a `charmbracelet/x/vt` grid**, seeded from
  `capture-pane -e -N` plus the pane state tmux exposes as formats, and **reseeded on every
  resize** — resizing the grid alone leaves it wrong for seconds. The pipe is armed before
  the seed is taken. `pipe-pane` is single-holder per pane, so a second reader displaces the
  first silently; a reader that sees EOF while `pane_pipe` is still 1 has been displaced,
  falls back to passive capture, and **says so**. A dead pane never closes the pipe at all,
  so `pane_dead` is polled rather than inferred from EOF.
- **Foreign bytes never reach the outer terminal.** Only composed cells are emitted. This is
  not an optimisation: pane bytes passed through leave the *outer* terminal on the alternate
  screen and reprogram its scrolling region. Passive preview gets this property free from
  `capture-pane`, which carries no such sequences; the moment bytes come from a pipe, the
  grid is the only safe consumer.
- **Input is dispatched on a verified identity**, re-resolved immediately before every send:
  socket path, server pid, `pane_id`, **`pane_pid`** and session name. `pane_id` alone is not
  sufficient — `respawn-pane` keeps it, and everything else tmux reports, unchanged.
- **The grid keeps its own bounded scrollback, and the wheel scrolls it.** This is the only
  way to scroll a full-screen agent: the alternate screen has no tmux history, which is why
  tmux's own wheel binding declines to enter copy-mode for it.
- **Modified navigation keys forward, like the unmodified ones, by tmux key name.**
  `Ctrl`, `Shift` and `Alt` combinations with the arrows, `Home`, `End` and the page keys are
  what word-wise movement and selection are built from in every agent's line editor, so a mode
  where they silently vanish is a mode the user has to leave to edit a line. Their encoding is
  as mode-dependent as a bare arrow's, so tmux names them for the same reason it names those.
  The forwardable set is **enumerated and tested key by key**, never left to a default branch:
  a key deck cannot encode must be a known, listed gap, because the failure it otherwise
  produces is a keystroke that does nothing and reports nothing.
- **Honesty about what the user cannot see.** When the target has not repainted since the
  resize, the panel says so; an empty frame otherwise reads as deck being broken rather than
  the agent being wedged. Help states that previewing and entering interactive mode both
  resize the agent's window, that output produced while narrow consumes scrollback faster, and
  that `[ui] preview_fit = false` turns the passive half of that off.

### 11.10 The list filter

`/` narrows the list as you type, incrementally, over what a row already shows — name, `cwd`,
workspace and status. It is a **view over the list and never a mutation**: nothing about a
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

---

## 12. Cross-session search

`f` opens a search view over three corpora, ranked and grouped by session:

1. session metadata (name, cwd, workspace, args),
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
| **State isolation** | `DECK_HOME` overrides the data/config/state root (XDG resolution applies only when unset). | Each scenario gets a pristine root; parallel scenarios never collide. |
| **tmux isolation** | `DECK_TMUX_SOCKET` overrides the socket name (default `deck`). | Scenarios run concurrently against private servers; teardown kills exactly one. |
| **Frozen / stepped clock** | `DECK_CLOCK=<rfc3339>` pins wall-clock now; `DECK_CLOCK_STEP` advances it on demand. **Wall clock only — durations, timeouts and budgets always use a monotonic clock and are never frozen**, or the §13.5 budget assertions would all measure zero. | Relative times ("2m", "31m") and quiet-hours windows become assertable without making elapsed time unmeasurable. |
| **Deterministic rendering** | `NO_COLOR`, `DECK_ASCII=1` (no nerd glyphs), `DECK_ANIM=0` (no spinner frames), fixed `COLUMNS`×`LINES`. | Screen text is byte-stable, so golden frames are meaningful. |
| **Explicit colour override** | `DECK_COLOR` forces colour on or off as a boolean, overriding both `NO_COLOR` and terminal detection. | `NO_COLOR` can only ever *disable*; a test that needs colour deliberately on — or a terminal deck mis-detects — has no other lever. |
| **Colour depth override** | `DECK_COLOR_DEPTH=truecolor\|16` forces the render path, overriding COLORTERM/TERM detection. | §11.6's quantised palette is behaviour, and a pty test cannot otherwise deterministically reach it — the harness terminal's advertised depth would decide which renderer runs. |
| **Mouse reporting override** | `DECK_MOUSE` forces mouse reporting on or off as a boolean, overriding `[ui] mouse` (§11.8). | Enabling reporting writes enable/disable sequences into the stream, so byte-exact frame assertions (§11.2's golden frame) need it off; mouse scenarios need it on regardless of the config file. |
| **Deterministic ids** | `DECK_ID_SEED` makes generated session/conversation UUIDs reproducible. | Assert exact resume arguments. |
| **Bounded ticks** | `DECK_RECONCILE_MS` (default 500) and `DECK_PREVIEW_MS` (default 250) — two rates, two knobs, matching §7 and §11. | Tests wait on state, not on wall clock; low values make scenarios fast. |
| **Interactive render rate** | `DECK_INTERACTIVE_MS` — §11.9's grid render-coalescing interval. A duration, like the two ticks above. | Render frequency, not parsing, dominates the transport's cost, so it is the one axis worth pinning in a scenario. |
| **Interactive transport** | `DECK_INTERACTIVE_TRANSPORT=pipe\|capture` pins §11.9's render path. | A *selector over two implementations of one contract*, not a behaviour switch: both paths must satisfy the same scenarios, so a scenario can exercise either deterministically. Stated explicitly because this section otherwise forbids knobs that change what the product does. |
| **Structured log** | JSONL to `$DECK_HOME/log/deck.jsonl`: every state transition, launch argv, hook receipt with duration, notification attempt with outcome. | The observability surface for things not visible on screen — argv, timings, retries. |
| **Launch audit** | Each launch appends the exact argv + resolved env keys (values redacted) to the log. | Proves "resume by id, never `--continue`" (R2) without reading agent internals. |

Nothing above changes behaviour; they narrow non-determinism. `DECK_*` variables are listed
in the help view.

### 13.2 Harness

```
 feature files (Gherkin)
        │  godog
 ┌──────▼───────────────────────────────────────────────────────────┐
 │ steps: keys in · screen out · files · webhooks · log             │
 └──┬──────────────┬───────────────┬──────────────┬────────────────┘
    │              │               │              │
 ┌──▼───────┐  ┌───▼──────────┐ ┌──▼───────────┐ ┌▼───────────────┐
 │ pty +    │  │ real tmux    │ │ fake agents  │ │ httptest       │
 │ VT100    │  │ private sock │ │ on PATH      │ │ webhook sink   │
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
- **tmux.** A real tmux on a per-scenario socket. Steps may assert tmux facts directly
  (`session exists`, `pane command is …`, `environment contains …`) — that's observable
  outside the app. Two of those facts carry §11's central preview guarantee and are
  therefore named here: **`list-clients` is empty** while a preview is live, and
  `#{window_width}x#{window_height}` is unchanged across any amount of previewing.
- **Fake agents.** `fake-claude`, `fake-pi`, `fake-codex` on `PATH`: tiny programs that
  honour the real argument contracts (`--session-id`, `--resume`, `--permission-mode`),
  write transcript files in the real on-disk layout, print recognisable pane text on
  demand, fire hook payloads at `deck _hook` on command, and can be told to hang, crash,
  or exit. They are the *contract* under test — real-agent conformance is a separate,
  tagged suite (§13.5). They also **record every terminal size they observe** — the initial
  one and each `SIGWINCH` — where a step can read it, which is what makes §11's fit and
  §11.9's restore assertions about the agent's own experience rather than inferences from
  tmux's bookkeeping. It is also what a scenario reads to prove the negatives: that a fit is
  coalesced to one `SIGWINCH` per settled selection rather than one per row walked, that no
  fit reaches a pane below the 7-row floor, and that `preview_fit = false` produces none at
  all.
- **Webhook sink.** An `httptest` server registered as a `webhook` channel; steps assert
  on requests received, bodies rendered, dedupe collapses, and non-delivery during quiet
  hours. Notification behaviour is fully black-box because §10 has no built-in service.
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
  crash.feature                 §7 — error + crash tail + notify, and never auto-relaunch
  environment.feature           §6 — layering, env↻, restart applies (and only restart)
  kill_delete_undo.feature      §9.2 — x/dd, undo windows, tombstone, cwd never touched,
                                reap leaves no trace, the agent's transcript survives `dd`
  shell_state.feature           §9.4 — history, scrollback replay, cwd restore, sensitive
  notifications.feature         §10 — rules, epoch dedupe, quiet hours, templates, retry
  codex_discovery.feature       §8.2 — serialised discovery, claims, ambiguity, unresolved
  layout_modes.feature          §11.2 — auto selection, | cycling, resize re-choice, floors
  preview.feature               §11 — coalesced fit, floor-skip, preview_fit=false is passive,
                                no attached client, no scroll, crop when unfitted, crash tail
                                for error, placeholder with no pane
  attention_sort.feature        §7/§11 — attention order, the collapsed strip's count,
                                workspace grouping and collapse, space walks what needs me
  mouse.feature                 §11.8 — click selects and enters interactive, wheel scrolls
                                without selecting, seam drag resizes, preview drag selects and
                                release copies, DECK_MOUSE=0 disables
  settings.feature              §11.5 — schema-generated fields, explicit save, atomicity
  themes.feature                §11.6 — picker, live preview/revert, fallback says so,
                                quantised rendering under DECK_COLOR_DEPTH=16
  search.feature                §12 — metadata/events/transcripts, resume from a hit
  health.feature                §9.5 — no tmux, old tmux, missing agent, PATH unresolvable
  real_agent_smoke.feature      @real-agents — the thin conformance subset (§13.5)
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
  And notification rules sending kind "waiting" to the sink
  When the agent fires notification type "permission_prompt"
  Then "api" shows "waiting" with reason "permission_prompt" within 1 reconcile
  And the webhook sink receives 1 request for kind "waiting"   # immediate, no debounce
  When the same notification type fires again
  Then the webhook sink has still received 1 request           # dedupe, same epoch
  When the agent fires "stop" with message "done"
  Then "api" shows "idle" and the detail pane contains "done"
  When the agent fires notification type "permission_prompt" again
  Then the webhook sink receives a 2nd request                 # new epoch after resolution
```

And three that pin the distinctions most easily got wrong — a clean exit read as a crash, a
crash read as nothing at all, and two conversations collapsed into one:

```gherkin
Scenario: a shell session that exits cleanly is stopped, not an error    # §7
  Given a session "notes" for agent "shell"
  When I type "exit" in its pane
  Then "notes" shows status "stopped"
  And no notification is sent
  And no crash tail is recorded

Scenario: a crashed agent is an error with its exit status               # §7
  Given a running session "api" for agent "claude"
  When its process is killed with SIGKILL
  Then within 1 reconcile "api" shows "error"
  And the crash tail contains the last lines that were on the pane
  And the launch count for "api" is still 1                   # never auto-relaunch

@codex
Scenario: two Codex sessions in one directory get distinct ids           # R2 + §8.2
  Given a working directory "~/work/svc"
  When I create Codex sessions "one" and "two" there within 2s
  Then discovery is serialised and each ends with a distinct conversation id
  And neither id is the other's transcript
  When discovery for a third session finds no unclaimed candidate
  Then it shows "id unresolved" and offers a candidate picker
  And it is not resumable while unresolved
```

### 13.5 Coverage beyond the headline scenarios

Also specified as features, one scenario per rule: resume-argv per adapter (including
Codex's serialised discovery and the ban on "most recent"); `_hook`'s store write inside its
budget, measured from monotonic log durations, with a separate scenario for the session-end
enqueue-only path; store migration from the previous schema version; scrollback replay
identical modulo the cap, absent entirely when `sensitive`, and off by default for agent
sessions; capture ownership (kill, session end, TUI shutdown, opportunistic) each covered;
env edit shows `env↻` and applies **only** after restart, with the conversation preserved;
`captured_path` precedence, and `login_shell` overriding it; notification templates render,
redact secret-shaped keys, respect a per-session rule set replacing the global one, and
record a channel error rather than silently no-op'ing when a channel is unreachable;
outbox retry on the next hook or tick; `s` refused from every status except `idle`;
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

**Fixture corpus.** Probe heuristics (§7) are driven by captured pane text per agent per
state, stored as fixtures and asserted through the UI badge, so a spinner or prompt
redesign upstream is a one-fixture fix.

---

## 14. Open questions

1. **Pi and Codex event sources.** Both plausibly support real event hooks (extension API;
   notify command). Each removes a probe path and upgrades a row from sampled to live.
   Worth a spike each: `Instrument` is part of the adapter interface by design (§8), so an
   event source is additive per adapter rather than a redesign — but the answer decides
   whether the probe corpus for those kinds is a permanent fixture or a stopgap.
2. **Codex conversation naming.** Its resume path accepts a name as well as an id; if a
   name can be assigned at launch, Codex joins the assigned-id group and `DiscoverID`
   disappears.
3. **Scrollback default.** 5 000 lines with escapes is generous and writes screen contents
   to disk. Smaller default, or `sensitive` inverted (opt-in capture)?
4. **Probe cadence when no TUI runs.** Non-hook agents go unclassified while unattended.
   Accept, or allow an opt-in periodic probe (which reintroduces something daemon-shaped)?
5. **Fork-on-resume.** Resuming a conversation that another client already resumed can, on
   some agents, produce divergent transcripts. Detect and offer fork, or refuse the second
   resume via the lease?
6. **Binary name** `deck` vs Kong's `decK` on `$PATH`.
7. **Immediate notification vs noise.** Dropping debounce (§10.2) means a prompt answered in
   three seconds still pinged you. Acceptable, or is a *resolution* notification ("no longer
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
