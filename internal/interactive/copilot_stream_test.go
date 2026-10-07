package interactive

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/tmux"
)

// copilotStreamPath is the R219a raw capture of copilot 1.0.93's startup
// bytes: the terminal queries it emits before the first frame, then the
// frame itself (R221 items 1-2).
const copilotStreamPath = "../agent/testdata/probes/copilot/stream.raw"

func readCopilotStream(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(copilotStreamPath)
	if err != nil {
		t.Fatalf("read the copilot capture: %v", err)
	}
	return raw
}

// copilotQueries are the startup queries copilot sends, one of each kind, in
// the spelling the capture carries: OSC 10/11/4 colour queries (ST
// terminated), the kitty keyboard query CSI ? u, DECRQM for modes 12 and
// 1007, the colour-scheme query CSI ? 996 n and XTVERSION CSI > q.
func copilotQueries() []string {
	qs := []string{
		"\x1b]10;?\x1b\\", "\x1b]11;?\x1b\\", "\x1b[?u", "\x1b[?996n", "\x1b[>q",
		"\x1b[?12$p", "\x1b[?1007$p",
	}
	for i := 0; i <= 15; i++ {
		qs = append(qs, fmt.Sprintf("\x1b]4;%d;?\x1b\\", i))
	}
	return qs
}

// renderStream writes p into a fresh 80x24 grid (the capture's own size) and
// returns the rendered screen.
func renderStream(p []byte) string {
	s := &Session{}
	g := s.newDrainedGrid(80, 24)
	_, _ = g.Write(p)
	out := g.Render()
	retireGrid(g)
	s.replyDrains.Wait()
	return out
}

// TestCopilotStartupQueriesWriteNoTextIntoTheGrid asserts the capture's
// queries leave the grid exactly as the same stream with the queries cut out
// (R221 item 1), that the frame itself did land (so the comparison is not
// blank against blank), and that each query alone on a blank grid writes
// nothing, however the write is split.
func TestCopilotStartupQueriesWriteNoTextIntoTheGrid(t *testing.T) {
	raw := readCopilotStream(t)
	stripped := raw
	for _, q := range copilotQueries() {
		if !bytes.Contains(raw, []byte(q)) {
			t.Fatalf("the capture no longer carries the query %q", q)
		}
		stripped = bytes.ReplaceAll(stripped, []byte(q), nil)
	}
	got, want := renderStream(raw), renderStream(stripped)
	if got != want {
		t.Fatalf("grid with the queries = %q, want the same as without them %q", got, want)
	}
	if !strings.Contains(ansiEscapeRe.ReplaceAllString(got, ""), "Resume") {
		t.Fatalf("the capture's frame did not reach the grid: %q", got)
	}
	blank := renderAfterChunks()
	for _, q := range copilotQueries() {
		for _, chunks := range splitEverywhere([]byte(q)) {
			if g := renderAfterChunks(chunks...); g != blank {
				t.Errorf("query %q split %q: grid = %q, want blank %q", q, chunks, g, blank)
			}
		}
	}
}

// TestCopilotBELTitlesNeverReachTheGrid asserts OSC 0 titles ending in BEL
// -- copilot's own, one carrying the user's prompt text, and ones with
// non-ASCII characters whose UTF-8 bytes include 0x80-0x9F -- print nothing,
// at every split of the write (R221 item 2).
func TestCopilotBELTitlesNeverReachTheGrid(t *testing.T) {
	blank := renderAfterChunks()
	for _, title := range []string{
		"GitHub Copilot",
		"Copilot: fix the failing test in internal/agent",
		"Copilot: ✳ résumé 日本語 — café 👍",
		"日本語のプロンプト",
		"✻ " + strings.Repeat("long prompt text ", 6) + "✓",
	} {
		data := []byte("\x1b]0;" + title + "\a")
		for _, chunks := range splitEverywhere(data) {
			if g := renderAfterChunks(chunks...); g != blank {
				t.Errorf("title %q split %q: grid = %q, want blank %q", title, chunks, g, blank)
			}
		}
	}
	// In place: a title between two frames leaves the surrounding text alone.
	frame := "\x1b[2;1H❯ fix the bug"
	title := "\x1b]0;✳ fix the bug in café 日本\a"
	if got, want := renderAfterChunks([]byte(frame), []byte(title)), renderAfterChunks([]byte(frame)); got != want {
		t.Errorf("title after the prompt changed the grid: %q, want %q", got, want)
	}
}

// TestCopilotStartupQueriesGetNoReplyInTheirPane drives the capture through a
// real Session on a real pane whose program records its stdin. The bytes
// reach the grid only as the seed, so tmux never sees them and the only
// thing that could write to that stdin is deck forwarding the emulator's
// replies. None may arrive; a marker typed afterwards proves the recorder is
// listening.
func TestCopilotStartupQueriesGetNoReplyInTheirPane(t *testing.T) {
	raw := readCopilotStream(t)
	// The emulator really does answer this stream, so an empty record means
	// deck discarded replies rather than that none were produced.
	if n := emulatorReplyBytes(raw); n == 0 {
		t.Fatalf("the capture produced no emulator reply; the test would pin nothing")
	}
	socket := interactiveSocket("copilot-reply")
	log := filepath.Join(t.TempDir(), "stdin.log")
	cmd := exec.Command("tmux", "-L", socket, "new-session", "-d", "-s", "s0", "-x", "80", "-y", "24",
		"stty raw -echo; exec cat > "+log)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("start the recording pane: %v: %s", err, out)
	}
	defer func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() }()
	if !waitFor(t, 5*time.Second, func() bool { _, err := os.Stat(log); return err == nil }) {
		t.Fatalf("the recorder never opened its log")
	}

	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	session, err := Start(context.Background(), client, "s0", 80, 24, func(context.Context) ([]byte, error) {
		return raw, nil
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer closeSessionWithin(t, session, replyDrainCloseTimeout)
	if !gridContains(session.Grid(), "Resume") {
		t.Fatalf("the seed did not reach the grid: %q", session.Grid().Render())
	}

	const marker = "MARKER"
	if out, err := exec.Command("tmux", "-L", socket, "send-keys", "-t", "s0", "-l", "--", marker).CombinedOutput(); err != nil {
		t.Fatalf("send-keys: %v: %s", err, out)
	}
	read := func() string { b, _ := os.ReadFile(log); return string(b) }
	if !waitFor(t, 5*time.Second, func() bool { return strings.Contains(read(), marker) }) {
		t.Fatalf("the recorder never saw the marker; log = %q", read())
	}
	time.Sleep(200 * time.Millisecond)
	if got := read(); got != marker {
		t.Fatalf("the pane's stdin = %q, want only the marker %q (deck forwarded a reply)", got, marker)
	}
}

// emulatorReplyBytes returns how many reply bytes an undrained emulator
// produces for p, read concurrently so its writes never park.
func emulatorReplyBytes(p []byte) int {
	g := newGrid(80, 24)
	defer retireGrid(g)
	counted := make(chan int, 1)
	go func() {
		buf := make([]byte, replyDrainBufSize)
		total := 0
		for {
			n, err := g.Read(buf)
			total += n
			if err != nil {
				counted <- total
				return
			}
		}
	}()
	_, _ = g.Write(p)
	retireGrid(g)
	return <-counted
}
