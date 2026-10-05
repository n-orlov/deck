package tmux

import (
	"context"
	"strings"
	"testing"
)

func TestIsEmptyServerViewRecognisesOnlyTheNoSessionsFailures(t *testing.T) {
	for _, message := range []string{
		"no server running on /tmp/tmux-1000/deck",
		"no sessions",
		"no current target",
		"error connecting to /tmp/tmux-1000/deck (No such file or directory)",
	} {
		if !isEmptyServerView(errString(message)) {
			t.Errorf("isEmptyServerView(%q) = false, want true", message)
		}
	}
	for _, message := range []string{
		"error connecting to /tmp/tmux-1000/deck (Permission denied)",
		"No such file or directory",
		"protocol version mismatch",
	} {
		if isEmptyServerView(errString(message)) {
			t.Errorf("isEmptyServerView(%q) = true, want false", message)
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestGroupPanesBySessionKeepsFirstSeenOrderAndSkipsForeignNames(t *testing.T) {
	sessions, err := groupPanesBySession("deck_b|%1|10|0|||80|24\n\nuser_session|%9|99|0|||80|24\ndeck_a|%2|11|0|||80|24\ndeck_b|%3|12|1|2||80|24\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 || sessions[0].Name != "deck_b" || sessions[1].Name != "deck_a" {
		t.Fatalf("sessions = %+v, want deck_b then deck_a (user_session is not deck's)", sessions)
	}
	if len(sessions[0].Panes) != 2 || sessions[0].Panes[0].ID != "%1" || sessions[0].Panes[1].ID != "%3" || !sessions[0].Panes[1].Dead {
		t.Fatalf("deck_b panes = %+v, want %%1 and the dead %%3 in report order", sessions[0].Panes)
	}
	if len(sessions[1].Panes) != 1 || sessions[1].Panes[0].ID != "%2" {
		t.Fatalf("deck_a panes = %+v", sessions[1].Panes)
	}
}

func TestGroupPanesBySessionRejectsMalformedLines(t *testing.T) {
	if _, err := groupPanesBySession("deck_a no separator here\n"); err == nil || !strings.Contains(err.Error(), "parse pane facts") {
		t.Fatalf("line without a separator error = %v", err)
	}
	if _, err := groupPanesBySession("deck_a|%1|notapid|0|||80|24\n"); err == nil || !strings.Contains(err.Error(), "parse pane PID") {
		t.Fatalf("line with bad facts error = %v", err)
	}
	if sessions, err := groupPanesBySession(""); err != nil || len(sessions) != 0 {
		t.Fatalf("empty output = %+v, %v; want no sessions and no error", sessions, err)
	}
}

func TestListThroughAFakeTmux(t *testing.T) {
	ctx := context.Background()
	if _, err := (Client{}).List(ctx); err == nil || !strings.Contains(err.Error(), "socket name is required") {
		t.Fatalf("List without a socket = %v", err)
	}
	empty := fakeTmuxClient(t, `echo "no server running on /tmp/x" >&2; exit 1`)
	if sessions, err := empty.List(ctx); err != nil || sessions == nil || len(sessions) != 0 {
		t.Fatalf("List with no server = %#v, %v; want a non-nil empty slice", sessions, err)
	}
	broken := fakeTmuxClient(t, `echo "protocol error" >&2; exit 1`)
	if _, err := broken.List(ctx); err == nil || !strings.Contains(err.Error(), "protocol error") {
		t.Fatalf("List with a failing tmux = %v, want the failure", err)
	}
	live := fakeTmuxClient(t, `echo "deck_a|%1|10|0|||80|24"`)
	if sessions, err := live.List(ctx); err != nil || len(sessions) != 1 || sessions[0].Name != "deck_a" {
		t.Fatalf("List = %+v, %v", sessions, err)
	}
}
