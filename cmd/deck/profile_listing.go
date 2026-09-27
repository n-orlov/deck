package main

import (
	"fmt"
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
// flagged "(invalid name: not selectable)" rather than skipped.
// config.ListProfiles is a directory scan only -- it never opens a
// state.db -- so this always exits 0, even when every profile's own
// database is missing, corrupt or unreadable.
func runProfilesListing(getenv func(string) string, userHome func() (string, error), stdout io.Writer) int {
	listings, err := config.ListProfiles(getenv, userHome)
	if err != nil {
		fmt.Fprintln(stdout, "deck profiles:", err)
		return 0
	}
	for _, listing := range listings {
		if !listing.Valid {
			fmt.Fprintf(stdout, "%s (invalid name: not selectable)\n", listing.Name)
			continue
		}
		lastUsed := "never"
		if !listing.LastUsed.IsZero() {
			lastUsed = listing.LastUsed.Format(time.RFC3339)
		}
		fmt.Fprintf(stdout, "%s\tsocket: %s\tconfig: %s\tdata: %s\tlast used: %s\n",
			listing.Name, listing.Socket, listing.ConfigFile, listing.DataDir, lastUsed)
	}
	return 0
}
