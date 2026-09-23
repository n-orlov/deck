package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestDeleteConfirmPurgeShowsExactPathForClaudeWithATranscript proves task
// 110's "displays the exact absolute path it will delete before deleting
// it" directly against the rendered dialog string (never against a
// terminal-wrapped screen capture, which a real transcript path -- deep
// inside a temp HOME -- would very likely overflow): opening dd's confirm
// dialog for a claude session whose declared transcript (internal/agent's
// TranscriptPaths, task 109) exists resolves deletePurgePath/OK eagerly,
// defaults to the non-default "keep" candidate, and cycling to "purge"
// renders that exact path verbatim.
func TestDeleteConfirmPurgeShowsExactPathForClaudeWithATranscript(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := filepath.Join(home, "work")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	project := strings.ReplaceAll(cwd, string(filepath.Separator), "-")
	transcriptDir := filepath.Join(home, ".claude", "projects", project)
	if err := os.MkdirAll(transcriptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	conversationID := "11111111-1111-1111-1111-111111111111"
	transcriptPath := filepath.Join(transcriptDir, conversationID+".jsonl")
	if err := os.WriteFile(transcriptPath, []byte(`{"message":"hi"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	model := NewWithShellCreator(nil, config.Settings{}, "", nil)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "stopped", CWD: cwd, ConversationID: conversationID}}
	model.selected = rowCursor(0)

	got, _ := model.Update(key("d"))
	model = got.(Model)
	got, _ = model.Update(key("d"))
	model = got.(Model)
	if !model.deleteConfirming {
		t.Fatal("dd did not open the confirm dialog")
	}
	if model.deletePurgeValue != "keep" {
		t.Fatalf("deletePurgeValue = %q, want %q as the non-default candidate's starting value", model.deletePurgeValue, "keep")
	}
	if !model.deletePurgeOK || model.deletePurgePath != transcriptPath {
		t.Fatalf("deletePurgePath/OK = %q/%v, want %q/true", model.deletePurgePath, model.deletePurgeOK, transcriptPath)
	}

	got, _ = model.Update(key("right"))
	model = got.(Model)
	if model.deletePurgeValue != "purge" {
		t.Fatalf("right did not cycle deletePurgeValue to %q: got %q", "purge", model.deletePurgeValue)
	}
	view := model.deleteConfirmBody()
	if !strings.Contains(view, transcriptPath) {
		t.Fatalf("delete confirm dialog with purge chosen does not show the exact transcript path %q:\n%s", transcriptPath, view)
	}
}

// TestDeleteConfirmPurgeDeclinesWhenNoTranscriptCanBeLocated proves the
// other half of the same requirement: an adapter with no declared
// transcript path (a shell session, Capabilities().HasTranscript false)
// shows a plain decline message when purge is chosen rather than
// pretending a path exists.
func TestDeleteConfirmPurgeDeclinesWhenNoTranscriptCanBeLocated(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	model := NewWithShellCreator(nil, config.Settings{}, "", nil)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "stopped", CWD: home}}
	model.selected = rowCursor(0)

	got, _ := model.Update(key("d"))
	model = got.(Model)
	got, _ = model.Update(key("d"))
	model = got.(Model)
	if model.deletePurgeOK {
		t.Fatalf("shell session unexpectedly resolved a transcript path %q", model.deletePurgePath)
	}

	got, _ = model.Update(key("right"))
	model = got.(Model)
	view := model.deleteConfirmBody()
	if !strings.Contains(view, "purge deletes nothing") {
		t.Fatalf("delete confirm dialog with purge chosen but no transcript did not decline:\n%s", view)
	}
}
