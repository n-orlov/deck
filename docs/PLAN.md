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
| **Phase 1 — durable identity & agents** | Adapter registry with declared capabilities; Claude adapter with deck-assigned `--session-id`; resume argv, never `--continue`; Pi adapter; pin/cleared; resume-failure handling; permission profiles (§5) incl. the `yolo` gate; the create-modal fields an agent launch needs; launch leases | **T1**, `durable_identity`, `same_directory`, `permission_modes` | **done** (operator-verified against a real claude; see `docs/DELIVERY-LOG.md`) |
| **Phase 2 — status truth** | `deck _hook`, including the bounded liveness pass that makes §3's "lazily by `_hook`" and §7's "next `_hook` invocation" real — the only path by which a crash is noticed with no TUI open (§10.3's third limitation); per-session settings injection; hook→status mapping incl. stop-failure and the session-end budget; probe engine + fixture corpus; live/sampled badges; precedence incl. `killed_by_user`; clean-vs-crash exit split via `remain-on-exit failed` and the crash tail (the reconcile gains `list-panes -F` with `pane_dead`/`pane_dead_status` — today it only lists *sessions*, so a retained dead pane leaves the row reading `starting` forever); shell liveness promotion (§7 — **which deliberately changes pinned Phase 1 assertions**: `lease_race.feature`'s `starting - awaiting signal` and no-screen-contains-`running` checks and `launch_lease.feature`'s three `starting - awaiting signal` rows all assert shell pre-promotion behaviour, and the PRD must enumerate them as required updates so a red run is not "fixed" on the wrong side; the `starting · awaiting signal` copy in `tui.go` and `helpView` is likewise agent-only after this); `waiting` probe-eligibility + clear-on-attach (§7); `Y` acknowledge; resume clears `killed_by_user` (§9.1); and separating "starting elsewhere" from "this row is not leasable" in the resume path. Dead-pane retention was the one open question this phase could not route around; it is now settled in §7 — **collect on sight**, capture the tail then kill the session, idempotent and unleased. Harness prerequisites first, all specified in §13.2/§13.1 but unbuilt by Phase 0: fake agents that **fire §8.1 events at `deck _hook` from inside their own pane** and **render a named fixture verbatim**, `DECK_CLOCK_STEP` advancing **on demand** rather than only per shell creation, and a SIGKILL step that kills the **agent process** rather than a deck client. Budget ~250 iterations / 12 approaches up front: this phase is larger than all of Phase 1 | **T3's status half** (its webhook assertions need Phase 5 — see below), `status_claude_hooks`, `status_probe`, `crash` | **done** — 45 requirements, 7/7 tasks on approach 5, iteration 261/330. Independently re-verified: 48/48 scenarios, `ci/stability.sh 10` 10/10 at `a0d7db3` (which contains the last production/test commit), no protected file touched. Operator walkthrough outstanding |
| **Preview spike — in-screen tmux embedding** | Decide whether §11's preview stays a 1 s `capture-pane -e` poll or becomes a live embedded view: does a second (read-only) client at preview size perturb the real pane under the `window-size latest` / `aggressive-resize on` options `internal/tmux` already sets, does foreign output compose inside §11.3's chrome through a cell-grid emulator, what does it cost continuously, and can §11.2's golden 80×24 frame survive it. Gates Phase 2b's preview architecture, so it runs **before** that PRD is cut. Throwaway: no product code, deliverable is a report proposing spec changes rather than applying them | `prds/spike-tmux-embedded-preview.md` → `docs/spikes/tmux-embedded-preview.md` | running (`deck-spike-preview`) |
| **Phase 2b — UX shell** | The §11 layout: session sidebar beside the live preview, layout modes/persistence/width keys (§11.2), rounded chrome/padding/single seam/visible focus (§11.3), the dialog contract (§11.4) **retrofitted onto the dialogs that already exist** (create, detail `i`, profile picker, pin, kill confirm, help) — dialogs for unbuilt features are *not* stubbed, per §11.3's footer rule — the settings takeover over the flat config schema (§11.5, §6.5; structured `[notify]` tables stay with Phase 5's rules dialog), and the theme system incl. picker and quantised floor (§11.6). Harness prerequisites first, three of them and none fakeable by the scenarios that need them: pty resize + SGR-attribute assertions and **SGR (1006) mouse-report synthesis** (§13.2), plus `DECK_COLOR_DEPTH` and `DECK_MOUSE` (§13.1). Note `internal/config/toml.go` is **replaced** by the schema-driven parser, not extended. Also **§11.8 mouse navigation** — click to switch, double-click to attach, wheel-scroll without selecting, seam drag, and the `[ui] mouse` / `DECK_MOUSE` opt-out — which lands here rather than later because it is hit-testing against this phase's own layout, and retrofitting it onto a finished view layer means writing the geometry twice. Budget ~250 iterations / 12 approaches up front | `layout_modes`, `settings`, `themes`, `mouse`, plus the §11.2 golden frame (side-by-side, 35/45) at exactly 80×24 | planned |
| **Phase 3 — sessions & lifecycle** | Create modal completed, incl. **§11.7 path entry**: the `recent_cwds` table, last-used prefill, `↑`/`↓` cycling, ghost completion accepted with `→`, and `tab` on bash's contract (deck deliberately ghosts nothing when matches are ambiguous — see §11.7); env editor showing winning layer → `env↻` → restart-to-apply (+ shell "inject instead"); kill/undo, `dd` tombstone, purge, archive, bulk marks; **rename** (an action inside the `i` detail dialog per §11.4, not a top-level key) **and the event log `E`** | `create_session`, `kill_delete_undo`, `environment` | planned |
| **Phase 4 — Codex adapter** | Store-backed claim-based id discovery (§8.2 — a CAS lease like §9.3, *not* a process mutex); `id unresolved` state and picker; never "most recent". **Spike §14.2 (can Codex take an assigned name at launch?) before cutting this PRD — if yes, discovery disappears entirely** | `codex_discovery` | planned |
| **Phase 5 — notifications** | Channel abstraction (webhook / command / desktop); rules table; epoch dedupe over the `notify_epoch` Phase 2 maintains; quiet hours; outbox + retry; redaction. **Flips one Phase 2 assertion deliberately:** with no `outbox` table at schema v1, Phase 2 pins "the session-end path enqueues nothing"; §8.1's contract is enqueue-only, so this phase makes it enqueue and re-aims that assertion | `notifications` against an httptest sink, **and T3 closed** | planned |
| **Phase 6 — shell state** | Per-session history file; scrollback capture ownership + replay; cwd tracking; `sensitive` | `shell_state` | planned |
| **Phase 7 — TUI completeness** | Attention sort, grouping, send-without-attach (§11.1), cross-session search, health view (**preview pane and layout moved to Phase 2b**) | `search`, `health`, a scenario per keybinding | planned |

**T2 (concurrency) is not a phase.** It lands in Phase 0's harness as `@multiclient` and is
re-run from then on — cheaper to keep green continuously than to retrofit.

**T3 spans two phases, deliberately.** `SPEC.md` §13.4's third headline scenario asserts both
halves of one story: that `waiting` is *truthful* (status, reason, resolution, clearing) and
that it is *deduped per episode* at the webhook sink. Status is Phase 2; channels, rules and
the outbox are Phase 5, because a notification has nothing truthful to fire on before status
detection is real. So Phase 2 owns T3's status assertions **and `notify_epoch`** — the counter
§7 bumps when a session leaves an attention state, which is what makes Phase 5's dedupe key
possible — and Phase 5 lands the sink assertions and closes the scenario. Neither phase is
allowed to claim T3 whole: Phase 2 writes the scenario with its notification steps absent
rather than stubbed (§11.3's footer rule, applied to tests), and Phase 5 adds them.

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
5. **The UX shell after status, not before it (Phase 2b).** The sidebar's payload is the
   attention sort and the preview, and both are worth little while every agent row honestly
   reads `starting · awaiting signal` — an attention-sorted list would have nothing to
   sort, and a preview would be the only place status was visible at all. Building the
   chrome first would also mean building it twice, since Phase 2 changes what a row says.
   It goes *before* Phase 3 rather than into Phase 7 because it is the phase that makes
   deck pleasant to use by hand, and because Phase 3's env editor and Phase 5's rules
   editor should be written against the §11.4 dialog contract rather than retrofitted to
   it.
6. **Polish last.** Search and the health view are the cheapest things to defer and the
   least likely to invalidate anything else. The preview pane moved out of Phase 7 into
   Phase 2b because it stopped being polish the moment the layout was specified around it.

**Permission modes (`SPEC.md` §5) were unassigned to any phase** until this revision — a
planning defect, since launching Claude in a skip-permissions mode was one of the first
requirements stated. They belong with the Claude adapter, because a profile is only ever
realised as that adapter's launch flags, so they land in Phase 1.

## Deliberately not in any phase

Everything in `SPEC.md`'s non-goals, and in particular: git worktrees, a user-facing CLI,
multi-host or remote sessions, a web UI, sandboxing, inbound remote control, auto-restart on
crash, and idle reaping.
