package config

import (
	"strings"
	"testing"
)

// TestParseIntegerValueRejectsMalformedLiterals pins the stated error for
// every way an integer-kind value can fail to parse: a seconds field takes
// a bare integer or a quoted Go duration and names both forms; any other
// integer field names only "an integer".
func TestParseIntegerValueRejectsMalformedLiterals(t *testing.T) {
	seconds, ok := FieldByFullKey("stale_after")
	if !ok || seconds.Unit != "seconds" {
		t.Fatalf("stale_after must be a seconds field, got %+v (found %v)", seconds, ok)
	}
	plain, ok := FieldByFullKey("ui.recent_cwd_limit")
	if !ok || plain.Unit == "seconds" {
		t.Fatalf("ui.recent_cwd_limit must be a non-seconds integer field, got %+v (found %v)", plain, ok)
	}

	for _, test := range []struct {
		name  string
		field Field
		raw   string
		want  string
	}{
		{"seconds: duration without closing quote", seconds, `"90s`, "must be seconds or a duration"},
		{"seconds: quoted text that is no duration", seconds, `"soon"`, "must be seconds or a duration"},
		{"seconds: bare word", seconds, "soon", "must be seconds or a duration"},
		{"plain: bare word", plain, "many", "must be an integer"},
		{"plain: quoted number", plain, `"5"`, "must be an integer"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseIntegerValue(test.field, test.raw)
			if err == nil {
				t.Fatalf("parseIntegerValue(%q) accepted %q", test.field.FullKey(), test.raw)
			}
			if !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), test.field.FullKey()) {
				t.Fatalf("error = %q, want it to name %q and %q", err, test.field.FullKey(), test.want)
			}
		})
	}
}
