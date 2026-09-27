package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tempDeckEnv points every root deck's config resolution can reach --
// DECK_HOME and, in case some path ever falls back to XDG resolution, HOME
// and the three XDG_* variables too -- at fresh, distinct t.TempDir()
// directories, and never at the real environment. Every case in this file
// must exit before touching disk, so assertEmptyDirs (below) walks every
// one of these afterwards and requires zero entries.
func tempDeckEnv(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	deckHome := t.TempDir()
	xdgConfig := t.TempDir()
	xdgData := t.TempDir()
	xdgState := t.TempDir()
	return []string{
		"HOME=" + home,
		"DECK_HOME=" + deckHome,
		"XDG_CONFIG_HOME=" + xdgConfig,
		"XDG_DATA_HOME=" + xdgData,
		"XDG_STATE_HOME=" + xdgState,
	}
}

// assertEmptyDirs walks every "NAME=path" env entry produced by
// tempDeckEnv and fails if any holds so much as one entry: a temp root
// created by t.TempDir() starts empty, so anything found afterwards was
// written by the run under test, meaning it reached disk before exiting.
func assertEmptyDirs(t *testing.T, env []string) {
	t.Helper()
	rootKeys := map[string]bool{"HOME": true, "DECK_HOME": true, "XDG_CONFIG_HOME": true, "XDG_DATA_HOME": true, "XDG_STATE_HOME": true}
	for _, kv := range env {
		parts := strings.SplitN(kv, "=", 2)
		if !rootKeys[parts[0]] {
			continue
		}
		root := parts[1]
		var found []string
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path != root {
				found = append(found, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
		if len(found) != 0 {
			t.Fatalf("%s is not empty after run(): %v", parts[0], found)
		}
	}
}

func runWithEnv(t *testing.T, env []string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	saved := make(map[string]string, len(env))
	for _, kv := range env {
		parts := strings.SplitN(kv, "=", 2)
		key := parts[0]
		if old, ok := os.LookupEnv(key); ok {
			saved[key] = old
		}
		t.Setenv(key, parts[1])
	}
	_ = saved
	var outBuf, errBuf bytes.Buffer
	code = run(append([]string{"deck"}, args...), strings.NewReader(""), &outBuf, &errBuf)
	return code, outBuf.String(), errBuf.String()
}

// TestRunRejectsUnknownFlagBeforePositional pins SPEC \u00a73.4: flags are
// recognized before the single positional profile argument, so an unknown
// flag is reported and rejected as a flag -- never mistaken for (or folded
// into validating as) a profile name -- with exit 2 and nothing written to
// disk.
func TestRunRejectsUnknownFlagBeforePositional(t *testing.T) {
	env := tempDeckEnv(t)
	code, _, stderr := runWithEnv(t, env, "-x")
	if code != 2 {
		t.Fatalf("run(deck -x) exit = %d, want 2 (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stderr, "error: unknown flag -x") {
		t.Fatalf("run(deck -x) stderr = %q, want it to contain %q", stderr, "error: unknown flag -x")
	}
	assertEmptyDirs(t, env)
}

// TestRunRejectsSecondPositionalArgument pins SPEC \u00a73.4: "deck work home"
// is an error -- at most one positional argument -- with exit 2 and
// nothing written to disk.
func TestRunRejectsSecondPositionalArgument(t *testing.T) {
	env := tempDeckEnv(t)
	code, _, stderr := runWithEnv(t, env, "a", "b")
	if code != 2 {
		t.Fatalf("run(deck a b) exit = %d, want 2 (stderr=%q)", code, stderr)
	}
	assertEmptyDirs(t, env)
}

// TestRunRejectsInvalidProfileNameBeforeDisk pins SPEC \u00a73.4's one
// validator, run at cmd/deck's own entry point before config.LoadFromProfile
// (and therefore before store.Open or any tmux client) is ever reached: an
// invalid name exits 2 and touches nothing, whether it arrives as the
// positional argument or as DECK_PROFILE.
func TestRunRejectsInvalidProfileNameBeforeDisk(t *testing.T) {
	invalidNames := []string{
		"Work",              // uppercase
		"a.b",               // disallowed rune
		"abcdefghijklmnopq", // 17 characters, over the 16 limit
	}
	for _, name := range invalidNames {
		t.Run("argument/"+name, func(t *testing.T) {
			env := tempDeckEnv(t)
			code, _, stderr := runWithEnv(t, env, name)
			if code != 2 {
				t.Fatalf("run(deck %s) exit = %d, want 2 (stderr=%q)", name, code, stderr)
			}
			assertEmptyDirs(t, env)
		})
		t.Run("DECK_PROFILE/"+name, func(t *testing.T) {
			env := append(tempDeckEnv(t), "DECK_PROFILE="+name)
			code, _, stderr := runWithEnv(t, env)
			if code != 2 {
				t.Fatalf("run(deck) with DECK_PROFILE=%s exit = %d, want 2 (stderr=%q)", name, code, stderr)
			}
			assertEmptyDirs(t, env)
		})
	}
}
