package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/n-orlov/deck/internal/config"
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
	uo := updateCommandOptions(opts.version)
	if opts.check {
		return reportUpdateCheck(ctx, uo, stdout, stderr)
	}
	res, err := update.Run(ctx, uo)
	if err != nil {
		sayln(stderr, "deck update:", update.RedactText(update.StripControl(err.Error())))
		return 1
	}
	reportUpdateInstall(res, buildVersion(), stdout)
	return 0
}

// reportUpdateInstall prints what `deck update` did: nothing changed, or the
// version moved and the notes that go with the install.
func reportUpdateInstall(res update.Result, old string, stdout io.Writer) {
	if res.Unchanged {
		sayf(stdout, "deck %s is already the latest release (%s); nothing changed\n", old, res.Tag)
		return
	}
	sayf(stdout, "deck updated: %s -> %s (%s)\n", old, res.Tag, res.Path)
	for _, n := range updateNotes(res) {
		sayln(stdout, "note: "+n)
	}
}

// updateNotes are the notes printed after an install: what was not verified,
// then the restart reminder. Each is deck's own wording, chosen from a
// Result state; no server text reaches them.
func updateNotes(res update.Result) []string {
	var notes []string
	if res.Attestation == update.AttestationSkipped {
		notes = append(notes, "only the checksum was verified; the build attestation could not be checked")
	}
	switch res.Immutable {
	case update.ImmutableVerified:
		notes = append(notes, "the release is a GitHub immutable release")
	case update.ImmutableNot:
		notes = append(notes, "the release is not verified immutable: GitHub reports it is not an immutable release (installed because "+update.EnvRequireImmutable+"=0 is set)")
	case update.ImmutableUnknown:
		notes = append(notes, "the release is not verified immutable: its immutability could not be checked (installed because "+update.EnvRequireImmutable+"=0 is set)")
	}
	return append(notes, "restart any running deck to use the new version; running sessions are not disturbed")
}

// reportUpdateCheck is `deck update --check`: one request, nothing written.
func reportUpdateCheck(ctx context.Context, uo update.Options, stdout, stderr io.Writer) int {
	res, err := update.Check(ctx, uo)
	if err != nil {
		sayln(stderr, "deck update:", update.RedactText(update.StripControl(err.Error())))
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

// commandInstallLockPath is the install lock `deck update` takes: beside the
// shared update cache, which does not depend on the config file. No resolvable
// cache means no lock, like an uncreatable lock file.
func commandInstallLockPath() string {
	cache, err := config.UpdateCachePath(os.Getenv, os.UserHomeDir)
	if err != nil {
		return ""
	}
	return update.InstallLockPath(cache)
}

// updateCommandOptions is what `deck update` hands update.Run and update.Check.
func updateCommandOptions(pinned string) update.Options {
	return update.Options{Current: version, Version: pinned, OnlyIfNewer: true, InstallLock: commandInstallLockPath()}
}
