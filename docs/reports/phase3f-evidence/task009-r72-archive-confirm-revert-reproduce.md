# Task 009 (R72, issue #10) — revert and reproduce

Requirement: `A` opens a confirm dialog and writes nothing until confirmed.
Base sha for this work: `9d43a32` (task 008). Toolchain: `ci/run.sh` (image
`deck-ci:local`), all commands run in a sibling container.

## What was reverted

`internal/tui/tui.go` was copied to `/tmp/tui.go.orig` and its `case "A":`
branch mutated **in place** to a *plausible naive* implementation — the
pre-R72 behaviour the issue reports, i.e. archive on the keypress itself with
no confirm:

```go
		case "A":
			// NAIVE (revert-and-reproduce only): the pre-R72 behaviour --
			// archive on the keypress itself, no confirm, nothing asked.
			if m.archiveSvc == nil || len(m.sessions) == 0 { ... }
			session := m.sessions[m.selected]
			return m, func() tea.Msg {
				return sessionArchived{session: session, err: m.archiveSvc(context.Background(), session)}
			}
```

Everything else (the dialog state, `updateArchiveConfirm`,
`archiveConfirmView`/`archiveConfirmBody`, the View/mouse/keymap wiring) was
left in the tree, so this is a behaviour revert and not a build break:
`ci/run.sh go build ./...` printed `BUILD_OK` with the naive branch in place.

## RED — unit half

```
== loadavg: 5.39 4.08 3.61 3/4128 10169
== RED: ci/run.sh go test -count=1 -run Archive ./internal/tui/
--- FAIL: TestArchiveKeyOpensAConfirmAndWritesNothing (0.00s)
    archive_confirm_test.go:67: A issued a command on the keypress itself (tui.sessionArchived from running it) -- it must write nothing until confirmed
--- FAIL: TestArchiveConfirmSubmitArchivesExactlyTheRowItNamed (0.00s)
    archive_confirm_test.go:105: Enter inside the archive confirm issued no command
--- FAIL: TestArchiveConfirmSuppressesTheBareLetterKeymap (0.00s)
    archive_confirm_test.go:170: "A" inside the archive confirm issued a command (tui.sessionArchived)
--- FAIL: TestArchiveConfirmFailedSubmitStaysOpenAndSaysWhy (0.00s)
    archive_confirm_test.go:224: a failed archive closed the confirm dialog
FAIL
FAIL	github.com/n-orlov/deck/internal/tui	0.004s
FAIL
```

## RED — scenario half, including a PRE-EXISTING caller of the repaired step

The third tag below (`features/filter.feature:49`) is one of the two
pre-existing callers of `clientArchivesSelectedSession`. It goes RED under the
naive product code, which is the proof that the repaired step really drives
the dialog through the keymap instead of calling the service or
auto-confirming: with no dialog on screen the step never finds
`Archive <session>` and times out.

```
== loadavg: 6.20 4.31 3.69 2/4173 10219
== RED: the new scenarios plus one pre-existing caller of the repaired step
    after scenario hook failed: timed out waiting for frame "Archive filter-archived-row": context deadline exceeded
    after scenario hook failed: timed out waiting for frame "Archive archive-confirm-live": context deadline exceeded
    after scenario hook failed: timed out waiting for frame "Archive archive-confirm-submit": context deadline exceeded
--- Failed steps:
      Error: after scenario hook failed: timed out waiting for frame "Archive filter-archived-row": context deadline exceeded
      Error: after scenario hook failed: timed out waiting for frame "Archive archive-confirm-live": context deadline exceeded
      Error: after scenario hook failed: timed out waiting for frame "Archive archive-confirm-submit": context deadline exceeded
3 scenarios (3 failed)
33 steps (6 passed, 3 failed, 24 skipped)
--- FAIL: TestFeatures (19.90s)
    --- FAIL: TestFeatures/the_filter_is_the_only_route_back_to_a_row_hidden_by_A_archive (6.75s)
        suite.go:640: after scenario hook failed: timed out waiting for frame "Archive filter-archived-row": context deadline exceeded
    --- FAIL: TestFeatures/A_on_a_running_session_opens_a_confirm_that_names_the_kill_and_writes_nothing_until_it_is_confirmed (6.55s)
        suite.go:640: after scenario hook failed: timed out waiting for frame "Archive archive-confirm-live": context deadline exceeded
    --- FAIL: TestFeatures/confirming_the_archive_dialog_lands_both_the_kill_and_the_archived_flag (6.56s)
        suite.go:640: after scenario hook failed: timed out waiting for frame "Archive archive-confirm-submit": context deadline exceeded
FAIL
FAIL	github.com/n-orlov/deck/features	19.915s
FAIL
```

## Restore, then GREEN

```
$ cp /tmp/tui.go.orig internal/tui/tui.go && diff /tmp/tui.go.orig internal/tui/tui.go
RESTORED_IDENTICAL
$ git status --short
 M features/kill_delete_undo.feature
 M features/kill_delete_undo_test.go
 M internal/tui/tui.go
?? internal/tui/archive_confirm_test.go
```

(the three modified paths and the new test file are this task's own change —
no stray experiment left behind)

```
== loadavg: 4.81 4.15 3.66 2/4154 10274
== GREEN (restored): ci/run.sh go test -count=1 ./internal/tui/
ok  	github.com/n-orlov/deck/internal/tui	0.637s
== GREEN (restored): the new scenarios plus every caller of the repaired step
ok  	github.com/n-orlov/deck/features	7.757s
```

The GREEN feature run's tag set was, in full:

```
@requirement-27-archive-confirm-writes-nothing,
@requirement-27-archive-confirm-kills-and-archives,
@requirement-33-filter-reaches-archived-row,
@requirement-33-unarchive-from-filter-results,
@requirement-32-event-log-newest-first,
@requirement-27-archive-stopped,
@requirement-27-archive-kill-and-archive,
@requirement-29-archive,
@requirement-29-kill-and-archive
```

i.e. the two new scenarios plus **every** scenario that calls
`clientArchivesSelectedSession` (`features/event_log.feature:15`,
`features/filter.feature:49` and `:67`, `features/kill_delete_undo.feature`'s
four archive scenarios). `cmd/...` (the PTY help window) and `internal/tui`
were also run green after the help line for `A` grew by one line: rendered
help is now **230** lines at `100x260`, re-measured the way
`cmd/deck/main_test.go:395` documents, still well under that test's 260-row
window.

## Not done here (scope)

The success toast with `u` to undo via `UnarchiveSession` is task 010, and the
end-to-end round trip is task 011. `A` was **not** rebound and no chord was
added (both declined by the operator).
