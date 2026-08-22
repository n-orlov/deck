Feature: Tilde expansion in the create modal's working directory
  SPEC §11.7 states that a leading `~` expands. The create modal used to stat
  the typed string verbatim, so a `~`-prefixed cwd that exists on disk was
  rejected as non-existent. A leading `~`/`~/` now expands to the user's home
  directory both when validating and when submitting, and the durable
  session row stores the resolved absolute path, never the tilde. Everything
  else in §11.7 (recent_cwds, prefill, ↑/↓ cycling, ghost completion, tab)
  stays out of scope for this phase.

  Scenario: a tilde-prefixed working directory is accepted and resolved
    Given deck client "A" is started with HOME set to the scenario home
    When deck client "A" creates shell session "tilde-session" with working directory "~/Projects/invp-ops-dev-agents"
    Then the state database session "tilde-session" has cwd "Projects/invp-ops-dev-agents" resolved under the scenario home
    When deck client "A" exits cleanly

  Scenario: ~otheruser is rejected with a stated reason rather than half-expanded
    Given deck client "A" is started with HOME set to the scenario home
    When deck client "A" attempts shell session "other-user-tilde" with working directory "~otheruser/work"
    # Task 030: framedDialog's box is a fixed 80% of the viewport (capped at
    # 80, inner 76) rather than growing to fit content, and this error
    # sentence is long enough to always wrap at that width -- right between
    # "home" and "directory" -- which would break a single contiguous "only
    # your own home directory" check. Assert the phrase up to the wrap point
    # and the parenthetical that follows it separately; both are specific
    # enough on their own that together they still pin down this exact
    # message.
    Then deck client "A" screen contains "only your own home"
    And deck client "A" screen contains "(~ or ~/...)"
    And the state database does not contain session "other-user-tilde"
    When deck client "A" closes the create modal
    And deck client "A" exits cleanly
