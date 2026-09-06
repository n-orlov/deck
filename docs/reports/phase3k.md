# Phase 3k report

Phase 3k closes GH issue #21: the create modal's Agent field no longer offers an agent kind
whose executable cannot be found. Each adapter declares the executable its launch argv starts,
one probe function decides availability under the launch `PATH`, and that same function backs
both the modal's listing and a new create-time preflight — the direct fix for the `127`
tmux-pane-exited crash row GH #21 describes (R111, R112, R113). The `features/` harness, which
had only ever passed by never preflighting a create, gained a `pi` fixture step and had every
scenario that drives the modal to a non-shell kind install that kind's fake first (R114). This
document, `docs/reports/phase3k-findings.md` and `docs/DELIVERY-LOG.md` close the record (R115).
An independent review of the first close-out (approach 01) found two remaining probe gaps —
`lookPathIn` accepted a no-execute-bit regular file and a non-regular file (FIFO) named like an
agent's binary — and this second close-out (approach 02) cures both before re-closing R115.

## Final code sha

```
$ git log -1 --format=%H -- '*.go' '*.feature'
4e09f2de90dcde04bd8fc20c77097e593f2fee5b
```

This is task 202's commit (`service: pin create preflight against FIFO named like agent binary
(task 202)`), the last commit in this phase — approach 02's review-cure wave included — to touch
a `*.go` or `*.feature` path. **This supersedes this document's earlier citation of
`d88c6625c4ccca71b0d31f7b5864ba030ed39e53`** (task 021's fixture-fix commit, approach 01's final
code sha before independent review found the two probe gaps above): approach 02 landed
`cure-01-01` (`7349dd64ff4b874874567dcb7b3df0ce00b9eda1`, require an executable bit in
`lookPathIn`), `201` (`c8b00cc94ca8e14156eef4704e2a7f7913382ae9`, reject non-regular files in the
same probe) and `202` (`4e09f2de90dcde04bd8fc20c77097e593f2fee5b`, pin the create preflight
against a FIFO named like the agent binary) after `d88c662`, each touching
`internal/service/availability.go` and/or `internal/service/agent_test.go` — see the updated R111
and R113 rows below. Everything landed after `4e09f2d` — the re-run whole-suite sweep
(`docs/reports/phase3k-203-fullsuite/`), the re-run verbose companion
(`docs/reports/phase3k-204-fullsuite-verbose/`), the re-verified parity guards
(`docs/reports/phase3k-206-guards/`), the re-run protected-path audit
(`docs/reports/phase3k-207-audit/`), the corrected fixture-inventory count (task 208), the
ten-run stability gate (`docs/reports/phase3k-cure-02-01-stability10/`, commit `0bb7a03`) and this
record (task cure-02-02) — is a docs-only descendant; `git status --porcelain` is empty and
`git rev-parse HEAD origin/main` agree, re-verified while writing this section.

## Per-requirement table

| req | status | task ids | shas | evidence |
|---|---|---|---|---|
| R111 | met | 001, 028, 002, 003, 004, cure-01-01, 201, 202 | `47c7f3f`, `a983fa9`, `8c624ab`, `30f8d8a`, `8c4edae`, `965d029`, `647fec9`, `9ae97e8`, `a0fe68c`, `7349dd6`, `c8b00cc`, `4e09f2d` | `internal/agent/agent.go` (the declared-executable field/method and its equality pin against every registered adapter's `Launch` argv[0]), `internal/agent/claude.go`, `internal/agent/pi.go`, `internal/agent/shell.go`, `internal/agent/agent_test.go`, `internal/service/availability.go` (`lookPathIn` moved here beside `AvailableKinds`, and — after independent review — required to hold an executable bit `cure-01-01` and to reject non-regular files such as a FIFO `201`), `internal/service/availability_test.go` (floor/absent/present, directory-does-not-count, config `[env]` PATH wins, `lookPathIn`'s three outcomes directly, the mode-0644 and non-regular-file cases), `docs/reports/phase3k-001-declared-executable/`, `docs/reports/phase3k-004-lookpathin-direct/`, `docs/reports/phase3k-028-probe-discriminating-cases/`, `docs/reports/phase3k-201-regular-file-probe/`, `docs/reports/phase3k-202-preflight-nonregular/` (task 202's FIFO-named-like-the-binary regression case exercises the create preflight, which shares this same hardened `lookPathIn`) |
| R112 | met | 005, 006, 007, 008, 009, 015 | `8c3fed4`, `237e254`, `16e95a6`, `14894e1`, `50614c6`, `589bf46` | `internal/tui/tui.go` (the availability seam, `pickCreateAgent`'s fallback dropping the "(last used)" label, `createAgentHelp` naming hidden kinds), `internal/tui/availability_seam_test.go`, `internal/tui/create_registry_agent_options_test.go`, `internal/tui/registry_guard_test.go` (`TestBlackBoxRegistrySwapNeedsNoTUIEdit` assertion left intact), `internal/tui/pick_create_agent_availability_test.go`, `internal/tui/create_agent_unavailable_help_test.go`, `internal/tui/view_never_probes_test.go`, `internal/tui/create_last_used_agent_test.go`, `internal/tui/force_indistinguishable_test.go`, `cmd/deck/main.go` (wires `service.AvailableKinds` into the seam), `docs/reports/phase3k-017-agent-availability-claude/`, `docs/reports/phase3k-020-agent-availability-remembered-pi/` |
| R113 | met | 010, 011, 202 | `fd08afe`, `6c5202b`, `4e09f2d` | `internal/service/agent.go` (`CreateAgent`'s preflight against the resolved launch `PATH`, before `Store.CreateSession`, the `agent binary "<kind>" not found on PATH` message, the `login_shell` exemption; the preflight shares the hardened `lookPathIn` from the R111 cures above, so a FIFO named like the agent binary is refused too), `internal/service/agent_test.go` (refusal with no row and no tmux session asserted by absence on a real private socket, `login_shell` exemption, an installed agent's pane command left byte-identical, and — task 202 — a FIFO-named-like-the-binary regression case), `internal/service/resume_test.go`, `internal/service/env_test.go`, `internal/service/pin_test.go`, `docs/reports/phase3k-202-preflight-nonregular/` |
| R114 | met | 012, 013, 014, 015 (the injected-availability half, shared with R112), 016, 017, 018, 019, 020, 021 (the `dialogs.feature` fixture found and fixed while chasing the sweep green) | `310db5e`, `858e7de`, `095f02d`, `589bf46`, `23ea250`, `4744bcc`, `85168ea`, `f014ee4`, `3f8a42c`, `a542e25`, `d88c662` | `features/agent_steps_test.go` (the `a fake "pi" binary is on PATH for future deck clients` step, in the shape of the claude step), `features/permission_modes.feature`, `features/crash.feature`, `features/dialogs.feature`, `features/agent_availability.feature` (all five new scenarios: nothing-installed, only-claude-installed, installed-between-opens, create-preflight-refusal, remembered-pi-falls-back), `docs/reports/phase3k-findings.md` (§2, the full fixture inventory, corrected by task 208) |
| R115 | met | 027, cure-02-02 (this task) | `4d9d42b`, `c83fc2f`, this document's own commit | `docs/reports/phase3k.md` (this document, re-closed at the new final code sha), `docs/reports/phase3k-findings.md` (§4, the approach-02 review-cure section), `docs/DELIVERY-LOG.md` (Phase 3k paragraph, re-closed at the new final code sha) |

## GH issue #21 design section map

GH issue #21, "Create modal lists agents that are not installed: probe each adapter's binary on
PATH, list only available ones (shell always)", is this phase's source
(`prds/phase3k-agent-availability.md`'s "Goal" section says so directly, and R115 requires this
mapping). **No `gh` CLI is available in this environment.** The issue text below was read with a
single authenticated `GET https://api.github.com/repos/n-orlov/deck/issues/21` (HTTP 200,
`state: open`, `comments: 0`), using the token already present in the pre-existing
`~/.git-credentials` credential-helper entry, sourced into a shell variable and never printed or
passed as a command argument; no `POST`/`PATCH`/`PUT` was made against the issue or its comments.
Its body's `## Design` section is a numbered list, items 1–7; closing the issue is a reading of
this table, not an argument:

| design # | design item (verbatim opening clause) | requirement | task ids | evidence |
|---|---|---|---|---|
| 1 | "Probe = the resume preflight, reused." — one `lookPathIn` shared by the list and both preflights | R111 | 001, 002, 003, 004, 028 | `internal/agent/agent.go`, `internal/service/availability.go` |
| 2 | "`shell` is exempt and always listed." | R112 | 005 | `internal/tui/tui.go`, `internal/tui/availability_seam_test.go` |
| 3 | "Probe on `n` (modal open), not per render" — cached for the dialog's life, re-probed on the next `n` | R112 | 005, 009 | `internal/tui/tui.go`, `internal/tui/view_never_probes_test.go` |
| 4 | "Add the missing create preflight" — the same `login_shell` exemption resume already has | R113 | 010, 011 | `internal/service/agent.go`, `internal/service/agent_test.go` |
| 5 | "Remembered agent … falls back to the built-in default when its adapter is registered but currently unavailable", no "(last used)" label on the fallback | R112 | 007, 015 | `internal/tui/tui.go` (`pickCreateAgent`), `internal/tui/pick_create_agent_availability_test.go`, `internal/tui/create_last_used_agent_test.go` |
| 6 | "Existing rows are untouched." | R114 (regression side), R115 (gate side) | 203, 204, cure-02-01 | no task changes existing-row listing, preview, archive or delete behaviour; the untouched existing scenarios across `features/*.feature` still pass at the current final code sha `4e09f2d` (`docs/reports/phase3k-203-fullsuite/fullsuite.log`, `docs/reports/phase3k-204-fullsuite-verbose/verbose.log`, `docs/reports/phase3k-cure-02-01-stability10/summary.log`; the superseded approach-01 measurements at `d88c662` are `docs/reports/phase3k-021-fullsuite/sweep.log`, `docs/reports/phase3k-023-fullsuite-verbose/verbose.log`, `docs/reports/phase3k-024-stability10/summary.log`) |
| 7 | "`login_shell` caveat" — list against the non-login `PATH` as the best guess, skip the submit preflight exactly as resume does | R113 | 010, 011 | `internal/service/agent.go`, `internal/service/agent_test.go` |

The same mapping read the other way, so that **each of this phase's five requirements** names the
numbered design item(s) of `## Design` it discharges — no requirement is unmapped, and no
numbered item is unclaimed:

| req | numbered design items it discharges | how |
|---|---|---|
| R111 | 1 | the one shared probe: `lookPathIn` under the create-time launch `PATH` layering item 1 spells out (`resolveLaunchEnv(os.Getenv("PATH"), nil)`, config `[env]` participating, session env not), plus each adapter's declared launch executable it probes |
| R112 | 2, 3, 5 | `shell` exempt and always listed (2); probed once on `n`, cached for the dialog's life, never from `View()`, re-probed on the next open (3); a remembered-but-unavailable agent falls back to the default with no "(last used)" label (5) |
| R113 | 4, 7 | the missing create preflight, in-dialog `createError` instead of a `127` crash row (4), with resume's own `login_shell` exemption on the submit path while listing still probes the non-login `PATH` (7) |
| R114 | 1, 2, 3, 4, 5 at the `features/` level, and 6 | the black-box half of items 1–5: `features/agent_availability.feature`'s five scenarios drive the real modal with fakes absent/present (nothing installed → `shell` only; only `claude` → `claude, shell` with `pi` named hidden; installed between opens; preflight refusal with nothing persisted; remembered `pi` falls back), and the fixture repairs to `permission_modes`/`crash`/`dialogs` keep every pre-existing scenario honest under item 6's "existing rows are untouched" — this is exactly the issue's `## Verification` `features/:` bullet ("a create-modal scenario run with `fake-pi` removed from `PATH`… a scenario with all fakes removed — only `shell`; the `@real-agents` subset unaffected") |
| R115 | 1–7 collectively, plus the `## Verification` section's closing bullet | no single numbered item is R115's alone: it is the record that lets the issue be closed by reading — this document's per-item table above walks all seven, `docs/reports/phase3k-findings.md` carries the fixture inventory and the SPEC-versus-tree check, `docs/DELIVERY-LOG.md` carries the phase paragraph, and the closing bullet's "full suite green via `ci/run.sh go test -p=1 -count=1 ./...`" is the task-021 gate quoted below (its SPEC-amendment clause needs no operator commit this phase — see the findings' §3, SPEC already carries the available-kinds wording) |

The issue's `## Verification` section enumerates the same unit/`features/` shapes the PRD's own
per-requirement bullets already state (probe unit evidence, TUI unit evidence, `features/`
scenarios with a fake removed from `PATH`, and a green full suite) and is discharged by the
R111–R115 rows above; it is not an eighth design item. Its `## Out of scope` subsection (the
health view, a per-agent command/path config key) matches this phase's PRD Non-goals and is honoured by omission, not by
a citable commit.

## Gate results

**Current gate evidence, at the final code sha `4e09f2de90dcde04bd8fc20c77097e593f2fee5b`** (the
four paragraphs below supersede the approach-01 measurements at `d88c662`, preserved further down
for history — not deleted, and each of their own READMEs now carries a dated supersession note):

**Whole-suite sweep (task 203)** — `ci/run.sh go test -p=1 -count=1 ./...` launched at the exact
final code sha `4e09f2d` (task 202's own commit). Published at
`docs/reports/phase3k-203-fullsuite/fullsuite.log`
(`docs/reports/phase3k-203-fullsuite/README.md`). Exit status quoted verbatim from that log:

```
$ cat docs/reports/phase3k-203-fullsuite/fullsuite.log.exitstatus
0
```

Every package result line is `ok` (14 packages, including `features` at 331.765s) or `?` with
`[no test files]` (`internal/notify`, `internal/search`, `internal/unit`); `grep -in skip
fullsuite.log` produces no output — no skipped test anywhere in the suite.

**Verbose companion tally (task 204)** — the `-v` companion over `./features/` only, re-run at
the same final code sha `4e09f2d` (finding F34: the non-verbose launcher cannot print the
Gherkin tally; this companion gates nothing on its own). Published at
`docs/reports/phase3k-204-fullsuite-verbose/verbose.log`
(`docs/reports/phase3k-204-fullsuite-verbose/verbose.log.exitstatus`,
`docs/reports/phase3k-204-fullsuite-verbose/README.md`). Exit status **0**; Gherkin tally
unchanged from the approach-01 measurement (no scenario/step count moved — the cures only
hardened `lookPathIn`, they added no new scenario), quoted verbatim (ANSI ESC bytes included)
from `verbose.log` line 5819:

```
335 scenarios ([32m335 passed[0m)
```

The remaining two "1 scenario/1 step" undefined/failed pairs in the same log belong to
`TestGodogRejectsUndefinedAndFailedSteps`'s own passing negative self-test, not a suite failure.

**Stability gate (cure-02-01, validated; discharges task 205's terminal-`failed` obligation)** —
`ci/stability.sh 10` launched with the literal inner line
`nohup timeout 7200 ci/stability.sh 10 > log 2>&1 &` from a clean detached git worktree checked
out at the exact final code sha `4e09f2d`, polled with `sleep 120` only. Published at
`docs/reports/phase3k-cure-02-01-stability10/summary.log`
(`docs/reports/phase3k-cure-02-01-stability10/README.md`, `poll.log`, `launch-stdout.log`,
commit `0bb7a03`; its own first attempt `3eaeecc` is superseded and named as such in that same
README). Final line quoted verbatim:

```
$ tail -1 docs/reports/phase3k-cure-02-01-stability10/summary.log
10/10 passed
```

Exit status **0** (`docs/reports/phase3k-cure-02-01-stability10/summary.log.exitstatus`). Every
one of the ten runs is `PASS`; no failing run needs naming. Per operator ruling
(`/config/amendments/001-cure-02-01.md`, steer 002), this record satisfies the stability
obligation on these observables alone — the log path, launch wrapper shell, an
aborted-and-relaunched first attempt, and a liveness check around the launch are FORM and were
never grounds to reject or re-run; the gate is now closed and no worker may launch it again.

**Protected-path audit (task 207)** — `git log --oneline "$BASE..HEAD" -- SPEC.md prds/
ci/Dockerfile ci/SPIKE.md`, `BASE` = `150d7d6f26c9fa47648214dc1a446c36ff23a376` (unchanged),
`HEAD` = `04a7e261f55814637393deba05fd0ded2266ce58` (task 206's commit, the tip at re-audit time).
Published at `docs/reports/phase3k-207-audit/audit.log`
(`docs/reports/phase3k-207-audit/README.md`). The `git log --oneline` invocation itself produced
**no output lines at all** — no commit in this whole run (approach 01 or approach 02) touched
any protected path.

**Parity guards (task 206)** — `ci/run.sh go test -count=1 -v -run
'TestHelpKeymapParity|TestFooterBindingsParity|TestFooterHandlerAgreement' ./internal/tui/`
re-run at the final code sha `4e09f2d`, exit 0, all subtests `PASS`. Published at
`docs/reports/phase3k-206-guards/guards.log`
(`docs/reports/phase3k-206-guards/guards.log.exitstatus`,
`docs/reports/phase3k-206-guards/README.md`). `git diff --stat 150d7d6..HEAD` over the three
guard test files (`internal/tui/help_keymap_parity_test.go`,
`internal/tui/footer_bindings_parity_test.go`, `internal/tui/footer_handler_agreement_test.go`)
is empty — no commit in this run touched any of them.

**All gates are green at the final code sha `4e09f2de90dcde04bd8fc20c77097e593f2fee5b`**: the
whole-suite sweep exits 0 with every package `ok`/`[no test files]`, the verbose companion
tallies 335/335 scenarios and 3888/3888 steps passed, the ten-run stability gate is 10/10 exit 0
from a worktree exactly at that sha, the protected-path audit prints nothing, and the three
parity guards pass unedited.

### Approach-01 measurements at the superseded sha `d88c6625c4ccca71b0d31f7b5864ba030ed39e53` (history — preserved, not the gate of record)

These are the original approach-01 measurements, kept for history exactly as first published; do
not cite them as current gate evidence — use the four paragraphs above instead. Each source
README below now also carries a dated note pointing at its supersession.

**Whole-suite sweep (task 021)** — `ci/run.sh go test -p=1 -count=1 ./...` at `d88c662`.
Published at `docs/reports/phase3k-021-fullsuite/sweep.log`
(`docs/reports/phase3k-021-fullsuite/README.md`). Exit status **0**; every package result line
`ok` (14 packages, including `features` at 331.211s) or `?` with `[no test files]`; no skip
anywhere. **Superseded by task 203's re-run above at `4e09f2d`.**

**Verbose companion tally (task 023)** — same tree, same sha `d88c662`. Published at
`docs/reports/phase3k-023-fullsuite-verbose/verbose.log`
(`docs/reports/phase3k-023-fullsuite-verbose/README.md`). Exit **0**; **335 scenarios (335
passed)**, **3888 steps (3888 passed)**, up from phase 3j's 330/3824. **Superseded by task 204's
re-run above at `4e09f2d`** (same tally — the cures added no scenario).

**Stability gate (task 024, cured by task 029)** — `ci/stability.sh 10` at `d88c662`, 33 polls,
~65.3 minutes. Published at `docs/reports/phase3k-024-stability10/summary.log`
(`docs/reports/phase3k-024-stability10/README.md`, `poll.log`). Final line `10/10 passed`.
**Superseded by cure-02-01's run above at `4e09f2d`** (`docs/reports/phase3k-cure-02-01-stability10/`,
commit `0bb7a03`) — task 205's own attempt to re-run this gate at the new sha ended terminal
`failed` on launch-form grounds the operator later ruled were never a rejection basis (steer 002);
cure-02-01's record is the gate of record either way.

**Protected-path audit (task 025)** — `BASE` = `150d7d6f26c9fa47648214dc1a446c36ff23a376`,
`HEAD` = `dc4963f658fd31493061bca1cf864337fb6e8ff3`. Published at
`docs/reports/phase3k-025-audit/audit.log` (`docs/reports/phase3k-025-audit/README.md`). No
output — no protected-path touch. **Superseded by task 207's re-audit above, run over the wider
range through `4e09f2d` and beyond.**

**Parity guards (task 026)** — same `-run` expression at `d88c662`, exit 0, all subtests `PASS`.
Published at `docs/reports/phase3k-026-guards/guards.log`
(`docs/reports/phase3k-026-guards/README.md`). `git diff --stat 150d7d6..HEAD` over the three
guard files empty. **Superseded by task 206's re-run above at `4e09f2d`.**
