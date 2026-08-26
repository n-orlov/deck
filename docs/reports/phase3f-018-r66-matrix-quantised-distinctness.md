# R66 — `matrix`'s seven §7 status tokens are pairwise distinct under 16-colour quantisation

Task 018, Phase 3f. Source of the defect: `docs/reports/phase3e-findings.md` §4b — five of
`matrix`'s tokens collapsed onto ANSI 8 (`#7f7f7f`) under `quantize()`, so `idle`, `stopped` and
`archived` painted the *same* colour on a 16-colour terminal. The defect was **collision, not
legibility**: every one of them already cleared the 3:1 floor.

## What changed

`internal/theme/builtin/matrix.toml` — three authored colours, nothing else:

| token | authored before | authored after | quantised before | quantised after | ANSI slot |
|---|---|---|---|---|---|
| `idle`     | `#33cc66` | `#22cc55` | `#7f7f7f` | **`#00cd00`** | 8 → **2** |
| `stopped`  | `#889988` | `#aaccaa` | `#7f7f7f` | **`#e5e5e5`** | 8 → **7** |
| `archived` | `#66aa77` | `#778877` | `#7f7f7f` | `#7f7f7f` (unchanged) | 8 → 8 |

Three tokens re-authored; **two pinned quantised values changed**, because `archived` deliberately
keeps ANSI 8 — with `idle` and `stopped` gone, that slot is now unshared among the seven statuses,
and "bright black" is the dimmest legible slot, which is what an archived row should be. Its
authored hex still moved (greener → greyer) so the true-colour appearance matches the 16-colour
mapping instead of contradicting it.

The resulting seven-way mapping (from `TestMatrixStatusTokensQuantiseToSevenDistinctReferenceEntries`'s
own `-v` log, `artifacts/task018-r66-three-packages.log` / `/tmp/theme.log` run):

    waiting  #ffff33 -> 11 #ffff00      running  #00ff66 -> 10 #00ff00
    idle     #22cc55 ->  2 #00cd00      starting #33aaff ->  6 #00cdcd
    stopped  #aaccaa ->  7 #e5e5e5      error    #ff3333 ->  9 #ff0000
    archived #778877 ->  8 #7f7f7f

## `TestBuiltinQuantizationPinned` reconciliation (the sanctioned kind)

Exactly two entries of the `"matrix"` map moved, `Idle` and `Stopped`; no other theme's pinned
entries were touched. `Archived` is stated in the table above as unchanged so the reconciliation is
auditable per token rather than by omission.

```diff
-			Idle:          "#7f7f7f",
+			// R66: idle #33cc66 -> #22cc55 moved off ANSI 8 onto ANSI 2, ...
+			Idle:          "#00cd00",
 			Starting:      "#00cdcd",
-			Stopped:       "#7f7f7f",
+			Stopped:       "#e5e5e5",
```

`git diff --name-only` for the whole change:

    internal/theme/builtin/matrix.toml
    internal/theme/quantize_test.go
    internal/theme/matrix_status_quantization_test.go   (new, untracked before the commit)

No other built-in theme (`cobalt`, `daylight`, `empire`, `parchment`) is touched, and no golden or
`NO_COLOR` frame moves — this is colour data only.

## Contrast floor, over both palettes (from `TestBuiltinContrastFloor` / `TestSessionRowTokensClearContrastFloorOnSurface`, `-v`)

    matrix idle/background      hex #22cc55/#001100 =  9.11:1   quant #00cd00/#000000 =  9.73:1
    matrix stopped/background    hex #aaccaa/#001100 = 11.04:1   quant #e5e5e5/#000000 = 16.67:1
    matrix archived/background   hex #778877/#001100 =  5.16:1   quant #7f7f7f/#000000 =  5.24:1
    matrix idle/surface          hex #22cc55/#002200 =  8.00:1   quant #00cd00/#000000 =  9.73:1
    matrix stopped/surface       hex #aaccaa/#002200 =  9.71:1   quant #e5e5e5/#000000 = 16.67:1
    matrix archived/surface      hex #778877/#002200 =  4.53:1   quant #7f7f7f/#000000 =  5.24:1

Every new value clears 3:1 on both palettes and against both backgrounds; the lowest margin is
`archived/surface` at 4.53:1 (it was 6.17:1 before, still well clear).

## The test, and proof it is not vacuous

New: `internal/theme/matrix_status_quantization_test.go`,
`TestMatrixStatusTokensQuantiseToSevenDistinctReferenceEntries`. Distinctness is computed over
`QuantizedColor()`, never `Color()`.

- **Revert-and-reproduce.** With only the three authored hexes put back to their old values
  (`#33cc66`/`#889988`/`#66aa77`, everything else identical), the new test fails:
  `artifacts/task018-r66-revert-RED.log` —
  `#7f7f7f: archived, idle, stopped` … `quantise onto 5 distinct ReferencePalette entries, want 7`.
- **The trap the PRD names, demonstrated.** On that same reverted tree the *true-colour* test
  `TestMatrixStatusTokensRenderAsSevenDistinctColours` still passes
  (`artifacts/task018-r66-truecolour-passes-with-defect.log`, `exit=0`): distinctness over authored
  values is true while the defect is present, which is exactly why the new test measures the
  quantised values. That existing test passes **unmodified** after the change too.
- The tree was restored from a `/tmp` copy afterwards; `git diff --name-only` shows only the three
  files above.

## Deliverable test run

`ci/run.sh go test -p=1 -count=1 ./internal/theme/ ./internal/tui/ ./features/` —
`artifacts/task018-r66-three-packages.log`.

## Choices and findings

- **`hint` and `badge` were NOT moved.** Both stay `#55cc77` → ANSI 8. They are not §7 statuses, so
  the requirement does not reach them; moving them would have widened the pinned diff beyond the
  sanctioned reconciliation for no gain the requirement asks for. The consequence is stated plainly:
  at 16-colour depth `hint`, `badge` and `archived` all render ANSI 8. The PRD explicitly licenses
  this shape ("leaving both on ANSI 8 alongside `stopped` is a legitimate choice"). Similarly `idle`
  now shares ANSI 2 with `dimmed` and `key` — again not §7 statuses.
- **FINDING (recorded, deliberately not fixed): every other built-in has the same collision.**
  Computed over each theme's authored status hexes with the same nearest-RGB rule:

  | theme | distinct slots for the 7 statuses | collisions |
  |---|---|---|
  | `cobalt` | 4 | ANSI 6 `#00cdcd`: running, starting · ANSI 8 `#7f7f7f`: idle, stopped, archived |
  | `daylight` | 3 | ANSI 1 `#cd0000`: waiting, starting, error · ANSI 8: idle, stopped, archived |
  | `empire` | 5 | ANSI 8 `#7f7f7f`: idle, stopped, archived |
  | `parchment` | 2 | ANSI 1 `#cd0000`: waiting, error · ANSI 8: running, idle, starting, stopped, archived |
  | `matrix` (after this change) | **7** | none |

  Generalising the property is the SPEC §11.6 change this phase deliberately does not make: §11.6
  requires legibility, not pairwise distinctness, and all four themes clear the legibility floor.
  This belongs in `docs/reports/phase3f-findings.md` (task 024), not in R66's scope.
