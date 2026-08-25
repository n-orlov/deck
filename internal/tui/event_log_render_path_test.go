package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestEventLogViewNeverReReadsTheStoreOnceOpen proves R61 (steer 3e-001
// §6.3, SPEC §11.4's "a dialog never reads the store from its render
// path"): opening the `E` event log issues exactly one real ListEvents
// call against the store (Store.ListEventsCallCount, incremented inside
// the store's own method -- not a mock/interface substitute), and that
// count never grows again no matter how many reconcileTick/previewTick
// messages land, or how many times View() itself runs, while the dialog
// stays open. A single-frame test could not tell "load once" apart from
// "load every frame"; this one lets several ticks pass and several View()
// calls happen before asserting the count is still exactly 1.
func TestEventLogViewNeverReReadsTheStoreOnceOpen(t *testing.T) {
	db := openEventLogTestStore(t)
	ctx := context.Background()
	if err := db.RecordOrphanEvent(ctx, store.EventInput{At: 1, Kind: "note", Reason: "user", Payload: "plain-value"}); err != nil {
		t.Fatal(err)
	}

	model := New(db, config.Settings{}, "")
	model.width, model.height = 100, 40

	// Opening the dialog dispatches loadEventLog as a tea.Cmd; execute it
	// and feed the reply back, exactly like the real bubbletea runtime.
	next, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	got := next.(Model)
	if !got.eventLogOpen {
		t.Fatalf("E did not open the event log")
	}
	if cmd == nil {
		t.Fatalf("E did not dispatch a load command")
	}
	next, _ = got.Update(cmd())
	got = next.(Model)

	if n := db.ListEventsCallCount(); n != 1 {
		t.Fatalf("opening the event log made %d ListEvents calls, want exactly 1", n)
	}

	// Render several times and let several reconcileTick/previewTick
	// messages pass while the dialog stays open -- none of these may
	// touch the store again.
	for i := 0; i < 5; i++ {
		_ = got.View()
		next, _ = got.Update(reconcileTick(time.Now()))
		got = next.(Model)
		next, _ = got.Update(previewTick(time.Now()))
		got = next.(Model)
		_ = got.View()
	}
	if !got.eventLogOpen {
		t.Fatalf("event log unexpectedly closed during ticking")
	}

	if n := db.ListEventsCallCount(); n != 1 {
		t.Fatalf("after several ticks and renders, ListEvents was called %d times, want exactly 1 (the render path must never re-read the store)", n)
	}
}
