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
func TestHookRulesAreStatedSomewhereAUserCanReach(t *testing.T) {
	model := New(nil, config.Settings{Socket: "test-socket"}, "")

	var blob strings.Builder
	blob.WriteString(helpText(false))
	blob.WriteString("\n")

	for _, row := range model.createFieldRows() {
		if row.label == "Pre-launch command" {
			blob.WriteString(row.help)
			blob.WriteString("\n")
		}
	}
	for _, row := range model.launchInputsFieldRows() {
		if row.label == "Pre-launch command" || row.label == "Post-destroy command" {
			blob.WriteString(row.help)
			blob.WriteString("\n")
		}
	}
	for _, field := range config.Schema {
		if field.Key == "pre_launch" || field.Key == "post_destroy" {
			blob.WriteString(field.Description)
			blob.WriteString("\n")
		}
	}

	text := strings.ToLower(blob.String())

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
