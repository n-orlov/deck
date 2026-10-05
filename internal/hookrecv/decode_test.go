package hookrecv

import (
	"context"
	"strings"
	"testing"
)

func TestReceiveRejectsUnusableInputBeforeTouchingTheStore(t *testing.T) {
	db := newHookStore(t)
	tests := []struct {
		name    string
		store   Store
		raw     string
		at      int64
		wantErr string
	}{
		{name: "nil store", store: nil, raw: `{"hook_event_name":"Stop"}`, at: 5, wantErr: "hook store is required"},
		{name: "zero timestamp", store: db, raw: `{"hook_event_name":"Stop"}`, at: 0, wantErr: "hook timestamp is required"},
		{name: "malformed JSON", store: db, raw: `{"hook_event_name":`, at: 5, wantErr: "decode hook payload:"},
		{name: "unsupported event", store: db, raw: `{"hook_event_name":"NoSuchEvent"}`, at: 5, wantErr: `unsupported hook event "NoSuchEvent"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Receive(context.Background(), tc.store, []byte(tc.raw), "row", "", tc.at)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Receive error = %v, want it to contain %q", err, tc.wantErr)
			}
			if result != (Result{}) {
				t.Fatalf("Receive result = %#v, want zero", result)
			}
		})
	}
	var events int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events`).Scan(&events); err != nil || events != 0 {
		t.Fatalf("events after rejected inputs = %d, %v; want none", events, err)
	}
}
