Feature: Interactive click forwarding to a mouse-tracking pane program (R202, GH #67)

  While interactive mode is on, a click of the left, middle or right button
  over the preview is the pane program's own input when that program has
  turned mouse reporting on (DECSET 1000 and 1006), as a press and a release
  at the press cell; a program that has not gets nothing. It is the routing
  rule the wheel uses (R183). A "-" and Enter between clicks starts a fresh
  output line, so a report never wraps at the pane's edge. Every scenario runs a shell program that shows
  its own input with `cat -v`, so the exact bytes it received are on the deck
  client's screen, and a marker painted at pane row 5, column 10 gives the
  expected cell without reading deck's own layout arithmetic.

  @requirement-202-click-forwarding
  Scenario: a right, middle and left click reach a mouse-tracking program with their own button code
    Given deck client "A" is started
    When deck client "A" creates shell session "click-fwd"
    Then deck client "A" screen contains "click-fwd"
    And deck client "A" selects session "click-fwd"
    When deck client "A" enters interactive mode
    And deck client "A" types "printf '\033[5;10HCLICK_%s\033[?1000h\033[?1006h' TARGET; stty -echo -icanon; echo READY_$((1+1)); cat -v" and Enter into the interactive pane
    Then deck client "A" screen contains "CLICK_TARGET"
    And deck client "A" screen contains "READY_2"
    When deck client "A" clicks the right button over the line containing "CLICK_TARGET" in the interactive preview
    Then deck client "A" screen contains "^[[<2;10;5M"
    And deck client "A" screen contains "^[[<2;10;5m"
    When deck client "A" types "-" and Enter into the interactive pane
    And deck client "A" clicks the middle button over the line containing "CLICK_TARGET" in the interactive preview
    Then deck client "A" screen contains "^[[<1;10;5M"
    And deck client "A" screen contains "^[[<1;10;5m"
    When deck client "A" types "-" and Enter into the interactive pane
    And deck client "A" clicks the left button over the line containing "CLICK_TARGET" in the interactive preview
    Then deck client "A" screen contains "^[[<0;10;5M"
    And deck client "A" screen contains "^[[<0;10;5m"
    When deck client "A" sends Ctrl+C
    And deck client "A" types "stty sane" and Enter into the interactive pane
    And deck client "A" leaves interactive mode
    Then deck client "A" exits cleanly

  @requirement-202-click-forwarding
  Scenario: a click over a plain shell reaches nothing
    Given deck client "A" is started
    When deck client "A" creates shell session "click-plain"
    Then deck client "A" screen contains "click-plain"
    And deck client "A" selects session "click-plain"
    When deck client "A" enters interactive mode
    And deck client "A" types "printf '\033[5;10HCLICK_%s' TARGET; stty -echo -icanon; echo READY_$((1+1)); cat -v" and Enter into the interactive pane
    Then deck client "A" screen contains "CLICK_TARGET"
    And deck client "A" screen contains "READY_2"
    When deck client "A" clicks the right button over the line containing "CLICK_TARGET" in the interactive preview
    And deck client "A" clicks the middle button over the line containing "CLICK_TARGET" in the interactive preview
    And deck client "A" clicks the left button over the line containing "CLICK_TARGET" in the interactive preview
    Then deck client "A" screen does not contain "^[[<"
    And deck client "A" screen does not contain "^[[M"
    And deck client "A" screen contains "READY_2"
    When deck client "A" sends Ctrl+C
    And deck client "A" types "stty sane" and Enter into the interactive pane
    And deck client "A" leaves interactive mode
    Then deck client "A" exits cleanly
