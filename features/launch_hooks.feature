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

  @requirement-105-global-hook-self-selects-on-name
  Scenario: a global pre_launch that self-selects on the session's own name exports a variable only for the matching session
    # CreateShell (the plain `shell` kind) never composes pre_launch at all
    # (a separate, already-noted gap) -- claude is the kind whose launch
    # path actually runs through buildPaneCommand's global-then-session
    # composition, so it is what these three scenarios use throughout.
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And the deck config runs global pre_launch command "case $DECK_SESSION_NAME in global-hook-match) export DECK_GLOBAL_PRELAUNCH_MARKER=matched ;; esac"
    And deck client "A" is started
    When deck client "A" creates claude session "global-hook-match" with permission profile "safe"
    Then deck client "A" screen contains "global-hook-match"
    And the live pane process environment for session "global-hook-match" key "DECK_GLOBAL_PRELAUNCH_MARKER" is "matched"
    When deck client "A" creates claude session "global-hook-bystander" with permission profile "safe"
    Then deck client "A" screen contains "global-hook-bystander"
    And the live pane process environment for session "global-hook-bystander" has no key "DECK_GLOBAL_PRELAUNCH_MARKER"
    When deck client "A" exits cleanly

  @requirement-105-global-and-session-hooks-compose
  Scenario: a global pre_launch and a session's own pre_launch compose, global first, in the same shell
    # The typed Pre-launch command field wraps at the create dialog's own
    # fixed content width, and a wrapped value's border/newline characters
    # break a literal substring match against the frame -- G_TOK/S_RES stay
    # short on purpose so the whole typed command renders on one line.
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And the deck config runs global pre_launch command "export G_TOK=tok"
    And deck client "A" is started
    When deck client "A" creates claude session "composed hook target" with permission profile "safe" and pre-launch command "[ $G_TOK = tok ] && export S_RES=ok"
    Then deck client "A" screen contains "composed hook target"
    And the live pane process environment for session "composed hook target" key "G_TOK" is "tok"
    And the live pane process environment for session "composed hook target" key "S_RES" is "ok"
    When deck client "A" exits cleanly

  @requirement-105-failing-global-hook-refuses-launch
  Scenario: a failing global pre_launch leaves the row in error with the agent never started and the hook's own output in the retained pane
    Given a fake "claude" binary is on PATH for future deck clients
    And the deck config runs global pre_launch command "echo global-prelaunch-failure-marker >&2; exit 9"
    And deck client "A" is started
    When deck client "A" creates claude session "failing global hook target" with permission profile "safe"
    Then within one configured reconcile interval deck client "A" screen contains "error"
    And the state database session "failing global hook target" has an event of kind "tmux.pane_dead" with reason containing "exited with status 9"
    When deck client "A" opens detail for session "failing global hook target"
    Then deck client "A" screen contains "global-prelaunch-failure-marker"
    # The fake claude fixture's first line of output is its own banner, so
    # the same retained-pane tail that shows the hook's stderr proves the
    # agent was never exec'd: the banner is nowhere in it.
    And deck client "A" screen does not contain "Fake Claude Code"
    When deck client "A" exits cleanly

  @requirement-108-launch-edit-applies-only-on-restart
  Scenario: a launch input edited on a live row shows launch↻ until R applies it and clears the badge
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "launch edit target" with permission profile "safe"
    Then deck client "A" screen contains "launch edit target"
    And the live pane process environment for session "launch edit target" has no key "DECK_LAUNCH_EDIT_MARKER"
    When deck client "A" opens the launch inputs editor for session "launch edit target"
    And deck client "A" types "export DECK_LAUNCH_EDIT_MARKER=applied" into the pre-launch field
    And deck client "A" submits the launch inputs editor
    And deck client "A" closes detail
    Then deck client "A" screen contains "launch*"
    And the state database session "launch edit target" is marked launch_dirty
    And the live pane process environment for session "launch edit target" has no key "DECK_LAUNCH_EDIT_MARKER"
    When deck client "A" presses R on session "launch edit target"
    Then within one configured reconcile interval deck client "A" screen contains "fake-claude resume:"
    And the state database session "launch edit target" is not marked launch_dirty
    And deck client "A" screen does not contain "launch*"
    And the live pane process environment for session "launch edit target" key "DECK_LAUNCH_EDIT_MARKER" is "applied"
    When deck client "A" exits cleanly
