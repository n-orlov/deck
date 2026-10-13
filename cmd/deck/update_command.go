package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/n-orlov/deck/internal/update"
)

// isUpdateRequest reports whether args is the `deck update` verb. The word
// `update` always selects the verb, whatever follows it: a profile named
// `update` is reached with DECK_PROFILE=update (SPEC §3.4). The verb is
// answered before the config and tmux checks, like --version, so it works on
// a machine with neither.
func isUpdateRequest(args []string) bool {
	return len(args) >= 2 && args[1] == "update"
}

// updateArgs are the parsed options of `deck update`.
type updateArgs struct {
	check   bool
	version string
}

// parseUpdateArgs parses the arguments after `deck update`: --check and
// --version vX.Y.Z (or --version=vX.Y.Z). Anything else is refused.
func parseUpdateArgs(rest []string, stderr io.Writer) (updateArgs, bool) {
	var out updateArgs
	for i := 0; i < len(rest); i++ {
		next, ok := out.parseOne(rest, i, stderr)
		if !ok {
			return out, false
		}
		i = next
	}
	if out.check && out.version != "" {
		sayln(stderr, "error: --check and --version cannot be combined")
		return out, false
	}
	return out, true
}

// parseOne reads rest[i] (and the value after it for a bare --version) into a,
// returning the index of the last argument it consumed.
func (a *updateArgs) parseOne(rest []string, i int, stderr io.Writer) (int, bool) {
	arg := rest[i]
	flag, value, hasValue := strings.Cut(arg, "=")
	switch {
	case arg == "--check":
		a.check = true
	case flag == "--version" && hasValue:
		a.version = value
	case flag == "--version" && i+1 < len(rest):
		i++
		a.version = rest[i]
	case flag == "--version":
		sayln(stderr, "error: --version needs a value (a release tag such as v1.2.3)")
		return i, false
	case strings.HasPrefix(arg, "-"):
		sayf(stderr, "error: unknown flag %s\n", arg)
		return i, false
	default:
		sayf(stderr, "error: deck update takes options only, got %q\n", arg)
		return i, false
	}
	return i, true
}

// runUpdateCommand is `deck update [--check] [--version vX.Y.Z]`. Exit 2 is a
// usage error, 1 any update failure or refusal (with the cause on stderr and
// the installed binary untouched), 0 success.
func runUpdateCommand(args []string, stdout, stderr io.Writer) int {
	opts, ok := parseUpdateArgs(args[2:], stderr)
	if !ok {
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	uo := update.Options{Current: version, Version: opts.version, OnlyIfNewer: true}
	if opts.check {
		return reportUpdateCheck(ctx, uo, stdout, stderr)
	}
	res, err := update.Run(ctx, uo)
	if err != nil {
		sayln(stderr, "deck update:", err)
		return 1
	}
	old := buildVersion()
	if res.Unchanged {
		sayf(stdout, "deck %s is already the latest release (%s); nothing changed\n", old, res.Tag)
		return 0
	}
	sayf(stdout, "deck updated: %s -> %s (%s)\n", old, res.Tag, res.Path)
	if res.Attestation == update.AttestationSkipped {
		sayln(stdout, "note: only the checksum was verified; the build attestation could not be checked")
	}
	sayln(stdout, "note: restart any running deck to use the new version; running sessions are not disturbed")
	return 0
}

// reportUpdateCheck is `deck update --check`: one request, nothing written.
func reportUpdateCheck(ctx context.Context, uo update.Options, stdout, stderr io.Writer) int {
	res, err := update.Check(ctx, uo)
	if err != nil {
		sayln(stderr, "deck update:", err)
		return 1
	}
	cur := buildVersion()
	switch {
	case uo.Current == update.DevVersion:
		sayf(stdout, "deck %s is a dev build and cannot be updated in place; the latest release is %s\n", cur, res.Latest)
	case res.Newer:
		sayf(stdout, "update available: %s -> %s (run `deck update` to install it)\n", cur, res.Latest)
	default:
		sayf(stdout, "deck %s is up to date (latest release: %s)\n", cur, res.Latest)
	}
	return 0
}
