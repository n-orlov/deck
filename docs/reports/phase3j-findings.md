# Phase 3j findings

Companion to `docs/reports/phase3j.md` (task 034, not yet written): what the requirement table
does not carry — a task that ended `failed` and the dependency doom it leaves behind, gaps a
validation pass found that were then closed within the same task rather than left open, the
protected-path audit's disposition over this run's own commit range, this phase's check for a
disagreement between the tree and `SPEC.md`, and a placeholder for both gates' dispositions
(tasks 030/032, filled in by tasks 033 and 034 per the plan).

Written by task 029 against the tree at commit `17cabb871d0c8c17dc742256632015ed1889db9b` (the
last commit touching `*.go`/`*.feature` as of this writing; `docs/reports/phase3j.md` §"Final
code sha", task 034, is the authority once it lands — this section does not repeat that
ancestry argument). Every sha cited resolves under `git cat-file -e` and every path cited is
tracked under `git ls-files --error-unmatch`, both checked in
[§5](#5-how-to-re-check-every-citation-in-this-report).

- [1. Task 011 ended `failed` (validation-exhausted); its residual gap dooms tasks 013 and 026 by dependency, unresolved as of this writing](#1-task-011-ended-failed-validation-exhausted-its-residual-gap-dooms-tasks-013-and-026-by-dependency-unresolved-as-of-this-writing)
- [2. Three tasks (002, 007, 023) had a validation-found gap that was closed within the same task's own follow-up commit, not left open](#2-three-tasks-002-007-023-had-a-validation-found-gap-that-was-closed-within-the-same-tasks-own-follow-up-commit-not-left-open)
- [3. No SPEC.md-versus-tree disagreement found in R104–R109's landed work (tasks 001–028)](#3-no-specmd-versus-tree-disagreement-found-in-r104r109s-landed-work-tasks-001028)
- [4. Protected-path audit over this phase's commit range is clean; carried-forward out-of-scope findings not yet checked against a whole-suite run](#4-protected-path-audit-over-this-phases-commit-range-is-clean-carried-forward-out-of-scope-findings-not-yet-checked-against-a-whole-suite-run)
- [5. How to re-check every citation in this report](#5-how-to-re-check-every-citation-in-this-report)
- [6. Placeholder: gate dispositions (whole-suite sweep and ten-run stability), filled in by tasks 033 and 034](#6-placeholder-gate-dispositions-whole-suite-sweep-and-ten-run-stability-filled-in-by-tasks-033-and-034)

## 1. Task 011 ended `failed` (validation-exhausted); its residual gap dooms tasks 013 and 026 by dependency, unresolved as of this writing

Task 011 ("Plumb `post_destroy` through the store's session write and read paths") is `failed`
in `tasks.json`, `failureKind: "validation-exhausted"`, after 3 validation attempts.

**What failed, quoted verbatim from task 011's own `validationNotes`:**

> Residual gap: CreateShell cannot accept or persist post_destroy because
> internal/service/shell.go's ShellCreateInput has no PostDestroy field and its
> store.CreateSessionInput omits PostDestroy; only CreateAgent propagates it.

The same note confirms what task 011 *did* land and verify: `store.Session` and
`store.CreateSessionInput` both carry `PostDestroy`; `CreateSession` writes the
`post_destroy` column; `sessionColumns`/`scanSession` read it back;
`TestCreateSessionRoundTripsAllPhase1FieldsAcrossReopen` proves a non-empty value survives
`GetSession` and `ListSessions`; `CreateAgent` passes `input.PostDestroy` through; and
`ci/run.sh go test -count=1 ./internal/store ./internal/service` exited 0 against that state.
Confirmed fresh against the committed code:

```
$ grep -n "PostDestroy" internal/service/shell.go
52:	// GlobalPostDestroy mirrors config.toml's top-level post_destroy key
59:	GlobalPostDestroy string
```

The only `PostDestroy`-named field is `GlobalPostDestroy` (the config-level default, not a
per-session value); `ShellCreateInput` itself has no per-session `PostDestroy` field, and
`CreateShell`'s `store.CreateSessionInput{...}` construction (`internal/service/shell.go:139`)
does not set one — a `shell` session created today can never
carry a `post_destroy` value, while a `claude`/`pi` session created through `CreateAgent` can.
Task 011's own record already proposes the discharge: a follow-up task, "Pass post_destroy
through CreateShell", with its own success criteria (add `PostDestroy` to `ShellCreateInput`,
pass it into `CreateShell`'s `store.CreateSessionInput`, add a service test creating a shell
with a non-empty value and verifying the returned/durable row retains it, require
`ci/run.sh go test -count=1 ./internal/service ./internal/store` to exit 0). No task in this
plan (`tasks.json`, ids 001–037) currently carries that follow-up; task 029 does not create one
— per the standing rules, resolving a `failed` task by carving its gap into a new task, or
relabelling it `skipped`, is a distinct action from this findings report and is left to whichever
future task or planning pass takes it up.

**Dependency doom, checked fresh against `tasks.json`.** Task 011 is neither `completed` nor
`validated`, so nothing that names it in `dependsOn` can ever be scheduled:

```
$ python3 -c "
import json
d = json.load(open('/run/ralphd/tasks.json'))
for t in d['tasks']:
    if '011' in (t.get('dependsOn') or []):
        print(t['id'], t['status'], t['dependsOn'])
"
013 pending ['011', '012']
026 pending ['011']
```

**Task 013** ("Run session-then-global `post_destroy` after Archive and Delete durably
succeed") and **task 026** ("Add a `post_destroy` field to the create modal beside
`pre_launch`") are both blocked forever by task 011's `failed` status, exactly as the harness's
own `taskBlockedBy` derivation would report. Neither task 013 nor task 026 has itself been
resolved (carved into a new task with `dependsOn` repointed, or relabelled) as of this writing —
that repointing is a planning-pass action, not something this findings report performs. This
section is the disclosure the standing rules call for; it does not itself clear the doom.

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
shows the constructor call in `main.go` and the method's definition in `tui.go`.

**Disposition.** All three tasks are `validated`; none has an open residual as of this writing.
This section exists because a reader of `tasks.json`'s `validationNotes` field alone, without
walking each task's own commit history, would see an apparently-unresolved criticism on a
`validated` task — this is the record that it was, in fact, resolved, by which commit, and how
to check that fresh.

## 3. No SPEC.md-versus-tree disagreement found in R104–R109's landed work (tasks 001–028)

The PRD's own instruction ("Where this PRD and `SPEC.md` disagree, `SPEC.md` wins and the
disagreement is a finding … never an edit") was checked against every SPEC section R104–R109
name and the code that claims to satisfy it:

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

None of the above produced a disagreement; each product statement matches the SPEC section it
claims to satisfy. This section will be revisited if a disagreement surfaces while R107/R108's
remaining tasks (013–019, 026) land, since those consume §6.5's teardown-order sentence
("Session's own first, then the global one" — the *reverse* of §6.5's launch order) which no
landed task yet exercises.

## 4. Protected-path audit over this phase's commit range is clean; carried-forward out-of-scope findings not yet checked against a whole-suite run

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
single-feature `internal/features` runs), never the whole-suite sweep that would exercise the
`features`/`internal/interactive` packages these seven findings live in — that sweep is task
030's own job, not yet run as of this writing. **This section does not claim a recurrence check
here**; task 035 ("Fill in the gate dispositions") is the task that runs the by-name recurrence
grep against task 030's actual sweep log and records the result, following §3's disposition
placeholder below.

## 5. How to re-check every citation in this report

Every backticked sha above resolves under `git cat-file -e`; every backticked repo-relative path
names a file tracked under `git ls-files --error-unmatch`:

```
$ for sha in 06ea5b7 be6e42b f1788b9 5bc6f3e c13a909 ed8b81e 38ad227 34f1560 db8d5c7 0316c51 \
    b50cc20 5ef971b f60b5e4 0299be9 66711eb 8931988 db2ea55 ad51022 325d00a 630ac90 9fb25f7 \
    6b8f1d0 895f58d 17cabb8; do \
    git cat-file -e "$sha^{commit}" && echo "$sha ok"; done
(all print "<sha> ok")
$ git ls-files --error-unmatch \
    internal/service/shell.go \
    internal/service/session_context.go \
    internal/service/session_context_test.go \
    internal/service/agent.go \
    internal/service/agent_test.go \
    internal/service/launch_inputs.go \
    internal/store/store.go \
    internal/config/schema.go \
    internal/tui/tui.go \
    internal/tui/launch_inputs.go \
    internal/tui/hook_help_coverage_test.go \
    cmd/deck/main.go \
    features/launch_hooks.feature \
    SPEC.md \
    prds/phase3j-launch-and-teardown-hooks.md
(all resolve; this report's own path, docs/reports/phase3j-findings.md, becomes tracked once
this task's commit lands, which is why it is excluded from the pre-commit check above and
verified separately after committing, below.)
```

`SPEC.md` and `prds/phase3j-launch-and-teardown-hooks.md` are quoted throughout this report,
never edited by it — nothing in this findings report writes to a protected path.

## 6. Placeholder: gate dispositions (whole-suite sweep and ten-run stability), filled in by tasks 033 and 034

Both gates are pending as of this writing: task 030 (whole-suite sweep,
`docs/reports/phase3j-030-fullsuite/`), task 031 (verbose Gherkin-tally companion,
`docs/reports/phase3j-031-fullsuite-verbose/`) and task 032 (ten-run stability gate,
`docs/reports/phase3j-032-stability10/`) have not yet run. Per this phase's own plan, task 033
re-verifies the protected-path audit and the branch guards at the true final code sha and task
034 writes `docs/reports/phase3j.md` (the requirement table and both gates' dispositions); task
035 then replaces this placeholder with the actual disposition of the whole-suite sweep and the
stability gate — their exit statuses, the code sha each ran at, their published report
directories, and any recurrence of a §4 carried-forward finding, marked advisory.

**This paragraph is intentionally not a disposition.** Do not read its absence as either gate
having failed or been skipped; it means only that tasks 030–032 had not yet run as of the commit
this report itself lands in.
