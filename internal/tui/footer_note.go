package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// footerNoteSource names one of the twelve finite transient-note sources
// that share the footer's single right-aligned slot (R280). The slot adds no
// row to the frame: computeLayout reserves nothing for any of them, so a
// note appearing or clearing never moves the panels (R281).
//
// The order is also the tie-break when two sources have the same stamp (a
// model built directly rather than through Update): earlier wins.
type footerNoteSource int

const (
	noteAttachError footerNoteSource = iota
	noteResume
	noteSelectionCopy
	noteFieldCopy
	noteKillUndo
	noteKillBatchUndo
	noteDeleteUndo
	noteDeleteBatchUndo
	noteArchiveUndo
	noteArchiveUndoneRebuild
	noteTeardownHook
	notePush
	footerNoteSourceCount
)

// footerNoteReaders reads each source's text, indexed by footerNoteSource.
// A reader reads the source's own state and changes none of it, so showing
// the newest note never touches an older note's state (an undo window stays
// open behind a newer note and is back on screen when the newer one clears).
var footerNoteReaders = [footerNoteSourceCount]func(Model) string{
	noteAttachError:   func(m Model) string { return m.attachError },
	noteResume:        func(m Model) string { return m.resumeNote },
	noteSelectionCopy: func(m Model) string { return m.selectionCopyNote },
	noteFieldCopy:     func(m Model) string { return m.fieldCopyNote },
	noteKillUndo: func(m Model) string {
		if m.undoSessionID == "" {
			return ""
		}
		return fmt.Sprintf("Killed %q \u2014 press u to undo", m.undoSessionName)
	},
	noteKillBatchUndo: func(m Model) string {
		if len(m.batchUndoSessionIDs) == 0 {
			return ""
		}
		return fmt.Sprintf("Killed %d sessions \u2014 press u to undo", len(m.batchUndoSessionIDs))
	},
	// The delete and archive notes never name the session: dd and A hide
	// the row, and their scenarios assert the name is gone from the whole
	// screen.
	noteDeleteUndo: func(m Model) string {
		if m.deleteUndoSessionID == "" {
			return ""
		}
		return "Deleted \u2014 press u to undo"
	},
	noteDeleteBatchUndo: func(m Model) string {
		if len(m.batchDeleteUndoSessionIDs) == 0 {
			return ""
		}
		return fmt.Sprintf("Deleted %d sessions \u2014 press u to undo", len(m.batchDeleteUndoSessionIDs))
	},
	// States the kill when there was one, and says `u` unarchives: the
	// agent stays stopped.
	noteArchiveUndo: func(m Model) string {
		switch {
		case m.archiveUndoSessionID == "":
			return ""
		case m.archiveUndoKilled:
			return "Killed and archived \u2014 press u to unarchive (agent stays stopped)"
		}
		return "Archived \u2014 press u to unarchive"
	},
	noteArchiveUndoneRebuild: func(m Model) string {
		if !m.archiveUndoneRebuildNote {
			return ""
		}
		return "Back stopped \u2014 post_destroy already ran; the next r rebuilds what it released"
	},
	noteTeardownHook: func(m Model) string { return m.teardownHookNote },
	// The rename push note belongs to the row it describes: it shows only
	// while that row is the selected one, and never in interactive mode.
	notePush: func(m Model) string {
		if m.interactive || !m.pushNoteShown() {
			return ""
		}
		return m.pushNote
	},
}

// footerNoteRenewal reads each source's renewal token, indexed by
// footerNoteSource: a value that changes every time the source is set, even
// to the same text it already showed. The undo and expiring sources use the
// generation their own expiry tick already carries; the four plain-text
// sources use the count their setters keep. A note renewed with identical
// text (a second kill of the same session, the same copy again) is therefore
// the newest note, exactly as a note with new text is.
var footerNoteRenewal = [footerNoteSourceCount]func(Model) int{
	noteAttachError:          func(m Model) int { return m.footerNoteSets[noteAttachError] },
	noteResume:               func(m Model) int { return m.footerNoteSets[noteResume] },
	noteSelectionCopy:        func(m Model) int { return m.footerNoteSets[noteSelectionCopy] },
	noteFieldCopy:            func(m Model) int { return m.footerNoteSets[noteFieldCopy] },
	noteKillUndo:             func(m Model) int { return m.undoGeneration },
	noteKillBatchUndo:        func(m Model) int { return m.batchUndoGeneration },
	noteDeleteUndo:           func(m Model) int { return m.deleteUndoGeneration },
	noteDeleteBatchUndo:      func(m Model) int { return m.batchDeleteUndoGeneration },
	noteArchiveUndo:          func(m Model) int { return m.archiveUndoGeneration },
	noteArchiveUndoneRebuild: func(m Model) int { return m.archiveUndoneRebuildGeneration },
	noteTeardownHook:         func(m Model) int { return m.teardownHookNoteGeneration },
	notePush:                 func(m Model) int { return m.footerNoteSets[notePush] },
}

// setAttachError, setResumeNote, setSelectionCopyNote and setFieldCopyNote
// are the only writers of the four plain-text sources: each also counts a
// non-empty assignment, so stampFooterNotes sees a renewal with unchanged
// text. Clearing a note is not a renewal and leaves the count alone.
func (m *Model) setAttachError(text string) {
	m.attachError = text
	if text != "" {
		m.footerNoteSets[noteAttachError]++
	}
}

func (m *Model) setResumeNote(text string) {
	m.resumeNote = text
	if text != "" {
		m.footerNoteSets[noteResume]++
	}
}

func (m *Model) setSelectionCopyNote(text string) {
	m.selectionCopyNote = text
	if text != "" {
		m.footerNoteSets[noteSelectionCopy]++
	}
}

func (m *Model) setFieldCopyNote(text string) {
	m.fieldCopyNote = text
	if text != "" {
		m.footerNoteSets[noteFieldCopy]++
	}
}

// footerNoteText is the text one source currently wants shown, or "" when it
// is not set.
func (m Model) footerNoteText(src footerNoteSource) string {
	return footerNoteReaders[src](m)
}

// footerNote is the one note the footer slot shows: the newest set source,
// collapsed to a single line (a multi-line message never wraps).
func (m Model) footerNote() string {
	best, bestStamp := footerNoteSource(-1), uint64(0)
	for src := footerNoteSource(0); src < footerNoteSourceCount; src++ {
		if m.footerNoteText(src) == "" {
			continue
		}
		if stamp := m.footerNoteStamp[src]; best < 0 || stamp > bestStamp {
			best, bestStamp = src, stamp
		}
	}
	if best < 0 {
		return ""
	}
	return strings.Join(strings.Fields(m.footerNoteText(best)), " ")
}

// stampFooterNotes records, on the model Update is about to return, which
// sources this message set, changed or renewed (footerNoteRenewal moved,
// even with the text unchanged), so footerNote can tell the newest from an
// older one. A source that is no longer set loses its stamp.
func stampFooterNotes(prev Model, next tea.Model) tea.Model {
	model, ok := next.(Model)
	if !ok {
		return next
	}
	for src := footerNoteSource(0); src < footerNoteSourceCount; src++ {
		text := model.footerNoteText(src)
		switch {
		case text == "":
			model.footerNoteStamp[src] = 0
		case text != prev.footerNoteText(src), footerNoteRenewal[src](model) != footerNoteRenewal[src](prev):
			model.footerNoteClock++
			model.footerNoteStamp[src] = model.footerNoteClock
		}
	}
	return model
}

// footerNoteGap is the minimum blank run between the footer's own content
// and the right-aligned note.
const footerNoteGap = 2

// footerWithNote composes the footer line with the note in its right-aligned
// slot. The note keeps its text up to the full width; whatever remains goes
// to the footer's own content, which gives up whole trailing entries first.
func (m Model) footerWithNote(note string) string {
	width, _ := m.frameSize()
	note = m.elideToWidth(note, width)
	noteWidth := stringWidth(note)
	budget := width - noteWidth - footerNoteGap
	left := ""
	if budget > 0 {
		left = m.footerLeftWithin(budget)
		if stringWidth(left) > budget {
			left = truncateToWidth(left, budget)
		}
	}
	pad := width - stringWidth(left) - noteWidth
	if pad < 0 {
		pad = 0
	}
	return left + strings.Repeat(" ", pad) + note
}

// footerLeftWithin is the footer's own content (the list's reason and key
// legend, or interactive mode's line) fitted to budget columns.
func (m Model) footerLeftWithin(budget int) string {
	if m.interactive {
		return m.interactiveFooterLineWithin(budget)
	}
	return m.footerReasonLineWithin(budget)
}
