package features

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/vt"
	"github.com/cucumber/godog"
)

// allureFixtureOutEnv, when set, is the directory the fixture test writes its
// Allure results into instead of a temporary one (it must start empty).
const allureFixtureOutEnv = "DECK_ALLURE_FIXTURE_OUT"

const allureFixtureFeature = `@claude
Feature: Allure fixture
  Background:
    Given a passing step

  @gh-61 @multiclient @slow
  Scenario: passes with tags
    When a passing step
    Then a passing step

  @nightly
  Scenario: fails with an attachment
    When a passing step
    And the harness fails with a frame
    Then a passing step

  Scenario: is skipped
    When a skipping step
    Then a passing step

  Scenario: fails once then passes
    When a flaky step
`

// allureFixtureScreen is what the fixture's in-memory ScreenDriver receives:
// visible text plus escape sequences, so its raw stream differs from its
// normalized frame.
const allureFixtureScreen = "\x1b[?25l\x1b[1;1Hsessions\x1b[2;1Halpha\x1b[3;1Hbeta-not-yet"

const allureFixtureCapture = "$ echo hi\nhi"

// allureFixtureDriver is a real ScreenDriver with no process behind it, fed
// allureFixtureScreen, so a failing WaitForFrame returns the exact error text
// (frame section, then the raw PTY stream) the harness writes for real.
func allureFixtureDriver() *ScreenDriver {
	d := &ScreenDriver{screen: vt.NewEmulator(30, 4), done: make(chan struct{}), updated: make(chan struct{})}
	_, _ = d.screen.Write([]byte(allureFixtureScreen))
	d.raw.WriteString(allureFixtureScreen)
	return d
}

// allureFixtureFailure is the failing step's error: ScreenDriver's own
// WaitForFrame timeout, followed by a tmux capture in the shape the agent steps
// print ("...; last capture (err=<nil>):\n<capture>").
func allureFixtureFailure() error {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := allureFixtureDriver().WaitForFrame(ctx, false, "beta-ready")
	return fmt.Errorf("%w\nsession \"alpha\"'s private tmux pane never printed \"ready\"; last capture (err=<nil>):\n%s", err, allureFixtureCapture)
}

// allureFixtureRun runs the fixture feature through the Format string
// godogFormat builds for DECK_GODOG_ALLURE, with the given path selector.
func allureFixtureRun(t *testing.T, dir, path string, flakyFails bool) int {
	t.Helper()
	t.Setenv(allureDirEnv, dir)
	t.Setenv("DECK_GODOG_JUNIT", "")
	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^a passing step$`, func() error { return nil })
			sc.Step(`^a skipping step$`, func() error { return godog.ErrSkip })
			sc.Step(`^a flaky step$`, func() error {
				if flakyFails {
					return os.ErrDeadlineExceeded
				}
				return nil
			})
			sc.Step(`^the harness fails with a frame$`, allureFixtureFailure)
		},
		Options: &godog.Options{
			Format: godogFormat(),
			Paths:  []string{path},
			Strict: true,
			Output: io.Discard,
		},
	}
	return suite.Run()
}

// openResultsRoot opens dir as an os.Root, so reading a name that a result file
// itself supplies (an attachment source) can never leave the results directory.
func openResultsRoot(t *testing.T, dir string) *os.Root {
	t.Helper()
	scope, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = scope.Close() }) // read-only handle; nothing to flush
	return scope
}

func readAllureResults(t *testing.T, dir string) []map[string]any {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*-result.json"))
	if err != nil {
		t.Fatal(err)
	}
	scope := openResultsRoot(t, dir)
	var results []map[string]any
	for _, file := range files {
		raw, err := scope.ReadFile(filepath.Base(file))
		if err != nil {
			t.Fatal(err)
		}
		var result map[string]any
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatalf("%s is not JSON: %v", file, err)
		}
		results = append(results, result)
	}
	return results
}

func allureLabelValues(result map[string]any, name string) []string {
	var values []string
	labels, _ := result["labels"].([]any)
	for _, l := range labels {
		label := l.(map[string]any)
		if label["name"] == name {
			values = append(values, label["value"].(string))
		}
	}
	sort.Strings(values)
	return values
}

func allureStepSummary(result map[string]any) []string {
	var out []string
	steps, _ := result["steps"].([]any)
	for _, s := range steps {
		step := s.(map[string]any)
		out = append(out, step["name"].(string)+"="+step["status"].(string))
	}
	return out
}

func allureResultNamed(t *testing.T, results []map[string]any, name string) []map[string]any {
	t.Helper()
	var found []map[string]any
	for _, r := range results {
		if r["name"] == name {
			found = append(found, r)
		}
	}
	if len(found) == 0 {
		t.Fatalf("no allure result named %q among %d results", name, len(results))
	}
	return found
}

// TestAllureFormatterWritesResultsForAFixtureRun pins R195: a pass, a
// fail-with-attachment, a skip and a retried-then-passed scenario produce
// Allure 2 result/container/attachment files with the right statuses, steps,
// labels, issue link, attachment and a shared historyId for the retry.
func TestAllureFormatterWritesResultsForAFixtureRun(t *testing.T) {
	dir := t.TempDir()
	results := filepath.Join(dir, "allure-results")
	if out := os.Getenv(allureFixtureOutEnv); out != "" {
		// ci/allure-smoke.sh (R196) runs this same fixture and keeps its
		// results to hand them to the pinned `allure generate`.
		results = out
	}
	feature := filepath.Join(dir, "allure_fixture.feature")
	if err := os.WriteFile(feature, []byte(allureFixtureFeature), 0o600); err != nil {
		t.Fatal(err)
	}

	// First run: the whole file; two scenarios fail. Then the suite's own
	// one-retry rule: only the flaky scenario again (path:line), now passing.
	if status := allureFixtureRun(t, results, feature, true); status == 0 {
		t.Fatal("first fixture run succeeded, want a failing run")
	}
	failsOnce := strings.Index(allureFixtureFeature, "Scenario: fails once")
	if failsOnce < 0 {
		t.Fatal(`fixture feature has no "Scenario: fails once"`)
	}
	retryLine := strings.Count(allureFixtureFeature[:failsOnce], "\n") + 1
	if status := allureFixtureRun(t, results, feature+":"+strconv.Itoa(retryLine), false); status != 0 {
		t.Fatalf("retry run status %d, want 0", status)
	}

	all := readAllureResults(t, results)
	if len(all) != 5 {
		t.Fatalf("got %d results, want 5 (4 scenarios + 1 retry attempt)", len(all))
	}

	t.Run("pass keeps steps, labels and the issue link", func(t *testing.T) {
		r := allureResultNamed(t, all, "passes with tags")[0]
		if r["status"] != "passed" {
			t.Fatalf("status = %v, want passed", r["status"])
		}
		want := []string{"a passing step=passed", "a passing step=passed", "a passing step=passed"}
		if got := allureStepSummary(r); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("steps = %v, want %v (Background step first, text verbatim)", got, want)
		}
		if got := allureLabelValues(r, "tag"); strings.Join(got, ",") != "multiclient,slow" {
			t.Fatalf("tag labels = %v, want multiclient and slow", got)
		}
		if got := allureLabelValues(r, "parentSuite"); strings.Join(got, ",") != "features" {
			t.Fatalf("parentSuite = %v, want exactly features (the report group)", got)
		}
		if got := allureLabelValues(r, "suite"); strings.Join(got, ",") != "claude" {
			t.Fatalf("suite = %v, want the @claude agent tag", got)
		}
		if got := allureLabelValues(r, "subSuite"); strings.Join(got, ",") != "Allure fixture" {
			t.Fatalf("subSuite = %v, want the feature name under the agent suite", got)
		}
		if got := allureLabelValues(r, "feature"); strings.Join(got, ",") != "Allure fixture" {
			t.Fatalf("feature label = %v", got)
		}
		links, _ := r["links"].([]any)
		if len(links) != 1 {
			t.Fatalf("links = %v, want exactly the @gh-61 issue link", links)
		}
		link := links[0].(map[string]any)
		if link["url"] != "https://github.com/n-orlov/deck/issues/61" || link["type"] != "issue" {
			t.Fatalf("link = %v, want the issue 61 link", link)
		}
		for _, s := range r["steps"].([]any) {
			step := s.(map[string]any)
			if step["stop"].(float64) < step["start"].(float64) || step["start"].(float64) == 0 {
				t.Fatalf("step %v has no usable start/stop", step)
			}
		}
	})

	t.Run("failure attaches only what the harness wrote", func(t *testing.T) {
		r := allureResultNamed(t, all, "fails with an attachment")[0]
		if r["status"] != "failed" {
			t.Fatalf("status = %v, want failed", r["status"])
		}
		if got := allureLabelValues(r, "tag"); strings.Join(got, ",") != "nightly" {
			t.Fatalf("tag labels = %v, want nightly", got)
		}
		steps := r["steps"].([]any)
		failing := steps[2].(map[string]any)
		if failing["name"] != "the harness fails with a frame" || failing["status"] != "failed" {
			t.Fatalf("step 3 = %v", failing)
		}
		if after := steps[3].(map[string]any); after["status"] != "skipped" {
			t.Fatalf("step after the failure = %v, want skipped", after["status"])
		}
		attachments := failing["attachments"].([]any)
		if len(attachments) != 2 {
			t.Fatalf("failing step attachments = %v, want the frame and the tmux capture", attachments)
		}
		bodies := map[string]string{}
		for _, a := range attachments {
			att := a.(map[string]any)
			raw, err := openResultsRoot(t, results).ReadFile(att["source"].(string))
			if err != nil {
				t.Fatalf("attachment %v is referenced but its file is missing: %v", att, err)
			}
			bodies[att["name"].(string)] = string(raw)
		}
		wantFrame := strings.TrimRight(allureFixtureDriver().Frame(false), "\n")
		if !strings.Contains(wantFrame, "beta-not-yet") {
			t.Fatalf("fixture driver frame = %q, want the fed screen text", wantFrame)
		}
		if bodies["last normalized pty frame"] != wantFrame {
			t.Fatalf("frame attachment = %q, want exactly the normalized frame %q (no raw PTY stream)", bodies["last normalized pty frame"], wantFrame)
		}
		for name, body := range bodies {
			if strings.Contains(body, "raw:") || strings.Contains(body, "\x1b") || strings.Contains(body, `\x1b`) {
				t.Fatalf("attachment %q carries the raw PTY stream, which R195 does not list: %q", name, body)
			}
		}
		if bodies["tmux capture"] != allureFixtureCapture {
			t.Fatalf("tmux capture attachment = %q", bodies["tmux capture"])
		}
		for i, s := range steps {
			if i != 2 && len(s.(map[string]any)["attachments"].([]any)) != 0 {
				t.Fatalf("step %d carries attachments: %v", i, s)
			}
		}
	})

	t.Run("skip", func(t *testing.T) {
		r := allureResultNamed(t, all, "is skipped")[0]
		if r["status"] != "skipped" {
			t.Fatalf("status = %v, want skipped", r["status"])
		}
	})

	t.Run("retry shares the historyId and stays one test", func(t *testing.T) {
		attempts := allureResultNamed(t, all, "fails once then passes")
		if len(attempts) != 2 {
			t.Fatalf("got %d attempts, want 2", len(attempts))
		}
		if attempts[0]["historyId"] != attempts[1]["historyId"] || attempts[0]["historyId"] == "" {
			t.Fatalf("historyIds differ: %v vs %v (%v / %v)", attempts[0]["historyId"], attempts[1]["historyId"], attempts[0]["fullName"], attempts[1]["fullName"])
		}
		statuses := []string{attempts[0]["status"].(string), attempts[1]["status"].(string)}
		sort.Strings(statuses)
		if strings.Join(statuses, ",") != "failed,passed" {
			t.Fatalf("attempt statuses = %v, want one failed and one passed", statuses)
		}
		if attempts[0]["uuid"] == attempts[1]["uuid"] {
			t.Fatal("attempts share a uuid, they must be separate result files")
		}
		seen := map[any]string{}
		for _, r := range all {
			if prev, dup := seen[r["historyId"]]; dup && prev != r["name"] {
				t.Fatalf("scenarios %q and %q share a historyId", prev, r["name"])
			}
			seen[r["historyId"]] = r["name"].(string)
		}
	})

	t.Run("one container per feature file lists its tests", func(t *testing.T) {
		files, _ := filepath.Glob(filepath.Join(results, "*-container.json"))
		if len(files) != 2 { // one per godog run; Allure merges by children
			t.Fatalf("containers = %v, want one per run", files)
		}
		children := 0
		scope := openResultsRoot(t, results)
		for _, file := range files {
			raw, _ := scope.ReadFile(filepath.Base(file))
			var c struct {
				Name     string   `json:"name"`
				Children []string `json:"children"`
			}
			if err := json.Unmarshal(raw, &c); err != nil || c.Name != "Allure fixture" {
				t.Fatalf("container %s = %s (%v)", file, raw, err)
			}
			children += len(c.Children)
		}
		if children != 5 {
			t.Fatalf("containers list %d children, want 5", children)
		}
	})
}

// TestAllureFailureSectionsAreCutOutOfTheErrorText pins the header rules that
// pick the harness's failure artifacts out of a step error, including the deck
// log and store dump kinds, and that text before the first header is not an
// attachment.
func TestAllureFailureSectionsAreCutOutOfTheErrorText(t *testing.T) {
	text := "client \"a\": no rendered row contains \"x\"\nfull frame:\nF1\nF2\n" +
		"deck log:\nL1\nstore dump:\nS1\n" +
		"first pane capture:\nC1\nlast capture (err=<nil>):\nC2\n"
	headline, sections := splitFailureText(text)
	if headline != "client \"a\": no rendered row contains \"x\"" {
		t.Fatalf("headline = %q", headline)
	}
	got := map[string]string{}
	for _, s := range sections {
		got[s.name] = s.body
	}
	want := map[string]string{
		"last normalized pty frame": "F1\nF2",
		"deck log slice":            "L1",
		"store dump":                "S1",
		"tmux capture":              "C1",
		"tmux capture (2)":          "C2",
	}
	if len(got) != len(want) {
		t.Fatalf("sections = %v, want %v", got, want)
	}
	for name, body := range want {
		if got[name] != body {
			t.Fatalf("section %q = %q, want %q", name, got[name], body)
		}
	}
	// ScreenDriver's own shapes: the raw PTY stream after a frame, the hung
	// client's input timeline, and comparison pairs end a section and are
	// never attachments.
	text = "hung deck client killed after 5s\nsent (input timeline):\n  [12:00:00.000] +0 \"j\"\nframe:\nF1\n" +
		"raw (includes a SIGQUIT goroutine dump if the runtime managed to print one before the follow-up SIGKILL): \"\\x1b[?25lF1\"\n" +
		"last capture (err=<nil>):\nC1\nwant:\nW1\ngot:\nG1\nbefore: \"b\"\nafter:  \"a\"\ninput:\nI1"
	headline, sections = splitFailureText(text)
	if headline != "hung deck client killed after 5s" {
		t.Fatalf("headline = %q", headline)
	}
	got = map[string]string{}
	for _, s := range sections {
		got[s.name] = s.body
	}
	if len(got) != 2 || got["last normalized pty frame"] != "F1" || got["tmux capture"] != "C1" {
		t.Fatalf("sections = %q, want only the frame F1 and the capture C1", got)
	}
	_, sections = splitFailureText("timed out waiting for frame \"x\": context canceled\nframe:\nNORMALIZED\nraw: \"\\x1b[?25l\"")
	if len(sections) != 1 || sections[0].body != "NORMALIZED" {
		t.Fatalf("WaitForFrame's frame+raw shape = %q, want one frame attachment holding only NORMALIZED", sections)
	}
	if _, sections := splitFailureText("plain error: no artifacts here"); len(sections) != 0 {
		t.Fatalf("an error with no artifact headers produced sections: %v", sections)
	}
}

// TestGodogFormatAddsAllureOnlyWhenTheEnvVarIsSet: DECK_GODOG_ALLURE adds the
// allure formatter next to (never instead of) pretty and JUnit.
func TestGodogFormatAddsAllureOnlyWhenTheEnvVarIsSet(t *testing.T) {
	t.Setenv("DECK_GODOG_JUNIT", "")
	t.Setenv(allureDirEnv, "")
	if got := godogFormat(); got != "pretty" {
		t.Fatalf("godogFormat() = %q, want exactly pretty", got)
	}
	dir := filepath.Join(t.TempDir(), "allure-results")
	junit := filepath.Join(t.TempDir(), "junit.xml")
	t.Setenv("DECK_GODOG_JUNIT", junit)
	t.Setenv(allureDirEnv, dir)
	want := "pretty,junit:" + junit + ",allure:" + filepath.Join(dir, "godog-allure-summary.txt")
	if got := godogFormat(); got != want {
		t.Fatalf("godogFormat() = %q, want %q", got, want)
	}
}

// TestAllureLabelsGroupFeaturesAndLiftTheAgentTagToSuite pins the Suites tree
// the report relies on: parentSuite is "features" and only that, the feature
// name is the suite, and an agent tag takes the suite level with the feature
// name under it as subSuite.
func TestAllureLabelsGroupFeaturesAndLiftTheAgentTagToSuite(t *testing.T) {
	doc := &allureFeatureDoc{name: "A feature", uri: "a.feature"}
	labelsFor := func(tags ...string) map[string]string {
		p := &godog.Scenario{}
		for _, name := range tags {
			// reflect: the tag type lives in a module go.mod lists as indirect,
			// and importing it would turn that into a direct dependency.
			tag := reflect.New(reflect.TypeOf(p.Tags).Elem().Elem())
			tag.Elem().FieldByName("Name").SetString(name)
			tags := reflect.ValueOf(&p.Tags).Elem()
			tags.Set(reflect.Append(tags, tag))
		}
		labels, _ := allureTagsToLabels(doc, p)
		got := map[string]string{}
		for _, l := range labels {
			if l.Name == "parentSuite" || l.Name == "suite" || l.Name == "subSuite" {
				if _, dup := got[l.Name]; dup {
					t.Fatalf("tags %v: label %s appears twice in %v", tags, l.Name, labels)
				}
				got[l.Name] = l.Value
			}
		}
		return got
	}
	if got := labelsFor("@slow"); got["parentSuite"] != "features" || got["suite"] != "A feature" || got["subSuite"] != "" {
		t.Fatalf("no agent tag: %v", got)
	}
	if got := labelsFor("@pi", "@slow"); got["parentSuite"] != "features" || got["suite"] != "pi" || got["subSuite"] != "A feature" {
		t.Fatalf("agent tag: %v", got)
	}
}
