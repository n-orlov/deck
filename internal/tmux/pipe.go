package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PanePipe is a long-lived stream of one pane's raw bytes, produced by
// `pipe-pane -IO` (PRD phase3b II-16). It is the ONLY primitive that reads
// a pane's live output; nothing else in this package taps pipe-pane, and
// a pane may only be connected to one command at a time (task 046/II-24
// covers what happens when something else displaces it).
//
// Close disarms the pipe and removes the temporary FIFO. It is idempotent
// and safe to call after a Read has already failed.
type PanePipe struct {
	client   Client
	target   string
	fifo     *os.File
	tempDir  string
	disarmed bool
}

// ArmPipePane starts `pipe-pane -IO` on target, writing into a fresh named
// pipe that this call opens and returns (as the embedded reader). Call
// this BEFORE taking any seed capture of the pane (PRD II-16): pipe-pane
// only streams bytes emitted from the moment it is armed onward, so a
// snapshot taken first and armed second loses everything the pane emits
// in between, permanently and undetectably -- the snapshot already ran,
// and the pipe was not listening yet, so neither side ever sees those
// bytes. Arming first means any such interstitial bytes are queued by the
// pipe and delivered to the reader once draining starts, even though they
// arrived before the caller ever asked for a seed.
func (c Client) ArmPipePane(ctx context.Context, target string) (*PanePipe, error) {
	tempDir, err := os.MkdirTemp("", "deck-interactive-pipe-")
	if err != nil {
		return nil, fmt.Errorf("create pipe temp dir: %w", err)
	}
	p := &PanePipe{client: c, target: target, tempDir: tempDir}
	failed := true
	defer func() {
		if failed {
			_ = os.RemoveAll(tempDir)
		}
	}()

	fifoPath := filepath.Join(tempDir, "pane.fifo")
	if output, err := exec.CommandContext(ctx, "mkfifo", fifoPath).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("mkfifo %s: %w: %s", fifoPath, err, output)
	}

	// O_RDWR avoids the read-only open blocking until tmux's own pipe-pane
	// job opens the write side (which does not exist yet at this line),
	// and keeps this single descriptor valid, as both ends, for the whole
	// life of the pipe -- Close below is what finally lets a blocked Read
	// return.
	p.fifo, err = os.OpenFile(fifoPath, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open pane fifo %s: %w", fifoPath, err)
	}

	if _, err := c.run(ctx, "pipe-pane", "-IO", "-t", target, "cat >> "+shellQuote(fifoPath)); err != nil {
		return nil, fmt.Errorf("arm pipe-pane -IO on %s: %w", target, err)
	}
	failed = false
	return p, nil
}

// Read satisfies io.Reader by reading raw pane bytes off the FIFO. It
// blocks like any other stream read; it returns an error once Close has
// closed the underlying descriptor.
func (p *PanePipe) Read(b []byte) (int, error) {
	return p.fifo.Read(b)
}

// Close disarms `pipe-pane` on the target (a bare `pipe-pane -t target`
// with no command disables it, per tmux(1)) and removes the FIFO. It is
// idempotent: calling it twice, or after the target session/pane has
// already gone away, never returns an error the caller has to handle
// specially -- disarming a pipe that is not this one's anymore (task
// 046/II-24's displacement case) is not this type's problem to detect.
func (p *PanePipe) Close() error {
	if p.disarmed {
		return nil
	}
	p.disarmed = true

	ctx, cancel := context.WithTimeout(context.Background(), p.client.timeout())
	defer cancel()
	_, _ = p.client.run(ctx, "pipe-pane", "-t", p.target)

	var err error
	if p.fifo != nil {
		err = p.fifo.Close()
		p.fifo = nil
	}
	if removeErr := os.RemoveAll(p.tempDir); err == nil {
		err = removeErr
	}
	return err
}

// shellQuote produces a single-quoted shell word safe to embed in the
// shell-command argument tmux hands to sh -c for pipe-pane/run-shell; the
// only character that needs escaping inside single quotes is a literal
// single quote itself.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
