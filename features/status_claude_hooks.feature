@status-claude-hooks
Feature: Claude hook status truth
  Claude events are fired by the fake agent inside its real pane through the
  per-session instrumentation supplied by the released deck binary.

  Scenario: Every declared Claude hook maps to honest status through both identity routes
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "hook truth" with permission profile "safe"
    Then session "hook truth"'s pane has the scenario hook environment
    And the scenario working directory contains no deck state

    When fake Claude session "hook truth" fires "SessionStart" for itself using injected identity:
      | source | fresh |
    Then the state database session "hook truth" has hook status "running", reason "fresh", message "", acknowledged 1, and notify_epoch 0
    And session "hook truth" has one "session_start" event with payload field "source" equal to "fresh"

    When fake Claude session "hook truth" fires "Notification" for itself using injected identity:
      | notification_type | permission_prompt |
    Then within one configured reconcile interval deck client "A" screen contains "waiting"
    And the state database session "hook truth" has hook status "waiting", reason "permission_prompt", message "", acknowledged 0, and notify_epoch 0
    And session "hook truth" has one "notification" event with payload field "notification_type" equal to "permission_prompt"

    When deck client "A" attaches to and detaches from its selected agent
    Then the state database session "hook truth" is "running" from "user" with acknowledged=1, notify_epoch=1, and 1 attached event
    When fake Claude session "hook truth" fires "Stop" for itself using conversation identity:
      | last_assistant_message | permission granted; work is complete |
    Then within one configured reconcile interval deck client "A" screen contains "idle"
    And the state database session "hook truth" has hook status "idle", reason "", message "permission granted; work is complete", acknowledged 1, and notify_epoch 1
    And session "hook truth" has one "stop" event with payload field "last_assistant_message" equal to "permission granted; work is complete"
    When deck client "A" opens detail for session "hook truth"
    Then deck client "A" screen contains "permission granted; work is complete"
    When deck client "A" closes the session detail

    When fake Claude session "hook truth" fires "UserPromptSubmit" for itself using conversation identity:
      | prompt | status please |
    Then the state database session "hook truth" has hook status "running", reason "", message "permission granted; work is complete", acknowledged 1, and notify_epoch 1
    And session "hook truth" has one "user_prompt_submitted" event with payload field "prompt" equal to "status please"

    When fake Claude session "hook truth" fires "Notification" for itself using injected identity:
      | notification_type | permission_prompt |
    Then within one configured reconcile interval deck client "A" screen contains "waiting"
    And the state database session "hook truth" has hook status "waiting", reason "permission_prompt", message "permission granted; work is complete", acknowledged 0, and notify_epoch 1

    When deck client "A" attaches to and detaches from its selected agent
    Then the state database session "hook truth" is "running" from "user" with acknowledged=1, notify_epoch=2, and 2 attached events

    # StopFailure's own hook write lands its 'error' status durably as far
    # as the hook write itself is concerned, but this pane stays alive for
    # the UserPromptSubmit retry that follows, so unlike the driven-to-death
    # panes above there is no window in which 'error' is observable here: the
    # released deck _hook subcommand's post-hook liveness pass
    # (cmd/deck/main.go's runHook -> ReconcileWithin) runs synchronously, in
    # the same subprocess invocation, immediately after the hook's own write
    # and before that subprocess ever returns control to the fake Claude
    # pane's send-keys. SPEC section 7's self-heal
    # (internal/service.reconcile's repairTerminalRowWithLivePane, R76) is
    # unconditional and finds this 'error' row paired with the still-live,
    # non-crashed 'hook truth' pane -- an invariant violation it repairs on
    # that very same pass: the row lands on the neutral 'starting' a fresh
    # pane always begins at, tmux-sourced, with the corrected-row reason, one
    # notify_epoch tick (leaving the 'error' attention status spends one),
    # and every other field untouched. The hook's own event is still audited
    # in full below.
    When fake Claude session "hook truth" fires "StopFailure" for itself using injected identity:
      | error_type | tool_failure |
    Then the state database session "hook truth" is repaired to "starting" from "tmux" with reason "tmux pane is alive; terminal row corrected", message "permission granted; work is complete", acknowledged 0, and notify_epoch 3
    And session "hook truth" has one "stop_failure" event with payload field "error_type" equal to "tool_failure"

    When fake Claude session "hook truth" fires "UserPromptSubmit" for itself using injected identity:
      | prompt | retry after failure |
    Then the state database session "hook truth" has hook status "running", reason "", message "permission granted; work is complete", acknowledged 0, and notify_epoch 3
    And session "hook truth" has an audited "user_prompt_submitted" event with payload field "prompt" equal to "retry after failure"

    # SessionEnd cannot be posed by firing it through the fake claude pane
    # while that pane is still alive: SPEC section 7's self-heal
    # (internal/service.reconcile's repairTerminalRowWithLivePane) treats the
    # hook's own "stopped" write, paired with a live, non-dead pane, as an
    # invariant violation and repairs it back to starting/tmux on the very
    # next reconcile tick -- the same self-heal task 040 hit for shell rows
    # posed by a raw state-database write. A real Claude process's own
    # SessionEnd genuinely coincides with that process ending, so this drives
    # the pane to a confirmed clean exit first (deck's remain-on-exit=failed
    # then removes the whole "deck_hook-truth" tmux session), and delivers
    # SessionEnd the way a real hook subprocess always does: as a one-shot
    # invocation of the released deck _hook, independent of the interactive
    # pane's own lifetime rather than a send-keys into it. The row's terminal
    # status is therefore read with no live pane underneath it to repair.
    When fake Claude session "hook truth" exits its pane cleanly
    And the released deck _hook receives "SessionEnd" for session "hook truth" using conversation identity:
      | reason | logout |
    Then the state database session "hook truth" has hook status "stopped", reason "logout", message "permission granted; work is complete", acknowledged 0, and notify_epoch 3
    And session "hook truth" has one "session_end" event with payload field "reason" equal to "logout"

    When the released deck _hook receives "SessionStart" for session "hook truth" using injected identity:
      | source | late-after-clean-stop |
    Then the state database session "hook truth" has hook status "stopped", reason "logout", message "permission granted; work is complete", acknowledged 0, and notify_epoch 3
    And session "hook truth" has an audited "session_start" event with payload field "source" equal to "late-after-clean-stop"
    And deck client "A" exits cleanly

  Scenario: A pane-fired hook cannot override a user-terminal verdict
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "terminal victim" with permission profile "safe"
    And deck client "A" creates claude session "hook emitter" with permission profile "safe"
    And deck client "A" kills session "terminal victim"
    Then the state database session "terminal victim" is "stopped" from "user" with killed_by_user=1
    When fake Claude session "hook emitter" fires "SessionStart" for session "terminal victim" using conversation identity:
      | source | resume |
    Then the state database session "terminal victim" is "stopped" from "user" with killed_by_user=1
    And session "terminal victim" has one "session_start" event with payload field "source" equal to "resume"
    When deck client "A" exits cleanly

  Scenario: A pane-fired hook cannot revive a process-crash terminal row
    Given a crash-tail fixture and long-running fake Claude are configured
    And deck client "A" is started
    When deck client "A" creates claude session "crash victim" with permission profile "safe"
    And deck client "A" creates claude session "crash emitter" with permission profile "safe"
    And fake Claude session "crash victim" renders the colored crash-tail fixture
    And the agent process "fake-claude-real" in private tmux session "deck_crash-victim" is killed with SIGKILL
    Then within one configured reconcile interval deck client "A" screen contains "error"
    And the state database session "crash victim" has a sanitized last-200-line crash artifact
    When fake Claude session "crash emitter" fires "SessionStart" for session "crash victim" using conversation identity:
      | source | late-after-process-crash |
    Then the state database session "crash victim" has a sanitized last-200-line crash artifact
    And session "crash victim" has one "session_start" event with payload field "source" equal to "late-after-process-crash"
    When deck client "A" exits cleanly
