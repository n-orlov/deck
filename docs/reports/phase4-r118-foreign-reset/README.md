# R118 — the foreign-content-to-deck boundary reset, gated on deck's own colour only

Task **cure-03-01-2** (approach 3's cure pass). Commit `a260abf` (the fix).
Final tail code sha of this approach: `a260abfaa36fe96068fb19f4735d66b3b040459b`.

## The defect

`internal/tui/panel.go`'s `canvasResetIfPainting` is the explicit
"\x1b[0m" that `previewContentLine`, `fullBoxPreviewContentLine` and
`paintForeignFill` (via `cropRow`) emit right after a captured tmux pane's
own SGR bytes, so a foreign row's colour/attribute state cannot bleed into
deck's own border/pad/crop-marker/chrome columns immediately past it — the
half of R118 ("captured pane content is never repainted") that is about
closing the boundary, not about leaving the pane's own bytes untouched
(the other half, unaffected by this task).

Before this fix, that reset fired only when deck's OWN colour painting was
enabled (`canvasBackground(theme.Background, ...)` would actually open a
background). Under `NO_COLOR` (or any colour-disabled `Settings{}` build),
the reset never fired at all — even when the CAPTURED pane's own bytes left
SGR open (a real full-width coloured row with no closing reset of its own,
exactly what `tmux capture-pane -e` returns for a still-coloured cell at
end of buffer). The pane's own foreground/background then leaked straight
into deck's border/pad/crop/chrome columns past the boundary, in both
preview content-line builders and in `cropRow`'s own pad-fill/crop-marker
tail.

## The fix

`canvasResetIfPainting` now gates the reset on two independent disjuncts:
deck's own colour state (unchanged, first branch) OR the foreign text
itself carrying any escape byte (`\x1b`) — a cheap, conservative proxy for
"this row's own bytes might have left SGR open". A genuinely plain,
escape-free foreign row under a colour-disabled build still gets no reset
at all (nothing was ever opened, so nothing is owed — this is what keeps
every existing plain-NO_COLOR-text/geometry assertion, and the
absence-of-unnecessary-escapes property, unchanged), while any row that
carries an escape byte gets the defensive close regardless of deck's own
colour setting. `paintForeignFill` now takes the actual preceding foreign
bytes (or `""` when `cropRow`'s crop marker has replaced the row entirely,
since then no foreign byte reaches that line's own output at all) so it
can make the same decision at its own two call sites. The captured/foreign
bytes themselves are never scanned, repainted or stripped — only the
boundary immediately after them gets an explicit, idempotent reset.

## The regression and its controls

`TestForeignBoundaryResetClosesCapturedSGRUnderNoColor`
(`internal/tui/preview_foreign_boundary_reset_test.go`) exercises both
production preview content-line builders (`previewContentLine`,
`fullBoxPreviewContentLine`) directly against a synthetic captured row that
opens truecolour fg/bg and never closes it, under `Settings{Color: false}`,
in both side-by-side and stacked sub-tests.

`TestForeignBoundaryResetClosesRealTmuxCaptureUnderNoColor`
(`internal/tui/preview_foreign_boundary_reset_live_test.go`) is the
companion proof against a REAL tmux `CapturePreview` (not a synthetic byte
string): a real pane respawned to print a full-width row with the same
open truecolour SGR and never close it, captured through the actual tmux
client, asserted through `Model.View()` in both layouts, with a
colour-enabled control (`color-control` sub-tests) proving the same
scenario already worked pre-fix when deck's own colour was on — so a
NO_COLOR-only red is a genuine colour-disabled-boundary gap, not a fixture
defect.

`red.log` is both tests' output against the pre-fix tree (`bfdb69e`):

```
review_nocolor_capture_test.go:22: deck right border inherits captured background "#445566" under NO_COLOR
review_nocolor_capture_test.go:23: deck right border inherits captured foreground "#112233" under NO_COLOR
...
review_nocolor_live_test.go:47: real capture leaked background #445566 into deck border at (119,1) color=false
...
review_nocolor_live_test.go:47: real capture leaked background #445566 into deck border at (119,13) color=false
```

(the two committed test files were later renamed from `review_nocolor_*` to
`preview_foreign_boundary_reset_*` and their `Test...` names updated to
match; `red.log` predates that rename, `green.log` postdates it.)

`green.log` is the same tests, run under their final committed names, at
the post-fix tree (`a260abf`): all sub-tests PASS, including both
`color-control` sub-tests (proving the colour-enabled path is unchanged).

## Result

Fixed. `internal/tui`'s full package suite
(`ci/run.sh go test -count=1 ./internal/tui/`) is green at `a260abf`; the
whole-tree gate (`docs/reports/phase4-a3-final-suite/`) and the ten-run
stability sweep (`docs/reports/phase4-a3-stability10/`, 10/10 clean) were
both re-taken at this same sha per the standing rule that any commit
touching `*.go`/`*.feature` forces both sweeps to be re-run from scratch.
`go build`/`go vet`/`gofmt` guards are unaffected (gofmt drift is exactly
the four pre-existing paths, unchanged).
