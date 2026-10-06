Feature: An OSC window title never prints into the interactive pane (R209, GH #69)

  Claude Code renames its session with an OSC 0 title prefixed by "✳"
  (UTF-8 E2 9C B3). The terminal emulator behind the interactive preview is
  byte-based and read 0x9C inside the OSC as the string terminator, so the
  rest of the title printed at the cursor, over the input row. A pre-filter
  in front of the emulator drops C1 bytes inside escape strings, so the same
  bytes leave the row as the pane drew it. The bytes reach deck through the
  pane's real pipe, exactly as Claude's do.

  @requirement-209-title-leak
  Scenario: the Claude title sequence leaves the interactive input row unchanged
    Given deck client "A" is started
    When deck client "A" creates shell session "title-leak"
    Then deck client "A" screen contains "title-leak"
    And deck client "A" selects session "title-leak"
    When deck client "A" enters interactive mode
    And deck client "A" types "exec bash --norc" and Enter into the interactive pane
    Then deck client "A" screen contains "bash-"
    When deck client "A" types "printf 'READY> \e]0;\xe2\x9c\xb3 Zq%sx\a\n' ''; echo DONE-$((6*7))" and Enter into the interactive pane
    Then deck client "A" screen contains "DONE-42"
    And deck client "A" screen contains "READY>"
    And deck client "A" screen does not contain "Zqx"
    When deck client "A" leaves interactive mode
    Then deck client "A" exits cleanly
