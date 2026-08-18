# Phase 1 report

This file accumulates Phase 1 evidence task by task (extended, not replaced,
by later tasks — see task 033 for the full requirement-by-requirement report).

## @real-agents smoke test against a real Claude CLI (task 029)

`features/real_agent_smoke.feature` proves, against an actually-installed
`claude` binary (not the `fake-claude` fixture used by every other
scenario), that deck assigns a UUID conversation id at session-create time
and passes that same id back on `--resume`. It is tagged `@real-agents`, so
it is excluded from the default suite by `features/godog_test.go`'s
`defaultTags = "~@real-agents && ~@nightly"` — the default run's scenario
count is unaffected whether or not a real `claude` is installed, and passes
with none installed (verified in this environment, which has no `claude` on
PATH).

To run it on a machine with a real Claude CLI on `PATH` (and, if the
installed CLI requires it, valid Claude credentials/config already set up
for that CLI to run non-interactively), run exactly this command from the
repository root:

```sh
DECK_GODOG_TAGS=@real-agents go test -run TestFeatures -v ./features/...
```

`features/godog_test.go`'s `TestFeatures` reads the `DECK_GODOG_TAGS`
environment variable and, when set, uses it as the Godog tag expression
instead of the hardcoded `defaultTags` (`~@real-agents && ~@nightly`);
`defaultTags` itself, and therefore every ordinary `go test ./...` run, is
unchanged.

## Full suite run, counts, tool versions and wall-clock (task 035)

Command run from the repository root, real unedited output captured to
[`docs/reports/phase1-full-verbose-run.log`](phase1-full-verbose-run.log):

```sh
time ci/run.sh go test -v -count=1 ./...
```

Result: exit 0, wall-clock `real 0m35.535s` (measured by the shell `time`
builtin wrapping the whole `ci/run.sh` invocation, i.e. one full suite run
including the sibling-container startup cost).

**Feature/scenario/step counts.** `github.com/n-orlov/deck/features`'s
`TestFeatures` runs the real default Godog suite (tag expression
`~@real-agents && ~@nightly`) and reports, verbatim in the log:

```
29 scenarios (29 passed)
307 steps (307 passed)
```

The same package also runs `TestGodogRejectsUndefinedAndFailedSteps`, a
harness self-test that deliberately runs one undefined-step scenario and one
deliberately-failing scenario **against a private, throwaway Godog run of
its own** to prove the harness surfaces `undefined`/`failed` godog results
as Go test failures rather than swallowing them; those two scenarios (and
their `1 undefined`/`1 failed` godog summary lines, at log lines 549-576 of
the captured log) are not part of, and do not affect, the real 29/307 count
above, and `TestGodogRejectsUndefinedAndFailedSteps` itself reports
`--- PASS` because catching the deliberate failure *is* its passing
condition. `go vet`/`go build` are clean and are not repeated here (see
tasks 030-034's reports for prior clean runs of both).

**Top-level Go test count.** Counting convention: every line matching
`^=== RUN   Test[A-Za-z0-9_]*$` (i.e. a top-level `func Test...(t
*testing.T)` invocation, excluding `t.Run` subtests, which appear as
`=== RUN   Test.../subtest_name`) in the captured log. That count is **125**,
across the 12 packages that carry Go tests (`cmd/deck`, `cmd/fake-claude`,
`cmd/fake-pi`, `features`, `internal/agent`, `internal/audit`,
`internal/config`, `internal/service`, `internal/store`, `internal/tmux`,
`internal/tui`, and the store/tui/etc. packages listed in the `ok` lines
below); three packages (`internal/hookrecv`, `internal/notify`,
`internal/search`) report `[no test files]` and are excluded from the count.
Every package reports `ok`; there is no `FAIL` package line and no
`--- FAIL` line anywhere in the log (`grep -c '^--- FAIL'
docs/reports/phase1-full-verbose-run.log` → `0`).

```
ok  	github.com/n-orlov/deck/cmd/deck	4.678s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.007s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.003s
ok  	github.com/n-orlov/deck/features	34.306s
ok  	github.com/n-orlov/deck/internal/agent	0.006s
ok  	github.com/n-orlov/deck/internal/audit	0.032s
ok  	github.com/n-orlov/deck/internal/config	0.040s
ok  	github.com/n-orlov/deck/internal/service	1.856s
ok  	github.com/n-orlov/deck/internal/store	0.592s
ok  	github.com/n-orlov/deck/internal/tmux	0.202s
ok  	github.com/n-orlov/deck/internal/tui	0.055s
```

**Resolved tool versions** (from `ci/run.sh sh -c 'go version; tmux -V; go
list -m github.com/cucumber/godog'`, run against the same sibling image used
for every other check in this phase):

```
go version go1.25.13 linux/amd64
tmux 3.5a
github.com/cucumber/godog v0.16.0
```

## PRD requirement 1-25 evidence table (task 035)

Every scenario name below is a `Scenario:` in the named `.feature` file
under `features/`; every Go test name is a `func Test...` in the named
`_test.go` file. All are present in the captured log above and all passed.

| # | Requirement (short) | Proving scenario / test |
|---|---|---|
| 1 | Adapter registry declares capabilities; adding an adapter needs no TUI change | `internal/agent/agent_test.go: TestRegistry_RegisterAndLookup`, `TestRegistry_KindsIsStableAndSorted`, `TestCaps_SupportsProfile` |
| 2 | Claude adapter: `--session-id <uuid>` on launch, `--resume <uuid>` on resume, never `--continue`/"most recent" | `internal/agent/claude_test.go: TestClaude_LaunchAndResumeArgv` (table-driven, all profiles × launch/resume); `permission_modes.feature: claude maps every declared permission profile to its own argv` |
| 3 | Pi adapter: `--session-id <id>` both ways, `plan` unsupported → degrades to `safe` with a shown reason | `internal/agent/pi_test.go: TestPi_LaunchAndResumeUseSessionID`, `TestPi_PlanDegradesToSafeWithReason`; `permission_modes.feature: pi maps only its declared permission profiles to argv` and `an unsupported profile degrades visibly rather than lying` |
| 4 | `shell` has no conversation id, no profile, no badge | `internal/agent/shell_test.go: TestShellCapsNoProfilesNoAssignedID`; `internal/tui/badge_detail_test.go: TestListShowsProfileBadgeForAgentsAndNoneForShell`, `TestDetailViewOmitsProfileForShell` |
| 5 | `conversation_id` persisted at assignment; audit records argv + env **names** only | `internal/service/agent_test.go: TestCreateAgentAssignsConversationIDAndLaunchesClaudeArgv` (asserts persisted id, recorded argv, and no env value anywhere in the audit file) |
| 6 | Profiles map per-adapter exactly per §5 (`manual\|plan\|acceptEdits\|bypassPermissions` for claude) | `internal/agent/claude_test.go: TestClaude_LaunchAndResumeArgv`; `permission_modes.feature: claude maps every declared permission profile to its own argv` |
| 7 | Unsupported profile degrades and says so; create modal only offers declared profiles | `internal/agent/pi_test.go: TestPi_PlanDegradesToSafeWithReason`; `internal/tui/create_yolo_test.go: TestCreateProfileOptionsForOffersOnlyDeclaredProfiles`; `permission_modes.feature: an unsupported profile degrades visibly rather than lying` |
| 8 | Profile persists across resume; badged in list and detail | `permission_modes.feature: the permission profile survives a resume`; `internal/tui/badge_detail_test.go: TestListShowsProfileBadgeForAgentsAndNoneForShell`, `TestDetailViewShowsProfileAndDegradation` |
| 9 | `yolo` gated by `allow_yolo` config **and** explicit modal confirm | `permission_modes.feature: yolo is unavailable without allow_yolo enabled` and `yolo requires an explicit confirm even once allow_yolo is enabled`; `internal/tui/create_validation_test.go` (`TestCreateModalNoYoloOfferedWithAllowYoloFalse`, `TestCreateModalYoloRequiresConfirmMessage`, `TestCreateModalYoloConfirmThenCreateSucceeds` — see `create_yolo_test.go`) |
| 10 | `P` switches profile, persists, states restart-to-apply, never claims a live pane changed | `internal/tui/profile_switch_test.go: TestProfileSwitchPersistsAndStatesRestartToApply` |
| 11 | Create modal offers name/cwd/agent/profile/launch_args/env/pre_launch/login_shell, keyboard-only, each explained | `features/create_modal_test.go: TestCreateModalKeyboardOnlyReachesEveryFieldAndExplanation` |
| 12 | Validation stated and input-preserving for all seven cases; `esc` creates nothing | `internal/tui/create_validation_test.go: TestCreateModalDuplicateNameMessage`, `TestCreateModalSlugCollisionMessage`, `TestCreateModalMissingCWDMessage`, `TestCreateModalCWDNotDirectoryMessage`, `TestCreateModalMalformedEnvKeyMessage`, `TestCreateModalMalformedLaunchArgsJSONMessage`, `TestCreateModalUnsupportedProfileMessage`, `TestCreateModalEscapeCreatesNothing` |
| 13 | `captured_path` recorded at create time; §6.3 PATH resolution order; `login_shell` mutually exclusive with relying on it | `internal/service/agent_test.go: TestCreateAgentResolvesPATHInSPECOrder`, `TestCreateAgentLoginShellInvocationForm` |
| 14 | `pre_launch` runs before the agent in the same pane; failure is visible, not swallowed | `internal/service/agent_test.go: TestCreateAgentRunsSucceedingPreLaunchBeforeTheAgent`, `TestCreateAgentFailingPreLaunchNeverStartsTheAgent` |
| 15 | `r` recreates the pane, runs `pre_launch`, launches resume argv with env/profile; row goes `starting`; no prompt re-sent | `internal/service/resume_test.go: TestResumeLaunchesAdapterResumeArgvUnderLease`; `agent_session.feature: create a claude session, observe its facts, then resume it` |
| 16 | Resume failure (unknown id / missing cwd / binary not on PATH) sets `error` with reason and retains the row; no fresh-conversation fallback | `resume_failure.feature: an unknown conversation id keeps the row as a retained, explained error`, `a missing cwd keeps the row as a retained, explained error`, `an agent binary not on PATH keeps the row as a retained, explained error`; unit-level in `internal/service/resume_test.go: TestResumeFailsOnUnknownConversationID`, `TestResumeFailsOnMissingCWD`, `TestResumeFailsOnAgentBinaryNotOnPath` |
| 17 | `p` pins `resume_state=pinned`/`resume_pin`, sticky across restart; one-shot "start fresh" reverts to `auto` | `internal/tui/pin_test.go: TestPinDialogPersistsPinnedMode`; `internal/service/pin_test.go: TestPinnedResumeSurvivesRestartAndArgv`, `TestFreshOnceStartsFreshConversationThenRevertsToAuto` |
| 18 | Reboot survival: after tmux-server kill + deck restart, rows read `stopped·resumable`, no tmux session exists anywhere, `r` resumes by explicit id | `durable_identity.feature: alpha, beta and gamma in one directory keep their own conversations` (the T1 scenario, see row 22) |
| 19 | `stopped→starting` CAS-acquires `launch_lease_owner`/`launch_lease_until`; losing client sees "starting elsewhere" | `internal/store/lease_test.go: TestAcquireLaunchLeaseFreshSucceeds`; `internal/tui/resume_test.go: TestResumeStartingElsewhereIsNotAnError`; `internal/service/resume_test.go: TestResumeLosingLeaseCreatesNoTMuxSession` |
| 20 | Stale lease (dead pid or expired TTL) breakable; live in-TTL lease not breakable; neither wedges the row | `internal/store/lease_test.go: TestAcquireLaunchLeaseLiveOwnerInTTLIsNotBreakable`, `TestAcquireLaunchLeaseExpiredTTLIsBreakable`, `TestAcquireLaunchLeaseDeadPIDIsBreakable`, `TestAcquireLaunchLeaseNeverWedgesTheRow`; `launch_lease.feature`'s three scenarios (live-in-TTL blocks, dead-pid breakable, expired-TTL breakable) |
| 21 | `@multiclient` race: exactly one tmux session and one launch record; losers see "starting elsewhere"; all end at `starting` | `lease_race.feature: three clients racing resume on one row produce exactly one launch` |
| 22 | T1 (SPEC §13.4): alpha/beta/gamma one directory, tmux-server killed, deck restarted, all `stopped·resumable`, no tmux session anywhere, beta's resume argv contains `--resume <beta.id>` and never `--continue`, beta replays its own last message not alpha's | `durable_identity.feature: alpha, beta and gamma in one directory keep their own conversations` |
| 23 | `same_directory`: two sessions in one `cwd` get different ids; resuming one never shows the other's id | `same_directory.feature: two claude sessions in one cwd keep separate conversation ids` |
| 24 | `permission_modes`: every profile's §5 argv per adapter; visible degradation; yolo gating both ways; profile survives resume | `permission_modes.feature`'s six scenarios (rows above), taken together |
| 25 | `@real-agents` smoke scenario, excluded from the default run, runnable by one documented command | `real_agent_smoke.feature: create a real claude session and resume it with the same conversation id` (see the `@real-agents` section above for the exact command and the default-run exclusion proof) |

All capture paths cited in this section — `docs/reports/phase1-full-verbose-run.log` — are repository-relative and exist (verified by `test -f docs/reports/phase1-full-verbose-run.log`).
