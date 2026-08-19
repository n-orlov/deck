package features

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/cucumber/godog"
)

const crashFixtureName = "colored-crash.txt"

func registerCrashStatusSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a crash-tail fixture and long-running fake Claude are configured$`, configureCrashFixture)
	sc.Step(`^shell session "([^"]+)" exits with status zero$`, shellSessionExitsZero)
	sc.Step(`^fake Claude session "([^"]+)" renders the colored crash-tail fixture$`, renderColoredCrashFixture)
	sc.Step(`^the state database session "([^"]+)" is cleanly stopped without a crash artifact$`, databaseSessionCleanlyStopped)
	sc.Step(`^the state database session "([^"]+)" has a sanitized last-200-line crash artifact$`, databaseSessionHasCrashArtifact)
	sc.Step(`^the crash artifact for session "([^"]+)" remains unchanged across another reconcile interval$`, crashArtifactRemainsUnchanged)
}

func configureCrashFixture(ctx context.Context) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	if err := installFakeClaudeOnPATH(ctx, true); err != nil {
		return err
	}
	fixtureDir := filepath.Join(h.Home, "crash-fixtures")
	if err := os.MkdirAll(fixtureDir, 0o700); err != nil {
		return fmt.Errorf("create crash fixture directory: %w", err)
	}
	var fixture strings.Builder
	for line := 1; line <= 205; line++ {
		fmt.Fprintf(&fixture, "fixture line %03d\n", line)
	}
	fixture.WriteString("\x1b[31mRED CRASH MARKER\x1b[0m\ncrash final line\n")
	if err := os.WriteFile(filepath.Join(fixtureDir, crashFixtureName), []byte(fixture.String()), 0o600); err != nil {
		return fmt.Errorf("write crash fixture: %w", err)
	}
	config := fmt.Sprintf("[env]\nFAKE_AGENT_FIXTURE_DIR = %q\n", fixtureDir)
	if err := os.WriteFile(filepath.Join(h.Home, "config.toml"), []byte(config), 0o600); err != nil {
		return fmt.Errorf("write crash fixture config: %w", err)
	}
	return nil
}

func shellSessionExitsZero(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	target := "deck_" + slug
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "-l", "exit"); err != nil {
		return fmt.Errorf("send clean exit to shell %q: %w", name, err)
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "Enter"); err != nil {
		return fmt.Errorf("submit clean exit to shell %q: %w", name, err)
	}
	return nil
}

func renderColoredCrashFixture(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	target := "deck_" + slug
	request, _ := json.Marshal(map[string]string{"command": "fixture", "name": crashFixtureName})
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "-l", string(request)); err != nil {
		return err
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "Enter"); err != nil {
		return err
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		plain, plainErr := tmuxOutput(ctx, h, "capture-pane", "-p", "-S", "-", "-t", target)
		escaped, escapedErr := tmuxOutput(ctx, h, "capture-pane", "-p", "-e", "-S", "-", "-t", target)
		if plainErr == nil && escapedErr == nil && strings.Contains(string(plain), "crash final line") && strings.Contains(string(escaped), "\x1b[") {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("pane %q did not render colored crash fixture; plain error=%v escaped error=%v\nplain pane:\n%s", target, plainErr, escapedErr, plain)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func databaseSessionCleanlyStopped(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var status, source, tail string
		var exit sql.NullInt64
		err := db.QueryRowContext(ctx, `SELECT status, status_source, pane_exit_status, COALESCE(crash_tail, '') FROM sessions WHERE name = ?`, name).Scan(&status, &source, &exit, &tail)
		if err == nil && status == "stopped" && source == "tmux" && !exit.Valid && tail == "" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q clean-exit fields = status %q source %q exit=%v tail=%q err=%v", name, status, source, exit, tail, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

type observedCrashArtifact struct {
	Status string
	Source string
	Exit   int64
	Tail   string
}

func readCrashArtifact(ctx context.Context, db *sql.DB, name string) (observedCrashArtifact, error) {
	var got observedCrashArtifact
	var exit sql.NullInt64
	err := db.QueryRowContext(ctx, `SELECT status, status_source, pane_exit_status, COALESCE(crash_tail, '') FROM sessions WHERE name = ?`, name).Scan(&got.Status, &got.Source, &exit, &got.Tail)
	if err != nil {
		return got, err
	}
	if !exit.Valid {
		return got, fmt.Errorf("session %q has no pane_exit_status", name)
	}
	got.Exit = exit.Int64
	return got, nil
}

func databaseSessionHasCrashArtifact(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	deadline := time.Now().Add(3 * time.Second)
	var got observedCrashArtifact
	for {
		got, err = readCrashArtifact(ctx, db, name)
		if err == nil && got.Status == "error" && got.Source == "tmux" && got.Exit != 0 && strings.Contains(got.Tail, "RED CRASH MARKER") && strings.Contains(got.Tail, "crash final line") {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q crash artifact did not settle: %#v err=%v", name, got, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	lines := strings.Split(got.Tail, "\n")
	if len(lines) > 200 {
		return fmt.Errorf("session %q crash tail has %d lines, want at most 200", name, len(lines))
	}
	if strings.Contains(got.Tail, "fixture line 001") || !strings.Contains(got.Tail, "fixture line 205") {
		return fmt.Errorf("session %q crash tail is not the last 200 pane lines: first=%q last=%q", name, lines[0], lines[len(lines)-1])
	}
	if strings.ContainsRune(got.Tail, '\x1b') {
		return fmt.Errorf("session %q crash tail retained an ANSI escape", name)
	}
	for _, r := range got.Tail {
		if r != '\n' && r != '\t' && (r == utf8.RuneError || unicode.IsControl(r)) {
			return fmt.Errorf("session %q crash tail retained control rune %U", name, r)
		}
	}
	return nil
}

func crashArtifactRemainsUnchanged(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	before, err := readCrashArtifact(ctx, db, name)
	if err != nil {
		return err
	}
	timer := time.NewTimer(scenarioReconcileInterval + 100*time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	after, err := readCrashArtifact(ctx, db, name)
	if err != nil {
		return err
	}
	if after != before {
		return fmt.Errorf("session %q crash artifact changed after racing reconciles: before=%#v after=%#v", name, before, after)
	}
	return nil
}
