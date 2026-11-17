package features

import "testing"

// The banner lines are found even when the pane's tty echo of text typed
// without a newline (deck's "/rename <name>") shares their row.
func TestCodexPaneAnnouncementFromCaptureFindsBannersAfterEchoedText(t *testing.T) {
	capture := "Fake Codex\n" +
		"{\"command\":\"prompt\",\"text\":\"hi\"}\n" +
		"/rename restart codexfake-codex session-id: df4c6b79-2bdb-478e-8a4e-22463a63ce5d\n" +
		"fake-codex transcript-path: /tmp/a dir/rollout-1.jsonl\n"
	got, ok := codexPaneAnnouncementFromCapture(capture)
	if !ok || got.SessionID != "df4c6b79-2bdb-478e-8a4e-22463a63ce5d" || got.TranscriptPath != "/tmp/a dir/rollout-1.jsonl" {
		t.Fatalf("announcement = %+v, ok=%v", got, ok)
	}
	if _, ok := codexPaneAnnouncementFromCapture("Fake Codex\n› Ask Codex to do anything\n"); ok {
		t.Fatal("a capture with no banner announced a session id")
	}
}

// A capture taken between submitPrompt's two banner writes (the session-id
// line is on screen, the transcript-path line is not yet) is not a finished
// announcement: the step keeps polling instead of recording an empty path.
func TestCodexPaneAnnouncementFromCaptureWaitsForTheTranscriptPathBanner(t *testing.T) {
	const id = "fake-codex session-id: df4c6b79-2bdb-478e-8a4e-22463a63ce5d\n"
	if got, ok := codexPaneAnnouncementFromCapture("Fake Codex\n" + id); ok {
		t.Fatalf("a capture with only the session-id banner announced %+v", got)
	}
	got, ok := codexPaneAnnouncementFromCapture("Fake Codex\n" + id + "fake-codex transcript-path: /tmp/r.jsonl\n")
	if !ok || got.TranscriptPath != "/tmp/r.jsonl" {
		t.Fatalf("announcement = %+v, ok=%v, want the transcript path", got, ok)
	}
}

// A pane that announces no transcript path at all (its CODEX_HOME/HOME did
// not resolve) finishes with the "Working" line printed after both banners.
func TestCodexPaneAnnouncementFromCaptureAcceptsAnOmittedTranscriptPathOnceWorking(t *testing.T) {
	capture := "fake-codex session-id: df4c6b79-2bdb-478e-8a4e-22463a63ce5d\n• Working (5s • esc to interrupt)\n"
	got, ok := codexPaneAnnouncementFromCapture(capture)
	if !ok || got.TranscriptPath != "" {
		t.Fatalf("announcement = %+v, ok=%v, want ok with no path", got, ok)
	}
}
