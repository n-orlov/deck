# deck — delivery log

Append-only record of phases actually run. `docs/PLAN.md` is the intended order;
`SPEC.md` is the product spec.

Each phase is one PRD run as one autonomous job. The **PRD blob** column is the git hash of
the exact PRD file contents used for that run — PRDs get edited between phases, so the path
alone is not an identification. Recover any past version with `git cat-file -p <blob>`.

## Phases

| # | Phase | PRD | PRD blob | Run ID | Started | Completed | Engine verdict | Verified |
|---|---|---|---|---|---|---|---|---|
| — | Toolchain spike | `prds/spike-sibling-toolchain.md` | blob 7ae5e05 | `deck-spike-sibling` | 2026-08-17 14:41 | 2026-08-17 14:54 | `failed / unverified` — iteration budget (10) exhausted at the audit task | **pass**, operator-verified: 4/4 tests re-run independently, ownership clean, both mount directions proven |
| 0 | Harness & walking skeleton | `prds/phase0-harness-and-skeleton.md` | blob 40af336 | `deck-phase0` | 2026-08-17 18:09 | 2026-08-17 21:21 | `failed / unverified` — review rejected approach 3 (last of 3) on R22 report defects | **substantially pass, with two defects carried to Phase 0b**: all 23 requirements implemented, suite green twice uncached (operator-run), but the suite is flaky (~1 run in 3) and the evidence report is deficient |
| 0b | Harness determinism & evidence | `prds/phase0b-harness-hardening.md` | blob 9b23161 | `deck-phase0b` | 2026-08-17 21:41 | 2026-08-17 22:33 | `failed / unverified` — review rejected all 3 approaches on stale wording in *derived* report text | **pass**, operator-verified: flake eliminated (10/10 consecutive full-suite runs), hold knob gone, evidence persisted in-repo, count convention correct. Three stale sentences fixed by the operator by hand |
| 1 | Durable identity & agents | `prds/phase1-durable-identity-and-agents.md` | blob a1951f8 (in-run snapshot blob 322a7e0, which lives in that run's own dir and not in this repo — see notes) | `deck-phase1` | 2026-08-18 14:50 | 2026-08-18 16:50 | `succeeded / verified` on approach 6 of 12, iteration 121/250 — review rejected approach 5 on a real unmet R1 | **pass**, operator-verified: 10/10 consecutive full-suite runs; R29 walkthrough executed against a **real** `claude` and the conversation provably survived the reboot stand-in. One blocking regression (create-modal default agent) was found by the operator *after* the review passed it |
| 2 | Status truth | `prds/phase2-status-truth.md` | blob 588c7fa | `deck-phase2` | 2026-08-19 18:05 | 2026-08-20 10:26 | `succeeded / verified` on approach 5 of 12, iteration 261 — reviews rejected approaches 1-4 | **pass**, operator-verified: full suite green, `ci/stability.sh 10` 10/10 |
| 2b-1 | The visible shell | `prds/phase2b1-visible-shell.md` | blob 6aea36f (in-run snapshot; the in-repo file's blob is now 3f7c1eb — see notes) | `deck-phase2b1` | 2026-08-20 15:58 | 2026-08-21 10:03 | `succeeded / verified` on approach 2 of 8, iteration 127 — review rejected approach 1 | **pass with one disagreement**: suite green, but my independent `ci/stability.sh 10` returned **9/10** where the job reported 10/10. The one failure was `crash.feature`'s SIGKILL scenario teardown, diagnosed and fixed in 2b-2 (task 014) |
| 2b-2 | Configuration & appearance | `prds/phase2b2-configuration-and-appearance.md` | blob 7331492 (the in-repo file; the in-run snapshot's blob was db465ad, which lives in that run's own dir and not in this repo — see notes) | `deck-phase2b2` | 2026-08-21 11:40 | 2026-08-22 14:34 | `succeeded / verified` on approach 4 of 8, iteration 223 — reviews rejected approaches 1-3, each on a genuinely unmet requirement | **pass**, operator-verified independently at `d346a4b`: `ci/run.sh go test -count=1 ./...` all packages `ok` (features 162.7 s), `ci/stability.sh 10` **10/10** (exit 0), `git diff 84af034..HEAD -- SPEC.md prds ci/Dockerfile ci/SPIKE.md` empty, tree clean. No disagreement with the job's verdict — unlike 2b-1, where my own 10-run came back 9/10 |

*Citation convention in this file:* an object id written inside backticks (`` `4fbd452` ``) is a
**commit of this repository** and resolves under `git cat-file -e <sha>^{commit}`. The **PRD blob**
column above holds *blob* ids of PRD files, and a few narrative ids below belong to another
repository (`n-orlov/ralphd`) or to a run's own directory rather than to this repo; those are
written in plain text with their kind named ("blob 40af336", "commit 08ce400"), never in
backticks, precisely because they are not commits here and must not read as if they were.

### Notes per run

**Toolchain spike** — proved all deck work can run in a sibling Go+tmux container with the
host workspace bind-mounted. Deliverables kept: `ci/Dockerfile`, `ci/run.sh`, `ci/SPIKE.md`.
Measured: Go 1.25.13, tmux 3.5a, cold suite 4.9 s / warm 1.1 s, no root-owned files.
The engine verdict was `unverified` purely because the 10-iteration budget ran out one task
short of its own audit step; every requirement had landed and was verified by hand instead.

Two defects were fixed in the delivered `ci/run.sh` afterwards: the cache volume had been
labelled with the run id *and hard-failed when it didn't match*, which would have broken
every subsequent job, and the script required `RALPHD_*` env so it only worked inside a job
container. It now shares the cache across runs and also works on the host.

Its most valuable output was a gotcha, now recorded in `SPEC.md` §13.2: **a pty is not a
terminal emulator** — bubbletea probes for background colour (OSC 11) and cursor position
(CPR) at start-up and blocks on the replies before rendering frame one, so a harness that
only reads will hang and look like a broken TUI.

**Phase 0** — running with `--model-strategy balanced`, strong `gpt-5.6-sol` (planning,
review, reflect) and fast `gpt-5.6-terra` (worker, verify), `--vigilant`,
`--allow-docker --network host`. Planning produced 31 tasks. Under 30-minute operator
oversight.

*Budget top-up, 2026-08-17 18:2x:* started with 40 iterations, which was too few for 31
tasks under vigilant verification. **An iteration budget cannot be topped up in flight** —
`JobConfig.load()` runs once at engine start, `budget_left()` reads that in-memory value,
and the API exposes `GET /config` with no budget mutation; editing the run dir's `job.yaml`
mid-run has no effect. The recoverable path, used here: `pause` (finishes the current
iteration and holds at a boundary) → `stop --force` → `resume --iterations +160`. Resumed at
iteration 9/200, same approach, 4 tasks completed and the workspace preserved.

Two things learned for later phases: budget generously up front (~200) since the fix costs a
container restart, and **`resume` does not inherit `--allow-docker`, `--network` or
`--image`** — they must be re-passed, or a resumed job silently loses the sibling toolchain
it needs to build anything. Note also that `stop --force` posts an abort internally, so the
run carries a cosmetic `reason: stop --force` marker; there is no way to stop a live
container without a terminal marker.

*Livelock, iterations 52–106:* the worker spent **55 consecutive iterations completing zero
tasks**. Task 018 (the `@real-agents` drift check) had been ordered before task 019 (godog
integration) that it depends on, so every iteration re-picked it, correctly concluded it was
blocked, rewrote `notes.md`, and stopped — ~25 s per iteration, 27% of the budget spent on
nothing. It also misread the requirement: the drift check only has to *exist* and be
*excluded* from the default suite; it is expected to fail without a real `claude` installed,
so no agent CLI was ever needed. Fixed by steering with the corrected order and the
requirement restated. **Lesson: an autonomous loop will spin on a blocked task forever unless
the prompt tells it to mark it `blocked` and move on — a no-op iteration must be treated as a
failure signal, not a neutral outcome.**

ralphd *does* have a stagnation breaker (`engine/loop.py:640`: three iterations with no task
progress fails the approach) and it did not fire once in those 55 iterations. The reason is
exact: it compares `json.dumps(tasks)` before and after each iteration, and every livelock
iteration *edited* `tasks.json` twice (flipping task 018's status and notes) without
completing anything. Any write to the file resets `stagnant = 0`. **The breaker measures "did
the task file change", but the property that matters is "did a task reach `completed`".** A
worker that touches the task file every iteration is invisible to it — which is precisely the
shape a stuck worker takes. The fix belongs in ralphd: count completed-task transitions, not
file mutations.

*`tasks.json` corruption:* while marking task 019 complete, the fast worker model emitted
degenerate tokens mid-write (`numerusform`, `to=functions.bash`, CJK spam) into the task
state file, leaving a valid 19-task JSON prefix followed by 7.5 kB of garbage. `ralphctl`
then reported `0/0 tasks` and the definitions for tasks 020–031 were gone. The run dir keeps
**no backup of `tasks.json`**. Recovered by extracting the planning iteration's original
`write` call from `iterations/0001/output.jsonl` — the full 31-task list is preserved there —
and replaying statuses from `vigilant-verified.json`. **Lessons: ralphd should write
`tasks.json` atomically and validate it parses before replacing, and `ralphctl status` should
say "unreadable" rather than "0/0" when it cannot parse the file.** Model output degeneration
into a state file is a failure mode a fast worker model makes real.

*Outcome, 2026-08-17 21:21:* the worker signalled COMPLETE with all tasks done and the
independent review then **rejected approach 3**, exhausting `max_approaches: 3` and taking the
run terminal at `failed / unverified` on iteration 164/200. The budget was never the binding
constraint; review quality was.

The review passes are the most valuable thing this phase produced. Approach 2 was rejected on
**seven real requirement violations that a fully green suite did not catch** — `RunReconciler`
had no production caller at all (R18: the 500 ms reconcile loop never ran in the shipped
binary); `Clock.Advance()` was called only from its own unit test, and `DECK_PREVIEW_MS`,
`DECK_ANIM` and `NO_COLOR` had no runtime consumers, so `determinism.feature` passed
vacuously (R6/R7); the help and footer advertised `r`, `f`, `space`, `dd`, `s`, `e`, `P` —
none implemented, several explicit non-goals (R4); the `starting → error` launch-failure
transition never reached the JSONL audit (R8); the harness had no pane-command step and
`NormalizeFrame` did not mask rendered relative times like `2m ago` (R19); and the evidence
report's counts came from cached runs and prose (R22). Approach 3 fixed all seven, verified
by hand. **Lesson: "the suite is green" is not evidence that a requirement is met — a
requirement can be satisfied by dead code and asserted by a vacuous test.** An adversarial
reviewer on the strong model is what caught it, and it caught defects the operator's own
spot-checks had approved.

Approach 3 was then rejected on R22 alone: the report states no unit-test count under any
defined convention (it says "75 countable `=== RUN Test…`", which conflates 42 top-level Go
tests, 13 Godog scenario subtests and 20 nested subtests), has no gotchas section, and its
R23 row cites stale cached captures.

Operator verification found **two further defects the review missed**:

1. **The suite is flaky — ~1 run in 3.** `fake_agent.feature`'s success-pane assertion fails
   with `pane does not contain "Fake Claude Code"`. Root cause: the fixture is launched with
   a fixed `FAKE_CLAUDE_HOLD_MS=1000` sleep, so the harness must land `capture-pane` inside a
   1-second window; the success fixture exits 0 and `remain-on-exit failed` retains only
   *failed* panes, so a slow round-trip destroys the pane before capture. The review ran the
   suite twice, passed both times, and concluded it was stable. **Two green runs cannot
   establish stability of a 1-in-3 flake** — R23's "passes twice" is too weak a bar, and a
   flaky harness is worse than a missing one because it teaches everyone to re-run.
2. **The report's evidence does not live in the repository.** Every "retained capture" it
   cites is a `/run/ralphd/artifacts/...` path inside the ephemeral ralphd run directory, so
   all of R22's evidence links die when that run dir is cleaned.

Both are carried into Phase 0b rather than hand-patched, so the fix is delivered and verified
the same way as everything else.

**Phase 0b** — run with the worker and verify phases switched to **Claude Sonnet 5**
(`amazon-bedrock/eu.anthropic.claude-sonnet-5`) after the fast OpenAI model's weaknesses in
Phase 0 proved to be exactly in this territory: vacuous tests, dead-code requirements and
miscounted evidence. Planning and review stayed on `gpt-5.6-sol`. The switch was made with
`--fast-model` under `--model-strategy balanced` (which maps worker and verify to the fast
tier), which is cleaner than patching `model_overrides` in a stopped run's `job.yaml`. The
Bedrock provider was added permanently to the ralphd LLM profile, so Sonnet 5 is available to
any future job. **The gateway requires the regional `eu.` prefix** — bare
`anthropic.claude-sonnet-5` and `global.…` are rejected with HTTP 400.

The quality difference was immediate and visible in the fix itself. The flake was cured
structurally rather than by widening the window: the fixture's own session pins
`remain-on-exit on` so a clean-exit pane survives, with an explicit comment that this is a
*fixture-local* departure from deck's SPEC §3.2 contract and says nothing about deck's own
behaviour, and the observation polls `capture-pane -S -` to a deadline instead of racing a
sleep. It also caught a consequence nobody had predicted: with the hold removed, tmux's own
"Pane is dead" status line can push the banner's first line off the *visible* screen, so
reading the screen would fail even when the output was correct — hence full scrollback.
`FAKE_CLAUDE_HOLD_MS` was removed from the fixture and its help text rather than left as a
dead knob. Eight of ten tasks landed in 19 iterations with zero model errors.

*Outcome:* terminal at `failed / unverified` after review rejected all three approaches in
52 minutes — but on **stale wording in derived documentation, not on the product**. The final
review confirmed, independently: build, vet and an uncached full suite all exit 0; its own
fresh ten-run sequence passed 10/10; the retained stability log carries ten exit-0 markers;
no hold knob remains; ANSI-stripping the raw transcript reproduces the readable one exactly;
repository-relative links resolve; protected files clean; no secrets. Its two findings were
that `test-count-evidence.md` still described its source as "ANSI-stripped" when that file is
now the raw transcript (652 ESC bytes), and that a *derived* package-results summary listed
eight of nine `ok` packages, omitting `internal/audit`.

Both were true, and both were single sentences in generated documentation. The operator
verified everything substantive independently — 10/10 consecutive full-suite runs, 42
top-level Go tests with exactly one `TestFeatures`, 9 `ok` packages, all evidence
repository-relative and resolving — then **fixed the three stale sentences by hand** (the
reviewer missed a third: the same parenthetical omitted `internal/agent` from the
`[no test files]` list) rather than spending a fourth job on report wording. The hand-written
ANSI claim was itself verified by byte-exact reproduction.

**Lesson for later phases: `max_approaches` is consumed by report-consistency nits as
readily as by real failures.** Requirement 10 of the 0b PRD ("every claim in the report must
be true of the tree as delivered") is correct and worth keeping — it is what caught genuinely
false evidence — but combined with a literal reviewer it can burn three approaches on three
sentences while the product sits finished. Future PRDs should either separate "the product is
correct" from "the report is internally consistent" into distinct review gates, or grant more
approaches for documentation-only defects. A rejection whose findings are all typographical
should not read the same as one that finds dead code.

**Phase 1** — Opus 5 on planning and review, Sonnet 5 on worker and verify
(`--model-strategy balanced`), `--vigilant`, `--allow-docker --network host`, 250 iterations,
40 planned tasks. Under 30-minute operator oversight. This phase committed and pushed its own
work, so the commit log is the durable record: 40 task commits plus 11 more for the R1 rework.

*Two PRD blobs.* The row cites the committed `prds/` blob, but agents never read that file:
`build_prompt` passes only the path of `run.prd_file` (`/run/ralphd/prd.md`), a **snapshot
seeded once** from the config dir `if not run.prd_file.exists()` and **never re-seeded on
resume**. The operator amended the workspace `prds/` copy first and it had no effect
whatsoever; the fix had to go into the run snapshot, which is why the two blobs differ.
**Lesson: mid-run PRD amendments must edit `~/.ralphd/runs/<id>/prd.md`.** Amending the repo
copy changes nothing until the next fresh run.

*Three self-kills (iterations 17, 75, 95).* The job SIGKILLed itself three times —
`signal=9`, `exitCode=137`, presenting externally as `API unreachable` while `status.json`
still said `running`, so it looked like a stall. Each time the last tool call was
`docker ps -a --filter label=ralphd.run=deck-phase1 -q | xargs -r docker rm -f`. The job's own
container carries that label. The root cause is **ralphd's own prompt**:
`_docker_siblings_note()` (`engine/loop.py:219`) instructs agents to label every sibling
`ralphd.run=$RALPHD_RUN_ID` "so it gets reaped with this job" and to "delete any you did not
mean to keep" — an instruction whose literal execution is suicide. Steering could not have
fixed it either: all three happened in the **verify** phase, and
`STEERING_ACTIONABLE_PHASES = {"planning", "worker"}`, so steering in any other phase is
injected as read-only "not for this phase" context. Fixed by an explicit counter-instruction in
the run's PRD snapshot naming the three fatal commands; it held for the remaining 100+
iterations. Filed against ralphd as issue #11 with a comment on #7.

*A DNS outage consumed two whole approaches (3 and 4, iterations 86–92).* The gateway went
unresolvable (`getaddrinfo EAI_AGAIN`) for three minutes and the stagnation breaker fired twice
90 seconds apart. Precise cause: `classify_fault()` (`engine/faults.py:88-101`) computes
`is_failure` from exit code and timeouts **before** consulting the error text, and these
iterations exited **0** with an infra error and `totalTokens: 0`. So `classify_fault` returns
`None` — no `infra_retry`, no refunded iteration — while `_check_instant_failure` requires
`exit_code not in (0, None)` and therefore *reset* the streak instead of tripping. **An infra
outage that exits 0 is billed to the model as a quality failure.**

*`max_approaches` cannot be raised in flight either* — same shape as Phase 0's budget lesson.
`self.cfg.max_approaches` is read once at engine start; editing `job.yaml` mid-run leaves
`status.json` reporting the old value until a `resume` restarts the engine. Raised 4 → 8 → 12
here, of which only the restart-backed raises ever took effect.

*Review quality.* The review earned the phase. It rejected approach 5 on a **real unmet R1**:
the registry existed and `internal/service` consumed it, but the TUI never received it —
`internal/tui/tui.go` carried a hardcoded `createAgentOptions = {"shell","claude","pi"}` and a
`createAgentCapabilities` switch constructing adapters by name, so a Phase 4 Codex adapter
would have been invisible and unusable in the TUI. It also **falsified the vacuity risk**
rather than asserting it away: it rekeyed `cmd/fake-claude`'s transcripts to a shared
`shared.jsonl` and confirmed the suite failed at exactly `durable_identity.feature:41` and
`same_directory.feature:22`, proving T1's "beta replays its own last message" is load-bearing.

*The defect the review missed, and the misdiagnosis that nearly buried it.* Approach 6's R1 fix
threaded the registry into the TUI correctly, but changed the create modal's default from
`createAgentOptions[0]` (`"shell"`, deliberately) to `registry.Kinds()[0]` — and `Kinds()` sorts
alphabetically, so the default silently became **claude**. Pressing `n` and Enter then tried to
launch a binary most machines do not have, where shell had been the safe default. Two `cmd/deck`
PTY tests caught it, and the job labelled them "a pre-existing PTY-timing flake in this sandbox,
confirmed via `git stash`" — reasoning that could not hold, because tasks 001-003 were already
**committed**, so stashing left the regression in place in both runs. "Identical before and
after" was true and meant nothing. The operator reproduced both failures, traced them to
`tui.go:351`, and steered. **Lesson: "confirmed by `git stash`" is only evidence when the
suspected change is the uncommitted one.** Left alone, the next task would have re-proven ten
green runs on a tree with two failing tests. The fix (`defaultCreateAgent`, shell-preferred with
a `Kinds()[0]` fallback) is now pinned by a test registering an adapter that sorts *before*
shell — the previous guard would have passed either way.

*Operator verification, 2026-08-18 18:00-18:10.* Ten consecutive full-suite runs: 10/10, suite
exit 0. No `--continue`, `resume --last` or "most recent" form is constructed anywhere; every
occurrence in the tree is a negative assertion or a comment. Every repository-relative evidence
path cited by `docs/reports/phase1.md` and `phase1-findings.md` exists.

The R29 walkthrough was then run **for real**, not partially, because the host has a genuine
`claude` on PATH — and it works end to end. `n` opens on `shell` (fix confirmed black-box in the
shipped binary), `right` once reaches `claude` exactly as documented, the profile field offers
only `safe, plan, edits` with `yolo` withheld *and the reason stated*. Creating gave
`argv: ["claude","--session-id","20c4ecb5-…","--permission-mode","manual"]` with
`env_keys: ["PATH"]` — names only, no values — a persisted UUID `conversation_id`, and a row
reading `starting - awaiting signal`, never `running`. A distinctive phrase was exchanged with
the real CLI; `tmux kill-server` as the reboot stand-in left the row `stopped - resumable` with
**no tmux server auto-started**; `r` produced
`["claude","--resume","20c4ecb5-…","--permission-mode","manual"]` — same id, no `--continue` —
and the attached pane came back carrying the original exchange, with `manual mode on` proving the
`safe` profile survived. That is the whole promise of the phase, observed rather than inferred.

One documentation nit, deliberately not blocked on: the walkthrough's inspection command
`grep smoke "$DECK_HOME"/log/deck.jsonl` returns nothing, because the audit log keys records by
session **id**, not name. The record it points at is correct and present.

*Operator hand-test findings, 2026-08-18 (three agents, real binary, real CLIs).* `shell`,
`claude` and `pi` sessions all launched; `ctrl+c` twice stopped the agent sessions and the rows
correctly went `stopped`. Three findings, none of which the suite could have caught because all
three are gaps in what deck *observes*, not in what it does:

1. **The reconciler is blind to dead panes.** `Reconcile` calls `list-sessions` only, so it
   detects a session disappearing but never a retained dead pane. Typing `exit` in a shell
   session returns the last command's status; when that is non-zero, `remain-on-exit failed`
   retains the pane by design (§3.2) and the tmux *session* still exists — so the row reads
   `starting` indefinitely. SPEC §7 already specifies the fix (`list-panes -F` with
   `pane_dead`/`pane_dead_status`, mapping a dead pane with non-zero status to `error` plus a
   crash tail); it is Phase 2's clean-vs-crash split, now called out explicitly in the plan.
2. **"starting elsewhere" is reported for a row that nobody has leased.** A real Phase 1
   defect, and a compounding one. `AcquireLaunchLease` returns `LaunchLeaseHeldElsewhere` both
   when a live owner holds the lease *and* when the row simply is not `stopped`
   (`internal/store/lease.go:148`), and `Resume` collapses every non-acquired outcome into
   `ResumeStartingElsewhere` (`internal/service/resume.go:104`). So a row wedged at `starting`
   by finding 1 reports "starting elsewhere" on `r` with no other client involved, and looks
   unresumable until the 30 s TTL and a status change. The store already carries the
   distinction — `HeldStatus` is populated and `HeldBy` is empty — so this is a UI-side
   conflation, not missing information. **Lesson: a message that names a cause must be derived
   from that cause**; "not acquired" and "someone else owns it" are different facts.
3. **A `shell` row would sit at `starting` forever, in every phase.** Not a Phase 1
   limitation: a shell has no hooks to fire and nothing to probe, so no rule in §7 as written
   could ever promote it. Fixed in the spec rather than in code — for `shell` rows only, tmux
   liveness promotes `starting → running`, sound precisely because no higher-precedence source
   exists for a shell that could contradict it, and explicitly *not* generalised to agent rows
   where the fabricated-`running` prohibition still binds.

**Phase 2** — Opus 5 planning/review/reflect, Sonnet 5 worker/verify, `--vigilant --reflect`,
`--allow-docker --network host`, 37 requirements, 84 commits (`55e04e6..17cc346`).
215 worker iterations, 37 verify, 5 planning, 5 review.

**Four approaches were rejected before the fifth passed**, and that is the phase's main lesson:
the reviews were right every time. Status truth is a domain where a plausible-looking
implementation is the failure mode — a row that says `running` because nothing proved otherwise
is exactly the fabrication the spec forbids — so "the tests pass" carried very little
information, and the review pass earned its cost.

**Phase 2b-1** — same model split and flags, 24 requirements, 59 commits
(`17cc346..84af034`). 81 worker iterations, 49 verify, 2 planning, 2 review, 1 reflect.

*Two run-killing incidents, both worth knowing about:*

- **The job SIGKILLed itself.** At iteration 9, in a verify turn, it ran
  `docker ps -a --filter "label=ralphd.run=deck-phase2b1" -q | xargs -r docker rm -f`.
  The job's *own* container carries that label, so the sweep killed the job mid-verify and
  lost task 004's verdict. Fixed forward in the PRD (`e5d2b58`) by removing requirement 46's
  container-hygiene ask entirely — every sibling `ci/run.sh` starts is already `--rm`, so there
  was nothing to clean — and by moving the label hazard and the safe read-only command *inline
  into the requirement whose verification triggers the hazard*, rather than leaving them in a
  Constraints section three sections away. A constraint that contradicts a requirement loses to
  the requirement.

  **Correction to `e5d2b58`'s own commit message** (recorded here because published history is
  fixed forward, never amended): that message says Phase 1 "died the same way at its own
  iteration 17, also in verify". `deck-phase1`'s event log does not support either detail —
  iteration 17 was a **worker** iteration that ended `exitCode: 0`, and the only interrupted
  iteration in that run is **68** (worker, 2026-08-18 12:24:53Z, `exitCode: -2`,
  `interrupted: true`). The substance of the commit — that this class of self-kill had happened
  before and the PRD was the right place to fix it — stands; the iteration number and phase in
  its narrative do not.

- **The in-run PRD snapshot differs from the in-repo file**, which is why the blob column
  records blob 6aea36f rather than today's blob 3f7c1eb. The run was given the version where the
  container-hygiene item was requirement 45; the repo's later edit renumbered it to 46. Same
  class of note as Phase 1's: the run dir's `prd.md` is the authoritative record of what a job
  was actually asked to build.

**Phase 2b-2** — Opus 5 planning/review/reflect, Sonnet 5 worker/verify, `--vigilant --reflect`,
54 requirements, 92 commits (`84af034..d346a4b`). 113 worker iterations, 104 verify, 4 planning,
4 review, 1 reflect, ~388 M tokens, 27 h wall clock across two containers.

**Four approaches, and unlike Phase 2 the reasons differ from each other.** Worth reading as a
set, because three of the four are avoidable and one is a ralphd defect:

1. **Approach 1 was lost to a scheduler bug, not to bad work.** The plan had 56 tasks; the worker
   signalled `COMPLETE` after 14 were verified, and the engine accepted it, spent a review pass
   (which correctly reported "roughly a quarter built"), archived the approach and re-planned.
   ~5 hours and one of eight approaches, gone. ralphd has no guard refusing a `COMPLETE` signal
   while `tasks.json` still has pending entries; it should.
2. **Approach 2 (38 tasks) actually built the phase** and its review found four real defects: a
   §9 lifecycle action (attach) reachable **by mouse** from inside the settings takeover
   (requirement 24); the `[env]` table not editable with the deviation undisclosed (17); the
   §11.4 dialog contract asserted for one of five dialogs (11); and 9/10 stability, judged
   acceptable (54).
3. **Approach 3 (19 tasks) closed all four**, and its review found two more, both genuine:
   only `[env]` was labelled restart-to-apply while none of the seven flat keys took effect live
   either (19), and a `ctrl+s` with nothing edited copied the environment's value over the file's
   own (21).
4. **Approach 4 (14 tasks) closed those** — and introduced, then fixed, one more.

**The most instructive defect in the phase**, because a passing test hid it: requirement 19's fix
(apply on save the fields whose scope claims they take effect live) reopened requirement 21 from
the other side. `settingsApplyLiveFields` never consulted `Settings.EnvOverrides`, so an *edited*
`ctrl+s` copied the file value into the running settings even while the overriding `DECK_*`
variable stayed set — and for `ui.mouse` it also emitted a real `tea.DisableMouse()`, so the
terminal's own mouse reporting followed a file edit the environment was supposed to outrank.
Three tests looked like they covered it and none did: the labelling test toggled the staged value
*toward* the env-resolved value, so the bug was invisible by **fixture coincidence**; the new
requirement-21 scenario is a no-edit save, which the live-apply guard never reaches; and the
`Scope`↔behaviour parity test never set a `DECK_*` variable at all. Found by operator inspection
after the tasks were complete, steered as `008-envoverride-applylive`, and closed by `d346a4b`
with negative proofs in both directions. The general lesson, now carried into Phase 3's PRD: a
test whose fixture makes the correct and incorrect behaviours produce the same observation is
not evidence, and boolean assertions are especially prone to it.

*Two infrastructure incidents, both ralphd's rather than deck's:*

- **The iteration-timeout crash killed the engine twice** (21 Aug 23:23, 22 Aug 00:48 BST) on the
  same iteration — the second immediately after a resume, so a deterministic crash loop that
  resuming alone cannot escape. `asyncio.wait_for` cancels the task it waits on, so a second
  `wait_for(pump_task, timeout=30)` raises `CancelledError`, and being a `BaseException` it
  escaped `loop.py`'s `except Exception` — the guard whose entire purpose is to make an iteration
  failure cost one iteration rather than the job. Unblocked by raising `iteration_timeout_s`
  3600 → 10800 in the run's own `job.yaml`; fixed properly in ralphd by commit 08ce400 of `n-orlov/ralphd`
  (another repository, not this one), which uses
  `asyncio.wait()` on the timeout path so the pump survives and the SIGINT drain still works.
- **The same crash hit a third time at 13:02 BST on 22 Aug when the wall clock expired
  mid-`verify`**, which is the more important shape: a *job timeout* also produces a dead engine
  with `status.json` still reading `running` and **no verdict written**. Resumed with
  `--iterations +100 --network host --allow-docker`; it continued approach 4 rather than
  re-planning, and the pending steer file survived the crash and was consumed on the first new
  iteration.

*Carried out of this phase deliberately, not lost:* deck enables no bracketed-paste handling at
all, and list/dialog navigation switches on `msg.String()`, so two keystrokes arriving in one
`read(2)` are both silently dropped while text fields (which read `msg.Runes`) are unaffected.
The 2b-2 harness flake this diagnosed is fixed; the product exposure is Phase 3's requirement 51.
The two naming defects also carried out of here — a test file named after a task number, and two
sections of `docs/reports/phase2b2-findings.md` that both opened `## Task 014 —`, because task ids
reset per approach — are **done, not outstanding**: Phase 3f's R67 renamed all eleven task-numbered
test files after their subjects (`b848d28`, evidence `300ee86`,
`docs/reports/phase3f-019-r67-test-file-names.md`) and retitled the two report sections after what
they found — `SIGKILL teardown hang …` (`:1249`) and `Requirement 19/21 correction …` (`:1511`) —
with every citation in `docs/` repointed
(`docs/reports/phase3f-020-r67-report-section-titles.md`).

**Phase 3f** — `prds/phase3f-residuals-and-suite-determinism.md`, run `deck-phase3f`, 2026-08-26.
Thirteen requirements: the PRD's eleven in two halves — the *residual* half (R63–R67, suite
determinism and report hygiene) and the *field* half (R68–R73, the operator's GitHub bug log, issues
#5–#10) — plus **R74 and R75**, which the PRD never contained and the operator authorised mid-run
from issue #11 (list (c) below, kept separate from both bug-log reconciliations). Evidence:
[`docs/reports/phase3f.md`](reports/phase3f.md) (per-requirement, with revert-and-reproduce proofs
for the nine requirements the PRD named a naive test for) and
[`docs/reports/phase3f-findings.md`](reports/phase3f-findings.md) (what it found and did not fix).
Commits run from `f3c25d5` to the final code commit `0a5034d`. The phase took two approaches: the
first ended at code sha `e47cb35`, whose tree is identical to `c12c30e` (its suite and stability runs
were made there) and was rejected on two deliverables; the second closed finding F1 (`2b39124`),
added R74 and R75, and re-took both deliverable runs at `0a5034d`.

**The bug log this phase inherited was two different lists, and they must not be read as one.**
One is Phase 3e's residual bug log, whose items were already fixed *before* Phase 3f started and
for which the PRD forbade writing requirements at all; the other is the six field defects the
operator hit on their daily driver on 26 Aug 2026 and filed as issues #5–#10, which is what the
field half of this phase actually built. Both are reconciled below, separately.

*(a) Already fixed before Phase 3f — five items, no Phase 3f work and no Phase 3f credit.*
Re-verified against the tree at `a9ff496` by task 024, and no row of the PRD's claim turned out
wrong ([`phase3f-findings.md` §7](reports/phase3f-findings.md)):

| Bug-log item | Fixed before this phase by |
|---|---|
| `probe.miss` grows unboundedly and wedges the `E` event log | `9f73996` (Phase 3e task 329, requirement R59) — `probe.miss` is no longer written as an event |
| A coalesced `KeyMsg` drops **both** runes (`"jm"` matches no case) | `465a7d9` (Phase 3 task 118, requirement 51) — a multi-rune `KeyRunes` is split into one single-rune message per character |
| `crash.feature`'s SIGKILL scenario hangs in its after-scenario hook, ~1 run in 10–30 | the same `465a7d9`; root-caused to the coalesced-keystroke mechanism (a coalesced `"iq"` reaching neither `case`) in `docs/reports/phase2b2-findings.md` |
| `harness.feature`'s "a fake agent renders a preview fixture once and then falls silent#01" | `17e91ce` (Phase 3 task 114, read-buffer race under host load) |
| `internal/interactive`'s `TestSessionResizeDuringLiveDrainIsRaceFree` goroutine-outlives-test panic | `fb9bd71` (Phase 3e task 325) — a real `sync.WaitGroup` join |

Phase 3f's contribution to these five was **re-verification, not repair**: each was re-checked
against today's tree, which matters most for the last row, since R68 rewrote the very drain and
grid machinery that test drives and the join plus a green `-race` package run still hold. Two of
the five now cite different lines than the PRD did (the `KeyMsg` split is at `tui.go:1990-2021` at
the tip), and the `crash.feature` row's confidence rests on thirty clean whole-suite runs rather
than twenty. The PRD's "seven items, five already fixed" is not a miscount: the other two rows of
its seven-row table are not fixes — the pasted-`"dd"`-as-a-delete-chord row is dispositioned
**never exposed** (bracketed paste sets `Paste: true`, and the split is guarded by `!msg.Paste`),
and the `phase3e-findings.md` §4a/§4c row points at report items rather than a field defect.

*(b) Fixed BY Phase 3f — the six field defects, issues #5–#10.* Each was filed with a live
reproduction before the phase was cut; each has its own regression test with the revert-and-
reproduce proof in [`phase3f.md`](reports/phase3f.md):

| Issue | What it did | Requirement | Fixing sha(s) |
|---|---|---|---|
| #5 — interactive preview deadlocks on a terminal query | whole TUI wedged, `q`/`Ctrl+C` dead, `SIGTERM` ignored; recovery was `SIGKILL` from another terminal | R68 | `f3c25d5` (drain the vt emulator's reply stream) + `7d060cd` (stop holding `s.mu` across `grid.Write`) |
| #6 — a retained dead pane wedges kill, resume and restart | session permanently unrecoverable from the UI, against `SPEC.md:547` verbatim | R69 | `b8f2513` (collect on sight whatever the row says) + `366dd78` (resume means "has a live pane"; kill removes a corpse) |
| #9 — `pane_exit_status` is never cleared | one crash removed a session from reconciliation forever | R70 | `0745ced` (a resume clears the replaced pane's crash verdict) |
| #8 — an archived session is startable | a live agent hidden behind `/`, hooks orphaned, status frozen, no in-app way back | R71 | `88742b2` (refuse before the launch lease) + `63d4189` (`U` unarchives from the filter results) + `9d43a32` (resolve hooks against every retained row) |
| #10 — `A` kills a live agent on one unconfirmed keystroke | how the operator lost a working session (`A` is `Shift`+`a`, and `a` is attach) | R72 | `10f3970` (confirm dialog) + `4822484` (`u` undo toast), end-to-end round trip `eb2089e` |
| #7 — scrollable overlays only page, via `PgUp`/`PgDn` | ergonomics only | R73 | `2714d1b` (line scroll on arrows and `j`/`k`) + `4edbfc2` (wheel) + `9c2e66a` (help overlay) |

*(c) Added AFTER the PRD by the phase's second approach — two requirements, issue #11.* Neither
belongs to either list above: these are not Phase 3e bug-log items from (a), and not field defects
the phase was cut to fix from (b). Both were authorised by operator steer mid-run, from issue #11,
once approach 01's eleven requirements had landed, and each has its own regression tests and
revert-and-reproduce proofs in [`phase3f.md`](reports/phase3f.md):

| Issue | What it did | Requirement | Fixing sha(s) |
|---|---|---|---|
| #11 — a late hook from a launch deck has already replaced stops the row deck just started | `DECK_SESSION_ID` names the *row*, so a killed launch's `SessionEnd` was indistinguishable from the live pane's | R74 | `a0d4887` (a random per-launch generation minted into `launch_lease_owner` and exported to the pane) + `196e6f4` (`hookrecv` declines a write from a superseded generation, for every event name) |
| #11 — a concluded launch keeps holding its 30-second launch lease | a row that legitimately became `stopped` again inside that window was refused with "starting elsewhere" by this process's own finished launch, against `SPEC.md` §9.3 | R75 | `0a5034d` (`ReleaseLaunchLease` sets `launch_lease_until = 0`, CASed on the acquiring owner, released by `defer` on every exit path; acquisition logic and the §9.3 guard unchanged, and the owner column deliberately kept because it carries R74's discriminator) |

The load-bearing orderings the PRD demanded were honoured and are visible in the log: R68 first
overall, R69 before R70 (same line, `internal/service/reconcile.go:61`), R71 before R72, R63 before
R65 and before the stability run.

*What the phase closed overall, and what it did not.* All six field defects are closed, and the
residual half landed too: R63 (a passive preview fit can never overlap itself, `f7b97fe`), R64 (the
two unsound shell-`starting` waypoints, `677f5a0`), R66 (matrix's seven status tokens quantise to
seven distinct 16-colour slots, `ce8ef91`) and R67 (eleven task-numbered test files and two
duplicate report section titles renamed after what they are, `b848d28` + `300ee86` + `e47cb35`).
With R74 and R75 from list (c) that makes **thirteen requirements met and none FAILED**.
**R65 was published as a FAILED requirement by approach 01 and is re-derived as met at `0a5034d`**
([re-derivation](reports/phase3f.md)): its assertion criteria were already met there (`5071389`
removed the poll-shaped unsoundness and both red directions were demonstrated), but its field
symptom — `received 1 SIGWINCH signals, want exactly 0` at `preview.feature:147` — recurred on that
tree, including in run 9 of its stability deliverable, so the claim the requirement was meant to
support did not hold and nothing was widened, tagged out or retried to hide it. The second approach
fixed that symptom as finding F1, by restructuring the scenario's prefix only — `2b39124`, no product
code and **no expected SIGWINCH count re-baselined**, the two counts byte-identical and merely moved
to `features/preview.feature:171` and `:173` — pinned by a deterministic new assertion that goes red
when the prefix is reverted (`deck client "solo" has been 30 rows tall at some point, want never more
than 9`, `features/preview_test.go:26`,
[`phase3f-028-f1-passive-fit-floor/README.md`](reports/phase3f-028-f1-passive-fit-floor/README.md)),
and the symptom then did not fire once across a fresh ten-run measurement at `0a5034d`
([`phase3f-033-stability10/README.md`](reports/phase3f-033-stability10/README.md)). Two limits are
published with that verdict rather than glossed: ten runs bound a per-run failure rate only loosely,
and the residual product-side mechanism is **not** claimed fixed — `previewFit`'s no-live-pane early
return still spends the row's one coalesced fit (`internal/tui/tui.go:1406-1413`), proved by
instrumented trace *not* to be what the scenario was losing to, and bounding it would move other
scenarios' counts, which this phase forbids. **Two stability rates are published, both real.**
`ci/stability.sh 10` at the final code commit `0a5034d` is **10/10, script exit 0**, no run re-run
and no eleventh run ([logs](reports/phase3f-033-stability10/)); approach 01's **9/10** at `c12c30e`
(exit status 1, 59m21s, no re-run) stays published unedited as the previous tree's history, with its
single failure root-caused to that named early-return race rather than blamed on host load (run 9
started at the *lowest* 1-minute loadavg of the ten). The whole suite is green at both trees:
`ci/run.sh go test -p=1 -count=1 ./...`, `exit 0`, 14 `ok` + 3 `[no test files]` at `0a5034d`
([log](reports/phase3f-032-fullsuite/go-test-p1-count1-all.log)) and 360s / 306 scenarios at
`e47cb35`, `defaultTags` untouched in both.

**Phase 3g** — `prds/phase3g-field-backlog.md`, run `deck-phase3g`, 2026-08-27 to 2026-08-30. Eighteen
requirements, **R76–R93**, the operator's field backlog against the Phase 3f build plus one
mid-run addition, **plus three independent-review findings raised against the six-approach build**
(review at `d266346`): finding 1 — R76's reconcile repair did not reach a bare hook/probe-sourced
`error` row with no `PaneExitStatus`; finding 2 — R80's `A`/`x` actions each had (or risked) a
footer-only eligibility definition diverging from the key handler's own; and finding 3 — task 205's
`SPEC.md` edit (`b69b5ba`) relied on a steering note's own licence to touch a protected section, and
review does not honour that licence. `SPEC.md` was amended twice for this run: first (`2eed8de`,
plan change `a03527c`, both pre-existing operator commits and ancestors of the run's own base
`1cfbd5a`) so R76–R92 each have a spec authority going in, then again mid-run (`b69b5ba`, task 205)
to add R93's own §11.8 "in-progress selection" clause, under an explicit operator licence (steering
018, github.com/n-orlov/deck issue #18) that finding 3 held does not bind this run — only a ruling
present under the run harness's read-only `/config/amendments/` directory does (a path outside this
repository), and the sole such ruling there (`001-202.md`) grants no protected-path exception.
Approach 09's **`2d61993`** (task 909) forward-reverts exactly those
nine §11.8 lines (`b69b5ba` itself stands unrewritten in published history), so R93's shipped
drag-selection behaviour briefly had no SPEC authority in this tree — a gap that did not stand:
`git diff --stat 1cfbd5a..HEAD -- SPEC.md` is **not** empty, and `git log --oneline
1cfbd5a..HEAD -- SPEC.md` lists exactly three touches to that path, newest first: `de90a5c`,
`2d61993` and `b69b5ba` (task 205's original §11.8 amendment). **The operator landed §11.8's R93
wording verbatim at `de90a5c`** (`git log --oneline -1 de90a5c -- SPEC.md` names it), so R93's
shipped drag-selection behaviour **does have SPEC authority in this tree**, as of that commit —
the opposite of what this paragraph used to claim about SPEC authority in this tree. The pre-`de90a5c`
gap is preserved as history, not deleted: at the time, it was specified only by the operator's own
wording, preserved verbatim in tracked
[`docs/reports/phase3g-909-spec-restore/README.md`](reports/phase3g-909-spec-restore/README.md)
and restated as finding **F41**; `de90a5c` predates this run's own base `a24ff8d` (it is `a24ff8d`'s
own parent commit) and is the operator's own act, outside this run's own edits — this run's own
protected-path guard (`git log --oneline a24ff8d..HEAD -- SPEC.md prds/ ci/Dockerfile ci/SPIKE.md`
empty) is untouched by it. **Phase 3h disposition (task 207, final code sha `4b1d4dc`): this
paragraph corrects the two stale claims review named** — the SPEC-authority-gap claim was never
withdrawn once `de90a5c` landed, and the `git diff 1cfbd5a..HEAD -- SPEC.md` empty claim was never
true of the full range, only of the narrower sub-range ending at `2d61993`. R76's reconcile repair itself was narrowed again by approach 09 (tasks
901–906, finding **F40**): it repairs a `stopped` row under a live pane whatever its source, and an
`error` row that carries a pane-exit verdict or a `tmux`-/`user`-sourced verdict, but it deliberately
never repairs a hook- or probe-sourced `error` row with no pane-exit verdict — narrower than
approach 07's `89edd3c`, which had made the repair reach every bare `error` row unconditionally to
close finding 1, before `SPEC.md`'s own precedence rule (`SPEC.md:509,512`) was found to forbid that
reading. Evidence: [`docs/reports/phase3g.md`](reports/phase3g.md) (per-requirement, with
revert-and-reproduce proofs for the eight requirements the PRD named a naive-test trap — R76, R77,
R79, R86, R87, R88, R89, R91 — plus R93's own retroactive proof, and dedicated sections for both
original review findings) and [`docs/reports/phase3g-findings.md`](reports/phase3g-findings.md)
(spec contradictions actually met, what the PRD got wrong, and defects found and deliberately not
fixed). Commits run `1cfbd5a..HEAD`, reaching `a5f8f6b` (task 1001).
**This citation of `a5f8f6b` as Phase 3g's final code sha is superseded — 3g's true final code sha is `fdf4507`**, per the close-out paragraph below.
The docs-only-after-it claim this section used to make does not hold across the full range: `git diff --stat a5f8f6b..HEAD -- '*.go' '*.feature'` is non-empty (tasks 1101–1204 and this run's own tasks 001–207 both touch code after it).
At the time task 1002 ran, the whole suite measured green at that then-believed-final sha: the mandated, unnarrowed
`ci/run.sh go test -p=1 -count=1 ./...` exited **`0`**
([`docs/reports/phase3g-1002-fullsuite/suite.log`](reports/phase3g-1002-fullsuite/suite.log),
exit status in the sibling
[`suite.log.exitstatus`](reports/phase3g-1002-fullsuite/suite.log.exitstatus), task 1002).
**This is, again, a superseded final-code-sha citation of `a5f8f6b` — `fdf4507` is 3g's true final code sha**:
`ci/stability.sh 10` at that same sha measured, verbatim from its own
summary line, **`10/10 passed`**, with the script's own captured exit status **`0`**
([`docs/reports/phase3g-1003-stability10/summary.log`](reports/phase3g-1003-stability10/summary.log),
exit status in [`script.exitstatus`](reports/phase3g-1003-stability10/script.exitstatus), task 1003;
originally closed on citation, no re-run, by task 1004 — both measurements are preserved here as
history and superseded by the true final state recorded below). The phase spent ten approaches, distinguished only
by their task-id range and each using the same `<area>: <why> (task NNN)` commit-subject
convention: 01 (`0NN`, tasks 001–042) landed R76–R81 and R83–R92 outright and R82 for every dialog
but two; 02 (`1NN`, tasks 101–113) closed the rename dialog's focused-field theming (task 105,
`ea6ce4b`, finding F18) and the phase's dialog/footer/report-hygiene residue; 03 (`2NN`, tasks
201–214) closed the three independent-review findings from that review pass, among them the
create-modal PTY-assertion conflict that had blocked task 016 (task 203, finding F27) — with those
two closures R82 is fully met, as is every one of the eighteen requirements; 04 (`3NN`, tasks
301–309) re-synchronised the scenarios review found still racing; 05 (`5NN`, tasks 501–512) fixed
three more races found under load and drove the stability gate toward green;
06 (`6NN`, tasks 601–608) was the reporting tail that closed the two suite-determinism gates by
citation at then-final code sha `b0a4e7d` and wrote that wave's own close-out; 07 (`7NN`, tasks
701–703) closed review finding 1 by making the repair reach a bare hook/probe `error` (task 701,
`89edd3c`), enumerated the nine scenarios that repair put back into play (task 702, `608e030`) and
re-pointed seven of them onto genuine pane exits or a widened poll, documenting the remaining two
rather than weakening them (task 703, `5ea9475` + `2094b83`); 08 (`8NN`, tasks 801–815) records
finding 1's own SPEC §7 contradiction (task 801, F36), re-points three more of finding 1's fallout
scenarios (tasks 803–805) while leaving the two that cannot be re-pointed without re-opening
finding 1 open at that wave's own tree (F38, task 802 `skipped`/unsatisfiable), closes finding 2's
`x` half outright (task 807, `b434079`) and its `A` half functionally but not on every literal
clause (task 806 `failed`, F39), records both findings' closures in the report (task 809) and every
residual this wave found (task 810, F37/F38/F39, plus a corrected F20 restatement), re-measures
both suite-determinism gates at that wave's then-final code sha and finds them regressed rather
than carrying the approach-06 numbers forward (tasks 811/812, both `skipped`/unsatisfiable), and
re-verifies every guard (task 813, `skipped`/unsatisfiable on one impossible clause); 09 (`9NN`,
tasks 901–909) resolved finding 3 by forward-reverting the protected `SPEC.md` edit (task 909,
`2d61993`, F41) and, reading `SPEC.md`'s own precedence rule against its self-heal paragraph,
narrowed the repair finding 1 had widened unconditionally down to the rule F40 records (tasks
901–906) — a narrowing, not a fix, that also makes `status_attach.feature:18` and the other
scenarios approach 08 left open at F38 pass again, closing that regression without re-opening
finding 1; 10 (`10NN`, tasks 1001–1009, this entry among them) is the final wave — it pins by test
that no eligibility predicate refuses a live-pane hook-sourced `error` row (task 1001, `a5f8f6b`),
re-sweeps the whole suite green at that sha (task 1002), re-measures the stability gate 10/10 at
the same sha and closes it on citation (tasks 1003/1004), brings `docs/reports/phase3g.md`'s R76
and R93 records to this state (tasks 1005/1006), re-verifies every guard at the true final sha
(task 1007), rewrites this paragraph (task 1008) and writes the approach's own close-out section
(task 1009).

**The two suite-determinism gates are red at the phase's true final code sha, not green — a direct,
understood consequence of review finding 1's own closure, cited rather than re-run to chase a
number.** Task 811's whole-suite sweep (`nohup ci/run.sh go test -p=1 -count=1 ./...`, verbatim,
unnarrowed) exits **`1`** at `17b1649` (code state `fdf4507`), deterministically, on
`TestFeatures/attach_acknowledges_a_live_error_without_replacing_its_verdict`
(`features/status_attach.feature:18`) — finding F38's own mechanism: R76's repair runs
synchronously inside the same `deck _hook` subprocess that wrote the hook-sourced `error`, before
that subprocess returns, so there is no window in which the scenario can observe the pre-repair row
without weakening its own `!`-marker assertions, and both available fixes (weaken the scenario, or
gate the repair) are independently forbidden by this run's own standing rules. Filed unsatisfiable
rather than fixed (task 811). `ci/stability.sh 10` at the same tree measures, quoted verbatim from
[`phase3g-812-stability10/summary.log`](reports/phase3g-812-stability10/summary.log) at code sha
**`17b1649`** (code state identical to `fdf4507`; `git diff --stat 17b1649..HEAD -- '*.go'
'*.feature' go.mod go.sum` empty):

```
0/10 passed
```

script exit status **`1`** ([`stability08.exitstatus`](reports/phase3g-812-stability10/stability08.exitstatus)),
all ten runs failing on the identical `attach_acknowledges_a_live_error_without_replacing_its_verdict`
mechanism above, with two further non-deterministic flakes also observed and logged:
`status_probe.feature`'s "Stale sampling..." scenario (6/10 runs, same repair-timing mechanism,
probe-sourced) and `attach_scroll.feature`'s wheel-notch scenario (1/10 runs, an unrelated tmux
status-line clock-tick timing artifact). Filed unsatisfiable rather than rounded up or re-run (task
812). Both gates were 10/10 (Go suite exit `0`) green at approach 06's then-final sha `b0a4e7d` —
task 507's round 3, [`phase3g-507-stability10/round3/summary.log`](reports/phase3g-507-stability10/round3/summary.log)
— and the regression is real, caused by making review finding 1's repair correctly unconditional
(F36), not a re-baselining or a rounding error.

**R82 (dialogs are themed) is resolved, not partial.** Task 016's own create-modal theming pass
reported its success criteria unsatisfiable (a pre-existing PTY assertion could not survive full
§11.6 theming), and that unsatisfiability was itself a genuine `SPEC.md`-vs-PRD contradiction
rather than a standing-rule collision — resolved by task 203, per the PRD's own precedence rule
that `SPEC.md` wins (finding
[F27](reports/phase3g-findings.md#3-defects-found-and-deliberately-not-fixed-and-why)). Task 021's
separate residual gap — the rename dialog's focused field never got the `theme.Selection`
background §11.4 requires — was closed by task 105, `ea6ce4b` (finding
[F18](reports/phase3g-findings.md#3-defects-found-and-deliberately-not-fixed-and-why)). The
per-requirement table in `docs/reports/phase3g.md` carries both closures; nothing in the eighteen
requirements is less than fully met.

**Six items are named open, never claimed fixed, exactly as this run's own standing rules
require — plus one item task 810 filed and this same wave already closed.** A genuine, durable
lost update in `internal/service/reconcile.go`'s unconditional shell-liveness promotion, discovered
while fixing a scenario-side synchronisation symptom in `attention_sort.feature` (finding
[F31](reports/phase3g-findings.md#3-defects-found-and-deliberately-not-fixed-and-why), part of the
F29–F32 cluster task 601 filed): diagnosed and measured (36/80 failures under synthetic load, 0 of
them on the count assertion itself) but not fixed, because no requirement in this plan covers that
promotion policy and a product change there would invalidate the (now-superseded) 10/10 gate.
**Phase 3h disposition (task 012): F31 is fixed for real.** Task 007's `2ccb1d3` adds
`AllowedCurrentStatuses: []string{"starting"}` to the `tmux.shell_live` promotion in
`internal/service/reconcile.go` — exactly the candidate fix named above — closing the lost update:
a forced-interleaving regression test pins the pre-fix red at task 006 (`8d6ed72`,
[`docs/reports/phase3h-006-f31/`](reports/phase3h-006-f31/README.md)) and the same test is
green after task 007's guard
([`docs/reports/phase3h-007-f31-guard/`](reports/phase3h-007-f31-guard/README.md)). F2, F20, F22
and F37 below are unaffected by this fix and every one of them stays open in Phase 3h's own record:
they are exactly the out-of-scope, never-claimed-fixed items that phase's standing rules name, and a
recurrence in its gate is reported with its committed log path, never fixed and never hidden here.
F37's entry below needs reading precisely, because the two statements are about different halves of
it and neither is withdrawn: 3g's own task 804 (`46dad5e`) fixed the *scenario* side —
`features/sort_order.feature`'s six raw `error` writes were rerouted through a genuine nonzero pane
exit, out of the repair's reach, so that file no longer races — while the *product* side F37 named,
the unconditional live-pane repair that can win the reconcile tick and reorder a row, is untouched
by task 804 and untouched by task 007's promotion guard above. That product-side mechanism is what
Phase 3h leaves recorded open for F37, alongside F2, F20 and F22; it remains reproducible only under
the tracked forced interleaving (`sh docs/reports/phase3g-810-findings/reproduce-f37.sh`), and Phase
3h claims no fix for any part of it.

Two standing Phase 3f flakes, out of scope by this run's own rules and not reproduced by any tracked
log this phase: `TestGoldenMinimumFrame`'s settle flake, F2
([disposition](reports/phase3g-findings.md#4-f2--the-golden-frame-settle-flake-no-recurrence-found)),
and `internal/interactive`'s `ByteArrivalPattern` connect-budget flake, F22
([row](reports/phase3g-findings.md#3-defects-found-and-deliberately-not-fixed-and-why)). The
`status_recovery.feature` dup-pane scenario's race against R76's own reconcile repair, first
disclosed by task 002's evidence and carried as finding F20
([row](reports/phase3g-findings.md#3-defects-found-and-deliberately-not-fixed-and-why)) — its
original scenario-level symptom was rewritten onto a store read by task 040, and task 810's own
restatement (`17b1649`) stopped that restatement contradicting itself, but the underlying
interaction between R76's self-heal and any scenario that poses a terminal write into a still-live
pane is a standing one, named here rather than declared closed. Task 810 also filed three findings
of its own this wave: [F37](reports/phase3g-findings.md#3-defects-found-and-deliberately-not-fixed-and-why)
(`features/sort_order.feature`'s six raw `error` writes racing F36's own repair — that scenario-side
red **fixed**, task 804, `46dad5e`, with a one-command tracked reproducer of the pre-fix red,
`sh docs/reports/phase3g-810-findings/reproduce-f37.sh`; the product-side repair mechanism the same
finding named stays open, per the Phase 3h disposition above); the two genuinely open ones this
paragraph's own gate section already used above,
[F38](reports/phase3g-findings.md#3-defects-found-and-deliberately-not-fixed-and-why) (a hook-sourced
live-pane `error` has zero observable window for `status_attach.feature`'s scenario shape — not
fixed, and not fixable without re-opening review finding 1) and
[F39](reports/phase3g-findings.md#3-defects-found-and-deliberately-not-fixed-and-why) (task 806's
mutually exclusive "tests unedited" and "rename away from the footer-scoped name" clauses — not
fixed, left as that task's own residual).

**Close-out.** [Task 113's original close-out](reports/phase3g-113-closeout/README.md) covered
approaches 01–02's protected-path audit and citation sweeps; approach 06's own
[close-out section](reports/phase3g.md#close-out-approach-06) (task 607) superseded it against
that wave's own then-final state (`b0a4e7d`, both gates green) and closed this paragraph's earlier
stale version as finding F35 (`bb72d96`/`d266346`). Approach 08's own close-out section, task 815,
supersedes approach 06's in turn against the phase's true final state recorded in this paragraph
above: final code sha `fdf4507`, both review findings' closures, both suite-determinism gates now
red and why, task 813's guard re-verification, and a table of every approach 07–08 task that ended
in a non-`completed` status naming, for each, the sha(s) and tracked evidence directory (or open
finding) that actually delivered its scope — the same shape approach 06's table used for approaches
01–05. This commit (task 814) is written first because task 815's own close-out section needs a
current delivery-log paragraph to point at, not the other way round.

**Phase 3h** — `prds/phase3h-suite-reconciliation.md`, run deck-phase3h, 2026-08-30. Four
requirements, **R94–R97**: three forward-reverts plus R95's one-token cwd-ghost re-point return
the suite to the operator's own §7 ruling landed in `de90a5c` — the transition table is the
design: a stopped row is repaired whatever its source, an error row is repaired only when it
carries a pane-exit verdict or a tmux/user source, and a hook- or probe-sourced error with no
pane-exit verdict is never repaired; R96 fixes F31, the reconcile lost update, for real; R97 is
this phase's own documentation refresh. Final code sha `4b1d4dc` (task 201's own commit, the
gofmt-clean realignment of the promotion write). At that sha both gates measured green: the
mandated, unnarrowed whole-suite sweep exits **0**, all 17 packages pass or report no test files,
nothing failed
([`docs/reports/phase3h-202-fullsuite/README.md`](reports/phase3h-202-fullsuite/README.md));
its verbose companion tallies **311 scenarios (311 passed)**, **3532 steps (3532 passed)**
([`docs/reports/phase3h-203-fullsuite-verbose/README.md`](reports/phase3h-203-fullsuite-verbose/README.md));
and the stability gate's own `docs/reports/phase3h-204-stability10/summary.log` final line reads,
verbatim, **10/10 passed**, script exit **0**, zero recurrence of F2, F20, F22, F37 or the
`features/filter.feature` dd/undo race across all ten runs
([`docs/reports/phase3h-204-stability10/README.md`](reports/phase3h-204-stability10/README.md)).
**The protected-path audit is operator-ruled satisfied** (steering 001, operator amendment
001-211, 2026-08-30): the range `de90a5c..HEAD` over `SPEC.md`, `prds/`, `ci/Dockerfile`,
`ci/SPIKE.md` contains exactly one commit, `a24ff8d` (the operator's own pre-launch commit adding
this phase's PRD, whose omission from the PRD's own legitimate-shas list was the operator's
authoring error, not this run's problem), and `a24ff8d..HEAD` over the same paths is empty — this
run's own writes never touch a protected path. Evidence:
[`docs/reports/phase3h.md`](reports/phase3h.md) (the per-requirement report) and
[`docs/reports/phase3h-findings.md`](reports/phase3h-findings.md) (the ruling, the
false-disposition-prose corrections to both Phase 3g reports, and this phase's own out-of-scope
recurrence check).

**Phase 3i** — `prds/phase3i-force-attach.md`, run `deck-phase3i-2`, 2026-09-02. Six requirements,
**R98–R103**: **F**, the force-attach steal of the interactive preview — the `F` entry path
itself (R98), the one-winner ownership claim over a live holder (R99), a durable window option
carrying the pre-any-deck geometry through an arbitrary chain of steals so the surviving
original geometry is never lost (R100), the lost-attach dialog that tells and silences the
displaced client (R101), the passive-fit stand-down that keeps a second client's mere row
selection from resizing a window it doesn't hold (R102), and the record-matches-the-tree
documentation requirement (R103). **Final code sha is now `3b70bfb`
(`3b70bfbc7e3552ff375ae675af117805a1eee944`), not the `a559e7c` this paragraph used to cite**:
approach 4 landed exactly two code commits, task 401 (`96bff56`) and task 402 (`3b70bfb`
itself), a SPEC-conformance fix in which SPEC §11.3's curated footer fixed-set sentence outranks
PRD R98's footer wording, so `F` comes out of `footerLegend`'s fixed set (and its now-dead
completeness pair) while staying bound, staying in the keymap and staying in the `?` overlay —
`3b70bfb` is the code-frozen sha every gate and document in this phase must now measure
against; the approach-2/3 gates (tasks 127/128/129/204 at `b9243a1`, and tasks 302/303/206 at
`a559e7c`) are stale and disclosed only as superseded, never as a discharge. At `3b70bfb` all
three current gates measured green: the mandated, unnarrowed whole-suite sweep exits **0**, all
17 packages pass or report no test files, nothing failed
([`docs/reports/phase3i-404-fullsuite/README.md`](reports/phase3i-404-fullsuite/README.md));
its verbose companion tallies **319 scenarios (319 passed)**, **3682 steps (3682 passed)**
([`docs/reports/phase3i-406-fullsuite-verbose/README.md`](reports/phase3i-406-fullsuite-verbose/README.md));
and the stability gate's own summary line reads, verbatim, **10/10 passed**, script exit **0**
([`docs/reports/phase3i-405-stability10/README.md`](reports/phase3i-405-stability10/README.md)).
**The PRD's literal protected-path clause is UNMET, not met, and is reported that way
deliberately**: the audit range `6197b53..HEAD` over `SPEC.md`, `prds/`, `ci/Dockerfile`,
`ci/SPIKE.md` necessarily contains one operator commit, `3090b68` (the operator's own commit
adding this phase's PRD, landing after `6197b53` opens the range and before any worker task in
this run could exist to violate the guard); no amendment narrows the range and no history
rewrite may move that commit out of it, so the clause stands UNMET exactly as written — see
[`docs/reports/phase3i-findings.md`](reports/phase3i-findings.md) §2 ("The protected-path audit
range `6197b53..HEAD` necessarily contains one operator commit") for the full disclosure,
including the worker-write range `3090b68..HEAD` over the same paths, which is empty: no worker
commit in this run touched a protected path. Six terminal non-completed tasks from the earlier
approaches are reported honestly rather than presented as discharging any requirement: approach
1's task 119 ("Prove the two displacement flavours tear down differently") ended `failed`
(validation-exhausted) on test strength, not on product behaviour, its residual discharged by
task 136 (`validated`, `fcdb994`, counts ownership-option reads rather than only unsets in the
stolen-claim teardown test), and task 134 ended `skipped`; approach 2's task 204 ("Run and
publish the whole-suite sweep at the new final code sha") ended `failed` (validation-exhausted)
on sweep-polling discipline, superseded first by approach 3's task 302 and now by approach 4's
task 404; and tasks 205 ("Publish the verbose companion sweep's Gherkin tally", its deliverable
superseded by approach 4's task 406), 211 and 212 ended `skipped` when the run moved to approach
3 — 205's work was still open at that moment, so the abandoned approach's own frozen task-state
file (a run-state file, not part of this repo) records the `pending` it held as the move landed,
while this phase's plan record carries all three as `skipped`; neither status discharges anything
here, which is why 406 had to run at `3b70bfb` for the tally above to exist at all.
Evidence: [`docs/reports/phase3i.md`](reports/phase3i.md)
(the per-requirement report, refreshed against `3b70bfb` by task 407) and
[`docs/reports/phase3i-findings.md`](reports/phase3i-findings.md) (task 119's disposition, the
protected-path audit disclosure, both gates' disposition quoted verbatim, the SPEC §11.3 footer
finding (numbered finding 8, task 403), and this phase's own out-of-scope recurrence check).

**Phase 3j** — `prds/phase3j-launch-and-teardown-hooks.md`, run `deck-phase3j`, 2026-09-03. Seven
requirements, **R104–R110**, closing GH issue #20: every pane now carries its own session's
`DECK_SESSION_*` context on both launch paths, including `CreateShell` (R104); a global
`pre_launch` composes global-first with the session's own, fail-closed, on every launch path
including `CreateShell` (R105); the hook's env-mutation contract — reaches the agent, not the
tmux session table, not deck's on-disk state database — is stated and tested (R106); a
global/per-session
`post_destroy` runs session-then-global on `A`/`dd`, fail-open, bounded by a named 30s timeout,
and never on `x` or a reap path (R107); the four editable launch inputs (`pre_launch`,
`post_destroy`, `launch_args`, `login_shell`) are editable on a live row through a
`launch_dirty` flag and `launch↻` badge, restart-to-apply, with `agent`/`cwd`/`slug`/
`captured_path` enforced un-mutable by a source-scanning guard (R108); the hook rules are
stated in user-reachable copy (R109); and the record — this document plus
[`docs/reports/phase3j.md`](reports/phase3j.md) and
[`docs/reports/phase3j-findings.md`](reports/phase3j-findings.md) — closes on the tree (R110).
**Final code sha `4fbd452430501805a860dd229ddca1cd3f5c1cd6`** — the last commit in the phase to
touch a `*.go` or `*.feature` path, task 080's own commit (a comment-only correction to a stale
`CreateShell` `pre_launch` description in `features/launch_hooks.feature`). It supersedes the
earlier `b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7` sha: task 080 changed only a comment inside a
`*.feature` file, but the plan's own Termination rule still counts that as touching a
`*.go`/`*.feature` path, so the final code sha moves past `b29afb8` regardless of the change
being comment-only. `b29afb8` had itself superseded the earlier `a44ee32` sha (task 030's own
create-modal keyboard-walk fix for task 026's Post-destroy field): a second, independent review
raised five blocking findings against the `a44ee32` tree, and the fix commits for the first
three of them (`1a4b9db`, `d71c02f`, `a936b30`, `52e529b`, `2042cb8`, `31e6aff`, `b29afb8`)
advanced the final code sha to `b29afb8`, which then stood as final code sha until task 080's
comment fix superseded it in turn. Three gates were re-run at task 080's sha
`4fbd452430501805a860dd229ddca1cd3f5c1cd6` and their existing report directories refreshed in
place, never renumbered: task 081 refreshed the mandated, unnarrowed whole-suite sweep
(`ci/run.sh go test -p=1 -count=1 ./...`), exit **0**, 14 packages `ok` plus 3 `[no test files]`
([`docs/reports/phase3j-030-fullsuite/README.md`](reports/phase3j-030-fullsuite/README.md));
task 082 refreshed its verbose companion (run only because the non-verbose launcher prints no
Gherkin tally), exit **0**, reporting byte-exact **330 scenarios (330 passed)**, **3824 steps
(3824 passed)**
([`docs/reports/phase3j-031-fullsuite-verbose/README.md`](reports/phase3j-031-fullsuite-verbose/README.md));
and task 083 refreshed the stability gate, whose
[`docs/reports/phase3j-032-stability10/summary.log`](reports/phase3j-032-stability10/summary.log)
ends, verbatim, **`10/10 passed`**,
script exit **`0`**
([`docs/reports/phase3j-032-stability10/README.md`](reports/phase3j-032-stability10/README.md)) —
task 083's own first collection at this same sha (commit `2de1700`) had reported 9/10, rejected
on poll-discipline procedure rather than on the number, with that 9/10 kept on the record as
evidence the suite's tmux/pty timing can flake intermittently rather than as a claim the suite
is flake-free. The protected-path audit and both branch guards were first verified and found empty and
agreeing at the then-current final code sha `b29afb8c4fd8a1cf193c7efef5c5f7e1456481f7`, and
task 092's commit `a98cbb6` (`docs/reports/phase3j-033-guards/README.md`) re-ran that same audit
and guard capture at the current final code sha above, again finding both empty and agreeing
([`docs/reports/phase3j-033-guards/`](reports/phase3j-033-guards/); its own
`final-code-sha.out` reads `4fbd452430501805a860dd229ddca1cd3f5c1cd6`). All five of that second
review's blocking findings are closed on the tree: findings 1–3 (a teardown-hook failure could
not raise a visible toast; the teardown timeout was a mutable `var`, not the required named
constant; R109's user-reachable hook copy was inaccurate and its coverage test missed the
inaccuracy) by `d71c02f`/`a936b30`/`52e529b`, `1a4b9db`, and `2042cb8`/`31e6aff`/`b29afb8`
respectively; findings 4 and 5 (record defects in this phase's own reports, and undemonstrated
gate-polling/one-sweep discipline) by `2dca034`, `807fe0a` and `c32a0c7`; approach 04's own
findings-file record repairs — closing section 1's history and purging backticked run-state
paths from `docs/reports/phase3j-findings.md` — landed in `4351a0e`, `82c498e`, `341798f` and
`bf560cd`. Task 011 ("Plumb `post_destroy` through the store's session write and read paths")
exhausted its validation attempts on one residual gap — `ShellCreateInput` had no
`PostDestroy` field, so a `CreateShell` row could never carry one, unlike a `CreateAgent`
row — but the operator's own ruling `001-011` reopened it as a steer-originated pending task
narrowed to exactly that gap, and it reached `validated` again once commit `9fb6aec` added the
field and threaded it through. Separately, task 038
(`2e5fc6b`, `566cb6d`) closed a §6.1 SPEC-conformance finding (findings §3) alongside its own
scope: routing `CreateShell`'s pane through the same `resolveLaunchEnv` + `buildPaneCommand`
composition the agent paths use, so a shell launch carries the session context and runs a
fail-closed session+global `pre_launch` the same way an agent launch does.
Evidence: [`docs/reports/phase3j.md`](reports/phase3j.md) (the per-requirement table and the GH
issue #20 design-section map) and
[`docs/reports/phase3j-findings.md`](reports/phase3j-findings.md) (task 011's history, three
validation-found gaps closed within their own task, findings §3's SPEC-conformance fix, the
protected-path/schema-version disclosures, and the three re-run gates' dispositions). One residual is left
in the tree deliberately and recorded as advisory in findings §3 rather than fixed: the create
modal's `Env` field and `Login shell` toggle are still not forwarded on the shell create path
(`internal/tui/tui.go`'s `submitCreate` hands `CreateShell` name, cwd and `pre_launch` only) — a
pre-existing UI-seam gap outside R104–R110, not a hook-composition gap, since a shell row's env
is settable after create through the §11.4 env editor and applies on its next launch. R109's
copy coverage is likewise bounded by design to the five hook claims task 028 asserts in
`internal/tui/hook_help_coverage_test.go` (a launch hook runs on every launch and must be
idempotent; a launch hook is fail-closed; a teardown hook is fail-open, runs on `A` and `dd` and
not on `x`; `post_destroy` plus an undo brings the row back stopped; the safe secret shape —
`export K=V` on stdout, diagnostics to stderr, never echo), each asserted present somewhere a
user can reach rather than on one nominated surface.

A final closeout tail added no code and re-ran no gate: it was docs-only from its first commit
to its last, leaving the final code sha `4fbd452430501805a860dd229ddca1cd3f5c1cd6` unmoved. Task
103 (`f01f2f95e5962ffeb1cd5cbbfd0d47f33b14150b`) cured stale present-tense claims across the
three record documents with a systematic marker audit; task 097
(`6acf6cafd48af03cb649f27dcaf70890fd56969f`) cured a findings-file table-of-contents entry that
misnamed the tasks behind its §8 heading; task 098 (`71f6f2f9fc994fc8fcb810cadd7d43e34d52f4ac`)
cured the missing record of why approaches 03–05 were rejected, tracing that rejection to the
review engine's own petition re-file gap, which the operator closed outside the tracked tree;
task 141 (`f0bedf6a3141efa6c8f5dd4244878031da18d6af`) cured nine present-tense clauses left inside
findings §12 by task 098's own commit; and task 142 (`48a172a45853a607a91ebbf7db00b9b922ec4a64`)
cured phase3j.md's R110 row and added its closing subsection, naming the four commits above and
describing, in the past tense, what each one cured.

**Phase 3k** — `prds/phase3k-agent-availability.md`, run `deck-phase3k`, 2026-09-05. Five
requirements, **R111–R115**, closing GH issue #21: each adapter declares the executable its
launch argv starts, and one probe function (`lookPathIn`, moved beside `AvailableKinds` in
`internal/service`) is shared by the create modal's listing and both create's and resume's
preflight, so the two can never disagree about a binary (R111); the create modal's Agent field
cycles only the kinds whose declared executable resolves on the launch `PATH` when `n` opens
(never re-probed from `View()`), `shell` is always in it, a remembered agent that is registered
but unavailable falls back to the default without the "(last used)" label, and the row's help
names the hidden kinds (R112); `CreateAgent` now preflights the executable exactly as `resume.go`
already does — a missing binary is a no-row, no-tmux-session refusal named in-dialog, exempted
when `login_shell` is on — the direct fix for GH #21's `127` crash-row report (R113); the
`features/` harness gained a `pi` fixture step in the shape of the existing `claude` one, and
every scenario that drives the modal to a non-shell kind now installs that kind's fake first —
three files needed a fixture edit (`permission_modes.feature`'s two `pi`-creating scenarios,
`crash.feature`'s failing-`pre_launch` scenario, and `dialogs.feature`'s every-field-reachable
walk, found while chasing the whole-suite sweep green) plus five new scenarios in the new
`features/agent_availability.feature` (R114); and the record — this document plus
[`docs/reports/phase3k.md`](reports/phase3k.md) and
[`docs/reports/phase3k-findings.md`](reports/phase3k-findings.md) — closes on the tree (R115).
**Final code sha `d88c6625c4ccca71b0d31f7b5864ba030ed39e53`** — task 021's own fixture-fix commit
to `features/dialogs.feature`, the last commit in the phase to touch a `*.go` or `*.feature`
path. All gates are green at that sha: the mandated, unnarrowed whole-suite sweep
(`ci/run.sh go test -p=1 -count=1 ./...`) exits **0**, 14 packages `ok` plus 3
`[no test files]`, no skip anywhere
([`docs/reports/phase3k-021-fullsuite/README.md`](reports/phase3k-021-fullsuite/README.md)); its
verbose companion (run only because the non-verbose launcher prints no Gherkin tally) exits
**0**, reporting byte-exact **335 scenarios (335 passed)**, **3888 steps (3888 passed)** — up
from phase 3j's 330/3824, consistent with the 5 new scenarios
([`docs/reports/phase3k-023-fullsuite-verbose/README.md`](reports/phase3k-023-fullsuite-verbose/README.md));
and the stability gate, relaunched from a clean tree and polled with `sleep 120` only across 33
polls after an earlier attempt's `sleep 5` first poll was rejected, ends with
[`docs/reports/phase3k-024-stability10/summary.log`](reports/phase3k-024-stability10/summary.log)
reading, verbatim, **`10/10 passed`**, script exit **`0`**
([`docs/reports/phase3k-024-stability10/README.md`](reports/phase3k-024-stability10/README.md)).
The protected-path audit (`git log --oneline 150d7d6..HEAD -- SPEC.md prds/ ci/Dockerfile
ci/SPIKE.md`, base computed as the commit adding this phase's own PRD) printed **no output** —
no commit in the run touched a protected path
([`docs/reports/phase3k-025-audit/README.md`](reports/phase3k-025-audit/README.md)) — and the
three parity guards (`TestHelpKeymapParity`, `TestFooterBindingsParity`,
`TestFooterHandlerAgreement`) still pass unedited, confirmed with a fresh run plus an empty
`git diff --stat` over their own three test files since the phase baseline
([`docs/reports/phase3k-026-guards/README.md`](reports/phase3k-026-guards/README.md)). No SPEC
disagreement was found while checking R111–R114's landed work against §5, §6.3 and §11/§11.4 —
see [`docs/reports/phase3k-findings.md`](reports/phase3k-findings.md) §3.
[`docs/reports/phase3k.md`](reports/phase3k.md) maps the phase both ways against GH #21's
numbered `## Design` items — item 1–7 → requirement, and each of R111, R112, R113, R114 and R115
→ the numbered items it discharges — so the issue closes on a reading of that pair of tables.
**This citation of `d88c6625c4ccca71b0d31f7b5864ba030ed39e53` as Phase 3k's final code sha is
superseded — 3k's true final code sha is `4e09f2de90dcde04bd8fc20c77097e593f2fee5b`** (task 202's
commit), per an independent review that found two remaining probe gaps in `lookPathIn` after this
paragraph was first written: it accepted a mode-0644 regular file with no execute bit, and it
accepted a non-regular file (a FIFO) named like an agent's binary. Both were cured, docs-only
from `internal/service/availability.go`'s point of view outward — `cure-01-01` (`7349dd6`)
required an executable bit, `201` (`c8b00cc`) rejected non-regular files, and `202` (`4e09f2d`)
added the FIFO regression case to the create preflight, which shares the same probe. All four
gates were re-measured at the new sha and are green: the whole-suite sweep exits **0**, 14
packages `ok` plus 3 `[no test files]`, no skip
([`docs/reports/phase3k-203-fullsuite/README.md`](reports/phase3k-203-fullsuite/README.md), task
203); its verbose companion again reports **335 scenarios (335 passed)**, **3888 steps (3888
passed)** — unchanged, since the cures added no scenario
([`docs/reports/phase3k-204-fullsuite-verbose/README.md`](reports/phase3k-204-fullsuite-verbose/README.md),
task 204); the ten-run stability gate, launched from a clean detached git worktree checked out at
the exact final code sha with the literal inner line `nohup timeout 7200 ci/stability.sh 10 >
log 2>&1 &` and polled with `sleep 120` only, ends **`10/10 passed`**, script exit **`0`**
([`docs/reports/phase3k-cure-02-01-stability10/summary.log`](reports/phase3k-cure-02-01-stability10/summary.log),
commit `0bb7a03`, task cure-02-01 — task 205's own attempt at this same re-run ended terminal
`failed` on launch-form grounds an operator ruling later held were never a rejection basis, so
this record discharges 205's obligation); the protected-path audit again prints nothing
([`docs/reports/phase3k-207-audit/README.md`](reports/phase3k-207-audit/README.md), task 207);
and the three parity guards still pass unedited
([`docs/reports/phase3k-206-guards/README.md`](reports/phase3k-206-guards/README.md), task 206).
The approach-01 measurements at `d88c662` cited above are preserved as history, each now carrying
its own dated supersession note pointing at the record above. [`docs/reports/phase3k.md`](reports/phase3k.md)
and [`docs/reports/phase3k-findings.md`](reports/phase3k-findings.md) are re-closed at the new sha
(task cure-02-02); the R111/R113 rows there additionally name the two cure commits.

**Phase 4** — `prds/phase4-codex-and-chrome.md`, run `deck-phase4`, 2026-09-16. Sixteen
requirements, **R116–R131** (Tier 1 R116–R127 plus Tier 2 R128–R131), closing the run's own
plan-gate objection: Tier 1 covers the codex adapter (argv, hook trust, permission-mode mapping,
resume/create parity) and chrome legibility (per-kind icon glyphs, gutter/status contrast,
grouping-agnostic row layout), each requirement's own commit(s) and test(s) cited in
[`docs/reports/phase4-report.md`](reports/phase4-report.md) (task 042); Tier 2 (session
grouping, settings CRUD, `DECK_SESSION_GROUP`) was decided NOT started —
[`docs/reports/phase4-tier2-decision.md`](reports/phase4-tier2-decision.md) (task 028) records
the budget at decision time (iteration 105: 699/800 iterations remaining, ~15h15m to deadline)
against Tier 2's own materially larger estimate (8–12h for schema migration + sidebar rewrite +
settings CRUD), too tight against the mandatory tail (039–044, including a 75-minute stability
sweep); every task from 029 through 038 reads that decision and is satisfied by its own
first-clause escape rather than landing Tier 2 code, and the resulting SPEC-versus-code grouping
gap is disclosed as an accepted non-finding rather than a defect. **Tail code sha
`db669658ce20de10ef6aaad311c94f6830538436` (`db66965`)** — task 047's commit (`features: fix
stale interactive_focus selection_idle text match (task 047)`), the last commit in the run to
touch a `*.go` or `*.feature` file; the plan's own baseline for the "Tier 2 not started" branch
named task 026, but three pre-existing red lanes the Tier 1 gate sweep (task 027) uncovered were
each carved into their own fixing task (045, 046, 047), and 047's own criterion supersedes 026 as
the run's last code-touching task, per task 028's decision record. Both gates were green or
advisory-only at that sha: the mandated, unnarrowed whole-suite sweep
(`ci/run.sh sh -c 'go test -p=1 -count=1 -timeout=40m ./...'`) exits **0**, all 18 packages listed
by `go list ./...` report `ok` or `[no test files]`, no `FAIL` line, 7m01s wall-clock, default
godog tag filter (`~@real-agents && ~@nightly`)
([`docs/reports/phase4-final-suite/README.md`](reports/phase4-final-suite/README.md), task 039);
the ten-run stability sweep (`ci/stability.sh 10`) measures **7/10 passed**, with all three
failures (runs 4, 8, 9) the same known-open, advisory "transient-`starting`" quiescence-race flake
in `TestGoldenMinimumFrame` (`features/golden_frame_test.go:74`, the golden frame settling with
row status `starting` by design, per that test's own doc comments and task 210's prior
disposition) — no occurrence of the other named flake class (the SIGWINCH exact-count assertion)
and no other package or scenario failing in any of the ten runs
([`docs/reports/phase4-stability10/README.md`](reports/phase4-stability10/README.md), task 041),
advisory per that task's own criteria wording and not a blocker to either gate or to this record.
`go build ./...`, `go vet ./...` and `gofmt -l .` are clean at the same sha but for the four
pre-existing drift files named pre-existing in the plan's own standing rules
([`docs/reports/phase4-guards/README.md`](reports/phase4-guards/README.md), task 040). Evidence:
[`docs/reports/phase4-report.md`](reports/phase4-report.md) (the per-requirement verdict table,
both gates and the guards, all cited at `db66965`) and
[`docs/reports/phase4-findings.md`](reports/phase4-findings.md) (the PRD's two known-unverified
codex items — `acceptEdits`/`plan`/`dontAsk` reachability and hook-trust hash version stability,
both measured only on codex `0.154.0` — plus the run's own findings ledger, one entry per
commit-message `FINDING:` line in `git log --grep='FINDING:' 08a1ffe3..HEAD`, four commits in
all, tasks 025/009/008/007).

**This citation of `db66965` as Phase 4's final code sha is superseded — Phase 4's true final
code sha is `0ba550a5e50bdfc84586d5328a0690af9c9888c4` (`0ba550a`).** Review rejected the
approach that landed at `db66965` on five findings (`review-findings.json`) — B0 (the
reviewer's Python disposable-clone protocol cannot apply to this Go module), B1 (R118's canvas
oracle exempted deck's own preview placeholder/fill/crop-marker cells from the
every-deck-owned-cell claim), B2 (`TranscriptInput.CodexHome` put codex-specific knowledge
inside `internal/tui`, violating the PRD's no-edit-to-add-a-kind constraint), B3 (R127's
`@codex` scenario asserted distinctness, not attribution, and had no two-second timing bound)
and R1 (a wrong `claude.go` rationale plus an undisclosed PRD-vs-SPEC permission-badge
disagreement) — and a second, cure-and-reverify approach (run `deck-phase4`, same PRD,
approach 2) answered exactly those five and no more, per its own standing rule ("a cure, not a
rebuild"): approach 1's Tier 1 work stays as committed and re-verified, not re-implemented.
- **B0** — not curable in code: task 001 supplied `ci/review.sh`, a Go-compatible analogue of
  the reviewer's own protocol (disposably clones the repo, asserts the module-path/`go list`
  identity property, then runs a caller-supplied `go test` target inside the clone) — no Python
  packaging was added to the Go product, and the operator was notified of the protocol and the
  decision it needs (`2786d3c`, task 001). This is also **B0's measurement protocol**, recorded
  in `docs/reports/phase4-review-protocol.md` alongside the identity assertion and a narrow
  smoke run over `./internal/agent/`, not a whole-suite run.
- **B1** — cured: task 002 (`96b0ba9`) gave every preview body line an explicit provenance
  (`previewLineOwner`, deck-owned vs. foreign) and routed only deck-owned lines through
  `canvasBackground`; task 003 (`0e72ec1`) painted the fill columns past a capture and its crop
  marker the same way while leaving the capture's own cells untouched; task 004 (`4614bff`)
  replaced the feature file's blanket exemption with the one true SPEC §11.3 exception (an
  actual pane capture) and asserted the background token over the preview interior for all five
  built-ins plus `NO_COLOR`/`DECK_COLOR_DEPTH=16`.
- **B2** — cured: task 005 (`e69c3d8`) replaced `CodexHome` with a generic
  `Caps.TranscriptEnvKeys []string` an adapter declares and a generic `Env map[string]string`
  the TUI populates from that list — the production transcript-resolution caller resolves only
  the keys an adapter declares this way and carries no `CODEX_HOME`-shaped field or branch of
  its own; task 006 (`ef9571d`) extended the black-box registry-swap guard to a replacement
  adapter's own invented transcript env key; task 007 (`1e97059`) regression-tested the
  production lookup caller with three competing `CODEX_HOME` layers live at once. (Approach 3
  task 007 corrects this entry's own citation: task 006's and task 007's own tests
  [`internal/tui/registry_guard_test.go`, `internal/tui/transcript_env_layers_test.go`] name
  `CODEX_HOME` in comments and fixtures to prove the seam, so `grep -rn 'CODEX_HOME'
  internal/tui` prints 14 intentional test matches rather than nothing — the all-files
  empty-grep phrasing above was never accurate past those two tasks' own commits, though the
  product-level seam they describe is unaffected.)
- **B3** — cured: task 008 (`2ff6024`) captured each pane's own authoritative
  `session_id`/`transcript_path` independently of the store and added
  `TestCodexIdentityMismatchCatchesSwappedStoredIDs` (`features/codex_hooks_swap_test.go`),
  which feeds the comparison two rows' stored ids swapped and asserts it fails on *both* halves
  (id and transcript path), proving the oracle is attribution-sensitive rather than merely
  distinctness-sensitive; task 009 (`98ac4e7`) added the two-second creation-bound and same-cwd
  assertions measured from the store's own columns.
- **R1** — cured: task 010 (`cf2d53b`) corrected `claude.go`'s comments to the true
  `default`→`manual` version-rename rationale (no argv change); task 011 (`e93a790`) went past
  the documentation-only ask and fixed the underlying SPEC-vs-code gap in code — the sidebar row
  now renders no permission badge at all for `safe`, per SPEC.md:1339 — closing the disagreement
  `docs/reports/phase4-findings.md` records under the R120-vs-SPEC-§11 heading.

**Tier 2's fate is unchanged: still NOT STARTED.** Task 012 (`425c9dc`) re-affirmed approach 1's
own decision without narrowing the gap — approach 2's scope was bounded to ten small,
single-purpose cure/regression tasks over already-shipped Tier 1 code, and Tier 2's four
requirements (an all-or-nothing store-schema migration, a sidebar grouping-model replacement, a
create-modal field, a settings CRUD surface) remain the same materially larger body of work
approach 1 already declined under a comparable deadline (633 iterations / ~11h36m remaining at
decision time). The resulting SPEC-versus-code grouping gap (code still groups by workspace, not
by a `groups` table) stays an explicitly disclosed, not-scored non-finding, unchanged from
approach 1 — never "cured" by editing SPEC.

A golden-fixture regression surfaced mid-approach, off review's own five findings, and is cured
in the same record: task 013's first gate sweep at `e93a790` (task 011's tail sha at the time)
found `TestGoldenMinimumFrame` red — task 011 hid the `[safe]` badge correctly but never
regenerated the golden fixture it changed. Task 011b regenerated it (`1f38195`) and re-swept, but
hit a second, independent flake in the same test: the "settled" baseline was taken with a bare
`client.Frame(true)` immediately after the content gate, which can be satisfied mid-repaint,
producing a genuinely torn frame (reproduced directly at 3 of 12 sub-runs). Commit `0ba550a`
took the baseline from `ScreenDriver.WaitForQuiescence` (300ms quiet window > deck's 250ms
`previewTick`) with a bounded retry — 16 of 16 sub-runs green over `-count=8` after the fix —
and became this approach's true, final last-code-touching commit, superseding task 011 and
task 012's own "task 011 is last" note.

**All three sweeps, re-measured from scratch at `0ba550a` (never re-run under the task that
found the red lane, per the standing rules):**
- **Full-suite gate** (task 013, closed by `646c523`): `ci/run.sh sh -c 'go test -p=1 -count=1
  -timeout=40m ./...'` — every package, no `-run` filter, no package list — exits **0**, all 18
  packages (`go list ./...`) report `ok` or `[no test files]`, no `FAIL` line, **7m21s**
  (`docs/reports/phase4-cure-final-suite/README.md`).
- **Build/vet/gofmt guards** (task 014, no commit of its own needed — recorded by task 011b's
  `0ca8667`): `go build ./...` and `go vet ./...` both exit 0 with empty output; `gofmt -l .`
  lists exactly the same four **pre-existing** drift files measured at plan time
  (`internal/theme/quantize_test.go`,
  `.spike-preview/{cmd/conformance/main.go,conformance/conformance.go,conformance/conformance_test.go}`)
  and nothing this approach wrote — **1.3s** wall clock, re-measured on the identical tree
  (`docs/reports/phase4-cure-guards/README.md`).
- **Ten-run stability sweep** (task 015, no commit of its own needed — recorded by the same
  `0ca8667`): `ci/stability.sh 10` — **10/10 PASS, every failure named: none.** `grep -h 'FAIL'
  run-*.log` over all ten committed logs returns nothing; **1h12m24s** total, each run ~6-7
  minutes (`docs/reports/phase4-cure-stability10/README.md`). Neither known-open flake class
  recurred: the golden-frame settle race (fixed at the root above) and
  `TestSigwinchCountDistinguishesTwoFromThree` both occurred zero times in these ten runs. An
  earlier, superseded sweep of the same directory at `e93a790` (011b's first attempt) had
  recorded 7/10 — two runs red on the golden-frame race this approach's own `0ba550a` fix
  resolved, and one run red on a single, non-reproduced
  `TestSendKeysInvalidHexByteIsSilentlyDiscarded` `capture-pane` miss
  (`internal/tmux/literal_send_test.go:123`), disclosed via the `FINDING:` line in commit
  `b98ce9c`'s body and absent from all ten clean runs.

The record itself was rewritten at `0ba550a` rather than left at `db66965`:
[`docs/reports/phase4-report.md`](reports/phase4-report.md) (`bb42aea` + `1ee3cbd`, task 016) —
the per-requirement verdict table naming this approach's cures and commits, R132's own verdict
against its four bullets, and both gates with command/duration/sha —
[`docs/reports/phase4-findings.md`](reports/phase4-findings.md) (`0e514ee`, task 017) — the same
two known-unverified codex items by name, this approach's own `FINDING:` inventory (verbatim
`git log --grep='FINDING:' 08a1ffe3..HEAD`), and the new R120-vs-SPEC-§11 permission-badge
disagreement section (SPEC authoritative, task 011's `e93a790` named as the code fix) — and this
paragraph (task 018). The protected-path audit (`git log --oneline 08a1ffe3..HEAD -- SPEC.md
prds/ ci/Dockerfile ci/SPIKE.md`) prints nothing across both approaches. Approach 1's own record
at `db66965` (`docs/reports/phase4-{final-suite,guards,stability10}/`, and the superseded
version of `phase4-report.md`) stays as history and is not edited.

**This citation of `0ba550a` as Phase 4's final code sha is superseded — approach 3's true final
code sha is `7bb1f8add502412618ebf4f195b18ffd5536b64a` (`7bb1f8a`, "features: wait for codex's
asynchronous first-hook identity adoption before checking (task cure-03-02)"), corrected here
from this entry's own first draft (`6c397ab`, task 009), which was written and pushed before two
facts below existed: cure-03-01/cure-03-02 landed after it, and the operator's B0 ruling arrived
after it. This paragraph replaces that draft in place; it is not a second, competing approach-3
entry.** Approach 3 (run `deck-phase4`, same PRD) was a narrow cure against its own plan-gate
review, which carried forward one still-blocking finding from approach 2 (B1's own same-class
residual: the interactive-preview branch) and left B0's authorization status pending; approach
2's already-cured B2, B3 and R1, and all of Tier 1, were re-verified rather than re-implemented,
per this approach's own standing rule ("a cure, not a rebuild"). Review pass 234, taken after
task 002 landed, found two further reds not itself in scope for tasks 001/002 — the settings
takeover's own footer left unpainted, and the real-Codex first-hook wait rejecting before the
asynchronous identity adoption it waits on — cured by tasks cure-03-01 and cure-03-02
respectively; both are code, and both moved the tail code sha from `3568bd7` to `7bb1f8a`.

- **B1** — cured: task 001 (`92619cf` + `48bce3d`) gave `cropPreviewBottomLeft`'s own geometry
  line and synthesized blank-fill rows their own per-row provenance
  (`previewLineDeckOwned`/`previewLineForeign` via `previewContentLine` and
  `fullBoxPreviewContentLine`), so they paint `theme.Background` instead of leaking the
  terminal's own background — new test `TestCropDecorationsCarryDeckBackground`
  (`internal/tui/crop_decoration_background_test.go`). Task 002 (`3568bd7`) cured the same class
  of gap in the interactive-preview branch, found at this approach's own plan time and not
  itself in review's B1 finding text: `interactiveBodyLines`'s deck-composed notice/pad rows were
  blanket-marked foreign alongside the live capture they surround. Gave `interactiveBodyLines`
  the same per-row provenance via `fitInteractiveBodyLines(lines, contentHeight, notice)` — new
  tests `TestFitInteractiveBodyLinesOwnership` (`internal/tui/interactive_test.go`) and
  `TestInteractiveNotRepaintedNoticeCarriesDeckBackground`
  (`internal/tui/interactive_notice_background_test.go`). Review pass 234 then found the same
  class of gap a third time in the settings takeover's own footer — cured by task cure-03-01
  (`2a04e5a`, "paint the settings takeover footer through the shared canvas helper"), which
  routes `settingsFooterLineContent` through the same `canvasBackground` wrapper the other two
  cures use, so every deck-drawn cell in every settings mode's footer paints `theme.Background`.
- **B0** — **ADJUDICATED AND WITHDRAWN by operator ruling.** This entry's own first draft
  (`6c397ab`, task 009) reproduced task 006's pre-ruling report snapshot, which had read the
  authorization as not yet on record; that pre-ruling reading is corrected in place here, not
  standing alongside this one. The operator recorded a wave-scoped ruling into
  `/run/ralphd/steering` at `2026-09-17T07:59:01Z` (delivered as
  `001-b0-authorized-use-ci-review-sh.md`), filed at `/config/amendments/001-WAVE.md` and
  timestamped `2026-09-17T07:57:41Z`, which the steering hat applied at `2026-09-17T09:28:47Z`.
  The ruling withdraws B0 as a blocking finding and authorizes
  `ci/review.sh` (commit `2786d3c`, approach 2 task 001, documented at
  `docs/reports/phase4-review-protocol.md`) as the Go-compatible replacement for the review
  prompt's Python disposable-clone/import-identity bootstrap — its `go list -m` ==
  `github.com/n-orlov/deck` plus clone-resident package-directory assertion is exactly as
  trustworthy as the prompt's own check, and measurements taken in that clone after it passes ARE
  valid evidence. Task 006 (`4542e96`) rewrote `docs/reports/phase4-report.md`'s own B0 section to
  this same disposition; this entry now states no status different from that report section. B0
  is not curable by any task in this Go repository's own code or tests and needed none: the gap
  was never a product defect, only a missing operator authorization, now granted and on record.
  No sentence anywhere in this file says the authorization is missing, unresolved or awaited.
- **R2 citation correction** (task 007, `1326945`; sha refreshed by task 007's own follow-on fix
  `8ae503c`): approach 2 task 005's own commit message asserted `grep -rn 'CODEX_HOME'
  internal/tui` printed nothing across the whole tree. That all-files claim stopped holding once
  approach 2's own tasks 006/007 landed `internal/tui/registry_guard_test.go` and
  `internal/tui/transcript_env_layers_test.go`, each naming `CODEX_HOME` in comments/fixtures to
  prove the transcript-env seam is agent-neutral (14 matches, both files test-only, none in a
  production caller). Corrected both this file's own B2 bullet above and `phase4-report.md`'s R121
  section to the accurate property: the production transcript-resolution caller resolves only an
  adapter's declared `Caps.TranscriptEnvKeys` and carries no `CODEX_HOME`-shaped field or branch
  of its own — the seam itself is unaffected, only the all-files empty-result phrasing of the
  evidence was ever wrong. `8ae503c` then fixed the same bullet's own stale tail-sha citation
  (`panel_background_themes.log`, still naming the superseded `3568bd7` after task 005's rewrite
  moved this approach's tail to `7bb1f8a`).

**Tier 2's fate is unchanged: still NOT STARTED.** No task in this approach touched Tier 2 code;
the SPEC-describes-manual-groups-while-code-groups-by-workspace gap stays the same explicitly
disclosed, not-scored non-finding approach 1 and approach 2 both left it as, never "cured" by
editing SPEC.

**Both sweeps, re-measured from scratch at `7bb1f8a` (never re-run under the task that found a
red lane — none was found; this supersedes the `3568bd7` measurements this entry's first draft
cited, which were themselves measurements of a tree the two footer/first-hook cures then
changed):**
- **Full-suite gate** (task 003, `abd963f`): `ci/run.sh go test -p=1 -count=1 -timeout=40m ./...`
  — every package, no `-run` filter, no package list — exits **0**, all 18 packages (`go list
  ./...`) report `ok` or `[no test files]`, no `FAIL` line, wall-clock **≈7m6s (~426s)**
  (`docs/reports/phase4-a3-final-suite/README.md`). Build/vet/gofmt guards recorded in the same
  commit: `go build ./...` and `go vet ./...` both exit 0 with empty output; `gofmt -l .` lists
  exactly the same pre-existing drift files measured at plan time and nothing this approach wrote.
- **Ten-run stability sweep** (task 004, `c069c34`): `ci/stability.sh 10` — **10/10 passed**,
  every `go test` exit status 0, no `FAIL` line in any of the ten per-run logs, **≈1h11m41s
  (~1h12m) end to end** (`docs/reports/phase4-a3-stability10/README.md`). Neither known-open
  advisory flake (`TestSigwinchCountDistinguishesTwoFromThree`; `internal/tmux`'s
  `TestSendKeysInvalidHexByteIsSilentlyDiscarded` empty-capture case) manifested in any of the
  ten runs.

The record itself was rewritten at `7bb1f8a`:
[`docs/reports/phase4-report.md`](reports/phase4-report.md) (`daea2fd` task 005 + `4542e96` task
006 + `8ae503c` task 007) — the per-requirement verdict table and both sweep citations re-taken at
the new tail sha, the B1/B0 review-findings section above (B1 now including the settings-footer
cure, B0 now the ruling-based disposition), and the R2 sha-refresh — and
[`docs/reports/phase4-findings.md`](reports/phase4-findings.md) (`8d2f8f7`, task 008) — the same
findings inventory refreshed at this tail sha, with the two crop findings (`0e72ec1`'s
geometry/blank-fill scope and `96b0ba9`'s cropRow fill/marker scope) split into their own entries.
The protected-path audit (`git log --oneline 08a1ffe3..HEAD -- SPEC.md prds/ ci/Dockerfile
ci/SPIKE.md`) prints nothing for this approach. Approach 2's own record at `0ba550a`
(`docs/reports/phase4-cure-{final-suite,guards,stability10}/`, and the superseded version of
`phase4-report.md`) stays as history and is not edited.

## Other milestones

| Date | What |
|---|---|
| 2026-08-17 | Repo created and published as `n-orlov/deck`; initial commit is the product spec |
| 2026-08-17 | `SPEC.md` v2: TUI-only, four agents, no daemon, pluggable notifications, BDD/black-box testability as a requirement |
| 2026-08-17 | Spec reviewed adversarially by a second model. Four of its "factual" findings were rejected against verified CLI/docs evidence; the rest were applied — debounce dropped (it required a daemon that the design forbids), `remain-on-exit failed` adopted so crash tails are capturable at all, Codex id discovery made serialised and claim-based, the three conflicting state machines reconciled into one, and dedupe given an epoch so a recurring prompt can't be muted forever |
| 2026-08-18 | `SPEC.md` §11 rewritten around a session sidebar beside a live preview, with layout modes and their breakpoints (§11.2), panel chrome and visible focus (§11.3), a single dialog contract (§11.4), a settings takeover generated from the config schema (§11.5 + new §6.5), and a semantic theme system with a 16-colour-quantised floor (§11.6). Informed by reading `agent-of-empires/agent-of-empires` — its `DESIGN.md` and `src/tui/responsive.rs`, which documents every breakpoint with a "below this it stops working" reason. Adopted: the three-mode layout, the single panel seam, rounded borders and padding, the settings takeover, and the theme-as-TOML model. Not adopted: the web dashboard (an explicit deck non-goal), the command palette, sounds and plugins. Landed as new **Phase 2b**, after status truth and before lifecycle polish |
| 2026-08-18 | Spec + plan adversarially reviewed by a second model after the §11 rewrite (9 blocking findings, 15 advisories), all resolved with the operator: §11.2's 80-column rationale contradicted its own mode table (resolved: side-by-side is the mode at every supported width, stacked is below-minimum degradation, widths are total cells, golden frame = 35/45 at 80×24); the §7 shell-liveness rule breaks three pinned Phase 1 lease assertions (now enumerated in Phase 2's plan entry so the job updates the right side); §7 had a stuck-`waiting` hole — answering a permission prompt fires no hook and `waiting` was never probe-eligible (resolved: probe-eligible after `stale_after` + attach clears `waiting → running`); §11.5's "every key" was unsatisfiable for `[notify]` tables (resolved: flat keys only, structured tables link to their Phase 5 dialog); theme contrast was unassertable (resolved: declared xterm reference palette, WCAG ≥ 3:1 over both palettes, `DECK_COLOR_DEPTH` knob); the §11.4 contract outlawed §5's mandatory `y` yolo confirm (resolved: declared per-dialog keys carve-out); §8.2's process-wide Codex mutex was unsound under R4's N-process model (resolved: store-backed CAS lease); rename/event-log/`Y`/`i`-detail had no owning phase (assigned: 3/3/2/2b-retrofit); `layout_mode`/`sidebar_width` persistence undecided (resolved: `state.db`, `auto` in the `|` cycle, `<`/`>` width keys, config.toml has exactly one writer). Also fixed: the glyph rule restated as no-EAW-Wide (even `●` and box-drawing are Ambiguous — no glyph set satisfies "single-width everywhere"), `killed_by_user` cleared on resume, `last_cwd`/captures schema home, hook budget stated as uncontended, harness resize/SGR prerequisites named, §13.3 synced. Operator decisions: Phase 2 and 2b stay whole (not split) with ~250 iterations / 12 approaches budgeted up front |
| 2026-08-19 | `SPEC.md` currency pass, and the rule that makes it repeatable: the spec now states, at the top, that it describes the product in the present tense and **never narrates its own revisions** — `git log`, `docs/PLAN.md` and this file are the only records of change, and a sentence that needs history or a phase number to parse is a defect there. Applied throughout: phase numbers and retrospectives stripped from §4, §7, §9.4, §11.3, §11.4, §11.5, §11.6, §11.7, §13.1 and §13.2 (the facts they carried live in `docs/PLAN.md`, which already had them). Real defects found and fixed while sweeping: the non-goals banned a "theme engine" that §11.6 then specified (reconciled — themes are colour-only data, an *engine* is still out); §9.4 referenced a `last_cwd` column that §4's schema never declared; §8's `Adapter` snippet had drifted from the built interface (`Launch(s Session) (argv, assignedID, err)` vs the real `Launch(LaunchInput) (argv, err)`) and pointed at §11 for search instead of §12; §11.4 listed a rename dialog no key or entry point reached, an R7 hole (now an action inside the `i` detail dialog, in both the keymap and the plan); §11.5 claimed "seventeen categories", a number borrowed from the reference tool and never true of deck; §13.3's feature layout named `permissions.feature` where the suite has `permission_modes.feature` and omitted every harness/foundation feature that exists; §13.4's "two more" scenarios were three; §2's tree omitted `internal/service`, `internal/config` and `internal/audit` and pointed at a `testdata/` that fake agents don't live in. The §7 state-machine ASCII diagram was replaced by an exhaustive transition table — its rails were a column out of alignment, it drew `error` twice (as a box and as a floating label), and its exit-to-`stopped` arrow pointed at the `waiting` box — and a second representation of the machine is a second thing to drift. Also added: the launch-lease "starting elsewhere" vs "not leasable" distinction stated as a spec rule rather than left in the plan, and the keymap/capability cross-check as an explicit obligation. Verified against the tree, not assumed: every `DECK_*` knob in §13.1 exists in the code, and the three that exist but aren't specced (`DECK_GODOG_TAGS`, `DECK_TEST_ENV`, `DECK_TMUX_ATTACH_*`) are confined to test files — no test-only path in the product |
| 2026-08-19 | Phase 2 PRD cut (`prds/phase2-status-truth.md`, 40 numbered requirements), and the two decisions it was blocked on settled. **§14.9 — dead-pane retention — resolved as collect-on-sight:** the reconcile pass that observes `pane_dead` captures the tail, writes `error` + `pane_exit_status` + `crash_tail`, then kills the session. Retention was rejected because it would keep two answers to "what did it print" (a bounded tail in the store, a full frozen scrollback on the socket), hold the session name against the next resume, and leave crashed sessions on the socket indefinitely; collection is idempotent and unleased (`WHERE pane_exit_status IS NULL`, kill-session a no-op when already gone) so N clients need no lease, and the unattended gap it leaves is the one §7 already states. §14 is now empty of blockers for planned work. **T3 was assigned to two phases at once** — its `waiting`-is-truthful half needs Phase 2, its dedupe-at-the-sink half needs Phase 5 — so `docs/PLAN.md` now splits it explicitly, gives Phase 2 the `notify_epoch` counter that makes Phase 5's dedupe key possible, and forbids either phase from claiming the scenario whole; Phase 2 writes the notification steps *absent* rather than stubbed. Also fixed: §4 did not declare `permission_profile_reason`, a column Phase 1 shipped and the detail view reads (found by diffing the spec's DDL against `schemaV1` in `internal/store/store.go`), and `crash_tail` had no bound — now stated as the last 200 lines, plain text, sanitised at capture, since it is rendered inside deck's own chrome. Recorded while writing the PRD: `internal/tmux` already returns `pane_dead`/`pane_dead_status`/`pane_current_path`, so the plan's "the reconcile gains `list-panes -F`" was one revision stale — only `service.Reconcile` ignores those facts, and `capture-pane` is the genuinely missing piece |
| 2026-08-19 | Phase 2 PRD reviewed adversarially by a second model (Fable) against the spec, the plan, both prior phase reports and the tree; it verified all six of the PRD's claims about existing code as true and returned three blocking findings, all confirmed and fixed. **(1) Harness prerequisites the PRD assumed and Phase 0 never built** — the fake agents cannot fire §8.1 events or render fixture text (only `FAKE_CLAUDE_EXIT_CODE` exists), the SIGKILL step kills a *deck client* rather than an agent process, and `DECK_CLOCK_STEP` advances the frozen clock only after a successful shell creation, per-process, so no scenario can make a row `stale_after` old. §13.1 promises "advances it on demand"; the tree does not deliver it, which left requirements 16–18 and 20 unwritable. Now requirements 41–44, explicitly first. Compounding trap found in the same sweep: `internal/store` falls back to real `time.Now()` when a writer passes no `At`, so one un-threaded write mixes real 2026 time into a store frozen at 2025 and every staleness comparison becomes nonsense while looking like a probe bug. **(2) A pinned Phase 1 assertion the PRD's own prescribed grep could not find** — `features/concurrency.feature:21` asserts a *shell* row is `starting` in the store, with no `awaiting signal` copy anywhere near it, so shell-liveness promotion breaks it invisibly. The table now covers it plus the two helpView sites, and the sweep instruction changed from "grep for the suffix" to "find shell rows asserted to be in `starting` by any means". **(3) §7's second crash-detection path was owned by nobody** — §3 says liveness is reconciled "lazily by `_hook`" and §7 says the transition to `error` happens on "the next TUI tick **or** the next `_hook` invocation", but the PRD gave dead-pane collection to the reconcile tick alone and no later phase claimed the rest, so unattended crash detection would never have been built at all. Now requirement 45, and §3.1's `_hook` contract states the liveness pass (non-session-end path only, bounded, never probing — probing there would falsify §10.3's second limitation). Five advisories also applied: the 20 ms budget cannot be measured from the log's existing durations, since `Clock.Elapsed()` is time since **process start**, so a naive assertion measures the whole `_hook` lifetime including Go start-up and SQLite open; the session-end "enqueues nothing" assertion is a *pinned absence Phase 5 must flip*, now recorded as such in both documents; requirement 18's "probe correctly ignored" direction could pass green against a probe engine that never fired, so it now demands evidence the probe ran and lost; and one cross-reference pointed at the wrong requirement number |
| 2026-08-17 | "Toolchain in a sibling" upstreamed into ralphd itself (`n-orlov/ralphd`, commit a5a18d2) as prompt-level guidance, docs, a mountable skill and 6 tests, so any future job gets the capability without a PRD explaining it |
