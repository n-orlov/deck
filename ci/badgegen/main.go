// Package main implements ci/badgegen, R289's badge generator: it turns the
// result files the CI gates leave behind into shields.io endpoint JSON, one
// file per gate.
//
//	go run ./ci/badgegen -in ci-results -out badges
//
// Each badges/<name>.json carries exactly the four keys shields.io's
// endpoint schema needs: schemaVersion, label, message and color. The five
// gates, and the one input file under -in each reads (every one is already
// written by ci/suite.sh or ci/quality.sh):
//
//	lint.json             quality-report.txt   the "=== golangci gate ===" section (ci/quality.sh)
//	unit-tests.json       junit-merged/junit-go.xml        the <testsuites> failures and errors (ci/suite.sh)
//	features.json         junit-merged/junit-features.xml  the <testsuites> failures and errors (ci/suite.sh)
//	coverage.json         coverage-merged.out  the statement coverage of the merged profile, cmd/fake-* excluded (ci/suite.sh)
//	vulnerabilities.json  quality-report.txt   the "=== govulncheck gate ===" and "=== trivy gate ===" sections (ci/quality.sh)
//
// A gate that passed renders green, a gate that failed renders red, and a
// result file that is missing or cannot be parsed renders a grey "unknown":
// a badge is green only when the file it reads says so. Coverage is the
// exception to the two-way split: it renders green at or above
// coverageGreenMin (85, the product total_floor in ci/quality.json), yellow
// at or above coverageYellowMin (70) and red below that.
//
// The lint badge also takes the result of the ci.yml lint job (-lint-result
// success|failure|...): CI skips the suite when lint fails, so no
// quality-report.txt exists on that run, yet lint is the one gate whose
// outcome is known. A failed lint job renders red whatever the report says;
// a passing one renders green unless the report says golangci failed; any
// other value (cancelled, skipped, empty) defers to the report alone.
//
// The generator exits 0 once it has written all five files, whatever they
// say, so a red or unknown badge never hides the files of the others; it
// exits 2 only when it cannot write them.
package main

import (
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// coverageGreenMin and coverageYellowMin are the coverage badge's
// thresholds, in percent of statements.
const (
	coverageGreenMin  = 85.0
	coverageYellowMin = 70.0
)

// Badge colours and the message of a badge whose input cannot be trusted.
const (
	colorGreen   = "green"
	colorYellow  = "yellow"
	colorRed     = "red"
	colorGrey    = "lightgrey"
	unknownLabel = "unknown"
)

// fakeFixturePrefix marks the fixture packages the coverage gate excludes
// from the product total (ci/covgate).
const fakeFixturePrefix = "cmd/fake-"

// badge is the shields.io endpoint schema, nothing more.
type badge struct {
	SchemaVersion int    `json:"schemaVersion"`
	Label         string `json:"label"`
	Message       string `json:"message"`
	Color         string `json:"color"`
}

// verdict is what one gate's input says: a message and a colour.
type verdict struct {
	message string
	color   string
}

var unknown = verdict{unknownLabel, colorGrey}

// gate names one badge: its file name, its label, the input file it reads
// and how that file's bytes become a verdict.
type gate struct {
	name  string
	label string
	input string
	eval  func(data []byte) verdict
}

var gates = []gate{
	{"lint", "lint", "quality-report.txt", func(d []byte) verdict { return passFail(sectionState(string(d), "golangci")) }},
	{"unit-tests", "unit tests", "junit-merged/junit-go.xml", evalJUnit},
	{"features", "features (e2e)", "junit-merged/junit-features.xml", evalJUnit},
	{"coverage", "coverage", "coverage-merged.out", evalCoverage},
	{"vulnerabilities", "vulnerability audit", "quality-report.txt", evalAudit},
}

// state is the three-way outcome of a gate section.
type state int

const (
	stateUnknown state = iota
	statePassed
	stateFailed
)

func passFail(s state) verdict {
	switch s {
	case statePassed:
		return verdict{"passing", colorGreen}
	case stateFailed:
		return verdict{"failing", colorRed}
	}
	return unknown
}

// sectionState reads one "=== <name> gate ===" section of ci/quality.sh's
// report. ci/qualitycheck ends a passing section with "what to do: nothing
// --" and a failing one with a different "what to do:" line; a section with
// neither is unknown.
func sectionState(report, name string) state {
	header := "=== " + name + " gate ==="
	_, rest, found := strings.Cut(report, header+"\n")
	if !found {
		return stateUnknown
	}
	if body, _, more := strings.Cut(rest, "\n=== "); more {
		rest = body
	}
	result := stateUnknown
	for _, line := range strings.Split(rest, "\n") {
		switch {
		case strings.HasPrefix(line, "what to do: nothing --"):
			return statePassed
		case strings.HasPrefix(line, "what to do:"):
			result = stateFailed
		}
	}
	return result
}

// evalAudit joins the two vulnerability gates: any failed section is red,
// otherwise any missing or unreadable one is unknown, otherwise green.
func evalAudit(data []byte) verdict {
	report := string(data)
	states := []state{sectionState(report, "govulncheck"), sectionState(report, "trivy")}
	for _, s := range states {
		if s == stateFailed {
			return verdict{"vulnerable", colorRed}
		}
	}
	for _, s := range states {
		if s != statePassed {
			return unknown
		}
	}
	return verdict{"no known vulnerabilities", colorGreen}
}

// junitRoot is the <testsuites> element ci/suite.sh writes.
type junitRoot struct {
	XMLName  xml.Name `xml:"testsuites"`
	Tests    int      `xml:"tests,attr"`
	Failures int      `xml:"failures,attr"`
	Errors   int      `xml:"errors,attr"`
}

// evalJUnit scores a merged JUnit file: any failure or error is red, a file
// that holds no tests is unknown, the rest is green with the test count.
func evalJUnit(data []byte) verdict {
	var root junitRoot
	if err := xml.Unmarshal(data, &root); err != nil || root.Tests <= 0 {
		return unknown
	}
	if bad := root.Failures + root.Errors; bad > 0 {
		return verdict{strconv.Itoa(bad) + " failed", colorRed}
	}
	return verdict{strconv.Itoa(root.Tests) + " passed", colorGreen}
}

// coverageColor maps a percentage onto the coverage thresholds.
func coverageColor(pct float64) string {
	switch {
	case pct >= coverageGreenMin:
		return colorGreen
	case pct >= coverageYellowMin:
		return colorYellow
	}
	return colorRed
}

// evalCoverage scores a Go coverprofile: statement coverage over every
// package except cmd/fake-*. A profile without the "mode:" header, with a
// line that is not a block, or without product statements is unknown.
func evalCoverage(data []byte) verdict {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "mode: ") {
		return unknown
	}
	var stmts, covered int
	for _, line := range lines[1:] {
		pkg, n, hit, ok := parseBlock(line)
		if !ok {
			return unknown
		}
		if strings.Contains(pkg, "/"+fakeFixturePrefix) {
			continue
		}
		stmts += n
		if hit {
			covered += n
		}
	}
	if stmts == 0 {
		return unknown
	}
	pct := float64(covered) * 100 / float64(stmts)
	return verdict{fmt.Sprintf("%.1f%%", pct), coverageColor(pct)}
}

// parseBlock reads "file:l.c,l.c <statements> <count>".
func parseBlock(line string) (file string, stmts int, hit, ok bool) {
	fields := strings.Fields(line)
	if len(fields) != 3 {
		return "", 0, false, false
	}
	file, _, found := strings.Cut(fields[0], ":")
	n, nerr := strconv.Atoi(fields[1])
	count, cerr := strconv.Atoi(fields[2])
	if !found || nerr != nil || cerr != nil || n < 0 || count < 0 {
		return "", 0, false, false
	}
	return file, n, count > 0, true
}

// render reads one gate's input under in and returns its badge. A file that
// cannot be read is as unknown as one that cannot be parsed.
func (g gate) render(in string) badge {
	return g.renderWith(in, "")
}

// renderWith is render plus the lint job's result, which only the lint gate
// reads.
func (g gate) renderWith(in, lintResult string) badge {
	v := unknown
	if data, err := os.ReadFile(filepath.Join(in, filepath.FromSlash(g.input))); err == nil { //nolint:gosec // G304: in is the tool's own command-line argument
		v = g.eval(data)
	}
	if g.name == "lint" {
		v = withLintJob(v, lintResult)
	}
	return badge{SchemaVersion: 1, Label: g.label, Message: v.message, Color: v.color}
}

// withLintJob folds the lint job's result into the lint gate's verdict from
// the quality report: a failed job is red, a passed one is green unless the
// report already says red, anything else leaves the report's verdict alone.
func withLintJob(fromReport verdict, jobResult string) verdict {
	switch jobResult {
	case "failure":
		return passFail(stateFailed)
	case "success":
		if fromReport.color == colorRed {
			return fromReport
		}
		return passFail(statePassed)
	}
	return fromReport
}

// generate writes badges/<name>.json for every gate. lintResult is the lint
// job's result ("" when unknown).
func generate(in, out, lintResult string) error {
	if err := os.MkdirAll(out, 0o755); err != nil { //nolint:gosec // badges are published, not secret
		return err
	}
	for _, g := range gates {
		body, err := json.MarshalIndent(g.renderWith(in, lintResult), "", "  ")
		if err != nil {
			return err
		}
		path := filepath.Join(out, g.name+".json")
		if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil { //nolint:gosec // badges are published, not secret
			return err
		}
	}
	return nil
}

func main() {
	in := flag.String("in", "ci-results", "directory ci/suite.sh and ci/quality.sh wrote their results to")
	out := flag.String("out", "badges", "directory to write the badge JSON files to")
	lint := flag.String("lint-result", "", "result of the ci.yml lint job (success, failure, cancelled, skipped); empty reads the quality report alone")
	flag.Parse()
	if err := generate(*in, *out, *lint); err != nil {
		fmt.Fprintln(os.Stderr, "badgegen:", err)
		os.Exit(2)
	}
}
