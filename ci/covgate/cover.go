package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// blockKey identifies one statement block of a coverprofile by file and
// source range, so a profile that lists the same block twice (concatenated
// profiles) is counted once.
type blockKey struct {
	file string
	pos  string
}

// blockStat is one block's statement count and whether any run executed it.
type blockStat struct {
	numStmt int
	covered bool
}

// profile is a parsed coverprofile, deduplicated by block.
type profile struct {
	blocks map[blockKey]blockStat
}

// parseProfile reads a coverprofile (`go test -coverprofile` or `go tool
// covdata textfmt`). A missing file, an empty file, a file without a
// "mode:" first line and a file with a mode line but no blocks are all
// errors: an empty profile must fail the gate, never pass it (R189).
func parseProfile(path string) (*profile, error) {
	f, err := os.Open(path) //nolint:gosec // G304: the coverprofile path is the tool's own command-line argument
	if err != nil {
		return nil, fmt.Errorf("coverprofile %q: %w", path, err)
	}
	defer func() { _ = f.Close() }() // read-only handle: Close cannot lose data

	p := &profile{blocks: make(map[blockKey]blockStat)}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	lines := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines++
		if lines == 1 {
			if !strings.HasPrefix(line, "mode:") {
				return nil, fmt.Errorf("coverprofile %q: first line %q is not a mode line", path, line)
			}
			continue
		}
		key, stat, perr := parseBlockLine(line)
		if perr != nil {
			return nil, fmt.Errorf("coverprofile %q: %w", path, perr)
		}
		prev := p.blocks[key]
		p.blocks[key] = blockStat{numStmt: stat.numStmt, covered: prev.covered || stat.covered}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("coverprofile %q: %w", path, err)
	}
	if lines == 0 {
		return nil, fmt.Errorf("coverprofile %q is empty", path)
	}
	if len(p.blocks) == 0 {
		return nil, fmt.Errorf("coverprofile %q has a mode line but zero coverage blocks", path)
	}
	return p, nil
}

// parseBlockLine parses "file:start,end numStmt count".
func parseBlockLine(line string) (blockKey, blockStat, error) {
	colon := strings.LastIndex(line, ":")
	if colon < 0 {
		return blockKey{}, blockStat{}, fmt.Errorf("missing ':' in %q", line)
	}
	rest := strings.Fields(line[colon+1:])
	if len(rest) != 3 {
		return blockKey{}, blockStat{}, fmt.Errorf("expected '<range> <numStmt> <count>' in %q", line)
	}
	numStmt, err := strconv.Atoi(rest[1])
	if err != nil || numStmt < 0 {
		return blockKey{}, blockStat{}, fmt.Errorf("bad statement count %q in %q", rest[1], line)
	}
	count, err := strconv.Atoi(rest[2])
	if err != nil || count < 0 {
		return blockKey{}, blockStat{}, fmt.Errorf("bad execution count %q in %q", rest[2], line)
	}
	return blockKey{file: line[:colon], pos: rest[0]}, blockStat{numStmt: numStmt, covered: count > 0}, nil
}
