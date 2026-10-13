// cihygiene_test.go pins the ci-hygiene behaviour of ci.yml (task 022):
// the self-hosted runners are a scarce queue, so a push run that a newer
// push superseded, and a push that touches only documentation, must not
// occupy them. The tests PARSE ci.yml and EVALUATE its expressions with the
// evaluator in concurrencygroups_test.go; none greps raw expression text.
//
//   - TestCIWorkflowPushTriggerIgnoresOnlyDocsPaths: `paths-ignore` is on
//     the `push` trigger alone and lists exactly the docs paths.
//   - TestCIWorkflowSelfHostedJobsAfterSuiteSkipASupersededPush: every
//     self-hosted job after `suite` skips for a cancelled push, and
//     `notify` still fires for the red cases that matter.
package workflowcheck

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCIWorkflowPushTriggerIgnoresOnlyDocsPaths(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read ci.yml: %v", err)
	}
	var wf struct {
		On map[string]yaml.Node `yaml:"on"`
	}
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("parse ci.yml: %v", err)
	}
	if _, ok := wf.On["push"]; !ok {
		t.Fatalf("ci.yml: no push trigger")
	}
	triggerCfg := func(name string) map[string]any {
		cfg := map[string]any{}
		n := wf.On[name]
		if n.Kind == yaml.MappingNode {
			if err := n.Decode(&cfg); err != nil {
				t.Fatalf("decode trigger %q: %v", name, err)
			}
		}
		return cfg
	}
	for name := range wf.On {
		if name == "push" {
			continue
		}
		cfg := triggerCfg(name)
		for _, key := range []string{"paths", "paths-ignore"} {
			if _, has := cfg[key]; has {
				t.Errorf("ci.yml: trigger %q carries %q; only push may filter paths (pull_request, schedule and workflow_dispatch always run)", name, key)
			}
		}
	}
	push := triggerCfg("push")
	if _, has := push["paths"]; has {
		t.Errorf("ci.yml: push carries `paths`; it must use `paths-ignore` with the docs globs only")
	}
	rawList, ok := push["paths-ignore"].([]any)
	if !ok {
		t.Fatalf("ci.yml: push.paths-ignore missing or not a list: %v", push["paths-ignore"])
	}
	var got []string
	for _, v := range rawList {
		s, _ := v.(string)
		got = append(got, s)
	}
	want := []string{"**.md", "docs/**", "prds/**"}
	sorted := append([]string(nil), got...)
	sort.Strings(sorted)
	if !reflect.DeepEqual(sorted, want) {
		t.Errorf("push.paths-ignore = %q, want exactly %q", got, want)
	}
	for _, g := range got {
		for _, code := range []string{"ci/", "cmd/", "internal/", "features/", ".github/", "badges/"} {
			if strings.HasPrefix(g, code) {
				t.Errorf("push.paths-ignore entry %q starts with %q: code paths must start a run", g, code)
			}
		}
		if strings.Contains(g, "go.mod") {
			t.Errorf("push.paths-ignore entry %q names go.mod: dependency changes must start a run", g)
		}
	}
}

func decodeStrings(t *testing.T, n yaml.Node) []string {
	t.Helper()
	switch n.Kind {
	case yaml.ScalarNode:
		return []string{n.Value}
	case yaml.SequenceNode:
		var out []string
		if err := n.Decode(&out); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		return out
	}
	return nil
}

// selfHostedAfterSuite returns the ids of the self-hosted jobs that run
// after `suite`: those whose `needs`, followed transitively, reach it.
func selfHostedAfterSuite(t *testing.T, jobs map[string]map[string]yaml.Node) []string {
	t.Helper()
	var reaches func(id string, seen map[string]bool) bool
	reaches = func(id string, seen map[string]bool) bool {
		if seen[id] {
			return false
		}
		seen[id] = true
		for _, dep := range decodeStrings(t, jobs[id]["needs"]) {
			if dep == "suite" || reaches(dep, seen) {
				return true
			}
		}
		return false
	}
	var out []string
	for id, fields := range jobs {
		if id == "suite" || !reaches(id, map[string]bool{}) {
			continue
		}
		for _, r := range decodeStrings(t, fields["runs-on"]) {
			if r == "self-hosted" {
				out = append(out, id)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

func TestCIWorkflowSelfHostedJobsAfterSuiteSkipASupersededPush(t *testing.T) {
	_, jobs := loadCIWorkflowRaw(t)
	after := selfHostedAfterSuite(t, jobs)
	for _, need := range []string{"notify", "report"} {
		found := false
		for _, id := range after {
			found = found || id == need
		}
		if !found {
			t.Fatalf("self-hosted jobs after suite = %v, want to include %q", after, need)
		}
	}

	type run struct {
		name, event, lint, suite string
	}
	ifOf := func(id string) string {
		n := jobs[id]["if"]
		if n.Kind != yaml.ScalarNode || n.Value == "" {
			t.Fatalf("ci.yml: job %q has no job-level if:", id)
		}
		return n.Value
	}
	eval := func(id string, r run) bool {
		ctx := map[string]string{
			"github.event_name": r.event,
			"github.ref":        "refs/heads/main",
			"github.repository": "n-orlov/deck",
			"github.event.pull_request.head.repo.full_name": "",
			"needs.lint.result":                             r.lint,
			"needs.suite.result":                            r.suite,
			"needs.report.result":                           "skipped",
		}
		return evalGhBoolTemplate(ifOf(id), ctx)
	}

	// A push to main superseded by a newer push: suite (or lint) cancelled.
	for _, id := range after {
		for _, r := range []run{
			{"push, suite cancelled", "push", "success", "cancelled"},
			{"push, lint cancelled", "push", "cancelled", "skipped"},
		} {
			if eval(id, r) {
				t.Errorf("job %q runs for %s: a superseded push run must not spend a self-hosted slot or notify", id, r.name)
			}
		}
	}

	for _, c := range []struct {
		r    run
		want bool
	}{
		{run{"push, suite failure", "push", "success", "failure"}, true},
		{run{"push, lint failure", "push", "failure", "skipped"}, true},
		{run{"schedule, suite failure", "schedule", "success", "failure"}, true},
		{run{"schedule, suite cancelled", "schedule", "success", "cancelled"}, true},
		{run{"workflow_dispatch, suite failure", "workflow_dispatch", "success", "failure"}, true},
		{run{"workflow_dispatch, suite cancelled", "workflow_dispatch", "success", "cancelled"}, true},
		{run{"push, all green", "push", "success", "success"}, false},
	} {
		if got := eval("notify", c.r); got != c.want {
			t.Errorf("notify for %s = %v, want %v", c.r.name, got, c.want)
		}
	}
}
