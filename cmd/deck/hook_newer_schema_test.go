package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

const oldHookSchema = 7

// buildOldSchemaDeck builds the real cmd/deck with the test-only
// deckoldschema tag, so the binary claims an older schema (R204) without ever
// being a real old release, and a recognisable version string.
func buildOldSchemaDeck(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "deck-old")
	ldflags := fmt.Sprintf("-X main.version=v0.0.1-old -X github.com/n-orlov/deck/internal/store.oldSchemaVersion=%d", oldHookSchema)
	build := exec.Command("go", "build", "-tags", "deckoldschema", "-ldflags", ldflags, "-o", binary, ".")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build old-schema deck: %v: %s", err, output)
	}
	return binary
}

// newerStateHome creates a state database one schema newer than this tree's
// own SchemaVersion, so it is newer than the old-schema binary too.
func newerStateHome(t *testing.T) (home string, dbSchema int) {
	t.Helper()
	home = t.TempDir()
	paths := config.Paths{Home: home, DataDir: home, ConfigFile: filepath.Join(home, "config.toml"), LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")}
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	dbSchema = store.SchemaVersion + 1
	if _, err := db.DB().Exec(`UPDATE meta SET version = ? WHERE key = 'schema_version'`, dbSchema); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return home, dbSchema
}

func runOldDeck(t *testing.T, binary, home, stdin string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = append(os.Environ(), "DECK_HOME="+home, "DECK_TMUX_SOCKET=priv-hook-newer-schema")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

// R204.1 (#56): an older _hook against a newer database tells the user to
// restart the session from deck, naming itself, and never says "upgrade deck".
func TestHookOlderThanDatabasePrintsRestartMessage(t *testing.T) {
	binary := buildOldSchemaDeck(t)
	home, dbSchema := newerStateHome(t)
	out, err := runOldDeck(t, binary, home, "{}", "_hook")
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("hook exit = %v, want a non-zero exit; output %q", err, out)
	}
	for _, want := range []string{
		binary,
		"v0.0.1-old",
		fmt.Sprintf("schema %d", oldHookSchema),
		fmt.Sprintf("schema %d", dbSchema),
		"Restart the session from deck (R)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("hook output lacks %q: %q", want, out)
		}
	}
	if strings.Contains(strings.ToLower(out), "upgrade deck") {
		t.Errorf("hook output says to upgrade deck: %q", out)
	}
	if !filepath.IsAbs(binary) {
		t.Fatalf("test binary path %q is not absolute", binary)
	}
}

// The TUI's own store-open path (the same old-schema binary, no _hook) keeps
// the "upgrade deck" wording: a user running deck itself can upgrade it.
func TestTUIOpenOlderThanDatabaseStillSaysUpgradeDeck(t *testing.T) {
	binary := buildOldSchemaDeck(t)
	home, _ := newerStateHome(t)
	out, err := runOldDeck(t, binary, home, "")
	if err != nil {
		t.Fatalf("deck exit = %v, output %q", err, out)
	}
	if !strings.Contains(out, "upgrade deck") {
		t.Errorf("TUI open output lacks \"upgrade deck\": %q", out)
	}
	if strings.Contains(out, "Restart the session") {
		t.Errorf("TUI open output carries the hook message: %q", out)
	}
}

func TestHookFailureMessagePassesOtherErrorsThrough(t *testing.T) {
	if got := hookFailureMessage(errors.New("boom")); got != "boom" {
		t.Fatalf("hookFailureMessage = %q, want boom", got)
	}
	wrapped := fmt.Errorf("open existing state database: %w", &store.NewerSchemaError{DB: 9, Supported: 8})
	if got := hookFailureMessage(wrapped); !strings.Contains(got, "Restart the session from deck (R)") || strings.Contains(got, "upgrade deck") {
		t.Fatalf("hookFailureMessage = %q", got)
	}
}
