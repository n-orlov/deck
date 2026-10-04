package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// R186 (SPEC §11.11): every text field's help line names the editor keys and
// alt+w, so a user is never left to guess that the field is a real editor. The
// keys below are the table's own rows, spelled as the help spells them.
var textFieldHelpKeys = []string{
	"Left/Right", "Home/End", "alt+b/alt+f", "ctrl+w", "alt+backspace", "ctrl+u/ctrl+k", "alt+w",
}

// squashFrame is the frame's visible text with ANSI removed and every run of
// whitespace and box-drawing border collapsed to one space, so a help line the
// dialog wrapped across rows is still found by its key names.
func squashFrame(view string) string {
	plain := strings.NewReplacer("|", " ", "│", " ").Replace(stripANSI(view))
	return strings.Join(strings.Fields(plain), " ")
}

func requireTextFieldHelp(t *testing.T, name, view string) {
	t.Helper()
	text := squashFrame(view)
	for _, key := range textFieldHelpKeys {
		if !strings.Contains(text, key) {
			t.Errorf("%s: the help line does not name %q:\n%s", name, key, stripANSI(view))
		}
	}
}

func TestRenameHelpLineNamesTheEditorKeysAndAltW(t *testing.T) {
	requireTextFieldHelp(t, "rename", caretRenameModel(t, "alpha").View())
}

func TestFilterHelpLineNamesTheEditorKeysAndAltW(t *testing.T) {
	requireTextFieldHelp(t, "filter", caretFilterModel().View())
}

func TestLaunchInputsHelpLineNamesTheEditorKeysAndAltW(t *testing.T) {
	m, _ := launchInputsSeededModel(t, "pre", "post")
	if !launchInputsFieldIsText(m.launchInputsField) {
		t.Fatalf("the editor opened on field %d, not a text field", m.launchInputsField)
	}
	requireTextFieldHelp(t, "launch inputs", m.View())
	// On the Login shell selection Left/Right/Space toggle, so the editor line
	// is not claimed there.
	m.launchInputsField = launchInputsFieldLoginShell
	if strings.Contains(squashFrame(m.View()), "alt+w") {
		t.Error("launch inputs: the editor keys are named while the Login shell selection has the focus")
	}
}

func TestEnvEditorHelpLineNamesTheEditorKeysAndAltW(t *testing.T) {
	requireTextFieldHelp(t, "env editor", caretEnvModel().View())
}

func TestCreateHelpLineNamesTheEditorKeysAndAltW(t *testing.T) {
	m := caretCreateModel(t, t.TempDir())
	for _, field := range []int{createFieldName, createFieldCWD, createFieldLaunchArgs, createFieldEnv, createFieldPreLaunch, createFieldPostDestroy} {
		m.createField = field
		requireTextFieldHelp(t, "create field "+createFieldLabels[field], m.View())
	}
	// A selection field cycles with Left/Right/Space, and its footer says so;
	// a text field must not claim that (Left/Right move its caret).
	m.createField = createFieldAgent
	frame := squashFrame(m.View())
	if !strings.Contains(frame, "Left/Right/Space cycles") {
		t.Errorf("create: the selection footer lost its cycle legend:\n%s", frame)
	}
	if strings.Contains(frame, "alt+w") {
		t.Error("create: the editor keys are named while a selection field has the focus")
	}
	m.createField = createFieldName
	if strings.Contains(squashFrame(m.View()), "Left/Right/Space cycles") {
		t.Error("create: a text field's footer still says Left/Right/Space cycles")
	}
}

func TestSettingsTextFieldsHelpLinesNameTheEditorKeysAndAltW(t *testing.T) {
	t.Run("group name", func(t *testing.T) {
		requireTextFieldHelp(t, "settings group rename", caretGroupRenameModel(t, "sprint").View())
	})
	t.Run("env entry", func(t *testing.T) {
		m := settingsOpenOnEnvField(t, config.Settings{Env: map[string]string{"A": "value"}})
		m.width, m.height = 100, 30
		requireTextFieldHelp(t, "settings env", caretPress(m, "enter", "enter").View())
	})
	t.Run("free text value", func(t *testing.T) {
		m := settingsOpenOnStringField(t, config.Settings{File: config.FileConfig{PreLaunch: "echo staged"}})
		m.width, m.height = 100, 30
		m = caretPress(m, "enter")
		if !m.settingsStringEditing {
			t.Fatal("enter did not open the free-text editor")
		}
		requireTextFieldHelp(t, "settings string", m.View())
	})
	t.Run("search", func(t *testing.T) {
		m := settingsOpenOnEnvField(t, config.Settings{})
		m.width, m.height = 100, 30
		m = caretPress(m, "/")
		if !m.settingsSearchActive {
			t.Fatal("/ did not open the settings search")
		}
		requireTextFieldHelp(t, "settings search", m.View())
	})
}

// TestHelpOverlayNamesTheEditorKeysAltWAndWheelForwarding: the `?` overlay
// states SPEC §11.11's whole key table and alt+w, and §11.9's wheel routing:
// forwarding to a mouse-tracking program, Shift+wheel scrolling the grid.
func TestHelpOverlayNamesTheEditorKeysAltWAndWheelForwarding(t *testing.T) {
	text := squashFrame(helpText(false))
	for _, want := range []string{
		"ctrl+b/ctrl+f", "home/end or ctrl+a/ctrl+e", "alt+b/alt+f or ctrl+←/ctrl+→",
		"backspace (ctrl+h) / delete (ctrl+d)", "ctrl+w", "alt+backspace", "ctrl+u / ctrl+k",
		"alt+w", "whole field",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the help overlay does not name %q", want)
		}
	}
	for _, want := range []string{"mouse", "forwarded", "Shift+wheel"} {
		if !strings.Contains(text, want) {
			t.Errorf("the help overlay does not describe wheel forwarding (missing %q)", want)
		}
	}
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 120, 200
	m.help = true
	rendered := squashFrame(m.View())
	for _, want := range []string{"alt+w", "alt+b/alt+f", "Shift+wheel", "forwarded"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the rendered ? overlay does not show %q", want)
		}
	}
}
