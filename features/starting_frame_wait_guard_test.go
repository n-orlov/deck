package features

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
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
// Not every "starting" literal in a WaitForFrame* call is this hazard,
// because the hazard is specific to a SHELL row's own instantaneous
// promotion (SPEC §9.2's state table: that same-tick pane-alive
// fast-forward is "shell rows only" -- an agent row only ever leaves
// "starting" on its first agent signal, a real, durably-observable delay,
// never a same-render-cycle skip). starting_frame_wait_guard_exemptions
// below is the audit's one-line-reason allowlist for every site the audit
// found and judged not to be the shell hazard; a hit at a site NOT on that
// list is new and must be converted onto the durable M1 observable
// (waitForSettledSessionRow, the store's own recorded transition, or an
// audited event) exactly like task 026's conversions, not added to the
// list to silence the guard.
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
		t.Fatalf("task 026: %d frame wait(s) on the lone transient \"starting\" status word, not in starting_frame_wait_guard_exemptions -- convert onto the durable M1 observable (waitForSettledSessionRow, the store's own transition, or an audited event), never widen this guard's exemption list to silence it:\n%s",
			len(violations), strings.Join(violations, "\n"))
	}
}

// starting_frame_wait_guard_exemptions is task 026's audit allowlist, keyed
// by (file, enclosing function). Every entry's own doc-comment reason is
// re-checked below in TestStartingFrameWaitGuardExemptionsAreGroundedInAgentSessions.
var startingFrameWaitGuardExemptions = map[string]map[string]string{
	"agent_steps_test.go": {
		// All five finish an agent (claude/codex/pi) create: the step
		// pattern they all register is `^deck client "([^"]+)" creates
		// ([a-z]+) session "([^"]+)" with permission profile "([^"]+)"...`
		// variants, and "shell" never carries a "permission profile" (no
		// *.feature file pairs "creates shell session" with "permission
		// profile"; create_session_test.go's shell steps use a disjoint
		// step pattern with no profile group at all) -- so kind here is
		// always a real agent adapter. SPEC §9.2's state table promotes an
		// agent row starting->running only on its first agent signal, never
		// a same-tick pane-alive fast-forward (that rule is shell-only,
		// SPEC §9.2 row "pane is alive, shell rows only"), so "starting" is
		// a durable rendered state for the whole of fake-claude/fake-codex's
		// own deliberate startup delay here, not a transient frame racing
		// a promotion.
		"clientCreatesAgentSessionWithProfileAndOptionalMessage": "agent-only create (profile-gated step, never shell); SPEC §9.2 promotes an agent row out of starting only on its first agent signal, not a same-tick fast-forward.",
		"clientCreatesAgentSessionWithProfileAndEnv":             "agent-only create (profile-gated step, never shell); SPEC §9.2 promotes an agent row out of starting only on its first agent signal, not a same-tick fast-forward.",
		"clientCreatesAgentSessionWithProfileAndLoginShell":      "agent-only create (profile-gated step, never shell); SPEC §9.2 promotes an agent row out of starting only on its first agent signal, not a same-tick fast-forward.",
		"clientCreatesAgentSessionWithFailingPreLaunch":          "agent-only create (profile-gated step, never shell); SPEC §9.2 promotes an agent row out of starting only on its first agent signal, not a same-tick fast-forward.",
		"clientCreatesAgentSessionWithSucceedingPreLaunch":       "agent-only create (profile-gated step, never shell); SPEC §9.2 promotes an agent row out of starting only on its first agent signal, not a same-tick fast-forward.",
	},
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
// line by line, tracking the enclosing top-level function so a hit can be
// checked against startingFrameWaitGuardExemptions. A line whose trimmed
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
			if reason, ok := startingFrameWaitGuardExemptions[path][currentFunc]; ok {
				_ = reason // exempt, audited
			} else {
				out = append(out, fmt.Sprintf("%s:%d: func %s's WaitForFrameFunc predicate checks the lone transient \"starting\" word with no \"running\" anywhere in the same predicate",
					path, predicateStartLine, currentFunc))
			}
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
			if reason, ok := startingFrameWaitGuardExemptions[path][currentFunc]; ok {
				_ = reason // exempt, audited
				continue
			}
			out = append(out, fmt.Sprintf("%s:%d: func %s waits for a frame on the lone transient \"starting\" word",
				path, ln, currentFunc))
		}
	}
	flushPredicate()

	return out, nil
}

// TestStartingFrameWaitGuardExemptionsAreGroundedInAgentSessions re-checks,
// against the real registered step patterns, the one factual claim every
// startingFrameWaitGuardExemptions entry for agent_steps_test.go rests on:
// that EVERY step reaching the clientCreatesAgentSessionWithProfile* family
// (the exempted functions, plus their own callers/wrappers in the same
// file) requires a "permission profile" group in its step text -- SPEC's
// agent-only field, never present on a plain "creates shell session" step
// -- so kind is always a real agent adapter at every exempted site. It
// scans every sc.Step registration whose handler name has that prefix,
// rather than looking up each exempted function individually, so it also
// catches a wrapper that lets a NEW step reach the family without that
// requirement. If a future edit wires any of them to a shell-reachable
// step, this test -- not just the guard above -- must fail.
func TestStartingFrameWaitGuardExemptionsAreGroundedInAgentSessions(t *testing.T) {
	raw, err := os.ReadFile("agent_steps_test.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	stepRe := regexp.MustCompile("sc\\.Step\\(`([^`]*)`,\\s*(clientCreatesAgentSessionWith[A-Za-z0-9_]*)\\)")
	matches := stepRe.FindAllStringSubmatch(src, -1)
	if len(matches) == 0 {
		t.Fatal("no sc.Step registration found for any clientCreatesAgentSessionWithProfile* handler -- regex is stale against agent_steps_test.go's current text")
	}
	grounded := map[string]bool{}
	for _, m := range matches {
		stepText, fn := m[1], m[2]
		grounded[fn] = true
		if !strings.Contains(stepText, "permission profile") {
			t.Fatalf("step registered for handler %s does not require \"permission profile\" (step text %q) -- it may now be reachable from a plain shell create; re-audit startingFrameWaitGuardExemptions before trusting it", fn, stepText)
		}
	}

	// Every exempted function not itself a direct step handler must only
	// ever be CALLED, within this file, from a function already grounded
	// above (directly profile-gated) -- otherwise some other, possibly
	// shell-reachable, caller could also reach it. callersOf walks the
	// file's own top-level function bodies (via goFuncHeaderRe, the same
	// tracker the guard scanner uses) collecting, for every call site
	// `fn(`, which function's body it appeared in.
	callersOf := func(fn string) []string {
		callRe := regexp.MustCompile(`\b` + regexp.QuoteMeta(fn) + `\(`)
		var callers []string
		currentFunc := ""
		for _, l := range strings.Split(src, "\n") {
			if m := goFuncHeaderRe.FindStringSubmatch(l); m != nil {
				currentFunc = m[1]
				continue // the func header itself is never a call site
			}
			if currentFunc != fn && callRe.MatchString(l) {
				callers = append(callers, currentFunc)
			}
		}
		return callers
	}
	var isGrounded func(fn string, seen map[string]bool) bool
	isGrounded = func(fn string, visiting map[string]bool) bool {
		if grounded[fn] {
			return true
		}
		if visiting[fn] {
			return false // cycle: never grounds anything on its own
		}
		visiting[fn] = true
		callers := callersOf(fn)
		if len(callers) == 0 {
			return false
		}
		for _, c := range callers {
			if !isGrounded(c, visiting) {
				return false
			}
		}
		return true
	}
	for fn := range startingFrameWaitGuardExemptions["agent_steps_test.go"] {
		if !isGrounded(fn, map[string]bool{}) {
			t.Fatalf("exempted func %s is not grounded in a permission-profile-gated step (directly, or through callers that are, within agent_steps_test.go) -- re-verify by hand which step(s) reach it and whether any is shell-reachable", fn)
		}
	}
}
