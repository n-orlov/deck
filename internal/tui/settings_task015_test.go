package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
)

// TestSettingsFieldValueDisplayPerKind pins settingsFieldValueDisplay's
// rendering rule for every FieldKind §11.5 names, including the three
// (string, path, link other than notify) config.Schema happens not to use
// today -- a synthetic config.Field proves the renderer, not just today's
// schema.
func TestSettingsFieldValueDisplayPerKind(t *testing.T) {
	oneHundred := 100
	cfg := config.FileConfig{
		AllowYolo:          true,
		StaleAfter:         45 * time.Second,
		CaptureMinInterval: 5 * time.Second,
		ASCII:              false,
		Mouse:              true,
		RecentCwdLimit:     5,
		Theme:              "midnight",
		Env:                map[string]string{"A": "1", "B": "2"},
	}

	cases := []struct {
		name  string
		field config.Field
		want  string
	}{
		{"toggle on", config.Field{Kind: config.KindToggle, Key: "allow_yolo"}, "On"},
		{"toggle off", config.Field{Kind: config.KindToggle, Section: "ui", Key: "ascii"}, "Off"},
		{
			"bounded integer",
			config.Field{Kind: config.KindInteger, Section: "ui", Key: "recent_cwd_limit", Unit: "entries", IntBounds: config.Bounds{Min: 0, Max: &oneHundred}},
			"5 entries (0-100)",
		},
		{
			"unbounded-above integer",
			config.Field{Kind: config.KindInteger, Key: "stale_after", Unit: "seconds", IntBounds: config.Bounds{Min: 1}},
			"45 seconds (min 1)",
		},
		{"enum selected", config.Field{Kind: config.KindEnum, Section: "ui", Key: "theme"}, "midnight"},
		{"enum empty falls back to default label", config.Field{Kind: config.KindEnum, Key: "unknown-enum"}, "(default)"},
		{"string synthetic field", config.Field{Kind: config.KindString, Default: "hello"}, "hello"},
		{"list-of-strings env", config.Field{Kind: config.KindListOfStrings, Section: "env"}, "2 entries"},
		{"list-of-strings generic default", config.Field{Kind: config.KindListOfStrings, Default: []string{"x", "y"}}, "x, y"},
		{"link notify", config.Field{Kind: config.KindLink, Section: "notify"}, "unavailable this phase"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := settingsFieldValueDisplay(c.field, cfg)
			if got != c.want {
				t.Errorf("settingsFieldValueDisplay(%+v) = %q, want %q", c.field, got, c.want)
			}
		})
	}
}

// TestSettingsCollapseTildeDisplaysPathUnderHome proves KindPath renders
// with the current user's home directory collapsed to `~`, the display-
// side inverse of tui.go's expandCreateCWD.
func TestSettingsCollapseTildeDisplaysPathUnderHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory in this environment")
	}
	got := settingsCollapseTilde(home + "/foo/bar")
	want := "~/foo/bar"
	if got != want {
		t.Fatalf("settingsCollapseTilde(%q) = %q, want %q", home+"/foo/bar", got, want)
	}
	if got := settingsCollapseTilde("/etc/passwd"); got != "/etc/passwd" {
		t.Fatalf("settingsCollapseTilde on a path outside home changed it: %q", got)
	}
}

// TestSettingsScopeLabelComesFromSchemaVerbatim proves every field's
// rendered scope label is exactly one of the three §11.5 names, taken
// straight off the schema Field, and that stale_after (global) never
// claims restart-to-apply while [env] (restart-to-apply per §6.2) never
// claims a live effect it does not have -- the "no field claims a live
// effect it does not have" successCriteria, pinned by name.
func TestSettingsScopeLabelComesFromSchemaVerbatim(t *testing.T) {
	staleAfter, ok := config.FieldByFullKey("stale_after")
	if !ok {
		t.Fatal("schema has no stale_after field")
	}
	if staleAfter.Scope != config.ScopeGlobal {
		t.Fatalf("stale_after scope = %q, want global", staleAfter.Scope)
	}
	lines := settingsFieldDetailLines(staleAfter, 60, "")
	if !strings.Contains(lines[0], "Scope: global") {
		t.Errorf("stale_after detail line = %q, want it to state Scope: global", lines[0])
	}

	env, ok := config.FieldByFullKey("[env]")
	if !ok {
		t.Fatal("schema has no [env] field")
	}
	if env.Scope != config.ScopeRestartToApply {
		t.Fatalf("[env] scope = %q, want restart-to-apply", env.Scope)
	}
	envLines := settingsFieldDetailLines(env, 60, "")
	if !strings.Contains(envLines[0], "Scope: restart-to-apply") {
		t.Errorf("[env] detail line = %q, want it to state Scope: restart-to-apply", envLines[0])
	}
}

// TestSettingsNotifyEntryIsNavigableAndOpensNothing proves §11.5's stated
// exception: [notify] appears as a single navigable entry (present in the
// rendered view, selectable like any other field) that states it is
// unavailable this phase and that activating it (enter/space) changes
// nothing -- no settings.settingsOpen state flips, no dialog opens, no
// panic on a field with no real schema backing.
func TestSettingsNotifyEntryIsNavigableAndOpensNothing(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	updated, _ := model.Update(key(","))
	model = updated.(Model)

	categories := settingsCategories()
	notifyIdx := -1
	for i, cat := range categories {
		if cat.Section == "notify" {
			notifyIdx = i
		}
	}
	if notifyIdx < 0 {
		t.Fatal("settingsCategories() has no notify category")
	}
	if len(categories[notifyIdx].Fields) != 1 {
		t.Fatalf("notify category has %d fields, want exactly 1", len(categories[notifyIdx].Fields))
	}
	notifyField := categories[notifyIdx].Fields[0]
	if notifyField.Kind != config.KindLink {
		t.Fatalf("notify entry kind = %q, want link", notifyField.Kind)
	}

	model.settingsCategoryIndex = notifyIdx
	model.settingsFieldIndex = 0
	model.settingsFocus = settingsFocusFields
	view := model.settingsView()
	if !strings.Contains(view, "unavailable this phase") {
		t.Errorf("settings view with notify selected does not say it is unavailable:\n%s", view)
	}

	updated, _ = model.Update(key("enter"))
	after := updated.(Model)
	if after.settingsOpen != model.settingsOpen || after.settingsCategoryIndex != model.settingsCategoryIndex {
		t.Fatal("enter on the notify entry changed takeover state; it must open nothing")
	}
}

// TestSettingsEnterTogglesBooleanField drives ctrl+s-free toggle editing
// end to end through Model.Update: enter on allow_yolo (General's first
// field) flips the staged value without touching m.settings itself (task
// 016 owns the write-through on save).
func TestSettingsEnterTogglesBooleanField(t *testing.T) {
	model := New(nil, config.Settings{AllowYolo: false}, "")
	updated, _ := model.Update(key(","))
	model = updated.(Model)
	model.settingsFocus = settingsFocusFields
	model.settingsFieldIndex = 0 // allow_yolo is General's first field

	if model.settingsEdits.AllowYolo {
		t.Fatal("premise broke: allow_yolo already staged true before any edit")
	}
	updated, _ = model.Update(key("enter"))
	model = updated.(Model)
	if !model.settingsEdits.AllowYolo {
		t.Fatal("enter on allow_yolo did not flip the staged value to true")
	}
	if model.settings.AllowYolo {
		t.Fatal("enter on allow_yolo mutated m.settings directly; editing must only touch settingsEdits")
	}
	updated, _ = model.Update(key("enter"))
	model = updated.(Model)
	if model.settingsEdits.AllowYolo {
		t.Fatal("second enter on allow_yolo did not flip it back to false")
	}
}

// TestSettingsPlusMinusAdjustsBoundedInteger proves +/- steps an integer
// field's staged value and clamps at its declared Bounds rather than
// stepping past them.
func TestSettingsPlusMinusAdjustsBoundedInteger(t *testing.T) {
	model := New(nil, config.Settings{RecentCwdLimit: 0}, "")
	updated, _ := model.Update(key(","))
	model = updated.(Model)

	categories := settingsCategories()
	uiIdx, fieldIdx := -1, -1
	for ci, cat := range categories {
		if cat.Section != "ui" {
			continue
		}
		for fi, f := range cat.Fields {
			if f.FullKey() == "ui.recent_cwd_limit" {
				uiIdx, fieldIdx = ci, fi
			}
		}
	}
	if uiIdx < 0 {
		t.Fatal("no ui.recent_cwd_limit field found")
	}
	model.settingsCategoryIndex = uiIdx
	model.settingsFieldIndex = fieldIdx
	model.settingsFocus = settingsFocusFields

	// recent_cwd_limit's declared Min is 0: `-` at 0 must clamp, not go
	// negative.
	updated, _ = model.Update(key("-"))
	model = updated.(Model)
	if model.settingsEdits.RecentCwdLimit != 0 {
		t.Fatalf("- at the floor moved RecentCwdLimit to %d, want clamped at 0", model.settingsEdits.RecentCwdLimit)
	}

	updated, _ = model.Update(key("+"))
	model = updated.(Model)
	if model.settingsEdits.RecentCwdLimit != 1 {
		t.Fatalf("+ moved RecentCwdLimit to %d, want 1", model.settingsEdits.RecentCwdLimit)
	}
}

// TestSettingsIntegerBoundsTextFormats pins settingsIntegerBoundsText's two
// shapes: a documented upper bound renders "min-max", an unbounded-above
// field renders "min N".
func TestSettingsIntegerBoundsTextFormats(t *testing.T) {
	max := 50
	bounded := config.Field{Kind: config.KindInteger, IntBounds: config.Bounds{Min: 0, Max: &max}}
	if got := settingsIntegerBoundsText(bounded); got != "0-50" {
		t.Errorf("settingsIntegerBoundsText(bounded) = %q, want %q", got, "0-50")
	}
	unbounded := config.Field{Kind: config.KindInteger, IntBounds: config.Bounds{Min: 1}}
	if got := settingsIntegerBoundsText(unbounded); got != "min 1" {
		t.Errorf("settingsIntegerBoundsText(unbounded) = %q, want %q", got, "min 1")
	}
}
