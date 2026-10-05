package tmux

import (
	"context"
	"strings"
	"testing"
)

func TestParsePreviewCaptureSplitsFactsFromScreen(t *testing.T) {
	capture, err := parsePreviewCapture("deck_a", []byte("0|80|24\nline one\nline two\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !capture.Live || capture.Width != 80 || capture.Height != 24 || string(capture.Bytes) != "line one\nline two\n" {
		t.Fatalf("capture = %+v, want live 80x24 with the screen after the facts line", capture)
	}
}

func TestParsePreviewCaptureReportsADeadPaneAsNotLive(t *testing.T) {
	capture, err := parsePreviewCapture("deck_a", []byte("1|80|24\nstale\n"))
	if err != nil || capture.Live || len(capture.Bytes) != 0 {
		t.Fatalf("capture = %+v, %v; want a zero, not-live capture and no error for a dead pane", capture, err)
	}
}

func TestParsePreviewCaptureRejectsMalformedFacts(t *testing.T) {
	for _, tc := range []struct{ data, want string }{
		{"no newline at all", "no pane facts"},
		{"0|80\nscreen", "parse pane facts"},
		{"0|80|24|9\nscreen", "parse pane facts"},
		{"0|wide|24\nscreen", "parse pane geometry"},
		{"0|80|tall\nscreen", "parse pane geometry"},
	} {
		capture, err := parsePreviewCapture("deck_a", []byte(tc.data))
		if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), `"deck_a"`) {
			t.Errorf("parsePreviewCapture(%q) = %+v, %v; want an error naming %q", tc.data, capture, err, tc.want)
		}
	}
}

func TestCapturePreviewThroughAFakeTmux(t *testing.T) {
	ctx := context.Background()
	live := fakeTmuxClient(t, `printf '0|90|30\nhello\n'`)
	capture, err := live.CapturePreview(ctx, "a")
	if err != nil || !capture.Live || capture.Width != 90 || capture.Height != 30 || string(capture.Bytes) != "hello\n" {
		t.Fatalf("live capture = %+v, %v", capture, err)
	}
	gone := fakeTmuxClient(t, `echo "can't find session: deck_a" >&2; exit 1`)
	if capture, err := gone.CapturePreview(ctx, "a"); err != nil || capture.Live {
		t.Fatalf("vanished session = %+v, %v; want not live and no error", capture, err)
	}
	broken := fakeTmuxClient(t, `echo "protocol error" >&2; exit 1`)
	if _, err := broken.CapturePreview(ctx, "a"); err == nil || !strings.Contains(err.Error(), "capture preview") || !strings.Contains(err.Error(), "protocol error") {
		t.Fatalf("other tmux failure error = %v, want it wrapped with the capture context", err)
	}
	if _, err := broken.CapturePreview(ctx, "Bad Slug"); err == nil || !strings.Contains(err.Error(), "invalid session slug") {
		t.Fatalf("invalid slug error = %v", err)
	}
	if _, err := (Client{}).CapturePreview(ctx, "a"); err == nil || !strings.Contains(err.Error(), "socket name is required") {
		t.Fatalf("missing socket error = %v", err)
	}
}
