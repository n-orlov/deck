// Package main implements ci/covgate, R189's coverage gate behind
// ci/quality.sh.
//
//	go run ./ci/covgate -config ci/quality.json -profile <merged coverprofile>
//
// It is stdlib-only. It reads the thresholds from the "coverage" object of
// ci/quality.json (the one place thresholds live) and scores the merged
// profile statement by statement:
//
//   - the product total (every package except cmd/fake-*) must reach
//     total_floor;
//   - every product package must reach package_floor (the ci/* Go tools
//     count as product);
//   - every cmd/fake-* fixture package must reach fixture_floor;
//   - a package with no statements in the profile fails unless it is named
//     by exact path in zeroStatementPackages below, and a name in that list
//     that does have statements (or no longer exists) fails too, so the
//     list can never turn into a hiding place;
//   - a package, or the total, that beats its floor by tightenMargin
//     percentage points or more is named in a "tighten" prompt.
//
// Exit codes: 0 every floor met, 1 at least one floor missed, 2 a
// usage/input error (bad config, empty or unreadable profile).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

// zeroStatementPackages is the explicit list, by exact path relative to the
// module root, of packages that have no executable statement and so can
// never appear in a coverage profile. It is not a glob and not an
// allow-list of weak packages: run fails when a listed package does have
// statements in the profile (the entry is stale) or no longer exists.
var zeroStatementPackages = []string{
	// internal/notify, internal/search and internal/unit are reserved
	// package names that so far hold only a doc.go (a package clause and
	// a comment), so no statement exists to measure.
	"internal/notify",
	"internal/search",
	"internal/unit",
	// internal/racebuild holds only the constant Enabled, split by the
	// race build tag into two files; a constant declaration is not a
	// statement, so the package has nothing for the profile to count.
	"internal/racebuild",
}

// fixturePrefix marks the test-fixture binaries R187 gives their own floor.
const fixturePrefix = "cmd/fake-"

// tightenMargin is how many percentage points a package (or the total)
// must beat its floor by before the gate prompts to raise the floor.
const tightenMargin = 1.0

// coverageConfig is the "coverage" object of ci/quality.json.
type coverageConfig struct {
	TotalFloor   float64 `json:"total_floor"`
	PackageFloor float64 `json:"package_floor"`
	FixtureFloor float64 `json:"fixture_floor"`
}

func main() {
	configPath := flag.String("config", "", "path to ci/quality.json (required)")
	profilePath := flag.String("profile", "", "path to the merged coverprofile (required)")
	flag.Parse()

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "covgate:", err)
		os.Exit(2)
	}
	report, code, err := run(*configPath, *profilePath, wd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "covgate:", err)
		os.Exit(code)
	}
	fmt.Print(report)
	os.Exit(code)
}

// loadConfig reads the thresholds and rejects a floor that is not in
// (0, 100]: a zero floor would pass anything.
func loadConfig(path string) (coverageConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: the config path is the tool's own command-line argument
	if err != nil {
		return coverageConfig{}, fmt.Errorf("reading %s: %w", path, err)
	}
	var wrapper struct {
		Coverage *coverageConfig `json:"coverage"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return coverageConfig{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	if wrapper.Coverage == nil {
		return coverageConfig{}, fmt.Errorf("%s has no \"coverage\" object", path)
	}
	cfg := *wrapper.Coverage
	for name, v := range map[string]float64{
		"total_floor": cfg.TotalFloor, "package_floor": cfg.PackageFloor, "fixture_floor": cfg.FixtureFloor,
	} {
		if v <= 0 || v > 100 {
			return coverageConfig{}, fmt.Errorf("%s: coverage.%s = %v, want a floor in (0, 100]", path, name, v)
		}
	}
	return cfg, nil
}

// pkgScore is one package's statement tally.
type pkgScore struct {
	path           string
	stmts, covered int
}

func (s pkgScore) pct() float64 {
	if s.stmts == 0 {
		return 0
	}
	return 100 * float64(s.covered) / float64(s.stmts)
}

func isFixture(pkg string) bool { return strings.HasPrefix(pkg, fixturePrefix) }

// run is main's testable core. moduleDir is any directory inside the module.
func run(configPath, profilePath, moduleDir string) (string, int, error) {
	if configPath == "" {
		return "", 2, fmt.Errorf("-config is required")
	}
	if profilePath == "" {
		return "", 2, fmt.Errorf("-profile is required")
	}
	cfg, err := loadConfig(configPath)
	if err != nil {
		return "", 2, err
	}
	prof, err := parseProfile(profilePath)
	if err != nil {
		return "", 2, err
	}
	root, modulePath, err := findModule(moduleDir)
	if err != nil {
		return "", 2, err
	}
	onDisk, err := sourcePackages(root)
	if err != nil {
		return "", 2, err
	}

	byPkg := make(map[string]*pkgScore)
	score := func(pkg string) *pkgScore {
		if byPkg[pkg] == nil {
			byPkg[pkg] = &pkgScore{path: pkg}
		}
		return byPkg[pkg]
	}
	for key, stat := range prof.blocks {
		pkg, perr := packageOf(key.file, modulePath)
		if perr != nil {
			return "", 2, perr
		}
		s := score(pkg)
		s.stmts += stat.numStmt
		if stat.covered {
			s.covered += stat.numStmt
		}
	}
	for pkg := range onDisk {
		score(pkg)
	}

	var problems []string
	zero := make(map[string]bool, len(zeroStatementPackages))
	for _, z := range zeroStatementPackages {
		zero[z] = true
		switch s := byPkg[z]; {
		case !onDisk[z]:
			problems = append(problems, fmt.Sprintf("%s: named in the zero-statement list but has no Go source on disk (remove the stale entry)", z))
		case s.stmts > 0:
			problems = append(problems, fmt.Sprintf("%s: named in the zero-statement list but the profile has %d statements for it (remove the entry; the package is measured)", z, s.stmts))
		}
	}

	names := make([]string, 0, len(byPkg))
	for pkg := range byPkg {
		names = append(names, pkg)
	}
	sort.Strings(names)

	var b strings.Builder
	var tighten []string
	var product pkgScore
	product.path = "product total"
	fmt.Fprintf(&b, "%-34s %8s %6s  %s\n", "package", "cover", "floor", "statements")
	for _, pkg := range names {
		s := byPkg[pkg]
		if s.stmts == 0 {
			if zero[pkg] {
				fmt.Fprintf(&b, "%-34s %8s %6s  0 (named in the zero-statement list)\n", pkg, "n/a", "n/a")
				continue
			}
			problems = append(problems, fmt.Sprintf("%s: zero statements in the profile and not named in the zero-statement list (the package was never built under the profile: run it under the suite, or, if it truly has no statements, name it by exact path in ci/covgate's zeroStatementPackages)", pkg))
			fmt.Fprintf(&b, "%-34s %8s %6s  0 MISSING\n", pkg, "-", "-")
			continue
		}
		floor, kind := cfg.PackageFloor, "package"
		if isFixture(pkg) {
			floor, kind = cfg.FixtureFloor, "fixture"
		} else {
			product.stmts += s.stmts
			product.covered += s.covered
		}
		status := "ok"
		if s.pct() < floor {
			status = "FAIL"
			problems = append(problems, fmt.Sprintf("%s: %.1f%% is below the %s floor of %v%% (%d of %d statements covered; add behaviour tests for its uncovered paths)", pkg, s.pct(), kind, floor, s.covered, s.stmts))
		} else if s.pct()-floor >= tightenMargin {
			tighten = append(tighten, fmt.Sprintf("%s is at %.1f%%, %.1f pp above its %s floor of %v%%", pkg, s.pct(), s.pct()-floor, kind, floor))
		}
		fmt.Fprintf(&b, "%-34s %7.1f%% %5v%%  %d %s\n", pkg, s.pct(), floor, s.stmts, status)
	}

	if product.stmts == 0 {
		return "", 2, fmt.Errorf("the profile holds no product statements (only cmd/fake-* fixtures, or nothing)")
	}
	status := "ok"
	if product.pct() < cfg.TotalFloor {
		status = "FAIL"
		problems = append(problems, fmt.Sprintf("product total: %.1f%% is below the total floor of %v%% (%d of %d statements covered)", product.pct(), cfg.TotalFloor, product.covered, product.stmts))
	} else if product.pct()-cfg.TotalFloor >= tightenMargin {
		tighten = append(tighten, fmt.Sprintf("the product total is at %.1f%%, %.1f pp above its floor of %v%%", product.pct(), product.pct()-cfg.TotalFloor, cfg.TotalFloor))
	}
	fmt.Fprintf(&b, "%-34s %7.1f%% %5v%%  %d %s (cmd/fake-* excluded)\n", product.path, product.pct(), cfg.TotalFloor, product.stmts, status)

	if len(tighten) > 0 {
		b.WriteString("tighten: raise the floor in ci/quality.json (floors never go down) -- these beat theirs by >= 1 pp:\n")
		for _, t := range tighten {
			fmt.Fprintf(&b, "  tighten: %s\n", t)
		}
	}
	if len(problems) > 0 {
		b.WriteString("coverage gate FAILED:\n")
		for _, p := range problems {
			fmt.Fprintf(&b, "  %s\n", p)
		}
		return b.String(), 1, nil
	}
	b.WriteString("coverage gate passed: every floor is met.\n")
	return b.String(), 0, nil
}
