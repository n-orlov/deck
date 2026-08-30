package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// dialogSelectionFloorTokens mirrors internal/theme/contrast_test.go's
// dialogSelectionTokens exactly: the set of theme tokens a dialog's
// focused row is allowed to compose over theme.Selection, per R84's
// contrast floor (internal/theme's TestThemedDialogTokensClearContrastFloor).
// The two packages cannot share the literal slice (theme's is
// package-private to internal/theme and typed as theme.Token), so this
// map is the tui-side half of that contract: if this test's own
// enumeration below ever finds a token not listed here, it fails loudly
// rather than silently letting an uncovered pair reach the screen.
// Keep in sync with dialogSelectionTokens by hand -- both name the same
// fact.
var dialogSelectionFloorTokens = map[string]bool{
	"Hint": true,
	"Text": true,
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

func isSelector(e ast.Expr, xName, selName string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == xName && sel.Sel.Name == selName
}

// isBgColorTokenSelectionCall reports whether call is exactly
// `<recv>.bgColorToken(theme.Selection, ...)`.
func isBgColorTokenSelectionCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "bgColorToken" || len(call.Args) == 0 {
		return false
	}
	return isSelector(call.Args[0], "theme", "Selection")
}

// isRenderCreateRowSegmentsCall reports whether call invokes
// `<recv>.renderCreateRowSegments(...)`.
func isRenderCreateRowSegmentsCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "renderCreateRowSegments"
}

// collectTokTokens walks node's subtree and returns the set of theme.X
// identifiers used as the Tok field of a settingsRowSegment composite
// literal within it. settingsRowSegment (settings.go) is the only struct
// in this package with a `Tok theme.Token` field, so matching on the
// `Tok:` key alone -- rather than resolving the composite literal's own
// type, which the untyped `{Text: ..., Tok: ...}` form inside a
// `[]settingsRowSegment{...}` slice literal elides -- reliably identifies
// every one, in both its typed and elided forms.
func collectTokTokens(node ast.Node) map[string]bool {
	found := map[string]bool{}
	ast.Inspect(node, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, elt := range cl.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok || key.Name != "Tok" {
				continue
			}
			if sel, ok := kv.Value.(*ast.SelectorExpr); ok {
				if x, ok := sel.X.(*ast.Ident); ok && x.Name == "theme" {
					found[sel.Sel.Name] = true
				}
			}
		}
		return true
	})
	return found
}

// TestDialogSelectionRenderersComposeOnlyFloorTokens is task 1204's own
// completeness proof: R84's contrast floor (internal/theme's
// TestThemedDialogTokensClearContrastFloor) only has to hold the pairs a
// dialog actually draws over theme.Selection, and this test proves that
// set is exactly dialogSelectionFloorTokens (Hint, Text) by static
// analysis of internal/tui's own source, in two steps:
//
//  1. bgColorToken(theme.Selection, ...) is called from exactly two
//     places in this package -- renderCreateRowSegments (tui.go) and
//     renderRenameFieldRow (rename.go) -- named explicitly so a third
//     site (a new dialog composing its own selection background some
//     other way) fails this test rather than silently going unchecked.
//  2. Every settingsRowSegment{Tok: theme.X, ...} composite literal built
//     in the scope that feeds either site (renderRenameFieldRow's own
//     body; for renderCreateRowSegments, the innermost enclosing
//     function/closure of each of its call sites, since it receives segs
//     as a parameter rather than building it itself) names a token in
//     dialogSelectionFloorTokens. A future edit that composes `dimmed`,
//     `key` or `error` over theme.Selection -- reintroducing a pair R84's
//     floor does not hold, per internal/theme/contrast_test.go's own
//     dialogSelectionTokens doc comment -- fails here.
func TestDialogSelectionRenderersComposeOnlyFloorTokens(t *testing.T) {
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

	var selectionSites []token.Pos
	var renderSites []token.Pos
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if isBgColorTokenSelectionCall(call) {
				selectionSites = append(selectionSites, call.Pos())
			}
			if isRenderCreateRowSegmentsCall(call) {
				renderSites = append(renderSites, call.Pos())
			}
			return true
		})
	}

	if len(selectionSites) != 2 {
		t.Fatalf("found %d call(s) to bgColorToken(theme.Selection, ...) in internal/tui, want exactly 2 (renderCreateRowSegments, renderRenameFieldRow) -- a new/removed site changes what R84's floor must cover", len(selectionSites))
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
		t.Fatalf("bgColorToken(theme.Selection, ...) sites are %v, want exactly %v", gotSites, wantSites)
	}
	for name := range wantSites {
		if !gotSites[name] {
			t.Fatalf("bgColorToken(theme.Selection, ...) sites are %v, want exactly %v", gotSites, wantSites)
		}
	}

	got := map[string]bool{}
	for _, fi := range allFuncs {
		if fi.name == "renderRenameFieldRow" {
			for tok := range collectTokTokens(fi.node) {
				got[tok] = true
			}
		}
	}
	if len(renderSites) == 0 {
		t.Fatal("found zero calls to renderCreateRowSegments -- enumeration is broken, not proving anything")
	}
	for _, pos := range renderSites {
		fi, ok := enclosingFunc(allFuncs, pos)
		if !ok {
			t.Fatalf("renderCreateRowSegments call at %s has no enclosing function", fset.Position(pos))
		}
		for tok := range collectTokTokens(fi.node) {
			got[tok] = true
		}
	}

	if len(got) == 0 {
		t.Fatal("collected zero tokens composed over theme.Selection -- enumeration is broken, not proving anything")
	}
	t.Logf("tokens composed over theme.Selection by dialog focused rows: %v", got)
	for tok := range got {
		if !dialogSelectionFloorTokens[tok] {
			t.Errorf("a dialog composes theme.%s over theme.Selection, but dialogSelectionFloorTokens (and internal/theme/contrast_test.go's dialogSelectionTokens, which must mirror it) does not list it -- add it to both so R84's contrast floor actually covers this pair", tok)
		}
	}
}
