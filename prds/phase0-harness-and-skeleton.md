# Phase 0 — BDD harness and walking skeleton

## Goal

Stand up `deck`'s foundation: a Go module, a real (if nearly featureless) TUI binary, its
store and tmux layer, and — the actual point of this phase — a **black-box BDD harness that
drives the released binary through a pty and proves behaviour end to end**.

Almost nothing user-visible ships here. What ships is the ability to *prove* everything
that comes after. Every later phase is specified as Gherkin against this harness, so if the
harness is weak, every later phase is unverifiable.

`SPEC.md` in this repository is the authoritative product spec. This PRD implements a
strict subset of it. Where this PRD and `SPEC.md` disagree, **`SPEC.md` wins** and the
disagreement is a finding to record (see *Findings*, below).

## Context the agent needs

### How this job is run, and where work happens

This job runs with `--allow-docker`, so the **toolchain-in-a-sibling** capability is
available and is the *only* supported way to build and test here: the job container has no
Go and no tmux, and cannot install them.

The wrapper is **already built, proven, and committed**: `ci/Dockerfile` (Go 1.25 + tmux
3.5a) and `ci/run.sh`. Use them:

```sh
ci/run.sh go build ./...
ci/run.sh go test ./...
```

`ci/run.sh` already handles the host-path bind mount, `--user`, and a shared module/build
cache. Do not reinvent it. If it needs extending (extra mounts, extra tools), extend it and
say so in the report.

### What the toolchain spike already proved — do not re-litigate

`ci/SPIKE.md` records measured results. The load-bearing ones:

1. **A pty is not a terminal emulator.** Bubbletea probes the terminal at start-up —
   background colour (OSC 11) and cursor position (CPR) — and **blocks waiting for replies
   before rendering its first frame**. A harness that only reads will hang and look like a
   broken TUI. The harness **must** answer those probes. Working code for this exists in
   `.spike/pty_test.go` — port it, don't rediscover it.
2. Real `tmux` works in the sibling on a private `-L` socket, including `capture-pane`
   read-back and server teardown. Working code in `.spike/tmux_test.go`.
3. Use `sh -c`, never `sh -lc`: the login shell resets `PATH` and the toolchain vanishes
   (`go: not found`).
4. Siblings must not leave root-owned files; `ci/run.sh` already ensures this. Keep it true.

`.spike/` is a throwaway scratch module (gitignored). Port its two techniques into the real
harness, then **delete `.spike/`** and drop it from `.gitignore`.

## Fidelity rules for mocked agents

Tests must not depend on a real coding agent being installed, network access, or model
output. Mocking the agents is therefore **required** — but a mock is only acceptable if its
*observable behaviour is indistinguishable from the real thing in the respects deck depends
on*. Concretely:

- **Real flag contract.** A fake agent accepts exactly the flags deck passes to the real
  one, with the real names and the real value shapes (e.g. a session id that must be a
  valid UUID is validated as one), and **fails the same way** on a flag the real CLI would
  reject.
- **Real on-disk layout.** Where deck reads an agent's files, the fake writes them at the
  real path, with the real naming convention and the real record shape.
- **Real terminal output shapes.** Where deck classifies pane text, the fake emits text of
  the same shape as the real agent's.
- **No convenience back channels.** A fake must never expose an interface the real agent
  lacks (no "tell deck your status" side channel). Deck must not be able to tell it is
  talking to a fake.
- **Drift alarm.** A conformance check, tagged `@real-agents` and **excluded from the
  default suite**, asserts the fake's flag contract still matches the installed real CLI
  (from its `--help`). It is expected to fail when an agent upstream changes; that is its
  purpose.

## Requirements

Each requirement must be individually verifiable by a command whose real output is recorded
in the phase report (R20).

### Module and build

1. A Go module `github.com/n-orlov/deck` at the repository root, Go 1.25, building cleanly
   with `ci/run.sh go build ./...` and vetting cleanly with `ci/run.sh go vet ./...`.
   Package layout follows `SPEC.md` §2. Build output goes to a gitignored directory.
2. `ci/run.sh go test ./...` is the **single** command that runs everything — unit tests and
   the whole BDD suite — and it exits non-zero if anything fails. The BDD suite is wired in
   as a normal Go test (godog's test-runner integration), not a separate script.

### The binary

3. `deck` with no arguments starts the TUI, renders a session list, and exits cleanly on
   `q` with status 0. With an empty store it renders a stated empty state, not a blank
   screen or an error.
4. `?` opens a help overlay listing the keys implemented in this phase and the `DECK_*`
   environment variables; a second `?` or `esc` closes it.
5. If `tmux` is missing or older than the minimum in `SPEC.md` §2, the TUI renders an
   actionable error state naming the problem instead of crashing or hanging.
6. The determinism controls of `SPEC.md` §13.1 are implemented and honoured: `DECK_HOME`,
   `DECK_TMUX_SOCKET`, `DECK_CLOCK`, `DECK_ID_SEED`, `DECK_RECONCILE_MS`,
   `DECK_PREVIEW_MS`, `DECK_ASCII`, `DECK_ANIM`, and `NO_COLOR`. `DECK_HOME` unset falls
   back to XDG resolution.
7. **`DECK_CLOCK` freezes wall-clock time only.** Every duration, timeout and elapsed
   measurement uses a monotonic source and keeps advancing while the wall clock is frozen.
8. A structured JSONL log at `$DECK_HOME/log/deck.jsonl`: one object per line, each with at
   least a monotonic-derived duration where a duration is meaningful, an event name, and a
   session id where one applies. It records every session state transition and, for every
   session launch, a **launch audit** entry carrying the exact argv and the *names* (never
   the values) of environment variables applied.

### Store

9. SQLite at `$DECK_HOME/state.db`, opened with `journal_mode=WAL`, `busy_timeout=5000`,
   `foreign_keys=ON`, created mode `0600` inside a `0700` directory.
10. Schema v1 covering the columns of `SPEC.md` §4 that this phase uses, plus the `events`
    table and a `meta` row holding the schema version. Unused-in-this-phase columns may be
    created now or added later, but the version must be recorded and checked.
11. A migration mechanism exists and is exercised: opening a store whose recorded version is
    newer than the binary supports fails with a clear message rather than corrupting it;
    opening an older supported version migrates it. Both paths are covered by a test.
12. Every mutation is a targeted `UPDATE`/`INSERT` for one row inside a transaction. There is
    no code path that rewrites the whole session list from in-memory state
    (`SPEC.md` §4 invariant, R4).

### tmux layer

13. All tmux interaction happens on a private socket (default `deck`, overridable by
    `DECK_TMUX_SOCKET`), never the default socket. deck creates the server itself when
    absent and sets, in that same invocation, every server/window option listed in
    `SPEC.md` §3.2 — `exit-empty off`, `remain-on-exit failed`, `window-size latest`,
    `aggressive-resize on` — verified by reading them back with `show-options`.
14. Slug derivation per `SPEC.md` §3.2: `[a-z0-9_-]+`, `.` and `:` excluded, uniqueness
    enforced by the store, tmux session named `deck_<slug>`. A name that collides is
    rejected in the UI with a stated reason.
15. Create, list, kill and attach are implemented over that socket. Attach hands the
    terminal to tmux and returns to a live, redrawn TUI on detach.

### Sessions in this phase

16. A create modal (`n`) takes a name and a working directory and creates a **`shell`**
    session: a tmux session running the user's shell in that directory. It appears in the
    list of every running TUI. No other agent type is implemented in this phase.
17. `x` kills the selected session: the tmux session is gone, the row survives as `stopped`
    and is labelled resumable, and nothing inside the session's working directory is
    touched — ever.
18. A 500 ms-default reconcile loop marks sessions whose tmux session has disappeared as
    `stopped`, using `list-sessions`/`list-panes` output, and logs the transition.

### The harness

19. A godog-based BDD harness meeting `SPEC.md` §13.2, with steps sufficient for the
    features below:
    - drives the **real built binary** in a pty at a fixed geometry, answering OSC 11 and
      CPR probes (see Context);
    - reads the screen from a VT100 emulator's cell grid and normalises it (trailing
      whitespace stripped, non-frozen timestamps masked) before matching;
    - spawns N independent clients against one `DECK_HOME` for `@multiclient` scenarios;
    - asserts tmux facts directly (session exists, pane command, session options);
    - asserts store and log facts by reading the files, and file-permission facts;
    - gives every scenario a fresh `DECK_HOME` and a fresh tmux socket, and **fails the
      scenario** on teardown if a tmux server, stray process or temp dir leaks;
    - provides a step that kills the tmux server, as the in-suite stand-in for a host
      reboot (`SPEC.md` §13.2).
20. Feature files exist and pass:
    - `walking_skeleton.feature` — start with an empty store and see the empty state; create
      a shell session; it is listed; the tmux session exists on the private socket with the
      right name and working directory; attach; detach; kill; the row remains `stopped`; the
      working directory is untouched.
    - `determinism.feature` — `NO_COLOR`/`DECK_ASCII` produce byte-stable frames; a frozen
      `DECK_CLOCK` produces stable relative times while a measured duration in the log still
      advances (proves R7); `DECK_ID_SEED` reproduces ids.
    - `store.feature` — file mode `0600`, WAL enabled, version recorded, both migration
      paths of R11.
    - `tmux_contract.feature` — private socket only, every option from R13 read back,
      slug/collision rules of R14.
    - `concurrency.feature` (`@multiclient`) — a session created in client A appears in
      clients B and C within one reconcile; a kill in B is reflected in A and C; `SIGKILL`
      of client C mid-run leaves A and B working and the store intact.
21. A fake agent binary exists, built from this repo, honouring the fidelity rules above for
    the flags deck will pass to Claude Code (`--session-id <uuid>` validated as a UUID,
    `--resume <uuid>`, `--permission-mode <mode>` with the real accepted values), plus a
    deterministic banner on stdout and a controllable exit status. This phase only needs it
    to prove the fixture *mechanism*: a scenario launches it as a session's command and
    asserts the argv reaching it. The `@real-agents` conformance check of the fidelity rules
    exists and is excluded from the default run.

### Evidence

22. `docs/reports/phase0.md` records, with **real unedited command output**: the full test
    run (counts of features/scenarios/steps and unit tests), one recorded run per numbered
    requirement above or an explicit statement of which scenario covers it, the resolved
    versions (Go, tmux, godog, bubbletea, the VT100 and pty libraries), the wall-clock
    duration of a full suite run, and every gotcha discovered.
23. The whole suite passes from a clean state — `ci/run.sh go test ./...` — twice in a row,
    with the second run starting from the artifacts of the first, proving no test leaks state
    into the next run.

## Findings, not spec edits

`SPEC.md` is the source of truth and **must not be modified by this job**. If you find a
contradiction, an impossibility, or a detail the spec left undefined that you had to decide,
record it in `docs/reports/phase0-findings.md`: what the spec says, what you did instead or
what you had to invent, and the consequence. A short, honest findings file is a deliverable,
not a failure.

## Non-goals for this phase

Do not implement, even partially: the Claude/Pi/Codex adapters (fake-agent fixtures only);
`deck _hook` or any status detection, probes or badges; permission profiles; the environment
editor; kill/delete undo, tombstones or archiving; launch leases; notifications or any
channel; cross-session search; the preview pane; scrollback capture, history files or cwd
tracking; the health view; systemd units; session send. They have their own phases.

Do not add a user-facing command line (`SPEC.md` R7): the binary's only user-facing mode is
the TUI. Hidden internal verbs are permitted only where a later phase's design requires
them, and none is required by this phase.

## Constraints

- **Do not run `git commit`, `git push`, or any other git write command.** Leave every
  change in the working tree for operator review.
- Do not modify `SPEC.md`, anything under `prds/`, `ci/Dockerfile`, or `ci/SPIKE.md`.
  `ci/run.sh` may be extended if genuinely needed; report it if so.
- Do all building and testing in siblings via `ci/run.sh`. Install nothing into the job
  container.
- No network dependency in the default test suite: no real agent binaries, no model calls,
  no fetching at test time. Dependency downloads at build time are fine.
- Every sibling container is `--rm`. Leave no stray containers, images beyond
  `deck-ci:local`, or volumes beyond the shared cache.
- Prefer a small, readable implementation over a complete one. This phase is judged by
  whether the harness proves things, not by feature count.
