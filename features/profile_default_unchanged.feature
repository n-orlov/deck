@requirement-156-default-install-unchanged
Feature: Profiles: plain `deck` on an existing default install is unchanged (SPEC §3.4)
  Named profiles (SPEC §3.4) must never disturb the default profile's own
  pre-existing, flat installation: plain `deck`, with no positional
  argument and no DECK_PROFILE, still resolves $XDG_CONFIG_HOME/deck's
  config.toml, $XDG_DATA_HOME/deck's state.db and $XDG_STATE_HOME/deck's
  log directory -- a REAL installation's own $HOME/XDG_* resolution, not
  this package's usual DECK_HOME shortcut -- and never creates a profiles/
  directory anywhere under those three roots.

  Scenario: plain deck resumes an existing default install on its own flat paths, unchanged
    Given a default install already exists at a temp home, with config.toml marked "install-marker-9f2c17" and deck client "A" having already created shell session "install-prior"
    When deck client "A" is started again on that same default install with no argument and no DECK_PROFILE
    Then deck client "A" screen contains "install-prior"
    And the default install header for deck client "A" reads exactly "socket: deck"
    When deck client "A" creates shell session "install-second"
    Then deck client "A" screen contains "install-second"
    And the default install session "install-second" is a live tmux session on socket "deck"
    When deck client "A" exits cleanly
    Then the default install's config.toml still contains the marker "install-marker-9f2c17"
    And the default install's flat state database holds exactly the sessions "install-prior" and "install-second"
    And the default install's flat log directory holds a deck.jsonl file
    And no profiles directory exists under the default install's temp config, data or state roots
