// Package suitecheck's covfixture tests are task 002's (R187) runnable
// regression for ci/suite.sh's DECK_* scrub and merged-coverage-profile
// behaviour: they run the repository's real ci/suite.sh, unmodified,
// against the deterministic fixture in testdata/covfixture/ (an all-passing
// unit/ package and a no-op features/ package, both deliberately free of
// any non-test statement, so -coverpkg=./... instruments zero blocks for
// either -- see testdata/covfixture/unit/unit_test.go's own header).
//
// Demonstrated failing against the pre-task ci/suite.sh (9f15f685de): the
// probe file holds the leaked DECK_SEED_PROBE value instead of "", the
// script exits 0, and <outdir>/coverage-merged.out is never written at
// all. See /run/ralphd/artifacts/002/pre-task-run.log.
package suitecheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// covFixtureRun is one ci/suite.sh run against a fresh copy of
// testdata/covfixture/.
type covFixtureRun struct {
	module string
	outdir string
	probe  string
	out    []byte
	err    error
}

// runCovFixture copies the covfixture module plus the real ci/suite.sh and
// ci/junitflaky into a temp dir and runs the script there with the caller's
// DECK_* stripped, then extraEnv appended.
func runCovFixture(t *testing.T, extraEnv ...string) covFixtureRun {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Fatalf("go toolchain not on PATH: %v", err)
	}
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	fixture := filepath.Join(root, "ci", "suitecheck", "testdata", "covfixture")
	module := t.TempDir()
	copyFile(t, filepath.Join(fixture, "go.mod.fixture"), filepath.Join(module, "go.mod"), 0o644)
	for _, rel := range []string{"unit/unit_test.go", "unit/extraflag_test.go", "features/fixture_test.go", "features/extraflag_test.go"} {
		copyFile(t, filepath.Join(fixture, filepath.FromSlash(rel)), filepath.Join(module, filepath.FromSlash(rel)), 0o644)
	}
	copyFile(t, filepath.Join(root, "ci", "suite.sh"), filepath.Join(module, "ci", "suite.sh"), 0o755)
	copyFile(t, filepath.Join(root, "ci", "junitflaky", "main.go"), filepath.Join(module, "ci", "junitflaky", "main.go"), 0o644)

	run := covFixtureRun{
		module: module,
		outdir: filepath.Join(module, "out"),
		probe:  filepath.Join(module, "probe.txt"),
	}

	env := make([]string, 0, len(os.Environ())+6+len(extraEnv))
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "DECK_") || strings.HasPrefix(kv, "GOWORK=") || strings.HasPrefix(kv, "GOCOVERDIR=") || strings.HasPrefix(kv, "EXTRA_PROBE_DIR=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env,
		"GOWORK=off",
		"DECK_CI_OUT="+run.outdir,
		"DECK_CI_GO_PACKAGES=./unit/",
		// Not a DECK_* name, so the scrub must leave it alone -- it is how
		// the fixture's own test process reports what it saw back to this
		// test, across the subprocess boundary.
		"PROBE_OUT="+run.probe,
	)
	env = append(env, extraEnv...)

	cmd := exec.Command("sh", filepath.Join(module, "ci", "suite.sh"))
	cmd.Dir = module
	cmd.Env = env
	run.out, run.err = cmd.CombinedOutput()
	t.Logf("ci/suite.sh output:\n%s", run.out)
	return run
}

func TestSuiteScriptScrubsDeckEnvironmentAndFailsOnEmptyMergedCoverage(t *testing.T) {
	// The seeded leak this test exists to catch: a DECK_* variable present
	// in the CALLER's own environment for a reason that has nothing to do
	// with this run (DECK_SEED_PROBE stands in for a real one like
	// DECK_GODOG_PATHS/DECK_HOME).
	run := runCovFixture(t, "DECK_SEED_PROBE=leaked-value")
	probe, outdir, runErr := run.probe, run.outdir, run.err

	seen, err := os.ReadFile(probe)
	if err != nil {
		t.Fatalf("fixture never wrote probe file %s (did unit/unit_test.go even run?): %v", probe, err)
	}
	if string(seen) != "" {
		t.Errorf("DECK_SEED_PROBE leaked into the go test process's own environment: saw %q, want \"\" (unset) -- ci/suite.sh must unset every DECK_* variable from its own environment before launching go test", seen)
	}

	if runErr == nil {
		t.Errorf("ci/suite.sh exited 0 for a run whose unit+features coverage data is empty (both fixture packages report \"[no statements]\"); it must exit non-zero when coverage-merged.out has no blocks")
	}

	mergedPath := filepath.Join(outdir, "coverage-merged.out")
	merged, err := os.ReadFile(mergedPath)
	if err != nil {
		t.Fatalf("%s was never written: %v", mergedPath, err)
	}
	// At most the "mode: <mode>" header line; a genuine block line would
	// mean this fixture failed to stay statement-free and the test is no
	// longer exercising the empty-profile path it claims to.
	lines := strings.Split(strings.TrimRight(string(merged), "\n"), "\n")
	blocks := 0
	for _, l := range lines {
		if l != "" && !strings.HasPrefix(l, "mode:") {
			blocks++
		}
	}
	if blocks != 0 {
		t.Fatalf("%s unexpectedly has %d coverage block line(s); fixture precondition (zero instrumented statements) no longer holds:\n%s", mergedPath, blocks, merged)
	}
}

// TestSuiteScriptPassesCapturedExtraFlagsToBothPasses is the regression for
// the scrub unsetting DECK_CI_GO_EXTRA_FLAGS before the two `go test`
// invocations expanded it (task 002, first validation): the nightly lane's
// -race silently vanished from both passes. -tags=suiteextraflag stands in
// for -race (which needs cgo) -- it builds the fixture's extraflag_test.go
// files only when the flag actually reaches `go test`, and each pass then
// writes its own marker.
func TestSuiteScriptPassesCapturedExtraFlagsToBothPasses(t *testing.T) {
	markers := t.TempDir()
	runCovFixture(t, "DECK_CI_GO_EXTRA_FLAGS=-tags=suiteextraflag", "EXTRA_PROBE_DIR="+markers)
	for _, pass := range []string{"unit", "features"} {
		got, err := os.ReadFile(filepath.Join(markers, pass))
		if err != nil {
			t.Errorf("%s pass never built its -tags=suiteextraflag file: DECK_CI_GO_EXTRA_FLAGS did not reach that pass's go test (%v)", pass, err)
			continue
		}
		if string(got) != "tagged" {
			t.Errorf("%s pass marker = %q, want \"tagged\"", pass, got)
		}
	}
}
