@multiclient
Feature: Multi-client session refresh
  Independent deck clients sharing one home and private tmux socket converge on
  durable session changes without a restart.

  Scenario: create and kill propagate and surviving clients tolerate a peer crash
    Given deck client "A" is started
    And deck client "B" is started
    And deck client "C" is started
    When deck client "A" creates shell session "shared session"
    Then within one configured reconcile interval deck client "B" screen contains "shared session"
    And within one configured reconcile interval deck client "C" screen contains "shared session"
    When deck client "B" kills its selected session
    Then within one configured reconcile interval deck client "A" screen contains "resumable"
    And within one configured reconcile interval deck client "C" screen contains "resumable"
    And the state database contains session "shared session" with status "stopped"
    When deck client "C" is killed with SIGKILL
    And deck client "A" creates shell session "after crash"
    Then deck client "B" screen contains "after crash"
    And the state database contains session "shared session" with status "stopped"
    # The durable-row assertion protects creation after a peer SIGKILL; shell
    # promotion legitimately changes its transient status to running.
    And the state database contains session "after crash"
    When deck client "A" exits cleanly
    And deck client "B" exits cleanly

  Scenario: a live client reconciles an externally killed private server
    Given deck client "A" is started
    When deck client "A" creates shell session "externally stopped"
    And the private tmux server is killed
    Then within one configured reconcile interval deck client "A" screen contains "resumable"
    And the state database contains session "externally stopped" with status "stopped"
    And the audit log contains event "tmux.session_gone" for a session
    And the private tmux session "deck_externally-stopped" does not exist
    When deck client "A" sends "?"
    Then deck client "A" screen contains "Runtime controls"
    When deck client "A" exits cleanly
