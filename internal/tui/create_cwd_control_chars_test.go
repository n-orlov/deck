package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// R197 (#61): a directory name is attacker-controlled bytes. An ESC or other
// control byte in one must never reach the terminal through the cwd field's
// ghost, its tab completion or the candidate list.

const hostileDirName = "evil\x1b[2Jdir\x07"

func hostileDirParent(t *testing.T, names ...string) string {
	t.Helper()
	parent := createEditShortDir(t)
	for _, n := range names {
		if err := os.Mkdir(filepath.Join(parent, n), 0o755); err != nil {
			t.Skipf("filesystem refuses control bytes in a directory name: %v", err)
		}
	}
	return parent
}

func assertNoControlBytes(t *testing.T, what, s string) {
	t.Helper()
	// The renderer's own styling is not under test here: its SGR sequences are
	// the only ESC bytes allowed, and the hostile name's "[2J" never follows one.
	for _, bad := range []string{"\x1b[2J", "\x07"} {
		if strings.Contains(s, bad) {
			t.Fatalf("%s draws the directory name's control bytes (%q found): %q", what, bad, s)
		}
	}
}

func TestCreateCWDGhostDrawsADirectoryNameWithControlBytesStripped(t *testing.T) {
	parent := hostileDirParent(t, hostileDirName)
	m := createEditOpen(t, t.TempDir())
	m.createField = createFieldCWD
	m = createEditType(t, m, filepath.Join(parent, "ev"))

	if got, want := m.createCWDGhostSuffix(), "il[2Jdir/"; got != want {
		t.Fatalf("ghost = %q, want the name's control bytes dropped: %q", got, want)
	}
	assertNoControlBytes(t, "the ghost row", createEditRow(t, m, "Working directory"))
	assertNoControlBytes(t, "the create view", m.createView())

	accepted := createEditPress(t, m, "right")
	if got := accepted.createText(createFieldCWD); strings.ContainsAny(got, "\x1b\x07") {
		t.Fatalf("accepting the ghost put control bytes in the field: %q", got)
	}
}

func TestCreateCWDCandidateListDrawsDirectoryNamesWithControlBytesStripped(t *testing.T) {
	parent := hostileDirParent(t, hostileDirName, "evil-other")
	m := createEditOpen(t, t.TempDir())
	m.createField = createFieldCWD
	m = createEditType(t, m, filepath.Join(parent, "ev"))
	m = createEditPress(t, m, "tab") // common prefix "evil" is already typed past "ev": completes it
	m = createEditPress(t, m, "tab") // nothing left to advance: lists the candidates
	if len(m.createCWDCandidates) != 2 {
		t.Fatalf("candidates = %q, want both directories listed", m.createCWDCandidates)
	}
	for _, c := range m.createCWDCandidates {
		if strings.ContainsAny(c, "\x1b\x07") {
			t.Fatalf("candidate %q keeps its control bytes", c)
		}
	}
	assertNoControlBytes(t, "the candidate list", m.createView())
}

func TestCreateCWDTabCompletionAppendsNoControlBytes(t *testing.T) {
	parent := hostileDirParent(t, hostileDirName)
	m := createEditOpen(t, t.TempDir())
	m.createField = createFieldCWD
	m = createEditType(t, m, filepath.Join(parent, "ev"))
	m = createEditPress(t, m, "tab")
	if got := m.createText(createFieldCWD); strings.ContainsAny(got, "\x1b\x07") {
		t.Fatalf("tab completion put control bytes in the field: %q", got)
	}
	assertNoControlBytes(t, "the create view", m.createView())
}

func TestCreatePrefilledValuesDrawWithControlBytesStripped(t *testing.T) {
	m := createEditOpen(t, t.TempDir())
	for _, field := range []int{createFieldName, createFieldCWD, createFieldLaunchArgs, createFieldEnv, createFieldPreLaunch, createFieldPostDestroy} {
		m.setCreateText(field, "pre\x1b[2Jfill\x07")
	}
	for _, field := range []int{createFieldName, createFieldCWD, createFieldLaunchArgs, createFieldEnv, createFieldPreLaunch, createFieldPostDestroy} {
		if got := m.createText(field); strings.ContainsAny(got, "\x1b\x07") {
			t.Fatalf("field %d holds control bytes: %q", field, got)
		}
	}
	assertNoControlBytes(t, "the create view", m.createView())
}
