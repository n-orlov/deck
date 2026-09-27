@requirement-154-hook-profile-isolation
Feature: Profiles: a hook's write stays walled to the pane's own profile (SPEC §3.4)
  "Every pane carries DECK_PROFILE ... so a pane writes to the database of the
  profile that created it for its whole life" and "a profile never reads
  another profile's state.db: no cross-profile counts, no aggregate view."
  This proves both halves at once with two profiles' decks running side by
  side in one scenario: a hook fired from a pane profile A created changes
  only A's own state.db, while profile B's state.db -- and B's still-running
  deck -- are left completely untouched.

  Scenario: a hook fired from a pane created by profile A writes only A's own database while profile B's deck keeps running
    Given deck client "a" is launched for the not-yet-existing profile "acme"
    And deck client "a" screen shows the creation prompt for profile "acme"
    When deck client "a" answers the creation prompt with "y"
    Then deck client "a" screen contains "socket: deck-acme"
    When deck client "a" exits cleanly
    And an uncontended Claude hook target "a-target" exists in profile "acme"'s state database
    Given deck client "b" is started for the existing profile "beta"
    Then deck client "b" screen contains "deck - sessions"
    And the profile "beta" state database is snapshotted as "before"
    When the released hook receiver handles a "SessionEnd" event for "a-target" in profile "acme"
    Then the profile "acme" state database session "a-target" is stopped by session end
    And the profile "beta" state database still matches snapshot "before" in content and session row count
    And deck client "b" screen contains "deck - sessions"
    When deck client "b" exits cleanly
