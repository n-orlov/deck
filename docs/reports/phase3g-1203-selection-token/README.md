# Task 1203 — stop composing a sub-floor token over the dialog selection background

## What was wrong

The create modal's cwd row, when focused, renders through
`renderCreateRowSegments` inside one `bgColorToken(theme.Selection, ...)`
span (`internal/tui/tui.go`). Its ghost-completion suffix (the
directory-name text the user never typed, `createCWDGhostSuffix`) took
`theme.Dimmed` for that span. `theme.Dimmed` over `theme.Selection` is one
of `internal/theme/contrast_test.go`'s own `dialogPairAllowlist` entries —
a knowingly sub-floor pair, measured there at 2.59:1 (cobalt), 2.69:1
(empire) and 2.51:1 (parchment), all below R84's 3.0:1 floor.

## Fix

`internal/tui/tui.go`'s `styledCreateBody` (the `colorLabelValue` closure)
now composes the ghost span in `theme.Hint` instead of `theme.Dimmed`. No
theme palette changed — `git diff 1cfbd5a..HEAD -- internal/theme/builtin/`
stays empty. `theme.Hint` is already one of
`dialogFocusedFieldTextTokens` R84 holds every built-in to over
`theme.Selection`, and per `internal/theme/contrast_test.go`'s own
`dialogPairAllowlist`, `hint/selection` is NOT a listed (i.e. sub-floor)
cell for any built-in — it already clears the floor everywhere the
allowlist's absence implies, confirmed below by direct measurement.

The typed portion of the cwd value keeps `theme.Text`; `theme.Hint` and
`theme.Text` resolve to different hex colours in every built-in, so the
ghost stays visually distinguishable from what the user actually typed
(new test assertion `ghostFg == typedFg` fails the test if that ever
stops being true).

## Test

`internal/tui/create_view_theme_test.go`'s
`TestCreateViewGhostRendersDimmedAtRenderTime` is renamed
`TestCreateViewGhostRendersHintAtRenderTime` and rewritten to assert the
focused cwd row's ghost segment renders in `theme.Hint`, the typed segment
renders in `theme.Text`, and the two resolved hex colours differ — the
"ghost segment token differs from the typed segment's token" success
criterion, read per-cell off a real `vt.Emulator` grid (not a
substring/plain-body assertion).

`red-before.log` / `red-before.exitstatus` (exit 1) is the new test run
against the UNCHANGED renderer (`theme.Dimmed` still in place — captured
by stashing only `internal/tui/tui.go`, keeping the new test): it fails
because the ghost cell resolves to empire's `#64748b` (dimmed) rather than
the expected `#94a3b8` (hint). `green-after.log` /
`green-after.exitstatus` (exit 0) is
`ci/run.sh go test -count=1 ./internal/tui/ ./internal/theme/` against the
fixed renderer.

No other dialog renderer composes a `settingsRowSegment` over
`theme.Selection`: `renderCreateRowSegments` (this task) and
`renderRenameFieldRow` (`internal/tui/rename.go:212`, using `hint`/`text`
only, already floor-clearing per R84 facts recorded in the handoff notes)
are the only two call sites.

None of the four protected plain-substring PTY test files
(`create_field_help_test.go`, `dialog_width_test.go`,
`archive_confirm_test.go`, `delete_purge_test.go`) were touched —
confirmed by `git show --stat` on this task's commit.

## Measured ratios (`ratio-table.md`)

Every built-in theme (cobalt, empire, parchment, matrix, daylight),
`dimmed` (old) vs `hint` (new) over `theme.Selection`, both the authored
hex palette and its 16-colour §11.6 quantisation. Every new (`hint`)
value is >= 3.0:1 in both colour spaces; the old (`dimmed`) values
reproduce the sub-floor cells already on record in
`internal/theme/contrast_test.go`'s `dialogPairAllowlist` (cobalt 2.59,
empire 2.69, parchment 2.51 — matrix and daylight were never sub-floor for
`dimmed`, since `dialogPairAllowlist` never lists them; matrix's own
`dimmed/selection` hex ratio here, 3.78:1, matches that).

## Commands run

```
ci/run.sh go build ./...
ci/run.sh go test -count=1 ./internal/tui/ -run 'TestCreateViewGhostRendersHintAtRenderTime' -v   # red-before (old code) / green (new code)
ci/run.sh go test -count=1 ./internal/tui/ ./internal/theme/                                       # green-after.log, exit 0
```
