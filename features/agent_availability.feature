@create-session
Feature: Nothing installed means the Agent field offers only shell (requirement 114)
  A deck client started with no coding-agent binary on PATH still has a
  usable create modal: the Agent field's cycle is exactly {shell}, cycling
  never advances off it, and the help text says which registered kinds are
  hidden and why.

  @requirement-114-nothing-installed
  Scenario: with nothing installed the Agent field offers only shell
    Given deck client "A" is started
    When deck client "A" opens the create modal
    Then deck client "A" screen contains "Agent: shell (left/right cycles: shell)"
    And deck client "A" screen contains "not on PATH: claude, pi"
    When deck client "A" presses down 2 times in the open dialog
    And deck client "A" cycles the open dialog's field right
    And deck client "A" cycles the open dialog's field right
    Then deck client "A" screen contains "Agent: shell (left/right cycles: shell)"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  @requirement-114-only-claude-installed
  Scenario: with only claude installed the Agent field offers shell and claude but not pi
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" opens the create modal
    Then deck client "A" screen contains "Agent: shell (left/right cycles: claude, shell)"
    And deck client "A" screen contains "not on PATH: pi"
    When deck client "A" presses down 2 times in the open dialog
    And deck client "A" cycles the open dialog's field right
    Then deck client "A" screen contains "Agent: claude (left/right cycles: claude, shell)"
    When deck client "A" cycles the open dialog's field right
    Then deck client "A" screen contains "Agent: shell (left/right cycles: claude, shell)"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly
