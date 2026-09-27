package features

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cucumber/godog"
)

// registerProfileCreationSteps wires SPEC §3.4's lazy profile-creation
// scenario (task 011, R153): launching deck for a not-yet-existing but
// validly named profile over a real pty, answering its terminal creation
// prompt, and proving the created profile's header, its own derived socket
// and its on-disk directories. The socket is proven by a session created
// through the released binary landing live on tmux -L deck-<name>, not by
// the header: SPEC §3.4 elides the header's socket half first when the
// sidebar is too narrow, as it is at this harness's default width.
func registerProfileCreationSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" is launched for the not-yet-existing profile "([^"]+)"$`, launchClientForNewProfile)
	sc.Step(`^deck client "([^"]+)" screen shows the creation prompt for profile "([^"]+)"$`, clientShowsCreationPrompt)
	sc.Step(`^deck client "([^"]+)" answers the creation prompt with "([^"]*)"$`, clientAnswersCreationPrompt)
	sc.Step(`^the profile "([^"]+)" data and log directories exist under the scenario's data root$`, profileDirectoriesExist)
}

// profileCreationPromptTimeout bounds the wait for the creation prompt.
// This text is printed by confirmAndCreateProfile (cmd/deck/main.go)
// before config.LoadFromProfile, store.Open or tea.NewProgram ever run --
// well before any bubbletea probe -- so it needs nowhere near the ~5s
// bubbletea-init gotcha cmd/deck's own pty tests note; 10s is generous
// headroom over the sub-second cost this actually measures at.
const profileCreationPromptTimeout = 10 * time.Second

// launchClientForNewProfile starts a client via
// ScenarioHarness.StartNamedClientForNewProfile and waits for the resulting
// process to reach a stable point: the terminal has just been handed the
// pty, so nothing else needs to settle before the very next step waits for
// the prompt text itself.
func launchClientForNewProfile(ctx context.Context, name, profile string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	if _, err := h.StartNamedClientForNewProfile(ctx, name, profile); err != nil {
		return fmt.Errorf("launch deck client %q for new profile %q: %w", name, profile, err)
	}
	return nil
}

// clientShowsCreationPrompt waits for SPEC §3.4's exact terminal prompt --
// "deck: no profile %q yet (known: %s). Create it? [y/N]" -- with "known"
// fixed at "default" because this step is only ever reached from a fresh
// scenario DECK_HOME whose profiles/ directory does not exist yet
// (config.KnownProfiles's own fallback for that case, mirrored here as a
// literal string per this package's black-box-only rule rather than by
// importing internal/config to compute it).
func clientShowsCreationPrompt(ctx context.Context, name, profile string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	want := fmt.Sprintf("deck: no profile %q yet (known: default). Create it? [y/N]", profile)
	wait, cancel := context.WithTimeout(ctx, profileCreationPromptTimeout)
	defer cancel()
	if err := client.WaitForFrame(wait, false, want); err != nil {
		return fmt.Errorf("waiting for profile %q's creation prompt: %w", profile, err)
	}
	return nil
}

// clientAnswersCreationPrompt writes answer plus a line terminator to the
// client's pty -- the exact byte shape confirmAndCreateProfile's
// bufio.Reader.ReadString('\n') reads from a real terminal, matching how
// cmd/deck's own pty-driven profile-creation tests answer it
// (cmd/deck/profile_creation_test.go).
func clientAnswersCreationPrompt(ctx context.Context, name, answer string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send(answer + "\n"); err != nil {
		return fmt.Errorf("answer creation prompt for client %q: %w", name, err)
	}
	return nil
}

// profileDirectoriesExist asserts SPEC §3.4's on-disk result of creation
// under $DECK_HOME mode's layout table: profiles/<name>/ (the profile's
// data directory, and -- in this mode -- also where its config.toml
// lands) and profiles/<name>/log/ together. It reads the scenario's own
// DECK_HOME (h.Home) directly rather than any internal/config helper, to
// stay a black-box assertion about the documented directory layout.
func profileDirectoriesExist(ctx context.Context, profile string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	dataDir := filepath.Join(h.Home, "profiles", profile)
	logDir := filepath.Join(dataDir, "log")
	for _, dir := range []string{dataDir, logDir} {
		info, err := os.Stat(dir)
		if err != nil {
			return fmt.Errorf("profile %q directory %s: %w", profile, dir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("profile %q path %s exists but is not a directory", profile, dir)
		}
	}
	return nil
}
