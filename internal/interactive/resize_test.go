// resize_test.go proves PRD phase3b II-21/II-22: a resize is always a
// full reseed into a fresh grid, never a bare Resize() call on the
// existing one, and that the "fresh grid" requirement is load-bearing
// specifically because it also buys a fresh parser.
package interactive

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/tmux"
)

// TestNoCodePathCallsGridResizeAlone is the mechanical guard for PRD
// II-21's "never merely vt.Resize()": the *only* legitimate way this
// package corrects a grid for a new size is Session.Resize's full
// reseed-into-a-fresh-grid (resize.go) -- nothing in this package's
// non-test source may call the emulator's own Resize method (embedded
// via vt.SafeEmulator, so `<expr>.Resize(` is the shape any such call
// would take) as a substitute.
//
// Demonstrate-then-revert: temporarily add `_ = g.Resize(1, 1)` for any
// *Grid value g to resize.go and rerun this test -- it goes red, naming
// the file, confirming the check is live -- then remove it again before
// committing.
func TestNoCodePathCallsGridResizeAlone(t *testing.T) {
	resizeCallRe := regexp.MustCompile(`\.Resize\(`)

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir(internal/interactive): %v", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", name, err)
		}
		if loc := resizeCallRe.FindIndex(data); loc != nil {
			line := 1 + strings.Count(string(data[:loc[0]]), "\n")
			t.Errorf("%s:%d: calls .Resize( directly -- every size correction must go through Session.Resize's full reseed (PRD II-21), never the grid's own Resize alone", name, line)
		}
	}
}

// TestReseedIntoSameGridCorruptsAfterTruncatedControlSequence is the RED
// control PRD II-22 names directly: reusing the SAME grid instance for a
// reseed -- rather than a fresh one -- hands whatever seed bytes come
// next to a parser that may still be sitting mid-way through a control
// sequence live output left unfinished, and the seed's own leading bytes
// get silently consumed as that sequence's continuation instead of being
// painted as the visible characters they actually are.
//
// The control sequence used here, CSI with no parameters, terminates on
// ANY subsequent letter (it does not require digits first) -- so a grid
// that stopped after just the introducer "\x1b[" treats the very next
// byte as the sequence's final byte, whatever it is. Ending the seed
// text with an "H" (a legal CSI final byte, mapping to CUP -- cursor
// position -- with defaulted row/column) reproduces the exact failure
// class II-22 measured: the "H" is consumed as that command instead of
// printed, and the grid renders "ELLO" where the seed said "HELLO".
func TestReseedIntoSameGridCorruptsAfterTruncatedControlSequence(t *testing.T) {
	g := newGrid(10, 3)

	// Live output stopped mid control sequence: only the CSI introducer
	// arrived before whatever cut the stream off (a resize, in II-22's
	// scenario) -- no parameters, no final byte yet.
	if _, err := g.Write([]byte("\x1b[")); err != nil {
		t.Fatalf("write truncated control sequence: %v", err)
	}

	// The "reseed", applied the WRONG way this test exists to rule out:
	// straight into the same, already-mid-sequence grid.
	seed := []byte("HELLO")
	if _, err := g.Write(seed); err != nil {
		t.Fatalf("write seed into reused grid: %v", err)
	}

	if gridContains(g, "HELLO") {
		t.Fatalf("expected corruption: reusing the same grid for a reseed after a truncated control sequence must NOT reproduce the seed's text faithfully -- if it did, this control proves nothing and the real bug (task 044/II-22) could still be present")
	}
	if !gridContains(g, "ELLO") {
		t.Fatalf("test assumption violated: expected the corrupted grid to at least show ELLO (with the leading H consumed as the truncated sequence's final byte), got neither HELLO nor ELLO -- the corruption mechanism assumed here did not occur")
	}
}

// TestSessionResizeIntoFreshGridDoesNotCorrupt is the positive case: the
// SAME truncated-control-sequence setup, but corrected the way
// Session.Resize actually does it -- into a brand-new grid rather than
// the old one -- reproduces the seed exactly, because a fresh grid has
// no unfinished parser state left over to misinterpret any of it.
func TestSessionResizeIntoFreshGridDoesNotCorrupt(t *testing.T) {
	socket := interactiveSocket("resize-fresh-parser")
	cleanup := newBareInteractiveSession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	session, err := Start(ctx, client, "s0", 40, 10, func(ctx context.Context) ([]byte, error) {
		return rawCapturePane(t, socket, "s0"), nil
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer session.Close()

	// Put the CURRENT grid into the same truncated-mid-control-sequence
	// state the RED control above starts from, by writing directly into
	// it (Session exposes no other way to reach into the grid, which is
	// exactly why this needs the real Grid() accessor rather than a
	// second constructor).
	if _, err := session.Grid().Write([]byte("\x1b[")); err != nil {
		t.Fatalf("write truncated control sequence into session grid: %v", err)
	}

	if err := session.Resize(ctx, 40, 10, func(ctx context.Context) ([]byte, error) {
		return []byte("HELLO"), nil
	}); err != nil {
		t.Fatalf("Resize: %v", err)
	}

	if !gridContains(session.Grid(), "HELLO") {
		t.Fatalf("Resize into a fresh grid must reproduce the seed exactly even after the old grid was left mid control-sequence; got a grid without HELLO")
	}
}

// TestSessionResizeReplacesGridInstance proves Resize genuinely swaps in
// a different *Grid rather than mutating the old one's canvas in place
// -- the property that makes the fresh-parser guarantee above hold.
func TestSessionResizeReplacesGridInstance(t *testing.T) {
	socket := interactiveSocket("resize-swap")
	cleanup := newBareInteractiveSession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	session, err := Start(ctx, client, "s0", 40, 10, func(ctx context.Context) ([]byte, error) {
		return rawCapturePane(t, socket, "s0"), nil
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer session.Close()

	before := session.Grid()
	if err := session.Resize(ctx, 50, 12, func(ctx context.Context) ([]byte, error) {
		return []byte("RESIZED"), nil
	}); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	after := session.Grid()

	if before == after {
		t.Fatalf("Resize must replace the session's grid with a new instance, not mutate the old one in place")
	}
	if after.Width() != 50 || after.Height() != 12 {
		t.Fatalf("Resize did not apply the new dimensions: got %dx%d, want 50x12", after.Width(), after.Height())
	}
	if !gridContains(after, "RESIZED") {
		t.Fatalf("Resize's grid does not contain the reseed content")
	}
}

// TestSessionResizeRejectsNilSeed proves Resize refuses to proceed
// without a seed callback -- there is no "just resize" path here at
// all, not even accidentally via a nil func value.
func TestSessionResizeRejectsNilSeed(t *testing.T) {
	socket := interactiveSocket("resize-nil-seed")
	cleanup := newBareInteractiveSession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	session, err := Start(ctx, client, "s0", 40, 10, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer session.Close()

	if err := session.Resize(ctx, 40, 10, nil); err == nil {
		t.Fatalf("Resize(nil seed) must return an error, not silently resize with no reseed")
	}
}

// TestSessionResizeDuringLiveDrainIsRaceFree exercises Resize
// concurrently with the drain goroutine actively writing pipe bytes into
// whatever the current grid is, under -race: currentGrid()'s lock is
// what makes a torn read (a Grid() call or a drain iteration observing
// a grid Resize half-replaced) impossible rather than merely unlikely.
func TestSessionResizeDuringLiveDrainIsRaceFree(t *testing.T) {
	socket := interactiveSocket("resize-race")
	cleanup := newBareInteractiveSession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	session, err := Start(ctx, client, "s0", 40, 10, func(ctx context.Context) ([]byte, error) {
		return rawCapturePane(t, socket, "s0"), nil
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer session.Close()

	// stop is only observed between iterations of the drain goroutine's
	// loop, at the top of the select -- closing it does not interrupt an
	// in-flight sendLiteralLine call. Without waiting for the goroutine to
	// actually exit, the test function can return (running deferred
	// cleanup, and then the *testing.T itself completing) while the
	// goroutine is still mid-call into sendLiteralLine; under CPU
	// contention that call's tmux exec can outlast the test, so a later
	// t.Fatalf from that goroutine panics with "Fail in goroutine after
	// ... has completed" instead of failing the test cleanly. wg makes
	// close(stop)+Wait() a real join: the test function cannot return
	// until the goroutine has observed stop and returned.
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
				i++
				sendLiteralLine(t, socket, "s0", fmt.Sprintf("LINE-%d", i))
				time.Sleep(2 * time.Millisecond)
			}
		}
	}()

	for i := 0; i < 20; i++ {
		if err := session.Resize(ctx, 40, 10, func(ctx context.Context) ([]byte, error) {
			return rawCapturePane(t, socket, "s0"), nil
		}); err != nil {
			t.Fatalf("Resize #%d: %v", i, err)
		}
		_ = session.Grid()
	}
	close(stop)
	wg.Wait()
}
