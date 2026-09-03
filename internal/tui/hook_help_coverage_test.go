package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// TestHookRulesAreStatedSomewhereAUserCanReach is PRD R109's own test, in
// the shape TestCreateFieldRowsPreLaunchLoginShellLaunchArgsDescribeWhatTheyDo
// (create_field_help_test.go) already uses for a single field: gather the
// copy from every surface R109 names -- the "?" help overlay, the create
// modal's pre_launch field help, the launch-inputs editor's two hook
// field rows, and the two global schema descriptions -- into one blob,
// then assert each of R109's five claims is present *somewhere* in that
// blob, by substance (a handful of required phrases, not a verbatim
// sentence match, so a copy edit that keeps the meaning keeps passing).
// It never asserts which single surface carries which claim: R109 says
// "between them", so the claims may double up or move between surfaces
// as the copy evolves, as long as a user can reach every one of them
// from at least one place.
// hookHelpCombinedBlob rebuilds the exact same combined copy blob both
// tests in this file assert against: the "?" help overlay, the create
// modal's pre_launch field help, the launch-inputs editor's two hook
// field rows, and the two global schema descriptions.
func hookHelpCombinedBlob() string {
	model := New(nil, config.Settings{Socket: "test-socket"}, "")

	// Every fragment is terminated with its own ".\n" (even one that
	// already ends in punctuation -- a stray ".." is harmless) so that
	// sentence-level checks (see
	// TestHookHelpCopyIsAccurateAboutEvalAndFailClosedTimeout) can split the
	// combined blob on "." without a fragment lacking its own terminal
	// punctuation (e.g. a field-help row) running on into the next
	// fragment's sentence.
	var blob strings.Builder
	blob.WriteString(helpText(false))
	blob.WriteString(".\n")

	for _, row := range model.createFieldRows() {
		if row.label == "Pre-launch command" {
			blob.WriteString(row.help)
			blob.WriteString(".\n")
		}
	}
	for _, row := range model.launchInputsFieldRows() {
		if row.label == "Pre-launch command" || row.label == "Post-destroy command" {
			blob.WriteString(row.help)
			blob.WriteString(".\n")
		}
	}
	for _, field := range config.Schema {
		if field.Key == "pre_launch" || field.Key == "post_destroy" {
			blob.WriteString(field.Description)
			blob.WriteString(".\n")
		}
	}

	return blob.String()
}

func TestHookRulesAreStatedSomewhereAUserCanReach(t *testing.T) {
	text := strings.ToLower(hookHelpCombinedBlob())

	claims := []struct {
		name    string
		phrases []string
	}{
		{
			name:    "a launch hook runs on every launch and must be idempotent",
			phrases: []string{"every launch", "idempotent"},
		},
		{
			name:    "a launch hook is fail-closed: the session refuses to start if it fails",
			phrases: []string{"fail-closed", "never starts"},
		},
		{
			name:    "a teardown hook is fail-open, runs on A and dd and not on x",
			phrases: []string{"fail-open", "archive", "dd (delete)", "not after x"},
		},
		{
			name:    "a post_destroy plus an undo brings the row back stopped and the next r rebuilds",
			phrases: []string{"undo", "comes back stopped", "rebuild"},
		},
		{
			name:    "the safe secret shape: export K=V on stdout, diagnostics to stderr, never echo, else sensitive",
			phrases: []string{"export k=v", "stderr", "never echo", "sensitive"},
		},
	}

	for _, claim := range claims {
		for _, phrase := range claim.phrases {
			if !strings.Contains(text, strings.ToLower(phrase)) {
				t.Errorf("claim %q not reachable: missing phrase %q in the combined help/field-help/schema copy", claim.name, phrase)
			}
		}
	}
}

// TestHookHelpCopyIsAccurateAboutEvalAndFailClosedTimeout is task 046's
// guard on the two inaccurate phrasings task 045 fixed (finding 3):
//   - the safe-secret-shape sentence must describe the CALLER doing
//     `eval "$(...)"` on the hook's stdout, never "deck" doing the eval --
//     deck never sources/evals a hook's stdout itself, only the pane's own
//     shell does after the caller's `eval "$(...)"`.
//   - the launch hook (pre_launch) is fail-closed on a non-zero exit only;
//     unlike post_destroy (which is fail-open and legitimately mentions a
//     timeout), no fail-closed sentence may claim a timeout also blocks the
//     launch -- pre_launch has no timeout semantics.
//
// It asserts both the positive (caller-eval shape present) and the two
// negatives (the exact old wordings absent) over the same combined blob
// TestHookRulesAreStatedSomewhereAUserCanReach builds, so a regression on
// either surface (tui.go's "?" help or launch_inputs.go's field-help row)
// fails this test the same way it would have failed against the
// pre-task-045 copy.
func TestHookHelpCopyIsAccurateAboutEvalAndFailClosedTimeout(t *testing.T) {
	text := strings.ToLower(hookHelpCombinedBlob())

	if !strings.Contains(text, "caller to eval") {
		t.Errorf("missing the caller-eval shape: expected a sentence saying the CALLER runs eval \"$(...)\" on the hook's stdout (got: %q)", text)
	}

	if strings.Contains(text, "deck to eval") {
		t.Errorf("found the inaccurate phrasing %q: deck never evals a hook's stdout itself, only the caller/pane shell does", "deck to eval")
	}

	normalized := strings.Join(strings.Fields(text), " ")
	for _, sentence := range strings.Split(normalized, ".") {
		if strings.Contains(sentence, "fail-closed") && strings.Contains(sentence, "timeout") {
			t.Errorf("found a launch-hook fail-closed sentence claiming a timeout: %q (pre_launch is fail-closed on a non-zero exit only; only post_destroy's fail-open sentence may mention a timeout)", sentence)
		}
	}
}
