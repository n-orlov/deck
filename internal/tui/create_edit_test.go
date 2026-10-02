package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
)

// This file is task 013's (R178, SPEC §11.4/§11.11) contract for the create
// modal's six text fields: name, cwd, launch args, env, pre-launch and
// post-destroy are edited by the shared line editor, so on a focused text
// field left, right and space are the editor's and the dialog contract's Cycle
// is for the selection fields (agent, profile, login shell, group) only. The
// tests drive Update and the rendered body; the real-tmux twin is
// features/create_text_editing.feature.

// createEditShortDir is a scratch directory with a short path: a cwd longer
// than its field scrolls and marks the clipped side, which these tests read
// the drawn text around.
func createEditShortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "c")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// createEditOpen opens the create modal the way a user does, with `n`.
func createEditOpen(t *testing.T, startCWD string) Model {
	t.Helper()
	if startCWD != "" {
		t.Chdir(startCWD)
	}
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 120, 40
	updated, _ := m.Update(key("n"))
	m = updated.(Model)
	if !m.creating {
		t.Fatal("n did not open the create modal")
	}
	return m
}

func createEditPress(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "space":
			msg = tea.KeyMsg(tea.Key{Type: tea.KeySpace, Runes: []rune{' '}})
		case "home", "end", "delete":
			msg = tea.KeyMsg(tea.Key{Type: map[string]tea.KeyType{"home": tea.KeyHome, "end": tea.KeyEnd, "delete": tea.KeyDelete}[k]})
		case "ctrl+k":
			msg = tea.KeyMsg(tea.Key{Type: tea.KeyCtrlK})
		default:
			msg = key(k)
		}
		updated, _ := m.Update(msg)
		m = updated.(Model)
	}
	return m
}

// createEditType types text one rune at a time, as a terminal delivers it.
func createEditType(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		m = createEditPress(t, m, string(r))
	}
	return m
}

// createEditRow is the drawn value of the row labelled label, with the caret's
// reverse-video cell and every other escape removed so the text reads plain.
func createEditRow(t *testing.T, m Model, label string) string {
	t.Helper()
	for _, line := range strings.Split(stripANSI(m.createBody()), "\n") {
		if i := strings.Index(line, label+": "); i >= 0 {
			return strings.TrimRight(line[i+len(label)+2:], " ")
		}
	}
	t.Fatalf("no %q row in the create modal:\n%s", label, m.createBody())
	return ""
}

func TestCreateModalNameLeftRightMoveTheCaretAndLeaveTheAgentAlone(t *testing.T) {
	m := createEditOpen(t, t.TempDir())
	if len(m.createAvailableAgentKinds) < 2 {
		t.Skipf("need two selectable agents to prove the selection is unchanged, have %v", m.createAvailableAgentKinds)
	}
	agent := m.createAgent
	m = createEditType(t, m, "abc")
	m = createEditPress(t, m, "left", "left")
	m = createEditType(t, m, "X")
	if got := createEditRow(t, m, "Name"); got != "aXbc" {
		t.Fatalf("typing after two lefts gave the name %q, want %q", got, "aXbc")
	}
	m = createEditPress(t, m, "right", "right", "right")
	if m.createAgent != agent || m.createField != 0 {
		t.Fatalf("left/right on the name field changed the agent to %q (was %q) or the field to %d", m.createAgent, agent, m.createField)
	}
	m = createEditType(t, m, "Y")
	if got := createEditRow(t, m, "Name"); got != "aXbcY" {
		t.Fatalf("right moved the caret to the end, then typing gave %q, want %q", got, "aXbcY")
	}
}

func TestCreateModalSpaceTypesASpaceOnATextFieldAndCyclesASelection(t *testing.T) {
	m := createEditOpen(t, t.TempDir())
	m = createEditType(t, m, "a")
	m = createEditPress(t, m, "space")
	m = createEditType(t, m, "b")
	if got := createEditRow(t, m, "Name"); got != "a b" {
		t.Fatalf("space on the name field gave %q, want %q", got, "a b")
	}
	// Selection fields still cycle: the login shell toggles on space and on
	// right, and the group field is a selection too.
	m.createField = createFieldLoginShell
	m = createEditPress(t, m, "space")
	if !m.createLoginShell {
		t.Fatal("space on the login shell selection did not toggle it")
	}
	m = createEditPress(t, m, "right")
	if m.createLoginShell {
		t.Fatal("right on the login shell selection did not toggle it back")
	}
	if m.createField != createFieldLoginShell {
		t.Fatalf("cycling moved the focus to field %d", m.createField)
	}
}

func TestCreateModalEveryTextFieldEditsInTheMiddle(t *testing.T) {
	for _, tc := range []struct {
		field int
		label string
	}{
		{createFieldName, "Name"},
		{createFieldLaunchArgs, "Launch args (JSON array)"},
		{createFieldEnv, "Env (key=value, comma-separated)"},
		{createFieldPreLaunch, "Pre-launch command"},
		{createFieldPostDestroy, "Post-destroy command"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			m := createEditOpen(t, t.TempDir())
			m.createField = tc.field
			m = createEditType(t, m, "abcd")
			m = createEditPress(t, m, "left", "left", "backspace")
			m = createEditType(t, m, "XY")
			if got := createEditRow(t, m, tc.label); got != "aXYcd" {
				t.Fatalf("backspace and typing in the middle gave %q, want %q", got, "aXYcd")
			}
			m = createEditPress(t, m, "home", "delete")
			if got := createEditRow(t, m, tc.label); got != "XYcd" {
				t.Fatalf("home then delete gave %q, want %q", got, "XYcd")
			}
			m = createEditPress(t, m, "ctrl+k")
			if got := createEditRow(t, m, tc.label); got != "" {
				t.Fatalf("ctrl+k at the start gave %q, want an empty field", got)
			}
		})
	}
}

func TestCreateModalCWDPrefillIsOfferedNotCommitted(t *testing.T) {
	dir := createEditShortDir(t)
	// A printable first keystroke replaces the offered value wholesale.
	m := createEditOpen(t, dir)
	m.createField = createFieldCWD
	if got := m.createText(createFieldCWD); got != dir {
		t.Fatalf("the cwd opened on %q, want the start directory %q", got, dir)
	}
	m = createEditType(t, m, "/x")
	if got := m.createText(createFieldCWD); got != "/x" {
		t.Fatalf("typing over the offered cwd gave %q, want %q", got, "/x")
	}
	// A caret key accepts it and edits it in place.
	m = createEditOpen(t, dir)
	m.createField = createFieldCWD
	m = createEditPress(t, m, "left")
	m = createEditType(t, m, "Z")
	want := dir[:len(dir)-1] + "Z" + dir[len(dir)-1:]
	if got := m.createText(createFieldCWD); got != want {
		t.Fatalf("left then typing in the offered cwd gave %q, want %q", got, want)
	}
	// So does backspace: it deletes the last character, not the whole value.
	m = createEditOpen(t, dir)
	m.createField = createFieldCWD
	m = createEditPress(t, m, "backspace")
	if got := m.createText(createFieldCWD); got != dir[:len(dir)-1] {
		t.Fatalf("backspace on the offered cwd gave %q, want %q", got, dir[:len(dir)-1])
	}
}

func TestCreateModalCWDGhostAndTabCompleteOnlyWithTheCaretAtTheEnd(t *testing.T) {
	parent := createEditShortDir(t)
	if err := os.Mkdir(filepath.Join(parent, "unique-directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := createEditOpen(t, t.TempDir())
	m.createField = createFieldCWD
	m = createEditType(t, m, filepath.Join(parent, "uni"))
	if got := createEditRow(t, m, "Working directory"); !strings.HasSuffix(got, "uni"+"que-directory/") {
		t.Fatalf("with the caret at the end the field shows %q, want the ghost completion", got)
	}

	// Caret away from the end: no ghost, tab does nothing, right is a caret key.
	away := createEditPress(t, m, "left")
	if got := createEditRow(t, away, "Working directory"); strings.Contains(got, "que-directory") {
		t.Fatalf("with the caret in the middle the field shows a ghost: %q", got)
	}
	tabbed := createEditPress(t, away, "tab")
	if got := createEditRow(t, tabbed, "Working directory"); got != filepath.Join(parent, "uni") || tabbed.createField != createFieldCWD {
		t.Fatalf("tab with the caret in the middle changed the field to %q (focus %d)", got, tabbed.createField)
	}
	// Right back to the end is a caret move, not an acceptance.
	back := createEditPress(t, away, "right")
	if got := createEditRow(t, back, "Working directory"); !strings.HasSuffix(got, "unique-directory/") {
		t.Fatalf("right back at the end shows %q, want the ghost again", got)
	}
	if strings.Count(stripANSI(back.createBody()), "unique-directory/") != 1 || filepath.Base(back.createText(createFieldCWD)) != "uni" {
		t.Fatalf("right that only moved the caret accepted the ghost: value %q", back.createText(createFieldCWD))
	}

	// Right at the end, with a ghost showing, accepts it.
	accepted := createEditPress(t, back, "right")
	if got := accepted.createText(createFieldCWD); got != filepath.Join(parent, "unique-directory")+"/" {
		t.Fatalf("right at the end with a ghost gave %q, want the completed directory", got)
	}
}

func TestCreateModalHasNoByteTrimmingBackspaceOrAppendCase(t *testing.T) {
	data, err := os.ReadFile("tui.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	start := strings.Index(src, "func (m Model) updateCreate(")
	end := strings.Index(src, "func (m *Model) submitCreate(")
	if start < 0 || end < start {
		t.Fatal("updateCreate not found in tui.go")
	}
	body := src[start:end]
	for _, banned := range []string{`case "backspace"`, "+= string(", "backspaceCreateField"} {
		if strings.Contains(body, banned) {
			t.Errorf("updateCreate still carries its own editing code: %q", banned)
		}
	}
	if strings.Contains(src, "func (m *Model) backspaceCreateField") {
		t.Error("backspaceCreateField still exists")
	}
}
