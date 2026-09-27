package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// TestSettingsSaveUnderNamedProfileTouchesOnlyThatProfile proves requirement
// 20/SPEC §3.4's per-profile isolation from the settings takeover's own
// write path: a ctrl+s save (task 012's config.WriteConfigFile) opened
// against a named profile's Settings (its Paths.ConfigFile already points
// at profiles/<name>/config.toml, per resolvePaths/profileRoot) must never
// touch the default profile's config.toml -- not its bytes, and not its
// mtime, which would move even on a byte-identical rewrite.
//
// Both files start non-empty and already parseable so the test can prove a
// real semantic change (allow_yolo flips) landed in the profile file, not
// merely that “some bytes changed”.
func TestSettingsSaveUnderNamedProfileTouchesOnlyThatProfile(t *testing.T) {
	home := t.TempDir()
	getenv := func(name string) string {
		if name == "DECK_HOME" {
			return home
		}
		return ""
	}
	userHome := func() (string, error) { return home, nil }

	defaultConfig := filepath.Join(home, "config.toml")
	profileConfig := filepath.Join(home, "profiles", "work", "config.toml")
	if err := os.MkdirAll(filepath.Dir(profileConfig), 0o755); err != nil {
		t.Fatalf("mkdir profile dir: %v", err)
	}
	const seed = "allow_yolo = false\n"
	if err := os.WriteFile(defaultConfig, []byte(seed), 0o644); err != nil {
		t.Fatalf("seed default config.toml: %v", err)
	}
	if err := os.WriteFile(profileConfig, []byte(seed), 0o644); err != nil {
		t.Fatalf("seed profile config.toml: %v", err)
	}

	defaultBefore, err := os.ReadFile(defaultConfig)
	if err != nil {
		t.Fatalf("read default config.toml before save: %v", err)
	}
	defaultInfoBefore, err := os.Stat(defaultConfig)
	if err != nil {
		t.Fatalf("stat default config.toml before save: %v", err)
	}

	loaded, err := config.LoadFromProfile(getenv, userHome, "work")
	if err != nil {
		t.Fatalf("config.LoadFromProfile: %v", err)
	}
	if loaded.Paths.ConfigFile != profileConfig {
		t.Fatalf("loaded settings' ConfigFile = %q, want the named profile's own file %q", loaded.Paths.ConfigFile, profileConfig)
	}

	m := New(nil, loaded, "")
	updated, _ := m.Update(key(","))
	m = updated.(Model)
	if !m.settingsOpen {
		t.Fatal(", did not open settings")
	}
	m.settingsFocus = settingsFocusFields
	m.settingsFieldIndex = 0

	before := m.settingsEdits.AllowYolo
	updated, _ = m.Update(key("enter"))
	m = updated.(Model)
	if m.settingsEdits.AllowYolo == before {
		t.Fatal("enter did not toggle allow_yolo before the save test could proceed")
	}

	updated, _ = m.Update(key("ctrl+s"))
	m = updated.(Model)
	if m.settingsDirty() {
		t.Fatal("a successful save left settingsDirty() true")
	}

	// The named profile's own file changed, and reflects the toggled value.
	profileAfter, err := os.ReadFile(profileConfig)
	if err != nil {
		t.Fatalf("read profile config.toml after save: %v", err)
	}
	if !strings.Contains(string(profileAfter), "allow_yolo") {
		t.Fatalf("saved profile config.toml does not mention allow_yolo:\n%s", profileAfter)
	}
	reloadedProfile, err := config.LoadFromProfile(getenv, userHome, "work")
	if err != nil {
		t.Fatalf("reload profile config.toml: %v", err)
	}
	if reloadedProfile.AllowYolo != m.settingsEdits.AllowYolo {
		t.Fatalf("reloaded profile AllowYolo = %v, want %v (the staged, saved value)", reloadedProfile.AllowYolo, m.settingsEdits.AllowYolo)
	}

	// The default profile's file must be byte-for-byte and mtime-for-mtime
	// untouched: a same-content atomic rewrite still moves mtime, so this
	// is a strictly stronger proof than a content-only comparison.
	defaultAfter, err := os.ReadFile(defaultConfig)
	if err != nil {
		t.Fatalf("read default config.toml after save: %v", err)
	}
	if string(defaultAfter) != string(defaultBefore) {
		t.Fatalf("default config.toml bytes changed by a named-profile save:\nbefore:\n%s\nafter:\n%s", defaultBefore, defaultAfter)
	}
	defaultInfoAfter, err := os.Stat(defaultConfig)
	if err != nil {
		t.Fatalf("stat default config.toml after save: %v", err)
	}
	if !defaultInfoAfter.ModTime().Equal(defaultInfoBefore.ModTime()) {
		t.Fatalf("default config.toml mtime changed by a named-profile save: before %v, after %v", defaultInfoBefore.ModTime(), defaultInfoAfter.ModTime())
	}
}
