# Task 210 part (c) — "a unique directory match ghosts in the dimmed token..." (2/10 in 209)

## Failure as observed in 209's taxonomy

`docs/reports/phase3d-209-stability/run-5.log` and `run-7.log` both show
`TestFeatures/a_unique_directory_match_ghosts_in_the_dimmed_token_and_right_accepts_it_with_a_trailing_slash`
(features/create_cwd_ghost.feature:14, `@requirement-14-ghost-unique-match-right-accepts`)
failing at its last two steps:

```
When deck client "A" submits the create modal
Then deck client "A" screen contains "starting"
```

The captured failure frame in both logs shows the session row already reading
`cwd-ghost-right-session run...` (i.e. **`running`**, truncated by column width) —
never `starting` — at the moment the 5s poll gave up. The ghost-completion part of
the scenario (the dimmed-token match, `right`, the completed path) is not implicated
at all; only the post-submit `"starting"` assertion fails.

## Hypothesis

This is the shell-only fast-forward rule in SPEC.md §7: `starting` → `running` the
moment the pane is alive, for `shell` rows only (`internal/service/reconcile.go`'s
`session.Agent == "shell" && session.Status == "starting"` promotion). This
scenario's create modal defaults to the `shell` agent (never changed by the
scenario). The promotion happens on the very next reconcile tick
(`scenarioReconcileInterval` = 250ms in this harness) once the launch's own
"user"→"tmux" source flip has landed. Two independent renders have to land in the
window between submit and that reconcile tick for the client's screen buffer to
ever show `"starting"` — under any contention that pushes both the deck process's
own scheduling and the harness's polling cadence, that window can close before a
frame painting `"starting"` is captured, and the shell session can appear to jump
straight from "not present" to `running`.

This is the same shape steer 019 §2 already established for two other tests in this
job (host contention plausibly explains an occasional ms/sub-second-scale timing
assertion miss) — not a guess to act on without the discriminating experiment.

## Reproduction attempt (low load)

Per this task's own successCriteria ("provably not reproducible at low load, 10
isolated runs each"), the scenario was isolated via a scratch `Paths` override in
`features/godog_test.go` (`[]string{"create_cwd_ghost.feature:14"}`, reverted
immediately after — `git diff` confirmed empty) and run 30x in a row (exceeds the
required 10):

```
ci/run.sh go test -count=30 -run TestFeatures -v ./features/
```

Host `uptime` 1-min loadavg at launch and completion: 0.58–0.60 (5-min ~1.3–1.6,
15-min ~3.1–3.4) — low and stable throughout, no docker/host contention.

**Result: 30/30 PASS.** Zero occurrences of the failure. Log:
`/tmp/210work/ghost-repro-30.log` (container-local scratch, per this run's existing
convention for scratch diagnostic logs cited by path).

## Resolution (per steer 019 §2's own decision rule, applied by analogy)

The failure does not reproduce at all in 30 isolated low-load runs, consistent with
a host-contention-sensitive render/reconcile-tick race rather than a defect in the
ghost-completion feature or the shell starting→running transition itself. No
product or test-harness fix is made here: this scenario is left exactly as written
(no sleep, no widened checkpoint, no retry, no tag, no assertion change) and stands,
unmodified, as part of task 210's stability population — the same disposition task
220 reached for its own two suspects.

Task 210's remaining item is (d): once every remaining suspect (parts 1, 2a, (b),
now (c)) has landed, re-declare task 217's final-code-commit sha (currently
`3a26ccb`, already the tip after part (b)) and run `ci/stability.sh 10` fresh at
that tip.
