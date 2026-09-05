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

  @requirement-114-installed-between-opens
  Scenario: installing an agent after a client starts does not change that client, but a new client sees it
    # A deck client's environment is fixed at process start: it probes PATH
    # once and never again, so the only way to observe "claude just became
    # available" is to start a second client after the fake is installed --
    # client "A" (already running) must keep offering only shell.
    Given deck client "A" is started
    When deck client "A" opens the create modal
    Then deck client "A" screen contains "Agent: shell (left/right cycles: shell)"
    When deck client "A" closes the create modal
    And a fake "claude" binary is on PATH for future deck clients
    And deck client "B" is started
    When deck client "B" opens the create modal
    Then deck client "B" screen contains "Agent: shell (left/right cycles: claude, shell)"
    When deck client "B" closes the create modal
    And deck client "A" exits cleanly
    And deck client "B" exits cleanly

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

  @requirement-114-create-preflight-refusal
  Scenario: an agent removed from PATH after the create modal opens is refused at submit, in-dialog, with nothing persisted
    # The Agent field's cycle list is only ever probed once, at client
    # start (task 005's availability seam) -- so it can still list claude
    # as available even after the fake binary that made it available has
    # since been removed from PATH. Submitting the create is the moment
    # deck's own create-preflight (internal/service.CreateAgent, task 010)
    # re-checks PATH for real: it must refuse in-dialog, leaving the modal
    # open with the typed name intact, rather than creating a doomed row.
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" opens the create modal
    And deck client "A" types "preflight-race" into the create modal name field
    And deck client "A" presses down 2 times in the open dialog
    And deck client "A" cycles the open dialog's field right
    Then deck client "A" screen contains "Agent: claude"
    When the fake "claude" binary is removed from PATH
    And deck client "A" submits the create modal expecting rejection
    Then deck client "A" screen contains "not found on PATH"
    And deck client "A" screen contains "Agent: claude"
    And deck client "A" screen contains "preflight-race"
    And the state database does not contain session "preflight-race"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly
