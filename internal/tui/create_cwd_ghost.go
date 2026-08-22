package tui

import (
	"os"
	"path/filepath"
	"sort"
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
	m.closeCreateCWDCandidates()
}

// createCWDCommonPrefix returns the longest prefix shared by every one of
// names (bash's own tab-completion contract, task 012): "" when names is
// empty, and the whole name unchanged when there is exactly one. Every
// name createCWDMatches ever returns already shares raw's typed segment
// as a common prefix, so the result is always at least that long -- it
// can only ever grow what is already on screen, never shrink it.
func createCWDCommonPrefix(names []string) string {
	if len(names) == 0 {
		return ""
	}
	prefix := names[0]
	for _, name := range names[1:] {
		n := len(prefix)
		if len(name) < n {
			n = len(name)
		}
		i := 0
		for i < n && prefix[i] == name[i] {
			i++
		}
		prefix = prefix[:i]
		if prefix == "" {
			return ""
		}
	}
	return prefix
}

// tabCompleteCreateCWD implements task 012's §11.7 requirement 16: "tab
// completes to the longest common prefix when that advances the text, and
// otherwise lists the candidates for selection" -- bash's own completion
// contract, applied to the cwd field's CURRENT raw text (m.createCWD).
//
// It reports handled=false, doing nothing at all, in the two cases where
// there is genuinely nothing to complete or list: the directory portion
// cannot be scanned or no entry's name starts with the segment (zero
// candidates), or the segment already spells out in full the one
// candidate there is (one candidate, nothing left to advance to and
// nothing to list) -- letting tab fall back to its ordinary "move to the
// next field" meaning in exactly those two cases, so a caller that tabs
// through an untouched, already-real cwd value onto the next field is
// unaffected by this task.
//
// Otherwise it reports handled=true: either the common prefix among the
// candidates is longer than the segment already typed, in which case the
// missing suffix is appended (plus a trailing "/" when that prefix is
// itself the one candidate's full, unique name -- the same unique-match
// case createCWDGhostCompletion's right/end already complete to); or the
// prefix cannot advance any further and at least two candidates remain,
// in which case createCWDCandidates is populated (sorted) for the user to
// pick from with up/down and enter (task 012's other new per-field keys).
func (m *Model) tabCompleteCreateCWD() bool {
	names, ok := createCWDMatches(m.createCWD)
	if !ok || len(names) == 0 {
		return false
	}
	_, segment := splitCWDSegment(m.createCWD)
	prefix := createCWDCommonPrefix(names)
	if len(prefix) > len(segment) {
		m.createCWD += prefix[len(segment):]
		if len(names) == 1 {
			m.createCWD += "/"
		}
		m.createCWDPrefilled = false
		m.createCWDLastUsed = false
		m.createCWDRecentIndex = -1
		m.closeCreateCWDCandidates()
		return true
	}
	if len(names) < 2 {
		return false
	}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	m.createCWDCandidates = sorted
	m.createCWDCandidateIndex = 0
	return true
}

// acceptCWDCandidate puts name -- one entry of m.createCWDCandidates,
// task 012's tab-completion listing branch -- into the field in place of
// the segment being completed, plus a trailing "/" (every candidate is a
// directory, never a file, exactly like every other completion path in
// this file), and closes the list. It is an edit like any other: it
// clears the §11.7 prefill/last-used labels and ends any recent_cwds
// up/down cycle in progress (task 009), same reasoning as
// acceptCWDGhost's.
func (m *Model) acceptCWDCandidate(name string) {
	dir, _ := splitCWDSegment(m.createCWD)
	m.createCWD = dir + name + "/"
	m.createCWDPrefilled = false
	m.createCWDLastUsed = false
	m.createCWDRecentIndex = -1
	m.closeCreateCWDCandidates()
}

// closeCreateCWDCandidates closes task 012's tab-completion candidate
// list without changing the field, safe to call whenever the list may or
// may not be open (nil slice, index 0 is simply the closed state).
func (m *Model) closeCreateCWDCandidates() {
	m.createCWDCandidates = nil
	m.createCWDCandidateIndex = 0
}
