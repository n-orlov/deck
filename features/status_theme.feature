@status-theme-tokens
Feature: the seven §7 status tokens colour the sidebar's status word (task 014)
  SPEC §11.6 declares one theme token per §7 status (waiting, running, idle,
  starting, stopped, error, archived); the sidebar's rendered status word
  and its unseen-attention glyph must be coloured from that session's own
  status token, never borrowed from another status. This asserts each of
  the seven independently, per-cell, against the default theme's real
  authored colour for that token, on a REAL running deck client (not the
  swatch emulator requirement 4's harness steps paint) — the render path
  this task actually changed.

  An "anchor" session stays selected throughout so the preview panel never
  shows the "target" session's own status placeholder copy (e.g. "Session
  is archived. No live preview to show.") alongside the sidebar row: that
  placeholder text is plain, uncoloured copy, and if it ever collided with
  the coloured status word substring the per-cell assertion below would
  see a mix of coloured and uncoloured cells for the same substring and
  fail for the wrong reason.

  Background:
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started with colour enabled
    When deck client "A" creates shell session "tok-anchor"
    Then within one configured reconcile interval deck client "A" screen contains "running"
    When deck client "A" creates shell session "tok-target"
    And deck client "A" creates claude session "tok-agent" with permission profile "safe"
    And deck client "A" selects session "tok-anchor"

  @requirement-status-tokens
  Scenario: the waiting status token colours the waiting status word
    When the state database session "tok-target" has status "waiting" 5 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "waiting"
    And deck client "A" text "waiting" has foreground token "waiting"
    And deck client "A" exits cleanly

  @requirement-status-tokens
  Scenario: the running status token colours the running status word
    When the state database session "tok-target" has status "running" 5 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "running"
    And deck client "A" text "running" has foreground token "running"
    And deck client "A" exits cleanly

  @requirement-status-tokens
  Scenario: the idle status token colours the idle status word
    When the state database session "tok-target" has status "idle" 5 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "idle"
    And deck client "A" text "idle" has foreground token "idle"
    And deck client "A" exits cleanly

  @requirement-status-tokens
  Scenario: the starting status token colours the starting status word
    When the state database session "tok-agent" has status "starting" 5 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "starting"
    And deck client "A" text "starting" has foreground token "starting"
    And deck client "A" exits cleanly

  @requirement-status-tokens
  Scenario: the stopped status token colours the stopped status word
    When the state database session "tok-target" has status "stopped" 5 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "stopped"
    And deck client "A" text "stopped" has foreground token "stopped"
    And deck client "A" exits cleanly

  @requirement-status-tokens
  Scenario: the error status token colours the error status word
    When the state database session "tok-target" has status "error" 5 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "error"
    And deck client "A" text "error" has foreground token "error"
    And deck client "A" exits cleanly

  @requirement-status-tokens
  Scenario: the archived status token colours the archived status word
    When the state database session "tok-target" has status "archived" 5 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "archived"
    And deck client "A" text "archived" has foreground token "archived"
    And deck client "A" exits cleanly

  @requirement-status-tokens
  Scenario: statuses never borrow each other's colour
    When the state database session "tok-target" has status "error" 5 seconds ago
    Then within one configured reconcile interval deck client "A" screen contains "error"
    And deck client "A" text "error" does not have foreground token "running"
    And deck client "A" text "error" does not have foreground token "waiting"
    And deck client "A" exits cleanly
