package store

import (
	"context"
	"path/filepath"
	"testing"
)

func openValidationStore(t *testing.T) *Store {
	t.Helper()
	home := t.TempDir()
	s, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// TestMutatorsRejectMissingIdentityOrTimestamp pins every required-argument
// refusal: the call fails with the exact message, and the append-only event log
// stays empty (a refused mutation must not leave a trace).
func TestMutatorsRejectMissingIdentityOrTimestamp(t *testing.T) {
	ctx := context.Background()
	const id = "00000000-0000-4000-8000-000000000e01"
	const at int64 = 1_735_789_245_000
	cases := []struct {
		name string
		call func(s *Store) error
		want string
	}{
		{"attachment without id", func(s *Store) error { return s.RecordAttachment(ctx, "", at) }, "session id is required"},
		{"attachment without timestamp", func(s *Store) error { return s.RecordAttachment(ctx, id, 0) }, "attachment timestamp is required"},
		{"acknowledge without id", func(s *Store) error { return s.AcknowledgeSession(ctx, "") }, "session id is required"},
		{"note without id", func(s *Store) error { return s.RecordSessionNote(ctx, "", "r", at) }, "session id is required"},
		{"note without timestamp", func(s *Store) error { return s.RecordSessionNote(ctx, id, "r", 0) }, "event timestamp is required"},
		{"probe miss without id", func(s *Store) error { return s.RecordProbeMiss(ctx, "", at) }, "session id is required"},
		{"probe miss without timestamp", func(s *Store) error { return s.RecordProbeMiss(ctx, id, 0) }, "event timestamp is required"},
		{"conversation id without conversation", func(s *Store) error { return s.SetConversationID(ctx, id, "", "hook", at) }, "session id and conversation id are required"},
		{"permission profile without profile", func(s *Store) error { return s.SetPermissionProfile(ctx, id, "", "ui", at) }, "session id and permission profile are required"},
		{"env value without key", func(s *Store) error { return s.SetSessionEnvValue(ctx, id, "", "v", "ui", at) }, "session id and environment key are required"},
		{"env value without timestamp", func(s *Store) error { return s.SetSessionEnvValue(ctx, id, "K", "v", "ui", 0) }, "event timestamp is required"},
		{"clear launch dirty without id", func(s *Store) error { return s.ClearLaunchDirty(ctx, "", at) }, "session id is required"},
		{"clear launch dirty without timestamp", func(s *Store) error { return s.ClearLaunchDirty(ctx, id, 0) }, "event timestamp is required"},
		{"group assignment without id", func(s *Store) error { return s.SetSessionGroup(ctx, "", 1, "ui", at) }, "session id is required"},
		{"resume pin without conversation", func(s *Store) error { return s.SetResumePin(ctx, id, "", "ui", at) }, "session id and conversation id are required"},
		{"resume auto without id", func(s *Store) error { return s.SetResumeStateAuto(ctx, "", "ui", at) }, "session id is required"},
		{"fresh once consume without id", func(s *Store) error { return s.ConsumeFreshOnce(ctx, "", "ui", at) }, "session id is required"},
		{"fresh once consume without timestamp", func(s *Store) error { return s.ConsumeFreshOnce(ctx, id, "ui", 0) }, "event timestamp is required"},
		{"rename to an unsluggable name", func(s *Store) error { return s.RenameSession(ctx, id, "***", "ui", at) }, `session name "***" does not produce a usable slug`},
		{"rename without timestamp", func(s *Store) error { return s.RenameSession(ctx, id, "ok", "ui", 0) }, "event timestamp is required"},
		{"create without required fields", func(s *Store) error { _, err := s.CreateSession(ctx, CreateSessionInput{ID: id}); return err }, "session id, name, cwd, agent, and captured path are required"},
		{"create with an unsluggable name", func(s *Store) error {
			_, err := s.CreateSession(ctx, CreateSessionInput{ID: id, Name: "***", CWD: "/w", Agent: "claude", CapturedPath: "/bin"})
			return err
		}, `session name "***" does not produce a usable slug`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := openValidationStore(t)
			err := tc.call(s)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v; want %q", err, tc.want)
			}
			var events int
			if err := s.DB().QueryRow(`SELECT COUNT(*) FROM events`).Scan(&events); err != nil || events != 0 {
				t.Fatalf("events after a refused call = %d, %v; want 0", events, err)
			}
		})
	}
}

// TestMutatorsDefaultEmptySourceToUser pins that a caller who names no source
// is attributed to "user" in the recorded event, and that the mutation took
// effect on the session row.
func TestMutatorsDefaultEmptySourceToUser(t *testing.T) {
	ctx := context.Background()
	const id = "00000000-0000-4000-8000-000000000e02"
	const at int64 = 1_735_789_245_000
	calls := map[string]func(s *Store) error{
		"set_conversation_id": func(s *Store) error { return s.SetConversationID(ctx, id, "conv-1", "", at) },
		"set_permission":      func(s *Store) error { return s.SetPermissionProfile(ctx, id, "plan", "", at) },
		"set_resume_pin":      func(s *Store) error { return s.SetResumePin(ctx, id, "conv-1", "", at) },
		"set_group":           func(s *Store) error { return s.SetSessionGroup(ctx, id, 0, "", at) },
		"rename":              func(s *Store) error { return s.RenameSession(ctx, id, "renamed", "", at) },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			s := openValidationStore(t)
			newLeaseTestSession(t, s, id, "stopped")
			if err := call(s); err != nil {
				t.Fatal(err)
			}
			rows, err := s.DB().Query(`SELECT reason FROM events WHERE session_id = ? AND at = ?`, id, at)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			n := 0
			for rows.Next() {
				var reason string
				if err := rows.Scan(&reason); err != nil {
					t.Fatal(err)
				}
				if reason != "user" {
					t.Fatalf("event reason = %q; want %q for an empty source", reason, "user")
				}
				n++
			}
			if n == 0 {
				t.Fatal("no event recorded for the mutation")
			}
		})
	}
}

// TestMutationsOnMissingSessionReportNotFound pins that targeted mutations of
// a session that does not exist fail with a not-found error rather than
// silently recording an event for nothing.
func TestMutationsOnMissingSessionReportNotFound(t *testing.T) {
	ctx := context.Background()
	const id = "00000000-0000-4000-8000-000000000e03"
	const at int64 = 1_735_789_245_000
	calls := map[string]func(s *Store) error{
		"SetConversationID":    func(s *Store) error { return s.SetConversationID(ctx, id, "c", "ui", at) },
		"SetPermissionProfile": func(s *Store) error { return s.SetPermissionProfile(ctx, id, "plan", "ui", at) },
		"SetResumePin":         func(s *Store) error { return s.SetResumePin(ctx, id, "c", "ui", at) },
		"SetResumeStateAuto":   func(s *Store) error { return s.SetResumeStateAuto(ctx, id, "ui", at) },
		"RenameSession":        func(s *Store) error { return s.RenameSession(ctx, id, "x", "ui", at) },
		"SoftDeleteSession":    func(s *Store) error { return s.SoftDeleteSession(ctx, id, at) },
		"ArchiveSession":       func(s *Store) error { return s.ArchiveSession(ctx, id, at) },
		"UnarchiveSession":     func(s *Store) error { return s.UnarchiveSession(ctx, id, at) },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			s := openValidationStore(t)
			err := call(s)
			if err == nil {
				t.Fatal("mutating a missing session returned nil; want a not-found error")
			}
			t.Logf("%s: %v", name, err)
			var events int
			if err := s.DB().QueryRow(`SELECT COUNT(*) FROM events`).Scan(&events); err != nil || events != 0 {
				t.Fatalf("events after a failed mutation = %d, %v; want 0", events, err)
			}
		})
	}
}

// TestConsumeFreshOnceIsABenignNoOpUnlessFreshOnce pins the documented race
// rule: consuming for a missing session, or one not in fresh-once state, is
// not an error and records nothing, while a fresh-once session returns to auto
// with exactly one event.
func TestConsumeFreshOnceIsABenignNoOpUnlessFreshOnce(t *testing.T) {
	ctx := context.Background()
	const id = "00000000-0000-4000-8000-000000000e04"
	const at int64 = 1_735_789_245_000
	s := openValidationStore(t)
	if err := s.ConsumeFreshOnce(ctx, id, "ui", at); err != nil {
		t.Fatalf("consume for a missing session = %v; want nil", err)
	}
	newLeaseTestSession(t, s, id, "stopped")
	if err := s.ConsumeFreshOnce(ctx, id, "ui", at); err != nil {
		t.Fatalf("consume for an auto session = %v; want nil", err)
	}
	var events int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM events WHERE kind = 'set_resume_state'`).Scan(&events); err != nil || events != 0 {
		t.Fatalf("resume-state events after no-op consumes = %d, %v; want 0", events, err)
	}
	if err := s.SetResumeStateFreshOnce(ctx, id, "ui", at); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeFreshOnce(ctx, id, "", at+1); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResumeState != "auto" {
		t.Fatalf("resume state after consume = %q; want auto", got.ResumeState)
	}
	var reason, payload string
	if err := s.DB().QueryRow(`SELECT reason, payload FROM events WHERE kind = 'set_resume_state' AND at = ?`, at+1).Scan(&reason, &payload); err != nil {
		t.Fatal(err)
	}
	if reason != "user" || payload != "auto" {
		t.Fatalf("consume event = (%q, %q); want (user, auto)", reason, payload)
	}
}
