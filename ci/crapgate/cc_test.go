package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// parseFirstFunc parses src as a complete Go file and returns its first
// *ast.FuncDecl -- the locked-cc-rules table test's one piece of shared
// scaffolding.
func parseFirstFunc(t *testing.T, src string) *ast.FuncDecl {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "snippet.go", src, 0)
	if err != nil {
		t.Fatalf("parsing snippet: %v\n%s", err, src)
	}
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok {
			return fd
		}
	}
	t.Fatalf("snippet has no top-level func decl:\n%s", src)
	return nil
}

// TestCyclomaticComplexity_LockedRules pins the exact gocyclo-style rule
// set PRD R187 names: cc = 1 + if + for + range + case + comm-case +
// && + ||, a default/select-default clause adding nothing, and a
// closure's branches counting into the enclosing function.
func TestCyclomaticComplexity_LockedRules(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "empty body",
			src:  `package p; func F() {}`,
			want: 1,
		},
		{
			name: "single if",
			src:  `package p; func F() { if true { _ = 1 } }`,
			want: 2,
		},
		{
			name: "if-else adds only for the if, not the else",
			src:  `package p; func F() { if true { _ = 1 } else { _ = 2 } }`,
			want: 2,
		},
		{
			name: "else-if is a nested if, counted separately",
			src:  `package p; func F(x int) { if x == 1 { _ = 1 } else if x == 2 { _ = 2 } else { _ = 3 } }`,
			want: 3,
		},
		{
			name: "for loop",
			src:  `package p; func F() { for { break } }`,
			want: 2,
		},
		{
			name: "for with condition is still just +1",
			src:  `package p; func F(x int) { for x > 0 { x-- } }`,
			want: 2,
		},
		{
			name: "range",
			src:  `package p; func F(xs []int) { for range xs { _ = 1 } }`,
			want: 2,
		},
		{
			name: "switch with two cases and a default: default adds nothing",
			src: `package p; func F(x int) {
				switch x {
				case 1:
					_ = 1
				case 2:
					_ = 2
				default:
					_ = 3
				}
			}`,
			want: 3, // base 1 + case 1 + case 2 (+0 for default)
		},
		{
			name: "switch with only a default clause adds nothing beyond base",
			src: `package p; func F(x int) {
				switch x {
				default:
					_ = 1
				}
			}`,
			want: 1,
		},
		{
			name: "select with two comm-clauses and a default: default adds nothing",
			src: `package p; func F(a, b chan int) {
				select {
				case <-a:
					_ = 1
				case <-b:
					_ = 2
				default:
					_ = 3
				}
			}`,
			want: 3, // base 1 + comm-case + comm-case (+0 for default)
		},
		{
			name: "logical AND adds one",
			src:  `package p; func F(a, b bool) { if a && b { _ = 1 } }`,
			want: 3, // base 1 + if + &&
		},
		{
			name: "logical OR adds one",
			src:  `package p; func F(a, b bool) { if a || b { _ = 1 } }`,
			want: 3, // base 1 + if + ||
		},
		{
			name: "chained && and || each add one",
			src:  `package p; func F(a, b, c bool) { if a && b || c { _ = 1 } }`,
			want: 4, // base 1 + if + && + ||
		},
		{
			name: "closure's branches count into the enclosing function",
			src: `package p; func F() {
				g := func() {
					if true {
						_ = 1
					}
				}
				g()
			}`,
			want: 2, // base 1 + the if INSIDE the closure
		},
		{
			name: "a closure's own branches add on top of the enclosing function's own if",
			src: `package p; func F(x int) {
				if x > 0 {
					_ = 1
				}
				g := func() {
					for range []int{} {
						_ = 2
					}
				}
				g()
			}`,
			want: 3, // base 1 + outer if + closure's range
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fd := parseFirstFunc(t, tc.src)
			got := cyclomaticComplexity(fd)
			if got != tc.want {
				t.Errorf("cyclomaticComplexity(%s) = %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}

// TestCyclomaticComplexity_ClosureNeverScoredSeparately asserts the other
// half of "closures counted into the enclosing func": scanFunctions must
// never emit a second, standalone score for the closure itself -- only
// the one named FuncDecl, F, is ever a scannedFunc.
func TestCyclomaticComplexity_ClosureNeverScoredSeparately(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/go.mod", "module example.com/closuretest\n\ngo 1.21\n")
	writeFile(t, dir+"/pkg.go", `package closuretest

func F() {
	g := func() {
		if true {
			_ = 1
		}
	}
	g()
}
`)
	fns, err := scanFunctions(dir, "")
	if err != nil {
		t.Fatalf("scanFunctions: %v", err)
	}
	if len(fns) != 1 {
		names := make([]string, len(fns))
		for i, f := range fns {
			names[i] = f.Name
		}
		t.Fatalf("scanFunctions found %d functions %v, want exactly 1 (F) -- the closure must not be scored on its own", len(fns), names)
	}
	if fns[0].Name != "F" {
		t.Errorf("scanFunctions found %q, want %q", fns[0].Name, "F")
	}
}
