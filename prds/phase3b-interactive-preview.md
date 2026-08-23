# Phase 3b — the interactive preview

## Goal

Make the preview a place you can *work*, without leaving the list. `Enter` hands the keyboard to
the selected session: deck fits that session's tmux window to the preview panel, streams the pane
into an in-process cell grid, and forwards keystrokes to it. `Ctrl+Q` returns. `a` remains the
escalation to a real full-screen terminal.

This closes the gap that makes deck a viewer rather than a console. Today answering a `waiting`
prompt costs a full-screen context switch for two keystrokes, an agent's transcript cannot be
wheel-scrolled at all, and `SPEC.md` §11.1's send-without-attach is a narrow protocol built
entirely around the fact that you cannot see what you are typing into.

Every technical claim in this PRD was measured on 2026-08-22 by three spikes, on **both** tmux
3.6b (the host) and 3.5a (the CI image), with 815 retained evidence files. Read
`~/deck-spikes/interactive-preview/README.md` first, then `a/REPORT.md` (geometry),
`b/REPORT.md` (transport) and `c/REPORT.md` (input and fit). **Where this PRD asserts a number,
the spike evidence is the citation.** Do not re-derive them; do not contradict them without
measuring.

## The requirement everything else serves

**Passive preview's non-perturbation guarantee survives this phase untouched.**

`SPEC.md` §11 says the preview attaches no client, opens no pipe, and never resizes a pane, and
`features/preview.feature`'s `@requirement-21-preview-no-side-effects` asserts it: `list-clients`
empty, `#{window_width}x#{window_height}` unchanged, and the fake agent's size log unchanged across
selection, mode, sidebar-width and outer-terminal changes. **That scenario must still pass,
unmodified, at the end of this phase.** It is the difference between "deck perturbs a pane when you
ask it to" and "deck perturbs a pane whenever you look at one", and the second is not shippable.

Everything interactive mode does is gated behind a deliberate keypress on one session, is claimed
before it acts, is restored when it ends, and is **refused** rather than degraded when it cannot be
done safely.

The second-order version: **an unusable interactive mode must announce itself.** A hung agent
narrowed to the preview box renders an empty frame with no echo — measured. That must read as "the
agent has not repainted", never as deck being broken.

`SPEC.md` is the authoritative product spec and **must not be modified**. Where this PRD and
`SPEC.md` disagree, `SPEC.md` wins and the disagreement is a finding.

## Context

### Where work happens

Unchanged from Phase 3: the job container has no Go and no tmux and cannot install them. All
building and testing happens in the sibling toolchain container via `ci/run.sh`.

### What already exists — do not rebuild it

- **`charmbracelet/x/vt` is already a direct dependency** (`go.mod:8`), used by four harness files
  including `features/emulator_placement_test.go` and `features/cell_attributes_token_test.go`. A
  product grid adds **no new module**. Its wide-cell placement conformance is already established.
- **Per-cell SGR assertions exist** (Phase 2b-2) — `theme_pin_test.go`,
  `cell_attributes_token_test.go` read real `Style.Fg` via `CellAt`. Focus and label assertions use
  this, not screen-scraping.
- **`selection_idle` already exists** in the theme schema (`internal/theme/token.go:19`,
  `SPEC.md:1162`) and is already documented as "selected row, unfocused panel". It needs a second
  consumer, not a definition.
- **Fake agents that record every terminal size they observe** exist from Phase 2b-1. They need a
  repaint-behaviour mode, not rebuilding.
- **Bubble Tea stays at v1.3.10.** The earlier spike's claim that it cannot coexist with current
  `x/vt` is **refuted** — the blocker was a stale transitive `x/cellbuf`, cleared by
  `go get github.com/charmbracelet/x/cellbuf@latest`. Do not migrate to v2 in this phase.

### Assertions this phase must deliberately change

Enumerated so a red run is not "fixed" on the wrong side.

1. **`features/preview.feature` `@requirement-21-preview-no-side-effects` does NOT change.** It
   describes passive preview. If it goes red, the defect is yours.
2. **`@requirement-23-preview-crop-geometry`** asserts the panel shows `\d+x\d+ of \d+x\d+`. That is
   a *crop* statement; fitted it degenerates to `45x22 of 45x22`. Scope the existing scenario to
   passive preview and give interactive mode its own copy.
3. **`SPEC.md` §11.3's "the sidebar is the single focusable region … there is no `tab` panel
   cycle"** becomes false. Any scenario or golden frame resting on one focus stop is re-aimed.
4. **`features/mouse.feature`'s double-click → attach** becomes double-click → interactive mode.
   Single-click-selects is unchanged and must stay green.
5. **`features/mouse.feature`'s wheel-over-the-preview-does-nothing** holds for passive preview and
   is re-aimed for interactive.
6. **`cmd/deck/main_test.go:442`'s released-help guard.** Entries this phase ships move out of the
   unavailable list and into the *present* list **in the same commit**, with the exact help wording
   asserted. This is the discipline steer 001 established in Phase 3 for `"undo"`. Do not delete an
   entry to turn the test green.
7. **The `a` binding takes over full attach from `Enter`.** Every scenario that presses `Enter`
   expecting a full-screen attach is re-aimed to `a`, and the footer copy changes with it.

## Requirements

### Harness prerequisites (build these first)

1. **A fake agent with three repaint behaviours**, selectable: repaints on `SIGWINCH`; ignores
   `SIGWINCH` but repaints on the next keystroke; never repaints. The third is the realistic worst
   case — an agent waiting on a network round-trip — and it is what proves requirement 44. Extend the
   existing size-recording fake rather than writing a fourth agent.
2. **A `SIGWINCH` count assertion**, not merely a size log. Requirement 11 is a count.
3. **A tmux option-scope assertion step**: the value of an option *and* the scope it lives in
   (`show -gv` versus `show -wv`). Requirements 8 and 10 are about scope, and an assertion that
   cannot see scope cannot prove them.
4. **`DECK_INTERACTIVE_MS`** — the grid's render-coalescing interval, a duration, declared in the
   §6.5 schema with its `DECK_` override like every other key.
5. **`DECK_INTERACTIVE_TRANSPORT=pipe|capture`** — pins the render path. State in the report that
   this is a *selector between two implementations of one contract*, not a behaviour switch of the
   kind §13.1 forbids: both paths must satisfy the same scenarios except the ones requirement 33
   names as pipe-only.
6. **A step that asserts a pane's `history-limit`**, for requirement 15.

### Geometry: owned, claimed, restored (spike A)

7. **Record before acting.** Entering captures `#{window_width}x#{window_height}` and the
   **window-local** `window-size` value (`show-options -wv`; an unset window option prints an empty
   line and exits 0, while an unset *user* option errors — treat both as "none").
8. **Resize the window, never the pane.** On a single-pane window `pane_height == window_height`
   and the status line contributes zero chrome; on a **split** window chrome is *proportional*, a
   naive pane-targeting re-assert loop **diverges forever** (measured: 12 iterations, `pane_height`
   stuck at 11), and a chrome-compensated loop needs 5-6 resizes — i.e. 5-6 `SIGWINCH` — to land a
   22-row pane inside a 42-row window. Targeting the window is deliberate; record it.
9. **Exit restores in this order, and the order is load-bearing**: `resize-window` back to the saved
   dimensions **only if `#{session_attached} == 0`**, then `set-option -w -u window-size`. Reversed,
   the resize re-flips `manual` and the window stays pinned. With a client attached, the unset alone
   restores the size immediately and an explicit resize-back costs a third wasted `SIGWINCH`.
10. **Prove `set -g window-size latest` is not a restore.** `resize-window` writes `window-size
    manual` into the **window** options, which shadow the global; a scenario must assert that the
    global write leaves a fresh client pinned at the preview size. This is the trap the shipping
    prior art documents wrongly, and pinning it is what stops a later "simplification".
11. **Exactly two `SIGWINCH` per enter/exit cycle**, attached and detached. Not three.
12. **Byte-exact restore.** After exit, `window-size` is the only option that was ever touched, in
    the window scope only, and every option table matches its pre-entry state. Assert the
    server-global value still reads `latest`.
13. **A fresh client at a third size governs the window after exit** — with the **mandatory negative
    control** that skipping the restore leaves it pinned, both while that client is attached and
    after it detaches. Without the control the restore is unproven.
14. **Ownership is a pid-tagged window option with a confirm-read.** Claim `@deck_isize_owner` as
    `<tag>:<pid>`, re-read, and stand down unless you see yourself; validate a found owner with
    `kill(pid, 0)` and steal from a dead one; release on exit. **No heartbeat and no TTL** — every
    writer of a tmux socket is on the socket's host, so liveness is a syscall rather than a lease.
    Two naive writers thrash unbounded (~6 `SIGWINCH`/s, neither winning), so exclusion is
    mandatory; the read-then-write is not atomic, which the confirm-read resolves.
15. **Set `history-limit` explicitly on deck's server.** tmux defaults to 2000 and deck sets
    nothing. A narrowed window consumes history ~2.7× faster and evicted rows never return —
    measured at 23 of 40 logical lines destroyed against a control that lost none. A pane's limit is
    fixed at creation, so it must be set before `new-session`, which means it belongs in
    `Client.Bootstrap` beside the other server options and must be **single-sourced** there.

### The transport (spike B)

16. **`pipe-pane -IO` into one long-lived `x/vt` grid**, and the pipe is armed **before** the seed
    capture is taken. Armed after, every byte in between is lost with no way to notice.
17. **The seed, in this order**, verified to reach zero differing cells *and* correct mode state:
    fresh parser; `ESC[?1049h/l` from `#{alternate_on}` **first** (1049 clears the buffer it
    switches to); a neutral painting state (`ESC[?6l ESC[r ESC[?7h ESC[4l`); the capture body
    **verbatim**; `DECSTBM` from the scroll region *after* the body (it homes the cursor); origin
    mode then the cursor; then the modes that must not disturb the paint.
18. **The capture body goes in verbatim — never re-addressed or SGR-reset per line.**
    `capture-pane -e` is **one continuous SGR stream across all rows**: a real capture's row 3
    begins with the glyph and *then* its SGR, having inherited the pen from row 2. A seed that emits
    `ESC[<row>;1H ESC[0m ESC[2K` per line is byte-perfect in content and **wrong in 156 cells'
    colour**. Use `capture-pane -p -e -N`; without `-N` trailing background-styled blanks are
    trimmed.
19. **Seed state from tmux formats, including the three the prior art omits**: `wrap_flag`,
    `origin_flag` and `scroll_region_upper/lower`, alongside `alternate_on`, `cursor_x/y`,
    `cursor_flag`, `insert_flag`, the keypad flags and the mouse flags. **A capture carries cell
    content and SGR only** — not one DEC private mode, no scroll region, no cursor position survives
    it. deck can seed strictly better than the shipping implementation because tmux hands these over
    free.
20. **Pair the capture and the state atomically.** Chain them in one invocation and re-probe,
    retrying while two probes disagree; `#{history_size}` and `#{pane_width/height}` are the
    discriminators. tmux processes pane output between two separate commands.
21. **Reseed on resize; never merely `vt.Resize()`.** Against an append-only pane, `Resize()` alone
    left ~1000 wrong cells for **3.4 s**; doing nothing left a stable divergence mask for **31.6 s**
    and never healed. Reseed corrected it in the same sample that detected it.
22. **A reseed constructs a fresh parser**, or at minimum leads with `CAN` (0x18). A grid whose
    input stopped mid-control-sequence parses the next bytes as a continuation and corrupts its own
    seed — measured.
23. **Poll `#{pane_dead}` on the live path.** Under `remain-on-exit failed`, **a dead pane never
    closes the pipe**: no EOF, the reader blocks on `read(2)` indefinitely, the grid freezes.
    Treating EOF as the only liveness signal renders a stale grid forever for a crashed agent, which
    is precisely the case deck's §7 crash handling exists to catch.
24. **Distinguish displacement from death, and say so.** `pipe-pane` is single-holder per pane; a
    second arm displaces the first in ~4 ms, silently, with no error to either party and an
    identical clean `rc=0` EOF. EOF with `pane_pipe` still 1 means displaced: fall back to passive
    capture **and tell the user in the panel**. EOF with `pane_pipe` 0 means the pipe was disabled.
25. **Release the pipe on exit** and assert `pane_pipe` returns to 0.
26. **Only composed cells reach the outer terminal.** This is a correctness requirement, not
    hygiene: raw pane bytes passed through leave the **outer** terminal on the alternate screen —
    deck's chrome on a buffer the user can no longer see — and reprogrammed the outer scrolling
    region 1,166 times in 12 s. A grid-composed payload contained **zero** occurrences of every
    dangerous class. Passive preview gets this free from `capture-pane`; that property belongs to
    `capture-pane`, not to deck.
27. **Coalesce renders** at `DECK_INTERACTIVE_MS`. Render frequency dominates the transport's cost
    (rendering per read is 1.99× the cost of coalescing to 60 ms), not parsing.

### The input path (spike C)

28. **Dispatch on a verified five-field identity**: socket path, server pid, `pane_id`, **`pane_pid`**
    and session name, captured at entry and **re-resolved immediately before every send**. Reject on
    any drift, on a non-zero exit, or on `pane_dead != 0`.
29. **Prove why `pane_pid` is in that tuple.** After `respawn-pane`, `pane_id`, `session_name`,
    `pane_dead`, `pane_start_time` and `pane_current_command` are **all unchanged** — only
    `pane_pid` moves. A scenario must show that verifying `pane_id` alone, and verifying
    `pane_id + session_name`, both **deliver the keystrokes into the replacement program**, and that
    adding `pane_pid` rejects. Include the negative control that the four-field verify still
    delivers against an unmutated pane, so it is not vacuous. `pane_start_time` is empty on both
    tmux versions and is useless as a discriminator.
30. **Never dispatch by session name**, and prove it: rename a session, create a new one reusing the
    name, and show the payload landing in the impostor's pane with `exit=0` and empty stderr.
31. **Carry the socket and a server-lifetime discriminator.** Pane ids are monotonic and never
    reused *within* a server lifetime, but a restarted server reissues `%0`, and ids **collide
    across sockets** — cross-socket dispatch silently hits the local pane with `exit=0`.
32. **`send-keys -l --` for every literal payload.** Without `--`, a payload beginning with `-` is
    **silently discarded with exit 0**; `--help` errors. Without `-l`, the literal string `Enter`
    becomes a carriage return.
33. **Peel exactly one trailing `;` and re-send it as `-H 3b`.** tmux consumes one trailing
    semicolon even after `--`; interior semicolons are safe; the `\;` escape is not composable.
34. **Named keys go by tmux name and are never hand-encoded.** tmux applies DECCKM to the cursor
    keys and knows the pane's current mode; deck does not. Note tmux emits vt220 `ESC[1~`/`ESC[4~`
    for Home/End rather than `ESC[H`/`ESC[F`, identically through `send-keys` and through a real
    attached client — so deck's translation is faithful to attach, which is the standard that
    matters.
35. **Validate key names against an allowlist before spawning.** An unknown name is **typed into
    the agent as literal text with exit 0** — `send-keys Frobnicate` delivers ten bytes.
36. **Chunk literals at 8192 bytes and `-H` at 4096 bytes.** The real ceiling is tmux's own ~16 KiB
    command length, not `ARG_MAX`: `-l --` fails at 16380 bytes and `-H` at 8192 args, both with
    `command too long`. Above that, `load-buffer` streams over stdin with no argv limit.
37. **Multi-line input goes through `load-buffer` + `paste-buffer -d -p`.** That is the only faithful
    path: `-p` supplies the `ESC[200~`/`ESC[201~` markers *and* `paste-buffer` translates `\n` to
    `\r`, which is what a real terminal sends inside a bracketed paste. `send-keys -H` with manual
    markers produces the markers with the wrong line endings. Call `delete-buffer` explicitly on
    failure — `-d` only deletes on success.
38. **Check every exit code.** Three distinct silent failures return 0: `-l -l`, `-H zz`, and an
    unknown key name.
39. **Coalesce a keystroke run into one write.** Bubble Tea v1.3.10 has no escape timeout: a **1 ms**
    split turns `alt+a` into `esc` then `a`, and a fragmented function key injects `[12~` as typed
    text. This is the same hazard as `SPEC.md` requirement 51's "no delay between writes".
40. **Forward `C-b` normally.** `send-keys` bypasses tmux's prefix table, so interactive mode can
    deliver the prefix key where an attached user cannot without a double-tap. No leader trick is
    needed.

### Keymap, focus and chrome

41. **`Enter` enters interactive mode. `a` performs the full attach `Enter` does today. `Ctrl+Q`
    leaves interactive mode.** `Ctrl+Q` survives because Bubble Tea installs raw mode, which clears
    `IXON`; in a cooked tty it is XON and is swallowed by the tty before the application sees it.
    Record that dependency rather than treating it as luck.
42. **`Ctrl+Enter` is not bound, and the reason is recorded as a finding, not retried.** Three
    independent gates: on v1.3.10 both enhanced encodings arrive as an unexported
    `unknownCSISequenceMsg` that bubbletea explicitly does not handle further, and
    `KeyEnter == KeyCtrlM` means no `KeyType` value can represent it; tmux flattens it to `0d`
    unless the **user's own** config sets `extended-keys always` (`on` is not enough); and
    `send-keys` cannot emit it in either direction.
43. **The footer is per mode and never advertises an unbound key.** List mode gains
    `↵ interactive` and `a attach`; interactive mode advertises the exit chord and what is
    forwarded. Help gains both. Requirement: the released-help guard's list is edited in the same
    commit as each shipped verb.
44. **Focus is unmistakable and survives `NO_COLOR`.** The focused surface's border uses
    `border_focus`, the unfocused one `border`, and the sidebar's selected row uses the existing
    **`selection_idle`** token while focus is in the preview. Because `NO_COLOR` drops deck to
    monochrome — and deck's own golden frames are captured that way, so a colour-only indicator
    would pass whether or not focus moved — the preview's **top border carries the target session's
    name as text** while interactive. Assert the border colours per cell via the existing `CellAt`
    steps, and the label as screen text in a `NO_COLOR` frame. Any glyph obeys §11's
    no-East-Asian-Wide rule and has a `DECK_ASCII` fallback.
45. **Double-click enters interactive mode; single click still only selects.** A click is an idle
    gesture, and click-to-enter would resize a live agent's window on a stray movement.
46. **The panel states fitted geometry differently from a crop.** `45x22 of 120x40` means "a window
    onto something bigger"; fitted it would read `45x22 of 45x22`.

### Refusals and honesty

47. **Refuse to enter, naming the reason and offering `a`, in three cases**: another client is
    attached to that session; the preview box has fewer than **7 inner rows**; or a live process
    holds ownership. The attached-client case is a refusal rather than a warning because the squeeze
    is unavoidable — one tmux window has one size, and a bystander at 120×40 watches their agent
    collapse into a 45×22 corner for the duration.
48. **7 inner rows is the measured floor.** Against a Claude-shaped full-screen program, 41×22
    (deck's default side-by-side inner box) and 36×22 (its 40-column preview floor) are comfortable;
    41×7 is the smallest usable box; 41×6 — which is deck's **stacked height floor** of 8 panel rows
    — renders pure chrome and zero transcript. A real agent that soft-wraps its input box needs
    more, so treat 7 as a floor and say so.
49. **Say when the target has not repainted.** A frozen agent at the fitted size renders empty
    bordered rows and no echo at all — the display is byte-identical before and after typing. The
    panel must distinguish "the agent has not repainted since the resize" from a working session,
    because otherwise the most likely moment a user reaches for interactive mode is the moment it
    looks like deck is broken.
50. **Help states the two costs plainly**: entering interactive mode resizes the agent's window, and
    output produced while the window is narrow consumes scrollback faster.

### Scrollback

51. **The grid keeps its own bounded scrollback, and the wheel and `Shift+PgUp`/`PgDn` scroll it.**
    This is the only way to scroll a full-screen agent: the alternate screen has no tmux history,
    which is why tmux's own wheel binding short-circuits on `alternate_on` rather than entering
    copy-mode. Bound it, state the bound, and prove memory does not grow without limit — the grid
    already costs ~53 MiB resident for one 120×40 emulator before any scrollback.
52. **Scrolling never changes what deck reports.** Phase 3's requirement 49 established that with
    `mouse on`, scrolling an attached pane enters copy-mode and deck's probe reads panes with
    `capture-pane -p`. The same question applies here: a user scrolling the grid must not change the
    session's badge, and the grid's scroll position must not leak into the status probe.

### Scenarios that define this phase

New feature file **`features/interactive_preview.feature`**, plus re-aimed scenarios in
`preview.feature` and `mouse.feature`. Green when:

- the passive `@requirement-21` scenario is still green, unmodified;
- entering and exiting leaves the window, every option table and a fresh third-size attach exactly
  as they were, with the negative control red without the restore;
- a `SIGWINCH` count of exactly two per cycle;
- keystrokes reach the target pane and **only** the target pane, with the by-name impostor and the
  respawn cases both proved and both failing closed;
- the three refusals fire with their reasons;
- a frozen agent is announced rather than shown blank;
- focus is visible in a `NO_COLOR` frame;
- the same scenarios pass with `DECK_INTERACTIVE_TRANSPORT=capture`, except those requirement 33
  marks pipe-only.

### Evidence and stability

`ci/run.sh go test -count=1 ./...` and `ci/stability.sh 10` both clean, and
`docs/reports/phase3b.md` records: the measured `SIGWINCH` counts, the restore recipe as issued,
the seed as issued, the resident-memory cost of one grid, and every place the implementation
diverged from a spike measurement and why.

## Review guidance

- **The one question to ask of every new assertion:** would it go red if the behaviour were
  reverted? Phase 2b-2 shipped an env-override defect that survived three tests, one purely by
  fixture coincidence. Phase 3's task 011 answered this well and is the model: rather than
  asserting three named strings were absent, it scanned the whole grid for any cell in the `dimmed`
  token, which also catches a ghost of text nobody thought to name.
- **A geometry assertion that cannot see option scope proves nothing** (requirement 3).
- **The restore's negative control is not optional** (requirement 13).
- **Refusals are features.** Requirement 47's three cases are the difference between a mode that is
  honest about what it cannot do and one that silently damages a bystander's terminal.

## Findings, not spec edits

Record in `docs/reports/phase3b-findings.md`, do not fix in `SPEC.md` or `prds/`:

- any place the spikes' measurements disagree with what this PRD asserts;
- whether a **real** agent repaints its full transcript on widening. All three spikes were fenced
  from launching one, so the alternate screen's irreversible content loss — 19 of 40 rows destroyed
  when a pane shrinks, unrecovered by widening — is characterised only against synthetic programs.
  **This is the single most valuable measurement this phase can add**, and if real agents do not
  repaint, requirement 49's announcement is load-bearing rather than defensive;
- whether ~53 MiB per gridded pane is acceptable. deck has no memory budget to judge it against.

## Non-goals for this phase

- A Bubble Tea v2 migration, and therefore `Ctrl+Enter` (requirement 42).
- Grids for more than the selected session. Resident memory is per emulator.
- Reviving `SPEC.md` §11.1's send-without-attach. This mode supersedes it; deleting §11.1 is the
  operator's edit, not this phase's.
- Phase 6's scrollback *capture and replay* across restarts. This phase's scrollback is the live
  grid's own, in memory, discarded on exit.
- Anything in Phase 4 (Codex), 5 (notifications) or 7.
- A user-facing command line. The TUI remains the only surface.

## Constraints

- Commit after each completed task. Do not rewrite history.
- Do not modify `SPEC.md`, anything under `prds/`, `ci/Dockerfile`, or `ci/SPIKE.md`.
- Do all building and testing in siblings via `ci/run.sh`.
- No network dependency in the default suite: no real agent binaries, no model calls.
- **Do not weaken `features/preview.feature`'s passive guarantees to make an interactive scenario
  pass.** If they conflict, the interactive design is wrong.
- Prefer a small, readable implementation. **This phase is judged by whether interactive mode is
  provably bounded, restorable and honest — not by how good it feels.**
