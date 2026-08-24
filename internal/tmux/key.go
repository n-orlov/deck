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
// Deliberately NOT included: a bare Ctrl-letter name (e.g. "C-a") --
// those are out of THIS allowlist's scope on purpose (task 060/II-40
// forwards Ctrl combinations as literal control bytes through
// SendLiteral instead, precisely so C-b reaches the agent without ever
// matching tmux's own prefix table, because a control byte's value is
// fixed and does not depend on the pane's terminal mode the way the
// modified-navigation names just below this comment do), and any
// mouse/wheel target name (e.g. "WheelUpPane") -- confirmed during the
// original survey to NOT be a key-string translation target at all
// (tmux typed it back literally, exactly like an unrecognized name), so
// it has no place in a KEY allowlist regardless of scope.
//
// Ctrl/Shift/Ctrl+Shift combinations with the arrows, Home, End and the
// page keys ARE included below (steer 017 item 1 / SPEC.md §11.9 "Modified
// navigation keys forward, like the unmodified ones, by tmux key name"):
// unlike a bare Ctrl-letter, their encoding IS mode-dependent (application
// cursor keys vs. not), exactly like a bare arrow or Home/End already
// above -- so the same reasoning that puts "Home" here puts "C-Left" here
// too. Every name below was confirmed the same way as the rest of this
// map: sent (raw, no deck code) to a pane running `cat` on a real tmux
// 3.5a server (this repo's ci/Dockerfile pins golang:1.25-trixie, whose
// apt tmux package is 3.5a; a protected path deck cannot edit to test
// against a newer one) and the resulting capture-pane bytes inspected.
// Alt-modified variants of these (e.g. Ctrl+Alt+Left) are NOT included:
// interactiveNamedKey refuses any msg.Alt key outright before it would
// ever ask for one of these names, so no caller can reach them yet --
// adding the name here without a caller that can produce it would be
// untested dead weight in an allowlist that exists specifically so
// nothing untested is ever passed to tmux.
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

	// Modified navigation -- see the comment above this map. Confirmed
	// against tmux 3.5a: each of these produces the identical CSI
	// sequence charmbracelet/bubbletea@v1.3.10/key.go's own `sequences`
	// table decodes back into the KeyCtrl*/KeyShift*/KeyCtrlShift*
	// constants interactiveNamedKey maps from (e.g. "C-Left" produces
	// the same bytes bubbletea's own table decodes as KeyCtrlLeft), so
	// tmux's translation and bubbletea's decoding agree byte for byte --
	// see key_test.go's TestSendNamedKeyDeliversModifiedNavigationKeysByTmuxsOwnTranslation
	// for the exact bytes captured, spelled there in capture-pane's own
	// caret notation rather than as a Go escape literal, which this
	// package's own grep guard (key_grep_test.go) forbids in this file.
	"C-Up": true, "C-Down": true, "C-Left": true, "C-Right": true,
	"C-Home": true, "C-End": true, "C-PgUp": true, "C-PgDn": true,
	"S-Up": true, "S-Down": true, "S-Left": true, "S-Right": true,
	"S-Home": true, "S-End": true,
	"C-S-Up": true, "C-S-Down": true, "C-S-Left": true, "C-S-Right": true,
	"C-S-Home": true, "C-S-End": true,
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
