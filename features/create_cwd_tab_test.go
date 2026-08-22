package features

import (
	"context"
	"time"

	"github.com/cucumber/godog"
)

// registerCreateCWDTabSteps backs requirement 16 (§11.7 tab-completion
// contract, task 012): tab completes the create modal's cwd field to the
// longest common prefix shared by every directory candidate for the
// segment being completed when that advances the text already typed, and
// otherwise -- when it cannot advance any further and more than one
// candidate remains -- lists the candidates for selection; choosing one
// puts it in the field. Reuses task 010/011's scratch-directory and
// cwd-field steps (features/create_cwd_ghost_test.go) rather than
// duplicating their setup.
func registerCreateCWDTabSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" presses "(tab|up|down|enter|esc)" in the cwd field$`, clientPressesTabContractKeyInCWDField)
	sc.Step(`^deck client "([^"]+)" screen contains the scratch directory labelled "([^"]+)" plus "([^"]*)"$`, clientScreenContainsScratchDirPlus)
}

// clientPressesTabContractKeyInCWDField sends the real terminal byte(s)
// for each of task 012's declared per-field keys in the cwd field: tab
// (completes or lists), up/down (moves the highlighted candidate while a
// list is open), enter (accepts the highlighted candidate while a list is
// open -- callers only use "enter" here while asserting that, never to
// submit the whole modal; clientSubmitsCreateModal owns that), and esc
// (closes an open list without cancelling the modal). A separate step from
// clientPressesKeyInCWDField (right/end, task 010) since those two never
// need to share a regex.
func clientPressesTabContractKeyInCWDField(ctx context.Context, name, key string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	seq := map[string]string{
		"tab":   "\t",
		"up":    "\x1b[A",
		"down":  "\x1b[B",
		"enter": "\r",
		"esc":   "\x1b",
	}[key]
	if err := client.Send(seq); err != nil {
		return err
	}
	time.Sleep(60 * time.Millisecond)
	return nil
}

// clientScreenContainsScratchDirPlus asserts the currently rendered frame
// contains the scratch directory labelled label's real, absolute path
// joined with suffix (e.g. "/prefix") -- the on-screen counterpart of
// sessionHasCWDExactlyScratchDirPlus, used here where a scenario checks the
// field's rendered value before ever submitting.
func clientScreenContainsScratchDirPlus(ctx context.Context, name, label, suffix string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	dir, err := namedDirectory(h, label)
	if err != nil {
		return err
	}
	return clientScreenContains(ctx, name, dir+suffix)
}
