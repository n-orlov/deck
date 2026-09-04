# Phase 3j report

Phase 3j closes GH issue #20: every pane now carries its own session's `DECK_SESSION_*`
context (R104) on every adapter and both launch paths; a global `pre_launch` composes
global-first with the session's own and the empty-global case is byte-identical to today's
tree (R105); the hook's env-mutation contract — reaches the agent, not the tmux session table,
not `state.db` — is stated and tested (R106); a global/per-session `post_destroy` runs
session-then-global on `A`/`dd`, fail-open, bounded by a 30s timeout, and never on `x` or a
reap path (R107); the four editable launch inputs (`pre_launch`, `post_destroy`,
`launch_args`, `login_shell`) are now editable on a live row through a `launch_dirty` flag and
`launch↻` badge, restart-to-apply, with `agent`/`cwd`/`slug`/`captured_path` enforced as
un-mutable by a source-scanning guard (R108); the hook rules are stated in user-reachable copy
(R109); and this document plus `docs/reports/phase3j-findings.md` and
`docs/DELIVERY-LOG.md` close the record (R110).

## Final code sha

```
$ git log -1 --format=%H -- '*.go' '*.feature'
b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7
```

This is task 046's fix commit (tightening `internal/tui/hook_help_coverage_test.go` to catch the
inaccurate R109 hook copy the independent review's finding 3 identified), the last commit in
this phase to touch a `*.go` or `*.feature` path. It supersedes task 030's earlier `a44ee32`:
review findings 1-3's fix commits (tasks 041-047 — `1a4b9db`, `d71c02f`, `a936b30`, `52e529b`,
`2042cb8`, `31e6aff`, `b29afb8`, `6a22181`) landed after `a44ee32`, advancing the final code sha.
Everything after `b29afb8` — including this report and the three refreshed gate directories
below — is a docs-only descendant; `git status --porcelain` was empty and
`git rev-parse HEAD origin/main` agreed at `ae62146b1534c136dcb1a25d30ac906594b66746` as of
task 061's own commit, the most recent verified boundary check before this one.

## Per-requirement table

| req | status | task ids | shas | evidence |
|---|---|---|---|---|
| R104 | met | 001, 002, 003; 038 (CreateShell composed through the same context builder) | `be6e42b`, `f1788b9`, `5bc6f3e`, `c13a909`, `2e5fc6b`, `566cb6d`, `4daf1d6`, `fa634db` | `internal/service/session_context.go`, `internal/service/session_context_test.go`, `internal/agent/claude.go`, `internal/service/agent.go`, `internal/service/resume.go`, `internal/service/shell.go`, `internal/service/shell_test.go`, `internal/tui/create_shell_pre_launch_test.go`, `features/launch_hooks.feature`, `docs/reports/phase3j-findings.md` (finding 3) |
| R105 | met | 004, 005, 006, 007; 038 (the same global-first composition on the `CreateShell` launch path — global `pre_launch` then the shell's own, one shell, fail-closed) | `ed8b81e`, `38ad227`, `34f1560`, `db8d5c7`, `0316c51`, `2e5fc6b`, `566cb6d`, `fa634db`, `4daf1d6` | `internal/config/schema.go`, `internal/config/schema_test.go`, `internal/service/agent.go`, `internal/service/agent_test.go`, `internal/service/resume.go`, `internal/service/resume_test.go`, `internal/service/shell.go`, `internal/service/shell_test.go` (`TestCreateShellComposesGlobalThenSessionPreLaunchBeforeTheShell`, `TestCreateShellFailingOwnPreLaunchIsFailClosed`), `internal/tui/create_shell_pre_launch_test.go`, `features/launch_hooks.feature`, `features/agent_steps_test.go`, `features/env_editor_test.go`, `docs/reports/phase3j-findings.md` (finding 3, closed by task 038) |
| R106 | met | 008, 009 | `b50cc20`, `5ef971b` | `internal/service/agent.go`, `internal/service/agent_test.go` (the export-reaches-the-agent proof against a real tmux socket and the two negative-boundary proofs — absent from the tmux session environment table and absent from every column of `state.db` for the session) |
| R107 | met | 010 (`schemaV6`, shared with R108), 011 (both the store round-trip and the CreateShell residual), 012, 013, 014, 015, 016, 017, 018, 019, 041 (`postDestroyTimeout` made an immutable constant), 042 (a failing teardown hook routed into a visible note), 043 (the note wired through `cmd/deck/main.go`), 044 (the feature step asserting the failing-hook toast text). The approach-01 commits (`f60b5e4`…`b9132c9`) alone left the immutable timeout and the production toast seam unmet; 041–044 closed both. | `f60b5e4`, `0299be9`, `9fb6aec`, `66711eb`, `259284b`, `720dafc`, `8876ad8`, `7ba666c`, `0e73841`, `3aa0fe3`, `c8e5911`, `b9132c9`, `1a4b9db`, `d71c02f`, `a936b30`, `52e529b` | `internal/store/store.go`, `internal/store/store_test.go`, `internal/config/schema.go`, `internal/service/post_destroy.go` (the immutable const), `internal/service/post_destroy_test.go`, `internal/service/post_destroy_no_hook_paths_test.go`, `internal/tui/mark_test.go`, `features/teardown_hooks.feature`, `features/teardown_hooks_test.go`, `internal/tui/archive_undo_rebuild_note_test.go`, `internal/tui/tui.go`, `internal/tui/teardown_hook_note_test.go`, `cmd/deck/main.go`, `internal/service/shell.go`, `internal/service/shell_test.go` |
| R108 | met | 010 (`launch_dirty`, shared with R107), 020, 021, 022, 023, 024, 025, 026, 027, 030 (`a44ee32`, the create-modal keyboard-walk step helpers updated for task 026's Post-destroy field) | `f60b5e4`, `8931988`, `db2ea55`, `ad51022`, `325d00a`, `630ac90`, `9fb25f7`, `6b8f1d0`, `6080c55`, `a44ee32`, `895f58d` | `internal/store/store.go`, `internal/store/store_test.go`, `internal/service/inject_launch_dirty_test.go`, `internal/service/restart.go`, `internal/service/restart_test.go`, `internal/store/no_forbidden_update_columns_test.go`, `internal/tui/launch_inputs.go`, `internal/service/launch_inputs.go`, `internal/service/launch_inputs_test.go`, `internal/tui/launch_inputs_wiring_test.go`, `internal/tui/launch_badge_test.go`, `internal/tui/create_post_destroy_test.go`, `features/dialogs_test.go`, `features/agent_steps_test.go`, `features/launch_inputs_editor_test.go`, `features/launch_hooks.feature` |
| R109 | met | 028, 045 (`2042cb8` correcting the caller-side-eval/no-pre_launch-timeout claim, `31e6aff` dropping the launch-inputs dialog's own pre_launch timeout claim), 046 (`b29afb8` tightening the coverage test to catch the inaccuracy) | `17cabb8`, `2042cb8`, `31e6aff`, `b29afb8` | `internal/tui/hook_help_coverage_test.go`, `internal/tui/tui.go`, `internal/tui/launch_inputs.go` (the idempotency claim, the fail-closed claim, the not-on-`x` claim and the never-echo claim, each asserted present somewhere a user can reach — `?` help, create-modal field help, the launch-inputs editor, settings' descriptions); `17cabb8`'s copy was inaccurate (it claimed a `pre_launch` timeout and mis-stated caller-side evaluation) until `2042cb8`, `31e6aff` and `b29afb8` landed |
| R110 | met (this document and the gates it cites; the findings/DELIVERY-LOG/Telegram tasks that round out the phase's remaining paperwork are tasks 035–037 — 035 and 036 landed as commits, 037 was a Telegram closing notification, which by its nature is sent, not committed to this repo) | 029, 030, 031, 032, 033, 034, 035, 036 (this report) | `a30accd`, `12e0e72`, `2d45ef3`, `204af7d`, `fbbda8f`, `2ad633d`, `b79228d`, `8b029e1`, `b4807ce`, `899af55`, `fe18ea9`, `7e8cf1c`, `d1d5f77`, `3a761d8` (035, `phase3j-findings.md` §8 gate dispositions), `3f658fe`, `028d25b`, `f0dee73`, `96716bb` (036, the DELIVERY-LOG.md Phase 3j paragraph) | `docs/reports/phase3j-findings.md`, `docs/DELIVERY-LOG.md`, `docs/reports/phase3j-030-fullsuite/`, `docs/reports/phase3j-031-fullsuite-verbose/`, `docs/reports/phase3j-032-stability10/`, `docs/reports/phase3j-033-guards/`, this document, the GH issue #20 design section map below |

## GH issue #20 design section map (task 034)

GH issue #20, "Launch/teardown hooks: export `DECK_SESSION_*` context, global `pre_launch`,
`post_destroy`, and editable launch inputs", is this phase's source
(`prds/phase3j-launch-and-teardown-hooks.md`'s "Goal" section says so directly, and R110
requires this mapping). Its body is organised into four numbered requirement sections
(`## R1` through `## R4`), each with its own heading; this table names, for each one, the
phase requirement that discharges it and the row already established above — closing #20 is a
reading, not an argument.

**No `gh` CLI is available in this environment.** The issue text below was read with a single
authenticated `GET https://api.github.com/repos/n-orlov/deck/issues/20` (HTTP 200,
`state: open`, `comments: 0`), so the four section headings quoted are exact; a second `GET`
immediately after showed the same `state: open` and the same comment count, so nothing was
posted or edited from here. No `POST`/`PATCH`/`PUT` was made against the issue or its
comments. The token used came from the pre-existing `~/.git-credentials` credential helper
entry and was never printed or passed as a command argument.

| issue § | design section (verbatim heading) | requirement | task ids | evidence |
|---|---|---|---|---|
| R1 | "Export the session's own identity into its pane" | R104 | 001, 002, 003; 038 | `internal/service/session_context.go`, `internal/service/session_context_test.go`, `features/launch_hooks.feature` |
| R2 | "A global `pre_launch`, editable in settings" (its own "The env-mutation contract, stated explicitly" subsection) | R105 (the global hook and its composition); R106 (the env-mutation contract subsection) | 004–007, 038 (R105); 008, 009 (R106) | `internal/config/schema.go`, `internal/service/agent.go`, `internal/service/agent_test.go`, `internal/service/shell.go`, `internal/service/shell_test.go`, `features/launch_hooks.feature` |
| R3 | "`post_destroy`, per-session and global" | R107 | 010–019 | `internal/store/store.go`, `internal/service/post_destroy.go`, `internal/service/post_destroy_test.go`, `features/teardown_hooks.feature` |
| R4 | "Every launch input editable post-start, restart-to-apply" (its own "`pre_launch` must be idempotent, and deck must say so" subsection) | R108 (the four editable inputs and the dirty flag/badge); R109 (the idempotency statement lands in user-reachable copy, not only in this issue's prose) | 020–027 (R108); 028 (R109) | `internal/store/store.go`, `internal/tui/launch_inputs.go`, `internal/tui/launch_badge_test.go`, `internal/tui/hook_help_coverage_test.go` |

The issue's own "## Out of scope" and "## Verification" subsections are not numbered design
sections and are not rows above; "Out of scope" (the motivating gateway's own auth/lifecycle,
in-place env mutation on a live pane, editing `agent`/`cwd`, per-agent-kind hook declarations)
matches this phase's PRD Non-goals and is honoured by omission, not by a citable commit.
"Verification" enumerates the same unit/feature shapes the PRD's own per-requirement bullets
already state and each is discharged by the shas in the per-requirement table above; it is not
a fifth design section.

## Gate results

**Whole-suite sweep (task 030, refreshed by task 058 at `b82e3de`)** — `ci/run.sh go test -p=1
-count=1 ./...` at the final code sha above. Published at
`docs/reports/phase3j-030-fullsuite/sweep.log` (`docs/reports/phase3j-030-fullsuite/README.md`).
Exit status quoted verbatim from that README:

```
$ cat docs/reports/phase3j-030-fullsuite/README.md | sed -n '/## Exit status/,/```/p'
## Exit status

```
0
```
```

Every package result line is `ok` (14 packages) or `?` with `[no test files]` (`internal/notify`,
`internal/search`, `internal/unit`); no `t.Skip`, no godog `@wip`/skipped marker anywhere in the
captured log. This refresh supersedes the earlier refresh published at `a44ee32` (itself a
supersession of `fbbda8f` per operator ruling `002-030`): review findings 1-3's fix commits
(tasks 041–046) landed after `a44ee32`, advancing the final code sha to `b29afb8`, so task 058
re-ran the sweep in place at the new sha; there is still no new numbered report directory.

**Verbose companion tally (task 031, refreshed by task 059 at `0fba55b`)** — the `-v` companion,
same tree, same final code sha. Published at
`docs/reports/phase3j-031-fullsuite-verbose/verbose.log`
(`docs/reports/phase3j-031-fullsuite-verbose/verbose.log.exitstatus`,
`docs/reports/phase3j-031-fullsuite-verbose/README.md`). This refresh supersedes the earlier
tally published at `a44ee32` (itself a supersession of `fbbda8f` per operator ruling `003-031`),
polled with `sleep 60` and nothing longer throughout.

**Stability gate (task 032, refreshed by task 060 at `df4f768`)** — `ci/stability.sh 10` from a
clean state, at the final code sha above. Published at
`docs/reports/phase3j-032-stability10/summary.log`
(`docs/reports/phase3j-032-stability10/README.md`). Final line quoted verbatim from that log:

```
$ tail -1 docs/reports/phase3j-032-stability10/summary.log
10/10 passed
```

The script's own captured exit status (recorded to a scratch path outside the tree at run
time, per task 060's README) was `0`; every one of the ten runs is a `PASS`, and no failing
run needs naming.

**Protected-path audit and branch guards (task 033, refreshed by task 061 at `ae62146`)** —
re-verified at the final code sha above. Published at `docs/reports/phase3j-033-guards/`. The
audit command (computed `BASE`, not pasted) printed nothing; `git status --porcelain` was empty
and `git rev-parse HEAD origin/main` agreed, both quoted verbatim in
`docs/reports/phase3j-033-guards/protected-path-audit.out`,
`docs/reports/phase3j-033-guards/git-status-porcelain.out` and
`docs/reports/phase3j-033-guards/rev-parse-head-origin-main.out`.

**Both gates are green at the true final code sha `b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7`**:
the whole-suite sweep exits 0 with every package `ok`/`[no test files]`, and the stability gate
is 10/10. Neither gate is qualified by an unresolved carried-forward finding from this phase's
own R104–R109 work; the carried-forward advisories predate this phase (see
`docs/reports/phase3j-findings.md` §4).
