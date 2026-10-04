package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// moduleWith builds a small module (module path example.com/m) under a temp
// dir with the given files (relative path -> content) and returns its root.
func moduleWith(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/m\n\ngo 1.25\n")
	for rel, content := range files {
		writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), content)
	}
	return root
}

func TestParseFilter(t *testing.T) {
	cases := []struct {
		in    string
		dir   string
		files []string
		whole bool
	}{
		{"", "", nil, true},
		{"internal/x", "internal/x", nil, false},
		{"internal/x:a.go, b.go,,", "internal/x", []string{"a.go", "b.go"}, false},
		{"internal/x:", "internal/x", nil, false},
	}
	for _, c := range cases {
		dir, files, whole := parseFilter(c.in)
		if dir != c.dir || !reflect.DeepEqual(files, c.files) || whole != c.whole {
			t.Errorf("parseFilter(%q) = (%q, %v, %v), want (%q, %v, %v)", c.in, dir, files, whole, c.dir, c.files, c.whole)
		}
	}
}

func TestSkipDirName(t *testing.T) {
	for name, want := range map[string]bool{".git": true, "vendor": true, ".hidden": true, "internal": false, "cmd": false} {
		if got := skipDirName(name); got != want {
			t.Errorf("skipDirName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestFindModuleErrors(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "go 1.25\n")
	t.Chdir(root)
	if _, _, err := findModule(); err == nil || !strings.Contains(err.Error(), "no module line") {
		t.Errorf("findModule with a module-less go.mod: err = %v, want 'no module line'", err)
	}
	if _, err := filterRoot(""); err == nil {
		t.Error("filterRoot must surface findModule's error")
	}
	if _, _, err := run("whatever.out", 10, ""); err == nil {
		t.Error("run over a missing profile must fail")
	}
}

func TestFilterRootResolvesPackageDirectory(t *testing.T) {
	root := moduleWith(t, nil)
	t.Chdir(root)
	whole, err := filterRoot("")
	if err != nil || whole != root {
		t.Fatalf("filterRoot(\"\") = (%q, %v), want %q", whole, err, root)
	}
	sub, err := filterRoot("internal/x:a.go")
	if err != nil || sub != filepath.Join(root, "internal", "x") {
		t.Fatalf("filterRoot(dir:file) = (%q, %v)", sub, err)
	}
}

func TestResolveSourceFilesWholeTreeSkipsTestsAndHiddenDirs(t *testing.T) {
	root := moduleWith(t, map[string]string{
		"a/a.go": "package a\n", "a/a_test.go": "package a\n", ".hidden/h.go": "package h\n",
		"vendor/v/v.go": "package v\n", "b/c/c.go": "package c\n", "README.md": "x",
	})
	got, err := resolveSourceFiles(root, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, "a", "a.go"), filepath.Join(root, "b", "c", "c.go")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveSourceFiles = %v, want %v", got, want)
	}
}

func TestResolveSourceFilesFilterForms(t *testing.T) {
	root := moduleWith(t, map[string]string{
		"a/a.go": "package a\n", "a/b.go": "package a\n", "a/a_test.go": "package a\n",
		"a/sub/s.go": "package s\n", "file.txt": "x",
	})
	// Bare directory: its own non-test .go files only, not subdirectories.
	got, err := resolveSourceFiles(root, "a")
	if err != nil || len(got) != 2 || filepath.Base(got[0]) != "a.go" || filepath.Base(got[1]) != "b.go" {
		t.Fatalf("bare dir filter = (%v, %v), want a.go and b.go", got, err)
	}
	// Explicit file list.
	got, err = resolveSourceFiles(root, "a:b.go")
	if err != nil || len(got) != 1 || filepath.Base(got[0]) != "b.go" {
		t.Fatalf("explicit file filter = (%v, %v), want only b.go", got, err)
	}
	// A typo'd directory, a file given as the directory, and a typo'd file are errors.
	for _, bad := range []string{"nope", "file.txt", "a:missing.go"} {
		if _, err := resolveSourceFiles(root, bad); err == nil {
			t.Errorf("resolveSourceFiles(%q) = nil error, want an error", bad)
		}
	}
}

func TestScanFunctionsReportsUnparsableSource(t *testing.T) {
	root := moduleWith(t, map[string]string{"a/a.go": "package a\nfunc {\n"})
	if _, err := scanFunctions(root, ""); err == nil || !strings.Contains(err.Error(), "parsing") {
		t.Fatalf("scanFunctions over a syntax error: err = %v, want a 'parsing' error", err)
	}
	if _, err := scanFunctions(root, "missing"); err == nil {
		t.Fatal("scanFunctions with a typo'd filter must fail")
	}
}

func TestFuncDisplayNameAndBodylessFunctions(t *testing.T) {
	root := moduleWith(t, map[string]string{"a/a.go": `package a

type T struct{}

func Plain() {}
func (T) Value() {}
func (*T) Pointer() {}
func Stub(x int) int

type G[X any] struct{}
`})
	fns, err := scanFunctions(root, "")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range fns {
		names = append(names, f.Name)
	}
	for _, n := range []string{"Plain", "(T).Value", "(*T).Pointer"} {
		found := false
		for _, got := range names {
			if got == n {
				found = true
			}
		}
		if !found {
			t.Errorf("function %q not scanned; got %v", n, names)
		}
	}
	for _, n := range names {
		if n == "Stub" {
			t.Error("a body-less declaration must not be scanned")
		}
	}
}

func TestProfileKeyFallsBackToBareRelativeWithoutModulePath(t *testing.T) {
	if got := profileKey("/r/a/b.go", "example.com/m", "/r"); got != "example.com/m/a/b.go" {
		t.Errorf("profileKey = %q", got)
	}
	if got := profileKey("/r/a/b.go", "", "/r"); got != "a/b.go" {
		t.Errorf("profileKey without a module path = %q, want a/b.go", got)
	}
	if got := profileKey("relative.go", "m", "/abs/root"); got != "relative.go" {
		t.Errorf("profileKey with an unrelatable path = %q, want the path verbatim", got)
	}
}

func TestParseProfileRejectsMalformedInput(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"no mode line":    "example.com/m/a.go:1.1,2.2 1 1\n",
		"no colon":        "mode: set\nnocolon 1 1\n",
		"wrong fields":    "mode: set\na.go:1.1,2.2 1\n",
		"no comma":        "mode: set\na.go:1.1 1 1\n",
		"bad start":       "mode: set\na.go:x.1,2.2 1 1\n",
		"bad end":         "mode: set\na.go:1.1,2 1 1\n",
		"bad stmts":       "mode: set\na.go:1.1,2.2 x 1\n",
		"bad count":       "mode: set\na.go:1.1,2.2 1 x\n",
		"bad end number":  "mode: set\na.go:1.1,y.2 1 1\n",
		"blank only file": "\n\n",
	} {
		path := filepath.Join(dir, "p.out")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := parseProfile(path); err == nil {
			t.Errorf("%s: parseProfile = nil error, want an error", name)
		}
	}
}

func TestRunOutputsReportOverMinimalModule(t *testing.T) {
	root := moduleWith(t, map[string]string{"a/a.go": "package a\n\nfunc F(x int) int {\n\tif x > 0 {\n\t\treturn 1\n\t}\n\treturn 0\n}\n"})
	prof := filepath.Join(root, "p.out")
	writeFile(t, prof, "mode: set\nexample.com/m/a/a.go:3.22,4.10 1 1\nexample.com/m/a/a.go:4.10,6.3 1 1\nexample.com/m/a/a.go:7.2,7.10 1 1\n")
	t.Chdir(root)
	report, code, err := run(prof, 5, "")
	if err != nil || code != 0 || !strings.Contains(report, "all functions at or under the CRAP ceiling") {
		t.Fatalf("run = (%q, %d, %v), want a clean pass", report, code, err)
	}
	report, code, err = run(prof, 0, "a")
	if err != nil || code != 1 || !strings.Contains(report, "a.go:3: F cc=2") {
		t.Fatalf("run with ceiling 0 = (%q, %d, %v), want exit 1 naming F", report, code, err)
	}
	if _, code, err := run("", 5, ""); err == nil || code != 2 {
		t.Errorf("run without -profile = (%d, %v), want exit 2", code, err)
	}
	if _, code, err := run(prof, 5, "typo"); err == nil || code != 2 {
		t.Errorf("run with a typo'd filter = (%d, %v), want exit 2", code, err)
	}
	empty := moduleWith(t, nil)
	t.Chdir(empty)
	if _, code, err := run(prof, 5, ""); err == nil || code != 2 || !strings.Contains(err.Error(), "zero scored functions") {
		t.Errorf("run over a module with no functions = (%d, %v), want exit 2 'zero scored functions'", code, err)
	}
}
