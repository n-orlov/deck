Feature: Godog harness wiring
  The repository's normal Go test command discovers default feature files.

  Scenario: default feature suite is wired
    Given the Godog harness is available

  Scenario: a private tmux server can be removed
    Given the private tmux server is killed
