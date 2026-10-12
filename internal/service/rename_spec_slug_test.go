package service

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestRepeatedRenamesKeepFirstSlugAndLiveTmuxSession pins the §3.2 rule beyond
// the single-rename case: the slug fixed at create survives a chain of renames,
// including one that would derive a different slug, and the original tmux
// session stays the only one.
func TestRepeatedRenamesKeepFirstSlugAndLiveTmuxSession(t *testing.T) {
	svc := newArchiveTestService(t)
	ctx := context.Background()
	row, err := svc.CreateShell(ctx, ShellCreateInput{Name: "first-name", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Second Name", "third_name", "Fourth-Name"} {
		got, err := svc.Rename(ctx, row.ID, name)
		if err != nil {
			t.Fatalf("rename %q: %v", name, err)
		}
		if got.Name != name || got.Slug != row.Slug {
			t.Fatalf("after rename %q: name=%q slug=%q, want slug %q", name, got.Name, got.Slug, row.Slug)
		}
		live, err := svc.TMux.Exists(ctx, row.Slug)
		if err != nil || !live {
			t.Fatalf("after rename %q: original tmux session live=%v err=%v", name, live, err)
		}
	}
}

// TestRenameOfTwoRowsLeavesEachRowsOwnSession proves a rename touches only its
// own row's name: a sibling's slug and live session are untouched too.
func TestRenameOfTwoRowsLeavesEachRowsOwnSession(t *testing.T) {
	svc := newArchiveTestService(t)
	ctx := context.Background()
	a, err := svc.CreateShell(ctx, ShellCreateInput{Name: "row-a", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.CreateShell(ctx, ShellCreateInput{Name: "row-b", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rename(ctx, a.ID, "row-b-renamed"); err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{a.Slug, b.Slug} {
		live, err := svc.TMux.Exists(ctx, slug)
		if err != nil || !live {
			t.Fatalf("tmux session deck_%s live=%v err=%v", slug, live, err)
		}
	}
	gotB, err := svc.Store.GetSession(ctx, b.ID)
	if err != nil || gotB.Name != "row-b" || gotB.Slug != b.Slug {
		t.Fatalf("sibling row changed: %+v err=%v", gotB, err)
	}
}

// TestSpecNamingSectionDoesNotClaimRenameMovesTmux guards the §3.2 wording: the
// naming bullet states that rename and harness adoption change the display name
// only and must not say a rename renames the slug or tmux session.
func TestSpecNamingSectionDoesNotClaimRenameMovesTmux(t *testing.T) {
	data, err := os.ReadFile("../../SPEC.md")
	if err != nil {
		t.Fatal(err)
	}
	spec := string(data)
	if strings.Contains(spec, "Rename renames both") {
		t.Fatal("SPEC still says \"Rename renames both\"; rename changes the display name alone")
	}
	i := strings.Index(spec, "- Session naming: `deck_<slug>`")
	if i < 0 {
		t.Fatal("SPEC §3.2 session-naming bullet missing")
	}
	bullet := spec[i : i+900]
	for _, want := range []string{"fixed when the session is created", "adopted from the harness", "display", "name (`sessions.name`) alone", "keep"} {
		if !strings.Contains(bullet, want) {
			t.Errorf("SPEC §3.2 naming bullet lacks %q", want)
		}
	}
}
