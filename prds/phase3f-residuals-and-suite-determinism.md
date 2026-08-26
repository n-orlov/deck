# Phase 3f — field defects, residuals, and suite determinism

## Goal

Close the operator's bug log. **Eleven requirements in two halves.**

*The residual half (R63–R67), written first.* Two residual product defects Phase 3e found,
root-caused and deliberately left; two unsound test-assertion **classes** that are the entire
reason `ci/stability.sh 10` publishes 7/10 instead of 10/10; and one artefact-findability sweep.

*The field half (R68–R73), added after a day of daily-driver use produced six new defects*, each
filed with a live reproduction and a root cause read off this same tip. These are not polish. One of
them **hangs the whole TUI** with no keyboard escape and no response to `SIGTERM`; two of them
produce a permanently wedged session no key can recover; one **kills a live agent on a single
unconfirmed keystroke next to the attach key**, which is how the operator lost a working session
while this PRD was being written. Every one was found by using the product, not by reading it —
which is the strongest argument in this document for why they go in before the §14.2 spike rather
than after.

This is no longer the small insert phase the residual half was scoped as. It now touches
`internal/interactive`, `internal/service`, `internal/store`, `internal/hookrecv` and
`internal/tui`; it adds one dialog, one keybinding and one store method; and — unlike the residual
half — **it needed a companion `SPEC.md` amendment, which the operator has already landed in
`c80a14c`.** See the next section: the dependency is discharged, not pending.

**The operator has decided the scope: all eleven requirements are in this phase.** A 3f/3g split at
the R67/R68 seam was considered and declined, so the orderings under Deliverables apply in full and
no part of this PRD is deferred.

## Read this before anything else: the `SPEC.md` amendment has LANDED, and it is still not yours to write

**`SPEC.md` is the authoritative product spec and must not be modified by this job.** Where this
PRD and `SPEC.md` disagree, `SPEC.md` wins and the disagreement is a finding.

The residual half is clean, exactly as originally cut: R63, R64, R65 and R67 are defect fixes and
harness work the spec never spoke to, and R66 makes a *stricter* property true than §11.6 demands
without adding or removing a rule.

**The field half was not.** Two of its requirements change specified behaviour, and one of them
contradicted a scenario §13.5 explicitly requires — so the operator amended the spec first, in its
own commit, exactly as in `6584299` before Phase 3e:

| requirement | what it contradicted, before `c80a14c` | how the amendment resolved it |
|---|---|---|
| R71 (archived rows are not startable; add an unarchive) | `SPEC.md:323-325`, `:498`, `:730`, `:1057-1062` **at the pre-amendment tip**; and `internal/store/store.go:1201-1205`/`:1256-1262`, which still document "an archived row has no restore at all" **as intended design** | `archived_at` is now reversible *and* an archived row is not startable — the spec says both halves are needed, since a one-way flag plus a resumable archived row is the combination that wedges. **The two store doc comments are now the ones contradicting `SPEC.md`, and they are part of your change** |
| R72 (`A` requires an explicit confirmation) | **`SPEC.md:1864` at the pre-amendment tip named "archiving refused for a live session" as a required scenario** — the opposite of the confirm-then-kill the operator chose | that scenario line is replaced. The spec was already self-inconsistent here: §4 said archiving "requires `stopped`" while *offering* kill-and-archive, §13.5 said refuse outright, and the code did a third thing (killed without asking) |

**Read the amended spec, not this table's history.** Every `SPEC.md` line number in this PRD is
given at the **post-amendment** tip (`c80a14c`), except where a row like the one above explicitly
says otherwise. The amendment also went one sentence beyond what R71 and R72 needed — see R70.

**Consequences you must honour:**

- **The recognised protected-path sha set for this phase is exactly TWO shas:** `c80a14c` (the
  `SPEC.md` amendment) and this PRD's own commit. Any *third* sha touching a protected path is a hard
  failure to report, not to classify. (The original cut said "exactly this PRD's own commit"; that
  sentence is superseded by this one.)
- **The amendment has landed, so nothing in this phase is blocked on it.** All eleven requirements
  are startable. Do not re-derive the amendment, do not "improve" it, and do not treat its content as
  open: where it and this PRD disagree, `SPEC.md` wins and the disagreement is a finding.
- **For every other requirement, the original rule stands unchanged: if you find yourself wanting to
  edit `SPEC.md`, you have misread a requirement.** File a finding instead. R68, R69 and R70 are pure
  defect fixes that *restore* invariants the spec already states — R69 restores `SPEC.md:547`
  verbatim — and none of them licenses touching it.

The rest of the protected-path rule is unchanged and still applies to `SPEC.md`, `prds/`,
`ci/Dockerfile` and `ci/SPIKE.md`: at close-out, verify by **sha**, never by author or committer
identity (they are byte-identical strings — see `docs/reports/phase3d-212-closeout/README.md`, which
got this right and is the model to copy), and treat any unrecognised sha in that range as a hard
failure to report rather than something to classify.

## Where work happens

Unchanged from Phase 3/3b/3c/3d/3e: the job container has no Go and no tmux and cannot install
them. **All building and testing happens in the sibling toolchain container via `ci/run.sh`.**

## Most of the operator's bug log is already closed — do not write requirements for these

Seven items were on the operator's list. **Five of them are already fixed in the tree**, verified
against it before this PRD was cut. They are recorded here so that no task re-derives them, no
requirement is written for them, and no report claims credit for them:

| Item | Disposition, verified |
|---|---|
| `probe.miss` grows unboundedly and wedges the `E` event log | **Closed by R59** (`docs/reports/phase3e.md:741`, task 329) — `probe.miss` is no longer written as an event |
| A coalesced `KeyMsg` drops **both** runes (`"jm"` matches no case) | **Closed by task 118 / requirement 51** — `internal/tui/tui.go:1830-1863` splits a multi-rune `KeyRunes` message into one single-rune message per character and dispatches each through `Update` |
| A pasted `"dd"` could be dispatched as a delete chord | **Never exposed.** The split at `tui.go:1831` is guarded by `!msg.Paste`, and bubbletea v1.3.10 enables bracketed paste by default (`cmd/deck/main.go:121` passes no `WithoutBracketedPaste`), so a real paste carries `Paste: true` and is ignored outright rather than split |
| `crash.feature`'s SIGKILL scenario hangs in its after-scenario hook, ~1 run in 10-30 | **Closed by the same task 118 fix.** It was root-caused in `docs/reports/phase2b2-findings.md:1249` to exactly the coalesced-keystroke mechanism — a coalesced `"iq"` matched neither `case "i":` nor `case "q":`, so the client never received `tea.Quit`. It has not recurred in any of the 20 `stability.sh` runs since |
| `harness.feature`'s "a fake agent renders a preview fixture once and then falls silent#01" | **Closed by `17e91ce`** (task 114, read-buffer race) |
| `internal/interactive`'s `TestSessionResizeDuringLiveDrainIsRaceFree` goroutine-outlives-test panic | **Closed by `fb9bd71`** — a real `sync.WaitGroup` join |
| `phase3e-findings.md` §4a, §4c | **Closed** by tasks 402/403 and 334 respectively |

**If you believe one of these is not actually fixed, that is a finding worth a report** — but prove
it with a reproduction, not with a reading of a stale document. The operator's own notes on several
of these predate the fixes and are stale; that is why this table exists.

## The field half: six defects found by using the product, all filed with live reproductions

Do not confuse this list with the one above. That table is Phase 3e's bug log, mostly already fixed.
**This one is new** — six defects the operator hit on their daily driver on 26 Aug 2026, each
investigated against a live process or a live store, each filed to GitHub with its evidence:

| issue | requirement | severity, stated plainly |
|---|---|---|
| [#5](https://github.com/n-orlov/deck/issues/5) — interactive preview deadlocks on a terminal query | **R68** | **worst in the list.** Whole TUI wedged, `q`/`Ctrl+C` dead, `SIGTERM` ignored; recovery is `SIGKILL` from another terminal. Reproduced twice |
| [#6](https://github.com/n-orlov/deck/issues/6) — a retained dead pane wedges kill, resume and restart | **R69** | session permanently unrecoverable from the UI; violates `SPEC.md:547` verbatim, including the consequence the spec predicts |
| [#9](https://github.com/n-orlov/deck/issues/9) — `pane_exit_status` is never cleared | **R70** | one crash removes a session from reconciliation **forever**; three live rows affected, one displaying a wrong status for 30+ hours |
| [#8](https://github.com/n-orlov/deck/issues/8) — an archived session is startable | **R71** | a live agent hidden behind `/`, its hooks orphaned, its status frozen, and no in-app way back |
| [#10](https://github.com/n-orlov/deck/issues/10) — `A` kills a live agent on one unconfirmed keystroke | **R72** | **this is how the operator lost a working session.** `A` is `Shift`+`a`, and `a` is attach |
| [#7](https://github.com/n-orlov/deck/issues/7) — scrollable overlays only page, via `PgUp`/`PgDn` | **R73** | ergonomics only, and the only item here that harms nothing but patience |

Two things about this evidence you need to know before you start:

**The reproductions are real, and two are stronger than a test could be.** #5 was root-caused by
attaching `gdb` to the hung process: 51 goroutines, all parked, with the stuck writer and the wedged
`View()` on the same `Session` pointer. #8's own hook error was captured from the pane, naming both
resolution keys and proving both were correct. Where a requirement below cites a goroutine stack or a
store row, it is transcribed, not reconstructed.

**The operator's live store has already been repaired by hand, so do not use it as a fixture.** To
recover the session, `archived_at`, `pane_exit_status` and `crash_tail` were cleared directly on the
`deck-dev` row with a `sqlite3` `UPDATE` outside deck. That recovery is itself evidence — orphan
events stopped immediately, a `probe.idle` landed three minutes later, and the status moved off
`starting` on its own, which is what proves R70 and R71 were the *whole* freeze and not two of three
causes. But it means the wedged state is no longer present on that box. **Build your fixtures from
the descriptions here, not from the operator's `state.db`.**

**One correction already published against these issues, so you do not repeat it.** A stale
`sessions.last_probe_at` was originally cited as evidence that the reconciler had skipped a row. It
is not: `RecordProbeMiss` (`internal/store/store.go:811`) is its only writer, so a stale value means
only that no probe **miss** was recorded — equally true of a perfectly healthy session. Both issues
were corrected. If you need to show a row was skipped by reconciliation, show the control flow or
show `status_at`, never `last_probe_at`.

## What already exists — do not rebuild it

Every root cause below was read off the tip this PRD is written against (`bce80ea`, working tree
clean, `git log origin/main..HEAD` empty). **They were read off the live tree, not guessed.** Verify
each before you fix it — and if a root cause here is wrong, say so in a report and fix the real one;
a PRD's diagnosis is evidence, not scripture.

Reusable machinery that already exists and must not be duplicated:

- **`ScreenDriver.WaitForQuiescence`** (`features/pty_driver_test.go:530`) blocks until a stated
  quiet window elapses with no new PTY output, and already has a production caller
  (`features/mouse_synthesis_test.go:226`, with `captureSettledQuietWindow`). **R65 is this helper
  applied to a step that currently samples instead of settling.** Do not write a second waiter.
- **The quantisation pipeline** — `internal/theme/quantize.go`'s `ReferencePalette` (the 16 fixed
  xterm values, declared in code because terminals disagree) and `quantize()` (nearest by Euclidean
  RGB distance, ties to the lower index). R66 needs no new machinery, only different authored hexes.
- **`TestBuiltinQuantizationPinned`** (`internal/theme/quantize_test.go:58`) pins every token of
  every built-in to its quantised value. Its own doc comment states the contract R66 relies on: *"A
  change to a built-in's authored colours, or to the quantisation method, must be reconciled here
  deliberately."* Updating it for R66 is **sanctioned reconciliation, not a weakened test** — see
  R66's own criteria for the line between the two.
- **`TestMatrixStatusTokensRenderAsSevenDistinctColours`**
  (`internal/tui/matrix_status_tokens_test.go:52`, task 317) already proves seven-way distinctness
  on the **true-colour** path. R66 is its missing sibling for the quantised path; the existing test
  must keep passing unchanged.
- **`DECK_COLOR_DEPTH=truecolor|16`** (`SPEC.md` §13.1) forces the render path, which is what makes
  quantised rendering assertable from a pty test at all.

For the field half:

- **The delete-confirm overlay is R72's entire template.** `m.deleteConfirming` and its dialog,
  reached by the `dd` chord, already solve "a destructive action must be confirmed" including the
  dialog contract and the suppression of the bare-letter keymap while open. **R72 is that shape
  applied to `A`.** Do not invent a second confirm mechanism.
- **Two undo trios already exist and are the template for R72's toast**: `undoSessionID` /
  `undoSessionName` / `undoGeneration` (`internal/tui/tui.go:226-234`, requirement 22 — `x`'s undo,
  bounded by `DECK_UNDO_MS`), and `deleteUndoSessionID` / `deleteUndoSessionName` /
  `deleteUndoGeneration` (`:243-245`, task 106 — `dd`'s). `u` checks the kill trio first and falls
  through. A third one for archive follows an established pattern; a novel one is a finding against
  you.
- **`Store.RestoreSession`** (`internal/store/store.go:1302`) **is the precedent R71's unarchive must
  mirror.** It is what `deleted_at` has and `archived_at` does not, and the asymmetry is the defect.
  Read it before writing `UnarchiveSession`; the shapes should be recognisably siblings.
- **`Store.ListArchivedSessions`** (`internal/store/store.go:1263`), **`Model.loadArchivedSessions`**
  (`internal/tui/tui.go:1206`) and the filter's own merge (`internal/tui/filter.go:76`, archived rows
  appended after live ones so a surfaced archived match never displaces a live row) already implement
  requirement 33's route to an archived row. **R71 adds an action reachable from there; it does not
  add a view.**
- **`framedDialogScrollable`** (`internal/tui/panel.go:864`), **`dialogScrollBy`** (`:841`),
  **`dialogContentBudget`** (`:813`) and **`dialogMaxScroll`** (`:827`) are R73's whole surface. The
  bounding and clamping already work; what is missing is a *step size*. Add a parameter, do not write
  a second scroller.
- **`crashedPane` and `crashTail`** (`internal/service/reconcile.go:70`, `:199`) already do R69's
  detection and capture. R69 changes *when the branch is reached*, not what it does once inside.
- **`interactive.Session`, `Grid = vt.SafeEmulator` and `PanePipe`** (`internal/interactive/grid.go:66`,
  `:93`; `internal/tmux/pipe.go`) are R68's subject. Note `PanePipe`'s doc comments record several
  hard-won facts about the FIFO's blocking behaviour that R68 must not disturb — read them before
  touching the drain loop.
- **`clientArchivesSelectedSession`** (`features/kill_delete_undo_test.go`, registered at `:43`)
  sends a real `A` through the pty and waits for the row to leave the frame. **R72 breaks this step
  and its two callers** (`features/event_log.feature:15`, `features/filter.feature:46`) — see R72's
  criteria, because *how* you repair it is the difference between testing the confirm and bypassing
  it.

---

# The requirements

Numbering continues the codebase's own: **R62 was the last** (Phase 3e delivered R52-R62 — R52-R58
from its PRD, R59-R62 added by steer `3e-001`). These are **63-73**: R63-R67 the residual half,
R68-R73 the field half.

**Four orderings below are load-bearing.** Each is restated at the requirement that depends on it,
and all four appear again under Deliverables:

- **R63 before R65.** Same debounce/reconcile family, and R63 is a **candidate root cause** of the
  flake R65 exists to make impossible. Doing R65 first destroys that information permanently, because
  a settled assertion cannot observe the race it was hiding.
- **R69 before R70.** They edit **the same line** — `internal/service/reconcile.go:61` — for two
  different reasons. Sequenced, the second lands on a guard the first has already rewritten and
  commented. Done in parallel or in the other order, you get a merge conflict in the one place in the
  codebase where a careless resolution silently restores a wedged session.
- **R71 before R72.** R72's undo toast is only honest if there is something to undo, and that is
  R71's `UnarchiveSession`. Doing R72 first means either shipping a toast that lies or shipping the
  confirm without one and revisiting it.
- **R68 first of the field half, ideally first overall.** It is the only defect in this document that
  makes deck unusable rather than wrong, and it is the one an operator cannot work around. If the
  phase is cut short, this is the requirement that must have landed.

## R63 — a passive preview fit can never overlap itself for the same session

**Source:** `docs/reports/phase3e-findings.md` §4e, found by task 406 while root-causing the
whole-suite-only `DECK_MOUSE=0` frame race (`docs/reports/phase3e-406-mouse-off-frame-race/
README.md` §7, "Residual risk"). Recorded there as a *genuine, if narrow, timing hazard in the
product code itself, not just the test harness*, and explicitly left for a future task.

**Root cause, as read off the tree.** `Model.previewFit` (`internal/tui/tui.go:1270-1313`) decides
whether a fit is needed by comparing the selected session against a single field:

```go
session := m.sessions[m.selected]
if session.ID == m.previewFitSessionID {
    return nil
}
```

`previewFitSessionID` is written in exactly one place — the `previewFitDone` case
(`internal/tui/tui.go:1808-1809`) — i.e. only when a fit has **already completed**. The
`previewTick` case (`tui.go:1790`) calls `m.previewFit()` on every tick (`tui.go:1804`), coalesced
against that tick per steer 018 item 4. So between the moment a fit is *scheduled* and the moment its
`previewFitDone` lands, the guard still reads the **old** `previewFitSessionID`, and the next tick
schedules a **second** `previewFit()` `tea.Cmd` for the same session. Nothing anywhere prevents it.
Two overlapping `FitWindowToPane` calls then run for one session.

Task 406 reproduced it deliberately at an artificially large single `resize-window` delay (0.5s,
double the 250ms reconcile interval), hitting it twice in 21 runs. At production cadence
(`DefaultPreviewMS` = 250ms, `DefaultReconcileMS` = 500ms, `internal/config/config.go:23-24`) the
window is much narrower — which is why this is a hardening requirement, not an outage.

**One trap already checked for you, so you do not have to fear it.** Every `return` inside the
closure `previewFit` hands back emits `previewFitDone{sessionID}` — the no-live-pane path
(`tui.go:1299`), the `tmux.SessionName` error path (`:1303`) and the success path (`:1311`). So a
completion message always arrives, and clearing an in-flight marker on `previewFitDone` cannot
wedge a session whose pane is dead. Confirm this yourself before relying on it; if you add a new
early return, it owes a `previewFitDone` too, and that is now an invariant worth a comment.

**What to build.** An in-flight marker set when a fit is *scheduled* and cleared when its
completion lands — a generation counter or an in-flight session id alongside `previewFitSessionID`,
whichever reads better — with `previewFit`'s guard consulting both "already fitted" and "a fit is
in flight". The two questions are genuinely different and one field cannot answer both; that is the
whole defect.

**Success criteria.**
- A unit test drives `Model.Update` directly — no pty, no tmux, no load — feeding two
  `previewTick` messages back to back with **no intervening `previewFitDone`**, and asserts the
  second produces **no** fit command. Assert on the returned `tea.Cmd`, not on a model field, so
  the test cannot pass against a model that records intent and issues the command anyway.
- The paired negative: the same two ticks **with** the first's `previewFitDone` delivered in
  between, on a selection that has since changed, **do** produce a second fit. A fix that simply
  suppresses all subsequent fits passes the first criterion and fails this one — that is the sloppy
  version of this change and this is the criterion that catches it.
- A completion for a **stale** session (a `previewFitDone` whose `sessionID` is no longer selected)
  clears the in-flight marker without wedging the next legitimate fit. Selection changes during an
  in-flight fit are ordinary, not exceptional.
- Revert the fix and show the first criterion's test red. Per non-negotiable 4, do it by actually
  reverting and reproducing, and say so in the report.
- `features/preview.feature` and `features/interactive_sigwinch_budget.feature` pass unmodified by
  this requirement. R63 is a fix to *when* a fit is scheduled, and must not change how many
  SIGWINCHes a settled, uncontended run produces. **If a SIGWINCH-count expectation has to change
  to accommodate R63, stop** — either the count was wrong before, or R63 is over-suppressing, and
  either way it is a finding to report before you touch the number.

## R64 — no scenario asserts a state the product is permitted to skip

**Source:** the top failure mechanism of the current stability evidence —
`docs/reports/phase3e-408-stability-10-at-75861e0/README.md` item 1, failing **two of ten runs**
(runs 4 and 10) — and the same mechanism already root-caused a phase earlier in
`docs/reports/phase3d-210-ghost-completion-starting-race.md` for a sibling scenario in the same
file.

**Root cause, as read off the tree.** `SPEC.md` §7's shell-only fast-forward rule promotes a
`shell` session from `starting` to `running` the moment its pane is alive
(`internal/service/reconcile.go:75`'s `session.Agent == "shell" && session.Status == "starting"`
promotion), and the reconcile tick under test is 250ms (`scenarioReconcileInterval`). So for a
shell session, `starting` is a state the product is **explicitly allowed to pass through without
ever rendering**. Under host contention that window closes entirely and the row goes from absent
straight to `running`.

Two scenarios assert it anyway, as a **waypoint rather than as the thing under test**:

- `features/create_cwd_ghost.feature:26` (`@requirement-14-ghost-unique-match-right-accepts`)
- `features/create_cwd_ghost.feature:91` (the tilde-expansion scenario at `:79`)

In both, the line after the `starting` assertion is the real one — a `state database session … has
cwd …` check. Neither scenario is about the status lifecycle at all; both create a session via the
create modal with no agent named, which defaults to `shell`. **The assertion is scaffolding that
happens to be unsound**, and removing it costs the scenario nothing.

**What to build.** Stop asserting a transient the spec permits to be skipped. Assert the durable
consequence instead — the row exists, or it reads `running` — so the scenario waits for something
that is guaranteed to become true and stay true.

**This is not a licence to delete assertions.** The scenario must still *wait* for the session to
appear before reading the store, or it trades a flake for a different flake. Bound the wait on the
observable consequence, per non-negotiable 3.

**Success criteria.**
- Both `create_cwd_ghost.feature` scenarios pass, and their cwd assertions — the actual
  requirements they exist to prove — are unchanged.
- **Sweep the class, do not fix two lines.** There are twelve-plus `screen contains "starting"`
  assertions across `features/`, and **most are sound**. Two discriminators decide, and both must be
  applied:
  - **The session's agent.** An agent session (`claude`, `codex`) gets no fast-forward, so
    `starting` is a real, observable state for it and its assertion is sound.
  - **Where the assertion reads.** The fast-forward is a *status transition*, so the store really
    does hold `starting` briefly; what can be skipped is the **rendered frame** that paints it. A
    step reading the **store** is therefore sound even for a shell session — `features/
    concurrency.feature:21` asserts exactly that, is a **pinned Phase 1 assertion**, and must not be
    touched. Only assertions that read a *frame* (`screen contains`, `row … contains`) are in
    scope.

  Enumerate every `starting` assertion in the suite, classify each on both axes, and fix only those
  that are frame-reading **and** shell. Phase 2's own review found this exact trap and the lesson is
  already recorded in `docs/DELIVERY-LOG.md:412`: the sweep instruction is *"find shell rows
  asserted to be in `starting` by any means"*, not "grep for the string". Copy that.
- Report the enumeration as a table: file:line, session, agent, reads store or frame, sound or not,
  and what you did. A sweep whose result is "these two and no others" is a fine outcome **if** the
  table shows the others were checked on both axes.
- No `sleep`, no widened timeout, no `@flaky` tag, no scenario skipped. Raising the 5s frame-wait
  would hide this rather than fix it.
- State plainly in the report that this mechanism is **not a product defect** and that no product
  code changed for R64. If you find yourself editing `internal/service/reconcile.go`, you have
  misdiagnosed it — the fast-forward is specified behaviour.

## R65 — an exact-count assertion is read after the count has settled, and stays exact

**Source:** `docs/reports/phase3e-408-stability-10-at-75861e0/README.md` item 2 (run 9), and
`docs/reports/phase3e-stability/README.md` item 1 (three of ten runs at the previous commit). The
same assertion shape has now failed in a stability run at two different commits.

**Root cause, as read off the tree.** The step `Then the fake "<agent>" agent received exactly N
SIGWINCH signals` reads a counter **once, immediately**, with no settle beyond the harness's normal
frame-wait. There are seven such assertions:

- `features/preview.feature:95`, `:107`, `:130`, `:147`, `:149`
- `features/interactive_sigwinch_budget.feature:33`, `:46`

`preview.feature:147` is the one observed failing (`received 1 SIGWINCH signals, want exactly 0`,
in the `@steer-018-preview-fit-on-navigation` scenario, at the highest start-loadavg of the ten
runs). Note the direction: it is an **`exactly 0`** assertion that failed by seeing **one**. So this
shape is racy in both directions — a late SIGWINCH from an earlier step can arrive after a `0` is
expected, and an awaited one can be missing when a `1` is expected. Sampling an asynchronous
counter at an arbitrary instant is unsound regardless of which way it breaks.

**What to build.** Make the step settle before it reads: wait until no further PTY output has
arrived for a stated quiet window (`WaitForQuiescence`, `features/pty_driver_test.go:530`, already
used at `features/mouse_synthesis_test.go:226`), then read the counter once and compare for
equality.

**Success criteria — and read this bullet twice, it is the whole requirement.**
- **The assertion stays `exactly`.** A "poll until the count reaches N" loop silently converts
  `exactly 1` into *at least 1* and `exactly 0` into *nothing at all*, and would pass an
  implementation that sends five SIGWINCHes or one that sends none. The step must settle **first**
  and compare for equality **after**. Prove it: with the settle in place, temporarily make the
  product emit one extra SIGWINCH and show the `exactly 1` assertions go **red**; and show an
  `exactly 0` assertion goes red when a SIGWINCH is injected. Both directions, both reported.
- All seven sites move to the settled step. Fixing only the one observed failing is not the
  requirement — the other six are the same unsound shape and are one host-load spike from failing.
- No expected count changes. If making a site settle changes what it observes, that is a finding
  about the product, not a number to update: report it and stop.
- `ci/stability.sh 10` evidence is in Deliverables, not here. R65 is a correctness requirement about
  the assertion, and it is required **even if R63 turns out to have removed the flake** — an
  unsound assertion that currently happens to pass is still unsound.

## R66 — `matrix`'s seven status tokens are pairwise distinct under 16-colour quantisation

**Source:** `docs/reports/phase3e-findings.md` §4b. **Operator decision, do not re-litigate:** the
palette is **widened**. Recording the collision as an accepted limitation was offered and declined.

**Scope, stated precisely because it bounds the blast radius:** this is a change to
`internal/theme/builtin/matrix.toml`'s authored colours **only**. No other built-in's palette
moves. No new general rule is added to `SPEC.md` §11.6, which continues to require legibility (WCAG
≥ 3:1 over both the hex palette and its quantisation) and **not** pairwise distinctness. You are
making one theme better than the floor, not raising the floor.

**Root cause, measured — you do not need to re-derive this.** Under `quantize()`'s nearest-by-
Euclidean-RGB rule against `ReferencePalette`, five of `matrix`'s tokens collapse onto ANSI 8
(`#7f7f7f`). Measured against `matrix`'s own background (`#001100`, which itself quantises to ANSI
0 `#000000`):

| token | authored | ANSI | quantised | contrast vs quantised bg |
|---|---|---|---|---|
| `waiting`  | `#ffff33` | 11 | `#ffff00` | 19.56 |
| `running`  | `#00ff66` | 10 | `#00ff00` | 15.30 |
| `starting` | `#33aaff` |  6 | `#00cdcd` | 10.61 |
| `error`    | `#ff3333` |  9 | `#ff0000` |  5.25 |
| **`idle`**     | `#33cc66` | **8** | `#7f7f7f` | 5.24 |
| **`stopped`**  | `#889988` | **8** | `#7f7f7f` | 5.24 |
| **`archived`** | `#66aa77` | **8** | `#7f7f7f` | 5.24 |
| `hint`     | `#55cc77` |  8 | `#7f7f7f` | 5.24 |
| `badge`    | `#55cc77` |  8 | `#7f7f7f` | 5.24 |

Every one of them clears the 3:1 floor — the defect is **collision, not legibility**. Four statuses
already occupy four distinct slots; the work is to move `idle`, `stopped` and `archived` onto three
mutually distinct slots. Fourteen of the sixteen reference slots clear 3:1 against ANSI 0 (all but
0 `#000000` at 1.00 and 4 `#0000ee` at 2.23), so there is room:

    1 #cd0000 3.60   2 #00cd00 9.73   3 #cdcd00 12.33   5 #cd00cd 4.48   6 #00cdcd 10.61
    7 #e5e5e5 16.67  8 #7f7f7f 5.24   9 #ff0000 5.25   10 #00ff00 15.30  11 #ffff00 19.56
    12 #5c5cff 4.43  13 #ff00ff 6.70  14 #00ffff 16.75  15 #ffffff 21.00

**What to build.** Re-author `idle`, `stopped` and `archived` in `matrix.toml` so all seven §7
status tokens quantise to seven distinct `ReferencePalette` entries, then reconcile the pinned
quantisation table. ANSI 2 (`#00cd00`) is the obvious home for `idle` — it keeps the theme green,
which is the point of `matrix` — and ANSI 8's grey suits `stopped`; `archived` is the one that
genuinely has to move somewhere new. **Those are suggestions, not instructions:** you are choosing
colours for a theme, and the criteria below are what actually has to hold.

**Success criteria.**
- A new test asserts the seven §7 status tokens of `matrix` quantise to **seven distinct**
  `ReferencePalette` entries. Compute distinctness over the quantised values, not the authored
  hexes — the authored hexes were already distinct while the bug was present, so a test over them
  passes with the defect in place. That is the trap.
- `TestBuiltinContrastFloor` and `TestSessionRowTokensClearContrastFloorOnSurface` pass for the new
  values, over **both** the hex palette and its quantisation. A new colour that is distinct but
  dips under 3:1 trades one defect for a worse one.
- `TestMatrixStatusTokensRenderAsSevenDistinctColours`
  (`internal/tui/matrix_status_tokens_test.go:52`) passes **unmodified**. True-colour distinctness
  was already true and must stay true; if your new hexes need that test changed, they are wrong.
- `TestBuiltinQuantizationPinned` (`internal/theme/quantize_test.go:58`) is updated for `matrix`'s
  three changed tokens and **for nothing else**. Its doc comment sanctions exactly this
  reconciliation. Show the diff in the report and state the before/after quantised value per token,
  so the reconciliation is auditable rather than asserted. **A diff touching another theme's pinned
  entries means you changed a palette you were not asked to change.**
- The other four built-ins (`cobalt`, `daylight`, `empire`, `parchment`) are **untouched**. Say so,
  and show it with `git diff --name-only`. If you discover they have the same collision — several
  probably do — that is a **finding to record, not scope to take**: generalising the property is the
  SPEC change this phase deliberately does not make.
- `NO_COLOR` and plain-text golden frames are **byte-identical**: this is colour data only. If a
  golden frame moves, you changed something other than a colour.
- State in the report whether `hint` and `badge` were moved. They are **not** §7 statuses, so the
  requirement does not reach them, and leaving both on ANSI 8 alongside `stopped` is a legitimate
  choice — but it is a choice, and the report should say which one you made and why.

## R67 — an artefact can be found by what it is, not by the task that made it

**Source:** `docs/DELIVERY-LOG.md`'s carried-items paragraph, which names both halves as *"naming
defects that make an artefact harder to find later"*.

**Root cause, as read off the tree.** Two independent instances, both pure naming:

1. **Eleven test files are named after task numbers**, not subjects. Task ids reset per approach, so
   a file called `settings_task017_test.go` names an id that has been reused, and the reader must
   consult a plan to learn what it covers:

       internal/config/config_task017_test.go
       internal/tui/settings_task002_test.go   internal/tui/settings_task003_test.go
       internal/tui/settings_task006_test.go   internal/tui/settings_task007_test.go
       internal/tui/settings_task015_test.go   internal/tui/settings_task016_test.go
       internal/tui/settings_task017_test.go   internal/tui/settings_task018_test.go
       internal/tui/settings_task019_test.go   internal/tui/settings_task310_test.go

   (`DELIVERY-LOG.md` names only `config_task017_test.go`; there are eleven. Ten of the eleven are
   in one package, which is why `internal/tui/settings_task017_test.go` and
   `internal/config/config_task017_test.go` can coexist and confuse.)
2. **`docs/reports/phase2b2-findings.md` has two sections titled "Task 014"** — `:1249` (the SIGKILL
   teardown hang root-cause) and `:1511` (requirement 19's live-apply defeating requirement 21).
   Two different investigations, one heading, and the first is a document this PRD cites.

**What to build.** Rename each test file after what it tests, and retitle the two report sections
after what they found. Use `git mv` so history follows the file.

**Success criteria.**
- No file under any package matches `*task[0-9]*_test.go`. Show the glob returning nothing.
- **The test bodies are unchanged.** This is a rename, not a rewrite. Prove the suite is identical
  before and after: capture `go test -list '.*' ./internal/tui/ ./internal/config/` (or
  `-run` counts) at the parent commit and at yours, and show the sets of test-function names are
  **equal**. A rename that drops a test is the failure mode here, and a plain `ok` per package will
  not catch it.
- Each new name says what it covers, and no two files in a package collide. Where a file's contents
  turn out to cover more than one subject, splitting it is allowed but not required; if you split
  one, say why.
- The two `phase2b2-findings.md` headings become distinct and descriptive. **Any document citing
  those sections by title is updated in the same commit** — grep for citations before you rename,
  and say what you found. `docs/reports/` cross-references are load-bearing; non-negotiable 8 makes
  a dangling citation a defect.
- `docs/DELIVERY-LOG.md`'s carried-items paragraph is updated to record that this is now done,
  rather than left claiming it is outstanding.

## R68 — a previewed pane can never block deck's event loop

**Source:** [#5](https://github.com/n-orlov/deck/issues/5). Reproduced twice on the daily driver and
root-caused with `gdb` on the live hung process. **Do this one first** — see the orderings above.

**One citation convention, because it will otherwise waste your time.** The `vt/*.go` line numbers
below are in the **module cache**, not this tree: `github.com/charmbracelet/x/vt`
`v0.0.0-20260816001655-68d539dca504`, under `$(go env GOMODCACHE)/github.com/charmbracelet/x/vt@<that
version>/`. Grepping the repo for them finds nothing. Every one was verified against that pinned
version while this requirement was written; if a `go.mod` bump moves them, re-derive rather than
assuming the mechanism changed.

**Root cause, as read off the tree and off the stopped process.** `vt.Emulator` answers terminal
capability queries by writing the reply into an **unbuffered `io.Pipe`** (`vt/emulator.go:102`,
`t.pr, t.pw = io.Pipe()`), drained only by `Emulator.Read` (`:251`). deck constructs the emulator at
`internal/interactive/grid.go:93` and **never calls `Read` on it** — the only `.Read(` in the package
is `s.pipe.Read(buf)` at `:411`, the tmux FIFO. An `io.Pipe` write completes only when a reader takes
the bytes, so the first reply byte blocks its writer forever.

That write happens inside `drain`, **while `s.mu` is held**:

```go
// internal/interactive/grid.go:422-424
s.mu.Lock()
_, _ = s.grid.Write(buf[:n])   // blocks forever inside the vt DA handler
s.mu.Unlock()
```

`SafeEmulator.Write` takes its own `se.mu` for the duration too, so the stuck goroutine holds both.
`RenderRows` needs `s.mu.RLock()` (`grid.go:585`) and is called straight from `View()`:

`View` → `mainView` → `renderSideBySideFrame` → `previewBodyLines` → `interactiveBodyLines` →
`RenderRows` → blocked.

Bubble Tea calls `View()` synchronously from `eventLoop` (`bubbletea@v1.3.10/tea.go:502`), so the
loop never returns to read the next message. **That is why `q` and `Ctrl+C` do nothing, and why
`SIGTERM` is ignored** — bubbletea's signal handling sits on the same blocked loop. Recovery is
`SIGKILL` from another terminal, which then leaks the FIFO temp dir and leaves `pipe-pane` armed.

The gdb dump showed 51 goroutines, **all** parked — a genuine deadlock, not a livelock — with the
stuck writer at `vt/handlers.go:695` (the DA1 reply, `cmd=99` = `'c'`) and the wedged `View()` on the
**same `Session` pointer**. Transcribed in the issue.

**Blast radius: every replying handler is its own trigger.** DA1 (`handlers.go:695`, the one
observed), DA2 (`:712`), DSR (`:806`, `:809`, `:826`), DECRQM (`csi.go:30`), OSC 10/11/12 colour
queries (`osc.go:87`, `:92`, `:97`), in-band resize (`csi_mode.go:95`). **This is not
Claude-specific** — Claude Code merely happens to emit DA1 at startup. Note also why previewing an
already-running agent usually looks fine: its DA1 went out before the preview opened, and the seed
comes from `capture-pane` with escapes already stripped. The window is "a querying program starts
while the preview is live", which is exactly the reported scenario.

**What to build — two independent fixes, and you need both.** Either alone leaves a real problem.

1. **Drain the emulator's reply stream.** A goroutine per `Session` reading `Grid.Read()`. Without a
   reader, *any* query is a permanent stall.
2. **Do not hold `s.mu` across `grid.Write`.** The lock is what escalates one stuck goroutine into a
   dead UI. Fix 1 removes today's known stall; fix 2 removes the *class*, so that the next
   undrained-writer bug anywhere under `Write` costs a stalled preview rather than a dead deck.

**Discard or forward? Decide deliberately and say which.** Discarding is the honest default: deck is
a viewer, and a capability reply synthesised by `vt` is not what the real terminal would have
answered. The pipe is armed `-IO`, so forwarding into the pane is *possible* — but that means feeding
a live agent a fabricated DA1 it did not get from its actual terminal, which is a behaviour change
with its own blast radius. **Default to discard**; if you forward, the report must argue it.

**Success criteria.**

- **The regression test must not hang. Read this bullet before you write a line of it.** Feed
  `\x1b[c` into a live `interactive.Session` and assert the session keeps draining and `RenderRows`
  still returns. On today's code that test **hangs rather than fails** — which means a naive version
  wedges the package until `go test`'s global timeout fires and dumps every goroutine with no
  attribution to your test. Put the work in a goroutine and bound it: `select` on a done channel
  versus a `time.After` deadline, and **fail on timeout with a message naming the deadlock**. A test
  that can only fail by timing out the whole package is not a regression test, it is a trap for the
  next person.
- Run that test against unfixed code and show it **red by your own deadline**, not red by package
  timeout. Then fix and show it green. Per non-negotiable 5, actually revert and reproduce.
- **Cover more than DA1.** DA1 is the one observed; the defect is the undrained pipe. Add at least
  DSR (`CSI 6n`) and one OSC colour query as table cases. A fix that special-cases `CSI c` passes a
  DA1-only test and leaves five triggers live — that is the sloppy version of this change.
- **`RenderRows`' atomicity requirement is real and must survive.** Its doc comment explains why it
  wants a consistent grid across its several emulator calls. A fix that simply drops the lock trades
  a hang for a torn frame, which is harder to notice and harder to diagnose. Satisfy both: state in
  the report what now guarantees a consistent read, and how you know.
- `features/preview.feature`, `features/interactive_sigwinch_budget.feature` and the rest of
  `features/interactive_*.feature` pass unmodified. **If a SIGWINCH count has to move to accommodate
  R68, stop and report** — same rule as R63.
- **Check whether the fake agent can emit a DA1 at startup**, and if it can, add the end-to-end
  scenario as well; the package test bounds the deadline precisely, but a scenario proves the whole
  seam. If it cannot, say so explicitly rather than silently omitting it.
- The two minor leaks noted in #5 are **findings, not requirements**: the orphaned
  `/tmp/deck-interactive-pipe-*` dir left by an abnormal exit (`internal/tmux/pipe.go:94` creates it,
  only `closeLocal` at `:280` removes it), and confirming a preview switch tears the old `Session`
  down rather than orphaning it. Record what you find; fix only if it is trivial and say so.

## R69 — a dead pane is collected on sight, even when the row already reads `stopped`

**Source:** [#6](https://github.com/n-orlov/deck/issues/6), observed on `deck-backlog-ideas`, with the
sequence proven from the row's own event log.

**This requirement restores an invariant `SPEC.md` already states, and the spec even names the
consequence that was observed** (`SPEC.md:547`):

> **A dead pane is collected on sight, never retained.** … while also **holding the session name
> against the next resume and leaving crashed sessions on the socket indefinitely.**

**Root cause, as read off the tree.** deck's server runs `remain-on-exit failed`, so a pane exiting
**non-zero** is retained as a dead pane along with its session. When `claude --resume` exited 1,
Claude's `SessionEnd` hook (reason `other`, which `sessionEndInSessionReasons` does *not* exempt)
wrote `status=stopped` in the **same millisecond**. The next reconcile pass then short-circuits on
that status **before tmux is ever consulted**:

```go
// internal/service/reconcile.go:61
if session.Status == "stopped" || session.PaneExitStatus != nil ||
   (session.Status == "starting" && session.StatusSource == "user") {
	continue
}
```

So the crashed-pane branch (`reconcile.go:70`-`:155`) — which captures the tail, writes `error` plus
`pane_exit_status`, and **then `s.TMux.Kill`s the session** — is never reached. `pane_exit_status`
stays NULL, the corpse is never collected, and nothing ever clears `status=stopped`, so the guard
holds forever.

Three actions then refuse, each consulting a *different* notion of liveness:

| key | code | guard | verdict |
|---|---|---|---|
| `x` kill | `internal/service/kill.go:24` | `session.Status == "stopped"` | refused: `Cannot kill: session is already stopped` |
| `r` resume | `internal/service/resume.go:72` | `TMux.Exists(slug)` → `ResumeAlreadyRunning` | **silent no-op** — `has-session` succeeds on a session whose only pane is dead, so requirement 46's adoption fires and the UI says `already running` |
| `R` restart | `internal/service/restart.go:43` | `session.Status == "stopped"` | refused: `it is already stopped (resume it instead)` |

**No UI workaround exists.** Bulk `m`+`x` skips stopped rows deliberately
(`internal/tui/tui.go:2053`, inside the bulk-kill closure), and the single-row `x` refuses at `:2063`.
Only `tmux -L deck kill-session` from a shell, or deleting the row, gets out.

**What to build — three legs.**

1. **Collect the corpse regardless of durable status.** `reconcile.go:61`'s short-circuit must not
   suppress *collection*. A retained dead pane is captured-and-killed even when the row already reads
   `stopped`.
2. **Make the resume decision mean "has a live pane".** A session whose only pane is dead is not the
   pane requirement 46 is about adopting.
3. **Let kill remove a retained corpse.** `kill.go`'s guard becomes "already stopped *and* no tmux
   session exists". With leg 1 in place this should be unreachable — which makes it cheap insurance,
   not duplication. Say in the report that you understand it is belt-and-braces.

**Success criteria.**

- **The discriminating fixture is a row reading `stopped` (source `hook`) *while* a dead pane is
  retained.** Assert reconcile collects it and that `x` then succeeds. **A test that merely kills a
  stopped row passes on today's code** — that is the trap, and it is the only fixture that proves
  anything here.
- **The status write stays guarded; only collection is unguarded.** Collection writes `error` with
  `pane_exit_status`, first-writer-wins (`WHERE pane_exit_status IS NULL`), and must **not** turn the
  crash into a clean stop — `reconcile.go:63-66`'s comment says exactly why, and it is still right.
  Assert the collected row reads `error` with its exit status and tail, not `stopped`.
- **If you change `TMux.Exists`, enumerate its callers first and say what you found.** It is a shared
  predicate; changing its meaning to fix one caller is how three others break. A separate
  live-pane-aware accessor used by the resume decision is the lower-risk shape, and either is
  acceptable **if the sweep is in the report**.
- The `ResumeAlreadyRunning` path keeps reporting honestly. Requirement 46 exists so a genuine
  already-running session is not misreported as a launch failure; do not regress that while making
  the dead-pane case launch.
- Revert and reproduce, per non-negotiable 5.
- **R69 lands before R70.** Same line of code — see the orderings above.

## R70 — a crash verdict never outlives the pane it describes

**Source:** [#9](https://github.com/n-orlov/deck/issues/9). Three rows on the operator's box were in
this state, one displaying a wrong status for **30+ hours** while serving its user normally.

**This requirement is now spec-backed, and that is the one place the amendment went beyond R71/R72.**
`SPEC.md:728` adds a §9.1 bullet — *"resume likewise clears `pane_exit_status` and the crash tail"* —
directly beneath the existing `killed_by_user` bullet it reasons by analogy from. The spec was
**silent** here rather than contradictory, so this requirement did not need an amendment and still
does not; the sentence was added because the silence is how a sticky column removed three live
sessions from reconciliation. Treat it as confirmation of the fix below, not as a change of scope: it
does not license touching `SPEC.md` yourself, and it does not add a leg.

**Root cause, as read off the tree.** `pane_exit_status` is **write-once**. The only write is a
`COALESCE`, which by construction cannot clear it (`internal/store/store.go:650`):

```sql
pane_exit_status = COALESCE(?, pane_exit_status),
```

`grep -n pane_exit_status internal/store/store.go` returns exactly four hits — the column list
(`:525`, `:578`), that `COALESCE` (`:650`), and the DDL (`:1614`). **There is no
`SET pane_exit_status = NULL` anywhere in the codebase.** And `AcquireLaunchLease`
(`internal/store/lease.go:172-178`) — the only thing that flips a row to `starting` on resume —
writes four columns and none of them is this one:

```sql
UPDATE sessions
   SET status = 'starting', killed_by_user = 0,
       launch_lease_owner = ?, launch_lease_until = ?
 WHERE id = ? AND status = 'stopped' ...
```

So a resume that successfully creates a **new** pane leaves the **previous** pane's exit status
attached to the row. Two guards then read it as a permanent verdict:

1. `reconcile.go:61` skips the row outright on `PaneExitStatus != nil`.
2. `store.go:610` drops any **hook** write of `running` while `paneExitStatus.Valid`. `SessionStart`
   and `UserPromptSubmit` both map to `running` (`internal/hookrecv/receiver.go:57-58`), so both are
   dropped — while `UpdateSessionStatus` still inserts the event unconditionally, which is what makes
   this hard to spot: **the event log looks healthy while the status column is stale.**

`r` cannot help either: `AcquireLaunchLease` returns `LaunchLeaseNotLeasable` for any
`status != "stopped"`.

**Evidence, transcribed.** `pytest-bdd-slides`: pane alive (`dead=0`, `cmd=claude`), not archived,
`pane_exit_status = 137`, resumed at 2026-08-26 07:25:07, its `session_start` hook **recorded as an
event** at 07:25:10 — and `status_at` still reading **2026-08-25 07:09:09**. The event landed; the
status write was dropped.

**What to build.** Clear `pane_exit_status` **and** `crash_tail` when a resume creates a new pane, in
`AcquireLaunchLease`'s existing transaction, next to the `killed_by_user = 0` clear that is already
there on exactly the same rationale — its own doc says "an explicit resume is the user action that
releases the terminal kill guard", and that argument applies verbatim to a crash verdict. Note the
`COALESCE` shape cannot express "set to NULL", so it changes or a dedicated statement is added.

**Success criteria.**

- **The discriminating test drives a post-launch transition.** A test that reads the status straight
  after `launch.ready` **passes on today's code**, because `starting` is genuinely correct at that
  instant. The bug is that it is still `starting` a day later. So: resume a row carrying a stale
  `pane_exit_status`, then either deliver a `SessionStart` hook and assert `running`, or run a
  reconcile pass and assert the row was observed. Both guards get a test; neither is optional.
- **Do not weaken `store.go:610`.** It is correct for the case it was written for — a genuine crash
  verdict on the *current* pane. Once the column is cleared on resume it becomes correct as written.
  Deleting it to make a test pass is the failure mode, and a diff that removes it fails review.
- **State the `crash_tail` retention decision explicitly.** Clearing it is the recommendation: a tail
  describing a pane that has been replaced answers a question nobody asked, and it is the column's
  presence — not its content — that gates reconciliation. If forensic retention is wanted, it belongs
  in `events`, not in a column with control-flow meaning. Either way the report says which and why.
- Assert the row is reconcilable **after** the resume: the same session, resumed, is observed against
  tmux on the next pass. This is the assertion that pins guard 1, and it fails today.
- Revert and reproduce, per non-negotiable 5.
- **R70 lands on top of R69's rewritten guard**, not beside it.

## R71 — an archived session is not startable, and archiving is reversible

**Source:** [#8](https://github.com/n-orlov/deck/issues/8). **Operator decision, do not re-litigate:**
an archived session must not be startable, and there must be a UI way to unarchive. Clearing
`archived_at` on resume was considered and **declined** in favour of this.

**The companion `SPEC.md` amendment has landed (`c80a14c`), so the spec now requires all three legs
below.** You are implementing the spec, not proposing a change to it.

**Root cause, as read off the tree.** `Archive` upholds §4's invariant in the direction it covers — a
non-stopped session is killed before `archived_at` is set (`internal/service/archive.go`). Nothing
upholds the other direction: `Resume` never reads or clears `archived_at`, so
`stopped+archived` → `r` → `starting+archived` produces exactly the state the invariant forbids
(`SPEC.md:323-332`): *"an archived row can never hide a live agent."* Confirmed live — the pane was
`dead=0, cmd=claude` while the row was archived and absent from the list.

**And the status freezes, because archived-row exclusion is implemented in exactly one place** —
`ListSessions`' `WHERE deleted_at = 0 AND archived_at = 0` — **which three consumers depend on for
correctness, not display.** The worst is hook resolution: `hookrecv.resolve`
(`internal/hookrecv/receiver.go:194-221`) resolves against `db.ListSessions` and nothing else, and
**both** routes SPEC §8.1 specifies — payload conversation id, then the injected row id — iterate that
same archived-excluding slice. Every hook from an archived-but-live session becomes an orphan
(`RecordOrphanEvent`, `session_id = NULL`).

The captured hook error proves both keys were correct and both failed:

```
Stop hook error: Failed with non-blocking status code: deck hook: hook session could not be resolved
  (conversation_id="78e4cab4-e90d-40d3-8ee1-0a62ee80bc69"
   injected_session_id="0aee5fc4-d2ed-4940-8f5d-5131dcf0d6e0")
```

The first is the row's `conversation_id`; the second is the row's `id`. **There is no key the hook
could have carried that would have resolved.** And it is not silent: the agent's own transcript gets
that banner every turn, reporting a failure the user cannot act on.

**What the amendment now says — read these five passages before writing a line of code** (all line
numbers at `c80a14c`):

- **`SPEC.md:323-332`** — the §4 invariant, in two bullets. An archived row is **not startable**, and
  **both** retention flags are reversible: `archived_at` has an unarchive exactly as `deleted_at` has
  a restore. The spec states why neither half stands alone, and names which code guards which
  direction. **`internal/store/store.go:1201-1205` and `:1256-1262` still say the opposite** ("an
  archived row has no restore at all") — they are now the text contradicting the spec, and correcting
  them is part of your change, not collateral.
- **`SPEC.md:506`** — the status table's `archived` row: *"not startable — unarchive (`U`) to resume"*.
- **`SPEC.md:718`** — §9.1's new bullet, which is leg 1 nearly verbatim: resume **and** restart refuse
  an archived session, checked **before the launch lease is taken**, with nothing created and a
  message naming `U`.
- **`SPEC.md:752-753`** — §9.2's action table. The `archive` row now carries the confirm, the kill, the
  toast and the not-startable consequence; the new `unarchive` / `U` row is leg 2.
- **`SPEC.md:1080-1087`** — the keymap, which now lists `U`. **The letter is settled: the spec says
  `U`.** The reasoning, recorded because it generalises: `U` sits next to `u` (undo) exactly as `A`
  sits next to `a` (attach), but unlike `A` it is **restorative**, so a slip costs nothing — adjacency
  is only a hazard for destructive keys. Note this makes §11.3's "never list a key that is not bound"
  momentarily untrue, by design: the spec leads the implementation here, and leg 2 is what makes it
  true again.
- **`SPEC.md:1887-1891`** — §13.5's replaced scenario line; see R72, which shares it.

**What to build — three legs.**

1. **Refuse resume and restart on an archived row.** Reject `ArchivedAt != 0` in `Resume`
   **before the launch lease is taken**, alongside the three SPEC-named rejections it already
   performs up front (unknown conversation id, missing cwd, agent not on `PATH`). `Restart` routes
   through `Resume` and inherits it. This is the leg that makes the wedge unreachable rather than
   merely recoverable.
2. **Add `Store.UnarchiveSession` clearing `archived_at`, and a key for it**, mirroring
   `RestoreSession`. Reachable from where an archived row is reachable — inside the `/` filter's
   results, which requirement 33 already establishes as the route to them.
3. **Stop `hookrecv.resolve` reading a display query.** It needs "all non-tombstoned rows", not "rows
   the sidebar shows".

**Success criteria.**

- **Leg 1's scenario asserts the absence of a pane, not the returned outcome.** A silent no-op and a
  correct refusal are indistinguishable from a return value alone — that is precisely the confusion
  #6 documents about `ResumeAlreadyRunning`. Assert **no tmux session was created** and the row is
  still `stopped`, plus a message the user can act on that names the unarchive key.
- **Leg 3 must not make tombstoned rows resolvable.** The lazy version drops both conditions from the
  `WHERE` clause. `deleted_at = 0` stays. Add the negative test: a tombstoned row's hook is still an
  orphan.
- **Leg 3's own test is an archived row with a live pane** — assert a hook naming its conversation id
  resolves and is recorded **against the row**. Keep this test even though leg 1 makes the state
  unreachable via resume: `Archive` kills *then* archives, so a hook in flight across that boundary is
  still orphaned today, and this assertion is what pins the fix.
- **Do not change `reconcile`'s use of `ListSessions`.** Once leg 1 holds, an archived row cannot have
  a live pane, so excluding it from reconciliation is *correct* — and including it would let
  reconciliation write statuses onto archived rows, which is a new bug. This is the one place where
  the obvious symmetry is wrong; if you change it, review will block. (Note the hook case is
  different because `UpdateSessionStatus` already refuses to let a hook resurrect a `stopped` row at
  `store.go:605`, so a late hook lands as evidence on the right row without reviving it — which is
  exactly the desired outcome.)
- **`features/filter.feature`'s Feature-level premise changes and must be updated deliberately.** It
  currently says the filter is *"the only route back to an archived row"*; after leg 2 the filter is
  how you **reach** it and `U` is how you **bring it back**. `@requirement-33-filter-reaches-archived-row`
  (`:42-50`) stays valid and should keep passing unmodified; add a new scenario for unarchive rather
  than repurposing it.
- A scenario proves the round trip end to end: create, archive (through R72's confirm), find via `/`,
  unarchive, resume, and assert the session is live and **in the default list without a filter**.
- Revert and reproduce each leg, per non-negotiable 5.
- **R71 lands before R72.**

## R72 — `A` never destroys anything without an explicit confirmation

**Source:** [#10](https://github.com/n-orlov/deck/issues/10). **Operator decision, do not
re-litigate:** `A` gets an explicit confirmation in the shape `dd` already uses. Refusing `A` on a
live row — which is what `SPEC.md:1864` required at the **pre-amendment** tip — was considered and
**declined**, because kill-and-archive as one action is genuinely wanted and §4 already described it.

**The companion `SPEC.md` amendment has landed (`c80a14c`).** `SPEC.md:752` now specifies the confirm,
and `:1887-1891` replaces the scenario that required a refusal, so this requirement implements the
spec rather than contradicting it.

**Root cause, as read off the tree.** `A` mutates on the keypress itself (`internal/tui/tui.go:2087`),
and `a` — one `Shift` away — is attach (`:2363`, `m.attachSelected()`). The adjacency is specified on
both sides: `SPEC.md:213` and `:1555` document `a` as "the escalation to a real terminal", and
`:1084` lists `A` as archive. In the reported incident the row's own event log shows `attached` at
10:19:18 that morning and the archive at 11:02:34 — `killed` (`killed by user`) and `archived`
(`user`) **one millisecond apart**, which is `Service.Archive`'s kill-then-archive signature for a
single press.

**And there is no feedback whatsoever.** `sessionArchived`'s success branch
(`internal/tui/tui.go:1492-1498`) sets no message at all:

```go
case sessionArchived:
	if msg.err != nil {
		m.attachError = "Cannot archive: " + msg.err.Error()
		return m, nil
	}
	m.attachError = ""
	return m, m.loadSessions
```

So the entire user-visible result of killing a live agent is that its row disappears. The very next
case, `sessionDeleted`, sets `deleteUndoSessionID` for a 60-second undo — for an action that is
**less** destructive, because it is reversible.

**The guarding in the code is inverted**, measured against §9.2's action table:

| action | key | guard today | reversible? |
|---|---|---|---|
| kill | `x` | none | yes — 10 s undo toast, `u` = resume (requirement 22) |
| delete | `dd` | chord **plus** confirm dialog | yes — 60 s, `RestoreSession` |
| archive | `A` | **none** | **no** (until R71) |

The one permanently irreversible action is the only one with no chord, no confirmation and no undo —
and on a live row `A` does strictly more than `x` with strictly less protection.

**What the amended spec now says** (at `c80a14c`): `SPEC.md:752`'s `archive` row states the confirm,
that the confirm covers the kill *and says so*, that **nothing is written until it is confirmed**, and
the success toast with `u` to undo. `:747` records the principle behind the change — friction scales
with reversibility, which is why `x` still confirms nothing while the two actions that remove a row
from view do. `:1887-1891` replaces §13.5's "archiving refused for a live session" with the scenario
this requirement must satisfy. And `:1248` is the pre-existing dialog rule you are inheriting rather
than inventing: *"Destructive actions confirm, and the confirmation names the target and what will
survive it."*

**What to build.** A confirm dialog on `A`, reusing the `deleteConfirming` shape including its dialog
contract and its suppression of the bare-letter keymap. The dialog names the session and, when the
row is not `stopped`, states plainly that **a live agent will be killed** — a consequence the spec now
states outright at `:752` and which was absent from its own description of the action before the
amendment. Add it to §11.4's dialog inventory in spirit as well as in the spec's list (`:1270`
already names it). On success, a toast with `u` to undo via R71's `UnarchiveSession`, following the
two existing undo trios.

**Success criteria.**

- **The discriminating scenario is `A` on a *running* row, asserting the keypress wrote NOTHING:**
  confirm dialog up, agent still alive, `archived_at` still `0`. Then a second scenario for the
  confirm landing both the kill and the flag. **A scenario that archives an already-`stopped` row
  passes on today's code** and is presumably the one that already exists.
- **`clientArchivesSelectedSession` breaks, and how you repair it decides whether this requirement
  is tested at all.** It currently sends a real `A` and waits for the row to leave the frame
  (`features/kill_delete_undo_test.go`, registered at `:43`); its callers are
  `features/event_log.feature:15` and `features/filter.feature:46`. **The correct repair drives the
  real dialog**: send `A`, assert the confirm is present, send the confirm key, then wait for the row
  to go. **The wrong repair calls the service directly, or auto-confirms** — the suite goes green and
  the confirm is never exercised by anything except its own new scenario. A diff in which that step
  stops going through the keymap fails review.
- **The dialog must suppress the bare-letter keymap while open**, per the existing contract, so a
  second `A` inside it does not do something surprising. Add the assertion; the contract exists but
  this dialog is new.
- The toast says what happened and `u` undoes it. Assert the undo actually restores the row to the
  default list — a toast offering an undo that does not work is worse than no toast.
- **Do not rebind `A`, and do not add a chord in this requirement.** Rebinding was declined. The
  chord is a legitimate separate question (`dd` has both) but it is explicitly out of scope here so
  that it cannot delay the confirm; if you think it is needed, record it as a finding.
- Revert and reproduce, per non-negotiable 5.

## R73 — a scrollable overlay scrolls by a line, by the arrows, and by the wheel

**Source:** [#7](https://github.com/n-orlov/deck/issues/7). The only item in the field half that harms
nothing but patience — and **the only one needing no spec amendment**; see below.

**Root cause, as read off the tree — three independent causes.** Exactly three overlays are
scrollable, all via `framedDialogScrollable`: `?` help (handler `updateHelpView`,
`internal/tui/tui.go:5022`), `i` detail (`updateDetailView`, `internal/tui/rename.go:41`), and `E`
event log (`updateEventLog`, `internal/tui/event_log.go:106`).

1. **Arrows and `j`/`k` are not bound.** All three handlers switch on `msg.String()` with cases for
   `pgup` and `pgdown` and nothing else; `up`/`down`/`k`/`j` fall through to the no-op.
2. **There is no line-granular scroll to bind them to.** The shared helper only ever moves by the
   whole content budget (`internal/tui/panel.go:841`):

   ```go
   next := current + dir*m.dialogContentBudget()   // always a full page
   ```

   So this is not a missing keybinding: `dialogScrollBy` has no notion of a step, and binding
   `up`/`down` to it as-is would make the arrows a second `PgUp`/`PgDn`.
3. **The wheel is swallowed by a blanket guard** (`internal/tui/tui.go:2445`): one early return
   covering all fifteen overlay flags, so a `MouseMsg` of any kind — including a wheel event over a
   scrollable viewport — is discarded before `handleMouse` sees it.

**Why no spec amendment: the guard over-applies the rule it cites.** `SPEC.md:1250` is about
**actions** — "The mouse can neither cancel nor confirm… no dialog action is reachable by mouse
alone". Wheel-scrolling a dialog's own viewport cancels nothing, confirms nothing, and reaches no
action. And §11.8 already supplies the discriminating test, in the passage allowing drag-to-select
over the preview: *"a drag is the exception, because selecting text is reading rather than acting: it
takes no focus, changes no status and moves no selection in the list."* Scrolling a read-only overlay
passes that test on all three counts. **The implementation is stricter than the spec, not compelled
by it** — so this is a defect fix, and the standing "do not edit `SPEC.md`" rule applies in full.

**What to build.** A step parameter on `dialogScrollBy` (or a sibling `dialogScrollByLines`), the
`up`/`down`/`k`/`j` bindings in all three handlers, and wheel routing for the three scrollable
overlays.

**Success criteria.**

- **Assert the first visible line advances by exactly 1 after `down`.** A test asserting merely that
  "the view changed" **passes on an implementation that wrongly bound `down` to the page step**,
  which is the specific mistake cause 2 makes easy. This is the whole test-design point of the
  requirement.
- `j`/`k` work too. They are the sidebar's own aliases for `↑`/`↓`, so a user navigating with them
  has no reason to expect them dead inside an overlay.
- **The wheel is routed for exactly the three scrollable overlays, and clicks and drags stay
  suppressed for all fifteen.** The cleanest shape is to test "scrollable overlay **and** wheel
  event" before the existing early return, leaving the action-suppressing rule exactly as strict as
  it is today for everything else. Assert both halves: the wheel scrolls the three, and a **click**
  inside a dialog still does nothing.
- **Update the help overlay's own text** — it is deck's only documentation (R7), so a new binding
  absent from it is undiscoverable, and R7 makes that a defect rather than a gap. **Note the trap:
  the help overlay is itself one of the three scrollable overlays, so adding lines changes its own
  scroll extent.** If a golden frame or a scroll-extent assertion pins the old text, update it
  deliberately and say so; do not discover it as a mystery failure.
- Pages currently do not overlap (`dialogScrollBy` steps by the full `dialogContentBudget()`, so
  consecutive pages share no lines). Most pagers keep a line or two. **Decide deliberately and record
  the decision**; changing it is optional, inheriting it silently from the arithmetic is not.
- `home`/`end`, or `g`/`G` matching the sidebar's own top/bottom keys, are **optional**. If you add
  them, they go in the help text too.
- **One thing to check and report, not to fix.** Every other dialog renders through `framedDialog`
  (`panel.go:776`), which applies **no height bound** and has no scroll offset: env editor, rename,
  create, settings, restart-choice, delete-confirm. If any can produce a body taller than the frame —
  the env editor with many variables at 80×24 is the likeliest — that overflow is unreachable.
  **This was flagged as unverified in #7 and stays unverified until someone checks.** Check it, report
  what you find, and treat a fix as out of scope unless it reproduces trivially.

---

# Non-negotiables

These are the standing rules of this codebase. Every one of them has been broken at least once in
an earlier phase and cost a repair task.

1. **`SPEC.md`, `prds/`, `ci/Dockerfile`, `ci/SPIKE.md` are protected.** Do not modify them. At
   close-out verify by **sha**, never by identity; the recognised set is **exactly two shas —
   `c80a14c` (the operator's `SPEC.md` amendment) and this PRD's own commit** — and any *third* sha
   touching a protected path is a hard failure to report, not to classify. Copy
   `docs/reports/phase3d-212-closeout/README.md`'s method. Both shas are already in history before
   your first commit, so an audit that finds a third has found a real violation, not a boundary case.
2. **Review blocks on real defects and tolerates documentation nits.** Dead code satisfying a
   requirement on paper, a vacuous test, a flaky scenario, a false claim about behaviour, or
   anything touching a session's `cwd` must block. Wording, formatting and stale sentences in
   derived summaries are recorded as notes and do not fail a phase. The test is whether a reader
   would be *misled about behaviour*. (Restated here because the review pass never reads
   `docs/PLAN.md`, and because a steer cannot reach review — the only way to change acceptance
   criteria mid-flight is to change this PRD.)
3. **No scenario is deleted, skipped, or tag-excluded to make a suite pass.** `defaultTags` stays
   `"~@real-agents && ~@nightly"`. No `@flaky`, no `@wip`, no retry loop. R64 and R65 are both
   *assertion* changes and this rule is what separates them from weakening: R64 replaces an unsound
   waypoint with a sound one, R65 adds a settle and keeps the comparison exact. Neither removes a
   scenario or relaxes what it proves. **The same rule reaches step definitions, and R72 is where it
   bites:** repairing `clientArchivesSelectedSession` by bypassing the new confirm — calling the
   service directly, or auto-confirming inside the step — keeps the suite green while silently
   removing the keymap from the tested path. A step that stops going through the real keypress is a
   weakened scenario wearing a green tick.
4. **A fix is root-caused, not padded.** No `sleep` added to make a test pass. If you bound a wait,
   bound it on the *observable consequence* of the thing you are waiting for, not on a proxy for "a
   render happened" — task 210's `029893a` is the worked example, and its report explains why
   `ResizeAndAwaitRender` alone was insufficient there.
5. **Ask "would this go red if the fix were reverted?" of every test you write, and answer it in the
   report by actually reverting and reproducing.** **Nine requirements here have a naive test that
   cannot distinguish correct from incorrect behaviour**, and each is named at the point of use.
   These are the nine places to spend the effort:

   | requirement | the naive test that proves nothing |
   |---|---|
   | R63 | a fix that suppresses *all* subsequent fits passes the first criterion |
   | R65 | a poll-until-N loop turns `exactly 1` into *at least 1* and `exactly 0` into nothing |
   | R66 | distinctness computed over authored hexes passes with the defect present |
   | R68 | a test that hangs instead of failing — see below, this one is worse than vacuous |
   | R69 | killing a merely-`stopped` row succeeds today |
   | R70 | reading the status right after `launch.ready` — `starting` is correct at that instant |
   | R71 | asserting the returned outcome instead of the absence of a pane |
   | R72 | archiving an already-`stopped` row never reaches the new code path |
   | R73 | "the view changed after `down`" passes if `down` was bound to the page step |

   **R68 is the one that needs care beyond the usual.** Its regression test *hangs* on unfixed code
   rather than failing, so a naive version wedges the package until `go test`'s global timeout fires
   and dumps every goroutine with no attribution. Bound it yourself and fail on your own deadline. A
   test whose only failure mode is timing out the suite is not a regression test.
6. **One commit per completed task, messages saying *why*.** Never force-push, amend, or rewrite
   published history — fix forward. `git status --short` and `git log origin/main..HEAD` both empty
   at close-out.
7. **Host load is a known confound.** A 1-min loadavg of 143.50 was recorded during Phase 3d and
   caused failures that looked like product bugs. Before attributing a flake to a code change, run
   the discriminating experiment at low load and record the loadavg. This phase is *about* two
   load-sensitive mechanisms, so record the loadavg for every stability and isolation run.
8. **Reports carry citations by sha and log path**, and every cited sha resolves and every cited log
   path exists.

# Deliverables

- All eleven requirements implemented, each with tests/scenarios as specified above. **Nothing is
  blocked on the spec:** the companion amendment landed in `c80a14c`, before this PRD's own commit.
- **The four orderings are honoured, and the report states that they were:** R63 before R65, R69
  before R70, R71 before R72, and R68 first of the field half.
- **R63 lands before the stability run, and before R65's own evidence is taken.** R63 is a candidate
  root cause of the SIGWINCH flake R65 hardens against — task 406's report already places both in
  the same debounce/reconcile family. Doing R63 first means the stability run measures a tree where
  the suspected cause is gone, and lets the report say whether R63 alone removed the flake. Doing
  R65 first destroys that information for good, because a settled assertion cannot observe the race
  it was hiding.
- **Each field-half commit names its GitHub issue** (`#5`–`#10`), and the close-out posts the fixing
  sha to each. Six issues were filed with live reproductions; leaving them open against a tree that
  has fixed them is how the next phase re-derives all of this. One of them, #8, carries a published
  correction in its comments — read the comments, not only the body, before implementing R71.
- `docs/reports/phase3f.md` — per-requirement evidence table, real command output, tool versions,
  wall-clock, gotchas. Same shape as `docs/reports/phase3e.md`.
- `docs/reports/phase3f-findings.md` — anything discovered that this PRD got wrong, any spec
  ambiguity, any defect found and not fixed (with why). **The already-closed table above is a claim
  about the tree; if any row of it is wrong, that finding belongs here.**
- A green whole-suite run at the final code commit: `ci/run.sh go test -p=1 -count=1 ./...`, exit 0,
  **every** package `ok` or `[no test files]`, cited by sha and log path.
- `ci/stability.sh 10` at that same final code commit. **The bar is 10/10, and this phase is the one
  that has to reach it** — Phase 3d reached it, 3e published 7/10 twice, and R64 and R65 target both
  mechanisms behind that number. Publish the real rate. If it is below 10/10, root-cause every
  failure to a mechanism rather than re-running for a streak, and state explicitly whether the
  mechanism is one this phase claimed to fix — a recurrence of R64's or R65's own mechanism is a
  **failed requirement**, not a host-load note. An honest 9/10 with a named new mechanism is worth
  more than a 10/10 nobody can explain; an unexplained 7/10 is a failed phase.
- A note in `docs/DELIVERY-LOG.md` recording what this phase closed, including the five bug-log
  items that turned out to be already fixed and by which commit — so the operator's list is
  reconciled against the tree once, durably, instead of being re-verified next phase. **Record the
  six field defects separately from those five**, with their issue numbers: one list is "already
  fixed before the phase", the other is "fixed by this phase", and conflating them destroys the only
  durable record of which is which.
- **A short note in the report on what the field half says about test coverage.** Six defects in one
  day of ordinary use, against a suite this large, is itself a finding. Three of them (#6, #8, #9)
  share a shape the suite has no scenario for: **a durable row and tmux disagreeing about liveness,
  with each guard consulting a different source of truth.** Whether that deserves its own scenario
  family is the operator's call and explicitly **not** scope to take here — but the observation
  belongs in `phase3f-findings.md` rather than in nobody's head.
