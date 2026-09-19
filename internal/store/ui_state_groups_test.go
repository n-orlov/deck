package store

import (
	"context"
	"path/filepath"
	"testing"
)

// TestCollapsedGroupsRoundTripsAcrossReopen exercises R129's persistence
// half of SPEC §11's "collapse state persists in ui_state so a group
// collapsed yesterday is still collapsed today" -- the collapsed set must
// survive a real store close/reopen, not just an in-process read of the
// value just written.
func TestCollapsedGroupsRoundTripsAcrossReopen(t *testing.T) {
	home := t.TempDir()
	dbPath := filepath.Join(home, "state.db")
	ctx := context.Background()

	st, err := OpenPath(home, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	g1, err := st.CreateGroup(ctx, "infra")
	if err != nil {
		t.Fatal(err)
	}
	g2, err := st.CreateGroup(ctx, "tooling")
	if err != nil {
		t.Fatal(err)
	}

	if err := st.SetCollapsedGroups(ctx, map[int64]bool{g1.ID: true, g2.ID: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenPath(home, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()

	collapsed, err := reopened.GetCollapsedGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(collapsed) != 2 || !collapsed[g1.ID] || !collapsed[g2.ID] {
		t.Fatalf("GetCollapsedGroups after reopen = %v; want {%d:true, %d:true}", collapsed, g1.ID, g2.ID)
	}

	// Un-collapsing one and persisting again must also round-trip -- the
	// setter replaces the whole set, it does not merge with what was there
	// before.
	collapsed[g1.ID] = false
	if err := reopened.SetCollapsedGroups(ctx, collapsed); err != nil {
		t.Fatal(err)
	}
	after, err := reopened.GetCollapsedGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[g1.ID] || !after[g2.ID] {
		t.Fatalf("GetCollapsedGroups after un-collapse = %v; want {%d:true}", after, g2.ID)
	}
}

// TestCollapsedGroupsAbsentRowYieldsEmptySet covers the documented default
// (SPEC §11.2: ui_state is not load-bearing) for collapsed_groups
// specifically: a database that has never had the key written at all must
// read back as "nothing collapsed", never an error.
func TestCollapsedGroupsAbsentRowYieldsEmptySet(t *testing.T) {
	home := t.TempDir()
	st, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	collapsed, err := st.GetCollapsedGroups(ctx)
	if err != nil {
		t.Fatalf("GetCollapsedGroups before any write returned err = %v", err)
	}
	if collapsed == nil || len(collapsed) != 0 {
		t.Fatalf("GetCollapsedGroups before any write = %v; want empty, non-nil map", collapsed)
	}
}

// TestLastCreateGroupAbsentRowDegradesToDefault covers GetLastCreateGroup's
// own documented default, following lastCreateAgentUIStateKey's precedent:
// a database that never had a create succeed against a real group reads
// back as nil (default), never an error.
func TestLastCreateGroupAbsentRowDegradesToDefault(t *testing.T) {
	home := t.TempDir()
	st, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	group, err := st.GetLastCreateGroup(ctx)
	if err != nil {
		t.Fatalf("GetLastCreateGroup before any write returned err = %v", err)
	}
	if group != nil {
		t.Fatalf("GetLastCreateGroup before any write = %v; want nil", group)
	}
}

// TestLastCreateGroupDanglingIDDegradesToDefaultNotEmptyValue is R130's own
// dangling-group case (SPEC §11: "a group_id that no longer resolves ...
// renders under default rather than vanishing"): once the remembered group
// is deleted, the accessor must degrade to nil (default) exactly like an
// absent row -- it must not surface some other, still-distinguishable
// "empty" sentinel value that a caller could mistake for "some group is
// still intended".
func TestLastCreateGroupDanglingIDDegradesToDefaultNotEmptyValue(t *testing.T) {
	home := t.TempDir()
	st, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	g, err := st.CreateGroup(ctx, "infra")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetLastCreateGroup(ctx, g.ID); err != nil {
		t.Fatal(err)
	}

	// Confirm it resolves while the group still exists.
	got, err := st.GetLastCreateGroup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || *got != g.ID {
		t.Fatalf("GetLastCreateGroup while group exists = %v; want %d", got, g.ID)
	}

	if err := st.DeleteGroup(ctx, g.ID); err != nil {
		t.Fatal(err)
	}

	got, err = st.GetLastCreateGroup(ctx)
	if err != nil {
		t.Fatalf("GetLastCreateGroup after group deleted returned err = %v", err)
	}
	if got != nil {
		t.Fatalf("GetLastCreateGroup after group deleted = %v; want nil (default)", got)
	}

	// The ui_state row itself is left as-is (still names the now-dangling
	// id) -- only the accessor's return value degrades, matching
	// lastCreateAgentUIStateKey's own "the row is not rewritten just
	// because a read degraded" precedent for an unrecognised value.
	var stored string
	if err := st.DB().QueryRow(`SELECT value FROM ui_state WHERE key = 'last_create_group'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	wantStored := "1"
	if g.ID != 1 {
		t.Skip("group id was not 1; sentinel string comparison would be misleading")
	}
	if stored != wantStored {
		t.Fatalf("stored ui_state value after dangling read = %q; want %q", stored, wantStored)
	}
}

// TestLastCreateGroupOverwritesInPlace mirrors
// TestLastCreateAgentAccessorsDegradeToDocumentedDefaults' own "overwriting
// an existing key updates in place rather than duplicating a row" check.
func TestLastCreateGroupOverwritesInPlace(t *testing.T) {
	home := t.TempDir()
	st, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	g1, err := st.CreateGroup(ctx, "infra")
	if err != nil {
		t.Fatal(err)
	}
	g2, err := st.CreateGroup(ctx, "tooling")
	if err != nil {
		t.Fatal(err)
	}

	if err := st.SetLastCreateGroup(ctx, g1.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.SetLastCreateGroup(ctx, g2.ID); err != nil {
		t.Fatal(err)
	}

	var rows int
	if err := st.DB().QueryRow(`SELECT count(*) FROM ui_state WHERE key = 'last_create_group'`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("ui_state last_create_group rows = %d, %v; want 1", rows, err)
	}

	got, err := st.GetLastCreateGroup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || *got != g2.ID {
		t.Fatalf("GetLastCreateGroup after overwrite = %v; want %d", got, g2.ID)
	}
}

// TestSetLastCreateGroupDefaultForgetsTheRememberedNamedGroup pins the
// other direction of R130's "the last group created into": a create into
// the structural default group is itself a remembered choice, so recording
// it (SetLastCreateGroup with id 0, the default group's id) must forget the
// named group remembered before it rather than leaving that named group as
// the value a later create opens on.
func TestSetLastCreateGroupDefaultForgetsTheRememberedNamedGroup(t *testing.T) {
	home := t.TempDir()
	st, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	g, err := st.CreateGroup(ctx, "platform")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetLastCreateGroup(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetLastCreateGroup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || *got != g.ID {
		t.Fatalf("GetLastCreateGroup after remembering %q = %v; want %d", g.Name, got, g.ID)
	}

	// The create into default: the group still exists, so this cannot be
	// mistaken for the dangling-id degrade the test above covers.
	if err := st.SetLastCreateGroup(ctx, 0); err != nil {
		t.Fatalf("SetLastCreateGroup(ctx, 0) returned err = %v", err)
	}
	got, err = st.GetLastCreateGroup(ctx)
	if err != nil {
		t.Fatalf("GetLastCreateGroup after a create into default returned err = %v", err)
	}
	if got != nil {
		t.Fatalf("GetLastCreateGroup after a create into default = %v; want nil (default), not the stale named group", got)
	}
	groups, err := st.ListGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].ID != g.ID {
		t.Fatalf("recording a create into default disturbed the groups table: %+v", groups)
	}
}
