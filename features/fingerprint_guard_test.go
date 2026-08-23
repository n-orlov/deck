package features

// TestNoFeatureFileUsesTheI16RedDemoStep is the mechanical guard for
// fingerprint_test.go's corruptNamedDirectoryForI16RedDemo registration
// (`^the directory "([^"]+)" is corrupted with mode "([^"]+)" for an I-16
// red demonstration$`): that step is destructive (it overwrites state.db,
// backdates mtimes an hour into the future, and os.Removes .hidden) and is
// registered permanently because it is reusable, but it must be a no-op in
// every COMMITTED scenario -- task 020's own use of it in
// features/kill_delete_undo.feature was temporary and reverted before that
// task's commit landed. This test keeps that "no committed .feature names
// it" claim mechanically true (idiom: 8c7deff's colour-literal guard,
// internal/tui/no_literal_color_test.go) instead of resting on a comment or
// a commit message that nothing later enforces.
//
// Demonstrate-then-revert: temporarily add a line like
//   And the directory "scratch" is corrupted with mode "content" for an I-16 red demonstration
// to any .feature file and rerun this test -- it goes red, naming the file
// and line, confirming the check is live -- then remove the line again
// before committing.
import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// i16RedDemoStepRe matches the same Gherkin text the step definition in
// fingerprint_test.go registers, so this guard tracks the step's actual
// pattern rather than a hand-copied approximation of it.
var i16RedDemoStepRe = regexp.MustCompile(`is corrupted with mode "[^"]+" for an I-16 red demonstration`)

func TestNoFeatureFileUsesTheI16RedDemoStep(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir(features): %v", err)
	}

	checked := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".feature") {
			continue
		}
		checked++

		data, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", name, err)
		}

		for lineNum, line := range strings.Split(string(data), "\n") {
			if i16RedDemoStepRe.MatchString(line) {
				t.Errorf("%s:%d: references the I-16 red-demonstration step (corruptNamedDirectoryForI16RedDemo) -- that step is destructive and permanent only because no committed .feature file names it; remove this line", name, lineNum+1)
			}
		}
	}

	if checked == 0 {
		t.Fatalf("no .feature files found in features/ -- check is not exercising anything")
	}
}
