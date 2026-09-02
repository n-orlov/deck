package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/interactive"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// waitForDisplacementTest polls cond until it reports true or timeout
// expires, mirroring internal/interactive/grid_test.go's own waitFor
// (unexported there, in a different package).
func waitForDisplacementTest(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

// findInteractiveDisplacementChecked runs a previewTick's own returned
// command (a tea.BatchMsg once both the unconditional reschedule and the
// backstop poll are both present -- see preview_fit_overlap_test.go's own
// previewTickCmds for the same tea.Batch-collapsing behaviour) and picks
// out the one interactiveDisplacementChecked message among its members.
func findInteractiveDisplacementChecked(t *testing.T, cmd tea.Cmd) interactiveDisplacementChecked {
	t.Helper()
	if cmd == nil {
		t.Fatal("previewTick returned no command at all; want the backstop poll among them")
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			if got, ok := c().(interactiveDisplacementChecked); ok {
				return got
			}
		}
		t.Fatal("previewTick's batch contained no interactiveDisplacementChecked command")
	}
	if got, ok := msg.(interactiveDisplacementChecked); ok {
		return got
	}
	t.Fatalf("previewTick returned %T (running its single command produced %T), want a tea.BatchMsg containing interactiveDisplacementChecked", cmd, msg)
	return interactiveDisplacementChecked{}
}

// TestPreviewTickRaisesLostAttachOnStolenClaim proves task 118's
// stolen-claim flavour end-to-end against a real tmux server: a second
// deck process force-claims (`F`) the SAME window a first process is
// already interactive against. Under the default interactive.
// TransportPipe, the steal's own entry re-arms a competing pipe-pane on
// the very same target as part of starting its own transport, which
// displaces the first holder's pipe -- the fast path
// (interactiveDisplacementFastPath) must notice this on the very next
// previewTick with no tmux round trip of its own, leave interactive mode
// and raise the lost-attach dialog naming the first holder's own session.
func TestPreviewTickRaisesLostAttachOnStolenClaim(t *testing.T) {
	socket := selectionTestSocket("displacestolen")
	newQuietSelectionPane(t, socket, "deck_displacestolen", 80, 24)
	client := tmux.Client{Socket: socket}

	newTestModel := func() Model {
		m := New(nil, config.Settings{Color: true}, "")
		m.width, m.height = 100, 30
		m.tmuxClient = client
		m.sessions = []store.Session{{ID: "sess-stolen-1", Name: "displacestolen", Slug: "displacestolen", Status: "waiting"}}
		m.selected = 0
		return m
	}

	next1, _ := newTestModel().enterInteractiveBody(false)
	got1 := next1.(Model)
	if !got1.interactive {
		t.Fatalf("first entry did not enter interactive mode: attachError=%q", got1.attachError)
	}
	if got1.interactiveGrid.Status() != interactive.StatusLive {
		t.Fatalf("first entry's transport status = %v before any steal, want StatusLive", got1.interactiveGrid.Status())
	}

	next2, _ := newTestModel().enterInteractiveBody(true)
	got2 := next2.(Model)
	if got2.attachError != "" {
		t.Fatalf("the steal was refused: %q", got2.attachError)
	}
	if !got2.interactive {
		t.Fatalf("the steal did not enter interactive mode")
	}
	defer got2.exitInteractive()

	if !waitForDisplacementTest(t, 2*time.Second, func() bool {
		return got1.interactiveGrid.Status() == interactive.StatusDisplaced
	}) {
		t.Fatalf("first holder's grid never reported StatusDisplaced within 2s of the steal")
	}

	updated, _ := got1.Update(previewTick(time.Now()))
	m := updated.(Model)
	if m.interactive {
		t.Fatalf("previewTick did not leave interactive mode after the fast-path StatusDisplaced signal")
	}
	if !m.lostAttach {
		t.Fatalf("previewTick did not raise the lost-attach dialog after the fast-path StatusDisplaced signal")
	}
	if m.lostAttachSession != "displacestolen" {
		t.Fatalf("lostAttachSession = %q, want %q", m.lostAttachSession, "displacestolen")
	}
}

// TestPreviewTickRaisesLostAttachOnClientAttach proves task 118's
// client-attached flavour: a REAL tmux client attaching directly to the
// claimed window (attachForceEnterPTY, shared with force_enter_test.go)
// never touches deck's own ownership option and never arms a competing
// pipe-pane, so the fast path's own StatusDisplaced signal never fires --
// only the backstop's SessionAttachedCount check catches it, on the very
// same previewTick, via its own async poll.
func TestPreviewTickRaisesLostAttachOnClientAttach(t *testing.T) {
	socket := selectionTestSocket("displaceattach")
	newQuietSelectionPane(t, socket, "deck_displaceattach", 80, 24)
	client := tmux.Client{Socket: socket}

	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	m.tmuxClient = client
	m.sessions = []store.Session{{ID: "sess-attach-1", Name: "displaceattach", Slug: "displaceattach", Status: "waiting"}}
	m.selected = 0

	next, _ := m.enterInteractiveBody(false)
	got := next.(Model)
	if !got.interactive {
		t.Fatalf("entry did not enter interactive mode: attachError=%q", got.attachError)
	}

	windowTarget, err := tmux.SessionName("displaceattach")
	if err != nil {
		t.Fatalf("SessionName: %v", err)
	}
	attachForceEnterPTY(t, socket, windowTarget)
	waitForSessionAttachedCountForce(t, client, windowTarget, 1)

	if got.interactiveGrid.Status() != interactive.StatusLive {
		t.Fatalf("grid status = %v after a plain client attach, want StatusLive: this flavour must be invisible to the fast path", got.interactiveGrid.Status())
	}

	updated, cmd := got.Update(previewTick(time.Now()))
	m2 := updated.(Model)
	if !m2.interactive {
		t.Fatalf("previewTick left interactive mode synchronously; the client-attached flavour is only caught by the async backstop poll")
	}
	if m2.lostAttach {
		t.Fatalf("previewTick raised the lost-attach dialog synchronously; the backstop must be async")
	}

	checked := findInteractiveDisplacementChecked(t, cmd)
	if !checked.displaced {
		t.Fatalf("backstop poll reported displaced=false against a real attached client")
	}

	updated2, _ := m2.Update(checked)
	m3 := updated2.(Model)
	if m3.interactive {
		t.Fatalf("interactiveDisplacementChecked did not leave interactive mode")
	}
	if !m3.lostAttach {
		t.Fatalf("interactiveDisplacementChecked did not raise the lost-attach dialog")
	}
	if m3.lostAttachSession != "displaceattach" {
		t.Fatalf("lostAttachSession = %q, want %q", m3.lostAttachSession, "displaceattach")
	}
}
