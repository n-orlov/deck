package tui

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoPrivateTextEditingOutsideLineedit is the R178 source-scan guard: every
// text field edits through internal/tui/lineedit, so no other non-test source
// file of internal/tui may carry a text handler's own backspace case, its own
// rune append, the per-field backspace helpers, the settings "_" cursor, or
// an "_" appended as a caret stand-in.
func TestNoPrivateTextEditingOutsideLineedit(t *testing.T) {
	offences := []struct {
		what string
		re   *regexp.Regexp
	}{
		{`a private backspace case`, regexp.MustCompile(`case\s+"backspace"\s*,\s*"ctrl\+h"`)},
		{`a private rune append`, regexp.MustCompile(`\+=\s*string\(\s*(msg\.)?[rR]unes\s*\)`)},
		{`a definition of backspaceCreateField`, regexp.MustCompile(`\bfunc\s+(\([^)]*\)\s*)?backspaceCreateField\b`)},
		{`a definition of backspaceLaunchInputsField`, regexp.MustCompile(`\bfunc\s+(\([^)]*\)\s*)?backspaceLaunchInputsField\b`)},
		{`a definition of settingsTextCursor`, regexp.MustCompile(`\b(const|var|func)\s+settingsTextCursor\b|\bsettingsTextCursor\s*(:=|=)`)},
		{`an underscore caret stand-in`, regexp.MustCompile(`\+\s*"_"`)},
	}

	var found []string
	scanned := 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// The shared editor's own source, and fixtures, are exempt.
			if path == "lineedit" || d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		scanned++
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
		for n := 1; sc.Scan(); n++ {
			line := sc.Text()
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			for _, o := range offences {
				if o.re.MatchString(line) {
					found = append(found, fmt.Sprintf("%s:%d: %s: %s", path, n, o.what, strings.TrimSpace(line)))
				}
			}
		}
		return sc.Err()
	})
	if err != nil {
		t.Fatalf("scan internal/tui: %v", err)
	}
	if scanned == 0 {
		t.Fatal("scanned no non-test Go files: the guard would pass vacuously")
	}
	if len(found) > 0 {
		t.Fatalf("%d private text-editing site(s) outside internal/tui/lineedit; edit through the shared editor instead:\n%s",
			len(found), strings.Join(found, "\n"))
	}
}
