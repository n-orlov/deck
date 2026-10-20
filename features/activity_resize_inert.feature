@activity-resize-inert
Feature: a click is never output in the `activity` order (R412, SPEC §7 output clock)
  With `[ui] sort_order = "activity"`, a click on a session row resizes that
  session's real tmux window, and tmux reflows the wrapped transcript the
  pane already holds. A resize from any source is never output (R409), so the
  clicked row keeps its place. The pane here is a still program that draws a
  transcript line wider than the narrowed window, a composer frame and a
  footer, is drawn after another process widened its window, and redraws nothing when the window changes size: the reflowed
  transcript first shows up in the tick that reads the pane's next output,
  which is the footer churn in a later output bucket.

  @r412-click-leaves-activity-order-unchanged
  Scenario: a click on the lower of two idle sessions leaves the activity order unchanged, and so does its statusline churning in a later bucket
    Given the scenario's config.toml is written with:
      """
      [ui]
      sort_order = "activity"
      output_bucket = 2
      """
    And deck client "A" is started
    When deck client "A" creates shell session "act-old"
    And deck client "A" creates shell session "act-new"
    And the private tmux window for session "act-old" is resized by another process to 100x27
    And the private tmux pane for session "act-old" draws a wrapped transcript, a composer frame and the footer "~/proj  main  ctx:12%"
    And the private tmux pane for session "act-new" draws a wrapped transcript, a composer frame and the footer "~/proj  main  ctx:12%"
    Then deck client "A" screen shows sessions in this order:
      | act-new |
      | act-old |
    And after 4 configured reconcile intervals deck client "A" screen shows sessions in this order:
      | act-new |
      | act-old |
    When the private tmux window for session "act-old" is captured as "before the click"
    And deck client "A" clicks on the row containing "act-old"
    Then the private tmux window for session "act-old" no longer matches "before the click"
    And after 4 configured reconcile intervals deck client "A" screen shows sessions in this order:
      | act-new |
      | act-old |
    # The click's discount window is min(output_bucket, 5 s) = 2 s. Wait it out
    # (a real wall-clock window deck schedules) so the churn below is not
    # covered by the discount and only the size-keyed baseline can keep the
    # reflowed transcript from being counted.
    When 2500 milliseconds pass
    And the private tmux pane for session "act-old" churns its footer to "~/proj  feature  ctx:41%" in a later 2-second output bucket
    Then after 4 configured reconcile intervals deck client "A" screen shows sessions in this order:
      | act-new |
      | act-old |
    When deck client "A" leaves interactive mode
    And deck client "A" exits cleanly

  @r412-second-client-resize-leaves-activity-order-unchanged
  Scenario: a second deck client resizing a session's window leaves the first client's activity order unchanged, and so does the session's statusline churning in a later bucket
    # Client "B" runs in a larger terminal on the same tmux socket. Selecting
    # the session in B fits its window to B's preview box, a resize client "A"
    # did not make and has no discount for: the only thing that keeps the
    # reflowed transcript from being counted as output in A is the size-keyed
    # baseline. The churn comes after every discount window of A and of B.
    Given the scenario's config.toml is written with:
      """
      [ui]
      sort_order = "activity"
      output_bucket = 2
      """
    And deck client "A" is started
    When deck client "A" creates shell session "act-old"
    And deck client "A" creates shell session "act-new"
    And the private tmux pane for session "act-old" draws a wrapped transcript, a composer frame and the footer "~/proj  main  ctx:12%"
    And the private tmux pane for session "act-new" draws a wrapped transcript, a composer frame and the footer "~/proj  main  ctx:12%"
    Then deck client "A" screen shows sessions in this order:
      | act-new |
      | act-old |
    And after 4 configured reconcile intervals deck client "A" screen shows sessions in this order:
      | act-new |
      | act-old |
    When the private tmux window for session "act-old" is captured as "before B"
    And deck client "B" is started with terminal size 140x40
    And deck client "B" selects session "act-old"
    Then the private tmux window for session "act-old" no longer matches "before B"
    And within 5 seconds the private tmux window for session "act-old" reports geometry "101x37"
    And after 4 configured reconcile intervals deck client "A" screen shows sessions in this order:
      | act-new |
      | act-old |
    # Wait out the discount window (min(output_bucket, 5 s) = 2 s) of any
    # client that acted on the window, so the churn is not covered by it.
    When 2500 milliseconds pass
    And the private tmux pane for session "act-old" churns its footer to "~/proj  feature  ctx:41%" in a later 2-second output bucket
    Then after 4 configured reconcile intervals deck client "A" screen shows sessions in this order:
      | act-new |
      | act-old |
    When deck client "B" exits cleanly
    And deck client "A" exits cleanly
