Feature: Phase 0 walking skeleton
  A user can manage a shell session solely through the released deck TUI.

  Scenario: create, attach, kill, and retain the durable shell session
    Given deck client "A" is started
    Then deck client "A" screen contains "No sessions yet"
    When deck client "A" creates shell session "walking session"
    Then deck client "A" screen contains "walking session"
    And the private tmux session "deck_walking-session" exists
    And the private tmux session "deck_walking-session" has one pane in the created working directory
    And the private tmux session "deck_walking-session" has exactly one pane running "sh"
    When deck client "A" attaches to and detaches from its session
    And deck client "A" kills its selected session
    Then the private tmux session "deck_walking-session" does not exist
    And the state database contains session "walking session" with status "stopped"
    And deck client "A" screen contains "resumable"
    And the audit log contains event "killed" for a session
    And the created working-directory sentinel is unchanged
    When deck client "A" exits cleanly
