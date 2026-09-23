package tui

import (
	"reflect"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestGroupSortsBeforeHonoursDefaultFirstFlagBothWays proves task 002's
// whole point: groupSortsBefore takes the `[ui] default_group_first` flag
// as a plain bool parameter and reorders the implicit default group (the
// empty group key) around it, comparing only the two group KEYS it is
// handed -- never m.sessions, attention or sort_order. With the flag
// false this is R129's original rule (default always LAST, even against
// "aaa", the alphabetically-earliest real name); with the flag true the
// same two keys reverse to default FIRST, even against "zzz", the
// alphabetically-latest real name.
func TestGroupSortsBeforeHonoursDefaultFirstFlagBothWays(t *testing.T) {
	// defaultFirst == false: real name always leads, whichever real name.
	if !groupSortsBefore("aaa", "", false) {
		t.Fatal(`groupSortsBefore("aaa", "", false) = false, want true (default last)`)
	}
	if groupSortsBefore("", "aaa", false) {
		t.Fatal(`groupSortsBefore("", "aaa", false) = true, want false (default last)`)
	}
	if !groupSortsBefore("zzz", "", false) {
		t.Fatal(`groupSortsBefore("zzz", "", false) = false, want true (default last, even after "zzz")`)
	}
	if groupSortsBefore("", "zzz", false) {
		t.Fatal(`groupSortsBefore("", "zzz", false) = true, want false (default last, even after "zzz")`)
	}

	// defaultFirst == true: the SAME two keys, flag flipped, reverse.
	if groupSortsBefore("aaa", "", true) {
		t.Fatal(`groupSortsBefore("aaa", "", true) = true, want false (default first)`)
	}
	if !groupSortsBefore("", "aaa", true) {
		t.Fatal(`groupSortsBefore("", "aaa", true) = false, want true (default first)`)
	}
	if groupSortsBefore("zzz", "", true) {
		t.Fatal(`groupSortsBefore("zzz", "", true) = true, want false (default first, even before "zzz")`)
	}
	if !groupSortsBefore("", "zzz", true) {
		t.Fatal(`groupSortsBefore("", "zzz", true) = false, want true (default first, even before "zzz")`)
	}

	// Two real names never consult the flag at all -- alphabetical either way.
	if !groupSortsBefore("aaa", "zzz", false) || !groupSortsBefore("aaa", "zzz", true) {
		t.Fatal(`groupSortsBefore("aaa", "zzz", ...) must be true regardless of the flag (both real names)`)
	}
	if groupSortsBefore("zzz", "aaa", false) || groupSortsBefore("zzz", "aaa", true) {
		t.Fatal(`groupSortsBefore("zzz", "aaa", ...) must be false regardless of the flag (both real names)`)
	}

	// Two empty keys: neither sorts before the other, regardless of the flag.
	if groupSortsBefore("", "", false) || groupSortsBefore("", "", true) {
		t.Fatal(`groupSortsBefore("", "", ...) must be false regardless of the flag`)
	}
}

// groupDefaultFirstTestModel builds a Model with sessions in groups "aaa",
// "zzz", the structural default (empty GroupName) and a user-created group
// literally named "Default" -- exercising the case task 002's criteria
// names explicitly: the structural default (key "") and a real group
// whose display name happens to also read "Default" must never collide,
// whichever way the default_group_first flag points.
func groupDefaultFirstTestModel(defaultFirst bool) Model {
	m := New(nil, config.Settings{DefaultGroupFirst: defaultFirst}, "")
	m.sessions = []store.Session{
		{ID: "z1", GroupName: "zzz"},
		{ID: "d1", GroupName: ""}, // implicit/structural default
		{ID: "a1", GroupName: "aaa"},
		{ID: "cap1", GroupName: "Default"}, // user-created group literally named "Default"
	}
	return m
}

// TestGroupOrderDefaultFirstFlagFalseMatchesR129Baseline proves the flag
// defaulting to false reproduces R129's original "default always last"
// order untouched, with the user-created "Default" group sorting purely
// alphabetically (capital D folds to lowercase "default", landing between
// "aaa" and "zzz") rather than being swept into the structural bucket.
func TestGroupOrderDefaultFirstFlagFalseMatchesR129Baseline(t *testing.T) {
	m := groupDefaultFirstTestModel(false)
	groups := m.groupSessions()
	var got []string
	for _, g := range groups {
		got = append(got, g.Name)
	}
	want := []string{"aaa", "Default", "zzz", ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("group order (default_group_first=false) = %v, want %v", got, want)
	}
}

// TestGroupOrderDefaultFirstFlagTrueMovesStructuralDefaultAloneToFront
// proves the flag flipped to true moves ONLY the structural default group
// (key "") to the front -- the user-created group literally named
// "Default" still sorts alphabetically among the real groups, proving the
// two do not collide under either flag value.
func TestGroupOrderDefaultFirstFlagTrueMovesStructuralDefaultAloneToFront(t *testing.T) {
	m := groupDefaultFirstTestModel(true)
	groups := m.groupSessions()
	var got []string
	for _, g := range groups {
		got = append(got, g.Name)
	}
	want := []string{"", "aaa", "Default", "zzz"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("group order (default_group_first=true) = %v, want %v", got, want)
	}
}

// TestCollapsedGroupsFoldStateByteIdenticalAcrossDefaultFirstFlagFlip
// proves the default_group_first flag only ever changes bucket ORDER --
// never fold state. Collapse state is keyed by group id (sessionGroupID),
// which does not depend in any way on where a group's bucket paints, so
// flipping the flag and recomputing groups must leave m.collapsedGroups
// byte-identical (deep-equal) to what it was before the flip.
func TestCollapsedGroupsFoldStateByteIdenticalAcrossDefaultFirstFlagFlip(t *testing.T) {
	m := groupDefaultFirstTestModel(false)
	m.collapsedGroups = map[int64]bool{0: true, 7: true}
	before := make(map[int64]bool, len(m.collapsedGroups))
	for k, v := range m.collapsedGroups {
		before[k] = v
	}

	_ = m.groupSessions() // false-flag order

	m.settings.DefaultGroupFirst = true
	_ = m.groupSessions() // true-flag order, same Model, same collapse map

	if !reflect.DeepEqual(m.collapsedGroups, before) {
		t.Fatalf("collapsedGroups = %v after flipping default_group_first, want unchanged %v", m.collapsedGroups, before)
	}
}
