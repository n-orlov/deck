package store

import (
	"context"
	"strings"
	"testing"
)

func newCreateInput(id, name string) CreateSessionInput {
	return CreateSessionInput{ID: id, Name: name, CWD: "/work/" + id, Agent: "shell", CapturedPath: "/bin", StatusAt: 100, CreatedAt: 100}
}

func TestCreateSessionRefusesASlugHeldByAnArchivedSessionNamingIt(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	if _, err := st.CreateSession(ctx, newCreateInput("id-one", "Beta Two")); err != nil {
		t.Fatal(err)
	}
	if err := st.ArchiveSession(ctx, "id-one", 200); err != nil {
		t.Fatal(err)
	}
	// A different name that slugs identically (beta-two) is refused with the
	// archived holder named and both routes out given.
	_, err := st.CreateSession(ctx, newCreateInput("id-two", "beta-two"))
	if err == nil {
		t.Fatal("creating a session whose slug an archived session holds succeeded")
	}
	for _, want := range []string{`archived session "Beta Two"`, "slug", "unarchive", "dd"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %q, want it to mention %q", err, want)
		}
	}
	live, err := st.ListSessions(ctx)
	if err != nil || len(live) != 0 {
		t.Fatalf("refused create left visible sessions %+v (err %v)", live, err)
	}
}

func TestCreateSessionRefusesADuplicateIDAsAnInsertFailure(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	if _, err := st.CreateSession(ctx, newCreateInput("same-id", "first")); err != nil {
		t.Fatal(err)
	}
	_, err := st.CreateSession(ctx, newCreateInput("same-id", "second"))
	if err == nil || !strings.Contains(err.Error(), "insert session") {
		t.Fatalf("err = %v, want an insert-session failure", err)
	}
	got, err := st.ListSessions(ctx)
	if err != nil || len(got) != 1 || got[0].Name != "first" {
		t.Fatalf("sessions after the refused duplicate id = %+v (err %v), want only \"first\"", got, err)
	}
}

func TestCreateSessionResolvesTheGroupNameInTheSameCall(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	g, err := st.CreateGroup(ctx, "team-a")
	if err != nil {
		t.Fatal(err)
	}
	in := newCreateInput("grouped", "grouped")
	in.GroupID = &g.ID
	got, err := st.CreateSession(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if got.GroupName != "team-a" || got.GroupID == nil || *got.GroupID != g.ID {
		t.Fatalf("created session group = %v %q, want %d \"team-a\"", got.GroupID, got.GroupName, g.ID)
	}

	dangling := int64(987654)
	in = newCreateInput("dangling", "dangling")
	in.GroupID = &dangling
	got, err = st.CreateSession(ctx, in)
	if err != nil {
		t.Fatalf("a group id naming no row must not fail the create: %v", err)
	}
	if got.GroupName != "" {
		t.Fatalf("dangling group rendered as %q, want empty (default)", got.GroupName)
	}
}

func TestCreateSessionReportsAGroupLookupFailureAndRollsBack(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	if _, err := st.db.Exec(`ALTER TABLE groups RENAME TO groups_gone`); err != nil {
		t.Fatal(err)
	}
	gid := int64(1)
	in := newCreateInput("needs-group", "needs-group")
	in.GroupID = &gid
	_, err := st.CreateSession(ctx, in)
	if err == nil || !strings.Contains(err.Error(), "resolve created session's group name") {
		t.Fatalf("err = %v, want a group-name lookup failure", err)
	}
	var n int
	if err := st.db.QueryRow(`SELECT count(*) FROM sessions`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("a failed create left %d session rows (err %v), want none", n, err)
	}
}

// TestCreateSessionNameSourceIsStoredAndValidated (R248): the provenance given
// at create is what GetSession reads back, empty means user, and a value the
// column would reject is refused before any insert.
func TestCreateSessionNameSourceIsStoredAndValidated(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	for _, want := range []struct{ id, given, stored string }{
		{"ns-empty", "", "user"}, {"ns-user", "user", "user"}, {"ns-derived", "derived", "derived"}, {"ns-harness", "harness", "harness"},
	} {
		in := newCreateInput(want.id, want.id)
		in.NameSource = want.given
		created, err := st.CreateSession(ctx, in)
		if err != nil || created.NameSource != want.stored {
			t.Fatalf("create with name source %q = %q, %v, want %q", want.given, created.NameSource, err, want.stored)
		}
		if got, err := st.GetSession(ctx, want.id); err != nil || got.NameSource != want.stored {
			t.Fatalf("GetSession name source = %q, %v, want %q", got.NameSource, err, want.stored)
		}
	}
	bad := newCreateInput("ns-bad", "ns-bad")
	bad.NameSource = "typed"
	if _, err := st.CreateSession(ctx, bad); err == nil || !strings.Contains(err.Error(), "name source") {
		t.Fatalf("create with an unknown name source: err = %v, want a name source refusal", err)
	}
}
