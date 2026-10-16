package tui

import (
	"context"
	"io"
	"os/exec"
	"reflect"
	"strings"
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
	return tmux.AttachSample{Width: width, Height: height, Prefix: "C-b", Prefix2: "None", ClientFound: true, ClientKeyTable: "root", Screen: []byte(screen)}
}

// preAttachSample is the baseline Run samples before the client exists: an
// 80x24 pane showing "prompt".
func preAttachSample() tmux.AttachSample {
	s := attachSampleAt(80, 24, "prompt")
	s.ClientFound, s.ClientKeyTable = false, ""
	return s
}

// A forwarded key counts the moment it is seen, confirmed by the next
// sample: no clock resolution is involved, so a key in the attach's first
// instant is a key.
func TestAttachDetectorKeyIsConfirmedByTheNextSample(t *testing.T) {
	d := attachEngagementDetector{settle: time.Second}
	t0 := time.Unix(1000, 0)
	d.observe(t0, preAttachSample(), 0)
	if got := d.observe(t0.Add(10*time.Millisecond), attachSampleAt(80, 24, "prompt x"), 1); got != "" {
		t.Fatalf("the sample after the key reported %q, want a pending key", got)
	}
	if got := d.observe(t0.Add(210*time.Millisecond), attachSampleAt(80, 24, "prompt x"), 1); got != engageKey {
		t.Fatalf("the confirming sample reported %q, want %q", got, engageKey)
	}
}

func TestAttachDetectorPrefixAndDetachAreNotKeys(t *testing.T) {
	d := attachEngagementDetector{settle: time.Second}
	t0 := time.Unix(1000, 0)
	d.observe(t0, preAttachSample(), 0)
	// A key the proxy could not name as the prefix leaves the client in the
	// prefix table: not a forwarded key.
	if got := d.observe(t0.Add(200*time.Millisecond), attachSampleAt(80, 24, "prompt"), 1); got != "" {
		t.Fatalf("a pending key reported %q", got)
	}
	prefix := attachSampleAt(80, 24, "prompt")
	prefix.ClientKeyTable = "prefix"
	if got := d.observe(t0.Add(400*time.Millisecond), prefix, 1); got != "" {
		t.Fatalf("a key in the prefix table reported %q", got)
	}
	// A key followed by the client's departure is a detach.
	if got := d.observe(t0.Add(600*time.Millisecond), attachSampleAt(80, 24, "prompt"), 2); got != "" {
		t.Fatalf("a pending key reported %q", got)
	}
	gone := attachSampleAt(80, 24, "prompt")
	gone.ClientFound = false
	if got := d.observe(t0.Add(800*time.Millisecond), gone, 2); got != "" {
		t.Fatalf("a detach was reported as %q", got)
	}
}

func TestAttachDetectorScrollIsCopyMode(t *testing.T) {
	d := attachEngagementDetector{settle: time.Second}
	t0 := time.Unix(1000, 0)
	d.observe(t0, preAttachSample(), 0)
	scrolled := attachSampleAt(80, 24, "prompt")
	scrolled.InMode, scrolled.ScrollPosition = true, "3"
	if got := d.observe(t0.Add(200*time.Millisecond), scrolled, 0); got != engageScroll {
		t.Fatalf("entering copy mode reported %q, want %q", got, engageScroll)
	}
}

func TestAttachDetectorScreenChangeCountsOnlyAfterTheResizeSettles(t *testing.T) {
	d := attachEngagementDetector{settle: time.Second}
	t0 := time.Unix(1000, 0)
	d.observe(t0, preAttachSample(), 0)
	// The attach's own resize, and the agent's redraws for it, are deck's.
	if got := d.observe(t0.Add(200*time.Millisecond), attachSampleAt(120, 40, "prompt reflowed"), 0); got != "" {
		t.Fatalf("the attach resize reported %q", got)
	}
	if got := d.observe(t0.Add(900*time.Millisecond), attachSampleAt(120, 40, "prompt redrawn"), 0); got != "" {
		t.Fatalf("the redraw for the resize reported %q", got)
	}
	if got := d.observe(t0.Add(1500*time.Millisecond), attachSampleAt(120, 40, "prompt redrawn"), 0); got != "" {
		t.Fatalf("an unchanged screen reported %q", got)
	}
	if got := d.observe(t0.Add(1700*time.Millisecond), attachSampleAt(120, 40, "prompt answered"), 0); got != engageScreen {
		t.Fatalf("a settled screen change reported %q, want %q", got, engageScreen)
	}
}

// An attach that does not resize the window has nothing to settle: a screen
// change in its first instant is a screen change.
func TestAttachDetectorScreenChangeWithoutResizeCountsAtOnce(t *testing.T) {
	d := attachEngagementDetector{settle: time.Second}
	t0 := time.Unix(1000, 0)
	d.observe(t0, preAttachSample(), 0)
	if got := d.observe(t0.Add(50*time.Millisecond), attachSampleAt(80, 24, "prompt"), 0); got != "" {
		t.Fatalf("an unchanged screen reported %q", got)
	}
	if got := d.observe(t0.Add(250*time.Millisecond), attachSampleAt(80, 24, "prompt answered"), 0); got != engageScreen {
		t.Fatalf("a screen change in the first second reported %q, want %q", got, engageScreen)
	}
}

// Without a pre-attach baseline the first sample is already attached: its
// resize may have happened, so the settle starts there.
func TestAttachDetectorLiveFirstSampleSettles(t *testing.T) {
	d := attachEngagementDetector{settle: time.Second}
	t0 := time.Unix(1000, 0)
	d.observe(t0, attachSampleAt(80, 24, "prompt"), 0)
	if got := d.observe(t0.Add(200*time.Millisecond), attachSampleAt(80, 24, "prompt redrawn"), 0); got != "" {
		t.Fatalf("a redraw inside the settle reported %q", got)
	}
}

// --- the key proxy: which forwarded bytes are keys ------------------------

func TestAttachKeyProxyCountsOnlyForwardedKeys(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		key   bool
	}{
		{"a printable key", "x", true},
		{"an arrow key", "\x1b[A", true},
		{"enter", "\r", true},
		{"a bracketed paste", "\x1b[200~hello\x1b[201~", true},
		{"the prefix alone", "\x02", false},
		{"prefix+d (detach)", "\x02d", false},
		{"prefix+[ then a key", "\x02[" + "x", true},
		{"an SGR mouse click", "\x1b[<0;10;5M\x1b[<0;10;5m", false},
		{"a mouse wheel", "\x1b[<64;10;5M", false},
		{"focus in and out", "\x1b[I\x1b[O", false},
		{"a primary device attributes reply", "\x1b[?62;22c", false},
		{"a terminal version reply", "\x1bP>|xterm(390)\x1b\\", false},
		{"a background colour reply", "\x1b]11;rgb:0000/0000/0000\x07", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newAttachKeyProxy("C-b", "None")
			p.forwarded([]byte(tc.input))
			if got := p.keys.Load(); (got > 0) != tc.key {
				t.Fatalf("%q counted %d keys, want a key: %v", tc.input, got, tc.key)
			}
		})
	}
}

func TestAttachKeyProxyHonoursACustomPrefix(t *testing.T) {
	p := newAttachKeyProxy("C-a", "M-Space")
	p.forwarded([]byte("\x01d"))
	p.forwarded([]byte("\x1b d"))
	if got := p.keys.Load(); got != 0 {
		t.Fatalf("custom prefixes and the keys after them counted %d keys", got)
	}
	p.forwarded([]byte("\x02"))
	if got := p.keys.Load(); got != 1 {
		t.Fatalf("C-b under prefix C-a counted %d keys, want 1", got)
	}
}

func TestTmuxKeyStroke(t *testing.T) {
	for name, want := range map[string]string{
		"C-b": "ctrl+b", "C-a": "ctrl+a", "M-a": "alt+a", "C-Space": "ctrl+space",
		"`": "`", "F12": "f12", "C-M-x": "ctrl+alt+x", "Escape": "esc",
	} {
		if got, ok := tmuxKeyStroke(name); !ok || got != want {
			t.Errorf("tmuxKeyStroke(%q) = %q, %v; want %q", name, got, ok, want)
		}
	}
	for _, name := range []string{"None", "", "Q-x", "PPage"} {
		if got, ok := tmuxKeyStroke(name); ok {
			t.Errorf("tmuxKeyStroke(%q) = %q, want no keystroke", name, got)
		}
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
// quiet pane through watchedAttach, and waits only until tmux lists the
// client: the user's first key may come at once.
func startLiveAttach(t *testing.T) *liveAttach {
	t.Helper()
	return startLiveAttachSized(t, 24)
}

// startLiveAttachSized is startLiveAttach with the pane created paneHeight
// rows tall; the client's terminal is 80x24, so a 24-row pane is resized by
// the attach (tmux's status line takes a row) and a 23-row one is not.
func startLiveAttachSized(t *testing.T, paneHeight int) *liveAttach {
	t.Helper()
	socket := selectionTestSocket("engage")
	slug := "engage-attach"
	name, err := tmux.SessionName(slug)
	if err != nil {
		t.Fatal(err)
	}
	newQuietSelectionPane(t, socket, name, 80, paneHeight)
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
	cmd.Env = append(cmd.Env, "TERM=xterm-256color")
	// The test's pty plays the user's terminal; watchedAttach runs the
	// client on a pty of its own and proxies this one to it.
	w := &watchedAttach{cmd: cmd, slug: slug, sample: client.AttachSample, interval: attachWatchInterval, settle: attachWatchSettle, stdin: tty, stdout: tty}
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
	for _, gap := range []time.Duration{0, 300 * time.Millisecond} {
		t.Run(gap.String(), func(t *testing.T) {
			live := startLiveAttach(t)
			if _, err := live.terminal.Write([]byte{0x02}); err != nil {
				t.Fatal(err)
			}
			time.Sleep(gap)
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
		})
	}
}

// The key is typed the moment tmux lists the client -- inside the attach's
// first clock second, where tmux's own client_activity cannot see it.
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
	t.Run("no resize: at once", func(t *testing.T) {
		live := startLiveAttachSized(t, 23)
		live.tmux(t, "send-keys", "-t", "="+live.name+":", "-l", "the agent printed this")
		time.Sleep(500 * time.Millisecond)
		if got := live.detach(t); got != engageScreen {
			t.Fatalf("a screen change reported %q, want %q", got, engageScreen)
		}
	})
	t.Run("after the attach's resize settles", func(t *testing.T) {
		live := startLiveAttachSized(t, 24)
		time.Sleep(attachWatchSettle + 2*attachWatchInterval)
		live.tmux(t, "send-keys", "-t", "="+live.name+":", "-l", "the agent printed this")
		time.Sleep(500 * time.Millisecond)
		if got := live.detach(t); got != engageScreen {
			t.Fatalf("a screen change reported %q, want %q", got, engageScreen)
		}
	})
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
