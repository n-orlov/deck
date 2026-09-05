package service

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
)

// TestPaneArgvFillsTheEmptyExecutableSlotWithTheResolvedShell pins the other
// half of R111's declared-executable rule, the half internal/agent cannot
// see: the `shell` adapter declares no executable and so leaves argv[0]
// empty (SPEC §5), and it is the launcher -- here, CreateAgent's paneArgv --
// that fills that slot with the one shell resolution Service owns
// (Service.Shell, else $SHELL, else /bin/sh), the same one CreateShell uses.
// Without this the pane would exec an empty argv[0] and buildPaneCommand
// would refuse the launch outright.
func TestPaneArgvFillsTheEmptyExecutableSlotWithTheResolvedShell(t *testing.T) {
	cwd := t.TempDir()
	svc, _, logger, _ := newAgentTestService(t, nil, "pane-argv-shell")
	svc.Shell = "/bin/sh"

	if _, err := svc.CreateAgent(context.Background(), AgentCreateInput{
		Name: "argv shell", CWD: cwd, Agent: "shell", LaunchArgs: []string{"-c", "sleep 2"},
	}); err != nil {
		t.Fatalf("create shell adapter session: %v", err)
	}

	var launched []string
	for _, record := range auditRecords(t, logger.Path()) {
		if record["event"] == "launch" {
			launched = jsonStrings(record["argv"])
		}
	}
	want := []string{"/bin/sh", "-c", "sleep 2"}
	if strings.Join(launched, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("audited launch argv = %q, want the resolved shell in the declared executable's slot %q", launched, want)
	}
}

// TestPaneArgvLeavesADeclaredExecutableAlone is the complement: an adapter
// that declares its own executable ("claude") keeps the argv[0] its Launch
// produced, so the shell-slot rule above cannot leak into agent launches.
func TestPaneArgvLeavesADeclaredExecutableAlone(t *testing.T) {
	svc, _, _, _ := newAgentTestService(t, nil, "pane-argv-claude")
	svc.Shell = "/bin/sh"
	adapter, ok := svc.Agents.Lookup("claude")
	if !ok {
		t.Fatal("claude adapter missing from the test registry")
	}
	argv, err := adapter.Launch(agent.LaunchInput{CWD: t.TempDir(), ConversationID: "conv-1", Profile: "safe"})
	if err != nil {
		t.Fatalf("claude Launch: %v", err)
	}
	got, err := svc.paneArgv(adapter.Capabilities(), argv)
	if err != nil {
		t.Fatalf("paneArgv: %v", err)
	}
	if strings.Join(got, "\x00") != strings.Join(argv, "\x00") {
		t.Fatalf("paneArgv rewrote a declared executable's argv: got %q, want %q unchanged", got, argv)
	}
	if got[0] != adapter.Capabilities().Executable {
		t.Fatalf("pane argv[0] = %q, want the declared executable %q", got[0], adapter.Capabilities().Executable)
	}
}
