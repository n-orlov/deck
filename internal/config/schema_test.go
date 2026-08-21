package config

import (
	"sort"
	"testing"
)

// TestSchemaPinsKeySet enumerates the schema and pins the exact set of
// flat config.toml keys (task 010): allow_yolo, stale_after,
// capture_min_interval, [ui] theme, [ui] ascii, [ui] mouse,
// [ui] recent_cwd_limit, and the [env] table. Adding, removing or
// renaming a key must be a deliberate edit to this test alongside the
// schema, never a silent drift.
func TestSchemaPinsKeySet(t *testing.T) {
	want := []string{
		"allow_yolo",
		"stale_after",
		"capture_min_interval",
		"ui.theme",
		"ui.ascii",
		"ui.mouse",
		"ui.recent_cwd_limit",
		"[env]",
	}
	var got []string
	for _, field := range Schema {
		got = append(got, field.FullKey())
	}
	sortedWant := append([]string{}, want...)
	sortedGot := append([]string{}, got...)
	sort.Strings(sortedWant)
	sort.Strings(sortedGot)
	if len(sortedWant) != len(sortedGot) {
		t.Fatalf("schema key count = %d (%v), want %d (%v)", len(sortedGot), got, len(sortedWant), want)
	}
	for i := range sortedWant {
		if sortedWant[i] != sortedGot[i] {
			t.Fatalf("schema key set = %v, want %v", sortedGot, sortedWant)
		}
	}
}

// TestSchemaFieldsAreComplete asserts every declared field carries the
// metadata task 010 requires: a kind, a non-empty description, a scope,
// and (for integer fields) bounds with at least a Min.
func TestSchemaFieldsAreComplete(t *testing.T) {
	for _, field := range Schema {
		full := field.FullKey()
		if field.Kind == "" {
			t.Errorf("%s: missing Kind", full)
		}
		if field.Description == "" {
			t.Errorf("%s: missing Description", full)
		}
		switch field.Scope {
		case ScopeGlobal, ScopeSessionOverride, ScopeRestartToApply:
		default:
			t.Errorf("%s: Scope %q is not one of the three documented labels", full, field.Scope)
		}
		if field.Kind == KindInteger {
			if field.IntBounds.Max != nil && *field.IntBounds.Max < field.IntBounds.Min {
				t.Errorf("%s: IntBounds.Max %d < Min %d", full, *field.IntBounds.Max, field.IntBounds.Min)
			}
		}
		if field.Kind == KindEnum && !field.DynamicEnum && len(field.EnumValues) == 0 {
			t.Errorf("%s: KindEnum field has neither static EnumValues nor DynamicEnum", full)
		}
	}
}

// TestSchemaScopes pins the scope each field carries, so a future change
// to how a field's edit takes effect is a deliberate edit here, not a
// silent drift discovered by a downstream test.
func TestSchemaScopes(t *testing.T) {
	want := map[string]Scope{
		"allow_yolo":           ScopeGlobal,
		"stale_after":          ScopeGlobal,
		"capture_min_interval": ScopeGlobal,
		"ui.theme":             ScopeGlobal,
		"ui.ascii":             ScopeGlobal,
		"ui.mouse":             ScopeGlobal,
		"ui.recent_cwd_limit":  ScopeGlobal,
		"[env]":                ScopeRestartToApply,
	}
	for _, field := range Schema {
		full := field.FullKey()
		wantScope, ok := want[full]
		if !ok {
			t.Fatalf("%s: no expected scope pinned in this test (schema drift?)", full)
		}
		if field.Scope != wantScope {
			t.Errorf("%s: Scope = %q, want %q", full, field.Scope, wantScope)
		}
	}
}

// TestFieldByFullKey exercises the lookup helper for a hit and a miss.
func TestFieldByFullKey(t *testing.T) {
	field, ok := FieldByFullKey("ui.theme")
	if !ok {
		t.Fatal("FieldByFullKey(\"ui.theme\") not found")
	}
	if field.Kind != KindEnum {
		t.Errorf("ui.theme Kind = %q, want %q", field.Kind, KindEnum)
	}
	if _, ok := FieldByFullKey("ui.nonexistent"); ok {
		t.Error("FieldByFullKey(\"ui.nonexistent\") unexpectedly found")
	}
}

// TestFieldFullKey pins FullKey's rendering for each of the three shapes:
// top-level, nested-in-a-table, and a whole-table field.
func TestFieldFullKey(t *testing.T) {
	cases := []struct {
		field Field
		want  string
	}{
		{Field{Section: "", Key: "allow_yolo"}, "allow_yolo"},
		{Field{Section: "ui", Key: "theme"}, "ui.theme"},
		{Field{Section: "env", Key: ""}, "[env]"},
	}
	for _, c := range cases {
		if got := c.field.FullKey(); got != c.want {
			t.Errorf("FullKey() = %q, want %q", got, c.want)
		}
	}
}
