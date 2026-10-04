package tui

import (
	"context"
	"os/exec"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// GH #52 (SPEC §11's newly-created bullet, §9.1, §11.9): a create from
// `n` (under [ui] attach_on_new) and a resume or restart from `r`/`R`
// (under [ui] attach_on_resume) enter the interactive preview on that
// session in the client that asked, once its pane is live, through one
// shared intent and `↵`'s own entry. Every test here
// runs against a real tmux server on a private socket, with the real
// capture engine wired as previewCapture, because "its pane is live" is
// the capture engine's own answer and entry's success path cannot be
// reached any other way.

// aeSession is the row every test here creates: `starting`, the status a
// brand-new session's first load actually shows.
var aeSession = store.Session{ID: "ae-born", Name: "born", Slug: "born", Status: "starting"}

// aeTmuxSession is aeSession's own tmux session name (tmux.SessionName).
const aeTmuxSession = "deck_born"

// aeModel builds a client wired to socket the way cmd/deck wires one,
// with prepareAttach counting the attachment transactions entry records.
func aeModel(t *testing.T, socket string, attachOnNew bool, height int, attachments *[]string) Model {
	t.Helper()
	return aeModelWith(t, socket, config.Settings{Color: true, AttachOnNew: attachOnNew}, 100, height, attachments)
}

func aeModelWith(t *testing.T, socket string, settings config.Settings, width, height int, attachments *[]string) Model {
	t.Helper()
	m := New(nil, settings, "")
	m.width, m.height = width, height
	client := tmux.Client{Socket: socket}
	m.tmuxClient = client
	m.previewCapture = client.CapturePreview
	m.prepareAttach = func(_ context.Context, id string) error {
		*attachments = append(*attachments, id)
		return nil
	}
	return m
}

func aeUpdate(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

// aeTick delivers one previewTick and then, synchronously, the capture
// that tick issues for the selected row -- the same pair a running
// program's event loop delivers, minus the goroutine.
func aeTick(m Model) Model {
	m = aeUpdate(m, previewTick(time.Now()))
	if cmd := m.capturePreview(); cmd != nil {
		m = aeUpdate(m, cmd())
	}
	return m
}

// aeCreate drives the create the way a user does: `n` opens the modal,
// the submit's shellCreated lands, and the reload that follows lists the
// new row.
func aeCreate(t *testing.T, m Model) Model {
	t.Helper()
	m = aeUpdate(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if !m.creating {
		t.Fatalf("fixture: n did not open the create modal")
	}
	m = aeUpdate(m, shellCreated{session: aeSession})
	m = aeUpdate(m, sessionsLoaded{sessions: []store.Session{aeSession}})
	if session, ok := m.selectedSession(); !ok || session.ID != aeSession.ID {
		t.Fatalf("fixture: the created session was not selected after its first load: (%+v, %v)", session, ok)
	}
	return m
}

func aeAssertInteractiveOnBorn(t *testing.T, m Model) {
	t.Helper()
	if !m.interactive {
		t.Fatalf("not interactive; entryRefusal = %+v", m.entryRefusal)
	}
	if session, ok := m.interactiveTargetSession(); !ok || session.ID != aeSession.ID {
		t.Fatalf("interactive target = (%+v, %v), want the created session %q", session, ok, aeSession.ID)
	}
}

// TestAttachOnNewEntersOnceThePaneIsLive: the intent waits through ticks
// on which the new session has no pane -- no entry and, crucially, no
// "no live pane" refusal -- then enters on the first capture that finds
// the pane live, recording the attachment exactly once, and never again.
func TestAttachOnNewEntersOnceThePaneIsLive(t *testing.T) {
	socket := selectionTestSocket("aelive")
	var attachments []string
	m := aeModel(t, socket, true, 30, &attachments)
	m = aeCreate(t, m)
	if m.interactive {
		t.Fatalf("entered interactive mode on the create's own reload, before any capture reported the pane live")
	}

	for i := 0; i < 2; i++ {
		m = aeTick(m)
		if m.interactive {
			t.Fatalf("tick %d: entered interactive mode before the pane existed", i)
		}
		if m.entryRefusal.active {
			t.Fatalf("tick %d: the wait for a live pane raised a refusal: %+v", i, m.entryRefusal)
		}
	}

	newQuietSelectionPane(t, socket, aeTmuxSession, 80, 24)
	m = aeTick(m)
	aeAssertInteractiveOnBorn(t, m)
	if m.pendingAutoEnterSessionID != "" {
		t.Fatalf("pendingAutoEnterSessionID = %q after entry, want it consumed", m.pendingAutoEnterSessionID)
	}
	if len(attachments) != 1 || attachments[0] != aeSession.ID {
		t.Fatalf("attachments = %v, want exactly one for %q -- entry must run the same transaction as ↵", attachments, aeSession.ID)
	}

	next, _ := m.exitInteractive()
	m = next.(Model)
	for i := 0; i < 3; i++ {
		m = aeTick(m)
	}
	if m.interactive {
		t.Fatalf("re-entered interactive mode after Ctrl+Q's exit -- the intent is one-shot")
	}
	if session, ok := m.selectedSession(); !ok || session.ID != aeSession.ID {
		t.Fatalf("selection after exit = (%+v, %v), want the created session still selected", session, ok)
	}
	if len(attachments) != 1 {
		t.Fatalf("attachments = %v after exit and more ticks, want still exactly one", attachments)
	}
}

// TestAttachOnNewCancelledByKeyBeforePaneIsLive: a key pressed while the
// intent still waits cancels it for good. `z` is bound to nothing and
// leaves the selection on the new row, so this proves the key itself
// cancels, not merely a selection that moved off the target. A mouse
// report does the same.
func TestAttachOnNewCancelledByKeyBeforePaneIsLive(t *testing.T) {
	cases := []struct {
		name string
		msg  tea.Msg
	}{
		{"key", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")}},
		{"mouse", tea.MouseMsg{X: 1, Y: 1, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			socket := selectionTestSocket("aecancel" + tc.name)
			var attachments []string
			m := aeModel(t, socket, true, 30, &attachments)
			m = aeCreate(t, m)
			m = aeTick(m)
			if m.interactive {
				t.Fatalf("fixture: entered before the pane existed")
			}

			m = aeUpdate(m, tc.msg)
			if m.pendingAutoEnterSessionID != "" {
				t.Fatalf("pendingAutoEnterSessionID = %q after a %s, want it cancelled", m.pendingAutoEnterSessionID, tc.name)
			}

			newQuietSelectionPane(t, socket, aeTmuxSession, 80, 24)
			for i := 0; i < 3; i++ {
				m = aeTick(m)
			}
			if m.interactive {
				t.Fatalf("entered interactive mode once the pane went live, after a %s had cancelled the intent", tc.name)
			}
			if session, ok := m.selectedSession(); !ok || session.ID != aeSession.ID {
				t.Fatalf("selection = (%+v, %v), want the created session still selected", session, ok)
			}
			if len(attachments) != 0 {
				t.Fatalf("attachments = %v, want none", attachments)
			}
		})
	}
}

// TestAttachOnNewRefusesATooSmallPreviewLikeEnter: with a preview box
// below §11.9's 7-inner-row floor, the intent's one attempt is `↵`'s own
// refusal -- the row-floor banner on the created session, which stays
// selected in the list -- and there is no retry once the room is there.
func TestAttachOnNewRefusesATooSmallPreviewLikeEnter(t *testing.T) {
	socket := selectionTestSocket("aefloor")
	newQuietSelectionPane(t, socket, aeTmuxSession, 80, 24)
	var attachments []string
	m := aeModel(t, socket, true, 8, &attachments)
	if _, height := m.previewContentSize(); height >= interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is not below the %d-row floor", height, interactiveMinInnerRows)
	}
	m = aeCreate(t, m)
	m = aeTick(m)

	if m.interactive {
		t.Fatalf("entered interactive mode below the row floor")
	}
	if !m.entryRefusal.active || m.entryRefusal.kind != entryRefusalRowFloor || m.entryRefusal.sessionID != aeSession.ID {
		t.Fatalf("entryRefusal = %+v, want ↵'s row-floor refusal on %q", m.entryRefusal, aeSession.ID)
	}
	if _, ok := m.activeEntryRefusalForSelection(); !ok {
		t.Fatalf("the refusal is not the selected row's banner")
	}
	if session, ok := m.selectedSession(); !ok || session.ID != aeSession.ID {
		t.Fatalf("selection = (%+v, %v), want the created session still selected", session, ok)
	}
	if m.pendingAutoEnterSessionID != "" {
		t.Fatalf("pendingAutoEnterSessionID = %q after the refused attempt, want it spent", m.pendingAutoEnterSessionID)
	}

	m = aeUpdate(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	for i := 0; i < 3; i++ {
		m = aeTick(m)
	}
	if m.interactive {
		t.Fatalf("retried entry once the preview grew past the floor -- a refused auto-entry must not retry")
	}
	if len(attachments) != 0 {
		t.Fatalf("attachments = %v, want none -- a refused entry claims nothing", attachments)
	}
}

// TestAttachOnNewOffSelectsOnly: with [ui] attach_on_new off the
// create selects the new session and nothing more, however live its pane.
func TestAttachOnNewOffSelectsOnly(t *testing.T) {
	socket := selectionTestSocket("aeoff")
	newQuietSelectionPane(t, socket, aeTmuxSession, 80, 24)
	var attachments []string
	m := aeModel(t, socket, false, 30, &attachments)
	m = aeCreate(t, m)
	if m.pendingAutoEnterSessionID != "" {
		t.Fatalf("pendingAutoEnterSessionID = %q with attach_on_new off, want it never armed", m.pendingAutoEnterSessionID)
	}
	for i := 0; i < 3; i++ {
		m = aeTick(m)
	}
	if m.interactive {
		t.Fatalf("entered interactive mode with attach_on_new off")
	}
	if m.entryRefusal.active {
		t.Fatalf("entryRefusal = %+v with attach_on_new off, want no entry attempted at all", m.entryRefusal)
	}
	if session, ok := m.selectedSession(); !ok || session.ID != aeSession.ID {
		t.Fatalf("selection = (%+v, %v), want the created session selected", session, ok)
	}
	if len(attachments) != 0 {
		t.Fatalf("attachments = %v, want none", attachments)
	}
}

// TestAttachOnNewNeverFiresInASecondClient: a second client on the same
// socket sees the same new row -- selected, even, since it is that
// client's only row -- with a live pane, and never enters, because the
// intent lives on the creating client's Model alone. The creating client
// still enters.
func TestAttachOnNewNeverFiresInASecondClient(t *testing.T) {
	socket := selectionTestSocket("aesecond")
	newQuietSelectionPane(t, socket, aeTmuxSession, 80, 24)
	var attachmentsA, attachmentsB []string
	a := aeModel(t, socket, true, 30, &attachmentsA)
	b := aeModel(t, socket, true, 30, &attachmentsB)

	a = aeCreate(t, a)
	b = aeUpdate(b, sessionsLoaded{sessions: []store.Session{aeSession}})
	if session, ok := b.selectedSession(); !ok || session.ID != aeSession.ID {
		t.Fatalf("fixture: second client did not land on the new row: (%+v, %v)", session, ok)
	}
	for i := 0; i < 3; i++ {
		b = aeTick(b)
	}
	if b.interactive || b.entryRefusal.active {
		t.Fatalf("second client attempted entry (interactive=%v, entryRefusal=%+v) -- only the creating client may", b.interactive, b.entryRefusal)
	}

	a = aeTick(a)
	aeAssertInteractiveOnBorn(t, a)
	defer a.exitInteractive()

	for i := 0; i < 3; i++ {
		b = aeTick(b)
	}
	if b.interactive || b.entryRefusal.active || len(attachmentsB) != 0 {
		t.Fatalf("second client attempted entry after the first entered (interactive=%v, entryRefusal=%+v, attachments=%v)", b.interactive, b.entryRefusal, attachmentsB)
	}
}

// TestAttachOnNewDropsAfterItsTickBudget: a pane that is still not live
// when the budget runs out leaves the session selected and the intent
// gone, so a pane that turns up later is not entered.
func TestAttachOnNewDropsAfterItsTickBudget(t *testing.T) {
	socket := selectionTestSocket("aebudget")
	var attachments []string
	m := aeModel(t, socket, true, 30, &attachments)
	m = aeCreate(t, m)
	for i := 0; i <= autoEnterTickBudget; i++ {
		m = aeTick(m)
	}
	if m.pendingAutoEnterSessionID != "" {
		t.Fatalf("pendingAutoEnterSessionID = %q after %d ticks, want it dropped", m.pendingAutoEnterSessionID, autoEnterTickBudget+1)
	}
	if m.entryRefusal.active {
		t.Fatalf("entryRefusal = %+v, want the intent dropped silently", m.entryRefusal)
	}

	newQuietSelectionPane(t, socket, aeTmuxSession, 80, 24)
	for i := 0; i < 3; i++ {
		m = aeTick(m)
	}
	if m.interactive {
		t.Fatalf("entered interactive mode after the intent's budget had run out")
	}
	if session, ok := m.selectedSession(); !ok || session.ID != aeSession.ID {
		t.Fatalf("selection = (%+v, %v), want the created session still selected", session, ok)
	}
}

// aeResumeModel is a client whose only row is the "born" session in
// status, selected, with m.resume/m.restart standing in for the service:
// each (re)starts the row's real tmux session on socket, as the service
// does before it returns, and reports service.ResumeStarted.
func aeResumeModel(t *testing.T, socket string, attachOnResume bool, status string, attachments *[]string) Model {
	t.Helper()
	m := aeModelWith(t, socket, config.Settings{Color: true, AttachOnNew: true, AttachOnResume: attachOnResume}, 100, 30, attachments)
	row := aeSession
	row.Status, row.Agent = status, "claude"
	m.sessions = []store.Session{row}
	m.baseSessions = m.sessions
	m.selected = rowCursor(0)
	relaunch := func(context.Context, string) (store.Session, service.ResumeOutcome, error) {
		_ = exec.Command("tmux", "-L", socket, "kill-session", "-t", aeTmuxSession).Run()
		newQuietSelectionPane(t, socket, aeTmuxSession, 80, 24)
		return aeStartedRow(), service.ResumeStarted, nil
	}
	m.resume = relaunch
	m.restart = relaunch
	return m
}

// aeStartedRow is the row as the reload after a resume or restart lists it.
func aeStartedRow() store.Session {
	row := aeSession
	row.Agent = "claude"
	return row
}

// aePressAndRun presses key and runs the command it returns synchronously,
// delivering the resume/restart result the way the event loop would.
func aePressAndRun(t *testing.T, m Model, key string) Model {
	t.Helper()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("fixture: %s issued no command (attachError %q)", key, m.attachError)
	}
	return aeUpdate(m, cmd())
}

// TestAttachOnResumeEntersAfterR: with [ui] attach_on_resume on, a
// successful `r` enters the interactive preview once this client's list
// shows the row no longer stopped and its pane live. A capture of the new
// pane that lands before the resume's own reload does not fire it early
// against the stale `stopped` row -- that would be `↵`'s stopped refusal.
func TestAttachOnResumeEntersAfterR(t *testing.T) {
	socket := selectionTestSocket("aeresume")
	var attachments []string
	m := aeResumeModel(t, socket, true, "stopped", &attachments)
	m = aePressAndRun(t, m, "r")
	if m.pendingAutoEnterSessionID != aeSession.ID {
		t.Fatalf("pendingAutoEnterSessionID = %q after r, want %q", m.pendingAutoEnterSessionID, aeSession.ID)
	}

	m = aeTick(m)
	if m.interactive || m.entryRefusal.active {
		t.Fatalf("acted before the list showed the row resumed (interactive=%v, entryRefusal=%+v)", m.interactive, m.entryRefusal)
	}

	m = aeUpdate(m, sessionsLoaded{sessions: []store.Session{aeStartedRow()}})
	m = aeTick(m)
	aeAssertInteractiveOnBorn(t, m)
	defer m.exitInteractive()
	if len(attachments) != 1 {
		t.Fatalf("attachments = %v, want exactly one", attachments)
	}
}

// TestAttachOnResumeOffByDefault: the default leaves `r` and `R` exactly
// as they were -- the session comes back and the client stays in the list.
func TestAttachOnResumeOffByDefault(t *testing.T) {
	home := t.TempDir()
	settings, err := config.LoadFrom(func(key string) string {
		if key == "DECK_HOME" {
			return home
		}
		return ""
	}, func() (string, error) { return home, nil })
	if err != nil {
		t.Fatal(err)
	}
	if settings.AttachOnResume {
		t.Fatal("attach_on_resume defaults to on, want off")
	}
	for _, tc := range []struct{ key, status string }{{"r", "stopped"}, {"R", "running"}} {
		t.Run(tc.key, func(t *testing.T) {
			socket := selectionTestSocket("aeresumeoff" + tc.status)
			var attachments []string
			m := aeResumeModel(t, socket, settings.AttachOnResume, tc.status, &attachments)
			m = aePressAndRun(t, m, tc.key)
			if m.pendingAutoEnterSessionID != "" {
				t.Fatalf("pendingAutoEnterSessionID = %q after %s with attach_on_resume off, want never armed", m.pendingAutoEnterSessionID, tc.key)
			}
			m = aeUpdate(m, sessionsLoaded{sessions: []store.Session{aeStartedRow()}})
			for i := 0; i < 3; i++ {
				m = aeTick(m)
			}
			if m.interactive || m.entryRefusal.active || len(attachments) != 0 {
				t.Fatalf("%s with attach_on_resume off attempted entry (interactive=%v, entryRefusal=%+v, attachments=%v)", tc.key, m.interactive, m.entryRefusal, attachments)
			}
		})
	}
}

// TestAttachOnResumeEntersAfterRestart: `R` arms the same intent `r` does.
func TestAttachOnResumeEntersAfterRestart(t *testing.T) {
	socket := selectionTestSocket("aerestart")
	newQuietSelectionPane(t, socket, aeTmuxSession, 80, 24)
	var attachments []string
	m := aeResumeModel(t, socket, true, "running", &attachments)
	m = aePressAndRun(t, m, "R")
	if m.pendingAutoEnterSessionID != aeSession.ID {
		t.Fatalf("pendingAutoEnterSessionID = %q after R, want %q", m.pendingAutoEnterSessionID, aeSession.ID)
	}
	m = aeTick(m)
	aeAssertInteractiveOnBorn(t, m)
	defer m.exitInteractive()
	if len(attachments) != 1 {
		t.Fatalf("attachments = %v, want exactly one", attachments)
	}
}

// TestAttachOnResumeNotArmedByUndo: `u` undoing an `x` resumes exactly
// like `r` but is not a resume aimed at working in the session, so it
// never arms the intent, even with attach_on_resume on.
func TestAttachOnResumeNotArmedByUndo(t *testing.T) {
	socket := selectionTestSocket("aeundo")
	var attachments []string
	m := aeResumeModel(t, socket, true, "stopped", &attachments)
	m.undoSessionID, m.undoSessionName = aeSession.ID, aeSession.Name
	m = aePressAndRun(t, m, "u")
	if m.pendingAutoEnterSessionID != "" {
		t.Fatalf("pendingAutoEnterSessionID = %q after u, want the intent never armed", m.pendingAutoEnterSessionID)
	}
}
