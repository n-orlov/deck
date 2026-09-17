package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestTranscriptPathForUsesActualServerEnvironmentNotObserverAmbient is
// R121's own regression (cure-03-02-2, review pass 234's B0-adjacent
// finding on top of task 007's three-fixture-tree guard in
// transcript_env_layers_test.go): a REAL tmux server, started while this
// test process's own CODEX_HOME is root-A, keeps root-A as what its pane
// actually inherited for the whole life of the server -- Unix processes
// never see a later ambient env change made by some other, later
// process. A later "observer" TUI whose own ambient CODEX_HOME is root-B,
// with no session or config override, must still resolve root-A's
// transcript file: transcriptPathFor's server-env layer answers "what did
// the session's own server actually hand its pane", never "what does this
// call happen to be running under right now".
//
// Before the fix, resolveEnvKey's server-env fallback was os.LookupEnv on
// this process's own environment -- which, for an observer started after
// the ambient value moved to root-B, resolves root-B's file instead, or
// none at all if root-A and root-B don't share a filesystem view. This
// test's own git history is the failure/repair pair the task asks for:
// `git show HEAD~1:internal/tui/env_editor.go` (this commit) still had
// the os.LookupEnv fallback and failed this same test red (recorded in
// docs/reports/phase4-r121-server-env/); this commit's tmux-queried
// fallback (tmux.Client.ServerEnvironment, `show-environment -g`) is what
// turns it green, because that command answers from the server's own
// table -- set once, when the server started -- rather than this
// process's mutable ambient environment.
func TestTranscriptPathForUsesActualServerEnvironmentNotObserverAmbient(t *testing.T) {
	const id = "01a09616-150d-7252-959c-d72a289dae41"
	makeTranscript := func(root, marker string) string {
		dir := filepath.Join(root, "sessions", "2026", "09", "12")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "rollout-2026-09-12T15-47-04-"+id+".jsonl")
		if err := os.WriteFile(path, []byte(marker), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	// Four DISTINCT roots, each holding its own copy of the same
	// conversation id's transcript file, so every layer's verdict is
	// distinguishable from every other layer's: a control that pointed an
	// override at serverHome would pass identically whether the override
	// won or the server-env fallback did, and so could not detect a
	// precedence regression at all.
	serverHome, observerHome := t.TempDir(), t.TempDir()
	sessionHome, configHome := t.TempDir(), t.TempDir()
	want := makeTranscript(serverHome, "actual-session")
	wrong := makeTranscript(observerHome, "unrelated-observer")
	wantSessionOverride := makeTranscript(sessionHome, "session-override")
	wantConfigOverride := makeTranscript(configHome, "config-override")

	// The server is started while our OWN process env still says
	// serverHome -- this is what it inherits, permanently.
	t.Setenv("CODEX_HOME", serverHome)
	socket := selectionTestSocket("transcript-server-env")
	const slug = "transcript-server-env"
	target, err := tmux.SessionName(slug)
	if err != nil {
		t.Fatal(err)
	}
	newQuietSelectionPane(t, socket, target, 80, 24)

	// The later "observer": only THIS process's ambient value moves, the
	// already-running server's own table cannot follow it.
	t.Setenv("CODEX_HOME", observerHome)

	// Fixture sanity: the server's own global table still says
	// serverHome, and the pane it hosts really did inherit it.
	out, err := exec.Command("tmux", "-L", socket, "show-environment", "-g", "CODEX_HOME").CombinedOutput()
	if err != nil {
		t.Fatalf("show server env: %v %s", err, out)
	}
	if strings.TrimSpace(string(out)) != "CODEX_HOME="+serverHome {
		t.Fatalf("invalid fixture: server env %q", out)
	}
	pidBytes, err := exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", target, "#{pane_pid}").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	paneEnv, err := os.ReadFile("/proc/" + strings.TrimSpace(string(pidBytes)) + "/environ")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(paneEnv), "CODEX_HOME="+serverHome+"\x00") {
		t.Fatal("invalid fixture: pane did not inherit server CODEX_HOME")
	}

	registry := codexRegistry()
	m := New(nil, config.Settings{}, "")
	m.agents = registry
	m.tmuxClient = tmux.Client{Socket: socket}
	session := store.Session{ID: "s", Slug: slug, Name: "codex", Agent: "codex", ConversationID: id}

	// Positive control: a session override names sessionHome, a root that
	// is neither the server's nor the config's nor the observer's, so this
	// sub-test fails if session-over-server precedence stops holding.
	t.Run("session-override-wins-over-actual-server", func(t *testing.T) {
		over := session
		over.Env = map[string]string{"CODEX_HOME": sessionHome}
		got, ok := m.transcriptPathFor(over)
		if !ok || got != wantSessionOverride {
			t.Fatalf("session override = (%q, %v), want the session's own root %q (not the server's %q or the observer's %q)", got, ok, wantSessionOverride, want, wrong)
		}
	})
	// Positive control: config over server, again pointed at its own
	// distinct root rather than at the server's.
	t.Run("config-override-wins-over-actual-server", func(t *testing.T) {
		configured := m
		configured.settings.Env = map[string]string{"CODEX_HOME": configHome}
		got, ok := configured.transcriptPathFor(session)
		if !ok || got != wantConfigOverride {
			t.Fatalf("config override = (%q, %v), want the config's own root %q (not the server's %q or the observer's %q)", got, ok, wantConfigOverride, want, wrong)
		}
	})
	// Precedence between the two override layers themselves: session wins
	// over config, and both are distinct from the server's root.
	t.Run("session-override-wins-over-config-override", func(t *testing.T) {
		configured := m
		configured.settings.Env = map[string]string{"CODEX_HOME": configHome}
		over := session
		over.Env = map[string]string{"CODEX_HOME": sessionHome}
		got, ok := configured.transcriptPathFor(over)
		if !ok || got != wantSessionOverride {
			t.Fatalf("session over config = (%q, %v), want the session's own root %q (not the config's %q)", got, ok, wantSessionOverride, wantConfigOverride)
		}
	})
	t.Run("no-override-falls-through-to-the-actual-server-not-the-observer", func(t *testing.T) {
		got, ok := m.transcriptPathFor(session)
		t.Logf("actual server root=%s observer ambient root=%s got=%s", serverHome, observerHome, got)
		if !ok || got != want {
			t.Fatalf("transcriptPathFor = (%q, %v), want the actual server's file %q, never the observer's own ambient file %q", got, ok, want, wrong)
		}
	})
}
