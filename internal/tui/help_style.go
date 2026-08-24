package tui

import (
	"strings"

	"github.com/n-orlov/deck/internal/theme"
)

// helpStyleHeaders is the set of helpText section-header lines that
// helpView colours with theme.Title (task 082, steer 005 item 2). It is
// an explicit literal list, not a structural heuristic over indentation,
// because helpText mixes true section headers ("Keys", "Runtime
// controls") with free-floating prose paragraphs that also sit at column
// 0 ("Yolo is gated by allow_yolo...", "Sessions use a private tmux server...")
// and with the dialog's own title line ("deck help", already visually
// set apart by framedDialogScrollable's border) and its closing sentence
// ("? closes help; Esc closes help; q quits deck.") -- none of which are
// section headers. Keeping the list explicit means a helpText wording
// change that silently drops a header just stops matching (rendered
// plain, not mis-detected) rather than guessing wrong; TestHelpStyle
// EveryHeaderGetsStyled (help_style_test.go) is what pins each string as
// still present verbatim, not this list.
//
// "Create dialog fields" is included alongside the four the task named
// explicitly (Keys, Settings, Theme picker, Mouse) plus "Runtime
// controls" (the env-var list) because it is structurally identical: a
// column-0 heading immediately followed by indented entries, no
// different in kind from the others. Recorded here, and in
// docs/reports/phase3.md's Steer 005 item 2 subsection, as a deliberate
// scope choice, not a silent addition.
var helpStyleHeaders = map[string]bool{
	"Keys":                              true,
	"Create dialog fields":              true,
	"Settings takeover (opened with ,)": true,
	"Theme picker (opened with t)":      true,
	"Runtime controls":                  true,
	"Mouse (every binding duplicates a key above; nothing here is mouse-only)": true,
}

// helpKeycapTokens is the "Keys" section's own keycap-phrase vocabulary
// (task 082): the leading token(s) of every "Keys" section entry, used
// only to decide where a rendered entry's keycap phrase ends and its
// prose begins. It is a second, independently-maintained copy of
// help_keymap_parity_test.go's helpKeyTokenToBoundKeys (same "Keys"
// section, same tokens) rather than a shared one, because that map lives
// in a _test.go file (unavailable to production code) and encodes a
// different fact about each token (which raw bound key(s) it names, for
// the requirement-38 cross-check) than this one needs (merely: is this
// token part of a keycap phrase, for colouring). A token added to the
// "Keys" section without a matching entry here still renders -- just
// without the Key-token colour on that one entry's leading phrase, caught
// by TestHelpKeysEntriesAllGetKeycapStyling (help_style_test.go) going
// red, not by any corruption of the help text itself: helpText is never
// touched by this file.
var helpKeycapTokens = map[string]bool{
	"↑/↓": true, "j/k": true, "↵": true, "a": true, "Y": true, "n": true,
	"x": true, "u": true, "dd": true, "A": true, "m": true, "r": true,
	"R": true, "P": true, "p": true, "i": true, "e": true, "E": true,
	"/": true, "space": true, "c": true, "g": true, "G": true, ",": true,
	"t": true, "|": true, "<": true, ">": true, "?": true, "q": true,
	"Ctrl+C": true, "PgUp/PgDn": true, "or": true,
}

// splitLeadingKeyPhrase finds the boundary between a "Keys" section
// entry's leading keycap phrase (e.g. "↑/↓ or j/k", "q or Ctrl+C", "dd")
// and its prose, by walking whitespace-separated tokens while each is in
// helpKeycapTokens and stopping at the first one that is not -- which is
// always the first word of the entry's own prose, since every real
// entry's key phrase is exactly its leading token(s). It operates on the
// entry line with its 2-space indent already stripped. An entry with no
// recognised leading token (n==0, never observed against today's helpText
// but handled defensively) returns ("", line): the caller renders it
// unstyled rather than guessing where prose starts.
func splitLeadingKeyPhrase(line string) (phrase, rest string) {
	fields := strings.Fields(line)
	n := 0
	for _, f := range fields {
		if !helpKeycapTokens[f] {
			break
		}
		n++
	}
	if n == 0 {
		return "", line
	}
	idx := 0
	count := 0
	for i := 0; i < len(line); {
		for i < len(line) && line[i] == ' ' {
			i++
		}
		start := i
		for i < len(line) && line[i] != ' ' {
			i++
		}
		if start == i {
			break
		}
		count++
		if count == n {
			idx = i
			break
		}
	}
	return line[:idx], line[idx:]
}

// styledHelpText renders helpText's structured (but unstyled) content
// through theme tokens (task 082, steer 005 item 2): section headers
// (helpStyleHeaders) in theme.Title; inside the "Keys" section only, each
// entry's leading keycap phrase in theme.Key, the rest of that entry's
// first line in theme.Text, and that entry's wrapped continuation lines
// (indented 3+ spaces, task 021/078's own boundary) in theme.Dimmed.
//
// helpText itself is never touched -- it stays plain, unstyled structured
// data -- because help_keymap_parity_test.go's helpKeysSectionEntries
// depends on that in three ways: locating the "Keys" section by exact
// string equality, splitting entries from continuations via a "  " vs
// "   " prefix check, and matching each entry's leading token(s) against
// a fixed vocabulary to translate it into the bound key(s) it names. Any
// escape sequence embedded in helpText's own string breaks all three.
//
// Every section besides "Keys" gets its header coloured but its body left
// exactly as helpText wrote it: those sections use a column-aligned
// continuation convention (padding to the prose column, not "Keys"'s flat
// 3-space indent) and none of their leading labels (field names, env var
// names, mouse actions) are themselves keycaps, so the
// keycap/text/continuation treatment does not generalise to them cleanly.
// This is a deliberate, documented scope choice (docs/reports/phase3.md's
// Steer 005 item 2 subsection), not an oversight.
func (m Model) styledHelpText() string {
	lines := strings.Split(helpText(m.settings.ASCII), "\n")

	inKeys := false
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		switch {
		case helpStyleHeaders[l]:
			out = append(out, m.colorToken(theme.Title, l))
			inKeys = l == "Keys"
		case l == "":
			inKeys = false
			out = append(out, l)
		case !inKeys:
			out = append(out, l)
		case strings.HasPrefix(l, "   "):
			// Continuation line: preserve its exact indentation (see the
			// helpText excerpt in help_style_test.go -- always 4 spaces
			// today) by re-adding the 2 spaces TrimPrefix below removes,
			// colouring only the visible remainder.
			out = append(out, "  "+m.colorToken(theme.Dimmed, strings.TrimPrefix(l, "  ")))
		case strings.HasPrefix(l, "  "):
			body := strings.TrimPrefix(l, "  ")
			phrase, rest := splitLeadingKeyPhrase(body)
			if phrase == "" {
				out = append(out, l)
				continue
			}
			out = append(out, "  "+m.colorToken(theme.Key, phrase)+m.colorToken(theme.Text, rest))
		default:
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
