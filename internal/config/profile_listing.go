package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// lastUsedMarkerName is SPEC §3.4's "last_used" marker file name inside a
// profile's own data root -- deck --profiles reads only its mtime
// (ListProfiles below), and TouchLastUsed is the only writer.
const lastUsedMarkerName = "last_used"

// ProfileListing is one row of `deck --profiles`' report (SPEC §3.4): a
// directory found under the data root's profiles/, or the always-known
// default, described purely by directory metadata -- no state.db is ever
// opened, so a missing, corrupt or permission-denied database can never
// change this listing or its exit code.
type ProfileListing struct {
	// Name is the profile's own directory name ("default" for the flat
	// layout). Always set, valid or not.
	Name string
	// Valid is false when Name failed ValidateProfileName -- SPEC's
	// "(invalid name: not selectable)" row. Such a row still carries
	// every field below, describing the directory as it sits on disk
	// (profiles/<name>/ literally, and the deck-<name> socket its name
	// would derive), so the listing keeps one shape for every row.
	Valid      bool
	Socket     string
	ConfigFile string
	DataDir    string
	// LastUsed is the last_used marker's mtime, zero when the marker does
	// not exist yet (the profile has never launched since it was made).
	LastUsed time.Time
}

// ListProfiles lists every profile for `deck --profiles` (SPEC §3.4):
// default first, then every directory under the data root's profiles/,
// sorted by name. It is a directory scan only: it never opens a state.db,
// so a profile whose database is missing, corrupt or unreadable (even
// mode 0000) is still listed unaffected. A directory whose name fails
// ValidateProfileName is still listed -- flagged, never skipped and never
// an error -- exactly as SPEC's own "(invalid name: not selectable)" row
// requires.
func ListProfiles(getenv func(string) string, userHome func() (string, error)) ([]ProfileListing, error) {
	defaultPaths, err := resolvePaths(getenv, userHome, DefaultProfile)
	if err != nil {
		return nil, err
	}
	listings := []ProfileListing{newProfileListing(DefaultProfile, socketForProfile(DefaultProfile), defaultPaths)}

	root, err := ProfilesRoot(getenv, userHome)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return listings, nil
		}
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		// resolvePaths only joins name beneath the root, never validates
		// it; a ReadDir entry name holds no separator and is never "."
		// or "..", so an invalid name still resolves to its own literal
		// profiles/<name>/ directories and nothing outside them.
		paths, err := resolvePaths(getenv, userHome, name)
		if err != nil {
			return nil, err
		}
		listing := newProfileListing(name, socketForProfile(name), paths)
		if err := ValidateProfileName(name); err != nil {
			listing.Valid = false
		}
		listings = append(listings, listing)
	}
	return listings, nil
}

// socketForProfile mirrors LoadFromProfile's own derivation (minus the
// DECK_TMUX_SOCKET override, which is a single running launch's knob, not
// a property of a profile the scan merely lists).
func socketForProfile(name string) string {
	if name == DefaultProfile {
		return DefaultSocket
	}
	return "deck-" + name
}

func newProfileListing(name, socket string, paths Paths) ProfileListing {
	listing := ProfileListing{Name: name, Valid: true, Socket: socket, ConfigFile: paths.ConfigFile, DataDir: paths.DataDir}
	if info, err := os.Stat(lastUsedMarkerPath(paths)); err == nil {
		listing.LastUsed = info.ModTime()
	}
	return listing
}

func lastUsedMarkerPath(paths Paths) string {
	return filepath.Join(paths.DataDir, lastUsedMarkerName)
}

// TouchLastUsed touches SPEC §3.4's last_used marker in paths' own data
// root -- creating it if absent, updating its mtime to now otherwise --
// so `deck --profiles`' "last used" column (ListProfiles above) reflects
// this launch. Called once per TUI launch (never for _hook, and never for
// --profiles itself), for every profile including default. A failure here
// (e.g. an unwritable data root) is reported by the caller as
// best-effort, exactly like the tombstone sweep beside its call site in
// cmd/deck -- it must never stop deck from starting.
func TouchLastUsed(paths Paths) error {
	if err := os.MkdirAll(paths.DataDir, 0o700); err != nil {
		return fmt.Errorf("create profile data directory: %w", err)
	}
	path := lastUsedMarkerPath(paths)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // G304: the marker path is built from the resolved profile data directory, never from input
	if err != nil {
		return fmt.Errorf("touch last_used marker: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("touch last_used marker: %w", err)
	}
	now := time.Now()
	if err := os.Chtimes(path, now, now); err != nil {
		return fmt.Errorf("touch last_used marker: %w", err)
	}
	return nil
}
