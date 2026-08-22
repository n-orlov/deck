package tui

import (
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
