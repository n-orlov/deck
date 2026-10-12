package main

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
)

// A dialog command draws Pi's confirm selector, which deck's Pi adapter reads
// as a focused dialog, and the next typed line answers it: the keystroke
// rename deck would have sent renames nothing and fires no hook. Once
// answered, a typed /name reaches the editor again.
func TestPiDialogSwallowsTheNextTypedLine(t *testing.T) {
	getenv, getwd, _ := namesEnvironment(t)
	out := runFake(t, getenv, getwd,
		`{"command":"start"}`+"\n"+`{"command":"dialog","name":"Allow bash?"}`+"\n"+"\x15/name swallowed\n"+"/name after\n",
		"--name", "launch")
	if !(agent.Pi{}).DialogFocused(piDialogScreen("Allow bash?")) {
		t.Fatalf("the fake's dialog screen is not a focused dialog to deck:\n%s", piDialogScreen("Allow bash?"))
	}
	if !strings.Contains(out, " Allow bash?\n") || !strings.Contains(out, "escape/ctrl+c cancel") {
		t.Errorf("pane lacks the dialog: %q", out)
	}
	if !strings.Contains(out, "fake-pi dialog answered: /name swallowed\n") {
		t.Errorf("pane lacks the swallowed line: %q", out)
	}
	var names []any
	for _, e := range sessionEntries(t, getenv) {
		if e["type"] == "session_info" {
			names = append(names, e["name"])
		}
	}
	if want := []any{"launch", "after"}; !equalAny(names, want) {
		t.Errorf("session_info names = %v, want %v (the dialog's line must rename nothing)", names, want)
	}
}

// JSON commands still run while a dialog is open: only typed lines answer it.
func TestPiDialogLetsJSONCommandsThrough(t *testing.T) {
	getenv, getwd, _ := namesEnvironment(t)
	out := runFake(t, getenv, getwd,
		`{"command":"dialog","name":"Pick"}`+"\n"+`{"command":"name","name":"via command"}`+"\n")
	if strings.Contains(out, "fake-pi dialog answered") {
		t.Errorf("a JSON command answered the dialog: %q", out)
	}
	if !strings.Contains(out, "fake-pi hook fired: session_info_changed\n") {
		t.Errorf("the name command did not run under the dialog: %q", out)
	}
}

func TestPrintableRuneDropsControlCharacters(t *testing.T) {
	if got := strings.Map(printableRune, "\x15/name x\x7f"); got != "/name x" {
		t.Errorf("got %q", got)
	}
}
