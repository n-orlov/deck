package store

import (
	"context"
	"path/filepath"
	"testing"
)

// TestHarnessNameAdoptOverrideRoundTrips: the per-session override is a
// tri-state in the v12 column: NULL inherits, 1 on, 0 off, and one event row
// records each change.
func TestHarnessNameAdoptOverrideRoundTrips(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	st, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	session, err := st.CreateSession(ctx, CreateSessionInput{ID: "s1", Name: "one", CWD: "/w", Agent: "shell", CapturedPath: "/bin", StatusAt: 1, CreatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	if session.HarnessNameAdopt != nil {
		t.Fatalf("a new row carries %v, want inherit (nil)", *session.HarnessNameAdopt)
	}
	on, off := true, false
	for _, want := range []*bool{&on, &off, nil} {
		if err := st.SetHarnessNameAdopt(ctx, "s1", want, "", 5); err != nil {
			t.Fatal(err)
		}
		got, err := st.GetSession(ctx, "s1")
		if err != nil {
			t.Fatal(err)
		}
		if (got.HarnessNameAdopt == nil) != (want == nil) || (want != nil && *got.HarnessNameAdopt != *want) {
			t.Fatalf("stored %v, read %v", want, got.HarnessNameAdopt)
		}
	}
	var n int
	if err := st.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = 's1' AND kind = 'set_harness_name_adopt'`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("set_harness_name_adopt events = %d, %v; want 3", n, err)
	}
	if err := st.SetHarnessNameAdopt(ctx, "missing", &on, "user", 6); err == nil {
		t.Fatal("an unknown session was accepted")
	}
}
