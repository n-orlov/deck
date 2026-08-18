# Phase 1 — sessions and lifecycle

## Goal

Make a session a *managed* thing rather than a row that happens to have a tmux session
behind it. Phase 0 proved deck can create, list, attach to and kill a shell session. Phase 1
delivers the lifecycle around that: the full create modal, per-session environment with
explicit restart-to-apply, kill/delete/archive with undo, launch leases so two TUIs cannot
double-start one session, and the clean-vs-crash exit split with a captured crash tail.

This is the first phase that ships **destructive** operations — kill, delete, tombstone,
purge, archive. The single most important property in it is the one requirement that is not
about a feature at all: **deck never writes to or deletes anything inside a session's
working directory** (`SPEC.md` §9.2, R1). Prove it adversarially, not incidentally.

`SPEC.md` is the authoritative product spec and **must not be modified**. Where this PRD and
`SPEC.md` disagree, `SPEC.md` wins and the disagreement is a finding (see *Findings*).

## Context

### Where work happens

The job container has no Go and no tmux and cannot install them. All building and testing
happens in the sibling toolchain container:

```sh
ci/run.sh go build ./...
ci/run.sh go test ./...
```

### What Phase 0 already gives you

Read `docs/reports/phase0.md` — especially its **Gotchas** section — before writing code.
Do not rediscover any of it. In particular the harness already provides: a pty driver that
answers Bubble Tea's OSC 11/CPR probes and reads a vt10x cell grid; per-scenario isolated
`DECK_HOME` and tmux socket with teardown that fails the scenario on leaks; multi-client
scenarios; direct tmux assertions (session exists, pane command, server options); store, log
and file-permission assertions; a step that kills the tmux server as the in-suite stand-in
for a reboot; and a faithful fake Claude fixture (`cmd/fake-claude`) with a controllable exit
status.

Existing feature files are `walking_skeleton`, `determinism`, `store`, `tmux_contract`,
`concurrency`, `fake_agent`, `harness` and the tag-excluded `fake_agent_drift`. Extend the
step library rather than duplicating steps.

### Agents are still out of scope

Phase 2 owns the Claude/Pi/Codex adapters. Phase 1 therefore proves lifecycle using **`shell`
sessions** and, where a controllable exit status is needed, the existing fake fixture. Where
`SPEC.md` describes agent-specific behaviour (relaunching with *resume* argv, conversation
ids, permission profiles), implement the shell-session half now and record the agent half as
explicitly deferred. Do not stub an adapter.

## Working practice: commit as you go

**This phase commits and pushes its own work.** The commit log is the job's durable memory,
alongside the handoff notes — a later iteration, a later phase, or the operator should be
able to read `git log` and understand what was done and why.

1. **Commit after each completed task**, not once at the end. One task, one commit, unless a
   task genuinely produces two unrelated changes.
2. Message shape: a concise imperative subject line, then a body that says **why** and notes
   anything surprising. Reference the requirement numbers the commit satisfies. Do not
   restate the diff — `git log` is memory, not a changelog.
3. **Push to `origin main` after each commit.** Credentials are mounted: `~/.gitconfig` and
   `~/.git-credentials` are already placed, so `git push origin main` works without any token
   handling from you. Never put a token in a URL, a file, or a commit message.
4. **Never** `git push --force`, rebase, amend, reset or otherwise rewrite published history.
   If you commit something wrong, fix it forward in a new commit.
5. **Never commit:** build output (`bin/` is gitignored — keep it that way), the sibling
   container's caches, secrets or tokens of any kind, editor debris, or anything under
   `SPEC.md`, `prds/`, `ci/Dockerfile`, `ci/SPIKE.md`.
6. Before each commit, run `ci/run.sh go build ./... && ci/run.sh go vet ./...` and the test
   suite. **Do not commit a red tree.** If you must commit work in progress to record a
   finding, say so explicitly in the message.
7. `git status` must be clean at the end of the phase apart from deliberately ignored paths.

## Requirements

Each requirement must be individually verifiable by a command or scenario whose real output
is recorded in the phase report (R24).

### The create modal

1. `n` opens a create modal covering, per `SPEC.md` §11: **name**, **cwd**, **agent**
   (`shell` only in this phase, but the field exists and validates), **launch_args** (extra
   args as a JSON array), the **env map**, **pre_launch** (one shell line), and
   **login_shell**. Every field is reachable and editable by keyboard alone, and the modal
   states what each field does — there is no CLI, so the UI is the only documentation the
   user gets.
2. Validation is stated, never silent: a duplicate name or slug collision, a non-existent or
   non-directory `cwd`, a malformed env key, and malformed `launch_args` each produce a
   specific in-modal message naming the problem, and the modal retains what the user typed.
   `esc` abandons the modal and creates nothing.
3. On create, `captured_path` records the `PATH` in effect at create time (`SPEC.md` §6.3),
   stored on the row. `login_shell = 1` runs the pane command through `$SHELL -lc`.
4. `pre_launch`, when set, runs in the pane **before** the session's own command, in the same
   pane, and its failure is visible rather than swallowed.
5. The launch audit (Phase 0 R8) records the exact argv and the **names** of every applied
   environment variable — never a value — for every create, resume and restart.

### Environment

6. `e` opens an env editor showing, per key, the **effective value and the layer that won**,
   with the resolution order of `SPEC.md` §6.1/§6.3: server env → `captured_path` → config
   `[env]` → session `env`. A key set in more than one layer displays the winner and is
   assertable as such.
7. Editing env while a session runs writes the session `env` map, sets `env_dirty = 1`,
   mirrors the change with `tmux set-environment -t`, and shows an **`env↻`** badge meaning
   *changed, not yet applied*. **Nothing is applied silently** (`SPEC.md` §6.2).
8. `R` restarts the session's pane and relaunches it with the new environment, clearing
   `env_dirty`. For `shell` sessions `R` additionally offers **"inject instead"**, which
   exports the changed keys into the live shell without restarting. Both paths are separately
   provable.
9. Values whose key matches `*TOKEN*|*SECRET*|*KEY*|*PASSWORD*|*CREDENTIAL*` are **masked in
   every view**, with reveal as an explicit per-view toggle (`SPEC.md` §6.4). Env values never
   appear in `events`, in the JSONL log, or in any notification payload — assert this by
   reading the files, not by inspecting code.

### Kill, delete, archive, undo

10. `x` kills the selected session: `tmux kill-session`, row → `stopped`, conversation
    untouched and labelled resumable, and a **10 s undo toast** where `u` resumes it
    (`SPEC.md` §9.2). No confirmation prompt.
11. `dd` deletes: kill plus tombstone (`deleted_at` set), hidden from the list immediately,
    undoable for **60 s**, purged after the grace period. A purged row is gone from
    `sessions`; its `events` rows go with it (`ON DELETE CASCADE`).
12. The delete confirm — and **only** there — offers **purge conversation** as a separate
    explicit choice. It is never implicit and never the default. In this phase it has no
    agent transcript to remove, so it must be present, disabled or clearly inert, and honest
    about why; record the decision as a finding.
13. `A` archives: the record is kept and hidden from the default list, reachable behind a
    filter. Archiving **requires `stopped`**, and the UI offers "kill and archive" as one
    action (`SPEC.md` §4 invariant). `archived_at` and `deleted_at` are flags, not statuses —
    an archived row keeps whatever `status` it had.
14. `m` marks rows; `x` and `dd` then act on the whole mark set, with one undo covering the
    batch.
15. **The working directory is sacred.** For every destructive path above — kill, delete,
    purge, archive, bulk, and undo of each — a scenario asserts that the session's `cwd` is
    byte-for-byte unchanged: same file list, same contents, same mtimes, and no new or
    removed entries. Seed the directory with files (including one named like a deck artifact,
    e.g. `state.db`, and one dotfile) so a naive cleanup would be caught. This is R1 and it
    is the requirement in this phase least acceptable to get wrong.

### Launch leases

16. Two clients pressing `r` on the same `stopped` session must not double-launch
    (`SPEC.md` §9.3). The transaction that flips `stopped → starting` CAS-acquires
    `launch_lease_owner` (`pid@boot_id`) and `launch_lease_until` (TTL ~30 s). The losing
    client shows **"starting elsewhere"** rather than an error.
17. A **stale** lease is breakable: a lease whose owner pid is dead, or whose TTL has
    expired, can be taken over by another client. A lease held by a live owner within TTL
    cannot. Both directions are proved, and neither leaves the row wedged.
18. A `@multiclient` scenario proves exactly one tmux session is created when N clients race
    `r` on one row — assert the tmux fact, not just the UI text.

### Clean exit versus crash

19. The reconcile loop distinguishes clean exit from crash using `pane_dead` /
    `pane_dead_status` under `remain-on-exit failed`, per the table in `SPEC.md` §7:
    session gone → `stopped`; session present with a dead pane and non-zero status → `error`
    with `pane_exit_status` recorded; session present with a live pane → status unchanged.
20. A shell session where the user types `exit` (status 0) becomes **`stopped`, never
    `error`** — this is the concrete bug the `remain-on-exit failed` choice exists to prevent.
21. A non-zero exit records `pane_exit_status` and captures a **crash tail** into
    `crash_tail` *before* the dead session is torn down, and the row shows `error`. Prove the
    tail contains pane output produced before death, and that a crash tail is captured for a
    session that dies with **no TUI attached to it** (the next reconcile tick does it).
22. **Never auto-relaunch** (`SPEC.md` §7, a non-goal): after a crash the session stays in
    `error` until the user acts. A scenario asserts no new tmux session appears on its own.
23. `killed_by_user` outranks automation: a user kill sets it, and it is not undone by a
    later reconcile or status update arriving afterwards (`SPEC.md` §7 precedence).

### Evidence and stability

24. `docs/reports/phase1.md` records, with **real unedited command output**: the full test
    run with feature/scenario/step counts and a top-level Go test count under a stated
    counting convention; one recorded run or named scenario per numbered requirement above;
    resolved tool versions; the wall-clock duration of a full suite run; and every gotcha
    discovered, each with its consequence if forgotten. Every capture it cites must be a
    **repository-relative path that exists** — never a path inside the job's run directory.
25. **The suite passes ten consecutive times from a clean state**, and the real output of the
    loop proving it is recorded. Two passes are not evidence of stability: Phase 0's
    "passes twice" bar was met while a scenario failed one run in three. If any run in the
    ten fails, fix it and restart the count.
26. No scenario may be deleted, skipped, or tag-excluded to make the suite pass. If a
    scenario is wrong, fix the scenario and say so in the report.

## Review guidance

The reviewing pass exists to catch **defects in the product**, and Phase 0 showed exactly how
much that is worth: it found a reconcile loop with no production caller and a determinism
feature passing vacuously against dead code, both behind a fully green suite.

- **Block on real defects.** Dead code satisfying a requirement on paper; a test that passes
  without exercising the behaviour it names; a requirement asserted only by a unit test when
  it claims black-box coverage; a flaky scenario; a false claim about what the code does; any
  weakening of a test to make it pass; anything that touches a session's `cwd`.
- **Do not block on minor documentation issues.** Wording, formatting, a stale sentence in a
  derived summary, a count convention that is stated but inelegant, an imperfect table — note
  these in the findings and **pass the phase**. Nit-picking prose while a correct product sits
  finished wastes approaches that exist to catch real bugs. Phase 0b was rejected three times
  over three sentences; do not repeat that.

The distinction is whether a reader would be *misled about behaviour*. "The report says the
suite covers R18 and it does not" is a real defect. "The report's package list is missing a
line" is a note.

## Findings, not spec edits

`SPEC.md` must not be modified. Record in `docs/reports/phase1-findings.md`: anything the
spec left undefined that you had to decide, anything you found contradictory or impossible,
and every deferral (with what is deferred and to which phase). If you need a new `DECK_*`
determinism control to make a timed behaviour testable — the 10 s undo window and 60 s delete
grace are the obvious candidates — introduce it, document it in the help overlay, and record
it as a finding so the operator can fold it into `SPEC.md`. Do not silently invent
configuration.

Durations must use a **monotonic** source and keep advancing while `DECK_CLOCK` is frozen
(Phase 0 R7). An undo window that stops ticking because the wall clock is pinned is a bug.

## Non-goals for this phase

Do not implement, even partially: the Claude/Pi/Codex adapters or any conversation-id
handling; `deck _hook`, hook receipt, status probes or live/sampled badges; permission
profiles; notifications or any channel; cross-session search; the preview pane; scrollback
capture, history files or cwd tracking on resume (`SPEC.md` §9.4 — Phase 6); the health view;
systemd units; session send (§11.1); attention sort, grouping or filtering beyond what
requirement 13 needs to show archived rows.

Do not add a user-facing command line (`SPEC.md` R7). The TUI remains the only user-facing
surface.

## Constraints

- Commit and push as described above. Do not rewrite history.
- Do not modify `SPEC.md`, anything under `prds/`, `ci/Dockerfile`, or `ci/SPIKE.md`.
- Do all building and testing in siblings via `ci/run.sh`. Install nothing into the job
  container.
- No network dependency in the default test suite: no real agent binaries, no model calls.
- Every sibling container is `--rm`. Leave no stray containers, images beyond
  `deck-ci:local`, or volumes beyond the shared cache.
- Prefer a small, readable implementation over a complete one. This phase is judged by
  whether the lifecycle is provably correct and non-destructive, not by feature count.
