package tui

import (
	"context"
	"hash/fnv"
	"io"
	"os/exec"
	"time"

	"github.com/n-orlov/deck/internal/tmux"
)

// R321 in a full attach (`a`). The real tmux client owns the terminal for the
// whole attach, so deck is not in the key path. Instead, a watcher runs
// beside the exec'd client and samples tmux (tmux.Client.AttachSample, a
// read-only display-message/list-clients/capture-pane) every
// attachWatchInterval. It reports the first of the three engagement
// triggers it sees, and onAttachFinished hands that trigger to
// Model.engageRow, the same one function the interactive preview's
// triggers go through:
//
//   - key: the attach client's own client_activity moves past its value at
//     attach while its key table is `root`, and the client is still attached
//     at the next sample. A prefix key (table `prefix`) is not a forwarded
//     key, and a detach (the client gone at the next sample) never counts.
//   - scroll: the pane enters copy mode, or its scroll position moves while
//     in copy mode. Only the user scrolls a pane.
//   - screen-content change: the visible screen's hash differs from its
//     baseline at an unchanged size. A size change (the attach's own resize)
//     re-baselines, and so does every sample for attachWatchSettle after it,
//     so the agent's redraw for that resize is never counted.
//
// The watcher writes nothing to tmux and nothing to the store itself; the
// one write is engageRow's, after the attach ends.

const (
	attachWatchInterval = 200 * time.Millisecond
	attachWatchSettle   = time.Second
)

// attachEngagementDetector is the watcher's pure decision: one observe per
// sample, returning a trigger the first time one is seen.
type attachEngagementDetector struct {
	settle time.Duration

	started        bool
	width, height  int
	inMode         bool
	scrollPosition string
	screenHash     uint64
	settleUntil    time.Time

	activityKnown bool
	activity      int64
	pendingKey    bool
}

func screenHash(screen []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(screen)
	return h.Sum64()
}

func (d *attachEngagementDetector) observe(now time.Time, s tmux.AttachSample) engagementTrigger {
	sum := screenHash(s.Screen)
	if !d.started {
		d.started = true
		d.width, d.height, d.inMode, d.scrollPosition, d.screenHash = s.Width, s.Height, s.InMode, s.ScrollPosition, sum
		d.settleUntil = now.Add(d.settle)
		if s.ClientFound {
			d.activityKnown, d.activity = true, s.ClientActivity
		}
		return ""
	}
	// Scroll: copy mode is entered, or scrolled, only by the user. Checked
	// first, since the wheel or key that scrolled also moved client_activity.
	if (s.InMode && !d.inMode) || (s.InMode && s.ScrollPosition != d.scrollPosition) {
		return engageScroll
	}
	d.inMode, d.scrollPosition = s.InMode, s.ScrollPosition
	// Key: a bump seen in the root table is confirmed by the client still
	// being attached one sample later (a detach removes it).
	if d.pendingKey {
		d.pendingKey = false
		if s.ClientFound {
			return engageKey
		}
	}
	if s.ClientFound {
		switch {
		case !d.activityKnown:
			d.activityKnown, d.activity = true, s.ClientActivity
		case s.ClientActivity > d.activity && s.ClientKeyTable == "root":
			// The key's own echo may already be on this screen: wait for the
			// confirming sample rather than calling it a screen change.
			d.activity = s.ClientActivity
			d.pendingKey = true
			return ""
		}
	}
	// Screen-content change at a settled, unchanged size.
	if s.Width != d.width || s.Height != d.height {
		d.width, d.height, d.screenHash = s.Width, s.Height, sum
		d.settleUntil = now.Add(d.settle)
		return ""
	}
	if now.Before(d.settleUntil) {
		d.screenHash = sum
		return ""
	}
	if sum != d.screenHash {
		return engageScreen
	}
	return ""
}

// attachSampler reads one AttachSample of the session a full attach holds;
// clientPID is the attach client's pid.
type attachSampler func(ctx context.Context, slug string, clientPID int) (tmux.AttachSample, error)

// watchedAttach is the tea.ExecCommand `a` runs when a sampler is wired: the
// tmux attach client itself, plus the engagement watcher for as long as it
// runs. Trigger is read by the exec callback, after Run has returned.
type watchedAttach struct {
	cmd      *exec.Cmd
	slug     string
	sample   attachSampler
	interval time.Duration
	settle   time.Duration
	now      func() time.Time

	Trigger engagementTrigger
}

func (w *watchedAttach) SetStdin(r io.Reader) {
	if w.cmd.Stdin == nil {
		w.cmd.Stdin = r
	}
}

func (w *watchedAttach) SetStdout(out io.Writer) {
	if w.cmd.Stdout == nil {
		w.cmd.Stdout = out
	}
}

func (w *watchedAttach) SetStderr(out io.Writer) {
	if w.cmd.Stderr == nil {
		w.cmd.Stderr = out
	}
}

// Run starts the attach client, watches it until it exits, and records the
// first engagement trigger the watcher saw.
func (w *watchedAttach) Run() error {
	if err := w.cmd.Start(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	found := make(chan engagementTrigger, 1)
	go func() { found <- w.watch(ctx, w.cmd.Process.Pid) }()
	err := w.cmd.Wait()
	cancel()
	w.Trigger = <-found
	return err
}

func (w *watchedAttach) watch(ctx context.Context, pid int) engagementTrigger {
	now := w.now
	if now == nil {
		now = time.Now
	}
	detector := attachEngagementDetector{settle: w.settle}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		sample, err := w.sample(ctx, w.slug, pid)
		if err == nil {
			if trigger := detector.observe(now(), sample); trigger != "" {
				return trigger
			}
		}
		select {
		case <-ctx.Done():
			return ""
		case <-ticker.C:
		}
	}
}
