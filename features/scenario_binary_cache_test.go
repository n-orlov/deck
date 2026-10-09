package features

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeDeckBuild stands in for `go build`: it writes a small executable to
// the -o path and counts every invocation, so the cache's contract is
// checked without the toolchain.
type fakeDeckBuild struct {
	mu    sync.Mutex
	calls [][]string
	dirs  []string
	fail  error
	block bool
}

func (f *fakeDeckBuild) run(ctx context.Context, dir string, args []string) ([]byte, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string(nil), args...))
	f.dirs = append(f.dirs, dir)
	fail, block := f.fail, f.block
	f.mu.Unlock()
	if block {
		<-ctx.Done()
		return []byte("still linking"), ctx.Err()
	}
	if fail != nil {
		return []byte("compile error"), fail
	}
	return nil, os.WriteFile(args[2], []byte("#!/bin/sh\necho "+strings.Join(args, " ")+"\n"), 0o755)
}

func (f *fakeDeckBuild) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func newFakeBuildCache(t *testing.T) (*goBuildCache, *fakeDeckBuild) {
	t.Helper()
	fake := &fakeDeckBuild{}
	cache := &goBuildCache{build: fake.run}
	t.Cleanup(func() {
		if err := cache.cleanup(); err != nil {
			t.Error(err)
		}
	})
	return cache, fake
}

func scenarioBinaryForTest(t *testing.T, cache *goBuildCache, coverDir, coverMode string) string {
	t.Helper()
	binary, err := cache.scenarioBinary("a scenario", "/repo", coverDir, coverMode)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(binary) })
	return binary
}

// TestScenarioBinaryBuildsOncePerConfigurationAndCopiesPerScenario: many
// scenarios under one build configuration cost exactly one `go build`, and
// each scenario still owns a distinct executable file at its own
// deck-godog-<pid>-<seq> path.
func TestScenarioBinaryBuildsOncePerConfigurationAndCopiesPerScenario(t *testing.T) {
	cache, fake := newFakeBuildCache(t)
	seen := map[string]bool{}
	for i := 0; i < 5; i++ {
		binary := scenarioBinaryForTest(t, cache, "", "")
		if seen[binary] {
			t.Fatalf("scenario %d reused path %s; every scenario must own a unique binary", i, binary)
		}
		seen[binary] = true
		if !strings.HasPrefix(filepath.Base(binary), "deck-godog-") || filepath.Dir(binary) != filepath.Clean(os.TempDir()) {
			t.Fatalf("scenario binary %s, want os.TempDir()/deck-godog-<pid>-<seq>", binary)
		}
		info, err := os.Stat(binary)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o100 == 0 {
			t.Fatalf("scenario binary %s mode %v is not executable", binary, info.Mode())
		}
	}
	if got := fake.count(); got != 1 {
		t.Fatalf("go build ran %d times for 5 scenarios under one configuration, want 1", got)
	}
}

// TestScenarioBinaryBuildsEachCoverConfigurationSeparately: a covered build
// (GOCOVERDIR/DECK_FEATURES_COVERMODE) never reuses the plain binary, and
// the build it runs carries the configuration's own flags.
func TestScenarioBinaryBuildsEachCoverConfigurationSeparately(t *testing.T) {
	cache, fake := newFakeBuildCache(t)
	scenarioBinaryForTest(t, cache, "", "")
	scenarioBinaryForTest(t, cache, "/cover", "")
	scenarioBinaryForTest(t, cache, "/cover", "atomic")
	scenarioBinaryForTest(t, cache, "/cover", "atomic")
	if got := fake.count(); got != 3 {
		t.Fatalf("go build ran %d times for 3 distinct configurations, want 3", got)
	}
	want := [][]string{
		deckBuildArgs("", "/repo", "", ""),
		deckBuildArgs("", "/repo", "/cover", ""),
		deckBuildArgs("", "/repo", "/cover", "atomic"),
	}
	for i, call := range fake.calls {
		got := append([]string{call[0], call[1], ""}, call[3:]...)
		if strings.Join(got, " ") != strings.Join(want[i], " ") {
			t.Fatalf("build %d args %q, want %q with its own -o path", i, call, want[i])
		}
	}
}

// TestScenarioBinaryCopyIsIndependentOfTheMaster: a scenario that removes or
// rewrites its own binary (the After hook removes it; stale_hook_binding
// swaps it) leaves the master and every later scenario's copy intact.
func TestScenarioBinaryCopyIsIndependentOfTheMaster(t *testing.T) {
	cache, fake := newFakeBuildCache(t)
	first := scenarioBinaryForTest(t, cache, "", "")
	want, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, []byte("clobbered"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(first); err != nil {
		t.Fatal(err)
	}
	second := scenarioBinaryForTest(t, cache, "", "")
	got, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("second scenario binary = %q, want the untouched build %q", got, want)
	}
	if n := fake.count(); n != 1 {
		t.Fatalf("go build ran %d times, want 1", n)
	}
}

// TestScenarioBinaryRebuildsAMissingMaster: a master removed behind the
// cache's back is rebuilt rather than copied from a path that is gone.
func TestScenarioBinaryRebuildsAMissingMaster(t *testing.T) {
	cache, fake := newFakeBuildCache(t)
	scenarioBinaryForTest(t, cache, "", "")
	for _, master := range cache.built {
		if err := os.Remove(master); err != nil {
			t.Fatal(err)
		}
	}
	scenarioBinaryForTest(t, cache, "", "")
	if got := fake.count(); got != 2 {
		t.Fatalf("go build ran %d times, want 2 (rebuild after the master vanished)", got)
	}
}

// TestScenarioBinaryNeverMemoizesAFailedBuild: a failing build fails the
// scenario that asked for it with the build output, and the next scenario
// builds again instead of inheriting the failure.
func TestScenarioBinaryNeverMemoizesAFailedBuild(t *testing.T) {
	cache, fake := newFakeBuildCache(t)
	fake.fail = errors.New("exit status 1")
	_, err := cache.scenarioBinary("broken", "/repo", "", "")
	if err == nil || !strings.Contains(err.Error(), "build deck for scenario lifecycle") || !strings.Contains(err.Error(), "compile error") {
		t.Fatalf("failed build error = %v, want the build diagnostic with its output", err)
	}
	fake.mu.Lock()
	fake.fail = nil
	fake.mu.Unlock()
	scenarioBinaryForTest(t, cache, "", "")
	if got := fake.count(); got != 2 {
		t.Fatalf("go build ran %d times, want 2 (the failure is not memoized)", got)
	}
}

// TestScenarioBinaryBuildKeepsTheInfraFaultDeadline: the one memoized build
// is still bounded by scenarioBuildDeadline and reports a stuck build as an
// infra fault naming the scenario, never memoizing it.
func TestScenarioBinaryBuildKeepsTheInfraFaultDeadline(t *testing.T) {
	cache, fake := newFakeBuildCache(t)
	fake.block = true
	saved := scenarioBuildDeadline
	scenarioBuildDeadline = 50 * time.Millisecond
	t.Cleanup(func() { scenarioBuildDeadline = saved })
	_, err := cache.scenarioBinary("stuck scenario", "/repo", "", "")
	if err == nil || !strings.Contains(err.Error(), "INFRA FAULT") || !strings.Contains(err.Error(), `"stuck scenario"`) {
		t.Fatalf("stuck build error = %v, want an INFRA FAULT naming the scenario", err)
	}
	if len(cache.built) != 0 {
		t.Fatalf("a timed-out build was memoized: %v", cache.built)
	}
}

// TestScenarioBinaryCleanupRemovesEveryMaster: TestMain's cleanup leaves no
// memoized binary behind.
func TestScenarioBinaryCleanupRemovesEveryMaster(t *testing.T) {
	cache, _ := newFakeBuildCache(t)
	scenarioBinaryForTest(t, cache, "", "")
	dir := cache.dir
	if err := cache.cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("build directory %s survived cleanup: %v", dir, err)
	}
	if err := cache.cleanup(); err != nil {
		t.Fatalf("second cleanup: %v", err)
	}
}

// TestMemoizedFixtureBuildKeysOnDirectoryFlagsAndPackage: the fake-agent
// fixture builds (memoGoBuild's callers) share one build per (dir, flags,
// package), run it in the caller's directory, and give every caller its own
// file at the exact path it asked for -- replacing a file already there, as
// `go build -o` does.
func TestMemoizedFixtureBuildKeysOnDirectoryFlagsAndPackage(t *testing.T) {
	cache, fake := newFakeBuildCache(t)
	out := t.TempDir()
	ctx := context.Background()
	build := func(dir, name, pkg string, flags ...string) string {
		t.Helper()
		target := filepath.Join(out, name)
		if _, err := cache.buildTo(ctx, dir, target, append(append([]string(nil), flags...), pkg)); err != nil {
			t.Fatal(err)
		}
		return target
	}
	build("/repo", "claude-1", "./cmd/fake-claude")
	build("/repo", "claude-2", "./cmd/fake-claude")
	build("/repo", "pi", "./cmd/fake-pi")
	build("/repo", "old-deck", "./cmd/deck", "-ldflags", "-X main.version=a")
	build("/repo", "old-deck-b", "./cmd/deck", "-ldflags", "-X main.version=b")
	build("/elsewhere", "claude-3", "./cmd/fake-claude")
	if got := fake.count(); got != 5 {
		t.Fatalf("go build ran %d times for 5 distinct (dir, flags, package) keys, want 5: %q", got, fake.calls)
	}
	if fake.dirs[0] != "/repo" || fake.dirs[4] != "/elsewhere" {
		t.Fatalf("builds ran in %q, want the caller's own directory", fake.dirs)
	}
	if err := os.WriteFile(filepath.Join(out, "claude-1"), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := build("/repo", "claude-1", "./cmd/fake-claude")
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) == "stale" || !strings.Contains(string(got), "./cmd/fake-claude") {
		t.Fatalf("existing output not replaced by the memoized build: %q", got)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("replaced output mode %v, want 0755", info.Mode().Perm())
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Fatalf("copy left a temporary file behind: %s", entry.Name())
		}
	}
}

// TestMemoizedFixtureBuildReportsTheBuildOutputOnFailure: a failing fixture
// build hands its caller the same combined output and error the direct
// `go build` would have, so the step's own diagnostic is unchanged.
func TestMemoizedFixtureBuildReportsTheBuildOutputOnFailure(t *testing.T) {
	cache, fake := newFakeBuildCache(t)
	fake.fail = errors.New("exit status 1")
	target := filepath.Join(t.TempDir(), "fake-claude")
	output, err := cache.buildTo(context.Background(), "/repo", target, []string{"./cmd/fake-claude"})
	if err == nil || string(output) != "compile error" {
		t.Fatalf("buildTo = (%q, %v), want the build's own output and error", output, err)
	}
	if _, statErr := os.Stat(target); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("a failed build left output %s behind: %v", target, statErr)
	}
}
