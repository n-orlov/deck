package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

func newAgentTestService(t *testing.T, configEnv map[string]string, idSeed string) (Service, *store.Store, *audit.Logger, string) {
	t.Helper()
	home := t.TempDir()
	clock, err := config.NewClock("2025-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
	paths := config.Paths{Home: home, LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")}
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	logger, err := audit.New(paths, clock)
	if err != nil {
		t.Fatal(err)
	}
	socket := "deck-agent-" + strings.ReplaceAll(time.Now().Format("150405.000000000"), ".", "")
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	registry := agent.NewRegistry()
	registry.Register(agent.NewClaude())
	registry.Register(agent.NewPi())
	registry.Register(agent.NewShell())
	service := Service{
		Store: db, TMux: tmux.Client{Socket: socket}, Audit: logger, Clock: clock,
		IDs: config.NewIDGenerator(idSeed), Agents: registry, ConfigEnv: configEnv,
		DeckExecutable: filepath.Join(home, "bin", "deck"), DeckHome: home,
	}
	return service, db, logger, socket
}

func TestCreateAgentAssignsConversationIDAndLaunchesClaudeArgv(t *testing.T) {
	cwd := t.TempDir()
	service, db, logger, socket := newAgentTestService(t, nil, "create-agent-test")

	session, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: session", CWD: cwd, Agent: "claude", PermissionProfile: "edits",
		Env: map[string]string{"VISIBLE": "yes", "SECRET_TOKEN": "not-in-audit"},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if session.Agent != "claude" || session.ConversationID == "" || session.PermissionProfile != "edits" {
		t.Fatalf("durable session = %#v", session)
	}

	rows, err := db.ListSessions(context.Background())
	if err != nil || len(rows) != 1 {
		t.Fatalf("durable rows = %#v, %v", rows, err)
	}
	if rows[0].ConversationID != session.ConversationID {
		t.Fatalf("persisted conversation id = %q, want %q", rows[0].ConversationID, session.ConversationID)
	}
	if len(rows[0].Env) != 2 || rows[0].Env["VISIBLE"] != "yes" || rows[0].Env["SECRET_TOKEN"] != "not-in-audit" {
		t.Fatalf("persisted user env = %#v, want only the two user-supplied keys", rows[0].Env)
	}
	if _, ok := rows[0].Env["DECK_SESSION_ID"]; ok {
		t.Fatalf("deck-owned instrumentation leaked into persisted user env: %#v", rows[0].Env)
	}
	if rows[0].StatusSource != "tmux" || rows[0].Status != "starting" {
		t.Fatalf("row status = %#v, want tmux-observed starting row", rows[0])
	}

	live, err := service.TMux.List(context.Background())
	if err != nil || len(live) != 1 || len(live[0].Panes) != 1 {
		t.Fatalf("live tmux sessions = %#v, %v", live, err)
	}

	records := auditRecords(t, logger.Path())
	var launch map[string]any
	for _, record := range records {
		if record["event"] == "launch" {
			launch = record
		}
	}
	if launch == nil {
		t.Fatalf("no launch audit record among %#v", records)
	}
	argv := jsonStrings(launch["argv"])
	wantPrefix := []string{"claude", "--session-id", session.ConversationID, "--permission-mode", "acceptEdits"}
	if len(argv) != len(wantPrefix)+2 || strings.Join(argv[:len(wantPrefix)], "\x00") != strings.Join(wantPrefix, "\x00") || argv[len(wantPrefix)] != "--settings" {
		t.Fatalf("launch argv = %#v, want base argv followed by deck --settings", argv)
	}
	if !strings.Contains(argv[len(argv)-1], "'"+service.DeckExecutable+"' _hook") {
		t.Fatalf("instrument settings = %q, want absolute deck executable %q", argv[len(argv)-1], service.DeckExecutable)
	}
	envKeys := jsonStrings(launch["env_keys"])
	if strings.Join(envKeys, ",") != "DECK_HOME,DECK_SESSION_ID,PATH,SECRET_TOKEN,VISIBLE" {
		t.Fatalf("launch env_keys = %#v, want user and deck-owned launch keys", envKeys)
	}
	assertTMuxEnvironment(t, socket, session.Slug, "DECK_SESSION_ID", session.ID)
	assertTMuxEnvironment(t, socket, session.Slug, "DECK_HOME", service.DeckHome)
	contents, err := readAuditFile(t, logger.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(contents, "not-in-audit") {
		t.Fatalf("audit leaked environment value: %s", contents)
	}
}

func TestCreateAgentDegradesUnsupportedProfileForPi(t *testing.T) {
	cwd := t.TempDir()
	service, db, logger, _ := newAgentTestService(t, nil, "create-agent-pi")

	session, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Pi: session", CWD: cwd, Agent: "pi", PermissionProfile: "plan",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if session.PermissionProfile != "safe" {
		t.Fatalf("resolved permission profile = %q, want degraded to safe", session.PermissionProfile)
	}
	rows, err := db.ListSessions(context.Background())
	if err != nil || len(rows) != 1 || rows[0].PermissionProfile != "safe" {
		t.Fatalf("persisted rows = %#v, %v", rows, err)
	}
	records := auditRecords(t, logger.Path())
	found := false
	for _, record := range records {
		if record["event"] != "launch" {
			continue
		}
		found = true
		argv := jsonStrings(record["argv"])
		if len(argv) < 2 || argv[0] != "pi" || argv[1] != "--session-id" {
			t.Fatalf("pi launch argv = %#v", argv)
		}
	}
	if !found {
		t.Fatalf("no launch record among %#v", records)
	}
}

func TestCreateAgentResolvesPATHInSPECOrder(t *testing.T) {
	cwd := t.TempDir()
	service, _, logger, _ := newAgentTestService(t, map[string]string{"PATH": "/config/bin", "FROM_CONFIG": "1"}, "create-agent-path")

	session, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: path", CWD: cwd, Agent: "claude", PermissionProfile: "safe",
		Env: map[string]string{"FROM_SESSION": "1"},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	_ = session
	records := auditRecords(t, logger.Path())
	var envKeys []string
	for _, record := range records {
		if record["event"] == "launch" {
			envKeys = jsonStrings(record["env_keys"])
		}
	}
	if strings.Join(envKeys, ",") != "DECK_HOME,DECK_SESSION_ID,FROM_CONFIG,FROM_SESSION,PATH" {
		t.Fatalf("launch env_keys = %#v, want config, session, and instrumentation keys present", envKeys)
	}
}

// waitForFile polls until path exists or the deadline passes, giving the
// pane's shell time to actually run in the real (test-socket) tmux server.
func waitForFile(t *testing.T, path string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestCreateAgentRunsSucceedingPreLaunchBeforeTheAgent(t *testing.T) {
	cwd := t.TempDir()
	service, _, _, _ := newAgentTestService(t, nil, "pre-launch-ok")
	preMarker := filepath.Join(cwd, "pre_marker")
	agentMarker := filepath.Join(cwd, "agent_marker")

	_, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Shell: pre-launch ok", CWD: cwd, Agent: "shell",
		PreLaunch:  "touch " + preMarker,
		LaunchArgs: []string{"-c", "touch " + agentMarker + " && sleep 2"},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if !waitForFile(t, preMarker, 5*time.Second) {
		t.Fatalf("pre_launch never ran: %s missing", preMarker)
	}
	if !waitForFile(t, agentMarker, 5*time.Second) {
		t.Fatalf("agent never started after a succeeding pre_launch: %s missing", agentMarker)
	}
}

func TestCreateAgentFailingPreLaunchNeverStartsTheAgent(t *testing.T) {
	cwd := t.TempDir()
	service, _, logger, socket := newAgentTestService(t, nil, "pre-launch-fail")
	agentMarker := filepath.Join(cwd, "agent_marker")

	session, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Shell: pre-launch fail", CWD: cwd, Agent: "shell",
		PreLaunch:  "echo pre-launch-boom >&2; exit 9",
		LaunchArgs: []string{"-c", "touch " + agentMarker},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// Give the pane time to run and fail; the agent must never have started.
	time.Sleep(500 * time.Millisecond)
	if _, err := os.Stat(agentMarker); err == nil {
		t.Fatalf("agent started despite a failing pre_launch: %s exists", agentMarker)
	}

	// The pane's own output is visible: deck's tmux server keeps a
	// non-zero-exit pane around (remain-on-exit failed) so the failure is
	// observable rather than silent.
	out, capErr := exec.Command("tmux", "-L", socket, "capture-pane", "-p", "-S", "-", "-t", "deck_"+session.Slug).CombinedOutput()
	if capErr != nil {
		t.Fatalf("capture-pane: %v (%s)", capErr, out)
	}
	if !strings.Contains(string(out), "pre-launch-boom") {
		t.Fatalf("pane output = %q, want it to show the pre_launch failure", out)
	}

	// The launch is still audited (including the pre_launch wrapper), even
	// though it never actually started the agent.
	records := auditRecords(t, logger.Path())
	found := false
	for _, record := range records {
		if record["event"] == "launch" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no launch audit record among %#v", records)
	}
}

func TestCreateAgentLoginShellInvocationForm(t *testing.T) {
	cwd := t.TempDir()
	service, _, logger, _ := newAgentTestService(t, nil, "login-shell")

	_, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Shell: login", CWD: cwd, Agent: "shell", LoginShell: true,
		Env: map[string]string{"FROM_SESSION": "1"},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	records := auditRecords(t, logger.Path())
	var argv, envKeys []string
	for _, record := range records {
		if record["event"] == "launch" {
			argv = jsonStrings(record["argv"])
			envKeys = jsonStrings(record["env_keys"])
		}
	}
	if len(argv) < 2 || argv[1] != "-lc" {
		t.Fatalf("login shell argv = %#v, want [<shell> -lc ...]", argv)
	}
	for _, key := range envKeys {
		if key == "PATH" {
			t.Fatalf("login_shell launch env_keys = %#v, must not inject PATH (mutually exclusive with captured_path)", envKeys)
		}
	}
	if strings.Join(envKeys, ",") != "FROM_SESSION" {
		t.Fatalf("login_shell launch env_keys = %#v, want only FROM_SESSION", envKeys)
	}
}

func TestCreateAgentShellHasNoInstrumentation(t *testing.T) {
	cwd := t.TempDir()
	service, _, logger, socket := newAgentTestService(t, nil, "shell-no-instrumentation")
	session, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "plain shell", CWD: cwd, Agent: "shell", LaunchArgs: []string{"-c", "sleep 2"},
	})
	if err != nil {
		t.Fatalf("create shell adapter session: %v", err)
	}
	for _, record := range auditRecords(t, logger.Path()) {
		if record["event"] != "launch" {
			continue
		}
		if argv := jsonStrings(record["argv"]); strings.Contains(strings.Join(argv, " "), "--settings") {
			t.Fatalf("shell launch argv contains instrumentation: %#v", argv)
		}
		for _, key := range jsonStrings(record["env_keys"]) {
			if strings.HasPrefix(key, "DECK_") {
				t.Fatalf("shell launch environment contains instrumentation key %q", key)
			}
		}
	}
	out, err := exec.Command("tmux", "-L", socket, "show-environment", "-t", "deck_"+session.Slug, "DECK_SESSION_ID").CombinedOutput()
	if err == nil {
		t.Fatalf("shell tmux environment unexpectedly has DECK_SESSION_ID: %s", out)
	}
}

func assertTMuxEnvironment(t *testing.T, socket, slug, key, want string) {
	t.Helper()
	out, err := exec.Command("tmux", "-L", socket, "show-environment", "-t", "deck_"+slug, key).CombinedOutput()
	if err != nil {
		t.Fatalf("show tmux environment %s: %v (%s)", key, err, out)
	}
	if got := strings.TrimSpace(string(out)); got != key+"="+want {
		t.Fatalf("tmux environment %s = %q, want %q", key, got, key+"="+want)
	}
}

func readAuditFile(t *testing.T, path string) (string, error) {
	t.Helper()
	contents, err := os.ReadFile(path)
	return string(contents), err
}
