package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// TestEnvCursorStepWrapsAndHoldsOnEmpty pins the env browse cursor's step:
// up from the first row wraps to the last, down from the last wraps to the
// first, and an empty list leaves the cursor untouched.
func TestEnvCursorStepWrapsAndHoldsOnEmpty(t *testing.T) {
	cases := []struct {
		name              string
		cursor, rows, dir int
		want              int
	}{
		{"up from first wraps to last", 0, 4, -1, 3},
		{"up from middle", 2, 4, -1, 1},
		{"down from last wraps to first", 3, 4, 1, 0},
		{"down from middle", 1, 4, 1, 2},
		{"empty list holds on up", 0, 0, -1, 0},
		{"empty list holds on down", 0, 0, 1, 0},
	}
	for _, tc := range cases {
		if got := envCursorStep(tc.cursor, tc.rows, tc.dir); got != tc.want {
			t.Errorf("%s: envCursorStep(%d, %d, %d) = %d, want %d", tc.name, tc.cursor, tc.rows, tc.dir, got, tc.want)
		}
	}
}

// TestCursorPositionFindsFirstMatchOrMinusOne pins the shared scan the
// sidebar's nearest-visible and page-step walks both use.
func TestCursorPositionFindsFirstMatchOrMinusOne(t *testing.T) {
	order := []sidebarCursor{headerCursor(7), rowCursor(0), rowCursor(1)}
	if got := cursorPosition(order, rowCursor(1)); got != 2 {
		t.Errorf("cursorPosition of row 1 = %d, want 2", got)
	}
	if got := cursorPosition(order, headerCursor(7)); got != 0 {
		t.Errorf("cursorPosition of header 7 = %d, want 0", got)
	}
	if got := cursorPosition(order, rowCursor(9)); got != -1 {
		t.Errorf("cursorPosition of an absent cursor = %d, want -1", got)
	}
}

// TestClampRowCursorPullsDriftedIndexIntoRange pins nearestVisibleSelection's
// leading clamp: a row index outside m.sessions is pulled back in, a header
// cursor is untouched, and an empty session list leaves the cursor alone.
func TestClampRowCursorPullsDriftedIndexIntoRange(t *testing.T) {
	m := Model{sessions: []store.Session{{ID: "a"}, {ID: "b"}}}
	if got := m.clampRowCursor(rowCursor(5)); got != rowCursor(1) {
		t.Errorf("clamp of row 5 = %v, want row 1", got)
	}
	if got := m.clampRowCursor(rowCursor(-3)); got != rowCursor(0) {
		t.Errorf("clamp of row -3 = %v, want row 0", got)
	}
	if got := m.clampRowCursor(headerCursor(4)); got != headerCursor(4) {
		t.Errorf("clamp of a header = %v, want it untouched", got)
	}
	if got := (Model{}).clampRowCursor(rowCursor(5)); got != rowCursor(5) {
		t.Errorf("clamp with no sessions = %v, want row 5 untouched", got)
	}
}
