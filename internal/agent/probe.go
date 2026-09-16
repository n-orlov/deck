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
	// tailPrefix, when set, additionally requires that the pane's last
	// non-blank, non-separator content line (see lastContentLine) start with
	// this prefix. It exists to narrow an otherwise-ambiguous marker that a
	// tool's own echoed output could also produce (SPEC requirement 38: pi's
	// bare "Error:" rule would otherwise flip a healthy session to error the
	// moment any command it ran printed that word to stdout).
	tailPrefix string
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
	// source. Refit against recorded captures of a real pi 0.84.1 binary — see
	// testdata/probes/pi-PROVENANCE.md for the capture method and for why
	// pi's "waiting" and "starting" states still have no rule here: a real pi
	// could not be driven into a capturable, durable-marker form of either
	// (permission prompts are extension-provided and never appeared; the only
	// pre-idle text observed is a container-specific helper-binary bootstrap
	// message, not pi's own semantics).
	//
	// "Error:" alone is real pi UI text (pi's own top-level agent-error
	// banner — traced to pi's bundled assistant-message component), but a
	// tool a healthy session ran can print that same substring to stdout.
	// tailPrefix disambiguates: pi's own error banner is always the pane's
	// last content line (nothing follows — the turn stops there), while a
	// tool's echoed "Error: ..." is always followed by more transcript
	// (a "Took Ns" line, a closing fence, or further assistant prose).
	{kind: "pi", contains: []string{"Error:"}, tailPrefix: "Error:", status: "error", reason: "agent error"},
	{kind: "pi", contains: []string{"Working..."}, status: "running", reason: "working indicator"},

	// pi draws a persistent two-line status footer at the very bottom of
	// EVERY pane regardless of status (see lastContentLine's doc comment) —
	// a cwd line followed by a usage-stats line containing the literal
	// "(auto)" and the bullet "•" (U+2022; distinct from the startup
	// banner's middle dot "·", U+00B7, so the two never collide). That
	// footer is durable across turns (confirmed by a real capture taken
	// after four conversational turns, long enough for the one-time startup
	// banner to scroll out of the pane entirely — see idle.txt) — unlike the
	// banner, which is why the banner was rejected as a marker and this is
	// not. cwd, model name, thinking level and the percentage vary; "(auto)"
	// and "•" do not, so only those are pinned.
	//
	// This rule MUST stay last among pi's rules: it infers idle from
	// *positive* evidence (the footer is present, meaning pi is alive and
	// rendering) plus the *absence* of the other two verdicts, never from
	// pane liveness alone (§7 forbids inferring running/idle from liveness).
	// A real capture mid-way through a 25s tool call (testdata/probes/pi/
	// sleep-midrun.txt) confirms "Working..." is still on screen throughout
	// a long tool call, so ordering alone (Error: and Working... rules run
	// first) is sufficient to keep this rule from firing while pi is busy —
	// see TestPiIdleRuleStaysLastAmongPiRules, which fails if this rule is
	// ever moved above them.
	{kind: "pi", contains: []string{"(auto)", "•"}, status: "idle", reason: "status footer, no working/error indicator"},

	// Codex is hook-instrumented (SPEC §8.1), so these rules are only the
	// fallback for the pre-first-prompt window before SessionStart fires
	// (codex's SessionStart hook does not fire at launch, only on first
	// prompt submission) plus a stale-hook fallback elsewhere. Fitted to a
	// real codex-cli 0.154.0 capture, not invented — see
	// testdata/probes/codex-PROVENANCE.md for the capture method. No rule
	// here keys on the fixtures' leading "WARNING: proceeding, ..." line
	// (a capture artefact deck's own sessions will not print) or on the
	// banner box (the banner scrolls away on a long session and is shared
	// by both starting and idle, see below).
	//
	// "Press enter to confirm or esc to cancel" is common to both approval
	// shapes captured (a shell-command prompt in waiting.txt, a file-edit
	// prompt via apply_patch in waiting-patch.txt) and appears nowhere
	// else in the corpus, so one rule covers both without a second
	// "waiting" entry.
	{kind: "codex", contains: []string{"Press enter to confirm or esc to cancel"}, status: "waiting", reason: "approval prompt"},
	// A leading "■" marks codex's terminal, retries-exhausted error line
	// (error.txt) and appears nowhere else in the corpus.
	{kind: "codex", contains: []string{"■"}, status: "error", reason: "terminal error"},
	// TRAP (see codex-PROVENANCE.md "Two traps"): "esc to interrupt" also
	// appears on a mid-retry network-error cell (retrying.txt:
	// "Reconnecting... 4/5 (2s • esc to interrupt)"), which is textually
	// indistinguishable from an ordinary running turn. That collision is
	// deliberate and asserted, not a bug: retrying.txt classifies as
	// running here, because only the exhausted state (a leading "■", with
	// "esc to interrupt" gone — handled by the error rule above, which
	// runs first) is safely classifiable as an error from the pane alone.
	{kind: "codex", contains: []string{"esc to interrupt"}, status: "running", reason: "working indicator"},
	// TRAP (see codex-PROVENANCE.md "Two traps"): starting and idle both
	// still show the banner box, the rotating "Tip:" line and the
	// "› Ask Codex to do anything" composer placeholder — the banner never
	// distinguishes them, and it can scroll away entirely on a long
	// session. The discriminator is transcript presence, not the banner:
	// idle has run at least one turn and therefore has an agent cell
	// ("•"); starting has launched but never prompted and has none. This
	// rule must stay ordered ahead of the starting rule below, since every
	// codex fixture with a transcript also still shows the composer
	// placeholder.
	{kind: "codex", contains: []string{"Ask Codex to do anything", "•"}, status: "idle", reason: "turn complete"},
	{kind: "codex", contains: []string{"Ask Codex to do anything"}, status: "starting", reason: "startup"},
}

func probe(kind, pane string) (status, reason string) {
	tail := lastContentLine(pane)
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
		if matched && rule.tailPrefix != "" && !strings.HasPrefix(tail, rule.tailPrefix) {
			matched = false
		}
		if matched {
			return rule.status, rule.reason
		}
	}
	return "", ""
}

// lastContentLine returns the pane's last non-blank transcript line,
// ignoring the composer/footer chrome pi and claude both draw at the very
// bottom of every pane regardless of status: two box-drawing separator
// lines framing the (usually blank) editable composer line, followed by a
// footer of cwd and usage stats that says nothing about the current turn.
// It answers "what did the agent actually leave on screen last".
func lastContentLine(pane string) string {
	lines := strings.Split(strings.ReplaceAll(pane, "\r\n", "\n"), "\n")

	// Find the last two (possibly non-adjacent) separator lines and drop
	// everything from the earlier of the two onward as footer chrome.
	end := len(lines)
	separators := 0
	for i := len(lines) - 1; i >= 0; i-- {
		if isSeparatorLine(strings.TrimSpace(lines[i])) {
			separators++
			if separators == 2 {
				end = i
				break
			}
		}
	}

	for i := end - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || isSeparatorLine(line) {
			continue
		}
		return line
	}
	return ""
}

// isSeparatorLine reports whether line is non-empty and consists entirely of
// box-drawing horizontal-rule characters (the composer separators pi and
// claude both draw), so it is never mistaken for a blank content line.
func isSeparatorLine(line string) bool {
	if line == "" {
		return false
	}
	for _, r := range line {
		if r != '─' && r != '-' {
			return false
		}
	}
	return true
}
