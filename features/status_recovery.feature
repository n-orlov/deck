@status-recovery
Feature: Status recovery chain (requirements 43-47)
  The magpie-row evidence behind requirements 43-47 is one chain, not four
  independent fixes: an in-session resume must not read as the process
  ending; the stored conversation_id must follow wherever Claude's own hooks
  say the live conversation now is; a stale lower-precedence verdict must not
  block a later hook from landing; `r` on a pane deck already owns must never
  masquerade as a launch failure; and none of that may let a hook resurrect a
  row the user explicitly killed. Each scenario below is one link.

  Scenario: An in-session resume pair leaves the row running and moves conversation_id
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "resume pair" with permission profile "safe"
    Then the state database session "resume pair" has a non-empty conversation id
    When fake Claude session "resume pair" resumes in-session into a new conversation
    Then the state database session "resume pair" now has a different, non-empty conversation id than before the resume
    And the state database session "resume pair" is "running" from "hook" with killed_by_user=0
    And session "resume pair" has one "session_end" event with payload field "reason" equal to "resume"
    And session "resume pair" has an audited "session_start" event with payload field "reason" equal to "resume"
    When deck client "A" exits cleanly

  Scenario: r on a session whose tmux session already exists reports already-running, never an error
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "dup pane" with permission profile "safe"
    Then the audit log has 1 launch record for session "dup pane"
    When fake Claude session "dup pane" fires "SessionEnd" for itself using injected identity:
      | reason | logout |
    Then the state database session "dup pane" is "stopped" from "hook" with killed_by_user=0
    And the private tmux session "deck_dup-pane" exists
    And within one configured reconcile interval deck client "A" screen contains "stopped - resumable"
    When deck client "A" presses r on session "dup pane"
    Then deck client "A" screen contains "already running"
    And the state database session "dup pane" is "stopped" from "hook" with killed_by_user=0
    And the audit log has 1 launch record for session "dup pane"
    When deck client "A" exits cleanly

  Scenario: A row at error from a stale tmux launch-failure verdict recovers on the next hook
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "recoverable" with permission profile "safe"
    And the state database session "recoverable"'s status is forced to "error" from tmux as a stale launch-failure verdict
    Then the state database session "recoverable" is "error" from "tmux" with killed_by_user=0
    When fake Claude session "recoverable" fires "Stop" for itself using conversation identity:
      | last_assistant_message | recovered after stale tmux verdict |
    Then within one configured reconcile interval deck client "A" screen contains "idle"
    And the state database session "recoverable" is "idle" from "hook" with killed_by_user=0
    And session "recoverable" has one "stop" event with payload field "last_assistant_message" equal to "recovered after stale tmux verdict"
    When deck client "A" exits cleanly

  Scenario: A session the user killed is not resurrected by a late hook
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "terminal row" with permission profile "safe"
    And deck client "A" creates claude session "hook emitter two" with permission profile "safe"
    And deck client "A" kills session "terminal row"
    Then the state database session "terminal row" is "stopped" from "user" with killed_by_user=1
    When fake Claude session "hook emitter two" fires "SessionStart" for session "terminal row" using conversation identity:
      | source | resume |
    Then the state database session "terminal row" is "stopped" from "user" with killed_by_user=1
    And session "terminal row" has one "session_start" event with payload field "source" equal to "resume"
    When deck client "A" exits cleanly
