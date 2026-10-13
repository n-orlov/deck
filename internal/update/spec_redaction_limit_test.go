package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSpecStatesShortCredentialCollisionLimit fails if SPEC.md's "Update error
// redaction" row loses the sentence naming the limit of the guarantee: a 1-3 byte
// credential that coincides with non-secret text of the configured URL.
func TestSpecStatesShortCredentialCollisionLimit(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "SPEC.md"))
	if err != nil {
		t.Fatal(err)
	}
	var row string
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "| **Update error redaction**") {
			row = line
			break
		}
	}
	if row == "" {
		t.Fatal("SPEC.md has no Update error redaction row")
	}
	for _, want := range []string{
		"a secret of at least 4 bytes is also replaced wherever it appears",
		"A configured credential of 1-3 bytes that coincides with non-secret text of the configured URL",
		"its scheme, host, port, path or a query key",
		"a two-byte `/q` equal to part of the path",
		"is outside this guarantee",
	} {
		if !strings.Contains(row, want) {
			t.Errorf("Update error redaction row lacks %q", want)
		}
	}
}
