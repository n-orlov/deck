# deck — product spec

A terminal session manager for CLI coding agents. Named sessions with durable
conversations, an attention-sorted list, and N concurrent clients — on one host, with
tmux as the only runtime dependency.

Interaction model: **`deck` is a TUI. Every user action happens in the UI.** There is no
user-facing command line.

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
- TUI: `charmbracelet/bubbletea` + `lipgloss` + `bubbles` (latest stable, pinned).
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
  `set -g window-size latest` (§3.3), and `setw -g aggressive-resize on`.
  `detach-on-destroy` is deliberately left at its default (`on`) so that killing a session
  another client is viewing returns that client to its TUI rather than silently hopping it
  into an unrelated session.
- Session naming: `deck_<slug>`, slug `[a-z0-9_-]+` derived from the name, uniqueness
  enforced in SQLite. Rename renames both. **`.` and `:` are excluded from slugs** —
  tmux rejects them in session names because they are target-syntax separators.
- One window, one pane per session (R6/non-goals). No drawers, no extra windows.
- Env is applied by launching the pane's command as `env K=V … <argv>` — portable across
  tmux versions — and mirrored with `set-environment -t` so any future pane agrees.

### 3.3 Attach

`Enter` attaches the client directly: `tmux -L deck attach -t deck_<slug>`. On detach the
client returns to the TUI, which resumes its render loop.

**Geometry, stated honestly:** two clients attached to the same session at different
terminal sizes *share* one view. Grouped sessions do not fix this — a group shares its
windows, and deck sessions have exactly one window (§3.2), so every client necessarily
displays the same window. `window-size latest` makes the most recently active client
govern the size, and `aggressive-resize` keeps the window matched to it; an idle client on
a smaller terminal will therefore see truncation until it becomes the active one. This is
the same behaviour every tmux-based manager has, it is not worth a redesign, and it must
be documented in the help view rather than promised away.

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
  launch_lease_owner TEXT,                  -- pid@boot_id holding a start (§9.3)
  launch_lease_until INTEGER NOT NULL DEFAULT 0,
  created_at         INTEGER NOT NULL,
  last_attached_at   INTEGER NOT NULL DEFAULT 0,
  archived_at        INTEGER NOT NULL DEFAULT 0,
  deleted_at         INTEGER NOT NULL DEFAULT 0  -- tombstone; purged after grace (§9.2)
);

CREATE TABLE events (               -- append-only: audit trail, search corpus, notify source
  seq        INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT REFERENCES sessions(id) ON DELETE CASCADE,
  at         INTEGER NOT NULL,
  kind       TEXT NOT NULL,         -- started|prompt|waiting|idle|error|ended|resumed|killed|env|note
  reason     TEXT,
  payload    TEXT                   -- bounded JSON
);

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
  whatever `status` it had. Archiving requires `stopped` (the UI offers "kill and archive"
  as one action) so an archived row can never hide a live agent.
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
- Creating or switching a session to `yolo` requires an explicit confirm in the create
  modal, gated behind `allow_yolo = true` in config (default false).
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
| top level | `allow_yolo` (default false, §5), `stale_after` (default 45 s, §7), `capture_min_interval` (§9.4) |
| `[env]` | the middle PATH/env layer (§6.1) |
| `[ui]` | `theme` (§11.6), `ascii` (§11), `mouse` (default true, §11.8), `recent_cwd_limit` (default 5, §11.7). **Not** `layout_mode`, `sidebar_width` or the recent-directory list itself — those are machine-local UI state/history and live in `state.db` (§11.2, §11.7), so a keypress never rewrites this file |
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
on purpose — it is a flag (§4), orthogonal to status, settable only on a `stopped` session.

| status | meaning | UI |
|---|---|---|
| `starting` | tmux session created, no agent signal yet | dim |
| `running` | agent is working | normal |
| `waiting` | **needs me** — permission prompt, question, idle prompt | bright, in the attention count, sorted first |
| `idle` | turn finished cleanly | check, `last_message` in the detail pane |
| `error` | turn or process died | red, unseen marker sticks, crash tail captured |
| `stopped` | record alive, no tmux session (post-reboot, killed) | grey, labelled *resumable* |
| `archived` | hidden from the default list, retained | hidden behind a filter |

Rules:
- **Precedence:** `user-terminal` > `hook` > `probe` > `tmux`. A user kill sets
  `killed_by_user`, which an in-flight hook arriving milliseconds later cannot undo —
  explicit human action outranks automation. Below that, a probe never overwrites a fresher
  hook verdict, and `tmux` only ever supplies liveness.
- `waiting` and `error` set `acknowledged = 0`; cleared by attaching or by `Y`. Leaving an
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
  | session present, pane alive | keep the current status |

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
- **Resume clears `killed_by_user`.** The flag exists so an in-flight hook cannot undo an
  explicit kill (§7); once the user explicitly resumes, that verdict is spent — if it
  survived, every future hook for the session would be outranked forever and the row could
  never leave `stopped` by automation again.
- Pinning, for forcing a specific conversation: pin sets `resume_state = pinned` and is
  sticky across restarts; a one-shot "start fresh" reverts to `auto` afterwards.
- `shell` sessions "resume" by recreating the shell with their history file, replayed
  scrollback, and last known working directory (§9.4). A shell session never re-runs a
  previous command on resume.

### 9.2 Kill and delete

Teardown is cheap because R1 means no session owns anything on disk. That earns a
no-confirm UI:

| action | key | effect |
|---|---|---|
| kill | `x` | `tmux kill-session` immediately. Row → `stopped`. Conversation untouched, resumable. 10 s undo toast (`u` = resume). |
| delete | `dd` | kill + tombstone the row (`deleted_at`). Hidden immediately, undoable for 60 s, purged after. |
| purge conversation | in the delete confirm only | additionally deletes the agent's transcript. Never implicit, never default, always a separate explicit choice. |
| archive | `A` | keep the record, hide from the default list. |
| bulk | `m` marks | `x` / `dd` act on the mark set. |

deck never writes to or deletes anything inside a session's `cwd`.

### 9.3 Launch leases (R4)

Two TUIs pressing `r` on the same `stopped` session must not double-launch. The
transaction that flips `stopped → starting` also CAS-acquires
`launch_lease_owner`/`launch_lease_until` (owner = `pid@boot_id`, TTL ~30 s). A stale lease
(dead pid or expired TTL) is breakable.

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

- Grouping by `workspace` (default: basename of `cwd`), collapsible. Never by repo.
- Sort: `waiting` (oldest first) → `error` → `running` → `starting` → `idle` → `stopped`.
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
- Preview: pane capture with escapes preserved, 1 s tick, selected row only. No embedded
  PTY emulator in v1 — the preview is a capture, so it is never interactive. `↵` is how
  you get a real terminal.
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
`E` event log · `f` find (§12) · `/` filter list · `m` mark · `z` snooze · `A` archive ·
`u` undo · `g`/`G` top/bottom · `,` settings (§11.5) · `t` theme picker (§11.6) · `|` cycle
layout mode, `<`/`>` sidebar width (§11.2) · `tab` move focus sidebar↔preview · `?` help ·
`q` quit.

**Every capability in this section has a key or a documented entry point here, and every
key here has a scenario (§13.5).** A capability listed above with nowhere to reach it is an
R7 defect, and a key listed here with no binding is a §11.3 defect — the two lists are
checked against each other, not maintained independently.

The mouse is a **shortcut over that keymap and never an alternative to it** (§11.8): click a
row to switch to it, double-click to attach, wheel to scroll, drag the seam to resize the
sidebar. Every mouse action names the key it duplicates, so the keymap above stays the
complete description of what deck can do.

Constraints: **80×24 minimum**, resize-safe at every size above it, and the degradation
path is specified rather than emergent (§11.2). No colour assumptions beyond 16 colours:
a theme's truecolour values are used when the terminal advertises truecolour and are
otherwise quantised to the 16-colour ANSI set, so **every theme remains legible on a
16-colour terminal** (§11.6). Usable without a nerd font — the glyphs above are all
BMP box-drawing/geometric characters, and an ASCII fallback exists for every one of them
(`DECK_ASCII`, §13.1).

### 11.1 Sending text without attaching

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
- **Preview floor 40** is the narrowest a pane capture renders without wrapping into
  unreadable hash-soup. Below it the preview is worse than absent, so it is not shown.
- **Sidebar floor 24** is `glyph + name + status` with nothing elided; a narrower sidebar
  cannot answer the one question it exists to answer.
- **Stacked list 5–12 rows**: 5 is selection plus one neighbour plus the spinner row, so
  the list still conveys movement; 12 keeps a tall terminal from starving the preview.
- **Collapsed strip 3 columns** is the `»` glyph plus its two borders. It exists to give
  the preview the maximum possible width while keeping the attention count on screen —
  one glance still answers "does anything need me", and `tab`/`|` restore the sidebar.

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
- **Focus is visible.** `tab` moves focus between sidebar and preview; the focused panel's
  border uses the theme's `border_focus` token. Focus changes what `↑`/`↓`/`PgUp`/`PgDn`
  scroll — the list, or the preview's capture history — and the footer's hints change with
  it. A keyboard-only UI that cannot show where the keys are going is unusable.
- **The footer is one line, outside both panels**, in the key/description pattern
  (`↵ attach · n new · …`). It is contextual: it lists what is bound *now*, in this mode,
  with this focus. **It never lists a key that is not bound** — a footer advertising a verb
  the binary does not have is worse than no footer, because it is the one place a user is
  entitled to trust.

### 11.4 Dialogs

Every dialog is a bordered, centred modal over a dimmed backdrop, and they all obey one
contract so learning any one of them teaches the rest:

- `esc` cancels and changes nothing. `↵` submits. `tab`/`shift+tab` move between fields.
  `←`/`→`/`space` change a selection. A dialog may declare **additional load-bearing keys
  of its own**, but only if it states them inline where they apply — §5's mandatory `y`
  yolo confirm ("press y to confirm yolo before creating") is the canonical example, and
  it is required by §5, not an exception to be normalised away. Nothing *undeclared* is
  load-bearing.
- **Validation is in-dialog and specific**, and it retains what the user typed. A dialog
  never closes to reveal an error somewhere else.
- **Destructive actions confirm**, and the confirmation names the target and what will
  survive it (`kill notes — the session's history and conversation id are kept`).
- **The mouse can neither cancel nor confirm.** A click outside a dialog does nothing, and
  no dialog action is reachable by mouse alone (§11.8).
- Width targets 80% of the viewport, clamped to `[26, 80]` columns. At every supported
  width (§11: minimum 80) that resolves to 64–80 columns; the lower clamp and the
  take-the-full-viewport rule at or below 26 columns apply only on a below-minimum
  terminal, as best-effort behaviour to preserve the input area, and are not a supported
  size with test obligations.

The inventory, all reachable from the list. **A dialog exists only once the behaviour behind
it does**: §11.3's "never list a key that is not bound" applies here too, so a dialog for
unbuilt behaviour is simply absent rather than a stub that opens onto nothing
(`docs/PLAN.md` is where each one is assigned to a phase). Create session · session detail
`i` — which is where §5's degradation reason and §7's `last_message` live, and from which
**rename** is reached · confirm (kill, delete, purge) · delete options (tombstone vs purge) ·
permission profile picker · pin conversation · send message (§11.1) · env editor · snooze
duration · notification rules · theme picker (§11.6) · event log · health view · find
(§12) · help overlay. Settings is deliberately *not* a dialog — see below.

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
- Navigation, since the takeover is not a §11.4 dialog and the global `tab` (panel focus)
  does not apply inside it: `tab`/`←`/`→` switch between the category list and the field
  list, `↑`/`↓` move within the focused list, `/` searches, `ctrl+s` saves, `esc` prompts
  to discard if anything changed and otherwise closes.
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
- `↑`/`↓` in the field cycle the recent list, shell-history style, showing `recent 2/5`
  so the user knows both where they are and that more exist. This is a declared
  per-field key set under §11.4's contract.
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
| click a sidebar row | selects that row and focuses the sidebar; the preview follows on its next tick | `↑`/`↓` |
| **double**-click a sidebar row | attach | `↵` |
| click a workspace group header | toggle collapse | the grouping key (§11) |
| click inside the preview | focus the preview | `tab` |
| wheel over a panel | scroll that panel's viewport — the list, or the preview's capture history | `↑`/`↓`/`PgUp`/`PgDn` on the focused panel |
| drag the seam | adjust `sidebar_width` live | `<`/`>` |
| click the collapsed strip | restore the previous non-collapsed mode | `|` |

Four decisions in that table are load-bearing, and each is the safer of two options rather
than the obvious one:

- **A single click never attaches.** Attaching hands the whole terminal to another program,
  and a stray or mis-aimed click must not be able to do that. A double-click is the
  deliberate second act that `↵` already is. This is also what makes the single click a
  *switch* rather than a commitment: one click moves the preview to that session, which is
  the fast path the sidebar exists to provide.
- **The wheel scrolls the panel under the pointer, not the focused one, and changes neither
  focus nor selection.** Scrolling to look at something is not selecting it. A wheel that
  moved the selection would fire status-changing side effects (§7's attach-clears-`waiting`
  is one keystroke away) from an idle gesture.
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

**What it costs, stated plainly.** Enabling mouse reporting takes the terminal's own
selection behaviour over: click-drag no longer selects text for copy, and the user needs
their terminal's override modifier (usually `shift`) to select and paste. That is a real
loss for a tool people read output in, so mouse reporting is **opt-outable** — `[ui] mouse`
(default true, §6.5) and `DECK_MOUSE` (§13.1) — and the `shift` caveat is documented in the
help view rather than left to be discovered.

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
declared); no drag-to-reorder (§11's sort order is defined by status and age, not arranged by
hand — a hand-arranged list would stop answering "which session needs me"); no clickable
footer (it is a hint line, not a toolbar, and §11.3 already binds it to what is bound *now*).

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
| **Bounded ticks** | `DECK_RECONCILE_MS` (default 500) and `DECK_PREVIEW_MS` (default 1000) — two rates, two knobs, matching §7 and §11. | Tests wait on state, not on wall clock; low values make scenarios fast. |
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
  outside the app.
- **Fake agents.** `fake-claude`, `fake-pi`, `fake-codex` on `PATH`: tiny programs that
  honour the real argument contracts (`--session-id`, `--resume`, `--permission-mode`),
  write transcript files in the real on-disk layout, print recognisable pane text on
  demand, fire hook payloads at `deck _hook` on command, and can be told to hang, crash,
  or exit. They are the *contract* under test — real-agent conformance is a separate,
  tagged suite (§13.5).
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
  permission_modes.feature      §5 — profile → argv mapping, badge, yolo gate, degradation
  status_claude_hooks.feature   R6 — waiting/running/idle/error via hook payloads, live badge
  status_probe.feature          R6 — sampled badge, staleness, precedence over probe
  crash.feature                 §7 — error + crash tail + notify, and never auto-relaunch
  environment.feature           §6 — layering, env↻, restart applies (and only restart)
  kill_delete_undo.feature      §9.2 — x/dd, undo windows, tombstone, cwd never touched
  shell_state.feature           §9.4 — history, scrollback replay, cwd restore, sensitive
  notifications.feature         §10 — rules, epoch dedupe, quiet hours, templates, retry
  codex_discovery.feature       §8.2 — serialised discovery, claims, ambiguity, unresolved
  layout_modes.feature          §11.2 — auto selection, | cycling, resize re-choice, floors
  attention_sort.feature        §7/§11 — attention order, the collapsed strip's count,
                                workspace grouping and collapse, space walks what needs me
  mouse.feature                 §11.8 — click selects, double-click attaches, wheel scrolls
                                without selecting, seam drag resizes, DECK_MOUSE=0 disables
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
archiving refused for a live session; a user kill not resurrected by a late-arriving hook;
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
8. **Shared attach geometry** (§3.3). Living with `window-size latest` is the plan. If
   two-terminals-at-once turns out to be a daily annoyance rather than a rare one, the only
   real fix is one tmux session per client per agent, which is a different architecture.
