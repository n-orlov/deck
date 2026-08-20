package features

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerPreviewSteps backs features/preview.feature (task 022, SPEC
// requirements 21/39): black-box coverage of the preview capture engine's
// own side-effect-free contract (no attached tmux client, no resized pane,
// no SIGWINCH, no capture while the panel is below its floor) and its
// visible behaviour (the real-geometry crop, the wide-cell boundary, the
// no-op mouse gestures, the crash tail, and the no-live-pane placeholder).
// Every step here observes deck through the released binary's own rendered
// frame or tmux's own public state, never an internal channel.
func registerPreviewSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" selects the next session$`, clientSelectsNextSession)
	sc.Step(`^deck client "([^"]+)" screen matches the pattern "([^"]+)"$`, clientScreenMatchesPattern)
	sc.Step(`^deck client "([^"]+)" every full-width row is bordered on both edges$`, clientEveryFullWidthRowIsBorderedOnBothEdges)
	sc.Step(`^deck client "([^"]+)" fills the selected session's pane with wide characters$`, clientFillsSelectedPaneWithWideCharacters)
	sc.Step(`^the private tmux pane for session "([^"]+)" prints "([^"]+)"$`, privateTMuxPaneForSessionPrints)
	sc.Step(`^the private tmux window for session "([^"]+)" is captured as "([^"]+)"$`, privateTMuxWindowForSessionIsCapturedAs)
	sc.Step(`^the private tmux window for session "([^"]+)" still matches "([^"]+)"$`, privateTMuxWindowForSessionStillMatches)
	sc.Step(`^the private tmux server reports no attached clients$`, privateTMuxServerReportsNoAttachedClients)
	sc.Step(`^the fake claude agent's size log is captured as "([^"]+)"$`, fakeClaudeAgentSizeLogIsCapturedAs)
	sc.Step(`^the fake claude agent's size log still matches "([^"]+)"$`, fakeClaudeAgentSizeLogStillMatches)
	sc.Step(`^deck client "([^"]+)" screen stops containing "([^"]+)"$`, clientScreenStopsContaining)
}

// clientSelectsNextSession drives the sidebar's own down-arrow binding
// (unchanged by this phase's mouse/layout work) to move the selection to
// the next row, so a scenario can exercise "the preview reacts to a
// selection change" without needing to know which row ends up selected.
func clientSelectsNextSession(ctx context.Context, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send("\x1b[B"); err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	return nil
}

// clientScreenMatchesPattern is clientScreenContains's regexp-aware sibling,
// used for assertions like requirement 23's "45x22 of 120x40" geometry line
// whose numbers are not fixed across runs. It polls the same way
// WaitForFrame does, rather than trusting a single immediate read.
func clientScreenMatchesPattern(ctx context.Context, name, pattern string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("compile pattern %q: %w", pattern, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		frame := client.Frame(false)
		if re.MatchString(frame) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("client %q screen never matched pattern %q:\n%s", name, pattern, frame)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// clientEveryFullWidthRowIsBorderedOnBothEdges is requirement 24's
// black-box border-alignment invariant: every rendered line long enough to
// be a real panel row (not the shorter footer line below the boxes) must
// start and end with one of this phase's own border glyphs. Under the
// scenario's DECK_ASCII=1 environment those are "+" (corners) and "|"
// (sides) exclusively; a wide glyph that was ever split mid-cell would push
// or shrink whatever comes after it in that row, and the row's own trailing
// border character (task 019's whole point) would land somewhere other
// than the fixed column every other row's border shares -- which this
// This checks the direct, visible symptom of that: the border character
// itself going missing from the row's last column.
func clientEveryFullWidthRowIsBorderedOnBothEdges(ctx context.Context, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	cols, _ := client.GridSize()
	frame := client.Frame(false)
	borderRunes := map[rune]bool{'+': true, '|': true}
	var bad []string
	for _, line := range strings.Split(frame, "\n") {
		runes := []rune(line)
		// A bordered panel row always fills the frame's full column width
		// (its own border glyph occupies the very last column, so
		// NormalizeFrame's TrimRight never shortens it); the footer line
		// below the boxes is deck's own copy, not a fixed-width border
		// row, so it is excluded by this exact-width match rather than a
		// fuzzy threshold.
		if len(runes) != cols {
			continue
		}
		first, last := runes[0], runes[len(runes)-1]
		if !borderRunes[first] || !borderRunes[last] {
			bad = append(bad, line)
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("client %q has %d full-width row(s) not bordered on both edges:\n%s\nfull frame:\n%s", name, len(bad), strings.Join(bad, "\n"), frame)
	}
	return nil
}

// clientFillsSelectedPaneWithWideCharacters prints a line of East-Asian-Wide
// glyphs into the scenario's one live pane, wide enough to force a
// horizontal crop against the preview panel at any layout this phase
// supports (requirement 24). It polls the pane's own capture-pane output
// (never deck's rendering) for the printed glyph before returning, so the
// caller's own assertion is never racing the shell's echo.
func clientFillsSelectedPaneWithWideCharacters(ctx context.Context, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := onlyLiveSessionSlug(h)
	if err != nil {
		return err
	}
	// The pane's own terminal (real geometry, not the panel) wraps the
	// printed line -- and, since send-keys -l echoes character by character
	// before Enter is even pressed, wraps the input itself mid-glyph across
	// physical capture-pane lines. Waiting for the single glyph rather than
	// the whole 60-glyph string sidesteps that: the string is still what
	// ends up in the pane (proven by the later "screen contains 界"
	// assertion against deck's own rendering), just not as one contiguous
	// substring in the harness's own wrapped capture.
	return printIntoPrivatePane(ctx, h, slug, strings.Repeat("界", 60), "界")
}

// privateTMuxPaneForSessionPrints prints literal text into the named
// session's own tmux pane (harness-level setup, exactly like crash_test.go's
// renderColoredCrashFixture, never a deck-internal channel), so a scenario
// can put a recognizable marker into a live pane and later assert whether
// deck's own preview panel shows or withholds it.
func privateTMuxPaneForSessionPrints(ctx context.Context, name, text string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	return printIntoPrivatePane(ctx, h, slug, text, text)
}

func printIntoPrivatePane(ctx context.Context, h *ScenarioHarness, slug, text, waitFor string) error {
	target := "deck_" + slug
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "-l", "printf '%s\\n' "+text); err != nil {
		return fmt.Errorf("print into private pane %q: %w", target, err)
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "Enter"); err != nil {
		return fmt.Errorf("submit print into private pane %q: %w", target, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		output, err := tmuxOutput(ctx, h, "capture-pane", "-p", "-S", "-", "-t", target)
		if err == nil && strings.Contains(string(output), waitFor) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("private pane %q never showed printed text %q (err=%v):\n%s", target, waitFor, err, output)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// onlyLiveSessionSlug is a convenience for scenarios that keep exactly one
// live session, so a step needs no session name of its own.
func onlyLiveSessionSlug(h *ScenarioHarness) (string, error) {
	db, err := openObservedDatabase(h)
	if err != nil {
		return "", err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT slug FROM sessions`)
	if err != nil {
		return "", fmt.Errorf("query session slugs: %w", err)
	}
	defer rows.Close()
	var slugs []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return "", err
		}
		slugs = append(slugs, slug)
	}
	if len(slugs) != 1 {
		return "", fmt.Errorf("expected exactly one session, found %d: %v", len(slugs), slugs)
	}
	return slugs[0], nil
}

// privateTMuxWindowForSessionIsCapturedAs snapshots the real tmux-reported
// window geometry (never deck's own layout numbers) for the named session's
// private window, so a later step can prove the preview capture engine
// never changed it (requirement 21: capture-pane is read-only).
func privateTMuxWindowForSessionIsCapturedAs(ctx context.Context, name, label string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	geometry, err := privateWindowGeometry(ctx, h, name)
	if err != nil {
		return err
	}
	if h.windowGeometrySnapshots == nil {
		h.windowGeometrySnapshots = make(map[string]string)
	}
	h.windowGeometrySnapshots[label] = geometry
	return nil
}

func privateTMuxWindowForSessionStillMatches(ctx context.Context, name, label string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	want, ok := h.windowGeometrySnapshots[label]
	if !ok {
		return fmt.Errorf("no private tmux window geometry was captured as %q", label)
	}
	got, err := privateWindowGeometry(ctx, h, name)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("private tmux window for session %q geometry changed: captured %q as %q, now %q", name, label, want, got)
	}
	return nil
}

func privateWindowGeometry(ctx context.Context, h *ScenarioHarness, name string) (string, error) {
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return "", err
	}
	target := "deck_" + slug
	output, err := tmuxOutput(ctx, h, "list-panes", "-t", target, "-F", "#{window_width}x#{window_height}")
	if err != nil {
		return "", fmt.Errorf("read private tmux window geometry for %q: %w", target, err)
	}
	geometry := strings.TrimSpace(string(output))
	if geometry == "" {
		return "", fmt.Errorf("private tmux window %q reported no geometry", target)
	}
	return geometry, nil
}

// privateTMuxServerReportsNoAttachedClients asserts, straight from tmux's
// own list-clients on the scenario's private socket, that deck's preview
// capture engine never attached a client to read a pane (requirement 21:
// capture-pane only, no attach, no control-mode client, no pipe-pane).
// list-clients with no -t targets the whole server, so this also catches an
// attach to any window, not only the one a scenario happens to name.
func privateTMuxServerReportsNoAttachedClients(ctx context.Context) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	output, err := tmuxOutput(ctx, h, "list-clients")
	if err != nil {
		// tmux exits non-zero with no output when a server has no clients
		// (and, on some versions, when it is asked for an empty list at
		// all) -- treat empty stdout on error the same as a clean empty
		// list, and only a genuinely non-empty result as a failure.
		if strings.TrimSpace(string(output)) == "" {
			return nil
		}
		return fmt.Errorf("list-clients on private tmux server: %w\n%s", err, output)
	}
	if strings.TrimSpace(string(output)) != "" {
		return fmt.Errorf("private tmux server has attached client(s), want none:\n%s", output)
	}
	return nil
}

// fakeClaudeAgentSizeLogPath mirrors cmd/fake-claude's own sizesLogPath
// (DECK_HOME/log/fake-claude-sizes.log): the scenario's deck client and its
// spawned fake-claude pane share DECK_HOME=h.Home (Environment()), so this
// is the same file the fixture itself appends to.
func fakeClaudeAgentSizeLogPath(h *ScenarioHarness) string {
	return filepath.Join(h.Home, "log", "fake-claude-sizes.log")
}

// fakeClaudeAgentSizeLogIsCapturedAs snapshots the fixture's own
// SIGWINCH-observed size log (task 007), so a later step can prove no
// SIGWINCH landed on its pane while the preview captured, cycled layout
// modes, adjusted sidebar_width or the outer terminal resized (requirement
// 21/25/27: capture-pane never resizes the pane it reads).
func fakeClaudeAgentSizeLogIsCapturedAs(ctx context.Context, label string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	content, err := readFakeClaudeSizeLog(h)
	if err != nil {
		return err
	}
	if h.sizeLogSnapshots == nil {
		h.sizeLogSnapshots = make(map[string]string)
	}
	h.sizeLogSnapshots[label] = content
	return nil
}

func fakeClaudeAgentSizeLogStillMatches(ctx context.Context, label string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	want, ok := h.sizeLogSnapshots[label]
	if !ok {
		return fmt.Errorf("no fake claude agent size log was captured as %q", label)
	}
	got, err := readFakeClaudeSizeLog(h)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("fake claude agent size log changed (a SIGWINCH landed on its pane): captured %q as %q, now %q", label, want, got)
	}
	return nil
}

func readFakeClaudeSizeLog(h *ScenarioHarness) (string, error) {
	data, err := os.ReadFile(fakeClaudeAgentSizeLogPath(h))
	if err != nil {
		return "", fmt.Errorf("read fake claude agent size log: %w", err)
	}
	return string(data), nil
}

// clientScreenStopsContaining polls until unwanted disappears from the
// rendered frame or a deadline passes, rather than checking once
// immediately: a resize's re-render (SIGWINCH -> bubbletea WindowSizeMsg ->
// mainView) is asynchronous with the resize step that triggered it, so an
// instantaneous check races that re-render instead of testing what the
// scenario means to test (requirement 27's suppression actually taking
// effect).
func clientScreenStopsContaining(ctx context.Context, name, unwanted string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	var frame string
	for {
		frame = client.Frame(false)
		if !strings.Contains(frame, unwanted) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("client %q screen still contains %q after %s:\n%s", name, unwanted, 3*time.Second, frame)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
