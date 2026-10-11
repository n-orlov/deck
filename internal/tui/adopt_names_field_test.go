package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// adoptEditorModel opens the launch-inputs editor with the real service's
// SetHarnessNameAdopt wired in, as cmd/deck/main.go wires it.
func adoptEditorModel(t *testing.T, db *store.Store, id string) Model {
	t.Helper()
	clock, err := config.NewClock("2025-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
	svc := service.Service{Store: db, Clock: clock}
	return launchInputsTestModel(t, db, id).WithAdoptSetter(svc.SetHarnessNameAdopt)
}

// TestAdoptSettingDialogOverrideCyclesAndPersists: the `i` dialog's adoption
// override cycles inherit -> on -> off -> inherit, persists in the v12 column
// at once (no dirty flag), and reopens on what was stored.
func TestAdoptSettingDialogOverrideCyclesAndPersists(t *testing.T) {
	db, id := newLaunchInputsTestStore(t)
	ctx := context.Background()
	for _, step := range []struct {
		wantMode eventHookMode
		wantCol  *bool
	}{{eventHookOn, boolPtrTUI(true)}, {eventHookOff, boolPtrTUI(false)}, {eventHookInherit, nil}} {
		m := adoptEditorModel(t, db, id)
		m = focusLaunchInputsField(t, m, launchInputsFieldAdoptNames)
		m = pressAll(m, "right")
		if m.launchInputsAdopt != step.wantMode {
			t.Fatalf("right selected %v, want %v", m.launchInputsAdopt, step.wantMode)
		}
		m = submitEditor(t, m)
		if m.launchInputsEditing || m.launchInputsNote != "" {
			t.Fatalf("a valid submit left the editor open (note %q)", m.launchInputsNote)
		}
		row, err := db.GetSession(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if (row.HarnessNameAdopt == nil) != (step.wantCol == nil) || (step.wantCol != nil && *row.HarnessNameAdopt != *step.wantCol) {
			t.Fatalf("HarnessNameAdopt = %v, want %v", row.HarnessNameAdopt, step.wantCol)
		}
		if row.EnvDirty || row.LaunchDirty {
			t.Fatalf("the adoption edit set env_dirty=%v launch_dirty=%v, want neither", row.EnvDirty, row.LaunchDirty)
		}
		if reopened := adoptEditorModel(t, db, id); reopened.launchInputsAdopt != step.wantMode {
			t.Fatalf("reopened editor shows %v, want %v", reopened.launchInputsAdopt, step.wantMode)
		}
	}
}

func TestAdoptSettingDialogRowIsNamedAndNotRestartToApply(t *testing.T) {
	db, id := newLaunchInputsTestStore(t)
	body := adoptEditorModel(t, db, id).launchInputsBody()
	if !strings.Contains(body, "harness_name_adopt") || !strings.Contains(body, "adopt_harness_names") {
		t.Fatalf("the dialog does not name the adoption override and its global:\n%s", body)
	}
}

func TestAdoptSettingDialogWithoutSetterReportsUnavailable(t *testing.T) {
	db, id := newLaunchInputsTestStore(t)
	m := launchInputsTestModel(t, db, id)
	m = focusLaunchInputsField(t, m, launchInputsFieldAdoptNames)
	m = pressAll(m, "right")
	got, cmd := m.Update(key("enter"))
	m = got.(Model)
	if cmd != nil || !strings.Contains(m.launchInputsNote, "unavailable") {
		t.Fatalf("submit without a setter: cmd=%v note=%q", cmd != nil, m.launchInputsNote)
	}
}

func boolPtrTUI(v bool) *bool { return &v }
