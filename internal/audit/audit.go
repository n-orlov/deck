// Package audit writes deck's append-only structured observability log.
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/n-orlov/deck/internal/config"
)

const fileName = "deck.jsonl"

// Logger appends one JSON object per line to $DECK_HOME/log/deck.jsonl.  It is
// safe for concurrent callers in one process. The append flag also means that
// independently started deck clients never truncate prior audit records.
type Logger struct {
	path  string
	clock *config.Clock
	mu    sync.Mutex
}

// Record is the common envelope for every audit line. DurationMS is deliberately
// derived from Clock.Elapsed rather than its wall clock, so it remains useful
// when DECK_CLOCK freezes displayed timestamps.
type Record struct {
	Event      string `json:"event"`
	SessionID  string `json:"session_id,omitempty"`
	Timestamp  string `json:"timestamp"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

// LaunchRecord is written for every command launched in a pane. EnvKeys holds
// names only: callers must not put environment values in the audit log.
type LaunchRecord struct {
	Record
	Argv    []string `json:"argv"`
	EnvKeys []string `json:"env_keys"`
}

// New creates the log directory if needed and returns a logger rooted at paths.
// It intentionally does not retain an open descriptor: each append is visible
// promptly to another deck client or an external black-box observer.
func New(paths config.Paths, clock *config.Clock) (*Logger, error) {
	if clock == nil {
		return nil, fmt.Errorf("audit logger requires a clock")
	}
	if err := os.MkdirAll(paths.LogDir, 0o700); err != nil {
		return nil, fmt.Errorf("create audit log directory: %w", err)
	}
	return &Logger{path: filepath.Join(paths.LogDir, fileName), clock: clock}, nil
}

// Path returns the JSONL file path for external observers and diagnostics.
func (l *Logger) Path() string { return l.path }

// Transition records a session state transition. sessionID is required because
// a transition always applies to one durable session.
func (l *Logger) Transition(sessionID, event string) error {
	if sessionID == "" {
		return fmt.Errorf("audit transition requires a session id")
	}
	return l.write(Record{Event: event, SessionID: sessionID, Timestamp: l.clock.Now().Format(time.RFC3339Nano), DurationMS: positiveMilliseconds(l.clock.Elapsed())})
}

// Event records an event that may not belong to a session, such as startup.
func (l *Logger) Event(event string) error {
	return l.write(Record{Event: event, Timestamp: l.clock.Now().Format(time.RFC3339Nano), DurationMS: positiveMilliseconds(l.clock.Elapsed())})
}

// Launch records the exact argv and resolved environment variable names. It
// copies and sorts key names, producing deterministic output and preventing a
// mutable caller slice from changing the record during encoding.
func (l *Logger) Launch(sessionID string, argv []string, env map[string]string) error {
	if sessionID == "" {
		return fmt.Errorf("audit launch requires a session id")
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return l.write(LaunchRecord{
		Record: Record{Event: "launch", SessionID: sessionID, Timestamp: l.clock.Now().Format(time.RFC3339Nano), DurationMS: positiveMilliseconds(l.clock.Elapsed())},
		Argv:   append([]string(nil), argv...), EnvKeys: keys,
	})
}

func positiveMilliseconds(elapsed time.Duration) int64 {
	milliseconds := elapsed.Milliseconds()
	if milliseconds < 1 {
		return 1
	}
	return milliseconds
}

func (l *Logger) write(value any) error {
	// Marshal first: a failed marshal must not create a partial JSONL line.
	line, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode audit record: %w", err)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("append audit record: %w", err)
	}
	return nil
}
