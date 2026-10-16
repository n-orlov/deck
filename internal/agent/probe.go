package agent

import (
	"regexp"
	"strings"
)

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
	// match, when set, is an additional line-aware test the pane must pass
	// (copilot's working footer and `✗ ` error line are positional, which a
	// bare substring cannot express).
	match func(pane string) bool
	// live, when set, tests the contains markers against the pane's live
	// region (claudeLiveRegion) instead of the whole capture (R311).
	live bool
}

var probeRules = []probeRule{
	// Claude fallback rules. Hooks are Claude's primary status source, but a
	// stale hook is deliberately eligible for this sampled fallback (SPEC §7).
	// R311: both markers count only in the live region (claudeLiveRegion): an
	// error or a dialog that scrolled up into the transcript is history.
	{kind: "claude", contains: []string{"API Error:"}, live: true, status: "error", reason: "api error"},
	{kind: "claude", contains: []string{"Do you want to proceed?", "Yes"}, live: true, status: "waiting", reason: "permission prompt"},
	// R312: the other dialog shapes whose markers repo evidence establishes
	// (claude_gate.go's dialog list; testdata/probes/claude-PROVENANCE.md). All
	// are live-region rules, like the permission dialog. AskUserQuestion, plan
	// approval and a write-file wording have no repo evidence and are petitioned,
	// so there is deliberately no rule for them.
	{kind: "claude", contains: []string{"Do you want to make this edit", "Yes"}, live: true, status: "waiting", reason: "file edit confirmation"},
	{kind: "claude", contains: []string{"Do you want to create", "Yes"}, live: true, status: "waiting", reason: "file create confirmation"},
	{kind: "claude", contains: []string{"Do you trust the files in this folder", "Yes"}, live: true, status: "waiting", reason: "folder trust"},
	{kind: "claude", contains: []string{"esc to interrupt"}, status: "running", reason: "working indicator"},
	// R310 (testdata/probes/claude-PROVENANCE.md says which footer texts were
	// verified): idle is the footer being present with no working indicator above it
	// having matched, never the startup banner (it scrolls away) and never the
	// composer's content (an unsent draft does not change status).
	{kind: "claude", contains: []string{"⏵⏵", "(shift+tab to cycle)"}, status: "idle", reason: "idle footer"},
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
	// (error.txt). R313: it counts only as the head of the last content block
	// above codex's composer (codexTerminalError), the way pi's "Error:" is
	// anchored to its last content line: "■" in a markdown body, a progress bar
	// or a tool's output is followed by more transcript, or is indented, and is
	// not an error.
	{kind: "codex", contains: []string{"■"}, match: codexTerminalError, status: "error", reason: "terminal error"},
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

// copilotProbeRules are copilot's rules, kept apart from probeRules because
// they are an ordered precedence list (several rules share a status) rather
// than one rule per verdict.
var copilotProbeRules = []probeRule{
	// Copilot CLI 1.0.93 (SPEC §8.4). Hooks carry its status; these rules are
	// the pane's say where no hook fires (folder trust, an aborted turn, an
	// error line) and the fallback when the plugin could not be installed.
	// Fitted to the real captures in testdata/probes/copilot/ (see
	// copilot-PROVENANCE.md). FIRST MATCH WINS and the order below is the
	// R219 precedence: trust, permission, question, working, error, idle.
	// Every marker is a substring that survives an 80-column wrap; none is a
	// whole footer line (the wrap splits "Manual Approval").
	{kind: "copilot", contains: []string{"Confirm folder trust", "Do you trust the files in this folder?"}, status: "waiting", reason: "folder trust"},
	{kind: "copilot", contains: []string{"Do you want to ", "↑/↓ to navigate · enter to select · esc to cancel"}, status: "waiting", reason: "permission prompt"},
	{kind: "copilot", contains: []string{"Copilot needs information."}, status: "waiting", reason: "question"},
	{kind: "copilot", match: copilotWorkingFooter, status: "running", reason: "working indicator"},
	{kind: "copilot", match: copilotErrorLine, status: "error", reason: "error line"},
	{kind: "copilot", contains: []string{"· / commands"}, status: "idle", reason: "ready"},
	{kind: "copilot", contains: []string{"@ files · # issues"}, status: "idle", reason: "ready"},
}

func probe(kind, pane string) (status, reason string) {
	tail := lastContentLine(pane)
	rules := probeRules
	if kind == "copilot" {
		rules = copilotProbeRules
	}
	for _, rule := range rules {
		if rule.kind == kind && rule.matches(pane, tail) {
			return rule.status, rule.reason
		}
	}
	return "", ""
}

// matches reports whether every one of the rule's conditions holds for pane.
// tail is the pane's lastContentLine, computed once by the caller.
func (rule probeRule) matches(pane, tail string) bool {
	text := pane
	if rule.live {
		text = claudeLiveRegion(pane)
	}
	for _, marker := range rule.contains {
		if !strings.Contains(text, marker) {
			return false
		}
	}
	if rule.match != nil && !rule.match(pane) {
		return false
	}
	return rule.tailPrefix == "" || strings.HasPrefix(tail, rule.tailPrefix)
}

// claudeLiveRegion returns the part of a Claude pane that can still be live:
// from the last transcript line down to the composer, composer excluded (R311).
// A transcript line is one that starts with Claude's turn glyphs: `●` (an
// assistant message or tool call), `⎿` (a tool result) or the prompt glyph
// `❯`/`>` followed by anything but a dialog option number (an echoed user
// turn). The composer is the chrome claudeComposerStart finds. Anything above
// the last transcript line has been followed by newer output, so a marker
// there (an old `API Error:`, an answered `Do you want to proceed?`) is
// history. A pane with no transcript line is live throughout.
func claudeLiveRegion(pane string) string {
	lines := strings.Split(strings.ReplaceAll(pane, "\r\n", "\n"), "\n")
	lines = lines[:claudeComposerStart(lines)]
	start := 0
	for i := len(lines) - 1; i >= 0; i-- {
		if isClaudeTranscriptLine(lines[i]) {
			start = i
			break
		}
	}
	return strings.Join(lines[start:], "\n")
}

// claudeComposerStart is the index where the composer begins: the shared
// separator boundary when the pane draws one, else (no composer chrome) a
// bare trailing prompt line, which is the composer too.
func claudeComposerStart(lines []string) int {
	if end := transcriptEnd(lines); end < len(lines) {
		return end
	}
	end := len(lines)
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	if end > 0 && isClaudePromptLine(lines[end-1]) {
		return end - 1
	}
	return len(lines)
}

func isClaudeTranscriptLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "●") || strings.HasPrefix(trimmed, "⎿") || isClaudePromptLine(line)
}

// isClaudePromptLine reports a line led by the prompt glyph that is not a
// dialog's numbered option (claudeOptionCursor).
func isClaudePromptLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "❯") && !strings.HasPrefix(trimmed, ">") {
		return false
	}
	return !claudeOptionCursor.MatchString(line)
}

// lastContentLine returns the pane's last non-blank transcript line,
// ignoring the composer/footer chrome pi and claude both draw at the very
// bottom of every pane regardless of status: two box-drawing separator
// lines framing the (usually blank) editable composer line, followed by a
// footer of cwd and usage stats that says nothing about the current turn.
// It answers "what did the agent actually leave on screen last".
func lastContentLine(pane string) string {
	lines := strings.Split(strings.ReplaceAll(pane, "\r\n", "\n"), "\n")
	for i := transcriptEnd(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || isSeparatorLine(line) {
			continue
		}
		return line
	}
	return ""
}

// transcriptEnd is the index of the first footer-chrome line: the composer's
// top separator (frameCuts' chosen cut), or len(lines) when the pane draws no
// composer frame. It is the one boundary lastContentLine, claudeLiveRegion and
// TranscriptRegion share.
func transcriptEnd(lines []string) int {
	cuts, _ := frameCuts(lines)
	return cuts[0]
}

// frameCuts lists where the composer frame may start, the chosen cut first,
// and returns the index of the frame's bottom rule (-1 when it has none).
//
// The bottom rule is the pane's last separator line; the top one is a separator
// above it drawn identically (same characters, same width), because pi, claude
// and copilot draw both rules of the composer frame as one full-width line. A
// draft line that is a rule of another shape ("---", an indented "  ───") is
// composer content, not the frame (R304). A pane with no identical pair cuts at
// the earlier of its last two (possibly non-adjacent) separator lines, and at
// len(lines) when it has fewer than two; either way that is its only cut.
//
// A draft can also hold a line drawn exactly like the frame (pasted, or typed
// to the pane's width), and so can the transcript (a divider a tool printed), so
// every identical rule is a candidate: plausibleTops drops those only the
// transcript can have drawn, then the frame's own shape picks the top one:
//   - claude and copilot lead the composer with their prompt glyph: the top
//     rule is the topmost candidate a column-0 "❯" line follows. A draft line
//     can quote "❯" too, but never above the frame's own, so the topmost wins.
//   - pi draws no glyph (its frame is followed by its cwd line, drawn from
//     column 0: glyphlessFrame), so a "❯" line under a rule is draft text
//     there and is no evidence; pi always leaves a blank line above the frame,
//     so its top rule is the topmost candidate a blank line (or the pane's top)
//     precedes.
//   - with neither evidence the nearest identical rule is taken.
//
// The other candidates follow the chosen cut, nearest first: the output clock
// (service.OutputClock) uses them to keep a draft-only edit from counting even
// when a draft mimics the frame more closely than this rule can tell apart.
func frameCuts(lines []string) ([]int, int) {
	bottom := -1
	fallback := len(lines)
	var identical []int // nearest first
	for i := len(lines) - 1; i >= 0; i-- {
		if !isSeparatorLine(strings.TrimSpace(lines[i])) {
			continue
		}
		if bottom < 0 {
			bottom = i
			continue
		}
		if fallback == len(lines) {
			fallback = i
		}
		if sameRule(lines[i], lines[bottom]) {
			identical = append(identical, i)
		}
	}
	if len(identical) == 0 {
		if fallback == len(lines) {
			return []int{fallback}, -1
		}
		return []int{fallback}, bottom
	}
	identical = plausibleTops(lines, bottom, identical)
	chosen := pickTopRule(lines, bottom, identical)
	cuts := []int{identical[chosen]}
	cuts = append(cuts, identical[:chosen]...)
	return append(cuts, identical[chosen+1:]...), bottom
}

// plausibleTops narrows the frame-identical rules (nearest first) to those that
// can be the composer's top, judged from the pane alone, so a rule the
// transcript itself drew (a divider printed by a tool, a markdown rule) is not
// taken for the frame even on the clock's first capture of a session. The
// shapes the three framed agents draw differ, and each leaves evidence the
// other side of the frame cannot fake:
//   - claude and copilot lead the composer with a column-0 "❯" line, and every
//     draft continuation is indented by the prompt's width (two columns), so a
//     rule the composer's first line does not follow, or one with a "●" turn
//     bullet drawn in column 0 or 1 between it and the bottom rule (claude
//     draws it flush left, copilot after one space), is transcript.
//   - pi draws no glyph and starts every composer line at column 0, while its
//     transcript is padded by one column, so a rule with an indented line
//     between it and the bottom rule is transcript.
//
// A pane none of whose rules passes keeps all of them: the evidence ranks, it
// never leaves the frame without a top. What remains is genuine ambiguity (a
// draft that mimics the frame and the transcript the same way), which
// pickTopRule and the clock's alternative cuts handle.
func plausibleTops(lines []string, bottom int, identical []int) []int {
	glyphless := glyphlessFrame(lines, bottom)
	keep := identical
	if !glyphless {
		keep = narrow(keep, func(i int) bool { return i+1 < bottom && strings.HasPrefix(lines[i+1], "❯") })
	}
	return narrow(keep, func(i int) bool { return !transcriptBetween(lines, i, bottom, glyphless) })
}

// narrow returns the entries of tops for which ok holds, or tops itself when
// none does.
func narrow(tops []int, ok func(int) bool) []int {
	var out []int
	for _, i := range tops {
		if ok(i) {
			out = append(out, i)
		}
	}
	if len(out) == 0 {
		return tops
	}
	return out
}

// transcriptBetween reports a line between the rule at top and the bottom rule
// that only the transcript draws (plausibleTops).
func transcriptBetween(lines []string, top, bottom int, glyphless bool) bool {
	for _, line := range lines[top+1 : bottom] {
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		switch {
		case trimmed == "" || isSeparatorLine(trimmed):
		case glyphless && indent > 0:
			return true
		case !glyphless && indent < 2 && strings.HasPrefix(trimmed, "●"):
			return true
		}
	}
	return false
}

// pickTopRule is the index into identical (nearest first) of the frame's top
// rule, by the evidence frameCuts describes.
func pickTopRule(lines []string, bottom int, identical []int) int {
	glyphless := glyphlessFrame(lines, bottom)
	prompted, blankAbove := -1, -1
	for k, i := range identical {
		if !glyphless && i+1 < bottom && strings.HasPrefix(lines[i+1], "❯") {
			prompted = k
		}
		if i == 0 || strings.TrimSpace(lines[i-1]) == "" {
			blankAbove = k
		}
	}
	switch {
	case prompted >= 0:
		return prompted
	case blankAbove >= 0:
		return blankAbove
	}
	return 0
}

// glyphlessFrame reports pi's composer frame: the line under its bottom rule is
// pi's cwd line, a path drawn from column 0. Claude and copilot indent their
// footer (or draw none), and lead their composer with the "❯" glyph instead.
func glyphlessFrame(lines []string, bottom int) bool {
	if bottom < 0 || bottom+1 >= len(lines) {
		return false
	}
	next := lines[bottom+1]
	return strings.HasPrefix(next, "/") || strings.HasPrefix(next, "~")
}

// sameRule reports whether two separator lines are drawn identically, ignoring
// trailing redraw padding.
func sameRule(a, b string) bool {
	return strings.TrimRight(a, " \t") == strings.TrimRight(b, " \t")
}

// outputCuts lists where the composer/footer chrome may start, the chosen cut
// first, for every supported pane shape: codex's "›" composer (it draws no
// separators, so its composer line and the status line under it are the
// chrome), else the separator frame pi, claude and copilot draw round their
// composer (frameCuts, with its alternative cuts), else a bare trailing claude
// prompt line. len(lines) when the pane draws none of them. bottom is the
// frame's bottom rule, or -1 when the chrome is not a frame.
func outputCuts(lines []string) (cuts []int, bottom int) {
	if end := trimTrailingBlank(lines, len(lines)); codexComposerStart(lines, end) < end {
		return []int{codexComposerStart(lines, end)}, -1
	}
	if cuts, bottom := frameCuts(lines); cuts[0] < len(lines) {
		return cuts, bottom
	}
	return []int{claudeComposerStart(lines)}, -1
}

// TranscriptCut is one reading of a pane: Region is the transcript above a
// candidate composer boundary (what TranscriptRegion returns for the chosen
// one), Composer the frame from that boundary down to its bottom rule, the
// draft included and the footer under it left out ("" when the chrome is not a
// separator frame).
type TranscriptCut struct {
	Region   string
	Composer string
}

// TranscriptCuts returns the pane's readings, the chosen boundary first
// (TranscriptRegion's), then any other frame-identical rule that could be the
// composer's top, nearest first. Only a framed draft holding a line drawn
// exactly like the frame yields more than one.
func TranscriptCuts(pane string) []TranscriptCut {
	lines := strings.Split(strings.ReplaceAll(pane, "\r\n", "\n"), "\n")
	cuts, bottom := outputCuts(lines)
	out := make([]TranscriptCut, 0, len(cuts))
	for _, cut := range cuts {
		reading := TranscriptCut{Region: maskLiveIndicator(normaliseRegion(lines[:cut]))}
		if bottom >= cut {
			reading.Composer = normaliseRegion(lines[cut : bottom+1])
		}
		out = append(out, reading)
	}
	return out
}

// TranscriptRegion returns the part of pane above the composer/footer chrome:
// everything above the composer's top separator (transcriptEnd, the boundary
// lastContentLine uses) or above codex's composer line, whichever shape the
// pane has (the whole pane when it has neither). Trailing whitespace on each
// line and trailing blank lines are dropped, so redraw padding is not a change.
// The R304 output clock hashes it, so a ctx:%, branch or model change in the
// footer never reads as output.
func TranscriptRegion(pane string) string {
	return TranscriptCuts(pane)[0].Region
}

var (
	// liveEscIndicator is claude's and codex's working indicator: an optional
	// spinner/bullet glyph, a status phrase, then a parenthetical that ends in
	// "esc to interrupt" and carries the elapsed/token counters ("✳ Working…
	// (12s · esc to interrupt)", "• Working (5s • esc to interrupt)",
	// "• Reconnecting... 4/5 (2s • esc to interrupt)"). Group 1 is the indent,
	// 2 the phrase, 3 the parenthetical.
	liveEscIndicator = regexp.MustCompile(`^(\s*)(?:[✳✶✻✢✽·*•]\s+)?(\S[^()]*?)\s*(\((?:[^()]*\s)?esc to interrupt\))\s*$`)
	// livePiSpinner is pi's working indicator, a braille spinner frame then its
	// message (" ⠧ Working..."). Group 1 is the indent, 2 the message.
	livePiSpinner = regexp.MustCompile(`^(\s*)[\x{2800}-\x{28FF}]\s+(\S.*)$`)
	// livePiElapsed is the elapsed counter pi draws under a running tool call,
	// alone on its line (" Elapsed 11.0s").
	livePiElapsed = regexp.MustCompile(`^\s*Elapsed [0-9]+(?:\.[0-9]+)?s$`)
	// liveRetryCounter is codex's "Reconnecting... 4/5" attempt counter.
	liveRetryCounter = regexp.MustCompile(`^Reconnecting\.\.\. [0-9]+/[0-9]+$`)
	liveDigits       = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`)
)

// maskLiveIndicator makes an agent's live working indicator, the line the
// agent redraws every tick while it works, a constant: its spinner frame and
// its elapsed/retry counters are statusline churn, not output (R304). Only the
// exact shapes the agents draw, in the one place they draw them (the bottom of
// the region, straight above the composer), are masked, so transcript prose
// that merely reads "Working on ticket 41" or "Elapsed 41 seconds", and a
// completed indicator- or counter-shaped line further up the transcript, still
// count:
//   - claude/codex: the head of the region's last cell (the lowest column-0
//     line; only indented continuation lines such as codex's "└ …" error
//     detail or claude's "⎿ Tip" may sit under it), when it is shaped
//     "<glyph> <phrase> (… esc to interrupt)", loses its glyph and the digits
//     inside the parenthetical (and codex's "Reconnecting... n/m" counter);
//     the phrase is kept.
//   - pi: a region whose last line is the braille spinner line loses that
//     frame glyph, and the last non-blank line above it, when it reads exactly
//     "Elapsed N.Ns" (the running tool's counter), is digit-masked.
//
// The indicator appearing or vanishing still changes the region, so a turn
// starting or ending counts; any other line is untouched.
func maskLiveIndicator(region string) string {
	if region == "" {
		return region
	}
	lines := strings.Split(region, "\n")
	if i := lastCellHead(lines); i >= 0 {
		if m := liveEscIndicator.FindStringSubmatch(lines[i]); m != nil {
			phrase := m[2]
			if liveRetryCounter.MatchString(phrase) {
				phrase = liveDigits.ReplaceAllString(phrase, "#")
			}
			lines[i] = m[1] + phrase + " " + liveDigits.ReplaceAllString(m[3], "#")
		}
	}
	last := len(lines) - 1
	if m := livePiSpinner.FindStringSubmatch(lines[last]); m != nil {
		lines[last] = m[1] + m[2]
		i := last - 1
		for i >= 0 && strings.TrimSpace(lines[i]) == "" {
			i--
		}
		if i >= 0 && livePiElapsed.MatchString(lines[i]) {
			lines[i] = liveDigits.ReplaceAllString(lines[i], "#")
		}
	}
	return strings.Join(lines, "\n")
}

// lastCellHead is the index of the region's lowest non-blank line drawn from
// column 0 (the head of the last transcript cell), or -1 when every non-blank
// line is indented.
func lastCellHead(lines []string) int {
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		return i
	}
	return -1
}

// normaliseRegion joins lines with trailing whitespace and trailing blank lines
// dropped.
func normaliseRegion(in []string) string {
	lines := make([]string, len(in))
	for i := range in {
		lines[i] = strings.TrimRight(in[i], " \t")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
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

// codexTerminalError reports whether the pane's last content block, above
// codex's composer and footer, is led by codex's error glyph: a line starting
// "■ " in column 0. Codex indents an agent cell's and a tool result's
// continuation lines, and wraps a long error line with indented continuations
// too, so the block's head is found by walking up over indented lines. The
// composer (codexComposerStart: the placeholder or a draft of any length, then
// the footer) is cut first; a pane without one is read to its end.
func codexTerminalError(pane string) bool {
	lines := strings.Split(strings.ReplaceAll(pane, "\r\n", "\n"), "\n")
	end := trimTrailingBlank(lines, len(lines))
	end = trimTrailingBlank(lines, codexComposerStart(lines, end))
	if end == 0 {
		return false
	}
	i := end - 1
	for i > 0 && (strings.HasPrefix(lines[i], " ") || strings.HasPrefix(lines[i], "\t")) {
		i--
	}
	return strings.HasPrefix(lines[i], "■ ")
}

// trimTrailingBlank is end moved up past blank lines.
func trimTrailingBlank(lines []string, end int) int {
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return end
}

// codexDialogMarker leads the last line of every codex selection dialog
// ("Press enter to confirm or esc to cancel", "Press enter to continue"). A
// dialog replaces the composer, so its "› N." cursor line is dialog content.
const codexDialogMarker = "Press enter to "

// codexComposerStart is the index of codex's composer, or end when the pane
// shows none. Codex draws every transcript cell ("› you", "• agent", "⚠", "■")
// from column 0 and indents its continuation lines; the composer is drawn the
// same way, its draft's further lines and the status line under it all
// indented. So the composer is the last column-0 line before end, provided it
// is a "›" line: whatever the draft says (a numbered list item included) and
// however many lines it spans. A pane whose last non-blank line is a dialog's
// "Press enter to ..." line has no composer.
func codexComposerStart(lines []string, end int) int {
	end = trimTrailingBlank(lines, end)
	if end == 0 || strings.HasPrefix(strings.TrimSpace(lines[end-1]), codexDialogMarker) {
		return end
	}
	for i := end - 1; i >= 0; i-- {
		line := lines[i]
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		if strings.HasPrefix(line, "›") {
			return i
		}
		break
	}
	return end
}

// copilotWorkingFooter reports whether the pane's live footer is Copilot's
// busy footer: a line led by one of the spinner glyphs ○ ◎ ● ◉ carrying
// "Working" and either "esc interrupt" or "esc edit prompt". Only the region
// below the composer's last separator rule is read (copilotFooterLines), so a
// "● Working ... esc interrupt" line quoted in the transcript above the
// composer never reads as the live footer; a pane with no composer has no
// footer and so no working verdict.
func copilotWorkingFooter(pane string) bool {
	for _, line := range copilotFooterLines(pane) {
		if !hasAnyPrefix(line, "○ ", "◎ ", "● ", "◉ ") || !strings.Contains(line, "Working") {
			continue
		}
		if strings.Contains(line, "esc interrupt") || strings.Contains(line, "esc edit prompt") {
			return true
		}
	}
	return false
}

// copilotErrorLine reports whether a line starting "✗ " sits in the
// transcript after the last turn and above the context line (the cwd line
// copilot draws over the composer). A "! " line is a warning, never an error.
func copilotErrorLine(pane string) bool {
	transcript := copilotTranscript(pane)
	if len(transcript) > 0 {
		transcript = transcript[:len(transcript)-1] // the context line
	}
	for i := len(transcript) - 1; i >= 0; i-- {
		line := transcript[i]
		if strings.HasPrefix(line, "✗ ") {
			return true
		}
		if strings.HasPrefix(line, "❯ ") {
			return false // the last turn: nothing newer carries an error
		}
	}
	return false
}

// copilotTranscript returns the pane's non-blank lines with the composer
// chrome (the two separators framing the input and the footer below them)
// removed, each trimmed on the right only so a leading glyph stays visible.
func copilotTranscript(pane string) []string {
	lines := strings.Split(strings.ReplaceAll(pane, "\r\n", "\n"), "\n")
	end, separators := len(lines), 0
	for i := len(lines) - 1; i >= 0 && separators < 2; i-- {
		if isSeparatorLine(strings.TrimSpace(lines[i])) {
			separators++
			end = i
		}
	}
	if separators < 2 {
		end = len(lines)
	}
	return nonBlankLines(strings.Join(lines[:end], "\n"))
}

// copilotFooterLines returns the non-blank lines below the composer's last
// separator, or nil when the pane shows no composer.
func copilotFooterLines(pane string) []string {
	lines := strings.Split(strings.ReplaceAll(pane, "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if isSeparatorLine(strings.TrimSpace(lines[i])) {
			return nonBlankLines(strings.Join(lines[i+1:], "\n"))
		}
	}
	return nil
}

// nonBlankLines splits pane into its non-blank lines, trimmed of surrounding
// whitespace.
func nonBlankLines(pane string) []string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(pane, "\r\n", "\n"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}
