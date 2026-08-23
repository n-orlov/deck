@requirement-22-undo-toast
Feature: Undo toast after x, and the dd delete/tombstone chord
  x still kills with no confirmation and still refuses an already-stopped
  row, but a successful kill leaves a toast naming undo, actionable for
  exactly DECK_UNDO_MS, that u resumes -- once the window is gone (expired,
  or already spent by an earlier u), u does nothing. Task 106 adds dd's
  own grace-window/reap scenarios to this same file.

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

  Scenario: a single d shows a pending-delete indicator and changes nothing in the store
    Given deck client "A" is started
    And deck client "A" creates shell session "dd-pending"
    When deck client "A" presses d
    Then deck client "A" screen contains "press d again to confirm"
    And the state database session "dd-pending" is not tombstoned
    And the private tmux session "deck_dd-pending" exists
    When deck client "A" clears the pending delete indicator with escape
    Then deck client "A" screen does not contain "press d again to confirm"
    And the state database session "dd-pending" is not tombstoned
    When deck client "A" exits cleanly

  Scenario: d followed by any other key performs no destructive action
    Given deck client "A" is started
    And deck client "A" creates shell session "dd-interrupted"
    When deck client "A" presses d
    And deck client "A" clears the pending delete indicator by pressing "j"
    Then deck client "A" screen does not contain "press d again to confirm"
    And the state database session "dd-interrupted" is not tombstoned
    And the private tmux session "deck_dd-interrupted" exists
    When deck client "A" exits cleanly

  Scenario: the second d opens a confirm dialog naming what survives, esc cancels leaving the row untombstoned
    Given deck client "A" is started
    And deck client "A" creates shell session "dd-confirm-cancel"
    When deck client "A" presses dd
    Then deck client "A" screen contains "Conversation:"
    And deck client "A" screen contains "Working directory:"
    When deck client "A" closes the dialog with escape
    Then the state database session "dd-confirm-cancel" is not tombstoned
    And the private tmux session "deck_dd-confirm-cancel" exists
    When deck client "A" exits cleanly

  Scenario: submitting the confirm dialog kills the live pane, tombstones the row, and it disappears from the sidebar immediately
    Given deck client "A" is started
    And deck client "A" creates shell session "dd-submit"
    When deck client "A" presses dd
    And deck client "A" submits the open dialog
    Then deck client "A" screen does not contain "dd-submit"
    And the private tmux session "deck_dd-submit" does not exist
    And the state database session "dd-submit" is tombstoned
    When deck client "A" exits cleanly

  Scenario: the dd confirm dialog obeys the §11.4 contract -- the mouse can neither cancel nor confirm it, at its border, its body or outside it
    Given deck client "A" is started
    And deck client "A" creates shell session "dd-mouse"
    When deck client "A" presses dd
    Then deck client "A" screen contains "Conversation:"
    When deck client "A" captures its frame as "before-dd-mouse"
    And deck client "A" clicks at column 1 row 1
    And deck client "A" clicks at column 40 row 5
    And deck client "A" clicks at column 95 row 25
    Then deck client "A" frame still matches the captured "before-dd-mouse" frame
    And the state database session "dd-mouse" is not tombstoned
    When deck client "A" closes the dialog with escape
    And deck client "A" exits cleanly

  Scenario: the dd confirm dialog width is 80% of the viewport clamped to [26,80], at both clamp ends
    Given deck client "A" is started with terminal size 30x60
    And deck client "A" creates shell session "dd-width-lower"
    When deck client "A" presses dd
    Then deck client "A" dialog box width is 26
    When deck client "A" closes the dialog with escape
    And deck client "A" exits cleanly

  Scenario: the dd confirm dialog width saturates at 80 well past the upper clamp
    Given deck client "A" is started with terminal size 220x30
    And deck client "A" creates shell session "dd-width-upper"
    When deck client "A" presses dd
    Then deck client "A" dialog box width is 80
    When deck client "A" closes the dialog with escape
    And deck client "A" exits cleanly
