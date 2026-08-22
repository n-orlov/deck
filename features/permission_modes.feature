Feature: Permission profile mapping, degradation and the yolo gate
  SPEC §5's permission profile is a deck-level concept translated per
  adapter, never a boolean: each declared profile maps to a specific argv, an
  adapter that does not support a requested profile degrades to safe and
  says so, yolo stays behind an `allow_yolo` config gate plus an explicit
  per-launch confirm, and the resolved profile is persisted so it survives a
  resume.

  Scenario: claude maps every declared permission profile to its own argv
    Given the deck config allows yolo
    And a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "csafe" with permission profile "safe"
    And deck client "A" creates claude session "cplan" with permission profile "plan"
    And deck client "A" creates claude session "cedits" with permission profile "edits"
    And deck client "A" creates claude session "cyolo" with permission profile "yolo" confirming yolo
    Then the audit log's most recent launch argv for session "csafe" contains "--permission-mode"
    And the audit log's most recent launch argv for session "csafe" contains "manual"
    And the audit log's most recent launch argv for session "cplan" contains "--permission-mode"
    And the audit log's most recent launch argv for session "cplan" contains "plan"
    And the audit log's most recent launch argv for session "cedits" contains "--permission-mode"
    And the audit log's most recent launch argv for session "cedits" contains "acceptEdits"
    And the audit log's most recent launch argv for session "cyolo" contains "--permission-mode"
    And the audit log's most recent launch argv for session "cyolo" contains "bypassPermissions"
    And the audit log's most recent launch argv for session "cyolo" does not contain "--dangerously"
    When deck client "A" exits cleanly

  Scenario: pi maps only its declared permission profiles to argv
    Given the deck config allows yolo
    And a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates pi session "psafe" with permission profile "safe"
    And deck client "A" creates pi session "pedits" with permission profile "edits"
    And deck client "A" creates pi session "pyolo" with permission profile "yolo" confirming yolo
    Then the audit log's most recent launch argv for session "psafe" does not contain "--approve"
    And the audit log's most recent launch argv for session "pedits" contains "--approve"
    And the audit log's most recent launch argv for session "pyolo" contains "--approve"
    When deck client "A" exits cleanly

  Scenario: an unsupported profile degrades visibly rather than lying
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates pi session "drift" with permission profile "safe"
    And the state database session "drift" is marked degraded from requesting permission profile "plan" on agent "pi"
    And deck client "A" opens detail for session "drift"
    # Task 030: framedDialog's box is a fixed 80% of the viewport (capped at
    # 80, inner 76) rather than growing to fit content, and this degradation
    # sentence is 76+ columns once the "  degraded: " label and "pi" are
    # counted, so it always wraps -- specifically right before "safe". A
    # bare screen-contains-"safe" check (task 012's review finding) is
    # near-vacuous: "safe" also appears a few rows above as the session's
    # own "Permission profile: safe" line, so that check alone would still
    # pass even if the fallback target were silently wrong or the whole
    # degradation sentence vanished. Assert the sentence up to the wrap
    # point verbatim, then pin "safe" to it across the forced line break
    # with a regex spanning the border padding, so the fallback target is
    # tied to this exact sentence rather than to any other "safe" onscreen.
    Then deck client "A" screen contains "degraded: pi does not support permission profile"
    And deck client "A" screen contains "falling back to"
    And deck client "A" screen matches the pattern "falling back to\s*\|[^\n]*\n\|\s*safe\b"
    When deck client "A" exits cleanly

  Scenario: yolo is unavailable without allow_yolo enabled
    Given a fake "claude" binary is on PATH for future deck clients
    # Task 030: framedDialog's box is now a fixed 80% of the viewport
    # clamped to [26, 80] columns (never grows to fit content), and content
    # that overflows the box wraps at a word boundary instead of being
    # truncated. Any viewport of 100 columns or more hits the 80-column
    # ceiling (inner budget 76), and at that width the profile-profile help
    # line's word-wrap break lands right before "yolo", keeping the whole
    # asserted sentence intact on its own wrapped line rather than split
    # mid-sentence -- so the full sentence can still be asserted verbatim
    # rather than the "yolo is not" prefix a narrower terminal would clip
    # it to. 220 columns is kept (rather than trimmed to 100) only because
    # nothing forces a change; either satisfies the >=100 threshold.
    And deck client "A" is started with terminal size 220x30
    When deck client "A" opens the create modal for agent "claude"
    Then deck client "A" screen contains "yolo is not offered because allow_yolo is not enabled in config.toml"
    And deck client "A" screen does not contain "yolo (left/right cycles"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  Scenario: yolo requires an explicit confirm even once allow_yolo is enabled
    Given the deck config allows yolo
    And a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" attempts claude session "noconfirm" with permission profile "yolo" without confirming
    Then deck client "A" screen contains "yolo requires confirmation"
    And the state database does not contain session "noconfirm"
    When deck client "A" closes the create modal
    And deck client "A" creates claude session "confirmed" with permission profile "yolo" confirming yolo
    Then deck client "A" screen contains "starting"
    And the state database session "confirmed" has permission profile "yolo"
    When deck client "A" exits cleanly

  Scenario: the permission profile survives a resume
    Given the deck config allows yolo
    And a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "sticky" with permission profile "yolo" confirming yolo
    Then deck client "A" screen contains "resumable"
    When deck client "A" presses r on session "sticky"
    Then deck client "A" screen contains "starting"
    And the audit log's most recent launch argv for session "sticky" contains "--permission-mode"
    And the audit log's most recent launch argv for session "sticky" contains "bypassPermissions"
    And the audit log's most recent launch argv for session "sticky" contains "--resume"
    When deck client "A" exits cleanly
