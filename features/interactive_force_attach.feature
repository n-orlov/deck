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

  # This scenario proves the OTHER displacement flavour task 118 names: a
  # real tmux client's full attach, never a second deck client's F-steal.
  # It never touches the ownership option (interactive_displacement.go's own
  # doc: "a plain `tmux attach` never touches pipe-pane"), so it can only be
  # caught by checkInteractiveDisplacementBackstop's SessionAttachedCount
  # read, not the pipe-displacement fast path the scenario above exercises --
  # and the same raiseLostAttach exit both flavours share still applies: A
  # leaves by the ordinary teardown, the window is restored per the existing
  # attached-client gating (unset window-size, let window-size latest
  # follow), and both isize options are released.
  Scenario: a real client's full attach displaces the interactive holder and the window is restored
    Given deck client "A" is started
    And deck client "A" creates shell session "grabbed"
    And deck client "A" selects session "grabbed"
    And the private tmux window for session "grabbed" is captured as "before-real-attach"
    When deck client "A" enters interactive mode
    Then deck client "A" screen contains "Ctrl+Q"
    When a real tmux client attaches to deck session "grabbed" at 80x24
    Then deck client "A" screen contains "Lost attach: grabbed"
    When deck client "A" dismisses the lost-attach dialog
    Then deck client "A" screen contains "deck - sessions"
    And the private tmux window for session "grabbed" still matches "before-real-attach"
    And tmux window "deck_grabbed" option "@deck_isize_owner" is unset in the window scope
    And tmux window "deck_grabbed" option "@deck_isize_geometry" is unset in the window scope
    And the real tmux client attached to session "grabbed" detaches
    And deck client "A" exits cleanly

  # The scenarios above all involve a steal or a real attach displacing an
  # existing interactive holder. This one proves the OTHER half of the same
  # ownership mechanism from the opposite direction: ordinary passive
  # preview (SPEC.md ~1115, "passive fit stands down entirely, issuing no
  # resize-window at all, while another live process holds §11.9's
  # ownership claim") declining to touch a window a SECOND client merely
  # SELECTS while a FIRST client is already interactive on it -- B never
  # enters interactive mode and never forces anything here. Selecting the
  # row is enough to schedule B's own previewFit on its next tick, and
  # that tick's foreign-live-claim probe (task 103, the same real-claim
  # mechanism task 125's unit test proved in isolation) must see A's real,
  # live ownership and issue no resize-window at all -- the window stays
  # exactly at the size A's own interactive entry fitted it to.
  Scenario: passive fit stands down for B while A holds the window interactively
    Given deck client "A" is started
    And deck client "B" is started
    And deck client "A" creates shell session "watched-live"
    Then within one configured reconcile interval deck client "B" screen contains "watched-live"
    And deck client "A" selects session "watched-live"
    When deck client "A" enters interactive mode
    Then deck client "A" screen contains "Ctrl+Q"
    And the private tmux window for session "watched-live" is captured as "A-held"
    When deck client "B" selects session "watched-live"
    And 300 milliseconds pass
    Then the private tmux window for session "watched-live" still matches "A-held"
    When deck client "A" leaves interactive mode
    And deck client "A" exits cleanly
    And deck client "B" exits cleanly

  # The scenarios above prove the steal itself; this one proves the refusal
  # that precedes it when the steal is NOT forced. B's plain Enter, tried
  # while A already holds the window's ownership claim, must be REFUSED
  # (requirement 47's live-ownership case: A's own interactive entry claims
  # the window via ClaimWindowOwnership, so B's plain Enter meets the same
  # "a live process holds ownership" refusal interactive_refusals.feature
  # exercises against a hand-crafted claim -- here against a real one) and
  # must name `F` as the way past it -- not silently degrade and not steal
  # on its own. Only B's SUBSEQUENT `F` may steal, and the durable row must
  # show exactly one attached event for that steal: neither B's refused
  # plain Enter nor A's own fall-out through the lost-attach dialog may add
  # a second one.
  # The lost-attach dialog raised on the displaced client (task 116) binds
  # only Enter -- updateLostAttachView routes every OTHER key to itself and
  # does nothing with it (SPEC.md ~1770, "swallows every key -- that is its
  # point"). This proves that literally for three keys that are anything
  # but inert in the ordinary list view: `j` moves the selection, `dd` opens
  # a confirm dialog naming what survives, `x` kills the selected session
  # outright. None of the three may reach the list's own handler while the
  # dialog is up -- the selection A had before the steal must still be
  # exactly where A left it once the dialog is dismissed, no confirm dialog
  # may appear, and the row's durable state (a shell session starts and
  # stays "running" from "tmux", RecordAttachment's default case is a
  # no-op for any status but waiting/error, so acknowledged/notify_epoch/
  # attached-event count never move for this session at any point in the
  # scenario) must read back identical to its pre-steal value.
  Scenario: the lost-attach dialog swallows j, dd and x
    Given deck client "A" is started
    And deck client "B" is started
    And deck client "A" creates shell session "swallowed"
    Then within one configured reconcile interval deck client "B" screen contains "swallowed"
    And deck client "A" selects session "swallowed"
    And deck client "B" selects session "swallowed"
    And the state database session "swallowed" is "running" from "tmux" with acknowledged=1, notify_epoch=0, and 0 attached events
    When deck client "A" enters interactive mode
    Then deck client "A" screen contains "Ctrl+Q"
    When deck client "B" forces entry into interactive mode
    Then deck client "B" screen contains "Ctrl+Q"
    And deck client "A" screen contains "Lost attach: swallowed"
    When deck client "A" sends "j"
    And deck client "A" sends "dd"
    And deck client "A" sends "x"
    Then deck client "A" screen contains "Lost attach: swallowed"
    And deck client "A" screen does not contain "Enter deletes"
    And the state database session "swallowed" is "running" from "tmux" with acknowledged=1, notify_epoch=0, and 0 attached events
    When deck client "A" dismisses the lost-attach dialog
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" has session "swallowed" selected
    And deck client "A" screen does not contain "Enter deletes"
    And the state database session "swallowed" is "running" from "tmux" with acknowledged=1, notify_epoch=0, and 0 attached events
    When deck client "B" leaves interactive mode
    And deck client "A" exits cleanly
    And deck client "B" exits cleanly

  Scenario: a refused plain entry names F, and only B's forced steal is durably attached
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    And deck client "B" is started
    When deck client "A" creates claude session "held entry" with permission profile "safe"
    Then within one configured reconcile interval deck client "B" screen contains "held entry"
    When the released running hook fires for session "held entry"
    And deck client "A" selects session "held entry"
    And deck client "A" enters interactive mode
    Then deck client "A" screen contains "Ctrl+Q"
    When the released waiting hook fires for session "held entry"
    Then the state database session "held entry" is "waiting" from "hook" with acknowledged=0, notify_epoch=0, and 0 attached events
    And within one configured reconcile interval deck client "B" row "held entry" contains "waiting"
    When deck client "B" selects session "held entry"
    And deck client "B" enters interactive mode
    Then deck client "B" screen contains "holds ownership of this window"
    And deck client "B" screen contains "press a to attach"
    And deck client "B" screen contains "F to force it"
    And deck client "B" screen contains "deck - sessions"
    And the state database session "held entry" has 0 attached events
    When deck client "B" forces entry into interactive mode
    Then deck client "B" screen contains "Ctrl+Q"
    And the state database session "held entry" is "running" from "user" with acknowledged=1, notify_epoch=1, and 1 attached event
    And deck client "A" screen contains "Lost attach: held entry"
    When deck client "A" dismisses the lost-attach dialog
    Then deck client "A" screen contains "deck - sessions"
    And the state database session "held entry" has 1 attached event
    When deck client "B" leaves interactive mode
    And deck client "A" exits cleanly
    And deck client "B" exits cleanly
