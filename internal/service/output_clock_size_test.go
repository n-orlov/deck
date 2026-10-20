package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// claudePane renders a claude-style pane at width: a `●` bullet whose text
// wraps at the width, the frame rules, the `❯` composer and a footer. A resize
// re-wraps the bullet, so the same words cut at another width are a different
// transcript region.
func claudePane(width int, footer string) string {
	words := strings.Fields("Reading the reconcile loop and the output clock to see how a resized pane " +
		"reflows its transcript before any agent output has arrived at all")
	var lines []string
	line := "●"
	for _, word := range words {
		if len(line)+1+len(word) > width {
			lines = append(lines, line)
			line = "  " + word
			continue
		}
		line += " " + word
	}
	lines = append(lines, line)
	rule := strings.Repeat("─", width)
	return strings.Join(lines, "\n") + "\n" + rule + "\n❯ \n" + rule + "\n" + footer + "\n"
}

// sizeRig drives ReconcileWithProbes against a scripted tmux whose
// list-panes reports the size in ./size and the activity in ./activity, whose
// capture-pane prints ./pane and, when ./size_after exists, then moves it over
// ./size: a resize landing between the facts read and the capture.
type sizeRig struct {
	t    *testing.T
	home string
	db   *store.Store
	svc  Service
	id   string
}

func newSizeRig(t *testing.T) *sizeRig {
	t.Helper()
	home := t.TempDir()
	clock, err := config.NewClock("2025-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
	paths := config.Paths{Home: home, LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")}
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	logger, err := audit.New(paths, clock)
	if err != nil {
		t.Fatal(err)
	}
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-0000000000d1", Name: "sized", CWD: t.TempDir(), Agent: "claude",
		CapturedPath: "/bin", Status: "idle", StatusSource: "hook", StatusAt: clock.Now().UnixMilli(), CreatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(home, "fake-tmux")
	body := strings.NewReplacer("@DIR@", home, "@SLUG@", session.Slug).Replace(`#!/bin/sh
for arg in "$@"; do
	case "$arg" in
	list-panes) printf 'deck_@SLUG@|%%1|4242|0|||%s|%s\n' "$(cat '@DIR@/size')" "$(cat '@DIR@/activity')"; exit 0 ;;
	capture-pane) echo x >> '@DIR@/captures'; cat '@DIR@/pane'
		if [ -f '@DIR@/size_after' ]; then mv '@DIR@/size_after' '@DIR@/size'; fi
		exit 0 ;;
	esac
done
exit 0
`)
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return &sizeRig{t: t, home: home, db: db, id: session.ID, svc: Service{
		Store: db, TMux: tmux.Client{Binary: script, Socket: "sized"}, Audit: logger, Clock: clock,
		Agents: agent.NewRegistry(), Output: NewOutputClock(10 * time.Second),
	}}
}

func (r *sizeRig) write(name, content string) {
	r.t.Helper()
	if err := os.WriteFile(filepath.Join(r.home, name), []byte(content), 0o600); err != nil {
		r.t.Fatal(err)
	}
}

// tick is one probing reconcile pass: window_activity seconds, the pane size
// as "width|height", the pane the capture will print and, optionally, the size
// the pane has by the time the capture has been read.
func (r *sizeRig) tick(activity, size, text, sizeAfter string) {
	r.t.Helper()
	r.write("activity", activity)
	r.write("size", size)
	r.write("pane", text)
	_ = os.Remove(filepath.Join(r.home, "size_after"))
	if sizeAfter != "" {
		r.write("size_after", sizeAfter)
	}
	if err := r.svc.ReconcileWithProbes(context.Background(), time.Hour); err != nil {
		r.t.Fatal(err)
	}
}

func (r *sizeRig) captures() int {
	r.t.Helper()
	data, err := os.ReadFile(filepath.Join(r.home, "captures"))
	if err != nil {
		return 0
	}
	return strings.Count(string(data), "x")
}

func (r *sizeRig) lastOutputAt() int64 {
	r.t.Helper()
	got, err := r.db.GetSession(context.Background(), r.id)
	if err != nil {
		r.t.Fatal(err)
	}
	return got.LastOutputAt
}

// storedReadingSize is the pane size the clock's stored reading for a session
// is keyed on (R409: the clock stores the pane size with each reading). It
// reads the reading by reflection so the table compiles against a clock whose
// reading carries no size, and reports ok=false there.
func storedReadingSize(c *OutputClock, sessionID string) (width, height int64, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	reading := reflect.ValueOf(c.state).MapIndex(reflect.ValueOf(sessionID))
	if !reading.IsValid() {
		return 0, 0, false
	}
	size := reading.FieldByName("size")
	if !size.IsValid() || size.Kind() != reflect.Struct {
		return 0, 0, false
	}
	w, h := size.FieldByName("Width"), size.FieldByName("Height")
	if !w.IsValid() || !h.IsValid() || w.Kind() != reflect.Int || h.Kind() != reflect.Int {
		return 0, 0, false
	}
	return w.Int(), h.Int(), true
}

// TestOutputClockSizeKeyedBaseline is R409's cases 1, 2, 3, 6 and 7, each a row
// that drives the real cut and hash (captureTranscriptReadings) over a
// claude-style pane. Tick 1 is the baseline at 80x24, activity 2000. Tick 2 is
// where the row differs. Tick 3 is footer churn in a later bucket (activity
// 2025) at the settled size. In no row does last_output_at move, the movement
// fallback's counted buckets (which only a write makes) stay at zero with it,
// and the stored reading ends keyed on the settled size.
//
// Row 7 is a resize and back to the original size before tick 2: tick 2's pane
// facts read 80x24 again, the redraw moved window_activity into a new bucket,
// and the capture is identical to the baseline.
func TestOutputClockSizeKeyedBaseline(t *testing.T) {
	const (
		wide, narrow = "80|24", "60|24"
		footer       = "~/proj  main  ctx:12%  opus"
		churn        = "~/proj  feature  ctx:41%  sonnet"
	)
	rows := []struct {
		name string
		// tick 2: activity, size the facts read, the width the pane is drawn
		// at when captured, and the size by the time the capture was read.
		activity     string
		size         string
		drawnAt      int
		sizeAfter    string
		wantCaptures int // cumulative, after tick 2
		settled      string
		settledWidth int
	}{
		{"1 resize with a reflow and no window_activity move", "2000", narrow, 60, "", 2, narrow, 60},
		{"2 resize with a reflow and window_activity moved in the same bucket", "2003", narrow, 60, "", 2, narrow, 60},
		{"3 resize with a reflow and window_activity moved into a new bucket", "2012", narrow, 60, "", 2, narrow, 60},
		{"6 the size changes between the facts read and the capture", "2012", wide, 60, narrow, 2, narrow, 60},
		{"7 a resize and back to the original size before the next tick, identical capture", "2012", wide, 80, "", 2, wide, 80},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			r := newSizeRig(t)
			r.tick("2000", wide, claudePane(80, footer), "")
			if got := r.captures(); got != 1 {
				t.Fatalf("baseline tick: %d captures, want 1", got)
			}
			r.tick(row.activity, row.size, claudePane(row.drawnAt, footer), row.sizeAfter)
			if got := r.lastOutputAt(); got != 0 {
				t.Fatalf("tick 2 wrote last_output_at = %d; a resize is never output", got)
			}
			if got := r.captures(); got != row.wantCaptures {
				t.Fatalf("tick 2: %d captures, want %d (a size change re-captures on that tick, bucket advanced or not)", got, row.wantCaptures)
			}
			r.tick("2025", row.settled, claudePane(row.settledWidth, churn), "")
			if got := r.lastOutputAt(); got != 0 {
				t.Fatalf("footer churn in a later bucket wrote last_output_at = %d", got)
			}
			if r.svc.Output.Moving(r.id) {
				t.Fatal("the movement fallback saw movement on a resize")
			}
			w, h, ok := storedReadingSize(r.svc.Output, r.id)
			if !ok {
				t.Fatal("the stored reading carries no pane size (R409: the clock stores the size with each reading)")
			}
			if got := fmt.Sprintf("%d|%d", w, h); got != row.settled {
				t.Fatalf("the stored reading is keyed on %s, want the settled size %s", got, row.settled)
			}
		})
	}
}
