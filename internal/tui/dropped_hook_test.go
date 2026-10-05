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
	db, id := droppedHookBaseStore(t)
	deliverSupersededHook(t, db, id, "Stop", "gen-killed", 20)
	return db, id
}

const droppedHookConversationID = "conversation-dropped-hook"

// droppedHookBaseStore is droppedHookTestStore without its hook: a running
// row on launch generation "gen-current" and no events at all.
func droppedHookBaseStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const id = "dropped-hook-session"
	if _, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: id, Name: id, CWD: "/work", Agent: "claude", CapturedPath: "/bin",
		Status: "starting", StatusSource: "user", StatusAt: 1, CreatedAt: 1,
		ConversationID: droppedHookConversationID,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := db.DB().Exec(`UPDATE sessions SET status = 'running', status_source = 'hook', status_at = 7, launch_lease_owner = '4242@boot#gen-current', launch_lease_until = 0 WHERE id = ?`, id); err != nil {
		t.Fatalf("prime session launch generation: %v", err)
	}
	return db, id
}

// deliverSupersededHook drives hookrecv.Receive for one hook event whose pane
// handed back generation hookGeneration (empty for a pane that carries none).
func deliverSupersededHook(t *testing.T, db *store.Store, id, hookEvent, hookGeneration string, at int64) {
	t.Helper()
	raw := []byte(`{"hook_event_name":"` + hookEvent + `","session_id":"` + droppedHookConversationID + `","last_assistant_message":"from the dead pane"}`)
	if _, err := hookrecv.Receive(context.Background(), db, raw, id, hookGeneration, at); err != nil {
		t.Fatalf("receive superseded %s hook: %v", hookEvent, err)
	}
}

// recordRestart writes the two events a real restart/resume leaves on the
// row (service/restart.go's "restart", then the lease acquisition), at
// sequence positions before whatever hook the test delivers next.
func recordRestart(t *testing.T, db *store.Store, id string) {
	t.Helper()
	for _, kind := range []string{"restart", "launch_lease_acquired"} {
		if _, err := db.DB().Exec(`INSERT INTO events (session_id, at, kind, reason, payload) VALUES (?, 10, ?, 'user', '')`, id, kind); err != nil {
			t.Fatalf("record %s event: %v", kind, err)
		}
	}
}

// detailViewAfterI presses "i" on the row and returns the rendered detail
// dialog once the dropped-hook lookup has landed.
func detailViewAfterI(t *testing.T, db *store.Store, id string) string {
	t.Helper()
	session, err := db.GetSession(context.Background(), id)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	model := New(db, config.Settings{}, "")
	model.width, model.height = 100, 40
	model.sessions = []store.Session{session}
	model.selected = rowCursor(0)
	updated, cmd := model.Update(key("i"))
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("pressing \"i\" did not dispatch the dropped-hook lookup")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if !model.detail {
		t.Fatal("pressing \"i\" did not open the detail dialog")
	}
	return model.View()
}

// eventLogViewAfterE presses "E" and returns the rendered event log.
func eventLogViewAfterE(t *testing.T, db *store.Store, id string) string {
	t.Helper()
	session, err := db.GetSession(context.Background(), id)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	model := New(db, config.Settings{}, "")
	model.width, model.height = 100, 40
	model.sessions = []store.Session{session}
	model.selected = rowCursor(0)
	updated, cmd := model.Update(key("E"))
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	return model.View()
}

// TestDetailViewShowsNoHookDeclinedForTheReplacedPanesSessionEnd is R200
// (#64): a restart, then the old pane's own SessionEnd arriving late under the
// old generation, is the routine teardown of a replaced launch. `i` must not
// present it as "Hook declined".
func TestDetailViewShowsNoHookDeclinedForTheReplacedPanesSessionEnd(t *testing.T) {
	db, id := droppedHookBaseStore(t)
	recordRestart(t, db, id)
	deliverSupersededHook(t, db, id, "SessionEnd", "gen-killed", 20)
	view := detailViewAfterI(t, db, id)
	if strings.Contains(view, "Hook declined") {
		t.Fatalf("detail view alarms on the replaced pane's expected session_end:\n%s", view)
	}
	if strings.Contains(view, "session_end.superseded") {
		t.Fatalf("detail view mentions the hidden session_end.superseded event:\n%s", view)
	}
}

// TestEventLogStillListsTheRawSessionEndSupersededEvent: `E` keeps the raw
// event even though `i` hides it (R200).
func TestEventLogStillListsTheRawSessionEndSupersededEvent(t *testing.T) {
	db, id := droppedHookBaseStore(t)
	recordRestart(t, db, id)
	deliverSupersededHook(t, db, id, "SessionEnd", "gen-killed", 20)
	view := eventLogViewAfterE(t, db, id)
	if !strings.Contains(view, "session_end.superseded") {
		t.Fatalf("event log lost the raw session_end.superseded event:\n%s", view)
	}
	if !strings.Contains(view, "declined: hook launch generation") {
		t.Fatalf("event log lost the raw event's reason:\n%s", view)
	}
}

// TestDetailViewStillDeclinesALateStopFromTheReplacedGeneration: only the
// session_end is expected; a stop from the old pane after a restart is loud
// (R200), including when a newer, hidden session_end.superseded follows it.
func TestDetailViewStillDeclinesALateStopFromTheReplacedGeneration(t *testing.T) {
	db, id := droppedHookBaseStore(t)
	recordRestart(t, db, id)
	deliverSupersededHook(t, db, id, "Stop", "gen-killed", 20)
	deliverSupersededHook(t, db, id, "SessionEnd", "gen-killed", 30)
	view := detailViewAfterI(t, db, id)
	if !strings.Contains(view, "Hook declined:") || !strings.Contains(view, "stop.superseded") {
		t.Fatalf("detail view lost the loud decline for a late stop:\n%s", view)
	}
}

// TestDetailViewStillDeclinesAGenerationlessHookWithNoRestartBefore: a hook
// carrying no generation, against a row that holds one, with no restart or
// resume before it, is suspicious and stays loud (R200). The same hook AFTER
// a restart is the replaced pre-lease pane ending and is hidden.
func TestDetailViewStillDeclinesAGenerationlessHookWithNoRestartBefore(t *testing.T) {
	db, id := droppedHookBaseStore(t)
	deliverSupersededHook(t, db, id, "SessionEnd", "", 20)
	view := detailViewAfterI(t, db, id)
	if !strings.Contains(view, "Hook declined:") || !strings.Contains(view, "session_end.superseded") || !strings.Contains(view, "carries no launch") {
		t.Fatalf("detail view hid a generation-less session_end with no restart before it:\n%s", view)
	}

	db, id = droppedHookBaseStore(t)
	recordRestart(t, db, id)
	deliverSupersededHook(t, db, id, "SessionEnd", "", 20)
	if view := detailViewAfterI(t, db, id); strings.Contains(view, "Hook declined") {
		t.Fatalf("detail view alarms on a generation-less session_end after a restart:\n%s", view)
	}

	db, id = droppedHookBaseStore(t)
	recordRestart(t, db, id)
	deliverSupersededHook(t, db, id, "Stop", "", 20)
	if view := detailViewAfterI(t, db, id); !strings.Contains(view, "Hook declined:") {
		t.Fatalf("detail view hid a generation-less stop after a restart:\n%s", view)
	}
}

// TestEventLogDisplaysTheDroppedHookKindAndReasonAfterPressingE is
// R90/task 033's `E` half, driven through the real key: pressing "E"
// opens the log and dispatches loadEventLog as a tea.Cmd (R61 -- the
// store read never happens inline in View()), and once that reply lands
// the frame shows the superseded hook's distinct "<baseKind>.superseded"
// kind and its "declined: ..." reason (task 032). Going through Update
// rather than setting eventLogOpen directly is what makes this a claim
// about the `E` key a reader can press, not about a helper.
func TestEventLogDisplaysTheDroppedHookKindAndReasonAfterPressingE(t *testing.T) {
	db, id := droppedHookTestStore(t)
	session, err := db.GetSession(context.Background(), id)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	model := New(db, config.Settings{}, "")
	model.width, model.height = 100, 40
	model.sessions = []store.Session{session}
	model.selected = rowCursor(0)

	updated, cmd := model.Update(key("E"))
	model = updated.(Model)
	if !model.eventLogOpen {
		t.Fatalf("pressing \"E\" did not open the event log")
	}
	if cmd == nil {
		t.Fatal("pressing \"E\" did not dispatch the event log fetch")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)

	view := model.View()
	if !strings.Contains(view, "Event log") {
		t.Fatalf("pressing \"E\" did not render the event log:\n%s", view)
	}
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
	model.selected = rowCursor(0)

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
	model.selected = rowCursor(0)
	model.detail = true
	view := model.View()
	if strings.Contains(view, "Hook declined:") {
		t.Fatalf("detail view showed a dropped-hook label with none declined:\n%s", view)
	}
}
