package theme

import (
	"sort"
	"strings"
	"testing"
)

// TestMatrixStatusTokensQuantiseToSevenDistinctReferenceEntries is R66's
// proof, and the missing quantised sibling of
// internal/tui's TestMatrixStatusTokensRenderAsSevenDistinctColours
// (which proves the same property on the true-colour path): under the
// built-in `matrix` theme, each of §7's seven status tokens
// (StatusTokens -- the six session.Status words plus the archived flag's
// own token) must quantise to its OWN ReferencePalette entry, so a
// 16-colour terminal can still tell the seven statuses apart.
//
// Distinctness is deliberately computed over the QUANTISED values, never
// over the authored hexes: matrix's authored hexes were already pairwise
// distinct while the defect was present (five of them collapsed onto ANSI
// 8 #7f7f7f -- docs/reports/phase3e-findings.md §4b), so a test written
// over Color() instead of QuantizedColor() passes with the bug in place
// and proves nothing.
//
// This is a property of `matrix` alone, on purpose. SPEC §11.6 requires
// legibility (≥ 3:1 over both the hex palette and its quantisation), NOT
// pairwise distinctness, and R66 widened one theme's palette rather than
// raising that floor for every theme -- the other built-ins still share
// ANSI 8 across idle/stopped/archived and are out of scope here.
func TestMatrixStatusTokensQuantiseToSevenDistinctReferenceEntries(t *testing.T) {
	th, ok := Builtin("matrix")
	if !ok {
		t.Fatal("built-in theme matrix is not registered")
	}
	if len(StatusTokens) != 7 {
		t.Fatalf("StatusTokens has %d entries, want 7 (§7's status set) -- this test's premise moved", len(StatusTokens))
	}

	quantised := make(map[Token]string, len(StatusTokens))
	byEntry := make(map[string][]string, len(StatusTokens))
	for _, tok := range StatusTokens {
		q, err := th.QuantizedColor(tok)
		if err != nil {
			t.Fatalf("matrix: QuantizedColor(%q): %v", tok, err)
		}
		member := false
		for _, ref := range ReferencePalette {
			if ref == q {
				member = true
				break
			}
		}
		if !member {
			t.Fatalf("matrix token %q: quantised colour %q is not a ReferencePalette entry", tok, q)
		}
		quantised[tok] = q
		byEntry[q] = append(byEntry[q], string(tok))
	}

	// Report every colliding group at once rather than short-circuiting,
	// so a regression names all the pairs it introduced.
	var collisions []string
	for q, toks := range byEntry {
		if len(toks) > 1 {
			sort.Strings(toks)
			collisions = append(collisions, q+": "+strings.Join(toks, ", "))
		}
	}
	sort.Strings(collisions)
	if len(collisions) > 0 {
		t.Errorf("matrix: %d ReferencePalette entries carry more than one of the seven §7 status tokens (not pairwise-distinct under 16-colour quantisation): %s",
			len(collisions), strings.Join(collisions, " | "))
	}
	if len(byEntry) != len(StatusTokens) {
		t.Errorf("matrix: the seven §7 status tokens quantise onto %d distinct ReferencePalette entries, want %d -- quantised map: %v",
			len(byEntry), len(StatusTokens), quantised)
	}

	// Log the full authored -> quantised table so the reconciliation in
	// TestBuiltinQuantizationPinned is auditable from a -v run.
	for _, tok := range StatusTokens {
		hex, err := th.Color(tok)
		if err != nil {
			t.Fatalf("matrix: Color(%q): %v", tok, err)
		}
		t.Logf("%-9s authored %s -> quantised %s", tok, hex, quantised[tok])
	}
}
