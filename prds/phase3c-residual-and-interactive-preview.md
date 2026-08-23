# Phase 3c — finish Phase 3, then the interactive preview

This PRD is **two parts in one run**, in order. Part I closes what `deck-phase3` left
unfinished when it was aborted at 43/59 tasks. Part II is the interactive preview, cut
verbatim from three measured spikes.

They are one run rather than two because Part II **re-aims** several of the same assertions
Part I is still finishing — `Enter` stops being the attach key, the preview stops being
inert, the help overlay grows keys — and doing that against a half-finished Phase 3 means
writing the same scenarios twice. The composition rules below are load-bearing; read them
before planning.

## How the two parts compose

1. **Part I's requirements are numbered `I-1 … I-21`. Part II's are its own `1 … 52`.**
   Part II is reproduced verbatim from the spike PRD, so its internal cross-references stay
   as written: **inside Part II, a bare "requirement N" means Part II's N**, except where the
   text says "Phase 3's requirement N", which means a `SPEC.md`/`docs/reports/phase3.md`
   requirement. In tasks, notes, commit messages and the reports, **always write `I-n` or
   `II-n`**. A task that says "requirement 29" is ambiguous — Part I's `I-16` and Phase 3's
   requirement 29 and Part II's 29 are three different things — and will be verified against
   the wrong one.
2. **Part I lands first, in full, before any Part II product code.** The one exception is
   `I-1`, which is a prerequisite for Part II and must be first overall.
3. **Two Part I requirements are deliberately deferred to the very end of the run**, after
   Part II's code is complete, because collecting them twice is waste and collecting them
   early is a lie:
   - `I-19` (the help overlay's keymap parity and its cross-check test) — Part II adds
     `a`, `Ctrl+Q`, `Shift+PgUp`/`PgDn` and changes `Enter`'s meaning. Build the
     cross-check test in Part I; run the *parity* assertion at the end.
   - `I-20` (ten consecutive clean-state suite passes after the last code commit) — there is
     exactly one last code commit in this run and it is in Part II.
4. **`SPEC.md` is authoritative and must not be modified.** Part II's requirements assume the
   staged `SPEC.md` amendment (new §11.9, eleven other edits) has already been applied by the
   operator. **If §11 still says the preview is "never interactive" and `Enter` is the attach
   key, stop and file a finding rather than implementing Part II against a contradicting
   spec.** Check this before starting Part II, not after.

## Reporting to the operator

The operator is not watching this run's log. A `telegram-notify` skill is attached and the
container has host networking, so `http://100.71.162.65:8090` is reachable. **Use it.** It is
send-only: there is no reply channel, so never phrase a message as a question you intend to
wait on, and never block or retry in a loop on a failed POST — if it fails, note it in the
task notes and carry on. A failed notification is never a reason to fail a task.

Send a message:

- **when a task completes** — one or two lines: the requirement id (`I-7`, `II-23`), what
  landed, the commit hash, and the evidence in one clause ("10/10 isolated runs", "3 unit
  tests, red without the fix"). Not a paragraph.
- **when a task fails validation, or you fail one twice** — say which requirement, what the
  verifier objected to, and what you are going to do about it. This is the most useful
  message you can send and the one most likely to save the run.
- **when you discover something that changes the plan** — a measurement that contradicts this
  PRD, a `SPEC.md` conflict, a requirement you judge unmeetable, a defect outside this run's
  scope. Say so when you find it, not in the final report.
- **at each of these milestones**: Part I complete; the `SPEC.md` §11.9 check of composition
  rule 4 passing or failing; Part II's first end-to-end enter/exit cycle working; `I-20`'s
  stability result whatever it is.
- **with the artifact, not a description of it**, when the news is a file: `POST /send` with
  `file=@` for a failing log, a measurement log, or a stability run. The operator would
  rather read the log than your summary of it.

Do **not** send a message per iteration, per commit within a task, or to report that you are
starting something. Roughly one message per completed task plus one per surprise is the
intended volume.

---

# Part I — the Phase 3 residual

## What happened, factually

`deck-phase3` ran 20 h, two approaches, 130 iterations, and was **aborted by the operator**
with verdict `unverified`. It completed **43 of 59** tasks. Nothing is wrong with the work it
did land; it ran out of wall clock. Its own records are the starting point and they are
unusually honest — read them rather than re-deriving:

- `docs/reports/phase3.md` — the per-requirement delivery table. 30 `DONE`, 5 `PARTIAL`,
  13 `PENDING` **as written**, and two of those rows are stale in the *conservative*
  direction (see `I-2`).
- `docs/reports/phase3-findings.md` — every gotcha the run paid for. The `@real-agents`
  tag-substring trap, the workspace-grouping fixture trap, the badge-truncation trap.
- `~/.ralphd/runs/deck-phase3/notes.md` — the plan's own running commentary.

## The requirement everything else serves

Unchanged from Phase 3: **`SPEC.md` §9.2's R1 — every destructive path carries a
working-directory fingerprint assertion.** Phase 3 delivered eleven such scenarios. What it
could not deliver is proof that three of them *execute*, which is `I-1`.

## Requirements

### I-1. Root-cause the single-keystroke drop, and prove which layer loses it

**This is the first task of the run and it blocks Part II, not merely Part I.**

Phase 3's last act was to measure the three marked-set fingerprint scenarios after the
coalesced-`KeyMsg` fix (`465a7d9`) landed. Measured on a 28-core host at load average
6.18–7.28, ten consecutive isolated runs each
(`docs/reports/phase3-task134-residual-measurement.log`):

| scenario | passed |
|---|---|
| `@requirement-29-bulk-kill` | **3/10** |
| `@requirement-29-bulk-delete` | **5/10** |
| `@requirement-29-batch-undo` | **8/10** |

The failure signature is identical in all three: a single, **non-coalesced** `j` sent between
two `m` keystrokes never takes effect, the selection stays on the first marked row, and the
poll-on-render checkpoint times out at 5 s with the cursor marker still on the wrong row. The
run concluded this is "a harness/PTY-delivery characteristic of this host, not a product
defect" and skipped the task with the requirement-29 row honestly downgraded to `PARTIAL`.

**That conclusion is unproven and this requirement is to prove or refute it.** Nobody has
established where the byte goes. The candidate layers, each of which implies a different fix
and one of which is a serious product defect:

1. the harness's own `Send` write never reaching the pty;
2. the pty/kernel buffer (implausible — pty writes do not drop bytes, so proving this is
   *not* it is cheap and worth doing first to eliminate it);
3. Bubble Tea v1.3.10's input reader losing a byte under scheduling pressure;
4. deck's own `Model.Update` dispatch discarding the message — **a real product defect**, and
   the same class of bug as requirement 51, which was found exactly this way;
5. the render never happening within 5 s (i.e. the keystroke *was* processed and the
   assertion is simply too fast) — the frame capture at timeout argues against this, but it
   has not been excluded by instrument.

**The instrument already exists and Phase 3 invented it.** Task 114 fixed a different flake by
statting a fixture for its exact byte length and polling the driver's accumulated raw byte
count (`waitForFixtureFullyRendered`). Apply the same idea to input: count what the *program*
received, so "delivered but not acted on" and "never delivered" stop being the same
observation. That is an instrument, not a widened timeout, and it is explicitly not forbidden
by the fences below.

Done when: the layer is identified by measurement; if it is layer 4 it is **fixed** and the
three scenarios return to 10/10; if it is layer 1 or 3 the harness is fixed and they return
to 10/10; if it is genuinely layer 2 or 5, the finding says so with the evidence and
`I-2`'s requirement-29 row states which paths remain unproven and why. **Do not close this by
lengthening a sleep, widening the 5 s checkpoint, adding a retry, tagging a scenario
`@flaky`/`@nightly`, removing an intermediate checkpoint, shrinking the marked set, marking by
any route other than real keystrokes, or deleting a scenario.**

Why it blocks Part II: Part II is *keyboard forwarding*. Every one of its input scenarios
sends keystrokes and asserts a pane's reaction. A suite that silently loses one keystroke in
three under ordinary load cannot evidence that phase at all, and `II-39`'s 1 ms coalescing
window makes deck's own input path load-sensitive by design.

### I-2. Correct the two stale rows in the delivery table, and only those

`docs/reports/phase3.md` understates what was delivered, in two places, because the commits
that satisfied them did not update the table:

- **requirement 51** reads `PENDING | Task 118.` — but task 118 landed as `465a7d9` with
  three `Model.Update` unit tests and a no-delay PTY proof. Fill the row from that commit.
- **requirement 43** reads `PENDING | File does not exist yet.` — `features/kill_delete_undo.feature`
  exists and holds thirteen scenarios. Fill the row from what is in it.

Correct nothing else in that table from memory. Every other row is either honest or is a
requirement in this list.

### I-3. Land task 119's uncommitted work

The abort caught task 119 mid-flight with its work complete in the tree but uncommitted:
seven modified files (`internal/tui/tui.go`, `internal/tui/group_test.go`,
`features/attention_sort.feature`, both guard-word test files, `docs/reports/phase3.md`,
`docs/reports/phase3-findings.md`), 194 insertions, and requirement 37's row already filled
in with its evidence. It rebinds group collapse from `g` to `c` and makes `g`/`G` jump to the
first/last visible row.

**Verify it before trusting it** — `ci/run.sh go build ./...`, `go vet ./...`, `gofmt -l`, the
`internal/tui` tests, and the two `attention_sort.feature` collapse scenarios plus the new
`@requirement-30-top-bottom` — then commit it as one commit for requirement 37. If a piece is
missing or red, finish it; do not discard the diff and start over. Nothing else in the run may
be committed until this is resolved, because every later `git status` assertion is meaningless
against a tree that is already dirty.

Note for `I-19`: the run recorded that `c` is **not** in `SPEC.md`'s own keymap list. That is
an operator-owed `SPEC.md` fold-in, not a change to make here. Keep it as a finding.

### I-4. `[ui] group_by_workspace`, declared once (requirement 34)

Phase 3's `SPEC.md` amendment is already applied for this: §11 states grouping is optional via
`[ui] group_by_workspace` (default `true`) and §6.5's `[ui]` row lists the key. Declare it once
in `internal/config/schema.go` with its `DECK_GROUP_BY_WORKSPACE` override, resolved exactly as
`ui.ascii`/`DECK_ASCII` and `tmux_mouse`/`DECK_TMUX_MOUSE` already are, recorded in
`Settings.EnvOverrides`, surfaced by `settingsCategories()`, and edited in settings with an
explicit save — **a keypress must never rewrite `config.toml`** (§6.5). Task 115's commit
(`64e6456`) is the model to copy; the schema parity tests will tell you if you declared it
twice.

### I-5. The flat, header-free sidebar (requirement 35)

With grouping off the sidebar is one flat list in §11's sort order with **no header rows**,
which is a different row budget for §11.2's page-size and elision arithmetic. Collapse state is
meaningless in flat mode and must be **absent rather than inert**. Prove the page-size and
elision maths in *both* modes, not just the one you developed in.

### I-6. Navigation is identical in both grouping modes (requirement 36)

Including the regression that motivated the requirement: two sessions in the **same**
workspace that are **not adjacent** in the flat sort order. Grouped, they sit together under
one header; flat, they do not. `↑`/`↓`/`g`/`G`/`space` must visit rows in the same visual order
the mode renders, with no row unreachable and no row visited twice.

Read `phase3-findings.md`'s task 113 section before writing the fixture: two scratch
directories with different labels land in **two** sidebar groups unless both labels end in the
same leaf path component, which is exactly the trap this requirement's fixture is most likely
to fall into.

### I-7. Mask secret-shaped values everywhere, with reveal (requirement 21)

Secret-shaped env values are masked in every view that can render them — the `e` env editor,
the `i` detail dialog, the `E` event log (`I-9`), and any settings surface — with an explicit
reveal toggle, and **no value ever reaches disk**: not the JSONL log, not the launch audit, not
the store, not a crash tail.

**The negative assertions here are the ones most likely to pass by coincidence.** Do not assert
that three named strings are absent. Do what Phase 3's task 011 did and the Phase 3b PRD holds
up as the model: scan the whole rendered grid for any cell carrying the value, and grep every
file the run wrote for the value, so a ghost nobody thought to name is also caught. Then prove
the instrument works by asserting an *unmasked* control value **is** found by the same scan.

**Watch item, carried from Phase 3's oversight:** if any task widens the env editor's key set to
include the server environment, masking inverts into a real leak. Say explicitly in the report
which key sets are in scope.

### I-8. Rename inside the `i` detail dialog only (requirement 31)

Rename is an action *inside* the `i` dialog, not a top-level key. The **tmux session name does
not change** — deck's name and tmux's name are decoupled — and the dialog states that on
screen rather than leaving the user to discover it. Obeys §11.4's dialog contract (`esc`
cancels, mouse can neither cancel nor confirm, 80% width clamped to `[26,80]`) asserted the way
`features/dialogs.feature` already asserts the existing dialogs, and the new dialog flag joins
every open-dialog gating list in `internal/tui`.

### I-9. The `E` event log view (requirement 32)

Newest first, each row showing kind, reason and a **bounded** payload, with env values masked
per `I-7`. Bounded means proven bounded: assert a long payload is truncated, not merely that a
short one fits.

### I-10. The `/` list filter (requirement 33)

Filters by name, workspace and cwd. Must also answer **how archived rows are reached** — Phase
3 shipped archive as a flag (`ecb2d8a`) and archived rows are hidden from the default list, so
the filter is the only route to them and the requirement is not met by filtering the visible
set alone.

### I-11. The store durability contract, decided and proven (requirement 40)

Decide it, state it in `SPEC.md` terms without editing `SPEC.md`, and prove it. "Proven" means a
test that kills the process at the dangerous moment and asserts what survived — not a comment
asserting a journal mode.

### I-12. The launch audit's argv and env-key records for restart (requirement 10)

The row is `PARTIAL`: create and resume are covered, restart is not. `R` restart must record
exact argv and env **key names, never a value** — same assertion shape as the create/resume
scenarios in `features/agent_session.feature`, extended to the restart path, with `I-7`'s
whole-file scan applied to the audit.

### I-13. Complete `features/create_session.feature`'s stated coverage (requirement 41)

The row lists what is covered and what is not. Close the named gaps — `pre_launch` and the
§11.7 recent-cwd interactions are the ones the row calls out — rather than adding scenarios for
what is already green.

### I-14. Complete `features/environment.feature`'s stated coverage (requirement 42)

Same treatment: masking and no-leak are the gaps, and they are `I-7`'s assertions applied in
that file. Do not duplicate `I-7`; cross-reference it.

### I-15. Finish requirement 30's frame-budget proof

The row is `PARTIAL`: the undo toast is proven inside the 80×24 budget in all four layout modes;
the other transient messages this phase added (the pending-delete indicator, the archive and
purge confirmations, the refusal messages) are not. Every transient message counts in the
reserved-row arithmetic, in every layout mode, at exactly 80×24.

### I-16. Requirement 29's residual, resolved by `I-1`'s outcome

If `I-1` fixes the drop, restore the requirement-29 row to `DONE` with the new 10/10
measurement and re-open task 113's substance: all six of `purge`, `archive`, `kill-and-archive`,
`bulk-kill`, `bulk-delete` and `batch-undo` must **reach and execute** their fingerprint
assertion. Prove the assertion can still fail by re-using task 003's four mutation modes on two
of them — a green run does not distinguish "the assertion passed" from "the assertion was a
no-op".

If `I-1` establishes the loss is genuinely environmental, the row stays `PARTIAL` and states
which paths are unproven, with the measurement. **It may not read `DONE` while any of the six
cannot reliably reach its assertion.**

### I-17. Audit `docs/reports/phase3-findings.md` for completeness

Phase 3's task 129, unstarted. Every gotcha the run paid for is in there or is added. Two known
defects to fix while you are in it, queued since 22 Aug and never applied: Phase 3's PRD
requirement 3 names only four mutation modes where requirement 29 treats modes as first-class,
and requirement 3 says "requirement 27" where it means 29. Those are `prds/` edits — **record
them as findings for the operator, do not edit `prds/`.**

### I-18. Complete `docs/reports/phase3.md`

No `PENDING` row left in the delivery table without either real evidence or an explicit "not
delivered, because" naming the reason. A fresh full-suite run is cited. This is the row-by-row
audit Phase 3's task 130 never reached, and `I-2` is not a substitute for it.

### I-19. Help overlay parity and the cross-check test (requirements 38, 39)

Build the cross-check test in Part I: it compares the help overlay's stated keymap against the
keys actually bound and **fails in either direction** — a bound key the help omits, and a help
line for a key that is not bound. Run the *parity* assertion at the end of the run, after Part
II's keys exist (see composition rule 3). The overlay stays inside the frame budget at 80×24.

The released-help guard in `cmd/deck/main_test.go` and `internal/tui/tui_test.go` moves each new
key from the unavailable-word list into the present-phrase list **in the same commit that ships
the key**. Steer 001 established this in Phase 3; do not delete a guard entry to turn a test
green.

### I-20. Ten consecutive clean-state suite passes after the last code commit (requirement 46)

Deferred to the end of the run by composition rule 3. `ci/stability.sh 10`, collected after the
final code commit of Part II, from a clean state.

**If the bar is missed, publish the measured rate, the runs, the failing scenarios and the host
load correlation. Do not manufacture 10/10** by re-running until a clean streak appears, by
excluding a scenario, or by narrowing `defaultTags` in `features/godog_test.go` (which is
`"~@real-agents && ~@nightly"` and stays that way).

### I-21. Final hygiene

Green `ci/run.sh go build ./...`, `go vet ./...`, `gofmt -l` clean, green suite, clean tree,
everything pushed. `git status --short` empty and `git log origin/main..HEAD` empty.

## Part I working practice

These are the disciplines Phase 3 established the hard way. They cost it three burnt validation
attempts and a whole extra task; do not rediscover them.

- **Commit *before* flipping a task to `completed`.** Phase 3's task 105 was functionally correct
  on the first attempt and failed validation three times purely because the diff sat
  uncommitted. Steer 004.
- **One task per commit, and the help/guard-word update lands in the same commit as the key.**
- **`ci/run.sh` does not forward `-e` into the sibling container.** `DECK_GODOG_TAGS` must be set
  *inside* the container:
  `ci/run.sh sh -c 'DECK_GODOG_TAGS="@tag" go test -count=1 -run TestFeatures ./features/'`.
- **Never put `@real-agents` in a `DECK_GODOG_TAGS` expression, even negated.** The gate is a
  substring match and `~@real-agents` cascades a real-Claude trust-file permission error across
  every claude-session-creating scenario in the run.
- **Ask of every negative assertion: would this go red if the fix were reverted?** Phase 2b-2
  shipped a defect that survived three tests, one purely by fixture coincidence.
- **`features/godog_test.go`'s `defaultTags` is `"~@real-agents && ~@nightly"`.** Any suite result,
  including your own, means nothing until you have read that line.

---

# Part II — the interactive preview

Part II is `~/deck-spikes/staged/phase3b-interactive-preview.md`, unchanged, its requirements
renumbered `II-1 … II-52`. It is reproduced in full below.

Its four operator decisions (`Ctrl+Q` semantics, `a` versus `Ctrl+Enter`, single versus
double click, and border weight) are settled as staged unless the launch note says otherwise:
`Ctrl+Q` **leaves** interactive mode and returns to the list, `a` is the full attach,
**double**-click enters interactive mode while single click still only selects, and focus is
shown by colour plus a text label rather than a heavier border.

## Part II scope note

Everything from here to the end of this document is the spike PRD verbatim. Its "Goal",
"Context", "Constraints" and "Non-goals" sections describe **Part II only**. Where they
conflict with Part I's working practice, Part I's is the same or stricter, so follow Part I.
Two clarifications that Part II's verbatim text cannot know:

- Its "Non-goals" say "Anything in Phase 4 (Codex), 5 (notifications) or 7." That still holds.
  It does **not** exclude Part I, which is this run's first half.
- Its constraint "Do not modify `SPEC.md`, anything under `prds/`, `ci/Dockerfile`, or
  `ci/SPIKE.md`" applies to the whole run, both parts.

---

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

