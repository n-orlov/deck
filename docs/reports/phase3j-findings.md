# Phase 3j findings

Companion to the phase closeout report task 034 writes (that task's own `successCriteria` in
the run's own task-state record names its path in prose rather than by a backticked path, because
every repo path this report cites is one already tracked in git — see §7): what the requirement
table does not carry — task 011, which ended `failed`, was reopened by operator ruling 001-011,
and was then closed by commit `9fb6aec`, after which the two tasks that had depended on it,
013 and 026, were subsequently delivered (commits `259284b` and `6080c55`) rather than left
unfinished (§1), gaps a validation pass found that were then closed within the same task rather
than left open, the
protected-path audit's disposition over this run's own commit range, this phase's check for a
disagreement between the tree and `SPEC.md`, the schema-version pins in `features/` that task
010's `SchemaVersion` bump left stale: the two stale schema-version literals in `features/` were
fixed by task 030's own commit `204af7dad3b4766ee82f80edf45f9f7f9c7d920b` (§6 has the full
disposition), and all three gate dispositions — the whole-suite sweep, the verbose tally
companion and the ten-run stability gate — are published, not pending: each was re-run at task
080's final code sha `4fbd452430501805a860dd229ddca1cd3f5c1cd6` and its disposition is recorded
in §8.

Written by task 029 against the tree at commit `17cabb871d0c8c17dc742256632015ed1889db9b` (the
last commit touching `*.go`/`*.feature` as of that writing) and kept current since by later tasks
in this same run, most recently approach 05's task 080, whose own comment-accuracy commit
`4fbd452430501805a860dd229ddca1cd3f5c1cd6` is now the final code sha per
`git log -1 --format=%H -- '*.go' '*.feature'` — this opening section is updated in place rather
than repeating a stale ancestry argument. Every sha cited resolves under `git cat-file -e` and
every path cited is tracked under `git ls-files --error-unmatch`, both checked in
[§7](#7-how-to-re-check-every-citation-in-this-report).

- [1. Task 011 ended `failed`, was reopened by operator ruling 001-011, and was closed by commit `9fb6aec`; tasks 013 and 026 were then delivered](#1-task-011-ended-failed-was-reopened-by-operator-ruling-001-011-and-was-closed-by-commit-9fb6aec-tasks-013-and-026-were-then-delivered)
- [2. Three tasks (002, 007, 023) had a validation-found gap that was closed within the same task's own follow-up commit, not left open](#2-three-tasks-002-007-023-had-a-validation-found-gap-that-was-closed-within-the-same-tasks-own-follow-up-commit-not-left-open)
- [3. One SPEC-versus-tree disagreement found in R104–R109's landed work — closed by task 038 at `2e5fc6b` and `566cb6d`](#3-one-spec-versus-tree-disagreement-found-in-r104r109s-landed-work--closed-by-task-038-at-2e5fc6b-and-566cb6d)
- [4. Protected-path audit over this phase's commit range is clean; carried-forward out-of-scope findings checked by name against task 030's whole-suite sweep, none recurred](#4-protected-path-audit-over-this-phases-commit-range-is-clean-carried-forward-out-of-scope-findings-checked-by-name-against-task-030s-whole-suite-sweep-none-recurred)
- [5. A pre-existing stale schema-version pin in `features/assertions_test.go`, found and fixed in flight by task 027](#5-a-pre-existing-stale-schema-version-pin-in-featuresassertions_testgo-found-and-fixed-in-flight-by-task-027)
- [6. Two stale schema-version literals in `features/` — fixed by task 030 at `204af7d`](#6-two-stale-schema-version-literals-in-features--fixed-by-task-030-at-204af7d)
- [7. How to re-check every citation in this report](#7-how-to-re-check-every-citation-in-this-report)
- [8. Gate dispositions: whole-suite sweep (task 058), verbose tally companion (task 059) and ten-run stability (task 060)](#8-gate-dispositions-whole-suite-sweep-task-058-verbose-tally-companion-task-059-and-ten-run-stability-task-060)
- [9. Independent review's blocking findings 1-3 (approach 01) are closed](#9-independent-reviews-blocking-findings-1-3-approach-01-are-closed)
- [10. Independent review's blocking findings 4 and 5 (record accuracy and gate-polling/one-sweep discipline) are closed](#10-independent-reviews-blocking-findings-4-and-5-record-accuracy-and-gate-pollingone-sweep-discipline-are-closed)
- [11. Approach 04's independent review finding 3 (R110 record still did not match the tree) is closed](#11-approach-04s-independent-review-finding-3-r110-record-still-did-not-match-the-tree-is-closed)

## 1. Task 011 ended `failed`, was reopened by operator ruling 001-011, and was closed by commit `9fb6aec`; tasks 013 and 026 were then delivered

Task 011 ("Plumb `post_destroy` through the store's session write and read paths") first ended
`failed` in the run's own task-state record, with `failureKind: "validation-exhausted"`, after 3
validation attempts.

**What failed, quoted verbatim from task 011's own `validationNotes` at that time:**

> Residual gap: CreateShell cannot accept or persist post_destroy because
> internal/service/shell.go's ShellCreateInput has no PostDestroy field and its
> store.CreateSessionInput omits PostDestroy; only CreateAgent propagates it.

The same note confirmed what task 011 had already landed and verified: `store.Session` and
`store.CreateSessionInput` both carried `PostDestroy`; `CreateSession` wrote the
`post_destroy` column; `sessionColumns`/`scanSession` read it back;
`TestCreateSessionRoundTripsAllPhase1FieldsAcrossReopen` proved a non-empty value survived
`GetSession` and `ListSessions`; `CreateAgent` passed `input.PostDestroy` through; and
`ci/run.sh go test -count=1 ./internal/store ./internal/service` exited 0 against that state.
Only the `ShellCreateInput`/`CreateShell` residual quoted above was missing at that point.

**The operator then reopened task 011.** Operator ruling 001-011 — delivered as a steering
message to the run, cited here by its ruling id rather than by its run-state filename — reset
task 011 from `failed` to `pending` with its criteria narrowed to exactly that residual: add
`PostDestroy` to `ShellCreateInput`, pass it into `CreateShell`'s `store.CreateSessionInput`, add a service test proving a shell row
created with a non-empty `post_destroy` retains it durably, and require
`ci/run.sh go test -count=1 ./internal/service ./internal/store` to exit 0 — everything task 011
had already landed and verified (the paragraph above) was left alone.

**Task 011 was closed by commit `9fb6aec`** ("service: pass post_destroy through CreateShell
(task 011)"), confirmed fresh against the committed code:

```
$ grep -n "PostDestroy" internal/service/shell.go
31:	// PostDestroy is this shell session's own teardown hook (SPEC §9.2,
32:	// R107), persisted verbatim into store.CreateSessionInput.PostDestroy.
36:	PostDestroy string
66:	// GlobalPostDestroy mirrors config.toml's top-level post_destroy key
73:	GlobalPostDestroy string
156:		PreLaunch: input.PreLaunch, PostDestroy: input.PostDestroy,
```

`ShellCreateInput` now carries its own `PostDestroy` field (line 36), and `CreateShell`'s
`store.CreateSessionInput{...}` construction now sets it (line 156). Commit `9fb6aec` also added
`TestCreateShellPersistsPostDestroyDurably` (`internal/service/shell_test.go`, line 95), which creates
a shell with `PostDestroy: "rm -rf /tmp/scratch"` and asserts the durable row returned from the
store still carries it. A `shell` session created today can carry a `post_destroy` value exactly
as a `claude`/`pi` session created through `CreateAgent` already could.

**Tasks 013 and 026, once blocked by task 011's `failed` status, were subsequently delivered
rather than left blocked.** Task 013 ("Run session-then-global `post_destroy` after Archive and
Delete durably succeed") landed in commit `259284b` ("service: run session-then-global
post_destroy after Archive/Delete commit (task 013)"). Task 026 ("Add a `post_destroy` field to
the create modal beside `pre_launch`") landed in commit `6080c55` ("tui: add post_destroy field
to create modal beside pre_launch (task 026)"). Both commits resolve under
`git cat-file -e <sha>^{commit}` against this tree. This section is now a historical record of
how the block was cleared, not a disclosure of an open one.

## 2. Three tasks (002, 007, 023) had a validation-found gap that was closed within the same task's own follow-up commit, not left open

Unlike task 011, three other tasks recorded a validation-found gap in `validationNotes` whose
history shows the gap closed by a second commit under the *same* task id, before the task
reached its current `validated` status. Each is confirmed fresh against the committed code
below, so this section is a disclosure of what validation caught mid-flight, not an open
residual.

**Task 002** ("Add `internal/service` unit evidence for the session-context map across
adapters and both launch paths"). `validationNotes` records: "The new test does not assert that
create and resume differ only in `DECK_SESSION_LAUNCH_KIND`. It deliberately omits
`DECK_SESSION_WORKSPACE` from `identityKeys` … so another `DECK_SESSION_*` key differs." The
task's own first commit was `f1788b9`; its second, `5bc6f3ed6d035b2369ef09d9f64be283c8c37a9d`
("service: export the workspace column verbatim so create and resume differ only in launch
kind (task 002)"), fixed the underlying product bug the test had exposed — `sessionContextEnv`
was reading `store.Session.Workspace` (the §11 grouping label, which defaults to the cwd's
basename on a re-read row but not on a fresh `CreateSession` return), rather than the raw
`sessions.workspace` column — and rewrote the test to compare every `DECK_SESSION_*` key except
the launch kind, in both directions, with none excluded. Confirmed fresh:

```
$ grep -n "except the launch kind itself" internal/service/session_context_test.go
208:			// DECK_SESSION_LAUNCH_KIND and in no other DECK_SESSION_* key (nor
```

and `internal/service/session_context.go`'s doc comment states the fix explicitly (reading
`session.WorkspaceColumn`, "the sessions.workspace column verbatim", not the label).

**Task 007** ("Add the three global-hook scenarios to `features/launch_hooks.feature`").
`validationNotes` records two gaps: the self-selection scenario exported its marker with an
*empty* value in the non-matching branch and asserted the key merely existed (proving nothing
about self-selection, since an empty export is present, not absent); and the failing-global-hook
scenario never asserted the fake Claude agent never started. The task's second commit,
`0316c51ac0b07c36d5d63bb132d1104706b9dbdc` ("features: prove the non-matching session carries
no hook variable and the agent never starts (task 007)"), added a genuine absence step
(`livePaneProcessEnvironmentHasNoKey`, reading `/proc/<pid>/environ` for the pane process rather
than testing for an empty value) and a banner-absence assertion for the failing-hook scenario.
Confirmed fresh:

```
$ grep -n "has no key" features/launch_hooks.feature
```
finds the new step wired into the self-selection scenario, and the failing-global-hook scenario
in the same file asserts the retained pane's tail does not contain the fake Claude banner.

**Task 023** ("Add the launch-inputs editor dialog reached from the `i` session detail
dialog"). `validationNotes` records the dialog rendered but could not persist anything in the
shipped binary: `launchInputsSetter` was never assigned by `New`, any exported constructor, or
`cmd/deck/main.go`, so `Enter` reached `submitLaunchInputs`'s nil branch and reported "editing
launch inputs is unavailable". The task's second commit,
`630ac90c104159db4723fd7cf2fa442d53fdcaf1` ("tui: wire the launch-inputs editor to a real setter
so the `i` dialog can edit (task 023)"), added `service.SetLaunchInputs`,
`tui.WithLaunchInputsSetter`, and wired `cmd/deck/main.go` to pass `sessions.SetLaunchInputs`
through it. Confirmed fresh:

```
$ grep -n "WithLaunchInputsSetter" cmd/deck/main.go internal/tui/tui.go
```
shows the constructor call in `cmd/deck/main.go` and the method's definition in
`internal/tui/tui.go`.

**Disposition.** All three tasks are `validated`; none has an open residual as of this writing.
This section exists because a reader of the run's own task-state record's `validationNotes`
field alone, without walking each task's own commit history, would see an apparently-unresolved criticism on a
`validated` task — this is the record that it was, in fact, resolved, by which commit, and how
to check that fresh.

## 3. One SPEC-versus-tree disagreement found in R104–R109's landed work — closed by task 038 at `2e5fc6b` and `566cb6d`

**The disagreement, named with its SPEC sections and its repo-relative source path.**

| | |
|---|---|
| SPEC sections | §6.4 (`pre_launch` "runs in the pane, in the same shell that then execs the agent"; "A hook must be idempotent, because it runs on every launch … fires on create, on `r`, on `R`"), §6.5 ("The two hook keys are global defaults that compose with a session's own, never replace it. … Both run, **global first**"), §6.1 (the resolution order includes `[env]` in the user's config.toml for every pane deck launches — "every adapter, `shell` included, on create and on resume alike"), §6.3 (`captured_path` sits **between** the server environment and `[env]`) |
| Source path | `internal/service/shell.go` (`Service.CreateShell`) |
| Disposition | **Fixed by task 038, commits `2e5fc6ba831300d1d9b259ded3f0cfcb842f899d` (env layering plus the global hook) and `566cb6d7216577540e7e2e7db98951db2e415d0e` (the session's own hook).** `CreateShell` now builds `launchEnv` through `resolveLaunchEnv(capturedPath, input.Env)` (the same PATH-resolution layering `CreateAgent`/`Resume` use — `captured_path`, then the user's config.toml `[env]` table, then the session's own env) and wraps its argv through `buildPaneCommand(s.GlobalPreLaunch, input.PreLaunch, false, argv)` before handing it to tmux, so both hook layers compose ahead of the shell binary on create, global-first, exactly as §6.5 requires. `ShellCreateInput` gained a `PreLaunch` field in `566cb6d`, which `CreateShell` also persists into the row's own `pre_launch` column, so the hook a shell create ran is the same line its later `r`/`R` re-runs through `Resume`'s pre-existing composition; the create modal's Pre-launch field, offered for every agent including `shell`, is now handed to `CreateShell` instead of dropped (`internal/tui/tui.go`'s `submitCreate`). Evidence: `internal/service/shell_test.go`'s `TestCreateShellPaneCarriesSessionContextWithRowsOwnValues` (criterion a), `TestCreateShellFailingGlobalPreLaunchLeavesRowInErrorWithPaneRetained` and `TestCreateShellFailingOwnPreLaunchIsFailClosed` (criterion b, mirroring `internal/service/agent_test.go`'s `TestCreateAgentFailingGlobalPreLaunchLeavesRowInErrorWithPaneRetained`), `TestCreateShellComposesGlobalThenSessionPreLaunchBeforeTheShell` (global-first order, the durable `pre_launch` column and the launch-audit pane command), `internal/tui/create_shell_pre_launch_test.go`'s `TestCreateModalShellSubmitPassesPreLaunchThrough` (the UI seam), plus `TestCreateShellPersistsLaunchesAndAudits`'s updated `env_keys` assertion (now includes `PATH`, proving `resolveLaunchEnv` is in the loop). |

`CreateShell` now builds its pane command through `buildPaneCommand` and its `launchEnv` through
`resolveLaunchEnv`, the same two functions `CreateAgent` and `Resume` call. Confirmed fresh
against the committed code:

```
$ grep -n 'buildPaneCommand(\|resolveLaunchEnv(' internal/service/shell.go
191:	launchEnv := s.resolveLaunchEnv(capturedPath, input.Env)
214:	paneCommand, err := buildPaneCommand(s.GlobalPreLaunch, input.PreLaunch, false, argv)
$ grep -n 'buildPaneCommand(\|resolveLaunchEnv(' internal/service/agent.go internal/service/resume.go
agent.go:133:	launchEnv := s.resolveLaunchEnv(envCapturedPath, input.Env)
agent.go:144:	paneCommand, err := buildPaneCommand(s.GlobalPreLaunch, input.PreLaunch, input.LoginShell, argv)
resume.go:250:	launchEnv := s.resolveLaunchEnv(envCapturedPath, session.Env)
resume.go:268:	paneCommand, err := buildPaneCommand(s.GlobalPreLaunch, session.PreLaunch, session.LoginShell, argv)
```

`CreateShell` is now among both functions' call sites, closing the three consequences the
original finding named:

- **Both hook layers now run when a `shell` session is created.**
  `Service.GlobalPreLaunch` and the session's own `pre_launch` reach a `shell` create through
  the same `buildPaneCommand` `Resume` already used, joined global-first ahead of the bare shell
  argv with `&&`, fail-closed on a non-zero exit from either one exactly like the agent paths
  (`TestCreateShellComposesGlobalThenSessionPreLaunchBeforeTheShell`,
  `TestCreateShellFailingGlobalPreLaunchLeavesRowInErrorWithPaneRetained`,
  `TestCreateShellFailingOwnPreLaunchIsFailClosed`; the two failure tests set `Service.Shell` to
  a marker-writing stand-in shell so "the shell was never reached" is a discriminating
  assertion rather than the absence of output a real idle `/bin/sh` also produces).
  `ShellCreateInput.PreLaunch` is persisted into the row's `pre_launch` column, so the same hook
  re-runs on every later `r`/`R` through `Resume`'s pre-existing composition rather than only on
  create. `login_shell` remains not a `ShellCreateInput` field (a shell session's argv already
  *is* the user's shell), so it is passed as `false` on this path; the create modal's `Login
  shell` toggle and its `Env` field are likewise still not forwarded on the shell create path
  (`internal/tui/tui.go`'s `submitCreate` passes name, cwd and `pre_launch` only) — a distinct,
  pre-existing UI-seam gap outside task 038's criteria, recorded here as advisory rather than
  fixed, and not a hook-composition gap: a shell row's `env` is settable after create through
  the §11.4 env editor and takes effect on its next launch.
- **The user's config.toml `[env]` layer and `captured_path` are now present in a freshly created shell
  pane**, via `resolveLaunchEnv`, agreeing with §6.1's stated resolution order and §6.3's PATH
  mitigation. `TestCreateShellPersistsLaunchesAndAudits`'s launch-audit `env_keys` assertion now
  includes `PATH` as evidence.

This was the launch-side half of a shape whose storage-side half is
[§1](#1-task-011-ended-failed-was-reopened-by-operator-ruling-001-011-and-was-closed-by-commit-9fb6aec-tasks-013-and-026-were-then-delivered)
(task 011's residual gap: `ShellCreateInput` carried no per-session `PostDestroy`). Both halves
are now closed: task 011 gave `ShellCreateInput` its `PostDestroy` field (commit `9fb6aec`), and
task 038 (commits `2e5fc6b` and `566cb6d`, this section) routed `CreateShell` through
`resolveLaunchEnv` and `buildPaneCommand` with both hook layers.

**The rest of R104–R109's landed work agrees with `SPEC.md`.** The PRD's own instruction ("Where
this PRD and `SPEC.md` disagree, `SPEC.md` wins and the disagreement is a finding … never an
edit") was checked against every SPEC section R104–R109 name and the code that claims to satisfy
it — five checks, none of which produced a further disagreement:

- **§6.1's nine-variable table** against `internal/service/session_context.go`'s
  `sessionContextEnv`: all nine keys present, `DECK_SESSION_PROFILE` reads
  `store.Session.PermissionProfile` — already the *resolved* profile after §5's degradation is
  applied (`internal/store/store.go`'s `PermissionProfile`/`PermissionProfileReason` pair) —
  agreeing with §6.1's "the resolved permission profile in force … never the requested one".
  `DECK_SESSION_WORKSPACE` reads the raw column per task 002's fix above, agreeing with §6.1's
  "empty rather than absent when the column behind it is unset".
- **§6.5's global-first composition rule** ("Both run, global first … Global-first is also what
  lets a session's hook observe whatever the global one exported") against
  `internal/service/agent.go`'s `buildPaneCommand` (task 005, `38ad227`): the global hook is
  composed ahead of the session's own, joined with `&&`, matching §6.5 and §9.1's fail-closed
  short-circuit.
- **§11.4's dialog inventory and field list** ("launch-inputs editor (§6.2 —
  `pre_launch`, `post_destroy`, `launch_args`, `login_shell`; every field labelled
  *restart-to-apply*. The two hook lines are shown verbatim rather than masked") against
  `internal/tui/launch_inputs.go`'s field rows (tasks 023/024): four fields, hook lines shown
  verbatim, each labelled restart-to-apply, `esc`/`↵`/`↑`/`↓` behaving per §11.4's dialog
  contract and no `tab` field-navigation binding (the dialog has no path field, so §11.4's `tab`
  reservation is vacuously satisfied).
- **§6.4's export-reaches-the-agent guarantee and its three boundaries** against
  `internal/service/agent.go`'s `buildPaneCommand` doc comment and
  `internal/service/agent_test.go` (tasks 008/009): the positive case and both negative
  boundaries (not in the tmux session environment table, not in any `state.db` column) are
  each their own test against a real tmux server.
- **R109's five claims** (idempotent/every-launch, fail-closed, fail-open+not-on-`x`,
  undo+rebuild, safe secret shape) against `internal/tui/tui.go`'s `helpText()` "Hooks" section
  and `internal/tui/hook_help_coverage_test.go` (task 028): each claim's required phrases are
  present somewhere across the five named surfaces, matching §6.4 and §9.2's own wording for
  each claim (idempotency, fail-closed, fail-open-and-not-on-`x`, undo-then-rebuild-on-next-`r`,
  the `export K=V`-on-stdout/`sensitive` shape).

None of the five bullets above produced a further disagreement; each product statement matches
the SPEC section it claims to satisfy. Tasks 013–019 and 026, which consume §9.2's
teardown-order sentence ("run the session's own `post_destroy` and then the global one from
config.toml (§6.5)" — the *reverse* of §6.5's launch order), have since landed (commits
`259284b`…`6080c55`), and `internal/service/post_destroy_test.go`'s
`TestArchiveRunsSessionThenGlobalPostDestroyExactlyOnce` and
`TestDeleteRunsSessionThenGlobalPostDestroyExactlyOnce` are the tests that exercise that exact
session-then-global order on `A` and `dd` respectively — no disagreement surfaced against §9.2
in that landed work.

## 4. Protected-path audit over this phase's commit range is clean; carried-forward out-of-scope findings checked by name against task 030's whole-suite sweep, none recurred

**Protected-path audit**, run fresh at this report's own tree, exactly as the PRD states it
(base computed, not pasted):

```
$ BASE=$(git log --format=%H --diff-filter=A -1 -- prds/phase3j-launch-and-teardown-hooks.md)
$ echo "$BASE"
06ea5b72d98ce0dda251d16af452b7f6d87e2c2f
$ git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
(no output)
```

Empty, as required. No task in 001–028 touched a protected path.

**Carried-forward out-of-scope findings** (per the PRD's "For the planner" section and the
standing rules): F2 (golden-frame settle), F20 (`status_recovery` dup-pane), F22
(`ByteArrivalPattern`), F37 (`sort_order` latent race), F7 (quantisation collisions), the
`features/filter.feature` dd/undo race, and the OSC 52 clipboard-reliability question. Tasks
001–028 ran only targeted package tests (`internal/service`, `internal/store`, `internal/tui`,
single-feature `features` runs), never the whole-suite sweep that would exercise the
`features`/`internal/interactive` packages these seven findings live in. Task 030 (refreshed at
the final code sha by task 081) supplied that sweep, at
`docs/reports/phase3j-030-fullsuite/sweep.log`. This section now records the by-name recurrence
check against that log, run fresh against the tree at this report's own commit:

```
$ grep -n -i "F2\b\|F20\b\|F22\b\|F37\b\|F7\b\|quantis\|filter.feature\|dd/undo\|OSC 52\|clipboard" \
    docs/reports/phase3j-030-fullsuite/sweep.log
(no output)
```

`sweep.log` is the non-verbose `go test -p=1 -count=1 ./...` package-summary log (17 lines: 14
`ok`, 3 `[no test files]`, no per-scenario or per-step names) — none of the seven carried-forward
items' names, or the substrings that would identify a recurrence of them, appear anywhere in it.
The result for each, checked by name against that log:

- **F2** (golden-frame settle) — no match; the packages that would surface it
  (`internal/interactive`, `features`) both show `ok`, no failure to attribute to it.
- **F20** (`status_recovery` dup-pane) — no match; same two packages `ok`.
- **F22** (`ByteArrivalPattern`) — no match; `internal/tmux` (`ok`) and `features` (`ok`) are the
  packages that would surface it.
- **F37** (`sort_order` latent race) — no match; `internal/store` (`ok`) and `internal/service`
  (`ok`) are the packages that would surface it.
- **F7** (quantisation collisions) — no match; `internal/theme` (`ok`) is the package that would
  surface it.
- **The `features/filter.feature` dd/undo race** — no match; `features` (`ok`, 339.455s, all
  scenarios/steps passing per §8's verbose tally).
- **OSC 52 clipboard reliability** — no match; `internal/tmux` (`ok`) and `features` (`ok`) are
  the packages that would surface it.

None of the seven recurred as a named or attributable failure in this sweep. This is a
non-recurrence check against one whole-suite run, not a claim that any of the seven is fixed,
resolved, or no longer a live concern — they remain advisory-only carried-forward items, as the
PRD's "For the planner" section and the standing rules require; §8 records the same check
re-run against the refreshed gate logs at the final code sha. One consequence of never having
run a whole-`features` package test during 001–028 is
[§6](#6-two-stale-schema-version-literals-in-features--fixed-by-task-030-at-204af7d): four
`features/store.feature` scenarios failed at that earlier tree (fixed since by task 030's commit
`204af7d`, per §6 below), and nothing in 001–028 would have noticed.

## 5. A pre-existing stale schema-version pin in `features/assertions_test.go`, found and fixed in flight by task 027

Task 010 (`f60b5e4`, "store: add schemaV6 (`post_destroy`, `launch_dirty`), bump `SchemaVersion`
to 6") moved `internal/store/store.go`'s `const SchemaVersion` from 5 to 6. Several *black-box*
test literals under `features/` copy that number by hand rather than importing it (deliberately —
that harness observes the released binary without importing `internal/...`), and task 010 did not
sweep them.

Task 027 (`895f58d`) hit one of them: `TestBlackBoxAssertionsObserveRealSession` in
`features/assertions_test.go` called `databaseSchemaVersion(stepCtx, 5)`, which made task 027's
own required command (`DECK_GODOG_PATHS=launch_hooks.feature go test ./features`) exit nonzero for
a reason unrelated to the scenario it was adding. Task 027's commit message records the fix
verbatim:

> Also fixes a stale schema-version pin in TestBlackBoxAssertionsObserveRealSession
> (features/assertions_test.go), hardcoded at 5 from before task 010 bumped
> internal/store.SchemaVersion to 6 -- an unrelated pre-existing gap that
> otherwise makes this task's own required command
> (DECK_GODOG_PATHS=launch_hooks.feature go test ./features) exit nonzero
> regardless of the new scenario's own correctness.

Confirmed fresh against the committed code:

```
$ grep -n "SchemaVersion = " internal/store/store.go
22:const SchemaVersion = 6
$ grep -n "databaseSchemaVersion(stepCtx" features/assertions_test.go
1095:	if err := databaseSchemaVersion(stepCtx, 6); err != nil {
```

**Disposition.** Fixed inside task 027, with a comment at the pin saying it must track
`internal/store.SchemaVersion` exactly. It is recorded here because it is a *pre-existing* gap
this phase inherited and repaired in passing rather than part of any task's own criteria, and
because it is the same drift class as §6's two literals, fixed below. Rule for any future
phase that bumps `SchemaVersion`: grep `features/` for the old number before calling the bump
done.

## 6. Two stale schema-version literals in `features/` — fixed by task 030 at `204af7d`

The drift of §5 survived in two more places that task 027's targeted single-feature run could not
reach. Both were found by task 029 while writing the earlier version of this report, by running
the one feature file nothing in tasks 001–028 had run (dev evidence, not a deliverable sweep), and
both were fixed by task 030's own commit `204af7dad3b4766ee82f80edf45f9f7f9c7d920b`
("features: fix stale schema version literals in store.feature (task 030)").

**(a) `features/store.feature` now pins schema version 6 in its three scenarios**, matching the
binary's current `internal/store.SchemaVersion`:

```
$ grep -n "schema version" features/store.feature
9:    And the state database has schema version 6
15:    Then the state database has schema version 6
24:    Then the state database has schema version 6
```

**(b) `features/store_feature_test.go`'s "newer unsupported" fixture now writes 7**, one past the
current `SchemaVersion` of 6, restoring the invariant its own comment states ("One past
`internal/store.SchemaVersion` (bumped to 6) -- must always stay strictly newer than the binary
understands, so a later `SchemaVersion` bump has to bump this literal too"):

```
$ grep -n "writeDatabaseFixture(h, 7)" features/store_feature_test.go
58:	if err := writeDatabaseFixture(h, 7); err != nil {
```

Confirmed fresh against the committed code, re-running the same targeted evidence command §5 and
the original version of this finding used:

```
$ ci/run.sh sh -c 'DECK_GODOG_PATHS=store.feature go test -count=1 ./features'
ok  	github.com/n-orlov/deck/features	17.745s
```

**Disposition: fixed by task 030's commit `204af7d`, no gate blocker remains.** Both literals now
track `internal/store.SchemaVersion` (`features/store.feature`'s three pins moved to 6;
`features/store_feature_test.go`'s fixture literal moved to 7, one past current), and the
`features` package's `features/store.feature` scenarios pass at the current tree. This was never a
`SPEC.md` disagreement (§4 of `SPEC.md` fixes the schema and its migration invariant, not these
black-box literals) and never a product bug: both were test literals that copy
`internal/store.SchemaVersion` by hand, the drift point §5 warns about.

## 7. How to re-check every citation in this report

Every backticked sha above resolves under `git cat-file -e`; every backticked repo-relative path
names a file tracked under `git ls-files --error-unmatch`:

```
$ for sha in 0316c51 17cabb8 1a4b9db 2042cb8 204af7d 259284b 2dca034 2e5fc6b 31e6aff \
    38ad227 52e529b 566cb6d 5bc6f3e 6080c55 630ac90 6a22181 6bb64b2 807fe0a 895f58d \
    9fb6aec a44ee32 a936b30 b29afb8 d71c02f f1788b9 f60b5e4 fbbda8f \
    06ea5b7 be6e42b c13a909 ed8b81e 34f1560 db8d5c7 b50cc20 5ef971b 0299be9 66711eb \
    8931988 db2ea55 ad51022 325d00a 9fb25f7 6b8f1d0; do \
    git cat-file -e "$sha^{commit}" && echo "$sha ok"; done
(all print "<sha> ok")
$ git ls-files --error-unmatch \
    SPEC.md \
    prds/phase3j-launch-and-teardown-hooks.md \
    cmd/deck/main.go \
    docs/reports/phase3j.md \
    docs/reports/phase3j-030-fullsuite \
    docs/reports/phase3j-030-fullsuite/README.md \
    docs/reports/phase3j-030-fullsuite/sweep.log \
    docs/reports/phase3j-031-fullsuite-verbose \
    docs/reports/phase3j-031-fullsuite-verbose/README.md \
    docs/reports/phase3j-031-fullsuite-verbose/verbose.log \
    docs/reports/phase3j-031-fullsuite-verbose/verbose.log.exitstatus \
    docs/reports/phase3j-032-stability10 \
    docs/reports/phase3j-032-stability10/README.md \
    docs/reports/phase3j-032-stability10/summary.log \
    features \
    features/assertions_test.go \
    features/filter.feature \
    features/launch_hooks.feature \
    features/store.feature \
    features/store_feature_test.go \
    features/teardown_hooks.feature \
    internal/interactive internal/notify internal/search internal/tmux internal/unit \
    internal/service internal/store internal/tui \
    internal/service/agent.go \
    internal/service/agent_test.go \
    internal/service/post_destroy.go \
    internal/service/session_context.go \
    internal/service/shell.go \
    internal/service/shell_test.go \
    internal/store/store.go \
    internal/tui/create_shell_pre_launch_test.go \
    internal/tui/hook_help_coverage_test.go \
    internal/tui/launch_inputs.go \
    internal/tui/tui.go
(all resolve, this report's own path docs/reports/phase3j-findings.md included — it became
tracked in commit a30accd, task 029's first commit.)
```

**No repo path in this report names an untracked file or a not-yet-generated directory.** The
three gate report directories of tasks 058–060 are now cited by path because they exist and are
tracked; the closeout report of task 034 is still referred to by the task that publishes it,
never by a path, precisely because `git ls-files --error-unmatch` cannot succeed for a path that
does not exist yet. Four kinds of backticked token above are deliberately *not* repo-relative
paths and are never claimed to be tracked: a Go package glob or import path fragment
(`internal/...`), a qualified Go symbol (`internal/store.SchemaVersion`), an absolute system path
(`/bin/sh`, `/proc/<pid>/environ`), and a path inside a quoted SPEC sentence
(`$XDG_CONFIG_HOME/deck/config.toml`, `$DECK_HOME/captures/<session_id>/`). The user's own
config.toml is likewise a runtime configuration file, not a repo file, and is named in plain prose
rather than backticked. This report also refers to the run's own loop state — its task-state
record, its review findings and the operator's rulings, cited by ruling id (for example ruling
001-011) — always in prose and never as a backticked filename, because that state is not part of
the repository and no path of it is quoted anywhere in this document.

`SPEC.md` and `prds/phase3j-launch-and-teardown-hooks.md` are quoted throughout this report,
never edited by it — nothing in this findings report writes to a protected path.

## 8. Gate dispositions: whole-suite sweep (task 081), verbose tally companion (task 082) and ten-run stability (task 083)

**The dispositions below supersede, rather than delete, the earlier dispositions this section
previously recorded for tasks 058, 059 and 060 (approach 03) at code sha
`b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7`**, which themselves superseded task 030/032's original
`a44ee320b93186496d56364836b0aed00a6f1e0b` dispositions. Task 080 of this approach (05) corrected a
stale comment in `features/launch_hooks.feature`; per the PRD's "Termination" rule that comment-only
change still moves the final code sha (`git log -1 --format=%H -- '*.go' '*.feature'`) forward from
`b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7` to task 080's own commit,
`4fbd452430501805a860dd229ddca1cd3f5c1cd6` — the final code sha for this document. Tasks 081, 082
and 083 re-ran all three gates at that new final code sha and refreshed their report directories
**in place** (no new numbered directories). Both `a44ee32` and `b29afb8` are historical record of
what ran at those earlier shas, never a claim about the current tree; the current disposition is
the one below.

**Task 081 — whole-suite sweep.** Exit status `0`. Code sha it ran at:
`4fbd452430501805a860dd229ddca1cd3f5c1cd6` (the final code sha, per
`git log -1 --format=%H -- '*.go' '*.feature'`; HEAD and origin/main at run time were the same sha).
Report path `docs/reports/phase3j-030-fullsuite/` (refreshed in place: `docs/reports/phase3j-030-fullsuite/README.md`
+ `docs/reports/phase3j-030-fullsuite/sweep.log`). Every package
result line is `ok` (14 packages, including `features` at 339.455s) or `?` with `[no test files]`
(`internal/notify`, `internal/search`, `internal/unit`) — no skipped marker anywhere in the log
(`grep -c '^ok' docs/reports/phase3j-030-fullsuite/sweep.log` = 14, `grep -c 'no test files'
docs/reports/phase3j-030-fullsuite/sweep.log` = 3, matching the refreshed README's own stated counts).
This supersedes the superseded `b29afb8`-sha disposition (task 058), which reported the same 14/3
split — the re-run at the advanced sha changed no package result.

**Task 082 — verbose tally companion.** Exit status `0`. Code sha it ran at: the same
`4fbd452430501805a860dd229ddca1cd3f5c1cd6` (HEAD/origin main at run time was
`680237e45b73b3666c81571e56065a41c9e81c0b`, task 081's docs-only refresh commit, a docs-only
descendant of that sha). Report path `docs/reports/phase3j-031-fullsuite-verbose/`
(refreshed in place: `docs/reports/phase3j-031-fullsuite-verbose/README.md` +
`docs/reports/phase3j-031-fullsuite-verbose/verbose.log` +
`docs/reports/phase3j-031-fullsuite-verbose/verbose.log.exitstatus`). This companion exists only
because the non-verbose launcher cannot print the godog scenario/step tally (finding F34); task 081's
sweep remains the deliverable gate. Tally as measured, quoted byte-exact (ESC bytes included) in the
refreshed README: **330 scenarios (330 passed)**, **3824 steps (3824 passed)** — unchanged from the
superseded `b29afb8` tally (task 080 touched only a comment block, adding and removing no scenario or
step). Package result lines match task 081's non-verbose sweep (14 `ok`, 3 `[no test files]`).

**Task 083 — ten-run stability gate.** Exit status `0` (`ci/stability.sh 10`'s own captured status).
Code sha it ran at: the same `4fbd452430501805a860dd229ddca1cd3f5c1cd6`. Report path
`docs/reports/phase3j-032-stability10/` (refreshed in place: `docs/reports/phase3j-032-stability10/README.md`
+ `docs/reports/phase3j-032-stability10/summary.log`, the script's
own combined summary published verbatim). Published result: `10/10 passed` — every one of the 10
runs `PASS`, no failing run to name; HEAD/origin main at this run's launch time was
`2de170049ba6fb820dacbfbb68c6b2dc4b375c15`, a docs-only descendant of `4fbd452430501805a860dd229ddca1cd3f5c1cd6`.
This supersedes the superseded `b29afb8`-sha disposition (task 060, also `10/10 passed`).

**Carried forward: task 083's first collection reported 9/10, not 10/10, at this same code sha.**
The FIRST collection under this directory (commit `2de1700`) reported `9/10 passed` — RUN 9 failed
on an intermittent tmux/pty timing flake in
`internal/tmux.TestSendKeysUnknownKeyNameIsDeliveredAsLiteralTextWithExitZero` (pane capture
`"F$ robnicate"` instead of the literal `"Frobnicate"`). That collection was rejected on polling
PROCEDURE, not on the number, and the gate was re-launched once at the same code sha with the
mandated `sleep 120`-only poll discipline followed exactly, giving the published `10/10` (commit
`7617eef`). The pair — 9/10 then 10/10 — is evidence that this test can flake intermittently under
load on this host; it is carried forward here as an advisory observation, never as a claim that the
suite is flake-free.

**Recurrence check against §4's carried-forward findings, re-run against the new logs.** Grepping the
refreshed logs (`docs/reports/phase3j-030-fullsuite/sweep.log` and
`docs/reports/phase3j-032-stability10/summary.log`) for the carried-forward items — F2, F20, F22, F37,
the F7 quantisation collisions, the `features/filter.feature` dd/undo race, OSC 52 clipboard reliability — finds
no match in either log:

```
$ grep -n -i "F2\b\|F20\b\|F22\b\|F37\b\|F7\b\|quantis\|filter.feature\|dd/undo\|OSC 52\|clipboard" \
    docs/reports/phase3j-030-fullsuite/sweep.log docs/reports/phase3j-032-stability10/summary.log
(no output)
```

No carried-forward finding recurred in either refreshed gate's run. This section is therefore the
disposition rather than a placeholder: none of the three gates failed, none is pending, and there is no
recurrence to mark advisory. (The carried-forward items themselves remain advisory-only, as recorded in
§4 and in the handoff notes' residuals list — this paragraph reports their non-recurrence in these
refreshed gate runs, not a change to their own disposition.)

## 9. Independent review's blocking findings 1-3 (approach 01) are closed

The independent review that rejected approach 01 (the run's own review-findings record under
approach 01, named in prose rather than backticked, not a tracked repo path and therefore not
cited by filename below beyond this one mention) raised
five blocking findings. Findings 4 and 5 are the R110 record-accuracy and task-030
polling/one-sweep-discipline gaps tracked elsewhere in this document (see §§4-8 and
`docs/reports/phase3j.md`'s own corrections). Findings 1, 2 and 3 are closed in this tree, by the
following commits:

- **Finding 1 — `post_destroy` failures could not raise a toast in the shipped application.**
  Closed by `d71c02f` (route a teardown-hook failure into a visible note, `internal/tui`),
  `a936b30` (wire the teardown-hook message through `cmd/deck/main.go`), and `52e529b` (a
  `features/` scenario asserting a failing teardown hook's toast text on screen). A reader can
  check the wiring directly: `cmd/deck/main.go` calls
  `model = model.WithTeardownHookReporters(sessions.Archive, sessions.Delete)`, replacing the
  discard-the-message adaptation the review observed.
- **Finding 2 — the teardown timeout was not the required named constant.** Closed by `1a4b9db`
  (make `postDestroyTimeout` an immutable constant). A reader can check
  `internal/service/post_destroy.go`, which declares `const postDestroyTimeout = 30 * time.Second`
  (no longer the mutable `var` the review found).
- **Finding 3 — R109's user guidance was inaccurate and its coverage test missed the error.**
  Closed by `2042cb8` (fix the R109 hook copy: caller-side eval, no `pre_launch` timeout claim),
  `31e6aff` (drop the launch-inputs dialog's `pre_launch` timeout claim), and `b29afb8` (tighten
  `internal/tui/hook_help_coverage_test.go` to catch the inaccurate copy). A reader can check
  `internal/tui/hook_help_coverage_test.go`, whose assertions now require the caller-eval shape
  (`t.Errorf` if `"caller to eval"` is absent), reject the old inaccurate phrasing (`t.Errorf` if
  `"deck to eval"` is present), and reject any sentence combining `"fail-closed"` with
  `"timeout"` (`pre_launch` is fail-closed on a non-zero exit only and has no timeout semantics;
  only `post_destroy`'s fail-open sentence may mention a timeout).

**Verification run for this section**
(`ci/run.sh go test -count=1 ./internal/service ./internal/store ./internal/tui ./cmd/deck`):
first attempt failed with the known load-sensitive pipe-pane fifo flake — not
`TestStolenFromTeardownIssuesNoPipePaneDisarm` itself this time, but the same fifo-timeout
failure class in the same package (`internal/tui`), on two unrelated stolen-pane tests:

```
--- FAIL: TestTwoSequentialStealsRestorePreEntryGeometry (5.08s)
    double_steal_restore_test.go:71: first entry did not enter interactive mode: attachError="Cannot enter interactive mode: arm pipe-pane before seed capture: wait for pipe-pane's job to connect to /tmp/deck-interactive-pipe-1890924179/pane.fifo: timed out after 5s waiting for pipe-pane's job to open the fifo"
--- FAIL: TestPreviewTickRaisesLostAttachOnStolenClaim (5.07s)
    interactive_displacement_test.go:85: first entry did not enter interactive mode: attachError="Cannot enter interactive mode: arm pipe-pane before seed capture: wait for pipe-pane's job to connect to /tmp/deck-interactive-pipe-2155222583/pane.fifo: timed out after 5s waiting for pipe-pane's job to open the fifo"
```

The permitted rerun of the identical unnarrowed command exited 0 across all four packages:

```
ok  	github.com/n-orlov/deck/internal/service	7.072s
ok  	github.com/n-orlov/deck/internal/store	3.728s
ok  	github.com/n-orlov/deck/internal/tui	4.175s
ok  	github.com/n-orlov/deck/cmd/deck	7.612s
```

This section's own commit is docs-only: `git log -1 --format=%H -- '*.go' '*.feature'` printed
`b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7` at that commit and was unchanged by it. That is a
statement about the tree as it stood then, not about the phase's final code sha now: task 080 later
edited a comment in `features/launch_hooks.feature`, so the final code sha is now
`4fbd452430501805a860dd229ddca1cd3f5c1cd6` (see §11 and `docs/reports/phase3j.md`).

## 10. Independent review's blocking findings 4 and 5 (record accuracy and gate-polling/one-sweep discipline) are closed

The same independent review named in §9 raised two further blocking findings against the R110
record itself, distinct from findings 1-3's product-code gaps: finding 4, that this phase's own
reports carried inaccurate claims (record defects), and finding 5, that the gate tasks' own
polling and one-sweep-per-iteration discipline was not evidenced as followed. Both are closed as
of this writing.

**Finding 4 — record defects, closed by tasks 062 (`2dca034`) and 063 (`807fe0a`).** Three
inaccurate claims in `docs/reports/phase3j.md` and this file's own §8 were corrected by those two
commits:

- **Uncommitted-tasks wording.** `docs/reports/phase3j.md`'s R110 table row previously read
  "the findings/DELIVERY-LOG/Telegram tasks that round out the phase's remaining paperwork are
  tasks 035–037, not yet committed as of this writing" — inaccurate once those tasks landed.
  Task 062 (`2dca034`) corrected it to name each task's actual disposition: "035 and 036 landed
  as commits, 037 was a Telegram closing notification, which by its nature is sent, not
  committed to this repo", with the commit shas for 035 and 036 added to the same row.
- **Package counts.** Both `docs/reports/phase3j.md` and this file's own §8 previously stated
  the whole-suite sweep's package count at the superseded `a44ee32` sha's figure as the current
  one. Tasks 062 (`2dca034`) and 063 (`807fe0a`) corrected every occurrence naming the *current*
  count to the figure `docs/reports/phase3j-030-fullsuite/sweep.log` actually shows at the final
  code sha (`internal/tmux` split out as its own package between the two shas) — confirmed
  fresh: `grep -c '^ok' docs/reports/phase3j-030-fullsuite/sweep.log` = 14, matching both files'
  stated current count. §8 above still names the superseded `a44ee32`-sha figure once, explicitly
  framed as history ("supersedes the `a44ee32`-sha disposition of 13 `ok` packages") — that
  mention is the corrected record's own citation of what it superseded, not a surviving defect.
- **Purged untracked citation, at `6a22181`.** Task 047 (`6a22181`, "docs: purge untracked
  run-state citation from phase3j reports") removed a citation of an untracked run-state path
  from `docs/reports/phase3j-030-fullsuite/README.md`, `docs/reports/phase3j-031-fullsuite-verbose/README.md`
  and `docs/reports/phase3j.md`, ahead of tasks 062/063's own corrections, so that none of this
  phase's reports names a path `git ls-files --error-unmatch` cannot resolve (the same rule §7
  states for this document).

All three corrections are confirmed fresh against the committed code: `git cat-file -e
2dca034^{commit}`, `git cat-file -e 807fe0a^{commit}` and `git cat-file -e 6a22181^{commit}`
each resolve, and `grep -c 'not yet committed as of this writing' docs/reports/phase3j.md` = 0.

**Finding 5 — gate polling and one-sweep-per-iteration discipline, closed by the refreshed
`phase3j-030-fullsuite` README's own polling record.** The refreshed
`docs/reports/phase3j-030-fullsuite/README.md` (task 058) states its own polling discipline in
its `## Command` section verbatim: "Backgrounded and polled with `sleep 60` only, never blocked
on; total wall time was about 6 minutes (well under the 30-minute `timeout` and a small fraction
of one iteration's cap)." That is the same discipline the standing rules require (a `sleep 60`
poll interval and nothing else, one whole-suite sweep per iteration) and it is the gate's own
report recording that it was followed, not a claim made about it from outside. No other
whole-suite sweep command appears anywhere in this run's tracked reports for the same code sha,
so the one-sweep-per-iteration half of the discipline is likewise satisfied by omission — there
is nothing else to conflict with it.

**Disposition.** Findings 4 and 5 are closed as of this writing. This section does not claim
findings 1-3 (§9) or the residual items below are affected by it — those are separate
dispositions covered elsewhere in this document.

**Residuals this closure does not claim fixed.** This section closes findings 4 and 5 only. It
does not claim to have fixed, and does not affect the disposition of:

- the create-modal `Env`/`Login-shell` seam (§3's advisory: `internal/tui/tui.go`'s
  `submitCreate` still does not forward a shell create's `Env` field or `Login shell` toggle to
  `CreateShell`);
- R109's copy-coverage evidence, which remains bounded to the assertions
  `internal/tui/hook_help_coverage_test.go` itself makes (§9's finding-3 closure);
- the carried-forward advisory items from §4: F2 (golden-frame settle), F20 (`status_recovery`
  dup-pane), F22 (`ByteArrivalPattern`), F37 (`sort_order` latent race), F7 (quantisation
  collisions), the `features/filter.feature` dd/undo race, and the OSC 52 clipboard-reliability
  question.

All of the above remain exactly as disclosed in §§3, 4 and 9 — advisory, not claimed fixed, and
unchanged by this section.

## 11. Approach 04's independent review finding 3 (R110 record still did not match the tree) is closed

Approach 04's independent review (its run-state review findings, cited here by approach number
rather than by a backticked run-state filename, per §7's rule) rejected that approach on, among
other grounds, its own finding 3: "R110's closeout record still does not match the tree or
current run state." That finding's evidence named five concrete defects. Each is closed in this
tree, by the commit named below; every sha resolves under `git cat-file -e <sha>^{commit}`.

1. **The findings-file opening's failing-literals/placeholder claim.** The finding quoted this
   file's own opening paragraph still saying the two schema-version literals were failing and
   both gate dispositions were an unfilled placeholder, although §6 already recorded the
   literals fixed and §8 already published the gates. Closed by commit
   `5bc097d6b5b796a15b74d03c0a4bb547c4694aa5` (task 084), which rewrote the opening paragraph to
   state the literals were fixed by task 030's commit and all three gate dispositions are
   published, matching §6 and §8.
2. **The §4 not-yet-run claim.** The finding quoted this file's §4 heading and body (as they
   stood at review time) saying the whole-suite recurrence check against the carried-forward
   findings had not yet run and remained a placeholder for a later task to fill in. Closed by
   commit `2028ff8e9aba81f2a1f7ffe6a4afeac0b76714b3` (task 085), which rewrote §4's heading and
   body to run the by-name recurrence grep against `docs/reports/phase3j-030-fullsuite/sweep.log`
   fresh and record its (empty, i.e. non-recurring) result in place, rather than deferring it.
3. **The `features/launch_hooks.feature` CreateShell comment.** The finding quoted that file's
   comment block (then at lines 49–50) asserting CreateShell "never composes pre_launch at
   all", although `internal/service/shell.go` already routed `CreateShell` through
   `buildPaneCommand` and the fresh suite already passed
   `TestCreateShellComposesGlobalThenSessionPreLaunchBeforeTheShell` and both shell fail-closed
   tests. Closed by commit `4fbd452430501805a860dd229ddca1cd3f5c1cd6` (task 080), which rewrote
   the comment to state CreateShell composes global-then-session `pre_launch` through
   `buildPaneCommand` exactly like every other launch path, and explains why the surrounding
   scenarios still exercise `claude` rather than `shell` (the suite's step vocabulary has no
   "creates shell session … with pre-launch command …" step, not because shell's own launch
   skips the composition).
4. **The DELIVERY-LOG skipped/failed claim.** The finding quoted `docs/DELIVERY-LOG.md` (then
   at lines 835–836) claiming "No task in this plan rests `skipped`, and none rests `failed` at
   close", although the run's own task-state record already held tasks ended `skipped`. Closed
   by commit `b8672bcf9f8833312a978db29a0347e7d23ae598` (task 087), which removed that sentence
   from the Phase 3j paragraph without substituting another status-counting claim, so the
   paragraph no longer asserts anything about which tasks rest `skipped` or `failed`.
5. **The R110 row stopping short of the approach-04 record work.** The finding observed that
   `docs/reports/phase3j.md`'s R110 row stopped at approach-03's task 065 and omitted the
   record-repair work of tasks 067–074 (approach 04) and, by the time of this closure, tasks
   080–089 (approach 05). Closed by commit `536d898940328e3e9868416c6a7c0d6a63a317cd` (task
   090), which extended the R110 row's task-ids and shas columns with 067, 070, 071, 072, 073,
   074 and 080–089, each with a parenthetical of what it discharged.

**Approach 03's review findings 1 and 2 are terminally refuted petition re-files and were not
re-filed by this closure.** Those two findings — the Task 204 whole-suite-sweep obligation and
the create/resume-context obligation — were each adjudicated REFUTED before approach 03 ran, are
named as such in this document's own standing rules, and are prohibited from being re-filed by
any task in this plan (approach 04's own findings 1 and 2 repeated the same two obligations, and
were likewise prohibited re-files, not new merits hearings). Neither is addressed by this section,
and this section makes no claim about either one.

**Verification.** Every sha backticked in this section resolves fresh:

```
$ for sha in 5bc097d6b5b796a15b74d03c0a4bb547c4694aa5 2028ff8e9aba81f2a1f7ffe6a4afeac0b76714b3 \
    4fbd452430501805a860dd229ddca1cd3f5c1cd6 b8672bcf9f8833312a978db29a0347e7d23ae598 \
    536d898940328e3e9868416c6a7c0d6a63a317cd; do \
    git cat-file -e "$sha^{commit}" && echo "$sha ok"; done
(all print "<sha> ok")
```

**Disposition.** All five defects named by approach 04's review finding 3 are closed as of this
writing. This section does not claim any other finding, from any review, is affected by it —
approach 01's findings 1–5 (§§9–10) and approach 03's findings 1 and 2 (prohibited re-files,
above) remain exactly as disclosed elsewhere in this document.
