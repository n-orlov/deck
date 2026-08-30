package tui

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// themeContrastTestPath is the file that owns R84's dialog contrast floor
// table. This test does NOT keep its own copy of that table: it parses
// the declaration below out of internal/theme's own test source, so the
// floor table and this completeness proof cannot drift apart by hand
// (task 1204's first attempt duplicated the set as a tui-side map, which
// is exactly the drift this avoids).
const themeContrastTestPath = "../theme/contrast_test.go"

// themeFloorTokensVar is the variable in themeContrastTestPath that lists
// every token R84's floor holds against theme.Selection.
const themeFloorTokensVar = "dialogSelectionTokens"

// loadThemeSelectionFloorTokens returns the token names listed by
// internal/theme/contrast_test.go's dialogSelectionTokens -- the actual
// floor table TestThemedDialogTokensClearContrastFloor walks -- read out
// of that file's AST. A missing, renamed or empty declaration is a hard
// failure: this test may never fall back to a locally written set,
// because then a floor table that stopped covering a drawn pair would
// still look complete here.
func loadThemeSelectionFloorTokens(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, themeContrastTestPath, nil, 0)
	if err != nil {
		t.Fatalf("ParseFile(%s): %v -- this test reads R84's floor table out of that file; it cannot verify completeness without it", themeContrastTestPath, err)
	}
	tokens := map[string]bool{}
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range spec.Names {
			if name.Name != themeFloorTokensVar || i >= len(spec.Values) {
				continue
			}
			found = true
			cl, ok := spec.Values[i].(*ast.CompositeLit)
			if !ok {
				t.Fatalf("%s in %s is not a composite literal (%T) -- extend this parser so the floor table stays machine-readable", themeFloorTokensVar, themeContrastTestPath, spec.Values[i])
			}
			for _, elt := range cl.Elts {
				switch e := elt.(type) {
				case *ast.Ident:
					tokens[e.Name] = true
				case *ast.SelectorExpr:
					tokens[e.Sel.Name] = true
				default:
					t.Fatalf("%s in %s lists an element this parser cannot read (%T at %s) -- extend it rather than guessing the floor table", themeFloorTokensVar, themeContrastTestPath, elt, fset.Position(elt.Pos()))
				}
			}
		}
		return true
	})
	if !found {
		t.Fatalf("no %s declaration found in %s -- R84's floor table was renamed or removed; this completeness proof reads it directly and must not be silently satisfied without it", themeFloorTokensVar, themeContrastTestPath)
	}
	if len(tokens) == 0 {
		t.Fatalf("%s in %s is empty -- an empty floor table would make any drawn pair 'uncovered'; refusing to pass", themeFloorTokensVar, themeContrastTestPath)
	}
	return tokens
}

// funcInfo names one FuncDecl (name set) or FuncLit (name "") node, so
// enclosingFunc can find the innermost function/closure containing a
// given position without maintaining an explicit traversal stack.
type funcInfo struct {
	node ast.Node
	name string
}

func collectFuncInfos(file *ast.File) []funcInfo {
	var out []funcInfo
	ast.Inspect(file, func(n ast.Node) bool {
		switch d := n.(type) {
		case *ast.FuncDecl:
			out = append(out, funcInfo{d, d.Name.Name})
		case *ast.FuncLit:
			out = append(out, funcInfo{d, ""})
		}
		return true
	})
	return out
}

// enclosingFunc returns the smallest (innermost) funcInfo whose node
// spans pos.
func enclosingFunc(funcs []funcInfo, pos token.Pos) (funcInfo, bool) {
	var best funcInfo
	found := false
	var bestSpan token.Pos
	for _, fi := range funcs {
		if fi.node.Pos() <= pos && pos <= fi.node.End() {
			span := fi.node.End() - fi.node.Pos()
			if !found || span < bestSpan {
				best, bestSpan, found = fi, span, true
			}
		}
	}
	return best, found
}

// resolveThemeTokens statically resolves expr to the set of theme.X token
// names it can evaluate to, looking through local variables assigned
// within scope. It reports ok=false for ANY expression it cannot resolve
// with certainty -- a function call, a map/slice index, a struct field, a
// parameter, a package-level variable. That fail-closed answer is the
// point: task 1204's first attempt only recognised a literal
// `Tok: theme.X`, so `tok := theme.Dimmed; ...{Tok: tok}` slipped past it
// silently. Every caller here turns ok=false into a test failure, so the
// only way to compose a token over theme.Selection without this test
// having an opinion about it is to make this analysis smarter first.
func resolveThemeTokens(expr ast.Expr, scope ast.Node, depth int) (map[string]bool, bool) {
	if depth > 8 {
		return nil, false
	}
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return resolveThemeTokens(e.X, scope, depth+1)
	case *ast.SelectorExpr:
		x, ok := e.X.(*ast.Ident)
		if !ok || x.Name != "theme" {
			return nil, false
		}
		return map[string]bool{e.Sel.Name: true}, true
	case *ast.Ident:
		if scope == nil {
			return nil, false
		}
		out := map[string]bool{}
		resolvedAny := false
		allOK := true
		ast.Inspect(scope, func(n ast.Node) bool {
			switch s := n.(type) {
			case *ast.AssignStmt:
				if len(s.Lhs) != len(s.Rhs) {
					return true
				}
				for i, lhs := range s.Lhs {
					id, ok := lhs.(*ast.Ident)
					if !ok || id.Name != e.Name {
						continue
					}
					vals, ok := resolveThemeTokens(s.Rhs[i], scope, depth+1)
					resolvedAny = true
					if !ok {
						allOK = false
						continue
					}
					for name := range vals {
						out[name] = true
					}
				}
			case *ast.ValueSpec:
				for i, name := range s.Names {
					if name.Name != e.Name {
						continue
					}
					resolvedAny = true
					if i >= len(s.Values) {
						// `var tok theme.Token` with no initialiser:
						// whatever it ends up holding is decided
						// elsewhere, so refuse to guess.
						allOK = false
						continue
					}
					vals, ok := resolveThemeTokens(s.Values[i], scope, depth+1)
					if !ok {
						allOK = false
						continue
					}
					for n := range vals {
						out[n] = true
					}
				}
			}
			return true
		})
		if !resolvedAny || !allOK || len(out) == 0 {
			return nil, false
		}
		return out, true
	default:
		return nil, false
	}
}

func isBgColorTokenCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "bgColorToken" && len(call.Args) > 0
}

// isRenderCreateRowSegmentsCall reports whether call invokes
// `<recv>.renderCreateRowSegments(...)`.
func isRenderCreateRowSegmentsCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "renderCreateRowSegments"
}

// tokExpr is one expression a settingsRowSegment's Tok field is set from
// -- a `Tok:` composite-literal value or an assignment's right-hand side
// -- kept with its position so an unresolvable one can be reported
// precisely. expr is nil when the Tok field is written in a form
// collectTokExprs cannot pair with a value; the caller turns that into a
// failure.
type tokExpr struct {
	expr ast.Expr
	pos  token.Pos
}

// collectTokExprs walks node's subtree and returns every expression a Tok
// field within it is set FROM, in either of the two forms Go offers:
//
//   - as the `Tok:` value of a composite literal (`{Text: ..., Tok: X}`),
//     and
//   - as the right-hand side of an assignment to a Tok field
//     (`seg.Tok = X`, `segs[i].Tok = X`, `p.Tok = X`) -- the form task
//     1204's second attempt missed, and the form validation used to slip
//     a sub-floor `theme.Dimmed` into renderRenameFieldRow's already-
//     built segments without this test noticing.
//
// settingsRowSegment (settings.go) is the only struct in this package with
// a `Tok theme.Token` field, so matching on the field NAME alone -- rather
// than resolving the composite literal's own type, which the untyped
// `{Text: ..., Tok: ...}` form inside a `[]settingsRowSegment{...}` slice
// literal elides -- reliably identifies every one, in both its typed and
// elided forms. An assignment whose shape this cannot read off (a tuple
// assignment `a.Tok, b.Tok = f()`, or a compound `a.Tok += x`) yields a
// nil expr, which the caller reports as unresolvable: fail closed, never
// skip.
//
// Resolution of the returned expressions (and the fail-closed error for
// anything unresolvable) belongs to the caller, which knows the enclosing
// scope.
func collectTokExprs(node ast.Node) []tokExpr {
	var found []tokExpr
	isTokSelector := func(e ast.Expr) bool {
		sel, ok := e.(*ast.SelectorExpr)
		return ok && sel.Sel.Name == "Tok"
	}
	ast.Inspect(node, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.CompositeLit:
			for _, elt := range s.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok || key.Name != "Tok" {
					continue
				}
				found = append(found, tokExpr{kv.Value, kv.Value.Pos()})
			}
		case *ast.AssignStmt:
			for i, lhs := range s.Lhs {
				if !isTokSelector(lhs) {
					continue
				}
				if s.Tok != token.ASSIGN || len(s.Rhs) != len(s.Lhs) {
					// A compound assignment, or a tuple assignment
					// whose right-hand side is a single call: the
					// value cannot be paired with this Tok field
					// here, so hand the caller a nil expr and let it
					// fail closed.
					found = append(found, tokExpr{nil, lhs.Pos()})
					continue
				}
				found = append(found, tokExpr{s.Rhs[i], s.Rhs[i].Pos()})
			}
		}
		return true
	})
	return found
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestDialogSelectionRenderersComposeOnlyFloorTokens is task 1204's own
// completeness proof: R84's contrast floor (internal/theme's
// TestThemedDialogTokensClearContrastFloor) only has to hold the pairs a
// dialog actually draws over theme.Selection, and this test proves the
// floor's table -- read live out of internal/theme/contrast_test.go's
// dialogSelectionTokens, never copied here -- lists every such token, by
// static analysis of internal/tui's own source, in three steps:
//
//  1. Every `bgColorToken(...)` call in the package has its background
//     argument statically resolved (resolveThemeTokens, which looks
//     through local variables and FAILS on anything it cannot resolve, so
//     `bg := theme.Selection; m.bgColorToken(bg, ...)` cannot hide a
//     site). Exactly two of them must resolve to theme.Selection, and
//     they must be renderCreateRowSegments (tui.go) and
//     renderRenameFieldRow (rename.go) -- a third site, or either of
//     these two disappearing, fails this test rather than silently going
//     unchecked.
//  2. Every settingsRowSegment Tok field written in the scope that
//     feeds either site -- as a `Tok:` composite-literal value OR as the
//     target of a later assignment (`segs[i].Tok = ...`) -- is resolved
//     the same fail-closed way (renderRenameFieldRow's own body; for
//     renderCreateRowSegments, the innermost enclosing function/closure
//     of each of its call sites, since it receives segs as a parameter
//     rather than building it itself): a token this analysis cannot pin
//     down is a failure, not a silent omission.
//  3. Every resolved token must appear in the floor table. A future edit
//     that composes `dimmed`, `key` or `error` over theme.Selection --
//     reintroducing a pair R84's floor does not hold, per
//     internal/theme/contrast_test.go's own dialogSelectionTokens doc
//     comment, which also records why theme.SelectionIdle is out of scope
//     -- fails here.
//
// This static pass is deliberately paired with the render-level proof in
// dialog_selection_floor_render_test.go
// (TestDialogSelectionCellsRenderOnlyFloorTokens), which reads the
// finished emulator grid of every themed dialog on every built-in and so
// catches a token that reaches a focused row by any route at all -- a
// helper this pass does not walk, a value computed at run time. Neither
// test may be weakened on the grounds that the other exists.
func TestDialogSelectionRenderersComposeOnlyFloorTokens(t *testing.T) {
	floorTokens := loadThemeSelectionFloorTokens(t)
	t.Logf("R84 floor table (%s %s): %v", themeContrastTestPath, themeFloorTokensVar, sortedKeys(floorTokens))

	fset := token.NewFileSet()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("Glob(*.go): %v", err)
	}

	var allFuncs []funcInfo
	files := map[string]*ast.File{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("ParseFile(%s): %v", path, err)
		}
		files[path] = f
		allFuncs = append(allFuncs, collectFuncInfos(f)...)
	}
	if len(files) == 0 {
		t.Fatal("no non-test .go files found in internal/tui -- Glob pattern or working directory is wrong")
	}

	// Step 1: enumerate every bgColorToken call and resolve its
	// background argument, failing closed on anything unresolvable.
	var selectionSites []token.Pos
	var bgCalls int
	var renderSites []token.Pos
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if isRenderCreateRowSegmentsCall(call) {
				renderSites = append(renderSites, call.Pos())
			}
			if !isBgColorTokenCall(call) {
				return true
			}
			bgCalls++
			scope := ast.Node(f)
			if fi, ok := enclosingFunc(allFuncs, call.Pos()); ok {
				scope = fi.node
			}
			bgs, ok := resolveThemeTokens(call.Args[0], scope, 0)
			if !ok {
				t.Errorf("bgColorToken call at %s: cannot statically resolve its background argument (%T) to a theme token -- extend resolveThemeTokens or pass a literal theme.X, because an unresolved background may be theme.Selection and would escape R84's floor check", fset.Position(call.Pos()), call.Args[0])
				return true
			}
			if bgs["Selection"] {
				selectionSites = append(selectionSites, call.Pos())
			}
			return true
		})
	}
	if bgCalls == 0 {
		t.Fatal("found zero bgColorToken calls in internal/tui -- enumeration is broken, not proving anything")
	}

	if len(selectionSites) != 2 {
		var where []string
		for _, pos := range selectionSites {
			where = append(where, fset.Position(pos).String())
		}
		t.Fatalf("found %d call(s) to bgColorToken(theme.Selection, ...) in internal/tui (%v), want exactly 2 (renderCreateRowSegments, renderRenameFieldRow) -- a new/removed site changes what R84's floor must cover", len(selectionSites), where)
	}

	gotSites := map[string]bool{}
	for _, pos := range selectionSites {
		fi, ok := enclosingFunc(allFuncs, pos)
		if !ok || fi.name == "" {
			t.Fatalf("bgColorToken(theme.Selection, ...) call at %s is not directly inside a named function", fset.Position(pos))
		}
		gotSites[fi.name] = true
	}
	wantSites := map[string]bool{"renderCreateRowSegments": true, "renderRenameFieldRow": true}
	if len(gotSites) != len(wantSites) {
		t.Fatalf("bgColorToken(theme.Selection, ...) sites are %v, want exactly %v", sortedKeys(gotSites), sortedKeys(wantSites))
	}
	for name := range wantSites {
		if !gotSites[name] {
			t.Fatalf("bgColorToken(theme.Selection, ...) sites are %v, want exactly %v", sortedKeys(gotSites), sortedKeys(wantSites))
		}
	}

	// Step 2: resolve every Tok expression composed in either site's own
	// scope, again failing closed.
	got := map[string]bool{}
	resolveScope := func(what string, scope ast.Node) {
		for _, te := range collectTokExprs(scope) {
			if te.expr == nil {
				t.Errorf("%s assigns a settingsRowSegment's Tok field at %s in a form this analysis cannot pair with a value (compound or tuple assignment) -- this row is drawn over theme.Selection, so extend collectTokExprs rather than leaving the token unchecked", what, fset.Position(te.pos))
				continue
			}
			toks, ok := resolveThemeTokens(te.expr, scope, 0)
			if !ok {
				t.Errorf("%s composes a settingsRowSegment whose Tok expression at %s (%T) cannot be statically resolved to a theme token -- this row is drawn over theme.Selection, so an unresolved token could be a pair R84's floor does not hold; extend resolveThemeTokens or use a literal theme.X", what, fset.Position(te.pos), te.expr)
				continue
			}
			for name := range toks {
				got[name] = true
			}
		}
	}

	renameSeen := false
	for _, fi := range allFuncs {
		if fi.name == "renderRenameFieldRow" {
			renameSeen = true
			resolveScope("renderRenameFieldRow", fi.node)
		}
	}
	if !renameSeen {
		t.Fatal("renderRenameFieldRow not found -- enumeration is broken, not proving anything")
	}
	if len(renderSites) == 0 {
		t.Fatal("found zero calls to renderCreateRowSegments -- enumeration is broken, not proving anything")
	}
	for _, pos := range renderSites {
		fi, ok := enclosingFunc(allFuncs, pos)
		if !ok {
			t.Fatalf("renderCreateRowSegments call at %s has no enclosing function", fset.Position(pos))
		}
		resolveScope(fmt.Sprintf("the renderCreateRowSegments caller at %s", fset.Position(pos)), fi.node)
	}

	if len(got) == 0 {
		t.Fatal("collected zero tokens composed over theme.Selection -- enumeration is broken, not proving anything")
	}
	t.Logf("tokens composed over theme.Selection by dialog focused rows: %v", sortedKeys(got))

	// Step 3: the floor table must list every one of them.
	for _, tok := range sortedKeys(got) {
		if !floorTokens[tok] {
			t.Errorf("a dialog composes theme.%s over theme.Selection, but %s's %s (the table TestThemedDialogTokensClearContrastFloor walks) lists only %v -- add it there so R84's contrast floor actually covers this pair", tok, themeContrastTestPath, themeFloorTokensVar, sortedKeys(floorTokens))
		}
	}
}

// TestNoDialogContrastAllowlistRemains is the tracked form of task 1204's
// "no allowlist" half: R84's floor may not be softened by a per-theme,
// per-pair exemption table anywhere under internal/. task 106 introduced
// one, named by joining the two halves of `banned` below; this fails if
// that identifier is ever reintroduced, so the criterion's
// `grep -rn <that name> internal/` staying empty is enforced by the suite
// rather than by hand. The name is assembled at run time precisely so
// this guard does not itself become the grep's only hit.
func TestNoDialogContrastAllowlistRemains(t *testing.T) {
	banned := "dialogPair" + "Allowlist"
	root := ".." // internal/, since tests run in their own package dir
	var hits []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), banned) {
			hits = append(hits, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if len(hits) > 0 {
		t.Errorf("%s reappears in %v -- R84's contrast floor is hard for every built-in and every pair it checks; a sub-floor pair is a finding for docs/reports/phase3g-findings.md, never an exemption", banned, hits)
	}
}
