package agent

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const copilotTestID = "7f1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"

// R216 item 1.
func TestCopilot_Capabilities(t *testing.T) {
	adapter := NewCopilot()
	if adapter.Kind() != "copilot" {
		t.Fatalf("Kind = %q, want copilot", adapter.Kind())
	}
	want := Caps{
		Profiles:              []string{"safe", "edits", "yolo"},
		AssignsConversationID: true,
		Resumable:             true,
		HasTranscript:         true,
		Executable:            "copilot",
		TranscriptEnvKeys:     []string{"COPILOT_HOME"},
	}
	if got := adapter.Capabilities(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Capabilities = %#v, want %#v", got, want)
	}
	if adapter.Capabilities().SupportsProfile("plan") {
		t.Fatal("copilot must not declare a plan profile")
	}
}

// R216 item 2: argv per profile, with ExtraArgs verbatim and last.
func TestCopilot_LaunchArgvPerProfile(t *testing.T) {
	cases := map[string][]string{
		"safe":  {"copilot", "--session-id", copilotTestID, "--no-auto-update"},
		"edits": {"copilot", "--session-id", copilotTestID, "--allow-tool=write", "--no-auto-update"},
		"yolo":  {"copilot", "--session-id", copilotTestID, "--allow-all", "--no-auto-update"},
	}
	extra := []string{"--model", "x y", "--flag=with space", ""}
	for profile, want := range cases {
		t.Run(profile, func(t *testing.T) {
			got, err := NewCopilot().Launch(LaunchInput{ConversationID: copilotTestID, Profile: profile})
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("Launch = %#v, %v; want %#v", got, err, want)
			}
			got, err = NewCopilot().Launch(LaunchInput{ConversationID: copilotTestID, Profile: profile, ExtraArgs: extra})
			if err != nil || !reflect.DeepEqual(got, append(append([]string{}, want...), extra...)) {
				t.Fatalf("Launch with ExtraArgs = %#v, %v", got, err)
			}
		})
	}
	if _, err := NewCopilot().Launch(LaunchInput{ConversationID: copilotTestID, Profile: "plan"}); err == nil {
		t.Fatal("Launch accepted the unsupported profile plan")
	}
}

// R216 item 3: Resume is Launch's argv for every profile, ExtraArgs included.
func TestCopilot_ResumeEqualsLaunch(t *testing.T) {
	extra := []string{"--model", "gpt", "--verbose"}
	for _, profile := range copilotProfiles {
		for _, args := range [][]string{nil, extra} {
			launch, err := NewCopilot().Launch(LaunchInput{ConversationID: copilotTestID, Profile: profile, ExtraArgs: args})
			if err != nil {
				t.Fatal(err)
			}
			resume, err := NewCopilot().Resume(ResumeInput{ConversationID: copilotTestID, Profile: profile, ExtraArgs: args})
			if err != nil || !reflect.DeepEqual(resume, launch) {
				t.Fatalf("Resume(%s, %v) = %#v, %v; want Launch's %#v", profile, args, resume, err, launch)
			}
			if len(args) > 0 && !reflect.DeepEqual(resume[len(resume)-len(args):], args) {
				t.Fatalf("Resume(%s) lost ExtraArgs: %#v", profile, resume)
			}
		}
	}
	if _, err := NewCopilot().Resume(ResumeInput{ConversationID: "not-a-uuid", Profile: "safe"}); err == nil {
		t.Fatal("Resume accepted a non-UUID id")
	}
	if _, err := NewCopilot().Resume(ResumeInput{ConversationID: copilotTestID, Profile: "plan"}); err == nil {
		t.Fatal("Resume accepted the unsupported profile plan")
	}
}

// R216 item 3: no argv carries a forbidden flag, and the adapter never asks
// for a fresh relaunch. Modelled on TestCodex_NeverEmitsForbiddenFlags.
func TestCopilot_NeverEmitsForbiddenFlags(t *testing.T) {
	forbidden := []string{"--continue", "--resume", "--connect", "--remote", "--acp", "--yolo"}
	check := func(t *testing.T, label string, argv []string) {
		t.Helper()
		joined := strings.Join(argv, " ")
		for _, bad := range forbidden {
			for _, a := range argv {
				if a == bad || strings.HasPrefix(a, bad+"=") {
					t.Fatalf("%s argv %v contains forbidden flag %q", label, argv, bad)
				}
			}
			if strings.Contains(joined, bad) {
				t.Fatalf("%s argv %v contains forbidden substring %q", label, argv, bad)
			}
		}
	}
	for _, profile := range copilotProfiles {
		t.Run(profile, func(t *testing.T) {
			launch, err := NewCopilot().Launch(LaunchInput{CWD: "/tmp/proj", ConversationID: copilotTestID, Profile: profile})
			if err != nil {
				t.Fatal(err)
			}
			check(t, "Launch("+profile+")", launch)
			resume, err := NewCopilot().Resume(ResumeInput{CWD: "/tmp/proj", ConversationID: copilotTestID, Profile: profile})
			if err != nil {
				t.Fatal(err)
			}
			check(t, "Resume("+profile+")", resume)
			instrument, _ := NewCopilot().Instrument(LaunchInput{CWD: "/tmp/proj", ConversationID: copilotTestID, Profile: profile})
			check(t, "Instrument("+profile+")", instrument)
		})
	}
	if _, ok := Adapter(NewCopilot()).(FreshRelauncher); ok {
		t.Fatal("Copilot implements FreshRelauncher, want it always relaunched with Resume")
	}
}

// R216 item 4: every TranscriptPaths branch.
func TestCopilot_TranscriptPaths(t *testing.T) {
	record := func(t *testing.T, root string) string {
		t.Helper()
		dir := filepath.Join(root, "session-state", copilotTestID)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "events.jsonl")
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	adapter := NewCopilot()

	t.Run("COPILOT_HOME wins over Home", func(t *testing.T) {
		home, override := t.TempDir(), t.TempDir()
		record(t, filepath.Join(home, ".copilot"))
		want := record(t, override)
		got, ok := adapter.TranscriptPaths(TranscriptInput{Home: home, ConversationID: copilotTestID, Env: map[string]string{"COPILOT_HOME": override}})
		if !ok || got != want {
			t.Fatalf("got %q, %v; want %q", got, ok, want)
		}
	})
	t.Run("Home/.copilot fallback", func(t *testing.T) {
		for name, env := range map[string]map[string]string{"nil env": nil, "empty COPILOT_HOME": {"COPILOT_HOME": ""}} {
			home := t.TempDir()
			want := record(t, filepath.Join(home, ".copilot"))
			got, ok := adapter.TranscriptPaths(TranscriptInput{Home: home, ConversationID: copilotTestID, Env: env})
			if !ok || got != want {
				t.Fatalf("%s: got %q, %v; want %q", name, got, ok, want)
			}
		}
	})
	t.Run("unknown Home and no COPILOT_HOME", func(t *testing.T) {
		if got, ok := adapter.TranscriptPaths(TranscriptInput{ConversationID: copilotTestID}); ok || got != "" {
			t.Fatalf("got %q, %v; want a decline", got, ok)
		}
	})
	t.Run("unknown Home with COPILOT_HOME resolves", func(t *testing.T) {
		override := t.TempDir()
		want := record(t, override)
		if got, ok := adapter.TranscriptPaths(TranscriptInput{ConversationID: copilotTestID, Env: map[string]string{"COPILOT_HOME": override}}); !ok || got != want {
			t.Fatalf("got %q, %v; want %q", got, ok, want)
		}
	})
	t.Run("id that is not one path component", func(t *testing.T) {
		home := t.TempDir()
		record(t, filepath.Join(home, ".copilot"))
		// Files a traversing or multi-component id would reach if it were joined.
		for _, rel := range []string{"session-state/a/b", "x"} {
			dir := filepath.Join(home, ".copilot", rel)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		for _, id := range []string{"", ".", "..", "a/b", `a\b`, "../x", "../" + copilotTestID, copilotTestID + "/x"} {
			if got, ok := adapter.TranscriptPaths(TranscriptInput{Home: home, ConversationID: id}); ok || got != "" {
				t.Fatalf("id %q: got %q, %v; want a decline", id, got, ok)
			}
		}
	})
	t.Run("missing file", func(t *testing.T) {
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".copilot", "session-state", copilotTestID), 0o700); err != nil {
			t.Fatal(err)
		}
		if got, ok := adapter.TranscriptPaths(TranscriptInput{Home: home, ConversationID: copilotTestID}); ok || got != "" {
			t.Fatalf("got %q, %v; want a decline", got, ok)
		}
	})
	t.Run("events.jsonl that is a directory", func(t *testing.T) {
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".copilot", "session-state", copilotTestID, "events.jsonl"), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, ok := adapter.TranscriptPaths(TranscriptInput{Home: home, ConversationID: copilotTestID}); ok {
			t.Fatal("a directory is not a transcript")
		}
	})
}

// R216 item 5: a non-UUID conversation id is deck's error, not a dead pane's.
func TestCopilot_LaunchRejectsNonUUIDConversationID(t *testing.T) {
	for _, id := range []string{"", "not-a-uuid", "7f1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4", copilotTestID + "0", " " + copilotTestID, "7f1b2c3d4e5f4a6b8c7d9e0f1a2b3c4d", "7f1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4z"} {
		for _, profile := range copilotProfiles {
			argv, err := NewCopilot().Launch(LaunchInput{ConversationID: id, Profile: profile})
			if err == nil || argv != nil {
				t.Fatalf("Launch(%q, %s) = %v, %v; want an error and no argv", id, profile, argv, err)
			}
		}
	}
	if _, err := NewCopilot().Launch(LaunchInput{ConversationID: strings.ToUpper(copilotTestID), Profile: "safe"}); err != nil {
		t.Fatalf("an upper-case UUID is a UUID: %v", err)
	}
}
