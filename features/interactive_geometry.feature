Feature: Interactive mode entry geometry (Part II, requirements 7-8)
  Entering interactive mode records the window's own geometry before doing
  anything else, then fits the window -- never the pane -- to the preview
  box. On a split window, the space a sibling pane and its border consume
  ("chrome") is proportional to the window's own current size, not fixed,
  so the resize is a chrome-compensated loop rather than a single shot; a
  naive alternative that requests the wanted PANE size as the window size
  directly, never compensating for chrome, is the mandatory negative
  control this feature also proves.

  Scenario: the chrome-compensated fit converges on a split window
    Given tmux session "geom-fit" is a bare 80x42 window split with a 10-row sibling pane
    When deck fits window "geom-fit" pane "geom-fit.0" to 80x22
    Then the fit converged in at most 6 resizes
    And tmux pane "geom-fit.0" is 80x22

  Scenario: the naive pane-targeting loop never converges on the same layout
    Given tmux session "geom-naive" is a bare 80x42 window split with a 10-row sibling pane
    When a naive pane-targeting loop targets window "geom-naive" pane "geom-naive.0" at 80x22 for at most 15 attempts
    Then the naive loop never converges
