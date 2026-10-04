package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
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
		err := filepath.WalkDir(root, func(path string, _ os.DirEntry, err error) error {
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

// TestRunRejectsExplicitEmptyProfileArgument pins R152/SPEC §3.4: an
// explicitly empty positional argument (`deck ""`) is a provided argument,
// not an absent one, so it must be validated (and rejected: 0 characters is
// below the 1-16 character rule) exactly like any other invalid name --
// exit 2, no profile-creation prompt, and nothing written to disk -- rather
// than being treated as though no argument had been given and falling
// through to DECK_PROFILE/default. Each case runs with and without
// DECK_PROFILE set, to confirm the explicit empty argument always wins over
// (and is validated ahead of) whatever DECK_PROFILE would otherwise
// resolve to -- including a DECK_PROFILE that is itself unset (empty), so
// this also pins that an empty DECK_PROFILE alone (no positional argument
// at all) still means "unset" and never reaches this validator.
func TestRunRejectsExplicitEmptyProfileArgument(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "deck ''", args: []string{""}},
		{name: "deck '' work", args: []string{"", "work"}},
		{name: "deck '' ''", args: []string{"", ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/DECK_PROFILE unset", func(t *testing.T) {
			env := tempDeckEnv(t)
			code, _, stderr := runWithEnv(t, env, tc.args...)
			if code != 2 {
				t.Fatalf("run(%s) exit = %d, want 2 (stderr=%q)", tc.name, code, stderr)
			}
			assertEmptyDirs(t, env)
		})
		t.Run(tc.name+"/DECK_PROFILE=work", func(t *testing.T) {
			env := append(tempDeckEnv(t), "DECK_PROFILE=work")
			code, _, stderr := runWithEnv(t, env, tc.args...)
			if code != 2 {
				t.Fatalf("run(%s) with DECK_PROFILE=work exit = %d, want 2 (stderr=%q)", tc.name, code, stderr)
			}
			assertEmptyDirs(t, env)
		})
	}
}

// TestValidateResolvedProfileEmptyEnvironmentIsUnset pins the documented
// empty-environment-as-unset rule this task's own regression above must not
// erode: when no positional argument was supplied at all (parseProfileArgs'
// own provided == false, never a stand-in produced by an explicitly empty
// argument), an absent or empty DECK_PROFILE must still resolve as unset --
// validateResolvedProfile returns nil, exactly as if DECK_PROFILE had never
// been set -- never the rejection an explicitly empty positional argument
// gets. This exercises validateResolvedProfile directly (the function the
// exit-2 cases above go through too) so the assertion covers the exact
// boundary the bug lived on, without launching the rest of run().
func TestValidateResolvedProfileEmptyEnvironmentIsUnset(t *testing.T) {
	for _, tc := range []struct {
		name        string
		deckProfile string
		set         bool
	}{
		{name: "DECK_PROFILE unset", set: false},
		{name: "DECK_PROFILE=''", deckProfile: "", set: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(key string) string {
				if key == "DECK_PROFILE" && tc.set {
					return tc.deckProfile
				}
				return ""
			}
			if err := validateResolvedProfile("", false, getenv); err != nil {
				t.Fatalf("validateResolvedProfile(\"\", false, ...) with %s = %v, want nil (unset)", tc.name, err)
			}
			if got := config.ResolveProfileName("", getenv); got != config.DefaultProfile {
				t.Fatalf("ResolveProfileName(\"\", ...) with %s = %q, want default %q", tc.name, got, config.DefaultProfile)
			}
		})
	}
}
