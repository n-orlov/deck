package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
)

func writeExtension(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "deck-hook.js")
	if err := os.WriteFile(path, []byte(agent.PiExtensionSource), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseTakesTheExtensionFlagInBothSpellings(t *testing.T) {
	for _, flag := range []string{"-e", "--extension"} {
		got, err := parse([]string{"--session-id", "s1", flag, "/x/ext.js", "hello"})
		if err != nil || got.extension != "/x/ext.js" || got.sessionID != "s1" || got.message != "hello" {
			t.Errorf("parse with %s = (%+v, %v)", flag, got, err)
		}
		if _, err := parse([]string{flag}); err == nil {
			t.Errorf("parse(%s) with no value succeeded", flag)
		}
	}
}

// The hook command runs with the payload on stdin, the session id the pi was
// launched with, and the extension's event name -- never as "deck _hook"
// directly -- for every event the extension subscribes.
func TestHookCommandRunsTheLaunchHookCommandForEverySubscribedEvent(t *testing.T) {
	record := filepath.Join(t.TempDir(), "payloads")
	hooks := extensionHooks{extension: writeExtension(t), sessionID: "conv-1", command: "cat >> '" + record + "'"}
	for _, event := range agent.PiHookEvents {
		var out bytes.Buffer
		if err := hooks.fire(&out, event, map[string]any{"reason": "x"}); err != nil {
			t.Fatalf("fire %s: %v", event, err)
		}
		if out.String() != "fake-pi hook fired: "+event+"\n" {
			t.Errorf("%s output = %q", event, out.String())
		}
	}
	recorded, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range agent.PiHookEvents {
		if want := `"hook_event_name":"` + event + `"`; !strings.Contains(string(recorded), want) {
			t.Errorf("payloads lack %s: %s", want, recorded)
		}
	}
	if strings.Count(string(recorded), `"session_id":"conv-1"`) != len(agent.PiHookEvents) {
		t.Errorf("session id missing from payloads: %s", recorded)
	}
}

func TestHookCommandRefusesAnEventTheExtensionNeverInstalled(t *testing.T) {
	var out bytes.Buffer
	for name, hooks := range map[string]extensionHooks{
		"no extension loaded":    {command: "true"},
		"unreadable extension":   {extension: filepath.Join(t.TempDir(), "missing.js"), command: "true"},
		"event not subscribed":   {extension: writeExtension(t), command: "true"},
		"empty extension source": {extension: func() string { p := filepath.Join(t.TempDir(), "e.js"); _ = os.WriteFile(p, nil, 0o600); return p }(), command: "true"},
	} {
		event := "Notification"
		if name == "empty extension source" {
			event = "SessionStart"
		}
		if err := hooks.fire(&out, event, nil); err == nil {
			t.Errorf("%s: fire succeeded", name)
		}
	}
}

func TestHookCommandWithoutALaunchCommandDoesNothing(t *testing.T) {
	var out bytes.Buffer
	if err := (extensionHooks{extension: writeExtension(t)}).fire(&out, "SessionStart", nil); err != nil || out.Len() != 0 {
		t.Fatalf("fire = %v, output %q; want a silent no-op like the extension's own early return", err, out.String())
	}
}

// A failing hook shows its stderr as a notification and never stops the fixture.
func TestFailingHookShowsItsStderrAndCarriesOn(t *testing.T) {
	var out bytes.Buffer
	hooks := extensionHooks{extension: writeExtension(t), command: "echo 'restart the session from deck' >&2; exit 1"}
	if err := hooks.fire(&out, "Stop", nil); err != nil {
		t.Fatalf("fire: %v", err)
	}
	if want := "fake-pi hook failed: Stop\nfake-pi notify: restart the session from deck\n"; out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestHookPaneCommandPlaysTheExtensionAndUnknownCommandsStillFail(t *testing.T) {
	var out bytes.Buffer
	hooks := extensionHooks{extension: writeExtension(t), sessionID: "c", command: "cat >/dev/null"}
	err := runCommands(strings.NewReader(`{"command":"hook","event":"SessionStart","payload":{"source":"startup"}}`+"\n"), &out, "", hooks)
	if err != nil || out.String() != "fake-pi hook fired: SessionStart\n" {
		t.Fatalf("runCommands = %v, output %q", err, out.String())
	}
	if err := runCommands(strings.NewReader(`{"command":"nope"}`+"\n"), &out, "", hooks); err == nil {
		t.Fatal("unknown command succeeded")
	}
}

func TestNewExtensionHooksReadsTheLaunchEnvironment(t *testing.T) {
	getenv := func(key string) string {
		if key == agent.PiHookCommandEnv {
			return "'/deck' _hook"
		}
		return ""
	}
	got := newExtensionHooks(options{sessionID: "s", extension: "/e.js"}, getenv)
	if got.command != "'/deck' _hook" || got.sessionID != "s" || got.extension != "/e.js" {
		t.Fatalf("newExtensionHooks = %+v", got)
	}
}

// TestRunHookCommandHandsTheLineToTheShellVerbatim covers cases beyond the
// launch's own command: a line with quoting and spaces, a line that reads its
// stdin, and a line that exits non-zero with stderr output.
func TestRunHookCommandHandsTheLineToTheShellVerbatim(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out file.txt")
	cases := []struct {
		name    string
		command string
		stderr  string
		failure bool
	}{
		{name: "quoted path with a space", command: `cat > '` + out + `'`},
		{name: "shell operators stay in the line", command: `cat > '` + out + `' && echo done >&2`, stderr: "done\n"},
		{name: "non-zero exit with stderr", command: `echo boom >&2; exit 3`, stderr: "boom\n", failure: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			err := runHookCommand("sh", tc.command, []byte("payload\n"), &stderr)
			if (err != nil) != tc.failure {
				t.Fatalf("err = %v, want failure=%v", err, tc.failure)
			}
			if stderr.String() != tc.stderr {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tc.stderr)
			}
			if !tc.failure {
				got, readErr := os.ReadFile(out)
				if readErr != nil || string(got) != "payload\n" {
					t.Fatalf("stdin payload = %q (%v)", got, readErr)
				}
			}
		})
	}
}

func TestRunHookCommandReportsAShellThatCannotStart(t *testing.T) {
	if err := runHookCommand(filepath.Join(t.TempDir(), "no-such-shell"), "true", nil, &bytes.Buffer{}); err == nil {
		t.Fatal("a missing shell must be an error")
	}
}
