package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/hookrecv"
	"github.com/n-orlov/deck/internal/store"
)

// droppedHookTestStore builds a real state.db with one claude session and
// drives internal/hookrecv.Receive through supersededLaunch's actual
// decline path (R74/R90, task 032): the row's launch_lease_owner names
// generation "gen-current", the hook's pane hands back "gen-killed", so
// the write is declined and recorded under supersededEventKind's own
// "<baseKind>.superseded" kind and supersededReason's own explanation --
// never a hand-built store.Event, so the tests below prove what the real
// write path produces, not what a test imagines it should.
func droppedHookTestStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	const id = "dropped-hook-session"
	const conversationID = "conversation-dropped-hook"
	if _, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: id, Name: id, CWD: "/work", Agent: "claude", CapturedPath: "/bin",
		Status: "starting", StatusSource: "user", StatusAt: 1, CreatedAt: 1,
		ConversationID: conversationID,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := db.DB().Exec(`UPDATE sessions SET status = 'running', status_source = 'hook', status_at = 7, launch_lease_owner = '4242@boot#gen-current', launch_lease_until = 0 WHERE id = ?`, id); err != nil {
		t.Fatalf("prime session launch generation: %v", err)
	}
	raw := []byte(`{"hook_event_name":"Stop","session_id":"` + conversationID + `","last_assistant_message":"from the dead pane"}`)
	if _, err := hookrecv.Receive(context.Background(), db, raw, id, "gen-killed", 20); err != nil {
		t.Fatalf("receive superseded hook: %v", err)
	}
	return db, id
}

// TestEventLogDisplaysTheDroppedHookKindAndReason is R90/task 033's `E`
// half: the event log renders event.Kind/event.Reason for every row
// unconditionally, so a superseded hook's distinct
// "<baseKind>.superseded" kind and its "declined: ..." reason (task 032)
// must already be visible without any special-casing -- this proves that
// stays true rather than merely being asserted by inspection.
func TestEventLogDisplaysTheDroppedHookKindAndReason(t *testing.T) {
	db, _ := droppedHookTestStore(t)
	model := eventLogTestModelWithRowsLoaded(t, db)
	view := model.View()
	if !strings.Contains(view, "stop.superseded") {
		t.Fatalf("event log missing the dropped hook's distinct kind:\n%s", view)
	}
	if !strings.Contains(view, "declined: hook launch generation") {
		t.Fatalf("event log missing the dropped hook's reason:\n%s", view)
	}
}

// TestDetailViewDisplaysTheDroppedHookKindAndReasonAfterPressingI is
// R90/task 033's `i` half: pressing "i" dispatches loadDetailDroppedHook
// (R61 -- the store read happens as a tea.Cmd, not inline in View()), and
// once its detailDroppedHookLoaded reply lands, the detail body states
// which hook was declined and why, in the same wording `E` uses.
func TestDetailViewDisplaysTheDroppedHookKindAndReasonAfterPressingI(t *testing.T) {
	db, id := droppedHookTestStore(t)
	session, err := db.GetSession(context.Background(), id)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	model := New(db, config.Settings{}, "")
	model.width, model.height = 100, 40
	model.sessions = []store.Session{session}
	model.selected = 0

	updated, cmd := model.Update(key("i"))
	model = updated.(Model)
	if !model.detail {
		t.Fatalf("pressing \"i\" did not open the detail dialog")
	}
	if cmd == nil {
		t.Fatal("pressing \"i\" did not dispatch the dropped-hook lookup")
	}
	msg := cmd()
	updated, _ = model.Update(msg)
	model = updated.(Model)

	view := model.View()
	if !strings.Contains(view, "Hook declined:") {
		t.Fatalf("detail view missing the dropped-hook label:\n%s", view)
	}
	if !strings.Contains(view, "stop.superseded") {
		t.Fatalf("detail view missing the dropped hook's distinct kind:\n%s", view)
	}
	if !strings.Contains(view, "declined: hook launch generation") {
		t.Fatalf("detail view missing the dropped hook's reason:\n%s", view)
	}
}

// TestDetailViewOmitsTheDroppedHookLabelWhenNoneWasDeclined proves the new
// field is conditional: a session that never had a hook declined shows no
// "Hook declined:" line at all, rather than an empty/placeholder one.
func TestDetailViewOmitsTheDroppedHookLabelWhenNoneWasDeclined(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{ID: "plain", Name: "plain-shell", Agent: "shell", Status: "running"}}
	model.selected = 0
	model.detail = true
	view := model.View()
	if strings.Contains(view, "Hook declined:") {
		t.Fatalf("detail view showed a dropped-hook label with none declined:\n%s", view)
	}
}
