package main

import (
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/update"
)

// TestUpdateCallersPassTheInstallLockPath: `deck update` and ctrl+u / the
// auto-update both hand update.Run the install lock beside the shared update
// cache, and it is not the cache's own check lock.
func TestUpdateCallersPassTheInstallLockPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DECK_HOME", home)
	cache := filepath.Join(home, config.UpdateCacheName)
	want := filepath.Join(home, update.InstallLockName)

	if got := updateCommandOptions("").InstallLock; got != want {
		t.Errorf("deck update InstallLock = %q, want %q", got, want)
	}
	var settings config.Settings
	settings.Paths.UpdateCache = cache
	got := updateInstallOptions(settings).InstallLock
	if got != want {
		t.Errorf("ctrl+u InstallLock = %q, want %q", got, want)
	}
	if got == cache+".lock" {
		t.Errorf("the install lock %q is the cache's check lock", got)
	}
}
