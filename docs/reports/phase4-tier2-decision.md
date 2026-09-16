# Tier 1 → Tier 2 decision

## Decision

**Tier 2 (R128–R131, tasks 029–038) is NOT being started this run.** Every
task from 029 through 038 lands no code; each reads this record and is
satisfied by it alone, per the plan's `tier2_conditionality` clause. The
last code-touching task for this run is **026** (`codex covered in the
ordinary feature places`), not 038. The freeze line begins the moment task
026's own commit landed — every commit from task 027 onward (the Tier 1 gate
sweep and this record included) is record-only, checked with `git show
--stat`, exactly as the standing rules require for the "Tier 2 not started"
branch.

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
