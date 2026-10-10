Feature: a running client picks up another client's config.toml change (R226, R227)
  Two real deck clients share one temp DECK_HOME (the scenario root, which is
  where config.toml and the themes directory resolve when XDG_CONFIG_HOME is
  unset). Client A changes the theme in the settings takeover and saves;
  client B, which never saw a keystroke, repaints with it within one poll
  interval. The poll interval is shortened only through the test-only
  DECK_CONFIG_POLL_MS override of SPEC §13.1, which production never sets.

  @requirement-226-config-reload
  Scenario: a theme saved in client A's settings reaches client B within one shortened poll interval
    Given the scenario's config.toml selects theme "empire"
    And deck client "A" is started with colour enabled and a shortened config poll interval
    And deck client "B" is started with colour enabled and a shortened config poll interval
    Then deck client "B" text "deck" has foreground token "title"
    And deck client "B" text "deck" does not have foreground "#fabd2f"
    When deck client "A" sends ","
    And deck client "A" sends "j"
    And deck client "A" sends "	"
    Then deck client "A" screen contains "Theme: empire"
    # "+" from empire steps forward through the sorted built-in names to
    # gruvbox-dark, whose title token is #fabd2f.
    When deck client "A" sends "+"
    Then deck client "A" screen contains "Theme: gruvbox-dark"
    When deck client "A" sends ""
    Then deck client "A" screen contains "saved "
    And within one shortened config poll interval deck client "B" text "deck" has foreground "#fabd2f"
    When deck client "A" sends ""
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" text "deck" has foreground "#fabd2f"
    When deck client "A" exits cleanly
    And deck client "B" exits cleanly

  @requirement-237-theme-picker-reload
  Scenario: a theme chosen in client A's theme picker reaches client B of the default profile within one shortened poll interval
    Given the scenario's config.toml selects theme "empire"
    And deck client "A" is started with colour enabled and a shortened config poll interval
    And deck client "B" is started with colour enabled and a shortened config poll interval
    Then deck client "B" text "deck" has foreground token "title"
    And deck client "B" text "deck" does not have foreground "#fabd2f"
    When deck client "A" sends "t"
    Then deck client "A" screen contains "Theme picker: empire"
    # Space steps forward through the sorted built-in names from empire to
    # gruvbox-dark, whose title token is #fabd2f; Enter selects it.
    When deck client "A" sends " "
    Then deck client "A" screen contains "Theme picker: gruvbox-dark"
    When deck client "A" sends ""
    Then within one configured reconcile interval deck client "A" screen does not contain "Theme picker"
    And within one shortened config poll interval deck client "A" text "deck" has foreground "#fabd2f"
    And within one shortened config poll interval deck client "B" text "deck" has foreground "#fabd2f"
    When deck client "A" exits cleanly
    And deck client "B" exits cleanly

  @requirement-237-theme-picker-reload
  Scenario: a theme chosen in client A's theme picker reaches client B of one named profile within one shortened poll interval
    Given the scenario's config.toml selects theme "matrix"
    And the scenario's profile "work" config.toml selects theme "empire"
    And the scenario's profile "play" config.toml selects theme "amber"
    And deck client "A" is started on profile "work" with colour enabled and a shortened config poll interval
    And deck client "B" is started on profile "work" with colour enabled and a shortened config poll interval
    And deck client "C" is started on profile "play" with colour enabled and a shortened config poll interval
    Then deck client "B" text "deck" has foreground "#f5c518"
    And deck client "B" text "deck" does not have foreground "#fabd2f"
    And deck client "C" text "deck" has foreground "#ffd266"
    When deck client "A" sends "t"
    Then deck client "A" screen contains "Theme picker: empire"
    # Space steps forward through the sorted built-in names from empire to
    # gruvbox-dark, whose title token is #fabd2f; Enter selects it.
    When deck client "A" sends " "
    Then deck client "A" screen contains "Theme picker: gruvbox-dark"
    When deck client "A" sends ""
    Then within one configured reconcile interval deck client "A" screen does not contain "Theme picker"
    And within one shortened config poll interval deck client "A" text "deck" has foreground "#fabd2f"
    And within one shortened config poll interval deck client "B" text "deck" has foreground "#fabd2f"
    # Client C runs a different profile. Its own config.toml gets an unrelated
    # edit so its poll really reloads; it must read profile "play" (amber),
    # not profile "work" (the change just made) and not the default (matrix).
    When the scenario's profile "play" config.toml gets an unrelated edit
    Then deck client "C" text "deck" keeps foreground "#ffd266" for 2 further shortened config poll intervals
    When deck client "A" exits cleanly
    And deck client "B" exits cleanly
    And deck client "C" exits cleanly

  @requirement-237-settings-view-reload
  Scenario: a theme saved in client A's settings view reaches client B of one named profile within one shortened poll interval and never reaches a client of another profile
    Given the scenario's config.toml selects theme "matrix"
    And the scenario's profile "work" config.toml selects theme "empire"
    And the scenario's profile "play" config.toml selects theme "amber"
    And deck client "A" is started on profile "work" with colour enabled and a shortened config poll interval
    And deck client "B" is started on profile "work" with colour enabled and a shortened config poll interval
    And deck client "C" is started on profile "play" with colour enabled and a shortened config poll interval
    Then deck client "B" text "deck" has foreground "#f5c518"
    And deck client "B" text "deck" does not have foreground "#fabd2f"
    And deck client "C" text "deck" has foreground "#ffd266"
    When deck client "A" sends ","
    And deck client "A" sends "j"
    And deck client "A" sends "	"
    Then deck client "A" screen contains "Theme: empire"
    # "+" from empire steps forward through the sorted built-in names to
    # gruvbox-dark, whose title token is #fabd2f.
    When deck client "A" sends "+"
    Then deck client "A" screen contains "Theme: gruvbox-dark"
    When deck client "A" sends ""
    Then deck client "A" screen contains "saved "
    And within one shortened config poll interval deck client "B" text "deck" has foreground "#fabd2f"
    # Client C runs a different profile. Its own config.toml gets an unrelated
    # edit so its poll really reloads; it must read profile "play" (amber),
    # not profile "work" (the change just saved) and not the default (matrix).
    When the scenario's profile "play" config.toml gets an unrelated edit
    Then deck client "C" text "deck" keeps foreground "#ffd266" for 2 further shortened config poll intervals
    When deck client "A" sends ""
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" text "deck" has foreground "#fabd2f"
    When deck client "A" exits cleanly
    And deck client "B" exits cleanly
    And deck client "C" exits cleanly

  # The layout a real install uses: DECK_HOME unset, config.toml at
  # $XDG_CONFIG_HOME/deck/config.toml. Every other scenario here resolves
  # config through the DECK_HOME shortcut.
  @requirement-237-xdg-layout-reload
  Scenario: a theme chosen in client A's theme picker reaches client B on the real-install XDG layout within one shortened poll interval
    Given the scenario runs on the real-install XDG layout with DECK_HOME unset and its config.toml selects theme "empire"
    And deck client "A" is started with colour enabled and a shortened config poll interval
    And deck client "B" is started with colour enabled and a shortened config poll interval
    Then deck client "B" text "deck" has foreground token "title"
    And deck client "B" text "deck" does not have foreground "#fabd2f"
    When deck client "A" sends "t"
    Then deck client "A" screen contains "Theme picker: empire"
    When deck client "A" sends " "
    Then deck client "A" screen contains "Theme picker: gruvbox-dark"
    When deck client "A" sends ""
    Then within one configured reconcile interval deck client "A" screen does not contain "Theme picker"
    And within one shortened config poll interval deck client "A" text "deck" has foreground "#fabd2f"
    And within one shortened config poll interval deck client "B" text "deck" has foreground "#fabd2f"
    When deck client "A" exits cleanly
    And deck client "B" exits cleanly

  @requirement-237-xdg-layout-reload
  Scenario: a theme saved in client A's settings view reaches client B on the real-install XDG layout within one shortened poll interval
    Given the scenario runs on the real-install XDG layout with DECK_HOME unset and its config.toml selects theme "empire"
    And deck client "A" is started with colour enabled and a shortened config poll interval
    And deck client "B" is started with colour enabled and a shortened config poll interval
    Then deck client "B" text "deck" has foreground token "title"
    And deck client "B" text "deck" does not have foreground "#fabd2f"
    When deck client "A" sends ","
    And deck client "A" sends "j"
    And deck client "A" sends "	"
    Then deck client "A" screen contains "Theme: empire"
    When deck client "A" sends "+"
    Then deck client "A" screen contains "Theme: gruvbox-dark"
    When deck client "A" sends ""
    Then deck client "A" screen contains "saved "
    And within one shortened config poll interval deck client "B" text "deck" has foreground "#fabd2f"
    When deck client "A" sends ""
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" text "deck" has foreground "#fabd2f"
    When deck client "A" exits cleanly
    And deck client "B" exits cleanly
