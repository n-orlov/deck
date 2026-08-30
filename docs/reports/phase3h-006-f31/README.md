# Task 006 — forced-interleaving red-before, F31's reconcile lost update

## What this pins

`reconcile.go`'s shell-liveness promotion (the `StatusUpdateInput` carrying
`EventKind: "tmux.shell_live"`, found by grepping that literal, never by line
number — R96) takes its status verdict from the in-memory row
`Store.ListSessions` snapshotted *before* `TMux.List` runs, and writes
`running`/`tmux` unconditionally: no `AllowedCurrentStatuses` guard exists on
that write today. If a second writer (a hook, in this test) lands a status
between that snapshot and the promotion write, the promotion clobbers it —
the lost update F31 names.

`internal/service/reconcile_promotion_lost_update_test.go` forces that
interleaving deterministically with a test-side seam only: a fake tmux script
(behind `tmux.Client.Binary`, the same seam
`reconcile_terminal_repair_fake_tmux_test.go` already uses) whose
`list-sessions` branch blocks on a rendezvous file (`list-sessions.started` /
`list-sessions.release` under the session's temp home) until released. Since
`reconcile()` calls `Store.ListSessions` first and only then `TMux.List`
(reconcile.go:48-53, with the ordering comment there explaining exactly why),
blocking inside the fake's `list-sessions` provably parks the pass strictly
after the snapshot and strictly before the promotion write. While parked, the
test writes `status="error", source="hook"` directly through
`Store.UpdateSessionStatus`, then releases the rendezvous file and lets
`Reconcile` run to completion.

The row is seeded `Status: "starting", StatusSource: "tmux"` (not `"user"`)
so it reaches the `session.Agent == "shell" && session.Status == "starting"`
promotion branch instead of being caught by reconcile's own `terminal`
short-circuit for a `starting`/`user` resume-in-flight row (kept in the notes
file as the gotcha from F31's earlier, now-dead 3g repro attempt).

## Command and result (red-before)

```
ci/run.sh go test -count=1 -run TestReconcileLosesInterleavedStatusWriteDuringShellPromotion ./internal/service/
```

Exit status: `1` (`docs/reports/phase3h-006-f31/red-before.exitstatus`).
Full log: `docs/reports/phase3h-006-f31/red-before.log`.

Observed failure text (verbatim from the log):

```
--- FAIL: TestReconcileLosesInterleavedStatusWriteDuringShellPromotion (0.08s)
    reconcile_promotion_lost_update_test.go:148: lost update: interleaved verdict status="error" source="hook" was overwritten by the shell-liveness promotion, got row store.Session{ID:"00000000-0000-4000-8000-000000000051", Name:"shell about to be promoted", Slug:"shell-about-to-be-promoted", CWD:"/tmp/TestReconcileLosesInterleavedStatusWriteDuringShellPromotion1098126327/002", Agent:"shell", CapturedPath:"/bin/sh", Status:"running", StatusReason:"tmux pane is alive", StatusSource:"tmux", StatusAt:1735787045000, CreatedAt:1, Workspace:"002", KilledByUser:false, PaneExitStatus:(*int)(nil), CrashTail:"", NotifyEpoch:1, LastMessage:"", Acknowledged:false, LaunchArgs:[]string{}, Env:map[string]string{}, PreLaunch:"", LoginShell:false, PermissionProfile:"safe", PermissionProfileReason:"", ConversationID:"", ResumePin:"", ResumeState:"auto", LaunchGeneration:"", LastProbeAt:0, EnvDirty:false, DeletedAt:0, ArchivedAt:0}
FAIL
FAIL	github.com/n-orlov/deck/internal/service	0.082s
FAIL
```

The interleaved `error`/`hook` verdict is gone; the row instead reads
`Status:"running"`, `StatusSource:"tmux"` — exactly the fabricated
`running`/`tmux` state the promotion writes with no regard for what landed
during the gap it raced.

## Disclosure

**This test is red by design and stays red until task 007 lands the
`AllowedCurrentStatuses` guard on the shell-liveness promotion write.** No
product file was touched in this commit — the fix is deliberately left for
the next task so the regression is pinned on its own, reviewable commit
first.
