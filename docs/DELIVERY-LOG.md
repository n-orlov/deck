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

## Other milestones

| Date | What |
|---|---|
| 2026-08-17 | Repo created and published as `n-orlov/deck`; initial commit is the product spec |
| 2026-08-17 | `SPEC.md` v2: TUI-only, four agents, no daemon, pluggable notifications, BDD/black-box testability as a requirement |
| 2026-08-17 | Spec reviewed adversarially by a second model. Four of its "factual" findings were rejected against verified CLI/docs evidence; the rest were applied — debounce dropped (it required a daemon that the design forbids), `remain-on-exit failed` adopted so crash tails are capturable at all, Codex id discovery made serialised and claim-based, the three conflicting state machines reconciled into one, and dedupe given an epoch so a recurring prompt can't be muted forever |
| 2026-08-17 | "Toolchain in a sibling" upstreamed into ralphd itself (`n-orlov/ralphd` `a5a18d2`) as prompt-level guidance, docs, a mountable skill and 6 tests, so any future job gets the capability without a PRD explaining it |
