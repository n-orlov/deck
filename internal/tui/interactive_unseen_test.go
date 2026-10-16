package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// unseenRig is an attached (interactive) model over n real tmux panes, each a
// raw `cat` appending every byte it receives to its own file. order is the
// session ids in painted list order; the view starts attached to order[0].
type unseenRig struct {
	m      Model
	order  []string
	fileOf map[string]string
}

// newUnseenRig attaches to the first row of n sessions. unseen names the list
// positions (0-based, painted order) that carry unseen output; every other
// row is seen. Bookmarks are off: the unseen jumps do not depend on them.
func newUnseenRig(t *testing.T, n int, unseen ...int) *unseenRig {
	t.Helper()
	home, err := os.MkdirTemp("", "unseenattach")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	socket := selectionTestSocket("unseenattach")
	rig := &unseenRig{fileOf: map[string]string{}}
	cwd := t.TempDir()
	var sessions []store.Session
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("uj%c", 'a'+i)
		s, err := db.CreateSession(context.Background(), store.CreateSessionInput{
			ID: "id-" + name, Name: name, CWD: cwd, Agent: "claude",
			CapturedPath: "/bin/true", Status: "idle", StatusAt: 1, CreatedAt: int64(i + 1),
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

	clock, err := config.NewClock("2025-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
	m := New(db, config.Settings{Color: true, Clock: clock}, "")
	m.width, m.height = 100, 30
	m.baseSessions = sessions
	m.sessions = m.filteredSessions()
	for _, c := range m.visualOrder() {
		if idx, ok := c.SessionIndex(); ok {
			rig.order = append(rig.order, m.sessions[idx].ID)
		}
	}
	mark := map[string]bool{}
	for _, pos := range unseen {
		mark[rig.order[pos]] = true
	}
	for i := range m.sessions {
		if mark[m.sessions[i].ID] {
			m.sessions[i].LastOutputAt, m.sessions[i].LastSeenAt = 10, 0
		}
	}
	for i := range m.baseSessions {
		if mark[m.baseSessions[i].ID] {
			m.baseSessions[i].LastOutputAt, m.baseSessions[i].LastSeenAt = 10, 0
		}
	}
	first := indexOfSessionID(m.sessions, rig.order[0])
	m.selected = rowCursor(first)
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

func (r *unseenRig) press(msgs ...tea.KeyMsg) {
	for _, k := range msgs {
		next, _ := r.m.Update(k)
		r.m = next.(Model)
	}
}

func (r *unseenRig) received(id string) string {
	b, _ := os.ReadFile(r.fileOf[id])
	return string(b)
}

func (r *unseenRig) waitReceived(t *testing.T, id, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for r.received(id) != want {
		if time.Now().After(deadline) {
			t.Fatalf("pane %s received %q, want %q", id, r.received(id), want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (r *unseenRig) target(t *testing.T) string {
	t.Helper()
	s, ok := r.m.interactiveTargetSession()
	if !ok {
		t.Fatal("interactive mode is bound to no session")
	}
	return s.ID
}

// requireAttachedTo asserts interactive mode is on and bound to id, and that
// a following ordinary key reaches that pane alone (so the attachment is
// live, not merely recorded).
func (r *unseenRig) requireAttachedTo(t *testing.T, id string) {
	t.Helper()
	if !r.m.interactive || r.target(t) != id {
		t.Fatalf("interactive=%v target=%q, want interactive on %q", r.m.interactive, r.target(t), id)
	}
}

func altShiftU() tea.KeyMsg { return altRune('U') }

// Forward jump: Alt+U from the first row lands on the next unseen row, skipping
// a seen one, and the pane that receives the next key is the new target's.
func TestInteractiveUnseenJumpForward(t *testing.T) {
	r := newUnseenRig(t, 3, 2)
	r.press(altRune('u'))
	r.requireAttachedTo(t, r.order[2])
	r.press(key("Z"))
	r.waitReceived(t, r.order[2], "Z")
}

// Backward jump: Alt+Shift+U from the first row wraps to the last unseen row
// and, with two unseen rows, picks the previous one rather than the next.
func TestInteractiveUnseenJumpBackward(t *testing.T) {
	r := newUnseenRig(t, 3, 1, 2)
	r.press(altShiftU())
	r.requireAttachedTo(t, r.order[2])
	r.press(altShiftU())
	r.requireAttachedTo(t, r.order[1])
	r.press(altRune('u'))
	r.requireAttachedTo(t, r.order[2])
	r.press(key("Z"))
	r.waitReceived(t, r.order[2], "Z")
}

// With no other unseen session both chords leave the view where it is and say
// so in the footer note slot until the next key.
func TestInteractiveUnseenJumpWithNoneUnseenKeepsTheView(t *testing.T) {
	for name, chord := range map[string]tea.KeyMsg{"alt+u": altRune('u'), "alt+U": altShiftU()} {
		t.Run(name, func(t *testing.T) {
			r := newUnseenRig(t, 3)
			r.press(chord)
			r.requireAttachedTo(t, r.order[0])
			if got := r.m.footerNote(); got != noUnseenNote {
				t.Fatalf("footer note = %q, want %q", got, noUnseenNote)
			}
			r.press(key("Z"))
			if got := r.m.footerNote(); got == noUnseenNote {
				t.Fatalf("the note outlived the next key")
			}
			r.waitReceived(t, r.order[0], "Z")
			for _, other := range r.order[1:] {
				if got := r.received(other); got != "" {
					t.Fatalf("pane %s received %q", other, got)
				}
			}
		})
	}
}

// Zero bytes: neither chord, switching or not, writes anything to any pane.
// A sentinel key sent last arrives alone, so a leaked byte would precede it.
func TestInteractiveUnseenKeysForwardZeroBytes(t *testing.T) {
	for name, chord := range map[string]tea.KeyMsg{"alt+u": altRune('u'), "alt+U": altShiftU()} {
		for _, unseen := range [][]int{{1}, nil} {
			t.Run(fmt.Sprintf("%s unseen=%v", name, unseen), func(t *testing.T) {
				r := newUnseenRig(t, 2, unseen...)
				r.press(chord)
				r.press(key("Z"))
				attached := r.target(t)
				r.waitReceived(t, attached, "Z")
				for _, id := range r.order {
					if id != attached {
						if got := r.received(id); got != "" {
							t.Fatalf("pane %s received %q, want nothing", id, got)
						}
					}
				}
			})
		}
	}
}

// The attached view survives a jump: still interactive, with a live
// dispatcher and grid, and a second jump works from the new view.
func TestInteractiveUnseenJumpStaysInteractive(t *testing.T) {
	r := newUnseenRig(t, 3, 1, 2)
	for _, want := range []string{r.order[1], r.order[2], r.order[1]} {
		chord := altRune('u')
		if want == r.order[1] && r.target(t) == r.order[2] {
			chord = altShiftU()
		}
		r.press(chord)
		r.requireAttachedTo(t, want)
		if r.m.interactiveDispatcher == nil || r.m.interactiveGrid == nil {
			t.Fatalf("no live dispatcher/grid after the jump to %q", want)
		}
		if sel := selectedID(t, r.m); sel != want {
			t.Fatalf("selection = %q, want %q", sel, want)
		}
	}
}

// The corpus of keys that are not the unseen chords reaches the pane with the
// same bytes it received before R327 existed (recorded at the parent commit).
func TestInteractiveForwardCorpusIsUnchangedByTheUnseenKeys(t *testing.T) {
	corpus := []struct {
		name string
		key  tea.KeyMsg
		raw  string
	}{
		{"u", key("u"), "u"},
		{"U", key("U"), "U"},
		{"alt+i", altRune('i'), "\x1bi"},
		{"alt+y", altRune('y'), "\x1by"},
		{"alt+a", altRune('a'), "\x1ba"},
		{"ctrl+u", tea.KeyMsg{Type: tea.KeyCtrlU}, "\x15"},
		{"alt+up", tea.KeyMsg{Type: tea.KeyUp, Alt: true}, "\x1b[1;3A"},
		{"enter", tea.KeyMsg{Type: tea.KeyEnter}, "\r"},
		{"esc", tea.KeyMsg{Type: tea.KeyEscape}, "\x1b"},
	}
	for _, row := range corpus {
		t.Run(row.name, func(t *testing.T) {
			r := newUnseenRig(t, 2, 1)
			r.press(row.key)
			r.waitReceived(t, r.order[0], row.raw)
			r.requireAttachedTo(t, r.order[0])
			if got := r.received(r.order[1]); got != "" {
				t.Fatalf("the other pane received %q", got)
			}
		})
	}
}

// The interactive footer and the `?` help state both chords (R327); on a
// narrow footer the segment drops before Ctrl+Q, which stays whole.
func TestInteractiveFooterAndHelpNameTheUnseenJumps(t *testing.T) {
	r := newUnseenRig(t, 2)
	footer := footerLineOf(r.m.View())
	for _, want := range []string{"Alt+U", "Alt+Shift+U", "unseen", "Ctrl+Q"} {
		if !strings.Contains(stripANSI(footer), want) {
			t.Errorf("interactive footer %q does not state %q", footer, want)
		}
	}
	narrow := r.m
	narrow.width = 70
	if got := narrow.interactiveFooterLineWithin(70); strings.Contains(stripANSI(got), "Alt+Shift+U") || !strings.Contains(stripANSI(got), "Ctrl+Q leave interactive mode") {
		t.Errorf("a 70-column footer = %q, want the unseen segment dropped and Ctrl+Q whole", got)
	}
	help := squashFrame(helpText(false))
	for _, want := range []string{"alt+u / alt+Shift+U", "unseen"} {
		if !strings.Contains(stripANSI(help), want) {
			t.Errorf("the help text does not state %q", want)
		}
	}
}
