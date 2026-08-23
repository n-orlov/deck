//go:build deckinputcount

package tui

import (
	"fmt"
	"os"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// i1TraceFileEnvironment names an append-only log of every Msg Model.Update
// sees, one line each, timestamped and naming the model's selected index
// before and after -- built purely for I-1's continuation (task 005): once
// task 003/004's byte-level counters proved a dropped keystroke DOES reach
// this Update loop in several reproductions, the remaining question is
// which OTHER message interleaves between the marking keystroke and the
// navigation keystroke and resets state. It is a no-op unless
// DECK_I1_TRACE_FILE is set, which no documented deck control ever sets,
// exactly like inputcount_hook.go's own instrument.
const i1TraceFileEnvironment = "DECK_I1_TRACE_FILE"

var (
	i1TraceMu   sync.Mutex
	i1TraceFile *os.File
	i1TraceOnce sync.Once
)

func i1Trace(label string, message tea.Msg, selectedBefore int) {
	path := os.Getenv(i1TraceFileEnvironment)
	if path == "" {
		return
	}
	i1TraceOnce.Do(func() {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err == nil {
			i1TraceFile = f
		}
	})
	if i1TraceFile == nil {
		return
	}
	i1TraceMu.Lock()
	defer i1TraceMu.Unlock()
	fmt.Fprintf(i1TraceFile, "%d %s %s selBefore=%d\n", time.Now().UnixNano(), label, describeMsg(message), selectedBefore)
}

func describeMsg(message tea.Msg) string {
	switch msg := message.(type) {
	case tea.KeyMsg:
		return fmt.Sprintf("KeyMsg(%q)", msg.String())
	case debugMsg:
		return string(msg)
	case sessionsLoaded:
		parts := make([]string, 0, len(msg.sessions))
		for _, s := range msg.sessions {
			parts = append(parts, fmt.Sprintf("%s:%s@%d", s.Name, s.Status, s.StatusAt))
		}
		return fmt.Sprintf("sessionsLoaded%v", parts)
	default:
		return fmt.Sprintf("%T", message)
	}
}

// i1TraceSessions logs m.sessions' current name/status/statusAt in index
// order every Update call, so a trace log can show exactly which index
// held which session (and its status) at the instant any given message was
// dispatched -- needed to correlate an index-based m.selected change with
// which named session actually moved.
func i1TraceSessions(m Model) {
	path := os.Getenv(i1TraceFileEnvironment)
	if path == "" {
		return
	}
	parts := make([]string, 0, len(m.sessions))
	for i, s := range m.sessions {
		parts = append(parts, fmt.Sprintf("%d:%s:%s@%d", i, s.Name, s.Status, s.StatusAt))
	}
	i1Trace("sessions", debugMsg(fmt.Sprintf("%v", parts)), m.selected)
}

type debugMsg string
