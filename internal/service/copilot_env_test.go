package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// copilotUserKeys is R222 item 3's finite list: variables deck's launch must
// leave exactly as the operator's own layers left them. COPILOT_ALLOW_ALL is
// on the list only outside the yolo profile.
var copilotUserKeys = []string{"COPILOT_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN", "COPILOT_MODEL", "HTTPS_PROXY", "COPILOT_ALLOW_ALL"}

// R222 item 3: with no operator value, a copilot launch puts none of the keys
// in the session's pane environment (neither a value nor a `-KEY` unset),
// outside yolo; yolo alone sets COPILOT_ALLOW_ALL (R217) and still touches
// nothing else. The ambient process values are deliberately set: they must
// not be cleared or copied either.
func TestCopilotLaunchNeitherSetsNorClearsTheUserOwnedVariables(t *testing.T) {
	for _, profile := range []string{"safe", "edits", "yolo"} {
		t.Run(profile, func(t *testing.T) {
			svc, _, socket, _ := newCopilotTestService(t, "copilot-env-absent-"+profile)
			for _, key := range copilotUserKeys {
				t.Setenv(key, "ambient-"+key)
			}
			created, err := svc.CreateAgent(context.Background(), AgentCreateInput{Name: "Copilot env " + profile, CWD: t.TempDir(), Agent: "copilot", PermissionProfile: profile})
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range copilotUserKeys {
				if key == "COPILOT_ALLOW_ALL" && profile == "yolo" {
					assertTMuxEnvironment(t, socket, created.Slug, key, "true")
					continue
				}
				assertTMuxEnvironmentAbsent(t, socket, created.Slug, key)
			}
		})
	}
}

// R222 item 3: values the operator did set (session env, config [env]) reach
// the pane exactly as written -- not edited, not replaced -- on a launch and on
// a relaunch, outside yolo.
func TestCopilotLaunchPassesTheUserOwnedVariablesThroughUnedited(t *testing.T) {
	svc, _, socket, _ := newCopilotTestService(t, "copilot-env-pass")
	svc.ConfigEnv = map[string]string{"HTTPS_PROXY": "http://proxy.invalid:3128", "GITHUB_TOKEN": "config-token", "COPILOT_MODEL": "config-model"}
	sessionEnv := map[string]string{"COPILOT_GITHUB_TOKEN": "session-token", "GH_TOKEN": "session-gh", "COPILOT_MODEL": "session-model", "COPILOT_ALLOW_ALL": "false"}
	created, err := svc.CreateAgent(context.Background(), AgentCreateInput{Name: "Copilot env pass", CWD: t.TempDir(), Agent: "copilot", PermissionProfile: "safe", Env: sessionEnv})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"COPILOT_GITHUB_TOKEN": "session-token", "GH_TOKEN": "session-gh", "GITHUB_TOKEN": "config-token",
		"COPILOT_MODEL": "session-model", "HTTPS_PROXY": "http://proxy.invalid:3128", "COPILOT_ALLOW_ALL": "false",
	}
	for key, value := range want {
		assertTMuxEnvironment(t, socket, created.Slug, key, value)
	}
	if _, outcome, err := svc.Restart(context.Background(), created.ID); err != nil || outcome != ResumeStarted {
		t.Fatalf("restart: %v, outcome %v", err, outcome)
	}
	for key, value := range want {
		assertTMuxEnvironment(t, socket, created.Slug, key, value)
	}
}

// R222 items 1 and 2: a session COPILOT_HOME override moves the transcript
// root (resolved through the session's own layering, not the ambient env) and
// leaves deck's plugin directory under its data root; nothing is written under
// the override.
func TestCopilotSessionHomeOverrideMovesTranscriptButNotThePluginDir(t *testing.T) {
	svc, auditPath, socket, home := newCopilotTestService(t, "copilot-home-override")
	ambient := t.TempDir()
	t.Setenv("COPILOT_HOME", ambient)
	override := t.TempDir()
	created, err := svc.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Copilot home", CWD: t.TempDir(), Agent: "copilot", PermissionProfile: "safe",
		Env: map[string]string{"COPILOT_HOME": override},
	})
	if err != nil {
		t.Fatal(err)
	}
	pluginDir := agent.CopilotPluginDir(svc.DeckHome)
	if !strings.HasPrefix(pluginDir, svc.DeckHome+string(filepath.Separator)) {
		t.Fatalf("plugin dir %q is not under the data root %q", pluginDir, svc.DeckHome)
	}
	argv := launchedArgv(t, auditPath)
	if len(argv) < 2 || argv[len(argv)-2] != "--plugin-dir" || argv[len(argv)-1] != pluginDir {
		t.Fatalf("argv = %#v, want it to end with --plugin-dir %s", argv, pluginDir)
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "hooks.json")); err != nil {
		t.Fatalf("plugin not installed under the data root: %v", err)
	}
	assertTMuxEnvironment(t, socket, created.Slug, "COPILOT_HOME", override)
	if entries, _ := os.ReadDir(override); len(entries) != 0 {
		t.Fatalf("the launch wrote %d entries under the session's COPILOT_HOME", len(entries))
	}
	assertNothingUnderCopilotHome(t, home)

	// The transcript convention follows the override; the ambient root and
	// $HOME/.copilot each hold a same-id file that must lose.
	record := func(root string) string {
		dir := filepath.Join(root, "session-state", created.ConversationID)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "events.jsonl")
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	want := record(override)
	record(ambient)
	record(filepath.Join(home, ".copilot"))
	adapter := agent.NewCopilot()
	row, err := svc.Store.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	env := svc.transcriptEnv(adapter.Capabilities().TranscriptEnvKeys, row)
	got, ok := adapter.TranscriptPaths(agent.TranscriptInput{Home: home, ConversationID: row.ConversationID, Env: env})
	if !ok || got != want {
		t.Fatalf("TranscriptPaths = (%q, %v), want the override's %q", got, ok, want)
	}

	// Without the override the same session resolves its server/ambient layers,
	// never the override, and the plugin dir is unchanged.
	row.Env = nil
	env = svc.transcriptEnv(adapter.Capabilities().TranscriptEnvKeys, row)
	if got, ok := adapter.TranscriptPaths(agent.TranscriptInput{Home: home, ConversationID: row.ConversationID, Env: env}); got == want {
		t.Fatalf("TranscriptPaths without the override = (%q, %v), still the override", got, ok)
	}
	if agent.CopilotPluginDir(svc.DeckHome) != pluginDir {
		t.Fatal("plugin dir moved")
	}
}

// R222 item 1: the layering the service resolves for COPILOT_HOME is session
// env > config [env] > the session's tmux server's global environment, and
// the ambient process value loses to every one of them.
func TestCopilotHomeResolvesThroughServerConfigAndSessionLayers(t *testing.T) {
	svc, _, _, _ := newCopilotTestService(t, "copilot-home-layers")
	serverRoot, ambientRoot, configRoot, sessionRoot := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("COPILOT_HOME", serverRoot)
	if _, err := svc.TMux.Create(context.Background(), tmux.Launch{Slug: "layers", CWD: t.TempDir(), Command: []string{"/bin/sh", "-c", "sleep 30"}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COPILOT_HOME", ambientRoot)

	keys := agent.NewCopilot().Capabilities().TranscriptEnvKeys
	if len(keys) != 1 || keys[0] != "COPILOT_HOME" {
		t.Fatalf("declared transcript env keys = %v, want [COPILOT_HOME]", keys)
	}
	resolve := func(session store.Session) string {
		return svc.transcriptEnv(keys, session)["COPILOT_HOME"]
	}
	if got := resolve(store.Session{}); got != serverRoot {
		t.Fatalf("server layer = %q, want %q (ambient is %q)", got, serverRoot, ambientRoot)
	}
	svc.ConfigEnv = map[string]string{"COPILOT_HOME": configRoot}
	if got := resolve(store.Session{}); got != configRoot {
		t.Fatalf("config layer = %q, want %q", got, configRoot)
	}
	if got := resolve(store.Session{Env: map[string]string{"COPILOT_HOME": sessionRoot}}); got != sessionRoot {
		t.Fatalf("session layer = %q, want %q", got, sessionRoot)
	}
}

// R222 item 4: a copilot pane that prints nothing for the whole launch window
// (a dead proxy keeps the TUI from drawing) stays `starting` and is never made
// `error` by the probe; the first output it draws moves the row on.
func TestCopilotSilentStartupStaysStartingAndLeavesItOnceOutputArrives(t *testing.T) {
	svc, db := newCopilotProbeService(t, "copilot-silent")
	cwd := t.TempDir()
	stale := svc.Clock.Now().UnixMilli() - 2*config.DefaultStaleAfter.Milliseconds()
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: fmt.Sprintf("00000000-0000-4000-8000-%012d", 800), Name: "copilot silent", CWD: cwd,
		Agent: "copilot", CapturedPath: "/bin", Status: "starting", StatusSource: "tmux",
		StatusAt: stale, CreatedAt: stale, PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatal(err)
	}
	draw := filepath.Join(t.TempDir(), "draw")
	pane, err := os.ReadFile(filepath.Join("..", "agent", "testdata", "probes", "copilot", "idle.txt"))
	if err != nil {
		t.Fatal(err)
	}
	observed, err := svc.TMux.Create(context.Background(), tmux.Launch{
		Slug: session.Slug, CWD: cwd,
		Command: []string{"/bin/sh", "-c", `while [ ! -e "$1" ]; do sleep 0.05; done; printf '%s' "$2"; sleep 30`, "silent-copilot", draw, string(pane)},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Several probe passes over a pane with no output at all: the row never
	// leaves starting and never becomes an error.
	for pass := 0; pass < 3; pass++ {
		got := probeCopilotRow(t, svc, db, session)
		if got.Status != "starting" || got.Status == "error" {
			t.Fatalf("pass %d: silent copilot row = status %q (%q), want starting", pass, got.Status, got.StatusReason)
		}
	}

	if err := os.WriteFile(draw, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		out, err := svc.TMux.CapturePane(context.Background(), observed.Panes[0].ID, tmux.CaptureOptions{StartLine: "-200", EndLine: "-"})
		if err == nil && strings.Contains(string(out), "Interactive") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the pane never drew its output (capture %q, err %v)", out, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	got := probeCopilotRow(t, svc, db, session)
	if got.Status == "starting" || got.Status == "error" || got.StatusSource != "probe" {
		t.Fatalf("after output the row = status %q source %q, want it past starting by the probe", got.Status, got.StatusSource)
	}
}
