// qualityreport_test.go is cure-01-01's probe (R187): every on quality gate's
// report section must reach the suite job's GITHUB_STEP_SUMMARY on every run,
// pass or fail, across the ci/run.sh sibling boundary. ci/quality.sh writes
// <outdir>/quality-report.txt; a runner-side `if: always()` step in ci.yml
// appends it to the summary. These tests fail if either end is cut.
package workflowcheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type reportStep struct {
	Name            string `yaml:"name"`
	ID              string `yaml:"id"`
	If              string `yaml:"if"`
	Run             string `yaml:"run"`
	ContinueOnError bool   `yaml:"continue-on-error"`
}

type reportJob struct {
	Steps []reportStep `yaml:"steps"`
}

func suiteSteps(t *testing.T, ciYAML []byte) []reportStep {
	t.Helper()
	var wf struct {
		Jobs map[string]reportJob `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(ciYAML, &wf); err != nil {
		t.Fatalf("parse ci.yml: %v", err)
	}
	suite, ok := wf.Jobs["suite"]
	if !ok {
		t.Fatalf("ci.yml has no suite job")
	}
	return suite.Steps
}

// qualityReportDeliveryProblems returns what is wrong with the suite job's
// delivery of the quality report to the job summary (empty = fine).
func qualityReportDeliveryProblems(steps []reportStep) []string {
	var problems []string
	var delivery *reportStep
	qualityIdx, deliveryIdx := -1, -1
	for i := range steps {
		s := &steps[i]
		if qualityIdx < 0 && strings.Contains(s.Run, "ci/run.sh ci/quality.sh") {
			qualityIdx = i
		}
		if strings.Contains(s.Run, "quality-report.txt") && strings.Contains(s.Run, "GITHUB_STEP_SUMMARY") {
			delivery, deliveryIdx = s, i
		}
	}
	if qualityIdx < 0 {
		return []string{"no suite step runs ci/quality.sh"}
	}
	if delivery == nil {
		return []string{"no suite step appends quality-report.txt to GITHUB_STEP_SUMMARY"}
	}
	if !strings.Contains(delivery.If, "always()") {
		problems = append(problems, "the delivery step is not `if: always()`, so a failing gate would skip it")
	}
	if deliveryIdx < qualityIdx {
		problems = append(problems, "the delivery step runs before the quality step")
	}
	if !steps[qualityIdx].ContinueOnError {
		problems = append(problems, "the quality step is not continue-on-error, so a failing gate would skip the delivery step")
	}
	return problems
}

func TestQualityReportReachesJobSummary(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range qualityReportDeliveryProblems(suiteSteps(t, raw)) {
		t.Error(p)
	}
	// The gate exit status must still fail the job.
	var failStep *reportStep
	steps := suiteSteps(t, raw)
	for i := range steps {
		if strings.Contains(steps[i].If, "steps.quality.outcome == 'failure'") {
			failStep = &steps[i]
		}
	}
	if failStep == nil || !strings.Contains(failStep.Run, "exit 1") {
		t.Errorf("no step fails the job on steps.quality.outcome == 'failure'")
	}
	// quality.sh must write the file the delivery step reads.
	sh, err := os.ReadFile(filepath.Join(root, "ci", "quality.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sh), `quality-report.txt`) {
		t.Errorf("ci/quality.sh does not write quality-report.txt")
	}
}

// TestQualityReportReachesJobSummaryOnSeededFailingGate mutates ci.yml the
// ways a regression would (delivery step removed, `always()` dropped, the
// quality step no longer continue-on-error, delivery moved ahead of the
// gate) and requires each to be reported; and it runs the real delivery
// step's script against a seeded failing-gate report.
func TestQualityReportReachesJobSummaryOnSeededFailingGate(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	good := suiteSteps(t, raw)
	if p := qualityReportDeliveryProblems(good); len(p) != 0 {
		t.Fatalf("checked-in ci.yml already fails the probe: %v", p)
	}

	clone := func() []reportStep { return append([]reportStep(nil), good...) }
	find := func(steps []reportStep, pred func(reportStep) bool) int {
		for i, s := range steps {
			if pred(s) {
				return i
			}
		}
		t.Fatalf("step not found")
		return -1
	}
	isDelivery := func(s reportStep) bool { return strings.Contains(s.Run, "quality-report.txt") }
	isQuality := func(s reportStep) bool { return strings.Contains(s.Run, "ci/run.sh ci/quality.sh") }

	removed := clone()
	i := find(removed, isDelivery)
	removed = append(removed[:i], removed[i+1:]...)

	noAlways := clone()
	noAlways[find(noAlways, isDelivery)].If = "success()"

	noContinue := clone()
	noContinue[find(noContinue, isQuality)].ContinueOnError = false

	moved := clone()
	di, qi := find(moved, isDelivery), find(moved, isQuality)
	moved[di], moved[qi] = moved[qi], moved[di]

	for name, steps := range map[string][]reportStep{
		"delivery step removed": removed, "always() dropped": noAlways,
		"quality step not continue-on-error": noContinue, "delivery before quality": moved,
	} {
		if len(qualityReportDeliveryProblems(steps)) == 0 {
			t.Errorf("mutation %q was not detected", name)
		}
	}

	// Run the real delivery step against a seeded failing-gate report.
	script := good[find(good, isDelivery)].Run
	work := t.TempDir()
	seeded := "=== crap gate ===\nfoo.go:1 f: 99 over the ceiling 10\nwhat to do: refactor the named offender(s)\n"
	if err := os.WriteFile(filepath.Join(work, "quality-report.txt"), []byte(seeded), 0o644); err != nil {
		t.Fatal(err)
	}
	summary := filepath.Join(work, "summary.md")
	run := func(outdir string) string {
		_ = os.Remove(summary)
		cmd := exec.Command("sh", "-c", script)
		cmd.Env = append(os.Environ(), "DECK_CI_OUT="+outdir, "GITHUB_STEP_SUMMARY="+summary)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("delivery step failed: %v\n%s", err, out)
		}
		b, err := os.ReadFile(summary)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	got := run(work)
	for _, want := range []string{"=== crap gate ===", "over the ceiling 10", "what to do:"} {
		if !strings.Contains(got, want) {
			t.Errorf("job summary lacks %q:\n%s", want, got)
		}
	}
	// A missing report still yields a summary note, never a failed step.
	if got := run(filepath.Join(work, "nonexistent")); !strings.Contains(got, "no gate report") {
		t.Errorf("missing report not noted in the summary:\n%s", got)
	}
}
