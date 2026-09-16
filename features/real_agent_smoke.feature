@real-agents
Feature: Real Claude agent session smoke test
  This scenario proves, against an actually installed Claude CLI (not the
  fake-claude fixture used everywhere else), that deck assigns a UUID
  conversation id at create time and passes that exact id back on resume.
  It deliberately runs only when a real `claude` binary is on PATH, so the
  default suite (which has none) is unaffected. Run it from the repository root
  with `DECK_GODOG_TAGS=@real-agents go test -run TestFeatures -v ./features/...`
  (also documented in docs/reports/phase1.md). Upstream fields are asserted
  without aliases or coercion: an incompatible Claude upgrade is a visible
  conformance failure, not something the harness silently normalizes.

  Scenario: create a real claude session and resume it with the same conversation id
    Given the installed Claude CLI is available
    And deck client "A" is started
    When deck client "A" creates claude session "real claude one" with permission profile "safe"
    Then the state database session "real claude one" has a non-empty conversation id
    And the audit log's most recent launch argv for session "real claude one" contains "--session-id"
    When deck client "A" kills its selected session
    Then the state database contains session "real claude one" with status "stopped"
    When deck client "A" presses r on session "real claude one"
    Then the audit log's most recent launch argv for session "real claude one" contains "--resume"
    And the audit log's most recent launch argv for session "real claude one" contains session "real claude one"'s conversation id
    When deck client "A" exits cleanly

  @codex-real-agent-conformance
  Scenario: create a real codex session and confirm the injected hook contract, skipping cleanly without an installed CLI
    # Mirrors the claude hook scenario below, with the two divergences a
    # real codex-cli forces (task 026, R127's own last third): its
    # instrumentation is a run of -c "hooks.<Event>=..." overrides, not a
    # --settings JSON blob (SPEC §8.2; internal/agent/codex.go's
    # codexHookOverride, task 017), and it never mints a conversation id or
    # fires SessionStart at launch -- only on the first prompt
    # (docs/reports/codex-cli-0.154.0-spike.md Q3d). This scenario is the
    # one place in this feature file that is expected to SKIP, not fail,
    # on a host with no installed `codex` (this container, always): the
    # Given step below states that reason and returns cleanly rather than
    # erroring, so an operator opting into @real-agents on such a host sees
    # "skipped", never a failure. See
    # docs/reports/phase4-scenario-logs/real-agents-codex-skip.log for that
    # exact run.
    Given the installed Codex CLI resolves on PATH, or this scenario is skipped with a stated reason
    And deck client "A" is started
    When deck client "A" creates codex session "real codex one" with permission profile "safe"
    Then session "real codex one"'s launch instrumentation routes "SessionStart" to the released deck _hook via codex's -c overrides
    And session "real codex one"'s launch instrumentation routes "UserPromptSubmit" to the released deck _hook via codex's -c overrides
    When session "real codex one" submits the prompt "Reply with OK only." to real Codex
    Then session "real codex one" receives a real Codex "SessionStart" hook
    And session "real codex one" receives a conforming real Codex "UserPromptSubmit" hook
    When deck client "A" exits cleanly

  Scenario: real claude accepts injected hooks and supplies the upstream payload contract
    Given the installed Claude CLI is available
    And deck client "A" is started
    When deck client "A" creates claude session "real claude hooks" with permission profile "safe"
    Then session "real claude hooks"'s launch instrumentation routes "SessionStart" to the released deck _hook
    And session "real claude hooks" receives a real Claude "SessionStart" hook
    When session "real claude hooks" submits the prompt "Reply with OK only." to real Claude
    Then session "real claude hooks"'s launch instrumentation routes "UserPromptSubmit" to the released deck _hook
    And session "real claude hooks" receives a conforming real Claude "UserPromptSubmit" hook
    When deck client "A" exits cleanly
