package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// TestSettingsKeyOpensAndEscCloses proves the takeover's own on/off switch:
// `,` opens it (never while help is showing, mirroring every other
// overlay's guard) and esc closes it, leaving every other field untouched.
func TestSettingsKeyOpensAndEscCloses(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	updated, _ := model.Update(key(","))
	model = updated.(Model)
	if !model.settingsOpen {
		t.Fatal(", did not open settings")
	}
	if model.settingsCategoryIndex != 0 || model.settingsFieldIndex != 0 {
		t.Fatalf("settings opened with non-zero selection: category=%d field=%d", model.settingsCategoryIndex, model.settingsFieldIndex)
	}
	updated, _ = model.Update(key("esc"))
	model = updated.(Model)
	if model.settingsOpen {
		t.Fatal("esc did not close settings")
	}

	// `,` while help is open is a no-op, exactly like `n`'s existing guard.
	model.help = true
	updated, _ = model.Update(key(","))
	if updated.(Model).settingsOpen {
		t.Fatal(", opened settings while help was showing")
	}
}

// TestSettingsViewRendersSchemaDerivedCategoriesAndFields proves the
// takeover has no second, hand-written field list: every category name and
// every field label settingsCategories()/settingsFieldLabel() derive from
// config.Schema actually appears in the rendered frame, and the view walks
// the schema rather than a fixture the test and the code happen to share.
func TestSettingsViewRendersSchemaDerivedCategoriesAndFields(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.settingsOpen = true
	view := model.settingsView()

	categories := settingsCategories()
	if len(categories) == 0 {
		t.Fatal("settingsCategories() returned no categories")
	}
	for _, cat := range categories {
		if !strings.Contains(view, cat.Name) {
			t.Errorf("settings view missing category %q:\n%s", cat.Name, view)
		}
	}

	// The view opens on category 0; every one of its fields' derived
	// labels must be on screen.
	for _, f := range categories[0].Fields {
		label := settingsFieldLabel(f)
		if !strings.Contains(view, label) {
			t.Errorf("settings view missing field label %q for %s:\n%s", label, f.FullKey(), view)
		}
	}

	// Selecting a later category (UI, whose fields differ from General's)
	// must swap the right-hand list to that category's own fields.
	uiIndex := -1
	for i, cat := range categories {
		if cat.Section == "ui" {
			uiIndex = i
		}
	}
	if uiIndex < 0 {
		t.Fatal("no ui category found — schema.go's ui.* fields moved?")
	}
	model.settingsCategoryIndex = uiIndex
	model.settingsFieldIndex = 0
	uiView := model.settingsView()
	for _, f := range categories[uiIndex].Fields {
		label := settingsFieldLabel(f)
		if !strings.Contains(uiView, label) {
			t.Errorf("ui category view missing field label %q for %s:\n%s", label, f.FullKey(), uiView)
		}
	}
}

// TestSettingsCategoriesGroupEveryFlatKeyExactlyOnce is the schema/settings
// parity precursor to task 018's structural test: every field
// config.Schema declares appears in exactly one category's Fields, and no
// category invents a field the schema does not declare -- except the one
// stated exception SPEC §11.5 itself names: "[notify]... settings shows
// [it] as a single navigable entry", which config.Schema deliberately
// does NOT declare (schema.go's own comment) since it is a structured
// table, not a flat key. Task 015 adds exactly that one synthetic entry
// (settingsNotifyEntry, FullKey "[notify]"); this test now names it
// explicitly as the sole permitted non-schema surplus rather than either
// silently tolerating any surplus or breaking on the one §11.5 requires.
func TestSettingsCategoriesGroupEveryFlatKeyExactlyOnce(t *testing.T) {
	categories := settingsCategories()
	seen := map[string]int{}
	for _, cat := range categories {
		for _, f := range cat.Fields {
			seen[f.FullKey()]++
		}
	}
	const notifyKey = "[notify]"
	if seen[notifyKey] != 1 {
		t.Fatalf("settingsCategories() surfaced %s %d times, want exactly 1 (the §11.5 single navigable entry)", notifyKey, seen[notifyKey])
	}
	delete(seen, notifyKey)
	if len(seen) != len(config.Schema) {
		t.Fatalf("settingsCategories() surfaced %d distinct non-notify fields, want %d (one per schema field)", len(seen), len(config.Schema))
	}
	for _, f := range config.Schema {
		if seen[f.FullKey()] != 1 {
			t.Errorf("schema field %s appears %d times across settings categories, want exactly 1", f.FullKey(), seen[f.FullKey()])
		}
	}
}

// TestSettingsFieldLabelIsHumanReadable pins settingsFieldLabel's rendering
// rule against a couple of real schema keys, so a future refactor of the
// helper is caught by name rather than only by the broader "label appears
// on screen" assertion above.
func TestSettingsFieldLabelIsHumanReadable(t *testing.T) {
	cases := []struct {
		field config.Field
		want  string
	}{
		{config.Field{Key: "allow_yolo"}, "Allow Yolo"},
		{config.Field{Key: "recent_cwd_limit"}, "Recent Cwd Limit"},
		{config.Field{Section: "env", Key: ""}, "Environment Variables"},
	}
	for _, c := range cases {
		if got := settingsFieldLabel(c.field); got != c.want {
			t.Errorf("settingsFieldLabel(%+v) = %q, want %q", c.field, got, c.want)
		}
	}
}

// TestSettingsViewFitsFrameBudget is requirement 6's frame-budget guarantee
// applied to the settings takeover (task 013's own success criteria): at a
// stated terminal size, the rendered frame never has more content lines
// than the terminal's rows, and no line's visible width (stringWidth,
// which already discounts SGR escapes the same way the harness's own
// per-cell extraction does) exceeds its columns. Checked at deck's own
// supported minimum (80x24) and at a larger size, so the guarantee is not
// an accident of one particular geometry.
func TestSettingsViewFitsFrameBudget(t *testing.T) {
	for _, size := range []struct{ width, height int }{
		{80, 24},
		{120, 40},
	} {
		model := New(nil, config.Settings{}, "")
		model.settingsOpen = true
		model.width, model.height = size.width, size.height
		view := model.settingsView()
		lines := strings.Split(view, "\n")
		if len(lines) > size.height {
			t.Errorf("at %dx%d: settings view has %d lines, exceeding the %d-row budget", size.width, size.height, len(lines), size.height)
		}
		for i, line := range lines {
			if w := stringWidth(line); w > size.width {
				t.Errorf("at %dx%d: settings view line %d is %d columns wide, exceeding the %d-column budget:\n%s", size.width, size.height, i, w, size.width, line)
			}
		}
	}
}

// TestSettingsNavTabAndLeftRightSwitchFocus proves §11.5's "tab/left/right
// switch between the category list and the field list": starting focused
// on categories, each of tab/left/right moves focus to fields and, pressed
// again, back to categories.
func TestSettingsNavTabAndLeftRightSwitchFocus(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.settingsOpen = true
	if model.settingsFocus != settingsFocusCategories {
		t.Fatalf("settings opened with focus %d, want settingsFocusCategories", model.settingsFocus)
	}

	for _, k := range []string{"tab", "left", "right"} {
		m := model
		updated, _ := m.Update(key(k))
		m = updated.(Model)
		if m.settingsFocus != settingsFocusFields {
			t.Errorf("%s from categories focus = %d, want settingsFocusFields", k, m.settingsFocus)
		}
		updated, _ = m.Update(key(k))
		m = updated.(Model)
		if m.settingsFocus != settingsFocusCategories {
			t.Errorf("%s twice returned focus %d, want settingsFocusCategories", k, m.settingsFocus)
		}
	}
}

// TestSettingsNavUpDownMovesFocusedListAndWraps proves §11.5's "up/down
// move within the focused list": while categories has focus, up/down walk
// the category list and wrap at both ends (matching every other cycled
// selection in this package); switching focus to fields and moving there
// proves the two lists are independently addressable, not one shared
// index.
func TestSettingsNavUpDownMovesFocusedListAndWraps(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.settingsOpen = true
	categories := settingsCategories()
	if len(categories) < 2 {
		t.Fatal("need at least two categories to prove wrap-around")
	}

	// Up from category 0 wraps to the last category.
	updated, _ := model.Update(key("up"))
	m := updated.(Model)
	if m.settingsCategoryIndex != len(categories)-1 {
		t.Fatalf("up from category 0 = %d, want %d (wrap to last)", m.settingsCategoryIndex, len(categories)-1)
	}
	// Down from the last category wraps back to 0.
	updated, _ = m.Update(key("down"))
	m = updated.(Model)
	if m.settingsCategoryIndex != 0 {
		t.Fatalf("down from last category = %d, want 0 (wrap to first)", m.settingsCategoryIndex)
	}

	// Switch focus to fields and move within the first category's own
	// field list; General has at least two fields (allow_yolo,
	// stale_after, capture_min_interval), so down from field 0 must land
	// on field 1 without disturbing settingsCategoryIndex.
	fields := categories[0].Fields
	if len(fields) < 2 {
		t.Fatal("first category needs at least two fields to prove within-list movement")
	}
	updated, _ = m.Update(key("tab"))
	m = updated.(Model)
	updated, _ = m.Update(key("down"))
	m = updated.(Model)
	if m.settingsCategoryIndex != 0 {
		t.Fatalf("moving the field list changed settingsCategoryIndex to %d", m.settingsCategoryIndex)
	}
	if m.settingsFieldIndex != 1 {
		t.Fatalf("down within fields = %d, want 1", m.settingsFieldIndex)
	}
}

// TestSettingsSearchMatchesByDescriptionAlone proves §11.5's "/ ...
// searches every field by label AND description": stale_after's label
// ("Stale After") does not contain "verdict", but its description does
// ("a hook-derived status verdict may reach"), so a search for "verdict"
// must still surface it — a search that only ever consulted the label
// would miss this even though the code would still compile and other
// tests would still pass.
func TestSettingsSearchMatchesByDescriptionAlone(t *testing.T) {
	var staleAfter config.Field
	found := false
	for _, f := range config.Schema {
		if f.FullKey() == "stale_after" {
			staleAfter = f
			found = true
		}
	}
	if !found {
		t.Fatal("schema has no stale_after field")
	}
	if strings.Contains(strings.ToLower(settingsFieldLabel(staleAfter)), "verdict") {
		t.Fatal(`test's premise broke: stale_after's label now contains "verdict"`)
	}

	results := settingsSearchMatches("verdict")
	if len(results) == 0 {
		t.Fatal(`settingsSearchMatches("verdict") found nothing; description search is not working`)
	}
	gotStaleAfter := false
	for _, r := range results {
		if r.Field.FullKey() == "stale_after" {
			gotStaleAfter = true
		} else {
			t.Errorf(`settingsSearchMatches("verdict") unexpectedly also matched %s`, r.Field.FullKey())
		}
	}
	if !gotStaleAfter {
		t.Fatal(`settingsSearchMatches("verdict") did not include stale_after`)
	}
}

// TestSettingsSearchKeyEntersSearchModeAndEnterJumps drives the search box
// end to end through Model.Update: `/` enters search mode, typed runes
// build the query, enter on the (single) match jumps
// settingsCategoryIndex/settingsFieldIndex to it and switches focus to the
// field list, leaving search mode.
func TestSettingsSearchKeyEntersSearchModeAndEnterJumps(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.settingsOpen = true

	updated, _ := model.Update(key("/"))
	m := updated.(Model)
	if !m.settingsSearchActive {
		t.Fatal("/ did not enter search mode")
	}

	updated, _ = m.Update(key("verdict"))
	m = updated.(Model)
	if m.settingsSearchQuery != "verdict" {
		t.Fatalf("settingsSearchQuery = %q, want %q", m.settingsSearchQuery, "verdict")
	}

	results := settingsSearchMatches(m.settingsSearchQuery)
	if len(results) != 1 || results[0].Field.FullKey() != "stale_after" {
		t.Fatalf("unexpected search results for %q: %+v", m.settingsSearchQuery, results)
	}

	updated, _ = m.Update(key("enter"))
	m = updated.(Model)
	if m.settingsSearchActive {
		t.Fatal("enter did not leave search mode")
	}
	if m.settingsFocus != settingsFocusFields {
		t.Fatalf("enter left focus at %d, want settingsFocusFields", m.settingsFocus)
	}
	categories := settingsCategories()
	gotField := categories[m.settingsCategoryIndex].Fields[m.settingsFieldIndex]
	if gotField.FullKey() != "stale_after" {
		t.Fatalf("enter jumped to %s, want stale_after", gotField.FullKey())
	}
}

// TestSettingsSearchEscCancelsWithoutMovingSelection proves esc while
// searching only leaves search mode (clearing the query) and never moves
// settingsCategoryIndex/settingsFieldIndex — unlike enter, it must not
// jump anywhere, and it must not close the whole takeover either.
func TestSettingsSearchEscCancelsWithoutMovingSelection(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.settingsOpen = true
	updated, _ := model.Update(key("/"))
	m := updated.(Model)
	updated, _ = m.Update(key("v"))
	m = updated.(Model)

	updated, _ = m.Update(key("esc"))
	m = updated.(Model)
	if m.settingsSearchActive {
		t.Fatal("esc did not leave search mode")
	}
	if m.settingsSearchQuery != "" {
		t.Fatalf("esc left settingsSearchQuery = %q, want empty", m.settingsSearchQuery)
	}
	if m.settingsCategoryIndex != 0 || m.settingsFieldIndex != 0 {
		t.Fatalf("esc from search moved selection to category=%d field=%d", m.settingsCategoryIndex, m.settingsFieldIndex)
	}
	if !m.settingsOpen {
		t.Fatal("esc from search closed the whole takeover, not just search mode")
	}
}

// TestSettingsSearchBackspaceShortensQuery proves backspace removes one
// rune from the typed query rather than clearing it outright or being a
// no-op.
func TestSettingsSearchBackspaceShortensQuery(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.settingsOpen = true
	updated, _ := model.Update(key("/"))
	m := updated.(Model)
	updated, _ = m.Update(key("thm"))
	m = updated.(Model)
	updated, _ = m.Update(key("backspace"))
	m = updated.(Model)
	if m.settingsSearchQuery != "th" {
		t.Fatalf("settingsSearchQuery after backspace = %q, want %q", m.settingsSearchQuery, "th")
	}
}
