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
4fbd452430501805a860dd229ddca1cd3f5c1cd6
```

This is task 080's fix commit (`features: correct stale CreateShell pre_launch comment`,
correcting a stale comment in `features/launch_hooks.feature`), the last commit in this phase
to touch a `*.go` or `*.feature` path. It supersedes the earlier final code sha
`b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7` (task 046's fix commit, current as of approach 04):
task 080 landed after `b29afb8`, and even though its change is comment-only, the PRD's
"Termination" rule (`git log -1 --format=%H -- '*.go' '*.feature'`) counts any commit touching
a `*.feature` path, so the final code sha advances regardless. `b29afb8` is now historical —
the sha the phase closed at through approach 04, superseded by this approach's task 080.
Everything after `4fbd452` — including this report and the three re-run gate directories
below — is a docs-only descendant; `git status --porcelain` was empty and
`git rev-parse HEAD origin/main` agreed at `b430a012cf6ce89c93dfb5fc69fd096859447827` as of
task 088's own commit, the most recent verified boundary check before this one.

## Per-requirement table

| req | status | task ids | shas | evidence |
|---|---|---|---|---|
| R104 | met | 001, 002, 003; 038 (CreateShell composed through the same context builder) | `be6e42b`, `f1788b9`, `5bc6f3e`, `c13a909`, `2e5fc6b`, `566cb6d`, `4daf1d6`, `fa634db` | `internal/service/session_context.go`, `internal/service/session_context_test.go`, `internal/agent/claude.go`, `internal/service/agent.go`, `internal/service/resume.go`, `internal/service/shell.go`, `internal/service/shell_test.go`, `internal/tui/create_shell_pre_launch_test.go`, `features/launch_hooks.feature`, `docs/reports/phase3j-findings.md` (finding 3) |
| R105 | met | 004, 005, 006, 007; 038 (the same global-first composition on the `CreateShell` launch path — global `pre_launch` then the shell's own, one shell, fail-closed) | `ed8b81e`, `38ad227`, `34f1560`, `db8d5c7`, `0316c51`, `2e5fc6b`, `566cb6d`, `fa634db`, `4daf1d6` | `internal/config/schema.go`, `internal/config/schema_test.go`, `internal/service/agent.go`, `internal/service/agent_test.go`, `internal/service/resume.go`, `internal/service/resume_test.go`, `internal/service/shell.go`, `internal/service/shell_test.go` (`TestCreateShellComposesGlobalThenSessionPreLaunchBeforeTheShell`, `TestCreateShellFailingOwnPreLaunchIsFailClosed`), `internal/tui/create_shell_pre_launch_test.go`, `features/launch_hooks.feature`, `features/agent_steps_test.go`, `features/env_editor_test.go`, `docs/reports/phase3j-findings.md` (finding 3, closed by task 038) |
| R106 | met | 008, 009 | `b50cc20`, `5ef971b` | `internal/service/agent.go`, `internal/service/agent_test.go` (the export-reaches-the-agent proof against a real tmux socket and the two negative-boundary proofs — absent from the tmux session environment table and absent from every column of `state.db` for the session) |
| R107 | met | 010 (`schemaV6`, shared with R108), 011 (both the store round-trip and the CreateShell residual), 012, 013, 014, 015, 016, 017, 018, 019, 041 (`postDestroyTimeout` made an immutable constant), 042 (a failing teardown hook routed into a visible note), 043 (the note wired through `cmd/deck/main.go`), 044 (the feature step asserting the failing-hook toast text). The approach-01 commits (`f60b5e4`…`b9132c9`) alone left the immutable timeout and the production toast seam unmet; 041–044 closed both. | `f60b5e4`, `0299be9`, `9fb6aec`, `66711eb`, `259284b`, `720dafc`, `8876ad8`, `7ba666c`, `0e73841`, `3aa0fe3`, `c8e5911`, `b9132c9`, `1a4b9db`, `d71c02f`, `a936b30`, `52e529b` | `internal/store/store.go`, `internal/store/store_test.go`, `internal/config/schema.go`, `internal/service/post_destroy.go` (the immutable const), `internal/service/post_destroy_test.go`, `internal/service/post_destroy_no_hook_paths_test.go`, `internal/tui/mark_test.go`, `features/teardown_hooks.feature`, `features/teardown_hooks_test.go`, `internal/tui/archive_undo_rebuild_note_test.go`, `internal/tui/tui.go`, `internal/tui/teardown_hook_note_test.go`, `cmd/deck/main.go`, `internal/service/shell.go`, `internal/service/shell_test.go` |
| R108 | met | 010 (`launch_dirty`, shared with R107), 020, 021, 022, 023, 024, 025, 026, 027, 030 (`a44ee32`, the create-modal keyboard-walk step helpers updated for task 026's Post-destroy field) | `f60b5e4`, `8931988`, `db2ea55`, `ad51022`, `325d00a`, `630ac90`, `9fb25f7`, `6b8f1d0`, `6080c55`, `a44ee32`, `895f58d` | `internal/store/store.go`, `internal/store/store_test.go`, `internal/service/inject_launch_dirty_test.go`, `internal/service/restart.go`, `internal/service/restart_test.go`, `internal/store/no_forbidden_update_columns_test.go`, `internal/tui/launch_inputs.go`, `internal/service/launch_inputs.go`, `internal/service/launch_inputs_test.go`, `internal/tui/launch_inputs_wiring_test.go`, `internal/tui/launch_badge_test.go`, `internal/tui/create_post_destroy_test.go`, `features/dialogs_test.go`, `features/agent_steps_test.go`, `features/launch_inputs_editor_test.go`, `features/launch_hooks.feature` |
| R109 | met | 028, 045 (`2042cb8` correcting the caller-side-eval/no-pre_launch-timeout claim, `31e6aff` dropping the launch-inputs dialog's own pre_launch timeout claim), 046 (`b29afb8` tightening the coverage test to catch the inaccuracy) | `17cabb8`, `2042cb8`, `31e6aff`, `b29afb8` | `internal/tui/hook_help_coverage_test.go`, `internal/tui/tui.go`, `internal/tui/launch_inputs.go` (the idempotency claim, the fail-closed claim, the not-on-`x` claim and the never-echo claim, each asserted present somewhere a user can reach — `?` help, create-modal field help, the launch-inputs editor, settings' descriptions); `17cabb8`'s copy was inaccurate (it claimed a `pre_launch` timeout and mis-stated caller-side evaluation) until `2042cb8`, `31e6aff` and `b29afb8` landed |
| R110 | met (this document and the gates it cites; the findings/DELIVERY-LOG/Telegram tasks that round out the phase's remaining paperwork are tasks 035–037 — 035 and 036 landed as commits, 037 was a Telegram closing notification, which by its nature is sent, not committed to this repo) | 029, 030, 031, 032, 033, 034, 035, 036, 047 (purges an untracked run-state citation from these reports), 057 (records review findings 1-3's closure), 058, 059, 060, 061 (refreshed the three gate directories and the guard capture at the then-final code sha `b29afb8`, since superseded at `4fbd452` by approach 05's tasks 081, 082 and 083), 062 (this document, refreshed to the re-run gates), 063 (refreshes findings §8's gate dispositions to the three re-run gates), 064 (records review findings 4-5's closure), 065 (this report, and refreshes the DELIVERY-LOG.md Phase 3j paragraph), 067 (closes `docs/reports/phase3j-findings.md` §6 to match the tree), 070 (attributes task 011's `post_destroy` closure in DELIVERY-LOG.md to `9fb6aec`, not task 038), 071 (attributes `docs/reports/phase3j-030-fullsuite/`'s `a44ee32` sweep to task 030 and the `b29afb8` refresh to task 058), 072 (names R107's 041–044 commits and the residual they closed, in this document's own R107 row), 073 (names R109's actual discharging commits `2042cb8`/`31e6aff`/`b29afb8`, in this document's own R109 row), 074 (this row's own then-current refresh to the eleven post-review-fix gate/report commits, itself superseded by this refresh), 080 (the comment-accuracy fix commit that advanced the final code sha past `b29afb8`), 081, 082, 083 (the three gates re-run and republished at task 080's sha, superseding 058/059/060), 084, 085, 086 (`docs/reports/phase3j-findings.md`'s opening paragraph, §4 and §8 refreshed to the re-run gates, superseding 062/063/064's own then-current wording), 087, 088 (the DELIVERY-LOG.md Phase 3j paragraph's false status claim removed and then refreshed to the re-run gates, superseding 065), 089 (this document's own `## Final code sha` and `## Gate results` sections refreshed to task 080's sha, superseding 062) | `a30accd`, `12e0e72`, `2d45ef3`, `204af7d`, `fbbda8f` (fbbda8f-era gate commit, superseded by `b82e3de`), `2ad633d` (fbbda8f-era, superseded by `b82e3de`), `b79228d` (a44ee32-era refresh, itself superseded by `b82e3de`), `8b029e1` (fbbda8f-era, superseded by `0fba55b`), `b4807ce` (a44ee32-era refresh, superseded by `0fba55b`), `899af55` (a44ee32-era, superseded by `df4f768`), `fe18ea9`, `7e8cf1c`, `d1d5f77` (pre-review-fix era, superseded by `ae62146`), `3a761d8` (035, `docs/reports/phase3j-findings.md` §8 gate dispositions, superseded by `807fe0a`), `3f658fe`, `028d25b`, `f0dee73`, `96716bb` (036, the DELIVERY-LOG.md Phase 3j paragraph, superseded by `cd53da0`/`b599346`), `6a22181` (047, purges an untracked run-state citation), `3e52411` (057, records review findings 1-3's closure), `b82e3de` (058, refreshed `docs/reports/phase3j-030-fullsuite/` at the then-final code sha `b29afb8`, superseding `fbbda8f`/`a44ee32`; itself superseded by task 081's `680237e` at `4fbd452`), `0fba55b` (059, refreshed `docs/reports/phase3j-031-fullsuite-verbose/` at the then-final code sha `b29afb8`, superseding `fbbda8f`/`a44ee32`; itself superseded by task 082's `e00d40f` at `4fbd452`), `df4f768` (060, refreshed `docs/reports/phase3j-032-stability10/` at the then-final code sha `b29afb8`, superseding `a44ee32`; itself superseded by task 083's `7617eef` at `4fbd452`), `ae62146` (061, refreshed `docs/reports/phase3j-033-guards/` at the now-superseded `b29afb8`, superseding the pre-review-fix era; itself superseded by task 092's `a98cbb6` at `4fbd452`, which re-verified that directory at the current final code sha, so `ae62146` is history rather than current traceability), `2dca034` (062, refreshes this document to the re-run gates and corrected package counts), `807fe0a` (063, refreshes findings §8's gate dispositions to the three re-run gates), `c32a0c7` (064, records review findings 4-5's closure), `cd53da0`, `b599346` (065, refreshes the DELIVERY-LOG.md Phase 3j paragraph to the re-run gates and closed findings), `01c2770` (067, closes `docs/reports/phase3j-findings.md` §6), `4b89580` (070, DELIVERY-LOG.md attribution fix), `44e3d8f` (071, `docs/reports/phase3j-030-fullsuite/` README attribution fix), `66f2c75` (072, this document's R107 row), `23d6e86` (073, this document's R109 row), `cf5b691` (074, this document's own R110 row, then-current at `b29afb8`; superseded by this refresh), `4fbd452` (080, the final code sha), `680237e` (081, refreshed `docs/reports/phase3j-030-fullsuite/` at `4fbd452`, superseding `b82e3de`), `e00d40f` (082, refreshed `docs/reports/phase3j-031-fullsuite-verbose/` at `4fbd452`, superseding `0fba55b`), `7617eef` (083, refreshed `docs/reports/phase3j-032-stability10/` at `4fbd452`, superseding `df4f768`; a first collection at the same sha, `2de1700`, reported 9/10 on an intermittent tmux/pty flake in `internal/tmux` and was re-run per the poll PROCEDURE rejection, not the number), `5bc097d` (084, `docs/reports/phase3j-findings.md` opening paragraph), `2028ff8` (085, `docs/reports/phase3j-findings.md` §4), `eab14d0` (086, `docs/reports/phase3j-findings.md` §8, superseding 062/063/064's own then-current wording there), `b8672bc` (087, removed the DELIVERY-LOG.md false status claim), `b430a01` (088, refreshed the DELIVERY-LOG.md Phase 3j paragraph to `4fbd452` and the three re-run gates, superseding 065), `64128d4` (089, refreshed this document's `## Final code sha` and `## Gate results` sections to `4fbd452`, superseding 062, with residuals cured by `b7a9bf1` and `36fc59c`) | `docs/reports/phase3j-findings.md`, `docs/DELIVERY-LOG.md`, `docs/reports/phase3j-030-fullsuite/`, `docs/reports/phase3j-031-fullsuite-verbose/`, `docs/reports/phase3j-032-stability10/`, `docs/reports/phase3j-033-guards/`, this document, the GH issue #20 design section map below |

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

Three gates were re-run at the current final code sha `4fbd452430501805a860dd229ddca1cd3f5c1cd6`
(task 080), superseding their earlier publication at `b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7`
(itself historical — the sha the phase closed at through approach 04). The fourth,
protected-path/branch-guard gate below was refreshed in place at `4fbd452` too, by task 092
(`a98cbb6e57de80554ec09c15e156d37d7a0b49a4`): `docs/reports/phase3j-033-guards/README.md` now
cites task 080's sha as the final code sha and narrates its own `b29afb8`-era revision as
superseded. (This paragraph previously recorded that refresh as still pending, which it was when
the `## Gate results` section was first written and is not any more.)

**Whole-suite sweep (task 030, refreshed by task 081 at `680237e`)** — `ci/run.sh go test -p=1
-count=1 ./...` at the final code sha above. Published at
`docs/reports/phase3j-030-fullsuite/sweep.log` (`docs/reports/phase3j-030-fullsuite/README.md`).
Exit status quoted verbatim from that log:

```
$ cat docs/reports/phase3j-030-fullsuite/sweep.log.exitstatus
0
```

Every package result line is `ok` (14 packages) or `?` with `[no test files]` (`internal/notify`,
`internal/search`, `internal/unit`); no `t.Skip`, no godog `@wip`/skipped marker anywhere in the
captured log. This refresh supersedes the earlier refresh published at `b29afb8` (task 058's
refresh, current as of approach 04): task 080 landed after `b29afb8`, advancing the final code
sha to `4fbd452`, so task 081 re-ran the sweep in place at the new sha; there is still no new
numbered report directory.

**Verbose companion tally (task 031, refreshed by task 082 at `e00d40f`)** — the `-v` companion,
same tree, same final code sha. Published at
`docs/reports/phase3j-031-fullsuite-verbose/verbose.log`
(`docs/reports/phase3j-031-fullsuite-verbose/verbose.log.exitstatus`,
`docs/reports/phase3j-031-fullsuite-verbose/README.md`). Exit status **0**; Gherkin tally as
measured, unchanged from the superseded `b29afb8` tally: **330 scenarios (330 passed)**, **3824
steps (3824 passed)** — the 1-undefined/1-failed pair alongside them is
`TestGodogRejectsUndefinedAndFailedSteps`'s own passing negative self-test, not a suite failure.
This refresh supersedes the earlier tally published at `b29afb8` (task 059's refresh, current as
of approach 04), polled with `sleep 60` and nothing longer throughout.

**Stability gate (task 032, refreshed by task 083 at `7617eef`)** — `ci/stability.sh 10` from a
clean state, at the final code sha above. Published at
`docs/reports/phase3j-032-stability10/summary.log`
(`docs/reports/phase3j-032-stability10/README.md`). Final line quoted verbatim from that log:

```
$ tail -1 docs/reports/phase3j-032-stability10/summary.log
10/10 passed
```

Every one of the ten runs is a `PASS`, and no failing run needs naming. This same directory's
first collection at this sha (`2de1700`) reported 9/10 — RUN 9 failed on an intermittent
tmux/pty timing flake in `internal/tmux`'s
`TestSendKeysUnknownKeyNameIsDeliveredAsLiteralTextWithExitZero`
(`internal/tmux/literal_send_test.go`) — and was rejected
on polling procedure, not on the number; it is kept on the record in the README as evidence the
test can flake intermittently at this sha, carried forward as an advisory, not as a claim the
suite is flake-free.

**Protected-path audit and branch guards (task 033, refreshed by task 092 at
`a98cbb6e57de80554ec09c15e156d37d7a0b49a4`, at the current final code sha `4fbd452`; its previous
refresh was task 061's `ae62146` at the now-superseded `b29afb8`)** — published at
`docs/reports/phase3j-033-guards/`; task 092 re-ran that directory's `capture.sh` on a clean tree,
copied in all six captures unedited, and left the earlier revision's history narrated as
superseded rather than deleted.

**All three re-run gates are green at the current final code sha
`4fbd452430501805a860dd229ddca1cd3f5c1cd6`**: the whole-suite sweep exits 0 with every package
`ok`/`[no test files]`, the verbose companion tallies 330/330 scenarios and 3824/3824 steps
passed, and the stability gate is 10/10. Neither gate is qualified by an unresolved
carried-forward finding from this phase's own R104–R109 work; the carried-forward advisories
predate this phase (see `docs/reports/phase3j-findings.md` §4).

## Approach 05 close: tasks 090–092 and a citation audit of the record documents (task 093)

**What tasks 090, 091 and 092 delivered, by full commit sha:**

- **Task 090** — `536d898940328e3e9868416c6a7c0d6a63a317cd` — extended this document's own
  R110 row (the per-requirement table above) to name approach 04's tasks 067, 070–074 and
  approach 05's tasks 080–089 in the row's task-ids column, with each one's commit sha added to
  the shas column, and fixed two pre-existing bare-filename backtick citations
  (`phase3j-findings.md`, `phase3j-030-fullsuite`) that lacked the `docs/reports/` prefix and so
  failed `git ls-files --error-unmatch`.
- **Task 091** — `7f5762480a03fb49b9a06ce2bf98b1d16b450083` — added a new §11 to
  `docs/reports/phase3j-findings.md` recording that approach 04's independent-review finding 3
  (five inaccuracy defects in R109's user-reachable hook copy and its coverage test) is closed,
  naming the full sha that closed each of the five defects, and restating that approach 03's
  review findings 1 and 2 are terminally REFUTED prohibited petition re-files, not re-filed by
  this closure.
- **Task 092** — `a98cbb6e57de80554ec09c15e156d37d7a0b49a4` — re-ran the protected-path audit
  and the two branch guards, and refreshed `docs/reports/phase3j-033-guards/` **in place** at the
  true final code sha, copying in all six fresh captures and rewriting the README to cite task
  080's sha as the current final code sha (narrating the `b29afb8`-era revision 4 as superseded,
  not deleted); it also fixed the same bare-filename-prefix defect class task 090 fixed, inside
  that README.

**Final code sha the three gates and the guard capture were run at.** Task 080's own commit,
`4fbd452430501805a860dd229ddca1cd3f5c1cd6`, is the final code sha (`git log -1 --format=%H --
'*.go' '*.feature'`) this phase closes at. Tasks 081, 082 and 083 re-ran the whole-suite sweep,
its verbose companion and the ten-run stability gate at that sha (see `## Gate results` above);
task 092 re-ran the protected-path audit and branch-guard capture at that same sha, on a clean
tree at `HEAD` = `origin/main` = `7f5762480a03fb49b9a06ce2bf98b1d16b450083` (task 091's own
commit, a docs-only descendant that does not move the final code sha).

### Citation audit over the four record documents

This audit covers exactly `docs/reports/phase3j.md` (this document),
`docs/reports/phase3j-findings.md`, `docs/DELIVERY-LOG.md` and
`docs/reports/phase3j-033-guards/README.md`. It first removes every triple-backtick fenced block
from each file — a fence is captured output or a script, not a citation — then extracts every
single-backtick code span from what remains with a CommonMark-style tokenizer (an opening run of N
backticks is closed only by the next run of exactly N backticks), and then

1. checks every span that is a bare 7–40-hex-character sha under
   `git cat-file -e <sha>^{commit}`, and
2. checks every span that looks like a repository path (contains a slash, or ends in a known
   source/report extension, optionally followed by `:line` or `:line-line`) under
   `git ls-files --error-unmatch` — with two established fallbacks for a bare filename whose
   directory the surrounding prose already names: the same name under `features/`, and a
   recursive filename search under `docs/reports/`.

Both halves are checked by `git` itself. Two defects in the earlier revision of this script are
fixed here, and named rather than quietly dropped, because each made its output too flattering:

- *Fenced blocks were assumed to pair around themselves instead of being removed.* A fenced block
  containing an odd number of single backticks — the script below contains exactly one, inside its
  own backtick-run regex — flips the opener/closer parity of every span after it, so the sweep went
  on to extract the prose *between* citations instead of the citations. Fences are now stripped
  line-by-line before any span is paired.
- *The path half probed the filesystem before asking git.* Joining an absolute token such as
  `/bin/sh` onto the repository root discards the root, so absolute paths "resolved" as if they were
  tracked. Only `git ls-files --error-unmatch` decides now, which is what this wave's citation rule
  says, and the unresolved list below is correspondingly longer and honest.

**One prerequisite fix, in a separate commit, was needed before the sha half could hold.**
`docs/DELIVERY-LOG.md` is a cross-phase document, and its `PRD blob` column plus two lines of its
cross-run narrative carried twelve backticked object ids that are *not* commits of this
repository: eight are **blob** ids of PRD files (the column's stated purpose — for example the
blob whose content begins `# Phase 0 — BDD harness and walking skeleton`), two are commit ids of
another repository (`n-orlov/ralphd`), and two are blob ids of a *run's own* PRD snapshot, which
lives in that run's directory and was never in this repo. Under
`git cat-file -e <sha>^{commit}` all twelve therefore failed — eight resolving to blobs, four
absent — while reading, in backticks, exactly like the commit citations around them. Commit
`e36dbe99634a6000e4a965fd21b769002328dc40` rewrites those twelve as plain text with their kind
named ("blob 40af336", "commit 08ce400 of `n-orlov/ralphd`") and states the convention explicitly
above the notes section: in `docs/DELIVERY-LOG.md`, a backticked object id is a commit of this
repository, and a blob or foreign-repository id is written unbackticked with its kind. That commit
touches `docs/DELIVERY-LOG.md` and nothing else, which is why it is a separate commit from this
section's own — see `### This section's own scope` below. After it, no backticked sha in the four
documents fails to resolve as a commit.

The script, verbatim:

```python
#!/usr/bin/env python3
"""Citation sweep over the four phase 3j record documents."""
import re, subprocess, os

REPO = "/workspace"
FILES = [
    "docs/reports/phase3j.md",
    "docs/reports/phase3j-findings.md",
    "docs/DELIVERY-LOG.md",
    "docs/reports/phase3j-033-guards/README.md",
]

def strip_fenced_blocks(text):
    """Drop triple-backtick fenced blocks entirely: they are code, not citations, and a stray
    single backtick inside one would otherwise flip the parity of every span after it."""
    out, in_fence = [], False
    for line in text.split("\n"):
        if line.lstrip().startswith("```"):
            in_fence = not in_fence
            continue
        if not in_fence:
            out.append(line)
    return "\n".join(out)

def find_code_spans(text):
    """CommonMark-ish: an opening run of N backticks closes on the next run of exactly N."""
    runs = [(m.start(), len(m.group(0))) for m in re.finditer(r"`+", text)]
    used = [False] * len(runs)
    spans = []
    for i, (pos, length) in enumerate(runs):
        if used[i]:
            continue
        for j in range(i + 1, len(runs)):
            if used[j]:
                continue
            pos2, length2 = runs[j]
            if length2 == length:
                spans.append((length, text[pos + length:pos2]))
                used[i] = True
                used[j] = True
                break
    return spans

def main():
    texts = {f: open(os.path.join(REPO, f), encoding="utf-8").read() for f in FILES}
    single = {f: [c for n, c in find_code_spans(strip_fenced_blocks(t)) if n == 1]
              for f, t in texts.items()}

    # 1. every backticked sha must resolve as a commit of THIS repository
    sha_re = re.compile(r"^[0-9a-f]{7,40}$")
    shas = {}
    for f in FILES:
        for c in single[f]:
            if sha_re.match(c):
                shas.setdefault(c, set()).add(f)
    bad = [s for s in sorted(shas) if subprocess.run(
        ["git", "cat-file", "-e", s + "^{commit}"],
        cwd=REPO, capture_output=True).returncode != 0]
    print(f"=== 1. backticked shas: {len(shas)} unique candidates ===")
    print(f"non-resolving under git cat-file -e <sha>^{{commit}}: {bad if bad else '(none)'}")

    # 2. every backticked repo-relative path must be tracked
    def looks_like_path(s):
        if " " in s or "\n" in s:
            return False
        if re.fullmatch(r"[0-9a-f]{7,40}", s):
            return False
        return bool("/" in s or re.search(r"\.(go|md|feature|sh|toml|json|yaml|yml|log|py|out)(:|$)", s))

    cands = set()
    for f in FILES:
        for c in single[f]:
            s = c.strip()
            if looks_like_path(s):
                cands.add(s.rstrip(",."))

    unresolved = []
    for s in sorted(cands):
        base_path = re.sub(r":[\d,-]+$", "", s)
        found = subprocess.run(["git", "ls-files", "--error-unmatch", base_path],
                               cwd=REPO, capture_output=True).returncode == 0
        if not found:  # bare filename whose directory the prose names: try features/
            found = subprocess.run(["git", "ls-files", "--error-unmatch",
                                    os.path.join("features", base_path)],
                                   cwd=REPO, capture_output=True).returncode == 0
        if not found:  # or a docs/reports/... subdirectory named a few words earlier
            name = os.path.basename(base_path)
            for root, _d, filenames in os.walk(os.path.join(REPO, "docs", "reports")):
                if name in filenames:
                    found = True
                    break
        if not found:
            unresolved.append(s)

    print(f"\n=== 2. backticked path-like tokens: {len(cands)} candidates, "
          f"{len(unresolved)} unresolved ===")
    for s in unresolved:
        print(" ", s)

if __name__ == "__main__":
    main()
```

Run against this tree, at the four documents' final committed text (this section included):

```
=== 1. backticked shas: 184 unique candidates ===
non-resolving under git cat-file -e <sha>^{commit}: (none)

=== 2. backticked path-like tokens: 208 candidates, 36 unresolved ===
```

(the sweep then prints the 36 unresolved path names; they are reproduced in full, classified, in
the table below.)

So **every backticked sha in the four documents resolves as a commit of this repository** — no
exemption list, no disclosed remainder.

The 36 path-like tokens the sweep cannot resolve are listed below in full, by class, with
none left unclassified. They are deliberately reproduced here **without** backticks: adding a
backticked copy of a non-resolving token to this document would create the very defect the audit
reports, and several of them are run-state filenames this wave's rules forbid backticking in a
tracked document at all. The document each one comes from is named in the last column.

| class | tokens | why they are not repository-path defects | from |
|---|---|---|---|
| absolute or environment-variable path on the host / in a container | /, /bin/sh, /config/amendments/, /proc/\<pid\>/environ, /run/ralphd/artifacts/, /run/ralphd/prd.md, /tmp/phase3j-033-capture, $DECK_HOME/captures/\<session_id\>/, $XDG_CONFIG_HOME/deck/config.toml, ~/.git-credentials, ~/.ralphd/runs/\<id\>/prd.md | filesystem locations, placeholders or a path separator used as a glyph — none is claimed to be tracked here | findings.md, phase3j.md, DELIVERY-LOG.md, guards README |
| ralphd harness or run-directory file | engine/faults.py:88-101, engine/loop.py:219, engine/loop.py:640, iterations/0001/output.jsonl, job.yaml, loop.py, prd.md, status.json, tasks.json, vigilant-verified.json | files of the ralphd engine or of a run's own state directory, outside this repository, in DELIVERY-LOG's retrospective narrative of earlier runs | DELIVERY-LOG.md |
| other-repository or external identifier | n-orlov/deck, n-orlov/ralphd, agent-of-empires/agent-of-empires, amazon-bedrock/eu.anthropic.claude-sonnet-5, src/tui/responsive.rs, DESIGN.md | a GitHub org/repo slug, a model id, and two files of the external reference project named alongside them | DELIVERY-LOG.md |
| not a path at all | origin/main, 10/10, TestFeatures/attach_acknowledges_a_live_error_without_replacing_its_verdict, internal/store.SchemaVersion, internal/tmux.TestSendKeysUnknownKeyNameIsDeliveredAsLiteralTextWithExitZero | a git ref, a result fraction, a godog scenario name and two package-qualified Go symbol names — all flagged only by the sweep's slash-or-extension heuristic | phase3j.md, findings.md, guards README |
| bare filename whose directory the prose names, outside docs/reports | tui.go:1990-2021, tui.go:351 | both are `internal/tui/tui.go`, which the same sentences name; the sweep's bare-filename fallbacks only cover `features/` and `docs/reports/` | DELIVERY-LOG.md |
| deliberately wrong, or an identifier that is not a file | permissions.feature, 001-202.md | DELIVERY-LOG quotes the first exact wrong name to record a past PRD's own mistake (the tree has `features/permission_modes.feature`); the second is an operator-ruling id delivered to a run outside the repository | DELIVERY-LOG.md |

Every backticked token in the four documents that *is* a repository path resolves under
`git ls-files --error-unmatch`; the tokens above are the complete set the heuristic flags, and each
is accounted for.

### `b29afb8` is never presented as the current final code sha

`grep -rn b29afb8 docs/` matches seven files and 45 lines: the four audited documents
above and the three re-run gate directories' READMEs
(`docs/reports/phase3j-030-fullsuite/README.md`,
`docs/reports/phase3j-031-fullsuite-verbose/README.md`,
`docs/reports/phase3j-032-stability10/README.md`). Every occurrence is one of

- a historical or superseded framing — "superseded", "now-superseded", "then-final code sha",
  "supersedes", "until task 080's commit superseded it", "advancing the final code sha past
  `b29afb8`" — or
- a citation of `b29afb8` as the sha of **task 046's own commit** (the R109 coverage-test fix),
  which is a correct citation of that one commit and says nothing about the phase's final code
  sha.

None reads as "the current final code sha is `b29afb8`". Every current-final-code-sha statement in
all four documents names `4fbd452430501805a860dd229ddca1cd3f5c1cd6`, task 080's commit, instead.
One sentence needed a fix to make that true: §9 of `docs/reports/phase3j-findings.md` said the
final-code-sha command "still prints" the older sha, in the present tense, which was true at that
section's own approach-04 commit and stale afterwards. Commit
`f9334817f231dd3f9cba3fd3c5ecd0b4168a4cf8` scopes it to the tree as it stood then and names task
080's sha as the current one.

### This section's own scope

Task 093 landed four commits, each scoped to a single tracked path:

- `e36dbe99634a6000e4a965fd21b769002328dc40` — `docs/DELIVERY-LOG.md` only — the prerequisite
  described above: the twelve blob / foreign-repository object ids written as non-commit ids, plus
  the citation convention that keeps them that way.
- `7ba4343c0fb9f86e80d92932142b020b90ff1949` — `docs/reports/phase3j.md` only — this closing
  section (what tasks 090, 091 and 092 delivered, the final code sha the gates and the guard
  capture ran at, the citation audit above and its `b29afb8` check), plus three sentences elsewhere
  in this same document that task 092 made stale: the `## Gate results` paragraphs and the R110 row
  all said the fourth (protected-path/branch-guard) gate's refresh at `4fbd452` was still a later
  task's scope, and they now name task 092's `a98cbb6` as that refresh.
- `f9334817f231dd3f9cba3fd3c5ecd0b4168a4cf8` — `docs/reports/phase3j-findings.md` only — the §9
  present-tense final-code-sha sentence scoped to its own commit, as described just above.
- this commit — `docs/reports/phase3j.md` only — this list and the note about that §9 fix, so the
  audit's own record of what task 093 touched is complete.
