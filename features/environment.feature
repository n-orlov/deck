@environment
Feature: The `e` env editor shows the effective value and winning layer per key (requirement 20, SPEC §6.1/§6.3)
  `e` opens a read-only editor listing every key deck resolved a layer for
  (config.toml's [env] table, the session's own env map, and PATH via
  captured_path), the value that actually reaches the pane, and which of
  the SPEC §6.1/§6.3 layers -- server env, captured_path, config [env],
  session env, lowest to highest -- supplied it. Esc closes it, changing
  nothing: writing an edit, env_dirty and the tmux mirror are a later
  task's own deliverable, not this one's.

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
