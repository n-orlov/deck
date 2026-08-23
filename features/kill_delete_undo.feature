@requirement-22-undo-toast
Feature: Undo toast after x
  x still kills with no confirmation and still refuses an already-stopped
  row, but a successful kill leaves a toast naming undo, actionable for
  exactly DECK_UNDO_MS, that u resumes -- once the window is gone (expired,
  or already spent by an earlier u), u does nothing. Tasks 105/106 add dd's
  own tombstone/grace scenarios to this same file.

  Scenario: x kills without confirmation and refuses to kill an already-stopped row
    Given deck client "A" is started
    And deck client "A" creates shell session "undo-basics"
    Then the private tmux session "deck_undo-basics" exists
    When deck client "A" kills its selected session
    Then the private tmux session "deck_undo-basics" does not exist
    And the state database session "undo-basics" is "stopped" from "user" with killed_by_user=1
    When deck client "A" kills its selected session
    Then deck client "A" screen contains "already stopped"
    When deck client "A" exits cleanly

  Scenario: u undoes the most recent kill inside its DECK_UNDO_MS window
    Given deck client "A" is started
    And deck client "A" creates shell session "undo-resumes"
    When deck client "A" kills its selected session
    Then deck client "A" screen contains "press u to undo"
    When deck client "A" presses u
    Then deck client "A" screen contains "running"
    And the private tmux session "deck_undo-resumes" exists
    And the state database contains session "undo-resumes" with status "running"
    When deck client "A" exits cleanly

  Scenario: u does nothing once the undo window has expired
    Given deck client "A" is started with a short undo window
    And deck client "A" creates shell session "undo-expires"
    When deck client "A" kills its selected session
    And 400 milliseconds pass
    Then deck client "A" screen does not contain "press u to undo"
    When deck client "A" presses u
    Then the private tmux session "deck_undo-expires" does not exist
    And the state database session "undo-expires" is "stopped" from "user" with killed_by_user=1
    When deck client "A" exits cleanly
