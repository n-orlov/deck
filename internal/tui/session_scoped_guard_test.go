package tui

import (
	"context"
	"os/exec"
	"reflect"
	"testing"
	"unsafe"

	tea "github.com/charmbracelet/bubbletea"
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

// assertInertOnHeader is the one shared assertion every case below makes:
// running msg through Update from the (optionally adjusted) fixture must
// return the SAME model (modulo the func-field wrinkle above) and a nil
// tea.Cmd. setup, when non-nil, adjusts the fixture (e.g. opening the `i`
// detail dialog) and runs BEFORE the "before" snapshot is taken, so a
// setup step's own, deliberate mutation is never mistaken for one Update
// produced.
func assertInertOnHeader(t *testing.T, name string, setup func(m Model) Model, run func(m Model) (Model, tea.Cmd)) {
	t.Helper()
	m := sessionScopedKeyGuardFixture()
	if setup != nil {
		m = setup(m)
	}
	before := modelSnapshotForEquality(m)
	after, cmd := run(m)
	if cmd != nil {
		t.Fatalf("%s on a header returned a non-nil tea.Cmd; every session-scoped key must be inert with the cursor on a header", name)
	}
	if got, want := modelSnapshotForEquality(after), before; !reflect.DeepEqual(got, want) {
		t.Fatalf("%s mutated the model while the cursor rested on a header:\nbefore: %+v\nafter:  %+v", name, want, got)
	}
}

// TestSessionScopedKeysAreInertOnAHeader is task 013/D.2's table: every key
// SPEC's keymap and the PRD name as session-scoped (enter, a, x, dd, r, R,
// i, e, P, p, s, z, Y, m, A, U, plus the `i` detail dialog's own move-group
// `g`) must do nothing at all with the cursor on a header. "s"/"z" are not
// wired to anything yet in this codebase (out of scope this phase) -- they
// are included because guardSessionScopedKey's own map already lists them,
// and an unbound key is trivially, uninterestingly inert either way; the
// real content of this test is the other fourteen.
func TestSessionScopedKeysAreInertOnAHeader(t *testing.T) {
	singleKeyCases := []string{
		"enter", "a", "x", "r", "R", "i", "e", "P", "p", "Y", "m", "A", "U", "s", "z",
	}
	for _, k := range singleKeyCases {
		k := k
		t.Run(k, func(t *testing.T) {
			assertInertOnHeader(t, k, nil, func(m Model) (Model, tea.Cmd) {
				updated, cmd := m.Update(key(k))
				out, ok := updated.(Model)
				if !ok {
					t.Fatalf("Update(%q) returned %T, not tui.Model", k, updated)
				}
				return out, cmd
			})
		})
	}

	t.Run("dd", func(t *testing.T) {
		assertInertOnHeader(t, "dd", nil, func(m Model) (Model, tea.Cmd) {
			updated1, cmd1 := m.Update(key("d"))
			m1, ok := updated1.(Model)
			if !ok {
				t.Fatalf("Update(%q) (first d) returned %T, not tui.Model", "d", updated1)
			}
			if cmd1 != nil {
				t.Fatalf("the first d of dd on a header already returned a non-nil tea.Cmd")
			}
			updated2, cmd2 := m1.Update(key("d"))
			m2, ok := updated2.(Model)
			if !ok {
				t.Fatalf("Update(%q) (second d) returned %T, not tui.Model", "d", updated2)
			}
			return m2, cmd2
		})
	})

	t.Run("detail g (move-group picker)", func(t *testing.T) {
		assertInertOnHeader(t, "detail g", func(m Model) Model {
			m.detail = true
			return m
		}, func(m Model) (Model, tea.Cmd) {
			updated, cmd := m.Update(key("g"))
			out, ok := updated.(Model)
			if !ok {
				t.Fatalf("updateDetailView's Update(%q) returned %T, not tui.Model", "g", updated)
			}
			return out, cmd
		})
	})
}
