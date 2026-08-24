// key_test.go proves PRD phase3c items 34 and 35 (II-34/II-35):
// Dispatcher.SendNamedKey sends a fixed, allowlisted key NAME to tmux
// (never a hand-built escape sequence), and refuses an unlisted name
// before tmux is ever invoked at all.
//
// The raw hazard PRD item 35 names is already demonstrated, with no
// deck code involved, by literal_send_test.go's
// TestSendKeysUnknownKeyNameIsDeliveredAsLiteralTextWithExitZero
// (`send-keys Frobnicate` types ten literal bytes into the pane, exit
// 0). This file's job is the other half: showing SendNamedKey refuses
// that exact input instead, and that the names it DOES accept are
// delivered by tmux's own key-string translation, not by any escape
// sequence deck constructed itself.
package tmux

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

func namedKeySocket(name string) string {
	return dispatchSocket("named-key-" + name)
}

// newBareCatSession starts a single-window session on socket, at
// 80x10, running `cat` as its only command instead of a login shell.
// Every other test in this package that wants to observe raw bytes
// landing in a pane relies on a shell's own readline echo (which
// interprets some escapes rather than leaving them as text); `cat`
// never interprets anything it reads on stdin -- it writes every byte
// straight back out, unedited -- so it is the only way to see the
// EXACT bytes tmux's key-string translation actually produced, byte
// for byte, in capture-pane's own output.
func newBareCatSession(t *testing.T, socket, session string) (cleanup func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	args := []string{"-L", socket, "new-session", "-d", "-s", session, "-x", "80", "-y", "10", "cat"}
	if output, err := exec.CommandContext(ctx, "tmux", args...).CombinedOutput(); err != nil {
		t.Fatalf("start bare cat session: %v: %s", err, output)
	}
	return func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		_ = exec.CommandContext(closeCtx, "tmux", "-L", socket, "kill-server").Run()
	}
}

// TestSendNamedKeyRejectsAnUnknownNameWithoutSpawningTmux is PRD item
// 35 read literally: "validate against an allowlist BEFORE spawning".
// Frobnicate is the exact payload literal_send_test.go's raw hazard
// test names -- ten bytes, delivered as literal text, exit 0, if it
// ever reached tmux. Verifications() not moving at all (not even by
// one) is the proof that this refusal happens before ANY tmux command
// runs, including the identity re-resolution Dispatcher.Send would
// otherwise perform first.
func TestSendNamedKeyRejectsAnUnknownNameWithoutSpawningTmux(t *testing.T) {
	socket := namedKeySocket("reject")
	cleanup := newBareCatSession(t, socket, "s0")
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	dispatcher, err := NewDispatcher(ctx, client, "%0")
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	before := dispatcher.Verifications()

	if err := dispatcher.SendNamedKey(ctx, "Frobnicate"); err == nil {
		t.Fatalf("SendNamedKey(Frobnicate): got nil error, want a refusal (PRD II-35)")
	}

	if after := dispatcher.Verifications(); after != before {
		t.Fatalf("Verifications() went from %d to %d for a rejected name, want unchanged -- the allowlist check must happen before tmux is ever invoked, not just before the payload is delivered", before, after)
	}

	capture := runTmux(t, socket, "capture-pane", "-p", "-t", "s0")
	if strings.Contains(capture, "Frobnicate") {
		t.Fatalf("pane capture = %q, contains \"Frobnicate\" -- SendNamedKey must not have delivered anything", capture)
	}
}

// TestSendNamedKeyRejectsEveryNameNotOnTheAllowlist widens the single
// Frobnicate case into a small table of other plausible-looking but
// unlisted names (task 054/055's own SendLiteral flags, a lowercase
// variant, and a made-up mouse-report name confirmed during this
// task's own tmux survey to not be a key-string translation target at
// all), so the allowlist is proven to be an allowlist and not a
// hand-picked denylist of one string.
func TestSendNamedKeyRejectsEveryNameNotOnTheAllowlist(t *testing.T) {
	for _, name := range []string{"Frobnicate", "home", "WheelUpPane", "KPDivide", "C-a", ""} {
		if IsNamedKeyAllowed(name) {
			t.Fatalf("IsNamedKeyAllowed(%q) = true, want false", name)
		}
	}
}

// TestSendNamedKeyDeliversHomeAndEndByTmuxsOwnTranslationNotHandEncoded
// is PRD item 34's positive half: sending the NAMES "Home" and "End"
// through SendNamedKey, into a pane running `cat`, delivers exactly the
// vt220 escape sequences tmux itself produces for those names --
// ESC[1~ and ESC[4~ -- confirmed directly against a real tmux 3.5a
// server while building namedKeyAllowlist (see key.go's own comment).
// Deck's code never constructs either byte sequence itself (key_grep_test.go
// proves that by grep); it only ever hands tmux the name.
func TestSendNamedKeyDeliversHomeAndEndByTmuxsOwnTranslationNotHandEncoded(t *testing.T) {
	// capture-pane -p renders a raw ESC control byte stored in the
	// screen grid as the two-character caret notation "^[", never the
	// literal 0x1b byte (confirmed by hexdumping capture-pane's own
	// output while building this test: 5e 5b 5b 31 7e -- "^", "[",
	// "[", "1", "~"). want below is written in that rendered form, not
	// as a Go escape literal, precisely so this test is comparing what
	// capture-pane actually returns, not what we assume it should.
	cases := []struct {
		name string
		want string
	}{
		{"Home", "^[[1~"},
		{"End", "^[[4~"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			socket := namedKeySocket("deliver-" + tc.name)
			cleanup := newBareCatSession(t, socket, "s0")
			defer cleanup()
			client := Client{Socket: socket, Timeout: 5 * time.Second}
			ctx := context.Background()

			dispatcher, err := NewDispatcher(ctx, client, "%0")
			if err != nil {
				t.Fatalf("NewDispatcher: %v", err)
			}
			if err := dispatcher.SendNamedKey(ctx, tc.name); err != nil {
				t.Fatalf("SendNamedKey(%s): %v", tc.name, err)
			}

			deadline := time.Now().Add(2 * time.Second)
			var capture string
			for time.Now().Before(deadline) {
				capture = runTmux(t, socket, "capture-pane", "-p", "-t", "s0")
				if strings.Contains(capture, tc.want) {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if !strings.Contains(capture, tc.want) {
				t.Fatalf("SendNamedKey(%s): pane capture = %q, want it to contain %q", tc.name, capture, tc.want)
			}
		})
	}
}

// TestSendNamedKeyDeliversModifiedNavigationKeysByTmuxsOwnTranslation is
// steer 017 item 1's real-tmux half: every modified-navigation name added
// to namedKeyAllowlist (Ctrl/Shift/Ctrl+Shift + arrows/Home/End/page-keys)
// is confirmed here against a real tmux 3.5a server, into a pane running
// `cat`, the same discipline TestSendNamedKeyDeliversHomeAndEndByTmuxsOwnTranslationNotHandEncoded
// already uses -- the want strings are exactly what this task's own
// tmux-3.5a survey captured (see key.go's allowlist comment) and exactly
// what charmbracelet/bubbletea@v1.3.10/key.go's `sequences` table decodes
// back into KeyCtrlLeft/KeyShiftHome/KeyCtrlShiftEnd/etc, so tmux's
// translation and bubbletea's decoding are proven to agree, not merely
// assumed to.
func TestSendNamedKeyDeliversModifiedNavigationKeysByTmuxsOwnTranslation(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"C-Up", "^[[1;5A"},
		{"C-Down", "^[[1;5B"},
		{"C-Right", "^[[1;5C"},
		{"C-Left", "^[[1;5D"},
		{"C-Home", "^[[1;5H"},
		{"C-End", "^[[1;5F"},
		{"C-PgUp", "^[[5;5~"},
		{"C-PgDn", "^[[6;5~"},
		{"S-Up", "^[[1;2A"},
		{"S-Down", "^[[1;2B"},
		{"S-Right", "^[[1;2C"},
		{"S-Left", "^[[1;2D"},
		{"S-Home", "^[[1;2H"},
		{"S-End", "^[[1;2F"},
		{"C-S-Up", "^[[1;6A"},
		{"C-S-Down", "^[[1;6B"},
		{"C-S-Right", "^[[1;6C"},
		{"C-S-Left", "^[[1;6D"},
		{"C-S-Home", "^[[1;6H"},
		{"C-S-End", "^[[1;6F"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			socket := namedKeySocket("modnav-" + tc.name)
			cleanup := newBareCatSession(t, socket, "s0")
			defer cleanup()
			client := Client{Socket: socket, Timeout: 5 * time.Second}
			ctx := context.Background()

			dispatcher, err := NewDispatcher(ctx, client, "%0")
			if err != nil {
				t.Fatalf("NewDispatcher: %v", err)
			}
			if err := dispatcher.SendNamedKey(ctx, tc.name); err != nil {
				t.Fatalf("SendNamedKey(%s): %v", tc.name, err)
			}

			deadline := time.Now().Add(2 * time.Second)
			var capture string
			for time.Now().Before(deadline) {
				capture = runTmux(t, socket, "capture-pane", "-p", "-t", "s0")
				if strings.Contains(capture, tc.want) {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if !strings.Contains(capture, tc.want) {
				t.Fatalf("SendNamedKey(%s): pane capture = %q, want it to contain %q", tc.name, capture, tc.want)
			}
		})
	}
}

// TestHomeAndEndEscapesMatchARealAttachedClientTypingTheSameKeys is the
// finding PRD item 34 asks this task to record: tmux emits the SAME
// vt220 escape (ESC[1~ for Home, ESC[4~ for End) whether the bytes
// arrive via `send-keys <Name>` or via a real attached client typing
// the physical key. A real terminal emulator translates the physical
// Home/End key into exactly those escape bytes before ever handing them
// to tmux (that translation happens client-side, before the bytes
// reach the wire at all) -- so writing those same bytes directly into
// an attached client's own pty, bypassing only the physical keyboard
// and terminal-emulator step, is a faithful stand-in for "a user
// pressed Home while attached": tmux itself does no retranslation of
// raw client input that does not match one of its own key bindings, it
// simply forwards what the client sent straight to the active pane.
// This is what makes SendNamedKey's translation "faithful to attach"
// rather than merely "consistent with itself".
func TestHomeAndEndEscapesMatchARealAttachedClientTypingTheSameKeys(t *testing.T) {
	// sent is the raw bytes written into the attached client's own pty
	// (what a real terminal emulator would send for the physical key);
	// wantRendered is what capture-pane -p reports back for it, in its
	// own caret-notation rendering of the stored ESC byte -- see the
	// comment on TestSendNamedKeyDeliversHomeAndEndByTmuxsOwnTranslationNotHandEncoded
	// above for how that rendering was confirmed.
	cases := []struct {
		name         string
		sent         string
		wantRendered string
	}{
		{"Home", "\x1b[1~", "^[[1~"},
		{"End", "\x1b[4~", "^[[4~"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			socket := namedKeySocket("attach-" + tc.name)
			cleanup := newBareCatSession(t, socket, "s1")
			defer cleanup()

			cmd := exec.Command("tmux", "-L", socket, "attach-session", "-t", "s1")
			cmd.Env = append(os.Environ(), "TERM=xterm-256color")
			terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 10, Cols: 80})
			if err != nil {
				t.Fatalf("pty.StartWithSize: %v", err)
			}
			t.Cleanup(func() { _ = terminal.Close() })
			var drain strings.Builder
			go func() { _, _ = io.Copy(&drain, terminal) }()

			time.Sleep(300 * time.Millisecond)
			// The raw bytes a real terminal emulator sends for the
			// physical Home/End key -- written directly into the
			// attached client's own pty, standing in for the keyboard
			// + terminal-emulator step a physical keypress would go
			// through before the bytes ever reach tmux.
			if _, err := terminal.Write([]byte(tc.sent)); err != nil {
				t.Fatalf("write %s bytes into attached client: %v", tc.name, err)
			}

			deadline := time.Now().Add(3 * time.Second)
			var capture string
			for time.Now().Before(deadline) {
				capture = runTmux(t, socket, "capture-pane", "-p", "-t", "s1")
				if strings.Contains(capture, tc.wantRendered) {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if _, err := terminal.Write([]byte("\x02d")); err != nil { // tmux prefix + detach
				t.Fatal(err)
			}
			_ = cmd.Wait()

			if !strings.Contains(capture, tc.wantRendered) {
				t.Fatalf("attached-client %s: pane capture = %q, want it to contain %q -- the same bytes SendNamedKey(%s) delivers", tc.name, capture, tc.wantRendered, tc.name)
			}
		})
	}
}
