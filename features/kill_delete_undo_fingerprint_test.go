package features

import (
	"context"
	"time"

	"github.com/cucumber/godog"
)

// registerKillDeleteUndoFingerprintSteps backs requirement 29 (task 108):
// for every destructive path this file already proves -- kill, kill-undo,
// dd, delete-undo and reap -- a scenario must additionally assert with
// requirement 3's fingerprint instrument (features/fingerprint_test.go)
// that the session's own cwd never moved, not even a mtime. The only new
// primitive needed is a way to point a real create at an *already seeded*
// scratch directory (registerFingerprintSteps' seedScratchDirectory,
// task 060) rather than the fresh, empty one
// createShellSessionInLabelledCWD (features/create_session_test.go)
// always makes for itself.
func registerKillDeleteUndoFingerprintSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" creates shell session "([^"]+)" with cwd the scratch directory labelled "([^"]+)"$`, clientCreatesShellSessionWithScratchCWDLabelled)
}

// clientCreatesShellSessionWithScratchCWDLabelled drives the real create
// modal to a successful shell-session creation, pointed at a directory a
// prior "a scratch directory ... is seeded with:" step already created and
// registered under label -- never a fresh directory of its own, since the
// whole point of requirement 29's adversarial seed (a deck-artifact-shaped
// file, a dotfile, a subdirectory, a read-only file) is that the real
// session cwd contains exactly those traps when the destructive action
// under test runs.
func clientCreatesShellSessionWithScratchCWDLabelled(ctx context.Context, clientName, sessionName, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	dir, err := namedDirectory(h, label)
	if err != nil {
		return err
	}
	if err := client.Send("n"); err != nil {
		return err
	}
	if err := client.WaitForFrame(ctx, false, "Create shell session"); err != nil {
		return err
	}
	if err := client.Send(sessionName); err != nil {
		return err
	}
	time.Sleep(75 * time.Millisecond)
	if err := client.Send("\t" + dir + "\r"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "starting")
}
