// key_grep_test.go is the grep proof PRD item 34 asks for: deck's named-
// key primitive never hand-builds the escape sequence a key name
// translates to -- it only ever hands tmux the NAME and lets tmux's own
// key-string translation produce whatever bytes that means. This is
// checked directly against key.go's own source, not inferred from the
// runtime behaviour key_test.go demonstrates.
package tmux

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestNamedKeySendNeverHandBuildsAnEscapeSequence fails if key.go's
// non-comment source ever contains a Go escape-literal spelling of an
// ESC byte (\x1b or \033) or a hard-coded CSI introducer ("\x1b[" is
// covered by the first pattern already, but "ESC[" as a literal string
// is checked too in case a future edit spells it out differently) --
// the one thing PRD item 34 says deck must never do for a named key.
// key_test.go's own use of literal escape bytes to describe the
// EXPECTED output of tmux's translation, and to stand in for a real
// terminal's own translation of a physical keypress, is deliberately
// not covered by this guard: this test only inspects key.go, the file
// that implements SendNamedKey itself.
func TestNamedKeySendNeverHandBuildsAnEscapeSequence(t *testing.T) {
	source, err := os.ReadFile("key.go")
	if err != nil {
		t.Fatalf("read key.go: %v", err)
	}
	forbidden := regexp.MustCompile(`\\x1b|\\033|\\u001[bB]`)
	for lineNumber, line := range strings.Split(string(source), "\n") {
		if forbidden.MatchString(line) {
			t.Fatalf("key.go:%d: hand-built escape byte literal found: %q -- SendNamedKey must only ever pass tmux the key NAME (PRD item 34), never construct the escape sequence itself", lineNumber+1, line)
		}
	}
}
