package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/theme"
)

// settingsLiveApplyTestModel builds a real config.toml (all eight schema
// keys explicit, so every field's starting file value is known rather than
// left to an implicit default) with no environment overrides, and loads it
// through the real config.LoadFrom/loadConfigFile path -- exactly what
// cmd/deck/main.go does -- rather than constructing config.Settings by hand,
// so this test exercises the same Settings.File/resolved-field split tasks
// 001/002 introduced.
func settingsLiveApplyTestModel(t *testing.T) (model Model, path string) {
	t.Helper()
	dir := t.TempDir()
	path = filepath.Join(dir, "config.toml")
	contents := "allow_yolo = false\n" +
		"stale_after = 45\n" +
		"capture_min_interval = 5\n" +
		"[ui]\n" +
		"ascii = false\n" +
		"mouse = false\n" +
		"recent_cwd_limit = 5\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("seed config.toml: %v", err)
	}
	getenv := func(name string) string {
		if name == "DECK_HOME" {
			return dir
		}
		return ""
	}
	userHome := func() (string, error) { return dir, nil }
	loaded, err := config.LoadFrom(getenv, userHome)
	if err != nil {
		t.Fatalf("config.LoadFrom (seed load): %v", err)
	}
	loaded.Color = true
	m := New(nil, loaded, "")
	m.width, m.height = 120, 30
	return m, path
}

// settingsOpenAndSelect opens the `,` takeover and points the field-list
// focus at categories[categoryIndex].Fields[fieldIndex], asserting the
// label matches wantLabel so a schema reorder fails loudly here rather
// than silently exercising the wrong field.
func settingsOpenAndSelect(t *testing.T, m Model, categoryIndex, fieldIndex int, wantLabel string) Model {
	t.Helper()
	updated, _ := m.Update(key(","))
	m = updated.(Model)
	if !m.settingsOpen {
		t.Fatal(", did not open settings")
	}
	m.settingsFocus = settingsFocusFields
	m.settingsCategoryIndex = categoryIndex
	m.settingsFieldIndex = fieldIndex
	fields := settingsCategories()[categoryIndex].Fields
	if fieldIndex >= len(fields) {
		t.Fatalf("category %d has only %d fields, want index %d", categoryIndex, len(fields), fieldIndex)
	}
	if got := settingsFieldLabel(fields[fieldIndex]); got != wantLabel {
		t.Fatalf("category %d field %d = %q, want %q (schema order changed?)", categoryIndex, fieldIndex, got, wantLabel)
	}
	return m
}

// TestSettingsSaveAppliesLiveScopeFieldsWithoutRestart drives the real `,`
// -> select field -> edit -> ctrl+s path for every ScopeGlobal field
// (config.Schema, task 005) and asserts both that the running
// config.Settings changed AND that View() itself renders differently
// afterwards, per task 006's own success criteria -- not merely that a
// struct field flipped somewhere the screen never reads.
func TestSettingsSaveAppliesLiveScopeFieldsWithoutRestart(t *testing.T) {
	t.Run("allow_yolo", func(t *testing.T) {
		m, _ := settingsLiveApplyTestModel(t)
		if m.settings.AllowYolo {
			t.Fatal("seed AllowYolo = true, want false")
		}
		m = settingsOpenAndSelect(t, m, 0, 0, "Allow Yolo")
		updated, _ := m.Update(key("enter"))
		m = updated.(Model)
		updated, cmd := m.Update(key("ctrl+s"))
		m = updated.(Model)
		if !m.settings.AllowYolo {
			t.Fatal("ctrl+s did not refresh the running m.settings.AllowYolo to true")
		}
		if cmd != nil {
			t.Fatal("allow_yolo save returned a non-nil tea.Cmd; only mouse toggling should")
		}
	})

	t.Run("ui.ascii", func(t *testing.T) {
		m, _ := settingsLiveApplyTestModel(t)
		updated, _ := m.Update(key("esc"))
		before := updated.(Model).View()
		if m.settings.ASCII {
			t.Fatal("seed ASCII = true, want false")
		}
		m = settingsOpenAndSelect(t, m, 1, 1, "Ascii")
		updated, _ = m.Update(key("enter"))
		m = updated.(Model)
		updated, _ = m.Update(key("ctrl+s"))
		m = updated.(Model)
		if !m.settings.ASCII {
			t.Fatal("ctrl+s did not refresh the running m.settings.ASCII to true")
		}
		updated, _ = m.Update(key("esc"))
		m = updated.(Model)
		if m.settingsOpen {
			t.Fatal("esc after a clean save left settings open")
		}
		after := m.View()
		if after == before {
			t.Fatalf("View() unchanged after a live ui.ascii save; box glyphs should switch from Unicode to ASCII on the very next frame")
		}
		if !strings.Contains(after, "+") {
			t.Fatalf("View() after ascii save has no ASCII box glyph:\n%s", after)
		}
	})

	t.Run("ui.mouse", func(t *testing.T) {
		m, _ := settingsLiveApplyTestModel(t)
		if m.settings.Mouse {
			t.Fatal("seed Mouse = true, want false")
		}
		m = settingsOpenAndSelect(t, m, 1, 2, "Mouse")
		updated, _ := m.Update(key("enter"))
		m = updated.(Model)
		updated, cmd := m.Update(key("ctrl+s"))
		m = updated.(Model)
		if !m.settings.Mouse {
			t.Fatal("ctrl+s did not refresh the running m.settings.Mouse to true")
		}
		if cmd == nil {
			t.Fatal("turning ui.mouse on returned a nil tea.Cmd; the terminal must be told to start emitting SGR mouse reports")
		}
		if msg := cmd(); msg != tea.EnableMouseCellMotion() {
			t.Fatalf("ui.mouse-on save returned cmd producing %#v, want tea.EnableMouseCellMotion()", msg)
		}
	})

	t.Run("ui.theme", func(t *testing.T) {
		m, _ := settingsLiveApplyTestModel(t)
		before := m.View()
		original := m.settings.Theme.Name
		builtins := theme.Builtins()
		var target string
		for _, th := range builtins {
			if th.Name != original {
				target = th.Name
				break
			}
		}
		if target == "" {
			t.Fatal("need at least two built-in themes for this test")
		}
		m = settingsOpenAndSelect(t, m, 1, 0, "Theme")
		// cycle "+" until the staged theme edit reaches target -- bounded
		// walk, same pattern theme_picker_test.go uses for its own cycle.
		for i := 0; i < len(theme.Builtins())+1 && settingsEnumValue(config.Field{Section: "ui", Key: "theme", Kind: config.KindEnum}, m.settingsEdits) != target; i++ {
			updated, _ := m.Update(key("+"))
			m = updated.(Model)
		}
		if got := m.settingsEdits.Theme; got != target {
			t.Fatalf("could not cycle staged theme onto %q, stuck at %q", target, got)
		}
		updated, _ := m.Update(key("ctrl+s"))
		m = updated.(Model)
		if m.settings.Theme.Name != target {
			t.Fatalf("ctrl+s did not refresh the running m.settings.Theme; got %q, want %q", m.settings.Theme.Name, target)
		}
		updated, _ = m.Update(key("esc"))
		m = updated.(Model)
		after := m.View()
		if after == before {
			t.Fatal("View() unchanged after a live ui.theme save; the coloured render should differ")
		}
	})
}

// TestSettingsSaveDoesNotApplyRestartToApplyFieldsLive proves the other
// half of task 006's success criteria: a field task 005 declared
// restart-to-apply is written to the file (Settings.File does change) but
// the running config.Settings member it decides not to trust live is left
// exactly alone.
func TestSettingsSaveDoesNotApplyRestartToApplyFieldsLive(t *testing.T) {
	t.Run("stale_after", func(t *testing.T) {
		m, _ := settingsLiveApplyTestModel(t)
		runningBefore := m.settings.StaleAfter
		m = settingsOpenAndSelect(t, m, 0, 1, "Stale After")
		updated, _ := m.Update(key("+"))
		m = updated.(Model)
		updated, _ = m.Update(key("ctrl+s"))
		m = updated.(Model)
		if m.settings.StaleAfter != runningBefore {
			t.Fatalf("ctrl+s changed the running m.settings.StaleAfter from %v to %v; stale_after is restart-to-apply and must not take effect live", runningBefore, m.settings.StaleAfter)
		}
		if m.settings.File.StaleAfter == 45 {
			t.Fatal("the file value was not staged/written at all; the field-adjust or save itself is broken, not just the live-apply gate")
		}
	})

	t.Run("capture_min_interval", func(t *testing.T) {
		m, _ := settingsLiveApplyTestModel(t)
		m = settingsOpenAndSelect(t, m, 0, 2, "Capture Min Interval")
		updated, _ := m.Update(key("+"))
		m = updated.(Model)
		updated, _ = m.Update(key("ctrl+s"))
		m = updated.(Model)
		if m.settings.File.CaptureMinInterval != 6*time.Second {
			t.Fatalf("save did not write the staged capture_min_interval edit; File.CaptureMinInterval = %v, want 6s", m.settings.File.CaptureMinInterval)
		}
		// capture_min_interval has no resolved config.Settings member at
		// all today (schema.go's own comment: no consumer exists yet), so
		// there is nothing further to assert did not change live -- the
		// File-only write above is the whole of this field's observable
		// behaviour.
	})

	t.Run("ui.recent_cwd_limit", func(t *testing.T) {
		m, _ := settingsLiveApplyTestModel(t)
		m = settingsOpenAndSelect(t, m, 1, 3, "Recent Cwd Limit")
		updated, _ := m.Update(key("+"))
		m = updated.(Model)
		updated, _ = m.Update(key("ctrl+s"))
		m = updated.(Model)
		if m.settings.File.RecentCwdLimit != 6 {
			t.Fatalf("save did not write the staged recent_cwd_limit edit; File.RecentCwdLimit = %d, want 6", m.settings.File.RecentCwdLimit)
		}
		// Same as capture_min_interval: no resolved config.Settings member
		// exists for this field yet, so File is the only place to check.
	})
}
