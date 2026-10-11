package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// TestRenamePushNoteReachesTheFooterAndClearsOnTheNextKey: a refused push's
// note, naming the reason, shows on the footer while the renamed row is
// selected, and the next list-mode key clears it.
func TestRenamePushNoteReachesTheFooterAndClearsOnTheNextKey(t *testing.T) {
	const note = "rename not pushed to claude: status is waiting: the harness is waiting for an answer"
	model := New(nil, config.Settings{}, "").WithRenamePusher(
		func(_ context.Context, id, name string) (store.Session, service.PushResult, error) {
			return store.Session{ID: id, Name: name, Agent: "claude", Status: "waiting"}, service.PushResult{Note: note}, nil
		})
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "waiting", Slug: "alpha"}}
	model.selected = rowCursor(0)
	model.detail = true

	got, _ := model.Update(key("r"))
	model = got.(Model)
	got, _ = model.Update(key("b"))
	model = got.(Model)
	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("enter did not dispatch the rename")
	}
	got, _ = model.Update(cmd())
	model = got.(Model)
	model.detail = false

	if footer := model.footerLineContent(); !strings.Contains(footer, "status is waiting") {
		t.Fatalf("footer %q does not name the refusal reason", footer)
	}
	got, _ = model.Update(key("down"))
	model = got.(Model)
	if footer := model.footerLineContent(); strings.Contains(footer, "status is waiting: the harness") {
		t.Fatalf("the note survived the next key: %q", footer)
	}
}
