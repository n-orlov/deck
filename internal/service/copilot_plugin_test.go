package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/agent"
)

func newCopilotTestService(t *testing.T, seed string) (svc Service, auditPath, socket, home string) {
	t.Helper()
	home = isolateAgentHome(t)
	t.Setenv("COPILOT_HOME", "")
	stubExecutableOnPath(t, "copilot")
	svc, _, logger, socket := newAgentTestService(t, nil, seed)
	svc.Agents.Register(agent.NewCopilot())
	return svc, logger.Path(), socket, home
}

func launchedArgv(t *testing.T, auditPath string) []string {
	t.Helper()
	var argv []string
	for _, record := range auditRecords(t, auditPath) {
		if record["event"] == "launch" {
			argv = jsonStrings(record["argv"])
		}
	}
	return argv
}

func assertNothingUnderCopilotHome(t *testing.T, home string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(home, ".copilot")); !os.IsNotExist(err) {
		t.Fatalf("the launch created %s/.copilot (stat err %v)", home, err)
	}
}

// R217: the launch path installs deck's plugin directory under the data root
// (mode 0700, constant content), names it with --plugin-dir, exports DECK_EXE,
// and only the yolo profile adds COPILOT_ALLOW_ALL; nothing is written under
// HOME/.copilot.
func TestCopilotLaunchInstallsThePluginDirectoryUnderTheDataRoot(t *testing.T) {
	for _, profile := range []string{"safe", "edits", "yolo"} {
		t.Run(profile, func(t *testing.T) {
			svc, auditPath, socket, home := newCopilotTestService(t, "copilot-plugin-"+profile)
			created, err := svc.CreateAgent(context.Background(), AgentCreateInput{Name: "Copilot: " + profile, CWD: t.TempDir(), Agent: "copilot", PermissionProfile: profile})
			if err != nil {
				t.Fatalf("create copilot: %v", err)
			}
			dir := agent.CopilotPluginDir(svc.DeckHome)
			argv := launchedArgv(t, auditPath)
			wantTail := []string{"--plugin-dir", dir}
			if len(argv) < 2 || strings.Join(argv[len(argv)-2:], "\x00") != strings.Join(wantTail, "\x00") {
				t.Fatalf("copilot argv = %#v, want it to end with %#v", argv, wantTail)
			}
			info, err := os.Stat(dir)
			if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
				t.Fatalf("plugin dir = %v, %v; want a 0700 directory", info, err)
			}
			for name, want := range map[string]string{"plugin.json": agent.CopilotPluginManifest, "hooks.json": agent.CopilotHooksConfig()} {
				if got, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(got) != want {
					t.Fatalf("%s = %q, %v; want the plugin constant", name, got, err)
				}
			}
			if left, _ := filepath.Glob(filepath.Join(dir, ".instrument-*")); len(left) != 0 {
				t.Fatalf("temporary files left behind: %v", left)
			}
			assertTMuxEnvironment(t, socket, created.Slug, "DECK_EXE", svc.DeckExecutable)
			if profile == "yolo" {
				assertTMuxEnvironment(t, socket, created.Slug, "COPILOT_ALLOW_ALL", "true")
			} else {
				assertTMuxEnvironmentAbsent(t, socket, created.Slug, "COPILOT_ALLOW_ALL")
			}
			if created.HookExecutable != svc.DeckExecutable {
				t.Fatalf("HookExecutable = %q, want %q", created.HookExecutable, svc.DeckExecutable)
			}
			assertNothingUnderCopilotHome(t, home)
		})
	}
}

// R217: a relaunch leaves a plugin directory whose content already matches
// untouched (same inode, same mtime), and replaces a stale file atomically.
func TestCopilotRelaunchLeavesAMatchingPluginUntouchedAndRepairsAStaleOne(t *testing.T) {
	svc, _, _, home := newCopilotTestService(t, "copilot-plugin-idem")
	created, err := svc.CreateAgent(context.Background(), AgentCreateInput{Name: "Copilot: idem", CWD: t.TempDir(), Agent: "copilot", PermissionProfile: "safe"})
	if err != nil {
		t.Fatal(err)
	}
	hooks := filepath.Join(agent.CopilotPluginDir(svc.DeckHome), "hooks.json")
	old := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(hooks, old, old); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(hooks)
	if err != nil {
		t.Fatal(err)
	}
	if _, outcome, err := svc.Restart(context.Background(), created.ID); err != nil || outcome != ResumeStarted {
		t.Fatalf("restart: %v, outcome %v", err, outcome)
	}
	after, err := os.Stat(hooks)
	if err != nil || !after.ModTime().Equal(old) || !os.SameFile(before, after) {
		t.Fatalf("a matching hooks.json was rewritten: mtime %v (want %v), same file %v, %v", after.ModTime(), old, os.SameFile(before, after), err)
	}

	if err := os.WriteFile(hooks, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, outcome, err := svc.Restart(context.Background(), created.ID); err != nil || outcome != ResumeStarted {
		t.Fatalf("second restart: %v, outcome %v", err, outcome)
	}
	if got, err := os.ReadFile(hooks); err != nil || string(got) != agent.CopilotHooksConfig() {
		t.Fatalf("stale hooks.json = %q, %v; want it replaced", got, err)
	}
	assertNothingUnderCopilotHome(t, home)
}

// writeFileAtomic holds an existing directory to the requested mode and never
// leaves a temporary file behind.
func TestWriteFileAtomicHoldsTheDirectoryModeAndKeepsMatchingContent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "plugin")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "f")
	if err := writeFileAtomic(path, []byte("one"), 0o700); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("dir = %v, %v; want mode 0700", info, err)
	}
	if err := writeFileAtomic(path, []byte("two"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "two" {
		t.Fatalf("content = %q", got)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("entries = %v, want just the file", entries)
	}
}

// R217: a plugin directory that cannot be written never fails the launch: the
// session starts with no --plugin-dir and carries a non-fatal note. Nothing is
// written under HOME/.copilot.
func TestCopilotLaunchWithAnUnwritablePluginDirectoryDegradesToNoPluginDir(t *testing.T) {
	svc, auditPath, socket, home := newCopilotTestService(t, "copilot-plugin-unwritable")
	db := svc.Store
	// A regular file where the plugin's parent directory belongs: MkdirAll
	// fails, whatever user the test runs as.
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "copilot"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	svc.DeckHome = root

	created, err := svc.CreateAgent(context.Background(), AgentCreateInput{Name: "Copilot: degraded", CWD: t.TempDir(), Agent: "copilot", PermissionProfile: "yolo"})
	if err != nil {
		t.Fatalf("create copilot with an unwritable plugin dir: %v", err)
	}
	if created.Status == "error" {
		t.Fatalf("the launch left the row in %q", created.Status)
	}
	argv := launchedArgv(t, auditPath)
	for _, arg := range argv {
		if arg == "--plugin-dir" {
			t.Fatalf("argv %#v still names --plugin-dir", argv)
		}
	}
	if len(argv) == 0 || argv[0] != "copilot" || argv[1] != "--session-id" {
		t.Fatalf("argv = %#v, want a normal copilot launch", argv)
	}
	assertTMuxEnvironment(t, socket, created.Slug, "DECK_EXE", svc.DeckExecutable)
	events, err := db.ListEvents(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	var notes []string
	for _, event := range events {
		if event.Kind == "note" && event.SessionID == created.ID {
			notes = append(notes, event.Reason)
		}
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "copilot hook instrumentation unavailable") {
		t.Fatalf("notes = %q, want one non-fatal instrumentation note", notes)
	}
	assertNothingUnderCopilotHome(t, home)
}

func TestDropArgvRun(t *testing.T) {
	got := dropArgvRun([]string{"a", "--plugin-dir", "d", "b"}, []string{"--plugin-dir", "d"})
	if strings.Join(got, " ") != "a b" {
		t.Fatalf("dropArgvRun = %v", got)
	}
	if got := dropArgvRun([]string{"a"}, []string{"x"}); len(got) != 1 {
		t.Fatalf("dropArgvRun of an absent run = %v", got)
	}
	if got := dropArgvRun([]string{"a"}, nil); len(got) != 1 {
		t.Fatalf("dropArgvRun of nothing = %v", got)
	}
}
