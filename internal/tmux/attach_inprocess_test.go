package tmux

import (
	"context"
	"strings"
	"testing"
)

// TestAttachThroughPTY drives Attach in a helper process (a real terminal), so
// its own coverage never reaches this process; these cases run it in-process
// against a fake tmux binary.
func TestAttachReturnsNilWhenTmuxDetaches(t *testing.T) {
	client := fakeTmuxClient(t, `test "$1 $2 $3 $4" = "-L fake-socket attach-session -t" && test "$5" = deck_a || exit 9`)
	if err := client.Attach(context.Background(), "a"); err != nil {
		t.Fatalf("Attach with a tmux that detaches cleanly = %v, want nil", err)
	}
}

func TestAttachWrapsTmuxFailureWithTheSlug(t *testing.T) {
	client := fakeTmuxClient(t, `exit 1`)
	err := client.Attach(context.Background(), "a")
	if err == nil || !strings.Contains(err.Error(), `attach session "a"`) {
		t.Fatalf("Attach error = %v, want it to name the session", err)
	}
}

func TestAttachRejectsWhatAttachCommandRejects(t *testing.T) {
	client := fakeTmuxClient(t, `exit 0`)
	if err := client.Attach(context.Background(), "Bad Slug"); err == nil || !strings.Contains(err.Error(), "invalid session slug") {
		t.Fatalf("Attach with an invalid slug = %v, want the slug error", err)
	}
	if err := (Client{}).Attach(context.Background(), "a"); err == nil || !strings.Contains(err.Error(), "socket name is required") {
		t.Fatalf("Attach without a socket = %v, want the socket error", err)
	}
}
