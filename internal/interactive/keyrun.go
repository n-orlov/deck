package interactive

import (
	"context"

	"github.com/n-orlov/deck/internal/tmux"
)

// SendKeyRun delivers payload -- the complete raw-byte encoding of ONE
// decoded keystroke, or of a run of several that arrived together in a
// single read on the way in (Bubble Tea's own PTY reader coalesces
// multiple keystrokes landing in one read into a single KeyMsg, exactly
// as internal/tui/tui.go's requirement-51 handling documents for the
// list's own dispatch) -- to the target via exactly ONE Dispatcher call.
//
// It is a thin wrapper over Dispatcher.SendLiteral, and the wrapping is
// deliberate: PRD II-39's entire point is a CALLING CONVENTION, not a
// new wire protocol -- a caller (interactive mode's own key forwarding,
// wired up by a later task) must assemble payload's full byte string
// before ever calling this, and must never call it twice for two halves
// of what is logically one event (e.g. an Escape byte now, the rest of
// an Alt combination or a function key's escape sequence a moment
// later). Doing that would split one logical write into two separate
// Dispatcher calls, each its own tmux client subprocess with its own
// real (if small) latency -- and PRD II-39 names Bubble Tea v1.3.10
// specifically because it has no escape timeout of its own:
// keyrun_test.go proves directly against Bubble Tea's public input path
// that ANY gap between the two halves, even far below what a human
// could produce, decodes wrong at the far end -- an Alt+rune keypress
// splits into a bare Escape key followed by an unmodified rune, and a
// function key's own escape sequence, split right after its leading ESC
// byte, splits into a bare Escape key followed by the remainder typed as
// literal text. Handing the WHOLE payload to a single SendLiteral call,
// as this function does, is what keyrun_test.go shows closes both.
//
// This lives in package interactive, not internal/tmux, deliberately:
// keyrun_test.go must import charmbracelet/bubbletea to reproduce PRD
// II-39's hazard against Bubble Tea's own decoder, and bubbletea's
// package init (tea_init.go) unconditionally probes the process's own
// os.Stdin/os.Stdout for the terminal's background colour the moment
// the package is linked into a binary -- confirmed the hard way: adding
// that import to internal/tmux's own test binary made
// TestAttachThroughPTY fail nearly every run, because that test re-execs
// os.Args[0] (the SAME compiled test binary) inside a fresh real pty,
// and the probe's own OSC 11 / cursor-position query bytes land in that
// pty ahead of the real tmux attach output the test is waiting to see.
// internal/interactive has no such re-exec test, so it is a safe home.
func SendKeyRun(ctx context.Context, d *tmux.Dispatcher, payload string) error {
	if payload == "" {
		return nil
	}
	return d.SendLiteral(ctx, payload)
}
