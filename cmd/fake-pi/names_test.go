package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
)

// namesEnvironment is a launch environment with a private HOME and cwd, plus a
// hook executable recording every payload it receives.
func namesEnvironment(t *testing.T) (getenv func(string) string, getwd func() (string, error), payloads string) {
	t.Helper()
	home, cwd := t.TempDir(), t.TempDir()
	payloads = filepath.Join(t.TempDir(), "payloads")
	env := map[string]string{
		"HOME": home, commandsEnvironment: "1", agent.PiHookExecutableEnv: hookScript(t, `cat >> '`+payloads+`'`),
		agent.PiControlDirEnv: agent.PiControlDir(home),
	}
	return func(k string) string { return env[k] }, func() (string, error) { return cwd, nil }, payloads
}

// sessionEntries decodes every line of the one session file under HOME.
func sessionEntries(t *testing.T, getenv func(string) string) []map[string]any {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(getenv("HOME"), ".pi", "agent", "sessions", "*", "*_conv-1.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("session files = %v, %v", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var entries []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("session line %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func runFake(t *testing.T, getenv func(string) string, getwd func() (string, error), commands string, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	args = append([]string{"--session-id", "conv-1", "-e", writeExtension(t)}, args...)
	if _, err := runWithIO(args, strings.NewReader(commands), &out, getenv, getwd); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// --name is persisted the way Pi does at startup: a session_info entry after
// the header, in Pi's record shape, and no session_info_changed hook fires.
func TestPiSessionInfoRecordIsWrittenForTheLaunchName(t *testing.T) {
	getenv, getwd, payloads := namesEnvironment(t)
	runFake(t, getenv, getwd, "", "--name", "billing audit")
	entries := sessionEntries(t, getenv)
	if len(entries) != 2 || entries[0]["type"] != "session" {
		t.Fatalf("entries = %v, want the header then one session_info", entries)
	}
	info := entries[1]
	if info["type"] != "session_info" || info["name"] != "billing audit" || info["id"] != "00000001" || info["parentId"] != nil {
		t.Errorf("session_info = %v", info)
	}
	if ts, _ := info["timestamp"].(string); !strings.HasSuffix(ts, "Z") {
		t.Errorf("timestamp = %v, want ISO-8601 UTC", info["timestamp"])
	}
	if _, err := os.Stat(payloads); err == nil {
		t.Error("a launch --name fired a hook; real Pi's does not fire session_info_changed")
	}
}

// A launch without --name writes no entry (Pi never names a session itself),
// and a resume that passes --name again appends a second entry chained to the first.
func TestPiSessionInfoRecordIsNotInventedAndChains(t *testing.T) {
	getenv, getwd, _ := namesEnvironment(t)
	runFake(t, getenv, getwd, "")
	if entries := sessionEntries(t, getenv); len(entries) != 0 {
		// Without --name no transcript is created before the run ends; the header alone is fine.
		for _, e := range entries {
			if e["type"] == "session_info" {
				t.Fatalf("an unnamed launch wrote %v", e)
			}
		}
	}
	runFake(t, getenv, getwd, "", "--name", "first")
	runFake(t, getenv, getwd, "", "--name", "second")
	entries := sessionEntries(t, getenv)
	last := entries[len(entries)-1]
	if last["name"] != "second" || last["id"] != "00000002" || last["parentId"] != "00000001" {
		t.Errorf("last entry = %v, want second chained to the first", last)
	}
}

// A name change inside Pi (the command or a typed /name line) appends an entry
// and reports it; start reports the held name as session_name.
func TestPiNameChangesPersistAndStartReportsTheHeldName(t *testing.T) {
	getenv, getwd, payloads := namesEnvironment(t)
	out := runFake(t, getenv, getwd,
		`{"command":"start"}`+"\n"+`{"command":"name","name":"Inside Pi"}`+"\n"+"\x15/name typed in pi\n"+`{"command":"name","name":""}`+"\n",
		"--name", "launch")
	entries := sessionEntries(t, getenv)
	var names []any
	for _, e := range entries[1:] {
		names = append(names, e["name"])
	}
	if want := []any{"launch", "Inside Pi", "typed in pi", ""}; !equalAny(names, want) {
		t.Errorf("session_info names = %v, want %v", names, want)
	}
	recorded, _ := os.ReadFile(payloads)
	for _, want := range []string{
		`"hook_event_name":"SessionStart","session_id":"conv-1","session_name":"launch","source":"startup"`,
		`"name":"typed in pi"`,
		`"cleared":true,"hook_event_name":"session_info_changed","name":""`,
	} {
		if !strings.Contains(string(recorded), want) {
			t.Errorf("payloads lack %s:\n%s", want, recorded)
		}
	}
	if !strings.Contains(out, "fake-pi hook fired: SessionStart\n") {
		t.Errorf("output = %q", out)
	}
}

// A session with no name starts without session_name.
func TestPiStartOmitsSessionNameWhenUnnamed(t *testing.T) {
	getenv, getwd, payloads := namesEnvironment(t)
	runFake(t, getenv, getwd, `{"command":"start"}`+"\n")
	recorded, _ := os.ReadFile(payloads)
	if strings.Contains(string(recorded), "session_name") {
		t.Errorf("an unnamed start carried a session_name: %s", recorded)
	}
}

// Deck's push through the control file is Pi's setSessionName: it persists.
func TestPiControlFilePushPersistsASessionInfoEntry(t *testing.T) {
	getenv, getwd, _ := namesEnvironment(t)
	home := getenv("HOME")
	stdin, commands := io.Pipe()
	out := &syncBuffer{}
	done := make(chan error, 1)
	go func() {
		_, err := runWithIO([]string{"--session-id", "conv-1", "-e", writeExtension(t)}, stdin, out, getenv, getwd)
		done <- err
	}()
	if _, err := io.WriteString(commands, `{"command":"start"}`+"\n"); err != nil {
		t.Fatal(err)
	}
	file, _ := agent.PiControlFile(home, "conv-1")
	waitFor(t, "the control file", func() bool { _, err := os.Stat(file); return err == nil })
	if delivered, err := (agent.Pi{}).PushControl(agent.ControlPushInput{DeckHome: home, ConversationID: "conv-1", Name: "pushed"}); err != nil || !delivered {
		t.Fatalf("PushControl = %v, %v", delivered, err)
	}
	waitFor(t, "the pushed name", func() bool { return strings.Contains(out.String(), "fake-pi name: pushed\n") })
	_ = commands.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	entries := sessionEntries(t, getenv)
	if last := entries[len(entries)-1]; last["type"] != "session_info" || last["name"] != "pushed" {
		t.Errorf("last entry = %v, want session_info named pushed", last)
	}
}

func TestTypedNameRecognisesOnlyTheNameCommand(t *testing.T) {
	for line, want := range map[string]string{"/name x y": "x y", "\x15/name x": "x", "/name": ""} {
		if got, ok := typedName([]byte(line)); !ok || got != want {
			t.Errorf("typedName(%q) = %q, %v; want %q", line, got, ok, want)
		}
	}
	for _, line := range []string{`{"command":"name"}`, "/names x", "hello"} {
		if _, ok := typedName([]byte(line)); ok {
			t.Errorf("typedName(%q) matched", line)
		}
	}
}

func equalAny(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
