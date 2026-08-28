# Task 108 — proving R86's on-screen text and the overlays' surviving line scroll

R86 (`↑`/`↓` navigate dialog fields; `tab` is completion only) was implemented in
approach 01, principally at task 025 (`internal/tui/dialog_contract.go`,
`internal/tui/tui.go`'s `updateCreate`, `cycleCreateCWDRecent`). This task is a
**proof** pass, not new implementation: it demonstrates every required piece of
on-screen text is correct and that the three surviving overlays (`?`/`i`/`E`)
still scroll one line per `↑`/`↓` press. No product code changed.

All commands below were run through `ci/run.sh` (sibling `deck-ci:local`); full
logs are in this directory.

## (a) On-screen text — grep evidence, by file:line

### `↑`/`↓` is field navigation; the contract only binds it when `Count > 1`

- `internal/tui/dialog_contract.go:82-92` — `applyDialogContract`'s `case "down"`/
  `case "up"`: mutates `*c.Fields.Index` only `if c.Fields.Count > 1`; declines
  (returns `handled=false`) otherwise. This is the ONE implementation every
  dialog below defers to.
- `internal/tui/tui.go:6144-6150` — the create modal is the only dialog that
  passes `Count: createFieldCount` (`internal/tui/tui.go:5802`, `= 8`) with a
  live `Index`, so it is the only dialog `↑`/`↓` actually moves between fields.
- `internal/tui/tui.go:4525`, `4679`, `4817`, `4970` — `updateProfileSwitch`,
  `updatePinDialog`, `updateRestartChoice`, `updateDeleteConfirm` each pass
  `dialogFields{Cycle: ...}` with **no** `Count`/`Index` (zero value, `Count 0`):
  `↑`/`↓` there stays declined by the contract, matching each one's footer
  correctly omitting a `↑/↓ field` legend (below) — there is nothing to move to.

### Every dialog footer (file:line, verbatim text)

| Dialog | file:line | Footer text |
|---|---|---|
| Create modal | `internal/tui/tui.go:6634` (plain) / `:6818` (styled) | `↑/↓ field · Left/Right/Space cycles · Enter submits · Esc cancels` |
| Profile switch | `internal/tui/tui.go:4574` (plain) / `:4646` (styled) | `Left/Right cycles · Enter confirms · Esc cancels` |
| Pin dialog | `internal/tui/tui.go:4718` (plain) / `:4783` (styled) | `Left/Right cycles · Enter confirms · Esc cancels` |
| Restart-or-inject | `internal/tui/tui.go:4872` (plain) / `:4939` (styled) | `Left/Right cycles · Enter confirms · Esc cancels` |
| Archive confirm | `internal/tui/tui.go:5132` (plain) / `:5200` (styled) | `Enter archives · Esc cancels` |
| Delete/purge confirm | `internal/tui/tui.go:5269` (plain) / `:5337` (styled) | `Enter deletes · Esc cancels` |
| Bulk delete confirm | `internal/tui/tui.go:5360` (`bulkDeleteSubmitText` const), applied at `:5541` | `Enter deletes all · Esc cancels` |
| Rename dialog | `internal/tui/rename.go:174` (plain) / `:254` (styled) | `Type a new name · Enter confirms · Esc cancels` |
| Env editor | `internal/tui/env_editor.go:197,203` (`envHintLine`) | edit mode: `Enter saves this key into the session's own env; Esc cancels this edit.`; browse mode: `j/k select a key, Enter edits it, r reveals/masks secret-shaped values; Esc closes.` |
| Event log | `internal/tui/event_log.go:74` (plain) / `:192` (styled) | `...Esc closes.` |

Only the create modal's footer names `↑/↓ field` — every single-field dialog
(profile switch, pin, restart-or-inject, delete confirm) correctly omits it
because their `dialogFields.Count` is 0 (table above); the rename dialog (one
free-text field, no `Cycle` at all) and the env editor (its own `j/k`
list-browse legend, outside `applyDialogContract` by design — it is not a
§11.4 dialog) likewise never claim a `↑/↓ field` binding they do not have. No
footer anywhere still advertises `tab`/`shift+tab` as navigation:
`grep -n 'Tab\|Shift+Tab' internal/tui/tui.go` inside any footer string
returns nothing (also directly asserted by
`TestCreateModalUpDownMoveFieldsTabDoesNot`, cited below).

### The create candidate-list caption (task 012/025, R86's own bullet: "keeps `↑`/`↓` while open ... caption stays true")

- `internal/tui/tui.go:6625` (plain) / `:6809` (styled):
  `"  candidates (up/down selects, enter or tab accepts, esc closes):"`
  — `up`/`down` still select within the open candidate list (handled inside
  `updateCreate`'s own `case "up"`/`case "down"` at `internal/tui/tui.go:6126,6136`,
  *before* `applyDialogContract` ever sees the keystroke — the candidate list's
  own `↑`/`↓` and the dialog's field-navigation `↑`/`↓` are mutually exclusive by
  construction, see the comment at `tui.go:6127-6130`), and `tab` there is
  candidate **acceptance** (`m.tabCompleteCreateCWD()`, `tui.go:6104`), not field
  navigation.

### `createCWDHelp` (R86's own bullet: "Recents move to `Ctrl+P`/`Ctrl+N`" and "`tab` completes")

- `internal/tui/tui.go:6499-6500`:
  ```go
  func (m Model) createCWDHelp() string {
      help := "the session's cwd; must exist and be a directory; Ctrl+P/Ctrl+N cycles recent history; right/end completes a shown directory match"
  ```
  States `Ctrl+P/Ctrl+N cycles recent history` — never `↑`/`↓` for recents.
- `internal/tui/tui.go:6507-6509` (ambiguous-match branch): when the segment
  matches more than one candidate, the help line becomes
  `fmt.Sprintf("%d matches — tab to list ", count) + help` — `tab` is advertised
  as listing/completing candidates, never as moving to another field.
- `internal/tui/tui.go:5950-5964` (`cycleCreateCWDRecent`'s own doc comment):
  "`Ctrl+P`/`Ctrl+N` cycle recents shell-history style ... moved onto
  `Ctrl+P`/`Ctrl+N` by task 025" — matches the on-screen text exactly.
- `internal/tui/tui.go:6092-6103` (`updateCreate`'s `case "tab"` comment):
  "(task 025, SPEC §11.4) the ONLY thing tab ever does anywhere in this
  dialog: 'tab completes to the longest common prefix when that advances the
  text, and otherwise lists the candidates for selection'" — `tab` is bound
  **nowhere else** in the create dialog; confirmed by `grep -n 'case "tab"'
  internal/tui/*.go`, which finds exactly this one case in `tui.go` plus two
  in `settings.go` (out of scope, R86: "Not in scope: §11.5's settings
  takeover... Leave `settings.go` alone").

### The `?` overlay's own keymap state

- `internal/tui/tui.go:7056`: the "Create dialog fields" section's own
  closing line — `"  ↑/↓ changes field; ↵ advances or submits; Esc cancels"` —
  states `↑`/`↓` is field navigation for the create dialog, matches the
  footer above, and names no `tab` binding at all (`grep -n 'tab\|Tab' `
  restricted to the "Create dialog fields" block, `tui.go:7041-7056`, returns
  nothing).
- `internal/tui/tui.go:6918-6931` (the top-level "Keys" section's `↑`/`↓`
  entry): documents the **list-mode** meaning of `↑`/`↓` (select a session)
  and explicitly the overlay exception this task's item (c) proves: "While
  `?` help, `E` the event log or `i` the detail view covers the list, these
  same keys instead scroll that overlay's own content by exactly one line
  per press, leaving the session selection where it was".
- No line of `helpText` anywhere advertises `Ctrl+P`/`Ctrl+N` or `tab` for the
  create dialog (`grep -n 'Ctrl+P\|Ctrl+N\|[Tt]ab' ` over the `helpText`
  function body finds only the unrelated Settings-takeover `Tab or
  Left/Right` line, `tui.go:7072`, which R86 explicitly excludes) — that
  vocabulary is stated once, in `createCWDHelp` and the candidate caption
  above, never duplicated or contradicted in the overlay.

### Automated parity nets that would catch a half-done change (R86's own requirement)

- `internal/tui/help_keymap_parity_test.go` — `TestHelpOverlayKeymapMatchesBoundKeys`
  (asserts, in both directions, every list-mode-bound key is documented in
  `helpText`'s "Keys" section and vice versa) and
  `TestFooterKeyLegendNamesOnlyBoundKeys` (same, for the list-mode footer).
- `internal/tui/footer_bindings_parity_test.go` — `TestFooterEntriesNameOnlyBoundKeysViaSourceParse`,
  `TestFooterEntryEligibilityMatchesRealPredicate`, `TestFooterFixedSetMatchesSpecAndExcludesRareKeys`.
- `internal/tui/dialog_contract_test.go:62-70` (`TestApplyDialogContractCoreKeys`'s
  `"tab and shift+tab are not contract keys"` subtest) and
  `:130-159` (`TestCreateModalUpDownMoveFieldsTabDoesNot`): directly prove `↑`/`↓`
  move the create modal's focused field, `tab`/`shift+tab` do not, and
  `createView()`'s own rendered text never contains `"Tab/Shift+Tab"` or
  `"Tab or Shift+Tab"`.

All pass — see `go-test-parity-verbose.log` for the five help/footer parity
tests, and `go-test-tui.log` for the package as a whole (which also runs
`dialog_contract_test.go`).

## (b) `help_keymap_parity_test.go` and `footer_bindings_parity_test.go` pass

```
$ ci/run.sh go test -count=1 -v -run 'TestHelpOverlayKeymapMatchesBoundKeys|TestFooterKeyLegendNamesOnlyBoundKeys|TestFooterEntriesNameOnlyBoundKeysViaSourceParse|TestFooterEntryEligibilityMatchesRealPredicate|TestFooterFixedSetMatchesSpecAndExcludesRareKeys' ./internal/tui/
=== RUN   TestFooterEntriesNameOnlyBoundKeysViaSourceParse
--- PASS: TestFooterEntriesNameOnlyBoundKeysViaSourceParse (0.00s)
=== RUN   TestFooterEntryEligibilityMatchesRealPredicate
--- PASS: TestFooterEntryEligibilityMatchesRealPredicate (0.00s)
=== RUN   TestFooterFixedSetMatchesSpecAndExcludesRareKeys
--- PASS: TestFooterFixedSetMatchesSpecAndExcludesRareKeys (0.01s)
=== RUN   TestHelpOverlayKeymapMatchesBoundKeys
--- PASS: TestHelpOverlayKeymapMatchesBoundKeys (0.00s)
=== RUN   TestFooterKeyLegendNamesOnlyBoundKeys
--- PASS: TestFooterKeyLegendNamesOnlyBoundKeys (0.00s)
PASS
ok  	github.com/n-orlov/deck/internal/tui	0.032s
```
Full output: `go-test-parity-verbose.log`. Both files also ran as part of the
whole-package run: `go-test-tui.log` (`ok github.com/n-orlov/deck/internal/tui`).

## (c) `?`/`i`/`E` still scroll exactly one line on `↑`/`↓` under task 025's dialog contract

`internal/tui/overlay_line_scroll_test.go` already covers all three overlays
through the **real** `Model.Update` path (`pressKey`, `overlay_line_scroll_test.go:142-150`,
calling `m.Update(key(name))`), which for each overlay first calls
`applyDialogContract` (task 025's dialog contract — the same function R86
extended) and only reaches the overlay's own `up`/`down` scroll case once the
contract declines (`Count` 0/1 for all three, since none has navigable
fields):

- `internal/tui/tui.go:2321-2331` — `Model.Update`'s dispatch: `m.help` →
  `updateHelpView` (`internal/tui/tui.go:6888`), `m.detail` →
  `updateDetailView` (`internal/tui/rename.go:42`), `m.eventLogOpen` →
  `updateEventLog` (`internal/tui/event_log.go:108`).
- `internal/tui/tui.go:6888-6892` (`updateHelpView`),
  `internal/tui/rename.go:43-50` (`updateDetailView`) and
  `internal/tui/event_log.go:108-112` (`updateEventLog`) each call
  `applyDialogContract(msg, dialogContract{Cancel: ...})` FIRST — no `Fields`
  at all (zero value, `Count` 0) — before falling to their own `case "up", "k"`
  / `case "down", "j"` one-line-scroll cases (`tui.go:6905-6910`,
  `rename.go:71-73`, `event_log.go:119-123`).
- Fixtures, one per overlay: `overlay_line_scroll_test.go:121` (`"help overlay"`,
  `?`), `:127` (`"detail view"`, `i`), `:133` (`"event log"`, `E`).
- Named assertions, all three overlays as subtests:
  - `TestScrollableOverlaysScrollOneLineWithArrows` (`overlay_line_scroll_test.go:156`) —
    subtests `help_overlay`, `detail_view`, `event_log`; each presses `down`
    twice then `up` once and requires (`assertScrolledByOneLine`,
    `overlay_line_scroll_test.go:46-72`) the first visible row advance by
    exactly one wrapped line each press.
  - `TestScrollableOverlaysScrollOneLineWithJK` (`:184`) — same three subtests,
    `j`/`k` (the sidebar's own aliases) instead of the arrow keys.
  - `TestScrollableOverlaysStillPageWithPgDn` (`:213`) — same three subtests,
    proving `PgUp`/`PgDn` still take a whole page (the negative control: a
    page-step regression on `↑`/`↓` would look identical to "the view moved"
    without this).

Verbatim run, all nine subtests (3 tests × 3 overlays) green:

```
$ ci/run.sh go test -count=1 -v -run 'TestScrollableOverlaysScrollOneLineWithArrows|TestScrollableOverlaysScrollOneLineWithJK|TestScrollableOverlaysStillPageWithPgDn' ./internal/tui/
--- PASS: TestScrollableOverlaysScrollOneLineWithArrows (0.03s)
    --- PASS: TestScrollableOverlaysScrollOneLineWithArrows/help_overlay (0.00s)
    --- PASS: TestScrollableOverlaysScrollOneLineWithArrows/detail_view (0.00s)
    --- PASS: TestScrollableOverlaysScrollOneLineWithArrows/event_log (0.00s)
--- PASS: TestScrollableOverlaysScrollOneLineWithJK (0.03s)
    --- PASS: TestScrollableOverlaysScrollOneLineWithJK/help_overlay (0.00s)
    --- PASS: TestScrollableOverlaysScrollOneLineWithJK/detail_view (0.00s)
    --- PASS: TestScrollableOverlaysScrollOneLineWithJK/event_log (0.00s)
--- PASS: TestScrollableOverlaysStillPageWithPgDn (0.02s)
    --- PASS: TestScrollableOverlaysStillPageWithPgDn/help_overlay (0.00s)
    --- PASS: TestScrollableOverlaysStillPageWithPgDn/detail_view (0.00s)
    --- PASS: TestScrollableOverlaysStillPageWithPgDn/event_log (0.00s)
PASS
```
Full output: `go-test-overlay-line-scroll-verbose.log`.

**Conclusion**: existing coverage already exercises the contract path itself
(the overlay's own `up`/`down` case is only reachable because
`applyDialogContract` first declines, and that decline is exactly R86's "a
contract that now binds `↑`/`↓` is exactly what could take those keys back" —
if `applyDialogContract` ever changed to claim `↑`/`↓` unconditionally instead
of gating on `Count > 1`, this suite would go from `handled=true` (dead end,
overlay's own case never runs) straight to a scroll-offset assertion failure,
since the overlay's `up`/`down` case would then never execute). No new test
was needed; nothing was added to `overlay_line_scroll_test.go`.

## (d) `internal/tui/settings.go` untouched

```
$ git log --oneline 1cfbd5a..HEAD -- internal/tui/settings.go
```
Output: empty. See `settings-go-untouched.log` (0 bytes). Matches R86's own
scope note ("Not in scope: §11.5's settings takeover... Leave `settings.go`
alone") and the standing rules' identical prohibition.

## (e) Test runs, both exit 0

```
$ ci/run.sh go test -count=1 ./internal/tui/
ok  	github.com/n-orlov/deck/internal/tui	1.040s
```
Full output: `go-test-tui.log`. Exit code: 0.

```
$ ci/run.sh env DECK_GODOG_PATHS=dialogs.feature,create_session.feature,event_log.feature go test ./features/ -run TestFeatures -count=1
ok  	github.com/n-orlov/deck/features	31.561s
```
Full output: `go-test-features.log`. Exit code: 0.

## Files in this directory

- `README.md` — this file.
- `go-test-tui.log` — `ci/run.sh go test -count=1 ./internal/tui/` (criterion e, first command).
- `go-test-features.log` — the three-feature godog run (criterion e, second command).
- `go-test-parity-verbose.log` — verbose run of the five help/footer parity tests (criterion b).
- `go-test-overlay-line-scroll-verbose.log` — verbose run of the three overlay-line-scroll tests, all nine subtests (criterion c).
- `settings-go-untouched.log` — `git log --oneline 1cfbd5a..HEAD -- internal/tui/settings.go` output, empty (criterion d).

No existing test was loosened, skipped, or rewritten to produce any of the
above; no product code changed for this task.
