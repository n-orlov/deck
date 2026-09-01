package tui

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file is task 015's footer<->bindings parity test: a cross-check,
// alongside help_keymap_parity_test.go's own help<->bindings one, that
// re-parses tui.go's own footerLegend slice literal (never a copy of it)
// and fails in either direction --
//
//   - a footer entry naming a key the list-mode switch does not bind
//     (direction a, reusing help_keymap_parity_test.go's own
//     listModeBoundKeys/helpKeyTokenToBoundKeys building blocks so this
//     is genuinely the same live extraction, not a second table that
//     could drift from the first);
//   - a footer entry that is SHOWN for a row its own eligibility
//     predicate rejects, or that carries no eligibility predicate at all
//     despite being a per-row action (direction b, SPEC §11.3: "nor does
//     it list a key that would refuse the current selection" and
//     "eligibility has exactly one definition per action, shared by the
//     footer and the key handler").
//
// It also cross-checks the fixed set itself against SPEC.md §11.3's own
// prose (never a copy of the key list into a Go table): removing `,`
// from footerLegend, or adding `P`/`p` back to it, both fail here.

// footerLegendEntryRe matches one footerLegend entry line in tui.go, in
// either of the two shapes every entry there is written in (footerLegend's
// own convention: a per-row action always routes through footerRowEligible,
// never a bespoke closure) --
//
//	{"<unicodeKey>", "<asciiKey>", "<hint>", nil},
//	{"<unicodeKey>", "<asciiKey>", "<hint>", func(m Model) bool { return footerRowEligible(m, <batch>, <predicate>) }},
//
// -- so a new entry written in a third shape does not silently vanish
// from the extraction: it fails to match at all, and parseFooterLegendSource's
// own entry-count floor below catches that.
var footerLegendEntryRe = regexp.MustCompile(`(?m)^\t\{"([^"]*)", "([^"]*)", "([^"]*)", (?:nil|func\(m Model\) bool \{ return footerRowEligible\(m, (true|false), (\w+)\) \})\},$`)

// parsedFooterEntry is one footerLegend entry as read from tui.go's own
// text: hasPredicate/batch/predicateName are only meaningful when
// hasPredicate is true (the entry's fourth field was a footerRowEligible
// closure, not a bare nil).
type parsedFooterEntry struct {
	unicodeKey, asciiKey, hint string
	hasPredicate               bool
	batch                      bool
	predicateName              string
}

// parseFooterLegendSource re-parses tui.go's `var footerLegend = []footerKeyHint{`
// slice literal directly, the same way listModeBoundKeys re-parses the
// list-mode key switch: every entry is read from the source text at test
// time, never copied into a fixture that could go stale independently of
// it.
func parseFooterLegendSource(t *testing.T) []parsedFooterEntry {
	t.Helper()
	data, err := os.ReadFile("tui.go")
	if err != nil {
		t.Fatalf("ReadFile(tui.go): %v", err)
	}
	src := string(data)

	start := strings.Index(src, "var footerLegend = []footerKeyHint{")
	if start < 0 {
		t.Fatalf("could not find `var footerLegend = []footerKeyHint{` in tui.go -- extraction is broken, not the source")
	}
	relEnd := strings.Index(src[start:], "\n}\n")
	if relEnd < 0 {
		t.Fatalf("could not find the end of footerLegend's slice literal (a `}` line on its own) in tui.go -- extraction is broken, not the source")
	}
	block := src[start : start+relEnd]

	var entries []parsedFooterEntry
	for _, line := range strings.Split(block, "\n") {
		m := footerLegendEntryRe.FindStringSubmatch(line)
		if m == nil {
			continue // the `var ... {` opener, a blank line, or a comment
		}
		e := parsedFooterEntry{unicodeKey: m[1], asciiKey: m[2], hint: m[3]}
		if m[5] != "" {
			e.hasPredicate = true
			e.batch = m[4] == "true"
			e.predicateName = m[5]
		}
		entries = append(entries, e)
	}
	if len(entries) < 10 {
		t.Fatalf("only parsed %d footerLegend entries from tui.go, expected 14+ -- extraction is broken, not the source", len(entries))
	}
	return entries
}

// TestFooterEntriesNameOnlyBoundKeysViaSourceParse is direction (a): every
// entry parsed out of footerLegend's own source text names a key that
// tui.go's list-mode switch actually binds. It reuses
// help_keymap_parity_test.go's own listModeBoundKeys (re-parses the
// switch) and helpKeyTokenToBoundKeys (the fixed display-glyph -> raw-key
// vocabulary the help overlay's own parity test already trusts), so a
// glyph with no translation there fails loudly rather than passing by
// omission.
func TestFooterEntriesNameOnlyBoundKeysViaSourceParse(t *testing.T) {
	bound := listModeBoundKeys(t)
	for _, e := range parseFooterLegendSource(t) {
		toks, ok := helpKeyTokenToBoundKeys[e.unicodeKey]
		if !ok {
			t.Fatalf("footer entry %q (glyph %q, parsed from tui.go's own footerLegend) has no helpKeyTokenToBoundKeys translation -- add one so this parity check can see it", e.hint, e.unicodeKey)
		}
		for _, key := range toks {
			if !bound[key] {
				t.Errorf("footer entry %q (glyph %q, parsed from tui.go's own footerLegend) names key %q that the list-mode switch does not bind", e.hint, e.unicodeKey, key)
			}
		}
	}
}

// footerPredicateByName maps a footerLegend entry's own predicate-function
// name -- parsed textually out of tui.go above, never hand-copied -- to
// the actual function it names, so TestFooterEntryEligibilityMatchesRealPredicate
// evaluates the very function the entry's own source names rather than a
// second implementation of its logic that could drift from it.
var footerPredicateByName = map[string]func(store.Session) bool{
	"canAcknowledge": canAcknowledge,
	"canKill":        canKill,
	"canResume":      canResume,
	"canRestart":     canRestart,
	"canDelete":      canDelete,
	"canReachPane":   canReachPane,
	"canShowDetail":  canShowDetail,
	"canArchive":     canArchive,
	"canUnarchive":   canUnarchive,
}

// footerGlobalKeysWithNoEligibility is the exact, closed set of footer
// entries allowed to carry a bare `nil` fourth field: global commands
// that never act on a row at all (navigation, `n`, `?`, `q`) plus `,`
// (settings, task 014: "it carries no eligible predicate at all"). Any
// OTHER entry parsed with no predicate is a per-row action that skipped
// the gating SPEC §11.3 requires ("Nor does it list a key that would
// refuse the current selection") -- which is exactly how `P`/`p` would
// silently reappear unconditionally if someone copied a global entry's
// shape instead of a gated one's.
var footerGlobalKeysWithNoEligibility = map[string]bool{
	"↑/↓": true,
	"n":   true,
	",":   true,
	"?":   true,
	"q":   true,
}

// newFooterParityRow builds a one-session (or, with an empty ID, a
// zero-session) Model for TestFooterEntryEligibilityMatchesRealPredicate's
// per-row eligibility check, mirroring footer_legend_test.go's own newModel
// helper.
func newFooterParityRow(session store.Session) Model {
	m := New(nil, config.Settings{}, "")
	if session.ID != "" {
		m.sessions = []store.Session{session}
	}
	m.selected = 0
	return m
}

// TestFooterEntryEligibilityMatchesRealPredicate is direction (b): every
// footerLegend entry that is a per-row action carries a real eligibility
// predicate (never a bare nil, see footerGlobalKeysWithNoEligibility
// above), and for a representative row of every state the current
// predicates distinguish, the entry is SHOWN in the rendered footer if
// and only if that same predicate -- looked up by the name the entry's
// own source names, and run through the real footerRowEligible with the
// same batch flag the source names -- accepts that row.
//
// Both of this task's demonstrated regressions fail here: `,` removed
// from footerLegend makes TestFooterFixedSetMatchesSpecAndExcludesRareKeys
// fail (below), and `P` added back with a bare `nil` fourth field (the
// simplest, and historically actual, way it would reappear) fails the
// first loop below, because `P` is not in footerGlobalKeysWithNoEligibility.
func TestFooterEntryEligibilityMatchesRealPredicate(t *testing.T) {
	entries := parseFooterLegendSource(t)

	for _, e := range entries {
		if e.hasPredicate {
			if _, ok := footerPredicateByName[e.predicateName]; !ok {
				t.Fatalf("footer entry %q names predicate %q that this test does not recognise -- add it to footerPredicateByName so the eligibility check below can evaluate it", e.hint, e.predicateName)
			}
			continue
		}
		if !footerGlobalKeysWithNoEligibility[e.unicodeKey] {
			t.Errorf("footer entry %q (glyph %q) carries no eligibility predicate and is not one of the fixed global keys allowed to have none -- a per-row action must gate on eligibility (SPEC \u00a711.3: \"nor does it list a key that would refuse the current selection\")", e.hint, e.unicodeKey)
		}
	}

	rows := []struct {
		name string
		m    Model
	}{
		{"empty list", newFooterParityRow(store.Session{})},
		{"live/running", newFooterParityRow(store.Session{ID: "s1", Status: "running"})},
		{"stopped", newFooterParityRow(store.Session{ID: "s1", Status: "stopped"})},
		{"stopped+archived", newFooterParityRow(store.Session{ID: "s1", Status: "stopped", ArchivedAt: 100})},
		{"waiting", newFooterParityRow(store.Session{ID: "s1", Status: "waiting"})},
	}
	// A batch row: the selected row (s1, live) would accept x/dd itself,
	// but the marked set (s2, stopped) would not -- x/dd's own batch
	// handling asks the marked set, not the selection, so this row is the
	// one place batch=true's own wiring (not just batch=false's) is
	// exercised end to end.
	batchRow := newFooterParityRow(store.Session{ID: "s1", Status: "running"})
	batchRow.sessions = append(batchRow.sessions, store.Session{ID: "s2", Status: "stopped"})
	batchRow.marked = map[string]bool{"s2": true}
	rows = append(rows, struct {
		name string
		m    Model
	}{"marked batch: only a non-killable row marked", batchRow})

	for _, row := range rows {
		legend := row.m.footerKeyLegend()
		for _, e := range entries {
			if !e.hasPredicate {
				continue
			}
			predicate := footerPredicateByName[e.predicateName]
			want := footerRowEligible(row.m, e.batch, predicate)
			matchText := strings.TrimSpace(e.unicodeKey + " " + e.hint)
			got := strings.Contains(legend, matchText)
			if got != want {
				t.Errorf("%s row: footer entry %q eligibility mismatch: predicate %q (batch=%v) says eligible=%v, rendered legend says shown=%v: legend=%q", row.name, matchText, e.predicateName, e.batch, want, got, legend)
			}
		}
	}
}

var backtickTokenRe = regexp.MustCompile("`([^`]+)`")
var whitespaceRunRe = regexp.MustCompile(`\s+`)

// specText reads SPEC.md, the authoritative and protected (read-only for
// this job) spec, collapsing every run of whitespace -- including the
// newlines §11.3's own prose wraps across -- to a single space so the
// sentence-anchored searches below don't have to know where a line break
// happens to fall.
func specText(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../SPEC.md")
	if err != nil {
		t.Fatalf("ReadFile(../../SPEC.md): %v", err)
	}
	return whitespaceRunRe.ReplaceAllString(string(data), " ")
}

// specFooterFixedSetGlyphs re-parses SPEC.md \u00a711.3's own sentence naming
// the footer's fixed set -- "It carries navigation, `\u21b5`, `a`, `Y`, `n`,
// `x`, `r`, `R`, `dd`, the eligible one of `A`/`U`, `,`, `i`, `?` and `q`."
// -- into the ordered list of glyphs it names (plus "\u2191/\u2193" for the plain-
// English "navigation"), by pulling out every backtick-quoted token
// between that sentence's own start and the next sentence's start. This
// is never copied into a fixed Go list: SPEC.md is read fresh every run.
func specFooterFixedSetGlyphs(t *testing.T) []string {
	t.Helper()
	text := specText(t)
	start := strings.Index(text, "fixed set is curated for the keys worth a whole line of the frame.** It carries")
	if start < 0 {
		t.Fatalf("could not find SPEC.md \u00a711.3's footer fixed-set sentence -- extraction is broken, not the source")
	}
	relEnd := strings.Index(text[start:], "Rarely-used per-row actions")
	if relEnd < 0 {
		t.Fatalf("could not find the end of SPEC.md \u00a711.3's footer fixed-set sentence -- extraction is broken, not the source")
	}
	sentence := text[start : start+relEnd]
	glyphs := []string{"\u2191/\u2193"}
	for _, m := range backtickTokenRe.FindAllStringSubmatch(sentence, -1) {
		glyphs = append(glyphs, m[1])
	}
	if len(glyphs) < 10 {
		t.Fatalf("only found %d glyphs in SPEC.md \u00a711.3's footer fixed-set sentence, expected 14+ -- extraction is broken, not the source", len(glyphs))
	}
	return glyphs
}

// specFooterExcludedGlyphs re-parses SPEC.md \u00a711.3's own naming of the two
// keys that must stay OUT of the footer -- "the permission switcher `P`,
// pin `p`" -- rather than copying them into a fixed Go list either.
func specFooterExcludedGlyphs(t *testing.T) []string {
	t.Helper()
	text := specText(t)
	m := regexp.MustCompile("permission switcher `([^`]+)`, pin `([^`]+)`").FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("could not find SPEC.md \u00a711.3's \"permission switcher `P`, pin `p`\" exclusion wording -- extraction is broken, not the source")
	}
	return []string{m[1], m[2]}
}

// TestFooterFixedSetMatchesSpecAndExcludesRareKeys cross-checks
// footerLegend's own parsed glyph set against SPEC.md \u00a711.3's prose
// directly, in both directions: every glyph the fixed-set sentence
// requires is present, and neither glyph the exclusion sentence names is.
// This is what fails if `,` is deleted from footerLegend (missing from
// the required side) or if `P` is added back to it (present on the
// excluded side) -- independently of TestFooterEntryEligibilityMatchesRealPredicate
// above, which also catches the `P` case a different way.
func TestFooterFixedSetMatchesSpecAndExcludesRareKeys(t *testing.T) {
	entries := parseFooterLegendSource(t)
	present := map[string]bool{}
	for _, e := range entries {
		present[e.unicodeKey] = true
	}

	for _, g := range specFooterFixedSetGlyphs(t) {
		if !present[g] {
			t.Errorf("SPEC.md \u00a711.3's footer fixed set requires %q but tui.go's footerLegend does not have it", g)
		}
	}
	for _, g := range specFooterExcludedGlyphs(t) {
		if present[g] {
			t.Errorf("SPEC.md \u00a711.3 keeps %q out of the footer (a rarely-used per-row action) but tui.go's footerLegend has it", g)
		}
	}
}
