Feature: Interactive preview draws the pane's cursor (R182, GH #60)

  The vt grid's own Render() never paints a cursor, so an interactive
  preview used to give no sign of where typed text would land. The preview
  now draws the grid's cursor cell in reverse video, taken from the same
  snapshot as the rows it overlays, and draws none while the pane has hidden
  its cursor (DECTCEM) or while the view is scrolled back into history.
  Every assertion reads real cells off the deck client's rendered grid
  against a real tmux pane. The first scenario starts bash in the pane
  because the fixture's login shell may be a line-editor-less sh, where the
  left arrow would not move the cursor at all.

  @requirement-182-cursor
  Scenario: the cursor cell is reverse video and the cells around it are not
    Given deck client "A" is started
    When deck client "A" creates shell session "cursor-target"
    Then deck client "A" screen contains "cursor-target"
    And deck client "A" selects session "cursor-target"
    When deck client "A" enters interactive mode
    And deck client "A" types "exec bash --norc" and Enter into the interactive pane
    Then deck client "A" screen contains "bash-"
    When deck client "A" types "abc" without Enter into the interactive pane
    Then deck client "A" shows the pane cursor right after "abc"
    When deck client "A" presses left 2 times in the interactive pane
    Then deck client "A" shows the pane cursor on the cell holding "b" in "abc"
    And deck client "A" shows no pane cursor on the cell holding "a" in "abc"
    And deck client "A" shows no pane cursor on the cell holding "c" in "abc"
    When deck client "A" leaves interactive mode
    Then deck client "A" exits cleanly

  @requirement-182-cursor
  Scenario: a pane that hides its cursor shows none, and shows it again on request
    Given deck client "A" is started
    When deck client "A" creates shell session "cursor-hide"
    Then deck client "A" screen contains "cursor-hide"
    And deck client "A" selects session "cursor-hide"
    When deck client "A" enters interactive mode
    And deck client "A" types "SHOWN" without Enter into the interactive pane
    Then deck client "A" shows the pane cursor right after "SHOWN"
    When deck client "A" clears the interactive pane's input line
    And deck client "A" types "printf '\e[?25l'" and Enter into the interactive pane
    And deck client "A" types "HIDDEN" without Enter into the interactive pane
    Then deck client "A" screen contains "HIDDEN"
    And deck client "A" shows no pane cursor right after "HIDDEN"
    When deck client "A" clears the interactive pane's input line
    And deck client "A" types "printf '\e[?25h'" and Enter into the interactive pane
    And deck client "A" types "BACK" without Enter into the interactive pane
    Then deck client "A" shows the pane cursor right after "BACK"
    When deck client "A" leaves interactive mode
    Then deck client "A" exits cleanly

  # The cursor is homed to the top row first. A cursor on the pane's bottom
  # row is pushed out of the view by ANY scroll, which would make "no cursor
  # while scrolled back" true of a build that never checks the offset; at
  # row 0 a one-notch scroll leaves the stale cursor row inside the view.
  @requirement-182-cursor
  Scenario: no cursor is drawn while scrolled back, and it returns at the live view
    Given deck client "A" is started
    When deck client "A" creates shell session "cursor-scroll"
    Then deck client "A" screen contains "cursor-scroll"
    And deck client "A" selects session "cursor-scroll"
    When deck client "A" enters interactive mode
    And deck client "A" types a 60-line numbered loop labelled "CURSOR_SCROLL_LINE" into the interactive pane
    Then deck client "A" screen contains "CURSOR_SCROLL_LINE_60"
    When deck client "A" types "printf '\e[H'" and Enter into the interactive pane
    And deck client "A" types "LIVE" without Enter into the interactive pane
    Then deck client "A" shows the pane cursor right after "LIVE"
    When deck client "A" scrolls the interactive wheel up 1 time over the line containing "LIVE"
    Then deck client "A" screen contains "LIVE"
    And deck client "A" shows no reverse-video cell in the preview panel
    When deck client "A" scrolls the interactive wheel down 1 time over the line containing "LIVE"
    Then deck client "A" shows the pane cursor right after "LIVE"
    When deck client "A" leaves interactive mode
    Then deck client "A" exits cleanly

  @requirement-182-cursor
  Scenario Outline: a reverse cell the program painted survives preview_paint <mode>
    Given deck client "A" is started with colour enabled and preview paint "<mode>"
    When deck client "A" creates shell session "cursor-paint"
    Then deck client "A" screen contains "cursor-paint"
    And deck client "A" selects session "cursor-paint"
    When deck client "A" enters interactive mode
    And deck client "A" types "printf '\e[7m%s%s\e[27m\n' RV SX" and Enter into the interactive pane
    Then deck client "A" screen contains "RVSX"
    And deck client "A" text "RVSX" is reverse
    When deck client "A" leaves interactive mode
    Then deck client "A" exits cleanly

    Examples:
      | mode  |
      | fit   |
      | nofit |
      | bg    |
      | off   |
