package interactive

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/tmux"
)

// R221 item 5: a resize while the copilot fixture is working reflows the grid
// to the new size with no cell left over from the old one. The fixture is the
// real cmd/fake-copilot binary in a real tmux pane; tmux's own resize-window
// raises SIGWINCH in it and it redraws at the size the kernel reports.

const copilotResizeSessionID = "5113b778-c479-4319-8cd6-f8c7b4604ca5"

// startWorkingCopilot builds the fake, runs it rendering its screen in a
// width x height tmux pane, and drives it into the Working state. It returns
// the tmux socket and the deck home the fake logs its sizes under.
func startWorkingCopilot(t *testing.T, width, height int) (socket, deckHome string) {
	t.Helper()
	dir := t.TempDir()
	binary := filepath.Join(dir, "fake-copilot")
	build := exec.Command("go", "build", "-o", binary, "./cmd/fake-copilot") //nolint:gosec // G204: fixed arguments
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fake-copilot: %v\n%s", err, out)
	}
	deckHome = filepath.Join(dir, "deck")
	copilotHome := filepath.Join(dir, "copilot")
	for _, d := range []string{deckHome, copilotHome} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	socket = interactiveSocket("copilot-resize")
	args := []string{
		"-L", socket, "new-session", "-d", "-s", "s0", "-c", dir,
		"-x", strconv.Itoa(width), "-y", strconv.Itoa(height),
		"env", "FAKE_COPILOT_COMMANDS=1", "FAKE_COPILOT_SCREEN=1",
		"DECK_HOME=" + deckHome, "COPILOT_HOME=" + copilotHome,
		binary, "--session-id", copilotResizeSessionID,
	}
	if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil { //nolint:gosec // G204: test-owned arguments
		t.Fatalf("start the fake copilot pane: %v: %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })

	waitPane(t, socket, "the first frame", func(p string) bool { return strings.Contains(p, "/ commands") })
	sendLiteralLine(t, socket, "s0", `{"command":"prompt","text":"resize me","hold":true}`)
	waitPane(t, socket, "the Working footer", func(p string) bool { return strings.Contains(p, "◉ Working") })
	return socket, deckHome
}

func capturePaneText(t *testing.T, socket string) string {
	t.Helper()
	return string(rawCapturePane(t, socket, "s0"))
}

func waitPane(t *testing.T, socket, what string, cond func(pane string) bool) {
	t.Helper()
	if !waitFor(t, 10*time.Second, func() bool { return cond(capturePaneText(t, socket)) }) {
		t.Fatalf("timed out waiting for %s; the pane shows:\n%s", what, capturePaneText(t, socket))
	}
}

// paneSeed is the reseed a resize takes: the pane's visible screen at its
// current size, one row per line.
func paneSeed(t *testing.T, socket string) func(context.Context) ([]byte, error) {
	return func(context.Context) ([]byte, error) {
		rows := strings.Split(strings.TrimRight(capturePaneText(t, socket), "\n"), "\n")
		return []byte("\x1b[2J\x1b[H" + strings.Join(rows, "\r\n")), nil
	}
}

// gridMatchesPane reports whether the grid's rows are the pane's rows, cell
// for cell, once trailing blanks are dropped from both.
func gridMatchesPane(g *Grid, pane string) bool {
	got := screenRows(g)
	want := strings.Split(strings.TrimRight(pane, "\n"), "\n")
	for i := range got {
		w := ""
		if i < len(want) {
			w = strings.TrimRight(want[i], " ")
		}
		if got[i] != w {
			return false
		}
	}
	return true
}

func TestCopilotResizeWhileWorkingReflowsTheGridWithNoStaleCells(t *testing.T) {
	const w0, h0, w1, h1 = 100, 30, 60, 20
	socket, deckHome := startWorkingCopilot(t, w0, h0)
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	session, err := Start(ctx, client, "s0", w0, h0, paneSeed(t, socket))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer session.Close()
	oldRule := strings.Repeat("─", w0)
	if !waitFor(t, 5*time.Second, func() bool { return gridContains(session.Grid(), oldRule) }) {
		t.Fatalf("the grid never showed the %d-column frame:\n%s", w0, joinedRows(session.Grid()))
	}

	// The window shrinks; tmux resizes the pane and signals the fake, which
	// redraws at the new size while it is still working.
	if out, err := exec.Command("tmux", "-L", socket, "resize-window", "-t", "s0", "-x", strconv.Itoa(w1), "-y", strconv.Itoa(h1)).CombinedOutput(); err != nil {
		t.Fatalf("resize-window: %v: %s", err, out)
	}
	newRule := strings.Repeat("─", w1)
	waitPane(t, socket, "the redraw at the new size", func(p string) bool {
		return strings.Contains(p, newRule) && !strings.Contains(p, newRule+"─") && strings.Contains(p, "◉ Working")
	})
	sizes, _ := os.ReadFile(filepath.Join(deckHome, "log", "fake-copilot-sizes.log"))
	if !strings.HasSuffix(string(sizes), strconv.Itoa(w1)+"x"+strconv.Itoa(h1)+"\n") {
		t.Fatalf("the fake never observed the resize: sizes log %q", sizes)
	}

	if err := session.Resize(ctx, w1, h1, paneSeed(t, socket)); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	g := session.Grid()
	if g.Width() != w1 || g.Height() != h1 {
		t.Fatalf("grid is %dx%d after the resize, want %dx%d", g.Width(), g.Height(), w1, h1)
	}
	if !gridMatchesPane(g, capturePaneText(t, socket)) {
		t.Fatalf("the reflowed grid differs from the pane:\ngrid:\n%s\npane:\n%s", joinedRows(g), capturePaneText(t, socket))
	}
	rows := joinedRows(g)
	if strings.Contains(rows, newRule+"─") {
		t.Errorf("a cell of the old %d-column rule survived the reflow:\n%s", w0, rows)
	}
	if !strings.Contains(rows, "◉ Working") {
		t.Errorf("the working footer is missing after the reflow:\n%s", rows)
	}

	// Live output keeps landing in the reflowed grid: ending the turn turns
	// the footer idle, and nothing of the working frame is left behind.
	sendLiteralLine(t, socket, "s0", `{"command":"stop"}`)
	waitPane(t, socket, "the idle footer", func(p string) bool { return strings.Contains(p, "/ commands") })
	if !waitFor(t, 5*time.Second, func() bool { return gridMatchesPane(session.Grid(), capturePaneText(t, socket)) }) {
		t.Fatalf("the live grid never caught up with the pane:\ngrid:\n%s\npane:\n%s", joinedRows(session.Grid()), capturePaneText(t, socket))
	}
	if strings.Contains(joinedRows(session.Grid()), "◉ Working") {
		t.Errorf("the working footer is stale after the turn ended:\n%s", joinedRows(session.Grid()))
	}
}
