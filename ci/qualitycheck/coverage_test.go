package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCoverageConfig writes a ci/quality.json with only the coverage gate on.
func writeCoverageConfig(t *testing.T) string {
	t.Helper()
	return writeTempFile(t, `{
		"coverage": {"enabled": true, "total_floor": 85, "package_floor": 80, "fixture_floor": 50},
		"crap": {"enabled": false, "ceiling": 30, "fixture_ceiling": 30}
	}`)
}

// fullyCoveredTreeProfile writes a profile in which every package that has
// executable source in this repo is 100% covered, and returns its path. It
// skips the packages that hold no statements (a doc.go only, or the
// constant-only internal/racebuild), which ci/covgate names explicitly.
func fullyCoveredTreeProfile(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("mode: set\n")
	walkErr := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if p != root {
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "vendor" || name == "testdata" || name == "ci-results" {
				return filepath.SkipDir
			}
			if _, statErr := os.Stat(filepath.Join(p, "go.mod")); statErr == nil {
				return filepath.SkipDir
			}
		}
		entries, readErr := os.ReadDir(p)
		if readErr != nil {
			return readErr
		}
		hasCode := false
		for _, e := range entries {
			n := e.Name()
			if !e.IsDir() && strings.HasSuffix(n, ".go") && !strings.HasSuffix(n, "_test.go") && n != "doc.go" && !strings.HasPrefix(n, "racebuild_") {
				hasCode = true
			}
		}
		if hasCode {
			rel, _ := filepath.Rel(root, p)
			fmt.Fprintf(&b, "github.com/n-orlov/deck/%s/x.go:1.1,2.2 100 1\n", filepath.ToSlash(rel))
		}
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
	path := filepath.Join(t.TempDir(), "coverage-merged.out")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestRun_CoverageGatePassesOverAFullyCoveredTree proves the on gate really
// runs ci/covgate over this repo and passes when every floor is met.
func TestRun_CoverageGatePassesOverAFullyCoveredTree(t *testing.T) {
	root := repoRoot(t)
	t.Chdir(root)
	report, exitCode, err := run(writeCoverageConfig(t), fullyCoveredTreeProfile(t, root))
	if err != nil || exitCode != 0 {
		t.Fatalf("run = (%d, %v), want exit 0:\n%s", exitCode, err, report)
	}
	for _, want := range []string{"=== coverage gate ===", "coverage gate passed", "internal/racebuild"} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}
}

// TestRun_CoverageGateFailsWhenOnePackageIsBelowItsFloor seeds a 79% ci/
// tool into an otherwise fully covered profile.
func TestRun_CoverageGateFailsWhenOnePackageIsBelowItsFloor(t *testing.T) {
	root := repoRoot(t)
	t.Chdir(root)
	profile := fullyCoveredTreeProfile(t, root)
	data, err := os.ReadFile(profile)
	if err != nil {
		t.Fatal(err)
	}
	seeded := strings.Replace(string(data),
		"github.com/n-orlov/deck/ci/crapgate/x.go:1.1,2.2 100 1\n",
		"github.com/n-orlov/deck/ci/crapgate/x.go:1.1,2.2 79 1\ngithub.com/n-orlov/deck/ci/crapgate/x.go:3.1,4.2 21 0\n", 1)
	if seeded == string(data) {
		t.Fatal("seeding did not change the profile (ci/crapgate line not found)")
	}
	if err := rewriteProfile(profile, seeded); err != nil {
		t.Fatal(err)
	}
	report, exitCode, err := run(writeCoverageConfig(t), profile)
	if err != nil || exitCode != 1 {
		t.Fatalf("run = (%d, %v), want exit 1:\n%s", exitCode, err, report)
	}
	for _, want := range []string{"=== coverage gate ===", "ci/crapgate: 79.0% is below the package floor of 80%", "what to do"} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}
}

// TestRun_CoverageGateEmptyProfileIsAnInputError: an empty profile fails,
// never passes vacuously.
func TestRun_CoverageGateEmptyProfileIsAnInputError(t *testing.T) {
	t.Chdir(repoRoot(t))
	for name, content := range map[string]string{"mode only": "mode: set\n", "empty": ""} {
		profile := filepath.Join(t.TempDir(), "coverage-merged.out")
		if err := os.WriteFile(profile, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		_, exitCode, err := run(writeCoverageConfig(t), profile)
		if err == nil || exitCode != 2 {
			t.Errorf("%s: run = (%d, %v), want exit 2 and an error", name, exitCode, err)
		}
	}
}

// TestRun_CoverageGateNeedsAProfilePath: switching the gate on with no
// profile is a usage error.
func TestRun_CoverageGateNeedsAProfilePath(t *testing.T) {
	_, exitCode, err := run(writeCoverageConfig(t), "")
	if err == nil || exitCode != 2 {
		t.Fatalf("run = (%d, %v), want exit 2 and an error", exitCode, err)
	}
}

// TestCheckedInConfigHasTheCoverageGateOn pins R189's switch: the gate is on
// in the real ci/quality.json at no less than the PRD's floors.
func TestCheckedInConfigHasTheCoverageGateOn(t *testing.T) {
	cfg, err := loadConfig(filepath.Join(repoRoot(t), "ci", "quality.json"))
	if err != nil {
		t.Fatal(err)
	}
	c := cfg.Coverage
	if !c.Enabled || c.TotalFloor < 85 || c.PackageFloor < 80 || c.FixtureFloor <= 0 {
		t.Errorf("ci/quality.json coverage = %+v, want enabled with total_floor >= 85 and package_floor >= 80", c)
	}
}

// rewriteProfile replaces the coverage profile at path with body, writing
// through an os.Root on its directory so the name stays inside the temp dir
// fullyCoveredTreeProfile created it in.
func rewriteProfile(path, body string) error {
	scope, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { _ = scope.Close() }() // nothing is read back through this handle
	return scope.WriteFile(filepath.Base(path), []byte(body), 0o600)
}
