# Task 807 — the single-row `x` handler consults `canKill` too (review finding 2)

## Review finding 2 restated for `x`

Review finding 2 named two actions with a footer/handler eligibility split.
The `A` half was closed by task 806 (`canArchive`, `docs/reports/phase3g-806-archive-eligibility/`).
This task closes the `x` half:

- the footer's `x` slot consults `canKill` (`internal/tui/tui.go:3669`,
  `footerRowEligible(m, true, canKill)`);
- the batch (marked-set) `x` path already consults `canKill` per row
  (`internal/tui/tui.go:2524`);
- the single-row `x` handler (`case "x"`, no marks) deliberately did **not**
  call `canKill` at all — it always dispatched `m.kill` and let
  `service.Kill`'s own already-stopped guard produce the refusal wording.

The old justification (comment on `canKill` and on `case "x"`, both removed
by this change) was issue #6: under `remain-on-exit failed` a row could read
`stopped` in the store while tmux still retained a **live** dead-or-alive
pane, so refusing on `Status` alone risked leaving that corpse un-collected
and the session name permanently unrecoverable. `service.Kill` did an extra
`tmux.Exists` round trip specifically to catch that case.

## Why the deferral is no longer needed

Two reconcile-side changes already closed the underlying gap, independently
of this task:

- **Crashed-pane collect-on-sight** (`internal/service/reconcile.go:68-79`,
  the `#6` comment above `terminal :=`): a dead pane's tmux session is
  captured and killed regardless of what the stored row says, on every
  reconcile pass. A stopped row can no longer be hiding an uncollected dead
  pane by the time a user presses `x` on it.
- **`repairTerminalRowWithLivePane`** (SPEC §7, `internal/service/reconcile.go:215-235`,
  invoked from the crashed-pane check at `reconcile.go:86-98`): a `stopped`
  (or bare `error`) row paired with a genuinely **live**, non-dead pane is
  corrected back to a non-terminal status (`starting`/`running`) instead of
  being left `stopped`. A stopped row can no longer be hiding a live pane
  either.

So a row `canKill` reads as `stopped` has, by construction of the reconcile
loop that runs ahead of every render, already had its corpse collected or
its status repaired. The single-row `x` handler can trust a locally-read
`Status` exactly as much as the footer and the batch path already do, and
the extra `tmux.Exists` round trip through the service is no longer buying
anything for the interactive path (it stays in `service.Kill` itself,
"belt-and-braces", per that function's own comment — only the *TUI key
handler's* deferral to it is removed here).

## The fix

`case "x"`'s single-row branch (no marks) now calls `canKill(session)`
before doing anything else:

- if false, it dispatches **no** kill command — `m.kill` is never called —
  and returns a `sessionKilled` message carrying a locally-constructed
  `errors.New("session is already stopped")`, the exact wording
  `service.Kill` used to produce, so `Cannot kill: session is already
  stopped` still renders via the existing `sessionKilled` branch in `Update`
  and `features/kill_delete_undo.feature:17`'s "screen contains \"already
  stopped\"" step keeps passing **unedited**.
- if true, it dispatches `m.kill(context.Background(), session)` exactly as
  before.

The comment above `canKill` and above `case "x"` no longer says "the
single-row path deliberately does NOT call this"; it names the reconcile's
collect-on-sight and the SPEC §7 repair (with file:line, both quoted above)
as what now frees a retained corpse before the handler is ever reached.

## Evidence

- `features-kill-status-recovery.log` (exit 0): `ci/run.sh env
  DECK_GODOG_PATHS=kill_delete_undo.feature,status_recovery.feature go test
  ./features/ -run TestFeatures -count=1`. `git show --stat` for this task's
  commit contains no line for `features/kill_delete_undo.feature` — the
  feature file is unedited.
- New test `internal/tui/kill_key_eligibility_test.go`,
  `TestKillKeyHandlerConsultsCanKillForTheSingleSelectedRow`: drives the real
  `case "x"` handler (via `Model.Update(key("x"))`) for a `stopped` row and a
  `running` row, with a fake `m.kill` that records whether it was called.
  - stopped row: asserts the fake `m.kill` is **not** called and the
    resulting `sessionKilled.err` contains `"already stopped"`.
  - running row: asserts the fake `m.kill` **is** called and the resulting
    `sessionKilled.err` is nil (the fake returns nil).
- `red-mutation-no-cankill-check.log` (exit 1) and
  `red-mutation-full-package.log` (exit 1): with the `if !canKill(session)`
  branch removed from `case "x"` (reverting to the old always-dispatch
  behaviour) the new test's `stopped row` subtest fails
  (`kill dispatched = true, want false`) and it is the **only** failure in
  the whole `internal/tui` package.
- `green-mutation-reverted.log` (exit 0): `ci/run.sh go test -count=1
  ./internal/tui/` after reverting the mutation (confirmed identical to the
  pre-mutation source by `diff`).

All logs were produced with `ci/run.sh`, each command run in the same shell
call that captured its exit status.
