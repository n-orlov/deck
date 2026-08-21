@themes
Feature: theme rendering end-to-end (requirement 49)
  SPEC §11.6 declares a fixed set of theme tokens (the seven §7 status
  tokens plus the chrome/list/footer tokens task 021 wired) and the
  discovery/fallback/live-preview contract task 024's first-paint notice
  and task 025's `t` picker implement. This file is requirement 49's own
  end-to-end proof, through the released deck binary (never a bare
  emulator or an internal/tui unit test in isolation, both already
  covered elsewhere): a built-in theme applied and read back per cell
  across every one of those tokens; a user theme discovered from the
  scenario's own themes directory (the same on-disk layout as
  $XDG_CONFIG_HOME/deck/themes -- see internal/theme/loader.go's
  ThemesDir); an unknown name falling back AND saying so on first paint;
  `t` previewing a theme live on the real list and reverting byte-for-byte
  (by colour, since Frame's own cell-grid text carries none) on esc;
  DECK_COLOR_DEPTH=16's quantised palette; NO_COLOR's monochrome,
  glyph-only status; and the frame's geometry staying identical across
  every built-in.

  @requirement-49-themes
  Scenario Outline: a built-in theme colours each of the seven §7 status tokens, read per cell from a real client
    Given the scenario's config.toml selects theme "empire"
    And a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "<name>" is started with colour enabled
    And deck client "<name>" creates shell session "anchor"
    Then within one configured reconcile interval deck client "<name>" screen contains "running"
    And deck client "<name>" creates shell session "target"
    And deck client "<name>" creates claude session "agent" with permission profile "safe"
    And deck client "<name>" selects session "anchor"
    When the state database session "<subject>" has status "<status>" 5 seconds ago
    Then within one configured reconcile interval deck client "<name>" screen contains "<status>"
    And deck client "<name>" text "<status>" has foreground token "<status>"
    And deck client "<name>" exits cleanly

    Examples:
      | name         | subject | status   |
      | tok49-wait   | target  | waiting  |
      | tok49-run    | target  | running  |
      | tok49-idle   | target  | idle     |
      | tok49-start  | agent   | starting |
      | tok49-stop   | target  | stopped  |
      | tok49-err    | target  | error    |
      | tok49-arch   | target  | archived |

  @requirement-49-themes
  Scenario: a built-in theme colours border_focus/border, selection/selection_idle, title, group and key/hint chrome, read per cell from a real client
    Given the scenario's config.toml selects theme "empire"
    And deck client "chrome" is started with colour enabled
    Then deck client "chrome" text "deck" has foreground token "title"
    And deck client "chrome" cell at row 0 column 0 has foreground token "border_focus"
    And deck client "chrome" cell at row 0 column 99 has foreground token "border"
    And deck client "chrome" text "Enter" has foreground token "key"
    And deck client "chrome" text "attach" has foreground token "hint"
    When deck client "chrome" creates shell session "grp-one"
    And deck client "chrome" creates shell session "grp-two"
    And the state database session "grp-two" has workspace "req49-beta-workspace"
    Then within one configured reconcile interval deck client "chrome" screen contains "req49-beta-workspace"
    And deck client "chrome" text "req49-beta-workspace" has foreground token "group"
    And deck client "chrome" text "grp-one" has background token "selection"
    When deck client "chrome" sends ","
    Then deck client "chrome" text "General" has background token "selection"
    And deck client "chrome" text "Allow Yolo" has background token "selection_idle"
    And deck client "chrome" cell at row 0 column 0 has foreground token "border_focus"
    And deck client "chrome" cell at row 0 column 99 has foreground token "border"
    When deck client "chrome" sends ""
    Then deck client "chrome" screen contains "deck - sessions"
    And deck client "chrome" exits cleanly

  @requirement-49-themes
  Scenario: a user theme placed in the scenario's own themes directory (the $XDG_CONFIG_HOME/deck/themes layout) is discovered and applied on a real client
    Given the scenario writes user theme file "req49-user.toml" into its themes directory with:
      """
      name = "req49-user"
      appearance = "dark"

      [colors]
      background        = "#000000"
      surface           = "#000000"
      border            = "#000000"
      border_focus      = "#000000"
      selection         = "#000000"
      selection_idle    = "#000000"
      title             = "#ff00ff"
      text              = "#111111"
      dimmed            = "#000000"
      hint              = "#000000"
      key               = "#000000"
      accent            = "#000000"
      group             = "#000000"
      search_match      = "#000000"
      badge             = "#000000"
      badge_warn        = "#000000"
      waiting           = "#000000"
      running           = "#000000"
      idle              = "#000000"
      starting          = "#000000"
      stopped           = "#000000"
      error             = "#000000"
      archived          = "#000000"
      """
    And the scenario's config.toml selects theme "req49-user"
    And deck client "user-theme" is started with colour enabled
    Then deck client "user-theme" text "deck" has foreground "#ff00ff"
    And deck client "user-theme" exits cleanly

  @requirement-49-themes
  Scenario: an unknown theme name falls back to the default AND says so on the very first painted frame
    Given the scenario's config.toml selects theme "does-not-exist-49"
    And deck client "fallback" is started with colour enabled
    Then deck client "fallback" screen contains "does-not-exist-49"
    And deck client "fallback" screen contains "not found; using default theme"
    And deck client "fallback" text "deck" has foreground token "title"
    And deck client "fallback" exits cleanly

  @requirement-49-themes
  Scenario: `t` previews a theme live on the real list and reverts to the exact original colour on esc
    Given deck client "picker" is started with colour enabled
    Then deck client "picker" text "deck" has foreground token "title"
    When deck client "picker" sends "t"
    Then deck client "picker" screen contains "Theme picker:"
    When deck client "picker" sends " "
    Then deck client "picker" text "deck" does not have foreground token "title"
    When deck client "picker" sends ""
    Then deck client "picker" text "deck" has foreground token "title"
    And deck client "picker" screen contains "deck - sessions"
    And deck client "picker" exits cleanly

  @requirement-49-themes
  Scenario: DECK_COLOR_DEPTH=16 renders the quantised reference-palette colour for a real client
    Given deck client "depth16" is started with colour enabled and colour depth "16"
    Then deck client "depth16" text "deck" has the quantised ANSI colour for token "title"
    And deck client "depth16" exits cleanly

  @requirement-49-themes
  Scenario: NO_COLOR renders every theme monochrome, with a session's status carried by its glyph alone
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "mono49" is started
    When deck client "mono49" creates claude session "mono49 session" with permission profile "safe"
    And fake Claude session "mono49 session" fires "SessionStart" for itself using injected identity:
      | source | fresh |
    And fake Claude session "mono49 session" fires "Notification" for itself using injected identity:
      | notification_type | permission_prompt |
    Then within one configured reconcile interval deck client "mono49" row "mono49 session" contains "waiting"
    And deck client "mono49" frame has no colour anywhere
    And deck client "mono49" exits cleanly

  @requirement-49-themes
  Scenario: the frame's geometry is identical across every built-in theme -- only colour differs
    Given the scenario's config.toml selects theme "daylight"
    And deck client "geom-daylight" is started with colour enabled
    Then deck client "geom-daylight" text "deck" has foreground token "title"
    When the scenario's config.toml selects theme "empire"
    And deck client "geom-empire" is started with colour enabled
    Then deck client "geom-empire" text "deck" has foreground token "title"
    And deck client "geom-daylight" screen text matches deck client "geom-empire" screen text
    When deck client "geom-daylight" exits cleanly
    And deck client "geom-empire" exits cleanly
