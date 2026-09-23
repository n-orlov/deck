package tui

import (
	"context"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// Task 013/D.2 (R137). The criterion this file pins is behavioural, not
// structural: every key SPEC's keymap and the PRD name as acting on "the
// selected session" must do NOTHING at all -- no model mutation, no tea.Cmd
// -- when the cursor rests on a group header (task 012/D.1's sidebarCursor
// makes that reachable) rather than a session row. sessionScopedKeyGuardFixture
// wires every service function a handler below could possibly call to a
// real, non-nil stub -- so a handler that skipped the guard would actually
// run its body (open a dialog, dispatch a command, touch m.marked, ...)
// rather than being masked by an unrelated "service unavailable" nil check.
//
// Before this task, most of these bindings each carried their own
// `if session, ok := m.selectedSession(); ok` (or, for rename.go's detail
// `g`, no check at all -- see session_scoped_guard.go's own doc comment).
// This test exercises the ONE shared guard (guardSessionScopedKey) that now
// intercepts every one of them centrally, in tui.go's Update and in
// rename.go's updateDetailView, before any handler's own body ever runs.

// sessionScopedKeyGuardFixture builds a model whose cursor names a group
// header (not a row), with a group and one real, fully-eligible session
// sitting under it, and every session-scoped handler's own service
// function wired to a real, distinguishable stub -- never left nil, so a
// handler that reached its body due to a missing guard would actually
// produce a mutation or a command instead of being masked by its own
// separate "service unavailable" nil check.
func sessionScopedKeyGuardFixture() Model {
	groupID := int64(42)
	m := New(nil, config.Settings{}, "")
	m.sessions = []store.Session{{
		ID:                "s1",
		Name:              "s1",
		CWD:               "/work/proj",
		GroupName:         "proj",
		GroupID:           &groupID,
		Status:            "running",
		Agent:             "claude",
		PermissionProfile: "default",
		ResumeState:       "auto",
	}}
	m.selected = headerCursor(groupID)
	m.kill = func(context.Context, store.Session) error { return nil }
	m.acknowledge = func(context.Context, string) error { return nil }
	m.resume = func(context.Context, string) (store.Session, service.ResumeOutcome, error) {
		return store.Session{}, service.ResumeOutcome(0), nil
	}
	m.restart = func(context.Context, string) (store.Session, service.ResumeOutcome, error) {
		return store.Session{}, service.ResumeOutcome(0), nil
	}
	m.profileSwitch = func(context.Context, string, string) (store.Session, error) {
		return store.Session{}, nil
	}
	m.resumeMode = func(context.Context, string, string) (store.Session, error) {
		return store.Session{}, nil
	}
	m.archiveSvc = func(context.Context, store.Session) error { return nil }
	m.unarchiveSvc = func(context.Context, string) (store.Session, error) {
		return store.Session{}, nil
	}
	m.attach = func(context.Context, string) (*exec.Cmd, error) {
		return exec.Command("true"), nil
	}
	return m
}

// zeroFuncFieldsForEquality zeroes every func-kind field it finds while
// recursing through struct fields (including unexported ones, via
// unsafe.Pointer -- reflect.Value.Set refuses an unexported field outright,
// but the address itself is readable regardless of exported-ness). This
// exists solely so modelSnapshotForEquality below can hand the result to
// reflect.DeepEqual: DeepEqual's own documented rule ("func values are
// deeply equal if both are nil; otherwise they are not deeply equal") would
// otherwise report every comparison of two models sharing the exact same
// non-nil service stub (kill, resume, ...) as unequal, even though neither
// side's handler ever reassigns those fields -- a false mutation report
// that has nothing to do with what this test actually checks.
func zeroFuncFieldsForEquality(v reflect.Value) {
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			zeroFuncFieldsForEquality(v.Field(i))
		}
	case reflect.Func:
		if v.CanAddr() {
			addr := unsafe.Pointer(v.UnsafeAddr())
			reflect.NewAt(v.Type(), addr).Elem().Set(reflect.Zero(v.Type()))
		}
	}
}

// modelSnapshotForEquality returns m with every direct/nested func field
// zeroed, safe to compare with reflect.DeepEqual against another such
// snapshot. Pointer-typed fields (m.store, m.interactiveScroll, ...) are
// left untouched -- none of the handlers under test ever reassign one, so
// DeepEqual's own pointer rule ("equal using == or point to deeply equal
// values") short-circuits on identity without ever needing to walk into
// whatever a pointee itself might contain.
func modelSnapshotForEquality(m Model) Model {
	zeroFuncFieldsForEquality(reflect.ValueOf(&m).Elem())
	return m
}

// readPossiblyUnexportedField returns field v's value as an interface even
// when v is an unexported struct field: reflect.Value.Interface() refuses
// one outright, but the field's ADDRESS is readable regardless of
// exported-ness, which is the same door zeroFuncFieldsForEquality above
// walks through.
func readPossiblyUnexportedField(v reflect.Value) any {
	if !v.CanAddr() {
		return nil
	}
	return reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem().Interface()
}

// differingModelFields names the top-level Model fields on which two
// snapshots disagree, as "field: before -> after". The whole point is the
// failure message: a Model has ~250 fields, so dumping two of them whole
// (what this file used to do) buries the one field a missing guard actually
// moved -- and that one field name is precisely what a commit message or a
// validation note needs to quote.
func differingModelFields(before, after Model) []string {
	bv := reflect.ValueOf(&before).Elem()
	av := reflect.ValueOf(&after).Elem()
	var diffs []string
	for i := 0; i < bv.NumField(); i++ {
		b := readPossiblyUnexportedField(bv.Field(i))
		a := readPossiblyUnexportedField(av.Field(i))
		if !reflect.DeepEqual(b, a) {
			diffs = append(diffs, fmt.Sprintf("%s: %+v -> %+v", bv.Type().Field(i).Name, b, a))
		}
	}
	if len(diffs) == 0 {
		return []string{"(fields compare equal one by one, yet the whole structs do not -- a nested or map-valued field changed)"}
	}
	return diffs
}

// assertInertOnHeaderAfterEveryKey is the one shared assertion every case
// below makes: from the (optionally adjusted) fixture, each key in keys is
// fed to Update one at a time, and after EVERY one of them the model must
// still equal the pre-press snapshot (modulo the func-field wrinkle above)
// and the returned tea.Cmd must be nil. Asserting after every press, not
// merely after the last, is what makes a multi-key chord honest: `dd`'s
// first `d` raises m.pendingDelete and its second `d` clears that same
// field again, so a test that inspected only the final model would report a
// chord whose first half mutated the model on a header as inert -- exactly
// the hole D.2's validation found in this file's previous shape.
// setup, when non-nil, adjusts the fixture (e.g. opening the `i` detail
// dialog) and runs BEFORE the snapshot is taken, so a setup step's own,
// deliberate mutation is never mistaken for one Update produced.
func assertInertOnHeaderAfterEveryKey(t *testing.T, name string, setup func(m Model) Model, keys []string) {
	t.Helper()
	m := sessionScopedKeyGuardFixture()
	if setup != nil {
		m = setup(m)
	}
	before := modelSnapshotForEquality(m)
	for i, k := range keys {
		updated, cmd := m.Update(key(k))
		out, ok := updated.(Model)
		if !ok {
			t.Fatalf("%s: Update(%q) (keypress %d of %d) returned %T, not tui.Model", name, k, i+1, len(keys), updated)
		}
		if cmd != nil {
			t.Fatalf("%s: keypress %d of %d (%q) returned a non-nil tea.Cmd; every session-scoped key must be inert with the cursor on a header, at every step of a chord", name, i+1, len(keys), k)
		}
		if got := modelSnapshotForEquality(out); !reflect.DeepEqual(got, before) {
			t.Fatalf("%s: keypress %d of %d (%q) mutated the model while the cursor rested on a header: %s", name, i+1, len(keys), k, strings.Join(differingModelFields(before, got), "; "))
		}
		m = out
	}
}

// TestSessionScopedKeysAreInertOnAHeader is task 013/D.2's table: every key
// SPEC's keymap and the PRD name as session-scoped (enter, a, x, dd, r, R,
// i, e, P, p, s, z, Y, m, A, U, plus the `i` detail dialog's own move-group
// `g`) must do nothing at all with the cursor on a header. "s"/"z" are not
// wired to anything yet in this codebase (out of scope this phase) -- they
// are included because guardSessionScopedKey's own map already lists them,
// and an unbound key is trivially, uninterestingly inert either way; the
// real content of this test is the other fifteen.
func TestSessionScopedKeysAreInertOnAHeader(t *testing.T) {
	cases := []struct {
		name  string
		keys  []string
		setup func(m Model) Model
	}{
		{name: "enter", keys: []string{"enter"}},
		{name: "a", keys: []string{"a"}},
		{name: "x", keys: []string{"x"}},
		// Both halves of the chord are asserted, not just the end state:
		// the first `d` is the press that used to raise m.pendingDelete on
		// a header without ever consulting the shared guard.
		{name: "dd", keys: []string{"d", "d"}},
		{name: "r", keys: []string{"r"}},
		{name: "R", keys: []string{"R"}},
		{name: "i", keys: []string{"i"}},
		{name: "e", keys: []string{"e"}},
		{name: "P", keys: []string{"P"}},
		{name: "p", keys: []string{"p"}},
		{name: "s", keys: []string{"s"}},
		{name: "z", keys: []string{"z"}},
		{name: "Y", keys: []string{"Y"}},
		{name: "m", keys: []string{"m"}},
		{name: "A", keys: []string{"A"}},
		{name: "U", keys: []string{"U"}},
		{name: "detail g (move-group picker)", keys: []string{"g"}, setup: func(m Model) Model {
			m.detail = true
			return m
		}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assertInertOnHeaderAfterEveryKey(t, tc.name, tc.setup, tc.keys)
		})
	}
}

// TestFirstDOfDDAsksTheSharedGuardOnAHeader pins the specific regression
// D.2's validation caught: the dd chord's FIRST `d` must be refused by
// guardSessionScopedKey itself (the shared guard), not by any private check
// of the delete path's own. It asserts both halves of that claim directly --
// the guard reports "swallow" for "d" with the cursor on a header, and the
// keypress leaves m.pendingDelete false -- so a change that made the guard
// ignore "d" again would fail here even if some other check happened to keep
// the indicator down.
func TestFirstDOfDDAsksTheSharedGuardOnAHeader(t *testing.T) {
	m := sessionScopedKeyGuardFixture()
	if !m.guardSessionScopedKey("d") {
		t.Fatalf("guardSessionScopedKey(%q) = false with the cursor on a group header; the dd chord is session-scoped and must be gated by the shared guard, not by the delete path's own check", "d")
	}
	updated, cmd := m.Update(key("d"))
	after, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update(%q) returned %T, not tui.Model", "d", updated)
	}
	if cmd != nil {
		t.Fatalf("the first d of dd on a header returned a non-nil tea.Cmd")
	}
	if after.pendingDelete {
		t.Fatalf("the first d of dd raised m.pendingDelete while the cursor rested on a group header: false -> true")
	}

	// The same key with a non-empty mark set is task 112's batch dd, which
	// acts on the marked set and not on the cursor's row -- so the guard
	// must NOT gate it, header cursor or not. Pinned here so the exemption
	// stays deliberate rather than becoming collateral of the check above.
	batch := sessionScopedKeyGuardFixture()
	batch.marked = map[string]bool{"s1": true}
	if batch.guardSessionScopedKey("d") {
		t.Fatalf("guardSessionScopedKey(%q) = true with a non-empty mark set; batch dd acts on the marked set, so a header cursor must not gate it", "d")
	}
	batchUpdated, _ := batch.Update(key("d"))
	batchAfter, ok := batchUpdated.(Model)
	if !ok {
		t.Fatalf("Update(%q) returned %T, not tui.Model", "d", batchUpdated)
	}
	if !batchAfter.pendingDelete {
		t.Fatalf("the first d of a batch dd did not raise m.pendingDelete with a non-empty mark set")
	}
}
