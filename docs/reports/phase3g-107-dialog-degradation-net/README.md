# Task 107 — net the themed dialogs against NO_COLOR/DECK_ASCII/width (R82)

New file: `internal/tui/dialog_degradation_net_test.go`.

## What it covers

`TestThemedDialogsDegradeCleanlyUnderNoColorAndASCII` iterates over all ten
dialogs themed this phase (create modal, env editor, bulk delete confirm,
delete/purge confirm, archive confirm, profile picker, pin conversation,
restart-or-inject, rename, event log), reusing each dialog's own existing
model-building test helper (`task016CreateTestModel`, `task017EnvTestModel`,
`task018BulkDeleteModel`, `task019ArchiveModel`/`task019DeleteModel`,
`task020ProfileModel`/`task020PinModel`/`task020RestartChoiceModel`,
`task021RenameModel`, `task021EventLogModel`) so this file can never silently
diverge from what those dialogs' own theme tests already assert is a themed
render.

For each dialog (and, for several, a couple of mutated states — a
validation/failure note, a purge-chosen branch, a read-error branch), it
builds:

- **themed**: the dialog's existing `Color: true` model.
- **degraded**: the SAME model with `settings.Color = false` and
  `settings.ASCII = true` — NO_COLOR and DECK_ASCII set together. This is the
  one combined scenario `config.Load` can actually produce (the two env vars
  gate entirely independent `Settings` fields — `internal/config/config.go`
  never lets DECK_ASCII touch `Color` or NO_COLOR touch `ASCII`), so testing
  them together rather than in isolation is what actually exercises both
  degradations at once.

And asserts:

1. The degraded styled body carries **no SGR byte** (`0x1b`) at all.
2. The degraded body's text is **byte-identical** to the themed body once
   ANSI is stripped back out (`stripANSI(themed) == degradedBody`) — no
   label or value lost to either the colour cut or the ASCII glyph fallback.
3. `wrapDialogLines` produces the **same physical line count** for the
   themed body and for its own ANSI-stripped twin, and each corresponding
   line is identical once stripped — proving colour never moves where a
   page boundary falls.
4. `dialogContentBudget()` is unaffected by Color/ASCII.
5. The degraded styled body's physical lines match
   `wrapDialogLines(the dialog's own PLAIN body, same degraded settings)`
   line for line — the same "styled == wrapped plain" proof every per-dialog
   theme test already runs under `Color: true`, now run under
   NO_COLOR+DECK_ASCII too.

`TestRenameFieldRowTruncationReemitsItsOwnReset` is the "at least one dialog"
truncation clause (SPEC §11.3): `renderRenameFieldRow` (task 105) composes
the rename dialog's one focused field under a single `Selection` BACKGROUND
span opened once and closed with exactly one trailing reset at the end of
the line. The test truncates a REAL themed line taken straight from the
dialog (not a synthetic string) to a budget narrower than its full width and
asserts the opening background escape survives and the reset is re-emitted
(`truncateToWidth`'s own documented background-span behaviour, panel.go).
Foreground-only spans are deliberately excluded from that guarantee by
`truncateToWidth` itself (its own doc comment explains why — only an open
BACKGROUND can bleed into whatever a caller concatenates next), so the
rename dialog's Selection-background row is the correct exhibit for this
clause; no dialog in this set opens a background span other than the one
already-focused field a `dialogFields.Index` navigates to.

## Why NO_COLOR+DECK_ASCII combined, not two separate scenarios

DECK_ASCII and NO_COLOR/DECK_COLOR resolve entirely independent
`config.Settings` fields (`ASCII` vs `Color`) — `DECK_ASCII` alone never
disables colour, and none of the ten dialog body functions call `m.glyph`
inside their normal (non-overflowing) render path, so DECK_ASCII alone would
be indistinguishable from the baseline for this fixture set. Testing both
together is the one scenario that is not already a strict subset of
"NO_COLOR alone" and is also the worst realistic case a themed dialog must
survive.

## Commands run (both exit 0, logs alongside this file)

- `ci/run.sh go test -count=1 ./internal/tui/` → `internal-tui-go-test.log`
- `ci/run.sh env DECK_GODOG_PATHS=dialogs.feature,environment.feature,create_session.feature go test ./features/ -run TestFeatures -count=1` → `features-go-test.log`
- `ci/run.sh go test -count=1 -run 'TestThemedDialogsDegradeCleanlyUnderNoColorAndASCII|TestRenameFieldRowTruncationReemitsItsOwnReset' ./internal/tui/ -v` → `task107-verbose.log`
