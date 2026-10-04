package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

// TestProfilesFlagListsDefaultFirstThenSortedAndExitsZero pins SPEC §3.4:
// `deck --profiles` prints one line per profile, default first, then the
// rest sorted by name regardless of the order their directories were
// created in, each line naming its socket, config path, data path and
// last-used time, and the whole command exits 0.
func TestProfilesFlagListsDefaultFirstThenSortedAndExitsZero(t *testing.T) {
	home := t.TempDir()
	// Created deliberately out of alphabetical order, so a pass would be
	// a coincidence of directory-scan order rather than an actual sort.
	for _, name := range []string{"zulu", "alpha"} {
		if err := os.MkdirAll(filepath.Join(home, "profiles", name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// alpha has launched before: its last_used marker's mtime is what its
	// "last used" field must report; zulu never has, so it reads "never".
	used := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	marker := filepath.Join(home, "profiles", "alpha", "last_used")
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(marker, used, used); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runWithEnv(t, []string{"DECK_HOME=" + home}, "--profiles")
	if code != 0 {
		t.Fatalf("deck --profiles exit = %d, want 0 (stderr=%q)", code, stderr)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("deck --profiles printed %d lines, want 3:\n%s", len(lines), stdout)
	}
	if want := "last used: " + used.Local().Format(time.RFC3339); !strings.HasSuffix(lines[1], want) {
		t.Fatalf("alpha line = %q, want it to end %q (the marker's mtime)", lines[1], want)
	}
	if !strings.HasSuffix(lines[2], "last used: never") {
		t.Fatalf("zulu line = %q, want \"last used: never\" with no marker", lines[2])
	}
	alphaDir := filepath.Join(home, "profiles", "alpha")
	if want := "config: " + filepath.Join(alphaDir, "config.toml") + "\tdata: " + alphaDir + "\t"; !strings.Contains(lines[1], want) {
		t.Fatalf("alpha line = %q, want its own paths %q", lines[1], want)
	}
	if !strings.HasPrefix(lines[0], "default\t") {
		t.Fatalf("first line = %q, want default listed first", lines[0])
	}
	if !strings.HasPrefix(lines[1], "alpha\t") || !strings.HasPrefix(lines[2], "zulu\t") {
		t.Fatalf("named profile lines = %q, %q, want alpha then zulu (sorted)", lines[1], lines[2])
	}
	for i, want := range []string{"socket: deck\t", "socket: deck-alpha", "socket: deck-zulu"} {
		if !strings.Contains(lines[i], want) {
			t.Fatalf("line %q missing %q", lines[i], want)
		}
	}
	for i, line := range lines {
		if !strings.Contains(line, "config: ") || !strings.Contains(line, "data: ") || !strings.Contains(line, "last used: ") {
			t.Fatalf("line %d = %q, missing SPEC \u00a73.4's config/data/last-used fields", i, line)
		}
	}
}

// TestProfilesFlagFlagsInvalidDirectoryNameAndExitsZero pins SPEC §3.4:
// "a directory under profiles/ whose name fails validation is listed
// flagged (invalid name: not selectable), and the exit code is still 0."
func TestProfilesFlagFlagsInvalidDirectoryNameAndExitsZero(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "profiles", "Bad.Name"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "profiles", "ok"), 0o700); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runWithEnv(t, []string{"DECK_HOME=" + home}, "--profiles")
	if code != 0 {
		t.Fatalf("deck --profiles exit = %d, want 0 (stderr=%q)", code, stderr)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("deck --profiles printed %d lines, want 3 (default, the flagged dir, ok):\n%s", len(lines), stdout)
	}
	// Sorted by name: "Bad.Name" < "ok" byte-wise, so the flagged row is
	// the second line, after default.
	bad := lines[1]
	badDir := filepath.Join(home, "profiles", "Bad.Name")
	// The flagged row keeps SPEC §3.4's one-line shape -- name, socket,
	// config and data paths, last used -- with the flag after the name.
	for _, want := range []string{
		"Bad.Name (invalid name: not selectable)\t",
		"socket: deck-Bad.Name\t",
		"config: " + filepath.Join(badDir, "config.toml") + "\t",
		"data: " + badDir + "\t",
		"last used: never",
	} {
		if !strings.Contains(bad, want) {
			t.Fatalf("flagged row = %q, want it to contain %q", bad, want)
		}
	}
	if !strings.HasPrefix(bad, "Bad.Name (invalid name: not selectable)\t") {
		t.Fatalf("flagged row = %q, want it to start with the name and its flag", bad)
	}
	if !strings.HasPrefix(lines[2], "ok\t") {
		t.Fatalf("stdout = %q, want the valid sibling profile still listed", stdout)
	}
}

// writeUnreadableStateDB creates path (parent directories included) with
// mode 0000: unreadable and unwritable even to its own owner, the sharpest
// available proof that nothing downstream opened it.
func writeUnreadableStateDB(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not a real sqlite file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
}

// TestProfilesFlagSucceedsWithEveryStateDBModeZero pins SPEC §3.4: "it
// never opens a state.db" -- the listing (and its exit code) is
// unaffected even when every profile's state.db, default included, is
// mode 0000.
func TestProfilesFlagSucceedsWithEveryStateDBModeZero(t *testing.T) {
	home := t.TempDir()
	writeUnreadableStateDB(t, filepath.Join(home, "state.db"))
	writeUnreadableStateDB(t, filepath.Join(home, "profiles", "work", "state.db"))
	code, stdout, stderr := runWithEnv(t, []string{"DECK_HOME=" + home}, "--profiles")
	if code != 0 {
		t.Fatalf("deck --profiles exit = %d, want 0 with every state.db mode 0000 (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, "default\t") || !strings.Contains(stdout, "work\t") {
		t.Fatalf("stdout = %q, want both profiles still listed with every state.db mode 0000", stdout)
	}
}

// TestDeckBinaryLaunchTouchesLastUsedMarker pins SPEC §3.4's other half:
// "'last used' [is] the mtime of a last_used marker the TUI touches in
// the profile's data root at launch, default included." A real deck
// binary launch (through a pty, as the other cmd/deck lifecycle tests do)
// against the default profile must advance an already-old marker's mtime
// before it ever reaches its first frame.
func TestDeckBinaryLaunchTouchesLastUsedMarker(t *testing.T) {
	binary := buildDeckProfileCreationTestBinary(t)
	home := t.TempDir()
	socket := "deck_test_lastused_" + strings.ReplaceAll(filepath.Base(home), "_", "")
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })

	marker := filepath.Join(home, "last_used")
	old := time.Now().Add(-1 * time.Hour).Truncate(time.Second)
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(marker, old, old); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(),
		"DECK_HOME="+home, "DECK_TMUX_SOCKET="+socket,
		"NO_COLOR=1", "DECK_ASCII=1", "DECK_ANIM=0", "TERM=xterm-256color", "SHELL=/bin/sh")
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 100})
	if err != nil {
		t.Fatalf("start deck under pty: %v", err)
	}
	defer terminal.Close()
	output := newPTYOutput()
	go pumpTerminal(output, terminal)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	waitForScreen(t, output, done, "\x1b[6n")
	if _, err := terminal.Write([]byte("\x1b]11;rgb:0000/0000/0000\x1b\\\x1b[1;1R")); err != nil {
		t.Fatalf("answer terminal queries: %v", err)
	}
	waitForScreen(t, output, done, "No sessions yet")

	info, err := os.Stat(marker)
	if err != nil {
		t.Fatalf("stat last_used marker after launch: %v", err)
	}
	if !info.ModTime().After(old) {
		t.Fatalf("last_used mtime = %v, want after %v -- the launch did not touch it", info.ModTime(), old)
	}

	cancel()
	<-done
}
