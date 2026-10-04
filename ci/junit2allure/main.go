// Command junit2allure writes a JUnit file (ci/suite.sh's junit-merged/*.xml)
// as native Allure 2 results (R196), so the Go unit tests land in the same
// report as features/'s native results under a group of their own:
//
//	go run ./ci/junit2allure -group unit -o <allure-results dir> <junit.xml>...
//
// Allure's own JUnit plugin cannot do this: it never sets a parentSuite, so
// unit and features tests could not be grouped. Every <testcase> becomes one
// *-result.json whose labels carry parentSuite=<group>, and each
// <rerunFailure>/<rerunError> child (ci/junitflaky's convention for a test
// that failed and then passed) becomes an earlier, failed result with the same
// historyId, so Allure shows one test with retries and its own flaky marker.
//
// The historyId is the plugin's own "<testsuite>:<classname>#<name>", so a
// unit test keeps the history and trend Allure already carries for it.
package main

import (
	"bytes"
	"crypto/md5" //nolint:gosec // an identity digest for result uuids, not a security primitive
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type problem struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Text    string `xml:",chardata"`
}

type testCase struct {
	Name         string     `xml:"name,attr"`
	ClassName    string     `xml:"classname,attr"`
	Time         string     `xml:"time,attr"`
	Failure      *problem   `xml:"failure"`
	Error        *problem   `xml:"error"`
	Skipped      *problem   `xml:"skipped"`
	RerunFailure []*problem `xml:"rerunFailure"`
	RerunError   []*problem `xml:"rerunError"`
}

type testSuite struct {
	Name      string      `xml:"name,attr"`
	Timestamp string      `xml:"timestamp,attr"`
	Cases     []testCase  `xml:"testcase"`
	Suites    []testSuite `xml:"testsuite"`
}

type testSuites struct {
	Suites []testSuite `xml:"testsuite"`
}

type label struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type details struct {
	Message string `json:"message,omitempty"`
	Trace   string `json:"trace,omitempty"`
	Flaky   bool   `json:"flaky,omitempty"`
}

type result struct {
	UUID          string   `json:"uuid"`
	HistoryID     string   `json:"historyId"`
	TestCaseID    string   `json:"testCaseId"`
	FullName      string   `json:"fullName"`
	Name          string   `json:"name"`
	Status        string   `json:"status"`
	StatusDetails *details `json:"statusDetails,omitempty"`
	Stage         string   `json:"stage"`
	Start         int64    `json:"start"`
	Stop          int64    `json:"stop"`
	Labels        []label  `json:"labels"`
	Links         []any    `json:"links"`
	Steps         []any    `json:"steps"`
	Attachments   []any    `json:"attachments"`
	Parameters    []any    `json:"parameters"`
}

func digest(s string) string {
	sum := md5.Sum([]byte(s)) //nolint:gosec // see the import
	return hex.EncodeToString(sum[:])
}

func uuidOf(s string) string {
	h := digest(s)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func millis(seconds string) int64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(seconds), 64)
	if err != nil || f < 0 {
		return 0
	}
	return int64(f * 1000)
}

func detailsOf(p *problem) *details {
	if p == nil {
		return nil
	}
	msg := p.Message
	if msg == "" {
		msg = strings.TrimSpace(p.Text)
	}
	return &details{Message: msg, Trace: strings.TrimSpace(p.Text)}
}

// writer numbers the results it writes so that, for one historyId, a later
// attempt always has a later start than an earlier one.
type writer struct {
	dir   string
	group string
	base  int64
	seq   int64
	count int
}

func (w *writer) emit(suite string, tc testCase, attempt int, status string, d *details, durationMS int64) error {
	history := suite + ":" + tc.ClassName + "#" + tc.Name
	w.seq++
	start := w.base + w.seq
	r := result{
		UUID:          uuidOf(fmt.Sprintf("%s/%s/%d/%d", w.group, history, attempt, w.seq)),
		HistoryID:     history,
		TestCaseID:    digest(w.group + "/" + history),
		FullName:      tc.ClassName + "." + tc.Name,
		Name:          tc.Name,
		Status:        status,
		StatusDetails: d,
		Stage:         "finished",
		Start:         start,
		Stop:          start + durationMS,
		Labels: []label{
			{"parentSuite", w.group}, {"suite", suite}, {"testClass", tc.ClassName},
			{"package", tc.ClassName}, {"language", "go"}, {"framework", "go test"},
		},
		Links: []any{}, Steps: []any{}, Attachments: []any{}, Parameters: []any{},
	}
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}
	w.count++
	return os.WriteFile(filepath.Join(w.dir, r.UUID+"-result.json"), body, 0o644) //nolint:gosec // report input, read by the Allure CLI
}

func (w *writer) suite(s testSuite, parent string) error {
	name := s.Name
	if name == "" {
		name = parent
	}
	for _, tc := range s.Cases {
		// Earlier failed attempts first: ci/junitflaky folded them into this case.
		attempt := 0
		for _, p := range tc.RerunFailure {
			attempt++
			if err := w.emit(name, tc, attempt, "failed", detailsOf(p), 0); err != nil {
				return err
			}
		}
		for _, p := range tc.RerunError {
			attempt++
			if err := w.emit(name, tc, attempt, "broken", detailsOf(p), 0); err != nil {
				return err
			}
		}
		status, d := "passed", (*details)(nil)
		switch {
		case tc.Failure != nil:
			status, d = "failed", detailsOf(tc.Failure)
		case tc.Error != nil:
			status, d = "broken", detailsOf(tc.Error)
		case tc.Skipped != nil:
			status, d = "skipped", detailsOf(tc.Skipped)
		}
		if attempt > 0 && status == "passed" {
			// The JUnit plugin's own convention for a test that passed after
			// failed attempts: flagged flaky, on top of the retries it shows.
			d = &details{Flaky: true}
		}
		if err := w.emit(name, tc, attempt+1, status, d, millis(tc.Time)); err != nil {
			return err
		}
	}
	for _, child := range s.Suites {
		if err := w.suite(child, name); err != nil {
			return err
		}
	}
	return nil
}

// rootElementIs reports whether the document's first element is named name.
func rootElementIs(raw []byte, name string) bool {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		if start, ok := tok.(xml.StartElement); ok {
			return start.Name.Local == name
		}
	}
}

func convert(path string, w *writer) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }() // read-only handle: Close cannot lose data
	raw, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	var root testSuites
	if rootElementIs(raw, "testsuite") {
		// A file whose root is one <testsuite> (not <testsuites>): its own
		// cases and nested suites all belong to it.
		var single testSuite
		if err := xml.Unmarshal(raw, &single); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		root.Suites = []testSuite{single}
	} else if err := xml.Unmarshal(raw, &root); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for _, s := range root.Suites {
		if err := w.suite(s, filepath.Base(path)); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return nil
}

func run(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("junit2allure", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", "", "directory the *-result.json files are written to (created if missing)")
	group := fs.String("group", "", "the parentSuite label every result carries (e.g. unit)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" || *group == "" || fs.NArg() == 0 {
		return errors.New("usage: junit2allure -group <name> -o <results dir> <junit.xml> [<junit.xml> ...]")
	}
	if err := os.MkdirAll(*out, 0o755); err != nil { //nolint:gosec // report input
		return err
	}
	w := &writer{dir: *out, group: *group, base: time.Now().UnixMilli()}
	for _, path := range fs.Args() {
		if err := convert(path, w); err != nil {
			return err
		}
	}
	if w.count == 0 {
		return errors.New("no <testcase> in any input: nothing to write")
	}
	if _, err := fmt.Fprintf(stderr, "junit2allure: wrote %d results (group %s) to %s\n", w.count, *group, *out); err != nil {
		return fmt.Errorf("report result count: %w", err)
	}
	return nil
}

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "junit2allure:", err)
		os.Exit(1)
	}
}
