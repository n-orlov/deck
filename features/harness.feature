Feature: Godog harness wiring
  The repository's normal Go test command discovers default feature files.

  Scenario: default feature suite is wired
    Given the Godog harness is available

  Scenario: a private tmux server can be removed
    Given the private tmux server is killed

  @requirement-29-fingerprint-harness
  Scenario: a directory's fingerprint is byte-for-byte and stat-for-stat identical to itself
    Given a scratch directory "cwd" is seeded with:
      | path           | kind | content              | mode |
      | state.db       | file | not a real database  |      |
      | .hidden        | file | dotfile content       |      |
      | sub            | dir  |                       |      |
      | sub/nested.txt | file | nested content        |      |
      | readonly.txt   | file | read-only content     | 0444 |
    And the directory "cwd" is fingerprinted as "before"
    Then the directory "cwd" still matches fingerprint "before"

  @requirement-1-cell-attributes
  Scenario: the emulator reads foreground, background, bold, dim and reverse from raw SGR bytes
    Given a fresh terminal emulator sized 20x1
    When the emulator receives "[1;31mA[0m[42mB[0m[2mC[0m[7mD[0mE"
    Then the emulator cell at column 0 has foreground "#800000"
    And the emulator cell at column 0 is bold
    And the emulator cell at column 1 has background "#008000"
    And the emulator cell at column 2 is dim
    And the emulator cell at column 3 is reverse

  @requirement-1-cell-attributes
  Scenario: a colour assertion fails when pointed at a cell of a different colour
    Given a fresh terminal emulator sized 20x1
    When the emulator receives "[31mA[0mB"
    Then the emulator cell at column 0 does not have foreground "#000000"
    And the emulator cell at column 1 does not have foreground "#800000"

  @requirement-1-cell-attributes
  Scenario: a real deck client's own coloured chrome is readable per cell, by name and by matched text
    Given deck client "coloured" is started with colour enabled
    Then deck client "coloured" text "deck" has foreground "#fbbf24"
    And deck client "coloured" text "No sessions yet" does not have foreground "#fbbf24"
    And deck client "coloured" cell at row 0 column 2 has foreground "#fbbf24"
    And deck client "coloured" exits cleanly

  @requirement-1-cell-attributes
  Scenario: a real deck client's coloured chrome is readable by naming a theme token, resolved through the scenario's pinned theme
    Given the scenario writes user theme file "matches-chrome.toml" into its themes directory with:
      """
      name = "matches-chrome"
      appearance = "dark"

      [colors]
      background        = "#000000"
      surface           = "#000000"
      border            = "#000000"
      border_focus      = "#000000"
      selection         = "#000000"
      selection_idle    = "#000000"
      title             = "#008080"
      text              = "#111111"
      dimmed            = "#000000"
      hint              = "#000000"
      key               = "#000000"
      accent            = "#008080"
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
    And the scenario's config.toml selects theme "matches-chrome"
    And deck client "themed" is started with colour enabled
    Then deck client "themed" text "deck" has foreground token "title"
    And deck client "themed" cell at row 0 column 2 has foreground token "title"
    And deck client "themed" text "deck" does not have foreground token "text"
    And deck client "themed" text "No sessions yet" does not have foreground token "title"
    And deck client "themed" exits cleanly

  @requirement-6-eaw-placement
  Scenario: the screen emulator places an East-Asian-Wide cell without splitting it
    Given a fresh terminal emulator sized 12x4
    When the emulator receives "界┌"
    Then the emulator cell at column 0 has width 2 and content "界"
    And the emulator cell at column 1 is a continuation cell
    And the emulator cell at column 2 has content "┌"

  @requirement-1-resize
  Scenario: a mid-scenario terminal resize changes the grid the frame is read from
    Given deck client "solo" is started with terminal size 100x30
    And deck client "solo" frame width is 100
    And deck client "solo" frame height is 30
    When deck client "solo" terminal is resized to 60x20
    Then deck client "solo" frame width is 60
    And deck client "solo" frame height is 20
    And deck client "solo" exits cleanly

  @requirement-6-frame-budget
  Scenario Outline: a real deck client's frame fits its own terminal at several sizes
    Given deck client "budget-<cols>x<rows>" is started with terminal size <cols>x<rows>
    Then deck client "budget-<cols>x<rows>" frame fits within <cols> columns and <rows> rows
    And deck client "budget-<cols>x<rows>" exits cleanly

    Examples:
      | cols | rows |
      | 100  | 30   |
      | 80   | 24   |
      | 60   | 20   |

  @requirement-6-frame-budget
  Scenario: the frame-budget check is discriminating, not a check that always passes
    Given deck client "tight" is started with terminal size 100x30
    Then deck client "tight" frame does not fit within 5 columns and 1 row
    And deck client "tight" exits cleanly

  @requirement-6-frame-budget
  Scenario: a bare emulator's overflow by height or by width fails the frame-budget check
    Given a fresh terminal emulator sized 20x10
    When the emulator receives "[4;1Hfour"
    Then the emulator frame does not fit within 20 columns and 2 rows
    And the emulator frame fits within 20 columns and 4 rows

  @requirement-37-message-budget
  Scenario Outline: an attachError or resumeNote message never pushes the frame past the terminal's own budget
    # Task 030: this outline's own subject is the attachError message's budget,
    # not the create modal's -- but it reaches that message by first creating
    # a claude session through the real create modal, and framedDialog's box
    # is now a fixed 80% of the viewport (clamped [26,80]) rather than
    # growing to fit content, so at 80 columns the create modal's own
    # field/help lines wrap (never truncate) onto more physical lines than
    # an 80x24 terminal has rows for -- a pre-existing gap SPEC.md:1282
    # already anticipates ("inside a dialog the wheel scrolls a scrollable
    # body") but that scrolling is not implemented yet, so a content-heavy
    # dialog simply needs a taller terminal today. 30 rows (this file's other
    # examples' height) gives the create modal room to render in full while
    # still exercising the 80-column width clamp (dialogWidth resolves to 64)
    # this outline's second example exists to cover.
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "budgeted" is started with terminal size <cols>x<rows>
    And deck client "budgeted" creates claude session "budget target" with permission profile "safe"
    Then deck client "budgeted" screen contains "resumable"
    When the state database session "budget target"'s conversation id is cleared
    And deck client "budgeted" presses r on session "budget target"
    Then deck client "budgeted" screen contains "Cannot resume: resume session"
    And deck client "budgeted" frame fits within <cols> columns and <rows> rows
    And deck client "budgeted" exits cleanly

    Examples:
      | cols | rows |
      | 100  | 30   |
      | 80   | 30   |

  @requirement-37-message-budget
  Scenario: a resumeNote message ('starting elsewhere') never pushes the frame past the terminal's own budget
    Given deck client "budgeted-note" is started with terminal size 100x30
    And deck client "budgeted-note" creates shell session "budget note target"
    And deck client "budgeted-note" kills its selected session
    Then the state database contains session "budget note target" with status "stopped"
    When the state database session "budget note target" has a launch lease held by a live process
    And deck client "budgeted-note" presses r on session "budget note target"
    Then deck client "budgeted-note" screen contains "starting elsewhere"
    And deck client "budgeted-note" frame fits within 100 columns and 30 rows
    And deck client "budgeted-note" exits cleanly

  @requirement-2-mouse-synthesis
  Scenario: SGR mouse reports are synthesized for click, double-click, wheel and drag
    Given deck client "solo" is started
    And deck client "solo" captures its frame as "before-mouse"
    When deck client "solo" clicks at column 5 row 3
    And deck client "solo" double-clicks at column 5 row 3
    And deck client "solo" scrolls the wheel up at column 5 row 3
    And deck client "solo" scrolls the wheel down at column 5 row 3
    And deck client "solo" drags from column 5 row 3 to column 50 row 10
    And deck client "solo" clicks at column 224 row 3
    Then deck client "solo" frame still matches the captured "before-mouse" frame
    And deck client "solo" exits cleanly

  @requirement-3-deck-mouse
  Scenario: DECK_MOUSE overrides the default mouse-reporting setting
    Given deck client "default-mouse" is started
    Then deck client "default-mouse" raw output enabled SGR mouse reporting
    And deck client "default-mouse" exits cleanly

  @requirement-3-deck-mouse
  Scenario: DECK_MOUSE=0 disables mouse reporting
    Given the deck config disables mouse reporting
    And deck client "no-mouse" is started
    Then deck client "no-mouse" raw output did not enable SGR mouse reporting
    And deck client "no-mouse" exits cleanly

  @requirement-4-fake-agent-sizes
  Scenario Outline: a fake agent records its initial size and every SIGWINCH-observed size
    Given a fake "<agent>" agent is started recording sizes at 40x12
    Then the fake "<agent>" agent recorded sizes are "40x12"
    When the fake "<agent>" agent terminal is resized to 60x20
    Then the fake "<agent>" agent recorded sizes are "40x12,60x20"
    And the fake "<agent>" agent is stopped

    Examples:
      | agent  |
      | claude |
      | pi     |

  @requirement-5-preview-fixtures
  Scenario Outline: a fake agent renders a preview fixture once and then falls silent
    Given a fake "<agent>" agent renders the "<fixture>" preview fixture and falls silent at 40x12
    Then the fake "<agent>" agent's rendered pane is byte-identical across two consecutive captures
    And the fake "<agent>" agent is stopped

    Examples:
      | agent  | fixture     |
      | claude | fitting.txt |
      | claude | oversized.txt |
      | claude | wide.txt    |
      | claude | coloured.txt |
      | pi     | fitting.txt |
      | pi     | oversized.txt |
      | pi     | wide.txt    |
      | pi     | coloured.txt |

  @requirement-7-sidebar-width
  Scenario: sidebar_width can be set and read back for a scenario
    Given deck client "solo" is started
    Then the scenario's persisted sidebar_width is unset
    When the scenario's sidebar_width is set to 50
    Then the scenario's persisted sidebar_width reads back as 50
    When the scenario's sidebar_width is set to 24
    Then the scenario's persisted sidebar_width reads back as 24
    And deck client "solo" exits cleanly

  @requirement-7-sidebar-width
  Scenario: a non-default sidebar_width actually widens deck's own rendered sidebar
    Given deck client "solo" is started
    And deck client "solo" creates a long-named shell session "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxWIDENOWyyyyyyyyyy"
    Then deck client "solo" screen does not contain "WIDENOW"
    When deck client "solo" presses ">" until "WIDENOW" is visible
    Then deck client "solo" screen contains "WIDENOW"
    And deck client "solo" exits cleanly

  @requirement-5-config-file
  Scenario: a config.toml written before start-up is asserted by its parsed content after the run
    Given the scenario's config.toml is written with:
      """
      allow_yolo = true
      stale_after = 90

      [ui]
      mouse = false

      [env]
      DECK_TEST_TOKEN = "abc123"
      """
    And deck client "configured" is started
    And deck client "configured" exits cleanly
    Then the scenario's config.toml parses with allow_yolo true
    And the scenario's config.toml parses with stale_after "1m30s"
    And the scenario's config.toml parses with mouse false
    And the scenario's config.toml parses with env "DECK_TEST_TOKEN" set to "abc123"

  @requirement-5-config-file
  Scenario: the config-file-content assertion fails on a mismatch
    Given the scenario's config.toml is written with:
      """
      allow_yolo = true
      stale_after = 90

      [ui]
      mouse = false

      [env]
      DECK_TEST_TOKEN = "abc123"
      """
    Then the scenario's config.toml does not parse with allow_yolo false
    And the scenario's config.toml does not parse with stale_after "45s"
    And the scenario's config.toml does not parse with mouse true
    And the scenario's config.toml does not parse with env "DECK_TEST_TOKEN" set to "wrong-value"
    And the scenario's config.toml does not parse with env "MISSING_KEY" set to "anything"

  @requirement-4-theme-pinning
  Scenario: a scenario pins a built-in theme by name via a config.toml it writes itself
    Given the scenario's config.toml selects theme "daylight"
    When the scenario's config-selected theme is painted onto a fresh swatch emulator sized 23x1 named "builtin-swatch"
    Then the "builtin-swatch" swatch emulator has foreground "#1e293b" for token "text"
    And the "builtin-swatch" swatch emulator has foreground "#166534" for token "running"

  @requirement-4-theme-pinning
  Scenario: a scenario discovers a user theme it writes into its own themes directory
    Given the scenario writes user theme file "custom.toml" into its themes directory with:
      """
      name = "custom"
      appearance = "dark"

      [colors]
      background        = "#000000"
      surface           = "#000000"
      border            = "#000000"
      border_focus      = "#000000"
      selection         = "#000000"
      selection_idle    = "#000000"
      title             = "#000000"
      text              = "#ff00ff"
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
    And the scenario's config.toml selects theme "custom"
    When the scenario's config-selected theme is painted onto a fresh swatch emulator sized 23x1 named "user-swatch"
    Then the "user-swatch" swatch emulator has foreground "#ff00ff" for token "text"

  @requirement-4-theme-pinning
  Scenario: an unknown theme name falls back to the default rather than to the user theme's colours
    Given the scenario writes user theme file "custom.toml" into its themes directory with:
      """
      name = "custom"
      appearance = "dark"

      [colors]
      background        = "#000000"
      surface           = "#000000"
      border            = "#000000"
      border_focus      = "#000000"
      selection         = "#000000"
      selection_idle    = "#000000"
      title             = "#000000"
      text              = "#ff00ff"
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
    And the scenario's config.toml selects theme "does-not-exist"
    When the scenario's config-selected theme is painted onto a fresh swatch emulator sized 23x1 named "fallback-swatch"
    Then the "fallback-swatch" swatch emulator does not have foreground "#ff00ff" for token "text"
    And the "fallback-swatch" swatch emulator has foreground "#cbd5e1" for token "text"

  @requirement-4-theme-pinning
  Scenario: a built-in and a user theme paint genuinely different colours at the same token, proving the pinning step is not a no-op
    Given the scenario's config.toml selects theme "empire"
    And the scenario writes user theme file "custom.toml" into its themes directory with:
      """
      name = "custom"
      appearance = "dark"

      [colors]
      background        = "#000000"
      surface           = "#000000"
      border            = "#000000"
      border_focus      = "#000000"
      selection         = "#000000"
      selection_idle    = "#000000"
      title             = "#000000"
      text              = "#ff00ff"
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
    When the scenario's config-selected theme is painted onto a fresh swatch emulator sized 23x1 named "builtin-only"
    Then the "builtin-only" swatch emulator has foreground "#cbd5e1" for token "text"
    And the "builtin-only" swatch emulator does not have foreground "#ff00ff" for token "text"
    When the scenario's config.toml selects theme "custom"
    And the scenario's config-selected theme is painted onto a fresh swatch emulator sized 23x1 named "user-only"
    Then the "user-only" swatch emulator has foreground "#ff00ff" for token "text"
    And the "user-only" swatch emulator does not have foreground "#cbd5e1" for token "text"

  @requirement-3-no-color @requirement-31-no-color-glyph
  Scenario: NO_COLOR renders a monochrome frame with a session's status carried by its glyphs alone
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "mono" is started
    When deck client "mono" creates claude session "mono session" with permission profile "safe"
    And fake Claude session "mono session" fires "SessionStart" for itself using injected identity:
      | source | fresh |
    And fake Claude session "mono session" fires "Notification" for itself using injected identity:
      | notification_type | permission_prompt |
    Then within one configured reconcile interval deck client "mono" row "mono session" contains "waiting"
    And deck client "mono" frame has no colour anywhere
    And deck client "mono" exits cleanly

  @requirement-2-color-depth-truecolor
  Scenario: DECK_COLOR_DEPTH=truecolor forces the truecolour render path from a real pty
    Given deck client "truecolor" is started with colour enabled and colour depth "truecolor"
    Then deck client "truecolor" text "deck" has foreground token "title"
    And deck client "truecolor" exits cleanly

  @requirement-29-color-depth-16
  Scenario: DECK_COLOR_DEPTH=16 renders the quantised reference-palette colour from a real pty
    Given deck client "quantised" is started with colour enabled and colour depth "16"
    Then deck client "quantised" text "deck" has the quantised ANSI colour for token "title"
    And deck client "quantised" exits cleanly
