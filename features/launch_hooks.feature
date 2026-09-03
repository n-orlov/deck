@launch-hooks
Feature: Every pane carries its own session's DECK_SESSION_* context (R104, SPEC §6.1)
  Every pane deck launches -- every adapter, `shell` included, on create and on
  resume alike -- carries the launching session's own row facts as
  DECK_SESSION_* variables, merged last so a session `env` map or a config
  `[env]` entry of the same name can never lie to a hook about which session
  it is running for (SPEC §6.1). This file proves that against the real,
  already-running pane process's own /proc/<pid>/environ, never deck's own
  view of it: the adapter that had no session identity of its own before this
  requirement now carries the full context, a session `env` entry cannot
  impersonate the real name, and one session's create launch and its later
  resume record different DECK_SESSION_LAUNCH_KIND values for the same row.

  @requirement-104-shell-carries-session-context
  Scenario: a shell session carries the deck-owned session context, the adapter that had none before
    Given deck client "A" is started
    When deck client "A" creates shell session "shell context target"
    Then deck client "A" screen contains "shell context target"
    And the live pane process environment for session "shell context target" key "DECK_SESSION_NAME" is "shell context target"
    And the live pane process environment for session "shell context target" key "DECK_SESSION_AGENT" is "shell"
    And the live pane process environment for session "shell context target" key "DECK_SESSION_LAUNCH_KIND" is "create"
    When deck client "A" exits cleanly

  @requirement-104-session-env-cannot-impersonate-name
  Scenario: a session env entry for DECK_SESSION_NAME loses to the real name
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "real name wins" with permission profile "safe" and env "DECK_SESSION_NAME=impersonated"
    Then deck client "A" screen contains "real name wins"
    And the live pane process environment for session "real name wins" key "DECK_SESSION_NAME" is "real name wins"
    When deck client "A" exits cleanly

  @requirement-104-create-then-resume-differ
  Scenario: a session created and then resumed records DECK_SESSION_LAUNCH_KIND as create, then resume
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "launch kind target" with permission profile "safe"
    Then deck client "A" screen contains "launch kind target"
    And the live pane process environment for session "launch kind target" key "DECK_SESSION_LAUNCH_KIND" is "create"
    When deck client "A" kills session "launch kind target"
    Then the state database session "launch kind target" is "stopped" from "user" with killed_by_user=1
    When deck client "A" presses r on session "launch kind target"
    Then deck client "A" screen contains "starting"
    And the live pane process environment for session "launch kind target" key "DECK_SESSION_LAUNCH_KIND" is "resume"
    When deck client "A" exits cleanly
