package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// TestHelpListsDeckProfile pins SPEC §3.4/R155/R157: the `?` help view
// names DECK_PROFILE beside the other DECK_* runtime variables it lists.
// The frame is 400 rows tall -- matching helpStyleTestModel's own
// workaround for task 078's height-bounding -- so nothing the assertion
// checks is ever scrolled out of the rendered window.
func TestHelpListsDeckProfile(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.help = true
	m.width, m.height = 100, 400

	rendered := m.helpView()

	if !strings.Contains(rendered, "DECK_PROFILE") {
		t.Errorf("help view does not name DECK_PROFILE; got:\n%s", rendered)
	}
}

// TestHelpGivesManualProfileDeletionSteps pins SPEC §3.4/R155/R157: deck
// never deletes a profile itself, so help must give the manual steps --
// killing that profile's own tmux server, then removing its directories
// on disk -- naming both the exact command and the exact path shape the
// SPEC prose uses.
func TestHelpGivesManualProfileDeletionSteps(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.help = true
	m.width, m.height = 100, 400

	rendered := m.helpView()

	if !strings.Contains(rendered, "tmux -L deck-<name> kill-server") {
		t.Errorf("help view does not give the kill-server step; got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "profiles/<name>/") {
		t.Errorf("help view does not name the profiles/<name>/ directories to remove; got:\n%s", rendered)
	}
}
