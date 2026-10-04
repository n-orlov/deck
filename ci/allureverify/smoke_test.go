package allureverify

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// R196: ci/allure-smoke.sh runs `allure generate` over the formatter fixture's
// results and fails unless the generated report has the fixture's steps and
// its @gh-61 link. The Allure CLI is a fake on PATH whose `generate` writes
// the test-case JSON (Allure's data/test-cases/*.json shape) it is told to, so
// each way the check can fail is reached without Java.

type smokeCase map[string]any

func smokeStep(name string) map[string]any { return map[string]any{"name": name, "status": "passed"} }

func goodSmokeCases() []smokeCase {
	return []smokeCase{
		{
			"name":      "passes with tags",
			"testStage": map[string]any{"steps": []any{smokeStep("a passing step"), smokeStep("a passing step"), smokeStep("a passing step")}},
			"links":     []any{map[string]any{"name": "gh-61", "type": "issue", "url": "https://github.com/n-orlov/deck/issues/61"}},
		},
		{
			"name":      "fails with an attachment",
			"testStage": map[string]any{"steps": []any{smokeStep("a passing step"), smokeStep("the harness fails with a frame")}},
		},
	}
}

// runSmoke runs the script with the given generated cases; results controls
// whether the fixture dir has a *-result.json, failGenerate makes the fake
// `allure generate` exit non-zero.
func runSmoke(t *testing.T, cases []smokeCase, results, failGenerate bool) (string, error) {
	t.Helper()
	root := repositoryRoot(t)
	work := t.TempDir()

	casesDir := filepath.Join(work, "cases")
	if err := os.MkdirAll(casesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, c := range cases {
		raw, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(casesDir, string(rune('a'+i))+".json"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	bin := filepath.Join(work, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	fake := "#!/bin/sh\ncase \"$1\" in\n--version) echo fake-allure;;\ngenerate)\n[ -z \"$FAKE_FAIL\" ] || { echo generate failed >&2; exit 3; }\nmkdir -p \"$5/data/test-cases\"; cp \"$FAKE_CASES\"/*.json \"$5/data/test-cases/\" 2>/dev/null || true;;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "allure"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}

	fixture := filepath.Join(work, "fixture")
	if err := os.MkdirAll(filepath.Join(fixture, "allure-results"), 0o755); err != nil {
		t.Fatal(err)
	}
	if results {
		if err := os.WriteFile(filepath.Join(fixture, "allure-results", "x-result.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cmd := exec.Command("sh", filepath.Join(root, "ci", "allure-smoke.sh"), fixture, filepath.Join(work, "report"))
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "FAKE_CASES="+casesDir, "TMPDIR="+work)
	if failGenerate {
		cmd.Env = append(cmd.Env, "FAKE_FAIL=1")
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestAllureSmokePassesWhenTheReportHasTheFixtureStepsAndLink(t *testing.T) {
	out, err := runSmoke(t, goodSmokeCases(), true, false)
	if err != nil {
		t.Fatalf("a report with the fixture's steps and @gh-61 link must pass: %v\n%s", err, out)
	}
	if !strings.Contains(out, "ci/allure-smoke.sh: ok") {
		t.Fatalf("missing the ok line:\n%s", out)
	}
}

func TestAllureSmokeFailsWhenGenerationFails(t *testing.T) {
	out, err := runSmoke(t, goodSmokeCases(), true, true)
	if err == nil {
		t.Fatalf("a failing `allure generate` must fail the smoke check:\n%s", out)
	}
	if !strings.Contains(out, "generate failed") {
		t.Fatalf("the failure should be the generate failure:\n%s", out)
	}
}

func TestAllureSmokeFailsWithoutFixtureResults(t *testing.T) {
	out, err := runSmoke(t, goodSmokeCases(), false, false)
	if err == nil || !strings.Contains(out, "wrote nothing") {
		t.Fatalf("an empty fixture dir must fail, got err=%v:\n%s", err, out)
	}
}

func TestAllureSmokeFailsWhenTheReportLacksTheSteps(t *testing.T) {
	cases := goodSmokeCases()
	cases[0]["testStage"] = map[string]any{"steps": []any{}}
	out, err := runSmoke(t, cases, true, false)
	if err == nil || !strings.Contains(out, "3 steps") {
		t.Fatalf("a report without the steps must fail on them, got err=%v:\n%s", err, out)
	}
}

func TestAllureSmokeFailsWhenTheReportLacksTheGHLink(t *testing.T) {
	cases := goodSmokeCases()
	delete(cases[0], "links")
	out, err := runSmoke(t, cases, true, false)
	if err == nil || !strings.Contains(out, "@gh-61 link") {
		t.Fatalf("a report without the @gh-61 link must fail on it, got err=%v:\n%s", err, out)
	}
}

func TestAllureSmokeFailsWhenTheReportHasNoTestCases(t *testing.T) {
	out, err := runSmoke(t, nil, true, false)
	if err == nil || !strings.Contains(out, "no data/test-cases") {
		t.Fatalf("an empty report must fail, got err=%v:\n%s", err, out)
	}
}
