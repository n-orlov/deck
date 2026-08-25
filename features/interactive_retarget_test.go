package features

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

// registerInteractiveRetargetSteps backs the two @requirement-54 scenarios
// in features/mouse.feature (task 314, SPEC's post-6584299 "a sidebar
// click works while interactive mode is active and re-targets it"):
// clicking a different sidebar row while interactive mode owns a session's
// tmux window leaves that window (restoring its geometry byte-exact, the
// same teardown Ctrl+Q performs) and enters the new one, while clicking
// the row that is ALREADY the interactive target is a no-op -- no leave,
// no re-enter, no resize, proven here by the tmux window's own ownership
// claim option (a fresh random tag on every real claim, see
// privateWindowOwnershipClaim below) rather than by an unchanged final
// size alone: a leave-then-re-enter that lands the window back at its
// starting size, with no client ever attached to force a real reflow,
// would satisfy that weaker check without satisfying this one.
func registerInteractiveRetargetSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" preview top border contains "([^"]+)"$`, clientPreviewTopBorderContains)
	sc.Step(`^the private tmux window ownership claim for session "([^"]+)" is captured as "([^"]+)"$`, privateTMuxWindowOwnershipClaimForSessionIsCapturedAs)
	sc.Step(`^the private tmux window ownership claim for session "([^"]+)" still matches "([^"]+)"$`, privateTMuxWindowOwnershipClaimForSessionStillMatches)
}

// windowOwnershipOption mirrors internal/tmux/ownership.go's own
// OwnershipOption constant by its literal value rather than importing the
// package -- every other black-box step in this file reads deck's state
// through the released binary's own rendered frame or tmux's own public
// state, never an internal Go type, and a window user option read via a
// real `tmux show-options` is exactly that: public tmux state, not an
// internal channel.
const windowOwnershipOption = "@deck_isize_owner"

// privateWindowOwnershipClaim reads task 313/R54's own no-op discriminator:
// ClaimWindowOwnership (internal/tmux/ownership.go) writes a FRESH,
// cryptographically random `<tag>:<pid>` value on every successful claim,
// including a claim that re-acquires the SAME window for the SAME pid a
// moment after releasing it -- unlike the window's own geometry, which a
// leave-then-re-enter to an unchanged preview size can restore-then-refit
// back to a byte-identical value with no observable resize in between
// (tmux only signals a real size *change*), this tag changes on every
// single claim, with no such coincidental collision possible short of a
// 64-bit random draw repeating. A click that is genuinely a no-op (task
// 313's guard) never calls ClaimWindowOwnership again at all, so the
// option's value is untouched.
func privateWindowOwnershipClaim(ctx context.Context, h *ScenarioHarness, name string) (string, error) {
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return "", err
	}
	target := "deck_" + slug
	output, err := tmuxOutput(ctx, h, "show-options", "-wv", "-t", target, windowOwnershipOption)
	if err != nil {
		return "", fmt.Errorf("read private tmux window ownership claim for %q: %w", target, err)
	}
	claim := strings.TrimSpace(string(output))
	if claim == "" {
		return "", fmt.Errorf("private tmux window %q reported no ownership claim", target)
	}
	return claim, nil
}

func privateTMuxWindowOwnershipClaimForSessionIsCapturedAs(ctx context.Context, name, label string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	claim, err := privateWindowOwnershipClaim(ctx, h, name)
	if err != nil {
		return err
	}
	if h.windowOwnershipSnapshots == nil {
		h.windowOwnershipSnapshots = make(map[string]string)
	}
	h.windowOwnershipSnapshots[label] = claim
	return nil
}

func privateTMuxWindowOwnershipClaimForSessionStillMatches(ctx context.Context, name, label string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	want, ok := h.windowOwnershipSnapshots[label]
	if !ok {
		return fmt.Errorf("no private tmux window ownership claim was captured as %q", label)
	}
	got, err := privateWindowOwnershipClaim(ctx, h, name)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("private tmux window ownership claim for session %q changed (a leave+re-enter re-claimed it): captured %q as %q, now %q", name, label, want, got)
	}
	return nil
}

// clientPreviewTopBorderContains asserts that the substring named by the
// interactive target's own identity ("While interactive, the preview's
// top border therefore carries the target session's name as text",
// SPEC.md's own words for this exact safeguard) appears specifically in
// the preview panel's own top border segment -- never merely somewhere on
// screen, since the sidebar always lists every session's name regardless
// of which one (if any) is the interactive target, which would make a
// plain "screen contains" assertion pass whether or not the retarget
// actually happened.
func clientPreviewTopBorderContains(ctx context.Context, name, text string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	frame := client.Frame(false)
	border, err := previewTopBorderText(frame)
	if err != nil {
		return fmt.Errorf("client %q: %w", name, err)
	}
	if !strings.Contains(border, text) {
		return fmt.Errorf("client %q preview top border %q does not contain %q\nfull frame:\n%s", name, border, text, frame)
	}
	return nil
}

// previewTopBorderText isolates the preview panel's own top border text
// from a full rendered frame, reusing layout_modes_test.go's shape-based
// detection (detectLayoutMode/seamColumn) rather than importing
// internal/tui, exactly as every other black-box step in this package
// does. Side-by-side and collapsed modes share one top border row with
// the sidebar (the seam's T-junction divides it); stacked mode draws the
// preview as its own fully-bordered box below the sidebar's, so its top
// border is a later, independent line.
func previewTopBorderText(frame string) (string, error) {
	mode, err := detectLayoutMode(frame)
	if err != nil {
		return "", err
	}
	lines := strings.Split(frame, "\n")
	if mode == "stacked" {
		for i := 1; i < len(lines); i++ {
			trimmed := strings.TrimRight(lines[i], " ")
			if strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "\u256d") {
				return lines[i], nil
			}
		}
		return "", fmt.Errorf("no second panel top border found in stacked frame:\n%s", frame)
	}
	col, err := seamColumn(frame)
	if err != nil {
		return "", err
	}
	if len(lines) == 0 {
		return "", fmt.Errorf("empty frame")
	}
	runes := []rune(lines[0])
	if col >= len(runes) {
		return "", fmt.Errorf("seam column %d out of range on top border line %q", col, lines[0])
	}
	return string(runes[col:]), nil
}
