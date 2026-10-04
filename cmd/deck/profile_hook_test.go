package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// envValue pulls "KEY=value"'s value back out of a tempDeckEnv-shaped
// slice, for tests that need to build config.Paths for a root tempDeckEnv
// already allocated (rather than re-deriving it and risking a second,
// different t.TempDir()).
func envValue(env []string, key string) string {
	for _, kv := range env {
		if name, value, ok := strings.Cut(kv, "="); ok && name == key {
			return value
		}
	}
	return ""
}

// listRelPaths walks root and returns every entry's path relative to root,
// sorted, excluding root itself. Comparing this slice before and after a
// run() call is this file's "created no file under the temp home" check --
// stronger than an existence probe on any one guessed path, since it also
// catches an unexpected file anywhere else under the same root.
func listRelPaths(t *testing.T, root string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(root, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		found = append(found, rel)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(found)
	return found
}

// defaultPathsFor mirrors config.resolvePaths' DECK_HOME branch for the
// default profile (unexported, so this file rebuilds the same four paths
// rather than reaching into config internals): the flat layout, unchanged.
func defaultPathsFor(deckHome string) config.Paths {
	return config.Paths{
		Home: deckHome, DataDir: deckHome,
		ConfigFile: filepath.Join(deckHome, "config.toml"),
		LogDir:     filepath.Join(deckHome, "log"),
		StateDB:    filepath.Join(deckHome, "state.db"),
	}
}

// namedPathsFor mirrors config.resolvePaths' DECK_HOME branch for a named
// profile: profiles/<name> inserted after DECK_HOME.
func namedPathsFor(deckHome, profile string) config.Paths {
	root := filepath.Join(deckHome, "profiles", profile)
	return config.Paths{
		Home: root, DataDir: root,
		ConfigFile: filepath.Join(root, "config.toml"),
		LogDir:     filepath.Join(root, "log"),
		StateDB:    filepath.Join(root, "state.db"),
	}
}

// seedSession opens paths' state.db, creates one session row, and closes
// it again, so a later hook run has both an existing, non-empty database to
// leave untouched (default's) and, for the positive case, a row a hook
// payload can actually resolve and update.
func seedSession(t *testing.T, paths config.Paths, id, conversationID string) {
	t.Helper()
	db, err := store.Open(paths)
	if err != nil {
		t.Fatalf("seed %s: open: %v", paths.StateDB, err)
	}
	if _, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: id, Name: id, CWD: paths.Home, Agent: "claude", CapturedPath: "/bin",
		Status: "starting", StatusSource: "user", StatusAt: 1000, CreatedAt: 1000,
		ConversationID: conversationID,
	}); err != nil {
		t.Fatalf("seed %s: create session: %v", paths.StateDB, err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("seed %s: close: %v", paths.StateDB, err)
	}
}

// TestHookRefusesBadOrMissingProfile pins SPEC §3.4: "_hook resolves its
// paths from [DECK_PROFILE] with the same resolver the TUI uses ... If the
// pane's DECK_PROFILE is invalid, or names a profile whose directory no
// longer exists, _hook writes nothing, creates nothing, never falls back to
// default's database ... prints one line on stderr naming the pane and the
// profile, and exits exactly 1." Both named cases -- a syntactically
// invalid name, and a syntactically valid name with no profiles/<name>
// directory -- must behave identically: exit 1, one stderr line naming
// TMUX_PANE and the profile, no new file anywhere under DECK_HOME
// (directory listing identical before/after), and default's own
// pre-existing state.db left byte-for-byte and mtime-for-mtime unchanged --
// the one guard against a silent fallback to default's database.
func TestHookRefusesBadOrMissingProfile(t *testing.T) {
	cases := []struct {
		name    string
		profile string
	}{
		{name: "invalid_name", profile: "Acme"}, // uppercase: fails ValidateProfileName
		{name: "missing_dir", profile: "ghost"}, // valid syntax, no profiles/ghost dir
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := tempDeckEnv(t)
			deckHome := envValue(env, "DECK_HOME")
			if deckHome == "" {
				t.Fatal("tempDeckEnv did not set DECK_HOME")
			}
			defaultPaths := defaultPathsFor(deckHome)
			seedSession(t, defaultPaths, "default-seed", "default-seed-conversation")

			listingBefore := listRelPaths(t, deckHome)
			statBefore, err := os.Stat(defaultPaths.StateDB)
			if err != nil {
				t.Fatal(err)
			}
			contentBefore, err := os.ReadFile(defaultPaths.StateDB)
			if err != nil {
				t.Fatal(err)
			}

			hookEnv := slices.Concat(env, []string{"DECK_PROFILE=" + tc.profile, "TMUX_PANE=%3"})
			code, _, stderr := runWithEnv(t, hookEnv, "_hook")
			if code != 1 {
				t.Fatalf("run(deck _hook) DECK_PROFILE=%s exit = %d, want 1 (stderr=%q)", tc.profile, code, stderr)
			}
			if strings.Count(stderr, "\n") != 1 {
				t.Fatalf("run(deck _hook) DECK_PROFILE=%s stderr = %q, want exactly one line", tc.profile, stderr)
			}
			if !strings.Contains(stderr, "TMUX_PANE") || !strings.Contains(stderr, "%3") {
				t.Fatalf("run(deck _hook) DECK_PROFILE=%s stderr = %q, want it to name TMUX_PANE and its value", tc.profile, stderr)
			}
			if !strings.Contains(stderr, tc.profile) {
				t.Fatalf("run(deck _hook) DECK_PROFILE=%s stderr = %q, want it to name the profile", tc.profile, stderr)
			}

			listingAfter := listRelPaths(t, deckHome)
			if len(listingBefore) != len(listingAfter) {
				t.Fatalf("DECK_HOME listing changed: before=%v after=%v", listingBefore, listingAfter)
			}
			for i := range listingBefore {
				if listingBefore[i] != listingAfter[i] {
					t.Fatalf("DECK_HOME listing changed: before=%v after=%v", listingBefore, listingAfter)
				}
			}

			statAfter, err := os.Stat(defaultPaths.StateDB)
			if err != nil {
				t.Fatal(err)
			}
			if !statAfter.ModTime().Equal(statBefore.ModTime()) {
				t.Fatalf("default state.db mtime changed: before=%s after=%s", statBefore.ModTime(), statAfter.ModTime())
			}
			contentAfter, err := os.ReadFile(defaultPaths.StateDB)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(contentBefore, contentAfter) {
				t.Fatalf("default state.db content changed for DECK_PROFILE=%s", tc.profile)
			}
		})
	}
}

// TestHookWithNamedProfileWritesOnlyThatProfilesDatabase pins the positive
// half of SPEC §3.4's _hook profile rule: a hook fired from a pane whose
// DECK_PROFILE names an EXISTING profile resolves and writes to that
// profile's own state.db, and default's database (already seeded, as in
// the refusal test above) is left completely untouched.
func TestHookWithNamedProfileWritesOnlyThatProfilesDatabase(t *testing.T) {
	env := tempDeckEnv(t)
	deckHome := envValue(env, "DECK_HOME")
	if deckHome == "" {
		t.Fatal("tempDeckEnv did not set DECK_HOME")
	}
	defaultPaths := defaultPathsFor(deckHome)
	seedSession(t, defaultPaths, "default-seed", "default-seed-conversation")

	namedPaths := namedPathsFor(deckHome, "acme")
	if err := os.MkdirAll(namedPaths.DataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	seedSession(t, namedPaths, "acme-row", "acme-conversation")

	statBefore, err := os.Stat(defaultPaths.StateDB)
	if err != nil {
		t.Fatal(err)
	}
	contentBefore, err := os.ReadFile(defaultPaths.StateDB)
	if err != nil {
		t.Fatal(err)
	}

	for _, kv := range append(env, "DECK_PROFILE=acme") {
		name, value, _ := strings.Cut(kv, "=")
		t.Setenv(name, value)
	}
	var stdout, stderr bytes.Buffer
	payload := `{"hook_event_name":"SessionEnd","session_id":"acme-conversation","reason":"logout"}`
	code := run([]string{"deck", "_hook"}, strings.NewReader(payload), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(deck _hook) DECK_PROFILE=acme exit = %d, want 0 (stderr=%q)", code, stderr.String())
	}

	namedDB, err := store.Open(namedPaths)
	if err != nil {
		t.Fatal(err)
	}
	row, err := namedDB.GetSession(context.Background(), "acme-row")
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "stopped" || row.StatusSource != "hook" {
		t.Fatalf("acme row after hook = %#v, want status=stopped source=hook", row)
	}
	if err := namedDB.Close(); err != nil {
		t.Fatal(err)
	}

	statAfter, err := os.Stat(defaultPaths.StateDB)
	if err != nil {
		t.Fatal(err)
	}
	if !statAfter.ModTime().Equal(statBefore.ModTime()) {
		t.Fatalf("default state.db mtime changed: before=%s after=%s", statBefore.ModTime(), statAfter.ModTime())
	}
	contentAfter, err := os.ReadFile(defaultPaths.StateDB)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(contentBefore, contentAfter) {
		t.Fatal("default state.db content changed by a hook targeting a named profile")
	}
	defaultDB, err := store.Open(defaultPaths)
	if err != nil {
		t.Fatal(err)
	}
	defaultRow, err := defaultDB.GetSession(context.Background(), "default-seed")
	if err != nil {
		t.Fatal(err)
	}
	if defaultRow.Status != "starting" {
		t.Fatalf("default-seed row status = %q, want unchanged %q", defaultRow.Status, "starting")
	}
	if err := defaultDB.Close(); err != nil {
		t.Fatal(err)
	}
}
