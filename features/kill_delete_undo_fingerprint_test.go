package features

import (
	"context"
	"time"

	"github.com/cucumber/godog"
)

// registerKillDeleteUndoFingerprintSteps backs requirement 29 (tasks 108 and
// 113): for every destructive path this file already proves -- kill,
// kill-undo, dd, delete-undo, reap, purge, archive, kill-and-archive, bulk
// x/dd and the batch undo -- a scenario must additionally assert with
// requirement 3's fingerprint instrument (features/fingerprint_test.go)
// that the session's own cwd never moved, not even a mtime. The only new
// primitives needed are ways to point a real create at an *already seeded*
// scratch directory (registerFingerprintSteps' seedScratchDirectory,
// task 060) rather than the fresh, empty one
// createShellSessionInLabelledCWD (features/create_session_test.go)
// always makes for itself -- one for a shell session (task 108) and one for
// a claude session with a message (task 113, purge needs a real declared
// transcript to purge).
func registerKillDeleteUndoFingerprintSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" creates shell session "([^"]+)" with cwd the scratch directory labelled "([^"]+)"$`, clientCreatesShellSessionWithScratchCWDLabelled)
	sc.Step(`^deck client "([^"]+)" creates claude session "([^"]+)" with permission profile "([^"]+)" and message "([^"]+)" with cwd the scratch directory labelled "([^"]+)"$`, clientCreatesClaudeSessionWithScratchCWDLabelled)
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

// clientCreatesClaudeSessionWithScratchCWDLabelled is
// clientCreatesShellSessionWithScratchCWDLabelled's claude counterpart
// (task 113): it points the scenario's single shared create-modal working
// directory (h.workingDir, the same field positionCreateModalOnProfileField
// and claudeTranscriptPathForSession both read) at an already-seeded
// scratch directory before delegating to the existing profile+message
// creation flow, so requirement 26's purge path can be fingerprint-tested
// against a real declared claude transcript rooted in the adversarially
// seeded directory rather than the harness's own default cwd. h.workingDir
// is otherwise only defaulted when still empty
// (positionCreateModalOnProfileField), so setting it first here is safe and
// needs no other step to cooperate.
func clientCreatesClaudeSessionWithScratchCWDLabelled(ctx context.Context, clientName, name, profile, message, label string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	dir, err := namedDirectory(h, label)
	if err != nil {
		return err
	}
	h.workingDir = dir
	return clientCreatesAgentSessionWithProfileAndMessage(ctx, clientName, "claude", name, profile, message)
}
