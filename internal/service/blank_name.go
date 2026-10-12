package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// BlankNameLayout is the timestamp layout of the blank-name default
// (`<cwd-basename>-<MMDD-HHMM>`, PRD "Auto", R248).
const BlankNameLayout = "0102-1504"

// DerivedSessionName is the one blank-name rule every entry point shares (the
// create modal and `deck new`): `<basename of cwd>-<MMDD-HHMM>` from now, then
// the first free `-2`, `-3`, ... suffix against the names taken. The caller
// records the result with NameSource "derived".
func DerivedSessionName(cwd string, now time.Time, taken map[string]bool) string {
	base := cwdNameBasename(cwd) + "-" + now.Format(BlankNameLayout)
	if !taken[base] {
		return base
	}
	for suffix := 2; suffix < 100000; suffix++ {
		candidate := fmt.Sprintf("%s-%d", base, suffix)
		if !taken[candidate] {
			return candidate
		}
	}
	// Unreachable outside a pathological setup; the store's UNIQUE constraint
	// then reports the failure.
	return base
}

// cwdNameBasename is the last element of cwd; a cwd that reduces to nothing is
// returned as it is.
func cwdNameBasename(cwd string) string {
	base := filepath.Base(strings.TrimRight(cwd, "/"))
	if base == "" {
		return cwd
	}
	return base
}

// ResolveBlankName returns the name a create with the given typed name uses and
// its provenance: a non-blank name verbatim as "user", a blank one derived
// (DerivedSessionName) against the store's current names as "derived".
func (s Service) ResolveBlankName(ctx context.Context, typed, cwd string) (name, nameSource string, err error) {
	if trimmed := strings.TrimSpace(typed); trimmed != "" {
		return trimmed, "user", nil
	}
	if s.Store == nil || s.Clock == nil {
		return "", "", errors.New("deriving a session name requires a store and a clock")
	}
	sessions, err := s.Store.ListSessions(ctx)
	if err != nil {
		return "", "", fmt.Errorf("list sessions to derive a name: %w", err)
	}
	taken := make(map[string]bool, len(sessions))
	for _, session := range sessions {
		taken[session.Name] = true
	}
	return DerivedSessionName(cwd, s.Clock.Now(), taken), "derived", nil
}
