package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// R233a: a session's own event-hook controls, event_hook_enabled and
// event_hook_events (SPEC §10.2), are edited in the launch-inputs editor and
// the create dialog. They are read at dispatch, so an edit applies at once and
// sets neither env_dirty nor launch_dirty.

// eventHookEditorModel opens the launch-inputs editor on the seeded session
// with the real service's SetEventHook wired in, as cmd/deck/main.go wires it.
func eventHookEditorModel(t *testing.T, db *store.Store, id string) Model {
	t.Helper()
	clock, err := config.NewClock("2025-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
	svc := service.Service{Store: db, Clock: clock}
	return launchInputsTestModel(t, db, id).WithEventHookSetter(svc.SetEventHook)
}

// submitEditor presses enter, runs the dispatched command and hands its
// reply back to the model, as the program loop does.
func submitEditor(t *testing.T, m Model) Model {
	t.Helper()
	got, cmd := m.Update(key("enter"))
	m = got.(Model)
	if cmd == nil {
		t.Fatalf("enter dispatched no command (note %q)", m.launchInputsNote)
	}
	got, _ = m.Update(cmd())
	return got.(Model)
}

// focusLaunchInputsField moves the editor's focus to field with ↓.
func focusLaunchInputsField(t *testing.T, m Model, field int) Model {
	t.Helper()
	for m.launchInputsField != field {
		got, _ := m.Update(key("down"))
		m = got.(Model)
	}
	return m
}

func TestLaunchInputsEditorEventHookFieldsRoundTripToTheStore(t *testing.T) {
	db, id := newLaunchInputsTestStore(t)
	ctx := context.Background()

	// on + a list of two kinds.
	m := eventHookEditorModel(t, db, id)
	m = focusLaunchInputsField(t, m, launchInputsFieldEventHook)
	got, _ := m.Update(key("right"))
	m = got.(Model)
	if m.launchInputsEventHook != eventHookOn {
		t.Fatalf("right on inherit selected %v, want on", m.launchInputsEventHook)
	}
	m = focusLaunchInputsField(t, m, launchInputsFieldEventHookEvents)
	m = typeInto(t, m, "error, ended")
	m = submitEditor(t, m)
	if m.launchInputsEditing || m.launchInputsNote != "" {
		t.Fatalf("a valid submit left the editor open (note %q)", m.launchInputsNote)
	}
	row, err := db.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if row.EventHookEnabled == nil || !*row.EventHookEnabled {
		t.Fatalf("EventHookEnabled = %v, want on", row.EventHookEnabled)
	}
	if want := []string{"error", "ended"}; !reflect.DeepEqual(row.EventHookEvents, want) {
		t.Fatalf("EventHookEvents = %#v, want %#v", row.EventHookEvents, want)
	}

	// Reopening shows what was stored; off + "none" is the empty list, which
	// is a value of its own, distinct from inheriting.
	m = eventHookEditorModel(t, db, id)
	if m.launchInputsEventHook != eventHookOn || m.launchInputsValue(launchInputsFieldEventHookEvents) != "error, ended" {
		t.Fatalf("reopened editor shows %v / %q, want on / %q", m.launchInputsEventHook, m.launchInputsValue(launchInputsFieldEventHookEvents), "error, ended")
	}
	m = focusLaunchInputsField(t, m, launchInputsFieldEventHook)
	m = pressAll(m, "right") // on -> off
	m = focusLaunchInputsField(t, m, launchInputsFieldEventHookEvents)
	m = typeInto(t, m, eventKindsNone)
	m = submitEditor(t, m)
	row, _ = db.GetSession(ctx, id)
	if row.EventHookEnabled == nil || *row.EventHookEnabled {
		t.Fatalf("EventHookEnabled = %v, want off", row.EventHookEnabled)
	}
	if row.EventHookEvents == nil || len(row.EventHookEvents) != 0 {
		t.Fatalf("EventHookEvents = %#v, want the empty (non-nil) list", row.EventHookEvents)
	}

	// Back to inherit for both: NULL, NULL.
	m = eventHookEditorModel(t, db, id)
	m = focusLaunchInputsField(t, m, launchInputsFieldEventHook)
	m = pressAll(m, "right") // off -> inherit
	m = focusLaunchInputsField(t, m, launchInputsFieldEventHookEvents)
	m = pressAll(m, "backspace", "backspace", "backspace", "backspace", "backspace") // clears "none" and the stray slot
	m = submitEditor(t, m)
	row, _ = db.GetSession(ctx, id)
	if row.EventHookEnabled != nil || row.EventHookEvents != nil {
		t.Fatalf("inherit stored %v / %#v, want nil / nil", row.EventHookEnabled, row.EventHookEvents)
	}
}

func TestLaunchInputsEditorEventHookEditSetsNeitherDirtyFlag(t *testing.T) {
	db, id := newLaunchInputsTestStore(t)
	m := eventHookEditorModel(t, db, id)
	m = focusLaunchInputsField(t, m, launchInputsFieldEventHook)
	m = pressAll(m, "right")
	m = focusLaunchInputsField(t, m, launchInputsFieldEventHookEvents)
	m = typeInto(t, m, "waiting")
	m = submitEditor(t, m)

	row, err := db.GetSession(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if row.EnvDirty || row.LaunchDirty {
		t.Fatalf("event-hook edit set env_dirty=%v launch_dirty=%v, want neither", row.EnvDirty, row.LaunchDirty)
	}
	if row.EventHookEnabled == nil || !*row.EventHookEnabled {
		t.Fatalf("the edit did not apply: EventHookEnabled = %v", row.EventHookEnabled)
	}
	// And the visible badge agrees: no restart-pending cue for the row.
	listed := m.sessions[0]
	if listed.EnvDirty || listed.LaunchDirty {
		t.Fatalf("the model's row carries a dirty flag: %+v", listed)
	}
}

// A launch-input edit made together with an event-hook edit still marks
// launch_dirty (the launch input needs a restart), but the event-hook fields
// are stored too.
func TestLaunchInputsEditorLaunchInputStillMarksLaunchDirtyBesideEventHook(t *testing.T) {
	db, id := newLaunchInputsTestStore(t)
	m := eventHookEditorModel(t, db, id)
	m = typeInto(t, m, "echo pre")
	m = focusLaunchInputsField(t, m, launchInputsFieldEventHook)
	m = pressAll(m, "right")
	submitEditor(t, m)
	row, _ := db.GetSession(context.Background(), id)
	if !row.LaunchDirty || row.PreLaunch != "echo pre" {
		t.Fatalf("launch input not stored as dirty: %+v", row)
	}
	if row.EventHookEnabled == nil || !*row.EventHookEnabled {
		t.Fatalf("event hook not stored beside it: %v", row.EventHookEnabled)
	}
}

func TestLaunchInputsEditorRefusesAnUnofferedEventKindInDialog(t *testing.T) {
	db, id := newLaunchInputsTestStore(t)
	m := eventHookEditorModel(t, db, id)
	m = focusLaunchInputsField(t, m, launchInputsFieldEventHookEvents)
	m = typeInto(t, m, "waiting, bogus")
	got, cmd := m.Update(key("enter"))
	m = got.(Model)
	if cmd != nil {
		t.Fatal("an unoffered kind dispatched a write")
	}
	if !m.launchInputsEditing || !strings.Contains(m.launchInputsNote, "bogus") || !strings.Contains(m.launchInputsNote, "event_hook_events") {
		t.Fatalf("editing=%v note=%q, want the editor open and a note naming event_hook_events and bogus", m.launchInputsEditing, m.launchInputsNote)
	}
	if m.launchInputsValue(launchInputsFieldEventHookEvents) != "waiting, bogus" {
		t.Fatalf("typed text not retained: %q", m.launchInputsValue(launchInputsFieldEventHookEvents))
	}
}

func TestLaunchInputsEditorRendersTheEventHookRowsAsNotRestartToApply(t *testing.T) {
	db, id := newLaunchInputsTestStore(t)
	m := eventHookEditorModel(t, db, id)
	body := m.launchInputsBody()
	for _, want := range []string{"Event hook (event_hook_enabled): inherit", "Event hook kinds (event_hook_events):", "applies immediately, no restart"} {
		if !strings.Contains(body, want) {
			t.Errorf("launch-inputs body lacks %q:\n%s", want, body)
		}
	}
	rows := m.launchInputsFieldRows()
	for _, row := range rows[launchInputsFieldEventHook:] {
		if strings.Contains(row.help, "restart-to-apply") {
			t.Errorf("row %q is labelled restart-to-apply: %q", row.label, row.help)
		}
	}
}

// The create dialog carries both fields and hands them to the create call.
func TestCreateDialogEventHookFieldsReachTheStore(t *testing.T) {
	db, _ := newLaunchInputsTestStore(t)
	for _, agent := range []string{"shell", "claude"} {
		t.Run(agent, func(t *testing.T) {
			m := newCreatingModel(t)
			m.createAgent = agent
			m.setCreateText(createFieldName, "hooked-"+agent)
			m.createField = createFieldEventHook
			m = pressAll(m, "left") // inherit -> off
			if m.createEventHook != eventHookOff {
				t.Fatalf("left on inherit selected %v, want off", m.createEventHook)
			}
			m = pressAll(m, "right", "right") // off -> inherit -> on
			m.createField = createFieldEventHookEvents
			m = typeInto(t, m, "idle, killed")

			var stored store.Session
			save := func(ctx context.Context, name string, enabled *bool, events []string) (store.Session, error) {
				// The same field-for-field mapping service.insertShellRow and
				// insertAgentRow make onto store.CreateSessionInput.
				s, err := db.CreateSession(ctx, store.CreateSessionInput{
					ID: "id-" + name, Name: name, CWD: "/work/" + name, Agent: "shell", CapturedPath: "/bin",
					StatusAt: 1, CreatedAt: 1, EventHookEnabled: enabled, EventHookEvents: events,
				})
				stored = s
				return s, err
			}
			m.create = func(ctx context.Context, in service.ShellCreateInput) (store.Session, error) {
				return save(ctx, in.Name, in.EventHookEnabled, in.EventHookEvents)
			}
			m.createAgentSession = func(ctx context.Context, in service.AgentCreateInput) (store.Session, error) {
				return save(ctx, in.Name, in.EventHookEnabled, in.EventHookEvents)
			}
			got, cmd := m.Update(key("enter"))
			if after := got.(Model); after.createError != "" {
				t.Fatalf("createError = %q", after.createError)
			}
			if cmd == nil {
				t.Fatal("enter issued no create command")
			}
			cmd()
			row, err := db.GetSession(context.Background(), stored.ID)
			if err != nil {
				t.Fatal(err)
			}
			if row.EventHookEnabled == nil || !*row.EventHookEnabled {
				t.Fatalf("EventHookEnabled = %v, want on", row.EventHookEnabled)
			}
			if want := []string{"idle", "killed"}; !reflect.DeepEqual(row.EventHookEvents, want) {
				t.Fatalf("EventHookEvents = %#v, want %#v", row.EventHookEvents, want)
			}
		})
	}
}

func TestCreateDialogLeavesEventHookToInheritByDefault(t *testing.T) {
	m := newCreatingModel(t)
	var got service.ShellCreateInput
	m.create = func(_ context.Context, in service.ShellCreateInput) (store.Session, error) {
		got = in
		return store.Session{Name: in.Name}, nil
	}
	_, cmd := m.Update(key("enter"))
	cmd()
	if got.EventHookEnabled != nil || got.EventHookEvents != nil {
		t.Fatalf("untouched dialog sent %v / %#v, want inherit / inherit", got.EventHookEnabled, got.EventHookEvents)
	}
}

func TestCreateDialogRefusesAnUnofferedEventKind(t *testing.T) {
	m := newCreatingModel(t)
	m.setCreateText(createFieldEventHookEvents, "waiting, nope")
	got, cmd := m.Update(key("enter"))
	after := got.(Model)
	if cmd != nil || !strings.Contains(after.createError, "nope") || !after.creating {
		t.Fatalf("cmd=%v createError=%q creating=%v, want a refusal naming nope in the open dialog", cmd != nil, after.createError, after.creating)
	}
	if after.createText(createFieldEventHookEvents) != "waiting, nope" {
		t.Fatalf("typed text not retained: %q", after.createText(createFieldEventHookEvents))
	}
}

func TestCreateDialogRowsNameBothEventHookFields(t *testing.T) {
	m := newCreatingModel(t)
	body := m.createBody()
	for _, want := range []string{"Event hook (event_hook_enabled): inherit", "Event hook kinds (event_hook_events):"} {
		if !strings.Contains(body, want) {
			t.Errorf("create body lacks %q", want)
		}
	}
}

func TestParseEventKindsDistinguishesInheritFromNone(t *testing.T) {
	cases := []struct {
		text string
		want []string
		bad  bool
	}{
		{"", nil, false},
		{"  ", nil, false},
		{"none", []string{}, false},
		{"waiting,error", []string{"waiting", "error"}, false},
		{"waiting, error ,", []string{"waiting", "error"}, false},
		{"none, error", nil, true},
		{"loud", nil, true},
	}
	for _, c := range cases {
		got, err := parseEventKinds(c.text)
		if (err != nil) != c.bad || !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseEventKinds(%q) = %#v, %v; want %#v, bad=%v", c.text, got, err, c.want, c.bad)
		}
	}
	if eventKindsText(nil) != "" || eventKindsText([]string{}) != "none" || eventKindsText([]string{"idle", "ended"}) != "idle, ended" {
		t.Error("eventKindsText does not invert parseEventKinds")
	}
}

// The Group row stays the last create field (the event-hook rows sit before
// it), and cycling it still moves through the groups and clears "(last used)".
func TestCreateDialogGroupStaysLastAndCycles(t *testing.T) {
	m := newCreatingModel(t)
	m.createGroups = []store.Group{{ID: 7, Name: "sprint"}}
	m.createGroupID, m.createGroupLastUsed = 0, true
	if createFieldGroup != createFieldCount-1 {
		t.Fatalf("Group is field %d of %d, want the last", createFieldGroup, createFieldCount)
	}
	m.createField = createFieldGroup
	m = pressAll(m, "right")
	if m.createGroupID != 7 || m.createGroupLastUsed {
		t.Fatalf("after right: group %d lastUsed=%v, want 7 / false", m.createGroupID, m.createGroupLastUsed)
	}
	m = pressAll(m, "right")
	if m.createGroupID != 0 {
		t.Fatalf("a second right did not wrap to default: %d", m.createGroupID)
	}
}
