package service

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/hookrecv"
)

// TestPiSessionNameIsAdoptedAsUser drives Pi's two name payloads (SPEC §8.3)
// through the real receiver and adoption: a name is adopted with
// name_source=user, a cleared name and a payload without the fields leave
// deck's name alone.
func TestPiSessionNameIsAdoptedAsUser(t *testing.T) {
	for _, tc := range []struct {
		name, payload, rowSource, wantName, wantSource string
	}{
		{"session_info_changed name", `"hook_event_name":"session_info_changed","name":"Chosen in Pi","cleared":false`, "derived", "Chosen in Pi", "user"},
		{"SessionStart session_name", `"hook_event_name":"SessionStart","source":"startup","session_name":"Named at start"`, "derived", "Named at start", "user"},
		{"harness row", `"hook_event_name":"session_info_changed","name":"Chosen in Pi"`, "harness", "Chosen in Pi", "user"},
		{"cleared keeps deck's name", `"hook_event_name":"session_info_changed","name":"","cleared":true`, "derived", "swift-otter", "derived"},
		{"cleared with a stale name keeps deck's name", `"hook_event_name":"session_info_changed","name":"old","cleared":true`, "derived", "swift-otter", "derived"},
		{"no fields: session_info_changed", `"hook_event_name":"session_info_changed"`, "derived", "swift-otter", "derived"},
		{"no fields: SessionStart", `"hook_event_name":"SessionStart","source":"startup"`, "derived", "swift-otter", "derived"},
		{"a user row is frozen", `"hook_event_name":"session_info_changed","name":"Chosen in Pi"`, "user", "swift-otter", "user"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, db, _, _ := newAgentTestService(t, nil, "pi-session-name")
			row := adoptRow(t, db, "row-a", "swift-otter", tc.rowSource)
			if _, err := db.DB().Exec(`UPDATE sessions SET agent = 'pi' WHERE id = ?`, row.ID); err != nil {
				t.Fatal(err)
			}
			origin := hookrecv.Origin{SessionID: row.ID, Adopt: func(ctx context.Context, id string, h hookrecv.AdoptHint) {
				_, _ = svc.AdoptNameWith(ctx, id, NameHint{TranscriptPath: h.TranscriptPath, SessionTitle: h.SessionTitle})
			}}
			payload := `{"session_id":"` + row.ConversationID + `",` + tc.payload + `}`
			if _, err := hookrecv.ReceiveFrom(context.Background(), db, []byte(payload), origin, 100); err != nil {
				t.Fatal(err)
			}
			got, err := db.GetSession(context.Background(), row.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Name != tc.wantName || got.NameSource != tc.wantSource || got.Slug != row.Slug {
				t.Fatalf("name=%q source=%q slug=%q; want %q %q %q", got.Name, got.NameSource, got.Slug, tc.wantName, tc.wantSource, row.Slug)
			}
		})
	}
}
