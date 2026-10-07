package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

const copilotEnvTestID = "7f1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"

func copilotEnvRegistry() *agent.Registry {
	r := agent.NewRegistry()
	r.Register(agent.NewShell())
	r.Register(agent.NewCopilot())
	return r
}

// copilotEventsIn records a copilot transcript for copilotEnvTestID under root
// and returns its path.
func copilotEventsIn(t *testing.T, root string) string {
	t.Helper()
	dir := filepath.Join(root, "session-state", copilotEnvTestID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// R222 item 1: COPILOT_HOME reaches Copilot.TranscriptPaths through the
// session's own layering -- session env over config [env] over the session's
// own tmux server's global environment -- and never from this (observing)
// process's ambient environment. Each layer names its own root holding a copy
// of the same conversation's events.jsonl, so every verdict is distinguishable,
// and the ambient root is a fourth tree that must never be the answer.
func TestCopilotHomeReachesTranscriptPathsFromTheSessionEnvLayering(t *testing.T) {
	serverRoot, ambientRoot := t.TempDir(), t.TempDir()
	configRoot, sessionRoot := t.TempDir(), t.TempDir()
	serverPath := copilotEventsIn(t, serverRoot)
	ambientPath := copilotEventsIn(t, ambientRoot)
	configPath := copilotEventsIn(t, configRoot)
	sessionPath := copilotEventsIn(t, sessionRoot)

	// The tmux server starts while this process's COPILOT_HOME is serverRoot,
	// so that is its global environment for its whole life; only then does
	// the ambient value move.
	t.Setenv("COPILOT_HOME", serverRoot)
	socket := selectionTestSocket("copilot-home-layers")
	const slug = "copilot-home-layers"
	target, err := tmux.SessionName(slug)
	if err != nil {
		t.Fatal(err)
	}
	newQuietSelectionPane(t, socket, target, 80, 24)
	t.Setenv("COPILOT_HOME", ambientRoot)

	m := New(nil, config.Settings{}, "")
	m.agents = copilotEnvRegistry()
	m.tmuxClient = tmux.Client{Socket: socket}
	session := store.Session{ID: "s", Slug: slug, Name: "copilot", Agent: "copilot", ConversationID: copilotEnvTestID}

	check := func(label string, model Model, sess store.Session, want string) {
		t.Helper()
		got, ok := model.transcriptPathFor(sess)
		if !ok || got != want {
			t.Fatalf("%s: transcriptPathFor = (%q, %v), want %q (server %q, config %q, session %q, ambient %q)",
				label, got, ok, want, serverPath, configPath, sessionPath, ambientPath)
		}
	}
	check("no override resolves the session's own server, not the ambient env", m, session, serverPath)

	configured := m
	configured.settings.Env = map[string]string{"COPILOT_HOME": configRoot}
	check("config [env] over the server", configured, session, configPath)

	over := session
	over.Env = map[string]string{"COPILOT_HOME": sessionRoot}
	check("session env over the server", m, over, sessionPath)
	check("session env over config [env]", configured, over, sessionPath)

	// A session root without the conversation declines instead of falling
	// through to any other layer's tree.
	empty := session
	empty.Env = map[string]string{"COPILOT_HOME": t.TempDir()}
	if path, ok := configured.transcriptPathFor(empty); ok {
		t.Fatalf("a session COPILOT_HOME without the conversation resolved %q from another layer", path)
	}
}

// R222 item 3: the env editor has no copilot-specific behaviour. A COPILOT_*
// variable lists, edits and persists like any user variable; only a
// secret-looking name (COPILOT_GITHUB_TOKEN) is masked, by SPEC §6.4's rule,
// until the reveal toggle is on.
func TestEnvEditorTreatsCopilotVariablesLikeAnyUserVariable(t *testing.T) {
	const shown = "copilot-value-do-not-show-1234"
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{
		ID: "s1", Name: "sess", Agent: "copilot",
		Env: map[string]string{
			"COPILOT_GITHUB_TOKEN": shown,
			"COPILOT_MODEL":        "plain-model-name",
			"COPILOT_HOME":         "/srv/copilot-home",
			"COPILOT_ALLOW_ALL":    "true",
		},
	}}
	updated, _ := model.Update(key("e"))
	m := updated.(Model)
	if !m.envEditing {
		t.Fatal("e did not open the env editor on a copilot session")
	}
	view := m.View()
	for _, want := range []string{"COPILOT_GITHUB_TOKEN", "COPILOT_MODEL", "plain-model-name", "COPILOT_HOME", "/srv/copilot-home", "COPILOT_ALLOW_ALL"} {
		if !strings.Contains(view, want) {
			t.Fatalf("env editor omits %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, shown) {
		t.Fatalf("env editor shows a secret-looking COPILOT_* value unmasked:\n%s", view)
	}
	updated, _ = m.Update(key("r"))
	m = updated.(Model)
	if !strings.Contains(m.View(), shown) {
		t.Fatalf("r did not reveal the COPILOT_GITHUB_TOKEN value:\n%s", m.View())
	}
	updated, _ = m.Update(key("r"))
	if strings.Contains(updated.(Model).View(), shown) {
		t.Fatal("a second r did not re-mask the COPILOT_GITHUB_TOKEN value")
	}
}

// R222 item 3: editing a COPILOT_* variable dispatches the ordinary
// setSessionEnv call with exactly the typed key and value, as for any key.
func TestEnvEditorEditsACopilotVariableThroughSetSessionEnv(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{ID: "s1", Name: "sess", Agent: "copilot", Env: map[string]string{"COPILOT_MODEL": "before"}}}
	model.envEditing = true
	var calls int
	var gotID, gotKey, gotValue string
	model.setSessionEnv = func(_ context.Context, id, k, v string) (store.Session, error) {
		calls++
		gotID, gotKey, gotValue = id, k, v
		return store.Session{ID: id, Name: "sess", Agent: "copilot", Env: map[string]string{k: v}, EnvDirty: true}, nil
	}
	next, _ := model.updateEnvDialog(tea.KeyMsg{Type: tea.KeyEnter})
	m := next.(Model)
	if m.envEditKey != "COPILOT_MODEL" || m.envEdit.Value() != "before" {
		t.Fatalf("enter = key %q value %q, want COPILOT_MODEL/before", m.envEditKey, m.envEdit.Value())
	}
	for len(m.envEdit.Value()) > 0 {
		next, _ = m.updateEnvDialog(tea.KeyMsg{Type: tea.KeyBackspace})
		m = next.(Model)
	}
	next, _ = m.updateEnvDialog(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("after")})
	m = next.(Model)
	_, cmd := m.updateEnvDialog(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("committing the edit returned no command")
	}
	cmd()
	if calls != 1 || gotID != "s1" || gotKey != "COPILOT_MODEL" || gotValue != "after" {
		t.Fatalf("setSessionEnv calls=%d (%q, %q, %q), want one call (s1, COPILOT_MODEL, after)", calls, gotID, gotKey, gotValue)
	}
}
