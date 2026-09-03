package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readNonEmptyLines reads path (tolerating "does not exist yet") and
// returns its non-empty lines -- used below to prove a hook ran, and ran
// exactly once, from the file it wrote rather than from a mock call count.
func readNonEmptyLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// TestArchiveRunsSessionThenGlobalPostDestroyExactlyOnce proves task 014's
// half of SPEC \u00a79.2 for `A`: both the session's own post_destroy and the
// service's GlobalPostDestroy run, each exactly once, in session-then-
// global order, and each sees DECK_TEARDOWN_KIND=archive and this
// session's own DECK_SESSION_ID -- all observed through files the two hook
// scripts write themselves, never a mock call count.
//
// Ordering is enforced structurally, not just observed: the global hook's
// shell line only writes its own file when the session hook's file
// already exists, so a session-and-global-swapped bug would leave the
// global file missing rather than merely reordered in a timestamp.
func TestArchiveRunsSessionThenGlobalPostDestroyExactlyOnce(t *testing.T) {
	svc := newArchiveTestService(t)
	dir := t.TempDir()
	sessionFile := filepath.Join(dir, "session-hook.out")
	globalFile := filepath.Join(dir, "global-hook.out")
	svc.GlobalPostDestroy = "test -f " + sessionFile + " && echo \"$DECK_TEARDOWN_KIND:$DECK_SESSION_ID\" >> " + globalFile

	session, err := svc.CreateShell(context.Background(), ShellCreateInput{
		Name: "archive-hook-order", CWD: t.TempDir(),
		PostDestroy: "echo \"$DECK_TEARDOWN_KIND:$DECK_SESSION_ID\" >> " + sessionFile,
	})
	if err != nil {
		t.Fatalf("create shell: %v", err)
	}
	if err := svc.Kill(context.Background(), session); err != nil {
		t.Fatalf("stop the session first: %v", err)
	}
	stopped, err := svc.Store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}

	if msg, err := svc.Archive(context.Background(), stopped); err != nil || msg != "" {
		t.Fatalf("archive: msg=%q err=%v, want empty message and no error (both hooks succeed)", msg, err)
	}

	want := []string{"archive:" + session.ID}
	if got := readNonEmptyLines(t, sessionFile); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("session post_destroy file lines = %#v, want exactly %#v (ran once, correct env)", got, want)
	}
	if got := readNonEmptyLines(t, globalFile); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("global post_destroy file lines = %#v, want exactly %#v (ran once after the session hook, correct env)", got, want)
	}
}

// TestDeleteRunsSessionThenGlobalPostDestroyExactlyOnce is the `dd`
// counterpart of the Archive test above: same session-then-global,
// exactly-once, DECK_TEARDOWN_KIND/DECK_SESSION_ID contract, this time
// with DECK_TEARDOWN_KIND=delete and Delete's own live-pane-kill-then-
// tombstone path ahead of the hooks.
func TestDeleteRunsSessionThenGlobalPostDestroyExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	sessionFile := filepath.Join(dir, "session-hook.out")
	globalFile := filepath.Join(dir, "global-hook.out")

	svc := newTombstoneTestService(t)
	svc.GlobalPostDestroy = "test -f " + sessionFile + " && echo \"$DECK_TEARDOWN_KIND:$DECK_SESSION_ID\" >> " + globalFile

	session, err := svc.CreateShell(context.Background(), ShellCreateInput{
		Name: "delete-hook-order", CWD: t.TempDir(),
		PostDestroy: "echo \"$DECK_TEARDOWN_KIND:$DECK_SESSION_ID\" >> " + sessionFile,
	})
	if err != nil {
		t.Fatalf("create shell: %v", err)
	}

	if msg, err := svc.Delete(context.Background(), session); err != nil || msg != "" {
		t.Fatalf("delete: msg=%q err=%v, want empty message and no error (both hooks succeed)", msg, err)
	}

	want := []string{"delete:" + session.ID}
	if got := readNonEmptyLines(t, sessionFile); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("session post_destroy file lines = %#v, want exactly %#v (ran once, correct env)", got, want)
	}
	if got := readNonEmptyLines(t, globalFile); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("global post_destroy file lines = %#v, want exactly %#v (ran once after the session hook, correct env)", got, want)
	}
}
