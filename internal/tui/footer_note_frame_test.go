package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// footerNoteRows is one row per R280 source: how to set that note on a model
// and the fragment of its text that must reach the footer.
var footerNoteRows = []struct {
	name string
	set  func(m *Model)
	show string
}{
	{"selectionCopyNote", func(m *Model) { m.selectionCopyNote = "Copied 12 characters" }, "Copied 12"},
	{"fieldCopyNote", func(m *Model) { m.fieldCopyNote = "Copied the field" }, "Copied the field"},
	{"attachError", func(m *Model) { m.attachError = "Cannot attach: no such pane" }, "Cannot attach"},
	{"resumeNote", func(m *Model) { m.resumeNote = "Resumed the session" }, "Resumed the"},
	{"kill-undo", func(m *Model) { m.undoSessionID, m.undoSessionName = "s1", "alpha" }, "Killed \"alpha\""},
	{"batch-kill-undo", func(m *Model) { m.batchUndoSessionIDs = []string{"s1", "s2"} }, "Killed 2 sessions"},
	{"delete-undo", func(m *Model) { m.deleteUndoSessionID = "s1" }, "Deleted — press u"},
	{"batch-delete-undo", func(m *Model) { m.batchDeleteUndoSessionIDs = []string{"s1", "s2"} }, "Deleted 2 sessions"},
	{"archive-undo", func(m *Model) { m.archiveUndoSessionID = "s1" }, "Archived — press u"},
	{"archive-undone-rebuild", func(m *Model) { m.archiveUndoneRebuildNote = true }, "Back stopped"},
	{"teardownHookNote", func(m *Model) { m.teardownHookNote = "post_destroy failed: exit 3" }, "post_destroy failed"},
}

// footerNoteFrameModel is a model with a live preview far taller than any
// panel, so the preview is cropped from the bottom and a panel that loses a
// row shows a different first line.
func footerNoteFrameModel(width, height int, interactive bool) Model {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = width, height
	m.sessions = []store.Session{{ID: "s1", Name: "alpha", Slug: "alpha", Agent: "shell", Status: "running", CWD: "/work"}}
	m.selected = rowCursor(0)
	var pane strings.Builder
	const paneRows = 200
	for i := 1; i <= paneRows; i++ {
		fmt.Fprintf(&pane, "pane line %03d\r\n", i)
	}
	m.previewLive = true
	m.previewSessionID = "s1"
	m.previewBytes = []byte(pane.String())
	m.previewPaneWidth, m.previewPaneHeight = 60, paneRows
	m.interactive = interactive
	return m
}

// firstPreviewLine is the first content row of the preview panel: the line
// below its top border, from the preview's left edge on.
func firstPreviewLine(t *testing.T, m Model, view string) string {
	t.Helper()
	layout := m.computeLayout()
	lines := strings.Split(view, "\n")
	row := layout.Preview.Y + 1
	if row >= len(lines) {
		t.Fatalf("preview row %d is outside the %d-line view", row, len(lines))
	}
	runes := []rune(lines[row])
	if layout.Preview.X > len(runes) {
		t.Fatalf("preview column %d is outside the line %q", layout.Preview.X, lines[row])
	}
	return string(runes[layout.Preview.X:])
}

// TestFooterNoteFrameStable is R281: for every source in R280 the number of
// lines mainView returns and the first line of the preview content are the
// same with the note set and cleared, at the 80x24 minimum and a wide size,
// in the list and in interactive mode, and the note itself is on the footer
// line, the last line of the view.
func TestFooterNoteFrameStable(t *testing.T) {
	sizes := []struct{ width, height int }{{80, 24}, {200, 50}}
	for _, row := range footerNoteRows {
		for _, size := range sizes {
			for _, interactive := range []bool{false, true} {
				mode := "list"
				if interactive {
					mode = "interactive"
				}
				t.Run(fmt.Sprintf("%s/%dx%d/%s", row.name, size.width, size.height, mode), func(t *testing.T) {
					cleared := footerNoteFrameModel(size.width, size.height, interactive)
					set := footerNoteFrameModel(size.width, size.height, interactive)
					row.set(&set)

					clearedView, setView := cleared.mainView(), set.mainView()
					clearedLines, setLines := strings.Split(clearedView, "\n"), strings.Split(setView, "\n")
					if len(setLines) != len(clearedLines) {
						t.Errorf("line count with the note set = %d, cleared = %d: the note took its own row", len(setLines), len(clearedLines))
					}
					if got, want := firstPreviewLine(t, set, setView), firstPreviewLine(t, cleared, clearedView); got != want {
						t.Errorf("first preview line with the note set = %q, cleared = %q: the preview lost a row", got, want)
					}
					if !strings.Contains(setLines[len(setLines)-1], row.show) {
						t.Errorf("footer line = %q, want the note %q in it", setLines[len(setLines)-1], row.show)
					}
					if strings.Contains(clearedLines[len(clearedLines)-1], row.show) {
						t.Errorf("footer line = %q with the note cleared, want no %q", clearedLines[len(clearedLines)-1], row.show)
					}
				})
			}
		}
	}
}

// TestFooterNoteSlotIsRightAligned pins that the slot sits at the right edge
// of the footer line, in both the list and interactive mode.
func TestFooterNoteSlotIsRightAligned(t *testing.T) {
	for _, interactive := range []bool{false, true} {
		m := footerNoteFrameModel(120, 30, interactive)
		m.attachError = "Cannot attach: no such pane"
		lines := strings.Split(m.mainView(), "\n")
		footer := lines[len(lines)-1]
		if !strings.HasSuffix(strings.TrimRight(footer, " "), m.attachError) {
			t.Errorf("interactive=%v: footer %q does not end with the note", interactive, footer)
		}
		if got := stringWidth(footer); got != 120 {
			t.Errorf("interactive=%v: footer is %d columns, want 120", interactive, got)
		}
	}
}

// TestNewerFooterNoteLeavesOlderUndoWindowIntact sets a kill-undo window, then
// a newer note through Update: the newer note is shown, the older note's
// state (the window `u` acts on) is untouched, and it is shown again once the
// newer note clears.
func TestNewerFooterNoteLeavesOlderUndoWindowIntact(t *testing.T) {
	m := footerNoteFrameModel(120, 30, false)
	m.undoSessionID, m.undoSessionName, m.undoGeneration = "s1", "alpha", 7
	got, _ := m.Update(key("A"))
	m = got.(Model)
	if !strings.Contains(m.attachError, "Archiving is unavailable") {
		t.Fatalf("test setup: A with no archiver left attachError = %q", m.attachError)
	}
	if note := m.footerNote(); !strings.Contains(note, "Archiving is unavailable") {
		t.Fatalf("footerNote() = %q, want the newer note", note)
	}
	if m.undoSessionID != "s1" || m.undoSessionName != "alpha" || m.undoGeneration != 7 {
		t.Fatalf("the newer note disturbed the undo window: id=%q name=%q generation=%d", m.undoSessionID, m.undoSessionName, m.undoGeneration)
	}
	m.attachError = ""
	if note := m.footerNote(); !strings.Contains(note, "press u to undo") {
		t.Fatalf("footerNote() = %q after the newer note cleared, want the undo note back", note)
	}
}
