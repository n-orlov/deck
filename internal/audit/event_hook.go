package audit

import (
	"fmt"
	"time"
)

// EventHookInvocation is what one spawn of the configured event hook did
// (SPEC §10.3). Detached is a session-end spawn that deck started and did not
// wait for, so it has no exit status and no duration. Error is the scrubbed
// text of a spawn that never started; ExitCode is then -1.
type EventHookInvocation struct {
	Kind     string
	Detached bool
	ExitCode int
	TimedOut bool
	Duration time.Duration
	Error    string
}

// EventHookRecord is the JSONL line of one event-hook invocation. ExitCode
// and ScriptDurationMS are absent for a detached spawn: deck never learns
// either. ScriptDurationMS is the script's own monotonic run time, separate
// from Record.DurationMS (process lifetime). The script's output is never
// logged: it is stored against the event row, capped and scrubbed.
type EventHookRecord struct {
	Record
	Kind             string   `json:"kind"`
	Detached         bool     `json:"detached,omitempty"`
	ExitCode         *int     `json:"exit_code,omitempty"`
	TimedOut         bool     `json:"timed_out,omitempty"`
	ScriptDurationMS *float64 `json:"script_duration_ms,omitempty"`
	Error            string   `json:"error,omitempty"`
}

// EventHook records one event-hook invocation for sessionID.
func (l *Logger) EventHook(sessionID string, inv EventHookInvocation) error {
	if sessionID == "" {
		return fmt.Errorf("audit event hook requires a session id")
	}
	rec := EventHookRecord{
		Record: Record{
			Event:      "event_hook",
			SessionID:  sessionID,
			Timestamp:  l.clock.Now().Format(time.RFC3339Nano),
			DurationMS: positiveMilliseconds(l.clock.Elapsed()),
		},
		Kind:     inv.Kind,
		Detached: inv.Detached,
		Error:    inv.Error,
	}
	if !inv.Detached {
		exit := inv.ExitCode
		ms := float64(max(inv.Duration, 0)) / float64(time.Millisecond)
		rec.ExitCode, rec.TimedOut, rec.ScriptDurationMS = &exit, inv.TimedOut, &ms
	}
	return l.write(rec)
}
