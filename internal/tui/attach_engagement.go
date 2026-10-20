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

	tea "github.com/charmbracelet/bubbletea"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/term"
	"github.com/creack/pty"
	"github.com/muesli/cancelreader"

	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// R321 in a full attach (`a`). The real tmux client draws the whole attach,
// but deck stays in its key path: watchedAttach runs the client on a pty deck
// owns and copies the user's terminal to it, byte for byte, so every key the
// user types passes through deck exactly once (attachKeyProxy). Beside that,
// a watcher samples tmux (tmux.Client.AttachSample, a read-only
// display-message/list-clients/capture-pane) every attachWatchInterval, and
// reads the attached row from the store before every sample. Each of the
// three engagement triggers below is bound to the status episode
// (status_at, status_seq) that held across the whole interval its evidence
// arrived in (episodeBracket); the latest such pair is recorded, and
// onAttachFinished hands the trigger and that observed row -- never the row
// as it is when the attach ends -- to Model.engageRow, the same one function
// the interactive preview's triggers go through:
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
// The watcher writes nothing to tmux. The seen write is engageRow's, issued by
// the watcher the moment a trigger is attributed to an episode (a prompt that
// arrives after the attach began is engaged by the first trigger that observes
// it, not at detach), and it names the observed episode, so it is a no-op if
// the row has moved on to a newer one (a later prompt sharing the millisecond,
// or a stepped-back clock, included): a newer unobserved episode stays
// unseen. onAttachFinished hands the latest trigger to engageRow again, which
// the once-per-episode record makes a no-op.

const (
	attachWatchInterval = 200 * time.Millisecond
	attachWatchSettle   = time.Second
	// attachOutputInterval is how often the attach watcher feeds the output
	// clock while the client is attached (R305): the reconcile ticks do not
	// run for the length of an attach.
	attachOutputInterval = time.Second
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

	// observed counts the samples seen so far; last is the index of the
	// latest one and keyFrom the index of the sample before the one that
	// first showed the pending key. They name where a trigger's evidence
	// interval opens (evidenceFrom), which is what binds the trigger to a
	// status episode (episodeBracket).
	observed int
	last     int
	keyFrom  int
}

// evidenceFrom is the index of the sample that opens the interval the
// trigger's evidence arrived in: the sample before the one that showed it
// (for a key, before the one that showed the forwarded key, not the one that
// confirmed it).
func (d *attachEngagementDetector) evidenceFrom(trigger engagementTrigger) int {
	if trigger == engageKey {
		return d.keyFrom
	}
	return d.last - 1
}

func screenHash(screen []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(screen)
	return h.Sum64()
}

func (d *attachEngagementDetector) observe(now time.Time, s tmux.AttachSample, keys int64) engagementTrigger {
	sum := screenHash(s.Screen)
	d.last = d.observed
	d.observed++
	if !d.started {
		d.start(now, s, keys, sum)
		return ""
	}
	// Scroll: copy mode is entered, or scrolled, only by the user. The new
	// position is the baseline of the next scroll, so a pane left in copy
	// mode is one scroll, not one per sample.
	scrolled := s.InMode && (!d.inMode || s.ScrollPosition != d.scrollPosition)
	d.inMode, d.scrollPosition = s.InMode, s.ScrollPosition
	if scrolled {
		return engageScroll
	}
	if trigger, done := d.observeKey(s, keys); done {
		return trigger
	}
	return d.observeScreen(now, s, sum)
}

// start takes the first sample as the baseline.
func (d *attachEngagementDetector) start(now time.Time, s tmux.AttachSample, keys int64, sum uint64) {
	d.started = true
	d.width, d.height, d.inMode, d.scrollPosition, d.screenHash = s.Width, s.Height, s.InMode, s.ScrollPosition, sum
	d.keys = keys
	if s.ClientFound {
		// No pre-attach baseline: this sample may already hold the
		// attach's resize, and the agent's redraw for it may follow.
		d.settleUntil = now.Add(d.settle)
	}
}

// observeKey is the key rule: a key forwarded since the last sample is
// confirmed by the client still being attached in the root table (a detach
// removes it; a prefix the proxy could not name leaves it in the prefix
// table). done reports that this sample is decided (a trigger, or a key
// waiting for its confirming sample).
func (d *attachEngagementDetector) observeKey(s tmux.AttachSample, keys int64) (trigger engagementTrigger, done bool) {
	if d.pendingKey {
		d.pendingKey = false
		if s.ClientFound && s.ClientKeyTable == "root" {
			return engageKey, true
		}
	}
	if keys > d.keys {
		d.keys = keys
		d.keyFrom = d.last - 1
		// The key's own echo may already be on this screen: wait for the
		// confirming sample rather than calling it a screen change.
		d.pendingKey = true
		return "", true
	}
	return "", false
}

// observeScreen is the screen-content rule, at a settled, unchanged size.
func (d *attachEngagementDetector) observeScreen(now time.Time, s tmux.AttachSample, sum uint64) engagementTrigger {
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
		d.screenHash = sum
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
	p.commit(p.classify(buf))
}

// attachInput is what one chunk of forwarded bytes holds: engagement keys
// (counted for R321) and whether it holds any key or paste at all (a prefix
// key and the key after it included), which the pane may answer with output
// that deck caused (R305).
type attachInput struct {
	keys   int64
	anyKey bool
}

// classify decodes a chunk deck is about to forward, before it is written,
// so the output discount opens before the pane can echo it.
func (p *attachKeyProxy) classify(buf []byte) attachInput {
	var in attachInput
	for len(buf) > 0 {
		n, event := p.decoder.Decode(buf)
		if n <= 0 {
			break
		}
		buf = buf[n:]
		p.event(event, &in)
	}
	return in
}

// commit counts a classified chunk's keys once it has reached the client.
func (p *attachKeyProxy) commit(in attachInput) {
	if in.keys > 0 {
		p.keys.Add(in.keys)
	}
}

func (p *attachKeyProxy) event(event uv.Event, in *attachInput) {
	switch e := event.(type) {
	case uv.MultiEvent:
		for _, inner := range e {
			p.event(inner, in)
		}
	case uv.KeyPressEvent:
		in.anyKey = true
		key := uv.Key(e)
		switch {
		case p.afterPrefix:
			p.afterPrefix = false
		case key.MatchString(p.prefixes...):
			p.afterPrefix = true
		default:
			in.keys++
		}
	case uv.PasteEvent:
		in.anyKey = true
		in.keys++
	}
}

// tmuxKeyStroke turns a tmux key name ("C-b", "M-a", "C-Space", "`", "F12")
// into uv's keystroke form ("ctrl+b"); "None" and names it does not know
// give false, and then only the watcher's key-table check stands guard.
func tmuxKeyStroke(name string) (string, bool) {
	mods, name, ok := tmuxKeyModifiers(name)
	if !ok {
		return "", false
	}
	key, ok := tmuxBaseKey(name, len(mods) > 0)
	if !ok {
		return "", false
	}
	return strings.Join(append(mods, key), "+"), true
}

// tmuxKeyModifiers strips the "C-", "M-" and "S-" prefixes off a tmux key
// name, returning uv's modifier names and the rest; any other prefix letter
// is a name it does not know.
func tmuxKeyModifiers(name string) (mods []string, rest string, ok bool) {
	names := map[byte]string{'C': "ctrl", 'M': "alt", 'S': "shift"}
	for len(name) > 2 && name[1] == '-' {
		mod, known := names[name[0]]
		if !known {
			return nil, "", false
		}
		mods = append(mods, mod)
		name = name[2:]
	}
	return mods, name, true
}

// tmuxBaseKey is uv's name for a tmux key name without modifiers: a named key,
// one character (lower-cased under a modifier), or a function key.
func tmuxBaseKey(name string, hasMods bool) (string, bool) {
	named := map[string]string{"Space": "space", "Escape": "esc", "Tab": "tab", "Enter": "enter", "BSpace": "backspace"}
	if key, ok := named[name]; ok {
		return key, true
	}
	switch {
	case utf8.RuneCountInString(name) == 1:
		if hasMods {
			return strings.ToLower(name), true
		}
		return name, true
	case len(name) >= 2 && name[0] == 'F' && strings.Trim(name[1:], "0123456789") == "":
		return strings.ToLower(name), true
	}
	return "", false
}

// attachSampler reads one AttachSample of the session a full attach holds;
// clientPID is the attach client's pid (0 before the client exists).
type attachSampler func(ctx context.Context, slug string, clientPID int) (tmux.AttachSample, error)

// attachRowReader reads the attached row as the store holds it now.
type attachRowReader func(ctx context.Context) (store.Session, error)

// attachEngagement is what the watcher reports: the trigger, and the row at
// the one status episode the trigger's evidence arrived in.
type attachEngagement struct {
	trigger  engagementTrigger
	observed store.Session
}

// episodeBracket binds a trigger to the status episode it observed (R319,
// R321, R322). The row is read before every sample; a trigger whose evidence
// arrived between sample i and a later one is attributed to an episode only
// when the read before sample i and a read taken after the trigger name the
// same (status_at, status_seq). status_seq only grows, so equal reads mean
// the episode held for the whole interval, whatever the clock did; when the
// row changed episode inside it the trigger is attributed to none.
type episodeBracket struct {
	read  func(ctx context.Context) (store.Session, bool)
	reads map[int]store.Session
}

// before records the row read before sample index.
func (b *episodeBracket) before(ctx context.Context, index int) {
	if row, ok := b.read(ctx); ok {
		b.reads[index] = row
	} else {
		delete(b.reads, index)
	}
	delete(b.reads, index-4)
}

// attribute is the row at the episode a trigger whose evidence interval
// opened at sample from observed, if that episode held until now.
func (b *episodeBracket) attribute(ctx context.Context, from int) (store.Session, bool) {
	start, ok := b.reads[from]
	if !ok {
		return store.Session{}, false
	}
	end, ok := b.read(ctx)
	if !ok || end.ID != start.ID || end.Episode() != start.Episode() {
		return store.Session{}, false
	}
	return end, true
}

// watchedAttach is the tea.ExecCommand `a` runs when a sampler is wired: the
// tmux attach client on a deck-owned pty, the proxy between it and the
// user's terminal, and the engagement watcher for as long as the client
// runs. Trigger and Observed are read by the exec callback, after Run has
// returned.
type watchedAttach struct {
	cmd      *exec.Cmd
	slug     string
	sample   attachSampler
	interval time.Duration
	settle   time.Duration
	now      func() time.Time
	// session is the attached row as deck showed it when the attach began;
	// read, when set, re-reads it from the store. Without read every
	// trigger is attributed to session's own episode.
	session store.Session
	read    attachRowReader
	// engage, when set, is Model.engageRow's seen write for a trigger and
	// the row of the episode it observed. The watcher runs it the moment the
	// trigger is attributed, while the attach is still going, so a prompt
	// that arrives after the attach began is marked seen by the key, scroll
	// or screen change that observed it, not at detach.
	engage func(engagementTrigger, store.Session) tea.Cmd
	// observeOutput, when set, feeds the output clock (R305) every
	// outputEvery while the client is attached: the TUI's reconcile ticks do
	// not run during an attach, so without it an agent's own output in a
	// long attach would only be read once the detach window had begun.
	observeOutput func(context.Context)
	outputEvery   time.Duration
	// discount, when set, is the output clock's Discount for the attached
	// session (R305). Deck forwards every key the user types and every
	// terminal resize to the client, so the pane's answer to each is
	// deck-caused: a forwarded key or paste opens a DiscountKey window just
	// before it is written, a relayed SIGWINCH a DiscountResize window just
	// before the client's pty is resized. Each window is the clock's bounded
	// one; output after it counts, attached or not.
	discount func(service.DiscountCause, time.Time)

	stdin  io.Reader
	stdout io.Writer

	// Trigger is the latest engagement trigger the watcher could attribute
	// to one status episode, "" when none; Observed is the row at that
	// episode, the only one Trigger may set seen.
	Trigger  engagementTrigger
	Observed store.Session
	// Ended is the instant Run saw the client's process end (R410), taken
	// the moment Wait returns: before the output drain, the terminal restore
	// and deck's own re-fit, so the detach repaint's discount span starts no
	// later than deck learns of the detach. Zero until the client has ended.
	Ended time.Time
	// Engaged holds the result of every seen write the watcher issued while
	// the attach ran; onAttachFinished reports the failed ones.
	Engaged []sessionEngaged
}

// engageNow issues the seen write for a trigger the moment it is attributed.
// The write is engageRow's own (decision, once-per-episode record and store
// call); a trigger on a row that is not an unseen waiting/error episode, or on
// an episode already written, issues nothing.
func (w *watchedAttach) engageNow(engaged attachEngagement) {
	if w.engage == nil {
		return
	}
	cmd := w.engage(engaged.trigger, engaged.observed)
	if cmd == nil {
		return
	}
	if result, ok := cmd().(sessionEngaged); ok {
		w.Engaged = append(w.Engaged, result)
	}
}

// readRow is the bracket's read: the store's row, or session when no reader
// is wired.
func (w *watchedAttach) readRow(ctx context.Context) (store.Session, bool) {
	if w.read == nil {
		return w.session, true
	}
	row, err := w.read(ctx)
	return row, err == nil
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
	// The bracket's first read precedes everything the watcher observes.
	bracket := &episodeBracket{read: w.readRow, reads: map[int]store.Session{}}
	bracket.before(context.WithoutCancel(ctx), 0)
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
	resizePty(ptmx, outFile, inFile)
	defer makeRawRestorer(inFile)()
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	go relayResizes(ctx, winch, w.relayedResize(ptmx, outFile, inFile))

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
		copyOutput(w.stdout, ptmx)
	}()
	inDone, input := w.startInputProxy(ptmx, proxy)

	found := make(chan attachEngagement, 1)
	go func() {
		var first *tmux.AttachSample
		if baseErr == nil {
			first = &baseline
		}
		found <- w.watch(ctx, w.cmd.Process.Pid, first, &proxy.keys, bracket)
	}()
	outputDone := w.startOutputObserver(ctx)
	err = w.cmd.Wait()
	w.Ended = time.Now()
	select {
	case <-outDone:
	case <-time.After(attachOutputDrain):
	}
	stopInputProxy(inDone, input)
	cancel()
	<-outputDone
	engaged := <-found
	w.Trigger, w.Observed = engaged.trigger, engaged.observed
	return err
}

// endStampedCommand is the plain attach client (no watcher) as tea.Exec runs
// it: it records the instant the client's process ended (R410), before
// bubbletea restores the terminal and before deck's re-fit, the way
// watchedAttach.Ended does for the watched attach.
type endStampedCommand struct {
	cmd   *exec.Cmd
	Ended time.Time
}

// Run runs the client to its end and stamps Ended the moment it returns.
func (c *endStampedCommand) Run() error {
	err := c.cmd.Run()
	c.Ended = time.Now()
	return err
}

// SetStdin, SetStdout and SetStderr hand the client the terminal unless the
// command already carries its own stream, as tea.ExecProcess does.
func (c *endStampedCommand) SetStdin(r io.Reader) {
	if c.cmd.Stdin == nil {
		c.cmd.Stdin = r
	}
}

func (c *endStampedCommand) SetStdout(w io.Writer) {
	if c.cmd.Stdout == nil {
		c.cmd.Stdout = w
	}
}

func (c *endStampedCommand) SetStderr(w io.Writer) {
	if c.cmd.Stderr == nil {
		c.cmd.Stderr = w
	}
}

// startOutputObserver runs observeOutput every outputEvery until ctx ends and
// returns a channel closed once it has stopped. Without an observer the
// channel is already closed.
func (w *watchedAttach) startOutputObserver(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	if w.observeOutput == nil {
		close(done)
		return done
	}
	every := w.outputEvery
	if every <= 0 {
		every = attachOutputInterval
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.observeOutput(ctx)
			}
		}
	}()
	return done
}

// resizePty gives the attach client's pty the size of the first of the user's
// terminals (out, then in) that reports one.
func resizePty(ptmx *os.File, outFile, inFile fileDescriptor) {
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

// relayedResize is what one relayed SIGWINCH does: open a resize discount
// (R305), then give the client's pty the user's new size, so the redraw the
// resize causes never lands before its discount.
func (w *watchedAttach) relayedResize(ptmx *os.File, outFile, inFile fileDescriptor) func() {
	return func() {
		w.discountOutput(service.DiscountResize, time.Now()) // R410: before the pty is resized
		resizePty(ptmx, outFile, inFile)
	}
}

// makeRawRestorer puts the user's terminal in raw mode and returns the func
// that restores it; with no terminal it does nothing and returns a no-op.
func makeRawRestorer(inFile fileDescriptor) func() {
	if inFile == nil || !term.IsTerminal(inFile.Fd()) {
		return func() {}
	}
	state, err := term.MakeRaw(inFile.Fd())
	if err != nil {
		return func() {}
	}
	return func() { _ = term.Restore(inFile.Fd(), state) }
}

// relayResizes calls resize for every SIGWINCH until ctx ends.
func relayResizes(ctx context.Context, winch <-chan os.Signal, resize func()) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-winch:
			resize()
		}
	}
}

// copyOutput relays the client's pty output to the user's terminal, or drops
// it when there is none.
func copyOutput(out io.Writer, ptmx io.Reader) {
	if out != nil {
		_, _ = io.Copy(out, ptmx)
	} else {
		_, _ = io.Copy(io.Discard, ptmx)
	}
}

// startInputProxy forwards the user's terminal to the client's pty, telling
// the key proxy about every forwarded chunk. The reader is cancelable, so no
// read of deck's own terminal outlives the attach and steals the first key
// meant for deck's UI. inDone is nil when there is no stdin to read.
func (w *watchedAttach) startInputProxy(ptmx io.Writer, proxy *attachKeyProxy) (chan struct{}, cancelreader.CancelReader) {
	if w.stdin == nil {
		return nil, nil
	}
	input, err := cancelreader.NewReader(w.stdin)
	if err != nil {
		return nil, nil
	}
	inDone := make(chan struct{})
	go func() {
		defer close(inDone)
		buf := make([]byte, 4096)
		for {
			n, err := input.Read(buf)
			if n > 0 {
				w.forward(ptmx, proxy, buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	return inDone, input
}

// forward writes one chunk of the user's input to the client. A chunk holding
// a key or paste opens the output discount first (R305), so the pane's echo
// can never land before it; its engagement keys are counted once written.
func (w *watchedAttach) forward(ptmx io.Writer, proxy *attachKeyProxy, chunk []byte) {
	in := proxy.classify(chunk)
	if in.anyKey {
		w.discountOutput(service.DiscountKey, time.Now()) // R410: before the write below
	}
	if _, err := ptmx.Write(chunk); err == nil {
		proxy.commit(in)
	}
}

// discountOutput opens one output discount for the attached session, when a
// clock is wired.
func (w *watchedAttach) discountOutput(cause service.DiscountCause, at time.Time) {
	if w.discount != nil {
		w.discount(cause, at)
	}
}

// stopInputProxy ends the input forwarder. A reader that cannot be canceled
// (stdin that is not a real file) is left behind rather than waited on.
func stopInputProxy(inDone chan struct{}, input cancelreader.CancelReader) {
	if inDone != nil && input.Cancel() {
		<-inDone
		_ = input.Close()
	}
}

// watch samples until ctx ends and reports the latest trigger it could
// attribute to one status episode. A full attach can outlive the episode it
// began on; only the newest episode engaged can still be the row's current
// one, and a trigger whose interval straddled an episode change is dropped
// rather than credited to either side.
func (w *watchedAttach) watch(ctx context.Context, pid int, baseline *tmux.AttachSample, keys *atomic.Int64, bracket *episodeBracket) attachEngagement {
	detector := attachEngagementDetector{settle: w.settle}
	if baseline != nil {
		// The baseline was sampled before the client existed: no key had
		// been forwarded yet.
		detector.observe(w.clock(), *baseline, 0)
	}
	var latest attachEngagement
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		if engaged, ok := w.step(ctx, pid, &detector, bracket, keys); ok {
			latest = engaged
			w.engageNow(engaged)
		}
		select {
		case <-ctx.Done():
			return latest
		case <-ticker.C:
		}
	}
}

// step is one watch round: the row read, the sample, and the trigger it
// shows bound to the episode it observed. ok is false when the round has no
// trigger or the trigger's interval straddled an episode change.
func (w *watchedAttach) step(ctx context.Context, pid int, detector *attachEngagementDetector, bracket *episodeBracket, keys *atomic.Int64) (attachEngagement, bool) {
	readCtx := context.WithoutCancel(ctx)
	bracket.before(readCtx, detector.observed)
	sample, err := w.sample(ctx, w.slug, pid)
	if err != nil {
		return attachEngagement{}, false
	}
	trigger := detector.observe(w.clock(), sample, keys.Load())
	if trigger == "" {
		return attachEngagement{}, false
	}
	row, ok := bracket.attribute(readCtx, detector.evidenceFrom(trigger))
	return attachEngagement{trigger: trigger, observed: row}, ok
}

// clock is the watcher's time: now in tests, the wall clock otherwise.
func (w *watchedAttach) clock() time.Time {
	if w.now == nil {
		return time.Now()
	}
	return w.now()
}

// attachRowReader is the full-attach watcher's store read of row id, nil
// when the Model has no store.
func (m Model) attachRowReader(id string) attachRowReader {
	if m.store == nil {
		return nil
	}
	db := m.store
	return func(ctx context.Context) (store.Session, error) { return db.GetSession(ctx, id) }
}
