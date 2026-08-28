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

## The floor is hard-enforced for every built-in, with a per-cell allowlist

The floor fails the suite (`t.Errorf`) for **every** built-in and every newly-covered pair,
not only for the reference theme `matrix`. The exception is not a theme, it is a short,
explicit list of individual cells: `dialogPairAllowlist` in `internal/theme/contrast_test.go`
names each `(theme, pair, colour space)` that was *already* sub-floor when this coverage
landed, together with the ratio measured here. Ten cells are listed (`cobalt` ×1, `empire`
×7, `parchment` ×2, verbatim in the `FINDING` lines of `dialog-contrast-v.log`); those are
the authored/quantised values finding F23 reports and this plan may not recolour.

Everything the allowlist does *not* name is a build-breaking assertion, and a listed cell is
pinned rather than waived:

- an unlisted cell that drops below 3.0:1 fails, in any theme;
- a listed cell whose ratio drifts from the recorded value by more than 0.01 fails (worse
  *or* better) — the recorded number no longer describes the palette;
- a listed cell that has climbed to the floor fails as a stale entry, so the allowlist can
  only shrink deliberately;
- an allowlist key matching no cell this test walks fails as unmatched, so a typo cannot
  quietly exempt a real cell.

Both halves are demonstrated, not asserted:

- `negative-unlisted-pair-fails.log` — with `"empire dimmed/selection hex"` deleted from the
  allowlist the package exits 1: `theme "empire" dimmed/selection: hex contrast 2.69:1 <
  3.0:1 (fg=#64748b bg=#26324b)`. A non-`matrix` theme is genuinely hard-enforced.
- `negative-recorded-ratio-drift-fails.log` — with `"parchment dimmed/selection hex"`
  recorded as `2.90` instead of the measured `2.51` the package exits 1: `theme "parchment"
  dimmed/selection (hex): 2.51:1 ... drifted from the recorded 2.90:1`. An allowlisted cell
  cannot silently move.

At the committed values `ci/run.sh go test -count=1 ./internal/theme/` exits 0
(`theme-suite-green.log`); `matrix` clears every new pair, thinnest `error/selection` at
3.16:1 hex, matching the PRD's own citation exactly.

## Thinnest newly-covered pair per built-in

Read from `dialog-contrast-v.log`'s own `SUMMARY` lines (the minimum, hex-or-quantised, over
all 13 new pairs: 3 `/surface` + 5 tokens × 2 backgrounds):

| theme | thinnest new pair | ratio | clears 3.0:1? |
|---|---|---|---|
| cobalt | `dimmed/selection` (hex) | 2.59:1 | **no** |
| daylight | `dimmed/selection` (quant) | 3.18:1 | yes |
| empire | `dimmed/selectionidle` (quant, `#7f7f7f`/`#7f7f7f`) | 1.00:1 | **no** |
| matrix | `error/selection` (hex) | 3.16:1 | yes (reference theme, nothing allowlisted) |
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
- `negative-unlisted-pair-fails.log`, `negative-recorded-ratio-drift-fails.log` — the two
  deliberate one-line mutations of `dialogPairAllowlist` described above, each exiting 1.
  Both mutations were reverted before the commit; `git show --stat` shows no palette file and
  no allowlist change beyond the committed table.

Reproduce: `ci/run.sh go test -count=1 -v ./internal/theme/ -run
TestThemedDialogTokensClearContrastFloor` then `ci/run.sh go test -count=1
./internal/theme/`.
