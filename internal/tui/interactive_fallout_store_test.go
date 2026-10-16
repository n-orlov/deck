package tui

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/interactive"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// attachedEventCountForTest counts durable 'attached' events for one
// session, read directly off the events table rather than from any
// in-memory list frame. Since R320 nothing writes one: entry is inert.
func attachedEventCountForTest(t *testing.T, db *store.Store, sessionID string) int {
	t.Helper()
	var n int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'attached'`, sessionID).Scan(&n); err != nil {
		t.Fatalf("count attached events: %v", err)
	}
	return n
}

// durableAttentionForTest is the whole durable observable task 120 cares
// about, read off the row (store.GetSession) and the events table -- never
// off the Model or the rendered list frame.
type durableAttentionForTest struct {
	Acknowledged  bool
	NotifyEpoch   int64
	AttachedCount int
}

func readDurableAttentionForTest(t *testing.T, db *store.Store, sessionID string) durableAttentionForTest {
	t.Helper()
	row, err := db.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession(%s): %v", sessionID, err)
	}
	return durableAttentionForTest{
		Acknowledged:  row.StatusSeen(),
		NotifyEpoch:   row.NotifyEpoch,
		AttachedCount: attachedEventCountForTest(t, db, sessionID),
	}
}

// newFalloutStoreModel wires a model to a REAL store (store.OpenPath, not a
// stub) holding one unseen waiting session: that row, which entry leaves
// untouched (R320), is the baseline every assertion below is made against. The returned model is otherwise exactly the interactive
// entry fixture the other displacement tests use (real tmux socket, a
// preview above the 7-row floor).
func newFalloutStoreModel(t *testing.T, client tmux.Client, slug, sessionID string) (Model, *store.Store) {
	t.Helper()
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	if _, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: sessionID, Name: slug, CWD: "/work", Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	// The waiting attention state before entry: unseen, epoch 0, no
	// 'attached' event. Entry and the fall-out below must leave it alone.
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
	m.sessions = []store.Session{{ID: sessionID, Name: slug, Slug: slug, Status: "waiting"}}
	m.selected = rowCursor(0)
	return m, db
}

// enterAndBaselineFallout enters interactive mode for real and returns the
// interactive model plus the durable baseline. The row is an unseen waiting
// one, so a wrongful AcknowledgeSession or any attention write during entry
// or the fall-out would change it and fail the comparison: entry itself must
// leave it exactly as it was (R320), which is asserted here.
func enterAndBaselineFallout(t *testing.T, m Model, db *store.Store, sessionID string) (Model, durableAttentionForTest) {
	t.Helper()
	next, _ := m.enterInteractiveBody(false)
	got := next.(Model)
	if !got.interactive {
		t.Fatalf("entry did not enter interactive mode: attachError=%q", got.attachError)
	}
	entered := readDurableAttentionForTest(t, db, sessionID)
	wantEntered := durableAttentionForTest{Acknowledged: false, NotifyEpoch: 0, AttachedCount: 0}
	if entered != wantEntered {
		t.Fatalf("durable state after entry = %+v, want the untouched unseen waiting row %+v (R320: entry is inert)", entered, wantEntered)
	}
	return got, entered
}

// TestPreviewTickStolenFalloutRecordsNothingDurable proves task 120 for the
// stolen-claim flavour: a REAL displacement (a second process force-claims
// the same window, exactly task 118's fast path) noticed by the real
// previewTick handler, driving the real fall-out -- no direct
// raiseLostAttach call anywhere in the test.
//
// The claim is made on the DURABLE STORE ROW, not the frame: the displaced
// client's whole fall-out must leave attached-event count, acknowledged and
// notify_epoch exactly as entry left them. Entry itself is inert
// (R320: it writes nothing); leaving the preview
// because someone else took the window must not acknowledge, re-notify or log
// an event of its own. A regression that added such a write on the way out
// (an AcknowledgeSession in the teardown, an event append in the dialog)
// fails here on the row itself.
//
// The stealing model is deliberately wired to its OWN separate store (a
// second store.OpenPath under its own temp dir, and in fact New(nil, ...)),
// so the winner's own legitimate entry can never
// write to the displaced client's row and mask or fake this assertion.
func TestPreviewTickStolenFalloutRecordsNothingDurable(t *testing.T) {
	socket := selectionTestSocket("falloutstolen")
	newQuietSelectionPane(t, socket, "deck_falloutstolen", 80, 24)
	client := tmux.Client{Socket: socket}

	const sessionID = "sess-falloutstolen-1"
	loser, db := newFalloutStoreModel(t, client, "falloutstolen", sessionID)
	got1, before := enterAndBaselineFallout(t, loser, db, sessionID)
	if got1.interactiveGrid.Status() != interactive.StatusLive {
		t.Fatalf("first entry's transport status = %v before any steal, want StatusLive", got1.interactiveGrid.Status())
	}

	// The steal: a second, store-less model force-claims the very same
	// window. Its own entry re-arms pipe-pane on the same target, which
	// displaces the first holder's pipe (task 118's fast path).
	winner := New(nil, config.Settings{Color: true}, "")
	winner.width, winner.height = 120, 40
	winner.tmuxClient = client
	winner.sessions = []store.Session{{ID: "sess-falloutstolen-2", Name: "falloutstolen", Slug: "falloutstolen", Status: "waiting"}}
	winner.selected = rowCursor(0)
	next2, _ := winner.enterInteractiveBody(true)
	got2 := next2.(Model)
	if got2.attachError != "" {
		t.Fatalf("the steal was refused: %q", got2.attachError)
	}
	if !got2.interactive {
		t.Fatalf("the steal did not enter interactive mode")
	}
	defer got2.exitInteractive()

	if !waitForDisplacementTest(t, 2*time.Second, func() bool {
		return got1.interactiveGrid.Status() == interactive.StatusDisplaced
	}) {
		t.Fatalf("displaced holder's grid never reported StatusDisplaced within 2s of the steal")
	}

	// Nothing durable moved merely because the steal landed, either: the
	// baseline still holds right up to the fall-out itself.
	if atSteal := readDurableAttentionForTest(t, db, sessionID); atSteal != before {
		t.Fatalf("the steal itself changed the displaced client's durable state: before=%+v after=%+v", before, atSteal)
	}

	// The real fall-out: previewTick's own displacement check notices the
	// StatusDisplaced signal and leaves interactive mode into the dialog.
	updated, _ := got1.Update(previewTick(time.Now()))
	m := updated.(Model)
	if m.interactive {
		t.Fatalf("previewTick did not leave interactive mode after the fast-path StatusDisplaced signal")
	}
	if !m.lostAttach {
		t.Fatalf("previewTick did not raise the lost-attach dialog after the fast-path StatusDisplaced signal")
	}

	if after := readDurableAttentionForTest(t, db, sessionID); after != before {
		t.Fatalf("the displaced client's fall-out changed durable state: before=%+v after=%+v (falling out of the preview must record nothing)", before, after)
	}
}

// TestPreviewTickClientAttachFalloutRecordsNothingDurable proves the same
// durable-store claim for task 118's OTHER displacement flavour, the one
// the fast path can never see: a real tmux client attaches directly to the
// claimed window, so only the async backstop poll
// (checkInteractiveDisplacementBackstop -> interactiveDisplacementChecked)
// catches it. The fall-out is again driven entirely through the real
// message path -- previewTick, then the backstop's own reply message -- and
// the assertion is again on the durable row, not the frame.
func TestPreviewTickClientAttachFalloutRecordsNothingDurable(t *testing.T) {
	socket := selectionTestSocket("falloutattach")
	newQuietSelectionPane(t, socket, "deck_falloutattach", 80, 24)
	client := tmux.Client{Socket: socket}

	const sessionID = "sess-falloutattach-1"
	m, db := newFalloutStoreModel(t, client, "falloutattach", sessionID)
	got, before := enterAndBaselineFallout(t, m, db, sessionID)

	windowTarget, err := tmux.SessionName("falloutattach")
	if err != nil {
		t.Fatalf("SessionName: %v", err)
	}
	// The displacement: a real client attaches directly. This never
	// touches deck's ownership option and never arms a competing
	// pipe-pane, so the fast path stays silent.
	attachForceEnterPTY(t, socket, windowTarget)
	waitForSessionAttachedCountForce(t, client, windowTarget, 1)
	if got.interactiveGrid.Status() != interactive.StatusLive {
		t.Fatalf("grid status = %v after a plain client attach, want StatusLive: this flavour must be invisible to the fast path", got.interactiveGrid.Status())
	}

	updated, cmd := got.Update(previewTick(time.Now()))
	m2 := updated.(Model)
	if !m2.interactive {
		t.Fatalf("previewTick left interactive mode synchronously; the client-attached flavour is only caught by the async backstop poll")
	}
	checked := findInteractiveDisplacementChecked(t, cmd)
	if !checked.displaced {
		t.Fatalf("backstop poll reported displaced=false against a real attached client")
	}

	// The real fall-out, through the backstop's own message.
	updated2, _ := m2.Update(checked)
	m3 := updated2.(Model)
	if m3.interactive {
		t.Fatalf("interactiveDisplacementChecked did not leave interactive mode")
	}
	if !m3.lostAttach {
		t.Fatalf("interactiveDisplacementChecked did not raise the lost-attach dialog")
	}

	if after := readDurableAttentionForTest(t, db, sessionID); after != before {
		t.Fatalf("the displaced client's fall-out changed durable state: before=%+v after=%+v (falling out of the preview must record nothing)", before, after)
	}
}
