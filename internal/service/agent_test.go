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
	service := Service{
		Store: db, TMux: tmux.Client{Socket: socket}, Audit: logger, Clock: clock,
		IDs: config.NewIDGenerator(idSeed), Agents: registry, ConfigEnv: configEnv,
	}
	return service, db, logger, socket
}

func TestCreateAgentAssignsConversationIDAndLaunchesClaudeArgv(t *testing.T) {
	cwd := t.TempDir()
	service, db, logger, _ := newAgentTestService(t, nil, "create-agent-test")

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
	wantArgv := []string{"claude", "--session-id", session.ConversationID, "--permission-mode", "acceptEdits"}
	if strings.Join(argv, "\x00") != strings.Join(wantArgv, "\x00") {
		t.Fatalf("launch argv = %#v, want %#v", argv, wantArgv)
	}
	envKeys := jsonStrings(launch["env_keys"])
	if strings.Join(envKeys, ",") != "PATH,SECRET_TOKEN,VISIBLE" {
		t.Fatalf("launch env_keys = %#v, want PATH, SECRET_TOKEN, VISIBLE", envKeys)
	}
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
	if strings.Join(envKeys, ",") != "FROM_CONFIG,FROM_SESSION,PATH" {
		t.Fatalf("launch env_keys = %#v, want config PATH override plus session key present", envKeys)
	}
}

func readAuditFile(t *testing.T, path string) (string, error) {
	t.Helper()
	contents, err := os.ReadFile(path)
	return string(contents), err
}
