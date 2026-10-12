@real-agents
Feature: Real harness session-name contracts (R265)
  deck reads a session's name from four upstream stores and trusts none of
  them blindly. These scenarios pin, against the actually installed CLIs, the
  exact shapes the readers rely on (SPEC §8, §13.5): Claude's `custom-title`,
  `agent-name` and `ai-title` transcript records, Codex's `state_5.sqlite`
  `threads.name`, Copilot's `workspace.yaml` `name` and `user_named`, and Pi's
  `session_info` session entry. Claude's `ai-title` is generated only for a
  launch without `--name` (an explicit name suppresses it, R254), so it is
  asserted on an unnamed launch; `custom-title` and `agent-name` on an explicit
  one. An incompatible upgrade is a visible failure
  here; the default-tier tests (internal/service `TestBrokenContract*`) prove
  that deck, meanwhile, keeps the row's dummy name and raises no error status.

  They follow real_agent_smoke.feature's conventions and run only when opted in,
  with `DECK_GODOG_TAGS=@real-agents DECK_GODOG_PATHS=real_agent_names.feature go test -run TestFeatures -v ./features/`.
  Each Given step skips its scenario with a stated reason when the CLI is not on
  PATH. Upstream fields are asserted without aliases or coercion.

  Scenario: real claude generates the ai-title record deck adopts for an unnamed launch
    Given the installed Claude CLI resolves on PATH, or this scenario is skipped with a stated reason
    And deck client "A" is started
    When deck client "A" creates claude session unnamed, labelled "real claude unnamed", with permission profile "safe"
    And session "real claude unnamed" submits the prompt "Reply with OK only." to real Claude
    Then the real Claude transcript of session "real claude unnamed" holds an "ai-title" record whose "aiTitle" is a non-empty string
    When deck client "A" exits cleanly

  Scenario: real claude writes the custom-title and agent-name records for an explicit name
    Given the installed Claude CLI resolves on PATH, or this scenario is skipped with a stated reason
    And deck client "A" is started
    When deck client "A" creates claude session "real claude names" with permission profile "safe"
    And session "real claude names" submits the prompt "Reply with OK only." to real Claude
    And session "real claude names" types the line "/rename pinned-claude-name" into its real pane
    Then the real Claude transcript of session "real claude names" holds a "custom-title" record whose "customTitle" is "pinned-claude-name"
    And the real Claude transcript of session "real claude names" holds an "agent-name" record whose "agentName" is "pinned-claude-name"
    When deck client "A" exits cleanly

  Scenario: real codex stores the thread name deck reads in state_5.sqlite
    Given the installed Codex CLI resolves on PATH, or this scenario is skipped with a stated reason
    And deck client "A" is started
    When deck client "A" creates codex session "real codex names" with permission profile "safe"
    And session "real codex names" submits the prompt "Reply with OK only." to real Codex
    Then session "real codex names" receives a real Codex "UserPromptSubmit" hook
    And the real Codex state_5.sqlite has a threads table with id and name columns
    When session "real codex names" types the line "/rename pinned-codex-name" into its real pane
    Then the real Codex state_5.sqlite thread of session "real codex names" has name "pinned-codex-name"
    When deck client "A" exits cleanly

  Scenario: real copilot writes name and user_named into workspace.yaml
    Given the installed Copilot CLI resolves on PATH and answers a prompt, or this scenario is skipped with a stated reason
    And the real Copilot CLI is isolated in a temporary COPILOT_HOME
    And deck client "A" is started
    When deck client "A" creates copilot session "real copilot names" with permission profile "safe"
    Then session "real copilot names" reaches the real Copilot idle prompt, answering the folder-trust prompt if it appears
    When session "real copilot names" sends the line "/rename pinned-copilot-name" to its real Copilot pane
    Then the real Copilot workspace.yaml of session "real copilot names" has "name" "pinned-copilot-name" and "user_named" true
    When deck client "A" exits cleanly

  Scenario: real pi writes a session_info entry carrying the name
    Given the installed Pi CLI resolves on PATH, or this scenario is skipped with a stated reason
    And deck client "A" is started
    When deck client "A" creates pi session "real pi names" with permission profile "safe"
    And session "real pi names" types the line "/name pinned-pi-name" into its real pane
    Then the real Pi session file of session "real pi names" holds a "session_info" entry whose "name" is "pinned-pi-name"
    When deck client "A" exits cleanly
