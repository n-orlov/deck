package tui

import (
	"context"
	"io"
	"os/exec"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/tmux"
)

// R321 in a full attach (`a`): attach_engagement.go's watcher reports the
// first forwarded key, scroll or screen-content change, and onAttachFinished
// hands it to engageRow -- the same one function as the interactive preview.

// --- the detector, sample by sample ---------------------------------------

func attachSampleAt(width, height int, screen string) tmux.AttachSample {
	return tmux.AttachSample{Width: width, Height: height, ClientFound: true, ClientActivity: 100, ClientKeyTable: "root", Screen: []byte(screen)}
}

func TestAttachDetectorKeyIsConfirmedByTheNextSample(t *testing.T) {
	d := attachEngagementDetector{settle: time.Second}
	t0 := time.Unix(1000, 0)
	d.observe(t0, attachSampleAt(80, 24, "prompt"))
	bump := attachSampleAt(80, 24, "prompt x")
	bump.ClientActivity = 101
	if got := d.observe(t0.Add(2*time.Second), bump); got != "" {
		t.Fatalf("the bump sample reported %q, want a pending key", got)
	}
	if got := d.observe(t0.Add(2200*time.Millisecond), bump); got != engageKey {
		t.Fatalf("the confirming sample reported %q, want %q", got, engageKey)
	}
}

func TestAttachDetectorPrefixAndDetachAreNotKeys(t *testing.T) {
	d := attachEngagementDetector{settle: time.Second}
	t0 := time.Unix(1000, 0)
	d.observe(t0, attachSampleAt(80, 24, "prompt"))
	prefix := attachSampleAt(80, 24, "prompt")
	prefix.ClientActivity, prefix.ClientKeyTable = 101, "prefix"
	if got := d.observe(t0.Add(2*time.Second), prefix); got != "" {
		t.Fatalf("a pending prefix reported %q", got)
	}
	// The `d` of prefix+d: the server may still list the detaching client
	// in the root table with its activity moved; it is gone a sample later.
	detaching := attachSampleAt(80, 24, "prompt")
	detaching.ClientActivity = 102
	if got := d.observe(t0.Add(2200*time.Millisecond), detaching); got != "" {
		t.Fatalf("the detaching sample reported %q", got)
	}
	gone := attachSampleAt(80, 24, "prompt")
	gone.ClientFound = false
	if got := d.observe(t0.Add(2400*time.Millisecond), gone); got != "" {
		t.Fatalf("a detach was reported as %q", got)
	}
}

func TestAttachDetectorScrollIsCopyMode(t *testing.T) {
	d := attachEngagementDetector{settle: time.Second}
	t0 := time.Unix(1000, 0)
	d.observe(t0, attachSampleAt(80, 24, "prompt"))
	scrolled := attachSampleAt(80, 24, "prompt")
	scrolled.InMode, scrolled.ScrollPosition, scrolled.ClientActivity = true, "3", 101
	if got := d.observe(t0.Add(200*time.Millisecond), scrolled); got != engageScroll {
		t.Fatalf("entering copy mode reported %q, want %q", got, engageScroll)
	}
}

func TestAttachDetectorScreenChangeCountsOnlyAfterTheResizeSettles(t *testing.T) {
	d := attachEngagementDetector{settle: time.Second}
	t0 := time.Unix(1000, 0)
	d.observe(t0, attachSampleAt(80, 24, "prompt"))
	// The attach's own resize, and the agent's redraws for it, are deck's.
	if got := d.observe(t0.Add(200*time.Millisecond), attachSampleAt(120, 40, "prompt reflowed")); got != "" {
		t.Fatalf("the attach resize reported %q", got)
	}
	if got := d.observe(t0.Add(900*time.Millisecond), attachSampleAt(120, 40, "prompt redrawn")); got != "" {
		t.Fatalf("the redraw for the resize reported %q", got)
	}
	if got := d.observe(t0.Add(1500*time.Millisecond), attachSampleAt(120, 40, "prompt redrawn")); got != "" {
		t.Fatalf("an unchanged screen reported %q", got)
	}
	if got := d.observe(t0.Add(1700*time.Millisecond), attachSampleAt(120, 40, "prompt answered")); got != engageScreen {
		t.Fatalf("a settled screen change reported %q, want %q", got, engageScreen)
	}
}

// --- the watcher against a real tmux client on a pty ----------------------

type liveAttach struct {
	socket, name string
	terminal     io.Writer
	done         chan error
	watched      *watchedAttach
}

// startLiveAttach runs a real `tmux attach-session` client on a pty against a
// quiet pane through watchedAttach, and waits until tmux lists the client and
// its first second of activity is over (client_activity has 1 s resolution).
func startLiveAttach(t *testing.T) *liveAttach {
	t.Helper()
	socket := selectionTestSocket("engage")
	slug := "engage-attach"
	name, err := tmux.SessionName(slug)
	if err != nil {
		t.Fatal(err)
	}
	newQuietSelectionPane(t, socket, name, 80, 24)
	client := tmux.Client{Socket: socket}
	cmd, err := client.AttachCommand(context.Background(), slug)
	if err != nil {
		t.Fatal(err)
	}
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ptmx.Close(); _ = tty.Close() })
	if err := pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
		t.Fatal(err)
	}
	go func() { _, _ = io.Copy(io.Discard, ptmx) }()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
	cmd.Env = append(cmd.Env, "TERM=xterm-256color")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	w := &watchedAttach{cmd: cmd, slug: slug, sample: client.AttachSample, interval: 50 * time.Millisecond, settle: 300 * time.Millisecond}
	live := &liveAttach{socket: socket, name: name, terminal: ptmx, done: make(chan error, 1), watched: w}
	go func() { live.done <- w.Run() }()
	t.Cleanup(func() { live.detach(t) })
	deadline := time.Now().Add(5 * time.Second)
	for {
		out, _ := exec.Command("tmux", "-L", socket, "list-clients", "-t", "="+name).CombinedOutput()
		if strings.TrimSpace(string(out)) != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the attach client never appeared")
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(1100 * time.Millisecond)
	return live
}

func (l *liveAttach) tmux(t *testing.T, args ...string) {
	t.Helper()
	if out, err := exec.Command("tmux", append([]string{"-L", l.socket}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("tmux %v: %v: %s", args, err, out)
	}
}

// detach ends the attach the way the user's detach does and returns the
// trigger the watcher reported.
func (l *liveAttach) detach(t *testing.T) engagementTrigger {
	t.Helper()
	if l.done == nil {
		return l.watched.Trigger
	}
	_ = exec.Command("tmux", "-L", l.socket, "detach-client", "-s", "="+l.name).Run()
	select {
	case <-l.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the attach client did not exit on detach")
	}
	l.done = nil
	return l.watched.Trigger
}

func TestFullAttachWithoutEngagementReportsNothing(t *testing.T) {
	live := startLiveAttach(t)
	time.Sleep(500 * time.Millisecond)
	if got := live.detach(t); got != "" {
		t.Fatalf("an attach the user only looked at reported %q (R320: attach is inert)", got)
	}
}

// TestFullAttachDetachByKeyReportsNothing: the user's own prefix+d is two
// keys tmux consumes, never a forwarded key, so an attach ended that way
// stays inert (R320).
func TestFullAttachDetachByKeyReportsNothing(t *testing.T) {
	live := startLiveAttach(t)
	if _, err := live.terminal.Write([]byte{0x02}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := live.terminal.Write([]byte("d")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-live.done:
		live.done = nil
	case <-time.After(5 * time.Second):
		t.Fatal("prefix+d did not detach the client")
	}
	if got := live.watched.Trigger; got != "" {
		t.Fatalf("an attach ended by prefix+d reported %q", got)
	}
}

func TestFullAttachFirstForwardedKeyIsReported(t *testing.T) {
	live := startLiveAttach(t)
	if _, err := live.terminal.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if got := live.detach(t); got != engageKey {
		t.Fatalf("a typed key reported %q, want %q", got, engageKey)
	}
}

func TestFullAttachFirstScrollIsReported(t *testing.T) {
	live := startLiveAttach(t)
	live.tmux(t, "copy-mode", "-t", "="+live.name+":")
	time.Sleep(300 * time.Millisecond)
	if got := live.detach(t); got != engageScroll {
		t.Fatalf("a scroll reported %q, want %q", got, engageScroll)
	}
}

func TestFullAttachFirstScreenContentChangeIsReported(t *testing.T) {
	live := startLiveAttach(t)
	live.tmux(t, "send-keys", "-t", "="+live.name+":", "-l", "the agent printed this")
	time.Sleep(500 * time.Millisecond)
	if got := live.detach(t); got != engageScreen {
		t.Fatalf("a screen change reported %q, want %q", got, engageScreen)
	}
}

// --- the Model: a full attach's trigger goes through engageRow ------------

// fullAttachEngagementCmd plays a full attach of the selected row ending with
// the watcher's trigger, and returns the engagement write cmd onAttachFinished
// issued (nil when it issued none).
func fullAttachEngagementCmd(t *testing.T, m Model, trigger engagementTrigger) tea.Cmd {
	t.Helper()
	session, ok := m.selectedSession()
	if !ok {
		t.Fatal("no selected row")
	}
	m.attachingSessionID = session.ID
	_, cmd := m.Update(attachFinished{engaged: trigger})
	var found tea.Msg
	var walk func(tea.Cmd)
	walk = func(c tea.Cmd) {
		if c == nil {
			return
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			for _, inner := range msg {
				walk(inner)
			}
		case sessionEngaged:
			found = msg
		}
	}
	walk(cmd)
	if found == nil {
		return nil
	}
	return func() tea.Msg { return found }
}

func TestFullAttachTriggersSetSeenThroughEngageRow(t *testing.T) {
	for _, trigger := range []engagementTrigger{engageKey, engageScroll, engageScreen} {
		for _, id := range engagementRows {
			t.Run(string(trigger)+"/"+id, func(t *testing.T) {
				rig := engagementFixture(t, SortOrderAttention, id, false)
				before := snapshotDurable(t, rig.db, id)
				orderBefore := inertOrder(rig.m)
				m := settleEngagement(t, rig.m, fullAttachEngagementCmd(t, rig.m, trigger))
				assertSeenOnly(t, "a full attach's "+string(trigger), rig.db, id, before)
				if *rig.writes != 1 {
					t.Fatalf("the full attach wrote %d times, want 1", *rig.writes)
				}
				if got := inertOrder(m); strings.Join(got, ",") != strings.Join(orderBefore, ",") {
					t.Fatalf("the full attach's engagement moved a row: before=%v after=%v", orderBefore, got)
				}
				// A second full attach with engagement writes nothing.
				if cmd := fullAttachEngagementCmd(t, m, trigger); cmd != nil {
					t.Fatal("a second full-attach engagement issued a write")
				}
				if *rig.writes != 1 {
					t.Fatalf("a second full-attach engagement reached the store: %d writes", *rig.writes)
				}
			})
		}
	}
}

func TestFullAttachWithoutTriggerWritesNothing(t *testing.T) {
	rig := engagementFixture(t, SortOrderAttention, "row-waiting", false)
	before := snapshotAll(t, rig.db)
	if cmd := fullAttachEngagementCmd(t, rig.m, ""); cmd != nil {
		t.Fatal("an attach with no engagement issued a write")
	}
	if *rig.writes != 0 {
		t.Fatalf("an attach with no engagement wrote %d times", *rig.writes)
	}
	for id, want := range before {
		if got := snapshotDurable(t, rig.db, id); !reflect.DeepEqual(got, want) {
			t.Fatalf("an attach with no engagement changed %s", id)
		}
	}
}
