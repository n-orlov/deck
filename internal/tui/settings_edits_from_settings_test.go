package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// Task 310 (R53 follow-up): settingsAssertFieldRoundTrips (task 018,
// settings_schema_parity_test.go) proves every schema field's kind-specific
// get/set pair round-trips through a bare config.FileConfig. That is a
// different code path from settingsEditsFromSettings (settings.go), the
// function settingsSave() actually calls to turn the *running* Model's
// config.Settings back into the config.FileConfig written to disk and
// fed back into settingsCategories() for redisplay -- and task 215 found
// that ui.preview_fit had a real get/set case in
// settingsToggleValue/settingsSetToggle (so task 018's own test passed)
// while settingsEditsFromSettings's struct literal simply had no
// `PreviewFit: s.File.PreviewFit,` line, silently dropping every save of
// that field back to Go's zero value. This file is the guard for that
// exact failure mode: it walks config.Schema, stages a non-default value
// into a config.FileConfig via the existing set helpers, wraps it in a
// config.Settings, runs it through settingsEditsFromSettings, and fails
// if the value that comes back out does not match what was staged in.
//
// Red proof (demonstrated by hand, not left as a permanent fixture, for
// the same reason task 018's own comment gives -- a real omission would
// also be caught by re-running this test after commenting out one field):
// commenting out the `SortOrder: s.File.SortOrder,` line in
// settingsEditsFromSettings makes TestSettingsEditsFromSettingsCoversEvery
// FlatKey fail with:
//
//	ui.sort_order (kind enum): settingsEditsFromSettings dropped the
//	staged value (got "", want "__task310_probe__")
//
// (the dropped assignment leaves SortOrder at its Go zero value, "";
// settingsEnumValue still has a case for ui.sort_order in its FullKey
// switch, it just reads the now-unset struct field, so this is not the
// Field.Default fallback. The string above is what was actually
// observed on the reverted tree and pasted into the commit message for
// this task). Restoring the field line returns the suite to green.
func TestSettingsEditsFromSettingsCoversEveryFlatKey(t *testing.T) {
	for _, f := range config.Schema {
		full := f.FullKey()
		if full == "[env]" {
			// Env is a map, not a scalar; exercised separately below
			// since none of the scalar set/get helpers apply to it.
			continue
		}

		var cfg config.FileConfig
		var want string

		switch f.Kind {
		case config.KindToggle:
			// The zero value of every declared toggle field is false,
			// so a dropped assignment could coincide with a probe of
			// false and pass by accident; probe true, which a Go zero
			// value can never equal.
			settingsSetToggle(&cfg, f, true)
			got := settingsToggleValue(f, settingsEditsFromSettings(config.Settings{File: cfg}))
			if !got {
				t.Errorf("%s (kind toggle): settingsEditsFromSettings dropped the staged value (got %v, want true)", full, got)
			}
			continue

		case config.KindInteger:
			probe := f.IntBounds.Min + 1
			if f.IntBounds.Max != nil && *f.IntBounds.Max < probe {
				probe = f.IntBounds.Min
			}
			settingsSetInteger(&cfg, f, probe)
			got := settingsIntegerValue(f, settingsEditsFromSettings(config.Settings{File: cfg}))
			if got != probe {
				t.Errorf("%s (kind integer): settingsEditsFromSettings dropped the staged value (got %d, want %d)", full, got, probe)
			}
			continue

		case config.KindEnum:
			want = "__task310_probe__"
			settingsSetEnum(&cfg, f, want)
			got := settingsEnumValue(f, settingsEditsFromSettings(config.Settings{File: cfg}))
			if got != want {
				t.Errorf("%s (kind enum): settingsEditsFromSettings dropped the staged value (got %q, want %q)", full, got, want)
			}
			continue

		case config.KindListOfStrings:
			// [env] is the only field of this kind and is handled
			// above; nothing else to probe generically.
			continue

		default:
			t.Errorf("%s: config.Schema declares a %s field with no settingsEditsFromSettings coverage check wired here -- add one before shipping this field", full, f.Kind)
		}
	}

	// [env]: settingsEditsFromSettings clones the map via
	// settingsCloneEnv rather than aliasing it; check both that the
	// entry survives the round trip AND that the two maps are backed by
	// distinct storage (settingsCloneEnv's own documented contract).
	src := config.FileConfig{Env: map[string]string{"TASK310_PROBE": "1"}}
	out := settingsEditsFromSettings(config.Settings{File: src})
	if out.Env == nil || out.Env["TASK310_PROBE"] != "1" {
		t.Errorf("[env]: settingsEditsFromSettings dropped the staged value (got %#v, want map with TASK310_PROBE=1)", out.Env)
	}
	if len(out.Env) > 0 {
		out.Env["TASK310_PROBE"] = "mutated"
		if src.Env["TASK310_PROBE"] != "1" {
			t.Errorf("[env]: settingsEditsFromSettings aliased the source map instead of cloning it")
		}
	}
}
