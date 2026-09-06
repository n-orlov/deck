# Phase 3k findings

Companion to `docs/reports/phase3k.md` (task 027): what the requirement table does not carry —
the protected-path audit's output, the R114 fixture inventory (which scenarios needed a fixture
and which commit gave it to them), and this phase's check for a disagreement between the tree
and `SPEC.md`.

- [1. R114 fixture inventory: which scenarios needed a fixture and which commit gave it to them](#1-r114-fixture-inventory-which-scenarios-needed-a-fixture-and-which-commit-gave-it-to-them)
- [2. Protected-path audit (task 025), output reproduced](#2-protected-path-audit-task-025-output-reproduced)
- [3. Tree-versus-`SPEC.md` disagreement check: none found](#3-tree-versus-specmd-disagreement-check-none-found)

## 1. R114 fixture inventory: which scenarios needed a fixture and which commit gave it to them

The PRD's own inventory command, re-run fresh against this tree:

```
$ grep -n 'creates \(claude\|pi\) session\|opens the create modal for agent' features/*.feature
```

hits 33 feature files. Reading every hit, three files needed a **new** fixture step installed
ahead of a scenario that drives the create modal to a non-shell kind (the pre-existing fixture
step, `a fake "claude" binary is on PATH for future deck clients`, already covered most claude
scenarios from earlier phases — the gap this phase closed is `pi` and the handful of scenarios
that had slipped through):

| feature file | scenario | what was missing | commit that added it |
|---|---|---|---|
| `features/permission_modes.feature` | "pi maps only its declared permission profiles to argv" | `a fake "pi" binary is on PATH for future deck clients` (the scenario had only the pre-existing `claude` fixture line, left over from before `pi` had its own fixture step, but created three `pi` sessions with nothing on `PATH` to back them) | `858e7de` (task 013) |
| `features/permission_modes.feature` | "an unsupported profile degrades visibly rather than lying" (the `drift` scenario) | same: `a fake "pi" binary is on PATH...` — this is the scenario the PRD's own "Read this before anything else" section named by name (installs only `claude`, then creates `pi`) | `858e7de` (task 013) |
| `features/crash.feature` | "a failing pre_launch leaves visible evidence without attaching" | `a fake "claude" binary is on PATH...` (the scenario created a `claude` session with no fixture at all — its crash-tail assertion had only ever passed because the pane died from a missing binary in a way indistinguishable, to that scenario's own assertions, from the `exit 1` pre_launch failure it meant to test) | `095f02d` (task 014) |
| `features/dialogs.feature` | "create dialog -- every field is reachable by keyboard alone, editable, and each edit is visible" | `a fake "claude" binary is on PATH...` — found only while chasing task 021's whole-suite sweep green, not named in the PRD's own "known offenders" list; the scenario walks the create modal through every agent kind and had never installed `claude`'s fixture | `d88c662` (task 021) |

The new `pi` fixture step itself — `a fake "pi" binary is on PATH for future deck clients`, in
the exact shape of the existing claude step, building `cmd/fake-pi` into `agentPATHDir` under the
name `pi` with the same linger-wrapper idiom — was added by `310db5e` (task 012), in
`features/agent_steps_test.go`. `configureProbeScenario` (`features/status_probe_test.go`) was
left unchanged: it already built both fakes into the same `agentPATHDir` before task 012, so
`features/status_probe.feature`'s two claude/pi-creating scenarios needed no fixture edit — its
`Background`'s `probe fixture agents and an advanceable frozen clock are configured` step already
covers them.

**Files checked and found to need no fixture edit**, with the reason recorded so a later reader
does not re-walk the same 33-file list from scratch: every other `permission_modes.feature`
scenario, and every scenario in `agent_session.feature`, `attention_sort.feature`,
`concurrency.feature`, `create_session.feature`, `durable_identity.feature`,
`environment.feature`, `event_log.feature`, `harness.feature`,
`interactive_force_attach.feature`, `interactive_repaint_notice.feature`,
`kill_delete_undo.feature`, `launch_hooks.feature`, `lease_race.feature`, `no_leak_scan.feature`,
`preview.feature`, `resume_failure.feature`, `same_directory.feature`, `shell_liveness.feature`,
`status_attach.feature`, `status_claude_hooks.feature`, `status_recovery.feature`,
`status_theme.feature`, `status_user_kill.feature`, `terminal_repair_field_route.feature`,
`themes.feature`, and the other three `crash.feature`/`dialogs.feature` scenarios that create a
`claude` session, already had the pre-existing `a fake "claude" binary is on PATH...` step in
their own `Given`/`Background` from before this phase (confirmed by the whole-suite sweep at
`d88c6625c4ccca71b0d31f7b5864ba030ed39e53` passing with every one of them exercised — none of
these files' scenarios create a `pi` session, so R112's narrower field never had anything to
refuse them on). `features/attach_scroll.feature` and `features/interactive_scroll.feature` each
do create one `claude` session — `sp-claude` (`attach_scroll.feature:43`) and `ig-claude`
(`interactive_scroll.feature:90`) — but neither needed a fixture edit: each scenario's own
probe-fixture setup step (`Given probe fixture agents for attach-scroll are configured`,
`attach_scroll.feature:40`, calling `configureAttachScrollProbeScenario`; and `Given probe
fixture agents for interactive-scroll are configured`, `interactive_scroll.feature:87`, calling
the same function) already installs the fake `claude` binary on `PATH` before the create step
runs, independently of the shared `a fake "claude" binary is on PATH...` step used elsewhere.
`features/real_agent_smoke.feature` is `@real-agents` (excluded by
`features/godog_test.go`'s `defaultTags`, per the standing rule against editing it) and uses a
real installed `claude`, never a fake — left alone, per the PRD's own instruction.

**New scenarios asserting the phase's own observables** (R112/R113's behaviour directly, not
existing scenarios repaired) all live in the new `features/agent_availability.feature`, added by
tasks 016–020 (`23ea250`, `4744bcc`, `85168ea`, `f014ee4`, `3f8a42c`, `a542e25`): nothing
installed → only `shell`; only `claude` installed → `claude, shell` with `pi` named as hidden;
installing between opens (a second client) takes effect; the create preflight refuses in-dialog
with nothing persisted; a remembered `pi` falls back without the "(last used)" label when `pi` is
gone.

## 2. Protected-path audit (task 025), output reproduced

The PRD's own audit command, base computed rather than pasted:

```
$ BASE=$(git log --format=%H --diff-filter=A -1 -- prds/phase3k-agent-availability.md)
$ echo "$BASE"
150d7d6f26c9fa47648214dc1a446c36ff23a376
$ git log --oneline "$BASE..HEAD" -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md
(no output)
```

Task 025 ran this at `HEAD = dc4963f658fd31493061bca1cf864337fb6e8ff3`
(`docs/reports/phase3k-025-audit/audit.log`, `docs/reports/phase3k-025-audit/README.md`); this
finding re-confirms the same base and the same empty result against the tree at this document's
own writing. No commit in this whole run — task 001 through this one — touched `SPEC.md`,
anything under `prds/`, `ci/Dockerfile`, or `ci/SPIKE.md`.

## 3. Tree-versus-`SPEC.md` disagreement check: none found

The PRD's own instruction ("Where this PRD and `SPEC.md` disagree, `SPEC.md` wins and the
disagreement is a finding … never an edit") was checked against every SPEC section R111–R114
name and the code that claims to satisfy it:

- **§5's declared-executable/floor sentence** ("An adapter likewise declares the **executable**
  its launch argv starts, and the create modal offers only the kinds whose executable resolves
  on the launch `PATH`… `shell` declares no executable and is always offered") against
  `internal/agent/agent.go`'s declared-executable field and `internal/tui/tui.go`'s availability
  seam: agrees — `shell`'s declared executable is the empty string, always available, and every
  registered adapter's declaration is pinned equal to its `Launch` argv[0] by
  `internal/agent/agent_test.go`.
- **§6.3's PATH-trap mitigation bullet** ("Availability is probed, not assumed, before a launch
  is attempted… deck's own `PATH` as `captured_path`, then `[env]`'s override if any… probed when
  the modal opens, never per frame… create and resume both refuse, before any pane exists…
  `login_shell` … the refusal is skipped on both paths") against
  `internal/service/availability.go` (`AvailableKinds`, `lookPathIn`),
  `internal/service/agent.go`'s `CreateAgent` preflight, and
  `internal/tui/view_never_probes_test.go`: agrees on every clause — the `PATH` layering matches
  exactly (`resolveLaunchEnv(os.Getenv("PATH"), nil)["PATH"]`), the probe runs once per `n` and
  never from `View()`, and the `login_shell` exemption on the create path is byte-for-byte the
  same rule `resume.go` already had.
- **§11's capability list naming the field "available kinds only"** and **§11.4's remembered-choice
  rule** ("falls back to the built-in default and drops the *last used* label" for an agent whose
  "adapter is no longer registered, or whose executable no longer resolves on the launch `PATH`")
  against `internal/tui/tui.go`'s `pickCreateAgent` and
  `internal/tui/pick_create_agent_availability_test.go`: agrees — the fallback rule is extended
  from *registered* to *available*, exactly as SPEC states, and the "(last used)" label is
  dropped only for the fallback case, not for an honoured remembered agent that is still
  available.

None of the three checks produced a disagreement: every product statement matches the SPEC
section it claims to satisfy, and no edit to `SPEC.md` was made or needed.

**One pre-existing, out-of-scope doc-comment wording is noted here rather than fixed.**
`internal/tui/tui.go` (around the create-modal help text, near the field descriptions) still
reads "shell, or an agent: claude or pi" and "shell, claude, or pi" — a literal enumeration of
every *registered* kind, not a claim about which are currently *available*. This is help copy
describing the field's possible values in general (what an operator could install), not a
per-render claim about what is listed right now — the row's own help clause added by R112
(`not on PATH: pi`) is what states the currently-hidden kinds, and it is exercised by
`internal/tui/create_agent_unavailable_help_test.go`. Nothing in SPEC §5/§6.3/§11 requires this
general enumeration to change, and no requirement's criteria touch it — recorded here as an
observation, not a finding of disagreement, and not curable-in-place against any stated
criterion.
