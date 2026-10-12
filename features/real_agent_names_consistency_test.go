package features

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
)

// R254/R265: an explicit `--name` suppresses Claude's own ai-title, so a real
// scenario may assert an "ai-title" record only on a launch that passes no
// name. These non-live tests check the real_agent_names.feature setup against
// that rule without running any CLI.

var (
	explicitClaudeCreateStep = regexp.MustCompile(`creates claude session "[^"]+" with permission profile`)
	unnamedClaudeCreateStep  = regexp.MustCompile(`creates claude session unnamed, labelled "[^"]+", with permission profile`)
	aiTitleAssertionStep     = regexp.MustCompile(`holds an? "ai-title" record`)
	explicitTitleAssertion   = regexp.MustCompile(`holds an? "(custom-title|agent-name)" record`)
)

// claudeNameScenarioProblems returns one message per scenario of feature text
// whose Claude launch kind contradicts the transcript record it asserts.
func claudeNameScenarioProblems(t *testing.T, text string) []string {
	t.Helper()
	// Scenario bodies, split on the "Scenario:" keyword lines.
	type scenario struct {
		Name  string
		Steps []string
	}
	var scenarios []scenario
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "Scenario:"):
			scenarios = append(scenarios, scenario{Name: strings.TrimSpace(strings.TrimPrefix(trimmed, "Scenario:"))})
		case len(scenarios) > 0 && regexp.MustCompile(`^(Given|When|Then|And|But) `).MatchString(trimmed):
			last := &scenarios[len(scenarios)-1]
			last.Steps = append(last.Steps, trimmed)
		}
	}
	var problems []string
	for _, sc := range scenarios {
		var explicit, unnamed, aiTitle, explicitTitle bool
		for _, stepText := range sc.Steps {
			switch {
			case explicitClaudeCreateStep.MatchString(stepText):
				explicit = true
			case unnamedClaudeCreateStep.MatchString(stepText):
				unnamed = true
			case aiTitleAssertionStep.MatchString(stepText):
				aiTitle = true
			case explicitTitleAssertion.MatchString(stepText):
				explicitTitle = true
			}
		}
		if aiTitle && !unnamed {
			problems = append(problems, sc.Name+": asserts ai-title but does not launch Claude unnamed")
		}
		if aiTitle && explicit {
			problems = append(problems, sc.Name+": asserts ai-title on an explicitly named launch, which suppresses it")
		}
		if explicitTitle && !explicit {
			problems = append(problems, sc.Name+": asserts custom-title/agent-name without an explicit-name launch")
		}
	}
	return problems
}

func TestRealAgentNamesFeatureSetupMatchesExplicitNameSuppression(t *testing.T) {
	data, err := os.ReadFile("real_agent_names.feature")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if problems := claudeNameScenarioProblems(t, text); len(problems) != 0 {
		t.Fatalf("real_agent_names.feature contradicts explicit-name suppression: %v", problems)
	}
	// All three raw record/field pins are still present.
	for _, pin := range []string{
		`"ai-title" record whose "aiTitle" is a non-empty string`,
		`"custom-title" record whose "customTitle" is "pinned-claude-name"`,
		`"agent-name" record whose "agentName" is "pinned-claude-name"`,
	} {
		if !strings.Contains(text, pin) {
			t.Errorf("pin %q is missing from real_agent_names.feature", pin)
		}
	}
	if !strings.HasPrefix(text, "@real-agents\n") {
		t.Error("real_agent_names.feature lost its @real-agents tag")
	}
}

func TestClaudeNameScenarioCheckerRejectsContradictorySetups(t *testing.T) {
	const head = "Feature: f\n  Scenario: s\n    Given deck client \"A\" is started\n"
	cases := []struct {
		name, steps string
		want        int
	}{
		{"ai-title on an explicit launch", `    When deck client "A" creates claude session "n" with permission profile "safe"
    Then the real Claude transcript of session "n" holds an "ai-title" record whose "aiTitle" is a non-empty string
`, 2},
		{"ai-title on an unnamed launch", `    When deck client "A" creates claude session unnamed, labelled "n", with permission profile "safe"
    Then the real Claude transcript of session "n" holds an "ai-title" record whose "aiTitle" is a non-empty string
`, 0},
		{"custom-title on an explicit launch", `    When deck client "A" creates claude session "n" with permission profile "safe"
    Then the real Claude transcript of session "n" holds a "custom-title" record whose "customTitle" is "x"
`, 0},
		{"agent-name on an unnamed launch", `    When deck client "A" creates claude session unnamed, labelled "n", with permission profile "safe"
    Then the real Claude transcript of session "n" holds an "agent-name" record whose "agentName" is "x"
`, 1},
		{"ai-title with no launch at all", `    Then the real Claude transcript of session "n" holds an "ai-title" record whose "aiTitle" is a non-empty string
`, 1},
	}
	for _, c := range cases {
		if got := claudeNameScenarioProblems(t, head+c.steps); len(got) != c.want {
			t.Errorf("%s: %d problems %v, want %d", c.name, len(got), got, c.want)
		}
	}
}

// The launch kinds the scenarios rely on: an unnamed (blank-name) launch
// passes no name flag, an explicit one does.
func TestClaudeArgvNameFlagFollowsLaunchKind(t *testing.T) {
	in := agent.LaunchInput{ConversationID: "c-1", Profile: "safe"}
	argv, err := agent.Claude{}.Launch(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range argv {
		if a == "--name" || a == "-n" || strings.HasPrefix(a, "--name=") {
			t.Fatalf("unnamed launch argv carries a name flag: %v", argv)
		}
	}
	in.Name = "pinned"
	argv, err = agent.Claude{}.Launch(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(argv, " "), "--name=pinned") {
		t.Fatalf("explicit launch argv lacks --name: %v", argv)
	}
}
