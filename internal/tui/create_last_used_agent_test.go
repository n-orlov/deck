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

// openStoreForLastCreateAgent opens a fresh state.db for these tests,
// mirroring layout_persistence_test.go's own setup.
func openStoreForLastCreateAgent(t *testing.T) *store.Store {
	t.Helper()
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// agentFieldRow returns createFieldRows' Agent row (index 2), failing the
// test outright if that ever drifts so a mislabelled assertion below never
// silently passes against the wrong field.
func agentFieldRow(t *testing.T, m Model) struct{ label, value, help string } {
	t.Helper()
	rows := m.createFieldRows()
	if len(rows) < 3 || rows[2].label != "Agent" {
		t.Fatalf("createFieldRows()[2] is not the Agent row: %+v", rows)
	}
	return rows[2]
}

// TestPickCreateAgentDegradesToDefaultOnMissingUnregisteredValue covers
// task 024's validation contract directly: pickCreateAgent must fall back
// to defaultCreateAgent whenever m.lastCreateAgent is empty (never
// persisted) or no longer present in m.registry().Kinds() (unregistered),
// and must use it -- reporting so via the second return -- only when it is
// both non-empty and currently registered.
func TestPickCreateAgentDegradesToDefaultOnMissingUnregisteredValue(t *testing.T) {
	db := openStoreForLastCreateAgent(t)
	ctx := context.Background()

	// Missing: no create has ever succeeded.
	fresh := New(db, config.Settings{}, "")
	kinds := fresh.registry().Kinds()
	agent, lastUsed := fresh.pickCreateAgent()
	if lastUsed {
		t.Fatalf("pickCreateAgent reported lastUsed=true with nothing persisted")
	}
	if agent != defaultCreateAgent(kinds) {
		t.Fatalf("pickCreateAgent() = %q, want defaultCreateAgent's fallback %q", agent, defaultCreateAgent(kinds))
	}

	// Unregistered: a value is persisted, but it names no adapter the
	// current registry knows about.
	if err := db.SetLastCreateAgent(ctx, "totally-unregistered-kind"); err != nil {
		t.Fatal(err)
	}
	stale := New(db, config.Settings{}, "")
	agent, lastUsed = stale.pickCreateAgent()
	if lastUsed {
		t.Fatalf("pickCreateAgent reported lastUsed=true for an unregistered persisted value")
	}
	if agent != defaultCreateAgent(kinds) {
		t.Fatalf("pickCreateAgent() with unregistered persisted value = %q, want fallback %q", agent, defaultCreateAgent(kinds))
	}

	// Valid: a value is persisted and is present in Kinds().
	if err := db.SetLastCreateAgent(ctx, "claude"); err != nil {
		t.Fatal(err)
	}
	valid := New(db, config.Settings{}, "")
	agent, lastUsed = valid.pickCreateAgent()
	if !lastUsed {
		t.Fatalf("pickCreateAgent reported lastUsed=false for a still-registered persisted value")
	}
	if agent != "claude" {
		t.Fatalf("pickCreateAgent() = %q, want %q", agent, "claude")
	}
}

// TestCreateModalOpensOnLastCreateAgentLabelled covers the "n" handler end
// to end: it must call pickCreateAgent (not defaultCreateAgent directly),
// pre-select the remembered agent, label the Agent field "(last used)" in
// the dialog, and re-derive createProfile through m.defaultCreateProfile --
// the SAME derivation the left/right cycle case (cycleCreateField's case 2)
// uses -- rather than leaving whatever createProfile held before "n".
func TestCreateModalOpensOnLastCreateAgentLabelled(t *testing.T) {
	db := openStoreForLastCreateAgent(t)
	if err := db.SetLastCreateAgent(context.Background(), "claude"); err != nil {
		t.Fatal(err)
	}

	m := New(db, config.Settings{}, "")
	// Sanity: New must have read the persisted value into model state
	// at store-open time (task 024), not deferred to first render.
	if m.lastCreateAgent != "claude" {
		t.Fatalf("New() did not read the persisted last-create-agent: got %q", m.lastCreateAgent)
	}

	updated, _ := m.Update(key("n"))
	m = updated.(Model)

	if m.createAgent != "claude" {
		t.Fatalf("opening the create modal chose Agent = %q, want %q", m.createAgent, "claude")
	}
	if !m.createAgentLastUsed {
		t.Fatal("createAgentLastUsed is false after opening on a valid persisted value")
	}
	if want := m.defaultCreateProfile("claude"); m.createProfile != want {
		t.Fatalf("createProfile = %q after opening on the last-used agent, want the same derivation the cycle case uses (%q)", m.createProfile, want)
	}
	if m.createProfileTouched {
		t.Fatal("createProfileTouched is true immediately after opening the modal")
	}

	row := agentFieldRow(t, m)
	if !strings.Contains(row.help, "(last used)") {
		t.Fatalf("Agent row help = %q, want it to contain %q", row.help, "(last used)")
	}

	view := m.createView()
	if !strings.Contains(view, "(last used)") {
		t.Fatalf("create modal view does not contain the \"(last used)\" label:\n%s", view)
	}
}

// TestCreateModalOpensWithoutLabelWhenNoLastCreateAgent is
// TestCreateModalOpensOnLastCreateAgentLabelled's negative twin: a fresh
// store (nothing persisted yet) must not show the Agent field's
// "(last used)" label at all.
func TestCreateModalOpensWithoutLabelWhenNoLastCreateAgent(t *testing.T) {
	db := openStoreForLastCreateAgent(t)
	m := New(db, config.Settings{}, "")

	updated, _ := m.Update(key("n"))
	m = updated.(Model)

	if m.createAgentLastUsed {
		t.Fatal("createAgentLastUsed is true with nothing ever persisted")
	}
	row := agentFieldRow(t, m)
	if strings.Contains(row.help, "(last used)") {
		t.Fatalf("Agent row help = %q, want no \"(last used)\" label with nothing persisted", row.help)
	}
}

// TestCyclingAgentFieldClearsLastUsedLabel proves the label is tied to the
// untouched remembered value, exactly like the cwd field's own
// createCWDLastUsed: the moment the Agent field is cycled away, the value
// shown is a deliberate choice, and the label must not linger.
func TestCyclingAgentFieldClearsLastUsedLabel(t *testing.T) {
	db := openStoreForLastCreateAgent(t)
	if err := db.SetLastCreateAgent(context.Background(), "claude"); err != nil {
		t.Fatal(err)
	}
	m := New(db, config.Settings{}, "")
	updated, _ := m.Update(key("n"))
	m = updated.(Model)
	if !m.createAgentLastUsed {
		t.Fatal("setup: createAgentLastUsed should be true before cycling")
	}

	m.createField = 2
	m.cycleCreateField(1)

	if m.createAgentLastUsed {
		t.Fatal("createAgentLastUsed is still true after cycling the Agent field")
	}
	row := agentFieldRow(t, m)
	if strings.Contains(row.help, "(last used)") {
		t.Fatalf("Agent row help = %q, want no label after cycling", row.help)
	}
}

// TestCycledThenAbandonedCreateModalDoesNotPersistLastCreateAgent is task
// 024's central write-gate assertion: cycling the Agent field and then
// abandoning the dialog with esc must change nothing in ui_state -- the
// value is written only on a SUCCESSFUL create, never merely on having
// shown a different value in the field.
func TestCycledThenAbandonedCreateModalDoesNotPersistLastCreateAgent(t *testing.T) {
	db := openStoreForLastCreateAgent(t)
	ctx := context.Background()

	before, err := db.GetLastCreateAgent(ctx)
	if err != nil {
		t.Fatal(err)
	}

	m := New(db, config.Settings{}, "")
	updated, _ := m.Update(key("n"))
	m = updated.(Model)
	m.createField = 2
	m.cycleCreateField(1)
	m.cycleCreateField(1)

	updated, _ = m.Update(key("esc"))
	m = updated.(Model)
	if m.creating {
		t.Fatal("esc did not close the create modal")
	}

	after, err := db.GetLastCreateAgent(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("ui_state last_create_agent changed from %q to %q after a cycled-then-abandoned dialog", before, after)
	}
}

// TestSuccessfulCreatePersistsLastCreateAgent proves the write DOES happen
// -- and happens with the agent the session actually got created with
// (msg.session.Agent), not merely whatever m.createAgent last showed -- on
// the shellCreated success path, and that the newly persisted value is
// immediately reflected in model state (no store read in a render path)
// well enough that the very next "n" opens on it, labelled.
func TestSuccessfulCreatePersistsLastCreateAgent(t *testing.T) {
	db := openStoreForLastCreateAgent(t)
	ctx := context.Background()

	m := New(db, config.Settings{}, "")
	updated, cmd := m.Update(shellCreated{session: store.Session{ID: "sess-1", Agent: "pi"}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("shellCreated success did not return a command")
	}
	// Run the batched commands synchronously, exactly as
	// layout_persistence_test.go does for its own async persistence
	// commands (unit tests do not run Bubble Tea's real event loop).
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

	if m.lastCreateAgent != "pi" {
		t.Fatalf("model.lastCreateAgent = %q after a successful create, want %q", m.lastCreateAgent, "pi")
	}
	persisted, err := db.GetLastCreateAgent(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if persisted != "pi" {
		t.Fatalf("ui_state last_create_agent = %q after a successful create, want %q", persisted, "pi")
	}

	// The very next "n": reads model state only, per the doc comment on
	// pickCreateAgent/lastCreateAgent -- no additional store round trip.
	updated, _ = m.Update(key("n"))
	m = updated.(Model)
	if m.createAgent != "pi" {
		t.Fatalf("next create modal open chose Agent = %q, want %q", m.createAgent, "pi")
	}
	if !m.createAgentLastUsed {
		t.Fatal("next create modal open did not label Agent as last used")
	}
}
