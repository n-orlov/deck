package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/store"
)

// maxEventLogRows is the `E` event log's own bound (SPEC \u00a712/requirement
// 32, task 124/I-9): however large the append-only events table grows over
// a long-running store's lifetime, the view fetches only this many of the
// newest rows.
const maxEventLogRows = 200

// maxEventLogPayloadRunes bounds each rendered payload cell. It is
// deliberately far smaller than a raw hook payload can be (RecordOrphanEvent
// preserves an entire unresolved hook JSON object verbatim) so one
// abnormally long payload can never push the dialog past the frame budget;
// a payload longer than this renders truncated with an ellipsis rather than
// wrapping the dialog to fit it.
const maxEventLogPayloadRunes = 96

// eventLogView renders task 124's `E` event log inside
// framedDialogScrollable (task 078, requirement 39 residual): every
// store.Event this store holds (across every session, newest first --
// see Store.ListEvents), each row's kind, reason and a bounded, masked
// payload. m.eventLogScroll (PgUp/PgDn, updateEventLog) selects the
// visible window once the log's up-to-200 rows push past the frame
// budget; Esc, also handled by updateEventLog, is the log's only other
// interaction. R61 (steer 3e-001 §6.3, SPEC §11.4's "a dialog never reads
// the store from its render path"): View() must never touch m.store on
// this path, so eventLogBody renders m.eventLogRows/m.eventLogErr, both
// state loaded exactly once per open by loadEventLog's tea.Cmd (see the
// "E" key handler in tui.go), never re-fetched here no matter how many
// reconcileTick/previewTick messages land while the dialog stays open.
func (m Model) eventLogView() string {
	return m.framedDialogScrollable(m.eventLogBody(), m.eventLogScroll)
}

// eventLogBody builds eventLogView's own content, split out (task 078) so
// updateEventLog's PgUp/PgDn handling can measure the same content
// dialogMaxScroll would, without duplicating the read/format logic. It
// reads only m.eventLogRows/m.eventLogErr (state loadEventLog populated
// when `E` opened the dialog) -- never m.store directly; see R61's comment
// on eventLogView above.
func (m Model) eventLogBody() string {
	var b strings.Builder
	b.WriteString("Event log\n\n")
	if m.store == nil {
		b.WriteString("(event log is unavailable: no store is attached)\n")
		return b.String()
	}
	if m.eventLogErr != nil {
		fmt.Fprintf(&b, "Cannot read events: %s\n", m.eventLogErr)
		return b.String()
	}
	if len(m.eventLogRows) == 0 {
		b.WriteString("(no events recorded yet)\n")
	} else {
		for _, event := range m.eventLogRows {
			reason := event.Reason
			if reason == "" {
				reason = "-"
			}
			payload := m.renderEventPayload(event.Payload)
			fmt.Fprintf(&b, "%-4s  %-22s %-20s %s\n", m.relativeAge(event.At), event.Kind, reason, payload)
		}
	}
	fmt.Fprintf(&b, "\nNewest first, up to the most recent %d events. Secret-shaped payload\nvalues mask the same way the env editor's do. Esc closes.\n", maxEventLogRows)
	return b.String()
}

// eventLogLoaded carries loadEventLog's ListEvents result back into
// Update (R61, steer 3e-001 §6.3): dispatched exactly once by the `E` key
// handler in tui.go when the dialog opens, never re-issued by any tick
// while it stays open, so the store read genuinely happens outside the
// render path rather than merely being framed to look that way.
type eventLogLoaded struct {
	events []store.Event
	err    error
}

// loadEventLog is the `E` event log's one store read (SPEC §11.4/§12,
// requirement 32, R61): issued once by the key handler that opens the
// dialog, mirroring loadArchivedSessions' own no-store-attached degrade
// (an empty result, never a panic). eventLogBody/View() never call the
// store directly -- only this tea.Cmd does, and only when the dialog is
// opening, not on every frame it stays open.
func (m Model) loadEventLog() tea.Msg {
	if m.store == nil {
		return eventLogLoaded{}
	}
	events, err := m.store.ListEvents(context.Background(), maxEventLogRows)
	return eventLogLoaded{events: events, err: err}
}

// updateEventLog handles keys while the `E` event log is open. Esc is the
// only field-like interaction (the log is read-only), handled through the
// shared §11.4 contract exactly like detailView/helpView's own single Esc
// case; PgUp/PgDn (task 078, requirement 39 residual) scroll the log's own
// window by a page, and up/down plus their j/k aliases by one line (R73,
// issue #7), once its content pushes past the frame budget.
func (m Model) updateEventLog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if cmd, handled := applyDialogContract(msg, dialogContract{Cancel: func() {
		m.eventLogOpen = false
	}}); handled {
		return m, cmd
	}
	switch msg.String() {
	case "pgup":
		m.eventLogScroll = m.dialogScrollByPage(m.eventLogScroll, m.eventLogBody(), -1)
	case "pgdown":
		m.eventLogScroll = m.dialogScrollByPage(m.eventLogScroll, m.eventLogBody(), 1)
	case "up", "k":
		// R73 (issue #7): one line per press, the arrows and their j/k
		// aliases both.
		m.eventLogScroll = m.dialogScrollByLines(m.eventLogScroll, m.eventLogBody(), -1)
	case "down", "j":
		m.eventLogScroll = m.dialogScrollByLines(m.eventLogScroll, m.eventLogBody(), 1)
	}
	return m, nil
}

// renderEventPayload is the event log's one display transformation of a
// raw events.payload column value: mask any secret-shaped key's value
// (maskEventPayload, task 010's predicate), then bound the result's length
// (truncateEventPayload) so one long payload cannot break the frame
// budget. Order matters: masking first means a value long enough to need
// truncation on its own is never partially revealed by truncating before
// masking gets to it.
func (m Model) renderEventPayload(payload string) string {
	return m.truncateEventPayload(m.maskEventPayload(payload))
}

// maskEventPayload applies task 010's single secret-shaped predicate
// (isSecretShapedKey / maskedSecretPlaceholder, internal/tui/secret.go) to
// a payload column value: if it decodes as a JSON object, every key
// matching isSecretShapedKey has its value replaced with the fixed
// placeholder and the object is re-marshalled; anything else (today, every
// real write path -- SetSessionEnvValue's own recorded payload is the
// changed key's NAME only, per SPEC's "env values never enter events"
// rule) passes through unchanged, since there is no value in it to mask.
// This is deliberately defensive rather than a no-op on the current write
// paths: the view masks whatever SHAPE of payload it is handed, not
// merely the shapes today's writers happen to produce, so a payload this
// store's caller could not anticipate is still checked the same way a
// live secret-shaped env value would be.
func (m Model) maskEventPayload(payload string) string {
	if payload == "" {
		return payload
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(payload), &obj); err != nil {
		return payload
	}
	masked := false
	for key, value := range obj {
		if _, ok := value.(string); !ok {
			continue
		}
		if !isSecretShapedKey(key) {
			continue
		}
		obj[key] = m.maskedSecretPlaceholder()
		masked = true
	}
	if !masked {
		return payload
	}
	rewritten, err := json.Marshal(obj)
	if err != nil {
		return payload
	}
	return string(rewritten)
}

// truncateEventPayload bounds a rendered payload cell at
// maxEventLogPayloadRunes runes, marking a genuinely truncated value with
// a trailing ellipsis (ASCII "..." under DECK_ASCII) rather than silently
// cutting it off indistinguishably from a payload that just happened to
// be exactly that long.
func (m Model) truncateEventPayload(payload string) string {
	runes := []rune(payload)
	if len(runes) <= maxEventLogPayloadRunes {
		return payload
	}
	return string(runes[:maxEventLogPayloadRunes]) + m.glyph("\u2026", "...")
}
