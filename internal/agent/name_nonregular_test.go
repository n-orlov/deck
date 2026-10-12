package agent

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// readNameWithin runs read in a goroutine and fails the test when it has not
// returned within one second. The FIFO it is pointed at has no writer, so a
// reader that opens it blocks; the cleanup opens a writer to free a blocked one.
func readNameWithin(t *testing.T, fifo string, read func() (string, bool)) {
	t.Helper()
	t.Cleanup(func() {
		if f, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			_ = f.Close()
		}
	})
	type result struct {
		name string
		ok   bool
	}
	done := make(chan result, 1)
	go func() {
		name, ok := read()
		done <- result{name, ok}
	}()
	select {
	case got := <-done:
		if got.ok || got.name != "" {
			t.Fatalf("ReadName = (%q, %v) for a FIFO, want no name", got.name, got.ok)
		}
	case <-time.After(time.Second):
		t.Fatal("ReadName still blocked on a FIFO with no writer after one second")
	}
}

func mkfifo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestClaudeReadNameReturnsOnAFIFOTranscript (R270): the transcript ReadName
// reads is a FIFO with no writer; the read returns at once with no name.
func TestClaudeReadNameReturnsOnAFIFOTranscript(t *testing.T) {
	home := t.TempDir()
	fifo := filepath.Join(home, ".claude", "projects", "-work-proj", nameTestUUID+".jsonl")
	mkfifo(t, fifo)
	in := NameProbeInput{Home: home, CWD: "/work/proj", ConversationID: nameTestUUID}
	readNameWithin(t, fifo, func() (string, bool) {
		name, _, ok := NewClaude().ReadName(in)
		return name, ok
	})
}

// TestCopilotReadNameReturnsOnAFIFOWorkspace (R270): workspace.yaml is a FIFO
// with no writer; the read returns at once with no name.
func TestCopilotReadNameReturnsOnAFIFOWorkspace(t *testing.T) {
	home := t.TempDir()
	fifo := filepath.Join(home, "session-state", copilotNameFixtureID, "workspace.yaml")
	mkfifo(t, fifo)
	in := NameProbeInput{ConversationID: copilotNameFixtureID, Env: map[string]string{"COPILOT_HOME": home}}
	readNameWithin(t, fifo, func() (string, bool) {
		name, _, ok := Copilot{}.ReadName(in)
		return name, ok
	})
}

// TestClaudeNamePathConfinesTheHookTranscript (R271): the hook's transcript
// path is used as given only when it is a regular file inside the Claude
// projects root. A path outside the root, a `..` escape, a symlink out of the
// root and a FIFO all fall back to the path derived from the cwd and the
// conversation id.
func TestClaudeNamePathConfinesTheHookTranscript(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	projects := filepath.Join(root, "projects")
	if err := os.MkdirAll(filepath.Join(projects, "-work-proj"), 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(path string) string {
		t.Helper()
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	inside := write(filepath.Join(projects, "-work-proj", "inside.jsonl"))
	elsewhere := write(filepath.Join(outside, "elsewhere.jsonl"))
	escaped := write(filepath.Join(root, "escaped.jsonl"))
	link := filepath.Join(projects, "-work-proj", "link.jsonl")
	if err := os.Symlink(elsewhere, link); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(projects, "-work-proj", "fifo.jsonl")
	mkfifo(t, fifo)
	derived := filepath.Join(projects, "-work-proj", nameTestUUID+".jsonl")

	for _, tc := range []struct {
		row  string
		hook string
		want string
	}{
		{"a inside the projects root", inside, inside},
		{"b outside the projects root", elsewhere, derived},
		{"c dotdot escape", filepath.Join(projects, "..", filepath.Base(escaped)), derived},
		{"d symlink pointing outside the root", link, derived},
		{"e FIFO", fifo, derived},
	} {
		t.Run(tc.row, func(t *testing.T) {
			got := claudeNamePath(NameProbeInput{
				TranscriptPath: tc.hook, CWD: "/work/proj", ConversationID: nameTestUUID,
				Env: map[string]string{claudeConfigDirKey: root},
			})
			if got != tc.want {
				t.Fatalf("claudeNamePath = %q, want %q", got, tc.want)
			}
		})
	}
}
