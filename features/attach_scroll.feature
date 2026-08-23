@attach-scroll
Feature: §3.2/§11.8 a wheel notch in an attached session scrolls the pane, not the shell (requirement 48)
  deck's private tmux server is bootstrapped with `mouse on` (task 115) so that
  a wheel notch delivered to an attached pane scrolls its scrollback via
  tmux's own copy-mode, instead of the outer terminal's alternate-scroll
  translating it into an Up/Down arrow that a shell reads as history recall.
  This proves the behaviour against real tmux -- the observable is pane
  content, never the `mouse` option's own value.

  @requirement-48-wheel-scrolls-attached-pane-without-typing
  Scenario: a wheel notch scrolls an attached pane's scrollback and leaves the shell's input line untouched
    Given deck client "A" is started
    When deck client "A" creates shell session "attach-scroll-target"
    And deck client "A" attaches to the selected session
    And deck client "A" fills the attached pane with more than one screen of scrollback
    And deck client "A" captures its frame as "before-wheel-scroll"
    And deck client "A" scrolls the wheel up 30 times over the attached pane at column 20 row 15
    Then deck client "A" attached pane shows the top of the scrollback
    When deck client "A" exits copy-mode on the attached pane
    Then deck client "A" frame still matches the captured "before-wheel-scroll" frame
    When deck client "A" detaches
    Then deck client "A" screen contains "deck - sessions"
    When deck client "A" exits cleanly

  # requirement 49: an experiment recorded at
  # docs/reports/phase3-task117-capture-pane-copy-mode-experiment.log shows
  # `capture-pane -p -S ... -E -` addresses the pane's real screen+history
  # buffer by absolute line number and never consults any attached client's
  # own copy-mode scroll offset -- scrolling is purely client-local state.
  # This scenario proves the invariant end to end against the real probe
  # path (internal/service.Service.ReconcileWithProbes ->
  # internal/tmux.Client.CapturePane), not just against the raw primitive:
  # client "A" scrolls an attached pane back to an old, superseded fixture
  # render while the pane's real (live) bottom already carries a newer one,
  # and both the durable probe verdict and a second, never-attached client
  # "B"'s own sidebar row are asserted to reflect the live content while "A"
  # is still sitting in copy-mode looking at the old one.
  @requirement-49-scrolling-does-not-flip-the-badge
  Scenario: scrolling an attached pane never flips its badge, and a stale probe reads the pane's live bottom, not the scrolled-back view
    Given probe fixture agents for attach-scroll are configured
    And deck client "A" is started
    And deck client "B" is started
    When deck client "A" creates claude session "sp-claude" with permission profile "safe"
    And fake agent session "sp-claude" renders golden fixture "claude/waiting.txt"
    And deck client "A" attaches to the selected session
    And fake agent session "sp-claude" renders these exact golden fixtures:
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
    And deck client "A" scrolls the wheel up 40 times over the attached pane at column 20 row 15 until it shows "Do you want to proceed?"
    Then deck client "A" attached pane shows "Do you want to proceed?"
    And the state database session "sp-claude" has probe status "error" with reason "api error"
    And within one configured reconcile interval deck client "B" row "sp-claude" contains "sampled"
    When deck client "A" exits copy-mode on the attached pane
    And deck client "A" detaches
    When deck client "A" exits cleanly
    And deck client "B" exits cleanly
