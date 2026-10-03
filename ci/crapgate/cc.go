package main

import (
	"go/ast"
	"go/token"
)

// cyclomaticComplexity returns the McCabe cyclomatic complexity of fn's
// body under the locked gocyclo-style rules (PRD R187):
//
//	cc = 1 + count(if) + count(for) + count(range) + count(case) +
//	         count(comm-case) + count(&&) + count(||)
//
// A case/comm-case clause only adds when it is NOT the default clause
// (an ast.CaseClause/ast.CommClause with a nil List is "default"/"default:"
// and contributes nothing). A closure (ast.FuncLit) defined inside fn is
// walked into, not skipped: its own branches/operators count into the
// enclosing function's score, and a closure is never scored on its own.
// fn may be nil (an external/assembly declaration with no body), in which
// case the complexity is defined as 1.
func cyclomaticComplexity(fn *ast.FuncDecl) int {
	if fn == nil || fn.Body == nil {
		return 1
	}
	return 1 + branchCount(fn.Body)
}

// branchCount walks node (and everything nested inside it, including the
// body of any closure) and sums one point for every branching construct
// the locked rules count. It never stops descending at a FuncLit boundary
// -- that is precisely what makes a closure's branches count into its
// enclosing named function instead of being scored separately.
func branchCount(node ast.Node) int {
	count := 0
	ast.Inspect(node, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.IfStmt:
			count++
		case *ast.ForStmt:
			count++
		case *ast.RangeStmt:
			count++
		case *ast.CaseClause:
			if v.List != nil { // nil List == "default:", adds nothing
				count++
			}
		case *ast.CommClause:
			if v.Comm != nil { // nil Comm == "default:", adds nothing
				count++
			}
		case *ast.BinaryExpr:
			if v.Op == token.LAND || v.Op == token.LOR {
				count++
			}
		}
		return true // always descend, including into nested FuncLit bodies
	})
	return count
}
