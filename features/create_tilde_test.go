package features

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerCreateTildeCWDSteps backs requirement 39: a leading `~`/`~/` in the
// create modal's working directory expands to the user's home and resolves
// to an absolute path, both when validated and when stored, while
// `~otheruser` is rejected with a stated reason rather than half-expanded.
func registerCreateTildeCWDSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" is started with HOME set to the scenario home$`, startNamedClientWithScenarioHome)
	sc.Step(`^deck client "([^"]+)" creates shell session "([^"]+)" with working directory "~/([^"]+)"$`, clientCreatesShellSessionWithTildeCWD)
	sc.Step(`^deck client "([^"]+)" attempts shell session "([^"]+)" with working directory "(~[^"]*)"$`, clientAttemptsShellSessionWithCWD)
	sc.Step(`^the state database session "([^"]+)" has cwd "([^"]+)" resolved under the scenario home$`, sessionHasCWDUnderScenarioHome)
}

// startNamedClientWithScenarioHome starts a named client with HOME pointed at
// the scenario's own DECK_HOME, so a `~`-prefixed cwd this scenario types
// resolves under a directory the scenario controls and tears down, rather
// than the real ambient HOME of whatever process is running the suite.
func startNamedClientWithScenarioHome(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.StartNamedClient(ctx, name, "HOME="+h.Home)
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

// clientCreatesShellSessionWithTildeCWD drives the real create modal with a
// "~/sub" working directory and waits for the session to start, proving the
// tilde expanded to an existing directory under HOME rather than being
// rejected as non-existent.
func clientCreatesShellSessionWithTildeCWD(ctx context.Context, clientName, name, sub string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	target := filepath.Join(h.Home, sub)
	if err := os.MkdirAll(target, 0o700); err != nil {
		return fmt.Errorf("create tilde-expansion target directory: %w", err)
	}
	if err := client.Send("n"); err != nil {
		return err
	}
	if err := client.WaitForFrame(ctx, false, "Create shell session"); err != nil {
		return err
	}
	if err := client.Send(name); err != nil {
		return err
	}
	time.Sleep(75 * time.Millisecond)
	if err := client.Send("\t~/" + sub + "\r"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "starting")
}

// clientAttemptsShellSessionWithCWD drives the create modal with an
// arbitrary working directory (used for the `~otheruser` rejection case) and
// leaves the modal open on whatever createError the submit produced, without
// asserting screen text itself -- the caller states what it expects.
func clientAttemptsShellSessionWithCWD(ctx context.Context, clientName, name, cwd string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send("n"); err != nil {
		return err
	}
	if err := client.WaitForFrame(ctx, false, "Create shell session"); err != nil {
		return err
	}
	if err := client.Send(name); err != nil {
		return err
	}
	time.Sleep(75 * time.Millisecond)
	if err := client.Send("\t" + cwd + "\r"); err != nil {
		return err
	}
	// The rejection is synchronous validation, not a launch outcome: give
	// the client a moment to repaint before the caller reads the frame.
	time.Sleep(150 * time.Millisecond)
	return nil
}

// sessionHasCWDUnderScenarioHome asserts the sessions table's cwd column for
// name equals filepath.Join(scenario home, relative) -- the resolved,
// absolute path -- and, redundantly but explicitly, that it does not still
// carry a leading tilde.
func sessionHasCWDUnderScenarioHome(ctx context.Context, name, relative string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	want := filepath.Join(h.Home, relative)
	var got string
	if err := db.QueryRowContext(ctx, "SELECT cwd FROM sessions WHERE name = ?", name).Scan(&got); err != nil {
		return fmt.Errorf("observe cwd for session %q: %w", name, err)
	}
	if strings.HasPrefix(got, "~") {
		return errors.New("session " + name + " cwd is still tilde-prefixed: " + got)
	}
	if got != want {
		return fmt.Errorf("session %q cwd = %q, want resolved %q", name, got, want)
	}
	return nil
}
