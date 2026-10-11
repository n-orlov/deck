@harness-names
Feature: Codex session names travel between deck and the harness
  Codex keeps a thread's name in state_5.sqlite (threads.name, NULL until the
  thread is named), appends each naming to session_index.jsonl and shows
  "<name> | <directory>" as its terminal title. The fake Codex persists those
  same records, so each scenario asserts through them and through deck's store
  and screen, never through a fixed sleep. Deck types a launch name and a
  pushed rename into the pane as "/rename <name>" once the prompt is ready.

  Scenario: an explicit name typed in deck reaches Codex before the first prompt
    Given a long-running fake "codex" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates codex session "billing audit" with permission profile "safe"
    Then the state database session "billing audit" has an event of kind "harness_name_sent" with reason containing "billing audit"
    And the fake Codex pane of session "billing audit" is titled "billing audit"
    When fake Codex session "billing audit" is prompted with "first"
    Then the fake Codex thread of session "billing audit" is named "billing audit"
    And the fake Codex session index of session "billing audit" ends with the name "billing audit"
    And the state database session "billing audit" has name source "user"
    When fake Codex session "billing audit" is asked to generate the title "Some generated guess"
    And fake Codex session "billing audit" finishes its turn with "done"
    Then the fake Codex thread of session "billing audit" is named "billing audit"
    And the state database session "billing audit" has name source "user"
    And the state database does not contain session "Some generated guess"
    And deck client "A" screen contains "billing audit"
    When deck client "A" exits cleanly

  Scenario: a name Codex generated is adopted from its database
    Given a long-running fake "codex" binary is on PATH for future deck clients
    And deck client "A" is started with the fake agents in a fresh directory labelled "nx" with the clock frozen at "2025-08-20T14:43:00Z"
    When deck client "A" creates codex session with a blank name expecting the derived name "nx-0820-1443"
    Then the state database session "nx-0820-1443" has name source "derived"
    When fake Codex session "nx-0820-1443" is prompted with "write a haiku"
    Then the fake Codex thread of session "nx-0820-1443" has no name
    When fake Codex session "nx-0820-1443" generates the title "Write flaky test haiku"
    And fake Codex session "nx-0820-1443" finishes its turn with "done"
    Then the state database session "Write flaky test haiku" has name source "harness"
    And deck client "A" screen contains "Write flaky test haiku"
    And the state database session "Write flaky test haiku" has an event of kind "renamed" with reason containing "harness:codex"
    And the state database session "Write flaky test haiku" has no pending harness name
    And session "Write flaky test haiku" has no "harness_rename_failed" event
    When deck client "A" exits cleanly

  Scenario: a rename in deck is pushed into Codex and does not loop back
    Given a long-running fake "codex" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates codex session "push me" with permission profile "safe"
    Then the state database session "push me" has an event of kind "harness_name_sent" with reason containing "push me"
    And the fake Codex pane of session "push me" is titled "push me"
    When fake Codex session "push me" is prompted with "first"
    And fake Codex session "push me" finishes its turn with "ready"
    Then the fake Codex thread of session "push me" is named "push me"
    When deck client "A" renames session "push me" to "pushed title" from its detail dialog
    Then the fake Codex thread of session "pushed title" is named "pushed title"
    And the fake Codex session index of session "pushed title" ends with the name "pushed title"
    And deck client "A" screen contains "pushed title"
    When fake Codex session "pushed title" is prompted with "carry on"
    Then the state database session "pushed title" has no pending harness name
    And the state database session "pushed title" has name source "user"
    And the state database session "pushed title" has an event of kind "renamed" with reason containing "user"
    And the state database session "pushed title" has no "renamed" event with reason containing "harness:"
    And session "pushed title" has no "harness_rename_failed" event
    And session "pushed title" has no "harness_rename_refused" event
    When deck client "A" exits cleanly

  Scenario: a rename typed inside Codex is adopted and freezes the name
    Given a long-running fake "codex" binary is on PATH for future deck clients
    And deck client "A" is started with the fake agents in a fresh directory labelled "ny" with the clock frozen at "2025-08-20T14:43:00Z"
    When deck client "A" creates codex session with a blank name expecting the derived name "ny-0820-1443"
    And fake Codex session "ny-0820-1443" is prompted with "hello"
    And fake Codex session "ny-0820-1443" generates the title "Generated first"
    And fake Codex session "ny-0820-1443" finishes its turn with "done"
    Then the state database session "Generated first" has name source "harness"
    When fake Codex session "Generated first" is renamed inside Codex to "Inside Codex"
    And fake Codex session "Generated first" is prompted with "and then"
    Then the state database session "Inside Codex" has name source "user"
    And deck client "A" screen contains "Inside Codex"
    And the state database session "Inside Codex" has an event of kind "renamed" with reason containing "harness:codex"
    When fake Codex session "Inside Codex" is asked to generate the title "Never adopted"
    And fake Codex session "Inside Codex" finishes its turn with "done again"
    Then the fake Codex thread of session "Inside Codex" is named "Inside Codex"
    And the state database session "Inside Codex" has name source "user"
    And the state database does not contain session "Never adopted"
    When deck client "A" exits cleanly

  Scenario: a rename is refused while a Codex dialog is waiting for an answer
    Given a long-running fake "codex" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates codex session "dialog guard" with permission profile "safe"
    Then the state database session "dialog guard" has an event of kind "harness_name_sent" with reason containing "dialog guard"
    And the fake Codex pane of session "dialog guard" is titled "dialog guard"
    When fake Codex session "dialog guard" is prompted with "first"
    And fake Codex session "dialog guard" requests approval to run tool "apply_patch"
    Then the state database session "dialog guard" has hook status "waiting", reason "apply_patch", message "", acknowledged 0, and notify_epoch 0
    When deck client "A" renames session "dialog guard" to "guard renamed" from its detail dialog
    Then the state database session "guard renamed" has an event of kind "harness_rename_refused" with reason containing "waiting for an answer"
    And the state database session "guard renamed" has name source "user"
    And deck client "A" screen contains "rename not pushed to codex"
    And the fake Codex thread of session "guard renamed" is named "dialog guard"
    And the state database session "guard renamed" has no pending harness name
    And session "guard renamed" has no "harness_rename_failed" event
    When deck client "A" exits cleanly
