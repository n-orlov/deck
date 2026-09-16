package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
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
	registry.Register(agent.NewCodex())
	service := Service{
		Store: db, TMux: tmux.Client{Socket: socket}, Audit: logger, Clock: clock,
		IDs: config.NewIDGenerator(idSeed), Agents: registry, ConfigEnv: configEnv,
		DeckExecutable: filepath.Join(home, "bin", "deck"), DeckHome: home,
	}
	return service, db, logger, socket
}

func TestCreateAgentAssignsConversationIDAndLaunchesClaudeArgv(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
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

// TestCreateAgentConsultsAssignsConversationIDPerAdapter covers R121: the
// create path mints, passes and stores a conversation id only for an
// adapter that declares Caps.AssignsConversationID. Codex declares false
// (it mints its own id later, from its first hook -- task 014/017), so a
// created codex row's stored conversation id must be empty; claude
// declares true, so its row's must not be. Asserting both halves in one
// test is deliberate: deleting the codex branch in CreateAgent (falling
// back to unconditionally minting an id for every adapter) would still
// leave the claude half green, so only the codex assertion alone would
// catch that regression -- and deleting the whole `if caps.
// AssignsConversationID` guard (unconditionally minting for everyone)
// makes the codex assertion fail while the claude one keeps passing. Both
// assertions together are what actually pins the adapter-conditional
// branch itself, not just one adapter's outcome.
func TestCreateAgentConsultsAssignsConversationIDPerAdapter(t *testing.T) {
	stubExecutableOnPath(t, "claude")
	stubExecutableOnPath(t, "codex")
	service, db, _, _ := newAgentTestService(t, nil, "conversation-id-per-adapter-test")

	codexSession, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Codex: session", CWD: t.TempDir(), Agent: "codex", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("create codex agent: %v", err)
	}
	if codexSession.ConversationID != "" {
		t.Fatalf("codex session conversation id = %q, want empty (codex mints its own)", codexSession.ConversationID)
	}

	claudeSession, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: session", CWD: t.TempDir(), Agent: "claude", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("create claude agent: %v", err)
	}
	if claudeSession.ConversationID == "" {
		t.Fatalf("claude session conversation id is empty, want deck to have minted one")
	}

	rows, err := db.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	var gotCodexRow, gotClaudeRow bool
	for _, row := range rows {
		switch row.ID {
		case codexSession.ID:
			gotCodexRow = true
			if row.ConversationID != "" {
				t.Fatalf("persisted codex conversation id = %q, want empty", row.ConversationID)
			}
		case claudeSession.ID:
			gotClaudeRow = true
			if row.ConversationID == "" {
				t.Fatalf("persisted claude conversation id is empty, want non-empty")
			}
		}
	}
	if !gotCodexRow || !gotClaudeRow {
		t.Fatalf("durable rows = %#v, want both a codex row and a claude row", rows)
	}
}

// TestCreateAgentPromotesCWDToRecentCwds covers task 007's service-level
// requirement that creating a session (agent path) promotes its cwd into
// the §11.7 directory history.
func TestCreateAgentPromotesCWDToRecentCwds(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
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
	stubExecutableOnPath(t, "pi")
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
	// Config's own PATH entirely overrides captured_path in the merged
	// launch env (SPEC §6.3), including for CreateAgent's own preflight, so
	// the stub claude the preflight needs to find has to live in this same
	// config-owned directory, not on the real $PATH.
	configPath := t.TempDir()
	writeStubExecutable(t, configPath, "claude")
	service, _, logger, _ := newAgentTestService(t, map[string]string{"PATH": configPath, "FROM_CONFIG": "1"}, "create-agent-path")

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

// TestCreateAgentPreLaunchExportReachesOnlyThePaneProcessEnvironment covers
// task 008: SPEC §6.4's guarantee ("a hook's exports reach that session's
// agent and nothing else") proved positive against a real tmux server on a
// private test socket. A shell-kind session's own pre_launch exports a
// value; the assertion reads it back not from deck's own store and not from
// tmux's session environment table (show-environment), but straight out of
// the pane process's own /proc/<pid>/environ, the same source
// features/env_editor_test.go's livePaneProcessEnvironmentLookup uses and
// for the same reason: it is the one place a bug in any mirrored table
// could still disagree with the actually-running process.
func TestCreateAgentPreLaunchExportReachesOnlyThePaneProcessEnvironment(t *testing.T) {
	cwd := t.TempDir()
	service, _, _, socket := newAgentTestService(t, nil, "pre-launch-export-reaches-pane")

	session, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Shell: pre-launch export", CWD: cwd, Agent: "shell",
		PreLaunch:  "export DECK_HOOK_EXPORT_TEST=reaches-the-agent-only",
		LaunchArgs: []string{"-c", "sleep 5"},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	value, found := waitForPaneProcessEnvironmentKey(t, socket, session.Slug, "DECK_HOOK_EXPORT_TEST", 5*time.Second)
	if !found {
		t.Fatalf("pane process environment for session %q never carried DECK_HOOK_EXPORT_TEST", session.Slug)
	}
	if value != "reaches-the-agent-only" {
		t.Fatalf("pane process environment DECK_HOOK_EXPORT_TEST = %q, want %q", value, "reaches-the-agent-only")
	}

	// The one leak path SPEC §6.4 names as closed: the tmux *session*
	// environment table (§3.2's -e mirror) never sees this export, because
	// buildPaneCommand never calls tmux's set-environment for it.
	out, showErr := exec.Command("tmux", "-L", socket, "show-environment", "-t", "deck_"+session.Slug, "DECK_HOOK_EXPORT_TEST").CombinedOutput()
	if showErr == nil {
		t.Fatalf("tmux session environment table unexpectedly carries the hook's export: %s", out)
	}
}

// TestCreateAgentPreLaunchExportNeverAppearsInSessionEnvironmentOrStateDB
// covers task 009: SPEC §6.4's two negative boundaries, proved together on
// the exact value the positive assertion just found in the pane, so a
// hypothetical regression that let the export leak into either sink would
// fail this test on the very value that proves the hook actually ran.
func TestCreateAgentPreLaunchExportNeverAppearsInSessionEnvironmentOrStateDB(t *testing.T) {
	cwd := t.TempDir()
	service, db, _, socket := newAgentTestService(t, nil, "pre-launch-export-negative-boundaries")

	// The value under test is generated at hook-run time by catting a file
	// the hook command only names by path, never by writing the value's own
	// text into config -- so a positive hit for exportedValue in the
	// pre_launch column (which legitimately, and expectedly, stores the raw
	// hook command the user configured) can only mean the value leaked, not
	// that config happened to quote it.
	const exportedValue = "hook-export-must-not-leak-9c3f1a"
	valueFile := filepath.Join(t.TempDir(), "boundary-value")
	if err := os.WriteFile(valueFile, []byte(exportedValue), 0o600); err != nil {
		t.Fatalf("write value file: %v", err)
	}
	session, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Shell: pre-launch export boundaries", CWD: cwd, Agent: "shell",
		PreLaunch:  "export DECK_HOOK_BOUNDARY_TEST=\"$(cat " + valueFile + ")\"",
		LaunchArgs: []string{"-c", "sleep 5"},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Positive control: proves the hook actually ran and exportedValue is
	// really the value that reached the agent, before either negative
	// assertion below can be trusted to mean anything.
	paneValue, found := waitForPaneProcessEnvironmentKey(t, socket, session.Slug, "DECK_HOOK_BOUNDARY_TEST", 5*time.Second)
	if !found || paneValue != exportedValue {
		t.Fatalf("pane process environment DECK_HOOK_BOUNDARY_TEST = (%q, found=%v), want (%q, true)", paneValue, found, exportedValue)
	}

	// Negative boundary 1: tmux's session environment table (show-environment,
	// SPEC §3.2's -e mirror) never sees it. If buildPaneCommand ever
	// regressed into mirroring the export via set-environment, this would
	// find exportedValue in the dump and fail.
	out, showErr := exec.Command("tmux", "-L", socket, "show-environment", "-t", "deck_"+session.Slug).CombinedOutput()
	if showErr == nil && strings.Contains(string(out), exportedValue) {
		t.Fatalf("tmux session environment table unexpectedly carries the hook's export: %s", out)
	}

	// Negative boundary 2: no column of the session's own state.db row
	// carries it either, checked generically (every column via SELECT *,
	// never a named allowlist) so a future column addition is covered
	// automatically instead of needing this test rewritten.
	columns := sessionRowColumnValues(t, db, session.ID)
	if len(columns) == 0 {
		t.Fatalf("session row %q returned no columns", session.ID)
	}

	// Sanity: prove the generic scan actually inspects real data -- it must
	// find a value genuinely present (the session's own slug) -- before its
	// report of an absence elsewhere in the row means anything.
	if slug, ok := columns["slug"]; !ok || slug != session.Slug {
		t.Fatalf("sessionRowColumnValues sanity check failed: columns[%q] = %q, want %q (generic scan may be broken)", "slug", slug, session.Slug)
	}

	for col, value := range columns {
		if strings.Contains(value, exportedValue) {
			t.Fatalf("state.db column %q of session %q unexpectedly contains the hook's export: %q", col, session.ID, value)
		}
	}
}

// sessionRowColumnValues reads every column of a session's own state.db row
// generically (SELECT * ...), scanning into interface{} rather than naming
// individual fields, so a negative assertion built on top of it covers any
// column -- including ones added after this helper was written -- not just
// the ones a hand-picked field list would name.
func sessionRowColumnValues(t *testing.T, db *store.Store, sessionID string) map[string]string {
	t.Helper()
	rows, err := db.DB().Query(`SELECT * FROM sessions WHERE id = ?`, sessionID)
	if err != nil {
		t.Fatalf("query session row: %v", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("read columns: %v", err)
	}
	if !rows.Next() {
		t.Fatalf("no state.db row for session %q", sessionID)
	}
	raw := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range raw {
		ptrs[i] = &raw[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		t.Fatalf("scan session row: %v", err)
	}

	values := make(map[string]string, len(cols))
	for i, col := range cols {
		switch v := raw[i].(type) {
		case nil:
			values[col] = ""
		case []byte:
			values[col] = string(v)
		default:
			values[col] = fmt.Sprintf("%v", v)
		}
	}
	return values
}

// paneProcessPID resolves the running pane's own process id via tmux's
// list-panes, the same server the rest of this file drives.
func paneProcessPID(socket, slug string) (int, error) {
	out, err := exec.Command("tmux", "-L", socket, "list-panes", "-t", "deck_"+slug, "-F", "#{pane_pid}").CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("list-panes: %w (%s)", err, out)
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}

// paneProcessEnvironmentLookup reads a running pane process's own
// /proc/<pid>/environ directly -- not deck's store and not tmux's mirrored
// session environment table, either of which could still agree with each
// other while disagreeing with the actually-running process.
func paneProcessEnvironmentLookup(socket, slug, key string) (string, bool, error) {
	pid, err := paneProcessPID(socket, slug)
	if err != nil {
		return "", false, err
	}
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return "", false, err
	}
	for _, entry := range strings.Split(string(raw), "\x00") {
		if entry == "" {
			continue
		}
		k, v, ok := strings.Cut(entry, "=")
		if ok && k == key {
			return v, true, nil
		}
	}
	return "", false, nil
}

// waitForPaneProcessEnvironmentKey polls paneProcessEnvironmentLookup until
// the key appears or the deadline passes, giving the pane's shell time to
// actually run its pre_launch export in the real (test-socket) tmux server.
func waitForPaneProcessEnvironmentKey(t *testing.T, socket, slug, key string, timeout time.Duration) (string, bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		value, found, err := paneProcessEnvironmentLookup(socket, slug, key)
		if err == nil && found {
			return value, true
		}
		if time.Now().After(deadline) {
			return "", false
		}
		time.Sleep(20 * time.Millisecond)
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

// TestCreateAgentPreflightRefusesAgentBinaryNotFoundOnPath is task 010/011's
// own evidence: an adapter whose declared executable is not on the PATH
// this pane would launch under must never get as far as a durable row or
// a tmux pane -- CreateAgent's preflight has to run and fail before
// Store.CreateSession, not after.
func TestCreateAgentPreflightRefusesAgentBinaryNotFoundOnPath(t *testing.T) {
	cwd := t.TempDir()
	// Deliberately no stubExecutableOnPath: the CI toolchain never installs
	// claude, so PATH genuinely lacks it.
	service, db, _, _ := newAgentTestService(t, nil, "create-agent-missing-binary")

	_, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: missing binary", CWD: cwd, Agent: "claude", PermissionProfile: "safe",
	})
	if err == nil {
		t.Fatalf("create agent: want error for an agent binary not on PATH, got none")
	}
	if !strings.Contains(err.Error(), `agent binary "claude" not found on PATH`) {
		t.Fatalf("create agent error = %q, want it to name the kind/executable in the shape agent binary %q not found on PATH", err.Error(), "claude")
	}

	rows, listErr := db.ListSessions(context.Background())
	if listErr != nil {
		t.Fatalf("list sessions: %v", listErr)
	}
	if len(rows) != 0 {
		t.Fatalf("durable rows = %#v, want none: the preflight must run before Store.CreateSession", rows)
	}

	live, listTmuxErr := service.TMux.List(context.Background())
	if listTmuxErr != nil {
		t.Fatalf("list tmux: %v", listTmuxErr)
	}
	if len(live) != 0 {
		t.Fatalf("live tmux sessions = %#v, want none: the preflight must run before any pane is created", live)
	}
}

// TestCreateAgentPreflightRefusesModeNonExecutableBinary is the R113 half
// of docs/reports/phase3k-findings.md finding 1 (independent review's
// second reproduction, review-nonexec-create.log): a mode-0644 regular
// file named "claude" on the launch PATH exists and is not a directory,
// but it is not executable, so the preflight (which shares lookPathIn with
// AvailableKinds) must still refuse before any durable row or tmux pane.
func TestCreateAgentPreflightRefusesModeNonExecutableBinary(t *testing.T) {
	cwd := t.TempDir()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
		t.Fatalf("write non-executable fake claude file: %v", err)
	}
	service, db, _, _ := newAgentTestService(t, nil, "create-agent-nonexec-binary")
	service.ConfigEnv = map[string]string{"PATH": dir}

	_, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: mode 0644", CWD: cwd, Agent: "claude", PermissionProfile: "safe",
	})
	if err == nil {
		t.Fatalf("create agent: want error for a mode-0644 claude on PATH, got none")
	}
	if !strings.Contains(err.Error(), `agent binary "claude" not found on PATH`) {
		t.Fatalf("create agent error = %q, want it to name the kind/executable in the shape agent binary %q not found on PATH", err.Error(), "claude")
	}

	rows, listErr := db.ListSessions(context.Background())
	if listErr != nil {
		t.Fatalf("list sessions: %v", listErr)
	}
	if len(rows) != 0 {
		t.Fatalf("durable rows = %#v, want none: the preflight must run before Store.CreateSession", rows)
	}

	live, listTmuxErr := service.TMux.List(context.Background())
	if listTmuxErr != nil {
		t.Fatalf("list tmux: %v", listTmuxErr)
	}
	if len(live) != 0 {
		t.Fatalf("live tmux sessions = %#v, want none: the preflight must run before any pane is created", live)
	}
}

// TestCreateAgentPreflightRefusesFIFONamedLikeAgentBinary is task 202's own
// pin: a mode-0755 FIFO named "claude" on the launch PATH this pane would
// resolve is not a regular file, so lookPathIn (hardened by task 201) must
// still refuse it, and CreateAgent's own preflight (which shares
// lookPathIn with AvailableKinds and resume.go's preflight) must never get
// as far as a durable row or a tmux pane for it -- the same non-regression
// shape TestCreateAgentPreflightRefusesAgentBinaryNotFoundOnPath and
// TestCreateAgentPreflightRefusesModeNonExecutableBinary already assert
// for the not-on-PATH and non-executable-mode cases.
func TestCreateAgentPreflightRefusesFIFONamedLikeAgentBinary(t *testing.T) {
	cwd := t.TempDir()
	dir := t.TempDir()
	fifoPath := filepath.Join(dir, "claude")
	if err := syscall.Mkfifo(fifoPath, 0o755); err != nil {
		t.Fatalf("mkfifo %s: %v", fifoPath, err)
	}
	service, db, _, _ := newAgentTestService(t, nil, "create-agent-fifo-binary")
	service.ConfigEnv = map[string]string{"PATH": dir}

	_, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: fifo", CWD: cwd, Agent: "claude", PermissionProfile: "safe", LoginShell: false,
	})
	if err == nil {
		t.Fatalf("create agent: want error for a FIFO named claude on PATH, got none")
	}
	if !strings.Contains(err.Error(), "not found on PATH") {
		t.Fatalf("create agent error = %q, want it to contain %q", err.Error(), "not found on PATH")
	}

	rows, listErr := db.ListSessions(context.Background())
	if listErr != nil {
		t.Fatalf("list sessions: %v", listErr)
	}
	for _, row := range rows {
		if row.Name == "Claude: fifo" {
			t.Fatalf("durable rows = %#v, want no row named %q: the preflight must run before Store.CreateSession", rows, "Claude: fifo")
		}
	}

	live, listTmuxErr := service.TMux.List(context.Background())
	if listTmuxErr != nil {
		t.Fatalf("list tmux: %v", listTmuxErr)
	}
	if len(live) != 0 {
		t.Fatalf("live tmux sessions = %#v, want none: the preflight must run before any pane is created", live)
	}
}

// TestCreateAgentLoginShellSkipsBinaryPreflight proves the login_shell=1
// exemption: a login shell resolves its own PATH via its own profile/rc
// scripts (SPEC §6.4), so CreateAgent must not preflight-check the
// adapter's declared executable for it, mirroring resume.go's own
// exemption for a resumed login shell.
func TestCreateAgentLoginShellSkipsBinaryPreflight(t *testing.T) {
	cwd := t.TempDir()
	// Deliberately no stubExecutableOnPath: claude is not on PATH, but
	// login_shell=1 must make CreateAgent skip the preflight entirely.
	service, db, _, _ := newAgentTestService(t, nil, "create-agent-login-shell-skip")

	session, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: login shell", CWD: cwd, Agent: "claude", PermissionProfile: "safe", LoginShell: true,
	})
	if err != nil {
		t.Fatalf("create agent with login_shell=1: %v", err)
	}
	rows, listErr := db.ListSessions(context.Background())
	if listErr != nil || len(rows) != 1 || rows[0].ID != session.ID {
		t.Fatalf("durable rows = %#v, %v, want exactly the created row", rows, listErr)
	}
}

// TestCreateAgentPreflightLeavesInstalledAgentPaneCommandUnchanged is task
// 011's third case: for an agent kind whose declared executable IS on
// PATH, R111's preflight (task 010) must be a pure pass-through -- create
// still succeeds, and the pane command it records is byte-identical to
// what today's tree (the adapter's own public Launch/Instrument contract,
// which the preflight never touches) produces for the same input. Rather
// than hand-duplicating Claude's --settings JSON shape (fragile, and would
// drift the moment claude.go's hook wiring changes), this recomputes the
// expected argv independently by calling the same public agent.Adapter
// methods CreateAgent itself calls, against the identical launch input,
// and requires exact (not merely prefix) equality with what got audited.
func TestCreateAgentPreflightLeavesInstalledAgentPaneCommandUnchanged(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
	service, _, logger, _ := newAgentTestService(t, nil, "create-agent-preflight-installed")

	session, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: preflight installed", CWD: cwd, Agent: "claude", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("create agent for an installed kind: %v", err)
	}

	claude := agent.NewClaude()
	launchInput := agent.LaunchInput{
		CWD: cwd, ConversationID: session.ConversationID, Profile: "safe",
		DeckExecutable: service.DeckExecutable, DeckSessionID: session.ID, DeckHome: service.DeckHome,
	}
	wantArgv, err := claude.Launch(launchInput)
	if err != nil {
		t.Fatalf("claude launch: %v", err)
	}
	instrumentArgv, _ := claude.Instrument(launchInput)
	wantArgv = append(wantArgv, instrumentArgv...)

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
	gotArgv := jsonStrings(launch["argv"])
	if !reflect.DeepEqual(gotArgv, wantArgv) {
		t.Fatalf("recorded pane command = %#v, want byte-identical to today's tree's own adapter.Launch+Instrument output for the same input: %#v", gotArgv, wantArgv)
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
