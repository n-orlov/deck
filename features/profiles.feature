Feature: Profiles: independent decks side by side (SPEC §3.4)
  A named profile is a whole, independent deck -- its own config, state, log
  and tmux socket -- created lazily, behind a typo guard, the first time a
  not-yet-existing but validly named profile is launched.

  @requirement-153-lazy-profile-creation
  Scenario: launching an unknown profile name, answered "y" at the terminal prompt, creates it and brings up its own socket
    When deck client "newprofile" is launched for the not-yet-existing profile "acme"
    Then deck client "newprofile" screen shows the creation prompt for profile "acme"
    When deck client "newprofile" answers the creation prompt with "y"
    Then deck client "newprofile" screen contains "profile: acme"
    When deck client "newprofile" creates shell session "acme-first"
    Then deck client "newprofile" screen contains "acme-first"
    And the profile "acme" session "acme-first" is a live tmux session on socket "deck-acme"
    And the profile "acme" data and log directories exist under the scenario's data root
    And deck client "newprofile" exits cleanly
