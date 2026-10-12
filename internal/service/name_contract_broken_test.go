package service

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/hookrecv"
	"github.com/n-orlov/deck/internal/store"
)

// R265: the default-tier proof for the upstream name contracts. The @real-agents
// scenarios (features/real_agent_names.feature) pin the contracts against the
// installed CLIs; CI never runs them, so these tests carry the degrade rule:
// when a harness release breaks its contract, the row keeps its dummy name,
// its status is untouched and nothing errors.

const brokenContractConversation = "0b6f1c1e-5d1a-4a53-9a43-3f1d2b0c7a11"

// brokenContractRow creates a derived-name row of the given agent bound to a
// UUID conversation and returns it with the status it must still have later.
func brokenContractRow(t *testing.T, db *store.Store, kind string) store.Session {
	t.Helper()
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "row-" + kind, Name: "swift-otter", CWD: t.TempDir(), Agent: kind, CapturedPath: "/bin",
		Status: "idle", StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
		ConversationID: brokenContractConversation, NameSource: "derived",
	})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

// requireDummyNameKept fails unless the row still carries its dummy name, its
// derived provenance, its slug and its status, and no rename was recorded.
func requireDummyNameKept(t *testing.T, db *store.Store, before store.Session, renamed bool, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("a broken name contract surfaced an error: %v", err)
	}
	if renamed {
		t.Fatal("a broken name contract renamed the row")
	}
	got, getErr := db.GetSession(context.Background(), before.ID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if got.Name != "swift-otter" || got.NameSource != "derived" || got.Slug != before.Slug {
		t.Fatalf("name=%q source=%q slug=%q; want the dummy swift-otter, derived, %q", got.Name, got.NameSource, got.Slug, before.Slug)
	}
	if got.Status != before.Status || got.StatusSource != before.StatusSource {
		t.Fatalf("status %q/%q changed from %q/%q: a broken name contract must not touch status", got.Status, got.StatusSource, before.Status, before.StatusSource)
	}
	events, listErr := db.ListEvents(context.Background(), 100)
	if listErr != nil {
		t.Fatal(listErr)
	}
	for _, e := range events {
		if e.Kind == "renamed" || e.Kind == "error" {
			t.Fatalf("event %q recorded for a broken name contract", e.Kind)
		}
	}
}

// Claude: the transcript's title record type is renamed upstream (custom-title
// -> custom-name, ai-title -> auto-title), so none of the three records the
// adapter reads is present.
func TestBrokenContractClaudeTranscriptRecordKeepsDummyName(t *testing.T) {
	svc, db, _, _ := newAgentTestService(t, nil, "broken-claude")
	row := brokenContractRow(t, db, "claude")
	transcript := filepath.Join(t.TempDir(), "t.jsonl")
	body := `{"type":"user","message":{"role":"user","content":"hi"}}` + "\n" +
		`{"type":"custom-name","customTitle":"my review"}` + "\n" +
		`{"type":"agent-label","agentName":"planner"}` + "\n" +
		`{"type":"auto-title","aiTitle":"Fix the parser"}` + "\n"
	if err := os.WriteFile(transcript, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	renamed, err := svc.AdoptNameWith(context.Background(), row.ID, NameHint{TranscriptPath: transcript})
	requireDummyNameKept(t, db, row, renamed, err)
}

// Claude, second half of the contract: the title field inside a still-known
// record type is renamed (customTitle -> title).
func TestBrokenContractClaudeTitleFieldKeepsDummyName(t *testing.T) {
	svc, db, _, _ := newAgentTestService(t, nil, "broken-claude-field")
	row := brokenContractRow(t, db, "claude")
	transcript := filepath.Join(t.TempDir(), "t.jsonl")
	body := `{"type":"custom-title","title":"my review"}` + "\n" + `{"type":"ai-title","text":"Fix the parser"}` + "\n"
	if err := os.WriteFile(transcript, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	renamed, err := svc.AdoptNameWith(context.Background(), row.ID, NameHint{TranscriptPath: transcript})
	requireDummyNameKept(t, db, row, renamed, err)
}

// Codex: state_5.sqlite's threads table lost its name column (a renamed column
// is the same shape), and the pane carries no usable title.
func TestBrokenContractCodexThreadsColumnKeepsDummyName(t *testing.T) {
	codexHome := t.TempDir()
	svc, db, _, _ := newAgentTestService(t, map[string]string{"CODEX_HOME": codexHome}, "broken-codex")
	row := brokenContractRow(t, db, "codex")
	conn, err := sql.Open("sqlite", filepath.Join(codexHome, "state_5.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	for _, stmt := range []string{
		`CREATE TABLE threads (id TEXT PRIMARY KEY, title TEXT)`,
		`INSERT INTO threads (id, title) VALUES ('` + brokenContractConversation + `', 'Write flaky test haiku')`,
	} {
		if _, err := conn.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	renamed, adoptErr := svc.AdoptName(context.Background(), row.ID)
	requireDummyNameKept(t, db, row, renamed, adoptErr)
}

// Copilot: workspace.yaml's name key is renamed upstream (name -> title,
// user_named -> named_by_user).
func TestBrokenContractCopilotWorkspaceYAMLKeyKeepsDummyName(t *testing.T) {
	copilotHome := t.TempDir()
	svc, db, _, _ := newAgentTestService(t, map[string]string{"COPILOT_HOME": copilotHome}, "broken-copilot")
	svc.Agents.Register(agent.NewCopilot())
	row := brokenContractRow(t, db, "copilot")
	dir := filepath.Join(copilotHome, "session-state", brokenContractConversation)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	yaml := "id: " + brokenContractConversation + "\ncwd: /work/app\ntitle: my review\nnamed_by_user: true\n"
	if err := os.WriteFile(filepath.Join(dir, "workspace.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	renamed, err := svc.AdoptName(context.Background(), row.ID)
	requireDummyNameKept(t, db, row, renamed, err)
}

// Pi: the session_info_changed event arrives without its name field (renamed
// to label), and SessionStart without session_name (renamed to sessionName).
func TestBrokenContractPiSessionInfoEventFieldKeepsDummyName(t *testing.T) {
	for _, payload := range []string{
		`"hook_event_name":"session_info_changed","label":"Chosen in Pi","cleared":false`,
		`"hook_event_name":"SessionStart","source":"startup","sessionName":"Named at start"`,
	} {
		t.Run(payload, func(t *testing.T) {
			svc, db, _, _ := newAgentTestService(t, nil, "broken-pi")
			row := brokenContractRow(t, db, "pi")
			// SessionStart maps to running whatever its name fields say, so
			// start there: the check is that the name contract adds nothing.
			if _, err := db.DB().Exec(`UPDATE sessions SET status = 'running' WHERE id = ?`, row.ID); err != nil {
				t.Fatal(err)
			}
			row.Status = "running"
			origin := hookrecv.Origin{SessionID: row.ID, Adopt: func(ctx context.Context, id string, h hookrecv.AdoptHint) {
				_, _ = svc.AdoptNameWith(ctx, id, NameHint{TranscriptPath: h.TranscriptPath, SessionTitle: h.SessionTitle})
			}}
			body := `{"session_id":"` + brokenContractConversation + `",` + payload + `}`
			_, err := hookrecv.ReceiveFrom(context.Background(), db, []byte(body), origin, 100)
			requireDummyNameKept(t, db, row, false, err)
		})
	}
}
