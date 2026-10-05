@requirement-204-stale-hook-binding
Feature: a session launched under an older deck binary survives a newer state database (R204, #56)
  A coding agent keeps the hook command it was launched with, and that command
  is the absolute path of the deck binary that launched it. When deck is later
  run as a binary with a newer schema, the old binary's `_hook` meets a state
  database it cannot open. It then re-execs the binary that last wrote the
  database, so the hook still lands; when that binary cannot be re-exec'd, the
  hook prints the restart message instead and the TUI hints in i that the
  session is bound to the old path. Binary A claims a lower schema through the
  test-only deckoldschema build tag and an ldflag, never a real old release;
  binary B is a copy of the current build.

  Scenario: a claude session launched under binary A heals through the re-exec once deck runs as binary B
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And the scenario deck binary is the old build "A" claiming schema 7
    And deck client "A" is started
    When deck client "A" creates claude session "bound" with permission profile "safe"
    Then the state database session "bound" is bound to the hook executable of binary "A"
    When deck client "A" exits cleanly
    And the scenario deck binary is "B", a copy of the current build
    And deck client "B" is started with terminal size 160x40
    Then the state database schema is newer than binary "A"'s
    When fake Claude session "bound" fires "Notification" for itself using injected identity:
      | notification_type | permission_prompt |
    Then session "bound" has 1 "notification" event
    And the state database session "bound" has hook status "waiting", reason "permission_prompt", message "", acknowledged 0, and notify_epoch 0
    When deck client "B" exits cleanly

  Scenario: a codex session launched under binary A heals through the re-exec once deck runs as binary B
    Given a long-running fake "codex" binary is on PATH for future deck clients
    And the scenario deck binary is the old build "A" claiming schema 7
    And deck client "A" is started
    When deck client "A" creates codex session "bound" with permission profile "safe"
    Then the state database session "bound" is bound to the hook executable of binary "A"
    When deck client "A" exits cleanly
    And the scenario deck binary is "B", a copy of the current build
    And deck client "B" is started with terminal size 160x40
    Then the state database schema is newer than binary "A"'s
    And the state database session "bound" has no conversation id
    When fake Codex session "bound" is prompted with "hello through binary A"
    Then within 3 seconds deck client "B" row "bound" contains "live"
    And the state database session "bound" has a non-empty conversation id
    When fake Codex session "bound" requests approval to run tool "apply_patch"
    Then within 3 seconds deck client "B" row "bound" contains "waiting"
    And the state database session "bound"'s status reason contains "apply_patch"
    When deck client "B" exits cleanly

  Scenario: a pi session under binary A takes the same re-exec when its hook reaches the newer database
    # Pi builds no hook command (SPEC 8.1), so no Pi agent ever runs binary A's
    # _hook by itself; binary A's _hook is run for the Pi row directly, which is
    # the same code path any Pi event source would take.
    Given a fake "pi" binary is on PATH for future deck clients
    And the scenario deck binary is the old build "A" claiming schema 7
    And deck client "A" is started
    When deck client "A" creates pi session "bound" with permission profile "safe"
    Then the state database session "bound" has no hook executable
    When deck client "A" exits cleanly
    And the scenario deck binary is "B", a copy of the current build
    And deck client "B" is started with terminal size 160x40
    Then the state database schema is newer than binary "A"'s
    When binary "A" runs _hook for session "bound" with a "SessionStart" payload
    Then that hook run succeeded and printed nothing
    And session "bound" has 1 "session_start" event
    When deck client "B" exits cleanly

  Scenario: a claude session whose binary A cannot re-exec shows the stale-binding hint in i
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And the scenario deck binary is the old build "A" claiming schema 7
    And deck client "A" is started
    When deck client "A" creates claude session "bound" with permission profile "safe"
    Then the state database session "bound" is bound to the hook executable of binary "A"
    When deck client "A" exits cleanly
    And the scenario deck binary is "B", a copy of the current build
    And deck client "B" is started with terminal size 160x40
    Then the state database schema is newer than binary "A"'s
    When binary "B" is no longer executable
    And binary "A" runs _hook for session "bound" with a "SessionStart" payload
    Then that hook run failed and its output names binary "A" and both schemas and says to restart the session from deck
    And session "bound" has 0 "session_start" events
    When deck client "B" opens detail for session "bound"
    Then deck client "B" screen shows the hint that session binds to binary "A"
    When deck client "B" closes detail
    And deck client "B" exits cleanly

  Scenario: a codex session whose binary A cannot re-exec shows the stale-binding hint in i
    Given a long-running fake "codex" binary is on PATH for future deck clients
    And the scenario deck binary is the old build "A" claiming schema 7
    And deck client "A" is started
    When deck client "A" creates codex session "bound" with permission profile "safe"
    Then the state database session "bound" is bound to the hook executable of binary "A"
    When deck client "A" exits cleanly
    And the scenario deck binary is "B", a copy of the current build
    And deck client "B" is started with terminal size 160x40
    Then the state database schema is newer than binary "A"'s
    When binary "B" is no longer executable
    And binary "A" runs _hook for session "bound" with a "SessionStart" payload
    Then that hook run failed and its output names binary "A" and both schemas and says to restart the session from deck
    And session "bound" has 0 "session_start" events
    When deck client "B" opens detail for session "bound"
    Then deck client "B" screen shows the hint that session binds to binary "A"
    When deck client "B" closes detail
    And deck client "B" exits cleanly

  Scenario: a pi session whose binary A cannot re-exec shows the stale-binding hint in i
    # Pi launches record no hook executable, so the scenario binds the Pi row to
    # binary A itself, exactly the fact a Claude or Codex launch records.
    Given a fake "pi" binary is on PATH for future deck clients
    And the scenario deck binary is the old build "A" claiming schema 7
    And deck client "A" is started
    When deck client "A" creates pi session "bound" with permission profile "safe"
    Then the state database session "bound" has no hook executable
    When deck client "A" exits cleanly
    And the scenario deck binary is "B", a copy of the current build
    And deck client "B" is started with terminal size 160x40
    Then the state database schema is newer than binary "A"'s
    When the scenario binds the state database session "bound" to the hook executable of binary "A"
    When binary "B" is no longer executable
    And binary "A" runs _hook for session "bound" with a "SessionStart" payload
    Then that hook run failed and its output names binary "A" and both schemas and says to restart the session from deck
    And session "bound" has 0 "session_start" events
    When deck client "B" opens detail for session "bound"
    Then deck client "B" screen shows the hint that session binds to binary "A"
    When deck client "B" closes detail
    And deck client "B" exits cleanly

  Scenario: a pi session launched under binary A records no binding and shows no hint
    Given a fake "pi" binary is on PATH for future deck clients
    And the scenario deck binary is the old build "A" claiming schema 7
    And deck client "A" is started
    When deck client "A" creates pi session "bound" with permission profile "safe"
    Then the state database session "bound" has no hook executable
    When deck client "A" exits cleanly
    And the scenario deck binary is "B", a copy of the current build
    And deck client "B" is started with terminal size 160x40
    Then the state database schema is newer than binary "A"'s
    When deck client "B" opens detail for session "bound"
    Then deck client "B" screen shows no "hooks: bound to" once the detail's declined-hook lookup has settled
    When deck client "B" closes detail
    And deck client "B" exits cleanly
