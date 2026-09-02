# Phase 3i report

Phase 3i lands `F`, the force-attach steal of the interactive preview: a single-shot
ownership claim over a live holder (R99), the `F` entry path itself (R98), a durable window
option carrying the pre-any-deck geometry through an arbitrary chain of steals (R100), the
lost-attach dialog that tells and silences the displaced client (R101), the passive-fit
stand-down that keeps a second client's mere row selection from resizing a window it doesn't
hold (R102), and the record-matches-the-tree documentation requirement (R103).

## Final code sha

```
$ git log -1 --format=%H -- '*.go' '*.feature'
a559e7c61a00ab5fa31c2d98eaf5ce787744e4dc
```

This is the approach-2 final code sha, not the `b9243a1` this section named when approach 1
closed. Approach 2 reopened three requirements found stale on review and each landed a real
code commit after `b9243a1`: task 201 (`c025c54`, `internal/tui/tui.go`'s helpText F entry),
task 202 (`5b554f2`, `internal/tui/help_force_semantics_test.go`, pinning that wording to the
force path's real semantics), and task 203 (`a559e7c`, `internal/tmux/restore_plain_unset_test.go`,
pinning `RestoreWindowGeometry`'s plain-unset shape against a set window-size) — the last of
the three, `a559e7c`, is now the final code sha. Everything after `a559e7c` at the time of
writing is docs-only (task 208's finding, this refresh, and whatever follows in this approach),
so `a559e7c` is what both gates and every R103 document in this phase must now measure
against; both gates (tasks 127/129, captured at `b9243a1`) and every prior R103 document are
stale against this new sha and are disclosed as such rather than silently superseded. Task
136's `fcdb994` remains a code commit that is an ancestor of `b9243a1`, landing before either
final code sha rather than after it. `git status --porcelain` is empty and
`git rev-parse HEAD origin/main` agree at the end of this task's commit.

## Per-requirement table

| req | status | tasks | shas | evidence |
|---|---|---|---|---|
| R98 | met | 104–109; 201, 202 (approach-2 correction) | `a65190e`, `0177991`, `e31c37e`, `8d1b41a`, `17e185c`, `7e141ad`, `a063296`, `8e5e036`, `55c0818`, `c025c54`, `5b554f2` | `internal/tui/interactive.go`, `internal/tui/tui.go`, `internal/tui/help_keymap_parity_test.go`, `internal/tui/footer_handler_agreement_test.go`, `internal/tui/refusal_f_naming_test.go`, `internal/tui/force_enter_test.go`, `internal/tui/force_indistinguishable_test.go`, `features/interactive_force_attach.feature`, `internal/tui/help_force_semantics_test.go` |
| R99 | met | 101, 102, 121 | `eb1e917`, `244f66f`, `d7f149c` | `internal/tmux/ownership.go`, `internal/tmux/force_ownership_test.go`, `features/interactive_force_attach.feature` (the `↵` refused, `F` wins, A is told scenario) |
| R100 | met, with a qualification (see below) | 110–115; 203 (approach-2 pin) | `a790061`, `e6157d8`, `4ef6d28`, `c525b35`, `cbc6692`, `625c663`, `cb27c5c`, `1b71c74`, `a559e7c` | `internal/tmux/isize_geometry.go`, `internal/tmux/isize_geometry_test.go`, `internal/tui/isize_geometry_entry_test.go`, `internal/tui/teardown_still_mine_gate_test.go`, `internal/tui/teardown_transport_gate_test.go`, `internal/tui/failed_entry_unwind_test.go`, `internal/tmux/reclaim.go`, `internal/tmux/reclaim_test.go`, `internal/tui/double_steal_restore_test.go`, `features/interactive_force_attach.feature` (the geometry-survives-the-chain scenario), `internal/tmux/restore_plain_unset_test.go` |
| R101 | met | 116, 117, 118, 120, 136 (not 119 — its disposition belongs to the phase 3i findings report, task 131); 121–123 (feature scenarios) | `4ac5970`, `c95e848`, `be7a387`, `5fb9e4f`, `e398881`, `fcdb994`, `d7f149c`, `8d0437f`, `704fd1a` | `internal/tui/lost_attach.go`, `internal/tui/lost_attach_test.go`, `internal/tui/lost_attach_swallow_test.go`, `internal/tui/interactive_displacement.go`, `internal/tui/interactive_displacement_test.go`, `internal/tui/interactive_fallout_store_test.go`, `internal/tui/displacement_teardown_test.go`, `features/interactive_force_attach.feature` (the `↵` refused/`F` wins/A is told, full-attach-displaces-the-holder and dialog-swallows-keys scenarios) |
| R102 | met | 124–126 | `7371c07`, `83e92d5`, `b9243a1` | `internal/tui/tui.go`, `internal/tui/preview_fit_foreign_claim_test.go`, `features/interactive_force_attach.feature` (the passive-fit-stands-down scenario) |
| R103 | in progress | 127–129 (gates, done); 130 (this report); 131–135 not yet committed | `0c022e8`, `4a9d745`, `3508c5a` | `docs/reports/phase3i-127-fullsuite/`, `docs/reports/phase3i-128-fullsuite-verbose/`, `docs/reports/phase3i-129-stability10/` |

**R100's qualification.** The PRD's R100 unit-evidence bullet reads literally as a
value-preserving restore ("the `window-size` value including its set shape"). SPEC.md 11.9
and PRD phase3b II-9 both name a plain, unconditional unset as the exit recipe's last step
instead, and the shipped product (`internal/tmux/geometry.go:243`'s unconditional
`unsetWindowSize` call) follows SPEC, not that literal PRD wording. Task 203's
`internal/tmux/restore_plain_unset_test.go` (`a559e7c`) pins this: it sets `window-size`
window-locally before capture and asserts the option reads back UNSET, not restored to that
set value, after `RestoreWindowGeometry`. Per the standing rule that SPEC wins where the PRD
and SPEC disagree, this is deliberate, not a defect, and R100 is still "met" against the
SPEC-governed behaviour the product actually ships. The full disagreement, its authority
resolution, and the evidence that `internal/tui/double_steal_restore_test.go` only ever proved
byte-exact restoration for the unset shape are recorded as numbered finding 5 of
`docs/reports/phase3i-findings.md` (task 208, `ae35983`) — see that finding for the complete
account; this row's "met, with a qualification" status points at it rather than repeating it.

R101's met verdict above rests only on tasks 116, 117, 118, 120 and 136 and on the feature
scenarios of tasks 121–123: detection on the preview tick and the dialog (116, 117, 118), the
falls-out-records-nothing guarantee (120), and the stolen flavour's teardown never reaching
`ownership.Release` (136, `fcdb994`). Task 119 is deliberately absent from that list and from
the row's shas; its status and disposition are recorded in the phase 3i findings report
(task 131), not here.

## GH issue #19 design section map (task 133)

GH issue #19, "Force-attach (F): steal the interactive preview from another holder", is this
phase's source (`prds/phase3i-force-attach.md`'s "Read this before anything else" says so
directly, and R103 requires this mapping). Its `## Design (agreed with operator, 2026-09-02)`
body is organised into six numbered subsections. This table names, for each one, which
requirement discharges it and the commit(s)/evidence already established in the
per-requirement table above — so closing #19 is a reading, not an argument.

**No `gh` CLI is available in this environment**, and **the issue itself was not edited from
here**: its text was read with a single authenticated `GET
https://api.github.com/repos/n-orlov/deck/issues/19` (HTTP 200, `state: open`, `comments: 1`,
none of them posted by this task) so the six subsection headings quoted below are exact —
no `POST`/`PATCH`/`PUT` was made against the issue or its comments, and a second `GET` after
the read showed the same `state: open` with the same comment count. The token used came from
the pre-existing `~/.git-credentials` credential helper entry and was never printed or passed
as a command argument.

| issue § | design subsection (verbatim heading) | requirement | tasks | evidence |
|---|---|---|---|---|
| 1 | "`f` = force-enter the interactive preview" (shipped as `F`; see PRD/SPEC's `F`-not-`f` ruling) | R98 | 104–109 | `internal/tui/interactive.go`, `internal/tui/tui.go`, `internal/tui/help_keymap_parity_test.go`, `internal/tui/footer_handler_agreement_test.go`, `internal/tui/refusal_f_naming_test.go`, `internal/tui/force_enter_test.go`, `internal/tui/force_indistinguishable_test.go`, `features/interactive_force_attach.feature` |
| 2 | "Force claim: exactly one winner" | R99 | 101, 102, 121 | `internal/tmux/ownership.go`, `internal/tmux/force_ownership_test.go`, `features/interactive_force_attach.feature` (the `↵` refused, `F` wins scenario, task 121, `d7f149c`) |
| 3 | "Original geometry survives steals" | R100 | 110–115 | `internal/tmux/isize_geometry.go`, `internal/tmux/isize_geometry_test.go`, `internal/tui/isize_geometry_entry_test.go`, `internal/tui/teardown_still_mine_gate_test.go`, `internal/tui/teardown_transport_gate_test.go`, `internal/tui/failed_entry_unwind_test.go`, `internal/tmux/reclaim.go`, `internal/tmux/reclaim_test.go`, `internal/tui/double_steal_restore_test.go`, `features/interactive_force_attach.feature` (the geometry-survives-the-chain scenario) |
| 4 | "The loser falls out, into a modal dialog" | R101 | 116, 117, 118, 120, 136 (119's residual, discharged by 136 — see the findings report); 121, 123 (feature scenarios) | `internal/tui/lost_attach.go`, `internal/tui/lost_attach_test.go`, `internal/tui/lost_attach_swallow_test.go`, `internal/tui/interactive_displacement.go`, `internal/tui/interactive_displacement_test.go`, `internal/tui/interactive_fallout_store_test.go`, `internal/tui/displacement_teardown_test.go`, `features/interactive_force_attach.feature` (the `F`-wins/A-is-told scenario, task 121, `d7f149c`; the dialog-swallows-keys scenario, task 123, `704fd1a`) |
| 5 | "Passive preview fit respects a live claim (pre-existing gap)" | R102 | 124–126 | `internal/tui/tui.go`, `internal/tui/preview_fit_foreign_claim_test.go`, `features/interactive_force_attach.feature` (the passive-fit-stands-down scenario) |
| 6 | "Full tmux attach stays as-is" | R101 (no code change of its own; the fall-out `a` now lands an interactive holder into is §4/R101's dialog) | 122 (feature scenario proving it) | `features/interactive_force_attach.feature` (the full-attach-displaces-the-holder scenario, task 122, `8d0437f`) |

Section 1's issue text still says the lowercase `f`; §11.9 and this phase's tasks 104–109
shipped the uppercase `F` per the operator's explicit ruling recorded in the PRD and standing
rules ("`F`, not `f`" — `f` stays §12's cross-session search). That is the one place the
shipped requirement's letter differs from the issue's own draft text; the *design* — skip the
attached-client refusal, steal the live claim — is what R98 discharges, unchanged.

The issue's "## Verification (when implemented)" and "## Out of scope" subsections are not
numbered design subsections and are not rows above; the "Out of scope" bullets (no detach of
the other client, no per-keystroke ownership verification) match this phase's own Non-goals
verbatim and are honoured by omission, not by a citable commit.

## Gate results (tasks 127–129)

**Whole-suite sweep (task 127, `0c022e8`)** — `ci/run.sh go test -p=1 -count=1 ./...` at
`b9243a1`. Published log path: `docs/reports/phase3i-127-fullsuite/sweep.log` (README at
`docs/reports/phase3i-127-fullsuite/README.md`). Exit status quoted verbatim from
`docs/reports/phase3i-127-fullsuite/sweep.log.exitstatus`:

```
$ cat docs/reports/phase3i-127-fullsuite/sweep.log.exitstatus
0
```

**Verbose companion tally (task 128, `4a9d745`)** — the `-v` companion (finding F34's
two-sweep discipline), same tree, same sha. Published log path:
`docs/reports/phase3i-128-fullsuite-verbose/verbose.log` (exit status in
`docs/reports/phase3i-128-fullsuite-verbose/verbose.log.exitstatus`, README at
`docs/reports/phase3i-128-fullsuite-verbose/README.md`). Tally quoted verbatim with source
line numbers from that log:

```
5642:319 scenarios (319 passed)
5643:3682 steps (3682 passed)
```

(Higher than phase 3h's 311/3532 because this phase's tasks — including 109, 115, 121, 122,
123 and 126 — added feature scenarios since then.) That same log holds two
further tally pairs at lines 5970-5971 and 5995-5996; those belong to
`TestGodogRejectsUndefinedAndFailedSteps`, a self-test fixture that deliberately feeds one
undefined and one failing step to prove the runner rejects them, and are not part of the
suite's own tally.

**Stability gate (task 129, `3508c5a`)** — `ci/stability.sh 10` at `b9243a1`. Published log
path: `docs/reports/phase3i-129-stability10/summary.log` (exit status in
`docs/reports/phase3i-129-stability10/summary.log.exitstatus`, README at
`docs/reports/phase3i-129-stability10/README.md`). Final line quoted verbatim from that log:

```
10/10 passed
```

All 10 runs `PASS (exit 0)`; no failures to name, no recurrence of any out-of-scope carried
finding.

## Remaining R103 work

Not yet committed at the time of this report: the re-verification of both guards at the true
final sha (task 134) and the closing report (task 135). The phase 3i findings report (task
131, which also carries task 119's disposition), the `docs/DELIVERY-LOG.md` paragraph (task
132) and the GH issue #19 design section map above (task 133) are now landed. R103's row
above is therefore still `in progress`, not `met`; it will be revised to `met` once 134 and
135 land, citing their own commits.
