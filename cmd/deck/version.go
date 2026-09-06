package main

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
)

// version is the release tag baked in at build time by the release workflow
// (.github/workflows/release.yml) via -ldflags "-X main.version=vX.Y.Z". A
// plain `go build` leaves it "dev", and printVersion then falls back to the
// module's own vcs revision when the binary carries one.
var version = "dev"

// isVersionRequest reports whether args asks for the version and nothing
// else. It is answered before configuration is loaded so the flag works on
// a workstation with no tmux, no config and no state directory.
func isVersionRequest(args []string) bool {
	if len(args) != 2 {
		return false
	}
	switch args[1] {
	case "--version", "-version", "-v", "version":
		return true
	}
	return false
}

// printVersion writes "deck <version> <os>/<arch>" to w, e.g.
// "deck v0.1.0 linux/amd64". A "dev" build adds the short vcs revision when
// the Go toolchain stamped one (go build from a git checkout does).
func printVersion(w io.Writer) {
	v := version
	if v == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" && len(s.Value) >= 12 {
					v = "dev-" + s.Value[:12]
					break
				}
			}
		}
	}
	fmt.Fprintf(w, "deck %s %s/%s\n", v, runtime.GOOS, runtime.GOARCH)
}
