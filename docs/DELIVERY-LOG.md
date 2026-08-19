# deck — delivery log

Append-only record of phases actually run. `docs/PLAN.md` is the intended order;
`SPEC.md` is the product spec.

Each phase is one PRD run as one autonomous job. The **PRD blob** column is the git hash of
the exact PRD file contents used for that run — PRDs get edited between phases, so the path
alone is not an identification. Recover any past version with `git cat-file -p <blob>`.

## Phases

| # | Phase | PRD | PRD blob | Run ID | Started | Completed | Engine verdict | Verified |
|---|---|---|---|---|---|---|---|---|
| — | Toolchain spike | `prds/spike-sibling-toolchain.md` | `7ae5e05` | `deck-spike-sibling` | 2026-08-17 14:41 | 2026-08-17 14:54 | `failed / unverified` — iteration budget (10) exhausted at the audit task | **pass**, operator-verified: 4/4 tests re-run independently, ownership clean, both mount directions proven |
| 0 | Harness & walking skeleton | `prds/phase0-harness-and-skeleton.md` | `40af336` | `deck-phase0` | 2026-08-17 18:09 | 2026-08-17 21:21 | `failed / unverified` — review rejected approach 3 (last of 3) on R22 report defects | **substantially pass, with two defects carried to Phase 0b**: all 23 requirements implemented, suite green twice uncached (operator-run), but the suite is flaky (~1 run in 3) and the evidence report is deficient |
| 0b | Harness determinism & evidence | `prds/phase0b-harness-hardening.md` | `9b23161` | `deck-phase0b` | 2026-08-17 21:41 | 2026-08-17 22:33 | `failed / unverified` — review rejected all 3 approaches on stale wording in *derived* report text | **pass**, operator-verified: flake eliminated (10/10 consecutive full-suite runs), hold knob gone, evidence persisted in-repo, count convention correct. Three stale sentences fixed by the operator by hand |
| 1 | Durable identity & agents | `prds/phase1-durable-identity-and-agents.md` | `a1951f8` (in-run snapshot `322a7e0`, see notes) | `deck-phase1` | 2026-08-18 14:50 | 2026-08-18 16:50 | `succeeded / verified` on approach 6 of 12, iteration 121/250 — review rejected approach 5 on a real unmet R1 | **pass**, operator-verified: 10/10 consecutive full-suite runs; R29 walkthrough executed against a **real** `claude` and the conversation provably survived the reboot stand-in. One blocking regression (create-modal default agent) was found by the operator *after* the review passed it |

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
| 2026-08-17 | "Toolchain in a sibling" upstreamed into ralphd itself (`n-orlov/ralphd` `a5a18d2`) as prompt-level guidance, docs, a mountable skill and 6 tests, so any future job gets the capability without a PRD explaining it |
