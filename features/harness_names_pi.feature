@harness-names
Feature: Pi session names travel between deck and the harness
  Pi keeps a session's name as session_info entries in its session file, tells
  the deck extension about a change with session_info_changed, and takes deck's
  rename through the extension's control file once session_start opened it, or
  as typed /name keystrokes when that file is gone, never into a focused dialog. Pi
  never titles a session itself, so every Pi name is explicit and there is
  nothing to auto-adopt. The fake Pi persists those same records, so each
  scenario asserts through them and through deck's store and screen, never
  through a fixed sleep.

  Scenario: an explicit name typed in deck reaches Pi at launch
    Given a long-running fake "pi" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates pi session "billing audit" with permission profile "safe"
    Then the audit log's most recent launch argv for session "billing audit" contains "--name"
    And the audit log's most recent launch argv for session "billing audit" contains "billing audit"
    And the fake Pi session file of session "billing audit" has the name "billing audit"
    And the state database session "billing audit" has name source "user"
    When fake Pi session "billing audit" starts its session
    And fake Pi session "billing audit" is prompted and finishes its turn
    Then the state database session "billing audit" has hook status "idle" with reason ""
    And the fake Pi session file of session "billing audit" has 1 session_info record
    And the fake Pi session file of session "billing audit" has the name "billing audit"
    And the state database session "billing audit" has name source "user"
    And the state database session "billing audit" has no pending harness name
    And session "billing audit" has no "renamed" event
    And deck client "A" screen contains "billing audit"
    When deck client "A" exits cleanly

  Scenario: a rename in deck is pushed into Pi and does not loop back
    Given a long-running fake "pi" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates pi session "push me" with permission profile "safe"
    And fake Pi session "push me" starts its session
    Then the fake Pi control file of session "push me" is open
    When deck client "A" renames session "push me" to "pushed title" from its detail dialog
    Then the fake Pi session file of session "pushed title" has the name "pushed title"
    And the fake Pi pane of session "pushed title" shows "fake-pi name: pushed title"
    And the state database session "pushed title" has no pending harness name
    And deck client "A" screen contains "pushed title"
    When fake Pi session "pushed title" is prompted and finishes its turn
    Then the state database session "pushed title" has name source "user"
    And the fake Pi session file of session "pushed title" has the name "pushed title"
    And the state database session "pushed title" has an event of kind "renamed" with reason containing "user"
    And the state database session "pushed title" has no "renamed" event with reason containing "harness:"
    And session "pushed title" has no "harness_rename_failed" event
    And session "pushed title" has no "harness_rename_refused" event
    When deck client "A" exits cleanly

  Scenario: a rename typed inside Pi is adopted and freezes the name, and Pi never titles the session itself
    Given a long-running fake "pi" binary is on PATH for future deck clients
    And deck client "A" is started with the fake agents in a fresh directory labelled "pw" with the clock frozen at "2025-08-20T14:43:00Z"
    When deck client "A" creates pi session with a blank name expecting the derived name "pw-0820-1443"
    And fake Pi session "pw-0820-1443" starts its session
    And fake Pi session "pw-0820-1443" is prompted and finishes its turn
    Then the state database session "pw-0820-1443" has hook status "idle" with reason ""
    And the fake Pi session file of session "pw-0820-1443" has no session_info record
    And the state database session "pw-0820-1443" has name source "derived"
    And session "pw-0820-1443" has no "renamed" event
    When fake Pi session "pw-0820-1443" is renamed inside Pi to "Inside Pi"
    Then the state database session "Inside Pi" has name source "user"
    And deck client "A" screen contains "Inside Pi"
    And the state database session "Inside Pi" has an event of kind "renamed" with reason containing "harness:pi"
    And the fake Pi session file of session "Inside Pi" has the name "Inside Pi"
    When fake Pi session "Inside Pi" is prompted and finishes its turn
    Then the state database session "Inside Pi" has name source "user"
    And the fake Pi session file of session "Inside Pi" has 1 session_info record
    And the state database session "Inside Pi" has no pending harness name
    When deck client "A" exits cleanly

  Scenario: a keystroke rename is refused while a Pi dialog is focused
    Given a long-running fake "pi" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates pi session "dialog guard" with permission profile "safe"
    And fake Pi session "dialog guard" starts its session
    And fake Pi session "dialog guard" is prompted and finishes its turn
    Then the state database session "dialog guard" has hook status "idle" with reason ""
    When the deck extension of fake Pi session "dialog guard" loses its control file
    Then the fake Pi control file of session "dialog guard" does not exist
    When fake Pi session "dialog guard" opens a confirm dialog "Allow bash?"
    Then the state database session "dialog guard" has hook status "idle" with reason ""
    When deck client "A" renames session "dialog guard" to "guard renamed" from its detail dialog
    Then the state database session "guard renamed" has an event of kind "harness_rename_refused" with reason containing "a harness dialog is focused"
    And the state database session "guard renamed" has name source "user"
    And deck client "A" screen contains "rename not pushed to pi"
    And the fake Pi pane of session "guard renamed" does not show "fake-pi dialog answered"
    And the fake Pi pane of session "guard renamed" does not show "/name guard renamed"
    And the fake Pi session file of session "guard renamed" has the name "dialog guard"
    And the fake Pi session file of session "guard renamed" has 1 session_info record
    And the state database session "guard renamed" has no pending harness name
    And session "guard renamed" has no "harness_rename_failed" event
    When deck client "A" exits cleanly
