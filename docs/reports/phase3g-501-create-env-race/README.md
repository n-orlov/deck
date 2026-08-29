# Task 501 — the Create new-session/set-environment race (crash.feature:46)

## Root cause

`internal/tmux.Client.Create` used to run three separate tmux client calls in
sequence: `new-session -d ...`, then one `set-environment -t <name> KEY VALUE`
per requested variable, then a final `list-panes -t <name>` (via the private
`session()` helper) to build the returned `Session`.

The PRD's own approach-04 root-cause pass (recorded only in this run's
non-tracked state, not citable here) attributed the flake to a raw tmux
timing quirk around an instantly-exiting pane, reproduced "1/300 under load".
This iteration went further and found the actual, much more reliable
trigger: **`internal/service/reconcile.go`'s own reconciliation loop runs
concurrently with any in-flight `Create` call**, and the moment its `List`
observes a dead pane it calls `Kill` on that very session
(`reconcile.go:183`) to collect the crash artifact — entirely independent of
tmux's own `remain-on-exit failed` handling (which only decides whether the
*pane* is retained, never whether deck's own reconciler leaves the *session*
alone). `crash.feature:46`'s scenario ("a failing pre_launch leaves visible
evidence without attaching") creates a session whose pane exits instantly
(`exit 1`), which is exactly the shape that gives the reconciler's next tick
a dead pane to react to while `Create` is still in the middle of its own
follow-up calls.

If the reconciler's `Kill` lands in the gap between `new-session` returning
and `Create`'s own later calls finishing, those later calls fail with `no
such session` (from `set-environment`) or `no current target` (from the
final `list-panes`), and `Create` aborts the whole create.

## Fix

`internal/tmux/tmux.go`'s `Client.Create` now mirrors the session's
environment via `new-session`'s own repeatable `-e KEY=VALUE` flag —
one flag per requested variable — instead of a separate `set-environment`
loop issued after `new-session` has already returned. This is available
because deck's documented minimum tmux is 3.2 and `-e` was added in tmux
3.0 (CI's tmux is 3.5a). The environment is now fully set **atomically with
session creation**, before the pane's own command has had any chance to run,
exit, and give a concurrent reconciler a dead pane to kill — there is no
longer a second, later call for that race to have a window in.

`Create`'s own final `list-panes` lookup (via the pre-existing `session()`
helper) still runs after `new-session` returns, so it is still theoretically
exposed to the same class of race if the reconciler's `Kill` lands in that
one remaining gap. Measured below (green-after, 300/300), this residual
window is negligible in practice: cutting three-or-more sequential
after-the-fact client calls down to exactly one removes essentially all of
the race's practical width. No test-side or product-side tolerance was added
for this: a genuine tmux failure at any call in `Create` still fails `Create`
exactly as before (see `TestCreateStillFailsOnGenuineEnvironmentError`).

**`launch.CWD` is untouched.** The `-c launch.CWD` argument keeps the exact
same value and the exact same position immediately before the `--` argv
terminator; `git show --stat` / the diff below only show it moving to a
later line in the `args` slice construction because the new `-e` flags are
inserted before it, never a change to the flag itself or to any session cwd
path:

```
-	args := []string{"new-session", "-d", "-s", name, "-c", launch.CWD, "--", "env"}
+	args := []string{"new-session", "-d", "-s", name}
+	for key, value := range pairs(env) {
+		args = append(args, "-e", key+"="+value)
+	}
+	args = append(args, "-c", launch.CWD, "--", "env")
```

## Evidence

### Red-before (unmodified pre-fix tree, HEAD `157bb52`)

`internal/tmux/reconciler_race_test.go`'s `TestCreateRacesConcurrentReconcilerKill`
plays `reconcile.go`'s own List-then-Kill-on-Dead shape in a tight polling
goroutine racing a real `Create` call whose command exits instantly — no
artificial CPU load needed; the two real deck actors (`Create`'s own
follow-up calls and the reconciler-shaped killer) are enough on their own.
Copied (test-only, no product-code change) into a `git worktree add
--detach .scratch-501 157bb52` checkout of the unmodified pre-fix tree and
run there:

- Exact command: `ci/run.sh sh -c 'cd .scratch-501 && go test -run
  TestCreateRacesConcurrentReconcilerKill -count=20 -v ./internal/tmux/'`
- Loop count: 20 (Go's own `-count=20`)
- Result: **18 of 20 failed**, exit status 1. Verbatim failure text (both
  shapes the same race produces, depending on exactly which of `Create`'s
  now-removed follow-up calls the reconciler's `Kill` lands ahead of):
  - `create session racing a concurrent reconciler kill: set environment for
    session "deck_reconciler_race": tmux -L deck-reconciler-race-... set-environment
    -t deck_reconciler_race DECK_RACE_B two: exit status 1: no such session:
    deck_reconciler_race`
  - `create session racing a concurrent reconciler kill: list panes for
    session "deck_reconciler_race": tmux -L deck-reconciler-race-... list-panes
    -t deck_reconciler_race -F ...: exit status 1: no current target`

  Full log: `01-red-before-pre-fix-tree.log` (command and exit status
  captured in the same shell call).

  A prior, much less targeted attempt at this same evidence — a bounded
  repeat loop of the literal pre-fix `new-session` + `set-environment`
  sequence via the plain tmux CLI (no deck code, mirroring `Client.Create`'s
  exact per-call sequence including a fresh `Bootstrap` every iteration),
  run for a combined ~19,000 iterations under heavy background CPU load
  (300+ `yes` spinners plus dozens of parallel `/bin/true` loops on a
  28-core sibling) — never reproduced the failure. That confirms the race is
  not really about raw tmux scheduling timing at all: it needs a second,
  concurrent deck *actor* (the reconciler) racing `Create`, which is exactly
  what `TestCreateRacesConcurrentReconcilerKill` supplies directly instead of
  waiting for it to happen by chance.

### Green-after (this fix, current HEAD)

- `ci/run.sh go test -count=20 -run TestCreateSurvivesInstantExitEnvironmentMirroring ./internal/tmux/`
  — exit 0. Log: `02-green-after-target-test-count20.log`.
- `ci/run.sh go test -run TestCreateRacesConcurrentReconcilerKill -count=300 ./internal/tmux/`
  — exit 0 (**300/300**, the same reconciler-race repro that failed 18/20 on
  the pre-fix tree). Log: `03-green-after-reconciler-race-count300.log`.
- `ci/run.sh go test -count=1 ./internal/tmux/`, three consecutive runs —
  exit 0 each time. Logs: `04-green-after-pkg-run1.log` .. `run3.log`.
- `ci/run.sh env DECK_GODOG_PATHS=crash.feature go test ./features/ -run TestFeatures -count=1`,
  three consecutive runs — exit 0 each time. Logs:
  `05-green-after-crash-feature-run1.log` .. `run3.log`.

## Tests added (`internal/tmux`)

- `create_env_race_test.go`:
  - `TestCreateSurvivesInstantExitEnvironmentMirroring` — the task's own
    literal ask: a session whose command exits instantly (`sh -c 'exit 1'`),
    asserts `Create` returns no error and every requested variable is still
    visible via `show-environment -t` on the session itself.
  - `TestCreateStillFailsOnGenuineEnvironmentError` — proves the fix added no
    tolerance broad enough to swallow a real failure: an invalid environment
    key still fails `Create`.
- `reconciler_race_test.go`:
  - `TestCreateRacesConcurrentReconcilerKill` — the red/green regression test
    for the actual mechanism above (a concurrent reconciler-shaped `Kill`
    racing `Create`'s own follow-up calls).
