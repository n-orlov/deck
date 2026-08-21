package config

import "testing"

// TestEnvOverridesRecordsWhichKeysEnvActuallyOverrode proves requirement
// 21's precondition: LoadFrom must expose which schema keys had a set
// environment variable win over config.toml (§6.5's "Environment always
// outranks the file"), by the same full key task 010's schema uses
// (Field.FullKey()), so settings (task 017) can label it without a
// second, hand-maintained list of "which env vars override which key".
func TestEnvOverridesRecordsWhichKeysEnvActuallyOverrode(t *testing.T) {
	dir := writeConfigFile(t, "[ui]\nascii = true\nmouse = false\n")

	// Neither DECK_ASCII nor DECK_MOUSE set: the file's own values apply
	// and neither key is an override.
	settings, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir}), fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.EnvOverrides) != 0 {
		t.Fatalf("EnvOverrides = %+v, want none when no DECK_* override is set", settings.EnvOverrides)
	}

	// DECK_ASCII set: ui.ascii is overridden and DECK_MOUSE, being unset,
	// is not -- proving the map is keyed per field, not all-or-nothing.
	settings, err = LoadFrom(environment(map[string]string{"DECK_HOME": dir, "DECK_ASCII": "false"}), fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	if settings.EnvOverrides["ui.ascii"] != "DECK_ASCII" {
		t.Fatalf("EnvOverrides[ui.ascii] = %q, want DECK_ASCII", settings.EnvOverrides["ui.ascii"])
	}
	if _, ok := settings.EnvOverrides["ui.mouse"]; ok {
		t.Fatalf("EnvOverrides = %+v, want ui.mouse absent when DECK_MOUSE is unset", settings.EnvOverrides)
	}
	if settings.ASCII {
		t.Fatal("DECK_ASCII=false should have overridden [ui] ascii = true")
	}

	// Both set: both keys are recorded.
	settings, err = LoadFrom(environment(map[string]string{"DECK_HOME": dir, "DECK_ASCII": "false", "DECK_MOUSE": "1"}), fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	if settings.EnvOverrides["ui.ascii"] != "DECK_ASCII" || settings.EnvOverrides["ui.mouse"] != "DECK_MOUSE" {
		t.Fatalf("EnvOverrides = %+v, want both ui.ascii and ui.mouse recorded", settings.EnvOverrides)
	}
	if len(settings.EnvOverrides) != 2 {
		t.Fatalf("EnvOverrides = %+v, want exactly two entries", settings.EnvOverrides)
	}
}
