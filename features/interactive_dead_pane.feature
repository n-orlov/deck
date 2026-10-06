Feature: A pane that dies while interactive is a dead pane, not a lost attach (R208, GH #70)

  Interactive mode's per-tick read answers two different questions: did
  another client take the window, and is the pane gone. A pane whose process
  exited (the target no longer resolves, or tmux reports it dead) used to
  fall into the takeover branch and raise the "Lost attach" dialog, which
  names the wrong cause. A dead pane is reported through the dead-pane path
  only; the dialog is reserved for a genuine takeover
  (interactive_force_attach.feature).

  Scenario: a shell that exits cleanly while interactive raises no lost-attach dialog
    Given deck client "A" is started
    When deck client "A" creates shell session "gone-clean"
    Then deck client "A" screen contains "gone-clean"
    And deck client "A" selects session "gone-clean"
    When deck client "A" enters interactive mode
    Then deck client "A" screen contains "Ctrl+Q"
    When shell session "gone-clean" exits with status zero
    Then the private tmux session "deck_gone-clean" does not exist
    When 800 milliseconds pass
    Then deck client "A" screen does not contain "Lost attach"
    And deck client "A" screen does not contain "took over"
    When deck client "A" leaves interactive mode
    Then deck client "A" screen contains "deck - sessions"
    When deck client "A" exits cleanly

  Scenario: a shell that crashes while interactive raises no lost-attach dialog
    Given deck client "A" is started
    When deck client "A" creates shell session "gone-crash"
    Then deck client "A" screen contains "gone-crash"
    And deck client "A" selects session "gone-crash"
    When deck client "A" enters interactive mode
    Then deck client "A" screen contains "Ctrl+Q"
    When shell session "gone-crash" exits with status 3
    Then the private tmux session "deck_gone-crash" does not exist
    When 800 milliseconds pass
    Then deck client "A" screen does not contain "Lost attach"
    And deck client "A" screen does not contain "took over"
    When deck client "A" leaves interactive mode
    Then deck client "A" screen contains "deck - sessions"
    When deck client "A" exits cleanly

  Scenario: an agent killed while interactive raises no lost-attach dialog
    Given a crash-tail fixture and long-running fake Claude are configured
    And deck client "A" is started
    When deck client "A" creates claude session "gone-agent" with permission profile "safe"
    And deck client "A" selects session "gone-agent"
    And deck client "A" enters interactive mode
    Then deck client "A" screen contains "Ctrl+Q"
    When the agent process "fake-claude-real" in private tmux session "deck_gone-agent" is killed with SIGKILL
    Then the private tmux session "deck_gone-agent" does not exist
    When 800 milliseconds pass
    Then deck client "A" screen does not contain "Lost attach"
    And deck client "A" screen does not contain "took over"
    When deck client "A" leaves interactive mode
    Then deck client "A" screen contains "deck - sessions"
    When deck client "A" exits cleanly
