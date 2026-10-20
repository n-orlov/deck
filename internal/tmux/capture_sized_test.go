// capture_sized_test.go covers CapturePaneSized: the output clock's capture
// and the pane size read just after it, in one tmux process (R409, R185).
package tmux

import (
	"context"
	"testing"
	"time"
)

func TestCapturePaneSizedMatchesCapturePaneAndReportsTheSize(t *testing.T) {
	socket := geometrySocket("capture-sized")
	cleanup := newBareGeometrySession(t, socket, "s0", 47, 13)
	defer cleanup()
	runInPaneBlocking(t, socket, `line one\nline two`)
	paneID := resolveSolePaneID(t, socket)
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()
	options := CaptureOptions{StartLine: "-200", EndLine: "-"}

	want, err := client.CapturePane(ctx, paneID, options)
	if err != nil {
		t.Fatalf("CapturePane: %v", err)
	}
	body, width, height, err := client.CapturePaneSized(ctx, paneID, options)
	if err != nil {
		t.Fatalf("CapturePaneSized: %v", err)
	}
	if string(body) != string(want) {
		t.Fatalf("CapturePaneSized body = %q, want CapturePane's %q", body, want)
	}
	if width != 47 || height != 13 {
		t.Fatalf("CapturePaneSized size = %dx%d, want 47x13", width, height)
	}
}

func TestCapturePaneSizedRejectsAnInvalidTarget(t *testing.T) {
	client := Client{Socket: "unused"}
	if _, _, _, err := client.CapturePaneSized(context.Background(), "s0", CaptureOptions{StartLine: "-", EndLine: "-"}); err == nil {
		t.Fatal("CapturePaneSized accepted a session name as a pane id")
	}
}

func TestSplitPaneSizeLine(t *testing.T) {
	for _, tc := range []struct {
		name, output, body string
		width, height      int
	}{
		{"body and size", "a\nb\ndeck-pane-size:80x24\n", "a\nb\n", 80, 24},
		{"empty body", "deck-pane-size:80x24\n", "", 80, 24},
		{"no trailing newline", "a\ndeck-pane-size:80x24", "a\n", 80, 24},
		{"no size line is all body", "a\nb\n", "a\nb\n", 0, 0},
		{"a malformed size is all body", "a\ndeck-pane-size:80by24\n", "a\ndeck-pane-size:80by24\n", 0, 0},
		{"a zero size is all body", "a\ndeck-pane-size:0x24\n", "a\ndeck-pane-size:0x24\n", 0, 0},
		{"empty output", "", "", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, width, height := splitPaneSizeLine([]byte(tc.output))
			if string(body) != tc.body || width != tc.width || height != tc.height {
				t.Fatalf("splitPaneSizeLine(%q) = %q, %d, %d; want %q, %d, %d", tc.output, body, width, height, tc.body, tc.width, tc.height)
			}
		})
	}
}
