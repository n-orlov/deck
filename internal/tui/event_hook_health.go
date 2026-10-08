package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/n-orlov/deck/internal/notify"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// Event-hook surface (SPEC §10.3, §11.4): the last hook result is shown in
// the session detail (`i`) and in the health lines above the panels, with a
// non-zero exit and a timeout marked, and the health lines probe that the
// configured script exists and is executable next to the PATH probe.

// The visible markers. A failure carries one of the two failure markers so a
// reader (and a test) can tell it from a clean run without reading the code.
const (
	hookMarkerOK       = "ok"
	hookMarkerFailed   = "FAILED"
	hookMarkerTimedOut = "TIMED OUT"
)

// hookHealth is what the health lines know about the event hook: the script
// probe (nil when the script can be spawned) and the newest recorded result.
type hookHealth struct {
	probe error
	run   store.EventHookRun
	found bool
}

// loadHookHealth reads the health lines' facts on a sessions reload: nothing
// at all while no event_hook is configured, so an inert feature costs a
// reload no query and no stat.
func (m Model) loadHookHealth() hookHealth {
	command := strings.Fields(m.settings.EventHook)
	if len(command) == 0 {
		return hookHealth{}
	}
	health := hookHealth{probe: notify.ProbeScript(command)}
	if m.store != nil {
		if run, found, err := m.store.LastEventHookResult(context.Background(), ""); err == nil {
			health.run, health.found = run, found
		}
	}
	return health
}

// hookRunVerdict is the marker and the sentence for one recorded result.
func hookRunVerdict(run store.EventHookRun) (marker, sentence string) {
	switch {
	case run.Error != "":
		return hookMarkerFailed, "could not start: " + run.Error
	case run.TimedOut:
		return hookMarkerTimedOut, "killed when event_hook_timeout expired"
	case run.ExitCode != 0:
		return hookMarkerFailed, fmt.Sprintf("exit status %d", run.ExitCode)
	}
	return hookMarkerOK, "exit status 0"
}

// hookRunSummary is the one-line account of a result: marker, exit status,
// the offered kind it ran for and when.
func (m Model) hookRunSummary(run store.EventHookRun) string {
	marker, sentence := hookRunVerdict(run)
	sep := m.glyph(" · ", " - ")
	parts := []string{marker, sentence}
	if run.Kind != "" {
		parts = append(parts, "for "+run.Kind)
	}
	if run.At > 0 {
		parts = append(parts, m.relativeTime(run.At))
	}
	return strings.Join(parts, sep)
}

// hookRunSurface colours a failure marker in the error token and leaves a
// clean run in the ordinary text token.
func (m Model) hookRunSurface(run store.EventHookRun, text string) string {
	if marker, _ := hookRunVerdict(run); marker != hookMarkerOK {
		return m.colorToken(theme.Error, text)
	}
	return text
}

// hookOutputTail returns the last n non-blank lines of a result's output.
func hookOutputTail(output string, n int) []string {
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(output, "\r", ""), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// hookProbeSentence says what is wrong with the configured script.
func hookProbeSentence(script string, probe error) string {
	switch {
	case probe == nil:
		return ""
	case errors.Is(probe, notify.ErrScriptNotExecutable):
		return script + ": script is not executable; events are recorded but nothing runs"
	default:
		return script + ": script does not exist; events are recorded but nothing runs"
	}
}

// writeDetailEventHook writes the `i` dialog's last-hook-result field and the
// output tail under it. A session no hook ever ran for gets no field.
func (m Model) writeDetailEventHook(b *strings.Builder, session store.Session) {
	if !m.detailHookFound || m.detailHookSessionID != session.ID {
		return
	}
	run := m.detailHookRun
	fmt.Fprintf(b, "%s\n", m.detailField("Last event hook:   ", m.hookRunSurface(run, m.hookRunSummary(run))))
	if lines := hookOutputTail(run.Output, 4); len(lines) > 0 {
		fmt.Fprintf(b, "%s\n", m.detailField("Hook output:       ", ""))
		for _, l := range lines {
			fmt.Fprintf(b, "  %s\n", l)
		}
	}
}

// eventHookHealthLines are the health view's event-hook lines: the script
// probe (only when it fails) and the newest recorded result with its output
// tail. Nothing at all while event_hook is unset.
func (m Model) eventHookHealthLines(width int) []string {
	script := strings.TrimSpace(m.settings.EventHook)
	if script == "" {
		return nil
	}
	var lines []string
	if sentence := hookProbeSentence(strings.Fields(script)[0], m.hookHealth.probe); sentence != "" {
		for _, l := range m.canvasWrapText("Event hook "+hookMarkerFailed+": "+sentence, width) {
			lines = append(lines, m.colorToken(theme.Error, l))
		}
	}
	if !m.hookHealth.found {
		return lines
	}
	run := m.hookHealth.run
	for _, l := range m.canvasWrapText("Event hook last run: "+m.hookRunSummary(run), width) {
		lines = append(lines, m.hookRunSurface(run, l))
	}
	for _, l := range hookOutputTail(run.Output, 2) {
		lines = append(lines, m.canvasWrapText("  "+l, width)...)
	}
	return lines
}
