@mouse-bindings
Feature: §11.8 mouse bindings and the [ui] mouse / DECK_MOUSE opt-out (requirements 33-37, 41)
  Every gesture below duplicates an existing key -- none is the only way to
  perform its action -- and hit-testing resolves each click through the
  same layout the renderer itself drew from (task 028). With mouse
  reporting off, every one of these gestures is a no-op and only the
  shortcut is lost; the keyboard path underneath keeps working.

  @requirement-33-click-selects-and-enters-interactive-mode
  Scenario: a single click on a sidebar row selects it and enters interactive mode on the same press, and Ctrl+Q returns to the list
    # task 311/312 (R55): the double-click gate is gone -- one press now
    # does what task 061's Enter keybinding does, so this scenario asserts
    # BOTH halves of that one press (selection moved AND interactive
    # entered), not just one of them, on a row that was not already
    # selected (click-enter-bravo, the second-created session, never the
    # default selection).
    Given deck client "A" is started
    When deck client "A" creates shell session "click-enter-alpha"
    And deck client "A" creates shell session "click-enter-bravo"
    And within one configured reconcile interval deck client "A" screen contains "running"
    And deck client "A" clicks on the row containing "click-enter-bravo"
    Then deck client "A" has session "click-enter-bravo" selected
    And deck client "A" screen contains "click-enter-bravo"
    And deck client "A" screen contains "interactive"
    And deck client "A" screen contains "Ctrl+Q"
    And deck client "A" captures its frame as "click-entered-bravo"
    When deck client "A" leaves interactive mode
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" has session "click-enter-bravo" selected
    When deck client "A" enters interactive mode
    Then deck client "A" frame still matches the captured "click-entered-bravo" frame
    When deck client "A" leaves interactive mode
    And deck client "A" exits cleanly

  @requirement-33-double-click-enters-interactive-mode
  Scenario: a double click on a sidebar row also enters interactive mode, and its second press lands harmlessly on the now-interactive preview
    # Rewritten for task 312: the first press of the double click already
    # enters interactive mode (task 311), so the second press's coordinate
    # -- unchanged, since DoubleClick reuses the row's original column/row
    # -- now lands over the interactive preview, not the sidebar. Task 313
    # (still pending) is what will re-target a sidebar click while
    # interactive; until then this second press is exactly the "plain
    # click, no drag, over the interactive preview" case
    # features/interactive_selection.feature already covers -- it commits
    # no selection and raises no error. This scenario is the harness's
    # proof that a double click is not silently broken (no crash, no
    # attachError, no double-attach) by task 311's change, keeping the
    # double-click synthesis step (DoubleClick/clientDoubleClicksOnRowContaining)
    # genuinely exercised rather than merely still compiling.
    Given deck client "A" is started
    When deck client "A" creates shell session "dbl-click-enter"
    And within one configured reconcile interval deck client "A" screen contains "running"
    And deck client "A" double-clicks on the row containing "dbl-click-enter"
    Then deck client "A" screen contains "dbl-click-enter"
    And deck client "A" screen contains "interactive"
    And deck client "A" screen contains "Ctrl+Q"
    And deck client "A" screen does not contain "Cannot enter interactive mode"
    When deck client "A" leaves interactive mode
    Then deck client "A" screen contains "deck - sessions"
    When deck client "A" exits cleanly

  @requirement-54-sidebar-click-retargets-interactive-mode
  Scenario: a sidebar click on a different row while interactive re-targets it, restoring the old session's window
    # SPEC post-6584299: "a sidebar click works while interactive mode is
    # active and re-targets it, leaving the old window and entering the
    # new one" (task 313's product change). retarget-first's tmux window
    # is captured BEFORE it is ever touched by interactive mode, so a
    # match after the retarget proves exitInteractive's restore actually
    # ran -- an implementation that fits the window on entry and never
    # restores it on retarget would leave it at the fitted size, not the
    # baseline, and fail this exact assertion. The preview's top border
    # (not "screen contains", which the sidebar's own row list would
    # satisfy regardless of which session is the interactive target) is
    # SPEC's own named safeguard for "which pane is receiving your
    # keystrokes".
    Given deck client "A" is started
    When deck client "A" creates shell session "retarget-first"
    And deck client "A" creates shell session "retarget-second"
    And within one configured reconcile interval deck client "A" screen contains "running"
    And the private tmux window for session "retarget-first" is captured as "retarget-first-baseline"
    And deck client "A" selects session "retarget-first"
    And deck client "A" enters interactive mode
    Then deck client "A" preview top border contains "retarget-first"
    When deck client "A" clicks on the row containing "retarget-second"
    Then deck client "A" has session "retarget-second" selected
    And deck client "A" preview top border contains "retarget-second"
    And the private tmux window for session "retarget-first" still matches "retarget-first-baseline"
    When deck client "A" leaves interactive mode
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" exits cleanly

  @requirement-54-sidebar-click-on-interactive-row-is-a-no-op
  Scenario: a sidebar click on the already-interactive row triggers no resize
    # SPEC post-6584299: "Clicking the already-interactive row is a no-op
    # rather than a leave-and-re-enter." A leave-then-re-enter that lands
    # the pane back at the SAME final size, with no attached client to
    # force a real reflow, produces no observable SIGWINCH or geometry
    # change either -- so the discriminator here is not the window's size
    # but its ownership claim (internal/tmux/ownership.go's own
    # @deck_isize_owner window option): ClaimWindowOwnership writes a
    # fresh, cryptographically random tag on every single claim, including
    # a same-pid re-claim of a window it just released, so a real
    # leave-then-re-enter always rewrites it and a genuine no-op never
    # touches it at all.
    #
    # A single already-interactive session is not enough to discriminate
    # task 313's absence on its own: pre-313 code forwards EVERY press while
    # interactive straight to task 216's drag-to-copy switch, so a click
    # anywhere over the sidebar (targeted row or not) is already a no-op by
    # that unrelated, pre-existing mechanism -- an ownership-unchanged
    # assertion taken in isolation would pass whether or not 313's retarget
    # feature exists at all. So this scenario first retargets from session A
    # onto session B with a click (a step that only succeeds because 313's
    # hit-test-the-press-first code path exists, exactly like the scenario
    # above), and only THEN clicks B's own row again to exercise the no-op
    # branch: if 313 is reverted, the retarget click never moves the
    # selection off A, so the very next assertion (B selected) already
    # fails red before the no-op check is ever reached.
    Given deck client "A" is started
    When deck client "A" creates shell session "retarget-noop-a"
    And deck client "A" creates shell session "retarget-noop-b"
    And within one configured reconcile interval deck client "A" screen contains "running"
    And deck client "A" selects session "retarget-noop-a"
    And deck client "A" enters interactive mode
    Then deck client "A" preview top border contains "retarget-noop-a"
    When deck client "A" clicks on the row containing "retarget-noop-b"
    Then deck client "A" has session "retarget-noop-b" selected
    And deck client "A" preview top border contains "retarget-noop-b"
    And the private tmux window ownership claim for session "retarget-noop-b" is captured as "retarget-noop-b-after-retarget"
    When deck client "A" clicks on the row containing "retarget-noop-b"
    Then deck client "A" has session "retarget-noop-b" selected
    And deck client "A" preview top border contains "retarget-noop-b"
    And the private tmux window ownership claim for session "retarget-noop-b" still matches "retarget-noop-b-after-retarget"
    When deck client "A" leaves interactive mode
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" exits cleanly

  @requirement-34-header-click-collapses
  Scenario: clicking a workspace group's header collapses only that group
    Given deck client "A" is started
    When deck client "A" creates shell session "header-click-default"
    And deck client "A" creates shell session "header-click-other"
    And the state database session "header-click-other" has workspace "header-click-workspace"
    Then deck client "A" screen contains "header-click-workspace"
    And deck client "A" screen contains "header-click-default"
    And deck client "A" screen contains "header-click-other"
    When deck client "A" clicks on the row containing "header-click-workspace"
    Then deck client "A" screen stops containing "header-click-other"
    And deck client "A" screen contains "header-click-default"
    And deck client "A" screen contains "header-click-workspace"
    When deck client "A" clicks on the row containing "header-click-workspace"
    Then deck client "A" screen contains "header-click-other"
    When deck client "A" exits cleanly

  @requirement-34-wheel-scrolls-without-selecting
  Scenario: the wheel scrolls the sidebar's view without changing selection
    Given deck client "A" is started
    When deck client "A" creates shell session "wheel-scroll-1"
    And deck client "A" creates shell session "wheel-scroll-2"
    And deck client "A" creates shell session "wheel-scroll-3"
    And deck client "A" creates shell session "wheel-scroll-4"
    And deck client "A" creates shell session "wheel-scroll-5"
    And the state database session "wheel-scroll-1" has status "idle" 50 seconds ago
    And the state database session "wheel-scroll-2" has status "idle" 40 seconds ago
    And the state database session "wheel-scroll-3" has status "idle" 30 seconds ago
    And the state database session "wheel-scroll-4" has status "idle" 20 seconds ago
    And the state database session "wheel-scroll-5" has status "idle" 10 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "idle"
    And deck client "A" sends "|"
    And deck client "A" sends "|"
    Then deck client "A" screen stops containing "wheel-scroll-5"
    When deck client "A" selects session "wheel-scroll-1"
    And deck client "A" scrolls the wheel down at column 5 row 5
    And deck client "A" scrolls the wheel down at column 5 row 5
    And deck client "A" scrolls the wheel down at column 5 row 5
    And deck client "A" scrolls the wheel down at column 5 row 5
    Then deck client "A" screen contains "wheel-scroll-5"
    When deck client "A" scrolls the wheel up at column 5 row 5
    And deck client "A" scrolls the wheel up at column 5 row 5
    And deck client "A" scrolls the wheel up at column 5 row 5
    And deck client "A" scrolls the wheel up at column 5 row 5
    Then deck client "A" screen contains "wheel-scroll-1"
    And deck client "A" has session "wheel-scroll-1" selected
    When deck client "A" exits cleanly

  @requirement-35-seam-drag-resizes
  Scenario: dragging the seam adjusts sidebar_width live
    Given deck client "A" is started
    When deck client "A" creates a long-named shell session "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxWIDENOWyyyyyyyyyy"
    Then deck client "A" screen does not contain "WIDENOW"
    When deck client "A" drags from column 36 row 6 to column 61 row 6
    Then deck client "A" screen contains "WIDENOW"
    When deck client "A" exits cleanly

  @requirement-33-preview-gesture-no-ops
  Scenario: clicking, double-clicking or scrolling over the PASSIVE preview panel does nothing
    # Scoped to passive preview (task 064/II-45): this session is never
    # selected into interactive mode, so the gestures below land on the
    # passive preview -- the panel that never accepts input outside
    # interactive mode. It says nothing about a click landing on the live
    # pane while interactive mode is active, which is a different surface
    # with its own forwarding contract (task 061).
    Given deck client "A" is started
    When deck client "A" creates shell session "preview-gesture-noop"
    And within one configured reconcile interval deck client "A" screen contains "running"
    And deck client "A" captures its frame as "before-preview-mouse-bindings"
    And deck client "A" clicks at column 70 row 15
    And deck client "A" double-clicks at column 70 row 15
    And deck client "A" scrolls the wheel up at column 70 row 15
    And deck client "A" scrolls the wheel down at column 70 row 15
    Then deck client "A" frame still matches the captured "before-preview-mouse-bindings" frame
    When deck client "A" exits cleanly

  @requirement-37-deck-mouse-disables-gestures @requirement-41-keyboard-still-works
  Scenario: DECK_MOUSE=0 disables every mouse gesture, and only the shortcut is lost
    Given the deck config disables mouse reporting
    And deck client "A" is started
    When deck client "A" creates shell session "mouse-off-alpha"
    And deck client "A" creates shell session "mouse-off-bravo"
    And the state database session "mouse-off-alpha" has status "idle" 20 seconds ago
    And the state database session "mouse-off-bravo" has status "idle" 10 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "idle"
    And deck client "A" captures its frame as "before-mouse-off-click"
    And deck client "A" clicks on the row containing "mouse-off-bravo"
    Then deck client "A" frame still matches the captured "before-mouse-off-click" frame
    When deck client "A" selects session "mouse-off-bravo"
    Then deck client "A" has session "mouse-off-bravo" selected
    When deck client "A" exits cleanly
