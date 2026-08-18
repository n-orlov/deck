# Phase 1 — durable identity and agents

## Goal

Make deck actually run coding agents, and make a conversation survive a reboot.

This is the phase where deck stops being a shell-session manager and becomes the product:
create a named `claude` session, work in it, kill it or reboot the host, press `r`, and get
**the same conversation** back — with the launch audit proving it resumed by explicit id and
never by `--continue`. It is also the first phase the operator can use by hand against a real
agent, so it must be usable, not merely correct.

It carries permission profiles (`SPEC.md` §5), because a profile is only ever realised as an
adapter's launch flags, and launch leases (§9.3), because deck is routinely driven from
several TUIs at once and two of them pressing `r` must not start two agents.

`SPEC.md` is the authoritative product spec and **must not be modified**. Where this PRD and
`SPEC.md` disagree, `SPEC.md` wins and the disagreement is a finding.

## The requirement everything else serves

`SPEC.md` R2: **several sessions may live in one directory.** That single fact bans every
"most recent conversation here" shortcut — `claude --continue`, `codex resume --last` — and
forces explicit-id resume everywhere. A resume path that works only when a directory holds
one session is a failed phase, not a partial one.

## Context

### Where work happens

The job container has no Go and no tmux and cannot install them. All building and testing
happens in the sibling toolchain container:

```sh
ci/run.sh go build ./...
ci/run.sh go test ./...
```

### What Phase 0 already gives you

Read `docs/reports/phase0.md` — especially its **Gotchas** section — before writing code, and
do not rediscover any of it. The harness already provides: a pty driver answering Bubble
Tea's OSC 11/CPR probes and reading a vt10x cell grid; per-scenario isolated `DECK_HOME` and
tmux socket with teardown that fails on leaks; multi-client scenarios; direct tmux assertions
(session exists, pane command, options); store, log and permission assertions; **a step that
kills the tmux server as the in-suite stand-in for a host reboot**; and `cmd/fake-claude`, a
faithful fixture already validating `--session-id` as a UUID, accepting `--resume` and
`--permission-mode` with the real enum, with a controllable exit status.

Extend the existing step library and fixture rather than duplicating either.

### Working practice: commit as you go

**This phase commits and pushes its own work** (see `docs/PLAN.md`). One commit per completed
task; messages say *why* and reference requirement numbers; the commit log is the durable
memory a later phase reads. `~/.gitconfig` and `~/.git-credentials` are already mounted, so
`git push origin main` works with no token handling from you — never put a token in a URL, a
file, or a message. Never force-push, amend, rebase or reset published history: fix forward.
Never commit build output, caches, secrets, or `SPEC.md`/`prds/`/`ci/Dockerfile`/`ci/SPIKE.md`.
Run build, vet and the suite before each commit; do not commit a red tree.

### What status can honestly be in this phase

Hooks and probes are Phase 2. Without them there is **no agent signal**, so a launched agent
row stays `starting` and must display `starting · awaiting signal` exactly as `SPEC.md` §7
prescribes. **Do not invent a `running` transition from tmux liveness** — §7 gives `tmux` the
lowest precedence and only liveness. An honest `starting · awaiting signal` is correct here;
a fabricated `running` is a defect.

## Requirements

Each requirement must be individually verifiable by a command or scenario whose real output
is recorded in the phase report (R21).

### Adapter layer

1. An adapter registry where each adapter **declares its capabilities** rather than having
   them assumed (`SPEC.md` §5, §8): which permission profiles it supports, whether it accepts
   a caller-assigned conversation id, and its launch and resume argv construction. Adding an
   adapter must not require touching the TUI.
2. **Claude adapter.** Launch assigns a deck-generated conversation id via
   `--session-id <uuid>` (a valid UUID — reuse `DECK_ID_SEED` so tests are reproducible).
   Resume uses `--resume <uuid>`. **`--continue` and any "most recent" form must never appear
   in any argv deck constructs** — assert this negatively.
3. **Pi adapter.** Launch and resume both use `--session-id <id>`, which creates the session
   if missing. Declare capabilities honestly: `plan` is unsupported and falls back to `safe`
   with the fallback shown in the UI, per §5.
4. `shell` remains an agent type with no conversation id and no permission profile; its rows
   must not display a meaningless profile badge.
5. `conversation_id` is persisted on the row as soon as deck assigns it, and the launch audit
   records the exact argv plus the **names** of applied environment variables, never values
   (Phase 0 R8).

### Permission profiles (`SPEC.md` §5)

6. The four deck profiles `safe | plan | edits | yolo` translate per adapter exactly as §5's
   table specifies, including Claude's `manual | plan | acceptEdits | bypassPermissions`.
   Prefer the structured `--permission-mode` flag over any `--dangerously-*` form.
7. A profile an adapter does not support **degrades to the nearest safe one and says so** in
   the row detail. It never silently lies, and the create modal only offers profiles the
   selected adapter declares.
8. The profile is **persisted**: a `yolo` session comes back `yolo` on resume. It is badged in
   the list and in the detail pane.
9. `yolo` is gated twice: `allow_yolo = true` in config (**default false**), *and* an explicit
   confirm in the create modal. With the config false, `yolo` is not offered at all, and the
   UI states why rather than hiding the option silently.
10. `P` switches the profile of an existing session. Because a mode change only reaches a new
    process, it is restart-to-apply and says so; it must not claim to have taken effect on a
    live pane.

### The create modal an agent launch needs

11. The create modal takes: **name**, **cwd**, **agent** (`claude | pi | shell`),
    **permission profile** (offered per capability), **launch_args** (JSON array),
    **env** map, **pre_launch** (one shell line), **login_shell**. Every field is reachable by
    keyboard alone and the modal states what each does — the TUI is the only documentation
    there is (R7, no CLI).
12. Validation is stated, never silent: duplicate name, slug collision, missing or
    non-directory `cwd`, malformed env key, malformed `launch_args`, unsupported profile —
    each produces a specific in-modal message and retains what the user typed. `esc` creates
    nothing.
13. `captured_path` records the `PATH` in effect at create time and takes part in resolution
    in the §6.3 order: server env → `captured_path` → config `[env]` → session `env`.
    `login_shell = 1` runs the pane command via `$SHELL -lc`, which marks `captured_path`
    advisory — the two are mutually exclusive by design and the UI says so.
14. `pre_launch`, when set, runs in the pane **before** the agent, in the same pane, and its
    failure is visible rather than swallowed. It exists so secrets need not be stored in
    `env` at all (§6.4) and help should say so.

### Resume, on demand (`SPEC.md` §9.1)

15. `r` on a `stopped` session recreates `deck_<slug>` at its `cwd`, runs `pre_launch`, and
    launches the agent with its **resume** argv, its env, and its permission profile. The
    session enters `starting`. **No prompt is ever re-sent** — resume reconstitutes context,
    it never resumes autonomous work.
16. **Resume failure** — unknown conversation id, missing directory, agent binary not on PATH
    — sets `error` with the reason and **retains the row**. It must never delete the row and
    must never silently start a *fresh* conversation in place of a failed resume. Each of the
    three causes is proved separately.
17. Pinning: `p` sets `resume_state = pinned` with `resume_pin` forcing a specific
    conversation id, sticky across restarts. A one-shot "start fresh" reverts to `auto`
    afterwards rather than staying cleared.
18. **Reboot survival, end to end.** After the tmux server is killed and deck restarted,
    every row reads `stopped` labelled `resumable`, **no tmux session exists** (nothing
    auto-started, R3), and `r` resumes by explicit id.

### Launch leases (`SPEC.md` §9.3)

19. The transaction that flips `stopped → starting` CAS-acquires `launch_lease_owner`
    (`pid@boot_id`) and `launch_lease_until` (TTL ~30 s). A losing client shows
    **"starting elsewhere"**, not an error.
20. A **stale** lease is breakable — dead owner pid, or expired TTL — and a lease held by a
    live owner within TTL is not. Both directions are proved and neither wedges the row.
21. A `@multiclient` scenario proves that when N clients race `r` on one row, **exactly one
    tmux session exists** and exactly one launch appears in the log. Assert the tmux fact,
    not only the UI text. (`SPEC.md` §13.4's T2 continues into `running` on a hook signal;
    that half belongs to Phase 2 — stop at all clients showing `starting`.)

### Scenarios that define this phase

22. **T1 — `SPEC.md` §13.4, the headline scenario, implemented verbatim in spirit:** three
    sessions `alpha`, `beta`, `gamma` for agent `claude` in **one** directory, each having
    exchanged a distinct message; kill the tmux server; restart deck; all three read
    `stopped · resumable` with no tmux session anywhere; resume `beta`; its launch audit
    contains `--resume <beta.conversation_id>` and **does not contain `--continue`**; and
    **`beta` replays its own last message, not `alpha`'s**.
    To make the last assertion real, the fake agent must keep a per-conversation transcript
    keyed by its assigned id — at the real path, with the real naming convention, as Phase 0's
    fidelity rules require — and replay that transcript on `--resume`. A fake that cannot tell
    two conversations apart cannot prove this requirement.
23. A `same_directory` scenario proving R2 directly: two sessions created in one `cwd` get
    **different** conversation ids, and resuming one never produces the other's id in argv.
24. A `permission_modes` scenario: each profile produces the argv §5 specifies for the chosen
    adapter; an unsupported profile degrades visibly; `yolo` is unavailable with
    `allow_yolo = false` and requires the explicit confirm when enabled; and the profile
    survives a resume.
25. **A real-agent smoke scenario, tagged `@real-agents` and excluded from the default run.**
    If a real `claude` is on PATH it creates a session, asserts deck assigned a UUID
    conversation id, and asserts a resume passes that same id. It must be runnable by one
    documented command so the operator can validate their own machine. It is expected to be
    skipped in CI and to fail if Claude's flags change upstream — that is its purpose.

### Evidence and stability

26. `docs/reports/phase1.md` records, with **real unedited command output**: the full test run
    with feature/scenario/step counts and a top-level Go test count under a stated counting
    convention; one recorded run or named scenario per numbered requirement; resolved tool
    versions; the wall-clock duration of a full suite run; and every gotcha discovered with
    its consequence. Every capture it cites must be a **repository-relative path that exists**.
27. **The suite passes ten consecutive times from a clean state**, with the loop's real output
    recorded. Two passes are not evidence: Phase 0's "passes twice" bar was met while a
    scenario failed one run in three.
28. No scenario may be deleted, skipped or tag-excluded to make the suite pass. Fix the
    scenario instead and say so.
29. **An operator smoke-test walkthrough** in `docs/reports/phase1.md`: the exact commands and
    keystrokes to build deck, create a real `claude` session, exchange a message, kill the
    tmux server to simulate a reboot, resume, and confirm the conversation came back — plus
    what "working" looks like at each step, and the known rough edge that an agent row reads
    `starting · awaiting signal` until Phase 2 lands hooks. This phase's whole point is that
    it becomes hand-usable; the report must show how.

## Review guidance

Block on **real defects**; do not block on documentation nits (see `docs/PLAN.md`). In this
phase specifically, these are blocking:

- Any argv deck constructs containing `--continue`, `resume --last`, or any "most recent"
  form; or a resume path that only works when a directory holds one session.
- A fabricated `running` status with no agent signal behind it.
- A fake agent that cannot distinguish two conversations, making requirement 22's replay
  assertion vacuous — the exact class of defect that got past a green suite in Phase 0.
- A permission profile that claims support it does not have, or a `yolo` path reachable
  without both gates.
- A test asserting only via unit test what it claims to prove black-box.

Wording, formatting and stale sentences in derived summaries are notes, not blockers. The
test is whether a reader would be **misled about behaviour**.

## Findings, not spec edits

Record in `docs/reports/phase1-findings.md` anything the spec left undefined that you had to
decide, anything contradictory or impossible, and every deferral with its target phase. If a
new `DECK_*` control is needed to make something testable, introduce it, document it in the
help overlay, and record it as a finding for the operator to fold into `SPEC.md` — do not
silently invent configuration. Durations use a **monotonic** source and keep advancing while
`DECK_CLOCK` is frozen (Phase 0 R7).

## Non-goals for this phase

Do not implement, even partially: `deck _hook`, hook receipt, per-session settings injection,
status probes, live/sampled badges or any `running`/`waiting`/`idle` transition (Phase 2);
the clean-vs-crash exit split and crash tail (Phase 2); the Codex adapter and its id
discovery (Phase 4); the env **editor**, `env↻` and restart-to-apply (Phase 3 — this phase
only needs env *set at create time*); kill/undo toasts, `dd` tombstones, purge, archive or
bulk marks beyond Phase 0's plain `x` kill (Phase 3); notifications (Phase 5); scrollback
capture, history files or cwd tracking (Phase 6); preview pane, search, health view, session
send (Phase 7); systemd units.

Do not add a user-facing command line (`SPEC.md` R7).

## Constraints

- Commit and push as described. Do not rewrite history.
- Do not modify `SPEC.md`, anything under `prds/`, `ci/Dockerfile`, or `ci/SPIKE.md`.
- Do all building and testing in siblings via `ci/run.sh`. Install nothing into the job
  container.
- **The default suite must not depend on a real agent binary, network access, or model
  output.** Only the `@real-agents` scenarios may touch an installed CLI, and they are
  excluded by default.
- deck never writes to or deletes anything inside a session's `cwd` (R1). It is not this
  phase's headline, but it remains true and any violation is a blocking defect.
- Every sibling container is `--rm`. Leave no stray containers, images beyond
  `deck-ci:local`, or volumes beyond the shared cache.
- Prefer a small, readable implementation over a complete one. This phase is judged by
  whether a conversation provably survives a reboot, not by feature count.
