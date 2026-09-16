@real-agents
Feature: Fake agent flag-contract drift alarm
  Each fake agent fixture must keep its real CLI's observable flag contract.
  These scenarios deliberately run only when the corresponding real CLI is
  installed, so upstream CLI changes are detected without making normal CI
  depend on that CLI.

  Scenario: fake Claude flags conform to installed Claude help
    Given the installed Claude CLI is available
    When I read the installed Claude CLI help
    And I read the repository-built fake Claude help
    Then both help texts document the UUID-valued "--session-id" flag
    And both help texts document the UUID-valued "--resume" flag
    And both help texts document the "--permission-mode" flag
    And the fake Claude permission modes equal the installed Claude permission modes

  Scenario: fake Codex flags conform to installed Codex CLI
    Given the installed Codex CLI is available
    When I read the installed Codex CLI help
    And I read the repository-built fake Codex help
    Then both help texts document the "--ask-for-approval" option
    And both help texts document the "--sandbox" option
    And the fake and installed Codex "--ask-for-approval" values are equal
    And the fake and installed Codex "--sandbox" values are equal
    And the installed Codex help does not document a "--session-id" flag
