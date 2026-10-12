package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// footerNoteSource names one of the finite transient-note sources that
// share the footer's single right-aligned slot (R280): the twelve R280
// names, plus the attached view's bookmark message (R276). The slot adds no
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
	noteBookmarkAttached
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
	// The bookmark message rides the slot only in interactive mode, where
	// the list's whole-footer bookmark line is not drawn (R276).
	noteBookmarkAttached: func(m Model) string {
		if !m.interactive || !m.settings.Bookmarks {
			return ""
		}
		return m.bookmarkNote
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
	noteBookmarkAttached:     func(m Model) int { return m.footerNoteSets[noteBookmarkAttached] },
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

// setBookmarkNote is the only writer of bookmarkNote in non-test code; the
// count it keeps is the attached view's renewal token (noteBookmarkAttached).
func (m *Model) setBookmarkNote(text string) {
	m.bookmarkNote = text
	if text != "" {
		m.footerNoteSets[noteBookmarkAttached]++
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
func stampFooterNotes(prev Model, next tea.Model, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	model, ok := next.(Model)
	if !ok {
		return next, cmd
	}
	var ticks []tea.Cmd
	for src := footerNoteSource(0); src < footerNoteSourceCount; src++ {
		text := model.footerNoteText(src)
		switch {
		case text == "":
			model.footerNoteStamp[src] = 0
		case text != prev.footerNoteText(src), footerNoteRenewal[src](model) != footerNoteRenewal[src](prev):
			model.footerNoteClock++
			model.footerNoteStamp[src] = model.footerNoteClock
			if tick := model.footerNoteExpiryTick(src); tick != nil {
				ticks = append(ticks, tick)
			}
		}
	}
	if len(ticks) == 0 {
		return model, cmd
	}
	return model, tea.Batch(append(ticks, cmd)...)
}

// footerNoteExpires marks the sources the [ui] note_timeout tick ends (R283):
// the four plain-text sources and the archive-undone rebuild note. The five
// undo-style sources keep DECK_UNDO_MS / the delete grace, and the teardown
// hook note keeps DECK_UNDO_MS; the rename push note and the attached
// bookmark message are unchanged and keep their own lifetimes.
var footerNoteExpires = [footerNoteSourceCount]bool{
	noteAttachError:          true,
	noteResume:               true,
	noteSelectionCopy:        true,
	noteFieldCopy:            true,
	noteArchiveUndoneRebuild: true,
}

// footerNoteExpired fires [ui] note_timeout after a note was set or renewed.
// It carries the renewal token the note had then, so it is generation-guarded:
// a tick from an older note, or from a note since replaced by a newer one
// (even one with identical text), finds a different token and clears nothing.
type footerNoteExpired struct {
	src footerNoteSource
	gen int
}

// footerNoteExpiryTick is the tick that ends src's note as it stands now, or
// nil for a source the note_timeout does not govern. A Settings that carries
// no NoteTimeout (the zero value: a Model built without config.Load, which
// always resolves one of at least a second) schedules no expiry at all.
func (m Model) footerNoteExpiryTick(src footerNoteSource) tea.Cmd {
	timeout := m.settings.NoteTimeout
	if !footerNoteExpires[src] || timeout <= 0 {
		return nil
	}
	msg := footerNoteExpired{src: src, gen: footerNoteRenewal[src](m)}
	return tea.Tick(timeout, func(time.Time) tea.Msg { return msg })
}

// onFooterNoteExpired clears the note when it is still the one the tick was
// started for. Clearing is the same write the next event performs, so an
// older note behind it (an undo window) is untouched.
func (m Model) onFooterNoteExpired(msg footerNoteExpired) (tea.Model, tea.Cmd) {
	if !footerNoteExpires[msg.src] || footerNoteRenewal[msg.src](m) != msg.gen {
		return m, nil
	}
	switch msg.src {
	case noteAttachError:
		m.setAttachError("")
	case noteResume:
		m.setResumeNote("")
	case noteSelectionCopy:
		m.setSelectionCopyNote("")
	case noteFieldCopy:
		m.setFieldCopyNote("")
	case noteArchiveUndoneRebuild:
		m.archiveUndoneRebuildNote = false
	}
	return m, nil
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
