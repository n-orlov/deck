package tmux

import (
	"context"
	"strings"
	"testing"
)

// TestPaneTitleReadsOnlyTheTitleFieldOfTheNamedSession: the title is read by
// its own display-message call against the exact session, and paneFactsFormat
// stays free of the field.
func TestPaneTitleReadsOnlyTheTitleFieldOfTheNamedSession(t *testing.T) {
	client := fakeTmuxClient(t, `case "$3" in
display-message) echo "$*" >> "$0.sent"; echo "✳ Fix the parser";;
esac`)
	title, err := client.PaneTitle(context.Background(), "a")
	if err != nil || title != "✳ Fix the parser" {
		t.Fatalf("PaneTitle = (%q, %v)", title, err)
	}
	if sent, want := readFakeSent(t, client), "-L fake-socket display-message -p -t =deck_a: #{pane_title}\n"; sent != want {
		t.Fatalf("tmux calls = %q, want %q", sent, want)
	}
	if strings.Contains(paneFactsFormat, "pane_title") {
		t.Fatalf("paneFactsFormat reads free text: %q", paneFactsFormat)
	}
}

func TestPaneTitleErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := (Client{}).PaneTitle(ctx, "a"); err == nil {
		t.Error("no socket: want an error")
	}
	if _, err := fakeTmuxClient(t, `exit 0`).PaneTitle(ctx, "A B"); err == nil {
		t.Error("invalid slug: want an error")
	}
	client := fakeTmuxClient(t, `echo "protocol error" >&2; exit 1`)
	if _, err := client.PaneTitle(ctx, "a"); err == nil || !strings.Contains(err.Error(), "read pane title") {
		t.Errorf("error = %v", err)
	}
	gone := fakeTmuxClient(t, `echo "can't find session: deck_a" >&2; exit 1`)
	if title, err := gone.PaneTitle(ctx, "a"); err != nil || title != "" {
		t.Errorf("vanished session = (%q, %v), want empty, nil", title, err)
	}
}
