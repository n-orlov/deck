// badgesteps_test.go is task 005's own probe (R290): a YAML structural check
// over ci.yml that fails the moment a status-badge step can run for anything
// but a push or a workflow_dispatch on refs/heads/main, or the moment a step
// a pull_request run can reach writes anything under badges/.
//
// The step conditions are evaluated, not string-matched: evalIf below is a
// small GitHub-expression evaluator (==, !=, &&, ||, !, parentheses, string
// literals, github.event_name / github.ref, success() / always()) over a
// concrete (event, ref) pair. A context it does not model (steps.*, needs.*,
// matrix.*) is "unknown", and unknown never counts as "will not run".
package workflowcheck

import (
	"fmt"
	"strings"
	"testing"
)

// tri is a three-valued truth: a condition over a context this probe does
// not model is unknown, and an unknown step condition is treated as
// reachable.
type tri int

const (
	triFalse tri = iota
	triTrue
	triUnknown
)

// exprVal is one evaluated operand: a string (a literal or a modelled
// github.* value), a boolean (a comparison or a status function), or unknown.
type exprVal struct {
	kind string // "str", "bool", "unknown"
	str  string
	b    bool
}

type exprParser struct {
	toks []string
	pos  int
	ctx  map[string]string
}

func tokenizeIf(src string) ([]string, error) {
	var toks []string
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '\'':
			j := i + 1
			for j < len(src) && src[j] != '\'' {
				j++
			}
			if j >= len(src) {
				return nil, fmt.Errorf("unterminated string in %q", src)
			}
			toks = append(toks, src[i:j+1])
			i = j + 1
		case strings.HasPrefix(src[i:], "&&"), strings.HasPrefix(src[i:], "||"),
			strings.HasPrefix(src[i:], "=="), strings.HasPrefix(src[i:], "!="):
			toks = append(toks, src[i:i+2])
			i += 2
		case c == '(' || c == ')' || c == '!':
			toks = append(toks, string(c))
			i++
		default:
			j := i
			for j < len(src) && !strings.ContainsRune(" \t\n\r()!&|='", rune(src[j])) {
				j++
			}
			if j == i {
				return nil, fmt.Errorf("unexpected %q in %q", src[i], src)
			}
			toks = append(toks, src[i:j])
			i = j
		}
	}
	return toks, nil
}

func (p *exprParser) peek() string {
	if p.pos < len(p.toks) {
		return p.toks[p.pos]
	}
	return ""
}

func truth(v exprVal) tri {
	switch v.kind {
	case "bool":
		if v.b {
			return triTrue
		}
		return triFalse
	case "str":
		if v.str == "" {
			return triFalse
		}
		return triTrue
	}
	return triUnknown
}

func fromTri(t tri) exprVal {
	switch t {
	case triTrue:
		return exprVal{kind: "bool", b: true}
	case triFalse:
		return exprVal{kind: "bool", b: false}
	}
	return exprVal{kind: "unknown"}
}

func (p *exprParser) parseOr() (exprVal, error) {
	l, err := p.parseAnd()
	if err != nil {
		return l, err
	}
	for p.peek() == "||" {
		p.pos++
		r, err := p.parseAnd()
		if err != nil {
			return r, err
		}
		lt, rt := truth(l), truth(r)
		switch {
		case lt == triTrue || rt == triTrue:
			l = fromTri(triTrue)
		case lt == triUnknown || rt == triUnknown:
			l = fromTri(triUnknown)
		default:
			l = fromTri(triFalse)
		}
	}
	return l, nil
}

func (p *exprParser) parseAnd() (exprVal, error) {
	l, err := p.parseEq()
	if err != nil {
		return l, err
	}
	for p.peek() == "&&" {
		p.pos++
		r, err := p.parseEq()
		if err != nil {
			return r, err
		}
		lt, rt := truth(l), truth(r)
		switch {
		case lt == triFalse || rt == triFalse:
			l = fromTri(triFalse)
		case lt == triUnknown || rt == triUnknown:
			l = fromTri(triUnknown)
		default:
			l = fromTri(triTrue)
		}
	}
	return l, nil
}

func (p *exprParser) parseEq() (exprVal, error) {
	l, err := p.parseUnary()
	if err != nil {
		return l, err
	}
	for p.peek() == "==" || p.peek() == "!=" {
		op := p.toks[p.pos]
		p.pos++
		r, err := p.parseUnary()
		if err != nil {
			return r, err
		}
		if l.kind != "str" || r.kind != "str" {
			l = fromTri(triUnknown)
			continue
		}
		eq := strings.EqualFold(l.str, r.str)
		l = exprVal{kind: "bool", b: eq == (op == "==")}
	}
	return l, nil
}

func (p *exprParser) parseUnary() (exprVal, error) {
	if p.peek() == "!" {
		p.pos++
		v, err := p.parseUnary()
		if err != nil {
			return v, err
		}
		switch truth(v) {
		case triTrue:
			return fromTri(triFalse), nil
		case triFalse:
			return fromTri(triTrue), nil
		}
		return fromTri(triUnknown), nil
	}
	return p.parsePrimary()
}

func (p *exprParser) parsePrimary() (exprVal, error) {
	t := p.peek()
	if t == "" {
		return exprVal{}, fmt.Errorf("unexpected end of expression")
	}
	p.pos++
	switch {
	case t == "(":
		v, err := p.parseOr()
		if err != nil {
			return v, err
		}
		if p.peek() != ")" {
			return v, fmt.Errorf("missing )")
		}
		p.pos++
		return v, nil
	case strings.HasPrefix(t, "'"):
		return exprVal{kind: "str", str: t[1 : len(t)-1]}, nil
	case t == "success()" || t == "always()":
		return fromTri(triTrue), nil
	case t == "cancelled()":
		// The probe models a run that was not cancelled.
		return fromTri(triFalse), nil
	case t == "true" || t == "false":
		return exprVal{kind: "bool", b: t == "true"}, nil
	}
	if v, ok := p.ctx[t]; ok {
		return exprVal{kind: "str", str: v}, nil
	}
	// A function call this evaluator does not model (contains(...), ...)
	// or a context (steps.*, needs.*) it has no value for.
	if strings.Contains(t, "(") {
		depth := strings.Count(t, "(") - strings.Count(t, ")")
		for depth > 0 && p.pos < len(p.toks) {
			depth += strings.Count(p.toks[p.pos], "(") - strings.Count(p.toks[p.pos], ")")
			p.pos++
		}
	}
	return exprVal{kind: "unknown"}, nil
}

// evalIf evaluates a step or job `if:` for one concrete trigger. An empty
// condition is true (an unconditioned step runs whenever its job does).
func evalIf(cond string, event, ref string) (tri, error) {
	cond = strings.TrimSpace(cond)
	cond = strings.TrimSuffix(strings.TrimPrefix(cond, "${{"), "}}")
	if strings.TrimSpace(cond) == "" {
		return triTrue, nil
	}
	toks, err := tokenizeIf(cond)
	if err != nil {
		return triUnknown, err
	}
	// Re-join "name" "(" ")" pieces: the tokenizer splits "success()" at "(".
	var joined []string
	for i := 0; i < len(toks); i++ {
		if i+2 < len(toks) && toks[i+1] == "(" && toks[i+2] == ")" && !strings.ContainsAny(toks[i], "()!&|='") {
			joined = append(joined, toks[i]+"()")
			i += 2
			continue
		}
		joined = append(joined, toks[i])
	}
	p := &exprParser{toks: joined, ctx: map[string]string{
		"github.event_name": event,
		"github.ref":        ref,
	}}
	v, err := p.parseOr()
	if err != nil {
		return triUnknown, fmt.Errorf("%q: %w", cond, err)
	}
	if p.pos != len(p.toks) {
		return triUnknown, fmt.Errorf("%q: trailing %q", cond, p.toks[p.pos:])
	}
	return truth(v), nil
}

// stepWritesBadges reports whether a step names the badge generator or the
// badges/ tree anywhere a write could be spelled: its run script, its env,
// or its with: inputs.
func stepWritesBadges(s ciStep) bool {
	has := func(text string) bool {
		return strings.Contains(text, "badgegen") || strings.Contains(text, "badges")
	}
	if has(s.Run) || has(s.Name) {
		return true
	}
	for k, v := range s.Env {
		if has(k) || has(v) {
			return true
		}
	}
	for k, v := range s.With {
		if has(k) || has(fmt.Sprint(v)) {
			return true
		}
	}
	return false
}

type badgeStepRef struct {
	job  string
	idx  int
	step ciStep
}

func badgeSteps(wf ciWorkflow) []badgeStepRef {
	var out []badgeStepRef
	for name, j := range wf.Jobs {
		for i, s := range j.Steps {
			if stepWritesBadges(s) {
				out = append(out, badgeStepRef{name, i, s})
			}
		}
	}
	return out
}

// checkBadgeStepsMainOnly is the R290 condition: there are badge steps that
// generate and publish, and each runs for exactly a push or a
// workflow_dispatch on refs/heads/main, and for no other event or ref.
func checkBadgeStepsMainOnly(wf ciWorkflow) error {
	steps := badgeSteps(wf)
	var generates, publishes bool
	for _, b := range steps {
		generates = generates || strings.Contains(b.step.Run, "ci/badgegen")
		publishes = publishes || strings.Contains(b.step.Run, "pages-persist.sh") && strings.Contains(b.step.Run, "badges")
	}
	if !generates {
		return fmt.Errorf("no step runs ci/badgegen")
	}
	if !publishes {
		return fmt.Errorf("no step publishes badges through ci/pages-persist.sh")
	}
	cases := []struct {
		event, ref string
		want       tri
	}{
		{"push", "refs/heads/main", triTrue},
		{"workflow_dispatch", "refs/heads/main", triTrue},
		{"push", "refs/heads/feature", triFalse},
		{"workflow_dispatch", "refs/heads/feature", triFalse},
		{"pull_request", "refs/pull/7/merge", triFalse},
		{"pull_request", "refs/heads/main", triFalse},
		{"schedule", "refs/heads/main", triFalse},
	}
	for _, b := range steps {
		if strings.TrimSpace(b.step.If) == "" {
			return fmt.Errorf("job %q step %d (%q) writes badges with no if: condition", b.job, b.idx, b.step.Name)
		}
		for _, c := range cases {
			got, err := evalIf(b.step.If, c.event, c.ref)
			if err != nil {
				return fmt.Errorf("job %q step %q: %w", b.job, b.step.Name, err)
			}
			if got != c.want {
				return fmt.Errorf("job %q step %q: if: %q evaluates to %v for %s on %s, want %v",
					b.job, b.step.Name, b.step.If, got, c.event, c.ref, c.want)
			}
		}
	}
	return nil
}

// checkNoPullRequestBadgeWrites asserts that no step a pull_request run can
// reach (a step condition that is not provably false for it) writes badges.
func checkNoPullRequestBadgeWrites(wf ciWorkflow) error {
	for name, j := range wf.Jobs {
		for i, s := range j.Steps {
			got, err := evalIf(s.If, "pull_request", "refs/pull/7/merge")
			if err != nil {
				return fmt.Errorf("job %q step %d: %w", name, i, err)
			}
			if got != triFalse && stepWritesBadges(s) {
				return fmt.Errorf("job %q step %d (%q) writes badges and a pull_request run can reach it (if: %q)", name, i, s.Name, s.If)
			}
		}
	}
	return nil
}

// copyWorkflow returns a deep-enough copy to mutate step conditions.
func copyWorkflow(wf ciWorkflow) ciWorkflow {
	out := ciWorkflow{Jobs: map[string]ciJob{}}
	for n, j := range wf.Jobs {
		j.Steps = append([]ciStep(nil), j.Steps...)
		out.Jobs[n] = j
	}
	return out
}

// TestBadgeStepsMainOnly: R290's badge steps run for a push or a
// workflow_dispatch on refs/heads/main and nothing else. The mutation half
// deletes the condition from each badge step in turn and requires the check
// to turn red every time.
func TestBadgeStepsMainOnly(t *testing.T) {
	wf := loadCIWorkflow(t)
	if err := checkBadgeStepsMainOnly(wf); err != nil {
		t.Fatalf("ci.yml: %v", err)
	}

	steps := badgeSteps(wf)
	if len(steps) < 2 {
		t.Fatalf("expected a generate and a publish badge step, found %d", len(steps))
	}
	for _, b := range steps {
		mut := copyWorkflow(wf)
		j := mut.Jobs[b.job]
		j.Steps[b.idx].If = ""
		mut.Jobs[b.job] = j
		if err := checkBadgeStepsMainOnly(mut); err == nil {
			t.Errorf("deleting the if: from badge step %q (job %q) did not turn the check red", b.step.Name, b.job)
		}

		// A condition that admits pull_request must also be refused.
		mut = copyWorkflow(wf)
		j = mut.Jobs[b.job]
		j.Steps[b.idx].If = "github.ref == 'refs/heads/main'"
		mut.Jobs[b.job] = j
		if err := checkBadgeStepsMainOnly(mut); err == nil {
			t.Errorf("a ref-only if: on badge step %q (job %q) was accepted, but it admits schedule", b.step.Name, b.job)
		}
	}
}

// TestNoPullRequestJobWritesBadges: the steps of ci.yml a pull_request run
// can reach write nothing under badges/. The mutation half removes the
// condition from a badge step and requires the check to see the write.
func TestNoPullRequestJobWritesBadges(t *testing.T) {
	wf := loadCIWorkflow(t)
	if err := checkNoPullRequestBadgeWrites(wf); err != nil {
		t.Fatalf("ci.yml: %v", err)
	}

	steps := badgeSteps(wf)
	if len(steps) == 0 {
		t.Fatalf("no badge step found to mutate")
	}
	for _, b := range steps {
		mut := copyWorkflow(wf)
		j := mut.Jobs[b.job]
		j.Steps[b.idx].If = ""
		mut.Jobs[b.job] = j
		if err := checkNoPullRequestBadgeWrites(mut); err == nil {
			t.Errorf("an unconditioned badge step %q (job %q) was not seen as reachable by a pull_request run", b.step.Name, b.job)
		}
	}
}

// TestBadgeConditionEvaluator pins the evaluator the two checks above rely
// on, so a broken evaluator cannot make them vacuous.
func TestBadgeConditionEvaluator(t *testing.T) {
	cond := "success() && (github.event_name == 'push' || github.event_name == 'workflow_dispatch') && github.ref == 'refs/heads/main'"
	rows := []struct {
		cond, event, ref string
		want             tri
	}{
		{cond, "push", "refs/heads/main", triTrue},
		{cond, "pull_request", "refs/heads/main", triFalse},
		{cond, "push", "refs/heads/x", triFalse},
		{"", "pull_request", "refs/pull/1/merge", triTrue},
		{"${{ !cancelled() && (github.event_name == 'push' || github.event_name == 'workflow_dispatch') && github.ref == 'refs/heads/main' }}", "push", "refs/heads/main", triTrue},
		{"${{ !cancelled() && (github.event_name == 'push' || github.event_name == 'workflow_dispatch') && github.ref == 'refs/heads/main' }}", "pull_request", "refs/pull/1/merge", triFalse},
		{"cancelled()", "push", "refs/heads/main", triFalse},
		{"steps.a.outcome == 'failure'", "push", "refs/heads/main", triUnknown},
		{"always() && needs.suite.result != 'skipped'", "push", "refs/heads/main", triUnknown},
		{"github.event_name != 'pull_request'", "pull_request", "refs/pull/1/merge", triFalse},
	}
	for _, r := range rows {
		got, err := evalIf(r.cond, r.event, r.ref)
		if err != nil {
			t.Errorf("%q: %v", r.cond, err)
			continue
		}
		if got != r.want {
			t.Errorf("evalIf(%q, %s, %s) = %v, want %v", r.cond, r.event, r.ref, got, r.want)
		}
	}
}
