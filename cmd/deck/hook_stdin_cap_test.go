package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// endlessReader yields an unterminated JSON object forever and counts how many
// bytes the reader pulled from it.
type endlessReader struct {
	prefix string
	given  int64
}

func (e *endlessReader) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) {
		if e.given < int64(len(e.prefix)) {
			p[n] = e.prefix[e.given]
		} else {
			p[n] = 'a'
		}
		n++
		e.given++
	}
	return n, nil
}

func hookCapSettings(t *testing.T) (config.Settings, config.Paths) {
	t.Helper()
	home := t.TempDir()
	paths := config.Paths{Home: home, DataDir: home, ConfigFile: filepath.Join(home, "config.toml"), LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")}
	clock, err := config.NewClock("", "")
	if err != nil {
		t.Fatal(err)
	}
	return config.Settings{Paths: paths, Clock: clock}, paths
}

func TestHookStdinOversizePayloadIsRejectedWithBoundedReads(t *testing.T) {
	settings, paths := hookCapSettings(t)
	src := &endlessReader{prefix: `{"hook_event_name":"Notification","padding":"`}
	err := runHook(context.Background(), settings, src)
	if !errors.Is(err, errHookPayloadTooLarge) {
		t.Fatalf("oversize payload err = %v, want errHookPayloadTooLarge", err)
	}
	if src.given > int64(maxHookStdinBytes)+1 {
		t.Fatalf("hook pulled %d bytes from stdin, want at most cap+1 = %d", src.given, int64(maxHookStdinBytes)+1)
	}
	if _, statErr := os.Stat(paths.StateDB); !os.IsNotExist(statErr) {
		t.Fatalf("rejected payload touched the state database: %v", statErr)
	}
}

func TestHookStdinLargeRealisticPayloadIsNotTruncated(t *testing.T) {
	settings, paths := hookCapSettings(t)
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "big", Name: "big", CWD: paths.Home, Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 1000, CreatedAt: 1000,
		ConversationID: "conversation-big",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	// 4 MiB: far above any fixture payload, well inside the cap. A truncating
	// reader would produce a JSON syntax error here.
	payload := `{"hook_event_name":"SessionEnd","session_id":"conversation-big","reason":"logout","padding":"` + strings.Repeat("x", 4<<20) + `"}`
	if err := runHook(context.Background(), settings, strings.NewReader(payload)); err != nil {
		t.Fatalf("4 MiB payload rejected: %v", err)
	}
}

func TestCappedHookReaderAllowsExactlyTheCap(t *testing.T) {
	c := &cappedHookReader{r: strings.NewReader("abcd"), remaining: 4}
	got, err := io.ReadAll(c)
	if err != nil || string(got) != "abcd" {
		t.Fatalf("exact-cap read = %q, %v", got, err)
	}
	c = &cappedHookReader{r: strings.NewReader("abcde"), remaining: 4}
	if _, err := io.ReadAll(c); !errors.Is(err, errHookPayloadTooLarge) {
		t.Fatalf("cap+1 read err = %v", err)
	}
}
