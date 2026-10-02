package tui

import "github.com/n-orlov/deck/internal/theme"

// textFieldEditKeysLine is the one help line every text field names its
// editor keys with (SPEC §11.11, R186): the readline caret and delete keys
// and alt+w, the whole-field copy. It is shared by every dialog that holds a
// shared-editor field so no two of them can state the keys differently, and
// it is shown only while a text field has the focus (a selection field
// cycles with Left/Right/Space instead).
const textFieldEditKeysLine = "Editing: Left/Right, Home/End, alt+b/alt+f word, ctrl+w, alt+backspace, ctrl+u/ctrl+k delete, alt+w copies the field"

// styledTextFieldEditKeys is the editing line through the Hint token, one
// colour span per already-wrapped physical line (wrap first, colour after, as
// every styled dialog body does).
func (m Model) styledTextFieldEditKeys(wrap func(string) []string) []string {
	var out []string
	for _, l := range wrap(textFieldEditKeysLine) {
		out = append(out, m.colorToken(theme.Hint, l))
	}
	return out
}

// createFooterLine is the create modal's closing legend. On a text field
// Left/Right move the caret (§11.11), so it does not claim they cycle; the
// editor keys and alt+w ride the same legend because the modal at 80x24
// has no row to spare: one more row scrolls the Name field out of view when a
// validation error brings the submit line up.
func (m Model) createFooterLine() string {
	if createFieldIsText(m.createField) {
		return "\u2191/\u2193 field · Enter submits · Esc cancels · Editing: Left/Right Home/End alt+b/alt+f ctrl+w alt+backspace ctrl+u/ctrl+k · alt+w copies the field"
	}
	return "\u2191/\u2193 field · Left/Right/Space cycles · Enter submits · Esc cancels"
}
