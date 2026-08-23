package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// openEventLogTestStore is this file's shared fixture: a real state.db
// (never a hand-built []store.Event slice) so ordering comes from
// Store.ListEvents' own ORDER BY, not from whatever order a test happened
// to build a slice in.
func openEventLogTestStore(t *testing.T) *store.Store {
	t.Helper()
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestEventLogViewOrdersEventsNewestFirst proves task 124's (I-9,
// requirement 32) `E` view renders three events whose insertion order
// differs from every other plausible sort (alphabetical by kind, by
// reason, or by payload) in newest-first order: the third recorded event's
// payload appears before the second's, which appears before the first's.
func TestEventLogViewOrdersEventsNewestFirstAndTruncatesALongPayload(t *testing.T) {
	db := openEventLogTestStore(t)
	ctx := context.Background()
	if err := db.RecordOrphanEvent(ctx, store.EventInput{At: 10, Kind: "archived", Reason: "user", Payload: "alpha-oldest"}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordOrphanEvent(ctx, store.EventInput{At: 20, Kind: "set_permission_profile", Reason: "user", Payload: "bravo-middle"}); err != nil {
		t.Fatal(err)
	}
	longPayload := strings.Repeat("z", maxEventLogPayloadRunes+40)
	if err := db.RecordOrphanEvent(ctx, store.EventInput{At: 30, Kind: "set_resume_pin", Reason: "user", Payload: longPayload}); err != nil {
		t.Fatal(err)
	}

	model := New(db, config.Settings{}, "")
	model.width, model.height = 100, 40
	model.eventLogOpen = true
	view := model.View()

	posOldest := strings.Index(view, "alpha-oldest")
	posMiddle := strings.Index(view, "bravo-middle")
	posNewest := strings.Index(view, strings.Repeat("z", 10))
	if posOldest < 0 || posMiddle < 0 || posNewest < 0 {
		t.Fatalf("event log view missing one or more payloads:\n%s", view)
	}
	// Newest first: the third-recorded (highest at) row renders above the
	// second, which renders above the first -- the reverse of insertion
	// order, and different from every alphabetical sort of kind/reason too
	// (archived < set_permission_profile < set_resume_pin alphabetically,
	// which is exactly insertion order and therefore NOT what this proves
	// on its own -- the row positions below are the actual proof).
	if !(posNewest < posMiddle && posMiddle < posOldest) {
		t.Fatalf("event log is not newest-first: positions oldest=%d middle=%d newest=%d in:\n%s", posOldest, posMiddle, posNewest, view)
	}

	// Boundedness: the long payload must be truncated, not shown in full,
	// and a short payload (bravo-middle, alpha-oldest) must NOT be
	// truncated merely because it happened to fit.
	if strings.Contains(view, longPayload) {
		t.Fatalf("event log rendered the full long payload instead of truncating it:\n%s", view)
	}
	if !strings.Contains(view, "bravo-middle") || !strings.Contains(view, "alpha-oldest") {
		t.Fatalf("event log truncated a short payload that already fit:\n%s", view)
	}
}

// TestEventLogViewMasksSecretShapedPayloadValueButNotOrdinaryOnes proves
// the view applies task 010's single predicate (isSecretShapedKey /
// maskedSecretPlaceholder) to a payload column value: a secret-shaped
// key's value never reaches the rendered string, while an ordinary key's
// value does, in the very same payload object.
func TestEventLogViewMasksSecretShapedPayloadValueButNotOrdinaryOnes(t *testing.T) {
	db := openEventLogTestStore(t)
	ctx := context.Background()
	payload := `{"AUDIT_ENV_TOKEN":"leak-eventlog-secret-8f21ac","note":"ordinary-value-9d3c"}`
	if err := db.RecordOrphanEvent(ctx, store.EventInput{At: 5, Kind: "note", Reason: "user", Payload: payload}); err != nil {
		t.Fatal(err)
	}

	model := New(db, config.Settings{}, "")
	model.width, model.height = 100, 40
	model.eventLogOpen = true
	view := model.View()

	if strings.Contains(view, "leak-eventlog-secret-8f21ac") {
		t.Fatalf("event log leaked a secret-shaped payload value:\n%s", view)
	}
	if !strings.Contains(view, "ordinary-value-9d3c") {
		t.Fatalf("event log masked an ordinary (non-secret-shaped) payload value:\n%s", view)
	}
}

// TestEventLogOpensOnECapitalAndClosesOnEsc proves the top-level `E`
// binding opens the dialog and that Esc -- the shared §11.4 contract key,
// via updateEventLog -- closes it again, changing nothing else.
func TestEventLogOpensOnECapitalAndClosesOnEsc(t *testing.T) {
	db := openEventLogTestStore(t)
	ctx := context.Background()
	if err := db.RecordOrphanEvent(ctx, store.EventInput{At: 1, Kind: "note", Reason: "user", Payload: "plain-value"}); err != nil {
		t.Fatal(err)
	}
	model := New(db, config.Settings{}, "")
	model.width, model.height = 100, 40

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	got := next.(Model)
	if !got.eventLogOpen {
		t.Fatalf("E did not open the event log")
	}
	view := got.View()
	if !strings.Contains(view, "Event log") || !strings.Contains(view, "plain-value") {
		t.Fatalf("event log view missing expected content:\n%s", view)
	}

	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got = next.(Model)
	if got.eventLogOpen {
		t.Fatalf("Esc did not close the event log")
	}
}

// TestEventLogViewWithNoStoreStatesUnavailableRatherThanPanicking proves
// the view degrades to an explicit message (matching every other
// store-optional feature's own convention in this package, e.g.
// submitEnvEdit's "editing the environment is unavailable") instead of
// dereferencing a nil store.
func TestEventLogViewWithNoStoreStatesUnavailableRatherThanPanicking(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.width, model.height = 100, 40
	model.eventLogOpen = true
	view := model.View()
	if !strings.Contains(view, "unavailable") {
		t.Fatalf("event log view with no store does not state unavailable:\n%s", view)
	}
}

// TestMaskEventPayloadLeavesNonJSONPayloadUnchanged proves the defensive
// masking pass is a no-op on the shape every real write path uses today
// (a bare key name, per SPEC's "env values never enter events" rule, or
// any other non-JSON-object string) rather than mangling it.
func TestMaskEventPayloadLeavesNonJSONPayloadUnchanged(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	for _, payload := range []string{"", "API_TOKEN", "plain text payload"} {
		if got := m.maskEventPayload(payload); got != payload {
			t.Errorf("maskEventPayload(%q) = %q, want unchanged", payload, got)
		}
	}
}
