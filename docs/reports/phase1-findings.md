# Phase 1 findings

`SPEC.md` remained the source of truth during Phase 1. `SPEC.md`, `ci/Dockerfile`
and `ci/SPIKE.md` are byte-identical to the phase's first commit (`758b5dc`, task
001) — verified with `git diff --stat 758b5dc -- SPEC.md ci/Dockerfile
ci/SPIKE.md`, which produces no output. `prds/phase1-durable-identity-and-agents.md`
has one operator-steering edit (commit `2dacc4f`, "forbid container sweeps by
ralphd.run label") that is unrelated to any task's content — it hardens the
job's own docker-sibling-cleanup guidance to prevent a worker from SIGKILLing its
own job container — and is not an edit made by any of this phase's numbered
tasks. No task altered PRD requirements, scope, or review guidance.

## Decisions where `SPEC.md`/the PRD left the mechanism undefined

### Env-resolution order at launch (SPEC §6.3)

- **Specification:** SPEC §6.3 states the PATH resolution order (server env ->
  `captured_path` -> config `[env]` -> session env) but does not spell out how a
  variable set at more than one layer should compose for non-PATH variables.
- **Decision:** `internal/service`'s `CreateAgent`/`Resume` apply the same
  layering to the whole environment, not only `PATH`: later layers overwrite
  earlier ones key-by-key, `captured_path` is recorded once at create time (not
  re-captured on resume), and the composed map — not raw layers — is what is
  launched and what is diffed for the audit log's variable-name list.
- **Consequence:** A session's env is deterministic and reproducible from its
  stored `env` map plus the config `[env]` layer at replay time, independent of
  the deck process's own ambient environment at the moment of resume.

### Pre-launch failure surfacing (SPEC §6.4)

- **Specification:** SPEC §6.4 says pre_launch exists for secrets/setup but does
  not define what "visible failure" means operationally.
- **Decision:** A failing `pre_launch` leaves the pane's own exit status and
  output as the primary observable signal (the pane is retained per the
  existing `remain-on-exit failed` contract) and the session row is moved to
  `error` with a stated reason; the agent binary is never invoked in that pane.
  `login_shell=1` is documented as running the pre_launch/agent pipeline via
  `$SHELL -lc` and is mutually exclusive with relying on `captured_path` (using
  one means the other's PATH is not consulted for that pane).
- **Consequence:** An operator gets both a durable row-level reason and a
  pane-level transcript, without deck attempting the agent launch against a
  broken environment.

### `pi`'s `plan` degradation (SPEC §5 vs adapter capability)

- **Specification:** SPEC §5's profile table implies every adapter supports
  every listed profile; the real `pi` binary this phase targets has no plan
  concept.
- **Decision:** The `pi` adapter declares `plan` unsupported in its `Caps` and
  the service degrades a requested `plan` to `safe`, producing a
  human-readable degradation reason string that the TUI renders explicitly
  (task 018) rather than silently launching in the requested-but-unsupported
  mode.
- **Consequence:** Nothing ever claims a permission mode the adapter cannot
  actually enforce; the badge and detail pane make the substitution visible
  instead of hiding it.

### `starting · awaiting signal` as the terminal rendered state for a launched agent row (SPEC R6/§9)

- **Specification:** SPEC forbids fabricating `running` from tmux liveness
  alone (R4/R6 read together with the Phase 2 scope boundary), but Phase 1 has
  no probe/hook mechanism yet (explicitly deferred, see below) to know whether
  an agent is actually working.
- **Decision:** A successfully launched/resumed agent row renders the literal
  string `starting · awaiting signal` (ASCII-folded to `starting - awaiting
  signal` under `DECK_ASCII=1`) and stays there; it is documented in the help
  overlay as a known Phase 1 rough edge, not implied to be a real status.
- **Consequence:** The UI never lies about liveness it cannot observe, at the
  cost of a genuinely unhelpful-looking status until Phase 2's probes land.
  `grep`-verified (task 019/030): no code path in `internal/tui` derives
  `running` from tmux liveness.

### `P` profile-switch double-gate mirrors the create-modal gate (operator steering, task 036)

- **Specification:** SPEC's yolo double-gate requirement is stated for
  creation; it does not explicitly re-derive the rule for switching an
  *existing* session's profile after the fact.
- **Decision:** Switching a profile to `yolo` via `P` requires the same
  explicit confirm keystroke as the create modal before `enter` persists it;
  switching away from `yolo`, or between two non-yolo profiles, needs no
  extra keystroke. `allow_yolo=false` removes `yolo` from the switch options
  entirely, matching the create modal.
- **Consequence:** There is exactly one way to reach a `yolo`-launched agent
  (explicit confirm), whether the session is being created or re-profiled,
  closing a gap task 020 initially left single-gated on this path.

## Contradictions / impossibilities found

### R6's "four agents" vs. this phase's actual scope

- **Specification:** SPEC R6 states deck supports four agents: Claude Code,
  Pi/oh-my-pi, Codex CLI, and shell.
- **Finding:** `prds/phase1-durable-identity-and-agents.md` explicitly scopes
  this phase to `claude`, `pi`, and `shell` only, and lists "the Codex adapter
  and its id discovery" under **Non-goals for this phase**, deferring it to
  Phase 4 (`prds/phase3-sessions-and-lifecycle.md` also names Phase 4 as
  "Codex later"). SPEC §8.2's serialised-discovery design for Codex (mint-
  after-launch id discovery, since Codex does not accept a caller-assigned id)
  is real and non-trivial, and is not implemented anywhere in this phase's
  code.
- **Resolution:** This is a documented, intentional phase-scope deferral, not
  a contradiction discovered mid-phase — recorded here per the PRD's
  instruction to log every deferral with its target phase.
- **Target phase:** Phase 4.

### "Most recent" ban (R2) vs. some adapters' native resume ergonomics

- **Specification:** SPEC R2 bans `--continue`/`resume --last`/any "most
  recent" resolution outright.
- **Finding:** Both real adapters targeted this phase (`claude`, `pi`) do
  support an explicit-id form deck can drive (`--session-id`/`--resume <id>`
  for Claude, `--session-id` for Pi on both launch and resume), so no
  contradiction actually arises for Phase 1's adapter set. This is recorded
  because it was the first thing table-driven adapter tests (tasks 003, 004)
  were built to prove negatively (asserting the banned forms are *never*
  present in any generated argv), and because Codex (deferred, above) is the
  adapter for which SPEC §8.2 anticipates this tension actually being live.

## New `DECK_*` controls introduced this phase

- **`DECK_ID_SEED`** (`internal/config.NewIDGenerator`): seeds deck's UUID
  generator so conversation ids assigned at agent-session create time are
  reproducible under test, while remaining real random UUIDs when unset. It is
  documented in the help overlay (task 030, `internal/tui/tui.go`'s
  `helpView`).

No other new `DECK_*` controls were introduced. `DECK_GODOG_TAGS` (task 029,
`features/godog_test.go`) is a test-harness-only environment variable read by
`TestFeatures` to override the Godog tag expression for a manual
`@real-agents` run; it is not a control of the released `deck` binary and is
therefore not, and should not be, listed in the help overlay.

## Deferrals with target phase

- **Codex adapter and its §8.2 serialised id-discovery.** Target: Phase 4 (per
  `prds/phase1-durable-identity-and-agents.md` and
  `prds/phase3-sessions-and-lifecycle.md`).
- **`deck _hook`, hook receipt, per-session settings injection, status probes,
  live/sampled badges, and any `running`/`waiting`/`idle` transition.** Target:
  Phase 2 (explicit PRD non-goal). This is why agent rows are stuck at
  `starting · awaiting signal` — see decision above.
- **Clean-vs-crash exit split and the crash tail.** Target: Phase 2 (explicit
  PRD non-goal).
- **The env editor, `env↻`, and restart-to-apply for env.** Target: Phase 3
  (explicit PRD non-goal) — this phase only supports env set at create time;
  `P`'s restart-to-apply wording (task 020) covers the permission profile
  only, not env.
- **Kill/undo toasts, `dd` tombstones, purge, archive, bulk marks beyond
  Phase 0's plain `x` kill.** Target: Phase 3 (explicit PRD non-goal).
- **Notifications.** Target: Phase 5 (explicit PRD non-goal).
- **Scrollback capture, history files, cwd tracking.** Target: Phase 6
  (explicit PRD non-goal).
- **Preview pane, search, health view, session send.** Target: Phase 7
  (explicit PRD non-goal).
- **systemd units.** No target phase stated in the PRDs read for this phase;
  flagged here rather than silently dropped.

## Task 032: ten-run stability loop's mislabelled-PASS defect

The pre-existing `docs/reports/phase1-ten-run-stability.log` (produced before
`ci/stability.sh` existed, by an earlier ad-hoc loop) labelled runs 5 and 9
`PASS (exit 0)` even though both runs' captured output contained a real
`FAIL github.com/n-orlov/deck/features` line from `go test`. Root cause: that
earlier loop piped `go test ... | tee run-N.log` (or equivalent) and then
checked `$?`/the loop's own exit status, which under a plain shell pipeline is
the exit status of the *last* command in the pipe (`tee`, which always
succeeds), not of `go test` itself — a classic pipefail-less pipeline bug, not
a flake in the product or in godog. Because the label came from the wrong
process's exit status, the log was not truthful evidence of ten green runs at
all, despite reading that way at a glance.
- **Consequence:** Any claim of "N consecutive green runs" that is not backed
  by a script that captures the *actual test command's* own exit status
  (never a pipeline's tail) is not trustworthy evidence; per this finding,
  PRD/report language must cite `ci/stability.sh`'s regenerated log (task 034)
  and never re-cite the original mislabelled file. `ci/stability.sh` (task
  032) fixes this by redirecting `go test`'s output to a file directly (no
  pipe) and capturing `$?` immediately, only `cat`-ing the file afterward for
  display — so PASS/FAIL always reflects the real test-command exit status.

## Task 033: same_directory resume flake root cause

The intermittent failure of `features/same_directory.feature`'s "two claude
sessions in one cwd keep separate conversation ids" scenario (~15-20% of runs;
logged in `docs/reports/phase1-ten-run-stability.log` runs 5 and 9, see the
task 032 stability-loop defect above) was reproduced directly at that rate by
looping `go test -run 'TestFeatures/two_claude_sessions_in_one_cwd'` inside
the sibling toolchain, and root-caused by preserving the failing scenario's
`DECK_HOME` (a temporary debug hook, reverted afterward) and inspecting its
`log/deck.jsonl`: in every captured failure, the resume's own audit `launch`
record (containing `--resume <conversation id>`) was genuinely present and
correctly ordered in the file. Deck's own Resume() correctly writes
`audit.Launch` strictly before the store row (and therefore the rendered
`starting` status) ever changes, so the *product* code has no ordering bug.
The race is entirely in the test harness: the feature file's `Then deck
client "A" screen contains "starting"` step only proves that client's own
rendered terminal grid contains that string; it gives no cross-process
ordering guarantee with the separate `go test` process's later, single-shot
read of the audit JSONL file written by the deck subprocess. The two
processes are only loosely ordered by wall-clock proximity around the
keypress, unlike the store-row assertions in this same file, which already
poll the database for up to a bounded deadline instead of reading once.

Fix (`features/agent_steps_test.go`, `launchArgvForSessionContains`): poll
the audit file for up to 2s (matching the existing `databaseSessionStatus`
pattern) instead of reading it exactly once, immediately after the screen
assertion returns. This is the one entry point in the file that needs it,
because every other launch-argv assertion (own/other's conversation id,
`does not contain`) runs strictly after it in both `same_directory.feature`
and `durable_identity.feature`, so by the time they run the record is already
confirmed present. No assertion's substance changed — it still fails hard,
with the same message, if `--resume` never appears within the deadline.
Verified with 160 consecutive runs of the isolated scenario (0 failures) after
the fix, versus a reproduced failure within 40 runs on the pre-fix code in the
same environment; also verified with the full `ci/run.sh go test -count=1
./...` suite green.

## No further Phase 1 contradiction found

Every other behaviour implemented this phase (adapter registry and
capabilities, store fields and mutations, CAS launch leases, `CreateAgent`/
`Resume`, the three resume-failure causes, fake-claude/fake-pi fixtures, the
create-modal field set and validation, profile badges/degradation, `r`/`p`/`P`,
and the `durable_identity`/`same_directory`/`permission_modes`/
`resume_failure`/lease-race/lease-stale feature suites) matches `SPEC.md` and
the Phase 1 PRD as read; no additional contradiction or impossibility was
found beyond the items recorded above.
