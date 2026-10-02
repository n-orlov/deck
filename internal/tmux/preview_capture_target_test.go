package tmux

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// previewTargetFixture starts n long-lived deck sessions on a private
// server, one of which (marked) prints a recognisable line.
func previewTargetFixture(t *testing.T, n int) Client {
	t.Helper()
	socket := fmt.Sprintf("priv-preview-target-%d-%d", os.Getpid(), time.Now().UnixNano())
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	t.Cleanup(func() { _ = client.command(context.Background(), "kill-server").Run() })
	for i := 0; i < n; i++ {
		if _, err := client.Create(context.Background(), Launch{
			Slug: fmt.Sprintf("tick-%02d", i), CWD: t.TempDir(),
			Command: []string{"sh", "-c", fmt.Sprintf("echo marker-%02d; sleep 60", i)},
		}); err != nil {
			t.Fatal(err)
		}
	}
	return client
}

// TestCapturePreviewIsOneProcessOverManySessions pins R185 (GH #49): one
// passive preview tick over 12 sessions spawns exactly one tmux process, a
// capture addressed by target, never a List first.
func TestCapturePreviewIsOneProcessOverManySessions(t *testing.T) {
	client := previewTargetFixture(t, 12)
	shim, invocationLog := countingTmuxShim(t)
	client.Binary = shim

	var capture PreviewCapture
	var err error
	deadline := time.Now().Add(3 * time.Second)
	for {
		_ = os.Remove(invocationLog)
		capture, err = client.CapturePreview(context.Background(), "tick-07")
		if err != nil || bytes.Contains(capture.Bytes, []byte("marker-07")) || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !capture.Live || !bytes.Contains(capture.Bytes, []byte("marker-07")) {
		t.Fatalf("capture = %#v, want the selected session's own screen", capture)
	}
	if capture.Width <= 0 || capture.Height <= 0 {
		t.Fatalf("capture geometry = %dx%d, want the pane's real geometry", capture.Width, capture.Height)
	}
	if invocations := shimInvocations(t, invocationLog); len(invocations) != 1 {
		t.Fatalf("one preview tick over 12 sessions spawned %d tmux processes, want exactly 1:\n%s", len(invocations), strings.Join(invocations, "\n"))
	}
}

// TestCapturePreviewMatchesSessionNameExactly: tmux prefix-matches a bare
// -t name, so a slug naming no session must not capture a session whose
// name merely starts with it ("tick-1" must not reach "tick-10").
func TestCapturePreviewMatchesSessionNameExactly(t *testing.T) {
	client := previewTargetFixture(t, 11)
	capture, err := client.CapturePreview(context.Background(), "tick-1")
	if err != nil || capture.Live || capture.Bytes != nil {
		t.Fatalf("slug tick-1 (no such session) captured %#v, err = %v, want inert", capture, err)
	}
}

// TestCapturePreviewVanishedTargetIsInertNotAnError: a session killed
// between reconcile and the tick yields the same transient answer as
// before — an inert capture with a nil error, never a failure.
func TestCapturePreviewVanishedTargetIsInertNotAnError(t *testing.T) {
	client := previewTargetFixture(t, 2)
	if err := client.Kill(context.Background(), "tick-01"); err != nil {
		t.Fatal(err)
	}
	capture, err := client.CapturePreview(context.Background(), "tick-01")
	if err != nil || capture.Live || capture.Bytes != nil {
		t.Fatalf("vanished target = %#v, err = %v, want inert and nil", capture, err)
	}
	// And with the whole private server gone.
	_ = client.command(context.Background(), "kill-server").Run()
	capture, err = client.CapturePreview(context.Background(), "tick-00")
	if err != nil || capture.Live {
		t.Fatalf("absent server = %#v, err = %v, want inert and nil", capture, err)
	}
}

// TestCapturePreviewDeadPaneIsInert: a retained dead pane is a corpse, not
// a preview target.
func TestCapturePreviewDeadPaneIsInert(t *testing.T) {
	socket := fmt.Sprintf("priv-preview-dead-%d-%d", os.Getpid(), time.Now().UnixNano())
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	t.Cleanup(func() { _ = client.command(context.Background(), "kill-server").Run() })
	if _, err := client.Create(context.Background(), Launch{Slug: "corpse", CWD: t.TempDir(), Command: []string{"sh", "-c", "exit 3"}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if pane, ok, _ := client.PreviewPane(context.Background(), "corpse"); !ok && pane.ID == "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	capture, err := client.CapturePreview(context.Background(), "corpse")
	if err != nil || capture.Live {
		t.Fatalf("dead pane = %#v, err = %v, want inert and nil", capture, err)
	}
}
