package hookrecv

import (
	"context"
	"testing"
)

// TestPiSessionNamePayloadHintsTheAdoption: Pi's optional name fields (SPEC
// §8.3) become the adoption hint -- session_name on SessionStart, name on
// session_info_changed -- a cleared name hints nothing but still reaches
// Adopt, and a payload without the fields behaves exactly as before.
func TestPiSessionNamePayloadHintsTheAdoption(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		wantCalls     int
		wantTitle     string
	}{
		{"session_info_changed name", `{"hook_event_name":"session_info_changed","session_id":"c-1","name":"Chosen","cleared":false}`, 1, "Chosen"},
		{"SessionStart session_name", `{"hook_event_name":"SessionStart","session_id":"c-1","source":"startup","session_name":"At start"}`, 1, "At start"},
		{"cleared", `{"hook_event_name":"session_info_changed","session_id":"c-1","name":"","cleared":true}`, 1, ""},
		{"cleared beats a stale name", `{"hook_event_name":"session_info_changed","session_id":"c-1","name":"old","cleared":true}`, 1, ""},
		{"session_info_changed without fields", `{"hook_event_name":"session_info_changed","session_id":"c-1"}`, 1, ""},
		{"SessionStart without fields", `{"hook_event_name":"SessionStart","session_id":"c-1","source":"startup"}`, 0, ""},
		{"name on another event is ignored", `{"hook_event_name":"Stop","session_id":"c-1","name":"x","session_name":"y"}`, 1, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newHookStore(t)
			createHookSession(t, db, "row-1", "pi", "c-1")
			var hints []AdoptHint
			origin := Origin{SessionID: "row-1", Adopt: func(_ context.Context, _ string, h AdoptHint) { hints = append(hints, h) }}
			if _, err := ReceiveFrom(context.Background(), db, []byte(tc.payload), origin, 100); err != nil {
				t.Fatal(err)
			}
			if len(hints) != tc.wantCalls {
				t.Fatalf("Adopt calls = %d, want %d", len(hints), tc.wantCalls)
			}
			if tc.wantCalls == 1 && hints[0] != (AdoptHint{SessionTitle: tc.wantTitle}) {
				t.Fatalf("hint = %#v, want title %q", hints[0], tc.wantTitle)
			}
		})
	}
}

// TestPiSessionNameLeavesTheStatusWriteAlone: the name fields change nothing
// the hook stores besides the hint.
func TestPiSessionNameLeavesTheStatusWriteAlone(t *testing.T) {
	run := func(payload string) (Result, string) {
		db := newHookStore(t)
		createHookSession(t, db, "row-1", "pi", "c-1")
		result, err := ReceiveFrom(context.Background(), db, []byte(payload), Origin{SessionID: "row-1"}, 100)
		if err != nil {
			t.Fatal(err)
		}
		var status string
		if err := db.DB().QueryRow(`SELECT status FROM sessions WHERE id = 'row-1'`).Scan(&status); err != nil {
			t.Fatal(err)
		}
		return result, status
	}
	without, statusWithout := run(`{"hook_event_name":"SessionStart","session_id":"c-1","source":"startup"}`)
	with, statusWith := run(`{"hook_event_name":"SessionStart","session_id":"c-1","source":"startup","session_name":"At start"}`)
	if without != with || statusWithout != statusWith {
		t.Fatalf("name field changed the write: %+v/%s vs %+v/%s", without, statusWithout, with, statusWith)
	}
}
