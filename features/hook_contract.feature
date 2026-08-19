@hook-contract
Feature: Hook receiver write contract
  Hook status writes are measured around only the durable transaction, and the
  terminal session-end path does not begin any later work.

  Scenario: An uncontended hook store write stays below twenty milliseconds
    Given an uncontended Claude hook target "latency" exists in the scenario store
    When the released hook receiver handles a "Notification" event for "latency"
    Then exactly one hook store write is recorded
    And its operation-scoped store duration is below 20 milliseconds

  Scenario: Session end performs one write and no subsequent work
    Given an uncontended Claude hook target "ending" exists in the scenario store
    And an active liveness sentinel exists in the scenario store
    When the released hook receiver handles a "SessionEnd" event for "ending"
    Then the scenario store session "ending" is stopped by session end
    And the liveness sentinel remains untouched
    And exactly one hook store write is recorded
    And the hook audit records no liveness, probe, dispatch, or enqueue attempt
