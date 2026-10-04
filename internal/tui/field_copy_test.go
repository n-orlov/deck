package tui

import (
	"bytes"
	"encoding/base64"
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// R180: alt+w copies the focused text field's whole text through the copy path
// drag-to-copy uses (tmux.Client.SetSelectionBuffer, then the best-effort OSC
// 52 write) and shows drag-to-copy's own confirmation. A masked secret copies
// nothing and says so; revealed, it copies.

var altW = tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'w'}, Alt: true})

// fieldCopyHarness is a private tmux server (outside the deck namespace) the
// copy lands in and a captured OSC 52 writer.
func fieldCopyHarness(t *testing.T) (client tmux.Client, selectionBuffer func() (string, error), osc *bytes.Buffer) {
	t.Helper()
	osc = &bytes.Buffer{}
	previous := oscClipboardWriter
	oscClipboardWriter = osc
	t.Cleanup(func() { oscClipboardWriter = previous })

	socket := selectionTestSocket("fieldcopy")
	newBareSelectionSession(t, socket, "fieldcopytarget")
	client = tmux.Client{Socket: socket}
	selectionBuffer = func() (string, error) {
		out, err := exec.Command("tmux", "-L", socket, "show-buffer", "-b", tmux.SelectionBufferName).CombinedOutput()
		return string(out), err
	}
	return client, selectionBuffer, osc
}

func oscPayload(t *testing.T, osc *bytes.Buffer) string {
	t.Helper()
	got := osc.String()
	const prefix, suffix = "\x1b]52;c;", "\x07"
	if !strings.HasPrefix(got, prefix) || !strings.HasSuffix(got, suffix) {
		t.Fatalf("OSC 52 write %q is not ESC]52;c;...BEL", got)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(strings.TrimPrefix(got, prefix), suffix))
	if err != nil {
		t.Fatal(err)
	}
	return string(decoded)
}

func TestAltWCopiesTheCreateModalNameFieldWholeThroughTheCopyPath(t *testing.T) {
	client, selectionBuffer, osc := fieldCopyHarness(t)
	m := createEditOpen(t, createEditShortDir(t)).WithTmuxClient(client)
	m = createEditPress(t, m, "m", "y", "space", "n", "a", "m", "e")
	if got := m.createEdits[0].Value(); got != "my name" {
		t.Fatalf("name field = %q before copy", got)
	}
	// Move the caret off the end: the copy is the WHOLE text, not the part
	// before the caret.
	m = createEditPress(t, m, "home")

	updated, _ := m.Update(altW)
	m = updated.(Model)

	if out, err := selectionBuffer(); err != nil || out != "my name" {
		t.Fatalf("show-buffer -b %s = %q, %v; want %q", tmux.SelectionBufferName, out, err, "my name")
	}
	if got := oscPayload(t, osc); got != "my name" {
		t.Fatalf("OSC 52 payload = %q, want %q", got, "my name")
	}
	if want := selectionCopyConfirmation("my name"); m.fieldCopyNote != want {
		t.Fatalf("fieldCopyNote = %q, want drag-to-copy's own confirmation %q", m.fieldCopyNote, want)
	}
	if view := m.View(); !strings.Contains(view, selectionCopyConfirmation("my name")) {
		t.Fatalf("the create modal does not show the confirmation:\n%s", view)
	}
	if got := m.createEdits[0].Value(); got != "my name" {
		t.Fatalf("alt+w changed the field: %q", got)
	}
	// One keypress later the note is gone.
	updated, _ = m.Update(key("x"))
	if strings.Contains(updated.(Model).View(), "to deck's own tmux buffer") {
		t.Fatal("the confirmation outlived the next keypress")
	}
}

func TestAltWOnAnEmptyFieldCopiesNothingAndSaysSo(t *testing.T) {
	client, selectionBuffer, osc := fieldCopyHarness(t)
	m := createEditOpen(t, createEditShortDir(t)).WithTmuxClient(client)
	updated, _ := m.Update(altW)
	m = updated.(Model)
	if _, err := selectionBuffer(); err == nil {
		t.Fatal("an empty field wrote the selection buffer")
	}
	if osc.Len() != 0 {
		t.Fatalf("an empty field wrote OSC 52: %q", osc.String())
	}
	if !strings.Contains(m.View(), "Nothing copied") {
		t.Fatalf("no on-screen message for an empty field:\n%s", m.View())
	}
}

func TestAltWFailedCopyIsReportedNotConfirmed(t *testing.T) {
	previous := oscClipboardWriter
	oscClipboardWriter = &bytes.Buffer{}
	defer func() { oscClipboardWriter = previous }()
	// A socket with no server behind it: load-buffer fails.
	m := createEditOpen(t, createEditShortDir(t)).WithTmuxClient(tmux.Client{Socket: selectionTestSocket("fieldcopyfail")})
	m = createEditPress(t, m, "a")
	updated, _ := m.Update(altW)
	view := updated.(Model).View()
	if !strings.Contains(view, "Cannot copy field") || strings.Contains(view, "Copied ") {
		t.Fatalf("a failed copy must say so and never confirm:\n%s", view)
	}
}

func envEditorOnSecret(t *testing.T, client tmux.Client) Model {
	t.Helper()
	m := New(nil, config.Settings{}, "").WithTmuxClient(client)
	m.width, m.height = 120, 40
	m.sessions = []store.Session{{
		ID: "s1", Name: "sess", Agent: "claude",
		Env: map[string]string{"AUDIT_ENV_TOKEN": "hunter2-secret"},
	}}
	updated, _ := m.Update(key("e"))
	m = updated.(Model)
	updated, _ = m.Update(key("enter"))
	m = updated.(Model)
	if m.envEditKey != "AUDIT_ENV_TOKEN" {
		t.Fatalf("env editor did not open the secret row for editing: %q", m.envEditKey)
	}
	return m
}

func TestAltWCopiesNothingFromAMaskedSecretAndCopiesItOnceRevealed(t *testing.T) {
	client, selectionBuffer, osc := fieldCopyHarness(t)
	m := envEditorOnSecret(t, client)

	updated, _ := m.Update(altW)
	m = updated.(Model)
	if _, err := selectionBuffer(); err == nil {
		t.Fatal("alt+w on a masked secret wrote the selection buffer")
	}
	if osc.Len() != 0 {
		t.Fatalf("alt+w on a masked secret wrote OSC 52: %q", osc.String())
	}
	view := m.View()
	if !strings.Contains(view, "masked") || strings.Contains(view, "Copied") {
		t.Fatalf("a masked secret must say nothing was copied:\n%s", view)
	}
	if strings.Contains(view, "hunter2-secret") {
		t.Fatal("the note leaked the masked value")
	}

	// Reveal is the dialog's own toggle, which is not available while an edit
	// is open: close the edit, reveal, reopen, copy.
	m.envEditing = true
	m.envEditKey, m.envReveal = "AUDIT_ENV_TOKEN", true
	updated, _ = m.Update(altW)
	m = updated.(Model)
	if out, err := selectionBuffer(); err != nil || out != "hunter2-secret" {
		t.Fatalf("revealed secret: show-buffer = %q, %v; want the secret", out, err)
	}
	if got := oscPayload(t, osc); got != "hunter2-secret" {
		t.Fatalf("OSC 52 payload = %q", got)
	}
	if !strings.Contains(m.View(), selectionCopyConfirmation("hunter2-secret")) {
		t.Fatalf("no confirmation after copying the revealed secret:\n%s", m.View())
	}
}

func TestAltWOnTheOtherTextFields(t *testing.T) {
	client, selectionBuffer, _ := fieldCopyHarness(t)
	// The sidebar filter.
	m := New(nil, config.Settings{}, "").WithTmuxClient(client)
	m.width, m.height = 120, 40
	m.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude"}}
	updated, _ := m.Update(key("/"))
	m = updated.(Model)
	for _, r := range "alp" {
		updated, _ = m.Update(key(string(r)))
		m = updated.(Model)
	}
	updated, _ = m.Update(altW)
	m = updated.(Model)
	if out, err := selectionBuffer(); err != nil || out != "alp" {
		t.Fatalf("filter field: show-buffer = %q, %v; want alp", out, err)
	}
	if !strings.Contains(m.View(), selectionCopyConfirmation("alp")) {
		t.Fatalf("filter field: no confirmation:\n%s", m.View())
	}

	// The rename dialog.
	m = New(nil, config.Settings{}, "").WithTmuxClient(client)
	m.width, m.height = 120, 40
	m.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude"}}
	m.selected = rowCursor(0)
	updated, _ = m.Update(key("i"))
	m = updated.(Model)
	updated, _ = m.Update(key("r"))
	m = updated.(Model)
	if !m.renaming {
		t.Fatal("r did not open rename")
	}
	updated, _ = m.Update(altW)
	m = updated.(Model)
	if out, err := selectionBuffer(); err != nil || out != "alpha" {
		t.Fatalf("rename field: show-buffer = %q, %v; want alpha", out, err)
	}
	if !strings.Contains(m.View(), selectionCopyConfirmation("alpha")) {
		t.Fatalf("rename field: no confirmation:\n%s", m.View())
	}
}
