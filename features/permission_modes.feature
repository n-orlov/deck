Feature: Permission profile mapping, degradation and the yolo gate
  SPEC §5's permission profile is a deck-level concept translated per
  adapter, never a boolean: each declared profile maps to a specific argv, an
  adapter that does not support a requested profile degrades to safe and
  says so, yolo stays behind an `allow_yolo` config gate (steer 017 item 2
  removed the separate per-launch confirm keystroke this feature used to
  require once allow_yolo was already enabled), and the resolved profile is
  persisted so it survives a resume. `yolo_default` (inert unless
  `allow_yolo` is also true) opens the create modal already on yolo.

  Scenario: claude maps every declared permission profile to its own argv
    Given the deck config allows yolo
    And a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "csafe" with permission profile "safe"
    And deck client "A" creates claude session "cplan" with permission profile "plan"
    And deck client "A" creates claude session "cedits" with permission profile "edits"
    And deck client "A" creates claude session "cyolo" with permission profile "yolo"
    Then the audit log's most recent launch argv for session "csafe" does not contain "--permission-mode"
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
    And a fake "pi" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates pi session "psafe" with permission profile "safe"
    And deck client "A" creates pi session "pedits" with permission profile "edits"
    And deck client "A" creates pi session "pyolo" with permission profile "yolo"
    Then the audit log's most recent launch argv for session "psafe" does not contain "--approve"
    And the audit log's most recent launch argv for session "pedits" contains "--approve"
    And the audit log's most recent launch argv for session "pyolo" contains "--approve"
    When deck client "A" exits cleanly

  Scenario: an unsupported profile degrades visibly rather than lying
    Given a fake "claude" binary is on PATH for future deck clients
    And a fake "pi" binary is on PATH for future deck clients
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

  Scenario: codex degrades an unsupported plan profile to safe, visibly, through ResolveProfile
    # R127 / SPEC §5's own table: codex has no `plan` (codexProfiles is
    # exactly safe/edits/yolo), and an unsupported profile must "degrade to
    # the nearest safe one and say so ... rather than silently lying". The
    # degrade target and the sentence below both come from the generic
    # agent.Caps.ResolveProfile -- the same call service.CreateAgent makes --
    # never a codex-specific branch and never an alias of plan onto safe
    # inside the adapter (the PRD's own two prohibitions).
    #
    # The create modal is the only surface that can hold an unsupported
    # request at all: it is where a profile is REQUESTED, and every other
    # surface (the `P` switch, the modal's own cycle list) narrows the offer
    # to what the selected adapter declares before the user can ask. So the
    # request is made against claude (which does declare plan) and the Agent
    # field is then cycled onto codex, which is exactly how a real user
    # reaches it: pick the mode, then pick the agent.
    Given a fake "claude" binary is on PATH for future deck clients
    And a fake "codex" binary is on PATH for future deck clients
    # 220x30 for the same reason the allow_yolo scenario below gives: any
    # viewport of 100+ columns hits framedDialog's 80-column ceiling (inner
    # 76), which is the widest this dialog ever gets and therefore the least
    # wrapped. The degrade sentence still gets its own dedicated line
    # (internal/tui.createBody, mirroring the name-reuse warning) rather
    # than being folded into the Permission profile row's help text, so a
    # wrap can only ever fall inside the sentence, never carry the row's
    # unrelated help words into the middle of it -- and the pattern below
    # tolerates that one wrap.
    And deck client "A" is started with terminal size 220x30
    When deck client "A" opens the create modal on agent "claude" for session "codex-plan" with permission profile "plan"
    And deck client "A" cycles the create modal's Agent field to "codex"
    Then deck client "A" screen contains "codex does not support permission profile"
    And deck client "A" screen matches the pattern "falling back to(\s*\|[^\n]*\n\|)?\s*safe\b"
    # The fallback is not merely announced: submitting now creates a codex
    # session that IS safe, with codex's own safe flag pair in its launch
    # argv and no trace of the profile that was asked for.
    When deck client "A" submits the create modal
    Then deck client "A" screen contains "starting"
    And the state database session "codex-plan" has permission profile "safe"
    And the audit log's most recent launch argv for session "codex-plan" contains "on-request"
    And the audit log's most recent launch argv for session "codex-plan" contains "workspace-write"
    And the audit log's most recent launch argv for session "codex-plan" does not contain "plan"
    And the audit log's most recent launch argv for session "codex-plan" does not contain "--last"
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

  Scenario: yolo takes effect immediately with no confirm once allow_yolo is enabled
    # Requirement change (steer 017 item 2): this scenario used to be named
    # "yolo requires an explicit confirm even once allow_yolo is enabled"
    # and asserted the opposite -- that Enter on profile "yolo" with no "y"
    # keystroke was refused. The operator's own SPEC push (395babf) removed
    # that confirm: allow_yolo alone gates yolo's availability now, so
    # choosing it takes effect on Enter directly, exactly like every other
    # profile. This is a deliberate inversion of the prior assertion, not an
    # assertion deleted without replacement.
    Given the deck config allows yolo
    And a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "confirmed" with permission profile "yolo"
    Then deck client "A" screen contains "starting"
    And the state database session "confirmed" has permission profile "yolo"
    When deck client "A" exits cleanly

  Scenario: yolo_default opens the create modal already on yolo once allow_yolo is enabled
    Given the deck config allows yolo and defaults new sessions to it
    And a fake "claude" binary is on PATH for future deck clients
    # Same 220-column rationale as the "yolo is unavailable" scenario above:
    # a viewport this wide always hits framedDialog's 80-column ceiling, so
    # the Permission profile row's exact text is asserted at a stable width.
    And deck client "A" is started with terminal size 220x30
    When deck client "A" opens the create modal for agent "claude"
    Then deck client "A" screen contains "yolo (left/right cycles"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly

  Scenario: the permission profile survives a resume
    Given the deck config allows yolo
    And a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "sticky" with permission profile "yolo"
    Then deck client "A" screen contains "resumable"
    When deck client "A" presses r on session "sticky"
    Then deck client "A" screen contains "starting"
    And the audit log's most recent launch argv for session "sticky" contains "--permission-mode"
    And the audit log's most recent launch argv for session "sticky" contains "bypassPermissions"
    And the audit log's most recent launch argv for session "sticky" contains "--resume"
    When deck client "A" exits cleanly
