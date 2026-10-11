package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// nameSourceOf reads the stored name_source of the one session called name.
func nameSourceOf(t *testing.T, db *store.Store, name string) string {
	t.Helper()
	sessions, err := db.ListSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sessions {
		if s.Name == name {
			return s.NameSource
		}
	}
	t.Fatalf("no stored session named %q in %d rows", name, len(sessions))
	return ""
}

// submitCreateModal opens the create modal with `n`, types typed into the Name
// field, presses enter and runs the resulting command, so a real
// service.CreateShell stores the row.
func submitCreateModal(t *testing.T, model tea.Model, typed string) {
	t.Helper()
	model, _ = model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("n")}))
	if typed != "" {
		model, _ = model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune(typed)}))
	}
	model, cmd := model.Update(tea.KeyMsg(tea.Key{Type: tea.KeyEnter}))
	if cmd == nil {
		t.Fatalf("enter on the create modal issued no command:\n%s", model.View())
	}
	cmd()
}

// TestNameSourceEntryPoints (R248): every entry point that creates or renames
// a session records who chose the name -- a blank name is `derived`, anything
// the user typed (create or rename) is `user`, a harness adoption is `harness`.
func TestNameSourceEntryPoints(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "copilot"), []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := tempDeckEnv(t)
	socket := "priv-nsrc-" + strings.ReplaceAll(filepath.Base(deckEnvValue(t, env, "XDG_STATE_HOME")), "_", "")
	defer killTmuxServer(socket)
	env = append(env, "DECK_TMUX_SOCKET="+socket)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
	settings, err := config.LoadFromProfile(os.Getenv, os.UserHomeDir, "")
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(settings.Paths)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	logger, err := audit.New(settings.Paths, settings.Clock)
	if err != nil {
		t.Fatal(err)
	}
	registry := newAgentRegistry()
	client := tmux.Client{Socket: settings.Socket}
	svc := newSessionService(db, settings, logger, client, registry, "deck")
	model := newDeckModel(db, settings, svc, client, registry, func(context.Context) error { return nil })

	blankCWD := filepath.Join(t.TempDir(), "blank-cwd")
	if err := os.MkdirAll(blankCWD, 0o700); err != nil {
		t.Fatal(err)
	}
	// The recent-cwd list outlives a subtest, so the modal's prefilled cwd must too.
	sharedCWD := t.TempDir()
	// Each case creates (or renames) one row and names the stored source it must leave.
	cases := []struct {
		name   string
		run    func(t *testing.T) (rowName string)
		wantNS string
	}{
		{"modal typed", func(t *testing.T) string {
			submitCreateModal(t, model, "typed-modal")
			return "typed-modal"
		}, "user"},
		{"deck new --name", func(t *testing.T) string {
			if code, _, stderr := runWithEnv(t, nil, "new", "--agent", "copilot", "--name", "typed-new", "--cwd", sharedCWD); code != 0 {
				t.Fatalf("deck new --name: exit %d %q", code, stderr)
			}
			return "typed-new"
		}, "user"},
		{"blank modal", func(t *testing.T) string {
			before := sessionNames(t, db)
			submitCreateModal(t, model, "")
			return newRowName(t, db, before)
		}, "derived"},
		{"blank deck new", func(t *testing.T) string {
			if code, _, stderr := runWithEnv(t, nil, "new", "--agent", "copilot", "--cwd", blankCWD); code != 0 {
				t.Fatalf("blank deck new: exit %d %q", code, stderr)
			}
			return "blank-cwd"
		}, "derived"},
		{"deck rename", func(t *testing.T) string {
			// Rename a derived row: the user's rename must flip it to user.
			if _, err := svc.Rename(context.Background(), sessionIDByName(t, db, "blank-cwd"), "renamed-by-user"); err != nil {
				t.Fatal(err)
			}
			return "renamed-by-user"
		}, "user"},
		{"harness adoption (store)", func(t *testing.T) string {
			id := sessionIDByName(t, db, "typed-new")
			if err := db.RenameSession(context.Background(), id, "adopted-name", "harness", settings.Clock.Now().UnixMilli()); err != nil {
				t.Fatal(err)
			}
			return "adopted-name"
		}, "harness"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := tc.run(t)
			if got := nameSourceOf(t, db, row); got != tc.wantNS {
				t.Fatalf("%s: stored name_source of %q = %q, want %q", tc.name, row, got, tc.wantNS)
			}
		})
	}
}

func sessionNames(t *testing.T, db *store.Store) map[string]bool {
	t.Helper()
	sessions, err := db.ListSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, s := range sessions {
		out[s.Name] = true
	}
	return out
}

func newRowName(t *testing.T, db *store.Store, before map[string]bool) string {
	t.Helper()
	for name := range sessionNames(t, db) {
		if !before[name] {
			return name
		}
	}
	t.Fatal("the modal stored no new row")
	return ""
}

func sessionIDByName(t *testing.T, db *store.Store, name string) string {
	t.Helper()
	sessions, err := db.ListSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sessions {
		if s.Name == name {
			return s.ID
		}
	}
	t.Fatalf("no session named %q", name)
	return ""
}
