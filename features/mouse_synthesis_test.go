package features

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

// SGR (1006) mouse-report synthesis (requirement 2). The harness deliberately
// never encodes X10 mouse reports: X10 packs a coordinate into a single byte
// as 32+coordinate, which overflows past column 223, so any scenario that
// exercises a coordinate beyond that column is also proof the harness is not
// quietly depending on X10. SGR instead writes the coordinate as decimal
// text, so it has no such ceiling.
const (
	sgrButtonLeft      = 0
	sgrButtonWheelUp   = 64
	sgrButtonWheelDown = 65
	sgrMotionFlag      = 32
)

// sgrPress encodes an SGR (1006) button-press report at the given 1-based
// column/row. Wheel reports are conventionally sent as a "press" with no
// matching release.
func sgrPress(button, col, row int) string {
	return fmt.Sprintf("\x1b[<%d;%d;%dM", button, col, row)
}

// sgrRelease encodes an SGR (1006) button-release report.
func sgrRelease(button, col, row int) string {
	return fmt.Sprintf("\x1b[<%d;%d;%dm", button, col, row)
}

// sgrMotion encodes an SGR (1006) motion report with a button still held
// (the drag case), which xterm distinguishes from a plain press by adding 32
// to the button number.
func sgrMotion(button, col, row int) string {
	return sgrPress(button+sgrMotionFlag, col, row)
}

// Click sends a synthesized SGR left-button press immediately followed by
// its release at the given 1-based cell coordinate -- a single click.
func (d *ScreenDriver) Click(col, row int) error {
	if err := d.Send(sgrPress(sgrButtonLeft, col, row)); err != nil {
		return err
	}
	return d.Send(sgrRelease(sgrButtonLeft, col, row))
}

// DoubleClick sends two Click sequences in immediate succession. SGR mouse
// reports carry no notion of "double" themselves; a double-click is exactly
// two ordinary clicks close together in time, which is what makes it
// indistinguishable at the byte level from two accidental single clicks
// spaced further apart -- the consumer, not the harness, owns that judgment.
func (d *ScreenDriver) DoubleClick(col, row int) error {
	if err := d.Click(col, row); err != nil {
		return err
	}
	return d.Click(col, row)
}

// WheelUp sends a synthesized SGR wheel-up report at the given coordinate.
func (d *ScreenDriver) WheelUp(col, row int) error {
	return d.Send(sgrPress(sgrButtonWheelUp, col, row))
}

// WheelDown sends a synthesized SGR wheel-down report at the given
// coordinate.
func (d *ScreenDriver) WheelDown(col, row int) error {
	return d.Send(sgrPress(sgrButtonWheelDown, col, row))
}

// Drag sends a press at (fromCol, fromRow), one motion report at
// (toCol, toRow), and a release at (toCol, toRow) -- a press-move-release
// gesture with the left button held throughout.
func (d *ScreenDriver) Drag(fromCol, fromRow, toCol, toRow int) error {
	if err := d.Send(sgrPress(sgrButtonLeft, fromCol, fromRow)); err != nil {
		return err
	}
	if err := d.Send(sgrMotion(sgrButtonLeft, toCol, toRow)); err != nil {
		return err
	}
	return d.Send(sgrRelease(sgrButtonLeft, toCol, toRow))
}

// CaptureSnapshot stores the current normalized frame under name, so a later
// step can assert a gesture changed nothing.
func (d *ScreenDriver) CaptureSnapshot(name string, clockFrozen bool) {
	frame := d.Frame(clockFrozen)
	d.mu.Lock()
	if d.snapshots == nil {
		d.snapshots = make(map[string]string)
	}
	d.snapshots[name] = frame
	d.mu.Unlock()
}

// SnapshotUnchanged reports whether the current normalized frame is
// byte-identical to the one captured under name.
func (d *ScreenDriver) SnapshotUnchanged(name string, clockFrozen bool) (bool, string, string, error) {
	d.mu.Lock()
	want, ok := d.snapshots[name]
	d.mu.Unlock()
	if !ok {
		return false, "", "", fmt.Errorf("no frame was captured as %q", name)
	}
	got := d.Frame(clockFrozen)
	return got == want, want, got, nil
}

func registerMouseSynthesisSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" clicks at column (\d+) row (\d+)$`, clientClicksAt)
	sc.Step(`^deck client "([^"]+)" double-clicks at column (\d+) row (\d+)$`, clientDoubleClicksAt)
	sc.Step(`^deck client "([^"]+)" scrolls the wheel up at column (\d+) row (\d+)$`, clientScrollsWheelUpAt)
	sc.Step(`^deck client "([^"]+)" scrolls the wheel down at column (\d+) row (\d+)$`, clientScrollsWheelDownAt)
	sc.Step(`^deck client "([^"]+)" drags from column (\d+) row (\d+) to column (\d+) row (\d+)$`, clientDragsFromTo)
	sc.Step(`^deck client "([^"]+)" captures its frame as "([^"]+)"$`, clientCapturesFrameAs)
	sc.Step(`^deck client "([^"]+)" frame still matches the captured "([^"]+)" frame$`, clientFrameStillMatchesCaptured)
}

func mouseSynthesisClient(ctx context.Context, name string) (*ScreenDriver, error) {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return nil, err
	}
	return h.Client(name)
}

func clientClicksAt(ctx context.Context, name string, col, row int) error {
	client, err := mouseSynthesisClient(ctx, name)
	if err != nil {
		return err
	}
	return client.Click(col, row)
}

func clientDoubleClicksAt(ctx context.Context, name string, col, row int) error {
	client, err := mouseSynthesisClient(ctx, name)
	if err != nil {
		return err
	}
	return client.DoubleClick(col, row)
}

func clientScrollsWheelUpAt(ctx context.Context, name string, col, row int) error {
	client, err := mouseSynthesisClient(ctx, name)
	if err != nil {
		return err
	}
	return client.WheelUp(col, row)
}

func clientScrollsWheelDownAt(ctx context.Context, name string, col, row int) error {
	client, err := mouseSynthesisClient(ctx, name)
	if err != nil {
		return err
	}
	return client.WheelDown(col, row)
}

func clientDragsFromTo(ctx context.Context, name string, fromCol, fromRow, toCol, toRow int) error {
	client, err := mouseSynthesisClient(ctx, name)
	if err != nil {
		return err
	}
	return client.Drag(fromCol, fromRow, toCol, toRow)
}

func clientCapturesFrameAs(ctx context.Context, name, label string) error {
	client, err := mouseSynthesisClient(ctx, name)
	if err != nil {
		return err
	}
	// Give any in-flight render triggered by a prior step a moment to settle
	// before the baseline snapshot is taken, so the comparison catches a real
	// change from the gesture rather than an unrelated late frame.
	time.Sleep(50 * time.Millisecond)
	client.CaptureSnapshot(label, false)
	return nil
}

func clientFrameStillMatchesCaptured(ctx context.Context, name, label string) error {
	client, err := mouseSynthesisClient(ctx, name)
	if err != nil {
		return err
	}
	// A mouse report deck does not react to produces no output at all, so
	// there is nothing to wait for here; comparing immediately (after a short
	// settle) is deliberate, not a race, since success is the absence of a
	// new frame.
	time.Sleep(100 * time.Millisecond)
	unchanged, want, got, err := client.SnapshotUnchanged(label, false)
	if err != nil {
		return err
	}
	if !unchanged {
		return fmt.Errorf("client %q frame changed after gesture, want unchanged from captured %q:\nwant:\n%s\ngot:\n%s", name, label, want, got)
	}
	return nil
}

func TestSGRMouseEncodesColumnsPastX10Ceiling(t *testing.T) {
	// X10 mouse reporting packs a coordinate into one byte as 32+coordinate,
	// which overflows for any column past 223 (255-32). SGR (1006) instead
	// writes the coordinate as decimal text, so it has no such ceiling; this
	// is the concrete proof that the harness's encoding is SGR, not X10.
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"press at column 224", sgrPress(sgrButtonLeft, 224, 1), "\x1b[<0;224;1M"},
		{"press at column 300", sgrPress(sgrButtonLeft, 300, 1), "\x1b[<0;300;1M"},
		{"release at column 224", sgrRelease(sgrButtonLeft, 224, 1), "\x1b[<0;224;1m"},
		{"motion at column 224", sgrMotion(sgrButtonLeft, 224, 1), "\x1b[<32;224;1M"},
		{"wheel up at column 224", sgrPress(sgrButtonWheelUp, 224, 1), "\x1b[<64;224;1M"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("got %q, want %q", test.got, test.want)
			}
		})
	}
}
