// Package pagespersist is the R165 probe for ci/pages-persist.sh (GH #50):
// an integration test that exercises the script for real against two
// local bare git repositories acting as `origin`, rather than parsing
// YAML -- the property under test ("a rejected push retries, and every
// still-live contribution survives the merge") is about the script's own
// runtime behaviour under a race, never about ci.yml's structure.
//
// Demonstrated failing against the pre-fix dc2b6f7ece tree, which has no
// ci/pages-persist.sh at all (the persist step was a bare `git push` with
// no retry): see /run/ralphd/artifacts/r165/pagespersist-dc2b6f7ece-fail.log.
package pagespersist

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func repositoryRoot() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory, nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", errors.New("locate repository root containing go.mod")
		}
		directory = parent
	}
}

// gitEnv gives every git invocation in this test a private, isolated
// identity and config -- never the operator's real ~/.gitconfig or
// credential store (this test never touches a network remote; every
// `origin` here is a local bare repo under t.TempDir()).
func gitEnv(home string) []string {
	env := os.Environ()
	filtered := env[:0]
	for _, kv := range env {
		if strings.HasPrefix(kv, "HOME=") {
			continue
		}
		filtered = append(filtered, kv)
	}
	return append(filtered,
		"HOME="+home,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=pagespersist-test",
		"GIT_AUTHOR_EMAIL=pagespersist-test@example.com",
		"GIT_COMMITTER_NAME=pagespersist-test",
		"GIT_COMMITTER_EMAIL=pagespersist-test@example.com",
	)
}

func runGit(t *testing.T, home, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv(home)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v (in %s): %v\n%s", args, dir, err, out)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

// newBareOrigin creates an empty bare repo at <tmp>/origin.git, to stand
// in for the real `gh-pages` remote (a plain local git server, since
// bare + `file://`/plain-path remotes need no daemon and no auth).
func newBareOrigin(t *testing.T, home, tmp string) string {
	t.Helper()
	bare := filepath.Join(tmp, "origin.git")
	if err := os.MkdirAll(bare, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", bare, err)
	}
	runGit(t, home, bare, "init", "--bare", "-q", ".")
	return bare
}

// checkoutGhPages gives back a directory checked out on gh-pages against
// origin -- either a fresh orphan (gh-pages does not exist on origin yet)
// or a real checkout of whatever origin already holds -- matching exactly
// the state ci.yml's own "Check out gh-pages, or start it as an orphan"
// step leaves `site` in before it ever calls ci/pages-persist.sh.
func checkoutGhPages(t *testing.T, home, tmp, name, origin string) string {
	t.Helper()
	dir := filepath.Join(tmp, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	runGit(t, home, dir, "init", "-q", ".")
	runGit(t, home, dir, "remote", "add", "origin", origin)
	hasBranch := exec.Command("git", "ls-remote", "--exit-code", "--heads", origin, "gh-pages")
	hasBranch.Dir = dir
	hasBranch.Env = gitEnv(home)
	if hasBranch.Run() == nil {
		runGit(t, home, dir, "fetch", "--quiet", "origin", "gh-pages")
		runGit(t, home, dir, "checkout", "-q", "-B", "gh-pages", "origin/gh-pages")
	} else {
		runGit(t, home, dir, "checkout", "-q", "--orphan", "gh-pages")
	}
	return dir
}

func commitAndPush(t *testing.T, home, dir, message string) error {
	t.Helper()
	runGit(t, home, dir, "add", "-A")
	runGit(t, home, dir, "commit", "-q", "-m", message)
	cmd := exec.Command("git", "push", "-q", "origin", "gh-pages")
	cmd.Dir = dir
	cmd.Env = gitEnv(home)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.New(string(out) + err.Error())
	}
	return nil
}

func pagesPersistScript(t *testing.T) string {
	t.Helper()
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	path := filepath.Join(root, "ci", "pages-persist.sh")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("ci/pages-persist.sh missing: %v", err)
	}
	return path
}

// TestPagesPersistRetriesPastAConcurrentPush is the success case: a
// concurrent run's `pr/42/` report lands on the shared `gh-pages` remote
// first (racing this run's own root report), rejecting this run's first
// push as a non-fast-forward. The script must retry: re-fetch, re-merge
// this run's fresh root report on top of the newer tree with
// ci/allure-site.sh, and push again -- ending with a tree that carries
// every pre-existing pr/<n>/ (both the one seeded before either run, and
// the one the concurrent run itself added) plus this run's own root.
func TestPagesPersistRetriesPastAConcurrentPush(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", home, err)
	}

	origin := newBareOrigin(t, home, tmp)

	// Seed gh-pages with a baseline root and one pre-existing pr/7/ --
	// content that predates both runs in this test and must survive
	// untouched by either.
	seed := checkoutGhPages(t, home, tmp, "seed", origin)
	writeFile(t, filepath.Join(seed, "index.html"), "root-baseline")
	writeFile(t, filepath.Join(seed, "pr", "7", "index.html"), "pr-7-baseline")
	if err := commitAndPush(t, home, seed, "baseline"); err != nil {
		t.Fatalf("seed push: %v", err)
	}

	// The checkout our own script under test will operate on -- taken
	// from the baseline, exactly like ci.yml's checkout step.
	site := checkoutGhPages(t, home, tmp, "site", origin)

	// A second, independent checkout simulates the concurrent run: it
	// starts from the same baseline, adds its own pr/42/ subtree, and
	// pushes -- and, being first, succeeds outright with no retry of its
	// own. This is what makes `site`'s own next push a non-fast-forward.
	concurrent := checkoutGhPages(t, home, tmp, "concurrent", origin)
	writeFile(t, filepath.Join(concurrent, "pr", "42", "index.html"), "pr-42-concurrent")
	if err := commitAndPush(t, home, concurrent, "concurrent pr 42 report"); err != nil {
		t.Fatalf("concurrent push: %v", err)
	}

	// This run's own freshly rendered report (ci/allure-report.sh's
	// output, stood in for by a marker file): a root report, distinct
	// from the baseline root so the assertion below can tell "ours" from
	// "still the old baseline".
	report := filepath.Join(tmp, "report")
	writeFile(t, filepath.Join(report, "index.html"), "root-fresh-this-run")

	cmd := exec.Command("sh", pagesPersistScript(t), site, report, "root", "5")
	cmd.Env = gitEnv(home)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ci/pages-persist.sh failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "pushed on attempt 2") {
		t.Errorf("expected the script to report succeeding on its retry (attempt 2), got:\n%s", out)
	}

	verify := checkoutGhPages(t, home, tmp, "verify", origin)
	if got := readFile(t, filepath.Join(verify, "index.html")); got != "root-fresh-this-run" {
		t.Errorf("root content = %q, want the retrying run's own report", got)
	}
	if got := readFile(t, filepath.Join(verify, "pr", "7", "index.html")); got != "pr-7-baseline" {
		t.Errorf("pre-existing pr/7/ = %q, want it to have survived untouched", got)
	}
	if got := readFile(t, filepath.Join(verify, "pr", "42", "index.html")); got != "pr-42-concurrent" {
		t.Errorf("concurrent pr/42/ = %q, want it to have survived the retry's re-merge", got)
	}
}

// TestPagesPersistFailsWhenRetriesExhausted is the bound case: every push
// this run makes is rejected (a bare repo `pre-receive` hook that declines
// unconditionally stands in for a remote that never lets this run land,
// e.g. persistent contention with other runs) so the script must give up
// with a non-zero exit once it has tried the given bound's worth of
// attempts, rather than retrying forever.
func TestPagesPersistFailsWhenRetriesExhausted(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", home, err)
	}

	origin := newBareOrigin(t, home, tmp)

	// Seed a baseline before the hook goes in -- the hook must reject
	// every push the script itself makes, not the setup that establishes
	// the starting state.
	seed := checkoutGhPages(t, home, tmp, "seed", origin)
	writeFile(t, filepath.Join(seed, "index.html"), "root-baseline")
	if err := commitAndPush(t, home, seed, "baseline"); err != nil {
		t.Fatalf("seed push: %v", err)
	}

	hook := "#!/bin/sh\nexit 1\n"
	hookPath := filepath.Join(origin, "hooks", "pre-receive")
	writeFile(t, hookPath, hook)
	if err := os.Chmod(hookPath, 0o755); err != nil {
		t.Fatalf("chmod %s: %v", hookPath, err)
	}

	site := checkoutGhPages(t, home, tmp, "site", origin)
	report := filepath.Join(tmp, "report")
	writeFile(t, filepath.Join(report, "index.html"), "root-fresh-this-run")

	const maxAttempts = "3"
	cmd := exec.Command("sh", pagesPersistScript(t), site, report, "root", maxAttempts)
	cmd.Env = gitEnv(home)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected ci/pages-persist.sh to fail once its retry bound is exhausted, but it exited 0:\n%s", out)
	}
	if !strings.Contains(string(out), "after 3 attempt") {
		t.Errorf("expected the script to report exhausting all %s attempts, got:\n%s", maxAttempts, out)
	}
}
