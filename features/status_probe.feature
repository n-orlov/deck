@status-probe
Feature: Sampled probe status truth
  Pane sampling uses the same captured fixture corpus as the adapter goldens and
  never presents a sampled verdict as a live hook verdict.

  Background:
    Given probe fixture agents and an advanceable frozen clock are configured
    And deck client "A" is started

  Scenario: Real fake-agent panes render the complete probe golden corpus
    When deck client "A" creates claude session "corpus claude" with permission profile "safe"
    And deck client "A" creates pi session "corpus pi" with permission profile "safe"
    Then fake agent session "corpus claude" renders these exact golden fixtures:
      | claude/starting.txt |
      | claude/running.txt  |
      | claude/waiting.txt  |
      | claude/idle.txt     |
      | claude/error.txt    |
    # pi's "starting" and "waiting" still have no golden fixture: SPEC
    # requirement 38's refit found no capturable, durable marker for them
    # against a real pi (see internal/agent/testdata/probes/pi-PROVENANCE.md).
    # pi's "idle" fixture (internal/agent/testdata/probes/pi/idle.txt) was
    # added later (task 037 / operator steer 003-pi-idle-rule.md); it is not
    # rendered in this scenario's list, but it is covered by
    # internal/agent/probe_test.go's golden-corpus table and rule-ordering
    # tests, which pin its verdict and its precedence against the pi
    # running/error rules.
    And fake agent session "corpus pi" renders these exact golden fixtures:
      | pi/running.txt |
      | pi/error.txt   |
    When deck client "A" exits cleanly

  Scenario: Stale sampling is visible, precedence-aware, and agent-only
    When deck client "A" creates claude session "raced claude" with permission profile "safe"
    And deck client "A" creates claude session "stale claude" with permission profile "safe"
    And deck client "A" creates claude session "hook emitter" with permission profile "safe"
    And deck client "A" creates pi session "sampled pi" with permission profile "safe"
    And deck client "A" creates persistent shell session "probe shell"
    And fake agent session "raced claude" renders golden fixture "claude/waiting.txt"
    And fake Claude session "stale claude" fires "SessionStart" for itself using conversation identity:
      | source | fresh |
    And fake Claude session "stale claude" fires "Notification" for itself using conversation identity:
      | notification_type | permission_prompt |
    And fake agent session "stale claude" renders golden fixture "claude/running.txt"
    And fake agent session "sampled pi" renders golden fixture "pi/error.txt"
    Then the probe event count for session "raced claude" is 0
    And the state database session "raced claude" has status "starting" from "tmux"
    And the state database session "stale claude" has status "waiting" from "hook"
    And the state database session "sampled pi" has status "starting" from "tmux"

    When the frozen clock advances across stale_after while a fresh hook races the next probe of "raced claude" from "hook emitter"
    Then the state database session "raced claude" has status "running" from "hook"
    And session "raced claude" has one losing "probe.waiting" event
    And within one configured reconcile interval deck client "A" row "raced claude" contains "live"
    And the state database session "stale claude" has probe status "running" with reason "working indicator"
    And within one configured reconcile interval deck client "A" row "stale claude" contains "sampled"
    And the state database session "sampled pi" has probe status "error" with reason "agent error"
    And within one configured reconcile interval deck client "A" row "sampled pi" contains "sampled"
    And the state database session "probe shell" has status "running" from "tmux"
    And the probe event count for session "probe shell" is 0
    And deck client "A" row "probe shell" does not contain "sampled"
    When deck client "A" exits cleanly
