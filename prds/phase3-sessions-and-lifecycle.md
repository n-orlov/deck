# Phase 3 — sessions and lifecycle

## Goal

Make a session a *managed* thing rather than a row that happens to have a tmux session
behind it. Phase 0 gave create/list/attach/kill; Phase 1 gave agents, durable identity,
permission profiles and launch leases; Phase 2 made status truthful; Phase 2b-1 made the
list visible and navigable; Phase 2b-2 made it configurable, themed, and gave every dialog
one contract. Phase 3 delivers the lifecycle around all of it: the create modal completed
**including §11.7 path entry**, per-session environment editing with explicit
restart-to-apply, kill/delete/reap/purge/archive with undo and bulk marks, rename, the
event log, and the list filter.

This is the first phase that ships **destructive** operations. The single most important
property in it is the one requirement that is not about a feature at all.

## The requirement everything else serves

**deck never writes to or deletes anything inside a session's working directory**
(`SPEC.md` §9.2, hard requirement R1). Every destructive path in this phase — kill, delete,
reap, purge, archive, bulk, and the undo of each — must be proven not to touch it, and
proven **adversarially**: a directory seeded to look exactly like something a naive cleanup
would sweep. A phase that ships a working `dd` and cannot prove this has failed, whatever
else is green.

The second-order version of the same rule: **deleting a deck row is not deleting the user's
conversation.** `SPEC.md:684-691` is explicit — Claude's and Pi's transcripts belong to
those tools, `dd` forgets deck's row while the conversation stays resumable by the agent's
own CLI, and the *only* place deck removes a file it did not create is the explicit **purge
conversation** choice in the delete confirm.

`SPEC.md` is the authoritative product spec and **must not be modified**. Where this PRD and
`SPEC.md` disagree, `SPEC.md` wins and the disagreement is a finding (see *Findings*).

## Context

### Where work happens

The job container has no Go and no tmux and cannot install them. All building and testing
happens in the sibling toolchain container:

```sh
ci/run.sh go build ./...
ci/run.sh go vet ./...
ci/run.sh go test ./...
ci/stability.sh 10
```

### What already exists — do not rebuild it

Read `docs/reports/phase0.md`, `phase1.md`, `phase2.md`, `phase2b1.md` and `phase2b2.md` —
especially their **Gotchas** sections — before writing code. Do not rediscover any of it.

- **The harness** (§13.2): a pty driver that answers Bubble Tea's OSC 11/CPR probes and
  reads a cell grid; per-scenario isolated `DECK_HOME` and tmux socket with teardown that
  fails on leaks; multi-client scenarios; direct tmux assertions; store, log and
  file-permission assertions; a tmux-server-kill step as the in-suite reboot stand-in;
  pty resize mid-scenario; SGR (1006) mouse-report synthesis; **per-cell SGR attribute
  assertions**; fake `claude` and `pi` fixtures that fire §8.1 events at `deck _hook` from
  inside their own pane, render named fixtures verbatim, record every terminal size they
  observe, and carry a controllable exit status.
- **`DECK_*` controls** (§13.1) including `DECK_CLOCK`, `DECK_CLOCK_STEP`, `DECK_ASCII`,
  `DECK_MOUSE`, `DECK_COLOR_DEPTH`. Extend this set when a timed behaviour needs it (see
  requirements 1–2); never add a knob that changes anything but determinism.
- **The create modal exists** with every field this phase needs already present:
  `internal/tui`'s `createName`, `createCWD`, `createAgent`, `createProfile`,
  `createLaunchArgs`, `createEnv`, `createPreLaunch`, `createLoginShell`, plus §5's `y`
  yolo confirm. Phase 3 **completes** it — §11.7 path entry, the blank-name default,
  stated validation — it does not rewrite it.
- **`x` exists and kills** (`internal/service/kill.go`), with no confirm, no undo toast, and
  a refusal when the row is already `stopped`. Phase 3 adds the undo toast; the refusal and
  the no-confirm rule stay.
- **The dialog contract** (§11.4) is implemented and asserted per dialog in
  `features/dialogs.feature`. Every dialog this phase adds inherits it rather than
  restating it: `esc` cancels and changes nothing, `↵` submits, `tab`/`shift+tab` move
  between fields, `←`/`→`/`space` change a selection, validation is in-dialog and retains
  what was typed, destructive actions confirm and name what survives, the mouse can neither
  cancel nor confirm, width is 80% clamped to `[26, 80]`.
- **The settings takeover** (§11.5) is generated from the config schema. A new flat key is
  declared once in the schema and appears in settings automatically; a key that exists in
  one and not the other is a defect the parity test already catches.
- **The theme token set** (§11.6). Every colour comes from a token. Any view this phase adds
  uses tokens; a hex literal in render code is a regression against 2b-2's requirement 33.
- **`internal/tui/group.go`'s `visualOrder()`** is the single reconciliation point between
  painted row order and `m.sessions` index order. Every navigation primitive already routes
  through it (2b-2 task 056, operator-reported). Anything this phase adds that moves the
  selection — the filter, the mark set, a row disappearing on `dd` — must route through it
  too, and must not reintroduce index arithmetic against visual geometry.
- **`internal/notify/doc.go` and `internal/search/doc.go`** are placeholders for Phases 5
  and 7. Leave them alone.

### Working practice: commit as you go

**This phase commits and pushes its own work.** The commit log is the job's durable memory
alongside its handoff notes — a later iteration, a later phase, or the operator should be
able to read `git log` and understand what was done and why.

1. **Commit after each completed task**, not once at the end. One task, one commit, unless a
   task genuinely produces two unrelated changes. **If review requires a correction to
   something already committed, add a follow-up commit — never amend.**
2. Message shape: a concise imperative subject line, then a body that says **why** and notes
   anything surprising. Reference the requirement numbers the commit satisfies. Do not
   restate the diff — `git log` is memory, not a changelog.
3. **Push to `origin main` after each commit.** Credentials are mounted: `~/.gitconfig` and
   `~/.git-credentials` are already placed, so `git push origin main` works without any token
   handling from you. Never put a token in a URL, a file, or a commit message. If a push is
   rejected because the remote moved, **fetch and rebase your own unpushed commits only** —
   never force, and never rewrite anything already on the remote.
4. The mounted identity is **`deck job (ralphd)`**, deliberately distinct from the operator's
   own, so a reviewer diffing a range can tell whose commit is whose. Do not override
   `user.name`/`user.email`; the last phase to share one identity with the operator had its
   reviewer attribute the operator's edits to the job as a protected-path violation.
5. **Never** `git push --force`, rebase published history, amend, reset or otherwise rewrite
   what has been pushed. Fix forward, always.
6. **Never commit:** build output (`bin/` is gitignored — keep it that way), the sibling
   container's caches, secrets or tokens of any kind, editor debris, or any change to
   `SPEC.md`, `prds/`, `ci/Dockerfile`, `ci/SPIKE.md`.
7. Before each commit run `ci/run.sh go build ./... && ci/run.sh go vet ./...` and the test
   suite. **Do not commit a red tree.** If you must commit work in progress to record a
   finding, say so explicitly in the message.
8. `git status` must be clean at the end of the phase apart from deliberately ignored paths,
   and every commit is pushed — a phase that ends with unpushed commits has not delivered.

### Assertions this phase must deliberately change

Enumerated so a red run is never "fixed" on the wrong side. Each of these is a **required
update**, not a regression:

- **`x` gains an undo toast.** Any existing scenario asserting that a kill produces no
  transient message, or asserting the exact frame after a kill, is re-aimed at the toast.
  The toast is rendered outside the panels, so it is subject to 2b-2's requirement 37 frame
  budget — see requirement 30.
- **`g` is rebound.** 2b-1 bound `g` to *toggle the selected row's workspace group*, because
  §11.8's collapsible headers had no key and §11's keymap assigns `g`/`G` to top/bottom.
  `SPEC.md:937` is the authority: `g`/`G` are top/bottom. Requirement 34 reconciles this,
  and `features/attention_sort.feature`'s collapse scenarios move to the new key.
- **Rows can now be hidden by a filter and by a tombstone**, so any scenario that asserts a
  row count or a full row list from a store fixture must state which filter is in force.
- **Workspace grouping becomes conditional** (requirement 31). Scenarios that assume group
  headers exist must pin `[ui] group_by_workspace = true` explicitly rather than relying on
  the default. Do not delete the flat-list case to keep the grouped assertions simple.
- **`recent_cwds` is a new table**, so `features/store.feature`'s schema assertions and the
  migration scenario gain a row. Migration from the current schema version must be proven,
  not assumed: `SPEC.md:287` — the store is never rebuilt and a session row is never
  recreated to gain a field.

## Requirements

Each requirement must be individually verifiable by a command or scenario whose real output
is recorded in the phase report (requirement 45).

### Harness prerequisites (build these first)

1. **`DECK_UNDO_MS` and `DECK_DELETE_GRACE_MS`** (§13.1's pattern, new knobs): override the
   10 s undo window and the 60 s delete grace so both the "undo works" and the "window
   expired" sides are assertable in a fast suite. Both default to the spec's values, both
   are documented in the help overlay, and both are recorded as findings for the operator
   to fold into `SPEC.md`. **Neither may change any behaviour other than the duration** —
   a knob that also skips the reap is a test-only branch in product code, which §13 bans.
2. **Both windows use a monotonic source and keep advancing while `DECK_CLOCK` is frozen**
   (Phase 0 R7). An undo window that stops ticking because the wall clock is pinned is a
   bug, and it is the specific bug this requirement exists to prevent. Prove it: freeze
   `DECK_CLOCK`, let the window expire, assert it expired.
3. **A working-directory fingerprint step.** Snapshot a directory — the full recursive entry
   list, each file's contents, mode and mtime — and assert later that it is **byte-for-byte
   and stat-for-stat identical**. This is the instrument requirement 27 is measured with, so
   it must itself be proven able to fail: a companion Go test mutates a seeded directory in
   each of four ways (content change, mtime touch, new file, removed file) and asserts the
   step reports each one.
4. **A transcript fixture with provenance.** Purge (requirement 22) deletes a real file, so
   the suite needs an agent transcript to delete. The fake `claude` and `pi` fixtures write
   a transcript at the path their **real** counterparts use, and the path convention is
   traceable to a recorded observation of the real CLI — not invented. Phase 2b-2's
   requirement 38 exists because pi probe fixtures were invented once already; do not repeat
   it. Record the capture in the report.

### The create modal, completed (§11, §11.4)

5. **Every field is reachable and editable by keyboard alone, and the modal states what each
   field does.** There is no CLI (`SPEC.md` R7), so the modal is the only documentation the
   user gets for `pre_launch`, `login_shell` and `launch_args`. A field with no description
   is an R7 defect.
6. **The blank-name default.** An empty name yields `<workspace>-<MMDD-HHMM>`, with a
   collision suffix when that name is taken. Assert both the plain and the colliding case
   under a frozen `DECK_CLOCK` so the generated name is deterministic.
7. **Validation is stated, never silent, and retains what was typed** (§11.4): a duplicate
   name, a slug collision, a non-existent `cwd`, a path that exists but is not a directory,
   a malformed env key, and malformed `launch_args` each produce a specific in-modal message
   naming the problem. `esc` abandons the modal and creates nothing — proven by store state,
   not by the absence of a frame.
8. **`captured_path` records the `PATH` in effect at create time** (§6.3) and is stored on
   the row. `login_shell = 1` runs the pane command through `$SHELL -lc`, and the row then
   marks `captured_path` **advisory** — §6.3 makes these two mutually exclusive by design,
   and the health view (Phase 7) reads that marking.
9. **`pre_launch`, when set, runs in the pane before the session's own command**, in the same
   pane, and **its failure is visible rather than swallowed**. A `pre_launch` that exits
   non-zero must leave evidence a user can find without attaching.
10. **The launch audit records the exact argv and the names of every applied environment
    variable — never a value** — for every create, resume and restart (Phase 0 R8, §6.4).
    Assert by reading the audit file, not by inspecting code.

### Path entry and recent working directories (§11.7)

`SPEC.md:1196-1236`. All three mechanisms share one text-input behaviour, so the create
modal's `cwd` and any later path field behave identically.

11. **The `recent_cwds` table** (`SPEC.md:268-271`): resolved absolute path as primary key,
    `used_seq` **monotonic and not a timestamp**, so the order stays assertable under a
    frozen `DECK_CLOCK`. Creating a session promotes its `cwd` to the front, deduplicated by
    resolved absolute path, evicting the oldest beyond `[ui] recent_cwd_limit` (default 5).
12. **The `cwd` field is pre-filled with the most recent entry**, labelled as the last used,
    and **typing replaces it wholesale** — it is offered, not committed. With no history it
    pre-fills the directory deck itself was started in. Nothing is silently assumed on the
    user's behalf.
13. **`↑`/`↓` cycle the recent list**, shell-history style, showing `recent 2/5`. This is a
    declared per-field key set under §11.4's contract, so the modal states it inline where it
    applies.
14. **Ghost completion**: with the cursor at the end of the field, the completion is shown
    inline in the theme's `dimmed` token and `→` (or `end`) accepts it. **Directories only.**
    The segment being completed is the text after the last `/`; hidden directories are
    candidates only when that segment starts with `.`; a leading `~` expands; a single match
    completes to it plus a trailing `/`.
15. **Ambiguity ghosts nothing.** Several matches with no further common prefix shows the
    count (`3 matches — tab to list`) and ghosts **nothing**. It must never ghost the
    alphabetically-first candidate: `→` would become a coin flip that silently puts the
    session in the wrong directory, and a wrong `cwd` is not a typo the user notices.
    **A scenario must assert the negative** — that with three matching directories the field
    contains no ghost text at all.
16. **`tab` is bash's contract**: complete to the longest common prefix when that advances,
    otherwise list the candidates for selection.
17. **The list is history and paths can be sensitive.** Settings (§11.5) offers clearing it,
    it never enters a notification payload, and `recent_cwds` remains non-load-bearing:
    dropping the table costs a prefill and never a session (`SPEC.md:283-285`).

### Environment (§6)

18. **`e` opens an env editor showing, per key, the effective value and the layer that won**,
    in §6.1/§6.3's order: server env → `captured_path` → config `[env]` → session `env`. A
    key set in more than one layer displays the winner **and is assertable as such** — the
    winning layer is on screen, not merely computed.
19. **Editing env while a session runs** writes the session `env` map, sets `env_dirty = 1`,
    mirrors the change with `tmux set-environment -t`, and shows the **`env↻`** badge meaning
    *changed, not yet applied*. **Nothing is applied silently** (§6.2).
20. **`R` restarts the pane and relaunches with the resume argv** — new environment, same
    conversation (§6.2) — clearing `env_dirty`. For `shell` sessions `R` additionally offers
    **"inject instead"**, exporting the changed keys into the live shell without restarting.
    Both paths are separately provable, and a scenario asserts the agent path resumes rather
    than starting a fresh conversation.
21. **Secret-shaped values are masked in every view** (§6.4): keys matching
    `*TOKEN*|*SECRET*|*KEY*|*PASSWORD*|*CREDENTIAL*`, with reveal as an explicit per-view
    toggle. Env values never appear in `events`, in the JSONL log, or in any notification
    payload — **assert this by reading the files**, not by inspecting code. Help recommends
    `pre_launch` over storing a token in `env` (§6.4's last bullet).

### Kill, delete, archive, undo (§9.2)

22. **`x` kills the selected session**: `tmux kill-session`, row → `stopped`, conversation
    untouched and labelled resumable, and a **10 s undo toast** where `u` resumes it. No
    confirmation prompt — teardown is cheap because R1 means no session owns anything on
    disk, and that is what earns the no-confirm UI.
23. **`dd` deletes**: kill plus tombstone (`deleted_at` set), hidden from the list
    immediately, undoable for **60 s**, **reaped** after the grace period. `dd` is a two-key
    sequence: a pending `d` is visible, `esc` cancels it, and `d` followed by any other key
    does nothing destructive. Assert the pending state — a `dd` that fires on the first `d`
    under load is the worst possible defect in this phase.
24. **Reaping leaves no trace** (`SPEC.md:674-682`). It deletes the `sessions` row and every
    deck-owned row hanging off it — `events` (via `ON DELETE CASCADE`), outbox entries, the
    `waiting`/`notify_epoch` state — **together with deck's own per-session files**: §9.4's
    history file and `$DECK_HOME/captures/<session_id>/`. Those are Phase 6's to *write*;
    the path convention and their removal are this phase's, because reaping is where they
    stop existing. Assert the directory is gone when it exists and that a missing one is not
    an error. Afterwards nothing remains to list, to search or to resume. **The one
    deliberate exception is the JSONL log**, which keeps its append-only record — a log that
    rewrites itself when a row is deleted is not a log.
25. **A delete without purge leaves the agent's transcript intact** (`SPEC.md:684-691`).
    This needs its own scenario reading the transcript file after `dd` and its reap.
26. **Purge conversation is offered in the delete confirm and nowhere else**, never implicit,
    never the default, and it **names the exact path it will delete before doing it**. This
    requires the adapter interface to declare where a transcript lives — add
    `TranscriptPaths` to `internal/agent`'s `Adapter`/`Caps` as a *declared capability*
    (Phase 7's search reads the same declaration, §12). An adapter that cannot locate its
    transcript **declines purge and says so**; it never guesses, and it never deletes a
    directory it inferred.
27. **`A` archives**: the record is kept and hidden from the default list, reachable behind
    the filter of requirement 33. Archiving **requires `stopped`**, and the UI offers "kill
    and archive" as one action (§4's invariant). `archived_at` and `deleted_at` are **flags,
    not statuses** — an archived row keeps whatever `status` it had, and the `▣` glyph is a
    rendering of the flag.
28. **`m` marks rows**; `x` and `dd` then act on the whole mark set, with **one undo covering
    the batch**. A mark set survives a re-sort and a re-group (it is keyed by session id, not
    by row), and clears on the action or on `esc`.
29. **The working directory is sacred.** For every destructive path above — kill, delete,
    reap, purge, archive, bulk, and the undo of each — a scenario asserts with requirement
    3's fingerprint that the session's `cwd` is unchanged: same entry list, same contents,
    same modes, same mtimes. Seed it with files a naive cleanup would catch: one named like
    a deck artifact (`state.db`), one dotfile, one directory, one file with no write
    permission. **This is R1 and it is the requirement in this phase least acceptable to get
    wrong.**
30. **Every transient message is inside the frame budget.** The undo toast, the pending-`d`
    indicator and the batch-undo message render outside the panels, which is exactly the
    hazard 2b-2's requirement 37 fixed for the startup banner: a message drawn outside the
    panel rectangles must be counted in `computeLayout`'s reserved rows, or the frame grows
    past its budget and the golden 80×24 frame shears. Assert the frame budget with the
    toast on screen, at the 80×24 minimum, in every layout mode.

### Rename, the event log, and the list filter

31. **Rename is an action inside the `i` detail dialog** (`SPEC.md:934`), **not** a top-level
    key. It re-validates uniqueness for both `name` and `slug`, and it does **not** rename
    the tmux session: `deck_<slug>` is the tmux identity and §3.2 pins it, so a rename that
    moved the tmux session would break every live pane's identity. State this in the dialog —
    the user is entitled to know the display name and the tmux name have diverged — and
    record the decision as a finding if `SPEC.md` leaves it open.
32. **`E` opens the event log** for the selected session: the `events` rows, newest first,
    with kind, reason and bounded payload, scrollable, `esc` to close. Env values never
    appear here (requirement 21). This is the view that makes the append-only audit trail
    reachable, which is what §11's capability list requires of it.
33. **`/` filters the list** by name, workspace and cwd, incrementally, with `esc` clearing.
    The filter is also **how archived rows are reached** (requirement 27) — a stated filter
    term for archived, not a separate mode — and the sidebar states the filter is in force so
    a hidden row is never mistaken for a deleted one. Filtering moves the selection through
    `visualOrder()`, never by index arithmetic.

### Workspace grouping becomes optional (operator request, 21 Aug 2026)

The operator asked for this after using the 2b-1 build: *"I am not sure I want workdir
grouping, make it optional."* `SPEC.md:875`/`:392` have been amended by the operator to make
it conditional; this phase implements the amendment. Read those lines as authoritative — if
they do not read as described here, **stop and record a finding** rather than implementing
this section from the PRD alone.

34. **`[ui] group_by_workspace`** (default `true`, preserving today's behaviour) is declared
    once in the config schema, so it appears in the settings takeover automatically
    (2b-2's requirement 22 parity test proves it did).
35. **With grouping off the sidebar renders a flat list** in attention-sort order with **no
    header rows**, which changes the row budget §11.2's page-size and elision maths read.
    Assert the page size and the elision boundary in both modes, and assert the golden 80×24
    frame in both. A flat list has no group to collapse, so `z`-style collapse state is
    simply absent rather than inert.
36. **Navigation is identical in both modes.** With grouping off, visual order equals index
    order and the class of defect the operator hit in 2b-1 is invisible — which is precisely
    why the grouped case keeps a regression test whose workspaces are **non-adjacent in
    `m.sessions`**. Do not let "turn grouping off" become the workaround for a navigation
    bug: `visualOrder()` stays the single reconciliation point in both modes.

### Keymap reconciliation and R7 completeness

37. **`g`/`G` are top/bottom** (`SPEC.md:937`). 2b-1 bound `g` to group-collapse because
    §11.8's collapsible headers had no key; that binding moves. Choose a key that collides
    with nothing in §11's keymap, state it in the help overlay and in the group header
    itself, and **record the choice as a finding** so the operator can fold it into
    `SPEC.md`. The mouse click on a header (§11.8) keeps working and keeps calling the same
    helper, so neither path is ever the only way to reach the capability.
38. **Every key this phase binds has a scenario, and every capability it adds has a key or a
    documented entry point** (§11's closing rule, §13.5). Produce the cross-check as a
    **test**, not a paragraph: enumerate the keymap in the help overlay, enumerate the bound
    keys, and fail on a difference in either direction. A key in the help overlay with no
    binding is a §11.3 defect; a capability with no entry point is an R7 defect.
39. **The help overlay lists the new keys** — `dd`, `e`, `R`, `A`, `m`, `u`, `E`, `/`, `G`,
    the rebound collapse key, and the new `DECK_*` knobs — and stays inside the frame budget
    at 80×24 with the longest keymap it will ever render in this phase.

### Store durability

40. **State the durability contract and prove it.** `internal/store/store.go:100` opens the
    database `journal_mode=WAL, synchronous=NORMAL`. In WAL mode `NORMAL` means a power cut
    can lose the last transactions — acceptable for a layout preference, **not** obviously
    acceptable for the write that records a conversation id, which is R3's whole promise, or
    for a tombstone. Decide explicitly: either keep `NORMAL` and prove which writes are
    allowed to be lost (with the reasoning in the findings), or raise the durability of the
    identity-carrying and lifecycle-carrying writes. Whichever is chosen, the reboot stand-in
    scenario must prove a conversation id written immediately before the server dies is still
    there afterwards. Do not change the pragma without a scenario that would have caught the
    difference.

### Scenarios that define this phase

Feature files are named for the area of behaviour, never for the phase (§13.3).

41. **`features/create_session.feature`** — agent choice, cwd, args, env, `pre_launch`, name
    collisions, the blank-name default, and **§11.7's recent-cwd prefill, cycling, ghost
    completion and `tab`**, including the ambiguity-ghosts-nothing negative.
42. **`features/environment.feature`** — §6 layering with the winning layer on screen,
    `env↻`, restart applies **and only restart applies**, the shell inject path, masking, and
    the read-the-files assertion that no value leaked.
43. **`features/kill_delete_undo.feature`** — `x`/`dd`, both undo windows and both
    expiries, the tombstone, the reap leaving no trace, **the agent's transcript surviving
    `dd`**, purge deleting it only when explicitly chosen, archive requiring `stopped`, bulk
    marks under one undo, and the `cwd` fingerprint on every one of those paths.
44. **Existing scenarios keep passing**, including `layout_modes`, `preview`,
    `attention_sort`, `mouse`, `settings`, `themes`, `dialogs`, `status_recovery`,
    `durable_identity`, `launch_lease`, `lease_race`, `concurrency` and the golden 80×24
    frame. Where this phase changes one deliberately, it is in the list above and the change
    is stated in the commit message and the report.

### Evidence and stability

45. **`docs/reports/phase3.md`** records, with **real unedited command output**: the full
    test run with feature/scenario/step counts and a top-level Go test count under a stated
    counting convention; one recorded run or named scenario per numbered requirement above;
    resolved tool versions; the wall-clock duration of a full suite run; and every gotcha
    discovered, each with its consequence if forgotten. Every capture it cites must be a
    **repository-relative path that exists** — never a path inside the job's run directory.
46. **The suite passes ten consecutive times from a clean state** (`ci/stability.sh 10`), and
    the real output of the loop is recorded. Two passes are not evidence: Phase 0's "passes
    twice" bar was met while a scenario failed one run in three. If any run in the ten fails,
    fix it and restart the count. The ten-run evidence must be collected **after** the last
    production or test commit, and the report must name the commit it was collected at.
47. **No scenario may be deleted, skipped, or tag-excluded to make the suite pass.** If a
    scenario is wrong, fix the scenario and say so in the report.

### Scrolling inside an attached session (operator-reported, 21 Aug 2026)

These three arrived from the operator after the rest of this PRD was written, which is why
they are numbered last rather than grouped with the mouse work. Requirements 45–47 apply to
them like any other: a recorded run per requirement, inside the ten-run stability evidence,
no scenario skipped. The new scenarios live in **`features/attach_scroll.feature`**.

`SPEC.md` §3.2's server-option list and §6.5's key table are **amended by the operator before
this phase starts**, in the same commit as §11's grouping key, so both already name what these
requirements ask for. Read them first. If either amendment is missing from the tree, the
correct move is the one this PRD requires everywhere else: build nothing on the contradiction,
file it in `docs/reports/phase3-findings.md`, and leave `SPEC.md` alone.

The report, verbatim: *"mouse scroll seems to do up/down keys in tmux. can we fix it so it
actually scrolls, like in native shell."* That is a precise description of a real mechanism,
not a vague annoyance. deck's tmux server is bootstrapped with `exit-empty off`,
`remain-on-exit failed`, `window-size latest` and `aggressive-resize on`
(`internal/tmux/tmux.go:529`, SPEC §3.2) and **never sets `mouse`**, so tmux leaves it at
tmux 3.6's default `off`. With mouse reporting off, tmux never asks the outer terminal for
wheel events, so the outer terminal handles the wheel itself — and because tmux occupies the
alternate screen, virtually every terminal's *alternate-scroll* behaviour translates a wheel
notch into an `Up`/`Down` arrow key. Those arrows reach the pane's shell, which reads them as
**history recall**. Nothing scrolls; the command line changes under the user instead. This is
the phase's cheapest large win: two words of configuration, and a scenario that would have
caught it.

48. **A wheel notch in an attached session scrolls the pane, and does not type.** Set
    `mouse on` alongside the existing options in `Bootstrap`, in the same invocation, on
    deck's own socket only. Then prove the behaviour against **real tmux**, not against the
    option value: with a shell session holding more output than one screen, deliver a wheel
    event to the pane and assert (a) the pane's visible region moves up through the
    scrollback, and (b) **the shell's input line is unchanged** — no history recall arrived.
    Assertion (b) is the one that matters, because it is the defect; a test that only checks
    `show-options -g mouse` proves nothing about what a wheel does.

    Deck is unusually safe ground for `mouse on`, and the reasoning belongs in the report:
    §3.2 gives every deck session exactly **one window and one pane**, so the click-to-select
    pane, click-the-window-list and drag-to-resize behaviours that make `mouse on` contentious
    in a hand-rolled tmux config have nothing to act on here. What it does cost is stated
    honestly rather than discovered: with mouse reporting on, a drag no longer makes the
    *terminal's* own selection, so copying text with the mouse means holding **Shift** (which
    every mainstream terminal uses to bypass mouse reporting) or using tmux copy-mode. Say so
    in the help view (§11.3) next to the existing `tmux -L deck ls` escape hatch — a user who
    loses drag-select and finds no explanation will call it a regression.

    A full-screen agent on the alternate screen is unaffected either way: it either requests
    mouse tracking itself, in which case tmux forwards the wheel to it, or it relies on the
    arrows-from-wheel path it already binds to its own scrolling. Claude Code's fullscreen
    renderer is the second kind, so `mouse on` is inert for it rather than harmful. Do not
    special-case agents.

    Wrong fixes, fenced off explicitly:
    - **Do not** disable the alternate screen (`set -ga terminal-overrides ',*:smcup@:rmcup@'`)
      to hand scrolling back to the outer terminal. It is the folklore answer, it breaks
      full-redraw applications, and it loses copy-mode entirely.
    - **Do not** bind the wheel to `send-keys Up`/`Down`. That is the present defect,
      formalised.
    - **Do not** touch the user's default socket, `~/.tmux.conf`, or any global tmux state
      outside deck's own server (§3.2: "the user's interactive tmux is untouchable").
    - **Do not** change §11.8's wheel-over-the-sidebar binding or make the preview pane
      scroll. deck's own TUI already behaves correctly and the preview's no-op is a
      deliberate binding; this requirement is about the *attached* session only.
    - Keep the option list **single-sourced**. `Bootstrap` is the only place that configures
      the server today; SPEC §3.1 anticipates a `deck _serve-tmux` unit that starts the server
      "with the right server options", and two hand-maintained lists would drift the moment
      that lands.

49. **Scrolling must not make deck lie about status.** With `mouse on`, a wheel notch over a
    shell pane puts that pane into **copy-mode**, and deck's status probe reads panes with
    `capture-pane -p` (`internal/service/reconcile.go`). Establish by experiment what a
    capture returns while the pane is scrolled back, and write the answer in the report. Then
    make the scenario prove the invariant either way: **a user scrolling an attached session
    never changes that session's badge**, and a probe taken during copy-mode does not resolve
    to a status derived from scrolled-away text. If capture *is* affected by the copy-mode
    view, pin the probe to the pane's live bottom with an explicit range rather than accepting
    a status flip — and if it is not affected, say that plainly with the capture that shows
    it, because the next person will assume it is. This is the second-order defect the
    configuration change introduces, and it is exactly the kind that ships silently: the
    status is wrong only while someone is reading their own scrollback.

50. **The behaviour is configurable, and the knob layers like every other.** Add a top-level
    `tmux_mouse` key (default `true`) to the §6.5 schema — top level, next to `stale_after`
    and `capture_min_interval`, because it is a property of deck's tmux contract (§3.2) and
    not of deck's own UI; `[ui] mouse` already means something different (§11.8) and must not
    be overloaded. Because §6.5 requires every key to be overridable from the environment so
    the harness can pin it, add `DECK_TMUX_MOUSE` with it. That knob is **not** an invented
    behaviour switch of the kind this phase otherwise forbids — it is the environment layer of
    a declared config key, which §6.5 mandates; state that in the report so review can tell
    the two apart. Phase 2b-2 made the schema the single source of truth for both the parser
    and the settings view, so this key must appear in the settings takeover **without a second
    declaration**, and the parity test added there must cover it. `false` must restore today's
    behaviour exactly, which is what makes the default safe to ship.

51. **A coalesced multi-rune keypress must not silently drop every key in it.** Carried from
    Phase 2b-2's task 014, which diagnosed this properly and then deferred it here. bubbletea's
    `detectOneMsg` deliberately reports the longest run of non-control runes arriving in one
    `read(2)` as a **single** `KeyRunes` message (to support IMEs and fast input). Deck's
    navigation dispatch switches on `msg.String()` per individual key
    (`internal/tui/tui.go:684` and `:2228`), so a two-key burst arrives as one `KeyMsg` whose
    `String()` is e.g. `"iq"`, matches no `case`, and **both keys are discarded with no
    feedback** — proven in Phase 2b-2 with a `tea.WithFilter` log
    (`tea.KeyMsg{Type:-1, Runes:[]int32{105, 113}}`) and a 60/60 minimal repro. Deck already
    knows how to do this correctly elsewhere: the text-entry paths read `msg.Runes` directly
    (`internal/tui/settings.go:459`, `:933`, `tui.go:2243`), which is why typing into a field is
    unaffected. Fix the navigation path to dispatch each rune of a multi-rune `KeyMsg` in order,
    so `"iq"` behaves exactly as `"i"` then `"q"` typed slowly.
    The trigger is **not** only a test harness: a laggy ssh or tmux link that buffers keystrokes
    during a stall and flushes them together produces the same single read, and deck enables no
    bracketed-paste handling at all (`cmd/deck/main.go:89` passes only `tea.WithAltScreen()` and
    conditionally `tea.WithMouseCellMotion()`; `grep -rn "Paste" cmd/ internal/ --include=*.go`
    excluding tests is empty), so a paste takes the same dropped path. Decide and state whether
    a paste into the **list** should dispatch its runes as keys or be ignored deliberately —
    silently dropping it is the one option this requirement forbids. Prove it from a pty with
    two keystrokes written with **no** delay between them (the harness's own
    `sendClientKeys` pads 25 ms, so the test must bypass that padding deliberately, and say so),
    asserting both keys took effect; a test that pads between writes cannot fail and does not
    count. Do **not** "fix" this by making the harness slower, and do **not** change what a
    single keypress does.

## Review guidance

The reviewing pass exists to catch **defects in the product**, and Phase 0 showed exactly
how much that is worth: it found a reconcile loop with no production caller and a
determinism feature passing vacuously against dead code, both behind a fully green suite.
Phase 2b-2's approach 1 was rejected for claiming completion at a quarter built, which is
the same skill applied to a report rather than to code.

- **Block on real defects.** Anything that touches a session's `cwd`. A destructive path
  with no fingerprint assertion. Dead code satisfying a requirement on paper. A test that
  passes without exercising the behaviour it names. A requirement asserted only by a unit
  test when it claims black-box coverage. A flaky scenario. A false claim about what the
  code does. Any weakening of a test to make it pass. A `DECK_*` knob that changes behaviour
  beyond a duration. A ghost completion that guesses. A purge that deletes a path the
  adapter inferred rather than declared.
- **Do not block on minor documentation issues.** Wording, formatting, a stale sentence in a
  derived summary, a count convention that is stated but inelegant, an imperfect table —
  note these in the findings and **pass the phase**. Nit-picking prose while a correct
  product sits finished wastes approaches that exist to catch real bugs. Phase 0b was
  rejected three times over three sentences with a finished product in the tree.

The distinction is whether a reader would be **misled about behaviour**. "The report says
the suite covers requirement 29 and it does not" is a real defect. "The report's package
list is missing a line" is a note.

**Check the task file against the tree before accepting a completion claim.** A worker that
signals COMPLETE with tasks still pending is claiming something the task file itself
contradicts; verify against `tasks.json` and `git log`, not against the claim.

## Findings, not spec edits

`SPEC.md` must not be modified. Record in `docs/reports/phase3-findings.md`: anything the
spec left undefined that you had to decide, anything contradictory or impossible, and every
deferral with what is deferred and to which phase. In particular this phase is expected to
produce findings for:

- `DECK_UNDO_MS` / `DECK_DELETE_GRACE_MS` (requirement 1) — new determinism controls.
- The rebound collapse key (requirement 37).
- Whether a rename should touch the tmux session (requirement 31).
- The store durability decision (requirement 40).
- The transcript path convention per adapter, with its provenance (requirement 4).

Do not silently invent configuration. A new flat config key is declared in the schema, shows
up in settings, and is recorded as a finding.

## Non-goals for this phase

Do not implement, even partially:

- The Codex adapter or its id discovery (Phase 4).
- Notifications, any channel, the outbox dispatch, quiet hours, rules, or `z` snooze
  (Phase 5). The `outbox` table already exists at schema v1; this phase reaps rows from it
  and writes none.
- Scrollback capture, history files, cwd tracking on resume, or `sensitive` (Phase 6). This
  phase defines and removes their paths (requirement 24); it does not write them.
- Cross-session search or `f` (Phase 7), beyond declaring `TranscriptPaths` (requirement 26).
- Send-without-attach or `s` (Phase 7, §11.1).
- The health view (Phase 7), beyond marking `captured_path` advisory (requirement 8).
- Any change to the hook/probe/status machinery Phase 2 and 2b-2 delivered.
- systemd units.
- A user-facing command line (`SPEC.md` R7). The TUI remains the only user-facing surface.

## Constraints

- Commit as described above. Do not rewrite history.
- Do not modify `SPEC.md`, anything under `prds/`, `ci/Dockerfile`, or `ci/SPIKE.md`.
- Do all building and testing in siblings via `ci/run.sh`. Install nothing into the job
  container.
- No network dependency in the default test suite: no real agent binaries, no model calls.
- Every sibling container is `--rm`. Leave no stray containers, images beyond
  `deck-ci:local`, or volumes beyond the shared cache.
- Prefer a small, readable implementation over a complete one. **This phase is judged by
  whether the lifecycle is provably correct and non-destructive, not by feature count.**
