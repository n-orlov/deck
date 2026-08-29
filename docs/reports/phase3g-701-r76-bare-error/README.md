# R76 bare-`error` evidence — review finding 1 (task 701)

Requirement: review finding 1 against `d266346`. `internal/service/reconcile.go`'s repair call
(`repairTerminalRowWithLivePane`) sat inside the `terminal` expression
(`stopped || PaneExitStatus != nil || (starting && source==user)`), which is never true for a
hook/probe `error` row that has no pane-exit verdict at all. That exact row — `Status: "error"`,
nil `PaneExitStatus`, under a live, non-dead pane — was therefore never repaired, contradicting
SPEC §7's rule that a terminal status paired with a live pane is always an invariant violation.

## Fix

`internal/service/reconcile.go`: the repair decision is now its own status test,
`session.Status == "stopped" || session.Status == "error"`, evaluated **before** and
independently of `terminal`. `terminal` itself is unchanged and keeps its other job — the
tmux-session-absent branch (`if terminal { continue }`, unmodified) — so a vanished session under
an `error` row is still not rewritten to a clean stop.

## Test

`internal/service/reconcile_bare_error_repair_test.go`,
`TestReconcileRepairsBareErrorRowWithLivePane`: builds an agent row with `Status: "error"`,
`StatusSource: "hook"`, nil `PaneExitStatus`, points the recording fake-tmux double
(`newFakeTMuxRepairService` / `fakeTMuxLivePane`, shared with the existing terminal-repair fake-
tmux tests) at a live pane for the row's session, and calls `Service.Reconcile` with no TUI
keypress anywhere in the test. Asserts:

- (a) the row is corrected: `starting` / `tmux`, one `tmux.terminal_pane_alive` event in the
  store and the audit log;
- (b) the pane survives: `assertNoPaneMutation` fails the test if any of `kill-session`,
  `kill-pane`, `kill-server`, `respawn-pane`, `respawn-window` or `send-keys` reached the fake
  tmux binary's argv log, and also fails if the double was never consulted (no vacuous pass);
- (c) the repair fires from `Reconcile` alone — nothing in the test touches the TUI;
- (d) a second `Reconcile` is a no-op: same status, source and `status_at`, no second event.

## Revert-and-reproduce evidence

Captured with `ci/run.sh` (sibling `deck-ci:local`), reverting only `internal/service/reconcile.go`
around the new, unchanged test (`git stash push -- internal/service/reconcile.go`, run, then
`git stash pop`):

| log | product code | result |
|---|---|---|
| `red-pre-fix.log` | pre-fix decision: repair call nested inside `if terminal` | RED — row stays `error`/`hook`, `PaneExitStatus` still nil, repair never called |
| `green-post-fix.log` | working tree with the fix | GREEN |

`ci-run-service-store.log` / `.exitstatus` is the mandated
`ci/run.sh go test -count=1 ./internal/service/ ./internal/store/`, exit `0`.
