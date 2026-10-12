package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

const (
	// applyNameVerb is the hidden verb a detached `deck new` re-executes to type
	// a launch name into a harness that takes it only by keyboard once ready.
	applyNameVerb = "_apply_name"
	// applyNameWait bounds how long that process waits for the prompt.
	applyNameWait = 2 * time.Minute
	// applyNamePoll is how often it looks at the screen.
	applyNamePoll = 250 * time.Millisecond
	// applyNameOpenTries and applyNameOpenPause bound the store-open retry.
	applyNameOpenTries = 20
	applyNameOpenPause = 100 * time.Millisecond
)

func isApplyNameRequest(args []string) bool {
	return len(args) == 3 && args[1] == applyNameVerb
}

// startNameApplier launches `deck _apply_name <id>` in its own session with no
// terminal, so it outlives the `deck new` that started it. It is a seam: tests
// run the applier in-process.
var startNameApplier = spawnNameApplier

// applierExecutable is the binary the applier re-executes: deck itself.
var applierExecutable = absoluteExecutable

func spawnNameApplier(sessionID string) error {
	executable, err := applierExecutable()
	if err != nil {
		return err
	}
	cmd := exec.Command(executable, applyNameVerb, sessionID) // #nosec G204 -- deck's own binary, verb fixed, id from its own store
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// runApplyNameCommand is the hidden `_apply_name` verb: exit 0 once the name is
// typed (or nothing is left to type), 1 with the reason on stderr otherwise.
// A failure is also on the row as harness_rename_failed; deck's own name is
// never touched.
func runApplyNameCommand(args []string, stdin io.Reader, stderr io.Writer) int {
	_, code, proceed := resolveStartupProfile([]string{args[0]}, false, stdin, stderr)
	if !proceed {
		return code
	}
	settings, err := config.LoadFromProfile(os.Getenv, os.UserHomeDir, "")
	if err != nil {
		sayln(stderr, "deck configuration:", err)
		return 1
	}
	db, sessions, err := openServiceAfterCreator(settings)
	if err != nil {
		sayln(stderr, "deck state:", err)
		return 1
	}
	defer func() { _ = db.Close() }()
	if err := sessions.ApplyLaunchNameWhenReady(context.Background(), args[2], applyNameWait, applyNamePoll); err != nil {
		_ = sessions.Audit.Event("deck _apply_name " + args[2] + ": " + err.Error())
		sayln(stderr, "deck:", err)
		return 1
	}
	return 0
}

// openServiceAfterCreator opens the store the way every command does, but
// retries a few times: the `deck new` that started this process may still hold
// the database for the instant its own close takes, and opening reads the
// schema version before SQLite's busy timeout is in force.
func openServiceAfterCreator(settings config.Settings) (*store.Store, service.Service, error) {
	registry := newAgentRegistry()
	db, sessions, err := openDetachedService(settings, registry)
	for attempt := 1; err != nil && attempt < applyNameOpenTries; attempt++ {
		time.Sleep(applyNameOpenPause)
		db, sessions, err = openDetachedService(settings, registry)
	}
	return db, sessions, err
}
