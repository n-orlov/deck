package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// codexAdoptRow creates a codex row bound to conv-<id>.
func codexAdoptRow(t *testing.T, db *store.Store, id, name, source string) store.Session {
	t.Helper()
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: id, Name: name, CWD: t.TempDir(), Agent: "codex", CapturedPath: "/bin",
		Status: "idle", StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
		ConversationID: "conv-" + id, NameSource: source,
	})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func setCodexThreadName(t *testing.T, home, id string, name any) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(home, "state_5.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS threads (id TEXT PRIMARY KEY, name TEXT)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO threads (id, name) VALUES (?, ?) ON CONFLICT(id) DO UPDATE SET name = excluded.name`, id, name); err != nil {
		t.Fatal(err)
	}
}

// TestCodexReadNameAdoptsGeneratedThenFreezesOnChange drives the heuristic of
// SPEC §8.2 through the service: the first threads.name is Codex's generated
// title (harness), a later different one was typed by a human (user, frozen).
func TestCodexReadNameAdoptsGeneratedThenFreezesOnChange(t *testing.T) {
	codexHome := t.TempDir()
	svc, db, _, _ := newAgentTestService(t, map[string]string{"CODEX_HOME": codexHome}, "codex-adopt")
	row := codexAdoptRow(t, db, "row-c", "swift-otter", "derived")

	setCodexThreadName(t, codexHome, row.ConversationID, nil)
	if renamed, err := svc.AdoptName(context.Background(), row.ID); err != nil || renamed {
		t.Fatalf("NULL name: renamed=%v err=%v, want untouched", renamed, err)
	}

	setCodexThreadName(t, codexHome, row.ConversationID, "Write flaky test haiku")
	if renamed, err := svc.AdoptName(context.Background(), row.ID); err != nil || !renamed {
		t.Fatalf("generated title: renamed=%v err=%v", renamed, err)
	}
	got, _ := db.GetSession(context.Background(), row.ID)
	if got.Name != "Write flaky test haiku" || got.NameSource != "harness" {
		t.Fatalf("after the generated title = (%q, %q), want the title as harness", got.Name, got.NameSource)
	}

	setCodexThreadName(t, codexHome, row.ConversationID, "release prep")
	if renamed, err := svc.AdoptName(context.Background(), row.ID); err != nil || !renamed {
		t.Fatalf("changed title: renamed=%v err=%v", renamed, err)
	}
	got, _ = db.GetSession(context.Background(), row.ID)
	if got.Name != "release prep" || got.NameSource != "user" {
		t.Fatalf("after a change = (%q, %q), want release prep as user (frozen)", got.Name, got.NameSource)
	}

	setCodexThreadName(t, codexHome, row.ConversationID, "something else")
	if renamed, _ := svc.AdoptName(context.Background(), row.ID); renamed {
		t.Fatal("a frozen user row followed Codex again")
	}
}
