# Task 108 — proving R86's on-screen text and the overlays' surviving line scroll

R86 (`↑`/`↓` navigate dialog fields; `tab` is completion only) was implemented in
approach 01, principally at task 025 (`internal/tui/dialog_contract.go`,
`internal/tui/tui.go`'s `updateCreate`, `cycleCreateCWDRecent`). This task is a
**proof** pass, not new implementation: it demonstrates every required piece of
on-screen text is correct and that the three surviving overlays (`?`/`i`/`E`)
still scroll one line per `↑`/`↓` press. No product code changed.

Second pass: criterion (a) now quotes the search commands and their whole output
(`criterion-a-greps.log`) instead of a hand-assembled table, and its footer
inventory accounts for the detail view's footer (`tui.go:5660`) and the help
overlay's closing footer (`tui.go:7152`), which the first cut's table omitted.
Only files in this directory changed; every test log was re-captured at the same
code.

Every test command below was run through `ci/run.sh` (sibling `deck-ci:local`);
the grep/sed evidence of criterion (a) is plain text search over the checkout and
needs no toolchain. All logs are in this directory.

## (a) On-screen text — quoted grep commands and their whole output

Every command in this section was run verbatim from the repository root at HEAD
`ab34cb4`; the same commands and the same output are captured in
`criterion-a-greps.log` in this directory (sections A1–A10 there match the
sections here one-for-one).

### The complete footer/legend inventory (one command, whole output)

Rather than a hand-assembled list, the inventory is a single grep over the whole
package for the closing words every footer in this tree ends with. Every hit is
accounted for in the table underneath — including the two the first cut of this
report missed: the **detail view**'s footer (`tui.go:5660`) and the **help
overlay**'s own closing footer (`tui.go:7152`).

```
# A1 — every dialog/overlay footer and closing legend (product code only)
$ grep -rn --include='*.go' 'Esc cancels\|Esc closes\|Esc reverts\|closes detail\|closes help\|Enter archives\|Enter deletes' internal/tui/ | grep -v '_test.go' | grep -v ':[0-9]*:[[:space:]]*//'

internal/tui/theme_picker.go:162:		lines := wrapText("Theme picker: no themes available (no built-ins embedded and no user themes discovered) -- Esc closes.", width)
internal/tui/theme_picker.go:173:	header := fmt.Sprintf("Theme picker: %s (%d of %d)%sLeft/Right or Up/Down changes%sEnter selects%sEsc reverts",
internal/tui/env_editor.go:197:		return "Enter saves this key into the session's own env; Esc cancels this edit."
internal/tui/env_editor.go:203:	return fmt.Sprintf("j/k select a key, Enter edits it, %s; Esc closes.", revealHint)
internal/tui/event_log.go:74:	fmt.Fprintf(&b, "\nNewest first, up to the most recent %d events. Secret-shaped payload\nvalues mask the same way the env editor's do. Esc closes.\n", maxEventLogRows)
internal/tui/event_log.go:192:	colorFooterLine(fmt.Sprintf("\nNewest first, up to the most recent %d events. Secret-shaped payload\nvalues mask the same way the env editor's do. Esc closes.", maxEventLogRows))
internal/tui/tui.go:4574:	b.WriteString("\nLeft/Right cycles · Enter confirms · Esc cancels\n")
internal/tui/tui.go:4646:	colorFooterLine("Left/Right cycles · Enter confirms · Esc cancels")
internal/tui/tui.go:4718:	b.WriteString("\nLeft/Right cycles · Enter confirms · Esc cancels\n")
internal/tui/tui.go:4783:	colorFooterLine("Left/Right cycles · Enter confirms · Esc cancels")
internal/tui/tui.go:4872:	b.WriteString("\nLeft/Right cycles · Enter confirms · Esc cancels\n")
internal/tui/tui.go:4939:	colorFooterLine("Left/Right cycles · Enter confirms · Esc cancels")
internal/tui/tui.go:5132:	b.WriteString("\nEnter archives · Esc cancels\n")
internal/tui/tui.go:5200:	colorFooterLine("Enter archives · Esc cancels")
internal/tui/tui.go:5269:	b.WriteString("\nEnter deletes · Esc cancels\n")
internal/tui/tui.go:5337:	colorFooterLine("Enter deletes · Esc cancels")
internal/tui/tui.go:5360:	bulkDeleteSubmitText           = "Enter deletes all · Esc cancels"
internal/tui/tui.go:5660:	b.WriteString("\n" + m.glyph("r renames · i or Esc closes detail", "r renames - i or Esc closes detail") + "\n")
internal/tui/tui.go:6634:	b.WriteString("\u2191/\u2193 field · Left/Right/Space cycles · Enter submits · Esc cancels\n")
internal/tui/tui.go:6818:	colorFooterLine("\u2191/\u2193 field · Left/Right/Space cycles · Enter submits · Esc cancels")
internal/tui/tui.go:7014:    reopen; Esc cancels an edit in progress, or closes the dialog otherwise
internal/tui/tui.go:7017:    secret-shaped values mask the same way the env editor's do; Esc closes
internal/tui/tui.go:7029:  , open/close settings (edit config.toml's keys); Esc closes, prompting to
internal/tui/tui.go:7032:    Enter selects, Esc reverts byte-for-byte
internal/tui/tui.go:7038:  ? open/close help; Esc closes help
internal/tui/tui.go:7056:  ↑/↓ changes field; ↵ advances or submits; Esc cancels
internal/tui/tui.go:7135:                            nothing (Esc cancels)
internal/tui/tui.go:7152:? closes help; Esc closes help; q quits deck.
internal/tui/rename.go:174:	b.WriteString("\nType a new name · Enter confirms · Esc cancels\n")
internal/tui/rename.go:254:	colorFooterLine("Type a new name · Enter confirms · Esc cancels")

(exit status: 0 — output above is complete)
```

Accounting for **every** line A1 printed, so nothing above is unclassified:

| Dialog / overlay | file:line (plain / styled) | Footer text | Names `↑/↓`? |
|---|---|---|---|
| Create modal | `tui.go:6634` / `:6818` | `↑/↓ field · Left/Right/Space cycles · Enter submits · Esc cancels` | **yes — field navigation** |
| Profile switch | `tui.go:4574` / `:4646` | `Left/Right cycles · Enter confirms · Esc cancels` | no (`Count` 0, A8) |
| Pin dialog | `tui.go:4718` / `:4783` | `Left/Right cycles · Enter confirms · Esc cancels` | no (`Count` 0, A8) |
| Restart-or-inject | `tui.go:4872` / `:4939` | `Left/Right cycles · Enter confirms · Esc cancels` | no (`Count` 0, A8) |
| Archive confirm | `tui.go:5132` / `:5200` | `Enter archives · Esc cancels` | no (no `Fields`, A8) |
| Delete/purge confirm | `tui.go:5269` / `:5337` | `Enter deletes · Esc cancels` | no (`Count` 0, A8) |
| Bulk delete confirm | `tui.go:5360`/`:5361` consts, applied `:5383-5385`, styled `:5541` | `Enter deletes all · Esc cancels` (+ ` · PgUp/PgDn scrolls` when it scrolls) | no |
| Rename dialog | `rename.go:174` / `:254` | `Type a new name · Enter confirms · Esc cancels` | no (one text field) |
| **Detail view (`i`)** | `tui.go:5660` (`m.glyph`, plain + ASCII twin) | `r renames · i or Esc closes detail` | no — see (c): `↑/↓` scrolls it one line, and that is stated in the `?` overlay's Keys section (A10, `tui.go:6937`), not in this footer |
| Event log (`E`) | `event_log.go:74` / `:192` | `Newest first, up to the most recent N events. … Esc closes.` | no — same as the detail view |
| **Help overlay (`?`)** | `tui.go:7152` (closing line of `helpText`) | `? closes help; Esc closes help; q quits deck.` | no — same as the detail view |
| Env editor | `env_editor.go:197` (edit) / `:203` (browse) | `Enter saves this key into the session's own env; Esc cancels this edit.` / `j/k select a key, Enter edits it, r reveals/masks secret-shaped values; Esc closes.` | no — its own `j/k` list legend; not a §11.4 field dialog |
| Theme picker | `theme_picker.go:173` legend, `:162` empty state | `Theme picker: … Left/Right or Up/Down changes · Enter selects · Esc reverts` / `Theme picker: no themes available … -- Esc closes.` | names `Up/Down` as **changing the previewed theme**, not as field navigation — it is not a `dialogContract` caller at all (absent from A8; its own `case "left", "up"` / `case "right", "down", " "` at `theme_picker.go:100,105`) |

The remaining A1 hits — `tui.go:7014`, `:7017`, `:7029`, `:7032`, `:7038`,
`:7056`, `:7135` — are lines of `helpText`, i.e. the `?` overlay's keymap body,
covered under A10 below, not dialog footers.

**The one `↑/↓ field` claim in the tree is the create modal's**, and A8/A9 show
it is the only dialog whose `dialogFields` carries a `Count` (`createFieldCount`,
8) for `applyDialogContract` to move — so the footers that omit the legend omit
it because the binding genuinely does not exist there.

A footer that does not contain one of A1's literal words could still hide from
that command, so two more commands close the inventory. `colorFooterLine` is the
one helper that styles a footer, so it enumerates every themed footer line, and
its nine sites map 1:1 onto A1's nine plain dialog footers (create, profile
switch, pin, restart-or-inject, archive, delete/purge, bulk delete via
`plainTail[1]` at `:5541`, rename, event log — the env editor builds its hint
through `envHintLine` instead, `env_editor.go:197,203`):

```
# A1b — every styled footer site (colorFooterLine is the one themed-footer helper): the styled twin of each plain footer in A1
$ grep -rn --include='*.go' 'colorFooterLine(' internal/tui/ | grep -v '_test.go'

internal/tui/event_log.go:192:	colorFooterLine(fmt.Sprintf("\nNewest first, up to the most recent %d events. Secret-shaped payload\nvalues mask the same way the env editor's do. Esc closes.", maxEventLogRows))
internal/tui/tui.go:4646:	colorFooterLine("Left/Right cycles · Enter confirms · Esc cancels")
internal/tui/tui.go:4783:	colorFooterLine("Left/Right cycles · Enter confirms · Esc cancels")
internal/tui/tui.go:4939:	colorFooterLine("Left/Right cycles · Enter confirms · Esc cancels")
internal/tui/tui.go:5200:	colorFooterLine("Enter archives · Esc cancels")
internal/tui/tui.go:5337:	colorFooterLine("Enter deletes · Esc cancels")
internal/tui/tui.go:5541:	tail = colorFooterLine(tail, plainTail[1])
internal/tui/tui.go:6818:	colorFooterLine("\u2191/\u2193 field · Left/Right/Space cycles · Enter submits · Esc cancels")
internal/tui/rename.go:254:	colorFooterLine("Type a new name · Enter confirms · Esc cancels")

(exit status: 0 — output above is complete)
```

and the bulk-delete confirm builds its footer from constants — the scrollable
variant (`· PgUp/PgDn scrolls`) is a footer A1's literals cannot see:

```
# A1c — the bulk-delete footer constants, incl. the scrollable variant A1's literals cannot see
$ grep -rn --include='*.go' 'bulkDeleteSubmitText' internal/tui/ | grep -v '_test.go' | grep -v ':[0-9]*:[[:space:]]*//'

internal/tui/tui.go:5360:	bulkDeleteSubmitText           = "Enter deletes all · Esc cancels"
internal/tui/tui.go:5361:	bulkDeleteSubmitTextScrollable = bulkDeleteSubmitText + " · PgUp/PgDn scrolls"
internal/tui/tui.go:5383:	submit := bulkDeleteSubmitText
internal/tui/tui.go:5385:		submit = bulkDeleteSubmitTextScrollable

(exit status: 0 — output above is complete)
```

### `↑`/`↓` is field navigation, and only the create modal claims it

```
# A2 — every on-screen string that names up/down (arrows or words)
$ grep -rn --include='*.go' '↑/↓\|up/down\|Up/Down' internal/tui/ | grep -v '_test.go' | grep -v ':[0-9]*:[[:space:]]*//'

internal/tui/theme_picker.go:173:	header := fmt.Sprintf("Theme picker: %s (%d of %d)%sLeft/Right or Up/Down changes%sEnter selects%sEsc reverts",
internal/tui/help_style.go:56:	"↑/↓": true, "j/k": true, "↵": true, "a": true, "Y": true, "n": true,
internal/tui/tui.go:3637:	{"↑/↓", "up/down", "", nil},
internal/tui/tui.go:6625:		b.WriteString("  candidates (up/down selects, enter or tab accepts, esc closes):\n")
internal/tui/tui.go:6809:		colorWhole(theme.Hint, "  candidates (up/down selects, enter or tab accepts, esc closes):")
internal/tui/tui.go:6919:  ↑/↓ or j/k select a session; as the selection settles (coalesced
internal/tui/tui.go:6933:  PgUp/PgDn page up/down through the list, one page at a time; while ?
internal/tui/tui.go:6937:    pages share no line; ↑/↓ or j/k there move one line at a time, and
internal/tui/tui.go:7056:  ↑/↓ changes field; ↵ advances or submits; Esc cancels
internal/tui/tui.go:7074:  Up/Down or j/k         move within the focused list
internal/tui/tui.go:7085:  Left/Right or Up/Down or Space   change the previewed theme
internal/tui/tui.go:7122:  click a sidebar row       select it (like ↑/↓); the preview follows on
internal/tui/tui.go:7127:                            (like ↑/↓/PgUp/PgDn)
internal/tui/tui.go:7129:                            detail view by one line (like ↑/↓ or j/k);
internal/tui/settings.go:133:			"costs only the create modal's cwd prefill, up/down recent-cycling and " +
internal/tui/settings.go:1523:		return truncateToWidth("type to search - up/down select - enter jump - esc cancel", width)
internal/tui/settings.go:1529:		return truncateToWidth("up/down move - enter edit/add - - remove - r reveal/mask - esc back to fields", width)
internal/tui/settings.go:1531:	footer := "tab/left/right switch - up/down move - enter/+/- edit - / search - ctrl+s save - esc close"

(exit status: 0 — output above is complete)
```

On-screen `up/down` vocabulary, line by line: `tui.go:6634`/`:6818` (create
footer, field navigation), `tui.go:7056` (`?` overlay, same claim for the same
dialog), `tui.go:6625`/`:6809` (candidate-list caption — selection *within the
open list*, A5), `tui.go:6919`/`:6933-6937` (list mode, plus the overlay
one-line-scroll exception this task's (c) proves), `tui.go:7074`/`:7085`
(settings takeover and theme picker, both out of R86's scope by its own wording),
`tui.go:7122`/`:7127`/`:7129` (mouse equivalences), `theme_picker.go:173`
(picker legend). `tui.go:3637` is the list-mode footer entry table and
`help_style.go:56` its key-token allowlist — the parity tests in (b) are what
keep those two agreeing. `settings.go:133,1523,1529,1531` are §11.5's settings
takeover, which R86 explicitly leaves alone (and criterion (d) shows this job
never touched that file).

No line anywhere claims `↑`/`↓` cycles cwd recents — that moved to
`Ctrl+P`/`Ctrl+N` (A3).

### Recents are `Ctrl+P`/`Ctrl+N`, on screen and in the code that serves them

```
# A3 — every string that names Ctrl+P / Ctrl+N (comments kept: they state the same contract)
$ grep -rn --include='*.go' 'Ctrl+P\|Ctrl+N' internal/tui/ | grep -v '_test.go'

internal/tui/tui.go:109:	// currently cycling through (task 009), fetched once when Ctrl+P/Ctrl+N
internal/tui/tui.go:120:	// state from the moment before the first "Ctrl+P" started a cycle, so
internal/tui/tui.go:121:	// pressing "Ctrl+N" back past the most recent entry restores exactly
internal/tui/tui.go:5951:// per-field key set (task 009, moved onto Ctrl+P/Ctrl+N by task 025):
internal/tui/tui.go:5952:// Ctrl+P/Ctrl+N cycle recents shell-history style. delta is +1 for
internal/tui/tui.go:5953:// "Ctrl+P" (older) and -1 for "Ctrl+N" (newer, eventually exiting the
internal/tui/tui.go:5956:// The first "Ctrl+P" snapshots both the recent_cwds list itself (so it
internal/tui/tui.go:5958:// plus its prefilled/last-used flags) so "Ctrl+N" can restore it exactly
internal/tui/tui.go:5962:// "Ctrl+N" with no cycle in progress, or a store with no history at all,
internal/tui/tui.go:5985:		// Ran "Ctrl+N" back past the most recent entry: exit the cycle and
internal/tui/tui.go:5986:		// restore exactly what was there before the first "Ctrl+P".
internal/tui/tui.go:5994:		// "Ctrl+P" at the oldest entry stays there rather than wrapping --
internal/tui/tui.go:6129:			// recent_cwds (Ctrl+P/Ctrl+N, below): the three per-field key sets
internal/tui/tui.go:6158:		// 009, moved off up/down onto Ctrl+P/Ctrl+N by task 025 once ↑/↓
internal/tui/tui.go:6162:		// every other field's Ctrl+P/Ctrl+N stays a no-op here.
internal/tui/tui.go:6500:	help := "the session's cwd; must exist and be a directory; Ctrl+P/Ctrl+N cycles recent history; right/end completes a shown directory match"

(exit status: 0 — output above is complete)
```

`tui.go:6500` is the only *on-screen* hit: `createCWDHelp` states
`Ctrl+P/Ctrl+N cycles recent history`. Every other hit is the implementation and
its doc comment (`cycleCreateCWDRecent`, `tui.go:5951-5998`; the `updateCreate`
dispatch at `:6158-6162`, whose comment records the move "off up/down onto
Ctrl+P/Ctrl+N by task 025 once ↑/↓ became field navigation"), so the text and the
binding cannot disagree without one of them being edited.

### `tab` is advertised only as path completion

```
# A4 — every on-screen string literal that names tab
$ grep -rni --include='*.go' '"[^"]*\btab\b[^"]*"' internal/tui/ | grep -v '_test.go' | grep -v ':[0-9]*:[[:space:]]*//'

internal/tui/tui.go:6092:		case "tab":
internal/tui/tui.go:6509:			return fmt.Sprintf("%d matches \u2014 tab to list ", count) + help
internal/tui/tui.go:6625:		b.WriteString("  candidates (up/down selects, enter or tab accepts, esc closes):\n")
internal/tui/tui.go:6809:		colorWhole(theme.Hint, "  candidates (up/down selects, enter or tab accepts, esc closes):")
internal/tui/settings.go:134:			"tab completion candidates (§11.7) the next time it opens. It never " +
internal/tui/settings.go:233:	case "tab", "left", "right":
internal/tui/settings.go:630:	case "tab":
internal/tui/settings.go:1526:		return truncateToWidth("type to edit - tab switch key/value - enter on value saves the entry - esc cancels", width)
internal/tui/settings.go:1531:	footer := "tab/left/right switch - up/down move - enter/+/- edit - / search - ctrl+s save - esc close"

(exit status: 0 — output above is complete)
```

Outside `settings.go` (out of scope), `tab` appears in exactly three on-screen
strings, all of them path completion: `tui.go:6509` (`N matches — tab to list`),
and the candidate caption at `:6625`/`:6809` (`enter or tab accepts`). The only
`case "tab"` in the create dialog is `tui.go:6092`, whose comment pins it to
SPEC §11.4's "tab completes to the longest common prefix … otherwise lists the
candidates". Nothing advertises `tab` as moving between fields (A7).

### The create candidate-list caption

```
# A5 — the create candidate-list caption
$ grep -rn --include='*.go' 'candidates (' internal/tui/ | grep -v '_test.go'

internal/tui/tui.go:6625:		b.WriteString("  candidates (up/down selects, enter or tab accepts, esc closes):\n")
internal/tui/tui.go:6809:		colorWhole(theme.Hint, "  candidates (up/down selects, enter or tab accepts, esc closes):")
internal/tui/settings.go:134:			"tab completion candidates (§11.7) the next time it opens. It never " +

(exit status: 0 — output above is complete)
```

`up/down` here selects inside the open candidate list — handled by
`updateCreate`'s own `case "up"`/`case "down"` (`tui.go:6126,6136`) *before*
`applyDialogContract` sees the key, so list selection and field navigation are
mutually exclusive by construction — and `tab` accepts a candidate. The caption
says exactly that.

### `createCWDHelp`

```
# A6 — createCWDHelp's own text, plus its ambiguous-match branch
$ sed -n '6499,6510p' internal/tui/tui.go

func (m Model) createCWDHelp() string {
	help := "the session's cwd; must exist and be a directory; Ctrl+P/Ctrl+N cycles recent history; right/end completes a shown directory match"
	// Ambiguous-match counting (task 011, requirement 15) only applies
	// while this field is actually focused and being typed into, exactly
	// like createCWDDisplayValue's ghost: "the cursor is at end of field"
	// only means something during an edit of THIS field, so another
	// field's rendering of this same row never shows a stale count left
	// over from before the user tabbed away.
	if m.createField == 1 {
		if count, ok := createCWDAmbiguousMatchCount(m.createCWD); ok {
			return fmt.Sprintf("%d matches \u2014 tab to list ", count) + help
		}

(exit status: 0 — output above is complete)
```

`Ctrl+P/Ctrl+N cycles recent history` — never `↑`/`↓` — and the ambiguous-match
branch (`tui.go:6507-6509`, quoted in A4) advertises `tab` as *listing* matches.

### Negative control: nothing claims `tab`/`shift+tab` moves between fields

```
# A7 — negative control: nothing outside settings.go advertises tab/shift+tab as field navigation
$ grep -rn --include='*.go' 'Tab/Shift+Tab\|Tab or Shift+Tab\|shift+tab\|[Tt]ab.*field' internal/tui/ | grep -v '_test.go' | grep -v ':[0-9]*:[[:space:]]*//' | grep -v 'internal/tui/settings.go'


(exit status: 1 — grep exits 1 when nothing matched: the empty output above IS the assertion)
```

Empty. The only near-miss the filter drops is `settings.go` (out of scope) and a
comment at `tui.go:4594` that says the opposite ("there is no Tab between fields
here"). `internal/tui/dialog_contract_test.go:62-70`
(`TestApplyDialogContractCoreKeys`'s `"tab and shift+tab are not contract keys"`)
and `:130-159` (`TestCreateModalUpDownMoveFieldsTabDoesNot`) assert the same
thing executably, including that `createView()`'s rendered text never contains
`"Tab/Shift+Tab"`.

### The gating this evidence has to trace to: every `dialogFields` literal

```
# A8 — what the contract actually gates on: every dialogFields literal in the tree
$ grep -rn -A5 --include='*.go' 'Fields: dialogFields{' internal/tui/ | grep -v '_test.go'

--
internal/tui/tui.go:4525:		Fields: dialogFields{Cycle: cycle},
internal/tui/tui.go-4526-		Cancel: func() {
internal/tui/tui.go-4527-			m.profileSwitching = false
internal/tui/tui.go-4528-			m.profileSwitchNote = ""
internal/tui/tui.go-4529-		},
internal/tui/tui.go-4530-		Submit: func() tea.Cmd {
--
internal/tui/tui.go:4679:		Fields: dialogFields{Cycle: func(delta int) {
internal/tui/tui.go-4680-			m.pinValue = cycleOption(resumeModeOptions, m.pinValue, delta)
internal/tui/tui.go-4681-		}},
internal/tui/tui.go-4682-		Cancel: func() {
internal/tui/tui.go-4683-			m.pinning = false
internal/tui/tui.go-4684-			m.pinNote = ""
--
internal/tui/tui.go:4817:		Fields: dialogFields{Cycle: func(delta int) {
internal/tui/tui.go-4818-			m.restartChoiceValue = cycleOption(restartChoiceOptions, m.restartChoiceValue, delta)
internal/tui/tui.go-4819-		}},
internal/tui/tui.go-4820-		Cancel: func() {
internal/tui/tui.go-4821-			m.restartChoosing = false
internal/tui/tui.go-4822-			m.restartChoiceNote = ""
--
internal/tui/tui.go:4970:		Fields: dialogFields{Cycle: func(delta int) {
internal/tui/tui.go-4971-			m.deletePurgeValue = cycleOption(deletePurgeOptions, m.deletePurgeValue, delta)
internal/tui/tui.go-4972-		}},
internal/tui/tui.go-4973-		Cancel: func() {
internal/tui/tui.go-4974-			m.deleteConfirming = false
internal/tui/tui.go-4975-			m.deleteNote = ""
--
internal/tui/tui.go:6144:		Fields: dialogFields{
internal/tui/tui.go-6145-			Count:          createFieldCount,
internal/tui/tui.go-6146-			Index:          &m.createField,
internal/tui/tui.go-6147-			Cycle:          m.cycleCreateField,
internal/tui/tui.go-6148-			SpaceTypesText: func() bool { return createFieldIsText(m.createField) },
internal/tui/tui.go-6149-		},
--
internal/tui/rename.go:90:		Fields: dialogFields{SpaceTypesText: func() bool { return true }},
internal/tui/rename.go-91-		Cancel: func() {
internal/tui/rename.go-92-			m.renaming = false
internal/tui/rename.go-93-			m.renameValue, m.renamePrefilled, m.renameNote = "", false, ""
internal/tui/rename.go-94-		},
internal/tui/rename.go-95-		Submit: m.submitRename,

(exit status: 0 — output above is complete)
```

`tui.go:6144-6149` (create modal) is the only literal with `Count`/`Index`;
`tui.go:4525`, `:4679`, `:4817`, `:4970` (profile switch, pin, restart-or-inject,
delete/purge) pass `Cycle` only, and `rename.go:90` only `SpaceTypesText`. The
archive confirm and the three overlays (`tui.go:6889`, `rename.go:43`,
`event_log.go:109`) pass no `Fields` at all.

### …and the one place `Count` is read

```
# A9 — applyDialogContract's up/down case: it moves a field only when Count > 1
$ sed -n '78,95p' internal/tui/dialog_contract.go

		if c.Submit == nil {
			return nil, false
		}
		return c.Submit(), true
	case "down":
		if c.Fields.Count <= 1 || c.Fields.Index == nil {
			return nil, false
		}
		*c.Fields.Index = (*c.Fields.Index + 1) % c.Fields.Count
		return nil, true
	case "up":
		if c.Fields.Count <= 1 || c.Fields.Index == nil {
			return nil, false
		}
		*c.Fields.Index = (*c.Fields.Index - 1 + c.Fields.Count) % c.Fields.Count
		return nil, true
	case "left":
		if c.Fields.Cycle == nil {

(exit status: 0 — output above is complete)
```

`Count <= 1` declines, so every dialog above except the create modal leaves
`↑`/`↓` to whatever comes after the contract — which for `?`/`i`/`E` is their own
one-line scroll (criterion (c)). This is the single implementation each footer in
the table is checked against, so the evidence traces to the gating and not merely
to matching prose.

### The `?` overlay's own keymap state

```
# A10 — the ? overlay's own keymap lines for the create dialog and the overlay arrow exception
$ sed -n '7041,7056p' internal/tui/tui.go

Create dialog fields
  Name                the display name; also the source of the tmux slug
  Working directory    the session's cwd; must exist and be a directory
  Agent               shell, claude, or pi; which adapter launches it
  Permission profile  how much the agent may do without asking (safe, plan,
                      edits, yolo); an unsupported profile for the chosen
                      agent degrades visibly instead of silently
  Launch args         extra argv appended after the adapter's own argv
  Env                 session-level environment variables (highest priority
                      in PATH resolution)
  Pre-launch command  runs in the pane before the agent starts; the SPEC's
                      intended use is loading secrets into the pane's
                      environment without deck ever storing or logging them
  Login shell         run the pane via $SHELL -lc instead of execing the
                      agent argv directly
  ↑/↓ changes field; ↵ advances or submits; Esc cancels

(exit status: 0 — output above is complete)

$ sed -n '6933,6938p' internal/tui/tui.go

  PgUp/PgDn page up/down through the list, one page at a time; while ?
    help, E the event log or i the detail view covers the list, the
    same keys instead page that overlay's own content once it grows
    taller than the frame -- a whole page per press, so consecutive
    pages share no line; ↑/↓ or j/k there move one line at a time, and
    with mouse reporting on a wheel notch over the overlay does the same

(exit status: 0 — output above is complete)

$ sed -n '7150,7153p' internal/tui/tui.go

  copy text while deck is running

? closes help; Esc closes help; q quits deck.
`

(exit status: 0 — output above is complete)
```

The create-dialog section's closing line (`tui.go:7056`) states `↑/↓ changes
field` and names no `tab` binding; the Keys section (`tui.go:6933-6938`) states
the overlay exception criterion (c) proves — while `?`, `E` or `i` covers the
list, `↑/↓ or j/k there move one line at a time`; and `tui.go:7152` is the
overlay's own closing footer, which binds only `?`, `Esc` and `q`. No line of
`helpText` offers `Ctrl+P`/`Ctrl+N` or `tab` for the create dialog — that
vocabulary is stated once, in `createCWDHelp` and the candidate caption (A3–A6),
and never contradicted here.

### Automated parity nets that would catch a half-done change (R86's own requirement)

- `internal/tui/help_keymap_parity_test.go` — `TestHelpOverlayKeymapMatchesBoundKeys`
  (asserts, in both directions, every list-mode-bound key is documented in
  `helpText`'s "Keys" section and vice versa) and
  `TestFooterKeyLegendNamesOnlyBoundKeys` (same, for the list-mode footer).
- `internal/tui/footer_bindings_parity_test.go` — `TestFooterEntriesNameOnlyBoundKeysViaSourceParse`,
  `TestFooterEntryEligibilityMatchesRealPredicate`, `TestFooterFixedSetMatchesSpecAndExcludesRareKeys`.
- `internal/tui/dialog_contract_test.go:62-70` and `:130-159` — quoted under A7 above.

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
--- PASS: TestFooterFixedSetMatchesSpecAndExcludesRareKeys (0.02s)
=== RUN   TestHelpOverlayKeymapMatchesBoundKeys
--- PASS: TestHelpOverlayKeymapMatchesBoundKeys (0.00s)
=== RUN   TestFooterKeyLegendNamesOnlyBoundKeys
--- PASS: TestFooterKeyLegendNamesOnlyBoundKeys (0.00s)
PASS
ok  	github.com/n-orlov/deck/internal/tui	0.037s
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
--- PASS: TestScrollableOverlaysScrollOneLineWithJK (0.02s)
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
ok  	github.com/n-orlov/deck/internal/tui	1.018s
```
Full output: `go-test-tui.log`. Exit code: 0.

```
$ ci/run.sh env DECK_GODOG_PATHS=dialogs.feature,create_session.feature,event_log.feature go test ./features/ -run TestFeatures -count=1
ok  	github.com/n-orlov/deck/features	31.117s
```
Full output: `go-test-features.log`. Exit code: 0.

## Files in this directory

- `README.md` — this file.
- `go-test-tui.log` — `ci/run.sh go test -count=1 ./internal/tui/` (criterion e, first command).
- `go-test-features.log` — the three-feature godog run (criterion e, second command).
- `go-test-parity-verbose.log` — verbose run of the five help/footer parity tests (criterion b).
- `go-test-overlay-line-scroll-verbose.log` — verbose run of the three overlay-line-scroll tests, all nine subtests (criterion c).
- `settings-go-untouched.log` — `git log --oneline 1cfbd5a..HEAD -- internal/tui/settings.go` output, empty (criterion d).
- `criterion-a-greps.log` — the ten grep/sed commands of criterion (a) with their
  whole output and exit status, as quoted in this file (added on the second pass,
  after validation asked for quoted command output instead of a hand-assembled
  table, and for the detail-view and help-overlay footers the first table omitted).

No existing test was loosened, skipped, or rewritten to produce any of the
above; no product code changed for this task.
