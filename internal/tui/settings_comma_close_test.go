package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// R199/#63: "," closes the Settings takeover through the identical handler
// esc uses, is a literal character in every text-editing mode, and is
// ignored while either confirm is up.

// TestSettingsCommaClosesWhenNothingIsDirty: "," with nothing staged closes
// the takeover with no prompt, exactly as esc does.
func TestSettingsCommaClosesWhenNothingIsDirty(t *testing.T) {
	model, _, _ := settingsTestModel(t)
	model = settingsPress(model, ",")
	if model.settingsOpen {
		t.Fatal(", with nothing dirty did not close the takeover")
	}
	if model.settingsDiscardConfirm {
		t.Fatal(", with nothing dirty raised the discard prompt")
	}
}

// TestSettingsCommaPromptsToDiscardWhenDirty: "," with a staged change raises
// the discard prompt and leaves the takeover open, exactly as esc does.
func TestSettingsCommaPromptsToDiscardWhenDirty(t *testing.T) {
	model, _, _ := settingsTestModel(t)
	model = settingsPress(model, "enter") // toggles allow_yolo
	if !model.settingsDirty() {
		t.Fatal("enter did not stage a change")
	}
	model = settingsPress(model, ",")
	if !model.settingsOpen || !model.settingsDiscardConfirm {
		t.Fatalf(", with a dirty edit: open=%v discardConfirm=%v, want the takeover open with the discard prompt up", model.settingsOpen, model.settingsDiscardConfirm)
	}
	model = settingsPress(model, "y")
	if model.settingsOpen {
		t.Fatal("confirming the discard prompt raised by , did not close the takeover")
	}
}

// TestSettingsCommaAndEscShareOneHandler pins "the same code path": the two
// keys reach the same state from the same starting point, dirty or clean.
func TestSettingsCommaAndEscShareOneHandler(t *testing.T) {
	for _, dirty := range []bool{false, true} {
		base, _, _ := settingsTestModel(t)
		if dirty {
			base = settingsPress(base, "enter")
		}
		viaEsc := settingsPress(base, "esc")
		viaComma := settingsPress(base, ",")
		if viaEsc.settingsOpen != viaComma.settingsOpen || viaEsc.settingsDiscardConfirm != viaComma.settingsDiscardConfirm {
			t.Fatalf("dirty=%v: esc -> open=%v confirm=%v, , -> open=%v confirm=%v", dirty,
				viaEsc.settingsOpen, viaEsc.settingsDiscardConfirm, viaComma.settingsOpen, viaComma.settingsDiscardConfirm)
		}
	}
}

// TestSettingsCommaIsLiteralInStringAndPathEntry: in the free-text editor
// (every KindString/KindPath field) "," is typed, and the takeover stays open
// in that editor.
func TestSettingsCommaIsLiteralInStringAndPathEntry(t *testing.T) {
	n := 0
	for _, cat := range settingsCategories() {
		for _, f := range cat.Fields {
			if f.Kind != config.KindString && f.Kind != config.KindPath {
				continue
			}
			n++
			m := settingsOpenOnStringField(t, config.Settings{})
			m = settingsFocusFieldByKey(t, m, f.FullKey())
			m = settingsPress(m, "enter", "a", ",", "b")
			if !m.settingsOpen || !m.settingsStringEditing {
				t.Fatalf("%s: , left the editor: open=%v editing=%v", f.FullKey(), m.settingsOpen, m.settingsStringEditing)
			}
			if got := m.settingsStringEdit.Value(); got != "a,b" {
				t.Fatalf("%s: typed value %q, want %q", f.FullKey(), got, "a,b")
			}
		}
	}
	if n == 0 {
		t.Fatal("no string/path field found in the schema")
	}
}

// TestSettingsCommaIsLiteralInEnvKey: the [env] entry editor's key field.
func TestSettingsCommaIsLiteralInEnvKey(t *testing.T) {
	m := settingsOpenOnEnvField(t, config.Settings{Env: map[string]string{}})
	m = settingsPress(m, "enter", "enter", "A", ",", "B")
	if !m.settingsOpen || !m.settingsEnvEditing || !m.settingsEnvEditingKeyPart {
		t.Fatalf(", left the env key field: open=%v editing=%v keyPart=%v", m.settingsOpen, m.settingsEnvEditing, m.settingsEnvEditingKeyPart)
	}
	if got := m.settingsEnvKeyEdit.Value(); got != "A,B" {
		t.Fatalf("env key = %q, want %q", got, "A,B")
	}
}

// TestSettingsCommaIsLiteralInEnvValue: the [env] entry editor's value field.
func TestSettingsCommaIsLiteralInEnvValue(t *testing.T) {
	m := settingsOpenOnEnvField(t, config.Settings{Env: map[string]string{"A": "old"}})
	m = settingsPress(m, "enter", "enter", "x", ",", "y")
	if !m.settingsOpen || !m.settingsEnvEditing || m.settingsEnvEditingKeyPart {
		t.Fatalf(", left the env value field: open=%v editing=%v keyPart=%v", m.settingsOpen, m.settingsEnvEditing, m.settingsEnvEditingKeyPart)
	}
	if got := m.settingsEnvValueEdit.Value(); got != "x,y" {
		t.Fatalf("env value = %q, want %q", got, "x,y")
	}
}

// TestSettingsCommaIsLiteralInGroupNameEntry: the group create prompt and the
// group rename prompt (both the group-name editor).
func TestSettingsCommaIsLiteralInGroupNameEntry(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	m := settingsOpenOnGroupsFields(t, db)
	m = settingsPress(m, "n", "a", ",", "b")
	if !m.settingsOpen || !m.settingsGroupCreating {
		t.Fatalf(", left the group create prompt: open=%v creating=%v", m.settingsOpen, m.settingsGroupCreating)
	}
	if got := m.settingsGroupEdit.Value(); got != "a,b" {
		t.Fatalf("new group name = %q, want %q", got, "a,b")
	}

	r, _ := settingsRenameModel(t, "sprint")
	r = settingsPress(r, "x", ",", "y")
	if !r.settingsOpen || !r.settingsGroupRenaming {
		t.Fatalf(", left the group rename prompt: open=%v renaming=%v", r.settingsOpen, r.settingsGroupRenaming)
	}
	if got := r.settingsGroupEdit.Value(); got != "x,y" {
		t.Fatalf("renamed group name = %q, want %q", got, "x,y")
	}
}

// TestSettingsCommaIsLiteralInSearch: the "/" search query.
func TestSettingsCommaIsLiteralInSearch(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.settingsOpen = true
	m = settingsPress(m, "/", "a", ",", "b")
	if !m.settingsOpen || !m.settingsSearchActive {
		t.Fatalf(", left the search: open=%v active=%v", m.settingsOpen, m.settingsSearchActive)
	}
	if got := m.settingsSearchEdit.Value(); got != "a,b" {
		t.Fatalf("search query = %q, want %q", got, "a,b")
	}
}

// TestSettingsCommaIsIgnoredUnderTheDiscardPrompt: "," neither closes nor
// dismisses the prompt, and leaves the staged change alone.
func TestSettingsCommaIsIgnoredUnderTheDiscardPrompt(t *testing.T) {
	m, _, _ := settingsTestModel(t)
	m = settingsPress(m, "enter", "esc")
	if !m.settingsDiscardConfirm {
		t.Fatal("esc on a dirty takeover did not raise the discard prompt")
	}
	m = settingsPress(m, ",")
	if !m.settingsOpen || !m.settingsDiscardConfirm || !m.settingsDirty() {
		t.Fatalf(", under the discard prompt: open=%v confirm=%v dirty=%v, want all true", m.settingsOpen, m.settingsDiscardConfirm, m.settingsDirty())
	}
	if !strings.Contains(m.settingsFooterLineContent(), "discard unsaved changes") {
		t.Fatal("the discard prompt is no longer showing")
	}
}

// TestSettingsCommaIsIgnoredUnderTheGroupDeleteConfirm: "," leaves the
// non-empty group's delete confirm up and the takeover open.
func TestSettingsCommaIsIgnoredUnderTheGroupDeleteConfirm(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	g, err := db.CreateGroup(t.Context(), "tidy-up")
	if err != nil {
		t.Fatal(err)
	}
	mustCreateSessionInGroup(t, db, "s1", "alpha", g.ID, "")
	m := settingsOpenOnGroupsFields(t, db)
	m = settingsPress(m, "d")
	if !m.settingsGroupDeleteConfirming {
		t.Fatal("d on a non-empty group did not open the delete confirm")
	}
	m = settingsPress(m, ",")
	if !m.settingsOpen || !m.settingsGroupDeleteConfirming {
		t.Fatalf(", under the group-delete confirm: open=%v confirming=%v, want both true", m.settingsOpen, m.settingsGroupDeleteConfirming)
	}
}

// TestSettingsFooterNamesCommaBesideEsc: the plain and Groups footers both
// advertise "," next to esc, and the discard prompt says "," does not answer it.
func TestSettingsFooterNamesCommaBesideEsc(t *testing.T) {
	m, _, _ := settingsTestModel(t)
	m.width, m.height = 200, 30
	if got := m.settingsFooterLineContent(); !strings.Contains(got, "esc/, close") {
		t.Fatalf("plain footer %q does not name , beside esc", got)
	}
	db := openStoreForLastCreateGroup(t)
	g := settingsOpenOnGroupsFields(t, db)
	g.width, g.height = 200, 30
	if got := g.settingsFooterLineContent(); !strings.Contains(got, "esc/, close") {
		t.Fatalf("groups footer %q does not name , beside esc", got)
	}
}
