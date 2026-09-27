@interactive-sidebar-wheel
Feature: R149 (GH #47) -- the sidebar wheel while interactive mode is active, and its drift-ending keystroke

  Tasks 001/002 gave the wheel over the sidebar the same job while
  interactive mode owns the keyboard as it already has in list mode
  (scroll the sidebar's own viewport, arming the same drift SPEC's wheel
  section describes) and made updateInteractive/exitInteractive end that
  drift -- follow the selection back into view -- on the very next key,
  exactly like guardSessionScopedKey already does outside interactive
  mode. This scenario proves both halves end to end, against a real tmux
  pane, real deck client and real terminal input, not merely the unit
  fixtures internal/tui/interactive_sidebar_wheel_test.go and
  internal/tui/interactive_drift_end_test.go already cover:
  - the wheel moves the sidebar's viewport (a session that was scrolled
    off screen becomes visible) while the interactive target, the
    preview panel's own rendered content, and the live tmux window's
    real size are all provably untouched;
  - the next ordinary keystroke both lands the selection back in view
    AND still reaches the real pane, byte-exact.

  @requirement-149-sidebar-wheel-and-drift-end
  Scenario: wheeling the sidebar while interactive mode is active moves only the sidebar viewport, and the next keystroke ends the drift and still reaches the pane
    Given deck client "A" is started
    When deck client "A" creates shell session "wheel149-1"
    And deck client "A" creates shell session "wheel149-2"
    And deck client "A" creates shell session "wheel149-3"
    And deck client "A" creates shell session "wheel149-4"
    And deck client "A" creates shell session "wheel149-5"
    Then within one configured reconcile interval deck client "A" screen contains "running"
    # Select the target BEFORE the shrink below: the sidebar viewport
    # follows the selection on a layout change too (SPEC §11), so the
    # newest row a create left selected would otherwise stay in view.
    And deck client "A" selects session "wheel149-1"
    And deck client "A" sends "|"
    And deck client "A" sends "|"
    Then deck client "A" screen stops containing "wheel149-5"
    When deck client "A" enters interactive mode
    Then deck client "A" preview top border contains "wheel149-1"
    And the private tmux window for session "wheel149-1" is captured as "wheel149-window-before-wheel"
    And deck client "A" preview content is captured as "wheel149-grid-before-wheel"
    When deck client "A" scrolls the wheel down at column 5 row 5
    And deck client "A" scrolls the wheel down at column 5 row 5
    And deck client "A" scrolls the wheel down at column 5 row 5
    And deck client "A" scrolls the wheel down at column 5 row 5
    Then deck client "A" screen contains "wheel149-5"
    And deck client "A" preview top border contains "wheel149-1"
    And the private tmux window for session "wheel149-1" still matches "wheel149-window-before-wheel"
    And deck client "A" preview content still matches the captured "wheel149-grid-before-wheel"
    When deck client "A" sends "Z"
    Then deck client "A" has session "wheel149-1" selected
    And the private tmux pane for session "wheel149-1" received "Z" byte-exact
    When deck client "A" leaves interactive mode
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" exits cleanly
