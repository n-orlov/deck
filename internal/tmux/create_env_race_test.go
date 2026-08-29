package tmux

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestCreateSurvivesInstantExitEnvironmentMirroring pins task 501:
// Client.Create must not fail merely because its pane's own command exits
// before the environment-mirroring step runs. Before the fix, Create ran
// `new-session` and then a separate, later `set-environment` call per
// variable; when Command exits essentially instantly (crash.feature:46's
// failing pre_launch is the natural-route case: `sh -c 'exit 1'` here is
// the same shape), tmux can tear the session back down before that later
// call reaches it, which failed with "no such session" and aborted the
// whole create. The fix mirrors the environment via new-session's own -e,
// atomically with session creation, so there is no later call left for
// that race to have a window in.
func TestCreateSurvivesInstantExitEnvironmentMirroring(t *testing.T) {
	socket := fmt.Sprintf("deck-instant-exit-%d-%d", os.Getpid(), time.Now().UnixNano())
	client := Client{Socket: socket, Timeout: 3 * time.Second}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.command(ctx, "kill-server").Run()
	})

	want := map[string]string{"DECK_RACE_A": "one", "DECK_RACE_B": "two"}
	created, err := client.Create(context.Background(), Launch{
		Slug:    "instant_exit",
		CWD:     t.TempDir(),
		Command: []string{"sh", "-c", "exit 1"},
		Env:     want,
	})
	if err != nil {
		t.Fatalf("create session whose command exits instantly: %v", err)
	}
	if created.Name != "deck_instant_exit" {
		t.Fatalf("created session = %#v, want deck_instant_exit", created)
	}

	output, err := client.command(context.Background(), "show-environment", "-t", created.Name).CombinedOutput()
	if err != nil {
		t.Fatalf("show-environment for %q: %v: %s", created.Name, err, output)
	}
	got := string(output)
	for key, value := range want {
		want := key + "=" + value
		if !strings.Contains(got, want) {
			t.Errorf("session environment = %q, want it to contain %q", got, want)
		}
	}
}

// TestCreateStillFailsOnGenuineEnvironmentError proves task 501's fix kept
// no tolerance broad enough to swallow a real environment failure: an
// invalid variable name must still fail Create, exactly as it did before
// the -e change (environmentArgs rejects it long before any tmux call).
func TestCreateStillFailsOnGenuineEnvironmentError(t *testing.T) {
	socket := fmt.Sprintf("deck-bad-env-%d-%d", os.Getpid(), time.Now().UnixNano())
	client := Client{Socket: socket, Timeout: 3 * time.Second}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.command(ctx, "kill-server").Run()
	})

	_, err := client.Create(context.Background(), Launch{
		Slug:    "bad_env",
		CWD:     t.TempDir(),
		Command: []string{"sh", "-c", "exit 0"},
		Env:     map[string]string{"BAD=KEY": "value"},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid environment variable") {
		t.Fatalf("create with invalid environment key: err = %v, want an invalid-environment-variable error", err)
	}
}
