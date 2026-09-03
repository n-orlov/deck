package store

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestUpdateStatementsNeverMutateAgentCwdSlugCapturedPath is a source-scanning
// guard: it parses every non-test .go file in this package, finds every
// string literal containing an `UPDATE sessions SET ...` statement, and fails
// if the SET clause of any such statement assigns to agent, cwd, slug, or
// captured_path. Those four columns are written once at session creation
// (see CreateSession) and must never be mutated by any later store UPDATE.
func TestUpdateStatementsNeverMutateAgentCwdSlugCapturedPath(t *testing.T) {
	forbidden := []string{"agent", "cwd", "slug", "captured_path"}
	colRe := make(map[string]*regexp.Regexp, len(forbidden))
	for _, c := range forbidden {
		colRe[c] = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(c) + `\s*=`)
	}

	// Only the SET clause matters: WHERE-clause references to a forbidden
	// name (unlikely, but possible in principle) are not mutations.
	setClauseRe := regexp.MustCompile(`(?is)UPDATE\s+sessions\s+SET\s+(.*?)(?:\bWHERE\b|$)`)
	hasUpdateRe := regexp.MustCompile(`(?i)UPDATE\s+sessions\s+SET\b`)

	files, err := filepath.Glob(filepath.Join(".", "*.go"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no .go files found in internal/store")
	}

	fset := token.NewFileSet()
	statementsSeen := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		node, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		ast.Inspect(node, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			val, uerr := strconv.Unquote(lit.Value)
			if uerr != nil {
				return true
			}
			if !hasUpdateRe.MatchString(val) {
				return true
			}
			statementsSeen++
			pos := fset.Position(lit.Pos())

			setClause := val
			if m := setClauseRe.FindStringSubmatch(val); m != nil {
				setClause = m[1]
			}

			for _, c := range forbidden {
				if colRe[c].MatchString(setClause) {
					t.Errorf(
						"forbidden column %q assigned by `UPDATE sessions SET` statement at %s:%d — "+
							"agent, cwd, slug and captured_path must never be mutated after CreateSession; "+
							"matched statement:\n%s",
						c, pos.Filename, pos.Line, val)
				}
			}
			return true
		})
	}

	if statementsSeen == 0 {
		t.Fatal("no `UPDATE sessions SET` statements found in internal/store non-test sources — " +
			"guard test may be broken (e.g. glob or regexp no longer matches real code)")
	}
}
