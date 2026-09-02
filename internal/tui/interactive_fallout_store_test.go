package tui

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// attachedEventCountForTest counts durable 'attached' events for one
// session -- the exact wire-side observable RecordAttachment's own INSERT
// produces (store.go's RecordAttachment), read directly off the events
// table rather than from any in-memory list frame.
func attachedEventCountForTest(t *testing.T, db *store.Store, sessionID string) int {
	t.Helper()
	var n int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'attached'`, sessionID).Scan(&n); err != nil {
		t.Fatalf("count attached events: %v", err)
	}
	return n
}

// TestRaiseLostAttachRecordsNothingDurable proves task 120: falling out of
// the interactive preview (either displacement flavour's shared exit,
// raiseLostAttach) is not itself a second deck-mediated attachment. Entry
// (enterInteractiveBody) already runs the one durable transaction SPEC §7
// assigns it -- store.RecordAttachment, wired as m.prepareAttach exactly as
// attachSelected's own `a` path uses it -- which answers a waiting row
// (acknowledged=1, notify_epoch+1) and appends one 'attached' event. This
// test captures that baseline on the DURABLE ROW right after entry, then
// drives raiseLostAttach (exitInteractive's own teardown, the same exit
// displacement_teardown_test.go exercises against tmux state), and asserts
// the row's attached-event count, acknowledged flag and notify_epoch are
// byte-identical to the pre-fall-out baseline: exitInteractive/
// teardownInteractiveClaim never call into the store at all, so a
// regression that added such a call (e.g. a second RecordAttachment, or an
// AcknowledgeSession, on the way out) is exactly what this test would catch
// on the row itself, not on the in-memory Model/list frame.
func TestRaiseLostAttachRecordsNothingDurable(t *testing.T) {
	socket := selectionTestSocket("fallout")
	session := "deck_fallout"
	newQuietSelectionPane(t, socket, session, 80, 24)
	client := tmux.Client{Socket: socket}

	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	const sessionID = "sess-fallout-1"
	if _, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: sessionID, Name: "fallout", CWD: "/work", Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	// Put the row into the waiting attention state before entry, so
	// enterInteractiveBody's own RecordAttachment call has something to
	// answer -- the exact durable side effect this test proves
	// raiseLostAttach's own fall-out never repeats or undoes.
	if err := db.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID: sessionID, Status: "waiting", Reason: "permission_prompt", Source: "hook", At: 10,
	}); err != nil {
		t.Fatal(err)
	}

	clock, err := config.NewClock("2025-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}

	m := New(db, config.Settings{Color: true, Clock: clock}, "")
	m.width, m.height = 100, 30
	if _, h := m.previewContentSize(); h < interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", h, interactiveMinInnerRows)
	}
	m.tmuxClient = client
	m.sessions = []store.Session{{ID: sessionID, Name: "fallout", Slug: "fallout", Status: "waiting"}}
	m.selected = 0

	next, _ := m.enterInteractiveBody(false)
	got := next.(Model)
	if !got.interactive {
		t.Fatalf("entry did not enter interactive mode: attachError=%q", got.attachError)
	}

	// Baseline: confirm entry itself DID record durably, so the assertion
	// below is against a row that actually moved, not one that started
	// (and stayed) at its zero value.
	before, err := db.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !before.Acknowledged || before.NotifyEpoch != 1 {
		t.Fatalf("test assumption violated: entry did not record the expected durable side effect: %#v", before)
	}
	beforeCount := attachedEventCountForTest(t, db, sessionID)
	if beforeCount != 1 {
		t.Fatalf("test assumption violated: attached-event count after entry = %d, want 1", beforeCount)
	}

	// Falling out of the preview: raiseLostAttach's own exitInteractive,
	// exactly as previewTick's fast path/backstop would drive it (task 118)
	// once the claim was stolen or a real client attached directly.
	after, _ := got.raiseLostAttach("fallout")
	if after.interactive {
		t.Fatalf("raiseLostAttach did not leave interactive mode")
	}
	if !after.lostAttach {
		t.Fatalf("raiseLostAttach did not raise the lost-attach dialog")
	}

	row, err := db.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if row.Acknowledged != before.Acknowledged || row.NotifyEpoch != before.NotifyEpoch {
		t.Fatalf("fall-out changed durable attention state: before=%#v after=%#v", before, row)
	}
	if gotCount := attachedEventCountForTest(t, db, sessionID); gotCount != beforeCount {
		t.Fatalf("fall-out changed attached-event count: before=%d after=%d", beforeCount, gotCount)
	}
}
