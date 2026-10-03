// Thresholds-loosening logic for R187's loosening test
// (TestThresholdsNotLoosened, task 085): "A Go test compares the
// thresholds file with its merge-base version (git merge-base HEAD
// origin/main, falling back to HEAD~1 on a shallow clone, with a clear
// message) and fails if any threshold is looser. Tightening passes. It
// is skipped nowhere: if the merge-base cannot be resolved, the test
// fails."
//
// The git plumbing (resolveBaseRef, loadBaseConfig) is kept separate
// from the comparison (looserThresholds) and from the test's own
// testable core (checkThresholdsNotLoosened) so a unit test can stub
// the "load the base config" step without needing a real shallow clone
// (criterion 2), while TestThresholdsNotLoosened itself still exercises
// the real git resolution end to end (criterion 1).
package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// resolveBaseRef implements R187's exact resolution order: `git
// merge-base HEAD origin/main`, falling back to `HEAD~1` only when that
// fails (the shallow-clone case named in R187), and a clear error
// naming both failures when even the fallback cannot resolve a ref.
// Callers never see a bare git error with no indication of which step
// failed.
func resolveBaseRef(repoRoot string) (string, error) {
	mergeBaseCmd := exec.Command("git", "merge-base", "HEAD", "origin/main")
	mergeBaseCmd.Dir = repoRoot
	if out, err := mergeBaseCmd.Output(); err == nil {
		return strings.TrimSpace(string(out)), nil
	} else {
		mergeBaseErr := err

		// Shallow clone (or no origin/main reachable): R187's named
		// fallback.
		fallbackCmd := exec.Command("git", "rev-parse", "HEAD~1")
		fallbackCmd.Dir = repoRoot
		out, fallbackErr := fallbackCmd.Output()
		if fallbackErr == nil {
			return strings.TrimSpace(string(out)), nil
		}

		return "", fmt.Errorf(
			"thresholds loosening test: could not resolve a base ref: "+
				"git merge-base HEAD origin/main failed (%v), and the "+
				"shallow-clone fallback git rev-parse HEAD~1 also failed (%v)",
			mergeBaseErr, fallbackErr)
	}
}

// loadBaseConfig resolves the base ref (resolveBaseRef) and reads
// ci/quality.json as it stood there, via `git show <ref>:ci/quality.json`
// -- the working tree's own ci/quality.json is never read for the base
// side of the comparison, only for the current side.
func loadBaseConfig(repoRoot string) (config, error) {
	base, err := resolveBaseRef(repoRoot)
	if err != nil {
		return config{}, err
	}

	showCmd := exec.Command("git", "show", base+":ci/quality.json")
	showCmd.Dir = repoRoot
	data, err := showCmd.Output()
	if err != nil {
		return config{}, fmt.Errorf("reading ci/quality.json at base ref %s: %w", base, err)
	}

	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return config{}, fmt.Errorf("parsing ci/quality.json at base ref %s: %w", base, err)
	}
	return cfg, nil
}

// looserThresholds returns one description per threshold that current
// loosened relative to base: a floor lowered, a ceiling raised, or a
// gate switched from on to off. A nil/empty result means current is at
// least as strict as base on every threshold (equal or tighter), which
// is the passing case -- tightening is always allowed.
func looserThresholds(base, current config) []string {
	var problems []string

	if base.Coverage.Enabled && !current.Coverage.Enabled {
		problems = append(problems, "coverage.enabled: switched off (base had it on)")
	}
	if current.Coverage.TotalFloor < base.Coverage.TotalFloor {
		problems = append(problems, fmt.Sprintf(
			"coverage.total_floor: %v < base %v (a floor must never go down)",
			current.Coverage.TotalFloor, base.Coverage.TotalFloor))
	}
	if current.Coverage.PackageFloor < base.Coverage.PackageFloor {
		problems = append(problems, fmt.Sprintf(
			"coverage.package_floor: %v < base %v (a floor must never go down)",
			current.Coverage.PackageFloor, base.Coverage.PackageFloor))
	}
	if current.Coverage.FixtureFloor < base.Coverage.FixtureFloor {
		problems = append(problems, fmt.Sprintf(
			"coverage.fixture_floor: %v < base %v (a floor must never go down)",
			current.Coverage.FixtureFloor, base.Coverage.FixtureFloor))
	}

	if base.Crap.Enabled && !current.Crap.Enabled {
		problems = append(problems, "crap.enabled: switched off (base had it on)")
	}
	if current.Crap.Ceiling > base.Crap.Ceiling {
		problems = append(problems, fmt.Sprintf(
			"crap.ceiling: %v > base %v (a ceiling must never go up)",
			current.Crap.Ceiling, base.Crap.Ceiling))
	}
	if current.Crap.FixtureCeiling > base.Crap.FixtureCeiling {
		problems = append(problems, fmt.Sprintf(
			"crap.fixture_ceiling: %v > base %v (a ceiling must never go up)",
			current.Crap.FixtureCeiling, base.Crap.FixtureCeiling))
	}

	return problems
}

// checkThresholdsNotLoosened is TestThresholdsNotLoosened's testable
// core. It loads the current thresholds file at currentPath, loads the
// base copy via loadBase, and returns a non-nil error whenever any
// threshold is looser, a gate was switched off that base had on, or
// loadBase itself could not resolve a base copy at all -- R187's "it is
// skipped nowhere: if the merge-base cannot be resolved, the test
// fails" is implemented structurally here: there is no path through
// this function, nor through its caller, that calls t.Skip -- a failed
// loadBase always becomes a returned error, never a skip.
func checkThresholdsNotLoosened(currentPath string, loadBase func() (config, error)) error {
	current, err := loadConfig(currentPath)
	if err != nil {
		return fmt.Errorf("loading current %s: %w", currentPath, err)
	}

	base, err := loadBase()
	if err != nil {
		return fmt.Errorf("resolving the base-branch ci/quality.json: %w", err)
	}

	if problems := looserThresholds(base, current); len(problems) > 0 {
		return fmt.Errorf("ci/quality.json has loosened relative to base:\n%s", strings.Join(problems, "\n"))
	}
	return nil
}
