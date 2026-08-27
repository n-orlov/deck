@terminal-repair-field-route
Feature: A terminal row with a live pane repairs itself through the field route (requirement 76)
  The field found deck wedged by a Claude `SessionEnd` hook that wrote
  `stopped` while the session's own pane -- the same process the hook's
  own subprocess ran inside -- kept running. Nobody pressed a key: the row
  simply stayed wrong until Reconcile (SPEC §7's self-healing rule) came
  along on its own schedule and corrected it. This reproduces that exact
  route -- a hook write, never a hand-edited state.db -- and proves the
  repair lands with no TUI keypress after the hook, and that the pane it
  never touches is still alive and attachable once the row is fixed.

  Scenario: A SessionEnd hook wedges an agent row while its pane survives, and Reconcile repairs it with nobody at the keyboard
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "wedged" with permission profile "safe"
    And fake Claude session "wedged" fires "SessionEnd" for itself using injected identity:
      | reason | logout |
    Then the state database session "wedged" has an event of kind "session_end" with reason containing "logout"
    And the private tmux session "deck_wedged" exists
    Then within one configured reconcile interval the state database session "wedged" is "starting" from "tmux" with killed_by_user=0
    And the state database session "wedged" has an event of kind "tmux.terminal_pane_alive" with reason containing "tmux pane is alive"
    And the private tmux session "deck_wedged" exists
    When deck client "A" attaches to and detaches from its session
    Then deck client "A" exits cleanly
