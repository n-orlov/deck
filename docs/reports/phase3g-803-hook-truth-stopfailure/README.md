# Phase 3g — task 803: re-point the StopFailure block onto the post-repair state

## Mechanism (unchanged from task 703/801/802's diagnosis, applied here)

`status_claude_hooks.feature`'s "Every declared Claude hook maps to honest status
through both identity routes" scenario fires a genuine `StopFailure` Claude hook
through the fake Claude fixture's real pane while that pane **stays alive**
afterward (a later `UserPromptSubmit` retries through the same pane). The
released `deck _hook` subcommand (`cmd/deck/main.go`'s `runHook`) writes the
hook's own `error` verdict via `hookrecv.Receive`, then — for every hook except
`SessionEnd` — calls `liveness.ReconcileWithin` **synchronously, in the same
subprocess invocation, before returning**. SPEC §7's self-heal
(`internal/service.reconcile`'s `repairTerminalRowWithLivePane`, R76, reviewer-
approved as unconditional per `/run/ralphd/review-findings.md` finding 1) finds
this `error` row paired with the still-live, non-crashed pane and repairs it on
that same pass, before the hook subprocess (and therefore the fixture's
send-keys step) ever returns control to the test. There is no window, ever, in
which the raw `error` verdict is durably observable through this route — traced
and measured previously for the sibling scenario in `status_attach.feature`
(task 802, reported unsatisfiable there because that scenario's *next* step also
needs a live-pane `!` marker gated on status staying `error`, which the repair
also removes) and for this exact scenario/line in
`docs/reports/phase3g-703-r76-error-fallout-fix/README.md`'s "Not fixed yet"
section.

Unlike task 802, this scenario's own criteria do **not** require an unweakened
`!`-marker assertion after this step — only the `stop_failure` event/payload
assertion and the following `UserPromptSubmit` → `running` assertion, both of
which this task's diff leaves byte-identical. So instead of an unsatisfiable
report, the fix here is a genuine re-point: assert the state the repair
actually, deterministically leaves behind.

## Derivation of the post-repair values (confirmed against `internal/store/store.go`)

`repairTerminalRowWithLivePane` (non-shell branch) writes
`status="starting"`, `reason="tmux pane is alive; terminal row corrected"`,
`source="tmux"`, `ClearKilledByUser`/`ClearCrashVerdict` set,
`LastMessage=""` (case-empty, so the prior message is preserved by
`UpdateSessionStatus`'s `CASE WHEN ? = '' THEN last_message ELSE ?` clause).
`isAttentionStatus("error")` is true and `isAttentionStatus("starting")` is
false, so `UpdateSessionStatus`'s `leavingAttention` branch increments
`notify_epoch` once more on top of the epoch already at 2 after `Notification`
→ two attaches → `StopFailure`'s own write (which itself does not increment
`notify_epoch`, only sets `acknowledged=0`) — landing at 3. `acknowledged`
stays 0 (the repair's `input.Acknowledged` is nil, so `UpdateSessionStatus`
leaves whatever the hook write already set). This is exactly what the pre-change
red log's step-error message confirms verbatim (see below) and matches task
703's independently-derived diagnosis for the same scenario/line.

## The fix

- `features/status_claude_hooks_test.go`: added one new step,
  `the state database session "X" is repaired to "Y" from "Z" with reason
  "...", message "...", acknowledged N, and notify_epoch M`, backed by
  `databaseSessionIsRepairedTo` — the same polling shape as the existing
  `databaseSessionHasHookStatus`, except it checks the caller-supplied
  `status_source` instead of hard-coding `"hook"` (which the existing step
  does, and which is exactly why it can never observe this route's outcome).
- `features/status_claude_hooks.feature`: the `StopFailure` block's `Then`
  line now asserts `is repaired to "starting" from "tmux" with reason "tmux
  pane is alive; terminal row corrected", message "permission granted; work
  is complete", acknowledged 0, and notify_epoch 3` instead of the
  unreachable raw `"error"` verdict, with an explanatory comment above it
  (mirroring the file's own pre-existing SessionEnd comment's style). The
  `stop_failure` event/payload assertion right after it, and the following
  `UserPromptSubmit` → `"running"` assertion with its reason/message/
  acknowledged/notify_epoch fields, are untouched (`notify_epoch 3` there
  was already correct before this change, since it already anticipated the
  repair having happened).

No scenario added or removed (3 before, 3 after). `features/godog_test.go`
untouched. No `t.Skip` added.

## Evidence

### Pre-change red (unmodified route, before this task's edit)

```
$ ci/run.sh env DECK_GODOG_PATHS=status_claude_hooks.feature go test ./features/ -run TestFeatures -count=1 -v
```
Captured exit status: `1` — [`pre-change-red.log.exitstatus`](pre-change-red.log.exitstatus).
Full log: [`pre-change-red.log`](pre-change-red.log). Failing step, verbatim:

```
step error: session "hook truth" = status "starting" source "tmux" reason "tmux pane is alive; terminal row corrected" message "permission granted; work is complete" acknowledged=0 epoch=3; want "error" hook "tool_failure" "permission granted; work is complete" 0 2
--- FAIL: TestFeatures/Every_declared_Claude_hook_maps_to_honest_status_through_both_identity_routes (6.29s)
```

This is exactly the post-repair tuple this task's fix now asserts.

### Post-change: 3 consecutive green runs

```
$ ci/run.sh env DECK_GODOG_PATHS=status_claude_hooks.feature go test ./features/ -run TestFeatures -count=1
```

| Run | Exit status (file) | Scenarios | Steps |
|---|---|---|---|
| 1 | [`post-change-run1.log.exitstatus`](post-change-run1.log.exitstatus) = `0` | `3 passed` | `65 passed` |
| 2 | [`post-change-run2.log.exitstatus`](post-change-run2.log.exitstatus) = `0` | `3 passed` | `65 passed` |
| 3 | [`post-change-run3.log.exitstatus`](post-change-run3.log.exitstatus) = `0` | `3 passed` | `65 passed` |

Full unedited `-v` logs: [`post-change-run1.log`](post-change-run1.log),
[`post-change-run2.log`](post-change-run2.log),
[`post-change-run3.log`](post-change-run3.log) — each shows all three
scenarios in the file passing, including
`--- PASS: TestFeatures/Every_declared_Claude_hook_maps_to_honest_status_through_both_identity_routes`.

## Scope

```
$ git diff --stat
 features/status_claude_hooks.feature      | 19 ++++++++++++++++++-
 features/status_claude_hooks_test.go      | 39 +++++++++++++++++++++++++++++++
```

Only these two files, only inside the `StopFailure` block (plus its new
comment) and the one new Go step/function. `features/godog_test.go` is
untouched; no scenario removed (3 → 3); no `t.Skip` added; the file's later
`SessionEnd`/`SessionStart` block is byte-identical to before this commit.

## Still open (unchanged by this task)

- `sort_order.feature:35,63,91,119,149,223` and `status_recovery.feature:57`:
  same R76 live-pane race (F36), not yet re-pointed — latent, not yet red.
- `status_attach.feature:18`'s "attach acknowledges a live error" scenario:
  reported unsatisfiable as written (task 802) — its own next assertion
  needs the now-unreachable `!` marker, which this scenario's criteria did
  not require, so the two tasks resolve differently even though both trace
  to the same root cause.
