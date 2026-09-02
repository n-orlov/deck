@multiclient
Feature: Forced entry into interactive mode answers a waiting row
  `F` (SPEC "F forces entry over whoever holds the window") is identical to
  plain Enter in every respect but two: it does not refuse for another
  client's claim and it steals ownership over a live holder instead of
  standing down for one (task 105). Stealing the window is still a
  deck-mediated attachment in exactly the sense SPEC section 7 gives `a`
  and plain Enter: it answers a waiting row and records an attached event
  at the client that forced its way in, using the same durable transaction
  (task 109), not a degraded or partial one.

  The row only becomes waiting after the first client is already inside the
  preview, so the answering attachment cannot be the first client's ordinary
  entry: entry on a running row is a durable no-op (store.RecordAttachment
  only answers waiting and acknowledges error), leaving the pre-force state
  identical to status_attach.feature's -- waiting from hook, acknowledged=0,
  notify_epoch=0, 0 attached events. The forcing client's `F` is therefore
  the only attachment in the scenario, and the post-state asserted below is
  status_attach.feature's post-state for it.

  Scenario: a forced entry over a waiting row answers it at the forcing client
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    And deck client "B" is started
    When deck client "A" creates claude session "contested entry" with permission profile "safe"
    Then within one configured reconcile interval deck client "B" screen contains "contested entry"
    When the released running hook fires for session "contested entry"
    And deck client "A" selects session "contested entry"
    And deck client "A" enters interactive mode
    Then deck client "A" screen contains "Ctrl+Q"
    When the released waiting hook fires for session "contested entry"
    Then the state database session "contested entry" is "waiting" from "hook" with acknowledged=0, notify_epoch=0, and 0 attached events
    And within one configured reconcile interval deck client "B" row "contested entry" contains "waiting"
    When deck client "B" selects session "contested entry"
    And deck client "B" forces entry into interactive mode
    Then deck client "B" screen contains "Ctrl+Q"
    And the state database session "contested entry" is "running" from "user" with acknowledged=1, notify_epoch=1, and 1 attached event
    # B's steal displaced A out of interactive mode entirely (task 118's
    # previewTick fast path/async backstop, raised on A's OWN model without
    # A doing anything): A is now looking at the lost-attach dialog (task
    # 116), not at interactive mode, so Ctrl+Q -- which the dialog swallows,
    # "Enter dismisses" being its only bound key -- is no longer A's way
    # back to the list; dismissing the dialog is.
    When deck client "B" leaves interactive mode
    And deck client "A" dismisses the lost-attach dialog
    And deck client "A" exits cleanly
    And deck client "B" exits cleanly

  # The steal chain does more than answer the row (the scenario above): it
  # must also leave the WINDOW exactly as it was before any deck client
  # ever touched it -- byte-exact geometry (task 114's unit-test proof,
  # here on the real multi-client path) and both isize options released,
  # once the displaced client (A) has been through the lost-attach dialog
  # and the winner (B) has left by the ordinary exit path.
  Scenario: the window survives entry, a steal, the displaced client's dismissal and the winner's exit unchanged
    Given deck client "A" is started
    And deck client "B" is started
    And deck client "A" creates shell session "chained"
    Then within one configured reconcile interval deck client "B" screen contains "chained"
    And deck client "A" selects session "chained"
    And deck client "B" selects session "chained"
    And the private tmux window for session "chained" is captured as "before-any-entry"
    When deck client "A" enters interactive mode
    Then deck client "A" screen contains "Ctrl+Q"
    When deck client "B" forces entry into interactive mode
    Then deck client "B" screen contains "Ctrl+Q"
    And deck client "A" screen contains "Lost attach: chained"
    When deck client "A" dismisses the lost-attach dialog
    Then deck client "A" screen contains "deck - sessions"
    When deck client "B" leaves interactive mode
    Then the private tmux window for session "chained" still matches "before-any-entry"
    And tmux window "deck_chained" option "@deck_isize_owner" is unset in the window scope
    And tmux window "deck_chained" option "@deck_isize_geometry" is unset in the window scope
    And deck client "A" exits cleanly
    And deck client "B" exits cleanly
