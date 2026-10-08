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

func readRecords(t *testing.T, path string) []map[string]any {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
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
	return records
}

// TestEventHookRecordsExitStatusAndDuration pins SPEC §13.1's structured log
// row: an attached invocation carries its kind, exit status, timeout flag and
// the script's own duration; a detached one carries neither, and a spawn that
// never started carries -1 and its error text.
func TestEventHookRecordsExitStatusAndDuration(t *testing.T) {
	t.Parallel()
	clock, err := config.NewClock("2030-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
	logger, err := New(config.Paths{LogDir: filepath.Join(t.TempDir(), "log")}, clock)
	if err != nil {
		t.Fatal(err)
	}
	invocations := []EventHookInvocation{
		{Kind: "waiting", ExitCode: 3, Duration: 1500 * time.Microsecond},
		{Kind: "error", ExitCode: -1, TimedOut: true, Duration: 2 * time.Second},
		{Kind: "ended", Detached: true, ExitCode: 7, Duration: time.Second},
		{Kind: "idle", ExitCode: -1, Error: "event hook: script not found"},
	}
	for _, inv := range invocations {
		if err := logger.EventHook("session-1", inv); err != nil {
			t.Fatal(err)
		}
	}
	if err := logger.EventHook("", invocations[0]); err == nil {
		t.Error("EventHook accepted an empty session id")
	}
	records := readRecords(t, logger.Path())
	if len(records) != len(invocations) {
		t.Fatalf("record count = %d, want %d", len(records), len(invocations))
	}
	for i, record := range records {
		if record["event"] != "event_hook" || record["session_id"] != "session-1" || record["kind"] != invocations[i].Kind {
			t.Errorf("record %d envelope = %#v", i, record)
		}
	}
	if got := records[0]; got["exit_code"] != float64(3) || got["script_duration_ms"] != 1.5 || got["timed_out"] != nil {
		t.Errorf("attached record = %#v, want exit_code 3, script_duration_ms 1.5, no timed_out", got)
	}
	if got := records[1]; got["exit_code"] != float64(-1) || got["timed_out"] != true || got["script_duration_ms"] != float64(2000) {
		t.Errorf("timed-out record = %#v", got)
	}
	if got := records[2]; got["detached"] != true || got["exit_code"] != nil || got["script_duration_ms"] != nil {
		t.Errorf("detached record = %#v, want no exit_code and no script_duration_ms", got)
	}
	if got := records[3]; got["exit_code"] != float64(-1) || got["error"] != "event hook: script not found" {
		t.Errorf("unstarted record = %#v", got)
	}
}
