package config

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestConfigDirectoryIsCreated0700UnderUmask022 pins R197 (#61): the
// directory holding config.toml (it can carry a notify webhook and env
// values) is created 0700 by both writers -- WriteConfigFile's atomic
// write and CreateProfile -- even under the common umask 022, which would
// have left a 0755 directory.
func TestConfigDirectoryIsCreated0700UnderUmask022(t *testing.T) {
	old := syscall.Umask(0o022)
	defer syscall.Umask(old)

	assertMode := func(t *testing.T, dir string) {
		t.Helper()
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("%s mode = %#o, want 0700", dir, got)
		}
	}

	t.Run("WriteConfigFile", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "deck")
		if err := WriteConfigFile(filepath.Join(dir, "config.toml"), FileConfig{}); err != nil {
			t.Fatal(err)
		}
		assertMode(t, dir)
	})

	t.Run("CreateProfile", func(t *testing.T) {
		root := t.TempDir()
		getenv := func(k string) string {
			switch k {
			case "XDG_DATA_HOME":
				return filepath.Join(root, "data")
			case "XDG_CONFIG_HOME":
				return filepath.Join(root, "config")
			case "XDG_STATE_HOME":
				return filepath.Join(root, "state")
			}
			return ""
		}
		userHome := func() (string, error) { return root, nil }
		if err := CreateProfile(getenv, userHome, "work"); err != nil {
			t.Fatal(err)
		}
		paths, err := resolvePaths(getenv, userHome, "work")
		if err != nil {
			t.Fatal(err)
		}
		assertMode(t, filepath.Dir(paths.ConfigFile))
	})
}
