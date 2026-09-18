Feature: Interactive mode's own bounded scrollback (Part II, requirement 51)

  The grid keeps its own bounded scrollback, and the wheel and
  Shift+PgUp/PgDn scroll it (docs/reports/phase3b.md's II-51 section,
  internal/interactive.ScrollbackMaxLines). This proves the scrolling
  itself against a real tmux pane, real deck client and real terminal
  input, not merely the internal RenderRows/offset arithmetic
  internal/interactive's own unit tests already cover: content genuinely
  scrolled off the live screen becomes visible again once scrolled back
  to, by both input methods, and disappears again once scrolled back down
  to the live view.

  Issue #29 adds the case the requirement always implied but no scenario
  stated: the content that scrolled off need not have been produced while
  deck was watching. A session that ran for a while with no preview open
  has all of that output in tmux's own pane history and none of it in any
  deck grid, so entering interactive preview and pressing Shift+PgUp used
  to reach NOTHING -- the grid's scrollback started empty. The entry seed
  now pulls that history in (internal/interactive.CaptureSeedWithHistory),
  which is also why no scenario here may state a FIXED number of pages
  back to a marker any more: how far back the live bottom is depends on
  the pane's own past, so each one scrolls until the marker appears within
  a bounded number of pages and then mirrors exactly that many pages
  forward.

  The GRANULARITY those fixed counts used to prove is not given up with
  them, because nothing else in the repo proves it: the search step also
  asserts it needed only a handful of pages, which a press that moved one
  line (or one wheel notch) instead of a whole page cannot satisfy even
  though it would still reach the marker in the end. See
  features/interactive_scroll_test.go's shiftPageScrollMaxPagesToMarker,
  and internal/tui's own TestShiftPageScrollStepsTheWholePreviewContentHeight
  for the same claim asserted directly against the offset arithmetic.

  @requirement-51-bounded-scrollback
  Scenario: Shift+PgUp/PgDn scroll the interactive grid's own scrollback
    Given deck client "A" is started
    When deck client "A" creates shell session "scroll-target"
    Then deck client "A" screen contains "scroll-target"
    And deck client "A" selects session "scroll-target"
    When deck client "A" enters interactive mode
    And deck client "A" types a 60-line numbered loop labelled "INTERACTIVE_SCROLL_LINE" into the interactive pane
    Then deck client "A" screen contains "INTERACTIVE_SCROLL_LINE_60"
    And deck client "A" screen does not contain "INTERACTIVE_SCROLL_LINE_1 "
    When deck client "A" scrolls back with shift+pgup until the screen contains "INTERACTIVE_SCROLL_LINE_1 "
    Then deck client "A" screen contains "INTERACTIVE_SCROLL_LINE_1 "
    When deck client "A" scrolls forward with shift+pgdown by the same number of pages
    Then deck client "A" screen does not contain "INTERACTIVE_SCROLL_LINE_1 "
    And deck client "A" screen contains "INTERACTIVE_SCROLL_LINE_60"
    When deck client "A" leaves interactive mode
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" exits cleanly

  # issue #29's own report, end to end: the 60 lines here are printed
  # straight into the session's real tmux pane by tmux send-keys, with deck
  # sitting in its ordinary sessions view -- never in interactive mode, so
  # no grid, no pipe-pane and no seed of any kind existed while they were
  # produced. The tmux-level control step is what makes the Shift+PgUp
  # assertion mean something: the marker line must be in the pane's
  # scrolled-off history and NOT on its visible screen, so a build that
  # seeds only the visible screen (every build before this one) cannot have
  # the marker anywhere in the grid to find. Scrolling back down again is
  # asserted too, so this cannot pass by leaving the view stuck in
  # scrollback.
  @requirement-51-bounded-scrollback @issue-29-pre-entry-history
  Scenario: Shift+PgUp reaches output the pane printed before deck entered interactive mode
    Given deck client "A" is started
    When deck client "A" creates shell session "pre-entry-scroll"
    Then deck client "A" screen contains "pre-entry-scroll"
    And deck client "A" selects session "pre-entry-scroll"
    When 60 numbered lines labelled "PRE_ENTRY_LINE" are printed into session "pre-entry-scroll"'s pane before deck enters interactive mode
    Then session "pre-entry-scroll"'s pane holds "PRE_ENTRY_LINE_1" in its tmux history but not on its visible screen
    And deck client "A" screen does not contain "PRE_ENTRY_LINE_1 "
    When deck client "A" enters interactive mode
    Then deck client "A" screen contains "PRE_ENTRY_LINE_60"
    And deck client "A" screen does not contain "PRE_ENTRY_LINE_1 "
    When deck client "A" scrolls back with shift+pgup until the screen contains "PRE_ENTRY_LINE_1 "
    Then deck client "A" screen contains "PRE_ENTRY_LINE_1 "
    When deck client "A" scrolls forward with shift+pgdown by the same number of pages
    Then deck client "A" screen does not contain "PRE_ENTRY_LINE_1 "
    And deck client "A" screen contains "PRE_ENTRY_LINE_60"
    When deck client "A" leaves interactive mode
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" exits cleanly

  @requirement-51-bounded-scrollback
  Scenario: the mouse wheel over the preview panel scrolls the interactive grid's own scrollback
    Given deck client "A" is started
    When deck client "A" creates shell session "wheel-scroll-target"
    Then deck client "A" screen contains "wheel-scroll-target"
    And deck client "A" selects session "wheel-scroll-target"
    When deck client "A" enters interactive mode
    And deck client "A" types a 60-line numbered loop labelled "WHEEL_SCROLL_LINE" into the interactive pane
    Then deck client "A" screen contains "WHEEL_SCROLL_LINE_60"
    And deck client "A" screen does not contain "WHEEL_SCROLL_LINE_1 "
    When deck client "A" scrolls the interactive wheel up 20 times over the line containing "WHEEL_SCROLL_LINE_60"
    Then deck client "A" screen contains "WHEEL_SCROLL_LINE_1 "
    When deck client "A" scrolls the interactive wheel down 20 times over the line containing "WHEEL_SCROLL_LINE_1 "
    Then deck client "A" screen does not contain "WHEEL_SCROLL_LINE_1 "
    When deck client "A" leaves interactive mode
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" exits cleanly

  @requirement-51-bounded-scrollback
  Scenario: typing while scrolled back snaps the view back to the live bottom
    Given deck client "A" is started
    When deck client "A" creates shell session "scroll-snap-target"
    Then deck client "A" screen contains "scroll-snap-target"
    And deck client "A" selects session "scroll-snap-target"
    When deck client "A" enters interactive mode
    And deck client "A" types a 60-line numbered loop labelled "SNAP_SCROLL_LINE" into the interactive pane
    Then deck client "A" screen contains "SNAP_SCROLL_LINE_60"
    When deck client "A" scrolls back with shift+pgup until the screen contains "SNAP_SCROLL_LINE_1 "
    Then deck client "A" screen contains "SNAP_SCROLL_LINE_1 "
    When deck client "A" types "echo AFTER_SNAP" and Enter into the interactive pane
    Then deck client "A" screen contains "AFTER_SNAP"
    And deck client "A" screen does not contain "SNAP_SCROLL_LINE_1 "
    When deck client "A" leaves interactive mode
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" exits cleanly

  # requirement 52 (task 069): the grid's own scroll position
  # (m.interactiveScrollOffset, internal/tui/interactive_scroll.go) is
  # purely local UI state -- it must never leak into what deck reports for
  # the session. grep proves the probe path never consults it:
  # interactiveScrollOffset/RenderRows/ScrollbackMaxLines appear only in
  # internal/tui and internal/interactive, never in internal/service
  # (Service.ReconcileWithProbes, the probe path), internal/store or
  # internal/tmux. This scenario proves the same invariant end to end
  # against the real product path, modelled on Phase 3's requirement 49
  # evidence (docs/reports/phase3-task117-capture-pane-copy-mode-
  # experiment.log, which showed the analogous invariant for tmux's own
  # copy-mode scroll position): client "A" scrolls the interactive grid
  # back to old, superseded fixture content while the pane's real (live)
  # bottom already carries a newer one, and both the durable probe verdict
  # and a second, never-attached client "B"'s own sidebar row are asserted
  # to reflect the live content while "A" is still scrolled back inside
  # interactive mode looking at the old one.
  @requirement-52-scrolling-the-grid-does-not-flip-the-badge
  Scenario: scrolling the interactive grid's own scrollback never flips the session's badge, and the probe reads the pane's live bottom, not the scrolled-back view
    Given probe fixture agents for interactive-scroll are configured
    And deck client "A" is started
    And deck client "B" is started
    When deck client "A" creates claude session "ig-claude" with permission profile "safe"
    Then deck client "A" screen contains "ig-claude"
    When deck client "A" enters interactive mode
    And fake agent session "ig-claude" renders golden fixture "claude/waiting.txt"
    Then deck client "A" screen contains "Do you want to proceed?"
    When fake agent session "ig-claude" renders these exact golden fixtures:
      | claude/running.txt |
      | claude/running.txt |
      | claude/running.txt |
      | claude/running.txt |
      | claude/running.txt |
      | claude/running.txt |
      | claude/running.txt |
      | claude/running.txt |
      | claude/running.txt |
      | claude/running.txt |
      | claude/error.txt   |
    Then deck client "A" screen contains "API Error"
    And deck client "A" screen does not contain "Do you want to proceed?"
    When deck client "A" scrolls back with shift+pgup until the screen contains "Do you want to proceed?"
    Then deck client "A" screen contains "Do you want to proceed?"
    And the state database session "ig-claude" has probe status "error" with reason "api error"
    And within several probe/repair cycles deck client "B" row "ig-claude" contains "sampled"
    When deck client "A" scrolls forward with shift+pgdown by the same number of pages
    Then deck client "A" screen does not contain "Do you want to proceed?"
    And deck client "A" screen contains "API Error"
    When deck client "A" leaves interactive mode
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" exits cleanly
    And deck client "B" exits cleanly
