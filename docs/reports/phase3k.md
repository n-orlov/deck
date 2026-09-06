# Phase 3k report

Phase 3k closes GH issue #21: the create modal's Agent field no longer offers an agent kind
whose executable cannot be found. Each adapter declares the executable its launch argv starts,
one probe function decides availability under the launch `PATH`, and that same function backs
both the modal's listing and a new create-time preflight — the direct fix for the `127`
tmux-pane-exited crash row GH #21 describes (R111, R112, R113). The `features/` harness, which
had only ever passed by never preflighting a create, gained a `pi` fixture step and had every
scenario that drives the modal to a non-shell kind install that kind's fake first (R114). This
document, `docs/reports/phase3k-findings.md` and `docs/DELIVERY-LOG.md` close the record (R115).

## Final code sha

```
$ git log -1 --format=%H -- '*.go' '*.feature'
d88c6625c4ccca71b0d31f7b5864ba030ed39e53
```

This is task 021's fixture-fix commit (`dialogs.feature: install fake claude fixture before the
every-field-reachable walk`), the last commit in this phase to touch a `*.go` or `*.feature`
path. Everything landed after it — the sweep log, the verbose companion, the stability gate, the
protected-path audit, the guard re-verification and this record — is a docs-only descendant;
`git status --porcelain` is empty and `git rev-parse HEAD origin/main` agree at
`51d227b5fc0a67ec4a06ce149abf5729e50ac9f0` as of task 026's own commit, the most recent verified
boundary check before this one.

## Per-requirement table

| req | status | task ids | shas | evidence |
|---|---|---|---|---|
| R111 | met | 001, 028, 002, 003, 004 | `47c7f3f`, `a983fa9`, `8c624ab`, `30f8d8a`, `8c4edae`, `965d029`, `647fec9`, `9ae97e8`, `a0fe68c` | `internal/agent/agent.go` (the declared-executable field/method and its equality pin against every registered adapter's `Launch` argv[0]), `internal/agent/claude.go`, `internal/agent/pi.go`, `internal/agent/shell.go`, `internal/agent/agent_test.go`, `internal/service/availability.go` (`lookPathIn` moved here beside `AvailableKinds`), `internal/service/availability_test.go` (floor/absent/present, directory-does-not-count, config `[env]` PATH wins, `lookPathIn`'s three outcomes directly), `docs/reports/phase3k-001-declared-executable/`, `docs/reports/phase3k-004-lookpathin-direct/`, `docs/reports/phase3k-028-probe-discriminating-cases/` |
| R112 | met | 005, 006, 007, 008, 009, 015 | `8c3fed4`, `237e254`, `16e95a6`, `14894e1`, `50614c6`, `589bf46` | `internal/tui/tui.go` (the availability seam, `pickCreateAgent`'s fallback dropping the "(last used)" label, `createAgentHelp` naming hidden kinds), `internal/tui/availability_seam_test.go`, `internal/tui/create_registry_agent_options_test.go`, `internal/tui/registry_guard_test.go` (`TestBlackBoxRegistrySwapNeedsNoTUIEdit` assertion left intact), `internal/tui/pick_create_agent_availability_test.go`, `internal/tui/create_agent_unavailable_help_test.go`, `internal/tui/view_never_probes_test.go`, `internal/tui/create_last_used_agent_test.go`, `internal/tui/force_indistinguishable_test.go`, `cmd/deck/main.go` (wires `service.AvailableKinds` into the seam), `docs/reports/phase3k-017-agent-availability-claude/`, `docs/reports/phase3k-020-agent-availability-remembered-pi/` |
| R113 | met | 010, 011 | `fd08afe`, `6c5202b` | `internal/service/agent.go` (`CreateAgent`'s preflight against the resolved launch `PATH`, before `Store.CreateSession`, the `agent binary "<kind>" not found on PATH` message, the `login_shell` exemption), `internal/service/agent_test.go` (refusal with no row and no tmux session asserted by absence on a real private socket, `login_shell` exemption, an installed agent's pane command left byte-identical), `internal/service/resume_test.go`, `internal/service/env_test.go`, `internal/service/pin_test.go` |
| R114 | met | 012, 013, 014, 015 (the injected-availability half, shared with R112), 016, 017, 018, 019, 020, 021 (the `dialogs.feature` fixture found and fixed while chasing the sweep green) | `310db5e`, `858e7de`, `095f02d`, `589bf46`, `23ea250`, `4744bcc`, `85168ea`, `f014ee4`, `3f8a42c`, `a542e25`, `d88c662` | `features/agent_steps_test.go` (the `a fake "pi" binary is on PATH for future deck clients` step, in the shape of the claude step), `features/permission_modes.feature`, `features/crash.feature`, `features/dialogs.feature`, `features/agent_availability.feature` (all five new scenarios: nothing-installed, only-claude-installed, installed-between-opens, create-preflight-refusal, remembered-pi-falls-back), `docs/reports/phase3k-findings.md` (§2, the full fixture inventory) |
| R115 | met | 027 (this task) | this document's own commit | `docs/reports/phase3k.md` (this document), `docs/reports/phase3k-findings.md`, `docs/DELIVERY-LOG.md` |

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
| 6 | "Existing rows are untouched." | non-goal — honoured by omission; no task changes existing-row listing, preview, archive or delete behaviour | — | — |
| 7 | "`login_shell` caveat" — list against the non-login `PATH` as the best guess, skip the submit preflight exactly as resume does | R113 | 010, 011 | `internal/service/agent.go`, `internal/service/agent_test.go` |

The issue's `## Verification` section enumerates the same unit/`features/` shapes the PRD's own
per-requirement bullets already state (probe unit evidence, TUI unit evidence, `features/`
scenarios with a fake removed from `PATH`) and is discharged by R111–R114's rows above; it is not
an eighth design item. Its `## Out of scope` subsection (the health view, a per-agent
command/path config key) matches this phase's PRD Non-goals and is honoured by omission, not by
a citable commit.

## Gate results

**Whole-suite sweep (task 021)** — `ci/run.sh go test -p=1 -count=1 ./...` at the final code sha
above. Published at `docs/reports/phase3k-021-fullsuite/sweep.log`
(`docs/reports/phase3k-021-fullsuite/README.md`). Exit status quoted verbatim from that log:

```
$ cat docs/reports/phase3k-021-fullsuite/sweep.log.exitstatus
0
```

Every package result line is `ok` (14 packages, including `features` at 331.211s) or `?` with
`[no test files]` (`internal/notify`, `internal/search`, `internal/unit`); `grep -in skip
sweep.log` produces no output — no skipped test anywhere in the suite.

**Verbose companion tally (task 023)** — the `-v` companion over `./features/...` only, same
tree, same final code sha (finding F34: the non-verbose launcher cannot print the Gherkin
tally). Published at `docs/reports/phase3k-023-fullsuite-verbose/verbose.log`
(`docs/reports/phase3k-023-fullsuite-verbose/verbose.log.exitstatus`,
`docs/reports/phase3k-023-fullsuite-verbose/README.md`). Exit status **0**; Gherkin tally as
measured, quoted verbatim (ANSI ESC bytes included) from `verbose.log` lines 5819–5820:

```
335 scenarios ([32m335 passed[0m)
3888 steps ([32m3888 passed[0m)
```

Up from phase 3j's 330 scenarios / 3824 steps, consistent with the 5 scenarios tasks 016–020
added to `features/agent_availability.feature`. The remaining two "1 scenario/1 step"
undefined/failed pairs in the same log belong to `TestGodogRejectsUndefinedAndFailedSteps`'s own
passing negative self-test, not a suite failure.

**Stability gate (task 024, cured by task 029)** — `ci/stability.sh 10` from a clean state, at
the final code sha above, relaunched from scratch and polled with `sleep 120` only (33 polls,
23:19:00 → 00:24:16 UTC, ~65.3 minutes). Published at
`docs/reports/phase3k-024-stability10/summary.log`
(`docs/reports/phase3k-024-stability10/README.md`, `poll.log`). Final line quoted verbatim:

```
$ tail -1 docs/reports/phase3k-024-stability10/summary.log
10/10 passed
```

Every one of the ten runs is `PASS`; no failing run needs naming.

**Protected-path audit (task 025)** — `git log --oneline "$BASE..HEAD" -- SPEC.md prds/
ci/Dockerfile ci/SPIKE.md`, `BASE` computed as the commit adding
`prds/phase3k-agent-availability.md` (`150d7d6f26c9fa47648214dc1a446c36ff23a376`), `HEAD` =
`dc4963f658fd31493061bca1cf864337fb6e8ff3`. Published at
`docs/reports/phase3k-025-audit/audit.log` (`docs/reports/phase3k-025-audit/README.md`). The
`git log --oneline` invocation itself produced **no output lines at all** — no commit in this
whole run touched any protected path.

**Parity guards (task 026)** — `ci/run.sh go test -count=1 -v -run
'TestHelpKeymapParity|TestFooterBindingsParity|TestFooterHandlerAgreement' ./internal/tui/` at
the final code sha, exit 0, all subtests `PASS`. Published at
`docs/reports/phase3k-026-guards/guards.log`
(`docs/reports/phase3k-026-guards/guards.log.exitstatus`,
`docs/reports/phase3k-026-guards/README.md`). `git diff --stat 150d7d6..HEAD` over the three
guard test files (`internal/tui/help_keymap_parity_test.go`,
`internal/tui/footer_bindings_parity_test.go`, `internal/tui/footer_handler_agreement_test.go`)
is empty — no commit in this run touched any of them.

**All gates are green at the final code sha `d88c6625c4ccca71b0d31f7b5864ba030ed39e53`**: the
whole-suite sweep exits 0 with every package `ok`/`[no test files]`, the verbose companion
tallies 335/335 scenarios and 3888/3888 steps passed, the stability gate is 10/10, the
protected-path audit prints nothing, and the three parity guards pass unedited.
