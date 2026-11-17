package features

import (
	"strings"
	"testing"
	"time"
)

func TestWaitForGeometryWaitsForTmuxToReportTheRequestedSize(t *testing.T) {
	reads := []string{"61x27", "61x27", "100x27"}
	n := 0
	err := waitForGeometry(5*time.Second, "100x27", func() (string, error) {
		g := reads[min(n, len(reads)-1)]
		n++
		return g, nil
	})
	if err != nil {
		t.Fatalf("want success once tmux reports 100x27, got %v", err)
	}
	if n != len(reads) {
		t.Fatalf("returned after %d reads, want it to wait for the third (the one reporting 100x27)", n)
	}
}

func TestWaitForGeometryFailsWhenTheWindowNeverReachesTheRequestedSize(t *testing.T) {
	err := waitForGeometry(300*time.Millisecond, "100x27", func() (string, error) { return "61x27", nil })
	if err == nil || !strings.Contains(err.Error(), "is 61x27, want 100x27") {
		t.Fatalf("want a failure naming 61x27 and 100x27, got %v", err)
	}
}

func TestNewestSessionFittedIsSetByTheHighestSessionID(t *testing.T) {
	cases := []struct {
		name, listing string
		wantErr       bool
	}{
		{"newest fitted, older not", "$0 80x24\n$1 61x27\n", false},
		{"newest still at creation size", "$0 61x27\n$1 80x24\n", true},
		{"ids compare numerically", "$9 61x27\n$10 80x24\n", true},
		{"no session", "\n", true},
	}
	for _, tc := range cases {
		if err := newestSessionFitted(tc.listing); (err != nil) != tc.wantErr {
			t.Errorf("%s: got %v, want error=%v", tc.name, err, tc.wantErr)
		}
	}
}
