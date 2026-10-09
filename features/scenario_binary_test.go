package features

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// goBuildCache memoizes the `go build` outputs the scenario lifecycle and the
// scenario steps hand every godog scenario: the released deck binary and the
// fake agent fixtures (the deferred task-219 requirement recorded in
// docs/reports/phase3-findings.md: "build the deck (and any other
// per-scenario fixture, e.g. cmd/fake-claude) binary ONCE per test-binary
// invocation instead of once per scenario"). Before it, the Before hook ran a
// fresh `go build ./cmd/deck` for every one of the 400+ scenarios and the
// fake-agent steps another `go build` each; every one re-links a whole
// binary, which was the dominant fixed cost of the features package (task
// 021: features/ alone ran 888-912s against go test's default 10m alarm).
//
// One build is made per distinct (directory, flags, package) -- so a plain
// run and a covered run (ci/suite.sh's GOCOVERDIR/DECK_FEATURES_COVERMODE),
// or two -ldflags variants, never share a binary. Isolation is unchanged:
// every caller still gets its OWN executable file at the exact path it asked
// for, a byte copy of the memoized build, which it may remove, overwrite or
// swap (the After hook removes h.Binary; stale_hook_binding_test.go swaps it)
// without touching the memoized build or any other scenario. A failed build
// is never memoized: the next caller builds again, exactly as each caller
// did before.
type goBuildCache struct {
	mu    sync.Mutex
	dir   string
	built map[string]string
	// build runs `go <args>` in dir under ctx; a field so the cache's own
	// tests can count builds and inject failures without the toolchain.
	build func(ctx context.Context, dir string, args []string) ([]byte, error)
}

func runGoBuild(ctx context.Context, dir string, args []string) ([]byte, error) {
	command := exec.CommandContext(ctx, "go", args...)
	command.Dir = dir
	return command.CombinedOutput()
}

// scenarioBuilds is the process-wide cache; TestMain removes its directory
// once every test has run.
var scenarioBuilds = &goBuildCache{build: runGoBuild}

// memoGoBuild is `go build <flags...> -o out <pkg>` run in dir, returning the
// same combined output and error the direct command would, with the build
// itself made once per distinct (dir, flags, pkg) per test process.
func memoGoBuild(ctx context.Context, dir, out string, pkg string, flags ...string) ([]byte, error) {
	return scenarioBuilds.buildTo(ctx, dir, out, append(append([]string(nil), flags...), pkg))
}

// buildTo copies the memoized build of flagsAndPkg (built in dir on first
// use, or again when an earlier one has gone missing) to out.
func (c *goBuildCache) buildTo(ctx context.Context, dir, out string, flagsAndPkg []string) ([]byte, error) {
	master, output, err := c.master(ctx, dir, flagsAndPkg)
	if err != nil {
		return output, err
	}
	if err := copyExecutable(master, out); err != nil {
		return nil, fmt.Errorf("copy memoized build %s to %s: %w", master, out, err)
	}
	return output, nil
}

func (c *goBuildCache) master(ctx context.Context, dir string, flagsAndPkg []string) (string, []byte, error) {
	key := dir + "\x00" + strings.Join(flagsAndPkg, "\x00")
	c.mu.Lock()
	defer c.mu.Unlock()
	if path, ok := c.built[key]; ok {
		if _, err := os.Stat(path); err == nil {
			return path, nil, nil
		}
		delete(c.built, key)
	}
	if c.dir == "" {
		buildDir, err := os.MkdirTemp("", fmt.Sprintf("deck-godog-build-%d-", os.Getpid()))
		if err != nil {
			return "", nil, fmt.Errorf("create scenario build directory: %w", err)
		}
		c.dir = buildDir
		c.built = make(map[string]string)
	}
	binary := filepath.Join(c.dir, fmt.Sprintf("build-%d", len(c.built)+1))
	args := append([]string{"build", "-o", binary}, flagsAndPkg...)
	output, err := c.build(ctx, dir, args)
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		_ = os.Remove(binary)
		return "", output, err
	}
	c.built[key] = binary
	return binary, output, nil
}

// scenarioBinary returns a fresh, scenario-owned copy of the released deck
// binary at a unique deck-godog-<pid>-<seq> path, its one build bounded by
// scenarioBuildDeadline.
func (c *goBuildCache) scenarioBinary(scenario, root, coverDir, coverMode string) (string, error) {
	binary := filepath.Join(os.TempDir(), fmt.Sprintf("deck-godog-%d-%d", os.Getpid(), scenarioSequence.Add(1)))
	buildCtx, cancel := context.WithTimeout(context.Background(), scenarioBuildDeadline)
	defer cancel()
	start := time.Now()
	// deckBuildArgs is `build -o <binary> <flags...> <pkg>`; the cache owns
	// the -o path of the memoized build.
	output, err := c.buildTo(buildCtx, "", binary, deckBuildArgs(binary, root, coverDir, coverMode)[3:])
	elapsed := time.Since(start)
	if buildCtx.Err() != nil {
		_ = os.Remove(binary)
		return "", fmt.Errorf("INFRA FAULT (not a product bug): `go build` for scenario %q (binary %s) did not finish within its %s bound; elapsed %s; partial output:\n%s", scenario, binary, scenarioBuildDeadline, elapsed, output)
	}
	if err != nil {
		_ = os.Remove(binary)
		return "", fmt.Errorf("build deck for scenario lifecycle: %w\n%s", err, output)
	}
	return binary, nil
}

// cleanup removes every memoized build.
func (c *goBuildCache) cleanup() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dir == "" {
		return nil
	}
	err := os.RemoveAll(c.dir)
	c.dir, c.built = "", nil
	return err
}

// copyExecutable writes src's bytes to dst as a new 0755 file, replacing any
// file already there by rename (as `go build -o` would), so a process still
// executing an old dst is never written into.
func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".tmp-")
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(tmp, in)
	closeErr := tmp.Close()
	if err := firstError(copyErr, closeErr, os.Chmod(tmp.Name(), 0o755)); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func TestMain(m *testing.M) {
	code := m.Run()
	if err := scenarioBuilds.cleanup(); err != nil {
		fmt.Fprintf(os.Stderr, "remove memoized scenario builds: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}
