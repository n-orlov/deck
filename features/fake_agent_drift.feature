@real-agents
Feature: Fake Claude flag-contract drift alarm
  The fake Claude fixture must keep the installed Claude CLI's observable flag
  contract. This scenario deliberately runs only when a real Claude CLI is
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
