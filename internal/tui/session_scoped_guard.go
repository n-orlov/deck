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
var sessionScopedKeys = map[string]bool{
	"enter": true, "a": true, "x": true, "d": true, "r": true, "R": true,
	"i": true, "e": true, "P": true, "p": true, "Y": true, "m": true,
	"A": true, "U": true, "s": true, "z": true, "l": true, "detail:g": true,
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
func (m Model) guardSessionScopedKey(key string) bool {
	if !sessionScopedKeys[key] {
		return false
	}
	return !m.hasSelectedSession()
}
