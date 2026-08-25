# Task 307 — R53 godog scenarios for all four sort orders, on a fixture where they differ

Feature: `features/sort_order.feature` (tag `@sort-order`, one scenario per order,
tags `@requirement-53-sort-order-attention`/`created`/`activity`/`name`).
New harness code: `features/sort_order_test.go` — one new step, `the state
database session "X" has created_at N seconds ago` (direct-state.db precedent
from `features/attention_sort_test.go`'s `setSessionStatusSecondsAgo`; unlike
that step this one never touches `status`/`status_at`, so it cannot race
`internal/service.reconcile`'s own status promotion — `created_at` is written
once at creation and reconcile never revisits it).

## The shared fixture and its four expected sequences

Four shell sessions — `ord-alpha`, `ord-bravo`, `ord-charlie`, `ord-delta` —
each scenario poses the SAME status/status_at/created_at fixture (only
`[ui] sort_order` in `config.toml` differs):

| session      | status    | status_at (seconds ago) | created_at (seconds ago) |
|--------------|-----------|--------------------------|---------------------------|
| ord-alpha    | running   | 20                       | 35                        |
| ord-bravo    | error     | 40                       | 5                         |
| ord-charlie  | idle      | 10                       | 25                        |
| ord-delta    | waiting   | 30                       | 15                        |

Expected rendered order per `sort_order` value:

```
attention: ord-delta, ord-bravo, ord-alpha, ord-charlie   (waiting -> error -> running -> idle)
created:   ord-bravo, ord-delta, ord-charlie, ord-alpha   (created_at descending, newest first)
activity:  ord-charlie, ord-alpha, ord-delta, ord-bravo   (status_at descending, newest first)
name:      ord-alpha, ord-bravo, ord-charlie, ord-delta   (case-insensitive ascending)
```

Every one of these four sequences is a DIFFERENT permutation of the same four
sessions — no two orders agree pairwise on this fixture (checked by hand,
listed above). A wrong comparator that happened to coincide with a different
order on this fixture would still fail its own scenario.

## A real race this fixture surfaced, and the fix

The first cut of this feature used `screen contains "ord-alpha"` as the
"has the fixture settled" gate before asserting order for the
created/activity/name scenarios. `ord-alpha` is visible on the VERY FIRST
frame (right after session creation, before the direct SQL writes are ever
read by a reconcile tick), so that gate does not actually wait for the
fixture writes to land. On this fixture, the pre-settle frame's natural
order (unmodified statuses, tie-broken by creation order) happens to equal
the intended `name` order by coincidence (sessions were created in
alpha/bravo/charlie/delta order) — so the `name` scenario passed even
against a comparator dispatch hard-wired to always return the attention
order (see the red proof below, first attempt, not kept). Fixed by gating on
`screen contains "waiting"` instead (mirroring
`features/attention_sort.feature`'s own settle gate) — `waiting` only ever
appears once `ord-delta`'s direct SQL status write has actually been read
and rendered by a reconcile pass, which is the same pass that also picked up
every other fixture write (one DB read, one sort, one frame).

## Green (this tree)

`ci/run.sh sh -c 'DECK_GODOG_TAGS=@sort-order go test -count=1 -v ./features/ -run TestFeatures'`

See `green.log`:

```
4 scenarios (4 passed)
67 steps (67 passed)
```

`features/attention_sort.feature` passes UNMODIFIED at this commit
(`git diff --stat features/attention_sort.feature` is empty):
`ci/run.sh sh -c 'DECK_GODOG_TAGS=@attention-sort go test -count=1 ./features/'` → `ok` (24.2s).

`ci/run.sh go test -count=1 ./internal/tui/ ./internal/config/` → both `ok`.

## Red (deliberately wrong comparator dispatch)

`internal/tui/sort_order.go`'s `sortSessionsByOrder` was temporarily replaced
with a body that ignores `order` and always returns
`sortSessionsByAttentionStable` (tasks 304/305's dispatch broken), then
`ci/run.sh sh -c 'DECK_GODOG_TAGS=@sort-order go test -count=1 -v ./features/ -run TestFeatures'`
was re-run. See `red-wrong-comparator-reverted.log`:

```
4 scenarios (1 passed, 3 failed)
--- PASS: TestFeatures/sort_order_defaults_to_attention_and_orders_by_the_waiting/error/running/idle_tiers
--- FAIL: TestFeatures/sort_order_=_created_orders_by_created_at_descending,_newest_first
--- FAIL: TestFeatures/sort_order_=_activity_orders_by_status_at_descending,_most_recently_changed_first
--- FAIL: TestFeatures/sort_order_=_name_orders_case-insensitively_ascending
```

(The `attention` scenario legitimately still passes — attention IS what the
broken dispatch returns.) `internal/tui/sort_order.go` was restored
byte-identical afterwards (diff-verified against git) before committing.
