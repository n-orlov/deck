package agent

import "strings"

// probeRule is one pane-text heuristic. Keep the complete corpus here: probe
// classification is adapter knowledge and must not leak into service or TUI
// code. Rules are ordered from the most specific verdict to the broadest (an
// idle prompt glyph, for example, can also be present above a running spinner).
type probeRule struct {
	kind     string
	contains []string
	status   string
	reason   string
}

var probeRules = []probeRule{
	// Claude fallback rules. Hooks are Claude's primary status source, but a
	// stale hook is deliberately eligible for this sampled fallback (SPEC §7).
	{kind: "claude", contains: []string{"API Error:"}, status: "error", reason: "api error"},
	{kind: "claude", contains: []string{"Do you want to proceed?", "Yes"}, status: "waiting", reason: "permission prompt"},
	{kind: "claude", contains: []string{"esc to interrupt"}, status: "running", reason: "working indicator"},
	{kind: "claude", contains: []string{"Claude Code", "❯"}, status: "idle", reason: "idle prompt"},
	{kind: "claude", contains: []string{"Starting Claude Code"}, status: "starting", reason: "startup"},

	// Pi has no verified hook source, so these sampled verdicts are its status
	// source. Keep error/waiting/running ahead of the broad idle prompt.
	{kind: "pi", contains: []string{"Error:"}, status: "error", reason: "agent error"},
	{kind: "pi", contains: []string{"Allow tool execution?", "Yes"}, status: "waiting", reason: "permission prompt"},
	{kind: "pi", contains: []string{"Working", "ctrl-c to stop"}, status: "running", reason: "working indicator"},
	{kind: "pi", contains: []string{"pi coding agent", ">"}, status: "idle", reason: "idle prompt"},
	{kind: "pi", contains: []string{"Starting pi"}, status: "starting", reason: "startup"},
}

func probe(kind, pane string) (status, reason string) {
	for _, rule := range probeRules {
		if rule.kind != kind {
			continue
		}
		matched := true
		for _, marker := range rule.contains {
			if !strings.Contains(pane, marker) {
				matched = false
				break
			}
		}
		if matched {
			return rule.status, rule.reason
		}
	}
	return "", ""
}
