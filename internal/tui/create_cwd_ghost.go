package tui

import (
	"os"
	"path/filepath"
	"strings"
)

// createCWDGhostCompletion computes §11.7's directory-only ghost completion
// (task 010) for the cwd field's CURRENT raw text. There is no other
// cursor position in this field (typing only appends, backspace only
// trims the end), so "the cursor is at end of field" always holds and the
// segment being completed is always whatever follows the last '/' in raw.
//
// It returns the missing suffix of a UNIQUE matching directory's name plus
// a trailing '/' -- exactly what accepting the ghost appends to raw -- and
// ok=false whenever there is nothing to show: no directory can be listed
// at all (does not exist, a permission error, or a bare "~otheruser" this
// package does not resolve), no entry's name starts with the segment, or
// more than one does (task 011 owns the ambiguous case, not this one).
//
// Only directories are ever candidates: a same-named FILE never blocks or
// substitutes for a directory match, and never counts toward "more than
// one candidate" either, however uniquely its name matches. A hidden
// directory (name starting with '.') is a candidate only when the segment
// itself starts with '.', exactly as a shell glob excludes dotfiles unless
// the pattern already starts with a dot -- an empty segment matching a
// hidden directory in every OTHER language would otherwise suggest one the
// user never asked to see. A leading '~' in the directory portion of raw is
// expanded (expandCWDDirForScan) so the real filesystem can be scanned,
// but the ghost returned is always just raw's missing suffix -- raw's own
// leading "~" is never rewritten in what gets displayed or appended.
func createCWDGhostCompletion(raw string) (ghost string, ok bool) {
	names, ok := createCWDMatches(raw)
	if !ok || len(names) != 1 {
		return "", false
	}
	_, segment := splitCWDSegment(raw)
	return names[0][len(segment):] + "/", true
}

// createCWDAmbiguousMatchCount reports the number of directory candidates
// for raw's segment (task 011, §11.7 requirement 15) whenever there are two
// or more -- the case createCWDGhostCompletion deliberately ghosts nothing
// for, since ghosting one arbitrary candidate (e.g. the alphabetically
// first) would make right/end a coin flip that silently sends the session
// to the wrong directory. ok=false whenever there is no ambiguity to
// report at all: zero or one match (createCWDGhostCompletion owns those),
// or the directory portion cannot be listed (does not exist, a permission
// error, an unresolvable "~otheruser").
func createCWDAmbiguousMatchCount(raw string) (count int, ok bool) {
	names, ok := createCWDMatches(raw)
	if !ok || len(names) < 2 {
		return 0, false
	}
	return len(names), true
}

// createCWDMatches lists every directory-only candidate (never a file, see
// createCWDGhostCompletion's doc comment for the hidden-directory and "~"
// rules, both enforced identically here) whose name starts with raw's
// segment being completed, shared by both createCWDGhostCompletion's
// unique-match case and createCWDAmbiguousMatchCount's several-matches
// case so the two agree on exactly what counts as a candidate. ok=false
// only when the directory portion itself cannot be listed at all; an empty,
// non-nil-ok result (no name starts with the segment) is a legitimate
// zero-candidate answer, not an error.
func createCWDMatches(raw string) (names []string, ok bool) {
	dir, segment := splitCWDSegment(raw)
	scanDir, ok := expandCWDDirForScan(dir)
	if !ok {
		return nil, false
	}
	entries, err := os.ReadDir(scanDir)
	if err != nil {
		return nil, false
	}
	dotSegment := strings.HasPrefix(segment, ".")
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !dotSegment && strings.HasPrefix(name, ".") {
			continue
		}
		if !strings.HasPrefix(name, segment) {
			continue
		}
		names = append(names, name)
	}
	return names, true
}

// splitCWDSegment splits raw at its last '/' into the directory portion
// (including the trailing '/', or "" when raw has no '/' at all) and the
// segment being completed (everything after it, possibly "").
func splitCWDSegment(raw string) (dir, segment string) {
	idx := strings.LastIndex(raw, "/")
	if idx < 0 {
		return "", raw
	}
	return raw[:idx+1], raw[idx+1:]
}

// expandCWDDirForScan resolves dir (splitCWDSegment's directory portion,
// always either "" or ending in '/') to a real, listable path: "" (no '/'
// typed yet) scans the process's own working directory, a leading "~/" or
// exactly "~" expands to the user's home exactly as expandCreateCWD does,
// and anything else is returned unchanged for os.ReadDir to resolve
// (absolute, or relative to the process's own cwd). A bare "~otheruser/"
// this package cannot resolve reports ok=false rather than guessing one.
func expandCWDDirForScan(dir string) (string, bool) {
	if dir == "" {
		return ".", true
	}
	if !strings.HasPrefix(dir, "~") {
		return dir, true
	}
	trimmed := strings.TrimSuffix(dir, "/")
	if trimmed != "~" && !strings.HasPrefix(trimmed, "~/") {
		return "", false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	if trimmed == "~" {
		return home, true
	}
	return filepath.Join(home, strings.TrimPrefix(trimmed, "~/")), true
}

// acceptCWDGhost appends the cwd field's current ghost completion (if any)
// to m.createCWD, exactly as right-arrow/end are documented to do (task
// 010). It is a no-op whenever there is nothing to accept, so binding it
// unconditionally on both keys never corrupts the field. Accepting is an
// edit like typing a rune: it clears the §11.7 prefill/last-used labels
// and ends any recent_cwds up/down cycle in progress (task 009), so a
// later "down" cannot resurrect a stale pre-cycle snapshot out from under
// the completed path.
func (m *Model) acceptCWDGhost() {
	ghost, ok := createCWDGhostCompletion(m.createCWD)
	if !ok {
		return
	}
	m.createCWD += ghost
	m.createCWDPrefilled = false
	m.createCWDLastUsed = false
	m.createCWDRecentIndex = -1
}
