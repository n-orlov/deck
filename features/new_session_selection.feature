@new-session-selection
Feature: A newly created session is selected as soon as it appears (requirement 52)
  SPEC.md §11's requirement, added by 6584299: creating a session selects its
  row the moment it appears, with no navigation keystroke needed to find it --
  and the selection this produces is a one-shot intent, not a standing "always
  select the newest" rule that would fight the user's own later navigation.

  Background:
    Given deck client "A" is started

  # The active order for both scenarios below is engineered so the freshly
  # created session never lands at index 0: an anchor session is forced to
  # "waiting" (attention rank 0) before the second session is created, and a
  # brand-new session always starts "starting" (attention rank 3, behind
  # waiting/error/running) -- so it must render at index 1, never index 0.
  # "deck client A screen shows sessions in this order" below asserts that
  # placement directly, ruling out a "selection happens to be at the top"
  # stand-in the way requirement 52's success criteria demands.
  @requirement-52-new-session-auto-selected
  Scenario: creating a new session selects its row immediately, without any navigation keystroke, even though it does not land at index 0
    When deck client "A" creates shell session "r52-anchor"
    And the state database session "r52-anchor" has status "waiting" 5 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "waiting"
    When deck client "A" creates shell session "r52-new"
    Then deck client "A" screen shows sessions in this order:
      | r52-anchor |
      | r52-new    |
    And deck client "A" has session "r52-new" selected
    When deck client "A" exits cleanly

  @requirement-52-one-shot-intent
  Scenario: the just-created row's auto-selection is one-shot -- moving away with k survives a later reconcile tick
    When deck client "A" creates shell session "r52-oneshot-anchor"
    And the state database session "r52-oneshot-anchor" has status "waiting" 5 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "waiting"
    When deck client "A" creates shell session "r52-oneshot-new"
    Then deck client "A" screen shows sessions in this order:
      | r52-oneshot-anchor |
      | r52-oneshot-new    |
    And deck client "A" has session "r52-oneshot-new" selected
    When deck client "A" sends "k"
    Then deck client "A" has session "r52-oneshot-anchor" selected
    And after one configured reconcile interval deck client "A" has session "r52-oneshot-anchor" selected
    When deck client "A" exits cleanly
