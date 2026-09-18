package tmux

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// paneSeedDiscriminatorFormat is the fields PRD phase3b II-20 names as
// the discriminators for whether a pane changed WHILE it was being
// captured: #{history_size} (grows whenever a line is pushed off the top
// into scrollback -- i.e. the pane produced output) and
// #{pane_width}/#{pane_height} (change on any resize). tmux processes a
// pane's own output between any two commands it runs, even two commands
// issued in the same invocation, so reading these once before the
// capture and once after is what makes the pairing provably atomic
// rather than merely fast.
//
// #{alternate_on} is the FOURTH discriminator, added for issue #29's
// history-inclusive seed. It is not a "did the pane produce output"
// signal: it is there because the alt-screen re-capture below
// (CapturePaneSeedAtomic) decides whether the body it just accepted is
// an alternate screen at all, and it decides that from #{alternate_on}.
// A pane that flips 0 -> 1 mid-capture hands us the harmful combination
// -- AlternateOn false, so no re-capture, over a body that IS an
// alternate screen with the pane's stale pre-alt-screen history sitting
// above it (a real, measured tmux behaviour: on #{alternate_on}=1,
// `-S -2000` returns the alt screen's rows with the primary screen's
// untouched history prepended). Making the flag a discriminator turns
// that window into a retry instead of a corrupt seed, which is why it
// must keep working even though nothing slices a body any more.
const paneSeedDiscriminatorFormat = "#{history_size}|#{pane_width}|#{pane_height}|#{alternate_on}"

const paneSeedDiscriminatorFieldCount = 4

// maxPaneSeedAtomicAttempts bounds the retry loop CapturePaneSeedAtomic
// runs while its before/after discriminator probes disagree. tmux gives
// no way to freeze a pane's output between two commands, even chained
// into one invocation -- chaining only narrows the race to the time
// capture-pane itself takes to run, it does not close it -- so a bounded
// retry, not a single chained call, is what actually delivers the
// atomicity PRD II-20 asks for.
const maxPaneSeedAtomicAttempts = 20

// paneSeedDiscriminators is one reading of the fields
// paneSeedDiscriminatorFormat lists. Every field is compared by plain
// struct equality (CapturePaneSeedAtomic's `before != after`), which is
// why AlternateOn is kept as the raw 0/1 int tmux reports rather than
// being decoded to a bool: nothing here interprets it, it only has to
// compare.
type paneSeedDiscriminators struct {
	HistorySize int
	PaneWidth   int
	PaneHeight  int
	AlternateOn int
}

// PaneSeedProbesNeverAgreedError is the ONE failure of
// CapturePaneSeedAtomic that means "the pane kept moving underneath us",
// as opposed to "the pane, or the tmux server, or the request itself, is
// broken". It is a distinct type rather than an unstructured
// fmt.Errorf string because internal/interactive.CaptureSeedWithHistory
// has to tell the two apart with errors.As: exhausting
// maxPaneSeedAtomicAttempts on a chatty pane is the exact and only
// condition its degrade-to-no-history retry exists for, and retrying a
// dead transport or a hung server instead would simply double the
// latency of a failure the caller is about to report anyway -- measured
// at 5.00s becoming 10.01s against a tmux that never answers, with
// internal/tui's enterInteractiveBody passing a deadline-less
// context.Background(), i.e. ten seconds of a frozen TUI before the
// "Cannot enter interactive mode: ..." message can render.
//
// Last is the final before/after disagreement observed, kept unwrappable
// so nothing downstream loses detail by classifying this error.
type PaneSeedProbesNeverAgreedError struct {
	// Target is the pane id whose probes never agreed.
	Target string
	// Attempts is how many times the chained invocation was run
	// (maxPaneSeedAtomicAttempts).
	Attempts int
	// Options is the range that actually failed, which is NOT always the
	// range the caller asked for: an alternate-screen pane asked for a
	// history-inclusive range is re-captured visible-only (see
	// CapturePaneSeedAtomic), and it is that second, already-narrow
	// capture which can then run out of attempts. Reporting it lets
	// internal/interactive.CaptureSeedWithHistory tell "the range you
	// asked for was too wide, try a narrower one" from "the narrow range
	// itself could not be paired", so it does not spend another
	// maxPaneSeedAtomicAttempts re-running a byte-identical request and
	// then misreport which range failed.
	Options CaptureOptions
	// Last is the last before/after disagreement, or nil if there was
	// somehow none.
	Last error
}

func (e *PaneSeedProbesNeverAgreedError) Error() string {
	return fmt.Sprintf("capture pane seed atomically for %q: probes never agreed after %d attempts: %v", e.Target, e.Attempts, e.Last)
}

func (e *PaneSeedProbesNeverAgreedError) Unwrap() error { return e.Last }

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
	return paneSeedDiscriminators{HistorySize: ints[0], PaneWidth: ints[1], PaneHeight: ints[2], AlternateOn: ints[3]}, nil
}

// CapturePaneSeedAtomic reads target's full seed state (PRD II-19) and its
// capture-pane body (II-18) chained into ONE tmux invocation --
// `display-message ; capture-pane ; display-message`, joined by tmux's
// own bare `;` command separator, exactly as `tmux new-window \;
// split-window` chains commands interactively -- and re-probes
// #{history_size}, #{pane_width}, #{pane_height} and #{alternate_on}
// immediately before and immediately after the capture (PRD II-20). If
// the two probes disagree, the pane changed size, flipped screen buffers
// or produced output while it was being captured, so the state and body
// just read could no longer match each other; the whole chain is retried
// (up to maxPaneSeedAtomicAttempts) until a pair of probes agrees.
//
// The returned body is BYTE-IDENTICAL to what Client.CapturePane returns
// for the RANGE ACTUALLY CAPTURED against the same quiescent pane,
// trailing newline included -- see capturePaneSeedAtomicOnce for why that
// costs one re-appended byte, and internal/interactive.BuildSeed for why
// the two producers agreeing matters more than it looks (issue #29). For
// every pane but one that range is the CaptureOptions passed in; the
// exception is the alt-screen re-capture below, which narrows the range
// on purpose and is then byte-identical to CapturePane over
// SeedCaptureOptions() instead.
//
// ALT-SCREEN HISTORY SUPPRESSION (issue #29) also lives here rather than
// in SeedCaptureOptionsWithHistory, because the range has to be chosen
// before the invocation runs while #{alternate_on} only comes back from
// the invocation itself: when the accepted state says the pane is on its
// ALTERNATE screen and a wider-than-visible range was requested, the
// whole chained invocation is run a SECOND time at the visible-only
// range and that result is returned instead. See the re-capture below
// for why re-asking tmux is the only correct way to do this and slicing
// the first body in Go is not.
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

	state, body, err := c.capturePaneSeedAtomicRange(ctx, target, options)
	if err != nil {
		return PaneSeedState{}, nil, err
	}

	// ALT-SCREEN SUPPRESSION (issue #29). A pane showing its alternate
	// screen must never be seeded with tmux-side history, for two
	// independent reasons:
	//
	//  1. tmux keeps the PRIMARY screen's history untouched while the
	//     alternate screen is up, and `capture-pane -S -<N>` happily
	//     returns it: measured directly against a real alt-screen pane,
	//     "-S -2000" came back with 24 rows of STALE pre-launch shell
	//     output above the 8 rows the alt screen was actually showing.
	//     Those rows are not this pane's scrollback in any sense a viewer
	//     would recognise -- a full `tmux attach` cannot scroll to them
	//     either, because the alternate screen has no scrollback.
	//  2. internal/interactive's grid bounds only its PRIMARY screen's
	//     scrollback (vt's SetScrollbackSize), and its RenderRows and
	//     ScrollbackLen both read that same primary screen. Rows written
	//     while the seed has already switched the grid to the alternate
	//     buffer would therefore be invisible AND unbounded.
	//
	// The suppression is a RE-CAPTURE, never a slice of the body already
	// in hand, and that distinction is the whole point. `capture-pane -e`
	// is ONE continuous SGR stream across every row: a row can begin with
	// a bare glyph and no SGR of its own, inheriting its pen from the
	// previous row's last cell (the very property
	// internal/interactive/seed.go's step 3 and
	// TestNaivePerLineResetVariantIsWrongInColour exist to protect). So
	// cutting a capture tmux has ALREADY produced at an arbitrary row is
	// not SGR-self-contained, while asking tmux for a narrower range is:
	// tmux re-derives the stream from cell attributes and emits whatever
	// introducers the new first row needs. Measured on tmux 3.6b at 30x8,
	// with an all-red-background history above an alt screen whose row 0
	// is a red bar: the history-inclusive capture emits NO SGR at the
	// history -> alt boundary (the pen is already red), so slicing off the
	// history rows produced "ALTBAR\x1b[49m" where the visible-only
	// capture produces "\x1b[41mALTBAR\x1b[49m" -- a red bar seeded as a
	// DEFAULT-background one. That is a five-byte difference that no row
	// count, and no "does the body still contain the stale text" check,
	// can see, which is why the byte-identity test
	// (TestCapturePaneSeedAtomicAlternateScreenHistoryRangeIsByteIdenticalToVisibleOnly)
	// is the one that guards it.
	//
	// Doing it here rather than in CaptureSeed/CaptureSeedWithHistory
	// means NO caller of CapturePaneSeedAtomic can be handed an
	// SGR-broken body, and it costs one extra chained invocation on
	// alternate-screen panes only.
	//
	// Recursion is impossible by construction rather than by a counter:
	// the re-capture asks for the visible-only range, and
	// `visible != options` is false for that range, so the second result
	// is returned whatever its own AlternateOn says. (It will normally say
	// true -- the pane is still on its alternate screen; the interesting
	// case is a pane that flipped BACK in between, whose second result is
	// then an ordinary primary-screen visible-only capture, still a
	// perfectly consistent state/body pair, just narrower than the caller
	// asked for. Widening it again would be an unbounded loop against a
	// pane toggling 1049h/1049l, which is exactly what a re-drawing editor
	// does.)
	if visible := visibleOnlyCaptureRange(options); state.AlternateOn && visible != options {
		return c.capturePaneSeedAtomicRange(ctx, target, visible)
	}
	return state, body, nil
}

// visibleOnlyCaptureRange is options with its START moved down to the top
// of the pane's VISIBLE SCREEN, preserving every other field (-e, -N, and
// the caller's own EndLine) so the re-captured body is the same KIND of
// body the caller asked for and differs from it in one bound alone. The
// start comes from SeedCaptureOptions rather than being written out here,
// so "where the visible screen begins" has exactly one definition in this
// package and the `visible != options` comparison in
// CapturePaneSeedAtomic cannot start disagreeing with what
// SeedCaptureOptions means.
//
// EndLine is deliberately NOT overwritten with SeedCaptureOptions' own
// "-". Doing that would WIDEN, not narrow, any request whose end was
// bounded above the pane's last row -- an alternate-screen pane asked for
// a bounded sub-range would silently come back carrying the whole visible
// screen, which is more than the caller asked for and the opposite of
// what a suppression should do. No caller in this repository passes such
// a range today (every seed range ends at "-"), so this is a guard
// against a future one rather than a live bug.
func visibleOnlyCaptureRange(options CaptureOptions) CaptureOptions {
	options.StartLine = SeedCaptureOptions().StartLine
	return options
}

// capturePaneSeedAtomicRange is CapturePaneSeedAtomic's retry loop for
// ONE requested range: it builds the chained invocation's argv and runs
// it until a pair of before/after discriminator probes agrees. It does no
// argument validation (its caller has already done that, and the only
// other range it is ever handed is derived from SeedCaptureOptions'
// constants) and no alt-screen decision of its own, so the re-capture
// above cannot recurse through it.
func (c Client) capturePaneSeedAtomicRange(ctx context.Context, target string, options CaptureOptions) (PaneSeedState, []byte, error) {
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
	// Deliberately a typed error, not fmt.Errorf: this is the one failure
	// internal/interactive.CaptureSeedWithHistory is allowed to degrade
	// past (see PaneSeedProbesNeverAgreedError).
	return PaneSeedState{}, nil, &PaneSeedProbesNeverAgreedError{Target: target, Attempts: maxPaneSeedAtomicAttempts, Options: options, Last: lastErr}
}

// capturePaneSeedAtomicOnce runs the chained invocation once and parses
// its three-line-boundaries output. It never retries itself; the retry
// loop lives in capturePaneSeedAtomicRange (it was CapturePaneSeedAtomic
// itself before issue #29 split the alt-screen re-capture decision out of
// it) so a test can observe a single attempt in isolation.
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
	// The body is returned EXACTLY as capture-pane produced it, with no
	// row-level transformation of any kind. In particular the alt-screen
	// history suppression (issue #29) deliberately does NOT live here as a
	// slice to the last pane_height rows: a `-e` capture is one continuous
	// SGR stream across rows, so a Go-side cut is not SGR-self-contained
	// and silently drops the pen the new first row was inheriting. It is a
	// second, narrower capture instead -- see CapturePaneSeedAtomic.
	bodyLines := lines[1 : len(lines)-1]

	// Re-append the ONE trailing "\n" capture-pane emitted after its last
	// row and the chained after-probe line consumed as its own separator,
	// so this body is byte-identical to Client.CapturePane's raw output
	// for the same options. The asymmetry it removes was harmless while
	// the body was visible-screen-only -- a visible-only body's content
	// lands on the grid's TOP rows either way -- but it is fatal with
	// history prepended, because it made the two producers need different
	// trim rules in BuildSeed and the only rule that worked for both
	// (bytes.TrimRight) deleted the body's whole trailing blank-row run,
	// shifting the live screen down off the grid (issue #29; see
	// internal/interactive/seed.go's step-3 comment).
	//
	// The guard is the degenerate zero-row body: an empty body must stay
	// empty rather than become a single blank row, so the newline is only
	// re-appended when there was at least one body line between the two
	// probe lines (equivalently, len(lines) > 2).
	body = []byte(strings.Join(bodyLines, "\n"))
	if len(bodyLines) > 0 {
		body = append(body, '\n')
	}
	return state, body, before, after, nil
}
