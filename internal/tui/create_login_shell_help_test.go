package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// TestCreateModalLoginShellHelpStatesCapturedPathIsAdvisory covers task 006:
// the create modal's Login shell field help must state that enabling login
// shell makes captured_path advisory (i.e. captured_path is not applied),
// since a login shell resolves its own PATH via the shell's own profile.
func TestCreateModalLoginShellHelpStatesCapturedPathIsAdvisory(t *testing.T) {
	model := New(nil, config.Settings{Socket: "test-socket"}, "")
	// Task 030: framedDialog's width is now 80% of the viewport (clamped to
	// [26, 80]), not grown to fit content, so a wide-enough viewport is
	// needed to keep this long help sentence's asserted phrases on one
	// wrapped line rather than split across a word-wrap boundary.
	model.width = 100
	view := model.createView()
	if !strings.Contains(view, "Login shell") {
		t.Fatalf("createView missing Login shell field:\n%s", view)
	}
	if !strings.Contains(view, "captured_path advisory") || !strings.Contains(view, "not applied") {
		t.Fatalf("createView's Login shell help does not state the captured_path advisory relationship:\n%s", view)
	}
}
