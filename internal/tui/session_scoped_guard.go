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
var sessionScopedKeys = map[string]bool{
	"enter": true, "a": true, "x": true, "d": true, "r": true, "R": true,
	"i": true, "e": true, "P": true, "p": true, "Y": true, "m": true,
	"A": true, "U": true, "s": true, "z": true, "detail:g": true,
}

// markBatchKeys are the two session-scoped keys task 112 gave a second,
// batch meaning: with a non-empty mark set, `x` kills and `dd` deletes the
// WHOLE marked set rather than the cursor's own row, so neither may be
// gated by a header cursor while marks are in force -- the batch does not
// need the cursor to name anything. With an empty mark set both act on the
// cursor's row alone and are guarded exactly like every other key above.
// This is one map rather than two ad-hoc `key == "x"` tests so the two
// batch keys can never drift apart on the header question.
var markBatchKeys = map[string]bool{"x": true, "d": true}

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
// The two markBatchKeys carry one deliberate exception: a non-empty mark
// set (task 112) means they act on the WHOLE marked batch instead of the
// cursor's own row, so a header cursor must never gate that batch path.
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
	if markBatchKeys[key] && len(m.marked) > 0 {
		return false
	}
	return !m.hasSelectedSession()
}
