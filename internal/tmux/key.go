package tmux

import (
	"context"
	"fmt"
)

// namedKeyAllowlist is PRD phase3c item 35 (II-35): every name interactive
// mode may ever ask tmux to translate, fixed at compile time and checked
// BEFORE tmux is ever invoked (SendNamedKey below). Nothing outside this
// set reaches send-keys as a bare (non-literal) argument -- an unlisted
// name is exactly PRD item 35's own named hazard
// (literal_send_test.go's TestSendKeysUnknownKeyNameIsDeliveredAsLiteralTextWithExitZero
// shows what happens if it were let through: ten bytes typed into the
// agent as literal text, exit 0, no error).
//
// Every entry here was confirmed against a real tmux 3.5a server, not
// copied from a manual page: each name was sent (raw, no deck code) to a
// pane running `cat` -- which echoes whatever bytes it receives right
// back, unedited -- and the resulting capture-pane content inspected
// byte for byte. A name that tmux does not recognize is typed back
// character by character (the Frobnicate/KPDivide/WheelUpPane shape);
// every name below instead comes back as tmux's own translated escape
// or control byte, which is what "sent by tmux name" (PRD item 34)
// means: deck never hand-builds the escape sequence itself, it names the
// key and lets tmux's own key-string translation produce the bytes, the
// same translation an attached client's raw input goes through key_test.go's
// TestHomeAndEndEscapesMatchARealAttachedClientTypingTheSameKeys proves
// this for Home/End specifically, which is the PRD item 34 finding this
// task also records in docs/reports/.
//
// Deliberately NOT included: any Ctrl/Shift/Alt-modified name (e.g.
// "C-a", "S-Up") -- those are out of this task's scope (task 060/II-40
// forwards Ctrl combinations as literal control bytes through
// SendLiteral instead, precisely so C-b reaches the agent without ever
// matching tmux's own prefix table), and any mouse/wheel target name
// (e.g. "WheelUpPane") -- confirmed during the same survey to NOT be a
// key-string translation target at all (tmux typed it back literally,
// exactly like an unrecognized name), so it has no place in a KEY
// allowlist regardless of scope.
var namedKeyAllowlist = map[string]bool{
	"Up":    true,
	"Down":  true,
	"Left":  true,
	"Right": true,

	"Home": true,
	"End":  true,

	"IC":     true, // insert-character; alias below.
	"DC":     true, // delete-character; alias below.
	"Insert": true,
	"Delete": true,

	"PPage":    true, // page-up; aliases below.
	"NPage":    true, // page-down; aliases below.
	"PageUp":   true,
	"PageDown": true,
	"PgUp":     true,
	"PgDn":     true,

	"BSpace": true,
	"BTab":   true, // shift-tab / backtab.
	"Tab":    true,
	"Enter":  true,
	"Escape": true,
	"Space":  true,

	"F1": true, "F2": true, "F3": true, "F4": true,
	"F5": true, "F6": true, "F7": true, "F8": true,
	"F9": true, "F10": true, "F11": true, "F12": true,
}

// IsNamedKeyAllowed reports whether name is on namedKeyAllowlist, so a
// caller (or a test) can ask the question without going through a
// Dispatcher at all.
func IsNamedKeyAllowed(name string) bool {
	return namedKeyAllowlist[name]
}

// SendNamedKey sends name to the dispatcher's target as a tmux KEY NAME
// -- `send-keys <name>`, deliberately never `-l` -- so tmux's own
// key-string translation produces whatever bytes that name means (PRD
// item 34), and deck never hand-encodes the escape sequence itself.
//
// name is checked against namedKeyAllowlist BEFORE anything else
// happens: an unlisted name returns an error here, with no tmux
// invocation of any kind -- not even the identity re-resolution
// Dispatcher.Send would otherwise perform first. This is PRD item 35's
// "validate against an allowlist before spawning" read literally:
// key_test.go's TestSendNamedKeyRejectsAnUnknownNameWithoutSpawningTmux
// proves it by showing Verifications() does not move at all for a
// rejected name.
func (d *Dispatcher) SendNamedKey(ctx context.Context, name string) error {
	if !namedKeyAllowlist[name] {
		return fmt.Errorf("send named key %q to %q: %q is not on the named-key allowlist (PRD II-35) -- refusing before tmux is invoked at all", name, d.target, name)
	}
	if err := d.Send(ctx, "send-keys", name); err != nil {
		return fmt.Errorf("send named key %q to %q: %w", name, d.target, err)
	}
	return nil
}
