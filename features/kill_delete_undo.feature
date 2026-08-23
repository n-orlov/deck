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

  @requirement-23-delete-undo
  Scenario: u restores a deleted row inside its DECK_DELETE_GRACE_MS window
    Given deck client "A" is started with a short delete grace window
    And deck client "A" creates shell session "dd-undo-restores"
    When deck client "A" presses dd
    And deck client "A" submits the open dialog
    Then deck client "A" screen contains "press u to undo"
    And the state database session "dd-undo-restores" is tombstoned
    When deck client "A" presses u
    Then deck client "A" screen contains "dd-undo-restores"
    And the state database session "dd-undo-restores" is not tombstoned
    When deck client "A" exits cleanly

  @requirement-23-delete-reap
  Scenario: u does nothing once the delete grace window has expired, and the row is reaped
    Given deck client "A" is started with a short delete grace window
    And deck client "A" creates shell session "dd-reap-expires"
    When deck client "A" presses dd
    And deck client "A" submits the open dialog
    And 400 milliseconds pass
    Then deck client "A" screen does not contain "press u to undo"
    And the state database session "dd-reap-expires" is reaped
    When deck client "A" presses u
    Then the state database session "dd-reap-expires" is reaped
    When deck client "A" exits cleanly

  @requirement-24-reap-leaves-no-trace
  Scenario: reaping a deleted session removes its store rows and deck's own per-session files, but never the audit log's earlier history
    Given deck client "A" is started with a short delete grace window
    And deck client "A" creates shell session "dd-reap-no-trace"
    And deck client "A" seeds captures and a history file for session "dd-reap-no-trace"
    When deck client "A" presses dd
    And deck client "A" submits the open dialog
    And 400 milliseconds pass
    Then the state database session "dd-reap-no-trace" is reaped
    And the captures directory and history file for reaped session "dd-reap-no-trace" are gone
    And the audit log still contains an earlier event for reaped session "dd-reap-no-trace"
    When deck client "A" exits cleanly

  @requirement-29-kill
  Scenario: killing a session leaves its adversarially-seeded cwd fingerprint unchanged
    Given deck client "A" is started
    And a scratch directory "fp-kill" is seeded with:
      | path         | kind | content             | mode |
      | state.db     | file | fake-database-bytes |      |
      | .hidden      | file | dotfile-content      |      |
      | subdir       | dir  |                      |      |
      | readonly.txt | file | cannot-write-me      | 0444 |
    And deck client "A" creates shell session "fp-kill-session" with cwd the scratch directory labelled "fp-kill"
    And the directory "fp-kill" is fingerprinted as "before-kill"
    When deck client "A" kills its selected session
    Then the directory "fp-kill" still matches fingerprint "before-kill"
    When deck client "A" exits cleanly

  @requirement-29-kill-undo
  Scenario: undoing a kill leaves its adversarially-seeded cwd fingerprint unchanged
    Given deck client "A" is started
    And a scratch directory "fp-kill-undo" is seeded with:
      | path         | kind | content             | mode |
      | state.db     | file | fake-database-bytes |      |
      | .hidden      | file | dotfile-content      |      |
      | subdir       | dir  |                      |      |
      | readonly.txt | file | cannot-write-me      | 0444 |
    And deck client "A" creates shell session "fp-kill-undo-session" with cwd the scratch directory labelled "fp-kill-undo"
    And the directory "fp-kill-undo" is fingerprinted as "before-kill-undo"
    When deck client "A" kills its selected session
    And deck client "A" presses u
    Then deck client "A" screen contains "running"
    And the directory "fp-kill-undo" still matches fingerprint "before-kill-undo"
    When deck client "A" exits cleanly

  @requirement-29-delete
  Scenario: dd (delete) leaves its adversarially-seeded cwd fingerprint unchanged
    Given deck client "A" is started
    And a scratch directory "fp-delete" is seeded with:
      | path         | kind | content             | mode |
      | state.db     | file | fake-database-bytes |      |
      | .hidden      | file | dotfile-content      |      |
      | subdir       | dir  |                      |      |
      | readonly.txt | file | cannot-write-me      | 0444 |
    And deck client "A" creates shell session "fp-delete-session" with cwd the scratch directory labelled "fp-delete"
    And the directory "fp-delete" is fingerprinted as "before-delete"
    When deck client "A" presses dd
    And deck client "A" submits the open dialog
    Then the state database session "fp-delete-session" is tombstoned
    And the directory "fp-delete" still matches fingerprint "before-delete"
    When deck client "A" exits cleanly

  @requirement-29-delete-undo
  Scenario: undoing a delete leaves its adversarially-seeded cwd fingerprint unchanged
    Given deck client "A" is started with a short delete grace window
    And a scratch directory "fp-delete-undo" is seeded with:
      | path         | kind | content             | mode |
      | state.db     | file | fake-database-bytes |      |
      | .hidden      | file | dotfile-content      |      |
      | subdir       | dir  |                      |      |
      | readonly.txt | file | cannot-write-me      | 0444 |
    And deck client "A" creates shell session "fp-delete-undo-session" with cwd the scratch directory labelled "fp-delete-undo"
    And the directory "fp-delete-undo" is fingerprinted as "before-delete-undo"
    When deck client "A" presses dd
    And deck client "A" submits the open dialog
    And deck client "A" presses u
    Then the state database session "fp-delete-undo-session" is not tombstoned
    And the directory "fp-delete-undo" still matches fingerprint "before-delete-undo"
    When deck client "A" exits cleanly

  @requirement-29-reap
  Scenario: reaping a deleted session leaves its adversarially-seeded cwd fingerprint unchanged
    Given deck client "A" is started with a short delete grace window
    And a scratch directory "fp-reap" is seeded with:
      | path         | kind | content             | mode |
      | state.db     | file | fake-database-bytes |      |
      | .hidden      | file | dotfile-content      |      |
      | subdir       | dir  |                      |      |
      | readonly.txt | file | cannot-write-me      | 0444 |
    And deck client "A" creates shell session "fp-reap-session" with cwd the scratch directory labelled "fp-reap"
    And the directory "fp-reap" is fingerprinted as "before-reap"
    When deck client "A" presses dd
    And deck client "A" submits the open dialog
    And 400 milliseconds pass
    Then the state database session "fp-reap-session" is reaped
    And the directory "fp-reap" still matches fingerprint "before-reap"
    When deck client "A" exits cleanly

  @requirement-2-monotonic-windows
  Scenario: both the undo window and the delete grace window keep advancing while DECK_CLOCK is frozen
    Given deck client "A" is started with the clock frozen at "2025-01-02T03:04:05Z" and short undo and delete windows
    And deck client "A" creates shell session "frozen-monotonic"
    When deck client "A" kills its selected session
    Then deck client "A" screen contains "press u to undo"
    When deck client "A" presses dd
    And deck client "A" submits the open dialog
    Then the state database session "frozen-monotonic" is tombstoned
    When 400 milliseconds pass
    Then deck client "A" screen does not contain "press u to undo"
    And the state database session "frozen-monotonic" is reaped
    When deck client "A" presses u
    Then the state database session "frozen-monotonic" is reaped
    When deck client "A" exits cleanly

  @requirement-25-delete-without-purge
  Scenario: dd without purge leaves the agent's transcript intact through delete and reap
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started with a short delete grace window
    When deck client "A" creates claude session "purge-keep" with permission profile "safe" and message "hello from purge-keep"
    Then deck client "A" screen contains "resumable"
    And the fake claude transcript for session "purge-keep" is captured as "keep-before"
    When deck client "A" presses dd
    Then deck client "A" screen contains "Purge:"
    And deck client "A" screen contains "keep"
    When deck client "A" submits the open dialog
    And 400 milliseconds pass
    Then the state database session "purge-keep" is reaped
    And the transcript captured as "keep-before" still exists byte-identical
    When deck client "A" exits cleanly

  @requirement-26-purge-conversation
  Scenario: choosing purge in the delete confirm removes only the agent's declared transcript file
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "purge-remove" with permission profile "safe" and message "hello from purge-remove"
    Then deck client "A" screen contains "resumable"
    And the fake claude transcript for session "purge-remove" is captured as "purge-before"
    When deck client "A" presses dd
    And deck client "A" cycles the open dialog's field right
    Then deck client "A" screen contains "Will delete:"
    When deck client "A" submits the open dialog
    Then the state database session "purge-remove" is tombstoned
    And the transcript captured as "purge-before" no longer exists
    When deck client "A" exits cleanly

  @requirement-27-archive-stopped
  Scenario: A archives an already-stopped session, hidden from the default list with its status unchanged
    Given deck client "A" is started with a short undo window
    And deck client "A" creates shell session "archive-stopped"
    When deck client "A" kills its selected session
    Then the state database contains session "archive-stopped" with status "stopped"
    When 300 milliseconds pass
    And deck client "A" archives its selected session "archive-stopped"
    Then the state database session "archive-stopped" is archived
    And the state database contains session "archive-stopped" with status "stopped"
    When deck client "A" exits cleanly

  @requirement-27-archive-kill-and-archive
  Scenario: A on a non-stopped session offers kill and archive as one action instead of refusing
    Given deck client "A" is started
    And deck client "A" creates shell session "archive-running"
    When deck client "A" archives its selected session "archive-running"
    Then the private tmux session "deck_archive-running" does not exist
    And the state database session "archive-running" is archived
    And the state database contains session "archive-running" with status "stopped"
    When deck client "A" exits cleanly
