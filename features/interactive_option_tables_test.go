package features

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/n-orlov/deck/internal/tmux"
)

// optionTableDump is every one of tmux's seven distinct option tables for
// one target, captured at one moment. PRD phase3b requirement 12 (II-12)
// asks for a byte-exact restore proof "dumped and compared, not
// spot-checked" -- a step that only re-read window-size itself could not
// tell the difference between "nothing else changed" and "something else
// changed and nobody happened to look at it". tmux's own option storage
// splits into: server options (`-s`, always global -- there is no
// per-object server table); session options, which have a global table
// (`-g`) and a local override table (bare `show-options -t target`);
// window options, likewise split into global (`-g -w`) and local (`-w`);
// and pane options, likewise (`-g -p` and `-p`). `-t target` is accepted
// (and harmless) even for the purely-global tables, so every read below
// uses the same target uniformly rather than special-casing which three
// of the seven need it.
type optionTableDump map[string]string

// optionTableSpecs names the seven tables and the show-options flags that
// select each one, in the fixed order dumpOptionTables always reports
// them so two dumps of the same target are directly comparable table by
// table.
var optionTableSpecs = []struct {
	name  string
	flags []string
}{
	{"server", []string{"-s"}},
	{"global-session", []string{"-g"}},
	{"session", nil},
	{"global-window", []string{"-g", "-w"}},
	{"window", []string{"-w"}},
	{"global-pane", []string{"-g", "-p"}},
	{"pane", []string{"-p"}},
}

// optionTableNames lists optionTableSpecs' own names, for error messages
// that need to say which of the seven a caller asked for that does not
// exist.
func optionTableNames() []string {
	names := make([]string, 0, len(optionTableSpecs))
	for _, spec := range optionTableSpecs {
		names = append(names, spec.name)
	}
	return names
}

// dumpOptionTables reads all seven of tmux's option tables for target in
// seven separate `show-options` invocations (tmux has no single command
// that dumps all scopes/types at once) and returns them keyed by table
// name. Each table's own output is exactly what tmux printed, trimmed of
// only the single trailing newline `CombinedOutput` always adds -- never
// re-sorted or otherwise normalized, since a real reordering of tmux's own
// output would itself be a difference this function must not hide.
func dumpOptionTables(ctx context.Context, socket, target string) (optionTableDump, error) {
	dump := make(optionTableDump, len(optionTableSpecs))
	for _, spec := range optionTableSpecs {
		commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		args := append([]string{"-L", socket, "show-options"}, spec.flags...)
		args = append(args, "-t", target)
		output, err := exec.CommandContext(commandCtx, "tmux", args...).CombinedOutput()
		cancel()
		if err != nil {
			return nil, fmt.Errorf("tmux -L %s show-options %v -t %s (table %q): %w: %s", socket, spec.flags, target, spec.name, err, strings.TrimSpace(string(output)))
		}
		dump[spec.name] = strings.TrimRight(string(output), "\n")
	}
	return dump, nil
}

// optionTableDiff is one table's own before/after difference, reported as
// the two full line sets that differ (not merely "table X differs") so an
// assertion can name exactly which line(s) changed, not just which table.
type optionTableDiff struct {
	table        string
	addedLines   []string
	removedLines []string
}

// diffOptionTables compares two dumps of the same target table by table.
// A table missing from either dump (which dumpOptionTables never
// produces, but a caller comparing mismatched captures could) is treated
// as wholly added/removed. Line-level, not whole-table-string, comparison
// is what lets a caller assert "the only difference is window-size
// changing to manual" rather than merely "the window table differs".
func diffOptionTables(before, after optionTableDump) []optionTableDiff {
	var diffs []optionTableDiff
	for _, name := range optionTableNames() {
		beforeLines := splitNonEmptyLines(before[name])
		afterLines := splitNonEmptyLines(after[name])
		added := linesOnlyIn(afterLines, beforeLines)
		removed := linesOnlyIn(beforeLines, afterLines)
		if len(added) == 0 && len(removed) == 0 {
			continue
		}
		diffs = append(diffs, optionTableDiff{table: name, addedLines: added, removedLines: removed})
	}
	return diffs
}

func splitNonEmptyLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// linesOnlyIn returns the lines present in a but absent from b, duplicates
// counted (a line appearing twice in a and once in b counts as one extra
// occurrence), sorted for a deterministic report order.
func linesOnlyIn(a, b []string) []string {
	remaining := map[string]int{}
	for _, line := range b {
		remaining[line]++
	}
	var only []string
	for _, line := range a {
		if remaining[line] > 0 {
			remaining[line]--
			continue
		}
		only = append(only, line)
	}
	sort.Strings(only)
	return only
}

// registerInteractiveOptionTableSteps wires PRD phase3b requirement 12
// (II-12): after a full enter/exit cycle, `window-size` must be the ONLY
// option ever touched, in the window scope only, and every other option
// table -- server, global/local session, global/local window,
// global/local pane -- must read exactly what it read before entry. Task
// 028's existing scope-aware step (tmuxWindowOptionIsInScope) covers the
// separate "server-global window-size still reads latest" half of the
// same requirement; this file only adds the whole-table dump/diff a
// single-option re-read could never prove.
func registerInteractiveOptionTableSteps(sc *godog.ScenarioContext) {
	sc.Step(`^tmux session "([^"]+)" is a bootstrapped bare (\d+)x(\d+) window$`, tmuxSessionIsABootstrappedBareWindow)
	sc.Step(`^the option tables for tmux target "([^"]+)" are captured as "([^"]+)"$`, theOptionTablesForTmuxTargetAreCapturedAs)
	sc.Step(`^every option table is byte-identical between "([^"]+)" and "([^"]+)" for tmux target "([^"]+)"$`, everyOptionTableIsByteIdenticalBetween)
	sc.Step(`^only the "([^"]+)" option table differs between "([^"]+)" and "([^"]+)" for tmux target "([^"]+)", and the only difference is "([^"]+)" changing to "([^"]+)"$`, onlyTheOptionTableDiffersAndTheOnlyDifferenceIs)
}

// tmuxSessionIsABootstrappedBareWindow creates a single-pane bare tmux
// session directly on this scenario's own private socket, but -- unlike
// tmuxSessionIsABareSplitWindow's own bare sessions -- runs
// internal/tmux.Client.Bootstrap against the socket FIRST, so the
// session's global session/window tables read exactly what a real deck
// server's do (`window-size latest`, `history-limit 10000`, etc. -- task
// 032's Bootstrap) rather than tmux's own untouched defaults. Without
// this, "the server-global window-size still reads latest" (part of this
// requirement) would be asserting a coincidence of tmux's own defaults,
// not deck's own Bootstrap contract.
func tmuxSessionIsABootstrappedBareWindow(ctx context.Context, session string, width, height int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	client := tmux.Client{Socket: h.Socket, Timeout: 5 * time.Second}
	if err := client.Bootstrap(ctx); err != nil {
		return fmt.Errorf("bootstrap tmux server on socket %q: %w", h.Socket, err)
	}
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(commandCtx, "tmux", "-L", h.Socket, "new-session", "-d", "-s", session,
		"-x", strconv.Itoa(width), "-y", strconv.Itoa(height)).CombinedOutput(); err != nil {
		return fmt.Errorf("tmux -L %s new-session -d -s %s -x %d -y %d: %w: %s", h.Socket, session, width, height, err, output)
	}
	return nil
}

func theOptionTablesForTmuxTargetAreCapturedAs(ctx context.Context, target, label string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	dump, err := dumpOptionTables(ctx, h.Socket, target)
	if err != nil {
		return fmt.Errorf("capture option tables for tmux target %q as %q: %w", target, label, err)
	}
	if h.optionTableDumps == nil {
		h.optionTableDumps = make(map[string]optionTableDump)
	}
	h.optionTableDumps[optionTableDumpKey(target, label)] = dump
	return nil
}

func optionTableDumpKey(target, label string) string {
	return target + "\x00" + label
}

func lookupOptionTableDump(h *ScenarioHarness, target, label string) (optionTableDump, error) {
	dump, ok := h.optionTableDumps[optionTableDumpKey(target, label)]
	if !ok {
		return nil, fmt.Errorf("no option tables captured as %q for tmux target %q", label, target)
	}
	return dump, nil
}

func everyOptionTableIsByteIdenticalBetween(ctx context.Context, beforeLabel, afterLabel, target string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	before, err := lookupOptionTableDump(h, target, beforeLabel)
	if err != nil {
		return err
	}
	after, err := lookupOptionTableDump(h, target, afterLabel)
	if err != nil {
		return err
	}
	diffs := diffOptionTables(before, after)
	if len(diffs) == 0 {
		return nil
	}
	return fmt.Errorf("option tables for tmux target %q differ between %q and %q: %s", target, beforeLabel, afterLabel, describeOptionTableDiffs(diffs))
}

func onlyTheOptionTableDiffersAndTheOnlyDifferenceIs(ctx context.Context, wantTable, beforeLabel, afterLabel, target, wantOption, wantValue string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	before, err := lookupOptionTableDump(h, target, beforeLabel)
	if err != nil {
		return err
	}
	after, err := lookupOptionTableDump(h, target, afterLabel)
	if err != nil {
		return err
	}
	diffs := diffOptionTables(before, after)
	if len(diffs) != 1 {
		return fmt.Errorf("option tables for tmux target %q between %q and %q: want exactly one differing table (%q), got %d: %s", target, beforeLabel, afterLabel, wantTable, len(diffs), describeOptionTableDiffs(diffs))
	}
	diff := diffs[0]
	if diff.table != wantTable {
		return fmt.Errorf("option tables for tmux target %q between %q and %q: the one differing table is %q, want %q (diff: %s)", target, beforeLabel, afterLabel, diff.table, wantTable, describeOptionTableDiffs(diffs))
	}
	wantLine := wantOption + " " + wantValue
	if len(diff.addedLines) != 1 || diff.addedLines[0] != wantLine || len(diff.removedLines) != 0 {
		return fmt.Errorf("option table %q for tmux target %q between %q and %q: want exactly one added line %q and nothing removed, got added=%v removed=%v", wantTable, target, beforeLabel, afterLabel, wantLine, diff.addedLines, diff.removedLines)
	}
	return nil
}

func describeOptionTableDiffs(diffs []optionTableDiff) string {
	var parts []string
	for _, diff := range diffs {
		parts = append(parts, fmt.Sprintf("table %q added=%v removed=%v", diff.table, diff.addedLines, diff.removedLines))
	}
	return strings.Join(parts, "; ")
}
