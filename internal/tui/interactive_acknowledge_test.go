package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestEnterInteractiveRecordsAttachment pins SPEC §7's "cleared by
// attaching": entering the interactive preview is a deck-mediated
// attachment exactly like `a`'s full attach, so a successful entry must
// consult the same m.prepareAttach hook (store.RecordAttachment in the
// wired model) with the durable row's ID -- once, and only after every
// refusal and fallible tmux step has passed. Proven against a real tmux
// server on a private socket because enterInteractive's success path
// cannot be reached any other way.
func TestEnterInteractiveRecordsAttachment(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", height, interactiveMinInnerRows)
	}

	socket := selectionTestSocket("ackentry")
	newQuietSelectionPane(t, socket, "deck_ackentry", 80, 24)
	m.tmuxClient = tmux.Client{Socket: socket}
	m.sessions = []store.Session{{ID: "sess-ack-1", Name: "ackentry", Slug: "ackentry", Status: "waiting"}}
	m.selected = rowCursor(0)
	var recorded []string
	m.prepareAttach = func(_ context.Context, id string) error {
		recorded = append(recorded, id)
		return nil
	}

	next, _ := m.enterInteractive()
	got := next.(Model)
	if got.attachError != "" {
		t.Fatalf("enterInteractive refused: %q", got.attachError)
	}
	if !got.interactive {
		t.Fatalf("enterInteractive did not enter interactive mode")
	}
	if len(recorded) != 1 || recorded[0] != "sess-ack-1" {
		t.Fatalf("prepareAttach calls = %v, want exactly one with the durable row's ID %q -- entering the interactive preview must record the attachment the way attachSelected does (SPEC §7)", recorded, "sess-ack-1")
	}
	got.exitInteractive()
}

// TestEnterInteractiveRefusalDoesNotRecordAttachment pins the other half
// of the same contract: a refused entry -- here the 7-inner-row floor,
// which refuses before any tmux call at all -- must not claim the user
// answered anything, so prepareAttach is never consulted.
func TestEnterInteractiveRefusalDoesNotRecordAttachment(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 8
	if _, height := m.previewContentSize(); height >= interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is not below the %d-row floor", height, interactiveMinInnerRows)
	}
	m.tmuxClient = tmux.Client{Socket: "deck-tui-ack-no-server"}
	m.sessions = []store.Session{{ID: "sess-ack-2", Name: "ackfloor", Slug: "ackfloor", Status: "error"}}
	m.selected = rowCursor(0)
	calls := 0
	m.prepareAttach = func(context.Context, string) error { calls++; return nil }

	next, _ := m.enterInteractive()
	got := next.(Model)
	if got.interactive {
		t.Fatalf("enterInteractive entered interactive mode below the row floor")
	}
	if !strings.Contains(got.entryRefusal.reason, "floor") || got.entryRefusal.kind != entryRefusalRowFloor {
		t.Fatalf("entryRefusal = %+v, want the row-floor refusal", got.entryRefusal)
	}
	if calls != 0 {
		t.Fatalf("prepareAttach was called %d time(s) on a refused entry, want 0 -- a refusal must not answer or acknowledge the row", calls)
	}
}

// TestEnterInteractiveAttachmentFailureUnwinds pins the failure branch: a
// store write that cannot record the attachment refuses the entry the way
// attachSelected refuses the attach, and unwinds the claim it already made
// -- proven by the refusal itself (not interactive, the store's own error
// named) and by an immediately-following entry succeeding, which it could
// not do if the failed one still held window ownership (this test process
// is alive, so ClaimWindowOwnership's kill(pid, 0) liveness check would
// refuse a claim the unwind had leaked).
func TestEnterInteractiveAttachmentFailureUnwinds(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	socket := selectionTestSocket("ackunwind")
	newQuietSelectionPane(t, socket, "deck_ackunwind", 80, 24)
	m.tmuxClient = tmux.Client{Socket: socket}
	m.sessions = []store.Session{{ID: "sess-ack-3", Name: "ackunwind", Slug: "ackunwind", Status: "waiting"}}
	m.selected = rowCursor(0)
	m.prepareAttach = func(context.Context, string) error { return errors.New("state.db is locked") }

	next, _ := m.enterInteractive()
	got := next.(Model)
	if got.interactive {
		t.Fatalf("enterInteractive entered interactive mode despite the attachment record failing")
	}
	if !strings.Contains(got.entryRefusal.reason, "state.db is locked") || got.entryRefusal.kind != entryRefusalOther {
		t.Fatalf("entryRefusal = %+v, want it to carry the store's own error", got.entryRefusal)
	}

	got.prepareAttach = func(context.Context, string) error { return nil }
	next, _ = got.enterInteractive()
	reentered := next.(Model)
	if !reentered.interactive {
		t.Fatalf("re-entry after a failed attachment record was refused: %q -- the failed entry did not release its window claim", reentered.attachError)
	}
	reentered.exitInteractive()
}
