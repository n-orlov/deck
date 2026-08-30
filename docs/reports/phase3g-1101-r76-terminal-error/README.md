# Task 1101 — re-widen the R76 live-pane repair to every terminal row, stopped or error

## What changed

`internal/service/reconcile.go`'s live-pane repair trigger (inside `reconcile`'s
`!crashed` branch) is widened back to

```go
if session.Status == "stopped" || session.Status == "error" {
```

with no `StatusSource`/`PaneExitStatus` exception. This reverts task 902's (`a1ca33e`)
narrowing, per review finding 2 and operator steering
(`/run/ralphd/steering/003-approach8-stall-rule-and-clock.md`): `SPEC.md:560-566`
("A terminal row with a live pane is an invariant violation, and the same pass
repairs it. A terminal status — `stopped`, `error` — claims there is nothing
running here, and a live, non-dead pane is direct evidence against it.") draws no
exception by source or by whether the row already carries a pane-exit verdict, and
steering forbids hunting for a product-side carve-on for one. The comment above the
trigger, and the doc comment above `repairTerminalRowWithLivePane`, are rewritten to
cite `SPEC.md:560-566` as the precedence authority instead of finding F40/task 901.

`internal/service/reconcile_live_error_precedence_test.go` (task 902's
`TestReconcileNeverRepairsHookSourcedErrorWithLivePane` and
`TestReconcileNeverRepairsProbeSourcedErrorWithLivePane`, which pinned the now-reverted
exclusion) is deleted, and `internal/service/reconcile_bare_error_repair_test.go`
restores task 701's (`89edd3c`) `TestReconcileRepairsBareErrorRowWithLivePane`: a bare
hook-sourced `error` row (`StatusSource: "hook"`, `PaneExitStatus: nil`) paired with a
live, non-dead pane is corrected to `starting`/`tmux` and records exactly one
`tmux.terminal_pane_alive` event, in both the store and the audit log, with the pane
itself untouched (no kill/respawn/send-keys argv reaches tmux) and a repeat
`Reconcile` a no-op.

## Evidence

### Red-before (narrowed trigger restored in a scratch worktree)

A detached worktree at `edc6680` (the narrowed-trigger sha immediately before this
task's fix) with only the new test file copied in, run with:

```
ci/run.sh sh -c 'cd .scratch-1101 && go test -count=1 ./internal/service/ -run TestReconcileRepairsBareErrorRowWithLivePane -v'
```

— `red-before.log` / `red-before.exitstatus` (**1**): the bare hook-sourced error row
is left untouched (`status="error" source="hook"`) because the narrowed trigger
excludes it.

### Green-after (this task's fix applied)

```
ci/run.sh go test -count=1 ./internal/service/ ./internal/tui/
```

— `green-after.log` / `green-after.exitstatus` (**0**): both packages `ok`.

## Scope note

Comments elsewhere that still describe the F40-narrowed exclusion in prose
(`internal/tui/tui.go:1582,2556-2557`, `internal/store/store.go:670-673`) are stale
after this widening but are out of this task's success criteria (which names only
`reconcile.go` and its test file); `store.go`'s actual gate logic
(`tmuxTerminalRepair`) already admits both `stopped` and `error` unconditionally by
source, so no behavioural change is needed there. Left for a later doc-consistency
pass (or task 1105/1106's scenario enumeration) to pick up if still relevant.
