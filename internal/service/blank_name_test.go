package service

import (
	"testing"
	"time"
)

func TestDerivedSessionName(t *testing.T) {
	now := time.Date(2025, 8, 20, 14, 43, 0, 0, time.UTC)
	cases := []struct {
		name  string
		cwd   string
		taken map[string]bool
		want  string
	}{
		{"plain", "/work/api", nil, "api-0820-1443"},
		{"trailing slash", "/work/api/", nil, "api-0820-1443"},
		{"dots kept", "/work/a.b", nil, "a.b-0820-1443"},
		{"first suffix", "/work/api", map[string]bool{"api-0820-1443": true}, "api-0820-1443-2"},
		{"first FREE suffix", "/work/api", map[string]bool{"api-0820-1443": true, "api-0820-1443-2": true, "api-0820-1443-4": true}, "api-0820-1443-3"},
		{"other dir unaffected", "/work/web", map[string]bool{"api-0820-1443": true}, "web-0820-1443"},
	}
	for _, tc := range cases {
		if got := DerivedSessionName(tc.cwd, now, tc.taken); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
	early := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	if got := DerivedSessionName("/w/p", early, nil); got != "p-0102-0304" {
		t.Errorf("zero padding: %q", got)
	}
}
