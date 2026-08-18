@reboot
Feature: Durable conversation identity survives a tmux-server kill
  SPEC §13.4's T1 scenario is the product's headline promise: three claude
  sessions sharing one working directory each keep their own conversation,
  and that identity survives losing every live tmux pane (this suite's
  stand-in for a host reboot, since CI has no host to actually reboot) and a
  full deck restart. Nothing is ever resumed by "most recent" — only by the
  session's own persisted conversation id.

  Scenario: alpha, beta and gamma in one directory keep their own conversations
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "alpha" with permission profile "safe" and message "alpha distinct message"
    And deck client "A" creates claude session "beta" with permission profile "safe" and message "beta distinct message"
    And deck client "A" creates claude session "gamma" with permission profile "safe" and message "gamma distinct message"
    Then deck client "A" screen contains "resumable"
    And the state database contains session "alpha" with status "stopped"
    And the state database contains session "beta" with status "stopped"
    And the state database contains session "gamma" with status "stopped"
    And the state database session "alpha" has a non-empty conversation id
    And the state database session "beta" has a non-empty conversation id
    And the state database session "gamma" has a non-empty conversation id
    And the state database sessions "alpha" and "beta" have different conversation ids
    And the state database sessions "beta" and "gamma" have different conversation ids
    When deck client "A" exits cleanly
    # CI stand-in for a host reboot: every live tmux pane is gone, but the
    # durable DECK_HOME state is untouched.
    And the private tmux server is killed
    Then no private tmux session exists
    # Deck restarted: a fresh process reading the same durable DECK_HOME.
    When deck client "B" is started
    Then deck client "B" screen contains "resumable"
    And the state database contains session "alpha" with status "stopped"
    And the state database contains session "beta" with status "stopped"
    And the state database contains session "gamma" with status "stopped"
    When deck client "B" presses r on session "beta"
    Then deck client "B" screen contains "starting"
    And the audit log's most recent launch argv for session "beta" contains "--resume"
    And the audit log's most recent launch argv for session "beta" contains session "beta"'s conversation id
    And the audit log's most recent launch argv for session "beta" does not contain "--continue"
    And session "beta" replays its own last message, not session "alpha"'s
    When deck client "B" exits cleanly
