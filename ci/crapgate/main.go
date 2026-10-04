// Package main implements ci/crapgate, R187's stdlib-only CRAP scorer.
//
//	go run ./ci/crapgate -profile <coverprofile> -max <ceiling> [-filter <pkg-dir>[:<file>[,<file>...]]]
//
// It imports only the Go standard library (go/ast, go/parser, go/token,
// bufio, flag, fmt, os, path/filepath, sort, strconv, strings) -- no
// golang.org/x/tools/cover, no third-party module of any kind -- so it
// never needs `go mod download` to run inside the gate.
//
//   - -profile is the path to a `go test -coverprofile` (or merged
//     `go tool covdata textfmt`) coverage profile. Required; a missing or
//     empty profile is a usage error (exit 2), not zero findings.
//   - -max is the CRAP ceiling: any scored function whose CRAP exceeds it
//     is reported as an offender. Required.
//   - -filter optionally restricts which source is scanned, in one of two
//     forms: a bare package directory ("internal/tui/", scans every
//     non-test .go file directly inside it), or "<pkg-dir>:<file,...>"
//     ("internal/tui/:tui.go,model.go", scans only the named files). With
//     no -filter, the whole repository (rooted at the profile's own
//     module, found by walking up for go.mod) is scanned.
//
// Exit codes: 0 nothing over the ceiling; 1 one or more functions scored
// over the ceiling; 2 a usage/input error (missing/empty profile, or a
// scan that found zero scored functions -- an empty filter typo must
// never read as "nothing to refactor").
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	profile := flag.String("profile", "", "path to a go coverprofile (required)")
	ceiling := flag.Float64("max", 0, "CRAP ceiling; any function scoring over this fails (required)")
	filter := flag.String("filter", "", "optional <pkg-dir> or <pkg-dir>:<file,...> to restrict scanning")
	flag.Parse()

	report, exitCode, err := run(*profile, *ceiling, *filter)
	if err != nil {
		fmt.Fprintln(os.Stderr, "crapgate:", err)
		os.Exit(exitCode)
	}
	fmt.Print(report)
	os.Exit(exitCode)
}

// run is main's testable core: it never touches flag/os.Exit directly, so
// every exit-code path below is exercised by ordinary table tests instead
// of a subprocess.
func run(profilePath string, ceiling float64, filter string) (report string, exitCode int, err error) {
	if profilePath == "" {
		return "", 2, fmt.Errorf("-profile is required")
	}

	profile, err := parseProfile(profilePath)
	if err != nil {
		return "", 2, err
	}

	moduleRoot, modulePath, err := findModule()
	if err != nil {
		return "", 2, err
	}

	root, err := filterRoot(filter)
	if err != nil {
		return "", 2, err
	}

	fns, err := scanFunctions(moduleRoot, filter)
	if err != nil {
		return "", 2, err
	}
	if len(fns) == 0 {
		return "", 2, fmt.Errorf("zero scored functions found under %q (filter %q) -- check the path, not a clean bill of health", root, filter)
	}

	scores := scoreFunctions(fns, modulePath, moduleRoot, profile)
	report, exitCode = formatReport(scores, ceiling)
	return report, exitCode, nil
}
