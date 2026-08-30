# R84 — no allowlist, floor hard-enforced, pair table proved complete (task 1204)

`SPEC.md` §11.6, PRD `prds/phase3g-field-backlog.md` R84. Task 106/task 1204 both concern
`internal/theme/contrast_test.go`'s `TestThemedDialogTokensClearContrastFloor`: this task
removes the `dialogPairAllowlist` task 106 added and proves, with a tracked test, that the
resulting hard floor covers every pair a dialog can actually put on screen — no more, no less.

## What changed

`internal/theme/contrast_test.go`:

- `dialogPairAllowlist`, `dialogPairKey`, `checkDialogPair` and `dialogFocusedFieldTextTokens`
  are gone. `grep -rn dialogPairAllowlist internal/` returns nothing.
- The table `TestThemedDialogTokensClearContrastFloor` walks is now exactly:
  `hint/surface`, `key/surface`, `error/surface` (unchanged, `dialogSurfaceChecks`) plus
  `hint/selection` and `text/selection` (`dialogSelectionChecks`, new — `text/selection` is
  already hard-enforced elsewhere by `TestBuiltinContrastFloor`'s own `contrastChecks`, and is
  repeated here so this table is self-contained).
- Every cell in that table is `t.Errorf` the instant it drops below 3.0:1, for every built-in,
  with no allowlist, tolerance table or `t.Logf`-only branch anywhere in the file.
- `theme.SelectionIdle` and the `dimmed`/`key`/`error` tokens are dropped from this table
  entirely — not because they clear the floor, but because no dialog draws them there. See
  `dialogSelectionTokens`'s own doc comment in `contrast_test.go` for the citation
  (`panel.go:134-139`, `settings.go:1318`) and why the excluded sub-floor cells (several,
  including empire's `SelectionIdle` quantisation collapse) are a finding for task 1208, not a
  gap in this floor.

`internal/tui/dialog_selection_floor_completeness_test.go` (new): the tracked completeness
proof. `TestDialogSelectionRenderersComposeOnlyFloorTokens` parses every non-test `.go` file in
`internal/tui` and, by AST inspection alone:

1. Finds every call of the exact shape `<recv>.bgColorToken(theme.Selection, ...)` and asserts
   there are precisely two, each directly inside a named function, and that those two names are
   exactly `renderCreateRowSegments` (`tui.go`) and `renderRenameFieldRow` (`rename.go`) — a
   third site, or either of these two disappearing, fails the test.
2. For `renderRenameFieldRow`, walks its own body; for `renderCreateRowSegments` (which receives
   `segs` as a parameter rather than building it), walks the innermost enclosing
   function/closure of each of its call sites — collecting every
   `settingsRowSegment{Tok: theme.X, ...}` literal's token, in both its explicitly-typed and
   the elided-type form a `[]settingsRowSegment{{...}}` slice literal uses.
3. Fails if any collected token is not in `dialogSelectionFloorTokens` (`{Hint, Text}`), the
   tui-side mirror of `contrast_test.go`'s `dialogSelectionTokens`.

Run today, it finds exactly `map[Hint:true Text:true]` — matching the floor table above.

`internal/tui/tui.go` and `internal/tui/create_view_theme_test.go`: two doc comments that named
the now-deleted `dialogPairAllowlist`/`dialogFocusedFieldTextTokens` symbols are reworded to
point at the surviving names (`TestThemedDialogTokensClearContrastFloor`, `dialogSelectionTokens`).

## No theme palette change

`git diff 1cfbd5a..HEAD -- internal/theme/builtin/` is empty — confirmed again below.

## Evidence

- `green.log` / `green.log.exitstatus` — `ci/run.sh go test -count=1 ./internal/theme/
  ./internal/tui/` at the committed state: exit `0`.

Two mutation proofs, each a deliberate one-line edit made directly in the workspace, captured,
then reverted (`git diff` clean afterward, confirmed both times before continuing) — the CI
toolchain only runs against the real `RALPHD_HOST_WORKSPACE` bind mount, so a `/tmp` copy
cannot be exercised through `ci/run.sh`:

- `mutation-added-subfloor-cell.log` / `.exitstatus` — "adding a sub-floor pair to the table":
  `dialogSelectionTokens` temporarily widened from `{Hint, Text}` to `{Hint, Text, Dimmed}`.
  With no allowlist left to absorb it, `TestThemedDialogTokensClearContrastFloor` goes straight
  to `t.Errorf` for the known sub-floor `dimmed/selection` cell on `cobalt` (2.59:1), `empire`
  (2.69:1) and `parchment` (2.51:1) — the floor itself, not just the completeness test, hard-fails
  the instant an unlisted-but-added sub-floor cell reaches the table. Exit status `1`.
- `mutation-dimmed-over-selection.log` / `.exitstatus` — "pointing a focused row at a sub-floor
  token": `renderRenameFieldRow`'s label segment temporarily changed from `theme.Hint` to
  `theme.Dimmed` (unlisted in `dialogSelectionFloorTokens`/`dialogSelectionTokens`, and itself
  sub-floor against `Selection` on the same three built-ins). `TestThemedDialogTokensClearContrastFloor`
  stays green (the static table it walks didn't change), but
  `TestDialogSelectionRenderersComposeOnlyFloorTokens` — the completeness proof — catches it:
  `a dialog composes theme.Dimmed over theme.Selection, but dialogSelectionFloorTokens ... does
  not list it`. Exit status `1`. This is exactly the gap a per-cell allowlist could never close:
  a renderer change that starts composing a new, uncovered, sub-floor pair.
- `theme-builtin-diff-empty.log` — `git diff 1cfbd5a..HEAD -- internal/theme/builtin/`, empty.

Reproduce: `ci/run.sh go test -count=1 ./internal/theme/ ./internal/tui/`.
