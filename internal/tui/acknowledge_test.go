package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

func TestYAcknowledgesOnlySelectedRowDurably(t *testing.T) {
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	for index, row := range []struct {
		id, name, status string
	}{
		{"other", "other row", "error"},
		{"selected", "selected row", "waiting"},
	} {
		at := int64(100 + index)
		if _, err := db.CreateSession(ctx, store.CreateSessionInput{
			ID: row.id, Name: row.name, CWD: "/work/" + row.id, Agent: "claude", CapturedPath: "/bin",
			Status: "running", StatusSource: "hook", StatusAt: at, CreatedAt: at,
		}); err != nil {
			t.Fatal(err)
		}
		if err := db.UpdateSessionStatus(ctx, store.StatusUpdateInput{
			SessionID: row.id, Status: row.status, Reason: "needs attention", Source: "hook", At: at + 10,
		}); err != nil {
			t.Fatal(err)
		}
	}

	settings := config.Settings{ASCII: true}
	model := New(db, settings, "")
	loaded := model.loadSessions().(sessionsLoaded)
	updated, _ := model.Update(loaded)
	model = updated.(Model)
	if len(model.sessions) != 2 {
		t.Fatalf("loaded %d sessions, want 2", len(model.sessions))
	}
	for i, session := range model.sessions {
		if session.ID == "selected" {
			model.selected = i
		}
	}
	if got := strings.Count(model.View(), "!"); got != 2 {
		t.Fatalf("initial unseen marker count = %d, want 2:\n%s", got, model.View())
	}

	updated, cmd := model.Update(key("Y"))
	if cmd == nil {
		t.Fatal("Y did not schedule acknowledgement")
	}
	model = updated.(Model)
	updated, load := model.Update(cmd())
	model = updated.(Model)
	if load == nil {
		t.Fatal("successful acknowledgement did not schedule a durable reload")
	}
	updated, _ = model.Update(load())
	model = updated.(Model)

	selected, err := db.GetSession(ctx, "selected")
	if err != nil {
		t.Fatal(err)
	}
	if !selected.Acknowledged || selected.Status != "waiting" || selected.StatusSource != "hook" || selected.StatusAt != 111 || selected.NotifyEpoch != 0 {
		t.Fatalf("selected row acknowledgement changed verdict fields: %#v", selected)
	}
	other, err := db.GetSession(ctx, "other")
	if err != nil {
		t.Fatal(err)
	}
	if other.Acknowledged || other.Status != "error" {
		t.Fatalf("Y changed unrelated row: %#v", other)
	}
	if got := strings.Count(model.View(), "!"); got != 1 {
		t.Fatalf("post-Y unseen marker count = %d, want 1:\n%s", got, model.View())
	}

	// Constructing a fresh model simulates a deck restart: the cleared marker
	// must come from SQLite rather than transient model state.
	restarted := New(db, settings, "")
	updated, _ = restarted.Update(restarted.loadSessions())
	restarted = updated.(Model)
	if got := strings.Count(restarted.View(), "!"); got != 1 {
		t.Fatalf("restart unseen marker count = %d, want 1:\n%s", got, restarted.View())
	}
}
