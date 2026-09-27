@requirement-154-hook-profile-isolation
Feature: Profiles: a hook's write stays walled to the pane's own profile (SPEC §3.4)
  "Every pane carries DECK_PROFILE ... so a pane writes to the database of the
  profile that created it for its whole life" and "a profile never reads
  another profile's state.db: no cross-profile counts, no aggregate view."
  This proves both halves at once with two profiles' decks running side by
  side, each on its own private tmux socket, in one scenario's data root: a
  hook fired from inside a pane profile A's own deck created changes only
  A's own state.db, while profile B's state.db -- and B's still-running
  deck -- are left completely untouched.

  Scenario: a hook fired from a pane created by profile A writes only A's own database while profile B's deck keeps running
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "a" is started for the existing profile "acme" on the scenario's private socket
    And deck client "b" is started for the existing profile "beta" on its own private socket
    When deck client "a" creates claude session "a-agent" with permission profile "safe"
    Then the profile "acme" session "a-agent"'s pane carries DECK_PROFILE "acme" and its own session id
    When deck client "b" creates shell session "b-shell"
    Then the profile "beta" state database session "b-shell" settles at status "running"
    And the profile "beta" state database is snapshotted as "before"
    When fake Claude session "a-agent" in profile "acme" fires "Notification" from its own pane:
      | notification_type | permission_prompt |
    Then the profile "acme" state database session "a-agent" has hook status "waiting" with reason "permission_prompt" and one "notification" event
    And within one configured reconcile interval deck client "a" screen contains "waiting"
    And deck client "b" screen contains "deck - sessions"
    And deck client "b" screen contains "b-shell"
    And the profile "beta" state database still matches snapshot "before" in content and session row count
    And the scenario's data root holds no default-profile state database
    When deck client "b" exits cleanly
    And deck client "a" exits cleanly
