package store

import (
	"context"
	"testing"
)

// ClaimSessionEvent records a marker once per value: the same reason as the
// latest event of that kind is refused, a different one is recorded, and
// another row or kind is independent.
func TestClaimSessionEventOncePerValue(t *testing.T) {
	db := openValidationStore(t)
	ctx := context.Background()
	for _, id := range []string{"a", "b"} {
		if _, err := db.CreateSession(ctx, CreateSessionInput{ID: id, Name: id, CWD: "/w", Agent: "shell", CapturedPath: "/bin", StatusAt: 1, CreatedAt: 1}); err != nil {
			t.Fatal(err)
		}
	}
	claim := func(id, kind, reason string) bool {
		t.Helper()
		ok, err := db.ClaimSessionEvent(ctx, id, kind, reason, 1)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	steps := []struct {
		id, kind, reason string
		want             bool
	}{
		{"a", "harness_name_sent", "one", true},
		{"a", "harness_name_sent", "one", false},
		{"a", "harness_name_sent", "two", true},
		{"a", "harness_name_sent", "one", true},
		{"b", "harness_name_sent", "one", true},
		{"a", "other", "one", true},
	}
	for _, s := range steps {
		if got := claim(s.id, s.kind, s.reason); got != s.want {
			t.Errorf("claim(%s,%s,%s) = %v, want %v", s.id, s.kind, s.reason, got, s.want)
		}
	}
	if _, err := db.ClaimSessionEvent(ctx, "", "k", "r", 1); err == nil {
		t.Error("a blank session id must be refused")
	}
}

func TestClaimSessionEventReportsAClosedStore(t *testing.T) {
	if _, err := closedStore(t).ClaimSessionEvent(context.Background(), "a", "k", "r", 1); err == nil {
		t.Error("a closed store must fail the claim")
	}
}
