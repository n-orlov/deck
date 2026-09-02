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
b9243a1f415ba9ca77cc2ffa2ec557ca8b1be4cb
```

Every commit after `b9243a1` at the time of writing is docs-only — the three gate captures
(`0c022e8` task 127, `4a9d745` task 128, `3508c5a` task 129) and this report — so `b9243a1`
remains the final code sha both gates (tasks 127/129) and every R103 document in this phase
measure against. Task 136's `fcdb994` is *not* one of them: it is a code commit (it changed
`internal/tui/displacement_teardown_test.go`) and it is an ancestor of `b9243a1`, landing
before the final code sha rather than after it. `git status --porcelain` is empty and
`git rev-parse HEAD origin/main` agree at the end of this task's commit.

## Per-requirement table

| req | status | tasks | shas | evidence |
|---|---|---|---|---|
| R98 | met | 104–109 | `a65190e`, `0177991`, `e31c37e`, `8d1b41a`, `17e185c`, `7e141ad`, `a063296`, `8e5e036`, `55c0818` | `internal/tui/interactive.go`, `internal/tui/tui.go`, `internal/tui/help_keymap_parity_test.go`, `internal/tui/footer_handler_agreement_test.go`, `internal/tui/refusal_f_naming_test.go`, `internal/tui/force_enter_test.go`, `internal/tui/force_indistinguishable_test.go`, `features/interactive_force_attach.feature` |
| R99 | met | 101, 102, 121 | `eb1e917`, `244f66f`, `d7f149c` | `internal/tmux/ownership.go`, `internal/tmux/force_ownership_test.go`, `features/interactive_force_attach.feature` (the `↵` refused, `F` wins, A is told scenario) |
| R100 | met | 110–115 | `a790061`, `e6157d8`, `4ef6d28`, `c525b35`, `cbc6692`, `625c663`, `cb27c5c`, `1b71c74` | `internal/tmux/isize_geometry.go`, `internal/tmux/isize_geometry_test.go`, `internal/tui/isize_geometry_entry_test.go`, `internal/tui/teardown_still_mine_gate_test.go`, `internal/tui/teardown_transport_gate_test.go`, `internal/tui/failed_entry_unwind_test.go`, `internal/tmux/reclaim.go`, `internal/tmux/reclaim_test.go`, `internal/tui/double_steal_restore_test.go`, `features/interactive_force_attach.feature` (the geometry-survives-the-chain scenario) |
| R101 | met | 116, 117, 118, 120, 136 (not 119 — its disposition belongs to the phase 3i findings report, task 131); 121–123 (feature scenarios) | `4ac5970`, `c95e848`, `be7a387`, `5fb9e4f`, `e398881`, `fcdb994`, `d7f149c`, `8d0437f`, `704fd1a` | `internal/tui/lost_attach.go`, `internal/tui/lost_attach_test.go`, `internal/tui/lost_attach_swallow_test.go`, `internal/tui/interactive_displacement.go`, `internal/tui/interactive_displacement_test.go`, `internal/tui/interactive_fallout_store_test.go`, `internal/tui/displacement_teardown_test.go`, `features/interactive_force_attach.feature` (the `↵` refused/`F` wins/A is told, full-attach-displaces-the-holder and dialog-swallows-keys scenarios) |
| R102 | met | 124–126 | `7371c07`, `83e92d5`, `b9243a1` | `internal/tui/tui.go`, `internal/tui/preview_fit_foreign_claim_test.go`, `features/interactive_force_attach.feature` (the passive-fit-stands-down scenario) |
| R103 | in progress | 127–129 (gates, done); 130 (this report); 131–135 not yet committed | `0c022e8`, `4a9d745`, `3508c5a` | `docs/reports/phase3i-127-fullsuite/`, `docs/reports/phase3i-128-fullsuite-verbose/`, `docs/reports/phase3i-129-stability10/` |

R101's met verdict above rests only on tasks 116, 117, 118, 120 and 136 and on the feature
scenarios of tasks 121–123: detection on the preview tick and the dialog (116, 117, 118), the
falls-out-records-nothing guarantee (120), and the stolen flavour's teardown never reaching
`ownership.Release` (136, `fcdb994`). Task 119 is deliberately absent from that list and from
the row's shas; its status and disposition are recorded in the phase 3i findings report
(task 131), not here.

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

Not yet committed at the time of this report: the phase 3i findings report (task 131, which
also carries task 119's disposition), the `docs/DELIVERY-LOG.md` paragraph (task 132), the GH issue #19 section mapping (task 133),
the re-verification of both guards at the true final sha (task 134), and the closing report
(task 135). R103's row above is therefore `in progress`, not `met`; it will be revised to
`met` once those tasks land, citing their own commits.
