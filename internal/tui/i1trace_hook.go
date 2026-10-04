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

func i1Trace(label string, message tea.Msg, selectedBefore sidebarCursor) {
	f := i1TraceOpen()
	if f == nil {
		return
	}
	i1TraceMu.Lock()
	defer i1TraceMu.Unlock()
	fmt.Fprintf(f, "%d %s %s selBefore=%+v\n", time.Now().UnixNano(), label, describeMsg(message), selectedBefore)
}

// i1TraceOpen returns the trace log, opening it on first use, or nil when
// DECK_I1_TRACE_FILE is unset or the file could not be opened.
func i1TraceOpen() *os.File {
	path := os.Getenv(i1TraceFileEnvironment)
	if path == "" {
		return nil
	}
	i1TraceOnce.Do(func() { i1TraceFile = openI1TraceFile(path) })
	return i1TraceFile
}

func openI1TraceFile(path string) *os.File {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil
	}
	return f
}

func describeMsg(message tea.Msg) string {
	switch msg := message.(type) {
	case tea.KeyMsg:
		return fmt.Sprintf("KeyMsg(%q)", msg.String())
	case debugMsg:
		return string(msg)
	default:
		return describeOtherMsg(message)
	}
}

func describeOtherMsg(message tea.Msg) string {
	if msg, ok := message.(sessionsLoaded); ok {
		return describeSessionsLoaded(msg)
	}
	return fmt.Sprintf("%T", message)
}

func describeSessionsLoaded(msg sessionsLoaded) string {
	parts := make([]string, 0, len(msg.sessions))
	for _, s := range msg.sessions {
		parts = append(parts, fmt.Sprintf("%s:%s@%d", s.Name, s.Status, s.StatusAt))
	}
	return fmt.Sprintf("sessionsLoaded%v", parts)
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
