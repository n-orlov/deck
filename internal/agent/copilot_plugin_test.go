package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// R217: Instrument returns --plugin-dir <dir under DeckHome> and DECK_EXE for
// every profile, and COPILOT_ALLOW_ALL=true for yolo only.
func TestCopilotInstrumentPluginDirAndEnvPerProfile(t *testing.T) {
	for _, profile := range []string{"safe", "edits", "yolo"} {
		t.Run(profile, func(t *testing.T) {
			in := LaunchInput{Profile: profile, DeckExecutable: "/opt/deck/bin/deck", DeckHome: "/data/deck"}
			argv, env := NewCopilot().Instrument(in)
			if want := []string{"--plugin-dir", "/data/deck/copilot/plugin"}; !reflect.DeepEqual(argv, want) {
				t.Fatalf("argv = %#v, want %#v", argv, want)
			}
			want := map[string]string{"DECK_EXE": "/opt/deck/bin/deck"}
			if profile == "yolo" {
				want["COPILOT_ALLOW_ALL"] = "true"
			}
			if !reflect.DeepEqual(env, want) {
				t.Fatalf("env = %#v, want %#v", env, want)
			}
			if rel, err := filepath.Rel(in.DeckHome, argv[1]); err != nil || strings.HasPrefix(rel, "..") {
				t.Fatalf("plugin dir %q is not under DeckHome %q", argv[1], in.DeckHome)
			}
		})
	}
}

func TestCopilotInstrumentCarriesTheLaunchGenerationOnlyWhenHeld(t *testing.T) {
	in := LaunchInput{Profile: "safe", DeckExecutable: "/d", DeckHome: "/h"}
	if _, env := NewCopilot().Instrument(in); env[LaunchGenerationEnv] != "" {
		t.Fatalf("env without a lease carries %q", env[LaunchGenerationEnv])
	}
	in.LaunchGeneration = "gen-7"
	if _, env := NewCopilot().Instrument(in); env[LaunchGenerationEnv] != "gen-7" {
		t.Fatalf("env = %#v, want the launch generation", env)
	}
}

type copilotHookEntry struct {
	Type       string `json:"type"`
	Bash       string `json:"bash"`
	TimeoutSec int    `json:"timeoutSec"`
}

func parseCopilotHooks(t *testing.T) map[string][]copilotHookEntry {
	t.Helper()
	var doc struct {
		Version int                           `json:"version"`
		Hooks   map[string][]copilotHookEntry `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(CopilotHooksConfig()), &doc); err != nil {
		t.Fatalf("hooks.json is not JSON: %v", err)
	}
	if doc.Version != 1 {
		t.Fatalf("hooks.json version = %d, want 1", doc.Version)
	}
	return doc.Hooks
}

// R217: the constant plugin content subscribes to exactly the six observational
// events, each with a 5 s timeout, and never to permissionRequest/preToolUse.
func TestCopilotPluginContentSubscribesToExactlyTheSixEvents(t *testing.T) {
	var manifest map[string]string
	if err := json.Unmarshal([]byte(CopilotPluginManifest), &manifest); err != nil {
		t.Fatalf("plugin.json is not JSON: %v", err)
	}
	if manifest["name"] == "" || manifest["hooks"] != "hooks.json" {
		t.Fatalf("plugin.json = %#v, want a named plugin pointing at hooks.json", manifest)
	}
	hooks := parseCopilotHooks(t)
	want := []string{"agentStop", "errorOccurred", "notification", "sessionEnd", "sessionStart", "userPromptSubmitted"}
	var got []string
	for event, entries := range hooks {
		got = append(got, event)
		if len(entries) != 1 || entries[0].Type != "command" || entries[0].TimeoutSec != 5 {
			t.Errorf("%s entries = %#v, want one command entry with a 5 s timeout", event, entries)
		}
		if !strings.Contains(entries[0].Bash, `"$DECK_EXE" _hook`) {
			t.Errorf("%s command %q does not run \"$DECK_EXE\" _hook", event, entries[0].Bash)
		}
		if !strings.Contains(entries[0].Bash, "DECK_HOOK_EVENT="+event+" ") {
			t.Errorf("%s command %q does not name its own event", event, entries[0].Bash)
		}
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("subscribed events = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(append([]string(nil), CopilotHookEvents...), []string{"userPromptSubmitted", "sessionStart", "notification", "agentStop", "errorOccurred", "sessionEnd"}) {
		t.Fatalf("CopilotHookEvents = %v", CopilotHookEvents)
	}
	for _, forbidden := range []string{"permissionRequest", "preToolUse"} {
		if _, ok := hooks[forbidden]; ok {
			t.Errorf("hooks.json registers %s", forbidden)
		}
		if strings.Contains(CopilotHooksConfig(), forbidden) || strings.Contains(CopilotPluginManifest, forbidden) {
			t.Errorf("plugin content mentions %s", forbidden)
		}
	}
}

// R217: every entry, run under bash with DECK_EXE unset, prints nothing and
// exits 0; with a DECK_EXE that is noisy and fails, still nothing and 0; with a
// working DECK_EXE it receives `_hook`, the event name and the stdin payload.
func TestCopilotPluginHookCommandsAreSilentAndAlwaysExitZero(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not available")
	}
	dir := t.TempDir()
	noisy := filepath.Join(dir, "noisy")
	record := filepath.Join(dir, "record")
	script := "#!/bin/sh\necho stdout-noise\necho stderr-noise >&2\necho \"$1 $DECK_HOOK_EVENT $(cat)\" >> " + record + "\nexit 3\n"
	if err := os.WriteFile(noisy, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	hooks := parseCopilotHooks(t)
	for _, event := range CopilotHookEvents {
		entry := hooks[event][0]
		for name, exe := range map[string]string{"unset": "", "empty": "-", "missing": filepath.Join(dir, "absent"), "failing": noisy} {
			cmd := exec.Command(bash, "-c", entry.Bash)
			cmd.Env = []string{"PATH=/usr/bin:/bin"}
			switch name {
			case "empty":
				cmd.Env = append(cmd.Env, "DECK_EXE=")
			case "unset":
			default:
				cmd.Env = append(cmd.Env, "DECK_EXE="+exe)
			}
			cmd.Stdin = strings.NewReader(`{"sessionId":"s"}`)
			var stdout bytes.Buffer
			cmd.Stdout = &stdout
			if err := cmd.Run(); err != nil {
				t.Errorf("%s/DECK_EXE %s: exit error %v, want exit 0", event, name, err)
			}
			if stdout.Len() != 0 {
				t.Errorf("%s/DECK_EXE %s: stdout = %q, want empty", event, name, stdout.String())
			}
		}
	}
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("the failing DECK_EXE never ran: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for i, event := range CopilotHookEvents {
		if want := `_hook ` + event + ` {"sessionId":"s"}`; lines[i] != want {
			t.Errorf("hook %d recorded %q, want %q", i, lines[i], want)
		}
	}
}

func TestCopilotInstrumentFilesAreTheConstantPluginUnderTheDataRoot(t *testing.T) {
	files := NewCopilot().InstrumentFiles(LaunchInput{DeckHome: "/data/deck"})
	if len(files) != 2 {
		t.Fatalf("files = %#v, want plugin.json and hooks.json", files)
	}
	dir := CopilotPluginDir("/data/deck")
	byName := map[string]InstrumentFile{}
	for _, f := range files {
		byName[f.Path] = f
		if !f.Optional || f.DirMode != 0o700 || !reflect.DeepEqual(f.DropArgv, []string{"--plugin-dir", dir}) {
			t.Errorf("%s: %#v, want an optional file in a 0700 dir dropping --plugin-dir", f.Path, f)
		}
	}
	if string(byName[filepath.Join(dir, "plugin.json")].Content) != CopilotPluginManifest ||
		string(byName[filepath.Join(dir, "hooks.json")].Content) != CopilotHooksConfig() {
		t.Fatalf("file content is not the plugin constants")
	}
	if got := NewCopilot().InstrumentFiles(LaunchInput{}); len(got) != 0 {
		t.Fatalf("files without a data root = %#v", got)
	}
	var _ FileInstrumenter = Copilot{}
}
