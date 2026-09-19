# Tier 1 → Tier 2 decision — phase 4b

## Decision

**GO.** Tier 2 (R128–R131, tasks 007–020) **is** being started this run.

Tier 1 (R133–R135, tasks 001–005) is fully validated — the sidebar/preview
scroll cue and its two follow-on fixes are landed and their targeted
evidence is committed under `docs/reports/phase4b-tier1-suite/` (task 005,
commit `ae81fee`). The plan's own gate for even considering Tier 2 is
therefore met, and the budget below shows comfortable room to attempt the
whole, all-or-nothing Tier 2 body (schema migration, sidebar rewrite,
create/detail wiring, settings CRUD, shared-`state.db` reload) plus the
mandatory tail (021 sweep, 022 guards, 023 stability, 024–026 records, 027
re-verify) without leaving the tree mid-migration if wall-clock runs out.

**Last code-touching task under this (GO) branch: task 020** (`groups:
another client's create, rename and delete become visible on the next
reload`), per the freeze-line rule in the Standing rules block — the
freeze begins once task 020 completes, and every commit from task 021's
launch onward is record-only.

## The budget that decided it

Snapshot taken from `/run/ralphd/status.json` at the moment this decision
was written (worker iteration 21, task 006), saved verbatim at
`/run/ralphd/artifacts/phase4b-tier2-budget.txt`:

- **Iteration budget**: `iterationsBudget` 800, `iterationsUsed` 21 →
  **779 iterations remaining**.
- **Wall-clock budget**: `deadlineAt` `2026-09-21T07:40:28Z`, `updatedAt`
  `2026-09-19T09:53:35Z` → **~45h47m (1 day 21h47m) remaining**.

## Observed pace on Tier 1 (the only measured data point this run has)

Worker-phase iteration log (`/run/ralphd/iterations/0010` through `/0020`):
tasks 001–005 (5 tasks, `estimatedIterations` summing to 8) ran across 11
worker+verify iteration-slots, from iteration 010's start
(`2026-09-19T08:23:14Z`) to iteration 020's end (`2026-09-19T09:53:35Z`) —
**1h30m21s wall-clock**, i.e. **~8.2 minutes per iteration-slot** and
**~18.1 minutes per task** including every verify pass (task 001 alone
needed two verify iterations, 011 and 012, before task 002's worker
iteration 013 started).

Tier 1's tasks are all small, single-file changes confined to
`internal/tui/interactive*.go` and one footer builder. Tier 2's tasks are
structurally larger — a store-schema migration (008, `estimatedIterations`
3), a full sidebar grouping-model replacement across four call sites (011,
013 — 3 each), a create-modal field and `i`-dialog move path (016, 017),
and a settings CRUD surface with two delete branches (018, 019 — 3 each).
Summing `estimatedIterations` across tasks 007–020 (Tier 2) gives **31**,
and across the mandatory tail 021–027 gives a further **10**, for **41**
estimated worker-iteration units of remaining planned work.

Applying this run's own measured Tier 1 rate (~11.3 minutes of wall-clock
per `estimatedIterations` unit, `90.35min / 8 units`) to those 41 units
gives a **~7.7-hour** naive estimate for the rest of the plan. Even a
generous 3× multiplier for Tier 2's larger surface area and the
migration's extra care (atomicity test, two-handle shared-DB test, guard
test, feature scenarios) puts the estimate at roughly **23 hours** — still
comfortably inside the **~45h47m** remaining, with the **779** remaining
iterations not the binding constraint at all (Tier 1 alone consumed only
11 of the 800-iteration budget for 5 tasks; even a 5× per-task cost for
Tier 2's 22 remaining tasks, 007–027, would consume on the order of
250–300 iteration-slots, well under 779).

Unlike Phase 4's own Tier 2 decision (`docs/reports/phase4-tier2-decision.md`,
which declined the tier with only ~15h15m of wall-clock left against a
comparable or larger body of remaining work), this run has roughly **3×**
the wall-clock runway and a larger iteration cushion, against materially
less total remaining work (Tier 1 is already fully validated here, whereas
Phase 4's decision point also carried a large fallout-fix tail). The
migration's irreversibility (Standing rules: "never leave the tree at a
commit where the schema bump has landed and its read path has not") is
respected by task 008 landing schema, migration, workspace removal and the
group-id read path in one commit and one transaction, exactly as task 008's
own `successCriteria` and the Standing rules require — this decision does
not relax that requirement, it only decides that there is enough runway to
reach it and everything after it.

## What GO commits this run to

- Tasks 007–020 land code under `internal/store`, `internal/tui`,
  `internal/config` and `internal/service`, replacing the workspace grouping
  model with SPEC's manual-groups model outright (no dual model, no
  `[ui] groups` switch — Standing rules "No two-grouping-models tree").
- Task 020 is the last code-touching task; the freeze line (Standing rules)
  begins the moment it completes.
- If the budget picture changes materially before task 020 completes (an
  unexpected cure cycle, a red lane that eats iterations), that is a fact to
  re-assess honestly at the time — this record is not a commitment to grind
  through a genuinely exhausted budget, and the Standing rules' "Tier
  discipline" section remains in force: budget beats ambition.
