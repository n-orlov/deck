package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// footerFitModel is a list-mode model with a short status reason on the
// selected row, so the footer's own content is the reason plus the key legend.
func footerFitModel(width int) Model {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = width, 30
	m.sessions = []store.Session{{ID: "s1", Name: "alpha", Slug: "alpha", Agent: "shell", Status: "running", CWD: "/work"}}
	m.selected = rowCursor(0)
	return m
}

// TestFooterNoteFitsOneLine is R282's table: a multi-line note collapses to
// one line and is never wrapped, the footer's own content gives up whole
// trailing entries when both do not fit, and the note keeps its text up to
// the full width, elided with an ellipsis only when it alone is longer than
// the line.
func TestFooterNoteFitsOneLine(t *testing.T) {
	long := strings.Repeat("a very long failure message ", 12)
	rows := []struct {
		name       string
		width      int
		note       string
		wantNote   string // the text that must be present, whole
		wantEnd    string // the line must end with this
		fullLegend bool   // the legend is shown in full beside the note
	}{
		{"wide terminal", 200, "Cannot attach:\nno such pane\n  (tmux said so)", "Cannot attach: no such pane (tmux said so)", "(tmux said so)", true},
		{"80 columns", 80, "Cannot attach:\nno such pane\n  (tmux said so, twice over)", "Cannot attach: no such pane (tmux said so, twice over)", "twice over)", false},
		{"note longer than the line", 80, long, strings.Fields(long)[0], "…", false},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			m := footerFitModel(row.width)
			m.setAttachError(row.note)
			line := stripANSI(m.footerLineContent())

			if strings.Contains(line, "\n") {
				t.Fatalf("the footer is more than one line: %q", line)
			}
			if got := stringWidth(line); got > row.width {
				t.Fatalf("footer is %d columns, want at most %d: %q", got, row.width, line)
			}
			if !strings.HasPrefix(row.wantNote, "a very") && !strings.Contains(line, row.wantNote) {
				t.Fatalf("footer %q lost part of the note %q", line, row.wantNote)
			}
			if !strings.HasSuffix(strings.TrimRight(line, " "), row.wantEnd) {
				t.Fatalf("footer %q does not end with %q", line, row.wantEnd)
			}

			legend := stripANSI(m.footerKeyLegend())
			switch {
			case row.fullLegend:
				if !strings.Contains(line, legend) {
					t.Fatalf("wide footer %q lost part of the legend %q", line, legend)
				}
			case row.wantEnd == "…":
				// The note alone fills the line: elided, not wrapped, and
				// the legend has no room left.
				if got := stringWidth(line); got != row.width {
					t.Fatalf("elided note is %d columns, want the full %d", got, row.width)
				}
				if !strings.HasPrefix(line, "a very long failure message") {
					t.Fatalf("elided note %q does not start with the note's own text", line)
				}
			default:
				// Both do not fit: the legend drops whole trailing entries,
				// marked by an ellipsis, and the note is intact.
				if strings.Contains(line, legend) {
					t.Fatalf("80-column footer %q still holds the whole legend, the test proves nothing", line)
				}
				budget := row.width - stringWidth(row.wantNote) - footerNoteGap
				if want := stripANSI(m.footerLegendWithin(budget)); !strings.HasPrefix(line, want) {
					t.Fatalf("footer %q does not start with the legend cut by whole entries %q", line, want)
				}
				if !strings.Contains(line, "…") {
					t.Fatalf("footer %q does not mark the legend entries it dropped", line)
				}
			}
		})
	}
}

// TestFooterNoteYieldsToBelowMinimumNotice pins that the below-minimum notice
// keeps the footer when a note is set.
func TestFooterNoteYieldsToBelowMinimumNotice(t *testing.T) {
	m := footerFitModel(60)
	m.height = 10
	if !m.computeLayout().BelowMinimum {
		t.Fatal("test setup: 60x10 must be below the minimum")
	}
	m.setAttachError("Cannot attach: no such pane")
	line := stripANSI(m.footerLineContent())
	if !strings.HasPrefix(line, "Terminal is below deck's supported minimum") {
		t.Fatalf("footer %q does not start with the below-minimum notice", line)
	}
	if strings.Contains(line, "Cannot attach") {
		t.Fatalf("footer %q shows the note over the notice", line)
	}
}

// TestPushNoteRendersInTheFooterSlot pins that the rename push note sits in
// the right-aligned slot with the legend beside it, adds no frame row, is
// not shown for another selection or in interactive mode, and is the newest
// note when set after another one.
func TestPushNoteRendersInTheFooterSlot(t *testing.T) {
	const note = "rename not pushed to claude: status is waiting"
	build := func() Model {
		m := footerFitModel(200)
		m.sessions = append(m.sessions, store.Session{ID: "s2", Name: "beta", Slug: "beta", Agent: "shell", Status: "running", CWD: "/work"})
		m.renamePusher = func(_ context.Context, id, name string) (store.Session, service.PushResult, error) {
			return store.Session{ID: id, Name: name, Agent: "shell", Status: "running"}, service.PushResult{Note: note}, nil
		}
		got, _ := m.Update(sessionRenamed{session: store.Session{ID: "s1", Name: "alpha"}, note: note})
		return got.(Model)
	}

	m := build()
	view := stripANSI(m.mainView())
	lines := strings.Split(view, "\n")
	footer := lines[len(lines)-1]
	if !strings.HasSuffix(strings.TrimRight(footer, " "), note) {
		t.Fatalf("footer %q does not end with the push note", footer)
	}
	if !strings.Contains(footer, stripANSI(m.footerKeyLegend())) {
		t.Fatalf("footer %q lost the legend beside the push note", footer)
	}
	plain := footerFitModel(200)
	if got, want := len(lines), len(strings.Split(plain.mainView(), "\n")); got != want {
		t.Fatalf("the push note changed the line count: %d, want %d", got, want)
	}

	m.selected = rowCursor(1)
	if strings.Contains(stripANSI(m.footerLineContent()), note) {
		t.Fatal("the push note is shown while another row is selected")
	}
	m = build()
	m.interactive = true
	if strings.Contains(stripANSI(m.footerLineContent()), note) {
		t.Fatal("the push note is shown in interactive mode")
	}

	m = build()
	m.setAttachError("Cannot attach: no such pane")
	m.footerNoteClock++
	m.footerNoteStamp[noteAttachError] = m.footerNoteClock
	if got := m.footerNote(); !strings.Contains(got, "Cannot attach") {
		t.Fatalf("footerNote() = %q, want the newer attach error", got)
	}
}

// TestLongAttachErrorStaysReachableInDetail pins the place SPEC §11 names for
// a note too long for the footer line: the `i` detail dialog shows the
// attach error's full text, wrapped across lines.
func TestLongAttachErrorStaysReachableInDetail(t *testing.T) {
	m := footerFitModel(80)
	m.height = 24
	m.setAttachError("Cannot attach: " + strings.Repeat("the pane is gone and tmux says so at length; ", 6) + "end of message")

	m.detail = true
	body := strings.Join(strings.Fields(stripANSI(m.detailBody())), " ")
	if want := strings.Join(strings.Fields(m.attachError), " "); !strings.Contains(body, want) {
		t.Fatalf("the detail dialog lost part of the attach error.\nwant: %q\nbody: %q", want, body)
	}
	m.detail = false
	if line := stripANSI(m.footerLineContent()); strings.Contains(line, "end of message") {
		t.Fatalf("the footer holds the whole long error: %q", line)
	}
}
