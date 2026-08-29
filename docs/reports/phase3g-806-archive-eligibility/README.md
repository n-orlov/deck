# Task 806 — one `A` eligibility definition, shared by footer and key handler

## Review finding 2 / R80 restated for `A`

Before this task, `A` had two separate eligibility functions in
`internal/tui/tui.go`:

- `footerArchiveEligible` (`session.ArchivedAt == 0`), consulted only by the
  footer's curated A/U slot (`footerLegend`'s `A` entry).
- `canArchive` (unconditionally `true`), consulted only by `case "A"` in the
  key-handler switch inside `Model.Update`.

That is exactly the parallel-definition shape finding 2/R80 forbids: the
footer already refused to *show* `A` for an already-archived row
(`TestFooterKeyLegendReflectsEligibility`'s "the eligible one of A/U shows,
never both" subtest, `internal/tui/footer_legend_test.go`, unedited by this
task), but pressing `A` on that same row still opened the confirm dialog,
because `canArchive` never looked at `ArchivedAt` at all.

## The fix

- Deleted `canArchive`.
- `case "A"` in `internal/tui/tui.go`'s key-handler switch now calls
  `footerArchiveEligible` — the same function `footerLegend`'s `A` entry
  already called via `footerRowEligible(m, false, footerArchiveEligible)`
  (`internal/tui/tui.go:3685`, unchanged) — before opening the confirm
  dialog.
- `footerArchiveEligible`'s doc comment no longer says "footer-only"; it now
  states plainly that both the footer and the key handler consult this one
  function.
- On refusal, `case "A"` sets `m.attachError = "Cannot archive: session is
  already archived; press U to unarchive"` and returns without setting
  `m.archiveConfirming` — nothing is written, no confirm opens, and the
  message names `U` as the route back.

```
$ grep -rn footerArchiveEligible internal/
internal/tui/footer_legend_test.go:75:  # comment only, unedited by this task
internal/tui/footer_bindings_parity_test.go:137:  # map entry, unedited by this task
internal/tui/tui.go:1588,1596,2579,2584 (doc comment, definition, handler comment, handler call)
internal/tui/tui.go:3685  # unchanged footerLegend entry
```

No match outside `internal/tui/`.

## New test

`internal/tui/archive_eligibility_test.go` (new file — `footer_legend_test.go`
and `footer_bindings_parity_test.go` are byte-for-byte unedited, confirmed by
`git diff --stat` showing no entry for either path):

- `TestArchiveKeyOnAnArchivedRowRefusesAndNamesU` — presses `A` on an
  already-archived row and asserts: no `tea.Cmd` is issued, the archive
  service is never called, `archiveConfirming` stays `false`, and
  `attachError` both states the row is already archived and names `U`.
- `TestArchiveKeyEligibilityMatchesFooterPredicate` — evaluates
  `footerArchiveEligible` directly across both states it distinguishes, as a
  second, more structural net.

## Mutation demonstration (red before revert, green after)

Per the standing evidence rule, the discriminating test was proven to fail
against a handler that stops consulting the shared predicate, using a live
mutation on top of this task's own (uncommitted, at the time of the trial)
fix — reverting `case "A"`'s eligibility check back to unconditionally
opening the confirm dialog (the exact shape of the original `canArchive`
defect this task removes):

```
-			if !footerArchiveEligible(m.sessions[m.selected]) {
-				m.attachError = "Cannot archive: session is already archived; press U to unarchive"
-				return m, nil
-			}
+			// MUTATION (task 806 red demonstration): handler no longer
+			// consults the shared predicate at all.
			m.archiveConfirming = true
```

- **Red** (mutated, predicate not consulted):
  `ci/run.sh go test -count=1 -run 'TestArchiveKeyOnAnArchivedRowRefusesAndNamesU|TestArchiveKeyEligibilityMatchesFooterPredicate' -v ./internal/tui/`
  → exit 1, logged at `red-mutation.log` / `red-mutation.log.exitstatus`.
- Mutation reverted exactly (verified against `git diff internal/tui/tui.go`
  before committing — no residual mutation text anywhere in the tree).
- **Green** (fixed code restored):
  same command → exit 0, logged at `green-mutation-reverted.log` /
  `green-mutation-reverted.log.exitstatus`.

## Required suite runs (this task's own criteria)

- `ci/run.sh go test -count=1 ./internal/tui/` → exit 0,
  `full-tui-package.log` / `full-tui-package.log.exitstatus`.
- `ci/run.sh env DECK_GODOG_PATHS=kill_delete_undo.feature,filter.feature go test ./features/ -run TestFeatures -count=1`
  → exit 0, `features-kill_delete_undo-filter.log` /
  `features-kill_delete_undo-filter.log.exitstatus`.

## Unchanged by this task (verified by `git diff --stat`)

- `internal/tui/footer_legend_test.go`
- `internal/tui/footer_bindings_parity_test.go`
- `features/testdata/golden/side_by_side_80x24.golden`

## Scope note

`x`/`canKill` (the other half of review finding 2's original framing) was
already reviewed and found honest at task-list time (see
`/run/ralphd/notes.md`'s "Review finding 2 (R80) facts" section): the footer
uses `canKill`, the single-row `x` handler deliberately skips it and defers
to the service's own verdict (a documented, intentional asymmetry, not a
parallel redefinition), and `kill_delete_undo.feature`'s wording was left
untouched. This task addresses only the `A` half, which is what still had a
real parallel-definition defect.
