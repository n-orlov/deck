# Task 105 — F18: rename dialog's focused field gets the Selection background

**Requirement**: SPEC.md:1355's mapping says "the focused field carries the
same selection treatment a selected list row does". Task 016 already applied
this to the create modal's currently-focused row
(`renderCreateRowSegments`, `internal/tui/tui.go`). F18 (findings report)
recorded that `internal/tui/rename.go` had 3 `colorToken` calls but no
`theme.Selection` background anywhere — the rename sub-dialog's single "New
name" field (always the focused field: there is nothing else to tab to while
`m.renaming` is true) never got the selection cue.

## Fix

`internal/tui/rename.go`'s `styledRenameBody` used to render the field via
the shared `detailField` helper (`tui.go`), whose own doc comment says
plainly it self-resets each half and is "never composed under a shared
selection background" — by design, since `detailView`/`pinView`/
`profileSwitchView` never need it. A new function, `renderRenameFieldRow`,
composes the label ("New name:  ", `theme.Hint`) and value (`m.renameValue`,
`theme.Text`) via `settingsRenderRowOpen` (`settings.go`, opens each
segment's foreground but never closes it) and wraps the whole result in
exactly one `bgColorToken(theme.Selection, ...)` — the same shape
`renderCreateRowSegments` uses for the create modal's focused row. A
per-segment `colorToken` reset would clear the outer Selection background
the instant the label's own text ended (`foregroundSGR`'s own doc comment,
`theme_color.go`) — that is exactly the defect this task closes.

`styledRenameBody` now calls `m.renderRenameFieldRow()` instead of
`m.detailField("New name:  ", m.renameValue)`. `detailField` itself is
untouched — `detailView`/`pinView`/`profileSwitchView` still use it exactly
as before.

## Evidence: red before, green after

New assertion: `TestRenameViewFocusedFieldGetsSelectionBackground`
(`internal/tui/rename_theme_test.go`), reading the rendered dialog off a
real `vt.Emulator` grid (same technique as
`TestCreateViewFocusedFieldGetsSelectionBackground`).

**Red** — `rename.go` reverted to the pre-fix `detailField` call
(`git stash -- internal/tui/rename.go`, test kept):

```
=== RUN   TestRenameViewFocusedFieldGetsSelectionBackground
    rename_theme_test.go:127: focused New name row label background = "" ok=false, want selection token #26324b
--- FAIL: TestRenameViewFocusedFieldGetsSelectionBackground (0.00s)
FAIL
FAIL	github.com/n-orlov/deck/internal/tui	0.004s
FAIL
```

**Green** — fix restored:

```
=== RUN   TestRenameViewFocusedFieldGetsSelectionBackground
--- PASS: TestRenameViewFocusedFieldGetsSelectionBackground (0.00s)
PASS
ok  	github.com/n-orlov/deck/internal/tui	0.004s
```

## Visible text unchanged

The pre-existing `TestRenameStyledBodyMatchesPlainBodyOnceStripped`
(`rename_theme_test.go`, task 021) already asserts, line for line, that
`styledRenameBody()` is byte-identical to `wrapDialogLines(renameBody())`
once every escape sequence is stripped — for the prefilled, typed and
failure-note cases. It stays green with this change (label/value plain text
is unchanged; only the SGR wrapping around it changed), so this task did not
need a second copy of that assertion — it is the same "byte-identical after
ANSI stripping" guarantee the success criteria ask for, already exercised by
that test and re-verified below.

## Full `internal/tui` run

`ci/run.sh go test -count=1 ./internal/tui/` — exit 0 (see validation log
alongside this task's commit).

## `git show` diff of rename_test.go

`rename_test.go` (the file with the pre-existing rename assertions this task
must only add to, never loosen) is untouched by this task: all edits landed
in `rename.go` (product code) and `rename_theme_test.go` (a NEW test file
added by task 021, extended here). `git show --stat <this task's commit> --
internal/tui/rename_test.go` shows no changes.
