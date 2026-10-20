package tui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// reflowPane is a fake pane in front of a real, quiet tmux session (R412).
// The click under test resizes the real window, as it does in production. The
// pane's program is fake: its capture is a claude-style transcript whose text
// is wrapped at the pane's width right now, so a resize reflows it the way a
// real agent's redraw does, and its window_activity is whatever the test says.
// The script serves the service's two reads (list-panes and capture-pane) and
// leaves every other tmux command to the real binary.
type reflowPane struct {
	t      *testing.T
	dir    string
	socket string
	client tmux.Client // the service's client: through the script
}

const reflowScript = `#!/bin/sh
sock="$2"
case " $* " in
*" list-panes "*)
	act=$(cat '@DIR@/activity')
	tmux "$@" | awk -F'|' -v OFS='|' -v a="$act" '{ $NF = a; print }'
	;;
*" capture-pane "*)
	pane=''; prev=''
	for a in "$@"; do
		if [ "$prev" = '-t' ]; then pane="$a"; break; fi
		prev="$a"
	done
	w=$(tmux -L "$sock" display-message -p -t "$pane" '#{pane_width}')
	printf '%s\n' "● $(cat '@DIR@/transcript')" | fold -s -w "$w"
	rule='────────────────────────────────'
	printf '%s\n%s\n%s\n%s\n' "$rule" '❯ ' "$rule" "$(cat '@DIR@/footer')"
	tmux -L "$sock" display-message -p -t "$pane" 'deck-pane-size:#{pane_width}x#{pane_height}'
	;;
*)
	exec tmux "$@"
	;;
esac
`

func newReflowPane(t *testing.T, socket string) *reflowPane {
	t.Helper()
	dir := t.TempDir()
	p := &reflowPane{t: t, dir: dir, socket: socket}
	script := filepath.Join(dir, "fake-tmux")
	body := strings.ReplaceAll(reflowScript, "@DIR@", dir)
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	p.client = tmux.Client{Binary: script, Socket: socket}
	p.set("transcript", "Reading the reconcile loop and the output clock to see how a resized pane "+
		"reflows its transcript before any agent output has arrived at all")
	p.set("footer", "~/proj  main  ctx:12%  opus")
	p.set("activity", "1700000000")
	return p
}

func (p *reflowPane) set(name, content string) {
	p.t.Helper()
	if err := os.WriteFile(filepath.Join(p.dir, name), []byte(content), 0o600); err != nil {
		p.t.Fatal(err)
	}
}

// width is the real pane's width right now.
func (p *reflowPane) width() int {
	p.t.Helper()
	out, err := exec.Command("tmux", "-L", p.socket, "display-message", "-p", "#{pane_width}").Output()
	if err != nil {
		p.t.Fatalf("read pane width: %v", err)
	}
	width, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		p.t.Fatalf("pane width %q: %v", out, err)
	}
	return width
}

// observe is one output-clock reading of the idle row's pane at window_activity
// seconds, through the real service and the real clock.
func (p *reflowPane) observe(svc service.Service, db *store.Store, activity int64) {
	p.t.Helper()
	p.set("activity", strconv.FormatInt(activity, 10))
	row, err := db.GetSession(context.Background(), "row-idle")
	if err != nil {
		p.t.Fatal(err)
	}
	if err := svc.ObserveAttachedOutput(context.Background(), row); err != nil {
		p.t.Fatalf("observe at %d: %v", activity, err)
	}
}

// clickInertOnActivityOrder is TestClickIsInertOnStatusSeenAndPosition's
// `activity` case: the sidebar is in activity order, the clicked row is idle
// and sits below the others, and its pane reflows when the click resizes it.
// A counted write would move it to the top, so neither last_output_at nor the
// position may change, after the click or after footer churn in a later
// bucket.
func clickInertOnActivityOrder(t *testing.T) {
	m, db := inertFixture(t, SortOrderActivity, "row-running", true)
	// The idle row has the oldest output, so a write to it moves it up.
	if err := db.RecordOutput(context.Background(), "row-idle", 5_000); err != nil {
		t.Fatal(err)
	}
	m = reloadInert(t, m)
	m = inertPane(t, m, db, "row-idle")
	pane := newReflowPane(t, m.tmuxClient.Socket)
	clock := service.NewOutputClock(service.DefaultOutputBucket)
	m = m.WithOutputDiscounter(clock)
	svc := service.Service{Store: db, TMux: pane.client, Output: clock}

	const base = int64(1_700_000_000)
	pane.observe(svc, db, base) // the clock's baseline: counts nothing
	before := snapshotAll(t, db)
	orderBefore := inertOrder(m)
	index := -1
	for i, id := range orderBefore {
		if id == "row-idle" {
			index = i
		}
	}
	if index != len(orderBefore)-1 {
		t.Fatalf("fixture: row-idle must be last in the activity order, got %v", orderBefore)
	}
	widthBefore := pane.width()

	x, y := findRow(t, m, index)
	updated, _ := m.Update(press(x, y))
	m = updated.(Model)
	if !m.interactive {
		t.Fatalf("click did not enter interactive mode: %q %+v", m.attachError, m.entryRefusal)
	}
	if got := pane.width(); got == widthBefore {
		t.Fatalf("fixture: the click left the pane %d wide; the capture would not reflow", got)
	}
	// The resized program redraws: window_activity moves into a new bucket.
	pane.observe(svc, db, base+10)
	assertInert(t, "click on an idle row, then its resize's reflow", m, db, orderBefore, before)

	pane.set("footer", "~/proj  feature  ctx:41%  sonnet")
	pane.observe(svc, db, base+25)
	assertInert(t, "footer churn after the click", m, db, orderBefore, before)

	updated, _ = m.Update(release(x, y))
	m = updated.(Model)
	m.exitInteractive()
	pane.observe(svc, db, base+40)
	assertInert(t, "click release, exit and the reflow back", m, db, orderBefore, before)
}
