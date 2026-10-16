package tui

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// R303: a row cleared with Y and then re-probed waiting keeps its position in
// the attention order, and in the activity order (which reads last_output_at,
// R306, and so cannot be moved by a status write at all). The re-probe repeats
// "waiting", so it neither re-dates the row (status_at drives attention) nor
// marks it unseen again.
func TestClearedWaitingRowReprobedWaitingKeepsItsSortPosition(t *testing.T) {
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	for _, id := range []string{"older", "newer"} {
		if _, err := db.CreateSession(ctx, store.CreateSessionInput{
			ID: id, Name: id, CWD: "/work", Agent: "claude", CapturedPath: "/bin",
			Status: "running", StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	for id, at := range map[string]int64{"older": 100, "newer": 200} {
		if err := db.UpdateSessionStatus(ctx, store.StatusUpdateInput{
			SessionID: id, Status: "waiting", Reason: "idle_prompt", Source: "probe", At: at, StaleAfter: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	// The older row printed more recently, so the activity order puts it first
	// -- the opposite of the id order, so the check cannot pass by accident.
	for id, at := range map[string]int64{"older": 500_000, "newer": 100_000} {
		if err := db.RecordOutput(ctx, id, at); err != nil {
			t.Fatal(err)
		}
	}
	order := func(sorter func([]store.Session) []store.Session) []string {
		t.Helper()
		rows, err := db.ListSessions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, row := range sorter(rows) {
			ids = append(ids, row.ID)
		}
		return ids
	}
	attention := func(rows []store.Session) []store.Session { return sortSessionsByAttentionStable(nil, rows) }
	activity := func(rows []store.Session) []store.Session {
		return sortSessionsByOrder(nil, rows, SortOrderActivity, 0)
	}
	wantAttention, wantActivity := []string{"older", "newer"}, []string{"older", "newer"}
	if got := order(attention); !equalIDs(got, wantAttention) {
		t.Fatalf("attention order before clearing = %v, want %v", got, wantAttention)
	}
	if got := order(activity); !equalIDs(got, wantActivity) {
		t.Fatalf("activity order before clearing = %v, want %v", got, wantActivity)
	}

	// Y on the older row, then the probe sees the same waiting prompt again.
	if err := db.AcknowledgeSession(ctx, "older"); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID: "older", Status: "waiting", Reason: "idle_prompt", Source: "probe", At: 300, StaleAfter: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if got := order(attention); !equalIDs(got, wantAttention) {
		t.Fatalf("attention order after Y then re-probe waiting = %v, want %v", got, wantAttention)
	}
	if got := order(activity); !equalIDs(got, wantActivity) {
		t.Fatalf("activity order after Y then re-probe waiting = %v, want %v", got, wantActivity)
	}
	row, err := db.GetSession(ctx, "older")
	if err != nil {
		t.Fatal(err)
	}
	if row.StatusAt != 100 || !row.StatusSeen() {
		t.Fatalf("re-probed row status_at=%d acknowledged=%v, want 100/true (cleared stays cleared)", row.StatusAt, row.StatusSeen())
	}
}

func equalIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
