package tui

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestHelpTextFEntryMatchesForcePathSemantics is task 202's source-level
// pin for task 201's fix: helpText's `F` entry (internal/tui/tui.go) must
// describe the two refusals SPEC.md 11.9 says `F` skips -- the attached-
// client refusal AND the live-ownership-holder refusal -- and must NOT
// re-claim, in its "every other refusal ↵ has ... still applies" list,
// that a live ownership holder still refuses `F`. That claim would be
// false: internal/tui/interactive.go's force path really does call
// Client.ForceClaimWindowOwnership, which acquires over a live holder
// instead of standing down for one (internal/tmux/ownership.go,
// internal/tmux/force_ownership_test.go).
//
// This test reads both source files as plain text -- not the compiled
// helpText() string -- so it keeps working even if helpText's rendering
// pipeline (ASCII/colour variants, wrapping) changes shape; the regression
// this guards is in the literal wording committed to tui.go.
func TestHelpTextFEntryMatchesForcePathSemantics(t *testing.T) {
	tuiSrc, err := os.ReadFile("tui.go")
	if err != nil {
		t.Fatalf("reading tui.go: %v", err)
	}
	interactiveSrc, err := os.ReadFile("interactive.go")
	if err != nil {
		t.Fatalf("reading interactive.go: %v", err)
	}

	block := extractHelpFEntry(t, string(tuiSrc))

	// Sanity: the force path this help entry describes really does call
	// ForceClaimWindowOwnership when force is requested. If this ever
	// stops being true, the whole premise of this test (and of SPEC
	// 11.9's F semantics) has changed underneath it, and the test should
	// say so rather than pass vacuously.
	forcePath := regexp.MustCompile(`(?s)if force \{\s*ownership, acquired, err = client\.ForceClaimWindowOwnership\(`)
	if !forcePath.MatchString(string(interactiveSrc)) {
		t.Fatalf("interactive.go's force path no longer calls ForceClaimWindowOwnership under `if force`; this test's premise no longer holds -- re-check SPEC 11.9's F semantics before touching this test")
	}

	// The entry must say F skips the live-ownership-holder refusal, not
	// only the attached-client one.
	if !strings.Contains(block, "live holder") {
		t.Fatalf("helpText's F entry no longer names the live-ownership-holder refusal as one F skips:\n%s", block)
	}

	// The entry's "every other refusal ... still applies" list must NOT
	// claim a live ownership holder still refuses F. The pre-201 wording
	// said "a live process already holding the window's own claim) still
	// applies" -- exactly the false claim this test exists to catch.
	stillApplies := regexp.MustCompile(`(?s)still applies`).FindStringIndex(block)
	if stillApplies == nil {
		t.Fatalf("helpText's F entry lost its \"still applies\" list entirely:\n%s", block)
	}
	// Look at the parenthetical immediately preceding "still applies".
	open := strings.LastIndex(block[:stillApplies[0]], "(")
	if open < 0 {
		t.Fatalf("helpText's F entry's \"still applies\" list has no opening paren:\n%s", block)
	}
	stillAppliesList := normalizeHelpWhitespace(block[open:stillApplies[0]])

	for _, bad := range []string{
		"live process",
		"holding the window",
		"own claim",
		"live holder",
		"ownership",
	} {
		if strings.Contains(stillAppliesList, bad) {
			t.Fatalf("helpText's F entry's still-applies list %q still claims a live-ownership/claim refusal applies to F (found %q), but interactive.go's force path calls ForceClaimWindowOwnership, which claims over a live holder instead of standing down for one", stillAppliesList, bad)
		}
	}
}

// extractHelpFEntry pulls the `  F force-enter ...` help line and its
// indented continuation lines out of tui.go's source, up to (but not
// including) the next top-level entry (`  a attach ...`).
func extractHelpFEntry(t *testing.T, src string) string {
	t.Helper()
	start := strings.Index(src, "  F force-enter interactive mode")
	if start < 0 {
		t.Fatalf("could not find helpText's F entry in tui.go")
	}
	rest := src[start:]
	end := strings.Index(rest, "\n  a attach the selected running session")
	if end < 0 {
		t.Fatalf("could not find the F entry's terminating `  a attach` line in tui.go")
	}
	return rest[:end]
}

// normalizeHelpWhitespace collapses newlines and repeated spaces so a
// phrase wrapped across the help text's own line-continuation indentation
// can still be matched as a contiguous substring.
func normalizeHelpWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Manual check performed while writing this test (not left in the tree):
// temporarily replacing helpText's F entry in tui.go with the exact
// pre-201 wording --
//
//   F force-enter interactive mode on the selected session, stealing it from
//     any client already attached to it -- the one refusal ↵ itself still
//     respects that F exists to skip; every other refusal ↵ has (the 7-row
//     floor, no-width squeeze, a stopped session, a live process already
//     holding the window's own claim) still applies
//
// -- made TestHelpTextFEntryMatchesForcePathSemantics fail (both the
// missing "live holder" skip-list check and the still-applies-list check
// on "holding the window" / "own claim" fired). Reverting to the current
// (task 201) wording makes it pass again.
