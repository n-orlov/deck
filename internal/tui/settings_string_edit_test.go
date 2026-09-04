package tui

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/n-orlov/deck/internal/config"
)

// This file covers the operator-reported defect that the settings
// takeover's two free-text keys -- pre_launch and post_destroy, the only
// config.KindString fields config.Schema declares -- rendered but could not
// be edited: settingsActivateField switched on Kind and handled toggle,
// [env] and link only, so enter/space on either row was a silent no-op and
// settingsSetString (which was correct, and had been since task 004) was
// reachable from tests alone. SPEC.md:532 already requires the opposite:
// "Both keys are editable in settings (§11.5) like every other flat key,
// and labelled restart-to-apply there, because a hook is consumed at
// launch." Every test below drives the real keymap through Model.Update
// (never updateSettingsStringEditing directly), because half the defect was
// about which handler a keystroke reaches at all -- a test that called the
// editor's own handler would have passed against the pre-fix tree.

// settingsFocusFieldByKey moves an already-open takeover's field-list
// selection onto the schema field with the given FullKey, by walking the
// live settingsCategories() rather than hard-coding a category/field index
// pair a schema reorder would silently invalidate (settingsOpenOnEnvField
// in settings_env_entries_editor_test.go does the same for [env]).
func settingsFocusFieldByKey(t *testing.T, m Model, fullKey string) Model {
	t.Helper()
	catIdx, fieldIdx := -1, -1
	for ci, cat := range settingsCategories() {
		for fi, f := range cat.Fields {
			if f.FullKey() == fullKey {
				catIdx, fieldIdx = ci, fi
			}
		}
	}
	if catIdx < 0 {
		t.Fatalf("no %s field found in settingsCategories()", fullKey)
	}
	m.settingsCategoryIndex = catIdx
	m.settingsFieldIndex = fieldIdx
	m.settingsFocus = settingsFocusFields
	return m
}

// settingsOpenOnStringField opens the takeover with `,` (the real key, so
// the open path's own state reset participates in the test) and selects the
// named free-text field. cfg.File is what settingsEditsFromSettings seeds
// the staged copy from (requirement 21), so a caller wanting a pre-existing
// value sets it there, exactly as a real config.toml load would.
func settingsOpenOnStringField(t *testing.T, cfg config.Settings, fullKey string) Model {
	t.Helper()
	model := New(nil, cfg, "")
	updated, _ := model.Update(key(","))
	m := updated.(Model)
	if !m.settingsOpen {
		t.Fatal(", did not open settings")
	}
	return settingsFocusFieldByKey(t, m, fullKey)
}

// TestSettingsStringEnterOpensEditorPrefilledWithStagedValue is the
// defect's own first symptom: enter on the pre_launch row did nothing at
// all. It must now open the free-text editor prefilled from the STAGED
// value (m.settingsEdits, via settingsStringValue) with nothing staged yet
// -- opening an editor is not an edit.
func TestSettingsStringEnterOpensEditorPrefilledWithStagedValue(t *testing.T) {
	model := settingsOpenOnStringField(t, config.Settings{File: config.FileConfig{PreLaunch: "echo staged"}}, "pre_launch")

	updated, _ := model.Update(key("enter"))
	m := updated.(Model)
	if !m.settingsStringEditing {
		t.Fatal("enter on the pre_launch row did not open the free-text editor (the reported defect: KindString fell through settingsActivateField)")
	}
	if m.settingsStringEditKey != "pre_launch" {
		t.Fatalf("settingsStringEditKey = %q, want %q", m.settingsStringEditKey, "pre_launch")
	}
	if m.settingsStringEditValue != "echo staged" {
		t.Fatalf("editor opened with %q, want the staged value %q", m.settingsStringEditValue, "echo staged")
	}
	if m.settingsDirty() {
		t.Fatal("merely opening the editor made the takeover dirty; only a commit may")
	}

	// space is §11.5's second activation key and must open the same editor
	// -- the pre-fix tree ignored it for exactly the same reason.
	updated, _ = model.Update(key(" "))
	if !updated.(Model).settingsStringEditing {
		t.Fatal("space on the pre_launch row did not open the free-text editor")
	}
}

// TestSettingsStringTypingAndEnterStagesTheValue proves the whole point of
// the fix: a typed command reaches m.settingsEdits through settingsSetString
// on enter, which makes settingsDirty() true on its own (it compares
// settingsEdits against settingsSavedEdits) without this editor knowing
// anything about the save flow. The typed text deliberately contains a
// space, so it also proves a lone space keystroke is text here rather than
// §11.5's activate key.
func TestSettingsStringTypingAndEnterStagesTheValue(t *testing.T) {
	model := settingsOpenOnStringField(t, config.Settings{}, "pre_launch")

	updated, _ := model.Update(key("enter"))
	m := updated.(Model)
	updated, _ = m.Update(key("direnv export bash"))
	m = updated.(Model)
	if m.settingsStringEditValue != "direnv export bash" {
		t.Fatalf("typed text = %q, want %q", m.settingsStringEditValue, "direnv export bash")
	}
	if m.settingsEdits.PreLaunch != "" {
		t.Fatalf("typing staged %q before enter; typing must only fill the buffer", m.settingsEdits.PreLaunch)
	}

	updated, _ = m.Update(key("enter"))
	m = updated.(Model)
	if m.settingsStringEditing {
		t.Fatal("enter did not leave the editor")
	}
	if m.settingsEdits.PreLaunch != "direnv export bash" {
		t.Fatalf("settingsEdits.PreLaunch = %q, want %q", m.settingsEdits.PreLaunch, "direnv export bash")
	}
	if m.settingsSavedEdits.PreLaunch != "" {
		t.Fatalf("committing staged into settingsSavedEdits (%q); only a successful ctrl+s may advance it", m.settingsSavedEdits.PreLaunch)
	}
	if !m.settingsDirty() {
		t.Fatal("a committed edit left settingsDirty() false, so esc would close without its discard-confirm")
	}
}

// TestSettingsStringEditingSwallowsNavigationRunes is the requirement the
// editing mode exists to satisfy: it takes over the whole keymap the way
// settingsEnvEditing does, so a rune that is otherwise a binding lands in
// the text. j/k are up/down, - is settingsAdjustField(-1) and / opens the
// field search; every one of them is a legitimate character in a shell
// command (`kill -9`, a path, a flag), and every one of them silently
// moved the selection or opened search in a mode that did not take the
// keymap over.
func TestSettingsStringEditingSwallowsNavigationRunes(t *testing.T) {
	model := settingsOpenOnStringField(t, config.Settings{}, "pre_launch")

	updated, _ := model.Update(key("enter"))
	m := updated.(Model)
	catBefore, fieldBefore := m.settingsCategoryIndex, m.settingsFieldIndex

	for _, k := range []string{"j", "k", "-", "/", "+", "="} {
		updated, _ = m.Update(key(k))
		m = updated.(Model)
	}

	if got, want := m.settingsStringEditValue, "jk-/+="; got != want {
		t.Fatalf("typed keybinding runes produced %q, want %q", got, want)
	}
	if m.settingsCategoryIndex != catBefore || m.settingsFieldIndex != fieldBefore {
		t.Fatalf("typing moved the selection: category %d->%d, field %d->%d", catBefore, m.settingsCategoryIndex, fieldBefore, m.settingsFieldIndex)
	}
	if m.settingsSearchActive {
		t.Fatal("typing / opened the field search instead of landing in the text")
	}
	if !m.settingsStringEditing {
		t.Fatal("typing left the editor")
	}
	if m.settingsDirty() {
		t.Fatal("typing staged something before enter")
	}
}

// TestSettingsStringEscCancelsLeavingStagedValueUntouched proves esc is a
// real cancel and not a commit: the buffer is dropped and the staged value
// is exactly what it was, so the takeover is not dirty and esc-on-a-clean-
// takeover still closes without the discard prompt.
func TestSettingsStringEscCancelsLeavingStagedValueUntouched(t *testing.T) {
	model := settingsOpenOnStringField(t, config.Settings{File: config.FileConfig{PreLaunch: "keep me"}}, "pre_launch")

	updated, _ := model.Update(key("enter"))
	m := updated.(Model)
	updated, _ = m.Update(key("throw away"))
	m = updated.(Model)
	updated, _ = m.Update(key("esc"))
	m = updated.(Model)

	if m.settingsStringEditing {
		t.Fatal("esc did not leave the editor")
	}
	if m.settingsEdits.PreLaunch != "keep me" {
		t.Fatalf("esc changed the staged value to %q, want it left at %q", m.settingsEdits.PreLaunch, "keep me")
	}
	if m.settingsDirty() {
		t.Fatal("a cancelled edit left the takeover dirty")
	}
	if !m.settingsOpen {
		t.Fatal("esc while editing closed the whole takeover; it must only leave the editor")
	}
}

// TestSettingsStringBackspaceDeletesOneRune pins backspace on a multi-byte
// value: it must trim exactly one RUNE, not one byte. A byte-sliced
// implementation would leave a truncated UTF-8 sequence, which
// config.WriteConfigFile would then quote into config.toml as invalid
// bytes -- hence the utf8.ValidString assertion alongside the equality one.
func TestSettingsStringBackspaceDeletesOneRune(t *testing.T) {
	model := settingsOpenOnStringField(t, config.Settings{File: config.FileConfig{PreLaunch: "echo héé"}}, "pre_launch")

	updated, _ := model.Update(key("enter"))
	m := updated.(Model)
	updated, _ = m.Update(key("backspace"))
	m = updated.(Model)

	if want := "echo hé"; m.settingsStringEditValue != want {
		t.Fatalf("backspace produced %q, want %q (one rune trimmed, not one byte)", m.settingsStringEditValue, want)
	}
	if !utf8.ValidString(m.settingsStringEditValue) {
		t.Fatalf("backspace left invalid UTF-8: %q", m.settingsStringEditValue)
	}
}

// TestSettingsStringCommittingEmptyClearsTheKey covers the one place this
// editor must NOT copy settingsEnvCommitEdit: an empty [env] KEY names no
// variable and so can only be an abandoned entry, but an empty hook command
// is a legitimate value -- schema.go's own default, meaning "no hook runs".
// Backspacing a configured hook away and pressing enter must therefore
// clear the key (and make the takeover dirty), never be swallowed as a
// second kind of cancel.
func TestSettingsStringCommittingEmptyClearsTheKey(t *testing.T) {
	model := settingsOpenOnStringField(t, config.Settings{File: config.FileConfig{PreLaunch: "abc"}}, "pre_launch")

	updated, _ := model.Update(key("enter"))
	m := updated.(Model)
	for i := 0; i < 3; i++ {
		updated, _ = m.Update(key("backspace"))
		m = updated.(Model)
	}
	if m.settingsStringEditValue != "" {
		t.Fatalf("buffer = %q after three backspaces, want empty", m.settingsStringEditValue)
	}
	updated, _ = m.Update(key("enter"))
	m = updated.(Model)

	if m.settingsEdits.PreLaunch != "" {
		t.Fatalf("committing an empty value left PreLaunch = %q; empty means \"no hook\" and must clear the key", m.settingsEdits.PreLaunch)
	}
	if !m.settingsDirty() {
		t.Fatal("clearing a configured hook left settingsDirty() false, so ctrl+s/esc would treat it as no change at all")
	}
}

// TestSettingsStringPostDestroyRoundTripsThroughCtrlS is the end-to-end
// claim SPEC.md:532 makes: a value typed into the takeover reaches
// config.toml and reads back through the real loader as
// config.FileConfig.PostDestroy. It also pins this fix's ctrl+s decision --
// ctrl+s while the editor is still open is SWALLOWED, never a save of
// half-typed text behind the user's back: enter is the commit gesture, and
// the footer says so.
func TestSettingsStringPostDestroyRoundTripsThroughCtrlS(t *testing.T) {
	model, configFile, reload := settingsTestModel(t)
	model = settingsFocusFieldByKey(t, model, "post_destroy")

	updated, _ := model.Update(key("enter"))
	m := updated.(Model)
	if !m.settingsStringEditing {
		t.Fatal("enter on the post_destroy row did not open the free-text editor")
	}
	updated, _ = m.Update(key("rm -rf /tmp/deck-scratch"))
	m = updated.(Model)

	// ctrl+s mid-edit: nothing may be staged and nothing may be written.
	updated, _ = m.Update(key("ctrl+s"))
	m = updated.(Model)
	if !m.settingsStringEditing {
		t.Fatal("ctrl+s left the editor; it must be swallowed so a half-typed command is never saved")
	}
	if m.settingsEdits.PostDestroy != "" {
		t.Fatalf("ctrl+s mid-edit staged %q", m.settingsEdits.PostDestroy)
	}
	if _, err := os.Stat(configFile); err == nil {
		t.Fatal("ctrl+s mid-edit wrote config.toml behind the user's back")
	}
	if m.settingsNote != "" {
		t.Fatalf("ctrl+s mid-edit left note %q, implying it did something", m.settingsNote)
	}

	updated, _ = m.Update(key("enter"))
	m = updated.(Model)
	updated, _ = m.Update(key("ctrl+s"))
	m = updated.(Model)

	if m.settingsDirty() {
		t.Fatal("a successful save left settingsDirty() true")
	}
	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("config file was not written: %v", err)
	}
	if !strings.Contains(string(data), "post_destroy") {
		t.Fatalf("saved config.toml does not mention post_destroy:\n%s", data)
	}
	parsed, err := reload()
	if err != nil {
		t.Fatalf("saved config.toml did not parse: %v", err)
	}
	if want := "rm -rf /tmp/deck-scratch"; parsed.File.PostDestroy != want {
		t.Fatalf("reloaded File.PostDestroy = %q, want %q", parsed.File.PostDestroy, want)
	}
}

// TestSettingsStringEmptyValueRendersNotSetPlaceholder covers the second
// half of the reported defect: an unset hook row rendered "Pre Launch: "
// with nothing after the colon, which reads as a broken row rather than an
// unset one. It now borrows KindEnum's own "(default)" idiom as
// "(not set)", and a configured value replaces it verbatim.
func TestSettingsStringEmptyValueRendersNotSetPlaceholder(t *testing.T) {
	model := settingsOpenOnStringField(t, config.Settings{}, "pre_launch")
	model.width, model.height = 120, 40

	label := settingsFieldLabel(config.Field{Key: "pre_launch"})
	row := settingsRowContaining(t, model.settingsView(), label+": ")
	if !strings.Contains(row, "(not set)") {
		t.Fatalf("unset pre_launch row = %q, want it to render the (not set) placeholder", row)
	}

	m := model
	m.settingsEdits.PreLaunch = "echo hi"
	row = settingsRowContaining(t, m.settingsView(), label+": ")
	if !strings.Contains(row, "echo hi") || strings.Contains(row, "(not set)") {
		t.Fatalf("configured pre_launch row = %q, want the value itself and no placeholder", row)
	}
}

// TestSettingsStringEditorRendersTypedTextCursorAndKeys proves the editing
// mode is visible rather than a hidden state: the panel names the field
// being edited, shows the in-progress text with the trailing "_" cursor
// this package already uses for every free-text field (env_editor.go,
// filter.go), and the footer names each key the mode binds -- the same
// "never binds a key it does not name" rule settingsFooterLine's own
// comment states for the takeover as a whole.
func TestSettingsStringEditorRendersTypedTextCursorAndKeys(t *testing.T) {
	model := settingsOpenOnStringField(t, config.Settings{}, "pre_launch")
	model.width, model.height = 120, 40

	updated, _ := model.Update(key("enter"))
	m := updated.(Model)
	updated, _ = m.Update(key("echo hi"))
	m = updated.(Model)

	view := m.settingsView()
	if !strings.Contains(view, "Edit "+settingsFieldLabel(config.Field{Key: "pre_launch"})) {
		t.Fatalf("editing view does not name the field being edited:\n%s", view)
	}
	if !strings.Contains(view, "echo hi"+settingsTextCursor) {
		t.Fatalf("editing view does not show the typed text with its cursor:\n%s", view)
	}
	footer := m.settingsFooterLine()
	for _, k := range []string{"enter", "esc", "ctrl+s"} {
		if !strings.Contains(footer, k) {
			t.Errorf("editing footer %q does not name %q", footer, k)
		}
	}
}

// TestSettingsWrapVerbatimKeepsLongValuesInsideThePanel is the layout half
// of requirement 4: a value far longer than the panel is wrapped, never
// allowed to widen a row, and -- unlike wrapText, whose strings.Fields pass
// collapses whitespace runs -- reassembles to exactly the text that will be
// staged, so what is on screen is what enter commits.
func TestSettingsWrapVerbatimKeepsLongValuesInsideThePanel(t *testing.T) {
	value := "echo a" + strings.Repeat("b", 300) + "  two spaces  and\ttab"
	rows := settingsWrapVerbatim(value, 20)
	if strings.Join(rows, "") != value {
		t.Fatalf("settingsWrapVerbatim altered the text it wrapped:\n got %q\nwant %q", strings.Join(rows, ""), value)
	}
	for i, row := range rows {
		if w := stringWidth(row); w > 20 {
			t.Errorf("row %d is %d columns wide, exceeding the 20-column budget: %q", i, w, row)
		}
	}

	// The whole editing frame stays inside deck's supported minimum too,
	// mirroring TestSettingsViewFitsFrameBudget's own two checks.
	model := settingsOpenOnStringField(t, config.Settings{}, "pre_launch")
	model.width, model.height = 80, 24
	updated, _ := model.Update(key("enter"))
	m := updated.(Model)
	updated, _ = m.Update(key(value))
	m = updated.(Model)
	lines := strings.Split(m.settingsView(), "\n")
	if len(lines) > 24 {
		t.Errorf("editing view has %d lines, exceeding the 24-row budget", len(lines))
	}
	for i, line := range lines {
		if w := stringWidth(line); w > 80 {
			t.Errorf("editing view line %d is %d columns wide, exceeding the 80-column budget:\n%s", i, w, line)
		}
	}
}

// settingsRowContaining returns the single rendered line of view containing
// needle, failing if there is none (a row assertion that silently matched
// nothing would pass for the wrong reason) or more than one (which would
// make "the row" ambiguous).
func settingsRowContaining(t *testing.T, view, needle string) string {
	t.Helper()
	var found []string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, needle) {
			found = append(found, line)
		}
	}
	if len(found) != 1 {
		t.Fatalf("found %d rows containing %q, want exactly 1:\n%s", len(found), needle, view)
	}
	return found[0]
}
