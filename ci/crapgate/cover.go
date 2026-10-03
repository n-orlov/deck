package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// coverBlock is one line of a go coverprofile: the statement block
// spanning [startLine.startCol, endLine.endCol), covering numStmt
// statements, executed count times (count == 0 means never executed).
type coverBlock struct {
	startLine, endLine int
	numStmt            int
	count              int
}

// profile is a parsed `go test -coverprofile` (or merged
// `go tool covdata textfmt`) file, keyed by the file identifier exactly
// as the profile recorded it (typically "<module path>/<rel path>.go").
type profile struct {
	mode   string
	blocks map[string][]coverBlock
}

// parseProfile reads and parses a coverprofile. A profile that does not
// exist, or that holds no block lines at all (whether because the file
// is literally empty or holds only a "mode:" line), is reported as an
// error -- PRD R187: "zero scored functions fails, an empty or missing
// profile fails".
func parseProfile(path string) (*profile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("coverprofile %q: %w", path, err)
	}
	defer f.Close()

	p := &profile{blocks: make(map[string][]coverBlock)}
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if lineNo == 1 {
			if !strings.HasPrefix(line, "mode:") {
				return nil, fmt.Errorf("coverprofile %q: first line %q is not a mode line", path, line)
			}
			p.mode = strings.TrimSpace(strings.TrimPrefix(line, "mode:"))
			continue
		}
		block, name, parseErr := parseProfileLine(line)
		if parseErr != nil {
			return nil, fmt.Errorf("coverprofile %q line %d: %w", path, lineNo, parseErr)
		}
		p.blocks[name] = append(p.blocks[name], block)
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return nil, fmt.Errorf("coverprofile %q: %w", path, scanErr)
	}
	if lineNo == 0 {
		return nil, fmt.Errorf("coverprofile %q is empty", path)
	}
	if len(p.blocks) == 0 {
		return nil, fmt.Errorf("coverprofile %q has a mode line but zero coverage blocks", path)
	}
	return p, nil
}

// parseProfileLine parses one "name:startLine.startCol,endLine.endCol
// numStmt count" line, the format go test -coverprofile and
// go tool covdata textfmt both emit.
func parseProfileLine(line string) (coverBlock, string, error) {
	colon := strings.LastIndex(line, ":")
	// The file name can itself contain ':' on some platforms, but the
	// position field never does except as its own separator; splitting
	// off everything after the LAST ':' that is followed by the
	// "N.N,N.N N N" shape handles the common case. Coverprofiles in this
	// repo use plain POSIX import paths, so a simple split is enough.
	if colon < 0 {
		return coverBlock{}, "", fmt.Errorf("missing ':' in %q", line)
	}
	name := line[:colon]
	rest := strings.Fields(line[colon+1:])
	if len(rest) != 3 {
		return coverBlock{}, "", fmt.Errorf("expected '<range> <numStmt> <count>' after ':' in %q", line)
	}
	startEnd := strings.SplitN(rest[0], ",", 2)
	if len(startEnd) != 2 {
		return coverBlock{}, "", fmt.Errorf("expected 'start,end' range in %q", line)
	}
	startLine, err := leadingLine(startEnd[0])
	if err != nil {
		return coverBlock{}, "", err
	}
	endLine, err := leadingLine(startEnd[1])
	if err != nil {
		return coverBlock{}, "", err
	}
	numStmt, err := strconv.Atoi(rest[1])
	if err != nil {
		return coverBlock{}, "", fmt.Errorf("numStmt %q: %w", rest[1], err)
	}
	count, err := strconv.Atoi(rest[2])
	if err != nil {
		return coverBlock{}, "", fmt.Errorf("count %q: %w", rest[2], err)
	}
	return coverBlock{startLine: startLine, endLine: endLine, numStmt: numStmt, count: count}, name, nil
}

// leadingLine parses the "line" part out of a "line.col" position.
func leadingLine(posField string) (int, error) {
	dot := strings.Index(posField, ".")
	if dot < 0 {
		return 0, fmt.Errorf("expected 'line.col' in %q", posField)
	}
	return strconv.Atoi(posField[:dot])
}
