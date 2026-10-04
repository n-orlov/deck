package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testModule = "example.com/m"

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// pkgCover is one seeded package: covered and total statements.
type pkgCover struct {
	dir            string
	covered, total int
}

// seed builds a module with a Go file in every named directory (plus the
// zero-statement packages, which have source but no profile blocks) and a
// profile with one covered and one uncovered block per package, so each
// package scores exactly covered/total.
func seed(t *testing.T, floors string, pkgs ...pkgCover) (cfgPath, profPath, root string) {
	t.Helper()
	root = t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module "+testModule+"\n\ngo 1.25\n")
	var prof strings.Builder
	prof.WriteString("mode: set\n")
	for _, p := range pkgs {
		writeFile(t, filepath.Join(root, p.dir, "x.go"), "package x\n")
		if p.covered > 0 {
			fmt.Fprintf(&prof, "%s/%s/x.go:1.1,2.2 %d 1\n", testModule, p.dir, p.covered)
		}
		if p.total-p.covered > 0 {
			fmt.Fprintf(&prof, "%s/%s/x.go:3.1,4.2 %d 0\n", testModule, p.dir, p.total-p.covered)
		}
	}
	for _, z := range zeroStatementPackages {
		writeFile(t, filepath.Join(root, z, "doc.go"), "package z\n")
	}
	cfgPath = filepath.Join(root, "quality.json")
	writeFile(t, cfgPath, `{"coverage": {"enabled": true, `+floors+`}}`)
	profPath = filepath.Join(root, "cover.out")
	writeFile(t, profPath, prof.String())
	return cfgPath, profPath, root
}

const floors = `"total_floor": 85, "package_floor": 80, "fixture_floor": 50`

func TestPassingProfileExitsZero(t *testing.T) {
	cfg, prof, root := seed(t, floors,
		pkgCover{"internal/a", 90, 100}, pkgCover{"internal/b", 86, 100}, pkgCover{"cmd/fake-x", 60, 100})
	report, code, err := run(cfg, prof, root)
	if err != nil || code != 0 {
		t.Fatalf("run = (%d, %v), want exit 0:\n%s", code, err, report)
	}
	for _, want := range []string{"coverage gate passed", "internal/racebuild", "named in the zero-statement list", "product total"} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}
}

func TestSeeded79PercentPackageFails(t *testing.T) {
	cfg, prof, root := seed(t, floors,
		pkgCover{"internal/a", 99, 100}, pkgCover{"internal/low", 79, 100}, pkgCover{"internal/big", 990, 1000})
	report, code, err := run(cfg, prof, root)
	if err != nil || code != 1 {
		t.Fatalf("run = (%d, %v), want exit 1 (a 79%% package):\n%s", code, err, report)
	}
	if !strings.Contains(report, "internal/low: 79.0% is below the package floor of 80%") {
		t.Errorf("report does not name the 79%% package:\n%s", report)
	}
	if strings.Contains(report, "product total: ") {
		t.Errorf("the total is fine here and must not be named as failing:\n%s", report)
	}
}

func TestSeeded84PercentTotalFails(t *testing.T) {
	// Every package is above the 80% package floor; only the total is short.
	cfg, prof, root := seed(t, floors,
		pkgCover{"internal/a", 84, 100}, pkgCover{"internal/b", 840, 1000})
	report, code, err := run(cfg, prof, root)
	if err != nil || code != 1 {
		t.Fatalf("run = (%d, %v), want exit 1 (an 84%% total):\n%s", code, err, report)
	}
	if !strings.Contains(report, "product total: 84.0% is below the total floor of 85%") {
		t.Errorf("report does not name the 84%% total:\n%s", report)
	}
	if strings.Contains(report, "below the package floor") {
		t.Errorf("no package is short here:\n%s", report)
	}
}

func TestExactlyAtFloorsPasses(t *testing.T) {
	cfg, prof, root := seed(t, floors, pkgCover{"internal/a", 85, 100})
	if report, code, err := run(cfg, prof, root); err != nil || code != 0 {
		t.Fatalf("run = (%d, %v), want exit 0 at exactly the floors:\n%s", code, err, report)
	}
}

func TestEmptyProfileFails(t *testing.T) {
	cfg, prof, root := seed(t, floors, pkgCover{"internal/a", 90, 100})
	for name, content := range map[string]string{
		"empty file":    "",
		"mode only":     "mode: set\n",
		"no mode line":  "example.com/m/internal/a/x.go:1.1,2.2 1 1\n",
		"malformed row": "mode: set\nnot-a-block\n",
	} {
		writeFile(t, prof, content)
		report, code, err := run(cfg, prof, root)
		if err == nil || code != 2 {
			t.Errorf("%s: run = (%d, %v), want exit 2 and an error:\n%s", name, code, err, report)
		}
	}
	if _, code, err := run(cfg, filepath.Join(root, "missing.out"), root); err == nil || code != 2 {
		t.Errorf("missing profile: run = (%d, %v), want exit 2 and an error", code, err)
	}
}

func TestOnlyFixturesInProfileFails(t *testing.T) {
	cfg, prof, root := seed(t, floors, pkgCover{"cmd/fake-x", 100, 100})
	if _, code, err := run(cfg, prof, root); err == nil || code != 2 {
		t.Fatalf("run = (%d, %v), want exit 2: a profile with no product statement is not a pass", code, err)
	}
}

func TestFixtureHasItsOwnFloor(t *testing.T) {
	cfg, prof, root := seed(t, floors, pkgCover{"internal/a", 95, 100}, pkgCover{"cmd/fake-x", 49, 100})
	report, code, _ := run(cfg, prof, root)
	if code != 1 || !strings.Contains(report, "cmd/fake-x: 49.0% is below the fixture floor of 50%") {
		t.Fatalf("run exit %d, want 1 naming the fixture floor:\n%s", code, report)
	}
	// A fixture at 60% passes the fixture floor although it is under the
	// product package floor, and does not drag the product total down.
	cfg, prof, root = seed(t, floors, pkgCover{"internal/a", 95, 100}, pkgCover{"cmd/fake-x", 60, 100})
	if report, code, err := run(cfg, prof, root); err != nil || code != 0 {
		t.Fatalf("run = (%d, %v), want exit 0:\n%s", code, err, report)
	}
}

func TestPackageMissingFromProfileFails(t *testing.T) {
	cfg, prof, root := seed(t, floors, pkgCover{"internal/a", 95, 100})
	writeFile(t, filepath.Join(root, "internal", "never", "n.go"), "package never\n")
	report, code, _ := run(cfg, prof, root)
	if code != 1 || !strings.Contains(report, "internal/never: zero statements in the profile and not named in the zero-statement list") {
		t.Fatalf("run exit %d, want 1 naming internal/never:\n%s", code, report)
	}
}

func TestZeroStatementListIsExactPathAndNotStale(t *testing.T) {
	// A package that merely sits beside a listed one is not excused.
	cfg, prof, root := seed(t, floors, pkgCover{"internal/a", 95, 100})
	writeFile(t, filepath.Join(root, "internal", "racebuild2", "d.go"), "package d\n")
	report, code, _ := run(cfg, prof, root)
	if code != 1 || !strings.Contains(report, "internal/racebuild2: zero statements") {
		t.Fatalf("a neighbour of a listed path was excused (exit %d):\n%s", code, report)
	}

	// A listed package that has statements in the profile is a stale entry.
	cfg, prof, root = seed(t, floors, pkgCover{"internal/a", 95, 100}, pkgCover{"internal/racebuild", 10, 10})
	report, code, _ = run(cfg, prof, root)
	if code != 1 || !strings.Contains(report, "internal/racebuild: named in the zero-statement list but the profile has 10 statements") {
		t.Fatalf("stale entry not reported (exit %d):\n%s", code, report)
	}

	// So is one whose source is gone.
	cfg, prof, root = seed(t, floors, pkgCover{"internal/a", 95, 100})
	if err := os.RemoveAll(filepath.Join(root, "internal", "racebuild")); err != nil {
		t.Fatal(err)
	}
	report, code, _ = run(cfg, prof, root)
	if code != 1 || !strings.Contains(report, "has no Go source on disk") {
		t.Fatalf("vanished entry not reported (exit %d):\n%s", code, report)
	}
}

func TestZeroStatementListNamesRacebuildByExactPath(t *testing.T) {
	found := false
	for _, z := range zeroStatementPackages {
		if strings.ContainsAny(z, "*?[") {
			t.Errorf("zero-statement entry %q is a glob; entries are exact paths", z)
		}
		if z == "internal/racebuild" {
			found = true
		}
	}
	if !found {
		t.Error("internal/racebuild is not named in zeroStatementPackages")
	}
}

func TestTightenPromptWhenBeatingFloorByOnePoint(t *testing.T) {
	cfg, prof, root := seed(t, floors,
		pkgCover{"internal/tight", 80, 100}, pkgCover{"internal/near", 809, 1000}, pkgCover{"internal/far", 9500, 10000})
	report, code, err := run(cfg, prof, root)
	if err != nil || code != 0 {
		t.Fatalf("run = (%d, %v):\n%s", code, err, report)
	}
	if !strings.Contains(report, "tighten: internal/far is at 95.0%") {
		t.Errorf("no tighten prompt for the package 15 pp above its floor:\n%s", report)
	}
	if strings.Contains(report, "tighten: internal/tight") || strings.Contains(report, "tighten: internal/near") {
		t.Errorf("tighten prompt for a package under 1 pp above its floor:\n%s", report)
	}
}

func TestNoTightenPromptWhenFloorsAreTight(t *testing.T) {
	cfg, prof, root := seed(t, `"total_floor": 85, "package_floor": 85, "fixture_floor": 50`, pkgCover{"internal/a", 85, 100})
	report, _, _ := run(cfg, prof, root)
	if strings.Contains(report, "tighten") {
		t.Errorf("unexpected tighten prompt:\n%s", report)
	}
}

func TestBadConfigIsUsageError(t *testing.T) {
	_, prof, root := seed(t, floors, pkgCover{"internal/a", 95, 100})
	bad := map[string]string{
		"zero package floor": `"total_floor": 85, "package_floor": 0, "fixture_floor": 50`,
		"over 100":           `"total_floor": 101, "package_floor": 80, "fixture_floor": 50`,
		"missing fixture":    `"total_floor": 85, "package_floor": 80`,
	}
	for name, fl := range bad {
		cfgPath := filepath.Join(root, "bad.json")
		writeFile(t, cfgPath, `{"coverage": {`+fl+`}}`)
		if _, code, err := run(cfgPath, prof, root); err == nil || code != 2 {
			t.Errorf("%s: run = (%d, %v), want exit 2", name, code, err)
		}
	}
	for name, content := range map[string]string{
		"no coverage object": `{"crap": {}}`,
		"not json":           `nope`,
	} {
		cfgPath := filepath.Join(root, "bad.json")
		writeFile(t, cfgPath, content)
		if _, code, err := run(cfgPath, prof, root); err == nil || code != 2 {
			t.Errorf("%s: run = (%d, %v), want exit 2", name, code, err)
		}
	}
	if _, code, err := run(filepath.Join(root, "absent.json"), prof, root); err == nil || code != 2 {
		t.Errorf("absent config: run = (%d, %v), want exit 2", code, err)
	}
	if _, code, err := run("", prof, root); err == nil || code != 2 {
		t.Errorf("no -config: run = (%d, %v), want exit 2", code, err)
	}
	if _, code, err := run(filepath.Join(root, "quality.json"), "", root); err == nil || code != 2 {
		t.Errorf("no -profile: run = (%d, %v), want exit 2", code, err)
	}
}

func TestProfileOutsideModuleIsUsageError(t *testing.T) {
	cfg, prof, root := seed(t, floors, pkgCover{"internal/a", 95, 100})
	writeFile(t, prof, "mode: set\nother.org/x/y.go:1.1,2.2 1 1\n")
	if _, code, err := run(cfg, prof, root); err == nil || code != 2 {
		t.Fatalf("run = (%d, %v), want exit 2 for a file outside the module", code, err)
	}
}

func TestDuplicateBlocksCountOnceAndCoveredWins(t *testing.T) {
	cfg, prof, root := seed(t, floors, pkgCover{"internal/a", 90, 100})
	// The same uncovered block again, and the same covered block with a
	// zero count: neither may change the 90/100 tally except that a
	// covered duplicate of the uncovered block raises it.
	extra := "example.com/m/internal/a/x.go:1.1,2.2 90 0\n" +
		"example.com/m/internal/a/x.go:3.1,4.2 10 1\n"
	f, err := os.OpenFile(prof, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(extra); err != nil {
		t.Fatal(err)
	}
	f.Close()
	report, code, err := run(cfg, prof, root)
	if err != nil || code != 0 || !strings.Contains(report, "internal/a") || !strings.Contains(report, "100.0%") {
		t.Fatalf("run = (%d, %v), want exit 0 with internal/a at 100.0%%:\n%s", code, err, report)
	}
}

func TestSourcePackagesSkipsTestdataHiddenAndNestedModules(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module "+testModule+"\n")
	writeFile(t, filepath.Join(root, "a", "a.go"), "package a\n")
	writeFile(t, filepath.Join(root, "a", "a_test.go"), "package a\n")
	writeFile(t, filepath.Join(root, "onlytests", "t_test.go"), "package t\n")
	writeFile(t, filepath.Join(root, "a", "testdata", "x.go"), "package x\n")
	writeFile(t, filepath.Join(root, ".hidden", "h.go"), "package h\n")
	writeFile(t, filepath.Join(root, "_skip", "s.go"), "package s\n")
	writeFile(t, filepath.Join(root, "vendor", "v", "v.go"), "package v\n")
	writeFile(t, filepath.Join(root, "ci-results", "r.go"), "package r\n")
	writeFile(t, filepath.Join(root, "nested", "go.mod"), "module other\n")
	writeFile(t, filepath.Join(root, "nested", "n.go"), "package n\n")
	writeFile(t, filepath.Join(root, "main.go"), "package main\n")
	got, err := sourcePackages(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got["a"] || !got["."] {
		t.Fatalf("sourcePackages = %v, want exactly {a, .}", got)
	}
}

func TestFindModule(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module \"quoted.example/m\"\n")
	sub := filepath.Join(root, "x", "y")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	gotRoot, gotPath, err := findModule(sub)
	if err != nil || gotRoot != root || gotPath != "quoted.example/m" {
		t.Fatalf("findModule = (%q, %q, %v)", gotRoot, gotPath, err)
	}
	writeFile(t, filepath.Join(root, "x", "go.mod"), "go 1.25\n")
	if _, _, err := findModule(sub); err == nil {
		t.Error("a go.mod with no module line must be an error")
	}
	if _, _, err := findModule(t.TempDir()); err == nil {
		t.Error("no go.mod above the directory must be an error")
	}
}

func TestParseBlockLineRejectsBadRows(t *testing.T) {
	for _, line := range []string{
		"nocolon", "f.go:1.1,2.2 1", "f.go:1.1,2.2 x 1", "f.go:1.1,2.2 1 y", "f.go:1.1,2.2 -1 1", "f.go:1.1,2.2 1 -1",
	} {
		if _, _, err := parseBlockLine(line); err == nil {
			t.Errorf("parseBlockLine(%q) = nil error", line)
		}
	}
}

func TestZeroStatementPackageScoreIsZero(t *testing.T) {
	if got := (pkgScore{}).pct(); got != 0 {
		t.Errorf("pct of an empty package = %v, want 0", got)
	}
}
