Feature: Fake Claude fixture mechanism
  The repository-built fake Claude is usable as a real tmux session command,
  without a deck-only status back channel.

  Scenario: SIGKILL targets the agent process and retains its failed pane
    Given the repository-built fake Claude fixture is ready
    And the fake Claude fixture is launched as a long-running private tmux session "deck_fake-agent-crash"
    When the agent process "fake-claude" in private tmux session "deck_fake-agent-crash" is killed with SIGKILL
    Then the private tmux session "deck_fake-agent-crash" retains a dead pane with a nonzero termination

  Scenario: accepted argv and controlled exit statuses are observable from panes
    Given the repository-built fake Claude fixture is ready
    When the fake Claude fixture is launched successfully as a private tmux session command
    Then its success pane shows the deterministic banner and exact accepted argv
    And the successful fake Claude session exits with status 0
    When the fake Claude fixture is launched with controlled failure status 7
    Then its failure pane shows the deterministic banner and exact accepted argv
    And the failed fake Claude session remains with status 7
