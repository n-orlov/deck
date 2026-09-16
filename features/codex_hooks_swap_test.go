package features

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCodexIdentityMismatchCatchesSwappedStoredIDs is B3's negative control
// (task 008). It builds two real Codex rollout transcripts under one
// shared CODEX_HOME -- exactly the shape codex_hooks.feature's own
// scenario produces (two sessions in one directory) -- then feeds
// codexIdentityMismatch (the exact comparison
// sessionCodexPersistedIdentityMatchesPaneAnnouncement calls) session
// "one"'s STORED conversation id against session "two"'s own captured
// pane announcement, and vice versa, proving the oracle fails on BOTH
// halves -- the id comparison AND the transcript-path comparison -- for
// an attribution mistake between two genuinely distinct, existing
// sessions. An oracle that only asked "are the two ids/paths different
// from each other" (mere distinctness) rather than "does THIS row's own
// stored id/path match THIS pane's own announcement" (attribution) would
// pass this swap by accident, since both ids/paths here really are
// distinct -- they are simply attributed to the wrong row. The final two
// assertions are the control's own sanity check: the SAME (unswapped)
// pairing must pass on both halves, or a failure above would prove
// nothing about attribution-sensitivity -- only that the plumbing itself
// is broken.
func TestCodexIdentityMismatchCatchesSwappedStoredIDs(t *testing.T) {
	home := t.TempDir()
	one := writeTestCodexRollout(t, home, "11111111-1111-1111-1111-111111111111", filepath.Join(home, "one-cwd"))
	two := writeTestCodexRollout(t, home, "22222222-2222-2222-2222-222222222222", filepath.Join(home, "two-cwd"))

	if _, idErr, pathErr := codexIdentityMismatch(home, one.SessionID, two); idErr == nil || pathErr == nil {
		t.Fatalf("codexIdentityMismatch(session one's stored id, session two's announcement) = idErr %v, pathErr %v, want both non-nil", idErr, pathErr)
	}
	if _, idErr, pathErr := codexIdentityMismatch(home, two.SessionID, one); idErr == nil || pathErr == nil {
		t.Fatalf("codexIdentityMismatch(session two's stored id, session one's announcement) = idErr %v, pathErr %v, want both non-nil", idErr, pathErr)
	}

	if _, idErr, pathErr := codexIdentityMismatch(home, one.SessionID, one); idErr != nil || pathErr != nil {
		t.Fatalf("codexIdentityMismatch(session one's own stored id, session one's own announcement) = idErr %v, pathErr %v, want both nil", idErr, pathErr)
	}
	if _, idErr, pathErr := codexIdentityMismatch(home, two.SessionID, two); idErr != nil || pathErr != nil {
		t.Fatalf("codexIdentityMismatch(session two's own stored id, session two's own announcement) = idErr %v, pathErr %v, want both nil", idErr, pathErr)
	}
}

// writeTestCodexRollout writes one rollout transcript under home exactly
// following cmd/fake-codex's own writeRolloutSessionMeta convention (task
// 021), the same convention internal/agent/codex.go's TranscriptPaths
// globs for, and returns the codexPaneAnnouncement a pane minting this
// same session would itself have announced.
func writeTestCodexRollout(t *testing.T, home, sessionID, cwd string) codexPaneAnnouncement {
	t.Helper()
	now := time.Now().UTC()
	dir := filepath.Join(home, ".codex", "sessions", now.Format("2006"), now.Format("01"), now.Format("02"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir rollout directory: %v", err)
	}
	path := filepath.Join(dir, "rollout-"+now.Format("2006-01-02T15-04-05")+"-"+sessionID+".jsonl")
	meta := map[string]any{"type": "session_meta", "session_id": sessionID, "cwd": cwd}
	encoded, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("encode session_meta: %v", err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write rollout transcript: %v", err)
	}
	return codexPaneAnnouncement{SessionID: sessionID, TranscriptPath: path}
}
