package agent

import (
	"regexp"
	"strings"
)

// claudeDialogMarkers are texts Claude draws when one of its own dialogs (a
// tool permission prompt, the folder-trust screen, a picker) has taken the
// keyboard from the composer. While any is on the screen, typed keys answer the
// dialog instead of renaming the session.
var claudeDialogMarkers = []string{
	"Do you want to proceed?",
	"Do you want to make this edit",
	"Do you want to create",
	"Do you want to allow",
	"Do you trust the files in this folder",
	"Do you trust the contents of this directory",
	"Enter to confirm · Esc to cancel",
	"Esc to cancel",
}

// claudeOptionCursor matches a dialog's selection cursor on a numbered option
// ("❯ 1. Yes"); the composer's own prompt glyph is never followed by a number.
var claudeOptionCursor = regexp.MustCompile(`^[│|\s]*[❯>]\s*\d+\.\s`)

// DialogFocused reports whether a Claude permission, trust or picker dialog is
// on the screen, so a rename is never typed into it (R267).
func (c Claude) DialogFocused(screen string) bool {
	for _, marker := range claudeDialogMarkers {
		if strings.Contains(screen, marker) {
			return true
		}
	}
	for _, line := range strings.Split(screen, "\n") {
		if claudeOptionCursor.MatchString(line) {
			return true
		}
	}
	status, _ := c.Probe(screen)
	return status == "waiting"
}

// PromptReady reports whether Claude's composer is on the screen, empty, with
// no dialog over it. The composer is the last line that starts with the prompt
// glyph (after any box border), because earlier such lines are the echoed
// turns of the transcript. A typed, unsent draft (anything after the glyph but
// Claude's own `Try "..."` placeholder), a startup screen with no composer yet
// and a dialog are all not ready, so the Ctrl-U that opens the push plan can
// never discard a user's draft.
func (c Claude) PromptReady(screen string) bool {
	if c.DialogFocused(screen) {
		return false
	}
	lines := strings.Split(screen, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.Trim(lines[i], " \t\r│|")
		for _, glyph := range []string{"❯", ">"} {
			if rest, ok := strings.CutPrefix(line, glyph); ok {
				rest = strings.TrimSpace(strings.Trim(rest, "│| \t"))
				return rest == "" || strings.HasPrefix(rest, `Try "`)
			}
		}
	}
	return false
}
