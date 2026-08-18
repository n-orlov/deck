Feature: Durable SQLite store
  The released deck binary maintains a private, compatible state database.

  Scenario: initialize a private v1 WAL database
    Given deck client "store" is started
    Then the scenario home has mode "700"
    And the state database has mode "600"
    And the state database journal mode is "wal"
    And the state database has schema version 1
    When deck client "store" exits cleanly

  Scenario: migrate an older supported database
    Given the scenario has an older supported database fixture
    When deck client "migration" is started
    Then the state database has schema version 1
    And the state database journal mode is "wal"
    And the state database has mode "600"
    When deck client "migration" exits cleanly

  Scenario: refuse a newer database without corruption
    Given the scenario has a newer unsupported database fixture
    When the released deck binary opens the newer database
    Then it clearly refuses the newer database
    And the newer database fixture remains unchanged
