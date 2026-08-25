# Task 302 — R52 godog scenarios for auto-selection of a newly created session

Feature: `features/new_session_selection.feature` (tag `@new-session-selection`).
New harness code: `features/new_session_selection_test.go` (one new step:
`after one configured reconcile interval deck client "A" has session "X"
selected`, a non-polling check mirroring
`clientScreenStillContainsAfterReconcileInterval`'s reasoning but keyed on the
`> name` selection marker `clientHasSessionSelected` already establishes).

## How index-0 was ruled out

Both scenarios seed an "anchor" shell session and force its status to
`waiting` (attention rank 0, `internal/tui/attention.go`) via direct SQL
before creating the session under test. A freshly created session always
starts at status `starting` (attention rank 3 — behind waiting/error/running,
ahead of idle/stopped), so it is *provably* ordered after the anchor. Each
scenario asserts this placement directly with the existing
`deck client "A" screen shows sessions in this order:` step (table: anchor
then the new session) before asserting the new session is selected — so a
"selection happens to be at the top" stand-in cannot pass by accident: the
new session's row is asserted to be at index 1, not index 0, and *then* the
selection assertion runs against that same row.

## Scenarios

1. `creating a new session selects its row immediately, without any
   navigation keystroke, even though it does not land at index 0` — no key
   is sent between the create finishing and the selection assertion.
2. `the just-created row's auto-selection is one-shot -- moving away with k
   survives a later reconcile tick` — after the create's one-shot intent
   fires, `k` moves the selection back to the anchor, and the selection is
   asserted to survive a full further reconcile cadence (the new step above),
   proving the intent does not re-fire on a later `sessionsLoaded`.

## Green (this tree, HEAD `d7f304d` + this task's new files)

See `green.log`. Both scenarios pass:

```
--- PASS: TestFeatures (2.50s)
    --- PASS: TestFeatures/creating_a_new_session_selects_its_row_immediately,_without_any_navigation_keystroke,_even_though_it_does_not_land_at_index_0 (1.04s)
    --- PASS: TestFeatures/the_just-created_row's_auto-selection_is_one-shot_--_moving_away_with_k_survives_a_later_reconcile_tick (1.42s)
PASS
ok  	github.com/n-orlov/deck/features	2.519s
```

## Red (task 301's `internal/tui/tui.go` change reverted, this task's new
files kept)

See `red-task301-reverted.log`. Reverting task 301's wiring (`git apply -R`
of the `internal/tui/tui.go` half of commit `d7f304d`) leaves both scenarios
red — the new session is never selected because nothing records or consumes
the one-shot intent:

```
2 scenarios (2 failed)
19 steps (12 passed, 2 failed, 5 skipped)
--- FAIL: TestFeatures (14.33s)
    --- FAIL: TestFeatures/creating_a_new_session_selects_its_row_immediately,_without_any_navigation_keystroke,_even_though_it_does_not_land_at_index_0 (7.21s)
    --- FAIL: TestFeatures/the_just-created_row's_auto-selection_is_one-shot_--_moving_away_with_k_survives_a_later_reconcile_tick (7.09s)
FAIL
```

Command used for both runs:

```
ci/run.sh sh -c 'DECK_GODOG_TAGS=@new-session-selection go test -count=1 ./features/ -run TestFeatures -v'
```

`internal/tui/tui.go` was restored to `d7f304d`'s state (`git apply` of the
same diff) immediately after capturing the red log; `git status --short`
was empty for that file afterward.
