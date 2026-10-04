package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// funcScore is one scanned function's final numbers: McCabe complexity,
// fractional coverage in [0,1], and the CRAP score those two combine
// into.
type funcScore struct {
	Name string
	File string // absolute path on disk
	Line int
	CC   int
	Cov  float64
	CRAP float64
}

// crapOf computes PRD R187's locked formula:
//
//	CRAP = cc^2 * (1 - cov)^3 + cc
//
// A zero-statement function is scored at 100% coverage by the caller
// before this is ever invoked (coverageOf below) -- crapOf itself has no
// special case; cov=1 here always yields CRAP == cc.
func crapOf(cc int, cov float64) float64 {
	ccF := float64(cc)
	return ccF*ccF*(1-cov)*(1-cov)*(1-cov) + ccF
}

// coverageOf returns a function's fractional coverage in [0,1] from the
// profile's blocks. A function whose body holds zero top-level statements
// is defined as 100% covered -- there is nothing to not cover, so it
// never drags a legitimately empty stub's CRAP above its own cc. A
// function with statements that matches no block at all in the profile
// (present in source, absent from the profile -- a file the build never
// instrumented) is scored at 0%, never skipped.
func coverageOf(fn scannedFunc, modulePath, moduleRoot string, p *profile) float64 {
	if len(fn.Decl.Body.List) == 0 {
		return 1.0
	}

	key := profileKey(fn.File, modulePath, moduleRoot)
	blocks := p.blocks[key]

	startLine := fn.Line
	endLine := endLineOf(fn)

	totalStmt, coveredStmt := 0, 0
	for _, b := range blocks {
		if b.startLine >= startLine && b.endLine <= endLine {
			totalStmt += b.numStmt
			if b.count > 0 {
				coveredStmt += b.numStmt
			}
		}
	}
	if totalStmt == 0 {
		// Present in source (and non-empty, checked above), absent from
		// the profile: scored at 0%, never skipped.
		return 0.0
	}
	return float64(coveredStmt) / float64(totalStmt)
}

// endLineOf is the last line of fn's declaration, used to bound which
// coverage blocks belong to it.
func endLineOf(fn scannedFunc) int {
	return fn.EndLine
}

// profileKey converts an absolute file path on disk to the identifier a
// coverprofile would have recorded it under: "<modulePath>/<path relative
// to moduleRoot, forward-slashed>". If, for whatever reason, the path
// cannot be made relative to moduleRoot, the file's own name is used
// verbatim as a last-resort key (a fixture that already writes profiles
// in bare relative form still matches).
func profileKey(absPath, modulePath, moduleRoot string) string {
	rel, err := filepath.Rel(moduleRoot, absPath)
	if err != nil {
		return absPath
	}
	rel = filepath.ToSlash(rel)
	if modulePath == "" {
		return rel
	}
	return modulePath + "/" + rel
}

// scoreFunctions computes a funcScore for every scanned function.
func scoreFunctions(fns []scannedFunc, modulePath, moduleRoot string, p *profile) []funcScore {
	scores := make([]funcScore, 0, len(fns))
	for _, fn := range fns {
		cc := cyclomaticComplexity(fn.Decl)
		cov := coverageOf(fn, modulePath, moduleRoot, p)
		scores = append(scores, funcScore{
			Name: fn.Name,
			File: fn.File,
			Line: fn.Line,
			CC:   cc,
			Cov:  cov,
			CRAP: crapOf(cc, cov),
		})
	}
	return scores
}

// formatReport renders every scored function over max as an offender,
// naming each by file:line, cc, coverage and CRAP (PRD R187's "the
// report names the offender"), and reports the exit code the caller
// should use: 0 if nothing is over max, 1 otherwise.
func formatReport(scores []funcScore, ceiling float64) (report string, exitCode int) {
	var offenders []funcScore
	for _, s := range scores {
		if s.CRAP > ceiling {
			offenders = append(offenders, s)
		}
	}
	sort.Slice(offenders, func(i, j int) bool {
		if offenders[i].CRAP != offenders[j].CRAP {
			return offenders[i].CRAP > offenders[j].CRAP
		}
		return offenders[i].File+fmt.Sprint(offenders[i].Line) < offenders[j].File+fmt.Sprint(offenders[j].Line)
	})

	var b strings.Builder
	fmt.Fprintf(&b, "crapgate: %d function(s) scanned, ceiling %.2f\n", len(scores), ceiling)
	if len(offenders) == 0 {
		fmt.Fprintf(&b, "crapgate: all functions at or under the CRAP ceiling\n")
		return b.String(), 0
	}
	fmt.Fprintf(&b, "crapgate: %d function(s) over the ceiling:\n", len(offenders))
	for _, o := range offenders {
		fmt.Fprintf(&b, "  %s:%d: %s cc=%d cov=%.1f%% CRAP=%.2f\n",
			o.File, o.Line, o.Name, o.CC, o.Cov*100, o.CRAP)
	}
	return b.String(), 1
}
