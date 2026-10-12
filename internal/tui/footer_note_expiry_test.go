package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/config"
)

// expiryRows is one row per R280 source the [ui] note_timeout tick ends
// (R283): how to set the note through its writer, and how to tell it is up.
var expiryRows = []struct {
	name string
	src  footerNoteSource
	set  func(m *Model, text string)
	up   func(m Model) bool
}{
	{"attachError", noteAttachError, func(m *Model, s string) { m.setAttachError(s) }, func(m Model) bool { return m.attachError != "" }},
	{"resumeNote", noteResume, func(m *Model, s string) { m.setResumeNote(s) }, func(m Model) bool { return m.resumeNote != "" }},
	{"selectionCopyNote", noteSelectionCopy, func(m *Model, s string) { m.setSelectionCopyNote(s) }, func(m Model) bool { return m.selectionCopyNote != "" }},
	{"fieldCopyNote", noteFieldCopy, func(m *Model, s string) { m.setFieldCopyNote(s) }, func(m Model) bool { return m.fieldCopyNote != "" }},
	{"archive-undone-rebuild", noteArchiveUndoneRebuild, func(m *Model, _ string) {
		m.archiveUndoneRebuildNote = true
		m.archiveUndoneRebuildGeneration++
	}, func(m Model) bool { return m.archiveUndoneRebuildNote }},
}

func expiryModel() Model {
	m := New(nil, config.Settings{NoteTimeout: time.Millisecond}, "")
	m.width, m.height = 120, 30
	return m
}

// setThrough applies a note write the way Update's tail sees it and returns
// the model plus the cmd the stamp step scheduled.
func setThrough(prev Model, set func(m *Model)) (Model, tea.Cmd) {
	next := prev
	set(&next)
	model, cmd := stampFooterNotes(prev, next, nil)
	return model.(Model), cmd
}

// expiryMsgs runs cmd (one tick, or a tea.Batch of ticks and nothing else
// here) and returns every footerNoteExpired it produced within a second.
func expiryMsgs(t *testing.T, cmd tea.Cmd) []footerNoteExpired {
	t.Helper()
	if cmd == nil {
		return nil
	}
	run := func(c tea.Cmd) tea.Msg {
		done := make(chan tea.Msg, 1)
		go func() { done <- c() }()
		select {
		case msg := <-done:
			return msg
		case <-time.After(time.Second):
			t.Fatal("a scheduled expiry tick did not fire within a second of a 1ms note_timeout")
			return nil
		}
	}
	msgs := []tea.Msg{run(cmd)}
	if batch, ok := msgs[0].(tea.BatchMsg); ok {
		msgs = nil
		for _, c := range batch {
			if c != nil {
				msgs = append(msgs, run(c))
			}
		}
	}
	var out []footerNoteExpired
	for _, msg := range msgs {
		if e, ok := msg.(footerNoteExpired); ok {
			out = append(out, e)
		}
	}
	return out
}

func update(m Model, msg tea.Msg) Model {
	got, _ := m.Update(msg)
	return got.(Model)
}

// TestFooterNoteExpiryClearsTheSlot is R283: a note on each source the key
// governs schedules one tick, and the tick clears the slot.
func TestFooterNoteExpiryClearsTheSlot(t *testing.T) {
	for _, row := range expiryRows {
		t.Run(row.name, func(t *testing.T) {
			m, cmd := setThrough(expiryModel(), func(m *Model) { row.set(m, "a note") })
			if !row.up(m) || m.footerNote() == "" {
				t.Fatal("test setup: the note is not up")
			}
			msgs := expiryMsgs(t, cmd)
			if len(msgs) != 1 || msgs[0].src != row.src {
				t.Fatalf("scheduled ticks = %+v, want exactly one for source %d", msgs, row.src)
			}
			m = update(m, msgs[0])
			if row.up(m) || m.footerNote() != "" {
				t.Fatalf("note still up after its tick: footerNote() = %q", m.footerNote())
			}
		})
	}
}

// TestFooterNoteExpiryIgnoresATickFromAnOlderNote is R283's generation
// guard: renewing a note with identical text, or replacing it with another,
// leaves the first tick unable to clear the successor; the successor's own
// tick does. A tick for one source never clears another source's note.
func TestFooterNoteExpiryIgnoresATickFromAnOlderNote(t *testing.T) {
	for _, row := range expiryRows {
		for _, replacement := range []string{"a note", "a different note"} {
			t.Run(row.name+"/"+replacement, func(t *testing.T) {
				m, first := setThrough(expiryModel(), func(m *Model) { row.set(m, "a note") })
				old := expiryMsgs(t, first)
				m, second := setThrough(m, func(m *Model) { row.set(m, replacement) })
				current := expiryMsgs(t, second)
				if len(old) != 1 || len(current) != 1 {
					t.Fatalf("ticks: first %+v, second %+v, want one each", old, current)
				}
				m = update(m, old[0])
				if !row.up(m) {
					t.Fatal("the older note's tick cleared the note that replaced it")
				}
				m = update(m, current[0])
				if row.up(m) {
					t.Fatal("the current note's own tick left it up")
				}
			})
		}
	}

	t.Run("another source", func(t *testing.T) {
		m, cmd := setThrough(expiryModel(), func(m *Model) { m.setFieldCopyNote("copied") })
		fieldTick := expiryMsgs(t, cmd)
		m, _ = setThrough(m, func(m *Model) { m.setAttachError("cannot attach") })
		m = update(m, fieldTick[0])
		if m.fieldCopyNote != "" || m.attachError == "" {
			t.Fatalf("field tick left fieldCopyNote=%q attachError=%q, want it to clear only its own source", m.fieldCopyNote, m.attachError)
		}
	})
}

// TestFooterNoteExpiryReachesTheRealWritersThroughUpdate drives the real
// paths: the A key with no archiver raises attachError, and Update itself
// (not the test) schedules the tick that clears it.
func TestFooterNoteExpiryReachesTheRealWritersThroughUpdate(t *testing.T) {
	m := footerNoteFrameModel(120, 30, false)
	m.settings.NoteTimeout = time.Millisecond
	got, cmd := m.Update(key("A"))
	m = got.(Model)
	if m.attachError == "" {
		t.Fatal("test setup: A raised no attachError")
	}
	msgs := expiryMsgs(t, cmd)
	if len(msgs) != 1 || msgs[0].src != noteAttachError {
		t.Fatalf("ticks from Update = %+v, want one for attachError", msgs)
	}
	m = update(m, msgs[0])
	if m.attachError != "" {
		t.Fatalf("attachError = %q after the tick Update scheduled", m.attachError)
	}
}

// TestFooterNoteExpiryLeavesUndoStyleAndTeardownNotesAlone pins R283's
// exceptions: the five undo-style notes and the teardown hook note have no
// note_timeout tick, and a forged one for them clears nothing.
func TestFooterNoteExpiryLeavesUndoStyleAndTeardownNotesAlone(t *testing.T) {
	m := expiryModel()
	m.undoSessionID, m.undoSessionName, m.undoGeneration = "s1", "alpha", 1
	m.batchUndoSessionIDs, m.batchUndoGeneration = []string{"s1", "s2"}, 1
	m.deleteUndoSessionID, m.deleteUndoGeneration = "s1", 1
	m.batchDeleteUndoSessionIDs, m.batchDeleteUndoGeneration = []string{"s1", "s2"}, 1
	m.archiveUndoSessionID, m.archiveUndoGeneration = "s1", 1
	m.teardownHookNote, m.teardownHookNoteGeneration = "post_destroy failed", 1
	for _, src := range []footerNoteSource{noteKillUndo, noteKillBatchUndo, noteDeleteUndo, noteDeleteBatchUndo, noteArchiveUndo, noteTeardownHook} {
		if m.footerNoteExpiryTick(src) != nil {
			t.Errorf("source %d has a note_timeout tick", src)
		}
		if m.footerNoteText(src) == "" {
			t.Fatalf("test setup: source %d is not set", src)
		}
		after := update(m, footerNoteExpired{src: src, gen: footerNoteRenewal[src](m)})
		if after.footerNoteText(src) == "" {
			t.Errorf("a forged note_timeout tick cleared source %d", src)
		}
	}
}

// TestFooterNoteExpiryFollowsTheConfiguredDuration shows the tick waits for
// Settings.NoteTimeout, not a constant: a long timeout has not fired after
// the short one has.
func TestFooterNoteExpiryFollowsTheConfiguredDuration(t *testing.T) {
	m := New(nil, config.Settings{NoteTimeout: time.Hour}, "")
	tick := m.footerNoteExpiryTick(noteAttachError)
	fired := make(chan struct{})
	go func() { tick(); close(fired) }()
	select {
	case <-fired:
		t.Fatal("a one-hour note_timeout tick fired at once")
	case <-time.After(100 * time.Millisecond):
	}
	short := New(nil, config.Settings{NoteTimeout: 20 * time.Millisecond}, "")
	start := time.Now()
	short.footerNoteExpiryTick(noteAttachError)()
	if elapsed := time.Since(start); elapsed < 15*time.Millisecond || elapsed > time.Second {
		t.Fatalf("a 20ms note_timeout tick fired after %s", elapsed)
	}
}

// TestFooterNoteExpiryNeedsAConfiguredDuration: a Settings with no
// NoteTimeout (never the case under config.Load, which resolves at least one
// second) schedules no expiry rather than clearing notes at once.
func TestFooterNoteExpiryNeedsAConfiguredDuration(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	if m.footerNoteExpiryTick(noteAttachError) != nil {
		t.Fatal("a zero NoteTimeout scheduled an expiry tick")
	}
	_, cmd := setThrough(m, func(m *Model) { m.setAttachError("x") })
	if cmd != nil {
		t.Fatal("a zero NoteTimeout made the note write return a command")
	}
}
