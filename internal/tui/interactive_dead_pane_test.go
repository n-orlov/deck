package tui

import (
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// neverConsulted is a stillMine that fails the test if the verdict reads the
// claim at all: a dead pane is excluded from the claim check (R208).
func neverConsulted(t *testing.T) func(tmux.InteractiveTick) bool {
	t.Helper()
	return func(tmux.InteractiveTick) bool {
		t.Helper()
		t.Fatal("the claim check ran for a dead pane; PaneDead must skip it")
		return false
	}
}

// R208: a read that fails because the target no longer resolves reports a
// dead pane, never a takeover.
func TestDisplacementVerdictFailedReadIsDeadPaneNotDisplaced(t *testing.T) {
	got := interactiveDisplacementVerdict("deck_x", "x", tmux.InteractiveTick{}, errors.New("can't find pane"), neverConsulted(t))
	if !got.paneDead {
		t.Fatalf("a failed read left paneDead false: %+v", got)
	}
	if got.displaced {
		t.Fatalf("a failed read set displaced: %+v", got)
	}
	if got.windowTarget != "deck_x" || got.sessionName != "x" {
		t.Fatalf("verdict lost its target/name: %+v", got)
	}
}

// R208: a PaneDead tick skips the claim and attach checks even when both
// would otherwise say "displaced" (foreign claim, another client attached).
func TestDisplacementVerdictPaneDeadSkipsClaimAndAttachChecks(t *testing.T) {
	tick := tmux.InteractiveTick{PaneDead: true, Attached: 2, Owner: "other", OwnerSet: true}
	got := interactiveDisplacementVerdict("deck_x", "x", tick, nil, neverConsulted(t))
	if !got.paneDead || got.displaced {
		t.Fatalf("PaneDead tick = %+v, want paneDead only", got)
	}
}

// R208: a genuine takeover on a live pane still reports displaced, for both
// flavours (claim stolen, another client attached), and is not a dead pane.
func TestDisplacementVerdictGenuineTakeoverStillDisplaced(t *testing.T) {
	cases := []struct {
		name      string
		tick      tmux.InteractiveTick
		stillMine bool
		want      bool
	}{
		{"claim stolen", tmux.InteractiveTick{}, false, true},
		{"client attached", tmux.InteractiveTick{Attached: 1}, true, true},
		{"nothing wrong", tmux.InteractiveTick{}, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := interactiveDisplacementVerdict("deck_x", "x", tc.tick, nil, func(tmux.InteractiveTick) bool { return tc.stillMine })
			if got.displaced != tc.want || got.paneDead {
				t.Fatalf("verdict = %+v, want displaced=%v paneDead=false", got, tc.want)
			}
		})
	}
}

// R208, end to end against a real tmux server: the pane's session is killed
// while interactive. The next previewTick's backstop poll reports a dead
// pane and not a displacement, and feeding it back through Update raises no
// "Lost attach" dialog, stays in interactive mode, and reports the death to
// the grid (Session.NotePaneDead), which is the dead-pane path the operator
// sees instead.
func TestPreviewTickDeadPaneIsNotLostAttach(t *testing.T) {
	socket := selectionTestSocket("deadnotlost")
	newQuietSelectionPane(t, socket, "deck_deadnotlost", 80, 24)
	client := tmux.Client{Socket: socket}

	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	m.tmuxClient = client
	m.sessions = []store.Session{{ID: "sess-dead-1", Name: "deadnotlost", Slug: "deadnotlost", Status: "waiting"}}
	m.selected = rowCursor(0)

	next, _ := m.enterInteractiveBody(false)
	got := next.(Model)
	if !got.interactive {
		t.Fatalf("entry did not enter interactive mode: attachError=%q", got.attachError)
	}
	defer got.exitInteractive()

	if err := exec.Command("tmux", "-L", socket, "kill-session", "-t", "deck_deadnotlost").Run(); err != nil {
		t.Fatalf("kill-session: %v", err)
	}

	_, cmd := got.Update(previewTick(time.Now()))
	checked := findInteractiveDisplacementChecked(t, cmd)
	if !checked.paneDead || checked.displaced {
		t.Fatalf("backstop poll against a vanished pane = %+v, want paneDead=true displaced=false", checked)
	}

	updated, _ := got.Update(checked)
	after := updated.(Model)
	if after.lostAttach {
		t.Fatal("a dead pane raised the Lost attach dialog")
	}
	if !after.interactive {
		t.Fatal("a dead pane left interactive mode through the takeover exit")
	}
	if !waitForDisplacementTest(t, time.Second, func() bool {
		select {
		case <-got.interactiveGrid.Dead():
			return true
		default:
			return false
		}
	}) {
		t.Fatal("a dead pane was not reported to the grid through NotePaneDead")
	}
}
