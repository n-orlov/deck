package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// wholeLegendPrefixes is every line footerLegendWithin may return: the
// legend's first n whole entries joined by the separator, with or without
// the elision marker after them, or nothing at all.
func wholeLegendPrefixes(m Model) map[string]bool {
	segments, _ := m.footerLegendSegments()
	sep := m.glyph(" · ", " - ")
	marker := m.glyph("…", "...")
	ok := map[string]bool{"": true}
	for n := 1; n <= len(segments); n++ {
		joined := stripANSI(strings.Join(segments[:n], sep))
		ok[joined] = true
		ok[joined+sep+marker] = true
	}
	return ok
}

// TestFooterNoteLeavesOnlyWholeLegendEntries is R282's narrow-budget case in
// list mode: whatever width the note leaves for the legend, from nothing up
// past the whole legend, the footer line fits the frame, holds the note whole,
// and its legend part is a run of whole entries -- never a clipped key or hint.
func TestFooterNoteLeavesOnlyWholeLegendEntries(t *testing.T) {
	for _, ascii := range []bool{false, true} {
		for _, bookmarks := range []bool{false, true} {
			m := footerFitModel(100)
			m.settings = config.Settings{ASCII: ascii, Bookmarks: bookmarks}
			allowed := wholeLegendPrefixes(m)
			for budget := 0; budget <= min(m.footerLegendWidth()+3, 100-footerNoteGap-1); budget++ {
				note := strings.Repeat("n", 100-budget-footerNoteGap)
				m.setAttachError(note)
				line := stripANSI(m.footerLineContent())
				if got := stringWidth(line); got != 100 {
					t.Fatalf("ascii=%v bookmarks=%v budget=%d: footer is %d columns, want 100: %q", ascii, bookmarks, budget, got, line)
				}
				if !strings.HasSuffix(line, note) {
					t.Fatalf("ascii=%v bookmarks=%v budget=%d: note was clipped: %q", ascii, bookmarks, budget, line)
				}
				legend := strings.TrimRight(strings.TrimSuffix(line, note), " ")
				if !allowed[legend] {
					t.Fatalf("ascii=%v bookmarks=%v budget=%d: legend part %q is not a run of whole entries", ascii, bookmarks, budget, legend)
				}
			}
		}
	}
}

// TestFooterNoteWithReasonDropsLegendEntriesWhole is the same rule with a
// status reason on the left: the reason is elided as prose, the legend still
// loses whole entries only.
func TestFooterNoteWithReasonDropsLegendEntriesWhole(t *testing.T) {
	m := footerFitModel(100)
	m.sessions[0].Status = "waiting"
	m.sessions[0].StatusReason = "needs your answer to the question"
	if m.selectedRowReason() == "" {
		t.Fatal("test setup: the selected row has no reason")
	}
	allowed := wholeLegendPrefixes(m)
	for left := 0; left <= 80; left++ {
		note := strings.Repeat("n", 100-left-footerNoteGap)
		m.setAttachError(note)
		line := stripANSI(m.footerLineContent())
		if got := stringWidth(line); got != 100 || !strings.HasSuffix(line, note) {
			t.Fatalf("left=%d: footer %q is %d columns or lost the note", left, line, got)
		}
		body := strings.TrimRight(strings.TrimSuffix(line, note), " ")
		if i := strings.Index(body, "    "); i >= 0 {
			if legend := body[i+4:]; !allowed[legend] {
				t.Fatalf("left=%d: legend part %q is not a run of whole entries", left, legend)
			}
		}
	}
}

// TestInteractiveFooterWithNoteNeverShowsAPartialQuitHint is R282's narrow
// remaining-budget case in interactive mode: with the note keeping its text,
// whatever is left holds the Ctrl+Q hint whole or holds nothing of it. The
// scrolled-back and mouse variants are the same rule over more segments.
func TestInteractiveFooterWithNoteNeverShowsAPartialQuitHint(t *testing.T) {
	const quit = "Ctrl+Q leave interactive mode"
	for _, mouse := range []bool{false, true} {
		for _, offset := range []int{0, 7} {
			m := New(nil, config.Settings{Mouse: mouse}, "")
			m.width, m.height = 100, 30
			m.sessions = []store.Session{{ID: "s1", Name: "alpha", Slug: "alpha", Agent: "shell", Status: "running", CWD: "/work"}}
			m.selected = rowCursor(0)
			m.interactive = true
			m.setInteractiveScrollOffset(offset)
			for leftover := 0; leftover <= len(quit)+3; leftover++ {
				note := strings.Repeat("n", 100-leftover-footerNoteGap)
				m.setBookmarkNote("")
				m.settings.Bookmarks = true
				m.setBookmarkNote(note)
				line := stripANSI(m.footerLineContent())
				if got := stringWidth(line); got != 100 || !strings.HasSuffix(line, note) {
					t.Fatalf("mouse=%v offset=%d leftover=%d: footer %q is %d columns or lost the note", mouse, offset, leftover, line, got)
				}
				left := strings.TrimRight(strings.TrimSuffix(line, note), " ")
				if strings.Contains(left, "Ctrl") && !strings.Contains(left, quit) {
					t.Fatalf("mouse=%v offset=%d leftover=%d: partial quit hint in %q", mouse, offset, leftover, left)
				}
				if leftover-footerNoteGap >= len(quit) && !strings.Contains(left, quit) {
					t.Fatalf("mouse=%v offset=%d leftover=%d: the hint fits but was dropped: %q", mouse, offset, leftover, left)
				}
				if leftover-footerNoteGap < len(quit) && left != "" && !strings.Contains(left, quit) {
					t.Fatalf("mouse=%v offset=%d leftover=%d: clipped text %q left of the note", mouse, offset, leftover, left)
				}
			}
		}
	}
}
