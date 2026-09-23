// Package sidebarcursor declares the value internal/tui's Model.selected
// holds: a sidebar cursor that is either a session ROW or a group HEADER
// (task 012/D.1, R137 -- headers became navigable stops in their own
// right, so "the selected session" stopped being a safe assumption at
// every call site that reads the cursor).
//
// It lives in its own package for one reason, and the reason is the
// compiler: Go field visibility is per PACKAGE, so a cursor type declared
// in package tui still exposes `m.selected.index` to every one of the ~490
// cursor reads across internal/tui, and nothing stops a future edit (or a
// careless merge) from reading a session index off a header cursor and
// silently addressing m.sessions[0]. Declared HERE, with every field
// unexported, the only ways into a Cursor's payload are the two-result
// accessors below -- SessionIndex and GroupID -- each of which resolves
// the cursor's kind before it hands anything back. A bare integer read of
// the payload from package tui is a compile error ("c.index undefined"),
// not a convention: that is the whole point of the package boundary, and
// internal/tui/cursor_kind_guard_test.go pins it.
package sidebarcursor

// kind distinguishes the two kinds of visual stop a Cursor can rest on. It
// is unexported and has no accessor of its own on purpose: callers ask
// IsRow/IsHeader, or -- better -- ask for the payload they actually want
// and honour the ok result.
type kind int8

const (
	kindRow kind = iota
	kindHeader
)

// Cursor is one sidebar cursor: deliberately NOT a bare int, and
// deliberately without a single exported field. SessionIndex is the only
// way to read "which session the cursor names", and it reports ok=false on
// a header cursor, so a header position can never be silently misread as
// session index 0. Symmetrically GroupID is the only way to read "which
// header", ok=false on a row cursor.
//
// The zero value is Row(0): kind defaults to kindRow and index to 0, so a
// freshly zero-valued tui.Model (every test fixture that never sets
// Model.selected explicitly) keeps behaving exactly as it did when
// Model.selected was a bare `int` defaulting to 0.
//
// Cursor stays comparable (all fields are comparable scalars), so `==`,
// `!=` and use as a map key work exactly as they did on the bare int --
// several hundred existing internal/tui assertions depend on that.
type Cursor struct {
	kind    kind
	index   int   // valid m.sessions index, only when kind == kindRow
	groupID int64 // valid durable group id, only when kind == kindHeader
}

// Row builds a cursor resting on the session at m.sessions[index].
func Row(index int) Cursor {
	return Cursor{kind: kindRow, index: index}
}

// Header builds a cursor resting on one group's header, keyed by its
// durable group id -- 0 is the implicit default group's sentinel, exactly
// like internal/tui's sessionGroupID sentinel (no real groups.id row can
// ever be 0, since SQLite rowids start at 1).
func Header(groupID int64) Cursor {
	return Cursor{kind: kindHeader, groupID: groupID}
}

// IsHeader and IsRow report which kind of stop c is.
func (c Cursor) IsHeader() bool { return c.kind == kindHeader }
func (c Cursor) IsRow() bool    { return c.kind == kindRow }

// SessionIndex resolves c's m.sessions index. ok is false when c is a
// header cursor: there is deliberately no other way -- from any package --
// to read a session index out of a Cursor, so every caller that needs one
// is forced to decide, at the call site, what a header cursor means for it
// (usually "there is no selected session right now").
func (c Cursor) SessionIndex() (int, bool) {
	if c.kind != kindRow {
		return 0, false
	}
	return c.index, true
}

// GroupID resolves c's header group id. ok is false when c is a row
// cursor; the group a ROW's session belongs to is a question about the
// session list, not about the cursor, and internal/tui's cursorGroupID
// answers it.
func (c Cursor) GroupID() (int64, bool) {
	if c.kind != kindHeader {
		return 0, false
	}
	return c.groupID, true
}
