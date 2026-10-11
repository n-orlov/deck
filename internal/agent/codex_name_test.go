package agent

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func codexScreen(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "probes", "codex", name+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestCodexRenamePlan: the plan is the typed command, then a lone Enter, and
// the declared gap between them is the ~0.7 s of R257.
func TestCodexRenamePlan(t *testing.T) {
	keys, ok := PushName(NewCodex(), NamePushInput{Name: "fix login"})
	if !ok || !reflect.DeepEqual(keys, []string{"/rename fix login", "Enter"}) {
		t.Fatalf("plan = %v %v", keys, ok)
	}
	if gap := PushGap(NewCodex()); gap != 700*time.Millisecond {
		t.Errorf("PushGap = %s, want 700ms", gap)
	}
	if _, ok := PushName(NewCodex(), NamePushInput{Name: " \x1b[0m\n "}); ok {
		t.Error("a name with nothing left was not declined")
	}
	if keys, _ := PushName(NewCodex(), NamePushInput{Name: "a\nb\x15c"}); keys[0] != "/rename a bc" {
		t.Errorf("control characters survived: %q", keys[0])
	}
}

// TestCodexRenameCapsAreDeclared: R257 declares the launch and push channels.
func TestCodexRenameCapsAreDeclared(t *testing.T) {
	caps := NewCodex().Capabilities().Name
	if caps.LaunchName != NameLaunchSendKeysAfterReady || caps.PushRename != NamePushSendKeys {
		t.Errorf("Codex NameCaps = %#v", caps)
	}
	if hasNameFlag(mustLaunch(t, NewCodex(), "hello")) {
		t.Error("Codex has no name flag, yet argv carries one")
	}
}

func mustLaunch(t *testing.T, a Adapter, name string) []string {
	t.Helper()
	argv, err := a.Launch(LaunchInput{CWD: "/w", Profile: "safe", Name: name})
	if err != nil {
		t.Fatal(err)
	}
	return argv
}

// TestCodexRenamePromptReady: ready is the probe seeing the empty composer;
// a working turn, an error, an approval, a draft and a modal are not.
func TestCodexRenamePromptReady(t *testing.T) {
	update := "✨ Update available! 0.154.0 -> 0.156.1\n 1. Update now\n 3. Skip until next version\n"
	trust := "Do you trust the contents of this directory?\n 1. Yes, continue\n 2. No, quit\n"
	for _, tc := range []struct {
		name   string
		screen string
		ready  bool
	}{
		{"idle", codexScreen(t, "idle"), true},
		{"first launch", codexScreen(t, "starting"), true},
		{"running", codexScreen(t, "running"), false},
		{"error", codexScreen(t, "error"), false},
		{"approval", codexScreen(t, "waiting"), false},
		{"update modal over the composer", update + codexScreen(t, "starting"), false},
		{"trust modal", trust, false},
		{"blank screen", "", false},
		{"draft typed (placeholder gone)", strings.ReplaceAll(codexScreen(t, "idle"), "Ask Codex to do anything", "my half typed prompt"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, gated := PromptReady(NewCodex(), tc.screen); !gated || got != tc.ready {
				t.Errorf("PromptReady = %v (gated %v), want %v", got, gated, tc.ready)
			}
		})
	}
	if !DialogFocused(NewCodex(), update) || !DialogFocused(NewCodex(), trust) || DialogFocused(NewCodex(), codexScreen(t, "idle")) {
		t.Error("DialogFocused does not separate the update/trust modals from the composer")
	}
}
