package tui

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestEnterInteractiveDrawsBannerForRealContentionHolders is task 009's own
// real-tmux proof (B.3, R143, GH #38): both of SPEC §11.9's CONTENTION
// refusals -- another client genuinely attached (a real pty attach through
// tmux, exactly TestForceEntersDespiteAnAttachedClient's own setup) and a
// live foreign ownership claim (a raw `set-option` naming an alive pid,
// exactly liveOwnershipRefusalMessage's own setup) -- are proved on a
// private `-L` socket all the way through the ACTUAL DRAWN banner
// (previewBodyLines, the same call previewTick/View render through), not
// merely through entryRefusalWayOut's own string. Earlier tests
// (refusal_f_naming_test.go) already read the way-out text off
// got.entryRefusal.kind for these two setups; this test additionally
// renders the preview body itself and checks the headline, the kind's own
// reason and the `F` hint all appear together in what a user would
// actually see on screen.
func TestEnterInteractiveDrawsBannerForRealContentionHolders(t *testing.T) {
	t.Run("attached elsewhere", func(t *testing.T) {
		m := New(nil, config.Settings{Color: true}, "")
		m.width, m.height = 100, 30
		if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
			t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", height, interactiveMinInnerRows)
		}

		socket := selectionTestSocket("realcontattached")
		newQuietSelectionPane(t, socket, "deck_realcontattached", 80, 24)
		client := tmux.Client{Socket: socket}
		m.tmuxClient = client
		m.sessions = []store.Session{{ID: "sess-realcont-1", Name: "realcontattached", Slug: "realcontattached", Status: "waiting"}}
		m.selected = rowCursor(0)

		windowTarget, err := tmux.SessionName("realcontattached")
		if err != nil {
			t.Fatalf("SessionName: %v", err)
		}
		attachForceEnterPTY(t, socket, windowTarget)
		waitForSessionAttachedCountForce(t, client, windowTarget, 1)

		next, _ := m.enterInteractive()
		got := next.(Model)
		if got.interactive {
			t.Fatalf("enterInteractive (force=false) entered while a real client was attached")
		}
		if got.entryRefusal.kind != entryRefusalAttachedElsewhere {
			t.Fatalf("entryRefusal.kind = %q, want %q", got.entryRefusal.kind, entryRefusalAttachedElsewhere)
		}

		joined := drawnPreviewJoined(t, got)
		if !strings.Contains(joined, "NOT ATTACHED: realcontattached") {
			t.Fatalf("drawn preview does not contain the banner headline:\n%s", joined)
		}
		if !strings.Contains(joined, "attached to this session") {
			t.Fatalf("drawn preview does not contain the attached-elsewhere reason:\n%s", joined)
		}
		if !strings.Contains(joined, "F forces entry") {
			t.Fatalf("drawn preview does not carry the F hint:\n%s", joined)
		}
		if !strings.Contains(joined, "a attaches instead") {
			t.Fatalf("drawn preview does not carry the a hint:\n%s", joined)
		}
	})

	t.Run("owned elsewhere", func(t *testing.T) {
		m := New(nil, config.Settings{Color: true}, "")
		m.width, m.height = 100, 30
		if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
			t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", height, interactiveMinInnerRows)
		}

		socket := selectionTestSocket("realcontowned")
		newQuietSelectionPane(t, socket, "deck_realcontowned", 80, 24)
		client := tmux.Client{Socket: socket}
		m.tmuxClient = client
		m.sessions = []store.Session{{ID: "sess-realcont-2", Name: "realcontowned", Slug: "realcontowned", Status: "waiting"}}
		m.selected = rowCursor(0)

		windowTarget, err := tmux.SessionName("realcontowned")
		if err != nil {
			t.Fatalf("SessionName: %v", err)
		}
		claim := "other-owner-claim-009:" + strconv.Itoa(os.Getpid())
		cmd := exec.CommandContext(context.Background(), "tmux", "-L", socket, "set-option", "-w", "-t", windowTarget, "@deck_isize_owner", claim)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("tmux -L %s set-option -w -t %s @deck_isize_owner %s: %v: %s", socket, windowTarget, claim, err, out)
		}

		next, _ := m.enterInteractive()
		got := next.(Model)
		if got.interactive {
			t.Fatalf("enterInteractive entered despite a live ownership holder")
		}
		if got.entryRefusal.kind != entryRefusalOwnedElsewhere {
			t.Fatalf("entryRefusal.kind = %q, want %q", got.entryRefusal.kind, entryRefusalOwnedElsewhere)
		}

		joined := drawnPreviewJoined(t, got)
		if !strings.Contains(joined, "NOT ATTACHED: realcontowned") {
			t.Fatalf("drawn preview does not contain the banner headline:\n%s", joined)
		}
		if !strings.Contains(joined, "holds ownership of this window") {
			t.Fatalf("drawn preview does not contain the owned-elsewhere reason:\n%s", joined)
		}
		if !strings.Contains(joined, "F forces entry") {
			t.Fatalf("drawn preview does not carry the F hint:\n%s", joined)
		}
		if !strings.Contains(joined, "a attaches instead") {
			t.Fatalf("drawn preview does not carry the a hint:\n%s", joined)
		}
	})
}

// drawnPreviewJoined renders m's preview body exactly as previewTick/View
// do (previewBodyLines) and joins it into one string for substring checks.
func drawnPreviewJoined(t *testing.T, m Model) string {
	t.Helper()
	cw, ch := m.previewContentSize()
	drawn, _, _ := m.previewBodyLines(cw, ch)
	return strings.Join(drawn, "\n")
}
