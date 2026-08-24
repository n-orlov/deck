// prefix_test.go proves PRD phase3b II-40: "forward C-b normally" --
// send-keys (via Dispatcher.SendLiteral, and therefore SendKeyRun, task
// 059/II-39's calling-convention wrapper over it) delivers a literal
// control byte straight into the target program without ever passing
// through tmux's own PREFIX TABLE, the mechanism an attached client's
// raw keystrokes go through that intercepts C-b (tmux's own default
// prefix, confirmed below against an unmodified bare session rather
// than assumed) and swallows it as a command-mode trigger instead of
// delivering it. That is exactly what would make C-b unusable to an
// agent running under an attached tmux client without a double-tap
// (C-b C-b) -- and PRD II-40 requires interactive mode to need no such
// trick, because dispatch never goes anywhere near the attached-client
// input path tmux's prefix table gates.
package interactive

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/tmux"
)

// ctrlB is the raw control byte C-b encodes to: 'b' is 0x62, and a
// Ctrl-modified letter is that letter's byte with bits 0x60 cleared --
// 0x62 &^ 0x60 = 0x02. This is the SAME byte an attached client's own
// terminal driver would generate for a real Ctrl+B keypress; nothing
// about it is deck- or tmux-specific.
const ctrlB = "\x02"

// assertBareSessionPrefixIsCtrlB confirms, against a real freshly
// started bare session with no -f config file (matching every other
// "unmodified defaults" bare-session helper in this repo -- grid_test.go
// and internal/tmux/geometry_test.go's own comments name the same
// pattern), that tmux's own default prefix really is C-b in this
// environment. Without this check, a green result below would prove
// nothing if some ambient config had already moved the prefix key
// elsewhere.
func assertBareSessionPrefixIsCtrlB(t *testing.T, socket string) {
	t.Helper()
	out, err := exec.Command("tmux", "-L", socket, "show-options", "-g", "prefix").Output()
	if err != nil {
		t.Fatalf("show-options -g prefix: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if got != "prefix C-b" {
		t.Fatalf("show-options -g prefix = %q, want %q -- this test's whole premise (C-b is the thing an attached client's raw keystrokes would intercept) depends on the default prefix, not a locally reconfigured one", got, "prefix C-b")
	}
}

// TestCtrlBReachesTheTargetProgramBypassingTmuxPrefixTable is PRD II-40's
// core positive proof. The pane runs `cat -v`, which echoes any control
// byte it receives back as a visible caret-notation sequence ("^B" for
// 0x02) rather than swallowing it invisibly -- so a capture-pane that
// never shows "^B" is unambiguous evidence the byte never reached the
// program at all (e.g. because tmux's prefix table consumed it as a
// command-mode trigger instead, which is exactly the failure this test
// rules out).
func TestCtrlBReachesTheTargetProgramBypassingTmuxPrefixTable(t *testing.T) {
	socket := interactiveSocket("ctrlb")
	cleanup := newBareInteractiveSession(t, socket, "s0", 40, 10)
	defer cleanup()

	assertBareSessionPrefixIsCtrlB(t, socket)

	sendLiteralLine(t, socket, "s0", "cat -v")

	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()
	dispatcher, err := tmux.NewDispatcher(ctx, client, "%0")
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	// SendKeyRun is the exact call a wired-up interactive mode makes for
	// one decoded keystroke/run (task 059/II-39). Sending the SAME byte
	// an attached client's own terminal driver would produce for a real
	// Ctrl+B keypress, through this exact call, is what makes this test
	// prove II-40 rather than merely re-proving SendLiteral works.
	if err := SendKeyRun(ctx, dispatcher, ctrlB); err != nil {
		t.Fatalf("SendKeyRun(ctrlB): %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	var capture string
	for {
		capture = string(rawCapturePane(t, socket, "s0"))
		if strings.Contains(capture, "^B") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("C-b (0x02) sent via SendKeyRun never appeared as \"^B\" in cat -v's echo -- either it never reached the program, or tmux's prefix table intercepted it as a command-mode trigger instead:\n%s", capture)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// prefixSpecialCasingPattern is what a "leader trick" or "double-tap"
// workaround would look like in source: either the literal string
// naming the workaround, or the named-key form of a Ctrl-modified key
// ("C-b" etc. passed to send-keys as a KEY NAME rather than as a literal
// control byte through SendLiteral) -- which key.go's own doc comment
// already states is deliberately out of scope precisely because II-40
// requires the literal-byte path instead. scanForPrefixSpecialCasing is
// the guard body, factored out so the non-vacuous test below can run it
// against a deliberately planted violation without going through
// go/testing's own file-discovery twice.
var prefixSpecialCasingPattern = regexp.MustCompile(`(?i)leader|double-tap|double tap|"C-[a-z]"`)

// scanForPrefixSpecialCasing walks every non-test .go file directly
// inside dir and returns the first line (as "file:lineNumber: text")
// that matches prefixSpecialCasingPattern outside of a "//" comment, or
// "" if none does.
func scanForPrefixSpecialCasing(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", dir, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		file, err := os.Open(path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		lineNumber := 0
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			lineNumber++
			trimmed := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if prefixSpecialCasingPattern.MatchString(trimmed) {
				file.Close()
				return path + ":" + strconv.Itoa(lineNumber) + ": " + trimmed
			}
		}
		if err := scanner.Err(); err != nil {
			file.Close()
			t.Fatalf("scan %s: %v", path, err)
		}
		file.Close()
	}
	return ""
}

// TestNoPrefixSpecialCasingInImplementation is PRD II-40's "no leader
// trick or double-tap in the implementation" half: it greps both
// packages a Ctrl combination's byte could plausibly pass through
// (internal/tmux, where SendLiteral/SendNamedKey live, and
// internal/interactive, this package, where SendKeyRun lives) for any
// sign of a workaround, rather than trusting that none was added.
func TestNoPrefixSpecialCasingInImplementation(t *testing.T) {
	for _, dir := range []string{".", "../tmux"} {
		if hit := scanForPrefixSpecialCasing(t, dir); hit != "" {
			t.Fatalf("prefix/leader special-casing found where none should exist -- PRD II-40 requires C-b (and every other Ctrl combination) to reach the target as a plain literal byte, with no leader trick or double-tap: %s", hit)
		}
	}
}

// TestNoPrefixSpecialCasingInImplementationIsNonVacuous plants a
// throwaway file containing exactly the shape the guard above hunts
// for, confirms scanForPrefixSpecialCasing actually reports it, then
// removes the file -- proving the pattern is not so narrow it would
// silently pass a real violation.
func TestNoPrefixSpecialCasingInImplementationIsNonVacuous(t *testing.T) {
	planted := filepath.Join(".", "zz_planted_leader_trick.go")
	content := "package interactive\n\n// sendLeaderDoubleTap is a planted violation for the guard test.\nfunc sendLeaderDoubleTap() {\n\t_ = \"C-b\"\n}\n"
	if err := os.WriteFile(planted, []byte(content), 0o644); err != nil {
		t.Fatalf("write planted file: %v", err)
	}
	defer os.Remove(planted)

	if hit := scanForPrefixSpecialCasing(t, "."); hit == "" {
		t.Fatalf("scanForPrefixSpecialCasing found nothing against a deliberately planted leader/double-tap-shaped violation -- the guard is vacuous")
	}
}
