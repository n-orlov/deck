package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// closedStore returns a Store whose database handle has been closed, so every
// statement the Store issues fails the way a vanished or unreadable state.db
// does at runtime.
func closedStore(t *testing.T) *Store {
	t.Helper()
	home := t.TempDir()
	s, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	return s
}

// TestStoreOperationsSurfaceDatabaseFailure pins that no Store operation
// swallows a database failure: with the handle closed each one must hand the
// underlying "database is closed" cause back to the caller rather than
// reporting success or an empty result.
func TestStoreOperationsSurfaceDatabaseFailure(t *testing.T) {
	ctx := context.Background()
	const id = "00000000-0000-4000-8000-000000000f01"
	const at int64 = 1_735_789_245_000
	ops := map[string]func(s *Store) error{
		"CreateSession": func(s *Store) error {
			_, err := s.CreateSession(ctx, CreateSessionInput{ID: id, Name: "closed", CWD: "/w", Agent: "claude", CapturedPath: "/bin", Status: "stopped", StatusAt: 1, CreatedAt: 1})
			return err
		},
		"GetSession": func(s *Store) error { _, err := s.GetSession(ctx, id); return err },
		"UpdateSessionStatus": func(s *Store) error {
			return s.UpdateSessionStatus(ctx, StatusUpdateInput{SessionID: id, Status: "working", Source: "hook", At: at})
		},
		"RecordAttachment":   func(s *Store) error { return s.RecordAttachment(ctx, id, at) },
		"AcknowledgeSession": func(s *Store) error { return s.AcknowledgeSession(ctx, id) },
		"RecordSessionNote":  func(s *Store) error { return s.RecordSessionNote(ctx, id, "note", at) },
		"RecordTranscript":   func(s *Store) error { return s.RecordTranscript(ctx, id, "/t/events.jsonl", at) },
		"RecordOrphanEvent": func(s *Store) error {
			return s.RecordOrphanEvent(ctx, EventInput{Kind: "orphan", At: at})
		},
		"RecordProbeMiss":            func(s *Store) error { return s.RecordProbeMiss(ctx, id, at) },
		"SetConversationID":          func(s *Store) error { return s.SetConversationID(ctx, id, "conv", "hook", at) },
		"SetPermissionProfileReason": func(s *Store) error { return s.SetPermissionProfileReason(ctx, id, "why", "probe", at) },
		"SetPermissionProfile":       func(s *Store) error { return s.SetPermissionProfile(ctx, id, "default", "ui", at) },
		"SetSessionEnvValue":         func(s *Store) error { return s.SetSessionEnvValue(ctx, id, "K", "v", "ui", at) },
		"ClearEnvDirty":              func(s *Store) error { return s.ClearEnvDirty(ctx, id, at) },
		"MarkEnvInjected":            func(s *Store) error { return s.MarkEnvInjected(ctx, id, at) },
		"DirtyEnvKeys":               func(s *Store) error { _, err := s.DirtyEnvKeys(ctx, id); return err },
		"SetLaunchInputs":            func(s *Store) error { return s.SetLaunchInputs(ctx, id, "pre", "post", []string{"-x"}, true, "ui", at) },
		"ClearLaunchDirty":           func(s *Store) error { return s.ClearLaunchDirty(ctx, id, at) },
		"SetSessionGroup":            func(s *Store) error { return s.SetSessionGroup(ctx, id, 1, "ui", at) },
		"SetSessionsPinned":          func(s *Store) error { return s.SetSessionsPinned(ctx, []string{id}, true, at) },
		"SetResumePin":               func(s *Store) error { return s.SetResumePin(ctx, id, "conv", "ui", at) },
		"SetResumeStateAuto":         func(s *Store) error { return s.SetResumeStateAuto(ctx, id, "ui", at) },
		"SetResumeFreshOnce":         func(s *Store) error { return s.SetResumeStateFreshOnce(ctx, id, "ui", at) },
		"ConsumeFreshOnce":           func(s *Store) error { return s.ConsumeFreshOnce(ctx, id, "ui", at) },
		"RenameSession":              func(s *Store) error { return s.RenameSession(ctx, id, "renamed", "ui", at) },
		"ListSessions":               func(s *Store) error { _, err := s.ListSessions(ctx); return err },
		"ListDeletedSessions":        func(s *Store) error { _, err := s.ListDeletedSessions(ctx); return err },
		"ListArchivedSessions":       func(s *Store) error { _, err := s.ListArchivedSessions(ctx); return err },
		"ListSessionsInclArch":       func(s *Store) error { _, err := s.ListSessionsIncludingArchived(ctx); return err },
		"SoftDeleteSession":          func(s *Store) error { return s.SoftDeleteSession(ctx, id, at) },
		"RestoreSession":             func(s *Store) error { return s.RestoreSession(ctx, id, at) },
		"ArchiveSession":             func(s *Store) error { return s.ArchiveSession(ctx, id, at) },
		"UnarchiveSession":           func(s *Store) error { return s.UnarchiveSession(ctx, id, at) },
		"ListEvents":                 func(s *Store) error { _, err := s.ListEvents(ctx, 10); return err },
		"LastAlarmingDroppedHook":    func(s *Store) error { _, _, err := s.LastAlarmingDroppedHook(ctx, id); return err },
		"EnforceEventRetention":      func(s *Store) error { return s.EnforceEventRetention(ctx, 30, at) },
		"SweepTombstones":            func(s *Store) error { _, err := s.SweepTombstones(ctx, time.Hour, at); return err },
		"DrainExpiredTombstones": func(s *Store) error {
			return s.DrainExpiredTombstones(ctx, time.Hour, at)
		},
		"TombstonedNameHolders": func(s *Store) error { _, err := s.TombstonedNameHolders(ctx, "n"); return err },
		"SessionRowExists":      func(s *Store) error { _, err := s.SessionRowExists(ctx, id); return err },
		"ReapSession":           func(s *Store) error { return s.ReapSession(ctx, id, at) },
		"GetLayoutMode":         func(s *Store) error { _, err := s.GetLayoutMode(ctx); return err },
		"SetLayoutMode":         func(s *Store) error { return s.SetLayoutMode(ctx, "split") },
		"GetSidebarWidth":       func(s *Store) error { _, err := s.GetSidebarWidth(ctx); return err },
		"SetSidebarWidth":       func(s *Store) error { return s.SetSidebarWidth(ctx, 30) },
		"GetLastCreateAgent":    func(s *Store) error { _, err := s.GetLastCreateAgent(ctx); return err },
		"SetLastCreateAgent":    func(s *Store) error { return s.SetLastCreateAgent(ctx, "claude") },
		"GetCollapsedGroups":    func(s *Store) error { _, err := s.GetCollapsedGroups(ctx); return err },
		"SetCollapsedGroups":    func(s *Store) error { return s.SetCollapsedGroups(ctx, map[int64]bool{1: true}) },
		"GetLastCreateGroup":    func(s *Store) error { _, err := s.GetLastCreateGroup(ctx); return err },
		"SetLastCreateGroup":    func(s *Store) error { return s.SetLastCreateGroup(ctx, 1) },
		"PromoteRecentCwd":      func(s *Store) error { return s.PromoteRecentCwd(ctx, "/w", 10) },
		"RecentCwds":            func(s *Store) error { _, err := s.RecentCwds(ctx); return err },
		"ClearRecentCwds":       func(s *Store) error { return s.ClearRecentCwds(ctx) },
		"CreateGroup":           func(s *Store) error { _, err := s.CreateGroup(ctx, "g"); return err },
		"RenameGroup":           func(s *Store) error { return s.RenameGroup(ctx, 1, "h") },
		"DeleteGroup":           func(s *Store) error { return s.DeleteGroup(ctx, 1) },
		"ListGroups":            func(s *Store) error { _, err := s.ListGroups(ctx); return err },
		"ReleaseLaunchLease":    func(s *Store) error { _, err := s.ReleaseLaunchLease(ctx, id, "1@b"); return err },
		"AcquireLaunchLease": func(s *Store) error {
			_, err := s.AcquireLaunchLease(ctx, id, "1@b", time.Minute, at)
			return err
		},
	}
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			err := op(closedStore(t))
			if err == nil {
				t.Fatal("operation on a closed database returned nil; want the database failure surfaced")
			}
			if !strings.Contains(err.Error(), "database is closed") {
				t.Fatalf("error = %v; want it to carry the underlying %q cause", err, "database is closed")
			}
		})
	}
}
