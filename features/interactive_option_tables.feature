Feature: Interactive mode's restore is byte-exact across every tmux option table (Part II, requirement 12)

  Exit's restore (task 035, II-9) must do more than land the pane at the
  right size: every one of tmux's seven option tables -- server; global
  and local session; global and local window; global and local pane --
  must read exactly what it read before entry, with the sole exception of
  the local WINDOW table's own `window-size`, which entering sets to
  "manual" as a side effect of resize-window (task 034) and exiting
  unsets again (task 035). Every table is dumped and diffed line by line,
  not spot-checked by re-reading window-size alone.

  Scenario: entering touches only window-size, only in the window scope, and exiting restores every table exactly
    Given tmux session "opttab" is a bootstrapped bare 80x24 window
    When the option tables for tmux target "opttab" are captured as "before"
    And deck enters interactive mode on tmux session "opttab" fitting to 45x15
    And the option tables for tmux target "opttab" are captured as "entered"
    Then only the "window" option table differs between "before" and "entered" for tmux target "opttab", and the only difference is "window-size" changing to "manual"
    When deck exits interactive mode on tmux session "opttab"
    And the option tables for tmux target "opttab" are captured as "after"
    Then every option table is byte-identical between "before" and "after" for tmux target "opttab"
    And tmux window "opttab" option "window-size" is "latest" in the global scope
