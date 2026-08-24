Feature: Interactive mode announces a target that has not repainted since the resize (Part II, requirement 49)

  Entering interactive mode always resizes the target window first
  (enterInteractive's FitWindowToPane). Requirement 49 names the realistic
  worst case: a target waiting on a network round-trip that produces no
  output at all in response. Without an explicit announcement, that leaves
  the panel showing an empty bordered frame with no way to tell "the
  target hasn't repainted yet" apart from "deck itself is broken". Task
  026's FAKE_CLAUDE_REPAINT_MODE fixture gives this a deterministic,
  observable stand-in for both the broken case ("never") and a working one
  ("sigwinch"), so the panel's own announcement can be demonstrated red
  without it and green with it, and distinguished from a working session.

  @requirement-49-panel-announces-stale-frame
  Scenario: the panel announces a target that never repaints, even after typing
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "stale" is started
    When deck client "stale" creates claude session "stale-target" with permission profile "safe" and env "FAKE_CLAUDE_REPAINT_MODE=never"
    Then deck client "stale" screen contains "stale-target"
    And deck client "stale" selects session "stale-target"
    When deck client "stale" enters interactive mode
    Then deck client "stale" screen contains "has not repainted since the resize"
    And deck client "stale" screen does not contain "repaint #"
    When deck client "stale" sends "a"
    Then deck client "stale" screen contains "has not repainted since the resize"
    And deck client "stale" screen does not contain "repaint #"
    When deck client "stale" leaves interactive mode
    Then deck client "stale" screen contains "deck - sessions"
    And deck client "stale" exits cleanly

  @requirement-49-panel-announces-stale-frame
  Scenario: the panel does not announce a working target that repaints on resize
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "live" is started
    When deck client "live" creates claude session "live-target" with permission profile "safe" and env "FAKE_CLAUDE_REPAINT_MODE=sigwinch"
    Then deck client "live" screen contains "live-target"
    And deck client "live" selects session "live-target"
    When deck client "live" enters interactive mode
    Then deck client "live" screen contains "repaint #"
    And deck client "live" screen does not contain "has not repainted since the resize"
    When deck client "live" leaves interactive mode
    Then deck client "live" screen contains "deck - sessions"
    And deck client "live" exits cleanly
