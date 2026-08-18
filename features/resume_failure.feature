Feature: Resume failure causes leave a retained, explained error row
  SPEC §9.3 names three specific ways a resume can fail: an unknown/rejected
  conversation id, a missing or non-directory cwd, and the agent binary not
  being found on PATH. None of these may be mistaken for (or silently
  produce) a fresh conversation: the row stays present with status "error"
  and a stated reason naming its own cause, and the audit log gains no
  additional launch record.

  Scenario: an unknown conversation id keeps the row as a retained, explained error
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "noid" with permission profile "safe"
    Then deck client "A" screen contains "resumable"
    And the audit log has 1 launch record for session "noid"
    When the state database session "noid"'s conversation id is cleared
    And deck client "A" presses r on session "noid"
    Then deck client "A" screen contains "Cannot resume: resume session"
    And deck client "A" screen contains "noid"
    And the state database contains session "noid" with status "error"
    And the state database session "noid"'s status reason contains "conversation id"
    And the audit log has 1 launch record for session "noid"
    When deck client "A" exits cleanly

  Scenario: a missing cwd keeps the row as a retained, explained error
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "gonecwd" with permission profile "safe"
    Then deck client "A" screen contains "resumable"
    And the audit log has 1 launch record for session "gonecwd"
    When the working directory shared by this scenario's sessions no longer exists
    And deck client "A" presses r on session "gonecwd"
    Then deck client "A" screen contains "Cannot resume: resume session"
    And deck client "A" screen contains "gonecwd"
    And the state database contains session "gonecwd" with status "error"
    And the state database session "gonecwd"'s status reason contains "missing or not a directory"
    And the audit log has 1 launch record for session "gonecwd"
    When deck client "A" exits cleanly

  Scenario: an agent binary not on PATH keeps the row as a retained, explained error
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "nobinary" with permission profile "safe"
    Then deck client "A" screen contains "resumable"
    And the audit log has 1 launch record for session "nobinary"
    When the state database session "nobinary"'s captured_path no longer contains the fake "claude" binary
    And deck client "A" presses r on session "nobinary"
    Then deck client "A" screen contains "Cannot resume: resume session"
    And deck client "A" screen contains "nobinary"
    And the state database contains session "nobinary" with status "error"
    And the state database session "nobinary"'s status reason contains "not found on PATH"
    And the audit log has 1 launch record for session "nobinary"
    When deck client "A" exits cleanly
