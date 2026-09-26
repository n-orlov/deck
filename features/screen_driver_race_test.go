package features

import (
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/vt"
)

// TestScreenDriverGridReadersAreRaceFreeAgainstRead drives ScreenDriver's own
// read loop -- the goroutine that writes every PTY chunk into the screen and
// shadow budget emulators -- with a continuous redraw stream, while the
// accessors feature steps use to read the grid (CellAt's returned cell,
// FindText, gridText, FrameFitsBudget, Frame) run concurrently from another
// goroutine, exactly as a step polling a live client does.
//
// It only proves anything under `go test -race` (the nightly lane, R145):
// nightly run 36234392586 reported a WARNING: DATA RACE between
// ScreenDriver.read's emulator Write and FindText reading a cell's Content
// after CellAt had already released the driver's mutex, because CellAt
// handed back a pointer into the live grid rather than a copy. Without the
// detector the test still exercises every accessor for panics.
func TestScreenDriverGridReadersAreRaceFreeAgainstRead(t *testing.T) {
	const cols, rows = 40, 6
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	d := &ScreenDriver{
		terminal:    readEnd,
		screen:      vt.NewEmulator(cols, rows),
		budget:      vt.NewEmulator(cols+frameBudgetMargin, rows+frameBudgetMargin),
		updated:     make(chan struct{}, 1),
		done:        make(chan struct{}),
		readDone:    make(chan struct{}),
		tallestRows: rows,
	}
	go d.read()
	go d.drainScreenInput()
	go d.drainBudgetInput()
	defer func() {
		closeInputPipe(d.screen)
		closeInputPipe(d.budget)
	}()

	var writer sync.WaitGroup
	writer.Add(1)
	go func() {
		defer writer.Done()
		defer writeEnd.Close()
		for i := 0; i < 400; i++ {
			word := "alpha"
			if i%2 == 1 {
				word = "bravo"
			}
			// Home the cursor and repaint every cell, so each chunk rewrites
			// exactly the cells the reader below is scanning.
			frame := "\x1b[H" + strings.Repeat(word+strings.Repeat(" ", cols-len(word)), rows-1)
			if _, err := writeEnd.WriteString(frame); err != nil {
				return
			}
		}
	}()

	for i := 0; i < 200; i++ {
		if cell := d.CellAt(0, 0); cell != nil {
			_ = cell.Content
			_ = cell.Style.Fg
		}
		_, _, _ = d.FindText("bravo")
		_ = d.gridText()
		_ = d.FrameFitsBudget(cols, rows)
		_ = d.Frame(false)
	}
	writer.Wait()
	<-d.readDone

	row, col, err := d.FindText("bravo")
	if err != nil {
		t.Fatalf("after the stream ends the final repaint must be findable: %v", err)
	}
	if row != 0 || col != 0 {
		t.Fatalf("FindText(bravo) = row %d col %d, want row 0 col 0", row, col)
	}
}
