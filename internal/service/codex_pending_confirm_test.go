package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// titledCodexPane is the codex pane recorder that also reports a pane title.
type titledCodexPane struct {
	*codexPane
	title string
}

func (p *titledCodexPane) PaneTitle(context.Context, string) (string, error) { return p.title, nil }

// pendingCodexRig is a codex row with no conversation id (Codex mints it on
// the first prompt) holding a pending pushed name, and its pane.
func pendingCodexRig(t *testing.T, pending string, ageMillis int64) (Service, *store.Store, *titledCodexPane, store.Session, string) {
	t.Helper()
	home := t.TempDir()
	svc, db, _, _ := newAgentTestService(t, map[string]string{"CODEX_HOME": home}, "codex-pending")
	pane := &titledCodexPane{codexPane: &codexPane{t: t}}
	svc.NamePane, svc.Pause = pane, pane.pause
	row := codexRow(t, db, "p1", "idle", "user")
	if err := db.SetPendingHarnessName(context.Background(), row.ID, pending, svc.Clock.Now().UnixMilli()-ageMillis); err != nil {
		t.Fatal(err)
	}
	return svc, db, pane, row, home
}

func tick(t *testing.T, svc Service, db *store.Store, id string) store.Session {
	t.Helper()
	svc.adoptOnTick(context.Background(), mustGet(t, db, id))
	return mustGet(t, db, id)
}

func assertSettled(t *testing.T, db *store.Store, pane *titledCodexPane, got store.Session) {
	t.Helper()
	if got.PendingHarnessName != "" || got.PendingHarnessNameAt != 0 {
		t.Fatalf("pending not cleared: %q at %d", got.PendingHarnessName, got.PendingHarnessNameAt)
	}
	if n := len(eventReasons(t, db, got.ID, EventHarnessRenameFailed)); n != 0 {
		t.Fatalf("a confirmed name emitted %d failed events", n)
	}
	if len(pane.log) != 0 {
		t.Fatalf("a confirmation sent keys: %v", pane.log)
	}
}

// TestCodexPendingNameConfirmedFromPaneTitleBeforeFirstPrompt (R252/R257):
// Codex titles its pane with the name before any thread (and so any
// conversation id) exists; that title confirms the pending startup name.
func TestCodexPendingNameConfirmedFromPaneTitleBeforeFirstPrompt(t *testing.T) {
	svc, db, pane, row, _ := pendingCodexRig(t, "startup name", 1000)
	if row.ConversationID != "" {
		t.Fatalf("fixture row has an id: %q", row.ConversationID)
	}
	pane.title = "startup name | " + filepath.Base(row.CWD)
	got := tick(t, svc, db, row.ID)
	assertSettled(t, db, pane, got)
	if got.Name != row.Name || got.NameSource != "user" {
		t.Fatalf("confirmation changed the row: %q (%s)", got.Name, got.NameSource)
	}
}

// TestCodexPendingNameConfirmationVariants: the same rule with a working
// spinner glyph in the title, a name Codex's sanitiser shortens, and the hook
// path (AdoptNameWith) instead of the tick.
func TestCodexPendingNameConfirmationVariants(t *testing.T) {
	t.Run("spinner glyph in front of the title", func(t *testing.T) {
		svc, db, pane, row, _ := pendingCodexRig(t, "startup name", 1000)
		pane.title = "⠋ startup name | " + filepath.Base(row.CWD)
		assertSettled(t, db, pane, tick(t, svc, db, row.ID))
	})
	t.Run("a pushed name with surrounding space is compared sanitised", func(t *testing.T) {
		svc, db, pane, row, _ := pendingCodexRig(t, "two words", 1000)
		pane.title = "  two   words | " + filepath.Base(row.CWD)
		assertSettled(t, db, pane, tick(t, svc, db, row.ID))
	})
	t.Run("a hook carrying no hint settles through the same read", func(t *testing.T) {
		svc, db, pane, row, _ := pendingCodexRig(t, "startup name", 1000)
		pane.title = "startup name | " + filepath.Base(row.CWD)
		if _, err := svc.AdoptNameWith(context.Background(), row.ID, NameHint{}); err != nil {
			t.Fatal(err)
		}
		assertSettled(t, db, pane, mustGet(t, db, row.ID))
	})
}

// TestCodexPendingNameUnconfirmedBeforeFirstPrompt: a title that is not the
// pending name (or not this session's directory) never confirms; inside the
// timeout the name stays pending, past it exactly one failed event is
// recorded, the field clears and no keystrokes are sent again.
func TestCodexPendingNameUnconfirmedBeforeFirstPrompt(t *testing.T) {
	t.Run("another name stays pending inside the timeout", func(t *testing.T) {
		svc, db, pane, row, _ := pendingCodexRig(t, "startup name", 1000)
		pane.title = "other name | " + filepath.Base(row.CWD)
		if got := tick(t, svc, db, row.ID); got.PendingHarnessName != "startup name" {
			t.Fatalf("pending = %q", got.PendingHarnessName)
		}
	})
	t.Run("the right name for another directory is not this session's", func(t *testing.T) {
		svc, db, pane, row, _ := pendingCodexRig(t, "startup name", 1000)
		pane.title = "startup name | elsewhere"
		if got := tick(t, svc, db, row.ID); got.PendingHarnessName != "startup name" {
			t.Fatalf("pending = %q", got.PendingHarnessName)
		}
	})
	t.Run("timeout fails once and never retries", func(t *testing.T) {
		svc, db, pane, row, _ := pendingCodexRig(t, "startup name", pushConfirmTimeout.Milliseconds()+1)
		pane.title = "host-name"
		for range 3 {
			tick(t, svc, db, row.ID)
		}
		got := mustGet(t, db, row.ID)
		if got.PendingHarnessName != "" || got.PendingHarnessNameAt != 0 {
			t.Fatalf("pending not cleared: %q", got.PendingHarnessName)
		}
		if n := len(eventReasons(t, db, row.ID, EventHarnessRenameFailed)); n != 1 {
			t.Fatalf("failed events = %d, want 1", n)
		}
		if len(pane.log) != 0 {
			t.Fatalf("retried: %v", pane.log)
		}
	})
}

// TestCodexPendingNameDatabaseReadStaysGatedOnABoundID: the thread database
// needs a valid bound conversation id. With none, a threads.name equal to the
// pending name (some other thread's) must not confirm it; with the id bound,
// the same database row does.
func TestCodexPendingNameDatabaseReadStaysGatedOnABoundID(t *testing.T) {
	t.Run("no id: another thread's name confirms nothing", func(t *testing.T) {
		svc, db, pane, row, home := pendingCodexRig(t, "startup name", 1000)
		setCodexThreadName(t, home, "someone-elses-thread", "startup name")
		pane.title = ""
		if got := tick(t, svc, db, row.ID); got.PendingHarnessName != "startup name" {
			t.Fatalf("an unbound row was confirmed from the database: %q", got.PendingHarnessName)
		}
	})
	t.Run("an unsafe id is not read either", func(t *testing.T) {
		svc, db, pane, row, home := pendingCodexRig(t, "startup name", 1000)
		setCodexThreadName(t, home, "../x", "startup name")
		if _, err := db.DB().Exec(`UPDATE sessions SET conversation_id = ? WHERE id = ?`, "../x", row.ID); err != nil {
			t.Fatal(err)
		}
		pane.title = ""
		if got := tick(t, svc, db, row.ID); got.PendingHarnessName != "startup name" {
			t.Fatalf("an unsafe id was read: %q", got.PendingHarnessName)
		}
	})
	t.Run("bound id: the database row confirms", func(t *testing.T) {
		svc, db, pane, row, home := pendingCodexRig(t, "startup name", 1000)
		setCodexThreadName(t, home, "conv-p1", "startup name")
		if _, err := db.DB().Exec(`UPDATE sessions SET conversation_id = ? WHERE id = ?`, "conv-p1", row.ID); err != nil {
			t.Fatal(err)
		}
		assertSettled(t, db, pane, tick(t, svc, db, row.ID))
	})
}

// TestPendingNameOfIDReadingAdapterNeedsABoundID: Claude reads its transcript
// by conversation id; with no id the pending name is not confirmed by anything.
func TestPendingNameOfIDReadingAdapterNeedsABoundID(t *testing.T) {
	svc, db, _, _ := newAgentTestService(t, nil, "claude-pending")
	pane := &titledCodexPane{codexPane: &codexPane{t: t}, title: "startup name | x"}
	svc.NamePane = pane
	row, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "cl1", Name: "n", CWD: t.TempDir(), Agent: "claude", CapturedPath: "/bin",
		Status: "idle", StatusSource: "hook", StatusAt: 1, CreatedAt: 1, NameSource: "user",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetPendingHarnessName(context.Background(), row.ID, "startup name", svc.Clock.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if got := tick(t, svc, db, row.ID); got.PendingHarnessName != "startup name" {
		t.Fatalf("pending = %q", got.PendingHarnessName)
	}
}
