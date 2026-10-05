package features

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerSupersededSessionEndSteps backs features/superseded_session_end.feature
// (R200, #64).
func registerSupersededSessionEndSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the launch generation of session "([^"]+)" is remembered$`, rememberLaunchGeneration)
	sc.Step(`^the released deck _hook receives "([^"]+)" for session "([^"]+)" carrying the remembered launch generation$`, releasedHookCarryingRememberedGeneration)
	sc.Step(`^deck client "([^"]+)" screen shows no "([^"]+)" once the detail's declined-hook lookup has settled$`, clientScreenShowsNoAfterDetailLookupSettles)
}

// rememberLaunchGeneration captures the generation half of the row's
// launch_lease_owner ("pid@boot#generation"), which is what the pane the next
// restart replaces has exported as DECK_LAUNCH_GENERATION.
func rememberLaunchGeneration(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var owner string
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(launch_lease_owner, '') FROM sessions WHERE name = ?`, name).Scan(&owner); err != nil {
		return fmt.Errorf("read launch lease owner of %q: %w", name, err)
	}
	_, generation, found := strings.Cut(owner, "#")
	if !found || generation == "" {
		return fmt.Errorf("session %q holds no launch generation yet (launch_lease_owner=%q)", name, owner)
	}
	if h.rememberedLaunchGenerations == nil {
		h.rememberedLaunchGenerations = map[string]string{}
	}
	h.rememberedLaunchGenerations[name] = generation
	return nil
}

// releasedHookCarryingRememberedGeneration delivers a hook to the released
// deck _hook exactly as the replaced pane's own hook subprocess would: the
// row's id and the generation that pane was launched with, which is no longer
// the row's current one.
func releasedHookCarryingRememberedGeneration(ctx context.Context, event, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	generation := h.rememberedLaunchGenerations[name]
	if generation == "" {
		return fmt.Errorf("no launch generation was remembered for session %q", name)
	}
	id, err := sessionIDByName(h, name)
	if err != nil {
		return err
	}
	conversationID, err := sessionConversationID(h, name)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{"hook_event_name": event, "session_id": conversationID, "reason": "other"})
	if err != nil {
		return err
	}
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, h.Binary, "_hook")
	cmd.Env = append(os.Environ(), h.Environment("DECK_SESSION_ID="+id, "DECK_LAUNCH_GENERATION="+generation)...)
	cmd.Stdin = bytes.NewReader(payload)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("released deck _hook for %q event %q: %w: %s", name, event, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// detailLookupSettleWindow is how long the detail dialog gets to deliver its
// declined-hook lookup (one store read dispatched as a tea.Cmd when `i`
// opens, R61) before a negative assertion is meaningful: a bare "not on the
// screen yet" would also pass for a line that simply had not arrived.
const detailLookupSettleWindow = time.Second

func clientScreenShowsNoAfterDetailLookupSettles(ctx context.Context, clientName, unwanted string) error {
	time.Sleep(detailLookupSettleWindow)
	return clientScreenDoesNotContain(ctx, clientName, unwanted)
}
