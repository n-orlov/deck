package tui

import (
	"context"
	"fmt"
)

// fieldCopyKey is the key that copies the focused text field's whole text
// (R180). It is the caller's, never the line editor's: lineedit.Update leaves
// it unhandled, so every text field's handler checks it BEFORE offering the
// key to the editor.
const fieldCopyKey = "alt+w"

// isFieldCopyKey reports whether msg is alt+w.
func isFieldCopyKey(msg interface{ String() string }) bool {
	return msg.String() == fieldCopyKey
}

// copyFieldText is R180's copy: the focused field's whole text goes through the
// same two halves drag-to-copy uses -- tmux.Client.SetSelectionBuffer (the
// load-bearing half, deck's own "deck-selection" buffer) and
// writeOSCClipboardBestEffort -- and the user sees the same confirmation
// (selectionCopyConfirmation) a drag gives, in fieldCopyNote, which every
// dialog, the filter line and the settings footer draw. A masked secret copies
// only while revealed: while masked nothing is copied (neither half runs) and
// the note says so. An empty field copies nothing and says so too, rather than
// replacing the buffer with an empty string.
func (m Model) copyFieldText(text string, masked bool) Model {
	switch {
	case masked:
		m.fieldCopyNote = "Nothing copied: the value is masked (reveal it first to copy it)"
		return m
	case text == "":
		m.fieldCopyNote = "Nothing copied: the field is empty"
		return m
	}
	if err := m.tmuxClient.SetSelectionBuffer(context.Background(), text); err != nil {
		m.fieldCopyNote = fmt.Sprintf("Cannot copy field: %v", err)
		return m
	}
	writeOSCClipboardBestEffort(text)
	m.fieldCopyNote = selectionCopyConfirmation(text)
	return m
}

// fieldCopyNoteBodyLines is the note as body lines a dialog frame appends
// below its own content (none when there is no note).
func (m Model) fieldCopyNoteBodyLines() []string {
	if m.fieldCopyNote == "" {
		return nil
	}
	return m.wrapDialogLines("\n" + m.fieldCopyNote)
}
