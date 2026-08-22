Feature: Real agent session creation and resume through the TUI
  A user can create a real coding-agent session (not just shell) and resume it
  through the released deck TUI, and every relevant fact is observable
  black-box: the assigned conversation id, the exact launch-audit argv, how
  many launches happened, and how many private tmux sessions exist for the
  session's slug.

  Scenario: create a claude session, observe its facts, then resume it
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "claude one" with permission profile "safe"
    Then deck client "A" screen contains "claude one"
    And the state database session "claude one" has a non-empty conversation id
    And the audit log's most recent launch argv for session "claude one" contains "--session-id"
    And the audit log's most recent launch argv for session "claude one" does not contain "--continue"
    And the audit log has 1 launch record for session "claude one"
    # fake-claude exits 0 shortly after doing its own observable work (the
    # fixture wrapper built by the step above adds a short deliberate delay
    # so it survives deck's own post-launch env mirroring, then exits); deck's
    # private tmux server keeps remain-on-exit failed (see
    # tmux_contract.feature), so the pane and its session are eventually torn
    # down and the row becomes resumable within the ordinary UI timeout.
    Then deck client "A" screen contains "resumable"
    And exactly 0 private tmux sessions match slug "deck_claude-one"
    When deck client "A" presses r on session "claude one"
    Then deck client "A" screen contains "starting"
    And deck client "A" screen contains "resumable"
    And the audit log has 2 launch records for session "claude one"
    And the audit log's most recent launch argv for session "claude one" contains "--resume"
    And the audit log's most recent launch argv for session "claude one" does not contain "--continue"
    When deck client "A" exits cleanly

  Scenario: R restarts a running claude session with the resume argv, preserving its conversation id
    # Task 022: `R` kills the selected session's live pane if one exists and
    # relaunches it with the adapter's resume argv -- the SAME conversation
    # id, never a fresh one -- asserted from both the store and the raw
    # audit log. A long-running fixture keeps the pane (and the row's
    # non-stopped status) alive long enough to press `R` without racing a
    # fixture that would otherwise exit almost immediately.
    Given a long-running fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "restart claude" with permission profile "safe"
    Then deck client "A" screen contains "restart claude"
    And the state database session "restart claude" has a non-empty conversation id
    And the audit log has 1 launch record for session "restart claude"
    And the audit log's most recent launch argv for session "restart claude" contains "--session-id"
    When deck client "A" presses R on session "restart claude"
    Then within one configured reconcile interval deck client "A" screen contains "fake-claude resume:"
    And the audit log has 2 launch records for session "restart claude"
    And the audit log's most recent launch argv for session "restart claude" contains "--resume"
    And the audit log's most recent launch argv for session "restart claude" does not contain "--session-id"
    And the audit log's most recent launch argv for session "restart claude" contains session "restart claude"'s conversation id
    When deck client "A" exits cleanly

  Scenario: login_shell marks captured_path advisory in the row and its detail
    # SPEC §6.3: enabling login_shell is mutually exclusive with relying on
    # captured_path (rc files may rewrite PATH), so the two are always
    # created together and the row must say so rather than silently keeping
    # a captured_path the launch no longer honours.
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "login shell one" with permission profile "safe" and login shell enabled
    Then the state database session "login shell one" has login_shell enabled
    And the state database session "login shell one" has a non-empty captured_path
    And the state database session "login shell one" has captured_path marked advisory
    When deck client "A" opens detail for session "login shell one"
    Then deck client "A" screen contains "Captured PATH:"
    And deck client "A" screen contains "advisory"
    When deck client "A" exits cleanly

  Scenario: the launch audit records environment key names but never a value, across create and resume
    # PRD requirement 10 / SPEC §6.4: the launch audit records the exact
    # argv and the names of every applied environment variable -- never a
    # value -- for every create and resume. This types a distinctive,
    # secret-shaped value into the create modal's Env field so its presence
    # or absence in the raw JSONL file is unambiguous either way.
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "audit env one" with permission profile "safe" and env "AUDIT_ENV_TOKEN=super-secret-do-not-log-8675309"
    Then deck client "A" screen contains "audit env one"
    And the audit log's most recent launch record for session "audit env one" names environment key "AUDIT_ENV_TOKEN"
    And the audit log file never contains "super-secret-do-not-log-8675309"
    And the audit log has 1 launch record for session "audit env one"
    Then deck client "A" screen contains "resumable"
    When deck client "A" presses r on session "audit env one"
    Then deck client "A" screen contains "starting"
    And the audit log has 2 launch records for session "audit env one"
    And the audit log's most recent launch record for session "audit env one" names environment key "AUDIT_ENV_TOKEN"
    And the audit log file never contains "super-secret-do-not-log-8675309"
    When deck client "A" exits cleanly
