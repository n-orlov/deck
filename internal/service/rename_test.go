package service

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// TestRenameChangesNameLeavesSlugAndTmuxSessionUntouched proves task 013's
// service half end to end (SPEC §11.4, PRD requirement 31, I-8): a rename
// changes the store's `name` column and nothing else -- slug (the tmux
// identity deck_<slug>) is unchanged, and the live tmux session created
// under the ORIGINAL slug is still there, under that same name, after the
// rename.
func TestRenameChangesNameLeavesSlugAndTmuxSessionUntouched(t *testing.T) {
	svc := newArchiveTestService(t)
	cwd := t.TempDir()
	session, err := svc.CreateShell(context.Background(), ShellCreateInput{Name: "before-rename", CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	originalSlug := session.Slug

	live, err := svc.TMux.Exists(context.Background(), originalSlug)
	if err != nil {
		t.Fatal(err)
	}
	if !live {
		t.Fatalf("tmux session %q was not created by CreateShell", originalSlug)
	}

	renamed, err := svc.Rename(context.Background(), session.ID, "after-rename")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if renamed.Name != "after-rename" {
		t.Fatalf("name = %q, want %q", renamed.Name, "after-rename")
	}
	if renamed.Slug != originalSlug {
		t.Fatalf("slug changed from %q to %q; rename must never touch slug", originalSlug, renamed.Slug)
	}

	stillLive, err := svc.TMux.Exists(context.Background(), originalSlug)
	if err != nil {
		t.Fatal(err)
	}
	if !stillLive {
		t.Fatal("the live tmux session under the original slug is gone after a rename")
	}
	newSlugLive, err := svc.TMux.Exists(context.Background(), store.Slug("after-rename"))
	if err != nil {
		t.Fatal(err)
	}
	if newSlugLive {
		t.Fatal("a rename created a second tmux session under a new slug; it must not")
	}
}

// TestRenameRejectsBlankNameAndUnknownSession proves the guard clauses: a
// blank (or blank-after-trim) new name is refused, and so is an unknown
// session id, before any store mutation.
func TestRenameRejectsBlankNameAndUnknownSession(t *testing.T) {
	svc := newArchiveTestService(t)
	cwd := t.TempDir()
	session, err := svc.CreateShell(context.Background(), ShellCreateInput{Name: "guarded", CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rename(context.Background(), session.ID, "   "); err == nil {
		t.Fatal("blank new name was accepted")
	}
	if _, err := svc.Rename(context.Background(), "not-a-real-id", "new-name"); err == nil {
		t.Fatal("unknown session id was accepted")
	}
	unchanged, err := svc.Store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Name != "guarded" {
		t.Fatalf("name = %q, want unchanged %q", unchanged.Name, "guarded")
	}
}
