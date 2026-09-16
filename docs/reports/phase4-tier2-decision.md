# Tier 1 → Tier 2 decision

## Approach 2 re-affirmation (cure-and-reverify)

This run's approach 1 (recorded above) already decided Tier 2 (R128–R131) is
not started; approach 2 is a bounded cure of review's B0–B3 and R1 findings
over Tier 1's already-landed work (tasks 001–011), not a rebuild, and it
changes nothing about that decision. **Tier 2 (R128–R131) is still NOT
started in this approach, and no Tier 2 code has landed.**

Budget at the moment this section was written (`/run/ralphd/status.json`,
approach 2, iteration 173): `iterationsBudget` 800, `iterationsUsed` 167 →
**633 iterations remaining**; `deadlineAt` `2026-09-17T11:00:06Z` against
`updatedAt` `2026-09-16T23:24:11Z` → **~11h36m of wall-clock remaining**.
Approach 2's own scope was exactly review's B1, B2, B3 and R1 (plus the B0
measurement protocol already answered in approach 1) — ten small,
single-purpose tasks (002–011), each a bounded fix or regression test over
already-shipped code, not a new feature surface. Tier 2's four requirements
(a Go store-schema migration that is explicitly all-or-nothing, a sidebar
grouping-model replacement across four call sites with flat mode removed
outright, a create-modal field and an `i`-dialog move path, and a full
settings CRUD surface) remain, requirement-for-requirement, the same
materially larger, higher-risk body of work approach 1 already declined to
start under a comparable wall-clock deadline; nothing about curing B0–B3/R1
narrows that gap, and starting Tier 2 now would still leave no safety margin
to land the mandatory tail (013 full gate, 014 guards, 015 stability sweep,
016–018 report/findings/DELIVERY-LOG) if a migration ran out of wall-clock
mid-flight.

**This approach's last code-touching task is task 011** (`tui: sidebar
row's permission badge follows SPEC §11's non-safe rule`, commit
`e93a790`), per the freeze line: task 012 (this record) onward is
record-only (`docs/reports/`, `docs/DELIVERY-LOG.md`, `docs/roadmap.md`
only).

The resulting gap is unchanged and remains a disclosed non-finding, not a
finding: SPEC.md describes group-based, manually-organized session
organization (R128–R131 — user-defined groups, group CRUD, moving a
session between groups), while the shipped code still groups sessions by
workspace (`store.go`'s `DefaultWorkspace`, `internal/tui`'s
workspace-based grouping gated by `ui.group_by_workspace`); this gap is
explicitly disclosed here — once at approach 1's decision above and
re-affirmed in this paragraph — and is never scored in
`docs/reports/phase4-findings.md`, and never "cured" by editing SPEC.md.

## Decision

**Tier 2 (R128–R131, tasks 029–038) is NOT being started this run.** Every
task from 029 through 038 lands no code; each reads this record and is
satisfied by it alone, per the plan's `tier2_conditionality` clause.

This is therefore the plan's "Tier 2 not started" branch, whose
last code-touching task the plan named as **026** (`codex covered in the
ordinary feature places`) rather than 038. That baseline was superseded
after the plan was written: the Tier 1 gate sweep (task 027) uncovered three
pre-existing red lanes, and each fix was carved into its own task — **045**
(`teardown_hooks.feature` text-badge assertion), **046**
(`internal/tui/tui.go`'s `renderStackedFrame` gutter drop) and **047**
(`interactive_focus.feature`'s stale `selection_idle` assertion) — whose
own tracked criteria each state, in the freeze-line rule's own terms, that
the task becomes the run's new last code-touching task, superseding 026,
then 045, then 046 in turn.

**So under the decision taken here, this run's last code-touching task is
047**, the last link of that chain (026 → 045 → 046 → 047); it is 047, not
026 and not 038. Its commit `db66965` (`features: fix stale
interactive_focus selection_idle text match (task 047)`) is the tail code
sha cited below. The freeze line began the moment task 047 completed — every
commit from `90388e3` (the Tier 1 gate sweep record) onward, this record
included, is record-only, checked with `git show --stat`, exactly as the
standing rules require for the "Tier 2 not started" branch.

## Tier 1 evidence

`docs/reports/phase4-tier1-suite/README.md` records the Tier 1 gate sweep:

- **Code sha**: `db66965` (`db669658ce20de10ef6aaad311c94f6830538436`), the
  most recent commit in history touching a `*.go` or `*.feature` file at the
  time of that sweep (task 047).
- **Result**: **PASS**. `ci/run.sh sh -c 'go test -p=1 -count=1 -timeout=40m
  ./...'` ran all 18 packages listed by `go list ./...`, exited `0`, no
  `FAIL` line, 7m00s wall-clock, default godog tag filter
  (`~@real-agents && ~@nightly`). A first, uncommitted attempt hit one
  settle-timing flake in `TestGoldenMinimumFrame` under load; three isolated
  reruns of that test alone all passed, confirming environmental flake, not
  a regression — the committed run is the clean re-run at the same sha.

Every Tier 1 requirement (R116–R127) is therefore green, which is the
precondition the plan sets for even considering Tier 2.

## The budget that decided it

Measured from `/run/ralphd/status.json` at the moment this decision was
written (iteration 105, task 028):

- **Iteration budget**: `iterationsBudget` 800, `iterationsUsed` 101 →
  **699 iterations remaining**.
- **Wall-clock budget**: `deadlineAt` `2026-09-17T11:00:02Z`; current time
  `2026-09-16T19:45:02Z` → **~15h15m remaining**.
- **Observed pace this run**: the worker phase began at iteration 14
  (`2026-09-16T07:30:24Z`) and had, by the time of this decision
  (`2026-09-16T19:43:32Z`, iteration 105), taken 92 worker iterations and
  12h13m of wall-clock to carry 30 tasks (001–027 plus the three fallout
  fixes 045–047) to `validated` — an average of **~3.1 iterations and
  ~24.4 minutes of wall-clock per task** on Tier 1, which is dominated by
  small, bounded, single-file changes.

Tier 2's ten tasks are not that shape: R128 is a store schema migration that
is explicitly all-or-nothing (a half-applied migration is a blocking
condition, not a resumable one); R129 replaces the sidebar's grouping model
across four call sites and removes flat mode outright; R130 and R131 add a
create-modal field, an `i`-dialog move path, and a full settings CRUD
surface (create/rename/delete, plus a non-empty-group deletion prompt) —
each materially larger than a typical Tier 1 task. Even a conservative 2–3×
multiplier over the Tier 1 per-task average puts Tier 2 alone at roughly
8–12 hours of wall-clock, on top of the final tail (039 final gate, 040
guards, 041 ten-run stability sweep at ~75 minutes by itself, 042–044
reports) which needs a further several hours regardless of the Tier 2
choice. That combined estimate does not fit inside the ~15h15m of
wall-clock remaining with any safety margin, even though the **699
remaining iterations are not the binding constraint** — the wall-clock
deadline is. A migration that runs out of wall-clock mid-flight would leave
the store in a partially migrated state, which the standing rules and the
PRD both call a blocking condition, not a recoverable one; there is no
budget left to safely absorb that risk and still land the mandatory tail
(039–044).

## Expected outcome, stated plainly

**An unstarted Tier 2 is an expected budget outcome.** It carries one
accepted, explicitly-not-a-finding consequence: the SPEC-versus-code
grouping gap (SPEC's group-based session organization, R128–R131, remains
unimplemented; the code continues to group sessions by workspace via
`store.go`'s `DefaultWorkspace` and `internal/tui`'s workspace-based
grouping, gated by `ui.group_by_workspace`). This is not scored as a finding
in `docs/reports/phase4-findings.md` — it is disclosed here, and tasks
029–038 will each read this record and land no code, keeping the tail
(039–044) reachable exactly as the plan's `tier2_conditionality` clause
describes.
