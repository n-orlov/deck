package tui

import (
	"reflect"
	"strings"
	"testing"
)

// TestParseCreateEnv drives parseCreateEnv directly through every branch,
// so its coverage (and so its CRAP score) never depends on which create
// modal tests or features happen to reach it in a given run.
func TestParseCreateEnv(t *testing.T) {
	for _, tc := range []struct {
		name    string
		raw     string
		want    map[string]string
		wantErr string
	}{
		{name: "empty input", raw: "", want: nil},
		{name: "whitespace-only input", raw: "  \t ", want: nil},
		{name: "single pair", raw: "A=1", want: map[string]string{"A": "1"}},
		{name: "several pairs", raw: "A=1,B=two", want: map[string]string{"A": "1", "B": "two"}},
		{name: "blank entries skipped", raw: ",A=1, ,,B=2,", want: map[string]string{"A": "1", "B": "2"}},
		{name: "only blank entries", raw: " , ,", want: map[string]string{}},
		{name: "entry and key whitespace trimmed", raw: "  A =1 ,\tB=2\t", want: map[string]string{"A": "1", "B": "2"}},
		{name: "value keeps inner whitespace", raw: "A= x y", want: map[string]string{"A": " x y"}},
		{name: "value containing =", raw: "OPTS=a=b=c", want: map[string]string{"OPTS": "a=b=c"}},
		{name: "empty value", raw: "A=", want: map[string]string{"A": ""}},
		{name: "later key wins", raw: "A=1,A=2", want: map[string]string{"A": "2"}},
		{name: "missing =", raw: "A=1,NOVALUE", wantErr: `env entry "NOVALUE" must be key=value`},
		{name: "empty key", raw: "=1", wantErr: `env entry "=1" must be key=value`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseCreateEnv(tc.raw)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("parseCreateEnv(%q) err = %v, want one containing %q", tc.raw, err, tc.wantErr)
				}
				if got != nil {
					t.Fatalf("parseCreateEnv(%q) = %v alongside an error, want nil", tc.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCreateEnv(%q) unexpected err: %v", tc.raw, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseCreateEnv(%q) = %#v, want %#v", tc.raw, got, tc.want)
			}
		})
	}
}
