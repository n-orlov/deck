# deck — delivery plan

`SPEC.md` is the product spec and the source of truth. This file is the delivery plan: how
that spec gets built, in what order, and why that order. `docs/DELIVERY-LOG.md` records what
actually happened.

## How work is delivered

Each phase is **one PRD**, run as **one autonomous job** (ralphd → `pi`), against this
repository as the mounted workspace. A phase is done when its Gherkin features pass and its
evidence report is written; the reviewing pass re-checks every numbered requirement
independently, so requirements are written atomic and individually verifiable.

Five standing rules for every phase:

- **Jobs commit and push their own work, one commit per completed task.** The commit log is
  the job's durable memory alongside its handoff notes: a later iteration or phase should be
  able to read `git log` and learn what was done and why. Messages say *why*, not what the
  diff already shows. Never force-push, amend or rewrite published history — fix forward. Git
  credentials are mounted into the job (`--creds`), so no token is ever handled in a prompt,
  a file or a commit message. From Phase 1 onward.
- **Review blocks on real defects and tolerates documentation nits.** Dead code satisfying a
  requirement on paper, a vacuous test, a flaky scenario, a false claim about behaviour, or
  anything touching a session's `cwd` must block. Wording, formatting and stale sentences in
  derived summaries are recorded as notes and do not fail a phase. The test is whether a
  reader would be *misled about behaviour*. Phase 0b was rejected three times over three
  sentences with a finished product in the tree; that is a bug in the process, not diligence.
- **A suite is "green" only when it is green ten consecutive times from a clean state.**
  Phase 0's "passes twice" bar was satisfied while a scenario failed one run in three, and
  the reviewer's two passing runs concluded it was stable. A flaky harness is worse than a
  missing one, because it teaches everyone to re-run instead of to trust. Equally, a green
  suite is not evidence a requirement is met: Phase 0 shipped a reconcile loop with no
  production caller and a determinism feature that passed vacuously against dead code.
- **`SPEC.md` is read-only to jobs.** A job that finds a contradiction records it in
  `docs/reports/phase<N>-findings.md`. Otherwise a job that hits a spec problem quietly
  edits the spec to match its own code, and the source of truth rots.
- **All build and test work happens in a sibling container** (`ci/Dockerfile` +
  `ci/run.sh`) carrying Go and tmux. The job container has neither and cannot install them.
  Proven end to end — see `ci/SPIKE.md`.

## Phases

| Phase | Scope | Green when | Status |
|---|---|---|---|
| **Toolchain spike** | Prove all deck work can run in a sibling Go+tmux container with the host workspace mounted: Go build/test, real tmux on a private socket, pty-driven bubbletea, no root-owned litter, warm cache | `ci/Dockerfile`, `ci/run.sh`, `ci/SPIKE.md` | **done** |
| **Phase 0 — harness & walking skeleton** | Go module; TUI binary rendering an empty list; determinism controls; JSONL log with launch audit; store schema v1 + migrations; tmux layer on a private socket; `shell` sessions create/list/attach/detach/kill; **godog harness driving the real binary through a pty**; fake-agent fixture mechanism | `walking_skeleton`, `determinism`, `store`, `tmux_contract`, `concurrency` (`@multiclient`) features; suite passes ten times from a clean state | **done** (+ Phase 0b hardening) |
| **Phase 1 — durable identity & agents** | Adapter registry with declared capabilities; Claude adapter with deck-assigned `--session-id`; resume argv, never `--continue`; Pi adapter; pin/cleared; resume-failure handling; permission profiles (§5) incl. the `yolo` gate; the create-modal fields an agent launch needs; launch leases | **T1**, `durable_identity`, `same_directory`, `permission_modes` | **next** |
| **Phase 2 — status truth** | `deck _hook`; per-session settings injection; hook→status mapping incl. stop-failure and the session-end budget; probe engine + fixture corpus; live/sampled badges; precedence incl. `killed_by_user`; clean-vs-crash exit split via `remain-on-exit failed` and the crash tail | **T3**, `status_claude_hooks`, `status_probe`, `crash` | planned |
| **Phase 3 — sessions & lifecycle** | Create modal completed; env editor showing winning layer → `env↻` → restart-to-apply (+ shell "inject instead"); kill/undo, `dd` tombstone, purge, archive, bulk marks | `create_session`, `kill_delete_undo`, `environment` | planned |
| **Phase 4 — Codex adapter** | Serialised claim-based id discovery; `id unresolved` state and picker; never "most recent" | `codex_discovery` | planned |
| **Phase 5 — notifications** | Channel abstraction (webhook / command / desktop); rules table; epoch dedupe; quiet hours; outbox + retry; redaction | `notifications` against an httptest sink | planned |
| **Phase 6 — shell state** | Per-session history file; scrollback capture ownership + replay; cwd tracking; `sensitive` | `shell_state` | planned |
| **Phase 7 — TUI completeness** | Attention sort, grouping, preview pane, send-without-attach (§11.1), cross-session search, health view | `search`, `health`, a scenario per keybinding | planned |

**T2 (concurrency) is not a phase.** It lands in Phase 0's harness as `@multiclient` and is
re-run from then on — cheaper to keep green continuously than to retrofit.

## Why this order

1. **Harness first.** Nothing later is verifiable without it, and the phase that builds the
   thing that proves correctness is the one place where cutting corners costs the most.
2. **Durable identity immediately after (Phase 1).** It is the product's entire reason to
   exist. If conversation-preserving resume across a reboot cannot be made to work, that is
   worth discovering before any polish is built on top of it. It is also the point at which
   the operator can first run deck against a real agent, so it is where the plan stops being
   a plan and starts being a product. This phase was originally sequenced second, behind
   lifecycle polish; it was moved ahead because "can I actually use this with claude" is both
   the biggest risk and the earliest useful milestone.
3. **Status before lifecycle polish.** A session list that cannot say *which session needs
   me* is the reason to run a manager at all. Until hooks land, an agent row honestly reads
   `starting · awaiting signal` (§7) — usable, but not yet a replacement for what it replaces.
   Undo toasts and archiving are worth less than legible status.
4. **Status before notifications.** Notifications have nothing truthful to fire on until
   status detection is real.
5. **Polish last.** Preview panes, search and the health view are the cheapest things to
   defer and the least likely to invalidate anything else.

**Permission modes (`SPEC.md` §5) were unassigned to any phase** until this revision — a
planning defect, since launching Claude in a skip-permissions mode was one of the first
requirements stated. They belong with the Claude adapter, because a profile is only ever
realised as that adapter's launch flags, so they land in Phase 1.

## Deliberately not in any phase

Everything in `SPEC.md`'s non-goals, and in particular: git worktrees, a user-facing CLI,
multi-host or remote sessions, a web UI, sandboxing, inbound remote control, auto-restart on
crash, and idle reaping.
