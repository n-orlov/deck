package agent

import "strings"

// piDialogHints are the key-hint lines Pi draws at the foot of a dialog that
// has taken the editor's place (pi 0.84 dist/modes/interactive/components:
// extension-selector.js, which ui.select and ui.confirm both render through,
// and extension-input.js for ui.input). With the default keybindings they read
// "↑↓ navigate  enter select  escape/ctrl+c cancel" and
// "enter submit  escape/ctrl+c cancel".
var piDialogHints = []string{"↑↓ navigate", "enter submit"}

// DialogFocused reports whether a Pi selector, confirm or input dialog is on
// the screen: a line carrying one of piDialogHints together with its "cancel"
// hint. Keys typed then would answer the dialog, not rename the session, so the
// keystroke fallback is refused while one is up (SPEC §8.3). The control-file
// push types nothing and never asks.
func (Pi) DialogFocused(screen string) bool {
	for _, line := range strings.Split(screen, "\n") {
		if !strings.Contains(line, "cancel") {
			continue
		}
		for _, hint := range piDialogHints {
			if strings.Contains(line, hint) {
				return true
			}
		}
	}
	return false
}
