@requirement-156-two-profiles-side-by-side
Feature: Profiles: two named profiles run side by side, each on its own socket and database (SPEC §3.4)
  Two named profiles are two whole, independent decks: each derives its own
  tmux socket ("deck-<name>") and opens only its own state.db. Nothing about
  one profile's session ever reaches the other's screen or database, even
  when both profiles happen to give their session the exact same name.

  Scenario: profiles "a" and "b" each create a session named "shared" at the same time, and stay fully apart
    Given deck client "a" is started for the existing profile "a" on its own derived socket
    And deck client "b" is started for the existing profile "b" on its own derived socket
    When deck client "a" creates shell session "shared"
    And deck client "b" creates shell session "shared"
    Then deck client "a" screen contains "shared"
    And deck client "a" screen does not contain "already exists"
    And deck client "b" screen contains "shared"
    And deck client "b" screen does not contain "already exists"
    And deck client "a" screen contains "profile: a · socket: deck-a"
    And the profile "a" session "shared" is a live tmux session on socket "deck-a"
    And the profile "b" session "shared" is a live tmux session on socket "deck-b"
    And the profile "a" state database holds exactly one session row, named "shared"
    And the profile "b" state database holds exactly one session row, named "shared"
    When deck client "b" exits cleanly
    And deck client "a" exits cleanly
