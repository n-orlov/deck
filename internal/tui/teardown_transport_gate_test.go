package tui

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestStolenFromTeardownIssuesNoPipePaneDisarm is the transport half of
// task 112's still-mine gate, and the one property the geometry/option
// test beside it cannot observe: a stolen-from holder's teardown must
// close ONLY its own end of its transport. tmux's disarm command is
// target-scoped (`pipe-pane -t target` disables whatever is currently
// piping that pane, whoever armed it -- see internal/tmux.PanePipe.Close),
// so an ungated teardown that closes its Session the ordinary way after
// its claim was stolen disarms the WINNER's live pipe as a side effect of
// tidying up its own already-lost one.
//
// The steal here takes the ownership option ALONE, with no second
// pipe-pane armed: that is what makes the hazard observable at all. In
// the full two-deck case the winner's own StartWithTransport displaces
// the loser's pipe first, so the loser's live path usually notices (EOF
// with #{pane_pipe} still 1) and performs PanePipe.CloseLocal on its own
// before exitInteractive ever runs, masking an ungated teardown behind a
// race. Stealing only the claim leaves the loser's pipe both armed and
// unnoticed, so exactly one thing decides whether #{pane_pipe} survives
// this teardown: whether the teardown consulted the probe BEFORE closing
// the transport.
func TestStolenFromTeardownIssuesNoPipePaneDisarm(t *testing.T) {
	socket := selectionTestSocket("stolenpipe")
	session := "deck_stolenpipe"
	newQuietSelectionPane(t, socket, session, 80, 24)
	client := tmux.Client{Socket: socket}
	ctx := context.Background()

	windowTarget, err := tmux.SessionName("stolenpipe")
	if err != nil {
		t.Fatalf("SessionName: %v", err)
	}

	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	if _, h := m.previewContentSize(); h < interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", h, interactiveMinInnerRows)
	}
	m.tmuxClient = client
	m.sessions = []store.Session{{ID: "sess-stolenpipe-1", Name: "stolenpipe", Slug: "stolenpipe", Status: "waiting"}}
	m.selected = rowCursor(0)

	next, _ := m.enterInteractiveBody(false)
	got := next.(Model)
	if !got.interactive {
		t.Fatalf("entry did not enter interactive mode: attachError=%q", got.attachError)
	}

	pane, ok, err := client.PreviewPane(ctx, "stolenpipe")
	if err != nil || !ok {
		t.Fatalf("PreviewPane: %v (ok=%v)", err, ok)
	}
	armed, err := client.PanePipe(ctx, pane.ID)
	if err != nil {
		t.Fatalf("PanePipe after entry: %v", err)
	}
	if !armed {
		t.Fatalf("test assumption violated: #{pane_pipe} is 0 right after a TransportPipe entry, so a wrongful disarm would be invisible")
	}

	// The steal: a second live claim on the same window, taken exactly the
	// way `F` takes one, and nothing else touched.
	winner, acquired, err := client.ForceClaimWindowOwnership(ctx, windowTarget)
	if err != nil {
		t.Fatalf("ForceClaimWindowOwnership (the steal): %v", err)
	}
	if !acquired {
		t.Fatalf("the steal did not acquire the claim")
	}
	winnerClaim, ok := readWindowOwnershipOptionForTest(t, socket, windowTarget)
	if !ok {
		t.Fatalf("%s unset after the steal", tmux.OwnershipOption)
	}

	// The stolen-from holder's own exit: it never observed the steal.
	got.exitInteractive()

	stillArmed, err := client.PanePipe(ctx, pane.ID)
	if err != nil {
		t.Fatalf("PanePipe after the stolen-from exit: %v", err)
	}
	if !stillArmed {
		t.Fatalf("#{pane_pipe} became 0 across the stolen-from exit: its teardown issued a target-scoped `pipe-pane` disarm, which in the real two-deck case takes the winner's own live transport down")
	}
	if after, ok := readWindowOwnershipOptionForTest(t, socket, windowTarget); !ok || after != winnerClaim {
		t.Fatalf("%s changed across the stolen-from exit: before=%q after=%q (set=%v), want the winner's claim untouched", tmux.OwnershipOption, winnerClaim, after, ok)
	}

	// Tidy up through the surviving claim only. The pipe this test
	// deliberately left armed is torn down with the tmux server itself by
	// newQuietSelectionPane's own cleanup.
	if err := winner.Release(ctx); err != nil {
		t.Fatalf("winner.Release: %v", err)
	}
}
