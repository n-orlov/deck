# Steer 018 item 4 — passive preview fit-on-navigation (task 215)

## What changed

SPEC.md §11 (post-`395babf`, "The preview fits the selected session's window to the
panel") and §11.9 describe fitting the selected session's tmux window to the preview
panel's own content box not only when a client enters interactive mode (already true
before this task) but as an ordinary, best-effort side effect of the list selection
settling in **passive** mode too. Before this task, passive preview only ever read
(`capture-pane -e`) and never wrote to the pane it showed.

1. New flat config key `[ui] preview_fit` (default `true`), added through the same R7
   schema mechanism task 214's `yolo_default` used: `internal/config/schema.go`
   (`Field`, `Section: "ui"`), `internal/config/toml.go` (`FileConfig.PreviewFit` +
   default-fill/value-set switches), `internal/config/toml_write.go` (round-trip),
   `internal/config/config.go` (`Settings.PreviewFit` + `DECK_PREVIEW_FIT` env
   override, mirroring `DECK_GROUP_BY_WORKSPACE`), and `internal/tui/settings.go`
   (toggle read/write/live-apply/display, plus `settingsEditsFromSettings` — see the
   cross-cutting bug fixed below).
2. `internal/tui/tui.go`: a new `previewFit() tea.Cmd`, run alongside `capturePreview()`
   on the same 250ms `previewTick` (never once per row walked while an arrow key is
   held — coalescing is via a single `previewFitSessionID` field on `Model`: a fit is
   only issued when the selected session's ID differs from the last one this ran for,
   so N ticks while sitting on the same selection, or bouncing rapidly between two
   already-fitted rows, cost at most one `resize-window` each). Guards, checked in this
   order, any one of which skips the fit silently (never an error, matching passive
   preview's existing best-effort character):
   - `preview_fit = false`,
   - already in interactive mode (interactive's own fit owns this pane instead),
   - no tmux client available,
   - nothing selected / preview panel not shown,
   - the selected session already matches `previewFitSessionID` (coalesced),
   - the panel's content box is below `interactiveMinInnerRows` (7 rows) — the same
     floor interactive mode itself refuses below; skipped, not queued, and retried on
     the very next tick once the panel is tall enough again, with no debounce/memory of
     the skip.
3. **No attached-client refusal check.** Interactive mode has three refusal cases
   (SPEC §11.9); passive fit deliberately does not replicate the "another client is
   attached to this session" one. SPEC's own text for this feature describes an
   attaching client as simply winning after the fact, not as a case passive fit must
   detect and refuse — best-effort, no ownership claim, no restore-on-exit. This is
   also why `exitInteractive` resets `previewFitSessionID` to `""`: interactive mode
   restores the pane to its pre-entry geometry (not the panel's current content size)
   on exit, so the coalescing guard must not treat that session as "already fitted"
   afterward, or a real resize the very next tick would be wrongly skipped.
4. Cost, stated in both places task 215 requires:
   - **Here** (this report): every fit is one `resize-window`, which is one SIGWINCH to
     the agent inside that pane, and, if the pane's contents don't reflow identically at
     the new size, a loss of terminal scrollback the agent itself cannot recover (the
     same irreversible cost interactive mode's own fit has always had — this task only
     widens when it can happen, from "on interactive entry" to "on interactive entry OR
     a settled passive selection change"). The coalescing guard bounds this to one
     resize per session per *settle*, not one per tick and not one per keypress; bouncing
     a selection back and forth after it has already converged costs nothing further,
     because `FitWindowToPane` is itself a no-op once the window already matches. A
     pathological case is still possible in principle if the panel's content box itself
     keeps changing size on every tick (e.g. a client is being interactively resized by
     its own user at the same time another client's selection settles) — each distinct
     content-box size is a fresh convergence target, so N distinct sizes cost up to N
     resizes, never unbounded per-tick churn for a *fixed* size.
   - **In the help view** (`internal/tui/tui.go`'s `helpText()`, same commit as the
     key per the guard-word convention): the `↑/↓ or j/k select a session` line now
     states that a settled selection also fits the session's window to the preview
     panel, coalesced against the preview tick (not once per row walked), names the
     SIGWINCH/scrollback cost explicitly, states the 7-row floor skip, states it is
     best-effort with no ownership claim, and names `preview_fit = false` as the
     opt-out.
5. `features/preview.feature`'s own header comment and the pre-existing
   `@requirement-21-preview-no-side-effects` scenario: the amendment above is a
   **deliberate, narrowly-scoped requirement change**, not a weakening. The original
   scenario's exact original assertions are unchanged and still pass, now behind
   `Given the deck config disables preview fit`, which isolates the pure
   `capture-pane -e` read-only guarantee (requirement 21's whole point) from this new,
   orthogonal, additive feature. Four new scenarios, tag `@steer-018-preview-fit-on-navigation`,
   prove: (a) the no-resize guarantee still holds on every OTHER axis (mode switch,
   sidebar width, outer-terminal resize) with `preview_fit` at its true default — only
   a *settled selection change* triggers a fit; (b) a settled selection fits the window
   to exactly the panel's content box, and bouncing between two already-settled
   sessions repeatedly, then entering interactive mode, costs no further resize at all
   (proving convergence on the *exact* box interactive mode itself would compute, not
   merely "a" size); (c) `preview_fit = false` costs zero SIGWINCH across the same
   bounce; (d) the 7-row floor skips the fit below it and retries once the panel grows
   back above it.
6. `docs/reports/phase3-findings.md`'s companion PRD text
   (`composite-prd.md:747-748`, quoted verbatim in `features/preview.feature`'s own
   header): "Do not weaken `features/preview.feature`'s passive guarantees ... If they
   conflict, the interactive design is wrong." Read literally, this sentence is about
   an *interactive-mode* change reaching backward to relax a *passive* guarantee in
   order to make itself pass — the direction of causation is interactive → passive.
   This task's causation runs the other way: SPEC.md's own §11 text (the operator's own
   pushed amendment, not code under this run's authorship) directly describes passive
   preview's behavior changing; nothing about the interactive design forced this or is
   even touched by it. Read in spirit — protect every OTHER passive-preview guarantee
   from silent erosion — it also does not block this: no attached client, no
   pipe-pane, no scroll-position side effect, best-effort, floor-respecting, cost-stated,
   and reversible via `preview_fit = false` are every one of them still intact and
   proven by the scenarios above; only the single narrow "capture never writes" claim
   on the SELECTION axis specifically is inverted, and only because SPEC's own text
   requires it.

## Cross-cutting bug found and fixed while proving this

`settingsEditsFromSettings()` (`internal/tui/settings.go`) builds the `FileConfig`
snapshot the settings takeover edits in place, and it omitted `PreviewFit` entirely —
every `,` open of the takeover silently reset the staged value to `false` regardless of
the true config, independent of anything the user did. This was invisible until this
task added a field whose zero value differs from its schema default, at which point
several unrelated existing settings subtests (`stale_after`, `capture_min_interval`,
`tmux_mouse`) started failing on unrelated assertions purely because `PreviewFit`
flipped true→false on every `ctrl+s` round-trip in those tests' shared setup path.
Fixed by adding `PreviewFit: s.File.PreviewFit` to the returned literal. Confirmed
non-vacuous by temporarily reverting just that one field and re-running
`ci/run.sh go test -count=1 ./internal/tui/...`, seeing the same unrelated failures
recur, then restoring it (`git diff` clean before commit).

## Test-harness fix found and fixed while proving this

`features/agent_steps_test.go`'s `selectRowByName` (used by every `deck client "X"
selects session "Y"` step, and by the launch-lease race/status-probe helpers that call
it directly) only ever sent down-arrows from wherever the cursor already was.
List navigation never wraps (`nextVisibleSelection`/`prevVisibleSelection`,
`internal/tui/group.go`) so a scenario that needs to select a row ABOVE the current one
— exactly what this task's "bounce between two settled sessions repeatedly" scenario
needs — could never reach it and would hang until the 50-attempt search gave up.
Generalized the helper to rewind to the top with up-arrows first (the same idiom
`status_probe_test.go` already used ad hoc in one place, now factored into the shared
helper), so the search is position-independent for every caller. All existing callers
re-verified green after the change.

## Non-vacuousness / evidence

- `internal/config` and `internal/tui` unit tests, including `TestSchemaPinsKeySet`,
  `TestSchemaScopes` (both extended for `ui.preview_fit`), the settings scope-parity
  test, and the settings navigation index test (`settings_task006_test.go`) all updated
  for the schema insertion and green:
  `ci/run.sh go test -count=1 ./internal/config/... ./internal/tui/...` — both packages
  `ok`.
- `ci/run.sh go test -count=1 ./cmd/...` — green, no help-text assertion breakage.
- Filtered `features/` runs, each via
  `ci/run.sh sh -c 'DECK_GODOG_TAGS="<tag>" go test -run TestFeatures ./features/ -v'`
  (never naming `@real-agents`, even negated, per the standing rule — that substring
  match is itself a latent test-harness bug this task's investigation surfaced but did
  not need to fix to stay correct, since the default suite never sets
  `DECK_GODOG_TAGS` at all):
  - `@steer-018-preview-fit-on-navigation`: 4 scenarios, 57 steps, all passed.
  - `@requirement-21-preview-no-side-effects`: 1 scenario, 19 steps, passed.
  - The remaining eight tags already in `preview.feature`
    (`@requirement-22-24-preview-colour-border-integrity`,
    `@requirement-23-preview-crop-geometry`, `@requirement-24-preview-wide-cell-boundary`,
    `@requirement-25-preview-gesture-no-ops`, `@requirement-26-preview-crash-tail`,
    `@requirement-26-preview-placeholder`, `@requirement-27-preview-suppressed-below-floor`,
    `@requirement-46-interactive-fitted-geometry`): 8 scenarios, 60 steps, all passed.
  - `@requirement-17-clear-recent-cwds-history` (the one `features/settings.feature`
    scenario whose `j`-count needed the schema-insertion fix): 1 scenario, 29 steps,
    passed.
- `ci/run.sh go build ./... && go vet ./...`: clean.
- Full untagged `./features/...` suite (the one-per-iteration budget item) run in the
  background for final confirmation before commit; see the commit for its result.

## SPEC.md gap check

No delta needed. §11 and §11.9's text for this feature (coalesced against the tick,
best-effort, floor-respecting, no attached-client refusal case) was matched exactly by
the implementation above; nothing here contradicts SPEC.md's own words.
