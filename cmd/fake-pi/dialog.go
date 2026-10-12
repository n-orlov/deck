package main

import (
	"bytes"
	"io"
	"strings"
)

// A dialog plays a Pi extension's ui.confirm (extension-selector.js in pi
// 0.84): the selector takes the editor's place, drawn between two borders with
// its title, the options and the key-hint line, and the next typed line
// answers it instead of reaching the editor. deck reads that hint line to
// refuse a keystroke rename while a dialog is up (SPEC §8.3); the fake records
// any line the dialog swallowed so a scenario can show nothing was typed.

const piDialogBorder = "────────────────────────────────────────"

// piDialogScreen is what the fake draws for a confirm dialog titled title.
func piDialogScreen(title string) string {
	return piDialogBorder + "\n\n " + title + "\n\n → Yes\n   No\n\n ↑↓ navigate  enter select  escape/ctrl+c cancel\n\n" + piDialogBorder + "\n"
}

// openDialog draws a confirm dialog and holds it open until a line answers it.
func (h extensionHooks) openDialog(out io.Writer, title string) error {
	if h.session != nil {
		h.session.mu.Lock()
		h.session.dialog = true
		h.session.mu.Unlock()
	}
	say(out, piDialogScreen(title))
	return nil
}

// dialogOpen reports whether a dialog is waiting for its answer.
func (h extensionHooks) dialogOpen() bool {
	if h.session == nil {
		return false
	}
	h.session.mu.Lock()
	defer h.session.mu.Unlock()
	return h.session.dialog
}

// answerDialog closes the dialog with the typed line as its answer; the line
// never reaches the editor, so a typed /name renames nothing.
func (h extensionHooks) answerDialog(out io.Writer, line string) error {
	if h.session != nil {
		h.session.mu.Lock()
		h.session.dialog = false
		h.session.mu.Unlock()
	}
	sayf(out, "fake-pi dialog answered: %s\n", strings.Map(printableRune, line))
	return nil
}

// printableRune drops control characters (the fallback's Ctrl-U) from an echo.
func printableRune(r rune) rune {
	if r < 0x20 || r == 0x7f {
		return -1
	}
	return r
}

// commandFor turns one pane line into a command: while a dialog is open any
// line that is not a JSON command answers it, otherwise a typed /name is Pi's
// built-in rename and anything else must be a JSON command.
func commandFor(line []byte, hooks extensionHooks) (fixtureCommand, error) {
	if hooks.dialogOpen() && !bytes.HasPrefix(bytes.TrimSpace(line), []byte("{")) {
		return fixtureCommand{Command: "answer", Name: string(line)}, nil
	}
	if name, typed := typedName(line); typed {
		return fixtureCommand{Command: "name", Name: name}, nil
	}
	var request fixtureCommand
	err := decodeCommand(line, &request)
	return request, err
}
