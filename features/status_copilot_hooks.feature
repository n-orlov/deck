@status-copilot-hooks
Feature: Copilot hook status truth
  Copilot's six observational events are fired by fake-copilot inside its real
  pane through deck's own plugin entries (R217, R218, R224). Copilot fires no
  event at launch, so a fresh row is starting until its first prompt.

  Scenario: Each Copilot hook maps to the status R218 gives it
    Given a long-running fake "copilot" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates copilot session "copilot truth" with permission profile "safe"
    Then the state database session "copilot truth" has status "starting" from "tmux"

    When fake Copilot session "copilot truth" starts working on "write the file"
    # The first prompt fires userPromptSubmitted and then sessionStart, whose
    # reason is the payload's source.
    Then the state database session "copilot truth" has hook status "running" with reason "new"
    And session "copilot truth" has a "session_start" event

    When fake Copilot session "copilot truth" raises a "permission_prompt" notification
    Then the state database session "copilot truth" has hook status "waiting" with reason "permission_prompt"
    And within one configured reconcile interval deck client "A" screen contains "waiting"

    When fake Copilot session "copilot truth" raises a "idle_prompt" notification
    Then the state database session "copilot truth" has hook status "waiting" with reason "permission_prompt"

    When fake Copilot session "copilot truth" finishes its turn
    Then the state database session "copilot truth" has hook status "idle" with reason "end_turn"
    And session "copilot truth" has a "transcript" event

    When fake Copilot session "copilot truth" starts working on "ask me something"
    Then the state database session "copilot truth" has hook status "running" with reason "prompt"

    When fake Copilot session "copilot truth" raises a "elicitation_dialog" notification
    Then the state database session "copilot truth" has hook status "waiting" with reason "elicitation_dialog"

    When fake Copilot session "copilot truth" finishes its turn
    Then the state database session "copilot truth" has hook status "idle" with reason "end_turn"

    When fake Copilot session "copilot truth" exits
    Then the state database session "copilot truth" has hook status "stopped" with reason "user_exit"

    When deck client "A" exits cleanly

  Scenario: An aborted Copilot turn is demoted from running to idle by the probe
    # Copilot fires no hook when a turn is aborted with Ctrl-C (R219c, R224b), so
    # the hook-derived "running" can only be corrected by reading the pane.
    Given the deck config probes agent panes quickly
    And a long-running fake "copilot" binary rendering its screen is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates copilot session "aborted turn" with permission profile "safe"
    And fake Copilot session "aborted turn" starts working on "a very long job"
    Then the state database session "aborted turn" has hook status "running" with reason "new"

    When fake Copilot session "aborted turn" is interrupted with Ctrl-C
    Then the state database session "aborted turn" has probe status "idle" with reason "ready"
    And session "aborted turn" has no "stop" event
    And within 3 seconds deck client "A" row "aborted turn" contains "sampled"

    When fake Copilot session "aborted turn" exits
    And deck client "A" exits cleanly
