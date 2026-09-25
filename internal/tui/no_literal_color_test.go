package tui

// TestNoColorLiterals is the mechanical guard for SPEC requirement 33: every
// colour this package emits must come from internal/theme's tokens, never
// from a literal baked into internal/tui's own source. It walks this
// package's non-test .go files (test files legitimately fabricate raw SGR
// bytes to simulate externally-captured pane content, e.g. a tmux
// capture-pane row -- that is fixture data standing in for someone else's
// escape codes, not this package's own styling choice) and fails on:
//
//   - a hex colour literal ("#rrggbb") anywhere in the file;
//   - a lipgloss.Color("...") call;
//   - a raw SGR colour escape literal, i.e. "\x1b[<code>m" where <code> is
//     written out as digits/semicolons rather than built from a variable
//     (theme_color.go's fmt.Sprintf("\x1b[%dm", code) templates don't match
//     this pattern -- "%d" isn't [0-9;] -- so the one legitimate dynamic
//     construction site stays green). The bare reset "\x1b[0m" is exempted:
//     0 selects no colour, it only clears attributes, and every dialog/list
//     renderer that shares a background across several foreground runs
//     (settings.go's settingsRenderRow, theme_color.go's colorToken/
//     bgColorToken) legitimately emits it once per composed line. "\x1b[7m"/
//     "\x1b[27m" (reverse video on/off, theme_color.go's reverseVideo, task
//     003/R141) are exempted for the identical reason: 7/27 select no
//     colour either, they only invert/restore whatever foreground and
//     background are already active -- exactly the attribute-not-colour
//     carve-out 0 already has, and precisely why R141 (SPEC §11: the header
//     cursor cue must survive NO_COLOR/ascii) reaches for this pair rather
//     than a theme token in the first place.
//
// Demonstrate-then-revert: temporarily add `const literalRed = "\x1b[31m"`
// to tui.go and rerun this test -- it goes red, confirming the check is
// live, not decorative -- then remove it again before committing.
import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	hexColourLiteralRe = regexp.MustCompile(`#[0-9A-Fa-f]{6}`)
	lipglossColorRe    = regexp.MustCompile(`lipgloss\.Color\(`)
	rawSGRLiteralRe    = regexp.MustCompile(`\\x1b\[([0-9;]+)m`)
)

func TestNoColorLiterals(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir(internal/tui): %v", err)
	}

	checked := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			continue // test fixtures may fabricate raw SGR to simulate captured panes
		}
		checked++

		data, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", name, err)
		}
		src := string(data)

		if loc := hexColourLiteralRe.FindStringIndex(src); loc != nil {
			t.Errorf("%s: hex colour literal %q found outside internal/theme -- colour must come from a theme token", name, src[loc[0]:loc[1]])
		}
		if loc := lipglossColorRe.FindStringIndex(src); loc != nil {
			t.Errorf("%s: lipgloss.Color(...) literal found -- colour must come from a theme token, not a hardcoded lipgloss colour", name)
		}
		for _, m := range rawSGRLiteralRe.FindAllStringSubmatch(src, -1) {
			code := m[1]
			if code == "0" || code == "7" || code == "27" {
				continue // bare reset / reverse-video on/off: attributes, not a colour selection
			}
			t.Errorf("%s: raw SGR colour escape literal %q found -- colour must be emitted through colorToken/bgColorToken from a theme token, not a hardcoded escape sequence", name, m[0])
		}
	}

	if checked == 0 {
		t.Fatalf("no non-test .go files found in internal/tui -- check is not exercising anything")
	}
}
