package tmux

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// paneSeedDiscriminatorFormat is the three fields PRD phase3b II-20 names
// as the discriminators for whether a pane changed WHILE it was being
// captured: #{history_size} (grows whenever a line is pushed off the top
// into scrollback -- i.e. the pane produced output) and
// #{pane_width}/#{pane_height} (change on any resize). tmux processes a
// pane's own output between any two commands it runs, even two commands
// issued in the same invocation, so reading these once before the
// capture and once after is what makes the pairing provably atomic
// rather than merely fast.
const paneSeedDiscriminatorFormat = "#{history_size}|#{pane_width}|#{pane_height}"

const paneSeedDiscriminatorFieldCount = 3

// maxPaneSeedAtomicAttempts bounds the retry loop CapturePaneSeedAtomic
// runs while its before/after discriminator probes disagree. tmux gives
// no way to freeze a pane's output between two commands, even chained
// into one invocation -- chaining only narrows the race to the time
// capture-pane itself takes to run, it does not close it -- so a bounded
// retry, not a single chained call, is what actually delivers the
// atomicity PRD II-20 asks for.
const maxPaneSeedAtomicAttempts = 20

// paneSeedDiscriminators is one reading of the three fields
// paneSeedDiscriminatorFormat lists.
type paneSeedDiscriminators struct {
	HistorySize int
	PaneWidth   int
	PaneHeight  int
}

func parsePaneSeedDiscriminators(line string) (paneSeedDiscriminators, error) {
	fields := strings.Split(line, "|")
	if len(fields) != paneSeedDiscriminatorFieldCount {
		return paneSeedDiscriminators{}, fmt.Errorf("pane seed discriminator format returned %d fields, want %d: %q", len(fields), paneSeedDiscriminatorFieldCount, line)
	}
	ints := make([]int, paneSeedDiscriminatorFieldCount)
	for i, f := range fields {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil {
			return paneSeedDiscriminators{}, fmt.Errorf("pane seed discriminator field %d (%q): %w", i, f, err)
		}
		ints[i] = n
	}
	return paneSeedDiscriminators{HistorySize: ints[0], PaneWidth: ints[1], PaneHeight: ints[2]}, nil
}

// CapturePaneSeedAtomic reads target's full seed state (PRD II-19) and its
// capture-pane body (II-18) chained into ONE tmux invocation --
// `display-message ; capture-pane ; display-message`, joined by tmux's
// own bare `;` command separator, exactly as `tmux new-window \;
// split-window` chains commands interactively -- and re-probes
// #{history_size}, #{pane_width} and #{pane_height} immediately before
// and immediately after the capture (PRD II-20). If the two probes
// disagree, the pane changed size or produced output while it was being
// captured, so the state and body just read could no longer match each
// other; the whole chain is retried (up to maxPaneSeedAtomicAttempts)
// until a pair of probes agrees.
//
// target must be a tmux pane id ("%N"), the same contract CapturePane
// already enforces, since this issues the identical capture-pane call
// chained alongside the state reads.
func (c Client) CapturePaneSeedAtomic(ctx context.Context, target string, options CaptureOptions) (PaneSeedState, []byte, error) {
	if c.Socket == "" {
		return PaneSeedState{}, nil, errors.New("tmux socket name is required")
	}
	if !paneIDPattern.MatchString(target) {
		return PaneSeedState{}, nil, fmt.Errorf("invalid tmux pane id %q", target)
	}
	if !captureLinePattern.MatchString(options.StartLine) {
		return PaneSeedState{}, nil, fmt.Errorf("invalid capture start line %q", options.StartLine)
	}
	if !captureLinePattern.MatchString(options.EndLine) {
		return PaneSeedState{}, nil, fmt.Errorf("invalid capture end line %q", options.EndLine)
	}

	captureArgs := []string{"capture-pane", "-p"}
	if options.IncludeEscapeSequences {
		captureArgs = append(captureArgs, "-e")
	}
	if options.PreserveTrailingBlankLines {
		captureArgs = append(captureArgs, "-N")
	}
	captureArgs = append(captureArgs, "-S", options.StartLine, "-E", options.EndLine, "-t", target)

	args := []string{"display-message", "-p", "-t", target, paneSeedStateFormat + "|" + paneSeedDiscriminatorFormat, ";"}
	args = append(args, captureArgs...)
	args = append(args, ";", "display-message", "-p", "-t", target, paneSeedDiscriminatorFormat)

	var lastErr error
	for attempt := 0; attempt < maxPaneSeedAtomicAttempts; attempt++ {
		state, body, before, after, err := c.capturePaneSeedAtomicOnce(ctx, target, args)
		if err != nil {
			return PaneSeedState{}, nil, err
		}
		if before != after {
			lastErr = fmt.Errorf("pane %q seed probes disagreed on attempt %d: before=%+v after=%+v", target, attempt+1, before, after)
			continue
		}
		return state, body, nil
	}
	return PaneSeedState{}, nil, fmt.Errorf("capture pane seed atomically for %q: probes never agreed after %d attempts: %w", target, maxPaneSeedAtomicAttempts, lastErr)
}

// capturePaneSeedAtomicOnce runs the chained invocation once and parses
// its three-line-boundaries output. It never retries itself; the retry
// loop lives in CapturePaneSeedAtomic so a test can observe a single
// attempt in isolation.
func (c Client) capturePaneSeedAtomicOnce(ctx context.Context, target string, args []string) (state PaneSeedState, body []byte, before, after paneSeedDiscriminators, err error) {
	output, runErr := c.run(ctx, args...)
	if runErr != nil {
		return PaneSeedState{}, nil, paneSeedDiscriminators{}, paneSeedDiscriminators{}, fmt.Errorf("capture pane seed atomically for %q: %w", target, runErr)
	}

	// The three commands' outputs land back to back with no extra
	// separator tmux inserts between them: each display-message call
	// already ends its own single line with one "\n", and capture-pane's
	// own body already ends with one "\n" after its last row. Trimming
	// exactly the one trailing "\n" the final display-message call
	// produced and then splitting on "\n" therefore yields
	// [stateLine, bodyLine..., afterLine] regardless of how many rows the
	// body has, including zero.
	trimmed := strings.TrimRight(string(output), "\n")
	lines := strings.Split(trimmed, "\n")
	if len(lines) < 2 {
		return PaneSeedState{}, nil, paneSeedDiscriminators{}, paneSeedDiscriminators{}, fmt.Errorf("capture pane seed atomically for %q: unexpected output %q", target, output)
	}

	stateFields := strings.Split(lines[0], "|")
	wantFields := paneSeedStateFieldCount + paneSeedDiscriminatorFieldCount
	if len(stateFields) != wantFields {
		return PaneSeedState{}, nil, paneSeedDiscriminators{}, paneSeedDiscriminators{}, fmt.Errorf("pane seed atomic state line for %q returned %d fields, want %d: %q", target, len(stateFields), wantFields, lines[0])
	}
	state, err = parsePaneSeedState(strings.Join(stateFields[:paneSeedStateFieldCount], "|"))
	if err != nil {
		return PaneSeedState{}, nil, paneSeedDiscriminators{}, paneSeedDiscriminators{}, fmt.Errorf("parse pane seed atomic state for %q: %w", target, err)
	}
	before, err = parsePaneSeedDiscriminators(strings.Join(stateFields[paneSeedStateFieldCount:], "|"))
	if err != nil {
		return PaneSeedState{}, nil, paneSeedDiscriminators{}, paneSeedDiscriminators{}, fmt.Errorf("parse pane seed atomic before-probe for %q: %w", target, err)
	}
	after, err = parsePaneSeedDiscriminators(lines[len(lines)-1])
	if err != nil {
		return PaneSeedState{}, nil, paneSeedDiscriminators{}, paneSeedDiscriminators{}, fmt.Errorf("parse pane seed atomic after-probe for %q: %w", target, err)
	}
	body = []byte(strings.Join(lines[1:len(lines)-1], "\n"))
	return state, body, before, after, nil
}
