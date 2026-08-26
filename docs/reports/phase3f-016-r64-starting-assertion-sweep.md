# Task 016 / R64 — the full classification of every `starting` assertion in `features/`

**This mechanism is specified behaviour, not a product defect, and no product code changed for
R64.** `SPEC.md` §7's shell-only fast-forward promotes a `shell` row from `starting` to `running`
the moment its pane is alive (`internal/service/reconcile.go:92`'s
`session.Agent == "shell" && session.Status == "starting"`), and the harness's reconcile tick is
250ms (`scenarioReconcileInterval`). For a shell session `starting` is therefore a state the
product is *allowed to pass through without ever rendering*: under host contention the row can go
from absent straight to `running`. `git diff --name-only` for this change shows only two
`features/*.feature` files and this report — nothing under `internal/`.

## Method: session + agent, never the string alone

The sweep discriminators are the two the PRD names, and `docs/DELIVERY-LOG.md:412`'s lesson ("find
shell rows asserted to be in `starting` by *any means*", not "grep for the string"):

1. **The session's agent.** A `claude`/`pi` row gets no fast-forward — it stays `starting` until an
   agent signal or probe arrives — so `starting` is a real, observable, *durable-until-signal*
   state for it and asserting it is sound.
2. **Where the assertion reads.** The fast-forward is a *status transition*; the store really does
   hold `starting` briefly, so a **store**-reading step is sound even for a shell row. Only steps
   reading a **frame** (`screen contains`, `row … contains`, `text … has foreground token`) are in
   scope.

So the sweep enumerated (a) every line in `features/*.feature` containing the string `starting`,
(b) every scenario that pins a row to the `starting` *attention rank* without the string (the
order tables), and resolved each one's session name back to the step that created it to get its
agent.

## The enumeration (line numbers at this commit)

### A. Status assertions naming `starting`

| file:line | session | agent | reads | sound? | action |
|---|---|---|---|---|---|
| `agent_session.feature:26` | `claude one` | claude | frame | sound — no fast-forward for an agent | untouched |
| `agent_session.feature:86` | `audit env one` | claude | frame | sound | untouched |
| `create_session.feature:182` | `cs-pre-launch-ok` | claude | frame | sound | untouched |
| `durable_identity.feature:37` | `beta` | claude | frame | sound | untouched |
| `lease_race.feature:15` | `unsignalled agent` | claude | **store** | sound on both axes | untouched |
| `lease_race.feature:18` | `race target` | claude | frame (`row … contains`) | sound | untouched |
| `lease_race.feature:19` | `race target` | claude | frame | sound | untouched |
| `lease_race.feature:20` | `race target` | claude | frame | sound | untouched |
| `permission_modes.feature:98` | `confirmed` | claude | frame | sound | untouched |
| `permission_modes.feature:121` | `sticky` | claude | frame | sound | untouched |
| `same_directory.feature:18` | `one` | claude | frame | sound | untouched |
| `shell_liveness.feature:18` | `unsignalled` | claude | frame (`still contains "starting - awaiting signal"`) | sound — and it is the thing under test | untouched |
| `shell_liveness.feature:20` | `unsignalled` | claude | **store** | sound | untouched |
| `status_probe.feature:47` | `raced claude` | claude | **store** | sound | untouched |
| `status_probe.feature:49` | `sampled pi` | pi | **store** | sound | untouched |
| `status_theme.feature:52` | `tok-agent` | claude | store *write* (setup, not an assertion) | n/a | untouched |
| `status_theme.feature:53` | `tok-agent` | claude | frame | sound | untouched |
| `status_theme.feature:54` | `tok-agent` | claude | frame (per-cell token) | sound | untouched |
| `status_user_kill.feature:17` | `terminal kill` | claude | frame | sound | untouched |
| `status_user_kill.feature:18` | `terminal kill` | claude | **store** | sound | untouched |
| `themes.feature:31-32` (Examples row `themes.feature:40`) | `agent` | claude | frame + per-cell token | sound | untouched |
| **`create_cwd_ghost.feature:26`** | `cwd-ghost-right-session` | **shell** | **frame** | **UNSOUND** | replaced by `has session … selected` |
| **`create_cwd_ghost.feature:42`** | `cwd-ghost-end-session` | **shell** | **frame** | **UNSOUND** | replaced (same class) |
| **`create_cwd_ghost.feature:74`** | `cwd-ghost-hidden-session` | **shell** | **frame** | **UNSOUND** | replaced (same class) |
| **`create_cwd_ghost.feature:91`** | `cwd-ghost-tilde-session` | **shell** | **frame** | **UNSOUND** | replaced (same class) |
| **`create_cwd_tab.feature:42`** | `tab-list-session` | **shell** | **frame** | **UNSOUND** | replaced (same class) |

The PRD named two lines; applying both discriminators to the whole class found **five**, all with
the identical shape (create modal with no agent named → `shell`, submit, frame-read `starting` used
only as a waypoint before a store-read `cwd` assertion). Fixing two and leaving three would have
left the same mechanism live in three scenarios.

### B. `starting` asserted without the string (attention rank)

| file:line | session | agent | reads | sound? | action |
|---|---|---|---|---|---|
| `attention_sort.feature:14` scenario's order table | `s-agent` holds the `starting` rank | claude | frame (row order) | sound — the shell rows in the same table are pinned by direct status writes or are expected to be `running` | untouched |
| `new_session_selection.feature:20`, `:32` order tables | `r52-new`, `r52-oneshot-new` | shell | frame (row order) | sound — the anchor is forced to `waiting` (rank 0), so the new row renders at index 1 whether it reads `starting` (rank 3) or `running` (rank 2) | untouched; the comment at `:14` explaining the placement via `starting` is stale but the assertion does not depend on it |
| `concurrency.feature:21` | `after crash` | shell | **store** | sound (store-reading); see note | untouched |

**`concurrency.feature:21` note (a PRD/tree disagreement, reported not classified):** the PRD calls
it a pinned Phase 1 **store-reading `starting`** assertion that must not be touched — a claim that
traces to `docs/DELIVERY-LOG.md:412`, which recorded it while it was still true. At this commit
there is **no `starting` assertion anywhere in `concurrency.feature`**: the assertion that stood at
line 21, `the state database contains session "after crash" with status "starting"`, was re-aimed to
the durable-row form `the state database contains session "after crash"` in `bd7b9a2` ("Re-aim shell
promotion assertions (R30)"), a phase earlier; line 21 today is the first line of that commit's
explanatory comment and the re-aimed assertion is `concurrency.feature:23`. Either way the line is
store-reading, sound, and untouched by this change.

### C. Occurrences of the string that are not status assertions

| file:line(s) | what it is | action |
|---|---|---|
| `harness.feature:74`, `:320`, `:356`, `:394`, `themes.feature:99` | `starting = "#000000"` — theme TOML token definitions | untouched |
| `harness.feature:168` (`budget note target`, shell) | the **resumeNote** toast `"starting elsewhere"` from the launch-lease refusal path — not a §7 status word; the row stays `stopped` | untouched |
| `launch_lease.feature:15` (`live lease target`, shell) | same resumeNote toast | untouched |
| `launch_lease.feature:20`, `:32`, `:46`, `:57` | negative assertions on the same toast | untouched |
| `lease_race.feature:17` | same toast, across three racing clients | untouched |
| `status_probe.feature:14` | fixture filename `claude/starting.txt` | untouched |
| `attention_sort.feature:4`, `:14`; `harness.feature:161`; `new_session_selection.feature:14`; `shell_liveness.feature:4`, `:14`; `status_probe.feature:19`; `status_theme.feature:4`, `:51` | prose, feature descriptions and scenario titles | untouched |
| `environment.feature:96` | the tag `@requirement-023-…-without-restarting` (substring only) | untouched |

## What replaced the five waypoints

Each `Then deck client "A" screen contains "starting"` became
`Then deck client "A" has session "<that session>" selected` — an existing step
(`features/attention_sort_test.go:125`, `clientHasSessionSelected`) that polls the frame for the
sidebar's `"> " + name` selection marker with the same ordinary 5s frame wait. It is a wait on a
**durable observable consequence**, not on a transient:

- the create modal's view *fully replaces* the sidebar, so the marker cannot appear until the modal
  has closed and the row has rendered;
- the row is rendered from the store, so the marker's presence proves the session row exists before
  the following `cwd` read (which reads the store exactly once, with no wait of its own);
- requirement 52 makes the freshly created row the selected one, and it stays selected for the rest
  of each scenario (only `exits cleanly` follows) — nothing can take it away again.

No `sleep`, no widened timeout, no `@flaky`, no scenario removed, no `cwd` assertion changed, and no
line-number drift (each replacement is a 1:1 line substitution, so `create_cwd_ghost.feature:26` and
`:91` still name the replaced steps).

**Incidental finding about the assertion that was removed.** In the captured failure frame
(`artifacts/task016-r64-positive-control-mutated-name.log`) the 100-column sidebar truncates the
status word of these long-named rows to `sta...`/`run...`
(`| > cwd-ghost-right-session run... |`), so `screen contains "starting"` was never matching the
row's status word at all: it matched the **preview placeholder** `Session is starting; no pane yet.`
— itself a transient that disappears the moment the pane exists. The waypoint was unsound twice
over. This is also why a replacement of the form `row … contains "running"` was rejected: the status
word is not rendered in full at this width.

## Evidence

Every run below used `ci/run.sh` (sibling toolchain container); `/proc/loadavg` 1-min was 1.6–3.4
throughout (`artifacts/task016-r64-loadavg.txt`). Every `artifacts/…` path below is relative to this
run's artifacts directory (`~/.ralphd/runs/deck-phase3f/artifacts` on the host), the same place the
other phase 3f tasks' evidence lives.

- **Targeted run, exactly as the task's success criteria spell it** —
  `ci/run.sh env DECK_GODOG_TAGS='@requirement-14-ghost-unique-match-right-accepts,@requirement-14-ghost-tilde-home-accepts' go test -count=1 ./features/`
  — green **3/3**: `artifacts/task016-r64-literal-criteria-tags-run{1,2,3}.log`.
  Note the second tag as written matches nothing in the tree (godog matches tags exactly; the
  tilde scenario's real tag is `@requirement-14-ghost-tilde-expands`), so that command exercises
  only the right-accepts scenario. It is recorded for the criteria, and the run below is the
  non-vacuous one.
- **All five changed scenarios, real tags** —
  `DECK_GODOG_TAGS='@requirement-14-ghost-unique-match-right-accepts,@requirement-14-ghost-end-accepts,@requirement-14-ghost-hidden-only-with-dot-segment,@requirement-14-ghost-tilde-expands,@requirement-16-tab-lists-candidates-and-selects'`
  with `-run TestFeatures` — green **3/3**:
  `artifacts/task016-r64-five-tags-testfeatures-run{1,2,3}.log`. The same tag set over the whole
  package went ok/ok on two of three runs; the third failed in `TestGoldenMinimumFrame`, an
  unrelated test — see the finding below (`artifacts/task016-r64-real-tags-run{1,2,3}.log`).
- **Positive control (would it go red?)** — mutating line 26's session name to
  `cwd-ghost-right-session-NOPE` fails with
  `timed out waiting for frame "> cwd-ghost-right-session-NOPE"`:
  `artifacts/task016-r64-positive-control-mutated-name.log`. The new assertion is not vacuous.
- **The wait is load-bearing** — deleting line 26 entirely (leaving submit → store read) fails
  **3/3**, twice with `no session named "cwd-ghost-right-session" in the state database` and once
  with a hung-client teardown: `artifacts/task016-r64-nowait-control-run{1,2,3}.log`. Removing the
  waypoint without replacing the wait would have traded one flake for a worse one.
- Workspace restored after every control run (`git diff --stat` showed only the five intended
  lines).

## Finding for the phase report: `TestGoldenMinimumFrame` "not settled" recurs

`features/golden_frame_test.go`'s `TestGoldenMinimumFrame` failed once here
(`artifacts/task016-r64-real-tags-run1.log`) and once in ten isolated runs
(`artifacts/task016-goldenframe-count10.log`), with the same message
`frame kept changing after the fixture rendered; not settled` that
`docs/reports/phase3d-210-golden-frame-settle-race.md` root-caused and fixed for a *different*
signature (a `41x37` → `41x21` preview-panel shrink). This recurrence's before/after frames are
identical except the footer line, which differs by one trailing column
(`… - Y acknowl` vs `… - Y acknow`), so the residual race is in the footer/status line, not the
preview geometry. It is **independent of R64**: under `-run TestGoldenMinimumFrame` godog's
`TestFeatures` never executes, so neither edited `.feature` file is read; ten isolated runs at the
pre-change tip were 10/10 green (`artifacts/task016-goldenframe-count10-at-HEAD.log`), which is a
single sample of a low-rate flake, not an attribution. Tasks 021/022 should expect it in the
whole-suite and stability populations, and task 024 should carry it as a finding.
