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

    When fake Claude session "hook truth" fires "StopFailure" for itself using injected identity:
      | error_type | tool_failure |
    Then the state database session "hook truth" has hook status "error", reason "tool_failure", message "permission granted; work is complete", acknowledged 0, and notify_epoch 2
    And session "hook truth" has one "stop_failure" event with payload field "error_type" equal to "tool_failure"

    When fake Claude session "hook truth" fires "UserPromptSubmit" for itself using injected identity:
      | prompt | retry after failure |
    Then the state database session "hook truth" has hook status "running", reason "", message "permission granted; work is complete", acknowledged 0, and notify_epoch 3
    And session "hook truth" has an audited "user_prompt_submitted" event with payload field "prompt" equal to "retry after failure"

    When fake Claude session "hook truth" fires "SessionEnd" for itself using conversation identity:
      | reason | logout |
    Then the state database session "hook truth" has hook status "stopped", reason "logout", message "permission granted; work is complete", acknowledged 0, and notify_epoch 3
    And session "hook truth" has one "session_end" event with payload field "reason" equal to "logout"

    When fake Claude session "hook truth" fires "SessionStart" for itself using injected identity:
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
