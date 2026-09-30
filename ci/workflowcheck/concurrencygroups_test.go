// concurrencygroups_test.go is task 008's own probe (R165, GH #50): it
// parses ci.yml's top-level `concurrency:` block as real YAML, then
// EVALUATES its `group`/`cancel-in-progress` GitHub Actions expressions
// against a small table of event scenarios with a tiny expression
// evaluator (ghExprEval below) -- never by grepping the raw expression
// text -- so it fails the moment the expression itself stops producing
// the four required properties, however the expression is spelled:
//
//  1. the `schedule` and `workflow_dispatch` events' own groups differ
//     from every `push` event's own group;
//  2. two `push` events with different `github.sha` values get
//     different groups (a per-commit push group, so pushing again to
//     main before an earlier run finished never queues behind, or
//     cancels, that earlier run's own result);
//  3. `cancel-in-progress` evaluates true only for `pull_request`, false
//     for every other event in the table;
//  4. the `publish` job carries no job-level `concurrency:` of its own
//     (job-level concurrency would let the retrying deploy-pages steps
//     inside it fight the workflow-level group instead of the
//     "deployment already in progress" back-off added alongside this
//     probe -- see ci.yml's own `publish` job comment and GitHub's Pages
//     API docs, https://docs.github.com/en/rest/pages/pages, "Build
//     requests are limited to one concurrent build per repository...").
//
// Demonstrated failing against the pre-fix dc2b6f7ece tree, whose
// concurrency group is `ci-${{ github.workflow }}-${{ github.ref }}` --
// identical for every push (same ref) and for schedule/workflow_dispatch
// too (same ref again) -- see
// /run/ralphd/artifacts/r165/concurrencygroups-dc2b6f7ece-fail.log.
package workflowcheck

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// --- a small GitHub Actions expression evaluator -----------------------
//
// Just enough of the expression language
// (https://docs.github.com/en/actions/learn-github-actions/expressions)
// to evaluate the handful of forms this repo's `concurrency:` block
// actually uses: dotted `github.*` context lookups, single-quoted string
// literals, `==`/`!=`, `&&`/`||` with GitHub's own short-circuit-return
// semantics (not a bare boolean AND/OR), and parentheses. It is not a
// general implementation of the expression grammar -- only what this
// probe needs to evaluate the real ci.yml expressions without resorting
// to string matching.

type ghValue struct {
	str    string
	isBool bool
	bval   bool
}

func strVal(s string) ghValue { return ghValue{str: s} }
func boolVal(b bool) ghValue  { return ghValue{isBool: true, bval: b} }

// truthy follows GitHub's own coercion for && / ||: a bool uses its own
// value; a string is truthy unless empty (GitHub actually coerces empty
// string to falsy, any other string to truthy -- see "Objects and
// literals to boolean" in the expressions docs above).
func (v ghValue) truthy() bool {
	if v.isBool {
		return v.bval
	}
	return v.str != ""
}

func (v ghValue) string() string {
	if v.isBool {
		return strconv.FormatBool(v.bval)
	}
	return v.str
}

func (v ghValue) equals(other ghValue) bool {
	if v.isBool || other.isBool {
		return v.truthy() == other.truthy()
	}
	return v.str == other.str
}

// ghExprEval evaluates one `${{ ... }}` expression body (the text between
// the delimiters, exclusive) against a context map keyed by the full
// dotted name (e.g. "github.event_name").
type ghExprEval struct {
	toks []string
	pos  int
	ctx  map[string]string
}

func tokenizeGhExpr(s string) []string {
	var toks []string
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n':
			i++
		case c == '\'':
			j := i + 1
			for j < len(s) && s[j] != '\'' {
				j++
			}
			toks = append(toks, s[i:j+1])
			i = j + 1
		case strings.HasPrefix(s[i:], "=="):
			toks = append(toks, "==")
			i += 2
		case strings.HasPrefix(s[i:], "!="):
			toks = append(toks, "!=")
			i += 2
		case strings.HasPrefix(s[i:], "&&"):
			toks = append(toks, "&&")
			i += 2
		case strings.HasPrefix(s[i:], "||"):
			toks = append(toks, "||")
			i += 2
		case c == '(' || c == ')':
			toks = append(toks, string(c))
			i++
		default:
			j := i
			for j < len(s) && !strings.ContainsRune(" \t\n()'", rune(s[j])) &&
				!strings.HasPrefix(s[j:], "==") && !strings.HasPrefix(s[j:], "!=") &&
				!strings.HasPrefix(s[j:], "&&") && !strings.HasPrefix(s[j:], "||") {
				j++
			}
			toks = append(toks, s[i:j])
			i = j
		}
	}
	return toks
}

func evalGhExpr(expr string, ctx map[string]string) ghValue {
	e := &ghExprEval{toks: tokenizeGhExpr(expr), ctx: ctx}
	v := e.parseOr()
	return v
}

func (e *ghExprEval) peek() string {
	if e.pos >= len(e.toks) {
		return ""
	}
	return e.toks[e.pos]
}

func (e *ghExprEval) next() string {
	t := e.peek()
	e.pos++
	return t
}

// parseOr / parseAnd implement GitHub's own value-returning short circuit:
// `a || b` returns a if truthy else b; `a && b` returns b if a truthy
// else a.
func (e *ghExprEval) parseOr() ghValue {
	left := e.parseAnd()
	for e.peek() == "||" {
		e.next()
		right := e.parseAnd()
		if !left.truthy() {
			left = right
		}
	}
	return left
}

func (e *ghExprEval) parseAnd() ghValue {
	left := e.parseEquality()
	for e.peek() == "&&" {
		e.next()
		right := e.parseEquality()
		if left.truthy() {
			left = right
		}
	}
	return left
}

func (e *ghExprEval) parseEquality() ghValue {
	left := e.parsePrimary()
	for e.peek() == "==" || e.peek() == "!=" {
		op := e.next()
		right := e.parsePrimary()
		eq := left.equals(right)
		if op == "!=" {
			eq = !eq
		}
		left = boolVal(eq)
	}
	return left
}

func (e *ghExprEval) parsePrimary() ghValue {
	tok := e.next()
	switch {
	case tok == "(":
		v := e.parseOr()
		if e.peek() == ")" {
			e.next()
		}
		return v
	case strings.HasPrefix(tok, "'") && strings.HasSuffix(tok, "'") && len(tok) >= 2:
		return strVal(tok[1 : len(tok)-1])
	default:
		if val, ok := e.ctx[tok]; ok {
			return strVal(val)
		}
		return strVal("")
	}
}

// evalGhTemplate evaluates a full YAML scalar that may mix literal text
// with `${{ ... }}` interpolations (as `group:` does) and returns the
// concatenated string result.
func evalGhTemplate(tmpl string, ctx map[string]string) string {
	var out strings.Builder
	i := 0
	for i < len(tmpl) {
		start := strings.Index(tmpl[i:], "${{")
		if start < 0 {
			out.WriteString(tmpl[i:])
			break
		}
		out.WriteString(tmpl[i : i+start])
		rest := tmpl[i+start+3:]
		end := strings.Index(rest, "}}")
		if end < 0 {
			out.WriteString(tmpl[i+start:])
			break
		}
		out.WriteString(evalGhExpr(rest[:end], ctx).string())
		i = i + start + 3 + end + 2
	}
	return out.String()
}

// evalGhBoolTemplate evaluates a scalar expected to be a single `${{ ... }}`
// expression yielding a boolean (as `cancel-in-progress:` is).
func evalGhBoolTemplate(tmpl string, ctx map[string]string) bool {
	tmpl = strings.TrimSpace(tmpl)
	if strings.HasPrefix(tmpl, "${{") && strings.HasSuffix(tmpl, "}}") {
		return evalGhExpr(tmpl[3:len(tmpl)-2], ctx).truthy()
	}
	return evalGhExpr(tmpl, ctx).truthy()
}

// --- ci.yml structural model this probe needs ---------------------------

type concurrencyBlock struct {
	Group            string `yaml:"group"`
	CancelInProgress string `yaml:"cancel-in-progress"`
}

func loadCIWorkflowRaw(t *testing.T) (concurrencyBlock, map[string]map[string]yaml.Node) {
	t.Helper()
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	path := filepath.Join(root, ".github", "workflows", "ci.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var top struct {
		Concurrency concurrencyBlock `yaml:"concurrency"`
	}
	if err := yaml.Unmarshal(raw, &top); err != nil {
		t.Fatalf("%s: parse YAML (concurrency): %v", path, err)
	}

	var jobsWrap struct {
		Jobs map[string]map[string]yaml.Node `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(raw, &jobsWrap); err != nil {
		t.Fatalf("%s: parse YAML (jobs): %v", path, err)
	}
	return top.Concurrency, jobsWrap.Jobs
}

// scenario is one event this probe evaluates the concurrency expressions
// against; ref/sha model what GitHub itself sets `github.ref`/`github.sha`
// to for that event (schedule and workflow_dispatch both evaluate against
// the default branch, exactly like the two push scenarios' own ref, which
// is exactly what makes "differs from any push group" a real test instead
// of a vacuous one).
type scenario struct {
	name      string
	eventName string
	ref       string
	sha       string
}

func (s scenario) ctx() map[string]string {
	return map[string]string{
		"github.workflow":   "ci",
		"github.event_name": s.eventName,
		"github.ref":        s.ref,
		"github.sha":        s.sha,
	}
}

func TestCIWorkflowConcurrencyGroupsDifferByEventAndPushSha(t *testing.T) {
	conc, _ := loadCIWorkflowRaw(t)
	if conc.Group == "" {
		t.Fatalf("ci.yml: concurrency.group is empty")
	}

	pushA := scenario{name: "push A", eventName: "push", ref: "refs/heads/main", sha: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	pushB := scenario{name: "push B", eventName: "push", ref: "refs/heads/main", sha: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	schedule := scenario{name: "schedule", eventName: "schedule", ref: "refs/heads/main", sha: "cccccccccccccccccccccccccccccccccccccccc"}
	dispatch := scenario{name: "workflow_dispatch", eventName: "workflow_dispatch", ref: "refs/heads/main", sha: "dddddddddddddddddddddddddddddddddddddddd"}
	pr := scenario{name: "pull_request", eventName: "pull_request", ref: "refs/pull/42/merge", sha: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"}

	groupOf := func(s scenario) string { return evalGhTemplate(conc.Group, s.ctx()) }

	gPushA := groupOf(pushA)
	gPushB := groupOf(pushB)
	gSchedule := groupOf(schedule)
	gDispatch := groupOf(dispatch)

	if gPushA == gPushB {
		t.Errorf("push A group %q == push B group %q: two pushes with different shas must get different groups", gPushA, gPushB)
	}
	if gSchedule == gPushA || gSchedule == gPushB {
		t.Errorf("schedule group %q collides with a push group (A=%q, B=%q): schedule must differ from any push group", gSchedule, gPushA, gPushB)
	}
	if gDispatch == gPushA || gDispatch == gPushB {
		t.Errorf("workflow_dispatch group %q collides with a push group (A=%q, B=%q): workflow_dispatch must differ from any push group", gDispatch, gPushA, gPushB)
	}

	if conc.CancelInProgress == "" {
		t.Fatalf("ci.yml: concurrency.cancel-in-progress is empty")
	}
	for _, s := range []scenario{pushA, pushB, schedule, dispatch, pr} {
		got := evalGhBoolTemplate(conc.CancelInProgress, s.ctx())
		want := s.eventName == "pull_request"
		if got != want {
			t.Errorf("cancel-in-progress for %s = %v, want %v (cancel-in-progress must be true only for pull_request)", s.name, got, want)
		}
	}
}

func TestCIWorkflowPublishJobHasNoJobLevelConcurrency(t *testing.T) {
	_, jobs := loadCIWorkflowRaw(t)
	for id, fields := range jobs {
		nameNode, hasName := fields["name"]
		if !hasName || nameNode.Value != "publish (Pages)" {
			continue
		}
		if _, has := fields["concurrency"]; has {
			t.Errorf("ci.yml: job %q (%q) carries its own job-level concurrency -- publish must rely only on the workflow-level group plus the deploy-pages retry back-off, never a job-level concurrency block of its own", id, nameNode.Value)
		}
		return
	}
	t.Fatalf("ci.yml: no job named %q found", "publish (Pages)")
}
