Feature: Launch-lease stale/live breaking
  The SPEC §9.3 launch lease must block a second launch only while a live
  owner still holds it within its TTL, and must never leave a row wedged: a
  lease from a dead process or one whose TTL has elapsed is breakable, and
  the row remains usable by a plain `r` afterwards in every case.

  Scenario: a live in-TTL lease blocks a second launch and the row stays usable
    Given deck client "A" is started
    And deck client "A" creates shell session "live lease target"
    And deck client "A" kills its selected session
    Then the state database contains session "live lease target" with status "stopped"
    When the state database session "live lease target" has a launch lease held by a live process
    And deck client "A" presses r on session "live lease target"
    Then deck client "A" screen contains "starting elsewhere"
    And the state database contains session "live lease target" with status "stopped"
    When the state database session "live lease target"'s launch lease is cleared
    And deck client "A" presses r on session "live lease target"
    Then deck client "A" screen contains "starting - awaiting signal"
    And deck client "A" screen does not contain "starting elsewhere"
    When deck client "A" exits cleanly

  Scenario: a lease from a dead process is breakable and the row stays usable
    Given deck client "A" is started
    And deck client "A" creates shell session "dead lease target"
    And deck client "A" kills its selected session
    Then the state database contains session "dead lease target" with status "stopped"
    When the state database session "dead lease target" has a launch lease owned by a dead process
    And deck client "A" presses r on session "dead lease target"
    Then deck client "A" screen contains "starting - awaiting signal"
    And deck client "A" screen does not contain "starting elsewhere"
    When deck client "A" exits cleanly

  Scenario: an expired-TTL lease is breakable and the row stays usable
    Given deck client "A" is started
    And deck client "A" creates shell session "expired lease target"
    And deck client "A" kills its selected session
    Then the state database contains session "expired lease target" with status "stopped"
    When the state database session "expired lease target" has an expired launch lease
    And deck client "A" presses r on session "expired lease target"
    Then deck client "A" screen contains "starting - awaiting signal"
    And deck client "A" screen does not contain "starting elsewhere"
    When deck client "A" exits cleanly
