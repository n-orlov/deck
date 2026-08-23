package tui

import (
	"context"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestCoalescedKeyMsgDispatchesNavigationalThenDestructiveRune proves task
// 118's core defect and its fix without any PTY, tmux or host load involved
// (per operator steering 005): bubbletea's own Key.String() for a
// tea.KeyMsg{Type: tea.KeyRunes} returns string(k.Runes) verbatim, so a
// coalesced "jx" (two quick presses of 'j' then 'x' landing in the same
// read) would match no case in Update's switch at all if each rune were not
// dispatched separately -- the event would be dropped whole, moving nothing
// and killing nothing. This feeds exactly that synthetic multi-rune KeyMsg
// and asserts BOTH runes took effect, in order: the selection moved from
// the first session to the second (the navigational half), and the kill
// command that resulted was built against the NEWLY selected second
// session, not the one selected before the coalesced message arrived (the
// destructive half) -- proof that 'x' was dispatched after 'j' had already
// moved the selection, not against a stale snapshot.
func TestCoalescedKeyMsgDispatchesNavigationalThenDestructiveRune(t *testing.T) {
	model := NewWithShellCreator(nil, config.Settings{}, "", nil)
	model.kill = func(_ context.Context, s store.Session) error {
		return nil
	}
	model.sessions = []store.Session{
		{ID: "s1", Name: "alpha", Status: "running"},
		{ID: "s2", Name: "beta", Status: "running"},
	}
	model.selected = 0

	got, cmd := model.Update(key("jx"))
	model = got.(Model)

	if model.selected != 1 {
		t.Fatalf("selected = %d after coalesced \"jx\", want 1 (the 'j' half moved nothing)", model.selected)
	}
	if cmd == nil {
		t.Fatal("coalesced \"jx\" produced no command at all -- the whole event was dropped")
	}
	msg := cmd()
	killed, ok := msg.(sessionKilled)
	if !ok {
		t.Fatalf("coalesced \"jx\" command produced %#v, want a sessionKilled message", msg)
	}
	if killed.session.ID != "s2" {
		t.Fatalf("coalesced \"jx\" killed session %q, want %q (the row 'j' moved onto)", killed.session.ID, "s2")
	}
}

// TestCoalescedKeyMsgDispatchesDDChordAsTwoSeparateDeletes proves the same
// fix against the second pair operator steering names explicitly: a
// coalesced "dd" (the delete chord itself, both presses landing in one
// read) must behave exactly as two separate 'd' keystrokes -- the first
// sets the pending-delete indicator, the second (dispatched against the
// model state the first one just produced, not the original) opens the
// confirm dialog. Before task 118's fix this coalesced message matched no
// switch case at all, so pressing "dd" fast enough to coalesce silently
// deleted nothing and showed nothing -- a fail-safe defect (nothing
// destructive happens) but still a dropped event, not the documented
// two-key chord.
func TestCoalescedKeyMsgDispatchesDDChordAsTwoSeparateDeletes(t *testing.T) {
	model := NewWithShellCreator(nil, config.Settings{DeleteGrace: time.Hour}, "", nil)
	model.sessions = []store.Session{
		{ID: "s1", Name: "alpha", Status: "stopped"},
	}
	model.selected = 0

	got, _ := model.Update(key("dd"))
	model = got.(Model)

	if model.pendingDelete {
		t.Fatal("pendingDelete is still set after the coalesced \"dd\" chord -- the second 'd' did not clear it by opening the confirm dialog")
	}
	if !model.deleteConfirming {
		t.Fatal("coalesced \"dd\" did not open the delete confirm dialog -- the second rune was not dispatched")
	}
}

// TestCoalescedKeyMsgSinglePressUnaffected proves the fix leaves an
// ordinary single keystroke's own KeyMsg (len(Runes)==1) completely
// unchanged: it must not be routed through the new split-and-replay path at
// all (that path only ever triggers for len(Runes)>1), so every existing
// single-key behaviour keeps working exactly as it did before this task.
func TestCoalescedKeyMsgSinglePressUnaffected(t *testing.T) {
	model := NewWithShellCreator(nil, config.Settings{}, "", nil)
	model.sessions = []store.Session{
		{ID: "s1", Name: "alpha", Status: "running"},
		{ID: "s2", Name: "beta", Status: "running"},
	}
	model.selected = 0

	got, _ := model.Update(key("j"))
	model = got.(Model)
	if model.selected != 1 {
		t.Fatalf("a single 'j' moved selected to %d, want 1", model.selected)
	}
}
