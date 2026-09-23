package tui

import (
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// This file is requirement 38 (I-19 first half): a cross-check test that
// compares the help overlay's stated keymap against the keys actually
// bound in internal/tui and fails in either direction -- a bound key the
// help omits, and a help line for a key that is not bound.
//
// Both sides are read from the live source at test time, not copied into
// a table of "the keys as of this commit": listModeBoundKeys re-parses
// tui.go's own switch statement, and helpKeysSectionEntries re-parses
// helpText's own "Keys" section. Only the *symbol vocabulary* --
// translating a display glyph like "↑/↓" or "Ctrl+C" into the bubbletea
// key string it names -- is a fixed table (helpKeyTokenToBoundKeys), and
// even that table participates in the check: a help line whose leading
// token is not in the table is, correctly, not recognised as naming any
// key, so an unbound word appearing there cannot silently pass by
// accident (see TestHelpOverlayKeymapMatchesBoundKeys's discussion of
// direction 2 below).

// listModeSwitchCaseRe matches a `case "a", "b":` line: every case in the
// list-mode switch is a plain string literal (or comma-joined list of
// them) on `msg.String()`, never a type switch or a bare identifier, so
// this pattern captures every bound key with nothing hand-picked.
var listModeSwitchCaseRe = regexp.MustCompile(`(?m)^\t\tcase ((?:"[^"]*"(?:, )?)+):`)

// caseLiteralRe pulls each individual quoted string literal out of a
// matched case list, e.g. `"q", "ctrl+c"` -> [`"q"`, `"ctrl+c"`]. It is
// used instead of strings.Split(list, ",") because one of the bound keys
// is itself the literal "," (the settings key), which a naive split on
// every comma would tear in half.
var caseLiteralRe = regexp.MustCompile(`"[^"]*"`)

// listModeBoundKeys extracts every raw bubbletea key string bound in
// Update's top-level list-mode key switch: the `switch msg.String() {
// case "q", "ctrl+c": ... }` block inside the tea.KeyMsg case, reached
// only once every dialog/overlay's own early-return above it (creating,
// profileSwitching, pinning, envEditing, restartChoosing,
// deleteConfirming, settingsOpen, themePicking, renaming, detail,
// eventLogOpen, filtering, pendingDelete) has declined the message --
// i.e. the keymap `?` help documents. It is anchored on the exact
// `switch msg.String() {` this switch opens with (the first occurrence
// in the file -- the create-dialog cwd-field switches at the same tag
// text further down are a different, narrower keymap for a single field
// and are correctly excluded by taking the *first* occurrence) and
// closed by the next `case tea.MouseMsg:`, which is what follows this
// switch's closing brace.
func listModeBoundKeys(t *testing.T) map[string]bool {
	t.Helper()
	data, err := os.ReadFile("tui.go")
	if err != nil {
		t.Fatalf("ReadFile(tui.go): %v", err)
	}
	src := string(data)

	start := strings.Index(src, "switch msg.String() {")
	if start < 0 {
		t.Fatalf("could not find the list-mode key switch (`switch msg.String() {`) in tui.go -- extraction is broken, not the source")
	}
	relEnd := strings.Index(src[start:], "\n\tcase tea.MouseMsg:")
	if relEnd < 0 {
		t.Fatalf("could not find the end of the list-mode key switch (`case tea.MouseMsg:`) in tui.go -- extraction is broken, not the source")
	}
	block := src[start : start+relEnd]

	keys := map[string]bool{}
	for _, m := range listModeSwitchCaseRe.FindAllStringSubmatch(block, -1) {
		// Split on each quoted literal directly (not on every comma --
		// one of the bound keys is itself the literal ","), so a case
		// list like `case "q", "ctrl+c":` yields exactly its two
		// literals and `case ",":` yields exactly one.
		for _, lit := range caseLiteralRe.FindAllString(m[1], -1) {
			key, err := strconv.Unquote(lit)
			if err != nil {
				t.Fatalf("could not unquote case literal %q: %v", lit, err)
			}
			keys[key] = true
		}
	}
	if len(keys) < 20 {
		t.Fatalf("only found %d bound keys in the list-mode switch, expected 30+ -- extraction is broken, not the source", len(keys))
	}
	return keys
}

// helpKeyTokenToBoundKeys translates a help-text display token (the
// leading, key-naming word(s) of one "Keys" section line) into the raw
// bubbletea key string(s) it stands for. "or" is a connector between two
// alternatives on the same line (e.g. "q or Ctrl+C") and names no key of
// its own.
var helpKeyTokenToBoundKeys = map[string][]string{
	"↑/↓":       {"up", "down"},
	"←/→":       {"left", "right"},
	"j/k":       {"j", "k"},
	"↵":         {"enter"},
	"a":         {"a"},
	"F":         {"F"},
	"Y":         {"Y"},
	"n":         {"n"},
	"x":         {"x"},
	"u":         {"u"},
	"dd":        {"d"},
	"A":         {"A"},
	"U":         {"U"},
	"m":         {"m"},
	"r":         {"r"},
	"R":         {"R"},
	"P":         {"P"},
	"p":         {"p"},
	"i":         {"i"},
	"e":         {"e"},
	"E":         {"E"},
	"/":         {"/"},
	"space":     {" "},
	"c":         {"c"},
	"g":         {"g"},
	"G":         {"G"},
	",":         {","},
	"t":         {"t"},
	"|":         {"|"},
	"<":         {"<"},
	">":         {">"},
	"?":         {"?"},
	"q":         {"q"},
	"Ctrl+C":    {"ctrl+c"},
	"PgUp/PgDn": {"pgup", "pgdown"},
	"or":        nil,
}

// helpKeysSectionEntries returns the "Keys" section's own lines from
// helpText(false), one per bound key or key-group, in source order. A
// continuation line (indented 4+ spaces, wrapping the previous entry's
// description) is not a new entry. The section is delimited by the
// literal "Keys" header line above it and the first line that is not
// indented at all below it (the next heading).
func helpKeysSectionEntries(t *testing.T) []string {
	t.Helper()
	lines := strings.Split(helpText(false), "\n")
	start := -1
	for i, l := range lines {
		if l == "Keys" {
			start = i + 1
			break
		}
	}
	if start < 0 {
		t.Fatalf("could not find the \"Keys\" section header in helpText -- extraction is broken, not the source")
	}
	var out []string
	for i := start; i < len(lines); i++ {
		l := lines[i]
		if !strings.HasPrefix(l, "  ") {
			break // blank line or next heading: section ended
		}
		if strings.HasPrefix(l, "   ") {
			continue // 4+-space continuation of the previous entry
		}
		out = append(out, strings.TrimPrefix(l, "  "))
	}
	if len(out) == 0 {
		t.Fatalf("\"Keys\" section parsed to zero entries -- extraction is broken, not the source")
	}
	return out
}

// leadingKeyTokens walks a Keys-section line's whitespace-separated
// tokens from the start and returns every one that names a key (per
// helpKeyTokenToBoundKeys), stopping at the first token that does not --
// which is always the first word of the line's prose description, since
// every real entry's key phrase is exactly its leading token(s) (single
// key, "X/Y" pair, or "X or Y"/"X / Y" alternation).
func leadingKeyTokens(line string) []string {
	var toks []string
	for _, f := range strings.Fields(line) {
		if _, ok := helpKeyTokenToBoundKeys[f]; !ok {
			break
		}
		toks = append(toks, f)
	}
	return toks
}

// helpDocumentedKeys is helpKeysSectionEntries translated through
// leadingKeyTokens and helpKeyTokenToBoundKeys into the set of raw bound
// -key strings the "Keys" section documents.
func helpDocumentedKeys(t *testing.T) map[string]bool {
	t.Helper()
	documented := map[string]bool{}
	for _, entry := range helpKeysSectionEntries(t) {
		toks := leadingKeyTokens(entry)
		if len(toks) == 0 {
			t.Fatalf("Keys section entry %q has no recognised leading key token -- helpKeyTokenToBoundKeys needs a new entry, or the line's wording changed", entry)
		}
		for _, tok := range toks {
			for _, key := range helpKeyTokenToBoundKeys[tok] {
				documented[key] = true
			}
		}
	}
	return documented
}

// escIsDocumentedElsewhere is the one, deliberate, named exception: "esc"
// is bound at the top level (closes help, clears the mark set), but its
// behaviour is genuinely contextual -- it means something different in
// every dialog/overlay it also closes -- so it is documented inline next
// to each of those, in the "?" line's own description and in the help
// text's closing sentence, rather than as its own leading "Keys" section
// entry. This function re-checks that documentation is still actually
// there rather than silently trusting the exception forever: if the
// wording it looks for is ever removed, this fails too, so the exception
// cannot quietly become "esc is undocumented".
func escIsDocumentedElsewhere(t *testing.T) bool {
	t.Helper()
	text := helpText(false)
	return strings.Contains(text, "Esc closes help") && strings.Contains(text, "esc") && strings.Contains(text, "closes help; q quits deck")
}

// TestHelpOverlayKeymapMatchesBoundKeys is requirement 38's cross-check
// test (I-19 first half): it fails in either direction --
//
//   - a key bound in the list-mode switch that the help overlay's "Keys"
//     section never mentions (direction 1 -- this is how the PgUp/PgDn
//     gap this task fixed was found: both keys page the list
//     (tui.go's `case "pgup":`/`case "pgdown":`) but were, until this
//     commit, named only in a Mouse-section parenthetical ("like
//     ↑/↓/PgUp/PgDn"), never as their own Keys-section entry);
//   - a "Keys" section entry naming a key that the switch does not bind
//     (direction 2 -- demonstrated by temporarily appending a
//     "  Z do something fictional" line during development; see the
//     commit message for the captured red log, reverted before commit).
func TestHelpOverlayKeymapMatchesBoundKeys(t *testing.T) {
	bound := listModeBoundKeys(t)
	documented := helpDocumentedKeys(t)

	var missingFromHelp []string
	for key := range bound {
		if key == "esc" {
			if !escIsDocumentedElsewhere(t) {
				missingFromHelp = append(missingFromHelp, key+" (the esc exception's own documentation went missing)")
			}
			continue
		}
		if !documented[key] {
			missingFromHelp = append(missingFromHelp, key)
		}
	}
	sort.Strings(missingFromHelp)
	if len(missingFromHelp) > 0 {
		t.Errorf("bound key(s) with no help-overlay entry: %v", missingFromHelp)
	}

	var undocumentedInHelp []string
	for key := range documented {
		if !bound[key] {
			undocumentedInHelp = append(undocumentedInHelp, key)
		}
	}
	sort.Strings(undocumentedInHelp)
	if len(undocumentedInHelp) > 0 {
		t.Errorf("help-overlay entry names key(s) that are not bound: %v", undocumentedInHelp)
	}
}

// TestFooterKeyLegendNamesOnlyBoundKeys is task 062's footer half of
// requirement 38's cross-check: it reuses task 021's own two building
// blocks -- listModeBoundKeys (re-parses tui.go's list-mode key switch)
// and helpKeyTokenToBoundKeys (the fixed display-glyph -> raw-key-string
// vocabulary already used to check the help overlay) -- to prove the
// list-mode footer's key legend (footerLegend, tui.go) never names a key
// that switch does not bind. Every footerLegend entry's unicodeKey is
// already one of helpKeyTokenToBoundKeys' keys (the legend and the help
// overlay share the same glyph vocabulary by construction), so a missing
// translation is itself a failure worth reporting, not a silent skip.
func TestFooterKeyLegendNamesOnlyBoundKeys(t *testing.T) {
	bound := listModeBoundKeys(t)
	for _, e := range footerLegend {
		toks, ok := helpKeyTokenToBoundKeys[e.unicodeKey]
		if !ok {
			t.Fatalf("footer legend entry %q has no helpKeyTokenToBoundKeys translation -- add one so this check can see it", e.unicodeKey)
		}
		for _, key := range toks {
			if !bound[key] {
				t.Errorf("footer legend names key %q (glyph %q) that the list-mode switch does not bind", key, e.unicodeKey)
			}
		}
	}
}

// TestHelpOverlayWidthStaysWithinFrameBudgetAt80Columns is requirement
// 38/39's frame-budget component for the *width* dimension: at deck's
// documented 80-column minimum, no rendered help-view line exceeds 80
// visible columns (framedDialog's dialogWidth clamp plus wrapText already
// guarantee this; this pins it for the exact box `?` renders, not a
// generic dialog).
//
// The *height* dimension is a separate, unresolved gap, not this test's
// claim: at 80x24 the rendered help view is 273 lines (measured against
// HEAD of this commit), because helpView/framedDialog never clips or
// paginates -- unlike the transient messages requirement 30 already pins
// (which must always fit alongside the live session list), the help
// overlay and the `E` event log (eventLogView, same framedDialog path)
// are the only thing on screen while open and were apparently never
// height-bounded even when requirement 39 was written 22 Aug. Recorded
// as a new task rather than silently claimed here.
func TestHelpOverlayWidthStaysWithinFrameBudgetAt80Columns(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.width, model.height = 80, 24
	model.help = true
	view := model.View()
	for i, line := range strings.Split(view, "\n") {
		if w := stringWidth(line); w > 80 {
			t.Errorf("help view line %d is %d columns wide, exceeding the 80-column budget: %q", i, w, line)
		}
	}
}
