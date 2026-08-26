# Task 010 (R72, issue #10) — archive success toast + `u` undo: revert-and-reproduce

Question asked of the new scenario `@requirement-27-archive-undo-toast`
(`features/kill_delete_undo.feature:516`): **would it go red if the toast wiring
were reverted?**

## Method

`internal/tui/tui.go` was copied to `/tmp/tui.go.orig`, then the `sessionArchived`
success branch was mutated *in place* to the plausible naive implementation — the
pre-task body, which reloads the list and offers nothing:

```go
	case sessionArchived:
		...
		m.archiveConfirming = false
		m.archiveNote = ""
		m.attachError = ""
-		m.archiveUndoSessionID = msg.session.ID
-		m.archiveUndoSessionName = msg.session.Name
-		m.archiveUndoKilled = msg.session.Status != "stopped"
-		m.archiveUndoGeneration++
-		archiveGeneration := m.archiveUndoGeneration
-		return m, tea.Batch(m.loadSessions, tea.Tick(m.settings.Undo, func(t time.Time) tea.Msg { return archiveUndoExpired(archiveGeneration) }))
+		return m, m.loadSessions
```

Everything else (the third trio's fields, `archiveUndoExpired`,
`archiveUndoNoteLines`, the `u` fall-through, the reservation in
`computeLayout`) was left in place, so this reverts exactly the wiring the task
adds and nothing else. The file was restored with `cp` afterwards and `diff`
against `/tmp/tui.go.orig` plus `git status --short` confirmed the tree clean of
the mutation.

`/proc/loadavg` at each run is recorded below.

## Reverted: RED

Command (loadavg `3.18 3.17 3.22`):

```
ci/run.sh env DECK_GODOG_TAGS='@requirement-27-archive-undo-toast' \
    go test -count=1 ./features/ -run TestFeatures
```

exit status 1. Full output: `task010-r72-archive-undo-toast-revert-reproduce.log`
(ANSI stripped, the pty `raw:` timeline dropped). The failing step and summary,
quoted verbatim:

```
--- Failed steps:

  Scenario: a confirmed archive says what happened and u puts the row back in the default list # kill_delete_undo.feature:517
    And deck client "A" screen contains "Killed and archived" # kill_delete_undo.feature:533
      Error: after scenario hook failed: client "A" did not show "Killed and archived" within 5s: timed out waiting for frame "Killed and archived": context deadline exceeded

1 scenarios (1 failed)
13 steps (5 passed, 1 failed, 7 skipped)
```

The steps that follow it (`u`, then the row's return to the default list) were
skipped, so the scenario is red on the toast itself and would be red again on
the undo: the naive branch leaves `archiveUndoSessionID` empty, and `u` with an
empty archive trio is a no-op that never reaches `UnarchiveSession`.

## Restored: GREEN

Same command, restored tree (loadavg `3.90 3.32 3.27`), run together with every
other archive/unarchive scenario so the toast cannot have broken one of them:

```
ci/run.sh env DECK_GODOG_TAGS='@requirement-27-archive-undo-toast,@requirement-27-archive-confirm-kills-and-archives,@requirement-27-archive-confirm-writes-nothing,@requirement-27-archive-stopped,@requirement-27-archive-kill-and-archive,@requirement-33-unarchive-from-filter-results,@requirement-33-filter-reveals-archived' \
    go test -count=1 ./features/ -run TestFeatures
ok  	github.com/n-orlov/deck/features	4.594s
```

Vacuity check (notes' gotcha: godog matches tags exactly, so a mistyped tag is
green but runs nothing): a deliberately absent tag returns in `0.019s`, the new
tag alone in `0.632s`, and the seven-tag set in `4.594s` — the scenario really
executes.

The undo trios and the `u` key's pre-existing behaviour were re-run separately
(loadavg `3.33 3.22 3.23`), since `u` now has a third window behind the four it
already had:

```
ci/run.sh env DECK_GODOG_TAGS='@requirement-23-delete-undo,@requirement-29-kill-undo,@requirement-29-delete-undo,@requirement-29-batch-undo,@requirement-2-monotonic-windows,@requirement-28-mark-bulk-actions,@requirement-29-archive,@requirement-29-kill-and-archive' \
    go test -count=1 ./features/ -run TestFeatures
ok  	github.com/n-orlov/deck/features	9.841s
```

## Unit tests

`internal/tui/archive_undo_test.go` (new, loadavg `2.80 3.11 3.20`):

```
ci/run.sh go test -count=1 ./internal/tui/ ./cmd/deck/
ok  	github.com/n-orlov/deck/internal/tui	0.574s
ok  	github.com/n-orlov/deck/cmd/deck	5.162s
```

`TestArchiveUndoWindowExpiresLikeItsSiblings` is the expiry assertion the
requirement names: a stale `archiveUndoExpired` generation cannot clear a newer
window, the matching one clears the trio and the toast, expiry issues **no**
command (unlike `deleteGraceExpired`, nothing is reaped), and `u` afterwards is
a no-op. `TestArchiveUndoIsCheckedBehindTheKillAndDeleteUndoTrios` pins the
ordering risk of a third window on one key: with all three outstanding, `u`
resumes, then restores, and only then unarchives.

## Wording notes (deliberate, not incidental)

- The toast never names the session: `@requirement-27-archive-confirm-kills-and-archives`
  asserts the archived name is gone from the **whole screen**, which a toast
  quoting it back would violate. `deleteUndoNoteLines` already refuses to name a
  deleted row for the same reason; a unit assertion pins it here too.
- It says `press u to unarchive`, not `press u to undo`, and adds
  `(agent stays stopped)` on the kill-and-archive path: `u` reverses
  `archived_at` via `UnarchiveSession` only. A bare "undo" would promise the
  killed agent back, which is a false behaviour claim, not a wording nit.
