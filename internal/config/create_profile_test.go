package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// xdgEnv is a getenv rooted entirely under root (no DECK_HOME), so each
// directory CreateProfile makes can be pre-empted independently.
func xdgEnv(root string) func(string) string {
	return func(k string) string {
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
}

func homeOf(root string) func() (string, error) {
	return func() (string, error) { return root, nil }
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCreateProfileMakesDirsAndCopiesTheDefaultConfigOnce(t *testing.T) {
	root := t.TempDir()
	getenv, home := xdgEnv(root), homeOf(root)
	def, err := resolvePaths(getenv, home, DefaultProfile)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, def.ConfigFile, "color = false\n")

	if err := CreateProfile(getenv, home, "work"); err != nil {
		t.Fatal(err)
	}
	paths, err := resolvePaths(getenv, home, "work")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{paths.DataDir, filepath.Dir(paths.ConfigFile), paths.LogDir} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("profile directory %s missing (err %v)", dir, err)
		}
	}
	got, err := os.ReadFile(paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "color = false\n" {
		t.Fatalf("copied config = %q, want the default profile's bytes", got)
	}
	info, err := os.Stat(paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("copied config mode = %#o, want 0600", info.Mode().Perm())
	}

	// The two files are independent from then on.
	writeFile(t, def.ConfigFile, "color = true\n")
	got, _ = os.ReadFile(paths.ConfigFile)
	if string(got) != "color = false\n" {
		t.Fatalf("changing the default config changed the profile's copy: %q", got)
	}
}

func TestCreateProfileWithoutADefaultConfigLeavesTheProfileWithNone(t *testing.T) {
	root := t.TempDir()
	getenv, home := xdgEnv(root), homeOf(root)
	if err := CreateProfile(getenv, home, "work"); err != nil {
		t.Fatalf("a missing default config must not be an error: %v", err)
	}
	paths, _ := resolvePaths(getenv, home, "work")
	if _, err := os.Stat(paths.LogDir); err != nil {
		t.Fatalf("log dir not created: %v", err)
	}
	if _, err := os.Stat(paths.ConfigFile); !os.IsNotExist(err) {
		t.Fatalf("profile got a config.toml with no default to copy (stat err %v)", err)
	}
}

func TestCreateProfileKeepsAnExistingProfileConfigUntouched(t *testing.T) {
	root := t.TempDir()
	getenv, home := xdgEnv(root), homeOf(root)
	def, _ := resolvePaths(getenv, home, DefaultProfile)
	writeFile(t, def.ConfigFile, "from = \"default\"\n")
	paths, _ := resolvePaths(getenv, home, "work")
	writeFile(t, paths.ConfigFile, "from = \"edited\"\n")

	if err := CreateProfile(getenv, home, "work"); err != nil {
		t.Fatalf("a racing second create must succeed silently: %v", err)
	}
	got, _ := os.ReadFile(paths.ConfigFile)
	if string(got) != "from = \"edited\"\n" {
		t.Fatalf("existing profile config was clobbered: %q", got)
	}
}

func TestCreateProfileReportsEachFailureNamingTheStep(t *testing.T) {
	t.Run("home directory cannot be resolved", func(t *testing.T) {
		boom := errors.New("no home")
		err := CreateProfile(func(string) string { return "" }, func() (string, error) { return "", boom }, "work")
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want it to wrap the userHome failure", err)
		}
	})

	// blocked pre-creates a regular file where a directory must go.
	blocked := func(t *testing.T, rel func(root string) string) error {
		t.Helper()
		root := t.TempDir()
		writeFile(t, rel(root), "in the way")
		return CreateProfile(xdgEnv(root), homeOf(root), "work")
	}

	t.Run("data directory", func(t *testing.T) {
		err := blocked(t, func(root string) string { return filepath.Join(root, "data", "deck") })
		if err == nil || !strings.Contains(err.Error(), `profile "work" data directory`) {
			t.Fatalf("err = %v, want a data-directory failure", err)
		}
	})
	t.Run("config directory", func(t *testing.T) {
		err := blocked(t, func(root string) string { return filepath.Join(root, "config", "deck") })
		if err == nil || !strings.Contains(err.Error(), `profile "work" config directory`) {
			t.Fatalf("err = %v, want a config-directory failure", err)
		}
	})
	t.Run("log directory", func(t *testing.T) {
		err := blocked(t, func(root string) string { return filepath.Join(root, "state", "deck") })
		if err == nil || !strings.Contains(err.Error(), `profile "work" log directory`) {
			t.Fatalf("err = %v, want a log-directory failure", err)
		}
	})
	t.Run("default config unreadable", func(t *testing.T) {
		root := t.TempDir()
		def, _ := resolvePaths(xdgEnv(root), homeOf(root), DefaultProfile)
		if err := os.MkdirAll(def.ConfigFile, 0o755); err != nil { // a directory where the file belongs
			t.Fatal(err)
		}
		err := CreateProfile(xdgEnv(root), homeOf(root), "work")
		if err == nil || !strings.Contains(err.Error(), "read default profile config") {
			t.Fatalf("err = %v, want a read-default-config failure", err)
		}
	})
}
