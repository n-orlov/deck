package tui

import (
	"context"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// R320: attach (full or interactive), click and passive preview never
// change a row's status or seen state, and the row never moves, in the
// attention and activity sort orders. The old waiting -> running flip on
// attach (and the error acknowledgement) are gone; these tests replace
// TestRecordAttachmentHandlesWaitingErrorAndRacedStatusAtomically (store),
// TestAAtomicallyClearsSelectedWaitingEpisodeBeforeAttach and
// TestAAcknowledgesErrorWithoutChangingVerdict, which pinned them.

// durableSnapshot is everything durable about one row: the whole session
// record (status, reason, source, status_at, seen_status_at, epoch, ...)
// plus the number of events it holds.
type durableSnapshot struct {
	Session store.Session
	Events  int
}

func openInertStore(t *testing.T) *store.Store {
	t.Helper()
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// seedInertRow creates a row that reached status through a hook transition,
// so a waiting/error row is genuinely unseen. n separates the rows' stamps
// and output buckets.
func seedInertRow(t *testing.T, db *store.Store, id, status string, n int64) {
	t.Helper()
	ctx := context.Background()
	at := 1000 * n
	if _, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: id, Name: id, CWD: "/work/" + id, Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: at, CreatedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
	if status != "running" {
		if err := db.UpdateSessionStatus(ctx, store.StatusUpdateInput{
			SessionID: id, Status: status, Reason: "seeded " + status, Source: "hook", At: at + 10,
		}); err != nil {
			t.Fatal(err)
		}
	}
	// One output bucket (10 s) per row, so activity order differs from
	// attention order and from creation order.
	if err := db.RecordOutput(ctx, id, (100-n)*10_000); err != nil {
		t.Fatal(err)
	}
}

func snapshotDurable(t *testing.T, db *store.Store, id string) durableSnapshot {
	t.Helper()
	row, err := db.GetSession(context.Background(), id)
	if err != nil {
		t.Fatalf("GetSession(%s): %v", id, err)
	}
	var n int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return durableSnapshot{Session: row, Events: n}
}

var inertRows = []struct{ id, status string }{
	{"row-idle", "idle"},
	{"row-running", "running"},
	{"row-waiting", "waiting"},
	{"row-error", "error"},
}

// inertFixture seeds one row per status, wires a model over the real
// store, loads it in sortOrder and selects target.
func inertFixture(t *testing.T, sortOrder, target string, mouse bool) (Model, *store.Store) {
	t.Helper()
	db := openInertStore(t)
	for i, row := range inertRows {
		seedInertRow(t, db, row.id, row.status, int64(i+1))
	}
	settings := config.Settings{ASCII: true, SortOrder: sortOrder, Mouse: mouse, AttachOnClick: mouse}
	m := NewWithShellCreatorAndAttacher(db, settings, "", nil,
		func(context.Context, string) (*exec.Cmd, error) { return exec.Command("true"), nil })
	m.width, m.height = 100, 30
	m = reloadInert(t, m)
	return selectInert(t, m, target), db
}

func reloadInert(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.Update(m.loadSessions())
	return updated.(Model)
}

func selectInert(t *testing.T, m Model, id string) Model {
	t.Helper()
	for i, s := range m.sessions {
		if s.ID == id {
			m.selected = rowCursor(i)
			return m
		}
	}
	t.Fatalf("row %s not loaded", id)
	return m
}

func inertOrder(m Model) []string {
	ids := make([]string, 0, len(m.sessions))
	for _, s := range m.sessions {
		ids = append(ids, s.ID)
	}
	return ids
}

// assertInert fails unless every row is durably byte-identical to before and
// the reloaded list shows the same order.
func assertInert(t *testing.T, what string, m Model, db *store.Store, order []string, before map[string]durableSnapshot) {
	t.Helper()
	m = reloadInert(t, m)
	for id, want := range before {
		if got := snapshotDurable(t, db, id); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s changed row %s (status/seen/events must be untouched):\nbefore=%+v\nafter =%+v", what, id, want, got)
		}
	}
	if got := inertOrder(m); !reflect.DeepEqual(got, order) {
		t.Fatalf("%s moved a row: order before=%v after=%v", what, order, got)
	}
}

func snapshotAll(t *testing.T, db *store.Store) map[string]durableSnapshot {
	t.Helper()
	all := map[string]durableSnapshot{}
	for _, row := range inertRows {
		all[row.id] = snapshotDurable(t, db, row.id)
	}
	return all
}

func slugOf(t *testing.T, db *store.Store, id string) string {
	t.Helper()
	row, err := db.GetSession(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return row.Slug
}

// inertPane starts a quiet tmux session for the target's slug and points the
// model at it.
func inertPane(t *testing.T, m Model, db *store.Store, target string) Model {
	t.Helper()
	slug := slugOf(t, db, target)
	socket := selectionTestSocket("inert")
	name, err := tmux.SessionName(slug)
	if err != nil {
		t.Fatal(err)
	}
	newQuietSelectionPane(t, socket, name, 80, 24)
	client := tmux.Client{Socket: socket}
	m.tmuxClient = client
	m.previewCapture = client.CapturePreview
	if _, h := m.previewContentSize(); h < interactiveMinInnerRows {
		t.Fatalf("preview content height %d is below the %d-row floor", h, interactiveMinInnerRows)
	}
	return m
}

// TestAttachIsInertOnStatusSeenAndPosition: the eight attach cases. Each
// does a full attach (`a`, then its detach) and an interactive entry/exit on
// the row and asserts status, seen state and position are unchanged.
func TestAttachIsInertOnStatusSeenAndPosition(t *testing.T) {
	for _, order := range []string{SortOrderAttention, SortOrderActivity} {
		for _, row := range inertRows {
			t.Run(order+"/"+row.status, func(t *testing.T) {
				m, db := inertFixture(t, order, row.id, false)
				m = inertPane(t, m, db, row.id)
				before := snapshotAll(t, db)
				orderBefore := inertOrder(m)
				if got := before[row.id].Session.Status; got != row.status {
					t.Fatalf("fixture: %s status = %q, want %q", row.id, got, row.status)
				}
				if row.status == "waiting" || row.status == "error" {
					if before[row.id].Session.StatusSeen() {
						t.Fatalf("fixture: %s row must start unseen", row.id)
					}
				}

				updated, cmd := m.Update(key("a"))
				if cmd == nil {
					t.Fatal("a did not schedule the attach")
				}
				m = updated.(Model)
				assertInert(t, "full attach", m, db, orderBefore, before)
				updated, _ = m.Update(attachFinished{})
				m = updated.(Model)
				assertInert(t, "full attach detach", m, db, orderBefore, before)

				next, _ := m.enterInteractive()
				m = next.(Model)
				if !m.interactive {
					t.Fatalf("interactive entry refused: %q %+v", m.attachError, m.entryRefusal)
				}
				assertInert(t, "interactive entry", m, db, orderBefore, before)
				m.exitInteractive()
				assertInert(t, "interactive exit", m, db, orderBefore, before)
			})
		}
	}
}

// TestClickIsInertOnStatusSeenAndPosition: a sidebar click (which selects and,
// with attach_on_click, enters the interactive preview) leaves every row's
// durable state and position alone.
func TestClickIsInertOnStatusSeenAndPosition(t *testing.T) {
	m, db := inertFixture(t, SortOrderAttention, "row-idle", true)
	m = inertPane(t, m, db, "row-waiting")
	before := snapshotAll(t, db)
	orderBefore := inertOrder(m)
	index := -1
	for i, id := range orderBefore {
		if id == "row-waiting" {
			index = i
		}
	}
	x, y := findRow(t, m, index)
	updated, _ := m.Update(press(x, y))
	m = updated.(Model)
	if m.selected != rowCursor(index) {
		t.Fatalf("click selected %v, want row %d", m.selected, index)
	}
	if !m.interactive {
		t.Fatalf("click did not enter interactive mode: %q %+v", m.attachError, m.entryRefusal)
	}
	assertInert(t, "click", m, db, orderBefore, before)
	updated, _ = m.Update(release(x, y))
	m = updated.(Model)
	m.exitInteractive()
	assertInert(t, "click release and exit", m, db, orderBefore, before)
}

// TestPassivePreviewIsInertOnStatusSeenAndPosition: ticking and capturing the
// passive preview of an unseen waiting row writes nothing and moves nothing.
func TestPassivePreviewIsInertOnStatusSeenAndPosition(t *testing.T) {
	m, db := inertFixture(t, SortOrderActivity, "row-waiting", false)
	m = inertPane(t, m, db, "row-waiting")
	before := snapshotAll(t, db)
	orderBefore := inertOrder(m)
	for i := 0; i < 3; i++ {
		updated, _ := m.Update(previewTick(time.Now()))
		m = updated.(Model)
		if cmd := m.capturePreview(); cmd != nil {
			updated, _ = m.Update(cmd())
			m = updated.(Model)
		}
	}
	if m.interactive || strings.TrimSpace(m.attachError) != "" {
		t.Fatalf("passive preview changed mode: interactive=%v attachError=%q", m.interactive, m.attachError)
	}
	assertInert(t, "passive preview", m, db, orderBefore, before)
}
