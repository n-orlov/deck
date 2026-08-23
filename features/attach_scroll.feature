@attach-scroll
Feature: §3.2/§11.8 a wheel notch in an attached session scrolls the pane, not the shell (requirement 48)
  deck's private tmux server is bootstrapped with `mouse on` (task 115) so that
  a wheel notch delivered to an attached pane scrolls its scrollback via
  tmux's own copy-mode, instead of the outer terminal's alternate-scroll
  translating it into an Up/Down arrow that a shell reads as history recall.
  This proves the behaviour against real tmux -- the observable is pane
  content, never the `mouse` option's own value.

  @requirement-48-wheel-scrolls-attached-pane-without-typing
  Scenario: a wheel notch scrolls an attached pane's scrollback and leaves the shell's input line untouched
    Given deck client "A" is started
    When deck client "A" creates shell session "attach-scroll-target"
    And deck client "A" attaches to the selected session
    And deck client "A" fills the attached pane with more than one screen of scrollback
    And deck client "A" captures its frame as "before-wheel-scroll"
    And deck client "A" scrolls the wheel up 30 times over the attached pane at column 20 row 15
    Then deck client "A" attached pane shows the top of the scrollback
    When deck client "A" exits copy-mode on the attached pane
    Then deck client "A" frame still matches the captured "before-wheel-scroll" frame
    When deck client "A" detaches
    Then deck client "A" screen contains "deck - sessions"
    When deck client "A" exits cleanly
