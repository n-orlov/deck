package tui

import (
	"reflect"
	"strings"
	"testing"
)

// Task 012/D.1 (R137), cure-03. The criterion this file guards is a
// COMPILER property, not a convention: "internal/tui declares a cursor
// type whose session index cannot be read without resolving its kind, so a
// bare integer read of the old field no longer compiles". Go's field
// visibility is per package, so a cursor struct declared in package tui
// would still hand `m.selected.index` to every cursor read in this package
// -- a same-package probe containing nothing but
// `_ = m.sessions[m.selected.index]` compiled cleanly while the type lived
// here, which is precisely the misread the type exists to prevent.
//
// Two structural facts together make the bare read impossible from ANY
// package, including this one, and this test pins both:
//
//  1. the cursor type is declared outside package tui (so its unexported
//     fields are invisible here), and
//  2. every one of its fields is unexported (so no other package can read
//     the payload directly either).
//
// Both are checked through reflection rather than prose. Before cure-03
// moved the type into internal/tui/sidebarcursor, assertion 1 failed with
// `sidebarCursor is declared in package "github.com/n-orlov/deck/internal/tui"`.
func TestSidebarCursorPayloadIsUnreachableWithoutResolvingKind(t *testing.T) {
	const tuiPkg = "github.com/n-orlov/deck/internal/tui"

	typ := reflect.TypeOf(sidebarCursor{})
	if typ.Kind() != reflect.Struct {
		t.Fatalf("sidebarCursor kind = %v, want a struct", typ.Kind())
	}
	if typ.PkgPath() == tuiPkg {
		t.Errorf("sidebarCursor is declared in package %q: its unexported fields are therefore readable from every internal/tui call site (e.g. m.sessions[m.selected.index]) without resolving the cursor's kind; declare it in its own package instead", typ.PkgPath())
	}
	if !strings.HasPrefix(typ.PkgPath(), tuiPkg+"/") {
		t.Errorf("sidebarCursor is declared in package %q, want a package under %s/ (the cursor is internal/tui's own vocabulary)", typ.PkgPath(), tuiPkg)
	}
	for i := 0; i < typ.NumField(); i++ {
		if field := typ.Field(i); field.IsExported() {
			t.Errorf("sidebarCursor field %q is exported: the payload must only be reachable through an accessor that resolves the cursor's kind", field.Name)
		}
	}
	if typ.NumField() == 0 {
		t.Errorf("sidebarCursor has no fields at all, so this guard is checking nothing: expected an unexported kind plus its two payloads")
	}
}

// TestSidebarCursorAccessorsAllReportKind pins the other half of the
// criterion: the accessors that DO exist cannot hand back a payload
// without also reporting whether it is meaningful. Any exported method
// returning an integer must also return an ok bool, so no caller can write
// `m.sessions[m.selected.SessionIndex()]`-shaped code.
func TestSidebarCursorAccessorsAllReportKind(t *testing.T) {
	typ := reflect.TypeOf(sidebarCursor{})
	sawIntegerAccessor := false
	for i := 0; i < typ.NumMethod(); i++ {
		method := typ.Method(i)
		sig := method.Type
		returnsInteger, returnsBool := false, false
		for r := 0; r < sig.NumOut(); r++ {
			switch sig.Out(r).Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				returnsInteger = true
			case reflect.Bool:
				returnsBool = true
			}
		}
		if !returnsInteger {
			continue
		}
		sawIntegerAccessor = true
		if !returnsBool {
			t.Errorf("sidebarCursor.%s returns an integer with no ok result: a payload read must report whether the cursor's kind makes it meaningful", method.Name)
		}
	}
	if !sawIntegerAccessor {
		t.Errorf("sidebarCursor exposes no integer accessor at all, so this guard is checking nothing: expected SessionIndex and GroupID")
	}
}

// TestSidebarCursorAccessorsResolveKind is the behavioural companion: the
// accessors refuse the payload that does not belong to the cursor's kind,
// rather than returning a zero a caller could mistake for a real index.
func TestSidebarCursorAccessorsResolveKind(t *testing.T) {
	row := rowCursor(3)
	if idx, ok := row.SessionIndex(); !ok || idx != 3 {
		t.Errorf("rowCursor(3).SessionIndex() = (%d, %v), want (3, true)", idx, ok)
	}
	if gid, ok := row.GroupID(); ok || gid != 0 {
		t.Errorf("rowCursor(3).GroupID() = (%d, %v), want (0, false)", gid, ok)
	}

	header := headerCursor(7)
	if idx, ok := header.SessionIndex(); ok || idx != 0 {
		t.Errorf("headerCursor(7).SessionIndex() = (%d, %v), want (0, false)", idx, ok)
	}
	if gid, ok := header.GroupID(); !ok || gid != 7 {
		t.Errorf("headerCursor(7).GroupID() = (%d, %v), want (7, true)", gid, ok)
	}

	// The default group's header is a real stop keyed by the 0 sentinel,
	// not an absent one.
	if gid, ok := headerCursor(0).GroupID(); !ok || gid != 0 {
		t.Errorf("headerCursor(0).GroupID() = (%d, %v), want (0, true)", gid, ok)
	}

	// The zero value still means "the first session row", as it did when
	// Model.selected was a bare int.
	var zero sidebarCursor
	if zero != rowCursor(0) {
		t.Errorf("zero sidebarCursor = %v, want rowCursor(0)", zero)
	}
	if !zero.IsRow() || zero.IsHeader() {
		t.Errorf("zero sidebarCursor: IsRow()=%v IsHeader()=%v, want true/false", zero.IsRow(), zero.IsHeader())
	}
}
