package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// previewFitExternalResizeModel is a model whose preview panel (100x30
// terminal) fits a session's window to a box that is neither 80x24 nor
// 100x27, talking to tmux through binary.
func previewFitExternalResizeModel(t *testing.T, socket, binary string, sessions []store.Session) Model {
	t.Helper()
	m := New(nil, config.Settings{PreviewFit: true, Preview: time.Millisecond}, "")
	m.width, m.height = 100, 30
	if w, h := m.previewContentSize(); h < interactiveMinInnerRows || (w == 80 && h == 24) || (w == 100 && h == 27) {
		t.Fatalf("test setup: preview box %dx%d cannot tell a fit from the sizes this test uses", w, h)
	}
	m.tmuxClient = tmux.Client{Socket: socket, Binary: binary}
	m.sessions = sessions
	m.selected = rowCursor(0)
	return m
}

// windowGeometry reads a window's WxH from the real tmux, outside deck.
func windowGeometry(t *testing.T, socket, target string) string {
	t.Helper()
	out, err := exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", target, "#{window_width}x#{window_height}").CombinedOutput()
	if err != nil {
		t.Fatalf("read geometry of %s: %v: %s", target, err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestPreviewFitLeavesAWindowAnotherProcessResizedWhileItRan is SPEC §11's
// "passive fit never undoes a resize another process made" with the resize
// made deterministic: the tmux the fit talks to resizes the window to 100x27
// itself, as another process, the moment the fit asks whether a client is
// attached. That is after the fit read the window's size and before it
// resizes anything, which is the window build 57's activity_resize_inert run 2
// lost (the window held deck's 61x27 fit, want 100x27). The fit must leave the
// window at 100x27 and count as settled for the session.
func TestPreviewFitLeavesAWindowAnotherProcessResizedWhileItRan(t *testing.T) {
	socket := selectionTestSocket("fitexternal")
	const slug = "fitexternal"
	target := "deck_" + slug
	newQuietSelectionPane(t, socket, target, 80, 24)

	dir := t.TempDir()
	marker := filepath.Join(dir, "resized")
	binary := filepath.Join(dir, "tmux")
	script := "#!/bin/sh\ncase \"$*\" in\n*'#{session_attached}'*)\n\tif [ ! -e '" + marker + "' ]; then\n\t\ttouch '" + marker + "'\n\t\ttmux -L \"$2\" resize-window -t '" + target + "' -x 100 -y 27 || exit 1\n\tfi ;;\nesac\nexec tmux \"$@\"\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	m := previewFitExternalResizeModel(t, socket, binary, []store.Session{{ID: "sess-fitexternal", Slug: slug, Name: slug, Status: "running"}})
	updated, cmd := m.Update(previewTick(time.Now()))
	m = updated.(Model)
	cmds := previewTickCmds(t, cmd)
	if len(cmds) != 2 {
		t.Fatalf("previewTick returned %d commands, want 2 (the reschedule plus one passive fit)", len(cmds))
	}
	done, ok := cmds[1]().(previewFitDone)
	if !ok {
		t.Fatal("the fit command did not produce previewFitDone")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("test setup: the other process's resize never ran, so nothing was raced")
	}
	if got := windowGeometry(t, socket, target); got != "100x27" {
		t.Fatalf("window after the fit = %s, want 100x27: the passive fit undid a resize another process made while it ran", got)
	}
	if done.selectionMoved || done.noLivePane || done.foreignLiveClaim || done.clientAttached {
		t.Fatalf("previewFitDone = %+v, want a plain settled fit", done)
	}
	updated, _ = m.Update(done)
	m = updated.(Model)
	if m.previewFitSessionID != "sess-fitexternal" {
		t.Fatalf("previewFitSessionID = %q, want the session settled, so the next tick does not fit over the other process's size", m.previewFitSessionID)
	}
}

// TestPreviewFitForARowNoLongerSelectedResizesNothing: a fit scheduled for
// one row whose selection moves (here a real keypress through Update)
// before the fit reaches tmux issues no resize-window on that row's window,
// and does not count as settled, so the row is fit when it is selected again.
// Update publishes the selection before the frame showing it is drawn, so
// a process that resizes the old row's window after seeing that frame is
// never overwritten by the old fit.
func TestPreviewFitForARowNoLongerSelectedResizesNothing(t *testing.T) {
	socket := selectionTestSocket("fitmoved")
	newQuietSelectionPane(t, socket, "deck_fitmoveda", 80, 24)
	newQuietSelectionPane(t, socket, "deck_fitmovedb", 80, 24)
	binary, wireLog := newTmuxWireLogger(t)

	m := previewFitExternalResizeModel(t, socket, binary, []store.Session{
		{ID: "sess-a", Slug: "fitmoveda", Name: "fitmoveda", Status: "running"},
		{ID: "sess-b", Slug: "fitmovedb", Name: "fitmovedb", Status: "running"},
	})
	updated, cmd := m.Update(previewTick(time.Now()))
	m = updated.(Model)
	cmds := previewTickCmds(t, cmd)
	if len(cmds) != 2 || m.previewFitInFlight != "sess-a" {
		t.Fatalf("previewTick returned %d commands with %q in flight, want a fit for sess-a", len(cmds), m.previewFitInFlight)
	}

	updated, _ = m.Update(key("down"))
	m = updated.(Model)
	if session, ok := m.selectedSession(); !ok || session.ID != "sess-b" {
		t.Fatalf("test setup: after down the selection is %q, want sess-b", session.ID)
	}

	done, ok := cmds[1]().(previewFitDone)
	if !ok {
		t.Fatal("the fit command did not produce previewFitDone")
	}
	if resizes := countWireCommands(t, wireLog, "resize-window"); resizes != 0 {
		t.Fatalf("the fit for a row no longer selected issued %d resize-window commands, want 0", resizes)
	}
	if got := windowGeometry(t, socket, "deck_fitmoveda"); got != "80x24" {
		t.Fatalf("sess-a's window = %s, want it untouched at 80x24", got)
	}
	if !done.selectionMoved || done.sessionID != "sess-a" {
		t.Fatalf("previewFitDone = %+v, want selectionMoved for sess-a", done)
	}
	updated, _ = m.Update(done)
	m = updated.(Model)
	if m.previewFitSessionID == "sess-a" || m.previewFitInFlight != "" {
		t.Fatalf("after the stood-down fit: settled %q, in flight %q; want sess-a not settled and nothing in flight", m.previewFitSessionID, m.previewFitInFlight)
	}

	// The next tick fits the row that is selected now.
	updated, cmd = m.Update(previewTick(time.Now()))
	m = updated.(Model)
	if len(previewTickCmds(t, cmd)) != 2 || m.previewFitInFlight != "sess-b" {
		t.Fatalf("the tick after the stand-down left %q in flight, want a fit for sess-b", m.previewFitInFlight)
	}
}

// TestPreviewFitStandDownKeepsAResizeAnotherProcessMadeBeforeItsUnpin is the
// same SPEC §11 rule for the attached-client stand-down, whose only act is the
// unpin: another process resizes (and so pins) the window after the fit read
// its size and before the unpin goes out (the tmux the fit talks to makes that
// resize itself as it answers the fit's session_attached read with 1). The
// unpin goes out only while the window still is the size the fit read, so the
// other process's 100x27 and its `manual` pin are left alone, and the fit
// still reports the stand-down.
func TestPreviewFitStandDownKeepsAResizeAnotherProcessMadeBeforeItsUnpin(t *testing.T) {
	socket := selectionTestSocket("fitstanddownext")
	const slug = "fitstanddownext"
	target := "deck_" + slug
	newQuietSelectionPane(t, socket, target, 80, 24)

	dir := t.TempDir()
	binary := filepath.Join(dir, "tmux")
	script := "#!/bin/sh\ncase \"$*\" in\n*'#{session_attached}'*)\n\ttmux -L \"$2\" resize-window -t '" + target + "' -x 100 -y 27 || exit 1\n\techo 1\n\texit 0 ;;\nesac\nexec tmux \"$@\"\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	client := tmux.Client{Socket: socket, Binary: binary}
	done, ok := fitPreviewPane(client, slug, "sess-fitstanddownext", 61, 27, nil).(previewFitDone)
	if !ok || !done.clientAttached {
		t.Fatalf("fit = %+v, want the attached-client stand-down", done)
	}
	if got := windowGeometry(t, socket, target); got != "100x27" {
		t.Fatalf("window after the stand-down = %s, want the other process's 100x27", got)
	}
	out, err := exec.Command("tmux", "-L", socket, "show-options", "-wv", "-t", target, "window-size").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "manual" {
		t.Fatalf("window-size after the stand-down = %q (%v), want the other process's manual pin left in place: the unpin overwrote it", out, err)
	}
}
