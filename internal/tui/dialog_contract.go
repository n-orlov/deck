package tui

import tea "github.com/charmbracelet/bubbletea"

// dialogFields describes the field navigation and value-cycling a §11.4
// dialog exposes to the shared contract below. A dialog with no navigable
// fields at all (detailView, helpView have nothing to submit or cycle) uses
// the zero value: Count 0 (or 1) leaves ↑/↓ a no-op — there is nothing
// else to move to — and a nil Cycle leaves left/right/space a no-op too.
type dialogFields struct {
	// Count is the number of fields ↑/↓ cycle through.
	Count int
	// Index is the dialog's own focused-field variable; ↑/↓ mutate *Index
	// in place. Only consulted when Count > 1.
	Index *int
	// Cycle changes the currently focused field's value by delta (-1 for
	// left, +1 for right or space). nil means nothing is cycled here.
	Cycle func(delta int)
	// SpaceTypesText, when non-nil, is asked before space is treated as
	// "change a selection" (SPEC §11.4): if it reports true for whatever
	// field is currently focused, space is NOT a contract key here — it
	// is ordinary typed input (createView's free-text fields, e.g. a name
	// containing a space) and falls through to the dialog's own switch
	// instead of being consumed as a cycle.
	SpaceTypesText func() bool
	// TextFocused, when non-nil, is asked before left, right and space are
	// treated as "change a selection" (SPEC §11.4, §11.11): if it reports
	// true the focused field is a text field, whose caret keys and typed
	// space belong to the shared line editor, so none of the three is a
	// contract key here and all fall through to the dialog's own handler.
	// Cycle then applies to selection fields only.
	TextFocused func() bool
}

// dialogContract is what a §11.4 dialog hands the shared implementation:
// its field navigation/cycling, plus the two actions the contract cannot
// perform generically because every dialog cancels/submits into different
// state.
type dialogContract struct {
	Fields dialogFields
	// Cancel implements esc. SPEC §11.4: "esc cancels and changes
	// nothing" — every dialog's Cancel must therefore only ever flip the
	// dialog closed and clear its own transient note, never touch the
	// session, the store or config.toml. nil means esc is not a contract
	// key here (no dialog in this package currently omits it).
	Cancel func()
	// Submit implements enter and returns the resulting tea.Cmd (if any).
	// nil means this dialog has nothing to submit (detailView, helpView):
	// enter is then simply not a contract key here and falls through
	// unhandled, exactly like any key the dialog does not bind.
	Submit func() tea.Cmd
}

// applyDialogContract is the ONE implementation of SPEC §11.4's shared
// dialog keys — esc cancels, enter submits, ↑/↓ move between fields,
// left/right/space change a selection — that createView's,
// profileSwitchView's and pinView's Update functions all defer to, and
// that the single esc case shared by detailView and helpView in
// Model.Update also calls, instead of five hand-written key switches that
// merely happen to agree with each other.
//
// tab/shift+tab are deliberately NOT bound here (SPEC §11.4: "tab is
// reserved for completion and never moves between fields"): on a path
// field tab is bash's own completion key (§11.7) and on every other field,
// and every dialog with no path field at all, it is simply unbound.
//
// It reports handled=false for any key outside that vocabulary, so a
// dialog's own ADDITIONAL load-bearing keys (createView's free-text
// typing and backspace; the yolo confirm keystroke steer 017 item 2
// removed used to be one such key) reach the dialog's own
// switch exactly as §11.4 allows: "a dialog may declare additional
// load-bearing keys of its own, but only if it states them inline where
// they apply". Nothing here invents an undeclared binding: a contract key
// this dialogContract does not wire up (nil Cancel/Submit, zero Fields)
// is left unhandled rather than silently doing something.
func applyDialogContract(msg tea.KeyMsg, c dialogContract) (cmd tea.Cmd, handled bool) {
	switch msg.String() {
	case "esc":
		if c.Cancel == nil {
			return nil, false
		}
		c.Cancel()
		return nil, true
	case "enter":
		if c.Submit == nil {
			return nil, false
		}
		return c.Submit(), true
	case "down":
		return nil, c.Fields.moveIndex(1)
	case "up":
		return nil, c.Fields.moveIndex(-1)
	case "left":
		return nil, c.Fields.cycleUnlessTyping(-1, false)
	case "right":
		return nil, c.Fields.cycleUnlessTyping(1, false)
	case " ":
		return nil, c.Fields.cycleUnlessTyping(1, true)
	}
	return nil, false
}

// moveIndex steps the focused field by delta (+1 down, -1 up), wrapping, and
// reports whether it handled the key: a dialog with fewer than two fields or
// no Index is left unhandled.
func (f dialogFields) moveIndex(delta int) bool {
	if f.Count <= 1 || f.Index == nil {
		return false
	}
	next := *f.Index + delta
	if delta < 0 {
		next += f.Count // keep up's (i-1+n)%n arithmetic exactly
	}
	*f.Index = next % f.Count
	return true
}

// cycleUnlessTyping cycles the focused selection field by delta and reports
// whether it handled the key. A focused text field keeps the key for typing
// (and, when space is true, so does a field whose space types text).
func (f dialogFields) cycleUnlessTyping(delta int, space bool) bool {
	if f.TextFocused != nil && f.TextFocused() {
		return false
	}
	if space && f.SpaceTypesText != nil && f.SpaceTypesText() {
		return false
	}
	if f.Cycle == nil {
		return false
	}
	f.Cycle(delta)
	return true
}
