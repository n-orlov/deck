package tui

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestSharedStateDBGroupEditsVisibleAcrossClients is task 020's own proof of
// R131's shared-database paragraph
// (prds/phase4b-manual-groups-and-scroll-cue.md, R131 section): "Multiple
// deck clients share one state.db, so a group edit in one is picked up by
// the others on their next reload; R128's dangling-group_id rule is what
// makes that safe." Two *store.Store handles are opened through
// store.OpenPath on the SAME on-disk state.db file (client A and client B),
// standing in for two independently-running deck processes -- never a
// single shared handle, which would prove nothing about cross-process
// visibility.
//
// Three cases, each driven by an edit on A's handle and observed on B's
// handle on B's next reload:
//
//  1. create -- a group A creates appears in B's own ListGroups the next
//     time B calls it.
//  2. rename -- A renames a group with one member; B's next ListGroups
//     shows the new name, and the member's own group_id (read back via B's
//     ListSessions) is untouched -- R128/store/group.go's RenameGroup doc
//     comment: "membership is by id, not name, so a rename is one row
//     update and carries every member for free".
//  3. delete -- A deletes a group out from under a member session
//     (store.DeleteGroup never touches member rows, per its own doc
//     comment) and B's reload must resolve that member under the implicit
//     default group rather than vanishing it or panicking. This case is
//     driven through the TUI's own reload path -- B's Model.loadSessions
//     producing a sessionsLoaded message fed through Model.Update, exactly
//     like a real client's periodic reload -- not a bare store call, so it
//     also proves the render path (sessionGroupLabel/groupSessions)
//     degrades correctly, not just the SQL.
func TestSharedStateDBGroupEditsVisibleAcrossClients(t *testing.T) {
	home := t.TempDir()
	dbPath := filepath.Join(home, "state.db")

	clientA, err := store.OpenPath(home, dbPath)
	if err != nil {
		t.Fatalf("client A OpenPath: %v", err)
	}
	t.Cleanup(func() { clientA.Close() })

	clientB, err := store.OpenPath(home, dbPath)
	if err != nil {
		t.Fatalf("client B OpenPath: %v", err)
	}
	t.Cleanup(func() { clientB.Close() })

	ctx := context.Background()

	// --- Case 1: create, seen by B's next ListGroups ---
	created, err := clientA.CreateGroup(ctx, "tooling")
	if err != nil {
		t.Fatalf("client A CreateGroup: %v", err)
	}

	groupsSeenByB, err := clientB.ListGroups(ctx)
	if err != nil {
		t.Fatalf("client B ListGroups after A's create: %v", err)
	}
	if !groupListContains(groupsSeenByB, created.ID, "tooling") {
		t.Fatalf("client B's group list after A's create = %+v, want it to contain {%d tooling}", groupsSeenByB, created.ID)
	}

	// --- Case 2: rename, with one member whose group_id must not move ---
	const memberID = "00000000-0000-4000-8000-000000000b01"
	if _, err := clientA.CreateSession(ctx, store.CreateSessionInput{
		ID: memberID, Name: "member-of-tooling", CWD: "/work/tooling", Agent: "shell",
		CapturedPath: "/bin", StatusAt: 1, CreatedAt: 1, GroupID: &created.ID,
	}); err != nil {
		t.Fatalf("client A CreateSession: %v", err)
	}

	if err := clientA.RenameGroup(ctx, created.ID, "tooling-renamed"); err != nil {
		t.Fatalf("client A RenameGroup: %v", err)
	}

	groupsSeenByB, err = clientB.ListGroups(ctx)
	if err != nil {
		t.Fatalf("client B ListGroups after A's rename: %v", err)
	}
	if !groupListContains(groupsSeenByB, created.ID, "tooling-renamed") {
		t.Fatalf("client B's group list after A's rename = %+v, want it to contain {%d tooling-renamed}", groupsSeenByB, created.ID)
	}

	sessionsSeenByB, err := clientB.ListSessions(ctx)
	if err != nil {
		t.Fatalf("client B ListSessions after A's rename: %v", err)
	}
	member := findSessionByID(t, sessionsSeenByB, memberID)
	if member.GroupID == nil || *member.GroupID != created.ID {
		t.Fatalf("member's group_id after A's rename (seen by B) = %v, want unchanged %d (R128: membership is by id, not name)", member.GroupID, created.ID)
	}
	if member.GroupName != "tooling-renamed" {
		t.Fatalf("member's resolved group name (seen by B) = %q, want %q", member.GroupName, "tooling-renamed")
	}

	// --- Case 3: delete, observed through the TUI's own reload path ---
	if err := clientA.DeleteGroup(ctx, created.ID); err != nil {
		t.Fatalf("client A DeleteGroup: %v", err)
	}

	mb := New(clientB, config.Settings{}, "")
	mb.width, mb.height = 100, 40

	msg := mb.loadSessions()
	loaded, ok := msg.(sessionsLoaded)
	if !ok {
		t.Fatalf("client B's loadSessions() = %T, want sessionsLoaded", msg)
	}
	if loaded.err != nil {
		t.Fatalf("client B's loadSessions() reported %v", loaded.err)
	}

	next, _ := mb.Update(loaded)
	mb = next.(Model)

	memberAfterDelete := findSessionByID(t, mb.sessions, memberID)
	if got := sessionGroupLabel(memberAfterDelete); got != "default" {
		t.Fatalf("member's rendered group label after A deleted its group = %q, want %q (dangling group_id degrades to default, never vanishes)", got, "default")
	}

	groups := mb.groupSessions()
	var foundInDefault bool
	for _, g := range groups {
		if g.GroupID != 0 {
			continue
		}
		for _, is := range g.Sessions {
			if is.Session.ID == memberID {
				foundInDefault = true
			}
		}
	}
	if !foundInDefault {
		t.Fatalf("member session %q not found under the default bucket (GroupID 0) in groupSessions() after A deleted its group -- it must degrade to default, never vanish", memberID)
	}
}

func groupListContains(groups []store.Group, id int64, name string) bool {
	for _, g := range groups {
		if g.ID == id && g.Name == name {
			return true
		}
	}
	return false
}

func findSessionByID(t *testing.T, sessions []store.Session, id string) store.Session {
	t.Helper()
	for _, s := range sessions {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("session %q not found among %d sessions", id, len(sessions))
	return store.Session{}
}
