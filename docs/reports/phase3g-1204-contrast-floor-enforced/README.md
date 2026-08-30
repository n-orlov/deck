# R84 — no allowlist, floor hard-enforced, pair table proved complete (task 1204)

`SPEC.md` §11.6, PRD `prds/phase3g-field-backlog.md` R84. Task 106/task 1204 both concern
`internal/theme/contrast_test.go`'s `TestThemedDialogTokensClearContrastFloor`: this task
removes the allowlist task 106 added and proves, with a tracked test, that the resulting hard
floor covers every pair a dialog can actually put on screen — no more, no less.

## What changed

`internal/theme/contrast_test.go`:

- task 106's per-theme/per-pair exemption table and its helpers are gone; the criterion's
  `grep -rn dialogPairAllowlist internal/` returns nothing, and
  `TestNoDialogContrastAllowlistRemains` (below) keeps it that way from the suite rather than
  by hand.
- The table `TestThemedDialogTokensClearContrastFloor` walks is now exactly:
  `hint/surface`, `key/surface`, `error/surface` (unchanged, `dialogSurfaceChecks`) plus
  `hint/selection` and `text/selection` (`dialogSelectionChecks`, new — `text/selection` is
  already hard-enforced elsewhere by `TestBuiltinContrastFloor`'s own `contrastChecks`, and is
  repeated here so this table is self-contained).
- Every cell in that table is `t.Errorf` the instant it drops below 3.0:1, for every built-in,
  over both the authored hex palette and the 16-colour quantisation, with no allowlist,
  tolerance table or `t.Logf`-only branch anywhere in the file.
- `theme.SelectionIdle` and the `dimmed`/`key`/`error` tokens are dropped from this table
  entirely — not because they clear the floor, but because no dialog draws them there. See
  `dialogSelectionTokens`'s own doc comment in `contrast_test.go` for the citation
  (`panel.go:134-139`, `settings.go:1318`) and why the excluded sub-floor cells (several,
  including empire's `SelectionIdle` quantisation collapse) are a finding for task 1208, not a
  gap in this floor.

`internal/tui/dialog_selection_floor_completeness_test.go` (new): the tracked completeness
proof, `TestDialogSelectionRenderersComposeOnlyFloorTokens`. It parses every non-test `.go`
file in `internal/tui` and, by AST inspection alone:

1. Reads the floor table itself out of `../theme/contrast_test.go`'s `dialogSelectionTokens`
   declaration (`loadThemeSelectionFloorTokens`). There is **no** tui-side copy of the set:
   the first attempt at this task duplicated it as a hand-synced map, so the two could drift
   and the "completeness" claim was really about the copy. A missing, renamed, non-literal or
   empty declaration is a hard `t.Fatal`, never a fallback.
2. Enumerates **every** `bgColorToken(...)` call and statically resolves its background
   argument through local variables (`resolveThemeTokens`), so
   `bg := theme.Selection; m.bgColorToken(bg, ...)` still counts as a selection site. Exactly
   two must resolve to `theme.Selection`, and they must be `renderCreateRowSegments` (tui.go)
   and `renderRenameFieldRow` (rename.go) — a third site, or either of these two disappearing,
   fails the test.
3. Collects every `settingsRowSegment{... Tok: <expr>}` built in the scope that feeds either
   site (`renderRenameFieldRow`'s own body; for `renderCreateRowSegments`, the innermost
   enclosing function/closure of each call site, since it receives `segs` as a parameter) and
   resolves each `Tok` expression the same way — **fail-closed**: any expression the analysis
   cannot pin down to a `theme.X` token (a call, an index, a parameter, an uninitialised var)
   is a `t.Errorf`, not a silent omission. This is the hole the first attempt had: it matched
   only a literal `Tok: theme.X`, so `tok := theme.Dimmed; ...{Tok: tok}` passed it green.
4. Fails if any resolved token is absent from the floor table read in step 1.

Run today it resolves exactly `[Hint Text]` from the renderers and `[Hint Text]` from the
floor table — the two agree, which is what makes the table complete rather than merely small.

`TestNoDialogContrastAllowlistRemains` (same file) walks every `.go` file under `internal/` and
fails if task 106's exemption-table identifier reappears. Its name is assembled at run time
(`"dialogPair" + "Allowlist"`) precisely so this guard is not itself the grep's only hit.

`internal/tui/tui.go` and `internal/tui/create_view_theme_test.go`: two doc comments that named
the now-deleted symbols are reworded to point at the surviving names
(`TestThemedDialogTokensClearContrastFloor`, `dialogSelectionTokens`).

## No theme palette change

`theme-builtin-diff-empty.log` is `git diff 1cfbd5a -- internal/theme/builtin/` over the
committed tree: empty (0 bytes).

## Evidence

Command for every log below: `ci/run.sh go test -count=1 ./internal/theme/ ./internal/tui/`
(the mutation logs run the identical command inside the scratch worktree, via
`ci/run.sh sh -c 'cd .scratch-1204 && go test -count=1 ./internal/theme/ ./internal/tui/'` —
`ci/run.sh` always bind-mounts `$RALPHD_HOST_WORKSPACE`, so the worktree is created *inside*
the workspace as `.scratch-1204/`, a dot-prefixed directory the Go tool ignores).

- `green.log` / `.exitstatus` — the deliverable run at the committed state: exit `0`.

The four mutation proofs were produced in a throwaway worktree
(`git worktree add --detach .scratch-1204 HEAD`, with this task's two changed files copied in),
mutated there, then `git worktree remove --force`d; `/workspace` itself was never mutated for
them, and `git status` was clean afterwards.

- `scratch-worktree-baseline.log` / `.exitstatus` — the unmutated worktree: exit `0`. This is
  what makes each red log below attributable to its own one-line mutation.
- `mutation-added-subfloor-cell.log` / `.exitstatus` — "adding a sub-floor pair to the table":
  `dialogSelectionTokens` widened from `{Hint, Text}` to `{Hint, Text, Dimmed}`. With no
  allowlist left to absorb it, `TestThemedDialogTokensClearContrastFloor` goes straight to
  `t.Errorf` for the known sub-floor `dimmed/selection` cell on `cobalt` (2.59:1), `empire`
  (2.69:1) and `parchment` (2.51:1). Exit `1`.
- `mutation-dimmed-over-selection-via-variable.log` / `.exitstatus` — "pointing a focused row
  at a sub-floor token", in the indirect form that defeated this task's first attempt:
  `renderRenameFieldRow` gains `tok := theme.Dimmed` and uses `{... Tok: tok}`. The floor test
  stays green (its table did not change), and the completeness proof now catches it:
  `a dialog composes theme.Dimmed over theme.Selection, but ../theme/contrast_test.go's
  dialogSelectionTokens ... lists only [Hint Text]`. Exit `1`.
- `mutation-unresolvable-token-expression.log` / `.exitstatus` — fail-closed proof:
  `{Text: "New name:  ", Tok: theme.Token(m.renameValue)}` composes a token no static analysis
  can pin down. The test refuses to shrug: `Tok expression at rename.go:209:30
  (*ast.CallExpr) cannot be statically resolved`. Exit `1`.
- `mutation-indirect-selection-background.log` / `.exitstatus` — the background argument hidden
  behind a local (`bg := theme.Selection; m.bgColorToken(bg, ...)`) *and* the label pointed at
  `theme.Dimmed`. The site is still recognised (so the "exactly 2 sites" check does not
  mis-fire and mask the real problem) and the unlisted token is reported. Exit `1`.

Together: a sub-floor cell reaching the table fails the floor (proof 1); a renderer starting to
draw an unlisted token fails the completeness proof whether it is written literally, through a
variable (proof 2), or with an indirect background (proof 4); and an expression the proof cannot
read fails instead of passing (proof 3). No branch remains through which a sub-floor pair could
reach the screen unchecked.

Reproduce: `ci/run.sh go test -count=1 ./internal/theme/ ./internal/tui/`.

## Second correction: the render-level proof (validation attempt 2's finding)

Validation of the state above found the remaining hole in the *static* pass: it read a Tok
field only where a composite literal named it (`{Tok: X}`), so a **later assignment** to an
already-built segment — validation's own mutation, `segs[0].Tok = theme.Dimmed` inserted just
before `renderRenameFieldRow` renders those segments through
`bgColorToken(theme.Selection, ...)` — reintroduced the known sub-floor `dimmed/selection` pair
with both tests green. Two changes close it, and the second closes the whole class rather than
that one form:

1. `collectTokExprs` now also collects the right-hand side of every assignment to a `.Tok`
   field (`seg.Tok = X`, `segs[i].Tok = X`), and hands the caller a nil expression — reported
   as unresolvable, i.e. a failure — for a compound (`+=`) or tuple assignment whose value it
   cannot pair with the field. Fail closed, as everywhere else in that file.
2. `internal/tui/dialog_selection_floor_render_test.go` (new) adds
   `TestDialogSelectionCellsRenderOnlyFloorTokens`, a **render-level** twin that does not
   reason about source at all. It reuses `dialogDegradationCases()` — the tracked table that
   already enumerates every dialog this phase themed, with each dialog's own model builder and
   its failure-note variants — renders each body once per built-in theme (`m.settings.Theme`)
   into a `vt.Emulator`, and inspects the finished grid: every non-blank cell whose background
   is that theme's `selection` colour must carry a foreground equal to one of the colours
   R84's floor table holds against `selection`. The accepted set is derived from
   `../theme/contrast_test.go`'s `dialogSelectionTokens` via the same AST reader
   (`loadThemeSelectionFloorTokens`) plus `floorTokenFor`, so there is still no second copy of
   the table; a cell with no explicit foreground on that background fails too (the terminal
   default is a colour the floor cannot hold); and a run that finds no selection-background
   cell at all is a `t.Fatal`, so the proof cannot pass vacuously.

Because it reads cells, no source-level route evades it: a helper the static pass never walks,
a value computed at run time, or a post-hoc field write all land on the grid as an unlisted
foreground.

### Evidence for this correction

- `green.log` / `.exitstatus` above was re-captured at this state: exit `0`, both packages `ok`.
- `mutation-tok-field-assignment.log` / `.exitstatus` — validation's exact mutation,
  `segs[0].Tok = theme.Dimmed` added to `renderRenameFieldRow` in the scratch worktree. Now
  **both** halves fail: the static pass reports `tokens composed over theme.Selection by dialog
  focused rows: [Dimmed Hint Text]` against a floor table of `[Hint Text]`, and the render pass
  reports the `dimmed` foreground on the selection background for `cobalt`, `empire`, `matrix`
  and `parchment`. Exit `1`.
- `mutation-tok-set-by-unwalked-helper.log` / `.exitstatus` — the same sub-floor token reached
  through a helper the static pass does not walk
  (`settingsRenderRowOpen(mutationHelperForProof(segs))`, the helper setting
  `out[0].Tok = theme.Dimmed`). The static pass stays green — it cannot see inside the helper —
  and the render pass alone fails, which is exactly the independent coverage it exists for.
  Exit `1`.

Both were produced in `git worktree add --detach .scratch-1204 HEAD` with this task's test
files copied in, run as
`ci/run.sh sh -c 'cd .scratch-1204 && go test -count=1 ./internal/theme/ ./internal/tui/'`, then
`git worktree remove --force .scratch-1204`; `/workspace` was left clean (`git status` showed
only this task's own two test files).

The earlier mutation logs (`mutation-added-subfloor-cell`,
`mutation-dimmed-over-selection-via-variable`, `mutation-unresolvable-token-expression`,
`mutation-indirect-selection-background`, `scratch-worktree-baseline`) were captured against the
previous state of these tests and are kept as-is: each still documents the branch it closed, and
none of the assertions they exercise was weakened by this correction.
