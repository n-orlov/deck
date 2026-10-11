package service

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// adoptFakeAdapter is Claude that claims the hook observation source and
// answers ReadName with a scripted result, counting each ask.
type adoptFakeAdapter struct {
	agent.Claude
	claims   bool
	name     string
	explicit bool
	ok       bool
	asked    *int
}

func (a adoptFakeAdapter) Capabilities() agent.Caps {
	caps := a.Claude.Capabilities()
	if a.claims {
		caps.Name = agent.NameCaps{ObserveName: []string{agent.ObserveHook}}
	}
	return caps
}

func (a adoptFakeAdapter) ReadName(agent.NameProbeInput) (string, bool, bool) {
	*a.asked++
	return a.name, a.explicit, a.ok
}

// adoptRow creates a claude row bound to a conversation, named name with provenance source.
func adoptRow(t *testing.T, db *store.Store, id, name, source string) store.Session {
	t.Helper()
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: id, Name: name, CWD: t.TempDir(), Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
		ConversationID: "conv-" + id, NameSource: source,
	})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

// TestAdoptNameOutcomes is R251's table: one case per finite outcome of
// reading a harness name for a row.
func TestAdoptNameOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		rowSource  string
		read       string
		explicit   bool
		other      string // another row already holding this name
		wantName   string
		wantSource string
		wantRename bool
	}{
		{name: "empty name is unchanged", rowSource: "derived", read: "", wantName: "swift-otter", wantSource: "derived"},
		{name: "equal to current is unchanged", rowSource: "harness", read: "swift-otter", wantName: "swift-otter", wantSource: "harness"},
		{name: "placeholder Claude Code is unchanged", rowSource: "derived", read: "Claude Code", wantName: "swift-otter", wantSource: "derived"},
		{name: "placeholder GitHub Copilot is unchanged", rowSource: "derived", read: "GitHub Copilot", wantName: "swift-otter", wantSource: "derived"},
		{name: "placeholder pi is unchanged", rowSource: "derived", read: "π", wantName: "swift-otter", wantSource: "derived"},
		{name: "new title is adopted as harness", rowSource: "derived", read: "Fix the parser", wantName: "Fix the parser", wantSource: "harness", wantRename: true},
		{name: "a harness row follows a new title", rowSource: "harness", read: "Fix the lexer", wantName: "Fix the lexer", wantSource: "harness", wantRename: true},
		{name: "collision gets a -2 suffix", rowSource: "derived", read: "Fix the parser", other: "Fix the parser", wantName: "Fix the parser-2", wantSource: "harness", wantRename: true},
		{name: "explicit name is adopted as user", rowSource: "derived", read: "my own title", explicit: true, wantName: "my own title", wantSource: "user", wantRename: true},
		{name: "a user row is never changed", rowSource: "user", read: "Fix the parser", wantName: "swift-otter", wantSource: "user"},
		{name: "a user row is never changed by an explicit name", rowSource: "user", read: "my own title", explicit: true, wantName: "swift-otter", wantSource: "user"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, db, _, _ := newAgentTestService(t, nil, "adopt-table")
			asked := 0
			svc.Agents.Register(adoptFakeAdapter{claims: true, name: tc.read, explicit: tc.explicit, ok: true, asked: &asked})
			if tc.other != "" {
				adoptRow(t, db, "other", tc.other, "user")
			}
			row := adoptRow(t, db, "row-a", "swift-otter", tc.rowSource)

			renamed, err := svc.AdoptName(context.Background(), row.ID)
			if err != nil {
				t.Fatal(err)
			}
			got, err := db.GetSession(context.Background(), row.ID)
			if err != nil {
				t.Fatal(err)
			}
			if renamed != tc.wantRename || got.Name != tc.wantName || got.NameSource != tc.wantSource {
				t.Fatalf("renamed=%v name=%q source=%q; want %v %q %q", renamed, got.Name, got.NameSource, tc.wantRename, tc.wantName, tc.wantSource)
			}
			if got.Slug != row.Slug || "deck_"+got.Slug != "deck_"+row.Slug {
				t.Fatalf("adoption moved the tmux slug %q -> %q", row.Slug, got.Slug)
			}
			assertAdoptionEvent(t, db, row.ID, tc.wantRename)
		})
	}
}

func assertAdoptionEvent(t *testing.T, db *store.Store, id string, want bool) {
	t.Helper()
	var n int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'renamed' AND reason = 'harness:claude'`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if (n == 1) != want || n > 1 {
		t.Fatalf("harness:claude renamed events = %d, want renamed=%v", n, want)
	}
}

// TestAdoptNameAsksOnlyAdaptersThatClaimReadName: an adapter whose caps claim
// no observation source is never asked, and a shell row is never eligible.
func TestAdoptNameAsksOnlyAdaptersThatClaimReadName(t *testing.T) {
	svc, db, _, _ := newAgentTestService(t, nil, "adopt-claims")
	asked := 0
	svc.Agents.Register(adoptFakeAdapter{claims: false, name: "Some title", ok: true, asked: &asked})
	row := adoptRow(t, db, "row-a", "swift-otter", "derived")
	if renamed, err := svc.AdoptName(context.Background(), row.ID); err != nil || renamed || asked != 0 {
		t.Fatalf("renamed=%v err=%v asked=%d; want an adapter that claims nothing never asked", renamed, err, asked)
	}
}

// TestAdoptNameOnTickRenamesLiveRowsOnly: the existing reconcile pass is an
// adoption trigger; it adopts for a live eligible row, leaves a user row and
// adds no poll of its own (one ask per eligible row per pass).
func TestAdoptNameOnTickAdoptsLiveEligibleRows(t *testing.T) {
	svc, db, _, _ := newAgentTestService(t, nil, "adopt-tick")
	asked := 0
	svc.Agents.Register(adoptFakeAdapter{claims: true, name: "Tick title", ok: true, asked: &asked})
	derived := adoptRow(t, db, "row-d", "swift-otter", "derived")
	user := adoptRow(t, db, "row-u", "my-name", "user")
	for _, row := range []store.Session{derived, user} {
		if _, err := svc.TMux.Create(context.Background(), tmux.Launch{Slug: row.Slug, CWD: row.CWD, Command: []string{"/bin/sh", "-c", "sleep 30"}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	gotDerived, _ := db.GetSession(context.Background(), derived.ID)
	gotUser, _ := db.GetSession(context.Background(), user.ID)
	if gotDerived.Name != "Tick title" || gotDerived.NameSource != "harness" || gotDerived.Slug != derived.Slug {
		t.Fatalf("derived row after tick = %q/%q slug %q", gotDerived.Name, gotDerived.NameSource, gotDerived.Slug)
	}
	if gotUser.Name != "my-name" || gotUser.NameSource != "user" {
		t.Fatalf("user row after tick = %q/%q", gotUser.Name, gotUser.NameSource)
	}
	if asked != 1 {
		t.Fatalf("ReadName asked %d times in one pass, want once (the derived row only)", asked)
	}
	live, err := svc.TMux.Exists(context.Background(), derived.Slug)
	if err != nil || !live {
		t.Fatalf("tmux session deck_%s live=%v err=%v after adoption", derived.Slug, live, err)
	}
}
