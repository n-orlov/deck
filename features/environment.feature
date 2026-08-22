@environment
Feature: The `e` env editor shows the effective value and winning layer per key, and writes an edit (requirements 20 and task 021, SPEC §6.1/§6.3)
  `e` opens an editor listing every key deck resolved a layer for
  (config.toml's [env] table, the session's own env map, and PATH via
  captured_path), the value that actually reaches the pane, and which of
  the SPEC §6.1/§6.3 layers -- server env, captured_path, config [env],
  session env, lowest to highest -- supplied it. Esc with nothing being
  edited closes the whole dialog, changing nothing. Enter on a highlighted
  row opens it for editing; typing then Enter commits the new value into
  the session's own env map, marks the row env_dirty (shown on the sidebar
  as `env↻`) and mirrors the key into tmux's own environment table for
  FUTURE panes only -- the live pane's already-running process keeps
  whatever it started with until an explicit restart applies it (task
  022's own deliverable).

  @requirement-20-env-editor-winning-layer
  Scenario: a key set in two layers names the session layer as the winner, and PATH names captured_path
    Given the scenario's config.toml is written with:
      """
      [env]
      ENV_LAYER_KEY = "config-value"
      """
    And a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "env layers" with permission profile "safe" and env "ENV_LAYER_KEY=session-value"
    Then deck client "A" screen contains "env layers"
    When deck client "A" opens the env editor for session "env layers"
    Then deck client "A" screen contains "ENV_LAYER_KEY"
    And deck client "A" screen contains "session-value"
    And deck client "A" screen contains "session env"
    And deck client "A" screen contains "PATH"
    And deck client "A" screen contains "captured_path"
    When deck client "A" closes the dialog with escape
    And deck client "A" exits cleanly

  @requirement-20-env-editor-esc-changes-nothing
  Scenario: esc closes the env editor changing nothing
    Given deck client "A" is started
    And deck client "A" creates shell session "plain env"
    And the scenario's config.toml is captured as "before-env-esc"
    When deck client "A" opens the env editor for session "plain env"
    And deck client "A" closes the dialog with escape
    Then deck client "A" screen contains "deck - sessions"
    And the scenario's config.toml still matches the captured "before-env-esc"
    And the state database session "plain env" has no env key "ENV_LAYER_KEY"
    When deck client "A" exits cleanly

  @requirement-021-env-editor-writes-env-dirty-and-tmux-mirror
  Scenario: committing an edit writes session env, marks it env_dirty, mirrors into tmux, and leaves the live pane's own environment untouched
    Given deck client "A" is started
    When deck client "A" creates shell session "env write target"
    Then deck client "A" screen contains "env write target"
    And the live pane process environment for session "env write target" key "PATH" is captured as "before-edit"
    When deck client "A" opens the env editor for session "env write target"
    Then deck client "A" screen contains "PATH"
    When deck client "A" edits the highlighted env key to "/edited/only/path"
    Then deck client "A" screen contains "/edited/only/path"
    When deck client "A" closes the dialog with escape
    Then deck client "A" screen contains "env*"
    And the state database session "env write target" has env key "PATH" with value "/edited/only/path"
    And the state database session "env write target" is marked env_dirty
    And the live tmux environment for session "env write target" has key "PATH" with value "/edited/only/path"
    And the live pane process environment for session "env write target" key "PATH" still matches the captured "before-edit"
    When deck client "A" exits cleanly

  @requirement-022-restart-applies-pending-env-edit-and-clears-badge
  Scenario: R kills and relaunches the pane with the resume argv, applying a pending env edit and clearing env_dirty
    Given deck client "A" is started
    When deck client "A" creates shell session "restart env target"
    Then deck client "A" screen contains "restart env target"
    And the live pane process environment for session "restart env target" key "PATH" is captured as "before-restart-edit"
    When deck client "A" opens the env editor for session "restart env target"
    Then deck client "A" screen contains "PATH"
    When deck client "A" edits the highlighted env key to "/restart/applied/path"
    Then deck client "A" screen contains "/restart/applied/path"
    When deck client "A" closes the dialog with escape
    Then deck client "A" screen contains "env*"
    And the state database session "restart env target" is marked env_dirty
    And the live pane process environment for session "restart env target" key "PATH" still matches the captured "before-restart-edit"
    When deck client "A" presses R on session "restart env target"
    Then within one configured reconcile interval deck client "A" screen contains "running"
    And the state database session "restart env target" is not marked env_dirty
    And deck client "A" screen does not contain "env*"
    And the live pane process environment for session "restart env target" key "PATH" is "/restart/applied/path"
    When deck client "A" exits cleanly
