package config

import "testing"

// TestBookmarksSettingDefaultFileAndEnvOverride pins R274's setting: the
// bookmarks key is a bool that is off when absent, takes the file's value,
// and is outranked by DECK_BOOKMARKS (recorded in EnvOverrides).
func TestBookmarksSettingDefaultFileAndEnvOverride(t *testing.T) {
	field, ok := FieldByFullKey("bookmarks")
	if !ok || field.Kind != KindToggle || field.Default != false {
		t.Fatalf("schema entry for bookmarks = %+v, %v; want a toggle defaulting to false", field, ok)
	}
	def, err := LoadFrom(environment(map[string]string{"DECK_HOME": t.TempDir()}), fakeHome)
	if err != nil || def.Bookmarks {
		t.Fatalf("default Settings.Bookmarks = %v, %v; want off", def.Bookmarks, err)
	}
	if _, overridden := def.EnvOverrides["bookmarks"]; overridden {
		t.Fatal("bookmarks reported as env-overridden with no DECK_BOOKMARKS set")
	}
	dirOn := writeConfigFile(t, "bookmarks = true\n")
	fromFile, err := LoadFrom(environment(map[string]string{"DECK_HOME": dirOn}), fakeHome)
	if err != nil || !fromFile.Bookmarks {
		t.Fatalf("bookmarks = true in the file gave Settings.Bookmarks = %v, %v", fromFile.Bookmarks, err)
	}
	if _, err := loadEventHookBody(t, "bookmarks = maybe\n"); err == nil {
		t.Fatal("a non-boolean bookmarks was accepted")
	}
	env := environment(map[string]string{"DECK_HOME": dirOn, "DECK_BOOKMARKS": "0"})
	off, err := LoadFrom(env, fakeHome)
	if err != nil || off.Bookmarks {
		t.Fatalf("DECK_BOOKMARKS=0 over bookmarks = true gave %v, %v; want off", off.Bookmarks, err)
	}
	if got := off.EnvOverrides["bookmarks"]; got != "DECK_BOOKMARKS" {
		t.Fatalf("EnvOverrides[bookmarks] = %q, want DECK_BOOKMARKS", got)
	}
	on, err := LoadFrom(environment(map[string]string{"DECK_HOME": t.TempDir(), "DECK_BOOKMARKS": "1"}), fakeHome)
	if err != nil || !on.Bookmarks {
		t.Fatalf("DECK_BOOKMARKS=1 over the default gave %v, %v; want on", on.Bookmarks, err)
	}
	if _, err := LoadFrom(environment(map[string]string{"DECK_HOME": dirOn, "DECK_BOOKMARKS": "sometimes"}), fakeHome); err == nil {
		t.Fatal("DECK_BOOKMARKS=sometimes was accepted")
	}
}
