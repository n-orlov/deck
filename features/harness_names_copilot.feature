@harness-names
Feature: Copilot session names travel between deck and the harness
  Copilot keeps a session's name in workspace.yaml (name: and user_named:),
  titles its terminal "<name> - GitHub Copilot", generates a title after the
  first prompt of an unnamed session (user_named: false) and takes a typed
  "/rename <name>" at once (user_named: true). The fake Copilot persists those
  same records, so each scenario asserts through them and through deck's store
  and screen, never through a fixed sleep.

  Scenario: an explicit name typed in deck reaches Copilot at launch
    Given a long-running fake "copilot" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates copilot session "billing audit" with permission profile "safe"
    Then the audit log's most recent launch argv for session "billing audit" contains "-n"
    And the audit log's most recent launch argv for session "billing audit" contains "billing audit"
    And the fake Copilot workspace of session "billing audit" is named "billing audit" with user_named true
    And the fake Copilot pane of session "billing audit" is titled "billing audit"
    And the state database session "billing audit" has name source "user"
    When fake Copilot session "billing audit" is prompted with "something else entirely" and finishes its turn
    Then the state database session "billing audit" has hook status "idle" with reason "end_turn"
    And the fake Copilot workspace of session "billing audit" is named "billing audit" with user_named true
    And the state database session "billing audit" has name source "user"
    And the state database does not contain session "something else entirely"
    And deck client "A" screen contains "billing audit"
    When deck client "A" exits cleanly

  Scenario: a name Copilot generated is adopted after the first turn and kept
    Given a long-running fake "copilot" binary is on PATH for future deck clients
    And deck client "A" is started with the fake agents in a fresh directory labelled "nz" with the clock frozen at "2025-08-20T14:43:00Z"
    When deck client "A" creates copilot session with a blank name expecting the derived name "nz-0820-1443"
    Then the state database session "nz-0820-1443" has name source "derived"
    And the fake Copilot workspace of session "nz-0820-1443" has no name
    When fake Copilot session "nz-0820-1443" is prompted with "tidy config loader now" and finishes its turn
    Then the state database session "tidy config loader now" has name source "harness"
    And the fake Copilot workspace of session "tidy config loader now" is named "tidy config loader now" with user_named false
    And deck client "A" screen contains "tidy config loader now"
    And the state database session "tidy config loader now" has an event of kind "renamed" with reason containing "harness:copilot"
    And the state database session "tidy config loader now" has no pending harness name
    And session "tidy config loader now" has no "harness_rename_failed" event
    When fake Copilot session "tidy config loader now" is prompted with "and then something else" and finishes its turn
    Then the fake Copilot workspace of session "tidy config loader now" is named "tidy config loader now" with user_named false
    And the state database session "tidy config loader now" has name source "harness"
    And the state database does not contain session "and then something else"
    When deck client "A" exits cleanly

  Scenario: a rename in deck is pushed into Copilot and does not loop back
    Given a long-running fake "copilot" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates copilot session "push me" with permission profile "safe"
    And fake Copilot session "push me" is prompted with "first" and finishes its turn
    Then the state database session "push me" has hook status "idle" with reason "end_turn"
    When deck client "A" renames session "push me" to "pushed title" from its detail dialog
    Then the fake Copilot workspace of session "pushed title" is named "pushed title" with user_named true
    And the fake Copilot pane of session "pushed title" is titled "pushed title"
    And deck client "A" screen contains "pushed title"
    When fake Copilot session "pushed title" is prompted with "carry on" and finishes its turn
    Then the state database session "pushed title" has no pending harness name
    And the state database session "pushed title" has name source "user"
    And the state database session "pushed title" has an event of kind "renamed" with reason containing "user"
    And the state database session "pushed title" has no "renamed" event with reason containing "harness:"
    And session "pushed title" has no "harness_rename_failed" event
    And session "pushed title" has no "harness_rename_refused" event
    When deck client "A" exits cleanly

  Scenario: a rename typed inside Copilot is adopted and freezes the name
    Given a long-running fake "copilot" binary is on PATH for future deck clients
    And deck client "A" is started with the fake agents in a fresh directory labelled "nw" with the clock frozen at "2025-08-20T14:43:00Z"
    When deck client "A" creates copilot session with a blank name expecting the derived name "nw-0820-1443"
    And fake Copilot session "nw-0820-1443" is renamed inside Copilot to "Inside Copilot"
    Then the state database session "Inside Copilot" has name source "user"
    And deck client "A" screen contains "Inside Copilot"
    And the state database session "Inside Copilot" has an event of kind "renamed" with reason containing "harness:copilot"
    And the fake Copilot pane of session "Inside Copilot" is titled "Inside Copilot"
    When fake Copilot session "Inside Copilot" is prompted with "hello" and finishes its turn
    Then the fake Copilot workspace of session "Inside Copilot" is named "Inside Copilot" with user_named true
    And the state database session "Inside Copilot" has name source "user"
    And the state database does not contain session "hello"
    And the state database session "Inside Copilot" has no pending harness name
    When deck client "A" exits cleanly

  Scenario: a rename is refused while a Copilot permission dialog is focused
    Given a long-running fake "copilot" binary rendering its screen is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates copilot session "dialog guard" with permission profile "safe"
    And fake Copilot session "dialog guard" starts working on "first"
    Then the state database session "dialog guard" has hook status "idle" with reason "new"
    When fake Copilot session "dialog guard" opens a permission dialog for "sleep 90"
    Then the fake Copilot pane of session "dialog guard" shows "Do you want to run this command?"
    And the state database session "dialog guard" has hook status "idle" with reason "new"
    When deck client "A" renames session "dialog guard" to "guard renamed" from its detail dialog
    Then the state database session "guard renamed" has an event of kind "harness_rename_refused" with reason containing "a harness dialog is focused"
    And the state database session "guard renamed" has name source "user"
    And deck client "A" screen contains "rename not pushed to copilot"
    And the fake Copilot pane of session "guard renamed" shows "Do you want to run this command?"
    And the fake Copilot workspace of session "guard renamed" is named "dialog guard" with user_named true
    And the state database session "guard renamed" has no pending harness name
    And session "guard renamed" has no "harness_rename_failed" event
    When deck client "A" exits cleanly
