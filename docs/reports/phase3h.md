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
4b1d4dcbd4480013470a0555795e6c64db3bf96d
```

Task 201's commit `4b1d4dc` (restoring `internal/service/reconcile.go`'s
`store.StatusUpdateInput` literal to gofmt-clean formatting) is now the last commit in this
phase to touch a `*.go` or `*.feature` path; every document below (including this one) is a
docs-only descendant of it. This **supersedes the earlier final code sha `2ccb1d3`**
(task 007's commit) — every remaining mention of `2ccb1d3` below is the superseded earlier
code sha the two original gate runs (tasks 008-010) were measured at, not the current one.

## R94 — the reconcile repair matches §7 as ruled (tasks 001–003)

Forward-reverted, in order, the three commits that had re-pointed the suite away from the
narrow repair: `f700025` (reverts `a53146a`, `features/status_probe.feature`'s
stale-sampling scenario), `2f952da` (reverts `c176751`, task 001,
`features/status_claude_hooks.feature`'s `StopFailure` block) and `afa55b8` (reverts
`0b7dce5`, task 002, `internal/service/reconcile.go`'s narrow live-pane repair, restoring
`internal/service/reconcile_live_error_precedence_test.go` and deleting the replacement test
file `0b7dce5` had added in its place — that file is absent from the tree at HEAD by design,
so it is cited here only through `afa55b8`'s own diff and never as a path). Task 003 proved
all three hold at HEAD and that `features/status_attach.feature:18`,
`features/status_claude_hooks.feature:6` and `features/status_probe.feature`'s
stale-sampling scenario pass in their pre-revert-range form, unedited beyond the reverts
themselves — evidence at `docs/reports/phase3h-001-status-probe-revert/`,
`docs/reports/phase3h-002-narrow-repair/` and `docs/reports/phase3h-003-r94-scenarios/`.

## R95 — the cwd-ghost scenarios track the shipped `hint` token (tasks 004–005)

Task 004 (`d578c03`) re-pointed `features/create_cwd_ghost.feature`'s three `Then` steps and
the `:14` scenario title from `dimmed` to `hint`. Task 005 (`2c2ec30`) re-pointed the two
`features/create_cwd_ghost_test.go` step helpers to `resolveScenarioTokenHex(ctx, "hint")`, adding the
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

## R97 — the record matches the tree (tasks 008–014, 201–209; 210–212 pending)

### Whole-suite sweep — original run, superseded (task 008)

Launched verbatim/unnarrowed at the now-**superseded** final code sha `2ccb1d3`. Exit status
quoted from `docs/reports/phase3h-008-fullsuite/.exitstatus` and its own
`docs/reports/phase3h-008-fullsuite/README.md`:

```
$ cat docs/reports/phase3h-008-fullsuite/.exitstatus
0
```

> Exit status **0**. All 17 packages pass (`ok`) or report no test files (`?`); nothing
> failed, nothing was skipped. The suite was green at the then-final code sha
> `2ccb1d3c698d3c256aa94aea7ef2e3a58dc0641a` — since **superseded** by task 201; see the
> re-run at the current final code sha below (task 202).

### Verbose companion tally — original run, superseded (task 009)

Docs-only descendant run, `-v` added and nothing else, at the superseded `2ccb1d3`. Scenario
tally quoted verbatim from `docs/reports/phase3h-009-fullsuite-verbose/scenario-summary.txt`:

```
311 scenarios (311 passed)
3532 steps (3532 passed)
```

Superseded by task 203's re-run at the current final code sha below.

### Stability gate — original run, superseded (task 010)

`ci/stability.sh 10` (the tracked script `ci/stability.sh`) at the superseded final code sha
`2ccb1d3`. Final line quoted verbatim from `docs/reports/phase3h-010-stability10/summary.log`:

```
10/10 passed
```

Every one of the 10 runs is `PASS (exit 0)` with all 17 packages `ok`/`[no test files]`; no
recurrence of F2/F20/F22/F37 or the `features/filter.feature` dd/undo race to classify.
Superseded by task 204's re-run at the current final code sha below.

### Documentation refresh (tasks 011–012)

Task 011 (`08171ce`) appended one disposition sentence each to `docs/reports/phase3g.md`'s
R76 row and `docs/reports/phase3g-findings.md`'s F36/F38/F40/F41 rows, naming `de90a5c` and
this phase's three forward-revert commits. Task 012 (`5708f52`, corrected by `5042852`)
brought `docs/DELIVERY-LOG.md`'s Phase 3g paragraph to its final state of record, including
splitting F37's disposition into its already-fixed scenario half (3g's `46dad5e`) and the
product half this phase leaves open — evidence at `docs/reports/phase3h-012-delivery-log/`.

### This report and its companion findings document (tasks 013–014)

Task 013 (`4673fb9`, `b5e4178`) published this report's original R94–R97 requirement table
and verified, in `docs/reports/phase3h-013-report/README.md`, every sha and path it cited at
that time. Task 014 (`ed469e4`) published the companion `docs/reports/phase3h-findings.md`.
Both commits deliver this phase and belong in the traceability table alongside every other
task; their earlier omission from it was found by review and is corrected here.

### Gofmt realignment forces both gates to re-run (task 201)

Task 201 (`4b1d4dc`) restored `internal/service/reconcile.go` to gofmt-clean formatting: the
`store.StatusUpdateInput` literal task 007's `2ccb1d3` added was gofmt-dirty from that commit
until this one (a source-format regression, not a behaviour change). Because it touches a
`*.go` path, it produces the new final code sha above and, per the standing rules, forces the
whole-suite sweep, the verbose tally and the stability gate to re-run at the new sha.

### Gate re-run at the new final code sha (tasks 202–204)

**Whole-suite sweep (task 202, `c9e88cb`)** — same mandated, unnarrowed command
(`ci/run.sh go test -p=1 -count=1 ./...`), launched at `4b1d4dc`. Exit status quoted verbatim
from `docs/reports/phase3h-202-fullsuite/.exitstatus`:

```
$ cat docs/reports/phase3h-202-fullsuite/.exitstatus
0
```

**Verbose companion tally (task 203, `fc358c0`; self-sha addendum `011b04b`)** — the `-v`
companion, same tree. Tally quoted verbatim with its source line numbers, from
`docs/reports/phase3h-203-fullsuite-verbose/verbose.log`:

```
5445:311 scenarios (311 passed)
5446:3532 steps (3532 passed)
```

**Stability gate (task 204, `79b56d9`)** — `ci/stability.sh 10` at `4b1d4dc`. Final line
quoted verbatim from `docs/reports/phase3h-204-stability10/summary.log`:

```
10/10 passed
```

captured exit status `0`; all ten runs `PASS (exit 0)`, zero recurrence of F2/F20/F22/F37 or
the `features/filter.feature` dd/undo race — full disposition at
`docs/reports/phase3h-204-stability10/README.md`.

The original `docs/reports/phase3h-008-fullsuite/`, `docs/reports/phase3h-009-fullsuite-verbose/`
and `docs/reports/phase3h-010-stability10/` evidence is **retained in the tree as superseded
history** — measured at the earlier final code sha `2ccb1d3` — and is no longer this phase's
current gate evidence; the current gate evidence is `docs/reports/phase3h-202-fullsuite/`,
`docs/reports/phase3h-203-fullsuite-verbose/` and `docs/reports/phase3h-204-stability10/`.

### Phase 3g disposition corrections and DELIVERY-LOG (tasks 205–208)

Task 205 (`52e9045`) rewrote `docs/reports/phase3g.md`'s R76 row's Phase 3h disposition
sentence from the actual diffs of `f700025`/`2f952da`/`afa55b8` (steering 001: the row had
claimed the reverts "touch neither the repair nor this row's own scenarios", which their
diffs contradict). Task 206 (`50960b9`) rewrote `docs/reports/phase3g-findings.md`'s
F36/F38/F40/F41 disposition sentences the same way. Task 207 (`9cff1ea`) brought
`docs/DELIVERY-LOG.md`'s Phase 3g section to a coherent final state of record, correcting the
stale claims steering 001 named ("R93 has no SPEC authority in this tree", false since
`de90a5c`, and `a5f8f6b` mislabelled as the final state rather than explicitly superseded).
Task 208 (`87bde8f`, validation fixes `0d9a551`/`f54873e`/`68ac4cf`) added
`docs/DELIVERY-LOG.md`'s Phase 3h paragraph at the new final code sha `4b1d4dc`, citing both
gate results, the protected-path ruling and this report.

### This report's own state-of-record refresh (task 209)

This section, the final-code-sha block above and the per-requirement table below were
brought current by task 209, whose own citation check is
`docs/reports/phase3h-209-report/README.md`. Tasks 210 (`docs/reports/phase3h-findings.md`'s
refresh), 211 (the guard re-verification report) and 212 (this report's close-out section)
are **pending** as of this commit and are not yet cited by sha.

## Per-requirement table

| req | status | tasks | shas | evidence |
|---|---|---|---|---|
| R94 | met | 001–003 | `f700025`, `2f952da`, `afa55b8`, `9cd8f37`, `357867e`, `67cefcd`, `7634895` | `docs/reports/phase3h-001-status-probe-revert/`, `docs/reports/phase3h-002-narrow-repair/`, `docs/reports/phase3h-003-r94-scenarios/` |
| R95 | met | 004–005 | `d578c03`, `2c2ec30` | `docs/reports/phase3h-005-hint-token/` |
| R96 | met | 006–007 | `8d6ed72`, `2ccb1d3` | `docs/reports/phase3h-006-f31/`, `docs/reports/phase3h-007-f31-guard/` |
| R97 | met | 008–014, 201–209 (210–212 pending) | `e686a97`, `1f919b7`, `2badb74`, `08171ce`, `5708f52`, `5042852`, `4673fb9`, `b5e4178`, `ed469e4`, `4b1d4dc`, `c9e88cb`, `fc358c0`, `011b04b`, `79b56d9`, `52e9045`, `50960b9`, `9cff1ea`, `87bde8f`, `0d9a551`, `f54873e`, `68ac4cf` | `docs/reports/phase3h-008-fullsuite/` (superseded), `docs/reports/phase3h-009-fullsuite-verbose/` (superseded), `docs/reports/phase3h-010-stability10/` (superseded), `docs/reports/phase3h-012-delivery-log/`, `docs/reports/phase3h-013-report/`, `docs/reports/phase3h-findings.md`, `docs/reports/phase3h-202-fullsuite/`, `docs/reports/phase3h-203-fullsuite-verbose/`, `docs/reports/phase3h-204-stability10/`, `docs/reports/phase3h-205-r76-disposition/`, `docs/reports/phase3h-206-3g-findings-disposition/`, `docs/reports/phase3h-207-delivery-log-3g/`, `docs/reports/phase3h-208-delivery-log-3h/` |

Every sha this report cites — the table's, plus the R94/R95/R96 revert-target shas
`a53146a`, `c176751`, `0b7dce5`, the operator's ruling/plan shas `de90a5c`, `a24ff8d` and
Phase 3g's `46dad5e` named in prose — and every path it cites, including the product,
feature and document paths named outside the table, is verified exhaustively in
`docs/reports/phase3h-209-report/README.md` (this task's own check, superseding task 013's
narrower `docs/reports/phase3h-013-report/README.md`, which is retained as superseded
history and still checked against the citations it covered): one quoted
`git cat-file -e <sha>^{commit}` run per sha and one quoted
`git ls-files --error-unmatch <path>` run per path, each with its exit status.
