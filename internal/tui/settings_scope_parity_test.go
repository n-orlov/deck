package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/theme"
)

// This file is task 007's mechanical parity test: it walks config.Schema
// itself -- the one canonical list, never a second hand-maintained one --
// and, for every declared field, drives a real save through the takeover's
// own key path (`,` -> select the field -> stage an edit if the field's
// Kind has one -> ctrl+s) and checks the field's declared Scope
// (internal/config/schema.go, task 005) against what actually happened:
//
//   - a field whose Scope is NOT restart-to-apply (today only ScopeGlobal
//     is used) must leave the running config.Settings visibly changed by
//     the save;
//   - a field whose Scope IS restart-to-apply must leave the running
//     config.Settings exactly as it was, AND its on-screen detail text
//     (settingsFieldDetailLines, rendered through the real m.View(), not
//     called directly) must say "restart-to-apply" somewhere.
//
// Both directions are checked for every field (not just the direction its
// current Scope claims), so a field mislabelled in either direction --
// claimed live but not actually applied, or claimed restart-to-apply but
// actually applied live -- fails here. See the commit message for this
// test's RED output against a single Scope label flipped in schema.go,
// and its GREEN output on the unmodified tree.

// settingsResolvedSnapshot captures every config.Settings member a save
// could plausibly refresh live (per settingsApplyLiveFields' own doc
// comment and schema.go's per-field consumer notes) -- deliberately never
// File, which every successful save refreshes regardless of Scope and so
// carries no information about whether THIS field applied live.
type settingsResolvedSnapshot struct {
	AllowYolo          bool
	StaleAfter         string // Duration.String(), so a diff prints readably
	CaptureMinInterval string
	ASCII              bool
	Mouse              bool
	RecentCwdLimit     int
	Env                map[string]string
	ThemeName          string
	ThemeReason        string
}

func snapshotResolvedSettings(s config.Settings) settingsResolvedSnapshot {
	themeName := ""
	if s.Theme != nil {
		themeName = s.Theme.Name
	}
	env := make(map[string]string, len(s.Env))
	for k, v := range s.Env {
		env[k] = v
	}
	return settingsResolvedSnapshot{
		AllowYolo:          s.AllowYolo,
		StaleAfter:         s.StaleAfter.String(),
		CaptureMinInterval: s.CaptureMinInterval.String(),
		ASCII:              s.ASCII,
		Mouse:              s.Mouse,
		RecentCwdLimit:     s.RecentCwdLimit,
		Env:                env,
		ThemeName:          themeName,
		ThemeReason:        s.ThemeReason,
	}
}

// settingsFindFieldIndex walks settingsCategories() -- the same walk of
// config.Schema the takeover itself uses to build its own lists -- and
// returns the category/field indices f.FullKey() sits at, so this test
// never hand-maintains a second copy of the schema's shape either.
func settingsFindFieldIndex(f config.Field) (categoryIndex, fieldIndex int, ok bool) {
	for ci, cat := range settingsCategories() {
		for fi, cf := range cat.Fields {
			if cf.FullKey() == f.FullKey() {
				return ci, fi, true
			}
		}
	}
	return 0, 0, false
}

// settingsStageRealEdit stages a change to the selected field through the
// exact key the real takeover binds for that Kind (settingsActivateField/
// settingsAdjustField's own doc comments: enter flips a toggle, +/- steps
// an integer or cycles an enum). KindListOfStrings ([env]) is deliberately
// left untouched: enter would hand focus to the entries list, whose own
// keymap does not bind ctrl+s (updateSettingsEnvList has no such case), so
// pressing it here would make the save this test still needs to drive
// silently never happen. [env] is restart-to-apply either way and has no
// resolved config.Settings member for a save to touch, so an unedited
// real save (file rewritten to the same value) still exercises the same
// ctrl+s path and proves the same "did not change the running settings"
// half of the parity check.
func settingsStageRealEdit(m *Model, f config.Field) {
	switch f.Kind {
	case config.KindToggle:
		updated, _ := m.Update(key("enter"))
		*m = updated.(Model)
	case config.KindInteger:
		updated, _ := m.Update(key("+"))
		*m = updated.(Model)
	case config.KindEnum:
		// A single "+" is not always enough: config.Schema's one enum
		// field today (ui.theme) starts staged at "" (nothing configured),
		// which theme.Resolve treats as an alias for the very first cycled
		// option (theme.DefaultName, "empire") -- cycling onto that name
		// explicitly changes the RAW staged string but not the RESOLVED
		// theme, so a bare single press here would stage a real edit that
		// still (correctly) does not move the running theme, and this test
		// would misreport that as a ScopeGlobal field failing to apply
		// live. Keep cycling, bounded by the option count, until the
		// resolved theme this field's raw value would produce actually
		// differs from the one already running.
		initialRaw := settingsEnumValue(f, m.settingsEdits)
		beforeThemeName := ""
		if f.FullKey() == "ui.theme" && m.settings.Theme != nil {
			beforeThemeName = m.settings.Theme.Name
		}
		options := m.settingsFieldOptions(f)
		for i := 0; i < len(options)+1; i++ {
			updated, _ := m.Update(key("+"))
			*m = updated.(Model)
			newRaw := settingsEnumValue(f, m.settingsEdits)
			if newRaw == initialRaw {
				continue
			}
			if f.FullKey() != "ui.theme" {
				break
			}
			userThemes, userErrs := theme.DiscoverUserThemes(theme.ThemesDir(m.settings.Paths.ConfigFile))
			resolved, _ := theme.Resolve(userThemes, userErrs, newRaw)
			if resolved.Name != beforeThemeName {
				break
			}
		}
	}
}

// TestSettingsScopeParityMatchesSaveBehaviour is task 007's parity test.
//
// requirement 21 correction (operator steer 008-envoverride-applylive.md,
// 22 Aug 2026 13:00 BST): the ScopeGlobal branch below ("default:" case)
// asserts a save DOES change the running config.Settings snapshot -- true
// for every ScopeGlobal field EXCEPT when that same key is also named in
// this fixture's config.Settings.EnvOverrides, in which case §6.5 ("the
// environment always outranks the file") demands the opposite: the
// running value must stay pinned to what the environment resolved, no
// matter what the file's own edit says. settingsLiveApplyTestModel never
// sets any DECK_* override, so this loop alone cannot see that case --
// TestSettingsScopeParityEnvOverrideExemption below drives the identical
// key path against a fixture that DOES set one, and inverts the very same
// assertion this loop makes for that field. This is the exemption the
// reviewer asked to be taught explicitly, not a relaxation of this test:
// nothing here becomes more permissive for a field with no override.
func TestSettingsScopeParityMatchesSaveBehaviour(t *testing.T) {
	for _, f := range config.Schema {
		f := f
		t.Run(f.FullKey(), func(t *testing.T) {
			m, _ := settingsLiveApplyTestModel(t)
			ci, fi, ok := settingsFindFieldIndex(f)
			if !ok {
				t.Fatalf("field %s not found in settingsCategories(); settingsFindFieldIndex/settingsCategories disagree with config.Schema", f.FullKey())
			}
			m = settingsOpenAndSelect(t, m, ci, fi, settingsFieldLabel(f))

			// Captured while the takeover is open on this exact field, so
			// this is the real on-screen text the operator would read,
			// not settingsFieldDetailLines called directly out of context.
			screen := m.View()

			before := snapshotResolvedSettings(m.settings)
			settingsStageRealEdit(&m, f)
			updated, _ := m.Update(key("ctrl+s"))
			m = updated.(Model)
			after := snapshotResolvedSettings(m.settings)

			if m.settingsNote == "" || strings.HasPrefix(m.settingsNote, "save failed") {
				t.Fatalf("field %s: ctrl+s did not report a successful save (note = %q)", f.FullKey(), m.settingsNote)
			}

			changedLive := !reflect.DeepEqual(before, after)

			switch f.Scope {
			case config.ScopeRestartToApply:
				if changedLive {
					t.Fatalf("field %s is declared Scope restart-to-apply (schema.go) but ctrl+s changed the running config.Settings snapshot:\nbefore: %+v\nafter:  %+v\nrestart-to-apply must leave the already-running client untouched until the process restarts", f.FullKey(), before, after)
				}
				if !strings.Contains(screen, "restart-to-apply") {
					t.Fatalf("field %s is declared Scope restart-to-apply (schema.go) but its on-screen detail text does not say \"restart-to-apply\" anywhere:\n%s", f.FullKey(), screen)
				}
			default:
				if !changedLive {
					t.Fatalf("field %s is declared Scope %q (schema.go), not restart-to-apply, but ctrl+s did not change the running config.Settings snapshot -- before and after are equal: %+v", f.FullKey(), f.Scope, before)
				}
			}
		})
	}
}

// settingsEnvOverrideParityTestModel builds the same eight-key config.toml
// shape settingsLiveApplyTestModel does, EXCEPT [ui] ascii and [ui] mouse
// both start true (rather than false) and envVar is set in the process's
// environment for this load -- so config.LoadFrom resolves BOTH the
// running value and the file's own starting value to true, and records
// the override in config.Settings.EnvOverrides at the same time (§6.5:
// the environment always outranks the file, even when the two already
// happen to agree). Starting the file value at true rather than false is
// deliberate: settingsStageRealEdit's KindToggle case (`enter` flips the
// staged value from whatever it starts at) then stages a MOVE AWAY FROM
// the resolved/running value (true -> false), which is the one direction
// that actually distinguishes "the running value stayed pinned to the
// environment" from "the running value happened to land on the same spot
// the edit was heading to anyway" -- the exact fixture blindness the
// operator steer (008-envoverride-applylive.md, 22 Aug 2026 13:00 BST)
// named in TestSettingsLabelsAnEnvOverriddenFieldAndDoesNotLie's original
// form.
func settingsEnvOverrideParityTestModel(t *testing.T, envVar string) (model Model, path string) {
	t.Helper()
	dir := t.TempDir()
	path = filepath.Join(dir, "config.toml")
	contents := "allow_yolo = false\n" +
		"stale_after = 45\n" +
		"capture_min_interval = 5\n" +
		"[ui]\n" +
		"ascii = true\n" +
		"mouse = true\n" +
		"recent_cwd_limit = 5\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("seed config.toml: %v", err)
	}
	getenv := func(name string) string {
		switch name {
		case "DECK_HOME":
			return dir
		case envVar:
			return "true"
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

// TestSettingsScopeParityEnvOverrideExemption is the exemption
// TestSettingsScopeParityMatchesSaveBehaviour's own comment names:
// requirement 21 (§6.5, "the environment always outranks the file")
// means a ScopeGlobal field's save must NOT change the running
// config.Settings when that field's key is also named in
// config.Settings.EnvOverrides, even though every other ScopeGlobal field
// (and this same field with no override set) must. Reverting the
// settingsApplyLiveFields exemption (internal/tui/settings.go) turns both
// subtests below red; see the commit message for the real RED/GREEN
// output.
func TestSettingsScopeParityEnvOverrideExemption(t *testing.T) {
	cases := []struct {
		fullKey    string
		envVar     string
		category   int
		fieldIndex int
		label      string
		wantCmdNil bool
	}{
		{fullKey: "ui.ascii", envVar: "DECK_ASCII", category: 1, fieldIndex: 1, label: "Ascii", wantCmdNil: true},
		{fullKey: "ui.mouse", envVar: "DECK_MOUSE", category: 1, fieldIndex: 2, label: "Mouse", wantCmdNil: true},
	}
	for _, c := range cases {
		c := c
		t.Run(c.fullKey, func(t *testing.T) {
			f, ok := config.FieldByFullKey(c.fullKey)
			if !ok {
				t.Fatalf("config.FieldByFullKey(%q) not found", c.fullKey)
			}
			if f.Scope != config.ScopeGlobal {
				t.Fatalf("field %s is Scope %q, want ScopeGlobal -- this test only proves the exemption inverts a live-applying field's behaviour", c.fullKey, f.Scope)
			}

			m, _ := settingsEnvOverrideParityTestModel(t, c.envVar)
			if _, overridden := m.settings.EnvOverrides[c.fullKey]; !overridden {
				t.Fatalf("seed load EnvOverrides = %+v, want %s overridden by %s", m.settings.EnvOverrides, c.fullKey, c.envVar)
			}

			m = settingsOpenAndSelect(t, m, c.category, c.fieldIndex, c.label)
			before := snapshotResolvedSettings(m.settings)
			settingsStageRealEdit(&m, f)
			updated, cmd := m.Update(key("ctrl+s"))
			m = updated.(Model)
			after := snapshotResolvedSettings(m.settings)

			if m.settingsNote == "" || strings.HasPrefix(m.settingsNote, "save failed") {
				t.Fatalf("field %s: ctrl+s did not report a successful save (note = %q)", c.fullKey, m.settingsNote)
			}

			// The inverted assertion: a ScopeGlobal field would normally have
			// to show changedLive == true here (see the default: branch
			// above); under this field's own env override it must be false.
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("field %s is env-overridden by %s but ctrl+s changed the running config.Settings snapshot anyway:\nbefore: %+v\nafter:  %+v\nrequirement 21 (\u00a76.5): the environment must still outrank the file for the ALREADY-RUNNING client, not just the next launch", c.fullKey, c.envVar, before, after)
			}
			// The file itself must still have been rewritten -- editing an
			// env-overridden field is never refused, only its effect on the
			// running value is (steer's fence: "do not make the takeover
			// refuse to edit an env-overridden field").
			if reflect.DeepEqual(m.settings.File, config.FileConfig{}) {
				t.Fatal("m.settings.File is the zero value; the save path itself is broken, not just the live-apply gate")
			}
			if c.wantCmdNil && cmd != nil {
				if msg := cmd(); msg != nil {
					t.Fatalf("field %s is env-overridden but ctrl+s returned a non-nil tea.Cmd producing %#v; an env-overridden mouse save must not tell the terminal to start/stop emitting SGR reports either", c.fullKey, msg)
				}
			}
		})
	}
}
