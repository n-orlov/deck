Feature: Interactive mode's own bounded scrollback (Part II, requirement 51)

  The grid keeps its own bounded scrollback, and the wheel and
  Shift+PgUp/PgDn scroll it (docs/reports/phase3b.md's II-51 section,
  internal/interactive.ScrollbackMaxLines). This proves the scrolling
  itself against a real tmux pane, real deck client and real terminal
  input, not merely the internal RenderRows/offset arithmetic
  internal/interactive's own unit tests already cover: content genuinely
  scrolled off the live screen becomes visible again once scrolled back
  to, by both input methods, and disappears again once scrolled back down
  to the live view.

  @requirement-51-bounded-scrollback
  Scenario: Shift+PgUp/PgDn scroll the interactive grid's own scrollback
    Given deck client "A" is started
    When deck client "A" creates shell session "scroll-target"
    Then deck client "A" screen contains "scroll-target"
    And deck client "A" selects session "scroll-target"
    When deck client "A" enters interactive mode
    And deck client "A" types a 60-line numbered loop labelled "INTERACTIVE_SCROLL_LINE" into the interactive pane
    Then deck client "A" screen contains "INTERACTIVE_SCROLL_LINE_60"
    And deck client "A" screen does not contain "INTERACTIVE_SCROLL_LINE_1 "
    When deck client "A" sends shift+pgup 3 times
    Then deck client "A" screen contains "INTERACTIVE_SCROLL_LINE_1 "
    When deck client "A" sends shift+pgdown 3 times
    Then deck client "A" screen does not contain "INTERACTIVE_SCROLL_LINE_1 "
    And deck client "A" screen contains "INTERACTIVE_SCROLL_LINE_60"
    When deck client "A" leaves interactive mode
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" exits cleanly

  @requirement-51-bounded-scrollback
  Scenario: the mouse wheel over the preview panel scrolls the interactive grid's own scrollback
    Given deck client "A" is started
    When deck client "A" creates shell session "wheel-scroll-target"
    Then deck client "A" screen contains "wheel-scroll-target"
    And deck client "A" selects session "wheel-scroll-target"
    When deck client "A" enters interactive mode
    And deck client "A" types a 60-line numbered loop labelled "WHEEL_SCROLL_LINE" into the interactive pane
    Then deck client "A" screen contains "WHEEL_SCROLL_LINE_60"
    And deck client "A" screen does not contain "WHEEL_SCROLL_LINE_1 "
    When deck client "A" scrolls the interactive wheel up 20 times over the line containing "WHEEL_SCROLL_LINE_60"
    Then deck client "A" screen contains "WHEEL_SCROLL_LINE_1 "
    When deck client "A" scrolls the interactive wheel down 20 times over the line containing "WHEEL_SCROLL_LINE_1 "
    Then deck client "A" screen does not contain "WHEEL_SCROLL_LINE_1 "
    When deck client "A" leaves interactive mode
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" exits cleanly

  @requirement-51-bounded-scrollback
  Scenario: typing while scrolled back snaps the view back to the live bottom
    Given deck client "A" is started
    When deck client "A" creates shell session "scroll-snap-target"
    Then deck client "A" screen contains "scroll-snap-target"
    And deck client "A" selects session "scroll-snap-target"
    When deck client "A" enters interactive mode
    And deck client "A" types a 60-line numbered loop labelled "SNAP_SCROLL_LINE" into the interactive pane
    Then deck client "A" screen contains "SNAP_SCROLL_LINE_60"
    When deck client "A" sends shift+pgup 3 times
    Then deck client "A" screen contains "SNAP_SCROLL_LINE_1 "
    When deck client "A" types "echo AFTER_SNAP" and Enter into the interactive pane
    Then deck client "A" screen contains "AFTER_SNAP"
    And deck client "A" screen does not contain "SNAP_SCROLL_LINE_1 "
    When deck client "A" leaves interactive mode
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" exits cleanly
