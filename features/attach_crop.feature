@attach-crop
Feature: `a`'s full attach is never cropped by deck's own preview fit
  Passive preview fits the selected session's window to the preview panel
  (SPEC §11), and `resize-window` writes `window-size manual` into the
  WINDOW options as a side effect -- which shadow §3.2's server-global
  `window-size latest`. Left there, that pin outlives the preview: the next
  full attach is shown the window at whatever size the panel happened to be,
  cropped into a corner of the terminal, and stays that way until some
  unrelated interactive-mode exit unsets the option. That is the operator's
  own report ("hit a to full tmux attach, observe view cropped to the deck
  preview size... Enter to attach input, Ctrl+Q back to list, a again, crop
  now fixed"), and the rule it settled: deck's own preview must never crop
  `a`. Only ANOTHER tmux session -- deck-mediated or a direct attach -- may.

  So passive fit unpins after fitting, and `a` releases a pin it finds (any
  pin but a live owner's) before handing the terminal over. The observable
  asserted here is tmux's own window geometry against the deck client's real
  terminal, never an option value on its own: an option is a mechanism, and
  a mechanism can be right while the window is still cropped.

  Scenario: a full attach after a settled passive fit fills the terminal, and the preview re-fits on the way back
    Given deck client "A" is started
    When deck client "A" creates shell session "uncropped"
    And deck client "A" selects session "uncropped"
    # The fit itself: the window is now the preview panel's own content box,
    # narrower than the terminal (that is what makes a crop possible at
    # all), and the pin it was made with is gone again.
    Then within one preview tick the private tmux window for session "uncropped" is narrower than deck client "A" terminal, and left unpinned
    When deck client "A" attaches to the selected session
    # The bug, as reported: this read the fitted box before the fix.
    Then the private tmux window for session "uncropped" is as wide as deck client "A" terminal
    When deck client "A" detaches
    Then deck client "A" screen contains "deck - sessions"
    # tmux leaves the window at the departing client's size, so the row the
    # user lands back on is the one row passive fit's per-session coalescing
    # would otherwise never fit again (SPEC §11's relaunch carve-out, now
    # also covering a return from `a`): exactly one fit is relicensed.
    And within one preview tick the private tmux window for session "uncropped" is narrower than deck client "A" terminal, and left unpinned
    And deck client "A" exits cleanly
