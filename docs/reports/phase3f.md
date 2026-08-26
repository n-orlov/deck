# Phase 3f report

Phase 3f closed eleven requirements — **R63–R73** — against
`prds/phase3f-residuals-and-suite-determinism.md`: the *residual* half (R63–R67,
suite determinism and report hygiene) and the *field* half (R68–R73, the
operator's GitHub bug log, issues #5–#10). This report gives, per requirement,
the implementing sha(s), the tests and scenarios added, real command output, and
— for each of the nine requirements the PRD names a naive test for — the
revert-and-reproduce proof that the test actually goes red without the fix.

Written incrementally, one section per write, by task 023.

- Sections: [orderings](#load-bearing-orderings-honoured) ·
  [R63](#r63-a-passive-preview-fit-can-never-overlap-itself-task-015) ·
  [R64](#r64-the-two-unsound-shell-starting-waypoints-task-016) ·
  [R65](#r65-settle-before-reading-the-sigwinch-counter-task-017) ·
  [R66](#r66-matrixs-seven-status-tokens-quantise-distinctly-task-018) ·
  [R67](#r67-report-and-test-file-names-say-what-they-are-tasks-019-020) ·
  [R68](#r68-a-terminal-query-can-never-stall-an-interactive-session-tasks-001-002) ·
  [R69](#r69-a-retained-dead-pane-is-collected-on-sight-tasks-003-004) ·
  [R70](#r70-a-resume-clears-the-replaced-panes-crash-verdict-task-005) ·
  [R71](#r71-archived-rows-refuse-launches-and-can-be-unarchived-tasks-006-008) ·
  [R72](#r72-a-confirms-before-it-archives-and-u-undoes-it-tasks-009-011) ·
  [R73](#r73-the-three-scrollable-overlays-scroll-by-line-and-by-wheel-tasks-012-014) ·
  [naive-test traps](#the-nine-naive-test-traps-revert-and-reproduce) ·
  [suite](#green-whole-suite-run-at-the-final-code-commit-task-021) ·
  [stability](#stability-the-real-rate-is-910-task-022) ·
  [table](#per-requirement-table)

## Tool versions and wall clock

Every command in this report ran in the sibling toolchain container (`ci/run.sh`,
image `deck-ci:local`, `--user 1000:1000`, Go module/build caches on the named
volume `deck-go-cache`). No Go and no tmux exist outside it.

```
$ ci/run.sh sh -c 'go version; tmux -V'
go version go1.25.13 linux/amd64
tmux 3.5a
```

Also on file at [`docs/reports/toolchain-versions.md`](toolchain-versions.md).

Wall clock, measured in this phase and unchanged as citations:

| command | wall clock | where |
|---|---|---|
| `ci/run.sh go test -p=1 -count=1 ./...` (whole suite) | **360s** at `e47cb35`/`c12c30e` | [`phase3f-021-fullsuite/`](phase3f-021-fullsuite/) |
| `ci/stability.sh 10` (ten whole-suite runs) | **59m21s** at `c12c30e` | [`phase3f-022-stability10/`](phase3f-022-stability10/) |
| per-package `ci/run.sh go test -count=1 ./internal/<pkg>/` | ≤20s (`internal/tmux` 19s, `internal/interactive` 11s) | this phase throughout |
| tag-scoped scenario run `DECK_GODOG_TAGS=...` | ~4–20s | this phase throughout |
| `features` package alone (all default-tag scenarios) | 297–329s | tasks 016/017/021 |

Host: 28 cores; `/proc/loadavg` was recorded for every stability, isolation and
timing claim (host load is a known confound on this box —
[`phase3f-022-stability10/loadavg-trace.log`](phase3f-022-stability10/loadavg-trace.log)
holds 725 five-second samples across the stability run).

## Load-bearing orderings, honoured

The PRD makes four orderings load-bearing. All four were honoured, and each is
visible directly in `git log --reverse 60c2c56..HEAD` (the phase's whole range,
from the PRD commit to the tip):

```
$ git log --reverse --format='%h %ad %s' --date=format:'%H:%M' 60c2c56..HEAD
f3c25d5 13:52 interactive: drain the vt emulator's reply stream so a terminal query cannot stall a Session (001)
7d060cd 14:25 interactive: stop holding the session lock across a grid write (task 002)
b8f2513 14:35 service: collect a retained dead pane on sight, whatever the row says (task 003)
366dd78 14:50 service: make the resume decision mean 'has a live pane' and let kill remove a corpse (task 004)
0745ced 15:02 store: clear the replaced pane's crash verdict when a resume relaunches (task 005)
88742b2 15:11 service: refuse to resume an archived row before the launch lease (task 006)
63d4189 15:29 tui: unarchive an archived row with U from the filter results (task 007)
9d43a32 15:39 hookrecv: resolve hooks against every retained row, not the sidebar query (task 008)
10f3970 15:51 tui: confirm before A archives, so a live agent is never killed by one keystroke (task 009)
4822484 16:04 tui: offer u to unarchive right after a confirmed A (task 010)
eb2089e 16:18 features: prove the whole A/U/r round trip in one scenario (task 011)
2714d1b 16:28 tui: scroll the three overlays by one line with the arrows and j/k (task 012)
4edbfc2 16:36 tui: let the wheel scroll the three scrollable overlays (task 013)
9c2e66a 16:55 tui: name the overlay scroll bindings in the help overlay (task 014)
f7b97fe 17:14 tui: never let a passive preview fit overlap itself for the same session (task 015)
677f5a0 17:35 features: stop waiting on a shell status the spec lets the product skip (task 016)
5071389 18:10 features: settle before reading the SIGWINCH counter, and keep it exact (task 017)
202e1ba 18:20 features: let a targeted run select feature files by path (task 017)
ce8ef91 18:38 theme: give matrix's seven statuses seven distinct 16-colour slots (task 018)
b848d28 18:53 test: name the eleven task-numbered test files after what they test (task 019)
300ee86 18:54 docs: record the git-rename evidence task 019's report cites (task 019)
e47cb35 19:01 docs: title the two duplicate report sections after what they found (task 020)
c12c30e 19:15 docs: publish the green whole-suite run at the phase's final code commit (task 021)
1fe37de 20:29 docs: publish the real 9/10 stability rate at the final code commit (task 022)
```

1. **R68 first of the field half — and first overall.** `f3c25d5` (13:52) and
   `7d060cd` (14:25) are the first two commits in the range, ahead of R69's
   `b8f2513` and of every other requirement's commits. Nothing in the range
   precedes them.
2. **R63 before R65, and before the stability run.** R63 is `f7b97fe` (17:14);
   R65 is `5071389` (18:10) — 56 minutes later — and the stability run is
   `1fe37de`/`c12c30e` (19:15–20:29), last in the range. So the preview-fit
   overlap guard was in the tree for both.
3. **R69 before R70.** Both touch the same line, `internal/service/reconcile.go:61`.
   R69's legs are `b8f2513` (14:35) and `366dd78` (14:50); R70 is `0745ced`
   (15:02), after both.
4. **R71 before R72.** R71's three legs are `88742b2` (15:11), `63d4189` (15:29)
   and `9d43a32` (15:39) — R72 (`10f3970` 15:51, `4822484` 16:04) needs
   `Store.UnarchiveSession` from R71 leg 2 to exist for its `u` undo, and lands
   after all three. The round trip `eb2089e` (16:18) comes last of the two
   requirements, as it must.

Two deviations from "one commit per task" are recorded rather than hidden: **task
017** produced two commits (`5071389` the fix, `202e1ba` the
`DECK_GODOG_PATHS` selector the fix's own evidence needed) and **task 019** two
(`b848d28` the eleven renames, `300ee86` the rename evidence its report cites).

## R63: a passive preview fit can never overlap itself (task 015)

**Implementing sha: `f7b97fe`** (`tui: never let a passive preview fit overlap
itself for the same session`).

Defect: `previewFitSessionID` was the only coalescer for steer-018's passive
preview fit, and it is written only when an attempt *completes*
(`previewFitDone`), while `previewTick` fires every `DECK_PREVIEW_MS` (300ms)
regardless of what is still running. Two ticks landing inside one
`PreviewPane`+`FitWindowToPane` round trip both scheduled a fit **for the same
session** — two `resize-window` calls on one window, hence an extra SIGWINCH the
"exactly N" counting scenarios never asked for.

Fix: a new `previewFitInFlight` field set at **scheduling** time inside
`previewFit` (not at completion) and cleared unconditionally by
`previewFitDone`; `previewFit`'s guard consults both halves, so at most one
passive fit is ever outstanding. `exitInteractive` deliberately does *not* clear
it (documented in `interactive.go`): clearing `previewFitSessionID` must not
license a second overlapping fit for a session whose first attempt is still out.

Tests — `internal/tui/preview_fit_overlap_test.go`, three tests driving
`Model.Update` directly (no pty, no tmux, no socket) and asserting on the
`tea.Cmd` Update returns, so the fit closure never runs:
`TestPreviewFitDoesNotOverlapItself`,
`TestPreviewFitResumesAfterItsDoneLands`,
`TestPreviewFitDoneForUnselectedSessionClearsTheMarker` (the wedge case: the fit
reports for a session the user has already navigated away from).

```
$ ci/run.sh go test -count=1 ./internal/tui/
ok  	github.com/n-orlov/deck/internal/tui	0.844s
$ ci/run.sh env DECK_GODOG_TAGS='@preview,@requirement-18' go test -count=1 -run TestFeatures ./features/
ok  	github.com/n-orlov/deck/features	27.761s
```
[`phase3f-evidence/task015-r63-tui-package.log`](phase3f-evidence/task015-r63-tui-package.log),
[`phase3f-evidence/task015-r63-features-exacttags-green.log`](phase3f-evidence/task015-r63-features-exacttags-green.log).

**Revert-and-reproduce** (trap: a test that only asserts the coalescer's field is
set would pass on the old code). The `previewFitInFlight` half of the guard was
reverted in a scratch copy, the tests kept:

```
--- FAIL: TestPreviewFitDoesNotOverlapItself (0.00s)
    preview_fit_overlap_test.go:96: second previewTick with the first fit still in flight returned 2 commands, want 1 (the reschedule alone): a second overlapping fit resizes the same window again and costs an extra SIGWINCH
FAIL	github.com/n-orlov/deck/internal/tui	0.003s
```
[`phase3f-evidence/task015-r63-reverted-guard.log`](phase3f-evidence/task015-r63-reverted-guard.log);
full write-up [`phase3f-evidence/task015-r63-preview-fit-overlap.md`](phase3f-evidence/task015-r63-preview-fit-overlap.md).

## R64: the two unsound shell-`starting` waypoints (task 016)

**Implementing sha: `677f5a0`** (`features: stop waiting on a shell status the spec
lets the product skip`). **No product code changed** — that commit touches two
`features/*.feature` files and a report; nothing under `internal/`. The mechanism
is specified behaviour: `SPEC.md` §7's shell-only fast-forward promotes a `shell`
row from `starting` to `running` the moment its pane is alive
(`internal/service/reconcile.go:92`), and the harness reconcile tick is 250ms, so
for a shell session `starting` is a state the product may pass through **without
ever rendering it**.

The sweep used the two discriminators the PRD names, never the string alone:
(1) the session's **agent** — a `claude`/`pi` row gets no fast-forward, so
`starting` is durable-until-signal and asserting it is sound; (2) **where the
assertion reads** — the store really does hold `starting` briefly, so a
store-reading step is sound even for a shell row; only frame-reading steps are in
scope.

### R64 classification table — every `starting` status assertion in `features/`

(Line numbers as at `677f5a0`. Full sweep, including the attention-rank and
non-assertion classes: [`phase3f-016-r64-starting-assertion-sweep.md`](phase3f-016-r64-starting-assertion-sweep.md).)

| file:line | session | agent | reads | sound? | action |
|---|---|---|---|---|---|
| `agent_session.feature:26` | `claude one` | claude | frame | sound (no fast-forward for an agent) | untouched |
| `agent_session.feature:86` | `audit env one` | claude | frame | sound | untouched |
| `create_session.feature:182` | `cs-pre-launch-ok` | claude | frame | sound | untouched |
| `durable_identity.feature:37` | `beta` | claude | frame | sound | untouched |
| `lease_race.feature:15` | `unsignalled agent` | claude | store | sound on both axes | untouched |
| `lease_race.feature:18,19,20` | `race target` | claude | frame | sound | untouched |
| `permission_modes.feature:98` | `confirmed` | claude | frame | sound | untouched |
| `permission_modes.feature:121` | `sticky` | claude | frame | sound | untouched |
| `same_directory.feature:18` | `one` | claude | frame | sound | untouched |
| `shell_liveness.feature:18` | `unsignalled` | claude | frame | sound — and it is the thing under test | untouched |
| `shell_liveness.feature:20` | `unsignalled` | claude | store | sound | untouched |
| `status_probe.feature:47` | `raced claude` | claude | store | sound | untouched |
| `status_probe.feature:49` | `sampled pi` | pi | store | sound | untouched |
| `status_theme.feature:52` | `tok-agent` | claude | store *write* (setup) | n/a | untouched |
| `status_theme.feature:53,54` | `tok-agent` | claude | frame (incl. per-cell token) | sound | untouched |
| `status_user_kill.feature:17` | `terminal kill` | claude | frame | sound | untouched |
| `status_user_kill.feature:18` | `terminal kill` | claude | store | sound | untouched |
| `themes.feature:31-32` (Examples `:40`) | `agent` | claude | frame + per-cell token | sound | untouched |
| **`create_cwd_ghost.feature:26`** | `cwd-ghost-right-session` | **shell** | **frame** | **UNSOUND** | replaced by `has session … selected` |
| **`create_cwd_ghost.feature:42`** | `cwd-ghost-end-session` | **shell** | **frame** | **UNSOUND** | replaced (same class) |
| **`create_cwd_ghost.feature:74`** | `cwd-ghost-hidden-session` | **shell** | **frame** | **UNSOUND** | replaced (same class) |
| **`create_cwd_ghost.feature:91`** | `cwd-ghost-tilde-session` | **shell** | **frame** | **UNSOUND** | replaced (same class) |
| **`create_cwd_tab.feature:42`** | `tab-list-session` | **shell** | **frame** | **UNSOUND** | replaced (same class) |
| `attention_sort.feature:14` order table | `s-agent` holds the `starting` rank | claude | frame (row order) | sound | untouched |
| `new_session_selection.feature:20,:32` order tables | `r52-new`, `r52-oneshot-new` | shell | frame (row order) | sound — the anchor is forced `waiting` (rank 0), so the new row renders at index 1 either way | untouched |
| `concurrency.feature:21` (→ `:23`) | `after crash` | shell | store | sound; **PRD/tree disagreement, reported not classified** | untouched |
| theme TOML tokens, `resumeNote` toasts, fixture filenames, prose (`harness.feature:74,320,356,394`; `themes.feature:99`; `launch_lease.feature:15,20,32,46,57`; `lease_race.feature:17`; `status_probe.feature:14`; `environment.feature:96`; …) | — | — | not status assertions | n/a | untouched |

The PRD named **two** lines; applying both discriminators to the whole class
found **five**, all with the identical shape (create modal with no agent named →
`shell`, submit, frame-read `starting` used only as a waypoint before a
store-read `cwd` assertion). Fixing two would have left the mechanism live in
three scenarios. `concurrency.feature:21` is a PRD/tree disagreement recorded as
a finding rather than classified: the `starting` assertion the PRD pins there was
re-aimed a phase earlier in `bd7b9a2`, and today's `:23` is store-reading, sound
and untouched.

Each waypoint became `Then deck client "A" has session "<name>" selected` — the
existing `clientHasSessionSelected` step (`features/attention_sort_test.go:125`),
which polls for the sidebar's `"> " + name` marker with the ordinary 5s frame
wait. It waits on a **durable observable consequence**: the create modal fully
replaces the sidebar, so the marker cannot appear before the modal closed and the
row rendered from the store, and requirement 52 keeps that row selected for the
rest of the scenario. No `sleep`, no widened timeout, no scenario removed, no
`cwd` assertion changed, and 1:1 line substitution, so no line-number drift.

**Revert-and-reproduce** (trap: "drop the waypoint" and "replace it with
something that always matches" both look green):

- *Positive control* — mutating line 26's session name to
  `cwd-ghost-right-session-NOPE` fails with `timed out waiting for frame
  "> cwd-ghost-right-session-NOPE"`
  ([`phase3f-evidence/task016-r64-positive-control-mutated-name.log`](phase3f-evidence/task016-r64-positive-control-mutated-name.log)).
  The new assertion is not vacuous.
- *The wait is load-bearing* — deleting line 26 entirely (submit → store read)
  fails **3/3**, twice with `no session named "cwd-ghost-right-session" in the
  state database`, once with a hung-client teardown
  ([`task016-r64-nowait-control-run{1,2,3}.log`](phase3f-evidence/)). Removing the
  waypoint without replacing the wait trades one flake for a worse one.
- *Green* — all five changed scenarios by real tag, 3/3
  ([`task016-r64-five-tags-testfeatures-run{1,2,3}.log`](phase3f-evidence/)); the
  criteria's literal tag pair also 3/3
  ([`task016-r64-literal-criteria-tags-run{1,2,3}.log`](phase3f-evidence/)), with
  the caveat recorded there that one of those two tags matches nothing in the tree
  (godog matches tags exactly) — which is why the real-tag run is the
  non-vacuous one. `/proc/loadavg` 1.6–3.4 throughout
  ([`task016-r64-loadavg.txt`](phase3f-evidence/task016-r64-loadavg.txt)).

Incidental finding: at 100 columns the sidebar truncates these long rows' status
word to `sta...`/`run...`, so `screen contains "starting"` was never matching the
row's status at all — it matched the preview placeholder `Session is starting; no
pane yet.`, itself a transient. The waypoint was unsound twice over, and that is
why a `row … contains "running"` replacement was rejected.

## R65: settle before reading the SIGWINCH counter (task 017)

**Implementing shas: `5071389`** (`features: settle before reading the SIGWINCH
counter, and keep it exact`) **and `202e1ba`** (`features: let a targeted run
select feature files by path` — the selector this requirement's own targeted
evidence needed; the second commit is the recorded deviation from one-commit-per-task).
No product code changed in either.

Defect: `Then the fake "<agent>" agent received exactly N SIGWINCH signals`
(`features/fake_agent_size_test.go`) read the counter through
`waitForSigwinchCount`, which returned the instant it first saw `want` **or more**.
So `exactly 1` behaved as *at least 1* (a second signal still in flight was never
noticed) and `exactly 0` did no waiting at all (0 is the initial value — a bare
sample of an asynchronous counter).

Fix: the step now **settles, then reads once, then compares for equality** —
`h.settleClients(ctx, captureSettledQuietWindow)` waits until every still-running
pty client this scenario started has produced no new output for 400ms, through the
**existing** `ScreenDriver.WaitForQuiescence` (`features/pty_driver_test.go:530`,
already used by `features/mouse_synthesis_test.go:226`) and the **existing**
`captureSettledQuietWindow` constant. No second waiter, no new number, no `sleep`,
no widened timeout: the passive fit's `resize-window` convergence changes the
preview panel's content, so waiting for that PTY output to stop waits on the
observable consequence. `waitForSigwinchCount` survives, unused by any step, as
the *arrival* wait for `features/sigwinch_count_test.go`.

There is one step definition, so all seven assertion sites moved together and
**no expected count changed**: `preview.feature:95` (1), `:107` (1), `:130` (0),
`:147` (0), `:149` (1), `interactive_sigwinch_budget.feature:33` (2), `:46` (2).

**Revert-and-reproduce** (trap: a settle can be made permissive, so it must still
go red in *both* directions — an extra signal and an unexpected one). Both
controls mutated **product** code with the new settle already in place, then
restored it:

```
# +1 extra paced resizeWindow in internal/tmux/geometry.go FitWindowToPane
      6 step error: fake "claude" agent received 2 SIGWINCH signals, want exactly 1
--- FAIL: TestFeatures (11.50s)
# previewFit's config gate dropped + clamp instead of refuse below the floor
      6 step error: fake "claude" agent received 1 SIGWINCH signals, want exactly 0
--- FAIL: TestFeatures (11.63s)
```
[`phase3f-evidence/task017-r65-red-extra-sigwinch.log`](phase3f-evidence/task017-r65-red-extra-sigwinch.log)
(loadavg `2.50 1.92 2.11`),
[`phase3f-evidence/task017-r65-red-zero-sigwinch.log`](phase3f-evidence/task017-r65-red-zero-sigwinch.log)
(loadavg `1.67 1.83 2.07`).

Green, targeted at the two files that carry all seven sites (only reachable at all
because of `202e1ba`: the budget feature's scenarios carry no tags), three
consecutive runs with rising load (3.62 → 7.13), 15 scenarios / 150 steps each,
and the seven counts read `1 1 0 0 1 2 2` in every run:

```
$ ci/run.sh env DECK_GODOG_PATHS='preview.feature,interactive_sigwinch_budget.feature' \
    go test -count=1 -run TestFeatures ./features/
ok  	github.com/n-orlov/deck/features	18.847s
```
[`task017-r65-targeted-two-files-run{1,2,3}.log`](phase3f-evidence/) and
[`…-summary.txt`](phase3f-evidence/task017-r65-targeted-two-files-summary.txt).

**R65 is nevertheless recorded as a FAILED requirement.** Its assertion criteria
are met, but its field-symptom claim is not: three whole-suite runs on this tree
went 2 green / 1 red, run 3 failing `preview.feature:134` at `:147` with
`received 1 SIGWINCH signals, want exactly 0` — the same site and the same message
R65's own source cited as the symptom to remove
([`task017-r65-full-suite-run3-RED-trimmed.log`](phase3f-evidence/task017-r65-full-suite-run3-RED-trimmed.log),
[`…-summary.txt`](phase3f-evidence/task017-r65-full-suite-summary.txt)) — and the
same failure then took run 9 of the ten-run stability deliverable (see
[stability](#stability-the-real-rate-is-910-task-022)). Root cause, written up in
[`phase3f-017-r65-settled-sigwinch-count.md`](phase3f-017-r65-settled-sigwinch-count.md)
"FINDING": `:134` creates `beacon` at 100x30 where a passive fit is licensed, and
`previewFit`'s no-live-pane early return still emits `previewFitDone`, spending
the row's one coalesced fit on a failed attempt — so the scenario observes 0 only
when it loses that race. The settle did not cause it (the same scenario flaked
the same way before, [`task015-r63-features-exacttags-flake.log`](phase3f-evidence/task015-r63-features-exacttags-flake.log));
it raises its detection rate, which is what R65 exists to do. Fixing it means
restructuring the scenario's prefix, **never re-baselining its counts**, and it is
carried forward as an open item, not silently absorbed here.

## R66: matrix's seven status tokens quantise distinctly (task 018)

**Implementing sha: `ce8ef91`** (`theme: give matrix's seven statuses seven distinct
16-colour slots`).

Defect (from `docs/reports/phase3e-findings.md` §4b): five of `matrix`'s tokens
collapsed onto ANSI 8 `#7f7f7f` under `quantize()`, so `idle`, `stopped` and
`archived` painted the **same** colour on a 16-colour terminal. The defect was
collision, not legibility — every one of them already cleared the 3:1 floor.

Fix: three authored colours in `internal/theme/builtin/matrix.toml`, nothing else.

| token | authored before → after | quantised before → after | ANSI slot |
|---|---|---|---|
| `idle` | `#33cc66` → `#22cc55` | `#7f7f7f` → **`#00cd00`** | 8 → **2** |
| `stopped` | `#889988` → `#aaccaa` | `#7f7f7f` → **`#e5e5e5`** | 8 → **7** |
| `archived` | `#66aa77` → `#778877` | `#7f7f7f` (unchanged) | 8 → 8 |

`archived` deliberately keeps ANSI 8 — with `idle` and `stopped` gone the slot is
no longer shared among the seven statuses, and "bright black" is the dimmest
legible slot an archived row should have; its authored hex still moved greener →
greyer so the true-colour appearance matches the mapping. Resulting seven-way
mapping: `waiting #ffff33→11`, `running #00ff66→10`, `idle #22cc55→2`,
`starting #33aaff→6`, `stopped #aaccaa→7`, `error #ff3333→9`,
`archived #778877→8`. Contrast recomputed over both palettes and both
backgrounds — lowest margin `archived/surface` **4.53:1**, still well clear of 3:1.

Scope held exactly where the standing rules put it: `matrix.toml`'s three tokens
plus **two** entries of `TestBuiltinQuantizationPinned`'s `"matrix"` map (`Idle`,
`Stopped`, each with a comment naming R66); `cobalt`, `daylight`, `empire`,
`parchment` untouched, and no golden or `NO_COLOR` frame moves (colour data only).
`hint`/`badge` were **not** moved — they are not §7 statuses, so R66 does not reach
them, and the consequence is stated plainly rather than hidden: at 16 colours
`hint`, `badge` and `archived` all render ANSI 8.

New test: `internal/theme/matrix_status_quantization_test.go`,
`TestMatrixStatusTokensQuantiseToSevenDistinctReferenceEntries`, computing
distinctness over `QuantizedColor()` — never `Color()`.

```
$ ci/run.sh go test -p=1 -count=1 ./internal/theme/ ./internal/tui/ ./features/
ok  	github.com/n-orlov/deck/internal/theme	0.004s
ok  	github.com/n-orlov/deck/internal/tui	0.858s
ok  	github.com/n-orlov/deck/features	297.291s
```
[`phase3f-evidence/task018-r66-three-packages.log`](phase3f-evidence/task018-r66-three-packages.log).

**Revert-and-reproduce**, and the PRD's named trap demonstrated on the same tree.
With only the three authored hexes put back:

```
$ ci/run.sh go test -count=1 -run TestMatrixStatusTokens ./internal/theme/
    #7f7f7f: archived, idle, stopped
    ... quantise onto 5 distinct ReferencePalette entries, want 7
FAIL	github.com/n-orlov/deck/internal/theme
```
([`task018-r66-revert-RED.log`](phase3f-evidence/task018-r66-revert-RED.log)) — while the
**true-colour** test `TestMatrixStatusTokensRenderAsSevenDistinctColours` still
*passes* on that defective tree, `exit=0`
([`task018-r66-truecolour-passes-with-defect.log`](phase3f-evidence/task018-r66-truecolour-passes-with-defect.log)).
That is precisely the naive test the PRD warned about: distinctness over authored
values is true while the collision is live, which is why the new test measures the
quantised values. Full write-up, including the survey showing all four other
built-ins have the same collision (recorded as a finding, deliberately not fixed —
§11.6 requires legibility, not distinctness):
[`phase3f-018-r66-matrix-quantised-distinctness.md`](phase3f-018-r66-matrix-quantised-distinctness.md).

## R67: report and test file names say what they are (tasks 019, 020)

**Implementing shas: `b848d28`** (`test: name the eleven task-numbered test files
after what they test`), **`300ee86`** (`docs: record the git-rename evidence task
019's report cites` — the second recorded one-commit-per-task deviation) and
**`e47cb35`** (`docs: title the two duplicate report sections after what they
found`). No product code in any of them.

**Part 1 — eleven test files renamed (`b848d28`).** Eleven files were named after
the task number that created them, not the behaviour they cover; ten sit in
`internal/tui`, so `settings_task017_test.go` and `internal/config/config_task017_test.go`
coexisted covering unrelated things, and task ids are reused across phases. All
eleven moved by `git mv` with **no body edited**, e.g.
`settings_task002_test.go` → `settings_env_override_write_test.go`,
`settings_task015_test.go` → `settings_field_display_and_edit_test.go`,
`settings_task310_test.go` → `settings_edits_from_settings_test.go`; the full
eleven-row mapping with what each covers is in
[`phase3f-019-r67-test-file-names.md`](phase3f-019-r67-test-file-names.md). Two
comments pointing at old filenames were repointed in the same commit
(`internal/tui/matrix_status_tokens_test.go:18`,
`internal/tui/settings_edits_from_settings_test.go:10`) so no dangling in-code
reference remains.

The check that makes this a rename and not a rewrite — `go test -list '.*'` over
both packages, captured at the parent `ce8ef91` and after:

```
$ diff go-test-list-parent-ce8ef91.txt go-test-list-renamed.txt && echo "DIFF-EMPTY exit=$?"
DIFF-EMPTY exit=0
$ ls internal/*/*task[0-9]*_test.go
ls: cannot access 'internal/*/*task[0-9]*_test.go': No such file or directory
$ ci/run.sh go test -count=1 ./internal/tui/ ./internal/config/
ok  	github.com/n-orlov/deck/internal/tui	0.789s
ok  	github.com/n-orlov/deck/internal/config	0.033s
```
441 test functions before, 441 after, same names. Captures and the `git show
--stat` rename evidence:
[`phase3f-019-r67-test-file-names/`](phase3f-019-r67-test-file-names/)
(`go-test-list-parent-ce8ef91{,.raw}.txt`, `go-test-list-renamed{,.raw}.txt`,
`git-rename-evidence.txt`).

**Part 2 — two duplicate section titles (`e47cb35`).**
`docs/reports/phase2b2-findings.md` had two sections both opening `## Task 014 —`
(`:1249` the SIGKILL teardown hang, `:1511` the requirement 19/21 correction), so a
citation "see the `Task 014` section" could not be followed. Both were retitled
subject-first, one line each, so **`:1249` and `:1511` still land on the same
sections** — which matters because the protected
`prds/phase3f-residuals-and-suite-determinism.md:88` cites `:1249` by line and
keeps resolving without editing a protected file. `phase2b2.md:463`/`:592`, which
were citing `Requirement 19/21 correction` against a heading that did not exist,
now resolve and carry the line number too.

The check that can go red is a citation sweep committed with the report,
`docs/reports/phase3f-020-r67-report-section-titles/citation-sweep.py`: over all 96
tracked `*.md` in `docs/` and `prds/` it requires every by-title citation of a
`phase2b2-findings.md` section to resolve to **exactly one** `##` heading (zero =
dangling, >1 = ambiguous) and every by-line citation to exist.

```
$ python3 docs/reports/phase3f-020-r67-report-section-titles/citation-sweep.py
citations checked: 16 by title, 1 by line number, over 96 md files
finding (disclosed, not a failure): docs/reports/phase2b2.md: 'Task 034' ambiguous, matches lines [770, 801, 849, 870, 1048] -- ...
finding (disclosed, not a failure): prds/phase3f-residuals-and-suite-determinism.md: 'Task 014' dangling -- cites the pre-R67 title; prds/ is protected and must not be edited by this job
OK: every section citation resolves to exactly one heading; no dangling line citation
$ echo $?
0
```

**Revert-and-reproduce.** The same script in a throwaway `git worktree` at the
parent `300ee86` — only the heading/citation fix absent — exits 1 on exactly this
defect:

```
PROBLEMS:
  - docs/DELIVERY-LOG.md: section citation 'Task 014' is ambiguous, matches lines [1249, 1511]
  - docs/reports/phase2b2.md: section citation 'Requirement 19/21 correction' is dangling
  - docs/reports/phase2b2.md: section citation 'Requirement 19/21 correction' is dangling
exit=1
```
[`…/task020-r67-citation-sweep-BEFORE-RED.log`](phase3f-020-r67-report-section-titles/task020-r67-citation-sweep-BEFORE-RED.log)
vs [`…-AFTER.log`](phase3f-020-r67-report-section-titles/task020-r67-citation-sweep-AFTER.log),
plus the `grep -rn 'Task 014'` before/after sweeps in the same directory. Two
protected-path findings are disclosed rather than edited away: the PRD's `:467-479`
lists the eleven files under their OLD names and its `:480` quotes the OLD duplicate
title. A third, out of R67's scope: `phase2b2-findings.md` has **five** `Task 034`
sections (`:770`, `:801`, `:849`, `:870`, `:1048`) cited ambiguously — the sweep
prints it every run instead of letting it pass unseen.

## R68: a terminal query can never stall an interactive session (tasks 001, 002)

**Implementing shas: `f3c25d5`** (fix 1, the reply drain) and **`7d060cd`** (fix 2,
the write lock). GitHub issue **#5**. These are the phase's **first two commits** —
the ordering the PRD makes load-bearing (see
[orderings](#load-bearing-orderings-honoured)).

Defect: a pane emitting a terminal query (DA1 `\033[c`, DSR `\033[6n`, OSC 11) made
`vt` compose an answer and write it into an **unbuffered `io.Pipe` nobody read**, so
`Session.drain` parked inside `grid.Write` — while holding `s.mu`. `RenderRows`
could then never take the read lock, which parked `View()` and with it bubbletea's
event loop: the whole TUI wedged, not just the preview.

**Fix 1 (`f3c25d5`)** — `internal/interactive/grid.go`, `resize.go`:
`Session.startReplyDrain` runs one goroutine per `*Grid` that reads `Grid.Read()`
and **discards** it (deck is a viewer; forwarding would feed a live agent a DA1
deck's own emulator fabricated). `newDrainedGrid` starts the drain **before** the
first byte is written, so a query inside a seed/reseed capture parks the drain and
not `Start`/`Resize`. `retireGrid` closes the emulator's reply-pipe **write end**
(`io.PipeWriter.CloseWithError(io.EOF)`) so a displaced grid's drain exits —
necessary because `captureLoop` reseeds every 200ms, i.e. five potential leaked
goroutines a second; `Emulator.Close()` was rejected because `vt`'s
`Emulator.closed` is unsynchronised (recorded as a finding, not worked around).
`installGrid` swaps under the write lock and retires what it displaces; `Close`
retires the last grid and `Wait`s every drain.

**Fix 2 (`7d060cd`)** — the single lock split in two plus a one-frame cache, so a
repaint never waits on the transport at all: `s.mu` now guards the grid **pointer**
only and is **never** held across a `Write`; a new `s.writes` guards grid mutation;
`lastFrame` (`atomic.Pointer[renderedFrame]`) holds the last composed frame.
`writeGrid` is the one place an installed grid is mutated — it resolves the pointer
under `s.mu`, **releases it**, then takes `s.writes` around `g.Write`, then
`MarkDirty`. `RenderRows` composes under `s.mu.RLock` and takes `s.writes` with
**`TryRLock`**: a write in flight means it touches the emulator not at all (a parked
writer also holds vt's `se.mu`, so even `g.Width()` would block) and serves
`staleRows` immediately. Staleness is bounded, not permanent: `MarkDirty` fires
*after* `writes` is released, so the last write always ends in a fresh composition.
Lock order is pinned in the `Session` doc comment (`mu` then `writes`, never the
reverse). `AbsoluteRow`/`SelectedText` deliberately take a *blocking* read — one
mouse gesture wants exactness, and the wait is bounded by fix 1.

Tests — `internal/interactive/replydrain_test.go` (fix 1):
`TestTerminalQueryInPaneOutputNeverStallsSession`, table-driven over **DA1, DSR and
OSC 11** (two CSI triggers and one OSC, not just the observed sequence), each
driving a **real tmux pane** down the actual defect path
(pane output → `pipe-pane` → `Session.drain` → `grid.Write`);
`TestRetireGridUnblocksItsReplyDrain`; `TestEmulatorInputPipeIsAPipeWriter` (pins
the library detail a `go.mod` bump could break). The marker the test looks for is
passed as a `printf` **argument**, never spelled in the format string, because the
shell echoes the typed command line into the pane first — a marker visible in that
echo would pass against unfixed code. `internal/interactive/writestall_test.go`
(fix 2): `TestStalledGridWriteNeverBlocksRenderRows` (≥20 completed `RenderRows`
calls in 250ms while a write is parked, the served frame still carries the
pre-stall marker, **and the write is still parked at the end** — otherwise the test
would be inconclusive rather than green),
`TestStalledGridWriteNeverBlocksAReseed`,
`TestConcurrentWritesAndReadsStayConsistent` (300 rounds × 3 goroutines).

```
$ ci/run.sh go test -count=1 -run 'TestTerminalQueryInPaneOutputNeverStallsSession|TestRetireGridUnblocksItsReplyDrain|TestEmulatorInputPipeIsAPipeWriter' -v ./internal/interactive/
--- PASS: TestTerminalQueryInPaneOutputNeverStallsSession (0.11s)
    --- PASS: TestTerminalQueryInPaneOutputNeverStallsSession/da1 (0.04s)
    --- PASS: TestTerminalQueryInPaneOutputNeverStallsSession/dsr (0.04s)
    --- PASS: TestTerminalQueryInPaneOutputNeverStallsSession/osc11 (0.04s)
ok  	github.com/n-orlov/deck/internal/interactive	0.119s
$ ci/run.sh go test -race -count=1 ./internal/interactive/
ok  	github.com/n-orlov/deck/internal/interactive	87.9s
$ ci/run.sh env DECK_GODOG_TAGS='<the 9 tags in features/interactive_*.feature>' go test -count=1 -v ./features/
14 scenarios (14 passed)
```
(loadavg `4.06 2.59 2.27` green / `1.34 1.95 2.06` red;
[`task002-race-interactive.log`](phase3f-evidence/task002-race-interactive.log),
[`task002-godog-tagsubset.log`](phase3f-evidence/task002-godog-tagsubset.log).)

**Revert-and-reproduce.** Fix 1: `startReplyDrain`'s goroutine returned immediately
(exactly pre-fix behaviour — nothing calls `Grid.Read()`), tests kept. All three
query cases fail **on the test's own 5s deadline, never the package timeout**, and
name the mechanism:

```
replydrain_test.go:149: DEADLOCK: after the pane emitted primary device attributes (CSI c), which vt
  answers at handlers.go:695 -- the query issue #5 was root-caused on, the assertion goroutine never
  returned within 8s -- Session.drain is parked inside grid.Write (vt writes the reply into an
  unbuffered io.Pipe nobody reads) while holding s.mu, so RenderRows can never take the read lock.
--- FAIL: TestTerminalQueryInPaneOutputNeverStallsSession (39.09s)
    --- FAIL: .../da1 (13.03s)  --- FAIL: .../dsr (13.03s)  --- FAIL: .../osc11 (13.04s)
--- FAIL: TestRetireGridUnblocksItsReplyDrain (5.00s)
```
Fix 2, two independent controls: **A** — `grid.go` reverted to `f3c25d5` with the
new tests kept: tests 1 and 2 fail on their own 3s deadlines (test 3 passes, being
the race guard rather than the wedge guard), so the wedge tests cannot pass
vacuously. **B** — the fix applied but `s.writes` stripped, under `-race`:
`WARNING: DATA RACE` (ultraviolet `Buffer.DeleteLineArea` write vs `SelectedText`
read), [`task002-red-nogate-race.log`](phase3f-evidence/task002-red-nogate-race.log)
— so the new gate is load-bearing, not decoration. Every control was applied to a
copy and the tree diffed back afterwards; no `TEMPORARY RED CONTROL` marker remains.
Full write-ups: [`task001-r68-reply-drain.md`](phase3f-evidence/task001-r68-reply-drain.md),
[`task002-r68-write-lock.md`](phase3f-evidence/task002-r68-write-lock.md).

## R69: a retained dead pane is collected on sight (tasks 003, 004)

**Implementing shas: `b8f2513`** (leg 1) and **`366dd78`** (legs 2+3). GitHub issue
**#6**. Both land **before R70's `0745ced`** — the two requirements touch the same
line, `internal/service/reconcile.go:61`.

Defect: with tmux's `remain-on-exit failed`, a crashed pane is **retained** —
`has-session` still succeeds — so a row could read `stopped` while a corpse held the
session name. Reconciliation stepped over it, `r` reported `ResumeAlreadyRunning`
(adopting a pane with nothing in it) and `x` refused as "already stopped": the
session was unrecoverable from inside deck.

**Leg 1 (`b8f2513`)** — reconcile collects a retained dead pane **on sight**,
whatever the row says, instead of skipping the row because its status already reads
terminal (SPEC.md:547, "collected on sight, never retained").

**Legs 2+3 (`366dd78`)** — a new, separate accessor rather than a change of meaning
to an existing one: `tmux.Client.HasLivePane` (`list-panes -t deck_<slug> -F
'#{pane_dead}'`, true iff some pane reads `0`; absent target = `false` as in
`Exists`). **`Exists` is unchanged**, so no existing caller moved. All five `Exists`
call sites were swept and each verdict recorded:
`resume.go:69` → **`HasLivePane`** (the bug); `kill.go`'s new guard stays `Exists`
(a corpse is exactly what kill must remove); `env.go:36` stays (the session's env
table outlives the pane); `restart.go:46` stays (`R` must kill the corpse too, or it
re-earns the duplicate-session refusal); `inject.go:51` stays and is recorded as a
**separate** defect, not silently fixed here. Resume also collects the corpse
(`Exists` → `Kill`) *after* it owns the launch lease and immediately before
`TMux.Create`, because `new-session` would otherwise be refused as `duplicate
session: deck_<name>`. Leg 3 makes kill's refusal "already stopped **and** no tmux
session exists" — belt-and-braces kept deliberately, since being wrong costs a
permanently unrecoverable session and being right costs one `has-session` round trip.
Reachability from the UI is part of the fix: single-row `x` no longer refuses on
`Status == "stopped"` locally but delegates to `service.Kill` (same refusal string),
without which leg 3's guard would be dead code from every real keypress; the bulk
`m`+`x` skip is untouched.

The fixture (`retainedCorpseFixture`) **refuses to proceed unless it is issue #6's
trap**: row `stopped` from a *hook* with `pane_exit_status` NULL, `Exists` true and
`HasLivePane` false at the same instant. It records the corpse's pane id, so the
resume test proves a **new** pane (`%N` differs) rather than adoption.

```
$ ci/run.sh go test -count=1 ./internal/service/ ./internal/tmux/ ./internal/tui/
ok  	github.com/n-orlov/deck/internal/service	3.677s
ok  	github.com/n-orlov/deck/internal/tmux	19.959s
ok  	github.com/n-orlov/deck/internal/tui	0.653s
$ ci/run.sh env DECK_GODOG_TAGS='@requirement-46-interactive-fitted-geometry' go test -count=1 ./features/
ok  	github.com/n-orlov/deck/features	18.107s
$ ci/run.sh env DECK_GODOG_TAGS='@requirement-22-undo-toast' go test -count=1 ./features/
ok  	github.com/n-orlov/deck/features	47.722s
```
loadavg `7.01 3.07 3.17` (leg 1), `10.36 5.00 3.73` → `10.09 5.32 3.86` (legs 2+3),
`9.69 7.14 4.85` (the scenario run).

**Revert-and-reproduce** — each of the four new tests, reverted individually
(`git stash push -- <file>` for service, `cp` for tmux; the tree came back
byte-identical, `diff` empty):

| test | reverted output |
|---|---|
| `TestReconcileCollectsRetainedDeadPaneWhenRowAlreadyReadsStopped` | `reconcile_retained_corpse_test.go:92: collected row reads "stopped" (source "hook"), want error: the crash was not collected` |
| `TestHasLivePaneSeparatesALiveSessionFromARetainedCorpse` | `haslivepane_test.go:92: HasLivePane reported true for a session whose only pane is dead` |
| `TestResumeRelaunchesARowWhoseOnlyPaneIsARetainedCorpse` | `retained_corpse_recovery_test.go:109: resume reported ResumeAlreadyRunning for a session whose only pane is a corpse (#6)` |
| `TestKillRemovesARetainedCorpseAndStillRefusesAGenuinelyGoneSession` | `retained_corpse_recovery_test.go:165: kill a stopped row that still has a retained corpse: session is already stopped` |

The pre-existing `TestResumeAdoptsAlreadyRunningTMuxSessionInsteadOfDuplicateError`
(`resume_test.go:212`) is the **negative control** for requirement 46 and still
passes: a genuinely live pane still reports `ResumeAlreadyRunning`, creates no second
tmux session and records no second launch — so R69 narrowed the check without
weakening the adoption it exists to do.

**Honesty note carried from the evidence**: `DECK_GODOG_TAGS='@requirement-46'`, the
tag as the task worded it, selects **no scenarios at all** (godog matches tags
exactly — [`task004-godog-requirement46-exact.log`](phase3f-evidence/task004-godog-requirement46-exact.log)),
so that green would have been vacuous; the evidence-bearing runs are the suffixed
tag ([`…-suffixed.log`](phase3f-evidence/task004-godog-requirement46-suffixed.log))
and `@requirement-22-undo-toast`
([`…-undo-toast.log`](phase3f-evidence/task004-godog-requirement22-undo-toast.log),
11 scenarios), which drives the real `x` refusal wording through a real keypress.
Write-ups: [`task003-r69-leg1.md`](phase3f-evidence/task003-r69-leg1.md),
[`task004-r69-legs23.md`](phase3f-evidence/task004-r69-legs23.md).

## R70: a resume clears the replaced pane's crash verdict (task 005)

**Implementing sha: `0745ced`** (`store: clear the replaced pane's crash verdict when
a resume relaunches`). GitHub issue **#9**. Landed on top of R69's rewritten
`terminal` guard, not beside it — R69 (`b8f2513`, `366dd78`) first, as required.

Defect: `pane_exit_status`/`crash_tail` outlived the pane they described. After a
resume the row carried the *previous* pane's crash verdict, which armed two guards
against the new pane: `reconcile.go`'s `terminal` gate skipped the row, and
`store.go:610` dropped the new pane's own `running` hook — `session_start` present in
`events` while the status column still read the previous day, exactly as the
operator's transcript showed.

Fix: one clause, in the transaction that already clears `killed_by_user` —
`internal/store/lease.go`, `AcquireLaunchLease`'s acquisition `UPDATE` now also sets
`pane_exit_status = NULL, crash_tail = NULL`. `git diff --stat` for the product change
is exactly `internal/store/lease.go | 23 ++++++--`; `store.go` and `reconcile.go` are
not modified at all. Both standing prohibitions hold: **`store.go:650`'s `COALESCE` is
untouched** (first-writer-wins on a crash verdict still holds for `UpdateSessionStatus`)
and **`store.go:610`'s hook guard is untouched** (a `running` hook is still dropped
while `pane_exit_status` is set — correct as written once the column is cleared by the
only transaction that knows a *new* pane is replacing the described one).

`crash_tail` is cleared **deliberately**, not for convenience: it has control-flow and
display meaning (the TUI renders it as the row's durable crash artifact), so a retained
tail would caption a live, freshly launched session with a dead one's last words — the
exact class of defect #9 is about. The forensics stay where forensics belong: the
`tmux.pane_dead` event still names the exit status (`tmux pane exited with status 137`)
and the audit log still carries the transition.

Tests — all three legs, and **both required tests drive a post-launch transition**, as
the PRD demands, because reading the status straight after `launch.ready` passes on
unfixed code (`starting` is genuinely correct at that instant):

| test | what it pins | file |
|---|---|---|
| `TestResumeMakesACrashedRowReconcilableAgain` | guard 1, `reconcile.go`'s `terminal` skip | `internal/service/resume_crash_verdict_test.go` |
| `TestResumedRowAcceptsTheNewPanesFirstHook` (`SessionStart`, `UserPromptSubmit`) | guard 2, `store.go:610` | `internal/hookrecv/resumed_crash_verdict_test.go` |
| `TestAcquireLaunchLeaseClearsTheReplacedPanesCrashVerdict` | the column write, including raw `NULL`ness | `internal/store/lease_test.go` |

The hook leg asserts on the **status column** (`running`/`hook`/`status_at`) and
separately asserts the event landed either way (count 1 before *and* after the fix) —
that asymmetry is the trap in the operator's row. The reconcile leg's fixture is built
through real code paths (a real pane exits 137, a real `svc.Reconcile` collects it, the
row is really killed through `svc.Kill` — the only in-app route back to a leasable row)
and it **refuses to proceed unless the row is discriminating**: `stopped` *and* still
carrying the previous pane's verdict. The row is a `shell` so "observed, not skipped"
has an observable consequence (the `starting → running` promotion, its
`tmux.shell_live` event, its audit line), and the observation assertions come **before**
the column assertions on purpose, so unfixed code fails on the behaviour rather than on
the cause.

```
$ ci/run.sh go test -count=1 ./internal/store/ ./internal/service/ ./internal/hookrecv/
ok  	github.com/n-orlov/deck/internal/store	2.939s
ok  	github.com/n-orlov/deck/internal/service	3.993s
ok  	github.com/n-orlov/deck/internal/hookrecv	4.812s
$ ci/run.sh env DECK_GODOG_TAGS='@crash' go test -count=1 ./features/
ok  	github.com/n-orlov/deck/features	23.709s
```
[`task005-r70-green.log`](phase3f-evidence/task005-r70-green.log),
[`task005-r70-crash-feature.log`](phase3f-evidence/task005-r70-crash-feature.log);
`gofmt -l` and `go vet` clean on all three packages. loadavg `3.10 3.01 3.67` (revert),
`3.17 3.03 3.67` (green), `3.99 3.25 3.73` (scenarios).

**Revert-and-reproduce** — `git stash push -- internal/store/lease.go` (product code
only, tests kept), run, `git stash pop`, then `diff` against a pre-stash copy:
`lease.go` restored byte-identical.

```
--- FAIL: TestAcquireLaunchLeaseClearsTheReplacedPanesCrashVerdict (0.04s)
    lease_test.go:317: pane_exit_status = 137 after resume; want it cleared with the pane it described
--- FAIL: TestResumedRowAcceptsTheNewPanesFirstHook/SessionStart (0.04s)
    resumed_crash_verdict_test.go:94: the new pane's SessionStart was dropped by the replaced pane's
    crash verdict (#9): status="starting" source="tmux" status_at=13
--- FAIL: TestResumedRowAcceptsTheNewPanesFirstHook/UserPromptSubmit (0.02s)
--- FAIL: TestResumeMakesACrashedRowReconcilableAgain (0.12s)
    resume_crash_verdict_test.go:114: the resumed row was skipped by reconciliation rather than
    observed (#9): ... Status:"starting" ... PaneExitStatus:(*int)(0xc000304950),
    CrashTail:"shell: killed by the OOM reaper\n...Pane is dead (status 137, ...)"
```
[`task005-r70-revert-reproduce.log`](phase3f-evidence/task005-r70-revert-reproduce.log).
Blast radius checked: no scenario expects a crash tail to survive a resume
(`features/crash.feature` and `features/preview.feature:215` read the artifact on the
**error** row and never resume it). Write-up:
[`task005-r70.md`](phase3f-evidence/task005-r70.md).

## R71: archived rows refuse launches and can be unarchived (tasks 006–008)

**Implementing shas: `88742b2`** (leg 1), **`63d4189`** (leg 2), **`9d43a32`** (leg 3).
GitHub issue **#8**. All three land **before R72's `10f3970`** — R72's `u` undo needs
leg 2's `Store.UnarchiveSession` to exist.

**Leg 1 (`88742b2`)** — `resume` refuses an archived row **before the launch lease and
before anything is created** (SPEC.md:718: an archived session is not startable);
`restart` inherits the refusal because `R` routes through resume, so one guard covers
both. Red with the guard stashed and the test file kept:

```
--- FAIL: TestResumeRefusesAnArchivedRowBeforeAnythingIsCreated (0.10s)
    resume_archived_refusal_test.go:53: resume of an archived row succeeded (SPEC.md:718: an archived session is not startable)
--- FAIL: TestRestartInheritsTheArchivedRefusalFromResume (0.09s)
    resume_archived_refusal_test.go:119: restart of an archived row succeeded (SPEC.md:718: R routes through resume, so one guard covers both)
```
Green at the leg-1 tree: `ok internal/service 3.267s`, plus the neighbours (archive still
sets the flag, kill-and-archive still one action) `ok features 2.813s` and the guard's own
code path (launch lease, `R` applies an env edit, inject-instead restart) `ok features
6.705s` — loadavg 2.2–3.1.
[`task006-r71-leg1-revert-reproduce.log`](phase3f-evidence/task006-r71-leg1-revert-reproduce.log),
[`…-feature-green.log`](phase3f-evidence/task006-r71-leg1-feature-green.log),
[`…-service-green.log`](phase3f-evidence/task006-r71-leg1-service-green.log).

**Leg 2 (`63d4189`)** — `Store.UnarchiveSession` (modelled on `RestoreSession`, as the
standing rules require — no new machinery) and `U` bound in the TUI, **reachable from the
filter results**, which is the only place an archived row can be seen. Two mutations show
both halves are load-bearing:

- *Mutation A, the naive `U`*: act on `m.baseSessions[m.selected]` instead of the
  displayed (filtered) `m.sessions[m.selected]`. It compiles, `U` still exists, help
  parity still passes — and the one row `U` exists for becomes unreachable, because an
  archived row is never in `baseSessions`:
  `unarchive_test.go:64: U inside the filter results issued no command at all`, and the
  scenario fails on the real keypress with the real frame showing
  `Cannot unarchive: session is not archived` /
  `Filter "unarchive-target" in force (1 matching)`.
- *Mutation B*: reload only the default list —
  `unarchive_test.go:110: a successful unarchive returned tui.sessionsLoaded, want a tea.Batch of both reloads`.

Green baseline at `63d4189`: `ok internal/tui 0.709s`, `ok internal/store 2.411s`,
`ok internal/service 3.653s`, `ok cmd/deck 5.121s`, and 4 scenarios / 47 steps passed —
including `@requirement-33-filter-reaches-archived-row`, **unmodified by this task**.
After each mutation the file was restored from a `/tmp` copy and the tree proved
byte-identical (`diff` silent, `git status --short` empty).
[`task007-r71-leg2-revert-reproduce.md`](phase3f-evidence/task007-r71-leg2-revert-reproduce.md),
[`…-feature-green.log`](phase3f-evidence/task007-r71-leg2-feature-green.log),
[`…-green.log`](phase3f-evidence/task007-r71-leg2-green.log),
[`…-revert-reproduce.log`](phase3f-evidence/task007-r71-leg2-revert-reproduce.log).

**Leg 3 (`9d43a32`)** — hook resolution reads a new
`store.ListSessionsIncludingArchived` (`WHERE deleted_at = 0`) instead of the sidebar's
display query `ListSessions` (`WHERE deleted_at = 0 AND archived_at = 0`). **Each half of
the new `WHERE` clause is pinned in the direction it matters**, which is what makes the
pair non-vacuous:

- *Experiment 1 — the pre-task code itself* (resolve reads `ListSessions`):
  `archived_resolution_test.go:44: conversation-id hook on an archived row: hook session could not be resolved (conversation_id="archived-conversation" injected_session_id="")`
  — issue #8's captured pane banner in miniature. The negative test
  (`TestReceiveKeepsATombstonedRowsHookAnOrphan`) passes under this revert, **as it must**.
- *Experiment 2 — the lazy fix the PRD warns about* (drop `deleted_at = 0` too, i.e. "all
  rows"): the tombstone test fails in both key modes
  (`want unresolved orphan`) and
  `TestListSessionsIncludingArchivedKeepsArchivedAndDropsTombstoned` fails with
  `[archived archived-then-deleted plain tombstoned], want [archived plain]`.

Green baseline `ok internal/hookrecv 4.690s`, `ok internal/store 3.037s`, `go vet ./...`
clean (loadavg 6.97→9.06). **`reconcile`'s use of `ListSessions` is deliberately
unchanged** — `git diff internal/service/reconcile.go` is 0 lines, as the standing rules
require: once leg 1 refuses launches on an archived row, an archived row cannot own a live
pane, and including archived rows in reconciliation would let it **write** statuses onto
them, a new bug. Tombstoned rows remain unresolvable.
[`task008-r71-leg3-revert-reproduce.md`](phase3f-evidence/task008-r71-leg3-revert-reproduce.md).

## R72: `A` confirms before it archives, and `u` undoes it (tasks 009–011)

**Implementing shas: `10f3970`** (the confirm), **`4822484`** (the toast + `u` undo),
**`eb2089e`** (the R71/R72 end-to-end round trip). GitHub issue **#10**. All after
R71's three legs, as required. `A` was **not** rebound and no chord was added.

**The confirm (`10f3970`)** — `A` opens a dialog that names the kill and **writes
nothing** until it is confirmed; the confirm reuses the existing `deleteConfirming`
shape rather than new machinery, suppresses the bare-letter keymap while open, and stays
open with a reason when the archive fails. Red under the *plausible naive*
implementation — the pre-R72 behaviour restored in place (archive on the keypress, no
confirm), everything else left in the tree so `go build ./...` still printed `BUILD_OK`:

```
--- FAIL: TestArchiveKeyOpensAConfirmAndWritesNothing (0.00s)
    archive_confirm_test.go:67: A issued a command on the keypress itself (tui.sessionArchived from running it) -- it must write nothing until confirmed
--- FAIL: TestArchiveConfirmSubmitArchivesExactlyTheRowItNamed (0.00s)
--- FAIL: TestArchiveConfirmSuppressesTheBareLetterKeymap (0.00s)
--- FAIL: TestArchiveConfirmFailedSubmitStaysOpenAndSaysWhy (0.00s)
```

The scenario half carries the load the standing rules put on it: the repaired
`clientArchivesSelectedSession` step still sends a **real `A`** and drives the **real
confirm dialog** — and the proof is that a **pre-existing** caller of that step
(`features/filter.feature:49`) goes red under the naive product code, which it could not
if the step called the service or auto-confirmed:

```
3 scenarios (3 failed) / 33 steps (6 passed, 3 failed, 24 skipped)
  Error: after scenario hook failed: timed out waiting for frame "Archive filter-archived-row": context deadline exceeded
  Error: after scenario hook failed: timed out waiting for frame "Archive archive-confirm-live": context deadline exceeded
  Error: after scenario hook failed: timed out waiting for frame "Archive archive-confirm-submit": context deadline exceeded
```
Restored (`RESTORED_IDENTICAL`, `git status --short` showing only this task's own paths)
and green: `ok internal/tui 0.637s`, `ok features 7.757s` over a nine-tag set that is the
two new scenarios **plus every** scenario calling `clientArchivesSelectedSession`
(`event_log.feature:15`, `filter.feature:49` and `:67`, `kill_delete_undo.feature`'s four
archive scenarios). Rendered help grew by one line to **230** at `100x260`, re-measured
the way `cmd/deck/main_test.go:395` documents — still under that test's 260-row window.
[`task009-r72-archive-confirm-revert-reproduce.md`](phase3f-evidence/task009-r72-archive-confirm-revert-reproduce.md).

**The toast and `u` (`4822484`)** — a confirmed archive says what happened
(`Killed and archived — press u to unarchive (agent stays stopped)`) and `u` undoes it
through R71's `UnarchiveSession`, using the same undo-trio shape as the two existing
trios. Reverting just that wiring to the pre-task body (`return m, m.loadSessions`), with
the fields, `archiveUndoExpired`, `archiveUndoNoteLines`, the `u` fall-through and the
`computeLayout` reservation all left in place:

```
  Scenario: a confirmed archive says what happened and u puts the row back in the default list # kill_delete_undo.feature:517
    And deck client "A" screen contains "Killed and archived" # kill_delete_undo.feature:533
      Error: after scenario hook failed: client "A" did not show "Killed and archived" within 5s: ...
1 scenarios (1 failed) / 13 steps (5 passed, 1 failed, 7 skipped)
```
The following `u` steps were skipped, so the scenario is red on the toast **and** would be
red on the undo: the naive branch leaves `archiveUndoSessionID` empty and `u` never
reaches `UnarchiveSession`. Restored and green together with every other
archive/unarchive scenario, `ok features 4.594s`, with an explicit **vacuity check**
against the notes' gotcha that godog matches tags exactly: a deliberately absent tag
returns in `0.019s`, the new tag alone in `0.632s`, the seven-tag set in `4.594s` — so the
scenario really executes.
[`task010-r72-archive-undo-toast.md`](phase3f-evidence/task010-r72-archive-undo-toast.md),
[`…-revert-reproduce.log`](phase3f-evidence/task010-r72-archive-undo-toast-revert-reproduce.log).

**The round trip (`eb2089e`)** — one new scenario,
`features/filter.feature:58` `@requirement-33-archive-round-trip`: create two shell
sessions, `A` on the selected target, assert the confirm names the kill and has written
nothing, submit, assert the row left the default frame (pane gone, `archived_at` set),
find it with `/`+Enter, `U`, `r`, clear the filter, assert the row is back in the plain
default list and live. **No new step definitions** — every leg goes through the real
keypress on the real pty, nothing calls archive/unarchive/resume directly; no `sleep`, no
widened timeout, no `@flaky`, and the only waits are the package's pre-existing
consequence-bound polls. Red under the same `case "A"` mutation (taken from
`git show 10f3970`, so a real pre-fix body rather than a delete):

```
  Scenario: the whole round trip: A plus its confirm hides the row, / finds it, U and r bring it back live ... # filter.feature:59
    When deck client "A" presses A on its selected session "trip-target" # filter.feature:80
      Error: after scenario hook failed: timed out waiting for frame "Archive trip-target": context deadline exceeded
1 scenarios (1 failed) / 30 steps (5 passed, 1 failed, 24 skipped)
```
and the failing frame shows the mutation's own damage: the row is gone and the toast reads
`Killed and archived — press u to unarchive` although **no confirm was ever shown** — the
field defect R72 fixed, reproduced. loadavg `2.16 2.18 2.63`.
[`task011-r71-r72-round-trip.md`](phase3f-evidence/task011-r71-r72-round-trip.md),
[`task011-mutation-red.log`](phase3f-evidence/task011-mutation-red.log).

## R73: the three scrollable overlays scroll by line and by wheel (tasks 012–014)

**Implementing shas: `2714d1b`** (leg 1, line scroll), **`4edbfc2`** (leg 2, wheel
routing), **`9c2e66a`** (leg 3, help text + recorded judgement calls). GitHub issue **#7**.

**Leg 1 (`2714d1b`)** — no second scroller was written, per the standing rule against
duplicating machinery: the existing `dialogScrollBy` gained a **step parameter**
(`step < 1` treated as 1, so a keypress can never be a silent no-op) with its existing
`[0, dialogMaxScroll(body)]` clamping untouched, plus two thin named wrappers —
`dialogScrollByPage` (step = `dialogContentBudget()`, PgUp/PgDn's old behaviour) and
`dialogScrollByLines` (step = 1). `updateHelpView`, `updateDetailView` and
`updateEventLog` each gained `case "up", "k":` / `case "down", "j":`.

Tests (`internal/tui/overlay_line_scroll_test.go`, three tests × the three overlays as
subtests, driving the real `Update`/`View` path) compare **rendered rows**, never
`helpScroll`/`detailScroll`/`eventLogScroll` and never "the view changed" — which is the
PRD's whole test-design point, since binding `down` to the *page* step also changes the
view: after `down`, the first rendered content row is the row that was **second** before
the press. `TestScrollableOverlaysStillPageWithPgDn` **refuses to run** if a page step
there were < 2 lines, so it cannot silently stop discriminating. Fixtures reuse the
existing overflowing ones. `ok internal/tui 0.662s`, `ok cmd/deck 5.386s`.
[`task012-r73-line-scroll.md`](phase3f-evidence/task012-r73-line-scroll.md),
[`…-reverted.log`](phase3f-evidence/task012-r73-line-scroll-reverted.log).

**Leg 2 (`4edbfc2`)** — the wheel reaches the three scrollable overlays **without
loosening the fifteen-flag action-suppressing guard by one flag**: a new
`wheelScrollableOverlay()` (which of the three owns the keyboard, mirroring `Update`'s own
dispatch order) and `scrollWheelOverlay(dir)` (same `dialogScrollByLines` step, same body
the key handlers pass), called from one block placed immediately **before** the early
return. The guard's condition is byte-for-byte unchanged:

```
$ git diff -U0 internal/tui/tui.go | grep -c '^[-+].*m.settingsDiscardConfirm'
0        # the fifteen-flag line is not touched by this commit
```

Three mutations, each applied in place and reverted (`diff` clean afterwards):

| # | plausible naive implementation | result |
|---|---|---|
| A | delete the wheel block — the pre-R73 tree, guard swallows every `MouseMsg` | RED: "advanced by 0 lines, want exactly 1" for all three overlays; **click tests stay green**, correctly — clicks were already suppressed ([log](phase3f-evidence/task013-mutationA-guard-swallows-wheel.log)) |
| B | the tempting one-liner: drop `m.help`/`m.detail`/`m.eventLogOpen` from the fifteen-flag guard | RED: `TestClickInsideOverlayStillDoesNothing/{detail_view,help_overlay}` — "press over the hidden sidebar row underneath moved the selection from 0 to 1". **Wheel tests stay green**, which is exactly why the click half is needed ([log](phase3f-evidence/task013-mutationB-scrollable-overlays-let-clicks-through.log)) |
| C | wheel wired to `dialogScrollByPage` | RED: "moved by more than one page, want exactly 1 line"; "advanced by 14 lines, want exactly 1" ([log](phase3f-evidence/task013-mutationC-wheel-bound-to-page-step.log)) |

Clicks and drags stay suppressed **for all fifteen overlays**, pinned by
`TestClickInsideOverlayStillDoesNothing` (press at the border, over body text, over the
hidden sidebar row, outside the box, plus drag, release and a second press — frame
byte-identical) over two scrollable and two unscrollable overlays, and by
`TestWheelOverUnscrollableOverlayDoesNothing`; `TestWheelInOverlayHonoursMouseOptOut`
keeps requirement 3/37's `Mouse=false` opt-out, and
`TestWheelStillScrollsTheSidebarWithNoOverlayOpen` keeps requirement 34's binding.
`ok internal/tui 0.802s`; mouse scenario tags `ok features 37.046s`, non-vacuous (a bogus
tag returns in `0.020s` against 37s) —
[`task013-mouse-feature-tags.log`](phase3f-evidence/task013-mouse-feature-tags.log),
[`task013-r73-wheel-routing.md`](phase3f-evidence/task013-r73-wheel-routing.md).

**Leg 3 (`9c2e66a`)** — the help overlay names the new bindings (six lines: the
`↑/↓ or j/k` tail, the `PgUp/PgDn` tail, and a new Mouse-section
`wheel over an overlay` entry), pinned by two **existing** tests that read the live
`helpText` — `TestEmptyAndHelpViewsAreDiscoverable` and the real-PTY
`TestDeckBinaryEmptyHelpAndQuitThroughPTY`. Red with the six lines removed and everything
else at HEAD, in both, naming all six phrases:

```
tui_test.go:89: help view missing "wheel over an overlay"
tui_test.go:89: help view missing "pages share no line"
main_test.go:484: released help missing "by exactly one line per" through the real PTY: ...
FAIL	github.com/n-orlov/deck/internal/tui
FAIL	github.com/n-orlov/deck/cmd/deck
```
[`task014-help-text-mutation.log`](phase3f-evidence/task014-help-text-mutation.log).

No golden frame pinned this text and **no expected count was changed to accommodate it**:
the PTY help window stays at **260 rows** because `helpView` re-measured at **239** lines
at 100 cols (was 229), leaving 21 rows of headroom
([`task014-help-height-probe.log`](phase3f-evidence/task014-help-height-probe.log)), and no
test asserts an absolute scroll extent — `TestHelpOverlayScrollReachesEveryLine` presses
PgDn 40 times (40×22 = 880 > the new `dialogMaxScroll` 357) and the R73 tests compare rows
relatively. Three stale "273 lines at 80x24" comments were refreshed to the re-measured
379; `help_keymap_parity_test.go`'s 273 is explicitly a historical measurement and left
alone. Two judgement calls are recorded rather than inherited silently: **PgUp/PgDn pages
stay non-overlapping** (the new one-line step is exactly the page-seam affordance an
overlap would have bought, and overlap would break the `PgDn`→`PgUp` round trip) — now
stated in the product's own help text and in `dialogScrollByPage`'s doc comment — and the
`framedDialog` overflow survey (three dialogs can overflow;
[`task014-framed-overflow-probe.log`](phase3f-evidence/task014-framed-overflow-probe.log)).
Deliverable run for this leg: `ok internal/tui 0.719s`, `ok features 303.284s`
([`task014-criteria-suite.log`](phase3f-evidence/task014-criteria-suite.log), loadavg
`2.91 3.04 3.20`). Write-up:
[`task014-r73-help-text-and-decisions.md`](phase3f-evidence/task014-r73-help-text-and-decisions.md).

## The nine naive-test traps: revert and reproduce

The PRD names, for nine of the eleven requirements, a *naive* test that would pass
without the fix. Each was answered the same way — **actually revert the fix (or apply
the plausible naive implementation), run, quote both outputs, restore and prove the
tree byte-identical**. R64 and R67 have no named trap; R64 is instead covered by the
two controls in its own section (a mutated-name positive control and a
waypoint-deleted control) and R67 by its citation sweep going red at the parent commit.

| # | req | the trap: what a naive test would do | what was reverted/mutated | red output (abbreviated) |
|---|---|---|---|---|
| 1 | R63 | assert the coalescer *field* is set — true on the old code too | the `previewFitInFlight` half of the guard | `second previewTick with the first fit still in flight returned 2 commands, want 1 (the reschedule alone)` |
| 2 | R65 | trust a settle that has been made permissive; or assert only one direction | product code, twice, **with the settle in place**: an extra paced `resizeWindow`; then a fit forced where none is expected | `received 2 SIGWINCH signals, want exactly 1` / `received 1 SIGWINCH signals, want exactly 0` |
| 3 | R66 | measure distinctness over **authored** colours | the three authored hexes only | new test: `#7f7f7f: archived, idle, stopped … 5 distinct … want 7`; **and the true-colour test still passes**, `exit=0` — the trap demonstrated, not just described |
| 4 | R68 | let the test hang on the package timeout, or look for a marker the shell already echoed | fix 1: the drain goroutine returns immediately; fix 2 (A): `grid.go` back to `f3c25d5`; fix 2 (B): `s.writes` stripped, under `-race` | `DEADLOCK: … Session.drain is parked inside grid.Write … so RenderRows can never take the read lock` — **on the test's own 5s deadline**, never the package timeout; and `WARNING: DATA RACE` |
| 5 | R69 | check only the reconcile leg, or accept a hook-sourced `stopped` fixture that is not the trap | leg 1's guard; then `resume.go`+`kill.go`; then `HasLivePane` delegating to `Exists` | `collected row reads "stopped" (source "hook"), want error` / `resume reported ResumeAlreadyRunning for a session whose only pane is a corpse (#6)` / `kill …: session is already stopped` / `HasLivePane reported true for a session whose only pane is dead` |
| 6 | R70 | read the status straight after `launch.ready`, where `starting` is genuinely correct | the two `NULL` clears in `AcquireLaunchLease` | `pane_exit_status = 137 after resume; want it cleared with the pane it described`; `the new pane's SessionStart was dropped by the replaced pane's crash verdict (#9)`; `the resumed row was skipped by reconciliation rather than observed (#9)` |
| 7 | R71 | bind `U` against the unfiltered list (works for every row except the only one that matters); or widen the hook query to "all rows" | leg 1's guard; `m.sessions` → `m.baseSessions`; both-reloads → one; `resolve` back to `ListSessions`; then `deleted_at = 0` dropped as well | `resume of an archived row succeeded (SPEC.md:718 …)` / `U inside the filter results issued no command at all` / `conversation-id hook on an archived row: hook session could not be resolved` / `want unresolved orphan` |
| 8 | R72 | let the step call the service or auto-confirm instead of pressing the real key | `case "A"` restored to its pre-R72 body (archive on the keypress); then the toast/undo wiring only | `A issued a command on the keypress itself … it must write nothing until confirmed`; and a **pre-existing** caller of the repaired step goes red: `timed out waiting for frame "Archive filter-archived-row"`; `did not show "Killed and archived" within 5s` |
| 9 | R73 | assert the scroll *offset* or "the view changed" — both true if `down` were bound to the page step | the wheel block deleted; the fifteen-flag guard loosened; the wheel bound to the page step; the six help lines removed | `advanced by 0 lines, want exactly 1` / `press over the hidden sidebar row underneath moved the selection from 0 to 1` / `advanced by 14 lines, want exactly 1` / `help view missing "wheel over an overlay"` |

Two properties of the table are worth stating explicitly, because they are what makes
it evidence rather than decoration:

- **Several rows are pairs of opposed controls**, so neither direction can be satisfied
  by a permissive test: R65's extra-signal *and* unexpected-signal reverts; R71 leg 3's
  "drop `archived_at = 0`" (required) *and* "keep `deleted_at = 0`" (required); R73
  leg 2's mutation A (wheel red, clicks green) *and* mutation B (clicks red, wheel
  green). A single-sided fix fails one of each pair.
- **The negative controls stayed green under the reverts they should**: requirement 46's
  `TestResumeAdoptsAlreadyRunningTMuxSessionInsteadOfDuplicateError` (R69),
  `TestReceiveKeepsATombstonedRowsHookAnOrphan` (R71 leg 3),
  `TestConcurrentWritesAndReadsStayConsistent` under fix-2 control A (R68), and R73's
  click-suppression tests under mutation A. A revert that turned *everything* red would
  prove only that the build was broken.

Every revert was done on a copy or via `git stash`, and each write-up records the
restore proof (`diff` silent / `git status --short` showing only that task's own paths).
No `TEMPORARY RED CONTROL`, `if true` or mutated fixture survives in the tree.

## Green whole-suite run at the final code commit (task 021)

**Cited by sha and by log path**, as the evidence rules require.

| | |
|---|---|
| commit | **`e47cb35`** — the phase's **final code tree** |
| command | `ci/run.sh go test -p=1 -count=1 ./...` (no tag filter, no path filter, no `-run`, nothing excluded) |
| result | **exit 0**; 14 × `ok` + 3 × `[no test files]` (`internal/notify`, `internal/search`, `internal/unit`) |
| wall clock | **360s** (6m00s), start `2026-08-26T19:04:45Z` (`1787858685`); `features` alone 308.184s |
| loadavg | `2.69 2.60 2.82` at start → `3.82 4.29 3.60` at end (28-core host) |
| log | [`phase3f-021-fullsuite/go-test-p1-count1-all.log`](phase3f-021-fullsuite/go-test-p1-count1-all.log) (raw stdout, `EXIT=0` appended); report [`phase3f-021-fullsuite/README.md`](phase3f-021-fullsuite/README.md) |

`e47cb35` and its parent `300ee86` are docs-only, so the tree under test is
byte-identical to `b848d28`'s in every source file — **checked, not asserted**:

```
$ git diff --stat b848d28 e47cb35 -- '*.go' '*.feature' '*.toml' '*.sh' go.mod go.sum
$                     # empty: no code difference
$ awk 'length($0)<400 && $0 !~ /^(ok|\?)/' go-test-p1-count1-all.log | grep -v EXIT=0
$                     # empty: no FAIL, no failure block, no build error
```

**One green run is not the stability claim, and it was not made to look like one**: it
is the first and only whole-suite invocation at `e47cb35` — nothing discarded, nothing
repeated, no streak hunted — and the report names the two known intermittent reds
(`preview.feature:134`, `TestGoldenMinimumFrame`) that did **not** fire, without
treating their absence as evidence they are gone.

## Stability: the real rate is 9/10 (task 022)

**The published rate is 9/10. Not 10/10.** Ten runs were commissioned, ten were run,
none was re-run and no eleventh run was made to hunt a streak.

| | |
|---|---|
| command | `ci/stability.sh 10` (no arguments beyond the count; no tags, no path filter, no env override) |
| commit | **`c12c30e`**, whose **code tree is identical to `e47cb35`** — the phase's final code commit, the same tree task 021 ran |
| script exit status | **1**; its own verdict line `9/10 passed` |
| wall clock | **3561s = 59m21s**, `1787772301` → `1787775862` (`2026-08-26T19:25:01Z` → `20:24:22Z`) |
| per-run command | `ci/run.sh go test -p=1 -count=1 ./...`, one throwaway sibling container per run |
| mktemp log dir | `/tmp/deck-stability.QRo6Ac/` (`summary.log`, `run-1.log` … `run-10.log`) |
| committed logs | [`phase3f-022-stability10/`](phase3f-022-stability10/): [`README.md`](phase3f-022-stability10/README.md), [`run-markers-timestamped.log`](phase3f-022-stability10/run-markers-timestamped.log), [`loadavg-trace.log`](phase3f-022-stability10/loadavg-trace.log) (725 five-second samples), [`run-pass-logs.log`](phase3f-022-stability10/run-pass-logs.log), [`run-9-FAIL-trimmed.log`](phase3f-022-stability10/run-9-FAIL-trimmed.log) |

Code-tree identity **checked, not asserted**, and the required ordering restated:

```
$ git rev-parse HEAD
c12c30e5104b59123b1b8ad6b9ba72b09f466dfb
$ git diff --stat e47cb35 HEAD -- '*.go' '*.feature' '*.toml' '*.sh' go.mod go.sum
                                    # empty: no code, feature, theme, script or module change
$ git status --short                # empty, before and after the ten runs
```
R63 (`f7b97fe`) landed **before** this run, as the PRD requires, and before R65's own
evidence commits (`5071389`, `202e1ba`).

Nothing was skipped, tagged out or retried: `defaultTags` stayed
`"~@real-agents && ~@nightly"`, no `DECK_GODOG_TAGS`/`DECK_GODOG_PATHS` was set, and run
9's log shows the full population — `306 scenarios (305 passed, 1 failed)`,
`3440 steps (3436 passed, 1 failed, 3 skipped)`, the 3 skipped being the remainder of the
one failed scenario, not a tag exclusion. Every PASS run is 14 `ok` + 3 `[no test files]`,
`features` 297.3–328.7s (run 9's failing `features` 303.1s, squarely inside that band).

**The one failure, run 9** — quoted from
[`run-9-FAIL-trimmed.log`](phase3f-022-stability10/run-9-FAIL-trimmed.log):

```
--- FAIL: TestFeatures (284.45s)
    --- FAIL: TestFeatures/a_fit_is_skipped_below_the_7-inner-row_floor,_and_retried_once_the_panel_grows_back_above_it (2.56s)
        suite.go:640: after scenario hook failed: fake "claude" agent received 1 SIGWINCH signals, want exactly 0
  Scenario: a fit is skipped below the 7-inner-row floor …  # preview.feature:134
    Then the fake "claude" agent received exactly 0 SIGWINCH signals   # preview.feature:147
FAIL	github.com/n-orlov/deck/features	303.113s
```

**Named mechanism** (not "flaky"): a pre-resize passive preview fit for `beacon`,
licensed at the client's default 100x30 before the scenario shrinks the panel to 100x9,
whose success is decided by an unbounded race in `previewFit`'s no-live-pane early return
— which emits `previewFitDone` anyway and so spends the row's single coalesced fit. Run
9's own frames carry the signature: the crop line reads `61x6 of 61x27` (six occurrences)
— the **100x30** geometry, not the 100x9 one the scenario resizes to — identical to task
017's legible reproduction.

**Host load is ruled out, not merely doubted.** Run 9 started at the **lowest** 1-minute
loadavg of all ten starts (`1.49`) with an in-run band of `1.21`–`4.33`, while runs 1–3
peaked at `11.6`–`15.0` and all passed. (This inverts Phase 3e's observation, where the
same assertion failed at the *highest* start-loadavg of ten.)

**Requirement attribution, stated plainly:**

- **R64's mechanism did not recur.** No `starting` assertion failed in any of the ten
  runs, and neither `create_cwd_ghost.feature` scenario appears in run 9's failures.
- **R65's mechanism did recur — same feature file, same line, same assertion, same
  message as R65's own cited source** (`docs/reports/phase3e-408-stability-10-at-75861e0/README.md`
  item 2). Per the standing evidence rules that makes R65 a **FAILED requirement**, not a
  host-load note: R65's *assertion* criteria are met (the poll-shaped unsoundness is gone,
  both red directions demonstrated), its *field-symptom* claim is not. What remains is a
  second, product-side mechanism at the same site that R65 was explicitly forbidden to
  paper over and therefore left standing, with the finding written up in advance and
  flagged as threatening this very deliverable.
- **`TestGoldenMinimumFrame`** ("frame kept changing", seen at task 016) did **not** fire:
  `internal/tui` is `ok` in all ten logs. It stays open and unproven-fixed rather than
  retired by this run.

Carried forward as an open item needing **its own task**: restructure
`preview.feature:134`'s prefix so no fit is licensed at the larger size, or bound
`previewFit`'s no-live-pane return so it cannot spend the coalesced fit. **Re-baselining
the counts is forbidden.**

## Per-requirement table

One row per requirement: implementing sha(s), the tests/scenarios added, the
revert-and-reproduce proof, and the verdict. Every sha in this table resolves under
`git cat-file -e` and every log path referenced from this report exists in the tree — the
checking command and its clean output are quoted in this report's own commit message.

| req | issue | sha(s) | tests / scenarios added | trap named by the PRD? | revert-and-reproduce | verdict |
|---|---|---|---|---|---|---|
| **R63** | steer-018 residual | `f7b97fe` | `internal/tui/preview_fit_overlap_test.go`: `TestPreviewFitDoesNotOverlapItself`, `TestPreviewFitResumesAfterItsDoneLands`, `TestPreviewFitDoneForUnselectedSessionClearsTheMarker` | yes | `previewFitInFlight` half reverted → `returned 2 commands, want 1` | **met** |
| **R64** | — | `677f5a0` (no product code) | five waypoints replaced by `has session … selected` in `create_cwd_ghost.feature` (×4) and `create_cwd_tab.feature`; full classification table above | no (two controls done anyway) | mutated name → `timed out waiting for frame`; waypoint deleted → 3/3 red | **met** (5 sites found where the PRD named 2) |
| **R65** | phase3e-408 item 2 | `5071389`, `202e1ba` | settled `theFakeAgentReceivedExactlySigwinchSignals` over all seven sites; `DECK_GODOG_PATHS` selector | yes | both directions: `received 2 … want exactly 1`, `received 1 … want exactly 0` | **FAILED** — assertion criteria met, field symptom recurred at `preview.feature:134`/`:147` in task 021/022 runs |
| **R66** | phase3e-findings §4b | `ce8ef91` | `internal/theme/matrix_status_quantization_test.go`: `TestMatrixStatusTokensQuantiseToSevenDistinctReferenceEntries`; two pinned entries reconciled | yes | authored hexes restored → `5 distinct … want 7`, **while the true-colour test still passes** | **met** |
| **R67** | — | `b848d28`, `300ee86`, `e47cb35` | eleven `git mv` renames (441 test names identical before/after); two sections retitled; `citation-sweep.py` | no | sweep at parent `300ee86` → `exit=1`, `'Task 014' is ambiguous`, `'Requirement 19/21 correction' is dangling` | **met** |
| **R68** | #5 | `f3c25d5`, `7d060cd` | `internal/interactive/replydrain_test.go` (DA1/DSR/OSC 11 over a real tmux pane), `writestall_test.go` (×3) | yes | drain disabled → `DEADLOCK …` on the test's **own** deadline; `grid.go`→`f3c25d5` → wedge tests red; `s.writes` stripped → `DATA RACE` | **met** |
| **R69** | #6 | `b8f2513`, `366dd78` | `reconcile_retained_corpse_test.go`, `haslivepane_test.go`, `retained_corpse_recovery_test.go` (×2); `@requirement-46-interactive-fitted-geometry`, `@requirement-22-undo-toast` runs | yes | four separate reverts, each red with the mechanism named; requirement 46's adoption test stays green | **met** |
| **R70** | #9 | `0745ced` | `resume_crash_verdict_test.go`, `resumed_crash_verdict_test.go` (`SessionStart`, `UserPromptSubmit`), `lease_test.go` addition | yes | the two `NULL` clears reverted → all four subtests red on post-launch behaviour | **met** |
| **R71** | #8 | `88742b2`, `63d4189`, `9d43a32` | `resume_archived_refusal_test.go` (×2), `unarchive_test.go` (×2), `archived_resolution_test.go`, `archive_test.go` addition; `@requirement-33-unarchive-from-filter-results` | yes | leg 1 guard stashed; `m.baseSessions` naive `U`; one-reload; `ListSessions` restore; `deleted_at` dropped — five reds, both `WHERE` halves pinned | **met** |
| **R72** | #10 | `10f3970`, `4822484`, `eb2089e` | `archive_confirm_test.go` (×4); `@requirement-27-archive-confirm-writes-nothing`, `…-kills-and-archives`, `@requirement-27-archive-undo-toast`, `@requirement-33-archive-round-trip` | yes | pre-R72 `case "A"` → unit reds **plus a pre-existing caller** of the repaired step red; toast wiring → `did not show "Killed and archived"` | **met** |
| **R73** | #7 | `2714d1b`, `4edbfc2`, `9c2e66a` | `overlay_line_scroll_test.go` (×3×3 overlays), `overlay_wheel_scroll_test.go` (×6), six help lines pinned by two existing tests incl. the real-PTY one | yes | four mutations: wheel block deleted, guard loosened, page step, help lines removed — opposed pairs both red | **met** |

**Phase verdict: ten of eleven requirements met; R65 recorded as FAILED** on its
field-symptom claim, with the residual mechanism root-caused, its restructure specified,
and its counts explicitly not re-baselined. Two flakes remain open and are **not** claimed
fixed: `preview.feature:134`'s pre-resize fit race (1 in 10 whole-suite runs here, ~1 in 4
in task 017) and `TestGoldenMinimumFrame`'s "frame kept changing" recurrence. Deliverable
runs: whole suite green at **`e47cb35`** ([log](phase3f-021-fullsuite/go-test-p1-count1-all.log)),
stability **9/10** at **`c12c30e`** ([logs](phase3f-022-stability10/)). Findings that are
not requirement failures — the four other built-in themes' quantisation collisions,
`inject.go:51`'s `Exists` check, `vt`'s unsynchronised `Emulator.closed`, the five
`Task 034` sections, and the PRD's stale citations at `:467-479`/`:480` (protected, not
edited) — belong to `docs/reports/phase3f-findings.md`.
