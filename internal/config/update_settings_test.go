package config

import (
	"bytes"
	"os"
	"testing"
)

// TestUpdateSettingsDefaults pins R297's two global keys: update_check is a
// toggle that is on by default and auto_update a toggle that is off by
// default; the file's value is taken; DECK_UPDATE_CHECK and DECK_AUTO_UPDATE
// outrank the file, are recorded in EnvOverrides and reject a non-boolean.
func TestUpdateSettingsDefaults(t *testing.T) {
	check, ok := FieldByFullKey("update_check")
	if !ok || check.Kind != KindToggle || check.Default != true || check.Scope != ScopeGlobal {
		t.Fatalf("schema entry for update_check = %+v, %v; want a global toggle defaulting to true", check, ok)
	}
	auto, ok := FieldByFullKey("auto_update")
	if !ok || auto.Kind != KindToggle || auto.Default != false || auto.Scope != ScopeGlobal {
		t.Fatalf("schema entry for auto_update = %+v, %v; want a global toggle defaulting to false", auto, ok)
	}
	def, err := LoadFrom(environment(map[string]string{"DECK_HOME": t.TempDir()}), fakeHome)
	if err != nil || !def.UpdateCheck || def.AutoUpdate {
		t.Fatalf("defaults: UpdateCheck=%v AutoUpdate=%v err=%v; want true, false", def.UpdateCheck, def.AutoUpdate, err)
	}
	for _, key := range []string{"update_check", "auto_update"} {
		if _, overridden := def.EnvOverrides[key]; overridden {
			t.Fatalf("%s reported as env-overridden with no variable set", key)
		}
	}
	dir := writeConfigFile(t, "update_check = false\nauto_update = true\n")
	fromFile, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir}), fakeHome)
	if err != nil || fromFile.UpdateCheck || !fromFile.AutoUpdate {
		t.Fatalf("file values: UpdateCheck=%v AutoUpdate=%v err=%v; want false, true", fromFile.UpdateCheck, fromFile.AutoUpdate, err)
	}
	over, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir, "DECK_UPDATE_CHECK": "1", "DECK_AUTO_UPDATE": "0"}), fakeHome)
	if err != nil || !over.UpdateCheck || over.AutoUpdate {
		t.Fatalf("env over file: UpdateCheck=%v AutoUpdate=%v err=%v; want true, false", over.UpdateCheck, over.AutoUpdate, err)
	}
	if over.EnvOverrides["update_check"] != "DECK_UPDATE_CHECK" || over.EnvOverrides["auto_update"] != "DECK_AUTO_UPDATE" {
		t.Fatalf("EnvOverrides = %v; want both keys naming their variables", over.EnvOverrides)
	}
	if over.File.UpdateCheck || !over.File.AutoUpdate {
		t.Fatalf("Settings.File must keep the file's own values under an override, got %+v", over.File)
	}
	for _, name := range []string{"DECK_UPDATE_CHECK", "DECK_AUTO_UPDATE"} {
		if _, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir, name: "sometimes"}), fakeHome); err == nil {
			t.Fatalf("%s=sometimes was accepted", name)
		}
	}
	if _, err := loadEventHookBody(t, "update_check = maybe\n"); err == nil {
		t.Fatal("a non-boolean update_check was accepted")
	}
}

// TestAutoUpdateImpliesCheck pins "auto_update implies the check": the check
// runs when update_check is on or auto_update is on, and only a settings
// pair with both off (or DECK_UPDATE_CHECK=0 without auto_update) disables it.
func TestAutoUpdateImpliesCheck(t *testing.T) {
	rows := []struct {
		name string
		file string
		env  map[string]string
		want bool
	}{
		{"defaults", "", nil, true},
		{"check off", "update_check = false\n", nil, false},
		{"check off, auto on", "update_check = false\nauto_update = true\n", nil, true},
		{"auto on alone", "auto_update = true\n", nil, true},
		{"env check off", "", map[string]string{"DECK_UPDATE_CHECK": "0"}, false},
		{"env check off, env auto on", "", map[string]string{"DECK_UPDATE_CHECK": "0", "DECK_AUTO_UPDATE": "1"}, true},
	}
	for _, r := range rows {
		env := map[string]string{"DECK_HOME": writeConfigFile(t, r.file)}
		for k, v := range r.env {
			env[k] = v
		}
		s, err := LoadFrom(environment(env), fakeHome)
		if err != nil {
			t.Fatalf("%s: %v", r.name, err)
		}
		if got := s.UpdateCheckEnabled(); got != r.want {
			t.Errorf("%s: UpdateCheckEnabled() = %v, want %v (UpdateCheck=%v AutoUpdate=%v)", r.name, got, r.want, s.UpdateCheck, s.AutoUpdate)
		}
	}
}

// TestUpdateSettingsLiveApply: both keys are ScopeGlobal, so a reload
// applies them to the running client without a restart, exactly like
// ui.attach_on_click: a poll that sees an edited config.toml returns the new
// values, and an env override still outranks the edit.
func TestUpdateSettingsLiveApply(t *testing.T) {
	for _, key := range []string{"update_check", "auto_update"} {
		f, _ := FieldByFullKey(key)
		if f.Scope != ScopeGlobal {
			t.Fatalf("%s scope = %v, want ScopeGlobal (live)", key, f.Scope)
		}
	}
	f := newReloadFixture(t)
	f.write(f.config, "update_check = false\nauto_update = true\n")
	added, s, changed := f.poll()
	if added != 1 || !changed || s.UpdateCheck || !s.AutoUpdate {
		t.Fatalf("reload: loads=%d changed=%v UpdateCheck=%v AutoUpdate=%v; want the edit applied", added, changed, s.UpdateCheck, s.AutoUpdate)
	}
	env := environment(map[string]string{"DECK_HOME": f.home, "DECK_UPDATE_CHECK": "1", "DECK_AUTO_UPDATE": "0"})
	pinned, err := LoadFrom(env, fakeHome)
	if err != nil || !pinned.UpdateCheck || pinned.AutoUpdate {
		t.Fatalf("env-pinned reload: UpdateCheck=%v AutoUpdate=%v err=%v; the environment must outrank the edit", pinned.UpdateCheck, pinned.AutoUpdate, err)
	}
}

// TestUpdateSettingsReloadWritesNothing: a reload that changes the update
// keys leaves config.toml's bytes and mtime alone and creates no cache file.
func TestUpdateSettingsReloadWritesNothing(t *testing.T) {
	f := newReloadFixture(t)
	f.write(f.config, "update_check = false\nauto_update = true\n")
	before, _ := os.ReadFile(f.config)
	infoBefore, _ := os.Stat(f.config)
	if added, _, changed := f.poll(); added != 1 || !changed {
		t.Fatalf("setup: the poll must reload (loads=%d changed=%v)", added, changed)
	}
	after, _ := os.ReadFile(f.config)
	infoAfter, _ := os.Stat(f.config)
	if !bytes.Equal(before, after) || !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Fatal("a reload changed config.toml")
	}
	if _, err := os.Stat(f.home + "/" + UpdateCacheName); err == nil {
		t.Fatal("a reload created the update-check cache")
	}
}

// TestUpdateCachePathIsSharedAndOutsideStateDB: every profile resolves the
// same cache file, and it is never a profile's state.db.
func TestUpdateCachePathIsSharedAndOutsideStateDB(t *testing.T) {
	for _, env := range []map[string]string{
		{"DECK_HOME": "/tmp/scenario"},
		{"XDG_DATA_HOME": "/data", "XDG_CACHE_HOME": "/cache"},
		{},
	} {
		def, err := LoadFromProfile(environment(env), fakeHome, "")
		if err != nil {
			t.Fatal(err)
		}
		work, err := LoadFromProfile(environment(env), fakeHome, "work")
		if err != nil {
			t.Fatal(err)
		}
		if def.Paths.UpdateCache == "" || def.Paths.UpdateCache != work.Paths.UpdateCache {
			t.Fatalf("env %v: caches %q vs %q; want one shared path", env, def.Paths.UpdateCache, work.Paths.UpdateCache)
		}
		if def.Paths.UpdateCache == def.Paths.StateDB || def.Paths.UpdateCache == work.Paths.StateDB {
			t.Fatalf("env %v: the cache is a state.db", env)
		}
	}
}
