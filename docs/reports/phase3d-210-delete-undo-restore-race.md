# Task 210 — root-caused and fixed a NEW failure surfaced by task 217's own
attempt-7 whole-suite run: `undoing a delete leaves its adversarially-seeded
cwd fingerprint unchanged` (`features/kill_delete_undo.feature:204`)

## How this was found

Not in 209's own taxonomy (it is a genuinely new survivor). Found while
re-running a single clean `ci/run.sh go test -p=1 -count=1 ./...` at the
current tip (`ea44a30`, which is `3a26ccb` plus a docs-only commit) in
preparation for 210's own part (d) — re-declaring task 217's final code
commit. That single run showed 285/287 scenarios passing, with 2 failures:
this one, and a second one (a "leading tilde expands for scanning" timing
race, see the "second failure" section below — investigated separately,
found NOT to reproduce at low load and left as-is per steer 019 §2's own
decision rule, same disposition as 210's earlier part (c)).

## Root cause

The scenario's original text:

```gherkin
    When deck client "A" presses dd
    And deck client "A" submits the open dialog
    And deck client "A" presses u
    Then the state database session "fp-delete-undo-session" is not tombstoned
    And the directory "fp-delete-undo" still matches fingerprint "before-delete-undo"
```

`the state database session ... is not tombstoned` (`stateDatabaseSessionIsNotTombstoned`,
`features/kill_delete_undo_test.go:445`) is a **one-shot direct DB read** —
no polling, no wait. `deck client "A" presses u` (`clientPressesUndo`) only
sends the keystroke and returns; it does not wait for anything.

Pressing `u` triggers `internal/tui.Model`'s `u` handler
(`internal/tui/tui.go:2062`), which calls `Service.Restore` as an **async**
`tea.Cmd`: `store.RestoreSession` (clears `deleted_at`) → `Audit.Transition`
→ `store.GetSession`, then the `sessionRestored` message triggers
`m.loadSessions`. This is a real round trip through the deck process's own
event loop, tmux/PTY transport, and SQLite — not instantaneous.

The scenario's one-shot DB check races this round trip: if the check runs
before `RestoreSession` has actually landed, it reads the still-tombstoned
row and fails with `"has deleted_at=<ms>, want not tombstoned"` — exactly
the observed failure. Isolated at LOW host load (1-min loadavg 0.9–3.2, no
spike), this reproduced **3/10 and then 6/10** across two separate 10-run
batches (see `/tmp/210work/two-new-failures-10x.log` and
`/tmp/210work/two-failures-postfix-10x.log`, both container-local scratch
logs) — i.e. this is a real, load-independent defect in the SCENARIO, not
host contention (unlike the ghost-completion race task 210 part (c) already
ruled out that way).

**The sibling scenario one screen up in the same file**,
`u restores a deleted row inside its DECK_DELETE_GRACE_MS window`
(`features/kill_delete_undo.feature:116`), does NOT have this race, because
it inserts exactly the missing synchronization: after pressing `u` it does
`Then deck client "A" screen contains "dd-undo-restores"` (a bounded 5s
poll on the client's own rendered screen, which only shows the row again
once `sessionRestored`'s `m.loadSessions` has actually reloaded it) BEFORE
checking the database directly. Isolated 10x at comparable load (1-min
loadavg ~2.5–2.7): **0/10 failures** (`/tmp/210work/sibling-restores-10x.log`).
That comparison is what pinned the race to the missing post-`u` sync, not
to the 200ms `DECK_DELETE_GRACE_MS` window itself (see "false lead" below).

## False lead this task discarded

The first hypothesis was that `u` was racing the OPPOSITE direction: sent
before the async `Delete()` round trip (kill + soft-delete) had completed
and the grace-window's own `deleteUndoGeneration` tick had even started —
i.e. `u` arriving as a no-op before `undoSessionID` was set, then genuinely
expiring 200ms later. The first fix attempt added a wait for the "press u
to undo" toast BEFORE sending `u` (that step is a legitimate belt-and-
suspenders addition, kept in the final diff, but it is not the fix). It did
NOT resolve the failure — an isolated 10-run batch with only that change
still showed 6/10 failures. This ruled out the pre-`u` hypothesis and
pointed at the post-`u` gap instead (confirmed by the sibling-scenario
comparison above).

## Fix

Add the same post-`u` synchronization the sibling scenario already has,
scoped to this scenario only:

```gherkin
    When deck client "A" presses dd
    And deck client "A" submits the open dialog
    Then deck client "A" screen contains "press u to undo"
    When deck client "A" presses u
    Then deck client "A" screen contains "fp-delete-undo-session"
    And the state database session "fp-delete-undo-session" is not tombstoned
    And the directory "fp-delete-undo" still matches fingerprint "before-delete-undo"
```

No product code changed — this is a test-harness synchronization gap, not
a deck defect (`Restore` behaves exactly as designed; the scenario just
checked its result before it could possibly have landed).

## Non-vacuousness

1. Isolated `-count=10` reruns of just this scenario (`Paths` scratch
   override in `features/godog_test.go`, reverted after every experiment,
   `git diff` on that file confirmed empty at the end) at low/moderate host
   load (1-min loadavg 0.9–3.2, `docker ps` confirmed only pre-existing
   non-self containers): **pre-fix 2/10 to 6/10 fail** across three separate
   batches; **post-fix 20/20 pass** (two independent 10-run batches).
2. `git stash` reverted only `features/kill_delete_undo.feature` (keeping the
   scratch `Paths` override), reran `-count=10`: **2/10 reproduced the
   identical failure** (`session "fp-delete-undo-session" has
   deleted_at=<ms>, want not tombstoned`). `git stash pop` restored the fix.
3. `git diff --stat` after `stash pop` showed exactly the two intended
   files; `features/godog_test.go`'s scratch `Paths` override was reverted to
   `[]string{"."}` and confirmed `git diff` empty on that file specifically.

## Second failure seen in the same attempt-7 clean-run measurement (NOT this task's fix)

`a leading tilde expands for scanning without being rewritten in the field`
(`features/create_cwd_ghost.feature:79`) failed once in the original
whole-suite run with `screen contains "starting"` timing out (row already
showing `running`) — the same shape as task 210 part (c)'s
ghost-completion race (SPEC §7's shell-only fast-forward promoting the
session before a frame ever paints "starting"). Isolated 10x at low host
load (1-min loadavg ~0.9–1.2, alongside this task's own fp-scenario
isolation runs): **0/10 reproductions**
(`/tmp/210work/two-new-failures-10x.log`,
`/tmp/210work/two-failures-postfix-10x.log`, `/tmp/210work/fp-scenario-*.log`
— all container-local). Per steer 019 §2's decision rule (already applied
by task 210 part (c) to an equivalent case): does not reproduce at low load
→ host-contention noise, not a defect; left exactly as written, no fix
made, stands as part of the stability population.

## Verification

`go build ./...`, `go vet ./...`, `gofmt -l $(git ls-files '*.go')`: clean.
`git status --short`: empty (after commit). Only
`features/kill_delete_undo.feature` carries a real diff;
`features/godog_test.go` is unmodified in the final commit.

## Remaining for task 210

This was an UNPLANNED discovery made while attempting 210's own part (d)
(re-declaring task 217's final code commit). Part (d) itself is still open:
this fix is a new code commit, so task 217's final-code-commit sha needs to
be re-declared once more at the tip that includes it, and `ci/stability.sh
10` collected fresh at that tip, per 210's own successCriteria.
