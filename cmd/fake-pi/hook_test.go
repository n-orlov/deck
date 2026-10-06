package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
)

// hookScript writes an executable stand-in for the deck binary at a path with
// a space, a quote and a ';' in it, running body with the argv it was given
// in $1, and returns that path.
func hookScript(t *testing.T, body string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "deck builds", "it's; here")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "deck")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n[ \"$1\" = _hook ] && [ $# -eq 1 ] || exit 64\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

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
	hooks := extensionHooks{extension: writeExtension(t), sessionID: "conv-1", executable: hookScript(t, `cat >> '`+record+`'`)}
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
		"no extension loaded":    {executable: hookScript(t, "true")},
		"unreadable extension":   {extension: filepath.Join(t.TempDir(), "missing.js"), executable: hookScript(t, "true")},
		"event not subscribed":   {extension: writeExtension(t), executable: hookScript(t, "true")},
		"empty extension source": {extension: func() string { p := filepath.Join(t.TempDir(), "e.js"); _ = os.WriteFile(p, nil, 0o600); return p }(), executable: hookScript(t, "true")},
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
	hooks := extensionHooks{extension: writeExtension(t), executable: hookScript(t, "echo 'restart the session from deck' >&2; exit 1")}
	if err := hooks.fire(&out, "Stop", nil); err != nil {
		t.Fatalf("fire: %v", err)
	}
	if want := "fake-pi hook failed: Stop\nfake-pi notify: restart the session from deck\n"; out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestHookPaneCommandPlaysTheExtensionAndUnknownCommandsStillFail(t *testing.T) {
	var out bytes.Buffer
	hooks := extensionHooks{extension: writeExtension(t), sessionID: "c", executable: hookScript(t, "cat >/dev/null")}
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
		if key == agent.PiHookExecutableEnv {
			return "/deck"
		}
		return ""
	}
	got := newExtensionHooks(options{sessionID: "s", extension: "/e.js"}, getenv)
	if got.executable != "/deck" || got.sessionID != "s" || got.extension != "/e.js" {
		t.Fatalf("newExtensionHooks = %+v", got)
	}
}

// TestRunHookCommandRunsTheExecutableWithItsPayloadAndNoShell covers an
// executable whose path holds a space, a quote and a ';' (hookScript's), a
// hook that reads its stdin, one that fails with stderr, and one the system
// cannot start.
func TestRunHookCommandRunsTheExecutableWithItsPayloadAndNoShell(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.txt")
	var stderr bytes.Buffer
	if err := runHookCommand(hookScript(t, "cat > '"+out+"' && echo done >&2"), []byte("payload\n"), &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, err := os.ReadFile(out); err != nil || string(got) != "payload\n" || stderr.String() != "done\n" {
		t.Fatalf("stdin payload = %q (%v), stderr %q", got, err, stderr.String())
	}
	stderr.Reset()
	if err := runHookCommand(hookScript(t, "echo boom >&2; exit 3"), nil, &stderr); err == nil || stderr.String() != "boom\n" {
		t.Fatalf("failing hook = %v, stderr %q", err, stderr.String())
	}
	if err := runHookCommand(filepath.Join(t.TempDir(), "no-such-deck"), nil, &bytes.Buffer{}); err == nil {
		t.Fatal("a missing executable must be an error")
	}
}
