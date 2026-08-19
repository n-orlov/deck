@crash
Feature: Clean exits and crashed agent panes
  Reconciliation distinguishes an ordinary shell exit from a failed agent,
  captures the failed pane before collection, and never relaunches it.

  Scenario: a shell exit zero is a clean stop without a crash artifact
    Given deck client "A" is started
    When deck client "A" creates shell session "clean shell"
    And shell session "clean shell" exits with status zero
    Then within one configured reconcile interval deck client "A" screen contains "resumable"
    And the state database session "clean shell" is cleanly stopped without a crash artifact
    And the private tmux session "deck_clean-shell" does not exist
    When deck client "A" exits cleanly

  Scenario: SIGKILL captures and sanitizes the agent pane without relaunching
    Given a crash-tail fixture and long-running fake Claude are configured
    And deck client "A" is started
    When deck client "A" creates claude session "crashed claude" with permission profile "safe"
    And fake Claude session "crashed claude" renders the colored crash-tail fixture
    And the agent process "fake-claude-real" in private tmux session "deck_crashed-claude" is killed with SIGKILL
    Then within one configured reconcile interval deck client "A" screen contains "error"
    And the state database session "crashed claude" has a sanitized last-200-line crash artifact
    And the private tmux session "deck_crashed-claude" does not exist
    And the audit log has 1 launch record for session "crashed claude"
    When deck client "A" opens detail for session "crashed claude"
    Then deck client "A" screen contains "crash final line"
    When deck client "A" exits cleanly

  @multiclient
  Scenario: racing clients collect one corpse idempotently
    Given a crash-tail fixture and long-running fake Claude are configured
    And deck client "A" is started
    And deck client "B" is started
    And deck client "C" is started
    When deck client "A" creates claude session "shared corpse" with permission profile "safe"
    And fake Claude session "shared corpse" renders the colored crash-tail fixture
    And the agent process "fake-claude-real" in private tmux session "deck_shared-corpse" is killed with SIGKILL
    Then the state database session "shared corpse" has a sanitized last-200-line crash artifact
    And the crash artifact for session "shared corpse" remains unchanged across another reconcile interval
    And the private tmux session "deck_shared-corpse" does not exist
    And the audit log has 1 launch record for session "shared corpse"
    When deck client "A" exits cleanly
    And deck client "B" exits cleanly
    And deck client "C" exits cleanly

  Scenario: a different hook detects a crash while no TUI is running
    Given a crash-tail fixture and long-running fake Claude are configured
    And deck client "A" is started
    When deck client "A" creates claude session "unattended victim" with permission profile "safe"
    And deck client "A" creates claude session "unattended emitter" with permission profile "safe"
    And fake Claude session "unattended victim" renders the colored crash-tail fixture
    And deck client "A" exits cleanly
    And the agent process "fake-claude-real" in private tmux session "deck_unattended-victim" is killed with SIGKILL
    And fake Claude session "unattended emitter" fires "SessionStart" for itself using injected identity:
      | source | fresh |
    Then the state database session "unattended victim" has a sanitized last-200-line crash artifact
    And the private tmux session "deck_unattended-victim" does not exist
    And the audit log has 1 launch record for session "unattended victim"
