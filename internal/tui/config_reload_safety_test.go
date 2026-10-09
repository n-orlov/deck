package tui

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

func (h *reloadHarness) key(msg tea.KeyMsg) {
	h.t.Helper()
	next, _ := h.m.Update(msg) // a settings key answers with at most a terminal command
	h.m = next.(Model)
}

func (h *reloadHarness) openSettings() {
	h.t.Helper()
	h.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{','}})
	if !h.m.settingsOpen {
		h.t.Fatal("the settings takeover did not open")
	}
}

// pollOnce delivers one poll and returns the configPolled result without
// failing on a reload error (reload() does).
func (h *reloadHarness) pollOnce() configPolled {
	h.t.Helper()
	next, cmd := h.m.Update(configPollTick(h.clock))
	h.m = next.(Model)
	polled, ok := cmd().(configPolled)
	if !ok {
		h.t.Fatal("the poll command did not produce a configPolled message")
	}
	next, _ = h.m.Update(polled)
	h.m = next.(Model)
	return polled
}

func (h *reloadHarness) configText() string {
	h.t.Helper()
	b, err := os.ReadFile(filepath.Join(h.dir, "config.toml"))
	if err != nil {
		h.t.Fatal(err)
	}
	return string(b)
}

// openEditHarness has the settings takeover open with an unsaved edit
// (ui.ascii staged on) while another writer changed the theme in the file.
func openEditHarness(t *testing.T) *reloadHarness {
	t.Helper()
	h := newReloadHarness(t, "[ui]\ntheme = \"empire\"\nsort_order = \"name\"\n")
	h.openSettings()
	h.m.settingsEdits.ASCII = true
	if !h.m.settingsDirty() {
		t.Fatal("setup: the staged edit is not dirty")
	}
	h.writeConfig("[ui]\ntheme = \"matrix\"\nsort_order = \"name\"\n")
	h.reload()
	return h
}

func assertEditSurvivesReload(t *testing.T, h *reloadHarness) {
	t.Helper()
	if !h.m.settingsOpen || !h.m.settingsEdits.ASCII {
		t.Fatalf("the reload clobbered the open edit (open=%v, ascii edit=%v)", h.m.settingsOpen, h.m.settingsEdits.ASCII)
	}
	if got := h.m.settings.Theme.Name; got != "empire" {
		t.Fatalf("the reloaded theme %q applied while the edit was open, want empire still", got)
	}
}

func TestReloadWithOpenSettingsEditKeepsEditAndAppliesAfterSave(t *testing.T) {
	h := openEditHarness(t)
	assertEditSurvivesReload(t, h)
	h.key(tea.KeyMsg{Type: tea.KeyCtrlS})
	if got := h.m.settings.Theme.Name; got != "matrix" {
		t.Fatalf("theme after save = %q, want the reloaded matrix applied", got)
	}
	if !h.m.settings.ASCII {
		t.Fatal("the saved edit (ui.ascii) was lost")
	}
	text := h.configText()
	if !strings.Contains(text, "ascii = true") || !strings.Contains(text, `theme = "matrix"`) {
		t.Fatalf("the save did not write both the edit and the reloaded key:\n%s", text)
	}
	if h.m.reloadHeld != nil {
		t.Fatal("the held reload survived the save")
	}
}

func TestReloadWithOpenSettingsEditAppliesAfterCancel(t *testing.T) {
	h := openEditHarness(t)
	assertEditSurvivesReload(t, h)
	h.key(tea.KeyMsg{Type: tea.KeyEsc})
	if !h.m.settingsDiscardConfirm {
		t.Fatal("esc on a dirty edit did not raise the discard prompt")
	}
	if got := h.m.settings.Theme.Name; got != "empire" {
		t.Fatalf("theme applied while the discard prompt was up: %q", got)
	}
	h.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if h.m.settingsOpen {
		t.Fatal("the takeover stayed open after the discard")
	}
	if got := h.m.settings.Theme.Name; got != "matrix" {
		t.Fatalf("theme after cancel = %q, want the reloaded matrix", got)
	}
	if h.m.settings.ASCII {
		t.Fatal("the cancelled edit leaked into the running settings")
	}
	if strings.Contains(h.configText(), "ascii = true") {
		t.Fatal("a cancel wrote the discarded edit to config.toml")
	}
}

func TestReloadWithTextEditorOpenWaitsForIt(t *testing.T) {
	h := newReloadHarness(t, "[ui]\ntheme = \"empire\"\n")
	h.openSettings()
	h.m.settingsStringEditing = true
	h.writeConfig("[ui]\ntheme = \"matrix\"\n")
	h.reload()
	if h.m.settings.Theme.Name != "empire" || h.m.reloadHeld == nil {
		t.Fatal("a reload applied under an open text editor")
	}
	h.m.settingsStringEditing = false
	h.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if h.m.settings.Theme.Name != "matrix" || h.m.reloadHeld != nil {
		t.Fatal("the held reload was not applied once the text editor closed")
	}
}

func TestOwnSettingsSaveTriggersNoSecondApply(t *testing.T) {
	h := newReloadHarness(t, "[ui]\ntheme = \"empire\"\n")
	h.openSettings()
	h.m.settingsEdits.Theme = "matrix"
	h.key(tea.KeyMsg{Type: tea.KeyCtrlS})
	if h.m.settings.Theme.Name != "matrix" {
		t.Fatal("setup: the save did not apply the theme")
	}
	applied := h.m.reloadApplied
	polled := h.pollOnce()
	if polled.changed {
		t.Fatal("the poll after our own save reported a change")
	}
	if h.m.reloadApplied != applied || h.m.reloaded != nil {
		t.Fatalf("own save applied a second time: counter %d -> %d, reloaded=%v", applied, h.m.reloadApplied, h.m.reloaded)
	}
	h.writeConfig("[ui]\ntheme = \"empire\"\n")
	if !h.pollOnce().changed || h.m.reloadApplied != applied+1 || h.m.settings.Theme.Name != "empire" {
		t.Fatal("a later external write was not applied once")
	}
}

func TestInvalidConfigKeepsPreviousSettingsShowsNoticeAndRecovers(t *testing.T) {
	const good = "[ui]\ntheme = \"matrix\"\nsort_order = \"name\"\nascii = true\n"
	for _, tc := range []struct{ name, body string }{
		{"truncated mid-string", "[ui]\ntheme = \"matr"},
		{"truncated mid-table", "[ui\nascii = tr"},
		{"garbage", "this is not = [toml\n\x00\x01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newReloadHarness(t, good)
			before := h.m.settings.File
			beforeTheme, applied := h.m.settings.Theme.Name, h.m.reloadApplied
			h.writeConfig(tc.body)
			polled := h.pollOnce()
			if !polled.changed || polled.err == nil {
				t.Fatalf("an invalid file was not reported as a failed reload: %+v", polled)
			}
			if !reflect.DeepEqual(h.m.settings.File, before) || h.m.settings.Theme.Name != beforeTheme || !h.m.settings.ASCII {
				t.Fatalf("an invalid file changed the running settings: %+v", h.m.settings.File)
			}
			if h.m.reloadApplied != applied || h.m.reloaded != nil {
				t.Fatal("an invalid file counted as an applied reload")
			}
			var notice []string
			for _, line := range strings.Split(h.m.View(), "\n") {
				if strings.Contains(line, "config.toml not reloaded") {
					notice = append(notice, line)
				}
			}
			if len(notice) != 1 {
				t.Fatalf("want exactly one notice line in the view, got %d", len(notice))
			}
			if h.pollOnce().changed {
				t.Fatal("the same invalid file was parsed again")
			}
			h.writeConfig("[ui]\ntheme = \"empire\"\nsort_order = \"name\"\nascii = false\n")
			if polled := h.pollOnce(); !polled.changed || polled.err != nil {
				t.Fatalf("the next valid write was not picked up: %+v", polled)
			}
			if h.m.settings.Theme.Name != "empire" || h.m.settings.ASCII {
				t.Fatalf("the valid write did not apply (theme=%s ascii=%v)", h.m.settings.Theme.Name, h.m.settings.ASCII)
			}
			if strings.Contains(h.m.View(), "config.toml not reloaded") {
				t.Fatal("the notice stayed after a valid reload")
			}
		})
	}
}

func TestReloadNeitherReadsNorWritesUIState(t *testing.T) {
	h := newReloadHarness(t, "[ui]\ntheme = \"empire\"\nsort_order = \"name\"\n")
	db, err := store.OpenPath(h.dir, filepath.Join(h.dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := db.SetLayoutMode(ctx, "stacked"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSidebarWidth(ctx, 41); err != nil {
		t.Fatal(err)
	}
	h.m = New(db, h.m.settings, "")
	h.m.width, h.m.height = 120, 30
	if h.m.layoutMode != "stacked" || h.m.sidebarWidth != 41 {
		t.Fatalf("setup: loaded ui state = %q/%d", h.m.layoutMode, h.m.sidebarWidth)
	}
	// Any write to ui_state from now on lands in the audit table; a change
	// made behind the model's back must not be read in either.
	for _, stmt := range []string{
		`CREATE TABLE ui_audit (op TEXT, key TEXT, value TEXT)`,
		`CREATE TRIGGER ui_audit_i AFTER INSERT ON ui_state BEGIN INSERT INTO ui_audit VALUES ('insert', NEW.key, NEW.value); END`,
		`CREATE TRIGGER ui_audit_u AFTER UPDATE ON ui_state BEGIN INSERT INTO ui_audit VALUES ('update', NEW.key, NEW.value); END`,
		`CREATE TRIGGER ui_audit_d AFTER DELETE ON ui_state BEGIN INSERT INTO ui_audit VALUES ('delete', OLD.key, OLD.value); END`,
	} {
		if _, err := db.DB().Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SetSidebarWidth(ctx, 47); err != nil {
		t.Fatal(err)
	}
	var probe int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM ui_audit`).Scan(&probe); err != nil || probe == 0 {
		t.Fatalf("setup: the audit trigger did not see a ui_state write (%d, %v)", probe, err)
	}
	if _, err := db.DB().Exec(`DELETE FROM ui_audit`); err != nil {
		t.Fatal(err)
	}
	h.writeConfig("[ui]\ntheme = \"matrix\"\nsort_order = \"created\"\ndefault_group_first = true\n")
	h.reload()
	if h.m.settings.Theme.Name != "matrix" {
		t.Fatal("setup: the reload did not apply")
	}
	if h.m.layoutMode != "stacked" || h.m.sidebarWidth != 41 {
		t.Fatalf("the reload re-read ui_state: model now %q/%d, want the loaded stacked/41 (the row says 47)", h.m.layoutMode, h.m.sidebarWidth)
	}
	var audited int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM ui_audit`).Scan(&audited); err != nil || audited != 0 {
		t.Fatalf("the reload wrote ui_state (%d writes, err %v)", audited, err)
	}
	if mode, _ := db.GetLayoutMode(ctx); mode != "stacked" {
		t.Fatalf("layout_mode row = %q", mode)
	}
	if width, _ := db.GetSidebarWidth(ctx); width != 47 {
		t.Fatalf("sidebar_width row = %d, want the untouched 47", width)
	}
}

func TestMergeHeldReloadKeepsOwnEditsAndTakesTheRest(t *testing.T) {
	baseline := config.FileConfig{Theme: "a", SortOrder: "name", Env: map[string]string{"K": "1"}}
	edits := baseline
	edits.ASCII = true
	reloaded := baseline
	reloaded.Theme = "b"
	reloaded.Env = map[string]string{"K": "2"}
	got := mergeHeldReload(edits, baseline, reloaded)
	if !got.ASCII || got.Theme != "b" || got.SortOrder != "name" || got.Env["K"] != "2" {
		t.Fatalf("merge = %+v", got)
	}
}

// userThemeHarness has a user theme "midnight" active and the settings
// takeover open with an unsaved edit (ui.ascii staged on), then rewrites the
// theme file's colours under the unchanged configured name and polls, so the
// reload is held. It returns the harness, the colours before and the file path.
func userThemeHeldHarness(t *testing.T, extraConfig string) (*reloadHarness, map[string]string) {
	t.Helper()
	h := newReloadHarness(t, "[ui]\ntheme = \"midnight\"\nsort_order = \"name\"\n")
	body := strings.Replace(builtinEmpireTheme(t), "name = \"empire\"", "name = \"midnight\"", 1)
	path := filepath.Join(h.dir, "themes", "midnight.toml")
	h.write(path, body)
	h.reload()
	first := h.m.settings.Theme
	if first == nil || first.Name != "midnight" {
		t.Fatalf("setup: the user theme was not picked up: %+v", first)
	}
	before := map[string]string{}
	var colour string
	for tok, c := range first.Colors {
		before[string(tok)] = c
		colour = c
	}
	edited := strings.Replace(body, colour, "#123456", 1)
	if edited == body {
		t.Fatalf("setup: colour %s not found in the theme file", colour)
	}
	h.openSettings()
	h.m.settingsEdits.ASCII = true
	h.write(path, edited)
	if extraConfig != "" {
		h.writeConfig(extraConfig)
	}
	h.reload()
	if h.m.reloadHeld == nil {
		t.Fatal("setup: the reload was not held for the open edit")
	}
	if !reflect.DeepEqual(h.m.settings.Theme.Colors, first.Colors) {
		t.Fatal("the theme file change applied while the edit was open")
	}
	return h, before
}

func themeColoursChanged(h *reloadHarness, before map[string]string) bool {
	for tok, c := range h.m.settings.Theme.Colors {
		if before[string(tok)] != c {
			return true
		}
	}
	return false
}

func TestHeldUserThemeFileChangeAppliesAfterSave(t *testing.T) {
	h, before := userThemeHeldHarness(t, "")
	h.key(tea.KeyMsg{Type: tea.KeyCtrlS})
	if !themeColoursChanged(h, before) {
		t.Fatal("saving did not apply the held user theme file change")
	}
	if !h.m.settings.ASCII || !strings.Contains(h.configText(), "ascii = true") {
		t.Fatal("the user's own edit was lost by the save")
	}
	applied := h.m.reloadApplied
	if h.pollOnce().changed || h.m.reloadApplied != applied {
		t.Fatal("the save's own write was applied a second time")
	}
}

func TestHeldUserThemeFileChangeAppliesAfterCancel(t *testing.T) {
	h, before := userThemeHeldHarness(t, "")
	h.key(tea.KeyMsg{Type: tea.KeyEsc})
	h.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if h.m.settingsOpen {
		t.Fatal("the takeover stayed open after the discard")
	}
	if !themeColoursChanged(h, before) {
		t.Fatal("cancelling did not apply the held user theme file change")
	}
	if h.m.settings.ASCII || strings.Contains(h.configText(), "ascii = true") {
		t.Fatal("the cancelled edit leaked")
	}
}

func TestHeldUserThemeFileChangeAppliesAfterSaveWithConfigKeyChange(t *testing.T) {
	// A second deferred component rides with the theme file: sort_order.
	h, before := userThemeHeldHarness(t, "[ui]\ntheme = \"midnight\"\nsort_order = \"created\"\n")
	h.key(tea.KeyMsg{Type: tea.KeyCtrlS})
	if !themeColoursChanged(h, before) {
		t.Fatal("the held theme file change was not applied by the save")
	}
	if h.m.settings.SortOrder != SortOrderCreated {
		t.Fatalf("sort_order = %q, want the held created applied", h.m.settings.SortOrder)
	}
	if !h.m.settings.ASCII {
		t.Fatal("the user's edit was lost")
	}
}

func TestHeldUserThemeFileChangeYieldsToThemeChosenInTheEdit(t *testing.T) {
	h, before := userThemeHeldHarness(t, "")
	h.m.settingsEdits.Theme = "matrix"
	h.key(tea.KeyMsg{Type: tea.KeyCtrlS})
	if h.m.settings.Theme.Name != "matrix" {
		t.Fatalf("theme = %q, want the user's matrix choice to win", h.m.settings.Theme.Name)
	}
	if themeColoursChanged(h, before) && h.m.settings.Theme.Name == "midnight" {
		t.Fatal("the held midnight theme overrode the user's choice")
	}
}

func TestHeldUserThemeFileChangeAppliesWhenEditIsReverted(t *testing.T) {
	h, before := userThemeHeldHarness(t, "")
	h.m.settingsEdits.ASCII = false // the user reverts the staged edit by hand
	h.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if !themeColoursChanged(h, before) {
		t.Fatal("the held theme file change was not applied once the edit was reverted")
	}
}

// nextThemeFileEdit rewrites the user theme file once more (a second external
// writer): the built-in body renamed to midnight with one colour replaced by to.
func nextThemeFileEdit(t *testing.T, h *reloadHarness, to string) {
	t.Helper()
	body := strings.Replace(builtinEmpireTheme(t), "name = \"empire\"", "name = \"midnight\"", 1)
	from := regexp.MustCompile(`#[0-9a-fA-F]{6}`).FindString(body)
	next := strings.Replace(body, from, to, 1)
	if next == body {
		t.Fatalf("setup: %s not found in the theme file", from)
	}
	h.write(filepath.Join(h.dir, "themes", "midnight.toml"), next)
}

func TestExternalThemeFileEditAfterAHeldApplyOnSaveIsStillPickedUp(t *testing.T) {
	h, _ := userThemeHeldHarness(t, "")
	h.key(tea.KeyMsg{Type: tea.KeyCtrlS})
	applied := h.m.reloadApplied
	if h.pollOnce().changed || h.m.reloadApplied != applied {
		t.Fatal("the save's own write was applied again")
	}
	nextThemeFileEdit(t, h, "#654321")
	if !h.pollOnce().changed || h.m.reloadApplied != applied+1 {
		t.Fatalf("a later external theme file edit was not applied once (counter %d -> %d)", applied, h.m.reloadApplied)
	}
	found := false
	for _, c := range h.m.settings.Theme.Colors {
		found = found || c == "#654321"
	}
	if !found {
		t.Fatal("the later external theme colour is not live")
	}
}

func TestHeldThemeFileChangeAppliedOnCancelIsNotAppliedAgainAndLaterEditsStillAre(t *testing.T) {
	h, _ := userThemeHeldHarness(t, "")
	h.key(tea.KeyMsg{Type: tea.KeyEsc})
	h.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if h.m.settingsOpen || h.m.reloadHeld != nil {
		t.Fatal("setup: the discard did not release the held reload")
	}
	applied := h.m.reloadApplied
	if h.pollOnce().changed || h.m.reloadApplied != applied {
		t.Fatal("the next unchanged poll applied the already-applied reload again")
	}
	nextThemeFileEdit(t, h, "#abcdef")
	if !h.pollOnce().changed || h.m.reloadApplied != applied+1 {
		t.Fatal("a later external theme file edit was not picked up after the cancel")
	}
}

func TestSecondHeldThemeFileEditReplacesTheFirstAndAppliesOnSave(t *testing.T) {
	h, _ := userThemeHeldHarness(t, "")
	nextThemeFileEdit(t, h, "#0a0b0c")
	if !h.pollOnce().changed || h.m.reloadHeld == nil || !h.m.settingsDirty() {
		t.Fatal("the second external edit was not held behind the open edit")
	}
	h.key(tea.KeyMsg{Type: tea.KeyCtrlS})
	found := false
	for _, c := range h.m.settings.Theme.Colors {
		found = found || c == "#0a0b0c"
	}
	if !found || !h.m.settings.ASCII {
		t.Fatalf("save did not apply the newest held theme file (found=%v, ascii=%v)", found, h.m.settings.ASCII)
	}
}
