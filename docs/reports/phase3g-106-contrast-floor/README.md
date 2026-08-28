# R84 — the contrast floor covers the pairs a dialog actually uses (task 106)

`SPEC.md` §11.6, PRD `prds/phase3g-field-backlog.md` R84. `internal/theme/contrast_test.go`
gains a third table-driven test, `TestThemedDialogTokensClearContrastFloor`, covering the
pairs R82's now-themed dialogs draw that neither `TestBuiltinContrastFloor`
(`text`/`hint`/`title`/status over `Background`, plus `text`/`Selection`) nor
`TestSessionRowTokensClearContrastFloorOnSurface` (`title`/`dimmed`/`text`/`badge`/
`badge_warn`/status over `Surface`) already check:

- `hint/surface`, `key/surface`, `error/surface` — the three tokens R82 draws that
  `sessionRowSurfaceChecks` doesn't (a dialog field's label, a footer keycap, a
  validation line).
- Every one of a dialog's focused-field text roles — `text` (value), `dimmed` (per-field
  help), `hint` (label), `key` (footer keycap), `error` (validation) — over **both**
  `theme.Selection` and `theme.SelectionIdle`, the two backgrounds a focused field row can
  carry.

Both the theme's authored hex palette and its §11.6 16-colour quantisation, exactly as the
two existing tests already do, for all five built-ins (`cobalt`, `daylight`, `empire`,
`matrix`, `parchment`).

## No palette change

Per the PRD's own instruction ("this requirement pins what is already true; it does not
license a palette change ... bring a failing pair to the operator via a finding rather than
editing a theme file to make a test pass"), `internal/theme/builtin/*.toml` is untouched —
`git show --stat` on this task's commit shows only `internal/theme/contrast_test.go`.

`matrix` is the PRD's own reference theme and is measured to clear every new pair (thinnest
`error/selection` at 3.16:1 hex, matching the PRD's own citation exactly) — that theme's
floor is hard-enforced (`t.Errorf`, fails the suite) inside the new test, so a future
regression there is caught. Every *other* built-in's new-pair failures are logged as
`FINDING` lines (still visible with `go test -v`, not silenced) and summarised in the
`SUMMARY` lines at the end of the run, but are **not** turned into a build-breaking
assertion — consistent with the PRD's "that is a finding to report, not a licence to
recolour" ruling for a non-reference theme. `ci/run.sh go test -count=1 ./internal/theme/`
exits 0 (`theme-suite-green.log`).

## Thinnest newly-covered pair per built-in

Read from `dialog-contrast-v.log`'s own `SUMMARY` lines (the minimum, hex-or-quantised, over
all 13 new pairs: 3 `/surface` + 5 tokens × 2 backgrounds):

| theme | thinnest new pair | ratio | clears 3.0:1? |
|---|---|---|---|
| cobalt | `dimmed/selection` (hex) | 2.59:1 | **no** |
| daylight | `dimmed/selection` (quant) | 3.18:1 | yes |
| empire | `dimmed/selectionidle` (quant, `#7f7f7f`/`#7f7f7f`) | 1.00:1 | **no** |
| matrix | `error/selection` (hex) | 3.16:1 | yes (reference theme, hard-enforced) |
| parchment | `dimmed/selection` (hex) | 2.51:1 | **no** |

`cobalt`, `empire` and `parchment` fail one or more of the newly-covered pairs, always on
`dimmed` (the per-field help colour) against `Selection`/`SelectionIdle` — `empire` further
collapses `hint`, `key` and `error` against `SelectionIdle` because that theme's
16-colour quantisation puts `SelectionIdle` and several foreground tokens on the exact same
reference colour (`#7f7f7f`). Recorded here and in `docs/reports/phase3g-findings.md`
(new row) with the measured ratios above; not recoloured.

## Evidence

- `dialog-contrast-v.log` — `ci/run.sh go test -count=1 -v ./internal/theme/ -run
  TestThemedDialogTokensClearContrastFloor`: every pair's hex/quant ratio for all five
  built-ins, the per-theme `FINDING` lines for sub-floor pairs, and the final `SUMMARY`
  lines this table is drawn from.
- `theme-suite-green.log` — `ci/run.sh go test -count=1 ./internal/theme/`: exit 0, the
  whole package (not just the new test), confirming the addition does not disturb any
  pre-existing theme test.

Reproduce: `ci/run.sh go test -count=1 -v ./internal/theme/ -run
TestThemedDialogTokensClearContrastFloor` then `ci/run.sh go test -count=1
./internal/theme/`.
