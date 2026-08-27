package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// This file is task 021's own test obligation for the `E` event log, the
// same shape rename_theme_test.go carries for the rename sub-dialog: the
// dialog must render in SPEC.md:1355's §11.6 tokens at render time, with
// no change to eventLogBody's own plain visible text.

// task021EventLogModel opens the event log with colour enabled and one
// row loaded through the real loadEventLog path (never a hand-built
// []store.Event slice; openEventLogTestStore is event_log_test.go's own
// shared fixture), so every token this task adds can be read off a real
// grid built from a real store read.
func task021EventLogModel(t *testing.T) Model {
	t.Helper()
	db := openEventLogTestStore(t)
	ctx := context.Background()
	if err := db.RecordOrphanEvent(ctx, store.EventInput{At: 1, Kind: "archived", Reason: "user", Payload: "plain-value"}); err != nil {
		t.Fatal(err)
	}
	m := New(db, config.Settings{Color: true}, "")
	m.width, m.height = 100, 40
	m.eventLogOpen = true
	loaded := m.loadEventLog().(eventLogLoaded)
	m.eventLogRows = loaded.events
	m.eventLogErr = loaded.err
	return m
}

// TestEventLogStyledBodyMatchesPlainBodyOnceStripped proves the styled
// body task 021 adds is the same dialog as eventLogBody -- what
// event_log_test.go's existing substring assertions check against -- with
// only colour added: once every escape sequence is stripped back out of
// BOTH sides, styledEventLogBody is byte-identical to
// wrapDialogLines(eventLogBody()) line for line, across the no-store,
// read-error, empty and populated branches alike.
func TestEventLogStyledBodyMatchesPlainBodyOnceStripped(t *testing.T) {
	for _, tc := range []struct {
		name string
		m    func(t *testing.T) Model
	}{
		{"no store", func(t *testing.T) Model {
			m := New(nil, config.Settings{Color: true}, "")
			m.width, m.height = 100, 40
			m.eventLogOpen = true
			return m
		}},
		{"read error", func(t *testing.T) Model {
			m := task021EventLogModel(t)
			m.eventLogErr = errRenameCollisionForTest
			return m
		}},
		{"empty", func(t *testing.T) Model {
			db := openEventLogTestStore(t)
			m := New(db, config.Settings{Color: true}, "")
			m.width, m.height = 100, 40
			m.eventLogOpen = true
			loaded := m.loadEventLog().(eventLogLoaded)
			m.eventLogRows = loaded.events
			m.eventLogErr = loaded.err
			return m
		}},
		{"populated", func(t *testing.T) Model { return task021EventLogModel(t) }},
	} {
		m := tc.m(t)

		plain := m.wrapDialogLines(m.eventLogBody())
		styled := strings.Split(m.styledEventLogBody(), "\n")
		if len(styled) != len(plain) {
			t.Fatalf("%s: styled body has %d physical lines, plain has %d:\nplain: %q\nstyled: %q", tc.name, len(styled), len(plain), plain, styled)
		}
		for i := range plain {
			if got, want := stripANSI(styled[i]), stripANSI(plain[i]); got != want {
				t.Fatalf("%s: line %d differs once escapes are stripped:\n styled: %q\n  plain: %q", tc.name, i, got, want)
			}
		}
	}
}

// TestEventLogViewCarriesSectionTokens proves the rendered event log
// actually carries §11.6 SGR escapes at render time (not merely that
// styledEventLogBody's plain text matches, which the test above already
// covers): the title, an event row's value text and the closing
// sentence's "Esc" each carry their own colour once escapes are counted
// rather than stripped.
func TestEventLogViewCarriesSectionTokens(t *testing.T) {
	m := task021EventLogModel(t)
	th := m.activeTheme()

	titleSGR, ok := m.sgrForToken(th, theme.Title)
	if !ok {
		t.Fatal("theme has no title token")
	}
	textSGR, ok := m.sgrForToken(th, theme.Text)
	if !ok {
		t.Fatal("theme has no text token")
	}
	keySGR, ok := m.sgrForToken(th, theme.Key)
	if !ok {
		t.Fatal("theme has no key token")
	}

	view := m.View()
	if !strings.Contains(view, titleSGR+"Event log") {
		t.Fatalf("event log title is not coloured with theme.Title:\n%q", view)
	}
	if !strings.Contains(view, textSGR) {
		t.Fatalf("event log row is not coloured with theme.Text:\n%q", view)
	}
	if !strings.Contains(view, keySGR+"Esc") {
		t.Fatalf("event log closing sentence's Esc is not coloured with theme.Key:\n%q", view)
	}
	if !strings.Contains(stripANSI(view), "plain-value") {
		t.Fatalf("event log lost its plain visible text once escapes are stripped:\n%s", stripANSI(view))
	}
}
