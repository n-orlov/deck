// nightlyscan_test.go is task 017's probe (R190): the scheduled (and
// manually dispatched) path of ci.yml runs both vulnerability scanners
// -- govulncheck and trivy, through ci/quality.sh -- on fresh databases,
// a failure of that step fails the job, and the notify job reads the
// job's result; and the cron slot is the one the nightly has always had.
package workflowcheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const nightlyCron = "17 3 * * *"

type nightlyStep struct {
	Name            string `yaml:"name"`
	ID              string `yaml:"id"`
	If              string `yaml:"if"`
	Run             string `yaml:"run"`
	ContinueOnError bool   `yaml:"continue-on-error"`
}

type nightlyJob struct {
	Needs any           `yaml:"needs"`
	If    string        `yaml:"if"`
	Steps []nightlyStep `yaml:"steps"`
}

type nightlyWorkflow struct {
	On struct {
		Schedule []struct {
			Cron string `yaml:"cron"`
		} `yaml:"schedule"`
	} `yaml:"on"`
	Jobs map[string]nightlyJob `yaml:"jobs"`
}

func loadNightlyWorkflow(t *testing.T) nightlyWorkflow {
	t.Helper()
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read ci.yml: %v", err)
	}
	var wf nightlyWorkflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("parse ci.yml: %v", err)
	}
	return wf
}

// runsOnSchedule reports whether a step with this `if:` executes on a
// schedule run: no condition at all, or one naming the schedule event.
func runsOnSchedule(cond string) bool {
	return cond == "" || strings.Contains(cond, "github.event_name == 'schedule'")
}

func TestNightlyCronSlotIsUnchanged(t *testing.T) {
	wf := loadNightlyWorkflow(t)
	if len(wf.On.Schedule) != 1 || wf.On.Schedule[0].Cron != nightlyCron {
		t.Fatalf("ci.yml on.schedule = %+v, want exactly one entry %q", wf.On.Schedule, nightlyCron)
	}
}

func TestNightlyScheduledPathRescansWithBothScannersOnFreshDatabases(t *testing.T) {
	wf := loadNightlyWorkflow(t)
	suite, ok := wf.Jobs["suite"]
	if !ok {
		t.Fatal("ci.yml has no suite job")
	}

	quality, refresh := -1, -1
	for i, s := range suite.Steps {
		if !runsOnSchedule(s.If) {
			continue
		}
		if strings.Contains(s.Run, "ci/quality.sh") && quality < 0 {
			quality = i
		}
		if strings.Contains(s.Run, "/go-cache/trivy/db") && strings.Contains(s.Run, "/go-cache/xdg-cache/govulncheck") && refresh < 0 {
			refresh = i
		}
	}
	if quality < 0 {
		t.Fatal("suite job: no step that runs on schedule invokes ci/quality.sh (govulncheck + trivy)")
	}
	if refresh < 0 {
		t.Fatal("suite job: no scheduled step drops both the trivy and the govulncheck database caches, so the nightly rescan may read a stale database")
	}
	if refresh > quality {
		t.Errorf("suite job: the database refresh (step %d) must come before the quality step (step %d)", refresh, quality)
	}
	if refresh >= 0 && suite.Steps[refresh].If == "" {
		t.Errorf("suite job: the database refresh step runs on every event; it must be limited to schedule/workflow_dispatch so pushes keep their warm cache")
	}

	// The scanner gates are what the quality step runs: both must be on.
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "ci", "quality.json"))
	if err != nil {
		t.Fatalf("read quality.json: %v", err)
	}
	var cfg struct {
		Trivy struct {
			Enabled bool `json:"enabled"`
		} `json:"trivy"`
		Govulncheck struct {
			Enabled bool `json:"enabled"`
		} `json:"govulncheck"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse quality.json: %v", err)
	}
	if !cfg.Trivy.Enabled || !cfg.Govulncheck.Enabled {
		t.Errorf("quality.json: trivy.enabled=%v govulncheck.enabled=%v, want both on", cfg.Trivy.Enabled, cfg.Govulncheck.Enabled)
	}

	// A red quality step must turn the job red, and a red suite job must
	// reach the notify job.
	q := suite.Steps[quality]
	var failStep *nightlyStep
	for i := range suite.Steps {
		if suite.Steps[i].Run == "exit 1" {
			failStep = &suite.Steps[i]
		}
	}
	if q.ID == "" {
		t.Fatal("suite job: the quality step has no id, so nothing can read its outcome")
	}
	if q.ContinueOnError {
		if failStep == nil || !strings.Contains(failStep.If, "steps."+q.ID+".outcome == 'failure'") {
			t.Errorf("suite job: the quality step is continue-on-error but no `exit 1` step reads steps.%s.outcome", q.ID)
		}
	}
	notify := wf.Jobs["notify"]
	if !strings.Contains(notify.If, "schedule") || !strings.Contains(notify.If, "needs.*.result, 'failure'") {
		t.Errorf("notify job `if:` does not fire for a red scheduled run: %q", notify.If)
	}
}
