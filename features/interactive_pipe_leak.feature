Feature: The next start reclaims a leaked interactive pipe (Part II, R89, task 030)

  SIGKILL cannot be handled at all, so a deck client killed mid-interactive
  leaves its armed `pipe-pane`, its FIFO temp dir and its window ownership
  claim all behind -- the guarantee has to be "the next start reclaims it",
  never "the exit cleans it". This proves both halves: the leak is real
  (the scan's own positive control), and the very next deck start against
  the same tmux server disarms the stale pipe, removes the leaked temp
  dir/FIFO, releases ownership and restores the window's geometry
  byte-exact (SPEC.md \u00a711.9), all without the reclaimed session's own
  window ever having had a live client attached to it in between.

  @requirement-89-reclaim-leaked-interactive-pipe
  Scenario: SIGKILL mid-interactive leaks pipe-pane and a temp dir; the next start reclaims both
    Given deck client "victim" is started
    And deck client "victim" creates shell session "leaky"
    Then within one configured reconcile interval deck client "victim" screen contains "running"
    And deck client "victim" selects session "leaky"
    And the private tmux window for session "leaky" is captured as "before-interactive"
    When deck client "victim" enters interactive mode
    Then deck client "victim" screen contains "Ctrl+Q"
    And tmux pane pipe is armed for session "leaky"
    And a deck interactive pipe temp directory for session "leaky" exists on disk
    And tmux window "deck_leaky" option "@deck_isize_owner" is set in the window scope
    When deck client "victim" is killed with SIGKILL
    Then tmux pane pipe is armed for session "leaky"
    And a deck interactive pipe temp directory for session "leaky" exists on disk
    Given deck client "rescuer" is started
    Then tmux pane pipe is not armed for session "leaky"
    And no deck interactive pipe temp directory for session "leaky" exists on disk
    And tmux window "deck_leaky" option "@deck_isize_owner" is unset in the window scope
    And the private tmux window for session "leaky" still matches "before-interactive"
    And deck client "rescuer" exits cleanly
