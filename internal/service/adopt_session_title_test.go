package service

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/hookrecv"
)

// TestHookSessionTitleIsAdoptedAsUser: a Stop hook whose payload carries a
// non-empty session_title that differs from deck's name renames the row with
// name_source=user, driven through the real receiver; a payload without it, or
// with the row's own name, leaves the row alone.
func TestHookSessionTitleIsAdoptedAsUser(t *testing.T) {
	for _, tc := range []struct {
		name, extra, rowSource, wantName, wantSource string
	}{
		{"adopted as user", `,"session_title":"Chosen in Claude"`, "derived", "Chosen in Claude", "user"},
		{"harness row adopted as user", `,"session_title":"Chosen in Claude"`, "harness", "Chosen in Claude", "user"},
		{"absent leaves the row", ``, "derived", "swift-otter", "derived"},
		{"blank leaves the row", `,"session_title":"  "`, "derived", "swift-otter", "derived"},
		{"same as deck's name leaves the row", `,"session_title":"swift-otter"`, "derived", "swift-otter", "derived"},
		{"placeholder leaves the row", `,"session_title":"Claude Code"`, "derived", "swift-otter", "derived"},
		{"a user row is frozen", `,"session_title":"Chosen in Claude"`, "user", "swift-otter", "user"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, db, _, _ := newAgentTestService(t, nil, "session-title")
			row := adoptRow(t, db, "row-a", "swift-otter", tc.rowSource)
			origin := hookrecv.Origin{SessionID: row.ID, Adopt: func(ctx context.Context, id string, h hookrecv.AdoptHint) {
				_, _ = svc.AdoptNameWith(ctx, id, NameHint{TranscriptPath: h.TranscriptPath, SessionTitle: h.SessionTitle})
			}}
			payload := `{"hook_event_name":"Stop","session_id":"` + row.ConversationID + `"` + tc.extra + `}`
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
