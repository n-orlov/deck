package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeModule makes a scan target: a go.mod with the given extra lines
// and one Go file.
func writeModule(t *testing.T, gomodExtra string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/app\n\ngo 1.25.0\n"+gomodExtra)
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeGo writes a stand-in go whose `version` prints out, returning its path.
func fakeGo(t *testing.T, out string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "go")
	writeFile(t, bin, "#!/bin/sh\necho '"+out+"'\n")
	if err := os.Chmod(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// fakeGovulncheck writes a stand-in that records its argv, its working
// directory and GOTOOLCHAIN, prints a report and exits with code.
func fakeGovulncheck(t *testing.T, code int) (bin, log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "log")
	bin = filepath.Join(dir, "govulncheck")
	script := fmt.Sprintf("#!/bin/sh\n{ printf 'argv:%%s\\n' \"$*\"; printf 'pwd:%%s\\n' \"$PWD\"; printf 'toolchain:%%s\\n' \"$GOTOOLCHAIN\"; } > '%s'\necho fake govulncheck report\nexit %d\n", log, code)
	writeFile(t, bin, script)
	if err := os.Chmod(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, log
}

func gvOpts(bin, goBin, target string) govulncheckOptions {
	return govulncheckOptions{Binary: bin, GoBinary: goBin, Target: target}
}

// TestGovulncheckGate_RunsOverModuleWithLocalToolchain: the command the
// criterion names, `govulncheck ./...` in the module root with
// GOTOOLCHAIN=local, even when the caller's environment says otherwise.
func TestGovulncheckGate_RunsOverModuleWithLocalToolchain(t *testing.T) {
	t.Setenv("GOTOOLCHAIN", "auto")
	bin, log := fakeGovulncheck(t, 0)
	target := writeModule(t, "toolchain go1.25.14\n")
	ok, out, err := runGovulncheckGate(gvOpts(bin, fakeGo(t, "go version go1.25.14 linux/amd64"), target))
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v out=%s", ok, err, out)
	}
	raw, _ := os.ReadFile(log)
	got := string(raw)
	resolved, _ := filepath.EvalSymlinks(target)
	for _, want := range []string{"argv:./...\n", "pwd:" + resolved + "\n", "toolchain:local\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("fake govulncheck saw %q, want it to contain %q", got, want)
		}
	}
}

// TestGovulncheckGate_CalledVulnerabilityFails: govulncheck's exit 3
// (a called vulnerability) fails the gate with its report.
func TestGovulncheckGate_CalledVulnerabilityFails(t *testing.T) {
	bin, _ := fakeGovulncheck(t, 3)
	ok, out, err := runGovulncheckGate(gvOpts(bin, fakeGo(t, "go version go1.25.14 linux/amd64"), writeModule(t, "toolchain go1.25.14\n")))
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, want a failing gate", ok, err)
	}
	if !strings.Contains(out, "fake govulncheck report") || !strings.Contains(out, "what to do") {
		t.Errorf("output lacks the report or guidance:\n%s", out)
	}
}

// TestGovulncheckGate_ToolFailureIsError: any exit other than 0/3, or a
// missing binary, is an input error and never a pass.
func TestGovulncheckGate_ToolFailureIsError(t *testing.T) {
	goBin := fakeGo(t, "go version go1.25.14 linux/amd64")
	target := writeModule(t, "toolchain go1.25.14\n")
	bin, _ := fakeGovulncheck(t, 2)
	if ok, _, err := runGovulncheckGate(gvOpts(bin, goBin, target)); err == nil || ok {
		t.Errorf("exit 2: ok=%v err=%v, want an error", ok, err)
	}
	if ok, _, err := runGovulncheckGate(gvOpts("/does/not/exist/govulncheck", goBin, target)); err == nil || ok {
		t.Errorf("missing binary: ok=%v err=%v, want an error", ok, err)
	}
}

// TestGovulncheckGate_OlderGoThanToolchainFails seeds a running go older
// than go.mod's toolchain line and asserts the gate fails -- before
// govulncheck runs at all.
func TestGovulncheckGate_OlderGoThanToolchainFails(t *testing.T) {
	bin, log := fakeGovulncheck(t, 0)
	target := writeModule(t, "toolchain go1.25.14\n")
	for _, running := range []string{"go1.25.13", "go1.24.9", "go1.25rc1", "go1.25"} {
		ok, out, err := runGovulncheckGate(gvOpts(bin, fakeGo(t, "go version "+running+" linux/amd64"), target))
		if err != nil || ok {
			t.Errorf("running %s: ok=%v err=%v, want a failing gate", running, ok, err)
		}
		if !strings.Contains(out, running) || !strings.Contains(out, "go1.25.14") {
			t.Errorf("running %s: output does not name both versions:\n%s", running, out)
		}
	}
	if _, err := os.Stat(log); err == nil {
		t.Error("govulncheck ran although the Go version check had failed")
	}
}

// TestGovulncheckGate_NotOlderGoPasses: equal and newer Go clear the
// version check; with no toolchain line the go line is the minimum.
func TestGovulncheckGate_NotOlderGoPasses(t *testing.T) {
	bin, _ := fakeGovulncheck(t, 0)
	withToolchain := writeModule(t, "toolchain go1.25.14\n")
	noToolchain := writeModule(t, "")
	for name, c := range map[string]struct{ target, running string }{
		"equal":              {withToolchain, "go1.25.14"},
		"newer patch":        {withToolchain, "go1.25.15"},
		"newer minor":        {withToolchain, "go1.26.4"},
		"go line is minimum": {noToolchain, "go1.25.0"},
	} {
		ok, out, err := runGovulncheckGate(gvOpts(bin, fakeGo(t, "go version "+c.running+" linux/amd64"), c.target))
		if err != nil || !ok {
			t.Errorf("%s: ok=%v err=%v out=%s", name, ok, err, out)
		}
	}
	if ok, _, err := runGovulncheckGate(gvOpts(bin, fakeGo(t, "go version go1.24.0 linux/amd64"), noToolchain)); err != nil || ok {
		t.Errorf("older than the go line: ok=%v err=%v, want a failing gate", ok, err)
	}
}

// TestGovulncheckGate_UnreadableGoVersionIsError: output that is not a
// Go version cannot read as "new enough".
func TestGovulncheckGate_UnreadableGoVersionIsError(t *testing.T) {
	bin, _ := fakeGovulncheck(t, 0)
	ok, _, err := runGovulncheckGate(gvOpts(bin, fakeGo(t, "not a go"), writeModule(t, "toolchain go1.25.14\n")))
	if err == nil || ok {
		t.Errorf("ok=%v err=%v, want an error", ok, err)
	}
}

// TestGovulncheckGate_EmptyScanTargetFails: an empty directory, a missing
// one, a tree without go.mod, and a module with no Go files all fail --
// and govulncheck never runs on them.
func TestGovulncheckGate_EmptyScanTargetFails(t *testing.T) {
	noGoMod := t.TempDir()
	writeFile(t, filepath.Join(noGoMod, "main.go"), "package main\n")
	noGoFiles := t.TempDir()
	writeFile(t, filepath.Join(noGoFiles, "go.mod"), "module example.com/x\n\ngo 1.25.0\n")
	writeFile(t, filepath.Join(noGoFiles, "vendor", "v.go"), "package v\n")
	aFile := filepath.Join(t.TempDir(), "file")
	writeFile(t, aFile, "x")
	for name, target := range map[string]string{
		"empty dir":   t.TempDir(),
		"missing":     filepath.Join(t.TempDir(), "nope"),
		"no go.mod":   noGoMod,
		"no go files": noGoFiles,
		"not a dir":   aFile,
	} {
		bin, log := fakeGovulncheck(t, 0)
		ok, out, err := runGovulncheckGate(gvOpts(bin, fakeGo(t, "go version go1.25.14 linux/amd64"), target))
		if ok || err != nil {
			t.Errorf("%s: ok=%v err=%v, want a failing gate (not a pass, not a tooling error)", name, ok, err)
		}
		if !strings.Contains(out, "govulncheck gate:") {
			t.Errorf("%s: output does not explain the failure:\n%s", name, out)
		}
		if _, serr := os.Stat(log); serr == nil {
			t.Errorf("%s: govulncheck ran against an empty target", name)
		}
	}
}

// vulnFixture builds, in a temp dir, a module that vendors example.com/
// vulnlib v0.5.0 and a local vulnerability database (a file:// URL, so
// no network) saying every version below 1.0.0 is affected in Danger.
// The module's main calls Danger when called is true, Safe otherwise.
func vulnFixture(t *testing.T, called bool) (target, dbURL string) {
	t.Helper()
	root := t.TempDir()
	db := filepath.Join(root, "db")
	osv, _ := json.Marshal(map[string]any{
		"schema_version": "1.3.1", "id": "GO-2026-0001",
		"modified": "2026-01-01T00:00:00Z", "published": "2026-01-01T00:00:00Z",
		"aliases": []string{"CVE-2026-0001"}, "summary": "fixture vulnerability in Danger", "details": "fixture",
		"database_specific": map[string]string{"url": "https://pkg.go.dev/vuln/GO-2026-0001"},
		"affected": []any{map[string]any{
			"package": map[string]string{"name": "example.com/vulnlib", "ecosystem": "Go"},
			"ranges": []any{map[string]any{"type": "SEMVER", "events": []any{
				map[string]string{"introduced": "0"}, map[string]string{"fixed": "1.0.0"}}}},
			"ecosystem_specific": map[string]any{"imports": []any{map[string]any{
				"path": "example.com/vulnlib", "symbols": []string{"Danger"}}}},
		}},
	})
	writeFile(t, filepath.Join(db, "index", "db.json"), `{"modified":"2026-01-01T00:00:00Z"}`)
	writeFile(t, filepath.Join(db, "index", "modules.json"), `[{"path":"example.com/vulnlib","vulns":[{"id":"GO-2026-0001","modified":"2026-01-01T00:00:00Z","fixed":"v1.0.0"}]}]`)
	writeFile(t, filepath.Join(db, "index", "vulns.json"), `[{"id":"GO-2026-0001","modified":"2026-01-01T00:00:00Z","aliases":["CVE-2026-0001"]}]`)
	writeFile(t, filepath.Join(db, "ID", "GO-2026-0001.json"), string(osv))

	target = filepath.Join(root, "app")
	writeFile(t, filepath.Join(target, "go.mod"), "module example.com/app\n\ngo 1.25.0\n\nrequire example.com/vulnlib v0.5.0\n")
	writeFile(t, filepath.Join(target, "vendor", "modules.txt"), "# example.com/vulnlib v0.5.0\n## explicit; go 1.25.0\nexample.com/vulnlib\n")
	writeFile(t, filepath.Join(target, "vendor", "example.com", "vulnlib", "lib.go"),
		"package vulnlib\n\nfunc Danger() string { return \"x\" }\n\nfunc Safe() string { return \"y\" }\n")
	call := "Safe"
	if called {
		call = "Danger"
	}
	writeFile(t, filepath.Join(target, "main.go"),
		"package main\n\nimport \"example.com/vulnlib\"\n\nfunc main() { println(vulnlib."+call+"()) }\n")
	return target, "file://" + db
}

// realGovulncheck returns the govulncheck on PATH; the deck-ci image
// installs it, so its absence is a broken environment, not a skip.
func realGovulncheck(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("govulncheck")
	if err != nil {
		t.Fatalf("govulncheck is not on PATH (the deck-ci image installs it): %v", err)
	}
	return bin
}

// TestGovulncheckGate_VulnerableModuleFixtureFails runs the real
// govulncheck against a module that vendors a vulnerable dependency and
// calls the vulnerable function: the gate must fail and name the finding.
func TestGovulncheckGate_VulnerableModuleFixtureFails(t *testing.T) {
	target, db := vulnFixture(t, true)
	o := govulncheckOptions{Binary: realGovulncheck(t), Target: target, DB: db, CacheDir: t.TempDir()}
	ok, out, err := runGovulncheckGate(o)
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, want a failing gate\n%s", ok, err, out)
	}
	for _, want := range []string{"GO-2026-0001", "example.com/vulnlib", "Fixed in"} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q:\n%s", want, out)
		}
	}
}

// TestGovulncheckGate_UncalledVulnerabilityPasses: the gate is on CALLED
// code -- the same vulnerable dependency, with its vulnerable function
// never called, passes.
func TestGovulncheckGate_UncalledVulnerabilityPasses(t *testing.T) {
	target, db := vulnFixture(t, false)
	o := govulncheckOptions{Binary: realGovulncheck(t), Target: target, DB: db, CacheDir: t.TempDir()}
	ok, out, err := runGovulncheckGate(o)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v, want a passing gate\n%s", ok, err, out)
	}
}

// TestGovulncheckGate_RunWiresTheGate drives run() with a seeded config:
// a failing govulncheck turns the whole run to exit 1 with its own
// report section, and a passing one to exit 0.
func TestGovulncheckGate_RunWiresTheGate(t *testing.T) {
	saved := govulncheckBase
	t.Cleanup(func() { govulncheckBase = saved })
	cfgPath := marshalConfig(t, config{Govulncheck: govulncheckConfig{Enabled: true}})
	target := writeModule(t, "toolchain go1.25.14\n")

	for code, wantExit := range map[int]int{0: 0, 3: 1} {
		bin, _ := fakeGovulncheck(t, code)
		govulncheckBase = govulncheckOptions{Binary: bin, GoBinary: fakeGo(t, "go version go1.25.14 linux/amd64"), Target: target}
		report, exit, err := run(cfgPath, "")
		if err != nil || exit != wantExit {
			t.Errorf("govulncheck exit %d: run exit=%d err=%v, want %d", code, exit, err, wantExit)
		}
		if !strings.Contains(report, "=== govulncheck gate ===") {
			t.Errorf("report lacks the govulncheck section:\n%s", report)
		}
	}
}

// TestGovulncheckGate_CheckedInConfigIsOn: the gate is on in
// ci/quality.json, and switching it off is a loosening.
func TestGovulncheckGate_CheckedInConfigIsOn(t *testing.T) {
	cfg, err := loadConfig(filepath.Join(repoRoot(t), "ci", "quality.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Govulncheck.Enabled {
		t.Error("govulncheck.enabled is false in ci/quality.json, want true")
	}
	off := cfg
	off.Govulncheck.Enabled = false
	if len(looserThresholds(cfg, off)) == 0 {
		t.Error("switching govulncheck off is not reported as a loosening")
	}
}

func TestGoModDirectives(t *testing.T) {
	cases := []struct {
		name, gomod, toolchain, goLine string
	}{
		{"both", "module m\n\ngo 1.25.0\ntoolchain go1.25.4\n", "go1.25.4", "go1.25.0"},
		{"go line only", "module m\ngo 1.24\n", "", "go1.24"},
		{"go line already prefixed", "go go1.24\n", "", "go1.24"},
		{"neither", "module m\nrequire x v1\n", "", ""},
		{"one-field and long lines are ignored", "go\ntoolchain\ngo 1.25 // c\n", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tc, goLine, err := goModDirectives(strings.NewReader(c.gomod))
			if err != nil || tc != c.toolchain || goLine != c.goLine {
				t.Fatalf("goModDirectives = (%q, %q, %v), want (%q, %q, nil)", tc, goLine, err, c.toolchain, c.goLine)
			}
		})
	}
}

func TestGoModMinimumPrefersToolchainThenGoLineAndRejectsBadInput(t *testing.T) {
	dir := func(gomod string) string {
		d := t.TempDir()
		writeFile(t, filepath.Join(d, "go.mod"), gomod)
		return d
	}
	if got, err := goModMinimum(dir("module m\ngo 1.25.0\ntoolchain go1.25.4\n")); err != nil || got != "go1.25.4" {
		t.Errorf("toolchain line: got (%q, %v), want go1.25.4", got, err)
	}
	if got, err := goModMinimum(dir("module m\ngo 1.25.0\n")); err != nil || got != "go1.25.0" {
		t.Errorf("go line fallback: got (%q, %v), want go1.25.0", got, err)
	}
	if _, err := goModMinimum(dir("module m\n")); err == nil || !strings.Contains(err.Error(), "neither a toolchain nor a go line") {
		t.Errorf("no directive: err = %v", err)
	}
	if _, err := goModMinimum(dir("module m\ntoolchain banana\n")); err == nil || !strings.Contains(err.Error(), "not a valid Go version") {
		t.Errorf("bad version: err = %v", err)
	}
	if _, err := goModMinimum(t.TempDir()); err == nil {
		t.Error("missing go.mod: want an error")
	}
}

func TestSkippedScanDir(t *testing.T) {
	for name, want := range map[string]bool{
		"vendor": true, "testdata": true, ".git": true, ".hidden": true,
		"internal": false, "cmd": false, "vendored": false, "data": false,
	} {
		if got := skippedScanDir(name); got != want {
			t.Errorf("skippedScanDir(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestCountGoFilesSkipsVendorTestdataAndHiddenBelowTheTarget(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"a.go", "pkg/b.go", "vendor/c.go", "pkg/testdata/d.go", ".hidden/e.go", "notes.txt"} {
		writeFile(t, filepath.Join(dir, f), "package x\n")
	}
	if got := countGoFiles(dir); got != 2 {
		t.Fatalf("countGoFiles = %d, want 2 (a.go, pkg/b.go)", got)
	}
	// The target itself may be a hidden or vendor-named directory.
	hidden := filepath.Join(t.TempDir(), ".target")
	writeFile(t, filepath.Join(hidden, "a.go"), "package x\n")
	if got := countGoFiles(hidden); got != 1 {
		t.Fatalf("countGoFiles(hidden target) = %d, want 1", got)
	}
	if got := countGoFiles(filepath.Join(dir, "missing")); got != 0 {
		t.Fatalf("countGoFiles(missing) = %d, want 0", got)
	}
}

func TestGovulncheckCommandBuildsTheInvocationFromTheOptions(t *testing.T) {
	def := govulncheckCommand(govulncheckOptions{Target: "/t"})
	if def.Args[0] != "govulncheck" || strings.Join(def.Args[1:], " ") != "./..." || def.Dir != "/t" {
		t.Errorf("defaults: args=%q dir=%q", def.Args, def.Dir)
	}
	if !containsEnv(def.Env, "GOTOOLCHAIN=local") {
		t.Error("defaults: GOTOOLCHAIN=local missing from the environment")
	}
	full := govulncheckCommand(govulncheckOptions{Binary: "/bin/gv", Target: "/t", DB: "file:///db", CacheDir: "/cache"})
	if full.Args[0] != "/bin/gv" || strings.Join(full.Args[1:], " ") != "-db file:///db ./..." {
		t.Errorf("overrides: args=%q", full.Args)
	}
	if !containsEnv(full.Env, "XDG_CACHE_HOME=/cache") || !containsEnv(full.Env, "GOTOOLCHAIN=local") {
		t.Errorf("overrides: env lacks the cache dir or the local toolchain")
	}
}

func containsEnv(env []string, kv string) bool {
	for _, e := range env {
		if e == kv {
			return true
		}
	}
	return false
}
