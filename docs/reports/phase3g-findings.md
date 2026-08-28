# Phase 3g findings

Companion to [`phase3g.md`](phase3g.md) (the per-requirement evidence report for
R76–R92). This file carries what the requirements themselves do not: any
`SPEC.md` contradiction actually met while doing the work, every defect found
and deliberately **not** fixed with the reason, anything
`prds/phase3g-field-backlog.md` got wrong, and the
disposition of the four Phase 3f documentation-nit findings (F7, F8, F10, F14)
this phase's non-goals section named. Modelled on
[`phase3f-findings.md`'s §6](phase3f-findings.md#6-defects-found-and-not-fixed-and-why).

Written by task 039 against the tree at `050ca9f` (task 038's HEAD; task 039
adds no code). Every sha cited resolves under `git cat-file -e` and every path
cited exists in the tree at that commit unless a line explicitly says
otherwise (the create-modal contradiction in [§2](#2-what-this-prd-got-wrong)
and the F2 recurrence check in [§4](#4-f2-the-golden-frame-settle-flake-no-recurrence-found)
are exactly that kind of negative check).

- [1. SPEC.md contradictions actually met](#1-specmd-contradictions-actually-met)
- [2. What this PRD got wrong](#2-what-this-prd-got-wrong)
- [3. Defects found and deliberately not fixed, and why](#3-defects-found-and-deliberately-not-fixed-and-why)
- [4. F2 — the golden-frame settle flake: no recurrence found](#4-f2-the-golden-frame-settle-flake-no-recurrence-found)
- [5. F7, F8, F10, F14 — disposition](#5-f7-f8-f10-f14-disposition)
- [6. How to re-check every citation in this report](#6-how-to-re-check-every-citation-in-this-report)

## 1. SPEC.md contradictions actually met

**None.** The PRD is explicit that eight of its seventeen requirements
contradicted `SPEC.md` as it stood, and that the correct response was to amend
the spec first rather than implement against a contradiction — which the
operator did, in `2eed8de` (plan change `a03527c`), *before* this phase's
tasks began. Every requirement below R76–R92 therefore has a spec authority
that already agrees with it; no task in this phase found a place where the
tree it was asked to build disagreed with `SPEC.md` as amended.

The nearest things to a contradiction, both already resolved without touching
`SPEC.md`:

- Task 021's residual (rename dialog's focused field has no `theme.Selection`
  background, §11.4) is a **gap between the delivered code and `SPEC.md`**,
  not a contradiction in the requirement itself — the fix is now landed, by
  task 105 (`ea6ce4b`). Recorded as [F18](#3-defects-found-and-deliberately-not-fixed-and-why) below,
  not here, because nothing about the *requirement* disagrees with `SPEC.md`.
- Task 016's unsatisfiable report (create modal) found that one **pre-existing
  PTY assertion**, not `SPEC.md` or the PRD, could not survive full §11.6
  theming. See [§2.3](#23-the-create-modals-unsatisfiable-task-016-was-a-standing-rule-vs-specmd-collision-not-a-prd-error) —
  it is filed as a standing-rule collision, not a `SPEC.md` contradiction,
  because `SPEC.md:1355` and the theming requirement were never in tension
  with each other, only with that one assertion's own scope.

## 2. What this PRD got wrong

### 2.1 R88's `§6.4` citation names the wrong SPEC.md section

R88's bullet says the restart-to-apply route is "§6.4's restart-to-apply
path". `SPEC.md` §6.4 is **"Secrets"** (`SPEC.md:432`); the section that
actually states the restart-to-apply rule ("a mid-flight edit is inherently
restart-to-apply") is **§6.2, "Editing while running"** (`SPEC.md:398`,
`SPEC.md:1427`). Task 029 (R88, finding F3) quoted the PRD's bullet verbatim
in its own refusal message rather than silently correcting it, and disclosed
the slip instead:
[`phase3g-029-inject-retained-dead-pane/README.md:34-41`](phase3g-029-inject-retained-dead-pane/README.md).
The behaviour R88 asked for is unaffected — the corpse-pane refusal is
correct either way — only the section number in the on-screen message and in
the PRD's own prose is off by one subsection.

### 2.2 The ordering section's "R85 … independent" claim does not hold

The PRD's ordering section (item 7) lists R85, R87, R88, R90, R91, R92 as
"independent; take them in whatever order the tier ladder favours" — as
opposed to R86, which item 6 calls out separately as needing to land "late"
because it touches on-screen text. In the tree, **R85 and R86 are coupled
through a defect neither requirement's author anticipated**, not through
anything either requirement's own text asks for:

- Task 024 (R85, `9991689`) makes the create modal pre-select the
  *last successfully-created* agent instead of always `shell`. That is
  exactly what R85 asks for.
- Fourteen `features/*_test.go` files (verified via
  `grep -rl 'Create shell session' features/*.go` against `050ca9f`) contain
  roughly nineteen to twenty individual call sites hard-coding the literal —
  notes.md's running tally says "~19", task 026's own `unsatisfiableReason`
  in `tasks.json` says "Roughly 20"; the exact count does not change the
  outcome below. The files: `features/agent_steps_test.go`,
  `features/assertions_test.go`, `features/coalesced_keymsg_test.go`,
  `features/create_blank_name_test.go`, `features/create_modal_test.go`,
  `features/create_reuse_warning_test.go`, `features/create_session_test.go`,
  `features/create_tilde_test.go`, `features/create_validation_test.go`,
  `features/determinism_test.go`, `features/dialogs_test.go`,
  `features/kill_delete_undo_fingerprint_test.go`,
  `features/mouse_reenable_after_attach_test.go`,
  `features/sidebar_width_test.go`. All hard-code the literal
  title `"Create shell session"` as the create modal's opening frame. That
  literal was true unconditionally before R85 (the modal always opened on
  `shell`) and is now only true when the *previously*-created agent in that
  scenario happened to be `shell`.
- The result: any scenario in those files that creates a second, non-shell
  session after a first one hangs/times out waiting for a title the modal
  never shows, deterministically (confirmed by task 026's own three full runs
  of the same criterion command, and by patching one call site, which only
  moved the hang to the next unpatched literal rather than removing it — see
  task 026's `unsatisfiableReason` in `tasks.json`).
- Task 025/026 (R86) built the title-independent replacement,
  `features/agent_steps_test.go:395-401`'s `ensureCreateModalAgent`, and
  converged most call sites onto it (`8cff03b`, plus one more in `cc36cfa`
  for `lease_race.feature`). One call site remains unconverged —
  `clientOpensCreateModalForAgent` at `features/agent_steps_test.go:521`,
  which still waits for the literal `"Create shell session"` — and two
  feature files are consequently red: `durable_identity.feature` and
  `status_claude_hooks.feature`. Running `ci/run.sh go test -count=1
  ./features/` blows the suite's own 10-minute timeout as a result.
- So R85's *own* work landed cleanly and met its own criterion; the breakage
  is entirely in R86's follow-on task (026), which is why 026 is `skipped`
  (unsatisfiable) rather than R85 being reopened. But the PRD's ordering
  section treats R85 as freestanding, and it demonstrably is not: it is the
  precondition for the specific defect that then made 026 impossible as
  written. This was expected to make task 040 (whole-suite green) fail until
  a follow-up task finished the convergence — done by task 104 (`045a6e6`); see
  [F19](#3-defects-found-and-deliberately-not-fixed-and-why) below.

### 2.3 The create modal's unsatisfiable (task 016) was a standing-rule vs. `SPEC.md` collision, not a PRD error

Not a PRD-wrong item — filed here to record why it is *not* one. The PRD's
R82 asks the create modal to render in §11.6 tokens (`SPEC.md:1355` pins
per-field help text to the `dimmed` token) and this job's standing rules ask
existing keyboard-only PTY assertions to stay green byte-unchanged through
the theming work. Both clauses are individually correct; they collided only
because one pre-existing assertion,
`features/create_cwd_ghost_test.go`'s `clientCWDFieldShowsNoGhostText`,
scanned the *whole grid* for any dimmed cell on the premise — true only while
the modal was unthemed — that the cwd-completion ghost was the sole dimmed
thing on screen. Full reproduction:
`/run/ralphd/artifacts/task016-unsatisfiable/README.md`. Nothing here
implicates the PRD's or `SPEC.md`'s text; the assertion's own scope was
wider than its own premise once §11.6 applied elsewhere on the same frame.

### 2.4 Notes.md corrections folded in above

The handoff notes file's "CRITICAL FINDING" section is the same defect as
§2.2 above (the ~19 hard-coded `"Create shell session"` sites); it is not a
second, separate correction. Notes.md's other running corrections (016
skipped, 021 failed, 026 skipped, 038 skipped) are dispositions of *tasks*,
not of the PRD's text, and are carried in [§3](#3-defects-found-and-deliberately-not-fixed-and-why)
below rather than repeated here.

## 3. Defects found and deliberately not fixed, and why

Numbering continues from [`phase3f-findings.md`'s F1–F17](phase3f-findings.md#6-defects-found-and-not-fixed-and-why).

| # | defect | where | why not fixed here |
|---|---|---|---|
| F18 | the rename dialog's always-focused "New name" field renders through `detailField` (`internal/tui/rename.go:172`, `:226`), which applies only `theme.Hint`/`theme.Text` — no `theme.Selection` background composes onto the focused row, contrary to §11.4's focused-field treatment | `internal/tui/rename.go` (task 021, `1c8cbad`) | task 021 ran all three validation attempts and landed everything else in its criterion (render-time `colorToken` calls in both target files, unchanged plain text, `./internal/tui/` and `event_log.feature` green); the tier ladder exhausted before this last gap closed. Follow-up task recorded verbatim in task 021's own `validationNotes` in `tasks.json`: apply `theme.Selection` to the field, assert a rendered-grid cell carries that background, keep plain text byte-identical. **Resolved by task 105** (`ea6ce4b`): `internal/tui/rename.go`'s focused field now composes `theme.Selection` onto the row exactly as the follow-up asked, with a new grid-cell assertion in `internal/tui/rename_theme_test.go` and plain text kept byte-identical; see [`phase3g-105-rename-selection/README.md`](phase3g-105-rename-selection/README.md). |
| F19 | ~19 `features/*_test.go` sites hard-code the literal `"Create shell session"` as the create modal's title, a literal R85 (task 024, `9991689`) made conditional on which agent was created last; one call site — `clientOpensCreateModalForAgent`, `features/agent_steps_test.go:521` — is still unconverged onto task 025/026's title-independent `ensureCreateModalAgent` helper | `features/agent_steps_test.go:521`; red features `durable_identity.feature`, `status_claude_hooks.feature` | out of scope for task 026's own deliverable (dialog on-screen text for the new `↑`/`↓`/`Ctrl+P`/`Ctrl+N` bindings) — task 026 is `skipped` (unsatisfiable) over exactly this, not over its own text-update work, which landed in `8cff03b`. The mechanical convergence of the one remaining site is a new, narrow task; see [§2.2](#22-the-ordering-sections-r85-independent-claim-does-not-hold) for the causal chain. **This is expected to make task 040 (whole-suite green) fail** — `ci/run.sh go test -count=1 ./features/` blows its own 10-minute timeout at `050ca9f` — until that follow-up lands. **Resolved by task 104** (`045a6e6`): the remaining site and every other hard-coded-title wait across the listed files now waits on the unconditionally-rendered Agent row directly, or via `ensureCreateModalAgent` when a specific agent must be forced, each with an inline comment; `create_session.feature` gains a scenario proving the converged path survives a prior claude create. See [`phase3g-104-title-independent-waits/README.md`](phase3g-104-title-independent-waits/README.md). |
| F20 | `features/status_recovery.feature`'s "`r` on a session whose tmux session already exists reports already-running, never an error" scenario (`:23-35`) is red at `050ca9f`: its frame-read assertion at `:32` (`... screen contains "stopped - resumable"`) races R76's own new reconcile repair (`89fcffc`/`15e33c6`), which now corrects the same terminal-row-with-live-pane contradiction the scenario deliberately constructs *before* the scenario's frame ever shows the pre-repair `"stopped"` text | `features/status_recovery.feature:32` | this is R76 doing exactly its job against a scenario written before R76 existed to contradict it — a genuine interaction between two requirements, not a defect in either alone. First flagged as a discovered side effect in task 002's own evidence (`docs/reports/phase3g-002-r76-field-route/README.md`), reconfirmed here at `050ca9f`: `ci/run.sh env DECK_GODOG_PATHS=status_recovery.feature go test ./features/ -run TestFeatures -count=1` → `4 scenarios (3 passed, 1 failed)`, `step error: client "A" did not show "stopped - resumable" within 500ms`. Not fixed by task 038 or 039 (both report-writing, out of scope for a scenario edit); the store-read assertion at `:30` is sound and is the model for the fix — rewrite `:32`'s frame read to a store read (`the state database session "dup pane" is "stopped - resumable" from ...`, or equivalent), a new, narrow follow-up task. **Resolved, in approach 01's own follow-on task 040** (`fe79040`, landed just after this report's own task 039 published it, so it postdates the `050ca9f` state of record above): the scenario was renamed and its `:32` frame-read wait repointed at the store, asserting the post-repair `starting`/`tmux` state directly, exactly the rewrite this row called for; `r`'s own decline is now asserted as `"not stopped"` rather than `"already running"`. Verified stable 8/8 standalone per the commit message. This row is retained as the historical record of the defect as found. |
| F21 | `internal/theme/contrast_test.go`'s floor does not yet cover `hint`/`surface`, `key`/`surface`, `error`/`surface`, or the text tokens over `Selection`/`SelectionIdle` that R82's now-themed dialogs actually draw | `internal/theme/contrast_test.go` (state of record `7033e12`; no sha in `1cfbd5a..050ca9f` touches the file) | **Resolved by task 106** (`57a6882`, refined in `0219e42`; `internal/theme/contrast_test.go`'s new `TestThemedDialogTokensClearContrastFloor`, see [`phase3g-106-contrast-floor/`](phase3g-106-contrast-floor/README.md)): the floor now covers exactly this set, for every built-in, both hex and quantised. This row is left as history of the earlier plan's task 023 numbering (superseded by this wave's task 106); F23 below carries what that new coverage actually found. |
| F22 | `internal/interactive`'s `TestSessionRendersAreCoalescedAgainstAKnownByteArrivalPattern` (`internal/interactive/render_test.go:155`) flakes when the package runs under multi-package parallel load: the tmux `pipe-pane` job does not open the run's FIFO inside the transport helper's 5s connect budget, `Start` returns `arm pipe-pane before seed capture: ... timed out after 5s waiting for pipe-pane's job to open the fifo`, and the test fails at `internal/interactive/render_test.go:175`. Observed once, at task 030's own three-package criterion run: `docs/reports/phase3g-030-reclaim-leaked-interactive-pipe/criterion-packages-run1.log:2-3`; it passed on its own re-run in the same task and has not recurred in any later run this phase. Also recorded in the handoff notes' open-items list | `internal/interactive/render_test.go:155`, `:175` (state of record `60b81ee`; `git log --oneline 1cfbd5a..050ca9f -- internal/interactive/render_test.go` is empty, so no phase3g sha touches the file) | pre-existing and out of scope: the flake is in the *test harness's* timeout budget against a real tmux pane, not in product code, and no R76–R92 requirement covers `internal/interactive`'s render-coalescing test or its connect budget. It was found and disclosed at task 030 ([`phase3g-030-reclaim-leaked-interactive-pipe/README.md:61-64`](phase3g-030-reclaim-leaked-interactive-pipe/README.md), "a pre-existing timing flake, not touched") rather than fixed there, because the only available fix is to raise that 5s budget, and this job's standing rules allow a timing re-baseline only against a derivation written **before** the change — never one read off a failure. Left for a follow-up task that derives the budget from the transport's own measured connect latency under load. F2's own out-of-scope wording is not what covers this one: F2 is a different flake in a different package ([§4](#4-f2-the-golden-frame-settle-flake-no-recurrence-found)), and this row stands on the two reasons above. If task 041's `ci/stability.sh 10` reproduces it, the rate belongs in that task's report. |
| F23 | R84's new contrast pairs (task 106, [`phase3g-106-contrast-floor/`](phase3g-106-contrast-floor/README.md)) fail on three of the five built-ins, always on `dimmed` against `Selection`/`SelectionIdle`: `cobalt` `dimmed/selection` hex 2.59:1, `parchment` `dimmed/selection` hex 2.51:1, and `empire` collapses hardest — `dimmed/selectionidle` hex 2.13:1, and on the 16-colour quantisation `dimmed`, `hint` and `error` all quantise to the *same* reference colour (`#7f7f7f`) as `SelectionIdle` itself, a 1.00:1 self-pair. `daylight` and `matrix` (the PRD's own reference theme) clear every new pair | `internal/theme/builtin/cobalt.toml`, `empire.toml`, `parchment.toml` (unmodified — this is a finding about the existing authored/quantised values, not a diff) | per the PRD's own R84 text, "this requirement pins what is already true; it does not license a palette change" and "the operator's ruling is legibility over distinctness, with matrix as the reference theme" — a failing pair in a non-reference built-in is reported here, not recoloured. The new test (`TestThemedDialogTokensClearContrastFloor`) hard-enforces the floor for **every** built-in; the ten already-sub-floor cells above are exempted one by one, at their measured ratios, in `dialogPairAllowlist` (`internal/theme/contrast_test.go`), which also fails on drift of more than 0.01 in either direction, on a listed cell that has climbed to the floor, and on a key matching no cell — so the suite stays green at today's palette (`phase3g-106-contrast-floor/theme-suite-green.log`) while any *new* sub-floor pair, in any theme, breaks the build (demonstrated: `phase3g-106-contrast-floor/negative-unlisted-pair-fails.log`, `negative-recorded-ratio-drift-fails.log`). Each exempted cell is still logged as a `FINDING` line with its ratio, plus a per-theme `SUMMARY` line, in `dialog-contrast-v.log`. Left for the operator to decide whether `dimmed`'s authored/quantised value should ever move — no task in this plan is scoped to touch a theme's palette. |
| F24 | `features/agent_steps_test.go`'s `ensureCreateModalAgent` waited on the literal `"Agent: " + want + " (left/right cycles"` joined against `ScreenDriver.Frame`'s `"\n"` line join; once the create modal's dialog box narrows enough to wrap `createFieldRows`' Agent-row sentence across two grid rows (`kill_delete_undo.feature`'s 26-column clamp scenario forces exactly this at a 30x60 terminal), the wrap point falls inside the awaited literal and the wait can never match, timing out deterministically | `features/agent_steps_test.go` (state of record `b6cbbc7`) | **Resolved by task 101** (`2549406`): `ensureCreateModalAgent` now matches `dewrapCreateModalAgentRow(frame)`, which finds the Agent row, strips both it and the following row's dialog-box border/padding, and rejoins them with a single space, reconstructing the same logical line `createFieldRows` produced before the box wrapped it; the awaited marker's own value is unchanged. Test-harness-only — `internal/tui.createFieldRows`'s wrapping is correct and untouched. Three consecutive green runs; see [`phase3g-101-agent-wait-wrap/README.md`](phase3g-101-agent-wait-wrap/README.md). |
| F25 | R85 (task 024, `9991689`) added the literal `(last used) ` as a prefix on the create modal's **Agent** row help (`createAgentHelp`), identical to the pre-existing prefix on the **Working-directory** row help (`createCWDHelp`) that `features/settings.feature`'s `@requirement-17-clear-recent-cwds-history` scenario was asserting the bare literal `"(last used)"` against; from that point the scenario's negative check could never legitimately pass, because the Agent row's own `(last used)` prefix survives clearing recent cwds regardless of the cwd prefill's own state | `features/settings.feature:402` (state of record `b6cbbc7`) | **Resolved by task 102** (`89682e5`): both the positive and negative `(last used)` assertions are now pinned to the cwd row's own help text specifically (`"(last used) the session's cwd"`), which only `createCWDHelp` ever renders; the Agent row's identical prefix is always followed by `"which coding agent..."`, never `"the session's cwd"`, so the two can no longer collide. Proven still load-bearing by a local, uncommitted, reverted disable-and-check of the clear itself. See [`phase3g-102-clear-recents-label/README.md`](phase3g-102-clear-recents-label/README.md). |
| F26 | A hook-declared terminal status delivered while the pane it came through is still alive triggers R76's reconcile self-heal (`repairTerminalRowWithLivePane`) exactly as it does for a raw state-database write — SPEC §7 names the repair from the pane's own physical liveness, not from which mechanism produced the terminal write, so a scenario that poses a hook (e.g. `SessionEnd`) into a still-live pane races the same repair [F20](#3-defects-found-and-deliberately-not-fixed-and-why) documents for a direct write, and loses it deterministically, one reconcile tick later | `internal/service/reconcile.go`'s `repairTerminalRowWithLivePane` (untouched); `features/status_claude_hooks.feature` (state of record `b6cbbc7`) | **Not a defect — R76 working as specified**, a sibling of F20's own interaction, confirmed by task 103 (`51b7f17`): a genuine Claude `SessionEnd` and a genuinely dead pane always arrive together in the real product, so the fix belongs in the scenario's route to the state, not in loosening R76. Task 103 gave the fixture a clean pane-exit command and a released, pane-independent `deck _hook` route for both `SessionEnd` and the `SessionStart` that follows it, with every `Then` assertion unchanged; three consecutive green runs. See [`phase3g-103-hook-sessionend-repair/README.md`](phase3g-103-hook-sessionend-repair/README.md). |

## 4. F2 — the golden-frame settle flake: no recurrence found

Phase 3f's F2 (`TestGoldenMinimumFrame` "frame kept changing after the
fixture rendered; not settled", `features/golden_frame_test.go:236`) is out
of scope for this phase by standing rule and must not be claimed fixed. A
grep of every log this phase produced —

```
$ grep -rl "not settled" docs/reports/phase3g* --exclude=phase3g-findings.md
$ grep -rl "settle" docs/reports/phase3g* --exclude=phase3g-findings.md
```

(this file itself is excluded — it is the one guaranteed false positive the
PRD warns about: a sweep that greps a report's own prose for citations flags
the report describing itself, since the words "not settled"/"settle" appear
above in this very section.) — finds no hit for `not settled` anywhere else
under `docs/reports/phase3g*` (checked against the tree at `050ca9f`); `settle`
alone hits six files, none of which mention `TestGoldenMinimumFrame` or the
settle flake —
`phase3g-030-reclaim-leaked-interactive-pipe/README.md` ("settles 75ms", about
the interactive pipe, unrelated), `phase3g-034-previewfit-derivation/README.md`
and `phase3g-035-previewfit-no-live-pane-latch/{README.md,tui.go.diff,
features-preview-sigwinch-green.log}` (previewFit's own "already settled"
vocabulary, R91, unrelated), and
`phase3g-038-r91-previewfit-latch/fix-9d6c22a-tui.go.diff` (the same R91 diff
quoted a second time). Task 014's own golden-frame regeneration run
(`phase3g-014-footer-fixed-set/golden-regen-and-pass.log`) shows
`TestGoldenMinimumFrame` passing on both its `run-1` and `run-2` internal
repeats. **This is not proof F2 is fixed** — it was never reproduced reliably
even in Phase 3f (one hit in a `-count=10` run) — only that this phase's own
work did not trip it. The only run with the statistical power to say more is
task 041's `ci/stability.sh 10`, which has not run yet as of this file; if it
surfaces F2, that recurrence and its log path belong in task 041's or 042's
own report, not retroactively edited into this one.

## 5. F7, F8, F10, F14 — disposition

The PRD's non-goals section names these four explicitly: F7 (theme palette,
out of scope entirely — "Theme palette changes" is its own non-goal bullet)
and F8/F10/F14 ("documentation nits in `docs/reports/`. Fix them if
convenient; they block nothing."). None of the four's files were touched by
any phase3g commit:

```
$ git log --oneline 1cfbd5a..050ca9f -- \
    internal/theme/builtin/cobalt.toml internal/theme/builtin/daylight.toml \
    internal/theme/builtin/empire.toml internal/theme/builtin/parchment.toml \
    docs/reports/phase2b2-findings.md docs/reports/phase2b2.md \
    features/new_session_selection.feature features/pty_driver_test.go
(empty)
```

- **F7** (four built-ins collide 2–5 status tokens onto one ANSI slot at 16
  colours) — **untouched, still open, explicitly out of scope.** R84 (task
  023, still pending) only *adds coverage* to the contrast floor over the
  pairs R82's dialogs use; it does not touch quantisation or any palette
  file, and the PRD forbids recolouring a built-in to make a test pass
  regardless. No phase3g task recolours `cobalt`/`daylight`/`empire`/
  `parchment`.
- **F8** (`phase2b2.md` cites "Task 034" by title; five headings in
  `phase2b2-findings.md` match) — **untouched, still open, not attempted.**
  It is a stale citation in a Phase 2b2 report, unrelated to any phase3g
  requirement; "fix if convenient" was not exercised because no phase3g task
  had reason to open that file.
- **F10** (`new_session_selection.feature:12-15`'s comment cites a `starting`
  attention rank the assertion no longer depends on) — **untouched, still
  open, not attempted**, for the same reason: no R76–R92 requirement touches
  that feature file or its comment.
- **F14** (F1's own Phase 3f write-up said `ResizeAndAwaitRender` "does not
  wait", which was a wrong sentence about a helper that does wait, not a
  helper defect) — **untouched, still open, not attempted.** Nothing in this
  phase changed `features/pty_driver_test.go` or re-argued F1; F14 was
  already corrected in place in `phase3f-findings.md` itself ("the sentence
  was wrong, not the helper, so nothing was changed") and needed no further
  action here.

## 6. How to re-check every citation in this report

Every backticked sha above resolves under `git cat-file -e`; every backticked
repo-relative path (a directory-qualified `.go`, `.md`, `.toml` or `.feature`
file, with an optional `:line` suffix) names a file that exists in the tree at
`050ca9f`. Spot-check:

```
$ for sha in 2eed8de a03527c 89fcffc 15e33c6 9991689 8cff03b cc36cfa \
             c791a6a 1c8cbad e699c31 3247a7a 050ca9f; do \
    git cat-file -e "$sha" && echo "$sha ok"; done
$ for f in internal/tui/rename.go internal/theme/contrast_test.go \
           features/status_recovery.feature features/agent_steps_test.go \
           docs/reports/phase3g-029-inject-retained-dead-pane/README.md \
           docs/reports/phase3g-002-r76-field-route/README.md \
           docs/reports/phase3g-014-footer-fixed-set/golden-regen-and-pass.log \
           docs/reports/phase3g.md docs/reports/phase3f-findings.md \
           internal/interactive/render_test.go \
           docs/reports/phase3g-030-reclaim-leaked-interactive-pipe/README.md \
           docs/reports/phase3g-030-reclaim-leaked-interactive-pipe/criterion-packages-run1.log \
           prds/phase3g-field-backlog.md \
           SPEC.md /run/ralphd/artifacts/task016-unsatisfiable/README.md; do \
    test -e "$f" && echo "$f ok"; done
```

`prds/phase3g-field-backlog.md` (this phase's PRD; the file name the earlier
draft of this report cited, `prds/phase3g-residuals-and-suite-determinism.md`,
was wrong — that stem belongs to Phase 3f's PRD,
`prds/phase3f-residuals-and-suite-determinism.md`) is a protected path for this
job and is quoted, never edited — every correction in [§2](#2-what-this-prd-got-wrong)
is disclosed here rather than fixed in place, in the same spirit as
`phase3f-findings.md`'s treatment of its own PRD's stale citations
([§1.3 there](phase3f-findings.md#1-what-this-prd-got-wrong-with-the-corrected-reading)).
`/run/ralphd/artifacts/task016-unsatisfiable/` is outside this repository
(the run's artifacts directory, not `docs/reports/`), cited because task 016's
own reproduction lives there and nowhere in the tree; it is not covered by a
committed citation checker and is spot-checked by hand above.

**Addendum (task 110, approach 02):** F18-F21's resolutions and the new F24-F26
rows above cite shas and paths this checklist did not yet cover at `050ca9f`.
Spot-check, against the tree at task 110's own HEAD:

```
$ for sha in ea6ce4b 045a6e6 fe79040 57a6882 0219e42 51b7f17 89682e5 2549406; do \
    git cat-file -e "$sha" && echo "$sha ok"; done
$ for f in docs/reports/phase3g-105-rename-selection/README.md \
           docs/reports/phase3g-104-title-independent-waits/README.md \
           docs/reports/phase3g-106-contrast-floor/README.md \
           docs/reports/phase3g-101-agent-wait-wrap/README.md \
           docs/reports/phase3g-102-clear-recents-label/README.md \
           docs/reports/phase3g-103-hook-sessionend-repair/README.md \
           features/settings.feature internal/service/reconcile.go; do \
    test -e "$f" && echo "$f ok"; done
```

Both loops are captured verbatim in
[`phase3g-110-findings-update/citation-check.log`](phase3g-110-findings-update/citation-check.log).
