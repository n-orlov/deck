Feature: Interactive wheel forwarding to a mouse-tracking pane program (R183, GH #59)

  While interactive mode is on, a wheel notch over the preview is the pane
  program's own input when that program has turned mouse reporting on, and
  scrolls deck's grid only otherwise. Every scenario runs a shell program
  that turns reporting on (DECSET 1000 and 1006) and shows its own input
  with `cat -v`, so the exact bytes the program received are on the deck
  client's screen. The pane has no raw mode of its own, so `stty -echo
  -icanon` lets cat see each byte as it arrives instead of at a newline.

  A marker is painted at a known pane cell (1-based row 5, column 10) with
  a cursor-addressing printf, so the expected report is known without
  reading deck's own layout arithmetic: a notch over the marker's first
  letter must reach the program as the SGR report for exactly that cell.
  The preview draws the pane's cursor only while the grid is at its live
  view (offset 0, R182), so "the cursor is right after the report" is the
  observable proof that forwarding left the grid's offset at 0.

  @requirement-183-wheel-forwarding
  Scenario: a notch over the preview reaches a mouse-tracking program as the exact SGR report and the grid stays live
    Given deck client "A" is started
    When deck client "A" creates shell session "wheel-fwd"
    Then deck client "A" screen contains "wheel-fwd"
    And deck client "A" selects session "wheel-fwd"
    When deck client "A" enters interactive mode
    And deck client "A" types a 60-line numbered loop labelled "WF_LINE" into the interactive pane
    Then deck client "A" screen contains "WF_LINE_60"
    When deck client "A" types "printf '\033[5;10HWHEEL_%s\033[?1000h\033[?1006h' TARGET; stty -echo -icanon; echo READY_$((1+1)); cat -v" and Enter into the interactive pane
    Then deck client "A" screen contains "WHEEL_TARGET"
    And deck client "A" screen contains "READY_2"
    When deck client "A" scrolls the interactive wheel down 1 time over the line containing "WHEEL_TARGET"
    Then deck client "A" screen contains "^[[<65;10;5M"
    And deck client "A" shows the pane cursor right after "^[[<65;10;5M"
    When deck client "A" scrolls the interactive wheel up 1 time over the line containing "WHEEL_TARGET"
    Then deck client "A" screen contains "^[[<64;10;5M"
    And deck client "A" shows the pane cursor right after "^[[<64;10;5M"
    And deck client "A" screen does not contain "WF_LINE_1 "
    When deck client "A" sends Ctrl+C
    And deck client "A" types "stty sane" and Enter into the interactive pane
    And deck client "A" leaves interactive mode
    Then deck client "A" exits cleanly

  @requirement-183-wheel-forwarding
  Scenario: after the program turns reporting off the next notch scrolls the grid
    Given deck client "A" is started
    When deck client "A" creates shell session "wheel-off"
    Then deck client "A" screen contains "wheel-off"
    And deck client "A" selects session "wheel-off"
    When deck client "A" enters interactive mode
    And deck client "A" types a 60-line numbered loop labelled "WO_LINE" into the interactive pane
    Then deck client "A" screen contains "WO_LINE_60"
    When deck client "A" types "printf '\033[5;10HWHEEL_%s\033[?1000h\033[?1006h' TARGET; stty -echo -icanon; echo READY_$((1+1)); cat -v" and Enter into the interactive pane
    Then deck client "A" screen contains "READY_2"
    When deck client "A" scrolls the interactive wheel down 1 time over the line containing "WHEEL_TARGET"
    Then deck client "A" screen contains "^[[<65;10;5M"
    And deck client "A" screen does not contain "WO_LINE_1 "
    When deck client "A" sends Ctrl+C
    And deck client "A" types "stty sane; printf '\033[?1000l'; echo OFF_$((1+1))" and Enter into the interactive pane
    Then deck client "A" screen contains "OFF_2"
    And deck client "A" screen does not contain "WO_LINE_1 "
    When deck client "A" scrolls the interactive wheel up 20 times over the line containing "OFF_2"
    Then deck client "A" screen contains "WO_LINE_1 "
    When deck client "A" scrolls the interactive wheel down 20 times over the line containing "WO_LINE_"
    Then deck client "A" screen does not contain "WO_LINE_1 "
    When deck client "A" leaves interactive mode
    Then deck client "A" exits cleanly

  @requirement-183-wheel-forwarding
  Scenario: Shift+wheel scrolls the grid while the program has reporting on, and nothing reaches the program
    Given deck client "A" is started
    When deck client "A" creates shell session "wheel-shift"
    Then deck client "A" screen contains "wheel-shift"
    And deck client "A" selects session "wheel-shift"
    When deck client "A" enters interactive mode
    And deck client "A" types a 60-line numbered loop labelled "WS_LINE" into the interactive pane
    Then deck client "A" screen contains "WS_LINE_60"
    When deck client "A" types "printf '\033[5;10HWHEEL_%s\033[?1000h\033[?1006h' TARGET; stty -echo -icanon; echo READY_$((1+1)); cat -v" and Enter into the interactive pane
    Then deck client "A" screen contains "READY_2"
    And deck client "A" screen does not contain "WS_LINE_1 "
    When deck client "A" scrolls the interactive wheel up 20 times holding Shift over the line containing "WHEEL_TARGET"
    Then deck client "A" screen contains "WS_LINE_1 "
    When deck client "A" scrolls the interactive wheel down 20 times holding Shift over the line containing "WS_LINE_"
    Then deck client "A" screen does not contain "WS_LINE_1 "
    And deck client "A" screen contains "READY_2"
    And deck client "A" screen does not contain "^[[<6"
    When deck client "A" sends Ctrl+C
    And deck client "A" leaves interactive mode
    Then deck client "A" exits cleanly
