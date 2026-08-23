@no-leak-scan
Feature: The whole-grid and whole-file no-leak scan instrument (I-7 assertion half, requirement 21, task 011)
  Requirement 21's masking claim ("masked in every view ... reveal is a
  per-view explicit toggle") is only as strong as the instrument used to
  check it. This scan walks every cell of the live screen grid and every
  regular file under DECK_HOME for a given value, rather than three named
  strings or one named file (SPEC §6.4). The first scenario proves the scan
  itself finds an unmasked value it is pointed at -- a scan that never
  really looks anywhere would let every later "never contains" assertion
  pass vacuously. The second scenario then points that same scan at a real
  secret-shaped env value: the masked env editor's screen grid and every
  file under DECK_HOME except the state database (which legitimately holds
  a session's own env map in plaintext so deck can relaunch the pane) must
  never carry it.

  @requirement-21-leak-scan-positive-control
  Scenario: the scan instrument finds an unmasked control value on screen and on disk
    Given deck client "A" is started
    When deck client "A" creates shell session "leak-scan-control-9c2f1a"
    Then deck client "A" screen contains "leak-scan-control-9c2f1a"
    And the leak scan for "leak-scan-control-9c2f1a" against deck client "A" finds it on screen
    And the leak scan for "leak-scan-control-9c2f1a" against deck client "A" finds it in a home directory file
    When deck client "A" exits cleanly

  @requirement-21-leak-scan-secret-shaped-env
  Scenario: a secret-shaped env value never reaches the masked screen grid or any file except the state database
    Given a fake "claude" binary is on PATH for future deck clients
    And deck client "A" is started
    When deck client "A" creates claude session "leak scan target" with permission profile "safe" and env "AUDIT_ENV_TOKEN=leak-scan-secret-3f9a7c"
    Then deck client "A" screen contains "leak scan target"
    When deck client "A" opens the env editor for session "leak scan target"
    Then deck client "A" screen contains "AUDIT_ENV_TOKEN"
    And deck client "A" screen grid never contains "leak-scan-secret-3f9a7c"
    When deck client "A" closes the dialog with escape
    Then no file under deck client "A" home directory, other than the state database, ever contains "leak-scan-secret-3f9a7c"
    When deck client "A" exits cleanly
