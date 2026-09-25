// Command junitflaky folds a retried test's attempts into one JUnit
// <testcase> so an Allure report (R146) shows it as what it is: green but
// flaky, never hidden and never red.
//
//	go run ./ci/junitflaky -o <merged.xml> <first.xml> [<rerun.xml>...]
//
// Inputs are JUnit files in chronological order: the first is the original
// run, every later one a rerun (gotestsum's --rerun-fails already appends
// its reruns to the same file, after the original attempt; ci/suite.sh's
// per-scenario features/ reruns each write a file of their own). A test is
// keyed by (testsuite name, testcase classname, testcase name), and its
// attempts are taken in that order: document order within a file, file
// order across files.
//
// Why this exists: Allure's junit-xml plugin gives every testcase in a
// testsuite the testsuite's own timestamp as its start time, so two
// attempts of one test in the same testsuite tie, and a godog rerun file
// written without the original's testcase is a separate, unrelated result.
// Either way Allure can pick the failed attempt as "latest" and render a
// test that passed on retry as failed, with flaky=false. The plugin's own
// explicit convention (the Maven Surefire one) avoids that: a single
// passing <testcase> carrying one <rerunFailure>/<rerunError> child per
// earlier failed attempt is reported passed, flaky=true, with each earlier
// attempt kept as a hidden retry.
//
// So a test whose LAST attempt passed after at least one failed attempt is
// rewritten in place (at its first occurrence) as its last, passing
// attempt plus one <rerunFailure> or <rerunError> per failed attempt,
// carrying that attempt's message, type and body text; its other
// occurrences are dropped. Every other testcase -- passed first time,
// skipped, or failed on every attempt -- passes through unchanged; a
// rerun's testcase that is not flaky is appended to the same-named
// testsuite of the merged file (a new testsuite if there is none), so a
// test that failed twice still shows both failures. Every tests/failures/
// errors/skipped count on each <testsuite> and on the root <testsuites>
// is recomputed from what the merged file actually holds.
package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// node is a generic XML element: enough of JUnit's shape to round-trip
// every element and attribute gotestsum and godog write.
type node struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Text    string     `xml:",chardata"`
	Nodes   []*node    `xml:",any"`
}

func (n *node) attr(name string) string {
	for _, a := range n.Attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func (n *node) hasAttr(name string) bool {
	for _, a := range n.Attrs {
		if a.Name.Local == name {
			return true
		}
	}
	return false
}

func (n *node) setAttr(name, value string) {
	for i, a := range n.Attrs {
		if a.Name.Local == name {
			n.Attrs[i].Value = value
			return
		}
	}
	n.Attrs = append(n.Attrs, xml.Attr{Name: xml.Name{Local: name}, Value: value})
}

func (n *node) child(name string) *node {
	for _, c := range n.Nodes {
		if c.XMLName.Local == name {
			return c
		}
	}
	return nil
}

// outcome is one testcase attempt's result.
type outcome int

const (
	passed outcome = iota
	failed
	errored
	skipped
)

func outcomeOf(tc *node) outcome {
	switch {
	case tc.child("failure") != nil:
		return failed
	case tc.child("error") != nil:
		return errored
	case tc.child("skipped") != nil:
		return skipped
	}
	// godog's JUnit formatter also states the scenario's status as an
	// attribute; trust it when no child element says otherwise.
	switch tc.attr("status") {
	case "failed":
		return failed
	case "undefined", "pending", "ambiguous":
		return errored
	case "skipped":
		return skipped
	}
	return passed
}

// trimSpace drops whitespace-only character data everywhere, so the merged
// file can be re-indented cleanly; element text that is not blank (failure
// bodies, system-out) is kept verbatim.
func trimSpace(n *node) {
	if strings.TrimSpace(n.Text) == "" {
		n.Text = ""
	}
	for _, c := range n.Nodes {
		trimSpace(c)
	}
}

func parse(path string) (*node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root node
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	trimSpace(&root)
	switch root.XMLName.Local {
	case "testsuites":
		return &root, nil
	case "testsuite":
		// A bare <testsuite> root: wrap it so every file has one shape.
		return &node{XMLName: xml.Name{Local: "testsuites"}, Nodes: []*node{&root}}, nil
	}
	return nil, fmt.Errorf("%s: root element <%s> is not a JUnit <testsuites>/<testsuite>", path, root.XMLName.Local)
}

type key struct{ suite, class, name string }

type attempt struct {
	suite *node // the <testsuite> holding it
	tc    *node
	file  int
}

// merge folds inputs (chronological order) into the first one's tree and
// returns it.
func merge(inputs []*node) *node {
	out := inputs[0]
	var order []key
	attempts := map[key][]attempt{}
	for fi, root := range inputs {
		for _, suite := range root.Nodes {
			if suite.XMLName.Local != "testsuite" {
				continue
			}
			for _, tc := range suite.Nodes {
				if tc.XMLName.Local != "testcase" {
					continue
				}
				k := key{suite.attr("name"), tc.attr("classname"), tc.attr("name")}
				if _, seen := attempts[k]; !seen {
					order = append(order, k)
				}
				attempts[k] = append(attempts[k], attempt{suite: suite, tc: tc, file: fi})
			}
		}
	}

	drop := map[*node]bool{}
	var appendLater []attempt
	for _, k := range order {
		as := attempts[k]
		last := as[len(as)-1]
		var earlierFails []*node
		for _, a := range as[:len(as)-1] {
			if o := outcomeOf(a.tc); o == failed || o == errored {
				earlierFails = append(earlierFails, a.tc)
			}
		}
		if outcomeOf(last.tc) == passed && len(earlierFails) > 0 {
			folded := fold(last.tc, earlierFails)
			first := as[0]
			if first.file == 0 {
				*first.tc = *folded
				for _, a := range as[1:] {
					if a.file == 0 {
						drop[a.tc] = true
					}
				}
			} else {
				appendLater = append(appendLater, attempt{suite: first.suite, tc: folded})
			}
			continue
		}
		for _, a := range as {
			if a.file != 0 {
				appendLater = append(appendLater, a)
			}
		}
	}

	for _, suite := range out.Nodes {
		if suite.XMLName.Local != "testsuite" {
			continue
		}
		kept := suite.Nodes[:0]
		for _, c := range suite.Nodes {
			if !drop[c] {
				kept = append(kept, c)
			}
		}
		suite.Nodes = kept
	}

	for _, a := range appendLater {
		target := findSuite(out, a.suite.attr("name"))
		if target == nil {
			target = &node{XMLName: a.suite.XMLName, Attrs: append([]xml.Attr(nil), a.suite.Attrs...)}
			for _, c := range a.suite.Nodes {
				if c.XMLName.Local == "properties" {
					target.Nodes = append(target.Nodes, c)
				}
			}
			out.Nodes = append(out.Nodes, target)
		}
		target.Nodes = append(target.Nodes, a.tc)
	}

	recount(out)
	return out
}

func findSuite(root *node, name string) *node {
	for _, s := range root.Nodes {
		if s.XMLName.Local == "testsuite" && s.attr("name") == name {
			return s
		}
	}
	return nil
}

// fold returns a copy of the passing attempt with one rerunFailure /
// rerunError child per earlier failed attempt, in attempt order.
func fold(pass *node, fails []*node) *node {
	out := &node{XMLName: pass.XMLName, Attrs: append([]xml.Attr(nil), pass.Attrs...), Text: pass.Text}
	for _, f := range fails {
		name, src := "rerunFailure", f.child("failure")
		if src == nil {
			if e := f.child("error"); e != nil {
				name, src = "rerunError", e
			}
		}
		if src == nil && outcomeOf(f) == errored {
			name = "rerunError"
		}
		r := &node{XMLName: xml.Name{Local: name}}
		if src != nil {
			for _, a := range src.Attrs {
				if a.Name.Local == "message" || a.Name.Local == "type" {
					r.Attrs = append(r.Attrs, a)
				}
			}
			r.Text = src.Text
		} else {
			r.setAttr("message", "status "+f.attr("status"))
		}
		out.Nodes = append(out.Nodes, r)
	}
	out.Nodes = append(out.Nodes, pass.Nodes...)
	return out
}

// recount rewrites every tests/failures/errors (and skipped, where the
// writer used it) count from the merged testcases themselves.
func recount(root *node) {
	var tt, tf, te, ts int
	for _, suite := range root.Nodes {
		if suite.XMLName.Local != "testsuite" {
			continue
		}
		var t, f, e, s int
		for _, tc := range suite.Nodes {
			if tc.XMLName.Local != "testcase" {
				continue
			}
			t++
			switch outcomeOf(tc) {
			case failed:
				f++
			case errored:
				e++
			case skipped:
				s++
			}
		}
		setCounts(suite, t, f, e, s)
		tt, tf, te, ts = tt+t, tf+f, te+e, ts+s
	}
	setCounts(root, tt, tf, te, ts)
}

func setCounts(n *node, t, f, e, s int) {
	n.setAttr("tests", strconv.Itoa(t))
	n.setAttr("failures", strconv.Itoa(f))
	n.setAttr("errors", strconv.Itoa(e))
	if n.hasAttr("skipped") {
		n.setAttr("skipped", strconv.Itoa(s))
	}
}

func write(w io.Writer, root *node) error {
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "\t")
	if err := enc.Encode(root); err != nil {
		return err
	}
	buf.WriteString("\n")
	_, err := w.Write(buf.Bytes())
	return err
}

func run(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("junitflaky", flag.ContinueOnError)
	fs.SetOutput(stderr)
	outPath := fs.String("o", "", "merged JUnit output file (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *outPath == "" || fs.NArg() == 0 {
		return errors.New("usage: junitflaky -o <merged.xml> <first.xml> [<rerun.xml>...]")
	}
	var inputs []*node
	for _, p := range fs.Args() {
		root, err := parse(p)
		if err != nil {
			return err
		}
		inputs = append(inputs, root)
	}
	merged := merge(inputs)
	f, err := os.Create(*outPath)
	if err != nil {
		return err
	}
	if err := write(f, merged); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "junitflaky:", err)
		os.Exit(1)
	}
}
