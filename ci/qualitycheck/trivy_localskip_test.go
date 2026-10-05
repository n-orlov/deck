package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitTree makes a scan target that is a git work tree holding:
// a tracked go.mod and main.go; a tracked file that matches an ignore
// pattern (force-added); an untracked, NOT ignored file; and untracked,
// ignored leftovers -- a whole directory (.scratch/, with a go.mod of its
// own) and a single file (local.env).
func gitTree(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("git is required by the trivy gate's local-skip rule: %v", err)
	}
	dir := scanTree(t)
	files := map[string]string{
		".gitignore":          ".scratch/\nlocal.env\nforced.lock\n",
		"forced.lock":         "tracked despite the pattern\n",
		"new.go":              "package main\n",
		".scratch/go.mod":     "module scratch\n",
		".scratch/deep/a.txt": "x\n",
		"local.env":           "X=1\n",
	}
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, argv := range [][]string{
		{"init", "-q"},
		{"add", "go.mod", "main.go", ".gitignore"},
		{"add", "-f", "forced.lock"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, argv...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", argv, err, out)
		}
	}
	return dir
}

// TestTrivyGate_SkipsOnlyUntrackedGitignoredPaths: inside a git work tree
// the gate skips exactly what git reports as untracked AND ignored (a
// developer's local leftovers, absent from a clean checkout) and still
// scans every tracked file -- even one matching an ignore pattern -- and
// every untracked file git would offer to commit.
func TestTrivyGate_SkipsOnlyUntrackedGitignoredPaths(t *testing.T) {
	bin, argvLog := fakeTrivy(t, 0)
	o := opts(t, bin, gitTree(t))
	if ok, out, err := runTrivyGate(o); err != nil || !ok {
		t.Fatalf("ok=%v err=%v out=%s", ok, err, out)
	}
	raw, _ := os.ReadFile(argvLog)
	joined := strings.Join(strings.Split(strings.TrimSpace(string(raw)), "\n"), " ")
	for _, want := range []string{"--skip-dirs .scratch ", "--skip-files local.env "} {
		if !strings.Contains(joined+" ", want) {
			t.Errorf("argv %q lacks %q", joined, want)
		}
	}
	for _, scanned := range []string{"forced.lock", "new.go", "main.go", "go.mod", ".gitignore"} {
		if strings.Contains(" "+joined+" ", " "+scanned+" ") {
			t.Errorf("argv %q skips %q, which the repository holds or would commit", joined, scanned)
		}
	}
}

// TestTrivyGate_OutsideAGitWorkTreeScansEverything: with no git work tree
// there is nothing git calls ignored, so the gate adds no skip beyond its
// fixed, commented list -- the fallback is the stricter scan.
func TestTrivyGate_OutsideAGitWorkTreeScansEverything(t *testing.T) {
	dir := scanTree(t)
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree").CombinedOutput(); err == nil {
		t.Fatalf("temp dir %s is inside a git work tree (%s); the non-git case needs a TMPDIR outside any repository", dir, out)
	}
	if got := localSkipArgs(dir); len(got) != 0 {
		t.Errorf("localSkipArgs outside a work tree = %q, want none", got)
	}
	got := len(trivyArgs(opts(t, "trivy", dir)))
	if want := 2*(len(trivySkipFiles)+len(trivySkipDirs)) + 12; got != want {
		t.Errorf("trivy argv has %d entries, want %d (fixed flags + fixed skips + target only)", got, want)
	}
}

// TestTrivyGlobEscaper: a local path with glob metacharacters is skipped
// literally, never as a wider pattern.
func TestTrivyGlobEscaper(t *testing.T) {
	for in, want := range map[string]string{
		".spike-preview": ".spike-preview",
		"a*b":            `a\*b`,
		"x?[y]{z}":       `x\?\[y]\{z}`,
		`back\slash`:     `back\\slash`,
	} {
		if got := trivyGlobEscaper.Replace(in); got != want {
			t.Errorf("escape(%q) = %q, want %q", in, got, want)
		}
	}
}
