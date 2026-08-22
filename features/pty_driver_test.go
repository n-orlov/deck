package features

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
)

// ScreenDriver drives a released deck binary through a real PTY.  It is kept
// in the feature-test package so feature steps can share it without granting
// the product any test-only surface.
type ScreenDriver struct {
	terminal *os.File
	cmd      *exec.Cmd
	screen   vt.Terminal
	// budget is a shadow capture of the exact same byte stream as screen,
	// but sized frameBudgetMargin columns and rows beyond the real terminal
	// so a client's own overflow (requirement 6/37: more rows or wider
	// lines than its terminal advertises) lands intact on cells past the
	// real budget rather than being autoscrolled off the top or
	// autowrapped onto the next row the way the real, correctly sized
	// screen emulator would -- see FrameFitsBudget.
	budget vt.Terminal

	mu          sync.Mutex
	raw         bytes.Buffer
	updated     chan struct{}
	done        chan struct{}
	readDone    chan struct{}
	exitErr     error // guarded by mu; done is closed after it is assigned
	oscAnswered bool  // guarded by mu
	cprAnswered bool  // guarded by mu
	// snapshots holds named frame captures for steps that assert a mouse
	// gesture changed nothing (requirement 2). Guarded by mu.
	snapshots map[string]string
	// sent is a diagnostic-only (task 014) record of every Send call, so a
	// hang's error message can show the exact input timeline leading up to
	// it. Guarded by mu.
	sent []sentRecord
}

// sentRecord is one entry in ScreenDriver.sent.
type sentRecord struct {
	at    time.Time
	bytes string
}

// frameBudgetMargin is how far beyond the real terminal's own column/row
// count ScreenDriver's shadow budget emulator is sized, so that content a
// buggy render pushes past the real budget still lands on a real, readable
// cell instead of triggering the shadow's own autoscroll/autowrap.
const frameBudgetMargin = 200

const (
	terminalColumns uint16 = 100
	terminalRows    uint16 = 30
)

// StartScreenDriver starts binary with the harness's default terminal
// geometry (100x30). It answers Bubble Tea's OSC 11 and CPR probes as soon as
// they are observed; a PTY alone is only a byte transport and otherwise
// leaves Bubble Tea waiting for frame 1. See StartScreenDriverWithSize for a
// scenario that needs a different starting geometry or a mid-scenario resize
// (requirement 1).
func StartScreenDriver(ctx context.Context, binary string, env []string) (*ScreenDriver, error) {
	return StartScreenDriverWithSize(ctx, binary, env, terminalColumns, terminalRows)
}

// StartScreenDriverWithSize is StartScreenDriver with an explicit initial PTY
// and emulator geometry.
func StartScreenDriverWithSize(ctx context.Context, binary string, env []string, cols, rows uint16) (*ScreenDriver, error) {
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(), env...)
	cmd.Env = append(cmd.Env, "TERM=xterm-256color", fmt.Sprintf("COLUMNS=%d", cols), fmt.Sprintf("LINES=%d", rows))
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: rows, Cols: cols})
	if err != nil {
		return nil, fmt.Errorf("start %s in pty: %w", binary, err)
	}
	d := &ScreenDriver{
		terminal: terminal,
		cmd:      cmd,
		screen:   vt.NewEmulator(int(cols), int(rows)),
		budget:   vt.NewEmulator(int(cols)+frameBudgetMargin, int(rows)+frameBudgetMargin),
		updated:  make(chan struct{}, 1),
		done:     make(chan struct{}),
		readDone: make(chan struct{}),
	}
	go d.read()
	go d.drainScreenInput()
	go d.drainBudgetInput()
	go func() {
		err := cmd.Wait()
		d.mu.Lock()
		d.exitErr = err
		d.mu.Unlock()
		close(d.done)
	}()
	return d, nil
}

// Resize changes the PTY's real kernel geometry via TIOCSWINSZ (creack/pty's
// Setsize), which is what makes the kernel deliver SIGWINCH to deck's
// foreground process group on its own -- ScreenDriver never raises the
// signal itself, matching how a real terminal emulator resize behaves. It
// then resizes the emulator's own grid so a subsequent Frame/GridSize call
// reads the screen at the new geometry rather than one still shaped for the
// old size (requirement 1).
func (d *ScreenDriver) Resize(cols, rows uint16) error {
	if err := pty.Setsize(d.terminal, &pty.Winsize{Rows: rows, Cols: cols}); err != nil {
		return fmt.Errorf("resize pty to %dx%d: %w", cols, rows, err)
	}
	d.mu.Lock()
	d.screen.Resize(int(cols), int(rows))
	d.budget.Resize(int(cols)+frameBudgetMargin, int(rows)+frameBudgetMargin)
	d.mu.Unlock()
	select {
	case d.updated <- struct{}{}:
	default:
	}
	return nil
}

// GridSize returns the emulator's current column and row count, letting a
// step prove a resize actually changed the grid a frame is read from rather
// than merely that the resize step ran without error.
func (d *ScreenDriver) GridSize() (cols, rows int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.screen.Width(), d.screen.Height()
}

// CellAt returns the single grid cell at (x, y) directly from the emulator,
// bypassing Frame's NormalizeFrame trim. A border character that a colour-
// width miscount pushed left of the row's real last column would have its
// old position back-filled with blank cells by the emulator; NormalizeFrame
// then trims those trailing blanks, shortening the row and making it look
// like a shorter, non-panel line rather than a shorn border -- exactly the
// blind spot a byte-width bug like the review's escape-sequence miscount
// would hide behind. Reading the fixed column directly has no such blind
// spot.
func (d *ScreenDriver) CellAt(x, y int) *uv.Cell {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.screen.CellAt(x, y)
}

// FindText scans the emulator grid, cell by cell, for the first run whose
// concatenated Content equals text, and returns its row and starting
// column. Unlike a plain substring search over Frame's rendered string, this
// reads the same real grid cells CellAt does, so the returned position is
// exactly where a per-cell attribute assertion should look.
func (d *ScreenDriver) FindText(text string) (row, col int, err error) {
	runes := []rune(text)
	if len(runes) == 0 {
		return 0, 0, fmt.Errorf("cannot search for empty text")
	}
	cols, rows := d.GridSize()
	for y := 0; y < rows; y++ {
		for x := 0; x+len(runes) <= cols; x++ {
			match := true
			for i, r := range runes {
				cell := d.CellAt(x+i, y)
				if cell == nil || cell.Content != string(r) {
					match = false
					break
				}
			}
			if match {
				return y, x, nil
			}
		}
	}
	return 0, 0, fmt.Errorf("text %q not found in frame:\n%s", text, d.Frame(false))
}

// drainScreenInput discards synthetic terminal-query responses the emulator
// queues on its own input pipe (for example, answering its own OSC 11
// background-colour query). Nothing needs to consume them, but leaving the
// pipe unread would block the emulator's Write call forever the first time a
// query fires, since io.Pipe writes are synchronous.
func (d *ScreenDriver) drainScreenInput() {
	buf := make([]byte, 256)
	for {
		if _, err := d.screen.Read(buf); err != nil {
			return
		}
	}
}

// drainBudgetInput is drainScreenInput's counterpart for the shadow budget
// emulator, which answers the same synthetic terminal queries on its own
// input pipe and would block d.budget.Write forever without a reader.
func (d *ScreenDriver) drainBudgetInput() {
	buf := make([]byte, 256)
	for {
		if _, err := d.budget.Read(buf); err != nil {
			return
		}
	}
}

// FrameFitsBudget reads the shadow budget emulator -- fed the identical byte
// stream as the real, correctly sized screen emulator, but never resized to
// match it -- and reports an error naming the offending line/extent when the
// client rendered more than rows content lines or a line wider than cols.
// Reading the oversized shadow rather than the real screen matters: the real
// emulator is exactly the size the client's own terminal advertises, so an
// overflowing frame autoscrolls or autowraps within it, and the excess is
// gone by the time anything reads the grid -- exactly the blind spot that
// let requirement 37 through undetected. The shadow has nowhere near its own
// edge to scroll or wrap into, so the overflow survives intact.
func (d *ScreenDriver) FrameFitsBudget(cols, rows int) error {
	d.mu.Lock()
	budget := d.budget
	d.mu.Unlock()
	return frameFitsBudget(budget, cols, rows)
}

func (d *ScreenDriver) read() {
	defer close(d.readDone)
	buf := make([]byte, 4096)
	for {
		n, err := d.terminal.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			d.mu.Lock()
			_, _ = d.raw.Write(chunk)
			_, _ = d.screen.Write(chunk)
			_, _ = d.budget.Write(chunk)
			// Answer each probe once. Looking through the complete accumulated
			// stream would re-answer forever, and the PTY echo would eventually
			// feed CPR replies back as user input.
			answerOSC := !d.oscAnswered && bytes.Contains(chunk, []byte("\x1b]11;?"))
			answerCPR := !d.cprAnswered && bytes.Contains(chunk, []byte("\x1b[6n"))
			d.oscAnswered = d.oscAnswered || answerOSC
			d.cprAnswered = d.cprAnswered || answerCPR
			d.mu.Unlock()

			// Bubble Tea asks these before its first render. The replies use the
			// conventional black background and home cursor position.
			if answerOSC {
				_, _ = d.terminal.Write([]byte("\x1b]11;rgb:0000/0000/0000\x1b\\"))
			}
			if answerCPR {
				_, _ = d.terminal.Write([]byte("\x1b[1;1R"))
			}
			select {
			case d.updated <- struct{}{}:
			default:
			}
		}
		if err != nil {
			return
		}
	}
}

// Send writes raw terminal input, rather than interpreting keys through a test
// framework. This is the same byte stream an interactive user produces.
func (d *ScreenDriver) Send(keys string) error {
	d.mu.Lock()
	d.sent = append(d.sent, sentRecord{at: time.Now(), bytes: keys})
	d.mu.Unlock()
	_, err := d.terminal.Write([]byte(keys))
	return err
}

// sentLog renders every Send call this driver made, in order, with the
// elapsed time since the previous one -- diagnostic-only (task 014), so a
// hang's error can show exactly what input reached (or, per the write's own
// return value, definitely reached the kernel's PTY buffer for) the client
// immediately before it stopped responding.
func (d *ScreenDriver) sentLog() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var b strings.Builder
	var prev time.Time
	for i, r := range d.sent {
		if i == 0 {
			fmt.Fprintf(&b, "  [%s] +0 %q\n", r.at.Format("15:04:05.000"), r.bytes)
		} else {
			fmt.Fprintf(&b, "  [%s] +%s %q\n", r.at.Format("15:04:05.000"), r.at.Sub(prev), r.bytes)
		}
		prev = r.at
	}
	return b.String()
}

// Frame returns cell-grid text, not the terminal's escape stream.
func (d *ScreenDriver) Frame(clockFrozen bool) string {
	d.mu.Lock()
	if d.screen == nil {
		d.mu.Unlock()
		return ""
	}
	frame := d.screen.String()
	d.mu.Unlock()
	return NormalizeFrame(frame, clockFrozen)
}

func (d *ScreenDriver) Raw() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.raw.String()
}

func (d *ScreenDriver) processError() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.exitErr
}

// WaitForFrame polls the emulator grid and includes raw and normalised screen
// diagnostics if deck exits or times out.
func (d *ScreenDriver) WaitForFrame(ctx context.Context, clockFrozen bool, want string) error {
	for {
		if frame := d.Frame(clockFrozen); strings.Contains(frame, want) {
			return nil
		}
		select {
		case <-d.done:
			return fmt.Errorf("deck exited before frame %q: %v\nframe:\n%s\nraw: %q", want, d.processError(), d.Frame(clockFrozen), d.Raw())
		case <-d.updated:
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for frame %q: %w\nframe:\n%s\nraw: %q", want, ctx.Err(), d.Frame(clockFrozen), d.Raw())
		}
	}
}

// Stop terminates a hung client and returns a useful transcript diagnostic.
//
// On timeout it signals SIGQUIT before the final SIGKILL: Go's runtime
// default handler for SIGQUIT prints every goroutine's stack to the
// process's own stderr (which is the same PTY d.raw already captures) and
// then exits, so a genuine hang's diagnostic (task 014: what syscall/state
// the process was blocked in) survives in the returned error instead of
// being thrown away by an unconditional SIGKILL.
func (d *ScreenDriver) Stop(timeout time.Duration) error {
	select {
	case <-d.done:
		if d.terminal != nil {
			_ = d.terminal.Close()
		}
		return d.processError()
	case <-time.After(timeout):
		if d.cmd.Process != nil {
			_ = d.cmd.Process.Signal(syscall.SIGQUIT)
		}
		select {
		case <-d.done:
		case <-time.After(2 * time.Second):
			if d.cmd.Process != nil {
				_ = d.cmd.Process.Kill()
			}
			select {
			case <-d.done:
			case <-time.After(time.Second):
			}
		}
		if d.terminal != nil {
			_ = d.terminal.Close()
		}
		return fmt.Errorf("hung deck client killed after %s\nsent (input timeline):\n%sframe:\n%s\nraw (includes a SIGQUIT goroutine dump if the runtime managed to print one before the follow-up SIGKILL): %q", timeout, d.sentLog(), d.Frame(false), d.Raw())
	}
}

var (
	wallTimestamp = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})\b`)
	relativeTime  = regexp.MustCompile(`\b(?:just now|\d+[mhd] ago)\b`)
)

// NormalizeFrame strips only right-hand cell padding. A frozen clock is part
// of the asserted product contract, so wall timestamps and the TUI's rendered
// relative times are masked only otherwise.
func NormalizeFrame(frame string, clockFrozen bool) string {
	lines := strings.Split(strings.ReplaceAll(frame, "\r\n", "\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t\r")
		if !clockFrozen {
			lines[i] = wallTimestamp.ReplaceAllString(lines[i], "<timestamp>")
			lines[i] = relativeTime.ReplaceAllString(lines[i], "<relative-time>")
		}
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func buildDeckBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "deck")
	command := exec.Command("go", "build", "-o", binary, "github.com/n-orlov/deck/cmd/deck")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build released deck binary: %v\n%s", err, output)
	}
	return binary
}

func TestScreenDriverLaunchesDeckAndAnswersTerminalProbes(t *testing.T) {
	binary := buildDeckBinary(t)
	home := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	driver, err := StartScreenDriver(ctx, binary, []string{
		"DECK_HOME=" + home,
		"DECK_TMUX_SOCKET=deck_pty_driver_test",
		"DECK_ASCII=1", "DECK_ANIM=0", "NO_COLOR=1",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := driver.Stop(time.Second); err != nil && !strings.Contains(err.Error(), "hung deck client") {
			t.Logf("deck exit: %v", err)
		}
	}()

	if err := driver.WaitForFrame(ctx, false, "No sessions"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(driver.Raw(), "\x1b[6n") {
		t.Fatalf("terminal did not observe Bubble Tea CPR probe: %q", driver.Raw())
	}
	if err := driver.Send("q"); err != nil {
		t.Fatalf("send q: %v", err)
	}
	if err := driver.Stop(3 * time.Second); err != nil {
		t.Fatalf("deck did not quit cleanly: %v", err)
	}
}

func TestNormalizeFrame(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		clockFrozen bool
		want        string
	}{
		{
			name:  "unfrozen timestamp and just now",
			input: "at 2026-08-17T19:45:00Z   \ncreated just now   \nplain   \n",
			want:  "at <timestamp>\ncreated <relative-time>\nplain",
		},
		{
			name:  "unfrozen numeric minute age",
			input: "created 12m ago   \n",
			want:  "created <relative-time>",
		},
		{
			name:  "unfrozen numeric hour age",
			input: "created 3h ago   \n",
			want:  "created <relative-time>",
		},
		{
			name:  "unfrozen numeric day age",
			input: "created 27d ago   \n",
			want:  "created <relative-time>",
		},
		{
			name:        "frozen values preserved",
			input:       "at 2026-08-17T19:45:00Z   \ncreated just now\ncreated 12m ago\ncreated 3h ago\ncreated 27d ago   \n",
			clockFrozen: true,
			want:        "at 2026-08-17T19:45:00Z\ncreated just now\ncreated 12m ago\ncreated 3h ago\ncreated 27d ago",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := NormalizeFrame(test.input, test.clockFrozen); got != test.want {
				t.Fatalf("NormalizeFrame() = %q, want %q", got, test.want)
			}
		})
	}
}

var _ io.Writer = vt.NewEmulator(1, 1)
