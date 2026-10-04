package tui

import (
	"context"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// R176 (GH #53): Y is offered and acted on only for a row that still has an
// unseen marker to clear. The footer's Y slot and the Y key handler both
// consult canAcknowledge, so these tests drive the footer legend and the
// key through the public Model seams.

func ackOfferFooter(m Model) string { return m.footerKeyLegend() }

func ackOfferModel(sessions []store.Session, selected sidebarCursor) Model {
	m := New(nil, config.Settings{}, "")
	m.sessions = sessions
	m.selected = selected
	return m
}

func TestFooterOffersYOnlyForUnacknowledgedRows(t *testing.T) {
	t.Run("acknowledged waiting row omits Y", func(t *testing.T) {
		m := ackOfferModel([]store.Session{{ID: "s1", Name: "a", Slug: "a", Agent: "claude", Status: "waiting", Acknowledged: true}}, rowCursor(0))
		if legend := ackOfferFooter(m); strings.Contains(legend, "Y acknowledge") {
			t.Fatalf("acknowledged row: footer must not offer Y: %q", legend)
		}
	})
	t.Run("acknowledged running row omits Y", func(t *testing.T) {
		m := ackOfferModel([]store.Session{{ID: "s1", Name: "a", Slug: "a", Agent: "claude", Status: "running", Acknowledged: true}}, rowCursor(0))
		if legend := ackOfferFooter(m); strings.Contains(legend, "Y acknowledge") {
			t.Fatalf("acknowledged running row: footer must not offer Y: %q", legend)
		}
	})
	t.Run("group header omits Y", func(t *testing.T) {
		m := ackOfferModel([]store.Session{{ID: "s1", Name: "a", Slug: "a", Agent: "claude", Status: "waiting", Acknowledged: false}}, headerCursor(0))
		if legend := ackOfferFooter(m); strings.Contains(legend, "Y acknowledge") {
			t.Fatalf("group header: footer must not offer Y: %q", legend)
		}
	})
	t.Run("unacknowledged waiting row shows Y", func(t *testing.T) {
		m := ackOfferModel([]store.Session{{ID: "s1", Name: "a", Slug: "a", Agent: "claude", Status: "waiting"}}, rowCursor(0))
		if legend := ackOfferFooter(m); !strings.Contains(legend, "Y acknowledge") {
			t.Fatalf("unacknowledged waiting row: footer must offer Y: %q", legend)
		}
	})
	t.Run("unacknowledged error row shows Y", func(t *testing.T) {
		m := ackOfferModel([]store.Session{{ID: "s1", Name: "a", Slug: "a", Agent: "claude", Status: "error"}}, rowCursor(0))
		if legend := ackOfferFooter(m); !strings.Contains(legend, "Y acknowledge") {
			t.Fatalf("unacknowledged error row: footer must offer Y: %q", legend)
		}
	})
}

// ackOfferStoreModel builds a model over a real store holding one waiting row
// that has not been acknowledged yet.
func ackOfferStoreModel(t *testing.T) (Model, *store.Store) {
	t.Helper()
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if _, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: "waiting", Name: "waiting", CWD: "/work", Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 100, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID: "waiting", Status: "waiting", Reason: "permission_prompt", Source: "hook", At: 110,
	}); err != nil {
		t.Fatal(err)
	}
	clock, err := config.NewClock("2025-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
	m := NewWithShellCreatorAndAttacher(db, config.Settings{ASCII: true, Clock: clock}, "", nil,
		func(context.Context, string) (*exec.Cmd, error) { return exec.Command("true"), nil })
	updated, _ := m.Update(m.loadSessions())
	return updated.(Model), db
}

func TestFooterDropsYOnceAttachAcknowledgesTheRow(t *testing.T) {
	m, _ := ackOfferStoreModel(t)
	if legend := ackOfferFooter(m); !strings.Contains(legend, "Y acknowledge") {
		t.Fatalf("before attach: footer must offer Y: %q", legend)
	}
	updated, cmd := m.Update(key("a"))
	if cmd == nil {
		t.Fatal("a did not schedule attachment")
	}
	m = updated.(Model)
	updated, _ = m.Update(m.loadSessions())
	m = updated.(Model)
	if legend := ackOfferFooter(m); strings.Contains(legend, "Y acknowledge") {
		t.Fatalf("after attach acknowledged the row: footer must drop Y: %q", legend)
	}
}

func TestFooterDropsYOnceYAcknowledgesTheRow(t *testing.T) {
	m, _ := ackOfferStoreModel(t)
	if legend := ackOfferFooter(m); !strings.Contains(legend, "Y acknowledge") {
		t.Fatalf("before Y: footer must offer Y: %q", legend)
	}
	updated, cmd := m.Update(key("Y"))
	if cmd == nil {
		t.Fatal("Y on an unacknowledged row did not schedule acknowledgement")
	}
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	updated, _ = m.Update(m.loadSessions())
	m = updated.(Model)
	if legend := ackOfferFooter(m); strings.Contains(legend, "Y acknowledge") {
		t.Fatalf("after Y acknowledged the row: footer must drop Y: %q", legend)
	}
}

func TestYOnAcknowledgedRowDoesNothingAndStaysInHelp(t *testing.T) {
	m, db := ackOfferStoreModel(t)
	ctx := context.Background()
	if err := db.AcknowledgeSession(ctx, "waiting"); err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(m.loadSessions())
	m = updated.(Model)
	if !m.sessions[0].Acknowledged {
		t.Fatalf("test assumption violated: row is not acknowledged: %#v", m.sessions[0])
	}
	calls := 0
	realAck := m.acknowledge
	m.acknowledge = func(c context.Context, id string) error {
		calls++
		return realAck(c, id)
	}
	before, err := db.GetSession(ctx, "waiting")
	if err != nil {
		t.Fatal(err)
	}
	eventCount := func() int {
		var n int
		if err := db.DB().QueryRow(`SELECT count(*) FROM events`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	eventsBefore := eventCount()

	next, cmd := m.Update(key("Y"))
	if cmd != nil {
		// A command would be the only route to a store write or a message;
		// run it anyway so a wrongly scheduled one is caught on its effects.
		t.Errorf("Y on an acknowledged row scheduled a command, want none; it yielded %#v", cmd())
	}
	if calls != 0 {
		t.Errorf("acknowledge calls = %d, want 0", calls)
	}
	after, err := db.GetSession(ctx, "waiting")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Errorf("store row changed on Y: before %#v after %#v", before, after)
	}
	if got := eventCount(); got != eventsBefore {
		t.Errorf("events = %d, want %d (no event)", got, eventsBefore)
	}
	got := next.(Model)
	if got.attachError != "" {
		t.Errorf("Y on an acknowledged row set a message: %q", got.attachError)
	}

	got.help = true
	got.width, got.height = 100, 400
	if view := got.View(); !strings.Contains(view, "Y acknowledge") {
		t.Errorf("the ? overlay must still list Y:\n%s", view)
	}
}
