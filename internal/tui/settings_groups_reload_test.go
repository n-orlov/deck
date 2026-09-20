package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// TestSettingsGroupsPanelRefreshesOnOrdinaryReload is task 002's own
// regression for finding B2: m.settingsGroups used to be recomputed only
// by the Groups panel's OWN key paths (create/rename/delete commits --
// settings.go:764,832,966,975), never by the ordinary periodic/on-demand
// sessionsLoaded reload every client also runs. So a create/rename/delete
// applied from a SECOND deck client sharing the same state.db (the exact
// cross-process shape group_shared_state_db_test.go already proves for
// ListGroups/ListSessions) landed on disk immediately, but an open Groups
// panel on this client kept rendering its stale snapshot until the
// operator happened to press n/r/d themselves.
//
// Two independently-opened *store.Store handles on the SAME on-disk
// state.db stand in for two live deck processes: handle A applies the
// edits, handle B has the Groups panel open and only ever runs its own
// ordinary loadSessions -> Update(sessionsLoaded) reload -- never a bare
// store call -- so this also proves the render path
// (settingsGroupsViewLines), not just the m.settingsGroups slice.
func TestSettingsGroupsPanelRefreshesOnOrdinaryReload(t *testing.T) {
	home := t.TempDir()
	dbPath := filepath.Join(home, "state.db")

	clientA, err := store.OpenPath(home, dbPath)
	if err != nil {
		t.Fatalf("client A OpenPath: %v", err)
	}
	t.Cleanup(func() { clientA.Close() })

	clientB, err := store.OpenPath(home, dbPath)
	if err != nil {
		t.Fatalf("client B OpenPath: %v", err)
	}
	t.Cleanup(func() { clientB.Close() })

	ctx := context.Background()

	// Two groups exist before B ever opens its Groups panel: one that A
	// will rename, one that A will delete.
	toRename, err := clientA.CreateGroup(ctx, "to-rename")
	if err != nil {
		t.Fatalf("client A CreateGroup(to-rename): %v", err)
	}
	toDelete, err := clientA.CreateGroup(ctx, "to-delete")
	if err != nil {
		t.Fatalf("client A CreateGroup(to-delete): %v", err)
	}

	mb := settingsOpenOnGroupsFields(t, clientB)
	mb.width, mb.height = 100, 40

	// Sanity: B's panel opened on the pre-edit snapshot.
	view := mb.View()
	if !strings.Contains(view, "to-rename") || !strings.Contains(view, "to-delete") {
		t.Fatalf("B's Groups panel before any edit = %q, want both pre-existing groups named", view)
	}

	// --- A applies a create, a rename and a delete, all through handle A ---
	if _, err := clientA.CreateGroup(ctx, "brand-new"); err != nil {
		t.Fatalf("client A CreateGroup(brand-new): %v", err)
	}
	if err := clientA.RenameGroup(ctx, toRename.ID, "renamed-done"); err != nil {
		t.Fatalf("client A RenameGroup: %v", err)
	}
	if err := clientA.DeleteGroup(ctx, toDelete.ID); err != nil {
		t.Fatalf("client A DeleteGroup: %v", err)
	}

	// --- B's ordinary reload: loadSessions -> Update, nothing else ---
	msg := mb.loadSessions()
	loaded, ok := msg.(sessionsLoaded)
	if !ok {
		t.Fatalf("client B's loadSessions() = %T, want sessionsLoaded", msg)
	}
	if loaded.err != nil || loaded.groupsErr != nil {
		t.Fatalf("client B's loadSessions() reported err=%v groupsErr=%v", loaded.err, loaded.groupsErr)
	}

	next, _ := mb.Update(loaded)
	mb = next.(Model)

	view = mb.View()
	if !strings.Contains(view, "brand-new") {
		t.Fatalf("B's Groups panel after its ordinary reload = %q, want the group A created (\"brand-new\") to show up", view)
	}
	if !strings.Contains(view, "renamed-done") {
		t.Fatalf("B's Groups panel after its ordinary reload = %q, want A's rename (\"renamed-done\") to show up", view)
	}
	if strings.Contains(view, "to-rename") {
		t.Fatalf("B's Groups panel after its ordinary reload = %q, still shows the stale pre-rename name \"to-rename\"", view)
	}
	if strings.Contains(view, "to-delete") {
		t.Fatalf("B's Groups panel after its ordinary reload = %q, still shows the group A deleted (\"to-delete\")", view)
	}

	// The underlying model state agrees with what the frame shows -- not
	// merely m.allGroups (which the pre-existing fix already refreshed),
	// but m.settingsGroups itself, the Groups panel's own live snapshot.
	var names []string
	for _, g := range mb.settingsGroups {
		names = append(names, g.Name)
	}
	wantNames := map[string]bool{"brand-new": true, "renamed-done": true}
	for want := range wantNames {
		found := false
		for _, n := range names {
			if n == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("mb.settingsGroups = %v after the ordinary reload, want it to contain %q", names, want)
		}
	}
	for _, n := range names {
		if n == "to-delete" || n == "to-rename" {
			t.Fatalf("mb.settingsGroups = %v after the ordinary reload, want neither the deleted nor the stale pre-rename name", names)
		}
	}
}

// TestSettingsGroupsPanelReloadPreservesSelectionAndInProgressEditing is
// task 002's second regression case: the ordinary reload that refreshes a
// stale Groups panel (the case above) must not do so by blowing away
// everything else the panel was mid-way through. Selection must follow the
// SURVIVING group's durable id (never its index -- an edit elsewhere in
// the list can move it), an in-progress "r" rename buffer that names a
// DIFFERENT group must survive untouched, and a staged scalar setting
// edit (m.settingsEdits, an entirely separate mechanism from the Groups
// panel's own immediate-write model) must survive right along with it.
func TestSettingsGroupsPanelReloadPreservesSelectionAndInProgressEditing(t *testing.T) {
	home := t.TempDir()
	dbPath := filepath.Join(home, "state.db")

	clientA, err := store.OpenPath(home, dbPath)
	if err != nil {
		t.Fatalf("client A OpenPath: %v", err)
	}
	t.Cleanup(func() { clientA.Close() })

	clientB, err := store.OpenPath(home, dbPath)
	if err != nil {
		t.Fatalf("client B OpenPath: %v", err)
	}
	t.Cleanup(func() { clientB.Close() })

	ctx := context.Background()

	// Alphabetical order (computeAvailableGroups' own precedent): alpha,
	// beta, gamma. B will select "beta" -- the one A leaves untouched.
	if _, err := clientA.CreateGroup(ctx, "alpha"); err != nil {
		t.Fatalf("client A CreateGroup(alpha): %v", err)
	}
	beta, err := clientA.CreateGroup(ctx, "beta")
	if err != nil {
		t.Fatalf("client A CreateGroup(beta): %v", err)
	}
	if _, err := clientA.CreateGroup(ctx, "gamma"); err != nil {
		t.Fatalf("client A CreateGroup(gamma): %v", err)
	}

	mb := settingsOpenOnGroupsFields(t, clientB)
	mb.width, mb.height = 100, 40

	// Select "beta" (index 1 of the alphabetical [alpha, beta, gamma]).
	updated, _ := mb.Update(key("down"))
	mb = updated.(Model)
	g, ok := mb.settingsSelectedGroup()
	if !ok || g.Name != "beta" || g.ID != beta.ID {
		t.Fatalf("selection before the reload = %+v (ok=%v), want beta (id %d)", g, ok, beta.ID)
	}

	// Open "r"'s in-progress rename editor on beta, and keep typing past
	// what CommitGroupEdit would need -- this text is never committed in
	// this test, so it must survive untouched.
	updated, _ = mb.Update(key("r"))
	mb = updated.(Model)
	if !mb.settingsGroupRenaming || mb.settingsGroupEditID != beta.ID {
		t.Fatalf("r did not open the rename editor on beta: renaming=%v editID=%d", mb.settingsGroupRenaming, mb.settingsGroupEditID)
	}
	mb = typeIntoGroupEditor(t, mb, "-in-progress")
	wantEditValue := "beta-in-progress"
	if mb.settingsGroupEditValue != wantEditValue {
		t.Fatalf("in-progress rename buffer = %q, want %q", mb.settingsGroupEditValue, wantEditValue)
	}

	// Stage a scalar setting edit -- an entirely separate mechanism
	// (m.settingsEdits, gated by ctrl+s/esc, never touched by the Groups
	// panel's own immediate-write commits) that an ordinary reload must
	// also leave alone.
	mb.settingsEdits.AllowYolo = !mb.settingsEdits.AllowYolo
	wantAllowYolo := mb.settingsEdits.AllowYolo

	// --- A edits the OTHER two groups, leaving beta itself untouched ---
	groupsBeforeEdit, err := clientA.ListGroups(ctx)
	if err != nil {
		t.Fatalf("client A ListGroups: %v", err)
	}
	var alphaID, gammaID int64
	for _, group := range groupsBeforeEdit {
		switch group.Name {
		case "alpha":
			alphaID = group.ID
		case "gamma":
			gammaID = group.ID
		}
	}
	if alphaID == 0 || gammaID == 0 {
		t.Fatalf("client A ListGroups = %+v, want both alpha and gamma", groupsBeforeEdit)
	}
	if err := clientA.RenameGroup(ctx, alphaID, "alpha-renamed"); err != nil {
		t.Fatalf("client A RenameGroup(alpha): %v", err)
	}
	if err := clientA.DeleteGroup(ctx, gammaID); err != nil {
		t.Fatalf("client A DeleteGroup(gamma): %v", err)
	}
	if _, err := clientA.CreateGroup(ctx, "delta"); err != nil {
		t.Fatalf("client A CreateGroup(delta): %v", err)
	}

	// --- B's ordinary reload ---
	msg := mb.loadSessions()
	loaded, ok := msg.(sessionsLoaded)
	if !ok {
		t.Fatalf("client B's loadSessions() = %T, want sessionsLoaded", msg)
	}
	if loaded.err != nil || loaded.groupsErr != nil {
		t.Fatalf("client B's loadSessions() reported err=%v groupsErr=%v", loaded.err, loaded.groupsErr)
	}

	next, _ := mb.Update(loaded)
	mb = next.(Model)

	// Selection followed beta's durable id, not its old index (gamma's
	// deletion and delta's/alpha-renamed's insertion both shuffle index
	// order in the alphabetical list).
	g, ok = mb.settingsSelectedGroup()
	if !ok || g.ID != beta.ID || g.Name != "beta" {
		t.Fatalf("selection after the ordinary reload = %+v (ok=%v), want it to stay on the surviving group beta (id %d)", g, ok, beta.ID)
	}

	// The in-progress rename editor is untouched: still open, on the same
	// target, with the exact same unc­ommitted text.
	if !mb.settingsGroupRenaming {
		t.Fatal("the ordinary reload closed the in-progress rename editor")
	}
	if mb.settingsGroupEditID != beta.ID {
		t.Fatalf("settingsGroupEditID after the reload = %d, want unchanged %d", mb.settingsGroupEditID, beta.ID)
	}
	if mb.settingsGroupEditValue != wantEditValue {
		t.Fatalf("in-progress rename buffer after the reload = %q, want unchanged %q", mb.settingsGroupEditValue, wantEditValue)
	}

	// The staged scalar setting edit is untouched.
	if mb.settingsEdits.AllowYolo != wantAllowYolo {
		t.Fatalf("staged m.settingsEdits.AllowYolo after the ordinary reload = %v, want unchanged %v", mb.settingsEdits.AllowYolo, wantAllowYolo)
	}

	// And the panel's own list reflects the edits A made to the OTHER
	// groups: alpha-renamed and delta present, gamma gone, beta itself
	// unaffected.
	view := mb.View()
	for _, want := range []string{"alpha-renamed", "delta", "beta"} {
		if !strings.Contains(view, want) {
			t.Fatalf("B's Groups panel after the reload = %q, want it to contain %q", view, want)
		}
	}
	if strings.Contains(view, "gamma") {
		t.Fatalf("B's Groups panel after the reload = %q, still shows the deleted group gamma", view)
	}
}
