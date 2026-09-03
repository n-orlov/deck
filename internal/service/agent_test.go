package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
	if strings.Join(envKeys, ",") != "DECK_HOME,DECK_SESSION_AGENT,DECK_SESSION_CONVERSATION_ID,DECK_SESSION_CWD,DECK_SESSION_ID,DECK_SESSION_LAUNCH_KIND,DECK_SESSION_NAME,DECK_SESSION_PROFILE,DECK_SESSION_SLUG,DECK_SESSION_WORKSPACE,PATH,SECRET_TOKEN,VISIBLE" {
		t.Fatalf("launch env_keys = %#v, want user, deck-owned session-context, and instrumentation keys", envKeys)
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

// TestCreateAgentPromotesCWDToRecentCwds covers task 007's service-level
// requirement that creating a session (agent path) promotes its cwd into
// the §11.7 directory history.
func TestCreateAgentPromotesCWDToRecentCwds(t *testing.T) {
	cwd := t.TempDir()
	service, db, _, _ := newAgentTestService(t, nil, "create-agent-recent-test")
	service.RecentCwdLimit = 5

	session, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: recent", CWD: cwd, Agent: "claude", PermissionProfile: "edits",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	recent, err := db.RecentCwds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].Path != session.CWD {
		t.Fatalf("recent cwds after create agent = %+v, want exactly [%q]", recent, session.CWD)
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
	if strings.Join(envKeys, ",") != "DECK_HOME,DECK_SESSION_AGENT,DECK_SESSION_CONVERSATION_ID,DECK_SESSION_CWD,DECK_SESSION_ID,DECK_SESSION_LAUNCH_KIND,DECK_SESSION_NAME,DECK_SESSION_PROFILE,DECK_SESSION_SLUG,DECK_SESSION_WORKSPACE,FROM_CONFIG,FROM_SESSION,PATH" {
		t.Fatalf("launch env_keys = %#v, want config, session, session-context, and instrumentation keys present", envKeys)
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

func TestCreateAgentGlobalPreLaunchRunsBeforeSessionPreLaunchAndAgent(t *testing.T) {
	cwd := t.TempDir()
	service, _, _, _ := newAgentTestService(t, nil, "global-pre-launch-ok")
	service.GlobalPreLaunch = "touch " + filepath.Join(cwd, "global_marker")
	globalMarker := filepath.Join(cwd, "global_marker")
	sessionMarker := filepath.Join(cwd, "session_marker")
	agentMarker := filepath.Join(cwd, "agent_marker")

	_, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Shell: global pre-launch ok", CWD: cwd, Agent: "shell",
		// The session hook only succeeds once the global marker already
		// exists, proving the global hook ran first.
		PreLaunch:  "test -f " + globalMarker + " && touch " + sessionMarker,
		LaunchArgs: []string{"-c", "touch " + agentMarker + " && sleep 2"},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if !waitForFile(t, globalMarker, 5*time.Second) {
		t.Fatalf("global pre_launch never ran: %s missing", globalMarker)
	}
	if !waitForFile(t, sessionMarker, 5*time.Second) {
		t.Fatalf("session pre_launch never ran after the global hook: %s missing", sessionMarker)
	}
	if !waitForFile(t, agentMarker, 5*time.Second) {
		t.Fatalf("agent never started after both hooks succeeded: %s missing", agentMarker)
	}
}

func TestCreateAgentFailingGlobalPreLaunchNeverReachesSessionHookOrAgent(t *testing.T) {
	cwd := t.TempDir()
	service, _, _, socket := newAgentTestService(t, nil, "global-pre-launch-fail")
	service.GlobalPreLaunch = "echo global-pre-launch-boom >&2; exit 9"
	sessionMarker := filepath.Join(cwd, "session_marker")
	agentMarker := filepath.Join(cwd, "agent_marker")

	session, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Shell: global pre-launch fail", CWD: cwd, Agent: "shell",
		PreLaunch:  "touch " + sessionMarker,
		LaunchArgs: []string{"-c", "touch " + agentMarker},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// Give the pane time to run and fail; neither the session hook nor the
	// agent must ever have started.
	time.Sleep(500 * time.Millisecond)
	if _, err := os.Stat(sessionMarker); err == nil {
		t.Fatalf("session pre_launch ran despite a failing global pre_launch: %s exists", sessionMarker)
	}
	if _, err := os.Stat(agentMarker); err == nil {
		t.Fatalf("agent started despite a failing global pre_launch: %s exists", agentMarker)
	}

	out, capErr := exec.Command("tmux", "-L", socket, "capture-pane", "-p", "-S", "-", "-t", "deck_"+session.Slug).CombinedOutput()
	if capErr != nil {
		t.Fatalf("capture-pane: %v (%s)", capErr, out)
	}
	if !strings.Contains(string(out), "global-pre-launch-boom") {
		t.Fatalf("pane output = %q, want it to show the global pre_launch failure", out)
	}
}

// TestCreateAgentFailingGlobalPreLaunchLeavesRowInErrorWithPaneRetained covers
// task 006's fail-closed row-and-pane assertion: a global pre_launch that
// fails, paired with a session pre_launch that would itself pass, must never
// let the session hook or the agent run, must retain the dead pane long
// enough for its own output to be observed (deck's server-wide `remain-on-exit
// failed`, SPEC.md:547), and once Reconcile collects that retained corpse,
// the durable row must read `error` carrying the hook's own exit status as
// its reason and the hook's own stderr in its crash tail.
func TestCreateAgentFailingGlobalPreLaunchLeavesRowInErrorWithPaneRetained(t *testing.T) {
	cwd := t.TempDir()
	service, db, _, _ := newAgentTestService(t, nil, "global-pre-launch-fail-error-row")
	service.GlobalPreLaunch = "echo global-pre-launch-boom >&2; exit 9"
	sessionMarker := filepath.Join(cwd, "session_marker")
	agentMarker := filepath.Join(cwd, "agent_marker")

	session, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Shell: global pre-launch fail, session hook would pass", CWD: cwd, Agent: "shell",
		// This session hook, on its own, always succeeds: if it ever ran, the
		// fixture would not be discriminating for a fail-closed global hook.
		PreLaunch:  "touch " + sessionMarker,
		LaunchArgs: []string{"-c", "touch " + agentMarker},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Fail-closed retention: deck's tmux server keeps the dead pane (and its
	// session) around under remain-on-exit failed rather than letting it exit
	// cleanly, so the failure is observable instead of silently vanishing.
	waitForDeadPane(t, service.TMux, session.Slug, 9)
	if _, err := os.Stat(sessionMarker); err == nil {
		t.Fatalf("session pre_launch ran despite a failing global pre_launch: %s exists", sessionMarker)
	}
	if _, err := os.Stat(agentMarker); err == nil {
		t.Fatalf("agent started despite a failing global pre_launch: %s exists", agentMarker)
	}

	// Before any reconciliation pass, the durable row is still whatever
	// CreateAgent itself wrote (launch.ready via tmux) -- proving the row does
	// not turn error on its own; only collecting the retained corpse does.
	preReconcile, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preReconcile.Status == "error" {
		t.Fatalf("row already reads error before reconciliation collected the retained pane")
	}

	if err := service.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile retained corpse: %v", err)
	}
	got, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "error" {
		t.Fatalf("collected row reads %q, want error: the failing global pre_launch was not collected as a crash", got.Status)
	}
	if got.PaneExitStatus == nil || *got.PaneExitStatus != 9 {
		t.Fatalf("pane exit status = %v, want the global hook's own exit code 9", got.PaneExitStatus)
	}
	if !strings.Contains(got.StatusReason, "9") {
		t.Fatalf("status reason = %q, want it to carry the hook's own exit status 9", got.StatusReason)
	}
	if !strings.Contains(got.CrashTail, "global-pre-launch-boom") {
		t.Fatalf("crash tail = %q, want it to carry the global pre_launch's own stderr", got.CrashTail)
	}
}

// TestBuildPaneCommand covers task 005: buildPaneCommand joins a global
// hook, a session hook and the adapter argv global-first with `&&`, without
// disturbing any case that already worked before the global hook existed.
func TestBuildPaneCommand(t *testing.T) {
	argv := []string{"agent", "--flag"}

	// Empty-global case, both sub-cases: byte-identical to pre-task-005
	// output for the same session.
	bareFastPath, err := buildPaneCommand("", "", false, argv)
	if err != nil {
		t.Fatalf("empty global, empty session, no login shell: %v", err)
	}
	if !reflect.DeepEqual(bareFastPath, argv) {
		t.Fatalf("bare argv fast path = %#v, want the adapter argv unchanged: %#v", bareFastPath, argv)
	}

	sessionOnly, err := buildPaneCommand("", "touch session", false, argv)
	if err != nil {
		t.Fatalf("empty global, session hook set: %v", err)
	}
	wantSessionOnly := []string{"/bin/sh", "-c", `touch session && exec "$@"`, "deck-agent", "agent", "--flag"}
	if !reflect.DeepEqual(sessionOnly, wantSessionOnly) {
		t.Fatalf("empty-global pane command = %#v, want %#v (byte-identical to pre-task-005 shape)", sessionOnly, wantSessionOnly)
	}

	// Global-only: the global hook runs, joined to `exec "$@"` with `&&`.
	globalOnly, err := buildPaneCommand("touch global", "", false, argv)
	if err != nil {
		t.Fatalf("global hook set, no session hook: %v", err)
	}
	wantGlobalOnly := []string{"/bin/sh", "-c", `touch global && exec "$@"`, "deck-agent", "agent", "--flag"}
	if !reflect.DeepEqual(globalOnly, wantGlobalOnly) {
		t.Fatalf("global-only pane command = %#v, want %#v", globalOnly, wantGlobalOnly)
	}

	// Both set: global runs first, then the session hook, then the agent.
	both, err := buildPaneCommand("touch global", "touch session", false, argv)
	if err != nil {
		t.Fatalf("global and session hooks set: %v", err)
	}
	wantBoth := []string{"/bin/sh", "-c", `touch global && touch session && exec "$@"`, "deck-agent", "agent", "--flag"}
	if !reflect.DeepEqual(both, wantBoth) {
		t.Fatalf("global+session pane command = %#v, want %#v (global must run first)", both, wantBoth)
	}

	// A global hook alone, with no session hook and no login shell, still
	// forces the shell-wrapped form rather than the bare-argv fast path.
	if reflect.DeepEqual(globalOnly, argv) {
		t.Fatalf("global-only pane command took the bare argv fast path, want the shell wrapper")
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
	// SPEC §6.1 (R104): every launch, login_shell included, still carries
	// deck's own session-context layer -- only the PATH-resolution layers
	// (captured_path/config/session) are affected by login_shell.
	if strings.Join(envKeys, ",") != "DECK_HOME,DECK_SESSION_AGENT,DECK_SESSION_CONVERSATION_ID,DECK_SESSION_CWD,DECK_SESSION_ID,DECK_SESSION_LAUNCH_KIND,DECK_SESSION_NAME,DECK_SESSION_PROFILE,DECK_SESSION_SLUG,DECK_SESSION_WORKSPACE,FROM_SESSION" {
		t.Fatalf("login_shell launch env_keys = %#v, want session-context keys plus only FROM_SESSION", envKeys)
	}
}

// TestCreateAgentLoginShellStoresCapturedPathButMarksItAdvisory proves task
// 017/SPEC §6.3: a created session with login_shell=1 still stores its
// create-time captured_path (never blanked, even though the launch itself
// does not inject it as PATH -- see TestCreateAgentLoginShellInvocationForm
// above), but the row records that captured_path as advisory via a
// persisted, queryable marking (store.Session.CapturedPathAdvisory, backed
// by the login_shell column), not a code comment. A sibling session created
// without login_shell reports the opposite on both counts.
func TestCreateAgentLoginShellStoresCapturedPathButMarksItAdvisory(t *testing.T) {
	cwd := t.TempDir()
	service, db, _, _ := newAgentTestService(t, nil, "login-shell-advisory")

	loginSession, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "advisory: login", CWD: cwd, Agent: "shell", LoginShell: true,
	})
	if err != nil {
		t.Fatalf("create login_shell agent: %v", err)
	}
	plainSession, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "advisory: plain", CWD: cwd, Agent: "shell",
	})
	if err != nil {
		t.Fatalf("create plain agent: %v", err)
	}

	loginRow, err := db.GetSession(context.Background(), loginSession.ID)
	if err != nil {
		t.Fatalf("get login_shell session: %v", err)
	}
	if loginRow.CapturedPath == "" {
		t.Fatal("login_shell=1 row must still store a non-empty captured_path")
	}
	if !loginRow.CapturedPathAdvisory() {
		t.Fatal("login_shell=1 row: CapturedPathAdvisory() = false; want true")
	}

	plainRow, err := db.GetSession(context.Background(), plainSession.ID)
	if err != nil {
		t.Fatalf("get plain session: %v", err)
	}
	if plainRow.CapturedPath == "" {
		t.Fatal("plain row must still store a non-empty captured_path")
	}
	if plainRow.CapturedPathAdvisory() {
		t.Fatal("login_shell=0 row: CapturedPathAdvisory() = true; want false")
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
			t.Fatalf("shell launch argv contains claude-only instrumentation: %#v", argv)
		}
		for _, key := range jsonStrings(record["env_keys"]) {
			if key == "DECK_LAUNCH_GENERATION" {
				t.Fatalf("shell launch environment unexpectedly carries a launch generation (no lease taken on create): %#v", record["env_keys"])
			}
		}
	}
	// SPEC §6.1 (R104): every adapter, shell included, now carries the
	// deck-owned session context -- this is the point of the requirement,
	// not an exception to it.
	assertTMuxEnvironment(t, socket, session.Slug, "DECK_SESSION_ID", session.ID)
	assertTMuxEnvironment(t, socket, session.Slug, "DECK_HOME", service.DeckHome)
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
