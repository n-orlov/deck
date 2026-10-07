package main

import (
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The screen mode (FAKE_COPILOT_SCREEN=1) makes the fixture look like Copilot's
// full-screen TUI: the startup control sequence of a real 1.0.93 start, an
// alternate-screen frame with the same footer, dialog and error text the probe
// fixtures in internal/agent/testdata/probes/copilot contain, and a redraw at
// the new size on SIGWINCH.

type screenMode int

const (
	modeIdle screenMode = iota
	modeWorking
	modePermission
	modeQuestion
	modeTrust
)

const (
	esc = "\x1b"
	// idleFooterFull and idleFooterShort both carry the "· / commands" anchor
	// R219's idle rule matches; the short one is for panes too narrow for the
	// sidebar and help hints.
	idleFooterFull  = " ← open sidebar · Interactive · %s · / commands · ? help · tab next tab"
	idleFooterShort = " Interactive · %s · / commands"
	workingFooter   = " ◉ Working · 130 B esc interrupt"
	// Reply and aborted-turn markers, as the real transcript draws them.
	replyMarker = "● "
	errorMarker = "✗ "
	promptMark  = "❯ "
	abortedLine = "Operation cancelled by user"
)

// startupSequence is what a real copilot 1.0.93 writes before its first frame
// (see stream.raw in the probe fixtures): keyboard-protocol and mode setup,
// alt screen 1049, bracketed paste, focus 1004, mouse 1003 + 1006, the DECRQM
// queries for modes 12 and 1007, the OSC 10/11/4 colour queries, a BEL-
// terminated OSC 0 title and the 996 / CSI > q / CSI ? u terminal queries.
func startupSequence() string {
	var b strings.Builder
	b.WriteString(esc + "[>4;2m" + esc + "[?u")
	b.WriteString(esc + "[?1049h" + esc + "[?2004h" + esc + "[?1004h" + esc + "[?1003h" + esc + "[?1006h")
	b.WriteString(esc + "[?25l" + esc + "[?12$p" + esc + "[?1007$p")
	b.WriteString(esc + "[H" + esc + "[2J")
	b.WriteString(esc + "]10;?" + esc + "\\" + esc + "]11;?" + esc + "\\")
	for colour := 0; colour < 16; colour++ {
		b.WriteString(esc + "]4;" + strconv.Itoa(colour) + ";?" + esc + "\\")
	}
	b.WriteString(esc + "[22;0t" + titleSequence("GitHub Copilot"))
	b.WriteString(esc + "[?996n" + esc + "[>q")
	return b.String()
}

// shutdownSequence undoes the modes startupSequence set, leaves the alternate
// screen and prints the resume hint Copilot leaves on the normal screen.
func shutdownSequence(sessionID string) string {
	return esc + "]111;\a" + esc + "]110;\a" + esc + "[?25h" + esc + "[23;0t" +
		esc + "[?1006l" + esc + "[?1003l" + esc + "[?1004l" + esc + "[?2004l" + esc + "[>4;0m" + esc + "[?1049l" +
		"\r\n  Resume     copilot --resume=" + sessionID + "\r\n"
}

// titleSequence is an OSC 0 window title, BEL-terminated as Copilot writes it
// (not ST-terminated).
func titleSequence(title string) string { return esc + "]0;" + title + "\a" }

// screen is the fixture's terminal: the transcript lines, what the composer
// shows and the writer the frames go to.
type screen struct {
	out       io.Writer
	size      func() (cols, rows int)
	record    func(cols, rows int)
	cwd       string
	allowAll  bool
	mode      screenMode
	dialog    string
	lines     []string
	partial   string
	sessionID string
	closed    bool
}

// start enters the TUI: the startup sequence, then the first frame.
func (s *screen) start() {
	say(s.out, startupSequence())
	s.resized()
}

// resized is the terminal's size changing (or being first read): it is logged,
// when a log is configured, and the frame redrawn at the new size.
func (s *screen) resized() {
	if s.record != nil {
		s.record(s.size())
	}
	s.redraw()
}

// stop leaves the TUI. Later writes draw nothing.
func (s *screen) stop() {
	if s.closed {
		return
	}
	s.closed = true
	say(s.out, shutdownSequence(s.sessionID))
}

// Write makes the screen an io.Writer for the fixture's own progress lines:
// complete lines join the transcript and the frame is redrawn.
func (s *screen) Write(p []byte) (int, error) {
	text := s.partial + string(p)
	parts := strings.Split(text, "\n")
	s.partial = parts[len(parts)-1]
	for _, line := range parts[:len(parts)-1] {
		s.lines = append(s.lines, strings.TrimRight(line, "\r"))
	}
	s.redraw()
	return len(p), nil
}

// add appends transcript lines and redraws.
func (s *screen) add(lines ...string) {
	s.lines = append(s.lines, lines...)
	s.redraw()
}

// setMode changes what the composer shows; dialog is the dialog's detail line
// (the command a permission dialog asks about, the question a question dialog
// asks).
func (s *screen) setMode(mode screenMode, dialog string) {
	s.mode, s.dialog = mode, dialog
	s.redraw()
}

// redraw writes one full frame at the current size, clearing the screen first
// so a resize never leaves stale cells.
func (s *screen) redraw() {
	if s.closed {
		return
	}
	cols, rows := s.size()
	frame := s.frame(cols, rows)
	var b strings.Builder
	b.WriteString(esc + "[?25l" + esc + "[H" + esc + "[2J")
	for i, line := range frame {
		b.WriteString(esc + "[" + strconv.Itoa(i+1) + ";1H" + line)
	}
	b.WriteString(esc + "[?25h")
	say(s.out, b.String())
}

// frame lays the screen out as exactly rows lines: the transcript, top
// anchored, then the composer (or a dialog) at the bottom.
func (s *screen) frame(cols, rows int) []string {
	bottom := s.bottom(cols)
	room := rows - len(bottom)
	if room < 0 {
		room = 0
	}
	var wrapped []string
	for _, line := range s.transcriptLines() {
		wrapped = append(wrapped, wrapText(line, cols)...)
	}
	if len(wrapped) > room {
		wrapped = wrapped[len(wrapped)-room:]
	}
	out := make([]string, 0, rows)
	out = append(out, wrapped...)
	for len(out) < room {
		out = append(out, "")
	}
	return append(out, bottom...)
}

func (s *screen) transcriptLines() []string {
	lines := s.lines
	if s.partial != "" {
		lines = append(append([]string(nil), lines...), s.partial)
	}
	return lines
}

// bottom is the composer, or the dialog that replaces it.
func (s *screen) bottom(cols int) []string {
	switch s.mode {
	case modePermission:
		return box(cols, permissionDialog(s.dialog))
	case modeQuestion:
		return box(cols, questionDialog(s.dialog))
	case modeTrust:
		return box(cols, trustDialog(s.cwd))
	}
	rule := strings.Repeat("─", cols)
	return []string{" " + s.cwd, rule, promptMark, rule, s.footer(cols)}
}

func (s *screen) footer(cols int) string {
	if s.mode == modeWorking {
		return workingFooter
	}
	label := "Manual Approval"
	if s.allowAll {
		label = "Allow All"
	}
	full := strings.Replace(idleFooterFull, "%s", label, 1)
	if utf8.RuneCountInString(full) <= cols {
		return full
	}
	return strings.Replace(idleFooterShort, "%s", label, 1)
}

func permissionDialog(command string) []string {
	if command == "" {
		command = "Shell command"
	}
	return []string{
		command, "",
		"Do you want to run this command?", "",
		promptMark + "1. Yes",
		"  2. Yes, and don't ask again for this command in this directory",
		"  3. No, and tell Copilot what to do differently (Esc to stop)", "",
		"↑/↓ to navigate · enter to select · esc to cancel",
	}
}

func questionDialog(question string) []string {
	if question == "" {
		question = "Which colour do you prefer?"
	}
	return []string{
		"Copilot needs information.", question, "",
		promptMark + "red", "  blue", "  Other (type your answer)", "",
		"↑/↓ select · enter accept · ctrl+d decline · esc cancel",
	}
}

func trustDialog(cwd string) []string {
	return []string{
		"Confirm folder trust", "", cwd, "",
		"Copilot can read files in this folder and, with your permission, edit them or run code and shell commands. It will remember your permissions for the rest of this session.", "",
		"Do you trust the files in this folder?", "",
		promptMark + "1. Yes",
		"  2. Yes, and remember this folder for future sessions",
		"  3. No (Esc)", "",
		"↑/↓ to navigate · enter to select · esc to cancel",
	}
}

// box frames content lines in a rounded border cols wide, wrapping long lines.
func box(cols int, content []string) []string {
	inner := cols - 4
	if inner < 1 {
		inner = 1
	}
	out := []string{"╭" + strings.Repeat("─", maxInt(cols-2, 0)) + "╮"}
	for _, line := range content {
		for _, part := range wrapText(line, inner) {
			pad := inner - utf8.RuneCountInString(part)
			out = append(out, "│ "+part+strings.Repeat(" ", maxInt(pad, 0))+" │")
		}
	}
	return append(out, "╰"+strings.Repeat("─", maxInt(cols-2, 0))+"╯")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// wrapText splits line into chunks of at most width runes; an empty line stays
// one empty chunk.
func wrapText(line string, width int) []string {
	runes := []rune(line)
	if width < 1 || len(runes) <= width {
		return []string{line}
	}
	var out []string
	for len(runes) > width {
		out = append(out, string(runes[:width]))
		runes = runes[width:]
	}
	return append(out, string(runes))
}

// terminalReply matches what a terminal answers the startup queries with: CSI
// strings (mode reports, focus and mouse events), OSC strings (colours) and DCS
// strings (the XTVERSION answer to CSI > q). It arrives on stdin ahead of, or
// inside, a command line.
var terminalReply = regexp.MustCompile("\x1b(\\[[0-9;?<>=$ ]*[A-Za-z~]|[\\]P][^\a\x1b]*(\a|\x1b\\\\))")

// stripTerminalReplies removes terminal query replies from a command line.
func stripTerminalReplies(line string) string {
	return strings.TrimSpace(terminalReply.ReplaceAllString(line, ""))
}
