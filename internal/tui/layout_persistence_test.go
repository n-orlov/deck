package tui

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestPipeAndAngleBracketsPersistToUIStateNotConfigToml covers task 016
// (SPEC §11.2, requirement 12): `|` and `<`/`>` must write layout_mode and
// sidebar_width to state.db's ui_state table only, never to config.toml,
// and a restarted client (a fresh New(db, ...) call, exactly as
// acknowledge_test.go's "restarted" model simulates it) must render the
// persisted mode/width rather than the defaults.
func TestPipeAndAngleBracketsPersistToUIStateNotConfigToml(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, "config.toml")
	configBody := []byte("[ui]\nmouse = true\n")
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}

	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	settings := config.Settings{}
	model := New(db, settings, "")
	model.width, model.height = 100, 24

	if model.layoutMode != LayoutAuto || model.sidebarWidth != store.DefaultSidebarWidth {
		t.Fatalf("fresh state.db should degrade to defaults, got layoutMode=%q sidebarWidth=%d", model.layoutMode, model.sidebarWidth)
	}

	// `|` cycles auto -> side-by-side; run the returned command synchronously
	// (unit tests do not run Bubble Tea's own event loop) and feed its
	// message back in, exactly like acknowledge_test.go does for its own
	// async commands.
	updated, cmd := model.Update(key("|"))
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("| did not return a persistence command")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if model.attachError != "" {
		t.Fatalf("| persistence reported an error: %s", model.attachError)
	}

	updated, cmd = model.Update(key("<"))
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("< did not return a persistence command")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if model.attachError != "" {
		t.Fatalf("< persistence reported an error: %s", model.attachError)
	}

	if model.layoutMode != LayoutSideBySide {
		t.Fatalf("layoutMode = %q, want %q", model.layoutMode, LayoutSideBySide)
	}
	if model.sidebarWidth != store.DefaultSidebarWidth-1 {
		t.Fatalf("sidebarWidth = %d, want %d", model.sidebarWidth, store.DefaultSidebarWidth-1)
	}

	after, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	afterBody, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(configBody, afterBody) {
		t.Fatalf("config.toml bytes changed:\nbefore: %q\nafter:  %q", configBody, afterBody)
	}
	if before.ModTime() != after.ModTime() || before.Size() != after.Size() {
		t.Fatalf("config.toml metadata changed: before=%v/%d after=%v/%d", before.ModTime(), before.Size(), after.ModTime(), after.Size())
	}

	ctx := context.Background()
	persistedMode, err := db.GetLayoutMode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if persistedMode != LayoutSideBySide {
		t.Fatalf("ui_state layout_mode = %q, want %q", persistedMode, LayoutSideBySide)
	}
	persistedWidth, err := db.GetSidebarWidth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if persistedWidth != store.DefaultSidebarWidth-1 {
		t.Fatalf("ui_state sidebar_width = %d, want %d", persistedWidth, store.DefaultSidebarWidth-1)
	}

	// A restarted client is a fresh New(db, ...) call over the same
	// state.db; it must render the persisted pin/width, not the defaults.
	restarted := New(db, settings, "")
	if restarted.layoutMode != LayoutSideBySide {
		t.Fatalf("restarted client layoutMode = %q, want %q", restarted.layoutMode, LayoutSideBySide)
	}
	if restarted.sidebarWidth != store.DefaultSidebarWidth-1 {
		t.Fatalf("restarted client sidebarWidth = %d, want %d", restarted.sidebarWidth, store.DefaultSidebarWidth-1)
	}
}
