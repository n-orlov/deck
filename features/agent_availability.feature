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
    When deck client "A" cycles the Agent field right 2 times
    Then deck client "A" screen contains "Agent: shell (left/right cycles: shell)"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly
