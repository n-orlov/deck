package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/agent"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestPiControlFileRoundTrip: a name written through the deck push path
// (agent.Pi.PushControl) reaches the fake Pi that opened the control file as
// the extension's session_start does, is delivered as Pi's own setSessionName
// (the pane shows it and the session_info_changed hook fires with it), and the
// file goes away at shutdown so the channel reads as unavailable again.
func TestPiControlFileRoundTrip(t *testing.T) {
	home := t.TempDir()
	record := filepath.Join(t.TempDir(), "payloads")
	hook := hookScript(t, `cat >> '`+record+`'`)
	env := map[string]string{
		commandsEnvironment: "1", agent.PiHookExecutableEnv: hook, agent.PiControlDirEnv: agent.PiControlDir(home),
	}
	stdin, closeStdin := io.Pipe()
	out := &syncBuffer{}
	done := make(chan error, 1)
	go func() {
		_, err := runWithIO([]string{"--session-id", "conv-1", "-e", writeExtension(t)}, stdin, out,
			func(k string) string { return env[k] }, func() (string, error) { return t.TempDir(), nil })
		done <- err
	}()
	file, _ := agent.PiControlFile(home, "conv-1")
	waitFor(t, "the control file", func() bool { _, err := os.Stat(file); return err == nil })

	delivered, err := agent.Pi{}.PushControl(agent.ControlPushInput{DeckHome: home, ConversationID: "conv-1", Name: "pushed from deck"})
	if err != nil || !delivered {
		t.Fatalf("PushControl = %v, %v", delivered, err)
	}
	waitFor(t, "the pushed name", func() bool { return strings.Contains(out.String(), "fake-pi name: pushed from deck\n") })
	waitFor(t, "the echo hook", func() bool {
		data, _ := os.ReadFile(record)
		return strings.Contains(string(data), `"hook_event_name":"session_info_changed","name":"pushed from deck","session_id":"conv-1"`)
	})

	_ = closeStdin.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("control file survived shutdown: %v", err)
	}
	if delivered, err := (agent.Pi{}).PushControl(agent.ControlPushInput{DeckHome: home, ConversationID: "conv-1", Name: "late"}); delivered || err != nil {
		t.Fatalf("PushControl after shutdown = %v, %v; want unavailable", delivered, err)
	}
}

// Without DECK_PI_CONTROL_DIR the extension opens nothing: the fixture creates no file.
func TestPiControlFileNotOpenedWithoutTheEnvironment(t *testing.T) {
	control, err := openControlFile(extensionHooks{extension: writeExtension(t), sessionID: "conv-1"}, func(string) string { return "" }, io.Discard)
	if control != nil || err != nil {
		t.Fatalf("openControlFile = %v, %v", control, err)
	}
}

// A line that is not a name, and a partial line, change nothing.
func TestPiControlFileIgnoresLinesThatAreNotNames(t *testing.T) {
	for _, line := range []string{"not json", `{"name":""}`, `{"name":"  "}`, `{"other":1}`} {
		if name, ok := agent.DecodePiControlLine([]byte(line)); ok {
			t.Errorf("DecodePiControlLine(%q) = %q, want ignored", line, name)
		}
	}
}
