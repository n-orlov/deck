# Phase 2b-1 — the visible shell

## Goal

Build the §11 layout: a **session sidebar beside a read-only preview**, with layout modes,
panel chrome, the attention sort and workspace grouping, and mouse navigation. This is the
phase that turns deck from a list into something a person uses by hand, so it is judged by
one question: **can the operator sit down in front of it, see which session needs them, and
get there?**

Nothing here invents behaviour. `SPEC.md` §11, §11.2, §11.3 and §11.8 specify this layout
down to the column, and §7 specifies the order. **Read them first — §11 changed materially
in commit `df527cc`,** which settled the preview as a cropped read-only capture, removed
preview scrolling and focus, and added the footer's status-reason line. A stale reading of
§11 will build the wrong thing.

## The requirement everything else serves

**The preview must not perturb what it previews.**

§11 states this as a requirement rather than an implementation note, because it is the one
property that a reasonable implementation gets wrong. A tmux client sized to the preview
panel reflows the shared window under §3.2's `window-size latest` and sends the agent
`SIGWINCH`; the agent then re-lays-out its TUI to 45×22 and the user's real terminal is
wrong until they attach again. `docs/spikes/tmux-embedded-preview.md` measured this, and its
addendum records what a mature implementation of the other choice costs.

So deck **reads** the pane and never participates in its geometry:

- `capture-pane -e`, no `attach`, no `pipe-pane`, no control-mode client.
- A pane larger than the panel is **cropped**, never resized and never reflowed.
- Throughout any amount of previewing: `tmux list-clients` is empty, `#{window_width}x#{window_height}`
  is unchanged, and the previewed program observes **no** `SIGWINCH`.

Requirement 21 is that assertion. If it cannot be made to hold, stop and record a finding —
do not weaken it, and do not "solve" a fit problem by resizing the pane.

## Context

### Where work happens

This job container has the docker CLI and the host docker socket; containers you start are
siblings on the host daemon, so every bind source must be a **host** path.
`$RALPHD_HOST_WORKSPACE` is the host path of the directory mounted here at `/workspace`.
`ci/run.sh` encapsulates this correctly — use it for all Go and tmux work
(`ci/run.sh go test ./...`). If the image is missing:
`docker build -t deck-ci:local -f ci/Dockerfile ci`. It carries Go 1.25.13 and tmux 3.5a.
Install no toolchain into this container.

### What already exists — do not rebuild it

- **A working TUI** (`internal/tui`) rendering a full-width list with status, badges,
  precedence, acknowledgement and the `i` detail dialog. Phase 2's status truth is done and
  operator-verified. This phase **re-shapes** that view into §11's two panels; it does not
  rewrite the model or re-derive status.
- **The store** (`internal/store`, schema v1) already has `sessions.workspace` — a free-text
  grouping label (§4) — so grouping needs no migration. It also has the status precedence and
  `notify_epoch` this phase's sort reads.
- **`internal/tmux`** already captures with escapes preserved via the
  `IncludeEscapeSequences` option (`capture-pane -e`). The preview's transport exists; use it.
- **`internal/config`** already parses `DECK_PREVIEW_MS`, whose default was changed to **250**
  in `df527cc` to match §11. Do not change it again.
- **Fake agents** (`cmd/fake-claude`, `cmd/fake-pi`) already fire §8.1 events at `deck _hook`
  from inside their own pane and **render a named fixture verbatim** — Phase 2 built both.
  The preview's deterministic pane source is an extension of that fixture mechanism, not a new
  one.
- **The godog harness** (`features/`) already drives the released binary through a pty at
  100×30, with a real tmux on a private socket. `ci/stability.sh 10` already exists.

### Working practice: commit as you go

**This phase commits and pushes its own work** (see `docs/PLAN.md`). **One commit per
completed task**; messages say *why* and reference requirement numbers; the commit log is the
durable memory a later phase reads. `~/.gitconfig` and `~/.git-credentials` are already
mounted, so `git push origin main` works with no token handling from you — never put a token
in a URL, a file, or a message.

**Commit hygiene, not a commit count.** If review requires a correction to something already
committed, **add a follow-up commit**. Never force-push, amend, rebase or reset published
history: fix forward, always. There is no upper bound on commits and no virtue in a low one;
the only rules are that each commit is a coherent completed unit and that the tree is green
when you make it. Run build, vet and the suite before each commit.

Never commit build output, caches, secrets, or `SPEC.md` / `prds/` / `ci/Dockerfile` /
`ci/SPIKE.md`.

### Assertions this phase must deliberately change

Phase 0–2 pinned the *full-width list* into the suite. §11's sidebar is 35 total columns —
**31 content columns** — so assertions that fit a 100-column row do not fit a sidebar row, and
a red run must be fixed on the correct side.

Two mechanical facts make this much smaller than the raw grep suggests, and finding them first
is worth an hour: `features/assertions_test.go:846` is a **shared helper** that every
`screen contains "resumable"` scenario routes through, and `internal/tui/tui.go:534`/`:537` are
the **only two** places reason text is appended to a row. Re-aim those three sites and most of
the list below follows.

| site | current assertion | after this phase |
|---|---|---|
| `internal/tui/tui.go:534` (`· resumable`), `:537` (`· awaiting signal`), and the §7 reason path feeding `:733` | reason text is appended to the **row** | the row carries **glyph, name, status word and badges, and no reason text** (§11.3). The reason moves to the footer for the **selected** row, and stays in the `i` dialog. §7's honesty is preserved: the copy is still on screen for the row the user is on, which is the only row it was ever informative for |
| **14 feature-file assertions on `resumable`** across `agent_session`, `concurrency` (×3), `crash`, `durable_identity` (×2), `launch_lease` (`stopped - resumable`), `permission_modes`, `resume_failure` (×3), `walking_skeleton` — plus `features/assertions_test.go:846` and `features/determinism_test.go:175` | asserted as row text on a wide list | re-aim through the shared helper where possible: select the row, assert the footer. Where a scenario asserts `resumable` for a session it has *not* selected, that assertion changes meaning and must be re-thought, not mechanically rewritten. **State in the commit message what each re-aim was protecting and how the new form still protects it** |
| `features/shell_liveness.feature:18` (`starting - awaiting signal`) and `:10` (`does not contain "awaiting signal"`) | the composite string on a row | `:18` re-aims to the footer with the row selected. **`:10`'s negative assertion survives almost unchanged and must stay** — a *shell* row must show no `awaiting signal` anywhere, footer included. Do not delete it as collateral |
| `internal/tui/tui_test.go:48` `TestStartingCopyDistinguishesShellFromSignalledAgents` — asserts `strings.Count(view, "starting · awaiting signal") == 2` over the rows | counts the copy on two agent rows at once | with the reason on the footer only one row's reason is visible at a time, so the count assertion is now wrong by construction. Re-aim by **selecting each row in turn** and asserting the footer — the shell-vs-agent distinction this test exists for must still be tested, and it is the §7 copy rule's only unit-level guard |
| `internal/tui/resume_test.go:49` | `view` contains `starting · awaiting signal` after a resume | same re-aim. Note its comment at `:16` states the intent — update the comment with the assertion, so the next reader is not told the row carries copy it no longer carries |
| `features/launch_lease.feature:15` and 4 negative sites, `features/lease_race.feature:17` (`starting elsewhere`) | §9.3's resume note as row text | `starting elsewhere` is a reason, so it follows the same rule. The **negative** assertions matter more than the positive one here: keep them, footer included |
| every scenario asserting a session **name** (`walking session`, `shared session`, `hook shared`, `after crash`, `changed verdict target`, `claude one`, `shell row`, …) | a name renders in full on a 100-column row | in a 31-column sidebar a long name elides. Fix by setting a wider `sidebar_width` for that scenario (requirement 7) — **not** by shortening the assertion to a prefix, which would silently stop testing elision *and* stop testing the name |
| `features/determinism.feature` and `features/determinism_test.go` | byte-stable frames of the old layout | re-record against the new layout. A golden frame is regenerated, not hand-edited — the regeneration must be reproducible, and the preview region needs the deterministic fake pane of requirement 5 |
| `features/walking_skeleton.feature:16`, `internal/tui/tui_test.go:16` | `No sessions yet` / `Press n` on a bare screen | the empty state now lives inside the sidebar panel, with the preview showing its own placeholder. Keep asserting the copy; it must still be reachable and legible at 80×24 |
| `internal/tui/tui_test.go:22-42` (help copy) and `cmd/deck/main_test.go:361` (the same overlay through a real pty at 90×100, under `DECK_ASCII=1`) | pin the help overlay incl. the runtime-controls list | the overlay is deck's only documentation (R7): it gains `space`, `|`, `<`/`>`, the mouse bindings, `DECK_MOUSE` and §11.8's `shift`-to-select caveat, and **loses `tab`**. Update both pins together, and remember the pty test runs in ASCII mode so its glyphs differ |
| `internal/tui/tui_test.go:44`'s "advertises unavailable action" list | forbids copy for verbs the binary lacks | **`tab` must be added to it** (§11.3 — the main view has one focusable region, so `tab` becomes a key deck must not advertise). Nothing is removed: none of this phase's verbs are on that list. This test is the mechanism enforcing §11.3's footer rule — strengthen it, never relax it |
| `cmd/deck/main_test.go:353-357` | three pty clients each waiting for `resumable` | the same re-aim, through a real terminal. This one also proves the footer is legible at the pty's geometry, so it is worth keeping rather than dropping to the Go-level tests |

Three rules about this table. First, **it is not exhaustive by construction** — sweep for
anything asserting screen text that assumes a full-width row, by any means (screen text,
golden frames, Go-level view tests), and treat what you find and it does not list as a
finding. Second, **no assertion may be weakened to green a run**; re-aim it and say in the
commit message what it was protecting and how the new form still protects it. That sentence is
the only thing standing between a re-aimed assertion and a deleted one. Third, if re-aiming
one would require changing behaviour `SPEC.md` specifies, that is a finding, not a licence.

**Negative assertions are the trap here.** Roughly half these sites assert that copy is
*absent* — a shell that must not say `awaiting signal`, a client that must not say `starting
elsewhere`. Moving reason text to the footer makes the positive assertions fail loudly and the
negative ones pass **vacuously**, because copy that moved off the row is trivially absent from
it. Every negative assertion touched in this phase must be re-aimed to the surface the copy now
lives on, or it silently stops testing anything. A negative assertion left pointing at the old
surface is a defect even though the suite is green.

## Requirements

Each requirement must be individually verifiable by a command or scenario whose real output is
recorded in the phase report (requirement 45).

**Numbering is not build order. Requirements 1–7 are harness prerequisites and land first**,
because nearly every scenario below is impossible without them, and discovering that halfway
through is how an iteration budget evaporates.

### Harness prerequisites (build these first)

1. **Mid-scenario pty resize.** The driver can resize the pty (`TIOCSWINSZ` + `SIGWINCH`) and
   re-read the grid, exposed as steps that set an initial geometry and change it later. §11.2's
   "a resize re-chooses the mode" cannot be asserted without it, and it cannot be faked by a
   scenario.
2. **SGR (1006) mouse-report synthesis.** Steps that send a click, a double-click, a wheel-up,
   a wheel-down and a press-move-release drag at a given cell coordinate, plus a step that
   asserts **nothing changed** (needed for the preview's no-op bindings). SGR only — the
   harness must not depend on X10 encoding.
3. **`DECK_MOUSE`** (§13.1): a boolean env control forcing mouse reporting on or off,
   overriding `[ui] mouse`. Documented in the help overlay like every other `DECK_*` knob.
4. **Fake agents record the terminal sizes they observe** — the initial size and every
   `SIGWINCH` — somewhere under `DECK_HOME` a step can read. This is what makes requirement 21
   an assertion about the agent's own experience rather than an inference from tmux bookkeeping.
5. **A deterministic fake pane for the preview.** Extend the existing fixture-rendering mode
   (Phase 2) so a fake agent draws a named fixture and then produces no further output, giving
   a byte-stable preview region. Ship at least three fixtures: one that fits the panel, one
   **wider and taller** than the panel (for the crop), and one containing **East-Asian-Wide**
   characters (for requirement 23).
6. **The screen emulator must place double-width cells correctly.** The harness reads the
   screen through `hinshun/vt10x`, and `docs/spikes/tmux-embedded-preview.md` found it
   **fails East-Asian-Wide placement**. The preview is the first place foreign bytes reach
   deck's screen, so this is now load-bearing: either move the harness to
   `charmbracelet/x/vt` (which the spike validated for exactly this) or demonstrate with a
   wide-character assertion that the current emulator is correct. Whichever you choose, the
   whole suite must be green afterwards, and the choice goes in the report with its evidence.
7. **A sidebar-width step.** Set and read back `sidebar_width` for a scenario. Needed by
   §11.2's width-key requirements anyway, and it is how existing scenarios that assert long
   session names stop being about elision.

**Each of requirements 1–6 gets its own coverage in `features/harness.feature`**, which is
where Phase 0 established that harness capabilities prove themselves rather than being trusted.
A resize step that silently does nothing, or a mouse step that writes bytes no one reads, turns
every scenario built on it green for the wrong reason — and that failure is invisible precisely
in the scenarios meant to catch it.

### Layout modes (`SPEC.md` §11.2)

8. Three modes, one always in force: `side-by-side`, `stacked`, `collapsed`. In `auto`, width
   ≥ 80 selects `side-by-side` and width < 80 selects `stacked`; **`collapsed` is never
   selected automatically.**
9. `|` cycles `auto → side-by-side → stacked → collapsed → auto`. The explicit modes pin the
   layout regardless of width; `auto` returns to width-based selection.
10. Every width and floor in §11.2's table is **total columns for that panel, borders and
    padding included**: sidebar default 35 / floor 24, preview floor 40, stacked list height
    `min(max(rows/3, 5), 12)` with preview floor 8, collapsed strip 3.
11. `<`/`>` adjust `sidebar_width` by one column, clamped to `[24, width − 40]`.
12. **`layout_mode` and `sidebar_width` persist in `state.db`, never `config.toml`** — a
    keypress must never rewrite the config file, because §11.5's settings takeover is that
    file's only writer.
13. A resize re-chooses the mode under `auto`. A **pinned** mode that cannot hold its floors at
    the current width renders as `auto` for as long as that is true, **without overwriting the
    pinned choice** — the pin returns when the terminal does.
14. Below 80×24, `auto` renders `stacked` as far as it fits and the footer states that the
    terminal is below the supported minimum.
15. `collapsed` renders a 3-column strip: `»` above the **attention count**, drawn vertically,
    with the preview taking everything else. `|` restores the sidebar (there is no `tab`).

### Panel chrome (`SPEC.md` §11.3)

16. Rounded borders (`╭╮╰╯`) on every panel, dialog and overlay — one border style throughout,
    with the existing `DECK_ASCII` fallback honoured.
17. Exactly one column of horizontal padding inside the sidebar and the preview, so content
    never touches a border.
18. **A single seam.** The sidebar draws top, left and bottom borders only; the preview draws
    all four and its left border *is* the divider. A `││` double seam is a defect.
19. **Focus.** The main view has exactly one focusable region, the sidebar; `↑`/`↓`/`PgUp`/`PgDn`
    always drive the list, and **`tab` is not bound in the main view**. The focused surface's
    border uses the focus colour, so an open dialog takes focus and the sidebar's border
    reverts.
20. **The footer** is one line outside both panels, contextual, in the key/description pattern,
    and **never lists a key that is not bound**. It carries the **selected row's status
    reason** on its left, separated from the keys.

### The preview (`SPEC.md` §11)

21. **The preview attaches no client and never resizes a pane.** Asserted, not asserted-about:
    with a preview live and ticking, `tmux list-clients` is empty, `#{window_width}x#{window_height}`
    equals its pre-preview value, and the fake agent's recorded size list (requirement 4) shows
    no `SIGWINCH`. This holds across selection changes, mode changes, sidebar-width changes and
    an outer-terminal resize.
22. Capture is `capture-pane -e`, **selected row only**, at `DECK_PREVIEW_MS` (default 250 ms).
    No other row is captured — one selected pane, one capture per tick.
23. **Crop, anchored bottom-left**: the newest rows, from column one. The panel states the real
    geometry (`45×22 of 120×40`) and lines cut at the right edge are marked. A pane smaller than
    the panel is not stretched.
24. **Cell-aware crop and elision.** Where a crop or ellipsis boundary falls inside a
    double-width cell, deck emits a **space**, never half a glyph. Asserted with the wide
    fixture from requirement 5, against the border column — a sheared border is the symptom
    this requirement exists to prevent.
25. **The preview does not scroll.** No capture history, no `PgUp`, and a wheel over the preview
    does nothing (requirement 33). There is no preview scroll state to persist.
26. **Rows with no live pane.** An `error` row renders §7's stored crash tail, headed with the
    fact that it is the last output before the exit and is **not live**. `stopped`, `archived`,
    and a `starting` row whose pane does not exist yet render a one-line placeholder naming the
    state. Stale bytes are never presented as live.
27. The preview is **not shown** when its panel would fall below §11.2's 40-column floor
    (8 rows in `stacked`); the sidebar takes the space.

### Attention sort and grouping (`SPEC.md` §7, §11)

28. Sort order is exactly: `waiting` (**oldest first**) → `error` → `running` → `starting` →
    `idle` → `stopped`.
29. The order is **total and deterministic**: ties within a status are broken by a documented,
    stable key so that a frozen clock yields one frame, not two. State the key in the report.
30. Grouping by `workspace` (default: basename of `cwd`), **collapsible**, and **never by
    repo**. Group headers are rendered in the current palette — the theme token set is Phase
    2b-2, so do not build it here.
31. `space` moves the selection to the **next session needing attention**, wrapping, and does
    nothing observable when nothing needs attention. It never changes status: §7's
    attach-clears-`waiting` must not fire from a navigation key.
32. The **attention count** rendered by `collapsed` mode (requirement 15) is the same
    computation, not a second one. One source of "needs me", used by the sort, the count and
    `space`.

### Mouse navigation (`SPEC.md` §11.8)

33. Click a sidebar row → selects it; the preview follows on its next tick. **A single click
    never attaches.** Double-click a row → attach. Click a group header → toggle collapse.
    Wheel over the sidebar → scrolls the list, changing **neither selection nor focus**. Wheel
    or click over the preview → **nothing at all**, and it must not fall through to the
    sidebar. Drag the seam → adjusts `sidebar_width` live. Click the collapsed strip → restores
    the previous non-collapsed mode.
34. **Hit-testing consults the layout that drew the frame.** A click resolves to a row through
    the same geometry that rendered it, never by independently recomputing row heights or panel
    offsets. Two implementations of the geometry drift the moment grouping, elision or a mode
    change touches one of them, and the symptom is a click that selects the wrong session —
    silent, intermittent, and indistinguishable from a mis-click.
35. **No capability is mouse-only** (R7). Every binding above duplicates a key, and deck renders
    no control only a mouse can operate — no scrollbar that is the only way to scroll, no close
    button that is the only way to dismiss.
36. SGR extended reporting (1006), so coordinates past column 223 are correct. Reporting is
    enabled on start and **disabled on every exit path, including a panic** — a deck that exits
    without disabling it leaves the user's shell printing escapes at every mouse move.
37. Opt-out via `[ui] mouse` (default true, §6.5) and `DECK_MOUSE` (§13.1), with the
    `shift`-to-select caveat documented **in the help overlay**. A terminal that reports no
    mouse events loses the shortcuts and nothing else.

### Scenarios that define this phase

38. **`layout_modes.feature`** — `auto` selection at and either side of 80 columns, `|` cycling
    through all four states, a mid-scenario resize re-choosing the mode, a pinned mode falling
    back and returning, `<`/`>` clamping at both ends, and persistence across a restart (in
    `state.db`, with `config.toml` proven unchanged).
39. **`preview.feature`** — requirement 21's no-client/no-resize/no-`SIGWINCH` assertion; the
    crop with its geometry line; the wide-cell boundary; the wheel and click no-ops; the crash
    tail on an `error` row; the placeholder where there is no pane; and the preview absent below
    its floor.
40. **`attention_sort.feature`** — the full order including `waiting` oldest-first, workspace
    grouping and collapse, the collapsed strip's count, and `space` walking what needs
    attention without changing any status.
41. **`mouse.feature`** — click selects and does not attach, double-click attaches, group-header
    click collapses, wheel scrolls without selecting, seam drag resizes, preview gestures do
    nothing, and `DECK_MOUSE=0` disables the lot while every keyboard path still works.
42. **The golden minimum frame**: side-by-side, sidebar 35 / preview 45, at exactly **80×24**,
    byte-exact, with `DECK_MOUSE=0` (its enable/disable sequences are bytes in the stream) and
    the deterministic fake pane of requirement 5.

### Evidence and stability

43. **The suite is green ten consecutive times from a clean state** — `ci/stability.sh 10`,
    10/10, with the real output in the report. Twice is not evidence: Phase 0's "passes twice"
    bar was met while a scenario failed one run in three.
44. The help overlay is updated (R7): every key this phase binds, every key it unbinds
    (`tab`), `DECK_MOUSE`, and §11.8's `shift` caveat. The overlay is deck's only
    documentation, so it is part of the deliverable, not a comment.
45. `docs/reports/phase2b1.md` records, per requirement number, the command or scenario that
    verifies it and its **real output** — not a claim that it passes. Include the emulator
    decision from requirement 6 with its evidence, and the tie-break key from requirement 29.
46. No root-owned files left in the workspace; no leftover tmux sockets; `git status` clean at
    the end. **Container hygiene needs no action from you and you must not attempt it**: every
    sibling `ci/run.sh` starts is `--rm`, so there is nothing to clean up. Check it read-only
    and scoped to the image, never by label:
    `docker ps -a --filter ancestor=deck-ci:local --format '{{.ID}} {{.Status}}'`.
    **A sweep filtered on `label=ralphd.run=…` deletes your own container** — the job container
    carries that label — which SIGKILLs this job mid-iteration, loses the verdict, and has now
    done so twice on this project. Verifying this requirement means running the read-only
    command above and reading its output. Nothing else.

## Review guidance

Block on **real defects**; do not block on documentation nits. Wording, formatting and stale
sentences in derived summaries are recorded as notes and do not fail a phase. The test is
whether a reader would be **misled about behaviour**. Phase 0b was rejected three times over
three sentences with a finished product in the tree; that is a bug in the process, not
diligence.

In this phase specifically, these are blocking:

- **Any preview that attaches a tmux client, resizes a pane, or lets the previewed program see
  a `SIGWINCH`** — the requirement the phase exists to protect, and the one a plausible
  implementation gets wrong.
- Requirement 21 asserted only by reading deck's own code or logs rather than by tmux's and the
  agent's own observations.
- A golden frame regenerated by hand-editing the expected bytes, or made stable by excluding
  the preview region rather than by making the pane deterministic.
- Hit-testing that recomputes geometry independently of the renderer (requirement 34), or a
  click that can attach.
- Mouse reporting left enabled on any exit path, including a panic.
- A weakened or deleted Phase 0–2 assertion instead of a re-aimed one, or a name assertion
  shortened to a prefix to dodge elision.
- **A negative assertion left pointing at the row after its copy moved to the footer** — green,
  vacuous, and no longer testing anything. `features/shell_liveness.feature:10` and
  `features/launch_lease.feature`'s four `does not contain "starting elsewhere"` lines are the
  named cases; sweep for the rest.
- A new harness capability (requirements 1–6) used by scenarios but not itself covered in
  `features/harness.feature`.
- A layout keypress that writes to `config.toml`.
- A sort, an attention count and a `space` target computed by three different code paths that
  can disagree (requirement 32).
- Any status change caused by navigation — `space`, a click, or the wheel.
- A capture of a row that is not selected, or a preview tick that continues while the preview
  is not shown.
- Building any of Phase 2b-2: §11.4's dialog contract, the settings takeover, or the theme
  token set.

## Findings, not spec edits

**`SPEC.md` is read-only to this job.** Record in `docs/reports/phase2b1-findings.md` anything
the spec left undefined that you had to decide, anything contradictory or impossible, and every
deferral with its target phase. Likely candidates, so they are not surprises: exactly how a
right-cut line is marked; where the geometry line sits on the preview's chrome; the tie-break
key of requirement 29; the placeholder copy of requirement 26; and how a group header renders
before the theme system exists. If a new `DECK_*` control is needed to make something testable,
introduce it, document it in the help overlay, and record it as a finding for the operator to
fold into `SPEC.md` — do not silently invent configuration, and do not edit the spec to match
your code.

## Non-goals for this phase

Do not implement, even partially: §11.4's dialog contract retrofit, the §11.5 settings
takeover, or the §11.6 theme system incl. the picker and quantised palette (**all Phase
2b-2** — leave the existing dialogs exactly as they are, and do not dim a backdrop); the
create modal's completion, §11.7 path entry, the env editor, kill/undo toasts, `dd` tombstones,
reap, purge, archive, bulk marks, rename or the event log (Phase 3); the Codex adapter
(Phase 4); notification channels, rules or the outbox (Phase 5); scrollback replay, history
files or `last_cwd` (Phase 6); send-without-attach (§11.1), cross-session search or the health
view (Phase 7); systemd units.

Do not make the preview interactive, do not add a keystroke path into a previewed pane, and do
not add a user-facing command line (R7). `↵` is how a user gets a real terminal, and that is
the whole design.

## Constraints

- Commit and push as described. **Fix forward; never rewrite published history.**
- Do not modify `SPEC.md`, anything under `prds/`, `ci/Dockerfile`, or `ci/SPIKE.md`.
- Do all building and testing in siblings via `ci/run.sh`. Install nothing into this container.
- **The default suite must not depend on a real agent binary, network access, or model
  output.** Only `@real-agents` scenarios may touch an installed CLI, and they are excluded by
  default.
- deck never writes to or deletes anything inside a session's `cwd` (R1).
- Every tmux server you start is on a **private socket** and is killed afterwards. Never touch
  the default socket.
- Every sibling container is `--rm`, so no cleanup sweep is needed or wanted. **Never run
  `docker rm` / `docker kill` filtered by `label=ralphd.run=…`, and never target the container
  named `ralphd-deck-phase2b1`: your own job container carries that label, so such a sweep
  SIGKILLs the job mid-iteration and loses the verdict.** To check for leftovers, do it
  read-only and scoped to the image:
  `docker ps -a --filter ancestor=deck-ci:local --format '{{.ID}} {{.Status}}'`.
- Prefer a small, readable implementation over a complete one. This phase is judged by whether
  the operator can find the session that needs them and get to it — not by feature count.
