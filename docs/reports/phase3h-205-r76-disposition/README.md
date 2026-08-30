# Task 205 — R76 Phase 3h disposition correction, evidence

Steering 001 / operator amendment (see notes.md) rejected the R76 row's Phase 3h
disposition sentence in `docs/reports/phase3g.md` for false disposition prose: it claimed
the three forward-reverts "restore R94 scenario shapes unrelated to R76" and "touch neither
the repair nor this row's own scenarios". That is false against the diffs themselves —
`afa55b8` IS the revert that restores R76's own narrow live-pane repair in
`internal/service/reconcile.go`, `f700025` changes `features/status_probe.feature`'s
stale-sampling scenario, and `2f952da` changes `features/status_claude_hooks.feature`'s
live-pane `StopFailure` assertion. This report quotes each commit's stat and its per-path
diff so the corrected sentence in `docs/reports/phase3g.md` is grounded in the actual diff,
not expectation.

## `git show --stat afa55b8`
```
commit afa55b89e9ce0a0daac47df0cb14f9b44b46d5e8
Author: Nik <nikolaiorl@gmail.com>
Date:   Sun Aug 30 11:33:23 2026 +0000

    service: forward-revert 0b7dce5 to restore the narrow live-pane repair (task 002)
    
    Task 1101 (0b7dce5) re-widened repairTerminalRowWithLivePane's trigger to
    every terminal row (session.Status == "stopped" || session.Status ==
    "error"), citing SPEC.md:560-566 as drawing no exception by StatusSource or
    pane-exit verdict. de90a5c ruled directly against that reading: the
    transition table is the design, and a hook- or probe-sourced error row with
    no pane-exit verdict is the table's own live state (running --turn or API
    failure--> error with the pane alive throughout) and is never repaired --
    erasing it would make a live-pane StopFailure unobservable (finding F38).
    The repair covers only a stopped row, or an error row that itself carries a
    pane_exit_status or a tmux/user source.
    
    This forward-reverts 0b7dce5 (git revert --no-commit 0b7dce5, zero
    conflicts, 8 files) to restore that narrower trigger in reconcile.go,
    delete reconcile_bare_error_repair_test.go, and restore
    reconcile_live_error_precedence_test.go's two NeverRepairs tests pinning the
    exclusion. It also removes docs/reports/phase3g-1101-r76-terminal-error/,
    whose red/green logs supported the now-reverted widening; nothing tracked
    cites that path (git grep -l phase3g-1101 prints nothing).
    
    Targeted proof: ci/run.sh go test -count=1 ./internal/service/
    ./internal/store/ committed under
    docs/reports/phase3h-002-narrow-repair/.

 .../phase3g-1101-r76-terminal-error/README.md      |  65 -------------
 .../green-after.exitstatus                         |   1 -
 .../green-after.log                                |   2 -
 .../red-before.exitstatus                          |   1 -
 .../phase3g-1101-r76-terminal-error/red-before.log |   6 --
 internal/service/reconcile.go                      |  48 +++++-----
 .../service/reconcile_bare_error_repair_test.go    |  82 ----------------
 .../reconcile_live_error_precedence_test.go        | 103 +++++++++++++++++++++
 8 files changed, 129 insertions(+), 179 deletions(-)
```

## `git show afa55b8 -- internal/service/reconcile.go`
```diff
commit afa55b89e9ce0a0daac47df0cb14f9b44b46d5e8
Author: Nik <nikolaiorl@gmail.com>
Date:   Sun Aug 30 11:33:23 2026 +0000

    service: forward-revert 0b7dce5 to restore the narrow live-pane repair (task 002)
    
    Task 1101 (0b7dce5) re-widened repairTerminalRowWithLivePane's trigger to
    every terminal row (session.Status == "stopped" || session.Status ==
    "error"), citing SPEC.md:560-566 as drawing no exception by StatusSource or
    pane-exit verdict. de90a5c ruled directly against that reading: the
    transition table is the design, and a hook- or probe-sourced error row with
    no pane-exit verdict is the table's own live state (running --turn or API
    failure--> error with the pane alive throughout) and is never repaired --
    erasing it would make a live-pane StopFailure unobservable (finding F38).
    The repair covers only a stopped row, or an error row that itself carries a
    pane_exit_status or a tmux/user source.
    
    This forward-reverts 0b7dce5 (git revert --no-commit 0b7dce5, zero
    conflicts, 8 files) to restore that narrower trigger in reconcile.go,
    delete reconcile_bare_error_repair_test.go, and restore
    reconcile_live_error_precedence_test.go's two NeverRepairs tests pinning the
    exclusion. It also removes docs/reports/phase3g-1101-r76-terminal-error/,
    whose red/green logs supported the now-reverted widening; nothing tracked
    cites that path (git grep -l phase3g-1101 prints nothing).
    
    Targeted proof: ci/run.sh go test -count=1 ./internal/service/
    ./internal/store/ committed under
    docs/reports/phase3h-002-narrow-repair/.

diff --git a/internal/service/reconcile.go b/internal/service/reconcile.go
index 663a2f1..ba9183d 100644
--- a/internal/service/reconcile.go
+++ b/internal/service/reconcile.go
@@ -83,19 +83,22 @@ func (s Service) reconcile(ctx context.Context, staleAfter time.Duration) error
 		if present {
 			pane, crashed := crashedPane(observed)
 			if !crashed {
-				// A live, non-dead pane paired with a terminal row is SPEC
-				// §7's invariant violation regardless of whether terminal
-				// (below) also holds for this row: the pane is the part that
-				// is right. SPEC.md:560-566 is the precedence authority here
-				// and draws no distinction by source or by whether the row
-				// already carries a pane-exit verdict: "A terminal status --
-				// stopped, error -- claims there is nothing running here, and
-				// a live, non-dead pane is direct evidence against it. So when
-				// the reconcile observes one under a terminal row it corrects
-				// the row." A hook- or probe-written error row with no
-				// pane-exit verdict is not excepted: the row still owns :188's
-				// session-absent branch, unmodified.
-				if session.Status == "stopped" || session.Status == "error" {
+				// A live, non-dead pane paired with a stopped row, or with an
+				// error row that itself carries a pane-exit or tmux/user-sourced
+				// verdict, is SPEC §7's invariant violation regardless of
+				// whether terminal (below) also holds for this row: the pane is
+				// the part that is right, and the stored verdict is either
+				// spent (a crash the live pane now contradicts) or was never
+				// more than tmux's own liveness guess in the first place. A
+				// hook- or probe-sourced error with no pane-exit verdict is
+				// deliberately excluded: SPEC §7's transition table allows
+				// running --turn or API failure--> error with no pane death at
+				// all, so that row is the agent's own considered verdict, not a
+				// contradiction tmux liveness gets to overrule (finding F40,
+				// task 901); it still owns :188's session-absent branch,
+				// unmodified.
+				if session.Status == "stopped" ||
+					(session.Status == "error" && (session.PaneExitStatus != nil || session.StatusSource == "tmux" || session.StatusSource == "user")) {
 					if err := s.repairTerminalRowWithLivePane(ctx, session); err != nil {
 						return err
 					}
@@ -217,16 +220,17 @@ func (s Service) reconcile(ctx context.Context, staleAfter time.Duration) error
 	return nil
 }
 
-// repairTerminalRowWithLivePane is SPEC §7's one self-healing rule
-// (SPEC.md:560-566, "A terminal row with a live pane is an invariant
-// violation, and the same pass repairs it"): any terminal row -- stopped or
-// error, whatever wrote it -- paired with a live, non-dead pane is an
+// repairTerminalRowWithLivePane is SPEC §7's one self-healing rule: a
+// stopped row, or an error row that itself already carries a pane-exit or
+// tmux/user-sourced verdict, paired with a live, non-dead pane is an
 // invariant violation, not evidence to act on -- the pane is the part that
-// is right, and SPEC.md:560-566 is the one precedence authority for this
-// rule; it draws no exception by StatusSource or by whether the row already
-// carries a pane-exit verdict. The caller (reconcile.go's terminal-row
-// branch above) enforces exactly that trigger -- Status == "stopped" ||
-// Status == "error" -- before ever calling this function. It corrects the row from what liveness alone can observe: for a
+// is right. A hook- or probe-sourced error row with no pane-exit verdict is
+// deliberately excluded: SPEC §7's transition table allows running --turn or
+// API failure--> error with no pane death at all, so that row is the
+// agent's own considered verdict, not a contradiction tmux liveness gets to
+// overrule (finding F40, task 901); the caller (reconcile.go's terminal-row
+// branch above) is what enforces that narrower trigger before ever calling
+// this function. It corrects the row from what liveness alone can observe: for a
 // shell row that is exactly the §7 shell-liveness rule (a live pane always
 // means running, because a shell has no other signal, ever); for an agent
 // row liveness supplies no verdict at all, so the row is reset to the
```

## `git show --stat f700025`
```
commit f700025acb39f65a15496803bd0707bb715d6806
Author: Nik <nikolaiorl@gmail.com>
Date:   Sun Aug 30 11:03:40 2026 +0000

    features: forward-revert a53146a's stale-sampling re-pointing (task 001)
    
    de90a5c ruled §7's transition table is the design: a hook- or probe-sourced
    error row with no pane_exit_status is that table's own live state and is
    never repaired, even with the pane alive throughout. a53146a (task 1202)
    re-pointed status_probe.feature's "sampled pi" leg onto the widened
    repair's own state instead of treating that widening as the defect de90a5c
    found it to be. This reverts a53146a: the scenario's original probe-status
    assertions and the removal of the now-unused databaseSessionStatusSourceReason
    step helper.
    
    Expected RED until task 003 lands (narrows internal/service/reconcile.go's
    repairTerminalRowWithLivePane back down per de90a5c); see
    docs/reports/phase3h-001-status-probe-revert/README.md for the command run
    and its failing step.

 .../phase3g-1202-probe-stale-repointed/README.md   | 83 ----------------------
 .../phase3g-1202-probe-stale-repointed/run-1.log   |  1 -
 .../run-1.log.exitstatus                           |  1 -
 .../phase3g-1202-probe-stale-repointed/run-2.log   |  1 -
 .../run-2.log.exitstatus                           |  1 -
 .../phase3g-1202-probe-stale-repointed/run-3.log   |  1 -
 .../run-3.log.exitstatus                           |  1 -
 .../phase3g-1202-probe-stale-repointed/run-4.log   |  1 -
 .../run-4.log.exitstatus                           |  1 -
 .../phase3g-1202-probe-stale-repointed/run-5.log   |  1 -
 .../run-5.log.exitstatus                           |  1 -
 .../phase3h-001-status-probe-revert/README.md      | 49 +++++++++++++
 features/status_probe.feature                      |  5 +-
 features/status_probe_test.go                      | 13 ----
 14 files changed, 51 insertions(+), 109 deletions(-)
```

## `git show f700025 -- features/status_probe.feature`
```diff
commit f700025acb39f65a15496803bd0707bb715d6806
Author: Nik <nikolaiorl@gmail.com>
Date:   Sun Aug 30 11:03:40 2026 +0000

    features: forward-revert a53146a's stale-sampling re-pointing (task 001)
    
    de90a5c ruled §7's transition table is the design: a hook- or probe-sourced
    error row with no pane_exit_status is that table's own live state and is
    never repaired, even with the pane alive throughout. a53146a (task 1202)
    re-pointed status_probe.feature's "sampled pi" leg onto the widened
    repair's own state instead of treating that widening as the defect de90a5c
    found it to be. This reverts a53146a: the scenario's original probe-status
    assertions and the removal of the now-unused databaseSessionStatusSourceReason
    step helper.
    
    Expected RED until task 003 lands (narrows internal/service/reconcile.go's
    repairTerminalRowWithLivePane back down per de90a5c); see
    docs/reports/phase3h-001-status-probe-revert/README.md for the command run
    and its failing step.

diff --git a/features/status_probe.feature b/features/status_probe.feature
index 9e92d27..b28c547 100644
--- a/features/status_probe.feature
+++ b/features/status_probe.feature
@@ -54,9 +54,8 @@ Feature: Sampled probe status truth
     And within one configured reconcile interval deck client "A" row "raced claude" contains "live"
     And the state database session "stale claude" has probe status "running" with reason "working indicator"
     And within one configured reconcile interval deck client "A" row "stale claude" contains "sampled"
-    And the state database session "sampled pi" has status "starting" from "tmux" with reason "tmux pane is alive; terminal row corrected"
-    And the probe event count for session "sampled pi" is 1
-    And within one configured reconcile interval deck client "A" row "sampled pi" contains "starting"
+    And the state database session "sampled pi" has probe status "error" with reason "agent error"
+    And within one configured reconcile interval deck client "A" row "sampled pi" contains "sampled"
     And the state database session "probe shell" has status "running" from "tmux"
     And the probe event count for session "probe shell" is 0
     And deck client "A" row "probe shell" does not contain "sampled"
```

## `git show --stat 2f952da`
```
commit 2f952dac550b2d7625a5fba3cf48f366adea139d
Author: Nik <nikolaiorl@gmail.com>
Date:   Sun Aug 30 11:30:35 2026 +0000

    features: forward-revert c176751's StopFailure re-pointing (task 001)
    
    de90a5c resolved SPEC.md §7's self-contradiction and restored R93 wording; per that ruling this
    plan's R94 reverts land in full. c176751 (task 1102) had re-pointed
    features/status_claude_hooks.feature's StopFailure Then step onto the
    unconditional-repair state (starting/tmux, notify_epoch 3) once task 1101's
    R76 narrowing was reverted. Task 002 has not yet landed 1101's own inverse
    (0b7dce5) in this plan, so the repair the c176751 assertion depended on is
    not present at this commit — this feature is expected to run red until
    task 002 lands the repair back. This is the exact git revert of c176751:
    it restores the pre-1102 StopFailure Then step (hook status "error",
    reason "tool_failure", acknowledged 0, notify_epoch 2) and its comment
    block, and removes c176751's own evidence directory
    docs/reports/phase3g-1102-hook-truth-repointed/ (nothing tracked cites
    phase3g-1102: 'git grep -l phase3g-1102' prints nothing). No scenario is
    deleted, skipped or tagged out (3 Scenario: lines before and after).

 .../phase3g-1102-hook-truth-repointed/README.md    | 40 ----------------------
 .../phase3g-1102-hook-truth-repointed/run1.log     |  1 -
 .../run1.log.exitstatus                            |  1 -
 .../phase3g-1102-hook-truth-repointed/run2.log     |  1 -
 .../run2.log.exitstatus                            |  1 -
 .../phase3g-1102-hook-truth-repointed/run3.log     |  1 -
 .../run3.log.exitstatus                            |  1 -
 features/status_claude_hooks.feature               | 30 +++++++---------
 8 files changed, 12 insertions(+), 64 deletions(-)
```

## `git show 2f952da -- features/status_claude_hooks.feature`
```diff
commit 2f952dac550b2d7625a5fba3cf48f366adea139d
Author: Nik <nikolaiorl@gmail.com>
Date:   Sun Aug 30 11:30:35 2026 +0000

    features: forward-revert c176751's StopFailure re-pointing (task 001)
    
    de90a5c resolved SPEC.md §7's self-contradiction and restored R93 wording; per that ruling this
    plan's R94 reverts land in full. c176751 (task 1102) had re-pointed
    features/status_claude_hooks.feature's StopFailure Then step onto the
    unconditional-repair state (starting/tmux, notify_epoch 3) once task 1101's
    R76 narrowing was reverted. Task 002 has not yet landed 1101's own inverse
    (0b7dce5) in this plan, so the repair the c176751 assertion depended on is
    not present at this commit — this feature is expected to run red until
    task 002 lands the repair back. This is the exact git revert of c176751:
    it restores the pre-1102 StopFailure Then step (hook status "error",
    reason "tool_failure", acknowledged 0, notify_epoch 2) and its comment
    block, and removes c176751's own evidence directory
    docs/reports/phase3g-1102-hook-truth-repointed/ (nothing tracked cites
    phase3g-1102: 'git grep -l phase3g-1102' prints nothing). No scenario is
    deleted, skipped or tagged out (3 Scenario: lines before and after).

diff --git a/features/status_claude_hooks.feature b/features/status_claude_hooks.feature
index 346ca6c..756764d 100644
--- a/features/status_claude_hooks.feature
+++ b/features/status_claude_hooks.feature
@@ -45,26 +45,20 @@ Feature: Claude hook status truth
     When deck client "A" attaches to and detaches from its selected agent
     Then the state database session "hook truth" is "running" from "user" with acknowledged=1, notify_epoch=2, and 2 attached events
 
-    # StopFailure's own hook write lands its 'error' status durably as far
-    # as the hook write itself is concerned, but this pane stays alive for
-    # the UserPromptSubmit retry that follows, so unlike the driven-to-death
-    # panes above there is no window in which 'error' is observable here: the
-    # released deck _hook subcommand's post-hook liveness pass
-    # (cmd/deck/main.go's runHook -> ReconcileWithin) runs synchronously, in
-    # the same subprocess invocation, immediately after the hook's own write
-    # and before that subprocess ever returns control to the fake Claude
-    # pane's send-keys. SPEC section 7's self-heal
-    # (internal/service.reconcile's repairTerminalRowWithLivePane, R76) is
-    # unconditional and finds this 'error' row paired with the still-live,
-    # non-crashed 'hook truth' pane -- an invariant violation it repairs on
-    # that very same pass: the row lands on the neutral 'starting' a fresh
-    # pane always begins at, tmux-sourced, with the corrected-row reason, one
-    # notify_epoch tick (leaving the 'error' attention status spends one),
-    # and every other field untouched. The hook's own event is still audited
-    # in full below.
+    # Task 901's finding (F40, docs/reports/phase3g-findings.md) narrowed
+    # SPEC section 7's live-pane self-heal (internal/service.reconcile's
+    # repairTerminalRowWithLivePane) so it no longer reaches a bare
+    # hook/probe 'error' row: it repairs 'stopped' unconditionally, and
+    # 'error' only when the row carries a pane-exit verdict or a
+    # tmux-/user-sourced verdict. This StopFailure verdict is hook-sourced
+    # and carries no pane-exit status, so it sits at the top of SPEC's
+    # precedence order ("user-terminal > hook > probe > tmux", SPEC.md:509,
+    # 512) and the repair no longer touches it -- task 902 implemented the
+    # narrowing. The hook's own 'error'/'tool_failure' write is therefore
+    # the durable, observable verdict here, exactly as the hook wrote it.
     When fake Claude session "hook truth" fires "StopFailure" for itself using injected identity:
       | error_type | tool_failure |
-    Then the state database session "hook truth" is repaired to "starting" from "tmux" with reason "tmux pane is alive; terminal row corrected", message "permission granted; work is complete", acknowledged 0, and notify_epoch 3
+    Then the state database session "hook truth" has hook status "error", reason "tool_failure", message "permission granted; work is complete", acknowledged 0, and notify_epoch 2
     And session "hook truth" has one "stop_failure" event with payload field "error_type" equal to "tool_failure"
 
     When fake Claude session "hook truth" fires "UserPromptSubmit" for itself using injected identity:
```

## Mechanical checks

`grep -n 'unrelated to R76' docs/reports/phase3g.md` (expect nothing):
```
(no output)
```

`grep -n 'touch neither the repair' docs/reports/phase3g.md` (expect nothing):
```
(no output)
```

`awk -F'\\| Phase 3h disposition' '/^\| R76 /{print $2}' docs/reports/phase3g.md | grep -c -e unrelated -e 'touch neither' -e 'does not touch' -e untouched` (expect `0`):
```
0
```

`grep -c 4b1d4dc docs/reports/phase3g.md` (final code sha citation, expect >= 1):
```
1
```

`git diff --numstat 08171ce^ HEAD -- docs/reports/phase3g.md` (expect exactly `1	1`):
```
1	1	docs/reports/phase3g.md
```
