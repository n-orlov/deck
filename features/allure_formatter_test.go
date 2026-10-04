package features

// A small in-repo Godog formatter that writes Allure 2 results (R195): one
// <uuid>-result.json per scenario, one <uuid>-container.json per feature file
// and one <uuid>-attachment.txt per attachment, into the directory named by
// DECK_GODOG_ALLURE. It lives in the test package (godog's own formatter
// types are internal, so the public formatters.Formatter interface is all it
// can use) and imports nothing outside the standard library and godog, so the
// product's go.mod is unchanged.
//
// Mapping:
//   - the Feature's name is the Allure `feature` label and the feature file's
//     URI the `package` label; one container per feature file lists its tests;
//   - a scenario (pickle) is one test, every Gherkin step one Allure step with
//     its own status, start/stop and the step text verbatim as its name;
//   - @gh-NN becomes an issue link, @multiclient/@slow/@nightly `tag` labels,
//     @claude/@pi/@codex the `suite` label (the feature name then moves to
//     `subSuite`); every result carries `parentSuite` "features", the group
//     ci/suite.sh's unit results ("unit") sit beside in the one report;
//   - the historyId is a digest of the feature URI, scenario name and example
//     row (for an outline: its substituted step texts) only, so the suite's one-retry rerun of a failed scenario (a separate
//     godog run into the same directory, see ci/suite.sh) carries the same
//     historyId and Allure shows it as a retry of one test, never two tests;
//   - on the failing step it attaches only what the harness already writes into
//     the step's error text on failure -- the last normalized pty frame, tmux
//     captures, a deck-log slice, a store dump -- cut out of that text by the
//     section headers the harness prints; the raw PTY stream, input timeline
//     and comparison dumps end a section but are never attached. Nothing is
//     collected anew.
//
// The formatter learns step timing from the moment godog calls it, so under
// godog's concurrent mode (replayed at flush) durations are approximate; this
// suite runs scenarios sequentially.

import (
	"crypto/md5" //nolint:gosec // an identity digest for Allure's historyId, not a security primitive
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/formatters"
)

// allureDirEnv names the allure-results directory. Unset, godogFormat adds no
// allure formatter at all.
const allureDirEnv = "DECK_GODOG_ALLURE"

const allureIssueURLPrefix = "https://github.com/n-orlov/deck/issues/"

func init() {
	godog.Format("allure", "Writes Allure 2 results into $"+allureDirEnv+"; the output file only receives a one-line summary", newAllureFormatter)
}

type allureLabel struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type allureLink struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Type string `json:"type"`
}

type allureParameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type allureAttachment struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Type   string `json:"type"`
}

type allureStatusDetails struct {
	Message string `json:"message,omitempty"`
	Trace   string `json:"trace,omitempty"`
}

type allureStep struct {
	Name          string               `json:"name"`
	Status        string               `json:"status"`
	StatusDetails *allureStatusDetails `json:"statusDetails,omitempty"`
	Stage         string               `json:"stage"`
	Start         int64                `json:"start"`
	Stop          int64                `json:"stop"`
	Steps         []allureStep         `json:"steps"`
	Attachments   []allureAttachment   `json:"attachments"`
	Parameters    []allureParameter    `json:"parameters"`
}

type allureResult struct {
	UUID          string               `json:"uuid"`
	HistoryID     string               `json:"historyId"`
	TestCaseID    string               `json:"testCaseId"`
	FullName      string               `json:"fullName"`
	Name          string               `json:"name"`
	Status        string               `json:"status"`
	StatusDetails *allureStatusDetails `json:"statusDetails,omitempty"`
	Stage         string               `json:"stage"`
	Start         int64                `json:"start"`
	Stop          int64                `json:"stop"`
	Labels        []allureLabel        `json:"labels"`
	Links         []allureLink         `json:"links"`
	Steps         []allureStep         `json:"steps"`
	Attachments   []allureAttachment   `json:"attachments"`
	Parameters    []allureParameter    `json:"parameters"`
}

type allureContainer struct {
	UUID     string   `json:"uuid"`
	Name     string   `json:"name"`
	Children []string `json:"children"`
	Befores  []any    `json:"befores"`
	Afters   []any    `json:"afters"`
	Start    int64    `json:"start"`
	Stop     int64    `json:"stop"`
}

// allureFeatureDoc is what the formatter keeps of one feature file.
type allureFeatureDoc struct {
	uri  string
	name string
}

// allureScenario is the in-flight state of one pickle.
type allureScenario struct {
	pickle *godog.Scenario
	doc    *allureFeatureDoc
	result *allureResult
	last   time.Time
	done   bool
}

type allureFormatter struct {
	out io.Writer
	dir string

	mu         sync.Mutex
	docs       map[string]*allureFeatureDoc // by feature URI
	scenarios  map[string]*allureScenario   // by pickle id
	order      []string                     // pickle ids in start order
	containers map[string]*allureContainer  // by feature URI
	written    int
}

func newAllureFormatter(_ string, out io.Writer) formatters.Formatter {
	return &allureFormatter{
		out:        out,
		dir:        strings.TrimSpace(os.Getenv(allureDirEnv)),
		docs:       map[string]*allureFeatureDoc{},
		scenarios:  map[string]*allureScenario{},
		containers: map[string]*allureContainer{},
	}
}

func (f *allureFormatter) TestRunStarted() {}

func (f *allureFormatter) Feature(doc *godog.GherkinDocument, uri string, _ []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	uri = allureURI(uri)
	d := &allureFeatureDoc{uri: uri}
	if doc != nil && doc.Feature != nil {
		d.name = doc.Feature.Name
	}
	f.docs[uri] = d
}

func (f *allureFormatter) Pickle(p *godog.Scenario) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.begin(p)
}

// begin registers a scenario the first time godog mentions it.
func (f *allureFormatter) begin(p *godog.Scenario) *allureScenario {
	if sc, ok := f.scenarios[p.Id]; ok {
		return sc
	}
	uri := allureURI(p.Uri)
	doc := f.docs[uri]
	if doc == nil {
		doc = &allureFeatureDoc{uri: uri, name: uri}
	}
	now := time.Now()
	sc := &allureScenario{pickle: p, doc: doc, last: now, result: &allureResult{
		UUID:   allureUUID(),
		Name:   p.Name,
		Stage:  "finished",
		Start:  now.UnixMilli(),
		Steps:  []allureStep{},
		Labels: []allureLabel{}, Links: []allureLink{},
		Attachments: []allureAttachment{}, Parameters: []allureParameter{},
	}}
	sc.result.FullName = doc.uri + ": " + p.Name
	sc.result.TestCaseID = allureDigest(sc.result.FullName)
	// A scenario outline's pickles share the outline's name; their (already
	// substituted) step texts tell the example rows apart. Plain scenarios
	// are identified by feature file and name alone.
	identity := sc.result.TestCaseID
	if len(p.AstNodeIds) > 1 {
		for _, st := range p.Steps {
			identity += "|" + st.Text
		}
	}
	sc.result.HistoryID = allureDigest(identity)
	sc.result.Labels, sc.result.Links = allureTagsToLabels(doc, p)
	f.scenarios[p.Id] = sc
	f.order = append(f.order, p.Id)
	return sc
}

var allureLineSuffix = regexp.MustCompile(`:\d+$`)

// allureURI is the feature file's identity: godog hands a path:line selected
// rerun (DECK_GODOG_PATHS=file:42) the URI "file:42", but that rerun is the
// same feature file, and its scenario must keep its historyId.
func allureURI(uri string) string {
	return filepath.ToSlash(allureLineSuffix.ReplaceAllString(uri, ""))
}

// allureFeaturesGroup is the parentSuite every features/ result carries.
const allureFeaturesGroup = "features"

var allureGHTag = regexp.MustCompile(`^@gh-(\d+)$`)

// allureTagsToLabels maps a pickle's Gherkin tags onto Allure labels and links.
func allureTagsToLabels(doc *allureFeatureDoc, p *godog.Scenario) ([]allureLabel, []allureLink) {
	labels := []allureLabel{
		{Name: "feature", Value: doc.name},
		{Name: "package", Value: doc.uri},
		{Name: "parentSuite", Value: allureFeaturesGroup},
		{Name: "framework", Value: "godog"},
		{Name: "language", Value: "go"},
	}
	links := []allureLink{}
	agent := ""
	for _, tag := range p.Tags {
		switch name := tag.Name; {
		case allureGHTag.MatchString(name):
			n := allureGHTag.FindStringSubmatch(name)[1]
			links = append(links, allureLink{Name: "#" + n, URL: allureIssueURLPrefix + n, Type: "issue"})
		case name == "@multiclient", name == "@slow", name == "@nightly":
			labels = append(labels, allureLabel{Name: "tag", Value: strings.TrimPrefix(name, "@")})
		case name == "@claude", name == "@pi", name == "@codex":
			agent = strings.TrimPrefix(name, "@")
		}
	}
	// Allure's Suites tree is parentSuite > suite > subSuite, and a second
	// value for one of those labels makes the test appear under both
	// branches. So the group is the only parentSuite; an agent tag takes the
	// `suite` level and the feature name drops to `subSuite`.
	if agent != "" {
		labels = append(labels, allureLabel{Name: "suite", Value: agent}, allureLabel{Name: "subSuite", Value: doc.name})
	} else {
		labels = append(labels, allureLabel{Name: "suite", Value: doc.name})
	}
	return labels, links
}

func (f *allureFormatter) Defined(p *godog.Scenario, _ *godog.Step, _ *formatters.StepDefinition) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.begin(p)
}

func (f *allureFormatter) Passed(p *godog.Scenario, s *godog.Step, _ *formatters.StepDefinition) {
	f.step(p, s, "passed", nil)
}

func (f *allureFormatter) Skipped(p *godog.Scenario, s *godog.Step, _ *formatters.StepDefinition) {
	f.step(p, s, "skipped", nil)
}

func (f *allureFormatter) Undefined(p *godog.Scenario, s *godog.Step, _ *formatters.StepDefinition) {
	f.step(p, s, "broken", fmt.Errorf("step is undefined"))
}

func (f *allureFormatter) Pending(p *godog.Scenario, s *godog.Step, _ *formatters.StepDefinition) {
	f.step(p, s, "broken", fmt.Errorf("step implementation is pending"))
}

func (f *allureFormatter) Failed(p *godog.Scenario, s *godog.Step, _ *formatters.StepDefinition, err error) {
	f.step(p, s, "failed", err)
}

func (f *allureFormatter) Ambiguous(p *godog.Scenario, s *godog.Step, _ *formatters.StepDefinition, err error) {
	f.step(p, s, "broken", err)
}

// step records one finished step and, after the scenario's last step,
// finalizes and writes the scenario.
func (f *allureFormatter) step(p *godog.Scenario, s *godog.Step, status string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sc := f.begin(p)
	now := time.Now()
	st := allureStep{
		Name: s.Text, Status: status, Stage: "finished",
		Start: sc.last.UnixMilli(), Stop: now.UnixMilli(),
		Steps: []allureStep{}, Attachments: []allureAttachment{}, Parameters: []allureParameter{},
	}
	if err != nil {
		headline, sections := splitFailureText(err.Error())
		st.StatusDetails = &allureStatusDetails{Message: headline, Trace: err.Error()}
		st.Attachments = f.writeSections(sections)
	}
	sc.last = now
	sc.result.Steps = append(sc.result.Steps, st)
	if len(p.Steps) > 0 && s.Id == p.Steps[len(p.Steps)-1].Id {
		f.finish(sc)
	}
}

// allureSection is one attachment cut out of a failing step's error text.
type allureSection struct {
	name string
	body string
}

// The harness prints each failure artifact in the step's error as a header line
// ending in a colon followed by the artifact's text ("frame:\n<frame>",
// "...last capture (err=...):\n<capture>"). Order matters: the first matching
// kind wins.
var allureSectionKinds = []struct {
	re   *regexp.Regexp
	name string
}{
	{regexp.MustCompile(`(?i)\bdeck[ -]?log\b.*:$`), "deck log slice"},
	{regexp.MustCompile(`(?i)\bstore dump\b.*:$`), "store dump"},
	{regexp.MustCompile(`(?i)\bframe\b.*:$`), "last normalized pty frame"},
	{regexp.MustCompile(`(?i)\b(capture|pane)\b.*:$`), "tmux capture"},
}

// allureUnlistedHeader matches the harness's other diagnostic headers -- the
// raw PTY byte stream ScreenDriver prints after every frame ("raw: %q", "raw
// (includes a SIGQUIT goroutine dump ...): %q"), the input timeline ("sent
// (input timeline):", "input:") and comparison pairs ("before:"/"after:",
// "want:"/"got:", "first:"/"second:", "now:"). Each ends the section before it
// and is not itself an attachment: R195 attaches only the listed artifacts.
// It is checked before allureSectionKinds and anchored to the whole line, so
// "first pane capture:" stays a tmux capture.
var allureUnlistedHeader = regexp.MustCompile(`^(raw|sent|input|before|after|want|got|first|second|now|fixture)( \([^\n]*\))?:( +".*")?$`)

// classifyFailureHeader reports whether line is a section header and, if so,
// the attachment it opens ("" for an unlisted header that opens none).
func classifyFailureHeader(line string) (string, bool) {
	line = strings.TrimRight(line, " \t")
	if allureUnlistedHeader.MatchString(line) {
		return "", true
	}
	for _, kind := range allureSectionKinds {
		if kind.re.MatchString(line) {
			return kind.name, true
		}
	}
	return "", false
}

// splitFailureText returns the text before the first header and the listed
// sections that follow it; an unlisted header's text is dropped (the full
// error stays in the step's trace).
func splitFailureText(text string) (string, []allureSection) {
	var headline []string
	var sections []allureSection
	inHeadline := true
	discard := false
	counts := map[string]int{}
	for _, line := range strings.Split(text, "\n") {
		if name, ok := classifyFailureHeader(line); ok {
			inHeadline = false
			discard = name == ""
			if discard {
				continue
			}
			counts[name]++
			label := name
			if counts[name] > 1 {
				label = fmt.Sprintf("%s (%d)", name, counts[name])
			}
			sections = append(sections, allureSection{name: label})
			continue
		}
		switch {
		case inHeadline:
			headline = append(headline, line)
		case !discard:
			sections[len(sections)-1].body += line + "\n"
		}
	}
	kept := sections[:0]
	for _, s := range sections {
		if s.body = strings.TrimRight(s.body, "\n"); s.body != "" {
			kept = append(kept, s)
		}
	}
	return strings.TrimSpace(strings.Join(headline, "\n")), kept
}

func (f *allureFormatter) writeSections(sections []allureSection) []allureAttachment {
	attachments := []allureAttachment{}
	for _, s := range sections {
		source := allureUUID() + "-attachment.txt"
		if f.writeFile(source, []byte(s.body)) {
			attachments = append(attachments, allureAttachment{Name: s.name, Source: source, Type: "text/plain"})
		}
	}
	return attachments
}

// allureStatusRank orders statuses for the scenario roll-up: worse wins.
var allureStatusRank = map[string]int{"passed": 0, "skipped": 1, "broken": 2, "failed": 3}

func rollUpStatus(steps []allureStep) (string, *allureStatusDetails) {
	status := "passed"
	var details *allureStatusDetails
	for _, st := range steps {
		if allureStatusRank[st.Status] > allureStatusRank[status] {
			status = st.Status
			details = st.StatusDetails
		}
	}
	if len(steps) == 0 {
		status = "skipped"
	}
	return status, details
}

func (f *allureFormatter) finish(sc *allureScenario) {
	if sc.done {
		return
	}
	sc.done = true
	sc.result.Stop = time.Now().UnixMilli()
	sc.result.Status, sc.result.StatusDetails = rollUpStatus(sc.result.Steps)
	data, err := json.MarshalIndent(sc.result, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "allure: marshal result:", err)
		return
	}
	if f.writeFile(sc.result.UUID+"-result.json", data) {
		f.written++
	}
	f.writeContainer(sc)
}

// writeContainer (re)writes the feature file's container, so a run that dies
// mid-way still leaves a container for every scenario written before it.
func (f *allureFormatter) writeContainer(sc *allureScenario) {
	c := f.containers[sc.doc.uri]
	if c == nil {
		c = &allureContainer{UUID: allureUUID(), Name: sc.doc.name, Befores: []any{}, Afters: []any{}, Start: sc.result.Start}
		f.containers[sc.doc.uri] = c
	}
	c.Children = append(c.Children, sc.result.UUID)
	c.Stop = sc.result.Stop
	if data, err := json.MarshalIndent(c, "", "  "); err == nil {
		f.writeFile(c.UUID+"-container.json", data)
	}
}

func (f *allureFormatter) writeFile(name string, data []byte) bool {
	if f.dir == "" {
		fmt.Fprintf(os.Stderr, "allure: %s is not set, nothing written\n", allureDirEnv)
		return false
	}
	if err := os.MkdirAll(f.dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "allure:", err)
		return false
	}
	if err := os.WriteFile(filepath.Join(f.dir, name), data, 0o644); err != nil { //nolint:gosec // report files are meant to be world-readable
		fmt.Fprintln(os.Stderr, "allure:", err)
		return false
	}
	return true
}

// Summary finalizes any scenario godog never finished (a scenario without
// steps, or a run stopped on first failure) and prints a one-line summary.
func (f *allureFormatter) Summary() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range f.order {
		f.finish(f.scenarios[id])
	}
	// The godog formatter interface gives Summary no error return, and a closed
	// stdout leaves nowhere to report to.
	_, _ = fmt.Fprintf(f.out, "allure: %d result(s) written to %s\n", f.written, f.dir)
}

func allureDigest(s string) string {
	sum := md5.Sum([]byte(s)) //nolint:gosec // identity digest, see import
	return hex.EncodeToString(sum[:])
}

func allureUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
