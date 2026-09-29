package tui

import (
	"strings"
	"testing"
)

// helpKeysSectionBlocks returns the "Keys" section as whole entries,
// leading bullet plus its indented continuation lines joined by a single
// space, keyed by the entry's leading key token(s) exactly as
// help_keymap_parity_test.go's helpKeysSectionEntries reads them (same
// delimiters: a literal "Keys" header above, the first non-indented line
// below). Unlike helpKeysSectionEntries (which returns only the bullet's
// own first line, continuations stripped), this keeps each entry's full
// text so a substring check against the whole description -- not just its
// first line -- is possible.
func helpKeysSectionBlocks(t *testing.T) []string {
	t.Helper()
	lines := strings.Split(helpText(false), "\n")
	start := -1
	for i, l := range lines {
		if l == "Keys" {
			start = i + 1
			break
		}
	}
	if start < 0 {
		t.Fatalf("could not find the \"Keys\" section header in helpText -- extraction is broken, not the source")
	}
	var blocks []string
	for i := start; i < len(lines); i++ {
		l := lines[i]
		if !strings.HasPrefix(l, "  ") {
			break
		}
		if strings.HasPrefix(l, "   ") {
			// continuation of the previous bullet
			if len(blocks) == 0 {
				t.Fatalf("Keys section continuation line with no preceding bullet: %q", l)
			}
			blocks[len(blocks)-1] += " " + strings.TrimSpace(l)
			continue
		}
		blocks = append(blocks, strings.TrimPrefix(l, "  "))
	}
	if len(blocks) == 0 {
		t.Fatalf("\"Keys\" section parsed to zero entries -- extraction is broken, not the source")
	}
	return blocks
}

// TestHelpOverlayListsSidebarPinAndNoTopLevelP is task 013's render test:
// the `?` overlay's top-level Keys section carries a `p` bullet describing
// the sidebar pin (task 010/011, SPEC §11's own pin bullet and keymap
// line), lists no top-level `P` bullet of its own (task 007 unbound
// top-level P; SPEC §11.4: "a top-level `P` is unbound"), and the only
// places `P` and `c` appear as key names in the whole Keys section are
// inside the `i` bullet's own description, naming them as actions reached
// from detail (SPEC §11.3's keymap line: "the permission profile `P` and
// the conversation lock `c` are actions inside it, not top-level keys").
func TestHelpOverlayListsSidebarPinAndNoTopLevelP(t *testing.T) {
	blocks := helpKeysSectionBlocks(t)

	var pBlock, iBlock string
	var foundP, foundI bool
	for _, b := range blocks {
		leading := leadingKeyTokens(b)
		if len(leading) == 0 {
			continue
		}
		switch leading[0] {
		case "P":
			t.Fatalf("Keys section has its own top-level `P` bullet, which SPEC §11.4 says must be unbound: %q", b)
		case "p":
			if foundP {
				t.Fatalf("more than one top-level `p` bullet found")
			}
			foundP = true
			pBlock = b
		case "i":
			if foundI {
				t.Fatalf("more than one top-level `i` bullet found")
			}
			foundI = true
			iBlock = b
		}
	}

	if !foundP {
		t.Fatalf("Keys section has no top-level `p` bullet describing the sidebar pin")
	}
	if !strings.Contains(pBlock, "pin") {
		t.Errorf("top-level `p` bullet does not mention \"pin\": %q", pBlock)
	}

	if !foundI {
		t.Fatalf("Keys section has no top-level `i` bullet")
	}
	if !strings.Contains(iBlock, "P switch the permission profile") {
		t.Errorf("`i` bullet does not describe `P` as an action reached from inside detail: %q", iBlock)
	}
	if !strings.Contains(iBlock, "c inside detail") {
		t.Errorf("`i` bullet does not describe `c` as an action reached from inside detail: %q", iBlock)
	}

	// Every OTHER top-level bullet must not itself introduce `P` or `c`
	// as its own leading key token (already checked above for `P`; `c`
	// is separately bound at the top level for fold/unfold, so its own
	// bullet is expected and is not the `i`-only action this test pins --
	// SPEC §11.3's keymap line documents both meanings of `c`: fold at
	// top level, conversation lock inside `i`). What this test pins is
	// narrower and unambiguous: `P` never has a top-level bullet, and
	// wherever `P` appears as a key name in the whole Keys section, it is
	// inside the `i` bullet's own text.
	for _, b := range blocks {
		leading := leadingKeyTokens(b)
		if len(leading) > 0 && leading[0] == "i" {
			continue
		}
		if strings.Contains(b, "`P`") || hasWholeWord(b, "P") {
			t.Errorf("a non-`i` Keys section bullet mentions `P`, which should only appear inside the `i` bullet: %q", b)
		}
	}
}

// hasWholeWord reports whether word appears in s as a standalone token
// (surrounded by whitespace or line boundaries), so a check for the
// single letter "P" does not false-positive on "PgUp" or a word that
// merely contains it.
func hasWholeWord(s, word string) bool {
	for _, f := range strings.Fields(s) {
		trimmed := strings.Trim(f, ".,;:()")
		if trimmed == word {
			return true
		}
	}
	return false
}
