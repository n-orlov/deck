package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// scannedFunc is one named function or method found in source, together
// with enough position information to attribute coverage blocks and to
// name it in a report.
type scannedFunc struct {
	Name    string // "Foo" or "(*T).Foo" for a method
	File    string // absolute path on disk
	Line    int    // 1-based line of the func keyword
	EndLine int    // 1-based line of the function's closing brace
	Decl    *ast.FuncDecl
}

// parseFilter splits an optional -filter value into the package
// directory it names (relative to the module root, "" meaning "scan the
// whole tree") and, if the <dir>:<file,...> form was used, the explicit
// file list to scan inside that directory (nil meaning "every non-test
// .go file directly in dir").
func parseFilter(filter string) (dir string, files []string, wholeTree bool) {
	if filter == "" {
		return "", nil, true
	}
	if idx := strings.Index(filter, ":"); idx >= 0 {
		dir = filter[:idx]
		list := filter[idx+1:]
		for _, f := range strings.Split(list, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				files = append(files, f)
			}
		}
		return dir, files, false
	}
	return filter, nil, false
}

// filterRoot resolves the -filter value to the directory scanning should
// be rooted at, purely for error messages and for a non-whole-tree scan;
// moduleRoot is where scanning starts when filter is "".
func filterRoot(filter string) (root string, err error) {
	moduleRoot, _, findErr := findModule()
	if findErr != nil {
		return "", findErr
	}
	dir, _, whole := parseFilter(filter)
	if whole {
		return moduleRoot, nil
	}
	return filepath.Join(moduleRoot, filepath.FromSlash(dir)), nil
}

// findModule walks up from the current working directory for the nearest
// go.mod, returning its directory (the module root) and the module path
// declared in it (the first "module " line).
func findModule() (root string, modulePath string, err error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	for {
		gomod := filepath.Join(dir, "go.mod")
		if data, readErr := os.ReadFile(gomod); readErr == nil { //nolint:gosec // G304: go.mod is looked up in the directories above the directory the tool was pointed at
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "module ") {
					return dir, strings.TrimSpace(strings.TrimPrefix(line, "module")), nil
				}
			}
			return dir, "", fmt.Errorf("%s has no module line", gomod)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", fmt.Errorf("no go.mod found above %s", dir)
		}
		dir = parent
	}
}

// skipDirName reports whether a directory name is never descended into
// during a whole-tree scan.
func skipDirName(name string) bool {
	switch name {
	case ".git", "vendor":
		return true
	}
	return strings.HasPrefix(name, ".")
}

// resolveSourceFiles returns the absolute paths of every Go source file
// scanning should consider, honouring -filter's two forms (PRD R187:
// "an optional package or <pkg-dir>:<file,...> filter").
func resolveSourceFiles(moduleRoot, filter string) ([]string, error) {
	dir, explicitFiles, whole := parseFilter(filter)
	if whole {
		return walkSourceFiles(moduleRoot)
	}

	absDir := filepath.Join(moduleRoot, filepath.FromSlash(dir))
	info, err := os.Stat(absDir)
	if err != nil {
		return nil, fmt.Errorf("filter directory %q: %w", absDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("filter directory %q is not a directory", absDir)
	}

	if explicitFiles != nil {
		return explicitSourceFiles(absDir, explicitFiles)
	}
	return dirSourceFiles(absDir)
}

// isSourceFile reports whether name is a non-test Go source file.
func isSourceFile(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
}

// walkSourceFiles returns every non-test Go file under moduleRoot, sorted,
// never descending into a skipDirName directory below the root.
func walkSourceFiles(moduleRoot string) ([]string, error) {
	var files []string
	walkErr := filepath.Walk(moduleRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if path != moduleRoot && skipDirName(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if isSourceFile(path) {
			files = append(files, path)
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Strings(files)
	return files, nil
}

// explicitSourceFiles joins each named file onto absDir, failing on the
// first one that does not exist.
func explicitSourceFiles(absDir string, names []string) ([]string, error) {
	files := make([]string, 0, len(names))
	for _, f := range names {
		p := filepath.Join(absDir, f)
		if _, statErr := os.Stat(p); statErr != nil {
			return nil, fmt.Errorf("filter file %q: %w", p, statErr)
		}
		files = append(files, p)
	}
	return files, nil
}

// dirSourceFiles returns the non-test Go files directly in absDir, sorted.
func dirSourceFiles(absDir string) ([]string, error) {
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && isSourceFile(e.Name()) {
			files = append(files, filepath.Join(absDir, e.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

// scanFunctions parses every file resolveSourceFiles names under root
// (root is the module root; filter restricts which files) and returns
// every top-level named function and method declaration with a body
// (a declaration-only, body-less FuncDecl -- an assembly stub -- is
// skipped: it has nothing a coverage profile could ever instrument).
func scanFunctions(root, filter string) ([]scannedFunc, error) {
	files, err := resolveSourceFiles(root, filter)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	var out []scannedFunc
	for _, path := range files {
		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, parseErr)
		}
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			pos := fset.Position(fd.Pos())
			endPos := fset.Position(fd.End())
			out = append(out, scannedFunc{
				Name:    funcDisplayName(fd),
				File:    pos.Filename,
				Line:    pos.Line,
				EndLine: endPos.Line,
				Decl:    fd,
			})
		}
	}
	return out, nil
}

// funcDisplayName renders a FuncDecl the way the report names it: a bare
// function name, or "(*T).Method"/"(T).Method" for a method, matching
// how Go programmers already read receiver notation.
func funcDisplayName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return fd.Name.Name
	}
	recvType := exprString(fd.Recv.List[0].Type)
	return fmt.Sprintf("(%s).%s", recvType, fd.Name.Name)
}

func exprString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return "*" + exprString(t.X)
	case *ast.Ident:
		return t.Name
	default:
		return fmt.Sprintf("%v", t)
	}
}
