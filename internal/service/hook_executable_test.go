package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
)

// TestLaunchesRecordTheHookExecutableTheAgentIsBoundTo is R204c's persistence
// leg: the deck binary a launch's hook command names is stored on the row, by
// the create and again by a restart under a different binary, so the TUI can
// tell a session still bound to an older deck from one that is current.
func TestLaunchesRecordTheHookExecutableTheAgentIsBoundTo(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
	service, db, _, _ := newAgentTestService(t, nil, "hook-executable")

	created, err := service.CreateAgent(context.Background(), AgentCreateInput{Name: "Claude: bound", CWD: cwd, Agent: "claude", PermissionProfile: "safe"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	original := service.DeckExecutable
	if created.HookExecutable != original {
		t.Fatalf("created row HookExecutable = %q, want %q", created.HookExecutable, original)
	}
	row, err := db.GetSession(context.Background(), created.ID)
	if err != nil || row.HookExecutable != original {
		t.Fatalf("stored HookExecutable = %q, %v; want %q", row.HookExecutable, err, original)
	}

	// The same session restarted by a different deck binary: the pane is
	// relaunched with that binary's hook command, and the row says so.
	upgraded := service
	upgraded.DeckExecutable = original + "-v2"
	restarted, outcome, err := upgraded.Restart(context.Background(), created.ID)
	if err != nil || outcome != ResumeStarted {
		t.Fatalf("restart: %v, outcome %v", err, outcome)
	}
	if restarted.HookExecutable != upgraded.DeckExecutable {
		t.Fatalf("restarted row HookExecutable = %q, want %q", restarted.HookExecutable, upgraded.DeckExecutable)
	}
	row, err = db.GetSession(context.Background(), created.ID)
	if err != nil || row.HookExecutable != upgraded.DeckExecutable {
		t.Fatalf("stored HookExecutable after restart = %q, %v; want %q", row.HookExecutable, err, upgraded.DeckExecutable)
	}
	if row.Status == "error" {
		t.Fatalf("restart left the row in %q", row.Status)
	}
}

// TestCodexAndPiRecordAHookExecutableAndShellDoesNot: Codex and Pi instrument
// hooks like Claude and so have a binding to go stale; a shell launches no
// hook command, so it records nothing and can never show the hint (R204c, and
// R204.4 for Pi).
func TestCodexAndPiRecordAHookExecutableAndShellDoesNot(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "codex")
	stubExecutableOnPath(t, "pi")
	service, _, _, _ := newAgentTestService(t, nil, "hook-executable-others")
	service.Shell = "/bin/sh"

	for _, kind := range []string{"codex", "pi"} {
		created, err := service.CreateAgent(context.Background(), AgentCreateInput{Name: kind + ": bound", CWD: cwd, Agent: kind, PermissionProfile: "safe"})
		if err != nil {
			t.Fatalf("create %s: %v", kind, err)
		}
		if created.HookExecutable != service.DeckExecutable {
			t.Fatalf("%s HookExecutable = %q, want %q", kind, created.HookExecutable, service.DeckExecutable)
		}
	}
	shell, err := service.CreateShell(context.Background(), ShellCreateInput{Name: "Shell: unbound", CWD: cwd})
	if err != nil {
		t.Fatalf("create shell: %v", err)
	}
	if shell.HookExecutable != "" {
		t.Fatalf("shell HookExecutable = %q, want none (a shell launches no hook command)", shell.HookExecutable)
	}
}

// TestPiLaunchInstallsTheHookCommandOfTheLaunchingBinary is R204.4's launch
// read for Pi, asserted against what the launch really produced: the audited
// argv names the deck-owned extension with -e, the extension file holds the
// agent package's source, the pane environment carries the hook command of
// the launching deck binary (and the launch generation on a lease-taking
// resume), and the row records that same binary.
func TestPiLaunchInstallsTheHookCommandOfTheLaunchingBinary(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "pi")
	service, db, logger, socket := newAgentTestService(t, nil, "hook-executable-pi")
	wantCommand := service.DeckExecutable
	wantExtension := agent.PiExtensionPath(service.DeckHome)

	created, err := service.CreateAgent(context.Background(), AgentCreateInput{Name: "Pi: launch", CWD: cwd, Agent: "pi", PermissionProfile: "safe"})
	if err != nil {
		t.Fatalf("create pi: %v", err)
	}
	var argv []string
	for _, record := range auditRecords(t, logger.Path()) {
		if record["event"] == "launch" {
			argv = jsonStrings(record["argv"])
		}
	}
	wantArgv := []string{"pi", "--session-id", created.ConversationID, "-e", wantExtension}
	if strings.Join(argv, "\x00") != strings.Join(wantArgv, "\x00") {
		t.Fatalf("pi launch argv = %#v, want %#v", argv, wantArgv)
	}
	source, err := os.ReadFile(wantExtension)
	if err != nil || string(source) != agent.PiExtensionSource {
		t.Fatalf("installed extension = %v, matches source: %v", err, string(source) == agent.PiExtensionSource)
	}
	assertTMuxEnvironment(t, socket, created.Slug, agent.PiHookExecutableEnv, wantCommand)
	assertTMuxEnvironmentAbsent(t, socket, created.Slug, "DECK_LAUNCH_GENERATION")
	if created.HookExecutable != service.DeckExecutable {
		t.Fatalf("pi HookExecutable = %q, want %q", created.HookExecutable, service.DeckExecutable)
	}

	// A resume by a different binary: stale extension bytes are replaced, the
	// hook command names the new binary, the lease generation travels with it.
	if err := os.WriteFile(wantExtension, []byte("// stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := service.TMux.Kill(context.Background(), created.Slug); err != nil {
		t.Fatalf("kill pane: %v", err)
	}
	stopSession(t, db, created.ID)
	upgraded := service
	upgraded.DeckExecutable = service.DeckExecutable + "-v2"
	resumed, outcome, err := upgraded.Resume(context.Background(), created.ID)
	if err != nil || outcome != ResumeStarted {
		t.Fatalf("resume pi: %v, outcome %v", err, outcome)
	}
	assertTMuxEnvironment(t, socket, resumed.Slug, agent.PiHookExecutableEnv, upgraded.DeckExecutable)
	assertTMuxEnvironment(t, socket, resumed.Slug, "DECK_LAUNCH_GENERATION", rowLaunchGeneration(t, db, created.ID))
	if source, err := os.ReadFile(wantExtension); err != nil || string(source) != agent.PiExtensionSource {
		t.Fatalf("extension after resume = %q, %v; want the agent package's source", source, err)
	}
	if resumed.HookExecutable != upgraded.DeckExecutable {
		t.Fatalf("resumed pi HookExecutable = %q, want %q", resumed.HookExecutable, upgraded.DeckExecutable)
	}
}

// TestRestartRebindsCodexAndPi is R204c's persistence leg for the other two
// harnesses: a Codex or Pi session restarted by a different deck binary
// records that binary (its hook command is rebuilt from it) and so stops
// being reported as bound to the old one.
func TestRestartRebindsCodexAndPi(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "codex")
	stubExecutableOnPath(t, "pi")
	service, db, _, _ := newAgentTestService(t, nil, "hook-executable-restart")
	upgraded := service
	upgraded.DeckExecutable = service.DeckExecutable + "-v2"

	for _, kind := range []string{"codex", "pi"} {
		created, err := service.CreateAgent(context.Background(), AgentCreateInput{Name: kind + ": restart", CWD: cwd, Agent: kind, PermissionProfile: "safe"})
		if err != nil {
			t.Fatalf("create %s: %v", kind, err)
		}
		if created.HookExecutable != service.DeckExecutable {
			t.Fatalf("%s bound to %q before the restart, want %q", kind, created.HookExecutable, service.DeckExecutable)
		}
		// Codex learns its conversation id from its first SessionStart hook; a
		// restart needs it, so stand in for that hook here.
		if created.ConversationID == "" {
			if err := db.SetConversationID(context.Background(), created.ID, "conversation-"+kind, "hook", 1); err != nil {
				t.Fatalf("set conversation id: %v", err)
			}
		}
		restarted, outcome, err := upgraded.Restart(context.Background(), created.ID)
		if err != nil || outcome != ResumeStarted {
			t.Fatalf("restart %s: %v, outcome %v", kind, err, outcome)
		}
		row, err := db.GetSession(context.Background(), created.ID)
		if err != nil || row.HookExecutable != upgraded.DeckExecutable || restarted.HookExecutable != upgraded.DeckExecutable {
			t.Fatalf("%s HookExecutable after restart = %q (row %q), %v; want %q", kind, restarted.HookExecutable, row.HookExecutable, err, upgraded.DeckExecutable)
		}
	}
}

// A Pi launch whose extension cannot be installed fails before any pane
// starts, naming the adapter, for each way the install can fail.
func TestWriteInstrumentFilesReportsEveryInstallFailure(t *testing.T) {
	input := func(home string) agent.LaunchInput { return agent.LaunchInput{DeckHome: home} }

	// The data root is a regular file: the extension directory cannot be made.
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// The extension's own path is a directory: the final rename cannot replace it.
	renameHome := t.TempDir()
	if err := os.MkdirAll(agent.PiExtensionPath(renameHome), 0o750); err != nil {
		t.Fatal(err)
	}
	// The extension directory is read-only: no temporary file can be created.
	readOnlyHome := t.TempDir()
	dir := filepath.Dir(agent.PiExtensionPath(readOnlyHome))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	for name, home := range map[string]string{"data root is a file": file, "extension path is a directory": renameHome, "extension directory is read-only": readOnlyHome} {
		err := writeInstrumentFiles(agent.NewPi(), input(home))
		if err == nil || !strings.Contains(err.Error(), "install pi instrumentation") {
			t.Errorf("%s: err = %v, want an install pi instrumentation error", name, err)
		}
	}
	if os.Geteuid() != 0 {
		if leftovers, _ := filepath.Glob(filepath.Join(renameHome, "pi", ".instrument-*")); len(leftovers) != 0 {
			t.Errorf("a failed install left temporary files behind: %v", leftovers)
		}
	}
}

// An adapter with no files to install (Claude, Codex, shell) writes nothing.
func TestWriteInstrumentFilesIgnoresAdaptersWithoutFiles(t *testing.T) {
	home := t.TempDir()
	for _, adapter := range []agent.Adapter{agent.NewClaude(), agent.NewCodex(), agent.NewShell()} {
		if err := writeInstrumentFiles(adapter, agent.LaunchInput{DeckHome: home}); err != nil {
			t.Fatalf("%s: %v", adapter.Kind(), err)
		}
	}
	if entries, _ := os.ReadDir(home); len(entries) != 0 {
		t.Fatalf("a file-less adapter wrote %d entries under the data root", len(entries))
	}
}
