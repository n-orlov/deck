# R76 evidence — the reconcile repairs a terminal row with a live pane (task 001)

Requirement: PRD R76 / `SPEC.md` §7. A row in a terminal status (`stopped`, `error`) under a
live, non-dead pane is an invariant violation: the reconcile corrects the row from what §7's
liveness rules can observe, records the correction as an event, and **touches the pane not at
all**. Dead-pane collection is still decided first.

Captured with `ci/run.sh` (sibling `deck-ci:local`) by reverting only the two product files
(`internal/service/reconcile.go`, `internal/store/store.go`) around the unchanged tests.

| log | product code | result |
|---|---|---|
| `red-1-no-repair.log` | `1cfbd5a` — the tree as the field found it | RED, 4 tests: the live-pane branch `continue`s, so the row stays `stopped`/`error` forever |
| `red-2-spent-verdicts-refreeze.log` | `89fcffc` — first attempt, status corrected but `killed_by_user` / `pane_exit_status` left set | RED, 2 tests: a user-killed row is not repaired at all (the `killed_by_user` precedence in `UpdateSessionStatus` sets `apply=false`), and a repaired crashed row keeps a spent crash verdict, which re-freezes it (`terminal` is `status=="stopped" \|\| PaneExitStatus != nil`) — §9.1's "spent verdict outranks everything forever" |
| `green-after-fix.log` | working tree at task 001's second commit | GREEN, all 4 repair tests plus the whole `./internal/service/` and `./internal/store/` packages |

The pane-survival half of the requirement is asserted by
`internal/service/reconcile_terminal_repair_fake_tmux_test.go`: tmux is driven through
`tmux.Client.Binary` (a seam the product already depends on — deck configures the binary it
runs), the double records every argv it receives, and the test fails if any of
`kill-session`, `kill-pane`, `kill-server`, `respawn-pane`, `respawn-window` or `send-keys`
appears — while also failing if the double was never consulted, so a vacuous pass is not
possible. Both fake-tmux tests run a second consecutive `Reconcile` and assert the row, its
`status_at` and the single correction event are unchanged: the repair is unleased and
idempotent, because once corrected the row is no longer terminal.

No TUI keypress is involved anywhere in these tests — the repair is `Reconcile`'s own work.
Task 002 proves the same repair through the field route (a hook writing `stopped` under a
surviving pane).
