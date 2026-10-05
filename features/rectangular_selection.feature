Feature: Rectangular selection in the interactive preview (R203, GH #66)

  While interactive mode is on, dragging over the preview with Alt held, or
  with Ctrl held (the fallback for window managers and terminals that
  swallow Alt), selects the rectangle spanned by the two corner cells, and
  releasing copies one line per row: the characters inside the column range,
  trailing blanks trimmed. A plain drag still selects by line: the first row from the press, whole rows between, the last row up to the release (so the middle rows keep their nine leading blanks). Every scenario
  paints a three-row block at pane rows 5-7, column 10 with a shell command
  (the text is split across printf arguments so the typed command line never
  contains it), then drags from column offset 1 of the first row to column
  offset 3 of the third row, so the expected copy is the middle three columns
  of each row, and the tmux buffer on deck's own private socket is read back.

  @requirement-203-rectangular-selection
  Scenario: an Alt+drag copies the rectangle from a fixture screen
    Given deck client "A" is started
    When deck client "A" creates shell session "rect-alt"
    Then deck client "A" screen contains "rect-alt"
    And deck client "A" selects session "rect-alt"
    When deck client "A" enters interactive mode
    And deck client "A" types "printf '\033[5;10H%s%s\033[6;10H%s%s\033[7;10H%s%s' AB CDEF GH IJKL MN OPQR; echo READY_$((1+1))" and Enter into the interactive pane
    Then deck client "A" screen contains "ABCDEF"
    And deck client "A" screen contains "READY_2"
    When deck client "A" Alt+drags from column offset 1 row offset 0 to column offset 3 row offset 2 of the text "ABCDEF" in the interactive pane
    Then deck's own selection buffer holds the rows "BCD | HIJ | NOP"
    When deck client "A" leaves interactive mode
    Then deck client "A" exits cleanly

  @requirement-203-rectangular-selection
  Scenario: a Ctrl+drag copies the same rectangle
    Given deck client "A" is started
    When deck client "A" creates shell session "rect-ctrl"
    Then deck client "A" screen contains "rect-ctrl"
    And deck client "A" selects session "rect-ctrl"
    When deck client "A" enters interactive mode
    And deck client "A" types "printf '\033[5;10H%s%s\033[6;10H%s%s\033[7;10H%s%s' AB CDEF GH IJKL MN OPQR; echo READY_$((1+1))" and Enter into the interactive pane
    Then deck client "A" screen contains "ABCDEF"
    And deck client "A" screen contains "READY_2"
    When deck client "A" Ctrl+drags from column offset 1 row offset 0 to column offset 3 row offset 2 of the text "ABCDEF" in the interactive pane
    Then deck's own selection buffer holds the rows "BCD | HIJ | NOP"
    When deck client "A" leaves interactive mode
    Then deck client "A" exits cleanly

  @requirement-203-rectangular-selection
  Scenario: a plain drag over the same block still copies the line run
    Given deck client "A" is started
    When deck client "A" creates shell session "rect-plain"
    Then deck client "A" screen contains "rect-plain"
    And deck client "A" selects session "rect-plain"
    When deck client "A" enters interactive mode
    And deck client "A" types "printf '\033[5;10H%s%s\033[6;10H%s%s\033[7;10H%s%s' AB CDEF GH IJKL MN OPQR; echo READY_$((1+1))" and Enter into the interactive pane
    Then deck client "A" screen contains "ABCDEF"
    And deck client "A" screen contains "READY_2"
    When deck client "A" drags from column offset 1 row offset 0 to column offset 3 row offset 2 of the text "ABCDEF" in the interactive pane
    Then deck's own selection buffer holds the rows "BCDEF |          GHIJKL |          MNOP"
    When deck client "A" leaves interactive mode
    Then deck client "A" exits cleanly
