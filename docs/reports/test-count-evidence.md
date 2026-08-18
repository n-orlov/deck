# Defined Go test count evidence

This report records the actual counting command and output used to state the
"how many Go tests exist" figure in `docs/reports/phase0.md`, derived from the
same uncached verbose suite capture documented in
`docs/reports/verbose-suite-capture.md`.

Source transcript: `docs/reports/verbose-suite-capture-source.log`. This is the
raw, unedited capture and still contains Godog's ANSI colour bytes, which is why
the counting command below anchors on `=== RUN` lines rather than on coloured
output. Stripping those bytes reproduces the readable transcript embedded in
`docs/reports/verbose-suite-capture.md` exactly.

## Counting convention

Only top-level `go test` invocations are counted, i.e. lines of the exact
form `=== RUN   TestXxx` with no `/` in them. Godog subtests reported by the
`testing` package as `=== RUN   TestFeatures/<scenario>/<step>` are
deliberately **excluded** by this pattern (the character class
`[A-Za-z0-9_]+` does not include `/`, so it cannot match past the top-level
test name), because they are not independent Go tests — they are steps of
the single `TestFeatures` Godog-driving test.

`TestFeatures` itself **is** counted as one top-level Go test by this
convention (it is one `go test` test function), but it is called out
separately below because it is a Godog umbrella test that drives many
feature-file scenarios/steps internally, not a "plain" unit test asserting
one thing directly.

## Counting command

Run from the repository root against the uncached verbose transcript:

```
grep -nE '^=== RUN   Test[A-Za-z0-9_]+$' docs/reports/verbose-suite-capture-source.log
```

## Full output (42 matches)

```
1:=== RUN   TestDeckBinaryShellCreateAndSlugCollisionThroughPTY
3:=== RUN   TestDeckBinaryRefreshesAllConcurrentClients
5:=== RUN   TestDeckBinaryEmptyHelpAndQuitThroughPTY
9:=== RUN   TestAcceptedClaudeFlagsProduceDeterministicTerminalRecord
11:=== RUN   TestRejectsInvalidUUIDsUnknownFlagsAndModes
21:=== RUN   TestExitCodeIsControlledOnlyByFixtureEnvironment
25:=== RUN   TestBlackBoxAssertionsObserveRealSession
27:=== RUN   TestFeatures
199:=== RUN   TestGodogRejectsUndefinedAndFailedSteps
235:=== RUN   TestScenarioHarnessTeardownReportsLeaks
243:=== RUN   TestScenarioHarnessSharesIsolationAndCleansUp
245:=== RUN   TestScreenDriverLaunchesDeckAndAnswersTerminalProbes
247:=== RUN   TestNormalizeFrame
262:=== RUN   TestJSONLRecordsTransitionsAndLaunchAudit
264:=== RUN   TestDurationAdvancesWithFrozenWallClock
266:=== RUN   TestSessionAuditRequiresSessionID
276:=== RUN   TestPathsRespectDeckHomeAndXDG
278:=== RUN   TestControlsAndClock
280:=== RUN   TestSeededUUIDsAreStableAndValid
282:=== RUN   TestInvalidControlsAreRejected
301:=== RUN   TestReconcilerStopsDisappearedSessionAndDoesNotRelaunchServer
303:=== RUN   TestCreateShellPersistsLaunchesAndAudits
305:=== RUN   TestKillStopsSessionPreservesCWDAndRecordsTransition
307:=== RUN   TestCreateShellRecordsCoherentFailureWhenTmuxCannotLaunch
311:=== RUN   TestOpenInitializesPrivateV1WALStore
313:=== RUN   TestOpenMigratesSupportedVersionZeroFixture
315:=== RUN   TestOpenRefusesNewerFixtureWithoutMutation
317:=== RUN   TestCreateSessionIsTargetedAndEnforcesNameAndSlugUniqueness
319:=== RUN   TestUpdateSessionStatusIsTargetedRecordsEventAndListsStoppedRows
321:=== RUN   TestSlugContainsOnlyTmuxSafeASCII
325:=== RUN   TestDiscoverRealTmux
327:=== RUN   TestDiscoverRejectsMissingTmux
329:=== RUN   TestDiscoverRejectsPre32Tmux
331:=== RUN   TestCreateListAndKillRealTmux
333:=== RUN   TestAttachHelper
335:=== RUN   TestAttachThroughPTY
337:=== RUN   TestLifecycleRejectsUnsafeInput
339:=== RUN   TestBootstrapConfiguresOnlyPrivateServer
343:=== RUN   TestEmptyAndHelpViewsAreDiscoverable
345:=== RUN   TestASCIIColorAndFrozenRelativeTimeRendering
347:=== RUN   TestSuccessfulCreationAdvancesConfiguredFrozenClock
349:=== RUN   TestHelpTogglesAndEscapeCloses
```

## Count check

```
$ grep -cE '^=== RUN   Test[A-Za-z0-9_]+$' docs/reports/verbose-suite-capture-source.log
42
```

```
$ grep -nE '^=== RUN   Test[A-Za-z0-9_]+$' docs/reports/verbose-suite-capture-source.log | grep -c TestFeatures
1
```

## Result

- **42** top-level Go tests exist under the convention above (one line per
  test function invocation, subtests excluded).
- Of those 42, **1** is `TestFeatures` (line 27), the Godog umbrella test
  that drives 13 feature scenarios / 92 Godog steps internally (see
  `docs/reports/verbose-suite-capture.md` for the scenario/step totals). The
  remaining 41 are plain unit/integration tests each asserting a single Go
  test function's behavior directly.
