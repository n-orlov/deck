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
// against a newer one directly from THIS repo's own CI) and the
// resulting capture-pane bytes inspected. Steer 020 §3 independently ran
// the identical survey method (private `tmux -L specver36` socket, pane
// running `sh -c "stty -echo; cat -v > file"` so bytes are recorded
// rather than re-interpreted) against a real tmux 3.6b server outside
// this workspace and reports all 20 names below pass, byte for byte
// identical to the 3.5a survey's own sequences -- so this holds on 3.5a
// (verified here) AND 3.6b (second-hand evidence, source: steer 020 §3,
// not independently re-verified from this repo). The ci/Dockerfile-pins-
// 3.5a-only caveat above still stands for what THIS suite can exercise;
// 3.6b coverage exists only as that steer's own report.
//
// Alt-modified variants of these (M-Left, C-M-Left, S-M-Home,
// C-M-S-End, ...) ARE included below as of issue #28, and this paragraph
// used to say the opposite: it justified their absence with
// "interactiveNamedKey refuses any msg.Alt key outright before it would
// ever ask for one of these names, so no caller can reach them yet".
// That refusal WAS the bug -- SPEC.md §11.9 names `Alt` alongside `Ctrl`
// and `Shift` in the very sentence this allowlist's modified-navigation
// entries come from, and the blanket refusal made every Alt-modified
// special key "a keystroke that does nothing and reports nothing", the
// failure §11.9's enumerate-key-by-key rule exists to prevent (see
// internal/tui/interactive.go's interactiveNamedKey and
// interactiveAltNamedKeys, which is now that caller). The names are
// therefore no longer untested dead weight: each one was surveyed the
// same way as every other entry here (one fresh tmux server per key on a
// private socket, sent raw with no deck code to a pane running `cat`,
// capture-pane bytes inspected) against a real tmux 3.6b, and
// key_test.go's
// TestSendNamedKeyDeliversAltModifiedNavigationKeysByTmuxsOwnTranslation
// re-runs that survey in the suite and holds the exact bytes. The 3.5a
// caveat above applies to the Alt names identically: the in-suite survey
// exercises whatever tmux the host provides, and ci/Dockerfile's pinned
// 3.5a is what CI itself verifies.
//
// Two Alt names are deliberately still absent, and this is where the
// "known, listed gap" §11.9 requires is recorded on the tmux side:
// "M-Insert"/"M-IC" (tmux translates it correctly, to the same bytes
// xterm sends, but bubbletea v1.3.10 cannot DECODE those bytes, so deck
// can never receive the keystroke to forward -- interactiveAltNamedKeys'
// own comment has the upstream transposition in full) and "M-BTab" (tmux
// itself discards the Alt: `send-keys M-BTab` was surveyed on 3.6b and
// delivers bytes identical to plain `BTab`, so naming it would forward
// Shift+Tab for a physical Alt+Shift+Tab). Both stay off this list
// because SendNamedKey's whole point is that a name reaching tmux has
// been confirmed to mean what the caller thinks it means.
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

	// Alt-modified navigation, page keys, Delete and the function keys
	// (issue #28) -- see the paragraph about them in the comment above
	// this map for why they are here now and were not before, and
	// internal/tui/interactive.go's interactiveAltNamedKeys for the
	// caret-notation byte table this set was surveyed against. tmux
	// spells Alt "M-" and accepts the modifier prefixes in any order;
	// these are written in the order the survey used. "M-Insert"/"M-IC"
	// and "M-BTab" are the two deliberate omissions the comment above
	// names.
	"M-Up": true, "M-Down": true, "M-Left": true, "M-Right": true,
	"M-Home": true, "M-End": true,
	"M-PageUp": true, "M-PageDown": true,
	"M-Delete": true,
	"C-M-Up":   true, "C-M-Down": true, "C-M-Left": true, "C-M-Right": true,
	"C-M-Home": true, "C-M-End": true, "C-M-PgUp": true, "C-M-PgDn": true,
	"S-M-Up": true, "S-M-Down": true, "S-M-Left": true, "S-M-Right": true,
	"S-M-Home": true, "S-M-End": true,
	"C-M-S-Up": true, "C-M-S-Down": true, "C-M-S-Left": true, "C-M-S-Right": true,
	"C-M-S-Home": true, "C-M-S-End": true,
	"M-F1": true, "M-F2": true, "M-F3": true, "M-F4": true,
	"M-F5": true, "M-F6": true, "M-F7": true, "M-F8": true,
	"M-F9": true, "M-F10": true, "M-F11": true, "M-F12": true,
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
