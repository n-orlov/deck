package tui

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/creack/pty"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestEntryRefusalClearsOnSelectionMove is task 008/R143's (GH #38) own
// SPEC §11.9 clause: "clears when the selection moves". setSelection is
// the ONE seam every key that moves the cursor (↑/↓/PgUp/PgDn/g/G/space,
// a mouse click) assigns the selection through, so a `j` press dispatched
// all the way through Update exercises the real path, not merely the
// helper directly.
func TestEntryRefusalClearsOnSelectionMove(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.sessions = []store.Session{
		{ID: "s1", Name: "alpha", Slug: "alpha", Status: "running"},
		{ID: "s2", Name: "beta", Slug: "beta", Status: "running"},
	}
	m.selected = rowCursor(0)
	m.setEntryRefusal("s1", entryRefusalOther, "boom")

	if _, ok := m.activeEntryRefusalForSelection(); !ok {
		t.Fatalf("test setup: banner is not showing before the move -- fixture is vacuous")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	got := next.(Model)

	if got.selected != rowCursor(1) {
		t.Fatalf("selection after j = %v, want row 1 -- criterion 4's \"keys stay live\" requires j to still move the selection while the banner is up", got.selected)
	}
	if got.entryRefusal.active {
		t.Fatalf("entryRefusal is still active after the selection moved: %+v, want cleared", got.entryRefusal)
	}
	if _, ok := got.activeEntryRefusalForSelection(); ok {
		t.Fatalf("banner is still showing after the selection moved")
	}

	// Moving back onto the originally-refused session must not resurrect
	// the stale banner -- the refusal was cleared OUTRIGHT, not merely
	// hidden by the session-match guard for the row it moved away from.
	next2, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	back := next2.(Model)
	if back.selected != rowCursor(0) {
		t.Fatalf("selection after k = %v, want row 0", back.selected)
	}
	if _, ok := back.activeEntryRefusalForSelection(); ok {
		t.Fatalf("banner resurrected after navigating back onto the originally-refused session")
	}
}

// TestEntryRefusalClearsOnSuccessfulEnter proves the ↵ leg of "clears ...
// when an entry (↵, F, a) succeeds": a stale refusal left on the model
// (as if an earlier attempt on this exact session had been refused) does
// not survive a SUCCESSFUL ↵ entry into that same session.
func TestEntryRefusalClearsOnSuccessfulEnter(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", height, interactiveMinInnerRows)
	}

	socket := selectionTestSocket("refusalclearenter")
	newQuietSelectionPane(t, socket, "deck_refusalclearenter", 80, 24)
	m.tmuxClient = tmux.Client{Socket: socket}
	m.sessions = []store.Session{{ID: "sess-rc-1", Name: "refusalclearenter", Slug: "refusalclearenter", Status: "waiting"}}
	m.selected = rowCursor(0)
	m.setEntryRefusal("sess-rc-1", entryRefusalOther, "a previous attempt was refused")

	next, _ := m.enterInteractive()
	got := next.(Model)
	if !got.interactive {
		t.Fatalf("enterInteractive did not enter interactive mode: %+v", got.entryRefusal)
	}
	if got.entryRefusal.active {
		t.Fatalf("entryRefusal is still active after a successful ↵ entry: %+v, want cleared", got.entryRefusal)
	}
	got.exitInteractive()
}

// TestEntryRefusalClearsOnSuccessfulForce is the same proof for `F`
// (enterInteractiveBody(true)): a successful forced entry clears a stale
// refusal exactly like a successful ↵ does.
func TestEntryRefusalClearsOnSuccessfulForce(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", height, interactiveMinInnerRows)
	}

	socket := selectionTestSocket("refusalclearforce")
	newQuietSelectionPane(t, socket, "deck_refusalclearforce", 80, 24)
	m.tmuxClient = tmux.Client{Socket: socket}
	m.sessions = []store.Session{{ID: "sess-rc-2", Name: "refusalclearforce", Slug: "refusalclearforce", Status: "waiting"}}
	m.selected = rowCursor(0)
	m.setEntryRefusal("sess-rc-2", entryRefusalAttachedElsewhere, "another client is attached to this session")

	next, _ := m.enterInteractiveBody(true)
	got := next.(Model)
	if !got.interactive {
		t.Fatalf("F did not enter interactive mode: %+v", got.entryRefusal)
	}
	if got.entryRefusal.active {
		t.Fatalf("entryRefusal is still active after a successful F entry: %+v, want cleared", got.entryRefusal)
	}
	got.exitInteractive()
}

// TestEntryRefusalClearsOnSuccessfulAttach is the `a` leg: attachSelected
// clears a stale refusal on a successful attach the same way
// enterInteractiveBody does on ↵/F.
func TestEntryRefusalClearsOnSuccessfulAttach(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.attach = func(context.Context, string) (*exec.Cmd, error) { return exec.Command("true"), nil }
	m.sessions = []store.Session{{ID: "sess-rc-3", Name: "refusalclearattach", Slug: "refusalclearattach", Status: "running"}}
	m.selected = rowCursor(0)
	m.setEntryRefusal("sess-rc-3", entryRefusalRowFloor, "preview panel has 5 inner rows, fewer than the 7-row floor")

	next, cmd := m.attachSelected()
	got := next.(Model)
	if got.attachError != "" {
		t.Fatalf("attachSelected refused: %q", got.attachError)
	}
	if cmd == nil {
		t.Fatalf("attachSelected returned a nil cmd on success, want the ExecProcess command")
	}
	if got.entryRefusal.active {
		t.Fatalf("entryRefusal is still active after a successful a attach: %+v, want cleared", got.entryRefusal)
	}
}

// TestEntryRefusalHolderCheckClearsWhenClientDetaches proves the
// "holder left" half of "clears ... when a later tick finds the reason
// gone" for the attached-elsewhere kind: a genuine attached tmux client
// (through a real pty, exactly like force_enter_test.go's own fixture)
// closes, and the NEXT tick's holder probe (entryRefusalHolderCheck,
// wired into previewTick) clears the banner.
func TestEntryRefusalHolderCheckClearsWhenClientDetaches(t *testing.T) {
	socket := selectionTestSocket("refusalholderattached")
	newQuietSelectionPane(t, socket, "deck_refusalholderattached", 80, 24)
	client := tmux.Client{Socket: socket}

	m := New(nil, config.Settings{}, "")
	m.tmuxClient = client
	m.sessions = []store.Session{{ID: "sess-rh-1", Name: "refusalholderattached", Slug: "refusalholderattached", Status: "waiting"}}
	m.selected = rowCursor(0)

	windowTarget, err := tmux.SessionName("refusalholderattached")
	if err != nil {
		t.Fatalf("SessionName: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "tmux", "-L", socket, "attach-session", "-t", windowTarget)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		t.Fatalf("attach %q through pty: %v", windowTarget, err)
	}
	go func() { _, _ = io.CopyBuffer(io.Discard, terminal, make([]byte, 4096)) }()
	waitForSessionAttachedCountForce(t, client, windowTarget, 1)

	m.setEntryRefusal("sess-rh-1", entryRefusalAttachedElsewhere, "another client is attached to this session")

	// While the client is still attached, the probe must NOT clear it.
	if probeCmd := m.entryRefusalHolderCheck(); probeCmd == nil {
		t.Fatalf("entryRefusalHolderCheck returned nil while a contention refusal is active")
	} else {
		next, _ := m.Update(probeCmd())
		got := next.(Model)
		if !got.entryRefusal.active {
			t.Fatalf("entryRefusalHolderCheck cleared the refusal while the client is still attached")
		}
	}

	// Detach the real client, then poll for session_attached to drop, then
	// re-probe: the SAME tick mechanism now finds the reason gone.
	if err := terminal.Close(); err != nil {
		t.Fatalf("close pty: %v", err)
	}
	cancel()
	waitForSessionAttachedCountForce(t, client, windowTarget, 0)

	probeCmd := m.entryRefusalHolderCheck()
	if probeCmd == nil {
		t.Fatalf("entryRefusalHolderCheck returned nil after the client detached")
	}
	next, _ := m.Update(probeCmd())
	got := next.(Model)
	if got.entryRefusal.active {
		t.Fatalf("entryRefusal is still active after the attached client detached: %+v, want cleared", got.entryRefusal)
	}
}

// TestEntryRefusalHolderCheckClearsWhenOwnershipReleased is the same
// proof for the owned-elsewhere kind: this test process claims the
// window's ownership itself (a live claim ProbeWindowOwnership, called
// with no claim of its own to compare against, reads as ClaimForeignLive
// exactly like a genuine other-deck steal would), then releases it --
// the next probe finds the reason gone.
func TestEntryRefusalHolderCheckClearsWhenOwnershipReleased(t *testing.T) {
	socket := selectionTestSocket("refusalholderowned")
	newQuietSelectionPane(t, socket, "deck_refusalholderowned", 80, 24)
	client := tmux.Client{Socket: socket}

	windowTarget, err := tmux.SessionName("refusalholderowned")
	if err != nil {
		t.Fatalf("SessionName: %v", err)
	}
	ownership, acquired, err := client.ClaimWindowOwnership(context.Background(), windowTarget)
	if err != nil || !acquired {
		t.Fatalf("claim ownership for the fixture: acquired=%v err=%v", acquired, err)
	}
	if state, err := client.ProbeWindowOwnership(context.Background(), windowTarget); err != nil || state != tmux.ClaimForeignLive {
		t.Fatalf("test setup: ProbeWindowOwnership (no claim of its own) = %v, %v, want %v, nil -- fixture does not reproduce a foreign live claim", state, err, tmux.ClaimForeignLive)
	}

	m := New(nil, config.Settings{}, "")
	m.tmuxClient = client
	m.sessions = []store.Session{{ID: "sess-rh-2", Name: "refusalholderowned", Slug: "refusalholderowned", Status: "waiting"}}
	m.selected = rowCursor(0)
	m.setEntryRefusal("sess-rh-2", entryRefusalOwnedElsewhere, "a live process holds ownership of this window")

	probeCmd := m.entryRefusalHolderCheck()
	if probeCmd == nil {
		t.Fatalf("entryRefusalHolderCheck returned nil while the ownership claim is still live")
	}
	next, _ := m.Update(probeCmd())
	got := next.(Model)
	if !got.entryRefusal.active {
		t.Fatalf("entryRefusalHolderCheck cleared the refusal while the ownership claim is still live")
	}

	if err := ownership.Release(context.Background()); err != nil {
		t.Fatalf("release ownership: %v", err)
	}

	probeCmd = m.entryRefusalHolderCheck()
	if probeCmd == nil {
		t.Fatalf("entryRefusalHolderCheck returned nil after the ownership claim was released")
	}
	next, _ = m.Update(probeCmd())
	got = next.(Model)
	if got.entryRefusal.active {
		t.Fatalf("entryRefusal is still active after the ownership claim was released: %+v, want cleared", got.entryRefusal)
	}
}

// TestEntryRefusalClearsWhenStoppedSessionStarts proves the "session
// started" half of the same clause, for the entryRefusalStopped kind: no
// tmux probe is involved, only the JUST-refreshed m.sessions status --
// dispatched through the real sessionsLoaded message, exactly as a
// background reload delivers it.
func TestEntryRefusalClearsWhenStoppedSessionStarts(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.sessions = []store.Session{{ID: "sess-started-1", Name: "wasstopped", Slug: "wasstopped", Status: "stopped"}}
	m.baseSessions = m.sessions
	m.selected = rowCursor(0)
	m.setEntryRefusal("sess-started-1", entryRefusalStopped, stoppedSessionRefusalTail)

	next, _ := m.Update(sessionsLoaded{sessions: []store.Session{
		{ID: "sess-started-1", Name: "wasstopped", Slug: "wasstopped", Status: "running"},
	}})
	got := next.(Model)
	if got.entryRefusal.active {
		t.Fatalf("entryRefusal is still active after sessionsLoaded reports the session running: %+v, want cleared", got.entryRefusal)
	}

	// A tick that finds the session STILL stopped must not clear it.
	m2 := New(nil, config.Settings{}, "")
	m2.sessions = []store.Session{{ID: "sess-started-2", Name: "stillstopped", Slug: "stillstopped", Status: "stopped"}}
	m2.baseSessions = m2.sessions
	m2.selected = rowCursor(0)
	m2.setEntryRefusal("sess-started-2", entryRefusalStopped, stoppedSessionRefusalTail)
	next2, _ := m2.Update(sessionsLoaded{sessions: []store.Session{
		{ID: "sess-started-2", Name: "stillstopped", Slug: "stillstopped", Status: "stopped"},
	}})
	got2 := next2.(Model)
	if !got2.entryRefusal.active {
		t.Fatalf("entryRefusal was cleared while the session is still stopped -- reason has not actually gone")
	}
}

// TestEntryRefusalEscClearsBannerBeforeMarks pins task 008's own criterion
// 2, quoting SPEC §11.9 verbatim ("clears ... on Esc -- which dismisses
// the banner before any other layer Esc clears"): with BOTH a non-empty
// mark set and an active refusal banner, the FIRST Esc clears only the
// banner and leaves the marks untouched; the SECOND Esc (banner now gone)
// clears the marks exactly as phase 4c's marks-only Esc (2d282e2) always
// did.
func TestEntryRefusalEscClearsBannerBeforeMarks(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.sessions = []store.Session{
		{ID: "s1", Name: "alpha", Slug: "alpha", Status: "running"},
		{ID: "s2", Name: "beta", Slug: "beta", Status: "running"},
	}
	m.selected = rowCursor(0)
	m.marked = map[string]bool{"s1": true, "s2": true}
	m.setEntryRefusal("s1", entryRefusalOther, "boom")

	esc := tea.KeyMsg{Type: tea.KeyEsc}

	next, _ := m.Update(esc)
	afterFirst := next.(Model)
	if afterFirst.entryRefusal.active {
		t.Fatalf("first Esc did not clear the banner: %+v", afterFirst.entryRefusal)
	}
	if len(afterFirst.marked) != 2 {
		t.Fatalf("first Esc touched the mark set: marked=%v, want it untouched at 2 entries", afterFirst.marked)
	}

	next2, _ := afterFirst.Update(esc)
	afterSecond := next2.(Model)
	if len(afterSecond.marked) != 0 {
		t.Fatalf("second Esc did not clear the mark set: marked=%v, want empty", afterSecond.marked)
	}
}

// TestNonRefusalNotesStillRenderAsFooterLinesWithBannerUp is criterion 3:
// an ordinary non-refusal footer note (a copy failure, a failed kill, a
// failed archive -- every one of them a plain m.attachError string, never
// routed through setEntryRefusal) keeps rendering as a footer line even
// while an entirely unrelated entry-refusal banner is up over the preview
// -- the two mechanisms are independent, and neither swallows the other.
func TestNonRefusalNotesStillRenderAsFooterLinesWithBannerUp(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 240, 24
	m.sessions = []store.Session{{ID: "sess-note-1", Name: "alpha", Slug: "alpha", Status: "running"}}
	m.selected = rowCursor(0)
	m.setEntryRefusal("sess-note-1", entryRefusalOther, "no live pane")
	m.attachError = "Cannot kill: exit status 1"

	if lines := m.attachErrorLines(80); len(lines) == 0 || !strings.Contains(lines[0], "Cannot kill") {
		t.Fatalf("attachErrorLines(80) = %v, want a footer line naming the kill failure", lines)
	}

	cw, ch := m.previewContentSize()
	drawn, _, _ := m.previewBodyLines(cw, ch)
	joined := strings.Join(drawn, "\n")
	if !strings.Contains(joined, "NOT ATTACHED: alpha") {
		t.Fatalf("banner did not draw over the preview while a non-refusal footer note is also set:\n%s", joined)
	}
	if strings.Contains(joined, "Cannot kill") {
		t.Fatalf("the non-refusal footer note leaked into the banner's own preview draw:\n%s", joined)
	}
}
