# Uncached verbose suite capture

Captured after the fake-agent harness fix (see `docs/reports/build-vet-evidence.md`
for the corresponding build/vet evidence and `docs/reports/toolchain-versions.md`
for the toolchain versions of the sibling container this ran in).

Command run from the repository root:

```
{ time ci/run.sh go test -count=1 -v ./... ; } 2>&1
```

`-count=1` disables the Go test result cache, so this is a genuinely uncached
run of the full package/scenario suite (not a re-report of cached results).

This file is a **normalized** rendering, derived from the raw, unedited capture
in `docs/reports/verbose-suite-capture-source.log` (the file to consult if you
need the byte-for-byte original, including the ANSI escape bytes Godog's
pretty formatter emits). The only normalization applied here is stripping
those ANSI color escape codes for plain-text readability; no lines, words, or
timings were removed, reordered, or otherwise altered.

## Full stdout/stderr and wall timing

```
=== RUN   TestDeckBinaryShellCreateAndSlugCollisionThroughPTY
--- PASS: TestDeckBinaryShellCreateAndSlugCollisionThroughPTY (1.16s)
=== RUN   TestDeckBinaryRefreshesAllConcurrentClients
--- PASS: TestDeckBinaryRefreshesAllConcurrentClients (2.42s)
=== RUN   TestDeckBinaryEmptyHelpAndQuitThroughPTY
--- PASS: TestDeckBinaryEmptyHelpAndQuitThroughPTY (0.66s)
PASS
ok  	github.com/n-orlov/deck/cmd/deck	4.246s
=== RUN   TestAcceptedClaudeFlagsProduceDeterministicTerminalRecord
--- PASS: TestAcceptedClaudeFlagsProduceDeterministicTerminalRecord (0.00s)
=== RUN   TestRejectsInvalidUUIDsUnknownFlagsAndModes
=== RUN   TestRejectsInvalidUUIDsUnknownFlagsAndModes/malformed_resume
=== RUN   TestRejectsInvalidUUIDsUnknownFlagsAndModes/unknown_flag
=== RUN   TestRejectsInvalidUUIDsUnknownFlagsAndModes/unknown_mode
=== RUN   TestRejectsInvalidUUIDsUnknownFlagsAndModes/malformed_session_ID
--- PASS: TestRejectsInvalidUUIDsUnknownFlagsAndModes (0.00s)
    --- PASS: TestRejectsInvalidUUIDsUnknownFlagsAndModes/malformed_resume (0.00s)
    --- PASS: TestRejectsInvalidUUIDsUnknownFlagsAndModes/unknown_flag (0.00s)
    --- PASS: TestRejectsInvalidUUIDsUnknownFlagsAndModes/unknown_mode (0.00s)
    --- PASS: TestRejectsInvalidUUIDsUnknownFlagsAndModes/malformed_session_ID (0.00s)
=== RUN   TestExitCodeIsControlledOnlyByFixtureEnvironment
--- PASS: TestExitCodeIsControlledOnlyByFixtureEnvironment (0.00s)
PASS
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.002s
=== RUN   TestBlackBoxAssertionsObserveRealSession
--- PASS: TestBlackBoxAssertionsObserveRealSession (0.80s)
=== RUN   TestFeatures
Feature: Multi-client session refresh
  Independent deck clients sharing one home and private tmux socket converge on
  durable session changes without a restart.
=== RUN   TestFeatures/create_and_kill_propagate_and_surviving_clients_tolerate_a_peer_crash

  Scenario: create and kill propagate and surviving clients tolerate a peer crash                  # concurrency.feature:6
    Given deck client "A" is started                                                               # assertions_test.go:26 -> github.com/n-orlov/deck/features.startNamedClient
    And deck client "B" is started                                                                 # assertions_test.go:26 -> github.com/n-orlov/deck/features.startNamedClient
    And deck client "C" is started                                                                 # assertions_test.go:26 -> github.com/n-orlov/deck/features.startNamedClient
    When deck client "A" creates shell session "shared session"                                    # assertions_test.go:42 -> github.com/n-orlov/deck/features.clientCreatesShellSession
    Then within one configured reconcile interval deck client "B" screen contains "shared session" # assertions_test.go:29 -> github.com/n-orlov/deck/features.clientScreenContainsWithinReconcileInterval
    And within one configured reconcile interval deck client "C" screen contains "shared session"  # assertions_test.go:29 -> github.com/n-orlov/deck/features.clientScreenContainsWithinReconcileInterval
    When deck client "B" kills its selected session                                                # assertions_test.go:50 -> github.com/n-orlov/deck/features.clientKillsSelectedSession
    Then within one configured reconcile interval deck client "A" screen contains "resumable"      # assertions_test.go:29 -> github.com/n-orlov/deck/features.clientScreenContainsWithinReconcileInterval
    And within one configured reconcile interval deck client "C" screen contains "resumable"       # assertions_test.go:29 -> github.com/n-orlov/deck/features.clientScreenContainsWithinReconcileInterval
    And the state database contains session "shared session" with status "stopped"                 # assertions_test.go:37 -> github.com/n-orlov/deck/features.databaseSessionStatus
    When deck client "C" is killed with SIGKILL                                                    # assertions_test.go:51 -> github.com/n-orlov/deck/features.clientIsKilledWithSIGKILL
    And deck client "A" creates shell session "after crash"                                        # assertions_test.go:42 -> github.com/n-orlov/deck/features.clientCreatesShellSession
    Then deck client "B" screen contains "after crash"                                             # assertions_test.go:28 -> github.com/n-orlov/deck/features.clientScreenContains
    And the state database contains session "shared session" with status "stopped"                 # assertions_test.go:37 -> github.com/n-orlov/deck/features.databaseSessionStatus
    And the state database contains session "after crash" with status "starting"                   # assertions_test.go:37 -> github.com/n-orlov/deck/features.databaseSessionStatus
    When deck client "A" exits cleanly                                                             # assertions_test.go:53 -> github.com/n-orlov/deck/features.clientExitsCleanly
    And deck client "B" exits cleanly                                                              # assertions_test.go:53 -> github.com/n-orlov/deck/features.clientExitsCleanly
=== RUN   TestFeatures/a_live_client_reconciles_an_externally_killed_private_server

  Scenario: a live client reconciles an externally killed private server                      # concurrency.feature:25
    Given deck client "A" is started                                                          # assertions_test.go:26 -> github.com/n-orlov/deck/features.startNamedClient
    When deck client "A" creates shell session "externally stopped"                           # assertions_test.go:42 -> github.com/n-orlov/deck/features.clientCreatesShellSession
    And the private tmux server is killed                                                     # godog_test.go:41 -> github.com/n-orlov/deck/features.initializeScenario.func2
    Then within one configured reconcile interval deck client "A" screen contains "resumable" # assertions_test.go:29 -> github.com/n-orlov/deck/features.clientScreenContainsWithinReconcileInterval
    And the state database contains session "externally stopped" with status "stopped"        # assertions_test.go:37 -> github.com/n-orlov/deck/features.databaseSessionStatus
    And the audit log contains event "tmux.session_gone" for a session                        # assertions_test.go:39 -> github.com/n-orlov/deck/features.auditContainsSessionEvent
    And the private tmux session "deck_externally-stopped" does not exist                     # assertions_test.go:43 -> github.com/n-orlov/deck/features.privateSessionDoesNotExist
    When deck client "A" sends "?"                                                            # assertions_test.go:27 -> github.com/n-orlov/deck/features.sendClientKeys
    Then deck client "A" screen contains "Runtime controls"                                   # assertions_test.go:28 -> github.com/n-orlov/deck/features.clientScreenContains
    When deck client "A" exits cleanly                                                        # assertions_test.go:53 -> github.com/n-orlov/deck/features.clientExitsCleanly

Feature: Deterministic runtime controls
  The released binary remains reproducible when its documented deterministic controls are set.
=== RUN   TestFeatures/deterministic_frames,_audit_timing,_and_generated_identifiers

  Scenario: deterministic frames, audit timing, and generated identifiers # determinism.feature:4
    Given deck frames are byte-stable with DECK_ASCII and NO_COLOR        # determinism_test.go:19 -> github.com/n-orlov/deck/features.deterministicFrames
    When a stepped frozen-clock shell session is created and killed       # determinism_test.go:20 -> github.com/n-orlov/deck/features.frozenClockSessionIsCreatedAndKilled
    Then its wall clock steps while monotonic durations advance           # determinism_test.go:21 -> github.com/n-orlov/deck/features.frozenAuditIsSteppedAndAdvances
    And repeating DECK_ID_SEED reproduces generated ids                   # determinism_test.go:22 -> github.com/n-orlov/deck/features.repeatingSeedReproducesID

Feature: Fake Claude fixture mechanism
  The repository-built fake Claude is usable as a real tmux session command,
  without a deck-only status back channel.
=== RUN   TestFeatures/accepted_argv_and_controlled_exit_statuses_are_observable_from_panes

  Scenario: accepted argv and controlled exit statuses are observable from panes            # fake_agent.feature:5
    Given the repository-built fake Claude fixture is ready                                 # fake_agent_feature_test.go:27 -> *fakeAgentScenario
    When the fake Claude fixture is launched successfully as a private tmux session command # fake_agent_feature_test.go:28 -> *fakeAgentScenario
    Then its success pane shows the deterministic banner and exact accepted argv            # fake_agent_feature_test.go:29 -> *fakeAgentScenario
    And the successful fake Claude session exits with status 0                              # fake_agent_feature_test.go:30 -> *fakeAgentScenario
    When the fake Claude fixture is launched with controlled failure status 7               # fake_agent_feature_test.go:31 -> *fakeAgentScenario
    Then its failure pane shows the deterministic banner and exact accepted argv            # fake_agent_feature_test.go:32 -> *fakeAgentScenario
    And the failed fake Claude session remains with status 7                                # fake_agent_feature_test.go:33 -> *fakeAgentScenario

Feature: Godog harness wiring
  The repository's normal Go test command discovers default feature files.
=== RUN   TestFeatures/default_feature_suite_is_wired

  Scenario: default feature suite is wired # harness.feature:4
    Given the Godog harness is available   # godog_test.go:40 -> github.com/n-orlov/deck/features.initializeScenario.func1
=== RUN   TestFeatures/a_private_tmux_server_can_be_removed

  Scenario: a private tmux server can be removed # harness.feature:7
    Given the private tmux server is killed      # godog_test.go:41 -> github.com/n-orlov/deck/features.initializeScenario.func2

Feature: Durable SQLite store
  The released deck binary maintains a private, compatible state database.
=== RUN   TestFeatures/initialize_a_private_v1_WAL_database

  Scenario: initialize a private v1 WAL database # store.feature:4
    Given deck client "store" is started         # assertions_test.go:26 -> github.com/n-orlov/deck/features.startNamedClient
    Then the scenario home has mode "700"        # assertions_test.go:40 -> github.com/n-orlov/deck/features.scenarioHomeMode
    And the state database has mode "600"        # assertions_test.go:41 -> github.com/n-orlov/deck/features.stateDatabaseMode
    And the state database journal mode is "wal" # assertions_test.go:36 -> github.com/n-orlov/deck/features.databaseJournalMode
    And the state database has schema version 1  # assertions_test.go:35 -> github.com/n-orlov/deck/features.databaseSchemaVersion
    When deck client "store" exits cleanly       # assertions_test.go:53 -> github.com/n-orlov/deck/features.clientExitsCleanly
=== RUN   TestFeatures/migrate_an_older_supported_database

  Scenario: migrate an older supported database                # store.feature:12
    Given the scenario has an older supported database fixture # store_feature_test.go:20 -> github.com/n-orlov/deck/features.olderDatabaseFixture
    When deck client "migration" is started                    # assertions_test.go:26 -> github.com/n-orlov/deck/features.startNamedClient
    Then the state database has schema version 1               # assertions_test.go:35 -> github.com/n-orlov/deck/features.databaseSchemaVersion
    And the state database journal mode is "wal"               # assertions_test.go:36 -> github.com/n-orlov/deck/features.databaseJournalMode
    And the state database has mode "600"                      # assertions_test.go:41 -> github.com/n-orlov/deck/features.stateDatabaseMode
    When deck client "migration" exits cleanly                 # assertions_test.go:53 -> github.com/n-orlov/deck/features.clientExitsCleanly
=== RUN   TestFeatures/refuse_a_newer_database_without_corruption

  Scenario: refuse a newer database without corruption          # store.feature:20
    Given the scenario has a newer unsupported database fixture # store_feature_test.go:21 -> github.com/n-orlov/deck/features.newerDatabaseFixture
    When the released deck binary opens the newer database      # store_feature_test.go:22 -> github.com/n-orlov/deck/features.releasedBinaryOpensNewerDatabase
    Then it clearly refuses the newer database                  # store_feature_test.go:23 -> github.com/n-orlov/deck/features.clearlyRefusesNewerDatabase
    And the newer database fixture remains unchanged            # store_feature_test.go:24 -> github.com/n-orlov/deck/features.newerDatabaseRemainsUnchanged

Feature: Private tmux contract
  deck keeps managed sessions on its configured private tmux socket.
=== RUN   TestFeatures/private_server_options,_safe_naming,_and_slug_collisions

  Scenario: private server options, safe naming, and slug collisions               # tmux_contract.feature:4
    Given deck client "contract" is started                                        # assertions_test.go:26 -> github.com/n-orlov/deck/features.startNamedClient
    When deck client "contract" creates shell session "Dots.And:Colons"            # assertions_test.go:42 -> github.com/n-orlov/deck/features.clientCreatesShellSession
    Then the private tmux session "deck_dots-and-colons" exists                    # assertions_test.go:30 -> github.com/n-orlov/deck/features.privateSessionExists
    And the default tmux socket does not have session "deck_dots-and-colons"       # assertions_test.go:44 -> github.com/n-orlov/deck/features.defaultSessionDoesNotExist
    And the private tmux option "exit-empty" is "off"                              # assertions_test.go:34 -> github.com/n-orlov/deck/features.privateOptionIs
    And the private tmux option "remain-on-exit" is "failed"                       # assertions_test.go:34 -> github.com/n-orlov/deck/features.privateOptionIs
    And the private tmux option "window-size" is "latest"                          # assertions_test.go:34 -> github.com/n-orlov/deck/features.privateOptionIs
    And the private tmux option "aggressive-resize" is "on"                        # assertions_test.go:34 -> github.com/n-orlov/deck/features.privateOptionIs
    When deck client "contract" attempts shell session "dots and colons"           # assertions_test.go:45 -> github.com/n-orlov/deck/features.clientAttemptsShellSession
    Then deck client "contract" screen contains "name collides with existing slug" # assertions_test.go:28 -> github.com/n-orlov/deck/features.clientScreenContains
    When deck client "contract" closes the create modal                            # assertions_test.go:46 -> github.com/n-orlov/deck/features.clientClosesCreateModal
    And deck client "contract" exits cleanly                                       # assertions_test.go:53 -> github.com/n-orlov/deck/features.clientExitsCleanly
=== RUN   TestFeatures/missing_tmux_is_actionable

  Scenario: missing tmux is actionable                                    # tmux_contract.feature:18
    Given deck client "missing" is started without tmux                   # assertions_test.go:47 -> github.com/n-orlov/deck/features.startClientWithoutTMux
    Then deck client "missing" screen contains "tmux unavailable"         # assertions_test.go:28 -> github.com/n-orlov/deck/features.clientScreenContains
    And deck client "missing" screen contains "Install tmux 3.2 or newer" # assertions_test.go:28 -> github.com/n-orlov/deck/features.clientScreenContains
    When deck client "missing" exits cleanly                              # assertions_test.go:53 -> github.com/n-orlov/deck/features.clientExitsCleanly
=== RUN   TestFeatures/old_tmux_is_actionable

  Scenario: old tmux is actionable                                    # tmux_contract.feature:24
    Given deck client "old" is started with tmux version "3.1c"       # assertions_test.go:48 -> github.com/n-orlov/deck/features.startClientWithTMuxVersion
    Then deck client "old" screen contains "tmux unavailable"         # assertions_test.go:28 -> github.com/n-orlov/deck/features.clientScreenContains
    And deck client "old" screen contains "tmux 3.1c is too old"      # assertions_test.go:28 -> github.com/n-orlov/deck/features.clientScreenContains
    And deck client "old" screen contains "Install tmux 3.2 or newer" # assertions_test.go:28 -> github.com/n-orlov/deck/features.clientScreenContains
    When deck client "old" exits cleanly                              # assertions_test.go:53 -> github.com/n-orlov/deck/features.clientExitsCleanly

Feature: Phase 0 walking skeleton
  A user can manage a shell session solely through the released deck TUI.
=== RUN   TestFeatures/create,_attach,_kill,_and_retain_the_durable_shell_session

  Scenario: create, attach, kill, and retain the durable shell session                                # walking_skeleton.feature:4
    Given deck client "A" is started                                                                  # assertions_test.go:26 -> github.com/n-orlov/deck/features.startNamedClient
    Then deck client "A" screen contains "No sessions yet"                                            # assertions_test.go:28 -> github.com/n-orlov/deck/features.clientScreenContains
    When deck client "A" creates shell session "walking session"                                      # assertions_test.go:42 -> github.com/n-orlov/deck/features.clientCreatesShellSession
    Then deck client "A" screen contains "walking session"                                            # assertions_test.go:28 -> github.com/n-orlov/deck/features.clientScreenContains
    And the private tmux session "deck_walking-session" exists                                        # assertions_test.go:30 -> github.com/n-orlov/deck/features.privateSessionExists
    And the private tmux session "deck_walking-session" has one pane in the created working directory # assertions_test.go:33 -> github.com/n-orlov/deck/features.privateSessionPaneInCreatedWorkingDir
    And the private tmux session "deck_walking-session" has exactly one pane running "sh"             # assertions_test.go:32 -> github.com/n-orlov/deck/features.privateSessionPaneCommand
    When deck client "A" attaches to and detaches from its session                                    # assertions_test.go:49 -> github.com/n-orlov/deck/features.clientAttachesAndDetaches
    And deck client "A" kills its selected session                                                    # assertions_test.go:50 -> github.com/n-orlov/deck/features.clientKillsSelectedSession
    Then the private tmux session "deck_walking-session" does not exist                               # assertions_test.go:43 -> github.com/n-orlov/deck/features.privateSessionDoesNotExist
    And the state database contains session "walking session" with status "stopped"                   # assertions_test.go:37 -> github.com/n-orlov/deck/features.databaseSessionStatus
    And deck client "A" screen contains "resumable"                                                   # assertions_test.go:28 -> github.com/n-orlov/deck/features.clientScreenContains
    And the audit log contains event "killed" for a session                                           # assertions_test.go:39 -> github.com/n-orlov/deck/features.auditContainsSessionEvent
    And the created working-directory sentinel is unchanged                                           # assertions_test.go:52 -> github.com/n-orlov/deck/features.createdSentinelIsUnchanged
    When deck client "A" exits cleanly                                                                # assertions_test.go:53 -> github.com/n-orlov/deck/features.clientExitsCleanly

13 scenarios (13 passed)
92 steps (92 passed)
6.905015986s
--- PASS: TestFeatures (6.91s)
    --- PASS: TestFeatures/create_and_kill_propagate_and_surviving_clients_tolerate_a_peer_crash (1.13s)
    --- PASS: TestFeatures/a_live_client_reconciles_an_externally_killed_private_server (0.60s)
    --- PASS: TestFeatures/deterministic_frames,_audit_timing,_and_generated_identifiers (1.12s)
    --- PASS: TestFeatures/accepted_argv_and_controlled_exit_statuses_are_observable_from_panes (0.44s)
    --- PASS: TestFeatures/default_feature_suite_is_wired (0.25s)
    --- PASS: TestFeatures/a_private_tmux_server_can_be_removed (0.26s)
    --- PASS: TestFeatures/initialize_a_private_v1_WAL_database (0.32s)
    --- PASS: TestFeatures/migrate_an_older_supported_database (0.37s)
    --- PASS: TestFeatures/refuse_a_newer_database_without_corruption (0.29s)
    --- PASS: TestFeatures/private_server_options,_safe_naming,_and_slug_collisions (0.59s)
    --- PASS: TestFeatures/missing_tmux_is_actionable (0.32s)
    --- PASS: TestFeatures/old_tmux_is_actionable (0.32s)
    --- PASS: TestFeatures/create,_attach,_kill,_and_retain_the_durable_shell_session (0.87s)
=== RUN   TestGodogRejectsUndefinedAndFailedSteps
=== RUN   TestGodogRejectsUndefinedAndFailedSteps/undefined
U 1


1 scenarios (1 undefined)
1 steps (1 undefined)
122.679µs

You can implement step definitions for undefined steps with these snippets:

func anUnregisteredStep() error {
	return godog.ErrPending
}

func InitializeScenario(ctx *godog.ScenarioContext) {
	ctx.Step(`^an unregistered step$`, anUnregisteredStep)
}

=== RUN   TestGodogRejectsUndefinedAndFailedSteps/failed
F 1


--- Failed steps:

  Scenario: error binding # /tmp/TestGodogRejectsUndefinedAndFailedStepsfailed3836812420/001/failure.feature:2
    Given a failing step # /tmp/TestGodogRejectsUndefinedAndFailedStepsfailed3836812420/001/failure.feature:3
      Error: deliberate step failure


1 scenarios (1 failed)
1 steps (1 failed)
300.213µs
--- PASS: TestGodogRejectsUndefinedAndFailedSteps (0.00s)
    --- PASS: TestGodogRejectsUndefinedAndFailedSteps/undefined (0.00s)
    --- PASS: TestGodogRejectsUndefinedAndFailedSteps/failed (0.00s)
=== RUN   TestScenarioHarnessTeardownReportsLeaks
=== RUN   TestScenarioHarnessTeardownReportsLeaks/surviving_child
=== RUN   TestScenarioHarnessTeardownReportsLeaks/responding_socket
=== RUN   TestScenarioHarnessTeardownReportsLeaks/surviving_root
--- PASS: TestScenarioHarnessTeardownReportsLeaks (1.79s)
    --- PASS: TestScenarioHarnessTeardownReportsLeaks/surviving_child (1.27s)
    --- PASS: TestScenarioHarnessTeardownReportsLeaks/responding_socket (0.26s)
    --- PASS: TestScenarioHarnessTeardownReportsLeaks/surviving_root (0.26s)
=== RUN   TestScenarioHarnessSharesIsolationAndCleansUp
--- PASS: TestScenarioHarnessSharesIsolationAndCleansUp (0.34s)
=== RUN   TestScreenDriverLaunchesDeckAndAnswersTerminalProbes
--- PASS: TestScreenDriverLaunchesDeckAndAnswersTerminalProbes (0.32s)
=== RUN   TestNormalizeFrame
=== RUN   TestNormalizeFrame/unfrozen_timestamp_and_just_now
=== RUN   TestNormalizeFrame/unfrozen_numeric_minute_age
=== RUN   TestNormalizeFrame/unfrozen_numeric_hour_age
=== RUN   TestNormalizeFrame/unfrozen_numeric_day_age
=== RUN   TestNormalizeFrame/frozen_values_preserved
--- PASS: TestNormalizeFrame (0.00s)
    --- PASS: TestNormalizeFrame/unfrozen_timestamp_and_just_now (0.00s)
    --- PASS: TestNormalizeFrame/unfrozen_numeric_minute_age (0.00s)
    --- PASS: TestNormalizeFrame/unfrozen_numeric_hour_age (0.00s)
    --- PASS: TestNormalizeFrame/unfrozen_numeric_day_age (0.00s)
    --- PASS: TestNormalizeFrame/frozen_values_preserved (0.00s)
PASS
ok  	github.com/n-orlov/deck/features	10.167s
?   	github.com/n-orlov/deck/internal/agent	[no test files]
=== RUN   TestJSONLRecordsTransitionsAndLaunchAudit
=== PAUSE TestJSONLRecordsTransitionsAndLaunchAudit
=== RUN   TestDurationAdvancesWithFrozenWallClock
=== PAUSE TestDurationAdvancesWithFrozenWallClock
=== RUN   TestSessionAuditRequiresSessionID
=== PAUSE TestSessionAuditRequiresSessionID
=== CONT  TestJSONLRecordsTransitionsAndLaunchAudit
=== CONT  TestSessionAuditRequiresSessionID
--- PASS: TestSessionAuditRequiresSessionID (0.00s)
--- PASS: TestJSONLRecordsTransitionsAndLaunchAudit (0.00s)
=== CONT  TestDurationAdvancesWithFrozenWallClock
--- PASS: TestDurationAdvancesWithFrozenWallClock (0.02s)
PASS
ok  	github.com/n-orlov/deck/internal/audit	0.019s
=== RUN   TestPathsRespectDeckHomeAndXDG
--- PASS: TestPathsRespectDeckHomeAndXDG (0.00s)
=== RUN   TestControlsAndClock
--- PASS: TestControlsAndClock (0.00s)
=== RUN   TestSeededUUIDsAreStableAndValid
--- PASS: TestSeededUUIDsAreStableAndValid (0.00s)
=== RUN   TestInvalidControlsAreRejected
=== RUN   TestInvalidControlsAreRejected/DECK_CLOCK_STEP
=== RUN   TestInvalidControlsAreRejected/DECK_RECONCILE_MS
=== RUN   TestInvalidControlsAreRejected/DECK_PREVIEW_MS
=== RUN   TestInvalidControlsAreRejected/DECK_TMUX_SOCKET
=== RUN   TestInvalidControlsAreRejected/DECK_ASCII
=== RUN   TestInvalidControlsAreRejected/DECK_CLOCK
--- PASS: TestInvalidControlsAreRejected (0.00s)
    --- PASS: TestInvalidControlsAreRejected/DECK_CLOCK_STEP (0.00s)
    --- PASS: TestInvalidControlsAreRejected/DECK_RECONCILE_MS (0.00s)
    --- PASS: TestInvalidControlsAreRejected/DECK_PREVIEW_MS (0.00s)
    --- PASS: TestInvalidControlsAreRejected/DECK_TMUX_SOCKET (0.00s)
    --- PASS: TestInvalidControlsAreRejected/DECK_ASCII (0.00s)
    --- PASS: TestInvalidControlsAreRejected/DECK_CLOCK (0.00s)
PASS
ok  	github.com/n-orlov/deck/internal/config	0.004s
?   	github.com/n-orlov/deck/internal/hookrecv	[no test files]
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
=== RUN   TestReconcilerStopsDisappearedSessionAndDoesNotRelaunchServer
--- PASS: TestReconcilerStopsDisappearedSessionAndDoesNotRelaunchServer (0.11s)
=== RUN   TestCreateShellPersistsLaunchesAndAudits
--- PASS: TestCreateShellPersistsLaunchesAndAudits (0.09s)
=== RUN   TestKillStopsSessionPreservesCWDAndRecordsTransition
--- PASS: TestKillStopsSessionPreservesCWDAndRecordsTransition (0.09s)
=== RUN   TestCreateShellRecordsCoherentFailureWhenTmuxCannotLaunch
--- PASS: TestCreateShellRecordsCoherentFailureWhenTmuxCannotLaunch (0.06s)
PASS
ok  	github.com/n-orlov/deck/internal/service	0.361s
=== RUN   TestOpenInitializesPrivateV1WALStore
--- PASS: TestOpenInitializesPrivateV1WALStore (0.05s)
=== RUN   TestOpenMigratesSupportedVersionZeroFixture
--- PASS: TestOpenMigratesSupportedVersionZeroFixture (0.08s)
=== RUN   TestOpenRefusesNewerFixtureWithoutMutation
--- PASS: TestOpenRefusesNewerFixtureWithoutMutation (0.04s)
=== RUN   TestCreateSessionIsTargetedAndEnforcesNameAndSlugUniqueness
--- PASS: TestCreateSessionIsTargetedAndEnforcesNameAndSlugUniqueness (0.05s)
=== RUN   TestUpdateSessionStatusIsTargetedRecordsEventAndListsStoppedRows
--- PASS: TestUpdateSessionStatusIsTargetedRecordsEventAndListsStoppedRows (0.06s)
=== RUN   TestSlugContainsOnlyTmuxSafeASCII
--- PASS: TestSlugContainsOnlyTmuxSafeASCII (0.00s)
PASS
ok  	github.com/n-orlov/deck/internal/store	0.284s
=== RUN   TestDiscoverRealTmux
--- PASS: TestDiscoverRealTmux (0.00s)
=== RUN   TestDiscoverRejectsMissingTmux
--- PASS: TestDiscoverRejectsMissingTmux (0.00s)
=== RUN   TestDiscoverRejectsPre32Tmux
--- PASS: TestDiscoverRejectsPre32Tmux (0.00s)
=== RUN   TestCreateListAndKillRealTmux
--- PASS: TestCreateListAndKillRealTmux (0.04s)
=== RUN   TestAttachHelper
--- PASS: TestAttachHelper (0.00s)
=== RUN   TestAttachThroughPTY
--- PASS: TestAttachThroughPTY (0.04s)
=== RUN   TestLifecycleRejectsUnsafeInput
--- PASS: TestLifecycleRejectsUnsafeInput (0.00s)
=== RUN   TestBootstrapConfiguresOnlyPrivateServer
--- PASS: TestBootstrapConfiguresOnlyPrivateServer (0.02s)
PASS
ok  	github.com/n-orlov/deck/internal/tmux	0.112s
=== RUN   TestEmptyAndHelpViewsAreDiscoverable
--- PASS: TestEmptyAndHelpViewsAreDiscoverable (0.00s)
=== RUN   TestASCIIColorAndFrozenRelativeTimeRendering
--- PASS: TestASCIIColorAndFrozenRelativeTimeRendering (0.01s)
=== RUN   TestSuccessfulCreationAdvancesConfiguredFrozenClock
--- PASS: TestSuccessfulCreationAdvancesConfiguredFrozenClock (0.00s)
=== RUN   TestHelpTogglesAndEscapeCloses
--- PASS: TestHelpTogglesAndEscapeCloses (0.00s)
PASS
ok  	github.com/n-orlov/deck/internal/tui	0.015s
?   	github.com/n-orlov/deck/internal/unit	[no test files]

real	0m11.116s
user	0m0.016s
sys	0m0.023s
```

## Package results

All packages returned `ok` with no `FAIL` anywhere in the run:

```
ok  	github.com/n-orlov/deck/cmd/deck	4.246s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.002s
ok  	github.com/n-orlov/deck/features	10.167s
ok  	github.com/n-orlov/deck/internal/audit	0.019s
ok  	github.com/n-orlov/deck/internal/config	0.004s
ok  	github.com/n-orlov/deck/internal/service	0.361s
ok  	github.com/n-orlov/deck/internal/store	0.284s
ok  	github.com/n-orlov/deck/internal/tmux	0.112s
ok  	github.com/n-orlov/deck/internal/tui	0.015s
```
All nine `ok` package lines from the raw transcript are reproduced above.
(`internal/agent`, `internal/hookrecv`, `internal/notify`, `internal/search` and
`internal/unit` report `[no test files]`, which is not a failure — they are
package-doc placeholders for later phases.)

## Godog feature/scenario/step totals

The single `TestFeatures` run against the real `features/*.feature` files
reported:

```
13 scenarios (13 passed)
92 steps (92 passed)
```

`TestGodogRejectsUndefinedAndFailedSteps` is a separate, self-contained unit
test of `features`' own Godog wiring: it deliberately runs two synthetic,
throwaway inner Godog suites (one with a step that always fails, one with an
undefined step) against temp-file fixtures it creates itself, and asserts
that Godog correctly reports them as failed/undefined. Their totals —
`1 scenarios (1 failed)` / `1 steps (1 failed)` and `1 scenarios (1
undefined)` / `1 steps (1 undefined)` — are the *expected, asserted* output
of that inner self-test, not failures of the outer suite; the outer test
itself reports `--- PASS: TestGodogRejectsUndefinedAndFailedSteps`. They are
not part of the real feature-suite totals above and must not be confused
with them.

## Wall timing

```
real	0m11.116s
user	0m0.016s
sys	0m0.023s
```

Overall command exit status: `0`.
