@requirement-32
Feature: The E event log: newest first, kind/reason/bounded payload, masked env (I-9, task 124)
  SPEC \u00a712/requirement 32 promises the event log is reachable and discoverable
  (\u00a711's R7 rule): `E` opens a read-only listing of every recorded event across
  every session, newest first, and any secret-shaped payload value masks the
  same way the `e` env editor's does (task 010's one predicate, checked here
  with task 011's own real-grid scan, never a substring search over the
  trimmed frame string).

  @requirement-32-event-log-newest-first
  Scenario: three events recorded in one order render newest first, not in insertion or alphabetical order
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates shell session "eventlog-archived"
    And deck client "A" archives its selected session "eventlog-archived"
    And deck client "A" creates claude session "eventlog-profile" with permission profile "safe"
    And deck client "A" opens the permission profile dialog for session "eventlog-profile"
    And deck client "A" cycles the open dialog's field right
    And deck client "A" submits the open dialog
    And deck client "A" creates shell session "eventlog-envkey"
    And deck client "A" opens the env editor for session "eventlog-envkey"
    And deck client "A" edits the highlighted env key to "eventlog-env-value"
    And deck client "A" closes the dialog with escape
    And deck client "A" opens the event log
    Then deck client "A" screen contains "Event log"
    And deck client "A" screen contains "archived"
    And deck client "A" screen contains "set_permission_profile"
    And deck client "A" screen contains "set_env"
    # Recorded oldest to newest: archived, set_permission_profile, set_env.
    # Newest first means the reverse -- which also differs from sorting the
    # three kinds alphabetically (archived < set_env < set_permission_profile).
    And deck client "A" screen text "set_env" appears above screen text "set_permission_profile"
    And deck client "A" screen text "set_permission_profile" appears above screen text "archived"
    When deck client "A" closes the dialog with escape
    And deck client "A" exits cleanly

  @requirement-32-event-log-masks-secret-shaped-payload
  Scenario: a secret-shaped key's value in a payload never reaches the event log's screen grid
    Given deck client "A" is started
    And the state database has a raw event with kind "note" reason "user", secret-shaped key "AUDIT_ENV_TOKEN" value "leak-eventlog-feature-secret-4c8a1e", and ordinary key "note" value "eventlog-ordinary-value-2f91"
    When deck client "A" opens the event log
    Then deck client "A" screen contains "note"
    And deck client "A" screen contains "eventlog-ordinary-value-2f91"
    And deck client "A" screen grid never contains "leak-eventlog-feature-secret-4c8a1e"
    When deck client "A" closes the dialog with escape
    And deck client "A" exits cleanly
