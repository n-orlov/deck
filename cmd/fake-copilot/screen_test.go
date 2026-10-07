package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"unicode/utf8"

	"github.com/creack/pty"

	"github.com/n-orlov/deck/internal/agent"
)

func screenEnv() map[string]string {
	return map[string]string{commandsEnvironment: "1", screenEnvironment: "1"}
}

// lockedBuffer is a writer a test can read while the fixture is still writing.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

var cursorRow = regexp.MustCompile("\x1b\\[(\\d+);1H")

// lastFrame interprets the fixture's output the way a terminal would for its
// own frames: everything after the last "clear screen", one entry per row.
func lastFrame(output string) string {
	const clearScreen = "\x1b[H\x1b[2J"
	start := strings.LastIndex(output, clearScreen)
	if start < 0 {
		return ""
	}
	body := output[start+len(clearScreen):]
	if end := strings.Index(body, "\x1b[?25h"); end >= 0 {
		body = body[:end]
	}
	var rows []string
	marks := cursorRow.FindAllStringSubmatchIndex(body, -1)
	for i, mark := range marks {
		stop := len(body)
		if i+1 < len(marks) {
			stop = marks[i+1][0]
		}
		rows = append(rows, body[mark[1]:stop])
	}
	return strings.Join(rows, "\n")
}

func probeOf(pane string) (string, string) { return (agent.Copilot{}).Probe(pane) }

func newScreen(cols, rows int) *screen {
	return &screen{out: io.Discard, size: func() (int, int) { return cols, rows }, cwd: "/tmp/work"}
}

// R224 item 4: every state R219 distinguishes is rendered with the substrings
// its rule matches, so the real probe gives the verdict the state stands for.
func TestRendersTheProbeStatesWithTheAnchorsRuleMatches(t *testing.T) {
	cases := []struct {
		name         string
		prepare      func(*screen)
		anchors      []string
		status, why  string
		noWorkingTag bool
	}{
		{"idle footer", func(*screen) {}, []string{"· / commands"}, "idle", "ready", true},
		{"working footer", func(s *screen) { s.add("❯ do it"); s.setMode(modeWorking, "") }, []string{"Working", "esc interrupt"}, "running", "working indicator", false},
		{"permission dialog", func(s *screen) { s.setMode(modePermission, "sleep 90") },
			[]string{"Do you want to ", "↑/↓ to navigate · enter to select · esc to cancel"}, "waiting", "permission prompt", true},
		{"question dialog", func(s *screen) { s.setMode(modeQuestion, "Which colour?") },
			[]string{"Copilot needs information."}, "waiting", "question", true},
		{"trust prompt", func(s *screen) { s.setMode(modeTrust, "") },
			[]string{"Confirm folder trust", "Do you trust the files in this folder?"}, "waiting", "folder trust", true},
		{"error line", func(s *screen) { s.add("❯ please fail", "✗ Failed to get response from the AI model") },
			[]string{"✗ Failed to get response"}, "error", "error line", true},
	}
	for _, tc := range cases {
		for _, cols := range []int{80, 120} {
			t.Run(tc.name+" at "+strconv.Itoa(cols), func(t *testing.T) {
				s := newScreen(cols, 30)
				tc.prepare(s)
				frame := s.frame(cols, 30)
				if len(frame) != 30 {
					t.Fatalf("frame has %d rows, want 30", len(frame))
				}
				for i, row := range frame {
					if utf8.RuneCountInString(row) > cols {
						t.Fatalf("row %d is %d columns wide, want at most %d: %q", i+1, utf8.RuneCountInString(row), cols, row)
					}
				}
				pane := strings.Join(frame, "\n")
				for _, anchor := range tc.anchors {
					if !strings.Contains(pane, anchor) {
						t.Errorf("pane lacks %q:\n%s", anchor, pane)
					}
				}
				status, reason := probeOf(pane)
				if status != tc.status || reason != tc.why {
					t.Errorf("probe = %q/%q, want %q/%q:\n%s", status, reason, tc.status, tc.why, pane)
				}
				if tc.noWorkingTag && strings.Contains(pane, "Working") {
					t.Errorf("a %s pane shows the Working footer:\n%s", tc.name, pane)
				}
			})
		}
	}
}

// The allow-all launch shows Allow All in the footer, as the real one does.
func TestFooterNamesTheApprovalMode(t *testing.T) {
	s := newScreen(120, 30)
	if got := strings.Join(s.frame(120, 30), "\n"); !strings.Contains(got, "Manual Approval") || strings.Contains(got, "Allow All") {
		t.Errorf("default footer:\n%s", got)
	}
	s.allowAll = true
	if got := strings.Join(s.frame(120, 30), "\n"); !strings.Contains(got, "Allow All") {
		t.Errorf("--allow-all footer lacks Allow All:\n%s", got)
	}
}

// A narrow pane drops the sidebar and help hints but keeps the idle anchor, and
// an overfull transcript keeps its newest lines.
func TestNarrowPanesKeepTheAnchorAndTheNewestTranscript(t *testing.T) {
	s := newScreen(60, 12)
	for i := 0; i < 40; i++ {
		s.add("line " + strconv.Itoa(i))
	}
	pane := strings.Join(s.frame(60, 12), "\n")
	if !strings.Contains(pane, "· / commands") || !strings.Contains(pane, "line 39") || strings.Contains(pane, "line 0\n") {
		t.Errorf("narrow pane:\n%s", pane)
	}
	// A pane shorter than the composer keeps the whole composer.
	if got := len(newScreen(60, 3).frame(60, 3)); got != 5 {
		t.Errorf("a 3-row pane has %d rows, want the 5 composer rows", got)
	}
}

func TestWrapTextAndBoxHandleDegenerateWidths(t *testing.T) {
	if got := wrapText("abcdef", 4); len(got) != 2 || got[0] != "abcd" || got[1] != "ef" {
		t.Errorf("wrapText = %q", got)
	}
	if got := wrapText("", 4); len(got) != 1 {
		t.Errorf("wrapText of an empty line = %q", got)
	}
	if got := box(2, []string{"abc"}); len(got) < 3 {
		t.Errorf("box at width 2 = %q", got)
	}
}

// R224 item 4 / R221: the startup output carries the whole sequence of a real
// copilot start, and each part is also in the captured real stream.
func TestStartupOutputHasTheRealStartSequence(t *testing.T) {
	h := newHarness(t)
	got := h.run([]string{"--session-id", testSessionID}, strings.NewReader(""), map[string]string{screenEnvironment: "1"})
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	capture, err := os.ReadFile(filepath.Join("..", "..", "internal", "agent", "testdata", "probes", "copilot", "stream.raw"))
	if err != nil {
		t.Fatal(err)
	}
	required := map[string]string{
		"OSC 10 query":     "\x1b]10;?\x1b\\",
		"OSC 11 query":     "\x1b]11;?\x1b\\",
		"OSC 4 query":      "\x1b]4;0;?\x1b\\",
		"CSI ? 996 n":      "\x1b[?996n",
		"CSI > q":          "\x1b[>q",
		"CSI ? u":          "\x1b[?u",
		"DECRQM 12":        "\x1b[?12$p",
		"DECRQM 1007":      "\x1b[?1007$p",
		"alt screen":       "\x1b[?1049h",
		"mouse 1003":       "\x1b[?1003h",
		"mouse 1006":       "\x1b[?1006h",
		"focus 1004":       "\x1b[?1004h",
		"BEL-terminated 0": "\x1b]0;GitHub Copilot\a",
	}
	for name, sequence := range required {
		if !strings.Contains(got.stdout, sequence) {
			t.Errorf("startup output lacks %s (%q)", name, sequence)
		}
		if !strings.Contains(string(capture), sequence) {
			t.Errorf("the real capture lacks %s (%q): the fixture would diverge from it", name, sequence)
		}
	}
	if strings.Contains(got.stdout, "\x1b]0;GitHub Copilot\x1b\\") {
		t.Error("the title is ST-terminated; Copilot terminates it with BEL")
	}
	if !strings.Contains(got.stdout, "\x1b[?1049l") || !strings.Contains(got.stdout, "--resume="+testSessionID) {
		t.Error("exit did not leave the alternate screen with the resume hint")
	}
}

// R224 item 4: the frame redraws on SIGWINCH at the size the kernel reports,
// and the size is logged.
func TestRedrawsAtTheNewSizeOnSigwinch(t *testing.T) {
	h := newHarness(t)
	deckHome := filepath.Join(t.TempDir(), "deck")
	cmd := exec.Command(os.Args[0], "--session-id", testSessionID) //nolint:gosec // G204: re-executes this test binary as the fake
	cmd.Dir = h.cwd
	cmd.Env = append(os.Environ(), runMainEnv+"=1", "COPILOT_HOME="+h.home, "DECK_HOME="+deckHome, commandsEnvironment+"=1", screenEnvironment+"=1")
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		t.Fatal(err)
	}
	var out lockedBuffer
	go func() { _, _ = io.Copy(&out, terminal) }()
	t.Cleanup(func() { _ = cmd.Process.Signal(syscall.SIGKILL); _ = cmd.Wait(); _ = terminal.Close() })

	waitFor(t, "the first frame", func() bool { return strings.Count(out.String(), "· / commands") >= 1 })
	if frame := lastFrame(out.String()); strings.Count(frame, "\n") != 23 {
		t.Fatalf("first frame has %d rows, want 24:\n%s", strings.Count(frame, "\n")+1, frame)
	}
	if err := pty.Setsize(terminal, &pty.Winsize{Rows: 30, Cols: 100}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "a frame at 100x30", func() bool {
		frame := lastFrame(out.String())
		return strings.Count(frame, "\n") == 29 && strings.Contains(frame, strings.Repeat("─", 100))
	})
	waitFor(t, "the sizes log", func() bool {
		data, _ := os.ReadFile(filepath.Join(deckHome, "log", sizesLogName))
		return string(data) == "80x24\n100x30\n"
	})
}

// R224 item 4: the SIGWINCH handler itself, on a screen whose size changes.
func TestSigwinchHandlerRedrawsWithTheCurrentSize(t *testing.T) {
	var out bytes.Buffer
	cols := 80
	var seen []int
	s := &screen{out: &out, size: func() (int, int) { return cols, 24 }, record: func(c, _ int) { seen = append(seen, c) }, cwd: "/tmp/work"}
	a := &app{screen: s}
	cols = 90
	if a.onSignal(syscall.SIGWINCH) {
		t.Fatal("SIGWINCH stopped the fixture")
	}
	if !strings.Contains(out.String(), strings.Repeat("─", 90)) || len(seen) != 1 || seen[0] != 90 {
		t.Errorf("redraw after SIGWINCH: sizes %v, output %q", seen, out.String())
	}
	(&app{}).onSignal(syscall.SIGWINCH) // plain mode: nothing to redraw
}

// R224 item 5: Ctrl-C (SIGINT) during work aborts the turn back to the idle
// footer and no hook runs for it.
func TestCtrlCDuringWorkAbortsTheTurnWithoutAHook(t *testing.T) {
	for _, how := range []string{"signal", "command"} {
		t.Run(how, func(t *testing.T) {
			h := newHarness(t)
			plugin := h.installPlugin(t)
			reader, writer := io.Pipe()
			signals := make(chan os.Signal, 1)
			var stdout lockedBuffer
			done := make(chan outcome, 1)
			go func() {
				var stderr bytes.Buffer
				code := runWithIO([]string{"--session-id", testSessionID, "--plugin-dir", plugin}, reader, &stdout, &stderr, h.getenv(screenEnv()), func() (string, error) { return h.cwd, nil }, signals)
				done <- outcome{code: code, stderr: stderr.String()}
			}()
			if _, err := writer.Write([]byte(`{"command":"prompt","text":"a long job","hold":true}` + "\n")); err != nil {
				t.Fatal(err)
			}
			waitFor(t, "the working footer", func() bool {
				status, _ := probeOf(lastFrame(stdout.String()))
				return status == "running" && equal(h.capturedEvents(t), []string{"userPromptSubmitted", "sessionStart"})
			})
			if how == "signal" {
				signals <- syscall.SIGINT
			} else if _, err := writer.Write([]byte(`{"command":"interrupt"}` + "\n")); err != nil {
				t.Fatal(err)
			}
			waitFor(t, "the idle footer", func() bool {
				status, reason := probeOf(lastFrame(stdout.String()))
				return status == "idle" && reason == "ready"
			})
			if frame := lastFrame(stdout.String()); !strings.Contains(frame, abortedLine) {
				t.Errorf("the aborted turn is not in the transcript:\n%s", frame)
			}
			if _, err := writer.Write([]byte(`{"command":"exit"}` + "\n")); err != nil {
				t.Fatal(err)
			}
			if got := <-done; got.code != 0 {
				t.Fatalf("exit %d: %s", got.code, got.stderr)
			}
			// Only the prompt's own hooks and the final sessionEnd ran: the
			// abort fired none, and agentStop never did.
			want := []string{"userPromptSubmitted", "sessionStart", "sessionEnd"}
			if events := h.capturedEvents(t); !equal(events, want) {
				t.Fatalf("hook events = %v, want %v", events, want)
			}
			types := eventTypes(t, filepath.Join(h.sessionDir(testSessionID), "events.jsonl"))
			if !strings.Contains(strings.Join(types, " "), "abort") {
				t.Errorf("events.jsonl %v lacks the abort", types)
			}
		})
	}
}

// With no turn open, Ctrl-C is the ordinary clean shutdown; outside the
// rendered mode an interrupt is only reported.
func TestInterruptWithoutATurnAndInPlainMode(t *testing.T) {
	h := newHarness(t)
	plugin := h.installPlugin(t)
	signals := make(chan os.Signal, 1)
	signals <- syscall.SIGINT
	var stdout, stderr bytes.Buffer
	code := runWithIO([]string{"--session-id", testSessionID, "--plugin-dir", plugin}, strings.NewReader(""), &stdout, &stderr, h.getenv(screenEnv()), func() (string, error) { return h.cwd, nil }, signals)
	_ = code
	if events := h.capturedEvents(t); !equal(events, []string{"sessionEnd"}) {
		t.Errorf("hook events = %v, want a clean shutdown", events)
	}

	plain := h.runCommands(t, "", `{"command":"prompt","text":"x","hold":true}`, `{"command":"interrupt"}`, `{"command":"interrupt"}`)
	if plain.code != 0 || strings.Count(plain.stdout, "fake-copilot turn aborted") != 1 {
		t.Errorf("plain-mode interrupt: exit %d, output %q", plain.code, plain.stdout)
	}
}

// R224 item 4: the dialogs and the trust prompt are driven by pane commands and
// startup configuration, and dismissed back to the right footer.
func TestDialogsTrustAndDismissal(t *testing.T) {
	h := newHarness(t)
	var out lockedBuffer
	reader, writer := io.Pipe()
	done := make(chan int, 1)
	env := screenEnv()
	env[trustEnvironment] = "1"
	go func() {
		done <- runWithIO([]string{"--session-id", testSessionID}, reader, &out, io.Discard, h.getenv(env), func() (string, error) { return h.cwd, nil }, nil)
	}()
	expect := func(status, reason string) {
		t.Helper()
		waitFor(t, status+"/"+reason, func() bool {
			s, r := probeOf(lastFrame(out.String()))
			return s == status && r == reason
		})
	}
	send := func(line string) {
		t.Helper()
		// A terminal's reply to a startup query may arrive ahead of the command.
		if _, err := writer.Write([]byte("\x1b]11;rgb:0000/0000/0000\x1b\\\x1b[?997;1n" + line + "\n\n")); err != nil {
			t.Fatal(err)
		}
	}
	expect("waiting", "folder trust")
	send(`{"command":"dismiss"}`)
	expect("idle", "ready")
	send(`{"command":"dialog","kind":"permission","message":"sleep 90"}`)
	expect("waiting", "permission prompt")
	send(`{"command":"dialog","kind":"question","message":"Which colour?"}`)
	expect("waiting", "question")
	send(`{"command":"prompt","text":"go","hold":true}`)
	send(`{"command":"dialog","kind":"trust"}`)
	expect("waiting", "folder trust")
	send(`{"command":"dismiss"}`)
	expect("running", "working indicator")
	send(`{"command":"error","message":"Could not connect"}`)
	expect("error", "error line")
	send(`{"command":"exit"}`)
	if code := <-done; code != 0 {
		t.Fatalf("exit %d", code)
	}
}

func TestDialogRejectsAnUnknownKindAndPlainModeReportsOne(t *testing.T) {
	h := newHarness(t)
	got := h.runCommands(t, "", `{"command":"dialog","kind":"nonsense"}`)
	if got.code != 2 || !strings.Contains(got.stderr, `unknown dialog kind "nonsense"`) {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
	got = h.runCommands(t, "", `{"command":"dialog","kind":"permission"}`, `{"command":"dismiss"}`)
	if got.code != 0 || !strings.Contains(got.stdout, "fake-copilot dialog: permission") {
		t.Errorf("exit %d, output %q", got.code, got.stdout)
	}
}

func TestStripTerminalReplies(t *testing.T) {
	for in, want := range map[string]string{
		"\x1b]10;rgb:ffff/ffff/ffff\a{}":    "{}",
		"\x1b[?12;2$y\x1b[?1007;2$y {} ":    "{}",
		"\x1b]4;1;rgb:1/2/3\x1b\\":          "",
		"\x1bP>|tmux 3.5a\x1b\\{}\x1b[I":    "{}",
		"plain {\"command\":\"exit\"} text": "plain {\"command\":\"exit\"} text",
	} {
		if got := stripTerminalReplies(in); got != want {
			t.Errorf("stripTerminalReplies(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTerminalSizeFallsBackOffATerminal(t *testing.T) {
	if cols, rows := terminalSize(io.Discard)(); cols != 80 || rows != 24 {
		t.Errorf("size = %dx%d, want 80x24", cols, rows)
	}
	if sizeRecorder(func(string) string { return "" }) != nil {
		t.Error("a recorder exists without DECK_HOME")
	}
	disableStdinEcho(strings.NewReader("")) // not a file: nothing to do
	file, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	disableStdinEcho(file) // a file that is not a tty: nothing to do
}

func TestSizeRecorderAppendsOneLinePerObservation(t *testing.T) {
	home := t.TempDir()
	record := sizeRecorder(func(key string) string {
		if key == "DECK_HOME" {
			return home
		}
		return ""
	})
	if record == nil {
		t.Fatal("no recorder with DECK_HOME set")
	}
	record(80, 24)
	record(100, 30)
	data, err := os.ReadFile(filepath.Join(home, "log", sizesLogName))
	if err != nil || string(data) != "80x24\n100x30\n" {
		t.Errorf("sizes log = %q, %v", data, err)
	}
	// An unwritable location is dropped, not a fixture failure.
	appendSizeLine(filepath.Join(home, "log", sizesLogName, "nested", "file"), 1, 1)
}

func TestDialogsDefaultTheirDetailLine(t *testing.T) {
	s := newScreen(100, 30)
	s.setMode(modePermission, "")
	if pane := strings.Join(s.frame(100, 30), "\n"); !strings.Contains(pane, "Shell command") {
		t.Errorf("permission dialog without a command:\n%s", pane)
	}
	s.setMode(modeQuestion, "")
	if pane := strings.Join(s.frame(100, 30), "\n"); !strings.Contains(pane, "Which colour do you prefer?") {
		t.Errorf("question dialog without a question:\n%s", pane)
	}
	s.partial = "half a line"
	if pane := strings.Join(s.frame(100, 30), "\n"); !strings.Contains(pane, "half a line") {
		t.Errorf("an unfinished progress line is not drawn:\n%s", pane)
	}
}
