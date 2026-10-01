package features

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// TestNoLoneStartingFrameWaitInGoSource (task 026) is the Go-source
// counterpart to R150's feature-text scan (r150_frame_settle_scan_test.go):
// it guards every *.go file under features/ (step functions, helpers, and
// sc.Before/sc.After/StepContext().After hooks alike) against a frame wait
// that accepts the transient status word "starting" ALONE, in either shape
// task 026's audit (artifacts/026/audit.md) enumerated:
//
//   - a literal single-target wait, `WaitForFrame(ctx, _, "starting")` or
//     `WaitForFrameGone(ctx, _, "starting")`;
//   - a WaitForFrameFunc predicate whose body checks the literal word
//     "starting" but never "running" anywhere in the same predicate, so it
//     can only ever be satisfied by the transient render.
//
// Either shape races SPEC §7's shell-only fast-forward rule exactly as
// task 006 found in clientCreatesShellSession: a shell row can be promoted
// starting->running within the very same pty read that first makes it
// visible, so a wait insisting on the literal word "starting" alone can
// time out even though the row settled correctly (inventory mechanism M1).
//
// The hazard is not shell-only: SPEC §7 also moves an AGENT row out of
// "starting" on its first agent signal, on a clean pane exit (the plain
// fake-claude fixture exits about half a second after its banner) or on a
// non-zero one (a failing pre_launch), so an agent create/resume's own
// "starting" render is just as transient (agent_starting_observable_test.go).
// There is therefore no Go-source exemption at all: every hit must be
// converted onto the durable M1 observable (waitForSettledSessionRow, the
// store's own recorded transition, or an audited event --
// waitForAgentCreateRecorded for an agent create), never allowlisted to
// silence this guard.
func TestNoLoneStartingFrameWaitInGoSource(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob *.go: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no *.go files found under features/ -- glob pattern is wrong")
	}
	sort.Strings(files)

	var violations []string
	for _, path := range files {
		vs, err := scanGoFileForLoneStartingFrameWait(path)
		if err != nil {
			t.Fatalf("scan %s: %v", path, err)
		}
		violations = append(violations, vs...)
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("task 026: %d frame wait(s) on the lone transient \"starting\" status word -- convert onto the durable M1 observable (waitForSettledSessionRow, the store's own transition, or an audited event such as waitForAgentCreateRecorded), never add an exemption to silence it:\n%s",
			len(violations), strings.Join(violations, "\n"))
	}
}

var (
	goFuncHeaderRe      = regexp.MustCompile(`^func\s+(?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	goLiteralStartingRe = regexp.MustCompile(`\bWaitForFrame(?:Gone)?\(\s*\w+\s*,\s*(?:true|false|[A-Za-z_][A-Za-z0-9_]*)\s*,\s*"starting"\s*\)`)
	goFrameFuncOpenRe   = regexp.MustCompile(`\bWaitForFrameFunc\(`)

	goStringLiteralStart = `"starting"`
	// goRunningRefRe matches ANY mention of "running" in a predicate body --
	// a quoted literal, part of an identifier (startingOrRunningRowGlyphs), or
	// a variable built from a format string
	// (waitForCreatedSessionPastStarting's own
	// `running := sessionName + " running"`) -- all of which make the
	// predicate accept something other than the lone word "starting".
	goRunningRefRe = regexp.MustCompile(`(?i)running`)
)

// scanGoFileForLoneStartingFrameWait applies
// TestNoLoneStartingFrameWaitInGoSource's rule to one features/*.go file,
// line by line, tracking the enclosing top-level function so a hit names
// the function it sits in. A line whose trimmed
// text starts with "//" is a comment and never matched -- the audit
// (artifacts/026/audit.md) confirmed every narrative "starting" mention
// left in features/*.go after task 026's conversions is exactly this
// shape.
func scanGoFileForLoneStartingFrameWait(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(raw), "\n")

	var out []string
	currentFunc := ""
	// braceDepth of an open WaitForFrameFunc predicate literal we are
	// currently inside, and the lines collected for it so far; 0 means not
	// currently inside one. Tracked by counting unmatched "{" vs "}" on
	// each line from the point the predicate's own "func(...) bool {"
	// opened, which is adequate for this package's own one-liner-bodied
	// predicates and the handful of multi-line ones alike.
	inPredicate := false
	predicateDepth := 0
	predicateStartLine := 0
	var predicateLines []string

	flushPredicate := func() {
		if !inPredicate {
			return
		}
		body := strings.Join(predicateLines, "\n")
		if strings.Contains(body, goStringLiteralStart) && !goRunningRefRe.MatchString(body) {
			out = append(out, fmt.Sprintf("%s:%d: func %s's WaitForFrameFunc predicate checks the lone transient \"starting\" word with no \"running\" anywhere in the same predicate",
				path, predicateStartLine, currentFunc))
		}
		inPredicate = false
		predicateDepth = 0
		predicateLines = nil
	}

	for i, l := range lines {
		ln := i + 1
		trimmed := strings.TrimSpace(l)

		if m := goFuncHeaderRe.FindStringSubmatch(l); m != nil {
			flushPredicate()
			currentFunc = m[1]
		}

		if inPredicate {
			predicateLines = append(predicateLines, l)
			predicateDepth += strings.Count(l, "{") - strings.Count(l, "}")
			if predicateDepth <= 0 {
				flushPredicate()
			}
			continue
		}

		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		if goFrameFuncOpenRe.MatchString(l) && strings.Contains(l, "func(") {
			inPredicate = true
			predicateStartLine = ln
			predicateDepth = strings.Count(l, "{") - strings.Count(l, "}")
			predicateLines = []string{l}
			if predicateDepth <= 0 {
				flushPredicate()
			}
			continue
		}

		if goLiteralStartingRe.MatchString(l) {
			out = append(out, fmt.Sprintf("%s:%d: func %s waits for a frame on the lone transient \"starting\" word",
				path, ln, currentFunc))
		}
	}
	flushPredicate()

	return out, nil
}

// TestNoLoneStartingFrameWaitInFeatureText (task 026) is the Gherkin half of
// the same guard: every *.feature step that has a deck client wait on, or
// read, the quoted status word "starting" (`screen contains "starting"`,
// `row "X" contains "starting"`, their "within ..." forms, a token check on
// text "starting") is a frame sync point on a transient render unless it is
// in startingFeatureFrameWaitExemptions. A create or resume is synchronised
// on `the audit log records session "X" entering starting N times`
// (auditRecordsSessionEnteringStartingNTimes) instead.
func TestNoLoneStartingFrameWaitInFeatureText(t *testing.T) {
	files, err := filepath.Glob("*.feature")
	if err != nil {
		t.Fatalf("glob *.feature: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no *.feature files found under features/ -- glob pattern is wrong")
	}
	sort.Strings(files)
	var violations []string
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, hit := range featureStartingFrameSteps(string(raw)) {
			if _, ok := startingFeatureFrameWaitExemptions[path][hit.scenario][hit.step]; ok {
				continue
			}
			violations = append(violations, fmt.Sprintf("%s:%d: scenario %q: %s", path, hit.line, hit.scenario, hit.step))
		}
	}
	if len(violations) > 0 {
		t.Fatalf("task 026: %d feature step(s) sync on a deck client frame showing the lone transient \"starting\" status word -- use `the audit log records session \"X\" entering starting N times` (or another durable observable), never add an exemption to silence it:\n%s",
			len(violations), strings.Join(violations, "\n"))
	}
}

// startingFeatureFrameWaitExemptions keys (file, scenario, step text) to the
// audit's one-line reason (artifacts/026/audit.md). Each entry is a step
// whose scenario is ABOUT the starting status itself and whose row provably
// cannot leave "starting" while the step runs; that premise is re-checked
// mechanically in TestStartingFeatureFrameWaitExemptionsAreGrounded.
var startingFeatureFrameWaitExemptions = map[string]map[string]map[string]string{
	"lease_race.feature": {
		"three clients racing resume on one row produce exactly one launch": {
			`And within one configured reconcile interval deck client "A" row "race target" contains "starting"`: startingHeldReason,
			`And within one configured reconcile interval deck client "B" row "race target" contains "starting"`: startingHeldReason,
			`And within one configured reconcile interval deck client "C" row "race target" contains "starting"`: startingHeldReason,
		},
	},
	"status_theme.feature": {
		"the starting status token colours the starting status word": {
			`Then within one configured reconcile interval deck client "A" screen contains "starting"`: startingHeldReason,
			`And deck client "A" text "starting" has foreground token "starting"`:                      startingHeldReason,
		},
	},
}

const startingHeldReason = "the long-running fake claude never signals or exits unaided, reconcile never promotes an agent row (reconcile.go: liveness only), and no config shortens stale_after below the probe floor -- the row stays starting until the scenario itself fires a hook"

type featureStartingStep struct {
	line           int
	scenario, step string
}

var featureStepKeywordRe = regexp.MustCompile(`^(Given|When|Then|And|But|\*)\s`)

// featureStartingFrameSteps returns every non-comment step line in a
// feature file that names a deck client and carries the quoted word
// "starting" as a whole argument, with its enclosing Scenario (or
// "Background").
func featureStartingFrameSteps(src string) []featureStartingStep {
	var out []featureStartingStep
	scenario := ""
	for i, l := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(l)
		if name, ok := featureBlockName(trimmed); ok {
			scenario = name
			continue
		}
		if !featureStepKeywordRe.MatchString(trimmed) {
			continue
		}
		if strings.Contains(trimmed, `deck client "`) && strings.Contains(trimmed, `"starting"`) {
			out = append(out, featureStartingStep{line: i + 1, scenario: scenario, step: trimmed})
		}
	}
	return out
}

func featureBlockName(trimmed string) (string, bool) {
	for _, kw := range []string{"Scenario Outline:", "Scenario:", "Background:"} {
		if rest, ok := strings.CutPrefix(trimmed, kw); ok {
			if kw == "Background:" {
				return "Background", true
			}
			return strings.TrimSpace(rest), true
		}
	}
	return "", false
}

// featureScenarioSteps returns the Background steps and the named
// scenario's steps (in order) of a feature file.
func featureScenarioSteps(src, scenario string) (background, steps []string) {
	block := ""
	for _, l := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(l)
		if name, ok := featureBlockName(trimmed); ok {
			block = name
			continue
		}
		if !featureStepKeywordRe.MatchString(trimmed) {
			continue
		}
		switch block {
		case "Background":
			background = append(background, trimmed)
		case scenario:
			steps = append(steps, trimmed)
		}
	}
	return background, steps
}

// TestStartingFeatureFrameWaitExemptionsAreGrounded re-checks every
// startingFeatureFrameWaitExemptions entry's premise against the feature
// text and the product defaults, so an edit that makes an exempted
// "starting" render transient fails here:
//   - Background+scenario install the LONG-RUNNING fake claude (command
//     mode: it never prints a hook or exits until told to) and no
//     short-lived fake agent at all;
//   - nothing configures stale_after (or any deck config), and
//     config.DefaultStaleAfter is well above the default UI wait, so the
//     row never becomes probe-eligible while the step waits;
//   - no step between the one that last put the row into starting (a
//     create, a resume, or a seeded "starting" status) and the exempted
//     one fires a hook, sends the fixture an exit, or crashes/kills the
//     pane;
//   - the exempted step text still exists in that scenario.
func TestStartingFeatureFrameWaitExemptionsAreGrounded(t *testing.T) {
	if config.DefaultStaleAfter < 2*defaultWaitDeadline/3 {
		t.Fatalf("config.DefaultStaleAfter = %v is no longer far above the %v default UI wait -- an exempted agent row could now be probed out of starting; re-audit startingFeatureFrameWaitExemptions", config.DefaultStaleAfter, defaultWaitDeadline)
	}
	shortLivedRe := regexp.MustCompile(`\ba fake "[a-z]+" binary is on PATH`)
	configRe := regexp.MustCompile(`(?i)stale_after|probes quickly|deck config`)
	startsRowRe := regexp.MustCompile(`presses r on session|race pressing r on session|creates claude session|has status "starting"`)
	forbiddenBetweenRe := regexp.MustCompile(`(?i)fires|hook|"exit"|crash|kill`)
	for path, scenarios := range startingFeatureFrameWaitExemptions {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("exempted feature %s: %v", path, err)
		}
		for scenario, exemptSteps := range scenarios {
			background, steps := featureScenarioSteps(string(raw), scenario)
			if len(steps) == 0 {
				t.Fatalf("%s: exempted scenario %q no longer exists", path, scenario)
			}
			all := append(append([]string{}, background...), steps...)
			longRunning := false
			for _, s := range all {
				if strings.Contains(s, `a long-running fake "claude" binary is on PATH for future deck clients`) {
					longRunning = true
				}
				if shortLivedRe.MatchString(s) {
					t.Fatalf("%s: scenario %q installs a short-lived fake agent (%q) -- its starting render is transient; convert the exempted step", path, scenario, s)
				}
				if configRe.MatchString(s) {
					t.Fatalf("%s: scenario %q configures deck (%q) -- re-audit whether the exempted row can be probed out of starting", path, scenario, s)
				}
			}
			if !longRunning {
				t.Fatalf("%s: scenario %q no longer installs the long-running fake claude", path, scenario)
			}
			for step := range exemptSteps {
				idx := -1
				for i, s := range steps {
					if s == step {
						idx = i
						break
					}
				}
				if idx < 0 {
					t.Fatalf("%s: exempted step %q is no longer in scenario %q", path, step, scenario)
				}
				// The last step (scenario first, else Background) that put
				// the row into starting before the exempted one.
				between := []string(nil)
				found := false
				for i := idx - 1; i >= 0; i-- {
					if startsRowRe.MatchString(steps[i]) {
						between, found = steps[i+1:idx], true
						break
					}
				}
				if !found {
					for i := len(background) - 1; i >= 0; i-- {
						if startsRowRe.MatchString(background[i]) {
							between, found = append(append([]string{}, background[i+1:]...), steps[:idx]...), true
							break
						}
					}
				}
				if !found {
					t.Fatalf("%s: no create/resume/seeded-starting step precedes exempted step %q in scenario %q", path, step, scenario)
				}
				for _, s := range between {
					if forbiddenBetweenRe.MatchString(s) {
						t.Fatalf("%s: scenario %q runs %q between starting the row and exempted step %q -- the row may have left starting; re-audit", path, scenario, s, step)
					}
				}
			}
		}
	}
}
