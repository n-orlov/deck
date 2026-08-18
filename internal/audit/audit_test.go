package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
)

func TestJSONLRecordsTransitionsAndLaunchAudit(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	clock, err := config.NewClock("2030-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
	logger, err := New(config.Paths{Home: home, LogDir: filepath.Join(home, "log")}, clock)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Transition("session-1", "session.stopped"); err != nil {
		t.Fatal(err)
	}
	if err := logger.Launch("session-1", []string{"env", "MODE=fast", "/bin/sh", "-lc", "echo hello"}, map[string]string{"SECRET_TOKEN": "must-not-appear", "MODE": "fast"}); err != nil {
		t.Fatal(err)
	}

	file, err := os.Open(logger.Path())
	if err != nil {
		t.Fatalf("open JSONL audit log: %v", err)
	}
	defer file.Close()
	var records []map[string]any
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("line is not one JSON object: %v", err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("record count = %d, want 2", len(records))
	}
	transition, launch := records[0], records[1]
	if transition["event"] != "session.stopped" || transition["session_id"] != "session-1" {
		t.Errorf("transition = %#v", transition)
	}
	if launch["event"] != "launch" || launch["session_id"] != "session-1" {
		t.Errorf("launch envelope = %#v", launch)
	}
	if got := launch["argv"]; !equalJSONStrings(got, []string{"env", "MODE=fast", "/bin/sh", "-lc", "echo hello"}) {
		t.Errorf("launch argv = %#v", got)
	}
	if got := launch["env_keys"]; !equalJSONStrings(got, []string{"MODE", "SECRET_TOKEN"}) {
		t.Errorf("launch env keys = %#v", got)
	}
	contents, err := os.ReadFile(logger.Path())
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) == "" || contains(string(contents), "must-not-appear") {
		t.Errorf("environment values leaked to log: %q", contents)
	}
}

func TestDurationAdvancesWithFrozenWallClock(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	clock, err := config.NewClock("2030-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
	logger, err := New(config.Paths{Home: home, LogDir: filepath.Join(home, "log")}, clock)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Event("started"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(15 * time.Millisecond)
	if err := logger.Event("finished"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(logger.Path())
	if err != nil {
		t.Fatal(err)
	}
	var records []map[string]any
	for _, line := range splitLines(string(data)) {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if records[0]["timestamp"] != records[1]["timestamp"] {
		t.Errorf("frozen timestamps differ: %v, %v", records[0]["timestamp"], records[1]["timestamp"])
	}
	first, second := records[0]["duration_ms"].(float64), records[1]["duration_ms"].(float64)
	if first < 1 || second <= first {
		t.Errorf("durations must be positive and advancing: %v, %v", first, second)
	}
}

func TestSessionAuditRequiresSessionID(t *testing.T) {
	t.Parallel()
	clock, _ := config.NewClock("", "")
	logger, err := New(config.Paths{LogDir: filepath.Join(t.TempDir(), "log")}, clock)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Transition("", "session.stopped"); err == nil {
		t.Error("Transition accepted empty session id")
	}
	if err := logger.Launch("", nil, nil); err == nil {
		t.Error("Launch accepted empty session id")
	}
}

func equalJSONStrings(value any, want []string) bool {
	got, ok := value.([]any)
	if !ok || len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func contains(value, needle string) bool {
	return len(needle) > 0 && len(value) >= len(needle) && (func() bool { return index(value, needle) >= 0 })()
}

func index(value, needle string) int {
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func splitLines(value string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(value); i++ {
		if value[i] == '\n' {
			if i > start {
				lines = append(lines, value[start:i])
			}
			start = i + 1
		}
	}
	if start < len(value) {
		lines = append(lines, value[start:])
	}
	return lines
}
