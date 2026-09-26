package tui

// sessionScopedKeys is the exact set SPEC §11's keymap and the PRD's D.2
// (task 013, R137) name as acting on "the selected session" -- every one of
// them must do nothing at all -- no model mutation, no tea.Cmd -- when the
// cursor rests on a group header rather than a session row (task 012/D.1's
// sidebarCursor makes a header a first-class stop, so this is now reachable
// with nothing else selected). "s" (send message, §11.1) and "z" (snooze)
// are listed even though neither key is wired to anything yet in this
// codebase -- out of scope for this phase -- precisely so that whichever
// task wires either one up inherits the guard for free instead of having to
// remember it exists.
//
// "detail:g" is a synthetic entry, never a real tea.KeyMsg.String() value:
// the `i` detail dialog binds its own move-group picker to a bare "g"
// (rename.go), a completely different action from the top-level `g`
// (jump to the first visual stop, tui.go) that happens to share the same
// letter -- the two are guarded through this one shared map under
// distinct keys so neither collides with or gates the other.
//
// "l" is the `i` detail dialog's own launch-inputs entry (rename.go, task
// 023, SPEC §6.2/§11.4). Unlike "g" it needs no synthetic key: the bare
// literal "l" is not bound to anything at the top level of Model.Update,
// so there is nothing for it to collide with there, and the requirement is
// the same either way -- the cursor must name a session before this key
// does anything.
// cure-01-03 (R142/R148, SPEC §11.9): "F" (force-attach) joined this map
// alongside its own entry in driftEndingKeys below. Before this fix, "F"
// was only in driftEndingKeys, never in sessionScopedKeys, so
// guardSessionScopedKey's refuse computation ("sessionScopedKeys[key] &&
// !m.hasSelectedSession()") was unconditionally false for "F" no matter
// what the cursor named -- a header cursor never refused it. The keypress
// itself stayed harmless (enterInteractiveBody's own !m.hasSelectedSession()
// check already makes force-attach a no-op on a header), but the guard's
// side effect ran anyway: endsDrift("F") is true, so a header press ended a
// live wheel drift for an action that changed nothing at all -- exactly
// the "unchanged action semantics" §11.9's "F is enter's own twin" must
// hold for. Adding "F" here makes guardSessionScopedKey refuse it on a
// header exactly like "enter" -- returning before the drift check ever
// runs -- while leaving the selected-session case (refuse always false
// there) byte-for-byte unchanged.
var sessionScopedKeys = map[string]bool{
	"enter": true, "a": true, "x": true, "d": true, "r": true, "R": true,
	"i": true, "e": true, "P": true, "p": true, "Y": true, "m": true,
	"A": true, "U": true, "s": true, "z": true, "l": true, "F": true,
	"detail:g": true,
}

// driftEndingKeys is task 005's (R142/GH #40) closed list of the keys that
// end a wheel drift through guardSessionScopedKey below. SPEC §11: a drift
// ends at "the user's next key that either moves the selection or acts on
// it" -- every session- or header-scoped key first brings the selection
// back into view -- while "keys that name no selection (`?`, `q`, `<`/`>`,
// settings) leave the drift alone"; the PRD's R142 adds filter toggles to
// that second list.
//
// It is deliberately an allow-list, not an exclusion list: an earlier
// version of this task ended the drift on every key the guard let through
// except `?`/`q`/`ctrl+c`/`<`/`>`, which silently swept in `,` (settings),
// `/` (the filter field), `t` (the theme picker) and every other global
// binding that names no selection. A key that is not listed here -- any
// global binding today, and any binding a later task adds without deciding
// otherwise -- leaves the drift in place.
//
// The members are: every sessionScopedKeys entry (checked directly in
// guardSessionScopedKey, so a key added to that map inherits this too),
// `F` (force-attach, `↵`'s own twin, named in SPEC §11's list), the header
// fold keys (`c`, `left`, `right`, all three of which re-invoke
// setSelection immediately after this guard runs), and every plain
// navigation key (up/k, down/j, pgup/pgdown, g/G, space's attention walk).
var driftEndingKeys = map[string]bool{
	"F": true,
	"c": true, "left": true, "right": true,
	"up": true, "k": true, "down": true, "j": true,
	"pgup": true, "pgdown": true, "g": true, "G": true, " ": true,
}

// endsDrift reports whether key is one of the keys driftEndingKeys and
// sessionScopedKeys together name as moving or acting on the selection.
func endsDrift(key string) bool {
	return sessionScopedKeys[key] || driftEndingKeys[key]
}

// guardSessionScopedKey is the ONE place task 013/D.2 decides whether a
// session-scoped keypress is allowed to reach its own handler at all. Before
// this guard existed, every one of the bindings above (bar one: rename.go's
// detail-mode `g`, the move-group picker, mutated unconditionally with no
// check whatsoever) made this decision for itself via its own
// `if session, ok := m.selectedSession(); ok` -- a per-site convention that
// only works as long as every site remembers it. This guard replaces that
// convention with a single fact checked once, before any binding's own
// service-availability or eligibility logic ever runs, so a header cursor
// short-circuits a binding even ahead of its own "service unavailable"
// messaging.
//
// It reports true (the keypress must be swallowed with no mutation and no
// command) when key names one of the bindings above AND the cursor cannot
// currently resolve to a session -- either because it rests on a group
// header or because m.sessions is empty or the cursor has drifted out of
// range (hasSelectedSession covers both).
//
// task cure-01-01 (F1, R137, SPEC §11): `x` and `dd` used to carry a
// deliberate exception here for a non-empty mark set (task 112's batch
// path), on the theory that a batch action needs no session named by the
// cursor. Review found that theory wrong: SPEC §11 makes every one of
// these keys inert on a header with NO exception, and the batch path is no
// different from the single-row path in that respect -- a header cursor
// must gate `x` and both presses of `dd` exactly the same whether or not
// marks are in force. There is no exemption left to carve out.
//
// `d` is in the map above and goes through this guard TWICE per dd chord,
// because both halves of the chord mutate: the first `d` raises
// m.pendingDelete (tui.go's case "d"), which the guard now refuses on a
// header before it happens, and the second `d` is intercepted by
// m.pendingDelete's own branch in Update, which runs ahead of this guard on
// purpose (it must swallow and clear the indicator for EVERY key, including
// keys this guard would otherwise refuse) and so asks this guard itself
// rather than re-deciding the header question with a second selectedSession
// check of its own.
//
// task 005 (R142/GH #40): this is also now the ONE shared site (this
// package's every call site -- tui.go's pre-switch call every key not
// intercepted by an overlay reaches, and rename.go's three `i`-detail-
// dialog calls -- is unchanged; only this function's own body grew a new
// side effect) that brings a drifted selection back into view. Whenever
// the key is let through (refuse comes back false) and endsDrift names it
// (driftEndingKeys above), a drift in force is followed with one row
// of context (followSelectionViewport, the same margin every ordinary
// setSelection call already uses) and cleared on this exact keypress --
// before the caller's own switch/case body ever runs, so an action that
// mutates on the very same press (dd's confirm, task 013's `i`, rename.go's
// `r`/`l`/detail `g`) already has the target row on screen by the time it
// acts. A refused key (sessionScopedKeys[key] true, no selected session)
// never reaches this: a header cursor names no row to bring into view, and
// SPEC's rule only ends the drift on a key that actually moves or acts on
// the selection, which a swallowed key by definition does not.
func (m *Model) guardSessionScopedKey(key string) bool {
	refuse := sessionScopedKeys[key] && !m.hasSelectedSession()
	if !refuse && m.sidebarScrollDrifted && endsDrift(key) {
		m.followSelectionViewport()
		m.sidebarScrollDrifted = false
	}
	return refuse
}
