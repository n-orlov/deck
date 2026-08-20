Feature: Durable SQLite store
  The released deck binary maintains a private, compatible state database.

  Scenario: initialize a private v2 WAL database
    Given deck client "store" is started
    Then the scenario home has mode "700"
    And the state database has mode "600"
    And the state database journal mode is "wal"
    And the state database has schema version 2
    When deck client "store" exits cleanly

  Scenario: migrate an older supported database
    Given the scenario has an older supported database fixture
    When deck client "migration" is started
    Then the state database has schema version 2
    And the state database journal mode is "wal"
    And the state database has mode "600"
    When deck client "migration" exits cleanly

  Scenario: migrate a v1 database in place without recreating a session row
    Given the scenario has a v1 database fixture with an existing session "kept-alive"
    When deck client "v1-upgrade" is started
    Then the state database has schema version 2
    And the state database contains session "kept-alive"
    And the state database session "kept-alive" still has id "v1-fixture-kept-alive"
    When deck client "v1-upgrade" exits cleanly

  Scenario: refuse a newer database without corruption
    Given the scenario has a newer unsupported database fixture
    When the released deck binary opens the newer database
    Then it clearly refuses the newer database
    And the newer database fixture remains unchanged
