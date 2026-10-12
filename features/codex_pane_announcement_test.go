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
