package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

func TestIsApplyNameRequest(t *testing.T) {
	for args, want := range map[string]bool{
		"deck _apply_name s1":   true,
		"deck _apply_name":      false,
		"deck _hook s1":         false,
		"deck _apply_name a b":  false,
		"deck new _apply_name1": false,
	} {
		if got := isApplyNameRequest(strings.Fields(args)); got != want {
			t.Errorf("isApplyNameRequest(%q) = %v, want %v", args, got, want)
		}
	}
}

// deckNewEnv is a throwaway deck home with a private tmux socket and a fake
// agent on PATH that just stays alive.
func deckNewEnv(t *testing.T, agents ...string) []string {
	t.Helper()
	bin := t.TempDir()
	for _, name := range agents {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	env := tempDeckEnv(t)
	socket := "priv-apn-" + strings.ReplaceAll(filepath.Base(deckEnvValue(t, env, "XDG_STATE_HOME")), "_", "")
	t.Cleanup(func() { killTmuxServer(socket) })
	return append(env, "DECK_TMUX_SOCKET="+socket, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// R257: `deck new` starts the detached applier for exactly the sessions whose
// harness takes its explicit name from the keyboard once ready, and for no
// other: a derived (blank) name and a launch-flag harness need none.
func TestDeckNewStartsTheNameApplierOnlyWhereTheNameIsTyped(t *testing.T) {
	env := deckNewEnv(t, "codex", "copilot")
	var started []string
	saved := startNameApplier
	startNameApplier = func(id string) error { started = append(started, id); return nil }
	defer func() { startNameApplier = saved }()
	cwd := t.TempDir()
	cases := []struct {
		args    []string
		applier bool
	}{
		{[]string{"new", "--agent", "codex", "--name", "typed-codex", "--cwd", cwd}, true},
		{[]string{"new", "--agent", "codex", "--cwd", cwd}, false},
		{[]string{"new", "--agent", "copilot", "--name", "typed-copilot", "--cwd", cwd}, false},
	}
	for _, tc := range cases {
		before := len(started)
		if code, _, stderr := runWithEnv(t, env, tc.args...); code != 0 {
			t.Fatalf("%v: exit %d %q", tc.args, code, stderr)
		}
		if got := len(started) - before; (got == 1) != tc.applier || got > 1 {
			t.Errorf("%v: applier started %d times, want it started=%v", tc.args, got, tc.applier)
		}
	}
}

// A refused spawn never fails the creation: the session exists under the name
// given, and the failure is on the row as harness_rename_failed (R257) as well
// as in the audit log.
func TestDeckNewSurvivesAnApplierThatCannotStart(t *testing.T) {
	env := deckNewEnv(t, "codex")
	saved := startNameApplier
	startNameApplier = func(string) error { return errors.New("no fork") }
	defer func() { startNameApplier = saved }()
	code, stdout, stderr := runWithEnv(t, env, "new", "--agent", "codex", "--name", "survives", "--cwd", t.TempDir())
	if code != 0 || strings.TrimSpace(stdout) != "survives" {
		t.Fatalf("exit %d stdout %q stderr %q", code, stdout, stderr)
	}
	assertLaunchRenameFailed(t, env, "survives", "no fork")
}

// The same rule at the real spawn boundary and for a second name: an
// executable that cannot be started is reported on that row too, each row its
// own event, and a derived-name row (nothing to type) gets none.
func TestDeckNewReportsEachUnstartableApplierOnItsOwnRow(t *testing.T) {
	env := deckNewEnv(t, "codex")
	savedExe := applierExecutable
	defer func() { applierExecutable = savedExe }()
	applierExecutable = func() (string, error) { return filepath.Join(t.TempDir(), "absent"), nil }
	cwd := t.TempDir()
	for _, name := range []string{"first typed", "second typed"} {
		if code, _, stderr := runWithEnv(t, env, "new", "--agent", "codex", "--name", name, "--cwd", cwd); code != 0 {
			t.Fatalf("%s: exit %d %q", name, code, stderr)
		}
		assertLaunchRenameFailed(t, env, name, "absent")
	}
	if code, _, stderr := runWithEnv(t, env, "new", "--agent", "codex", "--cwd", cwd); code != 0 {
		t.Fatalf("derived: exit %d %q", code, stderr)
	}
	db, _ := openEnvStore(t, env)
	defer func() { _ = db.Close() }()
	sessions, err := db.ListSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	failed := 0
	for _, row := range sessions {
		if _, found, _ := db.LastSessionEventReason(context.Background(), row.ID, "harness_rename_failed"); found {
			failed++
		}
	}
	if failed != 2 {
		t.Errorf("rows with harness_rename_failed = %d, want exactly the 2 typed-name rows", failed)
	}
}

// openEnvStore opens the store of the throwaway deck home env describes.
func openEnvStore(t *testing.T, env []string) (*store.Store, service.Service) {
	t.Helper()
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
	settings, err := config.LoadFromProfile(os.Getenv, os.UserHomeDir, "")
	if err != nil {
		t.Fatal(err)
	}
	db, sessions, err := openDetachedService(settings, newAgentRegistry())
	if err != nil {
		t.Fatal(err)
	}
	return db, sessions
}

// assertLaunchRenameFailed requires the row named name to still exist under
// that name with a harness_rename_failed event whose reason carries the cause.
func assertLaunchRenameFailed(t *testing.T, env []string, name, cause string) {
	t.Helper()
	db, _ := openEnvStore(t, env)
	defer func() { _ = db.Close() }()
	sessions, err := db.ListSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range sessions {
		if row.Name != name {
			continue
		}
		reason, found, err := db.LastSessionEventReason(context.Background(), row.ID, "harness_rename_failed")
		if err != nil || !found {
			t.Fatalf("%q: no harness_rename_failed event (found=%v err=%v)", name, found, err)
		}
		if !strings.HasPrefix(reason, "launch name not sent:") || !strings.Contains(reason, cause) {
			t.Errorf("%q: reason %q, want launch name not sent: ... %s", name, reason, cause)
		}
		return
	}
	t.Fatalf("no row named %q survived", name)
}

func TestSpawnNameApplierStartsTheExecutableWithTheVerb(t *testing.T) {
	saved := applierExecutable
	defer func() { applierExecutable = saved }()
	applierExecutable = func() (string, error) { return "/bin/true", nil }
	if err := spawnNameApplier("s1"); err != nil {
		t.Errorf("spawn /bin/true: %v", err)
	}
	applierExecutable = func() (string, error) { return filepath.Join(t.TempDir(), "absent"), nil }
	if err := spawnNameApplier("s1"); err == nil {
		t.Error("a binary that cannot start must be an error")
	}
	applierExecutable = func() (string, error) { return "", errors.New("no path") }
	if err := spawnNameApplier("s1"); err == nil {
		t.Error("an unresolvable executable must be an error")
	}
}

// The hidden verb reports through its exit code: 0 when the row needs nothing,
// 1 with the reason when it cannot.
func TestRunApplyNameCommand(t *testing.T) {
	env := deckNewEnv(t, "codex")
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
	settings, err := config.LoadFromProfile(os.Getenv, os.UserHomeDir, "")
	if err != nil {
		t.Fatal(err)
	}
	db, sessions, err := openDetachedService(settings, newAgentRegistry())
	if err != nil {
		t.Fatal(err)
	}
	row, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "derived1", Name: "d", CWD: t.TempDir(), Agent: "codex", CapturedPath: "/bin",
		Status: "starting", StatusSource: "probe", StatusAt: 1, CreatedAt: 1, NameSource: "derived",
	})
	if err != nil || sessions.NeedsLaunchNameApply(row) {
		t.Fatalf("derived row: %v, needs applier %v", err, sessions.NeedsLaunchNameApply(row))
	}
	_ = db.Close()
	var stderr bytes.Buffer
	if code := runApplyNameCommand([]string{"deck", applyNameVerb, "derived1"}, strings.NewReader(""), &stderr); code != 0 {
		t.Errorf("derived row: exit %d %q", code, stderr.String())
	}
	stderr.Reset()
	if code := runApplyNameCommand([]string{"deck", applyNameVerb, "nosuch"}, strings.NewReader(""), &stderr); code != 1 || stderr.Len() == 0 {
		t.Errorf("missing row: exit %d %q", code, stderr.String())
	}
}

func TestRunDetachedVerbOnlyHandlesItsOwnVerbs(t *testing.T) {
	var out, errb bytes.Buffer
	if _, handled := runDetachedVerb([]string{"deck", "work"}, strings.NewReader(""), &out, &errb); handled {
		t.Error("a profile name is not a detached verb")
	}
	if code, handled := runDetachedVerb([]string{"deck", "new", "--bogus"}, strings.NewReader(""), &out, &errb); !handled || code != 2 {
		t.Errorf("deck new --bogus: code %d handled %v", code, handled)
	}
}
