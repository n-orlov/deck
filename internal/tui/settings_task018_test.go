package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// Task 018 (SPEC §11.5/§22): the structural schema/settings parity test.
// settingsCategories() (settings.go) is documented as "the one and only
// place tasks 013-018 read the schema to build the takeover's category/
// field lists", but that alone does not prove every flat key is actually
// *editable* through settings -- a key present in the rendered list whose
// get/set dispatch (settingsToggleValue/settingsSetToggle etc.) has no
// case for its FullKey would render and even accept keystrokes while
// silently never changing anything, which is worse than a visible gap.
// This file proves both directions:
//
//  1. TestSettingsSchemaParity_EveryFlatKeyIsReachable: every field
//     config.Schema declares appears exactly once in settingsCategories(),
//     AND round-trips through its kind's generic get/set pair (not just a
//     fallback to Field.Default, which a missing FullKey case would also
//     produce and so must not be mistaken for success).
//  2. TestSettingsSchemaParity_EveryRenderedFieldIsBackedBySchema: every
//     field settingsCategories() renders has a matching config.Schema
//     entry, with exactly one documented exception -- the synthetic
//     [notify] link entry (settingsNotifyEntry), which schema.go's own
//     comment states is deliberately absent from config.Schema.
//
// Together these mean there is no second, hand-maintained key set:
// config.Schema is the only place a flat key is declared, and nothing in
// settings can silently drift from it in either direction.
//
// Red/green was demonstrated by hand while writing this test (not left as
// a permanent fixture, since a real schema addition without a matching
// round-trip case would also trip internal/config's own
// TestSchemaPinsKeySet/TestSchemaScopes pinning tests -- adding a second
// permanent "expect N keys" test here would itself become the second
// hand-maintained list task 018 exists to forbid): a temporary
//
//	{Section: "", Key: "task018_probe", Kind: KindToggle, Default: false,
//	 Description: "probe", Scope: ScopeGlobal}
//
// appended to config.Schema, with NO case added to settingsToggleValue/
// settingsSetToggle, makes
// TestSettingsSchemaParity_EveryFlatKeyIsReachable fail with "task018_probe
// (kind toggle): set-then-get round-trip did not reflect the write (get
// after set-true = false, want true)" -- proving the round-trip check
// catches exactly the defect it exists to catch, not just a rendering
// gap. Reverting the temporary field returns the suite to green. See the
// commit message for this task for the literal `go test` output of both
// runs.
func TestSettingsSchemaParity_EveryFlatKeyIsReachable(t *testing.T) {
	rendered := map[string]bool{}
	for _, cat := range settingsCategories() {
		for _, f := range cat.Fields {
			rendered[f.FullKey()] = true
		}
	}

	for _, f := range config.Schema {
		full := f.FullKey()
		if !rendered[full] {
			t.Errorf("%s: declared in config.Schema but does not appear in settingsCategories() -- unreachable in the settings takeover", full)
			continue
		}
		settingsAssertFieldRoundTrips(t, f)
	}
}

func TestSettingsSchemaParity_EveryRenderedFieldIsBackedBySchema(t *testing.T) {
	for _, cat := range settingsCategories() {
		for _, f := range cat.Fields {
			full := f.FullKey()
			// The one documented structural exception (schema.go,
			// settingsCategories(), settingsNotifyEntry()): [notify] is
			// a synthetic settings-only entry, never a config.Schema
			// member, because §11.5 gives it its own dialog rather than
			// a flattened field -- there is no dialog this phase, so it
			// is rendered as a link that opens nothing, per task 015.
			if full == "[notify]" {
				continue
			}
			if _, ok := config.FieldByFullKey(full); !ok {
				t.Errorf("%s: rendered by settingsCategories() but absent from config.Schema -- a second, hand-maintained settings field with no schema backing", full)
			}
		}
	}
}

// settingsAssertFieldRoundTrips drives field f's kind-appropriate generic
// get/set pair (the same functions settingsActivateField/settingsAdjust-
// Field/settingsSave use) through values distinct from Field.Default in
// both directions, and fails if what comes back out does not match what
// was staged. A FullKey switch case missing for f would instead fall back
// silently to Field.Default regardless of what was set -- exercising both
// a true and a false (or two distinct non-default) probe values makes
// that fallback distinguishable from a real round-trip even when Default
// happens to coincide with one of the probes.
func settingsAssertFieldRoundTrips(t *testing.T, f config.Field) {
	t.Helper()
	full := f.FullKey()
	switch f.Kind {
	case config.KindToggle:
		var cfg config.FileConfig
		settingsSetToggle(&cfg, f, true)
		if got := settingsToggleValue(f, cfg); !got {
			t.Errorf("%s (kind toggle): set-then-get round-trip did not reflect the write (get after set-true = %v, want true)", full, got)
		}
		settingsSetToggle(&cfg, f, false)
		if got := settingsToggleValue(f, cfg); got {
			t.Errorf("%s (kind toggle): set-then-get round-trip did not reflect the write (get after set-false = %v, want false)", full, got)
		}

	case config.KindInteger:
		var cfg config.FileConfig
		probe1 := f.IntBounds.Min
		probe2 := f.IntBounds.Min + 1
		if f.IntBounds.Max != nil && *f.IntBounds.Max < probe2 {
			probe2 = probe1
		}
		settingsSetInteger(&cfg, f, probe1)
		if got := settingsIntegerValue(f, cfg); got != probe1 {
			t.Errorf("%s (kind integer): set-then-get round-trip did not reflect the write (get after set(%d) = %d)", full, probe1, got)
		}
		if probe2 != probe1 {
			settingsSetInteger(&cfg, f, probe2)
			if got := settingsIntegerValue(f, cfg); got != probe2 {
				t.Errorf("%s (kind integer): set-then-get round-trip did not reflect the write (get after set(%d) = %d)", full, probe2, got)
			}
		}

	case config.KindEnum:
		var cfg config.FileConfig
		for _, probe := range []string{"__task018_probe_a__", "__task018_probe_b__"} {
			settingsSetEnum(&cfg, f, probe)
			if got := settingsEnumValue(f, cfg); got != probe {
				t.Errorf("%s (kind enum): set-then-get round-trip did not reflect the write (get after set(%q) = %q)", full, probe, got)
			}
		}

	case config.KindListOfStrings:
		// [env] (the one KindListOfStrings field config.Schema declares
		// today) has no generic set path: §11.5's per-entry editor for
		// it is out of scope through task 018 (settings.go's own
		// settingsFieldDetailLines documents this), so reachability for
		// it means "rendered and its count-only display does not
		// panic", already proven by this field appearing in `rendered`
		// above. Exercise the display path anyway so a future change
		// that makes it panic on an empty map is still caught here.
		_ = settingsListValueDisplay(f, config.FileConfig{})

	case config.KindString, config.KindPath, config.KindLink:
		// config.Schema declares no field of these kinds today (the
		// only KindLink entry, [notify], is the synthetic
		// settings-only exception the other test in this file
		// excludes). If one is ever added, it must fail here loudly
		// until a real get/set round-trip is wired for it, rather than
		// silently passing because there was nothing to check.
		t.Errorf("%s: config.Schema declares a %s field, but settings has no generic get/set round-trip wired for that kind yet -- add one (and a case here) before shipping this field", full, f.Kind)

	default:
		t.Errorf("%s: unknown Kind %q", full, f.Kind)
	}
}
