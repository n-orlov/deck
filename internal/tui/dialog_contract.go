package tui

import tea "github.com/charmbracelet/bubbletea"

// dialogFields describes the field navigation and value-cycling a §11.4
// dialog exposes to the shared contract below. A dialog with no navigable
// fields at all (detailView, helpView have nothing to submit or cycle) uses
// the zero value: Count 0 (or 1) leaves tab/shift+tab a no-op — there is
// nothing else to move to — and a nil Cycle leaves left/right/space a
// no-op too.
type dialogFields struct {
	// Count is the number of fields tab/shift+tab cycle through.
	Count int
	// Index is the dialog's own focused-field variable; tab/shift+tab
	// mutate *Index in place. Only consulted when Count > 1.
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
// dialog keys — esc cancels, enter submits, tab/shift+tab move between
// fields, left/right/space change a selection — that createView's,
// profileSwitchView's and pinView's Update functions all defer to, and
// that the single esc case shared by detailView and helpView in
// Model.Update also calls, instead of five hand-written key switches that
// merely happen to agree with each other.
//
// It reports handled=false for any key outside that vocabulary, so a
// dialog's own ADDITIONAL load-bearing keys (§5's `y` yolo confirm,
// createView's free-text typing and backspace) reach the dialog's own
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
	case "tab":
		if c.Fields.Count <= 1 || c.Fields.Index == nil {
			return nil, false
		}
		*c.Fields.Index = (*c.Fields.Index + 1) % c.Fields.Count
		return nil, true
	case "shift+tab":
		if c.Fields.Count <= 1 || c.Fields.Index == nil {
			return nil, false
		}
		*c.Fields.Index = (*c.Fields.Index - 1 + c.Fields.Count) % c.Fields.Count
		return nil, true
	case "left":
		if c.Fields.Cycle == nil {
			return nil, false
		}
		c.Fields.Cycle(-1)
		return nil, true
	case "right":
		if c.Fields.Cycle == nil {
			return nil, false
		}
		c.Fields.Cycle(1)
		return nil, true
	case " ":
		if c.Fields.SpaceTypesText != nil && c.Fields.SpaceTypesText() {
			return nil, false
		}
		if c.Fields.Cycle == nil {
			return nil, false
		}
		c.Fields.Cycle(1)
		return nil, true
	}
	return nil, false
}
