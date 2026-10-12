package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// bookmarkRig is an attached (interactive) model over two real tmux panes,
// each running a raw `cat` that appends every byte it receives to its own
// file, so a test reads exactly which bytes reached which pane.
type bookmarkRig struct {
	m      Model
	db     *store.Store
	a, b   store.Session
	fileOf map[string]string
}

// newBookmarkRig attaches to session A (list index 0) with session B at
// index 1, the bookmarks setting as given.
func newBookmarkRig(t *testing.T, on bool) *bookmarkRig {
	t.Helper()
	// Not t.TempDir(): its path carries the subtest name, whose shell
	// metacharacters (alt+$, alt+&, alt+() would reach the pane's script.
	home, err := os.MkdirTemp("", "bmattach")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	socket := selectionTestSocket("bmattach")
	rig := &bookmarkRig{db: db, fileOf: map[string]string{}}
	var sessions []store.Session
	for i, name := range []string{"bkalpha", "bkbravo"} {
		s, err := db.CreateSession(context.Background(), store.CreateSessionInput{
			ID: "id-" + name, Name: name, CWD: t.TempDir(), Agent: "claude",
			CapturedPath: "/bin/true", Status: "running", StatusAt: 1, CreatedAt: int64(i + 1),
		})
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		sessions = append(sessions, s)
		rig.fileOf[s.ID] = filepath.Join(home, name+".rx")
		ready := filepath.Join(home, name+".ready")
		script := fmt.Sprintf("stty raw -echo; : > %s; exec cat >> %s", ready, rig.fileOf[s.ID])
		target, err := tmux.SessionName(s.Slug)
		if err != nil {
			t.Fatalf("SessionName: %v", err)
		}
		args := []string{"-L", socket, "new-session", "-d", "-s", target, "-x", "80", "-y", "24", "sh", "-c", script}
		if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
			t.Fatalf("start pane: %v: %s", err, out)
		}
		deadline := time.Now().Add(10 * time.Second)
		for {
			if _, err := os.Stat(ready); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("pane %s never became ready", name)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	rig.a, rig.b = sessions[0], sessions[1]

	clock, err := config.NewClock("2025-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
	m := New(db, config.Settings{Color: true, Bookmarks: on, Clock: clock}, "")
	m.width, m.height = 100, 30
	m.baseSessions = sessions
	m.sessions = m.filteredSessions()
	m.selected = rowCursor(0)
	m.tmuxClient = tmux.Client{Socket: socket}
	next, _ := m.enterInteractive()
	got := next.(Model)
	if !got.interactive || got.interactiveDispatcher == nil {
		t.Fatalf("enterInteractive did not enter (attachError=%q refusal=%+v)", got.attachError, got.entryRefusal)
	}
	t.Cleanup(func() { got.exitInteractive() })
	rig.m = got
	return rig
}

func (r *bookmarkRig) press(t *testing.T, msgs ...tea.KeyMsg) {
	t.Helper()
	for _, k := range msgs {
		next, _ := r.m.updateInteractive(k)
		r.m = next.(Model)
	}
}

func (r *bookmarkRig) received(id string) string {
	b, _ := os.ReadFile(r.fileOf[id])
	return string(b)
}

// waitReceived polls until pane id has received exactly want.
func (r *bookmarkRig) waitReceived(t *testing.T, id, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for r.received(id) != want {
		if time.Now().After(deadline) {
			t.Fatalf("pane %s received %q, want %q", id, r.received(id), want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// targetID is the session interactive mode is bound to.
func (r *bookmarkRig) targetID(t *testing.T) string {
	t.Helper()
	s, ok := r.m.interactiveTargetSession()
	if !ok {
		t.Fatal("interactive mode is bound to no session")
	}
	return s.ID
}

// bookmarkKeyRow is one key sequence of the attached bookmark tables and the
// bytes a pane receives when none of it is intercepted.
type bookmarkKeyRow struct {
	name string
	keys []tea.KeyMsg
	raw  string
	slot int // slot a digit-completed or shifted row sets, or the slot a jump row names
}

func bookmarkInterceptRows() []bookmarkKeyRow {
	var rows []bookmarkKeyRow
	for d := 0; d <= 9; d++ {
		r := rune('0' + d)
		rows = append(rows, bookmarkKeyRow{name: "jump alt+" + string(r), keys: []tea.KeyMsg{altRune(r)}, raw: "\x1b" + string(r), slot: d})
	}
	rows = append(rows, bookmarkKeyRow{name: "chord start alt+m", keys: []tea.KeyMsg{altRune('m')}, raw: "\x1bm", slot: -1})
	rows = append(rows, bookmarkKeyRow{name: "chord alt+m then 7", keys: []tea.KeyMsg{altRune('m'), key("7")}, raw: "\x1bm7", slot: 7})
	for i, r := range ")!@#$%^&*(" {
		rows = append(rows, bookmarkKeyRow{name: "set alt+" + string(r), keys: []tea.KeyMsg{altRune(r)}, raw: "\x1b" + string(r), slot: i})
	}
	return rows
}

// TestInteractiveBookmarkIntercept: with bookmarks on, the jump, chord and
// slot-set keys are consumed next to Ctrl+Q and no byte of them reaches a pane.
// A jump switches the attached view to the slot's session, still interactive.
func TestInteractiveBookmarkIntercept(t *testing.T) {
	rows := bookmarkInterceptRows()
	if len(rows) != 22 {
		t.Fatalf("table has %d rows, want 22", len(rows))
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			r := newBookmarkRig(t, true)
			jump := strings.HasPrefix(row.name, "jump")
			if jump {
				if err := r.db.AssignBookmark(context.Background(), row.slot, r.b.ID); err != nil {
					t.Fatal(err)
				}
			}
			r.press(t, row.keys...)
			// A key forwarded after the interception reaches the pane in
			// order, so anything the interception leaked would precede it.
			r.press(t, key("Z"))
			wantTarget := r.a.ID
			if jump {
				wantTarget = r.b.ID
			}
			if !r.m.interactive || r.targetID(t) != wantTarget {
				t.Fatalf("interactive=%v target=%q, want interactive on %q", r.m.interactive, r.targetID(t), wantTarget)
			}
			other := r.b.ID
			if jump {
				other = r.a.ID
			}
			r.waitReceived(t, wantTarget, "Z")
			if got := r.received(other); got != "" {
				t.Fatalf("the other pane received %q, want nothing", got)
			}
			if jump {
				if sel := selectedID(t, r.m); sel != r.b.ID {
					t.Fatalf("selection = %q after the jump, want %q", sel, r.b.ID)
				}
				return
			}
			if row.slot >= 0 {
				slots, err := r.db.Bookmarks(context.Background())
				if err != nil || slots[row.slot] != r.a.ID {
					t.Fatalf("slot %d = %q (err %v), want the attached session %q", row.slot, slots[row.slot], err, r.a.ID)
				}
			}
		})
	}
}

// TestInteractiveBookmarkForward: with bookmarks on, a bare digit, Alt plus a
// non-digit and an Esc that cancels a pending chord all reach the pane
// unchanged.
func TestInteractiveBookmarkForward(t *testing.T) {
	rows := []bookmarkKeyRow{
		{name: "bare digit", keys: []tea.KeyMsg{key("5")}, raw: "5"},
		{name: "alt+a", keys: []tea.KeyMsg{altRune('a')}, raw: "\x1ba"},
		{name: "alt+x", keys: []tea.KeyMsg{altRune('x')}, raw: "\x1bx"},
		{name: "alt+.", keys: []tea.KeyMsg{altRune('.')}, raw: "\x1b."},
		{name: "esc after alt+m", keys: []tea.KeyMsg{altRune('m'), {Type: tea.KeyEscape}}, raw: "\x1b"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			r := newBookmarkRig(t, true)
			r.press(t, row.keys...)
			r.waitReceived(t, r.a.ID, row.raw)
			if r.m.bookmarkChord {
				t.Fatal("a chord is still pending")
			}
			if slots, _ := r.db.Bookmarks(context.Background()); len(slots) != 0 {
				t.Fatalf("a forwarded key wrote bookmarks: %v", slots)
			}
		})
	}
}

// TestInteractiveBookmarkKeysAreForwardedWhileTheSettingIsOff: with
// bookmarks off, the 22 intercept rows reach the pane byte for byte.
func TestInteractiveBookmarkKeysAreForwardedWhileTheSettingIsOff(t *testing.T) {
	rows := bookmarkInterceptRows()
	if len(rows) != 22 {
		t.Fatalf("table has %d rows, want 22", len(rows))
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			r := newBookmarkRig(t, false)
			if err := r.db.AssignBookmark(context.Background(), 0, r.b.ID); err != nil {
				t.Fatal(err)
			}
			r.press(t, row.keys...)
			r.waitReceived(t, r.a.ID, row.raw)
			if !r.m.interactive || r.targetID(t) != r.a.ID {
				t.Fatalf("the view left %q", r.a.ID)
			}
			if got := r.received(r.b.ID); got != "" {
				t.Fatalf("the other pane received %q", got)
			}
		})
	}
}

// TestInteractiveBookmarkJumpThatCannotSwitchKeepsTheView: a jump to an empty
// slot, to the session already shown or to a stopped session leaves the attached
// view where it is, forwards nothing and shows a one-line message in the
// rendered interactive footer, which the next key clears and forwards.
func TestInteractiveBookmarkJumpThatCannotSwitchKeepsTheView(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(r *bookmarkRig)
		want  string
	}{
		{"empty slot", func(*bookmarkRig) {}, "Bookmark 3 is empty"},
		{"the session already shown", func(r *bookmarkRig) {
			if err := r.db.AssignBookmark(context.Background(), 3, r.a.ID); err != nil {
				t.Fatal(err)
			}
		}, "Bookmark 3: bkalpha"},
		{"a stopped session", func(r *bookmarkRig) {
			if err := r.db.AssignBookmark(context.Background(), 3, r.b.ID); err != nil {
				t.Fatal(err)
			}
			r.m.sessions[1].Status = "stopped"
		}, "Bookmark 3: bkbravo is stopped"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newBookmarkRig(t, true)
			tc.setup(r)
			r.press(t, altRune('3'))
			if !r.m.interactive || r.targetID(t) != r.a.ID {
				t.Fatalf("the view left %q", r.a.ID)
			}
			if footer := stripANSI(r.m.footerLine()); !strings.Contains(footer, tc.want) {
				t.Fatalf("rendered footer = %q, want it to contain %q", footer, tc.want)
			}
			if view := stripANSI(r.m.View()); !strings.Contains(view, tc.want) {
				t.Fatalf("rendered view lacks %q:\n%s", tc.want, view)
			}
			r.press(t, key("Z"))
			if footer := stripANSI(r.m.footerLine()); strings.Contains(footer, tc.want) {
				t.Fatalf("the next key left the message on the footer: %q", footer)
			}
			r.waitReceived(t, r.a.ID, "Z")
			if got := r.received(r.b.ID); got != "" {
				t.Fatalf("the other pane received %q", got)
			}
		})
	}
}

// TestInteractiveBookmarkMessagesRenderInTheAttachedFooter: the chord prompt,
// a slot set and a successful jump each show their one-line message in the
// rendered interactive footer.
func TestInteractiveBookmarkMessagesRenderInTheAttachedFooter(t *testing.T) {
	for _, tc := range []struct {
		name  string
		keys  []tea.KeyMsg
		want  string
		after string
	}{
		{"chord pending", []tea.KeyMsg{altRune('m')}, "Bookmark: press a digit 0-9 to set the slot", "id-bkalpha"},
		{"slot set", []tea.KeyMsg{altRune('!')}, "Bookmark 1 set to bkalpha", "id-bkalpha"},
		{"jump", []tea.KeyMsg{altRune('4')}, "Bookmark 4: bkbravo", "id-bkbravo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newBookmarkRig(t, true)
			if err := r.db.AssignBookmark(context.Background(), 4, r.b.ID); err != nil {
				t.Fatal(err)
			}
			r.press(t, tc.keys...)
			if !r.m.interactive || r.targetID(t) != tc.after {
				t.Fatalf("interactive=%v target=%q, want %q", r.m.interactive, r.targetID(t), tc.after)
			}
			if footer := stripANSI(r.m.footerLine()); !strings.Contains(footer, tc.want) {
				t.Fatalf("rendered footer = %q, want it to contain %q", footer, tc.want)
			}
		})
	}
}
