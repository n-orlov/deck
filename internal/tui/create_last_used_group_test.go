package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// openStoreForLastCreateGroup mirrors openStoreForLastCreateAgent
// (create_last_used_agent_test.go) for the Group field's own set of tests.
func openStoreForLastCreateGroup(t *testing.T) *store.Store {
	t.Helper()
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// groupFieldRow returns createFieldRows' Group row (the last row, task 016
// appended it there), failing outright if that ever drifts so a
// mislabelled assertion below never silently passes against the wrong
// field.
func groupFieldRow(t *testing.T, m Model) struct{ label, value, help string } {
	t.Helper()
	rows := m.createFieldRows()
	last := len(rows) - 1
	if last < 0 || rows[last].label != "Group" {
		t.Fatalf("createFieldRows()'s last row is not the Group row: %+v", rows)
	}
	return rows[last]
}

// TestCreateGroupDefaultDegradesWhenRememberedGroupDeleted covers R130's
// explicit precedent from GetLastCreateAgent/lastCreateAgentUIStateKey,
// "including its fallback when the remembered value is gone": a deleted
// group must degrade the create modal's Group field to "default", never to
// an empty field. It exercises the degrade two ways -- through
// pickCreateGroup directly, and through the full "n" handler + rendered
// Group row -- so a regression that only broke one of the two call sites
// still fails this test.
func TestCreateGroupDefaultDegradesWhenRememberedGroupDeleted(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	ctx := context.Background()

	g, err := db.CreateGroup(ctx, "tooling")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetLastCreateGroup(ctx, g.ID); err != nil {
		t.Fatal(err)
	}

	// Sanity: before the delete, a fresh model actually remembers it.
	before := New(db, config.Settings{}, "")
	if before.lastCreateGroupID != g.ID {
		t.Fatalf("New() did not read the persisted last-create-group: got %d, want %d", before.lastCreateGroupID, g.ID)
	}

	if err := db.DeleteGroup(ctx, g.ID); err != nil {
		t.Fatal(err)
	}

	// GetLastCreateGroup itself already degrades a deleted group's id to
	// nil at read time (store-level check), so a fresh New() must not
	// even carry the dangling id in memory.
	after := New(db, config.Settings{}, "")
	if after.lastCreateGroupID != 0 {
		t.Fatalf("New() carried a deleted group's id into lastCreateGroupID: got %d, want 0 (default)", after.lastCreateGroupID)
	}

	updated, _ := after.Update(key("n"))
	m := updated.(Model)

	id, lastUsed := m.pickCreateGroup()
	if id != 0 {
		t.Fatalf("pickCreateGroup() = %d after its remembered group was deleted, want 0 (default)", id)
	}
	if lastUsed {
		t.Fatal("pickCreateGroup() reported lastUsed=true for a deleted group")
	}
	if m.createGroupID != 0 {
		t.Fatalf("createGroupID = %d after opening on a deleted remembered group, want 0 (default)", m.createGroupID)
	}
	if got := m.createGroupName(m.createGroupID); got != "default" {
		t.Fatalf("createGroupName(createGroupID) = %q, want %q -- never an empty field", got, "default")
	}

	row := groupFieldRow(t, m)
	if strings.TrimSpace(row.value) == "" || !strings.HasPrefix(row.value, "default ") {
		t.Fatalf("Group row value = %q, want it to open on %q, not blank", row.value, "default")
	}
	if strings.Contains(row.help, "(last used)") {
		t.Fatalf("Group row help = %q, want no \"(last used)\" label once the remembered group is gone", row.help)
	}

	view := m.createBody()
	if !strings.Contains(view, "Group: default ") {
		t.Fatalf("create modal body does not show the Group field defaulting to \"default\":\n%s", view)
	}
}

// TestRememberedCreateGroupSurvivesRestart proves R130's other half of the
// same precedent: a group a create actually succeeded into is still
// remembered after a full restart (a fresh New(db, ...) call, exactly as
// TestCreateModalOpensOnLastCreateAgentLabelled proves for the Agent
// field) -- not merely within the same running model.
func TestRememberedCreateGroupSurvivesRestart(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	ctx := context.Background()

	g, err := db.CreateGroup(ctx, "sprint work")
	if err != nil {
		t.Fatal(err)
	}

	m := New(db, config.Settings{}, "")
	updated, _ := m.Update(key("n"))
	m = updated.(Model)
	m.createGroups = []store.Group{g}
	m.createField = createFieldCount - 1
	m.createGroupID = g.ID

	updated, cmd := m.Update(shellCreated{session: store.Session{ID: "sess-1", Agent: "shell", GroupID: &g.ID}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("shellCreated success did not return a command")
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, sub := range batch {
			updated, _ = m.Update(sub())
			m = updated.(Model)
		}
	} else {
		updated, _ = m.Update(msg)
		m = updated.(Model)
	}

	if m.lastCreateGroupID != g.ID {
		t.Fatalf("model.lastCreateGroupID = %d after a successful create into a group, want %d", m.lastCreateGroupID, g.ID)
	}
	persisted, err := db.GetLastCreateGroup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if persisted == nil || *persisted != g.ID {
		t.Fatalf("ui_state last_create_group = %v after a successful create, want %d", persisted, g.ID)
	}

	// The actual restart: a brand new Model built from the same store, as
	// if the process had exited and relaunched.
	restarted := New(db, config.Settings{}, "")
	if restarted.lastCreateGroupID != g.ID {
		t.Fatalf("restarted model's lastCreateGroupID = %d, want %d (survived the restart)", restarted.lastCreateGroupID, g.ID)
	}

	updated, _ = restarted.Update(key("n"))
	restarted = updated.(Model)
	if restarted.createGroupID != g.ID {
		t.Fatalf("restarted create modal opened on Group id %d, want %d", restarted.createGroupID, g.ID)
	}
	if !restarted.createGroupLastUsed {
		t.Fatal("restarted create modal did not label Group as last used")
	}
	row := groupFieldRow(t, restarted)
	if !strings.Contains(row.help, "(last used)") {
		t.Fatalf("Group row help = %q after restart, want it to contain %q", row.help, "(last used)")
	}
}
