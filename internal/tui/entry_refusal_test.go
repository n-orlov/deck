package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestSetEntryRefusalNeverLeavesItFooterOnly is task 007/R143's (GH #38)
// own structural guarantee, over every one of SPEC §11.9's seven reason
// kinds: setEntryRefusal is the ONLY place in the package that
// constructs an entryRefusalState (grep confirms: no other assignment to
// m.entryRefusal exists outside this file, entry_refusal.go and
// clearEntryRefusal), and it clears m.attachError in the same call -- so
// nothing that goes through it can ALSO leave the OLD footer-only
// attachError note behind. Table-driven over allEntryRefusalKinds so a
// kind added to that slice later without a matching case here still
// exercises this guarantee, rather than silently skipping it.
func TestSetEntryRefusalNeverLeavesItFooterOnly(t *testing.T) {
	for _, kind := range allEntryRefusalKinds {
		t.Run(string(kind), func(t *testing.T) {
			m := New(nil, config.Settings{}, "")
			m.attachError = "stale footer note from a previous refusal"
			m.setEntryRefusal("sess-1", kind, "reason for "+string(kind))

			if m.attachError != "" {
				t.Fatalf("kind %q: setEntryRefusal left attachError=%q, want it cleared -- an entry refusal must never be left as a footer-only note", kind, m.attachError)
			}
			if !m.entryRefusal.active {
				t.Fatalf("kind %q: setEntryRefusal left entryRefusal.active=false", kind)
			}
			if m.entryRefusal.kind != kind {
				t.Fatalf("kind %q: entryRefusal.kind = %q, want %q", kind, m.entryRefusal.kind, kind)
			}
			if m.entryRefusal.sessionID != "sess-1" {
				t.Fatalf("kind %q: entryRefusal.sessionID = %q, want %q", kind, m.entryRefusal.sessionID, "sess-1")
			}
		})
	}
}

// wantEntryRefusalWayOut is SPEC §11.9's line 3 for each of the seven
// kinds: the kind's own way out, then the `keys go to the list` tail.
var wantEntryRefusalWayOut = map[entryRefusalKind]string{
	entryRefusalAttachedElsewhere: "F forces entry, a attaches instead \u2014 keys go to the list",
	entryRefusalOwnedElsewhere:    "F forces entry, a attaches instead \u2014 keys go to the list",
	entryRefusalStopped:           "r starts it, then \u21b5 enters \u2014 keys go to the list",
	entryRefusalRowFloor:          "a attaches instead \u2014 keys go to the list",
	entryRefusalNoLivePane:        "R restarts it, then \u21b5 enters \u2014 keys go to the list",
	entryRefusalShrank:            "grow it and \u21b5, or a attaches \u2014 keys go to the list",
	entryRefusalOther:             "\u21b5 retries, a attaches instead \u2014 keys go to the list",
}

// TestEveryEntryRefusalKindDrawsItsOwnBannerNotTheFooter is the second
// half of the same table: for every one of the seven kinds, the banner
// entryRefusalBannerLines/overlayEntryRefusalBanner draw -- exactly what
// previewBodyLines calls on every render while the refusal is active --
// actually differs from the plain background it is drawn over (SPEC
// §11.9's "passive capture stays visible around it" only makes sense if
// the banner rows themselves are new content), and every drawn banner row
// carries the headline and reason -- never merely a copy of the footer's
// old wording -- so "verified via rerun against every drawing" reruns
// previewBodyLines a second time and requires byte-identical output,
// catching any of the four kinds (stopped/row floor/no live pane/shrank)
// that might accidentally special-case its own draw into something
// nondeterministic.
func TestEveryEntryRefusalKindDrawsItsOwnBannerNotTheFooter(t *testing.T) {
	for _, kind := range allEntryRefusalKinds {
		t.Run(string(kind), func(t *testing.T) {
			m := New(nil, config.Settings{Color: true}, "")
			m.width, m.height = 240, 24
			m.sessions = []store.Session{{ID: "sess-1", Name: "alpha", Slug: "alpha", Status: "running"}}
			m.selected = rowCursor(0)
			m.setEntryRefusal("sess-1", kind, "reason for "+string(kind))

			cw, ch := m.previewContentSize()
			background, backgroundOwners, _ := m.previewBodyLinesBeforeRefusal(cw, ch)
			drawn, drawnOwners, _ := m.previewBodyLines(cw, ch)
			drawnAgain, drawnAgainOwners, _ := m.previewBodyLines(cw, ch)

			if len(drawn) != len(drawnAgain) {
				t.Fatalf("kind %q: previewBodyLines row count is not deterministic: %d then %d", kind, len(drawn), len(drawnAgain))
			}
			for i := range drawn {
				if drawn[i] != drawnAgain[i] || drawnOwners[i] != drawnAgainOwners[i] {
					t.Fatalf("kind %q: previewBodyLines row %d is not deterministic across two renders", kind, i)
				}
			}

			if len(drawn) != len(background) {
				t.Fatalf("kind %q: banner overlay changed the row count: background=%d drawn=%d", kind, len(background), len(drawn))
			}
			changed := false
			for i := range drawn {
				if drawn[i] != background[i] || drawnOwners[i] != backgroundOwners[i] {
					changed = true
				}
				if drawnOwners[i] != previewLineDeckOwned {
					t.Fatalf("kind %q: drawn row %d owner = %v, want previewLineDeckOwned -- every banner row is deck's own composed content", kind, i, drawnOwners[i])
				}
			}
			if !changed {
				t.Fatalf("kind %q: previewBodyLines drew nothing over the background -- an active entry refusal must draw its own lines, not fall back to the footer", kind)
			}

			joined := strings.Join(drawn, "\n")
			if !strings.Contains(joined, "NOT ATTACHED: alpha") {
				t.Fatalf("kind %q: drawn preview does not contain the banner headline:\n%s", kind, joined)
			}
			if !strings.Contains(joined, "reason for "+string(kind)) {
				t.Fatalf("kind %q: drawn preview does not contain the banner's own reason text:\n%s", kind, joined)
			}
			wayOut := entryRefusalWayOut(kind)
			if !strings.HasSuffix(wayOut, "keys go to the list") {
				t.Fatalf("kind %q: way-out text %q does not end with the required tail", kind, wayOut)
			}
			// SPEC §11.9 line 3 is "the way out for that reason" AND the
			// tail -- the tail alone is not a way out. Every kind must
			// name its own guidance ahead of it, exactly as specified.
			want, ok := wantEntryRefusalWayOut[kind]
			if !ok {
				t.Fatalf("kind %q: no expected way-out in wantEntryRefusalWayOut -- a new kind needs its own line 3", kind)
			}
			if wayOut != want {
				t.Fatalf("kind %q: way-out = %q, want %q", kind, wayOut, want)
			}
			guidance := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(wayOut, "keys go to the list"), " \u2014 "))
			if guidance == "" || guidance == wayOut {
				t.Fatalf("kind %q: way-out %q carries no kind-specific guidance ahead of the tail", kind, wayOut)
			}
			if !strings.Contains(joined, wayOut) {
				t.Fatalf("kind %q: drawn preview does not contain the banner's own way-out line %q:\n%s", kind, wayOut, joined)
			}

			// SPEC §11.9: F and a for contention, a for the floor, and
			// F never offered for a refusal that is not contention.
			switch kind {
			case entryRefusalAttachedElsewhere, entryRefusalOwnedElsewhere:
				if !strings.Contains(wayOut, "F forces entry") || !strings.Contains(wayOut, "a attaches") {
					t.Fatalf("kind %q: contention way-out %q does not name both F and a", kind, wayOut)
				}
			default:
				if strings.Contains(wayOut, "F ") || strings.Contains(wayOut, " F") {
					t.Fatalf("kind %q: non-contention way-out %q names F", kind, wayOut)
				}
			}
			if kind == entryRefusalRowFloor && !strings.HasPrefix(wayOut, "a attaches") {
				t.Fatalf("kind %q: floor way-out %q does not offer a", kind, wayOut)
			}

			// ascii mode draws the same guidance in plain ASCII.
			am := m
			am.settings = config.Settings{ASCII: true}
			asciiLines, _, _ := am.previewBodyLines(cw, ch)
			asciiJoined := strings.Join(asciiLines, "\n")
			asciiWay := am.entryRefusalWayOutText(kind)
			if !strings.Contains(asciiJoined, asciiWay) || !strings.HasSuffix(asciiWay, "keys go to the list") {
				t.Fatalf("kind %q: ascii drawing lacks way-out %q:\n%s", kind, asciiWay, asciiJoined)
			}
			for _, r := range asciiWay {
				if r > 0x7e {
					t.Fatalf("kind %q: ascii way-out %q carries non-ASCII rune %q", kind, asciiWay, r)
				}
			}
		})
	}
}

// entryRefusalGoldenPath is the checked-in, byte-exact rendering of SPEC
// §11.9's refusal banner (task 007/R143, GH #38) for one fixed
// attached-elsewhere refusal, in colour, NO_COLOR and ascii modes, at
// three widths (a wide pane, the narrowest width the banner still draws
// legibly into, and one column below that -- entryRefusalBannerLines'
// own contentWidth<5 bail).
//
// Regenerate only after a deliberate, reviewed change to the banner's own
// rendering:
//
//	ci/run.sh env UPDATE_GOLDEN=1 go test -count=1 -run TestEntryRefusalBannerGoldenIsByteIdentical ./internal/tui/
const entryRefusalGoldenPath = "testdata/golden/entry_refusal_banner.golden"

func renderEntryRefusalGolden(t *testing.T) string {
	t.Helper()
	refusal := entryRefusalState{
		active:    true,
		sessionID: "sess-1",
		kind:      entryRefusalAttachedElsewhere,
		reason:    "another client is attached to this session",
	}
	modes := []struct {
		name     string
		settings config.Settings
	}{
		{"colour", config.Settings{Color: true}},
		{"NO_COLOR", config.Settings{Color: false}},
		{"ascii", config.Settings{Color: true, ASCII: true}},
	}
	widths := []int{60, 5, 4}

	var b strings.Builder
	for _, mode := range modes {
		m := New(nil, mode.settings, "")
		for _, w := range widths {
			fmt.Fprintf(&b, "== %s / width=%d ==\n", mode.name, w)
			lines := m.entryRefusalBannerLines(w, "alpha", refusal)
			if len(lines) == 0 {
				fmt.Fprintf(&b, "(nil)\n")
				continue
			}
			for _, l := range lines {
				fmt.Fprintf(&b, "%q\n", l)
			}
		}
	}
	// Every one of the seven kinds' own banner (its own line 3 way out)
	// at a 60-column pane, in every render mode, so each kind's way out
	// is pinned byte-for-byte -- not only the contention kind above.
	for _, mode := range modes {
		m := New(nil, mode.settings, "")
		for _, kind := range allEntryRefusalKinds {
			fmt.Fprintf(&b, "== %s / kind=%s / width=60 ==\n", mode.name, kind)
			r := entryRefusalState{active: true, sessionID: "sess-1", kind: kind, reason: "reason for " + string(kind)}
			for _, l := range m.entryRefusalBannerLines(60, "alpha", r) {
				fmt.Fprintf(&b, "%q\n", l)
			}
		}
	}
	return b.String()
}

// TestEntryRefusalBannerGoldenIsByteIdentical proves task 007/R143's
// success criterion 3: the banner renders boxed, centred, spanning the
// full content width, in all three colour modes, byte-identical to the
// checked-in golden.
func TestEntryRefusalBannerGoldenIsByteIdentical(t *testing.T) {
	got := renderEntryRefusalGolden(t)
	if got != renderEntryRefusalGolden(t) {
		t.Fatalf("entry-refusal banner rendering is not deterministic across two renders")
	}
	if !strings.Contains(got, "NOT ATTACHED: alpha") {
		t.Fatalf("fixture rendered no banner headline:\n%s", got)
	}
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(entryRefusalGoldenPath), 0o700); err != nil {
			t.Fatalf("create golden directory: %v", err)
		}
		if err := os.WriteFile(entryRefusalGoldenPath, []byte(got), 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("wrote %s (%d bytes)", entryRefusalGoldenPath, len(got))
		return
	}
	want, err := os.ReadFile(entryRefusalGoldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", entryRefusalGoldenPath, err)
	}
	if got != string(want) {
		t.Fatalf("entry-refusal banner rendering drifted from golden %s\n--- got ---\n%s\n--- want ---\n%s", entryRefusalGoldenPath, got, string(want))
	}
}

// TestNoOtherSiteConstructsAnEntryRefusalState is the mechanical half of
// TestSetEntryRefusalNeverLeavesItFooterOnly's premise: grep every
// non-test .go file in this package for an assignment to m.entryRefusal
// (or a bare entryRefusalState{...} literal) OUTSIDE entry_refusal.go --
// the one file allowed to construct one, via setEntryRefusal/
// clearEntryRefusal. A call site that assembled its own literal instead
// of going through setEntryRefusal could set active/kind without also
// clearing attachError, silently reopening the footer-only path this
// task closes.
func TestNoOtherSiteConstructsAnEntryRefusalState(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir(internal/tui): %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if name == "entry_refusal.go" {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", name, err)
		}
		src := string(data)
		if strings.Contains(src, "m.entryRefusal = ") || strings.Contains(src, "entryRefusalState{") {
			t.Errorf("%s: constructs an entryRefusalState directly -- go through setEntryRefusal/clearEntryRefusal in entry_refusal.go instead", name)
		}
	}
}

// entryRefusalExpectedWayOut is SPEC §11.9's line three for each kind,
// written out literally per render mode (NOT derived from
// entryRefusalWayOut/entryRefusalWayOutText, so a regression in either
// function cannot silently move the expectation with it), together with
// the reason-specific escape keys that line must name: `F` and `a` for
// contention, `a` alone for the row floor (SPEC: "the row floor is
// deck's own limit and offers only `a`"), and the key each other kind's
// way out depends on (`r`/`R` then `↵`, `↵` then `a`). forbidden lists
// keys a kind must NOT offer.
type entryRefusalExpectedWayOut struct {
	unicode, ascii  string
	keys, asciiKeys []string
	forbidden       []string
}

var entryRefusalExpectedWayOuts = map[entryRefusalKind]entryRefusalExpectedWayOut{
	entryRefusalAttachedElsewhere: {
		unicode: "F forces entry, a attaches instead \u2014 keys go to the list",
		ascii:   "F forces entry, a attaches instead; keys go to the list",
		keys:    []string{"F", "a"}, asciiKeys: []string{"F", "a"},
	},
	entryRefusalOwnedElsewhere: {
		unicode: "F forces entry, a attaches instead \u2014 keys go to the list",
		ascii:   "F forces entry, a attaches instead; keys go to the list",
		keys:    []string{"F", "a"}, asciiKeys: []string{"F", "a"},
	},
	entryRefusalStopped: {
		unicode: "r starts it, then \u21b5 enters \u2014 keys go to the list",
		ascii:   "r starts it, then Enter enters; keys go to the list",
		keys:    []string{"r", "\u21b5"}, asciiKeys: []string{"r", "Enter"},
	},
	entryRefusalRowFloor: {
		unicode: "a attaches instead \u2014 keys go to the list",
		ascii:   "a attaches instead; keys go to the list",
		keys:    []string{"a"}, asciiKeys: []string{"a"},
		forbidden: []string{"F"},
	},
	entryRefusalNoLivePane: {
		unicode: "R restarts it, then \u21b5 enters \u2014 keys go to the list",
		ascii:   "R restarts it, then Enter enters; keys go to the list",
		keys:    []string{"R", "\u21b5"}, asciiKeys: []string{"R", "Enter"},
	},
	entryRefusalShrank: {
		unicode: "grow it and \u21b5, or a attaches \u2014 keys go to the list",
		ascii:   "grow it and Enter, or a attaches; keys go to the list",
		keys:    []string{"\u21b5", "a"}, asciiKeys: []string{"Enter", "a"},
	},
	entryRefusalOther: {
		unicode: "\u21b5 retries, a attaches instead \u2014 keys go to the list",
		ascii:   "Enter retries, a attaches instead; keys go to the list",
		keys:    []string{"\u21b5", "a"}, asciiKeys: []string{"Enter", "a"},
	},
}

// renderedRefusalBanner locates SPEC §11.9's boxed banner inside a
// rendered preview (ANSI stripped), asserting on the way that it is
// boxed (top edge, side-walled text rows, bottom edge, all exactly cw
// wide) and centred (each text row's left/right padding differ by at
// most one column), and returns its text rows, trimmed, in order.
func renderedRefusalBanner(t *testing.T, label string, lines []string, cw int, ascii bool) []string {
	t.Helper()
	tl, tr, bl, br, h, v := "\u256d", "\u256e", "\u2570", "\u256f", "\u2500", "\u2502"
	if ascii {
		tl, tr, bl, br, h, v = "+", "+", "+", "+", "-", "|"
	}
	top := tl + strings.Repeat(h, cw-2) + tr
	bottom := bl + strings.Repeat(h, cw-2) + br
	plain := make([]string, len(lines))
	for i, l := range lines {
		plain[i] = stripANSI(l)
	}
	start := -1
	for i, l := range plain {
		if l == top {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("%s: no banner top edge %q (exactly %d wide) in rendered preview:\n%s", label, top, cw, strings.Join(plain, "\n"))
	}
	var rows []string
	for i := start + 1; ; i++ {
		if i >= len(plain) {
			t.Fatalf("%s: banner has no bottom edge %q inside the preview -- it overflows:\n%s", label, bottom, strings.Join(plain, "\n"))
		}
		l := plain[i]
		if l == bottom {
			break
		}
		if !strings.HasPrefix(l, v+" ") || !strings.HasSuffix(l, " "+v) || stringWidth(l) != cw {
			t.Fatalf("%s: banner row %d %q is not a %d-wide side-walled box row", label, i, l, cw)
		}
		inner := strings.TrimSuffix(strings.TrimPrefix(l, v+" "), " "+v)
		text := strings.Trim(inner, " ")
		if text == "" {
			t.Fatalf("%s: banner row %d is blank", label, i)
		}
		left := len(inner) - len(strings.TrimLeft(inner, " "))
		right := len(inner) - len(strings.TrimRight(inner, " "))
		if d := left - right; d < -1 || d > 1 {
			t.Fatalf("%s: banner row %q is not centred (left pad %d, right pad %d)", label, l, left, right)
		}
		rows = append(rows, text)
	}
	return rows
}

// TestEntryRefusalWayOutSurvivesDefaultGeometry is a render REGRESSION
// test for a review finding (cure-01-01): at deck's supported 80x24
// floor the preview panel's own content width (37 columns, well below
// the up-to-56-column way-out text several kinds carry) used to let
// entryRefusalBannerLines' single centerTruncate row ellipsis away the
// mandatory "keys go to the list" tail entirely -- the unrendered
// source string (entryRefusalWayOut/entryRefusalWayOutText) always
// carried the tail; only the ACTUAL rendered preview output lost it, so
// this asserts against previewBodyLines' own return (and the full
// View() frame), not the source string
// TestEveryEntryRefusalKindDrawsItsOwnBannerNotTheFooter already
// covers. Table-driven over every one of SPEC §11.9's seven kinds, in
// all three render modes (colour, NO_COLOR, ascii) and at several
// supported geometries including 80x24. For each it checks, in the
// rendered banner: line one the headline, line two the reason, the
// remaining rows -- re-joined across the word wrap -- exactly the
// kind's literal way out (entryRefusalExpectedWayOuts), each of the
// kind's escape keys present as a whole word (so `a` cannot be
// satisfied by "alpha" or "attaches"), the keyboard destination on one
// row, and the box boxed, centred and inside the preview.
func TestEntryRefusalWayOutSurvivesDefaultGeometry(t *testing.T) {
	modes := []struct {
		name     string
		settings config.Settings
	}{
		{"colour", config.Settings{Color: true}},
		{"NO_COLOR", config.Settings{}},
		{"ascii", config.Settings{ASCII: true}},
	}
	geometries := []struct{ w, h int }{{80, 24}, {100, 30}, {160, 48}}
	for _, g := range geometries {
		for _, mode := range modes {
			for _, kind := range allEntryRefusalKinds {
				label := fmt.Sprintf("%dx%d/%s/%s", g.w, g.h, mode.name, kind)
				t.Run(label, func(t *testing.T) {
					exp, ok := entryRefusalExpectedWayOuts[kind]
					if !ok {
						t.Fatalf("%s: no expected way out recorded for kind %q", label, kind)
					}
					wantWayOut, keys := exp.unicode, exp.keys
					if mode.settings.ASCII {
						wantWayOut, keys = exp.ascii, exp.asciiKeys
					}

					m := New(nil, mode.settings, "")
					m.width, m.height = g.w, g.h
					m.sessions = []store.Session{{ID: "sess-1", Name: "alpha", Slug: "alpha", Status: "running"}}
					m.selected = rowCursor(0)
					reason := "reason for " + string(kind)
					m.setEntryRefusal("sess-1", kind, reason)

					cw, ch := m.previewContentSize()
					lines, _, _ := m.previewBodyLines(cw, ch)

					if len(lines) != ch {
						t.Fatalf("%s: preview row count = %d, want exactly contentHeight %d -- the banner must not overflow the preview", label, len(lines), ch)
					}
					for i, l := range lines {
						if w := stringWidth(l); w > cw {
							t.Fatalf("%s: preview row %d has display width %d > contentWidth %d:\n%q", label, i, w, cw, l)
						}
					}

					rows := renderedRefusalBanner(t, label, lines, cw, mode.settings.ASCII)
					if len(rows) < 3 {
						t.Fatalf("%s: banner has %d text rows, want headline, reason and way out: %q", label, len(rows), rows)
					}
					if rows[0] != "NOT ATTACHED: alpha" {
						t.Fatalf("%s: banner line one = %q, want the headline %q", label, rows[0], "NOT ATTACHED: alpha")
					}
					if rows[1] != reason {
						t.Fatalf("%s: banner line two = %q, want the reason %q", label, rows[1], reason)
					}
					wayOutRows := rows[2:]
					if got := strings.Join(wayOutRows, " "); got != wantWayOut {
						t.Fatalf("%s: rendered way out (rows %q) reads %q, want %q", label, wayOutRows, got, wantWayOut)
					}
					tailOnOneRow := false
					for _, r := range wayOutRows {
						if strings.Contains(r, "keys go to the list") {
							tailOnOneRow = true
						}
					}
					if !tailOnOneRow {
						t.Fatalf("%s: the keyboard destination %q is not on any single rendered row: %q", label, "keys go to the list", wayOutRows)
					}
					words := map[string]bool{}
					for _, r := range wayOutRows {
						for _, w := range strings.Fields(r) {
							words[strings.TrimRight(w, ",;")] = true
						}
					}
					for _, k := range keys {
						if !words[k] {
							t.Fatalf("%s: rendered way out %q does not name the escape key %q as a word", label, wayOutRows, k)
						}
					}
					for _, k := range exp.forbidden {
						if words[k] {
							t.Fatalf("%s: rendered way out %q offers %q, which SPEC §11.9 does not allow for this kind", label, wayOutRows, k)
						}
					}
					if mode.settings.ASCII {
						for _, l := range lines {
							for _, r := range stripANSI(l) {
								if r > 0x7e {
									t.Fatalf("%s: ascii-mode preview row %q carries non-ASCII %q", label, stripANSI(l), r)
								}
							}
						}
					}

					// The same rows reach the whole frame View() draws,
					// not only the preview body in isolation.
					frame := stripANSI(m.View())
					for _, r := range rows {
						if !strings.Contains(frame, r) {
							t.Fatalf("%s: banner row %q is missing from the full rendered frame:\n%s", label, r, frame)
						}
					}
				})
			}
		}
	}
}
