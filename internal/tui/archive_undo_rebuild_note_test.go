package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file backs SPEC §9.2's one statement about undoing a teardown
// (SPEC.md:508, and help's own Hooks section in the same words): a
// post_destroy that already ran is NOT undone by `u`, so the row comes back
// stopped and the next `r` rebuilds whatever the hook released. R72's archive
// undo (archive_undo_test.go) reverses archived_at and nothing else, which is
// exactly why the operator has to be told -- features/teardown_hooks.feature's
// A/undo scenario asserts this toast end to end against a real hook artefact;
// these tests pin the model-level rule the scenario relies on, including the
// negative case where no hook was configured at all.

// archiveUndoRebuildCase drives one full A → u cycle and returns the frame
// the undo left behind, so each case differs only in where post_destroy was
// configured (the session's own field, the global config key, neither).
func archiveUndoRebuildFrameAfterUndo(t *testing.T, sessionHook, globalHook string) (Model, string) {
	t.Helper()
	model, _, unarchived := archiveUndoTestModel(t)
	model.settings = config.Settings{Undo: time.Hour, PostDestroy: globalHook}
	model.sessions[0].PostDestroy = sessionHook
	model.width, model.height = 100, 40

	got, _ := model.Update(key("A"))
	model = got.(Model)
	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("submitting the archive confirm issued no command")
	}
	got, _ = model.Update(cmd())
	model = got.(Model)
	// The archived row leaves the default list exactly as it does in a real
	// reload; `u` still knows what it archived.
	model.sessions = nil
	model.selected = rowCursor(0)

	got, cmd = model.Update(key("u"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("u issued no command with an archive undo window open")
	}
	got, _ = model.Update(cmd())
	model = got.(Model)
	if len(*unarchived) != 1 {
		t.Fatalf("unarchive called %d times, want exactly once: %v", len(*unarchived), *unarchived)
	}
	model.sessions = []store.Session{{ID: "s-live", Name: "live-agent", Agent: "claude", Status: "stopped"}}
	return model, model.View()
}

// TestArchiveUndoSaysTheNextRRebuildsWhatPostDestroyReleased is the positive
// half, run once per place a teardown hook can be configured (SPEC §9.2 runs
// the session's own hook and then the global one, so either one having a value
// means a hook ran for this archive).
func TestArchiveUndoSaysTheNextRRebuildsWhatPostDestroyReleased(t *testing.T) {
	cases := []struct {
		name        string
		sessionHook string
		globalHook  string
	}{
		{name: "session-hook", sessionHook: "echo teardown >> /tmp/pd.txt", globalHook: ""},
		{name: "global-hook", sessionHook: "", globalHook: "echo teardown >> /tmp/pd.txt"},
		{name: "both-hooks", sessionHook: "echo session", globalHook: "echo global"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model, view := archiveUndoRebuildFrameAfterUndo(t, tc.sessionHook, tc.globalHook)
			if !model.archiveUndoneRebuildNote {
				t.Fatal("the archive undo raised no post_destroy rebuild note")
			}
			if !strings.Contains(view, "the next r rebuilds") {
				t.Fatalf("frame after the undo does not say the next r rebuilds:\n%s", view)
			}
			// The undo un-hides the row and leaves it stopped: the toast must
			// not promise the agent back.
			if strings.Contains(view, "press u to unarchive") {
				t.Fatalf("the archive toast survived its own undo:\n%s", view)
			}

			// Its own DECK_UNDO_MS window, generation-tied like its siblings: a
			// stale tick leaves it alone, the matching one takes it away.
			got, _ := model.Update(archiveUndoneRebuildNoteExpired(model.archiveUndoneRebuildGeneration - 1))
			model = got.(Model)
			if !model.archiveUndoneRebuildNote {
				t.Fatal("a stale rebuild-note tick cleared the current note")
			}
			got, _ = model.Update(archiveUndoneRebuildNoteExpired(model.archiveUndoneRebuildGeneration))
			model = got.(Model)
			if model.archiveUndoneRebuildNote {
				t.Fatal("the matching rebuild-note tick left the note on screen")
			}
			if strings.Contains(model.View(), "the next r rebuilds") {
				t.Fatalf("the rebuild note outlived its own expiry:\n%s", model.View())
			}
		})
	}
}

// TestArchiveUndoWithNoTeardownHookSaysNothingAboutRebuilding is the negative
// half: with no post_destroy anywhere, no hook ran, nothing was released, and
// claiming a rebuild would be a false statement about what just happened.
func TestArchiveUndoWithNoTeardownHookSaysNothingAboutRebuilding(t *testing.T) {
	model, view := archiveUndoRebuildFrameAfterUndo(t, "", "")
	if model.archiveUndoneRebuildNote {
		t.Fatal("an archive with no post_destroy raised the rebuild note anyway")
	}
	if strings.Contains(view, "the next r rebuilds") {
		t.Fatalf("frame after a hookless archive's undo claims a rebuild:\n%s", view)
	}
}
