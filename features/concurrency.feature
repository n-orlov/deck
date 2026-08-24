@multiclient
Feature: Multi-client session refresh
  Independent deck clients sharing one home and private tmux socket converge on
  durable session changes without a restart.

  Scenario: create and kill propagate and surviving clients tolerate a peer crash
    Given deck client "A" is started
    And deck client "B" is started
    And deck client "C" is started
    When deck client "A" creates shell session "shared session"
    Then within one configured reconcile interval deck client "B" screen contains "shared session"
    And within one configured reconcile interval deck client "C" screen contains "shared session"
    When deck client "B" kills its selected session
    Then within one configured reconcile interval deck client "A" screen contains "resumable"
    And within one configured reconcile interval deck client "C" screen contains "resumable"
    And the state database contains session "shared session" with status "stopped"
    When deck client "C" is killed with SIGKILL
    And deck client "A" creates shell session "after crash"
    Then deck client "B" screen contains "after crash"
    And the state database contains session "shared session" with status "stopped"
    # The durable-row assertion protects creation after a peer SIGKILL; shell
    # promotion legitimately changes its transient status to running.
    And the state database contains session "after crash"
    When deck client "A" exits cleanly
    And deck client "B" exits cleanly

  Scenario: one hook-driven status change propagates to every client
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    And deck client "B" is started
    And deck client "C" is started
    When deck client "A" creates claude session "hook shared" with permission profile "safe"
    Then within one configured reconcile interval deck client "B" screen contains "hook shared"
    And within one configured reconcile interval deck client "C" screen contains "hook shared"
    When fake Claude session "hook shared" fires "SessionStart" for itself using conversation identity:
      | source | propagation |
    Then within one configured reconcile interval deck client "A" row "hook shared" contains "running"
    And within one configured reconcile interval deck client "B" row "hook shared" contains "running"
    And within one configured reconcile interval deck client "C" row "hook shared" contains "running"
    And session "hook shared" has one "session_start" event with payload field "source" equal to "propagation"
    When deck client "A" exits cleanly
    And deck client "B" exits cleanly
    And deck client "C" exits cleanly

  Scenario: a live client reconciles an externally killed private server
    Given deck client "A" is started
    When deck client "A" creates shell session "externally stopped"
    And the private tmux server is killed
    Then within one configured reconcile interval deck client "A" screen contains "resumable"
    And the state database contains session "externally stopped" with status "stopped"
    And the audit log contains event "tmux.session_gone" for a session
    And the private tmux session "deck_externally-stopped" does not exist
    When deck client "A" sends "?"
    And deck client "A" terminal is resized to 100x130
    # This is a smoke check that the help overlay still opens and renders
    # correctly after a reconcile-driven status change and a resize while
    # open -- not a check that the WHOLE help text is reachable (there is
    # no PgDn step here). "deck help"'s own title line is always the
    # first content row at helpScroll==0 (reset on every open, task 078),
    # so it is on screen regardless of overlay height or terminal size.
    # Previously this asserted "Runtime controls" (far down helpText),
    # which relied on the pre-task-078 unclipped overlay's own overflow
    # pushing its TAIL onto screen by accident at this terminal size --
    # task 078's real pagination (helpScroll now fixed at 0 on open)
    # shows the TOP instead, so that string is no longer reachable
    # without an explicit PgDn this scenario never sends.
    Then deck client "A" screen contains "deck help"
    When deck client "A" exits cleanly
