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

Task 136 (`fcdb994`, docs-only descendant) and this report are the only commits after
`b9243a1` at time of writing; neither touches a `*.go`/`*.feature` path, so `b9243a1` remains
the final code sha both gates (tasks 127/129) and every R103 document in this phase measure
against. `git status --porcelain` is empty and `git rev-parse HEAD origin/main` agree at the
end of this task's commit.

## Per-requirement table

| req | status | tasks | shas | evidence |
|---|---|---|---|---|
| R98 | met | 104–109 | `a65190e`, `0177991`, `e31c37e`, `8d1b41a`, `17e185c`, `7e141ad`, `a063296`, `8e5e036`, `55c0818` | `internal/tui/interactive.go`, `internal/tui/tui.go`, `internal/tui/help_keymap_parity_test.go`, `internal/tui/footer_handler_agreement_test.go`, `internal/tui/refusal_f_naming_test.go`, `internal/tui/force_enter_test.go`, `internal/tui/force_indistinguishable_test.go`, `features/interactive_force_attach.feature` |
| R99 | met | 101, 102, 121 | `eb1e917`, `244f66f`, `d7f149c` | `internal/tmux/ownership.go`, `internal/tmux/force_ownership_test.go`, `features/interactive_force_attach.feature` (the `↵` refused, `F` wins, A is told scenario) |
| R100 | met | 110–115 | `a790061`, `e6157d8`, `4ef6d28`, `c525b35`, `cbc6692`, `625c663`, `cb27c5c`, `1b71c74` | `internal/tmux/isize_geometry.go`, `internal/tmux/isize_geometry_test.go`, `internal/tui/isize_geometry_entry_test.go`, `internal/tui/teardown_still_mine_gate_test.go`, `internal/tui/teardown_transport_gate_test.go`, `internal/tui/failed_entry_unwind_test.go`, `internal/tmux/reclaim.go`, `internal/tmux/reclaim_test.go`, `internal/tui/double_steal_restore_test.go`, `features/interactive_force_attach.feature` (the geometry-survives-the-chain scenario) |
| R101 | met | 116, 117, 118, 120, 136 (119 excluded — see below); 121–123 (feature scenarios) | `4ac5970`, `c95e848`, `be7a387`, `5fb9e4f`, `e398881`, `fcdb994`, `d7f149c`, `8d0437f`, `704fd1a` | `internal/tui/lost_attach.go`, `internal/tui/lost_attach_test.go`, `internal/tui/lost_attach_swallow_test.go`, `internal/tui/interactive_displacement.go`, `internal/tui/interactive_displacement_test.go`, `internal/tui/interactive_fallout_store_test.go`, `internal/tui/displacement_teardown_test.go`, `features/interactive_force_attach.feature` (the `↵` refused/`F` wins/A is told, full-attach-displaces-the-holder and dialog-swallows-keys scenarios) |
| R102 | met | 124–126 | `7371c07`, `83e92d5`, `b9243a1` | `internal/tui/tui.go`, `internal/tui/preview_fit_foreign_claim_test.go`, `features/interactive_force_attach.feature` (the passive-fit-stands-down scenario) |
| R103 | in progress | 127–129 (gates, done); 130 (this report); 131–135 not yet committed | `0c022e8`, `4a9d745`, `3508c5a` | `docs/reports/phase3i-127-fullsuite/`, `docs/reports/phase3i-128-fullsuite-verbose/`, `docs/reports/phase3i-129-stability10/` |

### R101's task 119 disposition

Task 119 ("Prove the two displacement flavours tear down differently") is `failed`
(validation-exhausted) and this row does **not** rely on it for R101's met verdict. Its
attempted commits (`5732a15`, `c36ed84`) remain in the tree as unit-test history but are not
cited above as discharging R101. Task 136 (`fcdb994`, `validated`, dependsOn `["118"]` per
steer 001) proves the residual property task 119 was going after — the stolen displacement
flavour never invokes `ownership.Release` — by counting the ownership-option **reads** on the
tmux wire in `internal/tui/displacement_teardown_test.go` (exactly one `show-options` read of
`@deck_isize_owner`, the `claimStillMine` probe, and zero unsets), with the load-bearing check
demonstrated (a comment recording that adding a call to `ownership.Release(ctx)` after the
failed still-mine probe makes the test fail). R101's other four properties — detection on the
tick, the two-flavour teardown split, the dialog, and the falls-out-records-nothing guarantee
— are proved independently by tasks 116, 117, 118 and 120. 119's own disposition (why it
stayed `failed` rather than being repaired) is left to
docs/reports/phase3i-findings.md (task 131, not yet committed) per the standing rules and
steer 001.

## Gate results (tasks 127–129)

**Whole-suite sweep (task 127, `0c022e8`)** — `ci/run.sh go test -p=1 -count=1 ./...` at
`b9243a1`. Exit status quoted verbatim from
`docs/reports/phase3i-127-fullsuite/sweep.log.exitstatus`:

```
$ cat docs/reports/phase3i-127-fullsuite/sweep.log.exitstatus
0
```

**Verbose companion tally (task 128, `4a9d745`)** — the `-v` companion (finding F34's
two-sweep discipline), same tree, same sha. Tally quoted verbatim with source line numbers
from `docs/reports/phase3i-128-fullsuite-verbose/README.md` (sourced from `verbose.log`):

```
5642:319 scenarios (319 passed)
5643:3682 steps (3682 passed)
```

(Higher than phase 3h's 311/3532 because this phase's tasks — including 109, 115, 121, 122,
123 and 126 — added feature scenarios since then.) The report's own README also quotes two
further tally pairs at lines 5970-5971 and 5995-5996; those belong to
`TestGodogRejectsUndefinedAndFailedSteps`, a self-test fixture that deliberately feeds one
undefined and one failing step to prove the runner rejects them, and are not part of the
suite's own tally.

**Stability gate (task 129, `3508c5a`)** — `ci/stability.sh 10` at `b9243a1`. Final line
quoted verbatim from `docs/reports/phase3i-129-stability10/summary.log`:

```
10/10 passed
```

All 10 runs `PASS (exit 0)`; no failures to name, no recurrence of any out-of-scope carried
finding.

## Remaining R103 work

Not yet committed at the time of this report: docs/reports/phase3i-findings.md (task 131),
the `docs/DELIVERY-LOG.md` paragraph (task 132), the GH issue #19 section mapping (task 133),
the re-verification of both guards at the true final sha (task 134), and the closing report
(task 135). R103's row above is therefore `in progress`, not `met`; it will be revised to
`met` once those tasks land, citing their own commits.
