// keyrun_test.go proves PRD phase3b II-39 directly against Bubble Tea
// v1.3.10's own public input-decoding path: splitting one logical
// keystroke's bytes across two separate writes, even with a real gap
// far below anything a human could produce, decodes wrong at the far
// end, and combining them into one write closes it. See keyrun.go's own
// doc comment for why this lives in package interactive rather than
// internal/tmux.
package interactive

import (
	"errors"
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/tmux"
)

// keyRunRecorder is the smallest possible Bubble Tea model: it records
// every tea.KeyMsg it receives, in arrival order, and asks the program
// to quit once it has seen want of them. It renders nothing (the
// program is started with tea.WithoutRenderer) and reacts to nothing
// else -- its only job is to be a faithful stand-in for "whatever real
// program Bubble Tea v1.3.10 decodes keystrokes for", which is exactly
// what PRD II-39 names as the thing with no escape timeout.
type keyRunRecorder struct {
	want int
	got  *[]tea.KeyMsg
}

func (m keyRunRecorder) Init() tea.Cmd { return nil }

func (m keyRunRecorder) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		*m.got = append(*m.got, k)
		if len(*m.got) >= m.want {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m keyRunRecorder) View() string { return "" }

// captureKeyMsgs starts a real Bubble Tea program (tea.WithInput over a
// pipe this function itself controls, tea.WithoutRenderer and
// tea.WithOutput(io.Discard) so nothing touches this test process's own
// terminal) and feeds it writes in order, sleeping waitBetween before
// each write after the first. It returns every tea.KeyMsg the program's
// own decoder produced before it saw want of them or before a 2s safety
// timeout fired, whichever came first -- this function never talks to
// tmux at all, because PRD II-39's hazard is Bubble Tea's OWN decoding,
// one hop past wherever this package's (or internal/tmux's)
// send-keys/load-buffer dispatch eventually delivers bytes, and
// reproducing it needs nothing more than Bubble Tea's public input
// path.
func captureKeyMsgs(t *testing.T, want int, waitBetween time.Duration, writes ...string) []tea.KeyMsg {
	t.Helper()

	pr, pw := io.Pipe()
	var got []tea.KeyMsg
	model := keyRunRecorder{want: want, got: &got}
	p := tea.NewProgram(model,
		tea.WithInput(pr),
		tea.WithOutput(io.Discard),
		tea.WithoutRenderer(),
		tea.WithoutSignalHandler(),
		tea.WithoutCatchPanics(),
	)

	done := make(chan error, 1)
	go func() {
		_, err := p.Run()
		done <- err
	}()

	go func() {
		for i, w := range writes {
			if i > 0 && waitBetween > 0 {
				time.Sleep(waitBetween)
			}
			if _, err := pw.Write([]byte(w)); err != nil {
				return
			}
		}
	}()

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatalf("program exited with error: %v", err)
		}
	case <-time.After(2 * time.Second):
		p.Quit()
		<-done
	}
	_ = pw.Close()
	_ = pr.Close()
	return got
}

// TestSplitAltRuneWithOneMillisecondGapDecodesAsEscapeThenPlainRune is
// PRD II-39's first named red control: alt+a is ESC (0x1b) immediately
// followed by 'a'. Delivered as two SEPARATE writes -- exactly what two
// separate Dispatcher calls (e.g. one for "Escape", a second for the
// literal rune) would look like on the wire, one tmux client subprocess
// apart -- with a real 1ms gap between them, Bubble Tea v1.3.10 decodes
// the ESC byte as a bare Escape keypress on its own (key.go's own
// comment: a short read is treated as a complete event boundary, so it
// never waits for more), then decodes 'a' as its own unmodified,
// non-Alt rune keypress a moment later. This is the row PRD II-39 names
// ("a 1 ms split turns alt+a into esc then a"); it is RED (i.e. true)
// with no coalescing in sight -- nothing under test here is deck's own
// code, deliberately, since the hazard is Bubble Tea's decoding, not
// this package's.
func TestSplitAltRuneWithOneMillisecondGapDecodesAsEscapeThenPlainRune(t *testing.T) {
	msgs := captureKeyMsgs(t, 2, time.Millisecond, "\x1b", "a")
	if len(msgs) != 2 {
		t.Fatalf("want 2 decoded messages (esc, then plain rune), got %d: %#v", len(msgs), msgs)
	}
	if msgs[0].Type != tea.KeyEsc {
		t.Fatalf("first message: want KeyEsc, got %#v (%s)", msgs[0], msgs[0])
	}
	if msgs[1].Type != tea.KeyRunes || msgs[1].Alt || string(msgs[1].Runes) != "a" {
		t.Fatalf("second message: want a bare, non-Alt rune 'a', got %#v (%s)", msgs[1], msgs[1])
	}
}

// TestCombinedAltRuneWriteDecodesAsOneAltKeypress is the corresponding
// green control: the SAME two bytes, handed to Bubble Tea in ONE write
// (as SendKeyRun's own contract requires: build the full payload first,
// call once), decode as a SINGLE Alt+rune keypress -- never a bare
// Escape, never a plain 'a'. This is what makes
// TestSplitAltRuneWithOneMillisecondGapDecodesAsEscapeThenPlainRune's
// hazard closeable at all: the fix is entirely about not splitting the
// write, not about teaching Bubble Tea anything new.
func TestCombinedAltRuneWriteDecodesAsOneAltKeypress(t *testing.T) {
	msgs := captureKeyMsgs(t, 1, 0, "\x1ba")
	if len(msgs) != 1 {
		t.Fatalf("want exactly 1 decoded message, got %d: %#v", len(msgs), msgs)
	}
	if msgs[0].Type != tea.KeyRunes || !msgs[0].Alt || string(msgs[0].Runes) != "a" {
		t.Fatalf("want a single Alt+'a' keypress, got %#v (%s)", msgs[0], msgs[0])
	}
}

// TestSplitFunctionKeyAfterLeadingEscapeInjectsItsTailAsLiteralText is
// PRD II-39's second named red control. F12's real escape sequence is
// "\x1b[24~" (confirmed against Bubble Tea's own key.go: the entry
// mapping "\x1b[24~" to KeyF12). Delivered as two separate writes split
// right after the leading ESC byte, with a real gap between them, Bubble
// Tea decodes the lone ESC as a bare Escape keypress (same short-read
// reasoning as the Alt+rune case above), and then decodes the remaining
// "[24~" -- four ordinary printable ASCII bytes with no leading ESC left
// to introduce them -- as literal typed text (Bubble Tea's own read
// coalesces same-read bytes into one KeyMsg, so all four runes arrive
// together as a single Runes="[24~" message). PRD II-39's own prose
// illustrates this with "[12~"; this test uses F12's real, verified
// sequence instead of an illustrative digit transposition, and the
// mechanism -- ESC decoded alone, its escape sequence's own tail typed
// in as text -- is identical either way.
func TestSplitFunctionKeyAfterLeadingEscapeInjectsItsTailAsLiteralText(t *testing.T) {
	msgs := captureKeyMsgs(t, 2, time.Millisecond, "\x1b", "[24~")
	if len(msgs) != 2 {
		t.Fatalf("want 2 decoded messages (esc, then the sequence's tail typed as text), got %d: %#v", len(msgs), msgs)
	}
	if msgs[0].Type != tea.KeyEsc {
		t.Fatalf("first message: want KeyEsc, got %#v (%s)", msgs[0], msgs[0])
	}
	if msgs[1].Type != tea.KeyRunes || msgs[1].Alt || string(msgs[1].Runes) != "[24~" {
		t.Fatalf("second message: want the literal text \"[24~\" typed in with no Alt modifier, got %#v (%s)", msgs[1], msgs[1])
	}
}

// TestCombinedFunctionKeyWriteDecodesAsOneNamedKeypress is the green
// control: F12's whole escape sequence, handed to Bubble Tea in ONE
// write, decodes as a single, recognized KeyF12 keypress -- never a bare
// Escape, never any part of it typed as literal text.
func TestCombinedFunctionKeyWriteDecodesAsOneNamedKeypress(t *testing.T) {
	msgs := captureKeyMsgs(t, 1, 0, "\x1b[24~")
	if len(msgs) != 1 {
		t.Fatalf("want exactly 1 decoded message, got %d: %#v", len(msgs), msgs)
	}
	if msgs[0].Type != tea.KeyF12 {
		t.Fatalf("want a single KeyF12 keypress, got %#v (%s)", msgs[0], msgs[0])
	}
}

// TestSendKeyRunRejectsAnEmptyPayloadWithoutTouchingDispatch is a small
// direct unit test of SendKeyRun's own contract (an empty run is a
// deliberate no-op, never an empty send-keys invocation) -- it does not
// need a live tmux target at all, since SendKeyRun returns before ever
// reaching d.SendLiteral (and therefore before touching the zero-value
// Dispatcher's unexported fields) when payload is "".
func TestSendKeyRunRejectsAnEmptyPayloadWithoutTouchingDispatch(t *testing.T) {
	d := &tmux.Dispatcher{}
	if err := SendKeyRun(t.Context(), d, ""); err != nil {
		t.Fatalf("SendKeyRun(\"\") should be a silent no-op, got error: %v", err)
	}
}
