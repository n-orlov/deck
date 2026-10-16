package tui

import (
	"context"
	"hash/fnv"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/term"
	"github.com/creack/pty"
	"github.com/muesli/cancelreader"

	"github.com/n-orlov/deck/internal/tmux"
)

// R321 in a full attach (`a`). The real tmux client draws the whole attach,
// but deck stays in its key path: watchedAttach runs the client on a pty deck
// owns and copies the user's terminal to it, byte for byte, so every key the
// user types passes through deck exactly once (attachKeyProxy). Beside that,
// a watcher samples tmux (tmux.Client.AttachSample, a read-only
// display-message/list-clients/capture-pane) every attachWatchInterval. The
// first of the three engagement triggers it sees is recorded, and
// onAttachFinished hands it to Model.engageRow, the same one function the
// interactive preview's triggers go through:
//
//   - key: a key event decoded from the bytes deck forwarded (mouse, focus
//     and the terminal's own replies to tmux's queries are not keys), that is
//     not tmux's prefix key nor the key after it, confirmed by the next
//     sample: the client still attached, in its `root` key table. So prefix+d
//     and any detach never count. There is no clock resolution involved: a
//     key in the attach's first millisecond counts like any other.
//   - scroll: the pane enters copy mode, or its scroll position moves while
//     in copy mode. Only the user scrolls a pane.
//   - screen-content change: the visible screen's hash differs from its
//     baseline at an unchanged size. The baseline is sampled before the
//     client exists. A size change (the attach's own resize) re-baselines,
//     and so does every sample for attachWatchSettle after it, so the
//     agent's redraw for that resize is never counted.
//
// The watcher writes nothing to tmux and nothing to the store itself; the
// one write is engageRow's, after the attach ends.

const (
	attachWatchInterval = 200 * time.Millisecond
	attachWatchSettle   = time.Second
)

// attachEngagementDetector is the watcher's pure decision: one observe per
// sample, returning a trigger the first time one is seen. keys is the
// running count of forwarded key events attachKeyProxy has decoded.
type attachEngagementDetector struct {
	settle time.Duration

	started        bool
	width, height  int
	inMode         bool
	scrollPosition string
	screenHash     uint64
	settleUntil    time.Time

	keys       int64
	pendingKey bool
}

func screenHash(screen []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(screen)
	return h.Sum64()
}

func (d *attachEngagementDetector) observe(now time.Time, s tmux.AttachSample, keys int64) engagementTrigger {
	sum := screenHash(s.Screen)
	if !d.started {
		d.started = true
		d.width, d.height, d.inMode, d.scrollPosition, d.screenHash = s.Width, s.Height, s.InMode, s.ScrollPosition, sum
		d.keys = keys
		if s.ClientFound {
			// No pre-attach baseline: this sample may already hold the
			// attach's resize, and the agent's redraw for it may follow.
			d.settleUntil = now.Add(d.settle)
		}
		return ""
	}
	// Scroll: copy mode is entered, or scrolled, only by the user.
	if (s.InMode && !d.inMode) || (s.InMode && s.ScrollPosition != d.scrollPosition) {
		return engageScroll
	}
	d.inMode, d.scrollPosition = s.InMode, s.ScrollPosition
	// Key: forwarded since the last sample, confirmed by the client still
	// being attached in the root table (a detach removes it; a prefix the
	// proxy could not name leaves it in the prefix table).
	if d.pendingKey {
		d.pendingKey = false
		if s.ClientFound && s.ClientKeyTable == "root" {
			return engageKey
		}
	}
	if keys > d.keys {
		d.keys = keys
		// The key's own echo may already be on this screen: wait for the
		// confirming sample rather than calling it a screen change.
		d.pendingKey = true
		return ""
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

// attachKeyProxy classifies the bytes deck forwards from the user's terminal
// to the attach client. It counts key presses (and pasted text) tmux hands
// to the pane: not tmux's prefix keys, not the key that follows a prefix
// (that one runs a tmux command, prefix+d among them), and nothing that is
// not a key at all -- mouse reports, focus changes, and the terminal's
// answers to tmux's own queries.
type attachKeyProxy struct {
	prefixes    []string // uv keystroke names of tmux's prefix and prefix2
	decoder     uv.EventDecoder
	afterPrefix bool
	keys        atomic.Int64
}

func newAttachKeyProxy(prefix, prefix2 string) *attachKeyProxy {
	p := &attachKeyProxy{}
	for _, name := range []string{prefix, prefix2} {
		if stroke, ok := tmuxKeyStroke(name); ok {
			p.prefixes = append(p.prefixes, stroke)
		}
	}
	return p
}

// forwarded records one chunk of bytes deck has just written to the client.
func (p *attachKeyProxy) forwarded(buf []byte) {
	for len(buf) > 0 {
		n, event := p.decoder.Decode(buf)
		if n <= 0 {
			return
		}
		buf = buf[n:]
		p.event(event)
	}
}

func (p *attachKeyProxy) event(event uv.Event) {
	switch e := event.(type) {
	case uv.MultiEvent:
		for _, inner := range e {
			p.event(inner)
		}
	case uv.KeyPressEvent:
		key := uv.Key(e)
		switch {
		case p.afterPrefix:
			p.afterPrefix = false
		case key.MatchString(p.prefixes...):
			p.afterPrefix = true
		default:
			p.keys.Add(1)
		}
	case uv.PasteEvent:
		p.keys.Add(1)
	}
}

// tmuxKeyStroke turns a tmux key name ("C-b", "M-a", "C-Space", "`", "F12")
// into uv's keystroke form ("ctrl+b"); "None" and names it does not know
// give false, and then only the watcher's key-table check stands guard.
func tmuxKeyStroke(name string) (string, bool) {
	var mods []string
	for len(name) > 2 && name[1] == '-' {
		switch name[0] {
		case 'C':
			mods = append(mods, "ctrl")
		case 'M':
			mods = append(mods, "alt")
		case 'S':
			mods = append(mods, "shift")
		default:
			return "", false
		}
		name = name[2:]
	}
	named := map[string]string{"Space": "space", "Escape": "esc", "Tab": "tab", "Enter": "enter", "BSpace": "backspace"}
	key, ok := named[name]
	switch {
	case ok:
	case utf8.RuneCountInString(name) == 1:
		key = name
		if len(mods) > 0 {
			key = strings.ToLower(name)
		}
	case len(name) >= 2 && name[0] == 'F' && strings.Trim(name[1:], "0123456789") == "":
		key = strings.ToLower(name)
	default:
		return "", false
	}
	return strings.Join(append(mods, key), "+"), true
}

// attachSampler reads one AttachSample of the session a full attach holds;
// clientPID is the attach client's pid (0 before the client exists).
type attachSampler func(ctx context.Context, slug string, clientPID int) (tmux.AttachSample, error)

// watchedAttach is the tea.ExecCommand `a` runs when a sampler is wired: the
// tmux attach client on a deck-owned pty, the proxy between it and the
// user's terminal, and the engagement watcher for as long as the client
// runs. Trigger is read by the exec callback, after Run has returned.
type watchedAttach struct {
	cmd      *exec.Cmd
	slug     string
	sample   attachSampler
	interval time.Duration
	settle   time.Duration
	now      func() time.Time

	stdin  io.Reader
	stdout io.Writer

	Trigger engagementTrigger
}

func (w *watchedAttach) SetStdin(r io.Reader) {
	if w.stdin == nil {
		w.stdin = r
	}
}

func (w *watchedAttach) SetStdout(out io.Writer) {
	if w.stdout == nil {
		w.stdout = out
	}
}

func (w *watchedAttach) SetStderr(io.Writer) {}

func clampUint16(v int) uint16 {
	switch {
	case v < 0:
		return 0
	case v > math.MaxUint16:
		return math.MaxUint16
	}
	return uint16(v)
}

// fileDescriptor is the user's terminal as Run needs it: a file it can put
// in raw mode and read the size of.
type fileDescriptor interface {
	Fd() uintptr
}

// attachOutputDrain bounds how long Run waits, after the client exits, for
// the last of its output (its terminal restore) to reach the user.
const attachOutputDrain = 2 * time.Second

// Run starts the attach client on its own pty, proxies the user's terminal
// to it until it exits, and records the first engagement trigger the
// watcher saw.
func (w *watchedAttach) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The baseline is sampled before the client exists, so the attach's own
	// resize shows up as a size change and its first second is not blind.
	baseline, baseErr := w.sample(ctx, w.slug, 0)
	proxy := newAttachKeyProxy(baseline.Prefix, baseline.Prefix2)

	ptmx, tty, err := pty.Open()
	if err != nil {
		return err
	}
	defer func() { _ = ptmx.Close() }()
	inFile, _ := w.stdin.(fileDescriptor)
	outFile, _ := w.stdout.(fileDescriptor)
	resize := func() {
		for _, f := range []fileDescriptor{outFile, inFile} {
			if f == nil || !term.IsTerminal(f.Fd()) {
				continue
			}
			if cols, rows, err := term.GetSize(f.Fd()); err == nil {
				_ = pty.Setsize(ptmx, &pty.Winsize{Rows: clampUint16(rows), Cols: clampUint16(cols)})
				return
			}
		}
	}
	resize()
	if inFile != nil && term.IsTerminal(inFile.Fd()) {
		if state, err := term.MakeRaw(inFile.Fd()); err == nil {
			defer func() { _ = term.Restore(inFile.Fd(), state) }()
		}
	}
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-winch:
				resize()
			}
		}
	}()

	w.cmd.Stdin, w.cmd.Stdout, w.cmd.Stderr = tty, tty, tty
	w.cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	startErr := w.cmd.Start()
	_ = tty.Close()
	if startErr != nil {
		return startErr
	}

	outDone := make(chan struct{})
	go func() {
		defer close(outDone)
		if w.stdout != nil {
			_, _ = io.Copy(w.stdout, ptmx)
		} else {
			_, _ = io.Copy(io.Discard, ptmx)
		}
	}()
	var inDone chan struct{}
	var input cancelreader.CancelReader
	if w.stdin != nil {
		// Cancelable, so no read of deck's own terminal outlives the attach
		// and steals the first key meant for deck's UI.
		if input, err = cancelreader.NewReader(w.stdin); err == nil {
			inDone = make(chan struct{})
			go func() {
				defer close(inDone)
				buf := make([]byte, 4096)
				for {
					n, err := input.Read(buf)
					if n > 0 {
						if _, werr := ptmx.Write(buf[:n]); werr == nil {
							proxy.forwarded(buf[:n])
						}
					}
					if err != nil {
						return
					}
				}
			}()
		}
	}

	found := make(chan engagementTrigger, 1)
	go func() {
		var first *tmux.AttachSample
		if baseErr == nil {
			first = &baseline
		}
		found <- w.watch(ctx, w.cmd.Process.Pid, first, &proxy.keys)
	}()
	err = w.cmd.Wait()
	select {
	case <-outDone:
	case <-time.After(attachOutputDrain):
	}
	if inDone != nil && input.Cancel() {
		// A reader that cannot be canceled (stdin that is not a real file)
		// is left behind rather than waited on.
		<-inDone
		_ = input.Close()
	}
	cancel()
	w.Trigger = <-found
	return err
}

func (w *watchedAttach) watch(ctx context.Context, pid int, baseline *tmux.AttachSample, keys *atomic.Int64) engagementTrigger {
	now := w.now
	if now == nil {
		now = time.Now
	}
	detector := attachEngagementDetector{settle: w.settle}
	if baseline != nil {
		detector.observe(now(), *baseline, keys.Load())
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		sample, err := w.sample(ctx, w.slug, pid)
		if err == nil {
			if trigger := detector.observe(now(), sample, keys.Load()); trigger != "" {
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
