package features

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/cucumber/godog"
	_ "modernc.org/sqlite"
)

// registerBlackBoxAssertionSteps deliberately talks only to the released deck
// binary, tmux CLI, SQLite file, audit JSONL, and filesystem. It never imports
// a deck internal package, so features observe the same contracts as users.
func registerBlackBoxAssertionSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" is started$`, startNamedClient)
	sc.Step(`^deck client "([^"]+)" sends "([^"]*)"$`, sendClientKeys)
	sc.Step(`^deck client "([^"]+)" screen contains "([^"]+)"$`, clientScreenContains)
	sc.Step(`^within one configured reconcile interval deck client "([^"]+)" screen contains "([^"]+)"$`, clientScreenContainsWithinReconcileInterval)
	sc.Step(`^after one configured reconcile interval deck client "([^"]+)" screen still contains "([^"]+)"$`, clientScreenStillContainsAfterReconcileInterval)
	sc.Step(`^the private tmux session "([^"]+)" exists$`, privateSessionExists)
	sc.Step(`^the private tmux session "([^"]+)" has one pane in "([^"]+)"$`, privateSessionPaneCWD)
	sc.Step(`^the private tmux session "([^"]+)" has exactly one pane running "([^"]+)"$`, privateSessionPaneCommand)
	sc.Step(`^the private tmux session "([^"]+)" has one pane in the created working directory$`, privateSessionPaneInCreatedWorkingDir)
	sc.Step(`^the private tmux option "([^"]+)" is "([^"]+)"$`, privateOptionIs)
	sc.Step(`^the state database has schema version ([0-9]+)$`, databaseSchemaVersion)
	sc.Step(`^the state database journal mode is "([^"]+)"$`, databaseJournalMode)
	sc.Step(`^the state database contains session "([^"]+)" with status "([^"]+)"$`, databaseSessionStatus)
	sc.Step(`^the audit log is valid JSONL$`, auditLogIsJSONL)
	sc.Step(`^the audit log contains event "([^"]+)" for a session$`, auditContainsSessionEvent)
	sc.Step(`^the scenario home has mode "([0-7]+)"$`, scenarioHomeMode)
	sc.Step(`^the state database has mode "([0-7]+)"$`, stateDatabaseMode)
	sc.Step(`^deck client "([^"]+)" creates shell session "([^"]+)"$`, clientCreatesShellSession)
	sc.Step(`^the private tmux session "([^"]+)" does not exist$`, privateSessionDoesNotExist)
	sc.Step(`^the default tmux socket does not have session "([^"]+)"$`, defaultSessionDoesNotExist)
	sc.Step(`^deck client "([^"]+)" attempts shell session "([^"]+)"$`, clientAttemptsShellSession)
	sc.Step(`^deck client "([^"]+)" closes the create modal$`, clientClosesCreateModal)
	sc.Step(`^deck client "([^"]+)" is started without tmux$`, startClientWithoutTMux)
	sc.Step(`^deck client "([^"]+)" is started with tmux version "([^"]+)"$`, startClientWithTMuxVersion)
	sc.Step(`^deck client "([^"]+)" attaches to and detaches from its session$`, clientAttachesAndDetaches)
	sc.Step(`^deck client "([^"]+)" kills its selected session$`, clientKillsSelectedSession)
	sc.Step(`^deck client "([^"]+)" is killed with SIGKILL$`, clientIsKilledWithSIGKILL)
	sc.Step(`^the agent process "([^"]+)" in private tmux session "([^"]+)" is killed with SIGKILL$`, agentProcessInPrivateSessionIsKilledWithSIGKILL)
	sc.Step(`^the private tmux session "([^"]+)" retains a dead pane with a nonzero termination$`, privateSessionRetainsNonzeroDeadPane)
	sc.Step(`^the created working-directory sentinel is unchanged$`, createdSentinelIsUnchanged)
	sc.Step(`^deck client "([^"]+)" exits cleanly$`, clientExitsCleanly)
}

func assertionHarness(ctx context.Context) (*ScenarioHarness, error) { return scenarioHarness(ctx) }

func startNamedClient(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.StartNamedClient(ctx, name)
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

func sendClientKeys(ctx context.Context, name, keys string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	return client.Send(keys)
}

func clientScreenContains(ctx context.Context, name, want string) error {
	return clientScreenContainsBefore(ctx, name, want, 5*time.Second)
}

// clientScreenContainsWithinReconcileInterval makes the multi-client timing
// contract observable. Its deadline is exactly the DECK_RECONCILE_MS value
// configured by ScenarioHarness, rather than the general-purpose five-second
// UI assertion timeout.
func clientScreenContainsWithinReconcileInterval(ctx context.Context, name, want string) error {
	return clientScreenContainsBefore(ctx, name, want, scenarioReconcileInterval)
}

// clientScreenStillContainsAfterReconcileInterval is deliberately not a
// polling assertion: it waits out a complete cadence before inspecting the
// current frame. This proves a value visible immediately after creation also
// survives a subsequent released-binary reconciliation pass.
func clientScreenStillContainsAfterReconcileInterval(ctx context.Context, name, want string) error {
	timer := time.NewTimer(scenarioReconcileInterval + 100*time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}

	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	frame := client.Frame(false)
	if !strings.Contains(frame, want) {
		return fmt.Errorf("client %q does not still show %q after reconcile interval %s:\n%s", name, want, scenarioReconcileInterval, frame)
	}
	return nil
}

func clientScreenContainsBefore(ctx context.Context, name, want string, timeout time.Duration) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	wait, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := client.WaitForFrame(wait, false, want); err != nil {
		return fmt.Errorf("client %q did not show %q within %s: %w", name, want, timeout, err)
	}
	return nil
}

func tmuxOutput(ctx context.Context, h *ScenarioHarness, args ...string) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "tmux", append([]string{"-L", h.Socket}, args...)...).CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("tmux -L %s %s: %w: %s", h.Socket, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

// agentProcessInPrivateSessionIsKilledWithSIGKILL deliberately discovers the
// process through tmux's pane facts and signals that OS process. It does not
// use kill-pane/kill-session and does not touch any ScreenDriver (deck client).
func agentProcessInPrivateSessionIsKilledWithSIGKILL(ctx context.Context, wantCommand, session string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	output, err := tmuxOutput(ctx, h, "list-panes", "-t", session, "-F", "#{pane_pid}|#{pane_current_command}|#{pane_dead}")
	if err != nil {
		return fmt.Errorf("locate agent process in session %q: %w", session, err)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != 1 {
		return fmt.Errorf("session %q has %d panes, want exactly one: %q", session, len(lines), strings.TrimSpace(string(output)))
	}
	fields := strings.Split(lines[0], "|")
	if len(fields) != 3 {
		return fmt.Errorf("parse pane facts for session %q: %q", session, lines[0])
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil {
		return fmt.Errorf("parse pane PID for session %q from %q: %w", session, fields[0], err)
	}
	if pid <= 0 {
		return fmt.Errorf("pane PID for session %q = %d, want positive", session, pid)
	}
	if fields[1] != wantCommand {
		return fmt.Errorf("session %q pane process command = %q, want agent %q", session, fields[1], wantCommand)
	}
	if fields[2] != "0" {
		return fmt.Errorf("session %q pane is already dead", session)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find agent process %d in session %q: %w", pid, session, err)
	}
	if err := process.Signal(syscall.SIGKILL); err != nil {
		return fmt.Errorf("SIGKILL agent process %d in session %q: %w", pid, session, err)
	}
	return nil
}

func privateSessionRetainsNonzeroDeadPane(ctx context.Context, session string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	var facts string
	for time.Now().Before(deadline) {
		output, queryErr := tmuxOutput(ctx, h, "list-panes", "-t", session, "-F", "#{pane_dead}|#{pane_dead_status}|#{pane_dead_signal}")
		if queryErr == nil {
			facts = strings.TrimSpace(string(output))
			fields := strings.Split(facts, "|")
			if len(fields) == 3 && fields[0] == "1" {
				// tmux reports an ordinary nonzero exit in pane_dead_status,
				// but a SIGKILL in pane_dead_signal with an empty status.
				// Either is a nonzero process termination retained by
				// remain-on-exit=failed.
				for _, value := range fields[1:] {
					termination, parseErr := strconv.Atoi(value)
					if parseErr == nil && termination != 0 {
						return nil
					}
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("private tmux session %q did not retain a nonzero dead pane; last facts %q", session, facts)
}

func waitForPrivateSession(ctx context.Context, name string) error {
	for {
		if err := privateSessionExists(ctx, name); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for private tmux session %q: %w", name, ctx.Err())
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func privateSessionDoesNotExist(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "tmux", "-L", h.Socket, "has-session", "-t", name).CombinedOutput()
	if err == nil {
		return fmt.Errorf("private tmux session %q still exists: %s", name, strings.TrimSpace(string(output)))
	}
	return nil
}

func privateSessionExists(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	output, err := tmuxOutput(ctx, h, "has-session", "-t", name)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(output)) != "" {
		return fmt.Errorf("unexpected has-session output for %q: %q", name, output)
	}
	return nil
}

// defaultSessionDoesNotExist proves the managed name was not created on the
// user's ordinary tmux socket. Unlike the private helpers, this deliberately
// omits -L and does not attempt to create a default server.
func defaultSessionDoesNotExist(ctx context.Context, name string) error {
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "tmux", "has-session", "-t", name).CombinedOutput()
	if err == nil {
		return fmt.Errorf("default tmux socket unexpectedly has deck session %q: %s", name, strings.TrimSpace(string(output)))
	}
	return nil
}

func privateSessionPaneInCreatedWorkingDir(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	if h.workingDir == "" {
		return errors.New("no scenario working directory was created")
	}
	return privateSessionPaneCWD(ctx, name, h.workingDir)
}

// privateSessionPaneCommand observes the process tmux reports for the one
// managed pane. This is intentionally a direct query against the scenario's
// private socket rather than a product-internal launch assertion.
func privateSessionPaneCommand(ctx context.Context, name, wantCommand string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	output, err := tmuxOutput(ctx, h, "list-panes", "-t", name, "-F", "#{pane_current_command}")
	if err != nil {
		return err
	}
	commands := strings.Fields(string(output))
	if len(commands) != 1 {
		return fmt.Errorf("private session %q has %d pane commands, want one: %q", name, len(commands), output)
	}
	if commands[0] != wantCommand {
		return fmt.Errorf("private session %q pane command = %q, want %q", name, commands[0], wantCommand)
	}
	return nil
}

func privateSessionPaneCWD(ctx context.Context, name, wantCWD string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	output, err := tmuxOutput(ctx, h, "list-panes", "-t", name, "-F", "#{pane_current_path}")
	if err != nil {
		return err
	}
	paths := strings.Fields(string(output))
	if len(paths) != 1 {
		return fmt.Errorf("private session %q has %d panes, want one: %q", name, len(paths), output)
	}
	wantCWD, err = filepath.Abs(wantCWD)
	if err != nil {
		return err
	}
	if paths[0] != wantCWD {
		return fmt.Errorf("private session %q pane cwd = %q, want %q", name, paths[0], wantCWD)
	}
	return nil
}

func privateOptionIs(ctx context.Context, option, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	// exit-empty is a server option; aggressive-resize is a window option; the
	// other two are global session options. Query their actual scopes directly.
	args := []string{"show-options", "-g", option}
	if option == "exit-empty" {
		args = []string{"show-options", "-s", option}
	}
	if option == "aggressive-resize" {
		args = []string{"show-window-options", "-g", option}
	}
	output, err := tmuxOutput(ctx, h, args...)
	if err != nil {
		return err
	}
	fields := strings.Fields(string(output))
	if len(fields) != 2 || fields[0] != option || fields[1] != want {
		return fmt.Errorf("private tmux option %q = %q, want %q", option, strings.TrimSpace(string(output)), option+" "+want)
	}
	return nil
}

func openObservedDatabase(h *ScenarioHarness) (*sql.DB, error) {
	return sql.Open("sqlite", filepath.Join(h.Home, "state.db"))
}

func databaseSchemaVersion(ctx context.Context, want int) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var got int
	if err := db.QueryRowContext(ctx, `SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&got); err != nil {
		return fmt.Errorf("observe schema version: %w", err)
	}
	if got != want {
		return fmt.Errorf("schema version = %d, want %d", got, want)
	}
	return nil
}

func databaseJournalMode(ctx context.Context, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var got string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&got); err != nil {
		return fmt.Errorf("observe journal mode: %w", err)
	}
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("journal mode = %q, want %q", got, want)
	}
	return nil
}

func databaseSessionStatus(ctx context.Context, name, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	// Poll rather than reading once: a session that just finished launching
	// (e.g. an agent fixture exiting on its own into "stopped") settles on
	// its own schedule, and this step is also used to observe several
	// sessions created back-to-back in one scenario.
	deadline := time.Now().Add(5 * time.Second)
	var got string
	for {
		if err := db.QueryRowContext(ctx, "SELECT status FROM sessions WHERE name = ?", name).Scan(&got); err != nil {
			return fmt.Errorf("observe session %q: %w", name, err)
		}
		if got == want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q status = %q, want %q", name, got, want)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func readAudit(h *ScenarioHarness) ([]map[string]json.RawMessage, error) {
	data, err := os.ReadFile(filepath.Join(h.Home, "log", "deck.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("read audit log: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return nil, errors.New("audit log is empty")
	}
	records := make([]map[string]json.RawMessage, 0, len(lines))
	for number, line := range lines {
		var record map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("parse audit JSONL line %d: %w", number+1, err)
		}
		if _, ok := record["event"]; !ok {
			return nil, fmt.Errorf("audit JSONL line %d has no event", number+1)
		}
		records = append(records, record)
	}
	return records, nil
}

func auditLogIsJSONL(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	_, err = readAudit(h)
	return err
}

func auditContainsSessionEvent(ctx context.Context, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	records, err := readAudit(h)
	if err != nil {
		return err
	}
	for _, record := range records {
		var event, sessionID string
		_ = json.Unmarshal(record["event"], &event)
		_ = json.Unmarshal(record["session_id"], &sessionID)
		if event == want && sessionID != "" {
			return nil
		}
	}
	return fmt.Errorf("audit log has no %q event with a session_id", want)
}

func expectedMode(value string) (fs.FileMode, error) {
	parsed, err := strconv.ParseUint(value, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("parse mode %q: %w", value, err)
	}
	return fs.FileMode(parsed), nil
}

func assertMode(path, expected string) error {
	want, err := expectedMode(expected)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		return fmt.Errorf("mode of %s = %04o, want %04o", path, got, want)
	}
	return nil
}

func scenarioHomeMode(ctx context.Context, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	return assertMode(h.Home, want)
}

func stateDatabaseMode(ctx context.Context, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	return assertMode(filepath.Join(h.Home, "state.db"), want)
}

func clientCreatesShellSession(ctx context.Context, clientName, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	cwd := filepath.Join(h.Home, "walking-skeleton-cwd")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		return fmt.Errorf("create scenario working directory: %w", err)
	}
	h.workingDir, h.sentinel = cwd, []byte("deck must not alter user cwd\n")
	if err := os.WriteFile(filepath.Join(cwd, "sentinel"), h.sentinel, 0o600); err != nil {
		return fmt.Errorf("write working-directory sentinel: %w", err)
	}
	if err := client.Send("n"); err != nil {
		return err
	}
	if err := client.WaitForFrame(ctx, false, "Create shell session"); err != nil {
		return err
	}
	if err := client.Send(name); err != nil {
		return err
	}
	time.Sleep(75 * time.Millisecond)
	if err := client.Send("\t" + cwd + "\r"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "starting")
}

// clientAttemptsShellSession drives the real modal through a second name that
// normalizes to an existing slug. The rendered error is the observable UI
// contract; this step neither calls store code nor infers collision locally.
func clientAttemptsShellSession(ctx context.Context, clientName, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if h.workingDir == "" {
		return errors.New("no scenario working directory was created")
	}
	if err := client.Send("n"); err != nil {
		return err
	}
	if err := client.WaitForFrame(ctx, false, "Create shell session"); err != nil {
		return err
	}
	if err := client.Send(name); err != nil {
		return err
	}
	time.Sleep(75 * time.Millisecond)
	if err := client.Send("\t" + h.workingDir + "\r"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "name collides with existing slug")
}

func clientClosesCreateModal(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send("\x1b"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

func startClientWithoutTMux(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	path := filepath.Join(h.Home, "missing-tmux-bin")
	if err := os.Mkdir(path, 0o700); err != nil {
		return fmt.Errorf("create missing-tmux PATH: %w", err)
	}
	return startClientWithPath(ctx, h, name, path)
}

func startClientWithTMuxVersion(ctx context.Context, name, version string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	path := filepath.Join(h.Home, "old-tmux-bin")
	if err := os.Mkdir(path, 0o700); err != nil {
		return fmt.Errorf("create old-tmux PATH: %w", err)
	}
	script := "#!/bin/sh\nprintf 'tmux " + version + "\\n'\n"
	if err := os.WriteFile(filepath.Join(path, "tmux"), []byte(script), 0o700); err != nil {
		return fmt.Errorf("write old tmux fixture: %w", err)
	}
	return startClientWithPath(ctx, h, name, path)
}

func startClientWithPath(ctx context.Context, h *ScenarioHarness, name, path string) error {
	client, err := h.StartNamedClient(ctx, name, "PATH="+path)
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

func clientAttachesAndDetaches(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send("\r"); err != nil {
		return err
	}
	// Bubble Tea must first hand the terminal to tmux before pane input is
	// meaningful; sending both batches together can deliver the command to the
	// departing TUI instead of the attached shell.
	time.Sleep(250 * time.Millisecond)
	if err := client.Send("echo walking-skeleton-attached\r"); err != nil {
		return err
	}
	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.WaitForFrame(wait, false, "walking-skeleton-attached"); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	if err := client.Send("\x02d"); err != nil {
		return err
	}
	return client.WaitForFrame(wait, false, "deck - sessions")
}

func clientKillsSelectedSession(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send("x"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "resumable")
}

// clientIsKilledWithSIGKILL models an ungraceful peer loss. Once reaped, the
// driver is removed from harness ownership because its non-zero exit is the
// expected observable outcome, not a teardown leak.
func clientIsKilledWithSIGKILL(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if client.cmd == nil || client.cmd.Process == nil {
		return fmt.Errorf("deck client %q has no process to kill", name)
	}
	if err := client.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("SIGKILL deck client %q: %w", name, err)
	}
	select {
	case <-client.done:
	case <-ctx.Done():
		return fmt.Errorf("wait for SIGKILLed deck client %q: %w", name, ctx.Err())
	}
	if client.terminal != nil {
		_ = client.terminal.Close()
	}
	for index, owned := range h.clients {
		if owned == client {
			h.clients = append(h.clients[:index], h.clients[index+1:]...)
			break
		}
	}
	delete(h.namedClients, name)
	return nil
}

func createdSentinelIsUnchanged(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	if h.workingDir == "" {
		return errors.New("no scenario working directory was created")
	}
	contents, err := os.ReadFile(filepath.Join(h.workingDir, "sentinel"))
	if err != nil {
		return fmt.Errorf("read working-directory sentinel: %w", err)
	}
	if string(contents) != string(h.sentinel) {
		return fmt.Errorf("working-directory sentinel changed: got %q, want %q", contents, h.sentinel)
	}
	return nil
}

func clientExitsCleanly(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send("q"); err != nil {
		return err
	}
	if err := client.Stop(5 * time.Second); err != nil {
		return fmt.Errorf("deck client %q did not exit cleanly: %w", name, err)
	}
	return nil
}

// This is a focused regression test for the assertion surface itself. It
// creates a real session through the released TUI, then observes every fact
// through tmux, SQLite, JSONL, and permissions without product imports.
func TestBlackBoxAssertionsObserveRealSession(t *testing.T) {
	h, err := newScenarioHarness(buildDeckBinary(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(h.Binary) }()
	defer func() {
		if err := h.Close(); err != nil {
			t.Error(err)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := h.StartNamedClient(ctx, "A")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.WaitForFrame(ctx, false, "No sessions"); err != nil {
		t.Fatal(err)
	}
	cwd := t.TempDir()
	if err := client.Send("n"); err != nil {
		t.Fatal(err)
	}
	if err := client.WaitForFrame(ctx, false, "Create shell session"); err != nil {
		t.Fatal(err)
	}
	if err := client.Send("black-box"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond) // separate real terminal key batches
	if err := client.Send("\t"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := client.Send(cwd); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := client.Send("\r"); err != nil {
		t.Fatal(err)
	}
	stepCtx := context.WithValue(ctx, scenarioHarnessKey{}, h)
	if err := waitForPrivateSession(stepCtx, "deck_black-box"); err != nil {
		t.Fatalf("%v\nframe:\n%s", err, client.Frame(false))
	}
	if err := privateSessionExists(stepCtx, "deck_black-box"); err != nil {
		t.Fatal(err)
	}
	if err := privateSessionPaneCWD(stepCtx, "deck_black-box", cwd); err != nil {
		t.Fatal(err)
	}
	if err := privateOptionIs(stepCtx, "exit-empty", "off"); err != nil {
		t.Fatal(err)
	}
	if err := databaseSchemaVersion(stepCtx, 1); err != nil {
		t.Fatal(err)
	}
	if err := databaseJournalMode(stepCtx, "wal"); err != nil {
		t.Fatal(err)
	}
	if err := databaseSessionStatus(stepCtx, "black-box", "starting"); err != nil {
		t.Fatal(err)
	}
	if err := auditLogIsJSONL(stepCtx); err != nil {
		t.Fatal(err)
	}
	if err := auditContainsSessionEvent(stepCtx, "launch"); err != nil {
		t.Fatal(err)
	}
	if err := scenarioHomeMode(stepCtx, "700"); err != nil {
		t.Fatal(err)
	}
	if err := stateDatabaseMode(stepCtx, "600"); err != nil {
		t.Fatal(err)
	}
	if err := client.Send("q"); err != nil {
		t.Fatal(err)
	}
	if err := client.Stop(3 * time.Second); err != nil {
		t.Fatal(err)
	}
}
