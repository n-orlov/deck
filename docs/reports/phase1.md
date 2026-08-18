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
| 1 | Adapter registry declares capabilities; adding an adapter needs no TUI change | `internal/agent/agent_test.go: TestRegistry_RegisterAndLookup`, `TestRegistry_KindsIsStableAndSorted`, `TestCaps_SupportsProfile` (registry-side declaration only); the TUI-side half of this requirement — that the TUI *derives* its adapter kinds/capabilities from the registry rather than hardcoding them — was found unproved by review and is now closed and proved by: `internal/tui/create_registry_agent_options_test.go: TestCreateModalAgentFieldDerivesFromRegistry` (Agent field options and cycling order come from a registry containing a throwaway kind absent from any internal/tui source string); `internal/tui/registry_capabilities_test.go: TestRegistryDrivenCapabilities_ProfileBadgeCreateAndPin` (profile offer list, badge, and `P`/`p` paths for a throwaway adapter's own declared profile set, all with zero internal/tui edits); `internal/tui/registry_guard_test.go: TestBlackBoxRegistrySwapNeedsNoTUIEdit` (black-box: extra kind present + a stock kind absent, extra kind's profiles including a degradation reason for an unsupported one, comment explicitly names PRD requirement 1); `internal/tui/registry_guard_test.go: TestDefaultCreateAgentPrefersShellRegardlessOfSortOrder` (pins that the create modal's default-agent selection stays registry-derived without accidentally depending on `Kinds()` alphabetical order, and covers the shell-absent fallback to `Kinds()[0]`). Together these prove that adding an adapter to `internal/agent`'s registry is sufficient — the Agent field, profile offers, badges, degradation reasons and default selection all update with zero corresponding change to `internal/tui`. |
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

## Operator smoke-test walkthrough (task 038)

This section is a manual, copy-pasteable walkthrough of the T1 reboot-survival
promise (SPEC §13.4, PRD requirement 22) against a real `claude` CLI, run from
a normal shell (not the CI sibling container) with `claude` on `PATH`. Every
command and keystroke below is consistent with `cmd/deck/main.go` (which takes
no flags — it only reads `DECK_*` env vars documented in the help overlay, see
`internal/tui/tui.go`'s `helpView`) and with
`features/real_agent_smoke.feature` (the one scenario that exercises a real
CLI in the test suite).

**0. Isolate the run.** deck keeps all mutable state under one `DECK_HOME`
root (`internal/config/config.go`'s `resolvePaths`) and all tmux panes on one
named socket (`DECK_TMUX_SOCKET`, default `deck`) — both worth pointing at a
scratch directory / throwaway socket name so this walkthrough never touches a
real login tmux server or a real `~/.deck`:

```sh
export DECK_HOME=$(mktemp -d)
export DECK_TMUX_SOCKET=deck-smoke-$$
mkdir -p /tmp/deck-smoke-cwd
```

**1. Build deck.**

```sh
go build -o /tmp/deck-smoke ./cmd/deck
```

What "working" looks like: exit 0, and `/tmp/deck-smoke` exists and is
executable.

**2. Create a real claude session.** Run `/tmp/deck-smoke`, then:

- press `n` to open the create modal,
- type a name, e.g. `smoke`,
- press `tab` to move to the working-directory field and type
  `/tmp/deck-smoke-cwd`,
- press `tab` to reach the agent field, press `right` once to cycle from
  `shell` to `claude`,
- press `tab` to reach the permission-profile field; leave it at `safe`
  (the default and first-listed option, `internal/tui/tui.go`'s
  `createProfileOptions`),
- press `enter` to create.

What "working" looks like: the modal closes, the row for `smoke` appears in
the list, and its status reads `starting · awaiting signal` (ASCII fallback
`starting - awaiting signal` with `DECK_ASCII=1`) — this is the known Phase 1
rough edge below, not a bug to chase.

**3. Exchange a message.** With `smoke` selected, press `enter` to attach to
its tmux pane (deck execs into `tmux attach` for that pane); interact with
the real `claude` CLI directly in the attached pane and send it one message
so it has conversation history to prove identity with later. Detach with
tmux's own detach keystroke (`ctrl+b d` under the default tmux prefix) to
return to the deck list — deck does not intercept or remap this.

What "working" looks like: back in the deck list, `smoke` still shows a
status; `claude` produced at least one reply in the pane before you detached.

**4. Kill the tmux server as a reboot stand-in.** Quit deck (`q`), then:

```sh
tmux -L "$DECK_TMUX_SOCKET" kill-server
```

What "working" looks like: `tmux -L "$DECK_TMUX_SOCKET" list-sessions`
now fails with "no server running on ..." — there is no live pane anywhere,
matching SPEC §13.4's reboot stand-in, while `$DECK_HOME/state.db` on disk is
untouched.

**5. Restart deck.**

```sh
/tmp/deck-smoke
```

What "working" looks like: `smoke` is listed with status `stopped`, badged
resumable, sourced entirely from `$DECK_HOME/state.db` — no tmux session
exists yet.

**6. Press `r` to resume, and confirm the same conversation returned.** With
`smoke` selected, press `r`.

What "working" looks like: the row's status becomes
`starting · awaiting signal` (never `running` — deck does not derive
liveness from tmux; see task 019). Press `enter` to attach again: the `claude`
CLI resumes with `--resume <smoke's conversation id>` (never `--continue` or
a "most recent" form — SPEC §13.4/§2), and the pane shows the same
conversation history exchanged in step 3, not a fresh session. The exact
argv used for the resume, and `smoke`'s conversation id, are recorded (argv
plus env variable *names* only, never values, per SPEC §6.4/task 009) in
`$DECK_HOME/log/deck.jsonl` and can be inspected with:

```sh
grep smoke "$DECK_HOME"/log/deck.jsonl | tail -1
```

**7. Clean up.**

```sh
tmux -L "$DECK_TMUX_SOCKET" kill-server 2>/dev/null
rm -rf "$DECK_HOME" /tmp/deck-smoke-cwd /tmp/deck-smoke
```

**The one documented command to run the automated `@real-agents` scenario**
(from the repository root, with a real `claude` on `PATH`):

```sh
DECK_GODOG_TAGS=@real-agents go test -run TestFeatures -v ./features/...
```

(see the `@real-agents` section above for what it proves and why it is
excluded from the default suite).

**Known Phase 1 rough edge.** Every agent-adapter row (claude, pi) reads
`starting · awaiting signal` from the moment it is launched or resumed and
stays there for the rest of the run — deck deliberately never claims
`running` because Phase 1 has no hook/probe mechanism to observe the agent's
own state (that lands in Phase 2; see `docs/reports/phase1-findings.md`).
This is expected, not a defect: the pane itself is fully live and usable via
`enter`/attach even while the row still reads `starting · awaiting signal`.

## Final verification (task 040)

Run from the repository root at `HEAD c9469dd` on branch `main`, using the
sibling toolchain (`ci/run.sh`, per `ci/SPIKE.md`):

```
$ ci/run.sh go build ./...
(no output, exit 0)

$ ci/run.sh go vet ./...
(no output, exit 0)

$ ci/run.sh go test -count=1 ./...
ok  	github.com/n-orlov/deck/cmd/deck	4.656s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.008s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.003s
ok  	github.com/n-orlov/deck/features	33.270s
ok  	github.com/n-orlov/deck/internal/agent	0.003s
ok  	github.com/n-orlov/deck/internal/audit	0.021s
ok  	github.com/n-orlov/deck/internal/config	0.010s
?   	github.com/n-orlov/deck/internal/hookrecv	[no test files]
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	1.887s
ok  	github.com/n-orlov/deck/internal/store	0.597s
ok  	github.com/n-orlov/deck/internal/tmux	0.201s
ok  	github.com/n-orlov/deck/internal/tui	0.021s
?   	github.com/n-orlov/deck/internal/unit	[no test files]
```

All packages pass, exit 0.

**No scenario disabled to make the suite pass.**

```
$ grep -rn '@nightly\|@skip\|t.Skip\|godog.ErrPending' features/ cmd/ internal/
features/godog_test.go:15:const defaultTags = "~@real-agents && ~@nightly"
```

The only hit is the default godog tag-filter constant itself (excludes
`@real-agents`, the deliberate real-`claude`-on-`PATH` smoke test documented
above and in task 029, and `@nightly`, a tag not used by any scenario in this
phase — `grep -rn '@nightly' features/*.feature` matches nothing). No
`@skip` tag, `t.Skip` call, or `godog.ErrPending` exists anywhere in the
tree.

**Read-only baseline untouched.**

```
$ git diff 758b5dc --stat -- SPEC.md ci/Dockerfile ci/SPIKE.md
(empty)

$ git diff 2dacc4f --stat -- prds/
(empty)
```

`SPEC.md`, `ci/Dockerfile` and `ci/SPIKE.md` remain byte-identical to the
phase's first commit; `prds/` has no changes beyond the one sanctioned
operator edit (`2dacc4f`, the docker-sweep guardrail).

**Tree state.**

```
$ git status --porcelain
(empty)

$ git rev-parse HEAD origin/main
c9469dd4a514425410a2404aae20ace5bdd0efce
c9469dd4a514425410a2404aae20ace5bdd0efce

$ git branch --show-current
main
```

The working tree is clean, `HEAD` equals `origin/main`, and the checkout is
on branch `main` (not detached). No force-push, `commit --amend` or rebase
was used for any Phase 1 task, including this one — every task's commit is
a plain fast-forward `git commit` followed by `git push`.

Phase 1 is complete: all 40 tasks in the plan are done, verified against
their stated success criteria, and pushed to `origin/main`.

## Registry/TUI coupling fix re-verification (task 007, R1 rework)

Re-run after tasks 001-006 (agent-registry-derived Agent field, capabilities
and profile applicability; login_shell/captured_path help wording), which
closed the Phase 1 review finding that the TUI hardcoded adapter kinds and
capability logic instead of deriving them from `internal/agent`'s registry.

**Regression found and fixed during this re-run.** The first full-suite run
after tasks 002/003 landed hung/failed
`TestDeckBinaryShellCreateAndSlugCollisionThroughPTY` and
`TestDeckBinaryRefreshesAllConcurrentClients` in `cmd/deck`. Root cause: the
create modal's default agent on pressing `n` had been
`m.registry().Kinds()[0]`, and `agent.Registry.Kinds()` returns kinds sorted
alphabetically (`claude`, `pi`, `shell`) rather than the old hardcoded
`{"shell", "claude", "pi"}` order. The modal now opens on `claude`, but only
the `shell` adapter supports real session creation in this phase (see the
`createAgent != "shell"` guard in `internal/tui/tui.go`), so the PTY tests -
which drive `n` and expect the shell create flow to start unattended - timed
out waiting for `"Create shell session"`. This was **not** a sandbox/PTY
infra flake; earlier attempts had misdiagnosed it as one. Fixed by adding
`defaultCreateAgent(kinds []string) string` in `internal/tui/tui.go`, which
prefers `"shell"` when present in the registry's kind list and otherwise
falls back to the first kind; the `n` key handler now calls it instead of
indexing `Kinds()[0]` directly. This keeps the Agent field's cycling order
registry-derived (per task 002) while keeping the modal's opening selection
independent of where `"shell"` happens to sort alphabetically.

```
$ ci/run.sh go build ./...
(no output, exit 0)

$ ci/run.sh go vet ./...
(no output, exit 0)

$ time ci/run.sh go test -count=1 -timeout=480s ./...
ok  	github.com/n-orlov/deck/cmd/deck	4.240s
ok  	github.com/n-orlov/deck/cmd/fake-claude	0.004s
ok  	github.com/n-orlov/deck/cmd/fake-pi	0.002s
ok  	github.com/n-orlov/deck/features	31.958s
ok  	github.com/n-orlov/deck/internal/agent	0.002s
ok  	github.com/n-orlov/deck/internal/audit	0.018s
ok  	github.com/n-orlov/deck/internal/config	0.006s
?   	github.com/n-orlov/deck/internal/hookrecv	[no test files]
?   	github.com/n-orlov/deck/internal/notify	[no test files]
?   	github.com/n-orlov/deck/internal/search	[no test files]
ok  	github.com/n-orlov/deck/internal/service	1.632s
ok  	github.com/n-orlov/deck/internal/store	0.502s
ok  	github.com/n-orlov/deck/internal/tmux	0.123s
ok  	github.com/n-orlov/deck/internal/tui	0.019s
?   	github.com/n-orlov/deck/internal/unit	[no test files]

real	0m32.846s
```

All packages pass, exit 0. `cmd/deck`'s three PTY-driven tests
(`TestDeckBinaryShellCreateAndSlugCollisionThroughPTY`,
`TestDeckBinaryRefreshesAllConcurrentClients`,
`TestDeckBinaryEmptyHelpAndQuitThroughPTY`) are included and green in this
run — they were the ones affected by the regression above.

**No scenario deleted or newly excluded.**

```
$ git diff 758b5dc..HEAD -- features/ | grep -c '^-.*Scenario:'
0
```

No `Scenario:` line was removed from `features/` across the whole registry
rework (tasks 001-007 inclusive), and no new `@skip`/`@nightly`-style tag was
introduced to dodge a scenario; the default godog filter
(`~@real-agents && ~@nightly`) is unchanged from earlier phase attempts.

## 2026-08-18 — task 008: ten consecutive clean-state suite passes (HEAD 132f725)

Re-ran the stability check after the registry-driven-TUI rework and the
default-create-agent fix (tasks 001-008a), from HEAD `132f72555aa9ded04c7e9a676e87fe6debbac1e7` — a fixed tree strictly newer than the
`a99fcb0`-era run captured in the pre-existing
`docs/reports/phase1-ten-run-stability.log`.

```
$ time ci/stability.sh 10
```

Real elapsed: 5m38.6s. Full unedited output committed as
`docs/reports/phase1-ten-run-stability-132f725.log`:

```
$ grep -c '^--- FAIL\|^FAIL' docs/reports/phase1-ten-run-stability-132f725.log
0
$ tail -1 docs/reports/phase1-ten-run-stability-132f725.log
10/10 passed
```

10/10 runs passed with no `--- FAIL` or `^FAIL` line in the combined
summary log; no flake observed, so no code fix was required for this
report.
