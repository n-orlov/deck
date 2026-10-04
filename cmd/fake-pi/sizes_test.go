package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// readCountEventually polls path until it holds want, failing the test if it
// never does: the recorder runs its signal loop in a goroutine, so the count
// file lags the signal that provoked it.
func readCountEventually(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(path)
		if err == nil {
			got = strings.TrimSpace(string(b))
			if got == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s = %q, want %q", path, got, want)
}

func TestStartSizeRecorderCountsEachSigwinch(t *testing.T) {
	home := t.TempDir()
	getenv := func(k string) string {
		if k == "DECK_HOME" {
			return home
		}
		return ""
	}
	stop := startSizeRecorder(getenv)
	defer stop()

	countPath := filepath.Join(home, "log", sigwinchCountName)
	for i, want := range []string{"1", "2", "3"} {
		if err := syscall.Kill(os.Getpid(), syscall.SIGWINCH); err != nil {
			t.Fatalf("signal %d: %v", i, err)
		}
		readCountEventually(t, countPath, want)
	}
}

func TestStartSizeRecorderWithoutHomeIsANoOp(t *testing.T) {
	stop := startSizeRecorder(func(string) string { return "" })
	if stop == nil {
		t.Fatal("stop func is nil")
	}
	stop() // must be callable and harmless
}

func TestStartSizeRecorderStopReleasesTheHandler(t *testing.T) {
	home := t.TempDir()
	getenv := func(k string) string {
		if k == "DECK_HOME" {
			return home
		}
		return ""
	}
	stop := startSizeRecorder(getenv)
	// A second recorder keeps SIGWINCH caught for the process, so a signal
	// sent after the first one stopped cannot kill the test binary.
	keep := startSizeRecorder(func(k string) string {
		if k == "DECK_HOME" {
			return t.TempDir()
		}
		return ""
	})
	defer keep()
	stop()
	if err := syscall.Kill(os.Getpid(), syscall.SIGWINCH); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(home, "log", sigwinchCountName)); err == nil {
		t.Fatal("a stopped recorder still counted a SIGWINCH")
	}
}

func TestRecordSigwinchCountOverwritesWithTheLatestTotal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log", sigwinchCountName)
	recordSigwinchCount(path, 7)
	recordSigwinchCount(path, 12)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "12" {
		t.Fatalf("count file = %q, want %q (bare decimal, overwritten)", b, "12")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("log dir holds %d entries, want only the count file (no temp leftovers)", len(entries))
	}
}

func TestRecordSigwinchCountSwallowsUnwritableTargets(t *testing.T) {
	root := t.TempDir()

	// The directory cannot be created: a regular file sits where it must go.
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	recordSigwinchCount(filepath.Join(blocker, "log", sigwinchCountName), 1)
	if b, _ := os.ReadFile(blocker); string(b) != "x" {
		t.Fatalf("blocking file was modified: %q", b)
	}

	// The rename cannot land: the target is a non-empty directory. The
	// scratch file must be cleaned up and the directory left untouched.
	dir := filepath.Join(root, "log")
	target := filepath.Join(dir, sigwinchCountName)
	if err := os.MkdirAll(filepath.Join(target, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	recordSigwinchCount(target, 3)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != sigwinchCountName {
		t.Fatalf("log dir after a failed rename = %v, want only the untouched target", entries)
	}
}
