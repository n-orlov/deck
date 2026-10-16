@harness-names
Feature: Claude session names travel between deck and the harness
  Names move through the fake Claude's own persisted state: the custom-title,
  agent-name and ai-title records it appends to its transcript, the pane title
  it sets, and the session_title it puts on hook payloads. Each scenario
  asserts through that state and through deck's store and screen, never
  through a fixed sleep.

  Scenario: an explicit name typed in deck reaches Claude at launch
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "billing audit" with permission profile "safe"
    Then the audit log's most recent launch argv for session "billing audit" contains "--name"
    And the audit log's most recent launch argv for session "billing audit" contains "billing audit"
    When fake Claude session "billing audit" fires "UserPromptSubmit" for itself using injected identity:
      | prompt | first |
    Then the state database session "billing audit" has name source "user"
    And the fake Claude transcript of session "billing audit" has a "custom-title" record titled "billing audit"
    And the fake Claude transcript of session "billing audit" has a "agent-name" record titled "billing audit"
    And the fake Claude transcript of session "billing audit" has no "ai-title" record
    When fake Claude session "billing audit" fires "Stop" for itself using injected identity:
      | last_assistant_message | done |
    And fake Claude session "billing audit" is asked to generate the title "Some generated guess"
    And fake Claude session "billing audit" fires "UserPromptSubmit" for itself using injected identity:
      | prompt | next |
    Then the state database session "billing audit" has name source "user"
    And the fake Claude transcript of session "billing audit" has no "ai-title" record
    And deck client "A" screen contains "billing audit"
    And the state database does not contain session "Some generated guess"
    When deck client "A" exits cleanly

  Scenario: a name Claude generated is adopted and followed until the user renames
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started with the fake agents in a fresh directory labelled "na" with the clock frozen at "2025-08-20T14:43:00Z"
    When deck client "A" creates claude session with a blank name expecting the derived name "na-0820-1443"
    Then the state database session "na-0820-1443" has name source "derived"
    When fake Claude session "na-0820-1443" generates the title "S3 upload retry policy"
    And fake Claude session "na-0820-1443" fires "Stop" for itself using injected identity:
      | last_assistant_message | done |
    Then the state database session "S3 upload retry policy" has name source "harness"
    And deck client "A" screen contains "S3 upload retry policy"
    And the state database session "S3 upload retry policy" has an event of kind "renamed" with reason containing "harness:claude"
    When fake Claude session "S3 upload retry policy" generates the title "Backoff for uploads"
    And fake Claude session "S3 upload retry policy" fires "UserPromptSubmit" for itself using injected identity:
      | prompt | and then |
    Then the state database session "Backoff for uploads" has name source "harness"
    When deck client "A" renames session "Backoff for uploads" to "my own name" from its detail dialog
    Then the state database session "my own name" has name source "user"
    Then the fake Claude transcript of session "my own name" has a "custom-title" record titled "my own name"
    When fake Claude session "my own name" is asked to generate the title "Claude would rename it again"
    And fake Claude session "my own name" fires "Stop" for itself using injected identity:
      | last_assistant_message | done |
    Then the state database session "my own name" has name source "user"
    And the state database does not contain session "Claude would rename it again"
    And deck client "A" screen contains "my own name"
    When deck client "A" exits cleanly

  Scenario: a rename in deck is pushed into Claude and does not loop back
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "push me" with permission profile "safe"
    And fake Claude session "push me" fires "Stop" for itself using injected identity:
      | last_assistant_message | ready |
    Then the state database session "push me" has hook status "idle", reason "", message "ready", seen, and notify_epoch 0
    When deck client "A" renames session "push me" to "pushed title" from its detail dialog
    Then the fake Claude transcript of session "pushed title" has a "custom-title" record titled "pushed title"
    And deck client "A" screen contains "pushed title"
    When fake Claude session "pushed title" fires "UserPromptSubmit" for itself using injected identity:
      | prompt | carry on |
    Then the state database session "pushed title" has no pending harness name
    And the state database session "pushed title" has name source "user"
    And the state database session "pushed title" has an event of kind "renamed" with reason containing "user"
    And the state database session "pushed title" has no "renamed" event with reason containing "harness:"
    And session "pushed title" has no "harness_rename_failed" event
    And session "pushed title" has no "harness_rename_refused" event
    When deck client "A" exits cleanly

  Scenario: a rename typed inside Claude is adopted and freezes the name
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started with the fake agents in a fresh directory labelled "ni" with the clock frozen at "2025-08-20T14:43:00Z"
    When deck client "A" creates claude session with a blank name expecting the derived name "ni-0820-1443"
    And fake Claude session "ni-0820-1443" is renamed inside Claude to "Inside Claude"
    And fake Claude session "ni-0820-1443" fires "UserPromptSubmit" for itself using injected identity:
      | prompt | hello |
    Then the state database session "Inside Claude" has name source "user"
    And deck client "A" screen contains "Inside Claude"
    And the state database session "Inside Claude" has an event of kind "renamed" with reason containing "harness:claude"
    When fake Claude session "Inside Claude" is asked to generate the title "Never adopted"
    And fake Claude session "Inside Claude" fires "Stop" for itself using injected identity:
      | last_assistant_message | done |
    Then the state database session "Inside Claude" has name source "user"
    And the fake Claude transcript of session "Inside Claude" has no "ai-title" record
    And the state database does not contain session "Never adopted"
    When deck client "A" exits cleanly

  Scenario: a rename is refused while a Claude dialog is waiting for an answer
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "dialog guard" with permission profile "safe"
    And fake Claude session "dialog guard" fires "Notification" for itself using injected identity:
      | notification_type | permission_prompt |
    Then the state database session "dialog guard" has hook status "waiting", reason "permission_prompt", message "", unseen, and notify_epoch 0
    When deck client "A" renames session "dialog guard" to "guard renamed" from its detail dialog
    Then the state database session "guard renamed" has an event of kind "harness_rename_refused" with reason containing "waiting for an answer"
    And the state database session "guard renamed" has name source "user"
    And deck client "A" screen contains "rename not pushed to claude"
    And the fake Claude transcript of session "guard renamed" has no record titled "guard renamed"
    And the state database session "guard renamed" has no pending harness name
    And session "guard renamed" has no "harness_rename_failed" event
    When deck client "A" exits cleanly
