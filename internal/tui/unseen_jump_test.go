package tui

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/store"
)

// R326: Alt+U / Alt+Shift+U in the list. Six behaviour cases (next, previous,
// wrap, none unseen, a seen error reachable until seen, space unaffected) and
// the decoder test for Alt+Shift+U.

func altKey(r rune) tea.KeyMsg {
	return tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{r}, Alt: true})
}

// jumpRig is the four-row store-backed fixture: attention order is waiting,
// error, idle, running; running is never unseen.
type jumpRig struct {
	t  *testing.T
	m  Model
	db *store.Store
}

func newJumpRig(t *testing.T, selected string) *jumpRig {
	t.Helper()
	m, db := inertFixture(t, SortOrderAttention, selected, false)
	return &jumpRig{t: t, m: m, db: db}
}

// view marks id viewed; ack marks its status seen.
func (r *jumpRig) view(ids ...string) {
	r.t.Helper()
	for _, id := range ids {
		if _, err := r.db.MarkViewed(context.Background(), id); err != nil {
			r.t.Fatal(err)
		}
	}
}

func (r *jumpRig) ack(ids ...string) {
	r.t.Helper()
	for _, id := range ids {
		if err := r.db.AcknowledgeSession(context.Background(), id); err != nil {
			r.t.Fatal(err)
		}
	}
}

func (r *jumpRig) reload() { r.m = reloadInert(r.t, r.m) }

func (r *jumpRig) unseen() []string {
	var ids []string
	for _, s := range r.m.sessions {
		if s.Unseen() {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

func (r *jumpRig) selectedID() string {
	s, ok := r.m.selectedSession()
	if !ok {
		r.t.Fatal("no session selected")
	}
	return s.ID
}

// press sends msg and settles the landing's viewing write.
func (r *jumpRig) press(msg tea.KeyMsg) {
	r.t.Helper()
	updated, cmd := r.m.Update(msg)
	r.m = settleView(r.t, updated.(Model), cmd)
}

func TestAltUJumpsToTheNextUnseenSessionAndOnlyViewsIt(t *testing.T) {
	r := newJumpRig(t, "row-waiting")
	r.view("row-idle")
	r.reload()
	want := []string{"row-waiting", "row-error"}
	if got := r.unseen(); !reflect.DeepEqual(got, want) {
		t.Fatalf("fixture unseen = %v, want %v", got, want)
	}
	before := snapshotAll(t, r.db)
	order := inertOrder(r.m)

	r.press(altKey('u'))

	if got := r.selectedID(); got != "row-error" {
		t.Fatalf("Alt+U from row-waiting selected %q, want row-error", got)
	}
	after := snapshotAll(t, r.db)
	landed := after["row-error"].Session
	if landed.LastSeenAt != landed.LastOutputAt {
		t.Fatalf("landing did not view the row: last_seen_at=%d last_output_at=%d", landed.LastSeenAt, landed.LastOutputAt)
	}
	// Landing views and nothing else: restore the one column it may write
	// and the whole durable state must be what it was.
	restored := before["row-error"].Session
	restored.LastSeenAt = landed.LastSeenAt
	if !reflect.DeepEqual(after["row-error"].Session, restored) || after["row-error"].Events != before["row-error"].Events {
		t.Fatalf("landing wrote more than last_seen_at:\nbefore=%+v\nafter =%+v", before["row-error"], after["row-error"])
	}
	if landed.StatusSeen() {
		t.Fatal("landing acknowledged the status (seen_status_at moved)")
	}
	for id, snap := range before {
		if id != "row-error" && !reflect.DeepEqual(after[id], snap) {
			t.Fatalf("landing changed another row %s", id)
		}
	}
	if got := inertOrder(r.m); !reflect.DeepEqual(got, order) {
		t.Fatalf("landing moved a row: %v -> %v", order, got)
	}
}

func TestAltShiftUJumpsToThePreviousUnseenSession(t *testing.T) {
	r := newJumpRig(t, "row-idle")
	r.press(altKey('U'))
	if got := r.selectedID(); got != "row-error" {
		t.Fatalf("Alt+Shift+U from row-idle selected %q, want row-error", got)
	}
	r.press(altKey('U'))
	if got := r.selectedID(); got != "row-waiting" {
		t.Fatalf("a second Alt+Shift+U selected %q, want row-waiting", got)
	}
}

func TestAltUAndAltShiftUWrapThroughTheList(t *testing.T) {
	r := newJumpRig(t, "row-idle")
	r.press(altKey('u')) // idle is the last unseen row; running is never unseen
	if got := r.selectedID(); got != "row-waiting" {
		t.Fatalf("Alt+U from the last unseen row selected %q, want a wrap to row-waiting", got)
	}
	r.press(altKey('U'))
	if got := r.selectedID(); got != "row-idle" {
		t.Fatalf("Alt+Shift+U from the first unseen row selected %q, want a wrap to row-idle", got)
	}
}

func TestAltUWithNothingUnseenShowsAOneLineMessageAndMovesNothing(t *testing.T) {
	r := newJumpRig(t, "row-error")
	r.view("row-waiting", "row-error", "row-idle")
	r.ack("row-waiting", "row-error")
	r.reload()
	if got := r.unseen(); len(got) != 0 {
		t.Fatalf("fixture still has unseen rows: %v", got)
	}
	before := snapshotAll(t, r.db)
	for _, k := range []rune{'u', 'U'} {
		r.press(altKey(k))
		if got := r.selectedID(); got != "row-error" {
			t.Fatalf("Alt+%c with nothing unseen moved the selection to %q", k, got)
		}
		line := r.m.footerLine()
		if !strings.Contains(line, noUnseenNote) || strings.Contains(line, "\n") {
			t.Fatalf("footer after Alt+%c = %q, want the one-line %q", k, line, noUnseenNote)
		}
	}
	for id, snap := range before {
		if got := snapshotDurable(t, r.db, id); !reflect.DeepEqual(got, snap) {
			t.Fatalf("a jump with nothing unseen wrote row %s", id)
		}
	}
}

func TestASeenErrorIsReachableByAltUUntilItIsViewed(t *testing.T) {
	r := newJumpRig(t, "row-waiting")
	r.view("row-waiting", "row-idle")
	r.ack("row-waiting", "row-error") // the error's status is seen; its output is not
	r.reload()
	errRow := r.m.sessions[indexOfSessionID(r.m.sessions, "row-error")]
	if !errRow.StatusSeen() || NeedsAttention(errRow) {
		t.Fatal("fixture: the error row must be a seen error")
	}
	if got := r.unseen(); !reflect.DeepEqual(got, []string{"row-error"}) {
		t.Fatalf("fixture unseen = %v, want only the seen error with unviewed output", got)
	}

	r.press(altKey('u'))
	if got := r.selectedID(); got != "row-error" {
		t.Fatalf("Alt+U skipped the seen error with unviewed output: selected %q", got)
	}
	if got := r.unseen(); len(got) != 0 {
		t.Fatalf("after landing the seen error is still unseen: %v", got)
	}
	r.press(altKey('u'))
	if line := r.m.footerLine(); !strings.Contains(line, noUnseenNote) {
		t.Fatalf("a second Alt+U footer = %q, want %q once the error is viewed", line, noUnseenNote)
	}
}

func TestSpaceIsUnaffectedByTheUnseenJumps(t *testing.T) {
	r := newJumpRig(t, "row-waiting")
	r.ack("row-error")
	r.view("row-waiting")
	r.reload()
	// Only row-waiting is blocked on me; row-error (seen, sorted after idle) and row-idle are unseen.
	if got := r.unseen(); !reflect.DeepEqual(got, []string{"row-waiting", "row-idle", "row-error"}) {
		t.Fatalf("fixture unseen = %v", got)
	}
	r.press(altKey('u'))
	r.press(altKey('u'))
	if got := r.selectedID(); got != "row-error" {
		t.Fatalf("two Alt+U presses selected %q, want the unseen seen-error row-error", got)
	}
	r.press(key(" "))
	if got := r.selectedID(); got != "row-waiting" {
		t.Fatalf("space from row-error selected %q, want row-waiting (status-blocked only)", got)
	}
	r.press(key(" "))
	if got := r.selectedID(); got != "row-waiting" {
		t.Fatalf("space selected %q: it must never land on a row that is merely unseen", got)
	}
}

// tmuxBytesFor sends tmux key name to a bare cat pane and returns the bytes
// that reached the pane, read back from capture-pane's caret notation.
func tmuxBytesFor(t *testing.T, name string) []byte {
	t.Helper()
	socket := selectionTestSocket("altu-" + strings.ToLower(name[:1]) + name[len(name)-1:])
	if out, err := exec.Command("tmux", "-L", socket, "new-session", "-d", "-s", "s0", "-x", "80", "-y", "10", "cat").CombinedOutput(); err != nil {
		t.Fatalf("start cat session: %v: %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	if out, err := exec.Command("tmux", "-L", socket, "send-keys", "-t", "s0", name).CombinedOutput(); err != nil {
		t.Fatalf("send-keys %s: %v: %s", name, err, out)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		capture := strings.TrimRight(tmuxCapturePane(t, socket, "s0"), "\n")
		if capture != "" || time.Now().After(deadline) {
			return []byte(strings.ReplaceAll(capture, "^[", "\x1b"))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

type decodeProbe struct{ got chan tea.KeyMsg }

func (decodeProbe) Init() tea.Cmd { return nil }
func (p decodeProbe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		p.got <- k
		return p, tea.Quit
	}
	return p, nil
}
func (decodeProbe) View() string { return "" }

// decodeKey runs bytes through bubbletea's own input decoder.
func decodeKey(t *testing.T, in []byte) tea.KeyMsg {
	t.Helper()
	probe := decodeProbe{got: make(chan tea.KeyMsg, 4)}
	p := tea.NewProgram(probe, tea.WithInput(bytes.NewReader(in)), tea.WithOutput(io.Discard), tea.WithoutSignalHandler())
	done := make(chan error, 1)
	go func() { _, err := p.Run(); done <- err }()
	select {
	case k := <-probe.got:
		<-done
		return k
	case <-time.After(5 * time.Second):
		p.Kill()
		t.Fatalf("bubbletea decoded no key from %q", in)
	}
	return tea.KeyMsg{}
}

// TestAltShiftUIsDistinguishableFromAltUThroughTmux proves, rather than
// assumes, that tmux keeps the case of M-u / M-U (ESC u vs ESC U) and that the
// decoder deck reads its input through names them "alt+u" and "alt+U", the
// two list keys. So the gap Alt+Shift+Tab has (tmux discards its modifier)
// does not apply here.
func TestAltShiftUIsDistinguishableFromAltUThroughTmux(t *testing.T) {
	lower := tmuxBytesFor(t, "M-u")
	upper := tmuxBytesFor(t, "M-U")
	if string(lower) != "\x1bu" || string(upper) != "\x1bU" {
		t.Fatalf("tmux delivered M-u as %q and M-U as %q, want ESC u and ESC U", lower, upper)
	}
	if got := decodeKey(t, lower).String(); got != unseenNextKey {
		t.Fatalf("the decoder names ESC u %q, want %q", got, unseenNextKey)
	}
	if got := decodeKey(t, upper).String(); got != unseenPreviousKey {
		t.Fatalf("the decoder names ESC U %q, want %q", got, unseenPreviousKey)
	}
}

// TestHelpAndSpecKeymapListBothUnseenChords: the `?` help and SPEC §11's
// keymap both name the two chords.
func TestHelpAndSpecKeymapListBothUnseenChords(t *testing.T) {
	help := helpText(false)
	spec := specText(t)
	for _, chord := range []string{"alt+u", "alt+Shift+U"} {
		if !strings.Contains(help, chord) {
			t.Errorf("the ? help does not list %q", chord)
		}
	}
	for _, chord := range []string{"`Alt+U`", "`Alt+Shift+U`"} {
		if !strings.Contains(spec, "· "+chord) && !strings.Contains(spec, "/ "+chord) {
			t.Errorf("SPEC §11's keymap does not list %s", chord)
		}
	}
}
