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

	report, code, err := runInWorkingDir(*configPath, *profilePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "covgate:", err)
		os.Exit(code)
	}
	fmt.Print(report)
	os.Exit(code)
}

// runInWorkingDir is main's testable core: it resolves the working
// directory (the module root the gate scores) and hands over to run, so
// main itself holds no branch beyond the shared report/exit step.
func runInWorkingDir(configPath, profilePath string) (report string, code int, err error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", 2, err
	}
	return run(configPath, profilePath, wd)
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

// gateInputs is everything run reads before scoring.
type gateInputs struct {
	cfg        coverageConfig
	prof       *profile
	modulePath string
	onDisk     map[string]bool
}

// loadInputs validates the flags and reads the config, the profile and the
// module's source packages; every failure is a usage/input error (exit 2).
func loadInputs(configPath, profilePath, moduleDir string) (gateInputs, error) {
	if configPath == "" {
		return gateInputs{}, fmt.Errorf("-config is required")
	}
	if profilePath == "" {
		return gateInputs{}, fmt.Errorf("-profile is required")
	}
	cfg, err := loadConfig(configPath)
	if err != nil {
		return gateInputs{}, err
	}
	prof, err := parseProfile(profilePath)
	if err != nil {
		return gateInputs{}, err
	}
	root, modulePath, err := findModule(moduleDir)
	if err != nil {
		return gateInputs{}, err
	}
	onDisk, err := sourcePackages(root)
	if err != nil {
		return gateInputs{}, err
	}
	return gateInputs{cfg: cfg, prof: prof, modulePath: modulePath, onDisk: onDisk}, nil
}

// scorePackages totals the profile's statements per package and adds every
// on-disk package, so an unbuilt one shows up with zero statements.
func scorePackages(in gateInputs) (map[string]*pkgScore, error) {
	byPkg := make(map[string]*pkgScore)
	score := func(pkg string) *pkgScore {
		if byPkg[pkg] == nil {
			byPkg[pkg] = &pkgScore{path: pkg}
		}
		return byPkg[pkg]
	}
	for key, stat := range in.prof.blocks {
		pkg, perr := packageOf(key.file, in.modulePath)
		if perr != nil {
			return nil, perr
		}
		s := score(pkg)
		s.stmts += stat.numStmt
		if stat.covered {
			s.covered += stat.numStmt
		}
	}
	for pkg := range in.onDisk {
		score(pkg)
	}
	return byPkg, nil
}

// zeroListProblems checks zeroStatementPackages against the profile and the
// disk, and returns the set of names the list holds.
func zeroListProblems(byPkg map[string]*pkgScore, onDisk map[string]bool) (map[string]bool, []string) {
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
	return zero, problems
}

// gateReport accumulates the gate's text, its problems and the tighten prompts.
type gateReport struct {
	b        strings.Builder
	problems []string
	tighten  []string
	product  pkgScore
}

// addPackage scores one package against its floor.
func (r *gateReport) addPackage(cfg coverageConfig, s *pkgScore, zero map[string]bool) {
	pkg := s.path
	if s.stmts == 0 {
		if zero[pkg] {
			fmt.Fprintf(&r.b, "%-34s %8s %6s  0 (named in the zero-statement list)\n", pkg, "n/a", "n/a")
			return
		}
		r.problems = append(r.problems, fmt.Sprintf("%s: zero statements in the profile and not named in the zero-statement list (the package was never built under the profile: run it under the suite, or, if it truly has no statements, name it by exact path in ci/covgate's zeroStatementPackages)", pkg))
		fmt.Fprintf(&r.b, "%-34s %8s %6s  0 MISSING\n", pkg, "-", "-")
		return
	}
	floor, kind := cfg.PackageFloor, "package"
	if isFixture(pkg) {
		floor, kind = cfg.FixtureFloor, "fixture"
	} else {
		r.product.stmts += s.stmts
		r.product.covered += s.covered
	}
	status := "ok"
	if s.pct() < floor {
		status = "FAIL"
		r.problems = append(r.problems, fmt.Sprintf("%s: %.1f%% is below the %s floor of %v%% (%d of %d statements covered; add behaviour tests for its uncovered paths)", pkg, s.pct(), kind, floor, s.covered, s.stmts))
	} else if s.pct()-floor >= tightenMargin {
		r.tighten = append(r.tighten, fmt.Sprintf("%s is at %.1f%%, %.1f pp above its %s floor of %v%%", pkg, s.pct(), s.pct()-floor, kind, floor))
	}
	fmt.Fprintf(&r.b, "%-34s %7.1f%% %5v%%  %d %s\n", pkg, s.pct(), floor, s.stmts, status)
}

// addTotal scores the product total; a profile with no product statements
// is an input error.
func (r *gateReport) addTotal(cfg coverageConfig) error {
	if r.product.stmts == 0 {
		return fmt.Errorf("the profile holds no product statements (only cmd/fake-* fixtures, or nothing)")
	}
	status := "ok"
	if r.product.pct() < cfg.TotalFloor {
		status = "FAIL"
		r.problems = append(r.problems, fmt.Sprintf("product total: %.1f%% is below the total floor of %v%% (%d of %d statements covered)", r.product.pct(), cfg.TotalFloor, r.product.covered, r.product.stmts))
	} else if r.product.pct()-cfg.TotalFloor >= tightenMargin {
		r.tighten = append(r.tighten, fmt.Sprintf("the product total is at %.1f%%, %.1f pp above its floor of %v%%", r.product.pct(), r.product.pct()-cfg.TotalFloor, cfg.TotalFloor))
	}
	fmt.Fprintf(&r.b, "%-34s %7.1f%% %5v%%  %d %s (cmd/fake-* excluded)\n", r.product.path, r.product.pct(), cfg.TotalFloor, r.product.stmts, status)
	return nil
}

// finish renders the tighten prompts and the verdict.
func (r *gateReport) finish() (string, int) {
	if len(r.tighten) > 0 {
		r.b.WriteString("tighten: raise the floor in ci/quality.json (floors never go down) -- these beat theirs by >= 1 pp:\n")
		for _, t := range r.tighten {
			fmt.Fprintf(&r.b, "  tighten: %s\n", t)
		}
	}
	if len(r.problems) > 0 {
		r.b.WriteString("coverage gate FAILED:\n")
		for _, p := range r.problems {
			fmt.Fprintf(&r.b, "  %s\n", p)
		}
		return r.b.String(), 1
	}
	r.b.WriteString("coverage gate passed: every floor is met.\n")
	return r.b.String(), 0
}

// run is main's testable core. moduleDir is any directory inside the module.
func run(configPath, profilePath, moduleDir string) (string, int, error) {
	in, err := loadInputs(configPath, profilePath, moduleDir)
	if err != nil {
		return "", 2, err
	}
	byPkg, err := scorePackages(in)
	if err != nil {
		return "", 2, err
	}
	zero, problems := zeroListProblems(byPkg, in.onDisk)

	names := make([]string, 0, len(byPkg))
	for pkg := range byPkg {
		names = append(names, pkg)
	}
	sort.Strings(names)

	r := &gateReport{problems: problems}
	r.product.path = "product total"
	fmt.Fprintf(&r.b, "%-34s %8s %6s  %s\n", "package", "cover", "floor", "statements")
	for _, pkg := range names {
		r.addPackage(in.cfg, byPkg[pkg], zero)
	}
	if err := r.addTotal(in.cfg); err != nil {
		return "", 2, err
	}
	out, code := r.finish()
	return out, code, nil
}
