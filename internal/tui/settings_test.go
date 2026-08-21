package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// TestSettingsKeyOpensAndEscCloses proves the takeover's own on/off switch:
// `,` opens it (never while help is showing, mirroring every other
// overlay's guard) and esc closes it, leaving every other field untouched.
func TestSettingsKeyOpensAndEscCloses(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	updated, _ := model.Update(key(","))
	model = updated.(Model)
	if !model.settingsOpen {
		t.Fatal(", did not open settings")
	}
	if model.settingsCategoryIndex != 0 || model.settingsFieldIndex != 0 {
		t.Fatalf("settings opened with non-zero selection: category=%d field=%d", model.settingsCategoryIndex, model.settingsFieldIndex)
	}
	updated, _ = model.Update(key("esc"))
	model = updated.(Model)
	if model.settingsOpen {
		t.Fatal("esc did not close settings")
	}

	// `,` while help is open is a no-op, exactly like `n`'s existing guard.
	model.help = true
	updated, _ = model.Update(key(","))
	if updated.(Model).settingsOpen {
		t.Fatal(", opened settings while help was showing")
	}
}

// TestSettingsViewRendersSchemaDerivedCategoriesAndFields proves the
// takeover has no second, hand-written field list: every category name and
// every field label settingsCategories()/settingsFieldLabel() derive from
// config.Schema actually appears in the rendered frame, and the view walks
// the schema rather than a fixture the test and the code happen to share.
func TestSettingsViewRendersSchemaDerivedCategoriesAndFields(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.settingsOpen = true
	view := model.settingsView()

	categories := settingsCategories()
	if len(categories) == 0 {
		t.Fatal("settingsCategories() returned no categories")
	}
	for _, cat := range categories {
		if !strings.Contains(view, cat.Name) {
			t.Errorf("settings view missing category %q:\n%s", cat.Name, view)
		}
	}

	// The view opens on category 0; every one of its fields' derived
	// labels must be on screen.
	for _, f := range categories[0].Fields {
		label := settingsFieldLabel(f)
		if !strings.Contains(view, label) {
			t.Errorf("settings view missing field label %q for %s:\n%s", label, f.FullKey(), view)
		}
	}

	// Selecting a later category (UI, whose fields differ from General's)
	// must swap the right-hand list to that category's own fields.
	uiIndex := -1
	for i, cat := range categories {
		if cat.Section == "ui" {
			uiIndex = i
		}
	}
	if uiIndex < 0 {
		t.Fatal("no ui category found — schema.go's ui.* fields moved?")
	}
	model.settingsCategoryIndex = uiIndex
	model.settingsFieldIndex = 0
	uiView := model.settingsView()
	for _, f := range categories[uiIndex].Fields {
		label := settingsFieldLabel(f)
		if !strings.Contains(uiView, label) {
			t.Errorf("ui category view missing field label %q for %s:\n%s", label, f.FullKey(), uiView)
		}
	}
}

// TestSettingsCategoriesGroupEveryFlatKeyExactlyOnce is the schema/settings
// parity precursor to task 018's structural test: every field
// config.Schema declares appears in exactly one category's Fields, and no
// category invents a field the schema does not declare.
func TestSettingsCategoriesGroupEveryFlatKeyExactlyOnce(t *testing.T) {
	categories := settingsCategories()
	seen := map[string]int{}
	for _, cat := range categories {
		for _, f := range cat.Fields {
			seen[f.FullKey()]++
		}
	}
	if len(seen) != len(config.Schema) {
		t.Fatalf("settingsCategories() surfaced %d distinct fields, want %d (one per schema field)", len(seen), len(config.Schema))
	}
	for _, f := range config.Schema {
		if seen[f.FullKey()] != 1 {
			t.Errorf("schema field %s appears %d times across settings categories, want exactly 1", f.FullKey(), seen[f.FullKey()])
		}
	}
}

// TestSettingsFieldLabelIsHumanReadable pins settingsFieldLabel's rendering
// rule against a couple of real schema keys, so a future refactor of the
// helper is caught by name rather than only by the broader "label appears
// on screen" assertion above.
func TestSettingsFieldLabelIsHumanReadable(t *testing.T) {
	cases := []struct {
		field config.Field
		want  string
	}{
		{config.Field{Key: "allow_yolo"}, "Allow Yolo"},
		{config.Field{Key: "recent_cwd_limit"}, "Recent Cwd Limit"},
		{config.Field{Section: "env", Key: ""}, "Environment Variables"},
	}
	for _, c := range cases {
		if got := settingsFieldLabel(c.field); got != c.want {
			t.Errorf("settingsFieldLabel(%+v) = %q, want %q", c.field, got, c.want)
		}
	}
}

// TestSettingsViewFitsFrameBudget is requirement 6's frame-budget guarantee
// applied to the settings takeover (task 013's own success criteria): at a
// stated terminal size, the rendered frame never has more content lines
// than the terminal's rows, and no line's visible width (stringWidth,
// which already discounts SGR escapes the same way the harness's own
// per-cell extraction does) exceeds its columns. Checked at deck's own
// supported minimum (80x24) and at a larger size, so the guarantee is not
// an accident of one particular geometry.
func TestSettingsViewFitsFrameBudget(t *testing.T) {
	for _, size := range []struct{ width, height int }{
		{80, 24},
		{120, 40},
	} {
		model := New(nil, config.Settings{}, "")
		model.settingsOpen = true
		model.width, model.height = size.width, size.height
		view := model.settingsView()
		lines := strings.Split(view, "\n")
		if len(lines) > size.height {
			t.Errorf("at %dx%d: settings view has %d lines, exceeding the %d-row budget", size.width, size.height, len(lines), size.height)
		}
		for i, line := range lines {
			if w := stringWidth(line); w > size.width {
				t.Errorf("at %dx%d: settings view line %d is %d columns wide, exceeding the %d-column budget:\n%s", size.width, size.height, i, w, size.width, line)
			}
		}
	}
}
