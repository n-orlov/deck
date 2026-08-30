@sort-order
Feature: `[ui] sort_order` -- attention, created, activity and name orders (requirement R53)
  SPEC §11's rewritten Sort bullet (amendment 6584299): `[ui] sort_order`
  selects among four total orders -- attention (default, unchanged), created
  (sessions.created_at descending), activity (sessions.status_at
  descending) and name (case-insensitive ascending) -- every one ending in
  id ascending as its tie-break.

  All four scenarios below share one fixture shape of four shell sessions --
  ord-alpha, ord-bravo, ord-charlie, ord-delta -- engineered so the four
  orders give FOUR DIFFERENT row sequences (no two agree):
    attention: ord-delta, ord-bravo, ord-alpha, ord-charlie   (by status: waiting, error, running, idle)
    created:   ord-bravo, ord-delta, ord-charlie, ord-alpha   (by created_at, newest first)
    activity:  ord-charlie, ord-alpha, ord-delta, ord-bravo   (by status_at, newest first)
    name:      ord-alpha, ord-bravo, ord-charlie, ord-delta   (case-insensitive ascending)
  A wrong comparator that happens to agree with a different order on this
  fixture would fail its scenario, since every expected sequence differs
  from every other one. The config write (where present) happens before the
  client starts, exactly like features/create_session.feature's
  recent_cwd_limit scenario, since ui.sort_order is read from config.toml at
  load and this file does not need task 306's live-apply path.

  @requirement-53-sort-order-attention
  Scenario: sort_order defaults to attention and orders by the waiting/error/running/idle tiers
    Given deck client "A" is started
    When deck client "A" creates shell session "ord-alpha"
    And deck client "A" creates shell session "ord-bravo"
    And deck client "A" creates shell session "ord-charlie"
    And deck client "A" creates shell session "ord-delta"
    And the state database session "ord-alpha" has created_at 35 seconds ago
    And the state database session "ord-bravo" has created_at 5 seconds ago
    And the state database session "ord-charlie" has created_at 25 seconds ago
    And the state database session "ord-delta" has created_at 15 seconds ago
    And the state database session "ord-alpha" has status "running" 20 seconds ago
    # "error" is posed here via a genuine pane exit, not a raw database
    # write, so the row's PaneExitStatus is actually set: SPEC section 7's
    # self-heal (internal/service.reconcile's repairTerminalRowWithLivePane)
    # narrows to a stopped row, or an error row that itself already carries
    # a pane-exit or tmux/user-sourced verdict, paired with a live pane -- a
    # bare hook/probe-sourced error with no such verdict is left alone
    # (finding F40, task 901). "ord-bravo"'s pane is genuinely dead by the time
    # this step returns, so reconcile's crash-collection (not the live-pane
    # repair) is what makes the row's "error" the real tmux.pane_dead
    # transition (task 804).
    And shell session "ord-bravo" exits with status 1
    And the state database session "ord-charlie" has status "idle" 10 seconds ago
    And the state database session "ord-delta" has status "waiting" 30 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "waiting"
    And within one configured reconcile interval deck client "A" row "ord-bravo" contains "error"
    And deck client "A" screen shows sessions in this order:
      | ord-delta   |
      | ord-bravo   |
      | ord-alpha   |
      | ord-charlie |
    When deck client "A" exits cleanly

  @requirement-53-sort-order-created
  Scenario: sort_order = created orders by created_at descending, newest first
    Given the scenario's config.toml is written with:
      """
      [ui]
      sort_order = "created"
      """
    And deck client "A" is started
    When deck client "A" creates shell session "ord-alpha"
    And deck client "A" creates shell session "ord-bravo"
    And deck client "A" creates shell session "ord-charlie"
    And deck client "A" creates shell session "ord-delta"
    And the state database session "ord-alpha" has created_at 35 seconds ago
    And the state database session "ord-bravo" has created_at 5 seconds ago
    And the state database session "ord-charlie" has created_at 25 seconds ago
    And the state database session "ord-delta" has created_at 15 seconds ago
    And the state database session "ord-alpha" has status "running" 20 seconds ago
    # "error" is posed here via a genuine pane exit, not a raw database
    # write, so the row's PaneExitStatus is actually set: SPEC section 7's
    # self-heal (internal/service.reconcile's repairTerminalRowWithLivePane)
    # narrows to a stopped row, or an error row that itself already carries
    # a pane-exit or tmux/user-sourced verdict, paired with a live pane -- a
    # bare hook/probe-sourced error with no such verdict is left alone
    # (finding F40, task 901). "ord-bravo"'s pane is genuinely dead by the time
    # this step returns, so reconcile's crash-collection (not the live-pane
    # repair) is what makes the row's "error" the real tmux.pane_dead
    # transition (task 804).
    And shell session "ord-bravo" exits with status 1
    And the state database session "ord-charlie" has status "idle" 10 seconds ago
    And the state database session "ord-delta" has status "waiting" 30 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "waiting"
    And within one configured reconcile interval deck client "A" row "ord-bravo" contains "error"
    And deck client "A" screen shows sessions in this order:
      | ord-bravo   |
      | ord-delta   |
      | ord-charlie |
      | ord-alpha   |
    When deck client "A" exits cleanly

  @requirement-53-sort-order-activity
  Scenario: sort_order = activity orders by status_at descending, most recently changed first
    Given the scenario's config.toml is written with:
      """
      [ui]
      sort_order = "activity"
      """
    And deck client "A" is started
    When deck client "A" creates shell session "ord-alpha"
    And deck client "A" creates shell session "ord-bravo"
    And deck client "A" creates shell session "ord-charlie"
    And deck client "A" creates shell session "ord-delta"
    And the state database session "ord-alpha" has created_at 35 seconds ago
    And the state database session "ord-bravo" has created_at 5 seconds ago
    And the state database session "ord-charlie" has created_at 25 seconds ago
    And the state database session "ord-delta" has created_at 15 seconds ago
    And the state database session "ord-alpha" has status "running" 20 seconds ago
    # "error" is posed here via a genuine pane exit, not a raw database
    # write, so the row's PaneExitStatus is actually set: SPEC section 7's
    # self-heal (internal/service.reconcile's repairTerminalRowWithLivePane)
    # narrows to a stopped row, or an error row that itself already carries
    # a pane-exit or tmux/user-sourced verdict, paired with a live pane -- a
    # bare hook/probe-sourced error with no such verdict is left alone
    # (finding F40, task 901). "ord-bravo"'s pane is genuinely dead by the time
    # this step returns, so reconcile's crash-collection (not the live-pane
    # repair) is what makes the row's "error" the real tmux.pane_dead
    # transition (task 804).
    And shell session "ord-bravo" exits with status 1
    And the state database session "ord-charlie" has status "idle" 10 seconds ago
    And the state database session "ord-delta" has status "waiting" 30 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "waiting"
    And within one configured reconcile interval deck client "A" row "ord-bravo" contains "error"
    # sort_order = activity ranks purely by status_at descending, so
    # ord-bravo's genuine crash (whose status_at otherwise lands wherever
    # the crash-collecting reconcile tick actually ran, in real wall-clock
    # time) must still be pinned to this fixture's engineered age. By this
    # point ord-bravo's pane is already dead and gone -- reconcile's
    # crash-collection already killed it (repairTerminalRowWithLivePane's
    # own doc comment) -- so this raw status_at-only write (status and
    # status_source untouched) can never race a live-pane repair the way a
    # raw status write would (task 804).
    And the state database session "ord-bravo" has status_at 40 seconds ago
    And deck client "A" screen shows sessions in this order:
      | ord-charlie |
      | ord-alpha   |
      | ord-delta   |
      | ord-bravo   |
    When deck client "A" exits cleanly

  @requirement-53-sort-order-name
  Scenario: sort_order = name orders case-insensitively ascending
    Given the scenario's config.toml is written with:
      """
      [ui]
      sort_order = "name"
      """
    And deck client "A" is started
    When deck client "A" creates shell session "ord-alpha"
    And deck client "A" creates shell session "ord-bravo"
    And deck client "A" creates shell session "ord-charlie"
    And deck client "A" creates shell session "ord-delta"
    And the state database session "ord-alpha" has created_at 35 seconds ago
    And the state database session "ord-bravo" has created_at 5 seconds ago
    And the state database session "ord-charlie" has created_at 25 seconds ago
    And the state database session "ord-delta" has created_at 15 seconds ago
    And the state database session "ord-alpha" has status "running" 20 seconds ago
    # "error" is posed here via a genuine pane exit, not a raw database
    # write, so the row's PaneExitStatus is actually set: SPEC section 7's
    # self-heal (internal/service.reconcile's repairTerminalRowWithLivePane)
    # narrows to a stopped row, or an error row that itself already carries
    # a pane-exit or tmux/user-sourced verdict, paired with a live pane -- a
    # bare hook/probe-sourced error with no such verdict is left alone
    # (finding F40, task 901). "ord-bravo"'s pane is genuinely dead by the time
    # this step returns, so reconcile's crash-collection (not the live-pane
    # repair) is what makes the row's "error" the real tmux.pane_dead
    # transition (task 804).
    And shell session "ord-bravo" exits with status 1
    And the state database session "ord-charlie" has status "idle" 10 seconds ago
    And the state database session "ord-delta" has status "waiting" 30 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "waiting"
    And within one configured reconcile interval deck client "A" row "ord-bravo" contains "error"
    And deck client "A" screen shows sessions in this order:
      | ord-alpha   |
      | ord-bravo   |
      | ord-charlie |
      | ord-delta   |
    When deck client "A" exits cleanly

  @requirement-53-sort-order-live-apply
  Scenario: cycling sort_order in the settings takeover and saving with ctrl+s keeps the same session selected even though its row index moves (requirement R53)
    # Same shared fixture as the four scenarios above: under attention
    # ord-alpha sits at index 2 (delta, bravo, alpha, charlie); under
    # created it moves to index 3 (bravo, delta, charlie, alpha). An
    # index-preserving (rather than id-preserving) live-apply would leave
    # the selection marker on row index 2, which under "created" is
    # ord-charlie -- a different session -- so this fixture fails an
    # index-preserving implementation instead of passing it by accident.
    Given deck client "A" is started
    When deck client "A" creates shell session "ord-alpha"
    And deck client "A" creates shell session "ord-bravo"
    And deck client "A" creates shell session "ord-charlie"
    And deck client "A" creates shell session "ord-delta"
    And the state database session "ord-alpha" has created_at 35 seconds ago
    And the state database session "ord-bravo" has created_at 5 seconds ago
    And the state database session "ord-charlie" has created_at 25 seconds ago
    And the state database session "ord-delta" has created_at 15 seconds ago
    And the state database session "ord-alpha" has status "running" 20 seconds ago
    # "error" is posed here via a genuine pane exit, not a raw database
    # write, so the row's PaneExitStatus is actually set: SPEC section 7's
    # self-heal (internal/service.reconcile's repairTerminalRowWithLivePane)
    # narrows to a stopped row, or an error row that itself already carries
    # a pane-exit or tmux/user-sourced verdict, paired with a live pane -- a
    # bare hook/probe-sourced error with no such verdict is left alone
    # (finding F40, task 901). "ord-bravo"'s pane is genuinely dead by the time
    # this step returns, so reconcile's crash-collection (not the live-pane
    # repair) is what makes the row's "error" the real tmux.pane_dead
    # transition (task 804).
    And shell session "ord-bravo" exits with status 1
    And the state database session "ord-charlie" has status "idle" 10 seconds ago
    And the state database session "ord-delta" has status "waiting" 30 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "waiting"
    And within one configured reconcile interval deck client "A" row "ord-bravo" contains "error"
    And deck client "A" screen shows sessions in this order:
      | ord-delta   |
      | ord-bravo   |
      | ord-alpha   |
      | ord-charlie |
    When deck client "A" selects session "ord-alpha"
    And deck client "A" sends ","
    And deck client "A" sends "j"
    And deck client "A" sends "	"
    And deck client "A" sends "j"
    And deck client "A" sends "j"
    And deck client "A" sends "j"
    And deck client "A" sends "j"
    And deck client "A" sends "j"
    And deck client "A" sends "j"
    Then deck client "A" screen contains "Sort Order: attention"
    When deck client "A" sends "+"
    Then deck client "A" screen contains "Sort Order: created"
    When deck client "A" sends ""
    Then deck client "A" screen contains "saved "
    When deck client "A" sends ""
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" screen shows sessions in this order:
      | ord-bravo   |
      | ord-delta   |
      | ord-charlie |
      | ord-alpha   |
    And deck client "A" has session "ord-alpha" selected
    When deck client "A" exits cleanly

  @requirement-53-sort-order-with-grouping
  Scenario: group_by_workspace (on by default) composes with a non-attention sort_order -- group order stays attention-led, rows within each group follow the chosen order (requirement R53)
    # `[ui] group_by_workspace` defaults to true (internal/config/schema.go)
    # and is not touched by this scenario. Two workspaces, two sessions
    # each:
    #   gso-beta-ws:  gso-bb (waiting), gso-ba (running)
    #   gso-alpha-ws: gso-ab (error),   gso-aa (idle)
    # Attention full order (waiting > error > running > idle):
    #   gso-bb, gso-ab, gso-ba, gso-aa
    # so gso-beta-ws's most urgent member (gso-bb, waiting) outranks
    # gso-alpha-ws's (gso-ab, error) -- gso-beta-ws leads the group order,
    # even though "gso-alpha-ws" and its lead member "gso-aa"/"gso-ab" sort
    # alphabetically first. A group-order rule that (wrongly) derives group
    # leadership from the CHOSEN order's own first-appearance instead of
    # from attention would put gso-alpha-ws first here -- this fixture
    # catches that.
    #
    # Under sort_order = name, each group's ROWS reorder to
    # case-insensitive-ascending, which disagrees with that group's own
    # attention order in both groups:
    #   gso-beta-ws:  attention gives bb, ba  -- name gives ba, bb
    #   gso-alpha-ws: attention gives ab, aa  -- name gives aa, ab
    # So the fully expected render (group order gso-beta-ws then
    # gso-alpha-ws, name order within each) is:
    #   gso-ba, gso-bb, gso-aa, gso-ab
    Given the scenario's config.toml is written with:
      """
      [ui]
      sort_order = "name"
      """
    And deck client "A" is started
    When deck client "A" creates shell session "gso-aa"
    And deck client "A" creates shell session "gso-ab"
    And deck client "A" creates shell session "gso-ba"
    And deck client "A" creates shell session "gso-bb"
    And the state database session "gso-aa" has workspace "gso-alpha-ws"
    And the state database session "gso-ab" has workspace "gso-alpha-ws"
    And the state database session "gso-ba" has workspace "gso-beta-ws"
    And the state database session "gso-bb" has workspace "gso-beta-ws"
    And the state database session "gso-bb" has status "waiting" 40 seconds ago
    # "error" is posed here via a genuine pane exit, not a raw database
    # write, so the row's PaneExitStatus is actually set: SPEC section 7's
    # self-heal (internal/service.reconcile's repairTerminalRowWithLivePane)
    # narrows to a stopped row, or an error row that itself already carries
    # a pane-exit or tmux/user-sourced verdict, paired with a live pane -- a
    # bare hook/probe-sourced error with no such verdict is left alone
    # (finding F40, task 901). "gso-ab"'s pane is genuinely dead by the time
    # this step returns, so reconcile's crash-collection (not the live-pane
    # repair) is what makes the row's "error" the real tmux.pane_dead
    # transition (task 804).
    And shell session "gso-ab" exits with status 1
    And the state database session "gso-ba" has status "running" 20 seconds ago
    And the state database session "gso-aa" has status "idle" 10 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "waiting"
    And within one configured reconcile interval deck client "A" row "gso-ab" contains "error"
    And deck client "A" screen shows sessions in this order:
      | gso-ba      |
      | gso-bb      |
      | gso-aa      |
      | gso-ab      |
    And deck client "A" screen contains "gso-beta-ws"
    And deck client "A" screen contains "gso-alpha-ws"
    When deck client "A" exits cleanly
