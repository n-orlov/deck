package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

func deckEnvValue(t *testing.T, env []string, key string) string {
	t.Helper()
	for _, kv := range env {
		if strings.HasPrefix(kv, key+"=") {
			return strings.TrimPrefix(kv, key+"=")
		}
	}
	t.Fatalf("%s missing from env", key)
	return ""
}

// R220.3: bare `deck new` is still the profile named `new`; only `new` followed
// by an option is the creation verb.
func TestIsNewCreateRequestLeavesBareNewAsTheProfile(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"deck", "new"}, false},
		{[]string{"deck", "work"}, false},
		{[]string{"deck", "work", "--agent", "copilot"}, false},
		{[]string{"deck", "new", "--agent", "copilot"}, true},
		{[]string{"deck", "new", "--name", "x"}, true},
	}
	for _, c := range cases {
		if got := isNewCreateRequest(c.args); got != c.want {
			t.Errorf("isNewCreateRequest(%v) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestParseNewCreateArgs(t *testing.T) {
	good := []struct {
		rest []string
		want newCreateArgs
	}{
		{[]string{"--agent", "copilot"}, newCreateArgs{agent: "copilot"}},
		{[]string{"--agent=copilot", "--name=n", "--cwd=/tmp", "--permission=edits"}, newCreateArgs{agent: "copilot", name: "n", cwd: "/tmp", permission: "edits"}},
		{[]string{"--permission", "yolo", "--cwd", "/x", "--name", "a b", "--agent", "codex"}, newCreateArgs{agent: "codex", name: "a b", cwd: "/x", permission: "yolo"}},
	}
	for _, c := range good {
		var stderr bytes.Buffer
		got, ok := parseNewCreateArgs(c.rest, &stderr)
		if !ok || got != c.want {
			t.Errorf("parseNewCreateArgs(%v) = %+v ok=%v (%q), want %+v", c.rest, got, ok, stderr.String(), c.want)
		}
	}
	bad := map[string]string{
		"--bogus":   "error: unknown flag --bogus",
		"stray":     `error: deck new takes options only, got "stray"`,
		"--agent":   "error: --agent needs a value",
		"-x":        "error: unknown flag -x",
		"--agentx=": "error: unknown flag --agentx=",
	}
	for arg, want := range bad {
		var stderr bytes.Buffer
		if _, ok := parseNewCreateArgs([]string{arg}, &stderr); ok || !strings.Contains(stderr.String(), want) {
			t.Errorf("parseNewCreateArgs(%q) ok=%v stderr=%q, want refusal %q", arg, ok, stderr.String(), want)
		}
	}
}

// R220.3: copilot is accepted from the flag and from config; any other
// unregistered kind is refused with the create service's own diagnostic.
func TestResolveNewAgentAcceptsCopilotAndRefusesUnknownKinds(t *testing.T) {
	registry := newAgentRegistry()
	for _, kind := range registry.Kinds() {
		if got, err := resolveNewAgent(kind, "", registry); err != nil || got != kind {
			t.Errorf("resolveNewAgent(--agent %s) = %q, %v", kind, got, err)
		}
		if got, err := resolveNewAgent("", kind, registry); err != nil || got != kind {
			t.Errorf("resolveNewAgent(config agent=%s) = %q, %v", kind, got, err)
		}
	}
	if got, err := resolveNewAgent("copilot", "codex", registry); err != nil || got != "copilot" {
		t.Errorf("the flag must win over config, got %q, %v", got, err)
	}
	for _, c := range [][2]string{{"ghost", ""}, {"", "ghost"}, {"Copilot", ""}, {"", " copilot"}} {
		_, err := resolveNewAgent(c[0], c[1], registry)
		kind := c[0] + c[1]
		if err == nil || err.Error() != agent.UnknownKindError(kind).Error() || !strings.HasPrefix(err.Error(), "unknown agent kind ") {
			t.Errorf("resolveNewAgent(%q, %q) error = %v, want the existing unknown-kind error", c[0], c[1], err)
		}
	}
	if _, err := resolveNewAgent("", "", registry); !errors.Is(err, errNoNewAgent) {
		t.Errorf("no agent anywhere: error = %v, want errNoNewAgent", err)
	}
}

func TestNewCreateInputDefaultsFromTheWorkingDirectory(t *testing.T) {
	wd := func() (string, error) { return "/repo/api", nil }
	in, err := newCreateInput(newCreateArgs{}, "copilot", wd)
	if err != nil || in.Name != "api" || in.CWD != "/repo/api" || in.Agent != "copilot" || in.PermissionProfile != "safe" {
		t.Fatalf("defaults = %+v, %v", in, err)
	}
	in, err = newCreateInput(newCreateArgs{name: "n", cwd: "/x/y", permission: "edits"}, "copilot", wd)
	if err != nil || in.Name != "n" || in.CWD != "/x/y" || in.PermissionProfile != "edits" {
		t.Fatalf("explicit = %+v, %v", in, err)
	}
}

func TestRunNewRefusesUnknownAgentWithTheExistingError(t *testing.T) {
	env := tempDeckEnv(t)
	code, _, stderr := runWithEnv(t, env, "new", "--agent", "ghost")
	if code != 2 || !strings.Contains(stderr, `error: unknown agent kind "ghost"`) {
		t.Fatalf("deck new --agent ghost: exit=%d stderr=%q, want 2 and the unknown-kind error", code, stderr)
	}
	if code, _, stderr = runWithEnv(t, env, "new", "--name", "x"); code != 2 || !strings.Contains(stderr, "needs --agent") {
		t.Fatalf("deck new --name x with no agent anywhere: exit=%d stderr=%q", code, stderr)
	}
	if code, _, stderr = runWithEnv(t, env, "new", "--nope"); code != 2 || !strings.Contains(stderr, "error: unknown flag --nope") {
		t.Fatalf("deck new --nope: exit=%d stderr=%q", code, stderr)
	}
}

func TestRunNewReadsTheConfigFileAgentKey(t *testing.T) {
	for _, c := range []struct {
		value    string
		wantErr  string
		wantExit int
	}{
		{"ghost", `error: unknown agent kind "ghost"`, 2},
		{"", "needs --agent", 2},
	} {
		env := tempDeckEnv(t)
		cfg := "agent = " + `"` + c.value + `"` + "\n"
		if err := os.WriteFile(filepath.Join(deckEnvValue(t, env, "DECK_HOME"), "config.toml"), []byte(cfg), 0o600); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := runWithEnv(t, env, "new", "--name", "x")
		if code != c.wantExit || !strings.Contains(stderr, c.wantErr) {
			t.Errorf("config agent=%q: exit=%d stderr=%q, want %d and %q", c.value, code, stderr, c.wantExit, c.wantErr)
		}
	}
}

// With copilot named by flag or by config, `deck new` gets as far as the create
// service, which refuses only because no copilot is on this PATH -- the same
// probe verdict a missing codex gets -- never as an unknown flag or kind.
func TestRunNewAcceptsCopilotAndReachesTheCreateService(t *testing.T) {
	emptyBin := t.TempDir()
	for _, viaConfig := range []bool{false, true} {
		env := tempDeckEnv(t)
		t.Setenv("PATH", emptyBin)
		args := []string{"new", "--agent", "copilot", "--cwd", t.TempDir()}
		if viaConfig {
			args = []string{"new", "--cwd", t.TempDir()}
			if err := os.WriteFile(filepath.Join(deckEnvValue(t, env, "DECK_HOME"), "config.toml"), []byte("agent = \"copilot\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		code, stdout, stderr := runWithEnv(t, env, args...)
		if code != 1 || stdout != "" {
			t.Fatalf("viaConfig=%v: exit=%d stdout=%q stderr=%q, want a creation failure (exit 1)", viaConfig, code, stdout, stderr)
		}
		if strings.Contains(stderr, "unknown") || !strings.HasPrefix(stderr, "deck new: ") || !strings.Contains(stderr, "copilot") {
			t.Fatalf("viaConfig=%v: stderr=%q, want the create service's copilot-not-found refusal", viaConfig, stderr)
		}
	}
}

func TestBareNewIsStillTheProfileNamedNew(t *testing.T) {
	env := tempDeckEnv(t)
	code, _, stderr := runWithEnv(t, env, "new")
	if code != 1 || !strings.Contains(stderr, `no profile "new" yet`) {
		t.Fatalf("bare deck new: exit=%d stderr=%q, want the profile-creation refusal", code, stderr)
	}
}

func TestNewAgentRegistryHoldsEveryShippedKind(t *testing.T) {
	want := []string{"claude", "codex", "copilot", "pi", "shell"}
	if got := newAgentRegistry().Kinds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
}

// R220.3, the success half: with a copilot on PATH, `deck new --agent copilot`
// creates a detached copilot session on a private tmux server, prints its name,
// and persists the kind and the requested permission profile.
func TestRunNewCreatesACopilotSession(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "copilot"), []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := tempDeckEnv(t)
	socket := "priv-new-" + strings.ReplaceAll(filepath.Base(deckEnvValue(t, env, "DECK_HOME")), "_", "")
	defer killTmuxServer(socket)
	env = append(env, "DECK_TMUX_SOCKET="+socket)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	cwd := t.TempDir()

	code, stdout, stderr := runWithEnv(t, env, "new", "--agent", "copilot", "--name", "cop-one", "--cwd", cwd, "--permission", "edits")
	if code != 0 || strings.TrimSpace(stdout) != "cop-one" {
		t.Fatalf("deck new --agent copilot: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
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
	sessions, err := db.ListSessions(context.Background())
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %v, %v, want exactly the one just created", sessions, err)
	}
	if got := sessions[0]; got.Agent != "copilot" || got.Name != "cop-one" || got.PermissionProfile != "edits" || got.CWD != cwd {
		t.Fatalf("persisted session = %+v", got)
	}
}
