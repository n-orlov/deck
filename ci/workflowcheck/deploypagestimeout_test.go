// deploypagestimeout_test.go pins that the actions/deploy-pages retry ladder
// in both publish jobs can actually be reached. actions/deploy-pages polls a
// deployment for its own `timeout` input (default 600000 ms, ten minutes)
// before it gives up and fails the step. With that default and a ten-minute
// job cap, one stalled Pages deployment ran the job out while attempt 1 was
// still polling: the job ended `cancelled`, attempts 2-4 never ran, and the
// whole push run concluded `cancelled`. So every attempt must carry an
// explicit, bounded `timeout`, and the worst case -- every attempt running to
// its own timeout plus every back-off sleep plus a minute of job setup --
// must fit inside the job's own `timeout-minutes`.
package workflowcheck

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// jobSetupAllowanceSeconds is the slack left for runner setup and action
// download ahead of the first attempt.
const jobSetupAllowanceSeconds = 60

type timedStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]string `yaml:"with"`
}

type timedJob struct {
	Name           string      `yaml:"name"`
	TimeoutMinutes string      `yaml:"timeout-minutes"`
	Steps          []timedStep `yaml:"steps"`
}

type timedWorkflow struct {
	Jobs map[string]timedJob `yaml:"jobs"`
}

var sleepSeconds = regexp.MustCompile(`^\s*sleep\s+(\d+)\s*$`)

// deployPagesWorstCaseSeconds returns the job's worst-case duration in
// seconds -- every deploy-pages attempt's own timeout, every back-off sleep,
// and the setup allowance -- plus the number of deploy-pages attempts and one
// problem per attempt that carries no explicit positive `timeout`.
func deployPagesWorstCaseSeconds(job timedJob) (seconds, attempts int, problems []string) {
	seconds = jobSetupAllowanceSeconds
	for _, s := range job.Steps {
		if strings.HasPrefix(s.Uses, "actions/deploy-pages@") {
			attempts++
			ms, err := strconv.Atoi(strings.TrimSpace(s.With["timeout"]))
			if err != nil || ms <= 0 {
				problems = append(problems, fmt.Sprintf("deploy-pages step %q has with.timeout %q, want an explicit positive millisecond count -- the action's 10-minute default outlives the job and the retry ladder never runs", s.Name, s.With["timeout"]))
				continue
			}
			seconds += (ms + 999) / 1000
			continue
		}
		if m := sleepSeconds.FindStringSubmatch(s.Run); m != nil {
			n, _ := strconv.Atoi(m[1])
			seconds += n
		}
	}
	return seconds, attempts, problems
}

func checkDeployPagesLadderFitsJobTimeout(t *testing.T, name, jobName string) {
	t.Helper()
	var wf timedWorkflow
	if err := yaml.Unmarshal(readWorkflowFile(t, name), &wf); err != nil {
		t.Fatalf("%s: parse YAML: %v", name, err)
	}
	var job *timedJob
	for id := range wf.Jobs {
		if j := wf.Jobs[id]; j.Name == jobName {
			job = &j
		}
	}
	if job == nil {
		t.Fatalf("%s: no job with name %q", name, jobName)
	}
	minutes, err := strconv.Atoi(job.TimeoutMinutes)
	if err != nil || minutes <= 0 {
		t.Fatalf("%s job %q: timeout-minutes %q, want a positive literal", name, jobName, job.TimeoutMinutes)
	}
	worst, attempts, problems := deployPagesWorstCaseSeconds(*job)
	if attempts < 2 {
		t.Fatalf("%s job %q: found %d deploy-pages attempt(s), want a retry ladder", name, jobName, attempts)
	}
	for _, p := range problems {
		t.Errorf("%s job %q: %s", name, jobName, p)
	}
	if limit := minutes * 60; worst > limit {
		t.Errorf("%s job %q: worst case %ds (every attempt at its timeout + back-off + %ds setup) exceeds timeout-minutes %d (%ds) -- the job would be cancelled before its last attempt could run", name, jobName, worst, jobSetupAllowanceSeconds, minutes, limit)
	}
}

func TestCIWorkflowPublishDeployPagesLadderFitsJobTimeout(t *testing.T) {
	checkDeployPagesLadderFitsJobTimeout(t, "ci.yml", "publish (Pages)")
}

func TestPagesPRPublishDeployPagesLadderFitsJobTimeout(t *testing.T) {
	checkDeployPagesLadderFitsJobTimeout(t, "pages-pr-publish.yml", "publish (Pages, PR)")
}

// TestDeployPagesWorstCaseFlagsDefaultTimeoutAndSumsLadder proves the check
// bites: attempts without an explicit timeout (the shape that cancelled the
// push run) are flagged, and explicit timeouts plus sleeps are summed.
func TestDeployPagesWorstCaseFlagsDefaultTimeoutAndSumsLadder(t *testing.T) {
	bare := timedJob{Steps: []timedStep{
		{Name: "attempt 1", Uses: "actions/deploy-pages@x"},
		{Name: "back off", Run: "sleep 30"},
		{Name: "attempt 2", Uses: "actions/deploy-pages@x"},
	}}
	if _, attempts, problems := deployPagesWorstCaseSeconds(bare); attempts != 2 || len(problems) != 2 {
		t.Fatalf("no with.timeout on two attempts: attempts=%d problems=%d, want 2 and 2", attempts, len(problems))
	}

	timed := timedJob{Steps: []timedStep{
		{Name: "attempt 1", Uses: "actions/deploy-pages@x", With: map[string]string{"timeout": "120000"}},
		{Name: "back off", Run: "sleep 30"},
		{Name: "attempt 2", Uses: "actions/deploy-pages@x", With: map[string]string{"timeout": "600000"}},
	}}
	seconds, _, problems := deployPagesWorstCaseSeconds(timed)
	if len(problems) != 0 {
		t.Fatalf("explicit timeouts flagged: %v", problems)
	}
	if want := jobSetupAllowanceSeconds + 120 + 30 + 600; seconds != want {
		t.Fatalf("worst case = %ds, want %ds", seconds, want)
	}
	if seconds <= 10*60 {
		t.Fatalf("a 10-minute attempt plus setup must not fit a 10-minute job: %ds", seconds)
	}
}
