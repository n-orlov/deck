Feature: Interactive mode's focus indicator is unmistakable, including under NO_COLOR (Part II, requirement 44)

  Entering interactive mode (task 061) moves the main view's one unit of
  keyboard focus off the sidebar and onto the preview panel for the first
  time -- every keystroke forwards to the live pane instead of driving list
  navigation. That move must be unmistakable through colour (the focused
  surface's border in border_focus, the unfocused one in plain border, the
  sidebar's still-selected row in the existing selection_idle token) AND
  through plain text once colour is unavailable, since NO_COLOR drops deck
  to monochrome and a colour-only cue would pass a golden-frame diff
  whether or not focus actually moved.

  @requirement-44-interactive-focus
  Scenario: entering interactive mode swaps the two panels' border tokens and the sidebar's selected row to selection_idle
    Given the scenario's config.toml selects theme "empire"
    And deck client "focus" is started with colour enabled
    And deck client "focus" creates shell session "focus-target"
    Then within one configured reconcile interval deck client "focus" screen contains "running"
    And deck client "focus" selects session "focus-target"
    And deck client "focus" cell at row 0 column 0 has foreground token "border_focus"
    And deck client "focus" cell at row 0 column 99 has foreground token "border"
    And deck client "focus" text "focus-target" has background token "selection"
    When deck client "focus" enters interactive mode
    Then deck client "focus" screen contains "Ctrl+Q"
    And deck client "focus" cell at row 0 column 0 has foreground token "border"
    And deck client "focus" cell at row 0 column 99 has foreground token "border_focus"
    And deck client "focus" text "> focus-target" has background token "selection_idle"
    When deck client "focus" leaves interactive mode
    Then deck client "focus" screen contains "deck - sessions"
    And deck client "focus" cell at row 0 column 0 has foreground token "border_focus"
    And deck client "focus" cell at row 0 column 99 has foreground token "border"
    And deck client "focus" text "focus-target" has background token "selection"
    And deck client "focus" exits cleanly

  @requirement-44-interactive-focus
  Scenario: the preview's top border names the target session as plain screen text, legible even under NO_COLOR
    Given deck client "plain" is started
    And deck client "plain" creates shell session "plain-target"
    Then within one configured reconcile interval deck client "plain" screen contains "running"
    And deck client "plain" selects session "plain-target"
    When deck client "plain" enters interactive mode
    Then deck client "plain" screen contains "plain-target"
    And deck client "plain" screen contains "interactive"
    And deck client "plain" screen contains "Ctrl+Q"
    When deck client "plain" leaves interactive mode
    Then deck client "plain" screen contains "deck - sessions"
    And deck client "plain" exits cleanly
