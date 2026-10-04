package main

import (
	"io"
	"time"

	"github.com/n-orlov/deck/internal/config"
)

// isProfilesRequest reports whether args asks for SPEC §3.4's `deck
// --profiles` listing and nothing else -- answered the same way
// isVersionRequest is, before any positional-argument parsing, so
// "--profiles" is never folded into parseProfileArgs' "unknown flag"
// rejection or mistaken for a profile name.
func isProfilesRequest(args []string) bool {
	return len(args) == 2 && args[1] == "--profiles"
}

// runProfilesListing implements `deck --profiles` (SPEC §3.4): one line
// per profile, default first, naming its socket, config and data paths
// and its last-used time; a directory whose name fails validation is
// flagged "(invalid name: not selectable)" rather than skipped, on a row
// of the same shape: the flag follows the name, and the socket, config,
// data and last-used fields follow the flag.
// config.ListProfiles is a directory scan only -- it never opens a
// state.db -- so this always exits 0, even when every profile's own
// database is missing, corrupt or unreadable.
func runProfilesListing(getenv func(string) string, userHome func() (string, error), stdout io.Writer) int {
	listings, err := config.ListProfiles(getenv, userHome)
	if err != nil {
		sayln(stdout, "deck profiles:", err)
		return 0
	}
	for _, listing := range listings {
		name := listing.Name
		if !listing.Valid {
			name += " (invalid name: not selectable)"
		}
		lastUsed := "never"
		if !listing.LastUsed.IsZero() {
			lastUsed = listing.LastUsed.Format(time.RFC3339)
		}
		sayf(stdout, "%s\tsocket: %s\tconfig: %s\tdata: %s\tlast used: %s\n",
			name, listing.Socket, listing.ConfigFile, listing.DataDir, lastUsed)
	}
	return 0
}
