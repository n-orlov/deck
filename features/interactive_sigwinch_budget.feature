Feature: Interactive mode's SIGWINCH budget (Part II, requirement 11)

  A full enter/exit cycle -- capture the window's own geometry, fit the
  window (never the pane) to the interactive size, then restore it on
  exit -- must cost the target exactly two SIGWINCH: one to enter, one to
  leave. Not three. This holds whether or not a real client stays attached
  to the session throughout the whole cycle.

  Both settle pauses below are load-bearing, not decorative: the fake
  fixture's own SIGWINCH counter (cmd/fake-claude/main.go) notifies on a
  channel with a buffer of exactly 1 (Go's os/signal contract: a signal
  arriving while the channel is already full is silently dropped, never
  queued). Back-to-back resizes with no pause between them can therefore
  undercount even when tmux really delivered two real kernel SIGWINCH --
  measured here: without a pause between enter and exit, this scenario's
  own "exactly 1" red control (enter fires, exit's SIGWINCH lands before
  the fixture's goroutine has drained the first) passed when it should
  have failed, because the second signal was coalesced away before the
  fixture ever counted it. sigwinch_count_test.go's own 50ms inter-resize
  pacing exists for the identical reason. The pause before the final
  assertion is separate and guards the read itself: this scenario starts no
  deck client, so the count step's own settle (R65: quiesce every pty
  client, then read once and compare for equality) has nothing to wait on
  here, and this pause is what gives the exit's SIGWINCH time to be counted
  before that single read.

  Scenario: a full enter/exit cycle costs exactly two SIGWINCH with nobody attached
    Given a fake "claude" agent occupies a bare tmux session "winch-detached" at 80x24
    When deck enters interactive mode on tmux session "winch-detached" fitting to 45x15
    And 200 milliseconds pass
    And deck exits interactive mode on tmux session "winch-detached"
    And 200 milliseconds pass
    Then the fake "claude" agent received exactly 2 SIGWINCH signals

  Scenario: a full enter/exit cycle costs exactly two SIGWINCH with a client attached throughout
    Given a fake "claude" agent occupies a bare tmux session "winch-attached" at 80x24
    # 80x25 raw, minus the attaching client's own one-row status line, is
    # 80x24 -- identical to the window's existing size, so this attach
    # costs no SIGWINCH of its own: the whole two-signal budget below
    # belongs to the enter/exit cycle alone, not to attaching.
    And a real tmux client attaches to session "winch-attached" at 80x25
    When deck enters interactive mode on tmux session "winch-attached" fitting to 45x15
    And 200 milliseconds pass
    And deck exits interactive mode on tmux session "winch-attached"
    And 200 milliseconds pass
    Then the fake "claude" agent received exactly 2 SIGWINCH signals
    And the real tmux client attached to session "winch-attached" detaches
