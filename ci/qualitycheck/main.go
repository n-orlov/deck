// Package main implements ci/qualitycheck, R187's gate driver behind
// ci/quality.sh.
//
//	go run ./ci/qualitycheck -config <ci/quality.json> -profile <merged coverprofile>
//
// It reads thresholds from exactly one place, the checked-in
// ci/quality.json ("coverage" floors, "crap" ceiling, each with its own
// "enabled" flag), runs every gate whose flag is on, prints one report
// section per on gate (top offenders, plus what to do about each), and
// exits non-zero if any on gate fails. With every gate off (the
// checked-in state at this task) it prints that nothing is enabled and
// exits 0.
//
// Exit codes mirror ci/crapgate's own convention: 0 every on gate
// passed (or none are on), 1 at least one on gate failed, 2 a
// usage/input error (missing -config, an unreadable/malformed
// ci/quality.json, or a profile a gate needed but did not get).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// coverageConfig is R187's coverage-gate thresholds: a floor over the
// whole merged profile, a floor every product package must individually
// clear, and the cmd/fake-* fixtures' own, lower floor (R187: "they get
// their own, lower coverage floor").
type coverageConfig struct {
	Enabled      bool    `json:"enabled"`
	TotalFloor   float64 `json:"total_floor"`
	PackageFloor float64 `json:"package_floor"`
	FixtureFloor float64 `json:"fixture_floor"`
}

// crapConfig is R187's CRAP-gate ceiling. FixtureCeiling is carried as
// its own field even though R187 sets it equal to Ceiling ("the same
// CRAP ceiling as product") -- storing it explicitly, rather than
// silently reusing Ceiling, is what lets a loosening test (task 085)
// catch either one being loosened on its own.
type crapConfig struct {
	Enabled        bool    `json:"enabled"`
	Ceiling        float64 `json:"ceiling"`
	FixtureCeiling float64 `json:"fixture_ceiling"`
}

// config is the whole of ci/quality.json. ci/quality.sh and every gate
// read thresholds only from here (criterion 1): no other file, flag or
// environment variable carries a threshold.
type config struct {
	Coverage coverageConfig `json:"coverage"`
	Crap     crapConfig     `json:"crap"`
	Trivy    trivyConfig    `json:"trivy"`

	Govulncheck govulncheckConfig `json:"govulncheck"`
}

// trivyBase carries the trivy gate's flag-supplied options (binary,
// target, cache dir, ignore file); run fills in the severity from the
// config and the clock. A package variable so run's signature, which
// every other gate's tests use, stays put.
var trivyBase = trivyOptions{Target: ".", IgnoreFile: ".trivyignore", CacheDir: "/go-cache/trivy"}

// govulncheckBase carries the govulncheck gate's flag-supplied options.
var govulncheckBase = govulncheckOptions{Target: "."}

func main() {
	configPath := flag.String("config", "", "path to ci/quality.json (required)")
	profilePath := flag.String("profile", "", "path to the merged coverage profile (required if an on gate needs it)")
	flag.StringVar(&trivyBase.Binary, "trivy-bin", "trivy", "trivy executable")
	flag.StringVar(&trivyBase.Target, "trivy-target", ".", "directory the trivy gate scans")
	flag.StringVar(&trivyBase.CacheDir, "trivy-cache", "/go-cache/trivy", "trivy DB cache directory")
	flag.StringVar(&trivyBase.IgnoreFile, "trivy-ignore", ".trivyignore", "trivy exceptions file")
	flag.StringVar(&govulncheckBase.Binary, "govulncheck-bin", "govulncheck", "govulncheck executable")
	flag.StringVar(&govulncheckBase.Target, "govulncheck-target", ".", "module root the govulncheck gate scans")
	flag.StringVar(&govulncheckBase.CacheDir, "govulncheck-cache", "", "XDG_CACHE_HOME for govulncheck (its vuln DB cache); empty inherits")
	flag.Parse()

	report, exitCode, err := run(*configPath, *profilePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "qualitycheck:", err)
		os.Exit(exitCode)
	}
	fmt.Print(report)
	os.Exit(exitCode)
}

// gateResult is one gate's outcome: its report section and whether it
// passed.
type gateResult struct {
	name   string
	ok     bool
	output string
}

// run is main's testable core: it never touches flag/os.Exit directly.
func run(configPath, profilePath string) (report string, exitCode int, err error) {
	if configPath == "" {
		return "", 2, fmt.Errorf("-config is required")
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		return "", 2, err
	}

	var results []gateResult
	anyEnabled := false

	if cfg.Crap.Enabled {
		anyEnabled = true
		ok, out, rerr := runCrapGate(profilePath, cfg.Crap.Ceiling)
		if rerr != nil {
			return "", 2, rerr
		}
		results = append(results, gateResult{name: "crap", ok: ok, output: out})
	}

	if cfg.Coverage.Enabled {
		anyEnabled = true
		// R189 (task 022) adds the real coverage scorer (ci/covgate).
		// Until it lands, an enabled coverage gate fails loudly rather
		// than passing vacuously -- the checked-in ci/quality.json
		// always carries "enabled": false, so this branch is dead code
		// in the committed state, but a mis-edited config must never
		// silently pass just because nothing implements the gate yet.
		results = append(results, gateResult{
			name: "coverage",
			ok:   false,
			output: "coverage gate: enabled in " + configPath + " but not yet implemented " +
				"(R189/task 022 adds ci/covgate)\nwhat to do: set coverage.enabled back to " +
				"false until ci/covgate lands, or implement it before enabling this gate.\n",
		})
	}

	if cfg.Trivy.Enabled {
		anyEnabled = true
		opts := trivyBase
		opts.Severity = cfg.Trivy.Severity
		opts.Now = time.Now()
		ok, out, rerr := runTrivyGate(opts)
		if rerr != nil {
			return "", 2, rerr
		}
		results = append(results, gateResult{name: "trivy", ok: ok, output: out})
	}

	if cfg.Govulncheck.Enabled {
		anyEnabled = true
		ok, out, rerr := runGovulncheckGate(govulncheckBase)
		if rerr != nil {
			return "", 2, rerr
		}
		results = append(results, gateResult{name: "govulncheck", ok: ok, output: out})
	}

	var b strings.Builder
	overallOK := true
	for _, r := range results {
		fmt.Fprintf(&b, "=== %s gate ===\n", r.name)
		b.WriteString(r.output)
		if !strings.HasSuffix(r.output, "\n") {
			b.WriteString("\n")
		}
		if !r.ok {
			overallOK = false
		}
	}
	if !anyEnabled {
		fmt.Fprintf(&b, "ci/quality.sh: no gates are enabled in %s\n", configPath)
	}

	if !overallOK {
		return b.String(), 1, nil
	}
	return b.String(), 0, nil
}

// loadConfig reads and parses path as a config. A missing or malformed
// file is a usage error (exit 2), never a silent "every gate off".
func loadConfig(path string) (config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return config{}, fmt.Errorf("reading %s: %w", path, err)
	}
	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return config{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	return cfg, nil
}

// exitStatusLineRe matches the diagnostic "exit status N" line `go run`
// itself appends to stderr whenever the target program exits non-zero.
// `go run` does not propagate that program's own exit code as its own
// (it always exits 1 for any build or run failure), so this is the only
// way to recover crapgate's REAL exit code (1: offenders found, 2:
// usage/input error) rather than treating every failure as a gate loss.
var exitStatusLineRe = regexp.MustCompile(`(?m)^exit status ([0-9]+)\n?`)

// runCrapGate shells out to ci/crapgate over the whole module (no
// -filter): R187's fixture rule gives cmd/fake-* the SAME CRAP ceiling
// as product ("the same CRAP ceiling as product"), so one whole-tree
// scan at Ceiling already enforces FixtureCeiling too for as long as
// the two stay equal -- exactly the invariant the loosening test
// (task 085) exists to keep true.
func runCrapGate(profilePath string, ceiling float64) (ok bool, output string, err error) {
	if profilePath == "" {
		return false, "", fmt.Errorf("-profile is required when the crap gate is enabled")
	}
	cmd := exec.Command("go", "run", "./ci/crapgate",
		"-profile", profilePath,
		"-max", strconv.FormatFloat(ceiling, 'f', -1, 64))
	out, runErr := cmd.CombinedOutput()
	text := string(out)

	exitCode := 0
	if runErr != nil {
		if _, isExit := runErr.(*exec.ExitError); !isExit {
			return false, text, fmt.Errorf("running ci/crapgate: %w", runErr)
		}
		// go run's own exit code is always 1 here; recover crapgate's real
		// one from the "exit status N" line it appends, and strip that
		// line out of the report -- it is go run's own diagnostic noise,
		// not part of crapgate's report.
		exitCode = 1
		if m := exitStatusLineRe.FindStringSubmatch(text); m != nil {
			if n, convErr := strconv.Atoi(m[1]); convErr == nil {
				exitCode = n
			}
			text = exitStatusLineRe.ReplaceAllString(text, "")
		}
	}

	switch exitCode {
	case 0:
		return true, text + "what to do: nothing -- every function is at or under the CRAP ceiling.\n", nil
	case 1:
		return false, text + "what to do: refactor the named offender(s) (split branches/loops, " +
			"extract helpers) or raise their test coverage; CRAP = cc^2*(1-cov)^3 + cc.\n", nil
	default:
		return false, text, fmt.Errorf("ci/crapgate exited %d (usage/input error): %s", exitCode, text)
	}
}
