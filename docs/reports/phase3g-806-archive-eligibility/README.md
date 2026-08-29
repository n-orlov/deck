# Task 806 — one `A` eligibility definition, shared by footer and key handler

## Review finding 2 / R80 restated for `A`

Before this task, `A` had two separate eligibility functions in
`internal/tui/tui.go`:

- a footer-named predicate (`session.ArchivedAt == 0`), consulted only by the
  footer's curated A/U slot (`footerLegend`'s `A` entry);
- `canArchive`, unconditionally `true`, consulted only by `case "A"` in the
  key-handler switch inside `Model.Update`.

That is exactly the parallel-definition shape finding 2/R80 forbids: the
footer already refused to *show* `A` for an already-archived row
(`TestFooterKeyLegendReflectsEligibility`'s "the eligible one of A/U shows,
never both" subtest, `internal/tui/footer_legend_test.go`), but pressing `A`
on that same row still opened the confirm dialog, because the handler's
predicate never looked at `ArchivedAt` at all.

## The fix (final state)

- There is now exactly **one** predicate, `canArchive`
  (`session.ArchivedAt == 0`), in the same family as `canKill`,
  `canUnarchive`, `canResume`, `canRestart`, `canDelete`, `canReachPane`.
  The old footer-scoped name is gone from `internal/`, and so is the
  always-true `canArchive` the handler used to call — the surviving name
  belongs to the action, not to one of its two callers:

  ```
  $ grep -rn footerArchiveEligible internal/ ; echo "exit=$?"
  exit=1        # no match anywhere under internal/ (product or test)
  ```

  The name survives only in this report and in `phase3g-014-footer-fixed-set`'s
  report, i.e. in history.
- `footerLegend`'s `A` entry: `footerRowEligible(m, false, canArchive)`.
- `case "A"` calls `canArchive(m.sessions[m.selected])` **by name** before
  opening the confirm dialog; on refusal it sets
  `m.attachError = "Cannot archive: session is already archived; press U to
  unarchive"` and returns — nothing is written, `archiveConfirming` stays
  false, and the message names `U` as the route back.
- `canArchive`'s doc comment does not say "footer-only" (it says the
  opposite, and says why the name carries no "footer" in it).

## Tests (`internal/tui/archive_eligibility_test.go`, new file)

- `TestArchiveKeyOnAnArchivedRowRefusesAndNamesU` — the behavioural half:
  presses `A` on an already-archived row and asserts no `tea.Cmd` is issued,
  the archive service is never called, `archiveConfirming` stays `false`, and
  `attachError` both states the row is already archived and names `U`.
- `TestArchiveKeyHandlerCallsTheFooterPredicateByName` — the structural half,
  and the one that enforces the *shared* definition. It reads the predicate
  name out of `footerLegend`'s own `A` entry (via
  `footer_bindings_parity_test.go`'s `parseFooterLegendSource`, the same live
  source extraction the footer's parity test already trusts) and then, from
  the `case "A":` arm of the list-mode key switch (anchored exactly as
  `help_keymap_parity_test.go`'s `listModeBoundKeys` anchors that switch, with
  `//` comments stripped so a comment cannot satisfy it), asserts that the arm
  **calls that function** and contains **no `ArchivedAt` reference of its
  own**. It also fails if the shared predicate's name contains "footer", if
  `footerPredicateByName` cannot resolve it, or if its doc comment calls it
  "footer-only".
- `TestArchiveKeyEligibilityMatchesFooterPredicate` — the value-level check:
  `canArchive` accepts an unarchived row and refuses an archived one.

Why the structural test is necessary: an inlined `ArchivedAt != 0` inside
`case "A"` is *behaviourally identical* to calling `canArchive` today, so no
behavioural assertion can distinguish the two — and "identical today" is
precisely the state finding 2 rejects, because it is what lets the footer and
the handler drift apart tomorrow. The first pass of this task (commit
`41ae9cd`) shipped only behavioural assertions, and independent validation
demonstrated exactly that hole by inlining the comparison with both tests
still green. The source parse closes it.

## Mutation demonstration (red before revert, green after)

Both mutations were applied live on top of this task's own (at the time
uncommitted) fix, then fully reverted; `git diff` was inspected before
committing to prove no mutation text survives.

**Mutation A — the handler stops consulting the shared predicate but keeps
identical behaviour** (the hole validation found in the first pass):

```
-			if !canArchive(m.sessions[m.selected]) {
+			if m.sessions[m.selected].ArchivedAt != 0 {
 				m.attachError = "Cannot archive: session is already archived; press U to unarchive"
 				return m, nil
 			}
```

- Red: `ci/run.sh go test -count=1 -v -run 'TestArchive' ./internal/tui/`
  → exit **1**, `red-mutation-inlined-copy.log` /
  `.log.exitstatus`. Exactly one test fails —
  `TestArchiveKeyHandlerCallsTheFooterPredicateByName`, with the offending
  `case "A"` arm quoted in the failure — while every behavioural assertion
  passes, which is the point being demonstrated.

**Mutation B — the handler drops the check entirely** (the pre-806 defect
shape, an always-true predicate):

```
-			if !canArchive(m.sessions[m.selected]) {
-				m.attachError = "Cannot archive: session is already archived; press U to unarchive"
-				return m, nil
-			}
 			m.archiveConfirming = true
```

- Red: `ci/run.sh go test -count=1 -v -run 'TestArchiveKey' ./internal/tui/`
  → exit **1**, `red-mutation-no-check.log` / `.log.exitstatus`.
  `TestArchiveKeyOnAnArchivedRowRefusesAndNamesU` fails ("A on an archived row
  opened the archive confirm dialog") and so does the structural test.

**Green after both reverts:**
`ci/run.sh go test -count=1 -v -run 'TestArchive|TestFooter' ./internal/tui/`
→ exit **0**, `green-mutations-reverted.log` / `.log.exitstatus` (zero `FAIL`
lines; the footer legend and footer/bindings parity tests are included in the
same run).

`red-mutation.log` and `green-mutation-reverted.log` are the first pass's
(commit `41ae9cd`) mutation-B-only demonstration, kept as the record of that
pass.

## Required suite runs (this task's own criteria)

- `ci/run.sh go test -count=1 ./internal/tui/` → exit 0,
  `full-tui-package.log` / `full-tui-package.log.exitstatus`.
- `ci/run.sh env DECK_GODOG_PATHS=kill_delete_undo.feature,filter.feature go test ./features/ -run TestFeatures -count=1`
  → exit 0, `features-kill_delete_undo-filter.log` /
  `features-kill_delete_undo-filter.log.exitstatus`.

## What the two existing footer tests did and did not need

`features/testdata/golden/side_by_side_80x24.golden` is untouched (the footer
renders identically — only a Go identifier changed).

`internal/tui/footer_legend_test.go` and
`internal/tui/footer_bindings_parity_test.go` keep **every assertion, case,
row and expectation exactly as they were**; they stay green unedited in that
sense, and the first pass left them byte-identical. This pass changes one
identifier in each, and only because removing the footer-scoped name from
`internal/` (this task's explicit `grep` criterion) makes it compulsory:

- `footer_bindings_parity_test.go:137` — the `footerPredicateByName` map is
  keyed by the predicate name parsed out of `tui.go`'s own `footerLegend`
  entry, so the entry for `A` becomes `"canArchive": canArchive`. Leaving the
  old key would not compile (the function no longer exists) and would break
  the parity test's own lookup.
- `footer_legend_test.go:75` — one word inside a comment ("the two entries
  share `canArchive`/`canUnarchive`"), so the file does not name a function
  that no longer exists.

Neither edit relaxes anything: both files were run in the green log above and
pass unchanged in behaviour.

## Scope note

`x`/`canKill` (the other half of review finding 2's original framing) was
already reviewed and found honest at task-list time: the footer uses
`canKill`, the single-row `x` handler deliberately skips it and defers to the
service's own verdict (a documented, intentional asymmetry, not a parallel
redefinition), and `kill_delete_undo.feature`'s wording was left untouched.
This task addresses only the `A` half, which is what still had a real
parallel-definition defect.
