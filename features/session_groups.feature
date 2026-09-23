@session-groups
Feature: SPEC §11's group navigation and order (Phase 4c Tier 1 -- GH #31, #33, #34)
  SPEC §13.5 has named features/session_groups.feature since Phase 4b and it
  was never written -- group behaviour has been spread across ten other
  feature files ever since. This file owns Tier 1's three independent
  fixes: the sidebar viewport follows the cursor on every keyboard move
  (R136, GH #31), a group header click works while a preview is live and
  not only in list mode (R138, GH #33), and `[ui] default_group_first` can
  render the implicit default group first instead of last (R139, GH #34).
  R137's header-cursor work (GH #25/#32) is Tier 2 and out of scope here.

  Background:
    Given a long-running fake "claude" binary is on PATH for future deck clients

  @issue-34-default-group-first
  Scenario: default_group_first renders the implicit default group first instead of last, without disturbing alphabetical order among the rest
    Given the scenario's config.toml is written with:
      """
      [ui]
      default_group_first = true
      """
    And deck client "A" is started
    When deck client "A" creates shell session "dgf-default-1"
    And deck client "A" creates shell session "dgf-aaa-1"
    And deck client "A" creates shell session "dgf-zzz-1"
    And the state database session "dgf-aaa-1" is in group "aaa-workspace"
    And the state database session "dgf-zzz-1" is in group "zzz-workspace"
    Then within one configured reconcile interval deck client "A" screen contains "aaa-workspace"
    And deck client "A" screen contains "zzz-workspace"
    And deck client "A" screen shows sessions in this order:
      | dgf-default-1 |
      | dgf-aaa-1     |
      | dgf-zzz-1     |
    When deck client "A" exits cleanly

  @issue-33-header-click-list-mode
  Scenario: clicking a group header in list mode collapses only that group, and clicking it again reopens it
    Given deck client "A" is started
    When deck client "A" creates shell session "hdrlist-default"
    And deck client "A" creates shell session "hdrlist-other"
    And the state database session "hdrlist-other" is in group "hdrlist-workspace"
    Then deck client "A" screen contains "hdrlist-workspace"
    And deck client "A" screen contains "hdrlist-default"
    And deck client "A" screen contains "hdrlist-other"
    When deck client "A" clicks on the row containing "hdrlist-workspace"
    Then deck client "A" screen stops containing "hdrlist-other"
    And deck client "A" screen contains "hdrlist-workspace"
    And deck client "A" screen contains "hdrlist-default"
    When deck client "A" clicks on the row containing "hdrlist-workspace"
    Then deck client "A" screen contains "hdrlist-other"
    When deck client "A" exits cleanly

  @issue-33-header-click-while-preview-is-live
  Scenario: a group header click collapses that group while a preview is live, and interactive mode is left undisturbed
    Given deck client "A" is started
    When deck client "A" creates shell session "hdrlive-default"
    And deck client "A" creates shell session "hdrlive-other"
    And the state database session "hdrlive-other" is in group "hdrlive-workspace"
    Then deck client "A" screen contains "hdrlive-workspace"
    And deck client "A" screen contains "hdrlive-default"
    And deck client "A" screen contains "hdrlive-other"
    When deck client "A" selects session "hdrlive-default"
    And deck client "A" enters interactive mode
    Then deck client "A" preview top border contains "hdrlive-default"
    When deck client "A" clicks on the row containing "hdrlive-workspace"
    Then deck client "A" screen stops containing "hdrlive-other"
    And deck client "A" screen contains "hdrlive-workspace"
    And deck client "A" screen contains "hdrlive-default"
    And deck client "A" preview top border contains "hdrlive-default"
    When deck client "A" clicks on the row containing "hdrlive-workspace"
    Then deck client "A" screen contains "hdrlive-other"
    And deck client "A" preview top border contains "hdrlive-default"
    When deck client "A" leaves interactive mode
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" exits cleanly

  @issue-31-viewport-follows-keyboard-cursor
  Scenario: the sidebar viewport follows the cursor on every keyboard move, not only the mouse wheel
    Given deck client "A" is started
    When deck client "A" creates shell session "kbfollow-1"
    And deck client "A" creates shell session "kbfollow-2"
    And deck client "A" creates shell session "kbfollow-3"
    And deck client "A" creates shell session "kbfollow-4"
    And deck client "A" creates shell session "kbfollow-5"
    And the state database session "kbfollow-1" has status "idle" 50 seconds ago
    And the state database session "kbfollow-2" has status "idle" 40 seconds ago
    And the state database session "kbfollow-3" has status "idle" 30 seconds ago
    And the state database session "kbfollow-4" has status "idle" 20 seconds ago
    And the state database session "kbfollow-5" has status "idle" 10 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "idle"
    # Shrinks the sidebar's content height (stacked layout) so all five
    # rows no longer fit on screen at once, the same technique
    # features/mouse.feature's wheel-scroll scenario already relies on.
    And deck client "A" sends "|"
    And deck client "A" sends "|"
    Then deck client "A" screen stops containing "kbfollow-5"
    And deck client "A" screen contains "kbfollow-1"
    When deck client "A" selects session "kbfollow-1"
    And deck client "A" sends "j"
    And deck client "A" sends "j"
    And deck client "A" sends "j"
    And deck client "A" sends "j"
    Then deck client "A" has session "kbfollow-5" selected
    And deck client "A" screen contains "kbfollow-5"
    When deck client "A" sends "k"
    And deck client "A" sends "k"
    And deck client "A" sends "k"
    And deck client "A" sends "k"
    Then deck client "A" has session "kbfollow-1" selected
    And deck client "A" screen contains "kbfollow-1"
    When deck client "A" exits cleanly
