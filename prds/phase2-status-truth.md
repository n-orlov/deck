# Phase 2 — status truth

## Goal

Make the session list answer the one question deck exists to answer: **which session needs
me?**

Today every agent row honestly reads `starting · awaiting signal` for its whole life, because
deck has no agent signal at all. This phase gives it two independent sources — a hook path
(`deck _hook`, live) and a probe path (pane-text classification, sampled) — resolves them
with §7's precedence, and makes the difference visible rather than implied. It also lands the
half of liveness that Phase 1 could not: telling a clean exit from a crash, capturing the
crash tail before the corpse is collected, and promoting a live shell pane out of `starting`.

`SPEC.md` §7 is the single authoritative state machine and this phase implements it whole.
Read §7 before writing a line of code; its transition table *is* the requirement list, and
every rule under that table is load-bearing.

`SPEC.md` is the authoritative product spec and **must not be modified**. Where this PRD and
`SPEC.md` disagree, `SPEC.md` wins and the disagreement is a finding.

## The requirement everything else serves

`SPEC.md` R6 plus one rule that outranks every feature in this phase: **deck never fabricates
a status.** A status deck cannot justify from a source must not be shown, and a source must
never be presented as better than it is. The two failure modes are equally fatal and this
phase can commit both:

- Inferring `running` for an *agent* row from a live tmux pane. §7 gives `tmux` the lowest
  precedence and only liveness. A pane being alive says nothing about whether an agent is
  working, waiting, or wedged.
- Labelling a probe verdict `live`, or a stale verdict as current. A `sampled` badge on a
  guess is honest; the same guess badged `live` is worse than no status at all, because the
  user stops checking.

A green suite that ships either of these is a failed phase, not a partial one. Phase 0 shipped
a determinism feature that passed vacuously against dead code and a reconcile loop with no
production caller; this phase has far more surface for the same mistake.

## Context

### Where work happens

The job container has no Go and no tmux and cannot install them. All building and testing
happens in the sibling toolchain container:

```sh
ci/run.sh go build ./...
ci/run.sh go test ./...
```

### What already exists — do not rebuild it

Read `docs/reports/phase0.md` (its **Gotchas** section especially) and
`docs/reports/phase1.md` before writing code, then check these specific facts, because
`docs/PLAN.md`'s summary of them is one revision behind the tree:

- **`internal/tmux` already reads the pane facts this phase needs.** `Client.List` calls
  `list-panes -F` per session and returns `Pane{ID, CurrentPath, PID, Dead, DeadStatus,
  Command}`. The gap is not in the tmux layer: `service.Reconcile` fetches those panes and
  then looks only at whether the *session* name is present. Wire up what is already there.
- **`internal/tmux` has no `capture-pane` yet.** That is new work, needed for the crash tail
  (and reused by Phase 6 for scrollback replay — do not build Phase 6's replay, but do not
  build a capture helper that cannot serve it either).
- **`killed_by_user`, `pane_exit_status`, `crash_tail`, `notify_epoch` and `acknowledged` are
  all already columns** on `sessions` at schema v1. Nothing writes any of them. **This phase
  needs no schema migration** — if you believe it does, that is a finding to record, not a
  v2 to invent.
- **`internal/hookrecv` is an empty `doc.go`.** It is the home for stdin JSON → store write
  (§2's tree). `internal/notify` is likewise empty and stays empty this phase (Phase 5).
- **`store.AcquireLaunchLease` deliberately collapses two outcomes into one**
  (`LaunchLeaseHeldElsewhere` covers both "a live owner holds it" and "the row was never
  leasable"). Requirement 31 splits them; the existing comment in `internal/store/lease.go`
  documents the collapse and must stop being true.
- `internal/agent` is the argv layer and stays pure: adapters never start a process, never
  write to the store, never touch the filesystem (§8). Everything in this phase that *does*
  those things belongs in `internal/service`, `internal/hookrecv` or `internal/tmux`.
- **The frozen clock is weaker than §13.1 claims, in three ways that all bite this phase.**
  `Clock.Advance()` has exactly one caller in the product — `internal/tui/tui.go`, after a
  successful shell creation — so there is no on-demand step. The tick count is **per process**
  (`internal/config/config.go`), so a `deck _hook` subprocess and a second TUI client each have
  their own idea of `now`. And `internal/store`'s status/event writes fall back to real
  `time.Now().UnixMilli()` when a caller passes no `At`, so one un-threaded write mixes real
  2026 time into a store frozen at 2025 and every staleness comparison in the phase becomes
  nonsense. Requirement 43 is what you build about it; all three facts are the reason it is a
  prerequisite and not a detail.
- **`Clock.Elapsed()` is monotonic time since *process start*, not since an operation began**
  (`internal/config/config.go`), and `internal/audit` writes exactly that into every record's
  `DurationMS`. Requirement 5's budget therefore cannot be asserted against the durations
  already in the log — read it before designing the measurement.

Extend the existing step library, fake agents and packages rather than duplicating any of them.

### Working practice: commit as you go

**This phase commits and pushes its own work** (see `docs/PLAN.md`). One commit per completed
task; messages say *why* and reference requirement numbers; the commit log is the durable
memory a later phase reads. `~/.gitconfig` and `~/.git-credentials` are already mounted, so
`git push origin main` works with no token handling from you — never put a token in a URL, a
file, or a message. Never force-push, amend, rebase or reset published history: fix forward.
Never commit build output, caches, secrets, or `SPEC.md`/`prds/`/`ci/Dockerfile`/`ci/SPIKE.md`.
Run build, vet and the suite before each commit; do not commit a red tree.

### Assertions this phase must deliberately change

Phase 1 pinned the *absence* of status truth into the suite. §7's shell-liveness promotion
makes some of those assertions wrong, and a red run must be fixed on the correct side. These
are the sites, all of them asserting shell rows before promotion existed:

| site | current assertion | after this phase |
|---|---|---|
| `features/lease_race.feature` (3 rows) | shell rows read `starting - awaiting signal` | shell rows reach `running` within a reconcile interval; the suffix is gone |
| `features/lease_race.feature` (3 rows) | no client's screen contains `running` | a live shell row *does* read `running` — the assertion must be re-aimed at what it was really protecting (no fabricated agent status), not deleted |
| `features/launch_lease.feature:18,29,40` | shell rows read `starting - awaiting signal` | same promotion; the `does not contain "starting elsewhere"` assertions stay exactly as they are |
| `internal/tui/tui.go` (~494, ~674) | the ` · awaiting signal` suffix is appended to every `starting` row | agent rows only (§7) — a shell row's label is plain `starting` |
| `features/concurrency.feature:21` | `the state database contains session "after crash" with status "starting"` — a **shell** row, asserted in the store, not on screen | promotion makes it `running` within a reconcile interval, and the poll runs after a screen-propagation wait, so this fails or flakes. Re-aim it at what it was protecting: that a client surviving a peer's SIGKILL still creates a durable row |
| `internal/tui/tui.go` (~1121) | helpView says a resumed row reads `starting · awaiting signal` "not 'running' — deck cannot yet tell when the agent is ready (a Phase 2 rough edge)" | the rough edge is gone; the copy must describe what deck now does. Pinned by `internal/tui/tui_test.go:26` and by the released-binary pty test `cmd/deck/main_test.go:369` — update all three together |
| `internal/tui/tui.go` (~1162) | helpView documents `DECK_CLOCK_STEP` as advancing "after each successful shell creation" | changes with requirement 43; the help overlay is the only documentation deck has (R7), so it is part of the deliverable, not a comment |
| `internal/tui/resume_test.go` | an agent row renders the suffix and never `running` | **stays valid** — it uses agent `claude`, which still has no signal until a hook or probe arrives |

Three rules about this table. First, it is **not exhaustive by construction**, and the obvious
grep will not find all of it: `features/concurrency.feature:21` asserts plain `starting` in the
*store*, with no `awaiting signal` copy anywhere near it. So sweep for shell rows asserted to be
in `starting` **by any means** — screen text, store queries, tmux facts — not just for the
suffix, and treat anything you find that is not listed here as a finding. Second, **no assertion
may be weakened to green a run.** `does not contain "running"` existed to catch a fabricated
agent status; re-aim it at an agent row and keep it. Third, when you re-aim one, say in the
commit message what the assertion was protecting and how the new form still protects it — that
sentence is the only thing standing between a re-aimed assertion and a deleted one.

## Requirements

Each requirement must be individually verifiable by a command or scenario whose real output is
recorded in the phase report (requirement 37).

**Numbering is not reading order.** Requirements **41–44 are harness prerequisites and land
first**, even though they are listed last, because nearly every scenario below is impossible
without them — three of the four are capabilities `SPEC.md` §13 already specifies but Phase 0
did not build, and discovering that halfway through is how an iteration budget evaporates.
Requirement **45** belongs with the liveness group and is placed there. The numbers are stable;
cite them.

### `deck _hook` (`SPEC.md` §3.1, §8.1)

1. `deck _hook` reads **one** JSON object on stdin, writes **one** status update and **one**
   event, and exits. It is hidden per §3.1: absent from help, absent from the UI, and never
   suggested to the user. It works with **no TUI running** — that is its whole point.
2. **It must never bootstrap the tmux server or create the store.** A hook invocation for a
   session that no longer exists, or against a `DECK_HOME` with no database, exits non-zero
   with a diagnostic on stderr and creates nothing. A stray hook that spawns a tmux server is
   a blocking defect.
3. **Session resolution, in the order §8.1 states:** by the payload's conversation id first,
   then by the session identity deck injected into the pane environment (requirement 6). Both
   paths are proved separately — the second is what makes a hook resolvable *before* the
   conversation id is confirmed, and for agents that report an id deck did not mint.
4. **Unresolvable events are stored as orphans, never dropped.** An `events` row with a NULL
   `session_id`, its kind and payload preserved. Assert the orphan is really there and that
   `_hook` still exits 0-or-documented rather than crashing.
5. **The budget, measured honestly — which requires a new measurement.** The store write
   completes in **< 20 ms uncontended**, asserted in a **single-writer** scenario from a
   monotonic duration in the JSONL log (§13.1). The durations already in the log will not do:
   `Clock.Elapsed()` measures time since **process start**, so asserting against it measures the
   whole `_hook` process lifetime — Go runtime start-up, stdin read, JSON parse and SQLite open
   included — which is both a different claim and a flake generator on a loaded runner. Add an
   **operation-scoped** duration record, and say in the report which span it covers.
   - It must **not** be asserted under `@multiclient`: SQLite may legally hold a writer up to
     `busy_timeout` under contention, so a contended assertion is a flake generator (§3.1 says
     this outright).
   - A separate scenario covers the **session-end** path, which does its one store write and
     exits with **no dispatch and no enqueue attempt of any kind**. There is no `outbox` table
     at schema v1, so this is the only implementable reading of §8.1's "enqueue only" today —
     but it is a **pinned absence, and Phase 5 must flip it** when the outbox lands. Record it
     as such in the findings and in the commit message, exactly as the assertion table above
     asks of Phase 1's pinned assertions. An absence assertion nobody knows is temporary is how
     the next phase gets fixed on the wrong side.
   - Prove the absence from the log, not from a comment.

### Per-session hook injection (`SPEC.md` §8.1)

6. **Instrumentation is per session and injected, never written into the user's settings.**
   `Adapter.Instrument(in LaunchInput) (argv []string, env map[string]string)` per §8 returns
   the argv additions and the environment the hook subprocess needs; `internal/service`
   applies them at launch. The adapter stays a pure function — if a settings *file* turns out
   to be necessary, `internal/service` writes it under `$DECK_HOME`, never the adapter, never
   the user's config, and **never anywhere inside the session's `cwd`** (R1).
   - Prefer Claude's settings-on-the-command-line with the JSON inline in argv, which keeps
     the adapter pure and leaves nothing on disk to clean up. If the real CLI rejects inline
     JSON, take the `$DECK_HOME` file path and **record which path you took and why as a
     finding** — this is a fact about an upstream CLI, so it is `@real-agents`' to confirm
     (requirement 36), not yours to assume.
7. **The hook command is the absolute path of the running deck binary**, resolved at launch,
   not the bare string `deck`. deck is a single portable binary that may not be on the agent's
   `PATH` — and under test it certainly is not the same `deck` the harness built unless the
   path is explicit.
8. **The injected environment carries what `_hook` needs to reach the right store**: at
   minimum the deck session identity and `DECK_HOME`. Without the latter a hook fired from a
   test session writes to the developer's real database — verify by asserting that a scenario's
   hook activity lands in the scenario's own `DECK_HOME` and that no write escapes it.
9. **Injected variables are deck's, not the user's.** They do not appear in the session's `env`
   map, are not editable as session env, and are not presented in the detail view as user
   configuration. (Phase 3 builds the env editor; it must not inherit a lie from here.)
10. A **`shell`** session is never instrumented and a hook naming one is an error, not a
    silent no-op.

### Hook → status mapping (`SPEC.md` §8.1, §7)

11. The mapping is **one table** keyed by event name, exactly as §8.1's table specifies:
    session start → `running` (with the source field distinguishing fresh / resumed /
    compacted), user prompt submitted → `running`, notification → `waiting` **with the
    notification type as `status_reason`**, stop → `idle` **carrying the last assistant
    message onto the row**, stop-failure → `error` with the error type as `status_reason`,
    session end → `stopped`.
12. **Every event name in that table is upstream's contract, not deck's.** The fake agent
    implements the set §8.1 declares. If a real CLI turns out not to emit one of them — or
    emits free text where §8.1 expects an enumerated type — the mapping table absorbs it (one
    unused row, or one normalisation entry that lives in the table as data), the probe path
    covers the status in the meantime, and the discrepancy is recorded as a finding for
    `@real-agents`. Do **not** redesign §8.1's event set to match a CLI you observed, and do
    **not** bury a normalisation regex inside the handler where it cannot be reviewed as data.
13. `last_message` is stored from the stop event, truncated per §4 (2 KiB), and shown in the
    detail pane. Take it from the hook payload — §8.1 is explicit that the transcript file
    lags.

### Probe engine and fixture corpus (`SPEC.md` §7, §13.5)

14. **Probe heuristics live in one table-driven file** with golden-file tests over captured
    pane text — a fixture per agent kind per state, so a spinner or prompt redesign upstream
    is a one-fixture fix. Each adapter's `Probe(pane string) (status, reason string)` consults
    that table; no classification logic hides in the TUI or the service layer.
15. **The same bytes drive the golden test and the black-box scenario.** The fake agents can
    be told to render a named fixture verbatim into their pane (requirement 42), so the UI badge assertion and
    the unit-level golden test are testing one corpus. A probe proved only by a Go unit test
    is not proved: §13 requires the behaviour be visible through the released binary.
16. **Probe eligibility is exactly §7's rule**: a `starting`, `running` **or `waiting`** row
    with no event for `stale_after` (config, default 45 s). `shell` rows are never probed —
    they have nothing meaningful to classify (§7). Probing happens on the TUI's reconcile tick;
    there is no daemon and no separate timer, and **`deck _hook` never probes** — it reconciles
    liveness only (requirement 45). §10.3's second accepted limitation depends on that line
    holding.
17. **Staleness is wall-clock-derived, and `DECK_CLOCK_STEP` must be made able to control it;** the
    requirement-5 budget is monotonic and therefore is not (§13.1). Scenarios make a row
    probe-eligible by stepping the clock (requirement 43), never by sleeping 45 seconds. Getting
    this backwards in either direction produces a test that either cannot fail or cannot pass.
18. **Precedence, implemented as §7 states it:** `user-terminal` > `hook` > `probe` > `tmux`.
    A probe never overwrites a **fresher** hook verdict; a probe *may* correct a hook verdict
    older than `stale_after`, and §7 explains why that is not a precedence violation. Prove
    both directions: a probe that is correctly ignored, and a probe that correctly wins.
    **The ignored direction needs evidence the probe ran and lost** — a probe record in the
    store or the audit log — because "the status did not change" is equally satisfied by a probe
    engine that never fired at all. That is Phase 0's vacuity failure with a new name: an
    assertion green against dead code.

### Live vs sampled (`SPEC.md` §8.1, §11)

19. A row's badge reports **the quality of its source, not a guess about the agent**:
    `status_source = hook` → **live**; `probe` → **sampled**. A status sourced from `tmux` or
    `user` carries neither badge, because neither is a claim about an agent. Pi rows are
    honestly `sampled` (§8.1 defers its event source), and the UI says so where the user will
    see it, not only in a detail view.
20. The badge is visible in the list, and the detail view additionally states the source and
    how old the verdict is. "How old" uses the frozen-clock-controllable wall clock, so it is
    assertable.

### Terminal verdicts, acknowledgement and epochs (`SPEC.md` §7, §9.1, §10.2)

21. **`x` sets `killed_by_user = 1`** with status `stopped` and source `user`, and a hook
    arriving afterwards — even milliseconds afterwards — **cannot undo it**. This is the
    precedence rule's sharpest edge and needs its own scenario: kill, then fire a `running`
    hook, and assert the row stays `stopped`.
22. **Resume clears `killed_by_user`** (§9.1). Without this the flag outranks every future
    hook for the session's whole life and the row can never leave `stopped` by automation
    again. Prove the sequence end to end: kill → resume → hook → `running`.
23. **`waiting` and `error` set `acknowledged = 0`.** `Y` acknowledges the selected row, and
    attaching acknowledges it too. The unseen marker sticks until one of those happens —
    including across a deck restart, since it is a column and not view state.
24. **Attaching to a `waiting` row also clears the status to `running`**, not merely the
    acknowledgement (§7). Answering the prompt is why the user attached, no hook fires on a
    prompt being answered, and without this rule the row stays `waiting` until the turn ends —
    a standing false positive in the one signal the product exists to provide. Scenario it
    explicitly; it is the rule most likely to be read as a nicety and dropped.
25. **`notify_epoch` increments whenever a session leaves an attention state** (`waiting` or
    `error` → anything else), in the same transaction as the status update. Nothing consumes it
    this phase — Phase 5's dedupe key does — so it must be asserted directly against the store,
    or it will be quietly wrong for a whole phase.

### Liveness, clean-vs-crash, and the crash tail (`SPEC.md` §7, §3.2)

26. **The reconcile pass implements §7's three-row observation table** using the pane facts
    `internal/tmux` already returns: session gone → `stopped`; session present, pane dead,
    status ≠ 0 → `error` with `pane_exit_status`; session present, pane alive → keep the
    current status. Today a retained dead pane leaves the row reading `starting` forever, which
    is the specific bug this requirement fixes.
27. **A shell session where the user typed `exit` is `stopped`, never a red `error`** — the
    clean-vs-crash split exists for exactly this, and getting it wrong makes the status column
    cry wolf on ordinary use.
28. **The crash tail is captured before teardown and is plain text.** `capture-pane` at death,
    last 200 lines per §4, control sequences **stripped** — unlike §9.4's replay capture, which
    keeps escapes because it is emitted into a raw pane. This tail is rendered inside deck's own
    chrome, so an un-sanitised escape from a crashing agent can corrupt the frame or worse. Cap
    and sanitise at the point of capture, not at the point of render.
29. **Dead panes are collected on sight** (§7): the same pass captures the tail, writes `error`
    + `pane_exit_status` + `crash_tail`, and *then* kills the session. Two properties, both
    required under R4 and both individually verifiable:
    - **Idempotent and unleased.** The tail is written once (`WHERE pane_exit_status IS NULL`,
      first writer wins) and killing an already-gone session is a **no-op, not an error**, so N
      clients observing one corpse need no lease between them. Prove it with a `@multiclient`
      scenario, not by reasoning about it.
    - **Never auto-relaunch** (non-goal): after a crash the launch count for the session is
      still 1. A crash loop must not be able to burn tokens or retry a destructive action.

### Unattended crash detection (`SPEC.md` §3, §7, §10.3)

45. **`deck _hook` reconciles liveness too — lazily, and this is the only way a crash is ever
    noticed with no TUI open.** §3 states it ("Liveness is reconciled by whichever TUI is
    running, and lazily by `_hook`") and §7 states the consequence: the transition to `error`
    happens "on the next TUI tick **or the next `_hook` invocation for that session, whichever
    comes first**". §10.3's third accepted limitation says the same thing from the user's side.
    Without this requirement the whole promise is unbuilt, because requirements 26 and 29 give
    dead-pane collection to the reconcile tick alone.
    - After its status write, and **outside** the requirement-5 budget window, a `_hook`
      invocation on the **non-session-end** path runs **one bounded liveness pass** — the same
      code as the TUI tick, dead-pane collection included. This is precisely why requirement 29
      demanded idempotent, unleased collection: a TUI and a hook can now race for the same
      corpse, and neither may fail because the other won.
    - **Not on the session-end path**, which shares a ~1.5 s budget with the user's own hooks
      (§8.1) and must stay one write and exit.
    - **Bounded by a timeout.** A stalled `tmux` must not hang the agent's hook, because that
      hook is blocking the agent.
    - **Liveness only, never probing.** §10.3's second limitation is explicit that
      probe-classified agents change status only while a TUI runs; a hook that started probing
      would quietly make that documented limitation false and put pane-text heuristics on the
      agent's critical path.
    - The scenario that proves it: **with no TUI running at all**, SIGKILL one agent, fire a hook
      for a *different* session, and assert the killed row becomes `error` with its crash tail.
      Assert it against the store, since there is no screen to read.

### Shell liveness promotion (`SPEC.md` §7)

30. **For `shell` rows only, tmux liveness promotes `starting → running`**, where `running`
    means "the pane is alive". It does **not** generalise: inferring `running` for an agent row
    from a live pane is the fabricated status §7 exists to forbid. Both halves need a scenario —
    the shell row that gets promoted, and the agent row in the same state that does not. The
    ` · awaiting signal` suffix becomes **agent-only copy**, and the assertion table above lists
    every site that changes.

### The resume path (`SPEC.md` §9.3)

31. **"starting elsewhere" is a claim about another client, so it is only made when one is
    actually there.** Split the lease outcome: another client holds a live in-TTL lease →
    *starting elsewhere*; the row was never leasable because it is not `stopped` → the row's own
    status and reason (*already running*, *already starting*). The store's answer distinguishes
    "held by \<owner\>" from "not leasable, status is \<status\>", and the UI says which.
    Reporting the second as the first sends the user hunting for a TUI that does not exist and
    hides the real state of the row — the same class of lie as a fabricated status. Both
    messages get a scenario, and the existing `launch_lease.feature` assertions for the live-lease
    case must keep passing unchanged.

### Scenarios that define this phase

`SPEC.md` §13.3 already names this phase's files — `status_claude_hooks.feature`,
`status_probe.feature` and `crash.feature` — and §13.3's rule is that a file is named for the
**area of behaviour**, never for the phase. Use those three names, put the rest where the area
says (lease outcomes in `launch_lease.feature`, promotion where the shell rows already are),
and add no phase-named file.

32. **T3's status half** (`SPEC.md` §13.4, third scenario), written as the spec has it **minus
    its webhook steps** — those need Phase 5 and are added there (`docs/PLAN.md` explains the
    split). So: a running `claude` session; the agent fires notification type
    `permission_prompt`; the row shows `waiting` with reason `permission_prompt` within one
    reconcile; the agent fires `stop` with a message; the row shows `idle` and the detail pane
    contains that message; the prompt fires again. Assert `notify_epoch` advanced across the
    resolution (requirement 25). **Do not stub the notification steps** — absent, not faked,
    per §11.3's footer rule applied to tests: a step that pretends to assert delivery is worse
    than no step.
33. **T2 completed** (`SPEC.md` §13.4, second scenario). Phase 1 stopped at "all three clients
    show `starting`". Now: the agent for the raced row fires `session_start` and **after one
    reconcile all three clients show `running`**. Note `lease_race.feature` currently races a
    *shell* row, which after requirement 30 reaches `running` by promotion and so cannot prove
    this — the agent-row case is what closes T2, and both belong in the suite.
34. **The two exit scenarios from §13.4** verbatim in spirit: a shell session that exits cleanly
    is `stopped`, with **no crash tail recorded**; a `claude` session whose agent process is
    killed with `SIGKILL` (requirement 44 — the process, not the pane) shows
    `error` within one reconcile, **the crash tail contains the last lines that were on the
    pane**, and the launch count is still 1.
35. **A `@multiclient` propagation scenario**: a hook-driven status change is visible on every
    client within one reconcile interval. R4 is not a Phase 1 achievement to be assumed; it is
    a property every phase can break.
36. **`@real-agents` conformance grows to cover this phase** (§13.5): against an actually
    installed `claude`, does the hook mechanism still inject, does a hook still fire at
    `deck _hook`, and does its payload still carry `session_id`, `cwd`, `transcript_path` and
    `permission_mode`? Excluded from the default run, runnable by one documented command, and
    **expected** to break when Claude upgrades — that is its purpose. Every §8.1 assumption you
    could not confirm belongs here as a scenario rather than in a comment.

### Evidence and stability

37. `docs/reports/phase2.md` records, with **real unedited command output**: the full test run
    with feature/scenario/step counts and a top-level Go test count under a stated counting
    convention; one recorded run or named scenario per numbered requirement; resolved tool
    versions; the wall-clock duration of a full suite run; and every gotcha discovered with its
    consequence. Every capture it cites must be a **repository-relative path that exists**.
38. **The suite passes ten consecutive times from a clean state**, with the loop's real output
    recorded. Two passes are not evidence: Phase 0's "passes twice" bar was met while a scenario
    failed one run in three, and this phase adds timing-sensitive behaviour to exactly the paths
    where that happened.
39. No scenario may be deleted, skipped or tag-excluded to make the suite pass, and none of the
    Phase 1 assertions in the table above may be weakened. Fix the scenario instead and say so.
40. **An operator smoke-test walkthrough** in `docs/reports/phase2.md`: the exact commands and
    keystrokes to build deck, create a real `claude` session, watch the row go
    `running → waiting → idle` while working in it, trigger a crash and see the tail, and
    confirm `Y` clears the marker — plus what "working" looks like at each step and every rough
    edge that remains. This phase's deliverable is a status column the operator can trust; the
    report has to show them how to check that for themselves.

### Harness prerequisites (build these first)

`SPEC.md` §13.2 describes fake agents that "print recognisable pane text on demand, fire hook
payloads at `deck _hook` on command, and can be told to hang, crash, or exit". Phase 0 built
only the last of those: `cmd/fake-claude` honours the argv contract, writes transcripts, and
takes `FAKE_CLAUDE_EXIT_CODE`. Everything else in this list is genuinely missing, and every one
of them gates scenarios above.

41. **The fake agents can fire each §8.1 event at `deck _hook` on command**, with a
    controllable payload, **from inside their own pane using the environment deck injected**.
    This distinction is the requirement, not an implementation detail: a harness step that pipes
    JSON straight into `deck _hook` proves the *verb* but proves nothing about instrumentation
    (requirements 6–8), because it supplies by hand exactly the wiring under test. Both
    assertions are needed and they are different — keep them separate and label them honestly.
42. **The fake agents can render a named fixture verbatim into their pane.** This is what makes
    requirement 15 possible — the same bytes reaching the released binary through a real pane as
    reach the golden test — and without it the probe corpus can only ever be unit-tested.
43. **The frozen wall clock becomes advanceable on demand, and one `DECK_HOME` gets one
    answer for `now`.** §13.1's contract is that `DECK_CLOCK_STEP` "advances it **on demand**";
    the tree delivers a single trigger, per-process tick counts, and a real-time fallback for
    un-threaded store writes (see "What already exists"). All three must be fixed together,
    because fixing only the first leaves scenarios that pass on one client and fail on another:
    - An advance mechanism a scenario can invoke against a **running** client. An env var read
      at startup cannot do this, so this is new product surface: design it, document it in the
      help overlay, and record it as a finding for the operator to fold into `SPEC.md`.
    - Every process sharing a `DECK_HOME` — TUI clients and `deck _hook` subprocesses alike —
      must agree on the frozen `now`. A hook that writes a real-2026 `status_at` into a store
      frozen at 2025 makes every staleness comparison in this phase meaningless, and the
      failure looks like a probe bug rather than a clock bug.
    - **No status or event write may take the real-time `At` fallback.** Thread the clock
      through every writer on this phase's paths and make the fallback impossible to reach by
      accident, rather than remembering not to reach it.
    Requirements 16–18 and 20 are untestable until this exists, and requirement 5 is
    *un-falsifiable* without the monotonic guarantee it preserves.
44. **The harness can SIGKILL the agent process inside a pane.** The existing step
    (`features/assertions_test.go`) kills a *deck client*, which is a different thing entirely;
    requirement 34 needs the process in the pane, and `tmux.Pane.PID` already carries what is
    needed to find it. A crash scenario that kills the pane instead of the process is not
    testing what §7 means by a crash.

## Review guidance

Block on **real defects**; do not block on documentation nits (see `docs/PLAN.md`). In this
phase specifically, these are blocking:

- Any fabricated status: `running` inferred for an agent row from tmux liveness, a probe verdict
  badged `live`, a stale verdict presented as current, or a status written with a source it did
  not come from.
- A precedence violation: a hook resurrecting a user kill, or a probe overwriting a fresher hook
  verdict.
- A probe proved only by a Go unit test, or a fixture corpus whose bytes never reach the
  released binary through a pane — the Phase 0 vacuity failure repeated on new ground.
- A hook path that writes outside its `DECK_HOME`, bootstraps a tmux server, or leaves anything
  inside a session's `cwd` (R1).
- An un-capped or un-sanitised crash tail, or a dead-pane collection that is not idempotent
  across clients.
- Weakening or deleting a Phase 1 assertion listed above instead of re-aiming it.
- The 20 ms budget asserted under contention (a flake factory), asserted against a frozen clock
  (unfalsifiable — it would measure zero), or asserted against a process-lifetime duration
  dressed up as an operation duration (a different claim wearing the right number).
- A liveness pass on the **session-end** hook path, or any probing from `deck _hook` — both
  falsify a documented limitation in §10.3 and put deck's work on the agent's critical path.
- A status or event write that reaches `internal/store`'s real-time `At` fallback on any path
  this phase touches. It cannot be caught by reading the diff and it makes staleness arithmetic
  silently wrong, which is indistinguishable from a probe bug.
- Any notification dispatch, channel, outbox or rules evaluation: that is Phase 5, and building
  it here means building it against unreviewed status behaviour.

Wording, formatting and stale sentences in derived summaries are notes, not blockers. The test
is whether a reader would be **misled about behaviour**.

## Findings, not spec edits

Record in `docs/reports/phase2-findings.md` anything the spec left undefined that you had to
decide, anything contradictory or impossible, and every deferral with its target phase. In
particular: every §8.1 event, payload field or notification type you could not confirm against
a real CLI; which hook-injection mechanism you took and why (requirement 6); and every badge or
copy decision §11 does not pin down. If a new `DECK_*` control is needed to make something
testable, introduce it, document it in the help overlay, and record it as a finding for the
operator to fold into `SPEC.md` — do not silently invent configuration. Durations use a
**monotonic** source and keep advancing while `DECK_CLOCK` is frozen.

## Non-goals for this phase

Do not implement, even partially: notification channels, rules, quiet hours, the outbox, dedupe
dispatch or redaction (Phase 5 — this phase maintains `notify_epoch` and nothing else about
notifications); the §11 sidebar, preview pane, layout modes, themes or the settings takeover
(Phase 2b — this phase adds a badge and copy to the list deck already has, and no new chrome);
the env editor, `env↻`, restart-to-apply, kill/undo toasts, `dd` tombstones, purge, archive,
bulk marks or rename (Phase 3); the Codex adapter and its id discovery (Phase 4); scrollback
replay, history files, `last_cwd` tracking or `sensitive` (Phase 6 — but see requirement 28:
the capture helper this phase adds should not be shaped so that Phase 6 has to replace it);
cross-session search, health view, attention sort, grouping or send-without-attach (Phase 7);
systemd units.

Do not add a user-facing command line (`SPEC.md` R7). `deck _hook` is a hidden internal verb
for an external caller, and the distinction is the whole of §3.1.

## Constraints

- Commit and push as described. Do not rewrite history.
- Do not modify `SPEC.md`, anything under `prds/`, `ci/Dockerfile`, or `ci/SPIKE.md`.
- Do all building and testing in siblings via `ci/run.sh`. Install nothing into the job
  container.
- **The default suite must not depend on a real agent binary, network access, or model output.**
  Only `@real-agents` scenarios may touch an installed CLI, and they are excluded by default.
- deck never writes to or deletes anything inside a session's `cwd` (R1). This phase gains two
  new ways to break it — the hook settings path and the crash-tail capture — so it is a live
  risk here, not a background one.
- Every sibling container is `--rm`, so no cleanup sweep is needed or wanted. **Never run
  `docker rm`/`docker kill` filtered by `label=ralphd.run=...`, and never target the container
  named `ralphd-deck-phase2`: your own job container carries that label, so such a sweep
  SIGKILLs the job mid-iteration and loses the verdict.** If you must check for leftovers, do it
  read-only and scoped to the image, e.g.
  `docker ps -a --filter ancestor=deck-ci:local --format '{{.ID}} {{.Status}}'`. Removing an
  image you built yourself (`docker rmi <your-image>`) is fine; sweeping by run label is not.
- Prefer a small, readable implementation over a complete one. This phase is judged by whether
  the status column can be trusted, not by feature count.
