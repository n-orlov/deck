package features

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
)

// laggyCreateTUI is a scripted stand-in for the deck client behind a
// synthetic ScreenDriver: no deck binary, no tmux. It reads the keys a step
// sends through the driver's terminal, applies each to a small model of the
// main view and the create modal AT ONCE (as bubbletea's Update does), and
// paints the resulting view only lag later (as a render loop starved by
// -race or a loaded host does). Task 026 uses it to reproduce two
// step-sync defects the 20/20 -race loops hit only now and then
// (inventory §10): permission_modes.feature:29's sleep-paced field cycling
// and event_log.feature:11's shell-create wait returning before the
// requirement-52 selection.
type laggyCreateTUI struct {
	d   *ScreenDriver
	lag time.Duration

	mu        sync.Mutex
	modal     bool
	field     int
	agents    []string
	profiles  []string
	agent     int
	profile   int
	lastAgent int
	name, cwd string
	rows      []laggyRow
	selected  int
	// onSubmit runs with mu held when the modal sees Enter.
	onSubmit func(t *laggyCreateTUI)
	// selectionPainted names the row whose SELECTED state has been written
	// to the screen at least once (set before the write lands).
	selectionPainted map[string]bool

	renders chan time.Time
	closed  bool // guarded by mu; renders is closed once it is set
	wg      sync.WaitGroup
}

type laggyRow struct{ name, glyph, status string }

func startLaggyCreateTUI(t *testing.T, lag time.Duration, lastAgent int, rows []laggyRow) (*laggyCreateTUI, *ScreenDriver) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	d := &ScreenDriver{
		terminal: w,
		screen:   vt.NewEmulator(int(terminalColumns), int(terminalRows)),
		done:     make(chan struct{}),
		updated:  make(chan struct{}, 1),
	}
	go d.drainScreenInput()
	t.Cleanup(func() { closeInputPipe(d.screen) })
	tui := &laggyCreateTUI{
		d:                d,
		lag:              lag,
		agents:           []string{"shell", "claude", "pi"},
		profiles:         []string{"safe", "auto", "yolo"},
		lastAgent:        lastAgent,
		rows:             rows,
		selectionPainted: map[string]bool{},
		renders:          make(chan time.Time, 4096),
	}
	tui.wg.Add(2)
	go tui.readKeys(r)
	go tui.renderLoop()
	t.Cleanup(func() {
		_ = w.Close()
		tui.wg.Wait()
		_ = r.Close()
	})
	tui.requestRender()
	return tui, d
}

func (t *laggyCreateTUI) requestRender() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.closed {
		t.renders <- time.Now()
	}
}

func (t *laggyCreateTUI) readKeys(r *os.File) {
	defer t.wg.Done()
	defer func() {
		t.mu.Lock()
		t.closed = true
		close(t.renders)
		t.mu.Unlock()
	}()
	br := bufio.NewReader(r)
	for {
		b, err := br.ReadByte()
		if err != nil {
			return
		}
		key := string(b)
		if b == 0x1b {
			b1, err1 := br.ReadByte()
			b2, err2 := br.ReadByte()
			if err1 != nil || err2 != nil {
				return
			}
			key = string([]byte{b, b1, b2})
		}
		t.mu.Lock()
		t.apply(key)
		t.mu.Unlock()
		t.requestRender()
	}
}

func (t *laggyCreateTUI) apply(key string) {
	if !t.modal {
		if key == "n" {
			t.modal, t.field, t.agent, t.profile, t.name, t.cwd = true, 0, t.lastAgent, 0, "", ""
		}
		return
	}
	switch key {
	case "\x1b[B":
		t.field = min(t.field+1, 3)
	case "\x1b[A":
		t.field = max(t.field-1, 0)
	case "\x1b[C":
		switch t.field {
		case 2:
			t.agent = (t.agent + 1) % len(t.agents)
		case 3:
			t.profile = (t.profile + 1) % len(t.profiles)
		}
	case "\r":
		t.modal = false
		t.lastAgent = t.agent
		if t.onSubmit != nil {
			t.onSubmit(t)
		}
	default:
		switch t.field {
		case 0:
			t.name += key
		case 1:
			t.cwd += key
		}
	}
}

func (t *laggyCreateTUI) renderLoop() {
	defer t.wg.Done()
	for at := range t.renders {
		if wait := time.Until(at.Add(t.lag)); wait > 0 {
			time.Sleep(wait)
		}
		t.mu.Lock()
		frame := t.view()
		if !t.modal && t.selected >= 0 && t.selected < len(t.rows) {
			t.selectionPainted[t.rows[t.selected].name] = true
		}
		t.mu.Unlock()
		t.d.mu.Lock()
		_, _ = t.d.screen.Write([]byte("\x1b[H\x1b[2J" + frame))
		t.d.mu.Unlock()
		select {
		case t.d.updated <- struct{}{}:
		default:
		}
	}
}

func (t *laggyCreateTUI) view() string {
	line := func(text string) string { return fmt.Sprintf("│ %-75s │", text) }
	var lines []string
	if t.modal {
		marker := func(field int) string {
			if field == t.field {
				return "> "
			}
			return "  "
		}
		lines = append(lines,
			line("Create session"),
			line(marker(0)+"Name: "+t.name),
			line(marker(1)+"Working directory: "+t.cwd),
			line(marker(2)+"Agent: "+t.agents[t.agent]+" (left/right cycles: "+strings.Join(t.agents, ", ")+")"),
			line(marker(3)+"Permission profile: "+t.profiles[t.profile]+" (left/right cycles: "+strings.Join(t.profiles, ", ")+")"),
		)
		return strings.Join(lines, "\r\n")
	}
	side := func(text string) string { return fmt.Sprintf("│ %-34s│ %-53s │", text, "") }
	lines = append(lines, side(fmt.Sprintf("▾ default  (%d)", len(t.rows))))
	for i, row := range t.rows {
		gutter := "  "
		if i == t.selected {
			gutter = "> "
		}
		lines = append(lines, side(gutter+row.glyph+" "+row.name+" "+row.status), side("  just now"))
	}
	return strings.Join(lines, "\r\n")
}

func (t *laggyCreateTUI) state() (modal bool, field int, agent, profile string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.modal, t.field, t.agents[t.agent], t.profiles[t.profile]
}

// TestCreateModalFieldCyclingNeverOvershootsALaggingRender is task 026's
// regression test for permission_modes.feature:29 (1/20 -race at
// 2957f2c7fd: the third create's Permission profile cycle sent four right
// arrows ~32 ms apart and never saw "yolo"). positionCreateModalOnProfileField
// and ensureCreateModalAgent cycled with "send an arrow, sleep 25 ms, read
// the frame", so any render slower than that sleep made them read a stale
// value and send arrows past the wanted one. Against a client that renders
// every key correctly but 200 ms late, the step must still leave the modal
// on exactly the requested agent and profile.
func TestCreateModalFieldCyclingNeverOvershootsALaggingRender(t *testing.T) {
	for _, tc := range []struct {
		name      string
		lastAgent int // index into laggyCreateTUI.agents pre-selected by the modal
	}{
		{"profile field only (claude pre-selected)", 1},
		{"agent field then profile field (pi pre-selected)", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tui, d := startLaggyCreateTUI(t, 200*time.Millisecond, tc.lastAgent, nil)
			h := &ScenarioHarness{
				Home:         t.TempDir(),
				namedClients: map[string]*ScreenDriver{"A": d},
				workingDir:   t.TempDir(),
			}
			ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), scenarioHarnessKey{}, h), 8*time.Second)
			defer cancel()
			if _, _, err := positionCreateModalOnProfileField(ctx, "A", "claude", "lagging", "yolo"); err != nil {
				t.Fatalf("positionCreateModalOnProfileField against a client rendering 200 ms late: %v", err)
			}
			modal, field, agent, profile := tui.state()
			if !modal || field != 3 || agent != "claude" || profile != "yolo" {
				t.Fatalf("modal state after the step: open=%v field=%d agent=%q profile=%q, want open on field 3 (Permission profile) with agent \"claude\" and profile \"yolo\"", modal, field, agent, profile)
			}
		})
	}
}

// TestShellCreateStepWaitsForTheCreatedRowsSelection is task 026's
// regression test for event_log.feature:11 (1/20 -race at 1eacb1a347: the
// step after a shell create pressed "g" then "e" 9 ms apart and "e" was
// lost). A periodic reload can paint a just-created row before the TUI has
// processed the create's own result; requirement 52 moves the selection
// onto the row only after that. The create step must not return until the
// created row is shown selected, or the next navigation step's settle
// check mistakes the selection move for its own key's effect.
//
// Scripted here: Enter paints the new row (in "starting", not selected,
// another row still selected), and only 400 ms later moves the selection
// onto it.
func TestShellCreateStepWaitsForTheCreatedRowsSelection(t *testing.T) {
	const created = "fresh-shell"
	tui, d := startLaggyCreateTUI(t, 30*time.Millisecond, 0, []laggyRow{{"older", "~", "running"}})
	tui.onSubmit = func(t *laggyCreateTUI) {
		t.rows = append(t.rows, laggyRow{created, ".", "starting"})
		time.AfterFunc(400*time.Millisecond, func() {
			t.mu.Lock()
			t.selected = len(t.rows) - 1
			t.mu.Unlock()
			t.requestRender()
		})
	}
	h := &ScenarioHarness{
		Home:         t.TempDir(),
		namedClients: map[string]*ScreenDriver{"A": d},
	}
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), scenarioHarnessKey{}, h), 8*time.Second)
	defer cancel()
	if err := clientCreatesShellSession(ctx, "A", created); err != nil {
		t.Fatalf("clientCreatesShellSession: %v", err)
	}
	tui.mu.Lock()
	painted := tui.selectionPainted[created]
	tui.mu.Unlock()
	if !painted {
		t.Fatalf("the shell create step returned before the created row %q was ever painted selected (requirement 52); a following navigation step would start from a selection about to move\nframe at return:\n%s", created, d.Frame(false))
	}
	// Let the scheduled selection move land before the cleanup closes the pipe.
	time.Sleep(450 * time.Millisecond)
}
