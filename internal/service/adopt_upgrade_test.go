package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// openAsSchema12 adopts names through the store, then turns the database back
// into what schema 12 left on disk (no name_adopted column, version 12) and
// reopens it, so the schema 13 migration runs over real adoption history.
func openAsSchema12(t *testing.T, seed func(*store.Store)) *store.Store {
	t.Helper()
	home := t.TempDir()
	paths := config.Paths{Home: home, LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")}
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	seed(db)
	for _, statement := range []string{
		`ALTER TABLE sessions DROP COLUMN name_adopted`,
		`ALTER TABLE sessions DROP COLUMN last_output_at`,
		`ALTER TABLE sessions DROP COLUMN probe_miss_streak`,
		`ALTER TABLE sessions DROP COLUMN seen_status_at`,
		`ALTER TABLE sessions ADD COLUMN acknowledged INTEGER NOT NULL DEFAULT 1`,
		`DROP INDEX events_session_at`,
		`UPDATE meta SET version = 12 WHERE key = 'schema_version'`,
	} {
		if _, err := db.DB().Exec(statement); err != nil {
			t.Fatalf("rewind to schema 12: %v", err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(paths)
	if err != nil {
		t.Fatalf("upgrade schema 12: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	return reopened
}

// TestAdoptNameUpgradeKeepsAdoptedTitlesOutOfArgv: names a harness supplied
// before name_adopted existed (explicit titles adopted as 'user', generated
// titles adopted as 'harness') stay out of every adapter's launch and resume
// argv after the upgrade, while a name typed in deck still reaches it.
func TestAdoptNameUpgradeKeepsAdoptedTitlesOutOfArgv(t *testing.T) {
	ctx := context.Background()
	type row struct {
		id, kind, typed, adopted, nameSource string
	}
	rows := []row{
		{id: "explicit-claude", kind: "claude", adopted: "external-origin-claude $(printf EXTERNAL)", nameSource: "user"},
		{id: "explicit-pi", kind: "pi", adopted: "external-origin-pi $(printf EXTERNAL)", nameSource: "user"},
		{id: "generated-codex", kind: "codex", adopted: "generated-codex `id`", nameSource: "harness"},
		{id: "retyped-claude", kind: "claude", adopted: "harness-old $(id)", nameSource: "user", typed: "typed in deck"},
	}
	db := openAsSchema12(t, func(st *store.Store) {
		for _, r := range rows {
			if _, err := st.CreateSession(ctx, store.CreateSessionInput{
				ID: r.id, Name: "derived-" + r.id, CWD: t.TempDir(), Agent: r.kind, CapturedPath: "/bin",
				Status: "idle", StatusSource: "hook", StatusAt: 1, CreatedAt: 1, NameSource: "derived",
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := st.AdoptSessionName(ctx, r.id, r.adopted, "harness:"+r.kind, r.nameSource, 5); err != nil {
				t.Fatal(err)
			}
			if r.typed != "" {
				if err := st.RenameSession(ctx, r.id, r.typed, "user", 6); err != nil {
					t.Fatal(err)
				}
			}
		}
	})
	adapters := map[string]agent.Adapter{"claude": agent.NewClaude(), "pi": agent.NewPi(), "codex": agent.NewCodex()}
	for _, r := range rows {
		got, err := db.GetSession(ctx, r.id)
		if err != nil {
			t.Fatal(err)
		}
		want := r.typed
		if got.NameAdopted == (r.typed != "") {
			t.Errorf("%s: name_adopted=%v after upgrade", r.id, got.NameAdopted)
		}
		if name := launchName(got); name != want {
			t.Errorf("%s: launchName = %q, want %q", r.id, name, want)
		}
		in := agent.LaunchInput{CWD: got.CWD, ConversationID: "c-1", Profile: "safe", Name: launchName(got)}
		launch, launchErr := adapters[r.kind].Launch(in)
		resume, resumeErr := adapters[r.kind].Resume(agent.ResumeInput{CWD: got.CWD, ConversationID: "c-1", Profile: "safe", Name: launchName(got)})
		if launchErr != nil || resumeErr != nil {
			t.Fatalf("%s: launch %v resume %v", r.id, launchErr, resumeErr)
		}
		joined := strings.Join(append(launch, resume...), "\x00")
		if strings.Contains(joined, "EXTERNAL") || strings.Contains(joined, "$(") || strings.Contains(joined, "`id`") {
			t.Errorf("%s: argv carries the adopted title: %q / %q", r.id, launch, resume)
		}
		if want != "" && !strings.Contains(joined, want) {
			t.Errorf("%s: a name typed in deck is missing from argv: %q / %q", r.id, launch, resume)
		}
	}
}
