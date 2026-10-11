package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAdoptHarnessNamesSettingDefaultsOnAndRoundTrips pins adopt_harness_names
// (SPEC §11.4): on when absent, off when the file says so, carried to the
// running Settings, and written back verbatim.
func TestAdoptHarnessNamesSettingDefaultsOnAndRoundTrips(t *testing.T) {
	absent, err := loadEventHookBody(t, "")
	if err != nil || !absent.AdoptHarnessNames {
		t.Fatalf("absent adopt_harness_names = %v, %v; want on", absent.AdoptHarnessNames, err)
	}
	off, err := loadEventHookBody(t, "adopt_harness_names = false\n")
	if err != nil || off.AdoptHarnessNames {
		t.Fatalf("adopt_harness_names = false parsed as %v, %v", off.AdoptHarnessNames, err)
	}
	if _, err := loadEventHookBody(t, "adopt_harness_names = maybe\n"); err == nil {
		t.Fatal("a non-boolean adopt_harness_names was accepted")
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := WriteConfigFile(path, off); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "adopt_harness_names = false") {
		t.Fatalf("written config.toml lacks the key:\n%s", raw)
	}
	set, err := LoadFrom(environment(map[string]string{"DECK_HOME": writeConfigFile(t, "adopt_harness_names = false\n")}), fakeHome)
	if err != nil || set.AdoptHarnessNames {
		t.Fatalf("Settings.AdoptHarnessNames = %v, %v; want off", set.AdoptHarnessNames, err)
	}
	def, err := LoadFrom(environment(map[string]string{"DECK_HOME": t.TempDir()}), fakeHome)
	if err != nil || !def.AdoptHarnessNames {
		t.Fatalf("default Settings.AdoptHarnessNames = %v, %v; want on", def.AdoptHarnessNames, err)
	}
}
