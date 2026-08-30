# Phase 3h report

Phase 3h brings the tree to the operator's §7 ruling in `de90a5c` (`spec: resolve §7's
error-under-live-pane self-contradiction and land §11.8's R93 wording (operator)`): **the
transition table is the design.** The repair covers a `stopped` row whatever its source, and
an `error` row that carries a `pane_exit_status` or a `tmux`/`user` source; a hook- or
probe-sourced `error` with no pane-exit verdict is `running → error`'s own live state and is
never repaired. This phase is four requirements — R94–R97 — three forward-reverts, one
one-token scenario re-point, one licensed product fix (F31), and the documentation refresh
that closes the phase.

## Final code sha

```
$ git log -1 --format=%H -- '*.go' '*.feature'
2ccb1d3c698d3c256aa94aea7ef2e3a58dc0641a
```

Task 007's commit `2ccb1d3` is the last commit in this phase to touch a `*.go` or
`*.feature` path; every document below (including this one) is a docs-only descendant of it.

## R94 — the reconcile repair matches §7 as ruled (tasks 001–003)

Forward-reverted, in order, the three commits that had re-pointed the suite away from the
narrow repair: `f700025` (reverts `a53146a`, `features/status_probe.feature`'s
stale-sampling scenario), `2f952da` (reverts `c176751`, task 001,
`features/status_claude_hooks.feature`'s `StopFailure` block) and `afa55b8` (reverts
`0b7dce5`, task 002, `internal/service/reconcile.go`'s narrow live-pane repair, restoring
`internal/service/reconcile_live_error_precedence_test.go` and removing
`reconcile_bare_error_repair_test.go`). Task 003 proved all three hold at HEAD and that
`status_attach.feature:18`, `status_claude_hooks.feature:6` and `status_probe.feature`'s
stale-sampling scenario pass in their pre-revert-range form, unedited beyond the reverts
themselves — evidence at `docs/reports/phase3h-001-status-probe-revert/`,
`docs/reports/phase3h-002-narrow-repair/` and `docs/reports/phase3h-003-r94-scenarios/`.

## R95 — the cwd-ghost scenarios track the shipped `hint` token (tasks 004–005)

Task 004 (`d578c03`) re-pointed `features/create_cwd_ghost.feature`'s three `Then` steps and
the `:14` scenario title from `dimmed` to `hint`. Task 005 (`2c2ec30`) re-pointed the two
`create_cwd_ghost_test.go` step helpers to `resolveScenarioTokenHex(ctx, "hint")`, adding the
new `cwdFieldLabelEndCol` helper so the field-label row (which also carries the `hint` token
per `SPEC.md:1355`) does not defeat the negative "no ghost" proof — evidence and the
collision writeup at `docs/reports/phase3h-005-hint-token/`.

## R96 — F31, the reconcile lost update, is fixed for real (tasks 006–007)

Task 006 (`8d6ed72`) landed a red-before test,
`TestReconcileLosesInterleavedStatusWriteDuringShellPromotion`, that forces a status write to
land between a reconcile pass's snapshot read and its shell-liveness promotion write, and
pins that the row must stay on the interleaved write rather than being clobbered back to
`running`/`tmux` — evidence at `docs/reports/phase3h-006-f31/`. Task 007 (`2ccb1d3`) added
the single field `AllowedCurrentStatuses: []string{"starting"}` to the
`EventKind: "tmux.shell_live"` `StatusUpdateInput` in `internal/service/reconcile.go`, making
task 006's test green without disturbing §7's precedence rules or the three R94 scenarios —
evidence at `docs/reports/phase3h-007-f31-guard/`.

## R97 — the record matches the tree (tasks 008–012)

### Whole-suite sweep (task 008)

Launched verbatim/unnarrowed at final code sha `2ccb1d3`. Exit status quoted from
`docs/reports/phase3h-008-fullsuite/.exitstatus` and its own `README.md`:

```
$ cat /tmp/phase3h-sweep.exitstatus
0
```

> Exit status **0**. All 17 packages pass (`ok`) or report no test files (`?`); nothing
> failed, nothing was skipped. The suite is green at the final code sha
> `2ccb1d3c698d3c256aa94aea7ef2e3a58dc0641a`.

### Verbose companion tally (task 009)

Docs-only descendant run, `-v` added and nothing else. Scenario tally quoted verbatim from
`docs/reports/phase3h-009-fullsuite-verbose/scenario-summary.txt`:

```
311 scenarios (311 passed)
3532 steps (3532 passed)
```

### Stability gate (task 010)

`ci/stability.sh 10` at final code sha `2ccb1d3`. Final line quoted verbatim from
`docs/reports/phase3h-010-stability10/summary.log`:

```
10/10 passed
```

Every one of the 10 runs is `PASS (exit 0)` with all 17 packages `ok`/`[no test files]`; no
recurrence of F2/F20/F22/F37 or the `filter.feature` dd/undo race to classify.

### Documentation refresh (tasks 011–012)

Task 011 (`08171ce`) appended one disposition sentence each to `docs/reports/phase3g.md`'s
R76 row and `docs/reports/phase3g-findings.md`'s F36/F38/F40/F41 rows, naming `de90a5c` and
this phase's three forward-revert commits. Task 012 (`5708f52`, corrected by `5042852`)
brought `docs/DELIVERY-LOG.md`'s Phase 3g paragraph to its final state of record, including
splitting F37's disposition into its already-fixed scenario half (3g's `46dad5e`) and the
product half this phase leaves open — evidence at `docs/reports/phase3h-012-delivery-log/`.

## Per-requirement table

| req | status | tasks | shas | evidence |
|---|---|---|---|---|
| R94 | met | 001–003 | `f700025`, `2f952da`, `afa55b8`, `9cd8f37`, `357867e`, `67cefcd`, `7634895` | `docs/reports/phase3h-001-status-probe-revert/`, `docs/reports/phase3h-002-narrow-repair/`, `docs/reports/phase3h-003-r94-scenarios/` |
| R95 | met | 004–005 | `d578c03`, `2c2ec30` | `docs/reports/phase3h-005-hint-token/` |
| R96 | met | 006–007 | `8d6ed72`, `2ccb1d3` | `docs/reports/phase3h-006-f31/`, `docs/reports/phase3h-007-f31-guard/` |
| R97 | met | 008–012 | `e686a97`, `1f919b7`, `2badb74`, `08171ce`, `5708f52`, `5042852` | `docs/reports/phase3h-008-fullsuite/`, `docs/reports/phase3h-009-fullsuite-verbose/`, `docs/reports/phase3h-010-stability10/`, `docs/reports/phase3h-012-delivery-log/` |

Every sha and path cited above (plus the R94/R95/R96 revert-target shas `a53146a`,
`c176751`, `0b7dce5` and the operator's ruling/plan shas `de90a5c`, `a24ff8d` named in
prose) is verified — `git cat-file -e <sha>^{commit}` and `git ls-files --error-unmatch
<path>` output for each — in `docs/reports/phase3h-013-report/README.md`.
